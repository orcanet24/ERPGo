# Configuración de desarrollo: Addon Signature Key

Fecha: 2026-10-15
Propósito: Firma de addons para desarrollo local (NO usar en producción)

## Clave pública (base64)

```
ERPGO_ADDON_PUBLIC_KEY=cNEAQ1HuD94hW2Dgk+N5vwwRbcFKdd0+RKlt6KZTzS8=
```

## Cómo usar

```bash
# En Windows PowerShell:
$env:ERPGO_ADDON_PUBLIC_KEY="cNEAQ1HuD94hW2Dgk+N5vwwRbcFKdd0+RKlt6KZTzS8="

# Arrancer ERPGo normalmente
go build -ldflags="-s -w" -o erpgow.exe ./cmd/server
.\erpgow.exe
```

## Firmar un addon (desarrollo)

```bash
# El private key está en la herramienta signtool.go — custodiar fuera del repo
go run signtool.go "<privkey-base64>" ./addons/<code>
```

## Generar nuevas claves

```bash
go run genkey.go
```

## Producción

Generar un par de claves offline, custodiar el private key en un HSM o vault,
y pasar solo la clave pública al binario de producción.
