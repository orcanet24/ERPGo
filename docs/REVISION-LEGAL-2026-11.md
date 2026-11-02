# Revisión legal — noviembre de 2026 (Providencias SENIAT 0093/0094 y jurisprudencia salarial)

**Advertencia metodológica (leer antes de usar):** el texto oficial no fue leído directo.
Los PDF de la Gaceta Oficial 43.484 no se pudieron descargar (solo resúmenes de firmas
tributarias y glosas de prensa: HLB Marambio, SoyTecnología, LStributos, Gerencia y
Tributos) y el sitio histórico del TSJ (`historico.tsj.gob.ve`) no respondió. Todo
artículo y fecha de este informe proviene de fuentes de terceros y debe validarse contra
la Gaceta y la sentencia original antes de parametrizar el sistema.

## 1. Inventario de las dos normas

| Norma | Materia | Gaceta | Vigencia | Alcance salarial |
|---|---|---|---|---|
| SNAT/2026/00093 | Régimen general de emisión de facturas, notas de débito/crédito, comprobantes de retención e IGTF | N° 43.484 (21/10/2026) | 180 días continuos → **≈ 19-20 de abril de 2027** | ninguna |
| SNAT/2026/00094 | Reorganización interna del SENIAT (recursos jerárquicos, cobro ejecutivo, medidas cautelares) | N° 43.473 (05/10/2026) | inmediata | ninguna |

**Ninguna de las dos providencias habla de salarios, ajustes salariales ni
nómina**: esa premisa de la consulta era errónea. La 00094 no cambia nada operativo de un
ERP (es pura reorganización orgánica; el contribuyente no modifica ni un formato). Todo el
impacto real recae en la 00093.

## 2. Impacto de la 00093 sobre los procesos de facturación del ERPGo

| Imperativo de la norma | Estado en el ERPGo | Qué queda por hacer |
|---|---|---|
| Número de control digital: 2 dígitos + secuencial de 8 desde `00-00000001` | fiscal.go ya emite `00-00000001` y lo reserva transacionalmente (invoice.go:428) | ★ mantener |
| Esquema 14 caracteres `AAAAMM + 8 secuenciales` en comprobantes de retención IVA/ISLR | **ya implementado por artículo 18** del esquema anterior; retencion_numero.go valida exactamente ese formato (línea 105) | añadir el **correo del proveedor obligatorio** en el comprobante |

| Fin de los talonarios/forma libre como medio ordinario: solo contingencia | n/a para el ERP (el ERP nunca fue talonario) | si el cliente venía con formas libres migradas, quedan fuera |
| Maquinas fiscales dejan de ser obligatorias (opcional) | el modo contingencia digital del ERP (contingencia_digital.go) ya existe | adecuar el mensaje del documento si el cliente mezcla máquina fiscal + ERP |
| Hora de emisión con segundos `HH.MM.SS a.m./p.m.` y fecha `DDMMAAAA` | revisar la plantilla de factura | el separador y el formato exacto se levantan de la Gaceta (art. 8.5) |
| Multidivisa obligatoria en el documento: monto en divisa + contravalor en Bs + tasa BCV aplicada simultáneos | el núcleo ya registra la tasa BCV por factura (invoice.go:375-399) y la audita | garantizar que factura, nota de débito/crédito, y POS impriman los tres valores |
| Nota de débito/crédito por el MISMO medio y con referencia (correlativo + control + fecha + monto de la factura afectada) | ya existe: las NC llevan su propio número de control y referencian el documento (invoice.go:878+) | ★ mantener; validar las referencias al activar la revisión 2027 |
| Consumidor final: cédula/pasaporte en lugar de RIF si no hay crédito fiscal | el alta de cliente EXIGE RIF por norma SENIAT anterior | ⚠️ excepción a permitir con riesgo controlado: no romper la exigencia de RIF cuando haya crédito fiscal. No implementar sin leer primero el condicionado del art. 10.4. |
| Contingencia: registro y envío del PDF vía imprenta digital o API del SENIAT al cesar el evento | contingencia_digital.go existe; revisar que el resumen diario responda `PDF + API` | integración con el nuevo Sistema Centralizado de Facturación del SENIAT |
| Leyenda obligatoria conforme a esta Providencia | sin implementar | cuando se conozca la leyenda exacta |

**Prioridad de proyecto que se abre:** ventana de 180 días desde el 21/10/2026, con
`go run` del ERP listo para abril de 2027. La integración con el Sistema Centralizado de
Facturación del SENIAT es el frente técnico principal (la 00093 lo exige para medios
digitales — Portal/Web/App del SENIAT o Proveedor Tecnológico autorizado — y obliga a
acceso permanente del fisco a los sistemas del proveedor).

