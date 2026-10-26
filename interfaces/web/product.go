package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ---------- Product module (replicates modules/product) ----------

func (s *Server) registerProductRoutes() {
	// products
	s.mux.HandleFunc("GET /product/list", s.productList)
	s.mux.HandleFunc("/product/list/data", s.productListData)
	s.mux.HandleFunc("GET /product/add", s.productForm)
	s.mux.HandleFunc("GET /product/edit/{id}", s.productForm)
	s.mux.HandleFunc("POST /product/save", s.productSave)
	s.mux.HandleFunc("POST /product/delete/{id}", s.productDelete)
	s.mux.HandleFunc("GET /product/search", s.productSearch)
	s.mux.HandleFunc("GET /product/autocomplete", s.productAutocomplete)
	s.mux.HandleFunc("GET /product/stock-check", s.productStockCheck)
	s.mux.HandleFunc("GET /product/details/{id}", s.productDetails)
	// categories
	s.mux.HandleFunc("GET /product/category", s.categoryList)
	s.mux.HandleFunc("GET /product/category/data", s.categoryListData)
	s.mux.HandleFunc("POST /product/category/save", s.categorySave)
	s.mux.HandleFunc("POST /product/category/delete/{id}", s.categoryDelete)
	// units
	s.mux.HandleFunc("GET /product/unit", s.unitList)
	s.mux.HandleFunc("GET /product/unit/data", s.unitListData)
	s.mux.HandleFunc("POST /product/unit/save", s.unitSave)
	s.mux.HandleFunc("POST /product/unit/delete/{id}", s.unitDelete)
}

func (s *Server) productList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "product_list.html", map[string]interface{}{"Title": "Gestionar Productos"})
}

