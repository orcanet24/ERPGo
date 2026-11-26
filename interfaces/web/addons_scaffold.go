package web

// addons_scaffold.go — El RAD mínimo de addons (fases A+B de docs/PLAN-ADDONS.md).
//
// Qué hace y qué NO hace, para que el panel no venda humo:
//
//   - Crea la carpeta addons/<id>/ con manifest.json, main.go y, si se pide,
//     schema.sql. Los tres pasan los MISMOS candados que el escáner aplica a
//     cualquier addon: el generador no tiene una puerta trasera.
//   - Intenta compilar el binario con `go build` cuando el servidor tiene Go y
//     la carpeta cuelga de un módulo. Si no puede, deja los archivos y devuelve
//     el comando exacto: nunca finge que compiló.
//   - NO inventa verbos de db_write: un verbo solo existe si el núcleo conecta
//     su handler (addonVerboRegistrar). El addon generado nace de solo lectura
//     y lo dice en su propia pantalla.
//   - NO pisa una carpeta existente: el trabajo del dueño no se sobrescribe.

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// addonScaffoldMainFuente es la plantilla del main.go generado. Se embebe para
// que el binario del ERP no dependa de archivos sueltos en producción.
// Los delimitadores [[ ]] dejan intactos los {{ }} que usa html/template dentro
// del addon generado.
//
//go:embed addons_scaffold_main.tmpl
var addonScaffoldMainFuente string

// addonScaffoldSolicitud es lo que el formulario pide. Los campos vacíos toman
// valores derivados del id (ruta, permiso, etiqueta de menú), no valores
// mágicos: el panel muestra esos defaults en el propio formulario.
type addonScaffoldSolicitud struct {
	ID          string
	Nombre      string
	Version     string
	Descripcion string
	Autor       string
	Ruta        string
	MenuLabel   string
	Icono       string
	Permiso     string
	Tabla       string
	DBRead      []string
	HooksOut    []string
}

// addonScaffoldResultado es lo que el panel devuelve al navegador.
type addonScaffoldResultado struct {
	ID          string   `json:"id"`
	Dir         string   `json:"dir"`
	Archivos    []string `json:"archivos"`
	Compilado   bool     `json:"compilado"`
	Compilacion string   `json:"compilacion"`
	Aviso       string   `json:"aviso,omitempty"`
}

// addonScaffoldDatos es el contexto de la plantilla del main.go.
type addonScaffoldDatos struct {
	ID            string
	Nombre        string
	Descripcion   string
	Ruta          string
	Hooks         []string
	TablasLectura []string
}

// addonScaffoldCrear valida, genera y escribe un addon nuevo. Todo se valida
// ANTES de tocar el disco; si una escritura falla, se retira la carpeta que
// este mismo acto creó (nunca una que ya existiera).
func addonScaffoldCrear(dirBase string, sol addonScaffoldSolicitud) (addonScaffoldResultado, error) {
	var res addonScaffoldResultado

	sol = addonScaffoldNormalizar(sol)
	m := addonScaffoldManifiesto(sol)
	if err := validarManifiesto(m); err != nil {
		return res, err
	}

	manifiesto, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return res, fmt.Errorf("no se pudo serializar el manifiesto: %w", err)
	}
	manifiesto = append(manifiesto, '\n')

	var schema string
	if sol.Tabla != "" {
		schema = addonScaffoldSchemaSQL(sol)
		if _, err := addonValidarSchema(schema, m); err != nil {
			return res, fmt.Errorf("el schema.sql generado no pasa el candado: %w", err)
		}
	}

	fuente, err := addonScaffoldMainGo(sol)
	if err != nil {
		return res, err
	}

	dir := filepath.Join(dirBase, sol.ID)
	if _, err := os.Stat(dir); err == nil {
		return res, fmt.Errorf("ya existe la carpeta %q: el generador no sobrescribe addons existentes; elija otro id o edite el addon a mano", dir)
	} else if !os.IsNotExist(err) {
		return res, fmt.Errorf("no se pudo revisar la carpeta %q: %w", dir, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, fmt.Errorf("no se pudo crear la carpeta %q: %w", dir, err)
	}
	limpiar := func() { _ = os.RemoveAll(dir) }

	archivos := []string{"manifest.json", "main.go"}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifiesto, 0o644); err != nil {
		limpiar()
		return res, fmt.Errorf("no se pudo escribir manifest.json: %w", err)
	}
	if schema != "" {
		if err := os.WriteFile(filepath.Join(dir, "schema.sql"), []byte(schema), 0o644); err != nil {
			limpiar()
			return res, fmt.Errorf("no se pudo escribir schema.sql: %w", err)
		}
		archivos = append(archivos, "schema.sql")
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), fuente, 0o644); err != nil {
		limpiar()
		return res, fmt.Errorf("no se pudo escribir main.go: %w", err)
	}

	res.ID = sol.ID
	res.Dir = dir
	res.Archivos = archivos
	return res, nil
}

