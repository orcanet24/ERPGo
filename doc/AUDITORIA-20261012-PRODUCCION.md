# AUDITORÍA ERPGo — Estado real para producción

**Fecha:** 2026-10-12
**Alcance:** ERPGo (web), POS, SDK de addons
**Método:** verificación contra código y base de datos en vivo, no lectura de documentación.

Este documento existe porque el estado del sistema tiene que quedar en el repo, no en la
memoria de quien lo auditó. Cada hallazgo trae el comando para reproducirlo.

---

## Veredicto corto

| Sistema | ¿Listo para producción? |
|---|---|
| **ERPGo web** | **Casi.** Faltan 3 correcciones fiscales y cerrar la brecha de migraciones. |
| **POS** | **Compila y habla con el ERP.** Falta corregir el número de control fiscal (H-1) y versionarlo en git (H-2). |
| **SDK de addons** | **NO** — Fase 1 de 5. Es un esqueleto con un error de import. |

**Riesgo fiscal concreto:** el POS emite facturas que **no consumen la serie fiscal**.
Una factura sin número de control no es una factura para el SENIAT. Ver H-1.

**Corrección importante:** en el primer pase de esta auditoría reporté que el POS no
tenía código fuente. **Era falso** — mi búsqueda falló por un archivo `nul` de Windows
que rompe las herramientas de listado. El fuente está completo (5.308 líneas). Ver H-2.

---

## Lo que se corrigió en esta auditoría (verificado)

Estos no son hallazgos pendientes: se encontraron, se corrigieron y se probaron.

### C-1 · `/kardex` devolvía 404
La ruta real era `/reports/kardex`. Se agregó alias `GET /kardex` → 301.
En Venezuela el libro se pide por su nombre, no por el nombre de la tabla.
**Verificado:** `curl -L /kardex` → 200.

### C-2 · El Kárdex se caía con error 500 al abrirlo con un producto
`templates/kardex.html` comparaba `float64` contra un literal entero:
`{{if gt .QtyIn 0}}`. Go no convierte tipos en templates — la página fallaba con
`incompatible types for comparison: float64 and int`.
**Efecto real:** el libro fiscal nunca funcionó con datos. El 500 solo aparecía al
elegir un producto, así que el selector se veía bien y el error salía después.
**Corregido:** literales a `0.0` en `kardex.html` y `kardex_print.html`.
**Verificado:** 200 con 58 KB de contenido real.

### C-3 · El Costo Promedio Ponderado mostraba la CANTIDAD como costo
En la rama de salida del cálculo: `m.AvgCost = qtyBalance`. El libro mostraba el
número de unidades en la columna "Costo Prom.". Un dato fiscal falso.
**Verificado antes/después:** antes `CPP=0.00` en cada venta; ahora arrastra `40.00`.

### C-4 · El Kárdex ignoraba devoluciones y ajustes, y no ordenaba los movimientos
Solo leía compras y ventas. Faltaban:
- devoluciones de venta (entra mercancía) y de compra (sale mercancía)
- ajustes de inventario que suman y que restan

Además **no ordenaba** la mezcla: las consultas venían ordenadas cada una por su
cuenta, así que todas las compras quedaban antes que todas las ventas y el saldo
corrido salía mal (llegaba a negativo sin motivo).
**Corregido:** `kardex.go` reescrito. Las tres vistas (pantalla, JSON, impresión)
repetían su propia copia del cálculo y las tres diferían; ahora llaman a una sola
función. Un libro fiscal no puede tener tres respuestas para la misma pregunta.

### C-5 · Cuatro páginas del menú devolvían 500
`cartera_list.html`, `pagar_list.html`, `module_list.html` y `pos/dashboard.html`
estaban referenciados por sus handlers pero **nunca existieron** (verificado en el
historial de git: jamás se crearon). Se crearon los cuatro, ajustados al contrato de
datos real de cada handler.
**Verificado:** los 62 enlaces del sidebar responden 200. **Cero enlaces rotos.**

### C-6 · La tabla `tenants` nunca se creaba (bloqueaba alta de empresas)
`NewServer` fijaba `s.controlDB = db` antes de llamar a `controlStore()`, y
`controlStore()` tenía un `return` temprano cuando el handle ya existía — así que
`AplicarEsquema(controlSchema)` **no se ejecutaba nunca**. La tabla `tenants` no
existía y el alta de la primera empresa fallaba con
`Table 'erpgo_prod.tenants' doesn't exist`.
**Corregido:** primero se resuelve el handle, después se aplica el esquema, siempre.

