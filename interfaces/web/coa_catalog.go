package web

// coa_catalog.go — De quién es el plan de cuentas.
//
// La pregunta que este archivo responde es "¿de quién es la codificación
// contable?" y la respuesta es: de la empresa. No del sistema.
//
// El error que se está corrigiendo fue tratar el catálogo como si fuera del
// sistema. ERPGo traía un catálogo heredado del PHP (433 cuentas), el seeder del
// SENIAT agregaba el suyo encima (1855) y los dos se mezclaban en acc_coa. Como
// HeadCode es un entero sin restricción de unicidad, los códigos cortos que
// ambos catálogos comparten (101, 201, 301, 401, 501...) quedaron definidos por
// el que se sembró primero, y el nombre del otro desapareció sin aviso. Nueve
// cuentas con dos nombres distintos según quién mire, y ningún asiento que lo
// dijera.
//
// El arreglo no es elegir un ganador: es dejar de mezclar. Cada empresa elige.
//
//   - 'propio'   (por defecto) la empresa codifica como quiera. No se siembra
//                nada encima de lo suyo.
//   - 'conasev'  se ofrece el catálogo del SENIAT como punto de partida. Se
//                carga sobre una base VACÍA o se rechaza con una explicación,
//                nunca se fusiona a ciegas sobre un plan existente.

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// Catalogos contables reconocidos.
const (
	COACatalogPropio  = "propio"  // la empresa define sus códigos
	COACatalogCONASEV = "conasev" // catálogo del SENIAT, como estándar ofrecido
)

// COACatalogStatus describe el plan de cuentas de una empresa.
type COACatalogStatus struct {
	Catalog              string // propio | conasev
	Source               string // de dónde salió
	AppliedAt            string // cuándo se aplicó
	Notes                string
	AccountCount         int      // cuentas en acc_coa
	CollisionCount       int      // códigos con dos nombres distintos
	Collisions           []string // detalle de las colisiones
	OrderAccountsMissing []string // cuentas de orden 0x que no caben en HeadCode
}

// COACatalog lee el catálogo vigente de la empresa. Si no hay ajuste, la empresa
// tiene plan propio: es el estado por defecto de quien nunca ha tocado esto, y
// también la respuesta honesta para una base migrada que ya tenía sus cuentas
// antes de que existiera esta tabla.
func COACatalog(db *sql.DB) (catalog, source, appliedAt, notes string, err error) {
	var c, s, a, n sql.NullString
	err = db.QueryRow(`SELECT catalog, source, applied_at, IFNULL(notes,'')
		FROM coa_catalog ORDER BY id LIMIT 1`).Scan(&c, &s, &a, &n)
	if err == sql.ErrNoRows {
		return COACatalogPropio, "plan propio de la empresa", "", "", nil
	}
	if err != nil {
		return "", "", "", "", err
	}
	catalog = c.String
	if catalog == "" {
		catalog = COACatalogPropio
	}
	return catalog, s.String, a.String, n.String, nil
}

// SetCOACatalog registra el catálogo vigente. Deliberadamente NO carga nada:
// elegir catálogo y aplicar catálogo son dos decisiones separadas, y aplicarlas
// de golpe convierte un clic en una modificación del plan contable.
func SetCOACatalog(db *sql.DB, catalog, source, notes, user string) error {
	if catalog != COACatalogPropio && catalog != COACatalogCONASEV {
		return fmt.Errorf("catálogo desconocido: %q", catalog)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	// Una sola fila: el ajuste vigente, no un historial. Si el plan se cambió, el
	// ajuste anterior ya no describe la realidad y guardarlo sería mentir.
	if _, err := db.Exec(`DELETE FROM coa_catalog`); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO coa_catalog (catalog, source, applied_at, notes, created_by, created_at)
		VALUES (?,?,?,?,?,?)`, catalog, source, now, notes, user, now)
	return err
}

// ApplyCOACatalog carga el catálogo CONASEV y registra la decisión.
//
// La regla que protege los datos del negocio: si acc_coa YA tiene cuentas que no
// son de CONASEV, la carga se RECHAZA y se explica. Fusionar dos catálogos en
// silencio es exactamente el bug que se está corrigiendo; hacerlo "porque sí" lo
// dejarlo pasando con otro nombre.
//
// Lo que se cuenta como "cuentas propias" son las que NO traen
// CreateBy='conasev'. El esquema base siembra el catálogo heredado del PHP en cada
// base, así que una empresa recién creada tiene 82 de esas cuentas y por eso el
// catálogo estándar se le rechaza: son suyas, no relleno.
func ApplyCOACatalog(db *sql.DB, user string) (SeedCOAResult, error) {
	var res SeedCOAResult

	var ajenas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM acc_coa
		WHERE IFNULL(CreateBy,'') <> 'conasev'`).Scan(&ajenas); err != nil {
		return res, err
	}
	if ajenas > 0 {
		return res, fmt.Errorf(
			"no se puede aplicar CONASEV: la empresa ya tiene %d cuentas propias. "+
				"Cargue el catálogo estándar solo sobre un plan que ya sea CONASEV, "+
				"o defina su propia codificación", ajenas)
	}

	res, err := SeedCONASEV(db)
	if err != nil {
		return res, err
	}
	if err := SetCOACatalog(db, COACatalogCONASEV, "SENIAT", "", user); err != nil {
		return res, err
	}
	// El plan cambió: los papeles que apuntaban a cuentas de la codificación
	// anterior ya no sirven. Se revalidan y, cuando el catálogo trae la cuenta
	// estándar, se vuelven a atar. Lo que no encuentra equivalente queda
	// pendiente y se le dice al contador, en vez de asentar contra un código
	// que ahora significa otra cosa.
	res.CuentasAsignadas, res.CuentasPendientes, _ = vincularCuentasEstandar(db)
	return res, nil
}

