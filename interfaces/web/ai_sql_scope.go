package web

// ai_sql_scope.go — LA FRONTERA ÚNICA del SQL que la IA escribe.
//
// El problema. aiValidateSQLSelect (ai_rules.go) comprueba que la consulta sea un
// SELECT de solo lectura, sin DML ni DDL. Eso es el guardián de ESCRITURA: evita
// que el agente modifique la base. Pero no es el guardián de LECTURA: un SELECT
// puede leer CUALQUIER tabla de las 139 de la base, incluidas las credenciales
// (ai_providers.api_key), las identidades (user_login) y las claves del POS
// (api_keys). El scopado del prompt (ai_schema_introspect.go) es solo una
// cortesía: un modelo puede escribir `SELECT * FROM hr_salary` igual, porque
// adivina el nombre, o porque lo leyó en un informe anterior.
//
// La regla. Un agente es un empleado más con un rol. Si es de cuentas por cobrar,
// lee cuentas por cobrar. Si además es administrador, lee el catálogo de negocio —
// pero NUNCA la lista negra, que no es un permiso negociable.
//
// UNA SOLA FRONTERA (eso es lo que este archivo aporta). Antes cada camino
// recordaba —o no— su propia validación:
//
//   - db.query pasaba por aiValidarSQLConScope (scope sí, lista negra no);
//   - los informes pasaban por aiValidateSQLSelect a secas (cualquier tabla valía),
//     y el pretendido «validateReportSQL» con la whitelist estaba muerto: nadie
//     lo llamaba (F2 de la auditoría);
//   - report.run_preview y la ejecución de plantillas no aplicaban scope (F4).
//
// Ese patrón «cada handler recuerda» es el agujero, no la defensa. Desde aquí todo
// el SQL de informes y de db.query se valida con validarSQLInformes, y lo que se
// ejecuta es EXACTAMENTE la cadena que devolvió esa función.
//
// Lo que NO hace: no parsea SQL con un parser completo. Recorre la sentencia
// token a token y reconstruye la lista de tablas de cada FROM/JOIN/INTO/UPDATE,
// incluidas las que van detrás de una coma, de un STRAIGHT_JOIN o entre
// paréntesis, y la contrasta con la lista negra y con el departamento. Un modelo
// que quisiera esquivar esto tendría que construir el nombre por concatenación,
// que es exactamente la clase de consulta que el resto del guardián (comentarios,
// literales, palabras prohibidas) ya rechaza.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// aiRechazoTablas es el error que devuelve la capa de tablas de la frontera.
//
// Vive tipado y no como un `strings.Contains` del mensaje porque quien muestra el
// error necesita decir cosas DISTINTAS según la causa: «este informe tocaba una
// tabla prohibida y por eso dejó de ser ejecutable» (norma nueva, plantilla
// antigua) no es lo mismo que «el SQL falla» (dato del momento). Fundirlas en un
// solo «Error» es lo que hace que el usuario persiga el síntoma equivocado.
type aiRechazoTablas struct{ motivo string }

func (e aiRechazoTablas) Error() string { return e.motivo }

// esRechazoDeTablas dice si el error que sube por la cadena es un no de la capa de
// tablas (lista negra o alcance), venga envuelto en %w o no.
func esRechazoDeTablas(err error) bool {
	var t aiRechazoTablas
	return errors.As(err, &t)
}

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
//
// STRAIGHT_JOIN está aquí porque es un operador de JOIN propio de MySQL que el
// escáner antiguo no conocía (M-3 de la auditoría): en
// `FROM invoice STRAIGHT_JOIN user_login` el token `straight_join` no abría una
// lista ni la cerraba, así que la tabla que venía detrás quedaba INVISIBLE para
// el control de tablas. La frontera, además, rechaza la palabra (ver
// validarSQLInformes): el orden de join nunca es una razón para leer otra tabla.
var aiArranqueDeListaSQL = map[string]bool{
	"from": true, "join": true, "into": true, "update": true,
	"straight_join": true,
}

