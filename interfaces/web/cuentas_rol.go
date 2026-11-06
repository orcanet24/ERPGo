package web

// cuentas_rol.go — El plan de cuentas del comercio, atado por PAPEL y no por código.
//
// POR QUÉ ESTO EXISTE
//
// La separación entre mercancía, consumo interno y servicios necesita cuentas:
// dónde entra el inventario de cada tipo, dónde se reconoce el gasto, dónde el
// IVA crédito fiscal. Escribir esos códigos en el código fuente funciona en la
// empresa que usa el plan estándar y falla en cualquier otra: el comercio que
// codifica a su manera (o que carga el catálogo CONASEV) termina asentando
// contra cuentas que no existen o que significan otra cosa.
//
// Por eso cada cuenta se pide por su PAPEL (inventario de suministros, gasto de
// servicios, IVA de compras...) y se resuelve contra `acc_predefine_account`,
// que es la configuración de la empresa. Esta pantalla es la que permite:
//
//   - vincular cada papel a cualquier cuenta del plan propio;
//   - volver a vincular los papeles de la separación al plan estándar cuando
//     la empresa lo usa;
//   - detectar papeles sin cuenta o con una cuenta que ya no sirve (inactiva,
//     de otro tipo) antes de que un asiento salga mal.
//
// La regla que se protege: un gasto nunca puede apuntar a una cuenta de
// ingreso, y un inventario nunca a una cuenta de resultado.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// RolCuenta describe un papel contable del sistema.
type RolCuenta struct {
	Clave      string   // columna de acc_predefine_account
	Titulo     string   // nombre para el usuario
	Ayuda      string   // para qué se usa, en una línea
	Grupo      string   // agrupación en la pantalla
	Naturaleza []string // HeadType aceptados: A activo, L pasivo, I ingreso, E gasto
	Estandar   int64    // código del plan estándar del ERP (0 = no hay sugerencia)

	// Auto: el vínculo con el plan estándar puede atar este papel por sí solo.
	// Se limita a las cuentas de la separación (inventarios, gastos, IVA crédito),
	// que son las que el sistema define y puede reconocer por nombre. Los papeles
	// generales (caja, ventas, clientes) dependen de cómo los tenga el comercio:
	// cambiarlos en silencio sería peor que pedir que los elija.
	Auto bool
	// Claves son palabras que deben aparecer en el nombre de la cuenta para
	// aceptarla en el vínculo automático. Sin esto, un código que en otro plan
	// significa otra cosa se ataría por coincidencia.
	Claves []string
}

