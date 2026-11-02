package web

// hr_concepto_test.go — Historia del catálogo de conceptos con atributos
// jurídicos (hallazgo 1 de docs/REVISION-LEGAL-2026-11.md).
//
// Criterios de la historia, en Given/When/Then:
//
//   1. Dado un concepto cuyo código o nombre sugiere algo salarial, cuando la
//      derivación lee sus atributos (causa=reembolso), entonces su carácter NO
//      es salarial: el atributo manda, no el nombre.
//   2. Dado un concepto retributivo que declara soporte requerido, cuando el
//      sistema no registra soporte presentado, entonces su carácter es «riesgo»
//      (presunción de salarialidad, 021/2022).
//   3. Dado el catálogo desplegado sobre una tabla legada sin las columnas,
//      cuando el sistema garantiza los atributos al primer uso, entonces cada
//      fila legada queda con los defectos conservadores (retribución,
//      recurrente, VES, sin soporte exigido) y el guard del formulario rechaza
//      todo valor fuera del vocabulario.
//
// Prueba nueva pantalla + candado, patrón del repo: *_test.go de interfaces/web.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpgo/configs"
)

// baseConceptos: base de pruebas con la tabla del catálogo LEGADA (sin las
// columnas nuevas), para que cada prueba compruebe el ensure real.
func baseConceptos(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	s, db := basePrueba(t)
	s.config = &configs.Config{AppEnv: "test", AppName: "ERPGo"}
	s.activeTenant = "default"

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS hr_payroll_concept (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code VARCHAR(30),
		name VARCHAR(120),
		kind VARCHAR(20) DEFAULT 'earning',
		tax_code VARCHAR(30) DEFAULT '',
		affects_net INTEGER DEFAULT 1,
		status INTEGER DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}

	// La caché de columnas es GLOBAL al proceso de pruebas: cada prueba debe
	// empezar con ella limpia o le heredaría la marca de otra base.
	hrConceptoColsMu.Lock()
	hrConceptoCols = map[string]bool{}
	hrConceptoColsMu.Unlock()
	t.Cleanup(func() {
		hrConceptoColsMu.Lock()
		hrConceptoCols = map[string]bool{}
		hrConceptoColsMu.Unlock()
	})

	return s, db
}

// TestCaracterSalarialSeDerivaDeAtributos: el atributo manda, no el nombre.
// Criterios 1 y 2 de la historia.
func TestCaracterSalarialSeDerivaDeAtributos(t *testing.T) {
	casos := []struct {
		nombre         string
		concepto       hrConcepto
		soporteCargado bool // evidencia presentada por el patrono
		esperado       string
	}{
		{
			// Given un código salarial de fabricación, when la causa es
			// reembolso, then NO integra — 523/2025 y art. 105 LOT.
			nombre:   "reembolso con nombre salarial NO integra",
			concepto: hrConcepto{Code: "BONO", Causa: CausaReembolso},
			esperado: CaracNoSalarial,
		},
		{
			// Simétrico: el nombre fácil de engañar es el contrario — un
			// «REEMB_TRA» mal registrado que en realidad es retribución
			// integra, y la corrección es arreglar el atributo, no el mote.
			nombre:   "retribución con nombre de reembolso SÍ integra",
			concepto: hrConcepto{Code: "REEMB_TRA", Causa: CausaRetribucion},
			esperado: CaracSalarioNormal,
		},
		{
			// Prestación social: integra prestaciones (art. 142), no salario
			// normal — 127/2022.
			nombre:   "beneficio social es prestacional",
			concepto: hrConcepto{Code: "BONO_VAC", Causa: CausaBeneficioSocial, Regularidad: RegularidadAnual},
			esperado: CaracPrestacional,
		},
		{
			// Given soporte requerido sin evidencia presentada, when se deriva,
			// then hay riesgo: se presume salarial (021/2022) hasta que el
			// patrono pruebe lo contrario.
			nombre:   "retribución con soporte exigido y ausente es riesgo",
			concepto: hrConcepto{Code: "CAJACHICA", Causa: CausaRetribucion, SoporteReq: true},
			esperado: CaracRiesgo,
		},
		{
			// Presentado el soporte, la presunción se disuelve.
			nombre:         "retribución con soporte presentado es salario normal",
			concepto:       hrConcepto{Code: "CAJACHICA", Causa: CausaRetribucion, SoporteReq: true},
			soporteCargado: true,
			esperado:       CaracSalarioNormal,
		},
		{
			// 523/2025: complemento en divisa. La MONEDA queda registrada por
			// concepto para la separación de bases de la historia siguiente.
			nombre:   "concepto en divisa conserva su atributo",
			concepto: hrConcepto{Code: "COMPL_USD", Causa: CausaRetribucion, Moneda: MonedaUSD},
			esperado: CaracSalarioNormal,
		},
	}
	for _, c := range casos {
		if got := caracterSalarial(c.concepto, c.soporteCargado); got != c.esperado {
			t.Errorf("%s: esperaba %s, dio %s", c.nombre, c.esperado, got)
		}
	}
}

