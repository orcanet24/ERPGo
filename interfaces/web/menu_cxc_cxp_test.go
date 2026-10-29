package web

// Pruebas de que Cuentas por Cobrar y Cuentas por Pagar son SECCIONES del menú.
//
// POR QUÉ SE PRUEBA LEYENDO LAS MIGRACIONES Y NO CON UNA LISTA A MANO. El menú
// lateral no vive en una plantilla: vive en la tabla `menu`, y quien la llena son
// los archivos de migrations/*.sql al arrancar. Un renglón que se cuelga del
// grupo equivocado —o que pide un permiso que nadie tiene sembrado— no da ningún
// error: simplemente la sección aparece escondida dentro de otro grupo, o no
// aparece, y desde el código todo parece correcto. Fue exactamente lo que pasó:
// las dos entradas quedaron como el último renglón DENTRO de Clientes y de
// Proveedores. Por eso esta prueba mira el efecto real de las sentencias, en el
// mismo orden en que las aplica el arranque, y después pasa el resultado por el
// MISMO filtro que arma el sidebar (buildMenuItem).
//
// LO QUE COMPRUEBA CADA CASO:
//
//	1. que alguna migración inserte la fila — si no, nadie llega a la pantalla;
//	2. que quede de primer nivel (parent_id NULL), no colgada de otro módulo;
//	3. que esté activa y con su nombre;
//	4. que su permiso exista en sub_module, porque si no el filtro la esconde
//	    para todos, incluido el administrador de la empresa;
//	5. que nadie cuelgue de ella: un nodo con hijos se pinta como grupo y su URL
//	    deja de ser un enlace;
//	6. y que el filtro real la muestre con el permiso concedido y la esconda sin
//	    él —el menú es una promesa: no puede ofrecer lo que el middleware
//	    responde 403—.
//
// Lo que NO se prueba, porque no se puede sin un MySQL: que las sentencias corran
// contra el motor. Eso lo cubre el registro de migraciones del arranque.

import (
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"erpgo"
)

var (
	reInsertDeMenu = regexp.MustCompile("(?is)^\\s*INSERT\\s+(?:IGNORE\\s+)?INTO\\s+`?menu`?\\s*\\(([^)]*)\\)\\s*(VALUES|SELECT)\\b(.*)$")
	reUpdateDeMenu = regexp.MustCompile("(?is)^\\s*UPDATE\\s+`?menu`?\\s+SET\\s+(.+?)\\s+WHERE\\s+`?url`?\\s*=\\s*'([^']*)'")
	reInsertDeSub  = regexp.MustCompile("(?is)^\\s*INSERT\\s+(?:IGNORE\\s+)?INTO\\s+`?sub_module`?\\s*\\(([^)]*)\\)\\s*(VALUES|SELECT)\\b(.*)$")
)

