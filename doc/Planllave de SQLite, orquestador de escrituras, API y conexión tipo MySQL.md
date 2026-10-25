# Plan: llave de SQLite, orquestador de escrituras, API y conexión tipo MySQL

> **Estado: aprobado. Pasos 1 y 3 hechos y verificados (03-10); el resto pendiente.**
> Guía de compilación en otra PC: [COMPILAR-EN-OTRA-PC.md](COMPILAR-EN-OTRA-PC.md)

Lo pedido son cinco piezas. Ninguna depende de que se apruebe presupuesto aparte: van juntas porque **el orquestador es la única puerta**, y tanto la API como el fork de conexión entran por ahí. Separadas serían mecanismos que se contradicen.

## Lo que ya existe (revisado hoy)

| Pieza | Dónde | Estado |
|---|---|---|
| Cola con workers fijos y journal en disco | `internal/queue/queue.go`, 460 líneas | Existe |
| Log con valor anterior y nuevo | `audit_log` (`old_value`, `new_value`, usuario, IP) | Existe |
| Bloques mensuales con hashes encadenados | `audit_chain_blocks` (`prev_block_hash` → `block_hash`) | Existe |
| FieldKey y sello | `cmd/server/fieldkey.go` | Existe, desconectado |
| API | `server.go` | 1 ruta: `/api/login` |

El borrado lógico con registro que secharted ya es `audit_log`, y la cadena de bloques que sella el mes también. Eso no se construye: se conecta.

---

## 1. Clave de SQLite

- `caae4c3c5cee440d10b89a315ea01bb0`, guardada en `doc/privado/KEYS.md` (ya hecho, fuera del versionado).
- SQLCipher en `OpenSQLite` (`infrastructure/persistence/database.go:40`): recibe la llave y la aplica al abrir.
- La llave viaja en el binario de entrega, la escribe `-X main.fieldKeyCifrada=...` desde `compilar-protegido.bat` y `compilar-lanzamiento.bat`, que ya existen.
- Migración de las bases que hoy están en claro, sin perder datos, con prueba de que se pueden leer después.
- **Reset de llave desde la aplicación.** Si un cliente la pierde y no hay recuperación, se ha perdido su negocio. Es la parte urgente de esta pieza.

Con esto, un binario de SQLite del sistema —el del técnico— no abre nada sin la llave. Efecto secundario que además resuelve lo del Navicat: no hay cliente externo que pueda saltarse las reglas.

## 2. Orquestador de escrituras — HECHO (03-10)

**Dónde**: `internal/orchestrator/` (el mecanismo) y
`interfaces/web/orchestrator.go` (la conexión al servidor).

Todo lo que pedía esta sección está construido y probado:

- **Correlativo propio**: la posición en la fila, no el `id` de SQLite. Vive en
  la cabecera del journal, en disco, así que sobrevive a un reinicio y no se
  reinicia nunca.
- **El archivo se limpia, no se borra**: tras una pausa sin escrituras queda en
  su cabecera, y se puede preguntar "¿hay cola?" sin comprobar antes si existe.
  Limpiar **conserva el correlativo**, comprobado releyendo la cabecera.
- **Nunca rechaza; bloquea el productor** si está lleno.
- **Idempotencia por clave única**: la misma clave dos veces escribe una vez,
  incluso tras una caída.
- **Limpieza tras la pausa**, cuando no llega nada y SQLite ha confirmado todo.
- **Cifrado con la FieldKey** (AES-GCM). Sin FieldKey va en claro.
- **Requisito duro cumplido**: el INSERT en `audit_log` va **en la misma
  transacción** que el dato. Si el rastro no se puede escribir, se deshace todo
  y el dato no entra. Hay un test que lo rompe a propósito para comprobarlo.

**Lo que falta para cerrar esto del todo**: migrar los handlers. Hoy hay 97
INSERT en el código que siguen escribiendo por su cuenta. La puerta existe y
funciona; el trabajo que queda es poner los módulos de negocio a usarla, y el
test que lo fuerza (que falle si aparece un INSERT fuera de la puerta) es lo
que obliga al resto.

El fallo previo ya está corregido: `Enqueue` devolvía `ErrFull` en vez de
esperar, y ahora frena.

## 3. API

Con niveles de roles y autorización por apikey. Todos con el mismo camino que el sistema: por el orquestador, con las mismas validaciones.

| Operación | Regla |
|---|---|
| Crear factura, nota de crédito, nota de débito | Por el orquestador; libros fiscales igual que en pantalla |
| Crear y editar clientes | Por el orquestador, con validación de RIF |
| Borrar clientes | **Solo si no tienen movimientos.** Con movimientos, borrado lógico a `audit_log` |
| Consultar inventario, productos y precios | Solo lectura |

El borrado de clientes es **idéntico** por API que por el propio sistema: es la misma regla, no dos.

Cada apikey tiene su usuario, su rol y su alcance. Se caduca.

## 4. Fork de conexión tipo MySQL

Puerto, usuario y clave, para que un cliente de base de datos se conecte y **controlar lo que se hace por esa vía**.

El fork habla el protocolo en su propio puerto y **enruta cada operación al mismo sitio**: las lecturas leen, y todo lo que escriba pasa por el orquestador con las reglas y el registro. Un `DELETE` sin movimientos pasa; con movimientos, queda como borrado lógico registrado y reversible. Un `UPDATE` que rompa la cadena de bloques se rechaza.

Por eso sirve: el cliente ve su base de datos, y no puede hacer nada que el sistema no haría.

## 5. Configuración de accesos

Pantalla única de **accesos**, desde el sistema:

- Accesos del fork de conexión: puerto, usuario, clave, rol.
- API keys: usuario, clave, rol, alcance, caducidad.
- Solo admin y super-admin crean y revocan accesos. Queda siempre quién lo creó.

## Orden

1. ~~Arreglar la pérdida de ventas del `Enqueue`.~~ **HECHO Y VERIFICADO el 03-10**: ahora frena y espera hueco en vez de rechazar. Comprobado con 60 inserciones y capacidad 4: cero perdidas, en 0,23 s.
2. Clave de SQLite (SQLCipher + migración + reset).
3. ~~Orquestador, conectado a `audit_log` sin atajo.~~ **HECHO Y VERIFICADO el 03-10**: la puerta existe, el rastro va en la misma transacción y hay 18 pruebas. Queda migrar los handlers y el test que lo fuerza.
4. Configuración de accesos (tabla de permisos y credenciales).
5. API.
6. Fork de conexión tipo MySQL.
7. Botón de "pendiente de aprobación" para el caso de que alguien pida un cambio que no puede ejecutar.

## Verificación de cada paso

`go build ./...`, `go vet`, `go test ./interfaces/web/ ./cmd/server/ ./internal/queue/`, `webcheck` de las 115 plantillas, y una prueba funcional de la operación nueva entrando por API y por el fork, comparando el resultado con el de la interfaz. Sin atajos: si una vía puede saltarse el orquestador, la prueba tiene que fallar.