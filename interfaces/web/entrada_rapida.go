package web

// entrada_rapida.go — Altas rápidas desde donde se está trabajando.
//
// QUÉ RESUELVE.
//
// Al cargar una compra aparece siempre el mismo par de fricciones: el proveedor
// no está dado de alta y el producto tampoco. Obligar a salir del formulario,
// abrir Proveedores, guardar, volver, abrir Productos, guardar y regresar es
// como se pierde una compra a medio cargar en el mostrador.
//
// Estas rutas dan de alta la contraparte y el artículo SIN escribir en las
// tablas por su cuenta: llaman a `supplierSave` y `productSave`, los mismos
// handlers que usa el formulario completo. Es la única forma de que el alta
// rápida no cree datos que el alta normal no habría creado: la cuenta contable
// del proveedor, su subcódigo, la validación del RIF y las reglas de duplicado
// salen del mismo sitio.
//
// La IA usa EXACTAMENTE estos mismos caminos (ver ai_tools_compras.go): el
// agente no tiene una puerta trasera a la base, tiene la misma puerta que el
// operador, con su rol como límite.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// registerEntradaRapidaRoutes registra las altas rápidas y el catálogo visual.
func (s *Server) registerEntradaRapidaRoutes() {
	s.mux.HandleFunc("POST /product/quick", s.productQuickCreate)
	s.mux.HandleFunc("GET /product/picker", s.productPicker)
	// POST /supplier/quick se registra en registerSupplierRoutes, junto a
	// /supplier/save: dos registros del mismo patrón hacen que http.ServeMux
	// entre en pánico al arrancar, y el servidor no levantaría.
}

// peticionInterna arma la petición que necesita un handler de escritura para
// correr fuera del ciclo HTTP: cuerpo de formulario, identidad y empresa.
//
// Las tres cosas son necesarias. Sin el ámbito, `dbFor` caería a la empresa
// activa del proceso y en multi-empresa el alta acabaría en la base equivocada;
// sin la sesión, la auditoría y los permisos del rol no tendrían a quién
// referirse.
func (s *Server) peticionInterna(sess *SessionData, campos url.Values) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(campos.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tenant := s.tenantOfSession(sess)
	return withScope(r, &scope{
		tenant:   tenant,
		db:       s.dbForTenant(tenant),
		settings: s.settingsOfTenant(tenant),
		sess:     sess,
	})
}

// respuestaDeHandler ejecuta un handler de escritura internamente y devuelve
// (código, JSON, cuerpo crudo).
//
// Se lee el CUERPO y no solo el código: cuando el handler devuelve 400 por un
// campo faltante, el motivo está en el cuerpo, y sin él la IA (o el usuario)
// recibe un \"no se pudo\" que no dice qué corregir.
func (s *Server) respuestaDeHandler(sess *SessionData, campos url.Values, h http.HandlerFunc) (int, map[string]interface{}, string) {
	w := &grabaWriter{}
	h(w, s.peticionInterna(sess, campos))
	codigo := w.codigo
	if codigo == 0 {
		codigo = 200
	}
	cuerpo := strings.TrimSpace(cuerpoDe(w))
	var datos map[string]interface{}
	_ = json.Unmarshal([]byte(cuerpo), &datos)
	return codigo, datos, cuerpo
}

// mensajeDeError saca el motivo legible de una respuesta de handler.
func mensajeDeError(datos map[string]interface{}, cuerpo string) string {
	if datos != nil {
		if e, ok := datos["error"].(string); ok && strings.TrimSpace(e) != "" {
			return e
		}
		if m, ok := datos["msg"].(string); ok && strings.TrimSpace(m) != "" {
			return m
		}
	}
	if cuerpo == "" {
		return "la operación no se pudo completar"
	}
	return cuerpo
}

