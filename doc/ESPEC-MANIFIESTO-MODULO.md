# Especificación — Manifiesto y ciclo de vida del módulo

**Estado:** decide. Desarrolla la sección 7 de `doc/PLAN-MODULAR-2.0.md`.
**Fecha:** 2026-10-04.
**Ámbito:** el formato del manifiesto de un módulo y las operaciones
`descubrir → detectar → activar → desactivar → desinstalar`. No describe cómo se
migra el código actual a módulos (eso es la Fase 0 del plan).

---

## 1. Dos cosas que se llaman "módulo" (no confundirlas)

En este repositorio ya existe una tabla `module` y su hijo `sub_module`, y **no
son el registro de addons**: son la **taxonomía del menú** (`module` es el grupo
de primer nivel; `sub_module.mid` lo referencia; la visibilidad se resuelve en
`role_permission`). Está definida en `migrations/schema.sql` y poblada por
`migrations/schema_menu_permissions.sql`:

```sql
CREATE TABLE `module` (
  `id` int NOT NULL, `name` varchar(50) NOT NULL, `description` text,
  `image` varchar(255), `directory` varchar(100), `status` tinyint(1)
);
CREATE TABLE `sub_module` (
  `id` int NOT NULL, `mid` int NOT NULL, `name` varchar(100) NOT NULL,
  `description` text, `image` varchar(100), `directory` varchar(50), `status` tinyint(1)
);
```

Por eso el **catálogo de addons vive en su propia tabla, `module_registry`**
(sección 6). Mezclarlas acoplaría dos ciclos de vida distintos: la taxonomía de
menú cambia con el producto; el addon se instala y se desinstala en el cliente.

El puente entre ambas es de **una sola dirección**: al activar, el **núcleo**
inserta las filas de menú en `module`/`sub_module`. El módulo nunca escribe esas
tablas; las declara (sección 7).

---

## 2. Estructura en disco

Modelo "Laragon": un directorio autocontenido. Todo lo que un módulo necesita
para instalarse vive junto, y el maestro lo descubre por convención.

```
ERPGo/
├── erpgow.exe                     # binario maestro (.bat o servicio de Windows)
├── addons/
│   └── carniceria/                # un directorio por módulo, nombrado por code
│       ├── module.json            # manifiesto (sección 3)
│       ├── module.sig             # firma Ed25519 sobre los bytes de module.json
│       ├── modulo_carniceria.exe  # binario del módulo (entry)
│       └── web.zip                # paquete web del módulo (opcional; puede ir embebido)
├── web/                           # despliegue de la web de cada módulo
│   └── carniceria/                # lo que web.zip descomprime aquí
├── templates/                     # layout.html global (header/footer), ya existe
├── static/
└── data/
```

Reglas de la convención:

- El nombre del directorio (`carniceria`) **debe coincidir con `code`**. Si no
  coincide, el módulo se marca en error: la ruta es parte de la identidad.
- `module.json` es la **única fuente del manifiesto antes de ejecutar** el
  binario. El maestro lo lee *sin arrancar* el `.exe`; por eso el manifiesto va
  en disco y no solo compilado dentro.
- `web.zip` puede ir como archivo al lado o embebido en el binario (campo `web`
  del manifiesto, sección 3). En Fase 0 no existe; se describe para Fase 2.

### Ya implementado: el núcleo resuelve `web/<code>/` (2026-10-04)

La resolución de archivos por módulo **ya está en el núcleo**, antes de que
exista ningún addon. `embed.go` trata cada `web/<code>/` como una raíz más:

- `web/<code>/templates/<pagina>.html` se pinta sin recompilar y **hereda el
  layout global**: `buildTemplateSet` lee el layout con `erpgow.GlobalTemplate`,
  que ignora los directorios de los módulos a propósito.
- `web/<code>/static/<hoja>` se sirve como `/static/<code>/<hoja>`
  (`staticOverlay`).
- El orden de búsqueda es **núcleo → módulos → embebido**: un módulo no puede
  pisar una plantilla del núcleo.
- `Migrations()` sigue **sin** raíces de módulo: un `.sql` suelto en
  `web/<code>/` no debe ejecutarse en el arranque de cada empresa.
- `RefreshModuleRoots()` vuelve a buscar directorios tras activar o desactivar
  un módulo en caliente.

Lo que falta de esta sección es solo el **empaquetado**: el `web.zip` y su
descompresión con validación de *path traversal* (sección 5.2, paso 2).

---

## 3. Formato del manifiesto — `module.json`

