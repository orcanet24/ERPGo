package web

// period_close.go — Cierre de período contable.
//
// Cerrar un mes no es marcar una casilla: es la afirmación de que ese mes ya no
// se toca. A partir del cierre, todo lo que entra en ese período tiene que pasar
// por un asiento de ajuste con fecha del período, y eso obliga a que el contador
// sea consciente del cambio en vez de corregir la factura original a escondidas.
//
// El módulo se apoya en validaciones que BLOQUEAN en vez de advertir. La
// diferencia importa: un aviso que se puede ignorar en un ERP se ignora, y un
// mes con el IVA descuadrado se descubre en la declaración, cuando ya no se
// corrigió a tiempo.
//
// Principio de reversión: reabrir un período cerrado es posible, pero deja
// rastro. Nunca se borra un cierre: se marca como reabierto y por quién, porque
// la pregunta "¿alguien volvió a tocar un mes ya firmado?" tiene que tener
// respuesta, y en un sistema contable esa pregunta siempre se hace tarde.

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// "time" se usa para el mes en curso cuando la fecha del documento no se puede
// leer (ver periodoDeFecha).

// PeriodStatus es el estado de un período (YYYY-MM) de una empresa.
type PeriodStatus struct {
	Period      string // YYYY-MM
	Status      string // open | closed | reopened
	ClosedAt    string
	ClosedBy    string
	ReopenedAt  string
	ReopenedBy  string
	Vouchers    int
	TotalDebit  float64
	TotalCredit float64
}

// PeriodValidation es el resultado de una comprobación sobre un período.
//
// El diseño separa "el período NO cuadra" de "el período cuadra pero hay avisos":
// Bloqueante impide cerrar, Aviso no impide cerrar pero queda registrado. Un
// sistema que bloquea por todo se rodea en dos semanas; uno que no avisa de nada
// no sirve de nada.
type PeriodValidation struct {
	Code     string // identificador estable de la regla
	Label    string // qué se comprobó
	Problem  string // qué está mal (vacío si no hay problema)
	Blocking bool
}

// PeriodCheckResult agrupa las validaciones con las cifras del período.
type PeriodCheckResult struct {
	Period      string
	Status      string
	Validations []PeriodValidation
	Blocking    int
	Warnings    int
	CanClose    bool
	Summary     map[string]interface{}
}

// ---------- Consultas del período ----------

// normalizePeriod valida el formato "AAAAMM" que usa el resto del sistema
// (fiscal.go) y devuelve el período ya normalizado.
//
// Se valida aquí y NO se asume que periodBounds lo hizo: periodBounds, por
// compatibilidad con el código heredado, SUSTITUYE un período inválido por el mes
// actual en lugar de fallar. Para un cierre de período eso sería peligroso:
// escribiría el cierre en un mes que nadie pidió. Aquí sí se rechaza.
func normalizePeriod(period string) error {
	p := strings.TrimSpace(period)
	if len(p) != 6 {
		return fmt.Errorf("el período debe tener el formato AAAAMM")
	}
	year, err := strconv.Atoi(p[:4])
	if err != nil {
		return fmt.Errorf("año inválido en el período")
	}
	month, err := strconv.Atoi(p[4:])
	if err != nil || month < 1 || month > 12 {
		return fmt.Errorf("mes inválido en el período")
	}
	// Rango razonable: ni el año 3000 ni el 1900. Un año absurdo casi siempre es
	// un typo, y aceptarlo crearía un período fantasma en el histórico.
	if year < 2000 || year > 2100 {
		return fmt.Errorf("año fuera de rango razonable: %d", year)
	}
	return nil
}