// productoDuplicado busca el artículo que corresponde a unos datos de alta.
//
// POR QUÉ POR CÓDIGO DE BARRAS Y NO SIEMPRE POR NOMBRE.
//
// El código de barras es la identidad del artículo en el pasillo: si ya existe
// uno con ese código, es ese. El nombre no lo es —\"Harina P.A.N. 1kg\" y
// \"harina pan 1 kg\" son el mismo saco, pero \"Coca-Cola 350ml\" y
// \"Coca-Cola 2L\" comparten nombre comercial con distinto modelo—, así que el
// nombre solo decide cuando además coincide el modelo, o cuando no hay modelo
// en ninguno de los dos.
func (s *Server) productoDuplicado(r *http.Request, barcode, name, model string) string {
	db := s.dbFor(r)
	if db == nil {
		return ""
	}
	if b := strings.TrimSpace(barcode); b != "" {
		var id string
		if err := db.QueryRow(`SELECT product_id FROM product_information WHERE TRIM(barcode) = ? LIMIT 1`, b).Scan(&id); err == nil && id != "" {
			return id
		}
	}
	name = strings.TrimSpace(name)
	model = strings.TrimSpace(model)
	if name == "" {
		return ""
	}
	var id string
	err := db.QueryRow(`SELECT product_id FROM product_information
		WHERE LOWER(TRIM(product_name)) = LOWER(?) AND LOWER(TRIM(IFNULL(product_model,''))) = LOWER(?)
		LIMIT 1`, name, model).Scan(&id)
	if err == nil && id != "" {
		return id
	}
	return ""
}

// proximoProductoID devuelve el siguiente id NUMÉRICO de producto.
//
// El formulario de productos genera ids alfanuméricos (generator(8)) cuando no
// se escribe ninguno, y el movimiento de inventario que registra una compra
// resuelve el producto con strconv.Atoi: un id \"A3F9KQ1B\" se convierte en 0 y
// el kardex queda apuntando a un producto que no existe. Un artículo creado
// desde una compra nace con id numérico para que su mercancía y su kardex
// hablen del mismo producto.
func (s *Server) proximoProductoID(r *http.Request) string {
	var n int64
	if err := s.dbFor(r).QueryRow(`SELECT IFNULL(MAX(CAST(product_id AS UNSIGNED)),0)+1 FROM product_information`).Scan(&n); err != nil || n <= 0 {
		return "1"
	}
	return strconv.FormatInt(n, 10)
}

// productQuickCreate da de alta un producto desde el formulario de compra.
//
// Antes de crear pregunta si el artículo ya está cargado: un alta rápida que
// duplica el catálogo es peor que no tener alta rápida, porque las compras de
// los dos gemelos se reparten y el stock de ninguno cuadra.
func (s *Server) productQuickCreate(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	name := strings.TrimSpace(r.FormValue("product_name"))
	if name == "" {
		jsonError(w, "El nombre del producto es obligatorio", 400)
		return
	}
	barcode := strings.TrimSpace(r.FormValue("barcode"))
	model := strings.TrimSpace(r.FormValue("product_model"))

	if id := s.productoDuplicado(r, barcode, name, model); id != "" {
		// Se devuelve el producto que ya estaba, con la forma que la fila
		// espera. Si la relectura falla se contesta el mínimo necesario: un
		// handler que revienta con un mapa nulo deja al usuario sin saber si el
		// producto se creó o no, y eso es peor que un dato menos.
		if p, err := s.datosProducto(r, id); err == nil {
			p["existente"] = true
			p["msg"] = "El producto ya existía y se cargó en la fila"
			writeJSON(w, p)
			return
		}
		writeJSON(w, map[string]interface{}{
			"status": 1, "product_id": id, "id": id, "name": name, "model": model,
			"barcode": barcode, "existente": true,
			"msg": "El producto ya existía y se cargó en la fila",
		})
		return
	}

	campos := url.Values{}
	campos.Set("product_id", s.proximoProductoID(r))
	campos.Set("product_name", name)
	campos.Set("product_model", model)
	campos.Set("barcode", barcode)
	campos.Set("price", r.FormValue("price"))
	campos.Set("unit", r.FormValue("unit"))
	campos.Set("category_id", r.FormValue("category_id"))
	campos.Set("product_vat", r.FormValue("product_vat"))
	campos.Set("tax", r.FormValue("tax"))
	campos.Set("product_details", r.FormValue("product_details"))
	campos.Set("item_type", r.FormValue("item_type"))

	codigo, datos, cuerpo := s.respuestaDeHandler(sess, campos, s.productSave)
	if codigo >= 400 {
		jsonError(w, mensajeDeError(datos, cuerpo), codigo)
		return
	}
	pid := ""
	if v, ok := datos["id"].(string); ok {
		pid = v
	}
	if pid == "" {
		pid = campos.Get("product_id")
	}
	p, err := s.datosProducto(r, pid)
	if err != nil {
		// El producto se creó; lo que no se pudo es releerlo. Se contesta el
		// mínimo que la fila necesita en vez de un error que haría creer que el
		// alta falló y provocaría un segundo intento (y un duplicado).
		p = map[string]interface{}{
			"status": 1, "product_id": pid, "id": pid, "name": name, "model": model,
			"barcode": barcode, "price": r.FormValue("price"), "unit": r.FormValue("unit"),
			"last_cost": 0, "stock": 0, "msg": "Producto creado",
		}
		writeJSON(w, p)
		return
	}
	s.aiAudit(sess, sessUserID(sess), "create", "products", "product_information", pid, nil,
		map[string]interface{}{"name": name, "origen": "alta rápida en compra"})
	writeJSON(w, p)
}

