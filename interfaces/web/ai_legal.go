package web

// ai_legal.go — Resúmenes legales cargados para la IA.
//
// Este archivo responde al punto que hace que la IA sea un empleado competente
// y no uno que improvisa. El problema de un LLM en un ERP venezolano no es que
// no sepa la teoría: es que su/training se quedó en una fecha y contesta con
// cifras que ya cambiaron. El IVA era 16%, hoy es 18. La alícuota de retención
// del art. 11 se movió. Una IA que "recuerda" una cifra desactualizada no
// aporta: aporta ERROR con mucha seguridad.
//
// Por eso las normas viven aquí, con vigencia y cita legal, y la herramienta
// legal_summary las consulta por fecha. La IA no recuerda: lee lo que el sistema
// tiene. Cuando el contador actualiza la tabla, la IA queda actualizada sin
// tocar una línea de prompt.
//
// Regla estructural, igual que en el resto del módulo: TODO dato normativo es
// tabla versionada con vigencia y fuente. Aquí no hay ni una constante legal en
// código, y ese es el punto.

import (
	"database/sql"
	"fmt"
	"strings"

	"erpgo/infrastructure/persistence"
)

// legalSchema define la base normativa.
//
// El contenido no se siembra con el resto del sistema a propósito: son
// afirmaciones que un contador tiene que validar antes de que la IA las use, y
// una versión sembrada automáticamente se acabaría creyendo correcta por
// costumbre. La tabla existe desde el arranque; el contenido lo carga quien
// tiene la autoridad para hacerlo.
//
// Se escribe en dialecto SQLite porque este DDL se ejecuta directamente desde
// Go (no pasa por el conversor de migraciones). Usar COMMENT o backticks aquí
// produciría un error de sintaxis en tiempo de arranque, que es la forma más
// cara de descubrir un error de dialecto.
var legalSchema = []string{
	// topic: iva | islr | retencion_iva | retencion_islr | nomina |
	//        contabilidad | libros | providencia
	// citation: ley, providencia o gaceta de la que sale la norma.
	// MySQL NO admite DEFAULT en una columna TEXT: da "BLOB, TEXT, GEOMETRY or
	// JSON column can't have a default value", y el error salta al crear la
	// tabla, no al insertar. Por eso estas columnas son VARCHAR: el contenido
	// es un topico, un título o una cita, nunca un documento. La única que se
	// queda en TEXT (el resumen) es la única que no lleva DEFAULT.
	//
	// `CREATE INDEX IF NOT EXISTS` tampoco existe en MySQL (solo acepta IF NOT
	// EXISTS el CREATE TABLE y el CREATE DATABASE), así que el índice va sin
	// eso. Si ya existe, el error "duplicate key name" se trata como "ya
	// estaba" en el ejecutor de migraciones, que es lo correcto.
	`CREATE TABLE IF NOT EXISTS ai_legal_rule (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		topic VARCHAR(60) NOT NULL,
		title VARCHAR(255) NOT NULL,
		citation VARCHAR(191) NOT NULL DEFAULT '',
		summary TEXT NOT NULL,
		effective_from VARCHAR(32) NOT NULL,
		effective_to VARCHAR(32) NOT NULL DEFAULT '',
		verified INTEGER NOT NULL DEFAULT 0,
		status INTEGER NOT NULL DEFAULT 1,
		created_at VARCHAR(32) NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX idx_ai_legal_topic ON ai_legal_rule(topic, effective_from)`,
}

// LegalRule es una norma cargada, con su vigencia.
type LegalRule struct {
	ID            int64
	Topic         string
	Title         string
	Citation      string
	Summary       string
	EffectiveFrom string
	EffectiveTo   string
	Verified      bool
}

// topicLabel traduce la clave técnica al nombre que entiende una persona.
var topicLabel = map[string]string{
	"iva":            "Impuesto al Valor Agregado",
	"islr":           "Impuesto sobre la Renta",
	"retencion_iva":  "Retención del IVA (art. 11)",
	"retencion_islr": "Retención del ISLR",
	"nomina":         "Régimen laboral y de nómina",
	"contabilidad":   "Normas contables",
	"libros":         "Libros y documentos obligatorios",
	"providencia":    "Providencias y otros actos",
}

