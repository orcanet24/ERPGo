1. A Dónde Va el Dinero: el Destino
Antes de recibir tu primer pago tienes que decidir una cosa: a dónde va a llegar el dinero.

Eso es un destino — en la API, un acuerdo de liquidación (agreement, POST /api/v1/ext/agreements). Es un juego de reglas que dice cómo recibes: a qué cuenta bancaria tuya se acredita, qué métodos de pago aceptas, y si lo que llegue ahí se reparte con otros.

Se define una vez y se reutiliza. No creas un acuerdo por pago: creas uno por cada forma distinta de recibir que tenga tu negocio, y después lo usas en todos los pagos que vayan ahí. Un negocio con una sola cuenta tendrá uno; uno que separa ingresos por sucursal, o que reparte con socios en unos casos y no en otros, tendrá varios.

Un destino también puede bifurcarse. Si lo que llega ahí va a varias cuentas, se declara en el propio destino: el dinero se reparte al llegar. → Split

Cada destino tiene su agreement_id, y es lo que nos dice, en cada pago, por dónde enrutarlo.

→ El acuerdo de liquidación, a fondo

2. Cómo Inicias un Pago: la Sesión
Con tus destinos definidos, recibir un pago es abrir una sesión de pago. Cada pago que recibes es una sesión, y la creas con una sola llamada.

Una sesión tiene dos partes.

Primero, por dónde lo enrutas. Pones el agreement_id del destino que quieras usar, y con eso ya está dicho a qué cuenta va el dinero y qué métodos aceptas — no lo repites en cada pago.

Después, los datos de este pago:

Qué dices	Con qué campos
Cuánto, y en qué moneda lo fijas	amount_reference, currency_reference
A quién se lo pides y por qué concepto	identifier_label, identifier, description
A dónde vuelve la persona al terminar	success_url, failure_url
Pones el precio en la moneda que uses para fijar precios —dólar, euro, bolívares— y calculamos lo que se paga en bolívares. La liquidación es siempre en bolívares.

Y después, lo que quieras añadir: a dónde te avisamos (webhook_url), cómo se reparte si ese destino se bifurca (split), y cuánto quieres que viva el enlace.

3. Qué Te Devuelve: el Enlace de Pago
La respuesta trae dos cosas que te importan: una payment_url, que es la pantalla donde tu cliente va a pagar, y un status, que arranca en pending.

Esa URL es lo único que tu cliente necesita ver. Cómo se la pones delante es tu decisión: un botón en tu propia web o app, o un enlace que le envías por correo, por WhatsApp, o como un QR que le muestras.

Usa la dirección que te devuelven, no la construyas
Guárdala tal cual viene en la respuesta. Armarla a mano a partir del identificador lleva a un enlace que no da error: lleva a una pantalla plausible donde se pierde el pago sin que nadie se entere.

4. Cuánto Tiene que Vivir Ese Enlace
Ahí está la decisión que conviene tomar bien. Las dos formas de sesión te devuelven una payment_url; lo que cambia es cuánto vive — y por eso sirven para cosas distintas.

Botón	Solicitud
Vive	duration_minutes — minutos, de 5 minutos a 20 minutos	due_date_session — una fecha que pones tú
Se crean	de una	por lote
Para qué	un checkout: tu cliente está ahí, decidiendo ahora	un pago que espera: una factura que mandas hoy y se paga la semana que viene
El error que esto te evita
La URL de un Botón también es una URL, así que se puede mandar por WhatsApp — y morirse antes de que la abran. Si el pago va a esperar, no uses un Botón.

→ Botón · Solicitud

5. Un Pago Ocurre en Dos Fases
Y aquí está el detalle que confunde a todo integrador. Una transacción no es un evento: son dos.

La primera fase es el pago. Tu cliente paga y el dinero sale de sus manos. No queda en poder de SPIDI: entra en una cuenta propia del banco, y el banco es su custodio. Tu cliente ya no puede echarse atrás. La sesión pasa a paid y se emite payment_session.paid.

Aquí ya puedes entregar. No esperes a la segunda fase para habilitar tu servicio o despachar tu producto: el dinero está en un banco y el banco lo custodia.

La segunda fase es la acreditación. El dinero se mueve desde esa cuenta del banco hasta la tuya —o hasta las de tus receptores, si el destino se bifurca—. Aquí se aplica la comisión del banco, y por eso lo que se acredita es el monto neto. Se emite payment_session.accredited.

Tarda entre uno y dos minutos, no segundos.

→ Transacción a dos fases

6. Cómo Te Enteras de Cada Fase
La sesión es el registro de la transacción, y las dos fases se ven ahí. Una sola consulta te da el estado completo:

GET /api/v1/ext/payment-sessions/status/{session_id}
  → data.status                              ← la fase 1
  → data.session_payment.receiver_credits    ← la fase 2

El aviso no es la única fuente: es el atajo. Te avisa en cuanto ocurre, y así no tienes que preguntar. Llega por POST a tu webhook_url con tres cabeceras:

spidi-signature — el sello HMAC-SHA256 de spidi-timestamp + "." + el cuerpo crudo, con el secreto de tu cuenta.
spidi-timestamp — cuándo se generó. Entra en el cálculo de la firma.
idempotency-key — para deduplicar reintentos.
Tu endpoint debe verificar la firma, deduplicar y responder 2xx. → Manejar notificaciones

El status nunca refleja la fase 2
Tras paid, el status se queda en paid. No existe un estado accredited: si sondeas esperándolo, esperas para siempre. Lo que cambia es que receiver_credits deja de venir null — es otro bucle y otra condición de salida.

Guarda el session_id de cada transacción
Si tu servidor estuvo caído, reintentamos y luego se rinde: no hay reenvío. Cuando vuelvas no habrá un aviso esperándote — habrá una sesión que puedes consultar. Pero no hay forma de listar tus sesiones, así que el session_id es tu única llave de vuelta.

Un aviso de acreditación por cada parte del acuerdo
Llega un evento por cada transacción de crédito: sin split, un solo payment_session.accredited; con split, una acreditación por receptor más el cierre, todas a tu mismo webhook_url. → Split

Las Dos Reglas de Oro
El estado real lo manda tu backend, no la redirección. Volver a success_url no significa que te pagaron: confírmalo con GET status o por el webhook.
paid es el pago; la acreditación es cuándo lo ves en tu cuenta. Puedes entregar en paid. Y las dos fases se consultan en la sesión — el aviso solo te ahorra preguntar.

