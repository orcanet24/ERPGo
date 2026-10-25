DigitalInvoice - Facturación Digital
En esta documentación se describe la REST API de Facturación Digital para Corporación Unidigital, Imprenta Digital en Venezuela autorizada por SENIAT que permite a los desarrolladores interactuar con la creación, anulación, busqueda y distribución de documentos fiscales. Aquí encontrarás información detallada sobre los recursos disponibles, los métodos admitidos, los parámetros de solicitud y respuesta, y más.

NOTA IMPORTANTE: Esta documentación esta diseñada para que crees un Fork en Postman, y teniendo las credenciales empieces a realizar tus pruebas. No es necesario solicitar unirse al Equipo de Unidigital.

Información de contacto y metadatos
Empresa: Corporación Unidigital 1220 C.A

Propietario de la API: Corporación Unidigital 1220 C.A

Creador de la API: Corporación Unidigital 1220 C.A

Página web: https://www.unidigital.global

Soporte del API: api@unidigital.global

URL API - Ambiente de producción: https://www.unidigital.global/digitalinvoice-core

Sitio Web - Ambiente de producción: https://www.unidigital.global/digitalinvoice-portal/

URL API - Ambiente de pruebas: https://qa.unidigital.global/digitalinvoice-core

Sitio Web - Ambiente de pruebas: https://qa.unidigital.global/digitalinvoice-portal

Aspectos Técnicos
¿Por qué un REST API?
En Unidigital, Imprenta Digital autorizada por el SENIAT hemos decidido ofrecer nuestros servicios a través de un REST API que permita a todos nuestros clientes construir experiencias basadas en tecnologías interconectadas donde los diferentes Sistemas se puedan comunicar de manera transparente y eficiente.

Este documento te ayudará a interactuar con nuestra REST API para que puedas conectar tu ERP, tu Sitio Web o Aplicación Móvil y así ofrecerle a tu cliente una mejor experiencia al administrar sus documentos fiscales.

En la construcción de nuestra REST API valoramos mucho las sugerencias de los desarrolladores que la utilizan, si tienes alguna puedes escribirnos a api@unidigital.global

Solicitud de acceso a Empresa SandBox
Lo primero que necesitas para usar el REST API es tener unas credenciales asociadas a la Empresa SandBox, si no tienes credenciales puedes solicitarla enviando un correo a api@unidigital.global indicando tu nombre, apellido, numero de cedula, y los datos fiscales de la empresa que representas. Una vez validada la información recibirás en tu correo las credenciales.

Codigos HTTP de respuesta
Las respuestas del API estan asociados a los estados HTTP

200 OK: El servidor ha procesado la solicitud correctamente y ha retornado una respuesta exitosa.

400 Bad Request: La solicitud no se pudo entender o estaba incompleta. Esta respuesta generalmente se asocia con un error en los datos enviados al servidor, algún campo requerido que no fue especificado o el incumplimiento de alguna regla de negocio como puede ser enviar un numero de documento repetido.

401 Unauthorized: La solicitud requiere autenticación, pero el cliente no ha proporcionado credenciales válidas. Generalmente se debe a que el token no ha sido especificado en el header de la solicitud o simplemente que el token esta vencido.

402 Not Found: La entidad buscada por el request no fue encontrada.

429 Too Many Request: una previa solicitud esta siendo atendida aun y por seguridad e integridad de datos debe esperarse que termine la solicitud anterior para continuar.

500 Internal Server Error: Se ha producido un error en el servidor mientras procesaba la solicitud. Estos errores son los menos comunes, es importante que del lado del cliente sean logeados y notificados al equipo de Unidigital para su evaluación y correción inmediata.

Notas: para las respuestas con HTTP Status Code 400 el json de respuesta siempre tendra la siguiente estructura

View More
javascript
{
    "result": null,
    "errors": [
        {
            "code": "0000",
            "message": "Mensaje explicativo del error",
            "extra": null
        }
    ],
    "success": [],
    "information": [],
    "haserrors": true
}
El campo errors es un arreglo que contiene una lista de todos los errores que fueron detectados durante el procesamiento de la solicitud, se recomienda que estos errores siempre sean logeados del lado del consumidor del API.

Entidades principales del API
Documento (Document)
Representa un Documento generado por el Sistema Corporativo del Contribuyente, por ejemplo una Factura, Nota de Crédito o Nota de Débito. Todo documento siempre formará parte de un ciclo. Si el documento es creado bajo el concepto de "Facturación en Linea" entonces se creará un ciclo por cada documento transmitido al API, por el contrario si el documento es creado bajo el concepto de "Facturación por Lotes" entonces se creará un ciclo que podra almacenar N cantidad de documentos.

Todos los documentos pasan por dos procesos de validación

Validación de Estructura: aqui el API se encarga de validar nombres del JSON y que los tipos de datos sean respetados. Mas adelante encontraras una tabla explicativa con todas las posibles propiedades que puede tener un documento.

Validación de Reglas de Negocio: aqui el API se encarga de validar que el documento cumpla con todos los requisitos de LEY, ejemplo: que sea correlativo con referencia al documento anterior, que las alicuotas esten calculadas correctamente. Mas adelante encontrarás una sección con las reglas de negocio y su explicación.

Ciclo o Lote (Batch)
El Ciclo o Lote representa un elemento que permite agrupar uno o muchos documentos y contiene información valiosa como puede ser el CurrentStatus (Estatus actual), fecha de creación, fecha de asignación de documentos, etc.

Tabla de Estaus

Estatus Valor Comentario
Open  1 Abierto, puede agregar documentos
Closed  2 Cerrado, no puede recibir documentos
Approved  3 Aprobado, en espera de asignación de números de control
Assigned  4 Asignado, ya los documentos le fueron asignados los numeros de control y quedan como documentos fiscales
Canceled  5 Cancelado, los documentos del Ciclo seran eliminados, nunca seran documentos fiscales
Aspectos Generales
Documentos fiscales soportados (tipos de documento)
Actualmente el Sistema soporta los siguientes documentos fiscales: Facturas, Notas de Credito, Notas de Debito, Guias de Despacho y Comprobantes de Retención de IVA e ISLR.

Numeros de documento
Cada documento emitido por la Empresa debe tener una numeración unica por tipo de documento asi tenemos que por ejemplo si emitimos 3 facturas, 2 notas de credito y 1 notas de debito, todos por primera vez las numeraciones quedarian de la siguiente forma:

Tipo  Fecha Numero
Factura 01/01/2023  1
Factura 01/01/2023  2
Factura 01/01/2023  3
Nota de Crédito 01/01/2023  1
Nota de Crédito 01/01/2023  2
Nota de Débito  01/01/2023  1
Como verán en la tabla anterior cada tipo de documento tiene su propia numeración, es decir su propio correlativo que en la medida que se emite el documento debe incrementarse el número.

Esta responsabilidad de manejar la numeración del documento por defeto es de la Empresa y no de la Imprenta Digital, sin embargo, también puede delegarse en la Imprenta solicitando la configuración.

El envio de documentos hacia la imprenta debe realizarse en estricto orden de numeración de forma ascedente por tipo de documento y serie.

Numeros de Control
El número de control es un entero generado por la Imprenta Digital y asignado a cada Documento fiscal emitido por la Empresa que contrata los servicios de facturación digital. Este número es unico e irrepetible independientemente del tipo de documento emitido.

Continuando con el ejemplo de Numeros de documento

Tipo  Fecha Numero  Numero de Control
Factura 01/01/2023  1 1
Factura 01/01/2023  2 2
Factura 01/01/2023  3 3
Nota de Crédito 01/01/2023  1 4
Nota de Crédito 01/01/2023  2 5
Nota de Débito  01/01/2023  1 6
Como verán el numero de control es un correlativo independiente del número de documento.

NOTA: La asignación de los números de control no son de manera inmediata, pueden tardar en un intervalo de 1-5 minutos, asi está la asignación de números de control ahorita pero, estamos mejorando cada día más para que los números de control se asignen de forma inmediata.

Talonarios digitales
La imprenta para gestionar los números de control utiliza la figura de talonarios digitales, los cuales agrupan una cantidad de numeros de control que sirven generalmente para cubrir el consumo de un mes de facturación.

Los talonarios digitales son la representación digital de los talonarios de formas libres que comunmente se utilizan con las imprentas tradicionales. Todo talonario tiene una fecha de creación, un rango de numeros de control y una serie asociada.

Los talonarios estan asociados a una serie, como veremos mas adelante toda Empresa tiene al menos una serie asociada que llamaremos serie en blanco o serie 0 (cero) las cuales representan un nivel de agrupación para los numeros de documentos.

Series de facturación
Cuando una empresa tiene varios sistemas, o varias lineas de negocio, surge la necesidad de crear series de facturación. Las series de facturación permiten asociar una cantidad de numeros de control con una serie, por defecto al registrar la empresa dentro de la plataforma una serie en blanco es asociada automaticamente, de manera que todos los numeros de control creados se haran bajo esa serie.

Si la empresa necesita tener mas de una serie, entonces la nueva serie se iniciara con la letra A.

Para la creación de series debe plantear su caso al departamento Legal de Unidigital y luego de la aprobación sera creada la serie dentro del sistema.

Continuando con el ejemplo para una Empresa que tiene la serie por defecto quedaria asi

Tipo  Fecha Numero  Numero de Control Serie
Factura 01/01/2023  1 1 0
Factura 01/01/2023  2 2 0
Factura 01/01/2023  3 3 0
Nota de Crédito 01/01/2023  1 4 0
Nota de Crédito 01/01/2023  2 5 0
Nota de Débito  01/01/2023  1 6 0
Como verán todos los documentos pertenecen a la Serie 0 (Cero) como le llamamos en Unidigital. La Serie 0 es la serie por defecto que se crea para todas las Empresas.

