package persistence

// RECONCILIADOR DEL ESQUEMA CANÓNICO (Fase 2 del plan de esquema canónico).
//
// QUÉ HACE.
//
// Compara el esquema deseado (el DDL canónico de schema/) contra el esquema
// real de la base y aplica SOLO el diff: crea tablas que faltan, agrega
// columnas que faltan y crea índices que faltan. Nunca destruye: ni DROP ni
// MODIFY que recorte datos. Una desviación que no sabe resolver se anota en
// Avisos y se sigue, igual que el runner de migraciones.
//
// POR QUÉ Y NO SEGUIR REPRODUCIENDO LA CADENA.
//
// La cadena de migraciones ordena parches por nombre de archivo; el
// reconciliador declara el estado final y converge a él. Instalar de cero y
// arrancar una base vieja llegan al mismo esquema, siempre.
//
// POR QUÉ UN PARSER PROPIO Y NO UNA LIBRERÍA.
//
// El canónico lo escribe este mismo paquete (tools/esquemacanonico, con
// SHOW CREATE TABLE): su formato es conocido y regular. Un parser enfocado a
// ese formato da menos superficie de fallo que una dependencia externa que
// además habría que mantener.
//
// LO QUE NO HACE, A PROPÓSITO.
//
// - No toca FKs de tablas que ya existen: si una tabla nació sin una FK, no
//   la agrega. Las FKs viven en el CREATE TABLE; una tabla creada por el
//   reconciliador las trae todas.
// - No modifica tipos de columnas existentes (un MODIFY puede recortar datos).

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"erpgo"
)

// ColumnaCanonica es una columna del DDL canónico: nombre y definición (tipo,
// nulabilidad, default, auto_increment, ...).
type ColumnaCanonica struct {
	Nombre     string
	Definicion string
}

// IndiceCanonico es un índice del DDL canónico, que en el volcado de MySQL va
// inline en el CREATE TABLE.
type IndiceCanonico struct {
	Nombre   string
	Columnas string // "`a`, `b`"
	Unico    bool
}

// TablaCanonica es una tabla del DDL canónico.
type TablaCanonica struct {
	Nombre   string
	Crear    string // CREATE TABLE completo, sin el ";" final
	Columnas []ColumnaCanonica
	Indices  []IndiceCanonico
}

// ResultadoReconciliacion cuenta lo que el reconciliador tocó.
type ResultadoReconciliacion struct {
	TablasCreadas     []string
	ColumnasAgregadas []string
	IndicesCreados    []string
	Avisos            []string
}

// DividirSentencias parte un SQL en sentencias, respetando los bloques
// BEGIN...END de los triggers. Compartido con tools/esquemacanonico y con el
// propio reconciliador: una sola implementación, una sola cosa que mantener.
func DividirSentencias(contenido string) []string {
	var sentencias []string
	var actual []string
	profundidad := 0
	for _, linea := range strings.Split(contenido, "\n") {
		t := strings.TrimSpace(linea)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		actual = append(actual, linea)
		campos := strings.Fields(strings.ToUpper(t))
		for i, campo := range campos {
			campo = strings.Trim(campo, ";,()")
			if campo == "BEGIN" {
				profundidad++
			}
			if campo != "END" {
				continue
			}
			// "END IF" / "END LOOP" cierran un IF o LOOP, no el BEGIN.
			if i+1 < len(campos) {
				siguiente := strings.Trim(campos[i+1], ";,()")
				if siguiente == "IF" || siguiente == "LOOP" || siguiente == "WHILE" {
					continue
				}
			}
			profundidad--
		}
		if profundidad <= 0 && strings.HasSuffix(t, ";") {
			sentencias = append(sentencias, strings.Join(actual, "\n"))
			actual = nil
			profundidad = 0
		}
	}
	if len(actual) > 0 {
		sentencias = append(sentencias, strings.Join(actual, "\n"))
	}
	return sentencias
}

// ParsearCanonica convierte el DDL canónico en el modelo del reconciliador.
//
// Entiende el formato que produce SHOW CREATE TABLE: una columna por línea
// que empieza por un identificador entre comillas, y restricciones de tabla
// (PRIMARY KEY, KEY, UNIQUE KEY, CONSTRAINT ... FOREIGN KEY) que no son
// columnas.
func ParsearCanonica(contenido string) ([]TablaCanonica, error) {
	var tablas []TablaCanonica
	for _, sentencia := range DividirSentencias(contenido) {
		lima := strings.TrimSpace(sentencia)
		if !strings.HasPrefix(strings.ToUpper(lima), "CREATE TABLE") {
			continue
		}
		tabla, err := parsearCreateTable(lima)
		if err != nil {
			return nil, err
		}
		tablas = append(tablas, tabla)
	}
	if len(tablas) == 0 {
		return nil, fmt.Errorf("el DDL canónico no trae ninguna CREATE TABLE")
	}
	return tablas, nil
}

