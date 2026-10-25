# Evaluación exhaustiva de ERPGo — lógica, seguridad y fiscalidad

**Fecha:** 2026-10-15
**Alcance:** `C:\Proyectos\GO\ERPGo` (commit `960253a`, rama `master`, con cambios locales NO confirmados: `cmd/server/*`, `configs/config.go`, `interfaces/web/server.go`, `internal/pos/grpc_server.go`, `internal/service/*`, `instalador/*`, `tools/firmar/*`, `Makefile`).
**Método:** lectura de código línea por línea de los caminos críticos (seguridad, fiscal, contabilidad, inventario, POS, IA) + búsqueda dirigida de patrones de riesgo en todo el árbol + verificación estática (`go build`, `go vet`, `gofmt`) + corrida acotada de pruebas.
**Base de datos:** MySQL (una base por empresa: `erpgo_<código>`), control en `erpgo`. La app se conecta con el usuario configurado en `.env`.

---

## 0. Sobre el criterio de "si la lógica está bien escrita, funciona"

La premisa es razonable como punto de partida —el 85 % de lo que sigue salió de **leer**, no de ejecutar— pero no se sostiene como regla. En esta misma sesión, la ejecución aportó tres cosas que la lectura no habría dado:

1. **Tres pruebas de seguridad están en rojo** (`TestCSRFRejectsForgedPost`, `TestCSRFAcceptsCorrectToken`, `TestCSRFSafeMethodsPass`) por un desajuste de nombres de base que solo aparece al correr.
2. **`go test` sin variables apunta a la base real** (`TEST_DB_NAME` por defecto = `erpgo`): mi corrida aplicó 34 migraciones y creó un usuario `admin` en esa base.
3. Hay clases de defecto **invisibles en una lectura**: el `CREATE TABLE IF NOT EXISTS` dentro de una transacción (MySQL hace COMMIT implícito del DDL y rompe la atomicidad), las carreras de numeración, y la semántica de `ON DUPLICATE KEY`/collation.

Conclusión práctica: **la revisión de lógica es la vía principal y hay que hacerla; la ejecución es el complemento que evita firmar en falso.** Un código coherente y bien comentado (y este lo está, con un nivel de documentación por encima de la media) puede esconder una comprobación que falla abierta, un índice único que no existe o un JOIN que no enlaza.

Nada de lo siguiente se apoya en suposiciones: cada hallazgo cita archivo y línea.

---

## 1. Resumen ejecutivo

El producto tiene una base seria: separación por empresa real por petición (contexto, no variable global en los handlers), guardián de escritura SQL para agentes IA, series fiscales centrales, cadena de auditoría, motor de asientos con partida doble, y un POS con serie fiscal compartida. La arquitectura está pensada y escrita con criterio.

El problema es de **integridad fiscal y de cierre de puertas**, no de falta de funcionalidad:

* La numeración fiscal y la de facturas **no tienen candado ni índice único**: dos cajas simultáneas pueden emitir el mismo número de control.
* Hay **caminos de escritura que evaden el control fiscal** (POS HTTP: factura sin `control_no`) y caminos que **borran documentos fiscales** (`/invoice/delete`).
* El **cierre de período no bloquea** la operación humana; solo frena a la IA.
* La **verificación de primer arranque falla abierta**: si MySQL no responde, `/setup/company` permite crear una empresa y un usuario master sin autenticación.
* Cualquier usuario autenticado puede **ejecutar SQL arbitrario** instalando un addon.
* El **token POS sobrevive al logout** y la app corre contra MySQL **como root sin clave** por defecto.

| Área | Críticos | Altos | Medios | Bajos |
|---|---|---|---|---|
| Seguridad / acceso | 1 | 4 | 5 | 3 |
| Fiscal / contabilidad | 1 | 5 | 6 | 1 |
| Lógica de negocio / datos | – | 2 | 6 | 4 |

**Veredicto:** no apto para producción fiscal (ni SENIAT ni auditoría) en el estado actual, y **no conviene exponer el canal POS** (HTTP ni gRPC) fuera de la LAN hasta cerrar S1–S5 y F1–F6. La base para arreglarlo es buena: casi todo lo crítico es cirugía localizada, no rediseño.

---

## 2. Hallazgos

### SEGURIDAD Y CONTROL DE ACCESO

#### S1 · CRÍTICO — La verificación de primer arranque falla abierta y permite crear un master sin sesión
`needsTenantSetup()` devuelve `true` **también cuando la consulta falla**:

```go
// interfaces/web/tenants.go:546
func (s *Server) needsTenantSetup() bool {
	ctl, err := s.controlStore()
	if err != nil {
		return true          // ← falla abierta
	}
	var n int
	_ = ctl.QueryRow(`SELECT COUNT(*) FROM tenants WHERE status = 1`).Scan(&n)
	return n == 0
}
```

`companySetupAllowed()` (tenants.go:733) se apoya en eso, y `/setup/company` está en la lista de rutas públicas de `authMiddleware` (server.go:518) y de `permissionMiddleware` (permissions.go:252), sin sesión y sin CSRF. Si MySQL está caído, lento o rechaza la conexión (y el arranque ya tolera eso: `NewServer` degrada con un log), **cualquiera en la red puede abrir `/setup/company` y crear una empresa con un usuario master**. Además hay TOCTOU: dos POST simultáneos pasan la comprobación.

**Corrección:** separar "no hay empresas" de "no pude leer" (error → 503, nunca `true`), exigir sesión master salvo confirmación explícita de catálogo vacío, y hacer la comprobación dentro de la transacción de creación.

#### S2 · ALTO — Cualquier usuario autenticado puede ejecutar SQL arbitrario instalando un addon
`/module/addons/` está exento en `authMiddleware`; `permissionMiddleware` no lo declara público, así que **solo se exige sesión, no rol ni módulo**. `addonInstall`/`addonUpdate` leen `addons/<code>/migrations/*.sql` del disco y las ejecutan contra `authDB(r)` partiendo el texto por `;` (addon_handlers.go:226-244 y 368-386):

```go
statements := strings.Split(string(migSQL), ";")
for _, stmt := range statements {
	if _, err := s.authDB(r).Exec(stmt); err != nil { ... }
}
```

Con la conexión de la aplicación (root de MySQL, ver S11), un empleado puede crear usuarios, alterar asientos o instalar lo que quiera. El troceo por `;` además rompe con `;` dentro de literales.
**Corrección:** solo `master`; migraciones por el runner de migraciones (no por `Split`), y las migraciones de addon ejecutadas contra la base de la empresa, no la de control.

#### S3 · ALTO — CORS abierto en `/api/` y exención total de CSRF para `/api/`
`corsMiddleware` responde `Access-Control-Allow-Origin: *` a todo `/api/` (server.go:550) y `enforceCSRF` deja pasar **todas** las peticiones a `/api/` (csrf.go:106). Los tokens POS se aceptan por *query string* (`?token=`, pos_api.go:41 y pos_auth.go:131), lo que se filtra a logs, historial y proxies. Un origen ajeno puede leer respuestas de endpoints que devuelven datos con `*` y provocar peticiones sin token CSRF.
**Corrección:** lista blanca de orígenes, tokens solo por cabecera, y exigir Origin/CSRF en `/api/` cuando la autenticación sea por cookie.

#### S4 · ALTO — El canal gRPC del POS no se autentica a nivel de transporte
Los interceptores prometen autenticación y solo hacen `log.Printf`:

```go
// internal/pos/grpc_server.go:135
func (s *GRPCServer) unaryInterceptor(...) (interface{}, error) {
	log.Printf("[gRPC] %s", info.FullMethod)
	return handler(ctx, req)     // sin credencial
}
```

El caso concreto: `SyncOffline` (stream) **no valida ninguna credencial** y resuelve la empresa con `s.dbOfTerminal(sale.GetTerminalId())` — es decir, con la API key de *otra* conexión que ya esté registrada (grpc_server.go, `SyncOffline`/`dbOfTerminal`). Basta abrir un stream, declarar el `terminal_id` de una caja conectada y empujar ventas. `reflection.Register` está habilitado, así que el contrato entero es descubrible.
**Corrección:** interceptor que valide API key (o mTLS) para todas las RPC, credencial ligada al contexto del stream, y la empresa resuelta por esa credencial, nunca por `terminal_id`; desactivar reflexión en producción.

#### S5 · ALTO — RPC que devuelven éxito sin hacer nada
```go
// internal/pos/grpc_server.go
func (s *GRPCServer) ProcessPayment(...) (*pb.PaymentResponse, error) { return &pb.PaymentResponse{Success: true}, nil }
func (s *GRPCServer) ProcessRefund(...)  (*pb.RefundResponse, error)  { return &pb.RefundResponse{Success: true}, nil }
```
Un terminal puede creer que una **devolución** quedó registrada cuando no existe ni documento, ni stock, ni asiento. En el cliente, `DownloadCustomers` devuelve `(nil, nil)` (POS/grpc/grpc_client.go) y `DownloadProducts` llama a `GetProducts`, que el servidor ahora responde `Unauthenticated` (grpc_server.go) sin que nadie lo haya quitado del cliente.
**Corrección:** devolver `codes.Unimplemented` y limpiar el proto/cliente, o implementarlos. Nunca éxito falso.

