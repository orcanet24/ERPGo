# Informe de etapa — La empresa cárnica que vive

> Una carnicería de barrio, un mes completo, y al final la pregunta que
> importa: ¿los números cuadran entre sí?

Fecha: 4 de octubre de 2026
Prueba: `interfaces/web/datos_carniceria_e2e_test.go` → `TestEmpresaCarnicaVive`
Módulo de la hoja de ruta: **Etapa 4 — Pruebas de datos**

---

## 1. Qué es esto y por qué

Un ERP no se rompe dentro de una función: se rompe **entre** funciones. Entre
el lunes y el martes. El producto que caduca la misma semana en que se factura
al paralelo y se cuenta el inventario a fin de mes.

Por eso la prueba no es una lista de casos sueltos. Es **una historia**: una
carnicería que abre, contrata, compra, vende, anota, cobra y cierra el mes, y al
final se le pide que sus propios números cuadren.

La prueba corre contra **MySQL**, que es el motor de producción, y crea su
propia base (`erpgo_test_...`) que borra al terminar.

### Los tres cuadres que se exigen al final

| Cuadre | Qué dice si falla |
|---|---|
| La contabilidad cuadra | Suma del debe = suma del haber, sobre el libro real (`acc_transaction`) |
| El stock histórico = la suma de los lotes | El ERP calcula el stock de dos maneras distintas y tienen que decir lo mismo |
| Lo que se debe cobrar = lo que dicen las facturas | En cada factura, `total = pagado + por cobrar` |

Los tres son invariantes, no cifras esperadas: valen igual el mes que sea, con
la cantidad que sea. Eso es lo que los hace útiles como red de seguridad.

---

## 2. La historia, etapa por etapa

### 1. La empresa abre y contrata

Cuatro personas: carnicero, deshuesador, vendedora y un limpiador.
Con ellos viene el motor de nómina (`hr_employee`, `hr_salary_assignment`,
`hr_payroll_run`), que es donde se comprueba que el esquema de RRHH aplica entero.

### 2. Tres proveedores y seis productos

Matadero (res, costillar, filete de pollo), avícola y un distribuidor de
carnes procesadas. Catálogo con código de barras, RIF y costo.

> **Trampa del motor:** `product_information.product_id` es TEXTO y genera
> ocho letras al azar; los módulos nuevos (lotes, reorden, conteo) guardan el
> producto como ENTERO. MySQL resuelve la comparación convirtiendo el texto y
> `"a1b2c3d4"` contra `1` da **cero filas sin error**. Un `WHERE` que no
> devuelve nada y no se queja. Por eso este escenario usa códigos numéricos
> (1001…1006) y por eso existe `numProducto()`.

### 3. Dos compras con lote y caducidad

- Día −20: 200 kg de res (caduca en 4 días), 120 de costillar, 100 de pollo.
- Día −12: 180 de pollo, 120 de pechuga, 60 de chorizo.

Los lotes caducan a **distinto ritmo**, y para cuando se vende parte de la
primera carne la segunda sigue fresca. Ese es exactamente el caso en el que
FEFO importa, y es el que no se puede comprobar con un solo lote.

### 4. Tres clientes, tres formas de pagar

Un supermercado que compra todos los días a crédito, un restaurante que paga
una vez por semana, y un puesto que paga al contado. Tres comportamientos de
cobro distintos en la misma cartera es lo que hace que la prueba valga.

### 5. Tipo de cambio, diez días

La serie Venezuela: la oficial casi plana (36,20 → 36,65) y la paralela con una
desviación que crece (un 6% el primer día, un 15% el último). Es el caso para el
que existe el módulo.

### 6. Un mes de ventas (11 facturas, 3.291,15)

La carne que caduca primero es la que más se vende: es lo que hace una carnicería
bien llevar. El filete de pollo casi no se toca, y ese es el que se convierte en
merma al cierre del mes.

**Comprobación de resultado, no de llamada:** el lote `L-RES-A` (200 kg) debía
quedar en **65 kg**, y quedó en 65. Eso demuestra que FEFO chose el lote que
caducaba antes y no el más conveniente.

### 7. La carne próxima a caducar avisa antes de perderse

Alertas a 30 días: **3.654,00 en riesgo, de los cuales 2.156,40 son críticos**
(caducan esta semana). El detalle por lote, con días que faltan y coste.

### 8. Nota de crédito y devolución

