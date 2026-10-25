Aquí tienes el **Manual de Diseño y Especificaciones Técnicas** para implementar en el ERP el **Libro / Registro de Entradas y Salidas de Inventario**, en estricto cumplimiento con el **Artículo 177 del Reglamento de la Ley de Impuesto sobre la Renta (RISLR)** de Venezuela.

---

# Manual de Especificaciones Técnicas

## Registro de Entradas y Salidas de Inventario (Art. 177 RISLR)

---

### 1. Marco Legal y Fundamento Técnico

El **Artículo 177 del Reglamento de la Ley de Impuesto Sobre la Renta (RISLR)** impone a las empresas comerciales, industriales y de servicios la obligación tributaria de mantener un registro de inventario permanente.

**Requisitos Normativos Clave:**

1. **Método de Valuación Obligatorio:** Debe utilizarse estrictamente el método del **Costo Promedio Ponderado** (CPP) para derivar el Costo de Ventas legal ante el SENIAT.
2. **Prohibición de Salvos Negativos:** No pueden registrarse existencias físicas ni montos valorizados en negativo bajo ningún concepto.
3. **Disponibilidad:** Debe poder ser consultado, filtrado e impreso por el SENIAT en cualquier momento (mensual o quincenalmente).
4. **Agrupación:** Debe estructurarse por producto/código SKU y en orden cronológico detallado.

---

### 2. Estructura y Diseño de Columnas (Formato SENIAT)

El reporte o vista del sistema se presenta en formato de **Kárdex Valorizado en Bolívares (VES)** a la Tasa oficial, dividido operacionalmente en tres bloques: **Entradas**, **Salidas** y **Existencias (Saldos)**.

#### Encabezado del Reporte (Mandatorio)

```text
EMPRESA: [Nombre de la Razón Social]
RIF: [J-00000000-0]
REGISTRO DE ENTRADAS Y SALIDAS DE INVENTARIO (ART. 177 RISLR)
PERÍODO: DESDE [DD/MM/YYYY] HASTA [DD/MM/YYYY]
MÉTODO DE VALUACIÓN: COSTO PROMEDIO PONDERADO
CÓDIGO DE PRODUCTO / SKU: [COD-001] | DESCRIPCIÓN: [Descripción del Artículo]

```

#### Tabla de Movimientos Kárdex

| Col. | Nombre del Campo | Tipo de Dato | Formato / Regla de Negocio |
| --- | --- | --- | --- |
| **1** | Fecha | Date | `DD/MM/YYYY` (Fecha efectiva de la transacción) |
| **2** | Concepto / Tipo Mov. | Enum / String | Compras, Ventas, Devolución Compras, Devolución Ventas, Ajuste+, Ajuste-, Auto-Consumo/Producción |
| **3** | N° Documento | String | N° Factura, N° Nota de Control, N° Guía de Recepción o N° Ajuste |
| **4** | Cantidad Entrada | Decimal | Unidades físicas que ingresan (0.00 si no aplica) |
| **5** | Costo Unit. Entrada | Decimal | Costo de adquisición unitario en Bs. |
| **6** | Total Bs. Entrada | Decimal | $\text{Cantidad Entrada} \times \text{Costo Unit. Entrada}$ |
| **7** | Cantidad Salida | Decimal | Unidades físicas que egresan (0.00 si no aplica) |
| **8** | Costo Unit. Salida | Decimal | **Costo Promedio Ponderado** vigente al momento de la salida |
| **9** | Total Bs. Salida | Decimal | $\text{Cantidad Salida} \times \text{Costo Unit. Salida}$ |
| **10** | Cantidad Existencia | Decimal | Stock físico remanente tras la operación ($Q_{anterior} + Q_{in} - Q_{out}$) |
| **11** | Costo Promedio Unit. | Decimal | Nuevo Costo Promedio Ponderado recalculado |
| **12** | Total Bs. Existencia | Decimal | $\text{Cantidad Existencia} \times \text{Costo Promedio Unit.}$ |

---

### 3. Reglas Matemáticas y Algoritmo de Cálculo

El motor del ERP debe ejecutar el recálculo del **Costo Promedio Ponderado** en cada evento de entrada al inventario:

#### A. Al registrar una ENTRADA (Compra o Ajuste Positivo):

$$\text{Nuevo Costo Promedio} = \frac{\text{Total Bs. Existencia Previa} + \text{Total Bs. Entrada}}{\text{Cantidad Existencia Previa} + \text{Cantidad Entrada}}$$

*La nueva existencia reflejará el costo unitario actualizado.*

