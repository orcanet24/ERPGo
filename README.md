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

Todo cambio de esquema pasa por `migrations/schema_X.sql`:

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

1. Ten MySQL o MariaDB instalado y arrancado como servicio del servidor
2. Ejecuta `erpgow.exe --setup` (o copia el binario con su `.env` ya configurado)
3. Abre `http://localhost:8080`

**El motor de base de datos NO viene con ERPGo.** El servidor debe tener su
propio MySQL o MariaDB instalado y corriendo antes de arrancar el ERP: el binario
no instala, no inicializa ni arranca ningún motor, solo se conecta al que existe
según `DB_HOST` y `DB_PORT` (3306 por defecto). Si no encuentra el servidor, se
detiene con un mensaje claro y no toca nada del sistema.

Una vez conectado, aplica las migraciones embebidas, siembra la empresa por
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
- MySQL 8.0+ o MariaDB 10.6+ instalado y arrancado como servicio (obligatorio; no se incluye)
- 4 GB RAM mínimo

---

## Licencia

ERPGo es **freeware**: el binario es gratuito para uso, no open source.
Distribuido por Orcanet1724 C.A.
El **código fuente es propietario** — no se redistribuye.

El SDK para crear addons (ERPGo-SDK) tiene su propia licencia (gratuita para usar,
no para revender).

## Modelo de Soporte

Cada cliente es un cliente. No hay foros de comunidad, no es open source.

Orcanet1724 C.A. ofrece soporte por niveles según el tipo de cuenta:

| Nivel | Binario ERPGo | Soporte |
|---|---|---|
| **Gratuito** | ✅ Gratis | 50 tickets/mes vía sistema de tickets |
| **De pago** | ✅ Gratis | Tickets ilimitados + soporte remoto |

Adicionalmente, Orcanet1724 C.A. ofrece por separado:

- **Hosting** — alojamiento del ERP en servidor administrado
- **Servicios de IA** — proveedores de inferencia, agentes, configuración personalizada

> El ERP siempre es gratis. Lo que se contrata es el soporte, el hosting y los
> servicios profesionales alrededor de la herramienta.

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
