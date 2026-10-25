# Hoja de ruta única — ERPGo

> Este documento reemplaza a los planes separados de cada sesión. Hay un solo
> camino y un solo criterio de orden: **retorno fiscal y correctitud legal
> primero, funciones de negocio después**.

**Estado:** 4 de octubre de 2026 · Todo el trabajo está **sin commitear** en el
working copy.

> ## ⚠️ Cambio de motor: MySQL (04-10)
>
> La tabla de abajo dice "SQLite 100%". **Está desactualizada.** El motor es
> **MySQL**, con una base por empresa y el servidor incluido arrancado por el
> propio programa. La decisión, sus mediciones y los tres fallos que destapó
> están en **[DECISION-MOTOR-DE-DATOS.md](DECISION-MOTOR-DE-DATOS.md)**, que
> **sustituye** a la sección "Motor de datos: SE QUEDA SQLite" del punto 5-cuad-bis.
>
> SQLite sigue en el repositorio como el proyecto anterior y se puede volver a
> usar con `DB_DRIVER=sqlite`; no es código muerto, es el historial del producto.

---

## 1. Por qué esta hoja existe

Había dos sesiones trabajando sobre el mismo sistema y en algunos casos sobre
los mismos archivos. El riesgo real de eso no es el conflicto de texto: es
construir dos veces el mismo módulo, o que una sesión pise un archivo que la
otra está editando.

Este documento fija **qué se construye, en qué orden, y qué se descarta**, para
que cualquier sesión que lo lea sepa qué es suyo y qué es de la otra.

---

## 2. Estado real verificado

### Entregado y probado

| Entrega | Dónde | Estado |
|---|---|---|
| ~~SQLite 100% (fuera MySQL)~~ → **MySQL** | `persistence/mysql.go`, `embedded_mysql.go` | **Cambiado 04-10** |
| Multi-empresa (una BD por empresa) | `interfaces/web/tenants.go` | Completo |
| **Sesiones persistentes** | `interfaces/web/session.go` | Completo (brecha cerrada 02-10) |
| Cola con límite de RAM | `internal/queue/` | Completo, 8 tests |
| Gobierno de IA Nivel 1 | `ai_governance.go` | Completo, 21 tests |
| **MCP completo + base normativa** | `ai_legal.go`, `ai_tools_mcp.go` | Completo (F6 cerrada) |
| RRHH: esquema + motor | `migrations/schema_z_hr.sql`, `payroll.go` | Completo |
| RRHH: servicio + HTTP + 6 pantallas | `hr_service.go`, `hr_handlers.go` | Completo, 15 tests |
| CRM completo (nada existía) | `crm_service.go`, `crm_handlers.go`, 5 plantillas | Completo, 13 tests |
| **Cierre de períodos** | `period_close.go`, `acc_period_close` | Completo, 11 tests |
| **Motor de informes por plantilla** | `report_engine.go` | Completo, 14 tests |
| Plan de cuentas CONASEV | `conasev.go` + `data/conasev_coa.csv` | Sembrado; falta cerrar predefine |
| **Arranque de primera vez usable** | `interfaces/web/company_bootstrap.go` | Completo (02-10) |
| Comprobador de plantillas | `internal/webcheck` | 114 plantillas, 0 error estructural |
| **Toma de inventario cíclico** | `interfaces/web/cycle_count.go` | Completo (04-10) |
| **FEFO con alertas de caducidad** | `interfaces/web/inventory_fefo.go` | Completo (04-10) |
| **Tipo de cambio BCV/paralelo** | `interfaces/web/exchange_rate_daily.go` | Completo (04-10) |
| **Punto de pedido / reposición** | `interfaces/web/reorder_point.go` | Completo (04-10) |
| **Prueba de datos: la empresa cárnica** | `datos_carniceria_e2e_test.go` | **PASS** (04-10) |
| **Mapa de BD para la IA, leído de MySQL** | `ai_schema_introspect.go` | Completo (04-10) |
| **Scopado por departamento + candado en SQL** | `ai_sql_scope.go` | Completo (04-10) |

## 2-bis. Los cuatro módulos y la prueba de datos (04-10)

**Los cuatro módulos pedidos están construidos, compilados y probados.** Informe
completo en **[INFORME-ETAPA-CARNICERIA.md](INFORME-ETAPA-CARNICERIA.md)**.

| Módulo | Código | Plantilla |
|---|---|---|
| Toma de inventario cíclico | `cycle_count.go` (abrir, contar, cerrar, ajustar) | `inventory_cycle_count.html` |
| FEFO con alertas de caducidad | `inventory_fefo.go` | `inventory_expiry.html` |
| Tipo de cambio BCV/paralelo | `exchange_rate_daily.go` | `exchange_rate_paralelo.html`, `tasa_diferencia.html` |
| Punto de pedido / reposición | `reorder_point.go` | `inventory_reorder.html` |

Las cuatro rutas están cableadas en `NewServer` (`server.go`).

**La prueba de datos** (`TestEmpresaCarnicaVive`) simula un mes entero de una
carnicería —personal, compras con lote, proveedores, 11 facturas, nota de
crédito, cobros, tasa de cambio diaria, punto de pedido, conteo cíclico— y
exige tres invariantes al final: que la contabilidad cuadre, que el stock diga
lo mismo por sus dos caminos de cálculo, y que en cada factura
`total = pagado + por cobrar`. **Pasa en 11,6 s.**

Cinco bugs reales salieron de ahí. Los dos graves no estaban en los módulos
nuevos sino en el núcleo que ya existía:

1. `acc_vaucher.CreateBy` y `approvedBy` son **INT** y el ERP les pasa un nombre
   → toda compra, factura o conteo se guardaba **sin su asiento**, sin mensaje y
   sin rastro. Invisible porque las 25 llamadas a `insertVoucherRowTx`
   descartan el error con `_ =`.
2. **Aprobar un asiento nunca funcionó con MySQL**: con `parseTime=true` las
   columnas DATE vuelven como `time.Time` y al escribirlas daba `Error 1292`.
   El error se comía la transacción entera, o sea que el posting se deshacía.

Los otros tres: `productSave` contestaba "actualizado" sin actualizar,
`sessionFromRequest(nil)` petaba con un panic, y el `UPDATE` de cobro de la
propia prueba —que pagaba el importe completo a cada factura con saldo, sin dar
ningún error— destapó que el ERP no sabe cerrar una cartera.

**Y uno más, encontrado al re-correr la suite completa y que NO es de esta
etapa:** `queue_tasks.last_error` era `TEXT NOT NULL` sin valor por defecto.
SQLite lo acepta; MySQL en modo estricto no, y el `INSERT` de la cola moría con
`Error 1364`. Es decir: **la cola no aceptaba ninguna tarea**. El planificador
de tareas de IA y el encolador de escrituras estaban mudos. Arreglado en
`internal/queue/queue.go`.

Lo que hay detrás es una clase de fallo, no un caso suelto: **el esquema
heredado tiene 479 columnas `NOT NULL` sin `DEFAULT`**, y el modo estricto de
MySQL convierte cada `INSERT` que omita una en un Error 1364. SQLite los
aceptaba en silencio. Cada uno hay que encontrarlo ejecutando, no leyendo: por
eso la red de tests tiene que correr entera y en verde antes de dar el trabajo
por terminado.

**Lo que queda:** el ERP no tiene un módulo de cobros que liquide la factura, y
el consumo de lote en la venta lo hace la prueba y no `invoiceSave`. Está
anotado en el informe, no escondido.

### Decisión: el plan de cuentas es de la empresa (02-10)

