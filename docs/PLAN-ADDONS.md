# PLAN-ADDONS — El contrato formal para crear módulos desde fuera del binario

## Por qué existe este documento

Este documento responde a una pregunta que ya costó dos fracasos y una semana
de trabajo perdido:

- **El SDK anterior** (retirado en el commit `2cfcc15`) dependía de `ergoaddon`
  **dentro de `go.mod`**: un «addons» era código que se compilaba dentro del
  binario. No había frontera real; agregar capacidades era tocar la fuente.
- **El módulo de GOWA** (eliminado en `26fd927`) tampoco tenía contrato: quedó
  tejido en permisos, menús, migraciones, `server.go`, `invoice.go`,
  `purchase.go` y `legal_books.go` a la vez. Extraerlo fue cirugía, no un
  apagón.

La regla que este documento fija, y que ningún módulo nuevo puede romper:

> **La frontera.** Un addon es un programa separado, con su propio ciclo de
> vida, sus propias vistas y su propio binario, que conversa con el núcleo por
> un contrato explícito. El núcleo jamás se recompila para agregar, quitar o
> actualizar un addon — ni para arreglar la vista de uno.

Este plan no contradice el audit de 2026-10-29
(`doc/informe-auditoria-tecnica-2026-10-29.md`): las reparaciones de jQuery y
vistas siguen su propio registro en
`doc/seguimiento-auditoria-tecnica-2026-10-29.md`, y este contrato las ayuda
impidiendo que el código de un negocio vuelva a contaminar las plantillas del
núcleo.

---

## 1. Qué es un addon y qué no es

**SÍ es:** un ejecutable (`erp-addon-<id>.exe` en Windows, ELF en Linux) que
vive en la carpeta `addons/` junto al binario del ERP, junto a un
`manifest.json`.

**NO es:**

- **No es una librería `.so`/`.dll` cargada por el ERP.** En Go, el paquete
  `plugin` nativo es una trampa: exige compilar addon y núcleo con la misma
  versión de Go y de dependencias, un pánico en el addon derriba **todo el
  ERP**, no se puede descargar, y en Windows ni existe. `LoadLibrary` por CGO
  justifica lo mismo y rompe el build. Por eso la vía elegida es el **proceso
  compañero**: el mismo modelo que usa el ecosistema de plugins de Terraform y
  Vault, probado en producción a escala mundial.
- **No es un módulo con archivos propios** dentro de `internal/`, `templates/`
  o `migrations/` del núcleo.

**Cada addon trae dentro de su binario:** sus templates HTML, sus CSS y JS
(`embed`), su lógica y sus tablas propias. La regla simple: si algo solo sirve
a un negocio, vive en ese negocio.

---

## 2. La carpeta y el manifiesto

```
erpgo.exe
addons/
  cobranza/
    manifest.json
    erp-addon-cobranza.exe
  inventario-avanzado/
    manifest.json
    erp-addon-inventario-avanzado
```

`manifest.json` es el **permiso de trabajo** del addon: dice lo que quiere
hacer y el núcleo decide si se lo concede. Sin manifiesto válido no arranca.

```json
{
  "id": "cobranza",
  "name": "Cobranza WhatsApp",
  "version": "1.0.0",
  "min_core_version": "0.1.0",
  "description": "Recordatorios de cobro por la pasarela de WhatsApp propia",
  "author": "ERPGo",
  "routes": ["/cobranza"],
  "menu": [
    { "parent": "customers", "label": "Cobranza", "icon": "fa-coins",
      "sort_order": 40, "url": "/cobranza", "perms": ["cobranza_view"] }
  ],
  "db_read":  ["customer_information", "invoices"],
  "db_write": [
    { "verb": "invoice.send", "perm": "cobranza_send" }
  ],
  "tables_own": ["cob_mensajes", "cob_reglas"],
  "perms": [
    { "code": "cobranza_view",   "label": "Ver cobranza" },
    { "code": "cobranza_config", "label": "Configurar la pasarela de cobranza" },
    { "code": "cobranza_send",   "label": "Mandar recordatorios" }
  ],
  "hooks_out": ["invoice.emitted", "purchase.registered"],
  "hooks_in":  ["send_whatsapp"]
}
```

**Reglas del manifiesto:**

- `id`: identificador corto (`[a-z0-9][a-z0-9_-]{1,38}`), único. Con él van
  todos los prefijos: la ruta (`/cobranza`), las tablas propias (`cob_*`) y los
  códigos de permiso (`cobranza_*`). Ningún addon puede declarar rutas,
  prefijos de tabla ni permisos que no empiecen con su `id`.
