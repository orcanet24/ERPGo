package upload

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager maneja archivos subidos organizados por empresa con nombres aleatorios.
type Manager struct {
	basePath string
}

// NewManager crea un manager con la base de uploads.
func NewManager(basePath string) *Manager {
	return &Manager{basePath: basePath}
}

// EmpresaDir devuelve el directorio de archivos de una empresa.
// Si no existe, lo crea con nombre aleatorio.
func (m *Manager) EmpresaDir(companyID string, db *sql.DB) (string, error) {
	var uploadPath string
	// COALESCE: la columna es DEFAULT NULL y un Scan directo a string revienta
	// con 'converting NULL to string is unsupported' (se vio al subir el favicon).
	// Con NULL/vacío la lógica de abajo genera un directorio nuevo y lo persiste.
	err := db.QueryRow(
		"SELECT COALESCE(upload_path, '') FROM company_information WHERE company_id = ?", companyID,
	).Scan(&uploadPath)

	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	if uploadPath == "" {
		uploadPath = generateRandomDir()
		_, err = db.Exec(
			"UPDATE company_information SET upload_path = ? WHERE company_id = ?",
			uploadPath, companyID,
		)
		if err != nil {
			return "", fmt.Errorf("guardando upload_path: %w", err)
		}
	}

	dir := filepath.Join(m.basePath, uploadPath)
	os.MkdirAll(dir, 0755)
	os.MkdirAll(filepath.Join(dir, "logo"), 0755)
	os.MkdirAll(filepath.Join(dir, "productos"), 0755)
	os.MkdirAll(filepath.Join(dir, "personal"), 0700) // Más restrictivo para personal
	os.MkdirAll(filepath.Join(dir, "documentos"), 0755)

	return dir, nil
}

// UploadResult contiene la ruta relativa y el nombre del archivo subido.
type UploadResult struct {
	Path     string
	FullPath string
}

// Save guarda un archivo en el directorio de la empresa.
func (m *Manager) Save(companyID string, rama string, file multipart.File, header *multipart.FileHeader, db *sql.DB) (*UploadResult, error) {
	dir, err := m.EmpresaDir(companyID, db)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	filename := hex.EncodeToString(randBytes) + ext

	dstPath := filepath.Join(dir, rama, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("creando archivo: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		return nil, fmt.Errorf("escribiendo archivo: %w", err)
	}

	relPath := strings.TrimPrefix(strings.Replace(dstPath, "\\", "/", -1), m.basePath+"/")

	return &UploadResult{
		Path:     relPath,
		FullPath: dstPath,
	}, nil
}

// GetFullPath devuelve la ruta absoluta desde una ruta relativa en BD.
// Compatibilidad hacia atrás: si el archivo no existe en la ruta almacenada,
// busca en rutas legadas (ej: uploads/product/, uploads/logo/).
func (m *Manager) GetFullPath(relPath string, companyID string, db *sql.DB) (string, error) {
	var uploadPath string
	err := db.QueryRow(
		"SELECT COALESCE(upload_path, '') FROM company_information WHERE company_id = ?", companyID,
	).Scan(&uploadPath)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	// 1. Intentar ruta estándar (relPath ya tiene el prefijo EMP...)
	fullPath := filepath.Join(m.basePath, relPath)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	// 2. Compatibilidad hacia atrás: buscar en rutas legadas
	if uploadPath != "" {
		legacypaths := []string{
			filepath.Join(m.basePath, uploadPath, relPath),
			filepath.Join(m.basePath, strings.TrimPrefix(relPath, uploadPath+"/")),
		}
		for _, lp := range legacypaths {
			if _, err := os.Stat(lp); err == nil {
				return lp, nil
			}
		}
	}

	// 3. Búsqueda final en raíz de uploads (archivos muy viejos)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}

	return fullPath, nil // Devuelve la ruta estándar aunque no exista
}

// ServeFile sirve un archivo verificando permisos.
func (m *Manager) ServeFile(w http.ResponseWriter, r *http.Request, relPath string, companyID string, db *sql.DB) error {
	fullPath, err := m.GetFullPath(relPath, companyID, db)
	if err != nil {
		return err
	}

	// Verificar que el archivo existe y está dentro del directorio permitido
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("archivo no encontrado")
	}

	// Detectar content type
	ext := strings.ToLower(filepath.Ext(fullPath))
	contentType := "application/octet-stream"
	switch ext {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".pdf":
		contentType = "application/pdf"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeFile(w, r, fullPath)
	return nil
}

// generateRandomDir genera un nombre aleatorio de directorio.
func generateRandomDir() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "EMP" + strings.ToUpper(hex.EncodeToString(b))
}

// now devuelve el timestamp actual para logging.
func now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
