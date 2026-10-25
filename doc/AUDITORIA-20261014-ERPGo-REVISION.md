# Evaluación técnica de ERPGo — 14 de octubre de 2026

**Fecha:** 2026-10-14
**Proyecto:** ERPGo (`erpgow`, Go)
**Alcance:** documentación, arquitectura multiempresa, configuración, arranque/servicio Windows, POS gRPC, firma del binario y estado del checkout.
**Método:** inspección estática del código y diffs locales, más pruebas/vet acotados. No se modificó código de aplicación ni se accedió a una base de datos. No se probó una venta fiscal real ni el SCM de Windows.

> ## Nota de corrección — 2026-10-15
>
> Revisión del informe contra el código actual (`master`, `960253a` + cambios locales). El fondo de los ocho hallazgos **se confirmó**: siguen abiertos en el código, con estas excepciones de redacción y cita, ya corregidas en este documento:
>
> 1. **H-6 — cita equivocada.** El `GracefulStop()` sin límite temporal no está en `internal/service/service_windows.go:330-340` (ese archivo tiene **82 líneas**), sino en `internal/pos/grpc_server.go:130-136`, invocado desde `cmd/server/main.go:330`. La afirmación era correcta; el lugar, no.
> 2. **H-7 — premisa equivocada.** No es que "el modo `0600` no garantice las ACL en Windows": el código escribe `.env` con **`0644`** (`configs/config.go:149`, `appendToFile`). El problema es más simple y más grave: los permisos restrictivos no se aplican en absoluto.
> 3. **H-1, H-3 y H-5 — citas ajustadas** a las líneas reales y añadida la ubicación donde falta `grpc.Creds` y donde el AVISO de firma no detiene el arranque.
> 4. **H-8 — inventario actualizado** (el `git status` cambió: los dos informes de auditoría figuran ahora como no rastreados).
> 5. **Tabla de verificaciones actualizada** con una corrida nueva del 15-10: `go build ./...` OK, `go vet` OK en los paquetes revisados, `gofmt` con 15 archivos pendientes y tres pruebas CSRF en rojo.
> 6. **Alcance ampliado** con tres hallazgos del mismo canal POS/HTTP que la primera versión no cubría (sección "Ampliación de alcance"). La auditoría del 15-10 (`doc/AUDITORIA-20261015-ERPGo-REVISION-EXHAUSTIVA.md`) cubre el resto del sistema.
>
> Ninguno de estos hallazgos se ha reparado en el código todavía.
>
> ## Estado de las reparaciones — 2026-10-15 (tarde)
>
> Lo de abajo **ya no es cierto para H-1 a H-4 en el canal POS gRPC**: se repararon en el código después de escribir esa nota. Lo que cambió, y con qué se comprobó:
>
> - **H-1 — reparado** en la parte de autorización: los interceptores autentican cada RPC con la API key del terminal (cabecera `x-api-key` o `Authorization: Bearer`) y dejan la empresa resuelta en el contexto; los handlers ya no la adivinan. El campo `Token` de `OpenTurn`/`CloseTurn`/`ProcessPayment`/`ProcessRefund` se trata como SESIÓN del cajero (se valida contra `pos_sessions`: activa, no revocada, no expirada) y ya no como si fuera la key. `Login` exige usuario y contraseña mediante un verificador inyectado desde `interfaces/web` (misma regla que el login POS por HTTP: usuario activo, contraseña válida, departamento 4) y no devuelve token si la sesión no se escribió. `Connect` rechaza un registro sin `terminal_id` y un mensaje que declare otro terminal. La reflexión gRPC pasa a estar apagada salvo `POS_GRPC_REFLECTION=1`.
> - **H-1 — NO reparado:** el puerto 9095 sigue sin TLS (`grpc.Creds` no se usa). La recomendación de mTLS/túnel sigue pendiente y el puerto no debe exponerse.
> - **H-2 — reparado:** `processSale` reclama la venta en `pos_sale_idempotency` con clave única `(terminal_id, offline_id)` DENTRO de la transacción, antes de consumir numeración; el reintento devuelve el documento original (misma factura y mismo número de control). El **checkout HTTP del POS conserva el defecto** (ver «Ampliación de alcance»).
> - **H-3 — reparado:** se propagan los errores de cada `invoice_details`, de `LastInsertId` al crear el cliente, de la escritura de `last_used_at` (se registra, no se rechaza la conexión) y de la sesión de turno. Un renglón que falle deshace la factura completa.
> - **H-4 — reparado** para el número interno: la reserva pasa a `UPDATE pos_counters SET value = LAST_INSERT_ID(GREATEST(value, ?) + 1)`, con la fila del contador sembrada por migración, en vez de `SELECT IFNULL(MAX(invoice),999)+1`. **Sigue sin `UNIQUE` sobre `invoice.invoice_id`**: el aplicador de migraciones clasifica «Duplicate entry» como «ya estaba», así que un `ALTER TABLE ... ADD UNIQUE` fallaría EN SILENCIO si el histórico ya tuviera números repetidos. Queda como revisión de datos.
> - **H-5 a H-8 — sin tocar.**
>
> Cambios: `internal/pos/grpc_server.go`, `interfaces/web/pos_auth.go`, `cmd/server/main.go`, `POS/grpc/grpc_client.go` (el terminal ya manda su credencial en la cabecera; sin eso el servidor nuevo rechaza toda venta) y la migración nueva `migrations/schema_z_pos_integridad.sql`. Pruebas nuevas en `internal/pos/grpc_server_test.go` (18 casos, 8 contra MySQL real): el detalle está en la tabla de verificaciones.