// aiFinDeListaSQL: palabras que cierran la lista de tablas. Al aparecer una, los
// identificadores que siguen no son tablas (alias, columnas, condiciones).
//
// `as` NO está en la lista a propósito: `FROM t AS x, y` debe seguir leyendo `y`
// después de la coma.
//
// Los MODIFICADORES de join (left/right/inner/outer/cross/natural) tampoco están:
// cerrar la lista con ellos era inocuo porque `join` la reabría, pero hacía que el
// escáner dependiera de que el operador estuviera escrito en dos palabras. Sin
// ellos en el mapa, cualquier `X JOIN y` —con el modificador que sea— abre la
// lista por `join` y lee la tabla. Es el mismo candado con menos supuestos.
var aiFinDeListaSQL = map[string]bool{
	"where": true, "group": true, "order": true, "limit": true, "having": true,
	"union": true, "on": true, "using": true,
	"select": true, "set": true, "values": true, "for": true, "lock": true,
}

// aiTablasEnSQL devuelve los nombres de tabla que aparecen en la sentencia, en
// el orden en que aparecen y sin repetir.
//
// Recorre la sentencia pieza a pieza en vez de casar una expresión regular. Con
// cada FROM/JOIN/STRAIGHT_JOIN/INTO/UPDATE empieza una lista; dentro de la lista,
// la primera referencia a tabla cuenta y, tras cada coma, la siguiente también.
// Los paréntesis abren y cierran subconsultas, y el estado de la lista se apila y
// se recupera para que `FROM (SELECT ... FROM hr_salary) x, otras` siga viendo
// las dos tablas.
//
// El paréntesis NO cancela la lista: `FROM (invoice, user_login)` es una tabla
// entre paréntesis (sintaxis válida en MySQL) y antes el escáner perdía las dos
// tablas porque el '(' apagaba el estado de lista.
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
				// Dentro del paréntesis hay DOS casos y los dos se recorren:
				// la subconsulta (`FROM (SELECT ... FROM t) x`) aporta sus
				// tablas por su propio FROM, y la lista entre paréntesis
				// (`FROM (a, b)`) sigue siendo la lista del FROM que la abre.
				pila = append(pila, estadoLista{enLista, esperandoTabla})
				if enLista {
					esperandoTabla = true
				}
			case ")":
				if n := len(pila); n > 0 {
					// Al cerrar no se espera tabla: lo que sigue es el alias.
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

// aiNombresLocalesSQL devuelve los nombres de CTE (`WITH x AS (...)`) de la
// sentencia. No son tablas: son el resultado de un SELECT que el propio SQL ya
// declara, y sus tablas reales las aporta el FROM de dentro. Sin excluirlos, la
// frontera rechazaría `WITH ventas AS (SELECT ... FROM invoice) SELECT * FROM
// ventas` por «ventas no está en el catálogo», que es un falso positivo que
// castiga al que escribe bien.
func aiNombresLocalesSQL(sql string) map[string]bool {
	locales := map[string]bool{}
	toks := aiTokenSQLPat.FindAllStringSubmatch(aiStripSQLCommentsAndLiterals(sql), -1)
	profundidad := 0
	for i, m := range toks {
		if m[2] == "(" {
			profundidad++
			continue
		}
		if m[2] == ")" {
			if profundidad > 0 {
				profundidad--
			}
			continue
		}
		// Un nombre de CTE está al nivel exterior y en la forma exacta
		// `nombre AS (`: si se aceptara cualquier `x AS y` del SELECT, un alias
		// de columna (`SELECT invoice AS cliente`) entraría en la lista de
		// «nombres locales» y le daría al traste con el control de tablas cada
		// vez que una columna se llame como una tabla.
		if profundidad == 0 && strings.ToLower(m[1]) == "as" && i > 0 && i+1 < len(toks) &&
			toks[i-1][2] == "" && toks[i+1][2] == "(" {
			locales[strings.ToLower(toks[i-1][1])] = true
		}
	}
	return locales
}

// aiTablaProhibida dice si el nombre exacto de tabla está en la lista negra dura.
func aiTablaProhibida(tabla string) bool {
	_, ok := aiTablasProhibidasEnSQL[strings.ToLower(strings.TrimSpace(tabla))]
	return ok
}

// aiMencionaTablaProhibida busca la lista negra en TODO el texto utilizable de la
// sentencia (sin literales ni comentarios), con frontera de palabra.
//
// POR QUÉ ADEMÁS DEL FROM. aiTablasEnSQL mira las posiciones donde syntácticamente
// va una tabla. Esta pasada cubre el resto: una tabla prohibida escrita en cualquier
// otro sitio (`DELETE` no llega porque el guardián SELECT ya la cortó, pero sí una
// referencia rara, un `EXPLAIN` de tabla, una función de sistema con nombre de
// tabla) deja de ser un caso que dependa de que el escáner la haya previsto. La
// frontera de palabra evita el falso positivo de siempre: `vat_retention` no
// contiene la palabra `retention` como nombre (el guion bajo es carácter de
// identificador), y `created_at` nunca fue un problema por el mismo motivo.
func aiMencionaTablaProhibida(stripped string) string {
	low := strings.ToLower(stripped)
	// Orden establecido: el motivo del rechazo debe ser siempre el mismo nombre
	// para el mismo SQL (ver aiListaNegraOrdenada).
	for _, nombre := range aiListaNegraOrdenada() {
		if aiContainsKeyword(low, nombre) {
			return nombre
		}
	}
	return ""
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
//
// IT queda exenta de esta capa: su conjunto «permitido» es el catálogo, y exigir
// catálogo a un administrador que ejecuta un informe clásico sobre una tabla de
// empresa recién creada sería un candado nuevo sobre una puerta que la auditoría
// no señala. Lo que IT NO puede saltarse es la lista negra (aiValidarTablasSQL) ni
// el guardián SELECT. Cerrar del todo el caso IT requiere vistas de reporting con
// usuario de base restringido —la recomendación de raíz del informe S-4—, que es
// trabajo de esquema, no de esta función.
func aiValidarScopeSQL(sql, dept string) error {
	d := aiDeptNormalizar(dept)
	if d == DeptIT {
		return nil
	}
	permitidas := aiTablasPermitidasParaDept(d)
	locales := aiNombresLocalesSQL(sql)
	var denegadas []string
	for _, t := range aiTablasEnSQL(sql) {
		if locales[t] || permitidas[t] {
			continue
		}
		// Aquí solo se rechaza por ALCANCE: la lista negra (credenciales,
		// identidad, internos de la IA) ya se cortó antes, en validateReportSQL,
		// y corta a todos —también a IT—.
		denegadas = append(denegadas, t)
	}
	if len(denegadas) == 0 {
		return nil
	}
	// Mensaje en el mismo idioma que el resto del producto: el agente lo lee y
	// lo reporta, no lo traga en silencio.
	return fmt.Errorf("tu rol (%s) no tiene acceso a %s; solicítalo a un administrador si lo necesitas",
		aiDeptLabel[d], strings.Join(denegadas, ", "))
}

// aiValidarTablasSQL es la capa de LECTURA de la frontera: lista negra dura para
// todos + alcance del departamento que ejecuta. Delega el veredicto en
// validateReportSQL (ai_governance.go), que es la función estricta que la
// auditoría encontró muerta y que desde aquí es la única que decide qué tablas se
// pueden leer.
func aiValidarTablasSQL(sql, dept string) error {
	v := validateReportSQL(sql, dept)
	if v.Allowed {
		return nil
	}
	return aiRechazoTablas{motivo: v.Reason}
}

// aiPalabrasDeOrdenProhibidas son operadores que la frontera rechaza de plano
// aunque el escáner ya sepa leerlos. STRAIGHT_JOIN es el caso: su única función es
// fijar el orden de tablas, y el orden de tablas nunca es una razón para leer una
// tabla que no te toca. Rechazarlo en vez de «tolerarlo bien interpretado» deja un
// solo criterio: si aparece, no se ejecuta.
var aiPalabrasDeOrdenProhibidas = []string{"straight_join"}

// aiNormalizarSQLInforme deja el SQL en el ÚNICO texto que se valida y que se
// ejecuta: sin comentarios, con los literales intactos, y RECHAZANDO lo que no se
// puede normalizar con seguridad.
//
// POR QUÉ EL TEXTO NORMALIZADO ES EL QUE SE EJECUTA (F4 de la auditoría). Antes se
// validaba una cadena y se ejecutaba otra: el guardián quitaba los comentarios para
// analizar, pero MySQL ejecutaba el crudo —y un `/*! SELECT api_key FROM
// ai_providers */` se saltaba el análisis porque el texto «limpio» no lo veía
// mientras el servidor sí lo ejecutaba (los comentarios de versión son CÓDIGO en
// MySQL). Con esta función la cadena que devuelve es la que viaja hasta Query(), así
// que lo analizado y lo ejecutado son idénticos por construcción.
//
// Rechaza, en lugar de limpiar:
//   - comentarios ejecutables: `/*!...*/`, `/*+ ... */` (hints del optimizador) y
//     `/*!40101 ... SET ...*/`. Nadie los necesita en un informe y no hay forma de
//     saber lo que MySQL hará con ellos sin ejecutarlo.
//   - un comentario o un literal sin cerrar: significaría que el texto que ve el
//     servidor no es el texto que vimos nosotros.
func aiNormalizarSQLInforme(raw string) (string, error) {
	r := []rune(strings.TrimSpace(raw))
	var b strings.Builder
	b.Grow(len(raw))

	i := 0
	for i < len(r) {
		c := r[i]
		switch {
		case c == '\'' || c == '"':
			// Literal de texto (MySQL usa también la comilla doble cuando el
			// sql_mode no trae ANSI_QUOTES). Se copia tal cual: es DATO.
			fin := -1
			for j := i + 1; j < len(r); j++ {
				if r[j] == '\\' {
					j++ // MySQL interpreta la barra como escape
					continue
				}
				if r[j] == c {
					if j+1 < len(r) && r[j+1] == c {
						j++ // comilla duplicada: sigue dentro del literal
						continue
					}
					fin = j
					break
				}
			}
			if fin < 0 {
				return "", fmt.Errorf("el SQL tiene un literal de texto sin cerrar (comilla %q): no se puede comprobar qué se ejecuta", string(c))
			}
			b.WriteString(string(r[i : fin+1]))
			i = fin + 1
		case c == '`':
			// Identificador delimitado: se copia entero (un `--` dentro es parte
			// del nombre, no un comentario).
			fin := -1
			for j := i + 1; j < len(r); j++ {
				if r[j] == '`' {
					if j+1 < len(r) && r[j+1] == '`' {
						j++
						continue
					}
					fin = j
					break
				}
			}
			if fin < 0 {
				return "", fmt.Errorf("el SQL tiene un identificador entre acentos graves sin cerrar")
			}
			b.WriteString(string(r[i : fin+1]))
			i = fin + 1
		case c == '/' && i+1 < len(r) && r[i+1] == '*':
			if i+2 < len(r) && (r[i+2] == '!' || r[i+2] == '+' || r[i+2] == '@') {
				return "", fmt.Errorf("los comentarios ejecutables (/*! o /*+) no se permiten en SQL de informes: MySQL ejecuta su contenido, así que validar el texto limpio y ejecutar el crudo sería validar otra cosa")
			}
			fin := -1
			for j := i + 2; j+1 < len(r); j++ {
				if r[j] == '*' && r[j+1] == '/' {
					fin = j + 1
					break
				}
			}
			if fin < 0 {
				return "", fmt.Errorf("el SQL tiene un comentario /* sin cerrar")
			}
			b.WriteRune(' ')
			i = fin + 1
		case c == '-' && i+1 < len(r) && r[i+1] == '-' &&
			(i+2 >= len(r) || r[i+2] == ' ' || r[i+2] == '\t' || r[i+2] == '\r' || r[i+2] == '\n'):
			// Comentario de línea de MySQL: `--` seguido de espacio. Se quita
			// hasta el fin de línea.
			hasta := i
			for hasta < len(r) && r[hasta] != '\n' {
				hasta++
			}
			b.WriteRune(' ')
			i = hasta
		case c == '#':
			hasta := i
			for hasta < len(r) && r[hasta] != '\n' {
				hasta++
			}
			b.WriteRune(' ')
			i = hasta
		default:
			b.WriteRune(c)
			i++
		}
	}
	return strings.TrimSpace(b.String()), nil
}

// validarSQLInformes es LA frontera: recibe el SQL tal como lo escribió el modelo
// (o el usuario desde la pantalla de informes) y devuelve la cadena aprobada, que
// es la que se ejecuta. No hay segunda validación pendiente en el llamador: si
// esta función no lo aprueba, no se ejecuta.
//
// Es la respuesta a S-4/F2/F4/M-3 de la auditoría:
//   - F2: la lista negra dura (credenciales, identidad, sesiones, auditoría,
//     migraciones, claves del POS y configuración fiscal/contable) se aplica por
//     dondequiera que salga SQL de informes, no solo en db.query.
//   - F4: el alcance departamental es el de QUIEN EJECUTA (lo resuelve el
//     llamador con aiDeptDelEjecutor o con el departamento del agente), nunca el
//     del que creó la plantilla. Un informe guardado por IT lo puede ejecutar un
//     empleado de cartera: ahí se le aplica el alcance de cartera.
//   - M-3: comentarios ejecutables y STRAIGHT_JOIN no se pasan: se normaliza una
//     vez y se valida y se ejecuta ESA cadena.
//
// maxRows <= 0 usa el tope duro del sistema (aiHardMaxRows).
func validarSQLInformes(raw, dept string, maxRows int) (string, error) {
	normal, err := aiNormalizarSQLInforme(raw)
	if err != nil {
		return "", err
	}
	// 1. Guardián de ESCRITURA: un único SELECT, sin DML/DDL, con LIMIT forzado.
	//    Corre sobre la cadena normalizada, no sobre el crudo.
	aprobado, err := aiValidateSQLSelect(normal, maxRows)
	if err != nil {
		return "", err
	}
	// 2. Operadores que no tienen motivo legítimo en un informe.
	low := strings.ToLower(aiStripSQLCommentsAndLiterals(aprobado))
	for _, kw := range aiPalabrasDeOrdenProhibidas {
		if aiContainsKeyword(low, kw) {
			return "", fmt.Errorf("%s no se permite en SQL de informes: el orden de tablas nunca justifica leer una tabla fuera de tu alcance", strings.ToUpper(kw))
		}
	}
	// 3. Lista negra dura + alcance del que ejecuta (validateReportSQL revive para
	//    esto: es la función estricta que la auditoría encontró sin llamadores).
	if err := aiValidarTablasSQL(aprobado, dept); err != nil {
		return "", err
	}
	return aprobado, nil
}

// aiValidarSQLConScope es el guardián completo que usaba db.query: escritura
// (SELECT puro) más lectura (lista negra + alcance). Se delega íntegro en la
// frontera única: db.query y los informes no pueden tener criterios distintos de
// lectura, porque entonces el agujero se busca por la puerta que falle.
func aiValidarSQLConScope(sql, dept string, maxRows int) (string, error) {
	return validarSQLInformes(sql, dept, maxRows)
}

// aiDeptDelEjecutor resuelve el departamento de QUIEN EJECUTA un SQL de informe.
//
// POR QUÉ NO EL DEL CREADOR. La plantilla la diseña el agente de TI —que ve el
// catálogo completo— y la ejecuta quien la abre en pantalla. Si el alcance se
// tomara del creador, un informe de IT ejecutado por un empleado de cartera leería
// nómina. El alcance tiene que ser el de la persona/agentE que está delante de la
// consulta, que es además lo que la interfaz ya muestra en los permisos.
//
// Orden de resolución:
//  1. Sin sesión: IT. No hay identidad de la que fiarse; la lista negra y el
//     guardián SELECT siguen aplicando, y el tope duro de filas también.
//  2. Sesión de agente (UserLevel «agent»): el departamento declarado del agente
//     (ai_agent.department o el nombre de su rol), que es el mismo criterio con el
//     que se le scopó el prompt: prompt y candado dicen lo mismo.
//  3. master/admin: IT (paridad con la interfaz, donde ven todos los módulos).
//  4. Cualquier otro usuario: el nombre de su rol (sec_role.type), con la misma
//     función que usan los agentes, para que un rol de «cuentas por pagar» lea
//     cuentas por pagar desde la pantalla de informes igual que su agente.
func (s *Server) aiDeptDelEjecutor(sess *SessionData) string {
	if sess == nil {
		return DeptIT
	}
	// Identidad de agente: SOLO se consulta ai_agent cuando la sesión se declara
	// agente (así la construye el runner). Un usuario normal que abre un informe
	// no tiene por qué pagar la lectura de la tabla de agentes en cada run.
	if sess.UserLevel == "agent" {
		if agente, ok := s.loadAIAgentByUser(sess, sess.UserID); ok {
			return aiDeptDelAgente(agente)
		}
	}
	level := ""
	for _, db := range s.roleDBsForSession(sess) {
		if db == nil {
			continue
		}
		if l, err := loadUserRole(db, sess.UserID); err == nil && l != "" {
			level = l
			break
		}
	}
	if level == "" {
		// Sin fila de rol en ninguna base: se usa el nivel de la sesión, que es
		// lo que la interfaz ya está usando para pintar el menú.
		level = sess.UserLevel
	}
	if level == "master" || level == "admin" {
		return DeptIT
	}
	if tipo := s.aiTipoRolDeSesion(sess); tipo != "" {
		return aiDeptDeRolNombre(tipo)
	}
	return aiDeptDeRolNombre(level)
}

// aiTipoRolDeSesion devuelve sec_role.type del rol de la sesión (empresa primero,
// control después: el rol se asigna en una u otra según la instalación, y es el
// mismo orden que recorre aiBuildRoleGate).
func (s *Server) aiTipoRolDeSesion(sess *SessionData) string {
	if sess == nil {
		return ""
	}
	roleID := s.sessionRoleID(sess)
	if roleID <= 0 {
		return ""
	}
	for _, db := range s.roleDBsForSession(sess) {
		if db == nil {
			continue
		}
		var tipo string
		if err := db.QueryRow(`SELECT type FROM sec_role WHERE id = ? LIMIT 1`, roleID).Scan(&tipo); err == nil {
			return strings.TrimSpace(tipo)
		}
	}
	return ""
}

// aiErrorInformeNoEjecutable redacta el mensaje que ve quien abre un informe
// guardado hace meses cuyo SQL ya no pasa la frontera.
//
// La norma es explícita (A.4 de la hoja de trabajo): una plantilla que toque una
// tabla prohibida NO se borra ni se «migran» sus datos: deja de ser ejecutable y
// dice por qué. Borrar definiciones sería destruir trabajo del usuario; ejecutarlas
// sería el agujero. El término medio es el que está escrito abajo.
func aiErrorInformeNoEjecutable(nombre string, err error) error {
	if esRechazoDeTablas(err) {
		return fmt.Errorf("el informe %q no se puede ejecutar: su SQL usa tablas que la frontera de informes no admite. %v "+
			"La plantilla NO se ha borrado: corrige su definición con tablas de negocio y vuelve a ejecutarlo", nombre, err)
	}
	return fmt.Errorf("el informe %q no se pudo ejecutar: %w", nombre, err)
}