En la sección de talonarios digitales comentamos que todo talonario esta asociado a una serie, esto quiere decir que si la Empresa tiene mas de una serie entonces un talonario digital al ser creado debera especificarse para que serie se necesita.

Supongamos que tenemos dos talonarios digitales, cada uno de 10 numeros de control, uno asociado para la serie 0 y otro para la serie A. Los talonarios se verian asi:

Serie Numero de Control Desde Numero de Control Hasta
0 1 10
A 11  20
Estos talonarios pueden ser usados en paralelo, y la creación de la serie permite que tengamos nuevos consecutivos para los tipos de documentos. Usemos un ejemplo para entender.

Ahora que tenemos dos talonarios una para la serie 0 y otro para la serie A, queremos crear 3 facturas para la serie 0, 2 notas de debito para la serie 0, 5 facturas para la serie A y 5 notas de credito para la serie A, ¿Como quedarian los numeros de documentos y numeros de control? Observemos:

View More
Tipo  Fecha Numero de Documento Numero de Control Serie
Factura 01/01/2023  1 1 0
Factura 01/01/2023  2 2 0
Factura 01/01/2023  3 3 0
Nota de Débito  01/01/2023  1 4 0
Nota de Débito  01/01/2023  2 5 0
Factura 01/01/2023  1 11  A
Factura 01/01/2023  2 12  A
Factura 01/01/2023  3 13  A
Factura 01/01/2023  4 14  A
Factura 01/01/2023  5 15  A
Nota de Crédito 01/01/2023  1 16  A
Nota de Crédito 01/01/2023  2 17  A
Nota de Crédito 01/01/2023  3 18  A
Nota de Crédito 01/01/2023  4 19  A
Nota de Crédito 01/01/2023  5 20  A
Si observan con detalle, la numeración de los documentos por serie y por tipo de documento es independiente pudiendo entonces tener una factura con numero 1 para la serie 0 y tambien en la serie A. Adicionalmente los numeros de control se van usando en la medida que son requeridos por la serie, sin embargo estos nunca se repiten.

Notas
En la mayoria de los casos una Serie sera suficiente para cubrir las necesidades de las empresas.
AUTHORIZATION
Bearer Token
Token
{{vault:authorization-secret}}

POST
Factura con Metodos de Pago
https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove
La creación de un documento en línea implica la creación de un ciclo, agregar un documento e inmediatamente aprobarlo para que quede disponible para la asignación de números de control.

Esta solicitud detalla cómo incluir Métodos de Pago en el proceso, permitiendo registrar:

El tipo de pago (efectivo, tarjeta, transferencia, etc).

El monto recibido y, en caso de transacciones en moneda extranjera, su conversión al tipo de cambio vigente.

Para saber cuáles son los Códigos de los Metodos de Pago, puedes verificar en esta otra petición, Operaciones Básicas > Metodos de Pago

Si desean utilizar sus propios códigos de Metodos de Pago por limitaciones tecnicas, se pueden configurar para uso y se deben seguir el ejemplo "Codigo de pago personalizado" que aparece en el JSON de ejemplo de este endpoint.

NOTA: Si algunos de los pagos no es en Bolivares (Currency != "VES"), la base del IGTF (IGTFBaseAmount) y el monto de IGTF (IGTFAmount) del documento debe ser mayor a 0

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderDocumentos Fiscales
Body
raw (json)
View More
json
{    
    "SerieStrongId": "•••••••", 
    "SucursalStrongId": "Obtener de GET /commercialOffice",
    "DocumentType": "FA", 
    "Number": 617, 
    "EmissionDateAndTime": "2026-09-11T20:51:01.743Z", 
    "Name": "Ethan", 
    "FiscalRegistryCode": "J", 
    "FiscalRegistry": "297182748", 
    "Address": "3351 Hank Cliff", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",

    "Currency": "USD",
    "ExemptAmount": 10, 
    "TaxBase": 8, 
    "TaxBaseReduced": 2,
    "Subtotal": 20,
    "Discount": 0, 
    "PreviousBalance": 0,
    "SubtotalPlusDiscount": 20, 
    "TaxPercent": 16, 
    "TaxPercentReduced": 8,
    "TaxPercentSumptuary": 31,
    "TaxAmount": 1.28, 
    "TaxAmountReduced": 0.16,
    "TaxAmountSumptuary": 0,
    "Taxes": 1.44,
    "Total": 21.54, 
    "IGTFBaseAmount": 21.44, 
    "IGTFAmount": 0.6432,
    "IGTFPercentage": 3, 
    "GrandTotal": 22.0832,
    "AmountLetters": "", 
    
    "ConversionCurrency":"VES",
    "ExemptAmountVES": 1000,
    "TaxBaseVES": 800,
    "TaxBaseReducedVES": 200,
    "TaxPercentSumptuaryVES": 0,
    "SubtotalVES": 2000,
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "SubtotalPlusDiscountVES": 2000,
    "TaxAmountVES": 128,
    "TaxAmountReducedVES": 16,
    "TaxAmountSumptuaryVES": 0,
    "TotalVES": 2144,
    "IGTFBaseAmountVES": 2144,
    "IGTFAmountVES": 64.32,
    "GrandTotalVES": 2208.32,
    "AmountLettersVES": "",

    "ExchangeRate": 100,

    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { }, 

    "Payments": [
        // Pago en dolares
        {
            "PaymentMethod": "cash",        // Código del metodo de Pago 
            "Currency": "USD",             // Moneda del pago
            "ConversionCurrency": "VES",   // Moneda conversion
            "Amount": 10,                  // Monto recibido
            "AmountVES": 1000,             // Conversión del monto recibido
            "PaymentDate": "2025-06-03",   // Fecha de Pago
            "ExchangeRate": 100,           // Tasa de Cambio
            "Description": "Pago en dolares"          // Descripción del Pago (Opcional)
        },
        // Pago en bolivares
        {
            "PaymentMethod": "p2p",        // Código del metodo de Pago 
            "Currency": "VES",             // Moneda del pago
            "ConversionCurrency": "VES",   // Moneda conversion
            "Amount": 1000,                  // Monto recibido
            "AmountVES": 1000,             // Conversión del monto recibido
            "PaymentDate": "2025-06-03",   // Fecha de Pago
            "ExchangeRate": 1,           // Tasa de Cambio
            "Description": "Pago en bolivares"          // Descripción del Pago (Opcional)
        },
        // Codigo de pago personalizado
        {
            "PaymentMethod": "custom",        // Siempre debe ser custom
            "CustomPaymentMethod": "01",   // Se debe indicar cuales son los codigos de los metodos de pago
            "Currency": "VES",             // Moneda del pago
            "ConversionCurrency": "VES",   // Moneda conversion
            "Amount": 1000,                  // Monto recibido
            "AmountVES": 1000,             // Conversión del monto recibido
            "PaymentDate": "2025-06-03",   // Fecha de Pago
            "ExchangeRate": 1,           // Tasa de Cambio
            "Description": "Pago en bolivares"          // Descripción del Pago (Opcional)
        }
    ],

    "Details": [
        {
            "Description": "Unbranded Plastic Pants",
            "Quantity": 1,
            "UnitPrice": 10,
            "Amount": 10,
            "TaxAmount": 0,
            "TaxPercent": 0,
            "TaxCode": "E",
            "IsExempt": true,
            "OperationCode": "C001",
            "AmountPlusDiscount": 10,
            "TotalAmount": 10,
            "ProductType": 1
        },
        {
            "Description": "Ergonomic Cotton Cheese",
            "Quantity": 1,
            "UnitPrice": 8,
            "Amount": 8,
            "TaxAmount": 1.28,
            "TaxPercent": 16,
            "TaxCode": "G",
            "IsExempt": false,
            "OperationCode": "C001",
            "AmountPlusDiscount": 8,
            "TotalAmount": 9.28,
            "ProductType": 1
        },
        {
            "Description": "Rustic Soft Salad",
            "Quantity": 1,
            "UnitPrice": 2,
            "Amount": 2,
            "TaxAmount": 0.16,
            "TaxPercent": 8,
            "TaxCode": "R",
            "IsExempt": false,
            "OperationCode": "C001",
            "AmountPlusDiscount": 2,
            "TotalAmount": 2.16,
            "ProductType": 1
        }
    ]
}
Example Request
200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove' \
--data-raw '{    
    "SerieStrongId": "•••••••", 
    "DocumentType": "FA", 
    "Number": 602, 
    "EmissionDateAndTime": "2026-09-11T20:51:03.782Z", 
    "Name": "Robbie", 
    "FiscalRegistry": "J297182748", 
    "Address": "02091 Nicolas Point", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
    "Currency": "USD",
    "PreviousBalance": 0, 
    "Discount": 0, 
    "ExemptAmount": 100, 
    "TaxBase": 100, 
    "TaxAmount": 16, 
    "TaxPercent": 16, 
    "TaxPercentReduced": 1,
    "TaxBaseReduced": 2,
    "TaxAmountReduced": 3,
    "Total": 216, 
    "IGTFBaseAmount": 216, 
    "IGTFAmount": 6.48,
    "IGTFPercentage": 3, 
    "GrandTotal": 222.48,
    "AmountLetters": "UN MIL SEISCIENTOS DIEZ CON 01/100", 
    
    "ConversionCurrency":"VES",
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "ExemptAmountVES": 1125.48,
    "TaxBaseVES": 1125.48,
    "TaxAmountVES": 180.07,
    "TaxPercentVES": 16,
    "TaxPercentReducedVES": 4, 
    "TaxBaseReducedVES":5,
    "TaxAmountReducedVES":6,
    "TotalVES": 2431.03,
    "IGTFBaseAmountVES": 2431.03,
    "IGTFAmountVES": 72.9309,
    "GrandTotalVES": 2503.9609,
    "AmountLettersVES": "UN MIL SEISCIENTOS DIEZ CON 01/100",
    "ExchangeRate": 11.2548,
   
    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { },        
    "ShippingAddress":"579 Cordelia Inlet",
   
    "Details": [
        {
            "Description": "Unbranded Soft Shirt",
            "Quantity": 1,
            "UnitPrice": 301.72,
            "Amount": 301.72,
            "TaxAmount": 48.28,
            "TaxPercent": 16,
            "IsExempt": true,
            "OperationCode": "C001",
            "TotalAmount":0
        }
    ]
}'
200 OK
Example Response
Body
Headers (16)
json
{
  "result": "94f6d15d-4fca-4f1e-b01a-5b2ce40144d5",
  "information": [
    "Documento creado y aprobado satisfactoriamente"
  ],
  "success": [],
  "hasErrors": false
}
POST
Factura con Propina
https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove
La creación de un documento en línea implica la creación de un ciclo, agregar un documento e inmediatamente aprobarlo para que quede disponible para la asignación de números de control.