func (s *Server) productListData(w http.ResponseWriter, r *http.Request) {
	draw, _ := strconv.Atoi(r.FormValue("draw"))
	start, _ := strconv.Atoi(r.FormValue("start"))
	length, _ := strconv.Atoi(r.FormValue("length"))
	if length <= 0 || length > 1000 {
		length = 10
	}
	search := strings.TrimSpace(r.FormValue("search[value]"))

	where := ""
	args := []interface{}{}
	if search != "" {
		where = " WHERE a.product_name LIKE ? OR a.product_model LIKE ? "
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	var total, filtered int
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_information").Scan(&total)
	_ = s.dbFor(r).QueryRow("SELECT COUNT(*) FROM product_information a"+where, args...).Scan(&filtered)

	// stock = purchases - sales (replicates product_stock pattern)
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), IFNULL(c.category_name,''),
			IFNULL(a.unit,''), a.price,
			IFNULL(a.status,1)
		FROM product_information a
		LEFT JOIN product_category c ON c.category_id = a.category_id
		`+where+`
		ORDER BY a.id DESC
		LIMIT ? OFFSET ?`, append(args, length, start)...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var pid, name, model, cat, unit, price sql.NullString
		var status int
		if err := rows.Scan(&pid, &name, &model, &cat, &unit, &price, &status); err != nil {
			continue
		}
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		list = append(list, map[string]interface{}{
			"id":       pid.String,
			"name":     name.String,
			"model":    model.String,
			"category": cat.String,
			"unit":     unit.String,
			"price":    price.String,
			"stock":    stock,
			"status":   status,
		})
	}
	writeJSON(w, map[string]interface{}{
		"draw":            draw,
		"recordsTotal":    total,
		"recordsFiltered": filtered,
		"data":            list,
	})
}

func (s *Server) productForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Añadir Producto"}
	// categories + units for dropdowns
	cats := []map[string]interface{}{}
	units := []map[string]interface{}{}
	// El selector va ordenado por ÁRBOL y con la profundidad pintada en la
	// etiqueta ("Calzado", "\u00a0\u00a0Hombres", …). MySQL ordenaría los caminos
	// lexicográficamente —10 antes que 2—, así que el orden se calcula con el
	// mismo árbol que usa la pantalla de categorías.
	if nodos, err := s.categoriaArbol(r); err == nil {
		for _, n := range nodos {
			cats = append(cats, map[string]interface{}{
				"id":    n.ID,
				"name":  n.Name,
				"label": strings.Repeat("\u00a0\u00a0", n.Depth) + n.Name,
			})
		}
	}
	rows, _ := s.dbFor(r).Query(`SELECT unit_id, unit_name FROM units WHERE status=1 ORDER BY unit_name`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var name string
			_ = rows.Scan(&id, &name)
			units = append(units, map[string]interface{}{"id": id, "name": name})
		}
		rows.Close()
	}
	data["Categories"] = cats
	data["Units"] = units

	if id := r.PathValue("id"); id != "" {
		var p struct {
			ID, Name, Model, Cat, Unit, Price, Serial, Vat, Tax, Details, Image, Barcode sql.NullString
		}
		err := s.dbFor(r).QueryRow(`SELECT product_id, product_name, product_model, category_id, unit, price,
			serial_no, product_vat, tax, product_details, image, IFNULL(barcode,'') FROM product_information WHERE product_id = ?`, id).
			Scan(&p.ID, &p.Name, &p.Model, &p.Cat, &p.Unit, &p.Price, &p.Serial, &p.Vat, &p.Tax, &p.Details, &p.Image, &p.Barcode)
		if err != nil {
			http.Error(w, "Producto no encontrado", 404)
			return
		}
		data["Title"] = "Editar Producto"
		data["P"] = p
	}
	s.render(w, r, "product_form.html", data)
}

func (s *Server) productSave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("product_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	id := r.FormValue("product_id")

	image := ""
	// Optional image upload via uploadMgr (tenant-aware)
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err == nil {
			if file, hdr, err := r.FormFile("image"); err == nil {
				defer file.Close()
				ext := strings.ToLower(filepath.Ext(hdr.Filename))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
					// Get companyID from session (default to 1)
					cid := "1"
					if err := s.dbFor(r).QueryRow(
						"SELECT IFNULL(company_id, '1') FROM company_information WHERE status = 1 ORDER BY company_id LIMIT 1",
					).Scan(&cid); err != nil {
						cid = "1"
					}
					result, err := s.uploadMgr.Save(cid, "productos", file, hdr, s.dbFor(r))
					if err == nil {
						image = result.Path
					}
				}
			}
		}
	}

	if id == "" {
		pid := generator(8)
		_, err := s.dbFor(r).Exec(`
			INSERT INTO product_information (product_id, category_id, product_name, price, price_usd, currency_code, unit, tax,
				serial_no, product_vat, product_model, product_details, image, barcode, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`,
			pid, r.FormValue("category_id"), name, r.FormValue("price"), parseMoney(r.FormValue("price_usd")), orDefault(r.FormValue("currency_code"), "VES"), r.FormValue("unit"),
			parseMoney(r.FormValue("tax")), r.FormValue("serial_no"), parseMoney(r.FormValue("product_vat")),
			r.FormValue("product_model"), r.FormValue("product_details"), image, r.FormValue("barcode"))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Producto creado", "id": pid})
		return
	}

	// Si el usuario escribe el código del producto, este tiene que ser el
	// ALTA, no una modificación.
	//
	// Antes, mandar un `product_id` que no existía hacía un UPDATE que no
	// tocaba ninguna fila y contestaba "Producto actualizado": el producto no
	// se creaba y el usuario veía que sí. Es el fallo más caro posible en un
	// alta, porque no da error y deja el catálogo sin el artículo.
	//
	// `product_id` es la clave natural —todo el resto de tablas se une por
	// ella— y por eso el guardado es un alta o una modificación según exista,
	// nunca una modificación ciega.
	var existe int
	if err := s.dbFor(r).QueryRow(
		`SELECT COUNT(*) FROM product_information WHERE product_id = ?`, id).Scan(&existe); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if existe == 0 {
		if _, err := s.dbFor(r).Exec(`
			INSERT INTO product_information (product_id, category_id, product_name, price, unit, tax,
				serial_no, product_vat, product_model, product_details, image, barcode, status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1)`,
			id, r.FormValue("category_id"), name, r.FormValue("price"), r.FormValue("unit"),
			parseMoney(r.FormValue("tax")), r.FormValue("serial_no"), parseMoney(r.FormValue("product_vat")),
			r.FormValue("product_model"), r.FormValue("product_details"), image, r.FormValue("barcode")); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]interface{}{"status": 1, "msg": "Producto creado", "id": id})
		return
	}

	// UPDATE: producto existe, actualizar con precio dual
	if image == "" {
		_, err := s.dbFor(r).Exec(`
			UPDATE product_information SET category_id=?, product_name=?, price=?, price_usd=?, currency_code=?, unit=?, tax=?,
				serial_no=?, product_vat=?, product_model=?, product_details=?, barcode=? WHERE product_id=?`,
			r.FormValue("category_id"), name, r.FormValue("price"), parseMoney(r.FormValue("price_usd")), orDefault(r.FormValue("currency_code"), "VES"), r.FormValue("unit"),
			parseMoney(r.FormValue("tax")), r.FormValue("serial_no"), parseMoney(r.FormValue("product_vat")),
			r.FormValue("product_model"), r.FormValue("product_details"), r.FormValue("barcode"), id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		_, err := s.dbFor(r).Exec(`
			UPDATE product_information SET category_id=?, product_name=?, price=?, price_usd=?, currency_code=?, unit=?, tax=?,
				serial_no=?, product_vat=?, product_model=?, product_details=?, image=?, barcode=? WHERE product_id=?`,
			r.FormValue("category_id"), name, r.FormValue("price"), parseMoney(r.FormValue("price_usd")), orDefault(r.FormValue("currency_code"), "VES"), r.FormValue("unit"),
			parseMoney(r.FormValue("tax")), r.FormValue("serial_no"), parseMoney(r.FormValue("product_vat")),
			r.FormValue("product_model"), r.FormValue("product_details"), image, r.FormValue("barcode"), id)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Producto actualizado", "redirect": saveRedirect(r, "/product/list")})
}

func (s *Server) productDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// check invoice usage
	var used int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM invoice_details WHERE product_id = ?`, id).Scan(&used)
	if used > 0 {
		jsonError(w, "No se puede eliminar: el producto tiene facturas asociadas", 400)
		return
	}
	res, err := s.dbFor(r).Exec(`DELETE FROM product_information WHERE product_id = ?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "Producto no encontrado", 404)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/product/list")})
}

// productSearch: find products for invoice/purchase line pickers.
func (s *Server) productSearch(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.FormValue("term"))
	supplier := r.FormValue("supplier_id")
	rows, err := s.dbFor(r).Query(`
		SELECT a.product_id, a.product_name, IFNULL(a.product_model,''), a.price, IFNULL(a.unit,''),
			IFNULL(sp.supplier_price, a.price) AS supplier_price
		FROM product_information a
		LEFT JOIN supplier_product sp ON sp.product_id = a.product_id AND sp.supplier_id = ?
		WHERE a.status = 1 AND (a.product_name LIKE ? OR a.product_model LIKE ?)
		ORDER BY a.product_name LIMIT 20`, supplier, "%"+term+"%", "%"+term+"%")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var pid, name, model, price, unit, sprice sql.NullString
		_ = rows.Scan(&pid, &name, &model, &price, &unit, &sprice)
		pidInt, _ := strconv.Atoi(pid.String)
		stock := s.stockForProduct(r, pidInt)
		out = append(out, map[string]interface{}{
			"id": pid.String, "label": name.String + " (" + model.String + ")",
			"product_id": pid.String, "name": name.String, "model": model.String,
			"price": price.String, "supplier_price": sprice.String, "unit": unit.String, "stock": stock,
		})
	}
	writeJSON(w, out)
}

// productAutocomplete: same as search but lighter (POS).
func (s *Server) productAutocomplete(w http.ResponseWriter, r *http.Request) {
	s.productSearch(w, r)
}

// productStockCheck: available quantity of a product.
func (s *Server) productStockCheck(w http.ResponseWriter, r *http.Request) {
	pid := r.FormValue("product_id")
	pidInt, _ := strconv.Atoi(pid)
	stock := s.stockForProduct(r, pidInt)
	writeJSON(w, map[string]interface{}{"available": stock})
}

// productDetails: purchase/sales history + stock.
func (s *Server) productDetails(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, model, price, image sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT product_name, IFNULL(product_model,''), price, IFNULL(image,'') FROM product_information WHERE product_id=?`, id).
		Scan(&name, &model, &price, &image)
	if err != nil {
		http.Error(w, "Producto no encontrado", 404)
		return
	}

	type hrow struct {
		Date  string
		Ref   string
		Qty   float64
		Rate  float64
		Total float64
	}
	purchases := []hrow{}
	sales := []hrow{}
	var stock float64

	rows, _ := s.dbFor(r).Query(`
		SELECT pp.purchase_date, ppd.purchase_id, ppd.quantity, ppd.rate, ppd.total_amount
		FROM product_purchase_details ppd JOIN product_purchase pp ON pp.purchase_id = ppd.purchase_id
		WHERE ppd.product_id = ? ORDER BY pp.id DESC LIMIT 50`, id)
	if rows != nil {
		for rows.Next() {
			var d, ref sql.NullString
			var h hrow
			_ = rows.Scan(&d, &ref, &h.Qty, &h.Rate, &h.Total)
			h.Date, h.Ref = d.String, ref.String
			purchases = append(purchases, h)
		}
		rows.Close()
	}
	rows, _ = s.dbFor(r).Query(`
		SELECT i.date, i.invoice_id, id.quantity, id.rate, id.total_price
		FROM invoice_details id JOIN invoice i ON i.invoice_id = id.invoice_id
		WHERE id.product_id = ? ORDER BY i.id DESC LIMIT 50`, id)
	if rows != nil {
		for rows.Next() {
			var d, ref sql.NullString
			var h hrow
			_ = rows.Scan(&d, &ref, &h.Qty, &h.Rate, &h.Total)
			h.Date, h.Ref = d.String, ref.String
			sales = append(sales, h)
		}
		rows.Close()
	}
	pidInt, _ := strconv.Atoi(id)
	stock = s.stockForProduct(r, pidInt)

	s.render(w, r, "product_details.html", map[string]interface{}{
		"Title":     "Detalle: " + name.String,
		"PID":       id,
		"PName":     name.String,
		"PModel":    model.String,
		"PPrice":    price.String,
		"Purchases": purchases,
		"Sales":     sales,
		"Stock":     stock,
		"Image":     image.String,
	})
}

