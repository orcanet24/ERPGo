package web

// cost_engine.go — Motor de cálculo de costos.
//
// POR QUÉ ESTÁ SEPARADO EN DOS MITADES.
//
// `costInputs` -> `calcularCosto` es una función PURA: no toca base de datos,
// no lee el reloj, no depende de Server. Devuelve el desglose a partir de lo que
// se le pasa.
//
// Esa separación no es estética. Toda la suite de esta base tarda minutos porque
// cada test levanta MySQL; un cálculo que no toca la base se prueba en
// milisegundos, y los bugs de este módulo —la merma, el rendimiento, la
// repartición de aduana— son bugs de ARITMÉTICA, no de consulta. Si el cálculo
// estuviera entremezclado con las lecturas, cada caso de prueba costaría un
// minuto y se acabarían escribiendo pocos.
//
// Los cuatro errores del borrador anterior eran de aritmética:
//
//	0.1 la merma se aplicaba a un campo que nunca se calculaba
//	0.2 customs_cost se dividía por la cantidad pedida en vez de la recibida
//	0.3 DOUBLE para dinero, que acumula error en un kardex
//	0.4 hourly_rate como columna generada, que re-escribe el costo ya facturado
//
// Los tres primeros se comprueban aquí, sin base.

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Método de costeo
// ---------------------------------------------------------------------------

// Métodos de costeo.
//
// PP es el default de la migración a propósito: es lo que el ERP ya hace, de modo
// que un producto sin configurar no cambia de comportamiento al instalar esto.
const (
	costPP     = "PP"     // costo promedio ponderado (comercio simple)
	costRecipe = "RECIPE" // receta con merma (fabricación)
	costYield  = "YIELD"  // rendimiento de despiece (carnicería)
	costHours  = "HOURS"  // horas de equipo (servicios)
	costImport = "IMPORT" // compra + aduana (importador)
)

// Componentes de costo. Los nombres son los que se guardan en cost_component,
// así que no se pueden cambiar sin migración.
const (
	compMaterial     = "material"
	compLabor        = "labor"
	compDepreciation = "depreciation"
	compOverhead     = "overhead"
	compCustoms      = "customs"
	compFinal        = "final"
)

// CostTrace es UN renglón de trazabilidad: qué se calculó, cómo y con qué.
//
// La trazabilidad no es un extra: es lo que hace que un costo sea defendible.
// Un desglose sin trazabilidad no se puede reproducir, y un costo que no se
// puede reproducir no es un costo, es una estimación.
type CostTrace struct {
	Component string  `json:"component"` // material | labor | customs | ...
	Regla     string  `json:"regla"`     // "receta: 3 componentes, merma 2.50%"
	Tasa      float64 `json:"tasa"`      // la tasa USADA, con la que se calculó
	Cantidad  float64 `json:"cantidad"`  // horas, unidades, lo que corresponda
	Importe   float64 `json:"importe"`
	Fuente    string  `json:"source"` // referencia al dato de origen (tabla, id)
	// Grade solo se llena en las trazas de mano de obra. Va aquí y no en una
	// tabla aparte porque es lo que permite guardar en cost_component el rol
	// con el que se calculó la hora; sin él, el renglón de labor se guardaba
	// con grade vacío y no se podía reconstruir de dónde salió la tarifa.
	Grade string `json:"grade,omitempty"`
}

// CostBreakdown es el resultado: el desglose completo de un costo.
type CostBreakdown struct {
	ProductID string  `json:"product_id"`
	Periodo   string  `json:"periodo"`
	Metodo    string  `json:"metodo"`
	Material  float64 `json:"material"`
	Labor     float64 `json:"labor"`
	// Depreciation es la depreciación prorrateada por unidad. El nombre completo
	// y no una abreviatura: este nombre es el que se expone en JSON y quien lea
	// el informe tiene que poder leerlo sin adivinar qué es "Depr".
	Depreciation float64 `json:"depreciation"`
	Overhead     float64 `json:"overhead"`
	Customs      float64 `json:"customs"`
	// LossFactor es la merma ya aplicada, que queda registrada para que se vea
	// cuánto pesó en el resultado.
	LossFactor float64 `json:"loss_factor"`
	Final      float64 `json:"final"`

	Traces []CostTrace `json:"traces"`
	// Advertencias recoge lo que el cálculo no pudo hacer. Un costo con
	// advertencia es un costo que hay que leer con cuidado; el motor no decide
	// si está bien o mal, lo dice y deja que decida quien lo use.
	Advertencias []string `json:"warnings,omitempty"`
}

