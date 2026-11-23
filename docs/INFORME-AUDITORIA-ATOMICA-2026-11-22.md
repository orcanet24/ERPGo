# Auditoría atómica integral — ERPGo + POSGo2 (2026-11-22)

Alcance: `C:\Proyectos\GO\ERPGo` (ERP, servidor, IA) y `C:\Proyectos\GO\POSGo2`
(terminal de cajas). Método: seis auditorías paralelas por frente (seguridad,
datos/migraciones, fiscal/contable, IA/MCP, usabilidad/frontend, POSGo2) más
verificación propia ejecutada de los reclamos críticos. Cada afirmación lleva
`archivo:línea`. Al final se separa **lo verificado** de **lo no verificado**.

## 1. Verificación ejecutada en esta auditoría (base de todo lo demás)

| Comando / prueba | Resultado |
|---|---|
| `go build ./...` (ERPGo) | **OK** (exit 0) |
| `go vet ./interfaces/web` y `go vet ./...` (ERPGo) | **OK** (exit 0) |
| `go test ./... -count=1` (ERPGo) | **OK**: 12 paquetes `ok`, 0 FAIL, 0 SKIP; `interfaces/web` 10,1 s, 319 funciones Test en 68 archivos |
| `go run ./internal/webcheck -dir templates` | **OK**: 167 plantillas, 0 errores estructurales, 0 enlaces rotos de 515 |
| Arranque real (`erpgo.exe` contra MySQL 8 aislado, base `erpgo_smoke_audit`) | Arranca, aplica 62 migraciones/742 sentencias, crea admin con clave **aleatoria** impresa en consola (sin credencial de fábrica) y sirve HTTP+gRPC |
| Recorrido autenticado de las 93 pantallas del menú | 76 → 200 OK sin errores de plantilla (media 75 ms, máx 158 ms). Las 17 restantes son 403 **por diseño**: `/user`, `/role`, `/settings/*`, `/ai/*`, `/module`, `/config/menu` son solo-master y la instalación recién creada no tiene master hasta correr `--setup`. Promovido a master, **las 17 respondieron 200**. |
| Cookie de sesión en vivo | `HttpOnly=True, Secure=False` (confirmado en la respuesta real) |
| `go test ./...` (POSGo2) | **FAIL reproducido**: `erpgo-pos/sqlite` 3 pruebas rojas (cifrado PII declarado pero no aplicado; la prueba busca `pos_test.key` y el código crea `<db>.key`) |
| `fiscalprinteradapter/adapter.go` (ERPGo, leído en vivo) | `NewDriver` devuelve **siempre** `&noopPrinter{}`; `CloseInvoice() → ("", nil)`: el modo serial del POS **no imprime nada y reporta éxito** |
| `config.json` de POSGo2 en git | API key en claro versionada (`erpgo-pos-main-key-2026`); **no existe `.gitignore`** en el repo |
| Higiene ERPGo | `.gitignore` no excluye `.freebuff/`: 13 944 archivos rastreados (caché de módulos Go entera, `cookies.txt`, logs). POSGo2 rastrea 5 `.exe` de 17–25 MB, 8 bases `.db` con datos, `.key` DPAPI y un volcado `.e2e/mysql-data` (repo 108 MB; ERPGo 241 MB) |
| Cierre de período (leído en vivo) | `period_close.go:115,176-179` valida `SUM(Debit)-SUM(Credit)` sobre `acc_vaucher`, y `insertVoucherRowTx` (`purchase.go:798-806`) escribe **una sola pata** (los asientos automáticos de venta/compra ponen `Credit=0`): con facturas del día, el cierre nunca cuadra |
| Rutas de escritura sin permiso (leído en vivo) | `POST /product_price/save` (`price_list.go:186-190`) solo comprueba sesión; `routeDepartment` (`permissions.go:42-98`) **no cubre** `/price_list`, `/product_price`, `/order`, `/delivery_order`, `/almacen`, `/consumption`, `/warehouse`, `/spidi`, `/yummy`, `/ocr` |
| Formulario de nota de crédito (leído en vivo) | `creditnote_form.html:11` `<form id="cnForm">` sin `method` ni `action`, botón `type=submit` en `:55`, y no hay handler `submit`/`$.post` en su `pagejs` → el navegador hace **GET y recarga**: el botón «Emitir Nota de Crédito» no emite nada (la ruta real es `POST /creditnote/save`, `creditnote.go:24`) |
| Doble clic en factura (leído en vivo) | `invoice_form.html:421-439`: submit propio con `preventDefault` y `$.post` **sin desactivar botón ni candado**; `guardado.js:305-306` ignora formularios sin `method="POST"`. Cada POST consume número fiscal (`invoice.go:364`). Compras sí está protegida (`purchase.go:288-302`) |
| `MAX(invoice)+1` del sync POS (leído en vivo) | `internal/pos/sync_server.go:149` numera sin bloqueo ni transacción, mientras `grpc_server.go:864-901` ya documenta que «un máximo no reserva nada» y usa `pos_counters` |
| Validação SQL de informes (leído en vivo) | `validateReportSQL` (la estricta con whitelist) existe en `ai_governance.go:288` y **no tiene ninguna llamada** en el árbol; `ai_reports.go:168` usa solo `aiValidateSQLSelect` (verbos, no tablas) |
| Revocación de sesiones (leído en vivo) | `session.go` solo borra por `id` (logout `handlers.go:193`, cambio de empresa `tenants.go:1163`); no existe cierre de sesiones por usuario al cambiar clave/baja |

