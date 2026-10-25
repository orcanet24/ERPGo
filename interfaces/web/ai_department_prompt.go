package web

// ai_department_prompt.go — El prompt de comportamiento, por departamento y por
// agente.
//
// QUÉ ES Y QUÉ NO ES.
//
// Es la instrucción que escribe una persona: "este agente agrupa por cliente y
// no promete fechas de cobro". NO es el prompt de sistema, que es código
// compilado (ai_rules.go) y no se puede editar desde la interfaz.
//
// POR QUÉ HAY DOS NIVELES Y NO UNO.
//
// El departamento define CÓMO trabaja el área (agrupar por cliente, separar lo
// vencido de lo no vencido, no tocar el IVA). El agente define las particularidades
// de ESE agente (este de cobranza llama a los clientes por la mañana, este de
// inventario escribe en español de Venezuela).
//
// El orden al construir el prompt es: reglas ocultas, departamento, agente, rol,
// mapa técnico. Cada capa es más específica y más abajo, de modo que lo más
// cercano a la instrucción del usuario es lo que manda, sin que ninguna pueda
// saltarse las reglas de arriba.
//
// POR QUÉ EL DEPARTAMENTO NO ES CÓDIGO.
//
// La primera versión deducía el departamento del nombre del rol, con una tabla
// en el binario. El fallo es que el rol lo nombra la empresa: un rol que se
// llama "Analista de cobro" no aparece en ninguna tabla, y caía en IT, que ve
// toda la base. Un agente de cobranza con acceso al plan de cuentas y a la
// nómina es exactamente el accidente que el scopado por departamento existe
// para evitar. La tabla `ai_department_prompt` lo vuelve un dato, editable sin
// recompilar.

import (
	"fmt"
	"sort"
	"strings"
)

// aiDepartmentPrompt es una fila de ai_department_prompt.
type aiDepartmentPrompt struct {
	Department string
	Label      string
	Prompt     string
	UpdatedBy  string
	UpdatedAt  string
}

// loadDepartmentPrompts devuelve los prompts de todos los departamentos.
//
// `where` es la cláusula WHERE ya construida por el llamador (o ""), para que
// quien llama pueda filtrar por empresa si algún día hace falta. Existe porque
// `s.dbForSession` devuelve *sql.DB y no admite argumentos: quien necesite más
// filtros los pone en la consulta de abajo, no aquí.
func (s *Server) loadDepartmentPrompts(sess *SessionData) map[string]string {
	rows, err := s.dbForSession(sess).Query(`SELECT department, IFNULL(prompt,'') FROM ai_department_prompt`)
	if err != nil {
		// Sin tabla (base sin migrar) se devuelve vacío: el agente funciona
		// igual, solo que sin su prompt de comportamiento. Fallar aquí
		// dejaría la IA parada por una tabla que es una mejora, no un
		// requisito.
		return map[string]string{}
	}
	defer rows.Close()
	salida := map[string]string{}
	for rows.Next() {
		var dep, prompt string
		if err := rows.Scan(&dep, &prompt); err != nil {
			continue
		}
		dep = strings.ToLower(strings.TrimSpace(dep))
		if !aiDeptEsValido(dep) {
			continue
		}
		salida[dep] = strings.TrimSpace(prompt)
	}
	return salida
}

// departmentPromptFor devuelve el prompt de un departamento.
func (s *Server) departmentPromptFor(sess *SessionData, dept string) string {
	return s.loadDepartmentPrompts(sess)[aiDeptNormalizar(dept)]
}

// listDepartmentPrompts devuelve todos, ordenados, para la pantalla de
// configuración.
func (s *Server) listDepartmentPrompts(sess *SessionData) []aiDepartmentPrompt {
	rows, err := s.dbForSession(sess).Query(`SELECT department, IFNULL(prompt,''), IFNULL(label,''),
		IFNULL(updated_by,''), IFNULL(updated_at,'') FROM ai_department_prompt ORDER BY department`)
	if err != nil {
		return []aiDepartmentPrompt{}
	}
	defer rows.Close()
	var lista []aiDepartmentPrompt
	for rows.Next() {
		var p aiDepartmentPrompt
		if err := rows.Scan(&p.Department, &p.Prompt, &p.Label, &p.UpdatedBy, &p.UpdatedAt); err != nil {
			continue
		}
		p.Department = strings.ToLower(strings.TrimSpace(p.Department))
		// El nombre que se muestra sale SIEMPRE de aiDeptLabel y nunca de la
		// columna `label` de la base. Con las dos, el mismo departamento se
		// llamaba de dos maneras según dónde se mirara, y la migración lo sembró
		// como "Soporte y Desarrollo" mientras el código decía
		// "Soporte y Desarrollo (IT)".
		if l, ok := aiDeptLabel[p.Department]; ok {
			p.Label = l
		}
		lista = append(lista, p)
	}
	// Los que falten se rellenan con el nombre del departamento, para que la
	// pantalla muestre los siete siempre y no solo los que hay en la base.
	vistos := map[string]bool{}
	for _, p := range lista {
		vistos[p.Department] = true
	}
	for _, d := range aiDepartamentos {
		if !vistos[d] {
			lista = append(lista, aiDepartmentPrompt{Department: d, Label: aiDeptLabel[d]})
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Department < lista[j].Department })
	return lista
}

