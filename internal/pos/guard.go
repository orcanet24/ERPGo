package pos

// guard.go — Punto de veto externo de una venta.
//
// El guard NO escribe: recibe las líneas, devuelve motivos y el núcleo decide.
// Vive aquí porque el paquete pos no puede importar interfaces/web (sería un
// ciclo: web ya importa pos). El host instala la función al arrancar, y si no
// hay addons con guard la variable queda en nil y no cuesta nada.

import (
	"context"
	"database/sql"
	"strings"
)

// GuardItem es una línea de venta tal como la ve el guard.
type GuardItem struct {
	ProductID string
	Barcode   string
	Quantity  float64
}

// GuardBlock es un motivo para no vender una línea.
type GuardBlock struct {
	Code      string
	ProductID string
	Message   string
}

// SaleGuardFunc evalúa las políticas externas de una venta.
type SaleGuardFunc func(ctx context.Context, db *sql.DB, terminalID string, items []GuardItem) []GuardBlock

// SaleGuard la instala el host. Es una variable de paquete y no un campo del
// servidor gRPC porque el addon es del proceso, no de una conexión: el mismo
// ERP sirve a todos los terminales.
var SaleGuard SaleGuardFunc

// SaleBlockedError distingue "la política dijo que no" de "algo falló". El
// terminal debe mostrar el motivo al cajero, no un error interno de servidor.
type SaleBlockedError struct {
	Blocks []GuardBlock
}

func (e *SaleBlockedError) Error() string {
	var parts []string
	for _, b := range e.Blocks {
		if b.ProductID != "" {
			parts = append(parts, b.Code+": "+b.ProductID+": "+b.Message)
		} else {
			parts = append(parts, b.Code+": "+b.Message)
		}
	}
	return strings.Join(parts, " | ")
}

// evaluarGuard llama a la política si existe. Un guard ausente no bloquea.
func evaluarGuard(ctx context.Context, db *sql.DB, terminalID string, items []GuardItem) error {
	if SaleGuard == nil || db == nil || len(items) == 0 {
		return nil
	}
	blocks := SaleGuard(ctx, db, terminalID, items)
	if len(blocks) == 0 {
		return nil
	}
	return &SaleBlockedError{Blocks: blocks}
}
