package web

// finance_alertas.go — Cuentas por cobrar y por pagar como tablero vivo.
//
// LA IDEA QUE ORDENA ESTE ARCHIVO.
//
// Un saldo no es información útil hasta que dice QUIÉN está mal y DESDE CUÁNDO.
// «Se deben 480.000» no mueve a nadie; «Ferretería El Tornillo debe 120.000 hace
// 96 días, y es el 31% de todo lo que hay por cobrar» sí. Y eso no puede ser una
// cuenta que alguien hace a mano cada mes: aquí se recalcula cada vez que se abre
// el tablero, y también cada vez que un agente llama a `finance.scan_alerts`.
//
// POR QUÉ LA ALERTA SE GUARDA Y NO SOLO SE DIBUJA.
//
// Porque el humano y la IA tienen que mirar LA MISMA lista. Si las alertas se
// calcularan en el navegador, un agente no las vería; si se calcularan en el
// agente, el humano no sabría qué dijo el agente. Guardadas en `finance_alert`,
// con su estado y su historial de avisos, hay una sola verdad: la que se ve en
// pantalla es la que el agente reporta, y la que el agente crea es la que la
// persona encuentra abierta al entrar.
//
// POR QUÉ SE CIERRAN SOLAS. El escaneo recalcula y marca `cerrada` toda alerta
// abierta cuya condición ya no se cumple —el cliente pagó, la deuda bajó—. Sin
// eso, la lista se convierte en un cementerio de avisos viejos y en dos semanas
// nadie la mira. Lo que sí respeta es la decisión humana: una alerta `resuelta` o
// `descartada` a mano no se reabre; si el problema vuelve con otra referencia,
// nace una alerta nueva, que es exactamente lo que uno quiere saber.
//
// LO QUE ESTE TABLERO NO PUEDE SABER, y conviene decirlo en la pantalla: la tabla
// `invoice` no guarda fecha de vencimiento. La antigüedad se mide desde la fecha
// de la factura, que es el criterio disponible y honesto; el día que se cargue el
// vencimiento por factura, cambia el origen de la resta y nada más.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	scopeCxC = "cxc"
	scopeCxP = "cxp"

	// Días de antigüedad a partir de los cuales una deuda deja de ser «reciente»
	// y pasa a ser un problema que hay que hablar con alguien.
	finDiasAviso   = 30
	finDiasCritico = 90

	// Un cliente con esta porción del total por cobrar es un riesgo de
	// concentración: si no paga, el problema es de la empresa, no del cliente.
	finConcentracionPct = 25.0

	// El tablero recalcula al abrirse, pero no más de una vez cada tanto: abrir y
	// cerrar la pantalla diez veces no tiene por qué recorrer la cartera diez veces.
	finEscaneoCache = 5 * time.Minute

	alertaAbierta  = "abierta"
	alertaCerrada  = "cerrada"
	alertaResuelta = "resuelta"
	alertaDescart  = "descartada"
)

// finEtiquetaAlerta traduce el motivo interno a lo que se lee en pantalla. Vive
// aquí y no en la plantilla para que la web, el correo de aviso y el informe del
// agente digan lo mismo.
var finEtiquetaAlerta = map[string]string{
	"cartera_vencida":           "Cartera vencida",
	"cartera_mora_anormal":      "Mora anormal",
	"cartera_concentracion":     "Concentración en un cliente",
	"pagar_vencida":             "Cuenta por pagar vencida",
	"pagar_retencion_pendiente": "Retención de IVA sin enviar",
	"pagar_concentracion":       "Concentración en un proveedor",
}

func finEtiquetaKind(kind string) string {
	if t, ok := finEtiquetaAlerta[kind]; ok {
		return t
	}
	return kind
}

func (s *Server) registerFinanceAlertRoutes() {
	s.mux.HandleFunc("GET /cartera/panel", s.financePanelCxC)
	s.mux.HandleFunc("POST /cartera/escanear", s.financeEscanearCxC)
	s.mux.HandleFunc("POST /cartera/avisar", s.financeAvisarCxC)
	s.mux.HandleFunc("GET /pagar/panel", s.financePanelCxP)
	s.mux.HandleFunc("POST /pagar/escanear", s.financeEscanearCxP)
	s.mux.HandleFunc("POST /pagar/avisar", s.financeAvisarCxP)
	s.mux.HandleFunc("POST /finance/alerta/resolver/{id}", s.financeResolverAlertaHandler)
	s.mux.HandleFunc("GET /finance/mensajes", s.financeMensajesHandler)
}

// ---------- Fechas y claves ----------

// finFechaDias devuelve hoy desplazado n días en el formato en que el sistema
// guarda las fechas (varchar con AAAA-MM-DD delante).
func finFechaDias(n int) string {
	return time.Now().AddDate(0, 0, n).Format("2006-01-02")
}

// finFechaCorta toma los primeros 10 caracteres de una fecha guardada como
// texto. Las columnas del ERP son varchar con «AAAA-MM-DD» o «AAAA-MM-DD
// HH:MM:SS» según el origen, y recortar el prefijo es más robusto que adivinar
// el formato completo.
func finFechaCorta(v string) string {
	t := strings.TrimSpace(v)
	t = strings.ReplaceAll(t, "/", "-")
	if len(t) > 10 {
		t = t[:10]
	}
	return t
}

