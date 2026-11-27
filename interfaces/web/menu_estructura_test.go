package web

// Pruebas de la estructura del menú por departamentos.
//
// POR QUÉ SE SIMULAN LAS MIGRACIONES EN VEZ DE LEER SQLITE. El menú vive en la
// tabla `menu` y lo siembran los archivos de migrations/*.sql al arrancar. En
// SQLite la siembra grande del menú no llega a la tabla (el INSERT multi-fila
// con ON DUPLICATE KEY se traduce a algo que no inserta y sin aviso), así que una
// prueba que leyera la base de pruebas daría un árbol vacío y no probaría nada.
// Aquí se aplican las sentencias REALES, en el mismo orden del arranque, sobre un
// mapa en memoria: es el efecto que tendría en MySQL, que es donde corre.
//
// QUÉ SE VIGILA. Un enlace colgado de un grupo apagado desaparece sin dar error;
// un enlace repetido en dos departamentos se ve, pero confunde. La prueba exige
// catorce departamentos en orden, dos niveles, sin repetidos, cada pantalla en su
// departamento y cada permiso del menú sembrado en sub_module (si no, el filtro
// lo pide y nadie puede tenerlo, ni el administrador).

import (
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"erpgo"
)

// filaMenu es el estado de una fila de `menu` tras aplicar las migraciones.
type filaMenu struct {
	id       int
	parentID *int
	label    string
	url      string
	perm     string
	sort     int
	status   int
}

var (
	reInsertMenu = regexp.MustCompile("(?is)^\\s*INSERT\\s+(IGNORE\\s+)?INTO\\s+`?menu`?\\s*\\(([^)]*)\\)\\s*(VALUES|SELECT)\\b(.*)$")
	reDeleteMenu = regexp.MustCompile("(?is)^\\s*DELETE\\s+FROM\\s+`?menu`?\\s+WHERE\\s+`?url`?\\s+IN\\s*\\(([^)]*)\\)")
	reUpdateMenu = regexp.MustCompile("(?is)^\\s*UPDATE\\s+`?menu`?\\s+SET\\s+(.+)$")
	reWhereURL   = regexp.MustCompile("(?is)\\bWHERE\\s+`?url`?\\s*=\\s*'([^']*)'")
	reWhereID    = regexp.MustCompile("(?is)\\bWHERE\\s+`?id`?\\s*=\\s*(\\d+)")
	reWhereIDIn  = regexp.MustCompile("(?is)\\bWHERE\\s+`?id`?\\s+IN\\s*\\(([^)]*)\\)")
	reCorteWhere = regexp.MustCompile("(?is)\\s+WHERE\\s+")
	reInsertSub  = regexp.MustCompile("(?is)^\\s*INSERT\\s+(?:IGNORE\\s+)?INTO\\s+`?sub_module`?\\s*\\(([^)]*)\\)\\s*(VALUES|SELECT)\\b(.*)$")
)

// sentenciasDeMenu devuelve, en el orden del arranque (alfabético), las
// sentencias que escriben en `menu` o `sub_module`.
func sentenciasDeMenu(t *testing.T) []string {
	t.Helper()
	entradas, err := fs.ReadDir(erpgo.Migrations(), "migrations")
	if err != nil {
		t.Fatalf("listar migraciones: %v", err)
	}
	nombres := make([]string, 0, len(entradas))
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			nombres = append(nombres, e.Name())
		}
	}
	sort.Strings(nombres)

	var out []string
	for _, nombre := range nombres {
		b, err := fs.ReadFile(erpgo.Migrations(), "migrations/"+nombre)
		if err != nil {
			t.Fatalf("leer %s: %v", nombre, err)
		}
		limpio := reComentarioBloque.ReplaceAllString(string(b), "")
		limpio = reComentarioLinea.ReplaceAllString(limpio, "")
		for _, s := range partirSentenciasSQL(limpio) {
			u := strings.ToUpper(s)
			if strings.Contains(u, "MENU") || strings.Contains(u, "SUB_MODULE") {
				out = append(out, s)
			}
		}
	}
	return out
}