Esta solicitud detalla cómo incluir Propina en el proceso, permitiendo registrar alguna bonificación afuera del total de la compra.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderDocumentos Fiscales
Body
raw (json)
View More
json
{    
    "SerieStrongId": "•••••••", 
    "SucursalStrongId": "Obtener de GET /commercialOffice",
    "DocumentType": "FA", 
    "Number": 617, 
    "EmissionDateAndTime": "2026-09-11T20:51:01.744Z", 
    "Name": "Dangelo", 
    "FiscalRegistryCode": "J", 
    "FiscalRegistry": "297182748", 
    "Address": "325 Mertz Via", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",

    "Currency": "USD",
    "ExemptAmount": 10, 
    "TaxBase": 8, 
    "TaxBaseReduced": 2,
    "Subtotal": 20,
    "Discount": 0, 
    "PreviousBalance": 0,
    "SubtotalPlusDiscount": 20, 
    "TaxPercent": 16, 
    "TaxPercentReduced": 8,
    "TaxPercentSumptuary": 31,
    "TaxAmount": 1.28, 
    "TaxAmountReduced": 0.16,
    "TaxAmountSumptuary": 0,
    "Taxes": 1.44,
    "Total": 21.54, 
    "IGTFBaseAmount": 21.44, 
    "IGTFAmount": 0.6432,
    "IGTFPercentage": 3, 
    "Tip": 2.00, //Propina recibida
    "GrandTotal": 24.0832, //Suma de Total, IGTFAmount y Tip
    "AmountLetters": "", 
    
    "ConversionCurrency":"VES",
    "ExemptAmountVES": 1000,
    "TaxBaseVES": 800,
    "TaxBaseReducedVES": 200,
    "SubtotalVES": 2000,
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "SubtotalPlusDiscountVES": 2000,
    "TaxAmountVES": 128,
    "TaxAmountReducedVES": 16,
    "TaxAmountSumptuaryVES": 0,
    "TotalVES": 2144,
    "IGTFBaseAmountVES": 2144,
    "IGTFAmountVES": 64.32,
    "TipVES": 200.00, //Conversion de la Propina 
    "GrandTotalVES": 2408.32, // Sumatoria de TotalVES, IGTFAmountVES y TipVES
    "AmountLettersVES": "",

    "ExchangeRate": 100,

    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { }, 

    "Details": [
        {
            "Description": "Sleek Soft Tuna",
            "Quantity": 1,
            "UnitPrice": 10,
            "Amount": 10,
            "TaxAmount": 0,
            "TaxPercent": 0,
            "TaxCode": "E",
            "IsExempt": true,
            "OperationCode": "C001",
            "AmountPlusDiscount": 10,
            "TotalAmount": 10,
            "ProductType": 1
        },
        {
            "Description": "Rustic Fresh Shirt",
            "Quantity": 1,
            "UnitPrice": 8,
            "Amount": 8,
            "TaxAmount": 1.28,
            "TaxPercent": 16,
            "TaxCode": "G",
            "IsExempt": false,
            "OperationCode": "C001",
            "AmountPlusDiscount": 8,
            "TotalAmount": 9.28,
            "ProductType": 1
        },
        {
            "Description": "Practical Plastic Tuna",
            "Quantity": 1,
            "UnitPrice": 2,
            "Amount": 2,
            "TaxAmount": 0.16,
            "TaxPercent": 8,
            "TaxCode": "R",
            "IsExempt": false,
            "OperationCode": "C001",
            "AmountPlusDiscount": 2,
            "TotalAmount": 2.16,
            "ProductType": 1
        }
    ]
}
Example Request
200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove' \
--data-raw '{    
    "SerieStrongId": "•••••••", 
    "DocumentType": "FA", 
    "Number": 602, 
    "EmissionDateAndTime": "2026-09-11T20:51:03.782Z", 
    "Name": "Kelly", 
    "FiscalRegistry": "J297182748", 
    "Address": "0156 O'\''Reilly Parks", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
    "Currency": "USD",
    "PreviousBalance": 0, 
    "Discount": 0, 
    "ExemptAmount": 100, 
    "TaxBase": 100, 
    "TaxAmount": 16, 
    "TaxPercent": 16, 
    "TaxPercentReduced": 1,
    "TaxBaseReduced": 2,
    "TaxAmountReduced": 3,
    "Total": 216, 
    "IGTFBaseAmount": 216, 
    "IGTFAmount": 6.48,
    "IGTFPercentage": 3, 
    "GrandTotal": 222.48,
    "AmountLetters": "UN MIL SEISCIENTOS DIEZ CON 01/100", 
    
    "ConversionCurrency":"VES",
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "ExemptAmountVES": 1125.48,
    "TaxBaseVES": 1125.48,
    "TaxAmountVES": 180.07,
    "TaxPercentVES": 16,
    "TaxPercentReducedVES": 4, 
    "TaxBaseReducedVES":5,
    "TaxAmountReducedVES":6,
    "TotalVES": 2431.03,
    "IGTFBaseAmountVES": 2431.03,
    "IGTFAmountVES": 72.9309,
    "GrandTotalVES": 2503.9609,
    "AmountLettersVES": "UN MIL SEISCIENTOS DIEZ CON 01/100",
    "ExchangeRate": 11.2548,
   
    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { },        
    "ShippingAddress":"4525 Jast Rue",
   
    "Details": [
        {
            "Description": "Licensed Wooden Chicken",
            "Quantity": 1,
            "UnitPrice": 301.72,
            "Amount": 301.72,
            "TaxAmount": 48.28,
            "TaxPercent": 16,
            "IsExempt": true,
            "OperationCode": "C001",
            "TotalAmount":0
        }
    ]
}'
200 OK
Example Response
Body
Headers (16)
json
{
  "result": "94f6d15d-4fca-4f1e-b01a-5b2ce40144d5",
  "information": [
    "Documento creado y aprobado satisfactoriamente"
  ],
  "success": [],
  "hasErrors": false
}
Por lotes
La intención de crear documentos por lotes es utilizar eficientemente los recursos enviando solicitudes con un arreglo de documentos asociados a un ciclo.

El ciclo es un componente que permite agrupar N cantidad de documentos, de hecho incluso al crear un documento (facturación en línea) también se crea un ciclo pero con un solo documento asociado.

Los ciclos deben seguir una ruta para que los documentos queden "Asignados" o "Cancelados".

El algortimo recomendado sería el siguiente:

View More
code
Abrir Ciclo
Hacer Mientras existan documentos
    Agregar documentos al Ciclo
    Si ocurrio un Error agregando Documentos al Ciclo
        Cerrar Ciclo 
        Cancelar Ciclo 
        Fin del Programa
    Fin Si
Fin del Hacer
Cerrar Ciclo
Aprobar Ciclo
Fin del Programa
Notas
Por defecto la cantidad de documentos que soporta el request es de 100, si desea ampliar 500 debe solicitarlo.

Guarda el GUID de los documentos creados, RECOMENDABLE.

A continuación se dará los pasos a seguir para la creación de documentos por lotes, este proceso no es igual que el CreateAndApproved, debido a que la facturación por lotes debes abrir, cerrar y aprobar o cancelar el ciclo.

AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Reglas de negocio
En el proceso de creación de documentos, hay un paso fundamental que es la validación de los datos enviados, cada documento transmitido pasa por ciertas validaciones, todo esto para que cumpla con las normativas establecidas por el Seniat, dichas validaciones pueden ser tales como el correlativo de Nro. De Documento, validación de plantillas, entre otras.

En esta sección podrás conocer todas y cada una de las reglas de negocio existentes, hay que hacer enfasis que NO todas las compañias/empresas van a tener asignadas todas las validaciones, esto porque algunas de las validaciones no son compatibles con el modo de facturación del cliente.

Para saber con más detalle el porque se establecieron estas reglas de negocio, te invitamos que vayas a la sección de Preguntas Frecuentes, alli conseguirás la respuesta.

NOTA: En la respuesta del API [Status 400], las reglas de negocio están identificadas como "BusinessRule" en el tipo de error.

AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Documento afectado debe existir
Regla: AffectedDocumentMustExist

Descripción: El documento afectado debe Existir

Para la generación de una Nota de Crédito y de Débito, se debe especificar a que Factura estás afectando. Por lo general, la Factura debe existir en Imprenta Digital; Sin embargo, existe la posibilidad de transmitir una Nota de Crédito/Débito afectando a una Factura previo a Imprenta Digital, es decir, en la antigua Providencia.

