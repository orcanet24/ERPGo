# PLAN DE ADOPCIÓN — El script de costeo `docs/addons/costos/bd_costos.sql`

## 0. Veredicto en una frase

**No se crea un addon de costos y no se reemplaza el módulo actual: se adopta
del script lo que al ERP le falta y se corrige lo que el ERP tiene roto.** El
módulo de costos del núcleo ya es, en arquitectura, superior al script; en
cobertura funcional, el script gana en tres cosas concretas (activos fijos con
depreciación, aplicación de indirectos y unidades de medida). El plan de abajo
incorpora esas tres al núcleo y arregla los defectos reales encontrados.

---

## 1. Qué trae el script (inventario honesto)

`docs/addons/costos/bd_costos.sql` es un esquema **autónomo de una sola base**
(`DROP DATABASE costeo; CREATE DATABASE costeo; USE costeo;`) con:

| Bloque | Objeto | Qué aporta |
|---|---|---|
| 1 | `config` | Parámetros (margen, moneda, merma, redondeo) |
| 1 | `unidades_medida` | UoM con `permite_decimal` |
| 1 | `items` | Catálogo propio de ítems (terminado/insumo/ambos/servicio) |
| 1 | `item_costos` | Costo unitario **con vigencia** (`fecha_desde`/`fecha_hasta`) |
| 2 | `recetas` | BOM con `version`, `rendimiento_pct`, `merma_global_pct`, `lote_referencia`, `activa` |
| 2 | `receta_detalle` | Componente con `um_id` (unidad por línea) y `merma_pct` |
| 3 | `activos_fijos` | Activo con precio, residual, vida útil, `es_global` |
| 3 | `activo_depreciacion_cuota` | Cuota mensual/anual calculada |
| 3 | `activo_depreciacion_mov` | Movimiento mensual: acumulada y valor de libro |
| 3 | `auditoria` | Bitácora genérica de cambios y recálculos |
| — | Triggers | Alta de activo calcula cuota; cambio de datos exige recálculo con motivo |
| — | Procedimientos | `sp_recalcular_depreciacion`, `sp_cerrar_depreciacion_mes` |
| — | Vistas | Estado de depreciación y costo directo de receta |

**Lo que el script NO trae:** integración con ventas, compras, inventario,
kardex, contabilidad, multiempresa, permisos, auditoría encadenada, ni una sola
prueba. Es un modelo de datos, no un sistema.

---

## 2. Qué tiene hoy ERPGo (con evidencia)

El módulo está **vivo y enrutado**: `interfaces/web/server.go:518` llama a
`registerCostRoutes()` (`cost_handlers.go:36`), y las pantallas existen
(`templates/cost_home.html`, `cost_product.html`, `cost_period.html`,
`cost_timesheet.html`, `cost_proyectos.html`, `cost_proyecto.html`).

### 2.1 Esquema (`migrations/schema_z_costos.sql`)

`cost_timesheet`, `product_recipe`, `product_yield`, `labor_grade`,
`cost_center`, `process_route`, `import_cost`, `price_list`,
`product_price_list`, `product_cost_config`, `cost_component`,
`cost_snapshot`, `cost_period_close`. Además `cost_project` y `cost_task`
(`schema_reconstruido_10_system.sql`).

### 2.2 Motor (`interfaces/web/cost_engine.go`)

- Cálculo **puro** (`calcularCosto`) separado de la lectura: se prueba sin base.
- Cinco métodos: `PP`, `RECIPE`, `YIELD`, `HOURS`, `IMPORT`.
- Merma **por componente** (no global) en receta.
- Rendimiento de despiece repartiendo el costo de la entrada sobre lo que sale.
- Aduana repartida sobre unidades **recibidas**, no pedidas.
- Trazabilidad por renglón (`CostTrace`: componente, regla, tasa, cantidad,
  importe, fuente).
- Período congelable (`cost_snapshot`) y reapertura con motivo obligatorio.

### 2.3 Integración que el script no tiene

- `product_information.cost` es el CPP que mantiene el kardex
  (`kardex.go`), y el motor **no lo recalcula con otra fórmula**: lee el mismo
  número. Eso evita que el informe de costos y el libro de inventario
  discrepen.
- Las listas de precio (`price_list`, `product_price_list`) ya son un módulo
  del núcleo (`price_list.go`) y las usa el POS y la factura.
- `cost_project`/`cost_task` resuelven horas facturables vs. no facturables con
  motivo (`cost_timesheet_service.go`), algo que el script no contempla.