## Opinión ejecutiva

ERPGo tiene una meta clara y bases de producto reales: una aplicación web fiscal, operación multiempresa, POS offline y un conjunto amplio de pruebas. No parece un proyecto vacío ni sin dirección. Aun así, en el código del POS gRPC revisado hay riesgos importantes: operaciones sin autorización robusta, métodos que responden éxito sin hacer el trabajo, posibilidad de duplicar una factura al reintentar una sincronización y fallos de escritura de detalles que no detienen la transacción. Por eso **no recomendaría exponer el puerto gRPC ni considerar esa ruta lista para producción fiscal** hasta corregir los hallazgos críticos y verificarlos con pruebas de integración.

La observación cubre particularmente el canal POS y algunos cambios recientes de empaquetado/servicio; no es una auditoría exhaustiva de todos los módulos ni una afirmación de que toda la app web sea insegura. El árbol local tenía numerosos cambios sin confirmar: la evaluación describe ese checkout y no equivale a aprobar una versión de release.

## Hallazgos priorizados

### H-1 · CRÍTICO — gRPC POS no aplica autorización uniforme a RPC unary

**Evidencia:** `internal/pos/grpc_server.go:140-149`: los interceptores solo registran el nombre del método y llaman al handler. `OpenTurn` y `CloseTurn` (`:488-520`) validan `req.GetToken()` como si fuera una API key mediante `validateAPIKey`; no verifican que sea una sesión activa, no revocada y no expirada de `pos_sessions`. `Login` (`:465-484`) verifica solamente API key, no usuario/contraseña; ignora el error al insertar la sesión y aun así devuelve éxito. `ProcessPayment`/`ProcessRefund` (`:563-570`) devuelven éxito incondicional sin persistir ni autenticar. El servidor se construye sin credenciales de transporte (`internal/pos/grpc_server.go:104-124`: no hay `grpc.Creds`, solo interceptores de log) y con reflexión activa (`:117`), y escucha en `:9095` (`cmd/server/main.go:312-318`) sin TLS.

**Riesgo:** un cliente con acceso de red puede llamar métodos que no validan identidad; algunas RPC reportan operaciones completadas que no ocurrieron. No debe exponerse el puerto 9095 a Internet o redes no confiables con el comportamiento actual.

**Recomendación:** definir credenciales y autorización por RPC en interceptores; separar API key de token de sesión, validar estado/expiración/tenant y vincular terminal con usuario/turno. Usar TLS/mTLS o un túnel de confianza. Métodos incompletos deben devolver `Unimplemented`/error explícito, no éxito. Añadir casos de rechazo para credencial vacía/inválida, sesión expirada/revocada y tenant incorrecto.

### H-2 · CRÍTICO — Una venta repetida puede crear varias facturas

**Evidencia:** `processSale` (`internal/pos/grpc_server.go:306-401`) siempre inserta una factura nueva. No busca `sale_id`/`offline_id` previo ni hay idempotencia visible. `ProcessSale` (`:529-560`) y `SyncOffline` (`:623-653`) vuelven a llamar esa función cuando reciben la venta. El camino HTTP del POS tiene el mismo defecto: ver "Ampliación de alcance".

