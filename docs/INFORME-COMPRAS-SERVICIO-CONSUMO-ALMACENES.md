# Informe: compras separadas por destino (servicio, consumo interno y venta) con almacenes reales

Fecha: 2026-11-06
Alcance: `interfaces/web`, `internal/pos`, `templates`, `migrations`, `docs`.

---

## 1. Qué estaba mal y por qué importaba

El ERP trataba **toda compra como mercancía**. No era un detalle cosmético: el
mismo error se propagaba al inventario, al costo y a la contabilidad.

| Síntoma | Dónde estaba | Consecuencia real |
|---|---|---|
| Todo renglón comprado entraba a inventario | `interfaces/web/purchase.go` (`purchaseSave`), `inventory_stock.go` | La luz, el agua o una asesoría quedaban como un activo con existencias; el gasto del período no aparecía |
| Un solo asiento para todo el documento | `purchase.go`: `COAID: purchaseCOA` en cada fila | Servicios y papelería se debitaban contra Inventario (1020401); el IVA engordaba el inventario |
| El stock se calculaba como “comprado − facturado” | `inventory_stock.go` | Los ajustes y conteos no cuadraban, y no existía stock por sucursal |
| El almacén era siempre `WH-MAIN` | `purchase.go`, `invoice.go`, `adjustment.go` | El multi-almacén era decorativo: no se podía operar más de una locación |
| El kárdex leía detalles de compra y de factura | `kardex.go` | Un ajuste o un consumo no aparecían; el costo promedio se contaminaba |
| El ajuste escribía en `product_purchase_details` | `adjustment.go` | Un ajuste no es una compra: ensuciaba el historial y el costo promedio |
| Se valorizaba el ajuste sumando cantidades | `adjustment.go` | Ajustar 3 unidades asentaba 3,00 Bs en vez de 3 × costo |
| El módulo de servicios usaba una cuenta de ingreso | `accounting.go` (`ServiceCode` → 3010401) | Un gasto se registraba como ingreso, con el signo económico invertido |
| El POS consultaba una tabla inexistente | `internal/pos/sync_server.go` (`product_inventory_config`) | La consulta fallaba en silencio y **los servicios descontaban stock** en el POS |
| No había pantalla de almacenes ni de consumo interno | — | No se podía configurar la separación aunque la tabla `warehouse` existiera |
| Las cuentas no se podían atar al plan del comercio | `accounting.go` y `acc_predefine_account`, sin pantalla | Al cambiar de plan, los asientos seguían apuntando a códigos que ya significaban otra cosa (o no existían) |

La causa de fondo era una sola: **el destino del renglón no era un dato**. Se
decidía “por documento” o, directamente, no se decidía. Este trabajo convierte el
destino en un dato persistido y lo usa en todos los consumidores.

---

## 2. El modelo: tres tipos de ítem, tres tratamientos

`product_information.item_type` es la única fuente de verdad:

| `item_type` | Etiqueta | Mueve stock | Cuenta al comprar | Almacén |
|---|---|---|---|---|
| `venta` | Producto de venta | Sí | 1020401 Inventario de mercancía | De venta |
| `consumo` | Consumo interno | Sí | 1020403 Inventario de suministros | De consumo |
| `servicio` | Servicio / gasto | No | Cuenta de gasto del renglón (p. ej. 4021401) | — |

Se descartó `product_inventory_config` (tabla que ninguna migración creaba y que
el POS ya intentaba leer) para no tener dos verdades: el tipo vive en el catálogo
y se valida contra él en cada carga.

### 2.1. Destino por renglón de compra

`product_purchase_details` gana cuatro columnas:

- `item_type` — el destino del renglón;
- `warehouse_id` — de qué almacén entra (vacío en servicios);
- `coa_id` — la cuenta de gasto elegida (solo servicios);
- `concept` — el concepto libre del servicio (“luz de noviembre”, “asesoría legal”).

Un documento puede mezclar los tres destinos: sigue siendo **una** compra para el
Libro de Compras, las retenciones y el proveedor, pero cada renglón sabe a dónde
va. El formulario obliga a elegir tipo, almacén y —si es servicio— concepto y
cuenta; el servidor lo vuelve a validar porque el alta rápida y la importación no
pasan por el formulario.

### 2.2. Almacenes con tipo

`warehouse.kind` (`venta` | `consumo`) impide el error clásico: meter papelería en
el almacén de mercancía. Las reglas:

- un renglón `venta` solo entra a un almacén `venta`; uno `consumo`, a uno `consumo`;
- hay **un** almacén por defecto de cada tipo (`is_default`);
- un almacén con movimientos no se elimina: se desactiva, para que el histórico
  siga cuadrando;
- la migración crea `WH-CONSUMO` y deja `WH-MAIN` como almacén de venta por defecto.

### 2.3. El stock se lee de un solo libro

Antes: “comprado − facturado” en `inventory_stock.go`. Ahora: **`stock_movement`**,
con signo por tipo de movimiento y filtro por almacén. Ventajas:

- el total de la empresa y el de una sucursal son dos lecturas del mismo libro;
- un ajuste, una devolución o un consumo aparecen (antes no);
- los servicios no tienen movimientos, así que no aparecen en existencias;
- el costo promedio y el último costo salen de las compras (`purchase`), no de
  filas de ajuste.

### 2.4. Contabilidad: una cuenta por destino, IVA aparte

`purchaseSave` parte el documento en **base + IVA**:

1. la base se reparte entre los renglones en proporción a su importe
   (`repartirImporte`, que cierra al centavo en el último peso);
2. cada renglón se debita a **su** cuenta: inventario de mercancía, inventario de
   suministros o cuenta de gasto del servicio;
3. el IVA se debita a **1020901 IVA crédito fiscal**, no al inventario;
4. el crédito (caja/banco/proveedor) se reparte en las mismas proporciones para
   que la partida doble cierre.

Ejemplo: mercancía 1.000 + suministros 300 + servicio 200 + IVA 240 = 1.740,
pagados 740 y 1.000 a crédito.

```
Debe  1020401 Inventario de mercancía        1.000,00
Debe  1020403 Inventario de suministros        300,00
Debe  4021401 Gastos de servicios              200,00
Debe  1020901 IVA crédito fiscal               240,00
Haber 101     Caja/Banco                       740,00   (repartido por destino)
Haber <proveedor>                              1.000,00 (repartido por destino)
```

Las cuentas nuevas se siembran en `acc_coa` y se enlazan en
`acc_predefine_account`:

| Cuenta | Nombre | Uso |
|---|---|---|
| 1020403 | Inventario de suministros y consumibles | Activo del consumo interno |
| 1020901 | IVA crédito fiscal (compras) | IVA de todas las compras |
| 4021401 | Gastos de servicios | Renglones de servicio por defecto |
| 4021402 | Gastos de suministros y limpieza | Contrapartida del consumo interno |

`accounting.go` gana `SuppliesInventoryCode`, `SuppliesExpenseCode`,
`ServiceExpenseCode` y `PurchaseVatCode`, con lectura tolerante (si la columna no
existe todavía, cae a un valor seguro) y métodos que **no** caen a la cuenta de
ingreso: un gasto nunca se asienta contra 3010401.

### 2.5. Las cuentas se atan al plan del comercio, no a un código fijo

Una separación contable solo sirve si cada papel se asienta en la cuenta que la
empresa realmente usa. El sistema pide las cuentas **por papel**, no por código:

| Papel | Dónde se usa |
|---|---|
| Inventario de mercancía para la venta | Entrada de renglones de venta |
| Costo de ventas | Salida de mercancía, faltantes de inventario |
| Inventario de suministros y consumibles | Entrada de consumo y consumo interno |
| Gasto por consumo de suministros | Contrapartida del consumo interno |
| Gasto por servicios recibidos | Valor por defecto del renglón de servicio |
| IVA crédito fiscal (compras) | IVA de todas las compras |
| Caja, banco, clientes, proveedores, ventas, IVA por pagar, impuestos, sueldos | Operación general |

Cada papel se guarda en `acc_predefine_account` y se administra en
**Configuración → Cuentas por defecto** (`/settings/accounts`), donde el contador
elige cualquier cuenta de detalle del plan de la empresa. Reglas:

- la cuenta debe existir, estar activa, admitir movimientos y ser de la
  naturaleza del papel (activo para inventarios e IVA crédito, gasto para
  gastos, ingreso para ventas). Así se impide el defecto original: un gasto
  contra una cuenta de ingreso;
- **no hay respaldos silenciosos**: si un papel no tiene cuenta, la compra o el
  consumo se detienen con un mensaje que dice cuál papel falta y dónde
  asignarlo. Antes caía al inventario general o al costo de ventas y el error
  pasaba inadvertido;
