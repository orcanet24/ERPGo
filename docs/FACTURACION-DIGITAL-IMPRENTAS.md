# Facturación digital: imprentas integradas y cómo cambiar de una a otra

Documento de las **tres** imprentas digitales integradas en el ERPGo:
**Unidigital**, **CG Imprenta Digital** y **The Factory HKA (TFHKA)**. Explica
qué hace el sistema, qué configura cada empresa y qué límites tiene cada
proveedor.

Las referencias de los proveedores (colección Postman de CG y referencia técnica
IT-21-01-01 v2 de THKA con sus ejemplos JSON) se reciben del proveedor y **no se
versionan en el repositorio**: `doc/FacturacionDigital/` está ignorado por git.
Este documento resume lo que el ERP necesita de ellas.

## 1. Las cuatro formas de facturar

Cada **empresa** elige una, y solo una:

| Modo | Cómo se representa | Quién emite |
| --- | --- | --- |
| Unidigital | `digital_invoice_config` con `addon_code='unidigital'` y `enabled=1` | Unidigital |
| CG Imprenta Digital | `addon_code='cgimprenta'` y `enabled=1` | CG |
| The Factory HKA | `addon_code='thka'` y `enabled=1` | TFHKA |
| Impresora fiscal | ninguna fila con `enabled=1` | La impresora fiscal de la caja |

La exclusividad la garantizan los datos y la pantalla: **activar una imprenta
desactiva la otra** y la respuesta de la pantalla lo avisa. Desactivar **no
borra** la configuración: la imprenta queda lista para volver a activarse con
sus credenciales intactas.

### Cómo se cambia

En `Configuración → Facturación Digital` las imprentas se presentan como un
**selector de pestañas**: cada una con su nombre y su estado (`ACTIVA`,
`CONFIGURADA` o `SIN CONFIGURAR`), y al elegirla se abre su panel. La que está
emitiendo abre seleccionada; el panel se recuerda en la URL (`#di-thka`) para
que al guardar no salte a otra.

Dentro del panel de cada imprenta:

- **Guardar**: guarda su configuración (URL, credenciales, serie). No activa.
- **Probar conexión**: valida credenciales y conectividad con lo que hay en
  pantalla. En Unidigital consulta sus series; en CG y en THKA pide un token.
- **Activar**: prueba la conexión **primero** y, solo si responde, activa esa
  imprenta y desactiva la otra. Si la prueba falla no se cambia nada.
- **Desactivar**: apaga esa imprenta y conserva su configuración.
- **Dejar la empresa solo con impresora fiscal**: apaga todas. Es la acción
  explícita para el cuarto modo; no hace falta desmarcar casillas.

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

The Factory HKA:

| Campo | Obligatorio | Descripción |
| --- | --- | --- |
| URL base de la API | sí | Por defecto el host de demostración (`https://demoemisionv2.thefactoryhka.com.ve/`). Producción es `https://emisionv2.thefactoryhka.com.ve/`. |
| Usuario de API | sí | `usuario` del `POST /api/Autenticacion` |
| Clave de API | sí | `clave` del `POST /api/Autenticacion` |
| Serie | no | Hasta 20 caracteres. THKA permite **no usar series**: dejarla vacía es válido y se envía `Serie: ""`. |
| Sucursal | no | Código de hasta 6 caracteres que THKA imprime en el documento |

Los campos secretos (token, contraseña y clave) **no se devuelven al
navegador**: la pantalla solo indica si ya hay uno guardado. Guardar con el campo
vacío conserva el valor anterior; para cambiarlo se escribe el nuevo.

## 3. Qué pasa al emitir una factura

1. La venta se guarda y se **encola** en `digital_invoice_log` (base de control).
   Da igual por dónde entre: factura del panel, checkout del POS en línea, cola
   de sincronización del POS offline o venta que llega por **gRPC** desde la
   caja. Las tres puertas del POS pasan por el mismo motor de registro
   (`registrarVentaTerminal`), que encola antes de responder "sincronizada".
   Si la empresa no tiene ninguna imprenta activa, **no se encola**: está en
   modo impresora fiscal.
2. El runner (cada 30 s) toma pendientes, monta el documento con los datos
   reales de la factura —cliente de su ficha, renglones con código y nombre del
   producto del catálogo, alícuota de IVA, descuento, si el producto es bien o
   servicio, moneda, tasa del día y si la venta es de contado o a crédito— y
   llama a la imprenta activa de **esa** empresa.
3. Con la respuesta se guardan `strong_id`, número de control (entero si la
   imprenta lo da así, y `nro_control_texto` con su serie), `pdf_url` y
   `xml_url`. Se registra también `addon_code` con la imprenta que emitió.
4. El PDF se archiva en el directorio de la empresa
   (`…/documentos/fiscales/factura-<número>-<fecha>.pdf`) cuando la imprenta lo
   entrega. El enlace del proveedor caduca; la copia del contribuyente no.
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
  las encola** (no hay pantalla que las emita por esta vía).

## 5. Particularidades de The Factory HKA

- **Autenticación:** `POST /api/Autenticacion` con `usuario` y `clave` devuelve
  un JWT que dura **doce horas**. Se cachea por host+usuario y se renueva al
  vencer o ante un `401` (con un reintento). La respuesta del token no está
  documentada con precisión, así que el adaptador acepta las variantes
  razonables: `token`, `jwt`, `accessToken`, `resultado` o el JWT suelto.
