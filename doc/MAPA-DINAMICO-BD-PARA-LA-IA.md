# El mapa de la base de datos para la IA: leído de MySQL, no escrito a mano

**Estado:** implementado y probado. **Fecha:** 2026-10-04.

Este documento explica qué se construyó, por qué, y qué se midió. Los bugs que
aparecieron están aquí porque son el hallazgo real de la etapa: el diseño estaba
bien, la implementación no lo estaba, y solo se vio hablando con un modelo de
verdad.

> **Este documento es la parte de IA del patrón de desarrollo.**
> Los pasos 7 y 8 del apartado 7 están aquí; los otros siete están en
> [PATRON-UNICO-DE-DESARROLLO.md](PATRON-UNICO-DE-DESARROLLO.md), con la
> plantilla copiable en [doc/plantillas/modulo/](plantillas/modulo/).

---

## 1. El problema, dicho sin rodeos

El agente IA recibía un mapa de la base de datos que estaba **escrito a mano en
código**: una lista de 40 tablas con su descripción, y para cada una, una
consulta a `information_schema` dentro de un bucle.

Eso producía tres fallos concretos, todos observados:

| Fallo | Cómo se manifiesta |
|---|---|
| Una tabla nueva no aparecía | El agente recibía 40 tablas de una base que tiene 131, y concluía que la que necesitaba no existía. No era que se la inventara: es que nadie se lo había dicho. |
| Las columnas se leían en bucle | 40 viajes de red por llamada, y `aiColumnList` hacía `continue` al fallar una consulta: el prompt salía con columnas de menos y sin ningún aviso. |
| No había departamentos | Un agente de cuentas por cobrar recibía el mapa entero: contabilidad, RRHH, nómina. Enorme, caro, y le enseñaba la estructura exacta de tablas que no le tocan. |

La frase que resume el enfoque: **no es un problema de prompt, es un problema de
sistema**. El modelo no adivinaba porque el prompt fuera malo; el sistema no le
decía lo que hay.

---

## 2. Lo que hay ahora

### 2.1 Un introspector real

[ai_schema_introspect.go](../interfaces/web/ai_schema_introspect.go) lee la base
**conectada** (la de la empresa de la sesión) en **cuatro consultas** a
`information_schema`, no una por tabla:

- tablas y vistas, con su comentario;
- **todas** las columnas de **todas** las tablas, de una vez;
- claves primarias y foráneas declaradas;
- una **huella** del esquema (`BIT_XOR(CRC32(...))` sobre todas las columnas).

Y devuelve JSON, no prosa:

```json
{"tabla":"invoice",
 "descripcion":"Facturas de venta (cabecera): cliente, fechas, totales...",
 "columnas":{"id":"int","invoice_id":"int","total_amount":"decimal","due_amount":"decimal"},
 "pk":["id"],
 "join":"JOIN customer_information c ON c.customer_id = invoice.customer_id"}
```

Lo que **no** va dentro, y por qué: nulabilidad, valores por defecto y
autoincremento. Son metadatos de escritura y este agente escribe plantillas de
informe, no filas. Lo que sí va es la descripción de negocio, que es la
diferencia entre que el modelo adivine qué hace `acc_vaucher` y sepa que
`Vtype` distingue JV de CV de DV.

### 2.2 Una caché con invalidación por huella

La huella se pregunta en cada llamada (es una agregación de milisegundos) y solo
si ha cambiado se relee el esquema. Sin esto había que elegir entre pagar la
introspección en cada paso del bucle de un agente o servir un mapa mentiroso
indefinidamente.

La caché va por **código de empresa**, no por handle de conexión: si el pool de
una base se cierra y se reabre, el esquema no ha cambiado.

### 2.3 Siete departamentos, y el scopado va en el SQL

| Departamento | Para qué |
|---|---|
| `it` | Soporte y desarrollo. El único que ve la base entera, porque es el que diseña informes. |
| `fac` | Facturación y ventas |
| `cxc` | Cuentas por cobrar |
| `cxp` | Cuentas por pagar |
| `contb` | Contabilidad |
| `rrhh` | Recursos humanos |
| `inv` | Inventario |

El departamento se deduce del **rol del agente** (`aiDepartamentosDeRol`), y el
agente es un empleado más: si su rol es administrador ve todo.

**Y aquí está lo que importa.** Recortar el prompt es educación. Recortar el SQL
es un candado. [ai_sql_scope.go](../interfaces/web/ai_sql_scope.go) extrae las
tablas de la sentencia y las contrasta con las del departamento **antes** de
ejecutarla:

```go
clean, err := aiValidarSQLConScope(sqlText, dept, limit)
```

Un modelo que se sabe las tablas de memoria, que las deduce, o que las leyó en un
informe anterior, recibe el mismo rechazo. El texto del error lo pone el código:
*"tu rol (Cuentas por Cobrar) no tiene acceso a acc_coa; solicítalo a un
administrador si lo necesitas"*.

### 2.4 La whitelist sigue siendo la frontera de seguridad

Hay dos filtros, y los dos importan:

1. **Whitelist** (`aiCatalogWhitelist`): una tabla que no está ahí no sale, ni
   aunque exista en la base. Es lo que impide que la IA lea `user_login` o
   `users`. **El filtro se aplica al leer**, no al scopear: que sea el único punto
   por el que se sale de la base evita que un camino nuevo se quede con la lista
   completa por olvido.
2. **Departamento**: qué área ve qué.

Ni soporte ni desarrollo ven la tabla de credenciales. Ser IT no da acceso a
`user_login`.

### 2.5 La parte que NO es automática (y hay que saberlo)

Las **columnas** se leen de MySQL: una migración añade una columna y el mapa la
refleja sin tocar código.

La **asignación de una tabla a un departamento** es una tabla escrita a mano
(`aiDeptDeTabla`, 75 entradas). Consecuencia medida, no supuesta:

| Situación | Quién la ve |
|---|---|
| Tabla nueva, no está en la whitelist | **Nadie**, ni siquiera IT |
| Tabla en la whitelist, sin departamento asignado | **Solo IT** |

Es decir: una tabla que se crea mañana no la ve ningún agente de negocio hasta que
alguien la añada a `aiDeptDeTabla`. Quien la meta solo en la whitelist creerá que
ya la ha hecho visible para el negocio, y no lo es.

Es la misma clase de fallo que ya se cometió dos veces aquí (RRHH con 0 tablas por
nombres inventados; el scopado de IT devolviendo las 131 tablas), así que está
fijado con dos tests en
[ai_dept_tabla_nueva_test.go](../interfaces/web/ai_dept_tabla_nueva_test.go):

- `TestTablaNuevaNoLaVeNingunDepartamento` comprueba las dos etapas de arriba y
  que el prompt y el candado digan lo mismo.
- `TestNingunDepartamentoVeTablasHuerfanas` **avisa en el log** cuando hay tablas
  de la whitelist sin departamento de negocio. No falla, porque mientras se
  trabaja una tabla puede estar a medio asignar; pero avisa.

Ese segundo test ya encontró un hueco real: `voucher_no_setting` (numeración de
vouchers) estaba en la whitelist sin departamento, y la veía solo IT. Corregido a
contabilidad. Hoy quedan **0 huérfanas**.

---

## 3. Números medidos

Sobre la base migrada de una empresa real (**131 tablas, 1315 columnas**):

| Departamento | Tablas que ve | Tamaño del JSON |
|---|---|---|
| `it` | 75 | 26.760 bytes |
| `fac` | 19 | 8.691 |
| `contb` | 22 | 8.683 |
| `inv` | 19 | 8.485 |
| `rrhh` | 23 | 6.138 |
| `cxc` | 11 | 5.510 |
| `cxp` | 9 | 3.531 |

Antes, todo agente recibía el mismo mapa de texto de ~43 KB. Un agente de CXC
pasa a recibir 5,5 KB: **8 veces menos**, y con menos cosas que no le tocan.

---

## 4. Bugs reales encontrados (esta es la parte que importa)

Aparecieron al ejecutar el agente contra el motor de inferencia. Cada uno tiene
un test que lo fija.

### 4.1 El scopado de "IT" devolvía las 131 tablas

`aiDeptEsVisible("it", tabla)` devolvía `true` siempre. El mapa de soporte —y el
de cualquier agente— incluía `user_login`, `users`, `ai_providers` y `ai_agent`.
La whitelist existed pero no se aplicaba en el scopado de IT.

**Arreglado** aplicando la whitelist en el punto de lectura.

### 4.2 RRHH no veía ninguna tabla

El mapa de `rrhh` salía con **0 tablas, 67 bytes de JSON**. Los nombres de la
tabla estaban inventados: `hr_salary`, `hr_employee_type`, `hr_pay_scale`, que no
existen. Los reales son `hr_salary_assignment`, `person_information`,
`designation`.

