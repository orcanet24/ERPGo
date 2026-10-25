# El patrón único de desarrollo de un módulo del ERPGo

**Estado:** normativo. Si un módulo no sigue estos pasos, no está terminado.
**Fecha:** 2026-10-04.
**Ámbito:** cualquier módulo nuevo o ampliación de uno existente.

---

## 0. Por qué este documento existe

Se implementaron 4 empresas de simulación, 13 tablas de costos, un motor de
costeo completo y varios módulos de IA **sin un patrón escrito**. El resultado
fue predecible y caro:

| Lo que pasó | Cuánto costó |
|---|---|
| 38 bases de prueba basura acumuladas (139 MB) | Limpieza manual |
| 6 bugs de aritmética en un solo módulo de costos | Reescritura del cálculo |
| Un módulo entero invisible para la IA (13 tablas sin registrar) | Descubierto por un agente que "alucinaba" |
| `sub_module` sin entradas: el módulo se llegaba por URL, no por menú | Usuario sin descubrir la función |
| Un motor de costos con 0 rutas HTTP | Función terminada e inusable |

Ninguno de esos fallos era de diseño. Eran de proceso: no había un checklist,
así que cada paso obligatorio dependía de que alguien lo recordara. Este
documento es ese checklist, y sus reglas son **bugs concretos convertidos en
norma**.

La regla de oro: **cada paso de este documento existe porque su ausencia ya
produjo un fallo real y verificable.** Si un paso no se puede cumplir, se
documenta por qué; no se salta en silencio.

---

## 1. Los 9 pasos obligatorios

Un módulo toca **8 ficheros** y **1 registro en 2 mapas**. Todos son
obligatorios.

| # | Fichero | Qué se hace | Qué pasa si se omite |
|---|---|---|---|
| 1 | `migrations/schema_z_<modulo>.sql` | `CREATE TABLE IF NOT EXISTS`, `utf8mb4_unicode_ci`, `decimal` para dinero | El módulo no existe |
| 2 | `interfaces/web/<modulo>.go` | Servicio + `register<Modulo>Routes()` | Sin HTTP ni lógica |
| 3 | `interfaces/web/server.go` | **Llamar** a `s.register<Modulo>Routes()` dentro de `NewServer` | Compila, no aparece: la función nunca se invoca |
| 4 | `templates/<modulo>.html` | `{{define "content"}}` + `{{define "pagejs"}}` | HTTP 500 al renderizar |
| 5 | `migrations/schema_z_<modulo>.sql` | Filas en `menu` (label, url, icon, perm) para el sidebar | Módulo invisible en el menú |
| 6 | `migrations/schema_z_<modulo>.sql` | `INSERT IGNORE INTO sub_module` | Se llega por URL y nadie lo sabe |
| 7 | `interfaces/web/ai_schema_catalog.go` | Línea en `aiCatalogWhitelist`: tabla + descripción de negocio | **La IA no ve la tabla. Ni IT.** |
| 8 | `interfaces/web/ai_schema_introspect.go` | Entrada en `aiDeptDeTabla` con sus departamentos | **Solo IT la ve** |
| 9 | `interfaces/web/<modulo>_test.go` | Tests puros de aritmética + test de integración | El bug aparece en producción |

### 1.1 Los pasos 3, 6, 7 y 8 no hacen falta para que "funcione"

Ese es exactamente el punto. Los cuatro pasos que se olvidan son los que no
fallan: el módulo queda silenciosamente incompleto. Un módulo sin paso 7 no
lanza error, la tabla simplemente no existe para el agente, que concluye que no
está creada. Un módulo sin paso 6 compila, pasa todos los tests y es
inalcanzable para el usuario.

**Regla: si algo no falla, sigue siendo obligatorio.**

---

## 2. Orden de ejecución

El orden no es arbitrario. La secuencia es:

1. **Primero el SQL de la migración** (paso 1). Sin tabla no hay nada, y
   escribir el cálculo antes de fijar las columnas obliga a redimensionar todo.
2. **Después el núcleo puro** (cálculo, sin BD) y sus tests. Es la única forma
   de encontrar bugs de aritmética en segundos y no en minutos.
3. **Luego la capa de datos** (lecturas y escrituras sobre la tabla ya creada).
4. **Luego las rutas** (pasos 3 y 4).
5. **Luego la visibilidad**: menú (pasos 5 y 6), IA (pasos 7 y 8).
6. **Al final el test de integración** (paso 9), que necesita todo lo anterior.

