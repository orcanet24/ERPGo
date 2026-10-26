# PLAN DE ACCIÓN — 2026-10-26

**Asunto:** Catálogo de productos jerárquico, variantes (talla/color), productos compuestos y combos, módulo de producción por lotes, multi-moneda completa y depreciación de activos fijos imputada a costo.

**Estado:** PENDIENTE DE APROBACIÓN. No se aplica ningún cambio hasta que este documento sea leído y aprobado. Cada fase se aprueba por separado.

**Base:** análisis directo del código de ERPGo en la rama `main` (commit `af62d9c`), tablas de `migrations/`, pantallas en `interfaces/web/` y `templates/`. Lo que se afirma como "existe" está verificado con fichero y línea; lo que se afirma como "no existe" se buscó y no se encontró.

---

## 0. Cómo leer este documento

Cada punto sigue el mismo patrón:

1. **Qué hay hoy** — estado real verificado en código.
2. **Qué se pide** — su solicitud traducida a requisito funcional.
3. **Qué falta** — la pieza o piezas concretas.
4. **Impacto sobre lo existente** — qué pantallas, tablas y flujos se tocan.
5. **Migración sin romper** — cómo se aplica sobre instalaciones con datos reales.
6. **Esfuerzo y riesgo** — días-hombre estimados y nivel de riesgo técnico.

Al final: orden de aplicación, tabla resumen de esfuerzos, política de migración y las decisiones que quedan pendientes de su aprobación.

---

## 1. Diagnóstico rápido: qué hay y qué no

| # | Punto | Estado hoy | Evidencia |
|---|-------|-----------|-----------|
| 1 | Categorías con niveles (familias/grupos) | **NO existe.** Tabla plana: `category_id, category_name, status`. Pantalla `/product/category` existe pero solo lista plana. | `migrations/schema_reconstruido_02_inventory.sql:9`; menú id 130 en `schema_z_menu.sql` |
| 2 | Variantes (talla, color) | **NO existe nada**: ni atributos, ni variantes, ni tabla relacionada. | Búsqueda en todo `migrations/` sin resultados |
| 3 | Productos compuestos (pan) | **Existe a medias.** Hay receta/BOM (`product_recipe` con merma por componente), flag `is_assembled` y método de costeo RECIPE en el motor de costos. **Falta la transacción de ensamblado**: no hay endpoint ni pantalla que descuente componentes y cree el producto terminado. | `schema_z_costos.sql:77-97`, `cost_config_service.go`, `cost_engine.go`; sin endpoint de ensamble en `interfaces/web/` |
| 4 | Combos / kits (ofertas) | **NO existe.** | Búsqueda sin resultados |
| 5 | Producción (órdenes, lotes, líneas) | **NO existe.** No hay tabla de órdenes de producción ni pantalla. La base de costeo SÍ está (receta, rendimiento, hoja de horas, grados de mano de obra, centros de costo, ruta de proceso). **Además**: `process_route.work_center_id` referencia una tabla `work_center` que **nunca se creó** — referencia colgante. | `schema_z_costos.sql:163` (sin `CREATE TABLE work_center` en ninguna parte) |
| 6 | Multi-moneda | **Existe y está avanzada.** Moneda y tasa en facturas, compras, comprobantes contables, libro de clientes/proveedores, precios `price_usd` de productos, tasas BCV/oficial/paralelo con auditoría (`exchange_rate`, `exchange_rate_daily`, `exchange_rate_audit`), reportes con selector de moneda (`report_currency.go`) y libro de diferencial cambiario. **Huecos por cerrar** (se detallan en el Punto 5). | `schema_z_zzz_recuperadas_cols.sql:18-42`, `multi_currency.go`, `report_currency.go` |
| 7 | Depreciación de activos fijos | **NO existe motor.** Solo hay: cuentas CONASEV en el plan de cuentas (2.3.9 depreciación acumulada, 6.82 amortización) y las columnas `IsDepreciation`/`ResidualRate` en `acc_coa`. No hay registro de activos ni cálculo ni imputación a costo. | `data/conasev_coa.csv:610,1118`, `infrastructure/persistence/admin.go:227` |

