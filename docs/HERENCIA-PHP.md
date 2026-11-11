# Herencia del PHP original (SalesERP)

Fecha: 2026-11-11
Estado: cerrada formalmente. El PHP ya no se ejecuta ni se mantiene; este
documento dice qué quedó, qué se retiró y qué regla rige de aquí en adelante.

## Contexto

El ERP nació como SalesERP v12.13 (PHP, ~2013). El ERPGo es su reescritura
completa en Go: no queda ni un archivo `.php` en el árbol de trabajo ni un
runtime PHP. La herencia que sí quedó es de datos y de costumbre, no de código.

## Qué quedó, a propósito

| Herencia | Dónde | Por qué se conserva |
|---|---|---|
| Esquema de tablas y columnas | `migrations/schema.sql` (volcado del PHP) y las 62 migraciones | Son los datos de los clientes. Renombrar tablas o columnas es riesgo sin valor fiscal. |
| Motor contable de partida doble | `interfaces/web/accounting.go` | Replica el `acc_vaucher` → `acc_transaction` del PHP; es el comportamiento contable que los clientes ya conocen. |
| Contraseñas de usuarios migrados | `interfaces/web/md5compat.go` | `md5hex` replica el `md5()` de PHP para que los usuarios migrados sigan entrando. Los usuarios nuevos usan bcrypt. |
| Nomenclatura de permisos | `module`/`sub_module` en `schema_menu_permissions.sql` (`new_invoice`, `hrm_management`, ...) | Los permisos existentes apuntan a esos nombres; cambiarlos rompería los roles ya creados. |
| Assets de la era PHP | `static/` (444 archivos: AdminLTE, flot, OwlCarousel, summernote, toastr) | Servidos verbatim; funcionan. Modernizarlos es un proyecto de interfaz, no de herencia. |

## Qué se retiró, a propósito

- **POS web y `/synchronizer`**: documentado en
  `migrations/schema_z_zzzzz_menu_limpieza.sql` ("el POS es OTRO PROYECTO");
  no hay handler registrado para `/synchronizer`.
- **Menú "Addons"**: retirado en `migrations/schema_z_zzzzzz_retiro_addons.sql`.
- **Referencia al árbol PHP**: la carpeta `php_original` que el docgen marcaba
  como excluida ya no existe y la marca se retiró de `cmd/docgen/scan.go`.

## Regla de aquí en adelante

**Nada nuevo hereda nombres ni estructuras del PHP.** Las tablas nuevas se
crean en el esquema canónico (`schema/esquema_canonico_mysql.sql`) con nombres
en español o con el nombre de dominio que corresponda; si una tabla nueva se
parece a una del PHP, se documenta la diferencia. El esquema viejo se conserva
por los datos, no como modelo a imitar.

## El esquema hoy

El esquema declarativo vive en `schema/esquema_canonico_mysql.sql` y se
reconcilia al arranque (`infrastructure/persistence/canonical.go`). Las 62
migraciones de `migrations/` siguen ahí como red de seguridad de las bases
instaladas sin sellar; las bases selladas (`esquema_canonico_v1` en el ledger)
ya no las ejecutan. Detalle en `docs/PLAN-ESQUEMA-CANONICO.md`.