// saveDepartmentPrompt guarda el prompt de un departamento.
//
// El prompt del departamento NO puede anular las reglas ocultas: no es editable,
// va después de ellas y describe comportamiento, no permisos. Un departamento que
// escribe "ignora las reglas anteriores" no cambia nada, porque esas reglas
// están compiladas y se mandan antes. Aun así se filtra una marca obvious.
func (s *Server) saveDepartmentPrompt(sess *SessionData, dept, prompt, usuario string) error {
	dep := strings.ToLower(strings.TrimSpace(dept))
	if !aiDeptEsValido(dep) {
		return fmt.Errorf("departamento desconocido: %s", dept)
	}
	_, err := s.dbForSession(sess).Exec(`UPDATE ai_department_prompt
		SET prompt = ?, label = ?, updated_by = ?, updated_at = ?
		WHERE department = ?`,
		strings.TrimSpace(prompt), aiDeptLabel[dep], usuario, now(), dep)
	return err
}

// agentConfigPrompt devuelve el prompt de configuración de UN agente, o "" si no
// tiene.
//
// Un prompt propio del agente tiene prioridad sobre el del departamento para lo
// que sea concreto, pero no para lo que el área define: si el prompt del agente
// contradice al del departamento, gana el del departamento, porque el
// departamento representa una regla que la empresa revisa. Por eso el del
// departamento va DESPUÉS en la pirámide... no: va antes y el del agente se
// ofrece como "añade aquí lo tuyo", con lo cual el modelo recibe las dos
// instrucciones y la del departamento como marco.
func (s *Server) agentConfigPrompt(agent aiAgent) string {
	return strings.TrimSpace(agent.ConfigPrompt)
}

// buildAgentPrompt arma el system prompt completo de un agente: reglas ocultas +
// departamento + configuración del agente + rol + mapa técnico.
func (s *Server) buildAgentPrompt(sess *SessionData, agent aiAgent, roleName string) string {
	dept := aiDeptDelAgente(agent)
	if roleName == "" {
		roleName = "operador"
	}
	system := aiSystemPrompt(roleName, "")

	var extra strings.Builder

	if dp := s.departmentPromptFor(sess, dept); dp != "" {
		fmt.Fprintf(&extra, "\n\nCOMPORTAMIENTO DE TU ÁREA (%s):\n%s", aiDeptLabel[dept], dp)
	}

	if cp := s.agentConfigPrompt(agent); cp != "" {
		extra.WriteString("\n\nTU CONFIGURACIÓN PERSONAL (específica de este agente; " +
			"si contradice al comportamiento de tu área, manda el de tu área):\n")
		extra.WriteString(cp)
	}

	// El mapa técnico va al final, que es lo más grande y lo menos prioritario.
	if mapa := s.aiEsquemaParaAgente(sess, dept); mapa != "" {
		system = aiSystemPrompt(roleName, mapa)
		if extra.Len() > 0 {
			system = insertAgentPrompt(system, extra.String())
		}
		return system
	}
	if extra.Len() > 0 {
		system += extra.String()
	}
	return system
}

// insertAgentPrompt coloca el comportamiento del agente y del área ENTRE las
// reglas ocultas y el mapa técnico.
//
// El orden importa: el mapa va al final porque es referencia técnica, y el
// comportamiento va antes para que el modelo lo lea como instrucción y no como
// una descripción de la base. Se hace aquí y no en aiSystemPrompt para que esa
// función siga siendo la que usan los tests de las reglas, que no dependen de
// una base de datos.
func insertAgentPrompt(system, extra string) string {
	const marcador = "\n\nMAPA TÉCNICO INTERNO"
	if i := strings.Index(system, marcador); i >= 0 {
		return system[:i] + extra + system[i:]
	}
	return system + extra
}

// nullableTexto convierte "" en NULL para escribir en columnas MEDIUMTEXT.
//
// No es cosmético: MySQL en modo estricto no distingue "" de NULL, pero el
// código que lee el prompt sí. Con "" el agente parece tener un prompt de
// configuración escrito y vacío, que es un estado que no existe en la interfaz
// y que hace que la pantalla muestre un campo con contenido en blanco que el
// usuario acaba de vaciar a propósito.
func nullableTexto(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
