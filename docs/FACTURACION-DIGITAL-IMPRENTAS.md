# Facturación digital: imprentas integradas y cómo cambiar de una a otra

Documento de la integración de **CG Imprenta Digital** como segunda imprenta
digital del ERPGo, junto a **Unidigital**. Explica qué hace el sistema, qué
configura cada empresa y qué límites tienen los dos proveedores. La referencia
del API de CG está en `doc/FacturacionDigital/API Gateway CG [Calidad]
[v14].postman_collection.json` y en su doc-site
(`https://calidad.cgimprenta.digital/doc/`).

## 1. Las tres formas de facturar

Cada **empresa** elige una, y solo una:

| Modo | Cómo se representa | Quién emite |
| --- | --- | --- |
| Unidigital | `digital_invoice_config` con `addon_code='unidigital'` y `enabled=1` | Unidigital |
| CG Imprenta Digital | `addon_code='cgimprenta'` y `enabled=1` | CG |
| Impresora fiscal | ninguna fila con `enabled=1` | La impresora fiscal de la caja |

La exclusividad la garantizan los datos y la pantalla: **activar una imprenta
desactiva la otra** y la respuesta de la pantalla lo avisa. Desactivar **no
borra** la configuración: la imprenta queda lista para volver a activarse con
sus credenciales intactas.

### Cómo se cambia

En `Configuración → Facturación Digital`, cada imprenta tiene su tarjeta:

- **Guardar**: guarda su configuración (URL, credenciales, serie). No activa.
- **Probar conexión**: valida credenciales y conectividad con lo que hay en
  pantalla. En CG pide un token; en Unidigital consulta sus series.
- **Activar**: prueba la conexión **primero** y, solo si responde, activa esa
  imprenta y desactiva la otra. Si la prueba falla no se cambia nada.
- **Desactivar**: apaga esa imprenta y conserva su configuración.
- **Dejar la empresa solo con impresora fiscal**: apaga todas. Es la acción
  explícita para el tercer modo; no hace falta desmarcar casillas.

## 2. Qué se configura en cada imprenta

Unidigital:

| Campo | Obligatorio | Descripción |
| --- | --- | --- |
| URL base de la API | sí | Por defecto `https://api.unidigital.ve/v1` |
| Bearer Token | sí | Token que entrega Unidigital |
| Serie Strong ID | sí | Identificador de la serie en su plataforma |
| Sucursal ID | no | Identificador técnico de sucursal |

CG Imprenta Digital:

| Campo | Obligatorio | Descripción |
| --- | --- | --- |
| URL base de la API | sí | Por defecto el host de **calidad** (`https://calidad.cgimprenta.digital/`). CG entrega el de producción al habilitar la integración. |
| Usuario de API | sí | `userName` del API de CG |
| Contraseña de API | sí | `userPassword` del API de CG |
| Serie de numeración | sí | La serie asignada al negocio (por ejemplo `A`). CG **no publica** un catálogo de series: se escribe aquí y se valida en la primera emisión. |
| Sucursal | no | Si se escribe, CG la imprime bajo la dirección fiscal de la factura |

Los campos secretos (token y contraseña) **no se devuelven al navegador**: la
pantalla solo indica si ya hay uno guardado. Guardar con el campo vacío conserva
el valor anterior; para cambiarlo se escribe el nuevo.

## 3. Qué pasa al emitir una factura

1. La venta (panel, POS en línea o POS sincronizado) se guarda y se **encola**
   en `digital_invoice_log` (base de control). Si la empresa no tiene ninguna
   imprenta activa, **no se encola**: está en modo impresora fiscal.
2. El runner (cada 30 s) toma pendientes, monta el documento con los datos
   reales de la factura —cliente de su ficha, renglones con código y nombre del
   producto del catálogo, alícuota de IVA, descuento, moneda y tasa del día— y
   llama a la imprenta activa de **esa** empresa.
3. Con la respuesta se guardan `strong_id`, número de control (entero si la
   imprenta lo da así, y `nro_control_texto` con su serie), `pdf_url` y
   `xml_url`. Se registra también `addon_code` con la imprenta que emitió.
4. El PDF se archiva en el directorio de la empresa
   (`…/documentos/fiscales/factura-<número>-<fecha>.pdf`). El enlace del
   proveedor caduca; la copia del contribuyente no.
5. Si la empresa tiene activada la entrega automática, el correo al cliente sale
   con el PDF/XML del proveedor. El destinatario sale siempre de la ficha del
   cliente y el envío es idempotente.
6. Si la emisión falla, la fila sube `intentos` y guarda el mensaje del
   proveedor en `error_message`. A los 8 intentos queda `failed` y el botón
   **Reintentar rechazadas** de la pantalla la devuelve a la cola (útil cuando
   la causa era un dato del cliente o la tasa, y ya se corrigió).