#### S6 · MEDIO — Contraseñas heredadas
`password.go` acepta filas en md5 y en **texto plano** y las reescribe a bcrypt en el login. El diseño es defendible (si no, esas instalaciones quedan inaccesibles) y la comparación es en tiempo constante, pero mientras existan filas md5 un volcado de la base es reversible por diccionario.
**Corrección:** contar las filas heredadas y forzar cambio de contraseña en el primer acceso; registrar en auditoría cada rehash.

#### S7 · MEDIO — La cookie de sesión no se marca `Secure` aunque haya TLS
`handlers.go:126-145` fija `HttpOnly` y `SameSite=Lax` pero no `Secure`, mientras `setCSRFCookie` sí lo hace según `r.TLS != nil` (csrf.go:110). Con HTTPS activo (necesario si el POS entra por internet), la cookie de sesión puede viajar por HTTP.
**Corrección:** `Secure: r.TLS != nil` (o HSTS + redirección forzada).

#### S8 · MEDIO — Tokens POS en claro y por URL
`pos_sessions.token` se guarda en texto (migrations/schema_reconstruido_07_pos.sql:30) y se acepta por `?token=` además de por cabecera (pos_api.go:41, pos_auth.go:131). Robar la tabla o mirar un log da una sesión POS válida de 12 h.
**Corrección:** guardar SHA-256 del token y aceptarlo solo por cabecera.

#### S9 · MEDIO — El logout del POS no invalida el token que sí sirve para facturar
`handlePOSLogout` pasa `is_active = 0` (pos_auth.go:196), pero el middleware del POS filtra por `revoked = 0` (pos_api.go:57). Son dos columnas distintas: el token **sigue facturando** hasta su expiración.
**Corrección:** un único estado/`revoked` actualizado en login, logout y revocación de caja.

#### S10 · MEDIO — La autorización de supervisor no queda ligada a la operación
`pos_supervisor_auth` inserta siempre `session_id = 0` (pos_auth.go, `INSERT INTO pos_supervisor_auth`): no se puede saber a qué venta o devolución autorizó, ni la autorización caduca. Es exactamente el "doble control" que audita un POS.
**Corrección:** ligar la fila al token/turno y al documento, con TTL corto (minutos) y consumo único.

#### S11 · MEDIO — MySQL embebido: root sin clave y la app conectando como root
`configs/config.go:64-70` define `DB_USER=root` y `DB_PASSWORD=""` por defecto; `embedded_mysql.go:413-430` inicializa con `--initialize-insecure` (root sin clave) y `asegurarClave` **solo** fija clave si el operador configuró una. La app abre todas las bases de empresa con esas credenciales (tenants.go:237).
**Corrección:** usuario de aplicación con privilegios limitados a `erpgo%`, clave aleatoria generada en la instalación, `.env` con permisos restrictivos y fuera del control de versiones.

#### S12 · BAJO — `JWT_SECRET` sin consumidores y escrito con permisos laxos
`getJWTSecret()` (config.go:130) genera un secreto, lo **añade a `.env` con 0644** (config.go:143) y no lo usa nadie (`JWTSecret` no referencia ningún paquete: verificado por búsqueda). Si `.env` no es escribible, cambia en cada arranque.
**Corrección:** eliminar el JWT sin uso, o usarlo y persistirlo con 0600.

#### S13 · BAJO — Errores internos devueltos al cliente
`jsonError(w, err.Error(), 500)` aparece en decenas de handlers (p. ej. `addon_handlers.go`, `ai_agents.go` con 12 usos, `bank.go` con 18): SQL, nombres de tabla y errores del driver llegan al navegador.
**Corrección:** mensaje genérico + detalle al log con correlación.

#### S14 · BAJO — XSS por diseño en addons
`renderAddonTemplate` inyecta el HTML del addon como `template.HTML` dentro del layout maestro (addon_handlers.go:~470). Es contenido instalado —y por S2 instalable por cualquiera—, así que el modelo de confianza no se sostiene. `addonServeStatic` compone `filepath.Join(dir, path)` con la ruta de la URL: hoy no hay traversal porque el `ServeMux` normaliza `..`, pero la defensa es accidental, no explícita.

---

### FISCAL Y CONTABILIDAD

#### F1 · CRÍTICO — Números de factura y de control fiscal sin unicidad ni bloqueo
La tabla `invoice` (migrations/schema_reconstruido_03_sales.sql) tiene `KEY` sobre `invoice` y `control_no`, **no `UNIQUE`**, y la asignación es lectura-siguiente-escritura sin bloqueo:

```go
// interfaces/web/invoice.go:313
if err := tx.QueryRow(`SELECT IFNULL(MAX(invoice),999)+1 FROM invoice`).Scan(&invoiceNo); err != nil {
```
```go
// interfaces/web/fiscal.go — nextFiscalDoc (SELECT en la 143, UPDATE en la 176)
doc := FiscalDoc{Number: nextNo, ControlNo: fmt.Sprintf("%s-%08d", ctrlPre, ctrlNext)}
_, err = tx.Exec(`UPDATE fiscal_series SET next_no = next_no + 1, control_next = control_next + 1 WHERE id = ?`, id)
```

Sin `SELECT ... FOR UPDATE`, dos transacciones leen el mismo `next_no` y dos documentos pueden salir con **el mismo número de control**. En Venezuela eso invalida el documento y descuadra el libro de ventas. El POS gRPC arrastra el mismo patrón (`MAX(invoice)+1` en `processSale`).
**Corrección:** `UNIQUE(series, control_no)` y `UNIQUE(invoice)`; consumir la serie con una sola sentencia atómica (`UPDATE fiscal_series SET next_no = LAST_INSERT_ID(next_no + 1) WHERE code = ?` y leer `LAST_INSERT_ID()`), y capturar el error de duplicado reintentando.

#### F2 · ALTO — El checkout POS emite facturas sin número de control fiscal y asienta en cuentas fijas
`/api/pos/checkout` inserta la factura directamente (pos_api.go:210), **sin `control_no`, sin `series` y sin pasar por `nextFiscalDoc`**, y genera el asiento con cuentas literales:

```go
VALUES (0, ?, ?, 'Invoice', ?, ?, ?, ?, ?, 0, ?, NOW(), 0, ?, ?, 0)`, ..., "1", ..., "2", ...
```

Es decir: documentos fiscales sin control, y contabilidad que ignora el COA configurado y el motor de partida doble (`acc_vaucher`/`approveVoucher`) que sí usa la venta web. Dos caminos de venta, dos contabilidades.
**Corrección:** un único servicio de emisión (número fiscal + asientos + stock) y prohibir `INSERT INTO invoice` fuera de él.

#### F3 · ALTO — No hay anulación, pero sí borrado destructivo de documentos fiscales
`is_anulled` se **lee** en 12 consultas (kardex, libros, cierres, reorder) y **no se escribe nunca**: verificado por búsqueda en todo el árbol. Y en su lugar existe:

```go
// interfaces/web/invoice.go — invoiceDelete
_, _ = tx.Exec(`DELETE FROM acc_transaction WHERE referenceNo = ?`, id)
_, _ = tx.Exec(`DELETE FROM acc_vaucher WHERE referenceNo = ?`, id)
_, _ = tx.Exec(`DELETE FROM invoice_details WHERE invoice_id = ?`, id)
res, err := tx.Exec(`DELETE FROM invoice WHERE invoice_id = ?`, id)
```

Un documento fiscal con su número de control desaparece junto con su rastro contable, y la serie queda con un hueco. La ruta se registra en invoice.go:20 y el handler está en invoice.go:786. El propio `creditnote.go` documenta "Las facturas NUNCA se borran ni se editan retroactivamente" — la intención y el código se contradicen.
**Corrección:** retirar la ruta (o limitarla a `is_draft = 'yes'`), implementar anulación con motivo, fecha y contra-asiento, y mantener la serie intacta.

#### F4 · ALTO — El cierre de período solo frena a la IA
`PeriodIsClosed` (period_close.go:500) se invoca únicamente desde `ai_tools_mcp.go:341` y `:385`. Ningún handler humano de factura, compra, cobro, banco o asiento lo consulta: se puede contabilizar libremente en un mes cerrado y romper el cierre. El agente IA está más restringido que el cajero.
**Corrección:** una comprobación única en el punto de escritura contable (asiento/orquestador), no en cada handler.

#### F5 · ALTO — La venta POS no es idempotente: los reintentos duplican factura y número fiscal
Ni `/api/pos/checkout` (que recibe `OfflineID` y solo lo guarda en `offline_invoice_no`, pos_api.go) ni `processSale` gRPC (grpc_server.go, que también recibe `OfflineId`) comprueban si esa venta ya se registró. Un reintento del terminal —o una resincronización de la cola offline— crea **otra factura y consume otro número de control fiscal**. La columna para evitarlo existe y no se usa como clave.
**Corrección:** `UNIQUE(terminal_id, offline_invoice_no)` (o tabla de deduplicación con el `sale_id`) y, ante duplicado, devolver el documento ya emitido.

#### F6 · ALTO — `invoice_details.invoice_id` significa dos cosas distintas según quién escriba
En la venta web la cabecera guarda el **número** y los renglones el **id interno**:

```go
// interfaces/web/invoice.go — invoiceSave
INSERT INTO invoice (invoice_id, ..., invoice, ...) VALUES (?, ...)   // invoice_id = invoiceNo
...
invInsertID, _ := res.LastInsertId()
INSERT INTO invoice_details (invoice_details_id, invoice_id, ...) VALUES (?, ?, ...)   // invoice_id = invInsertID
```

POS HTTP y gRPC guardan el número en ambos lados. Los dos contadores divergen (AUTO_INCREMENT por un lado, `MAX(invoice_id)+1` con base 1000 por otro), y los consumidores enlazan por esa columna: `kardex.go:165` (`JOIN invoice i ON i.invoice_id = id.invoice_id`), `invoiceDetails` (`WHERE d.invoice_id = ?`), `posAPIRecipt`, `carteraListData`. Resultado: hay facturas cuyos renglones no enlazan → kardex incompleto, detalle/edit vacío y recibos sin líneas.
**Corrección:** una sola clave canónica (los detalles deben apuntar a `invoice.id`), migración de los datos existentes y un test que lo fije.

#### F7 · MEDIO-ALTO — El libro de ventas y los filtros por fecha comparan texto
`invoice.date` es `varchar(50)` y la fecha entra sin validar (`orDefault(r.FormValue("invoice_date"), today())`), mientras los libros filtran por cadena:

```go
startDate := year + "-" + month + "-01"      // month tal cual llega del formulario
endDate := year + "-" + month + "-31"
... WHERE i.date BETWEEN ? AND ? ...
```

Dos consecuencias. Una fecha escrita en otro formato (`01/02/2026`) queda fuera de todos los libros y períodos. Y un mes sin cero a la izquierda rompe el rango: con `month = "3"` el rango es `'2026-3-01'..'2026-3-31'` y, por comparación lexicográfica, en el índice 5 `'0' < '3'`, de modo que **todos** los registros bien guardados (`'2026-03-15'`, `'2026-04-10'`, `'2026-10-05'`) quedan fuera y el libro sale vacío; solo entrarían fechas guardadas sin cero (`'2026-3-15'`). Si la plantilla envía `"03"` ocurre lo contrario. El resultado depende del formato de dos datos que nadie valida.
**Corrección:** `DATE` real en la base, validación/normalización al guardar, y rangos calculados en Go (`time.AddDate`), con `>=` y `<`.

#### F8 · MEDIO — El asiento del IGTF debita al cliente aunque el cobro ya se debitó a caja
invoiceSave calcula `igtfAmount` sobre lo pagado en divisa y luego asienta (invoice.go:486-491):

```go
vno := nextVoucherNoTx(tx, "JV")
... COAID: pre.CustomerCode, RevCodde: pre.TaxCode, Debit: igtfAmount ...
```

El IGTF percibido se cobra en el mismo acto, pero aquí se carga al **cliente**: la cuenta por cobrar queda con un saldo que nunca se cobra y el total de los débitos no cuadra con el total de la factura. (Con el pago fraccionado el efecto es peor: se duplica parte del importe.)
**Corrección:** decidir el tratamiento (debitar caja/banco por el IGTF percibido y abonar la cuenta de impuesto) y verificar el cuadre con una prueba de partida doble por documento.

#### F9 · MEDIO — Dos políticas de costeo para el mismo hecho
El asiento de costo de venta usa `product_lot.unit_cost` (FEFO) como primera fuente (`costoVentaDeProducto`, invoice.go), mientras el kardex valoriza las salidas al **costo promedio ponderado** (kardex.go:250-285). El libro Art. 177 RISLR y el costo contabilizado no coinciden, y el margen que sale de cada informe es distinto.
**Corrección:** una política por empresa (lot cost o CPP), aplicada en los dos sitios, y documentarla.

#### F10 · MEDIO — El arqueo de caja lo declara el cliente
`posAPIShiftClose` guarda `expected_balance` tal cual llega del terminal y calcula `difference = expected − opening` (pos_api.go), sin calcular en el servidor el esperado (apertura + cobros del turno por método) ni cruzar las ventas del turno. El "cuadre" no verifica nada. Además `warehouse_id` del turno se llena con `terminal_id` y `cashier` es texto con el id.
**Corrección:** calcular el esperado en el servidor desde `invoice_payment_summary`/`debt_collection` del turno y comparar contra lo declarado, guardando ambos.

#### F11 · MEDIO — Eliminar un cobro no reversa el asiento (y el comentario dice que sí)
`carteraDelete` recalcula `paid/due` de la factura y borra la fila de `debt_collection` (cartera.go:~330-395), pero **no toca `acc_vaucher` ni `acc_transaction`**: el libro mayor queda con un cobro que ya no existe. Tampoco hay tope: `newDue` puede quedar por encima del total de la factura.
**Corrección:** contra-asiento de reversión o marcar el cobro como anulado y excluirlo de los informes, con recálculo acotado.

#### F12 · MEDIO — `CREATE TABLE` dentro de la transacción de cobro rompe la atomicidad
```go
// interfaces/web/cartera.go:164
_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS debt_collection (...)`)
```
En MySQL el DDL provoca **COMMIT implícito**: a partir de esa sentencia la transacción termina y las siguientes corren en autocommit. Si el asiento falla, el cobro y la actualización de la factura quedan confirmados igual. Es el patrón que `testdb.go` ya documenta como causa de fallos difíciles de ver.
**Corrección:** mover el DDL a las migraciones.

#### F13 · MEDIO — Notas de crédito/débito sin control sobre el documento origen
`creditNoteSave` (insert en creditnote.go:216) consume la serie `NC`, inserta cabecera y renglones, y asienta; pero **no valida que el total no exceda la factura origen** (se puede acreditar de más), descarta el `item_id` (`_ = idv`) y deja `product_id` siempre NULL, **no mueve stock** aunque el comentario promete "reversión de stock", y no ajusta el saldo de la factura ni la cartera. Lo mismo en `debitNoteSave` (que además abona `SalesCode` incluso cuando el motivo es un gasto bancario).
**Corrección:** validar contra el documento origen con tope acumulado por factura, propagar el efecto a cartera y stock, y elegir la contrapartida según el motivo.

#### F14 · BAJO — Las consultas de período dependen de ISO por casualidad
`periodBounds` y los literales `"-31"` funcionan solo porque el formato esperado es ISO. Con `date` textual (F7) no hay garantía.

---

### LÓGICA DE NEGOCIO, DATOS Y OPERACIÓN

#### L1 · ALTO — Estado global mutable que contradice el propio modelo multi-empresa
`setTenant` cambia `s.db` y `s.activeTenant` **del proceso** (tenants.go:528-543) y `handleLogin` lo invoca en cada inicio de sesión (handlers.go:134). Es la clase de bug que el resto del código dice haber resuelto con el ámbito por petición: las rutas sin ámbito caen a la empresa "activa". Ejemplo real: `posTenantCode` usa la cabecera `X-POS-Tenant` y, si no viene, **la empresa activa** (pos_tenant.go:30-40) — una caja mal configurada escribe en la empresa del último que inició sesión. Lo mismo para cualquier handler que use `activeDB()`.
**Corrección:** no mutar el estado global en el login; derivar siempre de la sesión/cabecera y, si no hay empresa, **rechazar** la petición en vez de adivinar.

#### L2 · ALTO — El guardián de lectura de la IA se evade con una coma
`aiTablasEnSQL` extrae solo el identificador que sigue a `FROM|JOIN|INTO|UPDATE` (ai_sql_scope.go:37):

```go
var aiTablaSQLPat = regexp.MustCompile("(?i)\\b(?:from|join|into|update)\\s+`?([a-zA-Z_][a-zA-Z0-9_$]*)`?")
```

`SELECT * FROM invoice, user_login` solo declara `invoice`: la segunda tabla no va precedida de palabra clave, así que **no se comprueba** y el agente lee cualquier tabla de las 139, incluidas `user_login`/`hr_salary` que el diseño dice negar "ni a IT".
**Corrección:** parsear de verdad (o al menos recorrer la lista `FROM` completa incluyendo comas), o —mejor— exponer vistas por departamento con permisos de BD en lugar de filtrar texto.

