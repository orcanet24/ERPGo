package web

// addon_memo.go — Handlers de negocio de Memo: los verbos «nota.crear»,
// «grupo.crear» y «grupo.miembros» (docs/PLAN-MEMO.md §5, fases A y B).
//
// POR QUÉ VIVE AQUÍ. La frontera valida el CONTEXTO (token, verbo declarado,
// sesión viva, empresa, permiso); el QUÉ hacer ya es negocio del módulo y se
// escribe aquí, junto al resto de handlers, bajando por el orquestador con su
// journal y su audit_log. El addon no tiene credenciales: pide, y el núcleo
// aplica. Cada fase nueva de Memo agrega su verbo con su handler y su prueba;
// la frontera no se toca.
//
// LA REGLA QUE ESTE ARCHIVO PROTEGE: la visibilidad. Una nota `privada` no
// admite destinatarios (son suyos por definición), una `empresa` no los
// necesita (la ve la empresa entera), una `personas` sin destinatarios no se
// guarda (quedaría compartida con nadie) y una `grupo` sin grupos tampoco (con
// un grupo vacío la vería solo su autor, que es la misma mentira).
//
// LA OTRA REGLA, que la fase B afina: la identidad. Todo lo que se comparte y
// toda fila de `autor` se guarda en el MISMO espacio de id que trae
// X-ERP-User y con el que compara la consulta del muro, `users.user_id`. La
// fase A firmaba las notas con el nombre de login (`user_login.username`)
// porque «queda más legible», y con eso el autor dejó de ver sus propias
// notas: dos espacios de id que no se cruzaban nunca.

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"erpgo/internal/orchestrator"
)

// Tablas propias del módulo, declaradas en su manifiesto y creadas por su
// schema.sql. Este archivo no crea tablas: si no existen, el orquestador
// rechaza la escritura con su error, que es lo que debe pasar.
const (
	tablaMemoNotas         = "memo_notas"
	tablaMemoDestinatarios = "memo_destinatarios"
	tablaMemoGrupos        = "memo_grupos"
	tablaMemoMiembros      = "memo_grupo_miembros"
	tablaMemoInbox         = "memo_inbox"
)

// Límites que la pantalla también impone. El canal de negocio se puede llamar
// sin pasar por la pantalla, así que el candado se comprueba aquí igual.
const (
	memoMaxTitulo        = 200
	memoMaxTexto         = 20000
	memoMaxDestinatarios = 100 // personas por nota, o miembros por grupo
	memoMaxGrupos        = 10  // grupos por nota: acota el reparto Y los avisos
	memoMaxNombreGrupo   = 120
	memoMaxAvisos        = 500 // personas a las que UNA nota puede avisar
)

// memoReUsuario forma de un id de usuario del ERP (users.user_id, varchar(15)).
// Un id con signos raros no es una persona: es un intento de meter datos donde
// después se leerán como identidad.
var memoReUsuario = regexp.MustCompile(`^[A-Za-z0-9_-]{1,15}$`)

// memoReGrupo reconoce SOLO ids de grupo que generó este mismo archivo. Un id
// de grupo no es una persona: si se mezclaran, el destino sería un valor que
// nadie puede leer en la tabla de miembros. Al ser estricto, lo inventado se
// rechaza en la puerta en vez de buscarse en vano después.
var memoReGrupo = regexp.MustCompile(`^memo-grp-[0-9a-f]{16}$`)

// memoVisibilidades son los cuatro repartos admitidos: la fase B añadió
// «grupo» a los tres de la fase A.
var memoVisibilidades = map[string]bool{"privada": true, "empresa": true, "personas": true, "grupo": true}

// addonModuloMemoVerbos conecta los verbos del módulo Memo. Se llama desde
// NewServer después de addonsIniciar: la frontera ya cargó el manifiesto y
// validó que nota.crear esté declarado en db_write con su permiso
// (memo_publicar); aquí solo se conecta el QUÉ hacer.
func (s *Server) addonModuloMemoVerbos() {
	if s == nil || s.addons == nil {
		return // sin frontera no hay verbo que conectar (arranques en pruebas)
	}
	s.addonVerboRegistrar("memo", "nota.crear", s.addonMemoNotaCrear)
	s.addonVerboRegistrar("memo", "grupo.crear", s.addonMemoGrupoCrear)
	s.addonVerboRegistrar("memo", "grupo.miembros", s.addonMemoGrupoMiembros)
}

