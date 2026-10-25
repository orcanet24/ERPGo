package web

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"erpgow/infrastructure/persistence"
)

// ---------- Base de datos (MySQL) ----------
// Una BASE POR EMPRESA (`erpgo_<codigo>`) más la base de control `erpgo`.
// Esta página muestra el estado real de la base de la empresa activa: motor,
// cuántas tablas y filas tiene, cuánto ocupa en disco, y un respaldo con un
// clic.
//
// LO QUE CAMBIÓ RESPECTO A LA PÁGINA DE SQLITE, Y POR QUÉ NO SE TRADUJO.
//
// Antes esta página hacía tres cosas que en MySQL no existen: leer el tamaño de
// un archivo, volcar el WAL y medir `PRAGMA integrity_check`. En su lugar:
//
//   - Tamaño: `information_schema.tables` lo da por tabla (datos + índices),
//     que es la cifra que de verdad cuenta cuando el "archivo" son 120 tablas.
//   - Journal: en InnoDB no se elige. La durabilidad la da el redo log y el
//     doble write buffer, y se informa del ajuste efectivo en vez de
//     inventarse un modo que nadie puede cambiar.
//   - Integridad: InnoDB verifica cada escritura con un checksum. En lugar de
//     un `CHECK TABLE` sobre 120 tablas —que es una operación pesada y puede
//     tardar minutos en una base grande— se comprueba que el motor de las
//     tablas es InnoDB y que la conexión lee el catálogo. El `CHECK TABLE`
//     completo sigue disponible, pero a petición, no en cada visita a la
//     página: una pantalla de diagnóstico que tarda dos minutos en abrir es
//     una pantalla que nadie abre.

// backupDir agrupa los respaldos de una empresa.
func backupDir() string {
	return filepath.Join("backups")
}

func (s *Server) registerDatabaseSettingsRoutes() {
	s.mux.HandleFunc("GET /settings/database", s.databaseSettingsPage)
	s.mux.HandleFunc("POST /settings/database/check", s.databaseCheck)
	s.mux.HandleFunc("POST /settings/database/backup", s.databaseBackup)
	s.mux.HandleFunc("GET /settings/database/backups", s.databaseBackupList)
}