La regla que explica el paso 2: **todo cálculo con dinero va en una función pura
que no toca la base.** No es estética. Cada test que levanta MySQL tarda de 30
a 500 segundos; uno puro tarda milisegundos. Los bugs del módulo de costos
fueron cuatro de aritmética, y se encontraron porque había tests puros. Con la
base en medio, esos cuatro tests habrían costado horas y probablemente no se
habrían escrito.

```go
// interfaces/web/<modulo>.go — la forma correcta
type costInputs struct { /* solo datos */ }

func calcularCosto(in costInputs) CostBreakdown {
    // PURO: no lee la base, no lee el reloj, no depende de Server.
}

func (s *Server) calcularCostoDeProducto(ctx, id) (CostBreakdown, error) {
    in, err := s.cargarCostInputs(ctx, id) // lee
    if err != nil { return CostBreakdown{}, err }
    return calcularCosto(in), nil            // calcula
}
```

---

## 3. Reglas que nacen de bugs reales

Estas reglas no son estilo. Cada una tiene el fallo que la produjo.

### 3.1 El dinero es `decimal`, nunca `DOUBLE` ni `float` en la migración

- **Bug:** `DOUBLE` para costos unitarios, que acumula error de redondeo en cada
  suma del kardex. Un costo que no cuadra al céntimo es un costo que nadie firma.
- **Regla:** importes de cabecera `decimal(18,2)`; costos unitarios
  `decimal(18,4)` (es lo que ya usa `product_information.cost`).
- En Go el transporte es `float64`, pero **la suma se redondea con `round2()`**
  ([fiscal.go:289](../interfaces/web/fiscal.go)) antes de escribir. No se escribe
  un `float64` crudo en una columna de dinero.

### 3.2 `DECIMAL` de MySQL llega como `[]byte`, no como `float64`

- **Bug:** un informe de IA sumaba solo `float64`; ningún total de dinero se
  calculó jamás. Las filas salían y las cifras no, que es el peor resultado
  posible porque parece un informe completo.
- **Regla:** todo valor numérico que venga de la base pasa por `valorNumerico()`
  ([ai_reports.go:210](../interfaces/web/ai_reports.go)) o `toF()`
  ([cashsession.go:265](../interfaces/web/cashsession.go)) antes de operarse.
- **Síntoma si se olvida:** la pantalla funciona, las filas se ven, y todas las
  cifras son cero o vacías.

### 3.3 No se acumulan totales por dos claves a la vez

- **Bug:** el motor de informes acumulaba por el alias del `SELECT` **y** por la
  clave declarada. Cuando coincidían (lo normal) la columna se sumaba dos veces
  y un total de 775,90 salía como 1.551,80. Sin ningún aviso.
- **Regla:** si se indexa por dos nombres posibles, **solo se suma si son
  distintos.** Un total que cuadra al doble es peor que uno que no cuadra,
  porque parece un dato bueno.
- Y si un total pedido **no se pudo calcular**, se emite un aviso explícito en
  lugar de devolver la pantalla limpia.

### 3.4 Toda columna generada es una columna que se puede recalcular mal

- **Bug:** `hourly_rate` como columna `GENERATED` a partir del salario: al subir
  el salario, **reescribía el costo de horas ya facturadas y ya cerrada**.
- **Regla:** lo que se usó para producir un documento congelado se **copia a una
  columna normal** (`rate_used`) el día del cálculo. Un histórico que se
  recalcula no es un histórico.

### 3.5 Se aplica sobre la cantidad REAL, no sobre la pedida

- **Bug:** el reparto de aduana dividía el costo de importación entre las
  unidades **pedidas** y no entre las **recibidas**. En un envío con faltantes el
  costo por unidad salía inflado, y el error se congelaba en el snapshot.
- **Regla:** cualquier reparto por cantidad usa el denominador **efectivo**
  (`ReceivedQty`), y el motor guarda en la traza qué denominador usó.
- **Corolario:** lo que se aplicó a un campo que nunca se calcula es un bug
  silencioso. Si un campo no existe o no se puebla, la regla no se aplica — no se
  aplica "a ver qué pasa".

### 3.6 El rendimiento se aplica a la entrada, no a la salida

- **Bug:** el cálculo de rendimiento de despiece leía el costo del **producto de
  salida** en vez del de **entrada**, y no aplicaba el porcentaje de rendimiento.
