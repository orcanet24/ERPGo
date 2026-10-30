package web

// Pruebas del menú cuando la estructura y el estado del módulo viven en bases
// distintas: `menu` en control y `modules.is_active` en la empresa. Ese era el
// fallo real: el interruptor cambiaba la fila de la empresa y el menú seguía
// leyendo la de control, así que el módulo se activaba y no aparecía.

import (
	"net/http/httptest"
	"testing"
)

func TestElMenuApareceAlActivarElModuloEnLaBaseDeLaEmpresa(t *testing.T) {
	limpiarCachesDeRender()

	// Empresa: solo `modules`.
	empresa := abrirBasePrueba(t)
	if _, err := empresa.Exec(`CREATE TABLE modules (id TEXT PRIMARY KEY, is_active INTEGER DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := empresa.Exec(`INSERT INTO modules (id, is_active) VALUES ('mensajeria', 0)`); err != nil {
		t.Fatal(err)
	}

	// Control: la estructura del menú.
	control := abrirBasePrueba(t)
	ddl := []string{
		`ALTER TABLE menu ADD COLUMN module_id TEXT DEFAULT 'core'`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (1, NULL, 'Dashboard', '/home', NULL, NULL, NULL, 10, 1, 'core')`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (2, NULL, 'Mensajería', NULL, NULL, NULL, NULL, 20, 1, 'mensajeria')`,
		`INSERT INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id)
		 VALUES (3, 2, 'Bandeja', '/mensajeria', NULL, NULL, '/mensajeria', 10, 1, 'mensajeria')`,
	}
	for _, q := range ddl {
		if _, err := control.Exec(q); err != nil {
			t.Fatalf("preparar control: %v\n%s", err, q)
		}
	}

	s := &Server{db: empresa, controlDB: control, controlListo: true}
	r := reqPrueba(s, sessPrueba("master"), "/home")

	if menu := s.menuTree(r, map[string]bool{}, "/home"); len(menu) != 1 {
		t.Fatalf("con el módulo apagado solo debía verse Dashboard; se vio: %+v", menu)
	}

	// El interruptor escribe en la EMPRESA.
	if _, err := empresa.Exec(`UPDATE modules SET is_active = 1 WHERE id='mensajeria'`); err != nil {
		t.Fatal(err)
	}
	invalidarCacheMenu()

	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 2 || menu[1].Label != "Mensajería" {
		t.Fatalf("al activar el módulo debía aparecer Mensajería; se vio: %+v", menu)
	}
	if len(menu[1].Children) != 1 || menu[1].Children[0].URL != "/mensajeria" {
		t.Fatalf("el grupo no trajo la bandeja: %+v", menu[1])
	}
}

func TestUnModuloSinFilaNoPintaSuMenu(t *testing.T) {
	// Si la migración no dejó la fila, el menú del módulo no se anuncia: una
	// entrada que lleva a un 404 es peor que no tener la entrada.
	limpiarCachesDeRender()
	s, db := basePrueba(t)
	ddl := []string{
		`ALTER TABLE menu ADD COLUMN module_id TEXT DEFAULT 'core'`,
		`CREATE TABLE modules (id TEXT PRIMARY KEY, is_active INTEGER DEFAULT 1)`,
		`INSERT INTO menu (id, parent_id, label, url, sort_order, status, module_id)
		 VALUES (1, NULL, 'Dashboard', '/home', 10, 1, 'core')`,
		`INSERT INTO menu (id, parent_id, label, url, sort_order, status, module_id)
		 VALUES (2, NULL, 'Mensajería', '/mensajeria', 20, 1, 'mensajeria')`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("preparar: %v\n%s", err, q)
		}
	}
	r := reqPrueba(s, sessPrueba("master"), "/home")
	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 1 || menu[0].Label != "Dashboard" {
		t.Fatalf("un módulo sin fila no debía pintarse: %+v", menu)
	}
}

func TestElMenuSeRevalidaTrasElInterruptor(t *testing.T) {
	// El toggle del handler debe invalidar la caché; sin eso, el menú tarda
	// hasta 30 s en reflejar el cambio y parece que el botón no hizo nada.
	limpiarCachesDeRender()
	s, _ := baseDeMenuConModulo(t)
	r := reqPrueba(s, sessPrueba("master"), "/home")
	if menu := s.menuTree(r, map[string]bool{}, "/home"); len(menu) != 1 {
		t.Fatalf("estado inicial inesperado: %+v", menu)
	}
	req := reqPrueba(s, sessPrueba("master"), "/module/mensajeria/toggle")
	req.Method = "POST"
	req.SetPathValue("id", "mensajeria")
	res := httptest.NewRecorder()
	s.moduleToggle(res, req)
	if res.Code != 200 {
		t.Fatalf("toggle: %d %s", res.Code, res.Body.String())
	}
	menu := s.menuTree(r, map[string]bool{}, "/home")
	if len(menu) != 2 {
		t.Fatalf("el menú no se refrescó tras el toggle: %+v", menu)
	}
}