#### L3 · MEDIO — El guardián de SQL es heurístico y sin límite de tiempo real
Está construido sobre regex de comentarios/literales y una lista de palabras prohibidas (ai_rules.go), sin parser: es razonable como defensa en profundidad, pero es el único candado (ver L2). Y el límite de tiempo está definido y **nunca usado**: `aiHardMaxSeconds = 20` (ai_rules.go:292) no aparece en ninguna otra parte del árbol. No hay `context.WithTimeout` ni `MAX_EXECUTION_TIME` en el camino de `db_query` (ai_tools.go:609): una consulta pesada bloquea el servidor.
**Corrección:** envolver la ejecución en `context.WithTimeout`, fijar `MAX_EXECUTION_TIME` por sesión y presupuestar el `LIMIT` en el servidor (hoy 500 filas).

#### L4 · MEDIO — La auditoría se escribe antes de ejecutar y sin resultado
`auditMiddleware` registra la acción y sigue:

```go
// interfaces/web/audit_chain.go:310
if r.Method == http.MethodPost || ... {
	s.audit(r, action, moduleFromPath(path), path, "", nil, nil)   // antes del handler
}
next.ServeHTTP(w, r)
```

Se registran como hechas operaciones que el handler luego rechaza (403/400), sin recurso ni valores, lo que degrada la señal de la cadena de auditoría. Los handlers que llaman `appendAudit` sí aportan detalle, pero el ruido queda mezclado.
**Corrección:** registrar después, con estado HTTP, recurso e id, y preferiblemente en la misma transacción (esto conecta con la "puerta única de escritura" del orquestador, que hoy no es la única puerta).

#### L5 · MEDIO — El libro mayor es mutable: se revierte borrando
`reverseVoucher` borra las filas de `acc_transaction` y baja `isApproved` (accounting.go:~175-200); `deleteVoucher` borra el asiento; `invoiceDelete` borra por `referenceNo`. Sin contra-asiento no hay rastro de la corrección y cualquier verificación de integridad posterior (sumas, cuadre, auditoría) es inútil.
**Corrección:** asientos inmutables; toda corrección con contra-asiento referenciado.

#### L6 · MEDIO — La aprobación de asientos no es atómica
La idempotencia de `approveVoucher` es un `SELECT COUNT(*) ... isApproved = 1` sin bloqueo (accounting.go:71-80) seguido de dos `INSERT` y un `UPDATE`. La autoaprobación (tras el commit) y el botón manual pueden concurrir: se duplica la partida doble o se marca aprobado un asiento sin publicar.
**Corrección:** `SELECT ... FOR UPDATE` sobre las filas del asiento o `UPDATE acc_vaucher ... WHERE isApproved = 0` y comprobar `RowsAffected`.

#### L7 · MEDIO — Numeración de asientos por lectura de texto
`nextVoucherNoTx` (purchase.go:542) lee el último `VNo` del tipo, lo parte con `fmt.Sscanf` y suma 1, sin índice único ni bloqueo. Dos documentos simultáneos comparten `VNo`… y como `approveVoucher` aprueba **por VNo**, un documento puede aprobar las líneas de otro, o saltarse la aprobación por el chequeo de idempotencia.
**Corrección:** tabla de correlativos por tipo con `UPDATE ... LAST_INSERT_ID` y `UNIQUE(Vtype, VNo)`.

#### L8 · MEDIO — Errores de escritura contable descartados en la venta
Las llamadas a `insertVoucherRowTx` en `invoiceSave`, `creditNoteSave` y `debitNoteSave` se invocan con `_ =` (invoice.go, creditnote.go, debitnote.go). El propio `purchase.go` documenta el caso real: `CreateBy` no numérico hacía fallar el asiento y "el fallo era invisible porque las veinticinco llamadas descartan el error". La factura se confirma sin contabilidad y sin aviso.
**Corrección:** error fatal con rollback, o encolar reparación con alerta visible.

#### L9 · MEDIO — 77 escrituras con error descartado
Recuento en `interfaces/web` de `_, _ = tx.Exec` / `db.Exec`: **77** ocurrencias. No todas son graves, pero entre ellas hay actualizaciones de importes, estados y existencias: un fallo silencioso deja el documento y sus derivados incoherentes.
**Corrección:** revisarlas por criticidad (las que tocan dinero y existencias primero) y tratar el error.

#### L10 · MEDIO — El stock se calcula restando, no se mantiene
El inventario disponible se deriva de `SUM(product_purchase_details.quantity) − SUM(invoice_details.quantity)` (invoice.go `posBarcodeLookup`/`posProducts`, product.go). Ignora ajustes, devoluciones, lotes/FEFO y conteos cíclicos —que sí existen como documentos—, no bloquea ni valida disponibilidad al vender (se puede facturar en negativo) y no coincide con el saldo del kardex.
**Corrección:** tabla de existencias por producto/almacén/lote actualizada en la misma transacción del documento, con el cálculo por resta solo como verificación.

#### L11 · BAJO — Superficie gRPC contradictoria con el proto
`OpenTurn`/`CloseTurn` validan la API key en el campo `Token` (el token que devuelve `Login` no sirve para nada), `handlePayment`/`handleStatus` solo escriben en el log, `GetProductByCode`/`GetProducts` responden `Unauthenticated` y `DownloadProducts` no tiene llamadores. El contrato del proto y el comportamiento no coinciden.

#### L12 · BAJO — Formato y restos
`gofmt -l` señala ~15 archivos sin formatear (`interfaces/web/{debitnote,fiscal_agent,handlers,module_handlers,multi_currency,pagar,pos_admin,pos_api,pos_fiscal_blocks}.go`, `cmd/server/main.go`, `configs/config.go`, `internal/fiscalprinter/*`); `fiscal.go:15-16` tiene `var _ = fmt.Sprintf` / `var _ = strings.TrimSpace` como relleno de imports. Sin impacto funcional, pero impide cualquier puerta de formato en CI.

#### L13 · BAJO — Herramientas de auditoría propias
`infrastructure/persistence/plantilla.go` ejecuta `tasklist` y `mysqldump` (mysqldump.go:108) con argumentos construidos por la app: conviene revisar que ningún valor provenga de entrada del usuario antes de ampliar esa ruta.

---

## 3. Verificaciones ejecutadas

| Verificación | Comando | Resultado |
|---|---|---|
| Compilación | `go build ./...` | **OK** (exit 0) |
| Análisis estático | `go vet ./interfaces/web ./internal/... ./configs ./infrastructure/persistence` | **OK** (exit 0) |
| Formato | `gofmt -l interfaces cmd configs internal infrastructure tools` | **15 archivos sin formatear** |
| Pruebas (subconjunto) | `go test ./interfaces/web -run 'TestFiscal\|TestCSRF\|TestPeriod\|TestTenant\|TestKardex' -count=1` | **FALLA** |
| Suite completa | `go test ./...` | **No completó** (excede 300 s); no se cuenta como aprobado |

Detalle del fallo de pruebas (causa real, no hipótesis):

```
--- FAIL: TestCSRFRejectsForgedPost (32.77s)
    csrf_test.go:44: la base de la empresa "default" no existe o no es accesible:
      MySQL no responde en 5s: Error 1049 (42000): Unknown database 'erpgo_t1_26100__default'
--- FAIL: TestCSRFAcceptsCorrectToken (11.91s)   (mismo error, otro sufijo)
--- FAIL: TestCSRFSafeMethodsPass (16.74s)
    csrf_test.go:135: un GET legítimo fue bloqueado: HTTP 500
```

El servidor pide `erpgo_t1_26100**_default**`; el arnés de pruebas provisiona el nombre con **un** guion bajo. La causa está en la convención: `PrefijoDePrueba` ya devuelve el prefijo terminado en `_` (testdb.go:248, documentado como "con guion bajo delante para poder concatenarlo sin más") mientras `tenantDBName` añade otro `"_"` (tenants.go:92). Consecuencia: **las tres pruebas que guardan el CSRF y el aislamiento por empresa están en rojo**, así que esa protección está sin verificar en este checkout. Es un defecto de la infraestructura de pruebas, no del producto, pero el efecto es el mismo: no hay red de seguridad donde más falta hace.

**Hallazgo operativo adicional (ALTO, verificado en esta corrida):** `aplicarMySQLAConfig` usa `TEST_DB_NAME` con valor por defecto **`erpgo`** (testconfig_test.go:61). Mi corrida aplicó 34 archivos de migración y creó el usuario `admin` en esa base, imprimiendo además la contraseña temporal en la salida:

```
Migrations (MySQL): 34 archivos aplicados, 0 ya aplicados, 552 sentencias
=== TEMPORARY ADMIN PASSWORD: temp-... ===
No users found. Creating admin user "admin"...
```