## 2. Veredicto global

El núcleo del ERP está **mejor construido de lo que su marketing promete y peor de lo que su código presume**: arranca limpio, migraciones idempotentes con checksum, csrf/bcrypt/cripto correctos, y 319 pruebas en verde. Pero hay defectos que, en producción fiscal venezolana, **impiden cerrar un mes, duplican documentos fiscales, dejan las ventas POS fuera del libro mayor y no imprimen el ticket fiscal**. La palabra «MCP» del proyecto **no es protocolo MCP**: es un catálogo interno de herramientas; ningún cliente externo (Claude/Cursor) puede conectarse hoy. Y el frontend, siendo correcto en lo estructural, tiene tres pantallas donde el usuario pierde trabajo o duplica un documento sin enterarse.

Nada de esto invalida el proyecto: todo lo crítico tiene arreglo de raíz y las piezas para hacerlo ya existen en el propio repo (el hash anti-duplicado de compras, `pos_counters`, los drivers fiscales reales de 4 100 LOC sin usar, `guardado.js`).

## 3. Críticos P0 — fiscal y dinero

**F-1. El cierre de período es imposible con asientos automáticos.**
`period_close.go:115,176-179` (bloqueo si `SUM(Debit)-SUM(Credit) > 0.01` sobre `acc_vaucher`) contra `purchase.go:798-806` (cada asiento automático se inserta como fila única con `Debit` y `Credit=0`; la contra queda solo en `RevCodde` y se materializa en `acc_transaction` al aprobar, `accounting.go:147-173`). Verificado leyendo ambas funciones. Impacto: ningún mes con facturas cierra; el respaldo contable de la declaración no existe. Arreglo: una sola definición de «asiento cuadrado» (escribir las dos patas en `acc_vaucher`, o validar contra `acc_transaction`) con prueba que cierre un mes con una factura.

**F-2. Las ventas del POS no generan asiento ni bajan contabilidad.**
`internal/pos/sync_server.go:160-227` (HTTP: inserta invoice, detalles y `stock_movement`; ningún `acc_transaction`/`acc_vaucher`) y `grpc_server.go:764-786` (gRPC: invoice+detalles; ni siquiera stock). Además el cierre pide «factura publicada con asiento» (`period_close.go:278-282`), así que esos meses quedan también bloqueados por otro camino. Impacto: el libro mayor no ve las cajas. Arreglo: un único servicio de registro de venta (cabecera+renglones+stock+asiento+pagos) en una transacción, usado por checkout, sync HTTP y gRPC.

**F-3. Numeración fiscal duplicable y con huecos.**
`sync_server.go:149` (`SELECT IFNULL(MAX(invoice),999)+1` sin bloqueo ni transacción; el candado `sync.Mutex` del `SyncServer` no protege porque se construye por petición, `pos_sync.go:54-59`); `pos_fiscal_blocks.go:118-165,224-229` (reserva de bloque sin transacción: dos cajas pueden recibir el mismo rango; un UPDATE fallido deja el rango activo); y el terminal re-numera al sincronizar (`POSGo2/grpc/sync_worker.go:485-503`): el número del ticket impreso no es el que queda en el ERP. Arreglo: `pos_counters` con UPDATE-first (ya existe y está bien hecho en `fiscal.go:219-358`) en **todos** los caminos + `UNIQUE` en la base + reserva dentro de transacción con `FOR UPDATE`.

**F-4. El IVA no queda como pasivo.**
`invoice.go:668-676` asienta el impuesto `Debit: TaxCode / RevCodde: ProvStateTaxCode`, pero en la siembra actual (verificada en vivo contra base recién migrada) `tax=4021101` es «State Income Tax» (**gasto**) y `prov_state_tax=5020101` «Provision for State Income Tax» (provisión de ese gasto): no hay cuenta «IVA por pagar» real, y la venta se acredita íntegra (base+IVA) a `salesCode` (`invoice.go:534-553`). Impacto: el IVA no existe en el pasivo; el libro de ventas (`legal_books.go:186`, base=total−iva−igtf) no concilia con el mayor. Arreglo: cuenta de pasivo propia por régimen (IVA ventas / IVA repercutido IGTF), validar en arranque `tax ≠ prov_state_tax` y que sea pasivo, asiento base/IVA por patas separadas.

**F-5. «Editar factura» emite una factura nueva.**
`invoice.go:145-202` pinta el formulario de edición, pero `invoiceSave` (`invoice.go:456-467`) no lee ningún id: siempre `INSERT`; `invoice_form.html` no envía id. Combinado con F-6 (doble clic), la vía de corrección de una vez creada es duplicar el documento. Arreglo: desactivar edición post-emisión; corregir solo con nota de crédito.

**F-6. Doble clic = doble factura fiscal.** Verificado en §1 (`invoice_form.html:421-439` sin candado; compras sí lo tiene). Arreglo: aplicar el candado de `guardado.js` y extender a facturas el hash anti-duplicado de `purchase.go:288`.

**F-7. Nota de crédito descuadra CxC y no carga renglones.**
`creditnote.go:348-365`: NC de 100+16 acredita al cliente 116+16=132 (reversión de ingreso **y** de IVA contra el mismo cliente). Y el formulario no emite nada (§1) y su carga AJAX copia un `tbody` vacío (`creditnote_form.html:129` contra servidor que sí prepara `LinesJSON` en `creditnote.go:138-139` que nadie consume). Impacto: el cliente queda en cero o en saldo raro; la pantalla más usada para corregir ventas no funciona.

