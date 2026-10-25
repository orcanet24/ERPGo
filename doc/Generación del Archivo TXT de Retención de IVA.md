# Manual de Especificaciones Técnicas e Integración ERP

## Generación del Archivo TXT de Retención de IVA (SENIAT Venezuela)

---

### 1. Visión General del Requerimiento

El SENIAT (Servicio Nacional Integrado de Administración Aduanera y Tributaria de Venezuela) exige a los Agentes de Retención de IVA la presentación mensual de las retenciones practicadas a sus proveedores mediante un archivo plano de texto plano (`.TXT`) delimitado por tabulaciones (`\t`).

Este manual especifica las reglas de negocio, estructura de datos, validaciones y pseudocódigo/implementación técnica requerida para que el módulo de compras/impuestos del ERP genere este archivo de forma automatizada partiendo de la base de datos transaccional.

---

### 2. Estructura de Campos y Especificaciones Técnicas

El archivo consta de **16 columnas por registro (línea)**. Las columnas deben estar separadas estrictamente por el carácter de **Tabulación (`\t`)** y finalizadas con un salto de línea Estándar `CRLF` (`\r\n`) o `LF` (`\n`).

| Col. | Campo en Guía SENIAT | Tipo de Dato | Formato / Longitud | Descripción / Regla de Negocio |
| --- | --- | --- | --- | --- |
| **A** | RIF del Agente de Retención | String | Alfanumérico (10 car.) | RIF de la empresa local (ej: `J000000001`). Sin guiones ni espacios.

 |
| **B** | Período Impositivo | String | `YYYYMM` (6 car.) | Año y mes correspondiente al período declarado (ej: `202609`).

 |
| **C** | Fecha de Factura | Date | `YYYY-MM-DD` | Fecha de emisión del documento del proveedor.

 |
| **D** | Tipo de Operación | String | Char (1 car.) | Siempre `'C'` para compras/gastos sujetas a retención.

 |
| **E** | Tipo de Documento | String | Numérico (2 car.) | `'01'`: Factura<br>

<br>`'02'`: Nota de Débito<br>

<br>`'03'`: Nota de Crédito

 |
| **F** | RIF del Proveedor | String | Alfanumérico (10 car.) | RIF del sujeto retenido (ej: `J000000002`). Sin guiones ni espacios.

 |
| **G** | Número de Documento | String | Alfanumérico | Número correlativo del documento/factura de compra.

 |
| **H** | Número de Control | String | Alfanumérico | Número de control de la factura impresa/fiscal (ej: `00-01212`).

 |
| **I** | Monto Total del Documento | Decimal | `0.00` | Total general del documento (Base + IVA + Exento).

 |
| **J** | Base Imponible | Decimal | `0.00` | Monto sujeto a la alícuota de IVA declarada.

 |
| **K** | Monto del IVA Retenido | Decimal | `0.00` | Monto del IVA efectivamente retenido al proveedor.

 |
| **L** | Número de Doc. Afectado | String | Alfanumérico | `'0'` si es Factura. Si es Nota de Crédito/Débito, colocar el N° de factura original.

 |
| **M** | N° Comprobante Retención | String | 14 dígitos (`YYYYMMXXXXXXXX`) | Número del comprobante emitido por el ERP (ej: `20260900000001`).

 |
| **N** | Monto Exento de IVA | Decimal | `0.00` | Monto de bienes/servicios exentos o no sujetos al IVA.

 |
| **O** | Alícuota | Decimal | `0.00` | Porcentaje de IVA aplicado a la base (ej: `16.00`, `8.00` o `0.00`).

 |
| **P** | Número de Expediente | String | String/Zero | Asignado en procesos administrativos especiales. Por defecto: `'0'` o espacios en blanco según la norma habitual.

 |

---

### 3. Reglas de Negocio Validaciones Críticas

Para evitar errores en la carga al Portal SENIAT, el motor del ERP debe ejecutar las siguientes validaciones previas antes de permitir la descarga del archivo:

1. **Formato de Montos y Decimales:**
* Todos los campos numéricos con decimales (I, J, K, N, O) deben llevar exactamente dos decimales separados por punto (`.`), no por coma.


* No deben incluir separadores de miles (ejemplo válido: `1000.00`; no permitido: `1.000,00`).




2. **Formato del RIF:**
* Debe consistir en la letra del tipo de persona (`J`, `V`, `E`, `G`, `P`) seguida de 9 dígitos numéricos rellenados con ceros a la izquierda (total 10 caracteres). Sin guiones.




3. **Validación de la Matemática Impositiva:**

$$\text{Monto Total} = \text{Base Imponible} + \text{Monto IVA} + \text{Monto Exento}$$


$$\text{Monto IVA Retenido} = (\text{Base Imponible} \times \frac{\text{Alícuota}}{100}) \times \% \text{ Retención Aplicada}$$


* *Si la ecuación matemática no concuerda, la plataforma SENIAT rechazará la fila con un error impositivo*.




4. **Manejo de Períodos "Sin Movimiento" (Pago Cero):**
* Si en el mes no existieron compras sujetas a retención, el archivo NO debe estar vacío.


* Se debe generar una única fila de encabezado rellenando los campos numéricos y de texto con ceros (`0`), manteniendo únicamente el RIF de la Empresa (Columna A) y el Período Impositivo (Columna B).





---