UTF-8, JSON, sin comentarios. Es deliberadamente **pequeño y estable**: solo
lleva lo que el maestro necesita para decidir si puede ejecutar el binario.
Todo lo demás (menú, permisos, migraciones, esquema para la IA) no va aquí:
lo declara el módulo por RPC durante el *handshake* (sección 7), porque cambia
entre versiones y se firma con el binario, no con el archivo.

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `schema` | int | sí | Versión del formato del manifiesto. Hoy `1`. |
| `code` | string | sí | Identificador estable (`carniceria`). Nunca cambia. Debe ser `[a-z0-9_]{3,64}` y coincidir con el directorio. |
| `name` | string | sí | Nombre legible que verá el administrador. |
| `description` | string | no | "Función" del módulo, en una frase. |
| `version` | string | sí | SemVer `MAYOR.MENOR.PARCHE`. |
| `api` | int | sí | Versión de la interfaz `Module` que el módulo espera. El maestro solo carga `api <= MASTER_API`. |
| `entry` | string | sí | Ruta relativa al `.exe` dentro del directorio del módulo. |
| `binary_sha256` | string | sí | SHA-256 (hex, minúsculas) del binario `entry`. |
| `rif_binding` | string | sí | RIF del cliente al que se ata el módulo (`J-12345678-9`). |
| `runtime` | string | sí | `inproc` \| `netrpc` \| `grpc`. En Fase 0 solo `inproc`. |
| `web` | string | no | `embedded` \| `sidecar:web.zip` \| `none`. Ausente = `none`. |
| `signature` | string | no | **Solo si la firma va dentro del JSON** (variante B, sección 4). Se prefiere `module.sig`. |

Ejemplo mínimo (`addons/carniceria/module.json`):

```json
{
  "schema": 1,
  "code": "carniceria",
  "name": "Carniceria",
  "description": "Pesaje, despiece, merma y costeo por corte.",
  "version": "1.0.0",
  "api": 1,
  "entry": "modulo_carniceria.exe",
  "binary_sha256": "3f7a...e91c",
  "rif_binding": "J-12345678-9",
  "runtime": "netrpc",
  "web": "sidecar:web.zip"
}
```

### Por qué estos campos y no otros

- `api` es un solo entero, no un rango: el maestro exige compatibilidad hacia
  atrás y escribe la suya en `MASTER_API`. Un módulo que pide más de lo que el
  maestro ofrece **no se arranca**; uno que pide menos funciona.
- `binary_sha256` está **dentro del manifiesto firmado**, así que firmar el
  manifiesto ata también al binario: nadie puede intercambiar el `.exe` y
  conservar la firma.
- `rif_binding` replica el criterio ya fijado en el plan
  (`Sign_Orcanet(RIF + ID_Licencia + ID_Modulo)`): un módulo comprado para un RIF
  no arranca en otro.
- `runtime` es un dato del módulo, no una decisión del maestro. Permite que el
  mismo cargador soporte `inproc` (Fase 0/1) y subproceso (Fase 2) sin cambiar
  el manifiesto.

---

## 4. Firma y verificación

Algoritmo: **Ed25519** (`crypto/ed25519`). Firma el **emisor** (Orcanet) con su
clave privada; verifica el **maestro** con la clave pública **fijada en el
binario** (no leída del disco, que sería confiar en el atacante).

### Variante A (preferida) — firma en archivo aparte

`module.sig` contiene la firma en base64 sobre los **bytes exactos de
`module.json`**. No hay que canonicalizar JSON: se firma el archivo tal cual.

```
module.sig = base64( ed25519.Sign(privKey, bytes(module.json)) )
```

**Verificación (todos los pasos, en orden):**

1. `sha256(entry)` == `binary_sha256` del manifiesto. Si no, **no arrancar**.
2. `ed25519.Verify(pubKey, bytes(module.json), base64decode(module.sig))`. Si
   falla, **no arrancar**.
3. `rif_binding` == RIF de la licencia actual. Si no, **no arrancar**.
4. `code` == nombre del directorio. Si no, **error de instalación**.
5. `api <= MASTER_API`. Si no, **no arrancar** (avisar "requiere maestro más nuevo").
6. Solo entonces arrancar el binario y hacer el *handshake* (sección 7). El
   `Manifest` que devuelva el módulo debe coincidir con el sidecar (código,
   versión, API). Si difiere, **cuarentena**: se detiene el proceso y se anota
   `last_error`.

### Variante B — firma embebida