**F-8. Devoluciones sin tope acumulado y sin NC.**
`return.go:86-97` compara solo contra `ORDER BY id DESC LIMIT 1` de `stock_movement` (no contra lo ya devuelto) y no emite documento fiscal ni asiento. Se puede devolver de más, varias veces, sin rastro contable.

**F-9. Importes de factura tomados del formulario.**
`invoice.go:277-292`: `grand_total_price`, `total_vat_amnt`, `total_tax` vienen del navegador; no hay recálculo por línea en servidor (y `round2` del total por línea se calcula y se descarta, `invoice.go:778-780`). Arreglo: recalcular en servidor y rechazar desviación >0.01.

**F-10. POSGo2 con impresora fiscal noop y sin reimpresión.**
Verificado en §1: serial → `noopPrinter`; el POS registra «✅ Factura fiscal # impresa» con número vacío (`POSGo2/main.go:648-654`) y la venta se guarda. `internal/fiscalprinter/` (Hasar/Epson/Bixolon/TFHKA, ~4 100 LOC, con sus pruebas) **no lo importa nadie** (grep: solo un comentario en `fiscal/printer.go:4`). No existe ruta de reimpresión (`main.go:244-258`); el mensaje al usuario promete «podrá reimprimirse más tarde» (`main.go:650-651`) que no puede cumplirse. Arreglo: conectar el driver real, tratar `fiscalNumber==""` como error, cola de impresión persistente + endpoint de reimpresión.

**F-11. POSGo2 no es offline-first donde más importa: el login exige red.**
`POSGo2/main.go:299` → `grpc/auth_client.go:154` (único camino: remoto). `sqlite/profiles.go:177` tiene `ValidateProfileCredentials` local **sin ningún llamador** (verificado grep). Con el ERP caído la caja no abre turno ni vende; el README promete lo contrario (`README.md:24`). Arreglo: credenciales cacheadas + validación local con sincronización de supervisor al reconectar.

**F-12. Pagos, IGTF y tasa no llegan al ERP.**
El `SaleRequest` del proto no transporta pagos (`grpc_client.go:147-159` arma `Payments` y no lo usa; el camino HTTP sí los manda pero `sync_server.go:160-219` no los persiste). IGTF: cero menciones en todo el backend POS (`pos_api.go`, `pos_sync.go`, `sync_server.go`, `grpc_server.go`) — la percepción del 3% en divisas **no se calcula en ninguna ruta POS**, siendo obligación. El servidor fija `'VES', 1.0, 0.0` ciegamente (`sync_server.go:163-169`). Impacto: arqueo imposible en el central, IGTF omitido, ventas en divisas sin tasa auditable. (Nota de contraste: el checkout web en línea sí calcula IGTF, `invoice.go:556-565`.)

**F-13. Vuelto inflado como cobro y arqueo complaciente.**
`POSGo2/main.go:579-583` guarda lo entregado, no lo aplicado; `sqlite/turnos.go:187-221` suma eso como efectivo esperado → faltantes falsos en cada venta con vuelto. Además `turnos.go:342-343` nunca compara la tarjeta declarada, el modal precarga los totales esperados (`main.go:1624-1626`) y el supervisor no se exige por defecto (`main.go:704-707`).

**F-14. Idempotencia POS sobre `rowid` local.**
`sync_worker.go:467` envía `local-<autoincrement>` y el servidor deduplica por `(terminal_id, sale_local_id)` (`sync_server.go:121,240-260`): tras restaurar una base o reinstalar con `fresh.db`, las ventas nuevas se marcan «already_exists» y **el ERP nunca las factura**, en silencio. Arreglo: ULID/UUID por venta + `db_instance_id` (el propio PLAN.md:414 ya lo pedía).

## 4. Críticos P0 — seguridad y permisos

**S-1. Autorización fail-open por prefijo.** `permissionMiddleware` solo comprueba módulo si el prefijo está en `routeDepartment` (`permissions.go:561-571`, sin `else`); el mapa (§1) no cubre varias rutas de escritura, y los handlers correspondientes no llaman a `exigirPermiso`: `POST /product_price/save` reescribe precios solo con sesión (`price_list.go:186-190`); `/almacen`, `/order`, `/delivery_order`, `/consumption`, `/spidi`, `/yummy`, `/ocr` en el mismo patrón. Un cajero con rol «solo lectura» puede cambiarse los precios. Arreglo de raíz: inventario de rutas generado del mux, **sin clasificar ⇒ 403**, con prueba que falle al registrar rutas nuevas sin clasificar.

**S-2. Dos identidades de usuario sin sincronizar.** Alta/baja/cambio de clave escriben la base de control (`user.go:514-520,583-586`); el login POS autentica contra la copia de la **empresa** (`pos_auth.go:119-156`, verificado: `dbForTenant`). Despedir a un cajero no le cierra la caja. `identity.go:28-36` ya lo reconoce como «la parte que falta». Arreglo: una sola autoridad de identidad (POS autentica contra control) o escritura en ambas en la misma transacción + comprobar `status` por petición.

**S-3. Sesiones irrevocables y nivel congelado.** `session.go` borra solo por id (§1); `render.go:473` usa `sess.UserLevel` cacheado 24 h. Cambiar la clave de un usuario comprometido no lo expulsa; degradarlo sigue siendo «admin» hasta que expire. Arreglo: `session_version`/`DeleteByUserID` + nivel siempre desde BD.