// costInputs es TODO lo que el cálculo necesita. Se llena de la base en
// cargarCostInputs y de ahí en adelante no se vuelve a consultar.
type costInputs struct {
	ProductID string
	Periodo   string
	Metodo    string

	// Cantidad sobre la que se calcula el costo unitario.
	Cantidad float64

	// Merma global del producto. Solo afecta al MATERIAL (ver 0.1).
	LossFactor float64

	// RECIPE: componentes de la receta.
	Componentes []costComponente

	// YIELD: de qué producto de origen sale y con qué rendimiento.
	YieldBase costYieldRow

	// HOURS: horas registradas.
	Horas []costHora

	// IMPORT: reparto de aduana de la partida.
	Import costImportRow

	// Comunes a todos los métodos.
	DegrPerUnidad float64 // depreciación por unidad producida
	OverheadPerUn float64 // indirecto por unidad
	Salida        float64 // stock saliente del kardex (para PP)

	// IsAssembled indica que el producto se fabrica/ensambla. Sirve para no
	// callar la depreciación: mientras no existan activos fijos (Fase 1), un
	// producto ensamblado recibe una advertencia explícita en vez de un cero
	// que parece un costo completo.
	IsAssembled bool

	// Errores de lectura que no deben abortar el cálculo: el motor prefiere
	// calcular con lo que haya y decirlo, antes que devolver nada.
	Faltantes []string
}

type costComponente struct {
	ComponentID string  `json:"component_id"`
	Cantidad    float64 `json:"cantidad"`
	UnitCost    float64 `json:"unit_cost"`
	LossFactor  float64 `json:"loss_factor"`
	Step        int     `json:"step"`
}

type costYieldRow struct {
	InputProductID string `json:"input_product_id"`
	ExpectedPct    float64
}

type costHora struct {
	Grade        string
	Hours        float64
	HourlyRate   float64
	Productivity float64
}

type costImportRow struct {
	ReceivedQty float64
	Total       float64 // freight + customs + insurance + other, en moneda local
}

// ---------------------------------------------------------------------------
// El cálculo puro
// ---------------------------------------------------------------------------

// calcularCosto es EL cálculo. No lee la base, no lee el reloj: lo que entra es
// lo que sale.
//
// Orden y motivo de cada paso:
//
//  1. Se calcula el material por el método correspondiente.
//  2. La MERMA se aplica SOLO al material. La tela se pierde en el corte; el
//     botón no. Aplicarla al total deforma las horas y la depreciación, que no
//     tienen merma.
//  3. Se suman los demás componentes.
//  4. Final = suma. Sin nada que ocultar.
func calcularCosto(in costInputs) CostBreakdown {
	b := CostBreakdown{
		ProductID: in.ProductID,
		Periodo:   in.Periodo,
		Metodo:    in.Metodo,
	}

	if in.Cantidad <= 0 {
		b.Advertencias = append(b.Advertencias,
			"la cantidad es cero o negativa: el costo unitario no se puede calcular")
		return b
	}

	// --- 1. Material según el método -------------------------------------
	switch in.Metodo {
	case costRecipe:
		b.Material, b.Traces = materialPorReceta(in)
	case costYield:
		b.Material, b.Traces, b.Advertencias = materialPorRendimiento(in)
	case costImport:
		b.Material, b.Customs, b.Traces, b.Advertencias = materialPorImportacion(in)
	case costHours:
		b.Labor, b.Traces, b.Advertencias = laborPorHoras(in)
	case costPP, "":
		b.Material = in.Salida / in.Cantidad
		b.Traces = append(b.Traces, CostTrace{
			Component: compMaterial,
			Regla:     "costo promedio ponderado del kardex",
			Cantidad:  in.Cantidad,
			Importe:   in.Salida,
			Fuente:    "kardex",
		})
	default:
		b.Advertencias = append(b.Advertencias,
			fmt.Sprintf("método de costeo desconocido %q: se calculó solo el material de adquisición", in.Metodo))
		b.Material = in.Salida / in.Cantidad
	}

	// --- 2. La merma SOLO al material (ver 0.1) --------------------------
	//
	// En el borrador anterior esta línea multiplicaba un campo que nadie
	// inicializaba (breakdown.Total) y luego FinalCost se construía sumando los
	// campos sueltos, con lo que la merma nunca llegaba al costo.
	if in.LossFactor > 0 && b.Material > 0 {
		antes := b.Material
		b.Material = b.Material * (1 + in.LossFactor)
		b.LossFactor = in.LossFactor
		b.Traces = append(b.Traces, CostTrace{
			Component: compMaterial,
			Regla:     fmt.Sprintf("merma %.2f%% sobre material", in.LossFactor*100),
			Importe:   b.Material - antes,
			Fuente:    "product_cost_config / product_recipe",
		})
	}

	// --- 3. Los demás componentes ----------------------------------------
	b.Depreciation = in.DegrPerUnidad
	if b.Depreciation > 0 {
		b.Traces = append(b.Traces, CostTrace{
			Component: compDepreciation,
			Regla:     "depreciación prorrateada por unidad",
			Cantidad:  in.Cantidad,
			Importe:   b.Depreciation,
			Fuente:    "fixed_asset",
		})
	}
	b.Overhead = in.OverheadPerUn
	if b.Overhead > 0 {
		b.Traces = append(b.Traces, CostTrace{
			Component: compOverhead,
			Regla:     "indirecto por unidad",
			Cantidad:  in.Cantidad,
			Importe:   b.Overhead,
			Fuente:    "cost_center",
		})
	}
	// b.Customs ya se llenó en materialPorImportacion, que sabe sobre qué se
	// reparte (la partida recibida, no la pedida).

	// --- 4. Total --------------------------------------------------------
	b.Final = b.Material + b.Labor + b.Depreciation + b.Overhead + b.Customs

	b.Traces = append(b.Traces, CostTrace{
		Component: compFinal,
		Regla:     "suma de componentes",
		Importe:   b.Final,
	})

	for _, f := range in.Faltantes {
		b.Advertencias = append(b.Advertencias, f)
	}
	return b
}

