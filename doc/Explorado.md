Estoy en modo Plan. He explorado el proyecto y ahora presento el análisis completo y el plan de documentación.

---

## Exploración del proyecto ERPGo

El proyecto `ERPGo` (carpeta `C:\Proyectos\GO\ERPGo`) es un **ERP fiscal en Go** para PYMES venezolanas, con arquitectura modular. Aquí está cómo está organizado:

### 🌳 Árbol de directorios (resumen)

| Directorio | Propósito |
|---|---|
| **`cmd/`** | Puntos de entrada de cada ejecutable: `server` (servidor principal), `diag`, `docgen`, `genhash`, `semillar` |
| **`internal/`** | Lógica de negocio y servicios: `orchestrator` (escrituras/atómicas), `pos` (servidor gRPC cajas), `service` (servicio Windows), `logger`, `queue` (cola con spill a disco), `security`, `setup` (wizard instalación), `upload`, `webcheck` |
| **`interfaces/web/`** | **La capa HTTP/API** del servidor (handlers, vistas, templates, rutas). Es donde está la mayor concentración de código (llamas de facturación, contabilidad, caja, crm, etc.) |
| **`infrastructure/persistence/`** | Capas de acceso a datos: `database.go`, `embedded_mysql.go` (MySQL embebido en 3007), `mysql.go`, `migration_ledger.go`, `walguard.go`, etc. |
| **`proto/pos/`** | Definición gRPC (Protocol Buffers) del servicio POS: `pos.proto`, `pos.pb.go`, `pos_grpc.pb.go` |
| **`configs/`** | Configuración de entorno (`config.go`, `.env`) |
| **`templates/`** | Plantillas HTML para el panel de administración (varias cientos de archivos) |
| **`static/`** | Assets frontend: CSS, JS, librerías (DataTables, Summernote, SweetAlert, etc.) |
| **`migrations/`** | Esquemas SQL de base de datos (miles de líneas, por módulo) |
| **`addons/`** | Sistema de complementos (un addon de ejemplo `hello/`) |
| **`instalador/`** | Instalador de Windows (Inno Setup), Docker, binarios |
| **`deploy/`** | Configuración de Caddy (proxy/nginx) |
| **`assets/`** | Assets embebidos en el binario (templates, migraciones, static) |
| **`docs/`** | Documentación técnica (seguridad, docker, usabilidad) |
| **`.github/workflows/ci.yml`** | CI/CD: tests cortos en PR, suite completa nocturna |
| **`.commandcode/taste/`** | Instrucciones de uso para el agente (artefacto de desarrollo) |
| **`data/`** | Datos runtime: empresas, escrituras, backups |
| **`bin/`, `tmp_review/`, `tmp_tools/`, `uploads/`, `backup/`** | Directorios de salida y datos |

### 🔗 Principales conexiones

```
cmd/server/main.go  ──▶ configs.Config  (carga .env)
                       │
                       ▼
              infrastructure/persistence  (MySQL embebido 3307)
                       │
                       ▼
        internal/setup/wizard.go  (asistente instalación)
                       │
                       ▼
        interfaces/web/server.go  (servidor HTTP 8080 + gRPC 9095)
                       │        │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   internal/pos        internal/     interface
   (grpc_server.go)   orchestrator    /api/* (HTTP)
                       queue          workers
        │
        ▼
   proto/pos/pos.proto  (gRPC service POS)
```

- **Entrada**: `cmd/server/main.go` → parsea flags (`--setup`, `--migrate`, `--install`) → `configs.EnsureEnv()` → inicia logger → arranca `web.Server` (HTTP) + `pos.GRPCServer` (gRPC) en paralelo.
- **BD**: MySQL embebido (`embedded_mysql.go`) en puerto 3307 o MySQL externo en 3306. Las **empresas** son bases de datos separadas (`erpgo_<codigo>`) bajo una BD de control `erpgo`.
- **Escrituras**: `internal/orchestrator/orchestrator.go` es el **úni­co escritor**: serialize las escrituras en un journal, aplica en orden con correlativos, idempotencia y rastro en `audit_log`. La cola de `internal/queue` + spill a disco protege contra caídas.
- **POS**: `interfaces/web/` expone `POST /api/pos/*` y el `grpc_server.go` escucha en 9095. El `proto/pos/pos.proto` define el contrato gRPC bidireccional (Connect/Stream + operaciones individuales).
- **Addons**: `addons/` se cargan dinámicamente via `ERPGo-SDK` (reemplazado por `../ERPGo-SDK` en `go.mod`). Los handlers se registran en `addon_handlers.go` y `addon_admin.go` de `interfaces/web`.
- **IA**: `interfaces/web/ai_*.go` + `internal/pos/ai.go` integran IA para reportes, asistentes de tareas, etc.
- **Seguridad**: `internal/security` (cifrado), `internal/logger` (logs), `interfaces/web/authMiddleware` (sesiones JWT + CSRF), `interfaces/web/permissions.go` (roles/permisos), `interfaces/web/audit_chain.go` (cadena de auditoría).
- **SQLite**: está disponible (librería `glebarez/go-sqlite`) pero el recomendado es MySQL. El `cmd/server/main.go` rechaza arrancar con SQLite sobrante.

---

## Plan de documentación

Voy a generar **tres entregables** en `C:\Proyectos\GO\ERPGo\doc\`:

### 1. `indice.html` — Índice HTML interactivo
- Árbol de directorios completo (todas las carpetas y vistas de vista de archivos important)
- Secciones por módulo: `cmd/`, `internal/`, `interfaces/web/`, `proto/`, `migrations/`, `infrastructure/`, `instalador/`, `deploy/`, `tools/`, etc.
- Para cada carpeta, lista sus archivos principales (no todos los `templates/*.html` y `static/**` que son demasiado prolixos)
- Enlaces con `#` para navegar por secciones
- Tabla de dependencias/solapamientos entre módulos
- Notas de arquitectura (grupo de comunicación, puertos, bases de datos)

### 2. `graph.mmd` — Gráfico en Mermaid
- Grafo dirigido (flowchart) que muestra:
  - **Fuentes**: `cmd/server/main.go` (entrada)
  - **Nodos intermedios**: `configs`, `infrastructure/persistence`, `internal/*.go` (orchestrator, pos, queue, setup, logger, service)
  - **Salidas**: `interfaces/web` (HTTP + API), `proto/pos` (gRPC)
  - **Agregados**: BD (MySQL embed 3307 / externo 3306), Caja POS (9095), Addons, IA, Servicios Windows
  - Títulos, flechas con etiquetas de protocolo/dependencia
  - Legenda de colores por capa (infraestructura, lógica, presentación, datos)

### 3. `estructura.txt` — Resumen en texto plano
- Acentuación de la arquitectura entera
- Tabla de módulos y responsabilidades
- Resumen de puertos, bases de datos, archivos clave

---

### ¿Cómo procedo?

Para generar estos archivos (que son **cambios de estado** en el repositorio) necesito estar en **Modo Act**. Dame permiso para:
1. Crear `doc/indice.html` (el documento principal)
2. Crear `doc/graph.mmd` (el gráfico)
3. Crear `doc/estructura.txt` (el resumen en texto)
4. Validar que se generaron correctamente

Cuando estés listo, dímelo para activar el modo Act. ¿Qué prefieres revisar primero del árbol de directorios?