**S-4. SQL libre en informes personalizados.** Verificado en §1: la función estricta está muerta; `ai_reports.go:168` solo prohíbe verbos DML/DDL, no tablas → un rol con «crear informe» guarda `SELECT api_key FROM ai_providers` o `SELECT username,password FROM user_login` y lo exporta (las claves están en claro en la base, `schema_z_ai_providers.sql:18`; los informes nacen públicos si no hay ACL, `ai_reports.go:1274-1276`). Además el scope departamental solo se aplica en `db.query`; `report.run_preview` no (`ai_tools.go:166-175`, `ai_reports.go:330,344`). Arreglo: vistas de reporting con usuario de BD restringido + lista negra real de tablas + una sola función de validación viva en todos los caminos.

**S-5. Cookie sin `Secure` / HTTP por defecto / sin HSTS.** Confirmado en vivo (§1). En Wi-Fi de tienda o exposición sin TLS, robo de sesión 24 h.

**S-6. Respaldos cruzados entre empresas.** `setting.go:309-319` lista **todos** los `.sql` de `data/backup` y `:403-417` descarga cualquiera por nombre (el nombre es fecha al segundo, adivinable; sin rate limit). Un admin de la empresa A descarga el volcado completo de B. El respaldo nuevo además usa la empresa **activa del proceso**, no la de la petición (`database_settings.go:247,255`) → B cree respaldar y respalda A. Arreglo: carpeta por empresa + `tenantOf(r)` + retirar la ruta legada.

**S-7. `/uploads/` no comprueba empresa.** `server.go:340-362`: sesión + bloqueo de `/personal/` es todo; cualquier documento de otra empresa se abre con su enlace (y además usa el `s.db` global para decidir admin, `:350`).

**S-8. Fuerza bruta POS por `X-Forwarded-For` y oráculo de login.** `audit_chain.go:298-302` (`clientIP` confía la cabecera sin proxy de confianza) alimenta el limitador del POS (`pos_auth.go:362-366`): la clave de supervisor se prueba cambiando una cabecera, sin límite. En el web, usuario inexistente responde ~60 ms más rápido que clave errónea (bcrypt solo en el segundo) — enumeración por tiempo.

**S-9. GET que mutan + CSRF exenciones.** `/price_list/activate/{id}` es GET y ejecuta UPDATE (`price_list.go:22,125-126`) — matiz verificado: **sí** tiene `hasPermission` (`:115`), así que el agujero es CSRF (navegación de nivel superior con SameSite=Lax), no autorización. `csrf.go:99-109` exime `/api/` en bloque y `/login` no lleva token (login CSRF). Regla de raíz: ningún GET muta estado.

**S-10. Cadena de auditoría sin clave y no transaccional.** `audit_chain.go:238-243,272-296`: SHA-256 público (quien pueda escribir en la BD recalcula la historia), INSERT con hash vacío + UPDATE posterior (un crash deja la cadena rota), y el middleware audita **antes** del handler (`:365-368`). Ante el SENIAT, una cadena rehacer es una cadena sin valor de prueba. Arreglo: HMAC con clave fuera de la BD, insertar sellado en una sentencia, registrar el diff.

**S-11. Secretos en el repositorio.** POSGo2: API key operativa en tres `config_*.json` versionados (§1), bases `.db` con PII/RIF y `.key` DPAPI en git, `fresh.db`, `.e2e/mysql-data`. ERPGo: `.freebuff/cookies.txt` (csrf de una sesión local) y caché de módulos (13 944 archivos) rastreados porque `.gitignore` no cubre `.freebuff` (§1). Arreglo: `.gitignore` en ambos + `git filter-repo` + **rotar** la API key.

## 5. P1 — datos, migraciones y operación

