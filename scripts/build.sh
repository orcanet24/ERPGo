#!/usr/bin/env bash
# ============================================================================
# ERPGo — Compilación para LINUX
# Uso:
#   ./scripts/build.sh           → compilación corriente (vet + tests + build)
#   ./scripts/build.sh seguro    → compilación OFUSCADA (garble, anti-reversa)
#
# Qué hace "seguro":
#   - garble reemplaza nombres de funciones/variables por aleatorios,
#     ofusca literales (strings) y borra las rutas de fuente del binario.
#   - -trimpath: ninguna ruta de tu máquina queda embebida.
#   - -ldflags "-s -w": sin tabla de símbolos ni debug info.
#   - -buildmode=pie: binario independiente de posición (PIE), mucho más
#     difícil de parchar en caliente.
#   - CGO desactivado para reproducibilidad y portabilidad.
#
#   Nunca es 100% — el que recibe el binario, recibe el programa — pero esta
#   combinación elimina lo fácil: nombres claros, strings legibles y paths.
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")/.."

SEG=${1:-normal}
BIN="erpgow"
LDFLAGS="-s -w"

echo "==> go vet ./cmd/server"
go vet ./cmd/server

echo "==> go test ./internal/..."
go test ./internal/...

build_normal() {
  CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$BIN" ./cmd/server
}

# intenta PIE primero; si el host no lo soporta, cae al standard
build_pie() {
  CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -buildmode=pie -o "$BIN" ./cmd/server
}

if [ "$SEG" = "seguro" ]; then
  if ! command -v garble >/dev/null 2>&1; then
    echo "==> Falta garble. Instálalo una vez con:"
    echo "    go install mvdan.cc/garble@latest"
    echo "(garble pide una versión de Go igual o más nueva que la del módulo;"
    echo " si falla 'Go version ... is too old', actualiza Go y reintenta)"
    exit 1
  fi
  echo "==> Compilación OFUSCADA con garble..."  # cada build elige semilla nueva; garble la imprime y la dejamos en el log
  if ! CGO_ENABLED=0 garble -tiny -literals -seed=random build \
      -trimpath -ldflags "$LDFLAGS" -buildmode=pie -o "$BIN" ./cmd/server; then
    echo "(PIE no soportado con garble en este host; reintentando estándar ofuscado)"
    if ! CGO_ENABLED=0 garble -tiny -literals -seed=random build \
        -trimpath -ldflags "$LDFLAGS" -o "$BIN" ./cmd/server; then
      echo "ERROR: la compilación ofuscada falló. NO se sobreescribe el binario anterior." >&2
      exit 1
    fi
  fi
else
  echo "==> Compilación estándar..."
  if ! build_pie; then
    echo "(PIE no soportado aquí; compilando estándar)"
    build_normal
  fi
fi

echo ""
echo "==> Listo: $(ls -lh "$BIN" | awk '{print $5, $9}')"
if [ "$SEG" = "seguro" ]; then
  echo "Recuerda guardar la semilla de garble si compararás builds."
fi
