package web

import (
	"net/http"
)

// ---------- Voucher multi-line extension (adds /account/voucher/new-multi) ----------

func (s *Server) registerVoucherExtraRoutes() {
	s.mux.HandleFunc("GET /account/voucher/new-multi", s.voucherMultiForm)
}

// voucherMultiForm renders the advanced multi-line voucher form with dynamic JS rows.
func (s *Server) voucherMultiForm(w http.ResponseWriter, r *http.Request) {
	vtype := r.FormValue("type")
	if vtype == "" {
		vtype = "JV"
	}
	titles := map[string]string{
		"DV":     "Comprobante de Débito (Multi-Línea)",
		"CV":     "Comprobante de Crédito (Multi-Línea)",
		"CONTRA": "Comprobante Contra (Multi-Línea)",
		"JV":     "Comprobante de Diario (Multi-Línea)",
	}
	s.render(w, r, "voucher_multi.html", map[string]interface{}{
		"Title": titles[vtype],
		"VType": vtype,
		"Heads": s.transactionalHeads(r),
	})
}

// REGISTRO: agrega en server.go dentro de NewServer():
// s.registerVoucherExtraRoutes()