**Conclusión del diagnóstico:** de los 7 focos, tres están sólidos y no se tocan (multi-moneda avanzada, base de costos, base de recetas), dos existen a medias y solo necesitan la transacción que los conecta (compuestos → ensamble; categorías → jerarquía), y tres son nuevos (variantes, producción, depreciación). Esto importa: **la mitad del trabajo es construir ENCIMA de lo que ya hay, no rehacerlo.**

---

## 2. Los puntos, uno a uno

### Punto 1 — Categorías jerárquicas (familias, grupos, subcategorías)

**Qué hay hoy.** `product_category` es plana (3 columnas). Existe la pantalla `/product/category` con permiso `category`. El producto se asigna a una categoría en un `<select>` plano (`templates/product_form.html:18-21`). La usan: reportes (`report.go`), conteo cíclico (`cycle_count.go`, cuyo `scope='category'` filtra por una categoría), semillas y el catálogo de la IA (`ai_schema_catalog.go`).

**Qué se pide.** Subcategorías con niveles ilimitados para clasificar familias/grupos.

**Diseño propuesto.**

- Añadir a `product_category`: `parent_id INT NULL`, `path VARCHAR(500)`, `level TINYINT`, `sort_order INT`.
- `path` = ruta materializada (`/1/7/23/`), que permite traer un árbol y filtrar "toda la rama" con un `LIKE` sin recursión en SQL.
- `parent_id NULL` = categoría raíz. **Todas las categorías existentes quedan como raíz con NULL: ninguna fila se reescribe, ningún comportamiento cambia.**

**Impacto sobre lo existente.**

| Zona | Cambio | Riesgo |
|------|--------|--------|
| `/product/category` | La lista plana pasa a árbol con indentación + crear/editar con padre | Bajo |
| `/product/add` y `/product/list` | El `<select>` muestra las hojas con indentación | Bajo |
| Reportes / conteo cíclico | Filtrar por categoría pasa a incluir la rama entera (via `path`) | Bajo — es la mejora deseada |
| Catálogo IA | Actualizar la descripción de la tabla | Nulo |
| POS / POSGo | Sin cambios: el producto sigue enviando su `category_id` | Nulo |

**Migración sin romper.** Migración aditiva e idempotente (`ADD COLUMN IF NOT EXISTS` con el patrón `information_schema` que ya usa `schema_z_costos.sql`). Sin reescritura de datos. Cero toggles: la funcionalidad es aditiva y no altera flujos fiscales ni de inventario.

**Esfuerzo y riesgo:** 3–4 días · riesgo **bajo**.

---

### Punto 2 — Variantes de producto (tallas, colores)

**Qué hay hoy.** Nada. `product_information` es un producto plano con `barcode` único, `price`, `price_usd`, `cost`.

**Qué se pide.** Productos con características alternas (textil: talla × color; calzado: talla), donde el inventario se lleva **por combinación**.

**Diseño propuesto — y por qué este y no otro.**

- Modelo **plantilla + producto variante** (el que usan Odoo y los POS modernos):
  - `product_attribute` (talla, color, material…), `product_attribute_value` (S, M, L / negro, rojo).
  - `product_information` gana `parent_product_id VARCHAR(100) NULL` e `is_template TINYINT`.
  - La plantilla agrupa y **no se vende**; cada combinación es un **producto hijo real** con su propio `product_id`, SKU, código de barras, precio, costo y **su propio stock**.
- Un generador de matriz en la ficha del producto crea los hijos de un solo clic (talla × color → N productos con SKU automático, ej. `CAM-001-M-NEG`).

**Por qué este modelo y no tablas de atributos EAV:** con producto-hijo, **todo lo existente sigue funcionando sin tocarse**: kardex, lotes, conteo cíclico, listas de precio, facturación, libros fiscales, respaldos y el POS — porque cada variante ya es un producto que esas piezas entienden. El modelo de atributos "compuestos" obligaría a reescribir facturas, sync POS e inventario para usar claves compuestas: alto riesgo de romper exactamente lo que ya funciona.

