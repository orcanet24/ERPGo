# Diseño del instalador — Windows y Linux

> **Qué es este documento.** El análisis y el diseño de un instalador *simple,
> funcional y robusto* para ERPGo en Windows y Linux, con las tres formas de
> presentación que se pidieron: **ventana propia**, **abrir la terminal** y
> **terminal propia**. **No se implementa nada aquí**: es material de estudio
> para decidir después.
>
> **Qué manda sobre qué.** El proyecto ya tiene decisiones firmes sobre
> instalación (el motor de base de datos no es nuestro, los datos existentes no
> se tocan, el cliente reemplaza el binario y las migraciones corren solas).
> Este diseño **respeta esas decisiones** y solo propone la capa de empaquetado y
> arranque que falta. Donde propone tocar código, lo dice explícitamente y con
> el motivo.
>
> Documentos que este reemplaza como referencia de diseño:
> [INSTALADOR.md](INSTALADOR.md) (qué hace el `instalar.bat` de hoy),
> [instalador-windows.md](instalador-windows.md) (bosquejo con Inno Setup) y
> [PLAN-INSTALADOR-ERPGO-POS.md](PLAN-INSTALADOR-ERPGO-POS.md) (plan del wizard).
> Se conservan como historia; **lo que contradiga este documento queda anulado**,
> y lo que contradiga el código, manda el código.

---

## 1. Lo que YA existe (inventario verificado)

Antes de diseñar hay que saber qué está hecho, porque **el 70% del instalador ya
está escrito y no lo parece**.

