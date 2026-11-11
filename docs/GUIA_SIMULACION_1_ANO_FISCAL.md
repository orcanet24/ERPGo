## G. Pasos de ejecución resumidos (checklist final)

### Semana 1 (Día 1-7):
- [ ] Instalar ERPGo y configurar `.env`
- [ ] Aplicar migraciones y verificar `go build ./...` en verde
- [ ] Crear 4 proveedores (res, cerdo, pollo, huevos)
- [ ] Cargar categorías, unidades y productos base (1001-1008)
- [ ] Configurar precios mayor (18% descuento) en `product_price_list`
- [ ] Crear 6 empleados con perfiles y departamentos
- [ ] Cargar tasas ISLR, CTS, PENSION, ALIMENT en `tax_rate_table`
- [ ] Definir 4 clases de cliente (Mayor Fijo, Mayor Esporádico, Detalle, Restaurante)
- [ ] Insertar 4 perfiles de cliente

### Semana 2-3 (Día 8-21):
- [ ] Ejecutar flujo mensual completo 2 veces (para entrenar al equipo)
- [ ] Registrar compras de cada proveedor con lotes FEFO
- [ ] Registrar ventas mayoristas y detail (mínimo 10 facturas por tipo)
- [ ] Cerrar turnos de caja y conciliar diferencias
- [ ] Ejecutar nómina mensual (12 recibos por empleado = 72 recibos total)
- [ ] Aprobar asientos contables al cierre de cada mes
- [ ] Generar reportes SENIAT mensuales (`/report/tax`, `/report/sales`, `/report/stock`)

### Semana 4 (Día 22-30):
- [ ] Simular problema de personal: 1 baja voluntaria, 1 nueva contratación, 1 retiro por pensión
- [ ] Simular épocas festivas: Carnaval (febrero), Semana Santa (abril), Fin de clases (agosto), Regreso a clases (septiembre), Navidad (diciembre), Puente mayo (mayo)
- [ ] Ejecutar fiscalización a mitad de año (mes 6): contador revisa libros, retenciones, stock, correlatividad
- [ ] Generar informe de validación final (12 meses completos)
- [ ] Validar que todos los cuadros de mando cuadrino
- [ ] Verificar que `medirInformeCarniceria` no devuelva `Librovacio = true`
- [ ] Confirmar que `Descuadre` sea mínimo (< 0.01 Bs por redondeo)

### Post-simulación:
- [ ] Guardar esta documentación como referencia para futuras simulaciones
- [ ] Documentar cualquier ajuste de constantes (`rendimientoDespiece`, `costoPorKilo`, `canalesPorMes`, sueldos)
- [ ] Programar simulaciones trimestrales para validar que el sistema sigue funcionando después de cambios reales
- [ ] Realizar backup de la BD antes de cualquier cambio de producción

---
## H. Personalización y ajustes posteriores

### H.1 Cambiar constantes del negocio
Editar `interfaces/web/semilla_carniceria.go` línea 55 (`const rendimientoDespiece = 72.0`) si el rendimiento real es distinto.

### H.2 Ajustar sueldos
Modificar `hr_employee` wages según el mercado laboral actual.

### H.3 Añadir productos
Si se venden otros rubros (embutidos, pescados), agregar IDs de producto 1009+ y registrar en `product_category`.

### H.4 Nuevas épocas
Agregar nuevas promociones en `POST /quotation/save` con `is_festive` personalizado.

### H.5 Reportes adicionales
El sistema ya tiene handlers para ISLR, IGTF; consultar `GET /report/tax` y `/report/inventory` para detalles.

> **Regla de oro:** Todo cambio de datos pasa por los handlers oficiales
> (`POST /purchase/save`, `POST /invoiceSave`, `POST /payroll/run`, etc.)
> y nunca por INSERT directo en tablas, para que la auditoría y el journal
> (`internal/orchestrator/journal.go`) queden cuadrados.

---
*Fin de la Guía de Simulación Fiscal Completa 1 Año*
*Documento generado el `2026-11-11` para empresa simulada "Carnicería La Central"*
*Validar con: `go build ./... && go test ./...` antes y después de cada fase*
## F. Capa 5 — Validación final (después de 12 meses)

### F.1 Cuadros de mando obligatorios (deben verse en panel principal)

| Cuadro | Descripción | Fórmula / Origen |
|--------|-------------|------------------|
| **Ventas del Mes** | Total facturado (mayor + detalle) | `SUM(invoice.total_amount)` |
| **Costo de Ventas** | Costo de productos vendidos (costo real por lote) | `SUM(invoice_lines.quantity * product_cost_config.cost)` |
| **Margen Bruto** | (Ventas - Costo Ventas) / Ventas | Cálculo derivado |
| **Caja Final** | Efectivo + Tarjetas al cierre último turno | `cash_ledger.last_declared_cash + cash_ledger.last_declared_card` |
| **Deudores (CxC)** | Total pendiente cobrar por clientes | `SUM(invoice.due_amount) WHERE is_draft='no'` |
| **Acreedores (CxP)** | Total pendiente pagar a proveedores | `SUM(purchase.total_amount) WHERE status='pending'` |
| **Inventario Valorado** | Stock actual × costo promedio por producto | `SUM(product_lot.quantity_available * product_cost_config.cost)` |
| **Nómina del Mes** | Total pagado en planillas | `SUM(hr_payslip.net_pay) FOR period_to = mes actual` |
| **ISLR Retenido** | Total retenido y declarado a SENIAT | `SUM(hr_payroll_item.amount) WHERE concept_code='ISLR'` |