// PeriodSummary trae las cifras agregadas del período.
func (s *Server) PeriodSummary(r *http.Request, period string) (map[string]interface{}, error) {
	if err := normalizePeriod(period); err != nil {
		return nil, err
	}
	from, to := periodBounds(period)
	out := map[string]interface{}{}
	var (
		debit  sql.NullFloat64
		credit sql.NullFloat64
	)
	// La verdad del balance es el LIBRO (acc_transaction), no acc_vaucher.
	// Un asiento automático se guarda en acc_vaucher como patas sueltas con
	// su contra-cuenta DECLARADA (RevCodde) y solo al aprobarse se
	// materializa en el libro como débito + crédito (accounting.go:147-173).
	// Sumar Debit−Credit sobre acc_vaucher comparaba dos modelos distintos y
	// encontraba descuadre en cuanto existía UN asiento automático: ningún
	// mes con facturas podía cerrarse (F-1, auditoría 2026-11-22). En el
	// libro, en cambio, un descuadre SI significa escritura partida.
	//
	// IsPosted es char(10) heredado y el propio sistema lo escribe de dos
	// formas ('1' al aprobar, 'yes' desde el POS): la condición correcta es
	// EXCLUIR lo marcado como no publicado, no exigir un literal. Filtrar
	// por '=1' habría dejado las ventas POS fuera del cuadre.
	if err := s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0)
		FROM acc_transaction WHERE VDate >= ? AND VDate <= ? AND IFNULL(IsPosted,'yes') NOT IN ('no','0','')`, from, to).
		Scan(&debit, &credit); err != nil {
		return nil, err
	}
	// Movimientos del período: los asientos del panel (acc_vaucher) MÁS las
	// ventas POS que publican el libro directamente sin fila en acc_vaucher
	// (checkout, gRPC y ahora registrarVentaTerminal). Contar solo
	// acc_vaucher hacía que un mes de caja pura se viera «sin asientos» y no
	// pudiera cerrarse. UNION (no suma): el mismo VNo publicado aparece en
	// las dos tablas y contarlo doble inflaría el resumen.
	var n int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM (
		SELECT VNo FROM acc_vaucher WHERE VDate >= ? AND VDate <= ?
		UNION
		SELECT VNo FROM acc_transaction WHERE VDate >= ? AND VDate <= ? AND IFNULL(IsPosted,'yes') NOT IN ('no','0','')
		) movimientos`, from, to, from, to).Scan(&n); err != nil {
		return nil, err
	}
	// Pendientes de aprobar: no son descuadre, son trabajo sin publicar. Se
	// muestran aparte porque el cierre los exige cerrados (validación 2b).
	var pendientes int
	if err := s.dbFor(r).QueryRow(`SELECT COUNT(DISTINCT VNo)
		FROM acc_vaucher WHERE VDate >= ? AND VDate <= ? AND IFNULL(isApproved,0) = 0`, from, to).Scan(&pendientes); err != nil {
		return nil, err
	}
	out["vouchers"] = n
	out["pendientes"] = pendientes
	out["total_debit"] = round2(debit.Float64)
	out["total_credit"] = round2(credit.Float64)
	out["difference"] = round2(debit.Float64 - credit.Float64)

	// Facturas y retenciones del período: son las cifras que van a la
	// declaración, y conviene mostrarlas aunque el cierre no se apoye en ellas.
	var invoices, purchases float64
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(total_amount),0) FROM invoice
		WHERE date >= ? AND date <= ? AND IFNULL(is_draft,'no') = 'no'`, from, to).Scan(&invoices)
	// La tabla es product_purchase (no `purchase`), la fecha va en purchase_date
	// (varchar) y el total en grand_total_amount; `is_draft` no existe aquí, el
	// estado es `status=1`. Antes esta consulta fallaba en silencio y el cierre
	// mostraba compras en cero.
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(grand_total_amount),0) FROM product_purchase
		WHERE purchase_date >= ? AND purchase_date <= ? AND status = 1`, from, to).Scan(&purchases)
	out["sales"] = round2(invoices)
	out["purchases"] = round2(purchases)
	return out, nil
}

// ---------- Validaciones ----------

