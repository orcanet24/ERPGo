package web

// Pruebas del acceso a la pantalla de módulos y de la marca de master en el menú.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo/configs"
)

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

// TestModuleListDataNoCrasheaDataTableSinBase: un DataTables que recibe null
// en "data" cae en "Cannot read properties of null (reading 'length')". La
// corrección: si la base no está disponible, devolver {"data":[]} en vez de
// un error JSON que el parser frontend no espera.
func TestModuleListDataNoCrasheaDataTableSinBase(t *testing.T) {
	s, _ := basePrueba(t)

	// Sin scope: dbFor devuelve s.db (que está seteada). Para forzar el nil,
	// usamos un request sin scope sobre un Server sin db.
	sinDB := &Server{config: &configs.Config{AppEnv: "test"}}
	req := httptest.NewRequest(http.MethodGet, "/module/list/data", nil)
	res := httptest.NewRecorder()
	sinDB.moduleListData(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("esperaba 200 cuando la base es nil (no 500); llegó %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"data":[]`) {
		t.Errorf("debe devolver data como array vacío [] para no matar al DataTable; salió: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "null") {
		t.Errorf("no debe devolver null en data; salió: %s", res.Body.String())
	}

	_ = s
}
