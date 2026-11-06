package web

import (
	"database/sql"
	_ "embed"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Plan de Cuentas CONASEV (Comisión Nacional de Contabilidad, Venezuela).
// Fuente: doc/plantillas/SISTEMA CONTABLE EXCEL.xls, hoja "PC" — 1.855 cuentas
// con jerarquía Elemento (2) > Grupo (3) > Sub-grupo (4) > Cuenta (5) > Sub-cuenta (6).
//
//go:embed data/conasev_coa.csv
var conasevCOACSV string

// coaAccount es una cuenta del plan CONASEV normalizada para acc_coa.
type coaAccount struct {
	Code     int    // HeadCode (columna int del proyecto: no admite ceros a la izquierda)
	Name     string // HeadName
	Parent   string // PHeadCode
	ParentNm string // PHeadName
	Level    int    // HeadLevel = longitud del código
	Type     string // HeadType: A=Activo, L=Pasivo, I=Ingreso, E=Gasto
	Orden    string // código con ceros a la izquierda, para ordenar y agrupar
}

// parseCONASEVOrder devuelve SOLO las cuentas de orden del SENIAT: las que
// empiezan por 0 (06, 07, 0124...). Son un tipo de cuenta distinto —las cuentas
// de orden no forman parte del patrimonio— y no se cargan en acc_coa porque
// HeadCode es numérico y perdería el cero a la izquierda.
//
// Se exponen aparte para poder REPORTARLAS. Descartarlas en silencio parece que
// el catálogo se cargó completo cuando no es así, y eso es peor que la
// limitación en sí.
func parseCONASEVOrder() []coaAccount {
	rd := csv.NewReader(strings.NewReader(conasevCOACSV))
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil
	}
	var out []coaAccount
	vistos := map[string]bool{}
	for _, r := range rows[1:] {
		if len(r) < 5 {
			continue
		}
		cod := strings.TrimSpace(r[1])
		desc := strings.TrimSpace(r[2])
		if cod == "" || desc == "" || !strings.HasPrefix(cod, "0") {
			continue
		}
		if vistos[cod] {
			continue
		}
		vistos[cod] = true
		n, err := strconv.Atoi(cod)
		if err != nil {
			continue
		}
		nivel, _ := strconv.Atoi(strings.TrimSpace(r[0]))
		out = append(out, coaAccount{
			Code:  n,
			Name:  desc,
			Level: nivel,
			Orden: cod,
		})
	}
	return out
}

// parseCONASEV convierte el CSV embebido en cuentas normalizadas.
// Las cuentas de orden (código que empieza en 0) se descartan: la columna
// HeadCode es numérica y perdería el cero a la izquierda (p.ej. 07400 -> 7400).
// Ver parseCONASEVOrder, que las conserva solo para poder reportarlas.
func parseCONASEV() []coaAccount {
	rd := csv.NewReader(strings.NewReader(conasevCOACSV))
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil
	}
	nombre := make(map[string]string) // código -> nombre, para resolver PHeadName
	var cuentas []coaAccount
	for _, r := range rows[1:] {
		if len(r) < 5 {
			continue
		}
		cod := strings.TrimSpace(r[1])
		desc := strings.TrimSpace(r[2])
		if cod == "" || desc == "" {
			continue
		}
		nombre[cod] = desc
	}
	vistos := map[string]bool{}
	for _, r := range rows[1:] {
		if len(r) < 5 {
			continue
		}
		cod := strings.TrimSpace(r[1])
		desc := strings.TrimSpace(r[2])
		tipo := strings.TrimSpace(r[3])
		if cod == "" || desc == "" || strings.HasPrefix(cod, "0") {
			continue // cuentas de orden (0x): no caben en HeadCode numérico
		}
		if vistos[cod] {
			continue // el catálogo del SENIAT repite un par de códigos: gana el primero
		}
		vistos[cod] = true
		n, err := strconv.Atoi(cod)
		if err != nil {
			continue
		}
		nivel, _ := strconv.Atoi(strings.TrimSpace(r[0]))
		if nivel == 0 {
			nivel = len(cod)
		}
		cuentas = append(cuentas, coaAccount{
			Code:  n,
			Name:  desc,
			Level: nivel,
			Type:  conasevHeadType(tipo, cod),
			Orden: cod,
		})
	}
	// El catálogo del SENIAT no es un árbol estricto: hay cuentas hijas cuyo
	// nivel intermedio no viene en la plantilla (94112 cuelga de 941 porque 9411
	// no existe). Para que el árbol del sistema sea íntegro, cada cuenta se
	// engancha al ancestro existente más cercano.
	for i := range cuentas {
		padre := ""
		for l := len(cuentas[i].Orden) - 1; l >= 2; l-- {
			if n, ok := nombre[cuentas[i].Orden[:l]]; ok && n != "" {
				padre = cuentas[i].Orden[:l]
				break
			}
		}
		cuentas[i].Parent = padre
		cuentas[i].ParentNm = nombre[padre]
	}
	return cuentas
}