// finDiasDesde devuelve la antigüedad en días de una fecha ya guardada. -1
// significa «no se pudo leer la fecha», que es distinto de 0 (es de hoy) y no se
// puede confundir: una fecha ilegible no debe generar una alerta por mora.
func finDiasDesde(fecha string) int {
	f := finFechaCorta(fecha)
	if len(f) != 10 {
		return -1
	}
	t, err := time.Parse("2006-01-02", f)
	if err != nil {
		return -1
	}
	d := int(time.Since(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// finBucket clasifica una antigüedad en los tramos del reporte de antigüedad.
// Los tramos son los clásicos del cobrador porque son los que se accionan: hasta
// un mes se llama; de uno a dos se reclama por escrito; de dos a tres se avisa
// que se suspende el crédito; más de tres ya es decisión de la gerencia.
func finBucket(dias int) string {
	switch {
	case dias < 0:
		return "sin fecha"
	case dias <= 30:
		return "0-30"
	case dias <= 60:
		return "31-60"
	case dias <= 90:
		return "61-90"
	default:
		return "+90"
	}
}

// finHuella es la identidad de una alerta. Incluye el motivo, la contraparte y
// la referencia: la MISMA condición produce la misma huella y por tanto no se
// duplica, mientras que un problema nuevo sobre la misma contraparte (otra
// factura, otro período) es otra alerta y se ve aparte.
func finHuella(parts ...string) string {
	return aiHashKey("alerta|" + strings.Join(parts, "|"))
}

// finJSONMap lee el detalle guardado de una alerta sin romper nada si viene
// ilegible: una alerta sin detalle es mejor que una pantalla en blanco.
func finJSONMap(s string) map[string]interface{} {
	m := map[string]interface{}{}
	if strings.TrimSpace(s) == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func finMotivoDe(m map[string]interface{}) string {
	if v, ok := m["motivo"].(string); ok {
		return v
	}
	return ""
}

// ---------- Contactos ----------

// finContactoCliente devuelve a quién se le puede avisar, tomado DEL REGISTRO.
//
// Esto no es un detalle de implementación: es la razón por la que un agente puede
// usar esto sin poder mandar datos a donde quiera. El destinatario no lo elige el
// modelo ni el formulario; sale de la ficha del cliente. Si el correo está mal,
// se corrige la ficha, no el envío.
func (s *Server) finContactoCliente(db *sql.DB, id int64) (nombre, email, movil string) {
	var n, e, e2, m sql.NullString
	_ = db.QueryRow(`SELECT IFNULL(customer_name,''), IFNULL(customer_email,''), IFNULL(email_address,''), IFNULL(customer_mobile,'')
		FROM customer_information WHERE customer_id = ?`, id).Scan(&n, &e, &e2, &m)
	email = strings.TrimSpace(e.String)
	if email == "" {
		email = strings.TrimSpace(e2.String)
	}
	return n.String, email, strings.TrimSpace(m.String)
}

// finContactoProveedor hace lo mismo con un proveedor. `emailnumber` es una
// columna heredada del PHP original que en la práctica guarda el correo; se mira
// después de `email_address` porque ese es el campo del formulario actual.
func (s *Server) finContactoProveedor(db *sql.DB, id int64) (nombre, email, movil string) {
	var n, e, e2, m sql.NullString
	_ = db.QueryRow(`SELECT IFNULL(supplier_name,''), IFNULL(email_address,''), IFNULL(emailnumber,''), IFNULL(mobile,'')
		FROM supplier_information WHERE supplier_id = ?`, id).Scan(&n, &e, &e2, &m)
	email = strings.TrimSpace(e.String)
	if email == "" {
		email = strings.TrimSpace(e2.String)
	}
	return n.String, email, strings.TrimSpace(m.String)
}

// ---------- Carga de la cartera ----------

// finFacturaAbierta es una factura con saldo, ya con su antigüedad calculada.
type finFacturaAbierta struct {
	CustomerID int64
	Customer   string
	Email      string
	Mobile     string
	InvoiceID  int64
	InvoiceNo  int64
	Fecha      string
	Dias       int
	Due        float64
}

// finFacturasAbiertas trae la cartera viva. El filtro por `due_amount > 0` y
// `is_anulled = 0` deja fuera lo cobrado y lo anulado: una factura anulada no es
// una deuda, y contarla infla la mora y hace que se le reclame a un cliente algo
// que no debe.
func (s *Server) finFacturasAbiertas(db *sql.DB) []finFacturaAbierta {
	out := []finFacturaAbierta{}
	rows, err := db.Query(`SELECT i.customer_id, IFNULL(c.customer_name,''), IFNULL(c.customer_email,''),
		IFNULL(c.email_address,''), IFNULL(c.customer_mobile,''),
		IFNULL(i.invoice_id,0), IFNULL(i.invoice,0), IFNULL(CAST(i.date AS CHAR),''), i.due_amount
		FROM invoice i
		LEFT JOIN customer_information c ON c.customer_id = i.customer_id
		WHERE i.due_amount > 0.005 AND IFNULL(i.is_anulled,0) = 0
		ORDER BY i.date ASC, i.id ASC LIMIT 5000`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f finFacturaAbierta
		var mail, mail2 sql.NullString
		if err := rows.Scan(&f.CustomerID, &f.Customer, &mail, &mail2, &f.Mobile,
			&f.InvoiceID, &f.InvoiceNo, &f.Fecha, &f.Due); err != nil {
			continue
		}
		f.Email = strings.TrimSpace(mail.String)
		if f.Email == "" {
			f.Email = strings.TrimSpace(mail2.String)
		}
		f.Dias = finDiasDesde(f.Fecha)
		f.Fecha = finFechaCorta(f.Fecha)
		out = append(out, f)
	}
	return out
}

// finReferencia es el documento que originó una deuda con un proveedor, ya
// redactado para que el aviso diga «Compra #12 · factura 00123» y no «purchase:12».
type finReferencia struct {
	Ref string
	Doc string
}

// finDeudaProveedor es una cuenta por pagar viva: compra con saldo o servicio
// registrado y no pagado.
type finDeudaProveedor struct {
	SupplierID int64
	Supplier   string
	Email      string
	Mobile     string
	RefType    string
	RefID      string
	Doc        string
	Fecha      string
	Dias       int
	Due        float64
}

func (s *Server) finDeudasProveedor(db *sql.DB) []finDeudaProveedor {
	out := []finDeudaProveedor{}

	rows, err := db.Query(`SELECT p.supplier_id, IFNULL(s.supplier_name,''), IFNULL(s.email_address,''),
		IFNULL(s.emailnumber,''), IFNULL(s.mobile,''), p.purchase_id, IFNULL(p.chalan_no,''),
		IFNULL(p.control_no,''), IFNULL(CAST(p.purchase_date AS CHAR),''), p.due_amount
		FROM product_purchase p
		LEFT JOIN supplier_information s ON s.supplier_id = p.supplier_id
		WHERE p.due_amount > 0.005
		ORDER BY p.purchase_date ASC, p.id ASC LIMIT 5000`)
	if err != nil {
		return out
	}
	for rows.Next() {
		var d finDeudaProveedor
		var mail, mail2, chalan, control sql.NullString
		var pid int64
		if err := rows.Scan(&d.SupplierID, &d.Supplier, &mail, &mail2, &d.Mobile, &pid,
			&chalan, &control, &d.Fecha, &d.Due); err != nil {
			continue
		}
		d.Email = strings.TrimSpace(mail.String)
		if d.Email == "" {
			d.Email = strings.TrimSpace(mail2.String)
		}
		d.RefType = "purchase"
		d.RefID = strconv.FormatInt(pid, 10)
		d.Doc = "Compra #" + d.RefID
		if chalan.String != "" {
			d.Doc += " · factura " + chalan.String
		}
		if control.String != "" {
			d.Doc += " · control " + control.String
		}
		d.Dias = finDiasDesde(d.Fecha)
		d.Fecha = finFechaCorta(d.Fecha)
		out = append(out, d)
	}
	rows.Close()

	rows2, err := db.Query(`SELECT ap.supplier_id, IFNULL(s.supplier_name,''), IFNULL(s.email_address,''),
		IFNULL(s.emailnumber,''), IFNULL(s.mobile,''), ap.id, IFNULL(ap.concept,''),
		IFNULL(CAST(ap.created_at AS CHAR),''), ap.due_amount
		FROM accounts_payable ap
		LEFT JOIN supplier_information s ON s.supplier_id = ap.supplier_id
		WHERE ap.due_amount > 0.005 AND ap.origin = 'servicio'
		ORDER BY ap.created_at ASC LIMIT 5000`)
	if err != nil {
		return out
	}
	defer rows2.Close()
	for rows2.Next() {
		var d finDeudaProveedor
		var mail, mail2, concepto sql.NullString
		var aid int64
		if err := rows2.Scan(&d.SupplierID, &d.Supplier, &mail, &mail2, &d.Mobile, &aid,
			&concepto, &d.Fecha, &d.Due); err != nil {
			continue
		}
		d.Email = strings.TrimSpace(mail.String)
		if d.Email == "" {
			d.Email = strings.TrimSpace(mail2.String)
		}
		d.RefType = "servicio"
		d.RefID = strconv.FormatInt(aid, 10)
		d.Doc = concepto.String
		if d.Doc == "" {
			d.Doc = "Servicio #" + d.RefID
		}
		d.Dias = finDiasDesde(d.Fecha)
		d.Fecha = finFechaCorta(d.Fecha)
		out = append(out, d)
	}
	return out
}

// ---------- Indicadores ----------

type finSaldoParte struct {
	ID      int64
	Nombre  string
	Email   string
	Movil   string
	Total   float64
	Vencido float64
	Dias    int
	N       int
}

type finRetencion struct {
	SupplierID int64
	Supplier   string
	Email      string
	Movil      string
	Periodo    string
	N          int
	Base       float64
	IVA        float64
	Retenido   float64
}

// finSnapshotCxC arma los indicadores del lado por cobrar.
func (s *Server) finSnapshotCxC(db *sql.DB) map[string]interface{} {
	facturas := s.finFacturasAbiertas(db)

	partes := map[int64]*finSaldoParte{}
	total, vencido, mayor := 0.0, 0.0, 0.0
	tramos := map[string]float64{"0-30": 0, "31-60": 0, "61-90": 0, "+90": 0, "sin fecha": 0}
	tramosN := map[string]int{}
	facturasVencidas := 0
	for _, f := range facturas {
		total += f.Due
		p := partes[f.CustomerID]
		if p == nil {
			p = &finSaldoParte{ID: f.CustomerID, Nombre: f.Customer, Email: f.Email, Movil: f.Mobile}
			partes[f.CustomerID] = p
		}
		p.Total += f.Due
		p.N++
		if f.Dias > p.Dias {
			p.Dias = f.Dias
		}
		if f.Dias >= finDiasAviso {
			p.Vencido += f.Due
			vencido += f.Due
			facturasVencidas++
		}
		b := finBucket(f.Dias)
		tramos[b] += f.Due
		tramosN[b]++
	}
	for _, p := range partes {
		if p.Total > mayor {
			mayor = p.Total
		}
	}

	lista := make([]*finSaldoParte, 0, len(partes))
	for _, p := range partes {
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Total > lista[j].Total })

	top := []map[string]interface{}{}
	for i, p := range lista {
		if i >= 5 {
			break
		}
		top = append(top, map[string]interface{}{
			"customer_id": p.ID, "customer": p.Nombre,
			"due": money(p.Total), "due_num": round2(p.Total),
			"dias": p.Dias, "facturas": p.N, "email": p.Email, "mobile": p.Movil,
		})
	}
	top3 := 0.0
	for i, p := range lista {
		if i >= 3 {
			break
		}
		top3 += p.Total
	}
	top3Pct := 0.0
	if total > 0 {
		top3Pct = round2(top3 / total * 100)
	}
	pctVencido := 0.0
	if total > 0 {
		pctVencido = round2(vencido / total * 100)
	}

	// Días de cobro (DSO): cuántos días tarda en volver, en promedio, el dinero
	// que se factura. Se calcula sobre lo facturado de los últimos 30 días para
	// que mida el ritmo actual y no el histórico completo.
	var facturado30, facturado30Prev, cobrado30, cobrado30Prev float64
	_ = db.QueryRow(`SELECT IFNULL(SUM(total_amount),0) FROM invoice
		WHERE IFNULL(is_anulled,0) = 0 AND `+s.sqlDateYMD("date")+` >= ? AND `+s.sqlDateYMD("date")+` < ?`,
		finFechaDias(-30), finFechaDias(0)).Scan(&facturado30)
	_ = db.QueryRow(`SELECT IFNULL(SUM(total_amount),0) FROM invoice
		WHERE IFNULL(is_anulled,0) = 0 AND `+s.sqlDateYMD("date")+` >= ? AND `+s.sqlDateYMD("date")+` < ?`,
		finFechaDias(-60), finFechaDias(-30)).Scan(&facturado30Prev)
	_ = db.QueryRow(`SELECT IFNULL(SUM(amount),0) FROM debt_collection
		WHERE `+s.sqlDateYMD("collection_date")+` >= ?`, finFechaDias(-30)).Scan(&cobrado30)
	_ = db.QueryRow(`SELECT IFNULL(SUM(amount),0) FROM debt_collection
		WHERE `+s.sqlDateYMD("collection_date")+` >= ? AND `+s.sqlDateYMD("collection_date")+` < ?`,
		finFechaDias(-60), finFechaDias(-30)).Scan(&cobrado30Prev)
	dso := 0.0
	if facturado30 > 0 {
		dso = round2(total / facturado30 * 30)
	}
	variacion := 0.0
	if cobrado30Prev > 0 {
		variacion = round2((cobrado30 - cobrado30Prev) / cobrado30Prev * 100)
	}

	clientesVencidos := 0
	for _, p := range partes {
		if p.Vencido > 0.005 {
			clientesVencidos++
		}
	}

	abiertas, criticas := s.finContarAlertas(db, scopeCxC)

	tramosOut := []map[string]interface{}{}
	for _, b := range []string{"0-30", "31-60", "61-90", "+90", "sin fecha"} {
		if tramosN[b] == 0 {
			continue
		}
		pct := 0.0
		if total > 0 {
			pct = round2(tramos[b] / total * 100)
		}
		tramosOut = append(tramosOut, map[string]interface{}{
			"label": b, "amount": money(tramos[b]), "amount_num": round2(tramos[b]),
			"facturas": tramosN[b], "pct": pct,
		})
	}

	mayorNombre, mayorMonto := "", 0.0
	if len(lista) > 0 {
		mayorNombre, mayorMonto = lista[0].Nombre, lista[0].Total
	}

	semaforos := []map[string]interface{}{
		finSemaforo("Vencido", money(vencido), pctVencido, 20, "% del saldo con más de 30 días",
			"El saldo vencido es el que ya se debía haber cobrado. Por encima del 20% del total, la cobranza es el problema del mes."),
		finSemaforo("Días de cobro (DSO)", fmt.Sprintf("%.0f", dso), dso, 45, "días",
			"Cuánto tarda en volver el dinero facturado, medido contra lo facturado de los últimos 30 días."),
		finSemaforo("Cobrado (30 días)", money(cobrado30), -variacion, 0, "% de variación vs 30 días previos",
			"Si baja, o se cobra peor o se facturó menos: conviene mirar las dos cosas antes de celebrar."),
		finSemaforo("Concentración top 3", fmt.Sprintf("%.0f%%", top3Pct), top3Pct, finConcentracionPct, "del total",
			"Con tres clientes concentrando una cuarta parte de la cartera, un problema de uno es un problema de todos."),
	}

	return map[string]interface{}{
		"por_cobrar":         money(total),
		"por_cobrar_num":     round2(total),
		"vencido":            money(vencido),
		"vencido_num":        round2(vencido),
		"no_vencido":         money(round2(total - vencido)),
		"pct_vencido":        pctVencido,
		"dso_dias":           dso,
		"facturado_30":       money(facturado30),
		"facturado_30_prev":  money(facturado30Prev),
		"cobrado_30":         money(cobrado30),
		"cobrado_30_prev":    money(cobrado30Prev),
		"cobrado_variacion":  variacion,
		"clientes":           len(partes),
		"clientes_vencidos":  clientesVencidos,
		"facturas_abiertas":  len(facturas),
		"facturas_vencidas":  facturasVencidas,
		"mayor_deudor":       mayorNombre,
		"mayor_deudor_monto": money(mayorMonto),
		"concentracion_top3": top3Pct,
		"tramos":             tramosOut,
		"top":                top,
		"semaforos":          semaforos,
		"alertas_abiertas":   abiertas,
		"alertas_criticas":   criticas,
	}
}

// finRetencionesPendientes agrupa las retenciones de IVA por proveedor y período.
//
// POR QUÉ IMPORTA EN CUENTAS POR PAGAR. La retención que se le practica a un
// proveedor es dinero del proveedor que la empresa retiene y entera al SENIAT; el
// comprobante tiene que llegarle, y hoy eso se hacía fuera del sistema, cuando se
// hacía. Con el comprobante vencido, el proveedor no puede descontar su crédito y
// la llamada cae en el área que lo cargó, no en el SENIAT.
func (s *Server) finRetenciones(db *sql.DB) []finRetencion {
	out := []finRetencion{}
	rows, err := db.Query(`SELECT v.supplier_id, IFNULL(s.supplier_name,''), IFNULL(s.email_address,''),
		IFNULL(s.emailnumber,''), IFNULL(s.mobile,''), IFNULL(v.period,''), COUNT(*),
		IFNULL(SUM(v.base_amount),0), IFNULL(SUM(v.vat_amount),0), IFNULL(SUM(v.retention_amount),0)
		FROM vat_retention v
		LEFT JOIN supplier_information s ON s.supplier_id = v.supplier_id
		WHERE v.supplier_id IS NOT NULL
		GROUP BY v.supplier_id, s.supplier_name, s.email_address, s.emailnumber, s.mobile, IFNULL(v.period,'')
		ORDER BY IFNULL(v.period,'') DESC, v.supplier_id ASC LIMIT 500`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var r finRetencion
		var mail, mail2 sql.NullString
		if err := rows.Scan(&r.SupplierID, &r.Supplier, &mail, &mail2, &r.Movil, &r.Periodo,
			&r.N, &r.Base, &r.IVA, &r.Retenido); err != nil {
			continue
		}
		r.Email = strings.TrimSpace(mail.String)
		if r.Email == "" {
			r.Email = strings.TrimSpace(mail2.String)
		}
		out = append(out, r)
	}
	return out
}

// finSnapshotCxP arma los indicadores del lado por pagar.
func (s *Server) finSnapshotCxP(db *sql.DB) map[string]interface{} {
	deudas := s.finDeudasProveedor(db)

	partes := map[int64]*finSaldoParte{}
	total, vencido := 0.0, 0.0
	tramos := map[string]float64{"0-30": 0, "31-60": 0, "61-90": 0, "+90": 0, "sin fecha": 0}
	tramosN := map[string]int{}
	vencidas := 0
	for _, d := range deudas {
		total += d.Due
		p := partes[d.SupplierID]
		if p == nil {
			p = &finSaldoParte{ID: d.SupplierID, Nombre: d.Supplier, Email: d.Email, Movil: d.Mobile}
			partes[d.SupplierID] = p
		}
		p.Total += d.Due
		p.N++
		if d.Dias > p.Dias {
			p.Dias = d.Dias
		}
		if d.Dias >= finDiasAviso {
			p.Vencido += d.Due
			vencido += d.Due
			vencidas++
		}
		b := finBucket(d.Dias)
		tramos[b] += d.Due
		tramosN[b]++
	}

	lista := make([]*finSaldoParte, 0, len(partes))
	for _, p := range partes {
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Total > lista[j].Total })
	top := []map[string]interface{}{}
	for i, p := range lista {
		if i >= 5 {
			break
		}
		top = append(top, map[string]interface{}{
			"supplier_id": p.ID, "supplier": p.Nombre,
			"due": money(p.Total), "due_num": round2(p.Total),
			"dias": p.Dias, "documentos": p.N, "email": p.Email, "mobile": p.Movil,
		})
	}
	top3 := 0.0
	for i, p := range lista {
		if i >= 3 {
			break
		}
		top3 += p.Total
	}
	top3Pct := 0.0
	if total > 0 {
		top3Pct = round2(top3 / total * 100)
	}
	pctVencido := 0.0
	if total > 0 {
		pctVencido = round2(vencido / total * 100)
	}

	// Retenciones: las del último año que todavía no tienen comprobante enviado.
	retenciones := s.finRetenciones(db)
	enviadas := s.finRefsEnviadas(db, "retencion_iva")
	retPend, retPendMonto, retPendDetalle := 0, 0.0, []map[string]interface{}{}
	for _, r := range retenciones {
		clave := fmt.Sprintf("supplier:%d:%s", r.SupplierID, r.Periodo)
		if enviadas[clave] {
			continue
		}
		retPend++
		retPendMonto += r.Retenido
		retPendDetalle = append(retPendDetalle, map[string]interface{}{
			"supplier_id": r.SupplierID, "supplier": r.Supplier, "period": r.Periodo,
			"comprobantes": r.N, "base": money(r.Base), "iva": money(r.IVA),
			"retenido": money(r.Retenido), "retenido_num": round2(r.Retenido),
			"email": r.Email, "mobile": r.Movil,
		})
	}

	var pagado30 float64
	_ = db.QueryRow(`SELECT IFNULL(SUM(paid_amount),0) FROM accounts_payable
		WHERE IFNULL(CAST(payment_date AS CHAR),'') >= ?`, finFechaDias(-30)).Scan(&pagado30)

	abiertas, criticas := s.finContarAlertas(db, scopeCxP)

	tramosOut := []map[string]interface{}{}
	for _, b := range []string{"0-30", "31-60", "61-90", "+90", "sin fecha"} {
		if tramosN[b] == 0 {
			continue
		}
		pct := 0.0
		if total > 0 {
			pct = round2(tramos[b] / total * 100)
		}
		tramosOut = append(tramosOut, map[string]interface{}{
			"label": b, "amount": money(tramos[b]), "amount_num": round2(tramos[b]),
			"documentos": tramosN[b], "pct": pct,
		})
	}

	semaforos := []map[string]interface{}{
		finSemaforo("Por pagar", money(total), 0, 0, "saldo vivo",
			"Lo que hay que pagar hoy: documentos de compra con saldo más servicios y gastos registrados."),
		finSemaforo("Vencido", money(vencido), pctVencido, 20, "% del total con más de 30 días",
			"Deuda con más de 30 días de antigüedad. Aquí no se pierde el cliente: se pierde el crédito con el proveedor."),
		finSemaforo("Retenciones sin enviar", strconv.Itoa(retPend), float64(retPend), -1, "comprobantes",
			"Comprobantes de retención de IVA registrados que todavía no se le han enviado al proveedor. Es dinero suyo en manos de la empresa: mientras no los reciba, no puede descontar su crédito."),
		finSemaforo("Concentración top 3", fmt.Sprintf("%.0f%%", top3Pct), top3Pct, finConcentracionPct, "del total",
			"Con tres proveedores concentrando una cuarta parte de la deuda, una negociación de precio se vuelve urgente."),
	}

	return map[string]interface{}{
		"por_pagar":              money(total),
		"por_pagar_num":          round2(total),
		"vencido":                money(vencido),
		"vencido_num":            round2(vencido),
		"no_vencido":             money(round2(total - vencido)),
		"pct_vencido":            pctVencido,
		"documentos_abiertos":    len(deudas),
		"documentos_vencidos":    vencidas,
		"proveedores":            len(partes),
		"pagado_30":              money(pagado30),
		"retenciones_pendientes": retPend,
		"retenciones_monto":      money(retPendMonto),
		"retenciones":            retPendDetalle,
		"concentracion_top3":     top3Pct,
		"tramos":                 tramosOut,
		"top":                    top,
		"semaforos":              semaforos,
		"alertas_abiertas":       abiertas,
		"alertas_criticas":       criticas,
	}
}

// finSemaforo arma un indicador con su umbral y su estado, para que la pantalla
// pueda pintarlo sin repetir la regla en JavaScript. La regla vive aquí, en un
// solo sitio, y es la misma que usa el agente cuando reporta.
//
// Un umbral negativo es la convención para «cualquier valor mayor que cero ya es
// un aviso»: hay cosas —un comprobante de retención sin enviar— que no empiezan a
// importar a partir de un número, importan desde la primera.
func finSemaforo(nombre, valorTxt string, valor, umbral float64, unidad, explicacion string) map[string]interface{} {
	estado := "ok"
	switch {
	case umbral < 0:
		if valor > 0 {
			estado = "aviso"
		}
		if valor >= 5 {
			estado = "critico"
		}
	case umbral > 0 && valor >= umbral:
		estado = "aviso"
		if valor >= umbral*1.5 {
			estado = "critico"
		}
	}
	return map[string]interface{}{
		"label": nombre, "value": valorTxt, "estado": estado, "unidad": unidad,
		"umbral": umbral, "detalle": explicacion,
	}
}

// ---------- Alertas: conteo, lectura y cierre ----------

func (s *Server) finContarAlertas(db *sql.DB, scope string) (abiertas, criticas int) {
	_ = db.QueryRow(`SELECT COUNT(*) FROM finance_alert WHERE scope = ? AND status = ?`, scope, alertaAbierta).Scan(&abiertas)
	_ = db.QueryRow(`SELECT COUNT(*) FROM finance_alert WHERE scope = ? AND status = ? AND severity = 'critico'`,
		scope, alertaAbierta).Scan(&criticas)
	return abiertas, criticas
}

func (s *Server) finAlertas(db *sql.DB, scope string, incluirCerradas bool, limit int) []map[string]interface{} {
	out := []map[string]interface{}{}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, scope, kind, severity, party_type, party_id, party_name, ref_type, ref_id,
		amount, days, IFNULL(metric_json,''), status, IFNULL(CAST(notified_at AS CHAR),''), notify_count,
		IFNULL(CAST(updated_at AS CHAR),'')
		FROM finance_alert WHERE scope = ?`
	if !incluirCerradas {
		q += ` AND status = '` + alertaAbierta + `'`
	}
	// Orden pensado para que lo primero sea lo que hay que hacer: primero
	// criticidad, después el que más debe, y al final el más antiguo.
	q += ` ORDER BY CASE severity WHEN 'critico' THEN 0 WHEN 'aviso' THEN 1 ELSE 2 END ASC,
		amount DESC, days DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := db.Query(q, scope)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, partyID int64
		var sc, kind, sev, ptype, pname, rtype, rid, metric, status, notified, updated sql.NullString
		var amount float64
		var days, notifyCount int
		if err := rows.Scan(&id, &sc, &kind, &sev, &ptype, &partyID, &pname, &rtype, &rid,
			&amount, &days, &metric, &status, &notified, &notifyCount, &updated); err != nil {
			continue
		}
		m := finJSONMap(metric.String)
		out = append(out, map[string]interface{}{
			"id": id, "scope": sc.String, "kind": kind.String, "kind_label": finEtiquetaKind(kind.String),
			"severity": sev.String, "party_type": ptype.String, "party_id": partyID,
			"party":        pname.String,
			"ref_type":     rtype.String,
			"ref_id":       rid.String,
			"amount":       money(amount),
			"amount_num":   round2(amount),
			"days":         days,
			"status":       status.String,
			"motivo":       finMotivoDe(m),
			"metricas":     m,
			"notified_at":  notified.String,
			"notify_count": notifyCount,
			"updated_at":   updated.String,
		})
	}

	// El contacto se resuelve DESPUÉS, con el cursor ya cerrado.
	//
	// No es un detalle de estilo. Consultar mientras el cursor está abierto ocupa
	// una conexión del pool; con un pool de una sola conexión —una instalación
	// chica, o una prueba— la segunda consulta espera a la primera para siempre y
	// el tablero se queda colgado en vez de mostrar las alertas. Se resuelve aquí
	// para que la pantalla pueda decir a dónde va a salir el aviso ANTES de
	// mandarlo: el destinatario no se escribe a mano, sale de la ficha, y verlo
	// evita el «¿a qué correo le llegó?» de después.
	rows.Close()
	for _, a := range out {
		partyID := int64(0)
		if v, ok := a["party_id"].(int64); ok {
			partyID = v
		}
		var email, movil string
		if fmt.Sprintf("%v", a["party_type"]) == "supplier" {
			_, email, movil = s.finContactoProveedor(db, partyID)
		} else {
			_, email, movil = s.finContactoCliente(db, partyID)
		}
		a["contact_email"] = email
		a["contact_mobile"] = movil
	}
	return out
}

func (s *Server) finAlertaPorID(db *sql.DB, id int64) (map[string]interface{}, error) {
	var row struct {
		ID, PartyID                                                                        int64
		Scope, Kind, Severity, PType, PName, RType, RID, Metric, Status, Notified, Updated sql.NullString
		Amount                                                                             float64
		Days, NotifyCount                                                                  int
	}
	err := db.QueryRow(`SELECT id, scope, kind, severity, party_type, party_id, party_name, ref_type, ref_id,
		amount, days, IFNULL(metric_json,''), status, IFNULL(CAST(notified_at AS CHAR),''), notify_count,
		IFNULL(CAST(updated_at AS CHAR),'')
		FROM finance_alert WHERE id = ?`, id).
		Scan(&row.ID, &row.Scope, &row.Kind, &row.Severity, &row.PType, &row.PartyID, &row.PName,
			&row.RType, &row.RID, &row.Amount, &row.Days, &row.Metric, &row.Status, &row.Notified,
			&row.NotifyCount, &row.Updated)
	if err != nil {
		return nil, fmt.Errorf("la alerta %d no existe", id)
	}
	m := finJSONMap(row.Metric.String)
	return map[string]interface{}{
		"id": row.ID, "scope": row.Scope.String, "kind": row.Kind.String,
		"kind_label": finEtiquetaKind(row.Kind.String), "severity": row.Severity.String,
		"party_type": row.PType.String, "party_id": row.PartyID, "party": row.PName.String,
		"ref_type": row.RType.String, "ref_id": row.RID.String,
		"amount": money(row.Amount), "amount_num": round2(row.Amount), "days": row.Days,
		"status": row.Status.String, "motivo": finMotivoDe(m), "metricas": m,
		"notified_at": row.Notified.String, "notify_count": row.NotifyCount,
		"updated_at": row.Updated.String,
	}, nil
}

// ---------- Escaneo ----------

// finGuardarAlerta inserta la alerta o refresca sus números si ya existía.
//
// Devuelve "nueva", "actualizada" o "ignorada". «Ignorada» es el caso de una
// alerta ya resuelta o descartada por una persona: el escaneo pasa por encima sin
// reabrirla, porque volver a levantar lo que alguien cerró a propósito es la forma
// más rápida de que dejen de mirar el tablero.
func (s *Server) finGuardarAlerta(db *sql.DB, scope, kind, severity, partyType string, partyID int64,
	partyName, refType, refID string, amount float64, days int, motivo string, metricas map[string]interface{}) string {

	if metricas == nil {
		metricas = map[string]interface{}{}
	}
	metricas["motivo"] = motivo
	huella := finHuella(scope, kind, partyType, strconv.FormatInt(partyID, 10), refID)

	_, err := db.Exec(`INSERT INTO finance_alert (scope, kind, severity, party_type, party_id, party_name,
		ref_type, ref_id, amount, days, metric_json, fingerprint, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?, 'abierta', ?, ?)`,
		scope, kind, severity, partyType, partyID, partyName, refType, refID,
		amount, days, mustJSON(metricas), huella, now(), now())
	if err == nil {
		return "nueva"
	}
	if !aiEsClaveDuplicada(err) {
		return "ignorada"
	}

	var status string
	if qerr := db.QueryRow(`SELECT status FROM finance_alert WHERE fingerprint = ?`, huella).Scan(&status); qerr != nil {
		return "ignorada"
	}
	if status == alertaResuelta || status == alertaDescart {
		return "ignorada"
	}
	// Reabrir una cerrada es correcto: se cerró porque la condición había dejado
	// de cumplirse, y si volvió a cumplirse, hay que volver a mirarla.
	_, _ = db.Exec(`UPDATE finance_alert SET severity = ?, party_name = ?, amount = ?, days = ?,
		metric_json = ?, status = ?, updated_at = ? WHERE fingerprint = ?`,
		severity, partyName, amount, days, mustJSON(metricas), alertaAbierta, now(), huella)
	return "actualizada"
}

// finCerrarHuerfanas cierra las alertas abiertas cuya condición ya no se cumple.
func (s *Server) finCerrarHuerfanas(db *sql.DB, scope string, vigentes []string) int64 {
	q := `UPDATE finance_alert SET status = ?, updated_at = ? WHERE scope = ? AND status = ?`
	args := []interface{}{alertaCerrada, now(), scope, alertaAbierta}
	if len(vigentes) > 0 {
		ph := make([]string, 0, len(vigentes))
		for range vigentes {
			ph = append(ph, "?")
		}
		q += ` AND fingerprint NOT IN (` + strings.Join(ph, ",") + `)`
		for _, h := range vigentes {
			args = append(args, h)
		}
	}
	res, err := db.Exec(q, args...)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// financeEscanear recalcula las alertas de un ámbito y devuelve qué cambió.
func (s *Server) financeEscanear(db *sql.DB, scope, usuario string, force bool) (map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("sin base de datos")
	}
	res := map[string]interface{}{"scope": scope, "nuevas": 0, "actualizadas": 0, "cerradas": 0, "ignoradas": 0, "reciente": false}

	if !force && s.finEscaneoReciente(db, scope) {
		res["reciente"] = true
		res["momento"] = s.finUltimoEscaneo(db, scope)
		return res, nil
	}

	nuevas, actualizadas, ignoradas := 0, 0, 0
	vigentes := []string{}

	anotar := func(estado string) {
		switch estado {
		case "nueva":
			nuevas++
		case "actualizada":
			actualizadas++
		default:
			ignoradas++
		}
	}

	switch scope {
	case scopeCxC:
		facturas := s.finFacturasAbiertas(db)

		partes := map[int64]*finSaldoParte{}
		total := 0.0
		for _, f := range facturas {
			total += f.Due
			p := partes[f.CustomerID]
			if p == nil {
				p = &finSaldoParte{ID: f.CustomerID, Nombre: f.Customer, Email: f.Email, Movil: f.Mobile, Dias: -1}
				partes[f.CustomerID] = p
			}
			p.Total += f.Due
			p.N++
			if f.Dias > p.Dias {
				p.Dias = f.Dias
			}
			if f.Dias >= finDiasAviso {
				p.Vencido += f.Due
			}
		}

		// La referencia es la factura MÁS ANTIGUA con saldo: cuando esa se cobra,
		// la huella cambia y la alerta se cierra sola aunque queden otras abiertas.
		refMasVieja := map[int64]string{}
		docMasViejo := map[int64]string{}
		for _, f := range facturas {
			if _, ok := refMasVieja[f.CustomerID]; !ok {
				refMasVieja[f.CustomerID] = strconv.FormatInt(f.InvoiceID, 10)
				no := fmt.Sprintf("#%d", f.InvoiceNo)
				if f.InvoiceNo == 0 {
					no = fmt.Sprintf("interna %d", f.InvoiceID)
				}
				docMasViejo[f.CustomerID] = fmt.Sprintf("factura %s del %s", no, f.Fecha)
			}
		}

		for id, p := range partes {
			if p.Vencido <= 0.005 || p.Dias < finDiasAviso {
				continue
			}
			ref := refMasVieja[id]
			doc := docMasViejo[id]

			sev := "aviso"
			if p.Dias >= finDiasCritico {
				sev = "critico"
			}
			motivo := fmt.Sprintf("%s debe %s desde hace %d días (%s). Es el saldo vencido de %d factura(s).",
				nombreODefecto(p.Nombre, "El cliente "+strconv.FormatInt(id, 10)), money(p.Vencido), p.Dias, doc, p.N)
			vigentes = append(vigentes, finHuella(scopeCxC, "cartera_vencida", "customer", strconv.FormatInt(id, 10), ref))
			anotar(s.finGuardarAlerta(db, scopeCxC, "cartera_vencida", sev, "customer", id,
				p.Nombre, "invoice", ref, p.Vencido, p.Dias, motivo, map[string]interface{}{
					"vencido": round2(p.Vencido), "total": round2(p.Total), "facturas": p.N, "dias": p.Dias,
					"referencia": doc,
				}))

			// Mora anormal: no basta con deber, hay que deber fuera de su propia
			// costumbre. Se compara contra los días que TARDA en pagar este cliente
			// según sus cobros reales; el umbral mínimo (60 días) evita señalar a
			// quien simplemente tiene un crédito a 30.
			prom := s.finPromedioPagoCliente(db, id)
			if prom >= 0 && p.Dias >= 60 && float64(p.Dias) >= prom*2 && prom > 0 {
				vigentes = append(vigentes, finHuella(scopeCxC, "cartera_mora_anormal", "customer", strconv.FormatInt(id, 10), ref))
				anotar(s.finGuardarAlerta(db, scopeCxC, "cartera_mora_anormal", "critico", "customer", id,
					p.Nombre, "invoice", ref, p.Vencido, p.Dias,
					fmt.Sprintf("%s tiene %d días de mora cuando históricamente paga en %.0f días: el doble de su costumbre. No es un atraso normal, es un cambio de comportamiento.",
						nombreODefecto(p.Nombre, "El cliente"), p.Dias, prom),
					map[string]interface{}{
						"vencido": round2(p.Vencido), "dias": p.Dias, "promedio_pago": round2(prom),
					}))
			}
		}

		// Concentración: no es un problema del cliente, es un riesgo de la empresa.
		if total > 0 && len(partes) >= 2 {
			for id, p := range partes {
				pct := p.Total / total * 100
				if pct < finConcentracionPct {
					continue
				}
				ref := refMasVieja[id]
				vigentes = append(vigentes, finHuella(scopeCxC, "cartera_concentracion", "customer", strconv.FormatInt(id, 10), ref))
				anotar(s.finGuardarAlerta(db, scopeCxC, "cartera_concentracion", "aviso", "customer", id,
					p.Nombre, "cartera", ref, p.Total, p.Dias,
					fmt.Sprintf("%s concentra el %.0f%% de todo lo que hay por cobrar (%s). Si esa cuenta se atrasa, no se atrasa un cliente: se atrasa la cobranza entera.",
						nombreODefecto(p.Nombre, "El cliente "+strconv.FormatInt(id, 10)), pct, money(p.Total)),
					map[string]interface{}{"porcentaje": round2(pct), "saldo": round2(p.Total)}))
			}
		}

	case scopeCxP:
		deudas := s.finDeudasProveedor(db)

		partes := map[int64]*finSaldoParte{}
		total := 0.0
		referencias := map[int64]finReferencia{}
		for _, d := range deudas {
			total += d.Due
			p := partes[d.SupplierID]
			if p == nil {
				p = &finSaldoParte{ID: d.SupplierID, Nombre: d.Supplier, Email: d.Email, Movil: d.Mobile, Dias: -1}
				partes[d.SupplierID] = p
			}
			p.Total += d.Due
			p.N++
			if d.Dias > p.Dias {
				p.Dias = d.Dias
			}
			if d.Dias >= finDiasAviso {
				p.Vencido += d.Due
			}
			if _, ok := referencias[d.SupplierID]; !ok {
				referencias[d.SupplierID] = finReferencia{Ref: d.RefType + ":" + d.RefID, Doc: d.Doc + " del " + d.Fecha}
			}
		}

		for id, p := range partes {
			if p.Vencido <= 0.005 || p.Dias < finDiasAviso {
				continue
			}
			r := referencias[id]
			sev := "aviso"
			if p.Dias >= finDiasCritico {
				sev = "critico"
			}
			vigentes = append(vigentes, finHuella(scopeCxP, "pagar_vencida", "supplier", strconv.FormatInt(id, 10), r.Ref))
			anotar(s.finGuardarAlerta(db, scopeCxP, "pagar_vencida", sev, "supplier", id,
				p.Nombre, "purchase", r.Ref, p.Vencido, p.Dias,
				fmt.Sprintf("Se le deben %s a %s desde hace %d días (%s). Con el proveedor, pagar tarde no se cobra en intereses: se cobra en crédito y en precio.",
					money(p.Vencido), nombreODefecto(p.Nombre, "el proveedor "+strconv.FormatInt(id, 10)), p.Dias, r.Doc),
				map[string]interface{}{
					"vencido": round2(p.Vencido), "total": round2(p.Total), "documentos": p.N, "dias": p.Dias,
					"referencia": r.Doc,
				}))
		}

		if total > 0 && len(partes) >= 2 {
			for id, p := range partes {
				pct := p.Total / total * 100
				if pct < finConcentracionPct {
					continue
				}
				r := referencias[id]
				vigentes = append(vigentes, finHuella(scopeCxP, "pagar_concentracion", "supplier", strconv.FormatInt(id, 10), r.Ref))
				anotar(s.finGuardarAlerta(db, scopeCxP, "pagar_concentracion", "aviso", "supplier", id,
					p.Nombre, "cartera", r.Ref, p.Total, p.Dias,
					fmt.Sprintf("%s concentra el %.0f%% de lo que se debe (%s). Depender de un solo proveedor es un riesgo de operación, no de contabilidad.",
						nombreODefecto(p.Nombre, "El proveedor "+strconv.FormatInt(id, 10)), pct, money(p.Total)),
					map[string]interface{}{"porcentaje": round2(pct), "saldo": round2(p.Total)}))
			}
		}

		// Retenciones de IVA sin comprobante enviado.
		enviadas := s.finRefsEnviadas(db, "retencion_iva")
		for _, r := range s.finRetenciones(db) {
			clave := fmt.Sprintf("supplier:%d:%s", r.SupplierID, r.Periodo)
			if enviadas[clave] {
				continue
			}
			if r.Retenido <= 0.005 {
				continue
			}
			vigentes = append(vigentes, finHuella(scopeCxP, "pagar_retencion_pendiente", "supplier",
				strconv.FormatInt(r.SupplierID, 10), r.Periodo))
			anotar(s.finGuardarAlerta(db, scopeCxP, "pagar_retencion_pendiente", "aviso", "supplier", r.SupplierID,
				r.Supplier, "retention", r.Periodo, r.Retenido, 0,
				fmt.Sprintf("Hay %d comprobante(s) de retención de IVA de %s (período %s) por %s sin enviarle a %s. Es dinero suyo retenido por la empresa: mientras no lo reciba, no puede descontar su crédito fiscal.",
					r.N, money(r.IVA), periodoLegible(r.Periodo), money(r.Retenido),
					nombreODefecto(r.Supplier, "el proveedor "+strconv.FormatInt(r.SupplierID, 10))),
				map[string]interface{}{
					"periodo": r.Periodo, "comprobantes": r.N, "base": round2(r.Base),
					"iva": round2(r.IVA), "retenido": round2(r.Retenido),
				}))
		}

	default:
		return nil, fmt.Errorf("ámbito de escaneo desconocido: %s", scope)
	}

	cerradas := s.finCerrarHuerfanas(db, scope, vigentes)
	s.finMarcarEscaneo(db, scope, fmt.Sprintf("%d nuevas, %d actualizadas, %d cerradas", nuevas, actualizadas, cerradas))

	res["nuevas"] = nuevas
	res["actualizadas"] = actualizadas
	res["cerradas"] = cerradas
	res["ignoradas"] = ignoradas
	res["momento"] = now()
	res["usuario"] = usuario
	return res, nil
}

// nombreODefecto evita que una alerta diga « debe 1.200» cuando la ficha no
// tiene nombre: la pantalla tiene que ser legible incluso con datos incompletos.
func nombreODefecto(nombre, defecto string) string {
	n := strings.TrimSpace(nombre)
	if n == "" {
		return defecto
	}
	return n
}

// periodoLegible convierte «202603» en «marzo 2026», que es como lo lee una
// persona.
func periodoLegible(p string) string {
	if len(p) != 6 {
		return p
	}
	meses := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
		"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	m, err := strconv.Atoi(p[4:])
	if err != nil || m < 1 || m > 12 {
		return p
	}
	return fmt.Sprintf("%s %s", meses[m-1], p[:4])
}

// finPromedioPagoCliente devuelve, en días, cuánto tarda este cliente en pagar
// según sus cobros reales. -1 significa «no hay historia»: un cliente nuevo no
// puede tener mora ANORMAL, porque no hay normal contra la que compararlo.
func (s *Server) finPromedioPagoCliente(db *sql.DB, customerID int64) float64 {
	rows, err := db.Query(`SELECT IFNULL(CAST(i.date AS CHAR),''), IFNULL(CAST(dc.collection_date AS CHAR),'')
		FROM debt_collection dc
		JOIN invoice i ON i.invoice_id = dc.invoice_id
		WHERE dc.customer_id = ? AND dc.invoice_id > 0
		ORDER BY dc.id DESC LIMIT 200`, customerID)
	if err != nil {
		return -1
	}
	defer rows.Close()
	suma, n := 0.0, 0
	for rows.Next() {
		var fi, fc sql.NullString
		if err := rows.Scan(&fi, &fc); err != nil {
			continue
		}
		d := finDiasEntre(fi.String, fc.String)
		if d < 0 {
			continue
		}
		suma += float64(d)
		n++
	}
	if n == 0 {
		return -1
	}
	return round2(suma / float64(n))
}

// finDiasEntre devuelve los días entre dos fechas guardadas como texto; -1 si
// alguna no se puede leer.
func finDiasEntre(desde, hasta string) int {
	a := finFechaCorta(desde)
	b := finFechaCorta(hasta)
	if len(a) != 10 || len(b) != 10 {
		return -1
	}
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return -1
	}
	d := int(tb.Sub(ta).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// finRefsEnviadas devuelve las referencias que YA tienen un mensaje enviado de
// ese tipo. Es lo que convierte «hay retenciones registradas» en «hay retenciones
// que todavía no se le mandaron a nadie».
func (s *Server) finRefsEnviadas(db *sql.DB, refType string) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Query(`SELECT ref_id FROM finance_message WHERE ref_type = ? AND status = 'enviado'`, refType)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ref sql.NullString
		if err := rows.Scan(&ref); err != nil {
			continue
		}
		out[ref.String] = true
	}
	return out
}