// rolesCuenta es la lista completa y el único lugar donde se declara qué
// naturalezas admite cada papel. El orden es el de la pantalla.
func rolesCuenta() []RolCuenta {
	return []RolCuenta{
		// --- Operación diaria -------------------------------------------------
		{Clave: "cashCode", Titulo: "Caja", Grupo: "Operación",
			Ayuda: "Entradas y salidas de efectivo. Debe ser una cuenta de detalle, no el grupo «Caja».", Naturaleza: []string{"A"}, Estandar: 1020101},
		{Clave: "bankCode", Titulo: "Banco", Grupo: "Operación",
			Ayuda: "Movimientos por banco. Debe ser una cuenta de detalle con naturaleza bancaria.", Naturaleza: []string{"A"}, Estandar: 1020501},
		{Clave: "customerCode", Titulo: "Cuentas por cobrar a clientes", Grupo: "Operación",
			Ayuda: "Deuda de los clientes por ventas a crédito.", Naturaleza: []string{"A"}, Estandar: 1020801},
		{Clave: "supplierCode", Titulo: "Cuentas por pagar a proveedores", Grupo: "Operación",
			Ayuda: "Deuda con proveedores por compras a crédito.", Naturaleza: []string{"L"}, Estandar: 5020201},
		{Clave: "salesCode", Titulo: "Ingresos por ventas", Grupo: "Operación",
			Ayuda: "Contrapartida de la venta de mercancía.", Naturaleza: []string{"I"}, Estandar: 3010301},
		{Clave: "serviceCode", Titulo: "Ingresos por servicios", Grupo: "Operación",
			Ayuda: "Ingresos por servicios prestados. No se usa para gastos.", Naturaleza: []string{"I"}, Estandar: 3010401},

		// --- Inventario y costo ----------------------------------------------
		{Clave: "inventoryCode", Titulo: "Inventario de mercancía para la venta", Grupo: "Inventario y costo",
			Ayuda: "Donde entra lo que se compra para revender. Es la cuenta que antes recibía TODA compra.", Naturaleza: []string{"A"}, Estandar: 1020401,
			Auto: true, Claves: []string{"inventario", "inventory", "mercanc"}},
		{Clave: "costs_of_good_solds", Titulo: "Costo de ventas", Grupo: "Inventario y costo",
			Ayuda: "Costo de la mercancía vendida y contrapartida de los faltantes de inventario comercial.", Naturaleza: []string{"E"}, Estandar: 4010101,
			Auto: true, Claves: []string{"costo de venta", "cost of goods"}},
		{Clave: "purchaseCode", Titulo: "Compras (cuenta histórica)", Grupo: "Inventario y costo",
			Ayuda: "Cuenta que usaba el módulo anterior para el documento completo. Se conserva para no romper datos viejos.", Naturaleza: []string{"A", "E"}, Estandar: 0},

		// --- Separación servicio / consumo / venta ---------------------------
		{Clave: "supplies_inventory_code", Titulo: "Inventario de suministros y consumibles", Grupo: "Servicio, consumo y venta",
			Ayuda: "Donde entran la papelería y la limpieza. Nunca se mezcla con la mercancía de venta.", Naturaleza: []string{"A"}, Estandar: 1020403,
			Auto: true, Claves: []string{"suministro", "supplies", "consumible"}},
		{Clave: "supplies_expense_code", Titulo: "Gasto por consumo de suministros", Grupo: "Servicio, consumo y venta",
			Ayuda: "Contrapartida del consumo interno. No es costo de ventas.", Naturaleza: []string{"E"}, Estandar: 4021402,
			Auto: true, Claves: []string{"suministro", "supplies"}},
		{Clave: "service_expense_code", Titulo: "Gasto por servicios recibidos", Grupo: "Servicio, consumo y venta",
			Ayuda: "Luz, agua, internet, asesorías. Es el valor por defecto del renglón de servicio.", Naturaleza: []string{"E"}, Estandar: 4021401,
			Auto: true, Claves: []string{"servicio", "service"}},
		{Clave: "purchase_vat_code", Titulo: "IVA crédito fiscal (compras)", Grupo: "Servicio, consumo y venta",
			Ayuda: "IVA de las compras. Antes se sumaba al inventario; ahora va aquí.", Naturaleza: []string{"A"}, Estandar: 1020901,
			Auto: true, Claves: []string{"iva", "vat"}},

		// --- Impuestos y nómina ----------------------------------------------
		{Clave: "vat", Titulo: "IVA por pagar (ventas)", Grupo: "Impuestos y nómina",
			Ayuda: "IVA que se cobra al cliente. Es un pasivo, no un crédito fiscal.", Naturaleza: []string{"L", "A"}, Estandar: 0},
		{Clave: "tax", Titulo: "Impuestos y tasas", Grupo: "Impuestos y nómina",
			Ayuda: "Retenciones y tasas por defecto.", Naturaleza: []string{"L", "A", "E"}, Estandar: 4021101},
		{Clave: "salary_code", Titulo: "Sueldos y salarios", Grupo: "Impuestos y nómina",
			Ayuda: "Gasto de nómina.", Naturaleza: []string{"E"}, Estandar: 4020501},
		{Clave: "prov_state_tax", Titulo: "Impuesto estatal por pagar", Grupo: "Impuestos y nómina",
			Ayuda: "Pasivo por impuesto estatal.", Naturaleza: []string{"L", "A"}, Estandar: 5020101},
	}
}

// rolesPorClave indexa la lista para no repetir búsquedas lineales.
func rolesPorClave() map[string]RolCuenta {
	m := make(map[string]RolCuenta)
	for _, r := range rolesCuenta() {
		m[r.Clave] = r
	}
	return m
}