Recibe tu primer pago
¡Hola! En unos minutos vas a recibir tu primer pago de prueba con SPIDI, de punta a punta: lo creas, lo llevas a pagado y confirmas que te llegó el aviso. Que el dinero fluya.

Corre contra el simulador
Este tutorial usa el simulador hosteado de SPIDI: ábrelo en otra pestaña y lo tienes al lado mientras sigues los pasos. Te registras y completas un pago de prueba sin dinero real y sin credenciales en vivo. → Usar el simulador.

Verificado automáticamente
Los comandos de los pasos 1 a 5 se ejecutan contra el simulador cada vez que cambia algo: si alguno dejara de funcionar, lo detectamos antes de que llegue aquí. El Paso 0 es el alta en el simulador y se comprueba a mano.


Recibe tu primer pago con SPIDI
Guion: .docx · .yaml
Antes de Empezar
El simulador hosteado — https://sim.mispidi.com. No hay que instalar ni desplegar nada tuyo; te registras en el Paso 0.
curl para los ejemplos y un poco de Node para el receptor del aviso.
No necesitas credenciales reales: el registro te entrega una cuenta de prueba.
Paso 0 — Regístrate
Objetivo: obtener tus credenciales.

curl -X POST https://sim.mispidi.com/console/register \
 -H "Content-Type: application/json" \
 -d '{"email":"tu@correo.test","password":"lo-que-quieras"}'

Respuesta, y conviene saber para qué sirve cada pieza porque no son intercambiables:

{
 "console_token": "...",
 "key": {
 "account_id": "...",
 "token": "...",
 "webhook_secret": "whsec_...",
 "short_name": "...",
 "password": "apw_..."
 }
}

Pieza Para qué sirve
key.token El Bearer del API. Es el que usan todos los pasos de este tutorial
console_token La consola y la traza (GET /console/trace), y tus receptores de webhook
key.short_name + key.password Las credenciales de POST /api/spidipagos/login, para que puedas ejercitar el camino de autenticación que en producción es obligatorio
Cruzarlos devuelve 401, y ese 401 significa «token equivocado», no «token inválido».

En producción no haces este paso: obtienes el token con tu login real (/api/spidipagos/login), con las credenciales que te entregue SPIDI.

Detalle completo de las tres piezas, sus errores y la rotación del token de consola: → Credenciales del simulador.

Paso 1 — Crea Tu Acuerdo de Liquidación
Objetivo: definir cómo recibes el dinero (agreement_id).

curl -X POST https://sim.mispidi.com/api/v1/ext/agreements \
 -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
 -d '{
 "title": "Acuerdo sin Split",
 "description": "Sin distribución de fondos",
 "split": false,
 "payment_methods": { "immediate_debit": true, "crypto": false, "mobile_payment": true },
 "default_bank_account_id": "uuid_sofitasa_001"
 }'


Qué observar: la respuesta trae data.agreement_id —cuelga de data, no de la raíz—. Checkpoint: tienes un agreement_id.

Paso 2 — Crea la Sesión de Botón
Objetivo: generar el enlace de pago (payment_url).

curl -X POST https://sim.mispidi.com/api/v1/ext/payment-sessions/buttons \
 -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
 -d '{
 "amount_reference": 50,
 "currency_reference": "USD",
 "identifier_label": "Pedido",
 "identifier": "ORD-0001",
 "description": "Mi primer pago de prueba",
 "success_url": "https://miapp.com/pago-exitoso",
 "failure_url": "https://miapp.com/pago-fallido",
 "agreement_id": "<agreement_id>",
 "webhook_url": "https://tu-app.example/webhooks/spidi"
 }'

