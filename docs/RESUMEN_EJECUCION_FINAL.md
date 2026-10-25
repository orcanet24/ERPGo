# Resumen Final: Seguridad Multi-Tenant + Setup Wizard + Docker

## Estado de Implementación

### ✅ COMPLETADO

| Componente | Estado |
|------------|--------|
| **Paquete crypto** (AES-256-GCM) | ✅ Tests passing |
| **Migración de controlSchema** | ✅ Columnas db_user, db_password_enc en tenants |
| **Provisionador extendido** | ✅ Genera credenciales por empresa, encripta, guarda |
| **Configuración** | ✅ GRPC_PORT, DB_PROVISION_USER, DB_PROVISION_PASSWORD en .env |
| **Validación de puertos** | ✅ Valida y sugiere alternativas |
| **Schema validator** | ✅ Compara schema esperado vs real, genera ALTERs |
| **Backup manager** | ✅ Respaldo antes de resetear |
| **Health endpoint** | ✅ /health para Docker |
| **--setup wizard** | ✅ Interfaz interactiva por consola |
| **Binario Windows** | ✅ bin/windows/erpgo-lanzamiento.exe (45MB) |
| **Binario Linux** | ✅ bin/linux/erpgo-lanzamiento (45MB ELF) |
| **Docker Compose** | ✅ Solo app (sin MySQL externo) |
| **POS configurable** | ✅ POS_GRPC_PORT en .env |

### ⚠️ PENDIENTE (TODOs marcados en código)

| Acción | Ubicación |
|--------|-----------|
| Crear empresa inicial desde wizard | `wizard.go` → `runFreshInstall()` |
| Crear empresa desde menú mantenimiento | `wizard.go` → `runCreateTenant()` |
| Implementar backup real en reset | `wizard.go` → `runReset()` |
| Instalar servicio OS desde wizard | `wizard.go` |

---

## Cómo Usar el Binario

### Windows
```
erpgo --setup          → Wizard de instalación
erpgo --migrate        → Solo migraciones
erpgo --install        → Instalar como servicio Windows
erpgo --uninstall      → Desinstalar servicio Windows
erpgo                  → Arrancar servidor
```

### Linux
```
./erpgow --setup       → Wizard de instalación
./erpgow --migrate     → Solo migraciones
./erpgow               → Arrancar servidor
```

### Docker
```bash
cd instalador/docker
cp .env.example .env
# Editar .env con tu servidor MySQL
docker-compose up -d
```

---

## Archivos del Setup Wizard

| Archivo | Función |
|---------|---------|
| `internal/setup/ports.go` | Validación y recomendación de puertos |
| `internal/setup/wizard.go` | Wizard interactivo principal |
| `internal/setup/schema_check.go` | Compara schema vs real, genera ALTERs |
| `internal/setup/backup.go` | Respaldo antes de operaciones destructivas |
| `internal/setup/mariadb.go` | Instalador MariaDB para Windows/Linux |
| `interfaces/web/health.go` | Endpoint /health para Docker |
| `cmd/server/main.go` | Entry point con flag --setup |

---

## Validación de Puertos

El wizard valida automáticamente:
- Puerto ERP (default 8080)
- Puerto MySQL (default 3307)
- Puerto gRPC (default 9095)

Si un puerto está ocupado:
- Muestra mensaje de error
- Sugiere 5 puertos alternativos
- Permite reintentar sin salir del wizard

---

## Flujo de Instalación Nueva

```
[1/6] ¿Instalar MariaDB?
      → Sí: Instala MariaDB en ./mariadb, inicia servicio
      → No: Pide host:puerto de MySQL existente
      
[2/6] Validar puertos (8080, 3307, 9095)
      → Si hay conflicto: mostrar alternativas y reintentar
      
[3/6] Puerto HTTP del ERP
[4/6] Puerto gRPC (POS)
[5/6] JWT Secret (auto-generado si vacío)
[6/6] Empresa inicial
```

---

## Seguridad Implementada

1. **Credenciales por empresa**: AES-256-GCM encriptadas en catálogo
2. **Usuario de operación**: Sin CREATE/DROP DATABASE (usa empresas existentes)
3. **Usuario de provisión**: Solo para crear/eliminar empresas (no accede a datos)
4. **Puertos editables**: Sin hardcodeo, configurables por .env
5. **Validación de puertos**: Evita conflictos antes de iniciar
6. **Binario único**: go:embed, sin dependencias externas

---

## Próximos Pasos

1. Completar la integración de ProvisionTenantExtended en wizard.go
2. Agregar soporte systemd para Linux (--install en Linux)
3. Implementar descarga automática de MariaDB para Docker
4. Crear panel HTML de administración de empresas (solo master)