### F.2 Informe de validación final (después de 12 meses)

```go
tipo InformeValidacion struct {
    TotalFacturado      float64   `json:"total_facturado"`
    TotalCobrado        float64   `json:"total_cobrado"`
    TotalPorCobrar      float64   `json:"total_por_cobrar"`
    DebeLibro           float64   `json:"debe_libro"`
    HaberLibro          float64   `json:"haber_libro"`
    Descuadre           float64   `json:"descuadre"`
    LibroVacio          bool      `json:"libro_vacio"`
    CostosCongelados    int       `json:"costos_congelados"`
    PeriodosCerrados    int       `json:"periodos_cerrados"`
    EmpleadosActivos    int       `json:"empleados_activos"`
    NominasRealizadas   int       `json:"nomina_realizada"`
    CostoTotalPatronal  float64   `json:"costo_total_patronal"`
    ClientesMayor       int       `json:"clientes_mayor"`
    ProveedoresActivos  int       `json:"proveedores_activos"`
    SaldosDeudores      float64   `json:"saldos_deudores"`
    SaldosAcreedores    float64   `json:"saldos_acreedores"`
    VentasNavidad       float64   `json:"ventas_navidad"`
    VentasCarnaval      float64   `json:"ventas_carnaval"`
    VentasSemanaSanta   float64   `json:"ventas_semana_santa"`
    VentasFinDeClases   float64   `json:"ventas_fin_clases"`
}
```

### F.3 ¿Qué debe cuadrar al final?

1. **Libro mayor cuadrado:** `DebeLibro ≈ HaberLibro` (diferencia por redondeo < 0.01 Bs).
2. **No hay libro vacío:** `LibroVacio = false` (deben haber asientos contables).
3. **Stock coherente:** `CostosCongelados > 0` (deben haber costos SnapShot de al menos un mes).
4. **Personal rotativo:** `EmpleadosActivos` refleja altas, bajas, retiros.
5. **Clientes/proveedores al día:** `SaldosDeudores` y `SaldosAcreedores` coherentes con reportes.
6. **Festividades registradas:** Ventas estacionales aparecen en reportes correspondientes.
7. **ISLR y retenciones:** Totales declarados coinciden con movimientos de caja y nómina.
## E. Capa 4 — Flujo mensual completo (Día 4-25)

### E.1 Estructura de un mes típico (12 veces)

#### Paso 1: Compra de inventario (proveedores)
- **Día 1-3:** Ejecutar `POST /purchase/save` para cada proveedor.
- **Datos de ejemplo (Mes 1 - enero):**
  - Proveedor Res: 40 canales de 250 kg cada una → total 10.000 kg.
  - Proveedor Cerdo: 1 canal de 100 kg + 2 medias canales de 50 kg → total 200 kg.
  - Proveedor Pollo: 30 pollos enteros de 1.5 kg cada uno → total 45 kg.
  - Huevos: 50 cajas de 30 huevos cada una → total 1.500 unidades.

> **Handler usado:** `POST /purchase/save` → genera `purchase_head` y `purchase_lines` en `product_lot` con FEFO.

#### Paso 2: Despiece y asignación de rendimiento (product_yield)
- **Día 3-4:** El sistema reparte el costo de la canal según el rendimiento:
  - Muslo: 18% de la masa
  - Pechuga: 30% de la masa
  - Falda y costilla: 20% + 4% = 24% acumulada
  - Hígado y menudencia: 4% (o 10% si se incluye)
  - **Total = 72%** (por defecto en `semilla_carniceria.go` constante `rendimientoDespiece = 72.0`)

- **Costo por corte:**
  ```
  costo_corte = (costo_canal / 100) * porcentaje_rendimiento
  ```
  Ejemplo: Canal de res cuesta Bs. 100/kg. Muslo (18%): Bs. 18/kg. Pechuga (30%): Bs. 30/kg, etc.

#### Paso 3: Entrada a bodega (lotes con FEFO)
- **Día 4-5:** Los lotes se crean con `fecha_vence` = día 5 del mes siguiente.
- **Stock disponible:** Se lee de `product_lot`, no de `product_information`.
- **Validación:** `comprobarStock(producto, fecha_venta, cantidad)` → si no hay, la venta no se registra.

#### Paso 4: Ventas al Mayor (mayoristas)
- **Día 6-20:** `POST /invoiceSave` o `POST /sale/new` con tipo `mayor`.
- **Formato de factura:**
  - `customer_id = CLI-001` (Mayor Fijo) o `CLI-003` (Mayor Esporádico).
  - `payment_term = 30 dias` o `15 dias` según clase.
  - `items`: lista de productos con `quantity`, `unit_price` (precio de mostrador), `discount_pct` (según negociación).
  - `price_list_id = 'mayor'` → precio con 18% de descuento automático.

#### Paso 5: Ventas al Detalle (mostrador)
- **Día 6-20:** `POST /invoiceSave` o venta por POS (`POST /pos/sale`).
- **Formato:**
  - `customer_id = CLI-002` o `CLI-004` (o cliente sin RIF para mostrador).
  - `payment_term = contado` (efectivo/transferencia).
  - `items`: cantidades pequeñas (0.5kg, 1ud, etc.), precio sin descuento (o promociones aplicadas).

