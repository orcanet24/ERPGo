// addons/hola/main.go — El addon demo del contrato fase 2 (docs/PLAN-ADDONS.md).
//
// Qué demuestra, sin nada más:
//
//   1. Que un addon es un binario INDEPENDIENTE: este main.go se compila aparte
//      (erp-addon-hola) y no importa nada del ERP.
//   2. Que el núcleo le entrega por entorno el token efímero y el puerto local
//      donde debe escuchar (ERP_ADDON_TOKEN / ERP_ADDON_PORT).
//   3. Que su pantalla NUNCA recibe las cookies del ERP: recibe las cabeceras
//      X-ERP-* que el núcleo ya autenticó.
//   4. Que puede verificar el canal de negocio llamando /__addon/hola/ping con
//      su token.
//   5. (Fase 2) Que puede pedir una ESCRITURA de negocio por el canal:
//      POST /__addon/hola/mensaje.registrar con cuerpo JSON, token y el
//      contexto X-ERP-* que la pantalla recibió (el addon no inventa el
//      usuario: reenvía el contexto ya autenticado). El núcleo valida verbo,
//      sesión, empresa y permiso, y la aplica por el orquestador.
//
// Para probarlo: go build -o addons/hola/erp-addon-hola ./addons/hola y
// arrancar el ERP con la carpeta addons/ a la vista.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

const (
	healthOK = `{"status":"ok","addon":"hola"}`
)

var (
	token string
	core  string
)

