// addons/hola/main.go — El addon demo del contrato fase 1 (docs/PLAN-ADDONS.md).
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
//      su token (fase 1: solo ping).
//
// Para probarlo: go build -o addons/hola/erp-addon-hola ./addons/hola y
// arrancar el ERP con la carpeta addons/ a la vista.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
// (usuario, empresa, permisos) y el resultado de ping contra el canal. Todo
// texto estático: el addon demo no toca datos (fase 2).
func pantalla(w http.ResponseWriter, r *http.Request) {
	usuario := r.Header.Get("X-ERP-User")
	empresa := r.Header.Get("X-ERP-Tenant")
	permisos := r.Header.Get("X-ERP-Perms")
	cookies := r.Header.Get("Cookie")
	mostrarCookies := "ninguna (el núcleo las retiró, como exige el contrato)"
	if cookies != "" {
		// Si esto aparece, la frontera tiene un agujero: el demo lo dice en
		// carita, no lo calla.
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
<p><a href="/home">← volver al ERP</a> · <a href="/module/addons">Panel de addons</a></p>
</body></html>`,
		htmlQuitar(usuario), htmlQuitar(empresa), htmlQuitar(permisos), mostrarCookies, htmlQuitar(ping))
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
