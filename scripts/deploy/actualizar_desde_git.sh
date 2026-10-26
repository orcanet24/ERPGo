#!/usr/bin/env bash
# ============================================================================
# ERPGo — actualización del SERVIDOR desde GitHub (Linux / Git Bash en Windows)
#
# Uso (en el servidor):
#   ./scripts/deploy/actualizar_desde_git.sh
#
# Qué hace:
#   1) git fetch (barato, no toca tu copia de trabajo)
#   2) Si origin/main == HEAD → nada que hacer, sale en segundos
#   3) git pull --ff-only, compila el binario NUEVO al lado (nunca toca el que
#      corre: si la compilación falla, producción sigue con el actual)
#   4) Reemplaza el binario y reinicia el servicio
#   5) Verifica la salud con /health
#
# Configura con variables de entorno (o edita esta cabecera):
#   ERPDIR    = carpeta del código en el servidor       (/opt/erpgow)
#   SERVICE   = nombre del servicio                    (erpgo)
#   RESETYPE  = systemctl | none | custom              (systemctl)
#   CUSTOMRST = comando de reinicio cuando RESETYPE=custom
# ============================================================================
set -euo pipefail

ERPDIR="${ERPDIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ERPDIR"

git fetch origin main
LOCALED=$(git rev-parse HEAD)
REMOTED=$(git rev-parse origin/main)

if [ "$LOCALED" = "$REMOTED" ]; then
  echo "[deploy] Sin cambios (HEAD=$LOCALED). Nada que hacer."
  exit 0
fi

echo "[deploy] Nuevo commit detectado: ${REMOTED:0:10} (antes ${LOCALED:0:10})"
git pull --ff-only origin main

echo "[deploy] Compilando binario nuevo..."
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o erpgow.binario.nuevo ./cmd/server

# Reemplazo atómico: primero mover, luego reiniciar. Si la compilación falló,
# set -e ya abortó ANTES de tocar el binario en producción.
mv -f erpgow.binario.nuevo erpgow

echo "[deploy] Reiniciando servicio..."
RESETYPE="${RESETYPE:-systemctl}"
case "$RESETYPE" in
  systemctl) systemctl restart "${SERVICE:-erpgo}" ;;
  custom)    eval "$CUSTOMRST" ;;
  none)      echo "[deploy] RESETYPE=none: reinicia manualmente" ;;
esac

echo "[deploy] Verificando salud..."
for i in $(seq 1 20); do
  if curl -fsS http://127.0.0.1:8080/health >/dev/null 2>&1; then
    echo "[deploy] ERP arriba con el commit $(git rev-parse --short HEAD)"
    exit 0
  fi
  sleep 3
done
echo "[deploy] ADVERTENCIA: /health no respondió después de 60 s — revisa el servicio ya." >&2
exit 1
