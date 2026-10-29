# Plan de Mensajería (WhatsApp + Telegram) para ERPGo

**Estado:** guía de implementación por fases. Sustituye a `PLAN-GOWA-WHATSAPP.md`.
**Fecha:** 2026-10-29.
**Ámbito:** un solo módulo `mensajeria` con proveedores intercambiables
(WhatsApp vía GOWA, Telegram en sus tres caminos, correo, SMS y webhook),
apagado de fábrica, activable solo por el super administrador, con cola única,
ritmo anti-baneo, resolución determinista de canal por contacto y auditoría.

---

## 0. Decisiones que este documento fija

1. **Un módulo, varios proveedores.** No se construyen dos módulos gemelos. La
   cola, el opt-out, las plantillas, el horario, los permisos, la auditoría y la
   resolución de destinatario son **una sola vez**. Lo que cambia por proveedor
   es el transporte.
2. **GOWA es el módem, no el módulo.** Se integra como **servicio externo**
   fijado por versión (opción C del plan anterior). No se copia el código al
   repositorio: un módulo anidado queda fuera de `go build/vet/test ./...` y
   nadie lo probaría en CI. Si más adelante hace falta un instalador único, se
   empaqueta el binario; el código de integración no cambia.
3. **Tres llaves de activación**, todas de super administrador:
   instalación (`msg_global.enabled`, base de control), empresa
   (`msg_setting.enabled`, base de la empresa) y módulo visible
   (`modules.is_active = 1`). Sin las tres, las rutas responden **404** y el
   menú no aparece.
4. **Nada sale en el momento.** Todo aviso entra en una cola persistente con
   prioridad, horario, cupos y pausas aleatorias. No hay «enviar a todos».
5. **El destinatario sale del registro, nunca de la pantalla.** Y el canal
   también: se resuelve con una tabla de canales verificados por contacto, no
   adivinando.
6. **«Aceptado» no es «entregado».** La UI distingue `en_cola`, `aceptado`,
   `entregado`, `leído`, `fallido` y `bloqueado`. Ningún estado se inventa.
7. **Telegram no es «igual que WhatsApp».** Se ofrecen tres caminos con
   capacidades distintas y riesgo distinto (ver §8). El camino de cuenta
   personal (MTProto) se ofrece, se advierte y se limita igual que GOWA.
8. **Lo que no se parametriza:** pausa mínima entre mensajes, tope de lote,
   prohibición de difusión masiva, opt-out obligatorio y auditoría.

---

## 1. Correcciones obligatorias al plan anterior

Estas ocho cosas están verificadas contra el código y hay que resolverlas
**antes** de la primera pantalla del módulo.

| # | Problema real | Dónde | Corrección |
|---|---|---|---|
| 1 | El filtro de menú por `modules.is_active` con un `JOIN` normal borra **todo** el menú: `menu.module_id` tiene `DEFAULT 'core'` y no existe fila `modules` con id `core`. | `migrations/schema_reconstruido_10_system.sql:158`, `menu.go` | `LEFT JOIN modules` + `COALESCE(is_active,1)`; se oculta solo la fila cuyo módulo existe y está en 0. |
| 2 | `/module` no exige master: un gerente con permiso de configuración puede activar/desactivar módulos. | `permissions.go` (`masterOnlyRoutes`) | Añadir `"/module": true`. |
| 3 | `moduleToggle` no invalida la caché del menú: el cambio tarda hasta 30 s en verse. | `module_handlers.go:171` | Llamar a `invalidarCacheMenu()` al guardar. |
| 4 | `modules` se **lee** de la base de la empresa y se **escribe** en la de control: en multi-empresa la fila del módulo no aparece. | `module_handlers.go:64` vs `:319` | Sembrar la fila por migración (se aplica a todas las bases), no desde `registerModuleCore`. |
| 5 | `finDestinatario` manda `wapp` por la rama de correo: el destinatario de un WhatsApp es un email. | `outbound.go:195-208` | Un conjunto explícito `canalUsaMovil()`: `wapp`, `gowa`, `telegram`. |
| 6 | El OTP en `handleAPILogin` dejaría al POS sin poder vender. | `handlers.go:205` | OTP solo en `handleLogin` (web), con política por rol. `/api/login` no se toca. |
| 7 | Normalizar teléfonos con reglas fijas de Venezuela produce falsos negativos en otros países. | nuevo | Tabla por país, longitudes por rango, `raw` + `msisdn`, rechazo de lo ambiguo. |
| 8 | «Enviado» se pinta como «entregado». | nuevo | Estados separados; `aceptado` solo significa que el módem lo recibió. |