// cuentasColumnasLegacy son las que existen desde la primera versión del ERP.
var cuentasColumnasLegacy = []string{
	"cashCode", "bankCode", "purchaseCode", "salesCode", "serviceCode",
	"customerCode", "supplierCode", "costs_of_good_solds", "vat", "tax",
	"inventoryCode", "salary_code", "prov_state_tax",
}

// cuentaColumnasNuevas son las de la separación. Se leen aparte porque una base
// que todavía no corrió la migración no las tiene y el panel debe abrir igual.
var cuentaColumnasNuevas = []string{
	"supplies_inventory_code", "supplies_expense_code",
	"service_expense_code", "purchase_vat_code",
}

// leerCuentasRol devuelve el valor configurado para cada papel.
func leerCuentasRol(q consultadorUnaFila) (map[string]int64, error) {
	out := make(map[string]int64, len(rolesCuenta()))
	vals := make([]int64, len(cuentasColumnasLegacy))
	dest := make([]any, len(vals))
	for i := range vals {
		dest[i] = &vals[i]
	}
	err := q.QueryRow(`SELECT ` + strings.Join(cuentasColumnasLegacy, ", ") +
		` FROM acc_predefine_account ORDER BY id LIMIT 1`).Scan(dest...)
	if err != nil {
		return nil, fmt.Errorf("no hay configuración de cuentas (acc_predefine_account): %w", err)
	}
	for i, col := range cuentasColumnasLegacy {
		out[col] = vals[i]
	}
	nuevas := make([]int64, len(cuentaColumnasNuevas))
	destN := make([]any, len(nuevas))
	for i := range nuevas {
		destN[i] = &nuevas[i]
	}
	// Sin columnas nuevas (base sin migrar) los papeles quedan en 0 y la
	// pantalla los muestra como «sin asignar»; no se inventa un respaldo.
	if err := q.QueryRow(`SELECT IFNULL(supplies_inventory_code,0), IFNULL(supplies_expense_code,0),
			IFNULL(service_expense_code,0), IFNULL(purchase_vat_code,0)
		FROM acc_predefine_account ORDER BY id LIMIT 1`).Scan(destN...); err == nil {
		for i, col := range cuentaColumnasNuevas {
			out[col] = nuevas[i]
		}
	}
	return out, nil
}

// sinAcentos deja el texto comparable: «crédito» y «credito» son la misma
// palabra para reconocer una cuenta.
func sinAcentos(v string) string {
	reemplazos := map[rune]rune{
		'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u', 'ü': 'u',
		'Á': 'a', 'É': 'e', 'Í': 'i', 'Ó': 'o', 'Ú': 'u', 'Ü': 'u', 'ñ': 'n', 'Ñ': 'n',
	}
	return strings.Map(func(r rune) rune {
		if n, ok := reemplazos[r]; ok {
			return n
		}
		return r
	}, strings.ToLower(v))
}

// nombreCompatible comprueba que el nombre de la cuenta corresponda al papel.
// Es la segunda llave del vínculo automático, junto con la naturaleza: evita
// atar «Costo de ventas» a una cuenta de gasto cualquiera que casualmente tenga
// el mismo código en otro plan.
func nombreCompatible(nombre string, claves []string) bool {
	if len(claves) == 0 {
		return true
	}
	n := sinAcentos(nombre)
	for _, c := range claves {
		if strings.Contains(n, sinAcentos(c)) {
			return true
		}
	}
	return false
}

// estadoRolCuenta dice si el papel tiene una cuenta utilizable.
type estadoRolCuenta struct {
	Rol      RolCuenta
	Codigo   int64
	Estado   string // ok | sin_asignar | invalida
	Motivo   string
	Nombre   string
	Tipo     string
	Esperado string
}

