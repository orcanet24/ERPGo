package web

// ai_cost_accounting.go — El coste de la IA como partida contable.
//
// EL PROBLEMA.
//
// `ai_usage` mide el consumo: tokens, peticiones y coste estimado, por agente y
// por día. Eso sirve para facturarle a un cliente, pero no para la contabilidad de
// la propia empresa. Una IA que consume 40 dólares al mes es un gasto, y un
// gasto que no está en el libro no existe para el SENIAT.
//
// Y el problema de fondo es peor que la falta del asiento: un agente es un
// empleado que trabaja todos los días y no está en la nómina. Su desempeño se
// mide igual (informes que entregó, llamadas que hizo), pero su "salario" no es
// un salario sino lo que consumió. Sin llevar eso a un asiento, el dueño no sabe
// cuánto le cuesta la IA, y lo que no se mide no se controla.
//
// QUÉ HACE ESTE ARCHIVO.
//
// Cierra un período (mes) y mueve el consumo a un voucher contable:
//
//	debe  cuenta de gastos de IA        = coste total del período
//	haber cuenta de bancos / proveedores = coste total del período
//
// El asiento se postea en `acc_vaucher` con el mismo mecanismo que usa el resto
// del ERP (ver `insertVoucherRowTx` y el asiento del conteo cíclico), así que
// entra en los libros normales y no en una tabla paralela que nadie cuadra.
//
// LO QUE NO HACE, Y POR QUÉ.
//
// No cobra. Calcular el coste desde `ai_usage` y decidir a qué precio se factura
// son dos cosas distintas, y la segunda es del operador del servicio de IA, no
// del ERP. Aquí solo se refleja lo consumido.
//
// El tipo de cambio: `ai_usage` guarda dólares porque es como los proveedores
// cobran. El asiento va en la moneda de la empresa, y por eso hay una tasa que
// convertir. Si no hay tasa, NO se postea nada y se dice por qué: un asiento con
// una tasa inventada es peor que un asiento pendiente.

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// aiCostPeriod es el resumen de un período, ya agregado y en moneda local.
type aiCostPeriod struct {
	Periodo     string  // YYYY-MM
	Agentes     int     // agentes distintos que consumieron
	Llamadas    int64   // peticiones totales
	Tokens      int64   // tokens totales
	USD         float64 // coste estimado en dólares
	Tasa        float64 // BCV del período
	Moneda      string  // código de la moneda de la empresa
	Local       float64 // USD * Tasa
	Escriturado bool
	VoucherNo   string
}

// aiCostAccounts son las dos cuentas del asiento.
//
// Se leen de `acc_predefine_account` porque es donde el ERP guarda qué cuenta es
// cuál, y no se inventan aquí: un número de cuenta escrito en el código es un
// número que algún día no corresponde con el plan de cuentas de la empresa.
//
// Si la empresa no ha configurado la de gastos, se usa el código de gastos por
// defecto del catálogo. Es un fallback, y está marcado como tal en el texto del
// asiento para que quien lo lea vea que hubo que suponer.
const (
	aiCostFallbackExpense = 511 // gastos de operación
	aiCostFallbackBank    = 101 // caja y bancos
)

// aiCostPeriodoActual devuelve el mes en curso en formato YYYY-MM.
func aiCostPeriodoActual() string {
	return time.Now().Format("2006-01")
}

// summarizeAICost agrega el consumo de un período.
//
// `tasa` es la de cierre del período. Si es 0, `Local` sale 0 y el llamador
// debe abstain de postear: es preferible un asiento pendiente con el motivo
// escrito que un asiento con una cifra inventada.
func (s *Server) summarizeAICost(sess *SessionData, periodo string, tasa float64) (aiCostPeriod, error) {
	p := aiCostPeriod{Periodo: periodo, Tasa: tasa}

	if periodo == "" {
		periodo = aiCostPeriodoActual()
		p.Periodo = periodo
	}
	// El periodo es YYYY-MM. Se acota con BETWEEN sobre created_day, que ya es
	// una fecha: filtrar por fecha completa es lo que evita que un mes lea el
	// anterior.
	desde := periodo + "-01"
	hasta := p.ultimoDiaDelPeriodo()

	var usd sql.NullFloat64
	err := s.dbForSession(sess).QueryRow(`SELECT COUNT(DISTINCT agent_id), COALESCE(SUM(requests),0),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(estimated_cost_usd),0)
		FROM ai_usage WHERE created_day >= ? AND created_day <= ?`, desde, hasta).
		Scan(&p.Agentes, &p.Llamadas, &p.Tokens, &usd)
	if err != nil {
		return p, fmt.Errorf("leyendo el consumo de IA: %w", err)
	}
	p.USD = usd.Float64
	if tasa > 0 {
		p.Local = p.USD * tasa
	}
	p.Moneda = s.settingsForSession(sess).Currency
	return p, nil
}