// addonScaffoldNormalizar completa los defaults visibles del formulario y
// normaliza las listas (db_read y hooks_out) a nombres limpios y sin repetir.
func addonScaffoldNormalizar(sol addonScaffoldSolicitud) addonScaffoldSolicitud {
	sol.ID = strings.TrimSpace(sol.ID)
	sol.Nombre = strings.TrimSpace(sol.Nombre)
	sol.Version = strings.TrimSpace(sol.Version)
	sol.Descripcion = strings.TrimSpace(sol.Descripcion)
	sol.Autor = strings.TrimSpace(sol.Autor)
	sol.Ruta = strings.TrimSpace(sol.Ruta)
	sol.MenuLabel = strings.TrimSpace(sol.MenuLabel)
	sol.Icono = strings.TrimSpace(sol.Icono)
	sol.Permiso = strings.TrimSpace(sol.Permiso)
	sol.Tabla = strings.TrimSpace(sol.Tabla)

	if sol.Version == "" {
		sol.Version = "1.0.0"
	}
	if sol.Autor == "" {
		sol.Autor = "ERPGo"
	}
	if sol.Ruta == "" && sol.ID != "" {
		sol.Ruta = "/" + sol.ID
	}
	if sol.MenuLabel == "" {
		sol.MenuLabel = sol.Nombre
	}
	if sol.Icono == "" {
		sol.Icono = "fa-puzzle-piece"
	}
	if sol.Permiso == "" && sol.ID != "" {
		sol.Permiso = sol.ID + "_ver"
	}

	sol.DBRead = addonScaffoldLista(strings.Join(sol.DBRead, ","))
	sol.HooksOut = addonScaffoldLista(strings.Join(sol.HooksOut, ","))
	return sol
}

// addonScaffoldManifiesto arma el manifiesto con las MISMAS reglas del
// contrato: prefijo del id en permisos y tablas, mínimo del núcleo actual,
// menú bajo la ruta declarada y sin verbos de escritura inventados.
func addonScaffoldManifiesto(sol addonScaffoldSolicitud) *addonManifest {
	m := &addonManifest{
		ID:             sol.ID,
		Name:           sol.Nombre,
		Version:        sol.Version,
		MinCoreVersion: addonCoreVersion,
		Description:    sol.Descripcion,
		Author:         sol.Autor,
		Routes:         []string{sol.Ruta},
		Menu: []addonMenuEntry{{
			ParentID:     nil,
			Label:        sol.MenuLabel,
			URL:          sol.Ruta,
			Icon:         sol.Icono,
			Perms:        []string{sol.Permiso},
			SortOrder:    90,
			ActivePrefix: sol.Ruta,
		}},
		Perms:    []addonPermiso{{Code: sol.Permiso, Label: "Ver " + sol.Nombre}},
		// Los cortes vacíos se escriben como [] y no como null: el manifiesto
		// generado se lee de un vistazo y el panel no tiene que traducir null.
		DBRead:    append([]string{}, sol.DBRead...),
		DBWrite:   []addonVerbo{},
		HooksOut:  append([]string{}, sol.HooksOut...),
		HooksIn:   []string{},
		TablesOwn: []string{},
	}
	if sol.Tabla != "" {
		m.TablesOwn = []string{sol.Tabla}
	}
	return m
}

