package web

// semilla_despiece.go — El despiece: de la canal salen los cortes.
//
// ESTE PASO ES EL QUE FALTA Y EL QUE HACE QUE LA CARNICERÍA EXISTA.
//
// Una carnicería no vende canales: compra canales, las despieza, y vende cortes.
// Sin este paso, el catálogo de cortes no tiene existencias y no hay una sola
// venta — que es exactamente lo que pasó la primera vez que se corrió el
// semillero: se compraron 250 canales y se vendió nada.
//
// LA ARITMÉTICA, QUE NO ES LA INTUITIVA.
//
// Una canal de 250 kg entra a 10,00 Bs/kg: cuesta 2.500 Bs. De ella salen 180 kg
// que se venden y 70 kg que son hueso, piel y descarte, que no valen nada.
//
// El costo de los 2.500 Bs NO se reparte entre los 250 kg, sino entre los 180 kg
// que realmente se venden: los 70 kg de descarte no van a cargar con nada,
// porque el dueño no los compró para venderlos. El costo por kilo de venta es
//
//	2500 / 180 = 13,89 Bs/kg
//
// Y ese es el MISMO precio para todos los cortes, porque el reparto es por peso:
// un kilo de costilla y un kilo de pechuga de la misma canal cuestan lo mismo.
// Lo que cambia después es el PRECIO al que se venden, que es donde está el
// negocio de cada corte.
//
// POR QUÉ `product_yield.expected_pct` ES 72 Y NO 18.
//
// La tabla tiene UN porcentaje, y aquí se le da el RENDIMIENTO DE VENTA (72%),
// que es lo que usa el motor para repartir el costo. El reparto de masa entre
// cortes (18%, 30%, 20%, 4%) NO va en esa tabla, y es una limitación real del
// esquema: con un solo porcentaje, guardar ahí la cuota de masa haría que el
// motor dividiera 2.500 entre 250×0,18 = 45 kg, y el corte de res saldría a
// 55,56 Bs/kg en vez de 13,89.
//
// La cuota de masa vive en el plan de este fichero, que es de donde sale la
// operación real. Si algún día `product_yield` gana una columna de cuota, esto
// se mueve a la base y este fichero se puede borrar.

import (
	"database/sql"
	"fmt"
)

// repartoCanal es el plan de despiece de una canal: qué sale de ella y cuánto.
//
// `cuota` es el porcentaje de PESO. La suma de las cuotas de venta tiene que
// ser igual al rendimiento; el resto es descarte y se queda en la canal.
var repartoCanal = map[string][]struct {
	producto string
	cuota    float64 // % de la masa de la canal
	nota     string
}{
	// Res: 250 kg entran, 180 kg salen a venta (72%).
	"1007": {
		{"1001", 18, "Res de res: 45 kg por canal de 250 kg"},
		{"1002", 30, "Corte de res: 75 kg por canal de 250 kg"},
		{"1003", 20, "Costilla y falda: 50 kg por canal de 250 kg"},
		{"1006", 4, "Hígado y menudencia roja: 10 kg por canal de 250 kg"},
	},
	// Pollo: 250 kg entran, 180 kg salen a venta (72%).
	"1008": {
		{"1004", 62, "Pollo entero faenado: 155 kg por canal de 250 kg"},
		{"1005", 10, "Pechuga: 25 kg por canal de 250 kg"},
	},
}

