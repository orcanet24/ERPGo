package web

// company_bootstrap.go — Dejar una empresa lista para usarse.
//
// Este archivo existe por una razón concreta: había DOS caminos para crear una
// empresa (el arranque normal y el asistente multi-empresa) y cada uno sembraba
// un subconjunto distinto de catálogos. El resultado era una empresa creada
// desde el asistente sin admin, sin menús y sin plan de cuentas: se creaba, se
// veía, y no había forma de entrar.
//
// La regla que se aplica aquí es simple: una empresa está lista cuando tiene
// esquema, catálogos base y un administrador. Nada menos. Todo lo que una
// empresa necesita para funcionar se siembra desde PrepareCompany, y ambos
// caminos la llaman. Si mañana se agrega un catálogo nuevo, se agrega UNA vez
// en este archivo y toda empresa nueva lo tiene.

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"erpgo/infrastructure/persistence"
)

// PrepareCompany deja una base de empresa en estado utilizable. Es
// idempotente: se puede llamar en cada arranque sin duplicar nada, y sobre una
// empresa ya configurada no toca lo que el negocio haya customised.
//
// La única excepción, deliberada: la base normativa para la IA se crea VACÍA.
// Son afirmaciones que alguien con autoridad tiene que cargar y validar; un
// sistema que rellena sus propias leyes acaba creyéndose autor.
func PrepareCompany(db *sql.DB, dialect, legalName, rif string) {
	SeedFiscal(db, dialect)
	SeedHR(db)
	if err := SeedCRM(db); err != nil {
		log.Printf("CRM (etapas base): %v", err)
	}
	if err := SeedRemarketingSegments(db); err != nil {
		log.Printf("CRM (segmentos remarketing base): %v", err)
	}
	if err := EnsureLegalSchema(db); err != nil {
		log.Printf("base normativa para la IA: %v", err)
	}
	if catalog, _, _, _, err := COACatalog(db); err == nil && catalog == COACatalogCONASEV {
		st := InspectCOACatalog(db)
		logCOACatalog(st)
	}
	ensureCurrentFinancialYear(db)
	if legalName != "" {
		if err := setCompanyIdentity(db, legalName, rif); err != nil {
			log.Printf("datos de la empresa: %v", err)
		}
	}
}

// PrepareCompanyAs is PrepareCompany with caller-chosen admin credentials.
func PrepareCompanyAs(db *sql.DB, dialect, legalName, rif, adminUser, adminPass string) {
	SeedFiscal(db, dialect)
	SeedHR(db)

	// Etapas base del embudo comercial. Solo se siembran si la tabla está
	// vacía: una empresa que ya configuró su embudo no debe que se le imponga
	// el nuestro.
	if err := SeedCRM(db); err != nil {
		log.Printf("CRM (etapas base): %v", err)
	}
	if err := SeedRemarketingSegments(db); err != nil {
		log.Printf("CRM (segmentos remarketing base): %v", err)
	}
	if err := EnsureLegalSchema(db); err != nil {
		log.Printf("base normativa para la IA: %v", err)
	}

	// Plan de cuentas: NO se siembra nada aquí. El catálogo contable es de la
	// empresa, no del sistema (ver coa_catalog.go). Cargar el estándar del
	// SENIAT en cada arranque era lo que mezclaba dos catálogos en acc_coa y
	// dejaba nueve códigos con dos nombres distintos sin que nadie lo supiera.
	//
	// Si la empresa ya eligió CONASEV se verifica que siga íntegro; si no, no se
	// toca su plan. Cargar el catálogo es una acción explícita del contador.
	if catalog, _, _, _, err := COACatalog(db); err == nil && catalog == COACatalogCONASEV {
		st := InspectCOACatalog(db)
		logCOACatalog(st)
	}

	// Usuario administrador, empresa, preferencias, módulos y permisos.
	if err := persistence.InitializeAdminUser(db, dialect, adminUser, adminPass); err != nil {
		log.Printf("Admin user initialization: %v", err)
	}
	ensureCurrentFinancialYear(db)

	// La razón social y el RIF que escribió el asistente tienen que quedar en
	// company_information: es de donde salen los encabezados de facturas,
	// reportes y el nombre del menú. Sin esto la empresa se llama "Mi Empresa"
	// en todos sus documentos.
	if legalName != "" {
		if err := setCompanyIdentity(db, legalName, rif); err != nil {
			log.Printf("datos de la empresa: %v", err)
		}
	}
}

// setCompanyIdentity deja la razón social y el RIF como datos de la empresa.
// Usa el mismo patrón que el resto del código (upsert sobre company_id='1') en
// vez de un UPDATE a secas, porque en una base recién sembrada la fila aún no
// existe y un UPDATE no actualizaría nada.
func setCompanyIdentity(db *sql.DB, legalName, rif string) error {
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM company_information WHERE company_id='1'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		_, err := db.Exec(`INSERT INTO company_information (company_id, company_name, email, address, mobile, website, vat_no, cr_no, status)
			VALUES ('1', ?, '', '', '', '', ?, '', 1)`, legalName, rif)
		return err
	}
	_, err := db.Exec(`UPDATE company_information SET company_name=?, vat_no=? WHERE company_id='1'`, legalName, rif)
	return err
}

// ensureCurrentFinancialYear deja abierto el ejercicio fiscal del año en
// curso. Sin él la empresa no puede emitir facturas: la numeración cuelga del
// año fiscal, así que esto no es opcional aunque parezca un detalle.
func ensureCurrentFinancialYear(db *sql.DB) {
	year := time.Now().Year()
	name := fmt.Sprintf("%d", year)
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM financial_year WHERE yearName = ?`, name).Scan(&n)
	if n > 0 {
		return
	}
	_, _ = db.Exec(`INSERT INTO financial_year (yearName, startDate, endDate, isCloseReq, status, created_by)
		VALUES (?, ?, ?, 0, 1, 1)`, name, fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
}

