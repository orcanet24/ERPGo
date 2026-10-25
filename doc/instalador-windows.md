# Instaladores Windows para ERPGo y POS

> **Objetivo:** crear instaladores Windows (Inno Setup) que:
> - Instalen ERPGo completo (MySQL, Web, Contabilidad) o solo POS Terminal
> - Soliciten credenciales MySQL y configuración
> - Validen cada aspecto antes de continuar
> - Creen servicios Windows (MySQL + ERPGo web)
> - Creen accesos directos en escritorio
> - Permitan instalación silenciosa
> 
> **Stack:** Inno Setup, compiler `ISCC.exe`, Go compilado a `erpgo.exe` / `pos_client.exe`

---

## 1. Arquitectura del Proyecto de Instalación

```text
/ERPGo/
  /cmd/erpgo/              ← Source Go principal (web + contabilidad)
  /cmd/pos/                ← Source Go POS client (terminal)
  /dist/                    ← Binarios compilados
    ├── erpgo.exe           ← Web + contabilidad + servicio
    └── pos_client.exe      ← Terminal POS standalone
  /installer/inno/         ← Scripts Inno Setup
    ├── ERPGo_Full.iss     ← Instalador ERPGo completo (MySQL + Web)
    └── POS_Terminal.iss   ← Instalador POS terminal (solo cliente)
  /config/                  ← Archivos de configuración base
  /scripts/                 ← Scripts de setup, validación, SQL
```

---

## 2. Herramientas Recomendadas

| Herramienta | Uso | Comentario |
|-------------|-----|------------|
| **Inno Setup** | Installer EXE principal | Scriptable, soporta servicios, atajos, silent install |
| `ISCC.exe` | Compilador Inno Setup | Compila `.iss` → `setup.exe` |
| **Go** | Compilación binarios | `go build -o dist/erpgo.exe ./cmd/erpgo` |

**Por qué Inno Setup:**
- Soporte nativo para servicios Windows (`[Run]` con `service install`)
- Soporta instalación silenciosa (`/VERYSILENT`)
- Soporta validaciones Pascal ([Code] section)
- Soporta atajos de escritorio y menú Start automáticamente
- Común y bien documentado

---

## 3. Instalar Inno Setup

```bash
# 1. Descargar Inno Setup
# https://jrsoftware.org/isdl.php

# 2. Instalar (recomendar default)
# Instalar ISCC.exe en PATH para usar desde línea de comando

# 3. Verificar instalación
ISCC.exe --version
```

---

## 4. Compilar Binarios Go

### Pre-requisito: construir los binarios antes del installer

```bash
# Compilar ERPGo Web (Windows amd64)
cd C:\Proyectos\GO\ERPGo
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o dist\erpgo.exe ./cmd/server

# Compilar POS Client (si es proyecto separado o módulo)
go build -trimpath -ldflags "-s -w" -o dist\pos_client.exe ./cmd/pos

# Verificar
ls -la dist\
```

---

## 5. Installer ERPGo Completo - `ERPGo_Full.iss`

