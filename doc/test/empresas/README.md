# Empresas de simulación

**Ruta**: `doc/test/empresas/<empresa>/`

## Qué es esto

Un banco de pruebas real para el módulo de costos: cuatro empresas de distinto tipo, cada una
con su plan, sus minutas, su benchmark y su ciclo económico de 5 meses.

La idea no es generar datos bonitos. Es que **el mismo módulo** tenga que costear cuatro
negocios que fallan de maneras distintas. Si el motor aguanta las cuatro, sirve; si solo sabe
costear una, es un módulo de esa empresa con otro nombre.

## El directorio de cada empresa

| Archivo | Qué contiene |
|---|---|
| `00-plan-inicial.md` | Qué vende, qué método de costeo, qué tablas necesita |
| `01-minutas.md` | Decisiones tomadas, con el motivo y las descartadas |
| `02-benchmark.md` | Contra qué se compara y qué número hay que alcanzar |
| `03-ciclo-economico-5-meses.md` | Clientes, proveedores, compras, nómina, inventario, ventas |

Las **minutas** existen porque una decisión sin motivo escrita se reabre. Si dentro de seis
meses alguien pregunta "¿por qué el rendimiento de la carnicería es 72%?", la respuesta está
ahí y no se reconstruye de memoria.

## Las cuatro empresas

| Empresa | Fabrica | Inventario | Método dominante | Lo que pone a prueba |
|---|---|---|---|---|
| [Carnicería Lara](carniceria-lara/00-plan-inicial.md) | Sí (despiece) | Sí, perecedero | `YIELD` | Rendimiento y doble lista de precio |
| [Textilera El Telar](textilera-el-telar/00-plan-inicial.md) | Sí | Sí | `RECIPE` + `IMPORT` | Merma por componente y aduana |
| [Distribuidora Inca](autopartes-inca/00-plan-inicial.md) | No | Sí | `IMPORT` | Que lo simple se quede simple |
| [Softwara](softwara/00-plan-inicial.md) | No | **No** | `HOURS` | Que un negocio sin inventario entre |

La cuarta, Inca, es de autopartes: compra producto terminado e importa, no fabrica. Se añadió
como comprobación de que **el módulo no obliga a configurar nada que un negocio no necesite**:
ni receta, ni horas, ni rendimiento.

## Regla de cada empresa

1. **Todo dato que se crea debe producir un movimiento contable coherente.** Si la empresa
   compra, se genera el asiento; si factura, se genera la cobranza. Los datos que no pasan por
   las funciones del ERP no validan nada — y es exactamente el error que se evita así.
2. **Los totales se comprar contra la contabilidad**, no contra el SQL. Un informe que cuadra
   consigo mismo no prueba nada.
3. **El costo se congela al cerrar el mes** (`cost_snapshot`). Recalcular un mes pasado no
   cambia lo facturado.

## Orden de montaje

Primero el módulo, después las empresas:

1. ✅ `schema_z_costos.sql` — 13 tablas
2. ✅ `cost_engine.go` — los 5 métodos, cálculo puro y trazabilidad
3. ⬜ Registro de las tablas nuevas para la IA (whitelist + departamento)
4. ⬜ `00-plan-inicial.md` de las cuatro
5. ⬜ Datos de las cuatro empresas
6. ⬜ Verificación contra contabilidad

El paso 3 no es cosmético: si no se registra, la IA no ve las tablas **ni siendo IT**. Es el
fallo que hizo que RRHH viera 0 tablas durante semanas. El procedimiento completo está en
[MAPA-DINAMICO-BD-PARA-LA-IA.md](../../MAPA-DINAMICO-BD-PARA-LA-IA.md).
