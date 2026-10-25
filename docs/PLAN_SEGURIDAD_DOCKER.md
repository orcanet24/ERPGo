# Plan Detallado: Seguridad Multi-Tenant y Despliegue Docker

## Fase 1: Seguridad Multi-Empresa (Usuarios por BD)

### 1.1 Extensión de la tabla `tenants`

**Archivo:** `migrations/schema_multi_tenant_security.sql`

```sql
-- Extensión del catálogo de empresas para credenciales por empresa
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS db_user VARCHAR(128) DEFAULT '';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS db_password_enc VARBINARY(512) DEFAULT '';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS status INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS max_users INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS created_by VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS db_size_mb INTEGER NOT NULL DEFAULT 0;
```

### 1.2 Paquete de encriptación

**Archivo:** `internal/security/crypto.go`

```go
package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "io"
)

// Deriva una llave AES-256 desde JWT_SECRET
func DeriveKey(jwtSecret string) []byte {
    h := sha256.Sum256([]byte(jwtSecret + "::erpgo-db-key"))
    return h[:]
}

// Encripta con AES-256-GCM
func Encrypt(plaintext string, key []byte) (string, error) {
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Desencripta
func Decrypt(ciphertext string, key []byte) (string, error) {
    data, _ := base64.StdEncoding.DecodeString(ciphertext)
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonceSize := gcm.NonceSize()
    nonce, encrypted := data[:nonceSize], data[nonceSize:]
    plaintext, _ := gcm.Open(nil, nonce, encrypted, nil)
    return string(plaintext), nil
}
```

### 1.3 Modelo Tenant extendido

**Archivo:** `interfaces/web/tenants_model.go`

```go
type TenantCredentials struct {
    Code       string
    DBUser     string
    DBPassword string  // desencriptado en memoria
    Status     int
    MaxUsers   int
}
```

### 1.4 Provisionador de empresas con credenciales

**Modifica:** `interfaces/web/tenants.go` → `ProvisionTenant()`

Nuevo flujo:
```go
func (s *Server) ProvisionTenant(...) (ProvisionTenantResult, error) {
    // ... validaciones existentes ...
    
    // 1. Generar credenciales únicas
    dbUser := "erpgo_" + slug  // ej: erpgo_acme
    dbPass := generateSecurePassword(24)
    
    // 2. Crear usuario MySQL dedicado (con usuario de provisión)
    createMySQLUser(dbUser, dbPass)
    
    // 3. Crear BD vacía
    createDatabase("erpgo_" + slug)
    
    // 4. Dar permisos SOLO a esa BD
    grantPermissions(dbUser, "erpgo_"+slug)
    
    // 5. Encriptar contraseña y guardar en catálogo
    encryptedPass := Encrypt(dbPass, masterKey)
    saveToCatalog(code, dbUser, encryptedPass)
    
    // 6. Aplicar migraciones con el usuario de la empresa
    runMigrations(dbUser, dbPass, "erpgo_"+slug)
    
    // 7. Siembra con credenciales de empresa
    seedCompany(dbUser, dbPass, "erpgo_"+slug, adminUser, adminPass)
}
```

### 1.5 Conexión por empresa con credenciales propias

**Modifica:** `interfaces/web/tenants.go` → `openTenantDB()`

```go
func (s *Server) openTenantDB(code string) (*sql.DB, error) {
    // ... cacheo existente ...
    
    // Leer credenciales del catálogo
    tenant := s.getTenantCredentials(code)
    if tenant == nil {
        return nil, fmt.Errorf("empresa no encontrada en catálogo")
    }
    
    // Desencriptar contraseña
    pass := Decrypt(tenant.DBPasswordEnc, s.masterKey)
    
    // Conectar con credenciales únicas de la empresa
    return persistence.OpenMySQL(persistence.ConfigMySQL{
        Host:    s.config.DBHost,
        Port:    s.config.DBPort,
        Usuario: tenant.DBUser,    // usuario único
        Clave:   pass,              // contraseña única
        Nombre:  "erpgo_" + code,
    })
}
```

### 1.6 Panel de gestión de empresas

**Archivo:** `interfaces/web/tenant_admin.go`

Nuevo panel (solo master):
- `GET /admin/tenants` — lista empresas con estado, tamaño, usuarios
- `POST /admin/tenants/create` — crear empresa (con validaciones)
- `POST /admin/tenants/{code}/suspend` — suspender empresa
- `POST /admin/tenants/{code}/delete` — eliminar empresa (con confirmación)
- `POST /admin/tenants/{code}/rotate-pass` — rotar contraseña de BD
- `GET /admin/tenants/{code}/stats` — estadísticas de uso

### 1.7 Configuración de provisión

**Modifica:** `configs/config.go`

```go
type Config struct {
    // ... existentes ...
    
    // Usuario con permisos para CREAR bases y usuarios
    // Se usa SOLO al crear/eliminar empresas, no para operación diaria
    DBProvisionUser     string
    DBProvisionPassword string
}
```

```go
// En Load():
DBProvisionUser:     getEnv("DB_PROVISION_USER", ""),
DBProvisionPassword: getEnv("DB_PROVISION_PASSWORD", ""),
```

### 1.8 Migración de empresas existentes

**Script:** `tools/migrate_to_per_company_users.go`

```go
// Para cada empresa en el catálogo:
// 1. Crear usuario MySQL dedicado
// 2. Migrar permisos
// 3. Actualizar registro en tenants
// 4. Verificar conexión
```

---

## Fase 2: Compilación Protegida

### 2.1 Script de compilación para Linux

**Archivo:** `compilar-lanzamiento-linux.sh`

