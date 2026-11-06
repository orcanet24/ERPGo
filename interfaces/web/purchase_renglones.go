package web

// purchase_renglones.go — Cada renglón de compra dice QUÉ es y A DÓNDE va.
//
// Antes el guardado de compras recorría los renglones para insertarlos en el
// detalle y, sin preguntar nada, metía stock y asentaba todo contra la cuenta de
// inventario. Aquí se resuelve el renglón ANTES de escribir: tipo de ítem,
// almacén, cuenta que se debita y si mueve stock. Si algo no cuadra, la compra
// no se guarda y el usuario recibe el motivo concreto.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// renglonCompra es un renglón ya interpretado.
type renglonCompra struct {
	ProductID string
	Tipo      string
	Almacen   string
	Cuenta    int64
	Concepto  string

	Qty, Rate, Total        float64
	Disc, DiscP, VatP, VatV float64
	Batch, Expiry           string

	Destino destinoCompra
}

// renglonesDeCompra arma los renglones desde los arreglos del formulario.
//
// El tipo y el almacén pueden venir del formulario; si no vienen, se toman del
// catálogo (item_type y el almacén por defecto del tipo). Un renglón de servicio
// exige concepto; uno de mercancía o consumo exige producto.
func (s *Server) renglonesDeCompra(r *http.Request, ids, quantities, rates, totals, discounts,
	discPercents, vatPercents, vatValues, batches, expiries []string) ([]renglonCompra, error) {

	types := r.Form["line_item_type"]
	warehouses := r.Form["line_warehouse"]
	coas := r.Form["line_coa"]
	concepts := r.Form["line_concept"]

	if len(ids) == 0 {
		return nil, fmt.Errorf("agregue al menos un renglón a la compra")
	}

	out := make([]renglonCompra, 0, len(ids))
	for i, pid := range ids {
		pid = strings.TrimSpace(pid)
		l := renglonCompra{ProductID: pid}
		if i < len(quantities) {
			l.Qty = parseMoney(quantities[i])
		}
		if i < len(rates) {
			l.Rate = parseMoney(rates[i])
		}
		if i < len(totals) {
			l.Total = parseMoney(totals[i])
		}
		if i < len(discounts) {
			l.Disc = parseMoney(discounts[i])
		}
		if i < len(discPercents) {
			l.DiscP = parseMoney(discPercents[i])
		}
		if i < len(vatPercents) {
			l.VatP = parseMoney(vatPercents[i])
		}
		if i < len(vatValues) {
			l.VatV = parseMoney(vatValues[i])
		}
		if i < len(batches) {
			l.Batch = batches[i]
		}
		if i < len(expiries) {
			l.Expiry = expiries[i]
		}
		if i < len(concepts) {
			l.Concepto = strings.TrimSpace(concepts[i])
		}
		if i < len(coas) {
			l.Cuenta, _ = strconv.ParseInt(strings.TrimSpace(coas[i]), 10, 64)
		}

		tipoPedido := ""
		if i < len(types) {
			tipoPedido = strings.TrimSpace(types[i])
		}

		// El tipo del catálogo manda. Un producto marcado como servicio no puede
		// comprarse "como mercancía": sería volver a meter un gasto al inventario
		// con otro nombre.
		tipoCatalogo := ""
		if pid != "" {
			var item sql.NullString
			err := s.dbFor(r).QueryRow(`SELECT IFNULL(item_type,'venta') FROM product_information WHERE product_id = ?`, pid).Scan(&item)
			if err != nil {
				return nil, fmt.Errorf("el producto %s no existe en el catálogo", pid)
			}
			tipoCatalogo = normalizarTipoItem(item.String)
		}

		switch {
		case tipoPedido == "" && tipoCatalogo != "":
			l.Tipo = tipoCatalogo
		case tipoPedido == "":
			l.Tipo = ItemVenta
		default:
			l.Tipo = normalizarTipoItem(tipoPedido)
		}
		if tipoCatalogo != "" && tipoCatalogo != l.Tipo {
			return nil, fmt.Errorf("el producto %s está marcado como %s en el catálogo; no se puede cargar como %s",
				pid, etiquetaTipoItem(tipoCatalogo), etiquetaTipoItem(l.Tipo))
		}
		if l.Tipo != ItemServicio && pid == "" {
			return nil, fmt.Errorf("el renglón %d es %s y necesita un producto del catálogo", i+1, etiquetaTipoItem(l.Tipo))
		}
		if l.Tipo == ItemServicio && l.Concepto == "" {
			return nil, fmt.Errorf("el renglón %d es un servicio o gasto y necesita un concepto", i+1)
		}
		if l.Qty <= 0 {
			return nil, fmt.Errorf("el renglón %d no tiene cantidad", i+1)
		}
		if l.Total <= 0 {
			l.Total = l.Qty * l.Rate
		}

		if i < len(warehouses) {
			l.Almacen = strings.TrimSpace(warehouses[i])
		}
		out = append(out, l)
	}
	return out, nil
}

