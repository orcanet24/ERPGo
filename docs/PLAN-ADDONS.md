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
     `perm = permiso.code`), con la URL **normalizada a la puerta estable
     `/mod/<id>`** (las filas viejas que apuntaban a `/hola` se mueven, no se
     duplican). Si el addon desaparece de la carpeta, sus filas de `menu` y
     `modules` se marcan inactivas, **nunca se borran** (política de
     trazabilidad del repo, como en `schema_z_zzzzzz_retiro_addons.sql`).
   - Replica el registro en **todas las empresas** del catálogo, no solo en la
     activa: sin esto las demás se quedarían sin fila de módulo y su menú
     desaparecería. Los grants de lectura de cada empresa se agregan con la
     misma clave del usuario lector.
3. **Lanzamiento.** El núcleo lanza el binario del addon como **proceso hijo
   propio** y le entrega por variables de entorno:
   - `ERP_ADDON_TOKEN`: token efímero de 32 bytes, aleatorio **por sesión**;
     nueva tanda en cada arranque, nunca en disco.
   - `ERP_CORE_URL`: `http://127.0.0.1:<puerto-erp>` (loopback).
   - `ERP_ADDON_PORT`: el puerto local reservado donde el addon debe escuchar.
   - `ERP_DB_READ_DSN`: si el manifiesto pide `db_read`, cadena MySQL de solo
     lectura contra la base de la empresa **activa** de cada sesión (ver §7).

   El proceso es **único para todo el ERP** y solo vive si el addon está
   encendido en ALGUNA empresa: apagado en todas, no se lanza (y si estaba
   vivo, se detiene). El interruptor de Configuración → Módulos enciende y
   apaga el proceso sin reiniciar el ERP; la visibilidad por empresa sigue
   decidiéndose en cada petición.
4. **Salud.** Cada 30 s el núcleo hace `GET /healthz` al addon. Dos fallos
   seguidos lo marcan «caído» en la pantalla Módulos, sus rutas responden 502
   con mensaje claro («Cobranza no responde; reinícialo en Configuración →
   Módulos»), y el núcleo reintenta relanzarlo con retroceso exponencial
   (5 s, 10 s, 20 s… tope en 5 min). Un addon muerto **jamás** derriba al ERP:
   solo su pantalla queda fuera de servicio.
5. **Apagado ordenado.** Al cerrar el ERP, el núcleo manda `POST /shutdown`
   del addon y espera hasta 5 s antes de terminar el proceso.

### 3.1 Cómo compilar el binario del addon (y qué acepta el núcleo en Windows)

El nombre manda: el escáner busca en la carpeta del addon un binario llamado
`erp-addon-<id>` (así lo trae `addons/hola` del repo, con la id del
manifiesto). Los candidatos van **primero con `.exe` y después sin
extensión**: la instalación canónica en Windows es `erp-addon-<id>.exe`.

**Compilación correcta en cada plataforma** (desde la raíz del repo):

- En **Windows** (binario `.exe`, la forma normal de instalar):

  ```bat
  go build -o .\addons\hola\erp-addon-hola.exe .\addons\hola
  ```

  Nunca sueltes ahí un `go build` SIN `-o`: Go nombra el producto con el
  nombre de la carpeta (`hola.exe`), no con el que busca la frontera
  (`erp-addon-hola.exe`), y el escáner diría «no hay binario compilado»
  aunque la carpeta esté llena.
- En **Linux** (ELF, la forma normal de instalar):

  ```sh
  go build -o ./addons/hola/erp-addon-hola ./addons/hola
  ```

**Qué acepta la frontera en Windows.** La prueba de «¿esto es un binario
lanzable?» NO consulta bits de ejecución: en Windows `os.Stat` no los reporta (los archivos regulares salen con
`0666`), y exigirlos dejaría fuera
TODOS los binarios, con o sin `.exe`. Por eso en Windows un archivo regular
que lleva el nombre del binario (`erp-addon-<id>.exe` o, como candidatos
secundarios, `erp-addon-<id>`) cuenta como lanzable; en POSIX la prueba de
bits de ejecución sigue viva. El detalle está en `buscarBinario`
(interfaces/web/addons.go); si no encuentra nada, el log del arranque lo
dice con la ruta real que miró.

