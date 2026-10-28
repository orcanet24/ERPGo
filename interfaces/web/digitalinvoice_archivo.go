package web

// digitalinvoice_archivo.go — La copia propia del PDF fiscal.
//
// POR QUÉ EXISTE ESTE ARCHIVO.
//
// La imprenta digital devuelve una URL. Una URL no es un archivo: caduca, cambia
// de sitio, depende de que el proveedor siga alojando el documento y —si el
// proveedor deja de existir— se pierde el soporte de una factura que el fisco
// puede pedir dentro de cinco años. La factura es del cliente, pero el deber de
// guardarla es del contribuyente, no del tercero que imprimió.
//
// Aquí el PDF baja una vez, se guarda en el directorio de la empresa y queda
// apuntado en digital_invoice_log.pdf_path. Desde ese momento las pantallas
// piden la copia propia (con sesión y con permisos) y la URL del proveedor pasa
// a ser solo el respaldo.
//
// LO QUE NO HACE: reemplazar la emisión ni la cola. Si la columna pdf_path no
// existe todavía —una base anterior a esta migración—, el archivado se calla y
// todo sigue funcionando con la URL del proveedor: una característica nueva no
// tiene derecho a tumbar una factura emitida.
//
// El XML no se archiva aquí: se descarga por enlace. Es un archivo del proveedor
// que se consulta, no el soporte que hay que resguardar.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// diLimitePDF es el peso máximo que se baja del proveedor. Un PDF fiscal de una
// factura normal pesa cien kilobytes; veinte megabytes es un tope holgado que
// además impide que una respuesta equivocada llene el disco.
const diLimitePDF = 20 << 20

// diDirFiscales es la rama, DENTRO de la rama documentos del directorio de la
// empresa, donde viven los documentos fiscales. Es una rama propia y no
// `documentos` a secas: una factura de la imprenta no debe mezclarse con lo que
// alguien suba a mano, y el dueño puede copiar la carpeta entera para archivarla
// sin llevarse también las fotos que cargaron por pantalla.
const diDirFiscales = "fiscales"

// diColumnaPDFPath comprueba que la columna exista en ESTA base de control.
//
// Se comprueba con una consulta y no con el dialecto: si la columna no está, la
// consulta falla y se devuelve false, sea MySQL, SQLite o lo que venga. Una
// característica que depende de una migración hecha tiene que poder decir
// «todavía no» sin romper nada.
func diColumnaPDFPath(ctl *sql.DB) bool {
	if ctl == nil {
		return false
	}
	rows, err := ctl.Query(`SELECT pdf_path FROM digital_invoice_log LIMIT 1`)
	if err != nil {
		return false
	}
	rows.Close()
	return true
}

// diCompanyID devuelve el identificador de empresa de la base de la empresa.
// Es el mismo criterio que usa la subida de imágenes: la tabla es de la empresa,
// así que su company_id es el que abre su directorio.
func diCompanyID(db *sql.DB) string {
	cid := "1"
	if err := db.QueryRow(`SELECT IFNULL(company_id, '1') FROM company_information
		WHERE status = 1 ORDER BY company_id LIMIT 1`).Scan(&cid); err != nil || cid == "" {
		cid = "1"
	}
	return cid
}

// diArchivarPDF descarga el PDF devuelto por la imprenta y lo guarda en el
// directorio de la empresa, apuntándolo en la fila del documento.
//
// Corre en su propio goroutine desde el runner (ver el llamante): es I/O contra
// un tercero y no tiene por qué frenar la emisión de la cola.
//
// Las cuatro razones por las que NO se archiva no rompen nada: sin columna, sin
// enlace, sin peso o sin firma %PDF en el archivo, se registra y se sigue. La
// única forma de que esto empeore una factura es archivar encima de otra, y
// para eso la escritura de la columna exige que esté vacía.
func (s *Server) diArchivarPDF(req *http.Request, db, ctl *sql.DB, tenant string, correlativo int64, pdfURL, pdfBase64 string) {
	if db == nil || ctl == nil || correlativo <= 0 {
		return
	}
	if !diColumnaPDFPath(ctl) {
		return
	}
	// No archivar dos veces la misma factura: la columna ya escrita es el aviso.
	var yaGuardado string
	_ = ctl.QueryRow(`SELECT COALESCE(pdf_path,'') FROM digital_invoice_log
		WHERE company_id = ? AND invoice_id = ? LIMIT 1`, tenant, correlativo).Scan(&yaGuardado)
	if yaGuardado != "" {
		return
	}

	var contenido []byte
	var err error
	switch {
	case strings.TrimSpace(pdfBase64) != "":
		contenido, err = diDecodificarPDF(pdfBase64)
		if err != nil {
			log.Printf("facturación digital: la copia en base64 de la factura %s/%d no se pudo leer: %v", tenant, correlativo, err)
			return
		}
	default:
		contenido, err = diBajarPDF(pdfURL)
		if err != nil {
			log.Printf("facturación digital: no se pudo archivar la factura %s/%d: %v", tenant, correlativo, err)
			return
		}
	}

	relativa, absoluta, err := s.diGuardarContenido(db, correlativo, contenido)
	if err != nil {
		log.Printf("facturación digital: no se pudo escribir la copia de la factura %s/%d: %v", tenant, correlativo, err)
		return
	}

	// La fila es la de ESTA empresa: company_id evita que dos empresas con el
	// mismo correlativo se pidan la misma columna. El `pdf_path = ''` del WHERE
	// es la cerradura: dos corridas a la vez no escriben una sobre la otra.
	res, err := ctl.Exec(`UPDATE digital_invoice_log
		SET pdf_path = ?
		WHERE company_id = ? AND invoice_id = ? AND COALESCE(pdf_path,'') = ''`,
		relativa, tenant, correlativo)
	if err != nil {
		log.Printf("facturación digital: no se pudo apuntar la copia de la factura %s/%d: %v", tenant, correlativo, err)
		_ = os.Remove(absoluta)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Otra corrida llegó primero: su copia ya está, la mía sobra.
		_ = os.Remove(absoluta)
		return
	}
	log.Printf("facturación digital: factura %s/%d archivada en %s", tenant, correlativo, relativa)
}