```ini
; ================================================================
; ERPGo Full Installer - Inno Setup Script
; ================================================================
; Compilar con: ISCC.exe ERPGo_Full.iss
; ================================================================

#define MyAppName "ERPGo Empresarial"
#define MyAppVersion "1.0.0"
#define MyAppExe "erpgo.exe"
#define MyCompany "TuEmpresa"

[Setup]
; Identificación
AppId={{A7E5B00A-2861-4E05-9DF1-9EE004B21E04}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyCompany}
AppPublisherURL=https://erpgo.com
AppSupportURL=https://erpgo.com/support
AppUpdatesURL=https://erpgo.com/updates
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
OutputDir=dist
OutputBaseFilename=ERPGo-Setup-{#MyAppVersion}
SetupIconFile=installer/erpgo.ico
Compression=lzma2
SolidCompression=yes
UninstallDisplayIcon={app}\{#MyAppExe}
PrivilegesRequired=admin
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64

; Títulos personalizados
WelcomeLabel1=¡Bienvenido al instalador de {#MyAppName}!
WelcomeLabel2=Por favor, seleccione la ubicación de instalación.

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "desktopicon"; Description: "Crear acceso directo en el escritorio"; Flags: unchecked
Name: "quicklaunchicon"; Description: "Crear acceso en la barra rápida"; Flags: unchecked; OnlyBelowVersion: 6.1; Check: not IsAdminInstallMode

[Files]
; Archivos principales
Source: "dist\erpgo.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\pos_client.exe"; DestDir: "{app}"; Flags: ignoreversion

; Configuraciones
Source: "config\app.example.yaml"; DestDir: "{app}\config"; Flags: ignoreversion; Check: IsAdminInstallMode

; Scripts de setup
Source: "installer\scripts\init_db.sql"; DestDir: "{tmp}"; Flags: ignoreversion
Source: "installer\scripts\validate_env.bat"; DestDir: "{tmp}"; Flags: ignoreversion

[Icons]
Name: "{group}\Abrir {#MyAppName}"; Filename: "http://127.0.0.1:8080"
Name: "{group}\Configuración de servicio"; Filename: "{app}\{#MyAppExe}"; Parameters: "service status"
Name: "{group}\{#MyAppName} (Uninstall)"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "http://127.0.0.1:8080"; Tasks: desktopicon

[Run]
; 1. Detectar o instalar MySQL
Filename: "{tmp}\validate_env.bat"; Description: "Validando entorno..."; Flags: waituntilterminated; StatusMsg: "Validando entorno..."

; 2. Instalar servicio ERPGo
Filename: "{app}\{#MyAppExe}"; Parameters: "service install --name ERPGo"; Description: "Instalando servicio..."; Flags: waituntilterminated; StatusMsg: "Instalando servicio ERPGo..."

; 3. Iniciar servicio
Filename: "{app}\{#MyAppExe}"; Parameters: "service start"; Description: "Iniciando servicio..."; Flags: waituntilterminated; StatusMsg: "Iniciando servicio..."

; 4. Primeros pasos (opcional)
Filename: "{app}\{#MyAppExe}"; Parameters: "--first-run"; Description: "Abrir asistente de configuración"; Flags: postinstall nowait skipifsilent

[Code]
function InitializeSetup(): Boolean;
var
  FreeMB: Cardinal;
begin
  Result := True;

  ; Validar espacio en disco (mínimo 2 GB en la unidad de instalación)
  if not GetSpaceOnDisk64(ExpandConstant('{commonprograms}'), FreeMB, FreeMB) then begin
    FreeMB := GetSpaceOnDisk64(ExpandConstant('{sd}\\'), FreeMB, FreeMB);
  end;

  if (FreeMB < 2048) then begin
    MsgBox('Se requiere al menos 2 GB de espacio libre en disco para instalar {#MyAppName}.' + #13#10 + 
           'Espacio disponible: ' + IntToStr(FreeMB) + ' MB.', mb_Error, MB_OK);
    Result := False;
  end;
end;

procedure CurStepChanged(CurStep: Integer);
begin
  if CurStep = ssPostInstall then begin
    MsgBox('Instalación completada!' + #13#10 + #13#10 +
           'Servicios instalados:' + #13#10 +
           '  - ERPGo Web Service (puerto 8080)' + #13#10 +
           '  - MySQL Server' + #13#10 + #13#10 +
           'A continuación se abrirá el navegador para la configuración inicial.',
           mb_Info, MB_OK);
  end;
end;
```

### Detalle del Script de Validación `validate_env.bat`