**Lanzamiento directo de PEs sin extensión.** Aunque el SO corra el PE,
`exec.Cmd.Start` en Windows SIEMPRE resuelve extensiones (`lookExtensions`
del runtime, en `src/os/exec/exec.go`): un binario pelado sin `.exe` jamás
se lanzaría por ese camino. Consecuencia probada empíricamente en este
repo: `os.StartProcess` (CreateProcess pelado) sí corre el PE sin extensión
(código de salida 0 con el proceso real al aire). Por eso el lanzamiento
REAL del núcleo (`lanzarReal`) no usa `exec.Command`: arranca el proceso
hijo con `os.StartProcess` (con el `Dir` de la carpeta del addon y el `Env`
con token, puerto y DSN) y lo envuelve en un `*exec.Cmd` construido a mano,
de modo que todo el contrato `p.cmd.Process` que usan el supervisor y los
handlers siga intacto (nadie llama `Wait` a esos procesos: mueren por
`Process.Kill` o se apagan solos). El resultado práctico: en Windows un binario compilado sin `.exe` arranca
igual que uno con `.exe`; la forma
recomendada sigue siendo `.exe`, pero el nombre `erp-addon-<id>` es lo que
el escáner exige y no el sufijo.

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
  - `X-ERP-DB`: nombre de la **base** de esa empresa activa. Es la otra mitad
    de la lectura granular (§7): sin ella el addon recibe un DSN y unos
    grants pero no sabe en qué base están las filas de ESTA sesión, y el
    mismo addon atiende a varias empresas. El nombre de base no es un
    secreto: sin el `SELECT` concedido, entrar en ella no deja leer nada.
  - `X-ERP-Auth-DB`: nombre de la **base de identidad** (la de control), donde
    viven `users` y `user_login`. Los grants del lector ya la cubren
    (`crearUsuarioLectorAddon` empieza por `DB_NAME`); lo que faltaba era el
    nombre. Sin él, un módulo no puede mostrar a la gente de la empresa ni
    compartir con alguien: su propia tabla no guarda el nombre de nadie.
  - `X-ERP-CSRF`: el token CSRF de la sesión, para que la página del addon lo
    ponga en su formulario (`<input name="csrf_token">`). **El candado lo
    aplica el núcleo**, antes de reenviar: `enforceCSRF` exige token en todo
    POST que tenga sesión, y las pantallas de addon no llevan el layout que
    inyecta el token. El addon no valida CSRF, solo recibe la petición ya
    pasada por el filtro.
  - `X-ERP-Perms`: los códigos del manifiesto que ese usuario tiene, en CSV
    (`__master` para el super administrador).

**Firma del manifiesto (fase 3).** El manifiesto lleva una firma ed25519
verificada contra la clave pública del dueño, guardada en `addons/pubkey.ed25519`
(hex) y leída por el núcleo en el arranque. Con clave configurada, un addon sin
`manifest.sig` no carga salvo en modo de desarrollo (`ADDONS_UNSIGNED=1`, marca
«sin verificar»); una firma PRESENTE e inválida no carga en ningún modo. El
ciclo completo — generar la pareja, firmar, rotar la clave y el modo sin firma
— está en §4.1. Nadie compila la clave dentro del binario: la frontera la lee
del disco en cada arranque, así que cambiarla no exige recompilar el ERP.

### 4.1 El ciclo de firma: `tools/firmaraddon`, la rotación de `pubkey.ed25519` y `ADDONS_UNSIGNED`

La firma se genera y se aplica con `tools/firmaraddon` (la herramienta del
dueño) y se verifica en `interfaces/web/addons.go` con la política del
núcleo (`addonClavePublica` y `verificarFirmaAddon`). El ciclo completo de
una instalación:

1. **Generar la pareja (una vez):**

   ```sh
   go run ./tools/firmaraddon clave
   ```

   Deja la clave PRIVADA en `firmaraddon.key` del directorio corriente
   (hex, permiso 0600) e imprime por pantalla la pública, que es la que
   debe acabar en `<raíz>/addons/pubkey.ed25519`. La privada es la firma
   personal del dueño: JAMÁS al repositorio ni a la carpeta de addons
   (ahí solo vive la pública). Si no se quiere depender de `firmaraddon.key`
   en el directorio, las órdenes de firmar la toman de la variable de
   entorno `FIRMARADDON_KEY`, práctica habitual para firmar desde otro
   equipo o desde CI.
