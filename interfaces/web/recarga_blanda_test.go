package web

// Pruebas de la recarga blanda: guardar sin que la vista entera se vuelva a
// cargar (static/js/guardado.js).
//
// POR QUÉ SE PRUEBA UNA PLANTILLA Y UN ARCHIVO JAVASCRIPT AQUÍ. El fallo que
// cubren no es de sintaxis: es de ENCUADRE. guardado.js refresca la vista
// pidiendo la página de nuevo, pegando el HTML dentro de .content y volviendo
// a ejecutar los scripts del bloque #pagejs-host. Si alguien quita ese
// contenedor del layout, la operación sigue «funcionando» —el contenido se
// pinta— pero los manejadores de la página no vuelven a engancharse: los
// botones quedan mudos y nada se pone rojo. Es un fallo silencioso, que es
// exactamente el tipo de error que un test debe impedir.
//
// La segunda prueba fija la decisión de producto: ninguna vista recarga la
// página entera tras guardar. Es lo que hace que el ERP parezca rápido; si una
// pantalla nueva vuelve a location.reload(), el síntoma completo de antes —el
// «Espere...», el menú redibujado, el punto de scroll perdido— vuelve también.

import (
	"io/fs"
	"strings"
	"testing"

	"erpgo"
)

func TestElLayoutEnvuelveElPagejsDondeLaRecargaLoReejecuta(t *testing.T) {
	layout, err := fs.ReadFile(erpgo.Templates(), "templates/layout.html")
	if err != nil {
		t.Fatalf("no se pudo leer layout.html: %v", err)
	}
	ly := string(layout)

	if !strings.Contains(ly, `<div id="pagejs-host">{{template "pagejs" .}}</div>`) {
		t.Error("layout.html ya no envuelve {{template \"pagejs\"}} en #pagejs-host: " +
			"guardado.js sustituye ese bloque y lo vuelve a ejecutar al refrescar, " +
			"y sin el contenedor la vista quedaría con el HTML nuevo y los " +
			"manejadores viejos (botones mudos, sin error a la vista).")
	}
	if !strings.Contains(ly, `/static/js/guardado.js`) {
		t.Error("layout.html no carga /static/js/guardado.js: sin ese archivo " +
			"ERPGo.refrescar no existe y toda vista que lo llame fallaría en consola.")
	}
	// El script va en footerjs y por tanto ANTES de pagejs: pagejs llama a
	// ERPGo.refrescar en el momento de definirse, no después.
	if strings.Index(ly, `static/js/guardado.js`) > strings.Index(ly, `<div id="pagejs-host">`) {
		t.Error("guardado.js se carga después de #pagejs-host: los scripts de la " +
			"página pueden llamar a ERPGo.refrescar antes de que exista.")
	}
}

func TestGuardadoJsPublicaLoQueLasVistasLlaman(t *testing.T) {
	b, err := fs.ReadFile(erpgo.Static(), "static/js/guardado.js")
	if err != nil {
		t.Fatalf("no existe static/js/guardado.js: %v", err)
	}
	js := string(b)

	// Lo que las vistas usan, lo que la seguridad exige y la red de seguridad.
	esperados := []string{
		"ERPGo.refrescar",                   // el punto de entrada que llaman las vistas
		"X-CSRF-Token",                      // el token viaja igual que antes, por cabecera
		"application/x-www-form-urlencoded", // el cuerpo idéntico al de un envío real
		"e.isDefaultPrevented()",            // no toca los 79 formularios que ya usan AJAX
		"location.reload()",                 // ante cualquier fallo, lo de siempre
	}
	for _, s := range esperados {
		if !strings.Contains(js, s) {
			t.Errorf("guardado.js ya no contiene %q", s)
		}
	}
}

// recargaPermitida marca, en la propia plantilla, que esa pantalla sí debe
// recargar la página entera. Se lee como una explicación y como una salida: el
// test propone refrescar(), y la marca existe para que quien tenga un motivo
// real no tenga que pelearse con la prueba.
const recargaPermitida = "{{/* recarga completa permitida */}}"

func TestNingunaVistaRecargaLaPaginaEnteraTrasGuardar(t *testing.T) {
	entradas, err := fs.ReadDir(erpgo.Templates(), "templates")
	if err != nil {
		t.Fatalf("no se pudo leer templates/: %v", err)
	}

	var malas []string
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := fs.ReadFile(erpgo.Templates(), "templates/"+e.Name())
		if err != nil {
			continue
		}
		texto := string(b)
		if strings.Contains(texto, recargaPermitida) {
			continue
		}
		if strings.Contains(texto, "location.reload()") {
			malas = append(malas, e.Name())
		}
	}

	if len(malas) > 0 {
		t.Errorf("estas pantallas recargan la página entera tras guardar, que es "+
			"lo que hace que se note el «Espere...» y se pierda el punto de scroll. "+
			"Use ERPGo.refrescar() (recarga solo el contenido) o marque la plantilla "+
			"con %q si de verdad hace falta: %s",
			recargaPermitida, strings.Join(malas, ", "))
	}
}