- Auditoría con cadena de hash (`audit_log`, `audit_chain_blocks`).

---

## 3. Comparación funcional (quién es superior en qué)

| Capacidad | Script | ERPGo | Veredicto |
|---|---|---|---|
| Catálogo de ítems | Propio (`items`) | `product_information` | **ERPGo** (no duplicar maestro) |
| Costo promedio | No | CPP desde kardex real | **ERPGo** |
| Costo con vigencia histórica | `item_costos` | Solo costo actual + kardex | **Script** (mejorable) |
| Receta/BOM | Versión, rendimiento, lote, activa | Componentes + merma por línea | **Empate** (cada uno aporta) |
| Unidad por línea y conversión | `um_id` + `permite_decimal` | `unit` es etiqueta, sin conversión | **Script** |
| Rendimiento de despiece | No (solo receta) | `product_yield` | **ERPGo** |
| Mano de obra por rol | No | `labor_grade` + `cost_timesheet` | **ERPGo** |
| Horas facturables por proyecto | No | Reglas + motivo | **ERPGo** |
| Importación / landed cost | No | `import_cost` + reparto | **ERPGo** |
| Centros de costo / indirectos | No | Tabla `cost_center` | **ERPGo en datos, Script en uso** |
| Rutas de proceso / tiempo máquina | No | Tabla `process_route` | **ERPGo en datos, sin uso** |
| **Activos fijos** | Sí | **No existe** | **Script** |
| **Depreciación mensual** | Sí | **No existe** | **Script** |
| **Valor de libro / acumulada** | Sí | **No existe** | **Script** |
| Trazabilidad del costo | No | `CostTrace` + `cost_component` | **ERPGo** |
| Congelado por período | No | `cost_snapshot` | **ERPGo** |
| Auditoría | Tabla simple | Cadena de hash | **ERPGo** |
| Multiempresa | No | Una base por empresa | **ERPGo** |
| Permisos / menú | No | `sub_module` 200–202 | **ERPGo** |
| Pruebas | Ninguna | **Ninguna tampoco** | **Empate en deuda** |

**Conclusión de la tabla:** el script no es "más preciso" en general; es más
**completo** en activos fijos y unidades, y más **débil** en todo lo demás. Lo
que corresponde es adoptar sus conceptos faltantes, no su esquema.

---

## 4. Defectos reales encontrados en el módulo actual

Estos no son opiniones: salen de leer el código y el esquema. Se marcan como
**verificados por inspección**; no se reprodujeron contra MySQL en esta sesión.

### 4.1 `cost_component` no admite más de un renglón del mismo componente

- `migrations/schema_z_costos.sql:311` declara
  `UNIQUE KEY uq_cost_component (period, product_id, component)`.
- `cost_engine.go:275-306` (`materialPorReceta`) genera **una traza por
  componente de la receta**, todas con `Component: compMaterial`.
- `cost_engine.go:722-737` inserta cada traza con `INSERT` simple.

Resultado: guardar el costo de un producto `RECIPE` con **dos o más
componentes** choca con la clave única en el segundo `material` y
`calcularCostoDeProducto` devuelve error. El desglose (GET) funciona porque no
guarda; el botón "Guardar costo" falla. La semilla de carnicería no lo detectó
porque usa `YIELD`, que produce una sola traza de material
(`semilla_carniceria.go:562-572`).

### 4.2 Depreciación e indirectos siempre valen cero

- `costInputs.DegrPerUnidad` y `costInputs.OverheadPerUn` se declaran
  (`cost_engine.go:126-127`) y se usan (`:231`, `:241`), pero **nunca se
  asignan** en `cargarCostInputs` (`:483-663`).
- `product_cost_config.cost_center_id` se guarda
  (`cost_config_service.go:140-156`) y no se lee en el motor.
- `cost_center.overhead_rate_hour` / `overhead_rate_unit` se administran y no se
  aplican.

Resultado: la pantalla muestra "Depreciación" e "Indirectos" en 0.00 siempre.
Un costo que parece completo y no lo está es peor que uno que avisa.

### 4.3 `process_route` y `productivity_factor` son datos muertos

- `process_route` existe (`schema_z_costos.sql:159-171`) y solo aparece en el
  catálogo de IA; ningún cálculo lo lee.
- `labor_grade.productivity_factor` se guarda (`cost_config_service.go:353`)
  pero `calcularTasaHoraria` (`cost_engine.go:464-471`) no lo usa.