---

## 2. Arquitectura

```
┌──────────────────────────── ERPGo (un proceso) ────────────────────────────┐
│ /settings/mensajeria (master)      /mensajeria (roles con permiso)         │
│ activación, riesgo, proveedores    bandeja, envío manual, ficha            │
│                                                                             │
│ internal/mensajeria/  ← lógica pura (normalización, capacidades, ritmo)    │
│ interfaces/web/mensajeria_*.go  ← pantallas y handlers                     │
│                                                                             │
│ cola única (msg_outbox) ──► proveedor ──► transporte                       │
│                                     ├── GOWA (HTTP loopback, otro proceso) │
│                                     ├── Telegram Bot API (HTTPS)           │
│                                     ├── Telegram MTProto (gotd/td)         │
│                                     ├── Telegram Gateway (OTP oficial)     │
│                                     └── SMTP / SMS / webhook (los de hoy)  │
└─────────────────────────────────────────────────────────────────────────────┘
```

- **Base de control:** `msg_global`, `msg_device`, `msg_outbox`,
  `msg_otp_challenge`. Es infraestructura compartida y el webhook necesita
  resolver dispositivo → empresa sin sesión.
- **Base de la empresa:** `msg_setting`, `msg_provider`, `msg_template`,
  `msg_message`, `msg_contact_channel`, `msg_optout`, `msg_user_access`.
- **Disco:** `data/mensajeria/` (sesiones MTProto, media temporal). Se respalda
  con el ERP; restaurar una sesión no garantiza evitar el QR.

---

## 3. Proveedores y capacidades

Cada proveedor declara lo que puede hacer. La resolución de canal se apoya en
esas capacidades, no en suposiciones.

```go
type Capacidades struct {
    ResuelveTelefono   bool // ¿puedo pasar de teléfono a destinatario?
    EscribeADesconocidos bool // ¿puedo escribir a quien no me conoce?
    ExigeOptIn         bool // ¿la persona tiene que aceptar antes?
    Acuses             bool // ¿hay entregado/leído?
    Media              bool
    OTP                bool
    Riesgo             string // bajo | medio | alto
}
```

| Proveedor | Resuelve teléfono | Desconocidos | Opt-in | Acuses | OTP | Riesgo |
|---|---|---|---|---|---|---|
| `whatsapp_gowa` | Sí | Sí (con cupo) | Recomendado | Sí | Sí (personal) | Alto |
| `telegram_bot` | **No** | **No** (solo quien inició el bot) | Sí (implícito) | Limitados | No | Bajo |
| `telegram_user` | Sí | Sí (con cupo) | Sí | Sí | Sí | Alto |
| `telegram_gateway` | Sí | Sí (solo códigos) | Sí | Entrega del código | **Sí** | Bajo |
| `email` | No | Sí | No | No | Sí | Bajo |
| `sms` | Sí | Sí | No | Según pasarela | Sí | Bajo |
| `webhook` | No | — | — | — | No | Bajo |

Regla dura: si el proveedor no puede resolver el teléfono, **no se le pasa un
teléfono**. Un bot de Telegram nunca recibe «el móvil del cliente» como destino;
recibe el `chat_id` que la persona ya abrió.

---

## 4. Modelo de datos

### 4.1 Control

**`msg_global`** (una fila): `id, enabled, daemon_url, daemon_user,
daemon_password_enc, webhook_secret_enc, default_country, risk_text_version,
updated_by, updated_at`.

**`msg_device`**: `id, device_id (único, <empresa>.<area>), tenant, area, label,
provider, msisdn (único), state, jid, daily_cap, hourly_cap, min_delay_s,
max_delay_s, warmup_day, next_send_at, paused_until, last_seen_at, created_by,
created_at`.

**`msg_outbox`**: `id, tenant, provider, device_id, priority, kind, to_msisdn,
body, media_path, template_code, ref_type, ref_id, idem_key (único), status,
attempts, send_after, claimed_at, provider_ref, error, created_by, created_at,
updated_at`.

