@echo off
setlocal
cd /d "%~dp0"
if errorlevel 1 exit /b 1

if not exist "bin" mkdir "bin"
if not exist "bin\docker" mkdir "bin\docker"

echo Compilando para Windows...
set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
go build -o "bin\ERPGo.exe" ./cmd/server
if errorlevel 1 exit /b 1

echo Compilando para Linux...
set "GOOS=linux"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
go build -o "bin\ERPGo-linux" ./cmd/server
if errorlevel 1 exit /b 1

echo Listo: bin\ERPGo.exe y bin\ERPGo-linux
