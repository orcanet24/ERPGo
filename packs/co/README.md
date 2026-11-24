# Pack Colombia — esqueleto para un colaborador

Estado: **borrador** (`seed_version: 0`). **No se ofrece** en el asistente ni
en la empresa hasta que un contador colombiano lo valide.

Este directorio es la lista de trabajo. `seed.sql` no tiene ni una sentencia
ejecutable: todo está comentado a propósito para que nadie facture con datos a
medias.

## Qué hay que llenar

1. **PUC oficial** (clases 1–9) con la jerarquía, y las cuentas que el ERP usa:
   caja, bancos, cartera de clientes y proveedores, IVA descontable y generado,
   inventarios, costo de ventas, ingresos y retenciones.
2. **Cuentas por defecto** en `acc_predefine_account` para cada papel
   (columnas listadas en `seed.sql`; el código de cada una sale del PUC).
3. **Impuestos**: IVA 19 %, IVA 5 %, excluido/exento. El ERP cobra el impuesto
   por renglón (producto), así que aquí se documentan y se configuran al
   instalar.
4. **Series/documentos** (`fiscal_series`): prefijo y rango autorizado por la
   DIAN. No se inventan.
5. **Retenciones**: conceptos, tarifas y numeración.
6. **Factura electrónica**: addon que implemente el contrato `digitalinvoice`
   contra un proveedor tecnológico autorizado.

## Criterios de aceptación (para pasar a `validado`)

- Un contador colombiano revisa y firma estos casos sobre una base de prueba:
  - factura con IVA 19 %, IVA 5 % y exenta;
  - factura con descuento (porcentaje y valor);
  - nota de crédito que revierte una factura;
  - retención (fuente / ICA / IVA) con su numeración;
  - redondeo a 2 decimales y precio con impuesto incluido.
- Escribe su nombre en `validado_por` y sube `seed_version` a `1`.
- Recién entonces el registro (`interfaces/web/pais.go`) puede marcar el país
  como disponible.

## Fuentes

Ver `packs/co/pack.json` → `fuentes`. Cada dato del seed debe poder
responderse con una de esas fuentes (o una norma equivalente citada).

## Lo que no se acepta

- Tasas o códigos de cuenta “aprendidos de memoria”.
- Países marcados `validado` sin firma.
- SQL que borre o sobrescriba datos existentes.
- Facturación electrónica simulada: sin certificación se declara, no se finge.