**`msg_otp_challenge`**: `id, user_id, username, phone_masked, code_hash, salt,
expires_at, attempts, max_attempts, consumed_at, resend_count,
preauth_token_hash, ip, user_agent, created_at`.

### 4.2 Empresa

**`msg_setting`**: `id, enabled, risk_accepted_by, risk_accepted_at,
default_country, chat_enabled, otp_enabled, updated_by, updated_at`.

**`msg_provider`**: `id, provider, enabled, config_json, updated_by,
updated_at`. `config_json` no guarda secretos en claro salvo los que ya son
públicos (token de bot sí es secreto: se cifra).

**`msg_contact_channel`**: `id, msisdn, provider, address, verified_at, source,
status, preference, notes`. `address` es el `chat_id`, el JID o el teléfono
según el proveedor. Es la pieza que permite «este contacto tiene WhatsApp o
Telegram» sin adivinar.

**`msg_message`**: `id, device_id, provider, chat_jid, direction, message_id,
from_msisdn, to_msisdn, body, media_type, media_path, status, is_from_me,
sent_at, received_at, raw_json, created_at`.

**`msg_template`**, **`msg_optout`**, **`msg_user_access`** como en el plan
anterior, con `provider` donde aplique.

Índices mínimos: `msg_outbox(status, send_after)`, `msg_outbox(idem_key)` único,
`msg_message(device_id, chat_jid, sent_at)`, `msg_device(tenant, area)`,
`msg_device(msisdn)` único, `msg_contact_channel(msisdn, provider)` único.

---

## 5. Activación y permisos

### 5.1 Las tres llaves

| Llave | Tabla | Quién | Qué controla |
|---|---|---|---|
| Instalación | `msg_global.enabled` | master | ¿Hay servicio y está permitido aquí? |
| Empresa | `msg_setting.enabled` | master | ¿Esta empresa puede usarlo? |
| Módulo | `modules.is_active` | master | ¿Se muestra el módulo en esta empresa? |

- El módulo nace **apagado** (`is_active = 0`).
- Encender la empresa exige `risk_accepted_at` (texto de riesgo + responsable).
- Encender la instalación exige URL del servicio y credenciales.
- Apagar: la cola se pausa, el menú desaparece, las rutas dan 404, los datos se
  conservan. Al volver a encender, los pendientes pasan a `en_revision`.
- Todo cambio se audita con `s.audit(...)`.

### 5.2 Rutas

```go
routeDepartment["/mensajeria"] = 25        // module.id nuevo
routeDirectory["/mensajeria"]  = "mensajeria_chat|mensajeria_send|mensajeria_admin"
masterOnlyRoutes["/module"]    = true
```

- `/settings/mensajeria` ya cae bajo `/settings` (master).
- `/mensajeria` exige permiso y módulo activo; si está apagado, **404**.
- El webhook `/webhooks/mensajeria/<device_id>` va en la lista pública del
  middleware (sin sesión), se autentica con HMAC y se limita por tamaño.

### 5.3 Permisos

| Permiso | Qué permite |
|---|---|
| `mensajeria_chat` | Ver conversaciones de los dispositivos asignados |
| `mensajeria_send` | Enviar (siempre con `mensajeria_chat`) |
| `mensajeria_admin` | Dispositivos, áreas, plantillas y ajustes de la empresa |

---

## 6. Resolución de canal (el corazón del «sin falsos positivos»)

Al encolar un aviso para un contacto:

1. Si el contacto tiene **preferencia explícita** y ese proveedor está
   disponible y habilitado → se usa.
2. Si no, se recorre el **orden configurado por área** (por defecto
   WhatsApp → Telegram → SMS → correo).
3. Para cada candidato se comprueba:
   - que el proveedor esté habilitado en la empresa y en la instalación;
   - que exista una fila **verificada** en `msg_contact_channel` para ese
     `msisdn` y ese proveedor (o el dato nativo: correo, `chat_id` del bot);
   - que el proveedor tenga la capacidad necesaria (p. ej. OTP);
   - que el contacto no esté en `msg_optout` para ese tipo de mensaje.