Un `go test` en la máquina del desarrollador puede por tanto migrar y sembrar la base **real**, y la contraseña temporal queda en los logs de CI o de la consola. **Corrección:** que el valor por defecto sea inexistente (saltar los tests que necesiten base) o un nombre `erpgo_test_*` inequívoco, y borrar el usuario/contraseña temporal al terminar cada prueba.

---

## 4. Lo que NO cubre esta revisión

* No se ejecutó la suite completa ni se probó contra MySQL con datos reales de producción (no se abrió la base del cliente). Los hallazgos fiscales F1–F6 son de código y esquema; su explotación real depende de la concurrencia y de los datos actuales.
* No se revisó línea por línea todo el árbol (≈67 000 líneas Go + 18 000 de pruebas). Se priorizó por riesgo: seguridad de acceso, fiscal, contabilidad, POS, IA, multi-empresa y arranque. Módulos de menor impacto (`cost_handlers`, `payroll`, `hr_*`, `crm_*`, `report_engine`, impresoras fiscales `internal/fiscalprinter/*`, `instalador/`) quedaron revisados solo por patrones, no en detalle.
* No se auditó la interfaz web (plantillas/JS) más allá de lo necesario para seguir el flujo de datos (por ejemplo, los enlaces que revelan F6); no hay análisis de accesibilidad ni de UI.
* No hay pruebas de carga, de concurrencia real ni de recuperación ante caída.
* No se revisó el instalador ni el servicio de Windows en ejecución (solo el código de `internal/service`).

---

## 5. Plan de remediación recomendado

**Fase 0 — Cerrar lo que no puede esperar (días)**
1. S1 (setup fail-open), S2 (addons por cualquier usuario), S9 (logout POS), F3 (borrar facturas).
2. `TEST_DB_NAME` sin valor por defecto y limpieza del usuario temporal; arreglar la convención del prefijo de pruebas (guion bajo doble) y dejar el subconjunto de pruebas en verde.

**Fase 1 — Integridad fiscal y contable (2-3 semanas)**
3. F1: índices únicos + consumo atómico de series y de números de factura, con prueba de dos cajas concurrentes.
4. F5 y F2: idempotencia por `(terminal_id, offline_invoice_no)` y **un único servicio de emisión** de facturas (número fiscal + asientos + stock), por el que pasen la venta web, el POS HTTP y el gRPC.
5. F6: clave canónica de enlace de renglones + migración de datos + prueba que la fije.
6. F4 (períodos cerrados en el punto de escritura), L5/L6/L7 (asientos inmutables, aprobación atómica, correlativos con bloqueo), L8/L9 (dejar de descartar errores de escritura).
7. F7: `date` como `DATE` con validación en la entrada y rangos calculados en Go.

**Fase 2 — Superficie de acceso y POS (2-3 semanas)**
8. S4/S5 (autenticación gRPC y quitar los éxitos falsos), S3 (CORS/CSRF/token por cabecera), S7/S8 (cookie `Secure`, tokens hasheados), S10 (autorización de supervisor ligada a la operación).
9. S11 (usuario de aplicación con privilegios mínimos y clave generada), S12/S13/S14.

**Fase 3 — Datos y operación (1-2 meses)**
10. L10 (existencias mantenidas, no restando), F9 (una sola política de costeo), F8 (cuadre del IGTF), F10/F11 (arqueo y anulación de cobros en el servidor), F13 (control de notas contra el documento origen).
11. L1 (eliminar el estado global de empresa), L2/L3 (parseo real del SQL de la IA y límite de tiempo), L4 (auditoría después del handler, con resultado).

**Fase 4 — Red de seguridad**
12. Puerta de CI: `go build`, `go vet`, `gofmt -l` sin hallazgos y la suite completa en verde; pruebas de propiedad sobre partida doble (débitos = créditos por documento), unicidad de números fiscales y aislamiento por empresa.

---

## 6. Opinión final

El proyecto está más avanzado de lo que sugiere la lista de hallazgos: hay decisiones de diseño acertadas y documentadas (ámbito por petición, guardián de escritura de la IA, series fiscales centrales, cadena de auditoría, degradación visible en el arranque). Lo que falta es **endurecer los bordes**: puertas que no fallan abiertas, numeración fiscal con candado e índices, un único camino de emisión de documentos, y una red de pruebas que esté verde.

Con las fases 0 y 1 completas, el ERP pasa de "no lo expongas" a "operable en producción local con control fiscal defendible". Las fases 2 y 3 son las que lo vuelven apto para multi-empresa y para auditoría externa.

---

## 7. Estado de los hallazgos críticos (reparaciones aplicadas el 2026-10-15)

Principio que guio cada reparación: **nada se corrige rompiendo lo cerrado**. Un documento
emitido no se edita ni se borra; se deja sin efecto con el documento que existe para eso
(nota de crédito, nota de débito, asiento de reversión). Un mes firmado no se toca; la
corrección entra en el mes abierto y deja rastro. Y la regla vive en el punto de escritura
—no en cada pantalla—, de modo que vale igual para el cajero, el contador y el agente IA.

| Hallazgo | Estado | Dónde quedó y cómo se comprueba |
|---|---|---|
| **F1 · CRÍTICO** — números de factura y de control sin unicidad ni bloqueo | **Reparado** | `nextFiscalDoc` consume la serie con un `UPDATE` que incrementa EN LA BASE (bloqueo de fila hasta el commit) y lee dentro de la misma transacción. `reservarNumeroFacturaTx` usa el contador compartido `pos_counters` con reserva bloqueada y auto-corrección contra el máximo ya emitido. `migrations/schema_z_correlativos.sql` añade `UNIQUE(invoice)` y `UNIQUE(series, control_no)`. |
| **F2 · ALTO** — el checkout POS emitía facturas sin control fiscal, sin serie y sin fecha, y asentaba en cuentas literales | **Reparado (parte fiscal)** | `posAPICheckout` reserva el número en el mismo contador que la venta web y el POS gRPC, pide el número de control a la serie `FACT`, graba la fecha y asienta contra `acc_predefine_account` de la empresa del terminal. Prueba nueva: `TestLaVentaDelPOSEmiteDocumentoFiscalYNoSeDuplica`. **Pendiente:** unificar el asiento en el motor de partida doble (`acc_vaucher` + aprobación); depende de L1 (ámbito de petición del POS). |
| **F3 · ALTO** — borrado destructivo de documentos fiscales | **Reparado** | `invoiceDelete` sólo acepta borradores, con la condición también dentro del `DELETE`; rechaza eliminar si hay asientos publicados y deja la operación en la auditoría. La factura emitida se deja sin efecto con nota de crédito. |
| **F4 · ALTO** — el cierre de período sólo frenaba a la IA | **Reparado** | `exigirPeriodoAbiertoTx` se aplica en el punto único de escritura contable (`insertVoucherRowTx`, `insertVoucherRow`, aprobación de asiento) y antes de consumir la serie en venta, compra, nota de crédito y nota de débito. Misma regla para humanos y para la IA. |
| **F5 · ALTO** — la venta del POS no era idempotente | **Reparado** | `pos_sale_idempotency` con `UNIQUE(terminal_id, offline_id)`, en gRPC y en HTTP; el reintento devuelve el documento ya emitido con su número y su control, sin consumir otro. |
| **F13 · MEDIO-ALTO** — notas de crédito sin control sobre el documento origen | **Reparado** | Tope acumulado por factura, leído dentro de la transacción y con la fila de origen bloqueada (`FOR UPDATE`). Un borrador no se acredita. Pruebas nuevas: `TestLaNotaDeCreditoNoPuedeAcreditarMasQueLaFactura`, `TestUnaFacturaBorradorNoSeAcredita`. |
| **S1 · CRÍTICO** — la verificación de primer arranque fallaba abierta | **Reparado** | `tenantSetupNeeded` separa "no hay empresas" de "no pude leerlo" (el error no deja pasar: 503). Con empresas ya creadas sólo entra un master con sesión, y la creación de la PRIMERA empresa se serializa con un bloqueo con nombre (`GET_LOCK`) y se vuelve a comprobar dentro del bloqueo. |
| **S2 · ALTO** — cualquier usuario con sesión podía ejecutar SQL arbitrario instalando un addon | **Reparado** | Instalar, actualizar y desinstalar addons exige super administrador (master). Las migraciones del addon siguen ejecutándose como DDL/DML arbitrario, así que la puerta de entrada es lo que se cierra. **Pendiente:** pasar esas migraciones por el aplicador del proyecto y contra la base de la empresa. |
| **L2 · ALTO** — el guardián de lectura de la IA se evadía con una coma | **Reparado** | `aiTablasEnSQL` recorre la sentencia pieza a pieza y reconstruye la lista completa de cada `FROM`/`JOIN`/`INTO`/`UPDATE`, comas incluidas, con los paréntesis apilados para las subconsultas. Pruebas nuevas en `ai_sql_scope_test.go`. |
| **L12 · operativo** — 16 pruebas en rojo por un desajuste del arnés | **Reparado** | El arnés no preparaba nunca la base de la empresa «default» que esos tests abren: fallaban con `Unknown database`. Ahora se siembra UNA vez por proceso (con las funciones del producto) y cada test la clona; el nombre lo calcula el propio servidor. No se copió ninguna regla: se pidió. |
| **A1 · CRÍTICO** — los usuarios y sus credenciales vivían en dos sitios, y el login miraba el que no era | **Reparado** | Ver §7.1. La identidad vive en la base de control y un solo escritor (`asegurarCredencialDeEmpresa`, en `interfaces/web/identity.go`) la registra desde los dos caminos por los que nace un administrador: el asistente y el arranque. La base de control lleva el mismo esquema que una empresa (una sola definición, la de `migrations/`) porque el login lee de ahí. Prueba nueva para el camino del arranque, con las dos bases separadas: `TestLaInstalacionSembradaEntraConSuAdministradorPorSuCredencialDeControl`. |

