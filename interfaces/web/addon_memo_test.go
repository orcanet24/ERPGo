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
	if len(p.migraciones) != 2 {
		t.Fatalf("la fase A declara dos tablas y su schema.sql debía dar dos sentencias, dio %d", len(p.migraciones))
	}
	if len(p.manifiesto.DBWrite) != 1 || p.manifiesto.DBWrite[0].Verb != "nota.crear" {
		t.Fatalf("la fase A declara el verbo nota.crear y nada más: %v", p.manifiesto.DBWrite)
	}
	if p.manifiesto.DBWrite[0].Perm != "memo_publicar" {
		t.Fatalf("nota.crear debe exigir memo_publicar, exige %q", p.manifiesto.DBWrite[0].Perm)
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
	}
	for _, c := range casos {
		if _, err := s.addonMemoNotaCrear(nil, ses, c.cuerpo); err == nil {
			t.Errorf("%s: debió rechazarse y pasó", c.nombre)
		} else if !strings.Contains(err.Error(), c.que) {
			t.Errorf("%s: el motivo debía mencionar %q, dio %q", c.nombre, c.que, err.Error())
		}
	}
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
	if autor != "admin" {
		t.Fatalf("el autor debe salir de la sesión validada (admin), quedó %q", autor)
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