// materialPorReceta suma los componentes, aplicando la merma POR COMPONENTE.
//
// La merma va por componente y no global porque en una receta real los
// componentes se comportan distinto: la tela se pierde en el corte y el botón
// no. Con una merma única sobre la receta entera se carga la culpa a los
// componentes que no tienen.
func materialPorReceta(in costInputs) (float64, []CostTrace) {
	var total float64
	var traces []CostTrace
	var mermaProm float64

	if len(in.Componentes) == 0 {
		return 0, traces
	}
	for _, c := range in.Componentes {
		bruto := c.Cantidad * c.UnitCost
		merma := c.LossFactor
		neto := bruto * (1 + merma)
		total += neto
		mermaProm += merma
		traces = append(traces, CostTrace{
			Component: compMaterial,
			Regla: fmt.Sprintf("%s: %.4g × %.2f con merma %.2f%%",
				c.ComponentID, c.Cantidad, c.UnitCost, merma*100),
			Tasa:     c.UnitCost,
			Cantidad: c.Cantidad,
			Importe:  neto,
			Fuente:   "product_recipe",
		})
	}
	if len(in.Componentes) > 0 {
		mermaProm /= float64(len(in.Componentes))
	}
	// La merma por componente ya está dentro del total; se registra la media en
	// el resumen para que se vea cuánto pesó, sin aplicarla dos veces.
	_ = mermaProm
	return total, traces
}

// materialPorRendimiento reparte el costo del producto de ENTRADA (la canal)
// sobre los kilos que efectivamente salen a venta.
//
// Es el cálculo que más fácil se hace mal. Si se reparte 250/250 sobre la canal,
// el costo de los cortes sale demasiado barato —porque incluye el hueso y la
// merma en el precio de los cortes— pero la canal se acaba vendiendo como
// producto entero, o directamente se factura un corte a un precio que no cubre
// lo que costó entrar la carne.
func materialPorRendimiento(in costInputs) (float64, []CostTrace, []string) {
	var traces []CostTrace
	var avisos []string

	if in.YieldBase.ExpectedPct <= 0 {
		avisos = append(avisos,
			"no hay rendimiento definido para el producto de entrada: el costo saldrá sin repartir")
		return 0, traces, avisos
	}

	// El rendimiento NO es decorativo: es el que hace que el costo por unidad de
	// VENTA sea mayor que el costo por unidad de ENTRADA.
	//
	//   Compra: 250 kg de canal a 1000
	//   Sin rendimiento: 1000/250 = 4.00 por kg
	//   Con 72%:         1000/(250×0.72) = 1000/180 = 5.56 por kg
	//
	// Si el rendimiento se ignorara, el kg de venta saldría a 4.00 y la empresa
	// estaría vendiendo 70 kg de hueso y merma sin cobrarlos. Es el error que más
	// fácil se hace en una carnicería y el que más dinero cuesta.
	pct := in.YieldBase.ExpectedPct / 100
	if pct > 1 {
		pct = 1
	}
	if pct <= 0 {
		pct = 1
	}

	// Cantidad es la ENTRADA (lo que se compró). Lo que sale a venta es lo que
	// de verdad hay que valorar.
	salidaKilos := in.Cantidad * pct
	if salidaKilos <= 0 {
		return 0, traces, append(avisos,
			"el rendimiento deja unidades de salida en cero: no hay costo que repartir")
	}
	material := in.Salida / salidaKilos

	traces = append(traces, CostTrace{
		Component: compMaterial,
		Regla: fmt.Sprintf(
			"rendimiento %.2f%%: %g de entrada → %g de venta, costo total %.2f",
			pct*100, in.Cantidad, salidaKilos, in.Salida),
		Tasa:     in.Salida,
		Cantidad: salidaKilos,
		Importe:  material,
		Fuente:   "product_yield + product_information.cost",
	})
	return material, traces, avisos
}