// CheckPeriod ejecuta todas las validaciones de cierre sobre un período.
//
// El orden va de lo más grave a lo menos: si el balance no cuadra, los avisos
// menores son ruido y no tiene sentido listarlos primero.
func (s *Server) CheckPeriod(r *http.Request, period string) (*PeriodCheckResult, error) {
	if err := normalizePeriod(period); err != nil {
		return nil, err
	}
	from, to := periodBounds(period)

	summary, err := s.PeriodSummary(r, period)
	if err != nil {
		return nil, err
	}

	res := &PeriodCheckResult{Period: period, Summary: summary}
	res.Status, _ = s.periodStatus(r, period)

	add := func(code, label, problem string, blocking bool) {
		v := PeriodValidation{Code: code, Label: label, Problem: problem, Blocking: blocking}
		if problem != "" {
			if blocking {
				res.Blocking++
			} else {
				res.Warnings++
			}
		}
		res.Validations = append(res.Validations, v)
	}

	// 1. El balance global debe cuadrar. Es la validación madre: si esto falla,
	// todo lo demás es ruido.
	diff, _ := summary["difference"].(float64)
	if abs64(diff) > 0.01 {
		add("balance", "Suma de débitos = suma de créditos",
			fmt.Sprintf("diferencia de %.2f en el período", diff), true)
	} else {
		add("balance", "Suma de débitos = suma de créditos", "", true)
	}

	// 2. Ningún voucher individual descuadrado. Un asiento descuadrado se
	// compensa solo con otro error, y entonces el error real queda escondido.
	if problem := s.checkVouchersBalance(r, from, to); problem != "" {
		add("voucher_balance", "Cada voucher cuadra por sí solo", problem, true)
	} else {
		add("voucher_balance", "Cada voucher cuadra por sí solo", "", true)
	}

	// 2b. No cerrar con asientos pendientes de aprobar: su contra no está
	// todavía en el libro (la pata doble se materializa al aprobar), así que
	// cerrar ahora firmaría un libro incompleto que cambia al aprobar
	// después. Probar que un asiento pendiente de mes firmado podía
	// publicarse después y mover el libro de un mes cerrado fue el origen de
	// exigirPeriodoAbiertoTx en la aprobación; esta es la otra mitad: antes
	// de firmar, nada pendiente.
	if p, _ := summary["pendientes"].(int); p > 0 {
		add("pendientes", "No hay asientos pendientes de aprobar",
			fmt.Sprintf("%d asiento(s) pendiente(s): apruébelos para que pasen al libro antes de cerrar", p), true)
	} else {
		add("pendientes", "No hay asientos pendientes de aprobar", "", true)
	}

	// 3. Un período vacío no se cierra: probablemente es un mes que aún no se
	// usó, y cerrarlo deja un hueco que después nadie sabría explicar.
	if n, _ := summary["vouchers"].(int); n == 0 {
		add("vacio", "El período tiene movimientos", "el período no tiene ningún asiento", true)
	} else {
		add("vacio", "El período tiene movimientos", "", true)
	}

	// 4. Las facturas del período deben tener su asiento contable.
	if problem := s.checkInvoicesPosted(r, from, to); problem != "" {
		add("invoice_asiento", "Cada factura publicada tiene su asiento", problem, true)
	} else {
		add("invoice_asiento", "Cada factura publicada tiene su asiento", "", true)
	}

	// 5. Aviso: cuentas contables sin identificar.
	if problem := s.checkUnidentifiedAccounts(r, from, to); problem != "" {
		add("cuentas_sin_identificar", "Las cuentas usadas tienen descripción", problem, false)
	} else {
		add("cuentas_sin_identificar", "Las cuentas usadas tienen descripción", "", false)
	}

	// 6. Aviso: retenciones pendientes de enterar. Bloquear aquí sería
	// incorrecto: las retenciones se enteran al mes siguiente por ley, así que
	// su presencia en el mes es normal y esperado.
	if problem := s.checkPendingRetentions(r, from, to); problem != "" {
		add("retenciones", "Retenciones IVA/ISLR del período", problem, false)
	} else {
		add("retenciones", "Retenciones IVA/ISLR del período", "", false)
	}

	// 7. Un período ya cerrado no vuelve a cerrarse.
	if res.Status == "closed" {
		add("ya_cerrado", "El período está abierto", "el período ya fue cerrado", true)
	} else {
		add("ya_cerrado", "El período está abierto", "", true)
	}

	res.CanClose = res.Blocking == 0
	return res, nil
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// checkVouchersBalance busca vouchers con filas mal formadas.
//
// EL MODELO REAL, DECLARADO (F-1, auditoría): en acc_vaucher un asiento
// automático vive como PATAS SUELTAS — cada fila lleva Debit o Credit, y su
// contra-cuenta está DECLARADA en RevCodde; al aprobarse, accounting.go
// escribe en el libro las dos mitades. Por eso la versión antigua de esta
// comprobación (SUM(Debit)−SUM(Credit) por VNo) estaba siempre en rojo:
// comparaba filas de pata única contra una regla de doble entrada y declaraba
// descuadrado cualquier asiento automático. Lo que SÍ es una fila mal formada
// en este modelo:
//   - débito y crédito a la vez DESIGUALES en la misma fila (mitad pata,
//     mitad asiento: nadie lo produce intencionadamente), o
//   - fila en cero (asiento vacío ensuciando el libro).
// Las filas manuales equilibradas (d == c > 0, estilo account.go:391) y las
// pata únicas con contra declarada son las dos formas legítimas.
func (s *Server) checkVouchersBalance(r *http.Request, from, to string) string {
	rows, err := s.dbFor(r).Query(`
		SELECT VNo,
			SUM(CASE WHEN Debit > 0 AND Credit > 0 AND ABS(Debit - Credit) > 0.005 THEN 1 ELSE 0 END) AS mixtas,
			SUM(CASE WHEN Debit = 0 AND Credit = 0 THEN 1 ELSE 0 END) AS vacias
		FROM acc_vaucher
		WHERE VDate >= ? AND VDate <= ?
		GROUP BY VNo
		HAVING mixtas > 0 OR vacias > 0
		LIMIT 10`, from, to)
	if err != nil {
		return fmt.Sprintf("no se pudo verificar: %v", err)
	}
	defer rows.Close()
	var malas []string
	for rows.Next() {
		var vno string
		var mixtas, vacias int
		if rows.Scan(&vno, &mixtas, &vacias) == nil {
			malas = append(malas, fmt.Sprintf("%s (%d fila(s) con débitos/créditos desiguales, %d vacía(s))", vno, mixtas, vacias))
		}
	}
	if len(malas) == 0 {
		return ""
	}
	return "asientos con filas mal formadas: " + strings.Join(malas, "; ")
}

// checkInvoicesPosted detecta facturas publicadas sin asiento.
//
// Se consulta el LIBRO (acc_transaction), no acc_vaucher, y por dos razones
// (F-1/F-2, auditoría): los caminos POS (checkout en línea y ahora
// registrarVentaTerminal) publican su doble entrada directamente en el libro
// sin pasar por acc_vaucher —con la comprobación vieja esas ventas parecían
// «sin asiento» y bloqueaban el cierre de otro modo—, y el asiento pendiente
// lo cubre aparte la validación 'pendientes'. La fecha se compara normalizada
// porque invoice.date es varchar heredado y acc_transaction.VDate es DATE.
func (s *Server) checkInvoicesPosted(r *http.Request, from, to string) string {
	var n int
	// Una factura de crédito va con signo negativo y no genera asiento propio;
	// se excluye para no reportar como faltante algo que es correcto.
	err := s.dbFor(r).QueryRow(`
		SELECT COUNT(*) FROM invoice i
		WHERE i.date >= ? AND i.date <= ?
		  AND IFNULL(i.is_draft,'no') = 'no'
		  AND IFNULL(i.is_credit,0) = 0
		  AND NOT EXISTS (
			SELECT 1 FROM acc_transaction t
			WHERE CAST(t.VDate AS CHAR) >= ? AND CAST(t.VDate AS CHAR) <= ?
			  AND t.referenceNo = CAST(i.invoice AS CHAR)
		  )`, from, to, from, to).Scan(&n)
	if err != nil {
		return fmt.Sprintf("no se pudo verificar: %v", err)
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d factura(s) publicada(s) sin asiento contable", n)
}

// checkUnidentifiedAccounts avisa de cuentas que quedaron sin clasificar.
func (s *Server) checkUnidentifiedAccounts(r *http.Request, from, to string) string {
	rows, err := s.dbFor(r).Query(`
		SELECT DISTINCT v.COAID, IFNULL(c.HeadName,'')
		FROM acc_vaucher v
		LEFT JOIN acc_coa c ON c.HeadCode = v.COAID
		WHERE v.VDate >= ? AND v.VDate <= ?
		  AND (c.HeadCode IS NULL OR TRIM(IFNULL(c.HeadName,'')) = '')
		LIMIT 10`, from, to)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var huerfanas []string
	for rows.Next() {
		var coaid int64
		var name string
		if rows.Scan(&coaid, &name) == nil {
			if name == "" {
				name = "(sin nombre)"
			}
			huerfanas = append(huerfanas, fmt.Sprintf("%d %s", coaid, name))
		}
	}
	if len(huerfanas) == 0 {
		return ""
	}
	return fmt.Sprintf("cuentas sin identificar en asientos: %s", strings.Join(huerfanas, ", "))
}

// checkPendingRetentions informa de retenciones generadas y no enteradas.
func (s *Server) checkPendingRetentions(r *http.Request, from, to string) string {
	var n sql.NullInt64
	err := s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM invoice
		WHERE date >= ? AND date <= ?
		  AND IFNULL(is_draft,'no') = 'no'
		  AND IFNULL(total_tax,0) > 0
		  AND IFNULL(payment_type,1) = 3`, from, to).Scan(&n)
	if err != nil || !n.Valid || n.Int64 == 0 {
		return ""
	}
	return fmt.Sprintf("%d factura(s) con IVA retenido pendiente de.enterar", n.Int64)
}

// ---------- Cierre y reapertura ----------

// periodStatus devuelve el estado actual de un período.
func (s *Server) periodStatus(r *http.Request, period string) (string, error) {
	var status sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT status FROM acc_period_close WHERE period = ?`, period).Scan(&status)
	if err == sql.ErrNoRows || !status.Valid {
		return "open", nil
	}
	if err != nil {
		return "", err
	}
	return status.String, nil
}

// ClosePeriod cierra un período tras validar que nada bloquea.
//
// La validación se corre DENTRO de la transacción, no antes: entre la
// comprobación y la escritura otra sesión podría haber insertado un asiento
// descuadrado, y cerrar sobre una base que cambió sería cerrar un período que
// nadie revisó.
func (s *Server) ClosePeriod(r *http.Request, period, user string) error {
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := normalizePeriod(period); err != nil {
		return err
	}
	from, to := periodBounds(period)

	// Se revalida contra la misma conexión que va a escribir.
	//
	// El estado del período se comprueba AQUÍ, dentro de la transacción, y no
	// antes. Comprobarlo antes deja una ventana: si alguien cierra el mismo mes
	// entre la validación y esta escritura, el segundo cierre sobrescribe la
	// fecha y el responsable del primero, y el histórico miente sobre quién
	// firmo qué.
	var status, closedBy sql.NullString
	err = tx.QueryRow(`SELECT status, IFNULL(closed_by,'') FROM acc_period_close WHERE period = ?`, period).
		Scan(&status, &closedBy)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && status.Valid {
		switch status.String {
		case "closed":
			quien := closedBy.String
			if quien == "" {
				quien = "otro usuario"
			}
			return fmt.Errorf("el período %s ya fue cerrado por %s", period, quien)
		case "reopened":
			return fmt.Errorf("el período %s está reabierto; ciérrelo de nuevo para confirmar los cambios", period)
		}
	}

	// El cuadre se comprueba contra el LIBRO publicado, no contra las patas
	// sueltas de acc_vaucher (F-1: sumar Debit−Credit allí findía siempre
	// cualquier mes con facturas, porque los asientos automáticos se guardan
	// como pata única con RevCodde y la doble entrada se materializa en el
	// libro al aprobar). Dentro de la transacción, además, el libro no puede
	// tener pendientes sin publicar: se prohíben explícitamente.
	// (IsPosted es char(10): el panel publica con '1' y el POS con 'yes'; se
	// excluye lo no publicado, no se exige un literal —ver PeriodSummary—.)
	var dif float64
	if err := tx.QueryRow(`SELECT IFNULL(SUM(Debit),0) - IFNULL(SUM(Credit),0)
		FROM acc_transaction WHERE VDate >= ? AND VDate <= ? AND IFNULL(IsPosted,'yes') NOT IN ('no','0','')`, from, to).Scan(&dif); err != nil {
		return err
	}
	if abs64(round2(dif)) > 0.01 {
		return fmt.Errorf("el libro del período no cuadra: diferencia de %.2f", dif)
	}
	var pendientes int
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT VNo) FROM acc_vaucher
		WHERE VDate >= ? AND VDate <= ? AND IFNULL(isApproved,0) = 0`, from, to).Scan(&pendientes); err != nil {
		return err
	}
	if pendientes > 0 {
		return fmt.Errorf("no se puede cerrar con %d asiento(s) pendiente(s) de aprobar", pendientes)
	}

	// Movimientos: libro UNION asientos del panel (un mes de caja pura no
	// tiene filas en acc_vaucher y sería un período cerrado «sin asientos»,
	// F-2 de la auditoría: las ventas POS existían pero el cierre las negaba).
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM (
		SELECT VNo FROM acc_vaucher WHERE VDate >= ? AND VDate <= ?
		UNION
		SELECT VNo FROM acc_transaction WHERE VDate >= ? AND VDate <= ? AND IFNULL(IsPosted,'yes') NOT IN ('no','0','')
		) movimientos`, from, to, from, to).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no se puede cerrar un período sin asientos")
	}

	if err := escribirFilaCierre(tx, period, "closed", now(), user); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.appendAudit(r, user, "", "close_period", "accounting", "acc_period_close", period, nil,
		map[string]interface{}{"vouchers": n, "status": "closed"})
	return nil
}

