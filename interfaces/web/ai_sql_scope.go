package web

// ai_sql_scope.go — Qué tablas puede tocar cada agente en su SQL.
//
// El problema. aiValidateSQLSelect (ai_rules.go) comprueba que la consulta sea un
// SELECT de solo lectura, sin DML ni DDL. Eso es el guardián de ESCRITURA: evita
// que el agente modifique la base. Pero no es el guardián de LECTURA: un SELECT
// puede leer CUALQUIER tabla de las 139 de la base, incluidas las que el agente
// no debería conocer. El scopado del prompt (ai_schema_introspect.go) es solo
// una cortesía: un modelo puede escribir `SELECT * FROM hr_salary` igual, porque
// adivina el nombre, o porque lo leyó en un informe anterior.
//
// La regla. Un agente es un empleado más con un rol. Si es de cuentas por cobrar,
// lee cuentas por cobrar. Si además es administrador, lee todo. Eso se aplica en
// el SQL, no en la educación del prompt.
//
// Lo que NO hace: no parsea SQL con un parser completo. Recorre la sentencia
// token a token y reconstruye la lista de tablas de cada FROM/JOIN/INTO/UPDATE,
// incluidas las que van detrás de una coma, y la contrasta con el conjunto
// permitido. Un SELECT con una subconsulta anidada aporta sus propias tablas, así
// que la comprobación cubre el caso real. Un modelo que quisiera esquivar esto
// tendría que construir el nombre por concatenación, que es exactamente la clase
// de consulta que el resto del guardián (comentarios, literales, palabras
// prohibidas) ya rechaza.

import (
	"fmt"
	"regexp"
	"strings"
)

// aiTokenSQLPat parte la sentencia en los dos tipos de pieza que importan aquí:
// identificadores (con o sin acento grave) y la puntuación que delimita una
// lista de tablas (la coma y los paréntesis).
//
// POR QUÉ NO UN PATRÓN `FROM\s+(\w+)`. Ese fue el error original: solo captura
// la PRIMERA tabla tras FROM/JOIN/INTO/UPDATE, así que `SELECT * FROM invoice,
// user_login` declaraba `invoice` y la segunda tabla pasaba sin comprobar. La
// lista de tablas de un FROM se separa por comas, y una coma no es una palabra
// clave: hay que recorrer la lista, no casar una expresión regular contra ella.
var aiTokenSQLPat = regexp.MustCompile("`?([a-zA-Z_][a-zA-Z0-9_$]*)`?|(,|\\(|\\))")

// aiArranqueDeListaSQL: palabras tras las que empieza una referencia a tabla.
var aiArranqueDeListaSQL = map[string]bool{
	"from": true, "join": true, "into": true, "update": true,
}

// aiFinDeListaSQL: palabras que cierran la lista de tablas. Al aparecer una, los
// identificadores que siguen no son tablas (alias, columnas, condiciones).
//
// `as` NO está en la lista a propósito: `FROM t AS x, y` debe seguir leyendo `y`
// después de la coma.
var aiFinDeListaSQL = map[string]bool{
	"where": true, "group": true, "order": true, "limit": true, "having": true,
	"union": true, "join": true, "left": true, "right": true, "inner": true,
	"outer": true, "cross": true, "natural": true, "on": true, "using": true,
	"select": true, "set": true, "values": true, "for": true, "lock": true,
}

