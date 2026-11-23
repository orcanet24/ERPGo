package web

// ============================================================================
// ERPGo — Núcleo de localización fiscal venezolana (SENIAT)
// ============================================================================
// Centraliza lo fiscal: validación de RIF, series de documentos fiscales
// (numeración única y consecutiva), número de control fiscal, IGTF y
// parámetros fiscales. Todo handler fiscal pasa por aquí.
// ============================================================================

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var _ = fmt.Sprintf
var _ = strings.TrimSpace

// ----------------------------------------------------------------------------
// RIF — validación con dígito verificador (módulo 11, algoritmo SENIAT)
// ----------------------------------------------------------------------------

// valores de la letra inicial y pesos por posición (algoritmo estándar):
// val = letra + Σ peso[i]·dígito[i], DV = 11 - (val % 11); si DV >= 10 → 0
var rifLetterVal = map[byte]int{'V': 4, 'E': 8, 'J': 12, 'P': 16, 'G': 20}
var rifWeights = [8]int{3, 2, 7, 6, 5, 4, 3, 2}

// ValidateRIF valida un RIF venezolano: letra inicial (J/V/G/E/P), 8 dígitos
// y dígito verificador módulo 11. Acepta puntos y guiones.
// Un RIF vacío devuelve (false, "el RIF es obligatorio").
func ValidateRIF(rif string) (bool, string) {
	rif = strings.ToUpper(strings.TrimSpace(rif))
	rif = strings.NewReplacer(".", "", "-", "", " ", "").Replace(rif)
	if rif == "" {
		return false, "el RIF es obligatorio"
	}
	if len(rif) != 10 {
		return false, "el RIF debe tener 10 caracteres (ej: J123456789)"
	}
	letter := rif[0]
	lv, ok := rifLetterVal[letter]
	if !ok {
		return false, "el RIF debe iniciar con J, V, G, E o P"
	}
	for i := 1; i <= 8; i++ {
		if rif[i] < '0' || rif[i] > '9' {
			return false, "el RIF debe tener 8 dígitos después de la letra"
		}
	}
	if rif[9] < '0' || rif[9] > '9' {
		return false, "el dígito verificador del RIF debe ser un número"
	}

	sum := lv
	for i := 0; i < 8; i++ {
		sum += int(rif[i+1]-'0') * rifWeights[i]
	}
	dv := 11 - (sum % 11)
	if dv >= 10 {
		dv = 0
	}
	if dv != int(rif[9]-'0') {
		return false, "el dígito verificador del RIF no es válido"
	}
	return true, ""
}

// NormalizeRIF limpia separadores y uppercase.
func NormalizeRIF(rif string) string {
	return strings.ToUpper(strings.NewReplacer(".", "", "-", "", " ", "").Replace(strings.TrimSpace(rif)))
}

// ValidateDocumento valida un documento de cliente según el TIPO que el usuario
// declaró en el selector de la pantalla:
//
//   - "rif": RIF completo de 10 con dígito verificador (módulo 11). Es la
//     norma para J/G (jurídicas) y para V/E con firma personal YA asignada.
//   - "cedula": cédula de identidad con letra V/E (o P): SOLO letra + dígitos,
//     SIN verificador y SIN exigir 10 caracteres — una cédula puede tener de
//     3 a 8 dígitos. NO se rellena con ceros: el cero final falso es corrupto
//     para el SENIAT.
//
// J y G nunca pueden ser cédula: la letra manda sobre el selector.
func ValidateDocumento(rif, tipo string) (bool, string) {
	rif = NormalizeRIF(rif)
	if rif == "" {
		return true, "" // el RIF es opcional en clientes contados
	}
	letra := ""
	if len(rif) > 0 {
		letra = string(rif[0])
	}
	if tipo == "" {
		// Sin selector: J/G son RIF; V/E forzosamente RIF completo (comportamiento
		// anterior, estricto).
		tipo = "rif"
	}
	if tipo != "cedula" {
		tipo = "rif"
	}
	// La letra manda: jurídicas siempre RIF completo aunque el selector diga cédula.
	if letra == "J" || letra == "G" {
		tipo = "rif"
	}
	if tipo == "rif" {
		return ValidateRIF(rif)
	}
	// Cédula: letra V/E/P + solo dígitos, largo realista (3 a 8 dígitos).
	if letra != "V" && letra != "E" {
		return false, "una cédula debe iniciar con V o E (si es firma personal, corríjalo a RIF)"
	}
	digitos := rif[1:]
	if len(digitos) < 3 || len(digitos) > 8 {
		return false, "la cédula debe tener de 3 a 8 dígitos sin rellenar ceros"
	}
	for i := 0; i < len(digitos); i++ {
		if digitos[i] < '0' || digitos[i] > '9' {
			return false, "la cédula solo lleva dígitos después de la letra"
		}
	}
	return true, ""
}

