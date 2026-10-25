package web

import (
	"database/sql"
	"net/http"
	"strings"
)

// MenuItem es un nodo del menú lateral ya filtrado por permisos y listo para
// pintar. La estructura viene de la tabla `menu` (una fila por entrada); el
// layout solo la recorre.
type MenuItem struct {
	Label    string
	URL      string
	Icon     string
	Active   bool
	Children []MenuItem
}

// menuNode es la fila cruda antes de filtrar.
type menuNode struct {
	id       int
	parentID sql.NullInt64
	label    string
	url      string
	icon     string
	perm     string
	active   string
	children []*menuNode
}

// menuTree carga el menú de la base y lo poda a lo que este request puede ver.
//
// El filtro es el mismo de siempre —el mapa de permisos por sub_module.directory—
// pero ahora la ESTRUCTURA vive en la base (tabla `menu`) y no en layout.html.
// Un grupo (url vacío) solo aparece si le queda algún hijo visible; un enlace
// con `perm` vacío es público. `perm` admite una lista OR separada por "|".
func (s *Server) menuTree(r *http.Request, perm map[string]bool, currentURL string) []MenuItem {
	roots, err := s.loadMenu(r)
	if err != nil || len(roots) == 0 {
		return nil
	}
	out := make([]MenuItem, 0, len(roots))
	for _, n := range roots {
		if it, ok := buildMenuItem(n, perm, currentURL); ok {
			out = append(out, it)
		}
	}
	return out
}

// loadMenu lee la tabla `menu` y arma el árbol en memoria. Devuelve solo las
// raíces; los hijos quedan colgados de su padre.
func (s *Server) loadMenu(r *http.Request) ([]*menuNode, error) {
	rows, err := s.authDB(r).Query(`
		SELECT id, parent_id, label, url, icon, perm, active_prefix
		FROM menu
		WHERE status = 1
		ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []*menuNode
	byID := map[int]*menuNode{}
	for rows.Next() {
		n := &menuNode{}
		var url, icon, perm, active sql.NullString
		if err := rows.Scan(&n.id, &n.parentID, &n.label, &url, &icon, &perm, &active); err != nil {
			continue
		}
		n.url, n.icon, n.perm, n.active = url.String, icon.String, perm.String, active.String
		all = append(all, n)
		byID[n.id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var roots []*menuNode
	for _, n := range all {
		if n.parentID.Valid {
			if p := byID[int(n.parentID.Int64)]; p != nil {
				p.children = append(p.children, n)
			}
			// Padre ausente (fila borrada o deshabilitada): se ignora la entrada
			// en vez de mostrarla en el primer nivel, que es lo que el usuario
			// no espera.
			continue
		}
		roots = append(roots, n)
	}
	return roots, nil
}

// buildMenuItem convierte un nodo en su forma de plantilla, aplicando permisos.
// El segundo valor es false cuando el nodo debe ocultarse.
func buildMenuItem(n *menuNode, perm map[string]bool, currentURL string) (MenuItem, bool) {
	isLeaf := n.url != ""

	// EL PERMISO DEL PROPIO NODO MANDA, sea enlace o grupo.
	//
	// Un grupo lo declara cuando sus hijos son públicos, y hay dos formas de que
	// lo sean: enlaces sin `perm` (Bancos: sus cuatro, y Ajuste Inventario) o un
	// hijo suelto público dentro de un grupo con hijos con permiso (Servicios:
	// Factura de Servicio). Sin esta comprobación el grupo aparecía para todo el
	// mundo por culpa del hijo público —y con él, su nombre y su icono—, así que
	// el filtro decía una cosa y el menú mostraba otra. La comprobación estaba
	// solo en la rama de los enlaces, que es exactamente el caso en el que el
	// hijo público no puede delatar al grupo.
	if n.perm != "" && !anyPerm(perm, n.perm) {
		return MenuItem{}, false
	}

	if isLeaf {
		return MenuItem{
			Label:  n.label,
			URL:    n.url,
			Icon:   n.icon,
			Active: menuLinkActive(n, currentURL),
		}, true
	}

	kids := make([]MenuItem, 0, len(n.children))
	for _, c := range n.children {
		if it, ok := buildMenuItem(c, perm, currentURL); ok {
			kids = append(kids, it)
		}
	}
	if len(kids) == 0 {
		return MenuItem{}, false
	}
	active := false
	for _, k := range kids {
		if k.Active {
			active = true
			break
		}
	}
	return MenuItem{Label: n.label, Icon: n.icon, Active: active, Children: kids}, true
}

// anyPerm evalúa una lista OR separada por "|": basta con uno.
func anyPerm(perm map[string]bool, expr string) bool {
	for _, key := range strings.Split(expr, "|") {
		if key != "" && perm[key] {
			return true
		}
	}
	return false
}

// menuLinkActive decide si un enlace es el actual. Coincidencia exacta salvo
// que la fila declare `active_prefix`, que se usa para marcar también las
// subrutas (p. ej. /customer/ledger ilumina en /customer/ledger/5).
func menuLinkActive(n *menuNode, currentURL string) bool {
	if currentURL == n.url {
		return true
	}
	return n.active != "" && strings.HasPrefix(currentURL, n.active)
}