// revisarRolCuenta comprueba que la cuenta exista, esté activa, admita
// movimientos y sea de la naturaleza que el papel exige. Es la misma regla que
// aplica el guardado, para que la pantalla y el servidor no discrepen.
func revisarRolCuenta(q consultadorUnaFila, rol RolCuenta, codigo int64) error {
	if codigo <= 0 {
		return fmt.Errorf("sin cuenta asignada")
	}
	var tipo string
	var activa, transaccional int
	err := q.QueryRow(`SELECT IFNULL(HeadType,''), IFNULL(IsActive,0), IFNULL(IsTransaction,0)
		FROM acc_coa WHERE HeadCode = ?`, codigo).Scan(&tipo, &activa, &transaccional)
	if err == sql.ErrNoRows {
		return fmt.Errorf("la cuenta %d no existe en el plan de cuentas", codigo)
	}
	if err != nil {
		return fmt.Errorf("no se pudo leer la cuenta %d: %w", codigo, err)
	}
	if activa != 1 {
		return fmt.Errorf("la cuenta %d está inactiva", codigo)
	}
	if transaccional != 1 {
		return fmt.Errorf("la cuenta %d es de grupo: no admite movimientos, elija una cuenta de detalle", codigo)
	}
	tipo = strings.ToUpper(strings.TrimSpace(tipo))
	for _, n := range rol.Naturaleza {
		if tipo == n {
			return nil
		}
	}
	return fmt.Errorf("la cuenta %d es de %s y «%s» necesita %s", codigo,
		nombresNaturaleza([]string{tipo}), rol.Titulo, nombresNaturaleza(rol.Naturaleza))
}

// nombresNaturaleza traduce A/L/I/E a algo legible.
func nombresNaturaleza(ns []string) string {
	nombre := map[string]string{"A": "activo", "L": "pasivo", "I": "ingreso", "E": "gasto"}
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		if v, ok := nombre[n]; ok {
			out = append(out, v)
		} else {
			out = append(out, n)
		}
	}
	return strings.Join(out, " o ")
}

// estadoCuentasRol arma la foto que muestra la pantalla: valor, estado y motivo.
func estadoCuentasRol(q consultadorUnaFila) ([]estadoRolCuenta, error) {
	valores, err := leerCuentasRol(q)
	if err != nil {
		return nil, err
	}
	out := make([]estadoRolCuenta, 0, len(rolesCuenta()))
	for _, rol := range rolesCuenta() {
		e := estadoRolCuenta{Rol: rol, Codigo: valores[rol.Clave], Estado: "ok", Esperado: nombresNaturaleza(rol.Naturaleza)}
		if e.Codigo <= 0 {
			e.Estado = "sin_asignar"
			e.Motivo = "sin cuenta asignada"
			out = append(out, e)
			continue
		}
		if err := revisarRolCuenta(q, rol, e.Codigo); err != nil {
			e.Estado = "invalida"
			e.Motivo = err.Error()
		} else {
			_ = q.QueryRow(`SELECT HeadName FROM acc_coa WHERE HeadCode = ?`, e.Codigo).Scan(&e.Nombre)
			_ = q.QueryRow(`SELECT HeadType FROM acc_coa WHERE HeadCode = ?`, e.Codigo).Scan(&e.Tipo)
		}
		out = append(out, e)
	}
	return out, nil
}

// cuentasAsignables devuelve las cuentas que pueden moverse, para los selectores.
func (s *Server) cuentasAsignables(r *http.Request) []map[string]interface{} {
	rows, err := s.dbFor(r).Query(`
		SELECT HeadCode, HeadName, IFNULL(HeadType,'')
		FROM acc_coa
		WHERE IsActive = 1 AND IsTransaction = 1
		ORDER BY HeadCode`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var codigo int64
		var nombre, tipo string
		if rows.Scan(&codigo, &nombre, &tipo) != nil {
			continue
		}
		out = append(out, map[string]interface{}{"code": codigo, "name": nombre, "type": tipo,
			"type_label": nombresNaturaleza([]string{tipo})})
	}
	return out
}

