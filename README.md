[README.md](https://github.com/user-attachments/files/33939877/README.md)
# ERPGo — ERP Fiscal para PYMEs Venezolanas

**Un ERP completo, fiscalmente defendible ante el SENIAT, gratis como binario.**
Distribuido por **Orcanet1724 C.A** como aporte al comercio venezolano.

> Solo ejecuta el binario y ya tienes: facturación POS, contabilidad con asientos
> automáticos, libros fiscales, caja por turnos, notas de crédito, IGTF,
> retenciones, multi-empresa, IA para reportes y un terminal POS offline-first
> que factura aunque el internet se caiga. Sin instalar bases de datos.

---

## Política de Orden y Crecimiento (NO NEGOCIABLE)

> **Esta política rige todo desarrollo futuro. Violarla rompe la promesa de estabilidad.**

### 1. Los datos existentes son sagrado

Los datos de los clientes **NO se tocan directamente**. Cualquier cambio de esquema o migración debe ser:

- **Idempotente** — se puede ejecutar N veces sin duplicar ni destruir
- **Checksum-verificado** — cada migración tiene SHA-256 que evita reejecutar
- **Preserva datos históricos** — una BD de 2025 debe funcionar en 2026 sin cambios
- **Sin DELETE/DROP** — nunca borra datos, solo agrega o actualiza con `ON DUPLICATE KEY UPDATE`

### 2. Migraciones: la única vía de cambio

Todo cambio de esquema pasa por `migrations/schema_X.sql` (y `assets/migrations/`):

- `CREATE TABLE IF NOT EXISTS` — tablas nuevas no rompen existentes
- `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` — columnas nuevas son optativas
- `INSERT ... ON DUPLICATE KEY UPDATE` — semilla idempotente
- Nunca `DROP`, `TRUNCATE`, `DELETE` en migraciones

### 3. Orden en el tiempo

El sistema debe mantener **orden cronológico**:

- Los IDs de menús, permisos y catálogos son fijos y no se reutilizan
- Los registros históricos (facturas, asientos, stock_movement) nunca se modifican
- Las nuevas funcionalidades son **capas adicionales** que referencian lo existente por ID
- Si algo queda obsoleto: se marca como inactivo, **nunca se elimina**

### 4. Crecimiento modular

ERPGo crece agregando módulos, no reescribiendo los existentes:

- **Módulos fijos** — integrados en el binario, visibles para todos
- **Addons** — extensiones por casa de facturación, funcionalidad regional, etc.
- La interfaz estándar `DigitalInvoiceProvider` define el contrato; cada casa implementa la suya
- Los datos de un módulo no invaden otro: `digital_invoice_log` referencia `invoice` por ID

### 5. Regla de oro

> **Si un cambio rompe una BD existente o requiere intervención manual del usuario, NO se hace.**

El cliente descarga el nuevo binario y listo. Las migraciones corren solas. Los datos históricos siguen ahí.

---

## Inicio rápido (3 pasos)

1. Ejecuta `erpgow.exe`
2. Abre `http://localhost:8080`
3. Entra con las credenciales configuradas en tu instalación

**Eso es todo.** El servidor arranca, detecta si hay MySQL en el puerto 3306,
si no lo encuentra arranca el MySQL incluido en el puerto 3307 (para no chocar
con Laragon/XAMPP), aplica las migraciones embebidas, siembra la empresa por
defecto y queda listo para vender.

---

## Seguridad

- Cambia las credenciales por defecto inmediatamente después de instalar
- Configura `SERVER_HOST=127.0.0.1` si solo vas a usar localmente
- Configura `SERVER_HOST=0.0.0.0` solo si necesitas acceso desde la red local
- Las API keys de POS se configuran en `config.json` del POS

---

## Requisitos

- Windows 10/11 (64 bits)
- MySQL 8.0+ (opcional, puede usar el incluido)
- 4 GB RAM mínimo

---

## Licencia

ERPGo es **freeware**: gratuito para uso, no open source.
El binario es gratuito y distribuido por Orcanet1724 C.A.
Sin embargo, el **código fuente es propietario** — no se redistribuye.

El SDK para crear addons (ERPGo-SDK) tiene su propia licencia (gratuita para usar,
no para revender).

---

## Addons

El sistema de addons permite a desarrolladores de terceros crear extensiones
que se integran con ERPGo. Los addons se instalan desde **Configuración > Addons**
en el panel de administración.

Para crear addons: usa el [ERPGo Addon SDK](../ERPGo-SDK/) (licencia separada).

### Administración de addons

La vista de administración de addons está implementada en:

| Componente | Ubicación |
|---|---|
| Vista HTML | `templates/addon_admin.html` |
| Handler de datos | `interfaces/web/addon_handlers.go` → `addonAdminData()` |
| Guardar permisos | `interfaces/web/addon_handlers.go` → `addonSavePermissions()` |
| Guardar capabilities | `interfaces/web/addon_capabilities.go` → `addonSaveCapabilities()` |
| Activar addon | `interfaces/web/addon_capabilities.go` → `addonActivate()` |

### Flujo de activación

1. El administrador coloca el paquete firmado en `addons/<code>/`
2. Instala el addon (`POST /module/addons/{code}/install`)
3. Aprueba o rechaza capabilities desde la pantalla de administración
4. Activa el addon (`POST /module/addons/{code}/activate`) cuando no quedan pendientes

### Rutas disponibles

| Método | Ruta | Función |
|---|---|---|
| `GET` | `/module/addons/admin` | Vista de administración |
| `GET` | `/module/addons/admin/data` | JSON con addons, capabilities y permisos |
| `POST` | `/module/addons/{code}/install` | Instalar addon |
| `POST` | `/module/addons/{code}/uninstall` | Desinstalar addon |
| `POST` | `/module/addons/{code}/activate` | Activar addon (requiere capabilities aprobadas) |
| `POST` | `/module/addons/{code}/capabilities` | Guardar decisiones de capabilities |
| `POST` | `/module/addons/{code}/permissions` | Guardar permisos por rol |

Los addons de terceros son propiedad de sus creadores, quienes eligen su propia
licencia (comercial o libre). ERPGo no reclama propiedad sobre los addons.