### Lo que NO se reparó, y por qué

Nada de lo que sigue se tocó a medias: cada punto necesita una migración de datos, un
refactor que arrastra varios módulos, o una decisión de negocio que no es de una sesión.

* **F6** — clave canónica de enlace de los renglones (`invoice_details.invoice_id`) y migración de los datos existentes. Los consumidores (kárdex, detalle, recibos) tendrían que moverse en el mismo cambio.
* **F7** — `date` como texto: exige `DATE` real, validación en la entrada y rangos calculados en Go.
* **F8** (IGTF), **F9** (una sola política de costeo), **F10** (arqueo calculado en el servidor), **F11** (contra-asiento al anular un cobro), **F14**.
* **L1** — estado global de empresa: es la base de F2 (asiento unificado) y del ámbito del POS.
* **L5/L6/L7** — asientos inmutables, aprobación atómica y correlativos de asiento con bloqueo.
* **L8/L9** — errores de escritura contable descartados (77 sitios).
* **L10** — existencias mantenidas en lugar de calculadas por resta.
* **S3, S4, S5, S7, S8, S9, S10, S11, S13, S14, L3, L4** — superficie de acceso del POS y de la IA, y auditoría con resultado.
* **L12 (formato)** y el valor por defecto de `TEST_DB_NAME`, que hoy apunta a la base real `erpgo`: merece su propio cambio, porque decide si un `go test` puede tocar datos de un cliente.

---

### 7.1 · Dónde viven los usuarios y las credenciales (cierre del defecto de credenciales)

**El defecto.** `authenticateUser` busca al usuario en la base de control (`authDB`), mientras que el
asistente (`ProvisionTenant`) y el arranque (`PrepareCompanyAs` → `InitializeAdminUser`) sembraban el
administrador en la base de la empresa. La empresa se creaba, se registraba en el catálogo, quedaba con
su menú y su plan de cuentas, y su administrador **no podía entrar**: el login respondía
"Credenciales inválidas" con la clave correcta, que es la forma más confusa de fallar. Y había una
segunda mitad del mismo defecto: la base de control no tenía ni las tablas de identidad
(`users`, `user_login`), así que en una instalación nueva el login leía una tabla inexistente.

**La decisión: la identidad vive en la base de CONTROL, y se escribe en un solo sitio.**
Es la única base que responde las tres preguntas que el login hace antes de saber en qué empresa está
el usuario: si existe la credencial, a qué empresas pertenece (`tenant_users`) y qué rol tiene. Por eso:

* `controlStore` aplica el esquema completo a la base de control —una sola definición, la de
  `migrations/`—, una vez por proceso y solo si el handle es la base de control configurada. Sin sus
  tablas, la respuesta del login no sería "no lo sé" sino "no existe", y no existe es indistinguible
  de una clave mal escrita.
* `asegurarCredencialDeEmpresa` (`interfaces/web/identity.go`) es el ÚNICO código que deja la credencial
  de un administrador en control, y lo llaman los dos caminos por los que nace un administrador: el
  asistente (`ProvisionTenant`) y el arranque (`EnsureDefaultTenant`, que además repara las
  instalaciones anteriores: copia la credencial tal como está guardada —bcrypt o el MD5 heredado—, sin
  pedirle la clave a nadie, y por eso la clave temporal que el arranque imprime una vez también sirve).
* El aviso de clave de fábrica se calcula sobre esa misma credencial (`authDB`) y no sobre la copia de
  la empresa: leído de la copia, el aviso se quedaba encendido para siempre después de cambiar la clave
  en "Mi Perfil" —que sí escribe en control—, y un aviso que nunca se apaga entrena a ignorar avisos.
* La copia de la empresa se sigue creando (`InitializeAdminUser`), porque las pantallas de la empresa
  (Usuarios, roles, POS) leen de ahí y el esquema es un espejo del PHP heredado. Lo que no puede pasar
  —y ya no pasa— es que esa copia sea la que decide quién entra.

**Consecuencias que conviene saber.** Un nombre de usuario es UNA identidad global que puede pertenecer
a varias empresas: es el modelo que ya estaba implementado en el login (autentica, resuelve
pertenencia y, si hay más de una, manda al selector). Por eso crear una empresa con un usuario que ya
existe y **otra** contraseña se rechaza con un mensaje que pide otro nombre —dejarlo pasar sería
prometer una clave que no abre la puerta, y pisar el hash cambiaría la clave de la otra empresa sin que
nadie lo pidiera—; con la misma contraseña se acepta y solo se suma la pertenencia. Cuando el alta no
recibe credenciales (respaldo programático, no camino del asistente), la empresa queda usable con la
identidad existente y se deja el aviso en el log.

**Lo que falta del mismo trabajo, declarado.** Las pantallas de la empresa (Usuarios, roles, POS)
siguen leyendo y escribiendo la copia local de `users`/`user_login`: cambiar ahí una contraseña no
cambia la que abre la sesión. El paso siguiente es llevarlas a `authDB` con el filtro de `tenant_users`
para no mezclar empresas entre sí, y recién entonces quitar la copia. Se deja anotado como lo que es
—la parte que falta—, no como un arreglo.

---

## 8. Verificación ejecutada sobre las reparaciones

| Verificación | Comando | Resultado |
|---|---|---|
| Compilación | `go build ./...` | **OK** |
| Análisis estático | `go vet ./interfaces/web ./infrastructure/persistence ./internal/pos` | **OK** |
| POS / gRPC | `go test ./internal/pos` | **OK** (2.8 s) |
| Persistencia y migraciones | `go test ./infrastructure/persistence` | **OK** (105 s) |
| Web, pruebas de las áreas reparadas | `go test ./interfaces/web -run '…'` | **OK** (ver abajo) |
| Formato de los archivos tocados | `gofmt -l` | **OK** (sin hallazgos en lo tocado) |

Pruebas **nuevas** que cubren las reparaciones (todas en verde):

* `TestLaVentaDelPOSEmiteDocumentoFiscalYNoSeDuplica` — documento fiscal del POS (serie, control, fecha, enlace de renglones), cuentas del plan de cuentas de la empresa y reintento idempotente.
* `TestLaNotaDeCreditoNoPuedeAcreditarMasQueLaFactura` y `TestUnaFacturaBorradorNoSeAcredita` — tope acumulado de la nota contra su factura.
* `ai_sql_scope_test.go` (5 pruebas) — el guardián de lectura de la IA con listas `FROM` separadas por comas, joins, subconsultas, comentarios y literales.
* `TestLaInstalacionSembradaEntraConSuAdministradorPorSuCredencialDeControl` — el otro camino por el que nace un administrador (el arranque), con la base de control y la de la empresa SEPARADAS: la credencial queda en control, la clave sirve, el aviso de fábrica sale de donde sale la credencial y un segundo arranque no la duplica ni la pisa.

Familias que estaban en rojo y quedaron en verde (17 pruebas, comprobadas individualmente):
`TestCSRFRejectsForgedPost`, `TestCSRFAcceptsCorrectToken`, `TestCSRFSafeMethodsPass`,
`TestCatalogDefaultsToOwnCodes`, `TestPrepareCompanyIsIdempotent`,
`TestEnsureDefaultTenantRegistersTheStartingCompany`, `TestEnsureDefaultTenantKeepsExistingRegistration`,
`TestDetectsFactoryPasswordOnFreshInstall`, `TestFactoryPasswordWarningDisappearsAfterChange`,
`TestUnknownUserIsNotFactoryPassword`, `TestCONASEVIsOptInAndComplete`, `TestCONASEVRefusesToMergeOverOwnCodes`,
`TestStartupDoesNotSowCONASEV`, `TestNoLegacyCollisionOnFreshCompany`, `TestInspectReportsLegacyCollisions`,
`TestOrderAccountsAreReportedNotHidden`, `TestSwitchingBackToOwnCodesKeepsAccounts`.

