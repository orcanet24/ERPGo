# Plan de Implementación de Libros Legales Venezolanos en ERPGo

## 1. Análisis del Marco Legal Venezolano

Según el documento "En Venezuela, el marco legal exige a las empresas", los libros obligatorios se clasifican en:

### 1.1 Libros Contables (Código de Comercio, Art. 32-33)
- **Libro Diario** → Ya implementado: `acc_transaction` (asientos de débito/crédito cronológicos)
- **Libro Mayor** → Ya implementado: `/account/ledger` (movimientos por cuenta)
- **Libro de Inventarios y Balances** → Ya implementado: `/reports/stock` + `/account/balance-sheet`

### 1.2 Libros Mercantiles/Societarios
- **Libro de Accionistas** → No aplica (módulo de gestión societario avanzado, fuera del alcance inicial)
- **Libro de Actas de Asamblea** → No aplica
- **Libro de Actas de Junta Directiva** → No aplica

### 1.3 Libros Fiscales/Administrativos (SENIAT) — **NUEVOS, POR IMPLEMENTAR**
- **Libro de Compras (IVA)** → OBLIGATORIO
- **Libro de Ventas (IVA)** → OBLIGATORIO
- **Libro de Control de Inventario** → OBLIGATORIO
- **Libro de Retenciones de IVA** → OBLIGATORIO
- **Libro de Retenciones de ISLR** → OBLIGATORIO

### 1.4 Normativas Digitales Aplicables
- Moneda nacional (VES / Bolívares)
- Idioma castellano
- Formato de salida imprimible en hojas foliadas
- No inalterabilidad de registros cerrados

---

## 2. Plan de Implementación

### 2.1 Módulo: Libro de Compras (IVA)

**Descripción:** Consolida facturas de proveedores con créditos fiscales de IVA.

**Datos requeridos:**
- Número de factura / control del proveedor
- RIF del proveedor
- Fecha de emisión
- Base imponible
- IVA soportado (crédito fiscal)
- Exento / No gravado
- Número de comprobante de retención (si aplica)

**Fuente de datos en ERPGo:**
- `product_purchase` (chalan_no, purchase_date, grand_total_amount, total_vat_amnt)
- `supplier_information` (supplier_name, supplier_id)
- Relación con `product_purchase_details` (por producto)

**Endpoints:**
- `GET /reports/book-purchases` → Vista del libro de compras
- `GET /reports/book-purchases/data` → Datos JSON para DataTables
- `GET /reports/book-purchases/print` → Versión imprimible

**Archivos a crear:**
- `/interfaces/web/legal_books.go` (handlers para todos los libros legales)
- `/templates/book_purchases.html` (vista)
- `/templates/book_purchases_print.html` (versión imprimible)

---

### 2.2 Módulo: Libro de Ventas (IVA)

**Descripción:** Consolida facturas emitidas, notas de crédito/débito y débitos fiscales.

**Datos requeridos:**
- Número de factura
- RIF del cliente
- Fecha de emisión
- Base imponible
- IVA repercutido (débito fiscal)
- Exento / No gravado
- Número de comprobante de retención (si aplica)

**Fuente de datos en ERPGo:**
- `invoice` (invoice, date, total_amount, total_vat_amnt, total_tax)
- `customer_information` (customer_name, customer_id)
- `invoice_details` (por producto)

**Endpoints:**
- `GET /reports/book-sales` → Vista del libro de ventas
- `GET /reports/book-sales/data` → Datos JSON
- `GET /reports/book-sales/print` → Versión imprimible

**Archivos:**
- `/templates/book_sales.html`
- `/templates/book_sales_print.html`

---

### 2.3 Módulo: Libro de Control de Inventario

**Descripción:** Seguimiento detallado de unidades físicas y su costo (Reglamento ISLR).

**Datos requeridos:**
- Producto (código, descripción, modelo, unidad)
- Entradas (compras, ajustes +)
- Salidas (ventas, ajustes -)
- Saldo final (cantidad y valor)
- Costo promedio ponderado

**Fuente de datos en ERPGo:**
- `product_information` (product_id, product_name, product_model, unit, price)
- `product_purchase_details` (quantity, rate, total_amount, batch_id)
- `invoice_details` (quantity, rate, total_price)
- `store_adjustment_details` (quantity, type)

**Endpoints:**
- `GET /reports/inventory-control` → Vista
- `GET /reports/inventory-control/data` → Datos JSON
- `GET /reports/inventory-control/print` → Versión imprimible

**Archivos:**
- `/templates/inventory_control.html`
- `/templates/inventory_control_print.html`

---

### 2.4 Módulo: Libro de Retenciones de IVA

**Descripción:** Registro de retenciones de IVA efectuadas a proveedores.

**Datos requeridos:**
- Número de comprobante de retención
- RIF del proveedor
- Número de factura retenida
- Base imponible
- Porcentaje de retención
- Monto retenido
- Fecha

**Fuente de datos en ERPGo:**
- Tabla nueva `vat_retention` (a crear en schema)
- Relación con `product_purchase`

**Endpoints:**
- `GET /reports/vat-retention` → Vista
- `GET /reports/vat-retention/data` → Datos JSON
- `POST /reports/vat-retention/save` → Guardar retención

**Archivos:**
- `/templates/vat_retention.html`
- `/templates/vat_retention_print.html`

---

### 2.5 Módulo: Libro de Retenciones de ISLR

**Descripción:** Registro de retenciones de Impuesto Sobre La Renta a proveedores.

**Datos requeridos:**
- Número de comprobante de retención
- RIF del proveedor
- Número de factura / servicio
- Base imponible
- Porcentaje de retención (2%, 5%, 10%, 15% según tarifa)
- Monto retenido
- Fecha