Es exactamente el fallo que este trabajo vino a arreglar, cometido por el lado
que no se había automaticado: la lista escrita a mano mentía sobre el esquema.
Ahora los nombres se comprobaron contra `information_schema` y se añadieron las
tablas de los módulos nuevos (`product_lot`, `cycle_count`, `reorder_policy`,
`exchange_rate_daily`, `expiry_alert_log`).

### 4.3 El JSON era inválido

Tres veces seguidas, por tres motivos distintos, y todos del mismo tipo —armar el
JSON a mano en vez de serializarlo:

- `"columnas":[...]` abriendo un **array** y cerrándolo con pares `clave:valor`.
- Las claves foráneas escritas como pares sueltos en el objeto de la tabla, sin
  su envoltorio `"fk"`.
- El campo `"join"` escrito **fuera** del objeto, después del cierre de llave.

El síntoma es silencioso: el modelo recibe texto roto y escribe mal las
consultas sin decir nada. Hay un test que solo comprueba que el JSON parsea.

### 4.4 `describe_schema` devolvía SQL en vez de columnas

La herramienta ponía en el campo `columns` el **texto de la consulta** que lista
las columnas. El agente pedía ver `invoice` y recibía:

```
"columns": "SELECT CONCAT(column_name, ' ', LOWER(column_type))
            FROM information_schema.columns WHERE ..."
```

Medido en vivo (paso 2 del bucle). Peor que no devolver nada: parece información
y no lo es. Ahora sale del introspector.

### 4.5 El parámetro `name` de las herramientas se perdía

`aiArgumentosDeHerramienta` hacía `delete(plano, "name")` sobre el payload,
asumiendo que ahí venía el nombre de la herramienta. `report.save_template` tiene
un parámetro llamado `name`.

Medido: el agente consultaba la base **bien** (su parámetro es `sql`, que no
colisiona) y al guardar la plantilla recibía tres veces seguidas *"Nombre y
definition son obligatorios"*, con los argumentos perfectamente presentes.

El arreglo compara el valor con el nombre registrado (normalizando puntos y
guiones bajos) y solo entonces lo quita.

### 4.6 Un objeto JSON donde se esperaba texto

El esquema de la herramienta dice "JSON de definición: {...}". Buena parte de los
modelos manda un **objeto**, no su texto. `aiStrParam` solo aceptaba cadenas, así
que la definición llegaba y se descartaba.

### 4.7 Los tests que definían su propio DDL morían antes de probar nada

`NuevaBaseDePrueba` clona una plantilla ya migrada. Los tests de IA, `ai_usage`,
CONASEV y sesiones definen su propio DDL mínimo encima, y su primer
`CREATE TABLE` fallaba con *Error 1050*.

Existía ya `NuevaBaseDePrueba` para esquema completo y faltaba la otra mitad:
[testdb.go](../infrastructure/persistence/testdb.go) ahora tiene
**`NuevaBaseVaciaDePrueba`**, que devuelve una base con cero tablas.

### 4.8 La plantilla de tests se reconstruía en cada corrida (165 MB)

El nombre de la plantilla era `erpgo_plantilla_<huella>_<pid>_<unix>`. Como el
reloj iba en el nombre, `baseConEsquema` **nunca** encontraba una plantilla
existente: buscaba por un nombre que acababa de inventar. Cada corrida
reconstruía la base desde cero y dejaba la anterior atrás.

En el servidor de desarrollo: **46 plantillas de 3,6 MB, 165 MB**. El reloj se
saca del nombre y la antigüedad se lee de `information_schema.schemata.create_time`,
que es donde está de verdad.

---

## 5. La prueba de fuego

[ai_cartera_ia_test.go](../interfaces/web/ai_cartera_ia_test.go) monta una
empresa completa y le pide a la IA, por el camino real del producto
(`aiRunLoop`, con herramientas y reintentos):

> *"Consulta los clientes que deben dinero en los últimos 60 días. Para cada uno,
> el nombre y el total pendiente. Después crea la plantilla del informe."*

Resultado de la corrida, verbatim del log:

```
cartera real: 11 facturas por 3403.90 desde 2026-08-05
paso 1  [tool/db_query]           → filas reales de invoice
paso 3  [tool/db_query]           → SELECT date FROM invoice → 10 fechas
paso 8  [tool/db_query]           → SELECT CURDATE() ... → 2026-10-04 / 2026-08-05
paso 9  [tool/db_query]           → cartera agrupada por cliente
paso 10 [tool/db_query]           → 7 facturas con saldo
paso 11 [tool/report_save_template] → {"name":"Cartera 60 días","status":"ok","template_id":1}
paso 12 [model/]                  → informe final en Markdown
el agente hizo 9 consultas, todas dentro de su departamento
PASS
```

