package orchestrator

// journal.go — La cola de escrituras, en disco y cifrada.
//
// Por qué un archivo y no otra tabla de SQLite: la cola tiene que sobrevivir a
// un cierre eléctrico sin depender de que la base de datos esté sana, y tiene
// que poder afirmarse o negarse sin abrir la base ("¿hay cola?"). Un archivo
// con cabecera lo resuelve: si existe y supera la cabecera, hay cola.
//
// FORMATO. Cabecera fija de 32 bytes y luego registros encadenados:
//
//	magic     8 bytes  "ERPJRNL\x01"
//	version   4 bytes  uint32 little-endian
//	flags     4 bytes  bit 0 = cifrado
//	correl.   8 bytes  uint64, el último correlativo entregado
//	reservado 8 bytes
//
// Cada registro es:  uint32 longitud || nonce 12 || texto cifrado (AES-GCM)
// o, sin cifrar:     uint32 longitud || texto en claro.
//
// Sin cabecera no hay formato, así que un archivo que no empieza por el magic
// no es del orquestador y no se toca: se devuelve un error en vez de adivinar.
// Adivinar es como se corrompe un archivo.
//
// SOBRE EL CORRELATIVO. Vive en la cabecera, no en un contador en memoria, y
// por eso la limpieza no puede reiniciarlo en 1: limpiar reescribe la cabecera
// con el MISMO correlativo. Es la posición en la fila, no el id de SQLite: los
// dos mundos no se tocan, y dos escrituras del mismo segundo siguen teniendo
// orden.
//
// Si el proceso muere entre actualizar la cabecera y escribir el registro, el
// correlativo queda quemado (un hueco en la numeración). Eso es lo correcto:
// un hueco se ve; un correlativo repetido rompe el orden sin avisar.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	journalMagic   = "ERPJRNL\x01"
	journalVersion = uint32(1)
	journalHeader  = 32
	flagCifrado    = uint32(1)
)

// maxRegistro acota un registro. 8 MB es de sobra para una factura con sus
// líneas y es un tope real: sin él, un payload enorme convertiría la escritura
// en un problema de memoria.
const maxRegistro = 8 << 20

// entradaDisco es lo que viaja en el journal. Los nombres de campo son cortos
// porque el archivo se lee entero en cada arranque.
type entradaDisco struct {
	Correlativo uint64   `json:"c"`
	Empresa     string   `json:"em"`
	Clave       string   `json:"k"`
	Tabla       string   `json:"t"`
	Columnas    []string `json:"co"`
	Valores     []any    `json:"v"`
	Modulo      string   `json:"m"`
	Usuario     string   `json:"u"`
	IP          string   `json:"i"`
	Accion      string   `json:"a"`
	RegistroID  string   `json:"ri"`
	OldValue    any      `json:"ov,omitempty"`
	NewValue    any      `json:"nv,omitempty"`
	Creada      string   `json:"cr"`
}

// journal es el archivo de cola. No es seguro para uso concurrente: lo lleva
// el orquestador, que ya serializa a un solo escritor.
type journal struct {
	ruta        string
	f           *os.File
	aead        cipher.AEAD
	cifrado     bool
	correlativo uint64
	size        int64
}

// abrirJournal crea o abre la cola. Si el archivo no existe se crea con la
// cabecera; si existe se lee la cabecera y se continúa donde quedó.
func abrirJournal(ruta, clave string) (*journal, error) {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return nil, fmt.Errorf("orchestrator: no se pudo crear el directorio del journal: %w", err)
	}

	// La clave se deriva con SHA-256 a partir de la FieldKey. Sin FieldKey el
	// journal va en claro, que es el caso de desarrollo: la base está abierta
	// igualmente y cifrar aquí solo daría una falsa sensación.
	aead, cifrado, err := derivarAEAD(clave)
	if err != nil {
		return nil, err
	}

	j := &journal{ruta: ruta, aead: aead, cifrado: cifrado}

	var info os.FileInfo
	if st, errStat := os.Stat(ruta); errStat == nil && st.Size() > 0 {
		info = st
	}

	banderas := uint32(0)
	if cifrado {
		banderas |= flagCifrado
	}
	cabecera := cabeceraNueva(banderas, 0)

	f, err := os.OpenFile(ruta, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: no se pudo abrir el journal: %w", err)
	}
	j.f = f

	if info != nil {
		if err := j.leerCabecera(); err != nil {
			f.Close()
			return nil, err
		}
		return j, nil
	}
	if _, err := f.WriteAt(cabecera, 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("orchestrator: no se pudo escribir la cabecera: %w", err)
	}
	j.correlativo = 0
	j.size = journalHeader
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, fmt.Errorf("orchestrator: no se pudo sincronizar la cabecera: %w", err)
	}
	return j, nil
}

