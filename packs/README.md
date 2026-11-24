# Packs de país — el contrato del colaborador

Un **pack** es lo que convierte a ERPGo en un ERP de otro país. Y es el mismo
artefacto para las dos victorias del proyecto:

- Si aparece un **colaborador** (contador o desarrollador del país), completa el
  pack y gana el proyecto: la comunidad crece.
- Si aparece un **cliente**, usa el pack y gana quien lo atiende: soporte,
  instalación y puesta a punto.

No hay fork: el pack es datos + ficha, no una copia del código.

## Qué contiene un pack

```
packs/<cc>/
  pack.json          ← la ficha: estado, qué hay, qué falta, quién validó
migrations/
  packs_<cc>_1_*.sql ← las semillas reales (plan de cuentas, impuestos, cuentas
                       por defecto, series), aplicadas por el ledger normal
```

`<cc>` es el código ISO 3166-1 alfa-2 en minúsculas (`co`, `pe`, `ve`).

## La ficha (`pack.json`)

| Campo | Para qué |
|---|---|
| `code` | ISO alfa-2 en mayúsculas. Debe coincidir con el registro del core (`interfaces/web/pais.go`) |
| `name`, `currency` | Nombre y moneda ISO 4217 |
| `estado` | `borrador` → `validado` → `certificado` |
| `legacy_embebido` | `true` solo para Venezuela: su pack vive en las migraciones históricas |
| `validado_por` | Obligatorio si `estado` es `validado` (quién firmó la revisión) |
| `certificado_por`, `ambiente_pruebas` | Obligatorios si `estado` es `certificado` |
| `pendientes` | Obligatorio no vacío si `estado` es `borrador`: la lista honesta de lo que falta |
| `impuestos`, `plan_de_cuentas`, `factura_electronica` | Resumen de lo que trae el pack |

Regla de hierro: **un pack en `borrador` no es seleccionable** en el asistente ni
en la pantalla de empresa. Un país sin pack no se ofrece. Esto no es burocracia:
si alguien factura con un pack a medias, el que pierde es el contador que firmó.

## Cómo contribuir un país (7 pasos)

1. **Norma**: conseguí la información oficial del país (impuestos, tasas,
   retenciones, documentos obligatorios, plan de cuentas). Nada de memoria.
2. **Ficha**: copiá `packs/_PLANTILLA/pack.json` a `packs/<cc>/pack.json` y
   completá lo que sepas. Lo demás va a `pendientes`.
3. **Semillas**: escribí `migrations/packs_<cc>_1_*.sql` (idempotente, sin
   `DROP`, con `INSERT`/`UPDATE` portables). El plan de cuentas, los impuestos
   por defecto, las cuentas de cartera y las series.
4. **Core**: agregá el país a `interfaces/web/pais.go` con `PackDisponible:
   false` mientras el estado sea `borrador`.
5. **Validación**: un contador del país revisa los casos (factura con
   descuento, nota de crédito, retención, redondeo). Se escribe `validado_por`.
6. **Facturación electrónica**: implementá el contrato `digitalinvoice` en un
   addon (proveedor tecnológico o regulador). Se prueba en el ambiente de
   pruebas y se llena `certificado_por` + `ambiente_pruebas`.
7. **Prueba de campo**: una empresa real opera un mes completo. Recién entonces
   el país se anuncia.

## Qué se gana

- El **colaborador** que valida el pack queda como referente de soporte de su
  país: la comunidad le trae clientes y el proyecto le da la herramienta.
- El **cliente** recibe un ERP fiscal de su país sin licencias en dólares.
- El **proyecto** gana en los dos casos, porque el artefacto es el mismo.

## Lo que no se acepta

- Packs que “adivinan” tasas o códigos de cuentas sin fuente.
- Países marcados `validado` sin firma de un contador.
- SQL que borre o sobrescriba datos existentes (la política del README manda).
- Facturación electrónica simulada: sin certificación se declara, no se finge.
