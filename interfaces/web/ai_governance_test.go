package web

// Pruebas de gobernanza de la IA (27-10-2026).
//
// Cada test fija uno de los tres huecos que se encontraron revisando el módulo, y
// está escrito para FALLAR si el hueco vuelve:
//
//  1. La marca «requiere humano» se calculaba y se perdía al serializar el
//     journal, porque el campo no estaba exportado. El test la serializa y la
//     vuelve a leer, que es exactamente lo que hacía el runner.
//  2. Ninguna herramienta consultaba los permisos del rol. Los tests construyen
//     el guardián (gate) y comprueban que un rol sin el permiso de escritura no
//     pasa, que uno con el permiso sí pasa, y que el catálogo anunciado no ofrece
//     lo que el guardián va a rechazar.
//  3. No había llave de idempotencia. Los tests fijan que la misma llamada
//     produce la misma clave (independiente del orden del map), que la referencia
//     externa manda sobre los argumentos, y que la respuesta duplicada no deja
//     creer al agente que creó algo.
//
// Son funciones puras a propósito: no necesitan base de datos y por eso corren en
// cualquier máquina. Lo que sí toca la base (la reclamación del libro, la ranura)
// se cubre con las pruebas de integración del arranque.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// gateDePrueba arma un conjunto de permisos sin tocar la base.
func gateDePrueba(level string, concesiones map[string]string) aiRoleGate {
	g := aiRoleGate{level: level, granted: map[string]map[string]bool{}}
	for dir, acciones := range concesiones {
		g.granted[dir] = map[string]bool{}
		for _, a := range strings.Split(acciones, ",") {
			g.granted[dir][strings.TrimSpace(a)] = true
		}
	}
	return g
}

// TestNeedsHumanSobreviveAlJournal es la regresión del campo muerto.
//
// El runner persiste el journal con json.Marshal y lo relee con Unmarshal al
// revertir. Si el campo no está exportado (o no lleva etiqueta), el viaje de ida
// y vuelta lo borra en silencio: eso era el hueco, y este test lo detecta.
func TestNeedsHumanSobreviveAlJournal(t *testing.T) {
	j := &aiRunJournal{}
	j.add("payment", map[string]interface{}{"invoice_no": float64(7), "amount": float64(100)})
	j.NeedsHuman = true

	crudo, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("no se pudo serializar el journal: %v", err)
	}
	if !strings.Contains(string(crudo), "needs_human") {
		t.Fatalf("el journal serializado no lleva la marca; JSON: %s", string(crudo))
	}

	var vuelta aiRunJournal
	if err := json.Unmarshal(crudo, &vuelta); err != nil {
		t.Fatalf("no se pudo releer el journal: %v", err)
	}
	if !vuelta.NeedsHuman {
		t.Fatal("la marca de revisión humana se perdió al persistir el journal")
	}
	if len(vuelta.Actions) != 1 || vuelta.Actions[0].Kind != "payment" {
		t.Fatalf("el journal perdió acciones en el viaje: %+v", vuelta.Actions)
	}
}

// TestHerramientasDeEscrituraDeclaranPermiso fija la lista de herramientas que
// escriben y exige que cada una declare su permiso.
//
// La lista es explícita a propósito: obliga a tomar la decisión —¿qué permiso?—
// al añadir una herramienta nueva, que es el momento en que barato, y no cuando
// alguien descubra que un agente escribió sin permiso.
func TestHerramientasDeEscrituraDeclaranPermiso(t *testing.T) {
	escriben := []string{
		"invoice.create_draft", "invoice.post", "payment.record", "voucher.create",
		"report.save_template",
		// Declaradas y todavía sin implementar: nacen protegidas.
		"invoice.anull", "account.post_voucher", "customer.upsert",
		"payment.create", "payroll.run", "payroll.post",
		// Implementadas por el alta rápida de compras.
		"supplier.upsert", "product.upsert", "purchase.create",
	}
	for _, nombre := range escriben {
		tool, ok := aiToolByName[nombre]
		if !ok {
			t.Errorf("la herramienta '%s' ya no está en el registro", nombre)
			continue
		}
		if tool.Permission.Directory == "" || tool.Permission.Action == "" {
			t.Errorf("la herramienta '%s' escribe y no declara permiso: el guardián no puede exigirlo", nombre)
		}
	}
}

