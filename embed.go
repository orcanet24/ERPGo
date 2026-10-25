// Package erpgow embeds the application assets — templates, static files and
// SQL migrations — so erpgow.exe is a single self-contained binary.
//
// Every lookup goes to disk first (working directory, then the directory of
// the executable) and falls back to the embedded copy. That keeps local
// development convenient (edits are picked up without rebuilding) while a
// deployment that only ships the binary keeps working.
package erpgow

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

//go:embed migrations
var migrationsFS embed.FS

// Templates returns the templates/ tree.
//
// Busca en este orden: las raíces del núcleo, después las de cada módulo
// desplegado (web/<code>) y, si no aparece, la copia embebida. Así una vista de
// módulo se escribe en web/<code>/templates/<pagina>.html y ya está: sin
// recompilar y sin tocar el núcleo.
//
// El núcleo va PRIMERO a propósito. Un módulo no puede pisar una plantilla del
// núcleo, ni por accidente ni queriendo.
func Templates() fs.FS { return overlayFS{embedded: templatesFS, modules: moduleRoots()} }

// Static returns the static/ tree.
//
// A module's CSS/JS lives in web/<code>/static/ and is served as
// /static/<code>/<file>, so the code is the FIRST segment of the URL. That needs
// its own resolution: a plain overlay would look for
// web/<code>/static/<code>/<file>, duplicating the code for no reason.
// Order: the core first (static/<code>/<file>), then the module whose code is
// that first segment, then the embedded copy.
func Static() fs.FS { return staticOverlay{core: coreFS{embedded: staticFS}, modules: moduleRoots()} }

// staticOverlay resuelve /static/<code>/<file> contra web/<code>/static/<file>.
type staticOverlay struct {
	core    coreFS
	modules []string
}

// Open implements fs.FS.
//
// El nombre llega con el prefijo "static/" porque server.go sirve
// fs.Sub(Static(), "static"), y de ahí el primer segmento es el código.
func (s staticOverlay) Open(name string) (fs.File, error) {
	if f, err := s.core.Open(name); err == nil {
		return f, nil
	}
	if code, rest, found := strings.Cut(strings.TrimPrefix(name, "static/"), "/"); found {
		for _, root := range s.modules {
			if filepath.Base(root) != code {
				continue
			}
			if f, err := os.Open(filepath.Join(root, "static", filepath.FromSlash(rest))); err == nil {
				return f, nil
			}
		}
	}
	return s.core.Open(name)
}

// Migrations returns the migrations/ tree.
//
// SIN raíces de módulo, a propósito. Las migraciones de un módulo las aplica el
// núcleo con su propio ledger, no se leen de un directorio: si se leyeran de
// aquí, cualquier .sql soltado en web/<code> se ejecutaría en cada empresa.
func Migrations() fs.FS { return coreFS{embedded: migrationsFS} }

// GlobalTemplate lee una plantilla SOLO del núcleo, ignorando los directorios de
// los módulos.
//
// Existe para layout.html. El layout es lo que da header, footer y Appearance a
// TODAS las pantallas, así que no puede depender de qué módulos estén
// desplegados en ese cliente: si layout.html llegara a salir de
// web/<code>/templates/, el módulo decidiera el chrome de toda la aplicación.
func GlobalTemplate(name string) ([]byte, error) {
	return fs.ReadFile(coreFS{embedded: templatesFS}, name)
}

// overlayFS serves a path from disk when the file exists there and from the
// embedded filesystem otherwise.
type overlayFS struct {
	embedded fs.FS
	// modules son las raíces de los módulos desplegados (web/<code>).
	modules []string
}

// Open implements fs.FS.
//
// El orden de búsqueda es núcleo -> módulos -> embebido, y es lo que convierte
// web/<code>/ en una raíz más sin cambiar la forma de usar estos paquetes: el
// llamante sigue pidiendo "templates/<pagina>" o "static/<hoja>".
func (o overlayFS) Open(name string) (fs.File, error) {
	if fs.ValidPath(name) {
		for _, root := range diskRoots() {
			if f, err := os.Open(filepath.Join(root, filepath.FromSlash(name))); err == nil {
				return f, nil
			}
		}
		for _, root := range o.modules {
			if f, err := os.Open(filepath.Join(root, filepath.FromSlash(name))); err == nil {
				return f, nil
			}
		}
	}
	return o.embedded.Open(name)
}

