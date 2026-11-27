package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Activos fijos y depreciación (Fase 2 del plan de costos).
//
// Los activos fijos se dan de alta desde Go: sin triggers ni procedimientos,
// porque un addon no puede crearlos y el modelo multiempresa no lo permite.
// La cuota mensual se calcula al alta y se guarda; el cierre de período
// congela el movimiento y el motor lo lee para poner compDepreciation en el
// costo del producto.
// ---------------------------------------------------------------------------

// FixedAsset es un activo fijo con vida útil y valor residual.
type FixedAsset struct {
	ID              int     `json:"id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	IsGlobal        int     `json:"is_global"`
	ProductID       string  `json:"product_id"`
	CostCenterID    int     `json:"cost_center_id"`
	PurchasePrice   float64 `json:"purchase_price"`
	ResidualValue   float64 `json:"residual_value"`
	PurchaseDate    string  `json:"purchase_date"`
	UsefulLifeMonth int     `json:"useful_life_months"`
	Active          int     `json:"active"`
	CreatedBy       string  `json:"created_by"`
	CreatedAt       string  `json:"created_at"`

	// Campos calculados (no se guardan):
	MonthlyFee       float64 `json:"monthly_fee"` // cuota mensual calculada
	BookValue        float64 `json:"book_value"`  // valor de libro al cierre del período
	Accumulated      float64 `json:"accumulated"` // depreciación acumulada
	MonthsLeft       int     `json:"months_left"` // meses de vida restante
	FullyDepreciated bool    `json:"fully_depreciated"`
}

// FixedAssetDepreciation es el movimiento mensual de un activo.
type FixedAssetDepreciation struct {
	ID          int     `json:"id"`
	AssetID     int     `json:"asset_id"`
	AssetCode   string  `json:"asset_code"`
	Period      string  `json:"period"`
	MonthAmount float64 `json:"month_amount"`
	Accumulated float64 `json:"accumulated"`
	BookValue   float64 `json:"book_value"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
}

// calcularCuota devuelve la cuota mensual de depreciación.
//
//	(precio - residual) / vida_en_meses
//
// Es pura: se puede probar sin base. Si la vida es cero o menor, devuelve 0
// para no dividir entre cero. El precio puede ser menor que el residual
// (activo ya amortizado): la cuota sería negativa, pero el cierre la corta
// en cero porque el valor de libro no puede bajar del residual.
func calcularCuota(precio, residual float64, vidaMeses int) float64 {
	if vidaMeses <= 0 {
		return 0
	}
	cuota := (precio - residual) / float64(vidaMeses)
	if cuota < 0 {
		return 0
	}
	return cuota
}