**Fuente de datos:**
- Tabla nueva `islr_retention` (a crear en schema)
- Tabla `tax_settings` para porcentajes configurables

**Endpoints:**
- `GET /reports/islr-retention` → Vista
- `GET /reports/islr-retention/data` → Datos JSON
- `POST /reports/islr-retention/save` → Guardar retención

**Archivos:**
- `/templates/islr_retention.html`
- `/templates/islr_retention_print.html`

---

## 3. Estructura de Archivos

### 3.1 Nuevo Archivo Go
```
/interfaces/web/legal_books.go
```

**Contenido:**
- `registerLegalBooksRoutes()` → Registra todas las rutas
- `bookPurchases()` → Vista del libro de compras
- `bookPurchasesData()` → Datos JSON
- `bookPurchasesPrint()` → Versión imprimible
- `bookSales()` → Vista del libro de ventas
- `bookSalesData()` → Datos JSON
- `bookSalesPrint()` → Versión imprimible
- `inventoryControl()` → Vista control de inventario
- `inventoryControlData()` → Datos JSON
- `inventoryControlPrint()` → Versión imprimible
- `vatRetention()` → Vista retenciones IVA
- `vatRetentionData()` → Datos JSON
- `vatRetentionSave()` → Guardar retención
- `islrRetention()` → Vista retenciones ISLR
- `islrRetentionData()` → Datos JSON
- `islrRetentionSave()` → Guardar retención

### 3.2 Nuevos Templates
```
/templates/book_purchases.html
/templates/book_purchases_print.html
/templates/book_sales.html
/templates/book_sales_print.html
/templates/inventory_control.html
/templates/inventory_control_print.html
/templates/vat_retention.html
/templates/vat_retention_print.html
/templates/islr_retention.html
/templates/islr_retention_print.html
```

### 3.3 Actualizaciones

**`server.go`:**
```go
s.registerLegalBooksRoutes()  // En NewServer()
```

**`layout.html` (menú lateral):**
```html
{{if index .Perm "legal_books"}}
<li class="treeview">
    <a href="#"><i class="fa fa-balance-scale"></i><span>Libros Legales</span>...</a>
    <ul class="treeview-menu">
        <li><a href="/reports/book-purchases">Libro de Compras</a></li>
        <li><a href="/reports/book-sales">Libro de Ventas</a></li>
        <li><a href="/reports/inventory-control">Control Inventario</a></li>
        <li><a href="/reports/vat-retention">Retenciones IVA</a></li>
        <li><a href="/reports/islr-retention">Retenciones ISLR</a></li>
    </ul>
</li>
{{end}}
```

**`admin.go` (sub_modules):**
```go
{"10", "legal_books", "legal_books"},
```

**`schema_extra.sql` (tablas nuevas):**
```sql
CREATE TABLE IF NOT EXISTS `vat_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `purchase_id` bigint DEFAULT NULL,
  `supplier_id` bigint DEFAULT NULL,
  `invoice_no` varchar(50) DEFAULT NULL,
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `vat_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `islr_retention` (
  `id` int NOT NULL AUTO_INCREMENT,
  `retention_no` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `supplier_id` bigint DEFAULT NULL,
  `concept` varchar(255) DEFAULT NULL,
  `base_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `retention_percent` decimal(5,2) NOT NULL DEFAULT '0.00',
  `retention_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `created_by` varchar(30) NOT NULL,
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

---

## 4. Orden de Implementación

### Fase 1: Libros Fiscales Básicos (Compras y Ventas)
1. Crear `schema_extra.sql` con tablas de retenciones
2. Crear `legal_books.go` con handlers básicos
3. Crear templates `book_purchases.html` y `book_purchases_print.html`
4. Crear templates `book_sales.html` y `book_sales_print.html`
5. Registrar rutas en `server.go`
6. Agregar menú en `layout.html`
7. Compilar y verificar

### Fase 2: Control de Inventario
1. Crear `inventory_control.html` e `inventory_control_print.html`
2. Implementar handlers en `legal_books.go`
3. Agregar lógica de costo promedio ponderado

### Fase 3: Retenciones (IVA e ISLR)
1. Crear templates de retenciones
2. Implementar handlers para guardar retenciones
3. Implementar versión imprimible
4. Agregar validaciones de porcentajes según normativa

### Fase 4: Integración y Pulido
1. Agregar filtros por período (mes/año fiscal)
2. Agregar totales por página (para impresión)
3. Verificar cálculos de IVA/ISLR según normativa vigente
4. Agregar exportación a PDF (si es posible)

---

## 5. Notas Importantes

- Los libros deben poder generar **archivos imprimibles** en formato compatible con hojas foliadas (formato carta/oficio)
- Los montos deben mostrarse en **Bolívares** (VES)
- Los registros cerrados (períodos anteriores) **no deben poder eliminarse**
- Las retenciones deben poder **asociarse a facturas** (compras/servicios)
- El sistema debe mantener **auditoría** de quién generó qué libro
- Los porcentajes de retención ISLR deben ser configurables (tabla tax_settings)

---

## 6. Estado Final Esperado

| Libro | Estado |
|-------|--------|
| Libro Diario | ✅ Existente |
| Libro Mayor | ✅ Existente |
| Libro de Inventarios y Balances | ✅ Existente |
| Libro de Compras (IVA) | 🆕 Por implementar |
| Libro de Ventas (IVA) | 🆕 Por implementar |
| Libro de Control de Inventario | 🆕 Por implementar |
| Libro de Retenciones IVA | 🆕 Por implementar |
| Libro de Retenciones ISLR | 🆕 Por implementar |

**Nota:** Los libros de Actas (Accionistas, Asamblea, Junta Directiva) son de naturaleza corporativa y quedan fuera del alcance de la migración del ERP operativo.