// diBajarPDF baja el documento con tiempo y peso acotados.
func diBajarPDF(url string) ([]byte, error) {
	limpia := diURLSegura(url)
	if limpia == "" {
		return nil, fmt.Errorf("la imprenta no devolvió un enlace http(s) para descargar el PDF")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, limpia, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ERPGo-archivo-fiscal/1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo descargar de %s: %w", hostDe(limpia), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("el proveedor respondió %d al pedir el PDF", resp.StatusCode)
	}
	contenido, err := io.ReadAll(io.LimitReader(resp.Body, diLimitePDF+1))
	if err != nil {
		return nil, err
	}
	if len(contenido) > diLimitePDF {
		return nil, fmt.Errorf("el PDF pesa más de %d MB", diLimitePDF>>20)
	}
	return contenido, nil
}

// diDecodificarPDF convierte el pdf_base64 que el proveedor devuelve en la
// respuesta. El prefijo data: se quita si viene, que es como lo mandan algunas
// APIs.
func diDecodificarPDF(b64 string) ([]byte, error) {
	b64 = strings.TrimSpace(b64)
	if i := strings.Index(b64, ","); strings.HasPrefix(b64, "data:") && i > 0 {
		b64 = b64[i+1:]
	}
	b64 = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, b64)

	contenido, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("el pdf_base64 del proveedor no es base64 válido: %w", err)
	}
	return contenido, nil
}

// diGuardarContenido escribe el archivo en la rama fiscal del directorio de la
// empresa y devuelve la RUTA RELATIVA que se guarda en la columna —la misma
// forma en la que viven el logo y las imágenes de producto— y la ruta absoluta,
// que es la que sirve para borrarlo si no llegó a apuntarse.
func (s *Server) diGuardarContenido(db *sql.DB, correlativo int64, contenido []byte) (string, string, error) {
	if s.uploadMgr == nil {
		return "", "", fmt.Errorf("no hay gestor de archivos configurado")
	}
	if len(contenido) < 4 || string(contenido[:4]) != "%PDF" {
		return "", "", fmt.Errorf("lo que devolvió la imprenta no es un PDF")
	}
	empresaDir, err := s.uploadMgr.EmpresaDir(diCompanyID(db), db)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(empresaDir, "documentos", diDirFiscales)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}

	// Nombre legible y estable: factura-<correlativo>-<AAAA-MM-DD>.pdf. No lleva
	// aleatorio porque el archivo se reemplaza si se vuelve a archivar, y a una
	// persona que abra la carpeta le sirve el número de la factura.
	nombre := fmt.Sprintf("factura-%d-%s.pdf", correlativo, time.Now().Format("2006-01-02"))
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, contenido, 0o644); err != nil {
		return "", "", err
	}

	// La relativa es contra la BASE del gestor, con el mismo criterio que usa
	// upload.Save: «EMPxxxx/documentos/fiscales/factura-….pdf». Si se guardara
	// contra el directorio de la empresa, GetFullPath volvería a unir la base y
	// buscaría dos veces el mismo segmento.
	padre := strings.ReplaceAll(filepath.Dir(empresaDir), "\\", "/")
	plana := strings.ReplaceAll(ruta, "\\", "/")
	relativa := strings.TrimPrefix(plana, padre+"/")
	if relativa == plana {
		return "", "", fmt.Errorf("la ruta del archivo no queda dentro de la base de uploads")
	}
	return relativa, ruta, nil
}