- **«Vincular al plan estándar»** solo ata los papeles de la separación
  (inventarios, gastos de suministros/servicios e IVA crédito). Exige tres
  cosas: que el código exista, que su naturaleza sirva y que su **nombre**
  corresponda al papel. Así, si el plan nuevo usa el código 4010101 para otra
  cosa, no se ata «Costo de ventas» a ciegas;
- los papeles generales (caja, banco, ventas, clientes, proveedores) **no se
  tocan automáticamente**: la empresa los configuró antes y reemplazarlos por
  código sería adivinar. Si dejaron de ser válidos, se marcan y se corrigen en
  la pantalla;
- al cargar el catálogo CONASEV, el sistema **revalida** todos los papeles y
  re-ata los de la separación cuando encuentra equivalente seguro; el resto
  queda pendiente y se informa en la pantalla y en el catálogo, en vez de seguir
  apuntando a un código que ahora significa otra cosa.

---

## 3. Consumo interno: el módulo que faltaba

`interfaces/web/consumo_interno.go` + `templates/consumption_*.html`.

Flujo: se eligen productos de tipo `consumo`, el almacén de consumo y un concepto
obligatorio. El sistema:

- valida que cada producto sea de consumo y que haya stock **en ese almacén**;
- valoriza al costo promedio de compra (o al último costo) y rechaza el consumo
  si el producto no tiene costo: no se inventa un gasto en cero;
- escribe un movimiento `consumo` (salida) y un asiento:
  `Debe 4021402 Gastos de suministros / Haber 1020403 Inventario de suministros`;
- permite eliminar un consumo cargado por error: borra movimiento, asiento,
  renglones y cabecera en una transacción, y el stock vuelve al almacén.

El consumo interno **no toca costo de ventas ni la mercancía de venta**: ese era
exactamente el número que distorsionaba el margen.

---

## 4. Ventas, ajustes, devoluciones y reportes

- **Factura** (`invoice.go`): elige el almacén de salida del documento (selector
  en el formulario; respaldo al almacén de venta por defecto). Bloquea vender un
  ítem de consumo interno, no descuenta stock ni genera costo de ventas para
  servicios, y el COGS solo se calcula si el ítem tiene costo (`purchaseValue > 0`).
- **Ajuste** (`adjustment.go`): escribe `stock_movement` con el costo del producto;
  usa almacén por tipo (el del documento si corresponde, si no el del tipo);
  asienta mercancía contra costo de ventas y suministros contra gasto de
  suministros, por separado; el borrado limpia movimientos y asientos.
- **Conteo cíclico** (`cycle_count.go`): el descuadre se escribe como
  `adjustment_add`/`adjustment_remove` en `stock_movement` (antes iba al detalle
  de compras y, con la nueva fuente de stock, se habría perdido).
- **Devoluciones** (`return.go`): usan el almacén del movimiento original; la
  devolución a proveedor ya no inserta cantidades negativas (el signo lo da el
  tipo de movimiento).
- **Despacho** (`delivery_order.go`): usa el almacén real del despacho y omite
  servicios.
- **Kárdex** (`kardex.go`): una sola fuente (`stock_movement`), columna Almacén y
  aviso cuando el producto es un servicio (no tiene movimientos que mostrar).
- **Reporte de stock** (`report.go`): filtros por tipo y almacén, sin servicios,
  con etiqueta de tipo y stock por almacén.
- **Libro de inventario** (`legal_books.go`): renglones compartidos con el reporte,
  sin servicios; entradas, salidas y ajustes desde movimientos.
- **POS** (`internal/pos/sync_server.go`): lee `item_type` del catálogo; ya no
  descuenta stock de servicios ni de consumo interno. Sigue usando `WH-MAIN`
  porque el terminal no elige almacén (ver límites).

---

## 5. Archivos

### Nuevos

