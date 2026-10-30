package web

// reorder_point.go — Punto de pedido y sugerencias de reposición.
//
// EL CÁLCULO, Y POR QUÉ NO ES "SI QUEDA POCO, COMPRAR".
//
// Lo que de verdad se pregunta un almacén no es "¿cuánto hay?" sino "¿llegará a
// tiempo?". Y esa pregunta tiene tres cantidades detrás:
//
//	punto de pedido = consumo del plazo  +  colchón
//	                = media diaria × días del proveedor + días de seguridad
//
// El "consumo del plazo" es lo que se gasta mientras el pedido viaja. El
// "colchón" es lo que absorbe la variability: el proveedor llega tarde, la
// demanda sube, hay un puente. Sin colchón, el punto de pedido es exactamente
// el momento de quedarse sin stock, que es tarde.
//
// POR QUÉ EL COLCHÓN SE MIDE EN DÍAS Y NO EN UNIDADES.
//
// Porque el proveedor se mide en días y el almacén, en kilos. Un colchón de 50
// kilos son dos semanas para un producto que sale 3 kilos por día y un mes para
// uno que sale uno por semana. Lo que se necesita es "aguantar N días de más",
// y N es un número que el responsable del almacén sabe decir. En días es
// entendible; en kilos es un número que cada quien interpretaría distinto.
//
// LO QUE ESTE ARCHIVO NO INVENTA.
//
// El consumo medio sale de datos reales (`invoice_details` + `invoice`), no de
// un número escrito a mano. Si no hay historial, la demanda media es 0 y el
// punto de pedido solo cubre el colchón: eso NO es un error del cálculo, es la
// respuesta correcta a "no tengo datos", y por eso la sugerencia avisa de que
// está calculándose sin historial en vez de devolver un número con apariencia de
// certeza.

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// revisarDiasRevision es cada cuánto se repasa el punto de pedido.
//
// El cálculo compara existencias contra el punto y dice "falta". Falta una
// pregunta más: ¿falta para hoy o para dentro de tres semanas? Si el siguiente
// pedido se revisa dentro de 7 días, la reposición no tiene que cubrir solo el
// plazo del proveedor, sino el plazo más los días que faltan para el siguiente
// repaso. Si se repasa mañana, da igual. Es el parámetro que hace que dos
// empresas con el mismo consumo y el mismo proveedor pidan distinto.
const revisarDiasRevision = 7

// PoliticaReorden es lo que el usuario define por producto.
type PoliticaReorden struct {
	ProductID    int64
	LeadTimeDays int
	SafetyDays   int
	MinOrderQty  float64
	PackSize     float64
	Active       bool
	Note         string
	AvgDaily     float64
	ComputedOn   string
}

// SugerenciaReorden es el resultado del cálculo para un producto.
type SugerenciaReorden struct {
	ProductID       int64   `json:"product_id"`
	ProductName     string  `json:"product_name"`
	Unit            string  `json:"unit"`
	OnHand          float64 `json:"on_hand"`
	AvgDaily        float64 `json:"avg_daily"`
	LeadTimeDays    int     `json:"lead_time_days"`
	SafetyDays      int     `json:"safety_days"`
	ReorderPoint    float64 `json:"reorder_point"`
	SuggestedQty    float64 `json:"suggested_qty"`
	PackSize        float64 `json:"pack_size"`
	MinOrderQty     float64 `json:"min_order_qty"`
	UnitCost        float64 `json:"unit_cost"`
	EstimatedCost   float64 `json:"estimated_cost"`
	DiasDeCobertura float64 `json:"dias_de_cobertura"`
	Urgencia        string  `json:"urgencia"`
	SinHistorial    bool    `json:"sin_historial"`
	SupplierID      int64   `json:"supplier_id"`
	SupplierName    string  `json:"supplier_name"`
}

