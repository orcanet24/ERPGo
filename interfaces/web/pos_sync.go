package web

// pos_sync.go — sincronización POS (catálogo, clientes, ventas, estado).
//
// SEGURIDAD: estas rutas son públicas en el sentido de que no hay sesión de
// usuario web, pero NO son anónimas: TODAS las de datos pasan por
// posSyncKeyMiddleware, que exige una API key de terminal vigente y resuelve
// la empresa DESDE LA KEY (tabla api_keys), nunca desde lo que el cliente
// declare en X-POS-Tenant. Sin eso, cualquiera que llegara al puerto podía
// descargar el catálogo, leer los clientes con sus RIF y teléfonos, o
// inyectar facturas en la base de una empresa.
//
// La única ruta sin middleware es /api/pos/validate-key, que es justamente el
// enlace de arranque de una caja nueva: no devuelve datos, solo contesta si
// una key existe (401) y, si existe, su sucursal.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"erpgo/internal/pos"
)

// posSyncDBKey es la clave de contexto con la que el middleware deja la base YA
// resuelta para el resto de la petición. Es un tipo vacío propio: usar un
// string como clave de contexto es una colisión esperando a ocurrir.
type posSyncDBKey struct{}

// posSyncTenantKey guarda el CÓDIGO de la empresa resuelto por la API key.
// Lo necesita la cola de facturación digital: su configuración es por empresa
// y el solo handle de la base no dice a qué código pertenece.
type posSyncTenantKey struct{}

// POS sync server instance (initialized in registerPOSSyncRoutes)
var posSyncServer *pos.SyncServer

// SetPOSSyncServer initializes the POS sync server reference.
func SetPOSSyncServer(s *pos.SyncServer) {
	posSyncServer = s
}

// GetPOSSyncServer returns the global POS sync server.
func GetPOSSyncServer() *pos.SyncServer {
	return posSyncServer
}

// syncForDB devuelve un servidor de sincronización atado a UNA base ya
// resuelta. El auditor recibe estas operaciones con el mismo criterio que las
// del resto del ERP.
func (s *Server) syncForDB(db *sql.DB) *pos.SyncServer {
	return pos.NewSyncServer(db, func(action, module, table, recordID string, oldVal, newVal interface{}) {
		// La auditoría desde sync usa un request sintético vacío.
		// El userID será "pos_sync" para rastrear estas operaciones.
		s.audit(nil, action, module, table, recordID, oldVal, newVal)
	})
}

// syncFor devuelve un servidor de sincronización para ESTA petición.
//
// Si el middleware de API key ya resolvió la empresa, se usa esa base sin
// mirar nada más: la tabla api_keys es lo único de lo que el servidor puede
// fiarse. El resto del camino (X-POS-Tenant, ?tenant=) queda como respaldo
// para las rutas que no llevan key, como validate-key.
func (s *Server) syncFor(r *http.Request) *pos.SyncServer {
	if db, ok := r.Context().Value(posSyncDBKey{}).(*sql.DB); ok && db != nil {
		return s.syncForDB(db)
	}
	return s.syncForDB(s.posDBFor(r))
}

// registerPOSSyncRoutes registra las rutas de sincronización POS.
// Las cuatro de datos exigen API key; validate-key no (es el arranque).
func (s *Server) registerPOSSyncRoutes() {
	s.mux.HandleFunc("POST /api/pos/sync/sale", s.posSyncKeyMiddleware(s.handlePOSSyncSale))
	s.mux.HandleFunc("GET /api/pos/products", s.posSyncKeyMiddleware(s.handlePOSProducts))
	s.mux.HandleFunc("GET /api/pos/customers", s.posSyncKeyMiddleware(s.handlePOSCustomers))
	s.mux.HandleFunc("POST /api/pos/customers", s.posSyncKeyMiddleware(s.handlePOSCustomersSync))
	s.mux.HandleFunc("POST /api/pos/validate-key", s.handlePOSValidateKey)
	s.mux.HandleFunc("GET /api/pos/sync/status", s.posSyncKeyMiddleware(s.handlePOSSyncStatus))

	// Latido e identidad del terminal. Reemplaza a descargar el catálogo entero
	// cada pocos segundos solo para saber si hay red.
	s.mux.HandleFunc("GET /api/pos/ping", s.posSyncKeyMiddleware(s.handlePOSPing))

	// Cola de turnos offline. Usa API key (no caduca), no X-POS-Token.
	s.mux.HandleFunc("POST /api/pos/sync/shift/open", s.posSyncKeyMiddleware(s.handlePOSShiftOpenSync))
	s.mux.HandleFunc("POST /api/pos/sync/shift/close", s.posSyncKeyMiddleware(s.handlePOSShiftCloseSync))
	s.mux.HandleFunc("GET /api/pos/sync/shift/status", s.posSyncKeyMiddleware(s.handlePOSShiftStatusSync))
	// Bloques fiscales offline por caja (reserva, consumo, estado)
	s.mux.HandleFunc("POST /api/pos/fiscal-block/reserve", s.posSyncKeyMiddleware(s.reserveFiscalBlockHandler))
	s.mux.HandleFunc("POST /api/pos/fiscal-block/consume", s.posSyncKeyMiddleware(s.consumeFiscalNumberHandler))
	s.mux.HandleFunc("GET /api/pos/fiscal-block/status", s.posSyncKeyMiddleware(s.getFiscalBlockStatusHandler))
}

