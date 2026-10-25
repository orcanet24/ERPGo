package web

// addon_capabilities.go — Revisar y aprobar lo que un addon pide, ANTES de activarlo.
//
// EL PROBLEMA QUE RESUELVE.
//
// El manifiesto de un addon declara lo que quiere: leer lotes, vetar ventas,
// recibir eventos. Está firmado, así que sabemos quién lo declara; pero firmado
// no significa aprobado. Un addon legítimo puede pedir más de lo que este
// negocio quiere darle, y hasta ahora bastaba con instalarlo para que todas sus
// capabilities entraran en vigor a la vez que el proceso.
//
// LA REGLA, EN TRES FRASES.
//
//   - module.json dice lo que el addon PIDE.
//   - addon_capability_approvals dice lo que la empresa CONCEDE.
//   - Lo que no está concedido no se sirve, y un addon con capabilities sin
//     decidir no se puede activar.
//
// POR QUÉ SE FILTRA TAMBIÉN EN CALIENTE.
//
// Revocar una capability surte efecto en la petición siguiente, sin reinstalar
// ni reiniciar el proceso: el grant que viaja al addon se construye con las
// capabilities efectivas (manifiesto ∩ aprobaciones), no con el manifiesto. Si
// se filtrara solo al activar, revocar exigiría reinstalar y quedaría la duda de
// si el proceso viejo conserva los permisos viejos.
//
// POR QUÉ "SIN DECISIÓN" NO ES "APROBADO".
//
// El silencio no concede nada. Una capability sin fila está PENDIENTE, y las
// pendientes bloquean la activación. Rechazarla es una decisión explícita: el
// addon se instala y corre, pero esa consulta le devuelve 403 y el guard no se
// consulta. Rechazar y no decidir se parecen en el efecto y no en la intención:
// la pantalla tiene que distinguirlos, y por eso el estado tiene tres valores.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/orcanet/ergoaddon"
)

// Tipos de capability. Coinciden con las claves de ergoaddon.Capabilities, para
// que el JSON de la pantalla y el del manifiesto hablen el mismo idioma.
const (
	addonCapResource = "resource"
	addonCapCommand  = "command"
	addonCapEvent    = "event"
	addonCapGuard    = "sale_guard"
	addonCapNotify   = "notify"

	addonCapPending  = "pending"
	addonCapApproved = "approved"
	addonCapRejected = "rejected"
)