**Impacto sobre lo existente.**

| Zona | Cambio | Riesgo |
|------|--------|--------|
| Ficha de producto | Pestaña/panel "Variantes" con matriz y generación de hijos | Bajo |
| Búsqueda de producto (factura, compras) | Las plantillas se excluyen de las listas de venta (`is_template=0`); los hijos aparecen con su nombre compuesto (Padre + Talla + Color) | Medio — hay que filtrar en todos los selectores |
| Inventario / kardex / conteo | Sin cambios estructurales: cada hijo ya es producto | Nulo |
| Sync POS | Los hijos se sincronizan como productos normales; la plantilla no (o se envía marcada no vendible) | **Medio — toca POSGo también** |
| Código de barras / etiquetas | Funciona por hijo, que ya tiene barcode propio | Nulo |
| Listas de precio | Funcionan por hijo sin cambios | Nulo |

**Migración sin romper.** Tablas nuevas (no se altera el sentido de ninguna existente) + dos columnas nuevas con default NULL/0. Los productos existentes quedan `parent_product_id=NULL, is_template=0`: se comportan exactamente igual que hoy. Los selectores ganan un filtro que, mientras no haya plantillas, no cambia absolutamente nada.

**Esfuerzo y riesgo:** 6–8 días ERP · riesgo **medio** (los selectores de producto son muchos y hay que cazarlos todos). **POSGo necesita cambio aparte** para mostrar/agrupar variantes en terminal: estimar 2–3 días en el repositorio del POS, fuera de este plan.

---

### Punto 3 — Productos compuestos y combos

#### 3a. Compuesto real (el pan): ensamblado

**Qué hay hoy.** La receta existe y está viva: `product_recipe` (componente, cantidad, merma por componente, orden de pasos, sustitutos), `product_cost_config.is_assembled` ("Es fabricado", checkbox ya presente en `templates/cost_product.html:71`), y el motor de costos con método RECIPE que calcula el costo de fabricar. Incluso `costoVentaDeProducto()` en `invoice.go` ya prevé que lo producido sale del almacén con el costo del lote.

**Qué falta.** El **botón que hace el trabajo**: la transacción de ensamblado. Hoy el ERP sabe costear un pan pero no sabe *armarlo*.

**Diseño propuesto.**

- Pantalla/endpoint `Ensamblar`: producto ensamblado + cantidad → sistema calcula el escandallo (receta × cantidad, con merma) → verifica y descuenta componentes **por FEFO** de `product_lot` → crea lote del producto terminado con costo = Σ(componente × costo unitario × (1+merma)) → asiento de inventario (componentes → producto terminado) en la misma transacción.
- Todo dentro de una transacción SQL: o se arma completo o no arma nada.
- Si falta stock de algún componente: aviso claro con lo que falta, sin ensamblar a medias.

**Impacto.** Nuevo archivo de pantalla + endpoint; consumo por el mismo camino de movimientos de kardex que ya usan ajustes y facturas. **No toca facturación ni fiscal.** `costoVentaDeProducto` ya leerá el costo del lote creado: sin cambios ahí.

**Esfuerzo y riesgo:** 4 días · riesgo **medio-bajo**.

#### 3b. Combo / kit (ofertas y saldos)

**Qué se pide.** Vender "Combo X" descontando de inventario los productos que lo componen, sin pasar por producción.

**Diseño propuesto.**

- El combo es un producto normal con receta marcada como `kit` (columna nueva en `product_recipe` o flag en `product_cost_config`).
- Al facturar, la línea del kit **se expande a sus componentes ANTES de calcular totales**: salen del almacén los componentes, se costean los componentes, el cliente paga el precio del combo con su descuento. En el PDF de factura puede mostrarse la línea del kit (decisión de presentación).
- El combo **no tiene stock propio** (su disponibilidad depende de los componentes); esto se valida al momento de vender.