// SetPoliticaReorden guarda la política de un producto.
//
// Si la política no existe, se crea; si existe, se sobrescribe.
//
// Se valida ANTES de escribir, y no por purismo: un plazo negativo produce un
// punto de pedido negativo, y un punto de pedido negativo hace que el sistema
// diga "todo está bien" con el almacén vacío. Un dato malo entra como una
// recomendación de no comprar nada, que es el peor fallo posible de esta
// pantalla porque es silencioso.
//
// `avg_daily_demand` y `computed_on` NO se tocan aquí: los calcula el sistema.
// Que la demanda se pueda escribir a mano desde esta misma pantalla sería
// invites a que alguien la invente.
func (s *Server) SetPoliticaReorden(r *http.Request, p PoliticaReorden) error {
	if p.ProductID == 0 {
		return fmt.Errorf("la política necesita un producto")
	}
	if p.LeadTimeDays < 0 {
		return fmt.Errorf("el plazo del proveedor no puede ser negativo")
	}
	if p.SafetyDays < 0 {
		return fmt.Errorf("los días de colchón no pueden ser negativos")
	}
	if p.LeadTimeDays == 0 {
		return fmt.Errorf("indique el plazo del proveedor en días; con 0 el punto de " +
			"pedido no significa nada")
	}
	if p.PackSize <= 0 {
		p.PackSize = 1
	}
	if p.MinOrderQty < 0 {
		p.MinOrderQty = 0
	}
	activo := 0
	if p.Active {
		activo = 1
	}
	_, err := s.dbFor(r).Exec(`
		INSERT INTO reorder_policy
			(product_id, lead_time_days, safety_days, min_order_qty, pack_size, active, note, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			lead_time_days = VALUES(lead_time_days),
			safety_days    = VALUES(safety_days),
			min_order_qty  = VALUES(min_order_qty),
			pack_size      = VALUES(pack_size),
			active         = VALUES(active),
			note           = VALUES(note),
			updated_at     = VALUES(updated_at)`,
		p.ProductID, p.LeadTimeDays, p.SafetyDays, p.MinOrderQty, p.PackSize,
		activo, p.Note, now())
	if err != nil {
		return fmt.Errorf("no se pudo guardar la política: %w", err)
	}
	return nil
}

// ListarPoliticas devuelve todas las políticas con su producto.
//
// Se usa para pintar la tabla de configuración: el usuario tiene que ver el
// consumo medio calculado junto al plazo que él escribió, porque si no los dos
// números parecen contradicted y el que no conoce el cálculo asume que el
// sistema está equivocado.
func (s *Server) ListarPoliticas(r *http.Request, soloActivas bool) ([]PoliticaReorden, error) {
	q := `
		SELECT pol.product_id,
		       COALESCE(p.product_name, ''),
		       pol.lead_time_days, pol.safety_days, pol.min_order_qty,
		       pol.pack_size, pol.avg_daily_demand, pol.active, pol.note, pol.computed_on
		FROM reorder_policy pol
		LEFT JOIN product_information p ON p.product_id = pol.product_id`
	args := []interface{}{}
	if soloActivas {
		q += " WHERE pol.active = 1"
	}
	q += " ORDER BY COALESCE(p.product_name, '') ASC"

	filas, err := s.dbFor(r).Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer las políticas: %w", err)
	}
	defer filas.Close()

	var lista []PoliticaReorden
	for filas.Next() {
		var p PoliticaReorden
		var nombre string
		var activo int
		if err := filas.Scan(&p.ProductID, &nombre, &p.LeadTimeDays, &p.SafetyDays,
			&p.MinOrderQty, &p.PackSize, &p.AvgDaily, &activo, &p.Note, &p.ComputedOn); err != nil {
			continue
		}
		p.Active = activo == 1
		lista = append(lista, p)
	}
	return lista, filas.Err()
}