Una factura se anuló entera porque el cliente se llevó mercadería que no llegó.
Es de las dos correcciones que una carnicería hace cada semana.

### 9. Cobros

Se cobró el 75% de la cartera, **factura por factura y de la más antigua a la
más reciente**: 2.088,26 de 2.784,35; quedan 696,09.

> **Carencia del producto, anotada a propósito:** el ERP **no tiene un módulo de
> cobros que liquide la factura**. El cobro se registra como depósito bancario
> contra la cuenta de clientes y el saldo de la factura hay que liquidarlo a
> mano. El ERP sí sabe *leer* lo que se debe (el informe de cartera), pero no
> *cerrarlo*. Está en la hoja de ruta como funcionalidad pendiente.

### 10. Punto de pedido

Tres políticas, con la ventana de demanda en **diez días** y no noventa: en una
carnicería la rotación de la res cambia cada semana y un promedio de tres meses
ordenaría mal el pedido.

| Producto | Existen | Media/día | Punto de pedido | A pedir | Urgencia |
|---|---|---|---|---|---|
| Res de res | 65,00 | 13,50 | 67,50 | 100,00 | warning |

La cantidad a pedir se **redondea al paquete de compra al final**, no antes: si
se redondeara cada paso, el resultado acumularía error y la orden al proveedor
sería una cantidad que nadie pidió.

La política sin plazo de entrega se **rechaza**: con plazo 0 el punto de pedido
sería solo el colchón, y el sistema callaría que hay que comprar.

### 11. Inventario cíclico

Conteo de la categoría **A** (la res y el corte, que son el 80% del valor):
2 líneas. Se encontraron 5 kg de menos de res y 2 kg de más de corte —lo que
pasa de verdad en un almacén donde el corte se despacha por encima y la res se
pierde en la vitrina.

Al cerrar: 2 ajustes, valor **−20,80**, **un solo asiento** (JV-13) por la
diferencia neta. El lote de res quedó en **60 kg**, y el histórico de compras
quedó en el mismo número: los dos caminos de calcular el stock coinciden.

### 12. El contador aprueba y los números cuadran

Con `is_autoapprove_v` encendido (como viene la instalación) cada factura se
aprueba sola y al cierre no queda nada pendiente. Con esa opción apagada, es el
contador quien aprueba. En cualquiera de los dos casos, el libro tiene que
contener movimientos: **26 asientos** en `acc_transaction`.

### 13. El informe de cambio dice la verdad

Diez días registrados, diez filas en la comparación oficial/paralelo, y todas
con la paralela por encima de la oficial. Mil dólares a hoy:

- Oficial: **36.650,00**
- Paralelo: **42.147,50**
- **Diferencia: 5.497,50**

La tasa de facturación configurada manda en la conversión de facturas, y una
fuente inventada (`dolarblue`) se rechaza: mandar a facturar al valor de un
número de teléfono es peor que no tener el módulo.

---

## 3. Bugs reales que encontró esta prueba

Ninguno de estos los inventó nadie: los encontró la prueba, y los cinco
están arreglados.

### 3.1 `acc_vaucher.CreateBy` y `approvedBy` son INT, y el ERP les pasa un nombre

**Síntoma:** una compra, una factura o un conteo se guardaban **sin su asiento**,
sin mensaje y sin rastro. El descuadre aparecía semanas después, al cerrar el
mes.

**Causa:** esas dos columnas son enteros heredados del PHP original, y el ERP
pasa por todas partes el **nombre de usuario**, que es texto. Con MySQL en modo
estricto eso es `Error 1366: Incorrect integer value: 'carnicero' for column
'CreateBy'`.

**Por qué no se veía:** las veinticinco llamadas del proyecto a
`insertVoucherRowTx` descartan el error con `_ =`. El fallo era un silencio.

**Arreglo:** `voucherCreateBy()` traduce a entero cuando el usuario es numérico
—que es lo que guarda `users.id`— y a 0 cuando es un nombre. El nombre completo
queda en `acc_transaction.CreateBy`, que sí es VARCHAR.

> Merece la pena decirlo claro: **apagar el error no es arreglarlo**. Si
> `insertVoucherRowTx` sigue devolviendo errores que nadie mira, el próximo
> descuadre vuelve a ser invisible.

### 3.2 Aprobar un asiento nunca funcionó con MySQL

**Síntoma:** `Error 1292 (22007): Incorrect date value: '2026-10-04T00:00:00Z'
for column 'VDate'`.