// InspectCOACatalog revisa el plan de cuentas y explica qué hay.
//
// Esto existe porque el problema era invisible: nadie podía ver que 101 tenía dos
// nombres. Un diagnóstico que se puede leer es la diferencia entre un bug y una
// sorpresa en una declaración.
func InspectCOACatalog(db *sql.DB) COACatalogStatus {
	st := COACatalogStatus{}
	st.Catalog, st.Source, st.AppliedAt, st.Notes, _ = COACatalog(db)

	_ = db.QueryRow(`SELECT COUNT(*) FROM acc_coa`).Scan(&st.AccountCount)

	// Colisiones: el mismo código definido por dos catálogos. Solo tiene sentido
	// buscarlas si la base TIENE cuentas de CONASEV: en una empresa con plan
	// propio, que un código se parezca a uno del SENIAT no es una colisión, es
	// que la empresa codifica como le place. Reportar eso como conflicto sería
	// gritarle al contador por usar su propio criterio.
	var conasevRows int
	_ = db.QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE IFNULL(CreateBy,'') = 'conasev'`).Scan(&conasevRows)
	if conasevRows > 0 {
		if conasev := parseCONASEV(); len(conasev) > 0 {
			esperado := make(map[int]string, len(conasev))
			for _, c := range conasev {
				esperado[c.Code] = c.Name
			}
			rows, err := db.Query(`SELECT HeadCode, HeadName FROM acc_coa
				WHERE IFNULL(CreateBy,'') <> 'conasev'`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var code int
					var name string
					if rows.Scan(&code, &name) != nil {
						continue
					}
					want, ok := esperado[code]
					if !ok || sameAccountName(name, want) {
						continue
					}
					st.Collisions = append(st.Collisions,
						fmt.Sprintf("%d: %q en el plan de la empresa vs %q en CONASEV", code, name, want))
				}
			}
		}
	}
	st.CollisionCount = len(st.Collisions)

	// Cuentas de orden: el SENIAT numera con 0x a propósito (01, 06, 0124...) y
	// HeadCode es un entero, así que el cero a la izquierda se pierde. Eso no es
	// un detalle menor: la cuenta de orden "01" se volvería 1, que en un plan
	// cualquiera es el grupo "Activos", y la "04" sería "Gastos". Son cuentas
	// distintas con el mismo número, y una columna numérica no puede guardarlas.
	//
	// Aquí no se puede resolver sin cambiar el tipo de la columna, que es una
	// migración de verdad. Se reporta, conservando el cero a la izquierda en el
	// texto, para que el contador vea que el número que falta es 0124 y no 124.
	for _, c := range parseCONASEVOrder() {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM acc_coa WHERE HeadCode = ?`, c.Code).Scan(&n)
		if n == 0 {
			st.OrderAccountsMissing = append(st.OrderAccountsMissing,
				fmt.Sprintf("%s %s", c.Orden, c.Name))
		}
	}

	return st
}

// logCOACatalog deja el plan contable registrado al arrancar. No es ruido: la
// próxima persona que mire una cuenta 101 tiene que poder ver en el log de qué
// plan vino y qué se respetó.
func logCOACatalog(st COACatalogStatus) {
	if st.CollisionCount > 0 {
		log.Printf("Plan de cuentas (%s): %d códigos con nombre distinto al catálogo estándar; "+
			"se respetó lo que la empresa ya tenía:", st.Catalog, st.CollisionCount)
		for _, c := range st.Collisions {
			log.Printf("   %s", c)
		}
	}
	if len(st.OrderAccountsMissing) > 0 {
		log.Printf("Plan de cuentas: %d cuentas de orden (0x) del SENIAT no cargadas; "+
			"el código de cuenta es numérico y no las admite", len(st.OrderAccountsMissing))
	}
}

// normalizeCatalog deja el valor en la forma canónica y tolera lo que una
// empresa escribiría a mano ("CONASEV", "Conasev", "seniat").
func normalizeCatalog(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "conasev", "seniat", "estandar":
		return COACatalogCONASEV
	default:
		return COACatalogPropio
	}
}