La pregunta que se planteó era qué hacer con los 9 códigos que el catálogo
heredado del PHP y el del SENIAT definían distinto (101, 201, 301, 401, 501...).
La respuesta no es elegir un ganador: es **dejar de mezclar catálogos**.

El plan contable pertenece a la empresa. ERPGo trae un catálogo heredado en el
esquema base (433 cuentas) y el seeder del SENIAT agregaba el suyo encima (1855).
Como  es un entero sin unicidad, los códigos cortos compartidos
quedaron definidos por el que se sembró primero y el otro nombre desapareció
sin aviso.

- **Ninguno** (por defecto): la empresa codifica como quiera. No se siembra nada
  encima de lo suyo.
- **CONASEV**: el catálogo del SENIAT se ofrece como punto de partida. Solo se
  aplica sobre un plan vacío o que ya sea CONASEV; **nunca se fusiona** sobre un
  plan propio, y el intento se rechaza con 409 explicando por qué.

Decidir por catálogo se separó de aplicarlo: un clic elige, otro carga.

Código: `conasev.go`, `company_bootstrap.go`. 9 tests.

**Hallazgo adicional:** el SENIAT numera las cuentas de orden con cero a la
izquierda (01, 06, 0124) y `HeadCode` es numérico, así que no se pueden cargar.
Peor: `01` se vuelve `1`, que en cualquier plan es Activos, y `02` es Gastos — la
cuenta de orden 01 **no es** el grupo Activos, pero la columna no las distingue.
Se reportan en el diagnóstico en lugar de omitirse en silencio. Arreglarlo exige
cambiar el tipo de la columna, que es una migración de verdad.

### Herencia del PHP: qué queda y qué se limpió (02-10)

El esquema viene del PHP original: 236 sentencias `ALTER TABLE` en dialecto MySQL
que el conversor traduce a SQLite. Ese residuo es real y está, pero la mayor parte
funciona. Lo que estaba roto:

1. **El clasificador de migraciones tapaba fallos reales.** Cualquier error
   (`near`, `syntax error`, `no such column`) se contaba como *skipped*, que en el
   resumen se leía como *ya estaba*. Una columna que nunca se agrega es una
   columna que un handler encuentra faltante en producción, y el log no decía
   nada. Ahora se decide por la SENTENCIA (`ENGINE=`, `COLLATE`, `UNSIGNED`,
   `AUTO_INCREMENT`) y no por el mensaje genérico.
2. **Resultado real: 0 warnings, no 29.** Los 29 eran duplicados tolerados
   correctamente; ninguno era un fallo. El log ahora dice la verdad.
3. **Verificado que no falta ninguna columna**: `schema_integrity_test.go`
   comprueba 28 columnas que los handlers leen de verdad. Hoy: 0 ausentes.
4. **217 líneas de código muerto MySQL eliminadas** (`migrate.go`:
   `MigrateSQLiteToMySQL`, `copyTable`, `WriteDotEnv`, sin ninguna llamada) y el
   driver `go-sql-driver/mysql` retirado de `go.mod`.
5. **`.env` limpio**: las variables `DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`
   (con la clave de MySQL escrita aí) no se leían desde que SQLite es el único
   backend. Eran inofensivas pero mentías.
6. **6 caracteres CJK colados** en comentarios y documentación, de más de una
   sesión, corregidos.

**Suite completa: 221 tests en verde en `interfaces/web` más los de
`infrastructure/persistence` e `internal/queue`. `go build`, `go vet` y
`go run ./internal/webcheck -dir templates` limpios.**

### Arranque: qué se ejecuta de verdad en cada inicio (02-10)

**Antes:** en cada arranque se ejecutaban las **269 sentencias** de los 15
archivos de migración, y que eso fuera inofensivo dependía de que todas
estuvieran escritas de forma idempotente. No había registro de qué se habia
aplicado. Medido sobre la base real: 51 ms y **29 avisos** en cada inicio.

El riesgo no era el de los 51 ms: era que la seguridad dependía de una
convención. Bastaba con que alguien añadiera un `UPDATE` sin `WHERE` o un
`INSERT` sin `OR IGNORE` para que empezara a modificar datos en cada reinicio, y
nada en el código lo impediría.

**Ahora:** tabla `schema_migrations` (`filename`, `checksum`, `statements`,
`applied_at`). Un archivo ya aplicado con el mismo contenido **no ejecuta ni una
sentencia**. Tres reglas:

1. Si el contenido del archivo cambia, cambia su suma y se vuelve a aplicar.
   Editar una migración sigue significando "esto hay que ejecutarlo".
2. Si alguna sentencia falla, el archivo **no se anota** y se reintenta en el
   siguiente arranque. Anotar un archivo a medio aplicar dejaría el esquema
   incompleto sin que nadie lo volviera a mirar.
3. Si el registro no se puede leer, se aplica todo. Nunca se salta una
   migración por no haber podido comprobar qué se aplicó antes.

Medido sobre la base real del usuario:

| | antes | después |
|---|---|---|
| Sentencias en el 2.º arranque | 269 | **0** |
| Tiempo | 51 ms | **7 ms** |
| Avisos | 29 | **0** |

De paso, `duplicate column name` pasó a clasificarse como "ya estaba": SQLite lo
dice en minúscula y el comparador buscaba "Duplicate" en mayúscula, así que
cada `ALTER TABLE ADD COLUMN` sobre una columna existente contaba como fallo.

Las instalaciones existentes no tienen el registro: en su primer arranque con
esta versión se aplican las sentencias una última vez (idempotentes) y a partir
de ahí el arranque no ejecuta nada.

### Tema institucional y vistas por usuario (02-10)

**Color.** La paleta vive en una hoja nueva, `static/css/erpgow-tema.css`, que se
carga la última. No se tocó `custom.min.css`: es el tema heredado, minificado en
una línea, y editarlo obliga a meterse en 100 KB para cambiar un hexadecimal.
La hoja nueva declara los colores como variables CSS en `:root` y los aplica
sobre los selectores que el ERP usa. Cambiar la paleta institucional es, a
partir de ahora, editar ese bloque.

- Barra lateral (`<aside class="main-sidebar">`): **`#232525`**, submenús
  `#1a1c1c`, activo/cursor `#2f3333`, barra del ítem activo `#3f9d63` (un verde
  más claro que el institucional: sobre `#232525` el verde de marca no se
  distingue).
- Verde de acción `#1c6b3a` (5.9:1 sobre blanco, por encima del mínimo AA) y
  verde de resultado `#2f9c58`. Son tonos distintos a propósito: si fueran el
  mismo, un "Guardar" y un "Listo" se verían igual.
- Sustituidos los verdes del tema heredado (`#37a000`, `#45c203`, `#00a65a`) en
  enlaces, migas, paneles, botones, formularios, tablas, progreso, modales y
  avisos. También el fondo del login y de las pantallas sin layout.
- `bg-aqua` no existía en el tema heredado: las fichas del panel que lo usaban
  salían sin fondo. Está definido.

**Plantillas por usuario.** Sí es recomendable guardarlas, y **en la base de la
empresa**, no en la de control. La tabla es `user_view_pref`
(`username`, `view_key`, `name`, `payload`, `updated_at`; clave primaria triple).
Motivos:

1. La misma persona puede trabajar en dos empresas con menús y filtros
   distintos. Al vivir en el archivo de la empresa, esa separación sale gratis.
2. Un respaldo de la empresa se lleva por delante las preferencias de quien
   trabaja en ella. Si vivieran en `erpgo.db`, restaurar "la empresa del lunes"
   dejaría a su gente con las vistas del martes.
3. `report_template` (motor F5) es lo que parecía el sitionatural, pero es otra
   cosa: describe columnas y filtros de un reporte. Mezclarlas ataría la vida de
   un informe a la de un panel, y borrar un reporte borraría la vista de alguien.

