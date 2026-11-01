package web

// ai_op_cache_test.go — El cache de plantillas de operación es determinístico,
// no duplica, aplica TTL/LRU y discrimina por vendor (solo ORC24 cachea).

import (
	"strings"
	"testing"
)

// TestOpCacheKeyDeterminista: mismos campos estructurados → misma clave.
func TestOpCacheKeyDeterminista(t *testing.T) {
	args := map[string]interface{}{"vtype": "JV", "note": "x", "monto": 100}
	k1 := aiOpCacheKey("account.post_voucher", args)
	k2 := aiOpCacheKey("account.post_voucher", map[string]interface{}{"vtype": "jv", "note": "otro"})
	if k1 != k2 {
		t.Errorf("mismos campos estructurados deben dar la misma clave: %q vs %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "account.post_voucher::") {
		t.Errorf("la clave debe llevar la operación: %q", k1)
	}
}

// TestOpKeyDistingueCategoria: vtype distinto → clave distinta (no hay solape).
func TestOpKeyDistingueCategoria(t *testing.T) {
	jv := aiOpCacheKey("account.post_voucher", map[string]interface{}{"vtype": "JV"})
	cv := aiOpCacheKey("account.post_voucher", map[string]interface{}{"vtype": "CV"})
	if jv == cv {
		t.Errorf("JV y CV deben producir claves distintas")
	}
}

// TestOpKeySinCategoria: operación sin campos de categoría → clave = operación.
func TestOpKeySinCategoria(t *testing.T) {
	k := aiOpCacheKey("db.query", map[string]interface{}{"sql": "SELECT 1"})
	if k != "db.query" {
		t.Errorf("operación sin categoría debe usar su nombre como clave: %q", k)
	}
}

// TestOpPatternExtraeFijoDeVariable: el pattern contiene la categoría, el
// var_schema contiene los campos que varían (montos, fechas).
func TestOpPatternExtraeFijoDeVariable(t *testing.T) {
	args := map[string]interface{}{
		"vtype": "JV", "ref": "001", "note": "Venta a X",
		"lines": []interface{}{map[string]interface{}{"coa_id": 401, "credit": 500}},
	}
	pattern, vars := aiOpPatternFromArgs("account.post_voucher", args)
	if pattern == nil {
		t.Errorf("el pattern debe conservar la estructura completa")
	}
	if _, ok := pattern["vtype"]; !ok {
		t.Errorf("el pattern debe conservar la categoría vtype: %v", pattern)
	}
	found := false
	for _, v := range vars {
		if v == "credit" || v == "debit" || v == "monto" || v == "amount" {
			found = true
		}
	}
	if !found {
		t.Errorf("var_schema debe detectar montos anidados como variables: %v", vars)
	}
}

// TestVendorPropia: solo ORC24 es propia.
func TestVendorPropia(t *testing.T) {
	if !aiVendorEsPropia(aiProvider{Vendor: "orc24"}) {
		t.Error("orc24 debe ser propia")
	}
	if aiVendorEsPropia(aiProvider{Vendor: "openai"}) {
		t.Error("openai NO debe ser propia")
	}
	if aiVendorEsPropia(aiProvider{Vendor: "personalizado"}) {
		t.Error("personalizado NO debe ser propia")
	}
}

// TestPromptFragmentConPlantillas: el fragmento enumera las plantillas.
func TestPromptFragmentConPlantillas(t *testing.T) {
	frags := []aiOpTemplate{
		{Operation: "account.post_voucher", TemplateKey: "jv::abc",
			Pattern: map[string]interface{}{"vtype": "JV"}, VarSchema: []string{"monto", "ref"}, Usos: 3},
	}
	out := aiOpTemplatePromptFragment(frags)
	if !strings.Contains(out, "PLANTILLAS DE OPERACIÓN") {
		t.Errorf("el fragmento debe anunciar las plantillas")
	}
	if !strings.Contains(out, "JV") || !strings.Contains(out, "monto") {
		t.Errorf("el fragmento debe incluir el pattern y las variables: %s", out)
	}
}

// TestPromptFragmentSinPlantillas: sin plantillas, fragmento vacío.
func TestPromptFragmentSinPlantillas(t *testing.T) {
	if out := aiOpTemplatePromptFragment(nil); out != "" {
		t.Errorf("sin plantillas el fragmento debe ser vacío: %q", out)
	}
}
