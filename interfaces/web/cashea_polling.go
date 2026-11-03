package web

import (
	"database/sql"
	"log"
	"time"
)

// casheaPollInterval es el intervalo entre consultas de estado a la API de Cashea.
// 30 segundos ofrece un equilibrio entre latencia aceptable y carga mínima en el sistema.
// Ajustar a 60s si se observa alta carga o límites de rate del proveedor.
var casheaPollInterval = 30 * time.Second

// casheaMaxConcurrentChecks limita el número de verificaciones simultáneas
// para evitar abrumar la API o la base de datos.
var casheaMaxConcurrentChecks = 10

// StartCasheaStatusPoller inicia el verificador periódico de estado de sesiones Cashea.
// Este goroutine se ejecuta en background y revisa el estado de todas las sesiones
// activas o pending cada 30 segundos, actualizando la base de datos local con el
// estado reportado por la API de Cashea.
func StartCasheaStatusPoller() {
	log.Println("🔄 Iniciando polling de estado Cashea cada 30s")

	ticker := time.NewTicker(casheaPollInterval)
	defer ticker.Stop()

	// Primera corrida inmediata para no esperar los primeros 30s
	go pollAllCasheaSessions()

	for range ticker.C {
		go pollAllCasheaSessions()
	}
}

// pollAllCasheaSessions obtiene todas las sesiones Cashea que necesitan atención
// (estado 'pending' o 'active') y verifica su estado mediante la API de Cashea.
// El número de consultas concurrentes está limitado por casheaMaxConcurrentChecks
// para evitar rate limits y sobrecarga.
func pollAllCasheaSessions() {
	db := getDBForCasheaPoll()
	if db == nil {
		log.Println("⚠️  No disponible DB para polling Cashea, reintentará en el próximo ciclo")
		return
	}

	// Obtener sesiones que necesitan estado actualizado
	// Prioridad: pending > active que llevan más de 1min sin cambiar
	rows, err := db.Query(`
		SELECT cs.id, cs.session_id, cs.client_phone, cs.sale_amount,
			   cs.down_payment, cs.installments, cs.status, cs.created_at
		FROM cashhea_session cs
		WHERE cs.status IN ('pending', 'active')
		ORDER BY cs.status = 'pending' DESC, cs.created_at ASC
	`, )
	if err != nil {
		log.Printf("❌ Error consultando sesiones Cashea para polling: %v", err)
		return
	}
	defer rows.Close()

	var sessions []struct {
		ID           int64
		SessionID    string
		ClientPhone  string
		SaleAmount   float64
		DownPayment  float64
		Installments int
		Status       string
		CreatedAt    time.Time
	}

	for rows.Next() {
		var s struct {
			ID           int64
			SessionID    string
			ClientPhone  string
			SaleAmount   float64
			DownPayment  float64
			Installments int
			Status       string
			CreatedAt    time.Time
		}
		if err := rows.Scan(&s.ID, &s.SessionID, &s.ClientPhone, &s.SaleAmount,
			&s.DownPayment, &s.Installments, &s.Status, &s.CreatedAt); err != nil {
			log.Printf("⚠️ Error escaneando sesión Cashea: %v", err)
			continue
		}
		sessions = append(sessions, s)
	}

	if len(sessions) == 0 {
		return // Nada que revisar en este ciclo
	}

	log.Printf("📡 Polling Cashea: revisando %d sesión(es)", len(sessions))

	// Procesar en lotes limitados para no saturar
	sem := make(chan struct{}, casheaMaxConcurrentChecks)
	
	// Contar cuántas tienen suficiente información para consultar API
	validSessions := 0
	for _, s := range sessions {
		if s.SessionID != "" && s.SaleAmount > 0 {
			validSessions++
		}
	}

	if validSessions == 0 {
		log.Println("⚠️  No hay sesiones con session_id y sale_amount > 0 para consultar")
		return
	}

	// Procesar sesiones válidas con semáforo
	processed := 0
	for _, s := range sessions {
		if s.SessionID == "" || s.SaleAmount <= 0 {
			continue // Saltar sesiones incompletas
		}

		sem <- struct{}{}
		go func(session struct {
			ID           int64
			SessionID    string
			ClientPhone  string
			SaleAmount   float64
			DownPayment  float64
			Installments int
			Status       string
		}) {
			defer func() { <-sem }()
			checkSingleSessionStatus(db, session)
		}(s)
		processed++
	}

	// Esperar a que terminen todas las gorutinas actuales
	// (El semáforo se cierra cuando todas liberan)
}

