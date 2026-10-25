# Plan de Arquitectura Modular — ERPGo 2.0

**Estado:** decide. Este documento **gobierna** sobre `doc/Arquitectura Modular.md`.
**Fecha:** 2026-10-04.
**Ámbito:** cómo fragmentar ERPGo en módulos sin perder el norte.

> **2026-10-08 — Addons externos (decisión tomada: ecosistema abierto):** el
> modelo de **sub-binarios en `addons/` con SDK público y marketplace opcional**
> está especificado en **[PLAN-ADDONS-LICENCIA.md](PLAN-ADDONS-LICENCIA.md)**.
> Se descarta la licencia obligatoria por hardware; la firma Ed25519 queda solo
> como insignia "Verificado" opcional. El núcleo (`erpgow.exe` + fuente) sigue
> cerrado; lo que se abre son contratos y capacidades vía SDK. Reutiliza la espec
> de manifiesto/ciclo de vida de `ESPEC-MANIFIESTO-MODULO.md`. La Fase 0
> in-process sigue siendo refactor interno opcional, no prerrequisito de los
> addons externos.

---

## 0. Por qué este documento existe

`doc/Arquitectura Modular.md` propone un núcleo con *addons* de sub-binarios,
comunicados por gRPC/IPC y firmados con Ed25519, más inyección dinámica de UI y
licencia por RIF. La Tertulia #3 (`doc/tertulia/2026-10-04-modularidad-go-php.md`)
llegó a una conclusión más barata: **monolítico con módulos internos y flags**.

Las dos ideas no se contradicen: la primera es la **Fase 3** de la segunda. Este
documento fija el orden y el contrato, y marca qué del plan original se hace ya,
qué se aplaza y qué se descarta.

La regla de oro: **primero el dominio y los contratos, después (si acaso) los
procesos separados.** Construir IPC antes de desacoplar es construir un puente a
la nada.

---

## 1. Estado real verificado (medido, 2026-10-04)

| Dato | Valor |
|---|---|
| Lenguaje | Go 1.26.5, CGO opcional |
| Archivos Go / líneas | 179 archivos, ~60.000 líneas |
| `interfaces/web` | **92 archivos de código, 34.789 líneas, un solo paquete `web`** |
| Métodos sobre `*Server` | 654 |
| Registro de rutas | **42 funciones `registerXRoutes()`** invocadas en `NewServer` |
| Plantillas | 126 · `layout.html` 800 líneas con ~69 gates `index .Perm` |
| Menú lateral | Ítems **hardcodeados** en `layout.html`; la visibilidad sí viene de `sub_module` |
| `sub_module` | 58 filas; `role_permission` da permisos CRUD por rol |
| Capa de dominio | **No existe como paquete.** La lógica vive dentro de los handlers |
| SQL directo | 64 ficheros |
| Auditoría | `audit_chain` SHA-256 encadenada + `internal/orchestrator` (puerta única de escritura) |
| Motor de datos | **MySQL** (una base por empresa). SQLite queda como historial legacy |
| Control de versiones | Inicializado el 04-10 (commit `9f66466`), línea base de `go build ./...` verde |

### Lo que ya juega a favor del plan

- **El módulo ya existe como convención**: cada dominio es un archivo con su
  `registerXRoutes()`. Ese es el punto de costura que el plan necesita.
- `sub_module` + `role_permission` **ya filtran el menú por base de datos**.
- `audit_chain` + orquestador **ya son "el Core como único notario"**: ningún
  módulo debe escribir auditoría, ya se resuelve en el núcleo.
- `embed.go` con overlay **disco-primero** ya permite pisar templates/static.
- `internal/queue` ya da trabajo en segundo plano con backpressure.

### Lo que NO se debe asumir

- No hay contrato entre módulos: hay 654 métodos sobre un `*Server` y SQL suelto.
- El sidebar no es dinámico: agregar un módulo hoy exige editar `layout.html`.
- En Windows, el paquete `plugin` de Go **no existe**. La única vía de añadir
  funcionalidad **después de compilar** sin recompilar el core es un subproceso.

---

## 2. Decisión

1. **Se hace en este repositorio, incremental.** No hay rewrite big-bang. Los
   activos que no se arriesgan: localización fiscal SENIAT, CONASEV, IGTF,
   retenciones, auditoría encadenada y ~221 tests.
