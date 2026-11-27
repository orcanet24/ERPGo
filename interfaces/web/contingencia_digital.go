package web

// contingencia_digital.go — relación de los documentos que la caja emitió sin
// red y que después se transmitieron por facturación digital.
//
// POR QUÉ EXISTE ESTE ARCHIVO.
//
// La Providencia Administrativa SNAT/2024/000102 (Gaceta Oficial 43.032 del
// 19-dic-2024), artículo 16, admite que el contribuyente siga vendiendo cuando
// falla internet o el equipo, pero no lo deja en "ya veremos": superada la
// contingencia tiene que
//
//	1. registrar los comprobantes físicos en su sistema,
//	2. resguardar el soporte físico, y
//	3. NOTIFICAR POR ESCRITO a la Gerencia Regional de Tributos Internos y a la
//	   imprenta digital que le asigna los números de control, con una RELACIÓN
//	   DETALLADA de los documentos.
//
// La norma NO fija plazo para esa notificación ni un formulario oficial. Lo que
// sí exige es la relación. Este archivo la produce: por rango de fechas, con el
// número de contingencia que lleva el papel, los montos y —cuando ya se
// transmitió— el número de control que asignó la imprenta digital. Es la hoja
// que se anexa a la comunicación.
//
// Lo que NO hace: inventar un número de control. Mientras el documento es de
// contingencia su referencia es "CONT-<terminal>-<correlativo>"; el control
// fiscal real lo escribe el proveedor en digital_invoice_log.nro_control al
// emitir.

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// contingenciaFila es una línea de la relación.
type contingenciaFila struct {
	Invoice     int64
	Fecha       string
	Referencia  string // control_no del documento de contingencia
	RIF         string
	Base        float64
	IVA         float64
	Total       float64
	EstadoDI    string // pending | sent | failed | "" (no encolado)
	NroControl  string // control fiscal asignado por la imprenta digital
	ErrorDI     string
	Transmitida bool
}

// contingenciaRelacion devuelve los documentos de contingencia de la empresa de
// la petición, ordenados por factura. limit <= 0 significa 500.
func (s *Server) contingenciaRelacion(r *http.Request, desde, hasta string, limit int) ([]contingenciaFila, error) {
	if limit <= 0 {
		limit = 500
	}
	db := s.dbFor(r)
	if db == nil {
		return nil, fmt.Errorf("base de la empresa no disponible")
	}

	// El rango se filtra en SQL con fechas ISO (la columna es varchar(50) y se
	// guarda como AAAA-MM-DD). Sin fechas, entra todo lo de la serie.
	where := "series = 'CONT'"
	args := []interface{}{}
	if strings.TrimSpace(desde) != "" {
		where += " AND date >= ?"
		args = append(args, strings.TrimSpace(desde))
	}
	if strings.TrimSpace(hasta) != "" {
		where += " AND date <= ?"
		args = append(args, strings.TrimSpace(hasta))
	}
	args = append(args, limit)

	rows, err := db.Query(`SELECT invoice, COALESCE(date,''), COALESCE(control_no,''),
			COALESCE(customer_rif,''), total_amount, total_vat_amnt
		FROM invoice WHERE `+where+` ORDER BY invoice LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filas []contingenciaFila
	ids := []string{}
	porInvoice := map[int64]int{}
	for rows.Next() {
		var f contingenciaFila
		var total, iva float64
		if err := rows.Scan(&f.Invoice, &f.Fecha, &f.Referencia, &f.RIF, &total, &iva); err != nil {
			continue
		}
		f.Total = total
		f.IVA = iva
		f.Base = total - iva
		porInvoice[f.Invoice] = len(filas)
		filas = append(filas, f)
		ids = append(ids, strconv.FormatInt(f.Invoice, 10))
	}
	if len(filas) == 0 {
		return filas, nil
	}

	// Estado en la cola de facturación digital. Vive en la base de CONTROL, por
	// eso se consulta aparte: en la base de la empresa no está.
	if ctl, cerr := s.controlStore(); cerr == nil && len(ids) > 0 {
		// El número de control completo, cuando la base ya tiene la columna:
		// CG lo entrega con su serie ("00-00000056") y la columna entera no
		// puede guardarla. Ver diColumnaControlTexto.
		colControl := ""
		if diColumnaControlTexto(ctl) {
			colControl = ", COALESCE(nro_control_texto,'')"
		}
		q := `SELECT invoice_id, COALESCE(status,''), COALESCE(nro_control,0), COALESCE(error_message,'')` + colControl + `
			FROM digital_invoice_log
			WHERE company_id = ? AND invoice_id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
		qargs := []interface{}{s.tenantOf(r)}
		for _, id := range ids {
			qargs = append(qargs, id)
		}
		if rs, e := ctl.Query(q, qargs...); e == nil {
			for rs.Next() {
				var invID int64
				var estado, errMsg string
				var nro int64
				destinos := []interface{}{&invID, &estado, &nro, &errMsg}
				var texto string
				if colControl != "" {
					destinos = append(destinos, &texto)
				}
				if rs.Scan(destinos...) != nil {
					continue
				}
				if i, ok := porInvoice[invID]; ok {
					filas[i].EstadoDI = estado
					filas[i].ErrorDI = errMsg
					switch {
					case strings.TrimSpace(texto) != "":
						filas[i].NroControl = strings.TrimSpace(texto)
					case nro > 0:
						filas[i].NroControl = strconv.FormatInt(nro, 10)
					}
					filas[i].Transmitida = estado == "sent"
				}
			}
			rs.Close()
		}
	}
	return filas, nil
}

