package web

// items_almacen.go — La separación que el sistema no tenía: qué es cada cosa que
// se compra y en qué almacén vive.
//
// POR QUÉ EXISTE ESTE ARCHIVO
//
// Antes, comprar era una sola cosa: todo renglón entraba a inventario, todo
// renglón movía stock al almacén fijo "WH-MAIN" y todo el documento se asentaba
// contra la cuenta de inventario de mercancía. Un servicio (la luz, el internet)
// terminaba siendo un activo con existencias; la papelería y los productos de
// limpieza se mezclaban con la mercancía de venta y, al sacarlos, se cargaban al
// costo de ventas.
//
// Aquí se define, en un solo lugar, la respuesta a tres preguntas:
//
//   1. ¿Qué es este ítem?           venta | consumo | servicio
//   2. ¿Mueve stock?                venta y consumo sí; servicio nunca
//   3. ¿En qué almacén y contra qué cuenta?
//
// Compras, ventas, ajustes, consumo interno, kárdex y reportes llaman a estas
// funciones. Si la regla cambia, cambia aquí y cambia en todas partes: no hay
// una copia de la regla por pantalla.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Tipos de ítem del catálogo. Se guardan en product_information.item_type.
const (
	ItemVenta    = "venta"    // mercancía para la venta: inventario comercial
	ItemConsumo  = "consumo"  // suministro de uso propio: papelería, limpieza
	ItemServicio = "servicio" // servicio o gasto: nunca es inventario
)

// Tipos de almacén. Se guardan en warehouse.kind.
const (
	AlmacenTipoVenta   = "venta"
	AlmacenTipoConsumo = "consumo"
)

// Almacenes de respaldo cuando la empresa todavía no tiene filas en `warehouse`.
// Son los mismos códigos que usa el sistema desde el principio, para no romper
// los movimientos históricos.
const (
	AlmacenVentaPorDefecto   = "WH-MAIN"
	AlmacenConsumoPorDefecto = "WH-CONSUMO"
)

// normalizarTipoItem traduce lo que llega del formulario o de la base a uno de
// los tres tipos. Tolera los nombres del esquema viejo ("physical"/"service")
// porque hay movimientos y productos anteriores a esta separación.
func normalizarTipoItem(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ItemConsumo, "suministro", "suministros", "supply", "interno", "autoconsumo", "uso interno":
		return ItemConsumo
	case ItemServicio, "service", "gasto":
		return ItemServicio
	default:
		// "" es el valor de todo el catálogo existente: mercancía de venta.
		return ItemVenta
	}
}

// etiquetaTipoItem es el texto que ve el usuario.
func etiquetaTipoItem(v string) string {
	switch normalizarTipoItem(v) {
	case ItemConsumo:
		return "Consumo interno"
	case ItemServicio:
		return "Servicio / gasto"
	default:
		return "Producto de venta"
	}
}

// itemMueveStock dice si el tipo de ítem vive en un almacén.
// Un servicio no tiene existencias: no entra, no sale, no se cuenta.
func itemMueveStock(v string) bool {
	return normalizarTipoItem(v) != ItemServicio
}

// tipoAlmacenDeItem devuelve el tipo de almacén que le corresponde al ítem.
// Vacío para servicios: no se guardan en ninguna parte.
func tipoAlmacenDeItem(v string) string {
	switch normalizarTipoItem(v) {
	case ItemConsumo:
		return AlmacenTipoConsumo
	case ItemServicio:
		return ""
	default:
		return AlmacenTipoVenta
	}
}

// almacenPorDefectoPara devuelve el código de almacén de respaldo del tipo.
func almacenPorDefectoPara(kind string) string {
	if kind == AlmacenTipoConsumo {
		return AlmacenConsumoPorDefecto
	}
	return AlmacenVentaPorDefecto
}

// Almacen es una ubicación física o lógica de existencias.
type Almacen struct {
	ID        int64
	Code      string
	Name      string
	Kind      string
	IsDefault bool
	IsActive  bool
}

