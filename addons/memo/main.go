// addons/memo/main.go — Módulo Memo, fases A y B (docs/PLAN-MEMO.md).
//
// QUÉ HAY AQUÍ HOY. El muro: publicar una nota y ver, en orden, las notas que
// este usuario tiene derecho a ver — las suyas (sean privadas o no), las de
// visibilidad «empresa», las que otra persona le compartió a él en concreto y
// las que le compartió UN GRUPO del que forma parte. Además están los grupos
// de trabajo: se crean y se rellenan desde esta misma pantalla. Los
// comentarios, los adjuntos, el chat y el calendario llegan con las fases C a
// F del plan.
//
// CÓMO SE CUMPLE LA VISIBILIDAD. La consulta de lectura es el candado: la
// filtración no se decide en la pantalla sino en el WHERE, contra la base de
// la empresa activa que el núcleo indicó en X-ERP-DB. La gente (users) vive
// en la base de identidad que trae X-ERP-Auth-DB: de ahí salen los nombres
// para pintar y el listado con el que se comparte. Un grupo NO se expande a
// filas cuando se comparte: la pertenencia se resuelve en cada lectura, y por
// eso quien deja el grupo deja de ver las notas sin que nadie las reescriba.
//
// CÓMO SE ESCRIBE. Este proceso no hace INSERT: manda el formulario a
// POST /__addon/memo/{verbo} con el token efímero de la sesión y el contexto
// X-ERP-* que el proxy ya autenticó; el núcleo valida verbo, sesión, empresa
// y permiso, y baja por el orquestador (journal + audit_log).
//
// El formulario lleva el campo csrf_token con el token que el núcleo entregó
// en X-ERP-CSRF: enforceCSRF corre ANTES del proxy y sin ese campo el POST
// sale 403.
//
// Compilar: go build -o addons/memo/erp-addon-memo ./addons/memo
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
	tituloPantalla   = "Memo"
	maxTitulo        = 200
	maxTexto         = 20000
	maxNombreGrupo   = 120
	maxMiembrosGrupo = 100
	maxGruposNota    = 10
)