`name` vacío = la vista activa del usuario; con nombre = plantilla guardada. El
`payload` es JSON opaco para el servidor: permite añadir una preferencia nueva
sin migración y evita que el sistema afirme entender lo que no entiende.

Superficie: `GET/POST /user/view-pref/{pantalla}` (con CSRF, porque no es una API
JSON), `static/js/viewpref.js`, y el panel de control como primera pantalla que
lo usa: botón "Personalizar vista", autoguardado y plantillas con nombre.
Verificado en ejecución: ocultar fichas, recargar, aplicar plantilla,
restablecer y eliminar.

Un detalle que salió al probarlo: con autoguardado, tres casillas desmarcadas en
el mismo instante lanzan tres POST y el servidor no tiene por qué escribirlos en
ese orden — el que llegara último dejaba la vista con la casilla equivocada. Por
eso `persist()` admite **una sola petición abierta** y manda el estado más
reciente al terminar la anterior.

### Validación en ejecución (02-10, `iniciar.bat`)

`iniciar.bat` compila, detecta que el 8080 está ocupado por `gateway.exe`, arranca
en 8081, abre el navegador en el puerto correcto y entra. Recorridas 204 rutas
HTTP autenticadas contra una base recién sembrada: **0 devuelven 500**.

Login comprobado de punta a punta contra la instalación real: `GET /login` → 200,
`POST` con `admin` / `Admin@2026` → 302 al panel, y el panel, RRHH, CRM, cierre de
períodos, motor de informes y selector de empresa responden 200.

### Bugs reales que los tests cazaron antes de producción

Estos no son hipotéticos: fallaron en la máquina del autor.

1. **Nombres de herramienta de IA duplicados en tres sitios.** El catálogo, el
   mapa de riesgos y el `switch` del dispatcher eran tres listas. `payment.record`
   se añadió a dos y se olvidó en la tercera → el guardián bloqueaba un cobro
   legítimo. Resuelto con una tabla única (`aiToolRegistry`).
2. **Bono porcentual pagado al doble.** Se resolvía contra el salario mensual y
   luego solo se prorrateaba el salario base. Un bono del 10% se pagaba 100+100
   en vez de 100 al mes.
3. **Columna inexistente en la consulta de nómina.** Se pedía
   `hr_salary_assignment.code`, que no existe (el código vive en
   `hr_payroll_concept`). La nómina entera habría fallado al ejecutarse.
4. **Una empresa creada desde el asistente no se podía usar.** El alta aplicaba
   el esquema pero no el administrador, ni los menús, ni el plan de cuentas: la
   empresa se creaba, se veía en el catálogo, y el login respondía "credenciales
   inválidas" con la clave correcta. Había dos caminos de creación y cada uno
   sembraba un subconjunto distinto de catálogos. Ahora los dos llaman a
   `PrepareCompany`.
5. **La empresa inicial no se registraba en el catálogo.** El servidor sembraba
   su base de datos y no la anunciaba: el login no encontraba ninguna empresa y
   mandaba al asistente, que creaba una segunda empresa vacía. El usuario nunca
   veía los datos que ya existían. Ahora `EnsureDefaultTenant` la registra al
   arrancar.
6. **Cuatro pantallas devolvían HTTP 500.** `crm_home`, `hr_home`,
   `hr_payroll_view` y `switch_tenant` se rompían: las tres primeras no definían
   el bloque `{{define "pagejs"}}` que el layout exige, y la cuarta se pintaba
   con el layout cuando es una página completa. El comprobador de plantillas las
   daba por buenas porque parseaba cada archivo por separado; ahora combina cada
   una con el layout y la ejecuta.
7. **SQL roto por un espacio faltante.** `FROM tax_rate_table` + `where` sin
   espacio producía `tax_rate_tableWHERE` y las tasas de RRHH devolvían 500.
8. **Campo inexistente en la plantilla de proveedores de IA.** La plantilla pedía
   `VendorLabel`, que el handler no rellenaba. El handler ya calculaba la
   etiqueta para la API JSON; ahora también la expone a la plantilla.
9. **`iniciar.bat` con saltos de línea LF.** Windows corta las líneas por la
   mitad (`echo` → `cho`) y el script deja de funcionar entero. Se escribió con
   CRLF. Además ahora mata cualquier `erpgow*.exe` previo (no solo
   `erpgow.exe`), elige el siguiente puerto libre y abre el navegador en el
   puerto real: antes abría el 8080 equivocado y el usuario veía un 404 de otro
   programa.

---

## 3. Diagnóstico: qué aporta cada sesión y dónde se tocan

### 3.1 Lo que aporta el trabajo de plantillas (`doc/plantillas/`)

La otra sesión leyó 31 archivos de plantillas SENIAT y extrajo datos duros:

| Artefacto | Contenido | Cómo se usa |
|---|---|---|
| `conasev_plan_de_cuentas.csv` | 1.855 cuentas CONASEV, jerarquía 2→6 dígitos | **Ya sembrado** en `acc_coa` |
| `seniat_tipo_documento.csv` | 52 tipos de documento con código SENIAT | Faltante |
| `tipo_voucher.csv` | 6 tipos de voucher con correlativo | Faltante |
| `intereses_prestaciones_lot.csv` | Tabla oficial art. 108 LOT/LOTTT, mes × año | **Alta prioridad** (P2) |
| `salario_minimo_historico.csv` | Salario mínimo 1989-2013 | **Alta prioridad** (P2) |

**El hallazgo grande** es `Calculos Salariales 2013.xlsm`: una app de nómina de
20 hojas con las fórmulas exactas ya resueltas. Auditado contra el motor actual
(`payroll.go`), esto es lo que falta:

```
FALTA en el motor actual:
  prestaciones sociales + interés art. 108 (tabla oficial)
  vacaciones 21+1 (máx 15) y bono vacacional 15+1
  utilidades 120 días con INCE 0,5%
  horas extras / nocturnas / feriados por turnos (50%/30%/50%)
  cesta ticket (0,25 UT hasta 3 salarios mínimos)
  salario integral diario
  liquidación de finiquito (arts. 141/143/190/192/131/92 y prestaciones ×2)
  catálogo de conceptos de recibo (001, 101-106, 200-209)
```

### 3.2 Colisión concreta detectada entre ambos trabajos

Verificado ejecutando un diagnóstico contra el catálogo embebido:

```
COLISIÓN  fixedAsset  101 -> CONASEV dice "Caja"
libre     cashCode    10201, bankCode 10205, purchaseCode 1020401,
          salesCode 3010301, customerCode 1020801, supplierCode 5020201,
          vat 4021101, salary_code 4020501 → no existen en CONASEV
```

**Esto es lo importante y hay que entenderlo bien:**

- Los códigos que el sistema asumía (7 dígitos: `10201`, `3010301`, `4021101`)
  **no existen** en CONASEV, que usa 2 a 6 dígitos. Son de un esquema numérico
  distinto.
- El único choque real es `fixedAsset = 101`, donde el sistema apunta a "101"
  (activo fijo) y CONASEV tiene "101 Caja". Es una colisión semántica: el mismo
  número significa dos cosas distintas.

**Consecuencia:** el plan CONASEV sirve para *mostrar* y *reportar*, pero no
puede sustituir todavía a `acc_predefine_account` sin reasignar los 13 campos.
Ese reasignamiento es el trabajo de P1, y hay que hacerlo con cuidado porque un
`HeadCode` mal apuntado produce asientos que cuadran contra la nada.

---

## 4. Principio que ordena todo: las tasas son datos, no código

