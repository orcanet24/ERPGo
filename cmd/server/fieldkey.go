package main

// fieldkey.go — La llave de la FieldKey y el sello de integridad.
//
// Esto es la capa que se prepara ahora y se USA AL FINAL. En
// desarrollo la FieldKey no va puesta: el binario normal corre sin
// llave y no cifra nada, para poder trabajar con la base abierta y
// poder mirar los datos con cualquier cliente.
//
// Las tres piezas:
//
//   - fieldKeyCifrada es el ANCLA. Tiene que existir aunque no se use,
//     porque es donde el enlazador escribe el valor que pasa por
//     -ldflags "-X main.fieldKeyCifrada=...". Si esta variable no
//     existiera, el -X se ignoraria en silencio y el binario saldria
//     sin llave, sin ningun aviso: por eso vive aqui y no se borra.
//
//   - fieldKey() deshace el XOR y devuelve la llave en claro.
//
//   - verificarIntegridad() comprueba el sello SHA-256.
//
// QUE PROTEGE Y QUE NO: la llave va DENTRO del binario. Esto sube el
// coste de extraerla, no lo impide. Ver doc/SEGURIDAD.md.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fieldKeyCifrada es el ancla de la inyeccion por -ldflags.
//
// El valor que llega por -X es la llave YA XORada con la mascara, en
// hexadecimal. Vacio significa "no hay llave": es el caso normal de
// desarrollo y el de un binario compilado sin el script protegido.
var fieldKeyCifrada = ""

// mascaraXOR es la mascara del XOR. Viaja como constante en el binario.
const mascaraXOR byte = 0x5a

// fieldKeyDesarrollo es la llave que se usa cuando no hay ninguna
// inyectada. Deliberadamente NO cifra nada: mientras se desarrolla se
// necesita poder abrir el .sqlite3 con cualquier cliente para mirar
// datos.
const fieldKeyDesarrollo = ""

// tieneFieldKey dice si este binario lleva llave real.
func tieneFieldKey() bool {
	return strings.TrimSpace(fieldKeyCifrada) != ""
}

// fieldKey devuelve la llave en claro.
//
// El XOR es simetrico: el mismo paso deshace lo que hizo al cifrar.
// OJO: XOR con una constante NO autentica. Quien modifique un byte del
// resultado obtiene otra llave en vez de un error. Para un valor que se
// paga y se revoca, lo correcto seria AES-GCM; queda anotado como S-1
// en doc/SEGURIDAD.md, pendiente de decision.
func fieldKey() string {
	crudo := strings.TrimSpace(fieldKeyCifrada)
	if crudo == "" {
		return fieldKeyDesarrollo
	}
	bytes, err := hex.DecodeString(crudo)
	if err != nil {
		// Una llave ilegible no debe dejar el programa sin arrancar:
		// se avisa y se sigue con la de desarrollo. Parar aqui
		// significaria que un simple typo en el .bat deja la
		// instalacion sin servicio.
		fmt.Fprintf(os.Stderr,
			"[AVISO] FieldKey con formato invalido (%v); se usa la de desarrollo.\n", err)
		return fieldKeyDesarrollo
	}
	plano := make([]byte, len(bytes))
	for i, b := range bytes {
		plano[i] = b ^ mascaraXOR
	}
	return string(plano)
}

// verificarIntegridad comprueba el sello SHA-256 que deja la compilacion
// protegida.
//
// QUE NO PROTEGE, y conviene no olvidarlo: esta comprobacion vive
// DENTRO del binario que comprueba. Quien pueda modificar el ejecutable
// tambien puede reescribir esta funcion, y no gana nada parcheando el
// sello. Detecta corrupcion y sustitucion accidental; contra un atacante
// con acceso de escritura no es una frontera.
//
// Lo que si seria una frontera de verdad es la firma de codigo de
// Windows (Authenticode), que valida Windows antes de ejecutar. Eso ya
// es otra decision, anotada como S-3.
func verificarIntegridad(rutaSello string) error {
	esperado, ruta, err := leerSello(rutaSello)
	if err != nil {
		return err
	}

	datos, err := os.ReadFile(ruta)
	if err != nil {
		return fmt.Errorf("no se encuentra el binario %s: %w", ruta, err)
	}
	suma := sha256.Sum256(datos)
	obtenido := hex.EncodeToString(suma[:])
	if !strings.EqualFold(obtenido, esperado) {
		return fmt.Errorf("el binario ha cambiado: el sello dice %s y el fichero suma %s",
			esperado, obtenido)
	}
	return nil
}