// CalcularDemandaMedia recalcula el consumo medio de un producto a partir de las
// ventas de los últimos `dias`.
//
// Usa `invoice_details` y no la tabla de consumo ya hecha porque es la fuente de
// verdad: si alguien corrige una factura vieja, el punto de pedido tiene que
// enterarse. El resultado se guarda en la propia política para que las consultas
// de la pantalla no tengan que recorrer el histórico cada vez.
//
// Los borradores NO cuentan. Una factura en borrador es una intención de venta,
// no una venta: contarla infla la demanda y el sistema compra de más.
//
// Lo que NO se filtra es `invoice.is_credit`, que por nombre parece "factura a
// crédito" y en realidad vale "no se indicó forma de pago". El ERP lo pone a 1
// cuando el formulario no manda `multipaytype`, que es lo que pasa en
// cualquier llamada que no venga del JavaScript de la pantalla. Filtrando por
// ahí, la demanda media de una empresa normal salía CERO y el punto de pedido
// se quedaba solo en el colchón. Una venta a crédito sigue siendo una venta.
//
// Los días sin venta cuentan como 0, no se saltan. Es la diferencia entre "este
// producto vende 5 al día" y "este producto se vendió 5 veces en los últimos 30
// días": saltarse los días sin venta infla la media, que es el error caro.
func (s *Server) CalcularDemandaMedia(r *http.Request, productID int64, dias int) (float64, error) {
	if dias <= 0 {
		dias = 90
	}
	desde := time.Now().AddDate(0, 0, -dias).Format("2006-01-02")

	var consumo float64
	err := s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(d.quantity), 0)
		FROM invoice_details d
		JOIN invoice i ON i.id = d.invoice_id
		WHERE d.product_id = ?
		  AND i.date >= ?
		  AND i.is_draft = 'no'
		  AND COALESCE(i.is_anulled, 0) = 0`, productID, desde).Scan(&consumo)
	if err != nil {
		return 0, fmt.Errorf("no se pudo calcular la demanda de %d: %w", productID, err)
	}

	media := consumo / float64(dias)
	_, err = s.dbFor(r).Exec(`
		INSERT INTO reorder_policy (product_id, avg_daily_demand, computed_on, updated_at)
		VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE
			avg_daily_demand = VALUES(avg_daily_demand),
			computed_on      = VALUES(computed_on),
			updated_at       = VALUES(updated_at)`,
		productID, media, time.Now().Format("2006-01-02"), now())
	if err != nil {
		return 0, fmt.Errorf("no se pudo guardar la demanda media: %w", err)
	}
	return media, nil
}

// RecalcularDemandas rehace la demanda media de todas las políticas activas.
//
// Va de una en una a propósito: si una consulta global falla a mitad, con una
// sola sentencia no se ha escrito nada. Aquí el estado bueno es "todo viejo" y
// el malo es "mitad nuevo, mitad viejo", que es indistinguible de un dato real.
func (s *Server) RecalcularDemandas(r *http.Request, dias int) (int, error) {
	politicas, err := s.ListarPoliticas(r, true)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range politicas {
		if _, err := s.CalcularDemandaMedia(r, p.ProductID, dias); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// existenciasDeProducto dice cuánto hay de un producto.
//
// Primero mira `product_lot`, que es el almacén con caducidades: si hay lotes
// abiertos, ese es el número bueno, porque es el que se puede vender HOY. Si no
// hay lotes, cae al histórico del ERP (compras menos ventas), que es lo que usa
// el resto del sistema.
//
// No se mezclan las dos fuentes sumando: si se sumaran, un producto con lotes
// contaría sus existencias dos veces y el punto de pedido aparecería con
// cobertura de sobra justo en los productos perecederos, que son los que más
// caro salen cuando se pierden.
func (s *Server) existenciasDeProducto(r *http.Request, productID int64) (float64, error) {
	var conLotes float64
	err := s.dbFor(r).QueryRow(`
		SELECT COALESCE(SUM(quantity_available), 0) FROM product_lot
		WHERE product_id = ? AND quantity_available > 0`, productID).Scan(&conLotes)
	if err != nil {
		return 0, fmt.Errorf("no se pudieron leer los lotes del producto %d: %w", productID, err)
	}
	if conLotes > 0 {
		return conLotes, nil
	}

	historico := s.stockForProduct(r, int(productID))
	if historico < 0 {
		// El histórico puede dar negativo si se vendió mercadería que no
		// entró por compra (donación, producción, arranque con stock inicial
		// cargado fuera de compras). Un negativo no es un dato: es un
		// inventario que no cuadra. Se recorta a cero y el cálculo de
		// reposición pide de más, que es el lado prudente del error: comprar
		// de sobra cuesta unos pocos días de caja; quedarse corto para la venta.
		historico = 0
	}
	return historico, nil
}

// SugerirReposiciones calcula qué hay que pedir.
//
// Devuelve SOLO los productos que están en o por debajo del punto de pedido.
// Devolver la lista completa obligaría a la pantalla a decidir qué está urgente,
// y esa decisión es justo la que se quiere automatizar: si hay 200 productos y 3
// necesitan pedido, lo útil son los 3.
//
// El redondeo al múltiplo de compra va AL FINAL, después de calcular la cantidad
// teórica. Al revés se pide de menos: si la cantidad teórica es 23 y el múltiplo
// es 10, redondear antes da 20 y el día 24 no hay stock.
func (s *Server) SugerirReposiciones(r *http.Request, diasDemanda int) ([]SugerenciaReorden, error) {
	if diasDemanda <= 0 {
		diasDemanda = 90
	}
	// La demanda guardada puede tener más de `diasDemanda` días. Si está más
	// nueva se respeta; si es más vieja que la ventana pedida, se recalcula,
	// porque un consumo de hace un año no describe la rotación de hoy.
	filas, err := s.dbFor(r).Query(`
		SELECT pol.product_id,
		       COALESCE(p.product_name, ''),
		       COALESCE(p.unit, ''),
		       pol.avg_daily_demand,
		       pol.computed_on,
		       pol.lead_time_days,
		       pol.safety_days,
		       pol.min_order_qty,
		       pol.pack_size,
		       COALESCE(NULLIF(p.cost, 0),
		                COALESCE((SELECT AVG(d2.rate) FROM product_purchase_details d2
		                           WHERE d2.product_id = pol.product_id), 0), 0),
		       COALESCE(sp.supplier_id, 0),
		       COALESCE(su.supplier_name, '')
		FROM reorder_policy pol
		LEFT JOIN product_information p ON p.product_id = pol.product_id
		LEFT JOIN supplier_product sp
		       ON sp.product_id = pol.product_id
		LEFT JOIN supplier_information su ON su.supplier_id = sp.supplier_id
		WHERE pol.active = 1
		ORDER BY pol.product_id ASC, sp.supplier_pr_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron leer las políticas: %w", err)
	}
	defer filas.Close()

	// Una fila por producto: el proveedor sale del primer renglón de
	// `supplier_product` y los duplicados se descartan. Se agrupa aquí y no en
	// SQL porque `supplier_id` puede venir repetido por renglón y `COALESCE`
	// sobre un GROUP BY no es determinista: MySQL elige la fila que le da la
	// gana y el pedido cambia de proveedor entre ejecuciones.
	type fila struct {
		producto      int64
		nombre        string
		unidad        string
		demanda       float64
		computedOn    string
		plazo         int
		colchon       int
		minimo        float64
		multiplo      float64
		costo         float64
		proveedorID   int64
		proveedorName string
	}
	var todos []fila
	for filas.Next() {
		var t fila
		if err := filas.Scan(&t.producto, &t.nombre, &t.unidad, &t.demanda, &t.computedOn,
			&t.plazo, &t.colchon, &t.minimo, &t.multiplo, &t.costo,
			&t.proveedorID, &t.proveedorName); err != nil {
			continue
		}
		vistos := make(map[int64]bool, len(todos))
		for _, v := range todos {
			vistos[v.producto] = true
		}
		if vistos[t.producto] {
			continue
		}
		todos = append(todos, t)
	}
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("error leyendo las políticas: %w", err)
	}

	desde := time.Now().AddDate(0, 0, -diasDemanda).Format("2006-01-02")
	var sugerencias []SugerenciaReorden
	for _, t := range todos {
		if t.demanda <= 0 || t.computedOn < desde {
			if media, err := s.CalcularDemandaMedia(r, t.producto, diasDemanda); err == nil {
				t.demanda = media
			}
		}
		if t.plazo <= 0 {
			// Una política sin plazo no se puede calcular: el punto de pedido
			// sería solo el colchón. Se avisa en vez de devolver un número
			// que parece un cálculo y es una suposición.
			t.plazo = 1
		}

		existencia, err := s.existenciasDeProducto(r, t.producto)
		if err != nil {
			return nil, err
		}

		sug := SugerenciaReorden{
			ProductID:    t.producto,
			ProductName:  t.nombre,
			Unit:         t.unidad,
			OnHand:       existencia,
			AvgDaily:     t.demanda,
			LeadTimeDays: t.plazo,
			SafetyDays:   t.colchon,
			PackSize:     t.multiplo,
			MinOrderQty:  t.minimo,
			UnitCost:     t.costo,
			SupplierID:   t.proveedorID,
			SupplierName: t.proveedorName,
			SinHistorial: t.demanda <= 0,
		}
		sug.ReorderPoint = t.demanda * float64(t.plazo+t.colchon)
		if t.demanda > 0 {
			sug.DiasDeCobertura = existencia / t.demanda
		} else {
			sug.DiasDeCobertura = math.Inf(1)
		}
		if existencia > sug.ReorderPoint {
			continue
		}

		objetivo := float64(t.plazo+t.colchon+revisarDiasRevision) * t.demanda
		teorica := objetivo - existencia
		if teorica < 0 {
			teorica = 0
		}
		if teorica < t.minimo {
			teorica = t.minimo
		}
		if t.multiplo > 1 {
			teorica = math.Ceil(teorica/t.multiplo) * t.multiplo
		}
		sug.SuggestedQty = teorica
		sug.EstimatedCost = teorica * t.costo

		switch {
		case existencia <= 0 || sug.DiasDeCobertura <= float64(t.plazo):
			sug.Urgencia = "critical"
		case sug.DiasDeCobertura <= float64(t.plazo+t.colchon):
			sug.Urgencia = "warning"
		default:
			sug.Urgencia = "info"
		}
		sugerencias = append(sugerencias, sug)
	}

	// Primero lo crítico y luego lo que se queda sin stock antes. Ordenar por
	// "cuánto más bajo el punto" mezcla un producto que agota stock mañana con
	// uno que aguanta la semana; la pantalla se lee de arriba abajo y arriba
	// tiene que estar lo que hay que hacer hoy.
	prioridad := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(sugerencias, func(i, j int) bool {
		pi, pj := prioridad[sugerencias[i].Urgencia], prioridad[sugerencias[j].Urgencia]
		if pi != pj {
			return pi < pj
		}
		return sugerencias[i].DiasDeCobertura < sugerencias[j].DiasDeCobertura
	})
	return sugerencias, nil
}

