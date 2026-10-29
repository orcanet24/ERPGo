package web

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"erpgow"
)

// ---------- Settings module (replicates dashboard settings) ----------

func (s *Server) registerSettingRoutes() {
	s.mux.HandleFunc("GET /settings", s.settingForm)
	s.mux.HandleFunc("POST /settings/save", s.settingSave)
	s.mux.HandleFunc("GET /settings/company", s.companyForm)
	s.mux.HandleFunc("POST /settings/company/save", s.companySave)
	s.mux.HandleFunc("GET /settings/currency", s.currencyList)
	s.mux.HandleFunc("POST /settings/currency/save", s.currencySave)
	s.mux.HandleFunc("POST /settings/currency/delete/{id}", s.currencyDelete)
	s.mux.HandleFunc("GET /settings/backup", s.backupForm)
	s.mux.HandleFunc("POST /settings/backup/run", s.backupRun)
	s.mux.HandleFunc("GET /data/backup/{name}", s.backupDownload)
}

func (s *Server) settingForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "setting.html", map[string]interface{}{"Title": "Configuración"})
}

// allowedImageExts mirrors the PHP whitelist (webp|gif|jpg|png|jpeg|ico).
var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".ico": true,
}

// imageAssetPath normalizes a stored image path for template use. The DB keeps
// a relative path WITHOUT leading slash ("uploads/logo/x.png") and templates
// prepend "/". Returns "" (→ template fallback) when the value is empty, is not
// under uploads/ or static/ (legacy PHP rows keep "assets/img/icons/...") or the
// file no longer exists on disk.
func imageAssetPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"))
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "uploads/") && !strings.HasPrefix(p, "static/") {
		return ""
	}
	if _, err := os.Stat(filepath.FromSlash(p)); err == nil {
		return p
	}
	// static/ may live only in the embedded binary (single-file deployment).
	if strings.HasPrefix(p, "static/") {
		if _, err := fs.Stat(erpgow.Static(), p); err == nil {
			return p
		}
	}
	return ""
}

// removeStoredImage deletes a replaced upload. Only paths under uploads/ are
// ever removed — static/ assets ship with the app and are never touched.
func removeStoredImage(p string) {
	p = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "/")
	if !strings.HasPrefix(p, "uploads/") || strings.Contains(p, "..") {
		return
	}
	_ = os.Remove(filepath.FromSlash(p))
}

// saveImageUpload stores one image input of the settings form under the
// company's upload directory using s.uploadMgr. The rama parameter is one of
// "logo", "productos", "personal", "documentos". The returned relative path
// is stored in web_setting.logo / invoice_logo / favicon.
func (s *Server) saveImageUpload(r *http.Request, field, rama string) (string, string, int) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return "", "", 0
	}
	file, hdr, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", "", 0
		}
		return "", fmt.Sprintf("No se pudo leer el campo %q: %v", field, err), http.StatusInternalServerError
	}
	defer file.Close()
	if hdr == nil || strings.TrimSpace(hdr.Filename) == "" {
		return "", "", 0
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !allowedImageExts[ext] {
		return "", fmt.Sprintf("Extensión no permitida %q en %q. Formatos válidos: jpg, jpeg, png, gif, webp, ico", ext, field), http.StatusBadRequest
	}

	companyID := r.FormValue("company_id")
	if companyID == "" {
		companyID = "1"
	}

	result, err := s.uploadMgr.Save(companyID, rama, file, hdr, s.dbFor(r))
	if err != nil {
		return "", fmt.Sprintf("No se pudo guardar el archivo %q: %v", field, err), http.StatusInternalServerError
	}

	// Path returned by uploadMgr already has "uploads/EMP{hex}/{rama}/{file}" format
	// stored WITHOUT leading slash (template adds it back)
	return result.Path, "", 0
}