Este principio ya está implementado (`tax_rate_table` con vigencia y versión) y
es la razón de que las plantillas de 2013 sean utilizables **a pesar de tener
diez años**.

Las cifras de las plantillas son de 2013-2020. El IVA era 16% (hoy 18%), la tabla
de intereses de prestaciones termina en 2016, y la Ley de Precios Justos ya fue
sustituida. **Por eso no se copian como constantes**: se siembran con
`verified = 0` y el sistema no deja publicar una nómina hasta que un humano las
valida.

Es la misma regla para CONASEV, para el catálogo de documentos y para el
histórico de salario mínimo: **dato versionado con vigencia y fuente, nunca
constante en el código**.

---

## 5. Hoja de ruta (orden único)

### P1 — Cerrar el plan de cuentas CONASEV
**Por qué primero:** es la base de todo lo contable. Sin plan de cuentas
alineado, los estados financieros no significan nada ante el SENIAT.

- Auditar las 1.855 cuentas sembradas (integridad referencial de la jerarquía).
- Resolver la colisión `fixedAsset = 101` **explícitamente**: reasignar los 13
  campos de `acc_predefine_account` a códigos CONASEV reales, o dejar el
  predefine como está y documentar que el catálogo CONASEV es de consulta.
- Test de invariante: todo `HeadCode` referenciado por `acc_predefine_account`,
  `invoice`, `purchase` y `acc_vaucher` debe existir en `acc_coa`.

### P2 — Núcleo LOTTT de nómina (prestaciones, vacaciones, finiquito)
**Por qué segundo:** es el trabajo de las plantillas que más valor tiene y más
riesgo de error legal. Una prestación mal calculada es una deuda real.

- Nueva tabla `hr_prestacion_accumulo` (depósitos por año, base del interés).
- `hr_prestacion_rate` sembrada desde `intereses_prestaciones_lot.csv`, con
  vigencia y `verified = 0`.
- Extender el motor con: vacaciones 21+1, bono vacacional 15+1, utilidades
  120 días, cesta ticket, horas extras por turno.
- Finiquito como módulo aparte (arts. 141/143/190/192/131/92, prestaciones ×2).
- Catálogo de conceptos con los códigos oficiales (001, 101-106, 200-209).

### P3 — Comprobantes de retención IVA (art. 11) e ISLR
- Módulo nuevo. El sistema ya tiene `audit_igtf.go` y las plantillas dan el
  formato oficial con numeración `AAAA/MM/00000001`.
- Alta prioridad fiscal: es una obligación de información periódica.

### P4 — Cuentas por cobrar con aging y cobranza
**El faltante más visible del día a día.** De las 18 planillas cifradas, cinco
son de cobranza (`Cuentas a Cobrar`, `Listado Cobranza`, `Seguimiento de
Recibos`, `Planes de cuotas`). El sistema tiene clientes con saldo pero no aging
ni gestión de cobranza.

### P5 — CRM, estructura de costos, centros de costo

**Estado real verificado el 04-10:** los tres están **sin empezar**. No existe
ninguna tabla de costos ni de centros de costo (ni en `migrations/`, ni en
`erpgo_default`). Lo único parecido es `product_category`, que es catálogo de
productos, y `crm_stage`, que son las etapas del CRM. Tampoco existe **etiquetado
(tags/labels)** en ninguna forma.

- **CRM**: ya existe `migrations/schema_z_crm.sql` (contacto, oportunidad con
  etapas, actividades). Falta la capa de servicio y la UI.
- **Estructura de costos**: se arma con datos que el sistema ya tiene (costo de
  compra, precio de venta, mes). Nota: la plantilla es de la Providencia
  070/2015, **derogada**; usar el modelo de cálculo, no las alícuotas.
- **Centros de costo**: reparto por operación o porcentaje.
- **Etiquetado**: no está en el plan original. Habría que decidir si son
  etiquetas libres sobre cualquier entidad (producto, cliente, factura) o
  categorías jerárquicas; lo segundo es lo que ya hace `product_category`.

### P6 — Módulos revelados por las planillas cifradas
Cuando se consiga la clave del SENIAT, o se re-descarguen sin protección:
caja con detalle por cuenta, órdenes de compra, guías de remisión, recibos,
**viáticos**, cartera de valores, tablero de comando.

### P7 — Agentes IA como empleados (04-10) — EN CURSO
**Por qué es el siguiente:** el mapa dinámico de la base ya está hecho y es la
base de todo lo demás. Ver [MAPA-DINAMICO-BD-PARA-LA-IA.md](MAPA-DINAMICO-BD-PARA-LA-IA.md).

Lo hecho: el agente recibe la estructura REAL de su departamento, leída de MySQL,
y el scoping está en el SQL, no solo en el prompt.

Lo que falta, en orden:

1. **Alta de agente IA como usuario.** Al dar de alta un empleado se elige tipo de
   agente, rol y departamento; con eso ya se puede cargar "un usuario que es un
   agente de CXC con el rol X". Hoy el agente se crea aparte y el usuario se
   referencia por `user_id`.
2. **Configuración de agente por módulo.** Un prompt de comportamiento por
   departamento, que se antepone al prompt del rol (el que el administrador
   escribe al crear el agente).
3. **Coste por tokens como partida.** `ai_usage` ya mide tokens y coste por
   agente y periodo; falta el asiento que lo lleva a gastos, porque la IA no
   está en nómina pero sí consume.
4. **Exportar el mapa.** Botón de descarga del JSON completo para el
   administrador.
5. **Vistas de solo lectura.** El introspector ya las detecta (`es_vista`); falta
   decidir cuáles se exponen al mapa.

---

## 5-bis. Adecuación visual total (03-10)

Todo el sistema, del login al último estilo. Criterio rector: **el color no
se usa para distinguir cosas, sino para avisar**. Quien pasa ocho horas
delante de estas pantallas no aguanta una pantalla que le reclama atención.

**Una sola capa de color.** `static/css/erpgow-tema.css` reescrito como
vocabulario de tokens semánticos: superficies, texto, líneas, acento, estados
y series. `erpgow-refined.css` queda solo con estructura (padding, radios,
sombras, rejilla); se le borró todo hex y el global `transition-duration: .18s
!important` que impedía cualquier transición de tema. El tema heredado
(`custom.min.css`, `style.css`) no se toca.

**Paleta grafito.** El acento (`#384049`), el resultado y el informativo son
tres grises distintos. El ámbar es lo que va a vencer y el rojo es error,
eliminación y deuda vencida. No queda verde en ninguna parte. La elección no
fue estética: los verdes ocupaban pantalla completa y cansaban.

**Tema por usuario, sin destello.** `data-theme` (claro/oscuro/automático) y
`data-density` (cómoda/compacta) viajan en el `<html>` de origen, escritos por
el servidor (`interfaces/web/appearance.go` + `render.go`). Se guardan en
`user_view_pref` con `view_key = "appearance"`: **sin migración y sin endpoint
nuevo**, reutilizando la tabla y el CSRF que ya existían. Con sesión manda la
tabla; sin ella, una cookie, para que el login no contradiga lo elegido.

**Botones.** Cuatro estados (primario, neutro, destrucción, enlace) con altura,
radio, sombra y foco únicos. `btn-info` y `btn-warning` pasan a neutro: antes
decían lo mismo que `btn-default` con tres colores distintos.

**Anti-saturación.** Las fichas del panel dejaron de ser bloques de color; son
superficie neutra con el icono en tono suave. El color fuerte queda para lo que
sí avisa: en el CRM, los cobros vencidos.

**Inline.** Los 46 estilos con color de 85 plantillas pasaron a clases del
tema. Los 400 restantes son medidas y no dependen del tema, así que se
congelaron (no se tocan).

