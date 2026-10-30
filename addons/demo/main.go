// addons/demo/main.go — El módulo demo de la frontera (docs/PLAN-ADDONS.md, fase 2).
//
// QUÉ ES. El módulo de REFERENCIA: la carpeta que se copia para arrancar uno
// nuevo. Demuestra, en una sola pantalla, las tres capacidades de la fase 2
// y ninguna más:
//
//  1. Menú y permiso propios. El menú lo pinta el ERP desde el manifiesto;
//     aquí solo existe la pantalla y el candado de permiso está en el núcleo
//     (X-ERP-Perms), porque la sesión es del ERP y este módulo no hace login.
//  2. LECTURA GRANULAR. Lee los empleados de la empresa activa (hr_employee)
//     y sus propios avisos con el DSN de solo SELECT que el núcleo entregó por
//     entorno (ERP_DB_READ_DSN). La cabecera X-ERP-DB dice EN QUÉ base
//     consultar: el mismo módulo atiende a varias empresas y el DSN llega sin
//     base por defecto. Sin esa cabecera el grant no tiene a dónde ir.
//  3. ESCRITURA POR EL CANAL. Publicar un aviso no hace INSERT desde aquí:
//     el formulario sube a POST /__addon/demo/aviso.publicar con el token
//    efímero de esta sesión y el contexto X-ERP-* que la pantalla recibió;
//     el núcleo valida verbo, sesión, empresa y permiso, y escribe por el
//    orquestador (journal + audit_log). El módulo no toca la base de datos
//    del ERP: no tiene cómo.
//
// Las cookies del ERP nunca llegan a este proceso: el proxy las retira. La
// pantalla lo comprueba en vivo y lo dice en la cara si algún día falla.
//
// Para compilarlo: go build -o addons/demo/erp-addon-demo ./addons/demo
// (el ERP lo lanza solo si encuentra el binario junto al manifest.json).

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	tituloPantalla = "Módulo demo"
	maxTitulo      = 160
	maxMensaje     = 4000
)

var (
	token string
	core  string
	// dsnLectura es ERP_DB_READ_DSN: usuario MySQL de SOLO SELECT con la
	// clave de esta sesión, SIN base por defecto. La base la pone cada
	// petición en su cabecera X-ERP-DB.
	dsnLectura string
	// reBase acepta solo un nombre de base legible: lo que viene por cabecera
	// es dato de entrada y se concatena en un DSN, así que no se improvisa.
	reBase = regexp.MustCompile(`^[A-Za-z0-9_$-]+$`)
)