#### Paso 6: Cierre de caja y conciliación (caja por turno)
- **Día 20-25:** `POST /cash/close-turn` para cada caja POS.
- **Datos a reportar:**
  - `initial_fund` = fondo inicial del turno.
  - `declared_cash` = efectivo al cierre.
  - `declared_card` = tarjetas de crédito/débito.
  - `total_sales` = suma de todas las ventas del turno.
  - `refunds` = devoluciones (si las hay).
- **Sistema calcula:** `difference = declared - (initial + sales - refunds)` y genera reporte de variación de caja.

#### Paso 7: Liquidación de nómina (payroll run)
- **Día 25-27:** `POST /payroll/run` para el período mensual ya vencido.
- **El sistema genera:**
  - `hr_payroll_run` con `period_from`, `period_to`, `frequency`, `status`.
  - `hr_payslip` por cada empleado (líneas de earnings, deductions, employer_charge).
  - `tax_rate_version` registrado para auditoría.
- **Validación por empleado:** Cada recibo debe tener:
  - `gross` (sueldos)
  - `deductions` (ISLR 15% + CTS 10% + PENSION 5% + ALIMENT 4% + otras descuentos voluntarios)
  - `employer_cost` (cargas patronales ~34% sobre base)
  - `net_pay` (lo que cobra el empleado)

#### Paso 8: Aprobación de asientos contables (closing)
- **Día 27-28:** `POST /reports/daily-closing` o handler interno `aprobarAsientosCarniceria`.
- **El sistema mueve** los asientos de `acc_vaucher` (borrador) a `acc_transaction` (libro).
- **Validación final:** El informe `medirInformeCarniceria` debe mostrar:
  - `Facturado` > 0 (haber transacciones hubo ventas)
  - `Cobrado` = total cobrado en caja
  - `PorCobrar` = total de facturas con `due_amount > 0`
  - `Descuadre` = `DebeLibro - HaberLibro` debe ser `0` (o muy cercano por redondeo)
  - `Librovacio = false` (no puede haber 0 en ambos; eso indicaría que no se generaron asientos).

#### Paso 9: Reportes SENIAT (mensuales)
- **Día 28-30:** Generar los siguientes reportes mediante handlers o SQL:
  - `GET /report/sales` → Ventas totales del mes.
  - `GET /report/purchases` → Compras al proveedor.
  - `GET /report/stock` → Stock actual por producto y lote.
  - `GET /report/tax` → Retenciones ISLR, IGTF aplicadas.
  - `GET /report/inventory` → Inventario valorado (costo FIFO o weighted average).

#### Paso 10: Fiscalización a mitad de año (mes 6)
- **Qué verifica el contador:**
  1. **Libro de IVA:** Que los totales de `acc_transaction` cuadren con las facturas emitidas en `invoice`.
  2. **Retenciones ISLR/IGTF:** Que las retenciones practicadas a proveedores y clientes coincidan con las tasas vigentes y los montos pagados.
  3. **Libro de Compras:** Que todas las `purchase` registradas tiven correspondiente `invoice` de entrada y lotes consumidos.
  4. **Stock físico vs sistema:** Reconciliación de `product_lot` vs conteo real (desecho, mermas, pérdidas).
  5. **Nómina y prejubilaciones:** Que los pagos de nómina (`hr_payslip`) coincidan con los desembolsos bancarios y las retenciones depositadas a SENIAT.
  6. **Clientes y Crédito:** Que los saldos `due_amount` en `invoice` sean coherentes con los pagos recibidos y los convenios de crédito.
  7. **Correlatividad de facturas:** Que no haya huecos en los números de factura (se usa `acc_vaucher` + correlativo `VNo`).

> **Comandos de verificación en la interfaz:**
> - `GET /report/tax` → Ver retenciones del período.
> - `GET /report/sales` → Ver ventas por cliente y categoría.
> - `GET /report/stock` → Ver stock disponible por lote.
> - `GET /report/purchases` → Ver proveedores y montos adeudados.
## D. Capa 3 — Clientes y estrategia de ventas (Día 3-4)

### D.1 Clases de cliente (customer_class)
Crear 4 clases mediante `POST /settings/customer-class/save`:

| customer_class_id | class_name | description | credit_days | discount_pct |
|-------------------|------------|-------------|-------------|--------------|
| `1` | `Mayor Fijo` | Cliente que compra todo los meses, volumen alto | `30` | `10%` a partir de $500 |
| `2` | `Mayor Esporádico` | Compra intermitente, menor volumen | `15` | `5%` a partir de $200 |
| `3` | `Detalle (Mostrador)` | Cliente final, compra por unidad | `Contado` | `0%` |
| `4` | `Restaurantes / Comida` | Compra regular, granel | `45` | `15%` a partir de $1000 |

### D.2 Perfiles de cliente (users / customers)
Insertar en `users` y `customer` tables (o mediante handler `POST /customer/save`):

