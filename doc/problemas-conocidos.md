# Problemas Conocidos — ERPGo UI

**Fecha:** 27 de septiembre de 2026

## Problema principal: Templates no replican el PHP original

**Síntoma:** El diseño visual (CSS, layout, sidebar, header) no se ve igual al sistema PHP original. Los links rotos, menú simplificado, sin estilos correctos.

**Causa:** Los templates HTML son versiones simplificadas (~340 líneas) que no replican fielmente el layout del PHP (1461 líneas solo de sidebar + header completo + modales).

## Solución en progreso

Se está trabajando en reescribir completamente los templates para replicar fielmente el PHP:

| Archivo | PHP original | Go actual | Estado |
|---------|-------------|-----------|--------|
| layout.php | 213 líneas | layout.html 340 líneas (simplificado) | 🔄 En proceso |
| includes/head.php | 63 líneas (CSS/JS) | head en layout.html | 🔄 En proceso |
| includes/header.php | 138 líneas (navbar) | header en layout.html | 🔄 En proceso |
| includes/sidebar.php | 1461 líneas (menú completo) | sidebar en layout.html (simplificado) | 🔄 En proceso |
| includes/footer.php | modales + CSRF | sin footer separado | 🔄 En proceso |

## Rutas PHP → Go (diferencias)

El PHP usa CodeIgniter con rutas como `/add_invoice`, `/customer_list`, etc.
Go usa rutas como `/invoice/new`, `/customer/list`, etc.

**Impacto:** Los links en el sidebar necesitan mapearse a las rutas Go.

## Assets estáticos

- CSS: `/static/css/` ✅ idéntico al PHP
- JS: `/static/js/` ✅ idéntico al PHP  
- Plugins: `/static/plugins/` ✅ idéntico al PHP
- Imágenes: `/static/images/` ✅ idéntico al PHP

Los assets están correctos. El problema es solo el HTML de los templates.

## Próximos pasos

1. Replicar fielmente `layout.php` → `templates/layout.html`
2. Replicar fielmente `includes/head.php` → `templates/includes/head.html`
3. Replicar fielmente `includes/header.php` → `templates/includes/header.html`
4. Replicar fielmente `includes/sidebar.php` → `templates/includes/sidebar.html` (COMPLETO, todos los módulos)
5. Replicar fielmente `includes/footer.php` → `templates/includes/footer.html` (con modales)

## Temporal

Mientras se resuelve, el sistema es **funcionalmente completo**:
- Build compila sin errores ✅
- 28 archivos Go operativos ✅
- 81 templates funcionales (aunque simplificados) ✅
- Todos los módulos backend operativos ✅
- Base de datos migrada ✅

---

## RESUELTO (27/09/2026): páginas que se ven "sin CSS"

**Síntoma:** las vistas se renderizan pero sin estilos (HTML crudo, links azules).

**Causa raíz (doble):**

1. **Registro de Windows envenenado:** `HKCR\.css\Content Type = application/x-css` (y `.js` → `application/x-js`). Go usa el registro para asignar MIME; con la cabecera `X-Content-Type-Options: nosniff` que añade el servidor, **Chrome rechaza aplicar el CSS** ("Refused to apply style...").
   - *Fix:* tabla MIME fija en `interfaces/web/server.go` (`mimeTypesByExt` + `forceContentType`) que ignora el registro.
2. **Caché del navegador:** las respuestas malas quedan cacheadas de forma heurística (Last-Modified antiguo → semanas de vigencia); un F5 normal no las refresca.
   - *Fix:* `Cache-Control: no-cache` en `/static/` y `/uploads/` (siempre revalida → 304).
   - *Fix operativo:* recarga con **Ctrl+F5** la primera vez tras actualizar.

**Reglas para evitar regresión:**
- `iniciar.bat` y `instalar.bat` **compilan siempre** y borran binarios obsoletos (`erpgow.exe~`, `erpgow-test.exe`) — un exe viejo = CSS rotos.
- Si añades nuevas extensiones a `static/`, añade su MIME en `mimeTypesByExt`.

## RESUELTO (27/09/2026): `/static/data/` era público

`/static/` no requiere sesión. La carpeta residual del PHP contenía `outgoing/backup.sql`
(dump completo de MySQL, 1.2 MB), PDFs de facturas y CSVs — descargables por cualquiera.
*Fix:* el handler de `/static/` devuelve **404** para `/static/data/` y para archivos `.php`
(las vistas PHP residuales). La app Go escribe respaldos en `data/backup` (raíz) y los sirve
vía `/data/backup/{name}` autenticado.
## RESUELTO (27/09/2026): menú incompleto — 7 secciones desaparecidas

