# Plan de internacionalización de ERPGo

> Objetivo del dueño (2026-11-24): que ERPGo sirva a los comercios de la región
> con **sus** normas y leyes —Colombia, Perú, Ecuador, Chile, Panamá, Bolivia,
> El Salvador, México, Guatemala, Honduras, Brasil—. Proyecto libre (GPLv3), sin
> política, con el modelo de packs por país y validación local.
>
> **La regla que no se negocia:** un país sale a la venta cuando su pack está
> marcado `validado` (contador local) o `certificado` (ambiente oficial del
> regulador). Un pack a medio validar no se presenta como «cumple».

## 1. Diagnóstico del código actual (medido, no supuesto)

Lo que **ya está a favor**:

- Tasas, impuestos y cuentas viven en **datos** (`vat_tax_setting`,
  `tax_rate_table`, `fiscal_setting`, `payment_method`, `acc_coa` + catálogo
  CONASEV). Un país nuevo es, en gran parte, datos.
- Existe un **punto de extensión para facturación electrónica**:
  `interfaces/web/digitalinvoice_processor.go` + addons por empresa
  (`addon_code`, `strong_id`, PDF, estados). Es el enchufe natural de
  DIAN/SUNAT/SRI/DGI/FEL/DTE.
- Multiempresa, multimoneda y terminal POS (POSGo2) ya resueltos y libres.

Lo que **hoy ata a Venezuela**:

- 77 archivos mencionan SENIAT/IGTF/CONASEV/providencias; 95 mencionan BCV/VES.
- IGTF como impuesto especial cableado; retenciones IVA/ISLR venezolanas;
  libros legales de compras/ventas; numeración y textos legales.
- Hasta hoy **no existía la dimensión país**. Ya está creada la primera pieza:
  `company_information.country_code` (migración `schema_z_pais_empresa.sql`,
  default `VE`), el registro `interfaces/web/pais.go` (moneda, decimales,
  precio con impuesto incluido, fuente de tasa de cambio, addon de facturación
  electrónica y **estado** del pack) y sus pruebas.

## 2. Arquitectura: el país es una dimensión de primer nivel

1. **Empresa con país.** `company_information.country_code` + `tenants.country_code`
   (catálogo de control). El asistente de instalación y la pantalla de empresa
   solo ofrecen países con `PackDisponible`.
2. **Pack de país (datos versionados).** En `migrations/packs/<cc>_*.sql`:
   plan de cuentas, impuestos por defecto, cuentas por defecto
   (`acc_predefine_account`), series/documentos, retenciones y textos legales.
   Activar país = aplicar el pack; sin tocar código.
3. **Hooks de comportamiento (pocos y explícitos).** No un framework de plugins:
   - redondeo y decimales de la moneda;
   - precio con impuesto incluido (Colombia/Perú muestran el precio final);
   - fuente de tasa de cambio (BCV/TRM/SUNAT/…);
   - numeración y formato de documentos;
   - cálculo de retenciones;
   - **contrato de facturación electrónica**: `emitir`, `consultar`, `anular`,
     `pdf`. Implementado por addon (proveedor tecnológico) por país.
4. **Estado del país.** `borrador` → `validado` → `certificado`, declarado en
   `pais.go` y visible en la UI del pack.

## 3. Dificultad real por país (y por qué no son todos iguales)

| País | Impuesto | Factura electrónica | Dificultad | Notas |
|---|---|---|---|---|
| Colombia | IVA 19/5, exentos | DIAN (por proveedor tecnológico) | **Media** | PUC, retenciones; el proveedor tecnológico hace la validación DIAN |
| Perú | IGV 18 | SUNAT CPE (factura electrónica) | **Media** | PCGE, retenciones; SUNAT publica esquemas y beta |
| Ecuador | IVA 15 | SRI comprobantes (XML + firma XAdES) | **Media-alta** | Firma electrónica en la caja; certificado digital |
| Panamá | ITBMS 7 | DGI factura electrónica | Media | Documentación accesible |
| El Salvador | IVA 13 | DTE (Ministerio de Hacienda) | Media | Esquemas JSON/XML |
| Guatemala | IVA 12 | FEL (web service) | Media | Certificación por entorno |
| Honduras | ISV 15 | SAR facturación | Media-alta | Documentación menos pública |
| Bolivia | IVA 13 | SIAT en línea | Media | Depende de conexión |
| México | IVA 16 | CFDI 4.0 + PAC + SAT | **Alta** | Certificación de PAC, timbrado, complementos |
| Chile | IVA 19 | DTE/SII (XML + firma + certificación) | **Alta** | Certificación de documentos |
| Brasil | ICMS estatal + ISS municipal + federal | NF-e/CT-e/NFS-e, SPED | **Muy alta** | Tres niveles de gobierno y formatos distintos; requiere equipo |

## 4. Secuencia decidida (2026-11-24)

**Venezuela primero; después un país cada 1–2 meses.** La decisión del dueño es
la correcta por una razón de arquitectura: el **core operativo es el mismo** en
todos los países (facturación, kardex, contabilidad, caja, POS, auditoría,
multimoneda, migraciones). Lo que cambia por país es la **capa fiscal**: tasas,
retenciones, plan de cuentas, documentos/series, libros legales, fuente de tasa
de cambio y facturación electrónica. Por eso el esfuerzo de cada país se
concentra donde está el riesgo (leyes y normas), no en reescribir el ERP.

| Compartido (no se toca por país) | Capa país (lo que se investiga y valida) |
|---|---|
| Facturación, kardex, asientos, cierre, caja, POS, auditoría, multitenant, migraciones, seguridad | Impuestos y tasas; retenciones; plan de cuentas y cuentas por defecto; documentos y series; libros legales; tasa de cambio; facturación electrónica |