Y comprueba tres cosas, no una:

1. Que **ninguna** de las 9 consultas fue rechazada por scopado.
2. Que la plantilla guardada pasa el guardián de definiciones.
3. Que el SQL de la plantilla solo toca tablas de CXC.

### El negativo, que es el que prueba el diseño

```
TestElAgenteDeCXCNoPuedeVerLaNomina   →  el guardián contesta
                                          "tu rol no tiene acceso a hr_employee"
TestAgenteRRHHSiVeLaNomina            →  sí la ve
```

Si el scoping estuviera solo en el prompt, el modelo contestaría "no tengo
acceso" **por educación** y el usuario creería que hay una política. Con el
scoping en el guardián, la respuesta la pone el código y es la misma venga el
modelo que venga.

---

## 6. Qué falta (y es lo siguiente)

Esto es la base sobre la que se construye el resto del plan:

- [ ] **Alta de agente IA como usuario**: elegir tipo de agente, rol y
  departamento al dar de alta un usuario, igual que se elige un humano. Hoy el
  agente se crea aparte y el usuario se referencia por `user_id`.
- [ ] **Coste por tokens como partida contable**: `ai_usage` ya mide tokens y
  coste por agente y periodo; falta el asiento que lo lleva a gastos.
- [ ] **Configuración de agente por módulo**: un prompt de configuración por
  departamento, que se antepone al prompt del rol.
- [ ] **Vistas de solo lectura**: el introspector ya las marca (`es_vista`) y las
  trata como tablas; falta decidir cuáles se exponen.
- [ ] **Exportación del mapa**: dar al administrador el JSON completo descargable.

---

## 7. Cómo se integra un módulo NUEVO en la IA

Este apartado existe para que la próxima persona que cree un módulo no tenga que
averiguarlo, y sobre todo para que no se repita el fallo de RRHH.

### 7.1 Los siete pasos

Al crear un módulo hay que tocar **cinco ficheros de código y una migración**.
Los cinco son obligatorios: si se olvida uno, el módulo funciona para los humanos
y es invisible (o invisible a medias) para la IA.

| # | Dónde | Qué se hace | Qué pasa si se olvida |
|---|---|---|---|
| 1 | `migrations/schema_z_<modulo>.sql` | `CREATE TABLE` con `utf8mb4_unicode_ci` | No existe el módulo |
| 2 | `interfaces/web/<modulo>.go` | Servicio + `registerXRoutes()` | Sin HTTP ni lógica |
| 3 | `interfaces/web/server.go` | Llamar a `s.registerXRoutes()` en `NewServer` | El módulo compila y no aparece: la función nunca se llama |
| 4 | `templates/<modulo>.html` | Pantalla | Error al renderizar |
| 5 | **`aiCatalogWhitelist`** | Una línea: tabla + descripción de negocio | **La IA no ve la tabla.** Ni siquiera IT. Es el paso que se olvidó con RRHH |
| 6 | **`aiDeptDeTabla`** | Qué departamentos la usan | **Solo IT la ve.** Si se olvida el paso 5, tampoco esto |

Los pasos 5 y 6 son los de IA, y son los que no existen en ningún tutorial
porque no hacen falta para que el módulo funcione.

### 7.2 Ejemplo real: qué se tocó al crear el conteo cíclico

```
migrations/schema_z_inventory_advanced.sql   →  cycle_count, cycle_count_line,
                                               cycle_count_adjustment
interfaces/web/cycle_count.go                →  30.857 bytes, registerCycleCountRoutes()
interfaces/web/server.go:316                 →  s.registerCycleCountRoutes()
templates/inventory_cycle_count.html         →  12.052 bytes
aiCatalogWhitelist                           →  3 tablas con su descripción
aiDeptDeTabla                                →  {DeptInv}
```

### 7.3 El menu también se olvidó (y sigue pendiente)

Los cuatro módulos nuevos —conteo cíclico, FEFO, punto de pedido, tipo de
cambio— **no tienen entrada en `sub_module`**. Se llega por URL, no por menú.

Verificado en la base, no solo leyendo el código: en `erpgo_default.sub_module`
están `ai_agents`, `ai_settings`, `ai_tasks`, `ai_business_analyst_report` y
`exchange_rate`; no están `inventory_cycle_count`, `inventory_reorder`,
`inventory_expiry`, `exchange_rate_paralelo` ni `tasa_diferencia`.