// conasevHeadType traduce la clase de la plantilla (A/P/G/I) a la convención del
// sistema: A=Activo, L=Pasivo, I=Ingreso, E=Gasto.
func conasevHeadType(tipo, cod string) string {
	switch strings.ToUpper(tipo) {
	case "A":
		return "A"
	case "P":
		return "L"
	case "I":
		return "I"
	case "G":
		return "E"
	}
	switch cod[:2] {
	case "10", "11", "12", "13", "14", "16", "17", "18", "19", "20", "21", "22", "23",
		"24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35", "36",
		"37", "38", "39":
		return "A"
	case "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "50", "51", "52",
		"56", "57", "58", "59":
		return "L"
	case "60", "61", "62", "63", "64", "65", "66", "67", "68", "69", "94", "95", "97":
		return "E"
	default: // 70-89: ingresos y determination del resultado
		return "I"
	}
}

// SeedCOAResult resume lo que hizo el sembrado del plan CONASEV.
type SeedCOAResult struct {
	Inserted   int      // cuentas agregadas
	Skipped    int      // cuentas que ya existían
	Collisions []string // códigos que CONASEV y el plan anterior definen distinto

	// Después de cargar un plan hay que volver a atar los papeles contables del
	// sistema (inventario, gastos, IVA) a las cuentas del plan nuevo. Si el
	// catálogo no trae un equivalente, el papel queda pendiente y se informa.
	CuentasAsignadas  []string
	CuentasPendientes []string
}