- **Regla:** el rendimiento se calcula como `CostoEntradaTotal / (Cantidad ×
  RendimientoPct)`, leyendo siempre `input_product_id`.

### 3.7 Una tabla sin permiso no existe; una tabla sin rol no se usa

Ver sección 1, pasos 7 y 8.

### 3.8 `sqlShowTables` filtra por `DATABASE()`

- **Bug potential:** `information_schema` sin filtro de base devuelve las tablas
  de **todas** las empresas del servidor, y la IA ve el esquema de otras.
- **Regla:** toda consulta a `information_schema` que liste tablas lleva
  `WHERE table_schema = DATABASE()`. Ver [dialect.go:40](../interfaces/web/dialect.go).

### 3.9 El dialecto se declara al modelo, no se improvisa

- **Bug:** el motor de informes generaba SQL con `strftime`, que es SQLite. En
  MySQL el informe salía con fecha vacía.
- **Regla:** el SQL de este proyecto es **MySQL** y se le dice al modelo con
  `aiDialectMySQL` ([ai_rules.go:39](../interfaces/web/ai_rules.go)). No se escribe
  una función de SQLite "por si acaso".

### 3.10 El color vive en la capa del tema

- **Regla:** las plantillas referencian **clases**, nunca colores. Un `style="color:#fff"`
  en el HTML es una pantalla que en tema oscuro se pinta sola y que nadie puede
  arreglar desde el CSS. Lo vigilan dos tests automáticos
  ([templates_test.go](../interfaces/web/templates_test.go)).

### 3.11 La etiqueta IT no se duplica

- **Bug:** el mismo nombre de departamento ("IT") producía dos entradas en el
  mapa por rol, y el prompt salía con el mismo bloque repetido.
- **Regla:** los departamentos son constantes (`DeptIT`, `DeptContab`, …). Se
  comparan por constante, nunca por cadena literal.

---

## 4. Reglas de la capa web

### 4.1 El handler es delgado

El handler **valida, delega y traduce**. Toda la regla de negocio vive en el
servicio, que es donde se prueba sin levantar HTTP. Referencia:
[crm_handlers.go](../interfaces/web/crm_handlers.go) (handlers) contra
[crm_service.go](../interfaces/web/crm_service.go) (lógica).

### 4.2 Las rutas usan el método HTTP en la línea

```go
s.mux.HandleFunc("GET /crm/contact/list", s.crmContactList)
s.mux.HandleFunc("POST /crm/contact/save", s.crmContactSave)
```

No `"GET ..."` con condicional dentro, ni rutas registradas con un método que no
corresponde a lo que hace el handler.

### 4.3 La empresa se resuelve por petición

```go
db := s.dbFor(r)   // NUNCA s.db
```

`dbFor` ([tenant_request.go:82](../interfaces/web/tenant_request.go)) resuelve la
base de la empresa de la sesión. Usar `s.db` escribe en la base de la empresa
activa, que puede no ser la de la petición. En un ERP multiempresa eso es
corrupción cruzada entre clientes.

### 4.4 Toda escritura pasa por CSRF

`s.enforceCSRF` se aplica en los `POST`. Un endpoint de escritura sin CSRF es un
agujero, y un ERP con datos fiscales detrás necesita esa capa
([csrf.go](../interfaces/web/csrf.go)).

### 4.5 Respuestas JSON con los helpers existentes

`writeJSON(w, v)` y `jsonError(w, msg, code)`. Nunca `fmt.Fprintf` con JSON a
mano: el formato de error es parte del contrato con el JavaScript.

### 4.6 El módulo se registra en `server.go` **y** en el menú

Los dos. Compilar no es que esté conectado.

---

## 5. Reglas de plantilla

- Toda pantalla dentro del layout define **`{{define "content"}}`** y
  **`{{define "pagejs"}}`**. Las 110 pantallas actuales tienen los dos; faltar
  uno de los dos es un HTTP 500 que ningún test de sintaxis detecta.
- Los documentos autónomos (libros legales, notas de crédito, kardex) **no**
  llevan layout: se abren solos para imprimir, y su color es parte del documento
  que firman. Están listados en `documentosAutonomos`
  ([templates_test.go](../interfaces/web/templates_test.go)).
- Los totales se imprimen con `printf "%.2f"` y el símbolo de moneda de
  `{{.Currency}}`.