// posSyncDB devuelve la base resuelta por el middleware POS desde el contexto.
// Los handlers fiscales offline la necesitan: las tablas pos_fiscal_blocks viven
// en el tenant, no en la base de control.
func (s *Server) posSyncDB(r *http.Request) (*sql.DB, error) {
	if db, ok := r.Context().Value(posSyncDBKey{}).(*sql.DB); ok && db != nil {
		return db, nil
	}
	return nil, fmt.Errorf("no database resolved in context")
}

// extractPOSAPIKey saca la API key de una petición POS por el orden que sea
// razonable: cabecera explícita, Authorization Bearer y, en último término,
// el parámetro de la URL. Mismo criterio en todos los sitios para que una caja
// no tenga que adivinar dónde ponerla.
func extractPOSAPIKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-API-Key")); v != "" {
		return v
	}
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		if v := strings.TrimSpace(strings.TrimPrefix(a, "Bearer ")); v != "" {
			return v
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("api_key"))
}

// posSyncKeyMiddleware exige una API key de terminal vigente antes de tocar
// datos de sincronización.
//
// Orden de comprobaciones: que la key exista en la petición, que exista en la
// tabla api_keys de ALGÚNNA empresa activa, y solo entonces se deja pasar. La
// empresa se resuelve con DBForAPIKey (que además la cachea un minuto) y se
// deja en el contexto: syncFor la usa sin volver a preguntar.
//
// Límite de volumen por key: una caja en mal estado o una key robada no pueden
// inundar el servidor con descargas masivas de catálogo/clientes. Generoso para
// no apretar la operación real (ver pos_ratelimit.go).
func (s *Server) posSyncKeyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := extractPOSAPIKey(r)
		if apiKey == "" {
			jsonError(w, "API key requerida", http.StatusUnauthorized)
			return
		}

		db, err := s.DBForAPIKey(apiKey)
		if err != nil {
			jsonError(w, "API key inválida", http.StatusUnauthorized)
			return
		}

		if !posVolumeLimiter.allow("key:" + apiKey) {
			jsonError(w, "Demasiadas peticiones, reduzca la frecuencia", http.StatusTooManyRequests)
			return
		}

		ctx := context.WithValue(r.Context(), posSyncDBKey{}, db)
		if tenant, terr := s.TenantForAPIKey(apiKey); terr == nil && tenant != "" {
			ctx = context.WithValue(ctx, posSyncTenantKey{}, tenant)
		}
		next(w, r.WithContext(ctx))
	}
}

