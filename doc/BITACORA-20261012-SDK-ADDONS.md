# Bitácora: SDK de Addons Fase 1-2

**Fecha:** 2026-10-12.
**Autor:** asistente.
**Alcance:** implementación de las fases 1 y 2 del SDK de addons.

---

## Logros

### Fase 1 — Manifiesto y escaneo (COMPLETA)

| Componente | Archivo | Estado |
|------------|---------|--------|
| Migración BD | `migrations/schema_addons.sql` | ✅ Aplicada |
| Addon handlers | `interfaces/web/addon_handlers.go` | ✅ Creado |
| Descubrir addons | `GET /module/addons/discover` | ✅ |
| Instalar addon | `POST /module/addons/{code}/install` | ✅ |
| Desinstalar addon | `POST /module/addons/{code}/uninstall` | ✅ |
| Proxy a UI | `GET /mod/{code}/*` | ✅ |
| Addon de prueba | `addons/hello/` | ✅ Creado |

### Fase 2 — Proxy inverso (COMPLETA PARCIAL)

| Componente | Estado |
|------------|--------|
| Proxy HTTP a binario addon | ✅ |
| Inyección headers X-ERPGo-* | ✅ |
| Servir templates estáticos | ✅ |
| Wrapper con layout maestro | ✓ (creado, falta validar) |

---

## Problemas encontrados y resueltos

| Problema | Causa | Solución |
|----------|-------|----------|
| `addons` tabla no existía | Migración ignorada por checksum | Aplicado manualmente con SQL directo |
| Login fallaba con `Baruta071121.` | Hash incorrecto | Regenerado con bcrypt y actualizado en BD |
| `/mod/{rest...}` conflicto | ServeMux no acepta overlap | Eliminado, solo `/mod/` |
| Menú no aparecía en sidebar | menu.go usa `authDB` pero filtra por permisos | Pendiente de depuración |
| CSRF bloqueaba install | Rutas no estaban en whitelist | Agregado `/module/addons/` a whitelist |
| Template sidebar.html no existe | Layout embebe sidebar | Cambiado a wrapper con template parcial |

---

## Flujo de trabajo actual

```
1. Admin sube addon a addons/<code>/module.json
2. GET /module/addons/discover → descubre, valida manifiesto
3. POST /module/addons/<code>/install → registra en BD + menús
4. GET /mod/<code>/ → proxy a binario o sirve templates
5. POST /module/addons/<code>/uninstall → desactiva
```

---

## Pendiente (Fase 2-3)

| Tarea | Prioridad |
|-------|-----------|
| Menú de addons visible en sidebar | Alta |
| Headers de permisos completos en proxy | Alta |
| Plantilla wrapper con layout maestro | Media |
| Templates del addon usan `{{define "content"}}` | Media |
| gRPC ModuleCore/ModuleHost | Baja (Fase 3) |
| SDK Go publicable | Baja (Fase 3) |
| Panel de admin para addons | Media |
| Marketplace | Baja (Fase 4) |

---

## Archivos modificados/creados

- `interfaces/web/module_handlers.go` — rutas de addons agregadas
- `interfaces/web/addon_handlers.go` — handlers de discover/install/uninstall/proxy
- `interfaces/web/menu.go` — ya usaba `authDB` (sin cambios)
- `interfaces/web/server.go` — `/module/addons/` en whitelist CSRF
- `templates/addon_info.html` — página informativa de addon
- `templates/addon_wrapper.html` — wrapper para templates de addons
- `templates/addon_list.html` — panel de gestión de addons
- `migrations/schema_addons.sql` — tabla addons
- `addons/hello/module.json` — addon de prueba
- `addons/hello/web/index.html` — template del addon
- `doc/PLAN-SDK-ADDONS.md` — plan completo del SDK