| user_id | code | first_name | last_name | email | phone | customer_class_id | status |
|---------|------|------------|-----------|-------|-------|-------------------|--------|
| `1` | `CLI-001` | Roberto | Ramírez | `roberto@restaurant.com` | `0414-555-1111` | `1` (Mayor Fijo) | `1` |
| `2` | `CLI-002` | Laura | Flores | `laura@casa.com` | `0424-555-2222` | `3` (Detalle) | `1` |
| `3` | `CLI-003` | Pedro | Gutiérrez | `pedro@bodega.com` | `0416-555-3333` | `2` (Mayor Esporádico) | `1` |
| `4` | `CLI-004` | Sofia | Moreno | `sofia@restaurant2.com` | `0426-555-4444` | `4` (Restaurante) | `1` |

### D.3 Estrategias de ventas por épocas festivas

#### D.3.1 Carnaval (febrero)
- **Estrategia:** Descuento del 15% en cortes de res y pollo para pedidos anticipados (mayor de $500).
- **Promoción:** "Carnaval Pack": 5kg de mezcla (res+pollo) por precio de 4kg.
- **Clientes objetivo:** Mayor fijo + Restaurantes.
- **Handler:** `POST /quotation/new` / `POST /quotation/save` con `is_festive = 'carnaval'`.

#### D.3.2 Semana Santa (abril)
- **Estrategia:** Aumento de precio 5% en huevos (por alta demanda) y descuento 10% en sustitutos.
- **Promoción:** "Paquete Semana Santa": 1 canal de res + 30 huevos a precio paquete.
- **Clientes objetivo:** Mayor esporádico + Detalle.

#### D.3.3 Fin de clases (agosto)
- **Estrategia:** Paquete "Mochila Completa": 2kg de pollo + 1kg de res + 1 docena de huevos.
- **Descuento:** 12% para pagos al contado (efectivo/transferencia).
- **Clientes objetivo:** Familias (clientes detalle mayoristas).

#### D.3.4 Regreso a clases (septiembre)
- **Estrategia:** Descuento 8% en empaques familiares (cajas 5kg, 10kg) para mayoristas.
- **Promoción:** "Primera Compra del Año": 10% de descuento en la primera factura mayorista.

#### D.3.5 Navidad (diciembre)
- **Estrategia:** "Navideña Completa": Canal de res entero + 50 huevos + 2kg de jamón a precio paquete fijo.
- **Crédito especial:** Plazos de 60 días para clientes mayoristas históricos.
- **Incremento de stock:** Comprar 20% más de canales en noviembre (prever despiece).

#### D.3.6 Puente de mayo / Verano (mayo-junio)
- **Estrategia:** Descuento 7% en cortes de cerdo (bbq season).
- **Promoción:** "Asador Pack": 3kg de tocino + 2kg de lomo de cerdo + 1kg de pollo entero.

### D.4 Frecuencia de compra
- **Mayor Fijo:** Compra cada 25-30 días, monto promedio Bs. 2.500-4.000.
- **Mayor Esporádico:** Compra cada 45-60 días, monto promedio Bs. 800-1.500.
- **Detalle (Mostrador):** Compra semanal o cada 15 días, monto promedio Bs. 50-200 por visita.
- **Restaurantes:** Compra cada 15 días, monto promedio Bs. 1.200-3.000.
## C. Capa 2 — Personal y RRHH (Día 2-3)

### C.1 Perfiles de personal (hr_employee)
Crear 6 perfiles mediante `POST /settings/employee/save` (o SQL directo en `hr_employee`):

| employee_id | code | first_name | last_name | status | department_id | role | level | wage_monthly (Bs.) |
|-------------|------|------------|-----------|--------|---------------|------|-------|--------------------|
| `1` | `ADM-001` | María | Pérez | `1` | `1` | `admin` | `master` | `45000.00` |
| `2` | `COX-001` | Carlos | Gómez | `1` | `2` | `cajero` | `employee` | `25000.00` |
| `3` | `DES-001` | José | Rodríguez | `1` | `3` | `despachador` | `employee` | `22000.00` |
| `4` | `CON-001` | Ana | López | `1` | `4` | `contador` | `manager` | `35000.00` |
| `5` | `PRO-001` | Luis | Martínez | `1` | `5` | `proveedor` | `employee` | `28000.00` |
| `6` | `LIM-001` | Pedro | Suárez | `1` | `6` | `ayudante` | `employee` | `20000.00` |

> **Departmentos (departamento_id):**
> 1 = Administración
> 2 = Punto de Venta / Caja
> 3 = Despacho / Despiece
> 4 = Contabilidad / Fiscal
> 5 = Proveedores / Compras
> 6 = Limpieza / Almacén

### C.2 Cargar tasas y brackets ISLR (tax_rate_table)
El sistema usa `tax_rate_table` con vigencia. Cargar las siguientes tasas por concepto parafiscal (vigencia `2026-01-01`):

| concept_code | label | rate_pct | base_kind | applies_to | effective_from | legal_citation | verified |
|-------------|-------|----------|-----------|------------|----------------|----------------|----------|
| `ISLR` | Impuesto sobre la Renta | `15.00` | `employer` | `employee` | `2026-01-01` | Ley del ISLR art. 1 | `1` |
| `CTS` | Cesantía | `10.00` | `employer` | `employee` | `2026-01-01` | Decreto N° 2.544 | `1` |
| `PENSION` | Pensión | `5.00` | `employer` | `employee` | `2026-01-01` | Decreto N° 2.850 | `1` |
| `ALIMENT` | Alimentación | `4.00` | `employer` | `employee` | `2026-01-01` | AcuerdoMinisterial 1234 | `1` |

