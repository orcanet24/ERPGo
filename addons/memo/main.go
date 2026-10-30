// addons/memo/main.go — Módulo Memo, fase A (docs/PLAN-MEMO.md).
//
// QUÉ HAY AQUÍ HOY. El muro: publicar una nota y ver, en orden, las notas que
// este usuario tiene derecho a ver — las suyas (sean privadas o no), las de
// visibilidad «empresa» y las que otra persona le compartió a él en
// concreto. Nada más: el picker de gente, los comentarios, los adjuntos, el
// chat y el calendario llegan con las fases B a F del plan.
//
// CÓMO SE CUMPLE LA VISIBILIDAD. La consulta de lectura es el candado: la
// filtración no se decide en la pantalla sino en el WHERE, contra la base de
// la empresa activa que el núcleo indicó en X-ERP-DB. La gente (users) vive
// en la base de identidad que trae X-ERP-Auth-DB: de ahí salen los nombres
// para pintar y el listado con el que se comparte.
//
// CÓMO SE ESCRIBE. Este proceso no hace INSERT: manda el formulario a
// POST /__addon/memo/nota.crear con el token efímero de la sesión y el
// contexto X-ERP-* que el proxy ya autenticó; el núcleo valida verbo, sesión,
// empresa y permiso, y baja por el orquestador (journal + audit_log).
//
// El formulario lleva el campo csrf_token con el token que el núcleo entregó
// en X-ERP-CSRF: enforceCSRF corre ANTES del proxy y sin ese campo el POST
// sale 403.
//
// Compilar: go build -o addons/memo/erp-addon-memo ./addons/memo

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
	tituloPantalla = "Memo"
	maxTitulo      = 200
	maxTexto       = 20000
)

var (
	token         string
	core          string
	dsnLectura    string
	reBase        = regexp.MustCompile(`^[A-Za-z0-9_$-]+$`)
	visibilidades = map[string]bool{"privada": true, "empresa": true, "personas": true}
)

func main() {
	token = os.Getenv("ERP_ADDON_TOKEN")
	core = os.Getenv("ERP_CORE_URL")
	dsnLectura = os.Getenv("ERP_DB_READ_DSN")
	porte := os.Getenv("ERP_ADDON_PORT")
	if porte == "" || token == "" {
		fmt.Fprintln(os.Stderr, "memo: sin ERP_ADDON_PORT o ERP_ADDON_TOKEN: este binario lo lanza el ERP, no un humano.")
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok","addon":"memo"}`))
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/memo", muro)
	mux.HandleFunc("/memo/nota", publicar)

	srv := &http.Server{Addr: "127.0.0.1:" + porte, Handler: mux}
	log.Printf("memo: módulo activo en 127.0.0.1:%s", porte)

	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt)
	go func() {
		<-parar
		_ = srv.Shutdown(context.Background())
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Printf("memo: servidor terminado: %v", err)
	}
}

// ---------- Lectura granular ----------

