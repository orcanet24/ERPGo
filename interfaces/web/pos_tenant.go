package web

// pos_tenant.go — resuelve a qué EMPRESA pertenece cada terminal POS.
//
// El POS es un binario aparte que corre en cajas registradoras (misma red local
// o por internet) y habla con este servidor por HTTP y gRPC. Sus ventas, su
// catálogo y sus sesiones viven en la base de la empresa a la que pertenece la
// caja, NUNCA en la base de control: la de control solo guarda el catálogo de
// empresas y quién es miembro de cuál.
//
// Cómo se sabe qué empresa es:
//   - El terminal envía su empresa en la cabecera X-POS-Tenant (config.json
//     del terminal, campo "tenant"; por defecto "default").
//   - La API key del terminal vive en la tabla api_keys de la BASE DE LA
//     EMPRESA (el panel de administración la crea ahí, en pos_admin.go), así
//     que para resolver una key sin cabecera se busca en cada empresa
//     registrada. Ese camino (DBForAPIKey) es el que usa el servidor gRPC,
//     cuyo protocolo no transporta la cabecera.
//
// Una key no puede pertenecer a dos empresas: el primer acierto gana, y la
// búsqueda recorre las empresas en orden alfabético de código.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// posTenantCode devuelve el código de empresa que declara esta petición POS.
func (s *Server) posTenantCode(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-POS-Tenant")); t != "" {
		return slugifyTenant(t)
	}
	if t := strings.TrimSpace(r.URL.Query().Get("tenant")); t != "" {
		return slugifyTenant(t)
	}
	return s.activeTenantCode()
}

// posDBFor devuelve la base de la EMPRESA del terminal que hace esta petición.
// Es el punto por el que pasan login, checkout y sincronización del POS: sin
// él, las cajas de una empresa no-nueva escribían en la base equivocada y sus
// facturas no aparecían en ningún lado.
func (s *Server) posDBFor(r *http.Request) *sql.DB {
	return s.dbForTenant(s.posTenantCode(r))
}

// posKeyCache entrada de la caché corta de API key -> empresa.
type posKeyCacheEntry struct {
	tenant  string
	expires time.Time
}

var (
	posKeyCacheMu sync.Mutex
	posKeyCache   = map[string]posKeyCacheEntry{}
)

// posKeyCacheTTL cuánto se confía en una resolución ya hecha. Es corto a
// propósito: revocar una key en el panel debe surtir efecto en un minuto, no
// al reiniciar el servidor.
const posKeyCacheTTL = time.Minute

// InvalidatePOSKeyCache descarta la caché de resolución de UNA key (o de todas
// si apiKey es ""). Se llama desde el panel al revocar, regenerar o borrar una
// key, para que el efecto sea INMEDIATO en vez de esperar al TTL de un minuto:
// una caja robada no debe seguir sincronizando ni un minuto más de lo que
// tarde el admin en clicar revocar.
func InvalidatePOSKeyCache(apiKey string) {
	posKeyCacheMu.Lock()
	defer posKeyCacheMu.Unlock()
	if apiKey == "" {
		posKeyCache = map[string]posKeyCacheEntry{}
		return
	}
	delete(posKeyCache, apiKey)
}

// DBForAPIKey devuelve la base de la empresa cuya tabla api_keys contiene la
// key dada (activa). Es la vía del servidor gRPC, que recibe la key en el
// registro del terminal y no puede consultar una cabecera HTTP.
func (s *Server) DBForAPIKey(apiKey string) (*sql.DB, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("api key vacía")
	}

	posKeyCacheMu.Lock()
	if e, ok := posKeyCache[apiKey]; ok && time.Now().Before(e.expires) {
		posKeyCacheMu.Unlock()
		return s.dbForTenant(e.tenant), nil
	}
	posKeyCacheMu.Unlock()

	for _, code := range s.TenantCodes() {
		db := s.dbForTenant(code)
		if db == nil {
			continue
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE api_key = ? AND is_active = 1`,
			apiKey).Scan(&n); err != nil || n == 0 {
			continue
		}
		posKeyCacheMu.Lock()
		posKeyCache[apiKey] = posKeyCacheEntry{tenant: code, expires: time.Now().Add(posKeyCacheTTL)}
		posKeyCacheMu.Unlock()
		return db, nil
	}
	return nil, fmt.Errorf("api key no válida en ninguna empresa")
}