// aiTablasEnSQL devuelve los nombres de tabla que aparecen en la sentencia, en
// el orden en que aparecen y sin repetir.
//
// Recorre la sentencia pieza a pieza en vez de casar una expresión regular. Con
// cada FROM/JOIN/INTO/UPDATE empieza una lista; dentro de la lista, la primera
// referencia a tabla cuenta y, tras cada coma, la siguiente también. Los
// paréntesis abren y cierran subconsultas, y el estado de la lista se apila y se
// recupera para que `FROM (SELECT ... FROM hr_salary) x, otras` siga viendo las
// dos tablas.
func aiTablasEnSQL(sql string) []string {
	// Se limpia de comentarios y literales antes de recorrer: un nombre de tabla
	// dentro de un literal ('... FROM users ...') es dato, no una tabla que se
	// vaya a consultar.
	limpio := aiStripSQLCommentsAndLiterals(sql)

	vistos := map[string]bool{}
	var out []string
	agregar := func(t string) {
		t = strings.ToLower(t)
		if t == "" || vistos[t] {
			return
		}
		vistos[t] = true
		out = append(out, t)
	}

	type estadoLista struct{ enLista, esperandoTabla bool }
	enLista, esperandoTabla := false, false
	var pila []estadoLista

	for _, m := range aiTokenSQLPat.FindAllStringSubmatch(limpio, -1) {
		if m[2] != "" {
			switch m[2] {
			case ",":
				// La coma de una lista de tablas anuncia la siguiente.
				if enLista {
					esperandoTabla = true
				}
			case "(":
				// Subconsulta: abre su propio contexto. El FROM de dentro lo
				// encontrará el recorrido por sí solo.
				pila = append(pila, estadoLista{enLista, esperandoTabla})
				enLista, esperandoTabla = false, false
			case ")":
				if n := len(pila); n > 0 {
					enLista, esperandoTabla = pila[n-1].enLista, false
					pila = pila[:n-1]
				} else {
					enLista, esperandoTabla = false, false
				}
			}
			continue
		}

		baja := strings.ToLower(m[1])
		switch {
		case aiArranqueDeListaSQL[baja]:
			enLista, esperandoTabla = true, true
		case enLista && esperandoTabla && !aiFinDeListaSQL[baja]:
			// Primera tabla del FROM, o la que sigue a una coma.
			agregar(m[1])
			esperandoTabla = false
		case enLista && aiFinDeListaSQL[baja]:
			enLista, esperandoTabla = false, false
		}
	}
	return out
}

// aiTablasPermitidasParaDept es el conjunto de tablas que un departamento puede
// leer. Sale del scopado declarado (aiDeptDeTabla), no de lo que existe: así una
// tabla que se crea en la base pero no está asignada a ningún departamento sigue
// sin ser accesible, que es lo que se quiere de una tabla nueva.
func aiTablasPermitidasParaDept(dept string) map[string]bool {
	d := aiDeptNormalizar(dept)
	permitidas := map[string]bool{}
	if d == DeptIT {
		// IT ve las tablas de negocio declaradas. Las de autenticación y las
		// internas siguen fuera: ni soporte las necesita para diseñar informes,
		// y una IA con acceso a user_login es un agujero de seguridad con otro
		// nombre.
		for t := range aiCatalogWhitelist {
			permitidas[t] = true
		}
		return permitidas
	}
	for tabla, depts := range aiDeptDeTabla {
		for _, x := range depts {
			if x == d {
				permitidas[tabla] = true
				break
			}
		}
	}
	return permitidas
}

// aiValidarScopeSQL comprueba que todas las tablas del SELECT estén permitidas
// para el departamento. Devuelve un error legible, no un error de SQL: el modelo
// necesita entender que es un problema de permisos, no de sintaxis.
func aiValidarScopeSQL(sql, dept string) error {
	d := aiDeptNormalizar(dept)
	permitidas := aiTablasPermitidasParaDept(d)
	var denegadas []string
	for _, t := range aiTablasEnSQL(sql) {
		if permitidas[t] {
			continue
		}
		// Una tabla de autenticación o interna nunca se concede por
		// departamento, ni siquiera a IT.
		denegadas = append(denegadas, t)
	}
	if len(denegadas) == 0 {
		return nil
	}
	// Mensaje en el mismo idioma que el resto del producto: el agente lo lee y
	// lo reporta, no lo traga en silencio.
	if d == DeptIT {
		return fmt.Errorf("no tienes acceso a la tabla %s", strings.Join(denegadas, ", "))
	}
	return fmt.Errorf("tu rol (%s) no tiene acceso a %s; solicítalo a un administrador si lo necesitas",
		aiDeptLabel[d], strings.Join(denegadas, ", "))
}

// aiValidarSQLConScope es el guardián completo: escritura (SELECT puro) más
// lectura (scope por departamento). Es el que usa la herramienta db_query.
func aiValidarSQLConScope(sql, dept string, maxRows int) (string, error) {
	limpio, err := aiValidateSQLSelect(sql, maxRows)
	if err != nil {
		return "", err
	}
	if err := aiValidarScopeSQL(limpio, dept); err != nil {
		return "", err
	}
	return limpio, nil
}