// despieceCanal convierte los lotes de una canal en lotes de cortes.
//
// Se hace con la lógica de lotes de verdad (`ConsumirLote` y `RegistrarLote`),
// no con un UPDATE de cantidad: el corte tiene que salir de un lote concreto,
// porque los cortes caducan y el FEFO los tiene que poder distinguir.
//
// El costo unitario del corte sale del costo del lote de la canal y del
// rendimiento, NO de `product_information.cost`: el costo del corte se escribe
// al despiezar, como pasa en el almacén, y es el que luego usa el kardex.
func despieceCanal(s *Server, db *sql.DB, canal, periodo string) error {
	plan, ok := repartoCanal[canal]
	if !ok {
		return fmt.Errorf("no hay plan de despiece para la canal %s", canal)
	}

	// La cuota de venta tiene que cuadrar con el rendimiento del motor, o los
	// dos darían números distintos: el motor repartiría sobre 72% y el almacén
	// produziría otra masa.
	var cuotaVenta float64
	for _, p := range plan {
		cuotaVenta += p.cuota
	}
	if diff := cuotaVenta - rendimientoDespiece; diff > 0.01 || diff < -0.01 {
		return fmt.Errorf("el plan de despiece de la canal %s suma %.2f%% de venta "+
			"pero el motor usa un rendimiento de %.2f%%: los dos darían costos distintos",
			canal, cuotaVenta, rendimientoDespiece)
	}

	// Lotes de la canal que quedan en bodega.
	filas, err := db.Query(`SELECT id, lot_code, quantity_available, unit_cost
		FROM product_lot
		WHERE product_id = ? AND quantity_available > 0
		ORDER BY id`, mustEntero(canal))
	if err != nil {
		return err
	}
	type lote struct {
		id, codigo  string
		cant, costo float64
	}
	var lotes []lote
	for filas.Next() {
		var l lote
		var id int64
		if err := filas.Scan(&id, &l.codigo, &l.cant, &l.costo); err == nil {
			l.id = fmt.Sprint(id)
			lotes = append(lotes, l)
		}
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return err
	}
	if len(lotes) == 0 {
		// No hay canal en bodega: no hay nada que despiezar. No es un error.
		return nil
	}

	for _, l := range lotes {
		var idLote int64
		if _, err := fmt.Sscanf(l.id, "%d", &idLote); err != nil {
			continue
		}

		for _, corte := range plan {
			cantidad := l.cant * corte.cuota / 100
			if cantidad <= 0.001 {
				continue
			}
			// EL COSTO DEL CORTE, que es el número que hace bien todo el módulo.
			//
			// El costo TOTAL de la canal se reparte entre los kilos que salen a
			// venta, no entre los que entraron:
			//
			//	costo del corte = (costoTotalCanal / kgDeVenta) / kgDelCorte
			//
			// que es el mismo valor para todos los cortes de la misma canal.
			kgVenta := l.cant * rendimientoDespiece / 100
			if kgVenta <= 0 {
				continue
			}
			costoUnitario := round4(l.cant * l.costo / kgVenta)

			if _, err := s.RegistrarLote(nil, LotAlta{
				ProductID:   int64(mustEntero(corte.producto)),
				LotCode:     fmt.Sprintf("DESP-%s-%d", l.codigo, mustEntero(corte.producto)),
				EntryDate:   fmt.Sprintf("%s-03", periodo),
				ExpiryDate:  fechaVence(periodo, 5),
				QtyReceived: cantidad,
				UnitCost:    costoUnitario,
				SupplierID:  0,
				PurchaseID:  0,
			}); err != nil {
				return fmt.Errorf("lote del corte %s: %w", corte.producto, err)
			}
		}

		// La canal se consume entera: entra limpia y no se vende.
		if err := s.ConsumirLote(nil, idLote, l.cant); err != nil {
			return fmt.Errorf("consumiendo la canal %s: %w", l.codigo, err)
		}
	}
	return nil
}

// POR QUÉ LA FECHA VIENE POR PARÁMETRO Y NO SE RECONSTRUYE DEL CÓDIGO DEL LOTE.
//
// Porque el código del corte es "DESP-CAN-RES-2026-09-1002": sus últimos siete
// caracteres son "09-1002", no un período. Reconstruir la fecha desde ahí daba
// "09-1002-03", que no es una fecha, se guardaba como NULL, y el FEFO —que solo
// despacha lotes que no han caducado— no encontraba ninguno. El síntoma era
// "stock insuficiente" con 3.000 kg disponibles en la base.
//
// El código del lote es una etiqueta para el usuario; la fecha va en su columna.

// round4 redondea a cuatro decimales, que es la escala de los costos unitarios
// (`decimal(18,4)`). Un costo de 13,8888 se guarda en una columna de cuatro
// decimales: si se redondea antes a dos, el desglose del corte no cuadra con el
// total de la canal y el descuadre aparece en la conciliación.
func round4(v float64) float64 {
	return float64(int64(v*10000+copysign(0.5, v))) / 10000
}

func copysign(m, v float64) float64 {
	if v < 0 {
		return -m
	}
	return m
}