Un solo archivo, `signature` dentro del JSON. Aquí sí hace falta
canonicalización: se firma el JSON con la clave `signature` **omitida**, en
forma canónica (claves ordenadas lexicográficamente, sin espacios, UTF-8).
Es más cómoda de distribuir y más fácil de romper; por eso A es la preferida.

### Qué NO protege la firma

La firma prueba **procedencia e integridad**, no comportamiento: un módulo
firmado puede ser malicioso. El aislamiento real es la **regla dura** de que el
módulo no abre su propia conexión ni escribe auditoría; toda escritura pasa por
el orquestador del núcleo.

---

## 5. Estados y transiciones

El estado vive en `module_registry.status` (sección 6). Cinco estados:

| Estado | Significa | Aparece en el panel | Rutas montadas | Tablas | Web |
|---|---|---|---|---|---|
| `detected` | Firma válida y handshake OK; **no activado** | Sí, como disponible | No | No | No |
| `active` | Instalado y en uso | Sí | Sí | Sí | Sí |
| `disabled` | Activo y luego desactivado | Sí, como deshabilitado | No | Sí (intactas) | Sí (intacta) |
| `error` | Falló detección o activación | Sí, con `last_error` | No | Parcial (ver §5.3) | Parcial |
| *(sin fila)* | Rechazado o desinstalado | No | No | — | — |

```
                 descubrir()
   addons/ ────────────────────┐
                               ▼
                          [detected] ──activar(admin)──► [active]
                               │                            │  ▲
                     error en   │              desactivar    │  │ activar
                     activación │              (admin)       ▼  │
                               ▼                         [disabled]
                            [error] ◄─ desactivar/activar ──┘
                               │
                               │  reintentar()  → vuelve a [detected] si la firma sigue válida
                               │  desinstalar(admin) ──────────► (sin fila)
```

### 5.1 `descubrir` (maestro, al arrancar y en vigilancia)

1. Escanear `addons/*/module.json`.
2. Validar campos y `schema`; leer `module.sig` y verificar (sección 4 pasos 1–5).
   - Firma inválida / hash distinto / RIF distinto → **no arrancar**, escribir
     el motivo en el log. No se crea fila: no queremos basura inspeccionable.
3. Arrancar el binario e invocar `Hello` (sección 7). Comparar `Manifest` con el
   sidecar.
4. `UPSERT module_registry` con `status='detected'`, `detected_at=now`.
   - Si ya existía y estaba `active`/`disabled` con la **misma versión**: no se
     toca el estado (un reinicio no debe desactivar nada).
   - Si la versión **subió**: `status` queda como estaba, se anota
     `available_version` y el panel ofrece "actualizar".

**Nada de esto toca MySQL del negocio, ni `web/`, ni el menú.** Detectar es
puramente aditivo y reversible.

### 5.2 `activar` (acción del administrador)

Orquestada **por el núcleo**, en este orden. Cada paso es idempotente; si uno
falla, se para y se pasa a `error` con `last_error`.

1. **Migraciones.** El núcleo aplica los `.sql` del módulo con el **mismo ledger
   `schema_migrations`** que ya existe (`infrastructure/persistence/database.go`),
   con los archivos **namespaced**: `mod_<code>__<nombre>.sql`. Así el checksum y
   la idempotencia son los del sistema actual, no un mecanismo nuevo. El módulo
   **no ejecuta SQL**: devuelve el contenido y el núcleo lo aplica (regla dura 1).
2. **Web.** Descomprimir `web.zip` en `web/<code>/`. Antes, validar cada ruta
   del zip contra *path traversal* (`..`, rutas absolutas, symlinks) y abortar si
   alguna sale de `web/<code>/`. Si el `web` va embebido, el núcleo pide el
   archivo por RPC y lo escribe igual.
3. **Menú.** Insertar las filas en `module`/`sub_module` que el módulo declara,
   y concederlas al rol administrador en `role_permission`. Es exactamente lo que
   hoy hacen a mano los seeds; la diferencia es que viene del módulo.
4. **Rutas.** Registrar la tabla de rutas del módulo. En `inproc` es
   `Module.Register(env)`; en subproceso, el núcleo monta un *proxy* que reenvía
   al binario.
5. `status='active'`, `activated_at=now`, `last_error=NULL`.

**Idempotencia:** reactivar un `active` de la misma versión no re-ejecuta nada
(el ledger ya está, el zip ya está desplegado, el `INSERT IGNORE` de menú no
duplica).

### 5.3 `desactivar` (acción del administrador)

**No destructivo**, y por eso barato y reversible:

1. Desmontar las rutas del módulo.
2. Poner `sub_module.status=0` para sus directorios (desaparece del menú).
3. `status='disabled'`, `deactivated_at=now`.

