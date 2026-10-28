package web

// Pruebas de la copia propia del PDF fiscal.
//
// Lo que hay que demostrar aquí no es que el archivo se escriba (eso lo hace el
// sistema operativo), sino las REGLAS: se archiva UNA vez y no se pisa, lo que
// baja tiene que ser un PDF de verdad, y mientras la base no tenga la columna
// de la migración el archivado se calla en vez de romper la consulta que lee
// los enlaces de TODAS las pantallas.

import (
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erpgow/internal/upload"
)

// baseDeControlConPDFPath es la tabla de la facturación digital ya con la
// columna de la copia propia.
func baseDeControlConPDFPath(t *testing.T) *sql.DB {
	t.Helper()
	return nuevaBaseDeControlConCopia(t)
}

// TestLaCopiaPropiaDelPdfSeArchivaUnaVezYSeSirve es el circuito completo de la
// Fase B: la imprenta devuelve el PDF, el ERP lo guarda en el directorio de la
// empresa, apunta la copia en la fila, y a partir de ahí las pantallas piden la
// copia propia y el correo sigue llevando la URL del proveedor —que es la única
// que el cliente puede abrir sin sesión—.
func TestLaCopiaPropiaDelPdfSeArchivaUnaVezYSeSirve(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	ctl := baseDeControlConPDFPath(t)
	base := t.TempDir()
	s := &Server{db: db, uploadMgr: upload.NewManager(base)}
	completarFacturaDigital(t, db)

	pdf := []byte("%PDF-1.4\n1 0 obj\n<< >>\nendobj\ntrailer\n%%EOF\n")
	b64 := base64.StdEncoding.EncodeToString(pdf)

	insertarDocDigital(t, ctl, 10, "https://docs.test/factura-10.pdf", "")
	// La copia llega por el base64 de la respuesta, sin tocar la red.
	s.diArchivarPDF(nil, db, ctl, "empresa1", 10, "", b64)

	var relativa string
	if err := ctl.QueryRow(`SELECT COALESCE(pdf_path,'') FROM digital_invoice_log
		WHERE company_id='empresa1' AND invoice_id=10`).Scan(&relativa); err != nil {
		t.Fatalf("no se apuntó la copia: %v", err)
	}
	if relativa == "" {
		t.Fatal("la columna pdf_path quedó vacía: la copia no se archivó")
	}
	if !strings.Contains(relativa, "documentos/fiscales/") {
		t.Errorf("la copia no quedó en la rama fiscal: %s", relativa)
	}
	if !strings.HasSuffix(relativa, "factura-10-") && !strings.Contains(relativa, "factura-10-") {
		t.Errorf("el nombre del archivo no lleva el número de factura: %s", relativa)
	}

	// El archivo existe, está dentro de la base de uploads y es el PDF que bajó.
	full := filepath.Join(base, filepath.FromSlash(relativa))
	leido, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("el archivo no está en %s: %v", full, err)
	}
	if string(leido[:5]) != "%PDF-" {
		t.Errorf("el archivo guardado no empieza como PDF: %q", leido[:8])
	}

	// Las pantallas ya piden la copia propia, no la URL del proveedor.
	docs := diDocsDeFacturas(ctl, "empresa1", []int64{10})
	if docs[10].PDFPath != relativa {
		t.Errorf("la consulta no devuelve la copia: %q", docs[10].PDFPath)
	}
	if enlace := s.diEnlaceInterno(10, docs[10]); enlace != "/invoice/fiscal/10" {
		t.Errorf("la pantalla debe pedir la copia propia; salió %q", enlace)
	}
	// El correo, en cambio, lleva la URL del proveedor: el cliente no tiene sesión.
	if publico := diEnlacePublico(docs[10]); publico != "https://docs.test/factura-10.pdf" {
		t.Errorf("el correo debe llevar la URL del proveedor; salió %q", publico)
	}

	// Una segunda pasada no reescribe ni duplica: la columna ya escrita es el
	// aviso de que hay copia.
	s.diArchivarPDF(nil, db, ctl, "empresa1", 10, "", b64)
	var filas int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM digital_invoice_log WHERE invoice_id=10`).Scan(&filas)
	if filas != 1 {
		t.Errorf("la factura aparece %d veces en la cola", filas)
	}
	// Y no quedó ningún archivo huérfano en la rama fiscal.
	huerfanos, _ := filepath.Glob(filepath.Join(base, "*", "documentos", "fiscales", "*.pdf"))
	if len(huerfanos) != 1 {
		t.Errorf("se esperaba UN archivo archivado, hay %d: %v", len(huerfanos), huerfanos)
	}
}

// TestLaCopiaSeNiegaSiLoQueBajaNoEsUnPdf: el proveedor manda bytes, no un
// archivo. Un HTML de error de pago de 404 guardado como «factura.pdf» sería
// el peor resultado posible: el sistema diría que tiene el soporte y no lo
// tendría.
func TestLaCopiaSeNiegaSiLoQueBajaNoEsUnPdf(t *testing.T) {
	db := nuevaBaseDeCartera(t)
	ctl := baseDeControlConPDFPath(t)
	s := &Server{db: db, uploadMgr: upload.NewManager(t.TempDir())}
	completarFacturaDigital(t, db)
	insertarDocDigital(t, ctl, 11, "https://docs.test/factura-11.pdf", "")

	html := base64.StdEncoding.EncodeToString([]byte("<html><body>404</body></html>"))
	s.diArchivarPDF(nil, db, ctl, "empresa1", 11, "", html)

	var relativa string
	_ = ctl.QueryRow(`SELECT COALESCE(pdf_path,'') FROM digital_invoice_log WHERE invoice_id=11`).Scan(&relativa)
	if relativa != "" {
		t.Errorf("se archivó algo que no es un PDF: %s", relativa)
	}
	// Y sin enlace no se intenta siquiera.
	s.diArchivarPDF(nil, db, ctl, "empresa1", 11, "", "")
	_ = ctl.QueryRow(`SELECT COALESCE(pdf_path,'') FROM digital_invoice_log WHERE invoice_id=11`).Scan(&relativa)
	if relativa != "" {
		t.Errorf("sin enlace ni base64 no puede haber copia; quedó %s", relativa)
	}
}

// TestSinLaColumnaDeLaMigracionElArchivadoSeCalla es la regla de convivencia:
// una base anterior a la migración no puede ver cómo se cae la consulta que
// alimenta los enlaces de facturación y de cartera.
func TestSinLaColumnaDeLaMigracionElArchivadoSeCalla(t *testing.T) {
	ctl := nuevaBaseDeControl(t) // sin pdf_path
	if diColumnaPDFPath(ctl) {
		t.Fatal("diColumnaPDFPath dice que la columna existe en una tabla sin ella")
	}
	insertarDocDigital(t, ctl, 12, "https://docs.test/factura-12.pdf", "")

	// La consulta sigue funcionando: devuelve el documento sin la copia.
	docs := diDocsDeFacturas(ctl, "empresa1", []int64{12})
	if len(docs) != 1 {
		t.Fatalf("la consulta dejó de devolver documentos: %d", len(docs))
	}
	if docs[12].PDFPath != "" {
		t.Errorf("sin columna no puede haber copia: %q", docs[12].PDFPath)
	}
	if docs[12].PDF != "https://docs.test/factura-12.pdf" {
		t.Errorf("la URL del proveedor se perdió: %q", docs[12].PDF)
	}

	// Y el archivado no intenta escribir una columna que no existe.
	db := nuevaBaseDeCartera(t)
	s := &Server{db: db, uploadMgr: upload.NewManager(t.TempDir())}
	completarFacturaDigital(t, db)
	s.diArchivarPDF(nil, db, ctl, "empresa1", 12, "",
		base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n%%EOF")))
	var relativa string
	_ = ctl.QueryRow(`SELECT COALESCE(pdf_url,'') FROM digital_invoice_log WHERE invoice_id=12`).Scan(&relativa)
	if relativa == "" {
		t.Error("la fila original se tocó sin columna de destino")
	}
}