Para que el simulador hosteado te entregue el aviso tienes dos vías. Una es un webhook_url accesible desde internet: tu servicio desplegado, o un túnel tipo ngrok apuntando a tu puerto local. La otra es el relay, si tu receptor vive en localhost (p. ej. http://localhost:4020): abre un WebSocket saliente contra el simulador y le entrega el aviso a tu puerto, sin que tengas que exponer nada. → Conducir el ciclo.

Qué observar: data.payment_url y data.session_id. Checkpoint: tienes la payment_url y el session_id (ambos dentro de data). La sesión nace en pending; lo confirmas en el Paso 4 con GET status.

Los campos identifier_label, identifier y description son obligatorios (además de montos y URLs) — el contrato los exige.

Paso 3 — Lleva a Quien Paga al payment_url
Objetivo: que la persona complete el pago en la página de SPIDI.

<a href="<payment_url>" class="btn-spidi">Pagar con SPIDI</a>

En el simulador puedes abrir la payment_url en el navegador para ver la página de pago real (formulario de Pago Móvil + clave OTP), o forzar el desenlace desde el plano de control (siguiente paso). En la página, las claves OTP de prueba te dejan provocar cada resultado a mano — 000000 paga; otras fallan con su motivo. → Usar el simulador.

Paso 4 — Confirma el Pago del Lado del Servidor
Lección clave
La redirección a tu success_url no garantiza que te pagaron — solo dice que la persona volvió. Pregúntanos desde tu backend.

# 1) en el simulador, forzamos el desenlace (sin pago real):
curl -X POST https://sim.mispidi.com/control/sessions/<session_id>/outcome \
 -H "Authorization: Bearer <token>" \
 -H "Content-Type: application/json" \
 -d '{"leg":1,"outcome":"paid"}'

# 2) consultamos el estado REAL:
curl https://sim.mispidi.com/api/v1/ext/payment-sessions/status/<session_id> \
 -H "Authorization: Bearer <token>"

Qué observar: status: paid. Checkpoint: tu backend confirmó paid. Si sigue pending: nunca des por bueno el pago por la sola redirección.

Paso 5 — Recibe y Valida el Aviso de Pago (Webhook)
Al llegar a paid, el simulador envía a tu webhook_url el evento payment_session.paid con los headers spidi-signature (HMAC-SHA256 de spidi-timestamp + "." + el cuerpo crudo), spidi-timestamp e idempotency-key.

// Node — verificar firma (timestamp + "." + cuerpo CRUDO) + idempotencia
import { createHmac, timingSafeEqual } from "crypto";

app.post("/webhooks/spidi", (req, res) => {
 const ts = req.headers["spidi-timestamp"];
 const recibida = req.headers["spidi-signature"];
 if (!ts || !recibida) return res.status(401).end(); // sin timestamp no hay qué verificar

 const firma = createHmac("sha256", WEBHOOK_SECRET).update(`${ts}.${req.rawBody}`).digest("hex");
 const ok = firma.length === recibida.length &&
 timingSafeEqual(Buffer.from(firma), Buffer.from(recibida));
 if (!ok) return res.status(401).end(); // firma inválida
 if (yaVisto(req.headers["idempotency-key"])) return res.status(200).end(); // repetido
 procesar(req.body); marcarVisto(req.headers["idempotency-key"]);
 res.status(200).end();
});


Qué observar: la firma coincide; un aviso repetido (mismo idempotency-key) no se procesa dos veces. Si falla: firma inválida → 401; repetido → 200 sin reprocesar.

Dos cosas que hay que respetar o la verificación falla: la cadena firmada es spidi-timestamp + "." + cuerpo, y el cuerpo son los bytes recibidos, no el JSON re-serializado — un byte de diferencia invalida la firma. → Manejar notificaciones.

Paso 6 — ¡Listo! Y Cómo Te Llega el Dinero
Acabas de recibir tu primer pago con SPIDI: lo creaste, lo confirmaste del lado del servidor y procesaste el aviso de forma segura.

El flujo del dinero: el pago se debita (fase 1) y luego se acredita al receptor (fase 2, ya descontada la comisión del banco) — y eso viaja por un segundo webhook, payment_session.accredited. Ojo: el status nunca pasa de paid por la acreditación. La fase 2 la ves en el webhook, o en data.session_payment.receiver_credits. → Transacción a dos fases.

Solicitud
La Solicitud es la sesión para cuando el pago tiene que esperar: el enlace sigue vivo hasta una fecha que pones tú, así que puedes mandarlo por WhatsApp, por correo o como un QR y que lo paguen mañana. Se crean por lote, porque cuando mandas facturas normalmente no mandas una sola. → Las formas de recibir un pago

Una Solicitud no se marca failed
Solo tiene dos finales: o se paga (paid) o vence (expired). Si tu código reacciona a failed, esa rama nunca se ejecuta aquí. → Ciclo de vida.


Solicitud de pago: cuando el pago tiene que esperar
Guion: .docx · .yaml
Crear un Lote de Solicitudes
curl -X POST https://sim.mispidi.com/api/v1/ext/payment-sessions/request/batch \
 -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
 -d '{
 "continue_on_error": true,
 "items": [{
 "title": "Pago de Rafael",
 "currency_reference": "USD",
 "amount_reference": 100,
 "agreement_id": "<agreement_id>",
 "identifier_label": "Nombre Cliente",
 "identifier": "Rafael",
 "description": "Pago de servicio",
 "due_date_session": "2026-12-31T23:59:59Z",
 "due_date_reached_behavior": "keep_active",
 "late_notice_message": "Tu enlace de pago está por vencer",
 "internal_reference": "REF-0001",
 "success_url": "https://miapp.com/pago-exitoso",
 "failure_url": "https://miapp.com/pago-fallido",
 "webhook_url": "https://miapp.com/webhooks/spidi"
 }]
 }'

continue_on_error — si un ítem del lote falla, sigue con los demás.
items — una entrada por solicitud; cada una nace pending con su payment_url.
due_date_session / due_date_reached_behavior — hasta cuándo vale el enlace, y qué pasa al llegar la fecha:
due_date_reached_behavior Qué pasa al pasar la fecha
expire  El enlace deja de funcionar
keep_active El enlace sigue activo y se muestra el mensaje de aviso
Manda siempre el comportamiento, no te apoyes en el defecto
Si envías due_date_session sin due_date_reached_behavior, estás dejando en manos del defecto algo que tiene dos desenlaces opuestos: un enlace que muere solo o uno que sigue admitiendo pagos pasada la fecha.

Escríbelo explícito en cada sesión. Son cuatro caracteres de más en el cuerpo y te ahorran la clase de sorpresa que solo aparece cuando ya hay dinero de por medio.

Quien paga lo ve vencido; tu sistema no, hasta que preguntes
Son dos cosas distintas y conviene no mezclarlas:

Para quien abre el enlace, la fecha basta. Pasada la fecha con expire, la página muestra «Esta sesión ya no está disponible» y no deja pagar. Nadie tiene que hacer nada para que eso ocurra. (Comprobado en el sandbox el 9-sep-2026.)
Para tu sistema, no. Si das por válido un enlace sin consultar GET /api/v1/ext/payment-sessions/status/<session_id>, vas a seguir tratándolo como si aún admitiera un pago — y va a parecer que due_date_session no funciona, cuando lo que falta es la consulta.
Consulta el estado antes de dar por bueno un enlace. Es el mismo hábito que te salva en Success URL, aquí por un segundo motivo.

Pruébalo tú mismo, y ojo con la fecha que usas
En el sandbox de SPIDI, la fecha tiene que estar en el futuro. Una due_date_session ya pasada se rechaza al crear la sesión, con 400 y "Due date must be in the future". Así que la receta que funciona es: pon el vencimiento a un minuto de ahora, espera a que pase, y consulta.

# due_date_session: dentro de ~60 s · due_date_reached_behavior: "expire"
curl https://sandbox.api.spidipagos.com/api/v1/ext/payment-sessions/status/<session_id> \
 -H "Authorization: Bearer <token>"
# antes de la fecha -> { "data": { "status": "pending" } }
# después -> { "data": { "status": "expired" } } y user_message: "La sesión expiró…"


Con keep_active seguirá pending pasada la fecha, que es justo la diferencia.

En el simulador sí puedes usar una fecha en el pasado y ver el expired en la misma consulta, sin esperar. Es una diferencia deliberada para que probar no cueste un minuto de reloj, y está declarada en /fidelidad.

Y fíjate en lo que no pasa: hasta que no consultas, tu sistema no sabe nada. Esa es la lección, no un detalle del entorno.

late_notice_message — texto del aviso cuando el enlace está por vencer (obligatorio).
internal_reference — tu referencia interna de la solicitud (obligatorio).
La respuesta resume el lote: data.processed_count, data.successful_count y data.failed_count, un arreglo data.items[] (una sesión creada por ítem, cada una con su session_id y su payment_url) y data.errors[] con los ítems que no se pudieron crear.