**Impacto.** Toca `invoice.go` (donde se arma la factura) — es la zona fiscal y hay que expandir **antes** de impuestos y totales para que el IVA y los asientos salgan correctos. POSGo: para vender combos en terminal hace falta el mismo ajuste en el POS (2–3 días aparte).

**Migración (3a+3b).** Columna/flag nuevo con default "no es kit": ninguna factura existente cambia de comportamiento. La expansión solo ocurre cuando el producto tiene receta marcada como kit.

**Esfuerzo y riesgo:** 3b: 4 días ERP (+2–3 días POSGo, aparte) · riesgo **medio** (zona fiscal; se mitiga con prueba sobre copia de la base real y verificación de totales/IVA/asientos).

---

### Punto 4 — Módulo de producción por lotes (el punto más grande)

#### 4.1. El modelo estándar de amplio espectro

La pregunta clave: **¿existe un modelo que sirva para restaurante, panadería, taller, textilera y carnicería a la vez?** Sí, y el propio sistema ya trae las piezas sueltas. El modelo es:

> **UNA ORDEN = UN LOTE DE PRODUCCIÓN.**
> La orden dice qué se quiere hacer, en qué línea, con qué receta. Al ejecutarla: consume insumos (escandallo), registra horas (mano de obra), produce salidas (con rendimiento) y cierra con un costo calculado.

Las tres componentes del costo son las de siempre (MPD + MOD + CIF), y **cada una ya tiene casa en el sistema**:

| Componente | Qué es | Pieza existente que lo resuelve |
|------------|--------|--------------------------------|
| **MPD** — material | Lo que consume la receta, con merma | `product_recipe` + `product_lot` (FEFO) + kardex |
| **MOD** — mano de obra | Horas trabajadas por rol con su tarifa | `cost_timesheet` + `labor_grade` (ya existe) |
| **CIF** — carga fabril | Gastos indirectos prorrateados | `cost_center` con `overhead_rate_hour` (ya existe) |
| **Rendimiento** | De 1 canal salen X kg de venta | `product_yield` (ya existe) |
| **Ruta de proceso** | Estaciones, tiempos, preparación | `process_route` (existe — pero le falta su tabla `work_center`) |

**Lo que falta es solo el orquestador**: la orden que ejecuta estas piezas en secuencia. No hay que inventar el motor de costos — ya está y está probado por cuatro tipos de negocio (servicios, textilera, carnicería, autopartes, según el propio diseño de `schema_z_costos.sql`).

**Cómo se aplica a cada negocio** (la prueba de que el modelo es de amplio espectro):

| Negocio | Línea de producción | Una orden es… | Insumos | Salidas |
|---------|--------------------|----------------|---------|---------|
| **Panadería** | Horno | Lote de 200 panes | harina, levadura, azúcar (con merma) | 200 panes (si rinden 195, el rendimiento lo registra) |
| **Restaurante** | Cocina | Orden de preparación de una receta | ingredientes por porción | platos/porciones listas |
| **Taller** | Estación de ensamble | Orden de trabajo de N piezas | materiales + horas del operario | producto terminado + costo con horas |
| **Textilera** | Cortadora | Lote de 50 camisas | tela (merma de corte), botones | 50 camisas |
| **Carnicería** | Mesa de despiece | Lote de 1 canal | la canal | cortes a la venta (rendimiento %) |

**Control por lotes como eje:** cada orden genera **un lote de producto terminado** (`product_lot` ya existe, con código, fecha, vencimiento y costo). Ese lote es el que sale por FEFO en las ventas: trazabilidad de "este pan salió de la orden del martes 5 de la línea Horno 2" sin ninguna tabla nueva de trazabilidad.

#### 4.2. Diseño concreto

**Tablas nuevas** (aditivas):

