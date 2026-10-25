# Informe de Análisis Completo: SalesERP v12.13
## Evaluación de Viabilidad para Migración a ERPGo (Go)

---

**Fecha**: 2026-09-26
**Versión analizada**: salesERP_mainv12.13
**Objetivo**: Análisis exhaustivo para reescritura completa en Go manteniendo funcionalidad, vistas y operación.

---

## ÍNDICE

1. [Resumen Ejecutivo](#1-resumen-ejecutivo)
2. [Arquitectura del Sistema](#2-arquitectura-del-sistema)
3. [Base de Datos](#3-base-de-datos)
4. [API y Controladores](#4-api-y-controladores)
5. [Modelos y Lógica de Negocio](#5-modelos-y-lógica-de-negocio)
6. [Interfaz de Usuario](#6-interfaz-de-usuario)
7. [Assets y JavaScript](#7-assets-y-javascript)
8. [Módulos de Negocio](#8-módulos-de-negocio)
9. [Seguridad](#9-seguridad)
10. [Integraciones Externas](#10-integraciones-externas)
11. [Plan de Migración Propuesto](#11-plan-de-migración-propuesto)
12. [Estimación de Esfuerzo](#12-estimación-de-esfuerzo)

---

## 1. RESUMEN EJECUTIVO

### Magnitud del Proyecto Original

| Aspecto | Cantidad |
|---------|----------|
| **Módulos de negocio** | 19 |
| **Tablas de base de datos** | 67 |
| **Controladores** | 3 (API principal + Offline + Installer) |
| **Modelos** | 3 archivos (~1550 líneas c/u) |
| **Vistas (templates)** | 311 archivos PHP |
| **Archivos JavaScript** | 43+ archivos |
| **Archivos CSS** | 28+ archivos |
| **Rutas registradas** | 150+ |
| **Librerías PHP personalizadas** | 10 |
| **Idiomas soportados** | Multi-idioma (DB-driven) |

### Viabilidad de Migración

✅ **Altamente viable** con consideraciones importantes:

1. **La lógica de negocio está centralizada** en solo 3 controladores y 3 modelos
2. **La UI es completamente replicable** (Bootstrap 3 + AdminLTE + jQuery)
3. **El dominio es bien entendido** (ERP clásico: ventas, compras, inventario, contabilidad, nómina)
4. **Las integraciones están identificadas** (SMS, PDF, email, IA)

⚠️ **Riesgos identificados**:
1. Código duplicado entre Api.php y Offline.php (~70% overlap)
2. Seguridad deficiente (MD5, CSRF desactivado, SQL injection potencial)
3. MyISAM vs InnoDB mixto (sin transacciones en tablas críticas)
4. Sin pruebas automatizadas en el proyecto original

---

## 2. ARQUITECTURA DEL SISTEMA

### Stack Tecnológico Actual

```
Backend:     PHP 7.1+ / CodeIgniter 3.x (HMVC)
Frontend:    Bootstrap 3.3.7 + jQuery 3.4.1 + AdminLTE
Base datos:  MySQL 8.0 (mysqli driver)
Servidor:    Apache (.htaccess mod_rewrite)
PDF:         DomPDF 3.0.2
Excel:       PHPExcel 1.x (deprecated)
SMS:         Nexmo API
QR Codes:    PHP QR Code library
Capchas:     GD-based
```

### Patrón HMVC

```
application/
├── controllers/      → API endpoints (Api.php, Offline.php)
├── models/           → Data access (Api_model, Offline_model, newApi_model)
├── modules/          → Módulos HMVC independientes
│   ├── invoice/      (controllers, models, views, config)
│   ├── customer/
│   ├── product/
│   ├── purchase/
│   ├── account/
│   ├── hrm/
│   ├── dashboard/
│   ├── bank/
│   ├── report/
│   ├── service/
│   ├── return/
│   ├── supplier/
│   ├── quotation/
│   ├── tax/
│   ├── payrol/
│   ├── addon/
│   ├── autoupdate/
│   └── template/     → Layouts base
├── libraries/        → Utilidades personalizadas
├── helpers/          → Funciones auxiliares
└── config/           → Configuración CI
```

### Arquitectura Propuesta para ERPGo

```
erpgow/
├── cmd/
│   └── server/          → Punto de entrada
├── internal/
│   ├── domain/          → Entidades de negocio
│   │   ├── invoice/
│   │   ├── product/
│   │   ├── customer/
│   │   ├── accounting/
│   │   └── ...
│   ├── application/     → Casos de uso / servicios
│   │   ├── sales/
│   │   ├── purchases/
│   │   ├── inventory/
│   │   └── ...
│   ├── infrastructure/  → Adaptadores
│   │   ├── persistence/ → Repositorios (GORM/sqlx)
│   │   ├── http/        → Handlers REST
│   │   ├── pdf/         → Generación PDF
│   │   ├── sms/         → Gateway SMS
│   │   └── email/       → SMTP
│   └── interfaces/      → Capa de presentación
│       ├── web/         → HTML templates
│       └── api/         → JSON REST
├── pkg/                 → Utilidades compartidas
├── migrations/          → Migraciones SQL
├── templates/           → Vistas HTML (replicadas 1:1)
├── static/              → CSS/JS/Fonts
├── configs/             → Archivos de configuración
└── deployments/         → Docker, scripts
```

---

## 3. BASE DE DATOS

### Inventario Completo de Tablas (67 tablas)

#### 3.1 Contabilidad (8 tablas)

| Tablas | Motor | Descripción |
|--------|-------|-------------|
| `acc_coa` | InnoDB | Plan de cuentas (jerárquico) |
| `acc_monthly_balance` | InnoDB | Balances mensuales por cuenta |
| `acc_opening_balance` | InnoDB | Saldos iniciales por ejercicio |
| `acc_predefine_account` | InnoDB | Mapeo de cuentas predefinidas |
| `acc_subcode` | InnoDB | Sub-códigos (clientes/proveedores) |
| `acc_subtype` | InINNDB | Clasificación (Empleado, Cliente, Proveedor, Agente) |
| `acc_transaction` | InINNDB | Transacciones del libro mayor |
| `acc_vaucher` | InINNDB | Comprobantes (débito/crédito/contra/diario) |

#### 3.2 Ventas y Facturación (4 tablas)

| Tablas | Motor | Descripción |
|--------|-------|-------------|
| `invoice` | **MyISAM** | Cabeceras de factura de venta |
| `invoice_details` | **MyISAM** | Líneas de productos facturados |
| `service_invoice` | InINNDB | Facturas de servicio |
| `service_invoice_details` | InINNDB | Líneas de servicio facturado |

⚠️ **PROBLEMA**: `invoice` y `invoice_details` usan MyISAM. Sin transacciones ni foreign keys.

#### 3.3 Compras (3 tablas)

| Tablas | Motor | Descripción |
|--------|-------|-------------|
| `product_purchase` | **MyISAM** | Órdenes de compra |
| `product_purchase_details` | **MyISAM** | Líneas de compra |
| `supplier_product` | InINNDB | Mapeo producto-proveedor con precio |

#### 3.4 Productos e Inventario (3 tablas)

| Tablas | Motor | Descripción |
|--------|-------|-------------|
| `product_category` | InINNDB | Categorías de productos |
| `product_information` | **MyISAM** | Maestro de productos |
| `units` | InINNDB | Unidades de medida |

#### 3.5 Clientes y Proveedores (2 tablas)

| Tablas | Motor | Descripción |
|--------|-------|-------------|
| `customer_information` | **MyISAM** | Maestro de clientes |
| `supplier_information` | **MyISAM** | Maestro de proveedores |

#### 3.6 Nómina / RRHH (12 tablas)

| Tablas | Descripción |
|--------|-------------|
| `attendance` | Registros de asistencia |
| `designation` | Designaciones/puestos |
| `employee_history` | Datos maestros de empleados |
| `employee_salary_payment` | Pagos de salario |
| `employee_salary_setup` | Configuración de componentes salariales |
| `gmb_salary_advance` | Adelantos de salario |
| `gmb_salary_generate` | Registros de generación de salario |
| `gmb_salary_sheet_generate` | Lotes de nómina |
| `salary_setup_header` | Cabeceras de configuración salarial |
| `salary_sheet_generate` | Lotes de nómina (legacy) |
| `salary_type` | Tipos de componentes salariales |
| `payroll_tax_setup` | Tramos de impuesto sobre la renta |

#### 3.7 Banca (2 tablas)

| Tablas | Descripción |
|--------|-------------|
| `bank_add` | Maestro de cuentas bancarias |
| `daily_banking_add` | Transacciones bancarias diarias |

#### 3.8 Impuestos y Gastos (4 tablas)

| Tablas | Descripción |
|--------|-------------|
| `tax_settings` | Configuración de tasas impositivas |
| `vat_tax_setting` | Configuración de IVA |
| `tax_collection` | Registros de recaudación de impuestos |
| `expense` / `expense_item` | Comprobantes y categorías de gastos |

#### 3.9 Préstamos (4 tablas)

| Tablas | Descripción |
|--------|-------------|
| `personal_loan` | Transacciones de préstamos personales |
| `person_information` | Maestro de personas (prestatarios) |
| `person_ledger` | Libro mayor de préstamos |
| `pesonal_loan_information` | Detalles de préstamo (legacy, typo en nombre) |

#### 3.10 Cierre y Reportes (2 tablas)

| Tablas | Descripción |
|--------|-------------|
| `daily_closing` | Cierre diario de caja |
| `closing_records` | Instantáneas de cierre contable |

#### 3.11 Sistema y Configuración (18 tablas)

| Tablas | Descripción |
|--------|-------------|
| `company_information` | Perfil de la empresa |
| `currency_tbl` | Definiciones de moneda |
| `email_config` | Configuración SMTP |
| `financial_year` | Ejercicios fiscales |
| `language` | Frases de localización (1098 frases en BD) |
| `module` / `sub_module` | Registro de módulos (145 submódulos) |
| `role_permission` / `sec_role` / `sec_userrole` | RBAC (control de acceso basado en roles) |
| `setting` / `web_setting` / `app_setting` | Configuración de aplicación |
| `sms_settings` / `print_setting` | SMS e impresión |
| `synchronizer_setting` | Sincronización FTP |
| `users` / `user_login` | Usuarios y credenciales |
| `activity_logs` | Auditoría |
| `ai_setting` / `ai_report_types` | Integración OpenAI |
| `module_purchase_key` | Licencias de módulos |
| `seles_termscondi` | Términos y condiciones de venta |
| `quotation` / `quot_products_used` / `quotation_service_used` / `quotation_taxinfo` | Cotizaciones |

### 3.12 Recomendaciones para ERPGo

1. **Migrar MyISAM a InINNDB** → Añadir foreign keys y transacciones reales
2. **Normalizar nombres** → `pesonal_loan_information` tiene typo
3. **Añadir timestamps** → Muchas tablas carecen de `created_at`/`updated_at`
4. **Soft deletes** → Implementar `deleted_at` en lugar de DELETE físico
5. **UUID vs Auto-increment** → Evaluar IDs UUID para multi-tenant

---

## 4. API Y CONTROLADORES

### Controlador Principal: Api.php (3052 líneas)

**Patrón**: REST-like pero usa GET para escrituras (problema de seguridad).

#### Módulos Funcionales:

| Métodos | Módulo | Operaciones |
|---------|--------|-------------|
| 9 | Autenticación | Registro, login, cambio password, recuperación |
| 7 | Productos | CRUD + cálculo stock + QR |
| 5 | Categorías | CRUD completo |
| 8 | Clientes | CRUD + crédito/débito + búsqueda |
| 7 | Proveedores | CRUD + búsqueda |
| 5 | Unidades | CRUD |
| 5 | Ventas | Insertar factura + SMS + búsqueda |
| 6 | Compras | CRUD + búsqueda por fecha |
| 10 | Contabilidad | Comprobantes, libros, reportes |
| 7 | Stock | Reportes por producto/proveedor |
| 4 | Pagos/Suscripción | Checkout, duración |
| 2 | Sistema | Backup BD, búsqueda productos |

**Operación Crítica: insert_sale()**
```
1. Genera número de factura (max+1 desde 1000)
2. INSERT en `invoice`
3. INSERT múltiple en `acc_transaction`:
   - Débito a cliente (10203...)
   - Crédito a ingresos por ventas
   - Crédito a inventario
   - Débito/crédito por impuestos
4. INSERT en `invoice_details` (por cada línea)
5. Si está habilitado: envía SMS via Nexmo
```

**Operación Crítica: insert_purchase()**
```
1. INSERT en `product_purchase`
2. 4 INSERTs en `acc_transaction`:
   - Débito a inventario (1141)
   - Crédito a proveedor
   - Crédito a caja (111000001)
   - Débito a proveedor (pago)
3. INSERT en `product_purchase_details` (por línea)
```

### Controlador Offline.php (1895 líneas)

**Propósito**: Sincronización offline/POS con solapamiento del 70% con Api.php.

**Diferencias clave**:
- Filtrado por `is_online` (1=web, 2=offline)
- Reportes adicionales (ventas por producto, compras por producto, deudas)
- Métodos de configuración (user_list, bank_list, language_list)

### Controlador Installer.php

**Propósito**: Limpieza post-instalación, elimina directorio `/install`.

### Endpoints REST para ERPGo

El mapeo propuesto de endpoints JSON para ERPGo:

```
POST   /api/v1/auth/login
POST   /api/v1/auth/register
PUT    /api/v1/auth/password
POST   /api/v1/auth/recover

GET    /api/v1/products
POST   /api/v1/products
PUT    /api/v1/products/:id
DELETE /api/v1/products/:id
GET    /api/v1/products/:id/stock

GET    /api/v1/customers
POST   /api/v1/customers
PUT    /api/v1/customers/:id
DELETE /api/v1/customers/:id
GET    /api/v1/customers/:id/ledger

GET    /api/v1/suppliers
POST   /api/v1/suppliers
PUT    /api/v1/suppliers/:id
DELETE /api/v1/suppliers/:id

POST   /api/v1/invoices
GET    /api/v1/invoices
GET    /api/v1/invoices/:id
PUT    /api/v1/invoices/:id
DELETE /api/v1/invoices/:id
GET    /api/v1/invoices/:id/pdf
POST   /api/v1/invoices/:id/sms

POST   /api/v1/purchases
GET    /api/v1/purchases
PUT    /api/v1/purchases/:id

GET    /api/v1/accounts/coa
POST   /api/v1/accounts/vouchers
GET    /api/v1/accounts/ledger
GET    /api/v1/accounts/reports/profit-loss
GET    /api/v1/accounts/reports/balance-sheet

GET    /api/v1/reports/stock
GET    /api/v1/reports/sales
GET    /api/v1/reports/purchases
GET    /api/v1/reports/daily-closing

GET    /api/v1/settings
PUT    /api/v1/settings
```

---

## 5. MODELOS Y LÓGICA DE NEGOCIO

### 5.1 Api_model.php (1550 líneas, 77 métodos)

**Patrón**: Active Record de CI ($this->db->query).

**Métodos clave y lógica**:

| Método | Complejidad | Descripción |
|--------|-------------|-------------|
| `get_total_product($id)` | Media | 3 consultas: SUM compras, SUM ventas, datos producto |
| `product_stock()` | Alta | Stock con subconsultas: `totalBuyQnty - totalSalesQnty` |
| `customer_list()` | Media | Clientes con subquery de balance (SUM débitos - SUM créditos) |
| `purchase_entry()` | Alta | INSERT purchase + 4x acc_transaction + loop details |
| `supplier_payment_insert()` | Alta | Doble partida: débito proveedor + crédito caja |
| `profit_loss_report()` | Media | SUM por tipo cuenta (Income/Expense) agrupado |
| `suppliers_ledger()` | Media | Transacciones filtradas por proveedor + rango fechas |
| `taxfield()` | Baja | Campos dinámicos de impuestos |

**Cálculo de stock (patrón repetido)**:
```sql
SELECT 
  a.unit, a.product_name, a.product_id, a.price, a.product_model,
  (SELECT SUM(quantity) FROM invoice_details WHERE product_id = a.product_id) as totalSalesQnty,
  (SELECT SUM(quantity) FROM product_purchase_details WHERE product_id = a.product_id) as totalBuyQnty
FROM product_information a
GROUP BY product_id
-- Stock = totalBuyQnty - totalSalesQnty
```

**Cálculo de balance cliente (subquery)**:
```sql
SELECT a.*, 
  (SELECT IFNULL(SUM(Debit),0) FROM acc_transaction WHERE COAID=b.HeadCode AND IsAppove=1) -
  (SELECT IFNULL(SUM(Credit),0) FROM acc_transaction WHERE COAID=b.HeadCode AND IsAppove=1) as customer_balance
FROM customer_information a
LEFT JOIN acc_coa b ON b.customer_id = a.customer_id
GROUP BY customer_id
```

### 5.2 Offline_model.php (1621 líneas, 80 métodos)

**Diferencias con Api_model**:
- Filtrado `is_online` en ventas
- 5 INSERTs en `acc_transaction` en compras (añade gasto empresa COAID=402)
- Escribe archivos JSON en disco (`supplier.json`, `customer.json`) como caché
- Métodos adicionales: `settings()`, `user_list()`, `bank_list()`, `language_list()`, `phrases()`, `check_user()`

### 5.3 newApi_model.php (1538 líneas, 74 métodos)

**Diferencias**:
- Usa tabla `company_info` (no `company_information`)
- `supplier_advance_insert` inserta en `supplier_ledger`
- `previous_balance_add` incluye débito a inventario (COAID=1141)
- Tiene `send_sms()` que lee `sms_settings`
- Sin `retrieve_setting_editdata`, sin `customerheadcode`

### 5.4 Modelado Propuesto para ERPGo (GORM)

```go
// Entidades principales
type Product struct {
    ProductID   string  `gorm:"primaryKey"`
    Name        string
    CategoryID  *string
    Unit        string
    TaxRate     float64
    Price       float64
    Model       string
    Description string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Invoice struct {
    InvoiceID    string    `gorm:"primaryKey"`
    CustomerID   string
    Customer     Customer  `gorm:"foreignKey:CustomerID"`
    InvoiceDate  time.Time
    GrandTotal   float64
    TotalDiscount float64
    TotalTax     float64
    PaidAmount   float64
    DueAmount    float64
    Details      []InvoiceDetail
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// Repositorios (interfaces)
type InvoiceRepository interface {
    Create(ctx context.Context, inv *Invoice) error
    GetByID(ctx context.Context, id string) (*Invoice, error)
    List(ctx context.Context, filter InvoiceFilter) ([]Invoice, error)
    GetStock(ctx context.Context, productID string) (int, error)
}
```

---

## 6. INTERFAZ DE USUARIO

### 6.1 Estructura de Layout

**Layout Principal** (`application/modules/template/views/layout.php`):
```html
<html>
<body class="hold-transition sidebar-mini">
  <div class="wrapper">
    <?php $this->load->view('template/includes/head') ?>
    <?php $this->load->view('template/includes/header') ?>
    <?php $this->load->view('template/includes/sidebar') ?>
    <!-- Breadcrumb -->
    <?php $this->load->view('template/includes/messages') ?>
    <!-- Contenido dinámico: module/page -->
    <?php $this->load->view($module.'/'.$page) ?>
    <?php $this->load->view('template/includes/js') ?>
    <!-- Modales: #calculator, #cust_info -->
  </div>
</body>
</html>
```

**Layout POS** (`posLayout.php`):
- Sin sidebar, sin header
- Interfaz fullscreen para terminal punto de venta
- Mismos inputs ocultos (base_url, CSRF_TOKEN, currency, discount_type)

### 6.2 Sistema de Navegación (Sidebar)

14 secciones principales con control de permisos:

```
1.  Dashboard              → home
2.  Invoice               → Nueva, Gestionar, POS, Términos
3.  Customer              → Añadir, Listar, Crédito, Pagado, Mayor, Anticipo
4.  Supplier              → Añadir, Listar, Mayor, Anticipo
5.  Product               → Categoría, Unidad, Añadir, CSV, Gestionar
6.  Purchase              → Añadir, Gestionar
7.  Stock                 → Reporte
8.  Accounts              → COA, Saldo inicial, Comprobantes, Ajustes, Reportes, Métodos pago
9.  HRM                   → Empleado, Designación, Asistencia, Nómina, Préstamos, Adelanto
10. Service               → Factura, Pago
11. Quotation             → Añadir, Gestionar
12. Return                → Devolución cliente, Devolución proveedor, Desperdicio
13. Tax                   → Configuración, Reportes
14. Settings              → Empresa, Moneda, Usuarios, Roles, Backup, Sync
```

### 6.3 Vistas Clave y Su Funcionalidad

#### Dashboard (home.php)
- 4 tarjetas de estadísticas (total clientes, productos, proveedores, facturas)
- Gráfico Chart.js: Resumen anual ventas/compras (líneas)
- Gráfico Chart.js: Productos más vendidos (barras)
- Gráfico CanvasJS/Chart.js: Estado de gastos (doughnut)
- Tablas: Ventas vencidas, Compras vencidas, Ventas del día

#### Factura POS (gui_pos_invoice_dynamic.php)
- Layout POS completo
- Búsqueda de productos con grid de imágenes
- Escaneo de código de barras (keyboard wedge)
- Modal de cliente nuevo rápido
- Modal de calculadora
- Procesamiento de pago rápido

#### Configuración (setting.php)
- Logo de empresa, logo de factura, favicon
- Moneda, zona horario, idioma
- Toggle RTL/LTR
- Tipo de descuento, QR code toggle, auto-aprobar comprobantes

#### Login (login.php)
- Página independiente (fuera del layout)
- Google reCAPTCHA
- Modal de recuperación de contraseña

### 6.4 Replicación en ERPGo

**Opción A: Server-Side Rendering con html/template**
```go
// templates/dashboard.html
{{template "layout.html" .}}
{{define "content"}}
<div class="row">
  {{range .Stats}}
  <div class="col-lg-3 col-xs-6">
    <div class="small-box bg-{{.Color}}">
      <div class="inner">
        <h3>{{.Value}}</h3>
        <p>{{.Label}}</p>
      </div>
    </div>
  </div>
  {{end}}
</div>
{{end}}
```

**Opción B: SPA con framework JS (Vue/React/Svelte) + API REST**
- APIs REST completas (sección 4)
- Frontend moderno replicando mismo diseño visual
- Mayor trabajo inicial pero mejor UX

**Opción C: Híbrida (Recomendada)**
- html/template para páginas de gestión (CRUD, reportes, configuración)
- Componentes SPA para POS y operaciones en tiempo real
- Mismo CSS/JS para mantener apariencia visual

---

## 7. ASSETS Y JAVASCRIPT

### 7.1 Librerías Core

| Librería | Versión | Uso |
|----------|---------|-----|
| jQuery | 3.4.1 | DOM, AJAX, eventos |
| Bootstrap | 3.3.7 | Grid, componentes, modales |
| jQuery UI | - | Datepicker, sortable, autocomplete |
| DataTables | 1.x | Tablas con servidor (1000+ registros) |
| Select2 | - | Dropdowns avanzados con búsqueda |
| Chart.js | 2.5 | Gráficos de barras y líneas |
| CanvasJS | - | Gráficos de torta (legacy) |
| SweetAlert | - | Diálogos de confirmación |
| Toastr | - | Notificaciones no bloqueantes |
| jsTree | - | Árbol de plan de cuentas |
| Mousetrap | - | Atajos de teclado |
| Summernote | - | Editor WYSIWYG |
| Flot | - | Gráficos canvas (alternativa) |
| Pace | - | Barra de progreso de carga |
| jQuery.print | - | Impresión de elementos |
| Bootstrap Toggle | - | Interruptores on/off |
| WickedPicker | - | Selector de hora |

### 7.2 Funcionalidad JavaScript Clave

#### Facturación (invoice.js, invoice_update.js - 1653+ líneas)
```javascript
// Dinámico: Añadir campos de producto
function addInputField_invoice() {
  // Añade fila con: producto, precio, cantidad, descuento, impuesto, subtotal
}

// Cálculo en tiempo real
function bdtask_invoice_quantity_calculate() {
  // qty * price - discount + tax = line_total
  // Actualiza sumatorio global
}

// Validación de stock
function stockLimit(qty, product_id) {
  $.post(base_url + 'invoice/invoice/product_stock_check', {product_id: qty}, function(res) {
    if (res.available < qty) {
      toastr["error"]("Stock insuficiente");
      return false;
    }
  });
}

// Modal de pago con múltiples métodos
$('#payment_modal').on('show.bs.modal', function() {
  $.post(base_url + 'invoice/invoice/bdtask_showpaymentmodal', function(html) {
    $(this).find('.modal-body').html(html);
  });
});
```

#### POS (pos_invoice.js, guibarcode.js)
```javascript
// Escaneo de código de barras (keyboard wedge - 300ms timer)
let barcodeTimer = null;
let barcodeString = '';
$(document).keypress(function(e) {
  clearTimeout(barcodeTimer);
  barcodeString += String.fromCharCode(e.which);
  barcodeTimer = setTimeout(function() {
    if (barcodeString.length > 5) {
      addProductByBarcode(barcodeString);
    }
    barcodeString = '';
  }, 300);
});

// Auto-incrementar si producto ya existe en factura
function addProductByBarcode(barcode) {
  let existing = $('input[value="' + barcode + '"]');
  if (existing.length > 0) {
    let qtyInput = existing.closest('tr').find('.qty');
    qtyInput.val(parseInt(qtyInput.val()) + 1);
    bdtask_invoice_quantity_calculate();
  } else {
    $.post(base_url + 'invoice/invoice/gui_pos_invoice', {barcode}, addInvoiceRow);
  }
}
```

#### DataTables con procesamiento en servidor
```javascript
$('#invoiceTable').DataTable({
  serverMethod: 'post',
  ajax: {
    url: base_url + 'invoice/invoice/CheckInvoiceList',
    type: 'POST',
    data: function(d) {
      d.csrf_test_name = $('#CSRF_TOKEN').val();
    }
  },
  lengthMenu: [10, 25, 50, 100, 250, 500, 1000],
  dom: 'Bfrtip',
  buttons: ['copy', 'csv', 'excel', 'pdf', 'print'],
  footerCallback: function(row, data, start, end, display) {
    // Sumatorio de columnas monetarias
    api.column(5).footer().html(
      $.fn.dataTable.render.number(',', '.', 2, '$').display(total)
    );
  }
});
```

#### Plan de Cuentas (jsTree)
```javascript
$('#coaTree').jstree({
  'core': {
    'data': {
      'url': base_url + 'account/account/get_tree',
      'dataType': 'json'
    }
  },
  'plugins': ['contextmenu', 'dnd', 'search']
}).on('select_node.jstree', function(e, data) {
  loadAccountForm(data.node.id);
});
```

### 7.3 Migración de Frontend

**Estrategia recomendada para ERPGo**:

1. **Reutilizar assets**: Bootstrap, jQuery, DataTables, Chart.js son independientes del backend
2. **Reemplazar URLs**: Cambiar `base_url + 'module/controller/method'` por `fetch('/api/v1/...')`
3. **Plantillas HTML**: Migrar vistas PHP a html/template de Go
4. **Mantener CSRF**: Go puede usar gorilla/csrf o similar

```go
// Handler con template
func DashboardHandler(w http.ResponseWriter, r *http.Request) {
    stats := dashboardService.GetStats(r.Context())
    tmpl := template.Must(template.ParseFiles(
        "templates/layout.html",
        "templates/dashboard.html",
    ))
    tmpl.Execute(w, stats)
}
```

---

## 8. MÓDULOS DE NEGOCIO — ANÁLISIS COMPLETO

### 8.1 Tabla Resumen de Módulos (18 HMVC + API)

| Módulo | Controlador | Tamaño | Funcionalidad |
|--------|------------|--------|---------------|
| **invoice** | Invoice.php | 101 KB | Venta, POS, PDF, email, entrega, términos, IA |
| **account** | Accounts.php | 200 KB | COA, comprobantes, pagos, conciliación, reportes |
| **hrm** | Employee.php | Grande | Empleados, asistencia, nómina, préstamos |
| **report** | Report.php | Grande | 20+ reportes, Excel/PDF |
| **purchase** | Purchase.php | Mediana | Órdenes compra, lotes, vencimientos |
| **customer** | Customer.php | Mediana | CRM, crédito, mayor, anticipos |
| **product** | Product.php | Mediana | Productos, CSV, QR, barcode |
| **supplier** | Supplier.php | Mediana | CRM proveedores, mayor |
| **service** | Service.php | Mediana | Facturación servicios |
| **return** | Return.php | Mediana | Devoluciones cliente/proveedor |
| **quotation** | Quotation.php | 78 KB | Cotizaciones → venta |
| **bank** | Bank.php | Pequeña | Cuentas bancarias, transacciones |
| **tax** | Tax.php | Pequeña | Configuración impuestos |
| **dashboard** | Home.php | Grande | KPIs, settings, usuarios, roles, IA |
| **addon** | Addon.php | Pequeña | Gestión módulos externos |
| **autoupdate** | Autoupdate.php | 15 KB | Auto-actualización |
| **template** | - | - | Layouts, login, sidebar |
| **API** | Api.php (3052L) | REST móvil/offline |
| **Offline** | Offline.php (1895L) | Sincronización POS |

### 8.2 Módulo: Facturación / Invoice

**Controlador**: `Invoice.php` (101 KB) — El más complejo
**Modelo**: `Invoice_model.php` (67 KB)

**Tipos de venta/pago**:
| Tipo | payment_type | Descripción |
|------|-------------|-------------|
| Cash | 1 | Pago efectivo inmediato |
| Card | 2 | Tarjeta (método genérico) |
| Crédito | 3 | Venta a crédito (pago posterior) |
| Multi-pago | 4 | División entre varios métodos |
| Borrador | 5 | Draft, puede completarse después |

**Funcionalidades clave**:

1. **Factura estándar** (`add_invoice`):
   - Cliente (autocompleta o creación instantánea)
   - Productos (búsqueda por nombre/modelo, batch/expiry)
   - Por línea: cantidad, precio, descuento (% o fijo), subtotal
   - Impuestos dinámicos (tax0, tax1, tax2... configurables)
   - Descuentos a nivel factura
   - Costo de envío
   - Términos y condiciones
   - Múltiples métodos de pago

2. **GUI POS** (`bdtask_gui_pos`):
   - Grid de productos por categoría con imágenes
   - Búsqueda instantánea de productos
   - Escaneo de código de barras (keyboard wedge)
   - Cliente caminante (walk-in) configurable
   - Modal de descuento a nivel factura
   - Modal de costo de envío
   - Modal de pago múltiple (cálculo cambio)
   - Calculadora en pantalla
   - Atajos de teclado (Mousetrap)
   - Pestaña "ventas en proceso de hoy"
   - Validación de stock en tiempo real
   - Impresión térmica directa

3. **Contabilidad automática**:
   ```
   insert_sale_creditvoucher:
     Débito: Cliente/Banco (10203xxx o 11200000x)
     Crédito: Ingresos por ventas (4xxxxx)
   
   insert_sale_inventory_voucher:
     Débito: Costo de ventas (5xxxxx)
     Crédito: Inventario (1141)
   
   insert_sale_taxvoucher:
     Débito: (por cobrar)
     Crédito: IVA a pagar (2xxxxx)
   ```
   - `autoapprove($invoice_id)` respeta `web_setting.is_autoapprove_v`

4. **Impresión/PDF/Email**:
   - Pad print con header/footer configurables (mm)
   - POS print (térmica 80mm)
   - PDF vía DomPDF
   - Email con PDF adjunto vía SMTP
   - Amount in words vía `Numbertowords`

5. **Delivery Note**:
   - Creación, guardado, vista HTML
   - Relacionado con factura

6. **IA** (`test_ai_summary`):
   - Resumen de factura individual via OpenAI

### 8.3 Módulo: Compras / Purchase

**Funcionalidades**:
- CRUD órdenes de compra
- Número Chalan (albarán proveedor)
- Búsqueda productos filtrada por proveedor
- Precio específico por proveedor (`supplier_product`)
- Líneas con: batch_id, fecha vencimiento, descuento %, descuento monto, IVA% + monto
- Pago: efectivo/crédito/banco
- Débito automático a contabilidad:
  ```
  insert_purchase_debitvoucher:
    Débito: Inventario (1141)
    Crédito: Proveedor (2010xxx)
  ```
- Auto-aprueba comprobantes

### 8.4 Módulo: Productos / Inventory

**Maestros**:
- Categorías CRUD
- Unidades de medida CRUD

**Productos CRUD**:
- Imagen (upload via `Fileupload` → `uploads/product`)
- Modelo, número serie, precio, % IVA, % tax
- Campos dinámicos de impuestos (tax0, tax1)
- Código de barras automático (product_id)

**Importación masiva**:
- CSV bulk import (`bdtask_csv_product` / `uploadCsv`)
- Descarga archivo de muestra

**Códigos**:
- **Barcode**: `vendor/barcode.php` renderer + página de impresión
- **QR**: `Ciqrcode` (phpqrcode port) → PNG en `my-assets/image/qr/`

**Detalle de producto**:
- Historial de compras
- Historial de ventas
- Stock calculado (compras - ventas)

**Sin almacén**: Stock es global, derivado de compras - ventas. No hay tabla de almacenes ni transferencias.

### 8.5 Módulo: Clientes / Customer

**Funcionalidades**:
- CRUD + búsqueda
- Lista de clientes con crédito (balance > 0)
- Lista de clientes pagados (balance <= 0)
- **Ledger interactivo** (AJAX `customer_ledgerData`, filtrable por fecha)
- **Anticipos** (depósitos) con recibo imprimible
- Creación de cliente en POS (instantánea)

**Integración contable**:
- Auto-crea cuenta COA por cobrar (`acc_coa`)
- HeadCode secuencia: `113100000001`
- FK: `acc_coa.customer_id`

**Cálculo balance**:
```
customer_balance = SUM(Debit) - SUM(Credit) 
  FROM acc_transaction 
  WHERE COAID = acc_coa.HeadCode AND IsAppove = 1
```

### 8.6 Módulo: Proveedores / Supplier

**Funcionalidades**:
- CRUD + búsqueda
- Ledger interactivo (AJAX)
- Anticipos con recibo
- Información detallada por proveedor

**Integración contable**:
- Auto-crea cuenta COA por pagar (`acc_coa`)
- HeadCode secuencia para proveedores
- FK: `acc_coa.supplier_id`

**Tabla relacional**:
- `supplier_product` (N:M): producto-proveedor con precio específico

### 8.7 Módulo: Devoluciones / Return

**Tipos**:
| Tipo | Tabla | Descripción |
|------|-------|-------------|
| Devolución cliente | `product_return` | Desde factura de venta |
| Devolución proveedor | `product_return` | Hacia proveedor |
| Desperdicio | `product_return` | Daños (flag `usablity`) |

**Campos de devolución**:
- Cantidad comprada vs devuelta
- Deducción (monto)
- Total deducido
- Impuesto
- Neto devolución
- Razón/texto

**Compensación**: 
- `ret_adjust_amnt` y `returnable_amount` en factura
- Crédito compensa facturas futuras

**Contabilidad**: Auto-posting + autoaprobación + eliminación

### 8.8 Módulo: Cotizaciones / Quotation

**Controlador**: 78 KB (complejo)

**Funcionalidades**:
- Creación con productos Y servicios mezclados
- Edición, eliminación, gestión, vista detallada
- Numeración automática (`quot_number_generator`)
- Fecha de vencimiento
- Subtotales separados: productos vs servicios
- Descuentos separados
- VAT/tax separados

**Conversión a venta**:
```
quotation_to_sales / add_quotation_to_invoice
  → Genera invoice + service_invoice
  → Postea: insert_sale_creditvoucher, insert_sale_inventory_voucher, 
            insert_sale_taxvoucher, insertservsale_*
```

**PDF + Email**: Descarga PDF, envío con PDF adjunto

**Visibilidad**: Flag `cust_show` (visible para cliente)

### 8.9 Módulo: Servicios / Service

**Catálogo de servicios CRUD**:
- Cargo/price, IVA, impuesto fijo/dinámico

**Factura de servicio**:
- Asignación de empleado (`employee_id` en `service_invoice`)
- Cliente, descuentos, IVA, pagado/deuda, costo envío
- Balance anterior

**Operaciones**: Editar, ver, PDF, descargar, eliminar, autoaprobación

**Contabilidad**:
- `insert_servsale_creditvoucher`
- `insert_servsale_taxvoucher`

**Cobro**: Pantalla de cobro en módulo Accounts (`service_payment`)

### 8.10 Módulo: Contabilidad / Account

**Controlador**: 200 KB | **Modelo**: 193 KB — El más grande

**Submódulos**:

#### a) Plan de Cuentas (COA)
- Vista de árbol (`show_tree`, `treeview.php`)
- CRUD cuentas y subcuentas
- Subtipos: None/Empleado/Cliente/Proveedor/Agente (`acc_subtype`)
- Subcódigos (`acc_subcode`)
- Cuentas predefinidas (`acc_predefine_account`): efectivo, banco, ventas, compras, inventario, COGS, IVA, salario, NPF/ICF, estatal

#### b) Ejercicio Fiscal
- CRUD años fiscales
- Cierre de año (`year_closing`)
- Revertir cierre de año
- Registros de cierre

#### c) Saldos Iniciales
- Crear/editar/eliminar por FY + COA
- Importar desde cierre

#### d) Comprobantes
| Tipo | Abrev | Débito | Crédito |
|------|-------|--------|---------|
| Débito | DV | Caja/Banco | Cliente |
| Crédito | CV | Proveedor | Caja/Banco |
| Contra | CONTRA | Caja | Banco |
| Diario | JV | Cuenta A | Cuenta B |

- Cada tipo: crear, listar (DataTables), editar, actualizar, eliminar, revertir
- Vista HTML + PDF individual
- Aprobación (`aprove_v`, `bulk_voucher_approve`)
- Filtro `IsAppove=1` en reportes (no aprobados excluidos)

#### e) Pagos
- Pago a proveedor (`create_supplier_payment` + recibo)
- Cobro a cliente (`create_customer_receive` + recibo)
- Pago de servicio
- Gestión de métodos de métodos de pago (cuentas COA tipo "Cash at Bank")
- Ajuste de caja
- Ajuste de balance

#### f) Conciliación Bancaria
- Pantalla conciliación
- Aprobación
- Reporte + exportación Excel

#### g) Cheques
- Campos: chequeNo, chequeDate, isHonour
- Reporte de estado de cheques

#### h) Cierre
- Cierre diario (en Report)
- Cierre anual con P&L transferido a ganancias retenidas

### 8.11 Módulo: Banca / Bank

**Funcionalidades**:
- CRUD cuentas bancarias
- Subida de imagen de firma (`uploads/bank`)
- Lista cuentas
- Transacciones (depósito/retiro via `daily_banking_add`)
- Gestión débito/crédito
- Ledger bancario

**Integración**: Auto-crea cuenta COA naturaleza bancaria

### 8.12 Módulo: Impuestos / Tax

**Configuraciones**:
- Configuración de impuestos (tasa default, nombre, número NT, registro, mostrar-en-factura)
- Modo VAT/Impuesto: fijo vs dinámico (`vat_tax_setting`)
- Tramos de impuesto sobre la renta (`payroll_tax_setup`: monto inicio/fin + tasa)
- Reporte de impuestos
- Reporte de impuestos por factura

### 8.13 Módulo: RRHH y Nómina / HRM

#### Empleados y Designaciones
- CRUD designaciones/puestos
- CRUD empleados con foto de perfil
- Perfil: tipo tarifa (hourly rate), tipo sanguíneo, país/ciudad

#### Asistencia
- Entrada manual (grid masiva por día)
- Sign-in/sign-out
- Tiempo de permanencia
- Edición, gestión
- **CSV bulk upload** (`attendance_bulkupload`)
- Reporte: fecha, por empleado

#### Nómina — Dos sistemas coexistentes:

**Legacy** (`employee_salary_setup`):
- Salario por empleado, tipos de componentes (% o fijo, adicionar/deducir)
- `salary_sheet_generate`, `employee_salary_payment`
- Payslip

**GMB** (`gmb_salary_generate`):
- Gross/basic/transport/house rent/medical/other
- Ingreso por impuesto, seguridad social NPF (5%/10%), contribuciones patronales, ICF
- Deducción de préstamo, adelanto de salario, LWP, PF, timbre
- Salario neto, tarifas horarias prorrateadas
- Aprobación workflow (`approved`, `approved_by`, `approved_date`)
- Gráfico salarial
- Tax renta estatal (`state_income_tax`)
- Auto-posting: `insert_payroll_journalvoucher`, `insert_payroll_debitvoucher`

#### Adelanto de Salario
- Solicitud, gestión, actualización, eliminación
- Validación por mes
- Auto-deducir en nómina

#### Gastos
- CRUD categorías de gasto
- CRUD entradas de gasto (fecha, tipo, comprobante no, monto)
- Estado de gastos

#### Préstamos — Dos tipos:
- **Préstamos oficina** (person + préstamo + pago + ledger + resumen)
- **Préstamos personales** (person + préstamo + pago + ledger + resumen)
- Búsqueda por teléfono/nombre

### 8.14 Módulo: Reportes / Report

**Reportes de Ventas**:
- Ventas del día
- Ventas por fecha
- Ventas por vendedor (usuario)
- Ventas por producto
- Ventas por categoría
- Facturas vencidas
- Costo de envío
- Mejores productos vendidos
- Devolución de ventas

**Reportes de Compras**:
- Reporte general
- Compras por categoría
- Devolución a proveedor

**Reportes Inventario**:
- Reporte stock (por producto, proveedor, global)
- Sin stock (dashboard)
- Ledger de inventario (contabilidad)

**Reportes Contables**:
- Libro caja
- Libro banco
- Day book (+detalle)
- Libro mayor
- Sub-ledger
- Balance de comprobación (con/sin apertura)
- Estado de resultados
- Estado de gastos
- P&L
- Balance general
- Schedule de activos fijos/depreciación
- Recibo y pago
- Conciliación bancaria
- Impresión COA
- Flujo de caja

**Reportes RRHH**:
- Asistencia (fecha, empleado)
- Estado de gastos
- Resúmenes préstamos
- Payslips
- Gráfico salarial

**Formatos**: HTML + PDF + Excel (PHPExcel) + Print (pad/POS/térmico)

### 8.15 Módulo: Dashboard / Admin

#### KPIs (home):
- Total clientes, productos, proveedores, ventas
- Ventas del día, deuda del día
- Compras del día, deuda del día
- Gráfico 12 meses (ventas/compras)
- Mejores productos
- Widget sin stock

#### Autenticación:
- Login con Google reCAPTCHA
- Logout
- Recuperación de contraseña (token + email)

#### Usuarios y Roles:
- CRUD usuarios (validación email único)
- Perfil y actualización
- Cambio de contraseño
- CRUD roles (`sec_role`)
- Asignación rol-usuario (`sec_userrole`)
- Matriz de permisos (`role_permission`: create/read/update/delete por submódulo)

#### Settings:
- Software (logo, moneda, timezone, footer, lenguaje, RTL, QR-on-invoice, auto-aprobar, captcha, descuento tipo)
- App/POS (title, tienda, dirección, email, phone, hora apertura/cierre, IVA default, service charge, date format, align)
- Compañía (nombre, email, dirección, móvil, website, VAT, CR)
- Email SMTP
- SMS (API key/secret/sender)
- Sincronización FTP
- Impresión (header/footer)
- Moneda CRUD
- Impuestos + VAT + tramos renta
- Cuentas predefinidas COA
- IA (API key, model, temperatura, tokens, prompt)
- Configuración impresión (Padprint)

#### Gestión de Idiomas:
- Añadir idioma
- Añadir/editar frases (`language` tabla)

#### Backup/Restore:
- Backup BD (descarga)
- Restore
- Importar SQL
- Eliminar backup
- Sincronización FTP offline (`SyncManager`)

#### Analista IA de Negocio:
- `AI_Config` (OpenAI API key, model default `gpt-3.5-turbo`, temperatura, max tokens, prompt template)
- Tipos de reporte: Ventas, Compra, Gráfico, Stock, Devolución Ventas, Devolución Compras, Top 10 Productos, Resumen Tabla
- Genera análisis texto + datasets de gráficos desde datos reales
- CRUD tipos de reporte

#### Calculadora de Comisiones:
- Comisión cliente/producto (rate × qty)

### 8.16 Módulo: Add-ons / Actualizaciones

- Gestor de módulos descargados
- Consulta servidor BDTASK para módulos disponibles
- Subida ZIP de módulo con verificación purchase-key
- Descompresión en `application/modules/`
- Activar/desactivar módulos
- Gestor de temas (`Theme.php`, `Themes_model`)
- Auto-actualización (`Autoupdate.php` contra servidor vendor)

### 8.17 Módulo: Plantillas / Template

- `layout.php` — Layout admin principal
- `login.php` — Página login independiente
- `posLayout.php` — Layout POS sin sidebar
- Includes: `head.php`, `header.php`, `sidebar.php`, `js.php`, `messages.php`

### 8.18 API REST (Api.php + Offline.php)

**Api.php** (84 endpoints móviles):
- Registro/login basado en dispositivo
- Company info
- CRUD productos/categorías/unidades/clientes/proveedores
- Listas crédito/pagado
- Info producto por barcode
- Stock, reportes stock
- Insertar venta + compra
- Listas ventas/compras + búsqueda
- Comprobantes pago proveedor + inserción
- Comprobantes cobro cliente + inserción
- Búsqueda libro mayor
- Reporte P&R
- Reportes producto ventas/compras
- Reporte deudas
- SMS factura
- Recuperación contraseña

**Offline.php** (55 endpoints POS offline):
- Mismo catálogo
- Insertar/actualizar venta offline
- Listas ventas online/hoy
- Sincronización FTP (`SyncManager` sube/descarga `backup.sql`)

### 8.19 Relaciones entre Módulos (Grafo de Dependencias)

```
Customer ──┐
Product ───┼──→ Invoice ──→ Account (acc_transaction)
Supplier ─┘    │              │
               ├──→ PDF      ├──→ COA (acc_coa)
               ├──→ Email    ├──→ Reports
               └──→ SMS      └──→ Daily Closing
               
Supplier ───→ Purchase ──→ Account
Product ────→ │
               
Quotation ──→ Invoice (conversión)
Service ────→ ServiceInvoice ──→ Account
               
Employee ───→ Payroll ──→ Account
               
Return ─────→ Invoice (compensación)
Return ─────→ Purchase
               
Bank ───────→ Account
```

### 8.20 Notas Clave para Migración a Go

1. **Ledger-centric**: balances cliente/proveedor/banco no son columnas almacenadas, se calculan desde `acc_transaction` vía COA auto-creado. Migración debe preservar enlace COA↔entidad.

2. **Stock derivado**: `stock = SUM(compras) - SUM(ventas) - SUM(devoluciones)`. Con batch + expiry en líneas pero sin tabla stock ledger. Sin almacenes.

3. **Aprobación de comprobantes**: Controla visibilidad financiera (`IsAppove=1`) y editabilidad facturas.

4. **Dos motores nómina**: Legacy `employee_salary_*` y GMB `gmb_salary_*` con NPF/ICF/tributos locales (parece reglas de seguridad social de un país específico).

5. **Tres superficies API**: Web HMVC, REST móvil (Api.php), POS offline (Offline.php + FTP SyncManager).

6. **Passwords MD5**, CSRF por config CI, `.htaccess` guards — seguridad a modernizar.

7. **Código heredado**: `Invoice_backup.php` (99 KB) y módulo `payrol` son dead code.
```

### 8.5 Flujo de Compra

```
1. Usuario crea orden de compra
   → Selecciona proveedor
   → Añade productos con lote/fecha vencimiento
   → Cantidades y precios

2. Sistema procesa:
   → INSERT product_purchase (cabecera)
   → INSERT product_purchase_details (líneas)
   → INSERT acc_transaction × 4-5 (doble partida)
   → Actualización de stock automática
   → Actualización cuenta proveedor

3. Post-compra:
   → Impresión orden
   → Recepción parcial/total
   → Devolución (si aplica)
```

---

## 9. SEGURIDAD

### 9.1 Hallazgos Críticos

| Aspecto | Estado Actual | Riesgo | Recomendación Go |
|---------|--------------|--------|------------------|
| Hash contraseñas | `md5('gef'.$password)` | **CRÍTICO** | bcrypt/argon2 |
| Clave encriptación | Hardcodeada `MySuperEncryptionKEY2017` | **CRÍTICO** | Env vars + gestión segura |
| CSRF Protection | **Deshabilitada** | **ALTO** | Implementar tokens CSRF |
| SQL Injection | Consultas raw en algunos puntos | **ALTO** | Query builder + prepared statements |
| XSS | Global XSS filtering (deprecated) | **MEDIO** | Context-aware escaping |
| Sesión | Files-based, sin regeneración | **MEDIO** | JWT + refresh tokens + Redis |
| Control acceso | RBAC básico (module/method) | **BAYO** | Mejorar con políticas granulares |
| Archivos subidos | Validación básica tipo | **MEDIO** | MIME detection, tamaño, sandbox |
| API Auth | Sin tokens, sesión PHP | **ALTO** | JWT o API keys |

### 9.2 Modelo de Seguridad Propuesto para ERPGo

```go
// Autenticación: JWT con refresh tokens
type AuthService interface {
    Login(ctx, email, password) (*TokenPair, error)
    Refresh(ctx, refreshToken) (*TokenPair, error)
    Logout(ctx, token) error
}

// Autorización: RBAC con permisos granulares
type PermissionService interface {
    HasPermission(ctx, userID, resource, action) bool
    CheckModuleAccess(ctx, userID, module) bool
}

// Middleware
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        claims, err := validateToken(token)
        if err != nil {
            http.Error(w, "Unauthorized", 401)
            return
        }
        ctx := context.WithValue(r.Context(), "user", claims)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

---

## 10. INTEGRACIONES EXTERNAS

### 10.1 Integraciones Identificadas

| Integración | Tecnología | Uso | Implementación Go |
|-------------|-----------|-----|-------------------|
| **PDF** | DomPDF 3.0.2 | Facturas, reportes, etiquetas | gofpdf, unipdf, o ChromedPD |
| **Excel** | PHPExcel 1.x (deprecated) | Importar/exportar productos | excelize, xlsx |
| **SMS** | Nexmo API | Notificar ventas, alertas | nexmo-go, Twilio |
| **Email** | PHP mail() | Recuperación contraseña | net/smtp, gomail.v2 |
| **QR** | PHP QR Code | Productos, facturas | go-qrcode, skip2/go-qrcode |
| **Barcode** | Custom Code128/39 | Productos, facturas | boombuler/barcode |
| **AI** | OpenAI API | Análisis de negocio | openai-go |
| **reCAPTCHA** | Google reCAPTCHA | Formulario login | API verification |
| **FTP** | PHP FTP functions | Sincronización multi-servidor | goftp, rclone |

### 10.2 Configuración de Integraciones

```yaml
# config.yaml para ERPGo
database:
  driver: mysql
  host: ${DB_HOST}
  port: 3306
  name: erpgow
  user: ${DB_USER}
  password: ${DB_PASS}
  max_open_conns: 25
  max_idle_conns: 5

jwt:
  secret: ${JWT_SECRET}
  access_ttl: 15m
  refresh_ttl: 7d

pdf:
  engine: gotenberg  # o unipdf
  default_layout: a4

sms:
  provider: nexmo
  api_key: ${NEXMO_KEY}
  api_secret: ${NEXMO_SECRET}

email:
  smtp_host: ${SMTP_HOST}
  smtp_port: 587
  username: ${SMTP_USER}
  password: ${SMTP_PASS}
  from: "ERPGo <noreply@erpgow.local>"

openai:
  api_key: ${OPENAI_KEY}
  model: gpt-4
  max_tokens: 2000
```

---

## 11. PLAN DE MIGRACIÓN PROPUESTO

### Fase 1: Fundamento (Semanas 1-3)

```
□ Configuración proyecto Go (módulo, estructura)
□ Sistema de configuración (Viper/env)
□ Conexión base de datos (GORM + migraciones)
□ Entidades: Product, Customer, Supplier, User
□ Repositorios básicos (CRUD)
□ Autenticación JWT + middleware
□ Layout HTML base (replicar visualmente)
```

### Fase 2: Módulos Core (Semanas 4-10)

```
□ Módulo Productos (categorías, unidades, bulk import)
□ Módulo Clientes (CRM, ledger)
□ Módulo Proveedores (CRM, ledger)
□ Módulo Ventas (facturación, POS)
□ Módulo Compras (órdenes, recepción)
□ Módulo Inventario (stock, reportes)
□ DataTables con procesamiento servidor
□ Generación PDF (facturas, reportes)
□ Notificaciones (Toastr, SweetAlert)
```

### Fase 3: Contabilidad (Semanas 11-14)

```
□ Plan de cuentas (jsTree, jerárquico)
□ Comprobantes (débito/crédito/contra/diario)
□ Libro mayor
□ Balance de comprobación
□ Estado de resultados
□ Balance general
□ Cierre diario
□ Conciliación bancaria
```

### Fase 4: Nómina y RRHH (Semanas 15-18)

```
□ Empleados y designaciones
□ Asistencia
□ Nómina (setup, generación, pago)
□ Adelantos y préstamos
□ Gastos de oficina
```

### Fase 5: Módulos Avanzados (Semanas 19-22)

```
□ Servicios (facturación)
□ Cotizaciones
□ Devoluciones
□ Banca (cuentas, transacciones)
□ Impuestos (configuración, reportes)
```

### Fase 6: Reportes y Dashboard (Semanas 23-25)

```
□ Dashboard con gráficos
□ Reporte ventas (múltiples filtros)
□ Reporte compras
□ Reporte stock
□ Reporte impuestos
□ Exportación CSV/Excel/PDF
□ Gráficos interactivos
```

### Fase 7: Configuración y Admin (Semanas 26-28)

```
□ Configuración empresa
□ Gestión usuarios y roles
□ Permisos RBAC
□ Moneda e idioma
□ Backup base de datos
□ Sincronización
□ OpenAI integration
```

### Fase 8: Pulido y Producción (Semanas 29-32)

```
□ Pruebas unitarias (domain layer)
□ Pruebas integración (repositories)
□ Pruebas E2E (Playwright/CI)
□ Optimización de consultas
□ Caché Redis
□ Logs estructurados
□ Dockerización
□ CI/CD pipeline
□ Documentación API (Swagger)
```

---

## 12. ESTIMACIÓN DE ESFUERZO

### 12.1 Líneas de Código Estimadas

| Componente | Archivos Est. | Líneas Est. |
|-----------|---------------|-------------|
| Entidades de dominio | 25 | 2,500 |
| Casos de uso / Servicios | 20 | 4,000 |
| Repositorios (GORM) | 25 | 3,000 |
| Handlers HTTP | 15 | 3,000 |
| Middleware y Auth | 5 | 1,000 |
| Templates HTML | 50 | 8,000 |
| Static assets (copiados) | - | 15,000 |
| Migraciones SQL | 20 | 2,000 |
| Tests | 30 | 5,000 |
| Configuración y Docker | 10 | 500 |
| **TOTAL** | **~200** | **~44,000** |

### 12.2 Esfuerzo Humano

| Perfil | Dedicación | Duración |
|--------|-----------|----------|
| Senior Go Developer | 100% | 8 meses |
| Frontend Developer | 50% | 4 meses |
| QA / Testing | 25% | 2 meses |

### 12.3 Complejidad por Módulo (Relativa)

| Módulo | Complejidad | Justificación |
|--------|-------------|---------------|
| Dashboard | Baja | Consultas agregadas simples |
| Productos | Media | CSV import, QR, validaciones |
| Clientes/Proveedores | Media | Saldos calculados, búsquedas |
| Ventas (POS) | **Alta** | Double-entry, SMS, stock, impuestos, descuentos |
| Compras | **Alta** | Lotes, vencimientos, double-entry |
| Contabilidad | **Muy Alta** | Doble partida, múltiples reportes, cierre |
| Nómina | **Alta** | Cálculos salariales, préstamos, impuestos |
| Reportes | Media | DataTables server-side, filtros |
| Permisos/RBAC | Media | Matriz de permisos granular |
| Configuración | Baja | CRUD simple |

### 12.4 Riesgos del Proyecto

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|-------------|---------|------------|
| Alcance mayor del estimado | Alta | Alto | Iterativo, MVP primero |
| Pérdida de lógica oculta | Media | Alto | Pruebas E2E exhaustivas |
| Problemas de rendimiento en reportes | Media | Medio | Caché + índices + paginación |
| Incompatibilidad de datos al migrar | Baja | Alto | Migración gradual, scripts de verificación |
| Complejidad POS en tiempo real | Media | Medio | WebSockets, diseño incremental |

---

## CONCLUSIONES

### ¿Es viable migrar SalesERP a ERPGo en Go?

**Sí, completamente viable.** El proyecto tiene:

1. **Lógica de negocio bien contenida** en pocos archivos
2. **Funcionalidad completamente identificada** y documentada
3. **UI replicable** (mismas librerías frontend)
4. **Integraciones mapeadas** con equivalentes Go disponibles

### Ventajas de la migración a Go:

- **Rendimiento**: Go maneja concurrencia mucho mejor que PHP
- **Tipado estático**: Menos errores en runtime
- **Binario único**: Despliegue simplificado
- **Concurrencia**: WebSockets para notificaciones en tiempo real
- **Ecosistema maduro**: Librerías de calidad para todo lo necesario

### Desventajas / Costos:

- **Tiempo significativo**: ~8 meses para 1 developer senior
- **Reescritura total**: No hay migración incremental posible
- **Pruebas exhaustivas requeridas**: La lógica financiera no tolera errores
- **Sin tests heredados**: Todo debe verificarse manualmente

### Recomendación Final:

**Proceder con la migración** usando la estrategia de fases propuesta. Comenzar con autenticación y módulos core (productos, clientes), luego ventas/compras, contabilidad, y finalmente módulos avanzados y reportes. Usar el presente documento como mapa de ruta permanente.

---

*Documento generado: 2026-09-26*
*Análisis basado en código fuente de SalesERP v12.13*