- **Emisión:** `POST /api/Emision` con el sobre
  `{"Documentoelectronico": {Encabezado, DetallesItems}}`. La cabecera lleva
  `IdentificacionDocumento` (tipo, número, fecha `DD/MM/AAAA`, hora de 12 horas,
  serie, sucursal, tipo de venta, moneda, tipo de pago), `Comprador` (letra y
  número del RIF por separado, razón social, dirección, país, teléfono y correo
  como listas) y `Totales` (ítems, gravado, exento, subtotal, IVA, total con
  IVA, total a pagar e `ImpuestosSubtotal`).
- **La respuesta no se juzga por el HTTP:** THKA contesta `200` con un cuerpo
  `{codigo, mensaje, validaciones[], resultado}`. `codigo = "200"` es éxito;
  `"201"` es documento duplicado y `"203"/"205"` son rechazos por validación.
  El adaptador exige el código y un `numeroControl`: leer solo el HTTP daría por
  emitida una factura que no lo está.
- **Número de control:** lo asigna THKA en la emisión y llega en
  `resultado.numeroControl`. Se guarda completo en `nro_control_texto`.
- **PDF:** la emisión **no** devuelve el PDF. El adaptador pide
  `POST /api/DescargaArchivo` y acepta un PDF binario o un JSON con el archivo
  en base64 o un enlace; si la respuesta no es ninguna de esas formas, la
  emisión se da por buena igual y el documento queda con su número de control
  (la referencia no describe la respuesta de ese endpoint). El XML no existe en
  THKA.
- **Moneda:** se emite en bolívares (`Moneda: "VES"`). Una factura en divisas se
  rechaza antes de llamar, igual que con CG.
- **Bien o servicio:** THKA exige `IndicadorBienoServicio` (1 bien, 2 servicio) y
  una unidad de medida UNECE. El ERP los deriva del catálogo de productos
  (`item_type`): bienes con `NIU`, servicios con `ZZ`.
- **IGTF:** si la factura del ERP lleva IGTF, se declara como impuesto subtotal
  (`CodigoTotalImp: "IGTF"`, alícuota 3 %) y entra en `TotalAPagar`, que es la
  fórmula de la referencia.
- **Notas:** la nota de crédito/débito usa el mismo `/api/Emision` con tipo `02`
  o `03` y exige el número, la **fecha** y el **monto** de la factura afectada
  (el contrato `DatosDocumento` tiene esos campos para cuando el ERP emita notas;
  la factura normal los deja vacíos).

## 6. Cambios de esquema

`migrations/schema_z_zzzzzzzz_control_textual_cg.sql` agrega
`digital_invoice_log.nro_control_texto VARCHAR(32)` con su índice. Es
necesario porque CG y THKA devuelven el número de control con su serie
(`00-00000056`) y `nro_control` es `INT`. La columna entera se conserva para
Unidigital y el histórico.

Nada más cambia: `digital_invoice_config` sigue guardando una fila por imprenta
y empresa, con la configuración completa en `config_json`.

## 7. Cómo está probado

- `interfaces/web/digitalinvoice/cgimprenta/provider_test.go`: payload real,
  RIF sin guiones, tasa con cuatro decimales, alícuotas, descuento, reutilización
  y renovación del token, mensajes de rechazo del proveedor, duplicados y las
  validaciones previas (moneda, tasa, cliente, renglones, serie).
- `interfaces/web/digitalinvoice/thka/provider_test.go`: estructura completa del
  documento electrónico, cálculo de renglones y totales, IGTF en el total a
  pagar, token de doce horas (caché y renovación con 401), códigos del cuerpo
  (duplicado, validación, sin control), nota con factura afectada y las
  validaciones previas.
- `interfaces/web/digitalinvoice_processor_test.go`: que el documento lleve los
  renglones con las **dos** llaves con las que el sistema los guarda, que la tasa
  salga del registro del día, que no se encole sin imprenta activa, que un
  pendiente huérfano quede con su motivo, la emisión real por CG y por THKA, la
  venta del terminal encolada para SU empresa y **dos empresas con imprentas
  distintas emitiendo cada una por la suya**.
- `interfaces/web/digital_invoice_settings_test.go`: exclusividad entre
  imprentas, que desactivar una no apague la otra, que el modo de una empresa no
  toque a otra, el modo impresora fiscal, que guardar no cambie el modo, que los
  secretos guardados se conserven, las credenciales que exige cada proveedor y la
  pantalla representada (selector de imprentas, un panel visible y el resto
  ocultos, sin pintar la contraseña).

## 8. Límites conocidos (dichos antes de que se descubran)

- **Las credenciales se guardan en `config_json` sin cifrar**, igual que antes
  con el token de Unidigital. Quien pueda leer la base de control puede leerlas.
  Conviene restringir el acceso a la base y a los respaldos.
- **La anulación no está cableada al ERP.** El contrato recibe solo el número de
  control entero y las tres imprentas necesitan además el número de documento
  (THKA también la serie y el tipo); el adaptador lo dice con un mensaje
  explícito en vez de fingir. La anulación se hace desde el panel de la imprenta.
- **Consulta por identificador:** CG no expone consulta por `strong_id` y THKA
  consulta por serie/tipo/número de documento; Unidigital conserva sus métodos
  pendientes tal como estaban.
- **Facturas en divisas:** se rechazan en CG y en THKA con un mensaje claro. El
  ERP puede facturar en USD, pero por estas dos imprentas solo salen en Bs.
- **El PDF de THKA se pide a ciegas**: la referencia no documenta la respuesta
  de `/api/DescargaArchivo`. Si no se reconoce, el documento se emite igual y el
  PDF del cliente no queda archivado; el número de control (lo fiscal) sí.