// ----------------------------------------------------------------------------
// Parámetros fiscales (fiscal_setting)
// ----------------------------------------------------------------------------

func (s *Server) loadFiscalSettings(r *http.Request) fiscalSettings {
	fs := fiscalSettings{IGTFEnabled: true, IGTFRate: 3.0}
	rows, err := s.dbFor(r).Query(`SELECT ` + "`key`" + `, value FROM fiscal_setting`)
	if err != nil {
		return fs
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		switch k {
		case "igtf_enabled":
			fs.IGTFEnabled = v == "1"
		case "igtf_rate":
			if f, e := strconv.ParseFloat(v, 64); e == nil {
				fs.IGTFRate = f
			}
		case "walkin_rif_required":
			fs.WalkinRIFReq = v == "1"
		case "fiscal_footnote":
			fs.FiscalFootnote = v
		case "cashhea_enabled":
			fs.CashheaEnabled = v == "1"
		}
	}
	return fs
}

type fiscalSettings struct {
	IGTFEnabled    bool
	IGTFRate       float64
	WalkinRIFReq   bool
	FiscalFootnote string
	CashheaEnabled bool
}

// setFiscalSetting escribe un ajuste fiscal en la base de la empresa de esta
// petición. El handle llega desde el handler porque un ajuste fiscal es de una
// empresa concreta: escribirlo en la base de otra sería inventarse un dato
// tributario en el lugar equivocado.
func (s *Server) setFiscalSetting(r *http.Request, key, value string) error {
	return s.upsertFiscalSetting(s.dbFor(r), key, value)
}

// ----------------------------------------------------------------------------
// Series de documentos fiscales — numeración única y consecutiva
// ----------------------------------------------------------------------------

// FiscalDoc es el resultado de consumir una serie fiscal.
type FiscalDoc struct {
	Series    string // FACT, NC, ND, RET-IVA, RET-ISLR
	Number    int64  // número consecutivo
	DocNo     string // número formateado (00000001)
	ControlNo string // número de control fiscal formateado (00-00000001)
}

// nextFiscalDoc consume atómicamente la serie indicada y devuelve
// número + control fiscal. El formato del control es CCPrefijo-NNNNNNNN
// (8 dígitos), práctica común de las imprentas digitales autorizadas.
// NextFiscalDoc is the exported wrapper around (*Server).nextFiscalDoc so that
// cmd/server can inject it into the POS gRPC server without a type cycle.
func (s *Server) NextFiscalDoc(tx *sql.Tx, series string) (FiscalDoc, error) {
	return s.nextFiscalDoc(tx, series)
}