// escribirFilaCierre deja la fila de acc_period_close en el estado dicho,
// con UPDATE-primero-e-INSERT-si-no-existe en vez del `ON DUPLICATE KEY
// UPDATE` de MySQL: la redacción SQL del cierre es idéntica en los dos
// motores que el ERP soporta (MySQL en campo, SQLite en pruebas), y así el
// cierre puede estar PROBADO —con `ON DUPLICATE` pelado ninguna prueba de
// este paquete podía ejecutar ClosePeriod, que es como F-1 sobrevivió—.
// El INSERT que choca con un cierre concurrente no es error: se reintenta
// el UPDATE, que es exactamente lo que el competidor ya escribió.
func escribirFilaCierre(tx *sql.Tx, period, estado, fecha, usuario string) error {
	actualizar := func() (int64, error) {
		res, err := tx.Exec(`UPDATE acc_period_close
			SET status=?, closed_at=?, closed_by=?, reopened_at='', reopened_by='', reopen_reason=''
			WHERE period=?`, estado, fecha, usuario, period)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	tocadas, err := actualizar()
	if err != nil {
		return err
	}
	if tocadas > 0 {
		return nil
	}
	if _, err := tx.Exec(`INSERT INTO acc_period_close (period, status, closed_at, closed_by, reopened_at, reopened_by, reopen_reason)
		VALUES (?,?,?,?, '','','')`, period, estado, fecha, usuario); err != nil {
		if tocadas, err2 := actualizar(); err2 == nil && tocadas > 0 {
			return nil
		}
		return err
	}
	return nil
}

// ReopenPeriod reabre un período cerrado.
//
// Se exige un motivo explícito y queda registrado. Reabrir un mes firmado es una
// de las operaciones más delicadas de una contabilidad: no se trata de un cambio
// de dato, es un cambio de una afirmación que ya se hizo. Por eso deja rastro
// con autor, y por eso el período queda marcado como "reabierto", no como si
// nunca hubiera estado cerrado.
func (s *Server) ReopenPeriod(r *http.Request, period, user, motivo string) error {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return fmt.Errorf("reabrir un período cerrado exige un motivo")
	}
	if err := normalizePeriod(period); err != nil {
		return err
	}

	var status sql.NullString
	err := s.dbFor(r).QueryRow(`SELECT status FROM acc_period_close WHERE period = ?`, period).Scan(&status)
	if err == sql.ErrNoRows {
		return fmt.Errorf("el período no está registrado como cerrado")
	}
	if err != nil {
		return err
	}
	if !status.Valid || status.String != "closed" {
		return fmt.Errorf("el período no está cerrado")
	}

	_, err = s.dbFor(r).Exec(`
		UPDATE acc_period_close
		SET status='reopened', reopened_at=?, reopened_by=?, reopen_reason=?
		WHERE period = ?`,
		now(), user, motivo, period)
	if err != nil {
		return err
	}

	s.appendAudit(r, user, "", "reopen_period", "accounting", "acc_period_close", period,
		map[string]interface{}{"status": "closed"},
		map[string]interface{}{"status": "reopened", "motivo": motivo})
	return nil
}

