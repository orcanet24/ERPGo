package web

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/orcanet/ergoaddon"
)

// addonProcess representa un proceso de addon corriendo
type addonProcess struct {
	Code       string
	Name       string
	Version    string
	Cmd        *exec.Cmd
	PID        int
	HTTPPort   int
	GRPCPort   int
	Token      string
	StartedAt  time.Time
	RestartCnt int
	LastError  string
}

// addonManager maneja los procesos de addons y su ciclo de vida
type addonManager struct {
	mu          sync.RWMutex
	process     map[string]*addonProcess
	basePath    string
	maxRestarts int
}

var addonMgr = &addonManager{
	process:     make(map[string]*addonProcess),
	basePath:    "addons",
	maxRestarts: 3,
}

// addonPublicKey es la clave de verificación de firma de addons (thread-safe).
// Se configura por ERPGO_ADDON_PUBLIC_KEY (base64) o ERPGO_ADDON_PUBLIC_KEY_FILE.
var (
	addonPublicKeyMu sync.RWMutex
	addonPublicKey   ed25519.PublicKey
)

// configurePublicKey carga la clave pública desde env o archivo
func configurePublicKey() {
	loadKey := func(b64 string) ed25519.PublicKey {
		key, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil
		}
		return key
	}

	if keyB64 := os.Getenv("ERPGO_ADDON_PUBLIC_KEY"); keyB64 != "" {
		if key := loadKey(keyB64); key != nil {
			addonPublicKeyMu.Lock()
			addonPublicKey = key
			addonPublicKeyMu.Unlock()
		}
	}
	if pathStr := os.Getenv("ERPGO_ADDON_PUBLIC_KEY_FILE"); pathStr != "" {
		if data, err := os.ReadFile(pathStr); err == nil {
			if key := loadKey(strings.TrimSpace(string(data))); key != nil {
				addonPublicKeyMu.Lock()
				addonPublicKey = key
				addonPublicKeyMu.Unlock()
			}
		}
	}
}

// getAddonPublicKey devuelve la clave pública configurada (thread-safe)
func getAddonPublicKey() ed25519.PublicKey {
	addonPublicKeyMu.RLock()
	defer addonPublicKeyMu.RUnlock()
	return addonPublicKey
}

func init() {
	configurePublicKey()
}

// addonList lista los addons instalados
func (s *Server) addonList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	var addons []map[string]interface{}
	rows, err := s.authDB(r).Query(`
		SELECT id, name, version, IFNULL(description,''), IFNULL(is_active,0), IFNULL(needs_migration,0), created_at
		FROM addons ORDER BY name`)
	if err != nil {
		writeJSON(w, map[string]interface{}{"data": addons})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, version, description string
		var isActive, needsMigration int
		var createdAt time.Time
		_ = rows.Scan(&id, &name, &version, &description, &isActive, &needsMigration, &createdAt)
		addonMgr.mu.RLock()
		_, running := addonMgr.process[id]
		addonMgr.mu.RUnlock()
		addons = append(addons, map[string]interface{}{
			"code":            id,
			"name":            name,
			"version":         version,
			"description":     description,
			"installed":       isActive == 1,
			"running":         running,
			"needs_migration": needsMigration == 1,
			"created_at":      createdAt.Format("2006-01-02 15:04:05"),
		})
	}
	writeJSON(w, map[string]interface{}{"data": addons})
}

// addonInfo muestra información de un addon
func (s *Server) addonInfo(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	code := r.PathValue("code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}
	if code == "" {
		jsonError(w, "code requerido", 400)
		return
	}
	var name, desc, version, status string
	var isActive, needsMigration int
	var createdAt time.Time
	var lastError string
	err := s.authDB(r).QueryRow(
		"SELECT name, IFNULL(description,''), version, is_active, IFNULL(needs_migration,0), created_at, IFNULL(status,''), IFNULL(last_error,'') FROM addons WHERE id = ?",
		code).Scan(&name, &desc, &version, &isActive, &needsMigration, &createdAt, &status, &lastError)
	if err != nil {
		jsonError(w, "Addon no encontrado", 404)
		return
	}
	addonMgr.mu.RLock()
	proc, running := addonMgr.process[code]
	addonMgr.mu.RUnlock()
	result := map[string]interface{}{
		"code":            code,
		"name":            name,
		"description":     desc,
		"version":         version,
		"installed":       isActive == 1,
		"status":          status,
		"running":         running,
		"needs_migration": needsMigration == 1,
		"created_at":      createdAt.Format("2006-01-02 15:04:05"),
		"last_error":      lastError,
	}
	if running && proc != nil {
		result["pid"] = proc.PID
		result["http_port"] = proc.HTTPPort
		result["grpc_port"] = proc.GRPCPort
		result["restart_count"] = proc.RestartCnt
	}
	writeJSON(w, result)
}

// addonDiscover escanea addons/ y devuelve manifiestos válidos (API 1)
func (s *Server) addonDiscover(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	discoveries := []map[string]interface{}{}

	entries, err := os.ReadDir(addonMgr.basePath)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"status": 1,
			"data":   discoveries,
			"msg":    "Directorio addons/ no existe",
		})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		code := entry.Name()
		if !addonCodePattern.MatchString(code) {
			continue
		}

		dir := filepath.Join(addonMgr.basePath, code)
		manifest, err := ergoaddon.VerifyPackage(dir, getAddonPublicKey())
		if err != nil {
			discoveries = append(discoveries, map[string]interface{}{
				"code":   code,
				"status": "invalid",
				"error":  err.Error(),
			})
			continue
		}
		discoveries = append(discoveries, map[string]interface{}{
			"code":        manifest.Code,
			"name":        manifest.Name,
			"version":     manifest.Version,
			"description": manifest.Description,
			"author":      "",
			"web":         manifest.Web != "",
		})
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"data":   discoveries,
	})
}