func (s *Server) settingSave(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			jsonError(w, "No se pudo procesar el formulario: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// current values: needed to delete a replaced upload afterwards
	var prevLogo, prevInvoice, prevFavicon sql.NullString
	_ = s.dbFor(r).QueryRow(`SELECT logo, invoice_logo, favicon FROM web_setting WHERE setting_id=1`).
		Scan(&prevLogo, &prevInvoice, &prevFavicon)

	// files written so far: removed again if any later step fails
	var newFiles []string
	rollback := func() {
		for _, p := range newFiles {
			removeStoredImage(p)
		}
	}

	logo, errMsg, code := s.saveImageUpload(r, "logo", "logo")
	if errMsg != "" {
		jsonError(w, errMsg, code)
		return
	}
	if logo != "" {
		newFiles = append(newFiles, logo)
	}

	invoiceLogo, errMsg, code := s.saveImageUpload(r, "invoice_logo", "logo")
	if errMsg != "" {
		rollback()
		jsonError(w, errMsg, code)
		return
	}
	if invoiceLogo != "" {
		newFiles = append(newFiles, invoiceLogo)
	}

	favicon, errMsg, code := s.saveImageUpload(r, "favicon", "logo")
	if errMsg != "" {
		rollback()
		jsonError(w, errMsg, code)
		return
	}
	if favicon != "" {
		newFiles = append(newFiles, favicon)
	}

	sets := []string{
		"currency=?", "timezone=?", "footer_text=?", "language=?", "rtr=?",
		"is_qr=?", "is_autoapprove_v=?", "discount_type=?",
	}
	args := []interface{}{
		r.FormValue("currency"), r.FormValue("timezone"), r.FormValue("footer_text"),
		r.FormValue("language"), orDefault(r.FormValue("rtr"), "0"),
		boolInt(r.FormValue("is_qr") == "1"), boolInt(r.FormValue("is_autoapprove_v") == "1"),
		orDefault(r.FormValue("discount_type"), "1"),
	}
	// only columns with a NEW file are written: an untouched input keeps the
	// value already stored (never UPDATE logo='')
	for _, col := range []struct {
		name string
		val  string
	}{{"logo", logo}, {"invoice_logo", invoiceLogo}, {"favicon", favicon}} {
		if col.val != "" {
			sets = append(sets, col.name+"=?")
			args = append(args, col.val)
		}
	}

	if _, err := s.dbFor(r).Exec("UPDATE web_setting SET "+strings.Join(sets, ", ")+" WHERE setting_id = 1", args...); err != nil {
		rollback()
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// replaced uploads: drop the old file (static/ is never deleted)
	if logo != "" {
		removeStoredImage(prevLogo.String)
	}
	if invoiceLogo != "" {
		removeStoredImage(prevInvoice.String)
	}
	if favicon != "" {
		removeStoredImage(prevFavicon.String)
	}

	s.reloadSettings()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Configuración guardada", "redirect": saveRedirect(r, "/settings")})
}

func (s *Server) companyForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "company_form.html", map[string]interface{}{"Title": "Datos de la Empresa"})
}

func (s *Server) companySave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("company_name"))
	if name == "" {
		jsonError(w, "El nombre es obligatorio", 400)
		return
	}
	var exists int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM company_information`).Scan(&exists)
	if exists == 0 {
		_, err := s.dbFor(r).Exec(`INSERT INTO company_information (company_id, company_name, email, address, mobile, website, vat_no, cr_no, status)
			VALUES ('1',?,?,?,?,?,?,?,1)`,
			name, r.FormValue("email"), r.FormValue("address"), r.FormValue("mobile"),
			r.FormValue("website"), r.FormValue("vat_no"), r.FormValue("cr_no"))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		_, err := s.dbFor(r).Exec(`UPDATE company_information SET company_name=?, email=?, address=?, mobile=?, website=?, vat_no=?, cr_no=? WHERE company_id='1'`,
			name, r.FormValue("email"), r.FormValue("address"), r.FormValue("mobile"),
			r.FormValue("website"), r.FormValue("vat_no"), r.FormValue("cr_no"))
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	s.reloadSettings()
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Empresa guardada", "redirect": saveRedirect(r, "/settings")})
}

// ---------- Currency ----------

func (s *Server) currencyList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT id, currency_name, icon FROM currency_tbl ORDER BY id`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type cur struct {
		ID   int
		Name string
		Icon string
	}
	list := []cur{}
	for rows.Next() {
		var c cur
		_ = rows.Scan(&c.ID, &c.Name, &c.Icon)
		list = append(list, c)
	}
	s.render(w, r, "currency_list.html", map[string]interface{}{
		"Title":      "Monedas",
		"Currencies": list,
	})
}

