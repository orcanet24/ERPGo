package web

// orchestrator.go — El orquestador de escrituras, uno por empresa.
//
// POR QUÉ UNO POR EMPRESA. El correlativo del orquestador es la posición en la
// fila, y las filas son de una empresa concreta (una base por empresa, ver
// tenants.go). Un correlativo compartido entre empresas mezclaría dos negocios
// en una sola numeración, que es exactamente lo que un orden fiscal no puede
// hacer. Por eso el journal es data/escrituras/<empresa>.journal y no uno solo.
//
// QUÉ ES Y QUÉ NO ES TODAVÍA. El orquestador es la puerta y ya funciona con sus
// pruebas, pero los handlers antiguos que hacen INSERT por su cuenta siguen
// estándolo: migrarlos uno a uno es el trabajo que queda, y está anotado en
// doc/HOJA-DE-RUTA.md. Lo que ya no es cierto es que no exista la puerta.
//
// El fallo al abrir el orquestador NO impide arrancar la app. Es el mismo
// criterio que la cola: es preferible facturar sin orquestador (y decirlo en el
// log) a negarse a levantar el ERP por un problema de disco. Lo que no se
// permite es fingir que está activo.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"erpgo/internal/orchestrator"
)

// orchMu protege el mapa de orquestadores por empresa.
var orchMu sync.Mutex

// orquestadores guarda un orquestador por código de empresa.
var orquestadores = map[string]*orchestrator.Orquestador{}

// fieldKey es la FieldKey del binario, puesta por cmd/server antes de levantar
// el servidor. Vacía significa desarrollo: el journal va en claro.
//
// Existía el código para cifrar pero no la vía para entregar la clave, así que
// el journal se creaba siempre en claro. Esto es esa vía.
var (
	fieldKeyMu sync.RWMutex
	fieldKey   string
)

// SetFieldKey entrega al proceso la FieldKey del binario.
//
// Es un setter y no un parámetro de NewServer a propósito: la clave llega del
// paquete main (donde vive el ancla que escribe -ldflags) y los paquetes de
// interfaz no importan cmd/server. Pasarla por un setter explícito, en un punto
// visible del arranque, hace que se pueda comprobar que ocurre; esconderlo en
// una variable de paquete haría que pareciera magia que ya está resuelto.
func SetFieldKey(k string) {
	fieldKeyMu.Lock()
	defer fieldKeyMu.Unlock()
	fieldKey = k
}

// leerFieldKey devuelve la clave vigente.
func leerFieldKey() string {
	fieldKeyMu.RLock()
	defer fieldKeyMu.RUnlock()
	return fieldKey
}

// journalDir es donde viven las colas de escritura, bajo el directorio de datos
// y no dentro de data/empresas: no es una base de datos, es la cola que la
// acompaña.
func (s *Server) journalDir() string {
	return filepath.Join(s.config.DataPath(), "escrituras")
}

// startOrchestrator abre el orquestador de una empresa.
//
// El Aplicador por defecto es un INSERT genérico de las columnas que trae la
// entrada. Los handlers con validación propia (una factura, un producto) pasan
// el suyo: lo que aporta el orquestador es la transacción, el correlativo, la
// idempotencia y el rastro, no el SQL de negocio.
func (s *Server) startOrchestrator(tenant string) (*orchestrator.Orquestador, error) {
	orchMu.Lock()
	defer orchMu.Unlock()
	if o, ok := orquestadores[tenant]; ok {
		return o, nil
	}

	db, err := s.openTenantDB(tenant)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la base de %s: %w", tenant, err)
	}
	if err := os.MkdirAll(s.journalDir(), 0o700); err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio de escrituras: %w", err)
	}

	ruta := filepath.Join(s.journalDir(), tenant+".journal")
	clave := leerFieldKey()
	o, err := orchestrator.New(db, ruta, clave, insertorGenerico, orchestrator.Config{
		MaxPending: 1024,
	})
	if err != nil {
		return nil, err
	}
	o.Start()
	orquestadores[tenant] = o
	if clave != "" {
		log.Printf("orquestador de escrituras activo para %s (journal CIFRADO en %s)", tenant, ruta)
	} else {
		log.Printf("orquestador de escrituras activo para %s (journal en claro: sin FieldKey, %s)",
			tenant, ruta)
	}
	return o, nil
}

// insertorGenerico es el Aplicador por defecto: escribe exactamente las columnas
// que trae la entrada, y nada más.
//
// Los nombres de tabla y columna llegan ya validados por el propio orquestador
// (orchestrator.IdentificadorSeguro, llamado en validar()). Aquí se vuelven a
// comprobar porque este archivo CONSTRUYE la sentencia: una comprobación en
// memoria no protege a un INSERT que se arma en otro sitio, y el filtro solo
// vale si está en la puerta, no dentro de la puerta.
func insertorGenerico(ctx context.Context, tx *sql.Tx, e orchestrator.Entrada) (int64, error) {
	if !orchestrator.IdentificadorSeguro(e.Tabla) {
		return 0, fmt.Errorf("orquestador: nombre de tabla no válido: %q", e.Tabla)
	}
	if len(e.Columnas) == 0 {
		return 0, fmt.Errorf("orquestador: la escritura no indica columnas")
	}
	consulta := "INSERT INTO " + e.Tabla + " ("
	interrogaciones := ""
	for i, col := range e.Columnas {
		if !orchestrator.IdentificadorSeguro(col) {
			return 0, fmt.Errorf("orquestador: nombre de columna no válido: %q", col)
		}
		if i > 0 {
			consulta += ","
			interrogaciones += ","
		}
		consulta += col
		interrogaciones += "?"
	}
	consulta += ") VALUES (" + interrogaciones + ")"

	res, err := tx.ExecContext(ctx, consulta, e.Valores...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Escribir es la puerta por la que debe pasar toda escritura de negocio.
//
// El nombre es deliberadamente corto y obligatorio: mientras exista una vía
// alternativa, esta función se ignora y el rastro se vuelve opcional otra vez,
// que es el problema que el orquestador viene a cerrar.
func (s *Server) Escribir(ctx context.Context, e orchestrator.Entrada) (orchestrator.Resultado, error) {
	tenant := e.Empresa
	if tenant == "" {
		tenant = s.activeTenant
		e.Empresa = tenant
	}
	o, err := s.startOrchestrator(tenant)
	if err != nil {
		return orchestrator.Resultado{}, err
	}
	return o.Submit(ctx, e)
}

// stopOrchestrators para todos los orquestadores abiertos. Lo pendiente queda
// en los journals y se recupera al arrancar: apagar el servidor no puede
// costar una venta.
func stopOrchestrators() {
	orchMu.Lock()
	defer orchMu.Unlock()
	for code, o := range orquestadores {
		o.Stop()
		delete(orquestadores, code)
	}
}