### 4. Arquitectura de Datos Sugerida (Database Mapping)

Para consultar los registros en la base de datos del ERP, se sugiere estructurar la consulta SQL sobre las tablas de **Facturas de Compra / Cuentas por Pagar (AP)** y **Comprobantes de Retención**:

```sql
SELECT 
    UPPER(REPLACE(empresa.rif, '-', '')) AS col_a_rif_agente,
    TO_CHAR(retencion.fecha_periodo, 'YYYYMM') AS col_b_periodo,
    TO_CHAR(factura.fecha_emision, 'YYYY-MM-DD') AS col_c_fecha_factura,
    'C' AS col_d_tipo_operacion,
    LPAD(factura.tipo_documento_codigo, 2, '0') AS col_e_tipo_doc, -- '01', '02', '03'
    UPPER(REPLACE(proveedor.rif, '-', '')) AS col_f_rif_proveedor,
    factura.numero_factura AS col_g_num_doc,
    factura.numero_control AS col_h_num_control,
    TRIM(TO_CHAR(factura.monto_total, '999999990.00')) AS col_i_monto_total,
    TRIM(TO_CHAR(factura.base_imponible, '999999990.00')) AS col_j_base_imponible,
    TRIM(TO_CHAR(retencion.monto_iva_retenido, '999999990.00')) AS col_k_iva_retenido,
    COALESCE(factura_afectada.numero_factura, '0') AS col_l_doc_afectado,
    retencion.numero_comprobante AS col_m_num_comprobante,
    TRIM(TO_CHAR(factura.monto_exento, '999999990.00')) AS col_n_monto_exento,
    TRIM(TO_CHAR(factura.porcentaje_alicuota, '999999990.00')) AS col_o_alicuota,
    '0' AS col_p_num_expediente
FROM retenciones_iva retencion
INNER JOIN compras factura ON retencion.factura_id = factura.id
INNER JOIN proveedores proveedor ON factura.proveedor_id = proveedor.id
INNER JOIN empresas empresa ON factura.empresa_id = empresa.id
LEFT JOIN compras factura_afectada ON factura.factura_afectada_id = factura_afectada.id
WHERE retencion.periodo_year_month = :periodo_seleccionado
ORDER BY retencion.numero_comprobante ASC;

```

---

### 5. Algoritmo / Pseudocódigo de Generación (Backend)

```python
def generar_txt_retenciones_seniat(empresa_rif, periodo_str, registros_retencion):
    """
    Genera el contenido en texto delimitado por tabulaciones para SENIAT.
    :param empresa_rif: str (ej: 'J000000001')
    :param periodo_str: str (ej: '202609')
    :param registros_retencion: list of dicts con las retenciones del mes
    :return: str conteniendo la data del TXT
    """
    lineas = []
    
    # Caso 1: Período sin movimientos (Declaración en Cero)
    if not registros_retencion:
        # Col A a P separadas por tabulación con valores en cero
        fila_cero = [
            empresa_rif,  # A: RIF Agente
            periodo_str,  # B: Período
            "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0" # C a P
        ]
        return "\t".join(fila_cero) + "\r\n"

    # Caso 2: Período con registros de retención
    for reg in registros_retencion:
        fila = [
            str(empresa_rif).upper().replace("-", ""),              # Col A
            str(periodo_str),                                      # Col B
            reg['fecha_factura'].strftime('%Y-%m-%d'),              # Col C
            "C",                                                    # Col D (Compras)
            str(reg['tipo_documento']).zfill(2),                   # Col E ('01','02','03')
            str(reg['proveedor_rif']).upper().replace("-", ""),     # Col F
            str(reg['numero_factura']),                            # Col G
            str(reg['numero_control']),                            # Col H
            "{:.2f}".format(float(reg['monto_total'])),             # Col I
            "{:.2f}".format(float(reg['base_imponible'])),          # Col J
            "{:.2f}".format(float(reg['monto_iva_retenido'])),     # Col K
            str(reg['doc_afectado']) if reg['doc_afectado'] else "0", # Col L
            str(reg['num_comprobante']),                           # Col M
            "{:.2f}".format(float(reg['monto_exento'])),            # Col N
            "{:.2f}".format(float(reg['alicuota'])),                # Col O
            str(reg.get('expediente', '0'))                        # Col P
        ]
        lineas.append("\t".join(fila))
        
    return "\r\n".join(lineas) + "\r\n"

```

---

### 6. Protocolo de Pruebas e Integración (QA)

1. **Prueba de Formato Interno:**
* Verificar que al abrir el archivo `.txt` generado con un editor de texto plano (VSCode, Notepad++), los valores no contengan comillas dobles, comas como decimales, ni espacios de relleno no deseados.


2. **Prueba de Carga en el Portal SENIAT (Entorno de Pruebas):**
* Ir al portal web oficial del SENIAT en la sección de **"Sistemas en Línea / Retención de IVA (Prueba de Carga de Archivo)"**.


* Subir el archivo plano exportado por el ERP.


* **Resultado Esperado:** La plataforma debe devolver el mensaje: *"La revisión se ha realizado satisfactoriamente... El archivo de prueba no posee errores"* indicando las líneas procesadas y el monto total declarado.


* En caso de arrojar discrepancias o errores de estructura, revisar la columna señalada por la bitácora del portal SENIAT para ajustar los formateadores de la consulta SQL/código del ERP.