| Pieza | Dónde | Estado |
|---|---|---|
| Asistente de instalación interactivo (6 pasos, aviso legal, reintento de credenciales) | `erpgow --setup` → [internal/setup/wizard.go](../internal/setup/wizard.go) | **Funciona** |
| Aplicar esquema y salir | `erpgow --migrate` → [cmd/server/main.go:80](../cmd/server/main.go#L80) | **Funciona** |
| Servicio de Windows (SCM nativo, sin `sc.exe`, con recuperación ante fallos) | `erpgow --install` / `--uninstall` → [internal/service/manage_windows.go](../internal/service/manage_windows.go) | **Funciona en Windows**; en Linux devuelve error ([manage_other.go](../internal/service/manage_other.go)) |
| Protocolo de servicio (evita el Error 1053) | [internal/service/service_windows.go](../internal/service/service_windows.go) | **Funciona** |
| Nombre del servicio | `ERPGoServer` en [service_windows.go:14](../internal/service/service_windows.go#L14) | Constante, con comentario que dice que **debe coincidir con el instalador** |
| Esquema, plantillas y estáticos embebidos en el binario | [embed.go](../embed.go) | **Funciona** |
| Comprobación de la base al arrancar (falla con mensaje claro si el motor no está) | [cmd/server/main.go:152](../cmd/server/main.go#L152) | **Funciona** |
| Migraciones idempotentes + checksum, aplicadas al arrancar | `migrations/` | **Funciona** |
| Salud para verificar el arranque | `/health` → [interfaces/web/server.go:333](../interfaces/web/server.go#L333) | **Funciona** |
| Respaldo automático diario | `BACKUP_ENABLED=1` por defecto, [interfaces/web/backup_scheduler.go](../interfaces/web/backup_scheduler.go) | **Funciona si hay `mysqldump`** |
| Actualización desde Git con verificación de salud | [scripts/deploy/](../scripts/deploy/) | **Funciona** (cron/webhook + systemd/nssm) |
| Compilación ofuscada para entrega | [scripts/build.bat](../scripts/build.bat), [scripts/build.sh](../scripts/build.sh) | **Funciona** |
| Hoja de estilo e imágenes de instalador | `static/installer/` (Bootstrap + `installer.css` + iconos) | **Presentes y sin usar**: ningún `.go` las referencia |
| Pantalla web de alta de empresa | `templates/setup_company.html` vía `renderStandalone` → [interfaces/web/tenants.go:1253](../interfaces/web/tenants.go#L1253) | **Funciona**: es la semilla de una UI web de instalación |

**Lo que NO existe** (y es lo que hay que diseñar):

1. Un paquete ejecutable que instale en Windows y en Linux (hoy hay un `.bat`
   que **no está en el repositorio**: la raíz no tiene ningún `.bat`, ver §7.4).
2. Instalación como servicio en **Linux** (no hay unidad systemd en el binario).
3. Modo **no interactivo** del asistente (hoy lee de `stdin` con `bufio`).
4. **Versión** del producto visible para el instalador (§3.3).
5. Cualquier **registro escrito a archivo** durante la instalación (§3.5).
6. Rutas de datos **explícitas** en lugar de relativas al directorio actual (§3.1).

---

## 2. La decisión que manda sobre todo lo demás

**ERPGo no instala el motor de base de datos.** Está escrito tres veces, en tres
lugares que se sostienen entre sí:

- [README.md](../README.md): *"El motor de base de datos NO viene con ERPGo."*
- [cmd/server/main.go:152](../cmd/server/main.go#L152): si no hay nada
  escuchando en `DB_HOST:DB_PORT`, el proceso **se detiene sin tocar el sistema**.
- [internal/setup/wizard.go:215](../internal/setup/wizard.go#L215): el asistente
  **ya no** ofrece instalar MariaDB, y el comentario explica por qué (rompió
  servidores sanos al cambiar paquetes, servicios y el datadir del anfitrión).

Consecuencias para el instalador, que son **requisitos, no opiniones**:

1. El instalador **no** ejecuta `apt-get`, `dnf`, `yum`, `choco`, `winget`, ni
   descarga un MSI. No modifica el sistema operativo anfitrión.
2. Su primer paso real es **comprobar** que el motor responde, y su mensaje de
   fallo tiene que decir qué hacer (instalar MySQL 8 o MariaDB 10.6+, arrancarlo
   como servicio, revisar `DB_HOST`/`DB_PORT`, revisar permisos).
3. Como el motor es del cliente, el instalador **nunca** puede prometer "todo en
   un paso". El contrato honesto es: *"si su servidor ya tiene MySQL/MariaDB
   arrancado, esto son 4 pantallas; si no, instálelo primero (10 minutos) y
   vuelva"*.

Esto último no es un defecto: es lo que hace que el sistema no rompa servidores.
Pero el instalador debe **decirlo antes de empezar**, no al final.

---

## 3. Los cinco hallazgos que cambian el diseño

Estos salen de leer el código, no de suponer. Los tres primeros obligan a tocar
Go; los dos últimos son de empaquetado.

### 3.1 Rutas relativas al directorio actual — el riesgo número uno

Hoy casi todo se resuelve contra el **directorio de trabajo** (`CWD`), no contra
el directorio del ejecutable:

| Qué | Dónde se resuelve | Ruta |
|---|---|---|
| `.env` | `godotenv.Load(".env")` → [configs/config.go:76](../configs/config.go#L76) | relativa al CWD |
| `.env` (escritura) | `os.WriteFile(".env", …)` → [configs/config.go](../configs/config.go) | relativa al CWD |
| Respaldos | `"backup"` → [interfaces/web/database_settings.go:48](../interfaces/web/database_settings.go#L48) | relativa al CWD |
| Respaldos de la pantalla de respaldo | `filepath.Join("data","backup", …)` → [interfaces/web/setting.go:330](../interfaces/web/setting.go#L330) | relativa al CWD |
| Journal del orquestador de escrituras | `data/escrituras/<empresa>.journal` → [interfaces/web/orchestrator.go:9](../interfaces/web/orchestrator.go#L9) | relativa al CWD |
| Cargas de archivos | `UPLOAD_PATH` por defecto `./uploads` | relativa al CWD |
| 📌 **Log del servicio** | `filepath.Dir(exe)` → [cmd/server/main.go:374](../cmd/server/main.go#L374) | **relativa al exe (correcto)** |
| 📌 **Plantillas y estáticos (respaldo en disco)** | `resolvePath()` prueba exe, CWD y 3 niveles arriba → [cmd/server/main.go:32](../cmd/server/main.go#L32) | **tolera las dos** (por eso nunca saltó el problema) |

**El problema aparece justo en el modo que más se va a usar.** Un servicio de
Windows arranca con directorio de trabajo `C:\Windows\System32`; y
`manage_windows.go` registra el servicio **sin directorio de trabajo**
(`m.CreateService(name, exePath, cfg)` no lo admite: el SCM no tiene ese
concepto). Medido en el código: no hay ninguna línea que fije el CWD.

Qué pasaría hoy con `erpgow.exe --install` y reinicio del equipo:

- El servicio no encuentra `.env` (está en `C:\Program Files\ERPGo`), así que
  `EnsureEnv()` **crea uno nuevo en `C:\Windows\System32`** con los valores por
  defecto: apuntando a `DB_NAME=erpgo`, `DB_USER=root`, sin contraseña.
- Los respaldos, el journal y las cargas se escriben en `System32`.
- El ERP arranca contra la base equivocada o se detiene con "no hay MySQL en
  127.0.0.1:3306" — y el mensaje no menciona que el problema es el CWD.

**Diseño propuesto (dos capas, la segunda es la de verdad):**

- **Parche inmediato (una línea):** al arrancar como servicio, fijar el CWD al
  directorio del ejecutable — `os.Chdir(filepath.Dir(exePath))` — que es lo que
  hacen los servicios de Windows cuando no quieren sorpresas. Va en
  `main.go`, justo antes de `configs.EnsureEnv()`.
- **Solución real (la que hay que estudiar):** que **ninguna ruta dependa del
  CWD**. Introducir una raíz de datos explícita, resuelta así:
  `--data-dir` (línea de comandos) → `ERPGO_DATA_DIR` (entorno) →
  directorio del ejecutable (por defecto). Y que de ahí cuelguen `.env`,
  `data/backup`, `data/escrituras`, `uploads`, `logs` y `backup`. Es un cambio
  acotado (los sitios están en la tabla de arriba: siete puntos) y es lo que
  permite separar programa y datos (§5.1), que es la base de una actualización
  limpia.

> Regla que conviene adoptar y escribir en el código: **ninguna ruta de datos se
> resuelve contra el CWD; el CWD es una casualidad del lanzador, no una decisión
> del sistema.**

### 3.2 El asistente necesita un terminal, y el instalador nativo no lo tiene

`runFreshInstall()` lee con `bufio.NewReader(os.Stdin)`
([wizard.go:29](../internal/setup/wizard.go#L29)). Eso funciona con doble clic en
la consola de Windows y desde una terminal de Linux. **No funciona** cuando el
asistente lo lanza un instalador gráfico (Inno Setup): el instalador no es una
consola, no hay `stdin` que leer, y el resultado es un cuelgue o un error
incomprensible.

Y hay un segundo detalle: si el proceso se lanza **sin consola** (por ejemplo con
`-H=windowsgui`, que hoy **no** se usa) el asistente no puede preguntar nada.

**Diseño propuesto:** el asistente gana un **modo no interactivo** y una
**abstracción de preguntas**:

```
erpgow --setup                       # interactivo, terminal (lo de hoy)
erpgow --setup --answer-file=a.json  # no interactivo, sin stdin
erpgow --setup --ui=web              # servidor local + navegador (§4.2)
erpgow --setup --log=...             # registro a archivo (§3.5)
erpgow --setup --dry-run             # valida todo y no escribe nada
```

Las preguntas del wizard pasan por una interfaz con tres implementaciones
(consola, archivo de respuestas, web). **La lógica de instalación no se
duplica:** es la misma y ya existe. Esto es lo que hace que el instalador nativo
de Windows, el script de Linux y la ventana propia sean **el mismo programa con
tres puertas**, que es exactamente el objetivo de "simple y robusto".

### 3.3 No hay versión de producto — y el instalador la necesita

`grep` de `version`/`Version` en `cmd/server/`: **cero coincidencias**. Sin
versión embebida:

- El instalador no puede decidir *instalar*, *reparar* o *actualizar*.
- No puede negarse a un **downgrade** (binario viejo sobre esquema nuevo).
- Soporte no puede preguntar "¿qué versión tienes?" y obtener una respuesta.

**Diseño propuesto:** `-ldflags "-X main.buildVersion=x.y.z -X main.buildCommit=... -X main.buildDate=..."`
en los scripts de compilación (que ya usan `-ldflags`, así que no cuesta nada),
y un `erpgow --version` que además imprima: versión, commit, fecha, motor de base
detectado y versión del motor, y si el sello de integridad es válido. Ese comando
es el que el instalador usa para el paso "detectar instalación existente".

### 3.4 En un arranque en frío, el `.env` queda de una línea y con permisos 0644

Comprobado leyendo el orden real de llamadas:

1. `EnsureEnv()` llama a `configs.Load()`.
2. `Load()` construye `Config{ JWTSecret: getJWTSecret() }`.
3. `getJWTSecret()` no encuentra `JWT_SECRET` en el entorno, genera uno y lo
   **añade al archivo** con `appendToFile(".env", …)` usando modo **0644**.
4. Al volver a `EnsureEnv()`, `EnvFileExists()` ya devuelve `true` (el archivo
   acaba de nacer), así que **`WriteEnvFile()` no se ejecuta**.

Resultado: un `erpgow.exe` de doble clic, sin `--setup`, deja un `.env` con **una
sola línea** (`JWT_SECRET=…`) y permisos 0644, y todo lo demás sale de valores
por defecto del entorno. Con `--setup` sí se escribe el `.env` completo
(`✓ Super administrador creado y .env guardado`).

**Diseño propuesto:** el instalador **siempre** escribe el `.env` completo (es su
trabajo, no el del arranque accidental), con modo **0600** y propietario correcto
(`erpgo:erpgo` en Linux). Y en el código: `appendToFile` debería usar 0600, porque
un secreto de firma de sesiones en un archivo legible por todo el sistema es una
fuga silenciosa.

### 3.5 No queda registro de la instalación

El asistente imprime en `stdout`. Eso se pierde en cuanto la ventana se cierra,
y con doble clic el usuario no puede copiar nada para soporte. Para un instalador
que se va a usar en máquinas de clientes, **el registro es requisito**, no
comodidad.

**Diseño propuesto:** `--log=<ruta>` obligatorio en modo no interactivo, y por
defecto:

- Windows: `%ProgramData%\ERPGo\logs\instalacion-AAAAMMDD-HHMMSS.log`
- Linux: `/var/log/erpgo/instalacion-AAAAMMDD-HHMMSS.log`

Con las contraseñas **enmascaradas** (`DB_PASSWORD=****`), y con un resumen final
de: versión, host/puerto/base (sin clave), puerto HTTP elegido, ruta de datos,
resultado de cada comprobación, y el error exacto del primer fallo.

---

## 4. Las tres caras: cómo se presenta el instalador

Se pidió: *"que permita usar ventanas o abra la terminal, o una terminal propia
para proceder a usar la instalación"*. Traducido a opciones concretas:

| # | Cara | Cómo se ve | Cómo se lanza | Qué necesita del código |
|---|---|---|---|---|
| **A** | **Consola en su terminal** | La ventana de consola de siempre, con el asistente de 6 pasos | Doble clic en `erpgow.exe` (con `--setup`), o `erpgow.exe --setup` desde una terminal | Nada: **es lo de hoy** |
| **B** | **Terminal propia** | Ventana de consola con título e ícono propios, que **no se cierra al terminar** y deja leer el resultado | Un lanzador (`ERPGo-Instalar.exe`, 30 líneas de Go) que abre `cmd /k` o `wt.exe` con el título y ejecuta el asistente | `--setup` + registro a archivo (§3.5); opcional `--pause-al-final` |
| **C** | **Ventana propia (sin consola)** | Página web local, con la CSS de instalador que **ya está en el repositorio** (`static/installer/`), abierta en el navegador | `erpgow.exe --setup --ui=web`: escucha en `127.0.0.1:<puerto efímero>`, abre el navegador y cierra al terminar | Una interfaz de preguntas con implementación web (§3.2) y reutilizar `renderStandalone` como en `setup_company.html` |
| **D** | **Paquete nativo** | Asistente clásico de Windows (Inno Setup) o script de Linux: "Siguiente, siguiente, Finalizar" | `ERPGo-Setup-x.y.z.exe` / `./instalar.sh` | Que **el paquete no haga la instalación**: la delega en `erpgow --setup --answer-file` (modo no interactivo) |

### 4.1 Recomendación

**Un cerebro, tres puertas.** La lógica de instalación vive una sola vez, en Go
(la que ya existe en `wizard.go`). Lo demás son presentaciones:

- **Cara A** (consola) — se queda como está: es la que menos puede fallar y la
  que siempre funciona, incluso por SSH.
- **Cara C** (ventana propia) — **la recomendación como cara por defecto en
  Windows**, porque es la más simple de construir de las que dan "ventana": no
  requiere toolkit gráfico nativo, no mete CGO en el binario (se conserva la
  promesa `CGO_ENABLED=0` y el binario único), funciona igual en Linux, y **la
  hoja de estilo ya está escrita y sin usar**. El precio es que necesita un
  navegador, que en Windows 10/11 y en cualquier escritorio Linux existe.
- **Cara B** (terminal propia) — barata y muy útil como *puerta de rescate*:
  cuando la instalación falla, una ventana que no se cierra vale más que
  cualquier página.
- **Cara D** (paquete nativo) — se añade **después**, cuando A y C estén
  probadas, y solo si hay que instalar en máquinas de clientes en serie.

**Lo que NO se recomienda:** una GUI nativa hecha en Go (Fyne, walk, Qt).
Motivos concretos, no de gusto: esas bibliotecas exigen **CGO** (rompe la
compilación reproducible y multiplataforma verificada en
[COMPILAR-EN-OTRA-PC.md](COMPILAR-EN-OTRA-PC.md)), añaden 20–40 MB al binario,
necesitan hilos de interfaz propios (`runtime.LockOSThread`) y una biblioteca
distinta por plataforma — es decir, **dos instaladores que mantener** en vez de
uno. La cara C da una ventana sin ninguna de esas facturas.

### 4.2 La ventana propia (cara C), en concreto

```
erpgow --setup --ui=web
  ├─ comprueba que no haya ya una instalación (y ofrece mantenimiento)
  ├─ escucha SOLO en 127.0.0.1, puerto efímero (no 8080: puede estar ocupado)
  ├─ imprime en consola y en el log:  "Abra http://127.0.0.1:49321"
  ├─ abre el navegador del sistema (rundll32 url.dll / xdg-open)
  ├─ sirve el asistente como una página por paso, con el mismo motor
  └─ al terminar: muestra las credenciales, el puerto y el botón "Abrir ERPGo"
```

Detalles que deciden si esto sale bien o mal:

- **Puerto efímero y solo loopback.** Un instalador escuchando en `0.0.0.0`
  durante 3 minutos es una puerta abierta en la red del cliente.
- **Protección contra otro proceso local**: una clave de un solo uso en la URL
  (`http://127.0.0.1:49321/?t=<token>`) y cierre del servidor al terminar. Sin
  esto, cualquier programa de la misma máquina puede enviar el formulario.
- **La conexión a MySQL la prueba el servidor**, no el navegador (el navegador no
  puede hablar MySQL y no debe recibir credenciales de base por HTTP). El
  formulario envía las credenciales a `localhost` —aceptable porque es loopback—
  pero conviene no persistirlas en logs salvo enmascaradas.
- **Sin JavaScript de terceros**: Bootstrap y jQuery ya están en
  `static/installer/`. Un instalador no debe descargar nada de internet.

---

## 5. Arquitectura propuesta

```
                    ┌─────────────────────────────────────────┐
   paquetes →       │  Cara D: ERPGo-Setup.exe (Inno)         │
                    │          instalar.sh (Linux)            │
                    └───────────────┬─────────────────────────┘
                                    │ invoca (no implementa)
                    ┌───────────────▼─────────────────────────┐
   puertas →        │  A: consola   B: terminal propia        │
                    │  C: ventana propia (--ui=web)           │
                    └───────────────┬─────────────────────────┘
                                    │ mismas preguntas, un solo motor
                    ┌───────────────▼─────────────────────────┐
   cerebro →        │  erpgow --setup  (wizard.go, ya existe) │
                    │  + --answer-file (nuevo) + --log (nuevo)│
                    └───────────────┬─────────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        ▼                           ▼                           ▼
  Configuración              Base de datos                Servicio
  .env 0600                  CREATE DATABASE (si falta)   Windows: --install
  puertos                    migraciones (idempotentes)   Linux: unidad systemd
  credenciales admin         empresa + admin + catálogos  (o modo consola)
```

Tres principios que sostienen el conjunto:

1. **El paquete no instala: pregunta y delega.** Así el `.exe` de Inno y el
   `instalar.sh` no pueden divergir de lo que hace el binario.
2. **El asistente no reinicia servicios ni copia archivos.** Eso es del paquete
   (o del propio binario con `--install`, que ya sabe hacerlo en Windows).
3. **Todo lo que el instalador decide queda escrito** en `.env` y en el log: una
   instalación tiene que poder auditarse leyendo un archivo.

### 5.1 Dónde vive cada cosa

Separar **programa** de **datos** es lo que permite actualizar sin tocar el
negocio, y en Windows es además obligatorio por permisos (`Program Files` es de
solo lectura para un usuario normal, y los servicios corren como `LocalSystem`).

| | Windows | Linux |
|---|---|---|
| Programa | `%ProgramFiles%\ERPGo\erpgow.exe` (+ sello, `--version`) | `/opt/erpgo/erpgow` (+ sello) |
| Configuración | `%ProgramData%\ERPGo\.env` (0600, solo admins) | `/etc/erpgo/erpgo.env` (0640 root:erpgo) |
| Datos | `%ProgramData%\ERPGo\` | `/var/lib/erpgo/` |
| Respaldos | `%ProgramData%\ERPGo\backup\` | `/var/lib/erpgo/backup/` |
| Journal de escrituras | `%ProgramData%\ERPGo\data\escrituras\` | `/var/lib/erpgo/data/escrituras/` |
| Cargas (imágenes de producto, etc.) | `%ProgramData%\ERPGo\uploads\` | `/var/lib/erpgo/uploads/` |
| Logs | `%ProgramData%\ERPGo\logs\` | `/var/log/erpgo/` |
| Unidad de arranque | Servicio `ERPGoServer` (SCM) | `erpgo.service` (systemd) |
| Usuario | `LocalSystem` (o una cuenta de servicio si el cliente lo pide) | `erpgo` (usuario de sistema, sin login) |

**Esto es exactamente lo que hoy no se puede hacer** sin resolver §3.1: mientras
las rutas sean relativas, no hay forma de decirle al programa dónde están sus
datos. La alternativa barata (y aceptable como paso 1) es instalar todo junto en
una carpeta y fijar el directorio de trabajo, que es lo que hace hoy el
`instalar.bat`: `C:\ERPGo\` con el `.exe`, el `.env` y los datos al lado. Para
una PyME que instala en la máquina del mostrador, **esa simplicidad es válida**;
para el servidor de una empresa con varias cajas, no.

> Decisión a estudiar (§18, D-3): ¿un solo modo (todo junto, carpeta propia) o
> dos modos (portable y servidor)? La recomendación es **empezar por "todo
> junto" y dejar la puerta abierta** con `--data-dir`, que es lo que hace que el
> segundo modo no exija cambiar el instalador, solo pasar un parámetro.

---

## 6. Comprobaciones previas (preflight)

Son la diferencia entre "instalador robusto" y "instalador que falla raro". Cada
una con su mensaje exacto y su salida.

| # | Comprobación | Cómo | Si falla | Bloquea |
|---|---|---|---|---|
| 1 | Arquitectura y sistema | `runtime.GOOS/GOARCH`; Windows ≥ 10 1809, glibc ≥ 2.28 | Mensaje | Sí |
| 2 | Espacio libre | ≥ 2 GB en la unidad de datos (≥ 5 GB si va a facturar años) | Mensaje | Sí |
| 3 | Memoria | ≥ 4 GB (aviso si 2–4 GB) | Aviso | No |
| 4 | **Motor de base responde** | `EsServidorMySQLArriba(host, port)` (ya existe) | Guía de instalación del motor | Sí |
| 5 | **Versión del motor** | `SELECT VERSION()`: MySQL ≥ 8.0 o MariaDB ≥ 10.6 | Aviso claro (utf8mb4 y funciones de ventana) | No (aviso) |
| 6 | **Credenciales y permisos** | `Ping` + `CREATE DATABASE IF NOT EXISTS` de prueba (y `CREATE USER`/`GRANT` solo si se van a crear) | Explica qué permiso falta | Sí |
| 7 | **Colación y juego de caracteres** | `utf8mb4` + `utf8mb4_unicode_ci` | Aviso (los acentos y el `ñ` de los nombres) | No |
| 8 | **`mysqldump` disponible** | `localizar_mysql.go` (ya existe) + `mysqldump --version` | **Aviso fuerte**: los respaldos automáticos no funcionarán; ofrecer desactivarlos o indicar la ruta | No |
| 9 | Puerto HTTP libre | 8080, y si está ocupado **buscar el primero libre entre 8080 y 8099** (patrón del `instalar.bat`) | Propone otro puerto | Sí (resuelto solo) |
| 10 | Puerto gRPC libre (9095) | Igual, rango 9095–9109 | Propone otro | Sí (resuelto solo) |
| 11 | Instalación previa | Existe `.env` **o** la base tiene `user_login` con filas | Ofrece *Mantenimiento / Actualizar / Reinstalar* | Rama |
| 12 | Servicio existente | `mgr.OpenService("ERPGoServer")` / `systemctl is-active erpgo` | Ofrece detenerlo y reinstalar | Rama |
| 13 | Escritura en las carpetas destino | Crear y borrar un archivo de prueba | Error con la ruta exacta | Sí |
| 14 | Reloj del sistema | Aviso si la fecha está a más de 1 día del sello del binario | Aviso: afecta a series fiscales y períodos contables | No |
| 15 | Antivirus / bloqueo del ejecutable | Solo si hay sospecha: `--self-check` | Aviso | No |

Sobre el punto 8: hoy `BACKUP_ENABLED=1` por defecto y el respaldo del ERP depende
de `mysqldump`. **Un instalador que no verifica `mysqldump` y luego falla en el
primer respaldo a las 3 de la mañana es un mal instalador.** Aquí es donde se
gana o se pierde la confianza del cliente.

---

## 7. Flujos concretos

### 7.1 Windows, ventana propia (recomendado, cara C)

```
Usuario hace doble clic en  ERPGo-Instalar.exe   (o en un acceso directo)
  ├─ ¿hay permisos de administrador?
  │     no → ofrece modo "solo para mí" (%LOCALAPPDATA%, sin servicio)
  ├─ copia erpgow.exe a C:\ERPGo\   (o C:\Program Files\ERPGo si es servidor)
  ├─ lanza:  erpgow.exe --setup --ui=web --log=%TEMP%\erpgo-instalacion.log
  ├─ el binario abre el navegador en 127.0.0.1:<efímero>
  │     1. Aviso legal (el que ya existe en wizard.go)
  │     2. Motor de base: host, puerto, base, usuario, clave → [Probar conexión]
  │     3. Puertos del ERP (HTTP y gRPC) con detección de libres
  │     4. Empresa: razón social, RIF, dirección, teléfono, correo
  │     5. Administrador: usuario y contraseña (con medidor y confirmación)
  │     6. Resumen + [Instalar]
  │     … resultado: credenciales, URLs, ruta del log, [Abrir ERPGo]
  ├─ ¿instalar como servicio?  → erpgow.exe --install  (SCM, con recuperación)
  ├─ ¿acceso desde otras cajas (LAN)?
  │     sí → SERVER_HOST=0.0.0.0 + regla de firewall para 8080 y 9095
  ├─ accesos directos: "ERPGo", "ERPGo — detener", "ERPGo — registros"
  └─ registro de desinstalación (Add/Remove Programs) apuntando a desinstalar
```

### 7.2 Windows, consola y terminal propia (caras A y B)

- **A (consola):** `erpgow.exe --setup` desde `cmd` o PowerShell. Es lo que ya
  funciona; el instalador solo documenta la ruta y deja un acceso directo
  "Instalar / reparar" que abre `cmd` en esa carpeta.
- **B (terminal propia):** un lanzador mínimo que abre una consola con título
  propio y **no la cierra** al terminar, para que el usuario lea el resultado:

  ```
  cmd /k  title ERPGo — Instalación  &  erpgow.exe --setup --log=... --pause-al-final
  ```

  En Windows 11 puede usarse `wt.exe` (Windows Terminal) con `--title`; en
  Windows 10 `cmd` es lo que siempre está. **El lanzador debe ser un `.exe` de
  Go, no un `.bat`** (§7.4).

### 7.3 Linux (`instalar.sh` o unidad + binario)

La instalación en Linux tiene una diferencia de fondo: **el binario no sabe
instalarse como servicio** (`manage_other.go` devuelve error). Hoy eso lo
resuelve un administrador a mano. Diseño propuesto:

```
sudo ./instalar.sh
  ├─ comprueba root, systemd, mysql/mariadb, mysqldump, puertos, espacio
  ├─ crea usuario de sistema  erpgo  (sin shell, sin home propio)
  ├─ crea /opt/erpgo, /etc/erpgo, /var/lib/erpgo, /var/log/erpgo con propietario
  ├─ copia el binario y su sello (0600/0755 según corresponda)
  ├─ ejecuta el asistente en modo no interactivo si viene archivo de respuestas,
  │  o interactivo en consola si no
  ├─ escribe /etc/systemd/system/erpgo.service  (plantilla de abajo)
  ├─ systemctl daemon-reload && enable --now erpgo
  ├─ espera /health (hasta 60 s) y si no responde:  muestra journalctl -u erpgo
  └─ imprime credenciales, URL y puertos
```

Unidad propuesta (para estudiar; el endurecimiento se elige, no se copia a
ciegas — cada línea de más puede romper una instalación real):

```ini
[Unit]
Description=ERPGo — ERP fiscal
After=network-online.target mysql.service mariadb.service
Wants=network-online.target

[Service]
Type=simple
User=erpgo
Group=erpgo
WorkingDirectory=/var/lib/erpgo          # ← obligatorio por §3.1
EnvironmentFile=/etc/erpgo/erpgo.env     # ← el .env completo, 0640 root:erpgo
ExecStart=/opt/erpgo/erpgow
Restart=on-failure
RestartSec=5
TimeoutStopSec=30                        # el apagado ordenado usa 20 s
LimitNOFILE=8192
# Endurecimiento (discutible según el caso):
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/erpgo /var/log/erpgo

[Install]
WantedBy=multi-user.target
```

Nota importante: la unidad fija `WorkingDirectory=/var/lib/erpgo` y
`EnvironmentFile=/etc/erpgo/erpgo.env`, que es la forma de convivir con las rutas
relativas **sin tocar el código**. Pero atención: `configs.Load()` usa
`godotenv.Load(".env")` **y** las variables del entorno; con `EnvironmentFile`
las variables llegan del entorno, así que funcionan. Sin embargo el binario
**volvería a escribir** `.env` en el CWD si no lo encuentra — de ahí que la
solución de verdad siga siendo §3.1, y que `WorkingDirectory` sea un puente.

### 7.4 Una lección del propio repositorio: nada de `.bat`

[INSTALADOR.md](INSTALADOR.md) documenta **cuatro bugs reales** del instalador
`.bat`: un paréntesis dentro de un bloque `if` que rompía el script, `>nul`
convertido en `/dev/null` (que creaba archivos), `find`/`timeout` resolviendo a
las herramientas de Unix del PATH, y una llamada a `--install` **cuando ese flag
ya no existía**. [COMPILAR-EN-OTRA-PC.md](COMPILAR-EN-OTRA-PC.md) añade dos
trampas más: los `.bat` **deben** guardarse en CRLF y las redirecciones no son
las de bash.

A eso se suma un hallazgo del estado actual del repositorio: **la raíz no tiene
ningún `.bat`**. `instalar.bat`, `iniciar.bat` y los `compilar-*.bat` que citan
los documentos **no están versionados** (`.gitignore` ignora los binarios y los
secretos, pero ningún `.bat`): existen solo en la máquina de desarrollo. Es
decir: hoy **no hay forma de instalar el sistema a partir del repositorio**, solo
a partir de la carpeta de alguien.

**Regla propuesta, que es la conclusión de todas esas heridas:** *la lógica de
instalación se escribe en Go, y los scripts de empaquetado se limitan a copiar
archivos y llamar al binario.* Un `.bat` de 10 líneas que copia y ejecuta está
bien; uno que decide es una bomba de tiempo.

---

## 8. Actualización (upgrade) y downgrade

El proyecto ya promete el contrato correcto: *"El cliente descarga el nuevo
binario y listo. Las migraciones corren solas."* Lo que falta es hacerlo
verificable y reversible.

**Reglas de diseño:**

1. **Detectar antes de escribir**: la versión del binario nuevo se compara con la
   registrada (`--version` de la instalación existente). Sin versión embebida
   (§3.3) esto no se puede hacer.
2. **Respaldo antes de actualizar**: `mysqldump` de todas las bases
   (`erpgo` y `erpgo_*`) a la carpeta de respaldos, con nombre con fecha. Si el
   respaldo falla, **la actualización no empieza** (un upgrade sin red de
   seguridad no se hace).
3. **Reemplazo atómico del binario**: escribir `erpgow.nuevo` en el **mismo
   sistema de archivos**, renombrar el actual a `erpgow.previo`, y mover el nuevo
   a su sitio. En Windows eso exige detener el servicio primero (el archivo está
   bloqueado); los scripts de despliegue ya usan esa estrategia.
4. **Migraciones al arrancar**, que es lo que ya pasa: idempotentes y con
   checksum. El instalador **no** ejecuta SQL por su cuenta.
5. **Verificar**: `/health` en 60 s. Si no responde, **volver al binario
   anterior** (`erpgow.previo`) y decirlo. Los scripts de despliegue de hoy
   **no tienen rollback automático** (lo dicen ellos mismos): el instalador
   debería tenerlo.
6. **Negarse al downgrade**: si el esquema registrado es más nuevo que el binario,
   detener y explicar. Los datos son sagrados; un binario viejo sobre un esquema
   nuevo es la forma más fácil de corromperlos.
7. **Nunca tocar** `.env` ni los datos en una actualización. Si falta una variable
   nueva, **se añade** sin reescribir lo existente (mismo principio que las
   migraciones: agregar, nunca reemplazar).

---

## 9. Desinstalación

| Qué | Windows | Linux |
|---|---|---|
| Detener y quitar el servicio | `erpgow.exe --uninstall` | `systemctl disable --now erpgo` + borrar la unidad |
| Borrar el programa | Sí (Inno lo hace) | Sí (`/opt/erpgo`) |
| Borrar atajos y entrada de "Agregar o quitar programas" | Sí | n/a |
| **Configuración (`.env`)** | **Pregunta.** Por defecto **se conserva** (contiene el `JWT_SECRET` y la conexión) | **Se conserva** `/etc/erpgo/erpgo.env` |
| **Datos, respaldos y cargas** | **Nunca se borran sin una segunda confirmación explícita**, y se dice dónde quedaron | Igual |
| **La base de datos MySQL** | **Jamás se elimina.** Es del cliente y contiene sus libros fiscales | Igual |

Texto propuesto para el instalador, porque un instalador que borra sin decir nada
es un instalador peligroso:

> *"Se desinstalará el programa. Sus datos, sus respaldos y su base de datos en
> MySQL NO se eliminan. Si desea eliminarlos, hágalo usted mismo: están en
> `<rutas>` y en la base `<nombre>`."*

---

## 10. Instalación desatendida (para instalar en máquinas de clientes en serie)

### 10.1 Archivo de respuestas

Mismo formato que el `.env` (así el usuario ve un solo tipo de archivo en el
proyecto), más tres campos de control:

```ini
# --- respuestas del instalador ---
ADMIN_USER=admin
ADMIN_PASSWORD=             # vacío = generar una y mostrarla al final
COMPANY_NAME=Panadería El Trigal C.A.
COMPANY_RIF=J-30123456-7
DB_HOST=127.0.0.1
DB_PORT=3306
DB_ROOT_USER=root
DB_ROOT_PASSWORD=****       # el instalador la usa y NO la escribe en .env
DB_NAME=erpgo
SERVER_PORT=8080
SERVER_HOST=127.0.0.1       # 0.0.0.0 solo si hay cajas en red
INSTALL_SERVICE=1
OPEN_FIREWALL=0
ENABLE_BACKUP=1

# --- control ---
ANSWER_VERSION=1
REQUIRE_VERSION_MIN=1.0.0   # si el binario es anterior, no instala
ALLOW_OVERWRITE_ENV=0       # 0 = si ya hay .env, no lo toca
```

### 10.2 Códigos de salida (contrato con el paquete y con el soporte)

| Código | Significado |
|---|---|
| 0 | Instalado correctamente |
| 10 | Motor de base no responde |
| 11 | Credenciales o permisos de base rechazados |
| 12 | Puerto ocupado y ninguno libre en el rango |
| 13 | Sin espacio en disco |
| 14 | Sin permisos para escribir en la carpeta destino |
| 15 | Ya existe una instalación (y no se pidió sobrescribir) |
| 16 | Versión del binario demasiado vieja para el archivo de respuestas |
| 17 | Falló la creación del servicio |
| 18 | `/health` no respondió tras instalar (instalación dejada atrás y explicada) |
| 19 | Error interno del asistente (con el log en la ruta indicada) |

Sin esta tabla, el `.exe` de Inno solo puede decir "falló", que es exactamente lo
que no sirve para soporte remoto.

### 10.3 Compatibilidad con lo que ya existe

El `instalar.bat` documentado pasaba el **puerto por el entorno** y no lo escribía
en `.env`, para que el `.env` de una instalación no quedara con un puerto que
solo vale en ese equipo. **Ese patrón se conserva**: el entorno manda sobre el
`.env` (así está construido `getEnv`), y el instalador usa esa precedencia para
las decisiones locales (puerto, ruta de datos) sin ensuciar el archivo.

---

## 11. Seguridad del instalador

| Tema | Diseño |
|---|---|
| **Firma del paquete** | Authenticode en Windows (certificado; hoy **no hay**, y es la única frontera real contra el parcheo del binario: el sello SHA-256 interno no basta, porque vive dentro de lo que protege, como dice el comentario de [main.go:236](../cmd/server/main.go#L236)) |
| **Sello e integridad** | El instalador copia el binario **tal cual** y su sello; nunca lo reescribe (el arranque verifica `Sello SHA-256` y la firma Ed25519) |
| **Permisos de `.env`** | 0600 en Windows, 0640 `root:erpgo` en Linux. Hoy puede quedar 0644 (§3.4) |
| **Contraseñas en logs** | Enmascaradas siempre. El log es un archivo que el usuario va a enviar por correo |
| **Credenciales de base** | Se piden, se prueban, y el instalador **no** las deja en el archivo de respuestas si no lo pidió el usuario |
| **Puerto de la UI web** | Solo `127.0.0.1`, puerto efímero, token de un uso, servidor que se cierra al terminar |
| **Firewall** | Solo se abre si el usuario elige acceso desde la red, y **solo** 8080/9095. Nunca se desactiva el firewall |
| **TLS** | El diseño contempla `TLS_CERT_FILE`/`TLS_KEY_FILE`; para LAN y local, HTTP plano es lo correcto (ya está decidido en `config.go`). Si se expone a internet: HTTPS + Caddy/nginx, que ya está documentado |
| **No tocar el SO** | Nada de `apt/yum/dnf/choco/winget`, ni servicios del sistema, ni el `datadir` de MySQL, ni la configuración del motor |
| **`JWT_SECRET`** | Generado por el binario con `crypto/rand`; el instalador no lo inventa ni lo reutiliza entre instalaciones |

---

## 12. Qué falta en el código (lista accionable)

Ordenada por lo que desbloquea. **Nada de esto se implementa en este documento**,
pero sin esto el instalador se sostiene con alambre.

| # | Cambio | Dónde | Por qué es necesario |
|---|---|---|---|
| **C-1** | `os.Chdir(dir del exe)` al arrancar como servicio | [cmd/server/main.go](../cmd/server/main.go) | Parche inmediato de §3.1: evita escribir en `System32` |
| **C-2** | Raíz de datos explícita (`--data-dir` / `ERPGO_DATA_DIR`) y las **siete** rutas de §3.1 colgando de ella | `configs/`, `interfaces/web/`, `cmd/server/` | Permite separar programa y datos; es la base de las actualizaciones limpias |
| **C-3** | Versión embebida (`-ldflags -X`) + `erpgow --version` | `cmd/server/`, `scripts/build.*` | Sin versión no hay detectar/reparar/actualizar ni negarse a un downgrade |
| **C-4** | Modo no interactivo: `--answer-file`, `--dry-run`, códigos de salida | `internal/setup/` | Es lo que permite que Inno y `instalar.sh` **deleguen** en vez de reimplementar |
| **C-5** | Registro a archivo (`--log`) con claves enmascaradas | `internal/setup/`, `cmd/server/` | Soporte remoto sin depender de una ventana abierta |
| **C-6** | Interfaz de preguntas con implementación web (`--ui=web`) reutilizando `static/installer/` y `renderStandalone` | `internal/setup/`, `interfaces/web/` | Es la "ventana propia" sin CGO. Los recursos gráficos ya están en el repo |
| **C-7** | `--install` en Linux (unidad systemd) o, como mínimo, exportar la plantilla de la unidad | `internal/service/` | Hoy un Linux sin unidad no tiene arranque automático |
| **C-8** | Comprobación de `mysqldump` y aviso en la instalación si falta | `internal/setup/`, `infrastructure/persistence/localizar_mysql.go` | Los respaldos están activados por defecto: fallarían en silencio |
| **C-9** | `appendToFile` a 0600 y `.env` completo en el primer arranque | `configs/config.go` | Fuga de `JWT_SECRET` en un archivo legible por todos (§3.4) |
| **C-10** | `--health`/`--self-check` (versión + sello + conexión a base + puertos) | `cmd/server/` | Es lo que el instalador usa para verificar y lo que el cliente usa para diagnosticar |
| **C-11** | Rollback en la actualización (`erpgow.previo` + `/health`) | `scripts/deploy/`, instalador | Los scripts de hoy lo dejan anotado como pendiente |
| **C-12** | Versionar los lanzadores (hoy **no existen** en el repositorio) | raíz / `scripts/` | §7.4: no se puede instalar desde el repositorio |

---

## 13. Matriz de pruebas

Cada fila es una máquina y cada columna un escenario. Un instalador se aprueba
**fallando bien**, no solo instalando bien.

| Escenario | Windows 10 | Windows 11 | Server 2019 | Ubuntu 24.04 | Debian 12 | Rocky 9 |
|---|---|---|---|---|---|---|
| Instalación limpia con MySQL 8 local | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Instalación limpia con MariaDB local | ✅ | — | ✅ | ✅ | ✅ | ✅ |
| Base en **otra** máquina (host remoto) | ✅ | — | ✅ | ✅ | ✅ | ✅ |
| **Sin motor de base** (mensaje claro, nada tocado) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Credenciales de base equivocadas (reintento dentro) | ✅ | — | ✅ | ✅ | — | ✅ |
| Usuario sin permiso `CREATE DATABASE` | ✅ | — | — | ✅ | — | — |
| **Sin `mysqldump`** (aviso de respaldos) | ✅ | — | — | ✅ | ✅ | ✅ |
| Puerto 8080 ocupado (elige otro) | ✅ | ✅ | ✅ | ✅ | — | — |
| Instalación **sin administrador** (modo portable, sin servicio) | ✅ | ✅ | — | ✅ | ✅ | — |
| Reinstalación sobre instalación existente (**datos intactos**) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Actualización de versión (respaldo + `/health` + rollback) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Downgrade intentado (se niega y explica) | ✅ | — | — | ✅ | — | — |
| Desinstalar conservando datos | ✅ | ✅ | — | ✅ | ✅ | ✅ |
| Instalación **desatendida** con archivo de respuestas | ✅ | — | ✅ | ✅ | ✅ | ✅ |
| Reinicio del equipo (servicio vuelve a arrancar) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Corte de energía durante la instalación (se puede repetir) | ✅ | — | — | ✅ | — | — |
| Acceso desde otra PC de la red (firewall) | ✅ | — | ✅ | ✅ | — | — |
| Facturar de punta a punta tras instalar (POS incluido) | ✅ | — | — | ✅ | — | — |

**Criterios de aceptación del instalador:**

1. Instalación completa en **menos de 10 minutos** con el motor ya instalado.
2. **Ningún** fallo deja el sistema en un estado peor que antes, y ninguno escribe
   fuera de las rutas declaradas.
3. Todo fallo tiene **mensaje con causa y acción**, en español, sin jerga.
4. Repetir la instalación no rompe nada (idempotencia).
5. El log permite reconstruir qué pasó **sin acceso a la máquina**.
6. Desinstalar no pierde un solo dato.

---

## 14. Riesgos y trampas (con historial real del proyecto)

| Riesgo | Por qué es real aquí | Mitigación |
|---|---|---|
| **CWD ≠ carpeta de instalación** | Servicio de Windows arranca en `System32`; todas las rutas son relativas | C-1 y C-2 |
| **El instalador nativo cuelga el asistente** | `wizard.go` lee `stdin`; Inno no tiene consola | C-4, o cara C/B |
| **`.bat` con CRLF/libc/`>nul`** | Ya pasó cuatro veces ([INSTALADOR.md](INSTALADOR.md)) | Lógica en Go (§7.4) |
| **Flag que ya no existe** | El `instalar.bat` llamaba a `--install` cuando ya no existía | `--version`/`--help` verificados en preflight |
| **Respaldos silenciosamente rotos** | `BACKUP_ENABLED=1` + dependencia de `mysqldump` | C-8 |
| **Antivirus bloqueando el binario ofuscado** | Los binarios con garble parecen sospechosos a algunos antivirus | Firmar el paquete (Authenticode); documentar exclusión |
| **Instalación duplicada** | Dos `erpgow.exe` apuntando a la misma base | Preflight #11 y #12 |
| **MySQL remoto con `bind-address` local** | Muy común en hostings y en cPanel | Mensaje que distingue "no responde" de "rechaza al usuario" |
| **Rango de puertos ya usado** | El ERP tiene 8080 y 9095; el POS puede querer más | Búsqueda de rango libre y anotación en el log |
| **Actualización que no vuelve atrás** | Hoy no hay rollback | C-11 |
| **Zona horaria y moneda** | `TIMEZONE`, `currency` en `web_setting`; un instalador en UTC descuadra turnos y cierres | Pedir zona horaria en el asistente (hoy no se pregunta) |

---

## 15. Artefactos a publicar

| Artefacto | Contenido | Para quién |
|---|---|---|
| `erpgow-<v>-windows-amd64.zip` | Binario + sello + `LEEME.txt` + lanzadores | Instalación manual / soporte |
| `ERPGo-Setup-<v>.exe` (Inno) | Todo lo anterior + asistente nativo + servicio + atajos + desinstalador | Cliente típico |
| `erpgow-<v>-linux-amd64.tar.gz` | Binario + sello + `instalar.sh` + plantilla de unidad + `LEEME.txt` | Servidor Linux |
| `SHA256SUMS` + firma | Verificación de integridad de todo lo anterior | Quien descarga |
| `compilar-entrega.bat` / `.sh` (versionados) | Reproducir los artefactos anteriores con una orden | El propio equipo |

Hoy los scripts de compilación son `scripts/build.bat` y `scripts/build.sh`
(compilación y ofuscación), pero **no hay ningún script de empaquetado**: el paso
"y ahora armar el instalador" es manual y no está versionado.

---

## 16. Lo que hay que decidir antes de implementar

| # | Decisión | Opciones | Recomendación |
|---|---|---|---|
| **D-1** | Cara por defecto en Windows | consola / terminal propia / ventana propia (web) / Inno | **Ventana propia (web)**, con consola siempre disponible |
| **D-2** | ¿Se escribe una GUI nativa? | sí (Fyne/walk) / no | **No**: rompe `CGO_ENABLED=0` y duplica mantenimiento |
| **D-3** | Ubicación | todo junto en una carpeta / programa y datos separados | **Todo junto** para PyME; separado cuando exista `--data-dir` |
| **D-4** | Servicio o consola | servicio siempre / preguntar / solo con `--install` | **Preguntar**, con servicio por defecto en servidores y consola por defecto en la máquina del mostrador |
| **D-5** | Linux: ¿paquete o script? | `.deb`/`.rpm` / `tar.gz + instalar.sh` | **`tar.gz + instalar.sh`** primero; paquetes después, si hay demanda |
| **D-6** | ¿Se empaqueta el motor de base? | nunca / opcional en un paquete "todo incluido" | **Nunca** (decisión ya tomada, §2). Un paquete aparte y opcional podría estudiarse, pero contradice el principio de no tocar el anfitrión |
| **D-7** | Firma de código | certificado Authenticode / sin firma | **Con firma** cuando se distribuya en serie; es la única defensa real |
| **D-8** | Alcance del instalador | solo el servidor / servidor + POS | **Solo el servidor** ahora; el POS es una terminal que se configura desde el ERP |
| **D-9** | Credenciales por defecto | `admin`/`Admin@2026` / obligar a elegirlas | **Obligar a elegirlas** en instalación nueva (hoy `--setup` ya pregunta; el acceso directo de la documentación sugiere el default) |
| **D-10** | Modo desatendido | solo interactivo / archivo de respuestas | **Archivo de respuestas**: es lo que permite instalar 10 cajas iguales |

---

## 17. Orden de trabajo sugerido (cuando se decida hacerlo)

1. **Cimientos invisibles** (C-1, C-3, C-9): una tarde. Sin esto, todo lo demás
   hereda el problema del CWD y no hay versión para detectar instalaciones.
2. **Modo no interactivo + log + códigos de salida** (C-4, C-5, C-10): es lo que
   permite que cualquier cara y cualquier paquete deleguen en el asistente.
3. **Cara B (terminal propia)**: el lanzador mínimo. Ya deja una experiencia
   decente en Windows sin escribir una sola línea de interfaz.
4. **Cara C (ventana propia / UI web)** (C-6): reutilizando `static/installer/`.
   Aquí el producto empieza a verse como un instalador.
5. **Servicio en Linux + `instalar.sh`** (C-7): la paridad entre sistemas.
6. **Paquete Inno** (C-4 ya lo hace posible): última pieza de Windows.
7. **Actualización con rollback** (C-11) y pruebas de la matriz completa.
8. **Firma de código** y publicación de artefactos.

Cada paso deja el sistema **usable**: no hay ningún paso que dependa de que el
siguiente esté hecho. Eso es lo que "simple" significa aquí: pocos pasos, cada
uno con valor propio, y ninguno que rompa lo anterior.

---

## 18. Resumen en diez líneas

1. El motor de base no es nuestro y **no se instala**: se comprueba (§2).
2. El asistente ya existe (`--setup`) y **es el cerebro**; las caras son puertas (§4).
3. Tres puertas: consola, terminal propia y **ventana propia web**; Inno después (§4.1).
4. **Una sola implementación de la lógica**, en Go; los `.bat` solo copian (§7.4).
5. Rutas explícitas: el CWD no puede decidir dónde viven los datos (§3.1).
6. Versión embebida y `--version`: sin eso no hay detectar, reparar ni actualizar (§3.3).
7. Modo no interactivo, log a archivo y códigos de salida: lo que hace usable el paquete (§10).
8. Preflight honesto, sobre todo con `mysqldump` y con los puertos (§6).
9. Actualizar con respaldo previo y rollback; desinstalar **sin** perder datos (§8, §9).
10. Windows y Linux con el mismo diseño, distinta unidad de arranque (§5.1, §7.3).

---

## Referencias

- [README.md](../README.md) — requisitos y la política de orden y crecimiento
- [INSTALADOR.md](INSTALADOR.md) — qué hace hoy el instalador de Windows y sus bugs reales
- [instalador-windows.md](instalador-windows.md) — bosquejo previo con Inno Setup
- [PLAN-INSTALADOR-ERPGO-POS.md](PLAN-INSTALADOR-ERPGO-POS.md) — plan del asistente
- [COMPILAR-EN-OTRA-PC.md](COMPILAR-EN-OTRA-PC.md) — compilación, ofuscación y trampas de `.bat`
- [MANUAL-DESPLIEGUE-TLS.md](MANUAL-DESPLIEGUE-TLS.md) — exposición a internet con TLS
- [scripts/deploy/README_DEPLOY.md](../scripts/deploy/README_DEPLOY.md) — actualización desde Git
- [SEGURIDAD.md](SEGURIDAD.md) — medidas del binario (firma, sellos, FieldKey)
