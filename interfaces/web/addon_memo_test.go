package web

// addon_memo_test.go — Pruebas de la fase A del módulo Memo
// (docs/PLAN-MEMO.md, fase A).
//
// Lo que protegen, en orden:
//
//   - El candado: el bundle REAL del repo (manifest.json + schema.sql) pasa
//     el escáner tal cual lo corre el arranque.
//   - El registro: filas en modules/menu y sus DOS tablas creadas por la
//     migración declarada.
//   - El verbo: valida antes de escribir (título, texto, visibilidad y
//     destinatarios) y la escritura feliz baja por EL ORQUESTADOR con su
//     idempotencia, no por un INSERT suelto.
//   - La regla de visibilidad de los destinatarios: una nota privada no
//     admite reparto y una «personas» sin destinatarios no se guarda.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// carpetaMemoDelRepo copia el bundle real de addons/memo a una carpeta
// addons temporal: lo que se prueba es ese bundle, no una copia reescrita.
func carpetaMemoDelRepo(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "memo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nombre := range []string{"manifest.json", "schema.sql"} {
		contenido, err := os.ReadFile(filepath.Join("..", "..", "addons", "memo", nombre))
		if err != nil {
			t.Fatalf("el módulo memo del repo no trae %s: %v", nombre, err)
		}
		if err := os.WriteFile(filepath.Join(dir, nombre), contenido, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return raiz
}

func TestElManifiestoDelModuloMemoDelRepoPasaElCandado(t *testing.T) {
	_, h := hostDeAddon(t, carpetaMemoDelRepo(t))
	fila, avisos := h.escanear()
	if len(avisos) > 0 {
		t.Fatalf("el bundle de memo no pasó el escáner: %v", avisos)
	}
	p := fila["memo"]
	if p == nil {
		t.Fatal("el módulo memo no cargó con su manifiesto del repo")
	}
	if len(p.migraciones) != 5 {
		t.Fatalf("la fase B declara cinco tablas (las dos de la A, más grupos, miembros y bandeja) y su schema.sql debía dar cinco sentencias, dio %d", len(p.migraciones))
	}
	// Cada fase añade su verbo con su permiso: sin grupo.crear no hay reparto,
	// y sin grupo.miembros no hay forma de que alguien deje de ser miembro.
	verbosEsperados := map[string]string{
		"nota.crear":     "memo_publicar",
		"grupo.crear":    "memo_grupos",
		"grupo.miembros": "memo_grupos",
	}
	if len(p.manifiesto.DBWrite) != len(verbosEsperados) {
		t.Fatalf("la fase B declara %d verbos y el manifiesto trae %d: %v", len(verbosEsperados), len(p.manifiesto.DBWrite), p.manifiesto.DBWrite)
	}
	for _, v := range p.manifiesto.DBWrite {
		esperado, esSuyo := verbosEsperados[v.Verb]
		if !esSuyo {
			t.Errorf("el verbo %q no es de la fase B de memo: %v", v.Verb, p.manifiesto.DBWrite)
			continue
		}
		if v.Perm != esperado {
			t.Errorf("%q debe exigir %q, exige %q", v.Verb, esperado, v.Perm)
		}
	}
	// El permiso de grupos debe estar DECLARADO: si el manifiesto pide un
	// candado que él no define, el panel enseña una cerradura que nadie puede
	// conceder.
	permisos := map[string]bool{}
	for _, pc := range p.manifiesto.Perms {
		permisos[pc.Code] = true
	}
	for _, requerido := range []string{"memo_ver", "memo_publicar", "memo_grupos"} {
		if !permisos[requerido] {
			t.Errorf("el manifiesto no declara el permiso %q: %v", requerido, p.manifiesto.Perms)
		}
	}
	// La gente se lee de la base de identidad: sin `users` en db_read el
	// muro no tendría con qué nombrar a nadie.
	encontrada := false
	for _, tabla := range p.manifiesto.DBRead {
		if tabla == "users" {
			encontrada = true
		}
	}
	if !encontrada {
		t.Fatalf("db_read no declara `users`: el muro no podría nombrar a la gente (%v)", p.manifiesto.DBRead)
	}
}

func TestElModuloMemoSeRegistraEnModulosYMenuYSusTablasSeCrean(t *testing.T) {
	s, h := hostDeAddon(t, carpetaMemoDelRepo(t))
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}

	h.mu.Lock()
	cargado := h.progs["memo"] != nil
	h.mu.Unlock()
	if !cargado {
		t.Fatal("el módulo memo no quedó cargado tras el escaneo")
	}

	var activo int
	if err := s.db.QueryRow(`SELECT is_active FROM modules WHERE id = 'memo'`).Scan(&activo); err != nil {
		t.Fatalf("memo no dejó fila en modules: %v", err)
	}
	if activo != 1 {
		t.Fatal("memo debía quedar activo en modules")
	}

	var url, perm string
	if err := s.db.QueryRow(`SELECT url, perm FROM menu WHERE module_id = 'memo'`).Scan(&url, &perm); err != nil {
		t.Fatalf("memo no dejó entrada de menú: %v", err)
	}
	if url != "/memo" || perm != "memo_ver" {
		t.Fatalf("el menú de memo quedó raro (url=%q perm=%q)", url, perm)
	}

	for _, tabla := range []string{"memo_notas", "memo_destinatarios"} {
		var filas int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + tabla).Scan(&filas); err != nil {
			t.Fatalf("la migración de %s no se aplicó: %v", tabla, err)
		}
	}
}