**Síntoma:** el sidebar mostraba solo 10 secciones; faltaban **Servicios, Devoluciones,
Cotizaciones, Bancos, Ajuste Inventario, Libros Legales VE y Pista de Auditoría**.

**Causa:** esas secciones se pintan con `{{if index .Perm "<clave>"}}` y las 11 claves de
permiso (`create_service`, `manage_service`, `create_quotation`, `create_return`,
`manage_bank_transaction`, `manage_adjustment`, `manage_audit_log`, ...) **no existían** en
la tabla `sub_module` (solo había 28 filas). Al no estar, `index .Perm` devolvía falso y el
bloque completo se omitía.

**Fix 1 (migración):** `migrations/schema_menu_permissions.sql` — inserta los `module`/`sub_module`
faltantes con `INSERT IGNORE` (idempotente). Sus ids 29–40 son **idénticos** a los de la lista
`subModules` de `InitializeAdminUser` (`infrastructure/persistence/admin.go`), para que los dos
sembradores puedan correr en cualquier orden sin duplicados ni huecos.

**Fix 2 (seeder):** `InitializeAdminUser` ahora usa `INSERT IGNORE` para `module` y `sub_module`.
Antes usaba `INSERT` normal con ids explícitos: como `--install` ejecuta las migraciones **antes**
del seeder, en una **instalación limpia** chocaba con clave duplicada, hacía rollback de la
transacción y **dejaba el sistema sin usuario admin** (el error solo se registraba con `log.Printf`).
También se alinearon los `mid` del seeder con los módulos reales (p. ej. `setting`→14,
`create_service`→13, `bank`→10); antes apuntaban a módulos equivocados (aunque `mid` no se lee en
ninguna consulta Go: los permisos se resuelven por `sub_module.directory`).

**Resultado verificado en `/home`:** **17 secciones** y **63/63 links del menú responden 200/302**.

## RESUELTO (27/09/2026): migraciones con CRLF no se aplicaban (Windows)

**Síntoma:** `erpgow.exe --migrate` decía "0 aplicadas" (o "applied" sin efecto) para un
archivo `.sql` recién creado; el mismo `INSERT` ejecutado a mano sí funcionaba.

**Causa raíz:** `splitSQL` en `infrastructure/persistence/database.go` divide el archivo por
`";\n"`. Un `.sql` guardado en Windows tiene finales **CRLF** (`";\r\n"`), así que **nunca se
dividía**: todo el archivo se enviaba como **una sola sentencia** (multi-statement deshabilitado
en el DSN) y MySQL la rechazaba; al ser `INSERT IGNORE` idempotente el contador no lo reflejaba.

**Fix:** en `RunMigrations` se normaliza el contenido antes de parsearlo:

```go
sqlText := strings.ReplaceAll(string(content), "\r\n", "\n")
```

**Verificación E2E:** se borraron las 11 filas (`id 29–39`) y `--migrate` las restauró
(246 statements aplicados, 0 warnings); el menú volvió a mostrar las 17 secciones.

**Regla:** cualquier `.sql` nuevo en `migrations/` funciona con LF o CRLF (no hace falta
convertir finales de línea).

## RESUELTO (27/09/2026): `/service/invoice/new` devolvía HTTP 500

**Síntoma:** al abrir "Nueva Factura de Servicio" la página fallaba con
`template service_invoice_form.html:91: function "charge" not defined`.

**Causa:** en el `<script>` el rango usaba la acción desnuda `{{charge}}` (se interpreta como
llamada a función) en vez del campo del mapa `{{.charge}}`. Al ser un **error de parseo**, la
plantilla no se podía compilar y toda la página devolvía 500.

**Fix:** `templates/service_invoice_form.html`, línea 91:

```js
{{range .Services}}{ id: '{{.id}}', name: '{{.name}}', charge: '{{.charge}}' },{{end}}
```

Se dejó el valor entre comillas para que un `charge` vacío/NULL no rompa el JS.

**Verificación E2E:** se creó un servicio de prueba por la UI (`POST /service/save`) y la
página renderizó `{ id: '1', name: 'Servicio Prueba E2E', charge: '25.50' },`; el dato de
prueba se eliminó después (`POST /service/delete/1`).

**Regla:** en las plantillas, las claves de los mapas de datos van **con punto**
(`{{.charge}}`); sin punto solo valen funciones registradas en `templateFuncs`
(`add`, `sub`, `money`, `divf`, `query`, `today`).