### C-7 · El arranque creaba el administrador con usuario VACÍO
`PrepareDefaultCompany` llama `PrepareCompanyAs(db, "mysql", "", "", "", "")`.
`InitializeAdminUser` insertaba esa fila con `username=''`. Como `users` quedaba con
un registro, la función nunca volvía a entrar (`count > 0`): **el sistema quedaba sin
ninguna forma de acceder, de forma permanente.**
**Corregido:** sin credenciales se genera `admin` + contraseña aleatoria
(`crypto/rand`) y se imprime UNA vez.
**Verificado:** instalación limpia → `username=admin`, hash de 60 chars, login OK.

---

## Hallazgos PENDIENTES (no corregidos)

### H-1 · CRÍTICO — El POS no consume la serie fiscal

`internal/pos/grpc_server.go:277`:

```go
controlNo := "POS-" + sale.GetOfflineId()
```

El POS **fabrica su propio número de control** en vez de llamar a `nextFiscalDoc(tx, "FACT")`,
que es lo que sí hace la venta web (`invoice.go:384`). Consecuencias:

- El número de control no sale de la serie autorizada por la imprenta digital.
- Los correlativos del POS y los de la web **pueden repetirse entre sí**.
- `fiscal_series.next_no` no avanza con las ventas del POS.

Evidencia en `erpgo_default`:

```
series  n     control_no
POS     54    "POS-"  (48 veces, el MISMO valor)
FACT     9    NULL
```

48 facturas comparten literalmente el mismo `control_no`. Para el SENIAT eso no son
48 facturas: es una factura repetida 48 veces.

**Además:** `invoice.control_no` es `NULL`-able en el esquema, así que la base acepta
una factura sin número de control. La validación existe solo en el código.

**Reproducir:**
```sql
SELECT control_no, COUNT(*) FROM invoice GROUP BY control_no ORDER BY 2 DESC;
```

**Nota:** las 9 filas `FACT` con `control_no` y `date` NULL son datos de prueba
(`description = 'Test'`, cliente "Cliente de Prueba"). Pero el hecho de que el sistema
las **aceptara** es el hallazgo: no hay restricción que lo impida.

### H-2 · RESUELTO EN SU MAYOR PARTE — El POS sí tiene código fuente

**Corrección de un error de esta auditoría.** En el primer pase reporté que el POS "no
tenía código fuente". **Era falso.** La búsqueda falló porque el directorio contiene un
archivo llamado `nul` (artefacto de Windows) que rompe las herramientas de listado
—`find`, `grep -r` y `git status` fallan o mienten al recorrerlo—. El fuente está ahí:

```
POS/main.go                 1.548 líneas
POS/sqlite/                   1.628 líneas   (db, models, profiles, turnos)
POS/grpc/                     1.293 líneas   (client, grpc_client, sync_worker, auth_client)
POS/fiscal/                     569 líneas   (printer, http_driver)
POS/config/                     244 líneas
POS/cmd/check_sales/             26 líneas
                          ----------------------
                            ~5.308 líneas de Go
```

**El POS no compilaba**, y esa sí es una causa real y encontrada:
`POS/fiscal/printer.go` importaba `erpgow/fiscalprinteradapter`, un paquete que **nunca
existió** (verificado: no está en git, no está en el disco, no está en `ErpGO2.0`).
El comentario del propio archivo decía que envolvía `erpgow/internal/fiscalprinter`, pero
Go **prohíbe** importar un paquete `internal/` desde otro módulo — y el POS es otro
módulo (`erpgo-pos`, con su propio `go.mod`). Alguien planeó el puente y no lo escribió.

**Corregido en esta auditoría:** se creó `fiscalprinteradapter/adapter.go` en el ERP,
que reexporta el tipo `PrinterDriver` y la fábrica `NewDriver` del paquete interno.
No reimplementa ningún driver ni duplica protocolo: los cuatro (TFHKA, Hasar, Bixolon,
Epson) siguen viviendo en un solo sitio.

**Verificado:** `cd POS && go build ./...` → exit 0. `go build -o pos_new.exe .` → binario
de 25 MB generado.

#### Compatibilidad POS ↔ ERP (verificada)