// TestConceptosAtributosNacenSobreLaTablaLegada: el catálogo corre contra
// instalaciones existentes. Criterio 3 de la historia.
func TestConceptosAtributosNacenSobreLaTablaLegada(t *testing.T) {
	s, db := baseConceptos(t)

	// Fila legada escrita ANTES de existir las columnas.
	if _, err := db.Exec(`INSERT INTO hr_payroll_concept (code, name, kind) VALUES ('SALARIO','Salario base','earning')`); err != nil {
		t.Fatal(err)
	}

	// El ensure corre cuando la pantalla lo pide, como en producción.
	sess := &SessionData{UserID: "7", UserLevel: "admin"}
	req := httptest.NewRequest(http.MethodGet, "/hr/conceptos/data", nil)
	req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
	res := httptest.NewRecorder()
	s.hrConceptosData(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("conceptos/data respondió %d: %s", res.Code, res.Body.String())
	}

	var causa, regia, moneda string
	var sop int
	if err := db.QueryRow(`SELECT causa_pago, regularidad, moneda, soporte_requerido
		FROM hr_payroll_concept WHERE code = 'SALARIO'`).Scan(&causa, &regia, &moneda, &sop); err != nil {
		t.Fatalf("el ensure no dejó la fila legada con atributos: %v", err)
	}
	if causa != CausaRetribucion || regia != RegularidadRecurrente || moneda != MonedaVES || sop != 0 {
		t.Fatalf("defectos conservadores incompletos: causa=%s regularidad=%s moneda=%s soporte=%d",
			causa, regia, moneda, sop)
	}
}

// TestConceptosSaveValidaElVocabulario: el guard del servidor rechaza los
// valores libres (criterio 3) y guarda los válidos en MAYÚSCULAS de código,
// sin importar cómo llegaron.
func TestConceptosSaveValidaElVocabulario(t *testing.T) {
	s, db := baseConceptos(t)
	sess := &SessionData{UserID: "7", UserLevel: "admin"}

	guardar := func(forma string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/hr/conceptos/save", strings.NewReader(forma))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
		res := httptest.NewRecorder()
		s.hrConceptosSave(res, req)
		return res
	}

	// Guardado válido: los cuatro atributos grabados tal cual; el código se
	// normaliza a mayúsculas en el servidor y la moneda llega minúscula.
	res := guardar(`code=bono_turnos&name=Bono por turnos&kind=earning&causa=beneficio_social&regularidad=anual&moneda=usd&soporte_requerido=0`)
	if res.Code != http.StatusOK {
		t.Fatalf("guardado válido respondió %d: %s", res.Code, res.Body.String())
	}
	var ok struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &ok); err != nil || ok.Status != 1 {
		t.Fatalf("respuesta sin status=1: %s", res.Body.String())
	}
	var kind, causa, regia, moneda string
	var sop int
	if err := db.QueryRow(`SELECT kind, causa_pago, regularidad, moneda, soporte_requerido
		FROM hr_payroll_concept WHERE code = 'BONO_TURNOS'`).Scan(&kind, &causa, &regia, &moneda, &sop); err != nil {
		t.Fatalf("el concepto nuevo no quedó guardado: %v", err)
	}
	if causa != CausaBeneficioSocial || regia != RegularidadAnual || moneda != MonedaUSD || sop != 0 {
		t.Fatalf("atributos mal persistidos: causa=%s regularidad=%s moneda=%s soporte=%d",
			causa, regia, moneda, sop)
	}

	// Forma inválida 1: causa fuera del vocabulario → 400.
	if res := guardar(`code=X1&name=X&kind=earning&causa=propina&regularidad=anual&moneda=VES`); res.Code != http.StatusBadRequest {
		t.Fatalf("causa libre debe dar 400; dio %d: %s", res.Code, res.Body.String())
	}
	// Forma inválida 2: regularidad libre.
	if res := guardar(`code=X2&name=X&kind=earning&causa=retribucion&regularidad=cada_marte&moneda=VES`); res.Code != http.StatusBadRequest {
		t.Fatalf("regularidad libre debe dar 400; dio %d: %s", res.Code, res.Body.String())
	}
	// Forma inválida 3: moneda libre.
	if res := guardar(`code=X3&name=X&kind=earning&causa=retribucion&regularidad=recurrente&moneda=EUR`); res.Code != http.StatusBadRequest {
		t.Fatalf("moneda libre debe dar 400; dio %d: %s", res.Code, res.Body.String())
	}
	// Forma inválida 4: tipo fuera del vocabulario.
	if res := guardar(`code=X4&name=X&kind=regalia&causa=retribucion&regularidad=recurrente&moneda=VES`); res.Code != http.StatusBadRequest {
		t.Fatalf("tipo libre debe dar 400; dio %d: %s", res.Code, res.Body.String())
	}
	// Forma inválida 5: código duplicado.
	if res := guardar(`code=bono_turnos&name=Otro&kind=earning&causa=retribucion&regularidad=recurrente&moneda=VES`); res.Code != http.StatusBadRequest {
		t.Fatalf("código duplicado debe dar 400; dio %d: %s", res.Code, res.Body.String())
	}

	// Las formas inválidas NO dejaron filas: 400 sin escritura.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hr_payroll_concept`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("las formas inválidas no deben escribir; hay %d filas", n)
	}
}