**Causa:** el DSN va con `parseTime=true`, y las columnas `DATE` vuelven como
`time.Time`. `approveVoucher` las escaneaba a `string` y las escribía tal cual.
El error se come la transacción entera, así que **el posting se deshace**: el
libro se queda sin movimiento y el cuadre da cero.

**Arreglo:** `fechaBD()` normaliza a `YYYY-MM-DD`, con `""` para NULL.

### 3.3 El `UPDATE` de cobro que no es un cobro

Encontrado dentro de la propia prueba, y es el mismo error que haría cualquiera:

```sql
UPDATE invoice SET paid_amount = paid_amount + 2088.26 WHERE due_amount > 0
```

No da error, no avisa, y **paga el importe completo a cada factura con saldo**,
porque el valor de la derecha se evalúa una vez para todas las filas. Resultado:
una cartera donde cada cliente debe cero y se le ha pagado más de lo que
facturó.

El desbarajuste se vio semanas después en el cuadre: *facturado 3.291,15,
cobrado 15.124,62*.

### 3.4 `productSave` respondía "actualizado" sin haber actualizado

Hacía un `UPDATE` ciego con un `product_id` que podía no existir, y contestaba
que había guardado. El alta de un producto nuevo con el ERP ya en uso se
perdía en silencio. Ahora hace `INSERT` si no existe.

### 3.5 `sessionFromRequest(nil)` petaba

Un `panic` por pedirle los datos de sesión a una petición que no existe. Ahora
tiene guarda.

---

## 4. Cómo se ejecuta

```bash
# MySQL tiene que estar arriba
GOFLAGS=-p=1 go test ./interfaces/web/ -run TestEmpresaCarnicaVive -count=1 -v -timeout 20m
```

Crea su propia base, aplica las 17 migraciones (428 sentencias), ejecuta el mes
entero y borra la base al terminar. No toca los datos de la empresa.

```
facturado en el mes: 3291.16
valor en riesgo: 3654.00 (crítico 2156.40)
cobrado 2088.26 de 2784.35; queda 696.09
sugerencia: Res de res    existen 65.00, media/día 13.50, punto 67.50, pedir 100.00 (warning)
conteo CC-000001 cerrado: 2 ajustes, valor -20.80, asiento JV-13
asientos en el libro: 26
facturas: 11, total: 3291.15, pagado: 2595.06, por cobrar: 696.09
1000 USD: oficial 36650.00, paralelo 42147.50, diferencia 5497.50
--- PASS: TestEmpresaCarnicaVive (11.58s)
    --- PASS: la contabilidad cuadra
    --- PASS: el stock del histórico es el mismo que la suma de los lotes
    --- PASS: lo que se debe cobrar es lo que dicen las facturas
```

---

## 5. Lo que esta prueba NO cubre (y hay que saber)

1. **El consumo de stock en la venta se hace en la prueba, no dentro de
   `invoiceSave`.** El ERP original descuenta el stock por la diferencia entre
   compras y ventas; los módulos de caducidad llevan su propio contador de lote.
   Que los dos coincidan es lo que se comprueba al final, pero **que se
   sincronicen solos es una mejora pendiente**: hoy una venta que se hace por el
   punto de venta no toca el lote.
2. **No hay cobro real.** Ver §2, etapa 9.
3. **No hay concurrencia.** Un mes entero escrito de uno en uno. La prueba de
   carga va aparte (`tools/estress`).
4. **Los cuatro módulos nuevos no se ejercitan por HTTP**, sino por sus
   funciones. Las plantillas y las rutas están cableadas, pero el recorrido de
   un usuario por la pantalla de conteo cíclico no está probado de extremo a
   extremo.

---

## 6. Estado de los cuatro módulos al cierre de la etapa

| Módulo | Estado | Prueba |
|---|---|---|
| Toma de inventario cíclico | Completo (abrir, contar, cerrar, ajustar lote e histórico, un asiento por la diferencia neta) | Etapas 11 y cuadres |
| FEFO con alertas de caducidad | Completo (reparto por caducidad, consumo, alertas con severidad y valor en riesgo) | Etapas 3, 6, 7 |
| Tipo de cambio BCV/paralelo | Completo (oficial y paralelo por día, diferencia, costo de conversión, fuente de facturación) | Etapas 5, 13 |
| Punto de pedido / reposición | Completo (políticas, demanda media, punto de pedido, sugerencias redondeadas a paquete, estados con motivo) | Etapa 10 |