// TestElVerboDelModuloMemoValidaAntesDeEscribir: el candado se dobla en el
// handler, porque el canal se puede llamar sin pasar por la pantalla.
func TestElVerboDelModuloMemoValidaAntesDeEscribir(t *testing.T) {
	s, _ := hostDeAddon(t, carpetaMemoDelRepo(t))
	ses := sesionDeAddon{Usuario: "7", Empresa: defaultTenantCode, Sess: sessPrueba("master")}

	casos := []struct {
		nombre string
		cuerpo map[string]interface{}
		que    string
	}{
		{"sin título", map[string]interface{}{"texto": "hola", "visibilidad": "empresa"}, "titulo"},
		{"sin texto", map[string]interface{}{"titulo": "Reunión", "visibilidad": "empresa"}, "texto"},
		{"visibilidad inventada", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "todos"}, "no existe"},
		{"personas sin destinatarios", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "personas"}, "sin destinatarios"},
		{"destinatario que no es un id", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "personas",
			"destinatarios": []interface{}{"7'; DROP TABLE memo_notas--"}}, "id de usuario válido"},
		{"destinatario repetido", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "personas",
			"destinatarios": []interface{}{"7", "7"}}, "repetida"},
		{"título larguísimo", map[string]interface{}{
			"titulo": strings.Repeat("x", memoMaxTitulo+1), "texto": "y", "visibilidad": "empresa"}, "caracteres"},
		// ── Fase B: el reparto por grupo se valida igual de estricto ──
		{"grupo sin grupos", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "grupo"}, "sin grupos"},
		{"grupo que no es un id de Memo", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "grupo",
			"grupos": []interface{}{"logística"}}, "no es un id de grupo"},
		{"grupo repetido", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "grupo",
			"grupos": []interface{}{"memo-grp-0123456789abcdef", "memo-grp-0123456789abcdef"}}, "repetido"},
		{"más grupos de los que caben", map[string]interface{}{
			"titulo": "x", "texto": "y", "visibilidad": "grupo",
			"grupos": gruposDePrueba(memoMaxGrupos + 1)}, "no se reparte con más de"},
	}
	for _, c := range casos {
		if _, err := s.addonMemoNotaCrear(nil, ses, c.cuerpo); err == nil {
			t.Errorf("%s: debió rechazarse y pasó", c.nombre)
		} else if !strings.Contains(err.Error(), c.que) {
			t.Errorf("%s: el motivo debía mencionar %q, dio %q", c.nombre, c.que, err.Error())
		}
	}
}

// gruposDePrueba arma una lista de ids de grupo con la forma que genera este
// módulo (memo-grp- + 16 hex), para probar límites sin inventar valores que
// otro candado rechazaría antes de medir lo que toca.
func gruposDePrueba(n int) []interface{} {
	fuera := []interface{}{}
	for i := 0; i < n; i++ {
		fuera = append(fuera, fmt.Sprintf("memo-grp-%016x", i))
	}
	return fuera
}