// addonInstall instala un addon con verificación de firma
func (s *Server) addonInstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	code := r.PathValue("code")
	if code == "" {
		jsonError(w, "Código de addon inválido", 400)
		return
	}

	dir := filepath.Join(addonMgr.basePath, code)

	// 1. Verificar paquete (firma + hash) ANTES de cualquier registro
	manifest, err := ergoaddon.VerifyPackage(dir, getAddonPublicKey())
	if err != nil {
		jsonError(w, "Verificación de paquete fallida: "+err.Error(), http.StatusNotAcceptable)
		return
	}

	// 2. Rechazar migraciones hasta que el addon use el contrato SDK
	if len(manifest.Migrations) > 0 {
		jsonError(w, "Las migraciones de addons están deshabilitadas hasta contar con firma de paquete verificada", http.StatusNotImplemented)
		return
	}

	permissionJSON, _ := json.Marshal(manifest.Permissions)

	// 3. Leer bytes originales del manifiesto para guardar en BD
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "module.json"))
	if err != nil {
		jsonError(w, "Error leyendo manifiesto: "+err.Error(), 500)
		return
	}
	manifestJSON := string(manifestBytes)

	// 4. Registrar en BD
	authorName := ""
	authorEmail := ""
	if manifest.Author != nil {
		authorName = manifest.Author.Name
		authorEmail = manifest.Author.Email
	}
	_, err = s.authDB(r).Exec(`
		INSERT INTO addons (id, name, version, description, author_name, author_email, license_type, manifest_json, addon_path, entrypoint, is_active, web, grpc, needs_migration, permissions_json, installed_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, 1, ?, NOW(), 'pending')
		ON DUPLICATE KEY UPDATE name=VALUES(name), version=VALUES(version), description=VALUES(description), manifest_json=VALUES(manifest_json), permissions_json=VALUES(permissions_json), is_active=0, status='pending', needs_migration=1, updated_at=NOW()`,
		code, manifest.Name, manifest.Version, manifest.Description,
		authorName, authorEmail, manifest.License, manifestJSON,
		code, manifest.Entry, manifest.Web != "", manifest.GRPC, string(permissionJSON),
	)
	if err != nil {
		jsonError(w, "Error registrando addon: "+err.Error(), 500)
		return
	}

	// 4.b Capacidades: el manifiesto dice lo que el addon PIDE; sin decisión del
	// administrador no hay activación. Un addon que no declara capabilities
	// (hello) se activa como siempre: no hay nada que aprobar.
	pending, err := addonPendingCapabilities(s.authDB(r), code, manifest.Capabilities)
	if err != nil {
		jsonError(w, "No se pudo revisar las capabilities del addon: "+err.Error(), http.StatusInternalServerError)
		return
	}
	activar := len(pending) == 0
	// Los menús nacen ocultos si el addon todavía no está aprobado: un enlace
	// que lleva a un proceso que no corre es una pantalla en blanco con el
	// nombre del módulo encima.
	menuStatus := 0
	if activar {
		menuStatus = 1
	}

	// 4. Registrar menús
	if len(manifest.Menus) > 0 {
		tx, err := s.authDB(r).Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.Exec("UPDATE menu SET status = 0 WHERE module_id = ?", code); err != nil {
			jsonError(w, "Error ocultando menús antiguos: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, menu := range manifest.Menus {
			parentID := 0
			if menu.Parent != "" {
				_ = tx.QueryRow("SELECT id FROM menu WHERE label = ? AND status = 1 LIMIT 1", menu.Parent).Scan(&parentID)
			}
			var maxID int
			_ = tx.QueryRow("SELECT COALESCE(MAX(id), 1000) + 1 FROM menu").Scan(&maxID)
			_, err = tx.Exec(`
				INSERT INTO menu (id, parent_id, label, url, module_id, is_core, icon, sort_order, status, perm)
				VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
				maxID, parentID, menu.Label, menu.URL, code, menu.Icon, menu.SortOrder, menuStatus, menu.Permission)
			if err != nil {
				jsonError(w, "Error registrando menú: "+err.Error(), 500)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// 5. Sin capabilities pendientes, activar. Con pendientes, el addon queda
	// instalado pero inerte: ni proceso ni menús hasta que alguien decida. La
	// respuesta lo dice con un 202 y la lista, para que la pantalla pueda
	// pintarla sin una segunda petición.
	if !activar {
		if _, err := s.authDB(r).Exec("UPDATE addons SET is_active = 0, needs_migration = 0, status = 'pending_caps' WHERE id = ?", code); err != nil {
			jsonError(w, "Error registrando addon: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONStatus(w, http.StatusAccepted, map[string]interface{}{
			"status":  1,
			"msg":     "Addon registrado. Revise y apruebe sus capabilities para activarlo.",
			"code":    code,
			"menus":   len(manifest.Menus),
			"version": manifest.Version,
			"active":  false,
			"pending": pending,
		})
		return
	}

	if _, err := s.authDB(r).Exec("UPDATE addons SET is_active = 1, needs_migration = 0, status = 'active' WHERE id = ?", code); err != nil {
		jsonError(w, "Error activando addon: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Si el addon es exclusivo en su categoría, desactivar cualquier otro addon activo en esa misma categoría
	var cat string
	var exclusive int
	if err := s.authDB(r).QueryRow("SELECT category, exclusive_in_category FROM addons WHERE id = ?", code).Scan(&cat, &exclusive); err == nil && exclusive == 1 && cat != "" {
		if _, err := s.authDB(r).Exec("UPDATE addons SET is_active = 0, status = 'disabled', updated_at = NOW() WHERE id != ? AND category = ? AND is_active = 1", code, cat); err == nil {
			log.Printf("Addons en categoría '%s' desactivados por exclusividad (activado '%s')", cat, code)
		}
	}

	// 6. Spawn del proceso addon
	spawnErr := addonMgr.start(code)

	writeJSON(w, map[string]interface{}{
		"status":  1,
		"msg":     "Addon instalado correctamente",
		"code":    code,
		"menus":   len(manifest.Menus),
		"version": manifest.Version,
		"active":  true,
		"spawn":   spawnErr == nil,
		"spawn_error": func() string {
			if spawnErr != nil {
				return spawnErr.Error()
			}
			return ""
		}(),
	})
}

// addonUninstall desinstala un addon y detiene su proceso
func (s *Server) addonUninstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	code := r.PathValue("code")
	if code == "" {
		jsonError(w, "Código inválido", 400)
		return
	}

	if !addonCodePattern.MatchString(code) || strings.HasPrefix(code, "core_") {
		jsonError(w, "No se puede desinstalar este código de módulo", 403)
		return
	}

	tx, err := s.authDB(r).Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.Exec("UPDATE menu SET status = 0 WHERE module_id = ?", code)
	_, _ = tx.Exec("UPDATE addons SET is_active = 0, status = 'disabled' WHERE id = ?", code)

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	addonMgr.stop(code)

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Addon desinstalado",
		"code":   code,
	})
}

// addonUpdate actualiza un addon (migraciones + versión)
func (s *Server) addonUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	code := r.PathValue("code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}
	if code == "" {
		jsonError(w, "code requerido", 400)
		return
	}
	if !addonCodePattern.MatchString(code) {
		jsonError(w, "Código de addon inválido", http.StatusBadRequest)
		return
	}

	var currentVersion string
	err := s.authDB(r).QueryRow("SELECT version FROM addons WHERE id = ?", code).Scan(&currentVersion)
	if err != nil {
		jsonError(w, "Addon no encontrado", 404)
		return
	}

	dir := filepath.Join(addonMgr.basePath, code)
	manifest, err := ergoaddon.VerifyPackage(dir, getAddonPublicKey())
	if err != nil {
		jsonError(w, "Verificación de paquete fallida: "+err.Error(), http.StatusNotAcceptable)
		return
	}

	if currentVersion == manifest.Version {
		writeJSON(w, map[string]interface{}{
			"status": 1,
			"msg":    "El addon ya está actualizado a la versión " + currentVersion,
			"code":   code,
		})
		return
	}
	if len(manifest.Migrations) > 0 {
		jsonError(w, "Las migraciones de addons están deshabilitadas hasta contar con firma de paquete verificada", http.StatusNotImplemented)
		return
	}

	if _, err := s.authDB(r).Exec("UPDATE menu SET status = 0 WHERE module_id = ?", code); err != nil {
		jsonError(w, "Error desactivando menús antiguos: "+err.Error(), http.StatusInternalServerError)
		return
	}

	permissionJSON, _ := json.Marshal(manifest.Permissions)
	manifestBytes, err := os.ReadFile(filepath.Join(addonMgr.basePath, code, "module.json"))
	if err != nil {
		jsonError(w, "Error leyendo manifiesto: "+err.Error(), 500)
		return
	}
	if _, err := s.authDB(r).Exec("UPDATE addons SET version = ?, manifest_json = ?, permissions_json = ? WHERE id = ?", manifest.Version, string(manifestBytes), string(permissionJSON), code); err != nil {
		jsonError(w, "Error actualizando addon: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(manifest.Menus) > 0 {
		tx, err := s.authDB(r).Begin()
		if err != nil {
			jsonError(w, "Error registrando menús nuevos: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, menu := range manifest.Menus {
			var maxID int
			if err := tx.QueryRow("SELECT COALESCE(MAX(id), 1000) + 1 FROM menu").Scan(&maxID); err != nil {
				_ = tx.Rollback()
				jsonError(w, "Error reservando id de menú: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if _, err := tx.Exec(`INSERT INTO menu (id, parent_id, label, url, module_id, is_core, icon, sort_order, status, perm)
				VALUES (?, NULL, ?, ?, ?, 0, ?, ?, 0, ?)`, maxID, menu.Label, menu.URL, code, menu.Icon, menu.SortOrder, menu.Permission); err != nil {
				_ = tx.Rollback()
				jsonError(w, "Error registrando menú nuevo: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, "Error confirmando menús nuevos: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Una versión nueva puede traer capabilities nuevas. Si las trae, el addon se
	// queda apagado hasta que alguien las decida: activar lo que nadie revisó es
	// exactamente lo que este paso existe para evitar. Las capabilities ya
	// aprobadas siguen aprobadas; solo las nuevas quedan pendientes.
	pending, pendingErr := addonPendingCapabilities(s.authDB(r), code, manifest.Capabilities)
	if pendingErr != nil {
		jsonError(w, "No se pudo revisar las capabilities del addon: "+pendingErr.Error(), http.StatusInternalServerError)
		return
	}
	if len(pending) > 0 {
		if _, err := s.authDB(r).Exec(`UPDATE menu SET status = 0 WHERE module_id = ?`, code); err != nil {
			jsonError(w, "Error ocultando los menús del addon: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := s.authDB(r).Exec("UPDATE addons SET is_active = 0, status = 'pending_caps' WHERE id = ?", code); err != nil {
			jsonError(w, "Error desactivando el addon: "+err.Error(), http.StatusInternalServerError)
			return
		}
		addonMgr.stop(code)
		writeJSONStatus(w, http.StatusAccepted, map[string]interface{}{
			"status":      1,
			"msg":         "Addon actualizado. Las capabilities nuevas necesitan aprobación antes de volver a activarlo.",
			"code":        code,
			"old_version": currentVersion,
			"new_version": manifest.Version,
			"active":      false,
			"pending":     pending,
		})
		return
	}

	if _, err := s.authDB(r).Exec("UPDATE addons SET is_active = 1, status = 'active' WHERE id = ?", code); err != nil {
		jsonError(w, "Error activando versión nueva: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := s.authDB(r).Exec(`UPDATE menu SET status = 1 WHERE module_id = ?`, code); err != nil {
		jsonError(w, "Error mostrando los menús del addon: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reiniciar proceso si estaba corriendo para reflejar la nueva versión
	addonMgr.restartIfRunning(code)

	writeJSON(w, map[string]interface{}{
		"status":      1,
		"msg":         "Addon actualizado de " + currentVersion + " a " + manifest.Version,
		"code":        code,
		"old_version": currentVersion,
		"new_version": manifest.Version,
	})
}

// addonProxy proxy inverso con handshake obligatorio
func (s *Server) addonProxy(w http.ResponseWriter, r *http.Request) {
	if s.sessionFromRequest(r) == nil {
		http.Error(w, "No autorizado", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	pathVal := strings.TrimPrefix(r.URL.Path, "/mod/")
	parts := strings.SplitN(pathVal, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	code := parts[0]
	if !addonCodePattern.MatchString(code) {
		http.NotFound(w, r)
		return
	}

	var isActive int
	err := s.authDB(r).QueryRow("SELECT is_active FROM addons WHERE id = ?", code).Scan(&isActive)
	if err != nil || isActive != 1 {
		http.Error(w, "Addon desactivado o no encontrado", 404)
		return
	}

	sess := s.sessionFromRequest(r)
	permission, found, err := addonRoutePermission(s.authDB(r), code, r.URL.Path)
	if err != nil || !found {
		http.NotFound(w, r)
		return
	}
	if permission != "" {
		required := permissionForAction(permission, actionFromMethod(r.Method))
		if required == "" || !s.hasAddonPermission(r, sess, required, actionFromMethod(r.Method)) {
			http.Error(w, "Permiso insuficiente para el addon", http.StatusForbidden)
			return
		}
	}
	if permission == "" {
		if actionFromMethod(r.Method) != "read" {
			http.Error(w, "La ruta del addon no declara permiso de escritura", http.StatusForbidden)
			return
		}
		if sess.UserLevel != "master" && sess.UserLevel != "admin" {
			http.Error(w, "La ruta del addon no declara permiso de lectura", http.StatusForbidden)
			return
		}
	}

	addonMgr.mu.RLock()
	proc, running := addonMgr.process[code]
	addonMgr.mu.RUnlock()

	// Handshake gRPC antes de proxyar: verifica identidad del proceso
	if running && proc != nil && proc.GRPCPort > 0 {
		ok, handshakeErr := addonHandshakeCheck(proc)
		if !ok {
			http.Error(w, "Addon no responde handshake: "+handshakeErr, http.StatusBadGateway)
			return
		}
	}

	if running && proc != nil && proc.HTTPPort > 0 {
		target := fmt.Sprintf("http://127.0.0.1:%d", proc.HTTPPort)
		proxyURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "Endpoint local del addon inválido", http.StatusBadGateway)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(proxyURL)
		originalDirector := proxy.Director
		relativePath := ""
		if len(parts) == 2 {
			relativePath = strings.TrimPrefix(parts[1], "/")
		}
		proxy.Director = func(out *http.Request) {
			originalDirector(out)
			out.URL.Path = "/" + relativePath
			out.URL.RawPath = ""
			out.Host = proxyURL.Host
			out.Header.Del("X-ERPGo-User-ID")
			out.Header.Del("X-ERPGo-User-Name")
			out.Header.Del("X-ERPGo-User-Level")
			out.Header.Del("X-ERPGo-Tenant")
			out.Header.Set("X-ERPGo-User-ID", sess.UserID)
			out.Header.Set("X-ERPGo-User-Name", sess.Username)
			out.Header.Set("X-ERPGo-User-Level", sess.UserLevel)
			out.Header.Set("X-ERPGo-Tenant", s.tenantOfSession(sess))
		}
		proxy.Transport = &addonTransport{token: proc.Token, code: code}
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Set("X-ERPGo-Addon", code)
			return nil
		}
		proxy.ServeHTTP(w, r)
		return
	}

	if pathVal == "" || pathVal == code+"/" || pathVal == code {
		s.renderAddonTemplate(w, r, code)
		return
	}

	addonServeStatic(w, r, code, pathVal)
}

// addonHandshakeCheck verifica la identidad del proceso via gRPC Hello
func addonHandshakeCheck(proc *addonProcess) (bool, string) {
	if proc.GRPCPort <= 0 {
		return true, ""
	}
	addr := fmt.Sprintf("127.0.0.1:%d", proc.GRPCPort)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := ergoaddon.DialAddon(ctx, addr, proc.Token)
	if err != nil {
		return false, "dial: " + err.Error()
	}
	defer client.Close()

	resp, err := client.Hello(ctx, &ergoaddon.HelloRequest{
		Token:   proc.Token,
		Code:    proc.Code,
		Version: proc.Version,
	})
	if err != nil {
		return false, "hello: " + err.Error()
	}
	if resp == nil || !resp.Ok {
		return false, "handshake rechazado"
	}
	if resp.AddonName != proc.Code {
		return false, "identity mismatch"
	}
	return true, ""
}

// addonTransport inyecciona headers de autenticación
type addonTransport struct {
	token string
	code  string
}

func (t *addonTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.Header = req.Header.Clone()
	out.Header.Set("X-ERPGo-Token", t.token)
	out.Header.Set("X-ERPGo-Addon", t.code)
	return (&http.Client{Timeout: 30 * time.Second}).Do(out)
}

// addonServeStatic sirve archivos estáticos del addon
func addonServeStatic(w http.ResponseWriter, r *http.Request, code, pathVal string) {
	clean := filepath.Clean(filepath.FromSlash(pathVal))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	base := filepath.Join(addonMgr.basePath, code, "web")
	fullPath := filepath.Join(base, clean)
	rel, err := filepath.Rel(base, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, fullPath)
}

// renderAddonTemplate renderiza un template del addon con el layout del maestro
func (s *Server) renderAddonTemplate(w http.ResponseWriter, r *http.Request, code string) {
	templatePaths := []string{
		filepath.Join("web", code, "templates", "index.html"),
		filepath.Join("web", code, "index.html"),
		filepath.Join("addons", code, "web", "templates", "index.html"),
		filepath.Join("addons", code, "web", "index.html"),
	}

	var tmplPath string
	for _, p := range templatePaths {
		if _, err := os.Stat(p); err == nil {
			tmplPath = p
			break
		}
	}

	if tmplPath == "" {
		var name, description string
		_ = s.authDB(r).QueryRow("SELECT name, IFNULL(description,'') FROM addons WHERE id = ?", code).Scan(&name, &description)
		s.render(w, r, "addon_info.html", map[string]interface{}{
			"Title":       "Addon: " + code,
			"Code":        code,
			"Name":        name,
			"Description": description,
			"Message":     "Este addon no tiene interfaz web todavía.",
		})
		return
	}

	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, "Error de template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sess := s.sessionFromRequest(r)
	data := map[string]interface{}{
		"Title":   "Addon: " + code,
		"Code":    code,
		"IsAdmin": sess != nil && (sess.UserLevel == "admin" || sess.UserLevel == "master"),
		"User": map[string]interface{}{
			"ID":    "",
			"Name":  "",
			"Level": "",
		},
	}
	if sess != nil {
		data["User"].(map[string]interface{})["ID"] = sess.UserID
		data["User"].(map[string]interface{})["Name"] = sess.Username
		data["User"].(map[string]interface{})["Level"] = sess.UserLevel
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, "Error ejecutando template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.render(w, r, "addon_wrapper.html", map[string]interface{}{
		"Title":   "Addon: " + code,
		"Code":    code,
		"Content": template.HTML(buf.String()),
	})
}

var addonCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

// addonRoutePermission resolves the declared menu entry owning a request path.
func addonRoutePermission(db *sql.DB, code, requestPath string) (string, bool, error) {
	var raw string
	if err := db.QueryRow("SELECT manifest_json FROM addons WHERE id = ? AND is_active = 1", code).Scan(&raw); err != nil {
		return "", false, err
	}
	var manifest ergoaddon.Manifest
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil || manifest.Code != code {
		if err == nil {
			err = fmt.Errorf("manifest code mismatch")
		}
		return "", false, err
	}
	requestPath = path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	bestLen := -1
	permission := ""
	for _, menu := range manifest.Menus {
		menuPath := path.Clean("/" + strings.TrimPrefix(menu.URL, "/"))
		if menuPath == "/" || !strings.HasPrefix(menuPath+"/", "/mod/"+code+"/") || strings.TrimRight(menuPath, "/") != strings.TrimRight(menu.URL, "/") {
			continue
		}
		if requestPath == menuPath || strings.HasPrefix(requestPath, strings.TrimSuffix(menuPath, "/")+"/") {
			if len(menuPath) > bestLen {
				bestLen = len(menuPath)
				permission = menu.Permission
			}
		}
	}
	return permission, bestLen >= 0, nil
}

func permissionForAction(base, action string) string {
	if action == "read" {
		return base
	}
	base = strings.TrimSuffix(base, "_read")
	switch action {
	case "create", "update", "delete":
		return base + "_" + action
	default:
		return ""
	}
}

func permissionSupportsAction(permission, action string) bool {
	switch action {
	case "read":
		return strings.HasSuffix(permission, "_read") || permission == "read"
	case "create", "update", "delete":
		return permission == permissionForAction(permission, action)
	default:
		return false
	}
}

// manifestPermissionsJSON genera JSON de permisos desde Manifest
func manifestPermissionsJSON(manifest ergoaddon.Manifest) string {
	data, err := json.Marshal(manifest.Permissions)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (s *Server) hasAddonPermission(r *http.Request, sess *SessionData, permissionCode, action string) bool {
	if sess == nil || permissionCode == "" || action == "" {
		return false
	}
	if sess.UserLevel == "master" || sess.UserLevel == "admin" {
		return true
	}
	db := s.authDB(r)
	if db == nil {
		return false
	}
	rows, err := db.Query(`SELECT arp.permission_code, a.manifest_json
		FROM addon_role_permissions arp
		JOIN sec_userrole ur ON ur.roleid = arp.role_id
		JOIN addons a ON a.id = arp.addon_code AND a.is_active = 1
		WHERE ur.user_id = ? AND arp.permission_code = ? AND arp.is_allowed = 1`, sess.UserID, permissionCode)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var granted, raw string
		if rows.Scan(&granted, &raw) != nil || granted != permissionCode {
			continue
		}
		var manifest ergoaddon.Manifest
		if json.Unmarshal([]byte(raw), &manifest) != nil {
			continue
		}
		for _, declared := range manifest.Permissions {
			permission := declared.Code
			if permission == permissionCode && permissionSupportsAction(permission, action) {
				return true
			}
		}
	}
	return false
}

func (s *Server) requireAddonAdmin(w http.ResponseWriter, r *http.Request) bool {
	sess := s.sessionFromRequest(r)
	if sess != nil && sess.UserLevel == "master" {
		return true
	}
	jsonError(w, "Se requiere rol master para gestionar addons", http.StatusForbidden)
	return false
}

// addonAdminList muestra la vista de administración de addons y sus permisos por rol.
func (s *Server) addonAdminList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	s.render(w, r, "addon_admin.html", map[string]interface{}{
		"Title": "Addons",
	})
}

// addonAdminData devuelve JSON con addons instalados, roles disponibles y permisos.
func (s *Server) addonAdminData(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	db := s.authDB(r)

	// 1. Obtener addons instalados.
	type addonPerm struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}

	// Se leen TODAS las filas antes de consultar las aprobaciones. Hacer la
	// segunda consulta dentro del bucle de la primera falla en SQLite (un
	// resultado abierto no admite otra sentencia en la misma base en memoria) y
	// en MySQL deja un cursor vivo mientras se piden más datos: el estado salía
	// "pendiente" para todo, que es justo el error que esta pantalla no puede
	// permitirse.
	type addonRow struct {
		code, name, version, desc, author, permsJSON, manifestJSON, status string
		isActive                                                           int
	}
	var filas []addonRow
	rows, err := db.Query(`
		SELECT id, name, version, IFNULL(description,''), IFNULL(author_name,''), IFNULL(permissions_json,'[]'),
		       IFNULL(manifest_json,''), IFNULL(status,''), IFNULL(is_active,0)
		FROM addons ORDER BY name`)
	if err == nil {
		for rows.Next() {
			var f addonRow
			if rows.Scan(&f.code, &f.name, &f.version, &f.desc, &f.author, &f.permsJSON,
				&f.manifestJSON, &f.status, &f.isActive) == nil {
				filas = append(filas, f)
			}
		}
		rows.Close()
	}

	addons := []map[string]interface{}{}
	for _, f := range filas {
		var perms []addonPerm
		_ = json.Unmarshal([]byte(f.permsJSON), &perms)
		permList := make([]map[string]interface{}, 0, len(perms))
		for _, p := range perms {
			permList = append(permList, map[string]interface{}{
				"code":   p.Code,
				"reason": p.Reason,
			})
		}

		// Capacidades declaradas + decisión tomada. El estado se calcula aquí y
		// no en JavaScript: la pantalla no debe poder deducir "aprobado" de la
		// ausencia de una fila.
		var manifest ergoaddon.Manifest
		_ = json.Unmarshal([]byte(f.manifestJSON), &manifest)
		caps := addonCapabilityItems(db, f.code, manifest.Capabilities)
		counts := map[string]int{addonCapPending: 0, addonCapApproved: 0, addonCapRejected: 0}
		for _, c := range caps {
			counts[c.State]++
		}

		addons = append(addons, map[string]interface{}{
			"code":             f.code,
			"name":             f.name,
			"version":          f.version,
			"description":      f.desc,
			"author":           f.author,
			"permissions":      permList,
			"capabilities":     caps,
			"pending":          counts[addonCapPending],
			"approved":         counts[addonCapApproved],
			"rejected":         counts[addonCapRejected],
			"active":           f.isActive == 1,
			"status":           f.status,
			"needs_activation": f.isActive != 1 && counts[addonCapPending] == 0,
		})
	}

	// 2. Obtener roles.
	roleRows, err := db.Query(`SELECT id, type, IFNULL(level,'employee') FROM sec_role ORDER BY id`)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"addons":      addons,
			"roles":       []map[string]interface{}{},
			"permissions": []map[string]interface{}{},
		})
		return
	}
	defer roleRows.Close()

	roles := []map[string]interface{}{}
	for roleRows.Next() {
		var id int
		var typ, level string
		if roleRows.Scan(&id, &typ, &level) != nil {
			continue
		}
		roles = append(roles, map[string]interface{}{
			"id":    id,
			"type":  typ,
			"level": level,
		})
	}

	// 3. Obtener permisos existentes de addon_role_permissions.
	permRows, err := db.Query(`
		SELECT addon_code, permission_code, role_id, is_allowed
		FROM addon_role_permissions
		ORDER BY addon_code, permission_code, role_id`)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"addons":      addons,
			"roles":       roles,
			"permissions": []map[string]interface{}{},
		})
		return
	}
	defer permRows.Close()

	permList := []map[string]interface{}{}
	for permRows.Next() {
		var addonCode, permCode string
		var roleID, isAllowed int
		if permRows.Scan(&addonCode, &permCode, &roleID, &isAllowed) != nil {
			continue
		}
		permList = append(permList, map[string]interface{}{
			"addon_code":      addonCode,
			"permission_code": permCode,
			"role_id":         roleID,
			"is_allowed":      isAllowed == 1,
		})
	}

	writeJSON(w, map[string]interface{}{
		"addons":      addons,
		"roles":       roles,
		"permissions": permList,
	})
}

// addonSavePermissions guarda los cambios de permisos por rol para un addon.
func (s *Server) addonSavePermissions(w http.ResponseWriter, r *http.Request) {
	if !s.requireAddonAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	code := r.PathValue("code")
	if code == "" || !addonCodePattern.MatchString(code) {
		jsonError(w, "Código de addon inválido", 400)
		return
	}

	// Verificar que el addon existe.
	db := s.authDB(r)
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM addons WHERE id = ?", code).Scan(&exists); err != nil || exists == 0 {
		jsonError(w, "Addon no encontrado", 404)
		return
	}

	// Parsear body: array de {addon_code, permission_code, role_id, is_allowed}.
	var entries []struct {
		AddonCode      string `json:"addon_code"`
		PermissionCode string `json:"permission_code"`
		RoleID         int    `json:"role_id"`
		IsAllowed      bool   `json:"is_allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		jsonError(w, "Datos inválidos", 400)
		return
	}

	// Obtener los permisos declarados del addon para validar.
	var permsJSON string
	if err := db.QueryRow("SELECT IFNULL(permissions_json,'[]') FROM addons WHERE id = ?", code).Scan(&permsJSON); err != nil {
		jsonError(w, "Error leyendo addon", 500)
		return
	}
	var declaredPerms []struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(permsJSON), &declaredPerms)
	declaredSet := map[string]bool{}
	for _, dp := range declaredPerms {
		declaredSet[dp.Code] = true
	}

	// Validar roles existentes.
	roleRows, err := db.Query(`SELECT id FROM sec_role`)
	if err != nil {
		jsonError(w, "Error leyendo roles", 500)
		return
	}
	validRoles := map[int]bool{}
	for roleRows.Next() {
		var rid int
		if roleRows.Scan(&rid) == nil {
			validRoles[rid] = true
		}
	}
	roleRows.Close()

	// Transacción: borrar permisos existentes del addon e insertar los nuevos.
	tx, err := db.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM addon_role_permissions WHERE addon_code = ?", code); err != nil {
		jsonError(w, "Error limpiando permisos anteriores: "+err.Error(), 500)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO addon_role_permissions (addon_code, permission_code, role_id, is_allowed)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer stmt.Close()

	for _, e := range entries {
		if e.AddonCode != code {
			continue
		}
		if !declaredSet[e.PermissionCode] {
			continue
		}
		if !validRoles[e.RoleID] {
			continue
		}
		allowed := 0
		if e.IsAllowed {
			allowed = 1
		}
		if _, err := stmt.Exec(e.AddonCode, e.PermissionCode, e.RoleID, allowed); err != nil {
			jsonError(w, "Error guardando permisos: "+err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": 1,
		"msg":    "Permisos guardados",
		"code":   code,
	})
}

// registerAddonAdminMenu asegura que el item de menú "Addons" exista en Configuración.
func (s *Server) registerAddonAdminMenu() {
	ctl, err := s.controlStore()
	if err != nil {
		return
	}
	// Usar INSERT IGNORE para idempotencia.
	_, _ = ctl.Exec(`
		INSERT IGNORE INTO menu (id, parent_id, label, url, icon, perm, active_prefix, sort_order, status, module_id, is_core)
		VALUES (280, 21, 'Addons', '/module/addons/admin', 'fa fa-puzzle-piece', 'setting', '/module/addons', 110, 1, 'core_configuracion', 1)
	`)
}

// generateAddonToken genera un token aleatorio
func generateAddonToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// freePort encuentra un puerto libre en loopback
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// start inicia un proceso de addon real con supervisor
func (m *addonManager) start(code string) error {
	if !addonCodePattern.MatchString(code) {
		return fmt.Errorf("invalid addon code")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if p, exists := m.process[code]; exists && p.Cmd != nil && p.Cmd.Process != nil {
		return fmt.Errorf("addon %s ya está corriendo", code)
	}

	dir := filepath.Join(m.basePath, code)
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "module.json"))
	if err != nil {
		return fmt.Errorf("read module.json: %w", err)
	}
	var manifest ergoaddon.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse module.json: %w", err)
	}

	httpPort, err := freePort()
	if err != nil {
		return fmt.Errorf("free port: %w", err)
	}
	grpcPort, err := freePort()
	if err != nil {
		return fmt.Errorf("free grpc port: %w", err)
	}

	token := generateAddonToken()
	binaryPath := filepath.Join(dir, manifest.Entry)

	cmd := exec.Command(binaryPath,
		"--port", fmt.Sprintf("%d", httpPort),
		"--grpc-port", fmt.Sprintf("%d", grpcPort),
		"--token", token,
		"--addon", code,
		"--base", dir,
	)
	cmd.Dir = dir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start addon process: %w", err)
	}

	proc := &addonProcess{
		Code:      code,
		Name:      manifest.Name,
		Version:   manifest.Version,
		Cmd:       cmd,
		PID:       cmd.Process.Pid,
		HTTPPort:  httpPort,
		GRPCPort:  grpcPort,
		Token:     token,
		StartedAt: time.Now(),
	}
	m.process[code] = proc

	// Goroutine de supervisión: espera exit y restart
	go m.supervise(code, proc)

	return nil
}