- `min_core_version`: el addon ni se inicia si el núcleo es más viejo que la
  frontera que él exige.
- **Lo que no está declarado no existe para el addon**: sin entradas en `menu`
  no hay menú, sin `db_read` no hay conexión de lectura, sin `db_write` ni
  `tables_own` las escrituras le llegan denegadas, sin `hooks_out` no se le
  notifica nada. El manifiesto es el candado.

---

## 3. Arranque y ciclo de vida

1. **Escaneo.** Al arrancar el ERP —y cada vez que el master pulsa
   «Re-escanear addons» en Configuración → Módulos— el núcleo recorre
   `addons/*/manifest.json`. Valida: esquema del manifiesto, `id` único,
   `min_core_version` cumplido, y que rutas, prefijos de tabla y permisos no
   pisen a otro addon ni al núcleo.
2. **Registro.** Para cada addon válido el núcleo:
   - Crea o actualiza su fila en `modules` (`id = <addon.id>`,
     `is_active` según lo que el master haya guardado antes: el escaneo no
     vuelve a encender lo que el master apagó).
   - Sincroniza sus entradas en `menu` (`module_id = <addon.id>`,
     `perm = permiso.code`). Si el addon desaparece de la carpeta, sus filas
     de `menu` y `modules` se marcan inactivas, **nunca se borran** (política
     de trazabilidad del repo, como en `schema_z_zzzzzz_retiro_addons.sql`).
3. **Lanzamiento.** El núcleo lanza el binario del addon como **proceso hijo
   propio** y le entrega por variables de entorno:
   - `ERP_ADDON_TOKEN`: token efímero de 32 bytes, aleatorio **por sesión**;
     nueva tanda en cada arranque, nunca en disco.
   - `ERP_CORE_URL`: `http://127.0.0.1:<puerto-erp>` (loopback).
   - `ERP_ADDON_PORT`: el puerto local reservado donde el addon debe escuchar.
   - `ERP_DB_READ_DSN`: si el manifiesto pide `db_read`, cadena MySQL de solo
     lectura contra la base de la empresa **activa** de cada sesión (ver §7).
4. **Salud.** Cada 30 s el núcleo hace `GET /healthz` al addon. Dos fallos
   seguidos lo marcan «caído» en la pantalla Módulos, sus rutas responden 502
   con mensaje claro («Cobranza no responde; reinícialo en Configuración →
   Módulos»), y el núcleo reintenta relanzarlo con retroceso exponencial
   (5 s, 10 s, 20 s… tope en 5 min). Un addon muerto **jamás** derriba al ERP:
   solo su pantalla queda fuera de servicio.
5. **Apagado ordenado.** Al cerrar el ERP, el núcleo manda `POST /shutdown`
   del addon y espera hasta 5 s antes de terminar el proceso.

---

## 4. Seguridad: cómo conversan addon y núcleo

Todo el tráfico addon ↔ núcleo va por **loopback** (`127.0.0.1`) y autenticado
con el token efímero del arranque. Son dos canales distintos:

- **Canal de negocio (addon → núcleo).** El addon llama
  `POST /__addon/<id>/<verb>` con el header `X-ERP-Addon-Token`. Solo acepta
  peticiones desde `127.0.0.1` (la entrada `/__addon/` ya reservada en
  `permissions.go`) y solo para los verbs que su manifiesto declara.
- **Canal de navegación (usuario → núcleo → addon).** El núcleo valida la
  **sesión del usuario** en cada petición igual que lo hace para sus propias
  rutas (el addon jamás implementa login ni candados: la sesión es del ERP),
  y antes de reenviar adjunta cabeceras de contexto y retira las cookies del
  ERP para que no circulen hacia el addon:
  - `X-ERP-User`: id del usuario logueado.
  - `X-ERP-Tenant`: código de la empresa activa.
  - `X-ERP-Perms`: los códigos del manifiesto que ese usuario tiene, en CSV
    (`__master` para el super administrador).

**Firma del manifiesto (fase 3).** El manifiesto lleva una firma ed25519
verificada contra una clave pública embebida en el binario. Sin firma válida
no arranca en producción; en desarrollo se permite con `ADDONS_UNSIGNED=dev`
y la pantalla Módulos lo muestra como «sin verificar». El modo seguro de
`scripts/build.sh` compila la clave pública de producción dentro del binario.

---

## 5. Proxy inverso y rutas