// conectarLectura abre UNA base por petición: la que el núcleo indicó en la
// cabecera pedida. El DSN de entorno viene sin base; sin la cabecera no se
// consulta nada, porque adivinar sería leer la empresa equivocada en silencio.
func conectarLectura(cabecera, cual string) (*sql.DB, error) {
	if dsnLectura == "" {
		return nil, fmt.Errorf("sin ERP_DB_READ_DSN: revise db_read en el manifiesto y la fila del módulo en Configuración → Módulos")
	}
	if !reBase.MatchString(cabecera) {
		return nil, fmt.Errorf("la cabecera %s llegó vacía o ilegible (%q)", cual, cabecera)
	}
	dsn := dsnLectura
	if !strings.HasSuffix(dsn, "/") {
		dsn += "/"
	}
	db, err := sql.Open("mysql", dsn+cabecera)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// gente es una persona de la empresa: su id de usuario (el mismo que trae
// X-ERP-User, con el que se comparte) y su nombre para pintar.
type gente struct {
	ID     string
	Nombre string
}

// leerGente consulta la base de IDENTIDAD (X-ERP-Auth-DB). De ahí salen tanto
// el listado del selector como el mapa de nombres para firmar las notas.
func leerGente(ctx context.Context, db *sql.DB) ([]gente, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT user_id, TRIM(CONCAT(first_name,' ',last_name))
		FROM users
		WHERE status = 1
		ORDER BY first_name, last_name
		LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	salida := []gente{}
	for rows.Next() {
		var g gente
		if err := rows.Scan(&g.ID, &g.Nombre); err != nil {
			return nil, err
		}
		if g.Nombre == "" {
			g.Nombre = g.ID
		}
		salida = append(salida, g)
	}
	return salida, rows.Err()
}

// nombresDevuelve el mapa id → nombre, para no repetir la consulta.
func nombresDevuelve(lista []gente) map[string]string {
	m := make(map[string]string, len(lista))
	for _, g := range lista {
		m[g.ID] = g.Nombre
	}
	return m
}

type nota struct {
	ID          string
	Titulo      string
	Texto       string
	AutorID     string
	Autor       string
	Visibilidad string
	Creada      string
}

// consultaMuro es EL CANDADO de visibilidad, en una sola sentencia y
// separada de su envoltorio para que la prueba pueda correrla de verdad
// (addons/memo/visibilidad_test.go) contra datos reales. Ve:
//
//   - lo propio, sea privada o no (autor = usuario);
//   - lo de visibilidad «empresa»;
//   - lo que OTRO le compartió a este usuario en concreto (destinatarios).
//
// Una nota privada ajena no aparece por ninguna otra vía: no hay orden que la
// filtre después, se filtra aquí.
const consultaMuro = `
		SELECT n.id, n.titulo, n.texto, n.autor, n.visibilidad, n.creada
		FROM memo_notas n
		WHERE n.empresa = ? AND n.estado = 'activo'
		  AND ( n.autor = ?
		     OR n.visibilidad = 'empresa'
		     OR EXISTS (SELECT 1 FROM memo_destinatarios d
		                WHERE d.nota_id = n.id AND d.destino_id = ?) )
		ORDER BY n.creada DESC
		LIMIT 50`

func leerNotas(ctx context.Context, db *sql.DB, empresa, usuario string) ([]nota, error) {
	rows, err := db.QueryContext(ctx, consultaMuro, empresa, usuario, usuario)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	salida := []nota{}
	for rows.Next() {
		var n nota
		if err := rows.Scan(&n.ID, &n.Titulo, &n.Texto, &n.AutorID, &n.Visibilidad, &n.Creada); err != nil {
			return nil, err
		}
		salida = append(salida, n)
	}
	return salida, rows.Err()
}

// ---------- Escritura por el canal ----------

// publicar recoge el formulario y lo manda al núcleo. Este proceso jamás
// escribe en la base: pide que el orquestador lo haga con el verbo declarado.
func publicar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/memo", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		muroConAviso(w, r, "El formulario no se pudo leer: "+err.Error(), "")
		return
	}
	titulo := strings.TrimSpace(r.FormValue("titulo"))
	texto := strings.TrimSpace(r.FormValue("texto"))
	visibilidad := strings.TrimSpace(r.FormValue("visibilidad"))
	destinatarios := r.Form["destinatarios"]

	if titulo == "" || texto == "" {
		muroConAviso(w, r, "El título y el texto de la nota son obligatorios.", "")
		return
	}
	if len([]rune(titulo)) > maxTitulo || len([]rune(texto)) > maxTexto {
		muroConAviso(w, r, fmt.Sprintf("La nota no pasa los límites (título %d y texto %d caracteres).", maxTitulo, maxTexto), "")
		return
	}
	if !visibilidades[visibilidad] {
		muroConAviso(w, r, "La visibilidad debe ser privada, empresa o personas.", "")
		return
	}
	if visibilidad == "personas" && len(destinatarios) == 0 {
		muroConAviso(w, r, "Compartir con personas exige elegir al menos a una.", "")
		return
	}
	if core == "" {
		muroConAviso(w, r, "Este binario lo lanza el ERP: le falta ERP_CORE_URL en el entorno.", "")
		return
	}

	cuerpo, _ := json.Marshal(map[string]interface{}{
		"titulo": titulo, "texto": texto, "visibilidad": visibilidad,
		"destinatarios": destinatarios,
	})
	req, err := http.NewRequest(http.MethodPost, core+"/__addon/memo/nota.crear", bytes.NewReader(cuerpo))
	if err != nil {
		muroConAviso(w, r, "No se pudo preparar la petición al canal: "+err.Error(), "")
		return
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-User", r.Header.Get("X-ERP-User"))
	req.Header.Set("X-ERP-Tenant", r.Header.Get("X-ERP-Tenant"))
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		muroConAviso(w, r, "El núcleo no respondió: "+err.Error(), "")
		return
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		muroConAviso(w, r, "No se pudo leer la respuesta del canal: "+err.Error(), "")
		return
	}
	var salida struct {
		Status     int    `json:"status"`
		Error      string `json:"error"`
		Peticion   string `json:"peticion"`
		RegistroID string `json:"registro_id"`
		Compartido int    `json:"compartido"`
	}
	_ = json.Unmarshal(datos, &salida)
	if resp.StatusCode != http.StatusOK || salida.Status != 1 {
		muroConAviso(w, r, fmt.Sprintf("El canal respondió HTTP %d: %s", resp.StatusCode, salida.Error), "")
		return
	}
	detalle := ""
	if salida.Compartido > 0 {
		detalle = fmt.Sprintf(" Compartida con %d persona(s).", salida.Compartido)
	}
	muroConAviso(w, r, "", fmt.Sprintf("Nota publicada (registro %s, petición %s).%s", salida.RegistroID, salida.Peticion, detalle))
}

