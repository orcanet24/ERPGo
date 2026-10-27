package web

// ai_tools_compras.go — Las herramientas con las que un agente carga compras,
// proveedores y productos.
//
// LA IDEA QUE ORDENA ESTE ARCHIVO.
//
// El agente NO escribe en las tablas: usa los mismos handlers que el operador
// (purchaseSave, supplierSave, productSave) a través de peticionInterna. Lo que
// se gana con eso es que no existan dos verdades. Si mañana la compra aprende a
// retener IVA, el agente retiene igual; si el asiento cambia de cuenta, cambia
// para los dos. Una herramienta que reimplementara el INSERT tendría que
// acordarse de cada regla que el handler ya cumple, y esa copia se queda vieja
// sin avisar: es el mismo motivo por el que el semillero de empresas de prueba
// llama a los handlers de verdad en vez de sembrar con SQL.
//
// LOS DOS CONTROLES.
//
//  1. El interruptor de operador (OperatorOnly): sin él la herramienta no se
//     anuncia al modelo ni se ejecuta.
//  2. Los permisos del ROL (campo Perm de aiToolDef): el agente tiene el rol que
//     el administrador le asignó, y ese rol tiene que tener el permiso en la
//     matriz de Roles y Permisos (role_permission). Sin permiso la acción se
//     bloquea con el motivo escrito, no con un error genérico.
//
// El segundo control es el que hace literal la frase «un usuario más, con las
// limitaciones que se le impongan»: la misma tabla que limita a un empleado en
// la interfaz limita al agente aquí.

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ---------- Permisos del rol (Nivel 2) ----------

// aiPermisoDenegado devuelve el motivo del bloqueo, o "" si el rol del agente
// tiene el permiso que declara la herramienta.
func (s *Server) aiPermisoDenegado(sess *SessionData, agent aiAgent, tool string) string {
	def, ok := aiToolByName[tool]
	if !ok || def.Perm == "" {
		return ""
	}
	if s.aiPuedePermiso(sess, def.Perm) {
		return ""
	}
	rol := strings.TrimSpace(agent.RoleName)
	if rol == "" {
		rol = "sin rol asignado"
	}
	return fmt.Sprintf("su rol «%s» no tiene el permiso «%s» (crear) en Roles y Permisos, "+
		"así que el sistema no acepta esta operación del agente. "+
		"Pida al administrador que lo conceda a ese rol, o use un agente cuyo rol sí lo tenga.", rol, def.Perm)
}

// aiPuedePermiso comprueba un permiso de la matriz para la identidad del agente.
func (s *Server) aiPuedePermiso(sess *SessionData, perm string) bool {
	if perm == "" {
		return true
	}
	return s.hasPermission(s.requestForSession(sess), perm, "create")
}

// ---------- supplier.upsert ----------