```bat
@echo off
REM validate_env.bat - Validar entorno de instalación

setlocal enabledelayedexpansion

echo ========================================
echo Validando entorno para ERPGo
echo ========================================

REM 1. Verificar que MySQL está instalado (búsqueda de servicio)
sc query MySQL >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [!] MySQL no encontrado. Se usará la instalación bundled...
    goto :install_mysql
)

echo [OK] MySQL encontrado como servicio

REM 2. Verificar que MySQL responde
mysql -u root -e "SELECT 1" >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [!] MySQL no responde. Revisar configuración...
    goto :error
)

echo [OK] MySQL respondiendo

REM 3. Verificar espacio (mínimo 500 MB libres en C:)
wmic OS get FreePhysicalMemory | find /V "FreePhysicalMemory" | find /V "=" > nul
if %ERRORLEVEL% NEQ 0 (
    echo [OK] Espacio suficiente
) else (
    echo [!] Espacio insuficiente
    goto :error
)

goto :success

:install_mysql
echo [*] Iniciando instalación de MySQL Server...
; Aquí iría la lógica para instalar MySQL bundled o descargar
; Por ahora, fallar con mensaje claro
echo [!] MySQL bundled no implementado. Instale MySQL manualmente.
exit /b 1

:error
echo.
echo ========================================
echo Error de validación
echo ========================================
exit /b 1

:success
echo.
echo ========================================
echo Validación completada
echo ========================================
exit /b 0
```

---

## 6. Installer POS Terminal - `POS_Terminal.iss`

```ini
; ================================================================
; POS Terminal Installer - Inno Setup Script
; ================================================================
; Para instalación standalone de terminales POS sin BD local
; ================================================================

#define MyAppName "POS Terminal"
#define MyAppVersion "1.0.0"
#define MyAppExe "pos_client.exe"

[Setup]
AppId={{B7E5C00A-2961-4F05-9EF1-9EE004C31F05}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyCompany}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
OutputDir=dist
OutputBaseFilename=POS-Terminal-Setup-{#MyAppVersion}
SetupIconFile=installer/pos.ico
Compression=lzma2
SolidCompression=yes
UninstallDisplayIcon={app}\{#MyAppExe}
PrivilegesRequired=admin
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "desktopicon"; Description: "Crear acceso directo en el escritorio"; Flags: unchecked

[Files]
Source: "dist\pos_client.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\*.dll"; DestDir: "{app}"; Flags: ignoreversion; Check: IsAdminInstallMode
Source: "pos\assets\*"; DestDir: "{app}\assets"; Flags: ignoreversion

[Icons]
Name: "{group}\Abrir POS Terminal"; Filename: "{app}\{#MyAppExe}"
Name: "{group}\Configuración POS"; Filename: "{app}\{#MyAppExe}"; Parameters: "--config"
Name: "{group}\{#MyAppName} (Uninstall)"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExe}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExe}"; Parameters: "--setup"; Description: "Configurando terminal POS..."; Flags: waituntilterminated; StatusMsg: "Configurando terminal..."

[Code]
function InitializeSetup(): Boolean;
begin
  Result := True;
end;
```

---

## 7. Detalles de Configuración de MySQL

### Esquema de permisos recomendado (least privilege)

```sql
-- Crear base de datos
CREATE DATABASE IF NOT EXISTS erpgo
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

-- Crear usuario de aplicación (no root!)
CREATE USER 'erpgo_app'@'localhost'
  IDENTIFIED BY 'contraseña_segura_generada';

-- Permisos mínimos para ejecución
GRANT SELECT, INSERT, UPDATE, DELETE,
      CREATE, ALTER, INDEX, DROP,
      CREATE TEMPORARY TABLES
  ON erpgo.* TO 'erpgo_app'@'localhost';

-- Permisos adicionales de migración (solo durante setup)
-- GRANT CREATE, ALTER, DROP ON erpgo.* TO 'erpgo_migrator'@'localhost';
-- IDENTIFIED BY 'contraseña_migrador_segura';

FLUSH PRIVILEGES;
```

### Configuración en `config/app.example.yaml`