func (s *Server) currencySave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("currency_name"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	if name == "" || icon == "" {
		jsonError(w, "Nombre y símbolo son obligatorios", 400)
		return
	}
	id := r.FormValue("id")
	if id == "" {
		if _, err := s.dbFor(r).Exec(`INSERT INTO currency_tbl (currency_name, icon) VALUES (?,?)`, name, icon); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		if _, err := s.dbFor(r).Exec(`UPDATE currency_tbl SET currency_name=?, icon=? WHERE id=?`, name, icon, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Moneda guardada", "redirect": saveRedirect(r, "/settings")})
}

func (s *Server) currencyDelete(w http.ResponseWriter, r *http.Request) {
	if _, err := s.dbFor(r).Exec(`DELETE FROM currency_tbl WHERE id=?`, r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Eliminada", "redirect": saveRedirect(r, "/settings")})
}

// ---------- Backup ----------

func (s *Server) backupForm(w http.ResponseWriter, r *http.Request) {
	// list existing backups
	type bk struct {
		Name string
		Size int64
		Date string
	}
	backups := []bk{}
	entries, _ := os.ReadDir("data/backup")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			info, _ := e.Info()
			backups = append(backups, bk{
				Name: e.Name(),
				Size: info.Size(),
				Date: info.ModTime().Format("2006-01-02 15:04"),
			})
		}
	}
	s.render(w, r, "backup.html", map[string]interface{}{
		"Title":   "Copia de Seguridad",
		"Backups": backups,
	})
}

// backupRun dumps all tables to a .sql file (pure Go, no mysqldump needed).
func (s *Server) backupRun(w http.ResponseWriter, r *http.Request) {
	os.MkdirAll("data/backup", 0755)
	fname := fmt.Sprintf("backup_%s.sql", strings.ReplaceAll(strings.ReplaceAll(now(), ":", "-"), " ", "_"))
	f, err := os.Create(filepath.Join("data", "backup", fname))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer f.Close()

	f.WriteString("-- ERPGo backup " + now() + "\nSET FOREIGN_KEY_CHECKS=0;\n")

	tables, err := s.dbFor(r).Query(s.sqlShowTables())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var tnames []string
	for tables.Next() {
		var t string
		_ = tables.Scan(&t)
		tnames = append(tnames, t)
	}
	tables.Close()

	for _, t := range tnames {
		f.WriteString("\n-- Table " + t + "\n")
		rows, err := s.dbFor(r).Query("SELECT * FROM `" + t + "`")
		if err != nil {
			continue
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]sql.RawBytes, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}
			var sb strings.Builder
			sb.WriteString("INSERT INTO `" + t + "` (`" + strings.Join(cols, "`,`") + "`) VALUES (")
			for i, v := range vals {
				if i > 0 {
					sb.WriteString(",")
				}
				if v == nil {
					sb.WriteString("NULL")
				} else {
					// Escape estándar de MySQL para que el dump sea restaurable.
					// Orden: primero la barra invertida; después el resto.
					esc := strings.ReplaceAll(string(v), "\\", "\\\\")
					esc = strings.ReplaceAll(esc, "'", "\\'")
					esc = strings.ReplaceAll(esc, "\"", "\\\"")
					esc = strings.ReplaceAll(esc, "\n", "\\n")
					esc = strings.ReplaceAll(esc, "\r", "\\r")
					esc = strings.ReplaceAll(esc, "\x00", "\\0")
					esc = strings.ReplaceAll(esc, "\x1a", "\\Z")
					sb.WriteString("'" + esc + "'")
				}
			}
			sb.WriteString(");\n")
			f.WriteString(sb.String())
		}
		rows.Close()
	}
	f.WriteString("\nSET FOREIGN_KEY_CHECKS=1;\n")

	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": "Backup generado", "file": fname,
		"path": "/data/backup/" + fname,
	})
}

// backupDownload serves a generated backup file from data/backup.
func (s *Server) backupDownload(w http.ResponseWriter, r *http.Request) {
	fname := r.PathValue("name")
	// sanitize: no path traversal
	if strings.Contains(fname, "/") || strings.Contains(fname, "\\") || strings.Contains(fname, "..") {
		jsonError(w, "archivo inválido", 400)
		return
	}
	p := filepath.Join("data", "backup", fname)
	if _, err := os.Stat(p); err != nil {
		jsonError(w, "backup no encontrado", 404)
		return
	}
	w.Header().Set("Content-Type", "application/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+fname+"\"")
	http.ServeFile(w, r, p)
}