### 4.4 Cero pruebas del módulo de costos

No hay `*_test.go` que ejercite `calcularCosto`, `resolverRenglones`,
`calcularTasaHoraria`, `cerrarCostoDePeriodo` ni `costFaltantes`. Esto incumple
la regla del proyecto de que cada regla de negocio lleva su prueba. Es la causa
raíz de que 4.1 haya pasado inadvertido.

### 4.5 El script tampoco es perfecto (para no copiarlo a ciegas)

- `DROP DATABASE`/`CREATE DATABASE`/`USE` rompen el modelo multiempresa.
- Triggers y procedimientos no son instalables como addon (ver §5).
- `item_costos` no tiene restricción que impida vigencias solapadas; la vista
  `v_costo_directo_receta` podría multiplicar renglones.
- `ENUM` rígidos y `CHECK` que MariaDB 10.4 ignora.
- `auditoria` sin cadena de integridad, inferior a `audit_log`.

---

## 5. ¿Addon o núcleo? Factibilidad real

### 5.1 Lo que un addon puede hacer hoy (verificado en `addons.go`)

- Crear **solo tablas propias** con `CREATE TABLE IF NOT EXISTS <id>_...`
  (`addonReDDL`, `addonValidarSchema`).
- **Leer** tablas del núcleo declaradas en `db_read` (grants SELECT).
- **Escribir** en el núcleo solo por verbos con nombre (`db_write`), que el
  núcleo debe implementar.
- Tener su propio binario, rutas, menú y permisos.

### 5.2 Lo que un addon NO puede hacer

- Crear triggers, procedimientos, vistas, ni ejecutar `DROP`/`ALTER`/`INSERT`
  de siembra (`addons.go:96-101`, `addonValidarSchema`).
- Alterar tablas del núcleo (agregar `loss_factor`, `cost_center_id`, etc.).
- Cambiar el flujo de factura, inventario o contabilidad.
- Escribir `product_information.cost`, que es el número que usa el kardex.

### 5.3 Consecuencia

El script **no se puede instalar como addon tal cual**: usa `DROP DATABASE`,
triggers, procedimientos y vistas, y crea un catálogo `items` paralelo. Un addon
de costos quedaría como un **calculador de vitrina** que no puede alimentar el
costo real del inventario ni la factura, y crearía una segunda fuente de
verdad. Eso es exactamente lo que el proyecto prohíbe.

**Papel válido para un addon (opcional, no prioritario):** un
`costos-analitica` de solo lectura que lea `cost_snapshot`, `cost_component`,
`product_information` y `cost_timesheet` para simular precios y márgenes, con
sus propias tablas de escenarios. Útil, pero no sustituye al motor.

**Decisión:** la adecuación va al **núcleo**, en migraciones + Go + pruebas.

---

## 6. Plan de adaptación por fases

Cada fase termina con `gofmt`, `go build ./...`, `go vet ./interfaces/web`,
`go test ./...` y su propia prueba. Ninguna fase se cierra en rojo.

### Fase 0 — Corrección y red de seguridad (bloqueante)

Objetivo: que lo que ya existe no mienta.

1. **Arreglar `cost_component`.** Cambiar la clave única a
   `(period, product_id, component, source_ref, grade)` o añadir una columna
   `line_no`; ajustar `guardarComponentesCosto` para conservar todas las trazas
   de material. Migración idempotente que rehaga el índice en bases existentes.
2. **Poblar depreciación e indirectos.** Implementar en `cargarCostInputs`:
   - depreciación por unidad = cuota del período del activo asignado al
     producto / unidades producidas o vendidas;
   - indirecto por unidad = `cost_center.overhead_rate_unit` (o por hora ×
     horas de la ruta).
   Mientras no haya activos, devolver 0 **y una advertencia explícita**
   (`Faltantes`) en vez de un cero silencioso.
3. **Usar `productivity_factor`** en `calcularTasaHoraria` o retirarlo del
   formulario. Un campo que se guarda y no se usa es una promesa falsa.
4. **Pruebas de la Fase 0** (sin base, milisegundos):
   - `calcularCosto` con receta de 3 componentes produce 3 trazas de material y
     la suma coincide con el total;
   - `calcularCosto` con depreciación e indirectos > 0 los suma;
   - `calcularTasaHoraria` aplica productividad y no divide entre cero;
   - `guardarComponentesCosto` (con base de prueba) no pierde renglones.

