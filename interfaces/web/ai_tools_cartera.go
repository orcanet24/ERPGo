package web

// ai_tools_cartera.go — Las herramientas con las que un agente trabaja la
// cobranza y las cuentas por pagar.
//
// QUÉ PROBLEMA RESUELVE ESTO, Y POR QUÉ NO ES «OTRA CONSULTA SQL».
//
// El trabajo de cartera es repetitivo por naturaleza: mirar quién se atrasó,
// revisar si el atraso es normal o es un cambio, escribirle al cliente, mandarle
// al proveedor su comprobante de retención. Un agente que solo pudiera hacer
// `db.query` podría LEER la cartera y nada más: tendría que devolverle al humano
// la tarea de mandar el correo, que es justo la parte que cansa. Por eso estas
// herramientas existen, y por eso son distintas de una consulta:
//
//   - `finance.receivables` / `finance.payables` son las mismas alertas que ve el
//     operador en pantalla. No una versión para la IA: la MISMA. El agente reporta
//     la lista que el humano encuentra abierta al entrar.
//   - `customer.request_payment` y `supplier.send_retention` son las dos salidas
//     que se piden una y otra vez. Las dos pasan por el mismo camino que el botón
//     de la pantalla: canal configurado, destinatario tomado del registro,
//     enfriamiento e idempotencia, y todo escrito en `finance_message`.
//
// LAS CUATRO REGLAS QUE HACEN QUE ESO SEA ACEPTABLE.
//
//  1. DESTINATARIO DEL REGISTRO. El agente no puede escribir a quien quiera: el
//     correo o el teléfono salen de la ficha del cliente o del proveedor. La
//     herramienta ni siquiera acepta un parámetro «to». Eso es lo que impide que
//     un prompt inyectado dentro de un dato consultado convierta al ERP en
//     mensajero hacia una dirección elegida por un tercero.
//  2. CANAL AUTORIZADO. El canal tiene que existir, estar activo y tener marcado
//     «permitir agentes». La decisión de que un agente pueda sacar información es
//     del administrador, canal por canal.
//  3. NI REPETICIÓN NI INSISTENCIA. La clave de idempotencia impide el envío
//     duplicado del mismo día y el enfriamiento del canal impide el recordatorio
//     diario. Un agente que corre cada cinco minutos no puede convertirse en spam.
//  4. SIEMPRE QUEDA ESCRITO. Con su asunto, su cuerpo y su resultado. La respuesta
//     al modelo incluye el destinatario final, para que su informe pueda decir a
//     quién le escribió de verdad.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ---------- Lectura: los dos tableros ----------

