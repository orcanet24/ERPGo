package web

// mensajeria_plantillas.go — Plantillas de aviso, con vista previa.
//
// UNA PLANTILLA ES TEXTO CON HUECOS CONOCIDOS. No es un lenguaje: solo
// `{{cliente}}`, `{{monto}}`, `{{fecha}}`, `{{empresa}}`, `{{referencia}}` y
// `{{enlace}}`. El guardado rechaza cualquier otra variable, y el envío falla si
// falta un dato. Un aviso con «Hola , su saldo es » es peor que un aviso que no
// sale: el cliente lo lee y la empresa queda mal.
//
// LA PANTALLA ES JAVASCRIPT NATIVO. No usa jQuery: son tres llamadas `fetch` y
// un formulario. El token CSRF se lee de la cookie que ya publica el layout.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"erpgo/internal/mensajeria"
)

var reCodigoPlantilla = regexp.MustCompile(`^[a-z0-9_]{3,64}$`)

type msgPlantilla struct {
	ID            int64
	Code          string
	Area          string
	Name          string
	Language      string
	Body          string
	VariablesJSON string
	IsActive      bool
	CreatedBy     string
	UpdatedAt     string
}

func (s *Server) registerMensajeriaPlantillaRoutes() {
	s.mux.HandleFunc("GET /settings/mensajeria/plantillas", s.mensajeriaPlantillasForm)
	s.mux.HandleFunc("POST /settings/mensajeria/plantillas/guardar", s.mensajeriaPlantillaSave)
	s.mux.HandleFunc("POST /settings/mensajeria/plantillas/borrar", s.mensajeriaPlantillaDelete)
	s.mux.HandleFunc("POST /settings/mensajeria/plantillas/preview", s.mensajeriaPlantillaPreview)
}

func (s *Server) msgPlantillasDe(db *sql.DB) []msgPlantilla {
	out := []msgPlantilla{}
	rows, err := db.Query(`SELECT id, code, IFNULL(area,'general'), IFNULL(name,''), IFNULL(language,'es'),
		IFNULL(body,''), IFNULL(variables_json,''), IFNULL(is_active,1), IFNULL(created_by,''), IFNULL(updated_at,'')
		FROM msg_template ORDER BY area, code`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p msgPlantilla
		var activa int
		if err := rows.Scan(&p.ID, &p.Code, &p.Area, &p.Name, &p.Language, &p.Body,
			&p.VariablesJSON, &activa, &p.CreatedBy, &p.UpdatedAt); err == nil {
			p.IsActive = activa == 1
			out = append(out, p)
		}
	}
	return out
}

// msgPlantillaPorCodigo es la que usa el canal al enviar. Solo devuelve
// plantillas activas: una plantilla apagada no debe salir por un canal que
// todavía la referencia.
func (s *Server) msgPlantillaPorCodigo(db *sql.DB, code string) (msgPlantilla, error) {
	var p msgPlantilla
	var activa int
	err := db.QueryRow(`SELECT id, code, IFNULL(area,'general'), IFNULL(name,''), IFNULL(language,'es'),
		IFNULL(body,''), IFNULL(variables_json,''), IFNULL(is_active,1), IFNULL(created_by,''), IFNULL(updated_at,'')
		FROM msg_template WHERE code = ? AND is_active = 1`, strings.TrimSpace(code)).
		Scan(&p.ID, &p.Code, &p.Area, &p.Name, &p.Language, &p.Body, &p.VariablesJSON,
			&activa, &p.CreatedBy, &p.UpdatedAt)
	if err != nil {
		return p, fmt.Errorf("la plantilla «%s» no existe o está apagada", code)
	}
	p.IsActive = activa == 1
	return p, nil
}

func (s *Server) mensajeriaPlantillasForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "mensajeria_templates.html", map[string]interface{}{
		"Title":      "Plantillas de mensajería",
		"Plantillas": s.msgPlantillasDe(s.dbFor(r)),
		"Areas":      areasDeMensajeria,
		"Variables":  mensajeria.VariablesDeAviso,
		"Activa":     s.mensajeriaActiva(r),
		"Nota": "Una plantilla solo admite las variables de la lista. Si falta un dato al enviar, " +
			"el aviso queda en error en vez de salir con un hueco. Cambiar el texto no altera los " +
			"avisos que ya están en la cola.",
	})
}

func (s *Server) mensajeriaPlantillaSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	db := s.dbFor(r)
	if db == nil {
		jsonError(w, "No hay base de empresa disponible.", http.StatusServiceUnavailable)
		return
	}
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	code := strings.ToLower(strings.TrimSpace(r.FormValue("code")))
	area := strings.ToLower(strings.TrimSpace(r.FormValue("area")))
	nombre := strings.TrimSpace(r.FormValue("name"))
	cuerpo := strings.TrimSpace(r.FormValue("body"))
	activa := r.FormValue("is_active") == "1"

	if !reCodigoPlantilla.MatchString(code) {
		jsonError(w, "El código debe tener entre 3 y 64 caracteres: minúsculas, números y guion bajo (por ejemplo cxc_vencida_v1).", http.StatusBadRequest)
		return
	}
	if _, ok := areasDeMensajeria[area]; !ok {
		jsonError(w, "Elija el área de la plantilla.", http.StatusBadRequest)
		return
	}
	if nombre == "" {
		jsonError(w, "Póngale un nombre a la plantilla: es como la verá quien la elija.", http.StatusBadRequest)
		return
	}
	if err := mensajeria.ValidarPlantilla(cuerpo); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	variables, _ := json.Marshal(mensajeria.VariablesDe(cuerpo))

	if id <= 0 {
		_, err := db.Exec(`INSERT INTO msg_template (code, area, name, language, body, variables_json,
			is_active, created_by, updated_at) VALUES (?,?,?, 'es', ?, ?, ?, ?, ?)`,
			code, area, nombre, cuerpo, string(variables), boolAInt(activa), s.sessionUserID(r), now())
		if err != nil {
			if aiEsClaveDuplicada(err) {
				jsonError(w, "Ya existe una plantilla con el código «"+code+"». Use otro código.", http.StatusBadRequest)
				return
			}
			jsonError(w, "No se pudo guardar la plantilla: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// El código NO se cambia al editar: los canales lo referencian y
		// renombrarlo dejaría un canal apuntando a la nada sin avisar.
		res, err := db.Exec(`UPDATE msg_template SET area=?, name=?, body=?, variables_json=?,
			is_active=?, updated_at=? WHERE id=?`,
			area, nombre, cuerpo, string(variables), boolAInt(activa), now(), id)
		if err != nil {
			jsonError(w, "No se pudo guardar la plantilla: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "La plantilla no existe.", http.StatusNotFound)
			return
		}
	}
	s.audit(r, "guardar", "mensajeria", "msg_template", code, nil,
		map[string]interface{}{"area": area, "name": nombre, "activa": activa})
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Plantilla guardada."})
}

func (s *Server) mensajeriaPlantillaDelete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	db := s.dbFor(r)
	id, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	if id <= 0 {
		jsonError(w, "Plantilla inválida.", http.StatusBadRequest)
		return
	}
	var code string
	_ = db.QueryRow(`SELECT code FROM msg_template WHERE id=?`, id).Scan(&code)
	if _, err := db.Exec(`DELETE FROM msg_template WHERE id=?`, id); err != nil {
		jsonError(w, "No se pudo borrar: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.audit(r, "delete", "mensajeria", "msg_template", code, nil, nil)
	writeJSON(w, map[string]interface{}{"status": 1,
		"msg": "Plantilla borrada. Si un canal la usaba, sus avisos quedarán en error con el motivo hasta que se elija otra."})
}

// mensajeriaPlantillaPreview renderiza con datos de ejemplo. La vista previa no
// sale del ERP: no manda nada a nadie.
func (s *Server) mensajeriaPlantillaPreview(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	cuerpo := strings.TrimSpace(r.FormValue("body"))
	ejemplo := map[string]string{
		"cliente":    "Cliente de ejemplo",
		"monto":      "1.234,56",
		"fecha":      time.Now().Format("02/01/2006"),
		"empresa":    primeroNoVacio(s.finEmpresa(s.dbFor(r)), "Su empresa"),
		"referencia": "F-0001",
		"enlace":     "https://erp.ejemplo/estado-cuenta",
	}
	for _, v := range mensajeria.VariablesDeAviso {
		if propio := strings.TrimSpace(r.FormValue(v)); propio != "" {
			ejemplo[v] = propio
		}
	}
	texto, err := mensajeria.Render(cuerpo, ejemplo)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": 0, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "texto": texto})
}