// nextFiscalDoc consume la serie con BLOQUEO de fila.
//
// ANTES: se leía next_no/control_next y después venía un UPDATE sin bloqueo.
// Dos facturas simultáneas leían el mismo next_no y las dos salían con el MISMO
// número de control; el `UPDATE ... WHERE id = ?` no arreglaba nada porque
// escribía el valor que ya se había leído. En Venezuela eso invalida el
// documento y descuadra el libro de ventas, y es de lo primero que mira una
// revisión fiscal.
//
// AHORA: el UPDATE va primero y no lleva valor calculado en Go —incrementa en la
// propia base—, así que toma el bloqueo exclusivo de la fila y lo mantiene hasta
// que la transacción termina. La segunda venta espera a que la primera confirme
// o deshaga y recibe el número siguiente. La lectura posterior ve lo que
// ACABAMOS de escribir (misma transacción, misma conexión), así que el documento
// lleva el valor anterior al incremento.
//
// Se mantiene el auto-creado de la serie (una instalación nueva no tiene filas
// en fiscal_series y la primera factura debe poder emitirse).
func (s *Server) nextFiscalDoc(tx *sql.Tx, series string) (FiscalDoc, error) {
	consumir := func() (int64, error) {
		res, err := tx.Exec(`UPDATE fiscal_series
			SET next_no = next_no + 1, control_next = control_next + 1
			WHERE code = ? AND is_active = 1`, series)
		if err != nil {
			return 0, fmt.Errorf("consumiendo la serie fiscal %s: %w", series, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("comprobando el consumo de la serie %s: %w", series, err)
		}
		return n, nil
	}

	afectadas, err := consumir()
	if err != nil {
		return FiscalDoc{}, err
	}
	if afectadas == 0 {
		// No existe la serie: se crea (numera desde 1) y se vuelve a consumir.
		// Si existía pero estaba inactiva, el INSERT IGNORE no hace nada y el
		// segundo intento vuelve a dar 0: eso se informa, no se factura con una
		// serie apagada.
		if err := s.ensureFiscalSeries(tx, series); err != nil {
			return FiscalDoc{}, err
		}
		afectadas, err = consumir()
		if err != nil {
			return FiscalDoc{}, err
		}
		if afectadas == 0 {
			return FiscalDoc{}, fmt.Errorf("la serie fiscal %s no existe o esta inactiva", series)
		}
	}

	var (
		prefix, ctrlPre  string
		nextNo, ctrlNext int64
	)
	if err := tx.QueryRow(`SELECT prefix, control_prefix, next_no, control_next
		FROM fiscal_series WHERE code = ?`, series).
		Scan(&prefix, &ctrlPre, &nextNo, &ctrlNext); err != nil {
		return FiscalDoc{}, fmt.Errorf("leyendo la serie fiscal %s: %w", series, err)
	}

	// El UPDATE ya incrementó: el documento que se emite AHORA lleva el valor
	// anterior, y el siguiente documento recibirá el actual.
	numero, control := nextNo-1, ctrlNext-1
	return FiscalDoc{
		Series:    series,
		Number:    numero,
		DocNo:     fmt.Sprintf("%s%08d", prefix, numero),
		ControlNo: fmt.Sprintf("%s-%08d", ctrlPre, control),
	}, nil
}

// ----------------------------------------------------------------------------
// Correlativos con reserva bloqueada
// ----------------------------------------------------------------------------
//
// POR QUÉ NO `SELECT MAX(columna)+1`. Un máximo no reserva nada: dos
// transacciones simultáneas leen el mismo valor y las dos se quedan con el
// mismo número. El SELECT no bloquea y la transacción sola no serializa dos
// lecturas. La fila de un contador, en cambio, se bloquea en el UPDATE y el
// bloqueo dura hasta que la transacción termina: la segunda reserva espera y
// recibe el siguiente.

// errContadorAusente: la fila del contador no existe todavía.
var errContadorAusente = errors.New("contador ausente")

// reservarConsecutivoTx toma el siguiente valor del contador `nombre`.
//
// `piso` es el mayor número que YA está emitido en la tabla correspondiente. Si
// el contador viniera por debajo (base con histórico anterior a los contadores),
// se pone al día antes de entregar: nunca se reutiliza un número ya emitido.
func reservarConsecutivoTx(tx *sql.Tx, nombre string, piso int64) (int64, error) {
	valor, err := reservarUnaVez(tx, nombre)
	if errors.Is(err, errContadorAusente) {
		// Camino frío: la fila no está (base migrada antes de que existiera la
		// tabla, o alguien la borró). Se crea y se reserva otra vez. En régimen
		// normal la siembra la hace la migración.
		//
		// INSERT pelado con duplicado tolerado —no INSERT IGNORE, que es
		// exclusiva de MySQL y tiraba esta ruta en SQLite, donde el mismo
		// SQL se prueba (D-10)—: si la fila apareció entre la lectura y la
		// escritura, el error de única es exactamente "ya está", y la
		// segunda reserva lo confirma.
		if _, err := tx.Exec(`INSERT INTO pos_counters (name, value, updated_at) VALUES (?, ?, ?)`, nombre, piso, now()); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "unique") && !strings.Contains(msg, "constraint") {
				return 0, fmt.Errorf("creando el contador %s: %w", nombre, err)
			}
		}
		valor, err = reservarUnaVez(tx, nombre)
	}
	if err != nil {
		return 0, err
	}

	if valor <= piso {
		// El contador va por debajo de lo ya emitido. Se sube al piso y se vuelve
		// a reservar. La comparación va EN el UPDATE para no pisar un contador
		// que otro proceso haya subido mientras tanto.
		if _, err := tx.Exec(`UPDATE pos_counters SET value = ? WHERE name = ? AND value < ?`, piso, nombre, piso); err != nil {
			return 0, fmt.Errorf("poniendo al dia el contador %s: %w", nombre, err)
		}
		valor, err = reservarUnaVez(tx, nombre)
		if err != nil {
			return 0, err
		}
	}
	return valor, nil
}