- **D-1. Sello canónico apaga las semillas para siempre.** `mysql_migrations.go:58-74`: tras el primer sello la cadena nunca vuelve a correr; `canonical.go:318-374` reconcilia DDL (tablas/columnas/índices) pero **no aplica los 346 INSERT ni los triggers** del canónico. Verificado en vivo: el arranque imprimió «base sellada con el DDL canónico vigente; se omite la cadena y se reconcilia». Impacto: en instalaciones existentes no llegan menús/permisos/catálogos nuevos; en las limpias sí. Es la deriva de datos que el plan creía cerrada.
- **D-2. `ADD COLUMN IF NOT EXISTS` no existe en MySQL 8** (sí en MariaDB); `schema_apply.go:81-91` lo recorta, pero un ALTER multi-columna con una columna ya existente aborta entero y `esYaEstabaMySQL` (`mysql_migrations.go:474-497`) lo clasifica como benigno: **columnas nuevas que no se crean y archivo anotado como aplicado** (ejemplo real: `schema_z_zzz_recuperadas_cols.sql:18-20`). En SQLite el traductor (`sqlite_convert.go:341-345`) ni lo recorta. Regla: una sentencia por columna, sin `IF NOT EXISTS`.
- **D-3. Contradicciones README↔código** (el README lo promete; el árbol lo desmiente): «sin DELETE/DROP en migraciones» vs `schema_menu_inventario_v2.sql:50-51`, `schema_z_zzzzz_menu_limpieza.sql:30,43,46` (borran menú/permisos); «los datos corren por única vía de migraciones» vs F-1/D-1; «migraciones corren solas» vs sello canónico.
- **D-4. Borrado de ajuste destruye histórico.** `adjustment.go:441-476`: `DELETE` de `acc_transaction`, `acc_vaucher` y `stock_movement` sin mirar período cerrado (y un `DELETE ... WHERE purchase_id = ?` que recibe «ADJ-3», resto de copia-pega que no borra nada). Contradice la política de oro del propio repo.
- **D-5. `SET FOREIGN_KEY_CHECKS=0` sobre el pool, no la conexión** (`mysql_migrations.go:251-260`, `canonical.go:310-314): los ALTER pueden ejecutarse en otra sesión con las FK activas.
- **D-6. Sobreventa**: validación de stock con `SELECT SUM` sin `FOR UPDATE` (`invoice.go:743-776`, `items_almacen.go:294-307`) y sync POS ni siquiera la comprueba (`sync_server.go:211-217`). Dos cajas, stock 1, dos ventas.
- **D-7. Estado global multiempresa residual**: `dbForTenant` cae a la base **activa** si la pedida no abre (`tenant_request.go:185-201`) — silencio ante empresa equivocada; `s.db` sigue en uploads/poller/rol de informes (`server.go:350`, `cashea_polling.go:77,94`, `ai_reports.go:117`); `setTenant` muta estado global en login (`handlers.go:134`). Fail-closed es el arreglo.
- **D-8. Respaldo sin verificación real ni prueba de restauración**: `backup_scheduler.go:208-212` lee 1 byte y lo llama verificación; no existe restauración; el volcado usa `mysqldump --add-drop-database` (`mysqldump.go:93`), que convierte una restauración mal dirigida en borrado de base.
- **D-9. Rendimiento**: N+1 de stock por producto en reportes (`report.go:158-192`, `legal_books.go:337-362`) con índice no utilizable por faltar `tenant_id` (`esquema_canonico_mysql.sql:3154`); dashboard carga todo el histórico (`dashboard.go:167,187`).
- **D-10. SQLite no es equivalente** (solo motor de pruebas, según el plan) pero `dialect.go:8-13` sigue afirmando lo contrario y las pruebas corren sobre SQLite mientras producción es MySQL puro: los tests no representan el SQL real.

## 6. P1 — usabilidad y frontend

Medido con servidor real y leyendo plantillas/JS:

- **U-1. NC rota** (F-7): el botón «Emitir Nota de Crédito» recarga sin emitir — la corrección de una factura es inusable.
- **U-2. Sesión vencida = «Error» y trabajo perdido.** El 302 al login se sigue como XHR, la respuesta HTML no trae `status`, y el toast dice «Error» (`invoice_form.html:437-438`); no hay `returnTo` ni borrador. Arreglo: 401 JSON para XHR + aviso de sesión vencida sin navegar.
- **U-3. El buscador de menú no busca.** `layout.html:70` con 0 manejadores en todo el repo (verificado). Peor que no tenerlo.
- **U-4. Ctrl+C abre la calculadora** (`layout.html:337`) secuestrando el portapapeles en toda la aplicación; y la calculadora usa `eval()` (`custom.js:310`).
- **U-5. Para acreditar/devolver solo existen las últimas 200/100 facturas** (`creditnote.go:157-162`, `return.go:294`) en un `<select>` sin buscador; en devoluciones se muestra el ID interno, no el número fiscal. Con >200 facturas, la devolución se hace fuera del sistema.
- **U-6. Doble clic en factura** (F-6) y **en producto** (`product_form.html`: tras guardar el id oculto sigue vacío → segundo Guardar duplica).
- **U-7. IVA arranca en 0.00 por línea** (`invoice_form.html:215`) y «Impuesto» vs «IVA» sin explicar (`product_form.html:104-114`); el POS viejo no calcula IVA. Error fiscal silencioso.
- **U-8. Accesibilidad medida**: 424/559 campos visibles sin etiqueta; 404 `<label>` sin `for`; login sin `<label>` ni `autocomplete` (visto en el HTML real servido); zoom bloqueado en 4 pantallas (`maximum-scale=1`); `--erp-warn` con contraste 3,1:1 (falla AA justo en los avisos de RIF e IGTF); 147 botones `btn-xs`.
- **U-9. Peso por página**: 43 archivos/2,62 MB (DataTables 1,19 MB se carga siempre) y **2 `@import` a Google Fonts** en `custom.min.css:1-2` — los únicos escapes reales a internet del ERP; sin red, bloquean la primera pintura y luego caen a Arial (las fuentes no están en `static/fonts`).
- **U-10. Errores crudos**: 613 `jsonError(w, err.Error(), …)`; DataTables sin `errMode`/`language` en 63 de 64 tablas (alert gris en inglés ante un 500); `render.go:353-355` puede devolver el error de Go como página blanca.
- **U-11. Inconsistencias de flujo**: «Ver»/«Editar» pintados idénticos por el tema (`erpgow-tema.css:608-614`); «Due» en inglés; importes sin moneda mientras el dashboard sí la pone; tipos de venta sin «Tarjeta» en la factura web (`invoice.go:306-309` lo soporta); crédito sin fecha de vencimiento; «Balance Anterior» decorativo; POS web del repo (`pos.html`) desconectado de cualquier ruta (`invoice.go:23-25`).
- **U-12. Onboarding**: instalación limpia deja al admin sin acceso a 17 pantallas (solo-master) hasta `--setup` (reproducido en vivo; el aviso del log existe pero el usuario normal no mira el log).

## 7. IA y «MCP» — la verdad del claim

- **M-1. No hay MCP.** No existe `initialize`/`tools/list`/`tools/call`, ni endpoint `/mcp`, ni SDK en `go.mod` (verificado: grep `jsonrpc` sin resultados). `ai_tools.go:3` y `docs/INFORME-IA-2026-11.md:117` lo dicen con honestidad: es un catálogo interno «estilo MCP» y no hay conexión MCP ingresante. Un Claude Desktop no puede conectarse. Para serlo faltan: transporte JSON-RPC, token de máquina (hoy solo cookie+CSRF), tenant ligado al token, y cerrar S-4 antes de exponer nada.
- **M-2. Lo que sí es real y bueno**: identidad propia del agente (`ai_agents.go:394-445`, no impersonación), doble puerta para escribir (guardián + rol, `ai_governance.go:127-189`), idempotencia de herramientas (`ai_idempotency.go:81-107`), catálogo cerrado server-side con JSON Schema.
- **M-3. Agujeros**: aprobación humana **posterior** a la acción crítica, no previa (`ai_governance.go:184-189`) — el agente puede mover dinero antes de que nadie revise; scope eludible en `db.query` (`STRAIGHT_JOIN` no está en el escáner de tablas, `ai_sql_scope.go:44-58`; comentarios ejecutables `/*!...*/` se validan fuera y se ejecutan dentro, `ai_rules.go:221-234`); inyección de prompt desde datos del negocio (`ai.go:414`, `ai_tasks.go:648-650`); presupuesto/timeout de IA inerte (`worker.go:111` no pasa ctx; el TaskTimeout no cancela); ranura de tarea huérfana tras crash (`ai_run_slot.go` + `worker.go:111-118`: la reencolada se traga `errAIRunSlotTaken` como éxito → la tarea del día no corre en silencio); SSRF master-only vía `base_url` con IP literal que `ai_dns_guard.go:271-274` pasa sin validar; informes no reproducibles (no se guarda corrida ni versión de definición, `ai_reports.go:1666-1696`).

## 8. POSGo2 — operación de la caja

- Arquitectura real: binario monolítico (UI+servidor local loopback; el paquete `grpc/` es un cliente HTTP; el cliente gRPC real está muerto), SQLite local, cola de sync con backoff — la cola **está bien hecha** (turnos antes que ventas, transitorio vs permanente, nunca abandona una venta por fallo de red: `sync_worker.go:213-219`, `client.go:253-277`).
- Críticos ya listados (F-10 a F-14).
- Seguridad local: handlers de cobro/turno/cajón **sin ninguna autenticación local** (`main.go:492,709,790,1017`) — cualquier proceso o DNS-rebinding en la caja puede cobrar/abrir el cajón (el bind a localhost ayuda, la validación de `Host` no existe); «cifrado de campos» declarado y no conectado (pruebas rojas reproducidas en §1); `LogPath` configurado e ignorado; fechas UTC vs «hoy» local (ventas de la tarde caen en el día siguiente en reportes, `db.go:96` vs `models.go:491`); descuento no viaja al ERP; el README del repo tiene mojibake real (UTF-8 doble-codificado) — a diferencia de ERPGo, que está limpio (verificado por bytes).
- Higiene: sin `.gitignore`, 5 `.exe`, bases con datos, `INICIAR.bat` arranca la config de pruebas (`config.json` → `pos_test.db` + simulador HTTP) — instalar «como dice el manual» levanta una caja falsa.

## 9. Lo que está bien y hay que defender (no demoler)

Arranque/migración: checksum real con reintento (`database.go:183-300`), orden forzado de `schema.sql`, triggers anti-borrado (`proteccion_borrado.go`), reconciliador estrictamente aditivo con prueba propia. Cripto: bcrypt + rehash al entrar (`password.go:143-172`), `crypto/rand` en todo (0 `math/rand`), AES-GCM correcto, CSRF con comparación en tiempo constante, webhook Yummy con HMAC+ventana+idempotencia. Diseño de sesión/tenant por petición (`tenant_request.go`) con rotación al cambiar empresa. Numeración central con `pos_counters` (UPDATE-first) cuando se usa. Canal de addons fail-closed (loopback+grant+sesión+permiso). Compras: hash anti-duplicado — **el patrón que falta en ventas**. Frontend: `guardado.js` (candado de doble envío y refresco sin recarga — existe y la mayoría lo usa), RIF en vivo con dígito verificador, invalidación de caché por huella con el mejor comentario del proyecto (`render.go:20-42`), tema sin destello, confirmaciones swal en borrados peligrosos, gzip, 0 CDN en JS. Cola de tareas con reclamo atómico y spill. POSGo2: TLS-policy LAN/Internet (`netsec.go`), WAL+busy_timeout, backoff de cola sin pérdida. 319 pruebas Go verdes en ERPGo.

## 10. Lo que NO se verificó (honestidad)

- F-1/F-2/F-3/F-7: deducidos de código con fragmentos verificados por mí, pero **no ejecuté** una venta→cierre completo en vivo (habría exigido sembrar catálogo/comprobante en la base de prueba y emitir una factura real).
- El bypass `STRAIGHT_JOIN`/`/*!` (M-3) y la descarga cruzada de backups (S-6): concluidos leyendo el escáner/las rutas; no se exploraron contra MySQL.
- Impresión contra hardware real (Hasar/Epson/Bixolon): el noop está probado por código, no por impresora.
- Multi-proceso real (dos procesos compartiendo base de control) y concurrencia de dos cajas sobre el mismo stock: no montado.
- No auditie el contenido de `BackUp/*.rar` ni de `.e2e/mysql-data` (225 MB).
- No revisé todas las ~175 pantallas handler por handler: la ruta de permisos abiertos (S-1) se muestreó, no se enumeró exhaustivamente.
- No probé addons reales, OCR, ni las casas de facturación digital externas.
- El estado del repo remoto: `github.com/orcanet24/ERPGo` responde 404 sin sesión (privado, coherente con «código fuente propietario»); no verifiqué que el push llegue.

## 11. Hoja de ruta sugerida (causa raíz, en orden de dolor)

1. **Un solo camino de registro de venta** (web, sync HTTP, gRPC) con asiento+stock+pagos+IGTF en una transacción, numeración `pos_counters` única y prueba concurrente de dos cajas. Mata F-2, F-3, F-12, D-6 de raíz.
2. **Cerrar el círculo del asiento doble**: o `acc_vaucher` guarda las dos patas, o el cierre valida `acc_transaction`; prueba de cerrar un mes con una factura. Mata F-1 y F-4.
3. **Fail-closed de permisos**: inventario de rutas + prueba que explota si se registra una ruta sin clasificar; `exigirPermiso` envoltorio estándar; unificar identidad POS con control. Mata S-1, S-2.
4. **POSGo2 imprimible y offline**: driver real (ya existe, 4 100 LOC), número vacío = error, cola de reimpresión, login local cacheado, ULID por venta, pagos+IGTF+tasa en el payload. Mata F-10…F-14.
5. **Reparar las tres pantallas que destruyen confianza**: NC (POST+carga de renglones), factura (candado doble clic + hash servidor + recálculo de importes), sesión vencida (401 JSON + returnTo). Mata U-1, U-2, U-6, F-5, F-9.
6. **Semillas canónicas**: el reconciliador aplica el bloque de datos del canónico (idempotente) y sella solo sin avisos. Mata D-1, D-2, D-3.
7. **Informe personalizado con frontera real** (vistas de reporting + lista negra viva + scope para todos los caminos) o desactivar SQL libre. Mata S-4, parte de M-3.
8. **Higiene**: `.gitignore`+purga+rotación de la API key del POS; excluir `.freebuff` en ERPGo. Mata S-11, medio de S-5/S-6.
9. MCP: decidir — o renombrar la documentación a «herramientas internas», o construir el puente (transporte, token de máquina, tenant). Hoy es lo segundo lo que el dueño cree que tiene.

---
*Auditoría ejecutada 2026-11-22. Los artefactos temporales de la auditoría fueron eliminados al terminar; no se modificó ningún archivo de código ni datos de clientes.*
---

# ADDENDUM — Reparación ejecutada (madrugada del 2026-11-22 al 23)

El dueño autorizó reparar TODO en su ausencia («usa lo más recomendado / lo más
lógico y funcional»). Este addendum dice, hallazgo por hallazgo, QUÉ se hizo,
dÓNDE está la prueba y QUÉ NO se verificó. Nada aquí afirma lo que no corrió.

## Estado de los 9 frentes del plan

| # | Frente | Estado | Commits (ERPGo salvo nota) | Prueba |
|---|--------|--------|---------------------------|--------|
| 1 | Un camino de venta terminal | **HECHO** | `1b9ff38c`, `17ada74d`, `7cb3fe2a` | venta_terminal_test.go (3) sobre la cadena real; EN VIVO en MySQL: sync → factura 1000/1001/1002 con libro 116=116, IVA 16 y 22.40 en 5020701, IGTF 1.68/4.80 en 5020702, pagos desglosados, stock con el id interno, reintento `already_exists` sin duplicar, control impreso (00-00004242/9999) coincidente con el ticket, fecha del cobro (created_at) y no de la sincronización |
| 2 | Asiento doble y cierre | **HECHO** | `27e93883` | period_close_test.go (4); EN VIVO: /accounting/periods/check 202611 → 8 validaciones limpias, can_close:true, cierre escrito (antes de la reparación este mes era INCERRABLE por F-1/F-2) |
| 3 | Fail-closed de permisos | **HECHO** | `81d38d9f`, `e089bf9f`, `40f0954e` | rutas_clasificadas_test.go + candados_s3_s10_test.go (revocación, XFF, Secure/HSTS, fail-closed de empresa, cadena mixta h2+clásica con alteración detectada) |
| 4 | POSGo2 imprimible y offline | **HECHO, sin hardware** | POSGo2 `90a6178` | 80 funciones de prueba verdes (build/vet/test propios verificados por el integrador). NO VERIFICADO: impresora física (sin máquina aquí), y la extracción del correlativo Bixolon/Hasar es inferida del protocolo documentado — si el firmware lo trae en otro campo, la caja responde «no emitido» (fail-closed, no falso positivo) |
| 5 | Las tres pantallas | **HECHO** | `81d38d9f`, `27e93883` + recálculo servidor en `1b9ff38c` | webcheck 167 plantillas 0 errores; recarga_blanda_test.go; candado doble clic en factura/producto/NC/POSGo2; 401 JSON con returnTo en expirada |
| 6 | Semillas canónicas y cadena | **HECHO** | `27e93883`, `1b9ff38c`, `519ef578` | arranques reales: base nueva 64 archivos 0 avisos → sellada; con 1 archivo nuevo, el ledger repasó y aplicó SOLO ese (D-1 demostrado); ledger ya no es TEXT-ilegal-en-MySQL (Error 1101 corregido) |
| 7 | Frontera de informes IA | **HECHO, con límite declarado** | `e089bf9f` (parte IA: ai_sql_scope/validarSQLInformes), `40f0954e` (motor clásico) | ai_frontera_informes_test.go (6: tablas sensibles, STRAIGHT_JOIN, /*!*/, run_preview, crítica pendiente sin efecto, corrida registrada); EN VIVO por MCP: user_login → blocked. LÍMITE: IT/master no exige catálogo blanco (rompía informes clásicos legítimos); la raíz del todo es vistas de reporting con usuario restringido — tarea de esquema pendiente |
| 8 | Higiene | **HECHA — PURGA EJECUTADA** | ERPGo `21f52172` (13 944 archivos .freebuff fuera del índice); POSGo2 `d726c0f` + purga `2499d36`/`e067a11` (force-push) | filter-repo: **0 caminos sensibles en toda la historia** de POSGo2, .git 108,6→6,9 MB, disco intacto; ERPGo 15 022→1 078 rastreados. Respaldo espejo local `POSGo2-ORIGEN-RESPALDO.git` (destruir tras rotar). QUEDAN DEL DUEÑO: rotar la API key `erpgo-pos-main-key-2026` (urgente —la purga no des-venena lo ya copiado—), borrar el PR #1 de GitHub (su ref conserva los commits viejos) y re-clonar en cada máquina. `POSGo2/docs/PURGA-HISTORIAL.md` |
| 9 | MCP | **HECHO (v1 + pantalla)** | `519ef578`, `aa1aa63b` | mcp_bridge_test.go (5 grupos sobre MySQL real, con skip honesto sin servidor); EN VIVO: initialize/tools/list (10 herramientas, filtradas por operador)/tools/call (3 facturas reales)/401/blocked/405. Pantalla de tokens en IA → Agentes (generar con snippet, revocar, último uso). V1 = transporte streamable simple: sin SSE, sin lotes, sin Resources/Prompts |