- `work_center` — **la que falta** y que `process_route` ya espera: estaciones/líneas (Horno 1, Cocina, Cortadora, Estación 3).
- `production_order` — cabecera: producto salida, cantidad planificada/real, `work_center_id`, fechas, estado (`borrador → en curso → cerrada → cancelada`), costos (MPD, MOD, CIF, total, unitario), lote generado.
- `production_order_item` — renglones de consumo (componente, cantidad, lote) y de salida (producto, cantidad real).

**Flujo de la pantalla única "Producción":**

1. **Nueva orden**: elegir producto, cantidad → el sistema trae la receta automáticamente y muestra el escandallo.
2. **Ejecutar**: consumir insumos (FEFO automático, con aviso de faltantes) + registrar horas (opcional, se liga a `cost_timesheet`).
3. **Cerrar**: producir cantidad real (con rendimiento), sistema calcula costo unitario = (MPD + MOD + CIF) / producido, **crea el lote** y hace el asiento (insumos → producción en curso → producto terminado).
4. **Panel**: órdenes abiertas por línea, con su estado — la vista de "cómo va la producción" de cualquier negocio.

**Impacto sobre lo existente.**

| Zona | Cambio | Riesgo |
|------|--------|--------|
| Kardex / inventario | La orden mueve stock por los mismos camiones de movimiento que ya usan facturas y ajustes | **Medio-alto** — es el punto delicado; todo en transacción |
| Motor de costos | Se alimenta con datos reales de órdenes; no se modifica su lógica | Bajo |
| `process_route` | Se crea `work_center` y se cierra la referencia colgante | Bajo (hoy esa ruta ni se puede usar) |
| Menú / permisos | Entrada "Producción" con su prefijo de permiso (siguiendo `schema_z_menu.sql`) | Nulo |
| Facturación / fiscal | **No se toca** | Nulo |
| POS | **No se toca** (vende productos terminados como hoy) | Nulo |

**Migración sin romper.** Tablas nuevas puras; ninguna existente se modifica. El módulo completo es opcional: mientras nadie crea una orden, el sistema no cambia. Prerequisito de despliegue: copia de la base de producción para la prueba.

**Esfuerzo y riesgo:**

- **Fase P1 (recomendada): orden simple** — crear, consumir, producir, costo, lote, asiento, panel por línea: **8–10 días**, riesgo **medio-alto** (es el único punto que mueve inventario automáticamente; se mitiga con transacción única + prueba en copia + validación de cuadre de kardex antes/después).
- **Fase P2 (posterior, opcional): rutas y programación** — `process_route` operativa, tiempos de ciclo, eficiencias por línea: **5–7 días** adicionales.

---

### Punto 5 — Multi-moneda completa (doble moneda en todo)

**Qué hay hoy.** Es, con diferencia, el punto más maduro del sistema:

- Facturas: `currency_code`, `exchange_rate`, `total_amount_usd`, `exchange_rate_source` (facturas ya soportan VES/USD/EUR según el catálogo de monedas).
- Compras: moneda, tasa y total en USD.
- Contabilidad: `acc_vaucher` y `acc_transaction` con moneda y tasa.
- Clientes/proveedores (`person_ledger`, `supplier`): moneda.
- Productos: `price` (moneda base) + `price_usd` + `currency_code`.
- Tasas: `exchange_rate` (BCV diaria), `exchange_rate_daily` (oficial/paralelo, múltiples fuentes), `exchange_rate_audit` (qué tasa se aplicó en cada factura), consulta con fallback a la última tasa anterior (`multi_currency.go`).
- Reportes: selector de moneda (`report_currency.go`) y **Libro de Diferencial Cambiario** en Libros Legales VE.

**Qué falta (huecos a cerrar, verificación + ajustes puntuales):**

1. **Valorización del kardex**: confirmar/documentar en qué moneda se guarda `unit_cost` (VES por convención) y que una compra en USD se convierte al tasa de la fecha — si algún camino guarda solo la moneda de origen, cerrarlo.
2. **Listas de precio por moneda**: `product_price_list.price` es un monto simple; si se quiere vender "lista Mayoreo en USD", hoy no se puede distinguir. Decisión: columnas `currency` en listas, o listas separadas por moneda.
3. **Sync POS**: verificar que el catálogo y las ventas del POS llevan moneda y tasa correctamente.
4. **Barrido de reportes**: comprobar que todos los reportes monetarios pasan por `report_currency.go` y no calculan a mano.

