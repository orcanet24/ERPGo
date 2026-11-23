package web

// proxy_confianza.go — De quién se cree una X-Forwarded-For.
//
// LA REGLA (R8, auditoría atómica 2026-11-22): la IP de una petición es la
// conexión TCP, no lo que diga una cabecera. `X-Forwarded-For` solo vale
// cuando lo escribe un proxy que el OPERADOR declaró de confianza; si no,
// cualquiera firma ser quien quiera: el límite de fuerza bruta del POS se
// saltaba cambiando la cabecera y el ip_address de la auditoría mentía.
//
// Config: PROXY_TRUSTED_IPS="127.0.0.1, ::1, 10.0.0.0/8" en el .env. Vacío
// (el defecto) = no se cree ninguna cabecera: en el despliegue típico de PYME
// no hay proxy, y estar seguro por defecto es el orden correcto.

import (
	"log"
	"net"
	"strings"
	"sync"
)

var proxyConfianza struct {
	mu    sync.Mutex
	redes []*net.IPNet
	solo  []net.IP
}

// configurarProxyConfianza se llama una vez, con el arranque del servidor.
// Un valor mal formado no tumba el ERP: se avisa y esa entrada se ignora —
// quedarse sin lista es quedarse sin cabeceras, que es el lado seguro—.
func configurarProxyConfianza(lista string) {
	proxyConfianza.mu.Lock()
	defer proxyConfianza.mu.Unlock()
	proxyConfianza.redes = nil
	proxyConfianza.solo = nil
	for _, trozo := range strings.Split(lista, ",") {
		trozo = strings.TrimSpace(trozo)
		if trozo == "" {
			continue
		}
		if strings.Contains(trozo, "/") {
			if _, red, err := net.ParseCIDR(trozo); err == nil {
				proxyConfianza.redes = append(proxyConfianza.redes, red)
				continue
			}
			log.Printf("AVISO: PROXY_TRUSTED_IPS ignora %q (no es IP ni CIDR válido)", trozo)
			continue
		}
		if ip := net.ParseIP(trozo); ip != nil {
			proxyConfianza.solo = append(proxyConfianza.solo, ip)
		} else {
			log.Printf("AVISO: PROXY_TRUSTED_IPS ignora %q (no es IP válida)", trozo)
		}
	}
	if len(proxyConfianza.redes) > 0 || len(proxyConfianza.solo) > 0 {
		log.Printf("proxy de confianza declarado: X-Forwarded-For se aceptará solo desde esas redes")
	}
}

func esProxyConfiado(ip net.IP) bool {
	if ip == nil {
		return false
	}
	proxyConfianza.mu.Lock()
	defer proxyConfianza.mu.Unlock()
	for _, solo := range proxyConfianza.solo {
		if solo.Equal(ip) {
			return true
		}
	}
	for _, red := range proxyConfianza.redes {
		if red.Contains(ip) {
			return true
		}
	}
	return false
}