| Archivo | Para qué |
|---|---|
| `interfaces/web/items_almacen.go` | Reglas de tipo de ítem, almacenes por tipo, validación almacén–tipo, `stockPorMovimientos`, costo promedio y último costo, `destinoCompra` |
| `interfaces/web/purchase_renglones.go` | Interpreta los renglones del formulario, valida tipo contra catálogo, concepto de servicio, cuenta de gasto y reparte importes al centavo |
| `interfaces/web/consumo_interno.go` | Módulo de consumo interno (lista, alta, detalle, baja) con su asiento |
| `interfaces/web/almacen.go` | CRUD de almacenes, tipo, predeterminado por tipo, protección de borrado, datos para formularios |
| `interfaces/web/cuentas_rol.go` | Papeles contables, validación por naturaleza, guardado del mapa, vínculo con el plan estándar y detección de papeles pendientes |
| `interfaces/web/cuentas_rol_test.go` | Pruebas del mapa: rechazo de cuentas de ingreso como gasto, cuentas de grupo, re-vínculo que respeta lo propio y limpia lo inexistente |
| `templates/settings_accounts.html` | Pantalla Configuración → Cuentas por defecto |
| `interfaces/web/items_almacen_test.go` | Pruebas del núcleo: normalización, destino, cuentas sin caer a ingreso, respaldo de almacén, reparto al centavo, stock por almacén, devolución a proveedor |
| `templates/warehouse_list.html`, `warehouse_form.html` | Pantalla de almacenes |
| `templates/consumption_list.html`, `consumption_form.html`, `consumption_details.html` | Pantalla de consumo interno |
| `migrations/schema_zzzzz_servicio_consumo_almacenes.sql` | Toda la estructura y las semillas de la separación |
| `docs/INFORME-COMPRAS-SERVICIO-CONSUMO-ALMACENES.md` | Este informe |

### Modificados

| Archivo | Cambio y motivo |
|---|---|
| `interfaces/web/accounting.go` | Cuentas nuevas y helpers que no caen a la cuenta de ingreso ni al IVA por pagar |
| `interfaces/web/coa_catalog.go` | Al cargar un plan, revalida y re-ata los papeles contables al catálogo nuevo |
| `interfaces/web/coa_catalog_handlers.go` | Informa papeles pendientes y resultado del re-vínculo |
| `interfaces/web/conasev.go` | El resultado del sembrado incluye qué papeles se asignaron y cuáles quedaron pendientes |
| `templates/setting.html` | Acceso a la pantalla de cuentas por defecto desde Configuración |
| `templates/coa_catalog.html` | Aviso de papeles sin cuenta válida después de cambiar de plan |
| `interfaces/web/purchase.go` | `purchaseSave` valida destinos, asienta por renglón con IVA aparte, guarda tipo/almacén/cuenta/concepto y mueve stock solo si corresponde; el borrado limpia movimientos; formulario y detalle muestran el destino |
| `interfaces/web/inventory_stock.go` | Stock desde `stock_movement`, por almacén, sin servicios; reservas compatibles |
| `interfaces/web/invoice.go` | Almacén de salida por documento, bloqueo de consumo interno, sin stock/COGS en servicios |
| `interfaces/web/adjustment.go` | Movimiento real con costo, cuentas separadas por tipo, borrado completo |
| `interfaces/web/cycle_count.go` | Descuadre como movimiento de ajuste (no como compra) |
| `interfaces/web/kardex.go` | Reescrito sobre `stock_movement`, con almacén |
| `interfaces/web/report.go` | Reporte de existencias con filtros y tipo |
| `interfaces/web/legal_books.go` | Libro de inventario desde movimientos, sin servicios |
| `interfaces/web/return.go` | Almacén del movimiento original; sin cantidades negativas |
| `interfaces/web/delivery_order.go` | Almacén del despacho real |
| `interfaces/web/product.go` | `item_type` y `default_warehouse_id` en alta, edición, listado y búsqueda |
| `interfaces/web/entrada_rapida.go` | El alta rápida propaga tipo y almacén |
| `interfaces/web/purchase_hash.go` | Los servicios se identifican por concepto en la huella anti-duplicados |
| `interfaces/web/server.go` | Registra las rutas de almacenes y consumo interno |
| `interfaces/web/stock_movement_helper.go` | Fecha portable (`CURRENT_TIMESTAMP`) |
| `internal/pos/sync_server.go` | Tipo real del ítem; sin stock para servicios ni consumo |
| `templates/purchase_form.html` | Columna Tipo, concepto, cuenta de gasto, almacén por renglón, validación y modal de producto con tipo |
| `templates/purchase_details.html` | Muestra tipo, almacén, cuenta y concepto |
| `templates/product_form.html`, `product_list.html` | Tipo de ítem y almacén por defecto |
| `templates/invoice_form.html` | Selector de almacén de salida |
| `templates/adjustment_form.html` | Almacén del ajuste y aviso de que se valoriza al costo |
| `templates/report_stock.html` | Filtros por tipo/almacén y columna Tipo |
| `templates/kardex.html` | Columna Almacén y aviso para servicios |
| `docs/COMPORTAMIENTO-INVENTARIO.md` | Documenta el nuevo modelo; corrige que el kárdex ya no lee detalles de compra/venta |