// cabeceraNueva arma los 32 bytes de cabecera.
func cabeceraNueva(banderas uint32, correlativo uint64) []byte {
	cabecera := make([]byte, journalHeader)
	copy(cabecera, journalMagic)
	binary.LittleEndian.PutUint32(cabecera[8:], journalVersion)
	binary.LittleEndian.PutUint32(cabecera[12:], banderas)
	binary.LittleEndian.PutUint64(cabecera[16:], correlativo)
	return cabecera
}

// derivarAEAD construye el cifrador a partir de la FieldKey.
func derivarAEAD(clave string) (cipher.AEAD, bool, error) {
	if clave == "" {
		return nil, false, nil
	}
	resumen := sha256.Sum256([]byte("erpgo-orchestrator-v1\x00" + clave))
	bloque, err := aes.NewCipher(resumen[:])
	if err != nil {
		return nil, false, fmt.Errorf("orchestrator: no se pudo preparar el cifrado: %w", err)
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, false, fmt.Errorf("orchestrator: no se pudo preparar el cifrado: %w", err)
	}
	return aead, true, nil
}

// leerCabecera valida el archivo y carga el correlativo y el tamaño.
func (j *journal) leerCabecera() error {
	cabecera := make([]byte, journalHeader)
	if _, err := io.ReadFull(io.NewSectionReader(j.f, 0, journalHeader), cabecera); err != nil {
		return fmt.Errorf("orchestrator: cabecera del journal ilegible: %w", err)
	}
	if string(cabecera[:8]) != journalMagic {
		return errors.New("orchestrator: el archivo no es un journal del orquestador (magic incorrecto); no se toca")
	}
	if v := binary.LittleEndian.Uint32(cabecera[8:]); v != journalVersion {
		return fmt.Errorf("orchestrator: versión de journal %d, se esperaba %d", v, journalVersion)
	}
	banderas := binary.LittleEndian.Uint32(cabecera[12:])
	if banderas&flagCifrado != 0 && !j.cifrado {
		return errors.New("orchestrator: el journal está cifrado y este binario no trae FieldKey")
	}
	if banderas&flagCifrado == 0 && j.cifrado {
		return errors.New("orchestrator: hay FieldKey pero el journal se creó en claro; no se reescribe a ciegas")
	}

	st, err := j.f.Stat()
	if err != nil {
		return fmt.Errorf("orchestrator: no se pudo medir el journal: %w", err)
	}
	if st.Size() < journalHeader {
		return errors.New("orchestrator: journal truncado por debajo de la cabecera")
	}
	j.correlativo = binary.LittleEndian.Uint64(cabecera[16:])
	j.size = st.Size()
	return nil
}

// correlativoActual es el último correlativo entregado.
func (j *journal) correlativoActual() uint64 { return j.correlativo }

// hayPendientes dice si el archivo tiene algo más allá de la cabecera. Es la
// respuesta a "¿hay cola?" sin abrir la base de datos.
func (j *journal) hayPendientes() bool { return j.size > journalHeader }

// anadir escribe un registro al final y lo sincroniza a disco.
//
// La cabecera se actualiza ANTES y en el mismo fsync que el registro. Si ocurre
// un corte entre medias se quema un correlativo, que es un hueco visible;
// al revés, el registro se escribiría con un correlativo que la cabecera aún no
// conoce y el siguiente inicio lo reutilizaría.
func (j *journal) anadir(e entradaDisco) error {
	if e.Correlativo <= j.correlativo {
		return fmt.Errorf("orchestrator: el correlativo %d no avanza (actual %d)", e.Correlativo, j.correlativo)
	}
	cuerpo, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("orchestrator: no se pudo serializar la entrada: %w", err)
	}
	if len(cuerpo) > maxRegistro {
		return fmt.Errorf("orchestrator: entrada de %d bytes excede el máximo (%d)", len(cuerpo), maxRegistro)
	}
	registro, err := j.cifrar(cuerpo)
	if err != nil {
		return err
	}

	if err := j.escribirCorrelativo(e.Correlativo); err != nil {
		return err
	}
	if _, err := j.f.WriteAt(registro, j.size); err != nil {
		return fmt.Errorf("orchestrator: no se pudo escribir en el journal: %w", err)
	}
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("orchestrator: no se pudo sincronizar el journal: %w", err)
	}
	j.correlativo = e.Correlativo
	j.size += int64(len(registro))
	return nil
}

// cifrar arma el registro con su prefijo de longitud.
func (j *journal) cifrar(cuerpo []byte) ([]byte, error) {
	var largo [4]byte
	binary.LittleEndian.PutUint32(largo[:], uint32(len(cuerpo)))
	registro := append([]byte{}, largo[:]...)
	if !j.cifrado {
		return append(registro, cuerpo...), nil
	}
	nonce := make([]byte, j.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("orchestrator: no se pudo generar el nonce: %w", err)
	}
	registro = append(registro, nonce...)
	return j.aead.Seal(registro, nonce, cuerpo, nil), nil
}