// TestLosVerbosDeGrupoValidanAntesDeEscribir: la puerta de grupo se cierra
// igual que la de la nota, ANTES de tocar la base. Si no, un grupo sin gente
// o con un miembro inventado acabaría en la tabla de reparto y nadie sabría
// por qué la nota no la veía nadie.
func TestLosVerbosDeGrupoValidanAntesDeEscribir(t *testing.T) {
	s, _ := hostDeAddon(t, carpetaMemoDelRepo(t))
	ses := sesionDeAddon{Usuario: "7", Empresa: defaultTenantCode, Sess: sessPrueba("master")}

	casos := []struct {
		nombre string
		verbo  string
		cuerpo map[string]interface{}
		que    string
	}{
		{"grupo sin nombre", "grupo.crear", map[string]interface{}{
			"miembros": []interface{}{"7"}}, "falta el campo nombre"},
		{"nombre de grupo larguísimo", "grupo.crear", map[string]interface{}{
			"nombre": strings.Repeat("x", memoMaxNombreGrupo+1), "miembros": []interface{}{"7"}}, "caracteres"},
		{"grupo sin miembros", "grupo.crear", map[string]interface{}{
			"nombre": "Logística"}, "sin miembros"},
		{"miembro que no es un id", "grupo.crear", map[string]interface{}{
			"nombre": "Logística", "miembros": []interface{}{"7'; DROP TABLE memo_grupos--"}}, "id de usuario válido"},
		{"miembro repetido", "grupo.crear", map[string]interface{}{
			"nombre": "Logística", "miembros": []interface{}{"7", "7"}}, "repetida"},
		{"más miembros de los que caben", "grupo.crear", map[string]interface{}{
			"nombre": "Todos", "miembros": usuariosDePrueba(memoMaxDestinatarios + 1)}, "no puede pasar de"},
		{"grupo.miembros con id que no es de Memo", "grupo.miembros", map[string]interface{}{
			"grupo_id": "g1", "miembros": []interface{}{"7"}}, "no es un id de grupo"},
	}
	for _, c := range casos {
		var err error
		switch c.verbo {
		case "grupo.crear":
			_, err = s.addonMemoGrupoCrear(nil, ses, c.cuerpo)
		case "grupo.miembros":
			_, err = s.addonMemoGrupoMiembros(nil, ses, c.cuerpo)
		default:
			t.Fatalf("el caso %q apunta a un verbo que no existe: %q", c.nombre, c.verbo)
		}
		if err == nil {
			t.Errorf("%s: debió rechazarse y pasó", c.nombre)
		} else if !strings.Contains(err.Error(), c.que) {
			t.Errorf("%s: el motivo debía mencionar %q, dio %q", c.nombre, c.que, err.Error())
		}
	}
}

// usuariosDePrueba arma ids con la forma de users.user_id (hasta 15 caras).
func usuariosDePrueba(n int) []interface{} {
	fuera := []interface{}{}
	for i := 1; i <= n; i++ {
		fuera = append(fuera, fmt.Sprintf("u%04d", i))
	}
	return fuera
}

// TestElVerboDelModuloMemoEscribePorElOrquestador: la escritura feliz pasa por
// s.Escribir (journal + audit_log + idempotencia), y los destinatarios
// comparten la nota según la visibilidad elegida.
func TestElVerboDelModuloMemoEscribePorElOrquestador(t *testing.T) {
	s, h := hostDeAddon(t, carpetaMemoDelRepo(t))
	s.config.DBSQLitePath = filepath.Join(t.TempDir(), "empresas", "prueba.sqlite3")
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	// Empresa maestra propia de esta prueba: openTenantDB resuelve la base de
	// control (la de la prueba) y el orquestador queda en su mapa con esta
	// clave, sin pisar el de ninguna otra prueba.
	const empresa = "memo_orquestador"
	s.config.DBMasterTenantCode = empresa
	ses := sesionDeAddon{Usuario: "7", Empresa: empresa, Sess: sessPrueba("master")}

	// a) Compartida con personas: nota + una fila de destinatario.
	res, err := s.addonMemoNotaCrear(nil, ses, map[string]interface{}{
		"titulo": "Cierre de caja", "texto": "Hoy a las 5.", "visibilidad": "personas",
		"destinatarios": []interface{}{"7", "9"},
	})
	if err != nil {
		t.Fatalf("la escritura válida debía pasar por el orquestador: %v", err)
	}
	id, _ := res["registro_id"].(string)
	if !strings.HasPrefix(id, "memo-") {
		t.Fatalf("el registro_id debe llevar el prefijo del módulo: %q", id)
	}
	if res["compartido"] != 2 {
		t.Fatalf("debía reportar 2 personas compartidas, reportó %v", res["compartido"])
	}

	var visibilidad, autor, empresaGuardada string
	if err := s.db.QueryRow(`SELECT visibilidad, autor, empresa FROM memo_notas WHERE id = ?`, id).
		Scan(&visibilidad, &autor, &empresaGuardada); err != nil {
		t.Fatalf("la nota no quedó en su tabla: %v", err)
	}
	if visibilidad != "personas" || empresaGuardada != empresa {
		t.Fatalf("la nota quedó mal (visibilidad=%q empresa=%q)", visibilidad, empresaGuardada)
	}
	// EL AUTOR se guarda como X-ERP-User (users.user_id, "7"), que es el id
	// con el que el muro compara `n.autor` y con el que se pinta el nombre.
	// Guardar el nombre de login ("admin") dejaba al autor sin ver sus
	// propias notas: dos espacios de id que no se cruzaban.
	if autor != "7" {
		t.Fatalf("el autor debe ser el X-ERP-User de la sesión (\"7\"), quedó %q", autor)
	}
	var destinatarios int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_destinatarios WHERE nota_id = ?`, id).
		Scan(&destinatarios); err != nil {
		t.Fatalf("no se pudo contar los destinatarios: %v", err)
	}
	if destinatarios != 2 {
		t.Fatalf("la nota debía tener 2 destinatarios, tiene %d", destinatarios)
	}

	// b) Privada: los destinatarios que vinieran se IGNORAN, porque una nota
	// privada no se reparte con nadie. La tabla no debe mentir.
	res2, err := s.addonMemoNotaCrear(nil, ses, map[string]interface{}{
		"titulo": "Nota privada", "texto": "Solo mía.", "visibilidad": "privada",
		"destinatarios": []interface{}{"9"},
	})
	if err != nil {
		t.Fatalf("la nota privada debía guardarse: %v", err)
	}
	if res2["compartido"] != 0 {
		t.Fatalf("una nota privada no se comparte con nadie, reportó %v", res2["compartido"])
	}
	id2, _ := res2["registro_id"].(string)
	var colgados int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_destinatarios WHERE nota_id = ?`, id2).Scan(&colgados); err != nil {
		t.Fatal(err)
	}
	if colgados != 0 {
		t.Fatalf("una nota privada dejó %d destinatarios colgados", colgados)
	}
}

