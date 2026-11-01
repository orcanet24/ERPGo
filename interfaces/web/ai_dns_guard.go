package web

// ai-dns-guard.go — Validación de integridad DNS antes de hablar con un
// proveedor de IA (anti DNS-hijacking / anti MITM silencioso).
//
// Idea: antes de enviar una sola petición de IA, el host del proveedor se
// consulta por DOS resolvers públicos independientes vía DNS-over-HTTPS
// (Cloudflare y Google). Lo que AMBOS conocen es el "consenso público":
// una IP que ninguno de los dos haya visto jamás es casi seguro una
// inyección local. La resolución local del sistema se verifica contra esa
// credencial (debe estar incluida en la UNIÓN de las respuestas).
//
// POR QUÉ UNIÓN Y NO INTERSECCIÓN: los proveedores modernos viven detrás de
// CDNs con DNS geográfico (p. ej. api.longcat.ai → gccdn.net), y cada
// resolver recibe IPs de la región donde consulta el resolver. Las dos
// respuestas legítimas pueden NO compartir ni una sola IP: exigir la
// intersección marcaba proveedores perfectamente buenos como "secuestro".
// La unión mantiene el argumento de seguridad: si el DNS local está
// envenenado, la IP falsa no habrá sido servida por NINGÚN resolver público
// y el check corta; una edge legítima del CDN sí estará en al menos uno.
//
// Por qué importa: el cliente HTTP de Go valida el certificado TLS contra el
// nombre de host, así que un envenenamiento de DNS "a pelo" ya falla. Pero en
// redes corporativas es legal y habitual instalar un proxy de inspección con
// una CA de confianza: ahí el TLS pasa y el ERP acaba entregando los datos
// del negocio al proxy sin que nadie lo haya pedido. Esta validación detecta
// ese caso (el proxy no está en el consenso público) y corta la comunicación.
//
// TOCTOU: validar el DNS y luego conectar con el resolver normal dejaría una
// ventana en la que el DNS puede cambiar. Por eso, tras validar, la conexión
// se hace con DialContext FIJADO a las IPs VERIFICADAS DE LA RESOLUCIÓN
// LOCAL (las que el sistema ya era capaz de resolver) y el TLS se sigue
// validando contra el dominio original (ServerName intacto): el atacante
// tendría que controlar además el certificado. Pinning a la resolución local
// y no a las IPs remotas de los DoH: es la ruta que el CDN ya eligió para
// esta máquina; forzar las IPs de otra región ralentiza y desincroniza.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// aiDNSGuardTimeout acota cada consulta DoH y el handshake.
	aiDNSGuardTimeout = 10 * time.Second
	// aiDNSCacheTTL evita 2 consultas DoH por cada petición de IA.
	aiDNSCacheTTL = 10 * time.Minute
	// aiDNSGuardEnv permite desactivar el guardián ante un falso positivo
	// (ERPGO_AI_DNS_GUARD=off). Es una válvula de escape, no un interruptor
	// de negocio: dejarla abierta equivaldría a no validar nada.
	aiDNSGuardEnv = "ERPGO_AI_DNS_GUARD"
)

// aiDoHEndpoint es un resolver DNS-over-HTTPS con respuesta JSON (RFC 8484).
type aiDoHEndpoint struct {
	Name string
	URL  string // %s = nombre, %s = tipo (A / AAAA)
}

// aiDoHEndpoints: dos proveedores distintos, independientes entre sí. Basta
// con dos: la intersección ya es un consenso sólido; con tres solo se
// multiplica la latencia antes de cada sesión de IA.
var aiDoHEndpoints = []aiDoHEndpoint{
	{Name: "cloudflare", URL: "https://cloudflare-dns.com/dns-query?name=%s&type=%s"},
	{Name: "google", URL: "https://dns.google/resolve?name=%s&type=%s"},
}

// aiDoHResponse es el sobre común de Cloudflare y Google.
type aiDoHResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// aiDNSGuardCacheEntry memoriza las IPs verificadas de la resolución local
// por host durante el TTL.
type aiDNSGuardCacheEntry struct {
	ips     []string
	expires time.Time
}

// aiDNSGuard realiza la validación. Los resolvers son inyectables para poder
// probar la lógica sin red.
type aiDNSGuard struct {
	endpoints []aiDoHEndpoint
	// localResolve replica net.DefaultResolver.LookupIPAddr.
	localResolve func(ctx context.Context, host string) ([]net.IP, error)
	doh          *http.Client

	mu    sync.Mutex
	cache map[string]aiDNSGuardCacheEntry
}

var (
	aiGuardOnce sync.Once
	aiGuardInst *aiDNSGuard
)

