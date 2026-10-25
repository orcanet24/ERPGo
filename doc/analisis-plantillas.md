# Análisis de plantillasSENIAT / contables rifles — qué aportan a ERPGo

**Fecha:** 2 de octubre de 2026
**Origen:** `doc/plantillas/` (31 archivos: 6 xlsx/xlsm, 1 xls grande, 22 xls de
planillas, 3 PDF normativa, 1 PDF instructivo)
**Herramienta de lectura:** `tools/plantillas/extraer_tablas.py` (+ scripts de volcado)

---

## 1. Respuesta corta

**Sí se pueden leer y son muy aprovechables.** De los 31 archivos:

| Estado | Archivos | Detalle |
|---|---|---|
| **Leídos completos** | 7 | Los 6 xlsx/xlsm + `SISTEMA_CONTABLE EXCEL.xls` |
| **Cifrados con clave** | 18 | Los `.xls` de "PLANILLAS DE CONTABILIDAD" (`xlrd` y `msoffcrypto` los rechazan) |
| **Leídos solo como normas** | 6 | PDFs (LOPJ, Providencia 070/2015, Providencia 003/2014, instructivo) |

Los 18 cifrados **no se pierden**: por su nombre ya revelan el catálogo de módulos
que un contador venezolano espera encontrar (abajo, §4). Y los 7 leídos contienen
**datos duros** que el sistema hoy no tiene.

> Para leer los 18 restantes hace falta la clave con la que el SENIAT publica esas
> planillas (el usuario debe proporcionarla o volver a descargarlas sin protección).

---

## 2. Lo que sí aportan datos duros (ya extraídos a CSV)

Extraídos en [doc/plantillas_extraidas/](plantillas_extraidas/) por
`tools/plantillas/extraer_tablas.py`:

| Archivo | Hoja | Qué aporta | Artefacto |
|---|---|---|---|
| `SISTEMA_CONTABLE EXCEL.xls` | **PC** (1.882 filas) | **Plan de cuentas CONASEV completo**: 1.855 cuentas, jerarquía de 2 a 6 dígitos, tipo (A/P) y grupo | `conasev_plan_de_cuentas.csv` |
| `SISTEMA_CONTABLE EXCEL.xls` | **Tipo_Document** (52) | Catálogo de tipos de documento SENIAT con código (`01` factura, `07` NC, `08` ND, `20` comprobante de retención, …) | `seniat_tipo_documento.csv` |
| `SISTEMA_CONTABLE EXCEL.xls` | **Tipo_Vouchers** | Tipos de voucher y su correlativo (`00` apertura, `01` ventas, `02` compras, `03` ingresos, `04` egresos, `05` diario) | `tipo_voucher.csv` |
| `Calculos Salariales 2013.xlsm` | **InteresPres** | **Tabla de intereses de prestaciones sociales LOT/LOTTT (art. 108)**: matriz mes × año 1997-2016, con el cambio de LOT a LOTTT desde mayo 2012 | `intereses_prestaciones_lot.csv` |
| `Calculos Salariales 2013.xlsm` | **Salario minimo** | Histórico de salario mínimo 1989-2013 (base de las escalas LOTTT por antigüedad) | `salario_minimo_historico.csv` |
| `Calculos Salariales 2013.xlsm` | 20 hojas | **Motor de nómina completo con las fórmulas exactas** (ver §3) | — |
| `COMPROBANTE-RETENCION-MENSUAL-ISLR.xlsx` | 52 | Modelo de comprobante de retención ISLR (Decreto 1.808 / GO 36.203, numeración `AAAA/MM/00000001`) | — |
| `COMPROBANTES DE RETENCION IVA E ISLR N°2.xlsm` | — | Comprobante **IVA (art. 11 Ley IVA, 75% de retención) + ISLR** unificado, con código de archivo XML y alícuota de retención parametrizable | — |
| `formato de retención de iva…xlsx` | — | Segundo formato de retención IVA (compras internas/importaciones, notas de débito/crédito, factura afectada) | — |
| `ESTRUCTURA DE COSTOS 2017 SUNDDE.xlsx` | 12 meses | Cálculo de **estructura de costos y precio justo** (Providencia 070/2015 + Ley de Precios Justos): margen 30% sobre costo, 60% total, IVA, no imputable vs imputable | — |
| `CALCULADORA JET…2020.xlsx` | 4 | Calculadora de IVA con retención de 75%, y conversores Bs⇄USD⇄EUR por tasa | — |
| `Asignacion_Costos.xls` | 5 | **Centros de costo** y reparto de costos por operación/porcentaje | — |
| `Control de Stock.xls`, `Remito.xls`, `Caja Diaria.xls`, `Resumen de viaticos.xls` | — | Patrones de esos reportes | — |

---

## 3. El regalo mayor: motor de nómina LOTTT con las fórmulas exactas

`Calculos Salariales 2013.xlsm` es una aplicación de nómina completa (20 hojas,
funciones VBA propias: `amensual`, `horad`, `horan`, `horaen`, `lottt192`). Trae
**resueltos con fórmula** los cálculos que hoy ERPGo **no tiene**:

**Prestaciones sociales (LOT/LOTTT art. 108)** —ayers, que es el cálculo más
delicado: interés = (promedio de depósitos del año) × tasa oficial del mes × días/360…
La plantilla trae la tabla oficial mes × año completa, incluido el truncamiento de
5 días de LOT (mayo 2012) que dispara LOTTT.

**Liquidación de finiquito** con los artículos del LOTTT: antigüedad (art. 141), intereses
de antigüedad (143), vacaciones (190), bono vacacional (192), utilidades fraccionadas (131),
sustitución por preaviso (92), y **prestaciones ×2 en despido injustificado** (art. 186).