**Guardias nuevas** (`templates_test.go`), porque estos defectos no se ven
hasta que llegan a casa del cliente:

- `TestNingunaPlantillaEscribeColorEnLinea` — el color no vuelve al HTML.
- `TestNingunaPlantillaDeclaraColorEnSuPropioEstilo` — ni en `<style>` incrustados.
  Las 8 vistas `*_print.html` quedan exentas: son el documento en sí, no una
  pantalla.
- `TestCadaPaginaSePintaEnTemaOscuro` — las 103 plantillas se pintan con el
  tema oscuro y el atributo llega al `<html>`.
- `appearance_test.go` — normalización, payload, cookie y clave de pantalla.

**Impresión.** `@media print` fuerza los tokens claros: el tema del usuario no
puede contaminar un libro legal.

**Tres defectos que solo aparecieron al mirar la pantalla** (ninguno lo
detecta un test, y por eso se documentan aquí):

1. **El logo del login no se veía en oscuro.** El nombre de la aplicación
   usaba `--erp-text-inverse`, que en el tema oscuro es casi negro, sobre un
   fondo de marca que es oscuro en los dos temas. Se creo `--erp-brand-ink`
   para lo que se escribe ENCIMA del fondo de marca.
2. **Los botones no se veían en claro.** El botón neutro era
   `--erp-surface` (blanco) con borde `--erp-border-strong` muy claro, sobre
   un panel blanco. Ahora el neutro usa `--erp-surface-2` (un escalón por
   encima) y el borde tiene más presencia.
3. **Las fichas del panel no tenían maquetación.** `.info-box` **no existe
   en ningún CSS heredado**: era un `<div>` con un rectángulo de color de
   fondo y el texto amontonado en una línea. Funcionaba de lejos porque lo que
   se veía era el bloque de color, no la ficha. Al quitar el color (que es lo
   correcto) se vio que faltaba la maquetación entera: icono en su cuadrado,
   rótulo y cifra debajo. Está construida en la capa del tema.
4. **Los botones de tema del login se perdían contra la caja blanca** en
   tema claro. Se probaron dos arreglos y el primero era una trampa: ponerlos
   fuera de la caja, sobre el fondo de marca, funcionaba… mientras el fondo de
   marca siga siendo oscuro. Si algún día ese fondo pasa a ser claro, los
   botones vuelven a desaparecer sin que nada avise. Lo que se hizo al final
   es un control segmentado DENTRO de la caja, con la lectura apoyada en el
   TEXTO (`--erp-text`) y no en el relleno, y la selección marcada con una
   pastilla y un borde. Así funciona en los dos temas y no depende de ningún
   color de fondo.

   Al medirlo(resultado: 11.3:1 en claro y 11.4:1 en oscuro sobre su fondo)
   aparecieron tres fallos de paleta que no se ven a ojo: el texto atenuado
   estaba en 4.05:1, la superficie secundaria se separaba de la superficie en
   1.07:1, y el borde fuerte del tema oscuro se perdía sobre ella. Los tres
   están corregidos (`#5a636e`, `#dbe0e4`/`#2f373e`, `#454f58`).

5. **Estos fallos solo se ven mirando la pantalla, así que ahora se miden.**
   [contrast_test.go](../interfaces/web/contrast_test.go) lee
   `static/css/erpgow-tema.css`, saca el valor de cada variable en cada tema y
   calcula la razón de contraste con la fórmula de WCAG. Comprueba el texto
   (4.5:1), que las superficies se separen por el borde y no por el relleno, y
   que ninguna variable se añada al tema claro y se olvide en el oscuro. Se
   verificó que **falla** volviendo a poner el botón neutro en blanco, que es
   el fallo original. Nota: ignora a propósito el bloque `@media print`, que
   redeclara todos los tokens en blanco y negro y haría creer que el tema
   oscuro es blanco.

**Aviso para quien siga**: `static/` va EMBEBIDO en el binario. Compilar
`erpgow.exe` con la terminal abierta y recargar no basta: si el proceso sigue
corriendo, sirve los assets del binario viejo y los cambios de CSS no se ven
aunque el archivo en disco sea el correcto. Hay que **cerrar el proceso,
recompilar y arrancar**. El compositor del panel de vista previa en este
entorno tampoco repinta de forma fiable, así que las capturas pueden salir
desfasadas respecto al DOM; para juzgar el resultado hay que leer los estilos
calculados
(`getComputedStyle`), que sí reflejan el render real.

**Deuda que queda a propósito, documentada en el propio archivo:**

- `.btn` necesita `!important` en el borde porque `style.css` pone
  `border: none !important` en todos los botones. Competir con un `!important`
  del tema heredado no tiene otra forma.
- `custom.min.css`, `style.css` y `newstyle.css` siguen cargando. Retocarlos
  obligaría a editar 100 KB minificados que cualquier actualización del
  proveedor pisaría.
- `semantic.min.css` (537 KB) y `bdtask-main.css` (336 KB) **no los carga
  ninguna plantilla**. No cuestan ancho de banda, pero están en el disco y
  confieren: si alguien los añade alguna vez, entra 873 KB de golpe. Borrarlos
  o documentarlos como material de consulta es una decisión pendiente.

---

## 7-bis. Fallo conocido, ajeno a lo visual

`TestEncoladoRAM_vs_Directo` (`infrastructure/persistence/queue_bench_test.go`)
falla al escribir de 10 000 a 12 600 ventas: `se perdieron ventas`. Reproducido
con el servidor parado, así que **no lo ha causado el trabajo visual**. El modo
que peor sale es el DIRECTO, que no usa la cola, así que el problema está en el
camino de escritura y no en el encolado. Es un test de carga de 1,5 M de
escrituras: conviene confirmarlo en la máquina que vaya a producción antes de
darlo por bueno.

---

## 5-bis-bis. Compilar en otra computadora (03-10)

Guía completa en [COMPILAR-EN-OTRA-PC.md](COMPILAR-EN-OTRA-PC.md): qué se
necesita en cada caso, qué falta por hacer y los bloqueos reales.

Lo mínimo para trabajar es **Go 1.26+**. No hace falta compilador de C.
Para el binario protegido, la FieldKey puesta. Para el de lanzamiento,
además **Go 1.27+ y garble**, porque garble v0.18 no funciona con Go 1.26 y
aborta con "Go version is too old".

**CGO está disponible** (corregido el 03-10): MSYS2 con mingw-w64 en
`C:\msys64`, gcc 16.1.0. El proyecto entero compila con `CGO_ENABLED=1`
(verificado, 44,3 MB), así que **SQLCipher no está bloqueado por falta de
herramientas**. El bloqueo que se había documentado era falso.

> El gcc se usa **solo en la orden concreta**, nunca se añade a las variables
> de entorno del sistema: el PATH del sistema se queda como está. Dentro de un
> `.bat` va con `setlocal`, así que al salir no queda rastro.

---

## 5-cuad. Contraseñas y respaldos (03-10) — HECHO Y VERIFICADO

**Contraseñas.** El hallazgo S-0 estaba **MAL, y se ha corregido**. Medido sobre
las bases de verdad, no supuesto:

| Usuario | Antes | Ahora |
|---|---|---|
| `admin` | bcrypt | bcrypt |
| `revision` | `revision123` en texto plano | bcrypt, migrado al entrar |

Lo que había era que el login **ya usaba bcrypt** (`handlers.go:150`) y `user.go`
**ya generaba bcrypt**. Lo que quedaba eran filas heredadas, más una función
muerta `hashPassword` que decía «crea un hash bcrypt» y devolvía la contraseña
sin transformar (y que **nadie llamaba**).

Lo hecho:

- **`hashPassword` borrada**. No se arregla: si se arreglara, quedaría otro sitio
  donde escribir contraseñas.
- **`interfaces/web/password.go`**: un solo sitio para verificar y otro para
  crear hashes, con nombres que no mienten.
- **Rehash al entrar**: si alguien entra con una contraseña heredada, la fila se
  reescribe a bcrypt en ese instante. Es el único momento en que se tiene la
  contraseña en claro *y* se sabe que es correcta.
- **Comparación en tiempo constante**: el código anterior usaba `==`, que
  devuelve en cuanto encuentra una diferencia y filtra información por el
  tiempo que tarda.
- **7 pruebas** en `password_test.go`.

Verificado con el servidor de verdad: `revision` entró con su contraseña en
texto plano (HTTP 302) y la fila quedó en `$2a$10$…`; la misma contraseña
siguió entrando y una incorrecta fue rechazada.

**Respaldos.** `respaldar.bat` copia todo el proyecto a un ZIP fechado, con
SHA-256 y listado del contenido. Excluye **solo** `backup/` (si no, el respaldo
se incluiría a sí mismo y crecería sin límite) y las carpetas de compilación de
Go. **Las bases de datos sí van dentro**: sin ellas el respaldo no vale.
Ejecutado: 44,5 MB, 2062 entradas, verificado que abre.

Aparte: detectada y eliminada una base de datos de 1,1 MB que los **tests**
creaban dentro del código fuente (`interfaces/web/empresas/`). No se borró solo
el fichero, sino la causa: `testAIConfigSys()` no ponía `DBSQLitePath`, así que
`DataPath()` resolvía `data/empresas/` relativo al directorio del test. Ahora usa
`t.TempDir()`.

---

## 5-bis-ter. Orquestador de escrituras (03-10) — HECHO Y VERIFICADO

La puerta única por la que tienen que pasar las escrituras.
Código en [internal/orchestrator/](../internal/orchestrator/) y la conexión
al servidor en [orchestrator.go](../interfaces/web/orchestrator.go).

Lo que resuelve, y estaba roto:

- **El rastro era opcional.** `audit_log` existe y está bien hecho (cadena
  SHA-256, bloques mensuales), pero cada handler lo llamaba si se acordaba. Una
  llamada opcional en un sitio que nadie audita es una llamada que se pierde, y
  entonces el bloque mensual queda mal sellado sin que nada lo diga. Ahora el
  INSERT del rastro va **en la misma transacción** que el dato: si no se puede
  escribir, se deshace todo y el dato no entra.
- **Correlativo propio**, que es la posición en la fila y no el `id` de SQLite.
  Vive en la cabecera del journal, en disco, así que sobrevive a un reinicio.
- **Nunca rechaza; frena.** Con la cola llena, el productor espera. Es lo mismo
  que se corrigió en `internal/queue`, aplicado aquí a las escrituras.
- **Idempotencia por clave única**: la misma clave dos veces escribe una vez,
  incluso tras una caída.
- **Limpia, no borra**: tras una pausa sin escrituras el journal se queda en su
  cabecera, y **limpiar no reinicia el correlativo en 1**.
- **Cifrado con la FieldKey** (AES-GCM). Sin FieldKey va en claro, que es el
  caso de desarrollo.

Verificado con 13 pruebas del paquete y 5 de la conexión al servidor, y
arrancando el binario de verdad: el log dice `orquestador de escrituras activo`
y el journal aparece en `data/escrituras/<empresa>.journal`.

**Lo que NO está hecho, y hay que decirlo claro**: los handlers antiguos que
hacen INSERT por su cuenta (97 en el código) **siguen estándolo**. La puerta
existe y funciona, pero migrar los handlers uno a uno es el trabajo que queda.
Lo que ya no es cierto es que no exista camino.

Criterio para darlo por cerrado: un test que falle si aparece un INSERT fuera de
la puerta en los módulos de negocio. Ese test es el siguiente paso, porque es
el que obliga al resto.

## 5-cuad-bis. Banco de pruebas de escritura: lo que se midió (04-10)

Herramienta: `tools/estress` (`-modo cola|directo`). Resultados en
[benchmarks/](benchmarks/INDICE.md): un `.json` y un `.md` por ensayo, más un
`INDICE.md` que se **rehace** desde los JSON (ya no se reescribe a sí mismo: se
duplicaba la cabecera y perdía las filas).

### Ensayo 1 — ¿Es la estructura de la BD o el insert? (el que pedía la pregunta)

4 cajas x 25 facturas x 150 artículos = 15.000 líneas, **el mismo trabajo por los
dos caminos**, base y esquema reales.

| | directo | cola |
|---|---|---|
| líneas/s | **6.026** | 5.681 |
| p50 factura | 24,8 ms | 104,0 ms |
| p99 factura | 1.085,8 ms | **142,5 ms** |
| max factura | 1.695,0 ms | **145,5 ms** |
| colisiones | 0 | 0 |
| escrituras fallidas | 0 | 0 |
| líneas perdidas | 0 | 0 |
| inventario descuadrado | 0 | 0 |

**El insert NO es el problema.** 6.000 líneas/s en un solo proceso con el
esquema real de 300+sentencias. Un INSERT no tarda: lo que tarda es *competir
por el único lock de escritura*, y eso es SQLite, no el ERP.

Directo gana en caudal y **pierde en justicia**: p99 de 1,09 s frente a 142 ms.
Cuatro cajas compitiendo en vez de encolar. Con 200 cajas eso no es una latencia
alta, es el colapso que se vio antes en la caja 20.

### Ensayo 2 — 200 cajas a la vez (600.000 líneas)

| | |
|---|---|
| total | 506,97 s |
| líneas/s | **1.183** |
| p99 factura | 34.833 ms |
| colisiones | **0** |
| escrituras fallidas | **0** |
| líneas perdidas | **0** |
| inventario descuadrado | **0** |
| lecturas ok / fallidas | 21.472 / **0** |

Cero colisiones y cero pérdida a 200 cajas, y los 8 lectores no se pararon ni una
vez: 21.472 lecturas, 0 fallidas. Lo que falla es el plazo, y falla por aritmética:
30 M de líneas a 1.183/s son ~7 horas. **El objetivo de 5 s por caja con 150
artículos no lo cumple ningún encolador del mundo sobre un solo archivo SQLite.**

### El WAL: qué es y por qué crecía (medido, no supuesto)

Mismo trabajo exacto (12 cajas x 50 facturas x 150 artículos) dos veces, lo único
que cambia son los 8 lectores. Mirando el fichero `-wal` en disco mientras corre:

| tiempo | sin lectores | con 8 lectores |
|---|---|---|
| 5 s | 4,5 MB | 0,8 MB |
| 15 s | 4,8 MB | 39 MB |
| 25 s | 5,1 MB | **124 MB** |
| 30 s | 5,3 MB | **179 MB** |

**Sin lectores el WAL se queda plano en 5 MB. Con lectores crece sin parar.** Eso
confirma la causa, que hasta aquí era una hipótesis.

**Qué es el WAL.** En modo WAL, SQLite no escribe en el fichero de la base: escribe
en un log aparte, el `-wal`. Cada cierto número de páginas (por defecto 1000,
unos 4 MB) lo pliega de vuelta dentro del fichero principal y lo reinicia. Eso es
el *checkpoint* automático, y es lo que mantiene el WAL en 5 MB.

**Por qué un lector lo rompe.** Para plegar el log SQLite tiene que saber que nadie
está leyendo una versión antigua de los datos. Si un lector está a medias de un
`SELECT` largo, no puede saberlo todavía, así que **no pliega nada y sigue
añadiendo**. El WAL crece, y no tiene tope: solo para cuando el lector acaba.