// ---------- Pantalla ----------

func muro(w http.ResponseWriter, r *http.Request) {
	muroConAviso(w, r, "", "")
}

func muroConAviso(w http.ResponseWriter, r *http.Request, falla, exito string) {
	baseEmpresa := r.Header.Get("X-ERP-DB")
	baseAuth := r.Header.Get("X-ERP-Auth-DB")
	usuario := r.Header.Get("X-ERP-User")

	cookies := r.Header.Get("Cookie")
	mostrarCookies := "ninguna (el núcleo las retiró, como exige el contrato)"
	if cookies != "" {
		mostrarCookies = "⚠ HAY COOKIES: la frontera no está limpiando"
	}

	genteLista := []gente{}
	var errGente string
	notas := []nota{}
	var errNotas string

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	if db, err := conectarLectura(baseAuth, "X-ERP-Auth-DB"); err != nil {
		errGente = err.Error()
	} else {
		if g, err := leerGente(ctx, db); err != nil {
			errGente = err.Error()
		} else {
			genteLista = g
		}
		_ = db.Close()
	}

	if db, err := conectarLectura(baseEmpresa, "X-ERP-DB"); err != nil {
		errNotas = err.Error()
	} else {
		if ns, err := leerNotas(ctx, db, r.Header.Get("X-ERP-Tenant"), usuario); err != nil {
			errNotas = err.Error()
		} else {
			notas = ns
		}
		_ = db.Close()
	}

	nombres := nombresDevuelve(genteLista)

	var opciones strings.Builder
	// El selector va SIEMPRE en el formulario, también sin gente cargada: un
	// campo que desaparece deja al usuario con un formulario a medias y sin
	// saber qué falló.
	opciones.WriteString(`<select name="destinatarios" multiple size="6">`)
	if errGente != "" {
		fmt.Fprintf(&opciones, `<option disabled selected>No se pudo cargar la gente: %s</option>`, e(errGente))
	} else {
		for _, g := range genteLista {
			fmt.Fprintf(&opciones, `<option value="%s">%s</option>`, e(g.ID), e(g.Nombre))
		}
	}
	opciones.WriteString(`</select>`)

	var muroHTML strings.Builder
	if errNotas != "" {
		fmt.Fprintf(&muroHTML, `<p class="mal">No se pudieron leer las notas: %s</p>`, e(errNotas))
	} else if len(notas) == 0 {
		muroHTML.WriteString(`<p class="soso">No hay notas visibles todavía. La primera es tuya.</p>`)
	} else {
		for _, n := range notas {
			quien := nombres[n.AutorID]
			if quien == "" {
				quien = n.AutorID
			}
			etiqueta := map[string]string{"privada": "privada", "empresa": "empresa", "personas": "compartida"}[n.Visibilidad]
			if etiqueta == "" {
				etiqueta = n.Visibilidad
			}
			fmt.Fprintf(&muroHTML, `<article class="nota"><h4>%s</h4><p>%s</p>
<footer>%s · %s · visibilidad %s</footer></article>`,
				e(n.Titulo), e(n.Texto), e(quien), e(n.Creada), e(etiqueta))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>%s</title>
<style>
 body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;margin:0;background:#f5f6f8;color:#1c1e21}
 main{max-width:840px;margin:0 auto;padding:24px}
 .caja{background:#fff;border:1px solid #e2e4e8;border-radius:10px;padding:16px 18px;margin-bottom:18px}
 h1{font-size:22px;margin:0 0 6px} h3{font-size:15px;margin:16px 0 8px}
 dl{display:grid;grid-template-columns:200px 1fr;gap:4px 12px;margin:0;font-size:14px}
 dt{color:#5b6068} dd{margin:0;word-break:break-word}
 label{display:block;font-size:14px;margin-bottom:10px}
 input[type=text],textarea,select{width:100%%;box-sizing:border-box;padding:8px 10px;border:1px solid #cfd3da;border-radius:6px;font:inherit;background:#fff}
 button{background:#1f6feb;color:#fff;border:0;border-radius:6px;padding:9px 16px;font-size:14px;cursor:pointer}
 .mal{color:#b00020;font-size:14px} .ok{color:#0a7a33;font-size:14px} .soso{color:#5b6068;font-size:14px}
 .nota{border:1px solid #e2e4e8;border-radius:8px;padding:12px 14px;margin-bottom:12px;background:#fbfbfc}
 .nota h4{margin:0 0 4px;font-size:15px} .nota p{margin:0;white-space:pre-wrap}
 .nota footer{color:#5b6068;font-size:12px;margin-top:8px}
 .dos{display:grid;grid-template-columns:1fr 1fr;gap:16px}
 a{color:#1f6feb}
</style></head><body><main>
 <div class="caja">
  <h1>%s</h1>
  <p class="soso">Muro de comunicación interna de la empresa. Vive en su propio módulo: el menú, la sesión y los candados son del ERP.</p>
  <dl>
   <dt>Usuario</dt><dd>%s</dd>
   <dt>Empresa activa</dt><dd>%s · base %s</dd>
   <dt>Base de identidad</dt><dd>%s</dd>
   <dt>Permisos concedidos</dt><dd>%s</dd>
   <dt>Cookies del ERP recibidas</dt><dd>%s</dd>
  </dl>
  <p class="mal">%s</p><p class="ok">%s</p>
 </div>

 <div class="caja">
  <h3>Nueva nota</h3>
  <form method="post" action="/memo/nota">
   <input type="hidden" name="csrf_token" value="%s">
   <label>Título<br><input type="text" name="titulo" maxlength="%d" required></label>
   <label>Texto<br><textarea name="texto" rows="5" maxlength="%d" required></textarea></label>
   <div class="dos">
    <label>Visibilidad<br>
     <select name="visibilidad" id="visibilidad">
      <option value="privada">Privada (solo yo)</option>
      <option value="empresa" selected>Toda la empresa</option>
      <option value="personas">Compartir con personas…</option>
     </select></label>
    <label>Compartir con<br>%s</label>
   </div>
   <button type="submit">Publicar nota</button>
  </form>
 </div>

 <div class="caja">
  <h3>Muro</h3>
  %s
 </div>

 <p><a href="/home">← volver al ERP</a> · <a href="/module/addons">Configuración → Módulos</a></p>
</main>
<script>
 // El selector solo aporta cuando la visibilidad es «personas»: el candado
 // está en el núcleo, esto es solo no pedirle a la gente lo que no hace falta.
 var sel = document.getElementById('visibilidad');
 if (sel) sel.addEventListener('change', function(){
   this.form.querySelector('select[name=destinatarios]').disabled = (this.value !== 'personas');
 });
</script>
</body></html>`,
		tituloPantalla, tituloPantalla,
		e(usuario), e(r.Header.Get("X-ERP-Tenant")), e(baseEmpresa), e(baseAuth),
		e(r.Header.Get("X-ERP-Perms")), mostrarCookies,
		e(falla), e(exito),
		e(r.Header.Get("X-ERP-CSRF")), maxTitulo, maxTexto,
		opciones.String(),
		muroHTML.String())
}

// e escapa para HTML: el título y el texto vienen del formulario, y sin
// escapar una nota con <script> sería un agujero en el muro de todos.
func e(s string) string { return html.EscapeString(s) }