## 3. Jurisprudencia salarial (TSJ, 2024-2026) sobre la nómina del ERPGo

El horizonte salarial lo componen **12 criterios de la Sala de Casación Social**. Los de
mayor peso de cálculo:

| Sentencia | Fecha | Criterio operable |
|---|---|---|
| N° 306, exp. 22-154 | 26/07/2024 | Prestaciones por bolívares → literal «c» (30 días/año); por divisas → literales «a»+«b» (15/trimestre + adicionales). **Cálculo separado por moneda**, nunca consolidado. |
| N° 099, exp. 24-143 | 31/03/2025 | La causa del pago (movilización) decide, no la etiqueta: la «ayuda de traslado» no es salario. |
| N° 292, exp. 24-074 | 08/08/2025 | Salario variable → base de prestaciones = **promedio de los últimos 6 meses**. |
| N° 364, exp. 24-231 | 13/08/2025 | Suspensión laboral sin autorización de la Inspectoría → no se suspende el salario: devengan igual. |
| N° 523, exp. 24-058 | 13/11/2025 | El complemento de cestaticket en efectivo/divisas **no es salario** (art. 105.2) salvo pacto expreso. |
| N° 340, exp. 25-327 | 06/08/2026 | Si no se prueba el salario en divisas, base = salario mínimo del decretado vigente. |
| N° 127, exp. 20-070 | 12/08/2022 | Vacaciones NO integran el salario normal para utilidades. |
| N° 021, exp. 19-327 | 09/03/2022 | La carga de probar el carácter no salarial es del **patrono**; addendum posterior al reclamo no desnaturaliza. |

### Brechas frente a la nómina actual del ERPGo

El motor actual (payroll.go) trabaja con UT, salario mínimo como base parafiscal y una
lista de conceptos con `kind`/`rate_pct`. Para estar «a norma» con la doctrina queda:

1. **Catálogo de conceptos con atributos jurídicos**, no lista fija: `causa del pago`,
   `regularidad` (test de frecuencia parametrizable), `moneda`, `soporte_requerido`,
   `art_105_supuesto`, `pacto_expreso_salarial`. El carácter salarial se deriva de esos
   atributos, nunca del nombre del concepto.
2. **Tres bases separadas**: básico (extras y recargos), **salario normal** art. 104
   (utilidades, vacaciones, bono vacacional, feriados) y **salario integral** art. 122
   (prestaciones art. 142: alícuotas de bono vacacional y utilidades encima).
3. **Prestaciones art. 142 por moneda**: correr los tres literales en paralelo (a, b, c)
   y pagar el mayor, **separado por la porción en Bs y en divisa**, no consolidado.
4. **Prorrateo semestral** cuando el salario sea variable (parámetro «salario variable»
   por empleado que activa el promedio de 6 meses).
5. **Bloqueo de estatus** «suspensión» sin número/providencia de la Inspectoría
   referenciada (devengan igual, sentencia 364/2025).
6. **Histórico de decretos de salario mínimo con vigencias** (base de condena de la
   340/2026): hoy la fila del mínimo se lee as-is, sin repositorio temporal de vigencias.
7. **Reporte de riesgo salarial**: conceptos recurrentes sin justificación de art. 105 y
   sin soporte → bandera roja (bajo la 021/2022, probar que no son salario es del
   patrono).

## 4. Qué NO se pudo confirmar

- Texto oficial de las gacetas 43.473 y 43.484 (PDF no descargable): todas las citas de
  artículos son de segunda fuente.
- Régimen sancionatorio específico de la 00093 (solo referencias genéricas del COT).
- Motiva íntegra de las sentencias 292/2025, 099/2025, 523/2025, 167/2026 y 191/2026:
  elaboradas vía repositorios (IAJURISS, Badell & Grau, etc.), no el original del TSJ.
- No existe fallo de la Sala Constitucional venezolana en materia salarial 2024-2026.
- Ningún texto de providencia de la Inspectoría del Trabajo del período.

## 5. Recomendación de orden de trabajo

1. Confirmar el encabezado de la 00093 contra la Gaceta 43.484 (fuente primaria) y sólo
   entonces abrir las historias de facturación: hora con segundos, leyenda, multidivisa
   visible en el documento, desglose del comprobante de retención con correo del proveedor.
2. Nómina a norma: empezar por el catálogo de conceptos con atributos jurídicos y el
   histórico de salario mínimo — son prerrequisitos de los tres cálculos separados por
   moneda del art. 142.
3. La integración con el Sistema Centralizado de Facturación del SENIAT es el punto que
   puede retrasarse a tiempo: investigar si el cronograma emite a proveedores tecnológicos
   y cuál es la API de contingencia esperada.