// aiToolSupplierUpsert da de alta un proveedor, o devuelve el que ya existe.
//
// POR QUÉ DEVUELVE EL EXISTENTE EN VEZ DE FALLAR.
//
// La IA no ve la pantalla de proveedores: cuando recibe un listado o una factura
// con «Ferretería El Tornillo, J-30123456-7», no puede saber si ya está cargado.
// Fallar obligaría al humano a resolver una duda que el sistema puede resolver
// solo, y crear a ciegas poblaría el libro de compras con contrapartes gemelas
// —que es un problema fiscal, no de orden—. Se busca por RIF (la identidad
// fiscal) y por nombre exacto, y se informa cuál fue el caso.
func (s *Server) aiToolSupplierUpsert(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
	r := s.requestForSession(sess)
	sid := int64(aiNumParam(args, "supplier_id"))
	name := strings.TrimSpace(aiStrParam(args, "name"))
	rif := NormalizeRIF(aiStrParam(args, "rif"))
	if name == "" && sid <= 0 {
		return string(mustJSON(aiToolError{"indique name para crear el proveedor, o supplier_id para actualizarlo."})), nil
	}

	if sid <= 0 {
		if existente := s.proveedorDuplicado(r, rif, name); existente > 0 {
			return string(mustJSON(map[string]interface{}{
				"status": "ok", "created": false, "supplier_id": existente,
				"note": "el proveedor ya estaba registrado; use este id en la compra",
			})), nil
		}
	}

	campos := url.Values{}
	if sid > 0 {
		campos.Set("supplier_id", strconv.FormatInt(sid, 10))
	}
	campos.Set("supplier_name", name)
	campos.Set("rif", rif)
	for _, k := range []string{"mobile", "email_address", "address", "address2", "city", "state", "zip", "country", "details"} {
		if v := strings.TrimSpace(aiStrParam(args, k)); v != "" {
			campos.Set(k, v)
		}
	}
	// El correo puede llegar como «email», que es como lo escribe un humano.
	if v := strings.TrimSpace(aiStrParam(args, "email")); v != "" && campos.Get("email_address") == "" {
		campos.Set("email_address", v)
	}

	codigo, datos, cuerpo := s.respuestaDeHandler(sess, campos, s.supplierSave)
	if codigo >= 400 {
		return string(mustJSON(aiToolError{mensajeDeError(datos, cuerpo)})), nil
	}
	nuevo := int64(0)
	if v, ok := datos["supplier_id"].(float64); ok {
		nuevo = int64(v)
	}
	if nuevo <= 0 {
		return string(mustJSON(aiToolError{"el proveedor se guardó pero no se pudo recuperar su id para la compra"})), nil
	}
	journal.add("supplier", map[string]interface{}{"supplier_id": nuevo, "name": name})
	s.aiAudit(sess, aiAgentUsername(sess, agent), "create", "ai_operator", "supplier_information",
		strconv.FormatInt(nuevo, 10), nil, map[string]interface{}{"name": name, "rif": rif})
	return string(mustJSON(map[string]interface{}{
		"status": "ok", "created": true, "supplier_id": nuevo, "name": name, "rif": rif,
	})), nil
}

// ---------- product.upsert ----------

// aiToolProductUpsert da de alta un producto, o devuelve el que ya existe.
func (s *Server) aiToolProductUpsert(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
	r := s.requestForSession(sess)
	pid := strings.TrimSpace(aiStrParam(args, "product_id"))
	name := strings.TrimSpace(aiStrParam(args, "name"))
	model := strings.TrimSpace(aiStrParam(args, "product_model"))
	barcode := strings.TrimSpace(aiStrParam(args, "barcode"))
	if name == "" && pid == "" {
		return string(mustJSON(aiToolError{"indique name para crear el producto, o product_id para actualizarlo."})), nil
	}

	if pid == "" {
		if existente := s.productoDuplicado(r, barcode, name, model); existente != "" {
			return string(mustJSON(map[string]interface{}{
				"status": "ok", "created": false, "product_id": existente,
				"note": "el producto ya estaba en el catálogo; use este id en la compra",
			})), nil
		}
	}

	campos := url.Values{}
	if pid != "" {
		campos.Set("product_id", pid)
	} else {
		// Id numérico a propósito: el movimiento de inventario resuelve el
		// producto con un entero, y un id alfanumérico dejaría la entrada de
		// mercancía apuntando a un producto 0.
		campos.Set("product_id", s.proximoProductoID(r))
	}
	campos.Set("product_name", name)
	campos.Set("product_model", model)
	campos.Set("barcode", barcode)
	for _, k := range []string{"price", "price_usd", "unit", "category_id", "product_vat", "tax", "serial_no", "product_details"} {
		if v := strings.TrimSpace(aiStrParam(args, k)); v != "" {
			campos.Set(k, v)
		}
	}

	codigo, datos, cuerpo := s.respuestaDeHandler(sess, campos, s.productSave)
	if codigo >= 400 {
		return string(mustJSON(aiToolError{mensajeDeError(datos, cuerpo)})), nil
	}
	nuevo := campos.Get("product_id")
	if v, ok := datos["id"].(string); ok && v != "" {
		nuevo = v
	}
	journal.add("product", map[string]interface{}{"product_id": nuevo, "name": name})
	s.aiAudit(sess, aiAgentUsername(sess, agent), "create", "ai_operator", "product_information",
		nuevo, nil, map[string]interface{}{"name": name, "barcode": barcode})
	return string(mustJSON(map[string]interface{}{
		"status": "ok", "created": pid == "", "product_id": nuevo, "name": name,
	})), nil
}

