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
## Módulo OCR Addon

### Descripción General

El módulo OCR permite procesar facturas entrantes utilizando el **binario** de
Tesseract vía `exec` (sin bindings CGO: el ERP es un binario único que debe
compilar en Windows y Linux sin dependencias nativas; si falta el binario en
el servidor, el resultado es un error controlado, nunca un bloqueo del ERP).
Se activa con las variables de entorno `ENABLE_OCR=0|1`, `TESSERACT_BIN` y
`OCR_LANG` (default `spa`).

### Estado Actual
- **Modo por defecto**: `ENABLE_OCR=0` (desactivado) - El sistema funciona exactamente como antes sin OCR
- **Para activar**: `ENABLE_OCR=1` en `.env` (o en `settings/fiscal`); en Windows sin Tesseract en el PATH, asignar `TESSERACT_BIN` con la ruta del `tesseract.exe`
- **Motor OCR**: binario `tesseract` ejecutado por `exec` con TSV de salida (nivel 5) y soporte español `OCR_LANG=spa`

### Endpoints Disponibles

| Endpoint | Método | Descripción |
|----------|--------|-------------|
| `/ocr/process` | POST | Procesa imagen de factura subida (multipart/form-data con field `invoice_image`). Detecta automáticamente RIF, total y fecha. Requiere `ENABLE_OCR=1`. |
| `/ocr/status` | GET | Retorna el estado actual de OCR: `{"enabled": true/false, "message": "activado"|"desactivado"}`. |
| `/ocr/test` | GET | Retorna una muestra de resultado OCR con datos simulados (útil para desarrollo). |

### Estructura de Datos OCR

```go
type OCRResult struct {
    Text          string  `json:"text"`           // Todo el texto extraído
    Confidence    float64 `json:"confidence"`     // Nivel de confianza Tesseract (0-100)
    RIFFound      string  `json:"rif_found"`      // RIF detectado (ej: J-12345678-9)
    TotalFound    string  `json:"total_found"`    // Total/grand total detectado
    DateFound     string  `json:"date_found"`     // Fecha detectada (dd/mm/yyyy)
    InvoiceNo     string  `json:"invoice_no"`     // Número de factura/Comprobante
    SupplierName  string  `json:"supplier_name"`  // Nombre proveedor
    Status        string  `json:"status"`         // "ok", "low_confidence", "error", "disabled"
    RawFormat     string  `json:"raw_format"`     // "xml", "pdf", "image"
```

### Flujo de Trabajo

#### 1. OCR Desactivado (Recomendado por defecto)
```
Usuario sube factura → POST /ocr/process → Retorna: {"status": "disabled", ...}
Sistema continúa normal, usuario ingresa datos manualmente.
```

#### 2. OCR Activado (Superusuario)
```
Usuario sube factura escaneada → Gosseract extrae texto → 
Detecta: RIF, Total, Fecha → Retorna OCRResult → 
IA propone datos → Validador fiscal permite create_report → 
Usuario confirma → Sistema guarda factura con datos validados
```

#### 3. Baja Confianza (confidence < 30%)
```
OCR result → status: "low_confidence" → Usuario revisa manualmente
```

### Requisitos de Instalación

1. **Go Dependency**:
   ```bash
   go get github.com/otiai10/gosseract/v2
   go mod tidy
   ```

2. **Tesseract OCR Engine** (en el servidor Linux):
   ```bash
   # Debian/Ubuntu
   apt-get update && apt-get install -y tesseract-ocr tesseract-ocr-spa
   
   # O otros gestores de paquetes según SO
   ```

3. **Configuración de Entorno**:
   Agregar a `.env` o `settings/fiscal`:
   ```
   ENABLE_OCR=0      # Default: desactivado (seguro)
   # o
   ENABLE_OCR=1      # Activado: módulo OCR disponible
   ```

### Seguridad y Validador Fiscal

El validador fiscal implementado protege contra 18 operaciones fiscales prohibidas:

**Operaciones Bloqueadas:**
- ✅ Guardar factura con datos OCR automáticamente
- ✅ Modificar total después de OCR
- ✅ Borrar factura fiscal después de OCR
- ✅ Modificar tasas IGTF/IVA
- ✅ Ocultar facturas fiscales
- ✅ Suprimir comprobantes fiscales