// TestConceptosSoporteExigidoMarcaRiesgoEnData: el listado entrega el carácter
// derivado por servidor — el usuario no decide nada jurídico desde la pantalla.
func TestConceptosSoporteExigidoMarcaRiesgoEnData(t *testing.T) {
	s, db := baseConceptos(t)
	sess := &SessionData{UserID: "7", UserLevel: "admin"}

	req := httptest.NewRequest(http.MethodPost, "/hr/conceptos/save",
		strings.NewReader(`code=CAJADA&name=Caja chica&kind=earning&causa=retribucion&regularidad=recurrente&moneda=VES&soporte_requerido=1`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
	resGuard := httptest.NewRecorder()
	s.hrConceptosSave(resGuard, req)
	if resGuard.Code != http.StatusOK {
		t.Fatalf("guardado respondió %d: %s", resGuard.Code, resGuard.Body.String())
	}

	consulta := httptest.NewRequest(http.MethodGet, "/hr/conceptos/data", nil)
	consulta = withScope(consulta, &scope{tenant: "default", db: db, sess: sess})
	res := httptest.NewRecorder()
	s.hrConceptosData(res, consulta)

	var raw struct {
		Data []struct {
			Code     string `json:"code"`
			Salarial string `json:"salarial"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	encontrado := false
	for _, d := range raw.Data {
		if d.Code == "CAJADA" {
			encontrado = true
			if d.Salarial != CaracRiesgo {
				t.Fatalf("CAJADA sin soporte presentado debe ser «riesgo»; dio %q", d.Salarial)
			}
		}
	}
	if !encontrado {
		t.Fatal("CAJADA no apareció en el listado")
	}
}

// TestConceptosDataDeduccionesNoIntegran: el carácter del listado queda vacío
// para deducciones y cargas patronales (no hay nada que integrar).
func TestConceptosDataDeduccionesNoIntegran(t *testing.T) {
	s, db := baseConceptos(t)
	sess := &SessionData{UserID: "7", UserLevel: "admin"}

	req := httptest.NewRequest(http.MethodPost, "/hr/conceptos/save",
		strings.NewReader(`code=ADELANTO&name=Adelanto&kind=deduction&causa=retribucion&regularidad=recurrente&moneda=VES`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withScope(req, &scope{tenant: "default", db: db, sess: sess})
	resGuard := httptest.NewRecorder()
	s.hrConceptosSave(resGuard, req)
	if resGuard.Code != http.StatusOK {
		t.Fatalf("guardado de deducción respondió %d: %s", resGuard.Code, resGuard.Body.String())
	}

	consulta := httptest.NewRequest(http.MethodGet, "/hr/conceptos/data", nil)
	consulta = withScope(consulta, &scope{tenant: "default", db: db, sess: sess})
	res := httptest.NewRecorder()
	s.hrConceptosData(res, consulta)

	var raw struct {
		Data []struct {
			Code     string `json:"code"`
			Salarial string `json:"salarial"`
			Kind     string `json:"kind"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, d := range raw.Data {
		if d.Code == "ADELANTO" && d.Salarial != "" {
			t.Fatalf("una deducción no debe llevar carácter salarial; llevó %q", d.Salarial)
		}
	}
}