// ---------- purchase.create ----------

// aiLineaCompra es un renglón ya validado y resuelto a un producto del sistema.
type aiLineaCompra struct {
	pid     string
	nombre  string
	qty     float64
	rate    float64
	batch   string
	expiry  string
	discP   float64
	vatP    float64
	ultimo  float64 // precio de la última compra ANTES de esta
	descVal float64
	vatVal  float64
	total   float64
	nuevo   bool // el artículo se dio de alta en esta misma llamada
}

// aiToolPurchaseCreate registra una compra completa.
//
// SE CARGAN LOS RENGLONES Y SE RESPONDE CON LAS ALERTAS, no solo con un ok. El
// valor de esta herramienta para un humano que carga compras no es que guarde
// sola, es que avise de lo que él tendría que recordar: qué artículo no estaba
// en el catálogo, cuál subió de precio respecto a la compra anterior y cuánto.
// Por eso la variación se calcula ANTES de escribir y viaja en la respuesta: el
// archivo del reporte del agente es el sitio donde ese aviso se lee.
func (s *Server) aiToolPurchaseCreate(sess *SessionData, agent aiAgent, args map[string]interface{}, journal *aiRunJournal) (string, error) {
	r := s.requestForSession(sess)
	db := s.dbForSession(sess)

	rawItems, _ := args["items"].([]interface{})
	if len(rawItems) == 0 {
		return string(mustJSON(aiToolError{"items vacío: una compra necesita al menos un renglón con quantity y rate."})), nil
	}

	// ---- Proveedor: por id, o por nombre (se crea si no existe) ----
	supplierID := int64(aiNumParam(args, "supplier_id"))
	nombreProv := strings.TrimSpace(aiStrParam(args, "supplier_name"))
	rifProv := NormalizeRIF(aiStrParam(args, "supplier_rif"))
	proveedorCreado := false
	switch {
	case supplierID > 0:
		var existe int
		_ = db.QueryRow(`SELECT COUNT(*) FROM supplier_information WHERE supplier_id = ?`, supplierID).Scan(&existe)
		if existe == 0 {
			return string(mustJSON(aiToolError{fmt.Sprintf("el proveedor %d no existe en el sistema", supplierID)})), nil
		}
	case nombreProv != "":
		supplierID = s.proveedorDuplicado(r, rifProv, nombreProv)
		if supplierID <= 0 {
			// RIF obligatorio al crear: el libro de compras lo exige, y una
			// compra sin RIF del proveedor es un documento que el SENIAT no
			// reconoce. Es preferible detener aquí y decirlo.
			if rifProv == "" {
				return string(mustJSON(aiToolError{
					"el proveedor «" + nombreProv + "» no está registrado y no se indicó su RIF; " +
						"el libro de compras exige el RIF para dar de alta la contraparte.",
				})), nil
			}
			if !s.aiPuedePermiso(sess, "add_supplier") {
				return string(mustJSON(aiToolError{
					"el proveedor «" + nombreProv + "» no está registrado y su rol no tiene el permiso " +
						"«add_supplier» para darlo de alta. Cárguelo antes desde Proveedores.",
				})), nil
			}
			camposProv := url.Values{}
			camposProv.Set("supplier_name", nombreProv)
			camposProv.Set("rif", rifProv)
			if v := strings.TrimSpace(aiStrParam(args, "supplier_mobile")); v != "" {
				camposProv.Set("mobile", v)
			}
			if v := strings.TrimSpace(aiStrParam(args, "supplier_address")); v != "" {
				camposProv.Set("address", v)
			}
			codigo, datos, cuerpo := s.respuestaDeHandler(sess, camposProv, s.supplierSave)
			if codigo >= 400 {
				return string(mustJSON(aiToolError{"no se pudo dar de alta el proveedor: " + mensajeDeError(datos, cuerpo)})), nil
			}
			if v, ok := datos["supplier_id"].(float64); ok {
				supplierID = int64(v)
			}
			if supplierID <= 0 {
				return string(mustJSON(aiToolError{"el proveedor se creó pero no se pudo recuperar su id para la compra"})), nil
			}
			proveedorCreado = true
			journal.add("supplier", map[string]interface{}{"supplier_id": supplierID, "name": nombreProv})
		}
	default:
		return string(mustJSON(aiToolError{"indique supplier_id o supplier_name para saber a quién se le compra."})), nil
	}

	// ---- Renglones ----
	lineas := []aiLineaCompra{}
	creados := []map[string]interface{}{}
	for i, raw := range rawItems {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			return string(mustJSON(aiToolError{fmt.Sprintf("el renglón %d no es un objeto válido", i+1)})), nil
		}
		qty := aiNumParam(m, "quantity")
		if qty <= 0 {
			qty = aiNumParam(m, "qty")
		}
		if qty <= 0 {
			return string(mustJSON(aiToolError{
				fmt.Sprintf("el renglón %d no trae cantidad mayor que cero (quantity)", i+1),
			})), nil
		}
		rate := aiNumParam(m, "rate")
		if rate < 0 {
			rate = 0
		}

		pid := strings.TrimSpace(aiStrParam(m, "product_id"))
		barcode := strings.TrimSpace(aiStrParam(m, "barcode"))
		nombre := strings.TrimSpace(aiStrParam(m, "name"))
		modelo := strings.TrimSpace(aiStrParam(m, "product_model"))
		nuevo := false

		if pid != "" {
			var existe int
			_ = db.QueryRow(`SELECT COUNT(*) FROM product_information WHERE product_id = ?`, pid).Scan(&existe)
			if existe == 0 {
				// Un id inventado no puede crear un artículo sin nombre: se
				// trata como si no se hubiera identificado el producto.
				pid = ""
			}
		}
		if pid == "" {
			pid = s.productoDuplicado(r, barcode, nombre, modelo)
		}
		if pid == "" {
			if nombre == "" {
				return string(mustJSON(aiToolError{
					fmt.Sprintf("el renglón %d no identifica el producto: indique product_id, barcode o name", i+1),
				})), nil
			}
			if !s.aiPuedePermiso(sess, "create_product") {
				return string(mustJSON(aiToolError{
					fmt.Sprintf("el renglón %d trae «%s», que no está en el catálogo, y su rol no tiene el permiso "+
						"«create_product» para darlo de alta. Cárguelo antes desde Productos.", i+1, nombre),
				})), nil
			}
			camposProd := url.Values{}
			camposProd.Set("product_id", s.proximoProductoID(r))
			camposProd.Set("product_name", nombre)
			camposProd.Set("product_model", modelo)
			camposProd.Set("barcode", barcode)
			if v := strings.TrimSpace(aiStrParam(m, "unit")); v != "" {
				camposProd.Set("unit", v)
			}
			if v := aiNumParam(m, "product_vat"); v > 0 {
				camposProd.Set("product_vat", strconv.FormatFloat(v, 'f', 2, 64))
			}
			// El costo entra como precio de referencia del catálogo solo si el
			// producto no trae ninguno: el precio de venta no se inventa desde
			// una compra.
			codigo, datos, cuerpo := s.respuestaDeHandler(sess, camposProd, s.productSave)
			if codigo >= 400 {
				return string(mustJSON(aiToolError{
					fmt.Sprintf("el renglón %d trae «%s» y no se pudo dar de alta: %s", i+1, nombre, mensajeDeError(datos, cuerpo)),
				})), nil
			}
			pid = camposProd.Get("product_id")
			if v, ok := datos["id"].(string); ok && v != "" {
				pid = v
			}
			nuevo = true
			creados = append(creados, map[string]interface{}{"product_id": pid, "name": nombre, "barcode": barcode})
			journal.add("product", map[string]interface{}{"product_id": pid, "name": nombre})
		}

		// Último costo ANTES de guardar: es el número con el que se compara.
		var ultimo float64
		_ = db.QueryRow(`SELECT IFNULL((SELECT d.rate FROM product_purchase_details d
			JOIN product_purchase p ON p.purchase_id = d.purchase_id
			WHERE d.product_id = ? ORDER BY p.id DESC LIMIT 1), 0)`, pid).Scan(&ultimo)

		lineas = append(lineas, aiLineaCompra{
			pid: pid, nombre: nombre, qty: qty, rate: rate,
			batch:  strings.TrimSpace(aiStrParam(m, "batch_no")),
			expiry: strings.TrimSpace(aiStrParam(m, "expiry_date")),
			discP:  aiNumParam(m, "discount_per"),
			vatP:   aiNumParam(m, "vat_per"),
			ultimo: ultimo,
			nuevo:  nuevo,
		})
	}

	// ---- Importes, con la MISMA aritmética que el formulario ----
	// (bruto → descuento por línea → IVA por línea; el descuento global se resta
	// al final y el IVA se suma al final, igual que en el JS de purchase_form).
	sub, descTotal, vatTotal := 0.0, 0.0, 0.0
	campos := url.Values{}
	for i := range lineas {
		l := &lineas[i]
		bruto := l.qty * l.rate
		l.descVal = round2(bruto * l.discP / 100)
		base := bruto - l.descVal
		l.vatVal = round2(base * l.vatP / 100)
		l.total = round2(base)
		sub += l.total
		descTotal += l.descVal
		vatTotal += l.vatVal

		campos.Add("product_id", l.pid)
		campos.Add("product_quantity", strconv.FormatFloat(l.qty, 'f', 2, 64))
		campos.Add("product_rate", strconv.FormatFloat(l.rate, 'f', 2, 64))
		campos.Add("total_price", strconv.FormatFloat(l.total, 'f', 2, 64))
		campos.Add("discount_per", strconv.FormatFloat(l.discP, 'f', 2, 64))
		campos.Add("discountvalue", strconv.FormatFloat(l.descVal, 'f', 2, 64))
		campos.Add("vatpercent", strconv.FormatFloat(l.vatP, 'f', 2, 64))
		campos.Add("vatvalue", strconv.FormatFloat(l.vatVal, 'f', 2, 64))
		if l.batch != "" {
			campos.Add("batch_no", l.batch)
		}
		if l.expiry != "" {
			campos.Add("expiry_date", l.expiry)
		}
	}
	sub = round2(sub)
	descFactura := round2(aiNumParam(args, "discount"))
	grand := round2(sub - descFactura + vatTotal)

	// ---- Pagos (opcional: sin pagos la compra queda a crédito) ----
	paid := 0.0
	if rawPagos, ok := args["payments"].([]interface{}); ok {
		for _, rp := range rawPagos {
			m, _ := rp.(map[string]interface{})
			if m == nil {
				continue
			}
			monto := round2(aiNumParam(m, "amount"))
			if monto <= 0 {
				continue
			}
			metodo := int64(aiNumParam(m, "method_id"))
			if metodo <= 0 {
				return string(mustJSON(aiToolError{
					"cada pago necesita method_id (el id del método de pago del sistema); sin él no se sabe de qué cuenta sale el dinero.",
				})), nil
			}
			campos.Add("multipaytype", strconv.FormatInt(metodo, 10))
			campos.Add("pamount_by_method", strconv.FormatFloat(monto, 'f', 2, 64))
			paid += monto
		}
	}
	paid = round2(paid)
	if paid > grand {
		return string(mustJSON(aiToolError{
			fmt.Sprintf("los pagos (%.2f) superan el total de la compra (%.2f)", paid, grand),
		})), nil
	}
	due := round2(grand - paid)
	if len(campos["multipaytype"]) == 0 {
		// Compra a crédito: es la forma en que el formulario lo declara.
		campos.Set("multipaytype", "0")
	}

	fecha := strings.TrimSpace(aiStrParam(args, "date"))
	if !aiValidDate(fecha) {
		fecha = today()
	}
	campos.Set("supplier_id", strconv.FormatInt(supplierID, 10))
	campos.Set("supplier_rif", rifProv)
	campos.Set("purchase_date", fecha)
	campos.Set("chalan_no", strings.TrimSpace(aiStrParam(args, "chalan_no")))
	campos.Set("control_no", strings.TrimSpace(aiStrParam(args, "control_no")))
	campos.Set("purchase_details", strings.TrimSpace(aiStrParam(args, "details")))
	campos.Set("discount", strconv.FormatFloat(descFactura, 'f', 2, 64))
	campos.Set("total_discount", strconv.FormatFloat(round2(descTotal+descFactura), 'f', 2, 64))
	campos.Set("total_vat_amnt", strconv.FormatFloat(vatTotal, 'f', 2, 64))
	campos.Set("grand_total_price", strconv.FormatFloat(grand, 'f', 2, 64))
	campos.Set("paid_amount", strconv.FormatFloat(paid, 'f', 2, 64))
	campos.Set("due_amount", strconv.FormatFloat(due, 'f', 2, 64))

	// ---- La escritura: el handler de la compra, el mismo del formulario ----
	codigo, datos, cuerpo := s.respuestaDeHandler(sess, campos, s.purchaseSave)
	if codigo >= 400 {
		return string(mustJSON(aiToolError{mensajeDeError(datos, cuerpo)})), nil
	}
	purchaseID := int64(0)
	if v, ok := datos["purchase_id"].(float64); ok {
		purchaseID = int64(v)
	}
	if purchaseID <= 0 {
		return string(mustJSON(aiToolError{
			"la compra se guardó pero no se pudo recuperar su número; verifíquela en Gestionar Compras antes de repetir la carga",
		})), nil
	}

	// ---- Alertas de variación de precio ----
	alertas := []map[string]interface{}{}
	for _, l := range lineas {
		if l.ultimo <= 0 || l.rate <= 0 {
			continue
		}
		if round2(l.rate) == round2(l.ultimo) {
			continue
		}
		pct := round2((l.rate - l.ultimo) / l.ultimo * 100)
		direccion := "bajó"
		if pct > 0 {
			direccion = "subió"
		}
		alertas = append(alertas, map[string]interface{}{
			"product_id": l.pid, "name": l.nombre,
			"costo_anterior": round2(l.ultimo), "costo_actual": round2(l.rate),
			"variacion_pct": pct, "direccion": direccion,
		})
	}

	journal.add("purchase", map[string]interface{}{
		"purchase_id": purchaseID, "supplier_id": supplierID, "total": grand, "items": len(lineas),
	})
	s.aiAudit(sess, aiAgentUsername(sess, agent), "create", "ai_operator", "product_purchase",
		strconv.FormatInt(purchaseID, 10), nil, map[string]interface{}{
			"supplier_id": supplierID, "total": grand, "items": len(lineas), "fecha": fecha,
		})

	resp := map[string]interface{}{
		"status": "ok", "purchase_id": purchaseID, "supplier_id": supplierID,
		"date": fecha, "subtotal": sub, "discount": descFactura, "vat": round2(vatTotal),
		"total": grand, "paid": paid, "due": due, "items": len(lineas),
		"note": "Compra registrada por el mismo camino que el formulario: stock, lote y asiento contable incluidos.",
	}
	if proveedorCreado {
		resp["supplier_created"] = true
	}
	if len(creados) > 0 {
		resp["productos_creados"] = creados
	}
	if len(alertas) > 0 {
		resp["alerta_variacion_precio"] = alertas
	}
	return string(mustJSON(resp)), nil
}