// SeedCONASEV carga el plan de cuentas CONASEV en acc_coa.
// Es idempotente: sólo inserta las cuentas cuyo HeadCode no existe todavía, de
// modo que se puede ejecutar en cada arranque sin duplicar ni pisar las cuentas
// que el negocio ya tiene configuradas (clientes, proveedores, bancos).
func SeedCONASEV(db *sql.DB) (res SeedCOAResult, err error) {
	cuentas := parseCONASEV()
	if len(cuentas) == 0 {
		return res, fmt.Errorf("plan CONASEV vacio")
	}

	existentes := map[int]string{} // HeadCode -> nombre actual
	rows, err := db.Query(`SELECT HeadCode, HeadName FROM acc_coa`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c int
		var n string
		if err := rows.Scan(&c, &n); err == nil {
			existentes[c] = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	ahora := time.Now().Format("2006-01-02 15:04:05")
	// IsTransaction/IsGL: en CONASEV seposted en cuentas (5) y sub-cuentas (6);
	// los niveles superiores son grupos de mayor.
	ins := `INSERT INTO acc_coa
		(HeadCode, HeadName, PHeadName, PHeadCode, HeadLevel, IsActive, IsTransaction,
		 IsGL, isCashNature, isBankNature, HeadType, IsBudget, IsDepreciation,
		 DepreciationRate, CreateBy, CreateDate, UpdateBy, UpdateDate)
		VALUES (?,?,?,?,?,1,?,?,0,0,?,0,0,0,'conasev',?, 'conasev',?)`
	tx, err := db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(ins)
	if err != nil {
		return res, err
	}
	defer stmt.Close()
	for _, c := range cuentas {
		if actual, ok := existentes[c.Code]; ok {
			res.Skipped++
			if !sameAccountName(actual, c.Name) {
				res.Collisions = append(res.Collisions,
					fmt.Sprintf("%s: %q (del sistema) vs %q (CONASEV)", c.Orden, actual, c.Name))
			}
			continue
		}
		esGL := 1
		if c.Level >= 5 {
			esGL = 0
		}
		if _, err := stmt.Exec(c.Code, c.Name, c.ParentNm, c.Parent, c.Level,
			esGL, esGL, c.Type, ahora, ahora); err != nil {
			return res, fmt.Errorf("cuenta %d (%s): %w", c.Code, c.Name, err)
		}
		res.Inserted++
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// sameAccountName compara nombres ignorando mayúsculas, acentos y signos, para
// no reportar como conflicto dos formas de escribir la misma cuenta.
func sameAccountName(a, b string) bool {
	return normAccountName(a) == normAccountName(b)
}

func normAccountName(s string) string {
	var out []rune
	for _, r := range strings.ToUpper(s) {
		switch r {
		case 'Á':
			out = append(out, 'A')
		case 'É':
			out = append(out, 'E')
		case 'Í':
			out = append(out, 'I')
		case 'Ó':
			out = append(out, 'O')
		case 'Ú', 'Ü':
			out = append(out, 'U')
		case 'Ñ':
			out = append(out, 'N')
		case '(', ')', '.', ',', '-', '–', '/', ' ':
			// los signos no cuentan para comparar
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// ============================================================================
// Handlers HTTP y registro de rutas CONASEV.
// ============================================================================

// conasevSeed ejecuta SeedCONASEV(db) y devuelve el resultado como JSON.
func (s *Server) conasevSeed(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "coa", "update") {
		jsonError(w, "No tiene permiso para modificar el plan de cuentas", http.StatusForbidden)
		return
	}
	res, err := SeedCONASEV(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"status":     1,
		"msg":        "Catálogo CONASEV sembrado",
		"inserted":   res.Inserted,
		"skipped":    res.Skipped,
		"collisions": res.Collisions,
	})
}

// conasevData devuelve las cuentas CONASEV cargadas en acc_coa.
func (s *Server) conasevData(w http.ResponseWriter, r *http.Request) {
	rows, err := s.dbFor(r).Query(`SELECT HeadCode, HeadName, HeadLevel, HeadType
		FROM acc_coa WHERE IFNULL(CreateBy,'') = 'conasev' ORDER BY HeadCode`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var cuentas []map[string]interface{}
	for rows.Next() {
		var code, level int
		var name, tipo string
		if rows.Scan(&code, &name, &level, &tipo) != nil {
			continue
		}
		cuentas = append(cuentas, map[string]interface{}{
			"code":  code,
			"name":  name,
			"level": level,
			"type":  tipo,
		})
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"accounts": cuentas,
		"count":    len(cuentas),
	})
}

// conasevList devuelve las cuentas de orden (0x) del catálogo CONASEV,
// que no caben en HeadCode numérico pero se reportan para referencia.
func (s *Server) conasevList(w http.ResponseWriter, r *http.Request) {
	ordenes := parseCONASEVOrder()
	var cuentas []map[string]interface{}
	for _, c := range ordenes {
		cuentas = append(cuentas, map[string]interface{}{
			"code":  c.Code,
			"name":  c.Name,
			"level": c.Level,
			"orden": c.Orden,
		})
	}
	writeJSON(w, map[string]interface{}{
		"status":   1,
		"accounts": cuentas,
		"count":    len(cuentas),
	})
}

// conasevStatus muestra cuántas cuentas CONASEV hay vs el total.
func (s *Server) conasevStatus(w http.ResponseWriter, r *http.Request) {
	var total, conasev int
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_coa`).Scan(&total)
	_ = s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE IFNULL(CreateBy,'') = 'conasev'`).Scan(&conasev)
	propias := total - conasev

	st := InspectCOACatalog(s.dbFor(r))
	out := map[string]interface{}{
		"status":        1,
		"total":         total,
		"conasev":       conasev,
		"propias":       propias,
		"catalog":       st.Catalog,
		"source":        st.Source,
		"collisions":    st.CollisionCount,
		"order_missing": len(st.OrderAccountsMissing),
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=conasev_status.csv")
		fmt.Fprintln(w, "Método,Valor")
		fmt.Fprintf(w, "Catálogo,%s\n", st.Catalog)
		fmt.Fprintf(w, "Total cuentas,%d\n", total)
		fmt.Fprintf(w, "Cuentas CONASEV,%d\n", conasev)
		fmt.Fprintf(w, "Cuentas propias,%d\n", propias)
		fmt.Fprintf(w, "Colisiones,%d\n", st.CollisionCount)
		fmt.Fprintf(w, "Cuentas de orden faltantes,%d\n", len(st.OrderAccountsMissing))
		return
	}
	writeJSON(w, out)
}

// registerConasevRoutes registra las rutas HTTP del módulo CONASEV.
func (s *Server) registerConasevRoutes() {
	s.mux.HandleFunc("GET /conasev/list", s.conasevList)
	s.mux.HandleFunc("GET /conasev/data", s.conasevData)
	s.mux.HandleFunc("POST /conasev/seed", s.conasevSeed)
	s.mux.HandleFunc("GET /conasev/status", s.conasevStatus)
}