// datosProducto devuelve un producto con la forma que usan el catálogo y las
// filas de la compra.
func (s *Server) datosProducto(r *http.Request, pid string) (map[string]interface{}, error) {
	var id, name, model, barcode, unit, image, cat, itemType, wh sql.NullString
	var price, lastCost float64
	err := s.dbFor(r).QueryRow(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(a.barcode,''),
			IFNULL(a.unit,''), IFNULL(a.image,''), IFNULL(c.category_name,''), IFNULL(a.price,0),
			IFNULL(a.item_type,'venta'), IFNULL(a.default_warehouse_id,''),
			IFNULL((SELECT d.rate FROM product_purchase_details d
				JOIN product_purchase p ON p.purchase_id = d.purchase_id
				WHERE d.product_id = a.product_id ORDER BY p.id DESC LIMIT 1), 0)
		FROM product_information a
		LEFT JOIN product_category c ON c.category_id = a.category_id
		WHERE a.product_id = ?`, pid).
		Scan(&id, &name, &model, &barcode, &unit, &image, &cat, &price, &itemType, &wh, &lastCost)
	if err != nil {
		return nil, err
	}
	pidInt, _ := strconv.Atoi(id.String)
	return map[string]interface{}{
		"status": 1, "product_id": id.String, "id": id.String, "name": name.String,
		"model": model.String, "barcode": barcode.String, "unit": unit.String,
		"image": image.String, "category": cat.String, "price": price, "last_cost": lastCost,
		"stock": s.stockForProduct(r, pidInt), "item_type": normalizarTipoItem(itemType.String),
		"default_warehouse": wh.String,
		"msg":               "Producto creado",
	}, nil
}

// productPicker es el catálogo visual: la lista de productos con su foto, para
// identificar el artículo por su aspecto y no solo por el nombre.
//
// Devuelve el último costo de compra de cada uno: es el dato que convierte la
// lista en una herramienta de decisión, porque lo que se va a comparar es el
// precio que se está pagando ahora.
func (s *Server) productPicker(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.FormValue("term"))
	filtro := "a.status = 1"
	args := []interface{}{}
	if term != "" {
		like := "%" + term + "%"
		filtro += ` AND (a.product_name LIKE ? OR a.product_model LIKE ?
			OR IFNULL(a.barcode,'') LIKE ? OR IFNULL(a.serial_no,'') LIKE ?
			OR IFNULL(c.category_name,'') LIKE ?)`
		args = append(args, like, like, like, like, like)
	}
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(a.barcode,''),
			IFNULL(a.unit,''), IFNULL(a.price,0), IFNULL(a.image,''), IFNULL(c.category_name,''),
			IFNULL((SELECT d.rate FROM product_purchase_details d
				JOIN product_purchase p ON p.purchase_id = d.purchase_id
				WHERE d.product_id = a.product_id ORDER BY p.id DESC LIMIT 1), 0)
		FROM product_information a
		LEFT JOIN product_category c ON c.category_id = a.category_id
		WHERE `+filtro+`
		ORDER BY a.product_name LIMIT 60`, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, name, model, barcode, unit, image, cat sql.NullString
		var price, lastCost float64
		if err := rows.Scan(&id, &name, &model, &barcode, &unit, &price, &image, &cat, &lastCost); err != nil {
			continue
		}
		pidInt, _ := strconv.Atoi(id.String)
		out = append(out, map[string]interface{}{
			"id": id.String, "product_id": id.String, "name": name.String, "model": model.String,
			"barcode": barcode.String, "unit": unit.String, "price": price, "image": image.String,
			"category": cat.String, "last_cost": lastCost, "stock": s.stockForProduct(r, pidInt),
		})
	}
	writeJSON(w, map[string]interface{}{"status": 1, "products": out, "count": len(out)})
}

