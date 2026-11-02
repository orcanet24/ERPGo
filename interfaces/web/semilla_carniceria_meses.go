package web

// semilla_carniceria_meses.go — Los cinco meses de operación, mes a mes.
//
// QUÉ PASA EN CADA MES, Y POR QUÉ EN ESTE ORDEN.
//
//	1. Compra de canales   → handlers de compra, de verdad
//	2. Despiece            → el costo se reparte con product_yield
//	3. Entrada a bodega    → lotes con su unidad de costo
//	4. Venta               → handlers de factura, con FEFO consumiendo lotes
//	5. Nómina              → hr_payroll_run del mes
//	6. Cierre de costos    → cost_snapshot congela el costo del mes
//
// EL ORDEN IMPORTA Y HAY UNA RAZÓN CONCRETA.
//
// El costo de un corte se calcula a partir del costo de la canal que entró
// ANTES. Si se vendiera antes de comprar, el costo del mes saldría del promedio
// del mes anterior, y el margen de diciembre no tendría nada que ver con lo que
// se compró en diciembre. Comprar, después despachar, después vender: ese es el
// orden del almacén real.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// llamarHandler hace una llamada como la haría el navegador.
//
// Se usa para los handlers de verdad (`productSave`, `purchaseSave`,
// `invoiceSave`) en vez de escribir en las tablas a mano. La razón está escrita
// al principio de semilla_carniceria.go y no se repite: un seeder que escribe
// directo deja el libro mayor vacío.
//
// Devuelve el CUERPO cuando hay error, no solo el código: un 400 sin cuerpo no
// dice qué campo se rechazó.
func llamarHandler(s *Server, sesion, ruta string, campos url.Values,
	h func(http.ResponseWriter, *http.Request)) error {
	w := &grabaWriter{}
	r := newFormRequest(s, sesion, campos)
	h(w, r)
	if w.codigo >= 400 {
		return fmt.Errorf("%s devolvió %d: %s", ruta, w.codigo,
			strings.TrimSpace(cuerpoDe(w)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Un mes
// ---------------------------------------------------------------------------

// sembrarMes ejecuta el ciclo completo de un mes.
func sembrarMes(s *Server, db *sql.DB, sesion, periodo string, idx int, inf *InformeSemilla) error {
	canales := canalesPorMes[idx]
	kg := float64(canales) * kiloPorCanal

	// --- 1 y 3. Compra de canales y entrada a bodega ---------------------
	//
	// La compra registra la CABEZA de la factura; los lotes se abren después,
	// como en el almacén real, donde el lote se crea cuando se descarga el
	// camión y no cuando se firma la factura. El sistema tiene que poder
	// distinguir "llegó la mercadería" de "me dijeron que llegó".
	mes := mesDe(periodo)
	dia := 3
	fechaCompra := fmt.Sprintf("%s-%02d", periodo, dia)
	control := fmt.Sprintf("MAT-%s-%03d", periodo, idx+1)

	var totalCompra float64
	renglones := []renglonSemilla{
		{"1007", "CAN-RES-" + periodo, kg, costoPorKilo, fechaVence(periodo, 5)},
		// El pollo entra una vez cada dos meses: es más caro por kilo y la
		// carnicería de barrio lo compra menos.
	}
	if idx%2 == 1 {
		kgPollo := kg * 0.35
		renglones = append(renglones, renglonSemilla{
			"1008", "CAN-POL-" + periodo, kgPollo, costoPorKilo * 0.80, fechaVence(periodo, 4),
		})
		kg += kgPollo
	}
	// El total se SACA de los renglones y no se acumula a mano. Calcularlo dos
	// veces —una al montar y otra al recorrer— es exactamente cómo una compra y
	// su importe dejan de cuadrar sin que nadie lo vea en el resultado.
	for _, r := range renglones {
		totalCompra += r.Cantidad * r.Precio
	}

	campos := url.Values{}
	campos.Set("supplier_id", strconv.FormatInt(idProveedor(db, "Matadero Los Andes C.A."), 10))
	campos.Set("purchase_date", fechaCompra)
	campos.Set("control_no", control)
	campos.Set("chalan_no", control)
	campos.Set("payment_type", "0")
	campos.Set("multipaytype", "0")
	campos.Set("purchase_details", "Compra de canales "+mes)
	for _, r := range renglones {
		campos.Add("product_id", r.Producto)
		campos.Add("product_quantity", strconv.FormatFloat(r.Cantidad, 'f', 2, 64))
		campos.Add("product_rate", fmt.Sprintf("%.2f", r.Precio))
		campos.Add("total_price", fmt.Sprintf("%.2f", r.Cantidad*r.Precio))
		campos.Add("batch_no", r.Lote)
		campos.Add("expiry_date", r.Caduca)
		campos.Add("discountvalue", "0")
		campos.Add("discount_per", "0")
		campos.Add("vatpercent", "0")
		campos.Add("vatvalue", "0")
	}
	campos.Set("grand_total_price", fmt.Sprintf("%.2f", totalCompra))
	campos.Set("paid_amount", "0")
	campos.Set("due_amount", fmt.Sprintf("%.2f", totalCompra))

	if err := llamarHandler(s, sesion, "/purchase/save", campos, s.purchaseSave); err != nil {
		return fmt.Errorf("compra del %s: %w", fechaCompra, err)
	}
	inf.Compras++
	inf.Comprado += totalCompra

	var compraID int64
	if err := db.QueryRow(`SELECT purchase_id FROM product_purchase WHERE control_no = ?`,
		control).Scan(&compraID); err != nil {
		return fmt.Errorf("no se encontró la compra %s: %w", control, err)
	}
	for _, r := range renglones {
		if _, err := s.RegistrarLote(nil, LotAlta{
			ProductID:   int64(mustEntero(r.Producto)),
			LotCode:     r.Lote,
			EntryDate:   fechaCompra,
			ExpiryDate:  r.Caduca,
			QtyReceived: r.Cantidad,
			UnitCost:    r.Precio,
			SupplierID:  idProveedor(db, "Matadero Los Andes C.A."),
			PurchaseID:  compraID,
		}); err != nil {
			return fmt.Errorf("lote %s: %w", r.Lote, err)
		}
		inf.Lotes++
	}

	// --- 2. Despiece: de la canal salen los cortes ------------------------
	//
	// Va DESPUÉS de comprar y ANTES de vender, y no es un detalle de orden: sin
	// despiezar no hay existencias de cortes y no hay venta. Es el paso que
	// faltaba cuando se corrió el semillero por primera vez.
	for _, canal := range []string{"1007", "1008"} {
		if err := despieceCanal(s, db, canal, periodo); err != nil {
			return fmt.Errorf("despiece de la canal %s en %s: %w", canal, periodo, err)
		}
	}

	// --- 4. Ventas ------------------------------------------------------
	//
	// Se vende una fracción del stock disponible de cada corte. La fracción
	// sube con la temporada (diciembre es el mes que más carne se lleva una
	// carnicería de barrio) y baja en enero.
	facturado, err := sembrarVentasDelMes(s, db, sesion, periodo, idx, inf)
	if err != nil {
		return err
	}

	// --- 5. Nómina del mes ----------------------------------------------
	if err = sembrarNominaDelMes(db, periodo, idx); err != nil {
		return fmt.Errorf("nómina de %s: %w", periodo, err)
	}

	inf.Lineas = append(inf.Lineas, fmt.Sprintf(
		"%s  canales:%3d  comprado:%10.2f  facturado:%10.2f  margen:%9.2f",
		periodo, canales, totalCompra, facturado, facturado-totalCompra))
	return nil
}

// renglonSemilla es un renglón de compra del semillero.
//
// Se llama así, y no `renglonCompra`, porque el arnés de pruebas DECLARA un tipo
// con ese nombre en el mismo paquete: `datos_carniceria_test.go` vive en `web`,
// y el semillero también. Declarar los dos rompía el build con "redeclared in
// this block".
//
// Es el precio de que el semillero esté en `web` y no en un paquete propio. Se
// paga porque el semillero NECESITA llamar a funciones no exportadas del paquete
// (purchaseSave, invoiceSave, approveVoucher, RegistrarLote), y copiar ese
// código para evitar un choque de nombres sería mucho peor que un sufijo.
type renglonSemilla struct {
	Producto string
	Lote     string
	Cantidad float64
	Precio   float64
	Caduca   string
}

// sembrarVentasDelMes hace las ventas de mostrador y al mayor del mes.
//
// Cada venta consume lotes con el FEFO de verdad. Si un producto no tiene stock,
// el FEFO falla y el mes se detiene: una venta de mercadería que no existe no es
// una venta, y un seiller que la toleraría estaría montando una empresa de
// fantasía.
func sembrarVentasDelMes(s *Server, db *sql.DB, sesion, periodo string, idx int, inf *InformeSemilla) (float64, error) {
	// Mayoristas: 5 clientes que compran todos los días. Se les vende una vez
	// por semana, lo que da ~20 facturas al mes.
	mayoristas := clientesMayores(db)
	if len(mayoristas) == 0 {
		return 0, fmt.Errorf("no hay clientes al mayor: la lista grande está vacía")
	}

	var facturado float64

	// El precio de venta NO va escrito aquí: se lee de la base. Si el catálogo
	// cambia y estas constantes no, la venta se hace al precio viejo y el margen
	// que sale en el informe no corresponde a lo que realmente se cobró.
	for _, corte := range []struct {
		producto string
		volumen  float64 // kg por venta
	}{
		{"1002", 12}, // corte de res
		{"1001", 8},  // res de res
		{"1003", 10}, // costilla
		{"1004", 14}, // pollo
		{"1005", 7},  // pechuga
		{"1006", 3},  // hígado
	} {
		disponible := stockDisponible(db, corte.producto)
		if disponible <= 0.01 {
			continue
		}
		// Al mayor se lleva el 70% del corte, a precio de lista mayor; al
		// mostrador el resto, a precio de detalle.
		// El chequeo de venta usa la FECHA de la venta y los lotes VIGENTES a
		// ella, la misma verdad que el reparto FEFO aplica después.
		fechaMayor := fechaEnDia(periodo, 4)
		for _, cliente := range mayoristas {
			volumen := corte.volumen
			if idx == 3 {
				volumen *= 1.45 // diciembre
			}
			if idx == 4 {
				volumen *= 0.80 // enero
			}
			if err := comprobarStock(s, corte.producto, fechaMayor, volumen); err != nil {
				// No hay para esta semana: se salta la venta y se sigue. Es
				// real: en una carnicería hay días (y meses) que no alcanza.
				continue
			}
			precio := precioMayor(db, corte.producto)
			if _, err := sembrarVenta(s, db, sesion, cliente, fechaMayor, corte.producto,
				volumen, precio, true); err != nil {
				return 0, err
			}
			facturado += volumen * precio
			inf.Ventas++
		}

		// Mostrador: el resto, a precio de detalle, en dos ventas por semana.
		resto := disponible * 0.22
		if resto < 1 {
			resto = 1
		}
		mostrador := clientesMostrador(db)
		for i, cliente := range mostrador {
			volumen := resto / float64(len(mostrador))
			if i%2 == 0 {
				volumen *= 1.2
			}
			fecha := fechaEnDia(periodo, 1+i%20)
			if err := comprobarStock(s, corte.producto, fecha, volumen); err != nil {
				continue
			}
			precio := precioDetalle(db, corte.producto)
			if _, err := sembrarVenta(s, db, sesion, cliente, fecha, corte.producto,
				volumen, precio, true); err != nil {
				return 0, err
			}
			facturado += volumen * precio
			inf.Ventas++
		}
	}

	inf.Facturado += round2(facturado)
	return round2(facturado), nil
}

// sembrarVenta registra UNA factura y consume sus lotes por FEFO.
func sembrarVenta(s *Server, db *sql.DB, sesion string, cliente int64,
	fecha, producto string, cantidad, precio float64, aCredito bool) (float64, error) {

	importe := round2(cantidad * precio)

	campos := url.Values{}
	campos.Set("customer_id", strconv.FormatInt(cliente, 10))
	campos.Set("invoice_date", fecha)
	// POR QUÉ `sale_type` ES EL QUE DECIDE Y NO `payment_type`.
	//
	// `invoiceSave` lee el TIPO DE VENTA y de ahí se deduce si la factura queda
	// cobrada o por cobrar:
	//
	//	"cash"        -> paid = total,  due = 0   (contado)
	//	"credit_sale" -> paid = 0,      due = total (a crédito)
	//
	// Aquí se mandaba "sale_type=1", que NO es ninguno de esos casos: el switch
	// no entraba en ninguno, así que `paid_amount` y `due_amount` se quedaban
	// en 0 tal como venían del formulario. El resultado era una cartera
	// imposible de ver: 420 facturas por 26.808 Bs y "cobrado 0,00, por cobrar
	// 0,00", porque las facturas no decían a quién había que cobrarle.
	//
	// El plan da 15 días de crédito a los mayoristas, así que van a crédito; el
	// mostrador paga en el momento.
	if aCredito {
		campos.Set("sale_type", "credit_sale")
	} else {
		campos.Set("sale_type", "cash")
	}
	campos.Set("inva_details", "Gracias por su compra")
	campos.Set("warehouse_id", "CAMARA-1")
	campos.Add("product_id", producto)
	campos.Add("product_quantity", fmt.Sprintf("%.2f", cantidad))
	campos.Add("product_rate", fmt.Sprintf("%.2f", precio))
	campos.Add("total_price", fmt.Sprintf("%.2f", importe))
	campos.Add("discount_per", "0")
	campos.Add("discountvalue", "0")
	campos.Add("vatpercent", "0")
	campos.Add("vatvalue", "0")
	campos.Add("serial_no", "")
	campos.Set("grand_total_price", fmt.Sprintf("%.2f", importe))

	if err := llamarHandler(s, sesion, "/invoice/save", campos, s.invoiceSave); err != nil {
		return 0, fmt.Errorf("factura del %s al cliente %d: %w", fecha, cliente, err)
	}

	// FEFO: se consume el lote que caduca antes. Si no hay stock, `comprobarStock`
	// ya lo avisó, así que aquí un error es un fallo de verdad.
	//
	// La vigencia se juzga contra la FECHA DE LA VENTA, no contra el reloj del
	// servidor: siembra meses pasados, y un lote que caducó en octubre no lo está
	// contra una venta del 04 de septiembre.
	vigencia, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return 0, fmt.Errorf("fecha de venta inválida %q: %w", fecha, err)
	}
	reparto, _, err := s.RepartirFEFO(nil, int64(mustEntero(producto)), cantidad, vigencia)
	if err != nil {
		return 0, fmt.Errorf("FEFO del %s del %s: %w", producto, fecha, err)
	}
	for _, a := range reparto {
		if err := s.ConsumirLote(nil, a.LotID, a.Cantidad); err != nil {
			return 0, fmt.Errorf("consumiendo el lote %d: %w", a.LotID, err)
		}
	}
	return importe, nil
}

// ---------------------------------------------------------------------------
// Nómina mensual
// ---------------------------------------------------------------------------

// sembrarNominaDelMes registra la nómina del mes en `hr_payroll_run` y sus
// conceptos en `hr_payroll_item`.
//
// `hr_payroll_item` NO tiene columna de horas (id, run_id, employee_id,
// concept_code, kind, amount, rate_pct, base_amount). Eso es exactamente por lo
// que la carnicería NO usa `cost_timesheet`: sus horas son gasto de operación,
// no costo de producto. La nómina se asienta en la cuenta de sueldos del plan
// (206) y no toca el costo de la carne.
//
// En 2026-11 entra el segundo ayudante de mostrador, según el plan (+25.000).
func sembrarNominaDelMes(db *sql.DB, periodo string, idx int) error {
	desde, hasta := rangoDelMes(periodo)

	empleados := []struct {
		codigo string
		sueldo float64
	}{
		{"CAR-001", 45000},
		{"CAR-002", 25000},
		{"CAR-003", 22000},
		{"CAR-004", 20000},
	}
	if periodo >= "2026-11" {
		empleados = append(empleados, struct {
			codigo string
			sueldo float64
		}{"CAR-005", 25000})
	}

	var bruto float64
	for _, e := range empleados {
		bruto += e.sueldo
	}
	patronal := round2(bruto * cargaPatronal / 100)
	neto := round2(bruto)

	res, err := db.Exec(`INSERT INTO hr_payroll_run
		(period_from, period_to, frequency, status, employee_count,
		 total_gross, total_deductions, total_employer, total_net,
		 tax_rate_version, created_by, created_at)
		VALUES (?,?,1,'aprobado',?,?,0,?,?,'','admin',?)`,
		desde, hasta, len(empleados), bruto, patronal, neto,
		time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("nómina %s: %w", periodo, err)
	}
	runID, _ := res.LastInsertId()

	// Un concepto por empleado: salario y carga patronal.
	for _, e := range empleados {
		var empID int64
		if err := db.QueryRow(`SELECT id FROM hr_employee WHERE code = ?`, e.codigo).
			Scan(&empID); err != nil {
			continue
		}
		if _, err := db.Exec(`INSERT INTO hr_payroll_item
			(run_id, employee_id, concept_code, kind, amount, rate_pct, base_amount)
			VALUES (?,?,'SALARIO','ingreso',?,100,?)`,
			runID, empID, e.sueldo, e.sueldo); err != nil {
			return fmt.Errorf("concepto de %s: %w", e.codigo, err)
		}
		carga := round2(e.sueldo * cargaPatronal / 100)
		if _, err := db.Exec(`INSERT INTO hr_payroll_item
			(run_id, employee_id, concept_code, kind, amount, rate_pct, base_amount)
			VALUES (?,?,'PATRONAL','patronal',?,?,?)`,
			runID, empID, carga, cargaPatronal, e.sueldo); err != nil {
			return fmt.Errorf("carga patronal de %s: %w", e.codigo, err)
		}
	}
	_ = idx
	_ = nominaBase
	return nil
}

// ---------------------------------------------------------------------------
// Cierre de costos y cobranza
// ---------------------------------------------------------------------------

// cerrarCostosCarniceria calcula y congela el costo de cada mes.
//
// Primero CALCULA (que escribe `cost_component` con su trazabilidad) y después
// CIERRA (que congela en `cost_snapshot`). El orden no se puede invertir: cerrar
// congela lo que haya calculado, así que cerrar sin calcular congelaría ceros.
func cerrarCostosCarniceria(s *Server, db *sql.DB, meses []string) error {
	filas, err := db.Query(`SELECT product_id FROM product_information
		WHERE status = 1 AND product_id <> '1007' AND product_id <> '1008'
		ORDER BY product_id`)
	if err != nil {
		return err
	}
	var productos []string
	for filas.Next() {
		var id string
		if err := filas.Scan(&id); err != nil {
			filas.Close()
			return err
		}
		productos = append(productos, id)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return err
	}
	sort.Strings(productos)

	for _, periodo := range meses {
		for _, producto := range productos {
			// La cantidad que se calcula es la VENDIDA del mes: el costo que
			// importa es el del producto que salió, no el de un lote hipotético.
			var cantidad float64
			if err := db.QueryRow(`SELECT COALESCE(SUM(d.quantity),0)
				FROM invoice_details d JOIN invoice i ON i.id = d.invoice_id
				WHERE d.product_id = ? AND i.is_draft='no'
				  AND SUBSTRING(i.date,1,7) = ?`, producto, periodo).Scan(&cantidad); err != nil {
				return err
			}
			if cantidad <= 0 {
				continue
			}
			if _, err := s.calcularCostoDeProducto(db, producto, periodo, cantidad); err != nil {
				return fmt.Errorf("costo de %s en %s: %w", producto, periodo, err)
			}
		}
		if _, err := s.cerrarCostoDePeriodo(db, periodo, "admin"); err != nil {
			return fmt.Errorf("cierre de %s: %w", periodo, err)
		}
	}
	return nil
}

// cobrarCartera cobra la parte de las facturas de mostrador (que se pagan en el
// momento) y deja las de mayoristas con 15 días de crédito, que es lo que dice
// el plan.
//
// Se cobra el 95% de las ventas de mostrador y el 60% de las de mayoristas, que
// deja una cartera viva y realista: una carnicería no tiene el 100% cobrado.
func cobrarCartera(db *sql.DB, inf *InformeSemilla) error {
	filas, err := db.Query(`SELECT i.id, i.due_amount, i.total_amount,
		c.customer_type
		FROM invoice i JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.is_draft = 'no' AND i.due_amount > 0.009
		ORDER BY i.date, i.id`)
	if err != nil {
		return err
	}
	defer filas.Close()

	type fila struct {
		id, tipo int64
		saldo    float64
	}
	var lista []fila
	for filas.Next() {
		var f fila
		// El total no hace falta para cobrar: lo que importa es el SALDO. Se
		// escanea en una variable descartable en vez de en `new(float64)`, que
		// no es una dirección y no compila.
		var totalDescartable float64
		if err := filas.Scan(&f.id, &f.saldo, &totalDescartable, &f.tipo); err != nil {
			continue
		}
		lista = append(lista, f)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return err
	}

	var cobrado float64
	for _, f := range lista {
		fraccion := 0.95 // mostrador
		if f.tipo == 2 {
			fraccion = 0.60 // mayorista: queda cartera viva
		}
		aporte := round2(f.saldo * fraccion)
		if aporte <= 0 {
			continue
		}
		// El `WHERE id = ?` es lo que evita el error que el arnés documenta: un
		// `UPDATE ... WHERE due_amount > 0` pagaría el importe COMPLETO a cada
		// fila, porque el lado derecho se evalúa una sola vez.
		if _, err := db.Exec(`UPDATE invoice
			SET paid_amount = paid_amount + ?, due_amount = due_amount - ?
			WHERE id = ?`, aporte, aporte, f.id); err != nil {
			return fmt.Errorf("cobrando la factura %d: %w", f.id, err)
		}
		cobrado += aporte
	}
	inf.Cobrado = round2(cobrado)
	return nil
}

// ---------------------------------------------------------------------------
// Comprobaciones
// ---------------------------------------------------------------------------

// aprobarAsientosCarniceria mueve los asientos pendientes al libro.
//
// `acc_vaucher` es el borrador y `acc_transaction` es el libro. Sin aprobar, el
// libro sale VACÍO, y eso no es un descuadre: es un libro sin escribir. Un
// informe que mire `acc_transaction` sin este pasowidgetsbx说不出东西来。
func aprobarAsientosCarniceria(s *Server, db *sql.DB) (int, error) {
	filas, err := db.Query(`SELECT DISTINCT VNo FROM acc_vaucher
		WHERE isApproved = 0 ORDER BY VNo`)
	if err != nil {
		return 0, err
	}
	var numeros []string
	for filas.Next() {
		var v string
		if err := filas.Scan(&v); err != nil {
			filas.Close()
			return 0, err
		}
		numeros = append(numeros, v)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return 0, err
	}

	var n int
	for _, v := range numeros {
		// Se llama a `approveVoucher`, que es la vía DEL ERP, y no a un INSERT
		// propio. La razón es concreta: la primera versión copiaba los asientos
		// con columnas inventadas (`year`, `create_by`, `status`) y MySQL
		// respondía `Unknown column 'year' in 'field list'`. Las columnas
		// reales son `fyear`, `Vtype`, `CreateBy`, `IsPosted`... y hay más lógica
		// detrás de aprobarlos: marcar `isApproved`, fechar y copiar a
		// `acc_transaction` en el mismo paso.
		//
		// Aprobar con SQL propio dejaría `acc_vaucher.isApproved = 0` para
		// siempre, y el siguiente approve intentaría volver a copiar el mismo
		// asiento: el libro con duplicados.
		if err := s.approveVoucher(nil, v, "admin"); err != nil {
			return n, fmt.Errorf("aprobar el asiento %s: %w", v, err)
		}
		n++
	}
	return n, nil
}

// medirInformeCarniceria lee los totales finales y calcula el descuadre.
func medirInformeCarniceria(db *sql.DB, inf *InformeSemilla) error {
	var n int
	var total, pagado, porCobrar float64
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_amount),0),
		COALESCE(SUM(paid_amount),0), COALESCE(SUM(due_amount),0)
		FROM invoice WHERE is_draft='no'`).
		Scan(&n, &total, &pagado, &porCobrar); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no hay ninguna factura emitida: el semillero no llegó a vender")
	}
	inf.Facturado = round2(total)
	inf.Cobrado = round2(pagado)
	inf.PorCobrar = round2(porCobrar)

	if err := db.QueryRow(`SELECT COALESCE(SUM(Debit),0), COALESCE(SUM(Credit),0)
		FROM acc_transaction`).Scan(&inf.DebeLibro, &inf.HaberLibro); err != nil {
		return err
	}
	inf.Descuadre = round2(inf.DebeLibro - inf.HaberLibro)

	// UN LIBRO VACÍO CUADRA. Y ESO ES LO PEOR QUE PUEDE DECIR ESTE INFORME.
	//
	// Con 0 en el debe y 0 en el haber, la diferencia es 0 y el comando
	// anunciaba "el libro mayor cuadra" sobre una base con 840 asientos TODOS a
	// 0,00. Es el peor tipo de comprobación: la que pasa cuando no pasa nada.
	//
	// Por eso el comando no mira solo el descuadre: mira que HAYA importe. Si
	// el debe y el haber son ambos cero, no es que cuadre: es que no hay libro.
	if inf.DebeLibro <= 0 && inf.HaberLibro <= 0 {
		inf.Librovacio = true
	}

	_ = db.QueryRow(`SELECT COUNT(*) FROM cost_period_close WHERE status='cerrado'`).
		Scan(&inf.PeriodosCerrados)
	_ = db.QueryRow(`SELECT COUNT(DISTINCT product_id) FROM cost_snapshot`).Scan(&inf.CostosCongelados)
	return nil
}