**Riesgo:** si la base confirma la transacción pero se pierde el ACK, el POS puede reintentar y generar una segunda factura, consumir otra numeración fiscal y registrar el mismo cobro dos veces.

**Recomendación:** persistir un ID de venta externa con restricción única por empresa y guardar la respuesta fiscal asociada; al reintentar devolver el resultado original. Probar reenvío secuencial y concurrente, ACK perdido y reinicio entre commit y confirmación.

### H-3 · ALTO — Se ignoran errores al insertar los renglones de la factura

**Evidencia:** `internal/pos/grpc_server.go:385-388` descarta el error de `tx.Exec` al insertar cada `invoice_details`. También se ignoran `LastInsertId` (`:327`) y la escritura de `last_used_at` (`:415`). El error al insertar la cabecera, en cambio, sí se propaga (`:376-378`).

**Riesgo:** el `Commit` puede confirmar la cabecera fiscal aunque fallen uno o más productos, y el servidor puede enviar un ACK de éxito para una factura incompleta.

**Recomendación:** propagar cada error de escritura y abortar/rollback ante cualquier fallo de cabecera, detalle o numeración. Añadir pruebas que provoquen un error en un renglón y comprueben que no queda factura parcial ni ACK aceptado.

### H-4 · ALTO — `MAX(invoice)+1` no es una reserva segura bajo concurrencia

**Evidencia:** `internal/pos/grpc_server.go:331-335` obtiene el siguiente número interno con `SELECT IFNULL(MAX(invoice),999)+1`. Una transacción por sí sola no serializa necesariamente dos lecturas concurrentes de `MAX`. La columna `invoice` no tiene índice único en el esquema (`migrations/schema_reconstruido_03_sales.sql`: solo `KEY`).

**Riesgo:** ventas concurrentes pueden escoger el mismo número interno y colisionar, o quedar duplicado sin que la base lo impida. La serie fiscal inyectada no elimina esta carrera para `invoice`.

**Recomendación:** utilizar un correlativo transaccional con bloqueo (`SELECT ... FOR UPDATE` o `UPDATE ... LAST_INSERT_ID`), añadir `UNIQUE` donde corresponda, o usar una identidad interna independiente; separar explícitamente ID, número interno, número de documento y control fiscal. Confirmar restricciones únicas y hacer una prueba de concurrencia contra MySQL.

### H-5 · ALTO — La firma Ed25519 no comprueba directamente el hash del ejecutable y el fallo es solo aviso

**Evidencia:** `cmd/server/fieldkey.go:182-186` retorna éxito si falla `os.Executable`; `:198` lee el hash desde un `.sha256` y `:214` verifica la firma sobre esa cadena (`ed25519.Verify(pubKey, []byte(hashHex), firma)`), pero no calcula ahí el hash real del ejecutable. La comprobación SHA-256 del binario vive aparte (`cmd/server/main.go:233-239`) y sus errores solo se registran como aviso: el proceso sigue. Lo mismo la firma (`cmd/server/main.go:242-247`). `tools/firmar/main.go` genera un par de llaves si no encuentra la privada.

**Riesgo:** comprobar que la firma corresponde al hash escrito en un sello no demuestra por sí solo que el ejecutable actual corresponda a ese hash; además, una alteración no impide atender solicitudes. Es una señal de integridad, no un control fail-closed.

**Recomendación:** calcular SHA-256 del binario real, compararlo con el sello y verificar la firma sobre el hash calculado; validar longitudes/formato. Para compilaciones protegidas, detener el arranque si falla. Mantener la clave privada bajo custodia del proceso de release y probar binario, sello y firma alterados. La firma interna no sustituye Authenticode ni protege ante quien puede parchear el propio ejecutable.

### H-6 · ALTO — La gestión del servicio Windows puede declarar RUNNING sin readiness y ocultar fallo del POS

**Evidencia:** `internal/service/service_windows.go:51-54` inicia `app.Start()` en goroutine y comunica `svc.Running` inmediatamente, sin esperar que HTTP y gRPC estén escuchando. `cmd/server/main.go:312-318` registra el error de arranque gRPC pero no lo propaga a `Start()`. `Stop()` invoca `GracefulStop()` sin límite temporal (`internal/pos/grpc_server.go:130-136`, llamado desde `cmd/server/main.go:330`). El instalador (`internal/service/manage_windows.go:40-46`) elimina un servicio existente antes de crear el reemplazo.

**Riesgo:** Windows puede mostrar el servicio como iniciado mientras el POS no acepta conexiones; un stream persistente podría alargar el apagado; una reinstalación fallida podría dejar el servicio anterior eliminado.

