package web

// Pruebas del acceso a la pantalla de módulos y de la marca de master en el menú.

import (
	"testing"
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
