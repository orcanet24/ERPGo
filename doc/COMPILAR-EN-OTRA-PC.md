# Compilar ERPGo en otra computadora

Guía para dejar el proyecto compilando en un equipo distinto al de desarrollo.
Está escrita a día 3 de octubre de 2026 y todo lo de aquí está **verificado en
la máquina de desarrollo**, no supuesto.

---

## 1. Lo mínimo para compilar (el día a día)

Esto es lo único que hace falta para trabajar:

| Requisito | Versión | Nota |
|---|---|---|
| Go | **1.26.5 o superior** | La exige `go.mod` |
| Sistema | Windows, macOS o Linux | el proyecto es solo Go, sin C |

Compilación normal, la de siempre:

```bash
go build ./cmd/server
```

Y para arrancar y probar: [iniciar.bat](../iniciar.bat) en Windows, que compila y
levanta el servidor.

**No hace falta nada más.** El proyecto hoy **no necesita compilador de C**:
el driver de SQLite es `github.com/glebarez/go-sqlite`, que es SQLite escrito
en Go puro. Por eso `CGO_ENABLED=0` y compila en cualquier parte.

### Comprobación rápida de que todo va bien

```bash
go build ./...                                    # exit 0
go vet ./internal/queue/ ./cmd/server/            # exit 0
go test ./internal/queue/ ./cmd/server/ -count=1  # ok
go test ./interfaces/web/ -count=1                # ok (tarda ~40 s)
go run ./internal/webcheck -dir templates         # 115 plantillas, 0 errores
```

---

## 2. Solo para el binario de PROTECIDO (FieldKey puesta)

Además de lo anterior:

- La FieldKey, en un archivo **fuera del repositorio**:
  `%USERPROFILE%\.erpgo\fieldkey.txt`. Se crea solo la primera vez.
  El valor de desarrollo es `ORCANET-FIELDKEY-2026-ERP` (documentado en
  [privado/KEYS.md](privado/KEYS.md), que **no debe distribuirse**).

Se compila con [compilar-protegido.bat](../compilar-protegido.bat), que es
rápido (unos 70 s) y deja el binario normal intacto.

### Dos avisos que ya se han dado y hay que respetar

1. **Un `.bat` tiene que guardarse con saltos CRLF.** Con LF, `cmd.exe`
   parte las líneas y falla con errores como `"olor" no se reconoce`. Los
   tres `.bat` del proyecto están en CRLF; si se editan, hay que mantenerlo.
2. Las redirecciones son `>nul`, **no** `>/dev/null`: eso es de bash y en
   Windows no existe.

---

## 3. Solo para el binario de LANZAMIENTO (el que se entrega)

Añade:

| Requisito | Por qué |
|---|---|
| **Go 1.27 o superior** | `garble` v0.18.0 **no funciona** con Go 1.26 |
| `garble` v0.18.0 o superior | el ofuscador |

```bash
go install mvdan.cc/garble@latest
go install golang.org/dl/go1.27.1@latest
go1.27.1 download
```

**Esta es la trampa más importante del proyecto.** Compilando con el Go del
sistema (1.26), garble aborta con:

```
Go version "go1.26.5" is too old; please upgrade to go1.27.0 or newer
```

[compilar-lanzamiento.bat](../compilar-lanzamiento.bat) ya busca el Go 1.27 solo y
falla con un mensaje claro si no lo encuentra. **Tarda unos 3 minutos**: es
normal, no un cuelgue.

---

## 4. Lo que FALTA por hacer (pendiente real)

Esto es lo que queda del plan aprobado. **Nada de esto está implementado.**

### 4.1 Cifrar la base de datos con SQLCipher

**Estado**: la mecánica existe (`cmd/server/fieldkey.go`, con sus 6 tests en
verde) pero **no está conectada**. `OpenSQLite` no recibe la llave, así que
**la base de datos se abre en claro**.

**El compilador de C sí está disponible.** MSYS2 con mingw-w64 está instalado
en `C:\msys64`, con gcc 16.1.0. Lo que faltaba era **añadirlo al PATH**, y no
se había hecho. Verificado el 03-10:

```bash
# solo para ESTA orden; nada se guarda en el PATH del sistema
PATH="/c/msys64/mingw64/bin:$PATH" CGO_ENABLED=1 go build ./cmd/server
```

En Windows, dentro de un `.bat`, lo mismo con `setlocal`:

```bat
setlocal
set "PATH=C:\msys64\mingw64\bin;%PATH%"
set "CGO_ENABLED=1"
go build -p=2 -o erpgow-cgo.exe ./cmd/server
endlocal
```

El proyecto completo compila con CGO activado, así que **SQLCipher no está
bloqueado por falta de herramientas**.

> En una terminal normal de Windows el camino es `C:\msys64\mingw64\bin`
> (gcc 16.1.0 de MSYS2, usuario `orcanet24`).
>
> **NO añadirlo a las variables de entorno del sistema.** Decisión del usuario:
> el PATH del sistema se queda como está. El gcc se usa solo de forma local,
> en la misma línea de comando o dentro del script que compila, que además
> va con `setlocal`, así que al salir del script no queda rastro.

**Qué falta hacer**:

1. Añadir al `go.mod` el driver con SQLCipher, por ejemplo
   `github.com/mutecomm/go-sqlcipher/v4`, y aplicarlo en `OpenSQLite`
   (`infrastructure/persistence/database.go:40`) mediante `PRAGMA key`.
2. **Migrar las bases que ya están en claro**, sin perder datos.
3. **Reset de llave desde la aplicación.** Si un cliente pierde la llave y no
   hay recuperación, se ha perdido su negocio. Es la parte más urgente de
   esta lista.