func (s *Server) finEscaneoReciente(db *sql.DB, scope string) bool {
	var ultimo sql.NullString
	if err := db.QueryRow(`SELECT IFNULL(CAST(last_run AS CHAR),'') FROM finance_scan_state WHERE scope = ?`, scope).
		Scan(&ultimo); err != nil {
		return false
	}
	return finDentroDe(ultimo.String, finEscaneoCache)
}

func (s *Server) finUltimoEscaneo(db *sql.DB, scope string) string {
	var ultimo sql.NullString
	_ = db.QueryRow(`SELECT IFNULL(CAST(last_run AS CHAR),'') FROM finance_scan_state WHERE scope = ?`, scope).Scan(&ultimo)
	return ultimo.String
}

// finDentroDe dice si una fecha-hora guardada como texto cae dentro de la
// ventana. Sin fecha legible, NO se considera reciente: es preferible recalcular
// de más que dejar el tablero congelado por un valor ilegible.
func finDentroDe(valor string, ventana time.Duration) bool {
	v := strings.TrimSpace(valor)
	if len(v) < 19 {
		return false
	}
	t, err := time.ParseInLocation(nowLayout, v[:19], time.Local)
	if err != nil {
		return false
	}
	return time.Since(t) < ventana
}

func (s *Server) finMarcarEscaneo(db *sql.DB, scope, resultado string) {
	_, err := db.Exec(`INSERT INTO finance_scan_state (scope, last_run, last_result) VALUES (?,?,?)
		ON DUPLICATE KEY UPDATE last_run = VALUES(last_run), last_result = VALUES(last_result)`,
		scope, now(), resultado)
	if err != nil {
		// SQLite no entiende ON DUPLICATE KEY; el estado del escaneo es una
		// comodidad, no un dato del negocio: si no se puede anotar, se recalcula y
		// se sigue. Nunca se cae la pantalla por esto.
		_, _ = db.Exec(`DELETE FROM finance_scan_state WHERE scope = ?`, scope)
		_, _ = db.Exec(`INSERT INTO finance_scan_state (scope, last_run, last_result) VALUES (?,?,?)`,
			scope, now(), resultado)
	}
}

