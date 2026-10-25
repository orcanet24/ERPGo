package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"
)

// VIGILANTE DEL WAL — la red de seguridad que evita que el disco se llene.
//
// # QUÉ FALLA, Y POR QUÉ EXISTE ESTE CÓDIGO
//
// En modo WAL SQLite no escribe en el fichero de la base: escribe en un log
// aparte, el `-wal`. Cada 1000 páginas (unos 4 MB) lo pliega de vuelta dentro
// del fichero principal y lo vacía. Eso es el checkpoint automático, y es lo
// que mantiene el log en unos pocos megabytes.
//
// Para vaciarlo tiene que saber que NADIE está leyendo una versión antigua de
// los datos. Si hay un lector a medias de un SELECT largo, no puede saberlo,
// así que no vacía y sigue añadiendo. El log no tiene tope: solo para cuando
// ese lector acaba.
//
// Medido en este repositorio (doc/HOJA-DE-RUTA.md, sección 5-cuad-bis), mismo
// trabajo exacto y lo único que cambia son 8 lectores:
//
//	sin lectores:  4,5 MB -> 5,3 MB   (plano)
//	con lectores:  0,8 MB -> 179 MB   (crece sin parar)
//
// Lo grave del fallo no es el rendimiento, es que NO DA NINGÚN ERROR. La
// aplicación guarda todo, responde a todos y parece perfecta. Lo único que
// aparece es un fichero enorme. El día que se llena el disco, una caja dice
// "no se pudo guardar" sin poder decir por qué, y el negocio para sin aviso.
//
// # LA LECCIÓN QUE HAY EN ESTE FICHERO, Y QUE COSTÓ ENCONTRARLA
//
// Se intentaron cuatro veces. Las tres primeras fallaron, y el motivo no fue
// SQLite: fue la forma en la que se le pidió el trabajo.
//
//  1. `PASSIVE` — no bloquea a nadie, pero NO vacía el fichero. Copia las
//     páginas dentro de la base y deja el log igual de grande. Acota el
//     trabajo, no el disco, y el disco es lo que importa.
//  2. `TRUNCATE` con `context.WithTimeout` — VACÍA el fichero, pero al
//     cancelar el checkpoint a medias le deja el candado puesto al escritor:
//     `SQLITE_BUSY` en la caja que estaba cobrando. Un vigilante que rompe la
//     venta es peor que no tener vigilante.
//     3 y 4. TRUNCATE con disparadores por umbral y por proporción — bajaron de
//     185 MB a 51 MB, pero se quedaban ahí sin recuperar del todo.
//
// Y AHÍ ESTÁ LO QUE FALTABA: **la cancelación era el problema, no TRUNCATE.**
// SQLite ya sabe cuánto esperar, y tiene su propio mecanismo: `busy_timeout`.
// Cuando se le acaba, devuelve `SQLITE_BUSY` y deja todo consistente. En
// cambio, un `context.WithTimeout` que corta la llamada a medias interrumpe un
// trabajo que estaba a medio hacer y no garantiza nada.
//
// Por eso aquí el único límite es SQLite mismo: se le pone `busy_timeout` y se
// le deja trabajar. El contexto existe SOLO para conseguir la conexión, con un
// plazo holgado que jamás llega a dispararse sobre el checkpoint.
const (
	// Cada cuánto se intenta vaciar el log.
	intervaloWAL = 15 * time.Second

	// Cuánto espera SQLite antes de desistir. Este es el límite REAL, y lo
	// pone SQLite, no un context. Si no puede vaciar porque hay un lector,
	// devuelve SQLITE_BUSY limpio y a la siguiente vuelta se vuelve a
	// intentar. Nunca deja nada a medias.
	//
	// 2 s es deliberadamente corto: es tiempo de sobra para vaciar un log de
	// cientos de megabytes, y corto como para que un lector que acabe enseguida
	// no quede siempre fuera.
	busyTimeoutWAL = 2 * time.Second

	// Plazo para CONSEGUIR la conexión del pool, no para el checkpoint. Va
	// holgado y por encima de busyTimeoutWAL para que jamás dispare: si
	// disparara, sería otra cancelación a medias, que es justo el error que
	// se está corrigiendo.
	plazoConexionWAL = 30 * time.Second

	// Desde este tamaño merece la pena pagar el coste del TRUNCATE. Por
	// debajo, el auto-checkpoint de SQLite ya lo lleva bien y además este
	// modo sí espera a los lectores, que es coste en la caja.
	umbralWAL = 8 << 20 // 8 MB

	// Aviso para el operador, por encima del umbral normal.
	umbralAvisoWAL = 512 << 20 // 512 MB

	// Cada cuánto se recuerda en el log que el vigilante está vivo. Un
	// vigilante callado durante un mes es indistinguible de uno que no existe,
	// y no se confía en un mecanismo que no se ve funcionar.
	intervaloInformeWAL = 30 * time.Minute
)

// VigilarWAL mantiene el WAL acotado en el tiempo y devuelve la función que
// lo para. Se llama al abrir la base y el resultado se guarda para llamarlo al
// cerrar.
//
// No devuelve error: un vigilante que se cae es peor que uno que avisa, así
// que cualquier fallo se registra y la vuelta siguiente se reintenta. Para
// cuando no importa —base cerrándose—, lo único que se deja de hacer es
// vaciar el log, que es exactamente lo que pasa también si no se llama a la
// función devuelta.
func VigilarWAL(db *sql.DB, etiqueta string) func() {
	return vigilarWAL(db, etiqueta, intervaloWAL, umbralWAL)
}

