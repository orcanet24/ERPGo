package web

// crm_remarketing_handlers.go — Pantallas y endpoints del Remarketing CRM.
//
// Los handlers son delgados: validan, delegan en crm_remarketing_service.go y
// traducen a JSON o plantilla. El remarketing comparte la misma filosofía que
// el CRM: cada pantalla, cada envío y cada segmento lleva su pista de auditoría.

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) registerRemarketingRoutes() {
	// Panel principal
	s.mux.HandleFunc("GET /crm/remarketing", s.remarketingHome)
	// Segmentos
	s.mux.HandleFunc("GET /crm/remarketing/segments", s.segmentList)
	s.mux.HandleFunc("POST /crm/remarketing/segment/save", s.segmentSave)
	s.mux.HandleFunc("GET /crm/remarketing/segment/preview/{id}", s.segmentPreview)
	s.mux.HandleFunc("POST /crm/remarketing/segment/delete/{id}", s.segmentDelete)
	// Plantillas
	s.mux.HandleFunc("GET /crm/remarketing/templates", s.templateList)
	s.mux.HandleFunc("GET /crm/remarketing/template/add", s.templateForm)
	s.mux.HandleFunc("GET /crm/remarketing/template/edit/{id}", s.templateForm)
	s.mux.HandleFunc("POST /crm/remarketing/template/save", s.templateSave)
	s.mux.HandleFunc("POST /crm/remarketing/template/delete/{id}", s.templateDelete)
	// Campañas
	s.mux.HandleFunc("GET /crm/remarketing/campaigns", s.campaignList)
	s.mux.HandleFunc("GET /crm/remarketing/campaign/add", s.campaignForm)
	s.mux.HandleFunc("GET /crm/remarketing/campaign/edit/{id}", s.campaignForm)
	s.mux.HandleFunc("GET /crm/remarketing/campaign/view/{id}", s.campaignView)
	s.mux.HandleFunc("POST /crm/remarketing/campaign/save", s.campaignSave)
	s.mux.HandleFunc("POST /crm/remarketing/campaign/launch/{id}", s.campaignLaunch)
	s.mux.HandleFunc("POST /crm/remarketing/campaign/cancel/{id}", s.campaignCancel)
}

// ---------- Panel ----------

func (s *Server) remarketingHome(w http.ResponseWriter, r *http.Request) {
	segmentos, _ := s.dbSegmentList(r)
	campanas, _ := s.dbCampaignList(r)
	plantillas, _ := s.dbTemplateList(r)

	s.render(w, r, "crm_remarketing_home.html", map[string]interface{}{
		"Title":      "Remarketing",
		"Segmentos":  segmentos,
		"Campañas":   campanas,
		"Plantillas": plantillas,
		"Canales":    s.notifyCanales(s.dbFor(r), true),
		"Nota":       "El remarketing usa sus contactos, clientes y embudo comercial existentes. No duplica datos; los reutiliza.",
	})
}

// ---------- Segmentos ----------

func (s *Server) segmentList(w http.ResponseWriter, r *http.Request) {
	segmentos, _ := s.dbSegmentList(r)
	s.render(w, r, "crm_remarketing_segments.html", map[string]interface{}{
		"Title":     "Segmentos",
		"Segmentos": segmentos,
	})
}