**Lo grave es que no avisa.** No hay ningún error, ningún aviso, la aplicación
sigue funcionando perfectamente. Solo aparece el fichero grande. Con 4 GB de disco
restantes, un cliente cobra, otro cobra, y de pronto la caja dice «no se pudo
guardar» sin decir por qué.

**Qué tan realista es.** El banco usa 8 lectores en bucle cerrado haciendo
`SELECT SUM(quantity) FROM invoice_details`, sin `WHERE`: un recorrido COMPLETO
de la tabla, cientos de miles de filas, una vez cada milisegundo. Eso es más
agresivo que cualquier pantalla real.

Medido contra la base real de 500.000 filas:

| | |
|---|---|
| `EXPLAIN QUERY PLAN` | **`SCAN invoice_details`** (recorrido completo) |
| duración de la lectura | **234 / 205 / 219 ms** cada vez |
| misma consulta con `WHERE invoice_id` (usa el índice) | 58 ms frío, **1–2 ms** caliente |

Los 8 lectores solapan 215 ms de duración cada 1 ms de pausa, así que **hay
siempre uno dentro** y SQLite no puede reiniciar su log. Conviene decirlo claro
para no repetir este error: **no es falta de índice.** `invoice_details` ya tiene
`idx_invoice_details_invoice_id` (comprobado en `sqlite_master`), y un índice no
ayudaría a esta consulta aunque no lo tuviera, porque suma **todas** las filas.

Pero **el fallo existe y no tiene techo**: basta con que un informe pesado o una
pantalla lenta tengan una lectura abierta unos segundos mientras entra caja
fuerte. La diferencia entre el banco y la producción
es de frecuencia, no de naturaleza.

### El WAL: RESUELTO (04-10)

**Estado: arreglado y con tests.** Lo que sigue es el recorrido completo, porque
lo que se aprendió en los tres intentos fallidos vale más que el arreglo.

#### El arreglo, en una línea

**El límite lo pone SQLite (`busy_timeout`), no un `context.WithTimeout`.**

Las tres versiones anteriores fallaron por lo mismo: cancelaban el checkpoint a
medias con un contexto y le dejaban el candado puesto al escritor
(`SQLITE_BUSY` en la caja que estaba cobrando). SQLite ya sabe cuánto esperar:
cuando se le acaba devuelve `SQLITE_BUSY` **limpio**, sin dejar nada a medias.
Un contexto cortando a medias es otra cosa: interrumpe un trabajo a medio hacer.

Función: `persistence.ForzarCheckpointWAL`.

```go
ctx, cancel := context.WithTimeout(context.Background(), plazoConexionWAL) // SOLO para coger la conexión
conn, _ := db.Conn(ctx)
conn.ExecContext(ctx, "PRAGMA busy_timeout=2000")   // el límite real, lo pone SQLite
conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
```

#### Prueba del diagnóstico, sin escritor de por medio

