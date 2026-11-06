package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

// AccountingService replicates the SalesERP double-entry engine:
//   acc_vaucher (pending vouchers) --approve--> acc_transaction (posted, double rows)
// Every voucher row posted generates TWO acc_transaction rows:
//   1) the row itself (COAID, Debit/Credit as-is)
//   2) the reverse row (COAID=RevCodde, Debit/Credit swapped)
// Monthly balances are cached in acc_monthly_balance.

// VoucherRow is one line of a voucher.
type VoucherRow struct {
	VNo          string
	Vtype        string // DV | CV | CONTRA | JV
	ReferenceNo  string
	VDate        string // YYYY-MM-DD
	COAID        int64
	RevCodde     int64
	Narration    string
	Comment      string
	Debit        float64
	Credit       float64
	SubType      int
	SubCode      sql.NullInt64
	ChequeNo     string
	ChequeDate   string
	CurrencyCode string
	ExchangeRate float64
}

// insertVoucherRow writes one row into acc_vaucher (pending, unapproved).
// Mirrors insert_sale_creditvoucher / insert_purchase_debitvoucher.
func (s *Server) insertVoucherRow(r *http.Request, tx *sql.Tx, row VoucherRow, createBy string) error {
	if row.VDate == "" {
		row.VDate = today()
	}
	// Período cerrado: no se crea contabilidad en un mes ya firmado. La regla
	// vale igual para el cajero, para el contador y para el agente IA.
	if err := s.exigirPeriodoAbiertoTx(tx, row.VDate); err != nil {
		return err
	}
	chequeDate := interface{}(nil)
	if row.ChequeDate != "" {
		chequeDate = row.ChequeDate
	}
	_, err := tx.Exec(`
		INSERT INTO acc_vaucher
			(fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration, ledgerComment,
			 Debit, Credit, RevCodde, subType, subCode, chequeNo, chequeDate,
			 isApproved, CreateBy, CreateDate, status,
			 currency_code, exchange_rate)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,0,?,?)`,
		financialYear(s.dbFor(r)), row.VNo, row.Vtype, row.ReferenceNo, row.VDate, row.COAID,
		row.Narration, row.Comment, row.Debit, row.Credit, row.RevCodde,
		row.SubType, row.SubCode, row.ChequeNo, chequeDate, voucherCreateBy(createBy), now(),
		orDefault(row.CurrencyCode, "VES"), ifNull(row.ExchangeRate, 1.0))
	return err
}