// TestElRepartoPorGrupoEscribeAvisaYSeResuelveAlLeer: la fase B entera en una
// prueba —crear el grupo, repartir una nota por él, avisar en la bandeja,
// cambiar los miembros— y su cierre: un grupo vacío no reparte nada y se
// niega ANTES de escribir.
func TestElRepartoPorGrupoEscribeAvisaYSeResuelveAlLeer(t *testing.T) {
	s, h := hostDeAddon(t, carpetaMemoDelRepo(t))
	s.config.DBSQLitePath = filepath.Join(t.TempDir(), "empresas", "prueba.sqlite3")
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}
	// Empresa maestra propia de esta prueba, como en las demás: el
	// orquestador y tenantDB resuelven la base de control sin pisar a nadie.
	const empresa = "memo_grupos_prueba"
	s.config.DBMasterTenantCode = empresa
	ses := sesionDeAddon{Usuario: "7", Empresa: empresa, Sess: sessPrueba("master")}

	// a) El grupo se crea con sus dos miembros.
	creado, err := s.addonMemoGrupoCrear(nil, ses, map[string]interface{}{
		"nombre": "Logística", "miembros": []interface{}{"7", "9"},
	})
	if err != nil {
		t.Fatalf("el grupo válido debía crearse: %v", err)
	}
	grupoID, _ := creado["registro_id"].(string)
	if !strings.HasPrefix(grupoID, "memo-grp-") {
		t.Fatalf("el id del grupo debe llevar su prefijo: %q", grupoID)
	}
	if creado["miembros"] != 2 {
		t.Fatalf("debía reportar 2 miembros, reportó %v", creado["miembros"])
	}
	var nombreGrupo, empresaGrupo string
	if err := s.db.QueryRow(`SELECT nombre, empresa FROM memo_grupos WHERE id = ?`, grupoID).
		Scan(&nombreGrupo, &empresaGrupo); err != nil {
		t.Fatalf("el grupo no quedó en su tabla: %v", err)
	}
	if nombreGrupo != "Logística" || empresaGrupo != empresa {
		t.Fatalf("el grupo quedó mal (nombre=%q empresa=%q)", nombreGrupo, empresaGrupo)
	}
	var miembros int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_grupo_miembros WHERE grupo_id = ? AND empresa = ?`,
		grupoID, empresa).Scan(&miembros); err != nil {
		t.Fatalf("no se pudieron leer los miembros: %v", err)
	}
	if miembros != 2 {
		t.Fatalf("el grupo debía tener 2 miembros, tiene %d", miembros)
	}

	// b) La nota se reparte COMO GRUPO: una sola fila de destinatario con
	//    destino_tipo='grupo' (no dos personas copiadas), y un aviso por
	//    miembro en la bandeja. El autor ("7") es miembro y no se avisa a
	//    sí mismo, así que queda UN aviso, para "9".
	res, err := s.addonMemoNotaCrear(nil, ses, map[string]interface{}{
		"titulo": "Ruta nueva", "texto": "Sale a las 6 de la mañana.",
		"visibilidad": "grupo", "grupos": []interface{}{grupoID},
	})
	if err != nil {
		t.Fatalf("la nota de grupo debía publicarse: %v", err)
	}
	notaID, _ := res["registro_id"].(string)
	var tipo, destino string
	if err := s.db.QueryRow(`SELECT destino_tipo, destino_id FROM memo_destinatarios WHERE nota_id = ?`, notaID).
		Scan(&tipo, &destino); err != nil {
		t.Fatalf("la nota no quedó repartida: %v", err)
	}
	if tipo != "grupo" || destino != grupoID {
		t.Fatalf("el reparto debe apuntar AL GRUPO (tipo=%q destino=%q, grupo=%q)", tipo, destino, grupoID)
	}
	if res["compartido"] != 1 {
		t.Fatalf("debía reportar 1 destino (el grupo), reportó %v", res["compartido"])
	}
	if res["avisados"] != 1 {
		t.Fatalf("debía avisar a 1 persona (los dos miembros menos el autor), reportó %v", res["avisados"])
	}
	var para, ref string
	var leida int
	if err := s.db.QueryRow(`SELECT para, ref_id, leida FROM memo_inbox WHERE empresa = ?`, empresa).
		Scan(&para, &ref, &leida); err != nil {
		t.Fatalf("la bandeja no recibió el aviso: %v", err)
	}
	if para != "9" || ref != notaID || leida != 0 {
		t.Fatalf("el aviso quedó mal (para=%q ref=%q leida=%d)", para, ref, leida)
	}

	// c) Cambiar los miembros: entra 11 y sale 9. La fila de 9 se BORRA —la
	//    pertenencia es estado actual—, y por eso la próxima lectura ya no
	//    le muestra nada de este grupo.
	cambiado, err := s.addonMemoGrupoMiembros(nil, ses, map[string]interface{}{
		"grupo_id": grupoID, "miembros": []interface{}{"7", "11"},
	})
	if err != nil {
		t.Fatalf("cambiar los miembros debía funcionar: %v", err)
	}
	if cambiado["entraron"] != 1 || cambiado["bajaron"] != 1 {
		t.Fatalf("debía entrar 1 y salir 1, reportó entraron=%v bajaron=%v",
			cambiado["entraron"], cambiado["bajaron"])
	}
	var quedan, fuera int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_grupo_miembros WHERE grupo_id = ? AND empresa = ?`,
		grupoID, empresa).Scan(&quedan); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_grupo_miembros WHERE grupo_id = ? AND persona_id = ?`,
		grupoID, "9").Scan(&fuera); err != nil {
		t.Fatal(err)
	}
	if quedan != 2 || fuera != 0 {
		t.Fatalf("tras el cambio debían quedar 2 miembros sin el 9, quedan %d y sale %d", quedan, fuera)
	}

	// d) Un grupo sin gente no reparte nada: se niega ANTES de escribir, con
	//    su motivo, y no deja media nota colgada.
	if _, err := s.addonMemoGrupoMiembros(nil, ses, map[string]interface{}{
		"grupo_id": grupoID, "miembros": []interface{}{},
	}); err != nil {
		t.Fatalf("vaciar el grupo debía poderse: %v", err)
	}
	var notasAntes int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_notas WHERE empresa = ?`, empresa).Scan(&notasAntes); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addonMemoNotaCrear(nil, ses, map[string]interface{}{
		"titulo": "Nadie la verá", "texto": "x", "visibilidad": "grupo",
		"grupos": []interface{}{grupoID},
	}); err == nil || !strings.Contains(err.Error(), "no tiene miembros") {
		t.Fatalf("publicar por un grupo vacío debía negarse con su motivo, dio: %v", err)
	}
	var notasDespues int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memo_notas WHERE empresa = ?`, empresa).Scan(&notasDespues); err != nil {
		t.Fatal(err)
	}
	if notasDespues != notasAntes {
		t.Fatalf("la nota se rechazó pero se escribió igual: %d notas antes, %d después", notasAntes, notasDespues)
	}
}
