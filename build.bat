@echo off
REM build.bat — Compila el binario Windows de ERPGo en bin/ (no subido a git).
REM
REM Por qué un .bat en vez de un Makefile: Windows no trae make por defecto,
REM y este proyecto se desarrolla en Windows. El binario va a bin\ que está en
REM .gitignore, así no se comete el ejecutable.

setlocal

REM El directorio bin/ no se sube a GitHub: está en .gitignore.
if not exist bin mkdir bin

echo Compilando ERPGo...
go build -o bin/ERPGo.exe ./cmd/server
if errorlevel 1 (
    echo.
    echo *** ERROR: la compilacion fallo ***
    exit /b 1
)

echo.
echo Binario creado: bin\ERPGo.exe
echo.
echo Para probarlo:  bin\ERPGo.exe
echo (Ctrl+C para detener)

bin\ERPGo.exe