func parsearCreateTable(sentencia string) (TablaCanonica, error) {
	abierto := strings.Index(sentencia, "(")
	if abierto < 0 {
		return TablaCanonica{}, fmt.Errorf("CREATE TABLE sin paréntesis: %.80s", sentencia)
	}
	nombre := strings.Trim(strings.TrimSpace(sentencia[len("CREATE TABLE"):abierto]), "`\" ")
	if nombre == "" {
		return TablaCanonica{}, fmt.Errorf("CREATE TABLE sin nombre: %.80s", sentencia)
	}

	// El cuerpo va del primer "(" al paréntesis que lo cierra: el final de un
	// CREATE TABLE de MySQL es ") ENGINE=...", y contar profundidad evita
	// confundir los paréntesis de los tipos VARCHAR(64) con el cierre.
	cuerpo, err := extraerCuerpo(sentencia, abierto)
	if err != nil {
		return TablaCanonica{}, fmt.Errorf("%s: %w", nombre, err)
	}

	tabla := TablaCanonica{Nombre: nombre, Crear: strings.TrimRight(sentencia, "; \n\t")}
	for _, parte := range dividirComas(cuerpo) {
		lima := strings.TrimSpace(parte)
		if lima == "" {
			continue
		}
		upper := strings.ToUpper(lima)
		switch {
		case strings.HasPrefix(upper, "PRIMARY KEY"):
			cols := entreParentesis(lima)
			if cols != "" {
				tabla.Indices = append(tabla.Indices, IndiceCanonico{Nombre: "PRIMARY", Columnas: cols})
			}
		case strings.HasPrefix(upper, "UNIQUE KEY"), strings.HasPrefix(upper, "UNIQUE INDEX"):
			nombreIdx := primerTokenConComillas(lima)
			cols := entreParentesis(lima)
			if nombreIdx != "" && cols != "" {
				tabla.Indices = append(tabla.Indices, IndiceCanonico{Nombre: nombreIdx, Columnas: cols, Unico: true})
			}
		case strings.HasPrefix(upper, "KEY "), strings.HasPrefix(upper, "INDEX "),
			strings.HasPrefix(upper, "FULLTEXT KEY"), strings.HasPrefix(upper, "SPATIAL KEY"):
			nombreIdx := primerTokenConComillas(lima)
			cols := entreParentesis(lima)
			if nombreIdx != "" && cols != "" {
				tabla.Indices = append(tabla.Indices, IndiceCanonico{Nombre: nombreIdx, Columnas: cols})
			}
		case strings.HasPrefix(upper, "CONSTRAINT"), strings.HasPrefix(upper, "FOREIGN KEY"):
			// FKs y restricciones: viajan con el CREATE TABLE. Si la tabla ya
			// existe, el reconciliador no las toca (ver documento del archivo).
			continue
		default:
			col, def, ok := parsearColumna(lima)
			if ok {
				tabla.Columnas = append(tabla.Columnas, ColumnaCanonica{Nombre: col, Definicion: def})
			}
		}
	}
	if len(tabla.Columnas) == 0 {
		return TablaCanonica{}, fmt.Errorf("%s: no se encontraron columnas", nombre)
	}
	return tabla, nil
}

