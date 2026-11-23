package web

// adjustment_revert_test.go — D-4 (auditoría atómica 2026-11-22): el ajuste
// mal registrado se CORRIGE con su reverso, nunca se borra. La prueba fija
// las tres verdades del candado: el reverso existe con los tipos invertidos
// y el stock devuelto a su sitio; el original sigue vivo con su kardex y su
// asiento (nada de DELETEs); y dos reversiones del mismo ajuste es 409.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func baseAjustePrueba(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := baseTerminalPrueba(t)
	if _, err := db.Exec(`INSERT INTO stock_movement (movement_type, product_id, warehouse_id, quantity, unit_cost,
			reference_type, reference_id, reference_no, date, is_posted, posted_at, created_at)
		VALUES ('purchase', 1, 'WH-MAIN', 10, 4, 'prueba', 0, 'COMPRA-INICIAL', '2026-11-01', 1, '2026-11-01 08:00:00', '2026-11-01 08:00:00')`); err != nil {
		t.Fatalf("sembrando compra inicial: %v", err)
	}
	// El asiento del ajuste se valoriza al COSTO PROMEDIO DE COMPRAS
	// (product_purchase_details): sin esta fila el monto sale 0 y no hay
	// asientos que probar (la política es no asentar ceros).
	if _, err := db.Exec(`INSERT INTO product_purchase_details (product_id, quantity, total_amount, rate,
			concept, item_type, warehouse_id, discount_amnt, vat_amnt, vat_amnt_per)
		VALUES (1, 10, 40, 4, 'compra inicial de prueba', 'venta', 'WH-MAIN', 0, 0, 0)`); err != nil {
		t.Fatalf("sembrando detalle de compra: %v", err)
	}
	return s, db
}

func TestAjusteRevertCreaInversoYConservaElOriginal(t *testing.T) {
	s, db := baseAjustePrueba(t)

	// Alta normal: quitar 4 unidades.
	r := httptest.NewRequest(http.MethodPost, "/adjustment/save", strings.NewReader(url.Values{
		"product_id":      []string{"P-001"},
		"adjust_type":     []string{"remove"},
		"quantity":        []string{"4"},
		"warehouse_id":    []string{"WH-MAIN"},
		"adjustment_date": []string{"2026-11-05"},
	}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withScope(r, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	w := httptest.NewRecorder()
	s.adjustmentSave(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("alta del ajuste respondió %d: %s", w.Code, w.Body.String())
	}
	var idAjuste int64
	var adjNo string
	if err := db.QueryRow(`SELECT id, adjustment_no FROM store_adjustment WHERE status = 1 ORDER BY id DESC LIMIT 1`).Scan(&idAjuste, &adjNo); err != nil {
		t.Fatalf("el ajuste no quedó registrado: %v", err)
	}
	stockTrasAjuste := stockSQLite(t, db)
	if stockTrasAjuste != 6 {
		t.Fatalf("stock tras quitar 4 = %v, se quería 6", stockTrasAjuste)
	}

	// Reversión del ajuste.
	rv := httptest.NewRequest(http.MethodPost, "/adjustment/revert/1", nil)
	rv = withScope(rv, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	rv.SetPathValue("id", strconv.FormatInt(idAjuste, 10))
	wv := httptest.NewRecorder()
	s.adjustmentRevert(wv, rv)
	if wv.Code != http.StatusOK {
		t.Fatalf("reversión respondió %d: %s", wv.Code, wv.Body.String())
	}

	// 1. El reverso existe, enlazado y con los tipos invertidos.
	var adjReverso, ref string
	if err := db.QueryRow(`SELECT adjustment_no, reference FROM store_adjustment
		WHERE status = 1 AND id <> ? ORDER BY id DESC LIMIT 1`, idAjuste).Scan(&adjReverso, &ref); err != nil {
		t.Fatalf("no existe el ajuste reverso: %v", err)
	}
	if ref != "REVERTIR:"+adjNo {
		t.Errorf("reference del reverso = %q, se quería %q", ref, "REVERTIR:"+adjNo)
	}
	var tipoReverso string
	var qtyReverso float64
	if err := db.QueryRow(`SELECT type, quantity FROM store_adjustment_details WHERE adjustment_no = ?`, adjReverso).Scan(&tipoReverso, &qtyReverso); err != nil {
		t.Fatalf("el reverso no tiene detalle: %v", err)
	}
	if tipoReverso != "add" || qtyReverso != 4 {
		t.Errorf("reverso %s x%v, se quería add x4", tipoReverso, qtyReverso)
	}

	// 2. El stock volvió a su sitio sin borrar NINGÚN movimiento: los dos
	//    kardex (quita y pone) conviven.
	if got := stockSQLite(t, db); got != 10 {
		t.Errorf("stock tras revertir = %v, se quería 10", got)
	}
	var movs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stock_movement WHERE reference_type = 'adjustment'`).Scan(&movs); err != nil {
		t.Fatal(err)
	}
	if movs != 2 {
		t.Errorf("movimientos de ajuste = %d, se quería 2 (el original sigue en el kardex)", movs)
	}

	// 3. El original sigue VIVO y su contabilidad intacta.
	var statusOrig int
	if err := db.QueryRow(`SELECT status FROM store_adjustment WHERE id = ?`, idAjuste).Scan(&statusOrig); err != nil {
		t.Fatal(err)
	}
	if statusOrig != 1 {
		t.Errorf("el original quedó status=%d: revertir no puede desactivar ni borrar el documento origen", statusOrig)
	}
	var asientosOrig, asientosRev int
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_vaucher WHERE referenceNo = ?`, adjNo).Scan(&asientosOrig); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_vaucher WHERE referenceNo = ?`, adjReverso).Scan(&asientosRev); err != nil {
		t.Fatal(err)
	}
	if asientosOrig == 0 || asientosRev == 0 {
		t.Errorf("asientos: original=%d reverso=%d — ambos documentos deben tener el suyo", asientosOrig, asientosRev)
	}

	// 4. Doble reversión: candado de estado (409), no dos espejos.
	rv2 := httptest.NewRequest(http.MethodPost, "/adjustment/revert/1", nil)
	rv2 = withScope(rv2, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("master")})
	rv2.SetPathValue("id", strconv.FormatInt(idAjuste, 10))
	w2 := httptest.NewRecorder()
	s.adjustmentRevert(w2, rv2)
	if w2.Code != http.StatusConflict {
		t.Errorf("segunda reversión respondió %d (%s), se quería 409", w2.Code, w2.Body.String())
	}
}

func stockSQLite(t *testing.T, db *sql.DB) float64 {
	t.Helper()
	var v float64
	if err := db.QueryRow(`SELECT IFNULL(SUM(CASE WHEN movement_type IN ('sale','delivery','consumo','adjustment_remove') THEN -quantity ELSE quantity END), 0)
		FROM stock_movement WHERE product_id = 1`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