// supplierQuickCreate da de alta un proveedor desde el formulario de compra y
// devuelve su id para dejarlo seleccionado en la misma compra.
func (s *Server) supplierQuickCreate(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	name := strings.TrimSpace(r.FormValue("supplier_name"))
	if name == "" {
		jsonError(w, "El nombre del proveedor es obligatorio", 400)
		return
	}
	rif := NormalizeRIF(r.FormValue("rif"))

	if id := s.proveedorDuplicado(r, rif, name); id > 0 {
		writeJSON(w, map[string]interface{}{
			"status": 1, "supplier_id": id, "name": name, "rif": rif, "existente": true,
			"msg": "El proveedor ya existía y quedó seleccionado",
		})
		return
	}

	campos := url.Values{}
	campos.Set("supplier_name", name)
	campos.Set("rif", rif)
	campos.Set("mobile", r.FormValue("mobile"))
	campos.Set("email_address", r.FormValue("email_address"))
	campos.Set("address", r.FormValue("address"))
	campos.Set("city", r.FormValue("city"))
	campos.Set("details", r.FormValue("details"))

	codigo, datos, cuerpo := s.respuestaDeHandler(sess, campos, s.supplierSave)
	if codigo >= 400 {
		jsonError(w, mensajeDeError(datos, cuerpo), codigo)
		return
	}
	supplierID := int64(0)
	if v, ok := datos["supplier_id"].(float64); ok {
		supplierID = int64(v)
	}
	if supplierID <= 0 {
		jsonError(w, "El proveedor se guardó pero no se pudo recuperar su identificador", 500)
		return
	}
	s.aiAudit(sess, sessUserID(sess), "create", "suppliers", "supplier_information", strconv.FormatInt(supplierID, 10), nil,
		map[string]interface{}{"name": name, "origen": "alta rápida en compra"})
	writeJSON(w, map[string]interface{}{
		"status": 1, "supplier_id": supplierID, "name": name, "rif": rif, "msg": "Proveedor creado",
	})
}

// proveedorDuplicado busca el proveedor ya cargado por RIF o por nombre exacto.
//
// El RIF es la identidad fiscal del proveedor y por eso manda; el nombre se
// compara normalizado (sin espacios sobrantes y sin distinguir mayúsculas)
// porque es la otra forma en que se pide el mismo proveedor dos veces.
func (s *Server) proveedorDuplicado(r *http.Request, rif, name string) int64 {
	db := s.dbFor(r)
	if db == nil {
		return 0
	}
	if rif != "" {
		var id int64
		if err := db.QueryRow(`SELECT supplier_id FROM supplier_information WHERE TRIM(IFNULL(rif,'')) = ? LIMIT 1`, rif).Scan(&id); err == nil && id > 0 {
			return id
		}
	}
	if name == "" {
		return 0
	}
	var id int64
	if err := db.QueryRow(`SELECT supplier_id FROM supplier_information
		WHERE LOWER(TRIM(supplier_name)) = LOWER(?) ORDER BY supplier_id LIMIT 1`, name).Scan(&id); err == nil && id > 0 {
		return id
	}
	return 0
}