2. **Firmar cada addon:**

   ```sh
   go run ./tools/firmaraddon firmar addons/<id>
   ```

   Firma los bytes EXACTOS del `manifest.json` de la carpeta con la privada
   y escribe el resultado (hex) en `addons/<id>/manifest.sig`. La regla de
   oro es firmar cuando el manifiesto ya es definitivo: si después se
   cambia una coma del `manifest.json`, la firma deja de corresponder, el
   núcleo rechaza el addon con «la firma NO corresponde al manifest.json
   actual» y toca firmar de nuevo.
3. **Verificar fuera del ERP (opcional, para el dueño):**

   ```sh
   go run ./tools/firmaraddon verificar addons/<id>
   ```

   Reproduce la MISMA política del núcleo: lee la pública del directorio
   padre de la carpeta, exige `manifest.sig` y verifica. Sirve para saber
   si la firma está bien antes de que el ERP lo diga; sin clave pública en
   el disco, avisa que el núcleo correría «sin verificar», y con firma
   mala, dice exactamente por qué.

El núcleo no inventa nada aparte: en cada arranque y en cada «Re-escanear
addons» (`addonsIniciar` / `escanear`) lee la pública de
`addons/pubkey.ed25519` y verifica manifiesto y `manifest.sig` de cada addon.

**Los tres estados de la firma, todos fail-closed** (`verificarFirmaAddon`):

- **Sin `addons/pubkey.ed25519`:** no hay verificación posible y todo corre
  «sin verificar»; el log lo avisa y el panel Módulos muestra el estado en
  cada addon. No es un candado falso: es un estado visible y declarado.
- **Con clave, addon sin `manifest.sig`:** no carga, salvo en modo de
  desarrollo (`ADDONS_UNSIGNED=1`), donde carga con la marca «sin
  verificar».
- **Con clave y firma PRESENTE pero inválida** (el manifiesto cambió después
  de firmar, el hex de `manifest.sig` está corrupto, la firma es de otra
  clave): NO carga en ningún modo, tampoco con `ADDONS_UNSIGNED=1`. El
  desarrollo perdona la AUSENCIA de firma, porque nadie está iterando con
  el aparato de firma montado; jamás perdona el DAÑO: una firma que no
  corresponde significa que el manifiesto se manipuló después de firmar.

La política completa tiene su prueba en `TestLaFirmaDelManifiestoDecideQuienCarga`
y la lectura de la clave del disco (lo que corre en el arranque real) en
`TestLaClavePublicaDelDiscoDecideLaVerificacion`.

**Rotación de `pubkey.ed25519`.** Cambiar de pareja (pérdida sospechada de
la privada, cambio de responsable, caducidad) es un procedimiento de cinco
pasos:

1. Guardar la privada VIEJA en un lugar seguro fuera de la instalación: es
   lo único que permite revertir la rotación si el juego nuevo falla.
2. `go run ./tools/firmaraddon clave`: genera la pareja NUEVA y SOBREESCRIBE
   `firmaraddon.key` del directorio corriente; a partir de ese momento, esa
   carpeta firma con la nueva pareja.
3. Sustituir `addons/pubkey.ed25519` por la pública nueva que imprimió el
   paso 2.
4. Re-firmar TODOS los addons con la nueva privada:
   `go run ./tools/firmaraddon firmar addons/<id>` para cada uno. Con la
   pública nueva, las firmas de la clave retirada pasan a ser «firma
   inválida»; re-firmar todo es lo que deja la instalación consistente,
   sin addons firmados por la clave que se retira.
5. **Reiniciar el ERP.**

El reinicio no es descuido: el núcleo lee `pubkey.ed25519` UNA sola vez, en
el arranque (`addonClavePublica` desde `addonsIniciar`), y de ahí en adelante
esa es la clave de todo el proceso. Cambiar el archivo con el ERP en pie NO
cambia la verificación en caliente: con la clave vieja aún residente, los
addons re-firmados con la nueva darían «firma no corresponde» y los firmados
con la vieja seguirían pasando, un estado mezclado en la misma sesión. El
reinicio aplica el juego completo (pública y firmas) como bloque. Revertir
la rotación es la misma receta al revés: reponer la pública vieja, re-firmar
con la privada vieja y reiniciar.

