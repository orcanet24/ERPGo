package web

// ai_pins.go — Direcciones IP fijadas de los proveedores de IA (compiladas).
//
// POR QUÉ HAY UNA LISTA EN EL BINARIO Y NO SOLO UNA CONSULTA DNS.
//
// `ai_dns_guard.go` valida que la resolución local del equipo coincida con lo
// que contestan varios resolvers públicos. Eso corta el envenenamiento de DNS,
// pero hace que el ERP dependa de Internet para hablar con la IA: sin red, la
// validación no se puede hacer y la IA no funciona.
//
// La lista compilada invierte el orden: lo primero es si la IP resuelta está en
// la lista, y el DNS público solo se consulta cuando NO está —que es justo el
// caso de que el proveedor haya cambiado sus servidores, o de que la lista
// esté vieja. Un equipo sin Internet sigue hablando con la IA; un equipo con el
// `hosts` apuntando a 127.0.0.1 no la habla aunque tenga todas las líneas de red
// del mundo.
//
// QUÉ NO ES ESTO.
//
// No es una lista blanca de "proveedores permitidos": eso es `aiAllowedVendors`,
// en `ai_rules.go`, y controla a QUIÉN se puede llamar. Esta lista controla a
// DÓNDE: que la IP que resuelve ese host sea una IP que ese host tuvo de verdad.
//
// CÓMO SE MANTIENE.
//
// `tools/ai-pins` regenera este mapa desde los mismos resolvers públicos que usa
// el guardián. No se edita a mano: una IP escrita a dedo aquí es una IP que
// nadie verificó, que es exactamente lo que esta lista pretende evitar.
//
//	go run ./tools/ai-pins            muestra lo que hay ahora y lo que cambiaría
//	go run ./tools/ai-pins -escribir  reescribe ai_pins.go con el consenso actual

import (
	"strings"
)

// aiVendorPins es, para cada host de proveedor de IA, el conjunto de direcciones
// IP que los resolvers públicos coincidieron en dar.
//
// La intersección de tres resolvers independientes es lo que se compila: lo que
// uno solo contesta no se fía. Generado el 2026-10-05.
//
// OJO CON EL TAMAÑO: una CDN cambia de IP sin avisar. Por eso el guardián no
// falla cuando la lista queda vieja, sino que pregunta a los resolvers (ver
// `aiDNSGuard.verifyIPs`). Lo que la lista evita es la SALIDA a Internet en el
// caso normal, no la corrección.
var aiVendorPins = map[string][]string{
	// ---aiVendorPinsGeneratedBegin--- regenerado el 2026-10-06 con tools/ai-pins
	"api.openai.com": {
		"162.159.140.245",  // IPv4
		"172.66.0.243",     // IPv4
		"2606:4700:7::f3",  // IPv6
		"2a06:98c1:58::f3", // IPv6
	},
	"ia.orcanet1724.click": {
		"209.145.62.70", // IPv4
	},
	// ---aiVendorPinsGeneratedEnd---
}

// AIProviderHosts devuelve los hosts de todos los proveedores de IA admitidos.
//
// Lo exporta `tools/ai-pins` para que la herramienta que regenera la lista de
// IPs no mantenga su propia copia de los dominios. Una segunda copia de la
// lista de hosts es una lista que se queda vieja sin que nadie lo note, y la
// herramienta escribiría IPs de un proveedor que ya no existe.
func AIProviderHosts() []string {
	// Las URLs de los proveedores son configurables por el usuario (vienen de
	// la BD, no del binario). No hay hosts fijos que devolver.
	return []string{}
}

// aiPinsForHost devuelve las IPs fijadas de un host, ya normalizadas y
// deduplicadas. Vacío significa "este host no tiene lista compilada", que es un
// caso legítimo: se cae al consenso DNS público.
// AIPinsForHost es `aiPinsForHost` exportada: la usa `tools/ai-pins` para
// comparar lo compilado con lo que dicen hoy los resolvers.
func AIPinsForHost(host string) []string {
	return aiPinsForHost(host)
}

func aiPinsForHost(host string) []string {
	pins, ok := aiVendorPins[normalizeAIHost(host)]
	if !ok || len(pins) == 0 {
		return nil
	}
	return aiIPSetFromStrings(pins)
}

// normalizeAIHost deja el host en minúsculas y sin el punto final, que son las
// dos formas en que un mismo nombre se escribe de más en un `.env`, un hosts o
// una URL.
//
// Sin esto, `AI_VENDOR_ORC24_BASE_URL=https://Ia.Orcanet1724.Click./v1` se
// saltaría la lista sin querer y el ERP dejaría de hablar con la IA.
func normalizeAIHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	return h
}
