package web

// Pruebas de la regla de no borrado (compras, clientes y proveedores).
//
// Se prueba la DECISIÓN —que es lo que puede fallar en silencio— y, sobre todo,
// que las consultas de la regla nombren tablas y columnas que existen de verdad
// en las migraciones. Esa segunda parte es la que importa: un nombre mal escrito
// deja la guarda contando 0 para siempre y la puerta abierta sin que nada se
// ponga rojo. Se lee del paquete embebido, la misma fuente que aplica el arranque,
// en vez de una lista de tablas a mano que envejece al revés que el esquema.
//
// Lo que NO se prueba aquí, y no se puede sin un MySQL: el conteo contra una base
// con datos (ver infrastructure/persistence/testdb.go, donde los tests se saltan
// cuando no hay servidor).

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"erpgow"
	"erpgow/infrastructure/persistence"
)

func TestRevisarBorradoSinMovimientoNoBloquea(t *testing.T) {
	msg, bloqueado := revisarBorrado("el cliente", consejoCliente, []referencia{
		{Que: "facturas"},
		{Que: "cobros registrados", Cuantos: 0},
		{Que: "cotizaciones"},
	})
	if bloqueado {
		t.Fatalf("un cliente sin documentos no debe bloquear; salió %q", msg)
	}
	if msg != "" {
		t.Errorf("sin bloqueo el mensaje tiene que ir vacío; salió %q", msg)
	}
}

func TestRevisarBorradoNombraElMovimientoYElConsejo(t *testing.T) {
	msg, bloqueado := revisarBorrado("la compra", consejoCompra, []referencia{
		{Que: "asientos contables", Cuantos: 2},
		{Que: "renglones de mercancía", Cuantos: 3},
		{Que: "retenciones de IVA"},
	})
	if !bloqueado {
		t.Fatal("una compra con asientos publicados no puede borrarse")
	}
	// El mensaje lo lee alguien que está decidiendo: tiene que decir qué
	// bloquea, cuánto, y qué hacer en su lugar.
	for _, esperado := range []string{
		"la compra", "2 asientos contables", "3 renglones de mercancía", " y ", consejoCompra,
	} {
		if !strings.Contains(msg, esperado) {
			t.Errorf("el mensaje %q no dice %q", msg, esperado)
		}
	}
	// Y no debe nombrar lo que está en cero: un "0 retenciones" solo confunde.
	if strings.Contains(msg, "retenciones") {
		t.Errorf("el mensaje nombra una referencia que está en cero: %q", msg)
	}
}

func TestEnumerarUsaComasYConjuncionEspanolas(t *testing.T) {
	casos := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"1 factura"}, "1 factura"},
		{[]string{"1 factura", "2 cobros"}, "1 factura y 2 cobros"},
		{[]string{"3 facturas", "1 cobro", "2 cotizaciones"}, "3 facturas, 1 cobro y 2 cotizaciones"},
		{[]string{"a", "b", "c", "d"}, "a, b, c y d"},
	}
	for _, c := range casos {
		if got := enumerar(c.in); got != c.want {
			t.Errorf("enumerar(%v) = %q, esperado %q", c.in, got, c.want)
		}
	}
}

func TestMensajeNoComprobadoDiceElMotivoYLaEntidad(t *testing.T) {
	msg := mensajeNoComprobado("el proveedor", fs.ErrNotExist)
	for _, esperado := range []string{"el proveedor", "precaución", fs.ErrNotExist.Error()} {
		if !strings.Contains(msg, esperado) {
			t.Errorf("el mensaje %q no dice %q", msg, esperado)
		}
	}
}