```yaml
database:
  host: "127.0.0.1"
  port: 3306
  user: "erpgo_app"
  password: "contraseña_segura_generada"  ; ← reemplazar en instalación
  name: "erpgo"
  charset: "utf8mb4"

server:
  host: "0.0.0.0"
  port: 8080

pos:
  enabled: true
  auth_token_ttl: 24h
```

---

## 8. Servicios Windows

### Implementación recomendada en Go (usar `kardianos/service`)

```go
// cmd/server/service.go
package main

import (
    "log"
    "os"

    "github.com/kardianos/service"
)

var program = &ERPGoService{}

type ERPGoService struct{}

func (p *ERPGoService) Name() string {
    return "ERPGo"
}

func (p *ERPGoService) DisplayName() string {
    return "ERPGo Server"
}

func (p *ERPGoService) Description() string {
    return "Servidor ERPGo - Contabilidad, ventas, POS"
}

func (p *ERPGoService) Start(s service.Service) error {
    // Iniciar servidor HTTP, migrar BD, etc.
    log.Println("ERPGo iniciando...")
    go runServer()
    return nil
}

func (p *ERPGoService) Stop(s service.Service) error {
    // Graceful shutdown
    log.Println("ERPGo deteniéndose...")
    return nil
}

func main() {
    svcConfig := &service.Config{
        Name:        "ERPGo",
        DisplayName: "ERPGo Server",
        Description: "Servidor ERPGo",
    }

    prg := &ERPGoService{}
    s, err := service.New(prg, svcConfig)
    if err != nil {
        log.Fatal(err)
    }

    // Subcommands: service, install, remove, start, stop, status
    if len(os.Args) > 1 {
        var err error
        switch os.Args[1] {
        case "service":
            err = service.Control(s, os.Args[2])
        case "install":
            err = s.Install()
        case "remove":
            err = s.Remove()
        case "start":
            err = s.Start()
        case "stop":
            err = s.Stop()
        case "status":
            err = service.Control(s, "status")
        default:
            err = fmt.Errorf("comando desconocido: %s", os.Args[1])
        }
        if err != nil {
            log.Fatal(err)
        }
    }
}
```

### Comandos de servicio (para usar en installer)

```bash
# Instalar servicio
"C:\Program Files\ERPGo\erpgo.exe" service install

# Iniciar servicio
"C:\Program Files\ERPGo\erpgo.exe" service start

# Detener servicio
"C:\Program Files\ERPGo\erpgo.exe" service stop

# Estado
"C:\Program Files\ERPGo\erpgo.exe" service status

# Eliminar servicio (desinstalación)
"C:\Program Files\ERPGo\erpgo.exe" service remove
```

---

## 9. Script de Pos-instalación (First-Run Wizard)

### `cmd/server/firstrun.go`