**Cadencia objetivo:** un país cada 1–2 meses, con esta condición: el pack no
sale de `borrador` sin la firma de un contador del país. Si no hay validador, el
país espera: se prefiere el calendario honesto a once banderas.

**Orden:** Venezuela (consolidación) → Colombia → Perú → Ecuador → Panamá →
El Salvador → Guatemala → Honduras → Bolivia → (equipo/socio) México, Chile,
Brasil.

## 5. Olas de trabajo

- **Ola 0 (ahora): Venezuela.** Consolidar el pack de origen: cerrar los
  formatos de libros legales y la revisión periódica de providencias. Es la
  base que ya opera y la que da el molde de “pack validado”.
- **Ola 1: Colombia y Perú.** Mercado grande, impuesto simple y facturación
  electrónica delegable a proveedor tecnológico. Sirven de molde para los
  demás packs.
- **Ola 2:** Ecuador, Panamá, El Salvador, Guatemala, Honduras, Bolivia.
- **Ola 3:** México, Chile y Brasil. Estos no se abordan sin equipo o socio
  certificado; hacerlo a medias es el mayor riesgo reputacional del proyecto.

## 6. Checklist para abrir un país (proceso, no suerte)

1. **Norma escrita por un contador local:** impuestos y tasas, reglas de
   redondeo, retenciones, documentos obligatorios, moneda y decimales.
2. **Pack de datos:** plan de cuentas del país + cuentas por defecto + series
   + textos legales. Se marca `borrador` hasta el paso 4.
3. **Hooks:** completar los comportamientos que el país necesite (precio con
   impuesto incluido, retenciones, tasa de cambio).
4. **Validación contable:** un profesional del país revisa y firma los casos
   (factura con descuento, nota de crédito, retención, redondeo). Pasa a
   `validado`.
5. **Facturación electrónica:** integrar el proveedor/regulador en un addon que
   cumpla el contrato; probar en ambiente de pruebas. Pasa a `certificado`.
6. **Prueba de campo:** una empresa real del país opera un mes completo
   (facturación, cierre, libros). Solo entonces se anuncia el país.
7. **Mantenimiento:** las normas cambian; cada pack tiene versión y ficha de
   cambios, como las migraciones.

## 7. Riesgos y límites (honestos)

- **No se puede prometer «cumplimiento» sin certificación.** Cada país tiene su
  regulador y sus formatos; el pack declara su estado.
- **E-invoicing requiere proveedor o certificación propia.** Para Colombia y
  Perú se puede delegar en proveedores tecnológicos con API; México/Chile exigen
  trámites propios.
- **Brasil, México y Chile son proyectos aparte.** Intentarlos sin equipo es la
  forma más rápida de quemar credibilidad.
- **Soporte local**: sin un contador/socio por país, el pack se pudre con el
  primer cambio de tasa. El modelo GPL permite que ese socio viva del soporte.

## 8. Lo ya hecho (primer corte)

- `migrations/schema_z_pais_empresa.sql`: `company_information.country_code`
  (default `VE`, instalaciones existentes intactas).
- `tenants.country_code` en el catálogo de control (CREATE + ALTER).
- `interfaces/web/pais.go`: registro de países con moneda, decimales, precio
  con impuesto incluido, fuente de tasa, addon de e-invoicing, `PackDisponible`
  y `estado` (borrador/validado/certificado); helpers de normalización y de
  lectura del país de la empresa.
- `interfaces/web/pais_test.go`: Venezuela es el origen; un país sin pack **no
  se ofrece**; un código desconocido no inventa país; invariantes del registro;
  la columna nace con default y se lee bien sobre la cadena real de migraciones.
- **Kit del colaborador** (`packs/`): contrato del pack, plantilla
  (`packs/_PLANTILLA/pack.json`), fichas de Venezuela (legado, `seed_version=1`),
  Colombia y Perú en borrador, y validador (`interfaces/web/pack_pais.go` +
  pruebas) que no deja que un pack en borrador se ofrezca, que un «validado» no
  diga quién lo firmó ni que un borrador tenga sentencias ejecutables.
  `packs/README.md` es la guía de 7 pasos para contribuir un país.
- **Esqueleto del pack Colombia** (`packs/co/seed.sql` + `packs/co/README.md`):
  la lista de trabajo del colaborador —PUC, cuentas por defecto, impuestos,
  series, retenciones, DIAN— con todo comentado: nada se aplica hasta que un
  contador lo valide y suba `seed_version` a 1.
- Nada más se toca: el default `VE` garantiza que toda la base existente sigue
  igual.

## 9. Siguientes pasos concretos

1. **Runner de pack POR EMPRESA** (antes de cualquier semilla real): las
   semillas de un país NO pueden vivir en `migrations/` global, porque se
   aplicarían a todas las empresas y una empresa venezolana terminaría con el
   PUC colombiano. Hace falta una tabla `country_pack_migrations` por empresa y
   un aplicador al crear/abrir la empresa según su `country_code`.
2. **Selección de país en el asistente de instalación y en la empresa**
   (solo `paisesDisponibles()`).
3. **Pack Colombia v1 en `borrador`**: el esqueleto ya está; falta que un
   contador complete PUC, retenciones y cuentas por defecto y lo valide.
4. **Contrato formal del addon de facturación electrónica** (documentado sobre
   `digitalinvoice`) para que cada proveedor/país lo implemente.
5. **Primer contador aliado** (Colombia) que valide el pack: sin él, el pack se
   queda en borrador y así se mostrará. Perú después, reutilizando el molde.

---

*Documento vivo. Cada país que entra al registro con `PackDisponible` debe
tener aquí su fila, su estado y su responsable de validación.*
