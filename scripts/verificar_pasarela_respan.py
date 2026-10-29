#!/usr/bin/env python3
# ============================================================================
# ERPGo — prueba de humo de la pasarela de Respan
#
# QUÉ ES. El cliente de IA del ERP (interfaces/web/ai_respan.go) manda la
# llamada del informe de /ai/analyze a la pasarela alojada de Respan cuando
# RESPAN_API_KEY está definida en el entorno del proceso. Este script hace esa
# MISMA petición —mismo endpoint, mismo cuerpo OpenAI-compatible, misma
# cabecera Authorization— sin levantar el servidor ni tocar la base, para
# comprobar tres cosas antes de dar la integración por buena:
#
#   1. que la clave del proceso es válida y la cuenta está activa;
#   2. que el modelo activo del ERP existe en la pasarela;
#   3. que la llamada queda registrada allí (el id de la respuesta es lo que se
#      busca en el registro de la pasarela en la plataforma).
#
# No sustituye a la prueba real del ERP: es el paso previo, barato, cuando algo
# falla en el informe.
#
# Uso:
#   RESPAN_API_KEY='...' python3 scripts/verificar_pasarela_respan.py
#   RESPAN_API_KEY='...' python3 scripts/verificar_pasarela_respan.py gpt-5.5
#
# Solo biblioteca estándar: no instala nada. Python 3.11 o superior.
#
# La clave se lee del entorno y NO se imprime nunca, tampoco en los errores.
# ============================================================================

import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE_POR_DEFECTO = "https://api.respan.ai/api"
MODELO_POR_DEFECTO = "gpt-5.5"


def main() -> int:
    clave = os.environ.get("RESPAN_API_KEY", "").strip()
    if not clave:
        print(
            "Falta RESPAN_API_KEY en el entorno. La integración queda apagada y el "
            "ERP sigue llamando directo al proveedor.\n"
            "Defínela solo en el servidor (por ejemplo en su .env) y vuelve a intentarlo.",
            file=sys.stderr,
        )
        return 2

    base = os.environ.get("RESPAN_BASE_URL", "").strip().rstrip("/") or BASE_POR_DEFECTO
    endpoint = base + "/chat/completions"
    modelo = sys.argv[1] if len(sys.argv) > 1 else MODELO_POR_DEFECTO

    cuerpo = {
        "model": modelo,
        "messages": [
            {"role": "system", "content": "Eres un analista de negocio. Responde en una línea."},
            {"role": "user", "content": "Di «hola» y nada más."},
        ],
        "temperature": 0.7,
        "max_tokens": 40,
    }
    peticion = urllib.request.Request(
        endpoint,
        data=json.dumps(cuerpo).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer " + clave,
        },
        method="POST",
    )

    print(f"POST {endpoint}  (modelo: {modelo})")
    inicio = time.monotonic()
    try:
        with urllib.request.urlopen(peticion, timeout=60) as respuesta:
            crudo = respuesta.read(1 << 20)
            estado = respuesta.status
    except urllib.error.HTTPError as error:
        crudo = error.read(1 << 20)
        estado = error.code
        print(f"HTTP {estado}: {crudo.decode('utf-8', 'replace')[:400]}", file=sys.stderr)
        return 1
    except urllib.error.URLError as error:
        print(f"No se pudo conectar con la pasarela: {error.reason}", file=sys.stderr)
        return 1
    tardanza = time.monotonic() - inicio

    try:
        datos = json.loads(crudo.decode("utf-8", "replace"))
    except json.JSONDecodeError:
        print(f"La pasarela no devolvió JSON (HTTP {estado}): {crudo[:300]!r}", file=sys.stderr)
        return 1

    opciones = datos.get("choices") or []
    texto = ""
    if opciones:
        texto = (opciones[0].get("message") or {}).get("content") or ""
    if not texto.strip():
        print(f"HTTP {estado} sin contenido del modelo: {json.dumps(datos)[:400]}", file=sys.stderr)
        return 1

    uso = datos.get("usage") or {}
    print(f"HTTP {estado} en {tardanza:.2f} s")
    print(f"Respuesta: {texto.strip()}")
    print(
        "Tokens: prompt={0} completion={1} total={2}".format(
            uso.get("prompt_tokens", 0), uso.get("completion_tokens", 0), uso.get("total_tokens", 0)
        )
    )
    # El id de la respuesta es la pista para encontrar la llamada en el registro
    # de la pasarela (platform.respan.ai → Gateway → logs).
    if datos.get("id"):
        print(f"Id de la llamada: {datos['id']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