// aiDNSGuardSingleton devuelve el guardián compartido por todo el proceso.
func aiDNSGuardSingleton() *aiDNSGuard {
	aiGuardOnce.Do(func() {
		aiGuardInst = &aiDNSGuard{
			endpoints: aiDoHEndpoints,
			localResolve: func(ctx context.Context, host string) ([]net.IP, error) {
				addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}
				ips := make([]net.IP, 0, len(addrs))
				for _, a := range addrs {
					ips = append(ips, a.IP)
				}
				return ips, nil
			},
			doh:   &http.Client{Timeout: aiDNSGuardTimeout},
			cache: map[string]aiDNSGuardCacheEntry{},
		}
	})
	return aiGuardInst
}

// ---------- helpers de conjuntos de IPs ----------

// aiIPSet normaliza, deduplica y ordena IPs (string canónico, IPv4 sin ceros).
func aiIPSet(ips []net.IP) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		var s string
		if v4 := ip.To4(); v4 != nil {
			s = v4.String()
		} else {
			s = ip.String()
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// aiIPSetFromStrings normaliza un set ya textual (salida de DoH).
func aiIPSetFromStrings(ips []string) []string {
	parsed := make([]net.IP, 0, len(ips))
	for _, s := range ips {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
			parsed = append(parsed, ip)
		}
	}
	return aiIPSet(parsed)
}

// aiMissing devuelve los elementos de sub que NO aparecen en super.
func aiMissing(sub, super []string) []string {
	set := map[string]struct{}{}
	for _, s := range super {
		set[s] = struct{}{}
	}
	out := []string{}
	for _, s := range sub {
		if _, ok := set[s]; !ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// ---------- resolución DoH ----------

// resolveDoH consulta un resolver concreto y devuelve su set de IPs. Se
// consulta por endpoint (no mezclado) para que el consenso se calcule después
// entre resolvers realmente independientes.
func (g *aiDNSGuard) resolveDoH(ctx context.Context, ep aiDoHEndpoint, host string) ([]string, error) {
	var lastErr error
	// Ambas familias: un proyecto puede publicar solo IPv6.
	for _, qtype := range []string{"A", "AAAA"} {
		endpoint := fmt.Sprintf(ep.URL, host, qtype)
		ips, err := g.resolveDoHOne(ctx, ep.Name, endpoint)
		if err != nil {
			lastErr = err
			continue
		}
		if len(ips) > 0 {
			return ips, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("sin respuesta")
	}
	return nil, fmt.Errorf("no se pudo resolver %q por DNS-over-HTTPS: %v", host, lastErr)
}

// resolveDoHOne hace una consulta DoH concreta y devuelve sus IPs.
func (g *aiDNSGuard) resolveDoHOne(ctx context.Context, epName, endpoint string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", epName, err)
	}
	req.Header.Set("Accept", "application/dns-json")

	client := g.doh
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", epName, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", epName, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", epName, resp.StatusCode)
	}

	var parsed aiDoHResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%s: respuesta ilegible", epName)
	}
	// 0 = NOERROR.
	if parsed.Status != 0 {
		return nil, fmt.Errorf("%s: DNS status %d", epName, parsed.Status)
	}

	ips := []string{}
	for _, a := range parsed.Answer {
		// 1 = A, 28 = AAAA. Los CNAME (5) se resuelven y se descartan.
		if a.Type != 1 && a.Type != 28 {
			continue
		}
		if ip := net.ParseIP(strings.TrimSpace(a.Data)); ip != nil {
			ips = append(ips, ip.String())
		}
	}
	return aiIPSetFromStrings(ips), nil
}

// ---------- validación ----------