> Nota: cambiar a un driver con CGO tiene un efecto secundario a comprobar:
> la compilación deja de ser trivial. Ya se verificó que el proyecto entero
> compila con `CGO_ENABLED=1` y el gcc de MSYS2, pero conviene repetirlo en
> el equipo de destino antes de dar por buena cualquier otra cosa.

### 4.2 El orquestador de escrituras — HECHO (03-10)

**Estado**: la puerta está construida y conectada. Ya no hace falta crearla.

- `internal/orchestrator/` — el mecanismo: journal cifrado en disco con
  correlativo propio, escritor único, idempotencia por clave y rastro
  obligatorio.
- `interfaces/web/orchestrator.go` — un orquestador por empresa, con su journal
  en `data/escrituras/<empresa>.journal`.

Reutiliza, como estaba previsto: la idea de la cola de `internal/queue`,
`audit_log` (`old_value`/`new_value`) y la cadena de `audit_chain_blocks`.

**Lo único que falta**: que los handlers usen la puerta. Hoy siguen escribiendo
por su cuenta. El test que lo fuerza (que falle si aparece un INSERT fuera de
la puerta en módulos de negocio) es lo que obliga al resto.

### 4.3 Configuración de accesos, API y fork de conexión

Sin empezar:

- Tabla de permisos y credenciales (solo admin y super-admin).
- API con facturas, notas de crédito y débito, clientes, inventario y
  productos, con roles y apikeys.
- Fork de conexión tipo MySQL, con puerto, usuario y clave, enrutando cada
  operación al mismo sitio.

---

## 5. Advertencias sobre el código actual

Cosas que se han corregido pero conviene conocer:

1. **La cola ya no rechaza: frena.** `Enqueue` espera hueco en vez de
   devolver `ErrFull` (esto era lo que perdía ventas). Con el escritor parado
   más de `EnqueueTimeout` sí devuelve `ErrFull`. Por defecto son 30 s, pero
   el scheduler de IA usa 2 s a propósito (`interfaces/web/worker.go`).
2. **Un test cambió de significado**: `TestQueueBackpressure` pedía que la
   cola rechazara al llenarse. Ahora comprueba el frenado. No está debilitado:
   sigue exigiendo el tope y el error correcto.
3. **TRES TESTS RECONSTRUIDOS DE MEMORIA — REVISARLOS.** Al invertir
   `queue_test.go` se perdieron sin querer `TestQueueRunsTask`,
   `TestQueueRejectsUnknownKind` y `TestQueueRequiresTenant`, y se volvieron a
   escribir de memoria. **Pasan, pero no son necesariamente los originales**:
   pueden comprobar menos de lo que comprobaban. Conviene mirarlos antes de
   fiarse de ellos como red de seguridad. También se reconstruyó el helper
   `testStore` (con el DSN de producción).
4. **El DSN de la cola exige `_txlock=immediate`** (WAL + busy_timeout).
   Sin eso los workers chocan con `SQLITE_BUSY` y la cola nunca se vacía. Está
   documentado en la cabecera de `queue.go` y en el helper de test.
5. **`static/` y `templates/` van embebidos en el binario.** Cambiar el CSS o
   una plantilla no se ve recargando: hay que **cerrar el proceso, recompilar y
   arrancar**. Es la causa más frecuente de "no me aparece el cambio".
6. **Memoria**: con 8 GB y varias compilaciones en paralelo, Go falla con
   `errno=1455`. Los scripts usan `-p=2` por eso. Si aun así falla, compilar en
   un equipo más tranquilo o bajar a `-p=1`. En este equipo, `-p=1` ha resuelto
   siempre.
7. **`templates_test.go:184` tiene dos caracteres chinos colados** en un
   comentario, donde debería decir «sí». No rompe nada —está en un comentario,
   no en código— pero ensucia el repositorio. No se ha tocado porque esa sesión
   sigue trabajando en el fichero.

---

## 6. Estado verificado a 3 de octubre de 2026

```
go build ./...                                     exit 0
go vet ./...                                       exit 0
go test ./internal/... ./cmd/server/ -count=1       ok
go test ./interfaces/web/ -count=1                 ok  (26 s)
go run ./internal/webcheck -dir templates          115 plantillas, 0 errores
CGO_ENABLED=1 go build ./cmd/server                exit 0  (44,3 MB)
```

Además, arrancar el binario de verdad y comprobar que el orquestador levanta:
el log dice `orquestador de escrituras activo para default` y el journal
aparece en `data/escrituras/default.journal` con 32 bytes (solo la cabecera).

**Pendiente de verificar**: el paso 6 de `compilar-lanzamiento.bat` (sello
SHA-256) no se ha ejecutado de principio a fin; la compilación de garble sí se
verificó aparte (3 min 12 s, 42,9 MB, arranca y sirve el login).

> Nota sobre memoria: con `-p=1` las compilaciones van bien en este equipo de
> 8 GB. Sin eso, dos compilaciones en paralelo (por ejemplo, con otra sesión
> trabajando a la vez) fallan con `errno=1455`.

---

## Referencias

- [HOJA-DE-RUTA.md](HOJA-DE-RUTA.md) — hoja de ruta única
- [SEGURIDAD.md](SEGURIDAD.md) — diseño de las medidas del binario
- [BITACORA-SEGURIDAD.md](BITACORA-SEGURIDAD.md) — historial de seguridad
- [Planllave de SQLite, orquestador…](Planllave%20de%20SQLite,%20orquestador%20de%20escrituras,%20API%20y%20conexión%20tipo%20MySQL.md) — plan del orquestador y el acceso controlado
- [privado/KEYS.md](privado/KEYS.md) — **privado**, no distribuir