**Decisión:** el addon sirve en la ruta que su manifiesto declara
(`/cobranza`), sin prefijos de instalación tipo `/mod/`. El núcleo registra
esa ruta apuntando a su proxy interno (`/cobranza/*` →
`http://127.0.0.1:<puertoAddon>/cobranza/*`), de modo que para el usuario y
para el navegador el addon es una pantalla más del ERP: misma sesión, mismo
menú activo, misma estética de barra lateral.

Revalidación en cada petición: sesión del ERP (del lado del núcleo) y permiso
declarado (el canal de navegación también se puede declarar por entrada de
`menu` con `perms`). Si el addon está caído, el proxy responde 502 con el
mensaje de salud en vez de colgar al usuario.

---

## 6. Menú vía la pantalla Módulos (nada nuevo que inventar)

El addon **es un módulo** — y ese sistema ya existe y ya está probado:

- Fila `modules` con `is_active`, toggle **solo para el master**
  (`TestActivarModulosExigeMasterAunqueElRolTengaConfiguracion` en
  `interfaces/web/correcciones_modulos_test.go`).
- Entradas de `menu` filtradas por `module_id`
  (`TestMenuOcultaUnModuloApagadoYConservaElMenuCore`) con la caché de 30 s
  invalidada por el propio toggle (`TestToggleDeModuloInvalidaLaCacheDelMenu`).

Cero código nuevo de menú. La pantalla existente «Configuración → Módulos»
muestra los addons con dos detalles extra: estado del proceso (sano/caído)
y botón «Reiniciar» que relanza el hijo sin tocar al resto del ERP.

**Filosofía de vistas.** Cada addon trae sus templates, CSS y JS embebidos en
su binario y sirven **solo en sus rutas**. Un addon mal escrito no puede
romper el layout ni el jQuery del resto del sistema: no comparten plantilla
ni bundle. Esta es la frontera que faltaba cuando un arreglo de vistas
tenía al sistema atascado toda la semana.

---

## 7. Base de datos: quién lee, quién escribe

La regla de oro del repo — **una sola puerta de escritura** (el orquestador
`internal/orchestrator/`, con journal y `audit_log` en la misma transacción)
— no se negocia con addons:

- **Lectura directa y granular.** Si el manifiesto declara `db_read`, el
  núcleo crea un usuario MySQL `erp_ro_<id>@localhost` con permiso **solo de
  `SELECT`** sobre las tablas listadas, y le entrega la cadena de conexión
  (`ERP_DB_READ_DSN`) por el arranque, con contraseña distinta en cada
  sesión. Además, la vista es **por empresa activa**: el addon consulta las
  tablas del tenant que la sesión del usuario está usando; no existe para él
  la conexión «de todas las empresas». Consultas del addon para entender el
  negocio del ERP sin credenciales heredadas ni accesos a lo ciego.
- **Escritura solo por el canal de negocio.** Cada entrada de `db_write` es
  una **operación de negocio con nombre y su permiso requerido**, nunca SQL
  libre: el verbo lleva dos partes (`dominio.accion`) y el `perm` citado
  tiene que estar declarado en `perms` de este mismo manifiesto:

  ```json
  "db_write": [ { "verb": "invoice.send", "perm": "cobranza_send" } ]
  ```

  ```
  POST /__addon/cobranza/invoice.send
  { "customer_id": 42, "payload": { "mensaje": "recordatorio de pago" } }
  ```

  El núcleo valida: viene del token, de loopback, y el usuario de la sesión
  tiene el permiso requerido que la entrada de `db_write` exige. Después llama
  al handler de
  negocio correspondiente, que escribe **por el orquestador** (journal +
  `audit_log`). La trazabilidad queda triple: el journal del núcleo, el
  audit_log, y el registro del verb con su id de petición.
- **Tablas propias** (`tables_own`): el addon declara en su carpeta una
  migración SQL por cada tabla `cob_*` que necesita; el núcleo la ejecuta
  contra la base de la empresa activa al arrancar (idempotente, con
  `CREATE TABLE IF NOT EXISTS`, igual que el resto de `migrations/`). Los
  nombres siempre empiezan con el `id` del addon: no puede crear
  `customer_…` ni pisar tablas del núcleo.

---

## 8. Hooks: cómo un addon entra en el flujo de negocio

- **`hooks_out`** («me avisan cuando el núcleo hace algo»). Tras cada evento
  del manifiesto (factura emitida, compra registrada, retención emitida —
  los mismos puntos donde GOWA estaba enganchado a lo bruto), el núcleo
  notifica al addon con `POST /hooks/<evento>` (mismo token, mismo
  `X-ERP-Tenant`). **Sin bloquear la transacción principal**: el hook se
  encola (patrón del runner de facturación digital: cola persistente con
  reintentos) y se entrega *después* del `commit`. Un addon caído o lento
  **jamás** frena una emisión de factura: solo su cola de notificaciones
  espera.