// LegalTopics lista los temas disponibles y sus etiquetas.
func LegalTopics() []map[string]string {
	// El orden es fijo para que la herramienta devuelva siempre lo mismo: una
	// lista que cambia de orden entre llamadas hace que el modelo la lea como si
	// fuera información nueva.
	claves := []string{"iva", "islr", "retencion_iva", "retencion_islr", "nomina", "contabilidad", "libros"}
	out := make([]map[string]string, 0, len(claves))
	for _, c := range claves {
		out = append(out, map[string]string{"topic": c, "label": topicLabel[c]})
	}
	return out
}

// EnsureLegalSchema crea la base normativa si falta.
//
// Va en la BD de cada EMPRESA, no en la de control: la normativa de una
// empresa es suya. Dos empresas del mismo grupo pueden estar bajo regímenes
// distintos, y compartir el catálogo entre ellas sería untrue en un sentido
// importante.
func EnsureLegalSchema(db *sql.DB) error {
	if err := persistence.AplicarEsquema(db, legalSchema); err != nil {
		return fmt.Errorf("esquema legal: %w", err)
	}
	return nil
}

// LoadLegalRules devuelve las normas vigentes en una fecha.
//
// La vigencia se resuelve IGUAL que en las tasas de nómina: gana la versión más
// reciente que ya haya entrado en vigor. Es el mismo criterio porque es el mismo
// problema, y tener dos reglas distintas para "qué norma aplicaba hoy" acabaría
// con el sistema diciendo dos cosas.
func (s *Server) LoadLegalRules(sess *SessionData, topic, asOf string) ([]LegalRule, error) {
	query := `
		SELECT id, topic, title, IFNULL(citation,''), IFNULL(summary,''),
		       effective_from, IFNULL(effective_to,''), verified
		FROM ai_legal_rule
		WHERE status = 1 AND effective_from <= ?
		  AND (effective_to IS NULL OR effective_to = '' OR effective_to >= ?)`
	args := []interface{}{asOf, asOf}

	if topic != "" {
		query += ` AND topic = ?`
		args = append(args, topic)
	}
	query += ` ORDER BY topic, effective_from DESC`

	rows, err := s.dbForSession(sess).Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Si hay varias versiones del mismo título, gana la más reciente: repetir la
	// norma dos veces con fechas distintas solo confunde al modelo.
	//
	// La consulta viene ordenada por effective_from DESC, así que la PRIMERA
	// versión de cada título es la vigente. Sobrescribir sin mirar devolvería la
	// más antigua, que es justo la norma derogada.
	porTitulo := map[string]LegalRule{}
	var orden []string
	for rows.Next() {
		var r LegalRule
		var verified int
		if err := rows.Scan(&r.ID, &r.Topic, &r.Title, &r.Citation, &r.Summary,
			&r.EffectiveFrom, &r.EffectiveTo, &verified); err != nil {
			continue
		}
		r.Verified = verified == 1
		clave := r.Topic + "|" + r.Title
		if _, yaExiste := porTitulo[clave]; yaExiste {
			continue
		}
		porTitulo[clave] = r
		orden = append(orden, clave)
	}

	out := make([]LegalRule, 0, len(orden))
	for _, k := range orden {
		out = append(out, porTitulo[k])
	}
	return out, rows.Err()
}

// LegalSummaryFor arma el texto que la IA recibe como contexto legal.
//
// El formato es deliberadamente legible por un modelo y, sobre todo, explícito
// sobre lo que NO se sabe: si no hay norma cargada para un tema, se dice que no
// la hay. Dejar que el modelo rellene ese hueco con lo que "recuerda" es
// exactamente el fallo que este módulo existe para evitar.
func (s *Server) LegalSummaryFor(sess *SessionData, topic, asOf string) (string, error) {
	rules, err := s.LoadLegalRules(sess, topic, asOf)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("NORMATIVA VIGENTE AL " + asOf + "\n")
	b.WriteString("(cargada por el administrador de la empresa; si un tema no aparece, significa que NO hay norma cargada y debes decirlo en vez de suponerla)\n")
	b.WriteString(strings.Repeat("=", 60) + "\n")

	if len(rules) == 0 {
		t := topic
		if t == "" {
			t = "ninguno"
		}
		b.WriteString("\nNo hay normas cargadas para el tema '" + t + "'.\n")
		b.WriteString("Dilo así en la respuesta: no dispones de la norma vigente y no debes inventar una cifra.\n")
		return b.String(), nil
	}

	// Se agrupa por tema para que el modelo lea un bloque coherente.
	var temaActual string
	for _, r := range rules {
		if r.Topic != temaActual {
			temaActual = r.Topic
			b.WriteString("\n## " + legalTopicName(temaActual) + "\n")
		}
		b.WriteString("\n- " + r.Title)
		if r.Citation != "" {
			b.WriteString(" (" + r.Citation + ")")
		}
		b.WriteString("\n  Vigencia: " + r.EffectiveFrom)
		if r.EffectiveTo != "" {
			b.WriteString(" → " + r.EffectiveTo)
		} else {
			b.WriteString(" → vigente")
		}
		if !r.Verified {
			b.WriteString("\n  AVISO: esta norma NO ha sido validada por el administrador de la empresa.")
		}
		if r.Summary != "" {
			b.WriteString("\n  " + r.Summary)
		}
	}
	return b.String(), nil
}