// sentenciasDeMenuEnLasMigraciones devuelve, en orden alfabético de archivo —el
// mismo que usa el arranque—, las sentencias que tocan `menu` o `sub_module`.
func sentenciasDeMenuEnLasMigraciones(t *testing.T) []string {
	t.Helper()
	entradas, err := fs.ReadDir(erpgo.Migrations(), "migrations")
	if err != nil {
		t.Fatalf("no se pudieron listar las migraciones: %v", err)
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
			t.Fatalf("no se pudo leer %s: %v", nombre, err)
		}
		// Los comentarios se quitan primero: los archivos hablan del menú en sus
		// cabeceras y esas palabras no son sentencias.
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

// partirSentenciasSQL parte por punto y coma fuera de los literales de texto.
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
// detiene cuando lo que sigue ya no es otra tupla: así el `ON DUPLICATE KEY
// UPDATE` que cierra los INSERT de siembra no se confunde con valores.
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

// camposDeFila parte una lista de valores por comas de primer nivel. Los
// paréntesis se respetan porque `decimal(18,2)` es un valor, no dos.
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

// limpiarValorSQL quita las comillas y traduce NULL a cadena vacía, que es como
// se representa «sin valor» en estas comprobaciones. También quita el acento
// grave: los nombres de columna vienen envueltos (“ `url` “) y sin sacarlo la
// fila quedaría con la clave "`url`" y la comprobación no encontraría nada.
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

// antesDeFrom corta en el FROM de primer nivel: en un INSERT ... SELECT los
// valores son los literales que van entre SELECT y FROM.
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

// filasDeInsert devuelve una fila por cada renglón que inserta la sentencia:
// varias tuplas en un VALUES, o una sola en un INSERT ... SELECT.
func filasDeInsert(columnas, modo, resto string) []map[string]string {
	columnasLimpias := camposDeFila(columnas)
	asociar := func(valores []string) map[string]string {
		fila := map[string]string{}
		for i, c := range columnasLimpias {
			if i < len(valores) {
				fila[strings.ToLower(c)] = valores[i]
			}
		}
		return fila
	}
	if strings.EqualFold(modo, "VALUES") {
		var out []map[string]string
		for _, tupla := range gruposParentizados(resto) {
			out = append(out, asociar(camposDeFila(tupla)))
		}
		return out
	}
	return []map[string]string{asociar(camposDeFila(antesDeFrom(resto)))}
}

// TestCxCYCxPSonSeccionesDelMenu recorre las migraciones, arma el menú que
// quedaría en la base y comprueba que las dos secciones se ven.
func TestCxCYCxPSonSeccionesDelMenu(t *testing.T) {
	filas := map[string]map[string]string{} // por URL: última escritura del menú
	directorios := map[string]bool{}        // sub_module.directory sembrados

	for _, st := range sentenciasDeMenuEnLasMigraciones(t) {
		switch {
		case reInsertDeMenu.MatchString(st):
			m := reInsertDeMenu.FindStringSubmatch(st)
			// Un INSERT con su tabla derivada sólo entra si la fila no estaba
			// (es el patrón que se usa para no duplicar entradas); el resto de
			// los INSERT de siembra llevan ON DUPLICATE KEY UPDATE, así que
			// reescriben la fila con los valores que traen.
			soloSiFalta := strings.EqualFold(m[2], "SELECT")
			for _, fila := range filasDeInsert(m[1], m[2], m[3]) {
				url := fila["url"]
				if url == "" {
					continue
				}
				if _, ya := filas[url]; ya && soloSiFalta {
					continue
				}
				filas[url] = fila
			}
		case reUpdateDeMenu.MatchString(st):
			m := reUpdateDeMenu.FindStringSubmatch(st)
			fila, existe := filas[m[2]]
			if !existe {
				continue // un UPDATE no crea la fila
			}
			for _, asig := range camposDeFila(m[1]) {
				partes := strings.SplitN(asig, "=", 2)
				if len(partes) != 2 {
					continue
				}
				// La columna va con acento grave en el archivo (`parent_id`): si no
				// se quita, la asignación escribe la clave "`parent_id`" y la fila
				// conserva el valor viejo, con lo que el UPDATE parecería no correr.
				fila[limpiarValorSQL(strings.ToLower(strings.TrimSpace(partes[0])))] = limpiarValorSQL(partes[1])
			}
			filas[m[2]] = fila
		case reInsertDeSub.MatchString(st):
			m := reInsertDeSub.FindStringSubmatch(st)
			for _, fila := range filasDeInsert(m[1], m[2], m[3]) {
				if dir := fila["directory"]; dir != "" {
					directorios[dir] = true
				}
			}
		}
	}

	casos := []struct{ url, label, perm string }{
		{"/cartera/list", "Cuentas por Cobrar", "customer_ledger"},
		{"/pagar/list", "Cuentas por Pagar", "supplier_payment"},
	}
	for _, c := range casos {
		fila, existe := filas[c.url]
		if !existe {
			t.Errorf("ninguna migración inserta la entrada de menú %s: el módulo existe pero no hay forma de llegar a él desde la barra lateral", c.url)
			continue
		}
		if fila["parent_id"] != "" {
			t.Errorf("%s cuelga del menú con id %q: es una sección propia y tiene que estar en el primer nivel, como Devoluciones o Contabilidad; colgada de un grupo ajeno queda como el último renglón de otro módulo y no se encuentra",
				c.url, fila["parent_id"])
		}
		if fila["status"] != "1" {
			t.Errorf("la entrada %s queda con status=%q: el menú sólo lee las filas activas", c.url, fila["status"])
		}
		if fila["label"] != c.label {
			t.Errorf("la entrada %s se llama %q y se esperaba %q", c.url, fila["label"], c.label)
		}
		if fila["perm"] != c.perm {
			t.Errorf("la entrada %s pide el permiso %q y se esperaba %q: es el que el módulo usa en Roles y Permisos",
				c.url, fila["perm"], c.perm)
		}
		if !directorios[c.perm] {
			t.Errorf("el permiso %q no lo siembra ninguna migración en sub_module: el filtro del menú lo pediría y nadie podría tenerlo, ni el administrador de la empresa", c.perm)
		}
		for url, otra := range filas {
			if otra["parent_id"] != "" && otra["parent_id"] == fila["id"] {
				t.Errorf("%s quedó como grupo porque %s (id %s) cuelga de ella: un nodo con hijos se pinta como grupo y su URL deja de ser un enlace",
					fila["label"], url, otra["id"])
			}
		}

		id, _ := strconv.Atoi(fila["id"])
		nodo := &menuNode{id: id, label: fila["label"], url: c.url, perm: fila["perm"]}
		item, visible := buildMenuItem(nodo, map[string]bool{c.perm: true}, c.url)
		if !visible {
			t.Errorf("con el permiso %q concedido el filtro sigue escondiendo %q: la sección no se pintaría en el menú", c.perm, c.label)
		} else {
			if item.URL != c.url {
				t.Errorf("%q se pinta con la URL %q y se esperaba %q", c.label, item.URL, c.url)
			}
			if len(item.Children) != 0 {
				t.Errorf("%q se pinta como grupo con %d hijos", c.label, len(item.Children))
			}
			if !item.Active {
				t.Errorf("%q no se marca como activa estando en %s", c.label, c.url)
			}
		}
		if _, visible := buildMenuItem(nodo, map[string]bool{}, c.url); visible {
			t.Errorf("sin el permiso %q el menú muestra %q: prometería una pantalla que el middleware responde 403", c.perm, c.label)
		}
	}
}
