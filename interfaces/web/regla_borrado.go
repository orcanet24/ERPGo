package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"erpgo/infrastructure/persistence"
)

// regla_borrado.go — la regla de no borrado.
//
// Un registro del que ya colgó un documento NO se borra. El motivo
// no es estético. Los libros fiscales (ventas, compras, kárdex, retenciones) y
// la cadena de auditoría se construyen por consulta, así que un registro borrado
// no deja un hueco: deja un documento apuntando a un nombre que ya no existe. La
// factura sigue ahí —con su número, su IVA y su asiento— y el cliente al que se
// le emitió, no. El libro sale con un renglón sin RIF y la CxC sin deudor.
//
// POR QUÉ LA DECISIÓN VIVE EN UNA FUNCIÓN PURA Y NO SOLO EN LOS HANDLERS.
//
// Qué cuenta como movimiento y cómo se rechaza es la parte que puede fallar en
// silencio, y probarla contra la base real exigiría un MySQL en cada máquina que
// corra `go test` —cosa que la suite del proyecto no exige: sin MySQL los tests
// se SALTAN (ver infrastructure/persistence/testdb.go)—. Separada, la política
// se prueba sola y cada handler solo aporta su lista de referencias.
//
// LO QUE ESTA REGLA NO HACE: no impide un borrado hecho por fuera de la
// aplicación —SQLite trae las claves foráneas apagadas y el proyecto no ejecuta
// ningún PRAGMA que las active, así que ninguna base impide un huérfano— ni
// vuelve inmutable lo ya grabado. Es la puerta de la aplicación, que es la que
// el usuario usa.

// referencia es la cuenta de documentos que apuntan a un registro.
type referencia struct {
	Que     string // en plural y en minúscula: "facturas", "cobros registrados"
	Cuantos int
}

// consultaReferencia es una referencia por contar: qué es y la consulta que la
// cuenta. La consulta recibe UN solo parámetro, el id del registro.
type consultaReferencia struct {
	Que string
	SQL string
}

// revisarBorrado decide si un registro se puede borrar.
//
// Devuelve el motivo del rechazo y si está bloqueado. `entidad` va con artículo
// ("el cliente", "la compra") porque entra tal cual en el mensaje; `consejo` es
// qué hacer en su lugar, y vacío significa no decir nada.
func revisarBorrado(entidad, consejo string, refs []referencia) (string, bool) {
	var bloqueos []string
	for _, ref := range refs {
		if ref.Cuantos > 0 {
			bloqueos = append(bloqueos, fmt.Sprintf("%d %s", ref.Cuantos, ref.Que))
		}
	}
	if len(bloqueos) == 0 {
		return "", false
	}
	msg := fmt.Sprintf("No se puede eliminar %s: tiene %s.", entidad, enumerar(bloqueos))
	if consejo != "" {
		msg += " " + consejo
	}
	return msg, true
}

// enumerar une una lista en español: "a", "a y b", "a, b y c".
//
// Existe porque el mensaje lo lee una persona que está decidiendo qué hacer, y
// "3 facturas, 1 cotización" con las comas en su sitio se lee; el volcado de un
// slice, no.
func enumerar(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " y " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " y " + items[len(items)-1]
}

// contarReferencias cuenta cuántos documentos apuntan al registro, uno por
// consulta.
//
// SI UNA CONSULTA FALLA, DEVUELVE ERROR —NO CERO—, y el llamante bloquea. Es
// deliberado: contar 0 y seguir convierte una tabla que no está (una base vieja,
// una migración que no corrió) en un borrado permitido en silencio, que es
// justamente el daño que esta regla existe para impedir. Un bloqueo de más se ve
// en pantalla, con su motivo, y se arregla; un huérfano en el libro de compras
// no se ve nunca.
func (s *Server) contarReferencias(r *http.Request, id string, consultas []consultaReferencia) ([]referencia, error) {
	refs := make([]referencia, 0, len(consultas))
	for _, c := range consultas {
		var n int
		if err := s.dbFor(r).QueryRow(c.SQL, id).Scan(&n); err != nil {
			return nil, fmt.Errorf("%s: %w", c.Que, err)
		}
		refs = append(refs, referencia{Que: c.Que, Cuantos: n})
	}
	return refs, nil
}

// mensajeNoComprobado es lo que se responde cuando el conteo no se pudo hacer.
func mensajeNoComprobado(entidad string, err error) string {
	return fmt.Sprintf("No se pudo comprobar si %s tiene movimiento (%v). No se eliminó nada por precaución; "+
		"revise el registro de migraciones de la base antes de reintentar.", entidad, err)
}

// guardarBorrado es lo único que un handler necesita llamar: devuelve el mensaje
// de rechazo, o "" si el borrado puede seguir.
func (s *Server) guardarBorrado(r *http.Request, id, entidad, consejo string, consultas []consultaReferencia) string {
	refs, err := s.contarReferencias(r, id, consultas)
	if err != nil {
		return mensajeNoComprobado(entidad, err)
	}
	msg, bloqueado := revisarBorrado(entidad, consejo, refs)
	if !bloqueado {
		return ""
	}
	return msg
}