// ---------- Plantillas de aviso ----------

// finEmpresa devuelve el nombre con el que firma la empresa.
func (s *Server) finEmpresa(db *sql.DB) string {
	var nombre sql.NullString
	_ = db.QueryRow(`SELECT IFNULL(company_name,'') FROM company_information WHERE status = 1 ORDER BY company_id LIMIT 1`).Scan(&nombre)
	return strings.TrimSpace(nombre.String)
}

// finPlantillaAviso redacta el texto del aviso.
//
// POR QUÉ EL TEXTO VIVE AQUÍ Y NO EN LA PANTALLA. Porque el aviso del humano y el
// del agente tienen que decir lo mismo, y porque el agente no debe inventar un
// texto: lo que el sistema sabe —cuánto, desde cuándo, con qué referencia— lo
// pone el sistema. El humano puede editarlo antes de enviarlo; el agente, no.
func (s *Server) finPlantillaAviso(db *sql.DB, alerta map[string]interface{}, kind, destinatario string) (asunto, cuerpo string) {
	empresa := s.finEmpresa(db)
	if empresa == "" {
		empresa = "La empresa"
	}
	parte := fmt.Sprintf("%v", alerta["party"])
	motivo := fmt.Sprintf("%v", alerta["motivo"])
	monto := fmt.Sprintf("%v", alerta["amount"])

	switch fmt.Sprintf("%v", alerta["kind"]) {
	case "pagar_retencion_pendiente":
		m := finJSONMap(mustJSON(alerta["metricas"]))
		periodo := periodoLegible(fmt.Sprintf("%v", m["periodo"]))
		asunto = "Comprobante de retención de IVA — " + periodo
		cuerpo = fmt.Sprintf("Estimados %s:\n\n"+
			"Les informamos que %s practicó retención de IVA por %s correspondiente al período %s.\n"+
			"Base imponible: %s\nIVA: %s\nRetenido: %s\nComprobantes: %v\n\n"+
			"El comprobante de retención queda a su disposición para que puedan descontar su crédito fiscal. "+
			"Si necesitan el archivo en el formato del SENIAT, indíquenlo en respuesta a este mensaje.\n\n"+
			"Saludos,\n%s",
			parte, empresa, monto, periodo, fmt.Sprintf("%v", m["base"]), fmt.Sprintf("%v", m["iva"]),
			monto, m["comprobantes"], empresa)
		return asunto, cuerpo
	case "pagar_vencida":
		asunto = "Estado de cuenta — pendiente con su empresa"
		cuerpo = fmt.Sprintf("Estimados %s:\n\n%s\n\n"+
			"Les agradecemos indicarnos la fecha de pago o, si ya lo hicieron, el número de referencia "+
			"para aplicarlo a su cuenta.\n\nSaludos,\n%s", parte, motivo, empresa)
		return asunto, cuerpo
	default:
		asunto = "Estado de cuenta — saldo pendiente"
		cuerpo = fmt.Sprintf("Estimados %s:\n\n%s\n\n"+
			"Les agradecemos indicarnos la fecha de pago o, si ya lo hicieron, el número de referencia "+
			"para aplicarlo a su cuenta. Si hay alguna factura en discusión, respóndanos y la revisamos antes "+
			"de reactivar el cobro.\n\nSaludos,\n%s", parte, motivo, empresa)
		return asunto, cuerpo
	}
}