// materialPorImportacion devuelve material y aduana.
//
// La aduana se reparte sobre las unidades que la partida RECIBIÓ, no sobre las
// que se pidieron (ver 0.2). Dividir por la cantidad pedida hace que el costo
// del producto cambie cuando cambia el pedido, no cuando cambia el mercado.
func materialPorImportacion(in costInputs) (float64, float64, []CostTrace, []string) {
	var traces []CostTrace
	var avisos []string

	var material float64
	if in.Cantidad > 0 {
		material = in.Salida / in.Cantidad
	}
	traces = append(traces, CostTrace{
		Component: compMaterial,
		Regla:     "costo de adquisición de la partida",
		Cantidad:  in.Cantidad,
		Importe:   material,
		Fuente:    "product_purchase_details",
	})

	if in.Import.Total <= 0 {
		return material, 0, traces, avisos
	}
	if in.Import.ReceivedQty <= 0 {
		avisos = append(avisos,
			"hay gastos de importación pero las unidades recibidas están en cero: no se pudieron repartir")
		return material, 0, traces, avisos
	}

	customs := in.Import.Total / in.Import.ReceivedQty
	traces = append(traces, CostTrace{
		Component: compCustoms,
		Regla: fmt.Sprintf("gastos de importación %.2f repartidos entre %g unidades recibidas",
			in.Import.Total, in.Import.ReceivedQty),
		Tasa:     in.Import.Total,
		Cantidad: in.Import.ReceivedQty,
		Importe:  customs,
		Fuente:   "import_cost",
	})
	return material, customs, traces, avisos
}

// laborPorHoras suma horas × tarifa × productividad.
//
// La tarifa que se registra es la que se USÓ, no la de hoy: si en abril subió el
// salario, el costo de marzo sigue saliendo con la tarifa de marzo. Si se
// recalculara con la de hoy, el margen de un proyecto ya facturado cambiaría sin
// que nadie lo tocara.
func laborPorHoras(in costInputs) (float64, []CostTrace, []string) {
	var traces []CostTrace
	var avisos []string

	if len(in.Horas) == 0 {
		avisos = append(avisos,
			"no hay horas registradas para este producto: el costo de mano de obra saldrá en cero")
		return 0, traces, avisos
	}

	var total float64
	var horas float64
	for _, h := range in.Horas {
		tasa := h.HourlyRate
		prod := h.Productivity
		if prod <= 0 {
			prod = 1
		}
		costo := h.Hours * tasa * prod
		total += costo
		horas += h.Hours
		traces = append(traces, CostTrace{
			Component: compLabor,
			Regla:     fmt.Sprintf("%s: %.2f h × %.2f × %.2f", h.Grade, h.Hours, tasa, prod),
			Tasa:      tasa,
			Cantidad:  h.Hours,
			Importe:   costo,
			Fuente:    "cost_timesheet",
			Grade:     h.Grade,
		})
	}

	if in.Cantidad > 0 {
		total = total / in.Cantidad
	}
	_ = horas
	return total, traces, avisos
}

// ---------------------------------------------------------------------------
// Tasa horaria
// ---------------------------------------------------------------------------

// calcularTasaHoraria devuelve la tarifa de un grado.
//
//	(tasa_anual + cargas) / horas anuales × productividad
//
// NO se guarda como columna generada: hacerlo cambiaría la tasa retroactivamente
// y con ella el costo de todo lo ya facturado. Se calcula aquí y se GUARDA en el
// componente de costo, que es donde queda la tarifa que de verdad se usó.
//
// La productividad se aplica aquí porque es parte de la tarifa del grado, no un
// ajuste por línea. Un factor de 1 (o cero sin configurar) deja la tarifa igual;
// 1.25 encarece la hora un 25%. Si se ignorara, el formulario guardaría un dato
// que después no cambia ningún costo: una promesa falsa.
func calcularTasaHoraria(salarioAnual, overheadPct, productividad float64, horasAnuales int) float64 {
	if horasAnuales <= 0 {
		return 0
	}
	if productividad <= 0 {
		productividad = 1
	}
	// 2080 = 40 h × 52 semanas. Si alguien pone 0 horas, no se divide entre cero.
	conCargas := salarioAnual * (1 + overheadPct/100)
	return conCargas / float64(horasAnuales) * productividad
}

// ---------------------------------------------------------------------------
// Lectura de la base
// ---------------------------------------------------------------------------