// guardarCuentasRol valida y escribe el mapa de papeles. Todo o nada: si una
// cuenta no sirve, no se guarda ninguna, para que no quede media configuración.
func (s *Server) guardarCuentasRol(r *http.Request, valores map[string]int64) error {
	roles := rolesPorClave()
	columnas := []string{}
	args := []any{}
	for col, codigo := range valores {
		rol, ok := roles[col]
		if !ok {
			return fmt.Errorf("papel contable desconocido: %s", col)
		}
		if codigo > 0 {
			if err := revisarRolCuenta(s.dbFor(r), rol, codigo); err != nil {
				return fmt.Errorf("%s: %w", rol.Titulo, err)
			}
		}
		columnas = append(columnas, col+" = ?")
		args = append(args, codigo)
	}
	if len(columnas) == 0 {
		return fmt.Errorf("no hay cuentas que guardar")
	}
	res, err := s.dbFor(r).Exec(`UPDATE acc_predefine_account SET `+strings.Join(columnas, ", "), args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Sin fila no hay dónde guardar: no se inventa una con valores a medias.
		var existe int
		if s.dbFor(r).QueryRow(`SELECT COUNT(*) FROM acc_predefine_account`).Scan(&existe) == nil && existe == 0 {
			return fmt.Errorf("la empresa no tiene configuración contable: aplique la migración o cree la fila en acc_predefine_account")
		}
	}
	return nil
}

// vincularCuentasEstandar asigna al plan estándar los papeles de la separación
// que no tengan una cuenta válida.
//
// Solo toca los papeles marcados Auto y solo acepta una cuenta cuando el código
// existe, su naturaleza sirve y su NOMBRE corresponde al papel. Así, cargar el
// catálogo CONASEV no ata «Costo de ventas» a la cuenta 4010101 de ese catálogo
// si allí significa otra cosa: el papel queda pendiente y el contador lo elige.
//
// Los papeles generales (caja, ventas, clientes, proveedores) no se tocan: la
// empresa los tiene configurados desde antes y cambiarlos por código sería
// adivinar. Si dejaron de ser válidos, la pantalla los muestra y se corrigen ahí.
func vincularCuentasEstandar(db *sql.DB) (asignadas []string, pendientes []string, err error) {
	actuales, err := leerCuentasRol(db)
	if err != nil {
		return nil, nil, err
	}
	for _, rol := range rolesCuenta() {
		actual := actuales[rol.Clave]
		valido := actual > 0 && revisarRolCuenta(db, rol, actual) == nil
		if valido {
			continue // lo configurado sirve: la empresa manda
		}
		if !rol.Auto {
			if actual > 0 {
				// Tenía una cuenta que ya no sirve (por ejemplo, el plan cambió).
				// No se reemplaza a ciegas: se informa para que la elija.
				pendientes = append(pendientes, rol.Titulo)
			}
			continue
		}
		if rol.Estandar > 0 && revisarRolCuenta(db, rol, rol.Estandar) == nil {
			var nombre string
			_ = db.QueryRow(`SELECT IFNULL(HeadName,'') FROM acc_coa WHERE HeadCode = ?`, rol.Estandar).Scan(&nombre)
			if nombreCompatible(nombre, rol.Claves) {
				if _, e := db.Exec(`UPDATE acc_predefine_account SET `+rol.Clave+` = ?`, rol.Estandar); e != nil {
					return asignadas, pendientes, e
				}
				asignadas = append(asignadas, rol.Titulo)
				continue
			}
		}
		if actual > 0 {
			// La cuenta configurada ya no sirve y no hay equivalente seguro: se
			// deja en 0 en vez de seguir apuntando a una cuenta inexistente o de
			// otro significado.
			if _, e := db.Exec(`UPDATE acc_predefine_account SET ` + rol.Clave + ` = 0`); e != nil {
				return asignadas, pendientes, e
			}
		}
		pendientes = append(pendientes, rol.Titulo)
	}
	return asignadas, pendientes, nil
}

// papelesPendientesDeCuenta devuelve los papeles que el sistema usa para la
// separación y que hoy no tienen una cuenta válida. Se consulta antes de
// asentar y en la pantalla de configuración.
func (s *Server) papelesPendientesDeCuenta(r *http.Request) []string {
	estados, err := estadoCuentasRol(s.dbFor(r))
	if err != nil {
		return nil
	}
	criticos := map[string]bool{
		"inventoryCode": true, "costs_of_good_solds": true,
		"supplies_inventory_code": true, "supplies_expense_code": true,
		"service_expense_code": true, "purchase_vat_code": true,
	}
	var out []string
	for _, e := range estados {
		if criticos[e.Rol.Clave] && e.Estado != "ok" {
			out = append(out, e.Rol.Titulo)
		}
	}
	return out
}

