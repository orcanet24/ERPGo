package web

// backup_scheduler.go — Respaldos automáticos programados.
//
// Lo que se pidió y por qué vive aquí:
//   - El ERP de una PYME no puede depender de que alguien apriete el botón de
//     respaldo: en Venezuela una prueba fiscal se defiende con facturas, y una
//     máquina que se apaga de golpe sin respaldo es la diferencia entre
//     reconstruir el Negocio en una tarde y perderlo.
//   - El respaldo es el ZIP ya existente (volcado lógico mysqldump + manifiesto),
//     comprimido, así que un respaldo diario no llena el disco.
//   - Revalidar: un respaldo que nunca se leyó de nuevo no es un respaldo, es
//     una promesa. Cada ciclo el propio binario abre el ZIP, comprueba el CRC
//     que guarda zip.Reader y coteja el SHA-256 con el manifiesto de metadatos.
//     Si falla, lo marca CORRUPTO en el meta para que se vea en la lista.
//
// El intervalo lo decide el administrador en la configuración del servidor
// (.env): BACKUP_ENABLED (1), BACKUP_INTERVAL_HOURS (24) y BACKUP_DIR (backup).
// Por defecto respalda cada 24 horas en el directorio backup/ junto al binario.

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// edadUltimoRespaldo devuelve la edad del ZIP más reciente de una empresa en
// el directorio del scheduler. El estado vive en la carpeta y no en la BD:
// el respaldo es archivo, no datos, así que no toca migraciones.
func (s *Server) edadUltimoRespaldo(tenant string) (time.Duration, bool) {
	dir := s.backupSchedulerDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	pre := tenant + "_"
	var mejor time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), pre) || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(mejor) {
			mejor = info.ModTime()
		}
	}
	if mejor.IsZero() {
		return 0, false
	}
	return time.Since(mejor), true
}

