@echo off
rem ============================================================================
rem ERPGo - Compilacion para WINDOWS
rem Uso (en cmd):
rem   scripts\build.bat           - compilacion corriente (vet + tests + build)
rem   scripts\build.bat seguro    - compilacion OFUSCADA (garble, anti-reversa)
rem
rem Que hace "seguro":
rem   - garble reemplaza nombres de funciones/variables por aleatorios,
rem     ofusca literales (strings) y borra las rutas de fuente del binario.
rem   - -trimpath: ninguna ruta de tu maquina queda embebida.
rem   - -ldflags "-s -w": sin tabla de simbolos ni debug info.
rem   - -buildmode=pie: binario independiente de posicion (PIE), mas dificil
rem     de parchar en caliente.
rem   - CGO desactivado para reproducibilidad.
rem
rem Nunca es 100% - el que recibe el binario, recibe el programa - pero esta
rem combinacion elimina lo facil: nombres claros, strings legibles y paths.
rem ============================================================================
setlocal
cd /d %~dp0..

echo ==^> go vet .\cmd\server
go vet ./cmd/server
if errorlevel 1 goto :fallo

echo ==^> go test .\internal\...
go test ./internal/...
if errorlevel 1 goto :fallo

if /i "%~1"=="seguro" goto :seguro

echo ==^> Compilacion estandar...
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -buildmode=pie -o erpgow.exe .\cmd\server
if errorlevel 1 (
    echo ^(PIE no soportado en este host; compilando estandar^)
    go build -trimpath -ldflags "-s -w" -o erpgow.exe .\cmd\server
)
goto listo

:seguro
echo ==^> Compilacion OFUSCADA con garble...
where garble >nul 2>nul
if errorlevel 1 (
    echo Falta garble. Instalalo una vez con:
    echo     go install mvdan.cc/garble@latest
    echo ^(garble pide una version de Go igual o mas nueva que la del modulo;
    echo  si falla 'Go version ... is too old', actualiza Go y reintenta^)
    goto :fallo
)
set CGO_ENABLED=0
garble -tiny -literals -seed=random build -trimpath -ldflags "-s -w" -buildmode=pie -o erpgow.exe .\cmd\server
if errorlevel 1 (
    echo ^(PIE fallo con garble; reintentando estandar ofuscado^)
    garble -tiny -literals -seed=random build -trimpath -ldflags "-s -w" -o erpgow.exe .\cmd\server
    if errorlevel 1 goto :fallo
)
echo Recuerda guardar la semilla de garble si compararas builds.

:listo
echo.
echo ==^> Listo: erpgow.exe
exit /b 0

:fallo
echo ==^> ERROR: la compilacion fallo. Revisa los mensajes de arriba.
exit /b 1
