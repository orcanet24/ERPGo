package web

// addon_demo_test.go — Pruebas del módulo demo (addons/demo).
//
// El módulo demo no es un adorno: es la PLANTILLA que se copia para crear un
// módulo nuevo, así que sus pruebas protegen el recorrido completo que todo
// módulo va a repetir:
//
//   - El candado: el manifiesto y el schema.sql REALES del repo pasan el
//     escáner tal cual lo corre el arranque (no una copia a mano en el test).
//   - El registro: modules/menu en la base de la empresa y su tabla propia
//     creada por la migración declarada.
//   - El verbo: valida antes de escribir (título y mensaje obligatorios y
//     dentro de los límites de la columna) y la escritura feliz BAJA POR EL
//     ORQUESTADOR, con su clave de idempotencia, no por un INSERT suelto.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// carpetaDemoDelRepo copia el bundle real (manifest.json + schema.sql) de
// addons/demo a una carpeta addons temporal. Se copian los del repo —no se
// reimplementan aquí— porque lo que hay que probar es justamente ese bundle.
func carpetaDemoDelRepo(t *testing.T) string {
	t.Helper()
	raiz := t.TempDir()
	dir := filepath.Join(raiz, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nombre := range []string{"manifest.json", "schema.sql"} {
		origen := filepath.Join("..", "..", "addons", "demo", nombre)
		contenido, err := os.ReadFile(origen)
		if err != nil {
			t.Fatalf("el módulo demo del repo no trae %s: %v", nombre, err)
		}
		if err := os.WriteFile(filepath.Join(dir, nombre), contenido, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return raiz
}

// TestElManifiestoDelModuloDemoDelRepoPasaElCandado corre el escáner REAL
// sobre el bundle del repo: si mañana alguien edita addons/demo y deja un
// permiso sin declarar, una tabla sin prefijo o un DDL que no cuadra con
// tables_own, la prueba se entera antes que el arranque.
func TestElManifiestoDelModuloDemoDelRepoPasaElCandado(t *testing.T) {
	_, h := hostDeAddon(t, carpetaDemoDelRepo(t))
	fila, avisos := h.escanear()
	if len(avisos) > 0 {
		t.Fatalf("el bundle del módulo demo no pasó el escáner: %v", avisos)
	}
	p := fila["demo"]
	if p == nil {
		t.Fatal("el módulo demo no cargó con su manifiesto del repo")
	}
	if len(p.migraciones) != 1 {
		t.Fatalf("declaraba una tabla propia y su schema.sql debía dar una sentencia, dio %d", len(p.migraciones))
	}
	if !strings.Contains(strings.ToUpper(p.migraciones[0]), "CREATE TABLE IF NOT EXISTS DEMO_AVISOS") {
		t.Fatalf("la migración no es la creación de su tabla: %s", p.migraciones[0])
	}
	// Y el manifiesto es el que manda: sin db_read no hay lectura, sin
	// db_write no hay canal de escritura.
	if len(p.manifiesto.DBRead) == 0 || len(p.manifiesto.DBWrite) == 0 {
		t.Fatalf("el demo promete la fase 2 entera y su manifiesto no la declara: read=%v write=%v",
			p.manifiesto.DBRead, p.manifiesto.DBWrite)
	}
	// El permiso del verbo tiene que estar declarado: si no, el candado lo
	// habría rechazado ya, pero se comprueba la intención del bundle.
	if p.manifiesto.DBWrite[0].Perm != "demo_publicar" {
		t.Fatalf("el verbo aviso.publicar debe exigir demo_publicar, exige %q", p.manifiesto.DBWrite[0].Perm)
	}
}

// TestElModuloDemoSeRegistraEnModulosYMenuYSuCreaSuTabla: el escaneo refleja
// el manifiesto en las filas que el ERP ya usa para cualquier módulo (las
// mismas que apaga el toggle de master) y aplica su migración en la base de
// la empresa activa.
func TestElModuloDemoSeRegistraEnModulosYMenuYSuCreaSuTabla(t *testing.T) {
	s, h := hostDeAddon(t, carpetaDemoDelRepo(t))
	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}

	h.mu.Lock()
	cargado := h.progs["demo"] != nil
	h.mu.Unlock()
	if !cargado {
		t.Fatal("el módulo demo no quedó cargado tras el escaneo")
	}

	var activo int
	if err := s.db.QueryRow(`SELECT is_active FROM modules WHERE id = 'demo'`).Scan(&activo); err != nil {
		t.Fatalf("el módulo demo no dejó fila en modules: %v", err)
	}
	if activo != 1 {
		t.Fatal("el módulo demo debía quedar activo en modules")
	}

	var url, perm string
	if err := s.db.QueryRow(`SELECT url, perm FROM menu WHERE module_id = 'demo'`).Scan(&url, &perm); err != nil {
		t.Fatalf("el módulo demo no dejó entrada de menú: %v", err)
	}
	if url != "/mod/demo" || perm != "demo_ver" {
		t.Fatalf("el menú del demo quedó raro (url=%q perm=%q): debe apuntar a la puerta estable /mod/demo", url, perm)
	}

	var filas int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM demo_avisos`).Scan(&filas); err != nil {
		t.Fatalf("la migración de su tabla propia no se aplicó: %v", err)
	}
}

// TestElVerboDelModuloDemoValidaAntesDeEscribir: el candado del canal se
// dobla en el handler. Un cuerpo sin título o demasiado largo se rechaza ANTES
// de tocar al orquestador, con una razón que el operador pueda leer.
func TestElVerboDelModuloDemoValidaAntesDeEscribir(t *testing.T) {
	s, h := hostDeAddon(t, carpetaDemoDelRepo(t))
	_ = h
	ses := sesionDeAddon{Usuario: "7", Empresa: defaultTenantCode, Sess: sessPrueba("master")}

	casos := []struct {
		nombre string
		cuerpo map[string]interface{}
		que    string
	}{
		{"sin título", map[string]interface{}{"mensaje": "hola"}, "titulo"},
		{"sin mensaje", map[string]interface{}{"titulo": "Reunión"}, "mensaje"},
		{"título vacío", map[string]interface{}{"titulo": "   ", "mensaje": "hola"}, "titulo"},
		{"título larguísimo", map[string]interface{}{
			"titulo": strings.Repeat("x", addonDemoMaxTitulo+1), "mensaje": "hola"}, "caracteres"},
		{"aviso gigante", map[string]interface{}{
			"titulo": "corto", "mensaje": strings.Repeat("y", addonDemoMaxMensaje+1)}, "caracteres"},
	}
	for _, c := range casos {
		if _, err := s.addonDemoAvisoPublicar(nil, ses, c.cuerpo); err == nil {
			t.Errorf("%s: debió rechazarse y pasó", c.nombre)
		} else if !strings.Contains(err.Error(), c.que) {
			t.Errorf("%s: el motivo debía mencionar %q, dio %q", c.nombre, c.que, err.Error())
		}
	}
}

// TestElVerboDelModuloDemoEscribePorElOrquestador es la escritura feliz: pasa
// por s.Escribir (journal + audit_log + idempotencia), no por un INSERT del
// handler. Si alguien «arregla» este handler metiendo SQL directo, esta
// prueba es la que se entera.
func TestElVerboDelModuloDemoEscribePorElOrquestador(t *testing.T) {
	s, h := hostDeAddon(t, carpetaDemoDelRepo(t))
	// El journal del orquestador es un archivo junto a los datos de la empresa:
	// aquí se le da una ruta temporal para que las pruebas no dejen colas en
	// el repo.
	s.config.DBSQLitePath = filepath.Join(t.TempDir(), "empresas", "prueba.sqlite3")

	if err := h.reescanear(); err != nil {
		t.Fatal(err)
	}

	// Empresa de prueba MAESTRA y propia de este test: openTenantDB resuelve
	// la base de control (la de la prueba) y el orquestador queda registrado
	// en el mapa con esta clave, sin pisar el de ninguna otra prueba.
	const empresa = "demo_orquestador"
	s.config.DBMasterTenantCode = empresa

	ses := sesionDeAddon{Usuario: "7", Empresa: empresa, Sess: sessPrueba("master")}
	res, err := s.addonDemoAvisoPublicar(nil, ses, map[string]interface{}{
		"titulo": "Reunión de logística", "mensaje": "Mañana a las 9 en la sala 2.",
	})
	if err != nil {
		t.Fatalf("la escritura válida debía pasar por el orquestador: %v", err)
	}
	id, _ := res["registro_id"].(string)
	if !strings.HasPrefix(id, "demo-") {
		t.Fatalf("el registro_id debe llevar el prefijo de su módulo: %q", id)
	}
	correlativo, ok := res["correlativo"].(uint64)
	if !ok || correlativo == 0 {
		t.Fatalf("la respuesta debe traer el correlativo del orquestador: %v", res)
	}

	var titulo, autor, empresaGuardada string
	err = s.db.QueryRow(`SELECT titulo, autor, empresa FROM demo_avisos WHERE id = ?`, id).
		Scan(&titulo, &autor, &empresaGuardada)
	if err != nil {
		t.Fatalf("el aviso no quedó en su tabla: %v", err)
	}
	if titulo != "Reunión de logística" {
		t.Fatalf("el título guardado no coincide: %q", titulo)
	}
	if empresaGuardada != empresa {
		t.Fatalf("el aviso se guardó en la empresa %q y debía ser %q (aislamiento por empresa)", empresaGuardada, empresa)
	}
	if autor != "admin" {
		t.Fatalf("el autor debía salir de la sesión validada (admin), quedó %q", autor)
	}
}