// extraerCuerpo devuelve lo que hay entre el paréntesis en posición abierto y
// su cierre, contando profundidad.
func extraerCuerpo(sentencia string, abierto int) (string, error) {
	profundidad := 0
	for i := abierto; i < len(sentencia); i++ {
		switch sentencia[i] {
		case '(':
			profundidad++
		case ')':
			profundidad--
			if profundidad == 0 {
				return sentencia[abierto+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("paréntesis sin cerrar")
}

// dividirComas parte por las comas de nivel 0 (las de VARCHAR(64) no cuentan).
func dividirComas(cuerpo string) []string {
	var partes []string
	profundidad := 0
	desde := 0
	for i, r := range cuerpo {
		switch r {
		case '(':
			profundidad++
		case ')':
			profundidad--
		case ',':
			if profundidad == 0 {
				partes = append(partes, cuerpo[desde:i])
				desde = i + 1
			}
		}
	}
	partes = append(partes, cuerpo[desde:])
	return partes
}

func parsearColumna(parte string) (nombre, definicion string, ok bool) {
	if !strings.HasPrefix(parte, "`") {
		return "", "", false
	}
	cierre := strings.Index(parte[1:], "`")
	if cierre < 0 {
		return "", "", false
	}
	nombre = parte[1 : 1+cierre]
	definicion = strings.TrimSpace(parte[2+cierre:])
	if definicion == "" {
		return "", "", false
	}
	return nombre, definicion, true
}

func primerTokenConComillas(s string) string {
	inicio := strings.Index(s, "`")
	if inicio < 0 {
		return ""
	}
	fin := strings.Index(s[inicio+1:], "`")
	if fin < 0 {
		return ""
	}
	return s[inicio+1 : inicio+1+fin]
}

func entreParentesis(s string) string {
	abierto := strings.Index(s, "(")
	if abierto < 0 {
		return ""
	}
	profundidad := 0
	for i := abierto; i < len(s); i++ {
		switch s[i] {
		case '(':
			profundidad++
		case ')':
			profundidad--
			if profundidad == 0 {
				return s[abierto+1 : i]
			}
		}
	}
	return ""
}

// Reconciliar aplica el diff entre el DDL canónico y la base real.
//
// dialecto es "mysql" o "sqlite": cambian las consultas de introspección, no
// la forma de aplicar (CREATE TABLE / ADD COLUMN / CREATE INDEX son iguales).
func Reconciliar(db *sql.DB, tablas []TablaCanonica, dialecto string) (ResultadoReconciliacion, error) {
	var res ResultadoReconciliacion
	if db == nil {
		return res, fmt.Errorf("reconciliador: sin base de datos")
	}

	reales, columnasReales, indicesReales, err := introspeccionar(db, dialecto)
	if err != nil {
		return res, err
	}

	// FOREIGN_KEY_CHECKS a 0 durante la reconcilación: una tabla nueva con FKs
	// puede referenciar una tabla que el diff todavía no creó. Se restaura al
	// final; es una variable de sesión, no toca a otras conexiones.
	if dialecto == "mysql" {
		if _, err := db.Exec("SET FOREIGN_KEY_CHECKS=0"); err != nil {
			res.Avisos = append(res.Avisos, fmt.Sprintf("no se pudo poner FOREIGN_KEY_CHECKS=0: %v", err))
		}
		defer db.Exec("SET FOREIGN_KEY_CHECKS=1")
	}

	// Primero las tablas que faltan: sus columnas e índices vienen con ellas.
	for _, t := range tablas {
		if reales[t.Nombre] {
			continue
		}
		if _, err := db.Exec(t.Crear); err != nil {
			res.Avisos = append(res.Avisos, fmt.Sprintf("no se pudo crear %s: %v", t.Nombre, err))
			continue
		}
		res.TablasCreadas = append(res.TablasCreadas, t.Nombre)
		reales[t.Nombre] = true
	}

	// Columnas que faltan en tablas existentes.
	for _, t := range tablas {
		columnas, ok := columnasReales[t.Nombre]
		if !ok {
			continue
		}
		for _, c := range t.Columnas {
			if columnas[c.Nombre] {
				continue
			}
			sentencia := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", t.Nombre, c.Nombre, c.Definicion)
			if _, err := db.Exec(sentencia); err != nil {
				res.Avisos = append(res.Avisos, fmt.Sprintf("no se pudo agregar %s.%s: %v", t.Nombre, c.Nombre, err))
				continue
			}
			res.ColumnasAgregadas = append(res.ColumnasAgregadas, t.Nombre+"."+c.Nombre)
		}
	}

	// Índices que faltan. PRIMARY no se crea con CREATE INDEX: si falta, es un
	// aviso (un ALTER TABLE ADD PRIMARY KEY sobre datos existentes es delicado).
	for _, t := range tablas {
		indices, ok := indicesReales[t.Nombre]
		if !ok {
			continue
		}
		for _, ind := range t.Indices {
			if _, existe := indices[ind.Nombre]; existe {
				continue
			}
			if ind.Nombre == "PRIMARY" {
				res.Avisos = append(res.Avisos, fmt.Sprintf("%s: falta la PRIMARY KEY; no se agrega a mano sobre datos existentes", t.Nombre))
				continue
			}
			unico := ""
			if ind.Unico {
				unico = "UNIQUE "
			}
			sentencia := fmt.Sprintf("CREATE %sINDEX `%s` ON `%s` (%s)", unico, ind.Nombre, t.Nombre, ind.Columnas)
			if _, err := db.Exec(sentencia); err != nil {
				res.Avisos = append(res.Avisos, fmt.Sprintf("no se pudo crear el índice %s.%s: %v", t.Nombre, ind.Nombre, err))
				continue
			}
			res.IndicesCreados = append(res.IndicesCreados, t.Nombre+"."+ind.Nombre)
		}
	}
	return res, nil
}

// introspeccionar devuelve tablas, columnas por tabla e índices por tabla.
func introspeccionar(db *sql.DB, dialecto string) (map[string]bool, map[string]map[string]bool, map[string]map[string]string, error) {
	switch dialecto {
	case "mysql":
		return introspeccionarMySQL(db)
	case "sqlite":
		return introspeccionarSQLite(db)
	}
	return nil, nil, nil, fmt.Errorf("dialecto no soportado: %s", dialecto)
}

func introspeccionarMySQL(db *sql.DB) (map[string]bool, map[string]map[string]bool, map[string]map[string]string, error) {
	reales := map[string]bool{}
	filas, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'")
	if err != nil {
		return nil, nil, nil, err
	}
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			filas.Close()
			return nil, nil, nil, err
		}
		reales[nombre] = true
	}
	filas.Close()

	columnas := map[string]map[string]bool{}
	filasC, err := db.Query("SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE()")
	if err != nil {
		return nil, nil, nil, err
	}
	for filasC.Next() {
		var tabla, columna string
		if err := filasC.Scan(&tabla, &columna); err != nil {
			filasC.Close()
			return nil, nil, nil, err
		}
		if columnas[tabla] == nil {
			columnas[tabla] = map[string]bool{}
		}
		columnas[tabla][columna] = true
	}
	filasC.Close()

	indices := map[string]map[string]string{}
	// Toda tabla tiene entrada, aunque no tenga índices: sin esto, una tabla
	// sin índices reales quedaría fuera del mapa y sus índices deseados se
	// pasarían por alto (el diff nunca los crearía).
	for tabla := range reales {
		indices[tabla] = map[string]string{}
	}
	filasI, err := db.Query(`SELECT TABLE_NAME, INDEX_NAME, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX)
		FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE()
		GROUP BY TABLE_NAME, INDEX_NAME`)
	if err != nil {
		return nil, nil, nil, err
	}
	for filasI.Next() {
		var tabla, nombre, cols string
		if err := filasI.Scan(&tabla, &nombre, &cols); err != nil {
			filasI.Close()
			return nil, nil, nil, err
		}
		if indices[tabla] == nil {
			indices[tabla] = map[string]string{}
		}
		indices[tabla][nombre] = cols
	}
	filasI.Close()
	return reales, columnas, indices, nil
}

func introspeccionarSQLite(db *sql.DB) (map[string]bool, map[string]map[string]bool, map[string]map[string]string, error) {
	reales := map[string]bool{}
	filas, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, nil, nil, err
	}
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			filas.Close()
			return nil, nil, nil, err
		}
		reales[nombre] = true
	}
	filas.Close()

	columnas := map[string]map[string]bool{}
	for tabla := range reales {
		filasC, err := db.Query("PRAGMA table_info(" + tabla + ")")
		if err != nil {
			return nil, nil, nil, err
		}
		columnas[tabla] = map[string]bool{}
		for filasC.Next() {
			var cid int
			var nombre, tipo string
			var notNull int
			var defecto interface{}
			var pk int
			if err := filasC.Scan(&cid, &nombre, &tipo, &notNull, &defecto, &pk); err != nil {
				filasC.Close()
				return nil, nil, nil, err
			}
			columnas[tabla][nombre] = true
		}
		filasC.Close()
	}

	indices := map[string]map[string]string{}
	// Como en MySQL: toda tabla tiene entrada, aunque no tenga índices.
	for tabla := range reales {
		indices[tabla] = map[string]string{}
	}
	filasI, err := db.Query("SELECT tbl_name, name FROM sqlite_master WHERE type='index' AND sql IS NOT NULL AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, nil, nil, err
	}
	for filasI.Next() {
		var tabla, nombre string
		if err := filasI.Scan(&tabla, &nombre); err != nil {
			filasI.Close()
			return nil, nil, nil, err
		}
		if indices[tabla] == nil {
			indices[tabla] = map[string]string{}
		}
		indices[tabla][nombre] = ""
	}
	filasI.Close()
	return reales, columnas, indices, nil
}

// ReconciliarConCanonico carga el DDL canónico embebido y reconcilia la base.
// Es el punto de entrada del arranque.
func ReconciliarConCanonico(db *sql.DB, dialecto string) (ResultadoReconciliacion, error) {
	contenido, err := leerCanonica()
	if err != nil {
		return ResultadoReconciliacion{}, err
	}
	tablas, err := ParsearCanonica(contenido)
	if err != nil {
		return ResultadoReconciliacion{}, err
	}
	return Reconciliar(db, tablas, dialecto)
}

// leerCanonica lee schema/esquema_canonico_mysql.sql del embed (o del disco en
// desarrollo, igual que las migraciones).
func leerCanonica() (string, error) {
	contenido, err := fs.ReadFile(erpgo.Schema(), "schema/esquema_canonico_mysql.sql")
	if err != nil {
		return "", fmt.Errorf("no se pudo leer el DDL canónico: %w", err)
	}
	return string(contenido), nil
}

// Ordenadas devuelve una copia ordenada de una lista, para reportes estables.
func Ordenadas(lista []string) []string {
	salida := append([]string{}, lista...)
	sort.Strings(salida)
	return salida
}