// menuSimulado aplica las sentencias del menú y devuelve las filas por id.
func menuSimulado(t *testing.T) map[int]*filaMenu {
	t.Helper()
	filas := map[int]*filaMenu{}

	for _, st := range sentenciasDeMenu(t) {
		switch {
		case reDeleteMenu.MatchString(st):
			m := reDeleteMenu.FindStringSubmatch(st)
			for _, url := range comillasDeLista(m[1]) {
				for id, f := range filas {
					if f.url == url {
						delete(filas, id)
					}
				}
			}
		case reInsertMenu.MatchString(st):
			m := reInsertMenu.FindStringSubmatch(st)
			soloSiFalta := strings.TrimSpace(m[1]) != ""
			conDuplicado := strings.Contains(strings.ToUpper(st), "ON DUPLICATE")
			esSelect := strings.EqualFold(strings.TrimSpace(m[3]), "SELECT")
			resto := strings.TrimSpace(m[4])
			// Las siembras idempotentes usan INSERT ... SELECT ... WHERE NOT EXISTS,
			// que trae UNA fila en la lista del SELECT (antes del FROM).
			tuplas := gruposParentizados(resto)
			if esSelect {
				tuplas = []string{antesDeFrom(resto)}
			}
			for _, tupla := range tuplas {
				fila := asociarColumnas(m[2], camposDeFila(tupla))
				id, _ := strconv.Atoi(fila["id"])
				if id == 0 {
					continue
				}
				if _, ya := filas[id]; ya && (soloSiFalta || !conDuplicado) {
					continue
				}
				filas[id] = filaDesdeMapa(fila)
			}
		case reUpdateMenu.MatchString(st):
			m := reUpdateMenu.FindStringSubmatch(st)
			cuerpo := m[1]
			var destino []int
			switch {
			case reWhereURL.MatchString(cuerpo):
				url := reWhereURL.FindStringSubmatch(cuerpo)[1]
				for id, f := range filas {
					if f.url == url {
						destino = append(destino, id)
					}
				}
			case reWhereIDIn.MatchString(cuerpo):
				destino = idsDeLista(reWhereIDIn.FindStringSubmatch(cuerpo)[1])
			case reWhereID.MatchString(cuerpo):
				id, _ := strconv.Atoi(reWhereID.FindStringSubmatch(cuerpo)[1])
				if _, ok := filas[id]; ok {
					destino = append(destino, id)
				}
			}
			asignaciones := cuerpo
			if i := reCorteWhere.FindStringIndex(asignaciones); i != nil {
				asignaciones = asignaciones[:i[0]]
			}
			for _, id := range destino {
				aplicarAsignaciones(filas[id], asignaciones)
			}
		}
	}
	if len(filas) == 0 {
		t.Fatal("la simulación no produjo ninguna fila de menú")
	}
	return filas
}

// subModuleSembrados devuelve los directorios que las migraciones siembran en
// sub_module: la lista contra la que se resuelven los permisos.
func subModuleSembrados(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, st := range sentenciasDeMenu(t) {
		if !reInsertSub.MatchString(st) {
			continue
		}
		m := reInsertSub.FindStringSubmatch(st)
		resto := strings.TrimSpace(m[3])
		// En INSERT ... SELECT la fila está en la lista del SELECT, no entre
		// paréntesis: se toma entera y se corta una sola vez.
		var tuplas []string
		if strings.EqualFold(m[2], "SELECT") {
			tuplas = []string{antesDeFrom(resto)}
		} else {
			tuplas = gruposParentizados(resto)
		}
		for _, tupla := range tuplas {
			fila := asociarColumnas(m[1], camposDeFila(tupla))
			if d := strings.TrimSpace(fila["directory"]); d != "" {
				out[d] = true
			}
		}
	}
	return out
}