// almacenesDe lee los almacenes activos de la empresa.
//
// Tolera dos estados intermedios reales: bases anteriores a la migración (sin
// columna `kind`) y bases sin ninguna fila en `warehouse`. En el primer caso
// todos son de venta; en el segundo se devuelven los códigos de respaldo, para
// que una instalación vieja siga funcionando mientras se migra.
func (s *Server) almacenesDe(r *http.Request) []Almacen {
	rows, err := s.dbFor(r).Query(`
		SELECT id, code, name, IFNULL(kind,'venta'), IFNULL(is_default,0), IFNULL(is_active,1)
		FROM warehouse
		WHERE IFNULL(is_active,1) = 1
		ORDER BY is_default DESC, name ASC`)
	if err != nil {
		// Sin la columna `kind`: todos los almacenes son de venta.
		rows, err = s.dbFor(r).Query(`
			SELECT id, code, name, 'venta', IFNULL(is_default,0), IFNULL(is_active,1)
			FROM warehouse
			WHERE IFNULL(is_active,1) = 1
			ORDER BY is_default DESC, name ASC`)
	}
	if err == nil {
		defer rows.Close()
		lista := []Almacen{}
		for rows.Next() {
			var a Almacen
			var def, act int
			if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Kind, &def, &act); err != nil {
				continue
			}
			a.Kind = normalizarTipoAlmacen(a.Kind)
			a.IsDefault = def == 1
			a.IsActive = act == 1
			lista = append(lista, a)
		}
		if len(lista) > 0 {
			return lista
		}
	}
	return []Almacen{
		{Code: AlmacenVentaPorDefecto, Name: "Almacén Principal", Kind: AlmacenTipoVenta, IsDefault: true, IsActive: true},
		{Code: AlmacenConsumoPorDefecto, Name: "Consumo interno", Kind: AlmacenTipoConsumo, IsActive: true},
	}
}

// normalizarTipoAlmacen evita que un valor desconocido saque un almacén de la
// lista de venta por accidente.
func normalizarTipoAlmacen(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), AlmacenTipoConsumo) {
		return AlmacenTipoConsumo
	}
	return AlmacenTipoVenta
}

// almacenesPorTipo filtra los almacenes que sirven a un tipo de ítem.
func (s *Server) almacenesPorTipo(r *http.Request, kind string) []Almacen {
	todos := s.almacenesDe(r)
	filtrados := []Almacen{}
	for _, a := range todos {
		if a.Kind == kind {
			filtrados = append(filtrados, a)
		}
	}
	return filtrados
}

// almacenPorDefecto devuelve el código del almacén por defecto del tipo pedido,
// o el de respaldo si la empresa no tiene ninguno marcado.
func (s *Server) almacenPorDefecto(r *http.Request, kind string) string {
	for _, a := range s.almacenesPorTipo(r, kind) {
		if a.IsDefault {
			return a.Code
		}
	}
	if lista := s.almacenesPorTipo(r, kind); len(lista) > 0 {
		return lista[0].Code
	}
	return almacenPorDefectoPara(kind)
}

// almacenValidoPara comprueba que el código exista y que sea del tipo correcto.
// Es el candado que impide que una compra de limpieza entre al almacén de venta:
// no se "corrige" en silencio, se avisa con los almacenes disponibles.
func (s *Server) almacenValidoPara(r *http.Request, code, kind string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("falta el almacén para un renglón de tipo %s", etiquetaTipoItem(kind))
	}
	todos := s.almacenesDe(r)
	for _, a := range todos {
		if a.Code == code {
			if a.Kind != kind {
				return fmt.Errorf("el almacén %s es de %s y el renglón es de %s: elija uno de %s",
					a.Code, etiquetaTipoAlmacen(a.Kind), etiquetaTipoItem(kind), etiquetaTipoAlmacen(kind))
			}
			return nil
		}
	}
	return fmt.Errorf("el almacén %s no existe o está inactivo", code)
}

// etiquetaTipoAlmacen es el texto del tipo de almacén.
func etiquetaTipoAlmacen(kind string) string {
	if normalizarTipoAlmacen(kind) == AlmacenTipoConsumo {
		return "consumo interno"
	}
	return "venta"
}

// destinoCompra es la decisión contable y física de UN renglón de compra.
type destinoCompra struct {
	TipoItem   string
	MueveStock bool
	Almacen    string // vacío cuando es servicio
	Cuenta     int64  // cuenta que se debita por ese renglón
}

// destinoDeRenglonCompra resuelve el renglón. No toca la base: recibe lo ya
// leído para poder probarse sin servidor.
//
//   - venta:   debita inventario de mercancía, entra al almacén de venta.
//   - consumo: debita inventario de suministros, entra al almacén de consumo.
//   - servicio: debita la cuenta de gasto elegida (o la de gasto de servicios
//     por defecto), no toca almacén ni stock.
func destinoDeRenglonCompra(pre *PredefineAccount, tipoItem, almacen string, cuentaLinea int64) (destinoCompra, error) {
	tipo := normalizarTipoItem(tipoItem)
	d := destinoCompra{TipoItem: tipo, MueveStock: itemMueveStock(tipo)}
	switch tipo {
	case ItemServicio:
		if cuentaLinea > 0 {
			d.Cuenta = cuentaLinea
		} else {
			d.Cuenta = pre.CuentaGastoServicio()
		}
		if d.Cuenta == 0 {
			return d, fmt.Errorf("no hay cuenta de gasto para el servicio: elíjala en el renglón o asígnela en Configuración → Cuentas por defecto (papel «Gasto por servicios recibidos»)")
		}
		d.Almacen = ""
		return d, nil
	case ItemConsumo:
		d.Cuenta = pre.CuentaInventarioSuministros()
		if d.Almacen = strings.TrimSpace(almacen); d.Almacen == "" {
			d.Almacen = AlmacenConsumoPorDefecto
		}
	default:
		d.Cuenta = pre.CuentaInventarioMercancia()
		if d.Almacen = strings.TrimSpace(almacen); d.Almacen == "" {
			d.Almacen = AlmacenVentaPorDefecto
		}
	}
	if d.Cuenta == 0 {
		papel := "Inventario de mercancía para la venta"
		if tipo == ItemConsumo {
			papel = "Inventario de suministros y consumibles"
		}
		return d, fmt.Errorf("no hay cuenta para %s: asígnela en Configuración → Cuentas por defecto (papel «%s»)", etiquetaTipoItem(tipo), papel)
	}
	return d, nil
}