````
ANTES   wal=4128272  base=3203072
ronda 0: err=<nil> ocupado(busy)=0 enLog=0 plegados=0 wal=0
DESPUES wal=0       base=3530752
```

**4.128.272 bytes → 0.** Con `busy=0` y sin error.

#### Dónde está conectado

| sitio | por qué |
|---|---|
| `cmd/server/main.go` | base principal, con `defer` al cerrar |
| `tenants.go → openTenantDB` | **una vigilancia por cada empresa**, que es el fichero que se llena de verdad |
| `tenants.go → controlStore` | el catálogo `erpgo.db` |
| `tenants.go → Close` | se para ANTES de cerrar los handles, para no dejar goroutines sobre ficheros cerrados |

Cada empresa tiene su propio vigilante con su clave, así que dos empresas no se
pisas y ninguna vigila dos veces.

#### Tests (7, todos en verde)

| test | qué demuestra |
|---|---|
| `TestVaciarElWALNoRompeAlQueEscribe` | **el que importa**: con escritor corriendo, 0 fallos; al calmarse, WAL = 0 |
| `TestElCheckpointNoDejaCandadoAlEscritor` | 5 rondas de vaciar+escribir sin `SQLITE_BUSY` — el fallo original |
| `TestConLectorAbiertoNoSeCuelga` | vuelve en tiempo habil con lector abierto y la base sigue escriturando |
| `TestTamanoWALMideElFicheroDeVerdad` | la medida lee el disco de verdad (si no, todo lo demás pasa en vacío) |
| `TestRutaFicheroDevuelveElFicheroReal` | ídem |
| `TestElVigilanteArrancaYPara` | no se cuelga al parar; parar dos veces no revienta |
| `TestElVigilanteVacíaElWALSoloAlSuperarElUmbral` | llega a llamarse solo, sin que nadie lo pida |

#### Los cuatro intentos previos, por qué fallaron

| intento | qué se hizo | por qué falló |
|---|---|---|
| 1 | `PASSIVE` en cada vuelta | Copia al log pero **no vacía el fichero**. Acota el trabajo, no el disco. |
| 2 | `TRUNCATE` con `context.WithTimeout` | Vacía, pero **cortar a medias deja el candado puesto**: `SQLITE_BUSY` al escritor. |
| 3 | TRUNCATE con presupuesto de 2 s | 185 → 72 → 51 MB sin recuperar. Sigue siendo una cancelación a medias. |
| 4 | TRUNCATE por umbral y proporción | El umbral no se alcanzaba y el disparador se quedaba sin activarse. |

**Ninguno de los cuatro falló por una limitación de SQLite.** Fallaron todos por
la forma de pedirle el trabajo. Tres de ellos fueron el mismo error repetido.

#### Lo que sigue pendiente (y NO es necesario para que el WAL esté arreglado)

- **El lector del banco de pruebas**: hace `SELECT SUM(quantity) FROM
  invoice_details` sin `WHERE` cada milisegundo. Medido: `SCAN` de 500.000 filas,
  **205–234 ms cada vez**. Eso es lo que más estira el log en los ensayos.
  Arreglarlo mejora el banco, no el producto.
- **Alarma de tamaño de `-wal`** visible para el operador. El endpoint ya existe
  (`POST /settings/database/checkpoint`). El vigilante ya imprime el aviso por
  consola; falta exponerlo en pantalla.


### Decisión

**Se usa el encolador.** No es una preferencia: es lo que medido da mejor p99,
lo que no pierde una línea, y lo que ya está construido, auditado y con tests
(18). El modo directo queda en el banco como término de comparación, no como
camino de producción.

**Y con la escala real que citas (50 cajas, no 200), SQLite aguanta de sobra.**
Sale de lo medido, no de optimismo: el encolador hace una factura de 150 artículos
(cabecera + 150 líneas + 150 movimientos + rastro = 302 inserts) en **104 ms** en
el ensayo pequeño, o sea **2.900 inserciones por segundo**. Pero **ese es el
mejor caso, no el habitual**: en `ANTES-50cajas-500art` —50 cajas de verdad y
500 artículos por factura— el ritmo sostenido fue **1.198 líneas/s y 2
facturas/s**. A ese ritmo, tu ticket de 500 artículos (1.002 inserts) son **~0,85
s de escritor, no 0,35 s**, y el escritor aguanta **uno y algo** simultáneos,
tres no.

Aun así, **50 cajas caben de sobra**, y la razón es distinta: mientras una caja
carga su ticket grande, las otras 49 cobran de a 2 o 4 artículos, que a 1.198
líneas/s son **~3 ms por factura**. El cuello de botella no es el caudal medio,
es el pico, y el pico está lejos de saturar al escritor.

El objetivo original de 200 cajas x 1.000 facturas x 150 artículos en 5 s era
inviable, y está bien que lo fuera: se pidió a un solo fichero SQLite 30 M de
líneas en cinco segundos, y eso son 6 M de líneas por segundo. Como límite de diseño
-era absurdo-, pero como **banco de pruebas ha sido útil**: por él salieron el
descubrimiento del pool inanido, el de la latencia injusta del modo directo, y el
WAL sin techo. Ese era el objetivo.

### Motor de datos: SE QUEDA SQLite (04-10) — decisión tomada

Se planteó pasar a PostgreSQL para evitar bloqueos. **Se descarta.** Medido:

| | |
|---|---|
| ficheros con SQL directo | **64** |
| puntos de apertura de base (la costura) | **1** — `persistence/database.go:44` |
| driver | `glebarez/go-sqlite` — **puro Go, sin CGO ni gcc** |
| modelos multi-empresa | un fichero por empresa |

Un cambio de motor con esa superficie no es una configuración: es reescribir la
capa de persistencia entera, traducir el dialecto de 64 ficheros, sustituir
`sqlite_convert.go` y replantear el aislamiento por empresa. Con 5 días para la
entrega sería el cambio más arriesgado posible.

**Y el miedo va al revés:**

- **SQLite no puede hacer deadlock.** Tiene UN candado sobre el fichero, o se
  tiene o no se tiene. No hay orden de candados ni espera circular, así que la
  espera circular es **imposible por construcción**. Lo único que existe es
  `SQLITE_BUSY`, que ya lo resuelve `busy_timeout(10000)` en el DSN.
- **PostgreSQL SÍ hace deadlock**: dos transacciones actualizando filas en
  orden distinto, espera circular, PG la detecta al segundo y **mata una**,
  obligando a reintentar. Cambiar sería **introducir el fallo que se teme**.

**Capacidad.** Medido a escala real (50 cajas x 500 artículos):
**1.198 líneas/s sostenidas, 0 colisiones, 0 escrituras fallidas, 0 líneas
perdidas**. PostgreSQL se necesita con ~10 escritores intensivos sostenidos o
con varios servidores de aplicación en horizontal. 50 cajas no se acerca.

**Los "atascos" ya vividos estaban diagnosticados y arreglados**, y ninguno
venía del modelo de SQLite:

| lo que pasó | causa real | estado |
|---|---|---|
| se colgaba al subir de escrituras | inanición de pool (lectores se llevaban los 25) | ✅ dos handles |
| `SQLITE_BUSY` en lecturas | `_txlock=immediate` hacía que **leer** tomara lock de escritura | ✅ handle de lectura aparte |
| 3 facturas tardando minutos | aplicador fallando + 5 reintentos con 10 s de `busy_timeout` | ✅ contexto 60 s, `MaxIntentos: 1` |
| WAL crece sin tope | lectura de 215 ms abierta en bucle | ⚠️ 3 pasos pendientes |

**Queda como pendiente la red de seguridad que cubre el miedo de verdad:** un
vigilante de progreso que avise si el escritor no completa nada en N segundos.
Eso cubre cualquier atasco futuro —el que sea— sin tener que adivinarlo hoy.

## 5-ter. Seguridad del binario y de los secretos (03-10) — EN CURSO

Diseño en [SEGURIDAD.md](SEGURIDAD.md), historial en
[BITACORA-SEGURIDAD.md](BITACORA-SEGURIDAD.md).

**Hecho y verificado**: `FieldKey` existe (`cmd/server/fieldkey.go`), con su XOR
y su sello SHA-256; los tres `.bat` compilan y el sello coincide; el **sello se
comprueba al arrancar** (`Integridad: OK`, verificado con el binario real, con
el binario alterado y con el sello de otro binario); la FieldKey **ya llega al
orquestador** y el journal va cifrado; y el **orquestador de escrituras** está
construido y conectado (5-bis-ter). Contraseñas migradas a bcrypt (5-cuad).

**Pendiente**: la base se sigue abriendo en claro (SQLCipher).

Tres medidas pedidos: cifrar la clave con una llave que solo el binario conoce
(XOR o similar), garble sin `-tiny`, y verificación de integridad del binario.
Al mirar el terreno, ninguna es aplicable tal cual sin responder antes algo:

- **`FieldKey` no existe en el código.** Lo que hay con valor real son
  `ai_setting.api_key` y `ai_providers.api_key`, **en texto plano en la base de
  datos**. Hay que decidir cuál de las dos es "la clave".
- **No hay repositorio git.** garble necesita git para compilar de forma
  reproducible, y sobre todo hace falta un punto de vuelta: una ofuscación mal
  aplicada deja el binario sin arrancar y no hay forma de deshacer.
- **La llave embebida no protege de quien tenga el `.exe`**, solo de quien tenga
  el `.sqlite3`. Para proteger de los dos, la clave no debe guardarse: que se
  teclee en cada arranque o que viva en el almacén de credenciales del sistema.
- **XOR no autentica**: alteran un byte del texto cifrado y el programa lo
  desencripta sin avisar. AES-GCM, que es igual de fácil, detecta la
  manipulación.
- **La auto-verificación dentro del binario no protege**: quien parchea el
  binario parchea también la comprobación. Para anti-tampering de verdad, la vía
  es Authenticode (firmado por Windows antes de ejecutar).

**CORRECCIÓN (03-10).** Este hallazgo estaba **equivocado** y se corrige aquí
para que nadie lo dé por bueno: `hashPassword` en
[audit_igtf.go](../interfaces/web/audit_igtf.go) devolvía la contraseña sin
transformar, pero **nadie la llamaba**, y el login real ya usaba bcrypt. Medido
sobre las bases de verdad: `admin` en bcrypt, y solo una fila heredada en texto
plano, ya migrada. La función se ha borrado y el problema real (las filas
heredadas) está resuelto con el rehash al entrar. Ver sección 5-cuad.

Lo que **sí** era cierto de este punto, y sigue siendo el hallazgo más grave de
la lista: `ai_setting.api_key` y `ai_providers.api_key` están **en texto plano
en la base de datos**. Garble no lo arregla: no tiene que ver con el binario.

**Orden recomendado**: S-0 (bcrypt) → decidir D-1/D-2 → git → garble → cifrar
`api_key` → integridad. Las cuatro decisiones pendientes (D-1 a D-4) están en
la bitácora.

---

## 6. Cómo trabajamos sin pisarnos

1. **Una sola hoja de ruta, este archivo.** Quien la lea, sabe qué le toca.
2. **Regla de propiedad:** cada archivo tiene un dueño. Antes de editar uno que
   no sea tuyo, comprobar `git status` y asumir que el cambio en el working copy
   es de la otra sesión.
3. **Nada de commitear sin avisar.** Todo sigue sin commitear a propósito.
4. **Todo dato normativo entra como tabla versionada**, nunca como constante ni
   como número mágico en el código.

---

## 7. Advertencias honestas

- Las plantillas tienen **diez años**. Sirven como *modelo de cálculo
  parametrizado por fecha*, no como fuente de cifras actuales.
- **18 de 31 planillas están cifradas con clave.** Se leen los nombres, que ya
  revelan el catálogo de módulos que faltan, pero no el contenido. Con la clave
  del SENIAT se pueden extraer.
- El `HeadCode` de `acc_coa` es una columna **numérica**: no admite ceros a la
  izquierda. Las cuentas de orden del catálogo CONASEV que empiezan en `0` se
  descartan al sembrar (está documentado en `conasev.go`).
- `go test -race` no está disponible en esta máquina (requiere cgo).
- El paquete `web` tiene archivos preexistentes que `gofmt` marca como no
  formateados. Es baseline: no re-formatear, para no ensuciar el diff.