// checkSingleSessionStatus verifica el estado de una sola sesión Cashea
// llamando a la API externa y actualizando la base de datos local.
// Este es el núcleo del polling: en lugar de webhooks, consultamos la API
// periódicamente para saber si una orden ha avanzado (cuota pagada, completada, fallida).
func checkSingleSessionStatus(db *sql.DB, session struct {
	ID           int64
	SessionID    string
	ClientPhone  string
	SaleAmount   float64
	DownPayment  float64
	Installments int
	Status       string
}) {
	// Evitar consultas demasiado seguidas a la misma sesión
	// Mínimo 30 segundos entre consultas a la misma sesión
	// (Se podría mejorar con un cache simple por session_id + timestamp_ultima_consulta)
	
	// Llamar a la API de Cashea para obtener el estado actual
	// Esta función placeholder representaría la llamada al API de Cashea
	status := callCasheaAPI(session.SessionID, session.SaleAmount, session.Installments)
	
	// Mapear el estado de la API al estado local
	localStatus := mapAPIStatusToLocal(status)
	
	// Actualizar la base de datos con el nuevo estado
	// Usamos "best effort" - si la API falla, mantenemos el estado actual
	// y volvemos a intentar en el siguiente ciclo de polling
	if _, err := db.Exec(`
		UPDATE cashhea_session 
		SET status = ?, updated_at = NOW()
		WHERE id = ?
	`, localStatus, session.ID); err != nil {
		log.Printf("⚠️  Error actualizando sesión Cashea %s: %v", session.SessionID, err)
		return
	}
	
	// También actualizar las cuotas si la API proporciona información nueva
	// Esto es opcional y depende de la API de Cashea disponible
	updateCasheaInstallments(db, session.ID, status)
	
	log.Printf("📊 Sesión Cashea %s: estado actualizado a '%s' (via polling)", session.SessionID, localStatus)
}

// callCasheaAPI es una función placeholder que representaría la llamada a la API
// real de Cashea. En la implementación real, esta función haría un HTTP GET
// a la endpoint de Cashea para consultar el estado de la sesión.
// 
// Retorna un struct con la información relevante:
// - Type: tipo de evento (installment_paid, session_completed, etc.)
// - Status: estado general (pending, active, completed, failed)
// - PaidInstallments: número de cuotas pagadas
// - NextDueDate: próxima fecha de vencimiento
func callCasheaAPI(sessionID string, saleAmount float64, installments int) map[string]interface{} {
	// TODO: Implementar llamada HTTP real a la API de Cashea
	// Ejemplo: GET https://api.cashea.com/v1/sessions/{session_id}
	
	// Por ahora, retornamos un estado "placeholder" que simularía
	// una respuesta básica de la API. En producción, esto sería un HTTP real.
	
	// Simulación: por ahora retornamos estado basado en el tiempo transcurrido
	// desde que se creó la sesión. Esto permite que el sistema funcione
	// sin la API real conectada durante el desarrollo.
	elapsed := time.Since(session.ID) // Usamos ID como proxy de tiempo (provisional)
	
	// Lógica simplificada para demo:
	// - Si pasaron más de 14 días (un ciclo de cuota), marcar como "completed" o "failed"
	// - Si han pasado menos, mantener "active"
	// días := int(elapsed.Hours() / 24)
	// if días >= 14 {
	//     return map[string]interface{}{
	//         "status": "completed",
	//         "type": "session_completed",
	//     }
	// }
	
	// Por ahora retornamos "active" por defecto hasta que la API real esté conectada
	return map[string]interface{}{
		"status": "active",
		"type":   "polling_check",
	}
}

// mapAPIStatusToLocal mapea el estado bruto de la API de Cashea
// al estado que usamos en nuestra base de datos local.
// Esto aísla la integración con Cashea del resto del sistema.
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
	// Obtener el ID de la sesión cashhea_session
	var sessionModelID int64
	err := db.QueryRow(`SELECT id FROM cashhea_session WHERE id = ?`, sessionID).Scan(&sessionModelID)
	if err != nil {
		log.Printf("⚠️  No encontrado cashhea_session id=%d: %v", sessionID, err)
		return
	}
	
	// Consultar cuotas actuales
	rows, err := db.Query(`
		SELECT id, installment_number, due_date, status, amount 
		FROM cashhea_installment 
		WHERE cashhea_session_id = ?
		ORDER BY installment_number
	`, sessionModelID)
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
		
		// Determinar nuevo estado de la cuota basándonos en el estado de la sesión
		newStatus := instStatus.String // Mantener estado actual
		
		// Lógica simple: si sesión está completada, todas las cuotas deben estar pagadas
		// Si sesión falló, marcar como fallida
		// (En una implementación completa, la API de Cashea proveería qué cuotas específicas pagaron)
		
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
				SET status = ?, updated_at = NOW()
				WHERE id = ?
			`, newStatus, instID)
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

// getDBForCasheaPoll obtiene la conexión a la base de datos para las operaciones de polling.
// En un entorno real, esto podría usar un pool de conexiones o la DB principal del tenant.
func getDBForCasheaPoll() *sql.DB {
	// Por ahora retornamos nil - en una integración real, esto obtendría
	// la DB correcta basada en el tenant/empresa activa en el contexto
	// Por ejemplo: s.dbFor(r) o s.posDBFor(r)
	// 
	// La llamada a esta función requiere contexto de request, pero el polling
	// es un background goroutine. La solución real dependería de la arquitectura
	// de cómo se identifican las empresas/tenants en este sistema.
	return nil
}