**Recibo de pago** con catálogo de conceptos y códigos: `001` Sueldo, `101` Horas
extras, `102` Comisiones, `104` Caja de ahorro (aporte patrono), `200` Anticipo,
`201` IVSS, `202` Reg. prestacional de empleo, `204` LPH, `205`–`209` otros.

**Horas extras, nocturnas y feriados** con turnos (T1 00:00-06:00 / T2 06:00-22:00 /
T3 22:00-00:00): recargo 50% extra diurna, 30% nocturna, 50% feriado.

**Vacaciones y utilidades**: 21 + 1 día por año (máx. 15), bono vacacional 15 + 1,
120 días de utilidades, INCE 0,5%, y la **reciprocidad** entre trabajadores.

**Salario integral** diario = base/30 + (días utilidades/360)×(base/30) + (días bono vacacional/360)×(base/30).

**Cesta ticket**: 0,25 UT × 22 días, solo si el sueldo ≤ 3 salarios mínimos.

**Deducciones de ley**: IVSS 4% (techo 5 salarios mínimos), paro forzoso 0,5%
(techo 10 mínimos), LPH 1%, calculadas sobre los **lunes del mes** (`Lunes C`).

> Nota: el `payroll.go` actual del sistema (330 líneas) cubre IVSS obrero/patrono,
> INCEPS/FAOV/BPSI, brackets ISLR y salario mínimo, pero **no** prestaciones,
> vacaciones, utilidades, salario integral, horas extras ni finiquito. Esta
> plantilla llena exactamente ese hueco.

---

## 4. Módulos que revelan los 18 archivos cifrados

Aunque no se puedan abrir, sus nombres son la lista de lo que un contador venezolano
usa a diario — y casi nada de eso existe todavía:

| Planilla (cifrada) | Módulo que falta |
|---|---|
| Caja Diaria / Caja con Cuentas | **Caja con detalle por cuenta** (ERPGo tiene sesión de caja, falta el asiento por cuenta del día) |
| Cuentas a Cobrar / Listado Cobranza / Seguimiento de Recibos / Planes de cuotas | **CxC con aging y cobranza** (cobranzas y recibe) |
| Ordenes de Pedido / Registro de Compromisos | **Órdenes de compra / compromisos con proveedores** |
| Remito / Seguimiento de Remitos / Control de Stock | **Guías de remisión y entrada/salida de almacén** |
| Forma de Pago / Recibos / Recibo | **Recibos de pago imprimibles y formas de pago** |
| Rendición y Resumen de Viáticos | **Módulo de viáticos** |
| Registro de Llamadas Telefónicas / Seguimiento de horas por Clientes | **CRM mínimo: horas y llamadas por cliente** (facturable) |
| Equivalencias Monetarias | **Equivalencias UTM/UT** (ya hay módulo de cambio) |
| Administrador de Valores | **Cartera de valores / inversiones** |
| Ingreso_Gastos / Tablero de Comando | **Ingresos y gastos + tablero de indicadores** |

---

## 5. Advertencia de vigencia (importante)

Los datos son de **2013–2020**, así que **no se pueden copiar como constantes**:

- `CALCULADORA JET` usa **IVA 16%** — hoy es **18%** (mayo 2025).
- El IVA de la retención del art. 11 es 75% del impuesto (sigue vigente), pero la
  alícuota base cambió.
- La tabla de intereses de prestaciones **termina en 2016**: hay que seguir cargando
  las tasas oficiales anuales (SENIAT/IVSS publica un % por mes).
- Salario mínimo y UT desactualizados.
- La estructura de costos alude a la **Ley de Precios Justos** y a la Providencia
  070/2015, ambas ya derogadas o sustituidas por normas posteriores (ley de precios
  justos 2022+).

**Por eso la forma correcta de usarlas es como *plantilla de cálculo parametrizada
por fecha*, no como dato fijo** — que es exactamente el patrón que ya usa el sistema
en `tax_rate` (`loadTaxRates(asOf)` con versionado). Aplicado así, una actualización
normativa se resuelve cambiando la tabla de tasas, no el código.

---

## 6. Orden de implementación sugerido (por retorno fiscal)

1. **Plan de cuentas CONASEV** — el CSV ya está listo (1.855 cuentas, jerarquía
   2→6 dígitos). Es la base de todo: hoy el sistema usa cuentas predefinidas
   sueltas; con CONASEV el libro contable, los estados financieros y el
   balance de comprobación quedan alineados con lo que pide el SENIAT.
2. **Núcleo LOTTT de nómina** — prestaciones con tabla oficial, vacaciones,
   bono vacacional, utilidades, salario integral, horas extras, finiquito y
   recibo de pago. Es lo que la plantilla resuelve mejor y hoy falta entero.
3. **Comprobantes de retención IVA (art. 11, 75%) e ISLR** — con los dos formatos
   oficiales, numeración y salida para el archivo XML.
4. **CxC con aging y cobranza** — el faltante más visible del día a día.
5. **Estructura de costos / precio justo** — informe que se arma solo con los datos
   que ya tiene el sistema (costo de compra, inventario y precio de venta), por
   producto y mes.
6. **Centros de costo** — reparto por operación/porcentaje (patrón de `Asignacion_Costos`).

---

## 7. Cómo reproducir la lectura

```bash
python tools/plantillas/extraer_tablas.py     # regenerate CSVs
```

Requiere `xlrd` (para los `.xls`) — el resto se lee con la librería estándar de Python.