package web

// Polling de estado Cashea: el ERP pregunta cada 30s por las sesiones de
// pago en cuotas activas en vez de depender de webhooks (que pueden no estar
// configurados o costar). Es "best effort": si la consulta falla, el estado
// local se mantiene y se reintenta en el ciclo siguiente — una caída de la
// API de Cashea nunca detiene la venta del POS ni la facturación del ERP.

import (
	"database/sql"
	"log"
	"sync"
	"time"
)

// casheaPollInterval es el intervalo entre consultas de estado a la API de Cashea.
// 30 segundos ofrece un equilibrio entre latencia aceptable y carga mínima en el sistema.
// Ajustar a 60s si se observa alta carga o límites de rate del proveedor.
var casheaPollInterval = 30 * time.Second

// casheaMaxConcurrentChecks limita el número de verificaciones simultáneas
// para evitar abrumar la API o la base de datos.
var casheaMaxConcurrentChecks = 10

// casheaSesion es una fila de cashhea_session que el polling va a revisar.
// Es un type con nombre (no un struct anónimo repetido) para que el despacho
// de gorutinas y la función que consulta la API compartan UN contrato, y no
// tres copias que se desalinean con la primera edición.
type casheaSesion struct {
	ID           int64
	SessionID    string
	ClientPhone  string
	SaleAmount   float64
	DownPayment  float64
	Installments int
	Status       string
	CreatedAt    time.Time
}

// StartCasheaStatusPoller inicia el verificador periódico de estado de sesiones
// Cashea y devuelve el control de inmediato: el ciclo corre en segundo plano.
//
// POR QUÉ NO BLOQUEA: el primer arranque lo llamó en serie desde main y, como
// el ciclo interno es infinito (`for range ticker.C`), main se quedó colgado
// justo antes de levantar el HTTP: el ERP quedaba VIVO pero el puerto 8586 nunca
// se abría. El que arranca un polling no debería tener que saber que debe
// escribir `go`: la función lo hace ella misma.
//
// Solo actúa cuando el superusuario tiene el módulo encendido
// (fiscal_setting.cashhea_enabled = "1"): con el módulo apagado el ciclo no
// consume API ni CPU, y el ERP no debe hablar de Cashea si nadie lo pidió.
func (s *Server) StartCasheaStatusPoller() {
	go s.runCasheaStatusPoller()
}

// runCasheaStatusPoller es el ciclo infinito. Se separa de
// StartCasheaStatusPoller para que arrancar el polling no bloquee al caller y
// para que la prueba pueda medir el contrato sin ejecutar un bucle sin fin.
func (s *Server) runCasheaStatusPoller() {
	log.Println("🔄 Iniciando polling de estado Cashea cada 30s")

	ticker := time.NewTicker(casheaPollInterval)
	defer ticker.Stop()

	// Primera corrida inmediata para no esperar los primeros 30s
	go s.pollAllCasheaSessions()

	for range ticker.C {
		go s.pollAllCasheaSessions()
	}
}

// casheaModuloEncendido dice si el superusuario activó el módulo en esta
// empresa. Vuelve false en error: ante duda no se consulta la API externa.
func (s *Server) casheaModuloEncendido() bool {
	var valor string
	err := s.db.QueryRow(`SELECT value FROM fiscal_setting WHERE ` + "`key`" + ` = 'cashhea_enabled'`).Scan(&valor)
	return err == nil && valor == "1"
}