> **Nota:** Las tasas se congelarán en cada nómina (`tax_rate_version`). Si cambian, se añade una nueva fila con nueva `effective_from`.

### C.3 Configurar periodos de nómina y frecuencia
Usar `POST /payroll/frequency/save` o SQL en `hr_frequency`:
- `frequency_id=1`, `frequency_name="Semanal"`, `periods_per_year=52`
- `frequency_id=2`, `frequency_name="Quincenal"`, `periods_per_year=26`
- `frequency_id=3`, `frequency_name="Mensual"`, `periods_per_year=12` (por defecto)

### C.4 Simular problemas de personal (baja, reinserción, retiro)

#### C.4.1 Baja voluntaria (despido)
1. Cambiar `status = 0` en `hr_employee` para el empleado.
2. Registrar la baja en `hr_payroll_run` con `status = 'cancelled'`.
3. Ejecutar handler `POST /payroll/run` con el empleado dado de baja → el sistema generará recibo con `status = 'cancelled'` y `net_pay = 0` (o liquidación según ley).

#### C.4.2 Nueva contratación
1. Insertar nuevo row en `hr_employee` con `status = 1`.
2. Asignar `department_id` y `role`.
3. Ejecutar primer `POST /payroll/run` con frecuencia `mensual` → sistema generará recibo con `status = 'initialized'`.

#### C.4.3 Retiro por causa (pensión)
1. Mantenir empleado activo pero reducir `wage_monthly` a `0` (o a importe de pensión).
2. Marcar `retirement_date` en campo adicional (el ERPGo usa `update_date` + comentarios).
3. Liquidar último recibo con concepto `RETIRO` y `amount = importe_pensional`.
4. Dar de baja `status = 0` al mes siguiente.

### C.5 Planillas mensuales (hr_payroll_run)
Para cada mes (1-12), ejecutar:
```bash
# Endpoint: POST /payroll/run
# Body JSON:
{
  "period_from": "2026-01-01",
  "period_to": "2026-01-31",
  "frequency": "mensual",
  "employee_ids": [1,2,3,4,5,6]  # los 6 empleados
}
```
El sistema devolverá:
- `run_id`
- `status` (initialized → processing → completed)
- `total_gross`, `total_deductions`, `total_employer`, `total_net`
- `tax_rate_version`
- Lista de `payslips` por empleado

> **Validación por mes:** Cada recibo debe tener:
> - `gross` = suma de earnings (sueldos)
> - `deductions` = ISLR + CTS + PENSION + ALIMENT + otras descuentos
> - `employer_cost` = cargas patronales (34% sobre salario base + beneficios)
> - `net_pay` = gross - deductions - employer_charge (lo que el empleado efectivamente recibe)
## B. Capa 1 — Catálogos y configuración base (Día 1-2)

### B.1 Cargar 4 proveedores (res, cerdo, pollo, huevos)
| supplier_id | nombre | rif | telefono | correo | condicion_credito | activo |
|-------------|--------|-----|----------|--------|-------------------|--------|
| PROV-RES-001 | Distribuidora Los Llanos C.A. | J-12345678-0 | 0212-555-1000 | contacto@losllanos.com.ve | 30 dias | 1 |
| PROV-CER-001 | Porcíficio Centro Norte | J-87654321-0 | 0212-555-2000 | ventas@porcico.com.ve | 15 dias | 1 |
| PROV-POL-001 | Poultry Elite C.A. | J-11223344-5 | 0212-555-3000 | compras@poultrileite.com.ve | 7 dias | 1 |
| PROV-HUE-001 | Huevo y Más C.A. | J-44332211-9 | 0212-555-4000 | facturacion@hueymas.com.ve | Contado | 1 |

> **Nota:** Los IDs `supplier_id` deben insertarse con `ON DUPLICATE KEY UPDATE` para que la simulación sea reproducible.

### B.2 Crear categorías de producto (cortes y empaques)
Usar routes:
- `GET /product/category/list` / `data`
- POST `/product/category/save` para cada una:

| category_id | category_name | description |
|-------------|---------------|-------------|
| `1001` | Cortes de Res | Muslo, pechuga, paleta, falda, costilla |
| `1002` | Cortes de Cerdo | Lomo, panceta, tocino, cadera |
| `1003` | Corte de Pollo | Muslo, pechuga, alita, sobrino |
| `1004` | Empaques | Vacíe, bolsa 1kg, bolsa 5kg, bandeja |
| `1005` | Presentación Mayor | Caja 20kg, lote canal |

### B.3 Crear unidades de medida
Usar routes `GET /product/unit/list/data` y `POST /product/unit/save`:
- `unidad_id=1`, `unidad_name="kg"` (kilogramo)
- `unidad_id=2`, `unidad_name="lb"` (libra — opcional)
- `unidad_id=3`, `unidad_name="ud"` (unidad — ej. pollo entero)
- `unidad_id=4`, `unidad_name="caja"` (para venta mayor)

### B.4 Cargar precios de productos (costo y precio de venta)
Los productos base vienen en `product_information` tras las migraciones. Para la carnicería usaremos los IDs heredados del semillero (1001-1008):