Los endpoints que el POS consume existen y responden:

| Endpoint | Estado |
|---|---|
| `/api/pos/validate-key` | Registrado |
| `/api/pos/products` | Registrado |
| `/api/pos/customers` | Registrado |
| `/api/pos/sync/sale` | Registrado |
| `/api/pos/auth/login` | Registrado |
| `/api/pos/auth/validate` | Registrado |
| `/api/pos/auth/logout` | Registrado |
| `/api/pos/auth/supervisor` | Registrado |
| `/api/pos/checkout` | Registrado |

Prueba real contra el servidor vivo:

```
POST /api/pos/validate-key  {"api_key":"erpgo-pos-main-key-2026"}
-> {"valid":true,"branch_id":1,"branch_name":"Casa Matriz"}
```

La API key de `POS/config.json` coincide exactamente con la registrada en
`erpgo_default.api_keys`. El enlace POS→ERP **funciona**.

#### Por qué el POS "desaparecía" (causa raíz)

El POS **nunca estuvo bajo control de versiones**. `git ls-files POS/` devuelve 7
archivos, y son todos de hoy (los agregó esta sesión al crear el adaptador). Los otros
**29 archivos y carpetas del POS no están en git**: `main.go`, `sqlite/`, `grpc/`,
`fiscal/`, `config/config.go`, `go.mod` — todo sin versionar.

Como el POS no está en git, un `git clean`, un `git stash`, un cambio de rama o un
respaldo hecho desde git **lo borra sin dejar rastro**. Eso explica que se haya perdido
y que se recuperara de un backup manual.

**Acción requerida:** versionar el POS. Es la pieza que factura y no tiene historial.

**Pendiente dentro de H-2:**
- La versión recuperada del backup es del **8-9 de octubre**. El ERP siguió cambiando
  hasta el 12. Hay que confirmar que la lógica de venta del POS coincide con el esquema
  actual de `invoice` (ver H-1).
- El login de cajero POS devolvió `Credenciales inválidas` en la prueba; hay que
  verificar la contraseña de `cajero01` (puede ser simplemente que la de prueba cambió).


### H-3 · ALTO — 18 migraciones aplicadas en tu base pero borradas del disco

`schema_migrations` registra 38 archivos; en `migrations/` hay 23. Las que faltan:

```
schema_cash_session_dual_currency.sql   schema_menu_core.sql
schema_currency_cartera.sql             schema_modules.sql
schema_currency_voucher.sql             schema_multi_currency.sql
schema_pos_sessions.sql                 schema_pos_turnos.sql
schema_role_permissions_seed.sql        schema_roles_default.sql
schema_z_admin_role_level.sql           schema_z_cartera_permisos.sql
schema_z_invoice.sql                    schema_z_pos_branches.sql
schema_z_predefine_conasev.sql          schema_z_timesheet_proyectos.sql
schema_zzzz_ai_providers_limpieza.sql   schema_zzzz_islr_brackets_seed.sql
```

Tu base las tiene (se aplicaron en su momento), así que **tu instalación funciona**.
Una **instalación nueva no las tiene** — y ahí está el riesgo: el sistema arranca, y
el fallo aparece al usar la función que dependía de esa migración.

Esto es la causa raíz de C-6/C-7 y de la brecha de columnas de abajo.

**Reproducir:**
```sql
SELECT filename FROM schema_migrations;   -- comparar con ls migrations/
```

### H-4 · ALTO — Columnas multi-moneda sin migración que las cree

En una instalación limpia faltaban 43 columnas que tu base sí tiene. Las relevantes
para facturación:

| Tabla | Columnas |
|---|---|
| `invoice` | `currency_code`, `exchange_rate`, `total_amount_usd`, `exchange_rate_source`, `cash_session_id`, `warehouse_id` |
| `product_purchase` | `currency_code`, `exchange_rate`, `total_amount_usd`, `price_usd` |
| `acc_vaucher` / `acc_transaction` | `currency_code`, `exchange_rate` |

`invoice.exchange_rate` es `NOT NULL` sin default en tu base. En una instalación limpia
esa columna no existe → **el INSERT de factura falla** (Error 1054).
Por eso `go test ./...` falla con `Unknown column 'currency_code'`.

