package web

// mensajeria_areas.go — Los avisos que salen solos cuando ocurre un hecho.
//
// QUÉ ES UN «AVISO POR ÁREA». Facturar, comprar o retener son hechos del
// negocio; el cliente o el proveedor espera saberlo. Este archivo conecta esos
// hechos con la cola de mensajería sin que el handler de facturación tenga que
// saber nada de WhatsApp.
//
// TRES REGLAS QUE NO SE NEGOCIAN:
//
//  1. El aviso NO puede romper la operación. Se llama en una goroutine, con
//     recover, y cualquier fallo se queda en el log. Una factura no se cae
//     porque el número de WhatsApp esté desconectado.
//  2. Sin canal configurado para el área, no pasa nada. No se inventa un
//     número ni se manda por otro canal.
//  3. El aviso sale del REGISTRO (cliente/proveedor de la ficha), nunca de un
//     dato que venga del formulario. Es la misma regla de outbound.go.
//
// El canal de un área se resuelve por el ÁREA DEL DISPOSITIVO: si hay un número
// marcado «facturación», los avisos de facturación salen por él. Así no hay dos
// sitios donde decir a qué área pertenece un número.

import (
	"database/sql"
	"fmt"
	"log"
)

// msgAvisarArea encola un aviso para el área indicada. Es seguro llamarla desde
// una goroutine: no devuelve error y no propaga pánicos.
func (s *Server) msgAvisarArea(tenant, area, refType, refID, partyType string, partyID int64,
	asunto, cuerpo string, monto float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mensajería: aviso de %s para %s/%s abortado: %v", area, refType, refID, r)
		}
	}()

	ctl, err := s.controlStore()
	if err != nil || ctl == nil {
		return
	}
	if !msgGlobalActivo(ctl) {
		return // la instalación tiene el servicio apagado
	}
	db, err := s.tenantDB(tenant)
	if err != nil || db == nil {
		return
	}
	if !msgEmpresaYModuloActivos(db) {
		return // la empresa o el módulo están apagados
	}
	canal, ok := s.msgCanalDeArea(db, ctl, tenant, area)
	if !ok {
		return // no hay número para esta área: no se inventa uno
	}
	alerta := map[string]interface{}{
		"party_type": partyType,
		"party_id":   partyID,
		"amount":     monto,
		"kind":       refType,
	}
	req := s.requestForTenant(tenant)
	if _, err := s.financeEnviarAviso(req, db, canal, alerta, asunto, cuerpo, refType, refID, false); err != nil {
		log.Printf("mensajería: aviso de %s para %s/%s no se pudo encolar: %v", area, refType, refID, err)
	}
}

// msgEmpresaYModuloActivos comprueba las dos llaves que dependen de la empresa.
// La de la instalación ya se comprobó antes.
func msgEmpresaYModuloActivos(db *sql.DB) bool {
	var modulo int
	if err := db.QueryRow(`SELECT is_active FROM modules WHERE id = 'mensajeria'`).Scan(&modulo); err != nil || modulo != 1 {
		return false
	}
	var empresa int
	if err := db.QueryRow(`SELECT enabled FROM msg_setting ORDER BY id LIMIT 1`).Scan(&empresa); err != nil || empresa != 1 {
		return false
	}
	return true
}

// msgCanalDeArea busca un canal GOWA activo cuyo dispositivo sea de esa área.
//
// Los dispositivos viven en control y los canales en la empresa, así que no hay
// un JOIN posible: se leen los dispositivos del área y, por cada uno, se busca
// su canal. Son pocos y la consulta solo corre cuando hay un hecho que avisar.
func (s *Server) msgCanalDeArea(db, ctl *sql.DB, tenant, area string) (notifyChannel, bool) {
	rows, err := ctl.Query(`SELECT device_id FROM msg_device
		WHERE tenant = ? AND area = ? AND state <> 'retirado' ORDER BY id`, tenant, area)
	if err != nil {
		return notifyChannel{}, false
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()

	for _, id := range ids {
		c, err := notifyCanalEscanear(db.QueryRow(notifyCanalesSQL+
			` WHERE kind = 'gowa' AND is_active = 1 AND msg_device_id = ? LIMIT 1`, id))
		if err == nil {
			return c, true
		}
	}
	return notifyChannel{}, false
}

// msgAvisarFacturaEmitida se llama después de registrar una factura (no un
// borrador). El texto por defecto se usa si el canal no tiene plantilla.
func (s *Server) msgAvisarFacturaEmitida(tenant string, customerID, invoiceNo int64, total float64) {
	asunto := fmt.Sprintf("Factura %d", invoiceNo)
	cuerpo := fmt.Sprintf("Le informamos que se emitió la factura %d por un total de %.2f. Gracias por su compra.", invoiceNo, total)
	s.msgAvisarArea(tenant, "facturacion", "factura", fmt.Sprintf("%d", invoiceNo), "customer", customerID,
		asunto, cuerpo, total)
}

// msgAvisarCompraRegistrada se llama después de registrar una compra.
func (s *Server) msgAvisarCompraRegistrada(tenant string, supplierID, purchaseID int64, referencia string, total float64) {
	asunto := "Compra " + referencia
	cuerpo := fmt.Sprintf("Le informamos que registramos la compra %s por un total de %.2f.", referencia, total)
	s.msgAvisarArea(tenant, "compras", "compra", fmt.Sprintf("%d", purchaseID), "supplier", supplierID,
		asunto, cuerpo, total)
}

// msgAvisarRetencionEmitida se llama después de registrar una retención de IVA.
func (s *Server) msgAvisarRetencionEmitida(tenant string, supplierID int64, retentionNo string, monto float64) {
	asunto := "Retención de IVA " + retentionNo
	cuerpo := fmt.Sprintf("Le informamos que se emitió el comprobante de retención de IVA %s por un monto de %.2f.", retentionNo, monto)
	s.msgAvisarArea(tenant, "cxp", "retencion_iva", retentionNo, "supplier", supplierID,
		asunto, cuerpo, monto)
}
