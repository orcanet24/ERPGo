package web

// Pruebas del acceso a la pantalla de módulos, que era el eslabón que faltaba:
// sin ella no se puede encender mensajería.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLaPantallaDeModulosCreaLaFilaDeMensajeriaSiFalta(t *testing.T) {
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE modules (id TEXT PRIMARY KEY, name TEXT, version TEXT, description TEXT,
		is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 0, path TEXT, author TEXT, license_type TEXT)`); err != nil {
		t.Fatal(err)
	}
	r := reqPrueba(s, sessPrueba("master"), "/module/list")

	s.asegurarModuloMensajeria(r)
	s.asegurarModuloMensajeria(r) // dos veces: no debe duplicar

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modules WHERE id='mensajeria'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("filas de mensajería = %d; debe ser 1", n)
	}
	var activo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id='mensajeria'`).Scan(&activo); err != nil {
		t.Fatal(err)
	}
	if activo != 0 {
		t.Fatal("la fila recreada no puede nacer encendida")
	}
}

func TestMensajeriaNoSePuedeBorrarSoloApagar(t *testing.T) {
	s, db := basePrueba(t)
	if _, err := db.Exec(`CREATE TABLE modules (id TEXT PRIMARY KEY, name TEXT, is_core INTEGER DEFAULT 0, is_active INTEGER DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO modules (id, name, is_core, is_active) VALUES ('mensajeria','Mensajería',0,1)`); err != nil {
		t.Fatal(err)
	}
	req := reqPrueba(s, sessPrueba("master"), "/module/mensajeria")
	req.Method = http.MethodDelete
	req.SetPathValue("id", "mensajeria")
	res := httptest.NewRecorder()
	s.moduleDelete(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("borrar mensajería debía dar 403; dio %d", res.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM modules WHERE id='mensajeria'`).Scan(&n)
	if n != 1 {
		t.Fatal("la fila de mensajería se borró")
	}
}

func TestElMasterVePorQueElModuloEstaApagado(t *testing.T) {
	s, _ := baseMensajeria(t)
	r := reqPrueba(s, sessPrueba("master"), "/mensajeria")
	res := httptest.NewRecorder()
	s.mensajeriaHome(res, r)

	if res.Code != http.StatusNotFound {
		t.Fatalf("el módulo apagado debía dar 404; dio %d", res.Code)
	}
	cuerpo := res.Body.String()
	if !strings.Contains(cuerpo, "Módulos") || !strings.Contains(cuerpo, "/settings/mensajeria") {
		t.Fatalf("el 404 del master no explica cómo encenderlo: %s", cuerpo)
	}
}

func TestLaMarcaDeModulosSoloLaTieneElMaster(t *testing.T) {
	s, _ := basePrueba(t)

	master := s.permissionMap(reqPrueba(s, sessPrueba("master"), "/home"), sessPrueba("master"))
	if !master["__master"] {
		t.Fatal("el master debe ver la entrada de Módulos")
	}
	// La caché de permisos se indexa por empresa+usuario; para probar otro nivel
	// con el mismo id de prueba hay que vaciarla.
	limpiarCachesDeRender()
	admin := s.permissionMap(reqPrueba(s, sessPrueba("admin"), "/home"), sessPrueba("admin"))
	if admin["__master"] {
		t.Fatal("el admin de empresa no debe ver la administración de módulos")
	}
}