// ---------- Handlers ----------

func (s *Server) financePanelCxC(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "customer_ledger") {
		return
	}
	db := s.dbFor(r)
	usuario := sessUserID(s.sessionFromRequest(r))

	escaneo, err := s.financeEscanear(db, scopeCxC, usuario, false)
	if err != nil {
		escaneo = map[string]interface{}{"error": err.Error()}
	}
	writeJSON(w, map[string]interface{}{
		"status":      1,
		"indicadores": s.finSnapshotCxC(db),
		"alertas":     s.finAlertas(db, scopeCxC, false, 100),
		"escaneo":     escaneo,
		"mensajes":    s.finMensajesUltimos(db, scopeCxC, 8),
		"nota": "Las alertas se recalculan cada vez que se abre este tablero y también desde la herramienta de los agentes: " +
			"los dos ven la misma lista. Una alerta se cierra sola cuando la deuda se cobra; una que usted marque como " +
			"resuelta o descartada no se vuelve a abrir. La antigüedad se mide desde la fecha de la factura, porque el " +
			"sistema todavía no guarda fecha de vencimiento por factura.",
	})
}

func (s *Server) financePanelCxP(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
	db := s.dbFor(r)
	usuario := sessUserID(s.sessionFromRequest(r))

	escaneo, err := s.financeEscanear(db, scopeCxP, usuario, false)
	if err != nil {
		escaneo = map[string]interface{}{"error": err.Error()}
	}
	writeJSON(w, map[string]interface{}{
		"status":      1,
		"indicadores": s.finSnapshotCxP(db),
		"alertas":     s.finAlertas(db, scopeCxP, false, 100),
		"escaneo":     escaneo,
		"mensajes":    s.finMensajesUltimos(db, scopeCxP, 8),
		"nota": "Se le debe a un proveedor por dos vías: documentos de compra con saldo y servicios o gastos registrados. " +
			"Las retenciones de IVA aparecen como alerta mientras no tengan comprobante enviado, porque es dinero del " +
			"proveedor que está en manos de la empresa.",
	})
}