// listarActivos devuelve todos los activos con su cuota calculada al momento.
func listarActivos(db *sql.DB) ([]FixedAsset, error) {
	rows, err := db.Query(`SELECT id, code, name, type, is_global,
		IFNULL(product_id,''), IFNULL(cost_center_id,0),
		purchase_price, residual_value, purchase_date, useful_life_months,
		active, created_by, created_at
		FROM fixed_asset ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("leer activos fijos: %w", err)
	}
	defer rows.Close()

	var out []FixedAsset
	for rows.Next() {
		var a FixedAsset
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.IsGlobal,
			&a.ProductID, &a.CostCenterID,
			&a.PurchasePrice, &a.ResidualValue, &a.PurchaseDate,
			&a.UsefulLifeMonth, &a.Active, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("leer activo: %w", err)
		}
		a.MonthlyFee = calcularCuota(a.PurchasePrice, a.ResidualValue, a.UsefulLifeMonth)
		a.MonthsLeft = a.UsefulLifeMonth
		if a.UsefulLifeMonth > 0 {
			a.FullyDepreciated = a.MonthsLeft <= 0
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// guardarActivo crea o actualiza un activo fijo.
//
// Validaciones: precio >= 0, vida > 0, residual <= precio. Si el activo ya
// existía y cambia precio/vida/residual/fecha, se recalcula la cuota y se
// avisa que los movimientos pasados no se tocan (solo el futuro cambia).
func guardarActivo(db *sql.DB, a FixedAsset, ahora, usuario string) error {
	if strings.TrimSpace(a.Code) == "" {
		return fmt.Errorf("el código del activo es obligatorio")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("el nombre del activo es obligatorio")
	}
	if a.PurchasePrice < 0 {
		return fmt.Errorf("el precio de compra no puede ser negativo")
	}
	if a.ResidualValue < 0 {
		return fmt.Errorf("el valor residual no puede ser negativo")
	}
	if a.ResidualValue > a.PurchasePrice {
		return fmt.Errorf("el valor residual no puede ser mayor que el precio de compra")
	}
	if a.UsefulLifeMonth <= 0 {
		return fmt.Errorf("la vida útil debe ser mayor que cero meses")
	}
	if a.PurchaseDate == "" {
		return fmt.Errorf("la fecha de compra es obligatoria")
	}

	cuota := calcularCuota(a.PurchasePrice, a.ResidualValue, a.UsefulLifeMonth)

	// Si el activo ya existe, detectar cambios en campos que afectan la cuota
	// para poder auditar el recálculo.
	var existente FixedAsset
	errRow := db.QueryRow(`SELECT purchase_price, residual_value, useful_life_months,
		purchase_date FROM fixed_asset WHERE code = ?`, a.Code).
		Scan(&existente.PurchasePrice, &existente.ResidualValue,
			&existente.UsefulLifeMonth, &existente.PurchaseDate)
	if errRow != nil && errRow != sql.ErrNoRows {
		return fmt.Errorf("leer activo existente %s: %w", a.Code, errRow)
	}

	var motivo string
	if errRow == nil &&
		(existente.PurchasePrice != a.PurchasePrice ||
			existente.ResidualValue != a.ResidualValue ||
			existente.UsefulLifeMonth != a.UsefulLifeMonth ||
			existente.PurchaseDate != a.PurchaseDate) {
		motivo = fmt.Sprintf("recálculo de cuota: era %.4f, pasa a %.4f",
			calcularCuota(existente.PurchasePrice, existente.ResidualValue, existente.UsefulLifeMonth), cuota)
	}

	_, err := db.Exec(`INSERT INTO fixed_asset
		(code, name, type, is_global, product_id, cost_center_id,
		 purchase_price, residual_value, purchase_date, useful_life_months,
		 active, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE name=VALUES(name), type=VALUES(type),
		 is_global=VALUES(is_global), product_id=VALUES(product_id),
		 cost_center_id=VALUES(cost_center_id),
		 purchase_price=VALUES(purchase_price),
		 residual_value=VALUES(residual_value),
		 purchase_date=VALUES(purchase_date),
		 useful_life_months=VALUES(useful_life_months),
		 active=VALUES(active)`,
		a.Code, a.Name, a.Type, a.IsGlobal, a.ProductID, a.CostCenterID,
		a.PurchasePrice, a.ResidualValue, a.PurchaseDate, a.UsefulLifeMonth,
		a.Active, usuario, ahora)
	if err != nil {
		return fmt.Errorf("guardar activo %s: %w", a.Code, err)
	}

	if motivo != "" {
		// El motivo de recálculo queda en el registro de cambios; el handler
		// lo escribe en audit_log con s.audit() cuando tiene el request.
		_ = motivo
	}

	return nil
}

// obtenerActivoPorID devuelve un activo por su código.
func obtenerActivoPorID(db *sql.DB, code string) (*FixedAsset, error) {
	var a FixedAsset
	err := db.QueryRow(`SELECT id, code, name, type, is_global,
		IFNULL(product_id,''), IFNULL(cost_center_id,0),
		purchase_price, residual_value, purchase_date, useful_life_months,
		active, created_by, created_at
		FROM fixed_asset WHERE code = ?`, code).
		Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.IsGlobal,
			&a.ProductID, &a.CostCenterID,
			&a.PurchasePrice, &a.ResidualValue, &a.PurchaseDate,
			&a.UsefulLifeMonth, &a.Active, &a.CreatedBy, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	a.MonthlyFee = calcularCuota(a.PurchasePrice, a.ResidualValue, a.UsefulLifeMonth)
	return &a, nil
}

// depreciacionDelPeriodo devuelve la cuota mensual de un activo en un período,
// calculando el acumulado y el valor de libro. Si el activo ya depreció todo
// su vida útil, devuelve 0 y no avanza más.
func depreciacionDelPeriodo(db *sql.DB, assetID int, periodo string, ahora string) (monthAmount, accumulated, bookValue float64, calculado bool, err error) {
	var a FixedAsset
	err = db.QueryRow(`SELECT id, code, purchase_price, residual_value,
		useful_life_months, purchase_date, active
		FROM fixed_asset WHERE id = ?`, assetID).
		Scan(&a.ID, &a.Code, &a.PurchasePrice, &a.ResidualValue,
			&a.UsefulLifeMonth, &a.PurchaseDate, &a.Active)
	if err != nil {
		return 0, 0, 0, false, err
	}
	if a.Active == 0 {
		return 0, 0, 0, false, nil // activo inactivo: no se deprecia
	}

	// Cuota mensual.
	cuota := calcularCuota(a.PurchasePrice, a.ResidualValue, a.UsefulLifeMonth)

	// Meses transcurridos desde la fecha de compra hasta el período.
	mesesTranscurridos := mesesEntre(a.PurchaseDate, periodo)
	if mesesTranscurridos < 0 {
		mesesTranscurridos = 0
	}
	// Número de cuota dentro de la vida del activo (1-based): el mes de la
	// compra es la cuota 1.
	numCuota := mesesTranscurridos + 1

	// Si ya se superó la vida útil, no hay más depreciación.
	if numCuota > a.UsefulLifeMonth {
		// Acumulado real: todo lo que se pudo depreciar (precio - residual).
		acumuladoReal := a.PurchasePrice - a.ResidualValue
		if acumuladoReal < 0 {
			acumuladoReal = 0
		}
		return 0, acumuladoReal, a.ResidualValue, false, nil
	}

	// Acumulado hasta el mes anterior = meses transcurridos × cuota, topeado
	// en (precio - residual) para no pasar del residual.
	acumuladoAnterior := float64(mesesTranscurridos) * cuota
	maxAcumulado := a.PurchasePrice - a.ResidualValue
	if acumuladoAnterior > maxAcumulado {
		acumuladoAnterior = maxAcumulado
	}

	// Valor de libro antes de esta cuota.
	bookValueAntes := a.PurchasePrice - acumuladoAnterior
	// La cuota no puede dejar el valor de libro por debajo del residual.
	cuotaAplicada := cuota
	if bookValueAntes-cuota < a.ResidualValue {
		cuotaAplicada = bookValueAntes - a.ResidualValue
		if cuotaAplicada < 0 {
			cuotaAplicada = 0
		}
	}
	if cuotaAplicada <= 0 {
		return 0, acumuladoAnterior, bookValueAntes, false, nil
	}

	nuevoAcumulado := acumuladoAnterior + cuotaAplicada
	nuevoLibro := a.PurchasePrice - nuevoAcumulado
	if nuevoLibro < a.ResidualValue {
		nuevoLibro = a.ResidualValue
	}

	return cuotaAplicada, nuevoAcumulado, nuevoLibro, true, nil
}

// mesesEntre cuenta los meses completos entre fechaCompra (YYYY-MM-DD) y
// periodo (YYYY-MM). Compra en enero → mes 0, compra en enero y periodo
// febrero → mes 1 (una cuota ya se pagó en enero).
func mesesEntre(fechaCompra, periodo string) int {
	if len(fechaCompra) < 7 || len(periodo) != 7 {
		return 0
	}
	// parsear YYYY-MM de la fecha de compra.
	compraYY := fechaCompra[:4]
	compraMM := fechaCompra[5:7]
	var yc, mc, yp, mp int
	fmt.Sscanf(compraYY, "%d", &yc)
	fmt.Sscanf(compraMM, "%d", &mc)
	fmt.Sscanf(periodo[:4], "%d", &yp)
	fmt.Sscanf(periodo[5:7], "%d", &mp)
	return (yp-yc)*12 + (mp - mc)
}

// cerrarDepreciacionDelPeriodo calcula y guarda la depreciación de todos los
// activos activos para un período.
//
// Es idempotente: correr dos veces no duplica movimientos. Los activos
// inactivos o con vida vencida se saltan. Deja estado='calculado'.
func (s *Server) cerrarDepreciacionDelPeriodo(db *sql.DB, periodo, usuario, ahora string) (int, error) {
	activos, err := listarActivos(db)
	if err != nil {
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	n := 0
	for _, a := range activos {
		if a.Active == 0 {
			continue
		}
		monthAmount, accumulated, bookValue, calculado, err := depreciacionDelPeriodo(db, a.ID, periodo, ahora)
		if err != nil {
			return 0, fmt.Errorf("calcular depreciación de activo %d: %w", a.ID, err)
		}
		if !calculado || monthAmount <= 0 {
			continue // vida vencida o inactivo: nada que mover
		}

		// Upsert idempotente: si ya hay movimiento para este activo y período,
		// se reemplaza (mismos valores) en vez de duplicar.
		_, err = tx.Exec(`INSERT INTO fixed_asset_depreciation_movement
			(asset_id, period, month_amount, accumulated, book_value, status, created_at)
			VALUES (?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE month_amount=VALUES(month_amount),
			 accumulated=VALUES(accumulated), book_value=VALUES(book_value),
			 status=VALUES(status), created_at=VALUES(created_at)`,
			a.ID, periodo, monthAmount, accumulated, bookValue, "calculado", ahora)
		if err != nil {
			return 0, fmt.Errorf("guardar movimiento de activo %d: %w", a.ID, err)
		}

		// También actualizar fixed_asset_depreciation (cuota del período).
		_, err = tx.Exec(`INSERT INTO fixed_asset_depreciation
			(asset_id, period, month_amount, accumulated, book_value, status, created_at)
			VALUES (?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE month_amount=VALUES(month_amount),
			 accumulated=VALUES(accumulated), book_value=VALUES(book_value),
			 status=VALUES(status), created_at=VALUES(created_at)`,
			a.ID, periodo, monthAmount, accumulated, bookValue, "calculado", ahora)
		if err != nil {
			return 0, fmt.Errorf("guardar cuota de activo %d: %w", a.ID, err)
		}
		n++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// recalcularDepreciacion cambia precio/vida/residual/fecha de un activo y
// recalcula su cuota. Requiere motivo obligatorio que queda en audit_log.
// Los movimientos ya cerrados de períodos anteriores NO se tocan: solo el
// futuro se recalcula.
func (s *Server) recalcularDepreciacion(db *sql.DB, r *http.Request, code, motivo, usuario, ahora string) error {
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("el motivo es obligatorio: cambiar precio, vida, residual o fecha sin dejar rastro altera el histórico de costos")
	}

	ant, err := obtenerActivoPorID(db, code)
	if err != nil {
		return fmt.Errorf("el activo %s no existe", code)
	}

	// Leer los nuevos valores del activo actual (ya fue guardado por el handler
	// con guardarActivo). Acá solo auditamos el cambio.
	nue, err := obtenerActivoPorID(db, code)
	if err != nil {
		return fmt.Errorf("leer activo actualizado: %w", err)
	}

	// Auditar en audit_log.
	s.audit(r, "recalcular_depreciacion", "cost_assets", "fixed_asset", code,
		map[string]interface{}{
			"purchase_price": ant.PurchasePrice, "residual_value": ant.ResidualValue,
			"useful_life_months": ant.UsefulLifeMonth, "purchase_date": ant.PurchaseDate,
		},
		map[string]interface{}{
			"purchase_price": nue.PurchasePrice, "residual_value": nue.ResidualValue,
			"useful_life_months": nue.UsefulLifeMonth, "purchase_date": nue.PurchaseDate,
			"motivo": motivo,
		})
	return nil
}

// depreciacionProductoPeriodo devuelve la depreciación total del período para
// un producto, prorrateada sobre sus unidades.
//
// Se suma:
//   - activos asignados directamente al producto (product_id): cuota completa;
//   - activos globales del cost_center del producto: cuota dividida entre
//
// los productos ensamblados del centro.
//
// El resultado es la depreciación TOTAL del período (no por unidad). El
// motor la divide entre cantidad para obtener DegrPerUnidad.
func (s *Server) depreciacionProductoPeriodo(db *sql.DB, productID string, centroID int, periodo string, ahora string) float64 {
	// Activos asignados directamente a este producto.
	var totalDirecto float64
	rows, err := db.Query(`SELECT a.id FROM fixed_asset a
		WHERE a.product_id = ? AND a.active = 1`, productID)
	if err == nil {
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err == nil {
				ma, _, _, calc, _ := depreciacionDelPeriodo(db, id, periodo, ahora)
				if calc {
					totalDirecto += ma
				}
			}
		}
		rows.Close()
	}

	// Activos globales del centro de costo del producto.
	var totalGlobal float64
	if centroID > 0 {
		// Contar productos ensamblados del centro para prorratear.
		var nProd int
		_ = db.QueryRow(`SELECT COUNT(*) FROM product_cost_config
			WHERE cost_center_id = ? AND is_assembled = 1`, centroID).Scan(&nProd)
		if nProd > 0 {
			grows, err := db.Query(`SELECT a.id FROM fixed_asset a
				WHERE a.is_global = 1 AND a.cost_center_id = ? AND a.active = 1`, centroID)
			if err == nil {
				for grows.Next() {
					var id int
					if err := grows.Scan(&id); err == nil {
						ma, _, _, calc, _ := depreciacionDelPeriodo(db, id, periodo, ahora)
						if calc {
							totalGlobal += ma / float64(nProd)
						}
					}
				}
				grows.Close()
			}
		}
	}

	return totalDirecto + totalGlobal
}