// reservarUnaVez incrementa el contador y devuelve su valor nuevo.
func reservarUnaVez(tx *sql.Tx, nombre string) (int64, error) {
	res, err := tx.Exec(`UPDATE pos_counters SET value = value + 1, updated_at = ? WHERE name = ?`, now(), nombre)
	if err != nil {
		return 0, fmt.Errorf("reservando el contador %s: %w", nombre, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("comprobando la reserva de %s: %w", nombre, err)
	}
	if n == 0 {
		return 0, errContadorAusente
	}
	var valor int64
	if err := tx.QueryRow(`SELECT value FROM pos_counters WHERE name = ?`, nombre).Scan(&valor); err != nil {
		return 0, fmt.Errorf("leyendo el contador %s: %w", nombre, err)
	}
	return valor, nil
}

// reservarNumeroFacturaTx reserva el número interno de factura.
//
// `invoice.invoice` es el número que ve el cliente y el que enlazan los libros y
// el kardex, así que no puede repetirse ni reutilizarse: sale del contador
// compartido (el mismo que usa el POS) y no de un MAX de la tabla. El POS y la
// venta web comparten contador a propósito: dos series paralelas de numeración
// para la misma columna acabarían chocando.
func (s *Server) reservarNumeroFacturaTx(tx *sql.Tx) (int64, error) {
	var piso int64
	if err := tx.QueryRow(`SELECT IFNULL(MAX(invoice), 999) FROM invoice`).Scan(&piso); err != nil {
		return 0, fmt.Errorf("leyendo el ultimo numero de factura: %w", err)
	}
	return reservarConsecutivoTx(tx, "invoice", piso)
}

// SeedFiscal crea series fiscales base y métodos de pago VE si faltan.
// Se llama al migrar (--install/--migrate/--seed) o en cada arranque sqlite.
func SeedFiscal(db *sql.DB, dialect string) {
	seedFiscalSeries(db, dialect)
	seedPaymentMethodCOA(db)
	fmt.Println("Fiscal: series fiscales verificadas (FACT, NC, ND, RET-IVA, RET-ISLR)")
}

// seedPaymentMethodCOA asigna cuentas contables por defecto a los métodos de
// pago (efectivo→caja, electrónicos→banco, crédito→clientes) leyendo el COA
// predefinido. Idempotente: solo llena vacíos.
func seedPaymentMethodCOA(db *sql.DB) {
	var cash, bank, customer int64
	err := db.QueryRow(`SELECT cashCode, bankCode, customerCode FROM acc_predefine_account ORDER BY id LIMIT 1`).Scan(&cash, &bank, &customer)
	if err != nil {
		return
	}
	mappings := map[string]int64{
		"cash":          cash,
		"punto":         bank,
		"pago_movil":    bank,
		"transferencia": bank,
		"zelle":         bank,
		"cripto":        bank,
		"credito":       customer,
		"cashhea":       0, // Cashea: cuenta se asigna después o usa una por defecto
	}
	for code, coa := range mappings {
		if _, err := db.Exec(`UPDATE payment_method SET coa_id = ? WHERE code = ? AND (coa_id IS NULL OR coa_id = 0)`, coa, code); err != nil {
			log.Printf("fiscal: no se pudo asignar la cuenta contable al método de pago %s: %v", code, err)
		}
	}
}

// seedFiscalSeries crea las series base si la tabla está vacía.
func seedFiscalSeries(db *sql.DB, dialect string) {
	seed := func(code, name string) {
		if _, err := db.Exec(insertIgnorePrefix(dialect)+"fiscal_series (code, name, prefix, next_no, control_prefix, control_next, is_active, created_at) "+
			"VALUES (?,?,?,?,?,?,1,?)", code, name, "", 1, "00", 1, now()); err != nil {
			log.Printf("fiscal: no se pudo crear la serie fiscal %s (%s): %v", code, name, err)
		}
	}
	seed("FACT", "Facturas de venta")
	seed("NC", "Notas de crédito")
	seed("ND", "Notas de débito")
	seed("RET-IVA", "Retenciones de IVA")
	seed("RET-ISLR", "Retenciones de ISLR")
}

// ----------------------------------------------------------------------------
// IGTF — percepción del 3% (o el % configurado) sobre pagos en divisas/cripto
// ----------------------------------------------------------------------------

// IGTFResult detalla cómo se desglosa el IGTF de un cobro multimedios.
type IGTFResult struct {
	Rate         float64 // % aplicado
	ForeignTotal float64 // monto pagado en divisas/cripto
	IGTFAmount   float64 // percepción calculada (redondeada a 2 dec)
}

// computeIGTF suma los pagos en métodos is_foreign y aplica el porcentaje.
func computeIGTF(fs fiscalSettings, payTypes []paymentMethodRow, amounts []float64) IGTFResult {
	res := IGTFResult{Rate: fs.IGTFRate}
	if !fs.IGTFEnabled || fs.IGTFRate <= 0 {
		return res
	}
	for _, a := range amounts {
		res.ForeignTotal += a
	}
	res.IGTFAmount = round2(res.ForeignTotal * fs.IGTFRate / 100.0)
	return res
}

// paymentMethodRow replica una fila de payment_method.
type paymentMethodRow struct {
	ID        int
	Code      string
	Name      string
	IsForeign bool
	COAID     sql.NullInt64
}

func (s *Server) paymentMethodsActive(r *http.Request) []paymentMethodRow {
	rows, err := s.dbFor(r).Query(`SELECT id, code, name, is_foreign, coa_id FROM payment_method WHERE is_active = 1 ORDER BY sort_order, id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []paymentMethodRow{}
	for rows.Next() {
		var m paymentMethodRow
		var foreign int
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &foreign, &m.COAID); err != nil {
			continue
		}
		m.IsForeign = foreign == 1
		out = append(out, m)
	}
	return out
}

// resolveMethodByCode localiza un método activo por código.
func methodByCode(list []paymentMethodRow, code string) *paymentMethodRow {
	for i := range list {
		if list[i].Code == code {
			return &list[i]
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Utilidades de redondeo fiscal
// ----------------------------------------------------------------------------

// round2 redondeo a 2 decimales (mitad arriba), usado en TODOS los montos
// fiscales (IVA por renglón, IGTF, retenciones).
func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100.0
}

// ----------------------------------------------------------------------------
// Fechas fiscales
// ----------------------------------------------------------------------------

// periodBounds devuelve primer y último día del período YYYYMM (o del mes
// actual si viene vacío) — usado por los libros IVA y reportes IGTF.
func periodBounds(period string) (string, string) {
	if len(period) != 6 {
		n := time.Now()
		period = fmt.Sprintf("%04d%02d", n.Year(), int(n.Month()))
	}
	y, _ := strconv.Atoi(period[:4])
	m, _ := strconv.Atoi(period[4:6])
	start := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}