// addonCapability es una capability declarada junto con la decisión tomada.
//
// Description no es decorativa: el administrador que aprueba tiene que entender
// qué concede. Un `inventory.lot.read` sin explicación se aprueba por costumbre.
type addonCapability struct {
	Kind        string `json:"kind"`
	Code        string `json:"code"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	FailClosed  bool   `json:"fail_closed,omitempty"`
	DecidedBy   string `json:"decided_by,omitempty"`
}

// addonCapabilityDecision es una fila de addon_capability_approvals.
type addonCapabilityDecision struct {
	Approved bool
	By       string
}

// capabilityDescription explica en una línea qué concede cada capability
// conocida. Lo desconocido no se rechaza —el host es quien decide qué sirve—
// pero se dice tal cual: "declarada por el addon, sin descripción del núcleo".
func capabilityDescription(kind, code string) string {
	switch kind {
	case addonCapResource:
		switch code {
		case addonResProduct:
			return "Leer productos del catálogo (código, nombre, precio)."
		case addonResLot:
			return "Leer lotes y fechas de caducidad."
		case addonResCustomer:
			return "Leer datos de contacto de clientes."
		case addonResInvoice:
			return "Leer resúmenes de facturas emitidas."
		}
	case addonCapCommand:
		if code == addonCmdCRMActivity {
			return "Registrar actividades de CRM en nombre del usuario de la sesión."
		}
	case addonCapEvent:
		switch code {
		case addonEvtInvoiceIssued:
			return "Recibir aviso, después del commit, de cada factura emitida."
		case addonEvtPaymentRecv:
			return "Recibir aviso, después del commit, de cada cobro recibido."
		}
	case addonCapGuard:
		return "Opinar (y vetar) cada venta antes de facturar."
	case addonCapNotify:
		return "Permitir que el módulo " + code + " pida envíos de mensajería."
	}
	return "Declarada por el addon; el núcleo no la describe."
}

// addonCapabilityKey identifica una capability en la tabla de decisiones.
func addonCapabilityKey(kind, code string) string { return kind + ":" + code }

// addonDeclaredCapabilities expande las capabilities del manifiesto en la lista
// plana que se aprueba. El orden es estable para que dos pantallas no muestren
// las filas en distinto orden y el administrador crea que cambió algo.
func addonDeclaredCapabilities(caps *ergoaddon.Capabilities) []addonCapability {
	if caps == nil {
		return nil
	}
	var out []addonCapability
	add := func(kind string, values []string) {
		for _, v := range values {
			out = append(out, addonCapability{
				Kind:        kind,
				Code:        v,
				State:       addonCapPending,
				Description: capabilityDescription(kind, v),
			})
		}
	}
	add(addonCapResource, caps.Resources)
	add(addonCapCommand, caps.Commands)
	add(addonCapEvent, caps.Events)
	// Los módulos autorizados a pedir envíos son una capability más: se
	// aprueban uno por uno, y aprobar "facturacion" no autoriza a "compras".
	if caps.Notify != nil {
		add(addonCapNotify, caps.Notify.Modules)
	}
	if caps.SaleGuard != nil {
		out = append(out, addonCapability{
			Kind:        addonCapGuard,
			Code:        addonCapGuard,
			State:       addonCapPending,
			Description: capabilityDescription(addonCapGuard, addonCapGuard),
			FailClosed:  caps.SaleGuard.FailClosed,
		})
	}
	return out
}

// addonCapabilityDecisions lee las decisiones de un addon.
//
// Un error se devuelve tal cual y NO se convierte en "todo aprobado": quien
// llama decide qué hacer, y en la activación eso significa frenar, no seguir.
func addonCapabilityDecisions(db *sql.DB, code string) (map[string]addonCapabilityDecision, error) {
	out := map[string]addonCapabilityDecision{}
	if db == nil {
		return out, fmt.Errorf("base de datos no disponible")
	}
	rows, err := db.Query(`SELECT kind, code, is_approved, IFNULL(decided_by, '')
		FROM addon_capability_approvals WHERE addon_code = ?`, code)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, capCode, by string
		var approved int
		if err := rows.Scan(&kind, &capCode, &approved, &by); err != nil {
			return out, err
		}
		out[addonCapabilityKey(kind, capCode)] = addonCapabilityDecision{
			Approved: approved == 1,
			By:       by,
		}
	}
	return out, rows.Err()
}

// addonCapabilityItems devuelve las capabilities declaradas con su estado.
//
// Si la consulta falla, las devuelve todas como pendientes: el silencio no
// concede nada, así que un fallo de lectura no puede parecer una aprobación.
func addonCapabilityItems(db *sql.DB, code string, caps *ergoaddon.Capabilities) []addonCapability {
	items := addonDeclaredCapabilities(caps)
	decisions, err := addonCapabilityDecisions(db, code)
	for i := range items {
		if err != nil {
			items[i].State = addonCapPending
			continue
		}
		d, ok := decisions[addonCapabilityKey(items[i].Kind, items[i].Code)]
		switch {
		case !ok:
			items[i].State = addonCapPending
		case d.Approved:
			items[i].State = addonCapApproved
			items[i].DecidedBy = d.By
		default:
			items[i].State = addonCapRejected
			items[i].DecidedBy = d.By
		}
	}
	return items
}

// addonPendingCapabilities devuelve las declaradas sin decisión.
func addonPendingCapabilities(db *sql.DB, code string, caps *ergoaddon.Capabilities) ([]addonCapability, error) {
	if _, err := addonCapabilityDecisions(db, code); err != nil {
		return nil, err
	}
	var pending []addonCapability
	for _, item := range addonCapabilityItems(db, code, caps) {
		if item.State == addonCapPending {
			pending = append(pending, item)
		}
	}
	return pending, nil
}

// addonEffectiveCapabilities devuelve lo que el addon puede hacer DE VERDAD:
// lo declarado que además fue aprobado. Un addon sin aprobaciones recibe un
// conjunto vacío, no el del manifiesto.
func addonEffectiveCapabilities(db *sql.DB, code string, caps *ergoaddon.Capabilities) *ergoaddon.Capabilities {
	if caps == nil {
		return nil
	}
	decisions, err := addonCapabilityDecisions(db, code)
	approved := func(kind, value string) bool {
		if err != nil {
			return false
		}
		return decisions[addonCapabilityKey(kind, value)].Approved
	}

	out := &ergoaddon.Capabilities{}
	for _, r := range caps.Resources {
		if approved(addonCapResource, r) {
			out.Resources = append(out.Resources, r)
		}
	}
	for _, c := range caps.Commands {
		if approved(addonCapCommand, c) {
			out.Commands = append(out.Commands, c)
		}
	}
	for _, e := range caps.Events {
		if approved(addonCapEvent, e) {
			out.Events = append(out.Events, e)
		}
	}
	if caps.SaleGuard != nil && approved(addonCapGuard, addonCapGuard) {
		out.SaleGuard = caps.SaleGuard
	}
	if caps.Notify != nil {
		for _, m := range caps.Notify.Modules {
			if approved(addonCapNotify, m) {
				if out.Notify == nil {
					out.Notify = &ergoaddon.NotifyCapability{}
				}
				out.Notify.Modules = append(out.Notify.Modules, m)
			}
		}
	}
	return out
}

// addonEffectiveCapabilitiesFor lee el manifiesto guardado en la base y aplica
// las aprobaciones. Es el camino que usan el guard y los eventos, que parten de
// una fila de `addons`, no de un manifiesto en memoria.
func addonEffectiveCapabilitiesFor(db *sql.DB, code string, manifest *ergoaddon.Manifest) *ergoaddon.Capabilities {
	if manifest == nil {
		return nil
	}
	return addonEffectiveCapabilities(db, code, manifest.Capabilities)
}

// addonRecordCapabilityDecision escribe una decisión.
//
// No usa ON DUPLICATE KEY UPDATE ni INSERT OR REPLACE porque el mismo código
// tiene que correr en MySQL (producción) y en SQLite (pruebas). Se intenta el
// UPDATE; si no tocó filas se inserta; y si el INSERT choca —una carrera entre
// dos administradores, o un UPDATE que no cambió nada— se reintenta el UPDATE.
func addonRecordCapabilityDecision(tx *sql.Tx, code, kind, capCode string, approved bool, by string) error {
	flag := 0
	if approved {
		flag = 1
	}
	exec := func(query string, args ...interface{}) (int64, error) {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	n, err := exec(`UPDATE addon_capability_approvals SET is_approved = ?, decided_by = ?
		WHERE addon_code = ? AND kind = ? AND code = ?`, flag, by, code, kind, capCode)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := exec(`INSERT INTO addon_capability_approvals (addon_code, kind, code, is_approved, decided_by)
		VALUES (?, ?, ?, ?, ?)`, code, kind, capCode, flag, by); err == nil {
		return nil
	}
	// El INSERT falló casi con seguridad porque la fila ya existía y el UPDATE
	// no cambió ningún valor (MySQL devuelve 0 filas afectadas en ese caso).
	if _, retry := exec(`UPDATE addon_capability_approvals SET is_approved = ?, decided_by = ?
		WHERE addon_code = ? AND kind = ? AND code = ?`, flag, by, code, kind, capCode); retry != nil {
		return retry
	}
	return nil
}

// addonLoadCapabilities lee las capabilities del manifiesto guardado en `addons`.
func addonLoadCapabilities(db *sql.DB, code string) (*ergoaddon.Capabilities, string, error) {
	var manifestJSON string
	if err := db.QueryRow(`SELECT IFNULL(manifest_json, '') FROM addons WHERE id = ?`, code).Scan(&manifestJSON); err != nil {
		return nil, "", err
	}
	var manifest ergoaddon.Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, "", fmt.Errorf("manifiesto guardado ilegible: %w", err)
	}
	if err := ergoaddon.ValidateCapabilities(manifest.Capabilities); err != nil {
		return nil, "", err
	}
	return manifest.Capabilities, manifestJSON, nil
}

// --- Handlers ---------------------------------------------------------------

// addonSaveCapabilities guarda las decisiones de aprobación de un addon.
//
// Solo se aceptan capabilities QUE EL MANIFIESTO DECLARA: aprobar algo que el
// addon no pidió sería crear un permiso de la nada, y una errata del cliente
// quedaría como una concesión fantasma en la tabla.
func (s *Server) addonSaveCapabilities(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	code := r.PathValue("code")
	if code == "" || !addonCodePattern.MatchString(code) {
		jsonError(w, "Código de addon inválido", http.StatusBadRequest)
		return
	}

	db := s.authDB(r)
	caps, _, err := addonLoadCapabilities(db, code)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, "Addon no encontrado", http.StatusNotFound)
		return
	}
	if err != nil {
		// El addon existe pero su manifiesto guardado no se puede leer: es un
		// problema de datos, no una petición mal hecha. Sin capabilities no se
		// puede aprobar nada y la pantalla debe decirlo.
		jsonError(w, "Manifiesto guardado ilegible: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var body struct {
		Decisions []struct {
			Kind      string `json:"kind"`
			Code      string `json:"code"`
			IsAllowed bool   `json:"is_approved"`
		} `json:"decisions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		jsonError(w, "Datos inválidos", http.StatusBadRequest)
		return
	}

	declared := map[string]bool{}
	for _, item := range addonDeclaredCapabilities(caps) {
		declared[addonCapabilityKey(item.Kind, item.Code)] = true
	}
	var unknown []string
	for _, d := range body.Decisions {
		if !declared[addonCapabilityKey(d.Kind, d.Code)] {
			unknown = append(unknown, addonCapabilityKey(d.Kind, d.Code))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		jsonError(w, "El manifiesto no declara: "+strings.Join(unknown, ", "), http.StatusBadRequest)
		return
	}

	user := ""
	if sess := s.sessionFromRequest(r); sess != nil {
		user = sess.Username
	}

	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for _, d := range body.Decisions {
		if err := addonRecordCapabilityDecision(tx, code, d.Kind, d.Code, d.IsAllowed, user); err != nil {
			jsonError(w, "Error guardando la decisión: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pending, err := addonPendingCapabilities(db, code, caps)
	if err != nil {
		jsonError(w, "Decisión guardada, pero no se pudo releer el estado: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":       1,
		"msg":          "Capabilities guardadas",
		"code":         code,
		"capabilities": addonCapabilityItems(db, code, caps),
		"pending":      pending,
	})
}

// addonActivate activa un addon ya instalado, con dos condiciones: el paquete
// sigue verificando y no queda ninguna capability sin decidir.
//
// Se re-verifica la firma aquí y no se confía en la fila de la base. Activar es
// el momento en el que el proceso empieza a correr, y el directorio del addon
// pudo cambiar después de la instalación: el manifiesto aprobado es el del
// paquete que se va a ejecutar, no el que un día se guardó.
func (s *Server) addonActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	code := r.PathValue("code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}
	if code == "" || !addonCodePattern.MatchString(code) {
		jsonError(w, "Código de addon inválido", http.StatusBadRequest)
		return
	}

	dir := filepath.Join(addonMgr.basePath, code)
	manifest, err := ergoaddon.VerifyPackage(dir, getAddonPublicKey())
	if err != nil {
		jsonError(w, "Verificación de paquete fallida: "+err.Error(), http.StatusNotAcceptable)
		return
	}
	if len(manifest.Migrations) > 0 {
		jsonError(w, "Las migraciones de addons están deshabilitadas hasta contar con firma de paquete verificada", http.StatusNotImplemented)
		return
	}

	db := s.authDB(r)
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM addons WHERE id = ?`, code).Scan(&exists); err != nil || exists == 0 {
		jsonError(w, "Instale el addon antes de activarlo", http.StatusNotFound)
		return
	}

	pending, err := addonPendingCapabilities(db, code, manifest.Capabilities)
	if err != nil {
		jsonError(w, "No se pudo revisar las capabilities: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(pending) > 0 {
		_, _ = db.Exec(`UPDATE addons SET is_active = 0, status = 'pending_caps' WHERE id = ?`, code)
		writeJSONStatus(w, http.StatusConflict, map[string]interface{}{
			"status":  0,
			"code":    code,
			"msg":     "El addon declara capabilities sin decidir. Apruébelas o recházelas antes de activarlo.",
			"pending": pending,
		})
		return
	}

	manifestBytes, err := os.ReadFile(filepath.Join(dir, "module.json"))
	if err != nil {
		jsonError(w, "Error leyendo manifiesto: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := db.Exec(`UPDATE addons SET manifest_json = ?, is_active = 1, needs_migration = 0,
		status = 'active', last_error = NULL WHERE id = ?`, string(manifestBytes), code); err != nil {
		jsonError(w, "Error activando addon: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := db.Exec(`UPDATE menu SET status = 1 WHERE module_id = ?`, code); err != nil {
		jsonError(w, "Error mostrando los menús del addon: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := addonMgr.start(code); err != nil {
		// Un addon marcado activo que no arranca es peor que uno inactivo: si
		// declara fail_closed, cada venta se bloquearía por un proceso muerto y
		// la caja culparía al addon equivocado. Se deja constancia, se ocultan
		// sus menús y se vuelve atrás.
		_, _ = db.Exec(`UPDATE menu SET status = 0 WHERE module_id = ?`, code)
		_, _ = db.Exec(`UPDATE addons SET is_active = 0, status = 'error', last_error = ? WHERE id = ?`,
			err.Error(), code)
		jsonError(w, "No se pudo arrancar el addon: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  1,
		"msg":     "Addon activado",
		"code":    code,
		"version": manifest.Version,
	})
}
