// Command semillar carga la empresa Carnicería Lara con cinco meses de
// operación real en una base del ERPGo.
//
//	./semillar                    carga en la base de DB_NAME del .env
//	./semillar -base erpgo_nueva  carga en otra base
//	./semillar -vaciar            borra antes (pide confirmación)
//
// QUÉ HACE Y QUÉ NO HACE.
//
// Carga empresa, nómina, proveedores, clientes, catálogo, rendimientos, listas de
// precio y cinco meses de compras, ventas, cobranzas y cierre de costos, todo a
// través de los handlers de verdad del ERP. NO aplica migraciones: comprueba
// que la base ya las tiene y avisa si le faltan.
//
// POR QUÉ ES UN COMANDO Y NO UN SCRIPT DE SQL.
//
// Porque la contabilidad la genera el mismo código que la genera en producción.
// Un `INSERT` directo en `acc_vaucher` produciría una base con facturas y sin
// libro, que es lo que hace inútil un ERP para lo que sirve: demostrar que el
// número sale bien.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"erpgow/configs"
	"erpgow/infrastructure/persistence"
	"erpgow/interfaces/web"
)

func main() {
	base := flag.String("base", "", "base de datos donde cargar (vacío = DB_NAME del .env)")
	vaciar := flag.Bool("vaciar", false, "borra los datos de la carnicería antes de cargar")
	flag.Parse()

	// configs.Load() es la MISMA vía que usa cmd/server: lee el .env y las
	// variables de entorno. No se relee el .env a mano aquí, porque entonces este
	// comando y el servidor podrían apuntar a bases distintas sin que nadie lo
	// notara, que es la forma más cara de equivocarse.
	cfg, err := configs.Load()
	if err != nil {
		log.Fatalf("no se pudo leer la configuración: %v", err)
	}

	nombre := *base
	if nombre == "" {
		nombre = cfg.DBName
	}
	if nombre == "" {
		log.Fatal("no hay base de destino: pasa -base o pon DB_NAME en el .env")
	}

	conn, err := persistence.OpenMySQL(persistence.ConfigMySQL{
		Host:    cfg.DBHost,
		Port:    cfg.DBPort,
		Usuario: cfg.DBUser,
		Clave:   cfg.DBPassword,
		Nombre:  nombre,
	})
	if err != nil {
		log.Fatalf("no se pudo abrir la base %s: %v", nombre, err)
	}
	defer conn.Close()

	if err := comprobarEsquema(conn); err != nil {
		log.Fatalf("%v", err)
	}
	if *vaciar {
		if err := pedirConfirmacion(nombre); err != nil {
			log.Fatal(err)
		}
		if err := vaciarCarniceria(conn); err != nil {
			log.Fatalf("no se pudo vaciar: %v", err)
		}
		fmt.Println("Datos anteriores de la carnicería eliminados.")
	}

	fmt.Printf("Cargando la Carnicería Lara en %s...\n", nombre)
	inf, err := web.SemillarCarniceriaLara(conn, nombre)
	if err != nil {
		log.Fatalf("falló la siembra: %v", err)
	}
	fmt.Println()
	fmt.Println(inf.Resumen())

	// Un descuadre se avisa, no se esconde: el libro tiene que cuadrar Y tener
	// importes dentro.
	//
	// UN LIBRO VACÍO CUADRA, Y ESO ES LO PEOR QUE PUEDE DECIR ESTE INFORME.
	// Con 0 en el debe y 0 en el haber la diferencia es 0, y este comando
	// anunciaba "el libro mayor cuadra" sobre una base con 840 asientos TODOS a
	// 0,00. Es el peor tipo de comprobación: la que pasa cuando no pasa nada.
	// Por eso se mira también que HAYA importe.
	switch {
	case inf.Librovacio:
		fmt.Println("\nFALLO: el libro mayor no tiene importes.")
		fmt.Println("Hay asientos, pero todos a 0,00. Los asientos contables se")
		fmt.Println("crearon sin importe, así que la contabilidad NO está verificada.")
		os.Exit(1)
	case inf.Descuadre > 0.01 || inf.Descuadre < -0.01:
		fmt.Printf("\nFALLO: el libro mayor descuadra en %.2f.\n", inf.Descuadre)
		os.Exit(1)
	}
	fmt.Println("\nEl libro mayor cuadra y tiene importes.")
}

// comprobarEsquema verifica que la base tenga lo mínimo, y avisa con nombre de
// tabla si falta. Aplicar migraciones automáticamente sería escribir en una base
// de producción sin que nadie lo pidiera.
func comprobarEsquema(db interface{ QueryRow(string, ...any) *sql.Row }) error {
	requeridas := []string{
		"product_information", "product_purchase", "product_lot", "invoice",
		"customer_information", "supplier_information", "hr_employee",
		"hr_payroll_run", "cost_timesheet", "product_yield", "cost_snapshot",
	}
	for _, t := range requeridas {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, t).Scan(&n); err != nil {
			return fmt.Errorf("no se pudo comprobar el esquema: %v", err)
		}
		if n == 0 {
			return fmt.Errorf("a la base le falta la tabla %q.\n"+
				"El semillero NO aplica migraciones por su cuenta: aplica el esquema "+
				"primero (el binario del ERP lo hace al arrancar con -migrate).", t)
		}
	}
	return nil
}

// vaciarCarniceria borra lo que dejó una siembra anterior.
//
// El orden respeta las dependencias: primero los hijos (renglones), después las
// cabeceras. Y va de datos de negocio a catálogos, nunca al revés: borrar
// `product_information` antes que `product_lot` deja lotes apuntando a productos
// que ya no existen.
func vaciarCarniceria(db interface {
	Exec(string, ...any) (sql.Result, error)
}) error {
	tablas := []string{
		"cost_snapshot", "cost_component", "cost_period_close",
		"hr_payroll_item", "hr_payroll_run",
		"acc_transaction", "acc_vaucher",
		"invoice_details", "invoice",
		"product_lot",
		"product_purchase_details", "product_purchase",
		"supplier_product",
		"customer_information", "supplier_information",
		"product_cost_config", "product_yield",
	}
	for _, t := range tablas {
		if _, err := db.Exec("DELETE FROM `" + t + "`"); err != nil {
			return fmt.Errorf("borrando %s: %w", t, err)
		}
	}
	// Los empleados y los productos se quitan por código, no todos: la base
	// puede tener otros datos que no son de esta empresa.
	if _, err := db.Exec(`DELETE FROM hr_employee WHERE code LIKE 'CAR-%'`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM product_information
		WHERE product_id IN ('1001','1002','1003','1004','1005','1006','1007','1008')`); err != nil {
		return err
	}
	return nil
}

// pedirConfirmacion no borra nada sin que alguien lo escriba.
//
// Vaciar es la única operación de este comando que no se puede deshacer, así que
// exige una confirmación explícita. Aceptar por defecto —con un Enter, que es lo
// que pasa si alguien pulsa sin mirar— sería la peor opción posible.
func pedirConfirmacion(base string) error {
	fmt.Printf("\nSE VA A BORRAR lo que hay en %s.\nEscribe SI para confirmar: ", base)
	var r string
	if _, err := fmt.Scanln(&r); err != nil {
		return fmt.Errorf("cancelado")
	}
	if strings.TrimSpace(strings.ToUpper(r)) != "SI" {
		return fmt.Errorf("cancelado: no se escribió SI")
	}
	return nil
}