**Recomendación:** añadir señales de readiness por listener, informar `RUNNING` solo al estar ambos disponibles y propagar fallos. Establecer timeout de apagado gRPC con parada forzada, y probar SCM en Windows. Actualizar servicio con rollback o reconfiguración sin borrado previo.

### H-7 · MEDIO — `.env` de primer inicio contiene secretos, se escribe con permisos amplios y depende del directorio de trabajo

**Evidencia:** `configs/config.go:130-145` genera/persiste `JWT_SECRET` y `appendToFile` (`:148-155`) crea el archivo con **`0644`**. `WriteEnvFile` (`:175-224`) serializa secretos en `.env` en texto plano; `EnsureEnv` (`:225+`) y `godotenv.Load(".env")` resuelven la ruta contra el directorio de trabajo actual. Los servicios de Windows pueden tener un working directory distinto del de la carpeta del ejecutable.

**Riesgo:** configuración divergente según cómo se inicie el programa, primer arranque fallido sin permiso de escritura y credenciales legibles por cualquier usuario de la máquina (0644 las hace legibles para todo el sistema).

**Recomendación:** definir ubicación persistente y estable para datos/configuración (p. ej. ProgramData con ACL explícita), resolverla igual para consola/servicio/instalador, escribir con permisos restrictivos reales (`0600` y ACL explícita en Windows) y preferir DPAPI/Credential Manager para secretos. Probar primera ejecución con usuario de servicio limitado y desde otra carpeta.

### H-8 · MEDIO — Hay cambios locales numerosos y no revisados como conjunto

**Evidencia:** `git status --short` (2026-10-15) muestra modificados `cmd/server/fieldkey.go`, `cmd/server/main.go`, `compilar-lanzamiento.bat`, `configs/config.go`, `go.mod`, `interfaces/web/server.go`, `internal/pos/grpc_server.go`; y sin rastrear `Makefile`, `doc/AUDITORIA-20261014-ERPGo-REVISION.md`, `doc/AUDITORIA-20261015-ERPGo-REVISION-EXHAUSTIVA.md`, `doc/PLAN-INSTALADOR-ERPGO-POS.md`, `instalador/`, `internal/service/`, `tools/firmar/` y los respaldos `proto/pos/*.pb.go.bak`. No hice ni reescribí esos cambios.

**Riesgo:** no se distingue fácilmente qué está listo, qué es temporal y qué fue verificado; el binario distribuido puede no corresponder al conjunto auditado.

**Recomendación:** clasificar cambios y artefactos antes de release, revisar el diff exacto, y compilar/probar el mismo commit que se distribuirá. No borrar ni descartar archivos locales sin aprobación explícita.

## Ampliación de alcance (15-10)

Tres problemas del mismo canal POS/HTTP que la primera versión no recogía. Detalle completo en `doc/AUDITORIA-20261015-ERPGo-REVISION-EXHAUSTIVA.md` (F2, F5, S9):

- **El checkout HTTP (`/api/pos/checkout`, `interfaces/web/pos_api.go:210`) emite la factura sin `control_no`, sin serie y sin pasar por `nextFiscalDoc`**, y asienta con cuentas literales `"1"` y `"2"` en `acc_transaction` en lugar del motor de asientos. Es el camino de venta más usado por el terminal, así que el riesgo fiscal es mayor que el del gRPC.
- **El `logout` del POS no revoca el token que sí usa el middleware**: `handlePOSLogout` escribe `is_active = 0` (`interfaces/web/pos_auth.go:196`) mientras `posAPIMiddleware` filtra por `revoked = 0` (`interfaces/web/pos_api.go:57`). El token sigue facturando hasta su expiración (12 h).
- **`SyncOffline` resuelve la empresa con `dbOfTerminal(terminal_id)`**, es decir con la API key de otra conexión ya registrada, y no valida credencial alguna en ese stream. Cualquiera que declare el `terminal_id` de una caja conectada queda autenticado con sus credenciales.

## Aspectos positivos

- La intención de separar base de control y bases por empresa aparece tanto en el diseño como en la resolución de la base POS.
- `main.go` inyecta numeración fiscal central al POS; es la dirección correcta frente a generar números localmente, aunque requiere pruebas de concurrencia/idempotencia.
- Existen pruebas de web para CSRF, sesiones, aislamiento de tenant, auditoría y ciclos fiscales; hay una base útil para endurecer regresiones.
- El README presenta de forma clara el valor del producto para pequeñas empresas; las afirmaciones de disponibilidad/fiscalidad requieren validación de punta a punta antes de tomarse como garantías.

