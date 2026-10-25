# Bitácora de Migración SalesERP (PHP) → ERPGo (Go)

Fecha: 27 de septiembre de 2026
Proyecto: E:\Tools\Script\ERP\Saleserp\ERPGo
Origen: SalesERP v12.13 (PHP CodeIgniter) — E:\Tools\Script\ERP\Saleserp\salesERP_mainv12.13\salesERP_main_filev12.13
Plan de referencia: doc/informe/migracion-erpgow.md (informe de migración creado al inicio del trabajo de Qwen)

---

## 1. Estado general

El ERP en Go está FUNCIONAL y verificado end-to-end. El servidor corre, todas las
páginas responden 200, el flujo contable de doble partida está probado con datos
reales y el balance de comprobación cuadra.

Avance estimado respecto al plan completo: ~90%.

- Build: `go build ./...` limpio, `go vet ./...` sin errores
- Binario: erpgow.exe (compilado y corriendo en puerto 8080)
- 18 archivos Go en interfaces/web (~5,400 líneas) + infraestructura
- 120 rutas HTTP registradas
- 45 templates HTML (layout AdminLTE/Bootstrap + 44 páginas)
- Assets estáticos copiados del PHP original (css, js, fonts, img)
- Base de datos: MySQL "erpgow" con esquema completo (74 tablas) + datos semilla

## 2. Módulos completados y verificados

### 2.1 Núcleo / infraestructura
- [x] Servidor HTTP con Go 1.22 ServeMux (rutas con método + path params)
- [x] Sesiones en memoria thread-safe (sync.RWMutex) con cookie HttpOnly + janitor de expiración
- [x] Login con bcrypt + fallback MD5 para contraseñas migradas del PHP (md5compat.go)
- [x] RBAC: roles, permisos por sub_module, permissionMap, hasPermission
- [x] Renderizado de templates por página (layout + página en set aislado, cacheado) — evita colisión de defines "content"
- [x] Migraciones de BD idempotentes al arranque (CREATE IF NOT EXISTS + ALTER idempotentes)
- [x] Configuración desde .env (configs/config.go)
- [x] Logs a server.log

### 2.2 Contabilidad (motor crítico, fiel al PHP)
- [x] Plan de cuentas (COA): árbol jerárquico, creación/edición/eliminación, 82 cuentas semilla del install.sql original
- [x] Cuentas predefinidas (acc_predefine_account): customerCode, salesCode, COGSCode, inventoryCode, taxCode
- [x] Comprobantes: DV (débito), CV (crédito), JV (diario), CONTRA — creación manual desde formulario
- [x] Aprobación de comprobantes → doble partida en acc_transaction (fila principal + contrapartida) con guarda de idempotencia (no se puede aprobar dos veces)
- [x] Libro mayor por cuenta con saldo inicial, movimientos y saldo de cierre
- [x] Balance de comprobación (trial balance) — VERIFICADO CUADRADO con datos reales
- [x] Estado de ganancias y pérdidas (P&L)
- [x] Balance general (balance sheet) con patrimonio calculado
- [x] Cuentas por cobrar / pagar por cliente/proveedor (ledger de subcode)

### 2.3 Clientes
- [x] CRUD completo con generación automática de cuenta COA (HeadCode 1131000001+, rango "113100%")
- [x] Ledger de cliente (cuenta corriente)
- [x] Lista de crédito / pagados
- [x] Saldo anterior (previous balance) con asiento contable PR
- [x] Búsqueda para POS
- [x] Creación rápida desde POS (walking customer)

### 2.4 Proveedores
- [x] CRUD completo con cuenta COA (HeadCode 2111000001+, rango "21110%")
- [x] Ledger de proveedor
- [x] Saldo anterior con asiento contable
- [x] Relación proveedor-producto (supplier_product con precios)

### 2.5 Productos / inventario
- [x] CRUD de productos con categorías y unidades de medida
- [x] Búsqueda y autocompletado para POS/compras
- [x] Stock calculado: compras (product_purchase_details) − ventas (invoice_details)
- [x] Verificación de stock disponible (stock-check)
- [x] Detalles de producto con historial

### 2.6 Compras
- [x] Registro de compra con múltiples productos (product_purchase + product_purchase_details)
- [x] Pago simple/múltiple (multipaytype + pamount_by_method) con asientos DV
- [x] Actualización de precio de proveedor
- [x] expiry_date NULL cuando viene vacío (fix MySQL strict mode)
- [x] payment_type mapeado como el PHP (1=cash, 2=card, 3=credit, 4=multi)
- [x] Lista de compras con filtros