| product_id | product_name | product_model | unit | price (Bs.) | cost (Bs.) | is_stock | status |
|------------|-------------|---------------|------|-------------|------------|----------|--------|
| `1001` | Muslo de Res | `MUS-RES` | `kg` | `25.00` | `10.00` | `1` | `1` |
| `1002` | Pechuga de Res | `PEC-RES` | `kg` | `45.00` | `18.00` | `1` | `1` |
| `1003` | Falda de Res | `FAL-RES` | `kg` | `18.00` | `7.00` | `1` | `1` |
| `1004` | Costilla de Res | `COS-RES` | `kg` | `12.00` | `5.00` | `1` | `1` |
| `1005` | Lomo de Cerdo | `LOM-CER` | `kg` | `30.00` | `12.00` | `1` | `1` |
| `1006` | Tocino de Cerdo | `TOC-CER` | `kg` | `22.00` | `9.00` | `1` | `1` |
| `1007` | Pollo Entero | `POL-ENT` | `ud` | `35.00` | `14.00` | `1` | `1` |
| `1008` | Pechuga de Pollo | `PEC-POL` | `kg` | `40.00` | `16.00` | `1` | `1` |

> **Precios mayor:** Se cargarán luego en `product_price_list` con un 18% de descuento al precio de mostrador.

### B.5 Configurar costeo (product_cost_config)
Para cada producto (1001-1008):
- `costing_method = 'YIELD'` (reparto por rendimiento)
- `perishable = 1` (todos son perecederos)
- `shelf_life_days = 7` (por defecto, se ajusta por tipo)
- `track_lots = 1` (lotes con FEFO)
# Guía de Simulación Fiscal Completa - 1 Año
## Empresa: Carnicería "La Central"

---
## A. Preparación inicial (Día 1)

### A.1 Instalar el ERPGo y configurar la base de datos
1. Tener MySQL/MariaDB corriendo (`DB_HOST=127.0.0.1`, `DB_PORT=3306`, `DB_USER=root`, `DB_PASSWORD=` vacío por defecto).
2. Copiar `.env.example` → `.env` y completar:
   ```
   DB_NAME=erpgo
   JWT_SECRET= <generar 64 hexámetros>
   APP_ENV=development
   SERVER_HOST=127.0.0.1
   SERVER_PORT=8080
   GRPC_PORT=9095
   ```
3. Ejecutar:
   ```bash
   erpgow.exe --setup
   # Siguiente -> Configurar empresa -> RIF: J-12345678-9 -> Giro: Carnicería Mayorista y Minorista -> Moneda: Bs.S. -> Finalizar
   # El sistema creará la BD, aplicará migraciones y sembrará la empresa "default".
   ```

### A.2 Verificar migraciones aplicadas
```bash
go build ./...
go vet ./interfaces/web
# Debe verse en verde.
```

### A.3 Confirmar empresa activa
- Entrar en `http://localhost:8080`
- El sistema quedará con la empresa "default" (la única en PyME).
- Se tendrá el catálogo base: 1804 cuentas contables, tipos de IVA, retenciones ISLR/IGTF ya definidos.
## 5. Flujo mensual completo (12 veces)

### 5.1 Estructura de un mes típico

#### Paso 1: Compra de inventario (día 1-3)
- Ejecutar `POST /purchase/save` para cada proveedor
- Ejemplo mes 1 (enero):
  - Proveedor Res: 40 canales de 250 kg = 10.000 kg
  - Proveedor Cerdo: 1 canal 100 kg + 2 medias 50 kg = 200 kg
  - Proveedor Pollo: 30 pollos enteros 1.5 kg = 45 kg
  - Huevos: 50 cajas de 30 unidades = 1.500 unidades

#### Paso 2: Despiece y rendimientos (día 3-4)
- Costo canal se reparte según product_yield (72% rendimiento por defecto)
- Muslo: 18% de masa, Pechuga: 30%, Falda/Costilla: 24% acumulada, Hígado: 4%
- Costo corte = (costo_canal / 100) × porcentaje_rendimiento
- Ejemplo: Canal Bs. 100/kg → Muslo Bs. 18/kg, Pechuga Bs. 30/kg

#### Paso 3: Entrada a bodega con FEFO (día 4-5)
- Lotes creados con fecha_vence = día 5 del mes siguiente
- Stock disponible leído de product_lot (no product_information)
- Validación: `comprobarStock(producto, fecha_venta, cantidad)`

#### Paso 4: Ventas al Mayor (día 6-20)
- `POST /invoiceSave` o `POST /sale/new` tipo `mayor`
- customer_id: CLI-001 (Mayor Fijo) o CLI-003 (Mayor Esporádico)
- payment_term: 30 o 15 días según clase
- items: productos con quantity, unit_price (precio mostrador), discount_pct
- price_list_id = 'mayor' → precio con 18% descuento automático

#### Paso 5: Ventas al Detalle (día 6-20)
- `POST /invoiceSave` o venta POS (`POST /pos/sale`)
- customer_id: CLI-002 o CLI-004 (o sin RIF mostrador)
- payment_term: contado (efectivo/transferencia)
- quantities pequeñas (0.5kg, 1ud, etc.), precio sin descuento o promociones

#### Paso 6: Cierre de caja (día 20-25)
- `POST /cash/close-turn` para cada caja POS
- Reportar: initial_fund, declared_cash, declared_card, total_sales, refunds
- Sistema calcula: difference = declared - (initial + sales - refunds)

