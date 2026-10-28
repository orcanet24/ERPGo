package web

// retencion_numero.go — el número del comprobante de retención.
//
// POR QUÉ NADIE LO ESCRIBE EN PANTALLA.
//
// El número de un comprobante de retención no es una etiqueta: es el dato con
// el que el proveedor respalda su descuento en la declaración y el que el
// SENIAT cruza contra el TXT que sube la empresa. La Providencia SNAT/2026/00093
// le da forma —artículo 18: AAAAMM más un secuencial de ocho dígitos— y pide
// que sea consecutivo y no se repita.
//
// Mientras el número lo tecleaba quien registraba, no había ni forma ni
// consecutividad: cabía «RET-IVA-001» dos veces, y —peor— el campo `period` de
// la tabla, que es NOT NULL, ni siquiera se escribía, así que la retención no
// aparecía en ningún informe por período ni en el agrupado de cuentas por
// pagar. El defecto se veía lejos de donde estaba: la pantalla guardaba «OK» y
// el dato quedaba fuera de todos los reportes.
//
// AHORA el número lo reserva la base dentro de la MISMA transacción que guarda
// el comprobante, con el patrón ya usado por los correlativos de factura
// (fiscal.go): la fila del contador se bloquea en el UPDATE y el bloqueo dura
// hasta que la transacción termina, de modo que dos retenciones simultáneas no
// pueden quedarse con el mismo número. Y el piso —el mayor secuencial ya usado
// en el período— se calcula antes de reservar, para que una base con
// comprobantes heredados siga numerando por encima de los suyos.

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Series de comprobante de retención.
const (
	SerieRetencionIVA  = "RET-IVA"
	SerieRetencionISLR = "RET-ISLR"
)

// tablaComprobante traduce una serie a su tabla.
//
// Es una traducción, no una concatenación: el nombre de la tabla nunca sale de
// un formulario, y una serie desconocida se rechaza en vez de construir SQL con
// lo que venga.
func tablaComprobante(serie string) (string, error) {
	switch serie {
	case SerieRetencionIVA:
		return "vat_retention", nil
	case SerieRetencionISLR:
		return "islr_retention", nil
	}
	return "", fmt.Errorf("serie de comprobante desconocida: %q", serie)
}

// comprobanteDeRetencion reserva y devuelve el próximo número de comprobante
// para la serie en el período dado. Va dentro de la transacción que guarda.
func comprobanteDeRetencion(tx *sql.Tx, serie, periodo string) (string, error) {
	if len(periodo) != 6 {
		return "", fmt.Errorf("período de comprobante inválido: %q", periodo)
	}
	tabla, err := tablaComprobante(serie)
	if err != nil {
		return "", err
	}
	contador := "comprobante:" + serie + ":" + periodo

	piso, err := mayorSecuencial(tx, tabla, periodo)
	if err != nil {
		return "", fmt.Errorf("leyendo los comprobantes de %s: %w", serie, err)
	}
	if err := subirContador(tx, contador, piso); err != nil {
		return "", err
	}
	n, err := consumirContador(tx, contador)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%08d", periodo, n), nil
}

// mayorSecuencial devuelve el mayor secuencial ya usado en el período.
//
// Se recorre en Go y no con MAX(SUBSTR(...)) porque el sufijo es un número
// pegado al período y el cast distinto entre motores convertiría una consulta
// simple en dos dialectos. Solo se miran los comprobantes de ESTE período
// (prefijo exacto), que además acota mucho las filas.
func mayorSecuencial(tx *sql.Tx, tabla, periodo string) (int64, error) {
	rows, err := tx.Query(`SELECT retention_no FROM `+tabla+` WHERE retention_no LIKE ?`, periodo+"%")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var mayor int64
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			continue
		}
		// Solo el formato AAAAMM + 8 dígitos alimenta el piso. Un comprobante
		// heredado con otra forma («RET-IVA-001») no es un secuencial: no
		// puede bajar el piso ni subirlo por casualidad.
		if len(no) != 14 || !strings.HasPrefix(no, periodo) {
			continue
		}
		v, err := strconv.ParseInt(no[6:], 10, 64)
		if err == nil && v > mayor {
			mayor = v
		}
	}
	return mayor, rows.Err()
}