// ---------- Categories ----------

func (s *Server) categoryList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "category_list.html", map[string]interface{}{"Title": "Categorías de Producto"})
}

// catNodo es una fila de `product_category` colocada en su sitio del árbol.
type catNodo struct {
	ID       int64
	Name     string
	Parent   int64
	Sort     int
	Products int // productos directos en ESTA categoría
	Depth    int // profundidad real (0 = raíz); se calcula aquí, no en SQL
}

// categoriaArbol devuelve las categorías activas aplanadas en orden
// jerárquico: cada raíz seguida de su rama.
//
// El orden NO se delega al ORDER BY: los caminos ('/1/', '/1/10/') ordenan
// lexicográficamente y el 10 quedaría antes que el 2. El catálogo de un
// comercio es chico (decenas), así que el árbol se ordena en Go por hermano
// (sort_order, id) y se aplana en profundidad primera.
func (s *Server) categoriaArbol(r *http.Request) ([]catNodo, error) {
	rows, err := s.dbFor(r).Query(`
		SELECT c.category_id, c.category_name, IFNULL(c.parent_id, 0),
		       IFNULL(c.sort_order, 0),
		       (SELECT COUNT(*) FROM product_information p WHERE p.category_id = c.category_id)
		FROM product_category c WHERE c.status = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todo []catNodo
	for rows.Next() {
		var n catNodo
		if err := rows.Scan(&n.ID, &n.Name, &n.Parent, &n.Sort, &n.Products); err != nil {
			continue
		}
		todo = append(todo, n)
	}
	return construirArbol(todo), nil
}

// construirArbol ordena y aplana las categorías. Es pura (sin base) para que
// la parte delicada —ordenar por hermano y no por texto, y promover huérfanos
// a raíz— se pueda probar sin servidor.
func construirArbol(todo []catNodo) []catNodo {
	existe := map[int64]bool{}
	for _, n := range todo {
		existe[n.ID] = true
	}

	hijos := map[int64][]catNodo{}
	var raices []catNodo
	for _, n := range todo {
		// Un padre que no existe (borrado a mano, dato heredado) no convierte a
		// la categoría en algo invisible: sube a raíz y sigue listable.
		if n.Parent == 0 || !existe[n.Parent] {
			n.Parent = 0
			raices = append(raices, n)
		} else {
			hijos[n.Parent] = append(hijos[n.Parent], n)
		}
	}

	ordenar := func(g []catNodo) {
		sort.Slice(g, func(i, j int) bool {
			if g[i].Sort != g[j].Sort {
				return g[i].Sort < g[j].Sort
			}
			return g[i].ID < g[j].ID
		})
	}
	ordenar(raices)
	for _, g := range hijos {
		ordenar(g)
	}

	var plano []catNodo
	var bajar func(n catNodo, depth int)
	bajar = func(n catNodo, depth int) {
		n.Depth = depth
		plano = append(plano, n)
		for _, h := range hijos[n.ID] {
			bajar(h, depth+1)
		}
	}
	for _, raiz := range raices {
		bajar(raiz, 0)
	}
	return plano
}

// productosRama suma por categoría el total de su rama completa (ella y sus
// descendientes). Es pura y se recorre de atrás hacia adelante: en
// profundidad primera los hijos van DESPUÉS del padre, así que al retroceder
// cada hijo ya trae su total cuando lo aporta al suyo.
func productosRama(nodos []catNodo) map[int64]int {
	rama := map[int64]int{}
	for i := len(nodos) - 1; i >= 0; i-- {
		n := nodos[i]
		total := n.Products + rama[n.ID]
		rama[n.ID] = total
		if n.Parent != 0 {
			rama[n.Parent] += total
		}
	}
	return rama
}

func (s *Server) categoryListData(w http.ResponseWriter, r *http.Request) {
	nodos, err := s.categoriaArbol(r)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// Productos de la RAMA (la categoría y todo lo suyo). Se suman recorriendo
	// al revés: en profundidad primera los hijos van después del padre, así
	// que al retroceder cada hijo ya trae su total cuando lo suma el padre.
	rama := productosRama(nodos)
	list := []map[string]interface{}{}
	for _, n := range nodos {
		list = append(list, map[string]interface{}{
			"id": n.ID, "name": n.Name, "parent": n.Parent,
			"depth": n.Depth, "products": n.Products, "rama": rama[n.ID],
		})
	}
	writeJSON(w, list)
}

func (s *Server) categorySave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("category_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	id := r.FormValue("category_id")
	db := s.dbFor(r)

	// El padre: vacío o "0" = raíz.
	var parentID int64
	if pv := strings.TrimSpace(r.FormValue("parent_id")); pv != "" && pv != "0" {
		pid, err := strconv.ParseInt(pv, 10, 64)
		if err != nil {
			jsonError(w, "La categoría padre no es válida", 400)
			return
		}
		parentID = pid
	}

	// Validaciones de árbol ANTES de escribir: una categoría no puede ser hija
	// de sí misma ni de un descendiente suyo — eso cerraría un ciclo y dejaría
	// la rama inatraversable de forma permanente.
	var caminoPadre string
	nivelPadre := 0
	if parentID != 0 {
		if id != "" {
			idInt, _ := strconv.ParseInt(id, 10, 64)
			if parentID == idInt {
				jsonError(w, "Una categoría no puede ser hija de sí misma", 400)
				return
			}
		}
		err := db.QueryRow(`SELECT IFNULL(path,''), IFNULL(level,0) FROM product_category WHERE category_id = ? AND status = 1`, parentID).
			Scan(&caminoPadre, &nivelPadre)
		if err == sql.ErrNoRows {
			jsonError(w, "La categoría padre no existe", 400)
			return
		}
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if id != "" && strings.HasPrefix(caminoPadre, "/"+id+"/") {
			jsonError(w, "No puede moverse bajo una subcategoría suya", 400)
			return
		}
	}

	// La ruta: del padre, o '/raíz/' si no hay padre.
	base := "/"
	if parentID != 0 {
		base = caminoPadre
	}
	nivel := 0
	if parentID != 0 {
		nivel = nivelPadre + 1
	}

	if id == "" {
		var parentArg interface{}
		if parentID != 0 {
			parentArg = parentID
		}
		res, err := db.Exec(`INSERT INTO product_category (category_name, status, parent_id) VALUES (?,1,?)`, name, parentArg)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		newID, _ := res.LastInsertId()
		camino := fmt.Sprintf("%s%d/", base, newID)
		if _, err := db.Exec(`UPDATE product_category SET path = ?, level = ? WHERE category_id = ?`, camino, nivel, newID); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		var caminoViejo string
		nivelViejo := 0
		if err := db.QueryRow(`SELECT IFNULL(path,''), IFNULL(level,0) FROM product_category WHERE category_id = ?`, id).
			Scan(&caminoViejo, &nivelViejo); err != nil {
			jsonError(w, "Categoría no encontrada", 404)
			return
		}
		var parentArg interface{}
		if parentID != 0 {
			parentArg = parentID
		}
		caminoNuevo := base + id + "/"
		if _, err := db.Exec(`UPDATE product_category SET category_name = ?, parent_id = ?, path = ?, level = ? WHERE category_id = ?`,
			name, parentArg, caminoNuevo, nivel, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		// Mover una categoría mueve su rama: a los descendientes se les
		// reescribe el prefijo viejo por el nuevo y se les desplaza el nivel.
		// Sin esto, los caminos de la rama quedarían apuntando a un sitio al
		// que la categoría ya no pertenece y el filtrado por rama mentiría.
		if caminoViejo != "" && caminoNuevo != caminoViejo {
			delta := nivel - nivelViejo
			_, _ = db.Exec(`UPDATE product_category
				SET path = CONCAT(?, SUBSTRING(path, ?)), level = level + ?
				WHERE path LIKE CONCAT(?, '%') AND category_id <> ?`,
				caminoNuevo, len(caminoViejo)+1, delta, caminoViejo, id)
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado", "redirect": saveRedirect(r, "/product/list")})
}

func (s *Server) categoryDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := s.dbFor(r)

	// Con jerarquía, eliminar una categoría con hijos dejaría a los hijos
	// colgando de un id que ya no existe. Se pide borrar de abajo hacia arriba.
	var hijos int
	_ = db.QueryRow(`SELECT COUNT(*) FROM product_category WHERE parent_id = ?`, id).Scan(&hijos)
	if hijos > 0 {
		jsonError(w, "Tiene subcategorías: elimine primero las subcategorías", 400)
		return
	}

	// Los productos de la rama entera cuentan: la categoría vacía pero con
	// productos en una subcategoría no se puede borrar tampoco.
	var camino string
	_ = db.QueryRow(`SELECT IFNULL(path,'') FROM product_category WHERE category_id = ?`, id).Scan(&camino)
	var used int
	if camino != "" {
		_ = db.QueryRow(`SELECT COUNT(*) FROM product_information p
			WHERE p.category_id IN (SELECT category_id FROM product_category WHERE path LIKE CONCAT(?,'%'))`, camino).Scan(&used)
	} else {
		_ = db.QueryRow(`SELECT COUNT(*) FROM product_information WHERE category_id = ?`, id).Scan(&used)
	}
	if used > 0 {
		jsonError(w, "No se puede eliminar: hay productos en esta categoría o en sus subcategorías", 400)
		return
	}
	if _, err := db.Exec(`DELETE FROM product_category WHERE category_id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/product/list")})
}

// ---------- Units ----------

func (s *Server) unitList(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "unit_list.html", map[string]interface{}{"Title": "Unidades de Medida"})
}

func (s *Server) unitListData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT unit_id, unit_name FROM units WHERE status=1 ORDER BY unit_name`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	list := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		_ = rows.Scan(&id, &name)
		list = append(list, map[string]interface{}{"id": id, "name": name})
	}
	writeJSON(w, list)
}

func (s *Server) unitSave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("unit_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	id := r.FormValue("unit_id")
	if id == "" {
		if _, err := s.dbFor(r).Exec(`INSERT INTO units (unit_name, status) VALUES (?,1)`, name); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if _, err := s.dbFor(r).Exec(`UPDATE units SET unit_name=? WHERE unit_id=?`, name, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Guardado", "redirect": saveRedirect(r, "/product/list")})
}

func (s *Server) unitDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var used int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM product_information WHERE unit = ?`, id).Scan(&used)
	if used > 0 {
		jsonError(w, "No se puede eliminar: hay productos usando esta unidad", 400)
		return
	}
	if _, err := s.dbFor(r).Exec(`DELETE FROM units WHERE unit_id = ?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminado", "redirect": saveRedirect(r, "/product/list")})
}

var _ = fmt.Sprintf