**`ADDONS_UNSIGNED=1` es el modo SOLO de desarrollo.** La variable de entorno
se lee al arranque junto con la clave y cambia una sola regla: con la clave
pública configurada, un addon SIN `manifest.sig` carga marcado «sin
verificar» en lugar de quedar fuera. Sirve para iterar un addon nuevo sin
montar el ciclo de firma en cada cambio. No resucita firmas inválidas, no
trata la clave ausente como presente y no cambia nada en producción: la
instalación real del dueño va con `pubkey.ed25519` bien puesto y todos los
addons firmados, y el log deja constancia cuando el modo está activo.

---

## 5. Proxy inverso y rutas

**Decisión:** el addon declara sus rutas (`/cobranza`) y el núcleo expone una
**puerta estable `/mod/<id>`** que despacha por id. El menú apunta a esa
puerta, y `/mod/<id>/resto` se reenvía al proceso como `routes[0]/resto`:

```
/memo        → /mod/memo  → http://127.0.0.1:<puertoAddon>/memo
/memo/nota   → /mod/memo/nota → http://127.0.0.1:<puertoAddon>/memo/nota
```

El núcleo registra `/mod/` **una sola vez, al arrancar**, y el handler resuelve
el addon en caliente: un addon instalado después (re-escaneo) se sirve sin
reiniciar el ERP. Las rutas declaradas de los addons presentes al arrancar se
registran además como **alias heredado** (`/cobranza`), para que los enlaces
internos y los deep-links viejos sigan funcionando.

Para el usuario y para el navegador el addon es una pantalla más del ERP:
misma sesión, mismo menú activo, misma estética de barra lateral. Las cookies
del ERP nunca llegan al proceso.

Revalidación en cada petición: sesión del ERP (del lado del núcleo), empresa
activa (`is_active` de `modules`) y permiso declarado (el canal de navegación
también se puede declarar por entrada de `menu` con `perms`). Si el addon está
caído, el proxy responde 502 con el mensaje de salud en vez de colgar al
usuario.

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
  `SELECT`** sobre las tablas listadas, y le entrega la cadena de  conexión (`ERP_DB_READ_DSN`) por el arranque. Ese DSN viene **sin base por
  defecto**: el addon abre cada consulta sobre la base que le indica la
  cabecera `X-ERP-DB` de la petición que está atendiendo (§4), de modo que la
  vista es **por empresa activa**; no existe para él la conexión «de todas las
  empresas». Consultas del addon para entender el negocio del ERP sin
  credenciales heredadas ni accesos a lo ciego.

  Un addon puede declarar en `db_read` **también sus propias tablas** (`demo_avisos`,
  `memo_notas`…): los grants SELECT se aplican a las tablas declaradas sin
  preguntar de quién son, y sin esa entrada el addon no podría LEER lo que él
  mismo escribió por el canal para pintarlo en su pantalla. Es la lectura de
  su espejo, no una ampliación de poder: solo alcanza a lo que ya es suyo.
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

### Fase 1 — La frontera mínima (la primera; todo lo demás la necesita) ✅

> **Estado (2026-10-30): IMPLEMENTADA y probada.** El escáner, el proxy con
> cabeceras de contexto, el registro en `modules`/`menu`, el canal de negocio
> con token y loopback, y el addon demo (`addons/hola/`) viven en
> `interfaces/web/addons.go` con sus pruebas en `addons_test.go`.
>
> **Cierre 2026-11-26 (fase 0 + fase 1 estables).** Se cerraron los huecos que
> impedían llamarla terminada: supervisión con relanzamiento automático,
> arranque solo si el addon está encendido en alguna empresa, interruptor que
> enciende y apaga el proceso, registro replicado en todas las empresas y
> puerta `/mod/<id>` registrada una sola vez (addon nuevo en caliente, alias
> heredado intacto).
- Escáner de `addons/`, validación del manifiesto, lanzamiento del proceso
  hijo con token efímero y puerto reservado.
- Proxy inverso con **puerta estable `/mod/<id>`** (registrada una vez) y las
  rutas declaradas como alias heredado; validación de sesión del núcleo,
  empresa activa y cabeceras `X-ERP-User` / `X-ERP-Tenant` / `X-ERP-Perms`.