func filaDesdeMapa(m map[string]string) *filaMenu {
	id, _ := strconv.Atoi(m["id"])
	f := &filaMenu{
		id:     id,
		label:  m["label"],
		url:    m["url"],
		perm:   m["perm"],
		status: 1,
	}
	if m["status"] != "" {
		f.status, _ = strconv.Atoi(m["status"])
	}
	if m["sort_order"] != "" {
		f.sort, _ = strconv.Atoi(m["sort_order"])
	}
	if m["parent_id"] != "" {
		if p, err := strconv.Atoi(m["parent_id"]); err == nil {
			f.parentID = &p
		}
	}
	return f
}

func aplicarAsignaciones(f *filaMenu, asignaciones string) {
	if f == nil {
		return
	}
	for _, asig := range camposDeFila(asignaciones) {
		partes := strings.SplitN(asig, "=", 2)
		if len(partes) != 2 {
			continue
		}
		columna := limpiarValorSQL(strings.ToLower(strings.TrimSpace(partes[0])))
		valor := limpiarValorSQL(partes[1])
		switch columna {
		case "parent_id":
			if valor == "" {
				f.parentID = nil
			} else if p, err := strconv.Atoi(valor); err == nil {
				f.parentID = &p
			}
		case "label":
			f.label = valor
		case "url":
			f.url = valor
		case "perm":
			f.perm = valor
		case "sort_order":
			if n, err := strconv.Atoi(valor); err == nil {
				f.sort = n
			}
		case "status":
			if n, err := strconv.Atoi(valor); err == nil {
				f.status = n
			}
		}
	}
}

func asociarColumnas(columnas string, valores []string) map[string]string {
	out := map[string]string{}
	for i, c := range camposDeFila(columnas) {
		if i < len(valores) {
			out[strings.ToLower(limpiarValorSQL(c))] = valores[i]
		}
	}
	return out
}

// departamentos es el orden del sidebar. Si se agrega o se quita un
// departamento, esta lista se actualiza a mano a propósito: es una decisión de
// producto, no un detalle que deba cambiar solo.
var departamentos = []struct {
	label string
	hijos int
}{
	{"Inicio", 0},
	{"Ventas", 9},
	{"Clientes y cobros", 6},
	{"Compras y proveedores", 6},
	{"Productos", 4},
	{"Inventario y almacén", 8},
	{"Contabilidad", 12},
	{"Impuestos y libros", 7},
	{"Reportes", 6},
	{"CRM", 6},
	{"Costos", 4},
	{"Recursos Humanos", 5},
	{"Asistentes IA", 5},
	{"Configuración", 14},
}

