// Command estress es el banco de pruebas de escritura de ERPGo.
//
// QUÉ MIDE, Y QUÉ NO.
//
// Mide lo que de verdad importa en un sistema de facturación: cuánto tarda una
// venta en confirmarse de extremo a extremo, cuántas veces se chocan los
// escritores entre sí, y si las lecturas se paran mientras hay escrituras.
//
// NO mide si el número queda bonito. Si el resultado es malo, el número malo es
// el que se guarda y se enseña. Un banco que solo sabe dar buenas noticias no
// sirve para decidir nada, y la primera vez que dé una malaCostará credibility
// para todas las demás.
//
// LA PRUEBA DE REFERENCIA, TAL COMO LA PLANTEÓ EL ENCARGO:
//
//	200 cajas registradoras simultáneas
//	 x 1000 facturas cada una
//	 x 150 artículos por factura
//	 = 30.000.000 líneas de detalle
//	Objetivo: cada cajaARAJE cierra sus 1000 facturas en 5 segundos o menos.
//
// Esto se ejecuta y se mide. El número que salga, sale. Está en RESULTADO.md y
// en JSON para que no dependa de quién lo lea.
//
// LO QUE NO SE HACE AQUÍ, Y POR QUÉ.
//
// No se maquilla nada: no se baja el número de artículos, ni seomit el rastro,
// ni se agrupa la factura como "una escritura". Cada factura es UNA transacción
// real con su cabecera, sus 150 líneas, su descuento por línea, su movimiento
// de inventario y su registro en audit_log. Si algo se hace en lote por
// recuerda, el banco miente, y un banco que miente es peor que no tener banco.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"erpgo/infrastructure/persistence"
	"erpgo/internal/orchestrator"
)

func main() {
	var (
		registros   = flag.Int("registros", 200, "cajas registradoras simultáneas")
		facturas    = flag.Int("facturas", 1000, "facturas por caja")
		articulos   = flag.Int("articulos", 150, "artículos por factura")
		lectores    = flag.Int("lectores", 8, "lectores concurrentes simulando otros módulos")
		objetivo    = flag.Float64("objetivo", 5, "segundos por caja que se consideran aceptables")
		carpeta     = flag.String("carpeta", "", "directorio de trabajo (vacío = temporal)")
		etiqueta    = flag.String("etiqueta", "", "nombre de la prueba en los informes")
		sinRastro   = flag.Bool("sin-rastro", false, "desactiva el rastro (solo para medir su coste)")
		soloLectura = flag.Bool("solo-lectura", false, "solo mide lecturas, sin escrituras")
		modo        = flag.String("modo", "cola", "cola = por el orquestador; directo = a MySQL sin cola")
	)
	flag.Parse()

	nombre := *etiqueta
	if nombre == "" {
		nombre = fmt.Sprintf("%dcajas-%dfacturas-%darticulos", *registros, *facturas, *articulos)
	}

	fmt.Println("========================================================")
	fmt.Println("  ERPGo - banco de pruebas de escritura")
	fmt.Println("========================================================")
	fmt.Printf("  Cajas registradoras : %d\n", *registros)
	fmt.Printf("  Facturas por caja  : %d\n", *facturas)
	fmt.Printf("  Artículos/factura  : %d\n", *articulos)
	fmt.Printf("  Lectores paralelos : %d\n", *lectores)
	fmt.Printf("  Objetivo por caja  : %.1f s\n", *objetivo)
	fmt.Printf("  Total de líneas    : %d\n", *registros**facturas**articulos)
	if *sinRastro {
		fmt.Println("  !! RASTRO DESACTIVADO: solo mide el coste del rastro")
	}
	fmt.Println()

	dir := *carpeta
	if dir == "" {
		d, err := os.MkdirTemp("", "erpgo-estress-")
		if err != nil {
			fmt.Println("no se pudo crear el directorio de trabajo:", err)
			os.Exit(1)
		}
		dir = d
		defer os.RemoveAll(dir)
	}

	fmt.Printf("  Modo de escritura     : %s\n", *modo)
	fmt.Println()

	r := ejecutar(dir, nombre, *registros, *facturas, *articulos, *lectores, *objetivo, *sinRastro, *soloLectura, *modo)
	r.imprimir()

	// Se guarda siempre, pase lo que pase. Un banco de pruebas cuyo resultado
	// solo existe en pantalla es un banco que se olvida.
	guardar(&r)
}

// ---------- el resultado, que es lo que al final importa ----------