Confirmar el Pago
Igual que el Botón: confirma con GET status y/o el webhook payment_session.paid. → Success URL: verificar status · Manejar notificaciones.

paid no es el dinero acreditado
paid confirma el débito (fase 1) — no que el dinero ya está en manos del receptor. La acreditación (fase 2) no cambia el status: te llega por el webhook payment_session.accredited, o la consultas en data.session_payment.receiver_credits, en la misma respuesta del status. → Transacción a dos fases.

Probar en el Simulador
# fuerza el pago de una sesión del lote (su session_id sale en data.items[].session_id)
curl -X POST https://sim.mispidi.com/control/sessions/<session_id>/outcome \
 -H "Authorization: Bearer <token>" \
 -H "Content-Type: application/json" \
 -d '{"leg":1,"outcome":"paid"}'


Este flujo (crear lote → pending → paid) está verificado contra el simulador.

El simulador como API
Los números de esta página
Qué Cuánto  De quién
retardo de acreditación en el simulador 5000 ms Simulador
timeout del receptor en el simulador  5000 ms Simulador
reintentos del simulador  0 · 2000 · 5000 ms  Simulador
retención de datos del simulador  30 días Simulador
límite de peticiones por comercio 100 req/min SPIDI
El simulador es un sandbox hosteado que emula el contrato de la API Productos de SPIDI: la máquina de estados de la sesión, la acreditación a dos fases y los webhooks firmados. Te registras, obtienes tus credenciales y trabajas en tu propio espacio aislado — nadie más ve ni afecta tus datos de prueba. Todo sin tocar producción ni mover dinero real.

Esta página es la referencia de la herramienta: qué superficie expone, qué hace con cada llamada y qué garantías da. Todo lo de aquí se maneja por HTTP, sin navegador. Si lo que buscas es el camino paso a paso para probar tu integración, ese es otro: Probar tu integración.

¿Por qué un simulador?
El entorno de pruebas en vivo no deja forzar un paid o un failed (el pago es un formulario real) ni dispara webhooks de prueba. El simulador sí: provocas el desenlace que quieras y recibes el aviso firmado, de forma reproducible.


El simulador como API: contrato, plano de control y webhooks
Guion: .docx · .yaml
Está en pie y responde ahora mismo: https://sim.mispidi.com. No hay que instalarlo ni desplegarlo — se abre y se usa, y su consola vive en esa misma dirección.

La Distinción que Más Caro Sale Ignorar
El simulador expone dos superficies mezcladas en el mismo host, y solo una de las dos existe en producción:

Superficie  ¿Existe en producción?  Para qué
POST /api/spidipagos/login  Sí  Autenticarte. Es contrato
/api/v1/ext/** (17 operaciones) Sí  Acuerdos, partners, sesiones, paradas, status. Es contrato
/console/** No  Alta self-service, traza de eventos, retos, limpieza
/control/** No  Forzar desenlaces y reiniciar tu cuenta
/pay/:id  No  La página de pago espejo, con claves de prueba
POST /register  No  Alta histórica, sin token de consola. Se mantiene por compatibilidad; no la uses
GET /health No  Sonda de vida del propio simulador
Andamiaje dentro del camino a producción = rotura garantizada
Todo lo marcado No desaparece el día que apuntes a producción. Úsalo desde tus pruebas, nunca desde el código que va a desplegarse. Un POST /control/... colado en la ruta de integración no falla hoy: falla el día del estreno.

Credenciales: Dos Tokens que No Se Cruzan
El alta vigente es POST /console/register, y entrega de una vez las tres piezas: el token de API con el que integras, el token de consola con el que lees la traza, y el webhook_secret con el que verificas las firmas.

Pieza Abre  No abre
key.token (token de API)  /api/v1/ext/** y /control/**  /console/**
console_token /console/** la API y el plano de control
key.webhook_secret  (no viaja en ninguna petición)  —
Cruzarlos devuelve 401, y ese 401 significa "token equivocado", no "token inválido". El detalle completo —respuestas, errores, la rotación del token de consola al hacer login, y el alta histórica POST /register— está en Credenciales del simulador.

El Plano de Control
Dos operaciones. Van con el token de API y solo tocan tus datos.

# llevar una sesión al desenlace que quieras
POST /control/sessions/<session_id>/outcome   {"leg": 1|2, "outcome": "..."}

# dejar tu cuenta como recién creada (borra solo lo tuyo)
POST /control/reset

Los cinco desenlaces posibles, con qué webhook emite cada uno y desde qué estado se admiten, están tabulados en Conducir el ciclo. Tres comportamientos del simulador que conviene saber porque no se deducen:

Una transición inválida responde 500, con el motivo en message (leg1 inválido desde estado paid, leg2 requiere status paid). Es feo y es informativo.
Forzar paid por el plano de control NO programa la acreditación. La fase 2 solo queda programada cuando el pago entra por la página de pago. Si fuerzas paid y esperas el payment_session.accredited, no llega: fuerza también la fase 2.
Tu orden explícita gana. Si fuerzas la fase 2 sobre una sesión que ya tenía acreditación programada, el simulador cancela la programada para no entregarte dos avisos contradictorios del mismo pago.
Cada ruta /control/** tiene un alias /__test__/** idéntico, herencia del arranque del proyecto. Usa /control/**.

El Refresco de Token
POST /api/spidipagos/refresh-token   {"refreshToken": "…"}

Devuelve un token y un refreshToken nuevos, igual que el login. Existe aquí porque existe en producción, aunque el contrato no lo declare: si el simulador respondiera 404, quien siguiera la guía de autenticación se daría contra un muro justo al hacerlo bien.

Dos diferencias que conviene saber:

El token de API del simulador no caduca, así que aquí refrescar es ejercitar la mecánica, no una necesidad.
El refresh anterior sigue siendo válido después de refrescar. No te apoyes en lo contrario: fuera del simulador no está garantizado.
Cuánto Tarda la Acreditación
Unos 5 segundos tras el pago, configurable con SIM_ACCREDITATION_DELAY_MS.

Ese número es didáctico, no realista
Está puesto corto para que veas las dos fases en una sola pantalla. En un entorno real la acreditación tarda entre uno y dos minutos, y el aviso llega unos segundos después de que el dinero ya está.

No escribas lógica que dependa de que la acreditación llegue rápido, ni pruebas que fallen si tarda. → Transacción a dos fases

Cómo Emite los Webhooks
Esto es lo que hace el simulador cuando le toca avisarte. Lo que tiene que hacer tu receptor está en Manejar notificaciones.

Los Créditos de la Acreditación
Tras acreditar, GET status incluye receiver_credits y receiver_credits_summary dentro de session_payment, y el webhook los trae al mismo nivel que session_payment. Esa diferencia de ubicación es la del contrato, y el simulador la reproduce: si tu parser solo funciona con una de las dos fuentes, aquí lo descubres.

Con split, cada receptor aparece con su crédito y la comisión del banco se aplica por crédito, no sobre el total de la sesión — igual que en producción. El total_credits del resumen cuenta los receptores del reparto más el owner, que es la comprobación que conviene hacer al conciliar.

Los Cuatro Eventos
payment_session.created · payment_session.paid · payment_session.accredited · payment_session.accreditation_to_recipient_failed

Solo el de acreditación trae receiver_credits_summary con el neto y las comisiones; los demás llevan session_payment con id, origin y amount_reference.

Las Cabeceras de Cada Envío
Cabecera  Contenido
content-type  application/json
spidi-signature HMAC-SHA256 de spidi-timestamp + "." + el cuerpo crudo, en hexadecimal, con tu webhook_secret
spidi-timestamp Marca de tiempo de la sesión, en ISO 8601. Entra en la firma
idempotency-key Un identificador por emisión
La cadena que se firma es spidi-timestamp, un punto, y el cuerpo crudo byte a byte. Dos formas de equivocarse, y las dos dan el mismo síntoma —la firma no coincide—: dejarse el timestamp fuera del cálculo, o firmar el JSON re-serializado en vez de los bytes que llegaron. Guarda el cuerpo sin tocar y usa el header tal cual.

Reintentos y Rendición
Un intento por cada espera de la escalera —0 · 2000 · 5000 ms, en ese orden—, con 5000 ms de timeout cada uno. Un 2xx cierra la entrega; agotados los intentos, el simulador se rinde y lo registra como tal en la traza.

Este calendario es del simulador, no de producción
Las fases de arriba son los del simulador y están puestos para que veas la secuencia completa en segundos, no para reproducir el ritmo de producción. Lo que sí puedes dar por bueno es la forma —hay reintentos, hay límite y hay rendición—, no los números. No escribas lógica que dependa de que el segundo intento llegue a los 2 s.

La idempotency-key no cambia entre reintentos de una misma emisión. Ese es justamente el punto: si tu receptor tardó y respondió tarde, vas a recibir el mismo aviso otra vez con la misma clave, y deduplicar por ella es lo que evita que le cargues el pago dos veces a tu cliente.

Captura Primero
El simulador registra el webhook en la traza aunque no tenga a dónde entregarlo — sesión sin webhook_url, receptor caído, relay desconectado. El aviso queda con su payload y su firma. Es deliberado: un webhook que nunca salió es exactamente la información que necesitas cuando no entiendes por qué tu servidor no recibe nada.

Cómo Decide a Dónde Entrega: loopback, direct, relay
El simulador clasifica cada webhook_url automáticamente. No hay nada que configurar, y la clasificación aparece en la traza como routing.

routing Cuándo  Qué significa para ti
loopback  La URL apunta al propio simulador, a /console/hooks/<account_id>/(paid|accredited|fail) No estás probando nada tuyo. Te estás contestando a ti mismo
relay El host es local o privado: localhost, 127.0.0.1, 0.0.0.0, ::1, 10.x, 192.168.x, 172.16-31.x — o la URL no se puede parsear La entrega va por el relay, si lo tienes conectado
direct  Cualquier otro host El simulador hace el POST a tu servidor por internet
loopback es la trampa que hay que conocer
Los receptores por defecto (/console/hooks/…, los que ves en GET /console/keys) responden siempre: dos con 200 y el de fail con 500 a propósito. Sirven para ver la mecánica de entrega funcionando sin montar nada, y no ejecutan una sola línea de tu código: ni tu verificación de firma, ni tu deduplicación, ni tu manejo de errores. Si todo te sale verde y el routing dice loopback, no has probado tu integración.

Todo lo que Entra por un Receptor Queda en la Traza
Cada POST que llega a /console/hooks/… se registra como webhook.received, con:

El cuerpo crudo Tal cual llegó. Con él y el spidi-timestamp de las cabeceras se reconstruye la cadena firmada; reserializar el JSON la invalida
El cuerpo parseado  Para leerlo cómodo
Las cabeceras Incluidas spidi-signature e idempotency-key, ya extraídas aparte
Y hay un cuarto receptor, captura, que acepta cualquier cosa:

POST /console/hooks/<account_id>/captura

Responde 200 a todo y lo deja en tu traza. Los otros tres modelan desenlaces de entrega; este es un microscopio: apúntale lo que sea —incluso avisos de otro sistema— y mira exactamente qué te habría llegado, byte a byte.

Filtra la traza por webhook.received para verlos.

El Relay
El simulador hosteado no puede alcanzar tu localhost; tu máquina sí puede alcanzarlo a él. El relay usa esa asimetría: abre un WebSocket saliente contra el simulador, se queda escuchando, y cuando baja un webhook lo entrega con un POST a tu puerto local.

npx https://docs.mispidi.com/relay/spidi-sim-relay.tgz --key <tu-token-de-api> --mock https://sim.mispidi.com


--key es el token de API, no el de consola: es como el simulador sabe a qué cuenta pertenece ese relay.
--mock es la URL del simulador. El relay la convierte al esquema WebSocket por su cuenta.
Un relay por key. Si abres un segundo, el primero se cierra.
No hace falta declarar nada más: basta con poner tu URL local en webhook_url y dejarlo corriendo. La receta completa está en Conducir el ciclo.

La Página de Pago (/pay/:id)
Cada sesión tiene una página de pago que espeja la pantalla real: selector de método —limitado a los que el acuerdo declaró en payment_methods—, los datos que ese método pide, temporizador y clave de pago. Acepta claves mágicas que provocan cada desenlace (000000 paga; otras fallan con su motivo) y permite elegir de antemano cómo termina la fase 2. Es la vía manual, para verlo y demostrarlo; la tabla de claves está en Conducir el ciclo.

Para pruebas automatizadas, el plano de control es la vía: no necesita navegador y es reproducible.

La Consola Existe, y Es Otra Cosa
Además de esta superficie programática, el simulador trae una consola con interfaz gráfica: las prácticas, la traza de eventos, el workbench y los retos. Es para mirar —depurar y aprender—, no para integrar, y por eso se documenta aparte: → El simulador desde su consola.

Todo lo que la consola muestra se puede obtener también por HTTP con el token de consola (GET /console/trace, GET /console/sessions, GET /console/progress). La consola no sabe nada que la API no cuente.

Qué Emula y Qué No
Sí: responde la API Productos validando contra la OpenAPI · fuerza paid/failed/expired y lo refleja en GET status · emite webhooks firmados con reintentos e idempotencia · acredita a dos fases · aísla por cuenta.
No: no mueve dinero real ni habla con bancos · no reproduce la lógica interna de SPIDI, solo el contrato.
Catorce Operaciones con Comportamiento, Tres con la Forma
Esto es lo que más conviene saber antes de escribir una prueba contra el simulador, porque una respuesta de ejemplo se parece mucho a una de verdad:

Comportamiento real (estado, aislamiento por cuenta, webhooks)  Solo la forma, con el ejemplo del contrato
login · crear acuerdo · crear partner · sesión de Botón · lote de Solicitud · consultar estado · expirar sesión · acuerdo de recepción de split · Paradas SPIDI: crear, listar, detalle, borrar, lote de sesiones y reordenar Las tres operaciones de Paradas que tampoco están operativas en la plataforma de SPIDI: actualizar, histórico y consulta múltiple
Las de la derecha se delatan en la respuesta: llevan la cabecera x-spidi-sim: mock, así que puedes distinguirlas sin haber leído esta página. Responden 200 con datos inventados que son idénticos para todas las cuentas. No las uses para probar lógica de estado: sirven para que tu deserializador vea la forma correcta de la respuesta, y para nada más. → Paradas SPIDI

El porcentaje de comisión de los ejemplos es ilustrativo
El simulador reparte comisiones usando un ejemplo de 1 % en receiver_credits_summary. La comisión real la fija el banco y varía por acuerdo comercial → Límites y reglas de operación. Úsalo para comprobar que tu código lee bien el campo; no para calcular lo que vas a cobrar.

Dos parámetros más que verás en las fases: la acreditación tarda unos 5 segundos en llegar tras un pago por la página (deliberado — así ves que pagado no es recibido), y los datos de prueba caducan a los 14 días.

En Qué NO Se Parece a Producción, Dicho por Él Mismo
Perseguir la paridad con producción sería una carrera perdida: son dos bases de código distintas. Lo que sí puede hacer un simulador honesto es saber y decir en qué no se parece, y eso es una lista finita que se mantiene.

Está publicada, y la sirve la propia instancia contra la que estás integrando:

curl https://sim.mispidi.com/fidelidad

No pide token —habla del simulador, no de datos de nadie— y viene de su código, no de un documento aparte: si una diferencia deja de ser cierta, una prueba lo tumba antes de que la leas aquí.

Hay dos clases, y conviene no confundirlas:

Huecos — SPIDI hace algo que el simulador todavía no. Su silencio no es prueba de que tu código funcione:

Hueco Qué significa para ti
continue_on_error se ignora en el lote de Solicitud Ese lote sale entero o falla entero: su 207 de éxito parcial no se produce. Los lotes de Paradas sí lo respetan y sí devuelven 207
Las cabeceras X-RateLimit-* son nuestras  Las mandamos cuando el freno está encendido, pero no está medido si producción las envía — nuestra guía de errores dice «si vienen», y ese «si» sigue sin resolverse. No escribas código que dependa de que estén
Repetir nunca choca No se lee ninguna clave de idempotencia: repetir una llamada crea otra cosa en vez de devolverte el conflicto
El 422 solo aparece en un sitio Expirar una sesión ya pagada. El contrato lo declara en doce operaciones; aquí once no lo producen
Tres operaciones de Paradas responden el ejemplo  Ver arriba. Se delatan con x-spidi-sim: mock
Un reparto sin split habilitado no se rechaza Si mandas una split.distribution sobre un acuerdo creado con split: false, aquí sale 200 — y en el sandbox de SPIDI también (medido el 9-sep-2026). Es un hueco compartido: el receptor sí se valida, y eso hace que el 200 parezca una confirmación. No lo es. Verifica el split de tu acuerdo al crearlo
Divergencias — lo hace distinto a propósito, y cambiarlo sería peor:

Divergencia Por qué
Un recurso de otra cuenta responde 404, no 403  Un 403 confirmaría que el recurso existe, y con una lista de identificadores eso permite averiguar cuáles son de otro. Preferimos no revelarlo aunque el contrato lo permita. Y por lo mismo ese 404 no lleva ninguna cabecera que lo distinga de uno inexistente: marcarlo sería reabrir la fuga por la puerta de atrás
Aquí puedes fechar el vencimiento en el pasado; en SPIDI no El sandbox rechaza una due_date_session ya pasada con 400 y "Due date must be in the future" (medido el 9-sep-2026), así que allí probar el vencimiento cuesta un minuto de reloj por intento: fechar en el futuro inmediato y esperar. Aquí la aceptamos y la sesión sale vencida en la primera consulta, para que la prueba sea instantánea y siga enseñando lo que importa: tu sistema no se entera del vencimiento hasta que consulta. Pasada la fecha, SPIDI lo refleja igual que nosotros — expire deja el estado en expired, keep_active en pending — y además su página de pago se cierra sola: al abrir un enlace vencido muestra «Esta sesión ya no está disponible». Eso último no lo emulamos: aquí el enlace no tiene página que cerrar
El freno por exceso de peticiones no viene puesto: lo enciendes tú  Un 429 permanente rompería recetas, capturas, prácticas y tus propias pruebas de forma intermitente. Apagado no molesta, y encendido sirve para lo que hacía falta: ensayar tu backoff. Se enciende en Credenciales, con el límite por minuto que quieras, y vale solo para tu cuenta. Mientras esté encendido, el API responde 429 con Retry-After y X-RateLimit-* — y no alcanza a /console/**, para que no te quedes sin poder leer la traza ni volver a apagarlo
POST /api/spidipagos/refresh-token existe aquí y no está en el contrato El entorno real lo tiene y su OpenAPI no lo declara. Sin él, quien siga la documentación de producción se daría contra un 404 y creería que se equivocó. Se delata con x-spidi-sim: fuera-de-contrato
Antes de saltar al sandbox
Esta lista es justo lo que conviene repasar al pasar del simulador al sandbox: es donde están las sorpresas. Lo que aquí figura como hueco, allí sí ocurre.

Siguiente Paso
¿Vas a integrar? → Recibe tu primer pago, que corre contra este simulador de punta a punta.

¿Vas a probar en serio desde tu código? → Credenciales · Conducir el ciclo · Observar y depurar.

¿Contribuyes al simulador? También puede correrse localmente (MySQL 8.4 + npm run dev, o docker compose up) para desarrollo y CI. Eso vive en el repositorio del proyecto (simulador/, dev-services/), no es necesario para integrar contra el sandbox hosteado.

El simulador desde su consola
Los números de esta página
Qué Cuánto  De quién
retardo de acreditación en el simulador 5000 ms Simulador
timeout del receptor en el simulador  5000 ms Simulador
reintentos del simulador  0 · 2000 · 5000 ms  Simulador
retención de datos del simulador  30 días Simulador
El simulador se puede usar entero por HTTP, sin abrir un navegador — eso está en El simulador como API. La consola es la otra mitad: una interfaz web que sirve para ver lo que tu integración ya probó y lo que le falta.

No es un panel de administración ni un sustituto de tu código. Es un espejo con memoria: tú integras desde tu editor, y la consola te va diciendo, con la evidencia en la mano, hasta dónde has llegado de verdad.

Vive en la raíz del simulador —https://sim.mispidi.com, sin más ruta— y entras con el token de consola que te dio POST /console/register. La dirección vieja, /console, sigue funcionando y redirige → Credenciales.

La consola no sabe nada que la API no cuente
Todo lo que ves aquí sale de endpoints que puedes llamar tú (GET /console/trace, GET /console/sessions, GET /console/progress). Si prefieres automatizar, no pierdes nada.


La consola del simulador: nueve prácticas que se marcan solas
Guion: .docx · .yaml
Cómo Está Montada
La disposición es el mensaje, y así está repartida:

Zona  Dónde Qué hay
La barra superior Arriba, siempre El paso a las prácticas y a las cuatro herramientas, y el interruptor de la traza
El rail Franja izquierda, dentro de una práctica  Las nueve prácticas y los cinco casos borde, con su estado
El área de trabajo  Centro  El taller de la práctica en la que estés, o la herramienta que hayas abierto
La traza  Panel derecho, plegable Los eventos de tu cuenta, en vivo
La consola: el rail a la izquierda, el taller en el centro, la traza siempre a la derecha

La traza no es una pantalla aparte a la que se va: está a la derecha mientras trabajas, y si te estorba la pliegas desde la barra superior. Antes sí era una vista propia, y se cambió a propósito — mirar lo que pasó no es una tarea distinta de integrar, es parte de integrar. El divisor entre el centro y la traza se arrastra para darle más o menos ancho, y también responde a las flechas del teclado cuando tiene el foco. Lo que elijas se recuerda.

Las Nueve Prácticas
El rail es la columna vertebral. Cada práctica es una cosa concreta que tu integración tiene que haber hecho:

# Práctica  Qué prueba
1 Conectado Que tu código haga su primera llamada autenticada
2 Acuerdo creado  El acuerdo es la configuración que reutilizas; no se crea uno por pago
3 Sesión abierta  Crear la sesión y ver que devuelve una URL para quien paga
4 Pago recibido El pago ocurre fuera de tu app y vuelve a ti por webhook
5 Tu endpoint contestó  El salto de los webhooks de prueba a tu propio servidor
6 Firma verificada  Que tu endpoint rechaza lo que no viene firmado por SPIDI
7 Acreditación  La fase 2: el dinero acreditado, no solo el pago hecho
8 Casos borde Probar lo que rompe: duplicados, reintentos, fallos y expiración
9 Listo para producción Todo lo anterior, probado con evidencia y no de memoria
Cada uno tiene su taller en el área de trabajo: qué es, por qué importa, el código que lo consigue, y qué mirar en la traza cuando ocurra.

Se Marcan Solos, y No Hay Forma de Marcarlos a Mano
Esto es lo que separa a esta consola de una lista de tareas. No hay una casilla que puedas marcar tú. Cada práctica se enciende cuando el simulador observa el evento que la prueba:

Práctica  Lo que el simulador tiene que ver
Conectado Cualquier respuesta 2xx de la API
Acuerdo creado  Un 2xx en POST /api/v1/ext/agreements
Sesión abierta  Un 2xx creando una sesión de Botón o un lote de Solicitud
Pago recibido La transición real de la sesión a paid
Tu endpoint contestó  Un webhook entregado con routing direct o relay
Firma verificada  Que tu endpoint rechace un reto de firma corrupta
Acreditación  La transición de la fase 2 a accredited
«Tu endpoint contestó» no cuenta con loopback
Es la regla más importante de todo el rail. Si tu webhook_url apunta a los receptores de prueba del propio simulador, el 2xx lo produce el simulador contestándose a sí mismo: no prueba nada sobre tu servidor. Esa práctica exige direct o relay, y por eso puede quedarse apagada aunque veas entregas verdes en la traza. Está apagada con razón. → El enrutado, a fondo

Así se ve el rail de una cuenta que creó su acuerdo, abrió su sesión, recibió el pago y acreditó — pero mandó el aviso a los receptores del propio simulador. Las prácticas que dependen de tu servidor siguen apagadas, y es correcto:

El rail con las prácticas 1 a 4 y 7 encendidas, y la 5 «Tu endpoint contestó» apagada

Las dos últimas prácticas —Casos borde y Listo para producción— no tienen regla propia: se derivan de las demás. Nunca vas a poder encenderlas por la vía rápida.

La Práctica 6, que Es un Reto de Verdad
«Firma verificada» no se puede deducir mirando eventos: el simulador solo ve que tu endpoint respondió 2xx, no si comprobó la cabecera spidi-signature. Así que en vez de deducirlo, lo provoca.

Desde el taller de la práctica 6 lanzas el reto sobre una sesión tuya. El simulador hace dos disparos sobre tu endpoint, y el orden importa:

Qué te manda  Qué tiene que pasar
1 Un webhook bien firmado Tu endpoint lo acepta con 2xx
2 El mismo webhook con la firma corrompida en un solo carácter  Tu endpoint lo rechaza — cualquier respuesta que no sea 2xx
Pasas el reto solo si ocurren las dos cosas. Si aceptas el legítimo y rechazas el corrupto, has demostrado que distingues entre los dos, que es justo lo que se mide.

La firma corrupta sigue siendo hexadecimal de la misma longitud, así que solo se cae si de verdad recalculas el HMAC.

Por qué dos y no uno. Porque un disparo suelto no distingue «rechazó la firma» de «rechaza todo». Un endpoint cuya URL está mal escrita responde 404 a cualquier cosa, y eso no prueba que verifiques nada: prueba que ahí no hay nada. Si el primer disparo no llega a aceptarse, el reto queda inconcluso y te lo dice con esas palabras, en vez de felicitarte.

Y por eso mismo no tienes que preocuparte de con qué código rechazas: 401, 403 o lo que uses. Una vez que aceptaste el legítimo, cualquier rechazo del corrupto vale.

El reto se niega a correr contra un receptor loopback, y te lo dice: el simulador estaría contestándose a su propia firma corrupta. Conecta tu endpoint —o el relay— primero. El reto espera hasta 5 segundos por tu respuesta.

Los Cinco Casos Borde
Están ordenados por probabilidad de morderte en producción, no alfabéticamente:

Caso  Qué tienes que provocar
Webhook duplicado El mismo aviso otra vez, con la misma idempotency-key
Reintento Que una entrega falle y el simulador vuelva a intentarlo
Falla la acreditación Una fase 2 con accreditation_failed
Falla el pago Una fase 1 con failed
Sesión expirada Dejar vencer una sesión sin pagarla
Los cinco son parte de «Listo para producción». No es rigor decorativo: son exactamente los cuatro o cinco días malos que tiene cualquier integración de pagos, y llegar a producción sin haberlos visto es enterarse el día que ocurren.

La Traza
El panel derecho muestra, en orden, todo lo que le pasó a tu cuenta: llamadas a la API con su código, transiciones de estado, e intentos y entregas de webhook con su routing, su intento número, su código de respuesta y su idempotency-key.

Dos cosas que conviene saber al leerla:

Un webhook queda registrado aunque no se haya podido entregar. Sesión sin webhook_url, servidor caído, relay desconectado: el aviso aparece igual, con su payload y su firma. Es justo la información que necesitas cuando tu servidor no recibe nada y no sabes por qué.
El routing de cada entrega está a la vista. Es la forma más rápida de descubrir que llevas media tarde contestándote a ti mismo.
Lo que ENTRA por los receptores del simulador también queda, como webhook.received, con el cuerpo crudo, las cabeceras y la firma. La traza cuenta las dos direcciones, no solo la de salida.
Aquí se ve el caso completo: el intento con la etiqueta loopback a la vista y, justo debajo, el webhook.received del receptor que lo contestó — las dos direcciones de la misma entrega, una encima de la otra.

La traza mostrando webhook.attempt con la etiqueta loopback y, debajo, el webhook.received correspondiente

Cuando una entrega agota sus reintentos, la traza registra la rendición como evento terminal. No desaparece en silencio.

Las Seis Secciones
Aparte de los talleres, la barra de arriba da paso a seis secciones:

Empezar — la puerta de entrada: qué es cada parte de la consola, con capturas de las propias pantallas, y el sitio donde reclamas una key que sacaste fuera de la consola. Esa reclamación es el único puente entre el tutorial «Recibe tu primer pago» —que da token y webhook_secret pero no token de consola, así que su traza no la ve nadie— y todo lo demás de esta pantalla.

Credenciales — tus tokens, tu account_id y tu webhook_secret a mano, más los receptores por defecto de tu cuenta. Es de donde se copian las cosas que hacen falta en cada curl.

Acuerdos — los acuerdos que has creado, para no tener que ir a buscar un agreement_id a la traza.

Sesiones — las sesiones de la cuenta, la más reciente arriba, con su estado y su importe. Lo que trae a nadie aquí es el enlace de pago: sin esta pantalla, quien perdía la payment_url de una sesión no tenía forma de volver a abrir la página de quien paga, y la única salida era crear otra sesión. Por eso el enlace es la acción principal de cada ficha y no un dato más.

Workbench — un banco de pruebas con un set curado de cinco operaciones: crear acuerdo, crear sesión de Botón, crear un lote de Solicitud, forzar un desenlace y resetear tu cuenta. Dos detalles que lo hacen útil:

Los ejemplos de cada petición salen del propio OpenAPI, no de una copia escrita a mano que se desincroniza.
Valida la forma antes de enviar, con el mismo validador que usa el envío real. Un 400 por un campo mal puesto lo ves antes de gastarlo.
Recetas — flujos completos montados de una sola vez, para cuando quieres ver la mecánica sin escribir nada: Pago completo crea acuerdo y sesión y fuerza las dos fases hasta accredited; Expiración deja la sesión vencer. Sirven para entender el flujo y no marcan «Tu endpoint contestó», porque usan los receptores del propio simulador.

El Informe
La práctica 9 genera un informe en Markdown listo para pegar en un PR: las nueve prácticas y los cinco casos borde con la fecha en que se probó cada uno, el simulador contra el que se hizo, y una línea clara de si estás listo o no.

Lo importante es que funciona aunque no estés listo, y entonces lo que falta aparece por su nombre bajo «Lo que queda sin probar». Un informe que solo se puede generar cuando ya está todo bien no sirve para nada, porque justo entonces es cuando nadie lo necesita.

Es una herramienta, no una medalla: sirve para que quien revise tu PR vea con qué evidencia estás pidiendo el merge.

Direcciones Antiguas
La consola se reorganizó y las direcciones viejas siguen funcionando: /trace, /empezar, /pagos, /acuerdos, /workbench y /recetas te llevan a donde vive ahora cada cosa. /trace va a la primera práctica, porque la traza dejó de ser una vista para estar siempre presente.

Siguiente Paso
→ El simulador como API para hacer lo mismo sin navegador · Conducir el ciclo para el camino paso a paso · Observar y depurar para el catálogo de síntomas.