```go
// firstrun.go - Asistente de configuración inicial
package main

import (
    "bufio"
    "fmt"
    "os"

    "github.com/joho/godotenv"
)

type ConfigSetup struct {
    DBHost     string
    DBPort     int
    DBUser     string
    DBPassword string
    DBName     string
    AdminUser  string
    AdminPass  string
}

func (c *ConfigSetup) Prompt() error {
    reader := bufio.NewReader(os.Stdin)

    fmt.Println("=== Configuración inicial de ERPGo ===")
    fmt.Println()

    // MySQL
    fmt.Print("Host MySQL (default 127.0.0.1): ")
    c.DBHost, _ = reader.ReadString('\n')
    if c.DBHost == "" {
        c.DBHost = "127.0.0.1"
    }

    fmt.Print("Puerto MySQL (default 3306): ")
    fmt.Fscanln(reader, &c.DBPort)
    if c.DBPort == 0 {
        c.DBPort = 3306
    }

    fmt.Print("Usuario MySQL: ")
    c.DBUser, _ = reader.ReadString('\n')

    fmt.Print("Contraseña MySQL: ")
    c.DBPassword, _ = reader.ReadString('\n')

    fmt.Print("Nombre de base de datos (default erpgo): ")
    c.DBName, _ = reader.ReadString('\n')
    if c.DBName == "" {
        c.DBName = "erpgo"
    }

    // Admin ERPGo
    fmt.Println()
    fmt.Println("--- Crear usuario administrador ---")
    fmt.Print("Usuario admin ERPGo: ")
    c.AdminUser, _ = reader.ReadString('\n')

    fmt.Print("Contraseña admin ERPGo: ")
    c.AdminPass, _ = reader.ReadString('\n')

    return nil
}

func (c *ConfigSetup) Save() error {
    return godotenv.Write(map[string]string{
        "DB_HOST":     c.DBHost,
        "DB_PORT":     fmt.Sprintf("%d", c.DBPort),
        "DB_USER":     c.DBUser,
        "DB_PASSWORD": c.DBPassword,
        "DB_NAME":     c.DBName,
        "ADMIN_USER":  c.AdminUser,
        "ADMIN_PASS": c.AdminPass,
    }, ".env")
}

func RunFirstRun() {
    cfg := &ConfigSetup{}
    if err := cfg.Prompt(); err != nil {
        log.Fatal(err)
    }
    if err := cfg.Save(); err != nil {
        log.Fatal(err)
    }
    fmt.Println()
    fmt.Println("✓ Configuración guardada en .env")
    fmt.Println()
    fmt.Println("Ahora ejecutar:")
    fmt.Println("  go run ./cmd/server")
}
```

---

## 10. Validaciones del Installer

### Pre-requisitos (validar antes de instalar)

| Validación | Comprobación | Fallo |
|------------|--------------|-------|
| Espacio en disco | ≥ 2 GB libres | Error |
| Windows versión | ≥ Windows 10 / Server 2016 | Advertencia |
| RAM | ≥ 4 GB (advertencia) | Advertencia |
| MySQL Service | `sc query MySQL` OK | Instalar si no existe |
| Puerto 8080 libre | `netstat -an | findstr :8080` | Error si ocupado |

### Post-instalación (validar después)

| Validación | Comprobación |
|------------|--------------|
| Servicio ERPGo corre | `sc query ERPGo` → RUNNING |
| Navegador abre `http://127.0.0.1:8080` | Manual o automatizado |
| BD crecida y schema aplicado | `SHOW TABLES FROM erpgo;` |
| Usuario admin creado | Query en `erpgo.users` |

---

## 11. Instalación Silenciosa (Unattended)

### Comando de instalación silenciosa

```powershell
# ERPGo Completo - silencioso
.\ERPGo-Setup-1.0.0.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /DIR="C:\Program Files\ERPGo" /TASKS="desktopicon"

# POS Terminal - silencioso
.\POS-Terminal-Setup-1.0.0.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /DIR="C:\Program Files\POS" /TASKS="desktopicon"
```

### Variables de entorno para configuración siemántica

```powershell
$env:DB_HOST = "127.0.0.1"
$env:DB_PORT = "3306"
$env:DB_USER = "erpgo_app"
$env:DB_PASSWORD = "contraseña_segura"
$env:DB_NAME = "erpgo"

.\ERPGo-Setup-1.0.0.exe /VERYSILENT
```

---

## 12. Proceso de Actualización (Upgrade)

```powershell
# 1. Detener servicio
"C:\Program Files\ERPGo\erpgo.exe" service stop

# 2. Copiar nuevos archivos (Inno Setup lo hace automáticamente)
# Inno Setup detecta versión existente y solo reemplaza archivos cambiados

# 3. Ejecutar migraciones (si el servicio tiene flag)
"C:\Program Files\ERPGo\erpgo.exe" migrate --up

# 4. Reiniciar servicio
"C:\Program Files\ERPGo\erpgo.exe" service start

# 5. Verificar
"C:\Program Files\ERPGo\erpgo.exe" service status
```

---

## 13. Checklist de Validación del Installer

Antes de publicar, verificar:

- [ ] Compilación de binarios Go: `dist/erpgo.exe`, `dist/pos_client.exe` OK
- [ ] Script Inno Setup compila sin errores: `ISCC.exe ERPGo_Full.iss`
- [ ] Installer.exe generado en `dist/`
- [ ] Instalación en VM limpia de Windows 10/11
- [ ] MySQL instalado (o detectado) y corriendo
- [ ] Servicio ERPGo creado: `sc query ERPGo`
- [ ] Servicio ERPGo iniciado automaticamente
- [ ] `http://127.0.0.1:8080` responde en navegador
- [ ] Acceso directo en escritorio creado
- [ ] Primer-run wizard funciona (si aplica)
- [ ] Desinstalación elimina archivos y servicios correctamente
- [ ] Instalación silenciosa funciona: `/VERYSILENT`
- [ ] Logs de instalación generados en `%temp%\`
- [ ] Binarios firmados (opcional, para producción)

---

## 14. Publicación

```bash
# 1. Compilar binarios Go
cd C:\Proyectos\GO\ERPGo
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -o dist/erpgo.exe ./cmd/server
go build -o dist/pos_client.exe ./cmd/pos

# 2. Compilar instaladores
ISCC.exe installer/inno/ERPGo_Full.iss
ISCC.exe installer/inno/POS_Terminal.iss

# 3. Verificar outputs
ls -la dist\*.exe

# 4. Firmar (si es necesario, requiere certificado digital)
signtool sign /f certificate.pfx /p password dist/*.exe
```

---

## 15. Archivos y Estructura Final del Proyecto

```
ERPGo/
├── cmd/
│   ├── server/
│   │   ├── main.go
│   │   ├── service.go           ← Implementación servicio Windows
│   │   └── firstrun.go          ← Asistente configuración inicial
│   └── pos/
│       ├── main.go
│       └── pos_terminal.go
├── dist/
│   ├── erpgo.exe               ← Grupo ERPGo completo
│   ├── pos_client.exe          ← Grupo POS terminal
│   ├── ERPGo-Setup-1.0.0.exe  ← Installer ERPGo (generado)
│   └── POS-Terminal-Setup-1.0.0.exe  ← Installer POS (generado)
├── installer/
│   ├── erpgo.ico               ← Icono ERPGo
│   ├── pos.ico                 ← Icono POS
│   └── inno/
│       ├── ERPGo_Full.iss     ← Script instalador ERPGo
│       └── POS_Terminal.iss   ← Script instalador POS
├── config/
│   └── app.example.yaml        ← Config por defecto
└── scripts/
    ├── init_db.sql            ← Creación BD y usuario
    └── validate_env.bat       ← Validación entorno
```

---

## 16. Problemas Comunes y Soluciones

| Problema | Causa | Solución |
|----------|-------|----------|
| "Access denied" instalando servicio | No ejecutar como Admin | Ejecutar instalador como Administrador |
| MySQL no instala | No se descargó MySQL installer | Descargar MySQL MSI y empaquetar en installer |
| Puerto 8080 ocupado | Otra app usando puerto | Cambiar puerto en config o liberar puerto |
| Servicio no inicia | Error en código Go | Revisar logs de Windows Event Viewer |
| Instalación silenciosa falla | Error no manejado en setup | Ver logs `/LOG=` o `%temp%` |
| Upgrade rompe datos | Migraciones reversibles | Usar migraciones versionadas y backups |

---

## 17. Siguientes Pasos

1. **Crear el servicio Windows en Go** usando `kardianos/service`
2. **Escribir los scripts Inno Setup** con los detalles arriba
3. **Compilar y probar** en VMs limpias
4. **Agregar soporte para MySQL bundled** si se requiere auto-instalación
5. **Implementar first-run wizard** en Go
6. **Documentar para usuario final** (esta guía en /doc/)
7. **Pruebas de integración** con POS → ERPGo completa
8. **Firma de binarios** para distribución oficial

---

*Documento creado para ERPGo v1.0 - Guía de instalación Windows*