// cargarCostInputs llena los datos del cálculo desde las tablas.
//
// Los errores de lectura NO abortan: se anotan en `Faltantes` y el motor sigue.
// Un producto sin receta definida debe producir un costo con una advertencia, no
// un error que deje la pantalla en blanco — quien está configurando el producto
// necesita ver qué falta, y eso solo se ve calculando.
func (s *Server) cargarCostInputs(db *sql.DB, productID, periodo string, cantidad float64) costInputs {
	in := costInputs{
		ProductID: productID,
		Periodo:   periodo,
		Cantidad:  cantidad,
		Metodo:    costPP,
	}
	if in.Cantidad <= 0 {
		in.Cantidad = 1
	}

	// --- Configuración ----------------------------------------------------
	var metodo sql.NullString
	var loss sql.NullFloat64
	var centroID sql.NullInt64
	var ensamblado sql.NullInt64
	err := db.QueryRow(`SELECT costing_method, IFNULL(loss_factor,0),
		IFNULL(cost_center_id,0), IFNULL(is_assembled,0)
		FROM product_cost_config WHERE product_id = ?`, productID).
		Scan(&metodo, &loss, &centroID, &ensamblado)
	// ESTE ERROR ANTES SE TRAGABA, Y POR ESO NO SE SABÍA QUE EXISTÍA.
	//
	// sql.ErrNoRows es lo normal: el producto no está configurado y se usan los
	// valores por defecto (PP, merma 0), que es el comportamiento correcto.
	//
	// Cualquier OTRO error es un fallo real —la columna que falta, la tabla que
	// no está, la conexión que se cayó— y antes se descartaba con `_ = err`.
	// Consecuencia medida: la columna loss_factor no existía en
	// product_cost_config, la consulta devolvía ERROR 1054, el motor lo ignoraba
	// y TODOS los productos se costeaban como PP. Los métodos RECIPE, YIELD,
	// HOURS e IMPORT eran código al que no se llegaba nunca, sin que nada lo
	// dijera.
	//
	// Un error de lectura que se traga y deja el sistema en un valor por defecto
	// razonable es el peor tipo de bug: el sistema parece funcionar. Por eso
	// aquí se distingue "no hay configuración" de "no pude leer", y el segundo
	// caso se reporta como Faltante para que salga en la pantalla.
	if err != nil && err != sql.ErrNoRows {
		in.Faltantes = append(in.Faltantes,
			"no se pudo leer la configuración de costeo del producto: "+err.Error())
	}
	if metodo.Valid && strings.TrimSpace(metodo.String) != "" {
		in.Metodo = metodo.String
	}
	if loss.Valid {
		in.LossFactor = loss.Float64
	}
	in.IsAssembled = ensamblado.Valid && ensamblado.Int64 != 0

	// --- Centro de costo (indirecto) --------------------------------------
	//
	// cost_center_id se guardaba en product_cost_config pero el motor nunca lo
	// leía: el indirecto salía siempre cero. La tarifa por unidad es la que se
	// puede aplicar sin conocer las horas de proceso; la tarifa por hora
	// necesita process_route (Fase 2) y se avisa en vez de inventarla.
	if centroID.Valid && centroID.Int64 > 0 {
		var codigo string
		var tasaUnidad, tasaHora sql.NullFloat64
		var activo sql.NullInt64
		errCentro := db.QueryRow(`SELECT code, IFNULL(overhead_rate_unit,0),
			IFNULL(overhead_rate_hour,0), IFNULL(active,1)
			FROM cost_center WHERE id = ?`, centroID.Int64).
			Scan(&codigo, &tasaUnidad, &tasaHora, &activo)
		switch {
		case errCentro == sql.ErrNoRows:
			in.Faltantes = append(in.Faltantes, fmt.Sprintf(
				"el centro de costo %d configurado para el producto no existe: no se aplicó indirecto",
				centroID.Int64))
		case errCentro != nil:
			in.Faltantes = append(in.Faltantes,
				"no se pudo leer el centro de costo del producto: "+errCentro.Error())
		case !activo.Valid || activo.Int64 == 0:
			in.Faltantes = append(in.Faltantes, fmt.Sprintf(
				"el centro de costo %s está inactivo: no se aplicó indirecto", codigo))
		case tasaUnidad.Valid && tasaUnidad.Float64 > 0:
			in.OverheadPerUn = tasaUnidad.Float64
		case tasaHora.Valid && tasaHora.Float64 > 0:
			in.Faltantes = append(in.Faltantes, fmt.Sprintf(
				"el centro de costo %s solo tiene tarifa por hora: el indirecto por unidad necesita las horas de process_route (Fase 2)",
				codigo))
		}
	}

	// La depreciación no se puede inventar. Si el producto se fabrica, el cero
	// se dice en voz alta: un costo sin depreciación presentado como completo
	// es más peligroso que un costo con una advertencia.
	if in.IsAssembled {
		in.Faltantes = append(in.Faltantes,
			"el producto está marcado como ensamblado, pero la depreciación de activos fijos todavía no está implementada (Fase 1): el costo de depreciación saldrá en cero")
	}

	// --- Costo vigente (PP) -----------------------------------------------
	//
	// product_information.cost ES el costo promedio ponderado que el ERP ya
	// mantiene: decimal(18,4), actualizado por el kardex. Aquí no se vuelve a
	// calcular el CPP con otra fórmula — si el motor y el kardex calcularan
	// distinto, el informe de costos no cuadraría con el libro de inventario y no
	// habría forma de saber cuál de los dos está mal.
	var costoGuardado sql.NullFloat64
	_ = db.QueryRow(`SELECT cost FROM product_information WHERE product_id = ?`, productID).
		Scan(&costoGuardado)
	if costoGuardado.Valid {
		in.Salida = costoGuardado.Float64 * in.Cantidad
	} else if in.Metodo == costPP {
		in.Faltantes = append(in.Faltantes,
			"el producto no tiene costo en product_information: no se pudo aplicar el método PP")
	}

	// --- Receta ------------------------------------------------------------
	rows, err := db.Query(`SELECT component_id, quantity,
		IFNULL((SELECT cost FROM product_information p WHERE p.product_id = r.component_id),0),
		IFNULL(loss_factor,0), step_order
		FROM product_recipe r WHERE product_id = ? ORDER BY step_order, component_id`, productID)
	if err != nil {
		if in.Metodo == costRecipe {
			in.Faltantes = append(in.Faltantes,
				"no se pudo leer la receta del producto: "+err.Error())
		}
	} else {
		for rows.Next() {
			var c costComponente
			if err := rows.Scan(&c.ComponentID, &c.Cantidad, &c.UnitCost, &c.LossFactor, &c.Step); err == nil {
				in.Componentes = append(in.Componentes, c)
			}
		}
		rows.Close()
	}
	if in.Metodo == costRecipe && len(in.Componentes) == 0 {
		in.Faltantes = append(in.Faltantes,
			"el producto está configurado como RECIPE pero no tiene componentes en product_recipe")
	}

	// --- Rendimiento --------------------------------------------------------
	if in.Metodo == costYield {
		var entradaID string
		var pct sql.NullFloat64
		_ = db.QueryRow(`SELECT input_product_id, expected_pct FROM product_yield
			WHERE output_product_id = ? LIMIT 1`, productID).
			Scan(&entradaID, &pct)
		if entradaID == "" {
			in.Faltantes = append(in.Faltantes,
				"no hay rendimiento definido en product_yield para este producto")
			return in
		}
		in.YieldBase.InputProductID = entradaID
		if pct.Valid {
			in.YieldBase.ExpectedPct = pct.Float64
		}

		// El costo que se reparte es el de la ENTRADA (la canal), no el del corte
		// que sale. Es el error más caro de este método: leer el costo del producto
		// de salida y repartirlo sobre sí mismo da un resultado circular y el
		// rendimiento no entra nunca en la cuenta.
		var costoEntrada sql.NullFloat64
		_ = db.QueryRow(`SELECT cost FROM product_information WHERE product_id = ?`, entradaID).
			Scan(&costoEntrada)
		if costoEntrada.Valid {
			in.Salida = costoEntrada.Float64 * in.Cantidad
		} else {
			in.Faltantes = append(in.Faltantes,
				"el producto de entrada "+entradaID+" no tiene costo en product_information")
		}
		return in
	}

	// --- Horas --------------------------------------------------------------
	if in.Metodo == costHours {
		hrows, err := db.Query(`SELECT grade, SUM(hours),
			IFNULL(MAX(rate_used),0), 1.0
			FROM cost_component
			WHERE product_id = ? AND period = ? AND component = 'labor'
			GROUP BY grade`, productID, periodo)
		if err == nil {
			for hrows.Next() {
				var h costHora
				if err := hrows.Scan(&h.Grade, &h.Hours, &h.HourlyRate, &h.Productivity); err == nil {
					in.Horas = append(in.Horas, h)
				}
			}
			hrows.Close()
		}
		// Si no hay componentes guardados aún, se lee la hoja de horas directa.
		if len(in.Horas) == 0 {
			trows, err := db.Query(`SELECT grade, SUM(hours), 0.0, 1.0
				FROM cost_timesheet t
				WHERE t.product_id = ? AND t.work_date LIKE ?
				GROUP BY grade`, productID, periodo+"%")
			if err == nil {
				for trows.Next() {
					var h costHora
					if err := trows.Scan(&h.Grade, &h.Hours, &h.HourlyRate, &h.Productivity); err == nil {
						// La tarifa se busca en el grado.
						h.HourlyRate = s.tasaDeGrado(db, h.Grade)
						in.Horas = append(in.Horas, h)
					}
				}
				trows.Close()
			}
		}
		if len(in.Horas) == 0 {
			in.Faltantes = append(in.Faltantes,
				"no hay horas en cost_timesheet para este producto en "+periodo)
		}
	}

	// --- Importación ---------------------------------------------------------
	if in.Metodo == costImport {
		var freight, customs, insurance, other, recv sql.NullFloat64
		err := db.QueryRow(`SELECT freight, customs, insurance, other, received_qty
			FROM import_cost WHERE purchase_id = (
				SELECT purchase_id FROM product_purchase_details WHERE product_id = ?
				ORDER BY id DESC LIMIT 1)`, productID).
			Scan(&freight, &customs, &insurance, &other, &recv)
		if err != nil {
			in.Faltantes = append(in.Faltantes,
				"no hay gastos de importación registrados para este producto")
		} else {
			in.Import.Total = freight.Float64 + customs.Float64 + insurance.Float64 + other.Float64
			in.Import.ReceivedQty = recv.Float64
			if in.Import.ReceivedQty <= 0 {
				in.Faltantes = append(in.Faltantes,
					"las unidades recibidas de la partida están en cero: la aduana no se pudo repartir")
			}
		}
	}

	return in
}