// addonScaffoldSchemaSQL genera la única forma de DDL que el contrato acepta:
// CREATE TABLE IF NOT EXISTS de una tabla propia. El DDL es portable
// MySQL/SQLite (clave VARCHAR, sin AUTO_INCREMENT), como el del addon hola.
func addonScaffoldSchemaSQL(sol addonScaffoldSolicitud) string {
	return fmt.Sprintf(`-- schema.sql del addon %s, generado por el RAD de addons del ERPGo.
--
-- El núcleo solo aplica CREATE TABLE IF NOT EXISTS sobre tablas declaradas
-- en tables_own y con el prefijo del id. Ni DROP, ni ALTER, ni siembras: el
-- esquema ajeno no lo toca nadie.
CREATE TABLE IF NOT EXISTS %s (
    id        VARCHAR(64) NOT NULL PRIMARY KEY,
    creado_en VARCHAR(32) NOT NULL DEFAULT '',
    nota      VARCHAR(512) NOT NULL DEFAULT ''
);
`, sol.ID, sol.Tabla)
}

// addonScaffoldMainGo ejecuta la plantilla embebida y formatea el resultado.
// format.Source es el primer candado: si el generador produjera Go inválido,
// el error sale aquí y no se escribe nada.
func addonScaffoldMainGo(sol addonScaffoldSolicitud) ([]byte, error) {
	datos := addonScaffoldDatos{
		ID:            sol.ID,
		Nombre:        sol.Nombre,
		Descripcion:   sol.Descripcion,
		Ruta:          sol.Ruta,
		Hooks:         sol.HooksOut,
		TablasLectura: sol.DBRead,
	}
	t, err := template.New("main").
		Delims("[[", "]]").
		Funcs(template.FuncMap{"goq": strconv.Quote}).
		Parse(addonScaffoldMainFuente)
	if err != nil {
		return nil, fmt.Errorf("la plantilla del generador está mal formada: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, datos); err != nil {
		return nil, fmt.Errorf("no se pudo generar main.go: %w", err)
	}
	fuente, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("el generador produjo Go inválido (defecto del generador, no de los datos): %w", err)
	}
	return fuente, nil
}

// addonScaffoldCompilar intenta `go build` y devuelve (compiló, salida). Un
// fallo NO borra los archivos: el addon queda «sin binario» y el panel explica
// el motivo. Si el servidor no tiene Go o la carpeta no cuelga de un módulo,
// devuelve el comando exacto para compilarlo donde sí lo haya.
func addonScaffoldCompilar(dirBase, id string) (bool, string) {
	raiz, paquete, ok := addonScaffoldRaizModulo(dirBase, id)
	if !ok {
		return false, "No se compiló: no se encontró un go.mod que contenga la carpeta del addon. Los archivos quedaron listos; compile donde esté el código fuente con:\n\n  go build -o " + filepath.Join(dirBase, id, "erp-addon-"+id) + " ./addons/" + id
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return false, "No se compiló: el servidor no tiene el comando go en PATH. Los archivos quedaron listos; compile en una máquina con Go con:\n\n  go build -o " + filepath.Join(dirBase, id, "erp-addon-"+id) + " " + paquete
	}

	// La ruta del binario se resuelve absoluta: go build corre con cmd.Dir en
	// la raíz del módulo, y un -o relativo se interpretaría desde ahí, no desde
	// el directorio donde arrancó el ERP.
	dirAddon, err := filepath.Abs(filepath.Join(dirBase, id))
	if err != nil {
		return false, "No se compiló: no se pudo resolver la ruta del addon: " + err.Error()
	}
	binario := filepath.Join(dirAddon, "erp-addon-"+id)
	if runtime.GOOS == "windows" {
		binario += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "build", "-o", binario, paquete)
	cmd.Dir = raiz
	salida, err := cmd.CombinedOutput()
	texto := strings.TrimSpace(string(salida))
	if len(texto) > 6000 {
		texto = texto[:6000] + "\n… (salida truncada)"
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, "La compilación superó 120 s y se canceló. Los archivos quedaron listos; compile a mano para ver el error completo."
		}
		if texto == "" {
			texto = err.Error()
		}
		return false, "La compilación falló:\n" + texto
	}
	return true, texto
}