func main() {
	token = os.Getenv("ERP_ADDON_TOKEN")
	core = os.Getenv("ERP_CORE_URL")
	dsnLectura = os.Getenv("ERP_DB_READ_DSN")
	porte := os.Getenv("ERP_ADDON_PORT")
	if porte == "" || token == "" {
		fmt.Fprintln(os.Stderr, "demo: sin ERP_ADDON_PORT o ERP_ADDON_TOKEN: este binario lo lanza el ERP, no un humano.")
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok","addon":"demo"}`))
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		// Solo el núcleo, con el token de ESTA sesión, manda el apagado
		// (docs/PLAN-ADDONS.md §3.5).
		if r.Header.Get("X-ERP-Addon-Token") != token {
			http.Error(w, "Sin el token de esta sesión no hay apagado.", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"apagando"}`))
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(0)
		}()
	})
	mux.HandleFunc("/demo", pantalla)
	mux.HandleFunc("/demo/aviso", publicar)

	srv := &http.Server{Addr: "127.0.0.1:" + porte, Handler: mux}
	log.Printf("demo: módulo activo en 127.0.0.1:%s", porte)

	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt)
	go func() {
		<-parar
		_ = srv.Shutdown(context.Background())
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Printf("demo: servidor terminado: %v", err)
	}
}

// ---------- Lectura granular ----------

// conectarLectura abre la base que ESTA petición está usando. El DSN de
// entorno no trae base: aquí se le antepone la que el núcleo mandó en
// X-ERP-DB. Si la cabecera viene vacía o ilegible, no se consulta nada:
// adivinar la base sería leer la empresa equivocada en silencio.
func conectarLectura(base string) (*sql.DB, error) {
	if dsnLectura == "" {
		return nil, fmt.Errorf("sin ERP_DB_READ_DSN: revise db_read en el manifiesto y la fila de este módulo en Configuración → Módulos")
	}
	if !reBase.MatchString(base) {
		return nil, fmt.Errorf("la cabecera X-ERP-DB llegó vacía o ilegible (%q): no sé en qué base consultar", base)
	}
	dsn := dsnLectura
	if !strings.HasSuffix(dsn, "/") {
		dsn += "/"
	}
	db, err := sql.Open("mysql", dsn+base)
	if err != nil {
		return nil, err
	}
	// El ping es el que dice si el grant y la base coinciden de verdad:
	// sql.Open solo prepara la conexión y calla cualquier error.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

type persona struct {
	Codigo string
	Nombre string
}

// leerPersonas trae los empleados activos de la empresa. Es la tabla DEL ERP
// declarada en db_read: el núcleo concedió SELECT sobre ella y nada más.
func leerPersonas(ctx context.Context, db *sql.DB) ([]persona, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT IFNULL(code,''), TRIM(CONCAT(first_name,' ',last_name))
		FROM hr_employee
		WHERE status = 1
		ORDER BY first_name, last_name
		LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	salida := []persona{}
	for rows.Next() {
		var p persona
		if err := rows.Scan(&p.Codigo, &p.Nombre); err != nil {
			return nil, err
		}
		salida = append(salida, p)
	}
	return salida, rows.Err()
}

type aviso struct {
	Titulo  string
	Mensaje string
	Autor   string
	Creada  string
}

// leerAvisos trae los avisos de la tabla PROPIA del módulo. Se lee por el
// mismo canal de lectura granular porque la tabla está declarada en db_read:
// sin esa entrada, el módulo podría escribir lo que no podría pintar.
func leerAvisos(ctx context.Context, db *sql.DB) ([]aviso, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT titulo, mensaje, autor, creada
		FROM demo_avisos
		ORDER BY creada DESC, id DESC
		LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	salida := []aviso{}
	for rows.Next() {
		var a aviso
		if err := rows.Scan(&a.Titulo, &a.Mensaje, &a.Autor, &a.Creada); err != nil {
			return nil, err
		}
		salida = append(salida, a)
	}
	return salida, rows.Err()
}

// ---------- Escritura por el canal de negocio ----------

// publicar recoge el formulario y lo manda al núcleo por el canal. Este
// proceso NUNCA escribe en la base: pide que el orquestador lo haga, con el
// verbo declarado en el manifiesto y el contexto que el proxy ya autenticó.
func publicar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/demo", http.StatusSeeOther)
		return
	}
	titulo := strings.TrimSpace(r.FormValue("titulo"))
	mensaje := strings.TrimSpace(r.FormValue("mensaje"))
	if titulo == "" || mensaje == "" {
		renderizar(w, r, "El título y el aviso son obligatorios.", "")
		return
	}
	if len([]rune(titulo)) > maxTitulo || len([]rune(mensaje)) > maxMensaje {
		renderizar(w, r, fmt.Sprintf("El aviso no pasa los límites (título %d y texto %d caracteres como máximo).", maxTitulo, maxMensaje), "")
		return
	}
	if core == "" {
		renderizar(w, r, "Este binario lo lanza el ERP: le falta ERP_CORE_URL en el entorno.", "")
		return
	}
	cuerpo, _ := json.Marshal(map[string]interface{}{"titulo": titulo, "mensaje": mensaje})
	req, err := http.NewRequest(http.MethodPost, core+"/__addon/demo/aviso.publicar", bytes.NewReader(cuerpo))
	if err != nil {
		renderizar(w, r, "No se pudo preparar la petición al canal: "+err.Error(), "")
		return
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-User", r.Header.Get("X-ERP-User"))
	req.Header.Set("X-ERP-Tenant", r.Header.Get("X-ERP-Tenant"))
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		renderizar(w, r, "El núcleo no respondió: "+err.Error(), "")
		return
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		renderizar(w, r, "No se pudo leer la respuesta del canal: "+err.Error(), "")
		return
	}
	var salida struct {
		Status      int    `json:"status"`
		Error       string `json:"error"`
		Peticion    string `json:"peticion"`
		RegistroID  string `json:"registro_id"`
		Duplicado   bool   `json:"duplicado"`
		Correlativo uint64 `json:"correlativo"`
	}
	_ = json.Unmarshal(datos, &salida)
	if resp.StatusCode != http.StatusOK || salida.Status != 1 {
		renderizar(w, r, fmt.Sprintf("El canal respondió HTTP %d: %s", resp.StatusCode, salida.Error), "")
		return
	}
	extra := ""
	if salida.Duplicado {
		extra = " (ya estaba registrado: idempotencia)"
	}
	renderizar(w, r, "", fmt.Sprintf("Aviso publicado por el orquestador — registro %s, correlativo %d, petición %s%s.",
		salida.RegistroID, salida.Correlativo, salida.Peticion, extra))
}

// ---------- Pantalla ----------

func pantalla(w http.ResponseWriter, r *http.Request) {
	renderizar(w, r, "", "")
}

// renderizar pinta la pantalla completa: contexto que entrega el ERP, estado
// del canal, lectura granular (personas y avisos) y el formulario. Los dos
// parámetros libres son el fallo en rojo o el resultado en verde.
func renderizar(w http.ResponseWriter, r *http.Request, falla, exito string) {
	base := r.Header.Get("X-ERP-DB")
	cookies := r.Header.Get("Cookie")
	mostrarCookies := "ninguna (el núcleo las retiró, como exige el contrato)"
	if cookies != "" {
		// Si esto aparece, la frontera tiene un agujero: se dice en la cara.
		mostrarCookies = "⚠ HAY COOKIES: la frontera no está limpiando"
	}

	ping := estadoPing()

	// Lectura granular: dos consultas independientes, y cada fallo se explica
	// en su renglón en vez de dejar la pantalla en blanco.
	personas, avisos := []persona{}, []aviso{}
	var errPersonas, errAvisos string
	if db, err := conectarLectura(base); err != nil {
		errPersonas = err.Error()
		errAvisos = err.Error()
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if ps, err := leerPersonas(ctx, db); err != nil {
			errPersonas = err.Error()
		} else {
			personas = ps
		}
		if as, err := leerAvisos(ctx, db); err != nil {
			errAvisos = err.Error()
		} else {
			avisos = as
		}
		_ = db.Close()
	}

	var cuadro strings.Builder
	if errPersonas != "" {
		fmt.Fprintf(&cuadro, `<p class="mal">Lectura de empleados: %s</p>`, e(errPersonas))
	} else if len(personas) == 0 {
		cuadro.WriteString(`<p class="soso">La empresa activa no tiene empleados activos todavía.</p>`)
	} else {
		cuadro.WriteString(`<h3>Personas de la empresa (lectura granular sobre hr_employee)</h3><ul class="lista">`)
		for _, p := range personas {
			fmt.Fprintf(&cuadro, "<li><strong>%s</strong> <span>%s</span></li>", e(p.Nombre), e(p.Codigo))
		}
		cuadro.WriteString("</ul>")
	}

	var muro strings.Builder
	if errAvisos != "" {
		fmt.Fprintf(&muro, `<p class="mal">Lectura de avisos: %s</p>`, e(errAvisos))
	} else if len(avisos) == 0 {
		muro.WriteString(`<p class="soso">Todavía no hay avisos. El primero es tuyo.</p>`)
	} else {
		for _, a := range avisos {
			fmt.Fprintf(&muro, `<article class="aviso"><h4>%s</h4><p>%s</p><footer>%s · %s</footer></article>`,
				e(a.Titulo), e(a.Mensaje), e(a.Autor), e(a.Creada))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>%s</title>
<style>
 body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;margin:0;background:#f5f6f8;color:#1c1e21}
 main{max-width:820px;margin:0 auto;padding:24px}
 .caja{background:#fff;border:1px solid #e2e4e8;border-radius:10px;padding:16px 18px;margin-bottom:18px}
 h1{font-size:22px;margin:0 0 6px}
 h3{font-size:15px;margin:16px 0 6px}
 dl{display:grid;grid-template-columns:210px 1fr;gap:4px 12px;margin:0;font-size:14px}
 dt{color:#5b6068} dd{margin:0;word-break:break-word}
 form label{display:block;font-size:14px;margin-bottom:10px}
 input,textarea{width:100%%;box-sizing:border-box;padding:8px 10px;border:1px solid #cfd3da;border-radius:6px;font:inherit}
 button{background:#1f6feb;color:#fff;border:0;border-radius:6px;padding:9px 16px;font-size:14px;cursor:pointer}
 .mal{color:#b00020;font-size:14px} .ok{color:#0a7a33;font-size:14px} .soso{color:#5b6068;font-size:14px}
 .lista{list-style:none;padding:0;margin:6px 0 0;font-size:14px}
 .lista li{padding:5px 0;border-bottom:1px dashed #e2e4e8} .lista span{color:#5b6068;font-size:12px}
 .aviso{border:1px solid #e2e4e8;border-radius:8px;padding:10px 12px;margin-bottom:10px;background:#fbfbfc}
 .aviso h4{margin:0 0 4px;font-size:15px} .aviso p{margin:0;white-space:pre-wrap}
 .aviso footer{color:#5b6068;font-size:12px;margin-top:6px}
 a{color:#1f6feb}
</style></head><body><main>
 <div class="caja">
  <h1>%s</h1>
  <p class="soso">Esta pantalla vive en el <strong>binario del módulo</strong>, no en el núcleo del ERP. El menú, la sesión y los candados son del ERP.</p>
  <dl>
   <dt>Usuario</dt><dd>%s</dd>
   <dt>Empresa activa</dt><dd>%s</dd>
   <dt>Base consultada (X-ERP-DB)</dt><dd>%s</dd>
   <dt>Permisos concedidos</dt><dd>%s</dd>
   <dt>Cookies del ERP recibidas</dt><dd>%s</dd>
   <dt>Canal /__addon/demo/aviso.publicar</dt><dd>%s</dd>
  </dl>
  <p class="mal">%s</p>
  <p class="ok">%s</p>
 </div>

 <div class="caja">
  <h3>Publicar un aviso (escritura por el orquestador)</h3>
  <form method="post" action="/demo/aviso">
   <input type="hidden" name="csrf_token" value="%s">
   <label>Título<br><input name="titulo" maxlength="%d" required></label>
   <label>Aviso<br><textarea name="mensaje" rows="4" maxlength="%d" required></textarea></label>
   <button type="submit">Publicar por el canal</button>
  </form>
 </div>

 <div class="caja">%s</div>
 <div class="caja">%s</div>

 <p><a href="/home">← volver al ERP</a> · <a href="/module/addons">Configuración → Módulos</a></p>
</main></body></html>`,
		tituloPantalla, tituloPantalla,
		e(r.Header.Get("X-ERP-User")), e(r.Header.Get("X-ERP-Tenant")), e(base),
		e(r.Header.Get("X-ERP-Perms")), mostrarCookies, e(ping),
		e(falla), e(exito),
		e(r.Header.Get("X-ERP-CSRF")),
		maxTitulo, maxMensaje,
		cuadro.String(), muro.String())
}

// estadoPing verifica el canal de negocio con GET /__addon/demo/ping: el único
// GET permitido del canal, porque no escribe nada.
func estadoPing() string {
	if core == "" {
		return "sin ERP_CORE_URL en el entorno"
	}
	req, err := http.NewRequest(http.MethodGet, core+"/__addon/demo/ping", nil)
	if err != nil {
		return "no se pudo preparar la petición"
	}
	req.Header.Set("X-ERP-Addon-Token", token)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return "no se pudo contactar al núcleo: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return "OK"
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// e escapa para HTML. El contenido viene del formulario y de la base: sin
// escapar, un aviso con <script> sería un agujero abierto en la pantalla.
func e(s string) string { return html.EscapeString(s) }
