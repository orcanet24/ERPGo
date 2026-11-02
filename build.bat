@echo off
REM build.bat — Compila el binario Windows de ERPGo en bin/ (no subido a git).
REM
REM Por qué un .bat en vez de un Makefile: Windows no trae make por defecto,
REM y este proyecto se desarrolla en Windows. El binario va a bin\ que está en
REM .gitignore, así no se comitea el ejecutable.
REM
REM Qué verifica este script: que la compilación produce un binario ejecutable
REM válido (existe y con tamaño ^> 0). NO ejecuta el binario al final: el ERP
REM necesita un servidor MySQL/MariaDB escuchando en DB_PORT, y lanzarlo sin
REM motor daría un falso negativo (compiló bien, falló en runtime por MySQL).
REM El usuario lo ejecuta a mano cuando tiene el entorno listo.

setlocal

REM El directorio bin/ no se sube a GitHub: está en .gitignore.
if not exist bin mkdir bin

echo Compilando ERPGo...
go build -o bin/ERPGo.exe ./cmd/server
if errorlevel 1 (
    echo/
    echo [ERROR] La compilacion fallo.
    exit /b 1
)

REM Verificacion post-compilacion: el binario existe y tiene contenido.
if not exist bin\ERPGo.exe (
    echo/
    echo [ERROR] El binario no se creo.
    exit /b 1
)

for %%A in (bin\ERPGo.exe) do set SIZE=%%~zA
if %SIZE% LSS 1000 (
    echo/
    echo [ERROR] El binario esta vacio o corrupto ^(tamanio: %SIZE% bytes^).
    exit /b 1
)

echo/
echo Binario creado: bin\ERPGo.exe ^(%SIZE% bytes^)
echo/
echo Proximo paso: configure .env (DB_HOST, DB_USER, DB_PASSWORD...) y ejecute:
echo     bin\ERPGo.exe

exit /b 0