func (s *Server) aiToolFinanceReceivables(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	usuario := s.aiAgentUsername(sess, agent)

	escaneo, err := s.financeEscanear(db, scopeCxC, usuario, false)
	if err != nil {
		return string(mustJSON(aiToolError{"no se pudo recalcular la cartera: " + err.Error()})), nil
	}

	limit := int(aiNumParam(args, "limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	incluirCerradas := aiStrParam(args, "include_closed") == "1" || strings.EqualFold(aiStrParam(args, "include_closed"), "true")

	alertas := s.finAlertas(db, scopeCxC, incluirCerradas, limit)
	criticas, avisos := 0, 0
	for _, a := range alertas {
		switch fmt.Sprintf("%v", a["severity"]) {
		case "critico":
			criticas++
		case "aviso":
			avisos++
		}
	}

	return string(mustJSON(map[string]interface{}{
		"status":      "ok",
		"scope":       scopeCxC,
		"indicadores": s.finSnapshotCxC(db),
		"alertas":     alertas,
		"resumen": map[string]interface{}{
			"alertas": len(alertas), "criticas": criticas, "avisos": avisos,
		},
		"escaneo": escaneo,
		"note": "Esta es LA MISMA lista que ve el operador en Cuentas por Cobrar: no es una versión para agentes. " +
			"Cada alerta trae su motivo, el saldo, los días de atraso y a quién avisarle. " +
			"Para reclamar el pago use customer.request_payment con el alert_id: el aviso sale al correo o teléfono " +
			"registrado en la ficha del cliente, nunca a una dirección que usted escriba.",
	})), nil
}

func (s *Server) aiToolFinancePayables(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	escaneo, err := s.financeEscanear(db, scopeCxP, s.aiAgentUsername(sess, agent), false)
	if err != nil {
		return string(mustJSON(aiToolError{"no se pudo recalcular las cuentas por pagar: " + err.Error()})), nil
	}

	limit := int(aiNumParam(args, "limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	ind := s.finSnapshotCxP(db)
	return string(mustJSON(map[string]interface{}{
		"status":      "ok",
		"scope":       scopeCxP,
		"indicadores": ind,
		"alertas":     s.finAlertas(db, scopeCxP, false, limit),
		"escaneo":     escaneo,
		"note": "Lo que se debe por dos vías: documentos de compra con saldo y servicios o gastos registrados. " +
			"El indicador `retenciones` son comprobantes de retención de IVA ya registrados que todavía NO se le han " +
			"enviado al proveedor: es dinero suyo en manos de la empresa. Use supplier.send_retention para enviárselos.",
	})), nil
}

// aiToolFinanceScan recalcula las alertas a pedido, sin esperar a que alguien
// abra la pantalla. Es lo que permite que una tarea programada deje el tablero
// listo antes de que empiece el día.
func (s *Server) aiToolFinanceScan(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	usuario := s.aiAgentUsername(sess, agent)
	scope := strings.ToLower(strings.TrimSpace(aiStrParam(args, "scope")))

	ambitos := []string{scopeCxC, scopeCxP}
	switch scope {
	case scopeCxC, scopeCxP:
		ambitos = []string{scope}
	case "", "ambos", "both", "todos":
	default:
		return string(mustJSON(aiToolError{
			"scope inválido: use 'cxc' (cuentas por cobrar), 'cxp' (cuentas por pagar) o déjelo vacío para los dos.",
		})), nil
	}

	resultado := []map[string]interface{}{}
	for _, sc := range ambitos {
		res, err := s.financeEscanear(db, sc, usuario, true)
		if err != nil {
			res = map[string]interface{}{"scope": sc, "error": err.Error()}
		}
		resultado = append(resultado, res)
	}

	return string(mustJSON(map[string]interface{}{
		"status":  "ok",
		"escaneo": resultado,
		"note": "Las alertas nuevas quedan abiertas en la pantalla del módulo y las que ya no se cumplen se cierran solas. " +
			"Volver a escanear no duplica nada: cada alerta tiene una huella única.",
	})), nil
}

// ---------- Salida: reclamar el pago ----------

func (s *Server) aiToolCustomerRequestPayment(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	r := s.requestForSession(sess)

	alertID := int64(aiNumParam(args, "alert_id"))
	customerID := int64(aiNumParam(args, "customer_id"))

	// La alerta es la vía natural, y la que se debe preferir: trae el motivo
	// redactado con los números del sistema. El customer_id existe para el caso
	// legítimo de reclamar una cuenta que todavía no genera alerta.
	var alerta map[string]interface{}
	switch {
	case alertID > 0:
		a, err := s.finAlertaPorID(db, alertID)
		if err != nil {
			return string(mustJSON(aiToolError{err.Error()})), nil
		}
		if fmt.Sprintf("%v", a["scope"]) != scopeCxC {
			return string(mustJSON(aiToolError{"esa alerta es de cuentas por pagar, no de cobranza: use supplier.send_retention para el proveedor."})), nil
		}
		if fmt.Sprintf("%v", a["status"]) != alertaAbierta {
			return string(mustJSON(map[string]interface{}{
				"status": "sin_accion", "alerta": alertID, "estado": a["status"],
				"note": "Esa alerta ya está " + fmt.Sprintf("%v", a["status"]) + ": no se le manda un reclamo a un asunto cerrado. Mírela en Cuentas por Cobrar si necesita el detalle.",
			})), nil
		}
		alerta = a
	case customerID > 0:
		a, err := s.finAlertaParaCliente(db, customerID)
		if err != nil {
			return string(mustJSON(aiToolError{err.Error()})), nil
		}
		alerta = a
	default:
		return string(mustJSON(aiToolError{
			"indique alert_id (lo que devuelve finance.receivables) o customer_id. Sin uno de los dos no se sabe de quién es la deuda.",
		})), nil
	}

	canal, canalErr := s.notifyCanalParaAgente(db, aiStrParam(args, "channel_kind"), aiStrParam(args, "channel"))
	if canalErr != nil {
		return string(mustJSON(aiToolError{canalErr.Error()})), nil
	}

	asunto := aiStrParam(args, "subject")
	cuerpo := aiStrParam(args, "message")
	if asunto == "" || cuerpo == "" {
		sa, sc := s.finPlantillaAviso(db, alerta, canal.Kind, "")
		if asunto == "" {
			asunto = sa
		}
		if cuerpo == "" {
			cuerpo = sc
		}
	}

	refID := strings.TrimSpace(aiStrParam(args, "external_ref"))
	switch {
	case refID != "":
		refID = "ref:" + refID
	case alertID > 0:
		refID = strconv.FormatInt(alertID, 10)
	default:
		refID = "customer:" + strconv.FormatInt(customerID, 10) + ":" + finFechaCorta(now())
	}

	res, err := s.financeEnviarAviso(r, db, canal, alerta, asunto, cuerpo, "aviso_cobro", refID, false)
	if err != nil {
		return string(mustJSON(aiToolError{"no se pudo registrar el aviso: " + err.Error()})), nil
	}

	if enviado, _ := res["enviado"].(bool); enviado && alertID > 0 {
		_, _ = db.Exec(`UPDATE finance_alert SET notified_at = ?, notify_count = notify_count + 1, updated_at = ? WHERE id = ?`,
			now(), now(), alertID)
	}
	res["status"] = "ok"
	res["cliente"] = alerta["party"]
	res["alerta"] = alertID
	s.aiAudit(sess, s.aiAgentUsername(sess, agent), "create", "ai_operator", "finance_message",
		fmt.Sprintf("%v", res["id"]), nil, map[string]interface{}{
			"canal": canal.Name, "destino": res["destino"], "cliente": alerta["party"],
			"enviado": res["enviado"], "alerta": alertID,
		})
	return string(mustJSON(res)), nil
}

// finAlertaParaCliente arma la alerta de un cliente concreto cuando no viene de
// la lista. Redacta el motivo con los mismos números que el escaneo, para que el
// correo del cliente no sea un texto distinto según quién lo pidió.
func (s *Server) finAlertaParaCliente(db *sql.DB, customerID int64) (map[string]interface{}, error) {
	nombre, email, movil := s.finContactoCliente(db, customerID)
	if strings.TrimSpace(nombre) == "" {
		return nil, fmt.Errorf("el cliente %d no existe", customerID)
	}

	vencido, total, dias, facturas, ref := 0.0, 0.0, -1, 0, ""
	masVieja := ""
	for _, f := range s.finFacturasAbiertas(db) {
		if f.CustomerID != customerID {
			continue
		}
		total += f.Due
		facturas++
		if f.Dias > dias {
			dias = f.Dias
			no := fmt.Sprintf("#%d", f.InvoiceNo)
			if f.InvoiceNo == 0 {
				no = fmt.Sprintf("interna %d", f.InvoiceID)
			}
			masVieja = fmt.Sprintf("factura %s del %s", no, f.Fecha)
		}
		if f.Dias >= finDiasAviso {
			vencido += f.Due
		}
	}
	if total <= 0.005 {
		return nil, fmt.Errorf("%s no tiene facturas con saldo: no hay nada que reclamar", nombre)
	}
	if dias < 0 {
		dias = 0
	}
	if masVieja != "" {
		ref = masVieja
	}
	motivo := fmt.Sprintf("%s tiene %s pendiente (%s). La factura más antigua es del documento más viejo de su cuenta: %s.",
		nombre, money(total), pluralFacturas(facturas), nombreODefecto(masVieja, "la más antigua de su cuenta"))
	if vencido > 0.005 {
		motivo = fmt.Sprintf("%s debe %s, con %d días desde su factura más antigua. Del total, %s ya pasó los %d días.",
			nombre, money(total), dias, money(vencido), finDiasAviso)
	}
	return map[string]interface{}{
		"id": int64(0), "scope": scopeCxC, "kind": "aviso_cobro", "kind_label": "Reclamo de pago",
		"severity": "aviso", "party_type": "customer", "party_id": customerID, "party": nombre,
		"ref_type": "cartera", "ref_id": ref, "amount": money(total), "amount_num": round2(total),
		"days": dias, "status": alertaAbierta, "motivo": motivo,
		"metricas": map[string]interface{}{
			"total": round2(total), "vencido": round2(vencido), "dias": dias, "facturas": facturas, "motivo": motivo,
		},
		"contact_email": email, "contact_mobile": movil,
	}, nil
}

func pluralFacturas(n int) string {
	if n == 1 {
		return "1 factura abierta"
	}
	return strconv.Itoa(n) + " facturas abiertas"
}

// ---------- Salida: el comprobante de retención ----------

func (s *Server) aiToolSupplierSendRetention(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	r := s.requestForSession(sess)

	supplierID := int64(aiNumParam(args, "supplier_id"))
	if purchaseID := int64(aiNumParam(args, "purchase_id")); purchaseID > 0 && supplierID <= 0 {
		_ = db.QueryRow(`SELECT supplier_id FROM product_purchase WHERE purchase_id = ?`, purchaseID).Scan(&supplierID)
	}
	if supplierID <= 0 {
		return string(mustJSON(aiToolError{
			"indique supplier_id, o purchase_id de una compra de ese proveedor. El comprobante de retención es del proveedor: sin contraparte no hay a quién mandárselo.",
		})), nil
	}

	periodo := strings.TrimSpace(aiStrParam(args, "period"))
	retenciones := s.finRetenciones(db)
	elegidas := []finRetencion{}
	for _, x := range retenciones {
		if x.SupplierID != supplierID {
			continue
		}
		if periodo != "" && x.Periodo != periodo {
			continue
		}
		elegidas = append(elegidas, x)
	}
	// Sin período indicado se toma el ÚLTIMO: es el que un proveedor reclama
	// primero, y mandar doce períodos de golpe en un correo no lo ayuda.
	if periodo == "" && len(elegidas) > 0 {
		ultimo := elegidas[0].Periodo
		for _, x := range elegidas {
			if x.Periodo > ultimo {
				ultimo = x.Periodo
			}
		}
		filtradas := []finRetencion{}
		for _, x := range elegidas {
			if x.Periodo == ultimo {
				filtradas = append(filtradas, x)
			}
		}
		elegidas = filtradas
	}
	if len(elegidas) == 0 {
		if periodo != "" {
			return string(mustJSON(aiToolError{fmt.Sprintf("el proveedor %d no tiene retenciones registradas en el período %s", supplierID, periodo)})), nil
		}
		return string(mustJSON(aiToolError{fmt.Sprintf("el proveedor %d no tiene retenciones de IVA registradas, así que no hay comprobante que enviar", supplierID)})), nil
	}

	e := elegidas[0]
	comprobantes := s.finDetalleRetencion(db, supplierID, e.Periodo)

	alerta := map[string]interface{}{
		"id": int64(0), "scope": scopeCxP, "kind": "pagar_retencion_pendiente",
		"kind_label": finEtiquetaKind("pagar_retencion_pendiente"), "severity": "aviso",
		"party_type": "supplier", "party_id": supplierID, "party": e.Supplier,
		"ref_type": "retention", "ref_id": e.Periodo,
		"amount": money(e.Retenido), "amount_num": round2(e.Retenido), "days": 0, "status": alertaAbierta,
		"motivo": fmt.Sprintf("Hay %d comprobante(s) de retención de IVA de %s (período %s) por %s.",
			e.N, e.Supplier, periodoLegible(e.Periodo), money(e.Retenido)),
		"metricas": map[string]interface{}{
			"periodo": e.Periodo, "comprobantes": e.N, "base": round2(e.Base),
			"iva": round2(e.IVA), "retenido": round2(e.Retenido),
		},
		"contact_email": e.Email, "contact_mobile": e.Movil,
	}

	canal, canalErr := s.notifyCanalParaAgente(db, aiStrParam(args, "channel_kind"), aiStrParam(args, "channel"))
	if canalErr != nil {
		return string(mustJSON(aiToolError{canalErr.Error()})), nil
	}

	asunto := aiStrParam(args, "subject")
	cuerpo := aiStrParam(args, "message")
	if asunto == "" || cuerpo == "" {
		sa, sc := s.finPlantillaAviso(db, alerta, canal.Kind, "")
		if asunto == "" {
			asunto = sa
		}
		if cuerpo == "" {
			cuerpo = sc + detalleComprobantes(comprobantes)
		}
	}

	// La referencia es LA MISMA que comprueba el escaneo
	// («supplier:<id>:<período>»): enviar el comprobante cierra la alerta de
	// retención pendiente sin que nadie tenga que marcarla a mano.
	refID := fmt.Sprintf("supplier:%d:%s", supplierID, e.Periodo)
	res, err := s.financeEnviarAviso(r, db, canal, alerta, asunto, cuerpo, "retencion_iva", refID, false)
	if err != nil {
		return string(mustJSON(aiToolError{"no se pudo registrar el envío: " + err.Error()})), nil
	}
	res["status"] = "ok"
	res["proveedor"] = e.Supplier
	res["periodo"] = e.Periodo
	res["retenido"] = money(e.Retenido)
	res["comprobantes"] = comprobantes
	s.aiAudit(sess, s.aiAgentUsername(sess, agent), "create", "ai_operator", "finance_message",
		fmt.Sprintf("%v", res["id"]), nil, map[string]interface{}{
			"canal": canal.Name, "destino": res["destino"], "proveedor": e.Supplier,
			"periodo": e.Periodo, "retenido": e.Retenido, "enviado": res["enviado"],
		})
	return string(mustJSON(res)), nil
}

// finDetalleRetencion lista los comprobantes que respaldan el envío. Va en el
// cuerpo del mensaje y en la respuesta: el proveedor tiene que poder cruzar
// número por número, y el agente tiene que poder decirlo en su informe.
func (s *Server) finDetalleRetencion(db *sql.DB, supplierID int64, periodo string) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT IFNULL(retention_no,''), IFNULL(CAST(date AS CHAR),''), IFNULL(invoice_no,''),
		IFNULL(control_no,''), base_amount, vat_amount, retention_amount
		FROM vat_retention WHERE supplier_id = ? AND IFNULL(period,'') = ?
		ORDER BY date ASC, id ASC LIMIT 200`, supplierID, periodo)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var comp, fecha, factura, control sql.NullString
		var base, iva, retenido float64
		if err := rows.Scan(&comp, &fecha, &factura, &control, &base, &iva, &retenido); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"retention_no": comp.String, "date": finFechaCorta(fecha.String),
			"invoice_no": factura.String, "control_no": control.String,
			"base": money(base), "iva": money(iva), "retenido": money(retenido),
		})
	}
	return out
}

func detalleComprobantes(comprobantes []map[string]interface{}) string {
	if len(comprobantes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nComprobantes incluidos:\n")
	for _, c := range comprobantes {
		b.WriteString("- N° " + fmt.Sprintf("%v", c["retention_no"]) +
			" · " + fmt.Sprintf("%v", c["date"]) +
			" · factura " + fmt.Sprintf("%v", c["invoice_no"]) +
			" · control " + fmt.Sprintf("%v", c["control_no"]) +
			" · base " + fmt.Sprintf("%v", c["base"]) +
			" · IVA " + fmt.Sprintf("%v", c["iva"]) +
			" · retenido " + fmt.Sprintf("%v", c["retenido"]) + "\n")
	}
	return b.String()
}

// ---------- Salida: eventos para herramientas externas ----------

// aiToolNotifyEvent manda un evento a un webhook configurado.
//
// POR QUÉ EXISTE, SI YA HAY `finance.receivables`. Porque el uso real no es que
// el ERP lo muestre, sino que otra herramienta haga algo con eso: crear una tarea
// de cobranza, avisar por otro canal, dejar el registro en un tablero. El evento
// lleva un nombre propio (`cobranza.mora_anormal`, `pagar.retencion_pendiente`)
// para que quien lo reciba decida sin parsear texto. Y como todo lo que sale,
// necesita un canal autorizado por el administrador y queda escrito.
func (s *Server) aiToolNotifyEvent(sess *SessionData, agent aiAgent, args map[string]interface{}) (string, error) {
	db := s.dbForSession(sess)
	if db == nil {
		return string(mustJSON(aiToolError{"sin base de datos para esta empresa"})), nil
	}
	r := s.requestForSession(sess)

	evento := strings.TrimSpace(aiStrParam(args, "event"))
	if evento == "" {
		return string(mustJSON(aiToolError{
			"indique `event`: el nombre del evento que verá la herramienta que lo reciba (por ejemplo 'cobranza.mora_anormal').",
		})), nil
	}
	crudo := aiStrParam(args, "data")
	datos := interface{}(map[string]interface{}{})
	if crudo != "" {
		if !json.Valid([]byte(crudo)) {
			return string(mustJSON(aiToolError{"`data` tiene que ser JSON válido (un objeto). Lo que no es JSON no se le manda a nadie."})), nil
		}
		_ = json.Unmarshal([]byte(crudo), &datos)
	}

	canal, canalErr := s.notifyCanalParaAgente(db, "webhook", aiStrParam(args, "channel"))
	if canalErr != nil {
		return string(mustJSON(aiToolError{canalErr.Error()})), nil
	}

	// Idempotencia por evento + referencia + día: reintentar la entrega del mismo
	// evento reenvía el MISMO evento, no crea uno nuevo del otro lado.
	refID := strings.TrimSpace(aiStrParam(args, "external_ref"))
	if refID == "" {
		refID = finFechaCorta(now())
	}
	clave := aiHashKey("evento|" + evento + "|" + refID + "|" + strconv.FormatInt(canal.ID, 10))

	res, err := db.Exec(`INSERT INTO finance_message (channel_id, channel, channel_kind, recipient, subject, body,
		ref_type, ref_id, status, idem_key, created_by, created_at)
		VALUES (?,?,?,?,?,?, 'evento', ?, 'enviando', ?, ?, ?)`,
		canal.ID, canal.Name, canal.Kind, "webhook://"+canal.Name, evento, crudo, refID, clave,
		s.aiAgentUsername(sess, agent), now())
	if err != nil {
		if aiEsClaveDuplicada(err) {
			return string(mustJSON(map[string]interface{}{
				"status": "duplicado", "event": evento, "reference": refID,
				"note": "Ese mismo evento con esa misma referencia ya se entregó. No se repite: está en Mensajes enviados con su resultado.",
			})), nil
		}
		return string(mustJSON(aiToolError{"no se pudo registrar el evento: " + err.Error()})), nil
	}
	msgID, _ := res.LastInsertId()

	respuesta, err := s.envioHTTP(canal, evento, map[string]interface{}{
		"event": evento, "data": datos, "sent_at": now(),
		"origin": func() string {
			if empresa := s.finEmpresa(db); empresa != "" {
				return empresa
			}
			return "ERPGo"
		}(),
	})
	if err != nil {
		_, _ = db.Exec(`UPDATE finance_message SET status = 'error', error = ? WHERE id = ?`, err.Error(), msgID)
		return string(mustJSON(map[string]interface{}{
			"status": "error", "event": evento, "channel": canal.Name, "msg": err.Error(),
			"note": "El evento NO se entregó. El registro quedó en Mensajes enviados con el motivo. No lo dé por entregado en su informe.",
		})), nil
	}
	_, _ = db.Exec(`UPDATE finance_message SET status = 'enviado', sent_at = ?, provider_ref = ? WHERE id = ?`,
		now(), respuesta, msgID)

	s.aiAudit(sess, s.aiAgentUsername(sess, agent), "create", "ai_operator", "finance_message",
		strconv.FormatInt(msgID, 10), nil, map[string]interface{}{
			"event": evento, "canal": canal.Name, "referencia": refID,
		})
	_ = r
	return string(mustJSON(map[string]interface{}{
		"status": "ok", "event": evento, "channel": canal.Name, "reference": refID,
		"response": respuesta, "message_id": msgID,
		"note": "El evento queda registrado en Mensajes enviados. Si la herramienta externa responde con error, el sistema lo deja escrito y no reintenta solo.",
	})), nil
}