// TestGuardianExigeElPermisoDelRol es la regresión del hueco 2.
func TestGuardianExigeElPermisoDelRol(t *testing.T) {
	vacia := gateDePrueba("user", nil)
	v := aiGuardAction("voucher.create", map[string]interface{}{}, true, vacia)
	if v.Allowed {
		t.Fatal("un rol sin permiso de vouchers no puede ejecutar voucher.create")
	}
	if v.Rule != "permisos" {
		t.Errorf("el bloqueo debe identificarse como de permisos; salió %q (%s)", v.Rule, v.Reason)
	}
	if !strings.Contains(v.Reason, "vouchers") {
		t.Errorf("el motivo debe decir QUÉ permiso falta, para que se pueda arreglar: %s", v.Reason)
	}

	// Con el permiso concedido, la misma llamada pasa.
	conPermiso := gateDePrueba("user", map[string]string{"vouchers": "create"})
	if v := aiGuardAction("voucher.create", map[string]interface{}{}, true, conPermiso); !v.Allowed {
		t.Fatalf("con el permiso 'vouchers/create' la herramienta debe ejecutarse; dijo: %s", v.Reason)
	}
}

// TestGuardianNoDoblaElCandadoDeLectura confirma que las lecturas siguen
// dependiendo del departamento del rol y no de un permiso de directorio.
//
// Es deliberado: doblar la lectura por directorio rompería el diseñador de
// informes, que usa al agente por defecto para LEER.
func TestGuardianNoDoblaElCandadoDeLectura(t *testing.T) {
	vacia := gateDePrueba("user", nil)
	for _, nombre := range []string{"db.query", "describe_schema", "query_report", "data.summary"} {
		if v := aiGuardAction(nombre, map[string]interface{}{}, false, vacia); !v.Allowed {
			t.Errorf("la lectura '%s' no debe exigir permiso de directorio; dijo: %s", nombre, v.Reason)
		}
	}
}

// TestAdminYMasterConservanElAcceso comprueba la paridad con la web: quien puede
// hacerlo en pantalla puede hacerlo con la herramienta.
func TestAdminYMasterConservanElAcceso(t *testing.T) {
	for _, nivel := range []string{"admin", "master"} {
		g := gateDePrueba(nivel, nil)
		if v := aiGuardAction("payment.record", map[string]interface{}{}, true, g); !v.Allowed {
			t.Errorf("el nivel %s no debería quedar bloqueado por permisos; dijo: %s", nivel, v.Reason)
		}
	}
}

// TestEmpleadoSoloLee replica la regla de canAccessModule: el nivel employee no
// escribe, ni siquiera con el permiso marcado.
func TestEmpleadoSoloLee(t *testing.T) {
	g := gateDePrueba("employee", map[string]string{"manage_invoice": "create,read,update,delete"})
	if v := aiGuardAction("payment.record", map[string]interface{}{}, true, g); v.Allowed {
		t.Fatal("un agente con nivel employee no debe poder registrar cobros")
	}
	if v := aiGuardAction("db.query", map[string]interface{}{}, false, g); !v.Allowed {
		t.Fatalf("employee sí puede leer; dijo: %s", v.Reason)
	}
}

// TestCatalogoNoAnunciaLoQueElGuardianRechaza.
//
// El catálogo y el guardián tienen que decir lo mismo: ofrecer una herramienta
// que va a ser rechazada gasta pasos, tokens y confianza.
func TestCatalogoNoAnunciaLoQueElGuardianRechaza(t *testing.T) {
	vacia := gateDePrueba("user", nil)
	nombres := map[string]bool{}
	for _, t2 := range aiCatalogForPhase(true, vacia) {
		nombres[t2.Name] = true
	}
	for _, prohibida := range []string{"voucher.create", "payment.record", "invoice.create_draft", "invoice.post"} {
		if nombres[prohibida] {
			t.Errorf("el catálogo anuncia '%s' a un agente cuyo rol no la puede ejecutar", prohibida)
		}
	}
	if !nombres["db.query"] {
		t.Error("el catálogo debe seguir ofreciendo las lecturas")
	}

	conTodo := gateDePrueba("user", map[string]string{
		"vouchers": "create", "manage_invoice": "create,update", "new_invoice": "create,update",
	})
	nombres = map[string]bool{}
	for _, t2 := range aiCatalogForPhase(true, conTodo) {
		nombres[t2.Name] = true
	}
	for _, permitida := range []string{"voucher.create", "payment.record", "invoice.create_draft", "invoice.post"} {
		if !nombres[permitida] {
			t.Errorf("el catálogo debe anunciar '%s' cuando el rol tiene el permiso", permitida)
		}
	}
}

// TestClaveDeIdempotenciaEsEstable es la regresión del hueco 3, en su parte pura.
func TestClaveDeIdempotenciaEsEstable(t *testing.T) {
	scope := "task:3:day:2026-10-27"
	a := map[string]interface{}{"invoice_id": float64(7), "amount": float64(150), "note": "transferencia"}
	// Mismo contenido, construido en otro orden: encoding/json ordena las claves,
	// así que la clave tiene que salir idéntica. Si el orden importara, el
	// reintento de la cola no reconocería su propia llamada y volvería a escribir.
	b := map[string]interface{}{"note": "transferencia", "amount": float64(150), "invoice_id": float64(7)}
	if aiIdemKey("payment.record", scope, "", a) != aiIdemKey("payment.record", scope, "", b) {
		t.Fatal("la misma llamada produjo dos claves distintas: el reintento duplicaría el documento")
	}

	// Distinto ámbito (otro día) es otro documento: la clave debe cambiar.
	if aiIdemKey("payment.record", scope, "", a) ==
		aiIdemKey("payment.record", "task:3:day:2026-10-28", "", a) {
		t.Fatal("dos períodos distintos no pueden compartir clave")
	}
}

