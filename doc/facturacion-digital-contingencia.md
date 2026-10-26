# Facturación digital y contingencia (Venezuela)

Documento técnico-normativo del 26 de octubre de 2026. Explica cómo trata
ERPGo/POSGo la venta con facturación digital activa y qué pasa cuando la caja
se queda sin red. **No es asesoría legal**: fija el criterio con el que está
escrito el código y las fuentes.

## 1. Norma aplicable

**Providencia Administrativa SNAT/2024/000102** (SENIAT), publicada en Gaceta
Oficial **43.032 del 19 de diciembre de 2024**. Regula la emisión de facturas y
otros documentos por medios digitales y derogó la SNAT/2014/0032.

Artículos que mandan en este diseño:

| Artículo | Qué establece |
|---|---|
| 7 | Datos que debe llevar una factura (denominación, numeración única y consecutiva, RIF del emisor, fecha y hora, datos del adquirente, descripción, base imponible, IVA, total). |
| 16 | Contingencia: falla de internet → aplicación móvil; falla del equipo → aplicación offline; falla eléctrica → formatos/talonarios de imprenta física autorizada, con el número del documento **precedido por la palabra «contingencia»**. |
| 18 | Trazabilidad y auditoría electrónica de emisión, modificación y anulación; consulta por el SENIAT por **10 años**. |
| 29 | La imprenta digital reporta al SENIAT dentro de los **10 días continuos** siguientes al cierre de cada mes. |
| 30 | El número de control lo asigna la imprenta digital y es **consecutivo y único por emisor**. |

**Lo que la norma NO dice** (verificado en el texto, no supuesto):

- No fija plazo en horas ni días para transmitir lo emitido en contingencia.
- No declara válido un ticket genérico de caja como factura.
- No crea una serie estandarizada tipo «C-001»: exige el prefijo «contingencia».
- No fija plazo de conservación del papel de contingencia (sí exige resguardarlo).

**Después de la contingencia el contribuyente debe:**

1. registrar los comprobantes físicos en su sistema;
2. resguardar el soporte físico;
3. **notificar por escrito** a la Gerencia Regional de Tributos Internos y a la
   imprenta digital que le asigna los números de control, con una **relación
   detallada** de los documentos.

> Vigencia: la transcripción consultada indica que la 102 rige hasta el
> 18 de abril de 2027 y menciona una Providencia 2026/00093 posterior. Eso no se
> pudo confirmar en Gaceta accesible: antes de una revisión, confirmar la norma
> vigente.

## 2. Los tres modos de operación del sistema

| Modo | Qué pasa | Qué emite el sistema |
|---|---|---|
| **En línea** | La caja habla con el ERP y hay proveedor digital activo. | Factura fiscal con número de control del proveedor. Se guarda en `invoice` y se transmite en el acto. |
| **Degradado** (ERP sí, imprenta caída) | La venta se registra y la cola reintenta sola cada 30 s. | La caja entrega su ticket; la factura digital se emite al restablecerse el proveedor. La fila queda `pending` en `digital_invoice_log`. |
| **Sin red** (contingencia, art. 16) | La caja vende con su base local y su propio correlativo. | Documento de **contingencia**: serie `CONT`, referencia `CONT-<terminal>-<correlativo>`. Al sincronizar se registra y entra a la misma cola de emisión digital. |

Regla de oro del código: **nunca se inventa un número de control fiscal**. Antes
la sincronización offline guardaba `control_no = "POS-<n>"`, que no es control
fiscal (lo asigna la imprenta, art. 30), se repetía en cada caja y no existía en
ningún libro del SENIAT. Hoy esos documentos se identifican como contingencia y
su control real queda en `digital_invoice_log.nro_control` cuando el proveedor
emite.

## 3. Qué debe imprimir el ticket de la caja (POSGo)

Mientras el documento es de contingencia, el ticket **no sustituye la factura**;
es el comprobante que se lleva el cliente y la prueba de que la venta existió.
Debe llevar, como mínimo, los datos del art. 7 que apliquen:

- La leyenda **«DOCUMENTO DE CONTINGENCIA»** y el número precedido por
  «contingencia» (ej.: `contingencia CONT-T01-000123`).
- Razón social y RIF del emisor, y número de la caja/terminal.
- Fecha y hora de la operación.
- Nombre y RIF/cédula del cliente (o «Consumidor final» si no lo dio).
- Descripción de cada renglón, cantidad, precio unitario e importe.
- Base imponible, IVA por alícuota y total.
- Leyenda: «Este documento no es una factura fiscal. La factura digital será
  emitida y enviada al restablecerse la conexión».

Los datos fiscales del emisor salen de `company_information`
(`company_name`, `vat_no`); la caja los recibe al sincronizar catálogo.

## 4. Dónde verlo en el sistema

- **Facturación Digital → Documentos de contingencia**: lista de lo emitido sin
  red con su estado (Pendiente / Transmitida / Rechazada), el control fiscal
  asignado y el botón **Descargar relación (CSV)**.
- El CSV es el anexo de la notificación escrita: factura, fecha, referencia de
  contingencia, RIF, base, IVA, total, estado y número de control fiscal.

## 5. Lo que queda en manos del contribuyente (no automatizable)

1. Enviar la notificación escrita a la Gerencia Regional y a la imprenta digital
   con la relación descargada.
2. Conservar los tickets físicos de contingencia.
3. Revisar las filas **Rechazadas**: si el proveedor devuelve error, la factura
   no existe para el SENIAT aunque la venta esté cobrada.

## Fuentes

- Texto de la Providencia SNAT/2024/000102 (transcripción consultable):
  <https://docs.cachicamo.app/fiscal/providencia000102>
- Datos de publicación en Gaceta 43.032 y derogatoria de la 2014/0032:
  <https://iusdata.com/seniat-texto-medios-digitales-facturas-otros-documentos/>
- Requisitos de notas de crédito/débito (remisión a SNAT/2011/0071, arts. 22-23):
  <https://cachicamo.app/blog/anular-factura-vs-nota-credito/>