### Funciones clave

- `itemMueveStock`, `tipoAlmacenDeItem`, `almacenValidoPara`: una sola definición
  de qué mueve stock y a qué almacén puede ir.
- `destinoDeRenglonCompra`: traduce un renglón a `{tipo, cuenta, almacén, mueve stock}`.
- `repartirImporte`: reparte la base y cierra la suma al centavo.
- `stockPorMovimientos`: única lectura de existencias (empresa o almacén).
- `costoPromedioCompra` / `ultimoCostoCompra`: valorización del consumo y del ajuste.
- `insertStockMovement`: única vía de escritura de movimientos (cantidad positiva,
  signo por tipo).

---

## 6. Migración: qué toca en la base de datos

Archivo: `migrations/schema_zzzzz_servicio_consumo_almacenes.sql` (se aplica
sola al arrancar, después de todas las demás; es idempotente).

1. **Catálogo**: `product_information.item_type` y `default_warehouse_id`.
2. **Detalle de compra**: `item_type`, `coa_id`, `warehouse_id`, `concept`.
3. **Almacenes**: `warehouse.kind`; se crea `WH-CONSUMO`; `WH-MAIN` queda como
   almacén de venta por defecto si no hay otro.
4. **Cuentas**: se crean 1020403, 1020901, 4021401 y 4021402 en `acc_coa` como
   punto de partida. El enlace en `acc_predefine_account` solo se hace si la
   cuenta existe con la naturaleza del papel (activo para inventarios e IVA
   crédito, gasto para los gastos) y no pisa una configuración existente. Si el
   plan de la empresa usa otra numeración, los papeles se atan desde
   Configuración → Cuentas por defecto.
5. **`stock_movement.movement_type`**: de ENUM a `VARCHAR(30)` para admitir
   `consumo`, `transfer_in`, `transfer_out`; índices por almacén y por tipo/fecha.
6. **Consumo interno**: tablas `internal_consumption` e
   `internal_consumption_details`.
7. **Histórico**:
   - los ajustes antiguos (`store_adjustment_details`, que no dejaban movimiento)
     se convierten en `adjustment_add`/`adjustment_remove`;
   - las **compras anteriores al libro de movimientos** se rellenan como
     `purchase` desde `product_purchase_details` (excluyendo renglones de ajuste
     `ADJ-%` y de conteo `CC-%`), para que el inventario no arranque en negativo;
   - los conteos físicos antiguos (`CC-%`) se convierten en ajustes con el signo
     del descuadre;
   - las cantidades negativas de movimientos de salida se normalizan a positivas
     (el signo lo da el tipo), para no restar dos veces.
8. **Menú y permisos**: entradas 5005 (Almacenes), 5006 (Consumo Interno) y 5007
   (Nuevo consumo) bajo “Gestión Inventario”; submódulos 46 y 47.
9. **Configuración de flujo**: se crean `inventory_flow_config` y
   `stock_reservation`, que el código leía pero ninguna migración creaba. Se
   siembra el modo histórico (descontar al facturar, sin reservas).

No se borra ni se renombra ninguna columna existente: la migración es aditiva
salvo el `MODIFY` del ENUM, que solo amplía lo admitido.

### Cómo verificar después de aplicarla

```sql
-- Debe haber un almacén de consumo y uno de venta por defecto
SELECT code, name, kind, is_default FROM warehouse ORDER BY kind, code;

-- Las cuentas nuevas existen y son transaccionales
SELECT HeadCode, HeadName, HeadType, IsTransaction FROM acc_coa
 WHERE HeadCode IN (1020403,1020901,4021401,4021402);

-- Ninguna compra histórica quedó sin movimiento
SELECT COUNT(*) FROM product_purchase_details d
  JOIN product_purchase p ON p.purchase_id = d.purchase_id
 WHERE IFNULL(d.batch_id,'') NOT LIKE 'ADJ-%'
   AND IFNULL(d.batch_id,'') NOT LIKE 'CC-%'
   AND NOT EXISTS (SELECT 1 FROM stock_movement s
                    WHERE s.reference_type='purchase' AND s.reference_id=d.purchase_id
                      AND s.product_id=CAST(d.product_id AS UNSIGNED));

-- El stock por almacén (una fila por producto/almacén)
SELECT product_id, warehouse_id,
       SUM(CASE WHEN movement_type IN ('purchase','adjustment_add','return_customer','transfer_in','consumo_reversal')
                THEN quantity ELSE -quantity END) AS stock
  FROM stock_movement GROUP BY product_id, warehouse_id;
```