// escribirCorrelativo actualiza el contador de la cabecera conservando el
// resto del formato.
func (j *journal) escribirCorrelativo(correlativo uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], correlativo)
	if _, err := j.f.WriteAt(buf[:], 16); err != nil {
		return fmt.Errorf("orchestrator: no se pudo actualizar el correlativo: %w", err)
	}
	return nil
}

// leerPendientes devuelve las entradas sin aplicar que quedan en el archivo.
//
// Un registro a medio escribir (un corte de luz a mitad del append) se detecta
// porque el archivo acaba antes de donde el registro dice que acaba. Se
// devuelve lo que hay hasta ahí y se trunca el archivo ahí: no se puede inventar
// lo que no llegó a escribirse, y truncar evita que el siguiente append se pegue
// a un registro imposible de descifrar.
func (j *journal) leerPendientes() ([]entradaDisco, error) {
	var salida []entradaDisco
	pos := int64(journalHeader)
	for pos < j.size {
		var largo uint32
		if err := binary.Read(io.NewSectionReader(j.f, pos, 4), binary.LittleEndian, &largo); err != nil {
			return salida, j.truncarEn(pos)
		}
		if largo == 0 || largo > maxRegistro {
			return salida, j.truncarEn(pos)
		}
		inicio := pos + 4
		necesario := int64(largo) + int64(j.sobrecoste())
		if inicio+necesario > j.size {
			return salida, j.truncarEn(pos)
		}
		bruto := make([]byte, necesario)
		if _, err := j.f.ReadAt(bruto, inicio); err != nil {
			return salida, j.truncarEn(pos)
		}
		cuerpo, ok := j.descifrar(bruto)
		if !ok {
			return salida, j.truncarEn(pos)
		}
		var e entradaDisco
		if err := json.Unmarshal(cuerpo, &e); err != nil {
			return salida, j.truncarEn(pos)
		}
		salida = append(salida, e)
		pos = inicio + necesario
	}
	return salida, nil
}

// sobrecoste son los bytes extra del cifrado: nonce y etiqueta de GCM.
func (j *journal) sobrecoste() int {
	if !j.cifrado {
		return 0
	}
	return j.aead.NonceSize() + j.aead.Overhead()
}

// descifrar deshace el registro. El segundo valor dice si es recuperable.
func (j *journal) descifrar(bruto []byte) ([]byte, bool) {
	if !j.cifrado {
		return bruto, true
	}
	ns := j.aead.NonceSize()
	if len(bruto) < ns {
		return nil, false
	}
	cuerpo, err := j.aead.Open(nil, bruto[:ns], bruto[ns:], nil)
	if err != nil {
		return nil, false
	}
	return cuerpo, true
}

// truncarEn deja el archivo en `pos` y anota el nuevo tamaño.
func (j *journal) truncarEn(pos int64) error {
	if pos >= j.size {
		return nil
	}
	if err := j.f.Truncate(pos); err != nil {
		return fmt.Errorf("orchestrator: no se pudo truncar el journal a la mitad de un registro: %w", err)
	}
	j.size = pos
	return nil
}

// limpiar deja el archivo en su cabecera, CONSERVANDO el correlativo.
//
// "Limpia, no borra": el archivo sigue ahí, se puede preguntar si hay cola sin
// comprobar antes si existe, y el correlativo NO vuelve a 1. Si quedara un
// registro a medio escribir, reutilizar su correlativo perdería el orden justo
// en el momento de más riesgo, que es cuando se limpia.
func (j *journal) limpiar() error {
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("orchestrator: no se pudo sincronizar antes de limpiar: %w", err)
	}
	if err := j.f.Close(); err != nil {
		return fmt.Errorf("orchestrator: no se pudo cerrar el journal: %w", err)
	}
	j.f = nil

	banderas := uint32(0)
	if j.cifrado {
		banderas |= flagCifrado
	}
	tmp := j.ruta + ".limpieza"
	if err := os.WriteFile(tmp, cabeceraNueva(banderas, j.correlativo), 0o600); err != nil {
		return fmt.Errorf("orchestrator: no se pudo escribir la cabecera nueva: %w", err)
	}
	// El rename es atómico: o el journal nuevo está entero o el viejo sigue.
	// Un truncate en su lugar dejaría una ventana sin cabecera.
	if err := os.Rename(tmp, j.ruta); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("orchestrator: no se pudo renombrar el journal limpio: %w", err)
	}
	f, err := os.OpenFile(j.ruta, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("orchestrator: no se pudo reabrir el journal limpio: %w", err)
	}
	j.f = f
	j.size = journalHeader
	return nil
}

// cerrar cierra el archivo.
func (j *journal) cerrar() error {
	if j.f == nil {
		return nil
	}
	err := j.f.Close()
	j.f = nil
	return err
}

// ahoraTexto es el formato de fecha que usa todo el proyecto, incluido el
// audit_log. Se escribe aquí y no calculado en cada sitio para que el journal y
// la auditoría no puedan divergir por un formato distinto.
func ahoraTexto() string { return time.Now().Format("2006-01-02 15:04:05") }