- Registro de menú vía `modules` + `menu` (`module_id` del addon), replicado
  en todas las empresas, con la URL normalizada a `/mod/<id>` y la pantalla
  Módulos existente: toggle, «Re-escanear addons», «Reiniciar» y estado de
  salud del proceso.
- Supervisión real: dos fallos de `/healthz` marcan «caído» y el vigil
  relanza con retroceso exponencial; el interruptor del módulo detiene el
  proceso cuando queda apagado en todas las empresas.
- Canal de negocio `/__addon/<id>/<verb>` sin declarar verbs de escritura
  todavía (llega en fase 2): la frontera ya respira sin tocar datos del ERP.
- **Addon demo** en el repo (`addons/hola/`), mínimo: una pantalla, una
  entrada de menú y una tabla propia, que acredite el ciclo completo; su
  prueba `*_test.go` en el repo y todo en verde antes de cerrar.

### Fase 2 — Datos y escrituras ✅

> **Estado (2026-10-30): IMPLEMENTADA y probada.** Conexión de solo lectura
> granular (usuario `erp_ro_<id>`, clave nueva por sesión y cabecera
> `X-ERP-DB` para elegir la base), verbos de escritura por el orquestador y
> migraciones de tablas propias con `CREATE TABLE IF NOT EXISTS`.
- Conexión MySQL de solo lectura granular (`db_read`) por empresa activa,
  con usuario renovado por sesión.
- Verbs de escritura en `/__addon/` a través del orquestador, cada uno con su
  handler de negocio y su prueba.
- Migraciones de tablas propias en el arranque, nombres prefijados por `id`.

### Fase 3 — Confianza y reactividad (la frontera ya corre con ella)
- Firma ed25519 del manifiesto y verificación en el arranque. La clave
  pública del dueño vive en `addons/pubkey.ed25519`; con ella configurada la
  firma es obligatoria, `ADDONS_UNSIGNED=1` es el modo SOLO de desarrollo
  (los sin firma cargan con la marca «sin verificar»), y una firma presente
  e inválida no carga en NINGÚN modo: significa que el manifiesto cambió
  después de firmar. La pareja se genera y se usan con `tools/firmaraddon`
  (`clave` / `firmar addons/<id>` / `verificar`); la privada jamás al repo.
- `hooks_out` con cola persistente (`addon_hook_out` en la BD de control,
  esquema portable) y reintentos: el núcleo encola tras el commit (hoy el
  punto real es `factura.emitida`, en el registro de factura definitiva) y un
  runner cada 30 s entrega `POST /hooks/<evento>` con el token de ESTA sesión
  y `X-ERP-Tenant`. Backoff 1m→30m, tope 8 intentos; la fila queda 'sent',
  'failed' (con su motivo) o 'descartado' si el addon ya no está — jamás un
  martilleo infinito y jamás se frena la factura que originó el aviso.
- `hooks_in` con timeout de 3 s y otro camino: `Server.addonHookPedir(id,
  hook, empresa, payload, tope)` exige el hook declarado en `hooks_in`, y si
  el addon cae o tarda, el error va explicado y el flujo de negocio sigue sin
  él. Los puntos de negocio reales que lo llaman llegan con los addons que
  piden (p. ej. el regreso de WhatsApp como addon).
- Pantalla Módulos (fase 3): estado de firma por addon (con aviso global si
  falta la clave pública), suscripciones hooks_out / ofertas hooks_in,
  historia de hooks entregados con intentos y motivos, versión instalada de
  cada addon y botón «Reasignar puertos» (relanza cada addon en puerto
  loopback nuevo sin reiniciar el ERP).
- Criterios de cierre: pruebas nuevas `TestElManifiestoFase3ValidaHooks`,
  `TestLaFirmaDelManifiestoDecideQuienCarga`,
  `TestLosHooksOutSeEncolanYSeEntreganConReintentos`,
  `TestElHookInPideConPlazoYSigueSinElAddon`,
  `TestElPanelFase3MuestraFirmaHooksYHistoria` y
  `TestElBotonDePuertosRelanzaCadaAddon`; el demo `addons/hola` (v3.0.0)
  suscribe `factura.emitida` y la recibe de verdad a través del binario real.

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
