# Alertas financieras y canales de aviso (CxC / CxP)

Los módulos de **Cuentas por Cobrar** y **Cuentas por Pagar** dejaron de ser dos listados
de saldos y pasaron a ser un tablero que dice **quién está mal, desde cuándo y por qué**, con
una sola salida: avisar. El mismo tablero y la misma salida los usan las personas y los
agentes de IA.

## 1. El tablero

Al abrir `/cartera/list` o `/pagar/list` se recalcula todo (y también desde la herramienta
`finance.scan_alerts`, para que una tarea programada deje el tablero listo antes de que
empiece el día). Se calculan:

**Cuentas por cobrar**

| Indicador | Qué mide |
|---|---|
| Por cobrar / Vencido / % vencido | saldo vivo y cuánto pasó de 30 días |
| Días de cobro (DSO) | saldo por cobrar contra lo facturado en los últimos 30 días |
| Cobrado (30 días) y su variación | ritmo de cobranza contra los 30 días anteriores |
| Concentración top 3 | cuánto depende la cartera de tres clientes |
| Antigüedad | tramos 0-30 / 31-60 / 61-90 / +90 |
| Mayores deudores | los cinco primeros, con días de atraso |

**Cuentas por pagar**: los mismos indicadores del lado de la deuda, más
**retenciones de IVA sin comprobante enviado** (comprobantes registrados en `vat_retention`
que todavía no tienen un mensaje enviado al proveedor).

### Alertas

Las alertas son **registros**, no cálculos de pantalla: viven en `finance_alert` con su
estado, su historial de avisos y una huella única. Motivos:

- `cartera_vencida` — cliente con saldo de más de 30 días (crítico a partir de 90).
- `cartera_mora_anormal` — su atraso **duplica** el promedio de días que tarda en pagar
  según sus cobros reales, y ya supera los 60 días. No es un atraso: es un cambio.
- `cartera_concentracion` / `pagar_concentracion` — una contraparte concentra ≥25% del total.
- `pagar_vencida` — deuda con proveedor de más de 30 días.
- `pagar_retencion_pendiente` — comprobante de retención de IVA sin enviar.

**Se cierran solas.** El escaneo marca `cerrada` toda alerta cuya condición ya no se cumple
(el cliente pagó). Lo que no hace es reabrir una alerta que una persona marcó `resuelta` o
`descartada`: volver a levantar lo que alguien cerró a propósito es la forma más rápida de
que dejen de mirar el tablero. Si el problema reaparece con otra referencia, nace una alerta
nueva.

## 2. Los canales de aviso

En **Configuración → Canales de aviso** el administrador crea uno o varios canales:

| Tipo | Qué necesita | Qué recibe |
|---|---|---|
| Correo (SMTP) | servidor, puerto, cifrado (TLS 465 / STARTTLS 587 / ninguno), remitente, usuario y clave | un correo en texto plano UTF-8 |
| SMS | URL de la pasarela, método, cabecera y token de autenticación | `{"to": "...", "message": "..."}` |
| Webhook | URL, método, token y opcionalmente secreto de firma | el evento en JSON, con `X-ERPGo-Signature` (HMAC-SHA256) |

Cada canal tiene dos interruptores que importan:

- **Activo**: mientras esté apagado no sale nada y los intentos quedan registrados como fallo.
- **Permitir agentes** (`ai_allowed`, **nace apagado**): decide si además de una persona un
  agente puede usar ese canal. Es la única salida de datos del sistema hacia un tercero, así
  que la habilita el administrador, canal por canal.
- **Enfriamiento** (`cooldown_days`, 3 por defecto): días mínimos entre dos avisos al mismo
  destinatario por la misma referencia. `0` = sin límite, y es una decisión explícita.

Las credenciales viven en la base de la empresa y **nunca vuelven al navegador**: la pantalla
recibe solo id, nombre y tipo. Al editar, un campo de clave vacío significa «no la cambies».

## 3. Los avisos

Desde el tablero, el botón **Avisar** abre un diálogo con el texto ya redactado por el sistema
(se puede editar) y muestra **a qué contacto va a salir**. El destinatario sale siempre de la
ficha del cliente o del proveedor: **no hay campo para escribir otra dirección**. Si el correo
está mal, se corrige la ficha.

Tres garantías, iguales para humanos y agentes:

1. **Idempotencia** (`finance_message.idem_key`): el mismo aviso, al mismo destinatario, por el
   mismo canal y el mismo día no sale dos veces.
2. **Enfriamiento**: el mismo aviso por la misma referencia no se repite antes del plazo del
   canal. Se puede insistir con **forzar**, y eso queda escrito.
3. **Registro**: canal, destinatario, asunto, cuerpo, quién lo mandó, si salió o falló y por
   qué. Está en **Mensajes enviados**, que es la pantalla que responde «¿ya se le avisó?».

Un canal mal configurado **no se finge**: el mensaje queda en `error` con el motivo escrito.

## 4. Lo que hacen los agentes

| Herramienta | Riesgo | Qué hace |
|---|---|---|
| `finance.receivables` | read | indicadores y alertas de cobranza (la misma lista de la pantalla) |
| `finance.payables` | read | indicadores y alertas de pago, con las retenciones pendientes |
| `finance.scan_alerts` | execute | recalcula las alertas sin esperar a que alguien abra la pantalla |
| `customer.request_payment` | **critical** | reclama el pago por un canal autorizado |
| `supplier.send_retention` | **critical** | envía al proveedor el resumen de sus comprobantes de retención |
| `notify.event` | execute | manda un evento a un webhook para que otra herramienta actúe |

Las dos que escriben hacia afuera se clasifican `critical` a propósito: **se ejecutan, quedan
auditadas y la corrida se marca para revisión humana** (`ai_task_run.needs_human`). Además
exigen permiso del rol (`customer_ledger` / `supplier_payment`), modo operador, canal con
`ai_allowed` y respetan idempotencia y enfriamiento.

Un agente **no puede elegir el destinatario**: las herramientas no aceptan un parámetro «para».
Eso es lo que impide que un dato consultado con una instrucción dentro convierta al ERP en
mensajero hacia una dirección elegida por un tercero. La regla 2 del prompt de sistema
(`ai_rules.go`) se reescribió para describir exactamente esta única salida autorizada en lugar
de prohibir toda salida, que ya no era cierto y confundía al modelo.

## 5. Límites conocidos

- **No hay fecha de vencimiento por factura**: `invoice` no la guarda. La antigüedad se mide
  desde la fecha de la factura, y está dicho en la pantalla. El día que se cargue el
  vencimiento, cambia el origen de la resta.
- La **mora anormal** necesita historia de cobros del cliente. Sin cobros registrados no hay
  «normal» contra lo que comparar, así que no se genera.
- El **SMS** usa un contrato genérico (`to` + `message`): sirve para una pasarela propia o un
  relé, no para el SDK de un proveedor concreto.
- Las pruebas corren en SQLite en memoria; no se probó contra MySQL real.