func TestLasConsultasDeLaReglaApuntanATablasYColumnasReales(t *testing.T) {
	esquema := esquemaDeLasMigraciones(t)
	if len(esquema) < 100 {
		t.Fatalf("el esquema leído tiene %d tablas: no se están leyendo las migraciones", len(esquema))
	}
	t.Logf("esquema leído: %d tablas", len(esquema))

	regla := map[string][]consultaReferencia{
		"compras":     referenciasCompra,
		"clientes":    referenciasCliente,
		"proveedores": referenciasProveedor,
	}
	for entidad, consultas := range regla {
		if len(consultas) == 0 {
			t.Errorf("%s: la regla no tiene ninguna referencia que comprobar", entidad)
		}
		for _, c := range consultas {
			if strings.TrimSpace(c.Que) == "" {
				t.Errorf("%s: una consulta no dice qué cuenta (%q)", entidad, c.SQL)
			}
			if n := strings.Count(c.SQL, "?"); n != 1 {
				t.Errorf("%s (%s): la consulta lleva %d parámetros y el conteo pasa uno solo: %q",
					entidad, c.Que, n, c.SQL)
			}
			tablas, columnas := tablasYColumnasCitadas(c.SQL)
			if len(tablas) == 0 {
				t.Errorf("%s (%s): la consulta no cita ninguna tabla del esquema: %q", entidad, c.Que, c.SQL)
				continue
			}
			declaradas := map[string]bool{}
			for _, tabla := range tablas {
				cols, ok := esquema[tabla]
				if !ok {
					t.Errorf("%s (%s): la tabla %q no existe en las migraciones: %q", entidad, c.Que, tabla, c.SQL)
					continue
				}
				for col := range cols {
					declaradas[col] = true
				}
			}
			for _, col := range columnas {
				if !declaradas[col] {
					t.Errorf("%s (%s): la columna %q no pertenece a ninguna de las tablas citadas (%v); "+
						"la guarda contaría siempre 0: %q", entidad, c.Que, col, tablas, c.SQL)
				}
			}
		}
	}
}

// TestLasReglasDeBorradoDeLaBaseApuntanAEsquemaReal hace con las reglas que
// viven en la BASE (infrastructure/persistence/proteccion_borrado.go) lo mismo
// que la prueba de arriba con las del handler.
//
// Importa más que la otra, si cabe: el motor valida las columnas al crear el
// trigger, así que una errata en una condición deja ESA regla sin crear, el
// arranque solo deja un aviso en el log y nadie se entera de que la base dejó de
// proteger ese borrado.
func TestLasReglasDeBorradoDeLaBaseApuntanAEsquemaReal(t *testing.T) {
	esquema := esquemaDeLasMigraciones(t)
	reglas := persistence.ReglasDeBorrado()
	if len(reglas) == 0 {
		t.Fatal("no hay ninguna regla de protección en la base")
	}

	reTabla := regexp.MustCompile("(?i)\\bFROM\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?")
	rePalabra := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

	for _, regla := range reglas {
		if strings.TrimSpace(regla.Tabla) == "" || strings.TrimSpace(regla.Condicion) == "" || strings.TrimSpace(regla.Mensaje) == "" {
			t.Errorf("la regla %q está incompleta", regla.Nombre)
			continue
		}
		declaradas := map[string]bool{}
		cols, ok := esquema[regla.Tabla]
		if !ok {
			t.Errorf("la regla %q protege la tabla %q, que no existe en las migraciones", regla.Nombre, regla.Tabla)
			continue
		}
		for col := range cols {
			declaradas[col] = true
		}

		// Se buscan las tablas en TODA la condición, no solo antes del primer
		// WHERE: la condición de un cliente encadena varios EXISTS.
		esTabla := map[string]bool{}
		var tablas []string
		for _, m := range reTabla.FindAllStringSubmatch(regla.Condicion, -1) {
			tablas = append(tablas, m[1])
			esTabla[strings.ToLower(m[1])] = true
		}
		if len(tablas) == 0 {
			// La regla puede mirar solo su propia fila (una cuenta por pagar ya
			// pagada no necesita mirar otra tabla).
			if !strings.Contains(strings.ToUpper(regla.Condicion), "OLD.") {
				t.Errorf("la regla %q no mira ninguna tabla ni su propia fila: %q", regla.Nombre, regla.Condicion)
			}
		}
		for _, tabla := range tablas {
			otras, ok := esquema[tabla]
			if !ok {
				t.Errorf("la regla %q cita la tabla %q, que no existe en las migraciones", regla.Nombre, tabla)
				continue
			}
			for col := range otras {
				declaradas[col] = true
			}
		}
		for _, palabra := range rePalabra.FindAllString(regla.Condicion, -1) {
			if len(palabra) < 2 || palabrasSQL[strings.ToUpper(palabra)] || esTabla[strings.ToLower(palabra)] {
				continue
			}
			if !declaradas[palabra] {
				t.Errorf("la regla %q usa la columna %q, que no está en %q ni en las tablas que cita (%v): "+
					"el trigger no se podría crear y esa protección no existiría",
					regla.Nombre, palabra, regla.Tabla, tablas)
			}
		}
	}
}