// addonMemoNotaCrear publica una nota. Cuerpo esperado (del addon, en JSON):
//
//	{"titulo": "…", "texto": "…",
//	 "visibilidad": "privada|empresa|personas|grupo",
//	 "destinatarios": ["7", "9"], "grupos": ["memo-grp-…"]}
//
// El contexto de sesión (ses) llega YA VALIDADO de la frontera: aquí no se
// vuelve a preguntar quién llamó, solo se usa.
func (s *Server) addonMemoNotaCrear(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	titulo := memoTexto(cuerpo["titulo"])
	texto := memoTexto(cuerpo["texto"])
	visibilidad := strings.ToLower(memoTexto(cuerpo["visibilidad"]))
	if visibilidad == "" {
		visibilidad = "empresa"
	}
	if titulo == "" {
		return nil, fmt.Errorf("falta el campo titulo (el cuerpo del verbo nota.crear es {\"titulo\": …, \"texto\": …, \"visibilidad\": …, \"destinatarios\": […]})")
	}
	if texto == "" {
		return nil, fmt.Errorf("falta el campo texto: una nota vacía no se guarda")
	}
	if utf8.RuneCountInString(titulo) > memoMaxTitulo {
		return nil, fmt.Errorf("el título excede los %d caracteres que la columna admite", memoMaxTitulo)
	}
	if utf8.RuneCountInString(texto) > memoMaxTexto {
		return nil, fmt.Errorf("el texto excede los %d caracteres que la columna admite", memoMaxTexto)
	}
	if !memoVisibilidades[visibilidad] {
		return nil, fmt.Errorf("la visibilidad %q no existe: debe ser privada, empresa o personas", visibilidad)
	}

	// Los destinatarios solo significan algo en «personas»: en «privada» la
	// nota es del autor y en «empresa» la ve la empresa. Se descartan en vez
	// de guardarlos, para que la tabla diga la verdad.
	destinatarios := []string{}
	if visibilidad == "personas" {
		destinatarios = memoDestinatarios(cuerpo["destinatarios"])
		if len(destinatarios) == 0 {
			return nil, fmt.Errorf("visibilidad «personas» sin destinatarios: la nota quedaría compartida con nadie")
		}
		if len(destinatarios) > memoMaxDestinatarios {
			return nil, fmt.Errorf("una nota no se comparte con más de %d personas a la vez (%d pedidas)", memoMaxDestinatarios, len(destinatarios))
		}
		vistos := map[string]bool{}
		for _, d := range destinatarios {
			if !memoReUsuario.MatchString(d) {
				return nil, fmt.Errorf("%q no es un id de usuario válido: revisa a quién se comparte", d)
			}
			if vistos[d] {
				return nil, fmt.Errorf("la persona %q está repetida en los destinatarios", d)
			}
			vistos[d] = true
		}
	}

	// Los grupos solo significan algo en «grupo». Y se exige al menos uno:
	// una nota «de grupo» sin grupo es la misma mentira que «personas» sin
	// personas — se publicaría para nadie.
	grupos := []string{}
	if visibilidad == "grupo" {
		grupos = memoDestinatarios(cuerpo["grupos"])
		if len(grupos) == 0 {
			return nil, fmt.Errorf("visibilidad «grupo» sin grupos: la nota quedaría compartida con nadie")
		}
		if len(grupos) > memoMaxGrupos {
			return nil, fmt.Errorf("una nota no se reparte con más de %d grupos a la vez (%d pedidos)", memoMaxGrupos, len(grupos))
		}
		vistos := map[string]bool{}
		for _, g := range grupos {
			if !memoReGrupo.MatchString(g) {
				return nil, fmt.Errorf("%q no es un id de grupo de Memo: elige los grupos en su selector", g)
			}
			if vistos[g] {
				return nil, fmt.Errorf("el grupo %q está repetido en el reparto", g)
			}
			vistos[g] = true
		}
	}

	// ANTES de escribir nada: a quién se le avisa (y si los grupos elegidos
	// tienen gente hoy). Si esta lectura falla, no se publica a medias.
	avisar, err := s.memoQuienSeAvisa(ses.Empresa, visibilidad, destinatarios, grupos)
	if err != nil {
		return nil, err
	}

	id := "memo-" + randomHex(8)
	// EL AUTOR es X-ERP-User (users.user_id): el mismo id con el que el muro
	// decide «lo mío» y con el que pinta el nombre. No el nombre de login.
	autor := ses.Usuario
	ctx := context.Background()

	res, err := s.Escribir(ctx, orchestrator.Entrada{
		Empresa:    ses.Empresa,
		Tabla:      tablaMemoNotas,
		Columnas:   []string{"id", "empresa", "autor", "titulo", "texto", "visibilidad", "tipo", "estado", "fijada", "creada", "actualizada"},
		Valores:    []any{id, ses.Empresa, autor, titulo, texto, visibilidad, "nota", "activo", 0, now(), now()},
		Clave:      "memo_notas:crear:" + id,
		Modulo:     "addon/memo",
		Accion:     "nota.crear",
		Usuario:    autor,
		IP:         "127.0.0.1 (canal addon)",
		RegistroID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("el orquestador rechazó la nota: %w", err)
	}

	// El reparto: filas de destinatario, sea una persona o un grupo. Un grupo
	// se guarda COMO GRUPO (destino_tipo='grupo') y no expandido a la gente:
	// la vista no la decide esta fila sino quién HOY es miembro, y por eso
	// salirse del grupo surte efecto en la siguiente lectura sin tocar ninguna
	// nota.
	type reparto struct{ tipo, id string }
	destinos := []reparto{}
	for _, p := range destinatarios {
		destinos = append(destinos, reparto{"persona", p})
	}
	for _, g := range grupos {
		destinos = append(destinos, reparto{"grupo", g})
	}

	// El reparto va DESPUÉS de la nota y cada fila con su propia clave de
	// idempotencia. Si la segunda falla, queda una nota sin compartir: el
	// fallo es del autor contra su propia nota, nunca una fuga hacia gente a
	// la que no se la compartió — el orden está elegido por eso.
	compartido := 0
	for _, d := range destinos {
		fila := "memo-" + randomHex(8)
		if _, err := s.Escribir(ctx, orchestrator.Entrada{
			Empresa:    ses.Empresa,
			Tabla:      tablaMemoDestinatarios,
			Columnas:   []string{"id", "nota_id", "destino_tipo", "destino_id", "empresa"},
			Valores:    []any{fila, id, d.tipo, d.id, ses.Empresa},
			Clave:      "memo_destinatarios:crear:" + id + ":" + d.tipo + ":" + d.id,
			Modulo:     "addon/memo",
			Accion:     "nota.crear." + d.tipo,
			Usuario:    autor,
			IP:         "127.0.0.1 (canal addon)",
			RegistroID: fila,
		}); err != nil {
			return nil, fmt.Errorf("la nota %s se guardó pero no se pudo compartirla con %s %s: %w", id, d.tipo, d.id, err)
		}
		compartido++
	}

	// La bandeja (fase B): cada destinatario recibe su aviso. El autor no
	// recibe el suyo — avisarse a uno mismo es ruido—. Si algo falla aquí, la
	// nota YA está publicada y compartida, y el error lo dice así: no se
	// esconde un problema de notificación bajo un «no se pudo publicar».
	avisados := 0
	for _, para := range avisar {
		if para == autor {
			continue
		}
		fila := "memo-av-" + randomHex(8)
		if _, err := s.Escribir(ctx, orchestrator.Entrada{
			Empresa:    ses.Empresa,
			Tabla:      tablaMemoInbox,
			Columnas:   []string{"id", "para", "tipo", "ref_id", "texto", "leida", "empresa", "creada"},
			Valores:    []any{fila, para, "compartida", id, titulo, 0, ses.Empresa, now()},
			Clave:      "memo_inbox:crear:" + id + ":" + para,
			Modulo:     "addon/memo",
			Accion:     "nota.crear.aviso",
			Usuario:    autor,
			IP:         "127.0.0.1 (canal addon)",
			RegistroID: fila,
		}); err != nil {
			return nil, fmt.Errorf("la nota %s quedó publicada y compartida, pero la bandeja de %s no recibió el aviso: %w", id, para, err)
		}
		avisados++
	}

	return map[string]interface{}{
		"registro_id": id,
		"compartido":  compartido,
		"avisados":    avisados,
		"duplicado":   res.Duplicado,
	}, nil
}

// memoTexto convierte el campo del cuerpo en texto recortado. Un campo que no
// es texto (número, objeto) se toma como vacío: el candado de arriba se queja
// con un motivo claro en lugar de fallar de forma rara más abajo.
func memoTexto(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// memoDestinatarios normaliza una lista de personas o de grupos del JSON: un
// array JSON llega como []interface{}, y un addon que mande una cadena suelta
// («destinatario»: "7") también se acepta — es un error de forma, no de fondo.
func memoDestinatarios(v interface{}) []string {
	salida := []string{}
	switch lista := v.(type) {
	case []interface{}:
		for _, item := range lista {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				salida = append(salida, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, item := range lista {
			if strings.TrimSpace(item) != "" {
				salida = append(salida, strings.TrimSpace(item))
			}
		}
	case string:
		if strings.TrimSpace(lista) != "" {
			salida = append(salida, strings.TrimSpace(lista))
		}
	}
	return salida
}

// ---------- Fase B: grupos de trabajo ----------

// addonMemoGrupoCrear crea un grupo con sus miembros. Cuerpo esperado:
//
//	{"nombre": "Logística", "miembros": ["7", "9"]}
func (s *Server) addonMemoGrupoCrear(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	nombre := memoTexto(cuerpo["nombre"])
	if nombre == "" {
		return nil, fmt.Errorf("falta el campo nombre: un grupo sin nombre no aparece en el selector del reparto")
	}
	if utf8.RuneCountInString(nombre) > memoMaxNombreGrupo {
		return nil, fmt.Errorf("el nombre del grupo excede los %d caracteres", memoMaxNombreGrupo)
	}
	miembros, err := memoMiembrosValidos(cuerpo["miembros"], "de miembros")
	if err != nil {
		return nil, err
	}
	if len(miembros) == 0 {
		return nil, fmt.Errorf("un grupo sin miembros no comparte nada: añade al menos a una persona")
	}

	autor := ses.Usuario
	ctx := context.Background()
	id := "memo-grp-" + randomHex(8)
	if _, err := s.Escribir(ctx, orchestrator.Entrada{
		Empresa:    ses.Empresa,
		Tabla:      tablaMemoGrupos,
		Columnas:   []string{"id", "empresa", "nombre", "creada"},
		Valores:    []any{id, ses.Empresa, nombre, now()},
		Clave:      "memo_grupos:crear:" + id,
		Modulo:     "addon/memo",
		Accion:     "grupo.crear",
		Usuario:    autor,
		IP:         "127.0.0.1 (canal addon)",
		RegistroID: id,
	}); err != nil {
		return nil, fmt.Errorf("el orquestador rechazó el grupo: %w", err)
	}

	// Los miembros van DESPUÉS y uno a uno, cada uno con su clave de
	// idempotencia: si el tercero falla queda un grupo con los dos primeros —
	// se arregla reenviando la lista—, y jamás un miembro sin grupo.
	for _, persona := range miembros {
		fila := "memo-mb-" + randomHex(8)
		if _, err := s.Escribir(ctx, orchestrator.Entrada{
			Empresa:    ses.Empresa,
			Tabla:      tablaMemoMiembros,
			Columnas:   []string{"id", "grupo_id", "persona_id", "empresa"},
			Valores:    []any{fila, id, persona, ses.Empresa},
			Clave:      "memo_grupo_miembros:crear:" + id + ":" + persona,
			Modulo:     "addon/memo",
			Accion:     "grupo.crear.miembro",
			Usuario:    autor,
			IP:         "127.0.0.1 (canal addon)",
			RegistroID: fila,
		}); err != nil {
			return nil, fmt.Errorf("el grupo %s se creó pero %s no entró: %w", id, persona, err)
		}
	}

	return map[string]interface{}{"registro_id": id, "miembros": len(miembros)}, nil
}

// addonMemoGrupoMiembros reescribe los miembros de un grupo. Cuerpo esperado:
//
//	{"grupo_id": "memo-grp-…", "miembros": ["7", "9"]}
//
// La lista es el ESTADO nuevo, no una diferencia: quien no esté al final no es
// miembro. Esa baja es la que hace desaparecer sus notas compartidas del muro
// en la siguiente lectura — ninguna nota se reescribe para lograrlo.
func (s *Server) addonMemoGrupoMiembros(p *addonProc, ses sesionDeAddon, cuerpo map[string]interface{}) (map[string]interface{}, error) {
	grupoID := memoTexto(cuerpo["grupo_id"])
	if !memoReGrupo.MatchString(grupoID) {
		return nil, fmt.Errorf("grupo_id %q no es un id de grupo de Memo", grupoID)
	}
	nuevos, err := memoMiembrosValidos(cuerpo["miembros"], "de miembros")
	if err != nil {
		return nil, err
	}
	// La lista puede venir vacía: sacar a todo el mundo es legítimo (el grupo
	// queda dormido y, mientras tanto, no reparte nada).

	ctx := context.Background()
	db, err := s.tenantDB(ses.Empresa)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la base de la empresa: %w", err)
	}

	// El grupo se comprueba en SU empresa: un id que exista en otra compañía
	// no es un grupo de esta sesión.
	var nombre string
	if err := db.QueryRowContext(ctx, `SELECT nombre FROM memo_grupos WHERE id = ? AND empresa = ?`, grupoID, ses.Empresa).Scan(&nombre); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("el grupo %s no existe en esta empresa", grupoID)
		}
		return nil, fmt.Errorf("no se pudo leer el grupo: %w", err)
	}

	actuales, err := memoMiembrosActuales(ctx, db, ses.Empresa, grupoID)
	if err != nil {
		return nil, err
	}
	venia := map[string]bool{}
	for _, a := range actuales {
		venia[a] = true
	}
	habia := map[string]bool{}
	for _, n := range nuevos {
		habia[n] = true
	}
	entrar := []string{}
	for _, n := range nuevos {
		if !venia[n] {
			entrar = append(entrar, n)
		}
	}
	borrar := []string{}
	for _, a := range actuales {
		if !habia[a] {
			borrar = append(borrar, a)
		}
	}

	autor := ses.Usuario
	for _, persona := range entrar {
		fila := "memo-mb-" + randomHex(8)
		if _, err := s.Escribir(ctx, orchestrator.Entrada{
			Empresa:    ses.Empresa,
			Tabla:      tablaMemoMiembros,
			Columnas:   []string{"id", "grupo_id", "persona_id", "empresa"},
			Valores:    []any{fila, grupoID, persona, ses.Empresa},
			Clave:      "memo_grupo_miembros:crear:" + grupoID + ":" + persona,
			Modulo:     "addon/memo",
			Accion:     "grupo.miembros.entrar",
			Usuario:    autor,
			IP:         "127.0.0.1 (canal addon)",
			RegistroID: fila,
		}); err != nil {
			return nil, fmt.Errorf("el grupo %s quedó a medias: %s no entró: %w", nombre, persona, err)
		}
	}

	if len(borrar) > 0 {
		marcas := strings.TrimSuffix(strings.Repeat("?,", len(borrar)), ",")
		args := []interface{}{ses.Empresa, grupoID}
		for _, b := range borrar {
			args = append(args, b)
		}
		// POR QUÉ un DELETE directo y no por el orquestador: su puerta solo
		// INSERTa (no hay «borrar» en su entrada) y esta tabla guarda ESTADO
		// ACTUAL de mantenimiento, como product_recipe o bank_information en el
		// resto del ERP. La sentencia va acotada a ESTA empresa, ESTE grupo y
		// ESTAS personas: un DELETE que no esté acotado así no se escribe.
		if _, err := db.ExecContext(ctx,
			`DELETE FROM memo_grupo_miembros WHERE empresa = ? AND grupo_id = ? AND persona_id IN (`+marcas+`)`, args...); err != nil {
			return nil, fmt.Errorf("el grupo %s quedó a medias: entraron %d y no se pudieron quitar %d (%s): %w",
				nombre, len(entrar), len(borrar), strings.Join(borrar, ", "), err)
		}
	}

	return map[string]interface{}{
		"registro_id": grupoID,
		"nombre":      nombre,
		"miembros":    len(nuevos),
		"entraron":    len(entrar),
		"bajaron":     len(borrar),
	}, nil
}

// memoMiembrosValidos normaliza y valida una lista de personas del ERP. El
// candado vive AQUÍ y no solo en la pantalla: el canal se puede llamar sin
// pasar por ella. `para` nombra la lista en los mensajes, para que el error
// diga en qué se falló.
func memoMiembrosValidos(v interface{}, para string) ([]string, error) {
	lista := memoDestinatarios(v)
	if len(lista) > memoMaxDestinatarios {
		return nil, fmt.Errorf("la lista %s no puede pasar de %d personas (%d pedidas)", para, memoMaxDestinatarios, len(lista))
	}
	vistos := map[string]bool{}
	for _, p := range lista {
		if !memoReUsuario.MatchString(p) {
			return nil, fmt.Errorf("%q no es un id de usuario válido: revisa la lista %s", p, para)
		}
		if vistos[p] {
			return nil, fmt.Errorf("la persona %s está repetida en la lista %s", p, para)
		}
		vistos[p] = true
	}
	return lista, nil
}

// memoMiembrosActuales devuelve quién es miembro de un grupo HOY, en la base
// de la empresa indicada.
func memoMiembrosActuales(ctx context.Context, db *sql.DB, empresa, grupo string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT persona_id FROM memo_grupo_miembros WHERE empresa = ? AND grupo_id = ?`, empresa, grupo)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer los miembros del grupo: %w", err)
	}
	defer rows.Close()
	salida := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		salida = append(salida, p)
	}
	return salida, rows.Err()
}

// memoQuienSeAvisa resuelve a quién le entra un aviso por compartir la nota.
// Para «personas» son los propios destinatarios; para «grupo» se expanden SUS
// miembros EN ESTE MOMENTO: una notificación es el registro de un hecho
// fechado, no una vista viva —quien entre al grupo después verá la nota en su
// muro, pero no recibirá el aviso de ese día—.
//
// Se niega a seguir si un grupo elegido no existe o no tiene gente hoy:
// compartir con un grupo vacío es compartir con nadie con buena cara.
func (s *Server) memoQuienSeAvisa(empresa, visibilidad string, destinatarios, grupos []string) ([]string, error) {
	if visibilidad == "personas" {
		return append([]string{}, destinatarios...), nil
	}
	if visibilidad != "grupo" {
		return nil, nil // privada y empresa no avisan a nadie en particular
	}
	db, err := s.tenantDB(empresa)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la base de la empresa para resolver los grupos: %w", err)
	}
	leidos, err := memoLeerGruposConMiembros(context.Background(), db, empresa, grupos)
	if err != nil {
		return nil, err
	}

	salida := []string{}
	vistos := map[string]bool{}
	for _, g := range grupos {
		ficha := leidos[g]
		if ficha == nil {
			return nil, fmt.Errorf("el grupo %s no existe en esta empresa", g)
		}
		if len(ficha.Miembros) == 0 {
			return nil, fmt.Errorf("el grupo %q no tiene miembros hoy: añade gente o elige otro", ficha.Nombre)
		}
		for _, p := range ficha.Miembros {
			if !vistos[p] {
				vistos[p] = true
				salida = append(salida, p)
			}
		}
	}
	if len(salida) > memoMaxAvisos {
		return nil, fmt.Errorf("esta nota llegaría a %d personas en su bandeja (máximo %d): repártela en varias notas",
			len(salida), memoMaxAvisos)
	}
	return salida, nil
}

// memoGrupoLeido es lo que se sabe de un grupo al resolver el reparto.
type memoGrupoLeido struct {
	Nombre   string
	Miembros []string
}

// memoLeerGruposConMiembros trae, de UNA sola consulta, los grupos pedidos con
// su nombre y sus miembros actuales. El LEFT JOIN hace que un grupo sin gente
// siga apareciendo (con el miembro nulo): así se distingue «no existe» de
// «existe y está vacío», que son dos errores distintos para quien lo usa.
func memoLeerGruposConMiembros(ctx context.Context, db *sql.DB, empresa string, grupos []string) (map[string]*memoGrupoLeido, error) {
	salida := map[string]*memoGrupoLeido{}
	if len(grupos) == 0 {
		return salida, nil
	}
	marcas := strings.TrimSuffix(strings.Repeat("?,", len(grupos)), ",")
	args := []interface{}{empresa}
	for _, g := range grupos {
		args = append(args, g)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT g.id, g.nombre, m.persona_id
		FROM memo_grupos g
		LEFT JOIN memo_grupo_miembros m ON m.grupo_id = g.id AND m.empresa = g.empresa
		WHERE g.empresa = ? AND g.id IN (`+marcas+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer los grupos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, nombre string
		var persona sql.NullString
		if err := rows.Scan(&id, &nombre, &persona); err != nil {
			return nil, err
		}
		ficha := salida[id]
		if ficha == nil {
			ficha = &memoGrupoLeido{Nombre: nombre}
			salida[id] = ficha
		}
		if persona.Valid && strings.TrimSpace(persona.String) != "" {
			ficha.Miembros = append(ficha.Miembros, strings.TrimSpace(persona.String))
		}
	}
	return salida, rows.Err()
}