// PeriodsList devuelve los períodos con su estado, para la pantalla de resumen.
func (s *Server) PeriodsList(r *http.Request, limit int) ([]PeriodStatus, error) {
	if limit <= 0 || limit > 120 {
		limit = 24
	}
	rows, err := s.dbFor(r).Query(`
		SELECT c.period, IFNULL(c.status,'open'), IFNULL(c.closed_at,''),
		       IFNULL(c.closed_by,''), IFNULL(c.reopened_at,''), IFNULL(c.reopened_by,''),
		       COUNT(v.id), IFNULL(SUM(v.Debit),0), IFNULL(SUM(v.Credit),0)
		FROM acc_period_close c
		LEFT JOIN acc_vaucher v
		       ON substr(v.VDate,1,7) = c.period
		GROUP BY c.period
		ORDER BY c.period DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeriodStatus
	for rows.Next() {
		var p PeriodStatus
		if rows.Scan(&p.Period, &p.Status, &p.ClosedAt, &p.ClosedBy,
			&p.ReopenedAt, &p.ReopenedBy, &p.Vouchers, &p.TotalDebit, &p.TotalCredit) == nil {
			p.TotalDebit = round2(p.TotalDebit)
			p.TotalCredit = round2(p.TotalCredit)
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// PeriodIsClosed indica si un período está cerrado. Los handlers de documentos
// lo consultan para avisar de que un cambio caerá en un período ya firmado.
func (s *Server) PeriodIsClosed(r *http.Request, period string) bool {
	st, err := s.periodStatus(r, period)
	return err == nil && st == "closed"
}

// ---------- Puerta única de escritura contable ----------

// PeriodoCerradoError: se intentó escribir contabilidad en un mes ya firmado.
//
// Es un tipo y no un texto para que los handlers puedan responder 409 sin
// comparar cadenas, y para que el mensaje sea siempre el mismo: el operador que
// lee dos mensajes distintos para el mismo bloqueo acaba buscando la diferencia
// que no existe.
type PeriodoCerradoError struct{ Periodo string }

func (e PeriodoCerradoError) Error() string {
	return fmt.Sprintf("el periodo %s esta cerrado: reabralo (con motivo) o registre el documento en un periodo abierto", e.Periodo)
}

// periodoDeFecha devuelve el período YYYYMM de una fecha en texto.
//
// `invoice.date` es varchar y el formulario puede mandar cualquier cosa, así que
// aquí no se adivina: lo que no se puede leer devuelve "" y quien pregunta
// decide. Se admite 'YYYY-MM-DD' y el prefijo de 'YYYY-MM-DD HH:MM:SS'.
func periodoDeFecha(fecha string) string {
	f := strings.TrimSpace(fecha)
	if i := strings.IndexAny(f, " T"); i > 0 {
		f = f[:i]
	}
	if len(f) < 7 || f[4] != '-' {
		return ""
	}
	yyyy, mm := f[:4], f[5:7]
	for _, c := range yyyy + mm {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return yyyy + mm
}

// periodoCerradoTx dice si la fecha cae en un período CERRADO, leyendo DENTRO de
// la transacción que va a escribir.
//
// La tabla ausente no bloquea (sin tabla de cierres no hay ningún mes cerrado
// que respetar) pero se registra, para que se vea. Cualquier OTRO error sí
// bloquea: sin poder leer el estado no se puede afirmar que el período esté
// abierto, y un cierre que no se puede leer no es un cierre abierto.
func (s *Server) periodoCerradoTx(tx *sql.Tx, periodo string) (bool, error) {
	var status sql.NullString
	err := tx.QueryRow(`SELECT status FROM acc_period_close WHERE period = ?`, periodo).Scan(&status)
	if err == sql.ErrNoRows || !status.Valid {
		return false, nil
	}
	if err != nil {
		if esFaltaDeTabla(err) {
			log.Printf("AVISO: sin tabla de cierres de período; no se puede comprobar el mes %s: %v", periodo, err)
			return false, nil
		}
		return false, fmt.Errorf("no se pudo comprobar el estado del periodo %s: %w", periodo, err)
	}
	return strings.EqualFold(strings.TrimSpace(status.String), "closed"), nil
}

// esFaltaDeTabla reconoce "la tabla no existe" (MySQL 1146 / SQLite "no such
// table"). Es el único error de lectura que NO bloquea la escritura.
func esFaltaDeTabla(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1146 {
		return true
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "doesn't exist") || strings.Contains(m, "no such table")
}

// exigirPeriodoAbiertoTx impide escribir contabilidad en un período cerrado.
//
// POR QUÉ ESTA COMPROBACIÓN VIVE EN EL PUNTO DE ESCRITURA CONTABLE Y NO EN CADA
// HANDLER: el libro de un mes cerrado solo puede cambiar por un asiento, y todas
// las operaciones que mueven dinero y existencias (venta, compra, cobro, banco,
// notas, IA) pasan por aquí o por la aprobación del asiento. Puesta aquí, la
// regla vale para todos por igual: cajero, contador y agente IA. Antes solo la
// consultaba la IA (ai_tools_mcp), así que un humano podía contabilizar en un
// mes ya firmado y romper un cierre que la IA sí respetaba.
func (s *Server) exigirPeriodoAbiertoTx(tx *sql.Tx, fecha string) error {
	periodo := periodoDeFecha(fecha)
	if periodo == "" {
		// Fecha ilegible: se comprueba el mes en curso, que es el único del que
		// se puede afirmar algo. (La validación del formato de `date` es otro
		// asunto y no se resuelve aquí.)
		periodo = time.Now().Format("200601")
	}
	cerrado, err := s.periodoCerradoTx(tx, periodo)
	if err != nil {
		return err
	}
	if cerrado {
		return PeriodoCerradoError{Periodo: periodo}
	}
	return nil
}

// exigirPeriodoAbierto es la misma regla para los handlers de documentos, que
// tienen que poder decirle al operador qué hacer ANTES de crear el documento (y
// antes de consumir numeración fiscal, que no se devuelve).
func (s *Server) exigirPeriodoAbierto(r *http.Request, fecha string) error {
	periodo := periodoDeFecha(fecha)
	if periodo == "" {
		periodo = time.Now().Format("200601")
	}
	status, err := s.periodStatus(r, periodo)
	if err != nil {
		if esFaltaDeTabla(err) {
			return nil
		}
		// Fail closed: no se escribe sin poder comprobar el cierre.
		return fmt.Errorf("no se pudo comprobar el estado del periodo %s: %w", periodo, err)
	}
	if strings.EqualFold(status, "closed") {
		return PeriodoCerradoError{Periodo: periodo}
	}
	return nil
}