Familias que quedaron en verde en la segunda pasada (comprobadas una a una):
`TestProvisionedCompanyCanBeLoggedInto` y `TestSetupWizardRejectsShortAdminPassword` (el defecto de
§7.1), los cinco del aislamiento entre empresas (`TestDosEmpresasSimultaneasNoSeCruzan`,
`TestCadaSesionLeeSuPropiaEmpresa`, `TestLaSesionNoPuedeInventarseUnaEmpresa`,
`TestLaCapaDeIAResuelveLaEmpresaDeLaSesion`, `TestElPlanificadorEncolaParaTodasLasEmpresas`),
`TestProvisionTenantCreatesIsolatedDatabase`, `TestTwoCompaniesWithTheSameNameGetDistinctFiles`,
`TestCompanyCreationOpenOnlyOnFirstInstall`, `TestCompanyCreatedFromPanelIsImmediatelyUsable`,
`TestTwoCompaniesStayIsolatedAfterCreatingFromPanel`, las diez de sesión y las cinco de CSRF.
Los cinco de aislamiento fallaban por un desajuste del arnés de la MISMA clase que L12: abren la base
de una empresa que el arnés nunca preparaba (`openTenantDB` no crea bases a propósito) y morían con
`Unknown database`. Ahora el arnés clona la misma plantilla sembrada que usa «default», y ninguna
afirmación cambió.

### Fallos que permanecen, con su causa

**Bloqueados por el entorno (no son defectos de código):**
`TestElAgenteDeCXCInformaLaCartera` y `TestLaIACreaUnInformeQueCoincideConLaContabilidad` necesitan un
proveedor LLM con API key configurada; sin ella no llegan a ejecutar la consulta.

**Defecto real descubierto al verificar: reparado.**
`TestProvisionedCompanyCanBeLoggedInto` y `TestSetupWizardRejectsShortAdminPassword` fallaban con
`user not found` (la empresa se creaba y su administrador no podía entrar). Quedó cerrado en §7.1: la
identidad vive en la base de control y un solo escritor la registra desde los dos caminos por los que
nace un administrador. Ambos tests están en verde, y se añadió uno para el camino del arranque.

**Preexistentes y ajenos a estos cambios** (comprobado: fallan igual antes y después de tocar el
arnés de pruebas, y uno de ellos ni siquiera toca la base de datos):
`TestEveryTemplateRendersWithLayout` y `TestCadaPaginaSePintaEnTemaOscuro` (falta la plantilla
`pagejs` en el layout), `TestElMenuSeFiltraPorPermisos`, `TestUnGrupoSinHijosVisiblesDesaparece`,
`TestUnGrupoSinPermisoYConHijosPublicosSeControlaConSuPropioPermiso`,
`TestEscribirDejaDatoYSuRastro`, `TestEntrarReescribeLaFila`, `TestUnaFallaAlReescribirNoImpideEntrar`,
`TestCicloFiscalCompletoConImpresoraVirtual` (el RIF del propio fixture no pasa la validación del
dígito verificador), y `TestCompanyCreationRequiresAdminRole`: el producto pide nivel `master` para
crear una empresa desde el panel (`companySetupAllowed`) y el test pasa una sesión `admin`, así que
recibe 403. Falta decidir cuál de los dos manda —quién puede crear empresas es una decisión de
seguridad, no una línea que se cambie para que un test pase—; queda anotado, sin tocar.

**Limitación declarada:** la suite completa de `interfaces/web` (261 pruebas, ~25-80 minutos según la
máquina) sigue sin llevarse a término de una vez: se corta por agotamiento de memoria del equipo. Lo
verificado son las familias tocadas, una a una, y la corrida `-short` del paquete entero (donde solo
quedan en rojo las dos pruebas de plantillas que ya estaban: falta el `pagejs` en el layout). Los
comandos y sus resultados son los de la tabla de arriba, más: `go test ./interfaces/web -short`
(verde salvo `TestEveryTemplateRendersWithLayout` y `TestCadaPaginaSePintaEnTemaOscuro`).


---

## 9. Segunda pasada — los ocho fallos preexistentes, reparados (2026-10-15, tarde)

Los ocho fallos que la sección 7 dejó anotados como «preexistentes y ajenos a estos cambios»
ya no están: cada uno se reparó en su causa y se verificó con su prueba aislada. Ninguna
comprobación se debilitó: en todos los casos el arreglo fue hacer que el arnés (o el producto)
cumpliera lo que la prueba ya exigía.

| Fallo | Causa real | Reparación |
|---|---|---|
| `TestEveryTemplateRendersWithLayout` | `templates/debitnote_print.html` y `templates/addon_info.html` definían `content` y no `pagejs`, que el layout exige: la página abría con HTTP 500 en producción. | Se añadió `{{define "pagejs"}}{{end}}` a las dos. |
| `TestCadaPaginaSePintaEnTemaOscuro` | El mismo `pagejs` faltante: la ejecución abortaba antes de pintar el `data-theme`. | Se verifica con el mismo arreglo, y la comprobación del atributo sigue en pie. |
| `TestElMenuSeFiltraPorPermisos` | `buildMenuItem` solo aplicaba el permiso del nodo a los ENLACES: el de un grupo se ignoraba, y el grupo aparecía si le quedaba un hijo público. El filtro decía una cosa y el menú mostraba otra. | `menu.go`: el permiso del propio nodo manda también para los grupos. |
| `TestUnGrupoSinHijosVisiblesDesaparece` | Misma causa (el grupo «Bancos» se delataba por sus hijos sin `perm`). | Mismo arreglo. |
| `TestUnGrupoSinPermisoYConHijosPublicosSeControlaConSuPropioPermiso` | Misma causa (grupo «Servicios» con «Factura de Servicio» público). | Mismo arreglo. |
| `TestEscribirDejaDatoYSuRastro` | El arnés usaba `TEST_DB_NAME` con valor por defecto `erpgo`, la base REAL, y el login lee de la base de CONTROL: el usuario que el test creaba y comprobaba vivía donde nadie lo buscaba. | Base de control propia por prueba (`baseDeControlDePrueba`), que se borra al terminar. |
| `TestEntrarReescribeLaFila` | Misma causa; además el `user_id` fijo a 1 chocaba con el administrador de la instalación sembrada. | Una base de control por prueba y el identificador calculado en vez de fijado. |
| `TestUnaFallaAlReescribirNoImpideEntrar` | Misma causa. | Mismo arreglo. |

Además, dos que la sección 7 declaraba sin decidir:

* `TestCompanyCreationRequiresAdminRole` — **reparado en el test, no en el producto**. El test
  pedía nivel `admin` para crear una empresa desde el panel; `companySetupAllowed` exige `master`
  desde que se cerró S2, y esa es la decisión de seguridad (un `admin` de empresa que pudiera
  crear empresas se fabricaría una a su medida y entraría por ella). El test ahora fija la regla
  en los dos sentidos: `user` y `admin` reciben 403, `master` crea la empresa.
* `TestCicloFiscalCompletoConImpresoraVirtual` — **avanzó cinco defectos reales y sigue en rojo**
  (ver §9.2).

### 9.1 · Defectos de PRODUCTO que aparecieron al reparar los tests

Ninguno de estos era un problema del arnés; los cinco estaban en el código de producción.

1. **La caja no se podía abrir.** `cashSessionOpen` insertaba seis columnas con cinco valores
   (`status` faltaba en la lista de argumentos), así que abrir un turno respondía HTTP 500 con
   «sql: expected 6 arguments, got 5». Sin `status = 1` el turno nacía sin estado y
   `currentOpenSession` no lo encontraba. **Reparado** en `interfaces/web/cashsession.go`.
2. **La normalización de collation se saltaba las columnas.** `normalizarCollationMySQL` filtraba
   por `table_collation`, que es solo el valor por DEFECTO de la tabla: una tabla cuyo defecto ya
   era `utf8mb4_unicode_ci` pero que tenía columnas con `CHARACTER SET ... COLLATE ...` propio
   (el `CREATE TABLE` heredado del PHP las declara: `schema.sql:199` deja
   `acc_vaucher.referenceNo` en `utf8mb4_general_ci`, y hay columnas en `utf8mb3` y en `latin1`)
   nunca se convertía. Medido en una base recién migrada: cientos de columnas sin normalizar.
   **Reparado**: el filtro mira también las columnas, y la marca sube a `v2` para que las
   instalaciones que ya ejecutaron la `v1` se revisen.
   *Nota de operación:* la primera arrancada tras este cambio reconstruye esas tablas
   (`ALTER TABLE ... CONVERT TO`), así que en una base grande el arranque tardará más una vez.