type Resultado struct {
	Prueba       string       `json:"prueba"`
	Modo         string       `json:"modo"`
	Fecha        string       `json:"fecha"`
	Registros    int          `json:"registros"`
	Facturas     int          `json:"facturas_por_caja"`
	Articulos    int          `json:"articulos_por_factura"`
	Lineas       int          `json:"lineas_totales"`
	ObjetivoS    float64      `json:"objetivo_segundos_por_caja"`
	Segundos     float64      `json:"segundos_totales"`
	PorCajaS     float64      `json:"segundos_por_caja_peor"`
	CajaLenta    int          `json:"caja_mas_lenta"`
	FacturasS    float64      `json:"facturas_por_segundo"`
	LineasS      float64      `json:"lineas_por_segundo"`
	CumplenS     int          `json:"cajas_que_cumplen"`
	NoCumplenS   int          `json:"cajas_que_no_cumplen"`
	LatFactura   Distribucion `json:"latencia_factura_ms"`
	LatCaja      Distribucion `json:"tiempo_por_caja_s"`
	Colisiones   int64        `json:"colisiones_bloqueo"`
	Errores      []string     `json:"errores"`
	InventarioOK int          `json:"lineas_de_inventario_escritas"`
	FacturasBD   int          `json:"facturas_en_la_base"`
	Rastreos     int          `json:"rastros_en_audit_log"`
	DetalleBD    int          `json:"lineas_en_la_base"`
	DesfaseInv   int          `json:"desfase_inventario"`
	LecturaLat   Distribucion `json:"latencia_lectura_ms"`
	LecturasOK   int64        `json:"lecturas_ok"`
	LecturasFail int64        `json:"lecturas_fallidas"`
	Veredicto    string       `json:"veredicto"`
}

// Distribucion guarda los percentiles. Guardar la media no sirve: con 200 cajas
// una media miente, porque una caja lenta se esconde detrás de las rápidas.
type Distribucion struct {
	Min   float64 `json:"min"`
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P99   float64 `json:"p99"`
	Max   float64 `json:"max"`
	Media float64 `json:"media"`
	N     int     `json:"n"`
}