Los módulos de IA sí la insertan (`schema_z_ai_agents.sql`, con
`INSERT ... SELECT ... WHERE NOT EXISTS`, que además es idempotente). Los de
inventario no la insertan, y no está investigado por qué.

Un módulo sin menú no está terminado: el usuario no sabe que existe. La
inserción es idempotente, así que se puede añadir a
`schema_z_inventory_advanced.sql` sin riesgo de duplicar en bases ya migradas
(el `WHERE NOT EXISTS` lo cubre), y el `directory` debe coincidir con el nombre
de la plantilla que el handler renderiza (`inventory_cycle_count.html` →
`inventory_cycle_count`).

### 7.4 Cómo comprobar que quedó bien

```bash
# 1. Compila y pasa vet
GOFLAGS=-p=1 go build ./... && GOFLAGS=-p=1 go vet ./...

# 2. La tabla existe de verdad y la ve el departamento correcto
#    (el test de huérfanas avisa en el log si se olvidó el paso 6)
GOFLAGS=-p=1 go test ./interfaces/web/ -run TestNingunDepartamentoVeTablasHuerfanas -v

# 3. La IA la ve: debe salir en el mapa del departamento
GOFLAGS=-p=1 go test ./interfaces/web/ -run TestMapaPorRolVieneRecortado -v
```

Si el paso 3 falla con "no debe incluir", es que la tabla está en la whitelist
pero el scopado no la deja pasar, o al revés.

### 7.5 Las tres trampas, con su síntoma exacto

1. **Nombres de tabla inventados.** Escribir `hr_salary` cuando la tabla se
   llama `hr_salary_assignment`. Síntoma: el departamento ve **0 tablas** y el
   JSON pesa 67 bytes. No lanza error: es peor.
2. **Poner la tabla en la whitelist y olvidar el departamento.** Síntoma: el
   agente del área dice que no existe. `TestNingunDepartamentoVeTablasHuerfanas`
   lo avisa.
3. **Asignar un solo departamento donde hacen falta varios.** `invoice` la
   necesitan facturación, cobranza, contabilidad e inventario. Si solo la ve
   contabilidad, el agente de CXC no puede informar de la cartera.

```
GOFLAGS=-p=1 go build ./...                      limpio
GOFLAGS=-p=1 go vet ./interfaces/web/ ./infrastructure/...   limpio
gofmt -l .                                        sin diferencias
```

Tests de la etapa, todos en verde:

```
TestIntrospectorLeeMySQLReal              el JSON coincide con information_schema
TestIntrospectorIgnoraTablasDeSeguridad  ni user_login ni ai_agent salen
TestIntrospectorReflejaColumnaNueva       una migración aparece sin tocar código
TestEsquemaJSONEsJSONValido               el JSON parsea
TestScopePorDepartamentoEsRecortado       CXC no ve contabilidad
TestMapaPorRolVieneRecortado              el prompt va recortado
TestMapaEsMasGrandeParaIT                 el scopado recorta de verdad
TestGuardianScopeBloqueaTablaAjena        el candado funciona
TestGuardianScopeNoSeEngañaConTextos      un nombre en un literal no cuenta
TestCacheEsquemaSeReutiliza               sin cambios, la caché sirve
TestInvalidarCacheFuerzaRelectura         la invalidación funciona
TestAgenteCXCNoPuedeLeerContabilidad      por el camino del producto
TestAgenteCXCNoPuedeVerElMapaEntero       describe_schema tampoco es puerta trasera
TestAgenteITSiVeTodoLoDeclarado           soporte ve la base entera
TestAgenteITTampocoVeCredenciales         ser IT no da acceso a claves
TestDescribeSchemaDevuelveColumnasNoSQL   bug 4.4
TestElParametroNameNoSeCome               bug 4.5
TestStrParamAceptaObjetosJSON             bug 4.6
TestElAgenteDeCXCInformaLaCartera         extremo a extremo con IA real
```

La IA que se usó es la que hay en el `.env`. Solo los dos vendors compilados en
el binario (`openai` y `orc24`); el vendor es `orc24` y lo único que se puede
mover por entorno es el **servidor** (`AI_VENDOR_ORC24_BASE_URL`), porque el
propio todavía no está en pie. El día que esté, se quita la variable y el
sistema habla con ORC24 sin tocar el binario.