En caso de que la Factura afectada si este en Imprenta Digital solo es necesario especificar el Nro. de Documento de la misma, y la Regla valida que exista, este en la misma serie y sea Factura, si no, entonces dará error de que el documento no fue encontrado.

Caso contrario, si la Factura afectada es de la antigua Providencia, antes de Imprenta Digital, entonces debes enviar los datos de esa Factura, los cuáles son:

View More
Nombre  Tipo de Dato  Description y/o Comentario
AffectedDocumentType  string  Tipo de Documento Afectado, solo acepta el tipo Factura (FA)
AffectedDocumentSerie string  Serie de Documento Afectado, tiene que ser la misma serie que la Nota de Crédito/Débito que intentas transmitir
AffectedDocumentDate  datetime (ISO 8601) Fecha de Emisión de Documento Afectado, debe ser menor a la Fecha de Emisión del primer Documento emitido en Imprenta Digital.
AffectedDocumentCurrency  string  Moneda del Documento Afectado, debe ser la misma que la Nota de Crédito/Débito que estás transmitiendo
AffectedDocumentAmount  decimal(18,6) Monto Total del Documento Afectado, debe ser mayor a 0
Ejemplo:

json
{
  // En caso de que exista en Unidigital
  "AffectedDocumentNumber": 125,
  
  //En caso de que no exista en Unidigital, enviar esto también
  "AffectedDocumentType": "FA",
  "AffectedDocumentSerie": "A",
  "AffectedDocumentDate": "2025-12-15T10:59:28",
  "AffectedDocumentCurrency": "VES",
  "AffectedDocumentAmount": "1.254,54"
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
POST
Nota de Crédito afectando Factura dentro de Imprenta
https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove
Este apartado permite la creación de esas Notas de Crédito y de Débito donde estén afectando a Facturas que estén a la Imprenta Digital. En esta petición solo se envia la propiedad "AffectedDocumentNumber", debido a que con el Nro. de Documento, podemos validar la información de la Factura Afectada.

Propiedad Tipo de Dato  Descripción y/o Comentario
AffectedDocumentNumber  int Número de Documento de la Factura Afectada. En este caso tiene que existir en Imprenta Digital
NOTA: Después de mandar el request para la creación de un documento, aconsejamos e insistimos en guardar el GUID del documento, que te devuelve el request. Para saber más sobre porqué es recomendable guardar el GUID del documento, leer el apartado de preguntas frecuentes

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderDocumentos Fiscales
Body
raw (json)
View More
json
{    
    "SerieStrongId": "a1a81b57-d2a3-4025-bf49-cf714354d468", 
    "DocumentType": "NC", 
    "Number": 162, 
    "EmissionDateAndTime": "2026-09-11T20:51:01.748Z", 
    "Name": "Brady", 
    "FiscalRegistryCode": "J", 
    "FiscalRegistry": "297182748",
    "Address": "648 Travon Dam", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
    "Currency": "VES",
    "PreviousBalance": 0, 
    "Discount": 0, 
    "ExemptAmount": 0, 
    "TaxBase": 301.72, 
    "TaxBaseReduced": 0,
    "TaxBaseSumptuary": 0,
    "TaxAmount": 48.28, 
    "TaxPercent": 16, 
    "TaxPercentReduced": 8,  
    "TaxPercentSumptuary": 31,
    "Total": 350.00, 
    "IGTFBaseAmount": 350.00, 
    "IGTFAmount": 10.50,
    "IGTFPercentage": 3, 
    "GrandTotal": 360.50,
    "AmountLetters": "UN MIL SEISCIENTOS DIEZ CON 01/100", 
    
    "ConversionCurrency":"USD",
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "ExemptAmountVES": 0,
    "TaxBaseVES": 26.81,
    "TaxAmountVES": 4.29,
    "TotalVES": 32.03,
    "IGTFBaseAmountVES": 32.03,
    "IGTFAmountVES": 0.96,
    "GrandTotalVES": 32.99,
    "AmountLettersVES": "UN MIL SEISCIENTOS DIEZ CON 01/100",
    "ExchangeRate": 11.2548,
   
    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { },        
    "ShippingAddress":"65739 Zackery Overpass",

    //Propiedad muy importante para la creación de Notas de crédito/Débito, enviar el Nró. de Documento de la Factura a afectar
    "AffectedDocumentNumber":603,  

    "Details": [
        {
            "Description": "Awesome Soft Pants",
            "Quantity": 1,
            "UnitPrice": 301.72,
            "Amount": 301.72,
            "TaxAmount": 48.28,
            "TaxPercent": 16,
            "TaxCode": "G",
            "IsExempt": false,
            "OperationCode": "C001",
            "AmountPlusDiscount": 301.72,
            "TotalAmount":350.00,
            "ProductType": 1
        }
    ]
}
Example Request
200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/createandapprove' \
--data-raw '{    
    "SerieStrongId": "•••••••", 
    "DocumentType": "NC", 
    "Number": 160, 
    "EmissionDateAndTime": "2026-09-11T20:51:03.784Z", 
    "Name": "Connor", 
    "FiscalRegistry": "J297182748", 
    "Address": "69387 Towne Fall", 
    "Phone": 2123310585, 
    "EmailTo": "api@unidigital.global", 
    "EmailCc": "api@unidigital.global", 
    "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
    "Currency": "VES",
    "PreviousBalance": 0, 
    "Discount": 0, 
    "ExemptAmount": 100, 
    "TaxBase": 100, 
    "TaxAmount": 16, 
    "TaxPercent": 16, 
    "Total": 216, 
    "IGTFBaseAmount": 216, 
    "IGTFAmount": 6.48,
    "IGTFPercentage": 3, 
    "GrandTotal": 222.48,
    "AmountLetters": "UN MIL SEISCIENTOS DIEZ CON 01/100", 
    
    "ConversionCurrency":"USD",
    "PreviousBalanceVES": 0, 
    "DiscountVES": 0,
    "ExemptAmountVES": 1125.48,
    "TaxBaseVES": 1125.48,
    "TaxAmountVES": 180.07,
    "TaxPercentVES": 16,
    "TotalVES": 2431.03,
    "IGTFBaseAmountVES": 2431.03,
    "IGTFAmountVES": 72.9309,
    "GrandTotalVES": 2503.9609,
    "AmountLettersVES": "UN MIL SEISCIENTOS DIEZ CON 01/100",
    "ExchangeRate": 11.2548,
   
    "SystemReference": "",            
    "Note1": "",
    "Note2": "",
    "Note3": "",
    "Extra": { },        
    "ShippingAddress":"093 Purdy Roads",
   
    "AffectedDocumentNumber":603,

    "Details": [
        {
            "Description": "Handcrafted Frozen Shirt",
            "Quantity": 1,
            "UnitPrice": 301.72,
            "Amount": 301.72,
            "TaxAmount": 48.28,
            "TaxPercent": 16,
            "IsExempt": false,
            "OperationCode": "C001",
            "TotalAmount":0
        }
    ]
}'
200 OK
Example Response
Body
Headers (16)
json
{
  "result": "bc0dd3b8-8870-4cd9-8636-0f5625f162ce",
  "information": [
    "Documento creado y aprobado satisfactoriamente"
  ],
  "success": [],
  "hasErrors": false
}
Los totales deben coincidir con la suma de los montos de los detalles
Regla: TotalsMustCoincideWithDetails

Descripción: Los totales deben coincidir con la suma de los montos de los detalles

Válida que la base exenta y las bases imponibles del documento coincidan con la suma de sus respectivos ítems en los detalles

View More
json
{
    "result": null,
    "errors": [
            "internalPosition": 1,
            "documentType": "FA",
            "number": 1,
            "fiscalRegistry": "J297182747",
            "name": "Dominique",
            "emissionDateAndTime": "2025-05-06T00:00:00Z",
            "address": "26288 Winifred Village",
            "currency": "VES",
            "errors": [
                {
                    "errorType": "BusinessRule",
                    "whatIsEval": "TotalsMustCoincideWithDetails",
                    "errorMessage": "Analizando los montos del documento 1, la base imponible 16% (TaxBase = 1055 VES) y la suma de los items gravados 16% (1000 VES) no coinciden"
                },
    ],
    "information": [],
    "success": [],
    "hasErrors": true
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Validación de totales
Regla: TotalsMustBeValid

Descripción: Los totales deben coincidir con la suma de los demás montos

Válida que los totales del documento tengan sentido con respecto a los montos utilizados en su cálculo.

Subtotal = ExemptAmount + TaxBase + TaxBaseReduced + TaxBaseSumptuary + TaxBaseZero

SubtotalPlusDiscount = Subtotal - Discount + PreviousBalance

Total = SubtotalPlusDiscount + Taxes

IGTFAmount = IGTFBaseAmount * IGTFPercentage

GrandTotal = Total + IGTFAmount

View More
json
{
    "result": null,
    "errors": [
            "internalPosition": 1,
            "documentType": "FA",
            "number": 1,
            "fiscalRegistry": "J297182747",
            "name": "Dominique",
            "emissionDateAndTime": "2025-05-06T00:00:00Z",
            "address": "26288 Winifred Village",
            "currency": "VES",
            "errors": [
                {
                    "errorType": "BusinessRule",
                    "whatIsEval": "TotalsMustBeValid",
                    "errorMessage": "Analizando los totales del documento 1, el gran total (GrandTotal = 1194.83 VES) y la suma del total mas el igtf (Total + IGFTAmount = 1249.83 VES) no coinciden"
                },
    ],
    "information": [],
    "success": [],
    "hasErrors": true
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Validación de formas de pago
Regla: PaymentsMustBeValid

Descripción: Los metodos de pagos deben ser validos

Válida que las formas de pago enviadas en el campo (Payments) tengan una estructura válida.

La moneda debe ser válida (VES, USD, EUR)

La tasa de cambio debe ser enviada si el pago no fue en Bolívares (VES).

La fecha de pago no puede ser mayor a la actual

Los montos deben ser mayores a 0

Si hay una conversión, el cálculo debe tener sentido con el monto en Bolívares y la tasa de cambio.

View More
json
{
    "result": null,
    "errors": [
            "internalPosition": 1,
            "documentType": "FA",
            "number": 1,
            "fiscalRegistry": "J297182747",
            "name": "Dominique",
            "emissionDateAndTime": "2025-05-06T00:00:00Z",
            "address": "26288 Winifred Village",
            "currency": "VES",
            "errors": [
                {
                    "errorType": "BusinessRule",
                    "whatIsEval": "PaymentsMustBeValid",
                    "errorMessage": "Analizando los pagos del documento 1, el pago en la posición 0 tiene que tener una tasa de cambio (ExchangeRate = 0) valida ya que las monedas son diferentes"
                },
    ],
    "information": [],
    "success": [],
    "hasErrors": true
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Montos positivos
Regla: AmountsMustBePositive

Descripción: Los montos del documento deben ser mayores o iguales a 0 (cero).

Válida que todos los montos enviados en el documento tengan un valor mayor o igual a 0.

View More
json
{
    "result": null,
    "errors": [
            "internalPosition": 1,
            "documentType": "FA",
            "number": 1,
            "fiscalRegistry": "J297182747",
            "name": "Dominique",
            "emissionDateAndTime": "2025-05-06T00:00:00Z",
            "address": "26288 Winifred Village",
            "currency": "VES",
            "errors": [
                {
                    "errorType": "BusinessRule",
                    "whatIsEval": "AmountsMustBePositive",
                    "errorMessage": "Analizando el documento 1 y el detalle en la posición 0, el campo UnitPrice (precio unitario) debe ser mayor a 0 (-500)"
                },
    ],
    "information": [],
    "success": [],
    "hasErrors": true
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Validación del documento de identidad
Regla: FiscalRegistryMustBeValid

Descripción: Identificación y su tipo debe ser válido

Válida que el código del documento de identidad sea enviado, y los valores numéricos o alfanúmericos sean consistente con el tipo de Identificación.

Persona Natural (V): Valores númericos de 6-9 caracteres

Persona Juridica (J): Valores númericos de 6-9 caracteres

Gubernamental (G): Valores númericos de 6-9 caracteres

Comunal (C): Valores númericos de 6-9 caracteres

Pasaporte (P): Valores alfanumericos de 6-9 caracteres

Extranjero (E): Valores númericos de 6-9 caracteres

Propiedades Afectadas:

FiscalRegistry

FiscalRegistryCode

Ejemplo:

json
{
   "FiscalRegistryCode": "V",
   "FiscalRegistry": "10852653"
}
Ejemplo de error si no envias los valores solicitados

Si no envias los valores recibiras errores de validación en la respuesta, aqui tienes un ejemplo:

View More
json
{
    "result": null,
    "errors": [
        {
            "internalPosition": 1,
            "documentType": "FA",
            "number": 25,
            "fiscalRegistry": "V16247983",
            "name": "WILLIAMS REVERON",
            "emissionDateAndTime": "2025-05-19T11:33:08.617",
            "address": "Los 2 caminos",
            "currency": "VES",
            "systemReference": "105",
            "errors": [
                {
                    "errorType": "BusinessRule",
                    "whatIsEval": "FiscalRegistryMustBeValid",
                    "errorMessage": "Documento 25: El valor 'V16247983' no cumple con el formato para Venezolanos (V). Debe ser numerico y tener entre 6 y 9 digitos. Ejemplo: (V,J,E,G,C,P)"
                }
            ],
            "message": "[Error] BusinessRule FiscalRegistryMustBeValid Documento 25: El valor 'V16247983' no cumple con el formato para Venezolanos (V). Debe ser numerico y tener entre 6 y 9 digitos. Ejemplo: (V,J,E,G,C,P)",
            "code": "000",
            "extra": "",
            "batch": "6f1f8090-3c12-405c-985b-282725ea421e"
        }
    ],
    "information": [],
    "success": [],
    "hasErrors": true
}
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from folderDocumentos Fiscales
Números de control
En esta sección podrás obtenerlos números de control de un Ciclo determinado, así como también los de una Serie.

Hacemos hincapie en que los números de control NO son asignados de manera inmediata, esta asignación puede tardar en un intervalo de 1-5 minutos. Estamos trabajando para que los números de control sean asignados de forma inmediata.

Teniendo en cuenta este dato, si tu transmites un documento, procedes a verificar inmediatamente el número de control y ves que no lo tiene asignado, es un comportamiento normal, tienes que esperar un momento a que se le asigne el número de control.

AUTHORIZATION
Bearer Token
Token
•••••••

POST
Descargar archivo de numeros de control
https://qa.unidigital.global/digitalinvoice-core/batch/controlNumbersfile?batch=•••••••
En este request puedes descargar en un texto plano los números de control de un determinado ciclo.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderNúmeros de control
PARAMS
batch
•••••••

Example Request
Descargar archivo de numeros de control
View More
curl
curl --location --request POST 'https://qa.unidigital.global/digitalinvoice-core/batch/controlNumbersfile?batch=%E2%80%A2%E2%80%A2%E2%80%A2%E2%80%A2%E2%80%A2%E2%80%A2%E2%80%A2'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
Búsquedas
En esta sección podrás ver diferentes requests para solicitar un documento, sea con el strongId del documento, número de contro, Rif del cliente, etc.

Para la búsqueda de documentos de cualquier tipo, incluyendo los documentos recibidos o los documentos donde la compañía aparece como tercero, se recomienda el uso del endpoint "Buscar documentos" (/documents/search).

AUTHORIZATION
Bearer Token
Token
•••••••

POST
Documentos
/documents/search
AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
Body
raw (json)
View More
json
{
    "number": 1, // Pagina

    "from": "2025-11-01 00:00:00", // ISO8601 - Fecha Desde
    "to": "2025-11-30 00:00:00", // ISO8601 - Fecha Hasta
    "dateType": 1, // 1-Transmision, 2-Emision 3-Asignacion

    // Campos opcionales
    "strongId": null, // GUID - StrongId del documento
    "documentNumber": null, // INT - Numero de documento
    "serie": null, // STRING - Nomre de la serie, ejemplo: "A"
    "fiscalRegistry": null, // STRING - Registro fiscal del cliente
    "systemReference": null, // STRING
    "controlNumber": null, // INT
    "batchStrongId": null, // GUID - StrongId del ciclo
    "documentTypes": null, // STRING[] - null = Cualquiera. Puede aceptar estos valores. ["FA", "NC", "ND", "GD", "RI", "RR"],
    // "documentTypes": ["NC", "ND"] // Buscar solo notas de credito y notas de debito
    "isThird": -1, // INT, -1 = Cualquiera, 0 = No terceros, 1 = Solo de terceros,
    "annulled": -1, // INT, -1 = Cualquiera, 0 = No anulados, 1 = Solo anulados

    // Tipo de participacion de la compania
    // Emitidos: Documentos que la compania emite
    // Recibidos: Documentos emitidos a la compania
    // Terceros: Documentos donde la compania aparece como un tercero
    "participantTypes": [2] // 0 = Emitidos, 1 = Recibidos, 2 = Tercero
}
Example Request
Documentos
View More
curl
curl --location '/documents/search' \
--data '{
    "number": 1, // Pagina

    "from": "2025-11-01 00:00:00", // ISO8601 - Fecha Desde
    "to": "2025-11-30 00:00:00", // ISO8601 - Fecha Hasta
    "dateType": 1, // 1-Transmision, 2-Emision 3-Asignacion

    // Campos opcionales
    "strongId": null, // GUID - StrongId del documento
    "documentNumber": null, // INT - Numero de documento
    "serie": null, // STRING - Nomre de la serie, ejemplo: "A"
    "fiscalRegistry": null, // STRING - Registro fiscal del cliente
    "systemReference": null, // STRING
    "controlNumber": null, // INT
    "batchStrongId": null, // GUID - StrongId del ciclo
    "documentTypes": null, // STRING[] - null = Cualquiera. Puede aceptar estos valores. ["FA", "NC", "ND", "GD", "RI", "RR"],
    // "documentTypes": ["NC", "ND"] // Buscar solo notas de credito y notas de debito
    "isThird": -1, // INT, -1 = Cualquiera, 0 = No terceros, 1 = Solo de terceros,
    "annulled": -1, // INT, -1 = Cualquiera, 0 = No anulados, 1 = Solo anulados

    // Tipo de participacion de la compania
    // Emitidos: Documentos que la compania emite
    // Recibidos: Documentos emitidos a la compania
    // Terceros: Documentos donde la compania aparece como un tercero
    "participantTypes": [2] // 0 = Emitidos, 1 = Recibidos, 2 = Tercero
}'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
GET
Por StrongId
https://qa.unidigital.global/digitalinvoice-core/documents?strongId=de3e77dd-208b-4e04-a030-00932b27be07
Búsqueda de Documento por GUID

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
PARAMS
strongId
de3e77dd-208b-4e04-a030-00932b27be07

Example Request
Por StrongId 200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents?strongId=4A828B73-582A-4A7F-87B1-84108A243DCD'
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "strongId": "4a828b73-582a-4a7f-87b1-84108a243dcd",
    "codeName": "FA",
    "serieStrongId": "bff47ea8-ec26-4e31-82dd-4c162eb04201",
    "serie": "A",
    "number": 1,
    "fiscalRegistry": "J-30182976-0",
    "name": "ASOCIACION VENEZOLANA DE INTERPRETES Y PRODUCTORES DE FONOGRAMAS AVINPRO",
    "currency": "VES",
    "exemptAmount": 0,
    "taxBase": 0,
    "taxBaseReduced": 0,
    "taxAmount": 0,
    "total": 0,
    "igtfBaseAmount": 0,
    "igtfAmount": 0,
    "grandTotal": 0,
    "controlNumber": 5001,
    "batchStrongId": "2a6be516-a72e-43df-9a85-b0114c463843",
    "status": "Assigned"
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
GET
Ultimo documento creado
https://qa.unidigital.global/digitalinvoice-core/documents/last
Obtener último documento creado, independientemente de la Serie.

De ser necesario, puede obtener el último documento de una determinada Serie, solo tienes que especificarlo en los parametros.

Asi de igual manera puedes obtener el último documento especificando también el Tipo de Documento.

Nota: Tienes que pasarle el GUID de la Serie.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
PARAMS
Serie
bff47ea8-ec26-4e31-82dd-4c162eb04211

Type
FA

Example Request
Ultimo documento creado
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/last'
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "strongId": "3494066b-a110-478a-afaf-2c0feb8864fb",
    "codeName": "FA",
    "serie": "0",
    "number": 3,
    "fiscalRegistry": "J297182748",
    "name": "Sandrine",
    "currency": "VES",
    "exemptAmount": 100,
    "taxBase": 100,
    "taxBaseReduced": 2,
    "taxAmount": 16,
    "total": 216,
    "igtfBaseAmount": 216,
    "igtfAmount": 6.48,
    "grandTotal": 222.48,
    "controlNumber": 3
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
GET
Por Numero de Control
https://qa.unidigital.global/digitalinvoice-core/documents/controlnumber/3
Búsqueda de Documento por Número de Control

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
Example Request
Por Numero de Control
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/controlnumber/3'
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "strongId": "3494066b-a110-478a-afaf-2c0feb8864fb",
    "control": 3,
    "number": 3,
    "emissionDate": "2024-10-15T15:22:38.955",
    "documentType": "FA",
    "name": "Sandrine",
    "total": 216,
    "currency": "VES",
    "annulled": false
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
POST
Rif de Cliente Final
https://qa.unidigital.global/digitalinvoice-core/documents/searchbyrif
Búsqueda de documento por Rif del Cliente final

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
Body
raw (json)
json
{
    "FiscalRegistry":"J297182748"
}
Example Request
Rif de Cliente Final
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/searchbyrif' \
--data '{
    "FiscalRegistry":"J297182748"
}'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
POST
Serie y Nro Documento
https://qa.unidigital.global/digitalinvoice-core/documents/searchbynumberandserie
Búsqueda de documentos por serie, número de documento y especificando que tipo de documento desea buscar

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderBúsquedas
Body
raw (json)
json
 {
    "Number":1,
    "Serie":"0",
    "DocumentType":"FA"
}
Example Request
Buscar documento por Serie y Nro Documento
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/searchbynumberandserie' \
--data ' {
    "Number":1,
    "Serie":"0",
    "DocumentType":"FA"
}'
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": [
    {
      "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e",
      "control": 1,
      "number": 1,
      "emissionDate": "2024-10-11T19:43:31.594",
      "documentType": "FA",
      "name": "Jany",
      "total": 216,
      "currency": "VES",
      "annulled": true
    }
  ],
  "information": [],
  "success": [],
  "hasErrors": false
}
Operaciones Básicas
AUTHORIZATION
Bearer Token
This folder is using Bearer Token from collectionDigitalInvoice - Facturación Digital
POST
Documentos soportados
https://qa.unidigital.global/digitalinvoice-core/documenttypes/list
Permite saber cuáles son los productos configurados dentro de la facturación digital. Los productos son los tipos de documentos fiscales que pueden ser creados dentro del sistema. Por los momentos solo son soportados las facturas, notas de crédito, notas de débito y guías de despacho.

La información más importante de la respuesta es el StrongId de los productos ya que será necesaria para otras solicitudes.

AUTHORIZATION
Bearer Token
Token
•••••••

Example Request
Documentos soportados
View More
curl
curl --location --request POST 'https://qa.unidigital.global/digitalinvoice-core/documenttypes/list'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
POST
Series configuradas
https://qa.unidigital.global/digitalinvoice-core/series/list
Permite saber cuáles son las series configuradas. Si todo el negocio del Contribuyente depende de un Sistema Centralizado entonces solo podrá tener una Serie, para el caso donde un contribuyente tenga un sistema descentralizado o varios sistemas soportando su operación y que la caída de uno no afecte la facturación del resto del negocio se pueden manejar "Series" por cada Sistema.

La información más importante de la respuesta es el StrongId de la serie ya que será necesaria para otras solicitudes.

AUTHORIZATION
Bearer Token
Token
•••••••

Example Request
Series configuradas
View More
curl
curl --location --request POST 'https://qa.unidigital.global/digitalinvoice-core/series/list'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
GET
Monedas
https://qa.unidigital.global/digitalinvoice-core/companies/currency
Permite obtener todos las monedas disponibles para facturar.

USD: Doláres

VES: Bolívares

EUR: Euros

AUTHORIZATION
Bearer Token
Token
•••••••

Example Request
200
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/companies/currency'
200 OK
Example Response
Body
Headers (18)
View More
json
{
  "result": [
    {
      "isoName": "VES",
      "description": "Bolivar",
      "strongId": "e997fe99-d6f0-4d16-b9eb-f82720a1db97",
      "bName": "La moneda",
      "id": 1
    },
    {
      "isoName": "USD",
      "description": "Dolar Americano",
      "strongId": "91e46bf4-449d-409d-82c0-3bf86968ef26",
      "bName": "La moneda",
      "id": 2
    },
    {
      "isoName": "EUR",
      "description": "Euro",
      "strongId": "1f458a75-21e5-4fb0-984d-289b0a65dca9",
      "bName": "La moneda",
      "id": 3
    }
  ],
  "information": [],
  "success": [],
  "hasErrors": false
}
GET
Codigos de Retencion ISLR
https://qa.unidigital.global/digitalinvoice-core/companies/retentionCodes
Permite obtener todos los codigos de las retenciones de ISLR segun lo indicado por el SENIAT

AUTHORIZATION
Bearer Token
Token
•••••••

Example Request
200
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/companies/currency'
200 OK
Example Response
Body
Headers (18)
View More
json
{
  "result": [
    {
      "isoName": "VES",
      "description": "Bolivar",
      "strongId": "e997fe99-d6f0-4d16-b9eb-f82720a1db97",
      "bName": "La moneda",
      "id": 1
    },
    {
      "isoName": "USD",
      "description": "Dolar Americano",
      "strongId": "91e46bf4-449d-409d-82c0-3bf86968ef26",
      "bName": "La moneda",
      "id": 2
    },
    {
      "isoName": "EUR",
      "description": "Euro",
      "strongId": "1f458a75-21e5-4fb0-984d-289b0a65dca9",
      "bName": "La moneda",
      "id": 3
    }
  ],
  "information": [],
  "success": [],
  "hasErrors": false
}
POST
Listar documentos
https://qa.unidigital.global/digitalinvoice-core/documents/list?page=1&thefrom=2024-10-01 00:00:00&theTo=2024-10-30 23:59:59
Te permite listar todos los documentos dentro de un rango de fecha, el parámetro "page" puedes definirlo en cada petición que hagas dependiendo de cuentas páginas con documentos te regrese.

AUTHORIZATION
Bearer Token
Token
•••••••

PARAMS
page
1

thefrom
2024-10-01 00:00:00

theTo
2024-10-30 23:59:59

Example Request
Listar documentos
View More
curl
curl --location --request POST 'https://qa.unidigital.global/digitalinvoice-core/documents/list?page=1&thefrom=2024-10-01%2000%3A00%3A00&theTo=2024-10-30%2023%3A59%3A59' \
--data ''
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "data": [
      {
        "strongId": "3494066b-a110-478a-afaf-2c0feb8864fb",
        "id": 9517482,
        "serie": "0",
        "documenType": "FA",
        "number": 3,
        "controlNumber": 3,
        "emissionDate": "2024-10-15T15:22:38.955",
        "name": "Sandrine",
        "fiscalRegistry": "J297182748",
        "address": "033 D'''angelo Gardens",
        "phone": "2123310585",
        "emailTo": "api@unidigital.global",
        "emailCc": "api@unidigital.global",
        "paymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
        "currency": "VES",
        "previousBalance": 0,
        "discount": 0,
        "exemptAmount": 100,
        "taxBase": 100,
        "taxPercent": 16,
        "taxAmount": 16,
        "total": 216,
        "igtfBaseAmount": 216,
        "igtfPercentage": 3,
        "igtfAmount": 6.48,
        "grandTotal": 222.48,
        "amountLetters": "UN MIL SEISCIENTOS DIEZ CON 01/100",
        "conversionCurrency": "USD",
        "exchangeRate": 11.2548,
        "previousBalanceVES": 0,
        "discountVES": 0,
        "exemptAmountVES": 1125.48,
        "taxBaseVES": 1125.48,
        "taxPercentVES": 16,
        "taxAmountVES": 180.07,
        "totalVES": 2431.03,
        "igtfBaseAmountVES": 2431.03,
        "igtfAmountVES": 72.93,
        "grandTotalVES": 2503.96,
        "amountLettersVES": "UN MIL SEISCIENTOS DIEZ CON 01/100",
        "systemReference": "",
        "note1": "",
        "note2": "",
        "note3": "",
        "customerIdentificationType": {
          "id": 1,
          "description": "RIFCEDULA",
          "label": "RIF / C.I."
        },
        "affectedDocumentNumber": 0,
        "details": [
          {
            "operationCode": "C001",
            "description": "Small Fresh Car",
            "quantity": 1,
            "unitPrice": 301.72,
            "amount": 301.72,
            "taxAmount": 48.28,
            "taxPercent": 16,
            "isExempt": false
          }
        ]
      },
      {
        "strongId": "771e12ef-e108-4ae1-b7eb-faf977800f93",
        "id": 9516743,
        "serie": "0",
        "documenType": "FA",
        "number": 2,
        "controlNumber": 2,
        "emissionDate": "2024-10-14T14:22:27.864",
        "name": "Santos",
        "fiscalRegistry": "J297182748",
        "address": "261 Renner Valley",
        "phone": "2123310585",
        "emailTo": "api@unidigital.global",
        "emailCc": "api@unidigital.global",
        "paymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
        "currency": "VES",
        "previousBalance": 0,
        "discount": 0,
        "exemptAmount": 100,
        "taxBase": 100,
        "taxPercent": 16,
        "taxAmount": 16,
        "total": 216,
        "igtfBaseAmount": 0,
        "igtfPercentage": 0,
        "igtfAmount": 0,
        "grandTotal": 0,
        "conversionCurrency": "USD",
        "exchangeRate": 0,
        "previousBalanceVES": 0,
        "discountVES": 0,
        "exemptAmountVES": 1125.48,
        "taxBaseVES": 1125.48,
        "taxPercentVES": 16,
        "taxAmountVES": 180.07,
        "totalVES": 2431.03,
        "igtfBaseAmountVES": 0,
        "igtfAmountVES": 0,
        "grandTotalVES": 2503.96,
        "systemReference": "PRUEBA/123",
        "customerIdentificationType": {
          "id": 1,
          "description": "RIFCEDULA",
          "label": "RIF / C.I."
        },
        "affectedDocumentNumber": 0,
        "details": [
          {
            "operationCode": "C001",
            "description": "Licensed Soft Keyboard",
            "quantity": 1,
            "unitPrice": 301.72,
            "amount": 301.72,
            "taxAmount": 48.28,
            "taxPercent": 16,
            "isExempt": false
          }
        ]
      },
      {
        "strongId": "4a828b73-582a-4a7f-87b1-84108a243dcd",
        "id": 9516740,
        "serie": "A",
        "documenType": "FA",
        "number": 1,
        "controlNumber": 5001,
        "emissionDate": "2024-10-10T02:00:00",
        "name": "ASOCIACION VENEZOLANA DE INTERPRETES Y PRODUCTORES DE FONOGRAMAS AVINPRO",
        "fiscalRegistry": "J-30182976-0",
        "address": "AV. EL CENTRO CON AV. ROMULO GALLEGOS EDIFICO TORRE MEGA IV, PISO 6, LOS DOS CAMINOS",
        "phone": "-",
        "emailTo": "luiscuzzi1999@gmail.com",
        "emailCc": "luiscuzzi1999@gmail.com",
        "paymentType": "15 DIAS",
        "currency": "VES",
        "previousBalance": 0,
        "discount": 0,
        "exemptAmount": 0,
        "taxBase": 0,
        "taxPercent": 12,
        "taxAmount": 0,
        "total": 0,
        "igtfBaseAmount": 0,
        "igtfPercentage": 0,
        "igtfAmount": 0,
        "grandTotal": 0,
        "amountLetters": "",
        "conversionCurrency": "VES",
        "exchangeRate": 1,
        "previousBalanceVES": 0,
        "discountVES": 0,
        "exemptAmountVES": 0,
        "taxBaseVES": 0,
        "taxPercentVES": 12,
        "taxAmountVES": 0,
        "totalVES": 0,
        "igtfBaseAmountVES": 0,
        "igtfAmountVES": 0,
        "grandTotalVES": 0,
        "amountLettersVES": "",
        "creditNoteText": "",
        "note2": "",
        "sucursal": "SANTA  FE",
        "customerIdentificationType": {
          "id": 1,
          "description": "RIFCEDULA",
          "label": "RIF / C.I."
        },
        "affectedDocumentNumber": 0,
        "details": [
          {
            "operationCode": "SAF001",
            "description": "AFILIACION LABORAL SERVICIO MÉDICO BÁSICO",
            "quantity": 22,
            "unitPrice": 1,
            "amount": 0,
            "taxAmount": 0,
            "taxPercent": 0,
            "isExempt": true
          }
        ]
      },
      {
        "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e",
        "id": 9516727,
        "serie": "0",
        "documenType": "FA",
        "number": 1,
        "controlNumber": 1,
        "emissionDate": "2024-10-11T19:43:31.594",
        "name": "Jany",
        "fiscalRegistry": "J297182748",
        "address": "00257 Agustin Keys",
        "phone": "2123310585",
        "emailTo": "api@unidigital.global",
        "emailCc": "api@unidigital.global",
        "paymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
        "currency": "VES",
        "previousBalance": 0,
        "discount": 0,
        "exemptAmount": 100,
        "taxBase": 100,
        "taxPercent": 16,
        "taxAmount": 16,
        "total": 216,
        "igtfBaseAmount": 0,
        "igtfPercentage": 0,
        "igtfAmount": 0,
        "grandTotal": 0,
        "conversionCurrency": "USD",
        "exchangeRate": 0,
        "previousBalanceVES": 0,
        "discountVES": 0,
        "exemptAmountVES": 1125.48,
        "taxBaseVES": 1125.48,
        "taxPercentVES": 16,
        "taxAmountVES": 180.07,
        "totalVES": 2431.03,
        "igtfBaseAmountVES": 0,
        "igtfAmountVES": 0,
        "grandTotalVES": 2503.96,
        "customerIdentificationType": {
          "id": 1,
          "description": "RIFCEDULA",
          "label": "RIF / C.I."
        },
        "affectedDocumentNumber": 0,
        "details": [
          {
            "operationCode": "C001",
            "description": "Handcrafted Wooden Computer",
            "quantity": 1,
            "unitPrice": 301.72,
            "amount": 301.72,
            "taxAmount": 48.28,
            "taxPercent": 16,
            "isExempt": false
          }
        ]
      }
    ],
    "totalPages": 1,
    "currentPage": 1
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
GET
Informacion de documento
https://qa.unidigital.global/digitalinvoice-core/documents/info/1F970F1B-50B1-4CAE-9D59-FEAF34316D3E
Este request te devuelve toda la información de un solo documento mediante el GUID del documento

AUTHORIZATION
Bearer Token
This request is using Bearer Token from folderOtras operaciones
Example Request
Informacion de documento
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/info/1F970F1B-50B1-4CAE-9D59-FEAF34316D3E' \
--data ''
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e",
    "id": 9516727,
    "serie": "0",
    "documenType": "FA",
    "number": 1,
    "controlNumber": 1,
    "emissionDate": "2024-10-11T19:43:31.594",
    "name": "Jany",
    "fiscalRegistry": "J297182748",
    "address": "00257 Agustin Keys",
    "phone": "2123310585",
    "emailTo": "api@unidigital.global",
    "emailCc": "api@unidigital.global",
    "paymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",
    "currency": "VES",
    "previousBalance": 0,
    "discount": 0,
    "exemptAmount": 100,
    "taxBase": 100,
    "taxPercent": 16,
    "taxAmount": 16,
    "total": 216,
    "igtfBaseAmount": 0,
    "igtfPercentage": 0,
    "igtfAmount": 0,
    "grandTotal": 0,
    "conversionCurrency": "USD",
    "exchangeRate": 0,
    "previousBalanceVES": 0,
    "discountVES": 0,
    "exemptAmountVES": 1125.48,
    "taxBaseVES": 1125.48,
    "taxPercentVES": 16,
    "taxAmountVES": 180.07,
    "totalVES": 2431.03,
    "igtfBaseAmountVES": 0,
    "igtfAmountVES": 0,
    "grandTotalVES": 2503.96,
    "customerIdentificationType": {
      "id": 1,
      "description": "RIFCEDULA",
      "label": "RIF / C.I."
    },
    "affectedDocumentNumber": 0,
    "details": [
      {
        "operationCode": "C001",
        "description": "Handcrafted Wooden Computer",
        "quantity": 1,
        "unitPrice": 301.72,
        "amount": 301.72,
        "taxAmount": 48.28,
        "taxPercent": 16,
        "isExempt": false
      }
    ]
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
POST
Anular documento
https://qa.unidigital.global/digitalinvoice-core/documents/anulled
En caso de que el documento fiscal ya ASIGNADO, tenga un error y se necesite anular, puede disponer de este Request, donde solamente le pedirá el Nro. de Control del documento a anular.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from collectionDigitalInvoice - Facturación Digital
Body
raw (json)
json
{
 "Control": 100
}
Example Request
documents/anulled 200
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/anulled' \
--data '{
 "Control": 1
}'
200 OK
Example Response
Body
Headers (14)
json
{
  "result": {
    "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e"
  },
  "information": [
    "Documento anulado satisfactorimente"
  ],
  "success": [],
  "hasErrors": false
}
POST
Agregar Documentos al Ciclo
https://qa.unidigital.global/digitalinvoice-core/documents/create
Paso 2: Agregar documentos al ciclo
La intención es indicar hasta un maximo de 500 documentos para agregar a un ciclo o lote determinado.

Solo se pueden agregar documentos a Ciclos que esten actualmente con estatus Abierto.

Puedes agregar todo tipo de documentos en un Ciclo (FA, NC, ND, GD).

A diferencia de la facturación en línea, aqui no te devuelve el GUID del documento

En el Body de la petición están los datos mínimos para crear un documento, existen variedad de propiedades para distintos planteamientos, puede ver todos los campos existentes en el apartado de Crear documentos.

AUTHORIZATION
Bearer Token
Token
•••••••

Body
raw (json)
View More
json
{
    "BatchStrongId": "•••••••",
    "Docs": [
       {    
        "DocumentType": "FA", 
        "Number": 617, 
        "EmissionDateAndTime": "2026-09-11T20:51:01.757Z", 
        "Name": "Jonathan", 
        "FiscalRegistryCode": "J", 
        "FiscalRegistry": "297182748", 
        "Address": "5905 Rosenbaum Run", 
        "Phone": 2123310585, 
        "EmailTo": "api@unidigital.global", 
        "EmailCc": "api@unidigital.global", 
        "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",

        "Currency": "USD",
        "ExemptAmount": 10, 
        "TaxBase": 8, 
        "TaxBaseReduced": 2,
        "Subtotal": 20,
        "Discount": 0, 
        "PreviousBalance": 0,
        "SubtotalPlusDiscount": 20, 
        "TaxPercent": 16, 
        "TaxPercentReduced": 8,
        "TaxPercentSumptuary": 31,
        "TaxAmount": 1.28, 
        "TaxAmountReduced": 0.16,
        "TaxAmountSumptuary": 0,
        "Taxes": 1.44,
        "Total": 21.54, 
        "IGTFBaseAmount": 21.44, 
        "IGTFAmount": 0.6432,
        "IGTFPercentage": 3, 
        "GrandTotal": 22.0832,
        "AmountLetters": "", 
        
        "ConversionCurrency":"VES",
        "ExemptAmountVES": 1000,
        "TaxBaseVES": 800,
        "TaxBaseReducedVES": 200,
        "TaxPercentSumptuaryVES": 0,
        "SubtotalVES": 2000,
        "PreviousBalanceVES": 0, 
        "DiscountVES": 0,
        "SubtotalPlusDiscountVES": 2000,
        "TaxAmountVES": 128,
        "TaxAmountReducedVES": 16,
        "TaxAmountSumptuaryVES": 0,
        "TotalVES": 2144,
        "IGTFBaseAmountVES": 2144,
        "IGTFAmountVES": 64.32,
        "GrandTotalVES": 2208.32,
        "AmountLettersVES": "",

        "ExchangeRate": 100,

        "SystemReference": "",            
        "Note1": "",
        "Note2": "",
        "Note3": "",
        "Extra": { }, 
        "Details": [
            {
                "Description": "Sleek Rubber Salad",
                "Quantity": 1,
                "UnitPrice": 10,
                "Amount": 10,
                "TaxAmount": 0,
                "TaxPercent": 0,
                "TaxCode": "E",
                "IsExempt": true,
                "OperationCode": "C001",
                "AmountPlusDiscount": 10,
                "TotalAmount": 10,
                "ProductType": 1
            },
            {
                "Description": "Fantastic Wooden Chips",
                "Quantity": 1,
                "UnitPrice": 8,
                "Amount": 8,
                "TaxAmount": 1.28,
                "TaxPercent": 16,
                "TaxCode": "G",
                "IsExempt": false,
                "OperationCode": "C001",
                "AmountPlusDiscount": 8,
                "TotalAmount": 9.28,
                "ProductType": 1
            },
            {
                "Description": "Awesome Concrete Chair",
                "Quantity": 1,
                "UnitPrice": 2,
                "Amount": 2,
                "TaxAmount": 0.16,
                "TaxPercent": 8,
                "TaxCode": "R",
                "IsExempt": false,
                "OperationCode": "C001",
                "AmountPlusDiscount": 2,
                "TotalAmount": 2.16,
                "ProductType": 1
            }
        ]
      }
    ]
}
Example Request
200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/create' \
--data-raw '{
  "BatchStrongId": "•••••••",
  "Docs": [
    {    
      "SerieStrongId": "•••••••", 
      "DocumentType": "FA", 
      "Number": 63, 
      "EmissionDateAndTime": "2026-09-11T20:51:03.789Z", 
      "Name": "Jodie", 
      "FiscalRegistry": "J297182748", 
      "Address": "784 McLaughlin Tunnel", 
      "Phone": 2123310585, 
      "EmailTo": "api@unidigital.global", 
      "PaymentType": "CONVENIMIENTO DE PAGO 04 CUOTAS",

      "Currency": "USD",
      "ExemptAmount": 100, 
      "TaxBase": 100, 
      "TaxAmount": 16, 
      "TaxPercent": 16, 
      "Total": 216, 

      "ConversionCurrency":"VES",
      "ExemptAmountVES": 1125.48,
      "TaxBaseVES": 1125.48,
      "TaxAmountVES": 180.07,
      "TaxPercentVES": 16,
      "TotalVES": 2431.03,
      "GrandTotalVES": 2503.9609,
      "ExchangeRate": 11.2548,

      "Details": [
          {
              "Description": "Awesome Metal Pants",
              "Quantity": 1,
              "UnitPrice": 301.72,
              "Amount": 301.72,
              "TaxAmount": 48.28,
              "TaxPercent": 16,
              "IsExempt": false,
              "OperationCode": "C001"
          }
      ]
    }
  ]
}'
200 OK
Example Response
Body
Headers (14)
View More
json
{
  "result": [
    {
      "strongId": "d62831d8-282f-4c45-8f97-a601764e20fa",
      "serie": "0",
      "serieStrongId": "a1a81b57-d2a3-4025-bf49-cf714354d468",
      "documenType": "Factura",
      "number": 63
    }
  ],
  "information": [
    "Documentos creados satisfactoriamente"
  ],
  "success": [],
  "hasErrors": false
}
POST
Aprobar Ciclo
https://qa.unidigital.global/digitalinvoice-core/batch/approve
Al Aprobar un ciclo le esta indicando al Sistema que está conforme con su contenido y que el ciclo queda disponible para la asignación de los números de control y conversión a documento fiscal.

AUTHORIZATION
Bearer Token
Token
•••••••

Body
raw (json)
json
{
    "Id": "•••••••"
}
Example Request
Aprobar Ciclo
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/batch/approve' \
--data '{
    "Id": "•••••••"
}'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
POST
Cancelar Ciclo
https://qa.unidigital.global/digitalinvoice-core/batch/cancel
Al Cancelar un ciclo le esta indicando al Sistema que rechazas el contenido del ciclo y que este no debe afectar las numeraciones, libros de ventas, etc. El ciclo quedará disponible para ser eliminado en cualquier momento.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from collectionDigitalInvoice - Facturación Digital
Body
raw (json)
json
{
    "Id": "•••••••",
    "Errors":""
}
Example Request
Cancelar Ciclo
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/batch/cancel' \
--data '{
    "Id": "•••••••",
    "Errors":""
}'
Example Response
Body
Headers (0)
No response body
This request doesn't return any response body
GET
Obtener Nros. de Control
https://qa.unidigital.global/digitalinvoice-core/batch/0f0947e3-3bdf-4030-b3f5-0a19e13a5958/documents?number=1&size=10
Muchas veces nos han preguntado el porqué no devolvemos el Nro. de Control de una vez cuando creas un documento, eso es porque la asignación de Nro. de Control no es de forma inmediata, es decir, es asíncrono.

Teniendo eso en cuenta, debes consultar el Nro.de Control del documento depués de haber pasado un aproximado de 5-7 minutos.

Con esta petición te permitirá consultar los Nros. de Control de forma paginada, haciendo así la consulta más eficiente que consultar uno por uno.

AUTHORIZATION
Bearer Token
Token
•••••••

PARAMS
number
1

size
10

Example Request
200
View More
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/batch/0f0947e3-3bdf-4030-b3f5-0a19e13a5958/documents?number=1&pageSize=10'
200 OK
Example Response
Body
Headers (16)
View More
json
{
  "result": {
    "batchStrongId": "fdd85f08-ee09-4119-8a6b-4c57155515c6",
    "created": "2024-10-11T15:43:24.857",
    "createdBy": "api@unidigital.global",
    "currentStatus": 4,
    "opened": "2024-10-11T15:43:24.8599449",
    "closed": "2024-10-11T15:43:24.9288977",
    "approved": "2024-10-11T15:43:24.9315707",
    "assigned": "2024-10-14T10:28:03.7876658",
    "documents": {
      "number": 1,
      "totalPages": 1,
      "pageSize": 10,
      "totalCount": 1,
      "filters": [],
      "orders": [],
      "result": [
        {
          "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e",
          "documentType": "FA",
          "number": 1,
          "control": 1,
          "fiscalRegistry": "J297182748",
          "name": "Jany",
          "currency": "VES",
          "exemptAmount": 100,
          "taxBase": 100,
          "taxBaseReduced": 2,
          "taxAmount": 16,
          "total": 216,
          "igtfBaseAmount": 0,
          "igtfAmount": 0,
          "grandTotal": 0
        }
      ]
    }
  },
  "information": [],
  "success": [],
  "hasErrors": false
}
POST
Anular documento
https://qa.unidigital.global/digitalinvoice-core/documents/anulled
En caso de que el documento fiscal ya ASIGNADO, tenga un error y se necesite anular, puede disponer de este Request, donde solamente le pedirá el Nro. de Control del documento a anular.

AUTHORIZATION
Bearer Token
This request is using Bearer Token from collectionDigitalInvoice - Facturación Digital
Body
raw (json)
json
{
 "Control": 100
}
Example Request
documents/anulled 200
curl
curl --location 'https://qa.unidigital.global/digitalinvoice-core/documents/anulled' \
--data '{
 "Control": 1
}'
200 OK
Example Response
Body
Headers (14)
json
{
  "result": {
    "strongId": "1f970f1b-50b1-4cae-9d59-feaf34316d3e"
  },
  "information": [
    "Documento anulado satisfactorimente"
  ],
  "success": [],
  "hasErrors": false
}