func legalTopicName(t string) string {
	if n, ok := topicLabel[t]; ok {
		return n
	}
	return t
}

// LegalUnverifiedCount dice cuántas normas cargadas están sin validar.
//
// La IA lo recibe para poder advertir en su respuesta. Una norma sin validar no
// es falsa, pero presentarla como si lo estuviera sería exactamente el error que
// el módulo evita.
func (s *Server) LegalUnverifiedCount(sess *SessionData, asOf string) int {
	var n sql.NullInt64
	err := s.dbForSession(sess).QueryRow(`SELECT COUNT(*) FROM ai_legal_rule
		WHERE status = 1 AND verified = 0 AND effective_from <= ?
		  AND (effective_to IS NULL OR effective_to = '' OR effective_to >= ?)`, asOf, asOf).Scan(&n)
	if err != nil || !n.Valid {
		return 0
	}
	return int(n.Int64)
}

// SaveLegalRule guarda o actualiza una norma.
//
// No hay sembrado automático: una norma la carga alguien con autoridad, con su
// cita legal. Un sistema que rellena sus propias leyes desde un texto fijo acaba
// creyéndose autor, que es el peor defecto que puede tener un sistema fiscal.
// SaveLegalRule guarda o actualiza una norma.
//
// El parámetro se llama rule, no r: una norma es de una empresa, y la empresa la
// decide la sesión. Confundir ambas cosas era el error que hacía que el nombre
// dejara sitio para colar un handle equivocado.
func (s *Server) SaveLegalRule(sess *SessionData, rule *LegalRule) (int64, error) {
	rule.Title = strings.TrimSpace(rule.Title)
	if rule.Topic == "" || rule.Title == "" {
		return 0, fmt.Errorf("tema y título son obligatorios")
	}
	if !aiValidDate(rule.EffectiveFrom) {
		return 0, fmt.Errorf("la fecha de vigencia debe tener el formato AAAA-MM-DD")
	}
	if rule.EffectiveTo != "" && !aiValidDate(rule.EffectiveTo) {
		return 0, fmt.Errorf("la fecha de fin, si se indica, debe tener el formato AAAA-MM-DD")
	}
	if rule.EffectiveTo != "" && rule.EffectiveTo < rule.EffectiveFrom {
		return 0, fmt.Errorf("la fecha de fin no puede ser anterior a la de inicio")
	}

	db := s.dbForSession(sess)
	if rule.ID > 0 {
		_, err := db.Exec(`UPDATE ai_legal_rule SET
			topic=?, title=?, citation=?, summary=?, effective_from=?, effective_to=?, verified=?, status=?
			WHERE id=?`,
			rule.Topic, rule.Title, rule.Citation, rule.Summary, rule.EffectiveFrom, rule.EffectiveTo,
			boolInt(rule.Verified), 1, rule.ID)
		return rule.ID, err
	}
	res, err := db.Exec(`INSERT INTO ai_legal_rule
		(topic, title, citation, summary, effective_from, effective_to, verified, status, created_at)
		VALUES (?,?,?,?,?,?,?,1,?)`,
		rule.Topic, rule.Title, rule.Citation, rule.Summary, rule.EffectiveFrom, rule.EffectiveTo,
		boolInt(rule.Verified), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