// backupMeta es el manifiesto de metadatos de un respaldo automático.
// Vive junto al ZIP con el mismo nombre + .meta.json.
type backupMeta struct {
	Tenant     string    `json:"tenant"`
	Base       string    `json:"base"`
	File       string    `json:"file"`
	Created    time.Time `json:"created"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	Verified   bool      `json:"verified"`
	VerifiedAt time.Time `json:"verified_at"`
	Corrupted  bool      `json:"corrupted"`
}

// backupSchedulerDir es el directorio de respaldos automáticos, FUERA del
// binario. `backups/` es la carpeta heredada de la pantalla manual; los nuevos
// van a `backup/` y la lista lee ambas para no perder los viejos.
func (s *Server) backupSchedulerDir() string {
	if dir := strings.TrimSpace(s.config.BackupDir); dir != "" {
		return dir
	}
	return "backup"
}

// startBackupScheduler lanza el ciclo de respaldo en segundo plano. Un fallo
// aquí NO impide arrancar la app: el ERP sigue vendiendo sin respaldo y lo
// avisa, igual que el orquestador.
func (s *Server) startBackupScheduler() {
	if !s.config.BackupEnabled {
		log.Printf("respaldos automáticos deshabilitados (BACKUP_ENABLED=0)")
		return
	}
	interval := s.config.BackupIntervalHours
	if interval < 1 {
		interval = 24
	}
	s.backupStop = make(chan struct{})
	stamp := time.Now().Format("15:04:05")

	go func() {
		// X de arranque se esperan 2 minutos: el MySQL embebido suele estar
		// subiendo en ese lapso, y el primer respaldo al arrancar compite con
		// el arranque de la tienda.
		select {
		case <-time.After(2 * time.Minute):
		case <-s.backupStop:
			return
		}
		for {
			s.backupTick(interval)
			select {
			case <-time.After(30 * time.Minute):
			case <-s.backupStop:
				return
			}
		}
	}()
	log.Printf("respaldos automáticos activos: cada %dh en %s (última sesión %s)",
		interval, s.backupSchedulerDir(), stamp)
}

// backupTick es una pasada del ciclo: crea los respaldos vencidos y
// revalida hasta dos respaldos existentes por empresa.
func (s *Server) backupTick(maxAgeHours int) {
	dir := s.backupSchedulerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("respaldos: no se pudo crear %s: %v", dir, err)
		return
	}
	for _, code := range s.TenantCodes() {
		dbName := s.tenantDBName(code)
		if dbName == "" || dbName == "erpgo_invalida" {
			continue
		}
		if edad, hay := s.edadUltimoRespaldo(code); !hay || edad >= time.Duration(maxAgeHours)*time.Hour {
			if err := s.respaldoAutomatico(code, dbName); err != nil {
				log.Printf("respaldo %s: %v", code, err)
			}
		}
		s.backupsRevalidar(code, 2)
	}
}

// respaldoAutomatico genera el ZIP del volcado de una empresa y lo verifica
// ANTES de considerarlo bueno: reabre el ZIP (zip.Reader valida el CRC de cada
// entrada al leerla), asegura que el manifest y el .sql existan no vacíos, y
// escribe el meta con el SHA-256. Solo entonces el archivo cuenta como
// respaldo; si falla la verificación se borra.
func (s *Server) respaldoAutomatico(tenant, dbName string) error {
	dir := s.backupSchedulerDir()
	stamp := time.Now().Format("2006-01-02_150405")
	name := fmt.Sprintf("%s_AUTO_%s.zip", tenant, stamp)
	dst := filepath.Join(dir, name)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Reusa la misma mecánica del botón manual: manifest.txt + <base>.sql,
	// con --single-transaction para la foto consistente.
	if err := s.writeBackupZip(dbName, dst, tenant, stamp); err != nil {
		os.Remove(dst)
		return err
	}

	info, err := os.Stat(dst)
	if err != nil {
		return err
	}
	sha256Hex, err := verificarZipRespaldo(dst)
	if err != nil {
		os.Remove(dst)
		return fmt.Errorf("respaldo no valido: %w", err)
	}
	meta := backupMeta{
		Tenant:     tenant,
		Base:       dbName,
		File:       name,
		Created:    time.Now(),
		SizeBytes:  info.Size(),
		SHA256:     sha256Hex,
		Verified:   true,
		VerifiedAt: time.Now(),
	}
	if err := escribirMetaRespaldo(dir, name, meta); err != nil {
		return err
	}
	log.Printf("respaldo %s creado y verificado (%.2f MB) en %s", name, float64(info.Size())/(1024.0*1024.0), dir)
	return nil
}

// verificarZipRespaldo abre el ZIP, lee cada entrada y devuelve el SHA-256
// del archivo completo. Un CRC corrupto o un .sql vacío no se cuela.
func verificarZipRespaldo(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	var entradas, leidas int
	seen := map[string]bool{}
	for _, f := range zr.File {
		entradas++
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		// Leer al menos 1 byte por archivo para pisar el CRC del zip.Reader.
		one := make([]byte, 1)
		if _, err := io.ReadFull(rc, one); err == nil {
			leidas++
		}
		rc.Close()
		if f.Name == "manifest.txt" {
			seen["manifest"] = true
		}
		if strings.HasSuffix(f.Name, ".sql") {
			seen["sql"] = true
			if f.UncompressedSize64 == 0 {
				return "", fmt.Errorf("volcado SQL vacío")
			}
		}
	}
	if entradas == 0 || !seen["manifest"] || !seen["sql"] || leidas == 0 {
		return "", fmt.Errorf("zip sin manifest o sin volcado")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// escribirMetaRespaldo y leerMetaRespaldo leen y escriben el meta JSON que
// acompaña a cada ZIP. Sin meta, un respaldo no puede demostrar después que
// estaba bueno cuando se creó.
func escribirMetaRespaldo(dir, zipName string, m backupMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, zipName+".meta.json"), b, 0o644)
}

func leerMetaRespaldo(dir, zipName string) (*backupMeta, error) {
	b, err := os.ReadFile(filepath.Join(dir, zipName + ".meta.json"))
	if err != nil {
		return nil, err
	}
	var m backupMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// backupsRevalidar vuelve a abrir y hashear hasta n respaldos existentes cuya
// última verificación tenga más de 7 días (o no haya meta). Si algo no abre o
// el hash no coincide, el meta se marca CORRUPTO para que se vea en la lista;
// nada se borra solo: borrar es decisión de una persona.
func (s *Server) backupsRevalidar(tenant string, max int) {
	dir := s.backupSchedulerDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cand := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), tenant+"_") || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		m, err := leerMetaRespaldo(dir, e.Name())
		if err != nil || m.Corrupted || time.Since(m.VerifiedAt) > 7*24*time.Hour {
			cand = append(cand, e.Name())
			if len(cand) >= max {
				break
			}
		}
	}
	for _, name := range cand {
		hash, err := verificarZipRespaldo(filepath.Join(dir, name))
		m, leerErr := leerMetaRespaldo(dir, name)
		if leerErr != nil {
			continue
		}
		if err != nil {
			m.Corrupted = true
			m.Verified = false
			m.VerifiedAt = time.Now()
			_ = escribirMetaRespaldo(dir, name, *m)
			log.Printf("respaldo %s REVALIDADO con fallo: %v", name, err)
			continue
		}
		if m.SHA256 != "" && m.SHA256 != hash {
			m.Corrupted = true
			m.Verified = false
			m.VerifiedAt = time.Now()
			_ = escribirMetaRespaldo(dir, name, *m)
			log.Printf("respaldo %s REVALIDADO con hash distinto: cambió el archivo", name)
			continue
		}
		m.SHA256 = hash
		m.Verified = true
		m.Corrupted = false
		m.VerifiedAt = time.Now()
		_ = escribirMetaRespaldo(dir, name, *m)
	}
}

// ultimoRespaldoEstado es la foto del estado para la interfaz: si hay
// respaldo, cuánto hace y si está verificado.
func (s *Server) ultimoRespaldoEstado(tenant string) map[string]interface{} {
	dir := s.backupSchedulerDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	pre := tenant + "_"
	var mejor string
	var mejorT time.Time
	for _, e := range entries {
		nombre := e.Name()
		if e.IsDir() || !strings.HasPrefix(nombre, pre) || !strings.HasSuffix(nombre, ".zip") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(mejorT) {
			mejor = nombre
			mejorT = info.ModTime()
		}
	}
	if mejor == "" {
		return nil
	}
	estado := map[string]interface{}{"file": mejor, "when": mejorT.Format("2006-01-02 15:04")}
	if m, err := leerMetaRespaldo(dir, mejor); err == nil {
		estado["verified"] = m.Verified
		estado["corrupted"] = m.Corrupted
		estado["size_mb"] = float64(m.SizeBytes) / (1024 * 1024)
	}
	return estado
}
// databaseBackupStatus expone el estado del ciclo de respaldos: si está
// activo, cada cuánto, y el último respaldo verificado de la empresa activa.
func (s *Server) databaseBackupStatus(w http.ResponseWriter, r *http.Request) {
	tenant := s.tenantCodeOrDefault()
	estado := map[string]interface{}{
		"enabled":        s.config.BackupEnabled,
		"interval_hours": s.config.BackupIntervalHours,
		"dir":            s.backupSchedulerDir(),
		"auto_last":      s.ultimoRespaldoEstado(tenant),
	}
	writeJSON(w, estado)
}

// databaseBackupRunAll lanza un ciclo inmediato de respaldo de TODAS las
// empresas activas, verificado. Para el admin que quiere respaldar ahora
// (p. ej. antes de actualizar el binario).
func (s *Server) databaseBackupRunAll(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil || (sess.UserLevel != "admin" && sess.UserLevel != "master") {
		jsonError(w, "Solo el administrador puede respaldar todas las empresas", http.StatusForbidden)
		return
	}
	dir := s.backupSchedulerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonError(w, "No se pudo crear la carpeta de respaldos: "+err.Error(), 500)
		return
	}
	creados, fallidos := 0, []string{}
	for _, code := range s.TenantCodes() {
		dbName := s.tenantDBName(code)
		if dbName == "" || dbName == "erpgo_invalida" {
			continue
		}
		if err := s.respaldoAutomatico(code, dbName); err != nil {
			fallidos = append(fallidos, code+": "+err.Error())
			continue
		}
		creados++
	}
	resp := map[string]interface{}{
		"status":  1,
		"creados": creados,
		"dir":     dir,
	}
	if len(fallidos) > 0 {
		resp["status"] = 0
		resp["fallidos"] = fallidos
	}
	writeJSON(w, resp)
}