4. Si ninguno sirve → el aviso queda **`bloqueado`** con el motivo escrito y un
   enlace a la ficha. Nunca se cae en silencio a otro canal ni a otro
   destinatario.
5. El canal elegido, la dirección y el motivo quedan registrados.

**La verificación no se hace al enviar.** Se hace con una acción explícita
(«verificar número»), con la captura de un opt-in, o con el primer mensaje
entrante; se guarda con fecha y caducidad. Verificar en cada envío es tráfico,
levanta sospecha y hace que un aviso dependa de una llamada externa.

---

## 7. Cola, ritmo y anti-baneo

- Un despachador propio (goroutine, tic de 5 s), no `internal/queue`.
- Prioridades: `otp` (inmediata, cupo aparte), `transaccional`, `recordatorio`,
  `marketing` (apagado por defecto).
- Parámetros por dispositivo: pausa 45–180 s (mínimo duro 20 s), typing 2–8 s,
  pausa larga 5 % (5–15 min), cupo diario 120 (nuevo: 20→40→80→120), cupo
  horario 25, horario 08:00–17:00 Lun–Vie, lote máximo 25 con confirmación,
  reintentos 3 (1/5/30 min), cortacircuito tras 5 fallos o 401/429.
- Opt-out inmediato con «BAJA», «STOP», «NO MOLESTAR».
- Sin grupos, sin difusión masiva, sin listas compradas.
- Si el servicio cae: la cola sigue aceptando; la UI lo dice; el OTP cae al
  correo si está configurado.

---

## 8. Telegram: tres caminos, no uno

| Camino | Conexión | Alcance real | Riesgo | Uso recomendado |
|---|---|---|---|---|
| **Bot API** | Token de @BotFather | Solo a quien inició el bot o en grupos/canales donde sea admin. No resuelve teléfonos. | Nulo para el bot | Avisos a clientes que ya usan el bot, avisos internos, grupos de trabajo |
| **MTProto** (`gotd/td`, MIT, Go puro) | Código al teléfono o QR `tg://login` | Puede importar contactos y escribir a un teléfono | **Alto**: cuenta personal automatizada; Telegram limita cuentas que escriben a desconocidos (`PEER_FLOOD`). Requiere `api_id`/`api_hash`. | Solo con aceptación de riesgo por escrito, igual que GOWA |
| **Gateway** (oficial, de pago) | API con `api_id`/`api_hash` de gateway | Solo códigos de verificación, por número E.164, con `checkSendAbility` | Ninguno | **OTP** |

Decisiones:
- **Avisos:** bot primero; MTProto solo si el dueño lo pide y acepta el riesgo.
- **OTP:** Gateway (oficial) o correo. El OTP por WhatsApp/GOWA es solo para
  personal y con respaldo.
- El «conectar el teléfono» de Telegram es MTProto y se implementa con
  `gotd/td` **dentro** del proceso (no hace falta sidecar), guardando la sesión
  cifrada en `data/mensajeria/telegram/`.

---

## 9. Fases, con criterio de certificación

Cada fase termina con `go build ./...`, `go vet ./interfaces/web`,
`go test ./...`, `go run ./internal/webcheck -dir templates` (si se tocó una
plantilla), commit y push. **No se pasa a la siguiente sin esto en verde.**

### Fase -1 — Correcciones de base (sin nada visible)
1. `canalUsaMovil()` y destinatario correcto para `wapp`/`gowa`/`telegram`.
2. `/module` master-only.
3. `moduleToggle` invalida la caché del menú.
4. Filtro de menú por `modules.is_active` con `LEFT JOIN` seguro.

**Certificación:** pruebas de los cuatro puntos; el menú core sigue completo
cuando no hay filas en `modules`; un `wapp` recibe el móvil y no el correo.

### Fase 0 — Fundaciones y activación (módulo apagado)
- Migración: tablas de control y de empresa, `module` 25, `sub_module`,
  `modules` con `is_active = 0`, menús.
- Cliente GOWA (`internal/mensajeria/gowa`): health, info, dispositivos, envío,
  presencia, errores tipados.
- Normalizador `msisdn` con tabla por país.
- Pantalla `/settings/mensajeria`: estado, tres llaves, texto de riesgo,
  proveedores, prueba de conexión.
- Guard 404 y menú oculto.
- Auditoría de cada cambio.