func (s *Server) financeEscanearCxC(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "customer_ledger") {
		return
	}
	res, err := s.financeEscanear(s.dbFor(r), scopeCxC, sessUserID(s.sessionFromRequest(r)), true)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res["status"] = 1
	res["indicadores"] = s.finSnapshotCxC(s.dbFor(r))
	res["alertas"] = s.finAlertas(s.dbFor(r), scopeCxC, false, 100)
	writeJSON(w, res)
}

func (s *Server) financeEscanearCxP(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "supplier_payment") {
		return
	}
	res, err := s.financeEscanear(s.dbFor(r), scopeCxP, sessUserID(s.sessionFromRequest(r)), true)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res["status"] = 1
	res["indicadores"] = s.finSnapshotCxP(s.dbFor(r))
	res["alertas"] = s.finAlertas(s.dbFor(r), scopeCxP, false, 100)
	writeJSON(w, res)
}

func (s *Server) financeAvisarCxC(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "create", "customer_ledger") {
		return
	}
	s.financeAvisarDesdeAlerta(w, r, scopeCxC)
}

func (s *Server) financeAvisarCxP(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "create", "supplier_payment") {
		return
	}
	s.financeAvisarDesdeAlerta(w, r, scopeCxP)
}

// financeAvisarDesdeAlerta es el envío de un aviso desde el tablero, hecho por una
// persona.
//
// LAS TRES COMPROBACIONES QUE HACEN QUE ESTO SEA SEGURO, y las tres valen igual
// para el agente:
//  1. El destinatario sale del REGISTRO (ficha del cliente o del proveedor), no
//     del formulario. No hay campo «para» que se pueda apuntar a otra parte.
//  2. El canal tiene que existir, estar activo y estar configurado.
//  3. Nadie —ni la persona ni el agente— puede repetir el mismo aviso dentro del
//     enfriamiento del canal. Si de verdad hay que repetirlo, se pide a propósito
//     con `forzar`, y eso queda escrito en el mensaje.
func (s *Server) financeAvisarDesdeAlerta(w http.ResponseWriter, r *http.Request, scope string) {
	_ = r.ParseMultipartForm(32 << 20)
	db := s.dbFor(r)

	alertaID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("alert_id")), 10, 64)
	if alertaID <= 0 {
		jsonError(w, "Indique la alerta que se va a avisar.", http.StatusBadRequest)
		return
	}
	canalID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("channel_id")), 10, 64)
	if canalID <= 0 {
		jsonError(w, "Elija por qué canal sale el aviso. Si no aparece ninguno, configúrelo en Configuración > Canales de aviso.", http.StatusBadRequest)
		return
	}

	alerta, err := s.finAlertaPorID(db, alertaID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	if fmt.Sprintf("%v", alerta["scope"]) != scope {
		jsonError(w, "Esa alerta no pertenece a este módulo.", http.StatusBadRequest)
		return
	}
	if fmt.Sprintf("%v", alerta["status"]) != alertaAbierta {
		jsonError(w, "Esa alerta ya está "+fmt.Sprintf("%v", alerta["status"])+". No se le manda un aviso a un asunto cerrado.", http.StatusBadRequest)
		return
	}

	canal, err := s.notifyCanalPorID(db, canalID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if canal.IsActive != 1 {
		jsonError(w, "El canal «"+canal.Name+"» está desactivado.", http.StatusBadRequest)
		return
	}

	asunto := strings.TrimSpace(r.FormValue("subject"))
	cuerpo := strings.TrimSpace(r.FormValue("body"))
	if asunto == "" || cuerpo == "" {
		sa, sc := s.finPlantillaAviso(db, alerta, canal.Kind, "")
		if asunto == "" {
			asunto = sa
		}
		if cuerpo == "" {
			cuerpo = sc
		}
	}

	// Vista previa: el mismo texto que se va a mandar, antes de mandarlo. Existe
	// para que la persona lea lo que el sistema redacta y pueda corregirlo; sin
	// esto, el aviso sale con un texto que nadie vio.
	if r.FormValue("preview") == "1" {
		writeJSON(w, map[string]interface{}{
			"status": 1, "preview": true,
			"subject": asunto, "body": cuerpo,
			"canal": canal.Name, "kind": canal.Kind,
		})
		return
	}

	res, err := s.financeEnviarAviso(r, db, canal, alerta, asunto, cuerpo, "alerta",
		strconv.FormatInt(alertaID, 10), r.FormValue("forzar") == "1")
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if ok, _ := res["enviado"].(bool); ok {
		_, _ = db.Exec(`UPDATE finance_alert SET notified_at = ?, notify_count = notify_count + 1, updated_at = ? WHERE id = ?`,
			now(), now(), alertaID)
		s.appendAudit(r, sessUserID(s.sessionFromRequest(r)), clientIP(r), "create", "Cuentas por cobrar/pagar",
			"finance_message", fmt.Sprintf("%v", res["id"]), nil, map[string]interface{}{
				"alerta": alertaID, "canal": canal.Name, "destino": res["destino"],
				"scope": scope, "motivo": alerta["kind"],
			})
	}
	res["status"] = 1
	res["alerta"] = alertaID
	writeJSON(w, res)
}

func (s *Server) financeResolverAlertaHandler(w http.ResponseWriter, r *http.Request) {
	sess := s.sessionFromRequest(r)
	if sess == nil {
		jsonError(w, "No autorizado. Inicie sesión.", http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		jsonError(w, "Alerta inválida.", http.StatusBadRequest)
		return
	}
	_ = r.ParseMultipartForm(32 << 20)
	estado := strings.TrimSpace(r.FormValue("estado"))
	if estado != alertaResuelta && estado != alertaDescart && estado != alertaAbierta {
		jsonError(w, "Estado inválido: use resuelta, descartada o abierta.", http.StatusBadRequest)
		return
	}
	db := s.dbFor(r)
	alerta, err := s.finAlertaPorID(db, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	if !s.exigirPermiso(w, r, "read", permisoDelScope(fmt.Sprintf("%v", alerta["scope"]))) {
		return
	}
	nota := strings.TrimSpace(r.FormValue("nota"))
	metricas := finJSONMap(mustJSON(alerta["metricas"]))
	if nota != "" {
		metricas["nota_humana"] = nota
	}
	metricas["resuelto_por"] = sess.Username
	metricas["resuelto_at"] = now()
	if _, err := db.Exec(`UPDATE finance_alert SET status = ?, metric_json = ?, updated_at = ? WHERE id = ?`,
		estado, mustJSON(metricas), now(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.appendAudit(r, sessUserID(sess), clientIP(r), "update", "Alertas financieras", "finance_alert",
		strconv.FormatInt(id, 10), map[string]interface{}{"status": alerta["status"]},
		map[string]interface{}{"status": estado, "nota": nota})
	writeJSON(w, map[string]interface{}{
		"status": 1, "id": id, "estado": estado,
		"msg": "Alerta marcada como " + estado + ". El escaneo no la va a reabrir: si el problema vuelve con otra referencia, nace una alerta nueva.",
	})
}

// permisoDelScope traduce el ámbito de una alerta a su permiso de módulo. Una
// sola función para que el listado, las acciones y las herramientas de la IA
// coincidan: tres listas paralelas se desincronizan.
func permisoDelScope(scope string) string {
	if scope == scopeCxP {
		return "supplier_payment"
	}
	return "customer_ledger"
}

// finMensajesUltimos devuelve los últimos avisos de un ámbito, para la pantalla.
func (s *Server) finMensajesUltimos(db *sql.DB, scope string, limit int) []map[string]interface{} {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	refType := "alerta"
	q := `SELECT m.id, IFNULL(m.channel,''), IFNULL(m.channel_kind,''), IFNULL(m.recipient,''),
		IFNULL(m.subject,''), IFNULL(m.status,''), IFNULL(m.error,''), IFNULL(CAST(m.created_at AS CHAR),''),
		IFNULL(CAST(m.sent_at AS CHAR),''), IFNULL(m.created_by,''), IFNULL(m.ref_id,'')
		FROM finance_message m
		WHERE m.ref_type = ? AND (m.ref_id IN (SELECT CAST(id AS CHAR) FROM finance_alert WHERE scope = ?) OR ? = '')
		ORDER BY m.id DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := db.Query(q, refType, scope, "")
	if err != nil {
		return []map[string]interface{}{}
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var canal, kind, dest, asunto, estado, errv, creado, enviado, quien, ref sql.NullString
		if err := rows.Scan(&id, &canal, &kind, &dest, &asunto, &estado, &errv, &creado, &enviado, &quien, &ref); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "channel": canal.String, "channel_kind": kind.String, "recipient": dest.String,
			"subject": asunto.String, "status": estado.String, "error": errv.String,
			"created_at": creado.String, "sent_at": enviado.String, "created_by": quien.String, "ref_id": ref.String,
		})
	}
	return out
}

// financeMensajesHandler es el libro de avisos enviados: qué salió, a quién, por
// qué canal y si falló. Es la pantalla que responde «¿ya se le avisó?».
func (s *Server) financeMensajesHandler(w http.ResponseWriter, r *http.Request) {
	if !s.exigirPermiso(w, r, "read", "customer_ledger", "supplier_payment") {
		return
	}
	db := s.dbFor(r)
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	limit := 200

	q := `SELECT id, IFNULL(channel,''), IFNULL(channel_kind,''), IFNULL(recipient,''), IFNULL(subject,''),
		IFNULL(ref_type,''), IFNULL(ref_id,''), IFNULL(status,''), IFNULL(error,''),
		IFNULL(provider_ref,''), IFNULL(created_by,''), IFNULL(CAST(created_at AS CHAR),''), IFNULL(CAST(sent_at AS CHAR),'')
		FROM finance_message WHERE 1=1`
	args := []interface{}{}
	if scope == scopeCxC || scope == scopeCxP {
		q += ` AND ref_type = 'alerta' AND ref_id IN (SELECT CAST(id AS CHAR) FROM finance_alert WHERE scope = ?)`
		args = append(args, scope)
	}
	q += ` ORDER BY id DESC LIMIT ` + strconv.Itoa(limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	lista := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var canal, kind, dest, asunto, rtype, rid, estado, errv, prov, quien, creado, enviado sql.NullString
		if err := rows.Scan(&id, &canal, &kind, &dest, &asunto, &rtype, &rid, &estado, &errv, &prov, &quien, &creado, &enviado); err != nil {
			continue
		}
		lista = append(lista, map[string]interface{}{
			"id": id, "channel": canal.String, "channel_kind": kind.String, "recipient": dest.String,
			"subject": asunto.String, "ref_type": rtype.String, "ref_id": rid.String,
			"status": estado.String, "error": errv.String, "provider_ref": prov.String,
			"created_by": quien.String, "created_at": creado.String, "sent_at": enviado.String,
		})
	}

	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, map[string]interface{}{"status": 1, "mensajes": lista})
		return
	}
	s.render(w, r, "finance_messages.html", map[string]interface{}{
		"Title":    "Mensajes enviados",
		"Mensajes": lista,
	})
}