// handlePOSSyncSale recibe una venta desde el terminal POS y la sincroniza.
func (s *Server) handlePOSSyncSale(w http.ResponseWriter, r *http.Request) {
	srv := s.syncFor(r)

	var req pos.SyncSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON inválido: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validación mínima.
	if req.TerminalID == "" || req.BranchID == 0 {
		jsonError(w, "terminal_id y branch_id son obligatorios", http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 {
		jsonError(w, "la venta debe tener al menos un item", http.StatusBadRequest)
		return
	}

	// Aquí NO se evalúa el guard de addons: esta ruta es la cola OFFLINE del
	// terminal. Esas ventas ya se cobraron sin red; bloquearlas al sincronizar
	// dejaría la cola atascada con un cobro que el cliente ya pagó. Lo que se
	// valida en línea es el checkout (/api/pos/checkout) y el gRPC sin offline.
	resp, err := srv.SyncSale(&req)
	if err != nil {
		// Si ya existe (idempotente), no es error.
		if resp != nil && resp.Status == "already_exists" {
			writeJSON(w, resp)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// La venta sincronizada se apunta a la cola de facturación digital: el
	// runner la emisará al proveedor que la empresa tenga activo. Es asincrónico
	// a propósito: si Unidigital está caído o sin conexión, el ERP igual
	// confirma la sincronización a la caja y la cola reintenta sola.
	if resp != nil && resp.Success && resp.RemoteID != "" {
		dbSync, _ := s.posSyncDB(r)
		tenant, _ := r.Context().Value(posSyncTenantKey{}).(string)
		if dbSync != nil && tenant != "" {
			go s.diEnqueueVenta(dbSync, tenant, resp.RemoteID)
		}
	}

	writeJSON(w, resp)
}

// handlePOSProducts retorna todos los productos activos para sincronización POS.
func (s *Server) handlePOSProducts(w http.ResponseWriter, r *http.Request) {
	products, err := s.syncFor(r).GetProducts()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, products)
}

// handlePOSCustomers retorna los clientes activos para sincronización POS.
func (s *Server) handlePOSCustomers(w http.ResponseWriter, r *http.Request) {
	customers, err := s.syncFor(r).GetCustomers()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, customers)
}

// handlePOSValidateKey valida una API key y retorna información de la sucursal.
//
// Es el enlace de arranque de una caja nueva, por eso no pide API key por
// middleware: la key ES el argumento. A cambio no devuelve datos de negocio: o
// un 401 (la key no existe en ninguna empresa) o la sucursal a la que pertenece.
// La empresa se resuelve desde la propia key; X-POS-Tenant no se mira.
func (s *Server) handlePOSValidateKey(w http.ResponseWriter, r *http.Request) {
	apiKey := extractPOSAPIKey(r)
	if apiKey == "" {
		// También se acepta el cuerpo clásico {"api_key": "..."}.
		var reqBody struct {
			APIKey string `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		apiKey = strings.TrimSpace(reqBody.APIKey)
	}

	if apiKey == "" {
		jsonError(w, "API key requerida", http.StatusBadRequest)
		return
	}

	db, err := s.DBForAPIKey(apiKey)
	if err != nil {
		// 401, no 200 con valid:false: el POS distingue por código de estado,
		// y un 200 con "inválida" adentro es una puerta abierta a confusión.
		jsonError(w, "API key inválida", http.StatusUnauthorized)
		return
	}

	resp, err := s.syncForDB(db).ValidateAPIKey(apiKey)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp)
}

// handlePOSSyncStatus retorna estadísticas de sincronización para un terminal.
func (s *Server) handlePOSSyncStatus(w http.ResponseWriter, r *http.Request) {
	terminalID := r.URL.Query().Get("terminal_id")
	if terminalID == "" {
		jsonError(w, "terminal_id requerido", http.StatusBadRequest)
		return
	}

	resp, err := s.syncFor(r).GetSyncStatus(terminalID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp)
}

// posClienteSync es un cliente que el terminal creó OFFLINE en su SQLite y
// manda al reponerse la conexión.
type posClienteSync struct {
	LocalID  string `json:"local_id"` // id del cliente en el terminal
	RIF      string `json:"rif"`      // cédula/RIF; el reductor de duplicados
	Nombre   string `json:"name"`     // nombre
	Telefono string `json:"phone"`    // móvil opcional
}

// posClienteSyncResp es la resolución de UN cliente: o se creó, o la caja ya
// no necesita crearlo porque la empresa lo tiene (merged): en ese caso el
// terminal guarda el customer_id real y su factura se procesa normal.
type posClienteSyncResp struct {
	LocalID    string `json:"local_id"`
	CustomerID int64  `json:"customer_id"`
	Merged     bool   `json:"merged"`
	Created    bool   `json:"created"`
}

// handlePOSCustomersSync sube clientes creados por el terminal con
// deduplicación por RIF. Si dos cajas crearon el mismo cliente sin red, el
// primero que sincroniza lo crea y las demás reciben merged=true: su
// 'local' pasa a referir el customer_id real del ERP y nada se duplica.
func (s *Server) handlePOSCustomersSync(w http.ResponseWriter, r *http.Request) {
	db, ok := r.Context().Value(posSyncDBKey{}).(*sql.DB)
	if !ok || db == nil {
		jsonError(w, "Base no resuelta", http.StatusInternalServerError)
		return
	}
	var req struct {
		Customers []posClienteSync `json:"customers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "JSON inválido: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp := []posClienteSyncResp{}
	for _, c := range req.Customers {
		rif := strings.ToUpper(strings.TrimSpace(c.RIF))
		if rif == "" {
			rif = "V000000000" // mismo placeholder del findOrCreateCustomer de SyncSale
		}
		var customerID int64
		err := db.QueryRow(`SELECT customer_id FROM customer_information WHERE rif = ? LIMIT 1`, rif).Scan(&customerID)
		switch {
		case err == nil:
			// Ya existe: la terminal que lo creó adopta el id real. Nada que
			// escribir: los datos del terminal no sobreescriben los del ERP.
			resp = append(resp, posClienteSyncResp{LocalID: c.LocalID, CustomerID: customerID, Merged: true})
		case err == sql.ErrNoRows:
			if res, ierr := db.Exec(`INSERT INTO customer_information (customer_name, rif, address2, status, customer_type)
				VALUES (?, ?, '', 1, 1)`, c.Nombre, rif); ierr == nil {
				if id, ierr := res.LastInsertId(); ierr == nil {
					resp = append(resp, posClienteSyncResp{LocalID: c.LocalID, CustomerID: id, Created: true})
					continue
				}
			}
			// Carrera entre dos cajas haciendo el mismo insert: releer.
			if e2 := db.QueryRow(`SELECT customer_id FROM customer_information WHERE rif = ? LIMIT 1`, rif).Scan(&customerID); e2 == nil {
				resp = append(resp, posClienteSyncResp{LocalID: c.LocalID, CustomerID: customerID, Merged: true})
			}
		default:
			// Error de BD con este cliente: se reporta local y el terminal reintenta.
			resp = append(resp, posClienteSyncResp{LocalID: c.LocalID})
		}
	}
	writeJSON(w, map[string]interface{}{"results": resp})
}

