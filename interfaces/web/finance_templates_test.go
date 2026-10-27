package web

// Pruebas de que las pantallas nuevas —el tablero de cobranza, el de pagos, los
// canales de aviso y el historial de mensajes— se cargan y se representan.
//
// POR QUÉ SE PRUEBA UNA PLANTILLA. Porque es la cara visible de todo el módulo y
// su fallo no se parece a un fallo de plantilla: si `cartera_list.html` no
// parsea, el módulo de cuentas por cobrar entero devuelve un error de servidor y
// lo que el usuario concluye es «se rompió la cartera». Se comprueba además que
// la página convive con layout.html: un `define` con el mismo nombre que uno del
// layout rompería TODAS las pantallas, no solo esta.
//
// Y NO ES UNA PRUEBA DE HUMO: la primera vez que se corrió encontró que
// `notify_channels.html` pedía `{{.Url}}` a un campo llamado `URL`. En Go, el
// nombre del campo se escribe exacto, así que la pantalla de canales de aviso
// habría dado error de servidor al abrirse —la única pantalla desde la que se
// configura la salida de datos del sistema—.

import (
	"html/template"
	"strings"
	"testing"

	"erpgow"
)

func TestLasPantallasDeCarteraYCanalesSeRenderizan(t *testing.T) {
	canales := []map[string]interface{}{
		{"id": 1, "name": "Correo de cobranza", "kind": "email", "ai_allowed": 1},
	}

	casos := []struct {
		archivo string
		datos   map[string]interface{}
		// Lo que tiene que verse en el cuerpo de la página.
		esperado []string
		// Lo que tiene que estar en el script: los endpoints que usa el navegador.
		// Se revisan aparte porque viven en otro `define` de la misma plantilla.
		esperadoJS []string
	}{
		{
			archivo: "cartera_list.html",
			datos: map[string]interface{}{
				"Canales": canales, "PuedeAvisar": true, "PuedeCobrar": true, "PuedeBorrar": true,
			},
			esperado: []string{
				"Cobranza: indicadores y alertas", "finSemaforos", "finTramos", "finAlertas",
				"modalAviso", "Correo de cobranza (email)", "Destinatario (de la ficha)",
			},
			esperadoJS: []string{"/cartera/panel", "/cartera/escanear", "/cartera/avisar", "/finance/alerta/resolver/"},
		},
		{
			archivo: "pagar_list.html",
			datos: map[string]interface{}{
				"Canales": canales, "PuedeAvisar": true, "PuedePagar": true, "PuedeBorrar": true,
			},
			esperado: []string{
				"Pagos: indicadores y alertas", "apaSemaforos", "apaRetenciones", "apaAlertas",
				"modalAvisoPago", "Retenciones de IVA sin comprobante enviado",
			},
			esperadoJS: []string{"/pagar/panel", "/pagar/escanear", "/pagar/avisar"},
		},
		{
			archivo: "notify_channels.html",
			datos: map[string]interface{}{
				"Nota": "nota de ayuda",
				"Canales": []notifyChannel{{
					ID: 2, Kind: "email", Name: "Correo de cobranza", IsActive: 1, AIAllowed: 1,
					CooldownDays: 3, Host: "smtp.test", Port: 587, FromAddress: "cobranza@test",
					Security: "starttls",
				}},
			},
			esperado: []string{
				"Canales de aviso", "Correo de cobranza", "smtp.test:587", "Permitir agentes",
				"Déjela vacía para no cambiarla",
			},
			esperadoJS: []string{"/settings/notify/save", "/settings/notify/test", "/settings/notify/delete/"},
		},
		{
			archivo: "finance_messages.html",
			datos: map[string]interface{}{
				"Mensajes": []map[string]interface{}{{
					"id": 5, "channel": "Correo", "channel_kind": "email", "recipient": "cliente@test",
					"subject": "Estado de cuenta", "ref_type": "aviso_cobro", "ref_id": "3",
					"status": "enviado", "created_at": "2026-10-27 10:00:00", "created_by": "ana",
				}},
			},
			esperado: []string{"Mensajes enviados", "cliente@test", "Estado de cuenta", "enviado"},
		},
	}

	for _, c := range casos {
		t.Run(c.archivo, func(t *testing.T) {
			tpl, err := template.New(c.archivo).Funcs(templateFuncs(nil)).
				ParseFS(erpgow.Templates(), "templates/"+c.archivo)
			if err != nil {
				t.Fatalf("la plantilla no se puede cargar: %v", err)
			}
			if _, err := tpl.ParseFS(erpgow.Templates(), "templates/layout.html"); err != nil {
				t.Fatalf("la página no convive con layout.html: %v", err)
			}

			salida := representar(t, tpl, "content", c.datos)
			for _, esperado := range c.esperado {
				if !strings.Contains(salida, esperado) {
					t.Errorf("la pantalla no muestra %q: el usuario no vería esa pieza", esperado)
				}
			}
			if len(c.esperadoJS) == 0 {
				return
			}
			script := representar(t, tpl, "pagejs", c.datos)
			for _, esperado := range c.esperadoJS {
				if !strings.Contains(script, esperado) {
					t.Errorf("el script de la pantalla no llama a %q", esperado)
				}
			}
		})
	}
}

// representar ejecuta un `define` de la plantilla con los datos dados. Un
// `define` que no existe devuelve cadena vacía en text/template sin avisar, así
// que se comprueba que el nombre esté declarado: una prueba que ejecuta «nada» y
// pasa es peor que no tenerla.
func representar(t *testing.T, tpl *template.Template, define string, datos map[string]interface{}) string {
	t.Helper()
	if tpl.Lookup(define) == nil {
		t.Fatalf("la plantilla no declara el bloque %q", define)
	}
	var buf strings.Builder
	if err := tpl.ExecuteTemplate(&buf, define, datos); err != nil {
		t.Fatalf("no se puede representar %q: %v", define, err)
	}
	return buf.String()
}