#### B. Al registrar una SALIDA (Venta, Devolución sobre Compra, Mermas, Auto-consumo):

* La salida **NO altera** el Costo Promedio Ponderado unitario.
* La salida valoriza el egreso al **Costo Promedio Ponderado vigente en la fila anterior**:

$$\text{Total Bs. Salida} = \text{Cantidad Salida} \times \text{Costo Promedio Unit. Previsto}$$



$$\text{Nuevo Total Bs. Existencia} = \text{Total Bs. Existencia Previa} - \text{Total Bs. Salida}$$

---

### 4. Cuadrante de Conciliación Contable (Exigencia Auditoría SENIAT)

Al final del informe de inventarios, el ERP debe generar un resumen consolidado por período que permita a los fiscales cruzar el libro de inventario con el **Libro Mayor y la Declaración de ISLR**:

```text
========================================================================
RESUMEN GENERAL DE CONCILIACIÓN DEL PERÍODO (EN BOLÍVARES)
========================================================================
(+) Inventario Inicial (Saldo al Inicio del Período):    Bs. XXX,XXX.XX
(+) Total Compras/Entradas del Período (Col. Total Entradas): Bs. XXX,XXX.XX
(-) Inventario Final (Saldo al Cierre del Período):     Bs. XXX,XXX.XX
------------------------------------------------------------------------
(=) COSTO DE VENTAS DEL PERÍODO:                         Bs. XXX,XXX.XX
========================================================================

```

> **Validación de Control:**
> 1. El valor acumulado en **Total Entradas (Bs.)** durante el mes debe coincidir exactamente con la sumatoria de las **Compras del Libro de Compras de IVA** y el saldo de la cuenta contable de Compras/Mercancía en Tránsito.
> 2. El resultado del **Costo de Ventas del Período** debe cuadrar con el asiento contable cargado en la cuenta de Costos del Libro Mayor.
> 
> 

---

### 5. Lógica de Base de Datos y Tipos de Transacciones

Para permitir una auditoría transparente, la tabla transaccional debe catalogar el tipo de operación de acuerdo a las clasificaciones del SENIAT:

```sql
CREATE TABLE kardex_inventario_art177 (
    id BIGSERIAL PRIMARY KEY,
    producto_id BIGINT NOT NULL,
    fecha_transaccion TIMESTAMP NOT NULL,
    tipo_documento VARCHAR(20) NOT NULL, -- 'FACTURA_COMPRA', 'FACTURA_VENTA', 'AJUSTE_DANO', 'AUTOCONSUMO'
    numero_documento VARCHAR(50) NOT NULL,
    
    -- Bloque Entradas
    cantidad_entrada NUMERIC(18, 4) DEFAULT 0,
    costo_unitario_entrada NUMERIC(18, 4) DEFAULT 0,
    total_entrada_bs NUMERIC(18, 2) DEFAULT 0,
    
    -- Bloque Salidas
    cantidad_salida NUMERIC(18, 4) DEFAULT 0,
    costo_unitario_salida NUMERIC(18, 4) DEFAULT 0,
    total_salida_bs NUMERIC(18, 2) DEFAULT 0,
    
    -- Bloque Existencia / Saldos
    cantidad_saldo NUMERIC(18, 4) NOT NULL,
    costo_promedio_saldo NUMERIC(18, 4) NOT NULL,
    total_saldo_bs NUMERIC(18, 2) NOT NULL,
    
    usuario_id BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT chk_saldo_no_negativo CHECK (cantidad_saldo >= 0)
);

```

---

### 6. Controles de Fiscalización e Inalterabilidad para el Developer

1. **Restricción de Negativos (`CHECK constraint`):**
Si una venta genera un saldo negativo de stock (ej. $-1$ unidad), el backend **debe denegar la transacción** inmediatamente. Ante el SENIAT, las existencias negativas se asumen como presunción de ventas no declaradas u omisión de facturas de compra.
2. **Histórico e Inalterabilidad:**
No se deben permitir comandos `UPDATE` ni `DELETE` directos sobre las filas ya procesadas del Kárdex de Inventario. Si hay un error administrativo, debe registrarse un documento de **Ajuste de Inventario** (Suma o Resta) que quede reflejado de forma transparente con su respectiva justificación y acta de conteo físico.
3. **Manejo de Moneda y Tasa de Cambio:**
Si el ERP permite precios/costos en USD o EUR, la tabla de Kárdex debe registrar la conversión formal al tipo de cambio oficial publicado por el BCV para la fecha del documento, garantizando la presentación exclusiva en **Bolívares (VES)**.