## 4. Particularidades de CG Imprenta Digital

- **Autenticación:** `POST /api/Invoice/create_token_authenticator` con
  `userName`/`userPassword` devuelve un JWT que **vence a la hora**. El adaptador
  lo cachea por host+usuario, lo reutiliza entre facturas y lo renueva solo al
  vencer o cuando el API responde `401` (reintenta una vez).
- **Emisión:** `POST /api/Invoice/add_list_invoice` con `numeroSerie`,
  `cantidadFactura` y `facturas[]`. La respuesta trae `control_number`
  (`00-00000056`) y `invoice_pdf`; el `strong_id` que se guarda es el
  identificador del enlace público del PDF.
- **Moneda:** CG emite **solo en bolívares**. Una factura en USD/EUR se rechaza
  con un mensaje claro en vez de enviar dólares como si fueran bolívares.
- **Tasa:** `tasa_del_dia` y `fecha_tasa` son obligatorias. Se toman de la tasa
  registrada en la factura y, si no tiene, de la tasa BCV vigente para su fecha
  (`Configuración → Tasa BCV`). Sin tasa, la factura no se emite y el motivo
  queda en la cola.
- **Cliente:** CG exige RIF (letra `V/E/P/J/G/C` + 6 a 10 dígitos, sin guiones),
  nombre, correo, dirección y teléfono. Si a la ficha del cliente le falta
  alguno, la factura queda rechazada con ese motivo; se corrige en `Clientes` y
  se usa **Reintentar rechazadas**.
- **Renglones:** código de producto (máx. 13 caracteres, sin espacios), nombre,
  descripción, alícuota (`G` 16 %, `R` 8 %, `A` 31 %, `E` exento), cantidad y
  precio sin IVA. El IVA lo calcula CG por alícuota.
- **XML:** CG no devuelve XML; el enlace XML queda vacío y la pantalla no lo
  muestra.
- **Duplicados:** si el número de factura ya se emitió en esa imprenta, CG no lo
  reemite y lo informa; el ERP lo registra como documento duplicado.
- **Notas:** el adaptador implementa la nota de crédito total
  (`add_credit_note`) y la de débito (`add_debit_note`), pero **el ERP todavía no
  las encola** (no hay pantalla que las emita por esta vía). Lo que sí está
  soportado desde el ERP es la factura.

## 5. Cambios de esquema

`migrations/schema_z_zzzzzzzz_control_textual_cg.sql` agrega
`digital_invoice_log.nro_control_texto VARCHAR(32)` con su índice. Es
necesario porque CG devuelve el número de control con su serie
(`00-00000056`) y `nro_control` es `INT`. La columna entera se conserva para
Unidigital y el histórico.

Nada más cambia: `digital_invoice_config` sigue guardando una fila por imprenta
y empresa, con la configuración completa en `config_json`.

## 6. Cómo está probado

- `interfaces/web/digitalinvoice/cgimprenta/provider_test.go`: payload real,
  RIF sin guiones, tasa con cuatro decimales, alícuotas, descuento, reutilización
  y renovación del token, mensajes de rechazo del proveedor, duplicados y las
  validaciones previas (moneda, tasa, cliente, renglones, serie).
- `interfaces/web/digitalinvoice_processor_test.go`: que el documento lleve los
  renglones con las **dos** llaves con las que el sistema los guarda, que la tasa
  salga del registro del día, que no se encole sin imprenta activa, que un
  pendiente huérfano quede con su motivo y que la emisión real por CG guarde el
  control con su serie.
- `interfaces/web/digital_invoice_settings_test.go`: exclusividad entre
  imprentas, que desactivar una no apague la otra, el modo impresora fiscal, que
  guardar no cambie el modo, que los secretos guardados se conserven y que la
  pantalla se represente sin mostrar la contraseña.

## 7. Límites conocidos (dichos antes de que se descubran)

- **Las credenciales se guardan en `config_json` sin cifrar**, igual que antes
  con el token de Unidigital. Quien pueda leer la base de control puede leerlas.
  Conviene restringir el acceso a la base y a los respaldos.
- **La anulación no está cableada al ERP.** El contrato recibe solo el número de
  control entero y tanto CG como Unidigital necesitan además el número de
  documento; el adaptador de CG lo dice con un mensaje explícito en vez de
  fingir. La anulación se hace desde el panel de la imprenta.
- **Consulta por identificador:** CG no expone consulta por `strong_id`; sí
  permite listar documentos (que es lo que usa `UltimoDocumento`).
- **Unidigital** conserva sus métodos pendientes (consulta, anulación, último
  documento) tal como estaban: esta integración no los toca.