// TestReferenciaExternaMandaSobreTodo: cuando el documento viene de un listado,
// la referencia es la identidad, y da igual cuándo o con qué argumentos se llame.
func TestReferenciaExternaMandaSobreTodo(t *testing.T) {
	uno := aiIdemKey("invoice.create_draft", "task:1:day:2026-10-27", "LISTADO-77/linea-3",
		map[string]interface{}{"customer_id": float64(4)})
	otro := aiIdemKey("invoice.create_draft", "task:1:day:2026-11-15", " listado-77/LINEA-3 ",
		map[string]interface{}{"customer_id": float64(9)})
	if uno != otro {
		t.Fatal("la misma referencia externa debe dar la misma clave, sin importar día ni argumentos")
	}
	if uno == aiIdemKey("payment.record", "task:1:day:2026-10-27", "LISTADO-77/linea-3", nil) {
		t.Fatal("la misma referencia en dos herramientas distintas no puede colisionar")
	}
}

// TestRespuestaDuplicadaNoMiente: el agente tiene que entender que no creó nada.
func TestRespuestaDuplicadaNoMiente(t *testing.T) {
	resp := aiDuplicateResult(`{"status":"ok","vno":"CV-42","debit":100}`)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &m); err != nil {
		t.Fatalf("la respuesta duplicada no es JSON válido: %v", err)
	}
	if m["duplicate"] != true {
		t.Error("la respuesta duplicada debe llevar duplicate=true")
	}
	if m["vno"] != "CV-42" {
		t.Error("debe conservar el documento original: el agente lo necesita para el informe")
	}
	if _, ok := m["note"]; !ok {
		t.Error("debe explicar que no se creó nada nuevo")
	}

	// Un resultado no-JSON tampoco puede perderse.
	resp = aiDuplicateResult("no soy json")
	if !strings.Contains(resp, "no soy json") {
		t.Error("un resultado no-JSON debe viajar como previous_result, no desaparecer")
	}
}

// TestRanuraDeProgramacionUnaVezPorPeriodo es la regresión de la duplicación del
// ticker: una tarea diaria vale para todo el día, no para cada vuelta de 5 min.
func TestRanuraDeProgramacionUnaVezPorPeriodo(t *testing.T) {
	tarea := aiTask{ID: 9, ScheduleType: "daily", ScheduleValue: "6"}
	base := time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)
	for _, minuto := range []int{0, 5, 25, 55} {
		otra := base.Add(time.Duration(minuto) * time.Minute)
		if aiRunSlot(tarea, base) != aiRunSlot(tarea, otra) {
			t.Fatalf("la vuelta de las 6:%02d debería caer en la misma ranura que las 6:00", minuto)
		}
	}
	if aiRunSlot(tarea, base) == aiRunSlot(tarea, base.Add(24*time.Hour)) {
		t.Fatal("el día siguiente es otro período y debe ser otra ranura")
	}

	semanal := aiTask{ID: 9, ScheduleType: "weekly", ScheduleValue: "1"}
	if aiRunSlot(semanal, base) != aiRunSlot(semanal, base.Add(24*time.Hour)) {
		t.Error("dos días de la misma semana comparten ranura en una tarea semanal")
	}
	if aiRunSlot(semanal, base) == aiRunSlot(semanal, base.AddDate(0, 0, 7)) {
		t.Error("la semana siguiente es otro período")
	}

	cada5 := aiTask{ID: 9, ScheduleType: "cron5"}
	if aiRunSlot(cada5, base) == aiRunSlot(cada5, base.Add(5*time.Minute)) {
		t.Error("una tarea cron5 corre en cada vuelta: cada una es su propia ranura")
	}

	manual := aiTask{ID: 9, ScheduleType: "manual"}
	if aiRunSlot(manual, base) != "" {
		t.Error("una tarea manual no tiene período que reclamar")
	}
}

// TestAmbitoManualComparteDia con una tarea programada: lanzar a mano dos veces la
// misma tarea el mismo día no puede crear el documento dos veces.
func TestAmbitoManualComparteDia(t *testing.T) {
	manual := aiTask{ID: 5, ScheduleType: "manual"}
	if strings.TrimSpace(aiRunScope(manual, "")) == "" {
		t.Fatal("el disparo manual necesita un ámbito, o cada corrida sería un documento nuevo")
	}
}