- El `directory` de `sub_module` **debe coincidir** con el nombre de la
  plantilla que el handler renderiza: `inventory_cycle_count.html` →
  `inventory_cycle_count`.

---

## 6. Checklist de salida

Un módulo no está terminado hasta que **todas** estas líneas dan el resultado
esperado. Los comandos se ejecutan desde la raíz del proyecto y con el `.env`
cargado (`set -a && . ./.env && set +a`).

```bash
# 1. Compila y no tiene problemas estáticos
go build ./...            # sin salida
go vet ./interfaces/web/ # sin salida

# 2. Formato
gofmt -l interfaces/ migrations/ 2>/dev/null   # sin salida

# 3. La migración es idempotente: se aplica DOS veces y las dos salen con 0
#    (ver la sección 7)

# 4. Las tablas nuevas existen de verdad
mysql ... -e "SELECT TABLE_NAME FROM information_schema.TABLES
  WHERE TABLE_SCHEMA='erpgo_default' AND TABLE_NAME LIKE '<prefijo>%';"
# debe listar todas

# 5. Ninguna tabla quedó huérfana para la IA  (paso 8)
go test ./interfaces/web/ -run TestNingunDepartamentoVeTablasHuerfanas -v -count=1

# 6. La IA ve la tabla en el departamento correcto  (pasos 7 y 8)
go test ./interfaces/web/ -run TestMapaPorRolVieneRecortado -v -count=1

# 7. Todas las plantillas pintan dentro del layout, en claro y en oscuro
go test ./interfaces/web/ -run TestEveryTemplateRendersWithLayout -v -count=1
go test ./interfaces/web/ -run TestCadaPaginaSePintaEnTemaOscuro -v -count=1

# 8. Ninguna plantilla escribe color en el HTML
go test ./interfaces/web/ -run TestNingunaPlantillaEscribeColorEnLinea -v -count=1
```

> **Advertencia sobre pipelines.** `cmd | grep -v x; echo $?` devuelve el estado
> de `grep`, no el de `cmd`. Un test que falla puede aparecer como verde.
> Siempre `cmd | head; echo ${PIPESTATUS[0]}` o `cmd >/dev/null 2>&1; echo $?`.

### 6.1 Sobre los tiempos de prueba

La suite con base de datos tarda de 30 a 500 segundos **por grupo**: cada test
con base crea una BD MySQL nueva. La suite completa no cabe en una sola
ejecución de 600 segundos.

- Se ejecuta **por bloques** (`-run` con el nombre del módulo).
- Se prefiere un **test puro** por cada regla de aritmética: milisegundos.
- Si un test necesita credenciales de IA y no las hay, **se salta y lo dice**
  (`t.Skip`). Un `go test` sin claves debe saltar, no fallar.
- Las bases de prueba se limpian con `BorrarBasesConPrefijo` /
  `BorrarBasesDePruebaHuerfanas` ([testdb.go](../infrastructure/persistence/testdb.go)).
  **`t.Cleanup` siempre**, o la próxima corrida encuentra 38 bases basura otra vez.

---

## 7. La migración tiene que ser idempotente, y hay que probarlo

`transformStatement` ([database.go:393](../infrastructure/persistence/database.go))
ya convierte `CREATE TABLE` en `CREATE TABLE IF NOT EXISTS` y `INSERT INTO` en
`INSERT IGNORE INTO` automáticamente. Aun así:

- Se escribe **`CREATE TABLE IF NOT EXISTS`** explícitamente en el fichero. Que
  el sistema lo arregle no significa que el fichero sea legible por sí solo.
- Los `INSERT` de permisos y catálogos usan **`INSERT IGNORE`** con clave única,
  o `INSERT ... SELECT ... WHERE NOT EXISTS`.
- **La prueba de la idempotencia no es opcional:** aplicar la migración dos veces
  contra `erpgo_default` y comprobar que **las dos** terminan con código de
  salida 0. Es lo que se hizo con `schema_z_costos.sql` y es la única forma de
  saber que una base ya migrada no revienta al arrancar.

---

## 8. La plantilla reutilizable

Los esqueletos copiables están en [doc/plantillas/modulo/](plantillas/modulo/):