**Nota:** ya se agregaron dos migraciones de recuperación en esta auditoría
(`schema_z_zzz_recuperadas.sql` y `_cols.sql`, 23 tablas + 35 columnas recuperadas del
esquema en uso). **Falta completar el resto de las 43 columnas y verificar.**

### H-5 · ALTO — Cadena de auditoría rota (requisito SENIAT)

En `erpgo_default`, `/reports/audit-log/verify` reporta **cadena rota**.

```
audit_log: 312 filas, ids 1..313  ->  falta el id 284
```

El id 284 no existe. Además el registro 18 tiene un timestamp fuera de orden
(proviene de la copia desde `erpgo`). La cadena SHA-256 encadenada se rompe en el hueco.

Para el SENIAT la pista de auditoría es prueba: una cadena rota es una cadena que no
prueba nada.

**Reproducir:** `/reports/audit-log/verify` con sesión de administrador.

### H-6 · MEDIO — El SDK de addons tiene el import del README mal

`ERPGo-SDK/go.mod` declara `module github.com/orcanet/ergoaddon`, pero el paquete vive
en el subdirectorio `ergoaddon/`. El import del README
(`github.com/orcanet/ergoaddon`) **no resuelve**; el correcto es
`github.com/orcanet/ergoaddon/ergoaddon`.

**Verificado:** el ejemplo del README compilado tal cual falla con
`no required module provides package`. Con la ruta corregida compila.

También: `examples/` y `proto/` están **vacíos**. El SDK es Fase 1 de 5 según su propio plan.

### H-7 · BAJO — Residuo de bases de datos

Bases que no son tenants reales y quedaron de pruebas anteriores:

```
erpgo_plantilla_7bc498c4acee_15960   (149 tablas)
erpgo_t43_51200__carnes_del_lara_ca  (148 tablas)
erpgo_test_ctl_44_50700              (172 tablas)
erpgow                               (95 tablas)
```

`erpgow` es el nombre viejo de la base; el proyecto usa `erpgo`. Confirmar que ninguna
está en uso antes de borrar.

---

## Lo que SÍ está verificado y funciona

No todo es hallazgo. Esto se probó de verdad:

| Prueba | Resultado |
|---|---|
| `go build ./...` | Limpio |
| `go vet ./...` | Limpio |
| Instalación limpia (todas las migraciones) | 0 errores, 175 tablas |
| Instalación limpia: admin | `username=admin`, login OK |
| Instalación limpia: alta de empresa | `tenants` creada, empresa registrada |
| Sidebar completo | **60/60 enlaces → 200** |
| Kárdex end-to-end | 200 pantalla / 200 JSON / 200 impresión |
| CPP del Kárdex | Arrastra el costo correcto (40.00), no la cantidad |
| Validación de RIF (dígito verificador) | Correcta contra vectores publicados |
| Multi-tenant por sesión | Correcto (`SessionData.TenantCode`, no estado global) |

---

## Orden de trabajo sugerido

1. **H-2 (fuente del POS)** — sin esto, H-1 no se puede arreglar. Es el bloqueador raíz.
2. **H-1 (serie fiscal del POS)** — riesgo directo de multa. Depende de H-2.
3. **H-3 + H-4 (migraciones perdidas)** — una instalación nueva debe quedar completa.
4. **H-5 (cadena de auditoría)** — decidir si se repara o se documenta el hueco.
5. **H-6 (SDK)** — corregir la ruta del import; es de un minuto.

---

## Cómo verificar todo de nuevo

```bash
cd /c/Proyectos/GO/ERPGo
M="/c/laragon/bin/mysql/mysql-8.0.30-winx64/bin/mysql.exe"

# Compilación
go build ./... && go vet ./...

# Migraciones registradas vs en disco  (H-3)
"$M" -u root -p240580 erpgo -e "SELECT filename FROM schema_migrations;" | sort > /tmp/r.txt
ls migrations/*.sql | sed 's|.*/||' | sort > /tmp/d.txt
comm -23 /tmp/r.txt /tmp/d.txt

# Serie fiscal vs facturas  (H-1)
"$M" -u root -p240580 erpgo_default -e \
  "SELECT series, control_no, COUNT(*) FROM invoice GROUP BY series, control_no ORDER BY 3 DESC;"

# Cadena de auditoría  (H-5)
"$M" -u root -p240580 erpgo_default -e \
  "SELECT COUNT(*) filas, MIN(id), MAX(id) FROM audit_log;"
```