// ---------- lectura del esquema ----------

var (
	reComentarioLinea  = regexp.MustCompile(`(?m)--[^\n]*`)
	reComentarioBloque = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reCrearTabla       = regexp.MustCompile("(?is)\\bCREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([A-Za-z_][A-Za-z0-9_]*)`?\\s*\\(")
	reAlterarTabla     = regexp.MustCompile("(?is)\\bALTER\\s+TABLE\\s+`?([A-Za-z_][A-Za-z0-9_]*)`?([^;]*);")
	reAgregarCol       = regexp.MustCompile("(?is)\\bADD\\s+(?:COLUMN\\s+)?`?([A-Za-z_][A-Za-z0-9_]*)`?")
	reIdentificador    = regexp.MustCompile("^`?([A-Za-z_][A-Za-z0-9_]*)`?")
	// Los literales de texto no son ni tablas ni columnas: sin quitarlos, en
	// `status = 'open'` la palabra `open` se leería como una columna y la
	// comprobación fallaría por algo que está bien escrito.
	reLiteralSQL = regexp.MustCompile(`'(?:[^']|'')*'`)
)

// restriccion es lo que empieza un renglón de CREATE TABLE que no declara una
// columna.
var restriccion = map[string]bool{
	"PRIMARY": true, "KEY": true, "UNIQUE": true, "INDEX": true,
	"CONSTRAINT": true, "FOREIGN": true, "CHECK": true, "FULLTEXT": true, "SPATIAL": true,
}

// esquemaDeLasMigraciones devuelve, por tabla, las columnas declaradas: las del
// CREATE TABLE más las que añaden los ALTER TABLE.
func esquemaDeLasMigraciones(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entradas, err := fs.ReadDir(erpgow.Migrations(), "migrations")
	if err != nil {
		t.Fatalf("no se pudieron listar las migraciones: %v", err)
	}
	var todo strings.Builder
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(erpgow.Migrations(), "migrations/"+e.Name())
		if err != nil {
			t.Fatalf("no se pudo leer %s: %v", e.Name(), err)
		}
		// Los comentarios se quitan antes de parsear, y los de bloque primero: los
		// volcados traen líneas comentadas —y bloques enteros de definiciones
		// viejas— con nombres de tabla y de columna que ya no existen.
		limpio := reComentarioBloque.ReplaceAllString(string(b), "")
		todo.WriteString(reComentarioLinea.ReplaceAllString(limpio, ""))
		todo.WriteString("\n")
	}
	sql := todo.String()

	esquema := map[string]map[string]bool{}
	var sinCerrar []string
	for _, m := range reCrearTabla.FindAllStringSubmatchIndex(sql, -1) {
		tabla := sql[m[2]:m[3]]
		// m[1] es el índice SIGUIENTE al paréntesis que cierra el match, así que
		// el paréntesis abierto está una posición antes. Iba sin el -1 y el
		// helper rechazaba todas las tablas por «no cerrar su paréntesis».
		cuerpo, ok := cuerpoParentizado(sql, m[1]-1)
		if !ok {
			// Una definición partida (un paréntesis dentro de un literal, un
			// volcado a medias) no puede tumbar la comprobación entera: se salta
			// esa tabla y se sigue. El mínimo de tablas leídas, más abajo, es lo
			// que detecta que el parseo se haya roto de verdad.
			sinCerrar = append(sinCerrar, tabla)
			continue
		}
		cols := columnasDelCuerpo(cuerpo)
		if cols == nil {
			cols = map[string]bool{}
		}
		if previas, ya := esquema[tabla]; ya {
			for c := range previas {
				cols[c] = true
			}
		}
		esquema[tabla] = cols
	}
	for _, m := range reAlterarTabla.FindAllStringSubmatch(sql, -1) {
		tabla, cuerpo := m[1], m[2]
		cols, ya := esquema[tabla]
		if !ya {
			// Un ALTER sobre una tabla que este test no vio crear es una
			// migración que se aplica sobre algo de otra versión: se registra
			// igual, para no inventar un fallo que no es de la regla.
			cols = map[string]bool{}
			esquema[tabla] = cols
		}
		for _, a := range reAgregarCol.FindAllStringSubmatch(cuerpo, -1) {
			nombre := a[1]
			if restriccion[strings.ToUpper(nombre)] {
				continue
			}
			cols[nombre] = true
		}
	}
	return esquema
}