// addonScaffoldRaizModulo sube desde la carpeta de addons hasta el go.mod que
// la contiene y devuelve la raíz y el paquete Go del addon (./addons/<id>).
// Si la carpeta no cuelga de ningún módulo, no hay nada que compilar aquí.
func addonScaffoldRaizModulo(dirBase, id string) (raiz, paquete string, ok bool) {
	abs, err := filepath.Abs(dirBase)
	if err != nil {
		return "", "", false
	}
	objetivo := filepath.Join(abs, id)
	for p := abs; ; {
		if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
			rel, err := filepath.Rel(p, objetivo)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", "", false
			}
			return p, "./" + filepath.ToSlash(rel), true
		}
		padre := filepath.Dir(p)
		if padre == p {
			return "", "", false
		}
		p = padre
	}
}

// addonScaffoldLista parte una lista separada por comas, punto y coma o saltos
// de línea, quita vacíos y repetidos. Los nombres de tabla no llevan espacios;
// aceptarlos con espacios solo escondería un manifiesto que el escáner rechaza.
func addonScaffoldLista(s string) []string {
	campos := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	vistos := map[string]bool{}
	var salida []string
	for _, c := range campos {
		c = strings.TrimSpace(c)
		if c == "" || vistos[c] {
			continue
		}
		vistos[c] = true
		salida = append(salida, c)
	}
	return salida
}

// addonPanelScaffold es el POST del formulario «Crear un addon». Bajo /module,
// así que la puerta de master ya la aplicó el middleware; aquí solo se valida
// lo que el generador escribe y se re-escanea en caliente.
func (s *Server) addonPanelScaffold(w http.ResponseWriter, r *http.Request) {
	if s.addons == nil {
		jsonError(w, "No hay carpeta de addons en este arranque. Créela junto al binario y reinicie el ERP.", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		jsonError(w, "No se pudo leer el formulario: "+err.Error(), http.StatusBadRequest)
		return
	}
	sol := addonScaffoldSolicitud{
		ID:          r.FormValue("id"),
		Nombre:      r.FormValue("nombre"),
		Version:     r.FormValue("version"),
		Descripcion: r.FormValue("descripcion"),
		Autor:       r.FormValue("autor"),
		Ruta:        r.FormValue("ruta"),
		MenuLabel:   r.FormValue("menu_label"),
		Icono:       r.FormValue("icono"),
		Permiso:     r.FormValue("permiso"),
		Tabla:       r.FormValue("tabla"),
		DBRead:      addonScaffoldLista(r.FormValue("db_read")),
		HooksOut:    addonScaffoldLista(strings.Join(r.Form["hooks_out"], ",")),
	}

	res, err := addonScaffoldCrear(s.addons.directorio, sol)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	res.Compilado, res.Compilacion = addonScaffoldCompilar(s.addons.directorio, res.ID)
	if err := s.addons.reescanear(); err != nil {
		res.Aviso = "Los archivos se crearon, pero el re-escaneo falló: " + err.Error()
	} else {
		invalidarCacheMenu()
	}

	mensaje := "Addon " + res.ID + " creado."
	if res.Compilado {
		mensaje += " Compilado y re-escaneado: se sirve en /mod/" + res.ID + "."
	} else {
		mensaje += " Los archivos quedaron listos; revise la compilación antes de abrirlo."
	}
	writeJSON(w, map[string]interface{}{
		"status":    1,
		"msg":       mensaje,
		"resultado": res,
		"estado":    s.addonsEstado(),
	})
}