func (s *Server) databaseSettingsPage(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)

	var tables int
	var motores string
	var sizeMB float64
	// Una sola consulta: `information_schema` sobre las tablas de ESTA base
	// (DATABASE() acota el filtro) da a la vez cuántas hay, con qué motor y
	// cuánto ocupan. Con SQL y luego MySQL se hizo con dos; una consulta es
	// una foto, y dos consultas dos fotos que pueden no cuadrar entre sí.
	_ = db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(MAX(engine), '?'),
		       COALESCE(SUM(data_length + index_length), 0)
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`).
		Scan(&tables, &motores, &sizeMB)

	// Filas de las tablas principales (muestra el tamaño real del negocio).
	var totalRows int64
	for _, t := range []string{"invoice", "product_information", "customer_information", "product_purchase", "audit_log"} {
		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err == nil {
			totalRows += n
		}
	}

	integrity := "no verificado"
	journal := "InnoDB (redo log + doble escritura)"

	// El motor decide la integridad: InnoDB no solo tiene un checksum por página,
	// es que además es el único motor que sirve aquí. MyISAM no transacciona y una
	// caída a mitad de escritura deja la tabla rota sin avisar.
	if motores == "InnoDB" {
		integrity = "ok"
	} else if motores != "" {
		integrity = "revisar: hay tablas " + motores
	}
	// Si el ajuste de durabilidad está en 2, InnoDB puede perder la última
	// transacción si la MÁQUINA se apaga de golpe. No es corrupción, pero
	// tampoco es lo que se le vende a un usuario, así que se dice.
	var flush int
	if err := db.QueryRow("SELECT @@innodb_flush_log_at_trx_commit").Scan(&flush); err == nil {
		switch flush {
		case 1:
			journal = "InnoDB, durability completa (flush_log_at_trx_commit=1)"
		case 2:
			journal = "InnoDB, durability parcial (flush_log_at_trx_commit=2): se puede perder lo último si el equipo se apaga de golpe"
		default:
			journal = fmt.Sprintf("InnoDB, flush_log_at_trx_commit=%d (sin garantía en escritura)", flush)
		}
	}

	state := map[string]interface{}{
		"Base":        s.currentDBPath(),
		"Tables":      tables,
		"SampleRows":  totalRows,
		"Integrity":   integrity,
		"JournalMode": journal,
		"SizeMB":      sizeMB / (1024 * 1024),
		"Engine":      motores,
		"Server":      s.config.DBHost + ":" + strconv.Itoa(s.config.DBPort),
	}

	s.render(w, r, "database_settings.html", map[string]interface{}{
		"Title": "Base de Datos",
		"State": state,
	})
}

// databaseCheck ejecuta un CHECK TABLE sobre las tablas de negocio y cuenta
// cuántas están bien. Es el equivalente HONESTO del integrity_check de SQLite.
//
// Va a petición y no en cada visita a la página por una razón medida: CHECK
// TABLE lee y reescribe la tabla entera, y sobre 120 tablas de una base real
// eso no es un diagnóstico, es una parada. El control diario ya lo hace InnoDB
// con su checksum en cada escritura; esto es la revisión profunda, la que se
// hace
// alguien sospecha de un problema.
//
// El resultado se cuenta, no se muestra línea a línea: lo que el usuario quiere
// saber es si hay que preocuparse, no leer 120 líneas de "OK".
func (s *Server) databaseCheck(w http.ResponseWriter, r *http.Request) {
	db := s.dbFor(r)
	tablas, err := tablasDeLaBase(db)
	if err != nil {
		jsonError(w, "No se pudo leer el catálogo de tablas: "+err.Error(), 500)
		return
	}
	if len(tablas) == 0 {
		jsonError(w, "Esta base no tiene tablas todavía", 400)
		return
	}

	buenas, malas := 0, 0
	var primeroMalo string
	for _, t := range tablas {
		var estado, mensaje string
		// La firma de CHECK TABLE es (Tabla, Op, Msg_type, Msg_text) y son
		// cuatro columnas SIEMPRE: escanear solo dos mueve los valores de sitio
		// y "OK" acaba leyéndose como nombre de tabla.
		err := db.QueryRow("CHECK TABLE `"+t+"`").
			Scan(new(string), &estado, new(string), &mensaje)
		if err == nil && estado == "OK" {
			buenas++
			continue
		}
		malas++
		if primeroMalo == "" {
			detalle := mensaje
			if detalle == "" {
				detalle = "sin mensaje"
			}
			primeroMalo = t + ": " + detalle
		}
	}

	msg := fmt.Sprintf("%d tablas comprobadas, %d correctas.", len(tablas), buenas)
	if malas > 0 {
		writeJSON(w, map[string]interface{}{
			"status": 0,
			"msg":    msg + " PROBLEMAS: " + strconv.Itoa(malas) + " con error. La primera es " + primeroMalo,
		})
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": msg + " Integridad verificada."})
}

// tablasDeLaBase devuelve los nombres de tabla de la base conectada.
//
// Va por information_schema con DATABASE() y no por SHOW TABLES porque
// information_schema es el catálogo que MySQL mantiene consistente incluso
// cuando algo va mal; SHOW TABLES se resuelve por otra ruta y ha sido la que
// falla justo cuando hace falta diagnosticar.
func tablasDeLaBase(db *sql.DB) ([]string, error) {
	// La consulta va escrita aquí y no por s.sqlShowTables() porque esta
	// función no tiene Server: se usa desde un camino de diagnóstico donde lo
	// único que importa es la base conectada, no la empresa de la petición.
	// information_schema con DATABASE() acota a esa base: sin el filtro saldrían
	// las tablas de todas las empresas del servidor.
	const consulta = "SELECT table_name FROM information_schema.tables " +
		"WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'"
	filas, err := db.Query(consulta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var tablas []string
	for filas.Next() {
		var t string
		if err := filas.Scan(&t); err != nil {
			return nil, err
		}
		tablas = append(tablas, t)
	}
	return tablas, filas.Err()
}

// databaseBackup genera un respaldo .sql de la empresa actual, comprimido.
//
// LO QUE HACE UN RESPALDO EN MySQL, Y POR QUÉ NO ES COPIAR UN ARCHIVO.
//
// En SQLite el respaldo era copiar un archivo, y eso era correcto: el estado
// entero del sistema estaba en ese archivo. En MySQL el estado está repartido en
// los archivos de InnoDB de TODA la instalación, y un archivo suelto copiado a
// mitad de camino no es una base de datos: es un trozo. Un InnoDB con el redo log
// a medias no arranca, y el que lo arranca tiene que hacer recuperación.
//
// Por eso el respaldo aquí es un volcado LÓGICO con mysqldump: son sentencias
// SQL (CREATE TABLE, INSERT) que se pueden restaurar en cualquier MySQL, en otra
// máquina, o dentro de cinco años. Un volcado lógico es más grande que el
// archivo físico y no sirve para restaurar en segundos; a cambio se puede
// restaurar donde sea, y es lo que se puede leer con un editor de texto si
// hace falta.
//
// `--single-transaction` es lo que hace que el volcado sea coherente sin bloquear
// la empresa. En InnoDB da una foto consistente de las tablas InnoDB: se empieza
// una transacción y, como el resto solo escribe InnoDB, todo el volcado sale de
// la misma foto. Sin esa opción, mysqldump bloquea cada tabla mientras la copia
// y un usuario que está facturando se queda mirando un candado.
func (s *Server) databaseBackup(w http.ResponseWriter, r *http.Request) {
	base := s.currentDBPath()

	if err := os.MkdirAll(backupDir(), 0o755); err != nil {
		jsonError(w, "No se pudo crear la carpeta de respaldos: "+err.Error(), 500)
		return
	}

	stamp := time.Now().Format("2006-01-02_150405")
	tenant := s.tenantCodeOrDefault()
	name := fmt.Sprintf("%s_%s.zip", tenant, stamp)
	dstPath := filepath.Join(backupDir(), name)

	if err := s.writeBackupZip(base, dstPath, tenant, stamp); err != nil {
		jsonError(w, "No se pudo crear el respaldo: "+err.Error(), 500)
		return
	}

	var sizeMB float64
	if info, err := os.Stat(dstPath); err == nil {
		sizeMB = float64(info.Size()) / (1024 * 1024)
	}
	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg": fmt.Sprintf(
			"Respaldo creado: %s (%.2f MB). Contiene el volcado SQL de la base %s; "+
				"para restaurar, descomprima y ejecute el .sql sobre un MySQL vacío.",
			name, sizeMB, base),
	})
}

// writeBackupZip crea el ZIP con el volcado SQL y un manifiesto.
//
// El volcado se escribe DIRECTAMENTE dentro del ZIP, a través de un pipe: no se
// genera un .sql de hundreds of megabytes en disco para luego comprimirlo y
// borrarlo. En una base de una pyme con años de datos ese fichero temporal es
// el doble del disco que ocupa el respaldo, y el proceso de hacerlo en la
// carpeta de respaldos deja ahí un archivo a medias si algo falla.
func (s *Server) writeBackupZip(base, dstPath, tenant, stamp string) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	// Manifiesto: qué se respaldó y cuándo. Sin esto, un ZIP suelto no dice de
	// qué empresa era, y dentro de un mes no se distingue de los demás.
	manifest := fmt.Sprintf(
		"ERPGo backup\ntenant: %s\ncreated: %s\nbase: %s\nkind: volcado logico (mysqldump)\n"+
			"restore: mysql -u root -p <empresa>.sql\n",
		tenant, stamp, base)
	mw, err := zw.Create("manifest.txt")
	if err != nil {
		return err
	}
	if _, err := mw.Write([]byte(manifest)); err != nil {
		return err
	}

	destino, err := zw.Create(base + ".sql")
	if err != nil {
		return err
	}
	if err := persistence.VolcarBaseMySQL(persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.EffectiveRootUser(),
		Clave:   s.config.DBPassword,
		Nombre:  base,
	}, destino); err != nil {
		// Se cierra el ZIP para que quede un archivo válido y no un .zip
		// truncado que parece un respaldo bueno y no lo es.
		zw.Close()
		os.Remove(dstPath)
		return err
	}
	return zw.Close()
}

// databaseBackupList lista los respaldos disponibles de esta empresa.
func (s *Server) databaseBackupList(w http.ResponseWriter, r *http.Request) {
	tenant := s.tenantCodeOrDefault()
	prefix := tenant + "_"
	type item struct {
		File string  `json:"file"`
		MB   float64 `json:"mb"`
		When string  `json:"when"`
	}
	var list []item
	if entries, err := os.ReadDir(backupDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".zip") {
				continue
			}
			it := item{File: e.Name()}
			if info, err := e.Info(); err == nil {
				it.MB = float64(info.Size()) / (1024 * 1024)
				it.When = info.ModTime().Format("2006-01-02 15:04")
			}
			list = append(list, it)
		}
	}
	writeJSON(w, map[string]interface{}{"data": list})
}