3. **`SET NAMES utf8mb4` de las migraciones se llevaba por delante la collation de la conexión.**
   Los diez archivos de `migrations/` abren con `SET NAMES utf8mb4` (sin `COLLATE`), y las
   migraciones corren sobre el MISMO pool que las consultas: las conexiones que aplicaron un
   archivo quedaban con `collation_connection = utf8mb4_0900_ai_ci` (el defecto de MySQL 8) en vez
   de la que declara el DSN, y MySQL se niega a comparar una columna en `utf8mb4_unicode_ci` con
   una expresión en `0900_ai_ci`. **Reparado** neutralizando esas sentencias en el aplicador
   (`sinCambiarLaCollationDeLaConexion`); el DSN ya pide `charset=utf8mb4&collation=utf8mb4_unicode_ci`
   en cada conexión, que es la misma intención con la collation correcta.
4. **El formulario simulado de la factura no era el del panel** (dos veces: primero el RIF del
   fixture no pasaba el dígito verificador, después faltaban `total_vat_amnt` y
   `grand_total_price`, que el panel calcula con JS —`invoice_form.html:196-209`— y el producto lee
   tal cual). La factura nacía con IVA 0 y el test reprochaba al producto un IVA que él mismo no
   había mandado.
5. **El período se pedía en un formato que el producto no usa**: el test cerraba «2026-10» y
   `normalizePeriod` exige `AAAAMM`. Ahora lo pide al producto (`currentPeriod`).

### 9.2 · Lo que queda: el modelo contable (decisión, no línea)

`TestCicloFiscalCompletoConImpresoraVirtual` llega hoy hasta el cierre del período y **no puede
cerrarlo**, y el motivo es un defecto de producto ya anotado como L5/L6/L7/L8:

```
Suma de débitos = suma de créditos: diferencia de -1886.48 en el período
Cada voucher cuadra por sí solo: vouchers descuadrados: JV-2 (-1962.00), JV-3 (+35.84), JV-5 (+3.84), JV-6 (+35.84)
```

Medido con el escenario completo (no deducido): cada documento escribe **una fila por asiento** en
`acc_vaucher`, cada una con su propio `VNo` —`nextVoucherNoTx` se llama por fila en `invoice.go` y
`purchase.go`—, y esa fila es autocontenida: el asiento completo es el par
(`COAID`, `RevCodde`) que `approveVoucher` publica como débito y crédito en `acc_transaction`.
Pero las validaciones del cierre (`PeriodSummary`, `checkVouchersBalance`) leen las columnas
`Debit`/`Credit` de `acc_vaucher` y agrupan por `VNo`, que es el modelo de «asiento = varias
líneas» (el que siembran las pruebas de `period_close_test.go`). Los dos modelos conviven y no
describen lo mismo: un mes facturado nunca cuadra con el segundo.

No se ha tocado ni la comprobación ni el test: la decisión —qué modelo manda— es de arquitectura
(el «único servicio de emisión» de la fase 1), y las dos salidas son legítimas: (a) que el cierre
valide sobre `acc_transaction`, que es donde la partida doble existe de verdad, o (b) que los
documentos escriban un asiento con sus líneas de débito y crédito. Conviene decidirla antes de
tocar los 25 puntos de escritura contable.

### 9.3 · La suite completa, hasta el final

`go test ./interfaces/web/... -count=1 -v -timeout 4h` corrió **entera por primera vez**:
3071,7 s (51 min), 385 pruebas ejecutadas, **362 en verde, 2 saltadas y 2 en rojo** (una de ellas
reparada después; ver abajo). Las dos saltadas son de entorno, no de código:
`TestConsumoYaContabilizadoNoSeRepite` (necesita un proveedor de IA con clave) y
`TestPDFSeGuardaEnDisco` (necesita el binario de PDF).

| Prueba en rojo | Estado |
|---|---|
| `TestViewsDoNotCrossCompanies` | **Reparada.** El arnés abría la base de la empresa «caracas» sin crearla (`openTenantDB` no crea bases). Misma clase de desajuste que el aislamiento entre empresas: ahora el arnés clona la plantilla sembrada (`empresaDePruebaDejaLista`). Verificada aislada: en verde. |
| `TestCicloFiscalCompletoConImpresoraVirtual` | **Sigue en rojo, con causa declarada.** Ver §9.2: es la decisión del modelo contable. Todo lo demás del ciclo —factura con serie y control, caja abierta y cerrada, período con su formato— ya funciona; el error 1267 de collation que la hacía morir antes **desapareció** con los arreglos de §9.1. |

Lo que el arnés gana, medido: la primera corrida completa se cortó por el tiempo límite de `go test`
(10 min) tras 8 pruebas; la de ahora termina en 51 minutos. La diferencia no es de esta sesión
—el clonado de la plantilla ya existía— pero sí queda registrado que la suite **sí termina** en esta
máquina, que era una duda abierta desde la revisión anterior.

### 9.4 · Segunda corrida completa y el servidor que se cayó (honestidad del registro)

La corrida de confirmación se lanzó sobre los mismos archivos y llegó hasta la prueba 388 (de 414)
sin más rojo que la conocida… y ahí **el servidor MySQL se cayó**: el log se llena de
`wsasend: An existing connection was forcibly closed by the remote host` y todo lo que venía detrás
falla o se salta con «MySQL no disponible en 127.0.0.1:3306» (18 saltadas, entre ellas
`TestDosEmpresasSimultaneasNoSeCruzan`, que en la corrida anterior estaba verde).

O sea: esa segunda corrida **no vale como verificación** y así queda anotada. El servidor (MySQL
8.0.30 con `datadir = C:/laragon/data/mysql-8`, que acumula **57 bases `erpgo*`** de las corridas)
se recuperó y quedó escuchando; y la cola que la caída invalidó —los diez tests de aislamiento entre
empresas y las seis de vistas y plantillas— se volvió a ejecutar después: **16 en verde, 0 fallos,
0 saltos**.

Con eso el estado es: la suite completa termina (51 min), las **ocho** pruebas del encargo están en
verde, `TestViewsDoNotCrossCompanies` quedó reparada y solo `TestCicloFiscalCompletoConImpresoraVirtual`
sigue en rojo, esperando la decisión de §9.2.

Dos cosas operativas que conviene saber antes de repetir la suite:
1. **Conviene limpiar las bases de prueba antes de lanzarla.** Las 57 bases `erpgo*` que quedaron en
   disco son restos de corridas cortadas; con el servidor caído a mitad, los `DROP` de limpieza no
   llegan a ejecutarse. `tools/limpiar` existe para esto.
2. **La suite completa es un evento de 51 minutos y mueve mucho el servidor local.** Correrla con
   el ERP en uso, o dos veces seguidas, es pedirle al MySQL de desarrollo más de lo que su memoria da.

---

## 10. Continuación POS — hardening HTTP (2026-10-19)

Esta pasada se limitó a `interfaces/web/pos_api.go`, `pos_auth.go` y las pruebas POS; no se declara
cerrado el resto de los hallazgos de la revisión. El checkout POS HTTP ahora:

* rechaza tokens en la query string (solo acepta `X-POS-Token`), mantiene expiración, estado activo,
  revocación y usuario activo en el middleware;
* requiere identificador para cada renglón, cantidad positiva finita, precios/tasas válidos y
  métodos/montos de pago admitidos; evita pagos vacíos o insuficientes, permite vuelto solo hasta el
  efectivo recibido y registra el efectivo neto retenido;
* conserva emisión fiscal, clave idempotente por terminal y renglón enlazado con el `invoice.id`
  interno. La serie y el asiento usan el plan contable configurado; la apertura/cierre de turno
  derivan cajero/caja del contexto de sesión y registran eventos en la misma transacción.

Pruebas de regresión añadidas: `TestPOSMiddlewareNoAceptaTokenEnQueryString` y
`TestPOSCheckoutRechazaValidacionesDePagoYProducto`; se volvió a ejecutar también el conjunto POS,
incluyendo logout, documento fiscal e idempotencia.

**Verificación de esta continuación:** `gofmt -w interfaces/web/pos_api.go interfaces/web/pos_auth.go interfaces/web/pos_api_fiscal_test.go`,
`go test ./interfaces/web -run 'TestPOS|TestLaVentaDelPOSEmiteDocumentoFiscalYNoSeDuplica' -count=1`,
`go test ./internal/pos -run 'TestLogin|Test.*Credential|Test.*Session' -count=1`, `go build ./cmd/server`
y `git diff --check` de los archivos listados terminaron en verde. No se ejecutó aquí `go build ./...`,
`go vet` ni la suite completa. El gRPC, autorización de supervisor,
registro seguro de pago/contra-asiento, unicidad fiscal a nivel de base, CORS/CSRF general y los demás
hallazgos siguen pendientes de verificarse/cerrarse; esta sección no atribuye un arreglo a esas áreas.

**Nota operativa de datos:** al ejecutar los tests de carnicería, `empresaCarnica` provisiona una base
MySQL de prueba y el historial existente documenta que las suites de integración dejan bases `erpgo*`
si se interrumpen antes del cleanup. No se han eliminado bases ni ejecutado herramientas de limpieza.
Cola POS con `offline_id` vacío sigue sin clave de deduplicación; el terminal debe enviar ID para que
la idempotencia aplique.