func TestElMenuQuedaOrdenadoPorDepartamentos(t *testing.T) {
	filas := menuSimulado(t)

	visibles := map[int]*filaMenu{}
	for id, f := range filas {
		if f.status == 1 {
			visibles[id] = f
		}
	}
	hijos := map[int][]*filaMenu{}
	for _, f := range visibles {
		if f.parentID != nil {
			hijos[*f.parentID] = append(hijos[*f.parentID], f)
		}
	}

	// 1) Los departamentos, en orden y con sus enlaces.
	var raices []*filaMenu
	for _, f := range visibles {
		if f.parentID == nil {
			raices = append(raices, f)
		}
	}
	sort.Slice(raices, func(i, j int) bool {
		if raices[i].sort != raices[j].sort {
			return raices[i].sort < raices[j].sort
		}
		return raices[i].id < raices[j].id
	})
	if len(raices) != len(departamentos) {
		t.Fatalf("el sidebar debe tener %d departamentos y tiene %d: %v",
			len(departamentos), len(raices), etiquetasDe(raices))
	}
	for i, esperado := range departamentos {
		raiz := raices[i]
		if raiz.label != esperado.label {
			t.Errorf("departamento %d: %q, se esperaba %q (orden: %v)",
				i+1, raiz.label, esperado.label, etiquetasDe(raices))
		}
		if n := len(hijos[raiz.id]); n != esperado.hijos {
			t.Errorf("%q tiene %d enlaces y se esperaban %d", raiz.label, n, esperado.hijos)
		}
		if esperado.hijos > 0 && raiz.url != "" {
			t.Errorf("%q tiene hijos pero además URL %q: el sidebar lo trataría como enlace y sus hijos no se verían",
				raiz.label, raiz.url)
		}
		if esperado.hijos == 0 && raiz.url == "" {
			t.Errorf("%q no tiene enlaces ni URL: no hay forma de llegar a ninguna pantalla desde él", raiz.label)
		}
	}

	// 2) Dos niveles como máximo y ningún huérfano visible.
	for _, f := range visibles {
		if f.parentID == nil {
			continue
		}
		padre, ok := visibles[*f.parentID]
		if !ok {
			t.Errorf("%q (id %d) cuelga de %d, que no está visible: el enlace no se pinta", f.label, f.id, *f.parentID)
			continue
		}
		if len(hijos[f.id]) > 0 {
			t.Errorf("%q (id %d) cuelga de %q y además tiene hijos: el layout solo pinta dos niveles", f.label, f.id, padre.label)
		}
	}

	// 3) Ningún enlace repetido: dos entradas a la misma pantalla dejan al
	// usuario sin saber cuál es la sección «de verdad».
	vistos := map[string]*filaMenu{}
	for _, f := range visibles {
		if f.url == "" {
			continue
		}
		if otro, ya := vistos[f.url]; ya {
			t.Errorf("la URL %s está dos veces en el menú (%q y %q)", f.url, otro.label, f.label)
		}
		vistos[f.url] = f
	}

	// 4) Cada pantalla en el departamento que le toca.
	ubicacion := map[string]string{
		"/invoice/new":               "Ventas",
		"/quotation/list":            "Ventas",
		"/service/invoice/new":       "Ventas",
		"/cartera/list":              "Clientes y cobros",
		"/pagar/list":                "Compras y proveedores",
		"/supplier/list":             "Compras y proveedores",
		"/product/list":              "Productos",
		"/reports/stock":             "Inventario y almacén",
		"/warehouse/list":            "Inventario y almacén",
		"/delivery_order/list":       "Inventario y almacén",
		"/consumption/list":          "Inventario y almacén",
		"/bank/list":                 "Contabilidad",
		"/account/balance-sheet":     "Contabilidad",
		"/reports/kardex":            "Impuestos y libros",
		"/reports/sales":             "Reportes",
		"/reports/audit-log":         "Reportes",
		"/crm/remarketing/campaigns": "CRM",
		"/cost/period":               "Costos",
		"/hr/payroll":                "Recursos Humanos",
		"/ai/agents":                 "Asistentes IA",
		"/settings/digital-invoice":  "Configuración",
	}
	for url, departamento := range ubicacion {
		f, ok := vistos[url]
		if !ok {
			t.Errorf("%s no está en ningún departamento visible", url)
			continue
		}
		if f.parentID == nil {
			t.Errorf("%s quedó suelta en el primer nivel", url)
			continue
		}
		padre, ok := visibles[*f.parentID]
		if !ok {
			continue
		}
		if padre.label != departamento {
			t.Errorf("%s quedó en %q y su sitio es %q", url, padre.label, departamento)
		}
	}

	// 5) Las puertas viejas que duplicaban una pantalla siguen en la tabla pero
	// apagadas: si alguien las reactiva, esa pantalla sale dos veces.
	for _, id := range []int{7, 190, 191, 212} {
		f, ok := filas[id]
		if !ok {
			t.Errorf("la fila %d desapareció de la tabla: el menú se apaga, no se borra", id)
			continue
		}
		if f.status != 0 {
			t.Errorf("la entrada vieja %q (id %d) sigue visible: duplica una pantalla de su departamento", f.label, id)
		}
	}

	// 6) Todo permiso del menú tiene que existir en sub_module.
	sembrados := subModuleSembrados(t)
	for _, f := range visibles {
		if f.perm == "" || f.perm == "__master" {
			continue
		}
		for _, clave := range strings.Split(f.perm, "|") {
			if clave == "" {
				continue
			}
			if !sembrados[clave] {
				t.Errorf("%q pide el permiso %q, que ninguna migración siembra en sub_module: quedaría escondido para todos",
					f.label, clave)
			}
		}
	}
}