// pollAllCasheaSessions obtiene todas las sesiones Cashea que necesitan atención
// (estado 'pending' o 'active') y verifica su estado mediante la API de Cashea.
// El número de consultas concurrentes está limitado por casheaMaxConcurrentChecks
// para evitar rate limits y sobrecarga.
func (s *Server) pollAllCasheaSessions() {
	if !s.casheaModuloEncendido() {
		return // Módulo apagado para esta empresa: ciclo sin costo.
	}

	// Las sesiones viven en la base de la EMPRESA activa (no en control): el
	// mismo handle s.db que usan los handlers en PyME. En Enterprise, el
	// selector de empresa reinicia el proceso; por eso el handle de la activa
	// es el correcto para un ciclo de fondo.
	db := s.db
	if db == nil {
		log.Println("⚠️  No disponible DB para polling Cashea, reintentará en el próximo ciclo")
		return
	}

	// Prioridad: pending > active que llevan más de 1min sin cambiar.
	rows, err := db.Query(`
		SELECT cs.id, cs.session_id, cs.client_phone, cs.sale_amount,
		       cs.down_payment, cs.installments, cs.status, cs.created_at
		FROM cashhea_session cs
		WHERE cs.status IN ('pending', 'active')
		ORDER BY cs.status = 'pending' DESC, cs.created_at ASC`)
	if err != nil {
		log.Printf("❌ Error consultando sesiones Cashea para polling: %v", err)
		return
	}
	defer rows.Close()

	var sessions []casheaSesion
	for rows.Next() {
		var s casheaSesion
		if err := rows.Scan(&s.ID, &s.SessionID, &s.ClientPhone, &s.SaleAmount,
			&s.DownPayment, &s.Installments, &s.Status, &s.CreatedAt); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	rows.Close()

	// Contar cuántas tienen suficiente información para consultar API
	validSessions := 0
	for _, ses := range sessions {
		if ses.SessionID != "" && ses.SaleAmount > 0 {
			validSessions++
		}
	}
	if validSessions == 0 {
		log.Println("⚠️  No hay sesiones con session_id y sale_amount > 0 para consultar")
		return
	}

	// Procesar sesiones válidas con semáforo
	sem := make(chan struct{}, casheaMaxConcurrentChecks)
	var wg sync.WaitGroup
	for _, ses := range sessions {
		if ses.SessionID == "" || ses.SaleAmount <= 0 {
			continue // Saltar sesiones incompletas
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(sesion casheaSesion) {
			defer wg.Done()
			defer func() { <-sem }()
			checkSingleSessionStatus(db, sesion)
		}(ses)
	}
	wg.Wait()
}

// checkSingleSessionStatus verifica el estado de una sola sesión Cashea
// llamando a la API externa y actualizando la base de datos local.
// Este es el núcleo del polling: en lugar de webhooks, consultamos la API
// periódicamente para saber si una orden ha avanzado (cuota pagada, completada, fallida).
func checkSingleSessionStatus(db *sql.DB, sesion casheaSesion) {
	// Más vale secuencia simple que cache: el propio ciclo es de 30s, así que
	// no se necesita un mecanismo aparte contra consultas seguidas.

	// Llamar a la API de Cashea para obtener el estado actual
	status := callCasheaAPI(sesion)

	// Mapear el estado de la API al estado local
	localStatus := mapAPIStatusToLocal(status)

	// Actualizar la base de datos con el nuevo estado. "Best effort": si la
	// API falla, mantenemos el estado actual y volvemos a intentar en el
	// siguiente ciclo de polling.
	if _, err := db.Exec(`
		UPDATE cashhea_session
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, localStatus, time.Now().UTC().Format("2006-01-02 15:04:05"), sesion.ID); err != nil {
		log.Printf("⚠️  Error actualizando sesión Cashea %s: %v", sesion.SessionID, err)
		return
	}

	// También actualizar las cuotas si la API proporciona información nueva.
	// Es opcional: depende de lo que la API de Cashea exponga.
	updateCasheaInstallments(db, sesion.ID, status)

	log.Printf("📊 Sesión Cashea %s: estado actualizado a '%s' (via polling)", sesion.SessionID, localStatus)
}

// callCasheaAPI representa la llamada a la API real de Cashea. Debe implementarse
// como un HTTP GET al endpoint de Cashea para consultar el estado de la sesión.
//
// Retorna un map con la información relevante:
//   - Type: tipo de evento (installment_paid, session_completed, etc.)
//   - Status: estado general (pending, active, completed, failed)
//   - PaidInstallments: número de cuotas pagadas
//   - NextDueDate: próxima fecha de vencimiento
//
// TODO: Implementar llamada HTTP real a la API de Cashea
// Ejemplo: GET https://api.cashea.com/v1/sessions/{session_id}
//
// Mientras la API real no esté conectada, la simulación usa el tiempo
// transcurrido desde la creación de la sesión como proxy: permite que el
// sistema funcione en desarrollo sin la API real conectada.
func callCasheaAPI(sesion casheaSesion) map[string]interface{} {
	elapsed := time.Since(sesion.CreatedAt)

	// Simulación: si pasaron más de 14 días (un ciclo de cuota), presumir
	// completado. Menos de eso, mantener activo. En producción, esta rama
	// desaparece reemplazada por la llamada HTTP real.
	if int(elapsed.Hours()/24) >= 14 {
		return map[string]interface{}{
			"status": "completed",
			"type":   "session_completed",
		}
	}
	return map[string]interface{}{
		"status": "active",
		"type":   "polling_check",
	}
}

// mapAPIStatusToLocal mapea el estado bruto de la API de Cashea
// al estado que usamos en nuestra base de datos local. Esto aísla la
// integración con Cashea del resto del sistema.
func mapAPIStatusToLocal(apiStatus map[string]interface{}) string {
	status, ok := apiStatus["status"].(string)
	if !ok {
		return "pending" // Fallback seguro
	}

	switch status {
	case "completed", "paid":
		return "completed"
	case "failed", "failed_payment":
		return "failed"
	case "active", "in_progress":
		return "active"
	case "pending":
		return "pending"
	default:
		// Si el estado es desconocido, mantener el estado actual o "pending"
		return "pending"
	}
}

// updateCasheaInstallments actualiza el estado de las cuotas individuales
// basándose en el estado de la sesión mayor. Esto es opcional y depende
// de si la API de Cashea provee información de cuotas detalladas.
func updateCasheaInstallments(db *sql.DB, sessionID int64, apiStatus map[string]interface{}) {
	// Consultar cuotas actuales de la sesión
	rows, err := db.Query(`
		SELECT id, installment_number, due_date, status, amount
		FROM cashhea_installment
		WHERE cashhea_session_id = ?
		ORDER BY installment_number
	`, sessionID)
	if err != nil {
		log.Printf("⚠️  Error consultando cuotas: %v", err)
		return
	}
	defer rows.Close()

	var pendingCount int
	var completedCount int

	for rows.Next() {
		var instID, num int
		var dueDate string
		var instStatus, amount sql.NullString

		if err := rows.Scan(&instID, &num, &dueDate, &instStatus, &amount); err != nil {
			continue
		}

		// Estado nuevo de la cuota según el estado de la sesión:
		// sesión completada → cuotas pagadas; sesión fallida → fallidas.
		// (En una implementación completa, la API de Cashea diría qué
		// cuotas específicas se pagaron.)
		newStatus := instStatus.String // Mantener estado actual
		switch apiStatus["status"].(string) {
		case "completed":
			newStatus = "paid"
		case "failed":
			newStatus = "failed"
		}

		// Solo actualizar si el estado cambió
		if newStatus != instStatus.String {
			_, updateErr := db.Exec(`
				UPDATE cashhea_installment
				SET status = ?, updated_at = ?
				WHERE id = ?
			`, newStatus, time.Now().UTC().Format("2006-01-02 15:04:05"), instID)
			if updateErr != nil {
				log.Printf("⚠️  Error actualizando cuota %d: %v", instID, updateErr)
			} else {
				log.Printf("📈 Cuota %d de sesión %d actualizada a '%s'", num, sessionID, newStatus)
			}
		}

		if newStatus == "paid" {
			completedCount++
		} else {
			pendingCount++
		}
	}

	log.Printf("📋 Estado de cuotas sesión %d: %d pagadas, %d pendientes", sessionID, completedCount, pendingCount)
}