2. **"ErpGO2.0" es una rama/etiqueta de versión, no un repositorio nuevo.**
3. **Mecanismo de módulos: in-process + build tags.** Los sub-binarios con IPC
   se aplacen a Fase 3, y solo si el negocio necesita módulos descargables que
   funcionen sin recompilar.
4. **Si algún día se hace la Fase 3, el transporte es `net/rpc` sobre loopback o
   HTTP con handshake, NO gRPC**: `protobuf` + `protoc` rompería el orgullo del
   proyecto ("compilar en otra máquina con solo Go").

---

## 3. El contrato: interfaz `Module`

Todo módulo implementa esto. La misma interfaz sirve para el módulo in-process
(Fase 0-1) y, más tarde, para un cliente RPC que hable con un subproceso.

```go
// internal/module/module.go
package module

// Module es la unidad de extensión del ERP. Un módulo declara sus rutas,
// sus migraciones, su menú, sus permisos y sus assets. El núcleo no sabe
// qué módulos hay: los recorre a todos por igual.
type Module interface {
    // Code es el identificador estable ("carniceria", "textil"). Nunca cambia:
    // va en la firma del addon y en la tabla `module`.
    Code() string
    Name() string
    Version() string

    // Register monta las rutas del módulo sobre el router del núcleo.
    // Recibe el contexto (server, db, permisos) — NO un *sql.DB suelto.
    Register(env Env)

    // Migrations devuelve los .sql del módulo; el núcleo los aplica con el
    // ledger existente (schema_migrations) y su checksum.
    Migrations() []string

    // Menu declara las entradas del sidebar que este módulo aporta.
    Menu() []MenuEntry

    // Permissions declara los directorios (sub_module) que el módulo necesita.
    Permissions() []Permission

    // Assets devuelve los templates/static propios. El núcleo los combina con
    // los embebidos usando el overlayFS que ya existe.
    Assets() fs.FS

    // SchemaCatalog es lo que la IA puede ver de sus tablas (paso 7 y 8 del
    // PATRON-UNICO-DE-DESARROLLO). Si no lo declara, la IA no ve el módulo.
    SchemaCatalog() []TableCatalog
}
```

### Reglas duras (no son estilo)

1. **Un módulo nunca abre su propia conexión.** Recibe el `Env` del núcleo y
   usa `dbFor(r)`. El aislamiento por empresa es del núcleo.
2. **Un módulo nunca escribe auditoría.** Pide la transacción al núcleo
   (orquestador) y el núcleo es el único que calcula el hash encadenado.
3. **Un módulo declara su propio `SchemaCatalog`.** Si no, es invisible para la
   IA — el fallo silencioso que ya documentó el PATRON-UNICO-DE-DESARROLLO.
4. **El registro es idempotente y tolerante a fallos**: un módulo que no carga
   no impide arrancar el ERP, solo avisa (mismo criterio que la cola).

---

## 4. Fases

### Fase 0 — Seam modular in-process (2–4 días)

Sin procesos, sin IPC, cero regresión.

1. Crear `internal/module` con la interfaz y un `Registry`.
2. Escribir la tabla `module` (code, name, version, enabled, signature,
   rif_binding, installed_at) e integrarla al menú (render desde BD).
3. **Volver el sidebar dinámico**: `layout.html` deja de ser 800 líneas de HTML
   fijo y pasa a recorrer las entradas de los módulos activos. — **HECHO**: el
   menú se lee de la tabla `menu`.
4. Convertir los 42 `registerXRoutes()` en módulos. **Uno a uno, con la suite en
   verde en cada paso.**
5. Adaptar el checklist de 9 pasos del PATRON-UNICO-DE-DESARROLLO para módulos.

**Entregable:** el sistema hace exactamente lo mismo, pero cada dominio es un
módulo registrado, se puede habilitar/deshabilitar, y añadir un módulo ya no
exige editar `layout.html`.

### Fase 1 — Módulos por build tags (1–2 semanas)

El objetivo real ("no inflar el binario del comerciante") sin IPC.

1. Cada módulo de sector va detrás de un build tag: `//go:build carniceria`.
2. Un binario por sector compilado desde un solo código: `erpgow.exe` (núcleo)
   y `erpgow-carniceria.exe`, etc.