// consultadorStock es lo que necesitan las consultas de stock: sirve tanto una
// conexión como una transacción.
type consultadorStock interface {
	QueryRow(query string, args ...any) *sql.Row
}

// idInternoProducto resuelve el id ENTERO interno de un producto a partir del
// identificador que trae el formulario o el payload: puede ser el código
// (product_information.product_id, varchar) o el id en texto. El kardex
// (stock_movement.product_id) es entero: hacer Atoi al código era la trampa
// que dejaba los ajustes de productos alfanuméricos sin movimiento de stock
// —lo destapó la prueba de reversión D-4, auditoría 2026-11-22, y estaba
// igual en la venta del panel y en el consumo interno—.
func idInternoProducto(q consultadorStock, identificador string) int {
	identificador = strings.TrimSpace(identificador)
	if identificador == "" {
		return 0
	}
	var id int64
	if err := q.QueryRow(`SELECT id FROM product_information WHERE product_id = ?`, identificador).Scan(&id); err == nil && id > 0 {
		return int(id)
	}
	if n, err := strconv.ParseInt(identificador, 10, 64); err == nil {
		return int(n) // catálogos antiguos donde el código ES el número
	}
	return 0
}

// stockPorMovimientos calcula el stock real de un producto.
//
// La fuente es `stock_movement`, que es el libro donde quedan todas las
// entradas y salidas con su almacén. Antes el stock se calculaba como
// "comprado menos facturado": eso ignoraba almacenes, ajustes, devoluciones y
// consumo interno, y por eso el papel de oficina nunca salía del inventario.
//
// La cantidad se guarda siempre en positivo y el sentido lo da el tipo de
// movimiento, así que se suma con signo. Los tipos nuevos (`consumo`,
// `transfer_in`, `transfer_out`) entran en la misma regla.
func stockPorMovimientos(q consultadorStock, productID int, almacen string) float64 {
	if productID == 0 {
		return 0
	}
	sqlTexto := `
		SELECT IFNULL(SUM(CASE
			WHEN movement_type IN ('purchase','adjustment_add','return_customer','transfer_in') THEN quantity
			WHEN movement_type IN ('sale','delivery','adjustment_remove','return_supplier','consumo','transfer_out') THEN -quantity
			ELSE 0 END), 0)
		FROM stock_movement
		WHERE product_id = ? AND IFNULL(is_posted,1) = 1`
	args := []any{productID}
	if strings.TrimSpace(almacen) != "" {
		sqlTexto += ` AND warehouse_id = ?`
		args = append(args, almacen)
	}
	var stock float64
	_ = q.QueryRow(sqlTexto, args...).Scan(&stock)
	return stock
}

// costoPromedioCompra es el costo promedio ponderado de las compras del
// producto. Se usa para valorizar el consumo interno y las salidas cuando el
// movimiento no trae costo propio.
func costoPromedioCompra(q consultadorStock, productID int) float64 {
	var total, qty float64
	_ = q.QueryRow(`
		SELECT IFNULL(SUM(total_amount),0), IFNULL(SUM(quantity),0)
		FROM product_purchase_details
		WHERE product_id = ?`, productID).Scan(&total, &qty)
	if qty <= 0 {
		return 0
	}
	return total / qty
}

// ultimoCostoCompra es el precio del último renglón comprado. Sirve para
// proponer el costo de un consumo cuando no hay promedio (primera compra).
func ultimoCostoCompra(q consultadorStock, productID int) float64 {
	var rate float64
	_ = q.QueryRow(`
		SELECT IFNULL((
			SELECT d.rate FROM product_purchase_details d
			JOIN product_purchase p ON p.purchase_id = d.purchase_id
			WHERE d.product_id = ? AND IFNULL(d.rate,0) > 0
			ORDER BY p.purchase_date DESC, p.id DESC LIMIT 1), 0)`, productID).Scan(&rate)
	return rate
}