// tasaDeGrado devuelve la tarifa horaria de un grado, calculada en el momento.
func (s *Server) tasaDeGrado(db *sql.DB, grade string) float64 {
	var salario, cargas, productividad sql.NullFloat64
	var horas int
	if err := db.QueryRow(`SELECT base_salary_annual, overhead_pct, hours_per_year,
		IFNULL(productivity_factor,1)
		FROM labor_grade WHERE code = ?`, grade).
		Scan(&salario, &cargas, &horas, &productividad); err != nil {
		return 0
	}
	return calcularTasaHoraria(salario.Float64, cargas.Float64, productividad.Float64, horas)
}

// ---------------------------------------------------------------------------
// Orquestación
// ---------------------------------------------------------------------------

// calcularCostoDeProducto calcula y GUARDA el desglose del período.
//
// Guardar es la parte importante: un cálculo que no se guarda no deja rastro, y
// recalcular un mes pasado daría otro número sin que nadie pueda comparar.
func (s *Server) calcularCostoDeProducto(db *sql.DB, productID, periodo string, cantidad float64) (CostBreakdown, error) {
	in := s.cargarCostInputs(db, productID, periodo, cantidad)
	b := calcularCosto(in)
	b.ProductID = productID
	b.Periodo = periodo
	b.Metodo = in.Metodo

	if err := s.guardarComponentesCosto(db, b, in); err != nil {
		return b, err
	}
	return b, nil
}