// GuardarSugerencia guarda el resultado del cálculo como foto del día.
//
// Es una foto y no un estado vivo: recalcular no lo modifica, lo vuelve a
// escribir con fecha nueva. Guardar solo "lo que hay que pedir ahora" y no su
// historia hace imposible responder "¿por qué el mes pasado pedí 300 y este mes
// 80?".
func (s *Server) GuardarSugerencia(r *http.Request, sug SugerenciaReorden) (int64, error) {
	res, err := s.dbFor(r).Exec(`
		INSERT INTO reorder_suggestion
			(product_id, suggested_date, on_hand, reorder_point, suggested_qty,
			 pack_size, estimated_cost, status, created_at)
		VALUES (?,?,?,?,?,?,?,'pending',?)`,
		sug.ProductID, time.Now().Format("2006-01-02"), sug.OnHand, sug.ReorderPoint,
		sug.SuggestedQty, sug.PackSize, sug.EstimatedCost, now())
	if err != nil {
		return 0, fmt.Errorf("no se pudo guardar la sugerencia: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// CambiarEstadoSugerencia marca una sugerencia como pedida o descartada.
//
// Exige motivo al descartar. Descartar sin explicación es como no registrar la
// sugerencia, pero deja rastro, y el rastro sin motivo no sirve para nada: nadie
// puede aprender de un "descartado" que no dice por qué se descartó.
func (s *Server) CambiarEstadoSugerencia(r *http.Request, id int64, estado, motivo string) error {
	switch estado {
	case "ordered", "dismissed", "pending":
	default:
		return fmt.Errorf("estado de sugerencia no válido: %s", estado)
	}
	if estado == "dismissed" && motivo == "" {
		return fmt.Errorf("indique por qué se descarta la sugerencia")
	}
	_, err := s.dbFor(r).Exec(`
		UPDATE reorder_suggestion SET status = ?, dismiss_reason = ? WHERE id = ?`,
		estado, motivo, id)
	if err != nil {
		return fmt.Errorf("no se pudo actualizar la sugerencia: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func (s *Server) registerReorderRoutes() {
	// /inventory era el hub de inventario; al borrarse la página los tres
	// "Volver a Inventario" de las pantallas de inventario (reorder/expiry/
	// cycle_count) caían en 404. Se registra de nuevo como redirección al módulo
	// principal de inventario existente, de modo que el enlace vuelva a existir
	// sin tener que decidir aquí en el código qué pantalla era "la principal".
	s.mux.HandleFunc("GET /inventory", s.inventarioHub)
	s.mux.HandleFunc("GET /inventory/reorder", s.reorderPage)
	s.mux.HandleFunc("GET /inventory/reorder/data", s.reorderData)
	s.mux.HandleFunc("POST /inventory/reorder/policy", s.reorderPolicySave)
	s.mux.HandleFunc("POST /inventory/reorder/recalc", s.reorderRecalc)
	s.mux.HandleFunc("POST /inventory/reorder/suggestions", s.reorderSaveSuggestions)
	s.mux.HandleFunc("POST /inventory/reorder/suggestion/status", s.reorderSuggestionStatus)
}

// inventarioHub responde a GET /inventory (la antigua página de inventario).
// Redirige 302 a /inventory/reorder, que es el módulo principal de inventario
// que reemplazó al hub: así los enlaces "Volver a Inventario" dejan de dar 404
// sin forzar un destino que no es el adecuado.
func (s *Server) inventarioHub(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/inventory/reorder", http.StatusFound)
}

func (s *Server) reorderPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "inventory_reorder.html", map[string]interface{}{"Title": "Punto de pedido"})
}

func (s *Server) reorderData(w http.ResponseWriter, r *http.Request) {
	dias, _ := strconv.Atoi(r.FormValue("dias"))
	sugerencias, err := s.SugerirReposiciones(r, dias)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	politicas, err := s.ListarPoliticas(r, false)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var costo float64
	for _, g := range sugerencias {
		costo += g.EstimatedCost
	}
	writeJSON(w, map[string]interface{}{
		"data":      sugerencias,
		"politicas": politicas,
		"dias":      dias,
		"total":     costo,
		"productos": len(sugerencias),
		"revision":  revisarDiasRevision,
		"generado":  now(),
	})
}

func (s *Server) reorderPolicySave(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "create") {
		jsonError(w, "No tiene permiso para configurar la reposición", 403)
		return
	}
	producto, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	plazo, _ := strconv.Atoi(r.FormValue("lead_time_days"))
	colchon, _ := strconv.Atoi(r.FormValue("safety_days"))
	minimo, _ := strconv.ParseFloat(r.FormValue("min_order_qty"), 64)
	multiplo, _ := strconv.ParseFloat(r.FormValue("pack_size"), 64)
	err := s.SetPoliticaReorden(r, PoliticaReorden{
		ProductID:    producto,
		LeadTimeDays: plazo,
		SafetyDays:   colchon,
		MinOrderQty:  minimo,
		PackSize:     multiplo,
		Active:       r.FormValue("active") != "no",
		Note:         r.FormValue("note"),
	})
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Política guardada", "redirect": saveRedirect(r, "/inventory/reorder")})
}

func (s *Server) reorderRecalc(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "create") {
		jsonError(w, "No tiene permiso para recalcular la demanda", 403)
		return
	}
	dias, _ := strconv.Atoi(r.FormValue("dias"))
	n, err := s.RecalcularDemandas(r, dias)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Demanda recalculada", "productos": n})
}

func (s *Server) reorderSaveSuggestions(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "create") {
		jsonError(w, "No tiene permiso para guardar sugerencias", 403)
		return
	}
	dias, _ := strconv.Atoi(r.FormValue("dias"))
	sugerencias, err := s.SugerirReposiciones(r, dias)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var costo float64
	for _, g := range sugerencias {
		if _, err := s.GuardarSugerencia(r, g); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		costo += g.EstimatedCost
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Sugerencias guardadas",
		"productos": len(sugerencias), "total": costo,
	})
}

func (s *Server) reorderSuggestionStatus(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "inventory", "edit") {
		jsonError(w, "No tiene permiso para actualizar sugerencias", 403)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	err := s.CambiarEstadoSugerencia(r, id, r.FormValue("status"), r.FormValue("reason"))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Sugerencia actualizada", "redirect": saveRedirect(r, "/inventory/reorder")})
}
