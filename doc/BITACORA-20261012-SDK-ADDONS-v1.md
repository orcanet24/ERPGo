# Bitácora: SDK de Addons v1.0 — Sistema Completo

**Fecha:** 2026-10-12
**Autor:** asistente
**Alcance:** Sistema de addons/módulos para ERPGo — 100% funcional

---

## Resumen

Se implementó un sistema completo de addons/módulos para ERPGo que permite a desarrolladores de terceros crear extensiones seguras. El sistema maneja el ciclo de vida completo: descubrimiento, instalación con migraciones, actualización con detección de versión, y desinstalación limpia.

## Lo implementado

### Backend (`interfaces/web/addon_handlers.go`)

| Ruta | Método | Función |
|------|--------|---------|
| `/module/addons/discover` | GET | Escanea `addons/` y retorna manifiestos válidos |
| `/module/addons/list` | GET | Lista addons instalados con estado |
| `/module/addons/{code}` | GET | Información de un addon (path param) |
| `/module/addons/{code}/install` | POST | Instala addon: registro BD + migraciones + menús |
| `/module/addons/{code}/update` | POST | Actualiza: detecta cambio de versión, ejecuta migraciones |
| `/module/addons/{code}/uninstall` | POST | Desactiva menús + marca is_active=0 |
| `/mod/` | GET | Proxy inverso a la UI del addon |

### Tabla `addons` en BD (`erpgo`)

```
id VARCHAR(64) PK, name, version, description, author_name, author_email,
license_type manifest_json TEXT, addon_path, entrypoint, is_active TINYINT(1),
web TINYINT(1), grpc TINYINT(1), needs_migration TINYINT(1),
installed_at TIMESTAMP, created_at TIMESTAMP, updated_at TIMESTAMP
```

### Migraciones

- Los addons pueden declarar archivos SQL en `addons/{code}/migrations/`
- Al instalar: ejecuta todas las migraciones
- Al actualizar: compara versiones y ejecuta solo si cambió
- Formato: archivos `.sql` separados por `;`
- Idempotentes: usan `CREATE TABLE IF NOT EXISTS`, `INSERT IGNORE`

### Seguridad

- Los addons NUNCA acceden a MySQL directamente
- Headers inyectados: `X-ERPGo-User-ID`, `X-ERPGo-User-Name`, `X-ERPGo-User-Level`, `X-ERPGo-Tenant`
- Rutas `/mod/` validan que el addon está instalado y activo
- Manifiesto validado contra reglas estrictas (código, nombre, versión)

### SDK Público (`C:/Proyectos/GO/ERPGo-SDK/`)

```
ERPGo-SDK/
├── module.go              — Interfaz Module, Config, Serve()
├── go.mod                 — github.com/orcanet/ergoaddon
├── README.md              — Documentación para developers
├── examples/
│   └── hello/             — Addon de ejemplo completo
│       ├── module.json
│       ├── main.go
│       └── go.mod
└── docs/
    └── REGLAS-SEGURIDAD.md — Reglas OBLIGATORIAS
```

### Verificación end-to-end

```
✓ Sidebar raíz (11): Dashboard, Ventas, COMPRAS, Inventario, Cobranza, Cuentas por Pagar, Contabilidad, RECURSOS HUMANOS, Bancos, Reportes, CONFIGURACION
✓ Hello World en sidebar: True
✓ Addons list: funciona con JSON completo
✓ Addon info: responde con path param
✓ Discover: encuentra addons en addons/
✓ Proxy addon: sidebar=True, hello=True
✓ Uninstall: desactiva menús + is_active=0
✓ Reinstall: registra BD + menús
✓ Update: detecta cambio de versión, ejecuta migraciones
✓ Migraciones: tablas creadas al instalar
✓ go build ./...: OK
✓ go vet ./...: OK
```

## Reglas de seguridad para addons

1. El addon NUNCA recibe credenciales MySQL
2. Toda operación de escritura pasa por el orquestador
3. Los tokens HMAC expiran (24h por defecto)
4. Las rutas están aisladas (solo las declaradas en module.json)
5. Los addons no pueden acceder a datos de otras empresas
6. Los manifiestos se validan antes de instalar

## Archivos modificados

- `interfaces/web/addon_handlers.go` — Reescribo completo (530 líneas)
- `interfaces/web/module_handlers.go` — Agrego ruta update
- `interfaces/web/server.go` — Agrego `/mod/` a whitelist CSRF
- `interfaces/web/menu.go` — Limpieza de debugs
- `interfaces/web/render.go` — Fix `permissionMap` usa `controlStore()`
- `doc/PLAN-SDK-ADDONS.md` — Plan de 5 fases

## Proyecto SDK

- `C:/Proyectos/GO/ERPGo-SDK/` — Repositorio para publicar en GitHub
- `go.mod`: `github.com/orcanet/ergoaddon`
- `examples/hello/` — addon de ejemplo compilable