## Hallazgos nuevos destapados DURANTE la reparación (no estaban en la auditoría)

Y corregidos con su prueba, porque la política era causa-raíz:

- **`strconv.Atoi(código-producto)` en panel y ajustes**: con catálogo alfanumérico el kardex se escribía en el producto 0 y el tipo (venta/servicio) se leía mal. Resuelto con `idInternoProducto` + coincidencia por código O id (`7cb3fe2a`).
- **El sync del terminal insertaba el CÓDIGO en `stock_movement.product_id` (entero)**: funcionaba solo con códigos numéricos. Id interno ahora (`17ada74d`).
- **Huellas HMAC no cabían en varchar(64)**: con MasterKey (auto-generada), todo registro de auditoría moría en Error 1406 y —con la tx nueva— se deshacía silenciosamente. Ancho a 128 por migración + canónico (`17ada74d`); el INSERT de auditoría ya no es mudo.
- **`getSettings()` devolvía nil** contra veinte llamadores `st.Web.X`.
- **Traductor SQLite mentía en silencio** (causa de que «todo verde en pruebas» no probara la vida real): statements con cabecera de comentario se ejecutaban CRUDOS (pos_sale_sync nunca nacía), no traducía ENUM (stock_movement tampoco), dejaba pasar COLLATE suelto, y desconocía la clave heredada `ALTER…MODIFY col AUTO_INCREMENT`. Warnings de la cadena: 325 → 42, archivos aplicados: 30 → 54 (`1b9ff38c`, `7cb3fe2a`).
- **`INSERT IGNORE` en rutas compartidas** (ensureFiscalSeries, reservarConsecutivoTx): exclusiva de MySQL que tiraba en pruebas; reescrita portable (`1b9ff38c`, `7cb3fe2a`).
- **Ledger de migraciones con TEXT DEFAULT ''** (ilegal en MySQL → 0 archivos aplicados en silencio sobre bases nuevas por esa ruta) (`519ef578`).