```bash
#!/bin/bash
# Requiere Go 1.27+ y garble
export GOTOOLCHAIN=go1.27.1

garble -literals build \
  -ldflags "-s -w -X main.fieldKeyCifrada=${LLAVE_HEX}" \
  -o bin/linux/erpgo-lanzamiento \
  ./cmd/server

# Firmar
go run ./tools/firmar --binario bin/linux/erpgo-lanzamiento
```

### 2.2 Script de compilación para Windows

Ya existe: `compilar-lanzamiento.bat`

### 2.3 Output esperido

```
bin/
├── windows/
│   ├── erpgo-lanzamiento.exe
│   ├── erpgo-lanzamiento.sha256
│   └── erpgo-lanzamiento.sig
└── linux/
    ├── erpgo-lanzamiento
    ├── erpgo-lanzamiento.sha256
    └── erpgo-lanzamiento.sig
```

---

## Fase 3: Docker Compose

### 3.1 Estructura del directorio docker

```
instalador/docker/
├── docker-compose.yml
├── .env.example
├── erpgow/                    # Binario Linux copiado aquí
│   └── erpgo-lanzamiento
├── mysql/
│   └── init/
│       └── 01-init.sql        # Crea BD control + usuario de provisión
├── backups/                    # Volumen para backups
└── data/                       # Volumen para datos MySQL
```

### 3.2 docker-compose.yml

```yaml
version: '3.8'

services:
  mysql:
    image: mysql:8.0
    container_name: erpgo-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: ${DB_NAME:-erpgo}
    volumes:
      - mysql_data:/var/lib/mysql
      - ./mysql/init:/docker-entrypoint-initdb.d
    ports:
      - "${MYSQL_PORT:-3306}:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  erpgow:
    build:
      context: ./erpgow
    container_name: erpgo-server
    restart: unless-stopped
    depends_on:
      mysql:
        condition: service_healthy
    environment:
      DB_HOST: mysql
      DB_PORT: 3306
      DB_USER: ${DB_USER}
      DB_PASSWORD: ${DB_PASSWORD}
      DB_NAME: ${DB_NAME:-erpgo}
      DB_PROVISION_USER: ${DB_PROVISION_USER}
      DB_PROVISION_PASSWORD: ${DB_PROVISION_PASSWORD}
      SERVER_PORT: ${SERVER_PORT:-8080}
      SERVER_HOST: 0.0.0.0
      JWT_SECRET: ${JWT_SECRET}
      GRPC_PORT: ${GRPC_PORT:-9095}
      APP_ENV: production
    volumes:
      - ./backups:/app/backups
      - ./uploads:/app/uploads
    ports:
      - "${SERVER_PORT:-8080}:8080"

volumes:
  mysql_data:
```

### 3.3 Dockerfile

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /app
COPY erpgo-lanzamiento .
COPY .env .
RUN chmod +x erpgo-lanzamiento
EXPOSE 8080
CMD ["./erpgo-lanzamiento"]
```

### 3.4 Init SQL para MySQL

```sql
-- Crear usuario de provisión (crea empresas)
CREATE USER IF NOT EXISTS '${DB_PROVISION_USER}'@'%' 
  IDENTIFIED BY '${DB_PROVISION_PASSWORD}';
GRANT ALL PRIVILEGES ON `erpgo_%`.* TO '${DB_PROVISION_USER}'@'%';
GRANT CREATE, DROP, CREATE USER, RELOAD ON *.* TO '${DB_PROVISION_USER}'@'%';
FLUSH PRIVILEGES;

-- Crear usuario de operación (usa empresas existentes)
CREATE USER IF NOT EXISTS '${DB_USER}'@'%' 
  IDENTIFIED BY '${DB_PASSWORD}';
GRANT SELECT, INSERT, UPDATE, DELETE, ALTER, INDEX, CREATE, DROP, 
      CREATE VIEW, SHOW VIEW, TRIGGER ON `erpgo_%`.* TO '${DB_USER}'@'%';
FLUSH PRIVILEGES;
```

---

## Cronograma de implementación

| Paso | Tarea | Archivos |
|------|-------|----------|
| 1 | Crear paquete crypto | `internal/security/crypto.go` |
| 2 | Extender tabla tenants | Nueva migración |
| 3 | Crear provisionador | Modificar `tenants.go` |
| 4 | Modificar `openTenantDB` | Modificar `tenants.go` |
| 5 | Panel admin | Nuevo `tenant_admin.go` |
| 6 | Config de provisión | Modificar `config.go` |
| 7 | Compilar protegido Linux | Script nuevo |
| 8 | Compilar protegido Windows | Ya existe |
| 9 | Docker Compose | `instalador/docker/` |
| 10 | Documentar todo | READMEs + docs |

## Resultado final

```
 Servidor Linux
 ├── Docker Compose
 │   ├── MySQL (con init SQL)
 │   │   ├── BD control (erpgo)
 │   │   ├── BD empresa A (erpgo_a)
 │   │   ├── BD empresa B (erpgo_b)
 │   │   └── Usuarios MySQL por empresa
 │   └── ERPGo (binario protegido)
 │       ├── Escucha :8080
 │       ├── gRPC :9095
 │       └── Solo crea empresas el super admin
 └── Backups automatizados
```

## Notas de seguridad

1. **Nunca root en runtime** — solo el usuario de provisión crea empresas
2. **Credenciales encriptadas** — AES-256-GCM con llave derivada de JWT_SECRET
3. **Un credential = una empresa** — fuga de un credential no compromete a las demás
4. **Binario protegido** — garble + XOR + firma Ed25519
5. **Docker no expone root** — el MySQL root solo está en el init, no en la app