// supervise espera el exit del proceso y lo reinicia hasta maxRestarts
func (m *addonManager) supervise(code string, proc *addonProcess) {
	err := proc.Cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	current, exists := m.process[code]
	if !exists || current != proc {
		return // alguien ya lo paró manualmente
	}

	if err != nil {
		proc.LastError = err.Error()
	}

	proc.RestartCnt++
	if proc.RestartCnt > m.maxRestarts {
		current.LastError = fmt.Sprintf("max restarts reached (%d), last error: %s", m.maxRestarts, proc.LastError)
		return
	}

	// Reintentar con backoff
	backoff := time.Duration(proc.RestartCnt) * 2 * time.Second
	time.Sleep(backoff)

	m.mu.Unlock()
	retryErr := m.start(code)
	m.mu.Lock()

	if retryErr != nil {
		if current, ok := m.process[code]; ok {
			current.LastError = "restart failed: " + retryErr.Error()
		}
	}
}

// stop detiene un proceso de addon y notifica al supervisor
func (m *addonManager) stop(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, exists := m.process[code]
	if !exists {
		return
	}
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		_ = proc.Cmd.Process.Kill()
	}
	delete(m.process, code)
}

// restartIfRunning reinicia un proceso si estaba corriendo
func (m *addonManager) restartIfRunning(code string) {
	m.mu.RLock()
	_, running := m.process[code]
	m.mu.RUnlock()
	if running {
		m.stop(code)
	}
	_ = m.start(code)
}

// isRunning indica si un addon está corriendo
func (m *addonManager) isRunning(code string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.process[code]
	return ok
}