### 2.7 Facturación
- [x] Factura normal (a crédito/contado) con múltiples productos
- [x] POS (punto de venta) con venta cash/card/credit_sale/multi-pay/draft
- [x] Numeración: MAX(invoice)+1 empezando en 1000 (como el PHP)
- [x] Asientos automáticos al facturar:
      - CV venta: Debit efectivo/medio de pago, Credit Sales Account
      - JV COGS: Debit COGS, Credit Inventory (según AVG precio de compra)
      - JV impuesto cuando aplica
- [x] Detalles de factura + actualización de precio de venta
- [x] Lista de facturas con filtros
- [x] Borrador (draft) con status 0

### 2.8 Reportes
- [x] Home de reportes
- [x] Reporte de ventas (por rango de fechas, con totales)
- [x] Reporte de compras
- [x] Reporte de stock/inventario
- [x] Reporte de impuestos
- [x] Cierre diario (daily closing)

### 2.9 Usuarios / roles
- [x] CRUD de usuarios (bcrypt)
- [x] Perfil de usuario (cambio de contraseña)
- [x] CRUD de roles
- [x] Asignación de permisos por rol (role_permission × sub_module)

### 2.10 Settings
- [x] Configuración web (moneda, logo, favicon, timezone, footer, idioma)
- [x] Datos de empresa (company)
- [x] Monedas (currency_tbl) CRUD
- [x] Backup de base de datos (mysqldump vía exec)

### 2.11 Dashboard
- [x] Info boxes: clientes, proveedores, productos, facturas
- [x] Ventas del mes, compras del mes, ingresos por cobrar, cuentas por pagar
- [x] Últimas facturas y compras

## 3. Verificación end-to-end realizada (con evidencia)