## Verificaciones ejecutadas

| Comando | Resultado |
|---|---|
| `go vet ./internal/pos ./configs ./internal/service ./cmd/server` | Pasó (14-10) |
| `go test ./internal/pos ./configs ./internal/service ./cmd/server` | Pasó; `internal/pos`, `configs` e `internal/service` no tienen tests propios y `cmd/server` pasó (cacheado) |
| `go test ./...` | Se intentó; excedió 300 s. No se considera aprobado |
| `go vet ./...` | Se intentó, pero no quedó resultado confirmado dentro del tiempo disponible |
| `go build ./...` (15-10) | Pasó (exit 0) |
| `go vet ./interfaces/web ./internal/... ./configs ./infrastructure/persistence` (15-10) | Pasó (exit 0) |
| `gofmt -l interfaces cmd configs internal infrastructure tools` (15-10) | 15 archivos sin formatear |
| `go test ./interfaces/web -run 'TestFiscal\|TestCSRF\|TestPeriod\|TestTenant\|TestKardex'` (15-10) | **Falló**: las tres pruebas de CSRF piden la base de empresa `erpgo_t1_26100__default` y el arnés provisiona `..._default` (un solo guion bajo): `PrefijoDePrueba` ya devuelve el prefijo terminado en `_` (`infrastructure/persistence/testdb.go:248`) y `tenantDBName` añade otro (`interfaces/web/tenants.go:92`). `TestCSRFSafeMethodsPass` acaba en HTTP 500 por la misma causa |
| `go test ./internal/pos -count=1` (15-10, tarde) | **Pasó**: 18 casos. Incluye reintento de la misma venta, 6 peticiones simultáneas de la MISMA venta (1 sola factura), renglón que falla (0 facturas, contador sin avanzar), correlativo concurrente (8 reservas, 0 repeticiones y por encima del máximo existente), sesión POS viva/expirada/revocada, login con y sin verificador, y SyncOffline con una venta buena y una mala. `-count=2` también pasó (sin intermitencias) |
| `go build ./...` / `go vet ./...` (15-10, tarde) | Pasó (exit 0) en el módulo raíz y en `POS/` |
| `go test ./interfaces/web -run 'TestFiscal\|TestKardex\|TestTenant\|TestPOS\|TestLogin\|TestPassword'` (15-10, tarde) | Pasó (49 s) |
| Prueba de factura real y servicio Windows/SCM | No ejecutadas |
| Concurrencia del POS con MySQL real | Probada (ver fila de `internal/pos`); no probada contra el servidor del cliente |

Las verificaciones acotadas comprueban compilación y pruebas existentes de esos paquetes, pero no validan los hallazgos del ciclo POS en una BD MySQL real.

## Orden recomendado

1. Cerrar autenticación/autorización gRPC; devolver error en operaciones aún no implementadas.
2. Hacer sincronización de ventas idempotente y transaccional, con errores no ignorados —en el gRPC **y** en el checkout HTTP.
3. Resolver correlativos bajo concurrencia, con `UNIQUE` e índices, y validar el recorrido fiscal completo.
4. Corregir firma/arranque de modo que fallen de forma segura y añadir pruebas.
5. Probar readiness, recuperación, parada y actualización del servicio en Windows.
6. Estabilizar el directorio de configuración (con permisos reales) y revisar el conjunto exacto de cambios previo a release.

## Veredicto

**Mi opinión: ERPGo tiene una base de producto con trabajo sustancial, pero el nuevo camino POS gRPC no está listo para exponerse como interfaz de producción fiscal mientras H-1 a H-6 sigan pendientes.** No estoy concluyendo que cada módulo de la interfaz web esté fallando; faltaría revisar y probar aparte la cobertura fiscal, migraciones limpias, impresoras, aislamiento entre empresas y operación real.

## Límites

Esta es una revisión estática y focalizada al checkout local del 2026-10-14 (con la corrección de citas y la ampliación de alcance del 2026-10-15). No inspeccioné exhaustivamente todos los handlers, migraciones, plantillas ni reglas fiscales. No se accedió a datos de producción ni se realizó prueba de penetración. Los hallazgos deben validarse con tests reproducibles y revisión de dominio después de corregirlos.