## Lo que NO se tocó (decisión documentada, no olvido)

- N+1 de stock en listados y dashboard con escaneo completo (rendimiento; no corrompe datos; requiere diseño de índices por-tenant medido en producción).
- `checkVouchers` sin índice tenant_id, CSRF exenta en /api/, HMAC de bloques de sellado, endpoint estático por documento de upload.
- El puente MCP v1 no implementa Resources/Prompts ni streaming SSE.
- (Resueltos en la ronda del 2026-11-23 mañana: `Touch` de sesión → `TouchSliding` con prueba; `walkingCustomer` GET→POST con tx honesta; pantalla de tokens MCP; panel de aprobación de propuestas —incluido retirar el botón «marcar revisada» cuando hay pendientes, porque ese POST ejecuta—. La historia de POSGo2 ya está purgada —ver §8 y PURGA-HISTORIAL.md: rotación de clave, PR #1 y re-clonados quedan en manos del dueño—.)

## Baterías finales (las mismas que cada commit)

`go build ./...` + `go vet ./...` + `go test ./... -count=1` con y sin `TEST_DB_*` (con ellos: 12/12 paquetes ok; web 89.7 s con MySQL; persistence 73.4 s) + `go run ./internal/webcheck -dir templates` (167 plantillas, 0 errores). POSGo2: build/vet/test propios verdes (80 funciones). Y la prueba de fuego: **instalación desde cero contra MySQL 8.0.30 real** dejando arriba, con auditoría `h2:` encadenada, cierre mensual cuajado y puente MCP respondiendo a una IA externa.

*Addendum escrito 2026-11-23 04:20 (Caracas) por el equipo de reparación. Artefactos de verificación (bases `erpgo_final`, `erpgo_demo`, `erpgo_test_*`, carpeta `_final`) eliminados al cerrar; las bases de prueba MySQL se crearon con prefijo `erpgo_test_` descartable.*