// resolverDestinosCompra completa tipo, almacén y cuenta de cada renglón.
// Es el punto donde se decide si algo entra a inventario o se va a gasto.
func (s *Server) resolverDestinosCompra(r *http.Request, pre *PredefineAccount, renglones []renglonCompra) error {
	for i := range renglones {
		l := &renglones[i]
		d, err := destinoDeRenglonCompra(pre, l.Tipo, l.Almacen, l.Cuenta)
		if err != nil {
			return fmt.Errorf("renglón %d (%s): %w", i+1, etiquetaTipoItem(l.Tipo), err)
		}
		if d.MueveStock {
			// Si el formulario no mandó almacén, se usa el del tipo.
			if strings.TrimSpace(l.Almacen) == "" {
				l.Almacen = s.almacenPorDefecto(r, tipoAlmacenDeItem(l.Tipo))
				d.Almacen = l.Almacen
			}
			if err := s.almacenValidoPara(r, l.Almacen, tipoAlmacenDeItem(l.Tipo)); err != nil {
				return fmt.Errorf("renglón %d: %w", i+1, err)
			}
		}
		if l.Tipo == ItemServicio {
			if err := s.cuentaDeGastoValida(r, d.Cuenta); err != nil {
				return fmt.Errorf("renglón %d: %w", i+1, err)
			}
		}
		l.Tipo = d.TipoItem
		l.Destino = d
	}
	return nil
}

// cuentaDeGastoValida comprueba que la cuenta exista, esté activa y sea de
// gasto (E) o de activo (A: un servicio capitalizable). Asentarlo contra una
// cuenta de ingreso —como hacía el módulo de servicios— es exactamente lo que
// no se quiere volver a permitir.
func (s *Server) cuentaDeGastoValida(r *http.Request, coa int64) error {
	if coa == 0 {
		return fmt.Errorf("falta la cuenta contable")
	}
	var tipo string
	var activa, transaccional int
	err := s.dbFor(r).QueryRow(`SELECT HeadType, IFNULL(IsActive,1), IFNULL(IsTransaction,1)
		FROM acc_coa WHERE HeadCode = ?`, coa).Scan(&tipo, &activa, &transaccional)
	if err != nil {
		return fmt.Errorf("la cuenta %d no existe en el plan de cuentas", coa)
	}
	if activa != 1 || transaccional != 1 {
		return fmt.Errorf("la cuenta %d no está activa o no admite movimientos", coa)
	}
	tipo = strings.ToUpper(strings.TrimSpace(tipo))
	if tipo != "E" && tipo != "A" {
		return fmt.Errorf("la cuenta %d es de tipo %s y un gasto o servicio debe ir a una cuenta de gasto o de activo", coa, tipo)
	}
	return nil
}

// repartirImporte reparte un importe entre varios pesos y cierra la suma
// exacta: el último renglón con peso absorbe la diferencia de redondeo. Sin
// esto, una compra de tres renglones con IVA dejaba centavos sueltos y el
// asiento no cuadraba.
func repartirImporte(total float64, pesos []float64) []float64 {
	out := make([]float64, len(pesos))
	if len(pesos) == 0 {
		return out
	}
	var suma float64
	ultimo := -1
	for i, p := range pesos {
		if p > 0 {
			suma += p
			ultimo = i
		}
	}
	if ultimo < 0 {
		return out
	}
	if suma == 0 {
		parte := round2(total / float64(len(pesos)))
		for i := range out {
			out[i] = parte
		}
	} else {
		for i, p := range pesos {
			if p > 0 {
				out[i] = round2(total * p / suma)
			}
		}
	}
	var acum float64
	for i := 0; i < len(out); i++ {
		if i != ultimo {
			acum += out[i]
		}
	}
	out[ultimo] = round2(total - acum)
	return out
}