// ---------- Pantalla ----------

// grupoRoles junta los papeles por grupo conservando el orden de la lista.
// La pantalla los muestra separados: operación diaria, inventario y costo,
// separación servicio/consumo/venta, impuestos y nómina.
type grupoRoles struct {
	Nombre string
	Roles  []estadoRolCuenta
}

func agruparRoles(estados []estadoRolCuenta) []grupoRoles {
	orden := []string{}
	porNombre := map[string][]estadoRolCuenta{}
	for _, e := range estados {
		if _, ok := porNombre[e.Rol.Grupo]; !ok {
			orden = append(orden, e.Rol.Grupo)
		}
		porNombre[e.Rol.Grupo] = append(porNombre[e.Rol.Grupo], e)
	}
	out := make([]grupoRoles, 0, len(orden))
	for _, nombre := range orden {
		out = append(out, grupoRoles{Nombre: nombre, Roles: porNombre[nombre]})
	}
	return out
}

func (s *Server) cuentasRolForm(w http.ResponseWriter, r *http.Request) {
	estados, err := estadoCuentasRol(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	info, _, _, _, _ := COACatalog(s.dbFor(r))
	s.render(w, r, "settings_accounts.html", map[string]interface{}{
		"Title":      "Cuentas por defecto",
		"Roles":      estados,
		"Grupos":     agruparRoles(estados),
		"Cuentas":    s.cuentasAsignables(r),
		"Catalogo":   catalogLabel(info),
		"Pendientes": s.papelesPendientesDeCuenta(r),
	})
}

// cuentasRolSave guarda el mapa completo. Los campos vacíos se guardan como 0
// (sin asignar), no como «lo que había».
func (s *Server) cuentasRolSave(w http.ResponseWriter, r *http.Request) {
	// Es una decisión contable: se exige el mismo permiso que para tocar el
	// plan de cuentas.
	if !s.hasPermission(r, "coa", "update") {
		jsonError(w, "No tiene permiso para modificar las cuentas por defecto", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	valores := map[string]int64{}
	for _, rol := range rolesCuenta() {
		campo := "cuenta_" + rol.Clave
		crudo := strings.TrimSpace(r.FormValue(campo))
		if crudo == "" {
			valores[rol.Clave] = 0
			continue
		}
		codigo, err := strconv.ParseInt(crudo, 10, 64)
		if err != nil || codigo <= 0 {
			jsonError(w, fmt.Sprintf("%s: la cuenta «%s» no es un código válido", rol.Titulo, crudo), 400)
			return
		}
		valores[rol.Clave] = codigo
	}
	if err := s.guardarCuentasRol(r, valores); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"status": 1, "msg": "Cuentas por defecto guardadas"})
}

// cuentasRolEstandar vuelve a atar los papeles al plan estándar del ERP.
func (s *Server) cuentasRolEstandar(w http.ResponseWriter, r *http.Request) {
	if !s.hasPermission(r, "coa", "update") {
		jsonError(w, "No tiene permiso para modificar las cuentas por defecto", http.StatusForbidden)
		return
	}
	asignadas, pendientes, err := vincularCuentasEstandar(s.dbFor(r))
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	msg := "Cuentas vinculadas al plan estándar"
	if len(pendientes) > 0 {
		msg = fmt.Sprintf("Se vincularon %d cuentas; quedan %d por elegir en el plan de la empresa",
			len(asignadas), len(pendientes))
	}
	writeJSON(w, map[string]interface{}{
		"status": 1, "msg": msg,
		"asignadas": asignadas, "pendientes": pendientes,
	})
}

func (s *Server) registerCuentasRolRoutes() {
	s.mux.HandleFunc("GET /settings/accounts", s.cuentasRolForm)
	s.mux.HandleFunc("POST /settings/accounts/save", s.cuentasRolSave)
	s.mux.HandleFunc("POST /settings/accounts/standard", s.cuentasRolEstandar)
}