var (
	token         string
	core          string
	dsnLectura    string
	reBase        = regexp.MustCompile(`^[A-Za-z0-9_$-]+$`)
	visibilidades = map[string]bool{"privada": true, "empresa": true, "personas": true, "grupo": true}
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
	mux.HandleFunc("/memo/grupo", crearGrupo)
	mux.HandleFunc("/memo/grupo/miembros", guardarMiembros)

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

// grupo es un grupo de trabajo de la empresa con sus miembros HOY.
type grupo struct {
	ID       string
	Nombre   string
	Miembros []string
}

// leerGrupos trae los grupos de la empresa y sus miembros. Los nombres de las
// personas no se piden aquí: esta es la base de NEGOCIO y no guarda nombres
// de usuarios (viven en la de identidad), así que se resuelven con el mapa
// que ya se armó con `users`.
func leerGrupos(ctx context.Context, db *sql.DB, empresa string) ([]grupo, error) {
	filas, err := db.QueryContext(ctx,
		`SELECT id, nombre FROM memo_grupos WHERE empresa = ? ORDER BY nombre`, empresa)
	if err != nil {
		return nil, err
	}
	salida := []grupo{}
	posición := map[string]int{}
	for filas.Next() {
		var g grupo
		if err := filas.Scan(&g.ID, &g.Nombre); err != nil {
			filas.Close()
			return nil, err
		}
		posición[g.ID] = len(salida)
		salida = append(salida, g)
	}
	if err := filas.Err(); err != nil {
		filas.Close()
		return nil, err
	}
	_ = filas.Close()

	miembros, err := db.QueryContext(ctx,
		`SELECT grupo_id, persona_id FROM memo_grupo_miembros WHERE empresa = ?`, empresa)
	if err != nil {
		return nil, err
	}
	defer miembros.Close()
	for miembros.Next() {
		var gid, pid string
		if err := miembros.Scan(&gid, &pid); err != nil {
			return nil, err
		}
		if i, ok := posición[gid]; ok {
			salida[i].Miembros = append(salida[i].Miembros, pid)
		}
	}
	return salida, miembros.Err()
}

// selectorDeGente arma un select de personas. Va SIEMPRE en el formulario,
// también sin gente cargada ni sin grupos: un campo que desaparece deja al
// usuario con un formulario a medias y sin saber qué falló. `marcados` deja
// preseleccionados a los que ya están (al editar miembros de un grupo).
func selectorDeGente(nombre string, lista []gente, marcados map[string]bool, fallo string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<select name="%s" multiple size="6">`, e(nombre))
	switch {
	case fallo != "":
		fmt.Fprintf(&b, `<option disabled selected>No se pudo cargar la gente: %s</option>`, e(fallo))
	case len(lista) == 0:
		b.WriteString(`<option disabled selected>No hay nadie con sesión en esta empresa</option>`)
	default:
		for _, g := range lista {
			if marcados[g.ID] {
				fmt.Fprintf(&b, `<option value="%s" selected>%s</option>`, e(g.ID), e(g.Nombre))
			} else {
				fmt.Fprintf(&b, `<option value="%s">%s</option>`, e(g.ID), e(g.Nombre))
			}
		}
	}
	b.WriteString(`</select>`)
	return b.String()
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
//   - lo que OTRO le compartió a este usuario en concreto (destino «persona»);
//   - lo que le compartió UN GRUPO del que HOY es miembro (destino «grupo»
//     cruzado con los miembros actuales).
//
// Esa última rama es la razón de que salirse del grupo surta efecto sin
// tocar ninguna nota: aquí no se guarda quién ve qué, se decide CADA vez.
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
		                WHERE d.nota_id = n.id AND d.destino_tipo = 'persona'
		                  AND d.destino_id = ?)
		     OR EXISTS (SELECT 1 FROM memo_destinatarios d
		                JOIN memo_grupo_miembros m
		                  ON m.grupo_id = d.destino_id AND m.empresa = d.empresa
		                WHERE d.nota_id = n.id AND d.destino_tipo = 'grupo'
		                  AND m.persona_id = ?) )
		ORDER BY n.creada DESC
		LIMIT 50`

func leerNotas(ctx context.Context, db *sql.DB, empresa, usuario string) ([]nota, error) {
	rows, err := db.QueryContext(ctx, consultaMuro, empresa, usuario, usuario, usuario)
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

// respuestaCanal es lo que el núcleo devuelve al aplicar un verbo. Los campos
// de detalle (compartido, avisados, miembros…) los rellena el handler de
// negocio; status y error los pone la frontera.
type respuestaCanal struct {
	Status     int    `json:"status"`
	Error      string `json:"error"`
	Peticion   string `json:"peticion"`
	RegistroID string `json:"registro_id"`
	Compartido int    `json:"compartido"`
	Avisados   int    `json:"avisados"`
	Miembros   int    `json:"miembros"`
	Entraron   int    `json:"entraron"`
	Bajaron    int    `json:"bajaron"`
	Nombre     string `json:"nombre"`
}

// enviarAlNucleo manda UN verbo al canal con el contexto de ESTA sesión. Este
// proceso jamás escribe en la base: pide, y el núcleo aplica. Devuelve la
// respuesta ya leída y su HTTP, para que quien llama decida qué contarle al
// usuario.
func enviarAlNucleo(r *http.Request, verb string, cuerpo map[string]interface{}) (*respuestaCanal, int, error) {
	if core == "" {
		return &respuestaCanal{}, 0, fmt.Errorf("este binario lo lanza el ERP: le falta ERP_CORE_URL en el entorno")
	}
	buf, err := json.Marshal(cuerpo)
	if err != nil {
		return &respuestaCanal{}, 0, fmt.Errorf("no se pudo armar el cuerpo del verbo %s: %w", verb, err)
	}
	req, err := http.NewRequest(http.MethodPost, core+"/__addon/memo/"+verb, bytes.NewReader(buf))
	if err != nil {
		return &respuestaCanal{}, 0, fmt.Errorf("no se pudo preparar la petición al canal: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-User", r.Header.Get("X-ERP-User"))
	req.Header.Set("X-ERP-Tenant", r.Header.Get("X-ERP-Tenant"))
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return &respuestaCanal{}, 0, fmt.Errorf("el núcleo no respondió: %w", err)
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &respuestaCanal{}, resp.StatusCode, fmt.Errorf("no se pudo leer la respuesta del canal: %w", err)
	}
	salida := &respuestaCanal{}
	_ = json.Unmarshal(datos, salida)
	return salida, resp.StatusCode, nil
}

// avisoDeCanal traduce una respuesta fallida al mensaje de la pantalla.
// Devuelve vacío solo cuando el verbo se aplicó de verdad.
func avisoDeCanal(codigo int, res *respuestaCanal) string {
	if res == nil {
		return "El canal no devolvió respuesta."
	}
	if codigo != http.StatusOK || res.Status != 1 {
		if res.Error != "" {
			return fmt.Sprintf("El canal respondió HTTP %d: %s", codigo, res.Error)
		}
		return fmt.Sprintf("El canal respondió HTTP %d sin motivo legible.", codigo)
	}
	return ""
}

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
	grupos := r.Form["grupos"]

	if titulo == "" || texto == "" {
		muroConAviso(w, r, "El título y el texto de la nota son obligatorios.", "")
		return
	}
	if len([]rune(titulo)) > maxTitulo || len([]rune(texto)) > maxTexto {
		muroConAviso(w, r, fmt.Sprintf("La nota no pasa los límites (título %d y texto %d caracteres).", maxTitulo, maxTexto), "")
		return
	}
	if !visibilidades[visibilidad] {
		muroConAviso(w, r, "La visibilidad debe ser privada, empresa, personas o grupo.", "")
		return
	}
	if visibilidad == "personas" && len(destinatarios) == 0 {
		muroConAviso(w, r, "Compartir con personas exige elegir al menos a una.", "")
		return
	}
	// Un grupo vacío es la misma mentira que «personas» sin personas: se
	// publicaría para nadie. El grupo y sus miembros los valida además el
	// núcleo —el candado no está solo en la pantalla—, pero aquí se corta
	// antes de hacer un viaje de red inútil.
	if visibilidad == "grupo" && len(grupos) == 0 {
		muroConAviso(w, r, "Compartir con un grupo exige elegir al menos uno.", "")
		return
	}
	if visibilidad == "grupo" && len(grupos) > maxGruposNota {
		muroConAviso(w, r, fmt.Sprintf("Una nota no se reparte con más de %d grupos a la vez (%d elegidos).", maxGruposNota, len(grupos)), "")
		return
	}

	salida, codigo, err := enviarAlNucleo(r, "nota.crear", map[string]interface{}{
		"titulo": titulo, "texto": texto, "visibilidad": visibilidad,
		"destinatarios": destinatarios, "grupos": grupos,
	})
	if err != nil {
		muroConAviso(w, r, err.Error(), "")
		return
	}
	if av := avisoDeCanal(codigo, salida); av != "" {
		muroConAviso(w, r, av, "")
		return
	}
	detalle := ""
	if visibilidad == "grupo" {
		detalle = fmt.Sprintf(" Repartida en %d grupo(s); %d persona(s) avisada(s).", salida.Compartido, salida.Avisados)
	} else if salida.Compartido > 0 {
		detalle = fmt.Sprintf(" Compartida con %d persona(s).", salida.Compartido)
	}
	muroConAviso(w, r, "", fmt.Sprintf("Nota publicada (registro %s, petición %s).%s", salida.RegistroID, salida.Peticion, detalle))
}

// crearGrupo recoge el formulario de grupos y lo manda al verbo grupo.crear.
func crearGrupo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/memo", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		muroConAviso(w, r, "El formulario no se pudo leer: "+err.Error(), "")
		return
	}
	nombre := strings.TrimSpace(r.FormValue("nombre"))
	miembros := r.Form["miembros"]
	if nombre == "" {
		muroConAviso(w, r, "El nombre del grupo es obligatorio.", "")
		return
	}
	if len([]rune(nombre)) > maxNombreGrupo {
		muroConAviso(w, r, fmt.Sprintf("El nombre del grupo no pasa de %d caracteres.", maxNombreGrupo), "")
		return
	}
	if len(miembros) == 0 {
		muroConAviso(w, r, "Un grupo sin miembros no comparte nada: elige al menos a una persona.", "")
		return
	}
	if len(miembros) > maxMiembrosGrupo {
		muroConAviso(w, r, fmt.Sprintf("Un grupo no pasa de %d miembros (%d elegidos).", maxMiembrosGrupo, len(miembros)), "")
		return
	}

	salida, codigo, err := enviarAlNucleo(r, "grupo.crear", map[string]interface{}{
		"nombre": nombre, "miembros": miembros,
	})
	if err != nil {
		muroConAviso(w, r, err.Error(), "")
		return
	}
	if av := avisoDeCanal(codigo, salida); av != "" {
		muroConAviso(w, r, av, "")
		return
	}
	muroConAviso(w, r, "", fmt.Sprintf("Grupo «%s» creado con %d miembro(s).", nombre, salida.Miembros))
}