**Certificación:** doble llave + módulo; sin riesgo no enciende; apagado = 404 y
menú oculto; cliente contra un GOWA falso; tabla de normalización; auditoría.

### Fase 1 — Dispositivos, QR y webhook
- Alta/baja, QR (proxy de imagen), código de vinculación, estado, reconexión,
  logout.
- `msg_device` en control con unicidad de número.
- Webhook con HMAC, idempotencia y resolución dispositivo → empresa.
- Pantalla de dispositivos (master).

**Certificación:** firma inválida, dispositivo desconocido, evento duplicado,
aislamiento entre empresas.

### Fase 2 — Cola, plantillas y avisos
- Despachador, ritmo, cupos, horario, cortacircuito, opt-out.
- Plantillas con variables y vista previa.
- Canal `gowa` en `notify_channel`; corrección del destinatario móvil;
  integración con `finance_alert`.
- Resolución de canal por contacto (`msg_contact_channel`).

**Certificación:** normalización, idempotencia, cupos, horarios, prioridad,
opt-out, reintentos, caída del servicio, bloqueo explicado.

### Fase 3 — Chat
- Bandeja, hilo, envío, adjuntos, leído, typing, SSE.
- Vínculo con fichas y notas internas.
- Permisos por dispositivo y auditoría de lectura.

**Certificación:** permisos por rol y dispositivo, aislamiento, adjuntos,
auditoría.

### Fase 4 — OTP
- Retos, envío, verificación, reenvío, recuperación, política por rol,
  enrolamiento, respaldo por correo.
- Integración **solo** en `handleLogin`.

**Certificación:** caducidad, intentos, un solo uso, reenvío, límites, no
enumeración, fallback, POS intacto.

### Fase 5 — Telegram
- `telegram_bot` (avisos), `telegram_gateway` (OTP), `telegram_user` (MTProto
  con QR/código y aceptación de riesgo).
- Capacidades y resolución de canal ya probadas en la fase 2.

**Certificación:** bot no recibe teléfonos; MTProto no se activa sin riesgo
aceptado; Gateway solo emite códigos; aislamiento y auditoría.

### Fase 6 — Métricas y endurecimiento
- Consumo por dispositivo/área, tasa de fallo, bloqueos, alertas al master,
  runbook de baneo, revisión de retención.

---

## 10. Operación (Windows y Linux)

- GOWA se instala como servicio aparte (NSSM/servicio del sistema en Windows,
  systemd en Linux), escuchando **solo en 127.0.0.1**, con Basic Auth aleatoria
  y sin UI. ERPGo lo detecta por URL y hace health check cada 30 s.
- Variables del daemon: `APP_HOST=127.0.0.1`, `APP_PORT=<libre>`,
  `APP_BASIC_AUTH=<aleatorio>`, `APP_UI_ENABLED=false`, `MCP_ENABLED=false`,
  `DB_URI=<data>/mensajeria/whatsapp.db`,
  `CHAT_STORAGE_URI=<data>/mensajeria/chatstorage.db`,
  `WHATSAPP_WEBHOOK_INCLUDE_OUTGOING=true`,
  `WHATSAPP_AUTO_DOWNLOAD_MEDIA=true`, `WHATSAPP_LOG_LEVEL=ERROR`.
- Nunca se expone el puerto del daemon a la red.
- Respaldo: `data/mensajeria/` entra en el respaldo del ERP. Restaurar la sesión
  puede evitar el QR, pero no se garantiza entre versiones.
- Runbook de baneo: pausar, no reintentar, revisar patrón, comunicar, volver con
  cupo mínimo, registrar el incidente.

---

## 11. Riesgos aceptados (van en la pantalla, no en un anexo)

1. GOWA y MTProto **no son APIs oficiales**: el número puede perderse.
2. Se recomienda un número dedicado, nunca el principal de la empresa.
3. No se usarán listas masivas ni mensajes a desconocidos.
4. El OTP por estas vías no sustituye al correo como único factor.
5. La empresa asume el riesgo y designa un responsable.
6. La copia/servicio hay que mantenerlo: si WhatsApp cambia, hay que actualizar.

Sin `risk_accepted_at` no se enciende. Sin las tres llaves, el módulo no existe
para el usuario.