3. El instalador elige qué binario entrega según la licencia.

**Entregable:** el comerciante de una ferretería no carga el módulo cárnico.

### Fase 2 — Addons de subproceso + firma (solo si se necesita)

Aquí sí se implementa `doc/Arquitectura Modular.md` tal cual, ahora que la
interfaz `Module` permite que el "backend" sea un cliente RPC:

1. Cargador que escanea `/addons`, valida firma y **vinculación con el RIF**.
2. Firma `Sign_Orcanet(RIF + ID_Licencia + ID_Modulo)` con `crypto/ed25519`
   (`x/crypto` ya está en `go.mod`; `edwards25519` ya es dependencia indirecta).
3. Transporte `net/rpc` en loopback con handshake, **no gRPC**.
4. El módulo pide TODA escritura al core por RPC; el core sigue siendo el único
   notario de `audit_chain`.

**Entregable:** módulos descargables que funcionan sin recompilar el core.

---

## 5. Qué del `Arquitectura Modular.md` original se conserva

| Pieza del plan original | Destino |
|---|---|
| Núcleo + módulos desacoplados | **Fase 0** (in-process) |
| Directorio `/addons` con sub-binarios | **Fase 2** (aplazado, condicional) |
| IPC / gRPC | **Fase 2**, pero con `net/rpc`, no gRPC |
| Firma Ed25519 / RSA ligada al RIF | **Fase 2** (barato; ~100 líneas) |
| Inyección dinámica de UI | **Fase 0** (sidebar desde BD) |
| Core como único notario de `audit_chain` | **Ya existe** (audit_chain + orquestador) |
| Respeto a la cadena inmutable | **Regla dura** (sección 3) |
| Licencia única por comercio | **Fase 2** (mismo mecanismo de firma) |
| Protección de código (ofuscado) | Fuera de alcance de este plan (garble) |
| Cláusula Open Source a 20 años | Legal, no ingeniería |

---

## 6. Riesgos y banderas rojas

1. **Subproceso en Windows**: antivirus, firewall en loopback y SmartScreen para
   `.exe` sin firma Authenticode. Cada módulo sube ese costo. Motivo extra para
   no hacerlo en Fase 0.
2. **Motor de datos**: MySQL está decidido y en producción; SQLite queda legacy.
   No reabrir la decisión durante el refactor.
3. **El orden manda**: no extraer un módulo a proceso separado antes de que su
   dominio esté desacoplado en la Fase 0.
4. **Tests como red**: la suite debe quedar verde en cada paso. Un refactor que
   deja la suite roja "para arreglarla después" es cómo se pierde el norte.

---

## 7. El modelo Laragon (visión del usuario, 2026-10-04)

La ambición operativa es **un directorio autocontenido** — como Laragon — no
solo *feature flags*:

```
ERPGo/
├── erpgow.exe            # binario maestro (.bat o servicio de Windows)
├── addons/               # o lib/ — un .exe por módulo
│   ├── modulo_carniceria.exe
│   └── modulo_textil.exe
├── web/                  # la web del módulo, en su propio directorio
│   ├── carniceria/       # .ts .css .html .js del módulo
│   └── textil/
├── templates/            # layout global (header/footer) ya existente
└── static/
```

### Ciclo de vida descrito

1. El maestro arranca y **escanea `addons/`**. Por cada binario nuevo lee su
   manifiesto y verifica la **firma digital asignada** (Ed25519 ligada al RIF).
2. Si la firma es válida, **lo inicializa pero NO lo activa**: queda detectado.
3. MySQL recibe una fila en `module` (code, name, version, función, firma).
   El administrador ve "módulo nuevo disponible" en el panel.
4. El admin **activa**. Al activar:
   a. el módulo **instala sus tablas** en MySQL;
   b. **descomprime su web** (TypeScript/vistas) en su directorio bajo `web/`;
   c. se crean **menú y submenús** en la tabla de menú con links a las vistas.
5. Las vistas usan por defecto la **plantilla global**, respetando header y
   footer.
6. Maestro y módulo se comunican por **gRPC**.

### Encaje en las fases