// guardarMiembros reescribe la lista de miembros de un grupo con la del
// formulario. Puede venir vacía: sacar a todo el mundo es legítimo (el grupo
// queda dormido y, mientras tanto, no reparte nada).
func guardarMiembros(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/memo", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		muroConAviso(w, r, "El formulario no se pudo leer: "+err.Error(), "")
		return
	}
	grupoID := strings.TrimSpace(r.FormValue("grupo_id"))
	miembros := r.Form["miembros"]
	if grupoID == "" {
		muroConAviso(w, r, "Falta el grupo a actualizar.", "")
		return
	}
	if len(miembros) > maxMiembrosGrupo {
		muroConAviso(w, r, fmt.Sprintf("Un grupo no pasa de %d miembros (%d elegidos).", maxMiembrosGrupo, len(miembros)), "")
		return
	}

	salida, codigo, err := enviarAlNucleo(r, "grupo.miembros", map[string]interface{}{
		"grupo_id": grupoID, "miembros": miembros,
	})
	if err != nil {
		muroConAviso(w, r, err.Error(), "")
		return
	}
	if av := avisoDeCanal(codigo, salida); av != "" {
		muroConAviso(w, r, av, "")
		return
	}
	muroConAviso(w, r, "", fmt.Sprintf("Miembros de «%s» guardados: %d en el grupo (%d entraron, %d salieron).",
		salida.Nombre, salida.Miembros, salida.Entraron, salida.Bajaron))
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
	gruposLista := []grupo{}
	var errGrupos string

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

	// Las dos lecturas de la EMPRESA salen de la misma conexión: muro y
	// grupos son la misma base y abrir dos solo para mirar dos tablas propias
	// es dos pings y dos cierres de más.
	if db, err := conectarLectura(baseEmpresa, "X-ERP-DB"); err != nil {
		errNotas = err.Error()
		errGrupos = err.Error()
	} else {
		if ns, err := leerNotas(ctx, db, r.Header.Get("X-ERP-Tenant"), usuario); err != nil {
			errNotas = err.Error()
		} else {
			notas = ns
		}
		if gs, err := leerGrupos(ctx, db, r.Header.Get("X-ERP-Tenant")); err != nil {
			errGrupos = err.Error()
		} else {
			gruposLista = gs
		}
		_ = db.Close()
	}

	nombres := nombresDevuelve(genteLista)

	// Los selectores van SIEMPRE en el formulario, también sin gente ni sin
	// grupos cargados: un campo que desaparece deja al usuario con un
	// formulario a medias y sin saber qué falló.
	opcionesPersonas := selectorDeGente("destinatarios", genteLista, nil, errGente)
	opcionesMiembros := selectorDeGente("miembros", genteLista, nil, errGente)

	var opcionesGrupos strings.Builder
	opcionesGrupos.WriteString(`<select name="grupos" multiple size="4">`)
	switch {
	case errGrupos != "":
		fmt.Fprintf(&opcionesGrupos, `<option disabled selected>No se pudieron leer los grupos: %s</option>`, e(errGrupos))
	case len(gruposLista) == 0:
		opcionesGrupos.WriteString(`<option disabled selected>Todavía no hay grupos: créalo más abajo</option>`)
	default:
		for _, g := range gruposLista {
			fmt.Fprintf(&opcionesGrupos, `<option value="%s">%s (%d)</option>`, e(g.ID), e(g.Nombre), len(g.Miembros))
		}
	}
	opcionesGrupos.WriteString(`</select>`)

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
			etiqueta := map[string]string{"privada": "privada", "empresa": "empresa", "personas": "compartida", "grupo": "compartida con grupo"}[n.Visibilidad]
			if etiqueta == "" {
				etiqueta = n.Visibilidad
			}
			fmt.Fprintf(&muroHTML, `<article class="nota"><h4>%s</h4><p>%s</p>
<footer>%s · %s · visibilidad %s</footer></article>`,
				e(n.Titulo), e(n.Texto), e(quien), e(n.Creada), e(etiqueta))
		}
	}

	// La lista de grupos, con sus miembros HOY y un formulario por grupo
	// para reescribirlos. Los nombres salen del mapa de identidad: en la base
	// de la empresa no hay nombres de usuarios, solo ids.
	var gruposHTML strings.Builder
	switch {
	case errGrupos != "":
		fmt.Fprintf(&gruposHTML, `<p class="mal">No se pudieron leer los grupos: %s</p>`, e(errGrupos))
	case len(gruposLista) == 0:
		gruposHTML.WriteString(`<p class="soso">Todavía no hay grupos. Crea el primero con el formulario de abajo.</p>`)
	default:
		for _, g := range gruposLista {
			quienes := make([]string, 0, len(g.Miembros))
			marcados := map[string]bool{}
			for _, pid := range g.Miembros {
				marcados[pid] = true
				quien := nombres[pid]
				if quien == "" {
					quien = pid
				}
				quienes = append(quienes, quien)
			}
			lista := strings.Join(quienes, ", ")
			if lista == "" {
				lista = "sin miembros: no reparte nada hasta que alguien entre"
			}
			fmt.Fprintf(&gruposHTML, `<article class="nota"><h4>%s</h4><p class="soso">%s</p>
<details><summary>Editar miembros (%d)</summary>
<form method="post" action="/memo/grupo/miembros">
 <input type="hidden" name="csrf_token" value="%s">
 <input type="hidden" name="grupo_id" value="%s">
 <label>Miembros del grupo<br>%s</label>
 <button type="submit">Guardar miembros</button>
</form></details></article>`,
				e(g.Nombre), e(lista), len(g.Miembros),
				e(r.Header.Get("X-ERP-CSRF")), e(g.ID),
				selectorDeGente("miembros", genteLista, marcados, errGente))
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
      <option value="grupo">Compartir con un grupo…</option>
     </select></label>
    <label>Compartir con<br>%s</label>
   </div>
   <label>Compartir con grupos<br>%s</label>
   <button type="submit">Publicar nota</button>
  </form>
 </div>

 <div class="caja">
  <h3>Grupos de trabajo</h3>
  <p class="soso">Un grupo reparte la nota con su gente de una vez. La regla se resuelve en CADA lectura: quien deja el grupo deja de ver lo que se le compartió con él, sin que se reescriba ninguna nota.</p>
  %s
  <form method="post" action="/memo/grupo">
   <input type="hidden" name="csrf_token" value="%s">
   <label>Nombre del grupo<br><input type="text" name="nombre" maxlength="%d" required></label>
   <label>Miembros<br>%s</label>
   <button type="submit">Crear grupo</button>
  </form>
 </div>

 <div class="caja">
  <h3>Muro</h3>
  %s
 </div>

 <p><a href="/home">← volver al ERP</a> · <a href="/module/addons">Configuración → Módulos</a></p>
</main>
<script>
 // Solo lleva el formulario el selector que corresponde a la visibilidad
 // elegida: el candado está en el núcleo, esto es solo no pedirle a la gente
 // lo que no hace falta ni enviar listas que no significan nada. Los
 // selectores de los grupos de abajo no se tocan (son de OTRO formulario).
 var form = document.querySelector('form[action="/memo/nota"]');
 var sel = document.getElementById('visibilidad');
 function aplicarVisibilidad() {
   if (!form || !sel) return;
   var personas = form.querySelector('select[name=destinatarios]');
   var grupos = form.querySelector('select[name=grupos]');
   if (personas) personas.disabled = (sel.value !== 'personas');
   if (grupos) grupos.disabled = (sel.value !== 'grupo');
 }
 if (sel) sel.addEventListener('change', aplicarVisibilidad);
 aplicarVisibilidad();
</script>
</body></html>`,
		tituloPantalla, tituloPantalla,
		e(usuario), e(r.Header.Get("X-ERP-Tenant")), e(baseEmpresa), e(baseAuth),
		e(r.Header.Get("X-ERP-Perms")), mostrarCookies,
		e(falla), e(exito),
		e(r.Header.Get("X-ERP-CSRF")), maxTitulo, maxTexto,
		opcionesPersonas,
		opcionesGrupos.String(),
		gruposHTML.String(),
		e(r.Header.Get("X-ERP-CSRF")), maxNombreGrupo,
		opcionesMiembros,
		muroHTML.String())
}

// e escapa para HTML: el título y el texto vienen del formulario, y sin
// escapar una nota con <script> sería un agujero en el muro de todos.
func e(s string) string { return html.EscapeString(s) }
