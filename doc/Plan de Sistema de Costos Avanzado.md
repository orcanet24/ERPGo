# Plan de Sistema de Costos Avanzado

**Fecha**: 2026-10-04 (revisado)
**Relacionado**: [Tertulia #1 - Arquitectura Hexagonal](2026-10-04-arquitectura-hexagonal-mysql.md)

> **Historial de la revisión.**
>
> El borrador original tenía un diagnóstico bueno —el ejemplo de la panadería explica bien por
> qué el costo promedio ponderado no sirve— pero su código de muestra no calculaba el coste que
> prometía y usaba tipos de dato que se apartan de lo que el ERP ya hace. Esta versión corrige
> eso, y añade las cuatro empresas concretas que van a usar el módulo.
>
> Los cambios se enumeran en [Fase 0](#fase-0-lo-que-hay-que-arreglar-antes-de-empezar) y
> están justificados allí con el fallo concreto de cada uno.

## Conversación

Se discutió la necesidad de un sistema de costos granular para distintos tipos de negocio. El
punto de partida es que **un costo promedio ponderado (CPP) no sirve para decidir nada**, y la
razón la explica mejor la panadería que cualquier teoría:

- Un maestro pastelero cuesta X por hora, un ayudante cuesta Y por hora
- Si ambos trabajan 2 horas en un pastel, el costo **no** es `2*(X+Y)` ni un promedio
- Depende de: tiempo por rol, productividad, merma, horas no facturables, cargas patronales,
  depreciación del horno, energía, packaging

Cuando un producto se hace con materia prima propia y un proceso de fabricación, **el costo de
adquisición es solo una parte de la respuesta**. El CPP responde "cuánto me costó comprarlo",
que es correcto para un comercio; no responde "cuánto me cuesta hacerlo", que es lo que
necesita quien fabrica.

---

## Fase 0: Lo que hay que arreglar antes de empezar

Cuatro problemas concretos del borrador anterior. No son de estilo: cada uno produce un número
equivocado.

### 0.1 La merma no llegaba al costo final

```go
// Antes (incorrecto)
if c.config.LossFactor > 0 {
    breakdown.Total *= (1 + c.config.LossFactor)   // Total nunca se calcula
}
breakdown.FinalCost = breakdown.Material + breakdown.Labor + ...  // sin merma
```

`breakdown.Total` no se inicializa en ningún sitio: se multiplica por 0 y nadie lo usa. El
`FinalCost` suma los campos sueltos, con lo que **la merma se aplicaba a nada** y el costo final
salía sin ella.

```go
// Ahora: la merma se aplica donde se usa, y se calcula sobre las partes
// que la sufren (material), no sobre el total entero.
breakdown.Material = material * (1 + lossFactor)
breakdown.FinalCost = breakdown.Material + breakdown.Labor +
                      breakdown.Depreciation + breakdown.Overhead + breakdown.Customs
```

La merma es **de material**, no global: si un 3% de la tela se pierde en la corte, eso deforma
material, no las horas del operario ni la depreciación de la máquina.

### 0.2 `customs_cost / qty` no es un costo por unidad

```go
// Antes (incorrecto)
breakdown.Customs = c.config.CustomsCost / qty
```

`customs_cost` es un gasto de importación **total** de la partida. Dividirlo por la cantidad
generada da un número distinto según para cuántas unidades se pidió, y el costo del producto
cambia cuando cambia el pedido, no cuando cambia el mercado.

```go
// Ahora: el costo de importación se reparte entre las unidades que
// realmente recibió esa partida, y queda en la partida, no en el producto.
breakdown.Customs = partida.CustomsCost / partida.ReceivedQty
```

### 0.3 `DOUBLE` para dinero, contra un ERP que usa `decimal(18,2)`

El borrador proponía `DOUBLE` en todas las tablas nuevas. El ERP existente ya usa
`decimal(18,2)` en `invoice`, `product_purchase_details`, `acc_vaucher` y `decimal(18,4)` en
`product_information.cost`.

`DOUBLE` no representa 0.1 con exactitud. En un kardex con miles de entradas y salidas el error
se acumula y la diferencia contra contabilidad no la cuadra nadie. **Se usa `decimal(18,4)` en
costos unitarios y `decimal(18,2)` en importes**, que es lo que ya hace el resto.

### 0.4 `hourly_rate` como columna generada, sin historial

```sql
-- Antes (incorrecto)
hourly_rate DOUBLE GENERATED ALWAYS AS (
    (base_salary_annual * (1 + overhead_pct/100)) / hours_per_year
) STORED
```

Si el salario cambia, la tasa cambia retroactivamente y **el costo de lo ya facturado se
re-escribe solo**. Una empresa que factura y no guarda historial de tarifas pierde la
trazabilidad que el propio documento pide en "Compliance y Auditoría".

```go
// Ahora: la tasa se calcula en Go en el momento de usarla y se GUARDA en el
// asiento de costo. Lo que se cobró en marzo se calculó con la tasa de marzo.
tasa := calcularTasaAnual(salario, cargas, horasAnuales)
guardarComponenteCosto(periodo, producto, "labor", horas*tasa)
```

---

## Cuatro empresas, cuatro métodos de costeo

El módulo tiene que servir para las cuatro o no sirve. Cada una es un tipo de negocio
distinto, y por eso sirven como prueba de que el diseño no está atado a un solo caso:

| # | Empresa | Fabrica | Qué vende | Método de costeo dominante |
|---|---------|---------|-----------|------------------------------|
| 1 | **Servicios de software** | No | Proyectos, apps, webs | Costo por horas de equipo |
| 2 | **Textilera** | Sí | Ropa propia + calzado y textil importado | Receta + mano de obra + importación |
| 3 | **Distribuidora de carne** | Sí (despiece) | Canales de carne, mayor y detalle | Rendimiento de despiece + 2 listas de precio |
| 4 | **Autopartes** | No | Producto terminado importado | Costo de adquisición + importación |

Las tres primeras son las solicitadas; la cuarta entra como comprobación de que un negocio que
solo compra y revende **no necesita ni receta ni horas** y el módulo debe dejarlo en cero sin
obligar a configurar nada.

### 1. Servicios de software

No hay inventario. El costo es **equipo × horas × tarifa**, y la pregunta que hay que poder
responder es: *¿cuánto me costó realmente hacer este proyecto, y qué margen tuve?*

- **Falta**: una hoja de horas por proyecto. `hr_payroll_item` tiene `amount`, `rate_pct` y
  `base_amount`, pero **ninguna columna de horas**. Sin horas trabajadas registradas, no hay
  costo por proyecto que calcular: es el bloqueo más grande del módulo y se ataca primero.
- Conviene separar horas **facturables** de horas **no facturables** (reuniones, soporte,
  formación). Las no facturables son costo real, pero no ingresan; mezclarlas da un margen
  falso.

### 2. Textilera

Compra materia prima para fabricar ropa **y además importa** calzado y textil. Vende al detalle
y al mayor. Tiene dos orígenes de costo distintos:

- **Fabricación**: `product_recipe` con merma por componente + horas de confección
- **Importación**: `landed_cost` (flete, aduana, seguro) repartido por partida

Un producto importado no lleva receta. Un producto fabricado no lleva `landed_cost`. Si el
módulo obliga a los dos, es un error de diseño.

### 3. Distribuidora de carne

Compra canales, las despiece, y vende **al mayor y al detalle**. El punto crítico es el
rendimiento:

- De una canal de 250 kg salen 180 kg de venta y 70 kg de hueso, merma y descarte
- El costo de la canal **no se reparte 250/250** entre los cortes: se reparte sobre los
  kilos que efectivamente se venden
- El precio de venta es **dos números distintos** (mayor y detalle) para el mismo corte

Es el caso que más fácil se hace mal: si el costo sale sin rendimiento, el precio de venta
queda por debajo de lo que costó la canal y la empresa pierde dinero sin saberlo.

### 4. Autopartes

Compra producto terminado, lo importa, no fabrica, y vende al mayor y al detalle. Es el caso
más simple: **costo de adquisición + importación**, sin receta, sin horas, sin rendimiento.

Sirve para comprobar que el módulo no complica lo sencillo.

---

## Fase 1: Entidades del dominio de costeo

### Entidades base

**1. Receta / BOM** (`product_recipe`)

- `product_id`, `component_id`: producto compuesto e ingrediente
- `quantity`, `unit`: cantidad y unidad
- `loss_factor`: merma **por componente** (0.05 = 5%). Por componente, no global, porque una
  tela se pierde en el corte y un botón no.
- `step_order`: orden del proceso

**2. Centro de costo** (`cost_center`)

- `code`, `name`, `type`: producción | admin | logística | almacén
- `overhead_rate_hour`: indirecto por hora
- `overhead_rate_unit`: indirecto por unidad

**3. Ruta de proceso** (`process_route`)

- `product_id`, `sequence`, `work_center_id`
- `setup_time_min`: tiempo de preparación, que **no** se prorratea entre unidades
- `run_time_sec`, `machine_time_sec`: por unidad

**4. Grado de mano de obra** (`labor_grade`)

- `code`, `name`, `base_salary_annual`, `overhead_pct` (cargas patronales)
- `hours_per_year` (2080 = 40 h × 52), `productivity_factor`
- `hourly_rate`: **columna normal**, se calcula en Go al usarla y se guarda en el asiento de
  costo (ver 0.4)

**5. Hoja de horas** (`cost_timesheet`) — *lo que falta y bloquea todo lo demás*

- `employee_id`, `work_date`, `hours`
- `billable`: factura o no
- `project_id`, `product_id`: contra qué se va
- `grade`: el rol que se aplica (maestro / ayudante / supervisor)

**6. Producto de origen animal** (`product_yield`)

- `input_product_id`, `output_product_id`: canal → corte
- `expected_pct`: rendimiento esperado
- Permite repartir el costo de la canal sobre los kilos que salen a venta

**7. Costo de importación** (`import_cost`)

- `purchase_id`, `freight`, `customs`, `insurance`, `other`
- `received_qty`: sobre qué se reparte
- El reparto es por partida, no por producto (ver 0.2)

**8. Listas de precio** (`price_list`, `product_price_list`)

- `price_list`: `mayor`, `detalle`, `web`…
- `product_id`, `price_list_id`, `price`
- Sin esto, al mayor y al detalle es una sola cifra, que es justo lo que la
  carnicería no puede aceptar

**9. Configuración de costeo por producto** (`product_cost_config`)

- `costing_method`: `PP` | `RECIPE` | `YIELD` | `HOURS` | `IMPORT`
- `perishable`, `shelf_life_days`, `is_assembled`, `track_lots`, `imported`

### Cálculo de costo de mano de obra

```go
// CostoHora = (SalarioAnual + CargasPatronales) / HorasAnuales
// CostoOperacion = HorasTrabajadas × CostoHora × Productividad

type LaborCost struct {
    EmployeeID   int64
    Grade        string  // maestro, ayudante, supervisor
    Hours        float64 // horas registradas en la hoja de horas
    HourlyRate   float64 // tasa de ESTE período, guardada
    Productivity float64 // 0.8 = 80% eficiente
    TotalCost    float64 // horas × tasa × productividad
}
```

### Cálculo de depreciación

```go
// DepreciacionMensual = (CostoAdquisicion - ValorResidual) / (VidaUtil × 12)
// Al producto = DepreciacionMensual × HorasUso / HorasTotalesMes

type AssetDepreciation struct {
    AssetID         int64
    AcquisitionCost float64
    ResidualValue   float64
    UsefulLifeYears float64
    MonthlyDeprec   float64
    HoursUsed       float64
    AllocatedCost   float64
}
```

### Costos indirectos

- **Fijos**: por unidad de producción o por hora de centro de costo
- **Variables por proceso**: energía, insumos menores
- **Generales**: prorrateados por horas de producción o unidades

---

## Fase 2: Migración

Las tablas se crean en `migrations/schema_z_costos.sql`. Todo el SQL completo está allí;
este es el resumen de decisiones:

- **Dinero en `decimal`**, nunca `DOUBLE` (ver 0.3)
- **Migración idempotente**, con la misma forma que `schema_zzzz_ai_department.sql`
- **`WHERE NOT EXISTS`** en los sembrados, para no pisar lo que el cliente haya editado

---

## Fase 3: Motor de cálculo

```go
type CostCalculator struct {
    db     *sql.DB
    period string // el período se fija, no se lee del reloj
}

func (c *CostCalculator) Calculate(pid int64, qty int64) (Breakdown, error) {
    cfg := c.config(pid)

    b := Breakdown{}
    switch cfg.Method {
    case MethodHOURS:    // servicios de software
        b.Labor = c.labor(pid, qty)
    case MethodRECIPE:   // textilera: fabricación
        b.Material = c.recipe(pid, qty)
        b.Labor = c.labor(pid, qty)
    case MethodYIELD:    // carnicería: despiece
        b.Material = c.materialFromYield(pid, qty)
    case MethodIMPORT:   // autopartes: importado
        b.Material = c.purchaseCost(pid, qty)
        b.Customs = c.landedCost(pid, qty)
    case MethodPP:       // comercio simple (default)
        b.Material = c.purchaseCost(pid, qty)
    }

    // La merma solo afecta al material (ver 0.1)
    b.Material *= (1 + b.LossFactor)

    b.Depreciation = c.depreciation(pid, qty)
    b.Overhead = c.overhead(pid, qty)
    b.Final = b.Material + b.Labor + b.Depreciation + b.Overhead + b.Customs

    return b, nil
}
```

### Trazabilidad (la parte que no se puede dejar para después)

Cada cálculo deja su **rastro**: qué tasa de mano de obra se usó, qué depreciación, qué horas,
qué período. Sin eso, un costo no se puede defender en una auditoría, y con el RISLR eso deja
de ser teórico.

La trazabilidad va en la **primera** fase, no en "visión de futuro": un costo sin rastro no se
puede reproducir y un costo que no se puede reproducir no es un costo, es una estimación.

### Congelar el costo al cerrar el período

Si recalcular un mes pasado devuelve otro número, el historial no sirve. Al cerrar el período,
el costo de cada producto **se congela** en `cost_snapshot`. Recalcularlo después no cambia lo
que ya se facturó.

---

## Tabla para IA (Prompt Engineering)

El mapa de la BD que recibe el agente se genera de `information_schema`
(`ai_schema_introspect.go`), así que **no hay que escribir esto a mano**. Lo que sí hay que
hacer al crear las tablas del módulo es:

1. **`aiCatalogWhitelist`**: añadir las tablas nuevas. Sin esto, la IA no las ve, **ni siquiera
   IT**.
2. **`aiDeptDeTabla`**: asignar cada una a su departamento. Sin esto, solo IT las ve. Esto es
   el paso que se olvidó con RRHH, que vio 0 tablas durante semanas.

El detalle de los 7 pasos está en
[MAPA-DINAMICO-BD-PARA-LA-IA.md](MAPA-DINAMICO-BD-PARA-LA-IA.md).

---

## Orden de implementación

| Paso | Qué | Por qué ahí |
|---|---|---|
| 0 | `cost_timesheet` (hoja de horas) | Bloquea servicios y toda la mano de obra |
| 1 | Migración de costos | Sin tablas no hay motor |
| 2 | `CostCalculator` con `PP` | Caso simple, verifica que el camino funciona |
| 3 | `RECIPE` + merma por componente | Textilera, fabricación |
| 4 | `YIELD` + rendimiento | Carnicería, el más fácil de hacer mal |
| 5 | `IMPORT` + `landed_cost` | Autopartes y textilera |
| 6 | `price_list` (mayor/detalle) | Los tres que venden a dos precios |
| 7 | `cost_snapshot` + cierre de período | Congela historial antes de usarlo |
| 8 | UI de configuración | Se hace después, no antes |

### Entregables por empresa

Cada empresa tiene su directorio en `doc/test/empresas/<empresa>/` con:

- `00-plan-inicial.md`: qué vende, qué método de costeo, qué tablas necesita
- `01-minutas.md`: decisiones tomadas y por qué
- `02-benchmark.md`: contra qué se compara y qué número hay que alcanzar
- `03-ciclo-economico-5-meses.md`: clientes, proveedores, compras, nómina, inventario

---

## Visión de futuro

### Escenarios soportados

1. **Servicios de software**: horas de equipo, facturables vs no facturables, margen por proyecto
2. **Textilera**: fabricación con receta y merma + importación con aduana
3. **Carnicería**: despiece con rendimiento, mayor y detalle
4. **Autopartes**: compra e importación, sin fabricación
5. **Panadería artesanal**: el escenario original del ejemplo

### Integración con IA

- Proponer rutas de costeo alternativas
- Detectar productos con costos anormales (marca de auditoría)
- Simular impacto de cambiar tarifas salariales o depreciación
- Informes comparativos entre productos

### Compliance y auditoría

- Cada cálculo guarda su trazabilidad: tasa, depreciación, horas, período
- Los precios de venta se calculan con margen real sobre el **precio final**
- Verificación de precios mínimos legales
- Exportación a contabilidad: cada costo se asienta en el libro mayor

---

## Próximos pasos

1. `cost_timesheet` — lo primero, porque sin horas no hay costo laboral
2. Migración `schema_z_costos.sql`
3. `CostCalculator` con el método simple
4. Configurar los productos existentes
5. Migrar el Kardex (CPP) a la nueva arquitectura
6. UI de configuración de recetas y centros de costo
7. Integrar con el módulo de IA: registrar tablas en whitelist y por departamento

---

## Lo que se decidió NO hacer (todavía)

- **LIFO**: no está en Venezuela, y agregarlo sin que nadie lo pida es código que nadie prueba
- **Costo estándar con desviaciones**: útil cuando haya producción en serie; hoy no la hay
- **Multi-moneda en el costo**: el costo se guarda en la moneda de la empresa, con la tasa del
  período; convertir al leer es perder precisión

Se dejan anotados para que la decisión quede escrita y no se reabra sin motivo.