func etiquetasDe(filas []*filaMenu) []string {
	out := make([]string, 0, len(filas))
	for _, f := range filas {
		out = append(out, f.label)
	}
	return out
}

// ---- Lectura de las sentencias SQL (suficiente para menu y sub_module) ----
// Las expresiones para quitar comentarios (reComentarioBloque, reComentarioLinea)
// viven en regla_borrado_test.go: se reutilizan para no declararlas dos veces.

// partirSentenciasSQL parte por punto y coma fuera de literales de texto.
func partirSentenciasSQL(sql string) []string {
	var out []string
	var b strings.Builder
	enTexto := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case c == '\'':
			enTexto = !enTexto
			b.WriteByte(c)
		case c == ';' && !enTexto:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}

// gruposParentizados devuelve el contenido de cada (...) de primer nivel y se
// detiene cuando lo que sigue ya no es otra tupla (p. ej. un ON DUPLICATE).
func gruposParentizados(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) || s[i] != '(' {
			break
		}
		desde := i
		nivel := 0
		enTexto := false
		cerrado := false
		for ; i < len(s); i++ {
			c := s[i]
			if c == '\'' {
				enTexto = !enTexto
				continue
			}
			if enTexto {
				continue
			}
			if c == '(' {
				nivel++
			} else if c == ')' {
				nivel--
				if nivel == 0 {
					i++
					cerrado = true
					break
				}
			}
		}
		if !cerrado {
			break
		}
		out = append(out, s[desde+1:i-1])
	}
	return out
}

// camposDeFila parte una lista por comas de primer nivel, limpiando comillas.
func camposDeFila(s string) []string {
	var out []string
	nivel := 0
	enTexto := false
	desde := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			enTexto = !enTexto
		case enTexto:
		case c == '(' || c == '[':
			nivel++
		case c == ')' || c == ']':
			nivel--
		case c == ',' && nivel == 0:
			out = append(out, s[desde:i])
			desde = i + 1
		}
	}
	out = append(out, s[desde:])
	for i, v := range out {
		out[i] = limpiarValorSQL(v)
	}
	return out
}

// limpiarValorSQL quita comillas, acentos graves y traduce NULL a "".
func limpiarValorSQL(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	if len(v) >= 2 && v[0] == '`' && v[len(v)-1] == '`' {
		return v[1 : len(v)-1]
	}
	if strings.EqualFold(v, "NULL") {
		return ""
	}
	return v
}

// comillasDeLista devuelve los literales entre comillas de una lista SQL.
func comillasDeLista(s string) []string {
	var out []string
	var b strings.Builder
	enTexto := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			if enTexto {
				out = append(out, b.String())
				b.Reset()
			}
			enTexto = !enTexto
		case enTexto:
			b.WriteByte(c)
		}
	}
	return out
}

// idsDeLista lee una lista de ids como (4, 7, '190'): con comillas o sin ellas.
func idsDeLista(s string) []int {
	var out []int
	for _, parte := range strings.Split(s, ",") {
		txt := strings.Trim(strings.TrimSpace(parte), "'`\" ")
		if id, err := strconv.Atoi(txt); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// antesDeFrom corta en el FROM de primer nivel (para INSERT ... SELECT).
func antesDeFrom(s string) string {
	up := strings.ToUpper(s)
	nivel := 0
	enTexto := false
	for i := 0; i+5 <= len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			enTexto = !enTexto
		case enTexto:
		case c == '(':
			nivel++
		case c == ')':
			nivel--
		case nivel == 0 && strings.HasPrefix(up[i:], "FROM "):
			return s[:i]
		}
	}
	return s
}