// subirContador pone el contador por encima del piso si venía por debajo.
//
// Es el mismo criterio que reservarConsecutivoTx: un contador que arranca en 0
// en un período donde ya hay comprobantes reemitiría números. El piso va en el
// propio UPDATE —comparado allí, no leído aquí— para que dos transacciones no
// puedan bajar lo que la otra acaba de subir.
func subirContador(tx *sql.Tx, nombre string, piso int64) error {
	if piso <= 0 {
		return nil
	}
	res, err := tx.Exec(`UPDATE pos_counters SET value = CASE WHEN value < ? THEN ? ELSE value END,
		updated_at = ? WHERE name = ?`, piso, piso, now(), nombre)
	if err != nil {
		return fmt.Errorf("poniendo al día el contador %s: %w", nombre, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	// El contador del período todavía no existe: se crea YA EN el piso. Si se
	// creara en 0 y el piso se aplicara después, el primer comprobante de un
	// período con herederos reemitiría un número que ya salió.
	if _, err := tx.Exec(`INSERT INTO pos_counters (name, value, updated_at) VALUES (?,?,?)`,
		nombre, piso, now()); err != nil {
		if !compraEsDuplicadoDeClave(err) {
			return fmt.Errorf("creando el contador %s: %w", nombre, err)
		}
		// Otra transacción lo creó a la vez: se sube con el mismo criterio.
		if _, err := tx.Exec(`UPDATE pos_counters SET value = CASE WHEN value < ? THEN ? ELSE value END,
			updated_at = ? WHERE name = ?`, piso, piso, now(), nombre); err != nil {
			return fmt.Errorf("poniendo al día el contador %s: %w", nombre, err)
		}
	}
	return nil
}

// consumirContador incrementa el contador y devuelve su valor nuevo.
//
// El UPDATE escribe primero y sin valor calculado en Go, así que toma el
// bloqueo de la fila y lo conserva hasta que la transacción termine; la lectura
// posterior ve lo que acabamos de escribir (misma transacción, misma conexión).
// Si la fila no existe todavía, se crea con valor 0: el primer consumo entrega 1.
func consumirContador(tx *sql.Tx, nombre string) (int64, error) {
	if _, err := tx.Exec(`UPDATE pos_counters SET value = value + 1, updated_at = ? WHERE name = ?`, now(), nombre); err != nil {
		return 0, fmt.Errorf("consumiendo el contador %s: %w", nombre, err)
	}

	var v int64
	err := tx.QueryRow(`SELECT value FROM pos_counters WHERE name = ?`, nombre).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		// Camino frío: período nuevo, la fila todavía no existe. Se crea con 0
		// y se vuelve a consumir. Si otra transacción la creó a la vez, el
		// INSERT choca —se da por bueno— y el UPDATE de encima entra igual.
		err = nil
		if _, e := tx.Exec(`INSERT INTO pos_counters (name, value, updated_at) VALUES (?, 0, ?)`, nombre, now()); e != nil && !compraEsDuplicadoDeClave(e) {
			return 0, fmt.Errorf("creando el contador %s: %w", nombre, e)
		}
		if _, e := tx.Exec(`UPDATE pos_counters SET value = value + 1, updated_at = ? WHERE name = ?`, now(), nombre); e != nil {
			return 0, fmt.Errorf("consumiendo el contador %s: %w", nombre, e)
		}
		err = tx.QueryRow(`SELECT value FROM pos_counters WHERE name = ?`, nombre).Scan(&v)
	}
	if err != nil {
		return 0, fmt.Errorf("leyendo el contador %s: %w", nombre, err)
	}
	return v, nil
}