- **`hooks_in`** («el ERP pide que el addon haga algo»). Solo los verbos
  declarados (`send_whatsapp`, etc.), invocados desde handlers de negocio del
  núcleo con `timeout` de 3 s. si el addon no contesta a tiempo, el núcleo
  sigue como si el addon no existiera: se registra en el log y no rompe el
  flujo.

Con `hooks_out` suficiente, **GOWA/Telegram pueden volver algún día** — esta
vez como addons independientes: se desinstalan sin cirugía, su cola y sus
tablas son suyas, y el núcleo ni recuerda que existieron.

---

## 9. Plan por fases

### Fase 1 — La frontera mínima (la primera; todo lo demás la necesita)
- Escáner de `addons/`, validación del manifiesto, lanzamiento del proceso
  hijo con token efímero y puerto reservado.
- Proxy inverso de las rutas del manifiesto con validación de sesión del
  núcleo y cabeceras `X-ERP-User` / `X-ERP-Tenant` / `X-ERP-Perms`.
- Registro de menú vía `modules` + `menu` (`module_id` del addon) y la
  pantalla Módulos existente: toggle, «Re-escanear addons», «Reiniciar» y
  estado de salud del proceso.
- Canal de negocio `/__addon/<id>/<verb>` sin declarar verbs de escritura
  todavía (llega en fase 2): la frontera ya respira sin tocar datos del ERP.
- **Addon demo** en el repo (`addons/hola/`), mínimo: una pantalla, una
  entrada de menú y una tabla propia, que acredite el ciclo completo; su
  prueba `*_test.go` en el repo y todo en verde antes de cerrar.

### Fase 2 — Datos y escrituras
- Conexión MySQL de solo lectura granular (`db_read`) por empresa activa,
  con usuario renovado por sesión.
- Verbs de escritura en `/__addon/` a través del orquestador, cada uno con su
  handler de negocio y su prueba.
- Migraciones de tablas propias en el arranque, nombres prefijados por `id`.

### Fase 3 — Confianza y reactividad
- Firma ed25519 del manifiesto y verificación en el arranque; modo
  `ADDONS_UNSIGNED` solo para desarrollo, con la clara indicación «sin
  verificar» en la pantalla Módulos.
- `hooks_out` con cola persistente y reintentos; `hooks_in` con timeout y
  otro camino si el addon no responde.
- Pantalla Módulos: historia de hooks entregados, versión instalada de cada
  addon y botón de reasignación de puertos.

---

## 10. Qué NO hace el sistema de addons

Tan importante como lo anterior, para que nadie le pida a la frontera lo que
no puede dar:

- **No implementa lógica de negocio de un addon en el núcleo.** Si algo solo
  sirve a la Cobranza, vive en Cobranza. El núcleo solo implementa la
  frontera, una vez, con sus pruebas.
- **No comparte vistas ni bundle.** Un addon no puede romper el layout, el
  menú ni el jQuery del resto del sistema, porque no toca esos archivos.
- **No da acceso a la BD del núcleo como rey.** El addon recibe una vista
  limitada por empresa (y solo con `SELECT`) o verbos de negocio; SQL libre
  contra las tablas del ERP **no existe** en el contrato.
- **No pide confiar en el addon.** Todo permiso viene del manifiesto, todo
  acceso va autenticado por token y, en fase 3, todo addon está firmado. La
  confianza no es una variable política: es un check de código.

---

## 11. Cómo validamos el contrato (criterios de cierre de fase)

- El addon demo arranca, se ve en el menú, se apaga/enciende desde
  Configuración → Módulos con el toggle de master, y al matar su proceso el
  ERP sigue vivo (solo su ruta responde 502 con mensaje claro).
- Al quitar la carpeta del addon y re-escanear, queda «no instalado»: sus
  rutas dejan de proxyar y sus filas de `menu` y `modules` se marcan inactivas
  (nunca borradas).
- Un intento de escritura fuera de los verbs del manifiesto falla con 403 y
  queda registrado en el log del canal de negocio.
- Una ruta del addon sin sesión del ERP redirige a login, igual que cualquier
  ruta del núcleo; una petición al canal de negocio sin el token efímero y
  fuera de loopback también rechaza (403) con prueba.
- Todo lo anterior con pruebas en el patrón `*_test.go` del repo, en español,
  con comentarios que expliquen el POR QUÉ, y en verde antes de cada cierre
  (`go build ./...`, `go vet ./interfaces/web`, `go test ./...`).