// vigilarWAL es el vigilante con sus parámetros como argumento, para que los
// tests no tengan que esperar 15 s por vuelta. En producción solo se llama
// desde VigilarWAL.
func vigilarWAL(db *sql.DB, etiqueta string, intervalo time.Duration, umbral int64) func() {
	// Dos canales y no uno: `orden` es la ORDEN de parar y la cierra quien
	// para; `hecho` confirma que la goroutine se fue. Con un solo canal había
	// dos sitios cerrando el mismo —la goroutine al salir y el que para— y el
	// segundo cierre hacía PANIC. Que lo encontrara el test y no producción
	// es exactamente para lo que está.
	orden := make(chan struct{})
	hecho := make(chan struct{})

	var parar sync.Once

	go func() {
		defer close(hecho)
		t := time.NewTicker(intervalo)
		defer t.Stop()
		ultimoInforme := time.Now()

		for {
			select {
			case <-orden:
				return
			case <-t.C:
			}

			tam := tamanoWAL(db)
			if tam == 0 {
				continue // no hay log: nada que hacer
			}

			// Por debajo del umbral no se toca: el auto-checkpoint de
			// SQLite ya lo mantiene pequeño, y este modo sí espera a los
			// lectores, que es coste directo sobre la caja que cobra.
			if tam < umbral {
				continue
			}

			restante, err := ForzarCheckpointWAL(db)
			if err != nil {
				// SQLITE_BUSY no es un error del sistema: es "hoy no,
				// hoy hay un lector". Se anota en silencio salvo en el
				// informe periódico, porque pasaría a ser ruido y el ruido
				// es como no avisar.
				fmt.Printf("  WAL: %s sigue en %.0f MB y no se pudo vaciar (lector abierto): %v\n",
					etiqueta, float64(tam)/(1<<20), err)
			} else if restante > 0 {
				fmt.Printf("  WAL: %s quedó en %d páginas sin vaciar (lector abierto)\n",
					etiqueta, restante)
			}

			if tam >= umbralAvisoWAL {
				fmt.Printf("  AVISO: el WAL de %s llega a %.0f MB.\n"+
					"  Hay una lectura abierta que impide vaciar el log.\n"+
					"  Revise el espacio en disco antes de que una caja no pueda guardar.\n",
					etiqueta, float64(tam)/(1<<20))
			}

			if time.Since(ultimoInforme) >= intervaloInformeWAL {
				ultimoInforme = time.Now()
				fmt.Printf("  WAL: %s a %.1f MB tras %s de vigilancia\n",
					etiqueta, float64(tamanoWAL(db))/(1<<20), etiqueta)
			}
		}
	}()

	return func() {
		parar.Do(func() { close(orden) })
		<-hecho
	}
}

// ForzarCheckpointWAL vacía el fichero -wal. Devuelve las páginas que
// quedaron sin vaciar y el error, si lo hubo.
//
// ESTA ES LA FUNCIÓN QUE NO SE ESCRIBIÓ BIEN LAS TRES PRIMERAS VECES.
//
// Lo que la diferencia de las versiones anteriores es una sola cosa, y es la
// que importa: **no hay ningún `context.WithTimeout` sobre el checkpoint.** La
// espera la pone SQLite con `busy_timeout`, y SQLite sabe terminar de forma
// consistente cuando se le acaba. Un contexto cancelando a medias interrumpe
// un trabajo a medio hacer.
//
// El `busy_timeout` se fija en la conexión que se va a usar para esto, no en
// el DSN, porque así el límite es de ESTE checkpoint y no de todo lo demás que
// pase por esa conexión.
//
// No se usa un canal de cancelación ni una goroutine que lo mate: si el
// checkpoint tarda más de `busyTimeoutWAL`, SQLite devuelve el error y se
// acabó. Sin atasco posible.
func ForzarCheckpointWAL(db *sql.DB) (queda int64, err error) {
	if db == nil {
		return 0, nil
	}

	// Plazo SOLO para conseguir la conexión, holgado por encima del
	// busy_timeout. Si este plazo llegara a dispararse sería otra
	// cancelación a medias, y ese es justo el error corregido.
	ctx, cancel := context.WithTimeout(context.Background(), plazoConexionWAL)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		return tamanoWAL(db), err
	}
	defer conn.Close()

	// El único límite real de esta operación. Lo pone SQLite.
	if _, err := conn.ExecContext(ctx,
		fmt.Sprintf("PRAGMA busy_timeout=%d", int(busyTimeoutWAL/time.Millisecond))); err != nil {
		return tamanoWAL(db), err
	}

	var ocupado, enLog, plegados int64
	if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&ocupado, &enLog, &plegados); err != nil {
		return tamanoWAL(db), err
	}
	return enLog, nil
}

// tamanoWAL devuelve el tamaño en bytes del fichero -wal de esta base.
//
// Se usa os.Stat y no una consulta SQL porque la pregunta es "cuánto disco se
// está comiendo", y para eso la fuente es el disco. Una consulta a la base
// para averiguarlo sería además una lectura más, y puede ser justo la que
// impide el vaciado.
func tamanoWAL(db *sql.DB) int64 {
	ruta, err := rutaFichero(db)
	if err != nil {
		return 0
	}
	info, err := os.Stat(ruta + "-wal")
	if err != nil {
		return 0
	}
	return info.Size()
}

// rutaFichero devuelve la ruta de la base o vacío si no la hay.
func rutaFichero(db *sql.DB) (string, error) {
	if db == nil {
		return "", fmt.Errorf("sin base")
	}
	// database/sql no expone la ruta. Se le pregunta al propio motor con
	// database_list, que es la respuesta literal: adivinar de la primera
	// fila de sqlite_master no vale porque guarda el nombre tal como se
	// abrió y puede ser relativo.
	var seq int
	var name, path string
	if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("la base no está en un fichero (es :memory:)")
	}
	return path, nil
}