func main() {
	token = os.Getenv("ERP_ADDON_TOKEN")
	core = os.Getenv("ERP_CORE_URL")
	porte := os.Getenv("ERP_ADDON_PORT")
	if porte == "" || token == "" {
		fmt.Fprintln(os.Stderr, "hola: sin ERP_ADDON_PORT o ERP_ADDON_TOKEN: este binario lo lanza el ERP, no un humano.")
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(healthOK))
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		// Solo el núcleo, con su token, manda este verbo: es el apagado
		// ordenado del ciclo de vida (docs/PLAN-ADDONS.md §3.5).
		if r.Header.Get("X-ERP-Addon-Token") != token {
			http.Error(w, "Sin el token de esta sesión no hay apagado.", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"apagando"}`))
		// El apagado real lo hace el srv.Shutdown del goroutine de abajo.
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(0)
		}()
	})
	mux.HandleFunc("/hola", pantalla)
	mux.HandleFunc("/hola/mensaje", mensaje)

	srv := &http.Server{Addr: "127.0.0.1:" + porte, Handler: mux}
	log.Printf("hola: addon activo en 127.0.0.1:%s", porte)

	// Ctrl+C local y apagado ordenado por el núcleo terminan igual.
	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt)
	go func() {
		<-parar
		_ = srv.Shutdown(context.Background())
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Printf("hola: servidor terminado: %v", err)
	}
}

// pantalla es la página del addon. Muestra el contexto que le entrega el ERP
// (usuario, empresa, permisos), el resultado de ping contra el canal y el
// formulario que manda una escritura por el canal (fase 2) con el resultado
// de la última, si la hubo.
func pantalla(w http.ResponseWriter, r *http.Request) {
	renderizarPantalla(w, r, "", "")
}

// mensaje es la puerta de escritura del demo: recibe el POST del formulario
// (campo mensaje) y lo hace llegar al núcleo por el canal de negocio
// /__addon/hola/mensaje.registrar, con el token de esta sesión y las
// cabeceras X-ERP-* que la pantalla recibió (el addon no inventa el usuario:
// reenvía el contexto ya autenticado; quién es quién lo decide el núcleo).
func mensaje(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/hola", http.StatusSeeOther)
		return
	}
	texto := strings.TrimSpace(r.FormValue("mensaje"))
	if texto == "" {
		renderizarPantalla(w, r, "Escribe un mensaje: el campo llegó vacío.", "")
		return
	}
	if core == "" {
		renderizarPantalla(w, r, "Este binario lo lanza el ERP: le falta ERP_CORE_URL en el entorno.", "")
		return
	}
	cuerpo, _ := json.Marshal(map[string]interface{}{"mensaje": texto})
	req, err := http.NewRequest(http.MethodPost, core+"/__addon/hola/mensaje.registrar", bytes.NewReader(cuerpo))
	if err != nil {
		renderizarPantalla(w, r, "No se pudo preparar la petición al canal: "+err.Error(), "")
		return
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-ERP-Addon-Token", token)
	req.Header.Set("X-ERP-User", r.Header.Get("X-ERP-User"))
	req.Header.Set("X-ERP-Tenant", r.Header.Get("X-ERP-Tenant"))
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		renderizarPantalla(w, r, "El núcleo no respondió: "+err.Error(), "")
		return
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		renderizarPantalla(w, r, "No se pudo leer la respuesta del canal: "+err.Error(), "")
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
		renderizarPantalla(w, r, fmt.Sprintf("El canal respondió HTTP %d: %s", resp.StatusCode, salida.Error), "")
		return
	}
	renderizarPantalla(w, r, "", fmt.Sprintf(
		"Mensaje guardado por el orquestador (registro %s, correlativo %d, petición %s%s).",
		salida.RegistroID, salida.Correlativo, salida.Peticion,
		condicional(salida.Duplicado, "; ya estaba registrado: idempotencia", "")))
}

// renderizarPantalla pinta la pantalla con un estado opcional:
// aviso (algo falló, en rojo) o resultado (la escritura quedó aplicada).
func renderizarPantalla(w http.ResponseWriter, r *http.Request, aviso, resultado string) {
	usuario := r.Header.Get("X-ERP-User")
	empresa := r.Header.Get("X-ERP-Tenant")
	permisos := r.Header.Get("X-ERP-Perms")
	cookies := r.Header.Get("Cookie")
	mostrarCookies := "ninguna (el núcleo las retiró, como exige el contrato)"
	if cookies != "" {
		// Si esto aparece, la frontera tiene un agujero: el demo lo dice en
		// la cara, no lo calla.
		mostrarCookies = "⚠ HAY COOKIES: la frontera no está limpiando"
	}

	ping := "sin respuesta"
	if core != "" {
		cliente := &http.Client{Timeout: 3 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, core+"/__addon/hola/ping", nil)
		req.Header.Set("X-ERP-Addon-Token", token)
		if resp, err := cliente.Do(req); err == nil {
			if resp.StatusCode == http.StatusOK {
				ping = "OK"
			} else {
				ping = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			_ = resp.Body.Close()
		} else {
			ping = "no se pudo contactar al núcleo"
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>Hola, addon</title></head>
<body style="font-family:sans-serif;margin:2em">
<h1>Hola, addon</h1>
<p>Esta pantalla vive en el <strong>binario del addon</strong>, no en el núcleo del ERP.
Se sirve por el proxy inverso con la sesión del ERP ya validada.</p>
<ul>
	<li><strong>Usuario</strong>: %s</li>
	<li><strong>Empresa activa</strong>: %s</li>
	<li><strong>Permisos del manifiesto concedidos</strong>: %s</li>
	<li><strong>Cookies del ERP recibidas</strong>: %s</li>
	<li><strong>Canal de negocio /__addon/hola/ping</strong>: %s</li>
</ul>
<p style="color:#b00">%s</p>
<p style="color:#060">%s</p>
<form method="post" action="/hola/mensaje" style="margin:1.5em 0">
	<label>Mensaje para la tabla propia del addon (hola_mensajes):<br>
	<textarea name="mensaje" rows="3" cols="50" maxlength="512"></textarea></label><br>
	<button type="submit">Guardar por el canal (mensaje.registrar)</button>
</form>
<p><a href="/home">← volver al ERP</a> · <a href="/module/addons">Panel de addons</a></p>
</body></html>`,
		htmlQuitar(usuario), htmlQuitar(empresa), htmlQuitar(permisos), mostrarCookies, htmlQuitar(ping),
		htmlQuitar(aviso), htmlQuitar(resultado))
}

func condicional(condicion bool, a, b string) string {
	if condicion {
		return a
	}
	return b
}

func htmlQuitar(s string) string {
	reemplazos := map[string]string{
		"&": "&amp;", "<": "&lt;", ">": "&gt;", `"`: "&#34;", "'": "&#39;",
	}
	out := ""
	for _, c := range s {
		if v, ok := reemplazos[string(c)]; ok {
			out += v
		} else {
			out += string(c)
		}
	}
	return out
}