// mensajeDeIntegridadParaPantalla dice si un mensaje de error es, en realidad, un
// impedimento de integridad de la base, y lo devuelve limpio en ese caso.
//
// Va aquí y no en el handler porque el handler solo tiene el TEXTO del error
// (jsonError recibe una cadena). El conocimiento del dialecto de cada motor vive
// en la capa de persistencia, que es la que habla con la base.
func mensajeDeIntegridadParaPantalla(msg string) (string, bool) {
	limpio := persistence.MensajeDeIntegridad(errors.New(msg))
	if limpio == "" {
		return msg, false
	}
	return limpio, true
}

// ---------- QUÉ BLOQUEA A CADA REGISTRO ----------

// Una compra registrada no es una fila: la mercancía entró al almacén, el asiento
// se publicó, el IVA quedó en el libro de compras y el número de control en la
// retención. Borrar la fila no deshace nada de eso; solo deja el asiento y el
// kárdex apuntando a una compra que ya no está.
//
// OJO CON LAS DOS PRIMERAS CONSULTAS. Buscan por `referenceNo`, igual que el
// borrado al que sustituyen, y ahí va el NÚMERO DEL DOCUMENTO sin el tipo
// dentro: si una factura y una compra comparten número, el conteo puede sumar
// asientos de la otra. Bloquea de más, que es el lado seguro del error; la
// corrección de fondo —etiquetar el referenceNo con su tipo— es otra historia y
// no la resuelve esta regla.
var referenciasCompra = []consultaReferencia{
	{"asientos contables", `SELECT COUNT(*) FROM acc_vaucher WHERE referenceNo = ?`},
	{"asientos publicados", `SELECT COUNT(*) FROM acc_transaction WHERE referenceNo = ?`},
	{"renglones de mercancía", `SELECT COUNT(*) FROM product_purchase_details WHERE purchase_id = ?`},
	{"cuentas por pagar", `SELECT COUNT(*) FROM accounts_payable WHERE purchase_id = ?`},
	{"retenciones de IVA", `SELECT COUNT(*) FROM vat_retention WHERE purchase_id = ?`},
}

const consejoCompra = "En lugar de eliminarla, registre el reverso —ajuste de inventario y asiento— para que el libro de compras siga cuadrando."

// Cuenta toda factura, también las anuladas: una factura anulada sigue existiendo
// —es parte del libro de ventas y de la cadena de auditoría— y su cliente tiene
// que seguir teniendo nombre y RIF.
//
// EL CONSEJO NO DICE «MÁRQUELO INACTIVO» A PROPÓSITO: `customer_information`
// tiene `status`, pero su comentario en el esquema es `1=paid, 2=credit` —es la
// forma de pago, no un inactivo— y no hay pantalla que inactive un cliente.
// Mandar a hacer algo que no se puede hacer es peor que no decir nada.
var referenciasCliente = []consultaReferencia{
	{"facturas", `SELECT COUNT(*) FROM invoice WHERE customer_id = ?`},
	{"cobros registrados", `SELECT COUNT(*) FROM debt_collection WHERE customer_id = ?`},
	{"cotizaciones", `SELECT COUNT(*) FROM quotation WHERE customer_id = ?`},
	{"órdenes de entrega", `SELECT COUNT(*) FROM delivery_order WHERE customer_id = ?`},
	// El asiento de una venta a crédito o de un cobro lleva subType 3 y el
	// subCode que `subcodeID` resuelve para el cliente (accounting.go). Sin esta
	// consulta, un cliente sin facturas pero con contabilidad —un saldo inicial,
	// por ejemplo— se podría borrar dejando el mayor sin contrapartida.
	{"movimientos contables", `SELECT COUNT(*) FROM acc_transaction t JOIN acc_subcode s ON s.id = t.subCode WHERE t.subType = 3 AND s.subTypeId = 3 AND s.referenceNo = ?`},
}

const consejoCliente = "Conserve el registro y corrija el nombre o el RIF si están mal: las facturas ya emitidas, su cuenta por cobrar y el libro de ventas dependen de ellos."

var referenciasProveedor = []consultaReferencia{
	{"compras", `SELECT COUNT(*) FROM product_purchase WHERE supplier_id = ?`},
	{"cuentas por pagar", `SELECT COUNT(*) FROM accounts_payable WHERE supplier_id = ?`},
	{"retenciones de IVA", `SELECT COUNT(*) FROM vat_retention WHERE supplier_id = ?`},
	{"retenciones de ISLR", `SELECT COUNT(*) FROM islr_retention WHERE supplier_id = ?`},
	{"movimientos contables", `SELECT COUNT(*) FROM acc_transaction t JOIN acc_subcode s ON s.id = t.subCode WHERE t.subType = 4 AND s.subTypeId = 4 AND s.referenceNo = ?`},
}

const consejoProveedor = "Conserve el registro y corrija el nombre o el RIF si están mal: sus compras, sus retenciones y el libro de compras dependen de ellos."