**Criterio de aceptación:** guardar un producto `RECIPE` de varios componentes
termina en éxito y el total del desglose es igual a la suma de componentes.

### Fase 1 — Activos fijos y depreciación (la brecha más grande)

Adoptar del script, adaptado al núcleo:

1. Migración `schema_z_costos_activos.sql` con:
   - `fixed_asset` (`code`, `name`, `type`, `is_global`, `purchase_price`,
     `residual_value`, `purchase_date`, `useful_life_months`, `active`,
     `created_by`, `created_at`);
   - `fixed_asset_depreciation` (cuota mensual/anual por activo, calculada en
     Go dentro de la transacción de alta, **sin trigger**);
   - `fixed_asset_depreciation_movement` (`asset_id`, `period`, `month_amount`,
     `accumulated`, `book_value`, `status`, `created_at`) con
     `UNIQUE(asset_id, period)`.
2. Servicio Go `cost_assets_service.go`:
   - alta y edición con validación (precio ≥ 0, vida > 0, residual ≤ precio);
   - `calcularCuota` pura y probada;
   - `cerrarDepreciacionDelPeriodo(periodo, usuario)` idempotente: no duplica
     movimientos, salta activos no depreciables y deja `estado='calculado'`;
   - cambio de precio/vida/residual/fecha **solo** por
     `recalcularDepreciacion(assetID, motivo, usuario)` con motivo obligatorio,
     escrito en `audit_log` (se reemplaza el trigger del script por una regla de
     servicio, que es la única forma portable al modelo multiempresa).
3. Pantalla `/cost/assets` (listado, alta, estado, valor de libro) y permisos
   en `sub_module`/`menu`.
4. Enganche con el motor: la depreciación del período entra como
   `compDepreciation` prorrateada por unidad; si el activo es `is_global`, se
   reparte entre los productos con `cost_center_id` compatible.
5. Pruebas:
   - cuota mensual = (precio − residual) / vida, con redondeo decimal;
   - cierre de mes idempotente (correr dos veces no duplica);
   - activo con vida vencida no sigue depreciando;
   - cambio sin motivo se rechaza; con motivo queda en `audit_log`;
   - el costo del producto sube exactamente la depreciación por unidad.

**Criterio de aceptación:** un horno de 12.000 con residual 0 y vida 60 meses
aporta 200/mes; con 1.000 unidades del período, 0,20 por unidad, visible en el
desglose y congelado en `cost_snapshot`.

### Fase 2 — Indirectos y rutas de proceso (usar lo que ya está)

1. Poblar `OverheadPerUn` desde `cost_center`:
   - `overhead_rate_unit` si el producto se costea por unidad;
   - `overhead_rate_hour × horas de ruta` si se costea por proceso.
2. Leer `process_route` para tiempo de máquina y mano de obra por estación:
   - `setup_time_min` **no se prorratea** entre unidades (regla del script y del
     comentario de la tabla);
   - `run_time_sec` y `machine_time_sec` sí, por unidad;
   - `labor_grade` de la ruta se suma al costo de mano de obra.
3. Pantalla de centros y rutas (ya hay endpoints de centros; falta la ruta).
4. Pruebas:
   - indirecto por unidad y por hora dan el resultado esperado;
   - `setup_time_min` se cobra completo aunque la cantidad sea 1;
   - una ruta sin grado no rompe el cálculo y avisa.

### Fase 3 — Unidades de medida y costo con vigencia

1. `unit_of_measure` (`code`, `name`, `allows_decimal`) y
   `product_uom_conversion` (`product_id`, `from_uom`, `to_uom`, `factor`).
   `product_information.unit` se conserva como unidad base y se migra a código.
2. `product_recipe` gana `uom` por línea; el motor convierte a la unidad base
   antes de multiplicar por costo. Sin conversión, una receta en gramos sobre un
   costo por kilo sale 1.000 veces mal.
3. Costo histórico: **no** crear un segundo libro. Se expone
   `product_cost_at(product_id, fecha)` derivado del kardex
   (`stock_movement.unit_cost` + CPP) y, si se necesita velocidad, una tabla
   materializada de solo lectura alimentada por el kardex, nunca escrita a mano.
4. `product_recipe` gana `version`, `active`, `batch_reference` y `yield_pct`
   (rendimiento de la receta, distinto del despiece de `product_yield`).
5. Pruebas:
   - conversión g → kg en receta;
   - versión inactiva no se usa;
   - `yield_pct` reduce la salida y sube el costo unitario;
   - costo a una fecha pasada coincide con el kardex de esa fecha.

