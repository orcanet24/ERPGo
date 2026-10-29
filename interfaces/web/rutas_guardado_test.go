package web

// rutas_guardado_test.go — Cada «Guardar y Salir» debe aterrizar en una ruta
// que EXISTE.
//
// POR QUÉ ESTA PRUEBA EXISTE.
//
// Los handlers devuelven al navegador {"redirect": saveRedirect(r, "/módulo")}
// y guardado.js navega a esa URL. Con el tiempo los destinos quedaron cargados
// de rutas estilo PHP desfasadas («/setting», «/account/list», «/ai_tasks»…);
// el mux no encontraba ninguna y el fallback mandaba al usuario a la portada
// después de guardar: el dato estaba bien guardado pero el clic «no hacía
// nada». Con once destinos nos costó ver que eran TREINTA.
//
// La regla que aquí se defiende es estructural y no admite lista de
// excepciones: TODO literal que pasemos a saveRedirect tiene que estar
// registrado EXACTO en el mux. Las capturas se leen del código fuente del
// propio paquete (la carpeta de la prueba), así que la prueba no depende de
// montar un servidor completo ni de que nadie la actualice a mano: el día que
// alguien escriba un destino viejo, roja, y el día que mueva una pantalla de
// ruta, también roja hasta que revisen si los guardados deben seguir el cambio.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// reRutaRegistrada captura el primer argumento de cada mux.HandleFunc("...").
// Solo patrones literales: una ruta montada con texto dinámico no cuenta en el
// censo, y para destinos de guardado eso equivaldría a "no existe".
var reRutaRegistrada = regexp.MustCompile(`HandleFunc\("([^"]+)"`)

// reDestinoGuardado captura el literal de cada saveRedirect(x, "/…"). "x" puede
// variar de nombre entre handlers, por eso se acepta cualquier identificador.
var reDestinoGuardado = regexp.MustCompile(`saveRedirect\(\s*\w+\s*,\s*"([^"]+)"`)

// TestTodoSaveRedirectApuntaAUnaRutaRegistrada compara los destinos de los
// guardados contra el censo de rutas registradas en los *.go del paquete.
func TestTodoSaveRedirectApuntaAUnaRutaRegistrada(t *testing.T) {
	entradas, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("no se pudo leer el directorio del paquete: %v", err)
	}

	registradas := map[string]bool{}
	destinos := map[string][]string{} // destino de guardado -> archivos donde se usa

	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasSuffix(nombre, ".go") || strings.HasSuffix(nombre, "_test.go") {
			continue // solo código de producción: censo neto
		}
		texto, err := os.ReadFile(nombre)
		if err != nil {
			t.Fatalf("no se pudo leer %s: %v", nombre, err)
		}
		fuente := string(texto)
		for _, m := range reRutaRegistrada.FindAllStringSubmatch(fuente, -1) {
			// El patrón es "METHOD /ruta" o "/ruta" a secas (sin método). El
			// path es lo único que importa para aterrizar un guardado.
			campos := strings.Fields(m[1])
			registradas[campos[len(campos)-1]] = true
		}
		for _, m := range reDestinoGuardado.FindAllStringSubmatch(fuente, -1) {
			destinos[m[1]] = append(destinos[m[1]], nombre)
		}
	}

	if len(destinos) == 0 {
		t.Fatal("no se encontró ningún saveRedirect en el paquete: ¿se renombró la ayuda? Esa prueba quedaría mockeada a sí misma.")
	}
	if len(registradas) == 0 {
		t.Fatal("no se encontró ninguna ruta registrada: el extractor quedó desfasado y la prueba no sabe de lo que habla.")
	}

	// "/" es siempre inválido como DESTINO GUARDADO aunque esté registrada:
	// cae en el fallback de portada y el usuario no vuelve a su listado.
	for destino, archivos := range destinos {
		if destino == "/" {
			t.Errorf("saveRedirect → \"/\" en %s: guardar y salir no debe enviar a la portada, sino al listado del módulo",
				strings.Join(sinRepetir(archivos), ", "))
			continue
		}
		if !registradas[destino] {
			t.Errorf("saveRedirect → %q en %s no es una ruta registrada: tras guardar el navegador cae al fallback de portada",
				destino, strings.Join(sinRepetir(archivos), ", "))
		}
	}
}

// sinRepetir devuelve la lista de archivos sin duplicados y ordenada, para que
// el mensaje de error sea estable de una corrida a otra.
func sinRepetir(in []string) []string {
	vistos := map[string]bool{}
	fuera := make([]string, 0, len(in))
	for _, s := range in {
		if !vistos[s] {
			vistos[s] = true
			fuera = append(fuera, s)
		}
	}
	sort.Strings(fuera)
	return fuera
}