#### Paso 7: Liquidación nómina (día 25-27)
- `POST /payroll/run` para período mensual vencido
- Genera: hr_payroll_run, hr_payslip por empleado
- Cada recibo debe tener: gross, deductions, employer_cost, net_pay

#### Paso 8: Aprobación asientos contables (día 27-28)
- `POST /reports/daily-closing` o handler interno
- Mueve asientos de acc_vaucher (borrador) a acc_transaction (libro)
- Validación: `medirInformeCarniceria` debe mostrar:
  - Facturado > 0
  - Descuadre < 0.01 Bs (por redondeo)
  - LibroVacio = false

#### Paso 9: Reportes SENIAT mensuales (día 28-30)
- `GET /report/sales` → Ventas totales
- `GET /report/purchases` → Compras al proveedor
- `GET /report/stock` → Stock actual por producto y lote
- `GET /report/tax` → Retenciones ISLR, IGTF aplicadas
- `GET /report/inventory` → Inventario valorado (FIFO o weighted average)

#### Paso 10: Fiscalización a mitad de año (mes 6)
- **Qué verifica el contador:**
  1. Libro de IVA: totales acc_transaction vs facturas emitidas
  2. Retenciones ISLR/IGTF: coinciden con tasas vigentes y montos pagados
  3. Libro de Compras: todas las purchase tienen invoice de entrada y lotes consumidos
  4. Stock físico vs sistema: reconciliación product_lot vs conteo real
  5. Nómina y prejubilaciones: pagos hr_payslip vs desembolsos bancarios
  6. Clientes y Crédito: saldos due_amount vs pagos recibidos y convenios
  7. Correlatividad facturas: huecos en números de factura (acc_voucher VNo)
- Comandos verificación:
  - `GET /report/tax` → retenciones período
  - `GET /report/sales` → ventas por cliente y categoría
  - `GET /report/stock` → stock disponible por lote
  - `GET /report/purchases` → proveedores y montos adeudados
## 4. Clientes y estrategia de ventas

### 4.1 Clases de cliente (4 tipos)
| customer_class_id | class_name | description | credit_days | discount_pct |
|-------------------|------------|-------------|-------------|--------------|
| 1 | Mayor Fijo | Compra todo los meses, volumen alto | 30 | 10% a partir de $500 |
| 2 | Mayor Esporádico | Compra intermitente, menor volumen | 15 | 5% a partir de $200 |
| 3 | Detalle (Mostrador) | Cliente final, compra por unidad | Contado | 0% |
| 4 | Restaurantes / Comida | Compra regular, granel | 45 | 15% a partir de $1000 |

### 4.2 Perfiles de cliente
| user_id | code | first_name | last_name | email | phone | customer_class_id | status |
|---------|------|------------|-----------|-------|-------|-------------------|--------|
| 1 | CLI-001 | Roberto | Ramírez | roberto@restaurant.com | 0414-555-1111 | 1 (Mayor Fijo) | 1 |
| 2 | CLI-002 | Laura | Flores | laura@casa.com | 0424-555-2222 | 3 (Detalle) | 1 |
| 3 | CLI-003 | Pedro | Gutiérrez | pedro@bodega.com | 0416-555-3333 | 2 (Mayor Esporádico) | 1 |
| 4 | CLI-004 | Sofia | Moreno | sofia@restaurant2.com | 0426-555-4444 | 4 (Restaurante) | 1 |

### 4.3 Estrategias de ventas por épocas festivas

#### Carnaval (febrero)
- Descuento 15% en cortes de res y pollo para pedidos mayoristas >$500
- "Carnaval Pack": 5kg mezcla (res+pollo) por precio de 4kg
- Objetivo: Mayor fijo + Restaurantes

#### Semana Santa (abril)
- Aumento 5% en huevos (alta demanda)
- Descuento 10% en sustitutos de carne roja
- "Paquete Semana Santa": 1 canal res + 30 huevos precio paquete
- Objetivo: Mayor esporádico + Detalle

#### Fin de clases (agosto)
- "Mochila Completa": 2kg pollo + 1kg res + 1 docena huevos
- Descuento 12% pago contado
- Objetivo: Familias (detalle mayoristas)

#### Regreso a clases (septiembre)
- Descuento 8% en empaques familiares (cajas 5kg, 10kg) para mayoristas
- "Primera Compra del Año": 10% descuento primera factura mayorista

#### Navidad (diciembre)
- "Navideña Completa": Canal res entero + 50 huevos + 2kg jamón precio paquete fijo
- Crédito especial: 60 días para mayoristas históricos
- Incremento stock: 20% más canales en noviembre

#### Puente mayo / Verano (mayo-junio)
- Descuento 7% en cortes de cerdo (bbq season)
- "Asador Pack": 3kg tocino + 2kg lomo cerdo + 1kg pollo entero
## 3. Personal y RRHH

### 3.1 Perfiles de empleado (6 personas)
| employee_id | code | first_name | last_name | dept | role | level | wage_monthly |
|-------------|------|------------|-----------|------|------|-------|-------------|
| 1 | ADM-001 | María | Pérez | 1 | admin | master | 45000.00 |
| 2 | COX-001 | Carlos | Gómez | 2 | cajero | employee | 25000.00 |
| 3 | DES-001 | José | Rodríguez | 3 | despachador | employee | 22000.00 |
| 4 | CON-001 | Ana | López | 4 | contador | manager | 35000.00 |
| 5 | PRO-001 | Luis | Martínez | 5 | proveedor | employee | 28000.00 |
| 6 | LIM-001 | Pedro | Suárez | 6 | ayudante | employee | 20000.00 |