// ultimoDiaDelPeriodo devuelve el último día de un mes en YYYY-MM, para poder
// filtrar por BETWEEN sin depender de funciones de fecha del servidor (que
// cambian el resultado según el modo SQL activo).
func (p aiCostPeriod) ultimoDiaDelPeriodo() string {
	t, err := time.Parse("2006-01", p.Periodo)
	if err != nil {
		return p.Periodo + "-31"
	}
	fin := t.AddDate(0, 1, -1)
	return fin.Format("2006-01-02")
}

// postAICost escribe el voucher de un período.
//
// Idempotente por `referenceNo`: si el período ya tiene asiento, no se vuelve a
// postear. Sin eso, darle dos veces al botón de "cerrar mes de la IA" duplica
// el gasto, y un gasto duplicado es de los descuadres más caros de detectar
// porque el número parece plausible.
func (s *Server) postAICost(r *http.Request, p aiCostPeriod, usuario string) (string, error) {
	if p.Tasa <= 0 {
		return "", fmt.Errorf("no hay tasa de cambio para %s: el consumo de IA está en dólares y el asiento va en %s. Registra la tasa del período antes de cerrar el mes",
			p.Periodo, p.Moneda)
	}
	if p.Local <= 0 {
		return "", fmt.Errorf("no hay consumo de IA que contabilizar en %s", p.Periodo)
	}

	referencia := "AICOST-" + p.Periodo
	var yaExiste int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_vaucher WHERE referenceNo = ?`, referencia).Scan(&yaExiste); err == nil && yaExiste > 0 {
		return "", fmt.Errorf("el período %s ya tiene su asiento (%s): no se vuelve a contabilizar", p.Periodo, referencia)
	}

	expenseCode := int64(aiCostFallbackExpense)
	bankCode := int64(aiCostFallbackBank)
	var def struct{ Salary, Cash sql.NullInt64 }
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(salary_code,0), IFNULL(cashCode,0) FROM acc_predefine_account WHERE id = 1`).
		Scan(&def.Salary, &def.Cash)
	if def.Salary.Valid && def.Salary.Int64 > 0 {
		expenseCode = def.Salary.Int64
	}
	if def.Cash.Valid && def.Cash.Int64 > 0 {
		bankCode = def.Cash.Int64
	}

	vno := nextVoucherNo(s.dbFor(r), "JV")
	mont := math.Round(p.Local*100) / 100
	detalle := fmt.Sprintf("Consumo de IA %s: %d agentes, %d llamadas, %d tokens, USD %.2f a tasa %.4f",
		p.Periodo, p.Agentes, p.Llamadas, p.Tokens, p.USD, p.Tasa)
	narracion := "Consumo de inteligencia artificial (agentes IA)"

	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: referencia, VDate: p.ultimoDiaDelPeriodo(),
		COAID: expenseCode, Narration: narracion,
		Comment:  detalle,
		Debit:    mont,
		RevCodde: bankCode,
	}, usuario); err != nil {
		return "", fmt.Errorf("escribiendo el debe del asiento de IA: %w", err)
	}
	if err := s.insertVoucherRowTx(r, tx, VoucherRow{
		VNo: vno, Vtype: "JV", ReferenceNo: referencia, VDate: p.ultimoDiaDelPeriodo(),
		COAID: bankCode, Narration: narracion,
		Comment:  detalle,
		Credit:   mont,
		RevCodde: expenseCode,
	}, usuario); err != nil {
		return "", fmt.Errorf("escribiendo el haber del asiento de IA: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return vno, nil
}

// aiCostRateFor devuelve la tasa de cambio aplicable al período.
//
// Se busca la tasa BCV más reciente que no sea posterior al último día del
// período. Usar "la de hoy" para cerrar el mes anterior daría un asiento distinto
// cada vez que se recalculara, y dos empresas con el mismo consumo tendrian dos
// libros distintos.
//
// Si no hay ninguna, se devuelve 0 y el llamante NO postea.
func (s *Server) aiCostRateFor(sess *SessionData, periodo string) float64 {
	fin := aiCostPeriod{Periodo: periodo}.ultimoDiaDelPeriodo()
	db := s.dbForSession(sess)

	// Cada tabla tiene SU nombre de columna y son distintos. Escribir "rate" y
	// "rate_date" para las dos hacia que AMBAS consultas fallaran siempre con
	// "Unknown column", el error se tragaba en el segundo intento y la función
	// devolvía 0: el mes no se podía cerrar nunca y la pantalla decía "falta la
	// tasa" cuando lo que faltaba era la consulta.
	//
	//   exchange_rate       -> date, rate_bcv        (schema_extra.sql)
	//   exchange_rate_daily -> rate_date, official_rate (schema_z_inventory_advanced.sql)
	//
	// La primera es la del módulo de tipo de cambio; la segunda, la del módulo
	// BCV/paralelo. Se leen las dos porque una empresa puede usar solo una, y la
	// que no usa simplemente no devuelve nada.
	var tasa sql.NullFloat64
	if err := db.QueryRow(`SELECT rate_bcv FROM exchange_rate
		WHERE date <= ? AND currency = 'USD' ORDER BY date DESC LIMIT 1`, fin).Scan(&tasa); err != nil || !tasa.Valid {
		// official_rate es la del BCV. buy_rate y sell_rate son los del banco
		// del cliente, que no sirven para valorar consumo: son tipos de cambio de
		// compra y venta, y usarlos daría un importe distinto del que verá la
		// empresa al comparar con su propia contabilidad.
		_ = db.QueryRow(`SELECT official_rate FROM exchange_rate_daily
			WHERE rate_date <= ? AND currency = 'USD' AND official_rate > 0
			ORDER BY rate_date DESC LIMIT 1`, fin).Scan(&tasa)
	}
	if !tasa.Valid || tasa.Float64 <= 0 {
		return 0
	}
	return tasa.Float64
}

// aiCostStatus dice si un período se puede cerrar y por qué no, si no se puede.
//
// Existe para que la pantalla muestre el motivo real en vez de un botón que no
// hace nada: "no hay tasa" y "ya está contabilizado" son dos problemas distintos
// y el usuario tiene que saber cuál tiene delante.
func (s *Server) aiCostStatus(sess *SessionData, periodo string) (aiCostPeriod, string) {
	if periodo == "" {
		periodo = aiCostPeriodoActual()
	}
	tasa := s.aiCostRateFor(sess, periodo)
	p, err := s.summarizeAICost(sess, periodo, tasa)
	if err != nil {
		return p, "no se pudo leer el consumo: " + err.Error()
	}
	if p.Llamadas == 0 {
		return p, "no hubo consumo de IA en " + periodo
	}
	referencia := "AICOST-" + periodo
	var n int
	_ = s.dbForSession(sess).QueryRow(`SELECT COUNT(*) FROM acc_vaucher WHERE referenceNo = ?`, referencia).Scan(&n)
	if n > 0 {
		p.Escriturado = true
		return p, "ya contabilizado"
	}
	if tasa <= 0 {
		return p, "falta la tasa de cambio de " + periodo + ": el consumo está en dólares"
	}
	p.VoucherNo = "pendiente"
	return p, "listo para contabilizar"
}

// formatAICostPeriod formatea el período para mostrarlo ("octubre de 2026").
// Es una ayuda de pantalla; no lleva a ningún cálculo.
func formatAICostPeriod(periodo string) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo
	}
	meses := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
		"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	return fmt.Sprintf("%s de %d", meses[int(t.Month())-1], t.Year())
}

// aiCostCurrency devuelve el símbolo de la moneda de la empresa, para las
// etiquetas de la pantalla.
func (s *Server) aiCostCurrency(sess *SessionData) string {
	symbols := map[string]string{"USD": "$", "EUR": "€", "VES": "Bs", "BOB": "Bs", "COP": "$", "MXN": "$"}
	c := strings.ToUpper(strings.TrimSpace(s.settingsForSession(sess).Currency))
	if sym, ok := symbols[c]; ok {
		return sym
	}
	return c
}
