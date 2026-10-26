@echo off
rem ============================================================================
rem ERPGo - actualizacion del SERVIDOR desde GitHub (WINDOWS + Caddy)
rem
rem Uso (en el servidor, cmd):
rem   scripts\deploy\actualizar_desde_git.bat
rem
rem Variables a ajustar segun la instalacion:
rem   SERVICES=nombre del servicio Windows (ej: erpgo; si usas nssm: nssm restart erpgo)
rem   RESETYPE=systemctl | nssm | schtasks | none
rem   ERPDIR  = carpeta del codigo (default: dos niveles arriba de este script)
rem
rem Igual que la version Linux: solo compila SI hay commit nuevo, construye el
rem binario NUEVO al lado y solo reemplaza si compilacion y reinicio pueden
rem ejecutarse; si la compilacion falla, produccion sigue con el binario actual.
rem ============================================================================
setlocal EnableDelayedExpansion
cd /d %~dp0..\..

rem --- Configuracion ---
if "%ERPDIR%"=="" cd /d %~dp0..\..
if "%RESETYPE%"=="" set RESETYPE=nssm
if "%SERVICE%"=="" set SERVICE=erpgo
if "%HEALTHURL%"=="" set HEALTHURL=http://127.0.0.1:8080/health

git fetch origin main
for /f %%i in ('git rev-parse HEAD') do set LOCALED=%%i
for /f %%i in ('git rev-parse origin/main') do set REMOTED=%%i

if "%LOCALED%"=="%REMOTED%" (
    echo [deploy] Sin cambios. Nada que hacer.
    exit /b 0
)

echo [deploy] Nuevo commit detectado: %REMOTED:~0,10%
git pull --ff-only origin main
if errorlevel 1 ( echo [deploy] ERROR en pull & exit /b 1 )

echo [deploy] Compilando binario nuevo...
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -o erpgow.binario.nuevo.exe .\cmd\server
if errorlevel 1 ( echo [deploy] ERROR compilando; produccion sigue con el binario actual & exit /b 1 )

rem Reemplazo solo cuando ya se garantizo el build correcto
move /y erpgow.binario.nuevo.exe erpgow.exe >nul

echo [deploy] Reiniciando servicio %SERVICE%...
if /i "%RESETYPE%"=="nssm" (nssm restart %SERVICE%)
if /i "%RESETYPE%"=="schtasks" (schtasks /end /tn "ERPGo" & schtasks /run /tn "ERPGo")
if /i "%RESETYPE%"=="manual" echo [deploy] Reinicia el servicio manualmente.

echo [deploy] Verificando salud: %HEALTHURL%
set /a INT=0
:espera
set /a INT+=1
timeout /t 3 /nobreak >nul
curl -fsS %HEALTHURL% >nul 2>&1
if not errorlevel 1 goto salud
if %INT% lss 20 goto espera
echo [deploy] ADVERTENCIA: /health no respondio despues de 60 s — revisa el servicio ya.
exit /b 1
:salud
echo [deploy] ERP arriba con el commit
for /f %%i in ('git rev-parse --short HEAD') do echo    %%i
exit /b 0