---

## 7. Puesta en marcha (lo que hay que hacer una vez)

1. Aplicar la migración (arranque normal del ERP; se registra en
   `schema_migrations`).
2. Revisar el catálogo: marcar como `servicio` lo que no tiene existencias y como
   `consumo` la papelería/limpieza. Los ítems existentes quedan como `venta`.
3. Crear los almacenes de cada sucursal (`/warehouse`) y elegir el predeterminado
   de venta y de consumo.
4. Abrir Configuración → Cuentas por defecto y confirmar que cada papel esté
   atado a una cuenta del plan de la empresa (inventario de mercancía,
   inventario de suministros, gasto de suministros, gasto de servicios, IVA
   crédito fiscal). Si el plan es propio, asígnelas ahí; si usa el estándar,
   «Vincular al plan estándar» lo hace sin crear cuentas.
5. Cargar una compra de prueba con los tres tipos y comprobar: kárdex por almacén,
   reporte de existencias filtrado y los asientos del período.
6. Registrar un consumo interno de prueba y verificar que baja el almacén de
   consumo y no el de venta.

---

## 8. Verificación ejecutada

Con Go 1.27.3 en `$HOME/sdk/go/bin`:

- `go build ./...` — sin errores.
- `go vet ./interfaces/web` — sin observaciones.
- `go test ./...` — todo en verde (`erpgo/interfaces/web` incluido).
- `go run ./internal/webcheck -dir templates` — 166 plantillas revisadas, 0 con
  error estructural; 508 referencias, 0 rotas.
- Análisis sintáctico del SQL de la migración con dialecto MySQL: 39/39
  sentencias parseadas.

Las pruebas unitarias cubren las reglas que deciden dinero y stock
(`items_almacen_test.go`): normalización de tipos, destino por renglón, que una
cuenta de ingreso no se acepte como gasto, respaldo de almacén por tipo, reparto
al centavo y stock por almacén. `cuentas_rol_test.go` cubre el mapa contable:
rechazo de cuentas de ingreso en papeles de gasto, cuentas de grupo, cuentas
inactivas e inexistentes, y que el vínculo con el plan estándar respete lo ya
configurado, complete lo que falta y no invente cuentas.

---

## 9. Límites conocidos (honestos)

- **No se ejecutó contra MySQL real** en este entorno: no hay servidor MySQL
  disponible. La migración se validó por sintaxis y por revisión, no por
  ejecución. Antes de producción conviene aplicarla sobre una copia de la base.
- **El POS sigue escribiendo `WH-MAIN`**: el terminal no elige almacén. La
  separación por tipo sí se respeta (no descuenta servicios ni consumo).
- **No hay transferencias entre almacenes**: el tipo de movimiento
  `transfer_in`/`transfer_out` está admitido, pero no existe pantalla. Para mover
  mercancía entre sucursales hoy hay que usar ajuste de salida + ajuste de entrada.
- **Los servicios históricos** (compras anteriores a esta versión) quedaron
  cargados como inventario, porque no hay forma de adivinar cuáles eran gasto.
  Hay que corregirlos con un asiento de reclasificación o un ajuste de inventario;
  el informe no los reclasifica solo para no inventar contabilidad.
- **Borrar una compra, un ajuste o un consumo elimina sus asientos** sin
  recalcular `acc_monthly_balance`; es el mismo comportamiento que ya tenían los
  borrados del ERP. Si el período está cerrado, el borrado debe rechazarse (lo
  hace `exigirPeriodoAbierto`).
- **La pantalla de cuentas no crea cuentas**: si el plan no tiene una cuenta de
  inventario de suministros, el contador debe crearla en el árbol de cuentas y
  luego asignarla. Es deliberado: escribir cuentas en el plan de un comercio sin
  su decisión es lo que este cambio evita.
- **El reparto del crédito entre cuentas de destino** es proporcional al importe
  de cada renglón. El total por cuenta es correcto y la partida cierra al centavo,
  pero el detalle “cuánto de caja pagó cada renglón” es una asignación analítica,
  no un dato del documento.
- Las plantillas `inventory_control*.html` y `kardex_print.html` no muestran la
  columna Tipo/Almacén: se dejaron como estaban para no cambiar impresos legales
  sin pedido explícito.