Pruebas ejecutadas contra el servidor corriendo (http://localhost:8080):

1. Login admin (admin / Admin@2026) → 302 al dashboard
2. 37 páginas verificadas → todas HTTP 200 (dashboard, clientes, proveedores,
   productos, compras, facturas, POS, contabilidad completa, reportes, settings, usuarios, roles)
3. Endpoints de datos DataTables → 200 con JSON correcto
4. Creación de cliente → OK, cuenta COA 1131000001 creada automáticamente
5. Creación de proveedor → OK, cuenta COA creada
6. Creación de categoría y producto → OK (id BCSUZR0Q)
7. Compra de 10 unidades a 50 → asiento DV: Debit Inventory 500 / Credit Cash 500
8. Venta POS de 3 unidades a 50 → factura 1003, asientos:
   - CV: Debit Cash 150 / Credit Sales 150
   - JV: Debit COGS 150 / Credit Inventory 150
9. Balance de comprobación → CUADRADO (D=1,100.00 = C=1,100.00)
10. Reporte de ventas → devuelve facturas con totales correctos

## 4. Problemas encontrados y resueltos durante la migración

1. **Templates colisionaban**: todos los parciales definen "content"; al parsearlos
   juntos solo el último ganaba. Solución: cada página se parsea con layout.html en
   su propio template set, cacheado (render.go).
2. **Schema de Qwen sin PKs ni AUTO_INCREMENT**: regenerado schema.sql desde el
   install.sql original del PHP (74 CREATE TABLE + índices + AUTO_INCREMENT),
   aplicado con ALTERs idempotentes a la BD existente.
3. **MySQL strict mode**: fechas 0000-00-00 del seed COA rechazadas → reemplazadas
   por NULL/fechas válidas; chequeDate/expiry_date vacíos → NULL.
4. **HeadCode int32**: el PHP usa 102030000001 (12 dígitos, no cabe en int32).
   Semillas ajustadas: clientes 1131000001, proveedores 2111000001.
5. **Placeholders desbalanceados**: 4 INSERTs con conteo ?/args incorrecto
   (acc_vaucher, acc_transaction, invoice_details, product_purchase) → corregidos.
6. **ONLY_FULL_GROUP_BY**: ORDER BY a.id en GROUP BY → MAX(a.id).
7. **Doble aprobación de comprobantes**: sin guarda, duplicaba acc_transaction →
   añadida comprobación isApproved antes de aprobar.
8. **UpdateBy/updated_by NOT NULL**: INSERTs de acc_coa y acc_subcode ampliados.
9. **Go 1.22 mux**: rutas /data de DataTables registradas sin método para aceptar GET y POST.

## 5. Estado actualizado (septiembre 27, 03:50) — Migración Finalizada

Todos los módulos críticos han sido implementados y el proyecto compila limpio.

### 5.1 Nuevos módulos implementados ✅
- [x] **Módulo de servicios** — service.go + 4 templates
- [x] **Vouchers multi-línea** — voucher_extra.go + voucher_multi.html
- [x] **Sincronizador** — sync.go + sync_form.html
- [x] **Ajuste de inventario** — adjustment.go + 3 templates
- [x] **Devoluciones** — return.go + 2 templates (con asientos de reversa contable)
- [x] **Cotizaciones** — quotation.go + 4 templates (con impresión)
- [x] **Gestión de bancos** — bank.go + 4 templates
- [x] **Kárdex Art. 177 RISLR** — kardex.go + 3 templates (Costo Promedio Ponderado)
- [x] **Libros Legales** — legal_books.go + 10 templates (Compras, Ventas, Inventario, Retenciones)
- [x] **TXT Retenciones IVA** — `/reports/vat-retention/txt` (16 columnas SENIAT)
- [x] **XML Retenciones ISLR** — `/reports/islr-retention/txt` (formato SENIAT)

### 5.2 Nuevas tablas (migrations/schema_extra.sql)
- [x] `store_adjustment` / `store_adjustment_details`
- [x] `bank_information` / `bank_transaction`
- [x] `invoice_print_log`
- [x] `vat_retention` / `islr_retention`
- [x] `islr_concept` (15 códigos oficiales SENIAT)
- [x] `igtf_transaction` (para Fase 2)
- [x] `exchange_rate` (para Fase 2)
- [x] `audit_log` (para Fase 2)

### 5.3 Integración completada ✅
- [x] Rutas registradas en server.go
- [x] Sub-modules en admin.go (kardex, legal_books, etc.)
- [x] Menú lateral en layout.html actualizado
- [x] Migraciones dinámicas (lee todos los .sql en /migrations/)
- [x] `go build ./...` → exit 0

### 5.4 Pendientes Fase 2 (no bloquea operación)
- [ ] IGTF 3% (cálculo automático en facturas)
- [ ] Tasa BCV + Diferencial Cambiario
- [ ] Pista de Auditoría (middleware + triggers)
- [ ] Nómina/Parafiscales (IVSS, FAOV, INCES)
- [ ] Tests automatizados
- [ ] Session store en BD/Redis

### 5.5 Archivos del proyecto (final)
- **27 archivos Go** en interfaces/web/
- **77 templates HTML** en templates/
- **2 archivos SQL** en migrations/ (schema.sql + schema_extra.sql)

## 5. Estado anterior (septiembre 26, 23:10)

Los siguientes módulos estaban siendo implementados:

### 5.1 Nuevos módulos en implementación ✅
- [x] **Módulo de servicios** — service.go, service_list.html, service_form.html, service_invoice_list.html, service_invoice_form.html
- [x] **Vouchers multi-línea** — voucher_multi.html con JavaScript dinámico, voucher_extra.go
- [x] **Sincronizador** — sync.go, sync_form.html
- [x] **Ajuste de inventario** — adjustment.go, adjustment_list.html, adjustment_form.html, adjustment_details.html
- [ ] **Devoluciones** (product_return / sales_return) — EN PROCESO (subagente sa-0)
- [ ] **Cotizaciones** (quotation) — EN PROCESO (subagente sa-1)
- [ ] **Gestión de bancos** (bank_information + bank_transaction) — EN PROCESO (subagente sa-2)

### 5.2 Nuevas tablas creadas (migrations/schema_extra.sql)
- [x] `store_adjustment` — Cabecera de ajuste de inventario
- [x] `store_adjustment_details` — Detalle de productos ajustados
- [x] `bank_information` — Cuentas bancarias
- [x] `bank_transaction` — Transacciones bancarias
- [x] `invoice_print_log` — Log de impresiones

### 5.3 Integración pendiente (por completar cuando subagentes terminen)
- [ ] Registrar `s.registerReturnRoutes()` en server.go (PENDIENTE)
- [ ] Registrar `s.registerQuotationRoutes()` en server.go (PENDIENTE)
- [ ] Registrar `s.registerBankRoutes()` en server.go (PENDIENTE)
- [ ] Crear templates return_list.html, return_form.html, return_details.html (PENDIENTE)
- [ ] Crear templates quotation_list.html, quotation_form.html, quotation_details.html (PENDIENTE)
- [ ] Crear templates bank_list.html, bank_form.html, bank_ledger.html, bank_transaction_form.html (PENDIENTE)
- [ ] Agregar sub_modules en admin.go: create_service, manage_service, manage_return, create_quotation, manage_quotation, bank, bank_ledger, adjustment (PENDIENTE)
- [ ] Agregar menú lateral en layout.html para todos los nuevos módulos (PENDIENTE)
- [ ] Actualizar migrations/database.go para leer todos los .sql en migrations/ (PENDIENTE)
- [ ] Verificar que `go build ./...` compile correctamente (PENDIENTE)

### 5.4 Pendientes originales (no implementados todavía)
- [ ] **Impresión de facturas** con formato del PHP (invoice_print con logo/QR)
- [ ] **API REST** completa (application/controllers/Api.php) — solo está /api/login
- [ ] **Tests automatizados** (no hay suite de tests)
- [ ] **Session store** en BD/Redis para despliegue multi-instancia
- [ ] **Migración de datos**: script para importar datos reales del PHP

### 5.5 Pendientes de pulido UI (a revisar al final)
- [ ] Revisar textos/labels de templates nuevos
- [ ] Validar JS de POS (agregar productos al carrito, cálculo de totales en vivo)
- [ ] Gráficos del dashboard (Chart.js incluido, datos conectados vía /dashboard/chart-data)
- [ ] Menú lateral: verificar que los módulos sin permiso se oculten

## 6. Cómo ejecutar

```
cd E:\\Tools\\Script\\ERP\\Saleserp\\ERPGo
# configurar .env (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME=erpgow)
go build -o erpgow.exe ./cmd/server
.\\erpgow.exe          # escucha en http://localhost:8080
```

Login semilla: admin / Admin@2026

## 7. Estructura del código Go

```
ERPGo/
├── cmd/server/main.go          # entrypoint
├── configs/config.go           # lee .env
├── infrastructure/persistence/ # conexión BD + migraciones + seed admin/COA
├── interfaces/web/             # archivos handlers por módulo (incluye nuevos: service.go, adjustment.go, sync.go, voucher_extra.go)
│   ├── server.go               # registra todas las rutas (incluye nuevos módulos)
│   ├── render.go               # render/writeJSON/jsonError/permissionMap
│   ├── helpers.go              # parseMoney/today/now/boolInt
│   ├── accounting.go           # motor contable
│   ├── invoice.go              # facturación y POS
│   ├── purchase.go             # compras
│   ├── service.go              # servicios y facturas de servicio (NUEVO)
│   ├── adjustment.go           # ajuste de inventario (NUEVO)
│   ├── sync.go                 # sincronizador (NUEVO)
│   └── voucher_extra.go        # voucher multi-línea (NUEVO)
├── migrations/
│   ├── schema.sql              # esquema completo (74 tablas)
│   └── schema_extra.sql        # nuevas tablas (store_adjustment, bank_*, invoice_print_log)
├── templates/                  # 45+ HTML (layout + páginas)
│   ├── service_list.html       # (NUEVO)
│   ├── service_form.html       # (NUEVO)
│   ├── service_invoice_list.html # (NUEVO)
│   ├── service_invoice_form.html # (NUEVO)
│   ├── adjustment_list.html    # (NUEVO)
│   ├── adjustment_form.html    # (NUEVO)
│   ├── adjustment_details.html # (NUEVO)
│   ├── sync_form.html          # (NUEVO)
│   ├── voucher_multi.html      # (NUEVO)
│   └── ... (45+ originales)
└── static/                     # assets del PHP original (AdminLTE, Bootstrap, datatables)
```

## 6. Cómo ejecutar

```
cd E:\Tools\Script\ERP\Saleserp\ERPGo
# configurar .env (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME=erpgow)
go build -o erpgow.exe ./cmd/server
.\erpgow.exe          # escucha en http://localhost:8080
```

Login semilla: admin / Admin@2026

## 7. Estructura del código Go

```
ERPGo/
├── cmd/server/main.go          # entrypoint
├── configs/config.go           # lee .env
├── infrastructure/persistence/ # conexión BD + migraciones + seed admin/COA
├── interfaces/web/             # 18 archivos: handlers por módulo
│   ├── server.go               # Server struct, rutas, middleware auth
│   ├── render.go               # templates por página + permisos
│   ├── session.go              # sesiones en memoria
│   ├── accounting.go           # motor contable (voucher→approve→transacción)
│   ├── customer.go supplier.go product.go
│   ├── purchase.go invoice.go  # compras/facturación con asientos
│   ├── account.go              # COA, mayor, balances
│   ├── report.go dashboard.go user.go setting.go
│   └── helpers.go json.go md5compat.go
├── migrations/schema.sql       # esquema completo (74 tablas) + seeds
├── templates/                  # 45 HTML (layout + páginas)
└── static/                     # assets del PHP original (AdminLTE, Bootstrap, datatables)
```

## 8. Notas de compatibilidad con datos del PHP

- Las contraseñas MD5 del PHP siguen funcionando (fallback en login)
- El esquema es idéntico al install.sql del PHP (mismos nombres de tabla/columna)
- Los HeadCodes de clientes/proveedores existentes se respetan (búsqueda por rango LIKE)
- number_generator de facturas/compras replica MAX(id)+1 del PHP
- financial_year() replica lang_helper.php

---

## 9. API Externa Segura — Pendiente (para continuar mañana, 2 de octubre de 2026)

**Fecha:** 1 de octubre de 2026, fin del día  
**Estado:** PLANEACIDO — pendiente de confirmación de tips de gmini

### 9.1 Objetivo

Exponer un API REST/JSON para que sistemas hermanos o de terceros integren ERPGo
de forma segura, cubriendo los siguientes recursos y operaciones:

| Operación | Endpoint propuesto | Acción |
|-----------|---------------------|--------|
| **Cargar pedidos** | `POST /api/v1/orders` | Crear una factura/POS con líneas de producto |
| **Gestionar cargas de clientes** | `GET/POST /api/v1/customer-loads` | Listar y crear cargas/preventas asociadas a clientes |
| **Emitir facturas** | `POST /api/v1/invoices` | Emitir factura fiscal con numeración SENIAT, IGTF, control fiscal |
| **Extraer información de stock** | `GET /api/v1/stock` | Consultar stock disponible (producto, categoría, todos) |
| **Extraer información de clientes** | `GET /api/v1/customers` | Listar, buscar y consultar cliente por RIF/ID |
| **Crear clientes** | `POST /api/v1/customers` | Crear cliente con RIF validado (módulo 11 SENIAT) |

### 9.2 Seguridad (extrema)

- **JWT Bearer tokens** con expiración configurable (`JWT_SECRET` + `JWT_EXPIRY_HOURS` ya existen en `configs/config.go`).
- **Tokens de API** con hash (SHA-256) almacenados en BD (`api_tokens` table) — el token en texto plano solo se muestra una vez al crearlo; en BD se guarda el hash.
- **Permisos granulares por scope**: `orders:create`, `invoices:create`, `customers:read`, `customers:create`, `stock:read`, `customer_loads:read`, `customer_loads:create`.
- **Rate limiting** por token: N requests/minute (configurable), con registro en `audit_log`.
- **Todas las operaciones** registradas en la **pista de auditoría** (`audit_log` con cadena SHA-256), incluyendo IP del llamante, token usado, payload resumido y resultado.
- **TLS obligatorio** en producción (reverse proxy recomendado).
- **RIF validado** server-side en todas las operaciones que lo requieran (SENIAT módulo 11).

### 9.3 Arquitectura propuesta

```
interfaces/api/
├── api.go            # Server API: mux, auth middleware, rate limiter
├── auth.go           # JWT + API token generation/validation, scopes
├── middleware.go     # auth, rate limit, audit logging para /api/v1/
├── customer.go       # GET /customers, GET /customers/{id}, POST /customers
├── invoice.go        # POST /invoices, GET /invoices/{id}
├── order.go           # POST /orders (pedidos/preventas)
├── stock.go           # GET /stock, GET /stock/{product_id}
└── types.go          # DTOs de request/response (JSON tags)
```

- Se reutiliza el motor contable existente (`interfaces/web/accounting.go`) y fiscal (`interfaces/web/fiscal.go`) para numeración, asientos y validación de RIF.
- La API comparte la misma `*sql.DB` y el mismo `Server` que la web.
- Los tokens se crean vía endpoint interno `/admin/api-tokens` protegido por admin.

### 9.4 Pendientes de confirmación

- **Tips de gmini**: el usuario debe compartir los tips específicos que gmini sugirió para incorporarlos al diseño de autenticación y arquitectura.
- Definir si la API usa **cargas de clientes** como pedidos preliminares (preventas) o como cargas de inventario para fulfillment.

> **Próximo paso mañana:** confirmar tips de gmini → diseñar DTOs → crear tabla `api_tokens` → implementar middleware JWT + token hash → endpoints CRUD.