// verifyIPs devuelve las IPs verificadas de la RESOLUCIÓN LOCAL (las que la
// máquina va a usar de verdad) tras comprobar que todas aparecen en la UNIÓN
// de las respuestas de los resolvers públicos. Es el punto de corte: si algo
// no cuadra, no se habla con el proveedor.
func (g *aiDNSGuard) verifyIPs(ctx context.Context, host string) ([]string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil, fmt.Errorf("host vacío: no se puede validar")
	}
	// IP literal: no hay nada que resolver (ni que validar).
	if ip := net.ParseIP(host); ip != nil {
		return aiIPSet([]net.IP{ip}), nil
	}

	g.mu.Lock()
	entry, cached := g.cache[host]
	g.mu.Unlock()
	if cached && time.Now().Before(entry.expires) {
		return entry.ips, nil
	}

	conocidas, err := g.consensoPublico(ctx, host)
	if err != nil {
		return nil, err
	}

	local, err := g.localResolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("no se pudo resolver %q en este equipo: %v", host, err)
	}
	localSet := aiIPSet(local)
	if len(localSet) == 0 {
		return nil, fmt.Errorf("la resolución local de %q no devolvió direcciones IP", host)
	}
	if missing := aiMissing(localSet, conocidas); len(missing) > 0 {
		return nil, fmt.Errorf(
			"validación de seguridad fallida para %q: la resolución local (IP %s) no aparece en "+
				"las respuestas de Cloudflare ni de Google (IP %s). Posible interceptación de DNS o "+
				"proxy de inspección en la red. Se corta la conexión con el proveedor de IA para no "+
				"exponer los datos del negocio. Si estás en una red corporativa con proxy, configura "+
				"%s=off y avisa al administrador.",
			host, strings.Join(localSet, ", "), strings.Join(conocidas, ", "), aiDNSGuardEnv)
	}

	// Se fijan las IPs locales verificadas: es la ruta que el CDN ya eligió
	// para esta máquina; fijar las IPs que respondió un resolver remoto
	// desincronizaría la conexión (otra región, otro edge).
	g.mu.Lock()
	g.cache[host] = aiDNSGuardCacheEntry{ips: localSet, expires: time.Now().Add(aiDNSCacheTTL)}
	g.mu.Unlock()
	return localSet, nil
}

// consensoPublico consulta a todos los resolvers y devuelve la UNIÓN de sus
// respuestas: la credencial de "esto es de dominio público", no una copia
// exacta (con CDN geográfico las respuestas legítimas ya difieren). Exige que
// al menos DOS resolvers hayan respondido con registros: con una sola
// respuesta un secuestro del propio resolver pasaría indetectado.
func (g *aiDNSGuard) consensoPublico(ctx context.Context, host string) ([]string, error) {
	if len(g.endpoints) < 2 {
		return nil, fmt.Errorf("validación de DNS no disponible: se requieren al menos 2 resolvers públicos")
	}
	conocidas := map[string]struct{}{}
	resolversOK := 0
	var lastErr error
	for _, ep := range g.endpoints {
		ips, err := g.resolveDoH(ctx, ep, host)
		if err != nil {
			lastErr = err
			continue
		}
		if len(ips) == 0 {
			lastErr = fmt.Errorf("%s: sin registros A/AAAA", ep.Name)
			continue
		}
		resolversOK++
		for _, ip := range ips {
			conocidas[ip] = struct{}{}
		}
	}
	if resolversOK < 2 {
		if lastErr == nil {
			lastErr = fmt.Errorf("respondieron menos de 2 resolvers")
		}
		return nil, fmt.Errorf("validación de DNS no disponible para %q: %v", host, lastErr)
	}
	union := make([]string, 0, len(conocidas))
	for ip := range conocidas {
		union = append(union, ip)
	}
	sort.Strings(union)
	return union, nil
}

// ---------- cliente HTTP fijado a las IPs validadas ----------

// aiPinnedDialContext fuerza la conexión TCP a las IPs ya verificadas. Cierra
// la ventana TOCTOU: entre validar y conectar, el DNS local puede cambiar,
// pero aquí no vuelve a consultarse.
func aiPinnedDialContext(ips []string, timeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var errs []string
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				return conn, nil
			}
			errs = append(errs, err.Error())
		}
		return nil, fmt.Errorf("no se pudo conectar a %s por ninguna IP verificada (%s): %s",
			addr, strings.Join(ips, ", "), strings.Join(errs, "; "))
	}
}

// aiGuardedClient devuelve un cliente HTTP que valida el DNS del host del
// endpoint antes de conectar y se fija a las IPs verificadas. El TLS se valida
// igual que siempre contra el dominio (ServerName intacto).
func aiGuardedClient(endpoint string, timeout time.Duration) (*http.Client, error) {
	if !aiDNSGuardEnabled() {
		return &http.Client{Timeout: timeout}, nil
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("endpoint inválido %q: %v", endpoint, err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("el endpoint %q no tiene host", endpoint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiDNSGuardTimeout)
	defer cancel()

	ips, err := aiDNSGuardSingleton().verifyIPs(ctx, host)
	if err != nil {
		return nil, err
	}

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("no se pudo preparar la conexión segura con el proveedor de IA")
	}
	clone := tr.Clone()
	clone.DialContext = aiPinnedDialContext(ips, timeout)
	clone.TLSHandshakeTimeout = timeout

	return &http.Client{Timeout: timeout, Transport: clone}, nil
}

// aiDNSGuardEnabled permite la válvula de escape documentada en aiDNSGuardEnv.
func aiDNSGuardEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(aiDNSGuardEnv))) {
	case "0", "off", "false", "no":
		return false
	default:
		return true
	}
}