**Impacto.** Mayoritariamente **lectura y verificación**; los cambios son columnas/ajustes puntuales. Nada de lo existente cambia de comportamiento.

**Esfuerzo y riesgo:** auditoría **2–3 días** + cierre de huecos **2–4 días** (se define tras la auditoría) · riesgo **bajo**.

**Recomendación:** hacer esta auditoría **primero** (Fase 0), porque cualquier ajuste de variantes o producción debe diseñarse sobre la moneda correcta desde el día uno.

---

### Punto 6 — Depreciación de activos fijos imputada al costo de producto

**Qué hay hoy.** Cuentas CONASEV en el plan (`2.3.9` depreciación acumulada, `6.82/6.83` amortización) y las columnas `IsDepreciation`/`DepreciationRate` en `acc_coa`. No hay registro de activos ni cálculo.

**Qué se pide.** Un equipo tiene vida útil; mientras no esté pagado, cada venta amortiza su valor de forma proporcional al año y a los productos; al terminar su vida útil, su costo marginal es 0. Y todo esto **parametrizable**: muchos comercios NO imputan depreciación a costo.

**Diseño propuesto.**

- **Registro de activo fijo**: `fixed_asset` (descripción, costo de adquisición, fecha de compra, vida útil en años, valor residual, método = línea recta, cuentas contable contable de activo y de depreciación acumulada, centro de costo).
- **Calendario de depreciación**: `fixed_asset_dep` — anual = (costo − residual) / vida útil; mensual = anual / 12. Se genera automáticamente al cerrar cada mes.
- **Asiento mensual automático**: Débito gasto de depreciación (6.8x) / Crédito depreciación acumulada (2.3.9). Cuando la acumulada llega al costo, **el activo deja de depreciar y su costo marginal es 0** (se corta solo).
- **Imputación a costo de producto** (lo que decide usted): dos parámetros globales, ambos **APAGADOS POR DEFECTO**:
  1. `depreciacion_activa` — ¿se calcula depreciación? (default: no, hasta que se den de alta activos)
  2. `imputar_a_costo` — ¿la depreciación mensual entra al costo de los productos? (default: no; muchos negocios la dejan como gasto del período)
- **Forma de imputar cuando se activa** (dos opciones, a decidir):
  - **A) Por centro de costo** (recomendada): el activo se asigna a una línea/estación (el horno de la panadería) y su depreciación mensual entra como CIF de esa línea, prorrateada entre las órdenes que pasaron por ella. Es el modelo estándar y el que ya prepara `cost_center`.
  - **B) Por unidades** (más simple): depreciación anual ÷ unidades producidas/vendidas en el año = recargo por unidad. Más fácil de explicar, menos preciso.

**Nota de honestidad fiscal:** esto es un cálculo **contable y de gestión**. La deducción fiscal ante el SENIAT sigue sus propias reglas y tablas; el sistema debe permitir que lo que se impute a costo contable no se confunda con lo declarado. Por eso el feature viene apagado y separado: **quien no lo active, su contabilidad y sus costos quedan exactamente iguales que hoy.**

**Impacto.** Módulo nuevo con menú propio (Activos Fijos / Depreciación). Punto de contacto con lo existente: solo cuando `imputar_a_costo=1`, la depreciación entra al cálculo de CIF — que hoy solo se alimenta de `overhead_rate`. Default OFF ⇒ **cero impacto en instalaciones existentes**.

**Esfuerzo y riesgo:** 5–6 días · riesgo **bajo** (todo apagado por defecto).

---

## 3. Orden de aplicación propuesto

El orden no es arbitrario: primero lo que es base de lo demás y lo que da información para diseñar bien las fases siguientes.