**Operaciones Permitidas:**
- ✅ `create_report` con resultados OCR (solo reporte/propuesta, no modificación)
- ✅ Analizar OCR y proponer datos al usuario
- ✅ Mostrar resultados OCR para que el usuario decida

**Prompt Seguro para IA:**
```
"Analizar resultado OCR de factura y proponer datos: 
RIF=J-12345678-9, total=150.00, fecha=15/01/2025"
```

### Arquitectura del Sistema

```
┌─────────────────────────────────────┐
│          Flujo Completo OCR          │
├─────────────────────────────────────┤
│ 1. Usuario sube imagen               │
│ 2. POST /ocr/process                 │
│ 3. Verificar ENABLE_OCR=1           │
│ 4. Binario tesseract (exec) → texto y TSV      │
│ 5. extractRIF/extractTotal/extractDate│
│ 6. OCRResult con status              │
│ 7. IA recibe resultado + prompt      │
│ 8. Validador fiscal → bloquea/permite│
│ 9. Usuario confirma o corrige        │
│ 10. Guardar factura validada         │
└─────────────────────────────────────┘
```

### Archivos Modificados/Creados

- ✅ `interfaces/web/ocr_addon.go` (NUEVO) - Motor OCR completo
- ✅ `interfaces/web/server.go` (MODIFICADO) - Línea 396: `s.registerOCCRoutes()`
- ✅ Documentación en este README

### Próximos Pasos Recomendados

1. Instalar Tesseract: `apt-get install tesseract-ocr tesseract-ocr-spa` (Linux) o descargar el instalador UCSF/SUBBE de UB Mannheim (Windows) y definir `TESSERACT_BIN`
2. Configurar `ENABLE_OCR=1` en `.env`
3. Probar endpoints: `/ocr/process`, `/ocr/status`, `/ocr/test`
4. Diseñar interfaz de configuración en panel de administración
5. Integrar con flujo de compras existente

## Integración con YummyProxy2 (envíos)

ERPGo se conecta a la pasarela de Yummy (`orcanet24/YummyProxy2`), un servicio
EXTERNO con su propio binario/Docker que guarda la API key real de Yummy y
ofrece un API simple por clave `X-Gateway-Key`. El ERP es CLIENTE de la
pasarela: la clave `mk_live_…` de Yummy jamás vive en el ERP.

### Variables de entorno (todas en `.env` del ERP)

| Variable | Descripción |
|----------|-------------|
| `YUMMY_ENABLED` | `1` activa el módulo (default `0`: la operación diaria no depende de esto) |
| `YUMMY_GATEWAY_URL` | base de la pasarela, ej. `http://gw.tudominio.com` |
| `YUMMY_GATEWAY_KEY` | la clave `gw_live_…` del tenant del ERP (la emite el admin del gateway) |
| `YUMMY_CALLBACK_SECRET` | el `callback_secret` del alta del tenant: con él el gateway firma las entregas de webhooks al ERP |

### Endpoints del ERP

| Endpoint | Método | Descripción |
|----------|--------|-------------|
| `/webhooks/yummy` | POST | Receptor de webhooks del gateway (machine-to-machine: se autentica por firma HMAC `X-Gateway-Signature: t=…,v1=…`, ventana de frescura de 5 min, idempotencia por `X-Gateway-Event-Id`; fail-closed 403 sin `YUMMY_ENABLED=1`) |
| `/yummy/status` | GET | Diagnóstico: config completa y últimos 10 eventos recibidos |
| `/yummy/stock/sync` | POST | Envía el stock real de la empresa (suma de lotes activos de `product_lot`) al gateway (`POST /v1/stock/sync`) |
| `/yummy/orders` | GET | Consulta de órdenes de Yummy por la pasarela (`GET /v1/orders?query`) |

### Puesta en marcha (resumen)

1. Levantar la pasarela (ver README de YummyProxy2: `docker compose up -d --build`).
2. Alta del tenant ERP en el gateway con su `callback_url` = `https://TU-ERP/webhooks/yummy`; guardar el `callback_secret` y configurarlo como `YUMMY_CALLBACK_SECRET`.
3. Emitir la key del proyecto con permisos (`stock:read`, `stock:write`, `orders:read`) y configurar `YUMMY_GATEWAY_KEY`.
4. En Yummy configurar la `webhook_url` del tenant (es la del GATEWAY, no la del ERP).
5. En el ERP: `YUMMY_ENABLED=1` y verificar con `GET /yummy/status`.
####################################################################################