// contingenciaCSV descarga la relación en CSV (separador ';' y BOM, para que
// Excel en español la abra con las columnas separadas y los acentos enteros).
//
// Es el anexo de la notificación escrita: por eso va con los datos que pide una
// revisión —referencia del papel, RIF, base, IVA, total y control fiscal del
// documento digital que lo sustituyó— y no con los identificadores internos.
func (s *Server) contingenciaCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requireDigitalInvoiceAdmin(w, r) {
		return
	}
	desde := r.URL.Query().Get("desde")
	hasta := r.URL.Query().Get("hasta")
	filas, err := s.contingenciaRelacion(r, desde, hasta, 5000)
	if err != nil {
		jsonError(w, "No se pudo leer la relación: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nombre := "relacion-contingencia"
	if desde != "" || hasta != "" {
		nombre += "-" + strings.TrimSpace(desde) + "_" + strings.TrimSpace(hasta)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`.csv"`)
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	_ = cw.Write([]string{"Factura", "Fecha", "Referencia de contingencia", "RIF",
		"Base imponible", "IVA", "Total", "Estado facturacion digital",
		"Nro. de control fiscal", "Observacion"})
	for _, f := range filas {
		_ = cw.Write([]string{
			strconv.FormatInt(f.Invoice, 10),
			f.Fecha,
			f.Referencia,
			f.RIF,
			fmt.Sprintf("%.2f", f.Base),
			fmt.Sprintf("%.2f", f.IVA),
			fmt.Sprintf("%.2f", f.Total),
			estadoDIlegible(f.EstadoDI),
			f.NroControl,
			f.ErrorDI,
		})
	}
	cw.Flush()
	_ = cw.Error()
}

// estadoDIlegible traduce el estado interno al texto de la relación.
func estadoDIlegible(estado string) string {
	switch estado {
	case "sent":
		return "Transmitida"
	case "pending":
		return "Pendiente de transmitir"
	case "failed":
		return "Rechazada (requiere revision)"
	case "":
		return "Sin encolar"
	}
	return estado
}

// contingenciaResumen deja en el panel de facturación digital el conteo y las
// últimas filas: pendientes primero, porque son las que hay que empujar.
func (s *Server) contingenciaResumen(r *http.Request) ([]contingenciaFila, int, int) {
	filas, err := s.contingenciaRelacion(r, "", "", 500)
	if err != nil {
		return nil, 0, 0
	}
	pendientes, fallidas := 0, 0
	for _, f := range filas {
		switch f.EstadoDI {
		case "pending", "":
			pendientes++
		case "failed":
			fallidas++
		}
	}
	sort.SliceStable(filas, func(i, j int) bool {
		pi := filas[i].EstadoDI != "sent"
		pj := filas[j].EstadoDI != "sent"
		if pi != pj {
			return pi
		}
		return filas[i].Invoice > filas[j].Invoice
	})
	if len(filas) > 50 {
		filas = filas[:50]
	}
	return filas, pendientes, fallidas
}