// buscarSello localiza el archivo de sello que deja la compilación protegida,
// y devuelve "" si no hay ninguno.
//
// Devuelve "" en vez de un error porque SU AUSENCIA no es un fallo: en
// desarrollo se compila sin el script protegido y no hay sello, y eso es lo
// normal. Si esto devolviera error, el ERP no arrancaría nunca en desarrollo.
//
// Se buscan los dos nombres que usan los scripts, y se comprueba que el sello
// hable del binario que realmente se está ejecutando: un sello de una versión
// antigua al lado de un binario nuevo es peor que no tener sello, porque daría
// un OK falso.
func buscarSello() string {
	ejecutable, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(ejecutable)
	propio := strings.TrimSuffix(ejecutable, filepath.Ext(ejecutable))

	for _, ruta := range []string{propio + ".sha256", filepath.Join(dir, "erpgo-lanzamiento.sha256")} {
		if _, err := os.Stat(ruta); err != nil {
			continue
		}
		_, binario, err := leerSello(ruta)
		if err != nil {
			continue
		}
		// El sello tiene que ser DE ESTE binario. Comparar solo los nombres
		// sería frágil con las rutas; se comparan los ficheros.
		if binario != "" {
			if mismaFichero(binario, ejecutable) {
				return ruta
			}
			continue
		}
		return ruta
	}
	return ""
}

// mismaFichero compara dos rutas sin distinguir mayúsculas, que es como
// funciona Windows, y sin exigir que estén escritas igual.
func mismaFichero(a, b string) bool {
	na, err1 := filepath.Abs(a)
	nb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		na, nb = a, b
	}
	return strings.EqualFold(filepath.Clean(na), filepath.Clean(nb))
}

// Llave pública Ed25519 embebida. Se inyecta en compilación protegida
// (-X main.pubKeyEd25519=...) y se usa para verificar la firma del sello.
// Vacío = sin firma (desarrollo).
var pubKeyEd25519 = ""

// verificarFirmaEd25519 comprueba que el sello (.sha256) del binario esté
// firmado con la llave privada cuya pública va embebida en este binario.
//
// Ubicación del .sig: mismo directorio, mismo nombre base, extensión .sig.
func verificarFirmaEd25519() error {
	if pubKeyEd25519 == "" {
		return nil // desarrollo: no hay firma
	}

	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	base := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))

	// Leer la firma.
	sigPath := filepath.Join(dir, base+".sig")
	sigB64, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("no se encuentra la firma %s: %w", sigPath, err)
	}

	// Leer el sello (hash esperado).
	selloPath := filepath.Join(dir, base+".sha256")
	hashHex, _, err := leerSello(selloPath)
	if err != nil {
		return fmt.Errorf("no se encuentra el sello %s: %w", selloPath, err)
	}

	// Decodificar la firma y la pública.
	firma, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("firma con formato inválido: %w", err)
	}
	pubKey, err := base64.StdEncoding.DecodeString(pubKeyEd25519)
	if err != nil {
		return fmt.Errorf("pública con formato inválido: %w", err)
	}

	// Verificar.
	if !ed25519.Verify(pubKey, []byte(hashHex), firma) {
		return fmt.Errorf("firma Ed25519 inválida: el sello no fue firmado por la llave esperada")
	}
	return nil
}

// leerSello saca el hash esperado y el nombre del binario del archivo de
// sello, que tiene esta forma:
//
//	# ERPGo - Sello de integridad
//	# ...comentarios...
//	<hash sha256>  <nombre del binario>
//
// Se salta los comentarios y se queda con la primera linea que no lo es.
func leerSello(rutaSello string) (hash, binario string, err error) {
	contenido, err := os.ReadFile(rutaSello)
	if err != nil {
		return "", "", fmt.Errorf("no se encuentra el sello %s: %w", rutaSello, err)
	}
	for _, linea := range strings.Split(string(contenido), "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		campos := strings.Fields(linea)
		if len(campos) == 0 {
			continue
		}
		hash = campos[0]
		if len(campos) > 1 {
			binario = campos[1]
		}
		break
	}
	if hash == "" {
		return "", "", fmt.Errorf("el sello %s no contiene un hash", rutaSello)
	}
	if binario == "" {
		// Si el sello no dice contra que fichero es, se compara contra
		// el propio ejecutable, que es lo unico que se puede.
		if exe, errExec := os.Executable(); errExec == nil {
			binario = exe
		} else {
			binario = rutaSello
		}
	}
	return hash, binario, nil
}