// coreFS es el overlay SIN las raíces de los módulos: solo el núcleo y la copia
// embebida.
type coreFS struct {
	embedded fs.FS
}

// Open implements fs.FS.
func (c coreFS) Open(name string) (fs.File, error) {
	if fs.ValidPath(name) {
		for _, root := range diskRoots() {
			if f, err := os.Open(filepath.Join(root, filepath.FromSlash(name))); err == nil {
				return f, nil
			}
		}
	}
	return c.embedded.Open(name)
}

var (
	diskRootsOnce sync.Once
	diskRootsVal  []string
)

// diskRoots returns the directories checked before the embedded copies:
// the working directory and the directory holding the executable.
func diskRoots() []string {
	diskRootsOnce.Do(func() {
		seen := map[string]bool{}
		add := func(dir string) {
			if dir == "" {
				dir = "."
			}
			clean := filepath.Clean(dir)
			if !seen[clean] {
				seen[clean] = true
				diskRootsVal = append(diskRootsVal, clean)
			}
		}
		add("")
		if exe, err := os.Executable(); err == nil {
			add(filepath.Dir(exe))
		}
	})
	return diskRootsVal
}

// webDirName es el directorio donde se despliega la web de los módulos, uno por
// módulo: web/<code>/{templates,static}.
const webDirName = "web"

// moduleRootsFn descubre los directorios de módulo desplegados. Es una variable
// (y no una función directa) para que los tests puedan apuntarla a un temporal
// en vez de escribir dentro del árbol del repositorio.
var moduleRootsFn = scanModuleRoots

var (
	modMu     sync.RWMutex
	modRoots  []string
	modLoaded bool
)

// moduleRoots devuelve los directorios de módulos ya desplegados, ordenados.
//
// Se cachea porque se consulta en CADA apertura de plantilla y de hoja de
// estilo. Instalar un módulo en caliente es lo raro: por eso el cache se
// invalida con RefreshModuleRoots, explícitamente, y no con un temporizador que
// se paga en cada render.
func moduleRoots() []string {
	modMu.RLock()
	if modLoaded {
		v := modRoots
		modMu.RUnlock()
		return v
	}
	modMu.RUnlock()

	modMu.Lock()
	defer modMu.Unlock()
	if !modLoaded {
		modRoots, modLoaded = moduleRootsFn(), true
	}
	return modRoots
}

// RefreshModuleRoots vuelve a buscar los directorios de módulo.
//
// La llama quien instala, activa o desactiva un módulo para que sus vistas
// estén disponibles sin reiniciar. Si no hay ningún directorio web/, no hace
// nada: es el caso normal de una instalación sin módulos.
func RefreshModuleRoots() {
	modMu.Lock()
	defer modMu.Unlock()
	modRoots, modLoaded = moduleRootsFn(), true
}

// scanModuleRoots busca un web/<code>/ por cada raíz de disco. Los directorios
// sin subcarpetas templates/ o static/ se incluyen igual: es más simple y no
// cuesta nada, porque un archivo que no existe simplemente sigue la búsqueda.
func scanModuleRoots() []string {
	vistos := map[string]bool{}
	var salida []string
	for _, base := range diskRoots() {
		entradas, err := os.ReadDir(filepath.Join(base, webDirName))
		if err != nil {
			continue // no hay web/, que es lo normal
		}
		for _, e := range entradas {
			if !e.IsDir() {
				continue
			}
			limpio := filepath.Clean(filepath.Join(base, webDirName, e.Name()))
			if !vistos[limpio] {
				vistos[limpio] = true
				salida = append(salida, limpio)
			}
		}
	}
	sort.Strings(salida)
	return salida
}