// approveVoucher replicates Accounts_model->approved_vaucher($vno,'active'):
// posts every acc_vaucher row of VNo into acc_transaction (double entry),
// updates monthly balances, then marks the voucher approved.
func (s *Server) approveVoucher(r *http.Request, vno string, approvedBy string) error {
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotency: already-approved vouchers cannot be re-approved
	// (prevents duplicated acc_transaction rows like PHP's isApproved flag).
	var already int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM acc_vaucher WHERE VNo = ? AND isApproved = 1`, vno).Scan(&already); err != nil {
		return err
	}
	if already > 0 {
		return nil
	}

	rows, err := tx.Query(`SELECT id, fyear, VNo, Vtype, referenceNo, VDate, COAID,
		Narration, chequeNo, chequeDate, isHonour, ledgerComment, Debit, Credit, RevCodde, subType, subCode,
		currency_code, exchange_rate
		FROM acc_vaucher WHERE VNo = ?`, vno)
	if err != nil {
		return err
	}

	type vrow struct {
		id, fyear, coaid, revcode    int64
		vno, vtype, ref              string
		vdate                        interface{}
		narration, comment, chequeNo sql.NullString
		chequeDate                   interface{}
		isHonour, subType            sql.NullInt64
		subCode                      sql.NullInt64
		debit, credit                float64
		currencyCode                 string
		exchangeRate                 float64
	}
	var list []vrow
	for rows.Next() {
		var v vrow
		if err := rows.Scan(&v.id, &v.fyear, &v.vno, &v.vtype, &v.ref, &v.vdate, &v.coaid,
			&v.narration, &v.chequeNo, &v.chequeDate, &v.isHonour, &v.comment, &v.debit, &v.credit,
			&v.revcode, &v.subType, &v.subCode, &v.currencyCode, &v.exchangeRate); err != nil {
			rows.Close()
			return err
		}
		list = append(list, v)
	}
	rows.Close()

	if len(list) == 0 {
		return fmt.Errorf("voucher %s not found", vno)
	}

	for _, v := range list {
		// `acc_vaucher.VDate` y `chequeDate` son DATE y el DSN va con
		// `parseTime=true`: vuelven como time.Time y escribirlos tal cual en
		// `acc_transaction` da Error 1292 "Incorrect date value:
		// '2026-10-04T00:00:00Z'". Es decir: con MySQL, aprobar un asiento
		// NUNCA funcionó, y como el error se come la transacción entera, el
		// posting se deshace y el libro se queda sin movimiento.
		// `fechaBD` devuelve la fecha en texto y "" cuando el valor es NULL.
		vdate := fechaBD(v.vdate)
		// Aprobar es lo que MUEVE el libro: si el período está cerrado, no se
		// publica. Sin esto, un asiento pendiente de un mes firmado podía
		// aprobarse hoy y cambiar el libro de un mes ya revisado.
		if err := s.exigirPeriodoAbiertoTx(tx, vdate); err != nil {
			return err
		}
		var chequeDate interface{}
		if f := fechaBD(v.chequeDate); f != "" {
			chequeDate = f
		}
		chequeNo := interface{}(nil)
		if v.chequeNo.Valid {
			chequeNo = v.chequeNo.String
		}
		// main row
		if _, err := tx.Exec(`
			INSERT INTO acc_transaction
				(vid, fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration,
				 chequeNo, chequeDate, isHonour, ledgerComment, Debit, Credit,
				 StoreID, IsPosted, RevCodde, subType, subCode, IsAppove, CreateBy, CreateDate,
				 currency_code, exchange_rate)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,1,?,?,?,1,?,?,?,?)`,
			v.id, v.fyear, v.vno, v.vtype, v.ref, vdate, v.coaid, v.narration,
			chequeNo, chequeDate, v.isHonour, v.comment, v.debit, v.credit,
			v.revcode, v.subType, v.subCode, approvedBy, now(),
			orDefault(v.currencyCode, "VES"), ifNull(v.exchangeRate, 1.0)); err != nil {
			return err
		}
		// reverse row
		if _, err := tx.Exec(`
			INSERT INTO acc_transaction
				(vid, fyear, VNo, Vtype, referenceNo, VDate, COAID, Narration,
				 chequeNo, chequeDate, isHonour, ledgerComment, Debit, Credit,
				 StoreID, IsPosted, RevCodde, subType, subCode, IsAppove, CreateBy, CreateDate,
				 currency_code, exchange_rate)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,1,?,?,?,1,?,?,?,?)`,
			v.id, v.fyear, v.vno, v.vtype, v.ref, vdate, v.revcode, v.narration,
			chequeNo, chequeDate, v.isHonour, v.comment, v.credit, v.debit,
			v.coaid, v.subType, v.subCode, approvedBy, now(),
			orDefault(v.currencyCode, "VES"), ifNull(v.exchangeRate, 1.0)); err != nil {
			return err
		}
		// monthly balances for both legs
		if err := storeTransactionSummary(tx, v.coaid, vdate); err != nil {
			return err
		}
		if err := storeTransactionSummary(tx, v.revcode, vdate); err != nil {
			return err
		}
	}

	// `acc_vaucher.approvedBy` es INT: sin la traducción, aprobar un asiento
	// hecho por "carnicero" falla con Error 1366 y el posting entero (las dos
	// filas de acc_transaction) se deshace. El libro se queda sin movimiento y
	// el descuadre aparece al mes siguiente.
	if _, err := tx.Exec(`UPDATE acc_vaucher SET isApproved=1, approvedBy=?, approvedDate=?, status=1 WHERE VNo=?`,
		voucherCreateBy(approvedBy), now(), vno); err != nil {
		return err
	}
	return tx.Commit()
}

// reverseVoucher replicates Accounts_model->reverseVoucher: deletes posted
// transactions and marks the voucher pending again.
func (s *Server) reverseVoucher(r *http.Request, vno string) error {
	tx, err := s.dbFor(r).Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM acc_transaction WHERE VNo = ?`, vno); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE acc_vaucher SET isApproved=0, status=0, UpdateDate=? WHERE VNo=?`, now(), vno); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteVoucher removes a pending voucher completely.
func (s *Server) deleteVoucher(r *http.Request, vno string) error {
	_, err := s.dbFor(r).Exec(`DELETE FROM acc_vaucher WHERE VNo = ? AND status = 0`, vno)
	return err
}

// storeTransactionSummary replicates store_transation_summery: refreshes
// acc_monthly_balance for a COA in the month of the given date.
func storeTransactionSummary(tx *sql.Tx, coaid int64, date string) error {
	if coaid == 0 {
		return nil
	}
	// month number
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		// try datetime
		t, err = time.Parse("2006-01-02 15:04:05", date)
		if err != nil {
			return nil
		}
	}
	month := int(t.Month())
	fy := financialYearTx(tx)
	balance, err := closingBalanceTx(tx, coaid, date)
	if err != nil {
		return err
	}

	var existing int
	if err := tx.QueryRow(`SELECT id FROM acc_monthly_balance WHERE COAID=? AND fyear=?`, coaid, fy).Scan(&existing); err == sql.ErrNoRows {
		_, err = tx.Exec(fmt.Sprintf(`INSERT INTO acc_monthly_balance (fyear, COAID, balance%d, updatedDate) VALUES (?,?,?,?)`, month),
			fy, coaid, balance, now())
		return err
	} else if err != nil {
		return err
	}
	_, err = tx.Exec(fmt.Sprintf(`UPDATE acc_monthly_balance SET balance%d=?, updatedDate=? WHERE COAID=? AND fyear=?`, month),
		balance, now(), coaid, fy)
	return err
}

// financialYearTx: active FY id (tx version).
func financialYearTx(tx *sql.Tx) int {
	var id int
	err := tx.QueryRow(`SELECT id FROM financial_year WHERE startDate <= ? AND endDate >= ? AND status = 1 LIMIT 1`,
		today(), today()).Scan(&id)
	if err != nil {
		return 1
	}
	return id
}

// closingBalanceTx replicates get_clossing_balance: opening + current movement.
func closingBalanceTx(tx *sql.Tx, headcode int64, date string) (float64, error) {
	var headType string
	err := tx.QueryRow(`SELECT HeadType FROM acc_coa WHERE HeadCode = ?`, headcode).Scan(&headType)
	if err != nil {
		return 0, nil // unknown head: no balance
	}

	// opening = movements strictly before date
	var openDebit, openCredit, curDebit, curCredit float64
	_ = tx.QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0) FROM acc_transaction
		WHERE COAID = ? AND IsAppove = 1 AND VDate < ?`, headcode, date).Scan(&openDebit, &openCredit)
	_ = tx.QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0) FROM acc_transaction
		WHERE COAID = ? AND IsAppove = 1 AND VDate = ?`, headcode, date).Scan(&curDebit, &curCredit)

	if headType == "A" || headType == "E" {
		return (openDebit - openCredit) + (curDebit - curCredit), nil
	}
	return (openCredit - openDebit) + (curCredit - curDebit), nil
}

// headBalance computes the current balance of a COA head (approved only).
func (s *Server) headBalance(r *http.Request, headcode int64) float64 {
	var headType string
	var debit, credit float64
	err := s.dbFor(r).QueryRow(`SELECT HeadType FROM acc_coa WHERE HeadCode = ?`, headcode).Scan(&headType)
	if err != nil {
		return 0
	}
	_ = s.dbFor(r).QueryRow(`SELECT IFNULL(SUM(Debit),0), IFNULL(SUM(Credit),0) FROM acc_transaction
		WHERE COAID = ? AND IsAppove = 1`, headcode).Scan(&debit, &credit)
	if headType == "A" || headType == "E" {
		return debit - credit
	}
	return credit - debit
}

// ifNull devuelve el valor o el defecto si es 0/NULL
func ifNull(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// predefineAccount loads acc_predefine_account (cached per call).
type PredefineAccount struct {
	CashCode         int64
	BankCode         int64
	PurchaseCode     int64
	SalesCode        int64
	ServiceCode      int64
	CustomerCode     int64
	SupplierCode     int64
	COGSCode         int64
	VatCode          int64
	TaxCode          int64
	InventoryCode    int64
	SalaryCode       int64
	ProvStateTaxCode int64

	// Cuentas de la separación entre mercancía, consumo interno y servicios.
	// Se leen aparte y con tolerancia: una base anterior a la migración no las
	// tiene y el sistema debe seguir arrancando con los respaldos de abajo.
	SuppliesInventoryCode int64
	SuppliesExpenseCode   int64
	ServiceExpenseCode    int64
	PurchaseVatCode       int64
}

// CuentaInventarioMercancia: el inventario de lo que se vende.
func (p *PredefineAccount) CuentaInventarioMercancia() int64 { return p.InventoryCode }

// CuentaInventarioSuministros: el inventario de lo que se consume dentro de la
// empresa (papelería, limpieza). NO cae al inventario de mercancía: mezclar los
// dos es el defecto que esta separación corrige, y un respaldo silencioso lo
// reintroduciría sin que nadie lo note. Si falta, la operación se detiene con un
// mensaje que manda a Configuración → Cuentas por defecto.
func (p *PredefineAccount) CuentaInventarioSuministros() int64 {
	return p.SuppliesInventoryCode
}

// CuentaGastoSuministros: gasto por consumo de suministros (no es costo de
// ventas: la papelería no se vende). Tampoco cae al costo de ventas.
func (p *PredefineAccount) CuentaGastoSuministros() int64 {
	return p.SuppliesExpenseCode
}

// CuentaGastoServicio: gasto por servicios recibidos (luz, agua, internet).
// NO cae a ServiceCode: esa cuenta es de INGRESO y fue justo el defecto que
// hacía que un gasto se asentara restando ingresos.
func (p *PredefineAccount) CuentaGastoServicio() int64 { return p.ServiceExpenseCode }

// CuentaIvaCompras: IVA crédito fiscal de las compras. Antes el IVA se sumaba
// al inventario (el asiento debitaba el total del documento contra la cuenta de
// inventario); con la cuenta separada el IVA queda donde corresponde.
//
// No cae a la cuenta de IVA por pagar: un crédito fiscal no es un pasivo, y
// asentarlo ahí descuadra la declaración.
func (p *PredefineAccount) CuentaIvaCompras() int64 {
	return p.PurchaseVatCode
}

func (s *Server) predefineAccount(r *http.Request) (*PredefineAccount, error) {
	return predefineAccountTx(s.dbFor(r))
}

// consultadorUnaFila es una interfaz que permite pasar diferentes fuentes de consulta
// (sql.DB, sql.Tx, o cualquier tipo que implemente QueryRow) a funciones que leen un solo registro.
type consultadorUnaFila interface {
	QueryRow(query string, args ...any) *sql.Row
}

// predefineAccountTx lee el plan de cuentas predefinido DESDE UNA CONEXIÓN QUE
// SE LE PASA, no desde `dbFor(r)`.
//
// Existe porque los caminos del POS no tienen ámbito de petición: su empresa
// viaja en la cabecera X-POS-Tenant y `dbFor(r)` cae a la empresa ACTIVA, que es
// la del último que inició sesión en el panel. Leyendo así, un terminal podía
// asentar su venta con las cuentas de OTRA empresa. Recibiendo la conexión (la
// del terminal, ya resuelta), la lectura va donde tiene que ir.
func predefineAccountTx(q consultadorUnaFila) (*PredefineAccount, error) {
	var p PredefineAccount
	err := q.QueryRow(`SELECT cashCode, bankCode, purchaseCode, salesCode, serviceCode,
		customerCode, supplierCode, costs_of_good_solds, vat, tax, inventoryCode, salary_code, prov_state_tax
		FROM acc_predefine_account ORDER BY id LIMIT 1`).
		Scan(&p.CashCode, &p.BankCode, &p.PurchaseCode, &p.SalesCode, &p.ServiceCode,
			&p.CustomerCode, &p.SupplierCode, &p.COGSCode, &p.VatCode, &p.TaxCode, &p.InventoryCode, &p.SalaryCode, &p.ProvStateTaxCode)
	if err != nil {
		return nil, fmt.Errorf("acc_predefine_account missing: %w", err)
	}
	// Cuentas de la separación mercancía / consumo / servicios. La consulta va
	// aparte a propósito: si la base todavía no tiene estas columnas (proceso de
	// arranque, restauración a medio migrar), el error se ignora y los métodos de
	// respaldo deciden. Un fallo aquí no puede tumbar el arranque entero.
	_ = q.QueryRow(`SELECT IFNULL(supplies_inventory_code,0), IFNULL(supplies_expense_code,0),
			IFNULL(service_expense_code,0), IFNULL(purchase_vat_code,0)
		FROM acc_predefine_account ORDER BY id LIMIT 1`).
		Scan(&p.SuppliesInventoryCode, &p.SuppliesExpenseCode, &p.ServiceExpenseCode, &p.PurchaseVatCode)
	return &p, nil
}

// subcodeID returns acc_subcode.id for a reference (customer/supplier id) and subTypeId (3=customer, 4=supplier).
func (s *Server) subcodeID(r *http.Request, referenceNo int64, subTypeId int) sql.NullInt64 {
	var id int
	err := s.dbFor(r).QueryRow(`SELECT id FROM acc_subcode WHERE referenceNo = ? AND subTypeId = ? LIMIT 1`, referenceNo, subTypeId).Scan(&id)
	if err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(id), Valid: true}
}
