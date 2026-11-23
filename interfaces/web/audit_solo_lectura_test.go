package web

// Pruebas de la parte 3 del bloque A: la auditoría deja de registrar LECTURAS
// que llegan por POST.
//
// El problema que tenían: los listados piden sus datos por POST, y
// auditMiddleware registraba toda petición POST/PUT/DELETE. Cada registro son
// tres sentencias (SELECT del hash anterior, INSERT, UPDATE del hash nuevo)
// bajo auditChainMu, el candado que serializa las escrituras de la pista. Así
// que cada tecla en el buscador de una lista era una fila de ruido en
// audit_log y una espera por el candado.
//
// Lo que se comprueba aquí es que al quitar esas lecturas NO se pierde nada de
// lo que la pista existe para probar:
//
//  1. Toda MUTACIÓN sigue quedando registrada.
//  2. La cadena sigue ENCADENADA: la fila nueva apunta al hash final de la
//     anterior y su hash recalculado coincide con el guardado. (Si se perdiera
//     el orden, la verificación del libro entero saltaría en el primer
//     registro.)
//  3. El endpoint de solo lectura sigue ejecutándose: quitar la auditoría no
//     es quitar la ruta.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// clasificarRutasDeAuditoria: lo que SÍ se audita y lo que no.
func TestRutasDeSoloLecturaNoSeAuditan(t *testing.T) {
	soloLectura := []string{
		"/purchase/list/data",      // listado serverSide
		"/purchase/dup-check",      // pregunta si ya existe
		"/cartera/escanear",        // escaneo de CxC
		"/pagar/escanear",          // escaneo de CxP
		"/hr/tax-rates/verify",     // verificación de tasas
		"/settings/database/check", // integridad de la base
	}
	for _, p := range soloLectura {
		if !esLecturaPorPost(p) {
			t.Errorf("%s es de solo lectura y quedó en la lista de auditar", p)
		}
	}

	// Y lo que NUNCA se salta, aunque el nombre se le parezca.
	escritura := []string{
		"/purchase/save",
		"/customer/save",
		"/invoice/pos/save",
		"/cartera/cobrar",
		"/settings/notify/test",     // envía un correo de verdad
		"/settings/database/backup", // crea un respaldo
		"/config/menu/move",
	}
	for _, p := range escritura {
		if esLecturaPorPost(p) {
			t.Errorf("%s ESCRIBE datos y no puede quedar fuera de la auditoría", p)
		}
	}
}

// montarPeticionConSesion pone el ámbito de la petición (empresa + identidad)
// igual que lo pone attachScope en el ciclo real.
func montarPeticionConSesion(method, path string, s *Server) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	return withScope(r, &scope{tenant: defaultTenantCode, db: s.db, sess: sessPrueba("admin")})
}

// TestAuditoriaNoRegistraLecturasPeroSiEscrituras es el comportamiento nuevo.
func TestAuditoriaNoRegistraLecturasPeroSiEscrituras(t *testing.T) {
	s, db := basePrueba(t)

	// 1) Un listado por POST: pasa, ejecuta su handler y NO deja rastro.
	visto := false
	handler := s.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = true
		w.WriteHeader(http.StatusOK)
	}))
	r := montarPeticionConSesion("POST", "/purchase/list/data", s)
	handler.ServeHTTP(httptest.NewRecorder(), r)
	if !visto {
		t.Fatal("la ruta de solo lectura dejó de ejecutarse: se quitó la auditoría, no la ruta")
	}

	var lecturas int
	_ = db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&lecturas)
	if lecturas != 0 {
		t.Errorf("una búsqueda de listado escribió %d filas en la pista de auditoría", lecturas)
	}

	// 2) Una escritura de verdad: sigue registrándose.
	r = montarPeticionConSesion("POST", "/customer/save", s)
	handler.ServeHTTP(httptest.NewRecorder(), r)

	var escrituras int
	_ = db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&escrituras)
	if escrituras != 1 {
		t.Fatalf("un guardado debe dejar 1 fila de auditoría, hay %d", escrituras)
	}
}

// TestAuditoriaCadenaSigueEncadenada es la prueba de que al dejar de auditar
// lecturas la cadena criptográfica no se resiente: dos escrituras seguidas
// siguen enlazadas y con el hash correcto.
func TestAuditoriaCadenaSigueEncadenada(t *testing.T) {
	s, db := basePrueba(t)
	handler := s.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Dos mutaciones seguidas (una lectura intercalada no debe romper nada).
	for _, path := range []string{"/customer/save", "/purchase/list/data", "/product/save"} {
		handler.ServeHTTP(httptest.NewRecorder(), montarPeticionConSesion("POST", path, s))
	}

	type fila struct {
		id        int64
		userID    string
		action    string
		module    string
		table     string
		recordID  string
		oldV      string
		newV      string
		ip        string
		createdAt string
		prevHash  string
		hash      string
	}
	rows, err := db.Query(`SELECT id, user_id, action, module, table_name, record_id, old_value,
		new_value, ip_address, created_at, prev_hash, record_hash FROM audit_log ORDER BY id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var filas []fila
	for rows.Next() {
		var f fila
		if err := rows.Scan(&f.id, &f.userID, &f.action, &f.module, &f.table, &f.recordID,
			&f.oldV, &f.newV, &f.ip, &f.createdAt, &f.prevHash, &f.hash); err != nil {
			t.Fatal(err)
		}
		filas = append(filas, f)
	}

	// La lectura NO está: quedan sólo las dos escrituras.
	if len(filas) != 2 {
		t.Fatalf("la cadena debe tener 2 escrituras (la lectura no cuenta), tiene %d", len(filas))
	}
	if filas[0].action != "create" || filas[1].action != "create" {
		t.Errorf("acciones registradas: %q y %q", filas[0].action, filas[1].action)
	}

	// Encadenamiento: la segunda apunta al hash final de la primera…
	if filas[1].prevHash != filas[0].hash {
		t.Errorf("cadena rota: la fila 2 apunta a %q y la fila 1 termina en %q", filas[1].prevHash, filas[0].hash)
	}
	// …y su hash recalculado coincide con el guardado.
	querido := computeAuditHash(filas[1].prevHash, filas[1].id, filas[1].userID, filas[1].action,
		filas[1].module, filas[1].table, filas[1].recordID, filas[1].oldV, filas[1].newV,
		filas[1].ip, filas[1].createdAt, "")
	if querido != filas[1].hash {
		t.Errorf("hash de la fila 2 no recalcula: guardado %q, calculado %q", filas[1].hash, querido)
	}
}

// TestAuditoriaDeEscrituraUsaLaMismaIdentidad: quien aparece en la pista sigue
// siendo la sesión de la petición, no un «system» genérico.
func TestAuditoriaDeEscrituraUsaLaMismaIdentidad(t *testing.T) {
	s, db := basePrueba(t)
	handler := s.auditMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), montarPeticionConSesion("POST", "/customer/save", s))

	var usuario string
	if err := db.QueryRow(`SELECT user_id FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&usuario); err != nil {
		t.Fatal(err)
	}
	if usuario != "admin" {
		t.Errorf("la fila de auditoría quedó a nombre de %q y debía ser %q", usuario, "admin")
	}
}