| Pieza del modelo Laragon | Fase | Nota |
|---|---|---|
| Sidebar/menú desde BD | **Fase 0** | `permissionMap` ya lee `sub_module`; falta que `layout.html` recorra la BD |
| Ciclo `Install`/`Activate`/`Deactivate` | **Fase 0** (contrato), **Fase 2** (real) | Se define ya; en Fase 0 lo ejecuta el núcleo in-process |
| Web por módulo (`web/<code>/`) | **Fase 0/1** | El `overlayFS` disco-primero **ya** permite pisar templates/static sin recompilar |
| Escaneo de `addons/` + firma Ed25519 | **Fase 2** | ~150 líneas; `x/crypto/ed25519` ya está en `go.mod` |
| Descompresión de la web al activar | **Fase 2** | Necesita formato de paquete + `archive/zip` |
| gRPC | **Fase 2** | Ver nota de transporte abajo |

### Lo que ya existe y juega a favor

- **El overlay `embed.go` es disco-primero**: un archivo puesto en `web/<code>/`
  o en `templates/` pisa al embebido **sin recompilar**. Ese es el mecanismo
  exacto que "descomprime la web del módulo" necesita; solo hay que enseñarle al
  núcleo a resolver rutas por módulo.
- El static se sirve con `fs.FS` sobre disco: añadir un prefijo
  `/modules/<code>/static/` es servir otro `fs.Sub`.
- El menú **ya** se filtra por `sub_module` en la BD: insertar las filas del
  módulo es suficiente para que aparezca (con su fila en `role_permission`).
- `layout.html` + página se combinan en `buildTemplateSet`: el módulo solo aporta
  su `content`/`pagejs`, hereda header y footer gratis.

### Lo que falta (trabajo nuevo, no existe hoy)

1. `Module` necesita el **ciclo de vida**: `Manifest()`, `Install(Installer)`,
   `Uninstall()`, además de `Register`/`Migrations`/`Menu`/`Assets`.
2. ~~**Resolución de plantillas por módulo**~~ — **HECHO** (2026-10-04). Una vista
   en `web/<code>/templates/<pagina>.html` se pinta sin recompilar; el layout se
   lee siempre del núcleo (`erpgow.GlobalTemplate`), así que el chrome global no
   depende de qué módulos haya instalados.
3. ~~**Raíz static por módulo**~~ — **HECHO** (2026-10-04). `web/<code>/static/`
   se sirve como `/static/<code>/...`.
4. **Manifiesto + firma**: no existe escaneo de directorio ni verificación. El
   descubrimiento de `web/<code>/` ya existe (`embed.go`), pero no valida firmas.
5. **Ciclo Install/Activate**: no existe; el núcleo debe ser quien aplique el SQL
   (regla dura 1) y quien escriba el menú.

### Estado de la Fase 0

- **Sidebar dinámico: HECHO** (2026-10-04). El menú sale de la tabla `menu`
  (`migrations/schema_z_menu.sql`, 100 entradas) y `layout.html` solo lo
  recorre. Ver `interfaces/web/menu.go` y `interfaces/web/menu_test.go`.
- **Contrato `Module` + `Registry`: PENDIENTE.**
- **Los 42 `registerXRoutes()`: PENDIENTE** (uno a uno, con la suite en verde).

### Nota de transporte (gRPC) — disidencia técnica

El contrato debe sobrevivir a cualquier transporte. Recomendación: definir la
interfaz de cliente RPC y **arrancar con `net/rpc`/HTTP loopback** (solo
biblioteca estándar: preserva "compilar con solo Go") y migrar a gRPC si más
adelante se necesita *streaming* o clientes en otros lenguajes. gRPC obliga a
`protoc` + protobuf fuera del árbol de Go; es un costo real, no un detalle.

### Riesgo operativo de los sub-binarios en Windows

Antivirus, firewall en el loopback, y **SmartScreen** para `.exe` sin firma
Authenticode. Cada módulo que se añade sube ese costo. Motivo adicional para no
mover nada a subproceso hasta que la Fase 0 esté firme.

### Detalle del manifiesto y el ciclo de vida

El formato del `module.json`, la firma Ed25519, los estados y las operaciones
`descubrir → detectar → activar → desactivar → desinstalar` están especificados
en **[doc/ESPEC-MANIFIESTO-MODULO.md](ESPEC-MANIFIESTO-MODULO.md)**.