### Fase 4 — Informes e integración contable

1. Informe de costo vs. precio vs. margen por producto y por lista.
2. Costo de ventas del período desde `cost_snapshot` (no desde el costo actual:
   un mes cerrado no se reescribe).
3. Conciliación explícita: suma de `cost_snapshot` vs. salidas del kardex;
   si no cuadran, el informe lo dice con el renglón que difiere.
4. Pruebas de conciliación y de que un período cerrado no cambia al recalcular.

---

## 7. Mapa de objetos: script → núcleo

| Script | Destino en ERPGo | Acción |
|---|---|---|
| `config` | `settings` / configuración existente | **No crear**; mapear |
| `unidades_medida` | `unit_of_measure` (nueva) | Adoptar (Fase 3) |
| `items` | `product_information` | **Rechazar duplicado** |
| `item_costos` | `product_cost_at` desde kardex | Adoptar como lectura (Fase 3) |
| `recetas` | `product_recipe` + versión/rendimiento/lote | Ampliar (Fase 3) |
| `receta_detalle` | `product_recipe` + `uom` | Ampliar (Fase 3) |
| `activos_fijos` | `fixed_asset` (nueva) | Adoptar (Fase 1) |
| `activo_depreciacion_cuota` | `fixed_asset_depreciation` | Adoptar (Fase 1) |
| `activo_depreciacion_mov` | `fixed_asset_depreciation_movement` | Adoptar (Fase 1) |
| `auditoria` | `audit_log` | **No crear**; usar el existente |
| Triggers | Reglas de servicio Go | Reimplementar (Fase 1) |
| Procedimientos | Servicios Go transaccionales | Reimplementar (Fase 1) |
| Vistas | Consultas/reportes Go | Reimplementar (Fases 1 y 4) |

---

## 8. Riesgos y decisiones que hay que tomar antes de codificar

1. **Alcance de la depreciación:** ¿solo costo de producto o también asiento
   contable? La Fase 1 la deja como costo; el asiento contable es Fase 4 y
   requiere decidir cuentas.
2. **Prorrateo de activos globales:** ¿por horas de ruta, por unidades
   producidas o por consumo de centro? Hay que elegir una regla y dejarla
   escrita; el script no la define.
3. **Unidad base del producto:** migrar `product_information.unit` (texto
   libre) a códigos puede romper datos viejos. Se hará con tabla de
   equivalencias y aviso, no con `UPDATE` ciego.
4. **Versión de receta:** activar versiones cambia qué receta usa el cálculo.
   Debe quedar una sola activa por producto y el cambio debe quedar auditado.
5. **Compatibilidad:** las migraciones deben ser idempotentes y correr en
   MariaDB 10.4 y MySQL 8, como el resto del repo.

---

## 9. Lo que se verificó y lo que no

**Verificado por lectura de código y esquema en esta sesión:**

- Rutas del módulo registradas (`server.go:518`).
- Tablas de costos y sus columnas/índices.
- `DegrPerUnidad`/`OverheadPerUn` nunca asignados.
- `process_route` y `productivity_factor` sin uso en el cálculo.
- Clave única de `cost_component` incompatible con múltiples trazas de
  material.
- Ausencia total de pruebas del módulo.
- Restricciones reales del host de addons (solo `CREATE TABLE IF NOT EXISTS`
  propias; sin triggers/procedimientos/vistas).

**No verificado en esta sesión:**

- No se ejecutó MySQL ni la suite de integración, así que el fallo de clave
  duplicada de §4.1 está **inferido**, no reproducido. La Fase 0 incluye la
  prueba que lo reproduce antes de corregirlo.
- No se midió el impacto de las migraciones sobre bases existentes.
- No se validó el script contra MariaDB 10.4 real.

---

## 10. Recomendación final

1. **No crear el addon de costos.** No puede ser el sistema de registro y
   duplicaría el maestro de productos.
2. **Mantener el motor del núcleo** y arreglar la Fase 0 de inmediato: hoy hay
   un guardado que falla y dos componentes que siempre valen cero.
3. **Adoptar del script** activos fijos con depreciación (Fase 1), indirectos y
   rutas (Fase 2), y unidades/conversión (Fase 3).
4. **Usar `audit_log`**, no una tabla `auditoria` paralela.
5. **Escribir pruebas desde la Fase 0.** Sin ellas, la próxima brecha vuelve a
   pasar inadvertida.