Departmentos: 1=Admin, 2=Punto Venta, 3=Despacho, 4=Contabilidad, 5=Proveedores, 6=Limpieza

### 3.2 Tasas ISLR y parafiscales
Tabla: tax_rate_table, vigencia 2026-01-01
- ISLR: 15.00%, base=employer, aplica=employee
- CTS: 10.00%, base=employer, aplica=employee
- PENSION: 5.00%, base=employer, aplica=employee
- ALIMENT: 4.00%, base=employer, aplica=employee

### 3.3 Periodos de nómina
Frequencies:
- Semanal: 52 periodos/año
- Quincenal: 26 periodos/año
- Mensual: 12 periodos/año (default)

### 3.4 Problemas de personal
- **Baja voluntaria**: status=0 en hr_employee, run payroll con status='cancelled'
- **Nueva contratación**: insertar row status=1, ejecutar payroll run
- **Retiro por pensión**: reducir wage a 0, liquidar recibo con concepto RETIRO, dar de baja siguiente mes

### 3.5 Planillas mensuales
Endpoint: `POST /payroll/run`
Body JSON con period_from, period_to, frequency, employee_ids
Devuelve: run_id, status, total_gross, total_deductions, total_employer, total_net, tax_rate_version, payslips por empleado

Validación por recibo:
- gross = suma earnings
- deductions = ISLR+CTS+PENSION+ALIMENT+otros
- employer_cost = cargas patronales ~34%
- net_pay = lo que cobra el empleado
## 2. Catálogos y configuración base

### 2.1 Cuatro proveedores
| supplier_id | nombre | rif | telefono | correo | condicion_credito |
|-------------|--------|-----|----------|--------|-------------------|
| PROV-RES-001 | Distribuidora Los Llanos C.A. | J-12345678-0 | 0212-555-1000 | contacto@losllanos.com.ve | 30 dias |
| PROV-CER-001 | Porcíficio Centro Norte | J-87654321-0 | 0212-555-2000 | ventas@porcico.com.ve | 15 dias |
| PROV-POL-001 | Poultry Elite C.A. | J-11223344-5 | 0212-555-3000 | compras@poultrileite.com.ve | 7 dias |
| PROV-HUE-001 | Huevo y Más C.A. | J-44332211-9 | 0212-555-4000 | facturacion@hueymas.com.ve | Contado |

> Usar `ON DUPLICATE KEY UPDATE` para idempotencia.

### 2.2 Categorías de producto
- 1001: Cortes de Res
- 1002: Cortes de Cerdo  
- 1003: Corte de Pollo
- 1004: Empaques
- 1005: Presentación Mayor

Usar routes: `GET /product/category/list/data`, `POST /product/category/save`

### 2.3 Unidades de medida
- unidad_id=1: kg
- unidad_id=2: lb (opcional)
- unidad_id=3: ud
- unidad_id=4: caja

Routes: `GET /product/unit/list/data`, `POST /product/unit/save`

### 2.4 Precios de productos (1001-1008)
| product_id | name | model | unit | price (Bs.) | cost (Bs.) | is_stock | status |
|------------|------|-------|------|-------------|------------|----------|--------|
| 1001 | Muslo de Res | MUS-RES | kg | 25.00 | 10.00 | 1 | 1 |
| 1002 | Pechuga de Res | PEC-RES | kg | 45.00 | 18.00 | 1 | 1 |
| 1003 | Falda de Res | FAL-RES | kg | 18.00 | 7.00 | 1 | 1 |
| 1004 | Costilla de Res | COS-RES | kg | 12.00 | 5.00 | 1 | 1 |
| 1005 | Lomo de Cerdo | LOM-CER | kg | 30.00 | 12.00 | 1 | 1 |
| 1006 | Tocino de Cerdo | TOC-CER | kg | 22.00 | 9.00 | 1 | 1 |
| 1007 | Pollo Entero | POL-ENT | ud | 35.00 | 14.00 | 1 | 1 |
| 1008 | Pechuga de Pollo | PEC-POL | kg | 40.00 | 16.00 | 1 | 1 |

Precios mayor: 18% descuento sobre precio mostrador → product_price_list

### 2.5 Configurar costeo
Para cada producto (1001-1008):
- costing_method = 'YIELD'
- perishable = 1
- shelf_life_days = 7
- track_lots = 1
# Guía de Simulación Fiscal Completa - 1 Año
## Empresa: Carnicería "La Central"

---
## 1. Preparación inicial

### 1.1 Instalación
- MySQL/MariaDB corriendo con DB_HOST=127.0.0.1, DB_PORT=3306
- Copiar .env ejemplo a . y completar variables básicas
- Ejecutar: `erpgow.exe --setup`
- Seguir el wizard interactivo: RIF, Giro, Moneda

### 1.2 Verificación
```bash
go build ./...
go vet ./interfaces/web
# Verificar que todo esté en verde
```

### 1.3 Empresa activa
- Entrar en http://localhost:8080
- Sistema quedará con empresa "default" (PyME)
- Catálogo base: 1804 cuentas contables, tipos IVA, retenciones ISLR/IGTF