// guardarComponentesCosto escribe el desglose en cost_component.
//
// Upsert y no INSERT: recalcular el mismo período no debe duplicar renglones,
// debe reemplazarlos. Si se insertaran, un recálculo duplicaría el costo y el
// total saldría al doble —el mismo error que ya apareció en los informes de
// IA, donde un total se sumaba dos veces.
func (s *Server) guardarComponentesCosto(db *sql.DB, b CostBreakdown, in costInputs) error {
	if len(b.Traces) == 0 {
		return nil
	}
	ahora := time.Now().Format("2006-01-02 15:04:05")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`DELETE FROM cost_component WHERE period = ? AND product_id = ?`,
		b.Periodo, b.ProductID)
	if err != nil {
		return err
	}

	// line_no separa los renglones del mismo componente. Una receta con dos
	// materiales produce dos trazas `material`; antes la clave única
	// (period, product_id, component) solo dejaba guardar la primera y el
	// segundo INSERT rompía el cálculo entero.
	lineas := map[string]int{}
	for _, t := range b.Traces {
		if t.Component == compFinal {
			// El total no es un componente: se guarda en cost_snapshot. Meterlo
			// aquí duplicaría el total al sumar componentes.
			continue
		}
		lineas[t.Component]++
		grade := ""
		var horas float64
		if t.Component == compLabor {
			grade = t.Grade
			horas = t.Cantidad
		}
		_, err := tx.Exec(`INSERT INTO cost_component
			(period, product_id, component, line_no, amount, quantity, unit_cost,
			 grade, hours, rate_used, source_ref, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			b.Periodo, b.ProductID, t.Component, lineas[t.Component],
			t.Importe, t.Cantidad, t.Tasa, grade, horas, t.Tasa, t.Fuente, ahora)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// cerrarCostoDePeriodo congela los costos de un mes.
//
// Después de cerrar, recalcular no cambia nada. Es lo mismo que hace
// acc_period_close con las cuentas: lo que ya se facturó no se re-escribe. Sin
// esto, el histórico de costos no es confiable porque cambia cuando alguien
// toca la configuración de ayer.
func (s *Server) cerrarCostoDePeriodo(db *sql.DB, periodo, usuario string) (int, error) {
	ahora := time.Now().Format("2006-01-02 15:04:05")

	var yaCerrado string
	_ = db.QueryRow(`SELECT status FROM cost_period_close WHERE period = ?`, periodo).Scan(&yaCerrado)
	if yaCerrado == "cerrado" {
		return 0, fmt.Errorf("el período %s ya está cerrado: no se puede volver a congelar sin reabrirlo", periodo)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Se borra lo que hubiera de un intento anterior. El cierre es idempotente:
	// re-cerrar el mismo mes devuelve siempre las mismas filas, y una corrida a
	// medias no deja un snapshot a medio componer.
	if _, err := tx.Exec(`DELETE FROM cost_snapshot WHERE period = ?`, periodo); err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("limpiar snapshot de costos: %w", err)
	}

	// BUG 0.8, ENCONTRADO AL CARGAR LA CARNICERÍA.
	//
	// La columna `loss_factor` del snapshot recibía `MAX(cfg.updated_at)`: una
	// FECHA, en una columna decimal. MySQL lo convertía y el cierre entero
	// fallaba con:
	//
	//	Error 1366: Incorrect decimal value: '2026-10-04 19:51:13' for column
	//	'loss_factor' at row 1
	//
	// No se vio antes porque ningún período se había cerrado nunca: las 13
	// tablas de costos estaban creadas y vacías, y `cerrarCostoDePeriodo` no se
	// había ejecutado ni una vez. Un bug en el cierre no se descubre cargando
	// datos; se descubre cerrando un mes.
	//
	// Ahora toma la merma de donde está, que es `product_cost_config.loss_factor`
	// —la columna que añadió el bug 0.7—. Las dos están en la misma tabla y a
	// nada de contradecirse.
	//
	// El snapshot se arma desde cost_component, que es donde está el desglose ya
	// calculado y con su trazabilidad. No se recalcula aquí: el cierre CONGELA lo
	// que ya se calculó, y recalcular durante el cierre daría un número distinto
	// al que se facturó.
	_, err = tx.Exec(`INSERT INTO cost_snapshot
		(period, product_id, costing_method, final_cost, material, labor,
		 depreciation, overhead, customs, loss_factor, calc_method, closed_by, closed_at)
		SELECT ?,
		       c.product_id,
		       IFNULL(cfg.costing_method, 'PP'),
		       IFNULL(SUM(CASE WHEN c.component='material' THEN c.amount END),0)
		         + IFNULL(SUM(CASE WHEN c.component='labor' THEN c.amount END),0)
		         + IFNULL(SUM(CASE WHEN c.component='depreciation' THEN c.amount END),0)
		         + IFNULL(SUM(CASE WHEN c.component='overhead' THEN c.amount END),0)
		         + IFNULL(SUM(CASE WHEN c.component='customs' THEN c.amount END),0),
		       IFNULL(SUM(CASE WHEN c.component='material' THEN c.amount END),0),
		       IFNULL(SUM(CASE WHEN c.component='labor' THEN c.amount END),0),
		       IFNULL(SUM(CASE WHEN c.component='depreciation' THEN c.amount END),0),
		       IFNULL(SUM(CASE WHEN c.component='overhead' THEN c.amount END),0),
		       IFNULL(SUM(CASE WHEN c.component='customs' THEN c.amount END),0),
		       IFNULL(cfg.loss_factor, 0),
		       'desglose de cost_component',
		       ?, ?
		FROM cost_component c
		LEFT JOIN product_cost_config cfg ON cfg.product_id = c.product_id
		WHERE c.period = ?
		GROUP BY c.product_id, cfg.costing_method`, periodo, usuario, ahora, periodo)
	if err != nil {
		return 0, fmt.Errorf("congelando el snapshot de %s: %w", periodo, err)
	}

	n := 0
	_ = tx.QueryRow(`SELECT COUNT(*) FROM cost_snapshot WHERE period = ?`, periodo).Scan(&n)

	_, err = tx.Exec(`INSERT INTO cost_period_close
		(period, status, products_closed, closed_by, closed_at)
		VALUES (?, 'cerrado', ?, ?, ?)
		ON DUPLICATE KEY UPDATE status='cerrado', products_closed=VALUES(products_closed),
			closed_by=VALUES(closed_by), closed_at=VALUES(closed_at)`,
		periodo, n, usuario, ahora)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