// cuerpoParentizado devuelve el texto entre el paréntesis abierto en `desde` y su
// cierre, respetando los paréntesis anidados de decimal(18,2) o de un ENUM(...).
func cuerpoParentizado(sql string, desde int) (string, bool) {
	if desde >= len(sql) || sql[desde] != '(' {
		return "", false
	}
	nivel := 0
	for i := desde; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			nivel++
		case ')':
			nivel--
			if nivel == 0 {
				return sql[desde+1 : i], true
			}
		}
	}
	return "", false
}

// columnasDelCuerpo saca el primer identificador de cada definición separada por
// comas de nivel 0, que es exactamente donde va el nombre de la columna.
func columnasDelCuerpo(cuerpo string) map[string]bool {
	cols := map[string]bool{}
	nivel := 0
	desde := 0
	partes := []string{}
	for i := 0; i < len(cuerpo); i++ {
		switch cuerpo[i] {
		case '(', '[':
			nivel++
		case ')', ']':
			nivel--
		case ',':
			if nivel == 0 {
				partes = append(partes, cuerpo[desde:i])
				desde = i + 1
			}
		}
	}
	partes = append(partes, cuerpo[desde:])
	for _, parte := range partes {
		parte = strings.TrimSpace(parte)
		if parte == "" {
			continue
		}
		if m := reIdentificador.FindStringSubmatch(parte); m != nil {
			if restriccion[strings.ToUpper(m[1])] {
				continue
			}
			cols[m[1]] = true
		}
	}
	return cols
}

// palabrasSQL son las palabras de la consulta que no son ni tabla ni columna.
var palabrasSQL = map[string]bool{
	"SELECT": true, "COUNT": true, "FROM": true, "JOIN": true, "ON": true,
	"WHERE": true, "AND": true, "OR": true, "NOT": true, "AS": true,
	"LEFT": true, "RIGHT": true, "INNER": true, "IS": true, "NULL": true, "DISTINCT": true,
	// EXISTS y OLD salen en las condiciones de los triggers de la base.
	"EXISTS": true, "OLD": true,
	// La ordenación y el límite son gramática, no columnas: una consulta que
	// termina en `ORDER BY opened_at DESC LIMIT 1` es correcta, y sin estas
	// palabras la comprobación la denunciaría como si citara columnas
	// inexistentes.
	"ORDER": true, "BY": true, "ASC": true, "DESC": true, "LIMIT": true,
}

// tablasYColumnasCitadas parte una consulta en las tablas que nombra (las que
// están antes de un WHERE, un JOIN o el final) y los identificadores que quedan,
// que son columnas o alias.
//
// Los alias de una letra —`t` y `s` en el JOIN de contabilidad— no se cuentan
// como columna: son un nombre local de la consulta, no del esquema.
func tablasYColumnasCitadas(consulta string) (tablas, columnas []string) {
	consulta = reLiteralSQL.ReplaceAllString(consulta, "''")
	// Lo que va en el FROM/JOIN es la tabla; lo demás son columnas o alias.
	resto := consulta
	if i := strings.Index(strings.ToUpper(resto), " WHERE "); i >= 0 {
		resto = resto[:i]
	}
	esTabla := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+`+"`?"+`([A-Za-z_][A-Za-z0-9_]*)`).FindAllStringSubmatch(resto, -1) {
		tablas = append(tablas, m[1])
		esTabla[strings.ToLower(m[1])] = true
	}
	for _, palabra := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`).FindAllString(consulta, -1) {
		if len(palabra) < 2 || palabrasSQL[strings.ToUpper(palabra)] || esTabla[strings.ToLower(palabra)] {
			continue
		}
		columnas = append(columnas, palabra)
	}
	return tablas, columnas
}