func (s *Server) dbSegmentList(r *http.Request) ([]CRMSegment, error) {
	rows, err := s.dbFor(r).Query(`SELECT id, IFNULL(name,''), IFNULL(description,''),
		IFNULL(rule_type,'no_purchase_60d'), IFNULL(rule_days,0), status
		FROM crm_remarketing_segment WHERE status > -1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CRMSegment
	for rows.Next() {
		var seg CRMSegment
		if rows.Scan(&seg.ID, &seg.Name, &seg.Description, &seg.RuleType, &seg.RuleDays, &seg.Status) == nil {
			list = append(list, seg)
		}
	}
	return list, rows.Err()
}

func (s *Server) segmentSave(w http.ResponseWriter, r *http.Request) {
	seg := &CRMSegment{
		ID:          parseID(r.FormValue("id")),
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		RuleType:    r.FormValue("rule_type"),
		Status:      1,
	}
	seg.RuleDays, _ = strconv.Atoi(r.FormValue("rule_days"))

	id, err := s.SaveSegment(r, seg)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.remarketingAudit(r, "save_segment", strconv.FormatInt(id, 10), nil, map[string]interface{}{"name": seg.Name})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Segmento guardado", "id": id})
}

func (s *Server) segmentPreview(w http.ResponseWriter, r *http.Request) {
	seg, err := s.LoadSegment(r, parseID(r.PathValue("id")))
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	contactos, err := s.crmSegmentContacts(r, seg)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "total": len(contactos), "contacts": contactos,
	})
}

func (s *Server) segmentDelete(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	// Borrado lógico: no borramos para no romper campañas que referencian el segmento
	if _, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_segment SET status=0, updated_at=? WHERE id=?`, now(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.remarketingAudit(r, "delete_segment", strconv.FormatInt(id, 10), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Segmento desactivado"})
}

// ---------- Plantillas ----------

func (s *Server) templateList(w http.ResponseWriter, r *http.Request) {
	plantillas, _ := s.dbTemplateList(r)
	s.render(w, r, "crm_remarketing_templates.html", map[string]interface{}{
		"Title":      "Plantillas",
		"Plantillas": plantillas,
	})
}

func (s *Server) dbTemplateList(r *http.Request) ([]CRMTemplate, error) {
	rows, err := s.dbFor(r).Query(`SELECT id, IFNULL(name,''), IFNULL(subject,''), LEFT(IFNULL(body,''),80), status
		FROM crm_remarketing_template WHERE status > -1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CRMTemplate
	for rows.Next() {
		var tpl CRMTemplate
		var bodyPreview string
		if rows.Scan(&tpl.ID, &tpl.Name, &tpl.Subject, &bodyPreview, &tpl.Status) == nil {
			tpl.Body = bodyPreview
			list = append(list, tpl)
		}
	}
	return list, rows.Err()
}

func (s *Server) templateForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "Nueva Plantilla"}
	if id := r.PathValue("id"); id != "" {
		tpl, err := s.LoadTemplate(r, parseID(id))
		if err != nil {
			http.Error(w, "Plantilla no encontrada", 404)
			return
		}
		data["Title"] = "Editar Plantilla"
		data["Tpl"] = tpl
	}
	s.render(w, r, "crm_remarketing_template_form.html", data)
}

func (s *Server) templateSave(w http.ResponseWriter, r *http.Request) {
	tpl := &CRMTemplate{
		ID:      parseID(r.FormValue("id")),
		Name:    r.FormValue("name"),
		Subject: r.FormValue("subject"),
		Body:    r.FormValue("body"),
		Status:  1,
	}
	id, err := s.SaveTemplate(r, tpl)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.remarketingAudit(r, "save_template", strconv.FormatInt(id, 10), nil, map[string]interface{}{"name": tpl.Name})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Plantilla guardada", "id": id})
}

func (s *Server) templateDelete(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_template SET status=0, updated_at=? WHERE id=?`, now(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.remarketingAudit(r, "delete_template", strconv.FormatInt(id, 10), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Plantilla desactivada"})
}

// ---------- Campañas ----------

type campaignListItem struct {
	ID          int64
	Name        string
	Segment     string
	Template    string
	Channel     string
	ChannelKind string
	Status      int
	StatusLabel string
	Total       int
	Sent        int
	Errors      int
	CreatedAt   string
}

func (s *Server) campaignList(w http.ResponseWriter, r *http.Request) {
	campanas, _ := s.dbCampaignList(r)
	s.render(w, r, "crm_remarketing_campaigns.html", map[string]interface{}{
		"Title":    "Campañas",
		"Campañas": campanas,
		"Canales":  s.notifyCanales(s.dbFor(r), true),
	})
}

func (s *Server) dbCampaignList(r *http.Request) ([]campaignListItem, error) {
	rows, err := s.dbFor(r).Query(`
		SELECT camp.id, IFNULL(camp.name,''),
		       IFNULL(seg.name,''), IFNULL(tpl.name,''),
		       IFNULL(nc.name,''), IFNULL(nc.kind,''),
		       camp.status, IFNULL(camp.total_recipients,0),
		       IFNULL(camp.sent_count,0), IFNULL(camp.error_count,0),
		       IFNULL(camp.created_at,'')
		FROM crm_remarketing_campaign camp
		LEFT JOIN crm_remarketing_segment seg ON seg.id = camp.segment_id
		LEFT JOIN crm_remarketing_template tpl ON tpl.id = camp.template_id
		LEFT JOIN notify_channel nc ON nc.id = camp.channel_id
		ORDER BY camp.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statusLabels := map[int]string{0: "borrador", 1: "enviando", 2: "enviada", 3: "cancelada"}

	var list []campaignListItem
	for rows.Next() {
		var c campaignListItem
		if rows.Scan(&c.ID, &c.Name, &c.Segment, &c.Template, &c.Channel, &c.ChannelKind,
			&c.Status, &c.Total, &c.Sent, &c.Errors, &c.CreatedAt) == nil {
			c.StatusLabel = statusLabels[c.Status]
			list = append(list, c)
		}
	}
	return list, rows.Err()
}

func (s *Server) campaignForm(w http.ResponseWriter, r *http.Request) {
	segmentos, _ := s.dbSegmentList(r)
	plantillas, _ := s.dbTemplateList(r)

	data := map[string]interface{}{
		"Title":      "Nueva Campaña",
		"Segmentos":  segmentos,
		"Plantillas": plantillas,
		"Canales":    s.notifyCanales(s.dbFor(r), true),
	}
	if id := r.PathValue("id"); id != "" {
		camp, err := s.loadCampaign(s.dbFor(r), parseID(id))
		if err != nil {
			http.Error(w, "Campaña no encontrada", 404)
			return
		}
		data["Title"] = "Editar Campaña"
		data["Camp"] = camp
	}
	s.render(w, r, "crm_remarketing_campaign_form.html", data)
}

func (s *Server) campaignSave(w http.ResponseWriter, r *http.Request) {
	camp := &CRMCampaign{
		ID:         parseID(r.FormValue("id")),
		Name:       r.FormValue("name"),
		SegmentID:  parseID(r.FormValue("segment_id")),
		TemplateID: parseID(r.FormValue("template_id")),
		ChannelID:  parseID(r.FormValue("channel_id")),
	}
	id, err := s.SaveCampaign(r, camp)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	s.remarketingAudit(r, "save_campaign", strconv.FormatInt(id, 10), nil, map[string]interface{}{"name": camp.Name})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Campaña guardada", "id": id, "redirect": "/crm/remarketing/campaigns"})
}

func (s *Server) campaignView(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	camp, err := s.loadCampaign(s.dbFor(r), id)
	if err != nil {
		http.Error(w, "Campaña no encontrada", 404)
		return
	}

	rows, err := s.dbFor(r).Query(`
		SELECT IFNULL(recipient,''), status, IFNULL(error_msg,''), IFNULL(sent_at,'')
		FROM crm_remarketing_recipient WHERE campaign_id = ? ORDER BY id LIMIT 500`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type recipientRow struct {
		To    string
		State string
		Err   string
		Sent  string
	}
	var recipients []recipientRow
	for rows.Next() {
		var rec recipientRow
		if rows.Scan(&rec.To, &rec.State, &rec.Err, &rec.Sent) == nil {
			recipients = append(recipients, rec)
		}
	}

	statusLabels := map[int]string{0: "borrador", 1: "enviando", 2: "enviada", 3: "cancelada"}
	s.render(w, r, "crm_remarketing_campaign_view.html", map[string]interface{}{
		"Title":       camp.Name,
		"Camp":        camp,
		"StatusLabel": statusLabels[camp.Status],
		"Recipients":  recipients,
	})
}

func (s *Server) campaignLaunch(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if err := s.lanzaCampaña(r, id); err != nil {
		jsonError(w, "No se pudo lanzar la campaña: "+err.Error(), 500)
		return
	}
	s.remarketingAudit(r, "launch_campaign", strconv.FormatInt(id, 10), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Campaña lanzada. Vuelve en unos segundos para ver los resultados.", "redirect": fmt.Sprintf("/crm/remarketing/campaign/view/%d", id)})
}

func (s *Server) campaignCancel(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, err := s.dbFor(r).Exec(`UPDATE crm_remarketing_campaign SET status=3, updated_at=? WHERE id=?`, now(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.remarketingAudit(r, "cancel_campaign", strconv.FormatInt(id, 10), nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Campaña cancelada"})
}

// ---------- Utilidades ----------

// remarketingAudit deja constancia de una operación de remarketing.
func (s *Server) remarketingAudit(r *http.Request, action, recordID string, oldV, newV interface{}) {
	userID := ""
	if sess := s.sessionFromRequest(r); sess != nil {
		userID = sessUserID(sess)
	}
	s.appendAudit(r, userID, "", action, "crm_remarketing", "crm_remarketing_campaign", recordID, oldV, newV)
}