**No borra tablas ni la carpeta `web/<code>/`.** Desactivar es "no lo uses
ahora", no "bórralo". Reactivar es volver a montar rutas y poner `status=1`.

### 5.4 `desinstalar` (acción del administrador, con confirmación)

Pide confirmación explícita y ofrece dos niveles:

1. **Desinstalar (conserva datos):** desmonta rutas, borra filas de menú y
   `role_permission`, borra la carpeta `web/<code>/`, deja las tablas, elimina la
   fila de `module_registry`. Las migraciones quedan en el ledger (si se
   reinstala, no se re-ejecutan; correcto, las tablas siguen ahí).
2. **Desinstalar y purgar:** además ejecuta el SQL de reversa que el módulo
   declare (`Uninstall.sql`, con los `DROP TABLE` **explícitos**) y borra sus
   entradas del ledger.
   - **Si el módulo no declara reversa, no se purga automáticamente**: se avisa y
     se ofrece el nivel 1. Nunca se adivinan `DROP`.

### 5.5 `actualizar` (misma `code`, `version` nueva)

Se trata como un `activar` sobre un módulo ya activo: el ledger aplica solo las
migraciones nuevas (checksum distinto), se reemplaza `web/<code>/`, el menú se
reconcilia (altas/actualizaciones, no duplicados). Si la versión nueva cambia
`api` a más de `MASTER_API`, se rechaza antes de tocar nada.

### 5.6 Reglas de tolerancia

- Un módulo en `error` **no impide arrancar el ERP** ni a los demás módulos:
  mismo criterio que `internal/queue` y que el registro de la Fase 0.
- `error` es recuperable: se puede reintentar la operación que falló.
- La fila en `error` conserva `last_error` con el paso y el mensaje.

---

## 6. Tabla de registro — `module_registry`

Nueva, **separada** de `module`/`sub_module` (sección 1). Se crea con el ledger
existente, como `migrations/schema_z_modules.sql` (prefijo `z` = se aplica
después del esquema base).

```sql
CREATE TABLE IF NOT EXISTS `module_registry` (
  `code`           varchar(64)  NOT NULL,
  `name`           varchar(128) NOT NULL,
  `description`    text,
  `version`        varchar(32)  NOT NULL,
  `api`            int          NOT NULL DEFAULT 1,
  `entry`          varchar(255) NOT NULL,
  `binary_sha256`  char(64)     NOT NULL,
  `rif_binding`    varchar(32)  NOT NULL,
  `runtime`        varchar(16)  NOT NULL DEFAULT 'inproc',
  `signature`      varchar(255) NOT NULL DEFAULT '',
  `status`         varchar(16)  NOT NULL DEFAULT 'detected',
  `menu_mid`       int          NULL,   -- fila creada en `module` al activar
  `available_version` varchar(32) NULL,  -- versión nueva detectada sin activar
  `detected_at`    datetime     NOT NULL,
  `activated_at`   datetime     NULL,
  `deactivated_at` datetime     NULL,
  `last_error`     text         NULL,
  PRIMARY KEY (`code`),
  KEY `idx_estado` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

`menu_mid` guarda el `id` de la fila creada en `module` para poder deshacer sin
buscar por nombre. `status` es un `varchar` y no un `enum` a propósito: añadir un
estado en el futuro no debe obligar a una migración de tipo.

**Idempotencia:** todas las altas usan `INSERT ... ON DUPLICATE KEY UPDATE`
sobre `code`; el arranque nunca duplica filas.

---

## 7. Contrato en Go

Se añade al contrato `Module` de `doc/PLAN-MODULAR-2.0.md` la parte de ciclo de
vida. La misma interfaz sirve para `inproc` y para el cliente RPC.

```go
// internal/module/module.go

// Manifest es el lado Go del module.json (sección 3).
type Manifest struct {
    Schema       int    `json:"schema"`
    Code         string `json:"code"`
    Name         string `json:"name"`
    Description  string `json:"description,omitempty"`
    Version      string `json:"version"`
    API          int    `json:"api"`
    Entry        string `json:"entry"`
    BinarySHA256 string `json:"binary_sha256"`
    RIFBinding   string `json:"rif_binding"`
    Runtime      string `json:"runtime"`
    Web          string `json:"web,omitempty"`
}