| Fase | Punto | Días | Riesgo | Por qué este orden |
|------|-------|------|--------|--------------------|
| **F0** | Auditoría y cierre de multi-moneda | 2–3 (+2–4 de huecos) | Bajo | Da el terreno monetario sobre el que se diseñan todo lo demás |
| **F1** | Categorías jerárquicas | 3–4 | Bajo | Base de clasificación; bajo riesgo; desbloquea reportes por familia |
| **F2** | Variantes (talla/color) | 6–8 | Medio | Necesita las categorías para no crear un catálogo desordenado |
| **F3a** | Ensamblado de compuestos (pan) | 4 | Medio-bajo | Usa receta existente; requisito previo natural de producción |
| **F3b** | Combos/kits en venta | 4 | Medio | Zona fiscal; hacerla cuando F3a haya validado el patrón de expansión |
| **F4** | Módulo de producción (Fase P1) | 8–10 | Medio-alto | El más grande; se apoya en F1–F3a ya validados |
| **F6** | Depreciación e imputación | 5–6 | Bajo | Independiente; mejor al final, cuando producción ya da centros de costo reales |

**Total estimado: 34–46 días-hombre** (más 2–3 días de POSGo para variantes y combos en terminal, en el repositorio aparte del POS). No incluye pruebas en campo con clientes.

**Fase P2 de producción** (rutas, tiempos, eficiencias: 5–7 días) queda pendiente de decisión tras validar P1 con un negocio real.

---

## 4. Política de migración estricta (la garantía de "sin romper nada")

1. **Solo migraciones aditivas e idempotentes.** Ningún `ALTER` que reescriba datos; patrón `information_schema` que ya usa el proyecto para añadir columnas sin romper versiones viejas de MySQL.
2. **Ningún cambio de comportamiento por defecto.** Todo feature nuevo nace apagado o es puramente aditivo: una instalación existente que actualice y no toque nada nuevo se comporta **idénticamente**.
3. **Backup automático antes de cada fase** (el planificador de backups ya existe y está activo) + respaldo manual de la base de producción antes de aplicar cualquier migración nueva.
4. **Prueba sobre copia de la base real**, nunca sobre la producción directamente: cada fase se valida primero con datos reales copiados.
5. **Verificación por fase obligatoria antes de darla por terminada:** `go vet`, build del binario, auditor de rutas sin huérfanas, y prueba manual de TODAS las pantallas afectadas por la fase (no solo la nueva).
6. **Cuadre de inventario** en fases que mueven stock (F3, F4): kardex antes/después debe cuadrar.
7. **Nada se aplica hasta su aprobación**, fase por fase. Cada fase es un commit separado y reversible.

---

## 5. Decisiones que quedan pendientes de SU aprobación

Este plan no se toca hasta que usted diga "sí". Y al decirlo, puede decidir estas 5 cosas:

1. **Modelo de variantes**: ¿producto hijo (recomendado, seguro) o atributos compuestos (arriesgado)?
2. **Alcance de producción**: ¿solo Fase P1 (orden simple, 8–10 días) o ya se aprueba también P2 (rutas y programación, +5–7)?
3. **Depreciación — forma de imputar**: ¿por centro de costo (recomendada, precisa) o por unidades (simple)?
4. **Combos**: ¿expandir a componentes en la factura (recomendado, sin stock propio) o dar stock propio al combo?
5. **Orden**: ¿se respeta el orden F0→F6 propuesto o se reordena?

---

## 6. Qué NO entra en este plan

- **POSGo**: variantes y combos en la terminal del POS se estiman aparte (2–3 días) por ser otro repositorio con su propio ciclo de despliegue.
- **Capacidad de producción / planificación avanzada** (MRP, compras sugeridas): fuera de alcance por ahora; el `product_daily_demand` existente puede ser la semilla futura.
- **Ninguna modificación** a facturación fiscal, libros legales, respaldos, GOti o IA — este plan no los toca.

---

*Documento generado el 2026-10-26 a partir del análisis del código en `main` (`af62d9c`). Pendiente de lectura, evaluación y aprobación.*