```
doc/plantillas/modulo/
├── README.md                     qué copiar y en qué orden
├── schema.sql                    migración
├── servicio.go.txt               lógica pura + acceso a datos
├── handlers.go.txt               rutas HTTP delgadas
├── plantilla.html                pantalla
├── menu.sql                      filas de `menu` para el sidebar (NO se edita layout.html)
├── ai_catalog.txt                las dos líneas de la IA
└── modulo_test.go.txt            tests puros + test de integración
```

Los `.go.txt` llevan esa extensión a propósito: dentro de `doc/` un `.go` real
lo compilaría `go build ./...` y rompería el build del proyecto entero, porque
los marcadores `<Modulo>` no son Go válido. Se renombran al copiar.

**Cómo se usa:** se copia el contenido de cada fichero, se sustituyen los tres
marcadores (`<MODULO>` minúscula para tablas y rutas, `<Modulo>` mayúscula para
tipos y funciones, `modulo` sin ángulos para los handlers no exportados) y se
avanza en el orden de la sección 2.

### 8.1 La plantilla está verificada, y el HOW se encontró verificándola

No es un esqueleto escrito a ojo. Las tres plantillas de Go se sustituyeron por
un módulo concreto (`demomod`), se compilaron **dentro del paquete `web`** con
`go vet`, y sus tests se ejecutaron de verdad. Eso salió así:

| Lo que salió mal al compilar | Por qué |
|---|---|
| `undefined: testing` | A la plantilla de test le faltaba el bloque `import` |
| `mismatched types float64 and *float64` | `valorNumerico` devuelve `*float64` y el ejemplo lo sumaba como `float64` |
| `undefined: strings` | El handler usaba `strings.TrimSpace` sin importarlo |
| `out.Quantity undefined` | El handler escribía un campo que el desglose no expone |
| `declared and not used: s` | El test contra la base recibía el servidor sin usarlo |

Y al ejecutar los tests, uno más, que es el bueno:

| Fallo | Qué demostraba |
|---|---|
| El test del reparto esperaba 402,50 y salía 520,00 | **El cálculo de ejemplo repartía por lo pedido, no por lo recibido.** La plantilla de cálculo contradecía su propia regla 3.5. Se corrigió la plantilla, no el test |

Un esqueleto que no se compila es peor que no tener esqueleto: da la sensación de
que el módulo está a medio hacer cuando en realidad nadie lo ha comprobado
nunca. **La plantilla se verifica sustituyéndola y compilándola**, y esa
verificación es parte del entregable.

Lo mismo con el SQL: `schema.sql` se aplicó dos veces contra una base desechable
y las dos veces salieron con 0, con las tablas creadas y sin errores.

Y el test del paso 3 se probó en los dos sentidos: con la llamada a
`register...Routes()` ausente de `server.go` **falla** con el mensaje que explica
el problema, y al añadirla **pasa**. Un test que solo se ha visto pasar no
demonstra que detecte lo que dice detectar.

---

## 9. Lo que este patrón NO cubre

Ser honesto sobre los límites vale más que un checklist bonito:

- **Los datos que no existen no se inventan.** El módulo de costos sabe calcular
  rendimiento, pero sin pesar la canal el resultado es una suposición
  matemáticamente correcta y contablemente falsa. El patrón garantiza que la
  fórmula sea correcta; no puede fabricar el rendimiento real.
- **No hay control de versiones.** El repositorio no es un repositorio git. Un
  patrón que dice "haz commit" aquí sería fiction.
- **La suite completa no cabe en una ejecución.** Está pensada para ejecutarse
  por bloques, y eso significa que un "todo verde" exige disciplina de quien
  la ejecuta, no un único comando.
- **Un módulo sin cargar no prueba nada.** Las cuatro empresas de `doc/test/empresas/`
  son planes escritos, con cero clientes y cero facturas. El patrón no convierte
  un plan en una simulación.

---

## 10. Referencias

- [MAPA-DINAMICO-BD-PARA-LA-IA.md](MAPA-DINAMICO-BD-PARA-LA-IA.md) — el mapa de
  la base para la IA y los 7 pasos de integración desde el punto de vista de la
  IA. Este documento lo complementa con el resto del módulo.
- [Plan de Sistema de Costos Avanzado.md](Plan%20de%20Sistema%20de%20Costos%20Avanzado.md)
  — el módulo que sirvió de banco de pruebas del patrón: 4 bugs de aritmética y
  13 tablas sin registrar.
- [problemas-conocidos.md](problemas-conocidos.md) — fallos abiertos.