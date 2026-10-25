# BITACORA-20261016-ERPGo-Usabilidad

## Avances
- **Errores de permisos resueltos:** Ahora /settings/company devuelve 404 para no-admins (no revela existencia)
- **Menú "Nueva empresa" eliminado:** Ya no aparece en el desplegable de perfil
- **Mensajes de error elegantes:** "No tiene permisos para este módulo", "No tiene acceso a esta empresa"
- **Redirección a login:** Sin sesión → /login en vez de JSON
- **sec_role y sec_userrole corregidas:** Ahora con AUTO_INCREMENT y columna `level`
- **Wizard inserta roles:** 4 roles base + asignación master al super admin
- **Menciones a SQLite eliminadas:** Ya no aparece en /settings
- **Módulo synchronizer eliminado:** sync.go y template eliminados
- **Referencias fiscales eliminadas de ERPGo:** fiscal_printer.go, fiscal_agent.go eliminados; internal/fiscalprinter movido a POSGo

## Decisiones
- **Cada empresa tiene su directorio aleatorio:** EMP{8hex} para evitar colisiones y exposición
- **Subdirectorios por rama:** logo/, productos/, personal/, documentos/
- **personal/ protegido:** Solo master/admin pueden acceder; 403 para otros
- **Archivos con nombre aleatorio:** 32 hex chars + extensión
- **No compilar por ahora:** Modo descubrimiento de usabilidad

## Problemas descubiertos
- **Dependencias circulares POSGo↔ERPGo:** POSGo importa ERPGo; si fiscalprinter está en POSGo, ERPGo no puede importarlo sin ciclo
- **sec_userrole SIN AUTO_INCREMENT:** El wizard inserta con `VALUES (1, ...)` porque la tabla no tiene auto_increment
- **Drivers fiscales compartidos:** Terminal POS los necesita, pero ERPGo también los usaba para el agente fiscal web

## Correcciones aplicadas
- **menu, ai_agent, ai_providers, cash_session, cost_timesheet:** Columnas faltantes agregadas a CREATE TABLE principales
- **Migraciones nuevas instalaciones:** Ya no requerirán ALTER TABLE posteriores para estas columnas
- **assets/ sincronizado:** Todos los cambios replicados en assets/migrations/

## Pendientes (verificación post-compilación)
- [ ] **menu table:** INSERTs con `module_id` e `is_core` que no existen en tabla
- [ ] **ai_agent table:** INSERTs con `department`, `config_prompt`, `config_prompt_at` que no existen
- [ ] **ai_providers table:** INSERT con `vendor` que no existe
- [ ] **cash_session table:** INSERTs con `opening_amount_usd` que no existe
- [ ] **cost_timesheet table:** INSERTs con `task_id`, `change_order_amount`, `nonbillable_reason` que no existen
- [ ] **Subida de fotos de personal:** Agregar en perfil de empleado y HR
- [ ] **Foto de usuario en menú:** /html/body/div[3]/aside/div/div[1]/div[1] (actualmente vacío)
- [ ] **Logo de empresa en /settings:** Subida y visualización
- [ ] **Imágenes de productos:** Integrar upload.Manager
- [ ] **Integrar upload.Manager:** En setting.go, product.go, HR handlers
- [ ] **Compilar:** Cuando estén todos los puntos claros

## Archivos modificados hoy
- `interfaces/web/permissions.go` - Errores elegantes, redirección login
- `interfaces/web/server.go` - Eliminadas rutas fiscales, upload.Manager integrado
- `interfaces/web/render.go` - Función notFoundOrJSON
- `interfaces/web/audit_chain.go` - Eliminado "sincronizer"
- `interfaces/web/settings.html` - Eliminado SQLite
- `interfaces/web/layout.html*` - Eliminado "Nueva empresa"
- `internal/setup/wizard.go` - Roles base, sec_role, sec_userrole
- `migrations/schema.sql` - Columna upload_path en company_information, estructura sec_role/sec_userrole corregida
- `internal/upload/manager.go` - Nuevo manager de uploads

## Estado: 🔄 EN PROGRESO (descubrimiento de usabilidad)
- Binarios compilados y funcionales
- Queda pendiente integrar el upload.Manager en los handlers existentes