type Module interface {
    // --- identidad y montaje (ya en el plan) ---
    Code() string
    Name() string
    Version() string
    Register(env Env)
    Migrations() []string
    Menu() []MenuEntry
    Permissions() []Permission
    Assets() fs.FS
    SchemaCatalog() []TableCatalog

    // --- ciclo de vida ---
    // Install entrega al núcleo lo que hay que aplicar; NO aplica nada él.
    Install(inst Installer) error
    // Activate monta lo que Register dejó listo; Deactivate lo desmonta.
    Activate(env Env) error
    Deactivate() error
    // Uninstall declara (si existe) el SQL de reversa para el purgado.
    Uninstall(inst Installer) error
}

// Installer es la capacidad que el NÚCLEO ofrece al módulo durante install.
// El módulo no tiene *sql.DB: pide, y el núcleo ejecuta bajo su transacción
// y su ledger. Es la regla dura 1 hecha interfaz.
type Installer interface {
    ApplyMigrations(files []Migration) error        // namespaced por el núcleo
    DeployAssets(fsys fs.FS, dest string) error     // valida path traversal
    DeclareMenu(entries []MenuEntry) error          // escribe module/sub_module
}
```

### El handshake

El manifiesto en disco puede mentir; el binario tampoco es de fiar. El
handshake **reconcilia** ambos y solo entonces se marca `detected`.

```go
// RPC — independiente del transporte (sección 8).
type Host interface {
    Hello(ctx context.Context, empty struct{}) (Manifest, error)
}
```

El `Manifest` devuelto debe coincidir en `Code`, `Version` y `API` con el
sidecar. Discrepancia = cuarentena (sección 4).

---

## 8. Transporte

El contrato de la sección 7 no depende del transporte. Se implementa detrás de
una interfaz para poder cambiar sin tocar el manifiesto ni el ciclo de vida.

- **Fase 0/1 (in-process):** `runtime: inproc`. No hay RPC: el `Module` se llama
directamente. Es donde se valida todo el ciclo de vida.
- **Fase 2 (subproceso):** `runtime: netrpc` (recomendado) o `grpc`.

**Disidencia sobre gRPC** (ya anotada en el plan): obliga a `protoc` + protobuf
fuera del árbol de Go y rompe el "compilar con solo Go". Recomendación: empezar
con `net/rpc` sobre loopback detrás de la misma interfaz y migrar a gRPC solo si
se necesita *streaming* o clientes en otros lenguajes. Esbozo del servicio, por
si llega ese día:

```proto
service ModuleHost {
  rpc Hello      (HelloRequest)      returns (Manifest);
  rpc Migrations (Empty)             returns (MigrationList);
  rpc WebBundle  (Empty)             returns (stream Chunk);
  rpc Menu       (Empty)             returns (MenuList);
  rpc Routes     (Empty)             returns (RouteList);
  rpc Activate   (ActivateRequest)   returns (Empty);
  rpc Deactivate (Empty)             returns (Empty);
}
```

---

## 9. Encaje en las fases

| Pieza | Fase 0 (in-process) | Fase 2 (subproceso) |
|---|---|---|
| `module_registry` + estados | Sí | Sí (sin cambios) |
| `Manifest` y verificación de firma | Se implementa y **se prueba con módulos in-process** | Ahora también firma binarios |
| `Install/Activate/Deactivate/Uninstall` | Sí, los ejecuta el núcleo | Igual, por RPC |
| `web/<code>/` | Sí (assets del módulo) | Web desplegada desde `web.zip` |
| `descubrir` escaneando `addons/` | No (módulos compilados) | Sí |
| `net/rpc` o gRPC | No | Sí |

**La regla de oro sigue siendo la del plan:** primero el dominio y los contratos
(Fase 0), después los procesos separados (Fase 2). Ninguna pieza de la Fase 2
necesita existir antes de que la Fase 0 tenga el sidebar dinámico y una suite en
verde.

---

## 10. Resumen ejecutivo de decisiones

1. **Registro separado:** `module_registry` ≠ `module`/`sub_module`. El núcleo es
   el único que escribe el menú.
2. **Manifiesto en disco + firma aparte:** `module.json` + `module.sig`; se firma
   el archivo tal cual, sin canonicalizar JSON.
3. **`binary_sha256` dentro del manifiesto firmado:** ata binario y procedencia.
4. **Firma Ed25519 fijada al RIF:** un módulo de un cliente no arranca en otro.
5. **Cinco operaciones idempotentes:** descubrir, activar, desactivar (no
destructivo), desinstalar (conservar o purgar), actualizar.
6. **Tolerante a fallos:** un módulo roto no impide arrancar el ERP.
7. **Transporte tras interfaz:** `inproc` ahora, `net/rpc` o gRPC después.