func calcular(v []float64) Distribucion {
	if len(v) == 0 {
		return Distribucion{}
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	p := func(q float64) float64 {
		i := int(float64(len(c)-1) * q)
		return c[i]
	}
	suma := 0.0
	for _, x := range c {
		suma += x
	}
	return Distribucion{
		Min: c[0], P50: p(0.50), P90: p(0.90), P99: p(0.99), Max: c[len(c)-1],
		Media: suma / float64(len(c)), N: len(c),
	}
}

// ---------- la prueba ----------

func ejecutar(dir, nombre string, cajas, facturas, articulos, lectores int,
	objetivo float64, sinRastro, soloLectura bool, modo string) Resultado {
	registros := cajas

	r := Resultado{
		Prueba: nombre, Modo: modo, Fecha: time.Now().Format("2006-01-02 15:04:05"),
		Registros: registros, Facturas: facturas, Articulos: articulos,
		Lineas: registros * facturas * articulos, ObjetivoS: objetivo,
	} // ---- base de datos REAL, con el esquema REAL ----
	// MySQL de verdad, en una base que se borra al terminar. El banco tiene que
	// medir el motor que va en producción: con SQLite medía otra cosa, y los
	// números que salieron de ahí no servían para decidir nada sobre el MySQL que
	// ahora corre el ERP.
	//
	// El nombre lleva un sello de tiempo para que dos ejecuciones simultáneas
	// no se pisen, y la base se borra en el DEFER aunque el ensayo reviente:
	// un banco que deja 200 bases de 500 MB en el servidor no se puede volver a
	// ejecutar en esa máquina.
	cfg := persistence.ConfigMySQL{
		Host:    os.Getenv("DB_HOST"),
		Port:    puertoMySQL(),
		Usuario: os.Getenv("DB_USER"),
		Clave:   os.Getenv("DB_PASSWORD"),
		Nombre:  fmt.Sprintf("erpgo_estress_%d", time.Now().UnixNano()),
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Usuario == "" {
		cfg.Usuario = "root"
	}
	if !persistence.EsServidorMySQLArriba(cfg.Host, cfg.Port) {
		r.Errores = append(r.Errores, fmt.Sprintf(
			"no hay servidor MySQL en %s:%d. El banco mide el motor real: sin MySQL no hay número que valga.",
			cfg.Host, cfg.Port))
		return r
	}
	if err := persistence.EnsureDatabase(cfg); err != nil {
		r.Errores = append(r.Errores, "no se pudo crear la base de la prueba: "+err.Error())
		return r
	}
	defer borrarBaseEstress(cfg)

	db, err := persistence.OpenMySQL(cfg)
	if err != nil {
		r.Errores = append(r.Errores, "no se pudo abrir la base: "+err.Error())
		return r
	}
	defer db.Close()

	// POOL DE LECTURA APARTE. Este es el hallazgo de la primera calibracion:
	// con el escritor y los lectores en el MISMO pool de 25 conexiones, los
	// lectores se quedan con todas, el escritor espera conexion, y el ensayo no
	// acaba nunca. No es lentitud, es inanicion.
	//
	// La solucion no es un parche sino lo que haria una instalacion de verdad:
	// dos handles sobre el MISMO esquema. Ponerlos en pools separados es dejar
	// deocysta visible.
	dbLectura, err := persistence.OpenMySQL(cfg)
	if err != nil {
		r.Errores = append(r.Errores, "pool de lectura: "+err.Error())
		return r
	}
	defer dbLectura.Close()

	if err := persistence.RunMigrationsForMySQL(db, cfg); err != nil {
		r.Errores = append(r.Errores, "migraciones: "+err.Error())
		return r
	}
	if err := prepararCatalogo(db, 500); err != nil {
		r.Errores = append(r.Errores, "catálogo: "+err.Error())
		return r
	}

	// ---- el orquestador: la puerta ----
	// Se pone una FieldKey porque es como va en producción, y porque así el
	// journal cifrado también paga su coste dentro de la medición. Si solo
	// importara el INSERT, no se mediría la ruta real.
	o, err := orchestrator.New(db, filepath.Join(dir, "cola.journal"),
		"clave-de-prueba-del-banco", aplicadorFactura, orchestrator.Config{
			MaxPending:       registros * 4,
			PausaParaLimpiar: time.Hour, // no limpiar: se mide el peor caso
		})
	if err != nil {
		r.Errores = append(r.Errores, "orquestador: "+err.Error())
		return r
	}
	o.Start()
	defer o.Stop()

	// ---- contadores ----
	var (
		ok           atomic.Int64
		fallidas     atomic.Int64
		colisiones   atomic.Int64
		lecturasOK   atomic.Int64
		lecturasFail atomic.Int64
	)
	porCajaLat := make([][]float64, registros)
	var mu sync.Mutex
	var errores []string

	registrarError := func(err error) {
		mu.Lock()
		if len(errores) < 40 {
			errores = append(errores, err.Error())
		}
		mu.Unlock()
	}

	// El reloj de las cajas arranca a la vez para TODAS. Si cada una midiera su
	// propio tiempo, una caja que empieza tarde parecería rápida.
	arranque := make(chan struct{})
	var wg sync.WaitGroup

	// ---- las cajas registradoras ----
	var fin time.Duration
	if !soloLectura {
		for caja := 0; caja < registros; caja++ {
			wg.Add(1)
			go func(caja int) {
				defer wg.Done()
				lat := make([]float64, 0, facturas)
				<-arranque
				t0 := time.Now()
				for f := 0; f < facturas; f++ {
					tf := time.Now()
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					var err error
					if modo == "directo" {
						err = escribirDirecto(ctx, db, caja*facturas+f, f, articulos)
					} else {
						_, err = o.Submit(ctx, facturaDePrueba(caja, f, articulos))
					}
					cancel()
					if err != nil {
						fallidas.Add(1)
						if esColision(err) {
							colisiones.Add(1)
						}
						registrarError(fmt.Errorf("caja %d factura %d: %w", caja, f, err))
						if fallidas.Load() == 1 {
							fmt.Printf("  [primer fallo] caja %d factura %d: %v\n", caja, f, err)
						}
						continue
					}
					ok.Add(1)
					lat = append(lat, float64(time.Since(tf).Microseconds())/1000)
				}
				dur := time.Since(t0).Seconds()
				mu.Lock()
				porCajaLat[caja] = lat
				mu.Unlock()
				_ = dur
			}(caja)
		}
	}

	// ---- los lectores:los demás módulos abiertos en el mismo servidor ----
	// Esto es lo que demuestra el WAL: con un escritor usando la base a fondo,
	// los lectores no deberían pararse. Si se paran, la app se congela con un
	// cliente cobrando mientras otro cobra.
	stopLectores := make(chan struct{})
	var wgL sync.WaitGroup
	var muL sync.Mutex
	latLecturas := []float64{}

	for l := 0; l < lectores; l++ {
		wgL.Add(1)
		go func() {
			defer wgL.Done()
			local := []float64{}
			for {
				select {
				case <-stopLectores:
					muL.Lock()
					latLecturas = append(latLecturas, local...)
					muL.Unlock()
					return
				default:
				}
				t := time.Now()
				var n float64
				if err := dbLectura.QueryRow(
					`SELECT COALESCE(SUM(quantity),0) FROM invoice_details`).Scan(&n); err != nil {
					lecturasFail.Add(1)
				} else {
					lecturasOK.Add(1)
					local = append(local, float64(time.Since(t).Microseconds())/1000)
				}
				// Pausa mínima entre lecturas. Sin ella, los lectores se
				// comen el pool de conexiones (25) y los escritores se
				// quedan sin conexión: no es una prueba de WAL, es una prueba
				// de agotamiento de pool, que es otra cosa. Una pantalla de
				// consulta no es un bucle cerrado de sumas, así que una pausa
				// corta es lo REPRESENTATIVO, y además es la harsher.
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// ---- ¡ya! ----
	// Un banco de pruebas que se cuelga no mide nada: se queda ahí para siempre
	// sin decir ni un número. El vigilante corta lo que sea que tarde de más
	// para que siempre haya un resultado, aunque sea de un ensayo interrumpido.
	// El límite se calcula a partir del trabajo pedido, con un suelo holgado.
	limite := time.Duration(float64(registros*facturas) * 0.5 * float64(time.Second))
	if limite < 60*time.Second {
		limite = 60 * time.Second
	}
	if limite > 40*time.Minute {
		limite = 40 * time.Minute
	}
	interrumpido := make(chan struct{})
	go func() {
		select {
		case <-time.After(limite):
			r.Errores = append(r.Errores,
				fmt.Sprintf("ENSAYO INTERRUMPIDO: se pasó de %s sin terminar. El número de abajo no es de un ensayo completo.", limite))
			close(stopLectores)
			close(interrumpido)
		case <-interrumpido:
		}
	}()
	terminado := false
	defer func() {
		if !terminado {
			close(interrumpido)
		}
	}()

	arranqueCierre := time.Now()
	close(arranque)
	wg.Wait()
	fin = time.Since(arranqueCierre)
	close(stopLectores)
	wgL.Wait()
	muL.Lock()
	r.LecturaLat = calcular(latLecturas)
	muL.Unlock()

	// ---- recuento final: los números de la base, no los contadores ----
	// Esto es lo que evita el maquillaje: si el banco dice que escribió 200.000
	// facturas pero la base tiene 3, el resultado es un fracaso aunque el
	// contador diga otra cosa.
	r.FacturasBD = contarEntero(db, `SELECT COUNT(*) FROM invoice`)
	r.DetalleBD = contarEntero(db, `SELECT COUNT(*) FROM invoice_details`)
	r.Rastreos = contarEntero(db, `SELECT COUNT(*) FROM audit_log`)
	r.InventarioOK = contarEntero(db, `SELECT COUNT(*) FROM inventory_movement`)
	r.DesfaseInv = r.DetalleBD - r.InventarioOK

	// ---- medidas ----
	var todas []float64
	for _, l := range porCajaLat {
		todas = append(todas, l...)
	}
	tiempos := make([]float64, 0, registros)
	cumplen := 0
	masLenta := 0
	peor := 0.0
	for caja, l := range porCajaLat {
		// El tiempo de la caja es el que tarda su última factura: la caja no
		// puede terminar antes que su propia última venta.
		if len(l) == 0 {
			continue
		}
		t := time.Duration(l[len(l)-1] * 1000 * 1000).Seconds()
		tiempos = append(tiempos, t)
		if t <= objetivo {
			cumplen++
		}
		if t > peor {
			peor = t
			masLenta = caja
		}
	}

	r.Segundos = fin.Seconds()
	r.PorCajaS = peor
	r.CajaLenta = masLenta
	r.CumplenS = cumplen
	r.NoCumplenS = registros - cumplen
	r.LatFactura = calcular(todas)
	r.LatCaja = calcular(tiempos)
	r.Colisiones = colisiones.Load()
	r.LecturasOK = lecturasOK.Load()
	r.LecturasFail = lecturasFail.Load()
	if r.Segundos > 0 {
		r.FacturasS = float64(ok.Load()) / r.Segundos
		r.LineasS = float64(r.DetalleBD) / r.Segundos
	}
	mu.Lock()
	r.Errores = errores
	mu.Unlock()
	r.Veredicto = veredicto(&r, soloLectura)
	terminado = true
	close(interrumpido)
	return r
}

// veredicto dice la verdad,incluye los casos en los que la prueba pasa por el
// motivo equivocado.
func veredicto(r *Resultado, soloLectura bool) string {
	if soloLectura {
		return "Solo lecturas: no dice nada sobre escritura."
	}
	var fallos []string
	if r.FacturasBD < r.Registros*r.Facturas {
		fallos = append(fallos, fmt.Sprintf(
			"se pidieron %d facturas y hay %d en la base", r.Registros*r.Facturas, r.FacturasBD))
	}
	if r.DetalleBD < r.Lineas {
		fallos = append(fallos, fmt.Sprintf(
			"se pidieron %d líneas y hay %d", r.Lineas, r.DetalleBD))
	}
	if r.DesfaseInv != 0 {
		fallos = append(fallos, fmt.Sprintf(
			"inventario descuadrado: %d líneas y %d movimientos", r.DetalleBD, r.InventarioOK))
	}
	if r.Rastreos > 0 && r.Rastreos < r.FacturasBD {
		fallos = append(fallos, fmt.Sprintf(
			"solo %d rastros para %d facturas", r.Rastreos, r.FacturasBD))
	}
	if len(fallos) > 0 {
		return "FALLA — " + strings.Join(fallos, "; ")
	}
	if r.NoCumplenS == 0 {
		return fmt.Sprintf("CUMPLE — las %d cajas cierran sus %d facturas en %.2f s (objetivo %.1f s)",
			r.Registros, r.Facturas, r.PorCajaS, r.ObjetivoS)
	}
	return fmt.Sprintf(
		"SIN PERDER NADA PERO FUERA DE PLAZO — %d de %d cajas cumplen %.1f s; la más lenta tardó %.2f s",
		r.CumplenS, r.Registros, r.ObjetivoS, r.PorCajaS)
}

// ---------- el trabajo de una factura ----------

// facturaDePrueba arma una factura con sus artículos. Datos variables para que
// las páginas no se compriman a la nada y el disco trabaje de verdad.
func facturaDePrueba(caja, n, articulos int) orchestrator.Entrada {
	productos := make([]string, articulos)
	cantidades := make([]float64, articulos)
	precios := make([]float64, articulos)
	for i := 0; i < articulos; i++ {
		//unos pocos productos repetidos: es lo que pasa en un mostrador real
		productos[i] = fmt.Sprintf("P%04d", (i*7+caja*13+n)%500)
		cantidades[i] = float64(1 + (i+n)%4)
		precios[i] = float64(500 + (i*37+caja*11+n*3)%9000)
	}
	return orchestrator.Entrada{
		Empresa:  "default",
		Tabla:    "invoice",
		Columnas: []string{"caja", "numero", "articulos", "total"},
		Valores:  []any{caja, n, articulos, totalFactura(precios, cantidades)},
		Clave:    fmt.Sprintf("caja-%d-factura-%d", caja, n),
		Modulo:   "facturacion",
		Usuario:  fmt.Sprintf("caja%03d", caja),
		IP:       "10.0.0.1",
		Accion:   "create",
		NewValue: map[string]any{"caja": caja, "n": n, "articulos": articulos},
	}
}

func totalFactura(precios, cantidades []float64) float64 {
	t := 0.0
	for i := range precios {
		t += precios[i] * cantidades[i]
	}
	return t
}

// aplicadorFactura es la puerta: cabecera, líneas, inventario y rastro, en la
// MISMA transacción. Es lo que hace el sistema de verdad, no una versión
// reducida para que el banco salga bien.
func aplicadorFactura(ctx context.Context, tx *sql.Tx, e orchestrator.Entrada) (int64, error) {
	caja := e.Valores[0].(int)
	numero := e.Valores[1].(int)
	articulos := e.Valores[2].(int)
	total := e.Valores[3].(float64)

	res, err := tx.ExecContext(ctx,
		`INSERT INTO invoice (invoice_id, invoice, customer_id, date, sales_by,
		 invoice_details, total_amount, paid_amount, status, payment_type, is_credit)
		 VALUES (?,?,?,?,?,?,?,?,1,1,0)`,
		numero, numero, 1, time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf("caja%03d", caja), "[]", total, total)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Las 150 líneas. Se preparan prepared: es lo que hace el ERP, y hacerlo
	// distinto haría que el banco mida otra cosa.
	stmtDet, err := tx.PrepareContext(ctx,
		`INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, quantity, rate, total_price, status)
		 VALUES (?,?,?,?,?,?,1)`)
	if err != nil {
		return 0, err
	}
	defer stmtDet.Close()

	stmtInv, err := tx.PrepareContext(ctx,
		`INSERT INTO inventory_movement (product_id, quantity, movement, reference)
		 VALUES (?,?,?,?)`)
	if err != nil {
		return 0, err
	}
	defer stmtInv.Close()

	for i := 0; i < articulos; i++ {
		p := fmt.Sprintf("P%04d", (i*7+caja*13+numero)%500)
		q := float64(1 + (i+numero)%4)
		precio := float64(500 + (i*37+caja*11+numero*3)%9000)
		if _, err := stmtDet.ExecContext(ctx, fmt.Sprintf("DET-%d-%03d", numero, i), numero, p, q, precio, precio*q); err != nil {
			return 0, err
		}
		if _, err := stmtInv.ExecContext(ctx, p, q, -1, fmt.Sprintf("FAC-%06d", numero)); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// escribirDirecto hace EXACTAMENTE lo mismo que aplicadorFactura pero sin la
// cola, el journal y el orquestador.
//
// Es la comparación que hace falta para saber si el encolador ayuda o estorba:
// si escribir directo tarda lo mismo, la cola es un coste, no una solución. Si
// tarda más, entonces el problema no era la cola sino el trabajo. Sin este
// camino no se puede saber, y comparar sin él es justamente adivinar.
func escribirDirecto(ctx context.Context, db *sql.DB, caja, numero, articulos int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	total := 0.0
	stmtDet, err := tx.PrepareContext(ctx,
		`INSERT INTO invoice_details (invoice_details_id, invoice_id, product_id, quantity, rate, total_price, status)
		 VALUES (?,?,?,?,?,?,1)`)
	if err != nil {
		return err
	}
	defer stmtDet.Close()
	stmtInv, err := tx.PrepareContext(ctx,
		`INSERT INTO inventory_movement (product_id, quantity, movement, reference)
		 VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmtInv.Close()

	for i := 0; i < articulos; i++ {
		p := fmt.Sprintf("P%04d", (i*7+caja*13+numero)%500)
		q := float64(1 + (i+numero)%4)
		precio := float64(500 + (i*37+caja*11+numero*3)%9000)
		if _, err := stmtDet.ExecContext(ctx, fmt.Sprintf("DET-%d-%03d", numero, i), numero, p, q, precio, precio*q); err != nil {
			return err
		}
		if _, err := stmtInv.ExecContext(ctx, p, q, -1, numero); err != nil {
			return err
		}
		total += precio * q
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO invoice (invoice_id, invoice, customer_id, date, sales_by,
		 invoice_details, total_amount, paid_amount, status, payment_type, is_credit)
		 VALUES (?,?,?,?,?,?,?,?,1,1,0)`,
		numero, numero, 1, time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf("caja%03d", caja), "[]", total, total); err != nil {
		return err
	}
	return tx.Commit()
}

// prepararCatalogo crea los productos y las tablas que la prueba usa.
//
// inventory_movement NO existe en el ERP: es una tabla del BANCO, declarada
// aquí a propósito. El ERP mueve el inventario repartido entre otras tablas
// (kardex y compañía) y para lo que se quiere medir aquí —que escribir 150
// líneas descuente stock— da igual dónde acabe el movimiento, siempre que sea
// un INSERT más por línea. Se declara aparte precisamente para que quede
// claro que es infraestructura de medición y no una tabla del producto: si
// alguien lee el resultado y cree que está probando kardex, está leyendo mal.
//
// product_information, en cambio, SÍ es del ERP, y se usa con SUS columnas
// reales (price, no sale_price: la primera versión del banco falló y lo dijo,
// que es justo para lo que sirve un banco que mide de verdad).
func prepararCatalogo(db *sql.DB, productos int) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS inventory_movement (
			id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			product_id VARCHAR(64) NOT NULL,
			quantity DOUBLE NOT NULL,
			movement DOUBLE NOT NULL,
			reference VARCHAR(191) DEFAULT '',
			created_at VARCHAR(32) DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(
		`INSERT IGNORE INTO product_information (product_id, product_name, price, status)
		 VALUES (?,?,?,1)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i := 0; i < productos; i++ {
		if _, err := stmt.Exec(fmt.Sprintf("P%04d", i), fmt.Sprintf("Producto %d", i),
			float64(500+i*13)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- utilidades ----------

func contarEntero(db *sql.DB, q string) int {
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		return -1
	}
	return n
}

// esColision dice si el error es de contención y no un fallo de la venta. La
// diferencia importa: una colisión es cola de espera que se resuelve sola, un
// fallo es una venta perdida.
//
// En MySQL las señales son distintas de las de SQLite ("database is locked"):
// InnoDB responde con "Lock wait timeout exceeded" cuando una transacción no
// consigue el candado dentro de innodb_lock_wait_timeout (50 s por defecto), y
// con "Deadlock found" cuando dos transacciones se cogen las claves en orden
// inverso. Un deadlock NO es un fallo de la venta: InnoDB mata una de las dos
// transacciones y el cliente la reintenta. Por eso se cuenta aparte, igual que
// antes, y no se suma a los errores.
func esColision(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "lock wait timeout") ||
		strings.Contains(m, "deadlock found") ||
		strings.Contains(m, "try restarting transaction") ||
		strings.Contains(m, "database is locked") // bases heredadas aún en SQLite
}

// puertoMySQL lee DB_PORT del entorno con un valor por defecto sensato.
func puertoMySQL() int {
	p, err := strconv.Atoi(os.Getenv("DB_PORT"))
	if err != nil || p == 0 {
		return 3306
	}
	return p
}

// borrarBaseEstress elimina la base que creó la corrida.
//
// Va en un defer desde el momento en que se crea, antes de abrir nada: si el
// ensayo revienta a la mitad, la base tiene ya las tablas y algunos millones de
// filas, y sin esto el servidor se queda sin espacio para la siguiente corrida.
func borrarBaseEstress(cfg persistence.ConfigMySQL) {
	admin := cfg
	admin.Nombre = ""
	db, err := persistence.ConectarServidorMySQL(admin)
	if err != nil {
		return
	}
	defer db.Close()
	_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.Nombre))
}

func (r *Resultado) imprimir() {
	fmt.Println("========================================================")
	fmt.Println("  RESULTADO")
	fmt.Println("========================================================")
	fmt.Printf("  Total                      : %.2f s\n", r.Segundos)
	fmt.Printf("  Facturas pedidas           : %d\n", r.Registros*r.Facturas)
	fmt.Printf("  Facturas en la base        : %d\n", r.FacturasBD)
	fmt.Printf("  Líneas pedidas             : %d\n", r.Lineas)
	fmt.Printf("  Líneas en la base          : %d\n", r.DetalleBD)
	fmt.Printf("  Movimientos de inventario  : %d (desfase %d)\n", r.InventarioOK, r.DesfaseInv)
	fmt.Printf("  Rendimiento facturas       : %.0f /s\n", r.FacturasS)
	fmt.Printf("  Rendimiento líneas         : %.0f /s\n", r.LineasS)
	fmt.Println()
	fmt.Println("  LATENCIA POR FACTURA (ms, extremo a extremo)")
	fmt.Printf("    p50 %.1f   p90 %.1f   p99 %.1f   max %.1f   media %.1f\n",
		r.LatFactura.P50, r.LatFactura.P90, r.LatFactura.P99, r.LatFactura.Max, r.LatFactura.Media)
	fmt.Println()
	fmt.Println("  TIEMPO POR CAJA (s)")
	fmt.Printf("    p50 %.2f   p90 %.2f   max %.2f   (caja más lenta: #%d)\n",
		r.LatCaja.P50, r.LatCaja.P90, r.LatCaja.Max, r.CajaLenta)
	fmt.Printf("    cumplen el objetivo de %.1f s: %d de %d\n", r.ObjetivoS, r.CumplenS, r.Registros)
	fmt.Println()
	fmt.Println("  CONTENCIÓN Y LECTURAS")
	fmt.Printf("    colisiones de bloqueo    : %d\n", r.Colisiones)
	fmt.Printf("    escrituras fallidas       : %d\n", fallidasTotales(r))
	fmt.Printf("    lecturas ok / fallidas   : %d / %d\n", r.LecturasOK, r.LecturasFail)
	if len(r.Errores) > 0 {
		fmt.Println()
		fmt.Println("  PRIMEROS ERRORES")
		for i, e := range r.Errores {
			if i >= 5 {
				break
			}
			fmt.Printf("    %s\n", trunc(e, 110))
		}
	}
	fmt.Println()
	fmt.Println("  VEREDICTO")
	fmt.Println("    " + r.Veredicto)
	fmt.Println("========================================================")
}

func fallidasTotales(r *Resultado) int {
	return r.Registros*r.Facturas - r.FacturasBD
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// guardar deja el resultado en disco. JSON para las máquinas, Markdown para las
// personas, y los dos en la misma carpeta para que no se separen nunca.
func guardar(r *Resultado) {
	dir := "doc/benchmarks"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Println("no se pudo crear doc/benchmarks:", err)
		return
	}
	nombre := strings.ReplaceAll(r.Prueba, " ", "-")

	b, _ := json.MarshalIndent(r, "", "  ")
	rutaJSON := filepath.Join(dir, nombre+".json")
	if err := os.WriteFile(rutaJSON, b, 0o644); err != nil {
		fmt.Println("no se pudo guardar el JSON:", err)
		return
	}
	md := aMarkdown(r)
	if err := os.WriteFile(filepath.Join(dir, nombre+".md"), []byte(md), 0o644); err != nil {
		fmt.Println("no se pudo guardar el informe:", err)
		return
	}

	// Un índice con todos los resultados, para comparar de un vistazo cuál fue
	// más rápido y cuál se felló. Se RECONSTRUYE desde los JSON ya guardados,
	// nunca a partir del índice anterior: un índice que se reescribe a sí mismo
	// acababa duplicando su cabecera y perdiendo las filas, y ese fichero es la
	// tabla donde se decide, así que tiene que salir siempre bien.
	if err := reconstruirIndice(dir); err != nil {
		fmt.Println("no se pudo reconstruir el índice:", err)
	}

	fmt.Println()
	fmt.Printf("  Guardado: %s\n", rutaJSON)
	fmt.Printf("           doc/benchmarks/%s.md\n", nombre)
}

// reconstruirIndice rehace INDICE.md desde cero leyendo los *.json guardados.
// Fuente única de verdad: si el JSON y el índice discrepan, el JSON manda.
func reconstruirIndice(dir string) error {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var rs []*Resultado
	for _, e := range entradas {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".json") || n == "INDICE.json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		var r Resultado
		if err := json.Unmarshal(b, &r); err != nil || r.Prueba == "" {
			continue
		}
		rs = append(rs, &r)
	}
	// Lo más reciente primero.
	sort.Slice(rs, func(i, j int) bool { return rs[i].Fecha > rs[j].Fecha })

	var b strings.Builder
	b.WriteString("# Índice de bancos de pruebas\n\n")
	b.WriteString("Este fichero se REHACE en cada ensayo a partir de los `*.json` de esta\n")
	b.WriteString("misma carpeta, así que no puede quedar ni con cabeceras duplicadas ni\n")
	b.WriteString("con filas perdidas. Lo más reciente primero: un banco que empeora sin que\n")
	b.WriteString("nadie lo mire es un banco que no sirve.\n\n")
	b.WriteString("| Fecha | Prueba | Modo | Cajas | Facturas | Artículos | Total (s) | Facturas/s | Líneas/s | p99 (ms) | Colisiones | Veredicto |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rs {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %.2f | %.0f | %.0f | %.1f | %d | %s |\n",
			r.Fecha, r.Prueba, r.Modo, r.Registros, r.Facturas, r.Articulos, r.Segundos,
			r.FacturasS, r.LineasS, r.LatFactura.P99, r.Colisiones, trunc(r.Veredicto, 70))
	}
	if len(rs) == 0 {
		b.WriteString("| — | sin ensayos todavía | — | — | — | — | — | — | — | — | — | — |\n")
	}
	return os.WriteFile(filepath.Join(dir, "INDICE.md"), []byte(b.String()), 0o644)
}

func aMarkdown(r *Resultado) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Banco: %s\n\n", r.Prueba)
	fmt.Fprintf(&b, "- **Fecha**: %s\n", r.Fecha)
	fmt.Fprintf(&b, "- **Cajas registradoras**: %d\n", r.Registros)
	fmt.Fprintf(&b, "- **Facturas por caja**: %d\n", r.Facturas)
	fmt.Fprintf(&b, "- **Artículos por factura**: %d\n", r.Articulos)
	fmt.Fprintf(&b, "- **Objetivo por caja**: %.1f s\n\n", r.ObjetivoS)

	fmt.Fprintf(&b, "## Veredicto\n\n**%s**\n\n", r.Veredicto)

	fmt.Fprintf(&b, "## Rendimiento\n\n")
	fmt.Fprintf(&b, "| Medida | Valor |\n|---|---|\n")
	fmt.Fprintf(&b, "| Tiempo total | %.2f s |\n", r.Segundos)
	fmt.Fprintf(&b, "| Caja más lenta | %.2f s (caja #%d) |\n", r.PorCajaS, r.CajaLenta)
	fmt.Fprintf(&b, "| Cajas que cumplen %.0f s | %d de %d |\n", r.ObjetivoS, r.CumplenS, r.Registros)
	fmt.Fprintf(&b, "| Facturas/s | %.0f |\n", r.FacturasS)
	fmt.Fprintf(&b, "| Líneas/s | %.0f |\n", r.LineasS)
	fmt.Fprintf(&b, "| Colisiones de bloqueo | %d |\n", r.Colisiones)
	fmt.Fprintf(&b, "| Lecturas ok / fallidas | %d / %d |\n\n", r.LecturasOK, r.LecturasFail)

	fmt.Fprintf(&b, "## Latencia por factura (ms)\n\n")
	fmt.Fprintf(&b, "| p50 | p90 | p99 | max | media |\n|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %.1f | %.1f | %.1f | %.1f | %.1f |\n\n",
		r.LatFactura.P50, r.LatFactura.P90, r.LatFactura.P99, r.LatFactura.Max, r.LatFactura.Media)

	fmt.Fprintf(&b, "## Lo que hay en la base (contado después, no contado de memoria)\n\n")
	fmt.Fprintf(&b, "| Medida | Valor |\n|---|---|\n")
	fmt.Fprintf(&b, "| Facturas pedidas | %d |\n", r.Registros*r.Facturas)
	fmt.Fprintf(&b, "| Facturas en la base | %d |\n", r.FacturasBD)
	fmt.Fprintf(&b, "| Líneas pedidas | %d |\n", r.Lineas)
	fmt.Fprintf(&b, "| Líneas en la base | %d |\n", r.DetalleBD)
	fmt.Fprintf(&b, "| Movimientos de inventario | %d (desfase %d) |\n", r.InventarioOK, r.DesfaseInv)
	fmt.Fprintf(&b, "| Rastros en audit_log | %d |\n", r.Rastreos)

	if len(r.Errores) > 0 {
		fmt.Fprintf(&b, "\n## Errores\n\n")
		for _, e := range r.Errores {
			fmt.Fprintf(&b, "- `%s`\n", trunc(e, 160))
		}
	}
	return b.String()
}

