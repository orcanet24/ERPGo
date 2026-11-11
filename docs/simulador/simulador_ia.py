#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Simulador IA de la Guía de Simulación Fiscal (1 año fiscal) del ERPGo.

Lee docs/GUIA_SIMULACION_1_ANO_FISCAL.md, extrae las secciones A-H y ayuda al
guía a ejecutarlas paso a paso contra el ERPGo ya instalado, pidiendo a la IA
(LongCat, API compatible con OpenAI) un resumen acelerado de cada sección.

Regla de diseño: la IA SOLO resume lo que ya está en la guía. Los montos, IDs
y endpoints que muestra salen del texto de la guía; si la IA alucina, el
usuario debe detectarlo cotejando con la sección completa (--paso X).

Uso:
    python simulador_ia.py                 # modo interactivo (recomendado)
    python simulador_ia.py --listar        # ver secciones y avance
    python simulador_ia.py --paso B        # ver la sección B completa
    python simulador_ia.py --resumen B     # pedir a la IA el resumen acelerado
    python simulador_ia.py --hecho B       # marcar la sección B como hecha
    python simulador_ia.py --reiniciar     # borrar el avance guardado
    python simulador_ia.py --sin-ia ...    # forzar modo sin red

Solo usa la biblioteca estándar de Python: no hay nada que instalar.
"""

import argparse
import json
import re
import sys
import urllib.error
import urllib.request
from datetime import datetime
from pathlib import Path

CARPETA = Path(__file__).resolve().parent
RUTA_GUIA_DEFECTO = CARPETA.parent / "GUIA_SIMULACION_1_ANO_FISCAL.md"
RUTA_ENV = CARPETA / ".env"
RUTA_PROGRESO = CARPETA / "progreso.json"

# La guía tiene dos versiones: la vigente por letras (A-H) y una antigua
# numerada (1-5) que repite lo mismo. Este patrón toma SOLO las letras para
# no hacer ejecutar al guía el mismo trabajo dos veces.
PATRON_SECCION = re.compile(r"^## ([A-H])\. (.+)$", re.MULTILINE)
# Fin de cada sección: el siguiente encabezado '## ' (sea de otra letra o de la
# versión antigua numerada) o el título duplicado que abre la versión vieja.
# Por qué no corta en '## ' dentro de bloques de código: no los hay; los '#'
# sueltos dentro de los cercados bash ('# Endpoint: ...') llevan un solo #.
PATRON_FIN = re.compile(r"^## .+$|^# Guía de Simulación", re.MULTILINE)

# Por qué se fija UTF-8: la consola de Windows arranca en cp1252 y cualquier
# carácter de la guía (ó, í, ñ) revienta el print con UnicodeEncodeError.
def configurar_consola():
    for flujo in (sys.stdout, sys.stderr):
        try:
            flujo.reconfigure(encoding="utf-8", errors="replace")
        except (AttributeError, ValueError):
            pass


def cargar_env(ruta_env):
    """Lee pares CLAVE=valor del .env local. Sin python-dotenv a propósito."""
    valores = {}
    if not ruta_env.exists():
        return valores
    for linea in ruta_env.read_text(encoding="utf-8-sig").splitlines():
        linea = linea.strip()
        if not linea or linea.startswith("#") or "=" not in linea:
            continue
        clave, _, valor = linea.partition("=")
        valores[clave.strip()] = valor.strip().strip('"').strip("'")
    return valores


def configuracion_ia(env, timeout):
    cfg = {
        "api_key": env.get("LONGCAT_API_KEY", ""),
        "base_url": env.get("LONGCAT_BASE_URL", "https://api.longcat.ai/openai/v1"),
        "model": env.get("LONGCAT_MODEL", "LongCat-2.0"),
        "timeout": timeout,
    }
    cfg["disponible"] = bool(cfg["api_key"])
    return cfg


# --- Lectura y troceado de la guía -----------------------------------------


class Paso:
    """Una sección de la guía: A, B, C... con su título y su cuerpo."""

    def __init__(self, letra, titulo, cuerpo):
        self.letra = letra
        self.titulo = titulo.strip()
        self.cuerpo = cuerpo.strip()

    @property
    def encabezado(self):
        return f"Sección {self.letra} — {self.titulo}"


def leer_guia(ruta):
    if not ruta.exists():
        raise SystemExit(
            f"No se encontró la guía en {ruta}\n"
            "Pásala con --ruta-guia o restaura el archivo."
        )
    return ruta.read_text(encoding="utf-8")


def extraer_pasos(contenido):
    """Parte la guía en secciones por letra y las devuelve en orden A→H.

    El archivo las trae desordenadas (G, H, F... A) y con la versión antigua
    numerada intercalada tras la sección A; cada sección se recorta en el
    siguiente encabezado de cualquier tipo para no arrastrar ese material viejo.
    """
    cortes = list(PATRON_SECCION.finditer(contenido))
    if not cortes:
        raise SystemExit(
            "La guía no tiene secciones por letra (## A., ## B., ...). "
            "¿Es el archivo correcto?"
        )
    pasos = []
    for corte in cortes:
        fin_siguiente = PATRON_FIN.search(contenido, corte.end())
        fin = fin_siguiente.start() if fin_siguiente else len(contenido)
        pasos.append(Paso(corte.group(1), corte.group(2), contenido[corte.end():fin]))
    return sorted(pasos, key=lambda p: p.letra)


# --- Consulta a la IA --------------------------------------------------------

PROMPT_SISTEMA = (
    "Eres el guía acelerado de la simulación fiscal de 1 año del ERPGo "
    "(carnicería 'La Central'). Recibes UNA sección de la guía oficial. "
    "Devuelve, en español y SIN inventar nada que no esté en el texto:\n"
    "1. Checklist de acciones en el orden exacto.\n"
    "2. Datos clave listos para copiar: endpoints, IDs, montos, JSON de ejemplo.\n"
    "3. Qué debe verse en el sistema para dar la sección por hecha.\n"
    "Máximo 25 líneas. Si algún dato no está en el texto, di 'no figura en la "
    "guía' en lugar de suponerlo."
)


def consultar_ia(paso, cfg):
    """Pide el resumen acelerado de una sección. Devuelve texto o lanza error."""
    if not cfg["disponible"]:
        raise RuntimeError(
            "IA no configurada: falta LONGCAT_API_KEY en docs/simulador/.env "
            "(cópialo de .env.example y completa la clave)."
        )
    url = cfg["base_url"].rstrip("/") + "/chat/completions"
    cuerpo = {
        "model": cfg["model"],
        "temperature": 0.2,
        "messages": [
            {"role": "system", "content": PROMPT_SISTEMA},
            {
                "role": "user",
                "content": f"Sección {paso.letra}. {paso.titulo}\n\n{paso.cuerpo}",
            },
        ],
    }
    peticion = urllib.request.Request(
        url,
        data=json.dumps(cuerpo).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {cfg['api_key']}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(peticion, timeout=cfg["timeout"]) as respuesta:
            datos = json.load(respuesta)
    except urllib.error.HTTPError as error:
        detalle = error.read().decode("utf-8", errors="replace")[:300]
        raise RuntimeError(
            f"La IA respondió HTTP {error.code}: {detalle}"
        ) from error
    except (urllib.error.URLError, TimeoutError) as error:
        raise RuntimeError(f"No se pudo contactar la IA: {error}") from error
    try:
        return datos["choices"][0]["message"]["content"].strip()
    except (KeyError, IndexError, TypeError) as error:
        raise RuntimeError(
            f"Respuesta de la IA con formato inesperado: {json.dumps(datos)[:300]}"
        ) from error


# --- Progreso ----------------------------------------------------------------


def cargar_progreso():
    if RUTA_PROGRESO.exists():
        try:
            datos = json.loads(RUTA_PROGRESO.read_text(encoding="utf-8"))
            if isinstance(datos.get("hechos"), dict):
                return datos
        except (json.JSONDecodeError, OSError):
            pass  # progreso corrupto: se empieza de cero en vez de frenar al guía
    return {"hechos": {}, "iniciado": datetime.now().isoformat(timespec="seconds")}


def guardar_progreso(progreso):
    RUTA_PROGRESO.write_text(
        json.dumps(progreso, ensure_ascii=False, indent=2), encoding="utf-8"
    )


# --- Presentación ------------------------------------------------------------

LINEA = "─" * 70


def mostrar_paso(paso):
    print(LINEA)
    print(paso.encabezado)
    print(LINEA)
    print(paso.cuerpo)
    print(LINEA)


def pedir_resumen(paso, cfg):
    if not cfg["disponible"]:
        print("⚠  IA no configurada (falta LONGCAT_API_KEY en .env). "
              "Se muestra la sección completa en su lugar.\n")
        mostrar_paso(paso)
        return
    print(f"⏳ Consultando a {cfg['model']} sobre la sección {paso.letra}...")
    try:
        print(consultar_ia(paso, cfg))
    except RuntimeError as error:
        print(f"✗ La IA falló: {error}")
        print("  La sección completa sigue disponible con --paso "
              f"{paso.letra} o la opción [m].")
    print(LINEA)


def marcar_hecho(paso, progreso):
    progreso["hechos"][paso.letra] = datetime.now().isoformat(timespec="seconds")
    guardar_progreso(progreso)
    hechos = len(progreso["hechos"])
    print(f"✓ Sección {paso.letra} marcada como hecha ({hechos}/8).")


def mostrar_avance(pasos, progreso):
    print("\nAvance de la simulación:")
    for paso in pasos:
        fecha = progreso["hechos"].get(paso.letra)
        marca = f"✓ hecha {fecha}" if fecha else "pendiente"
        print(f"  [{paso.letra}] {paso.titulo}  →  {marca}")
    print()


# --- Modo interactivo --------------------------------------------------------


def bucle_interactivo(pasos, cfg, progreso):
    print("Simulador IA — Guía de Simulación Fiscal 1 año (ERPGo, La Central)")
    print("Escribe la letra de la sección o elige una opción.\n")
    while True:
        pendientes = [p for p in pasos if p.letra not in progreso["hechos"]]
        if not pendientes:
            print("🎉 Las 8 secciones están hechas. Simulación completa.")
            mostrar_avance(pasos, progreso)
            return
        siguiente = pendientes[0]
        print(f"Siguiente: {siguiente.encabezado}  "
              f"({len(progreso['hechos'])}/8 hechas)")
        try:
            entrada = input(
                "[r] resumen IA · [m] sección completa · [h] marcar hecha · "
                "[v] avance · [q] salir > "
            ).strip().lower()
        except (EOFError, KeyboardInterrupt):
            print("\nSaliendo. El avance queda guardado en progreso.json.")
            return
        elegida = next((p for p in pasos if entrada == p.letra.lower()), None)
        if elegida:
            mostrar_paso(elegida)
        elif entrada == "r":
            pedir_resumen(siguiente, cfg)
        elif entrada == "m":
            mostrar_paso(siguiente)
        elif entrada == "h":
            marcar_hecho(siguiente, progreso)
        elif entrada == "v":
            mostrar_avance(pasos, progreso)
        elif entrada == "q":
            return
        else:
            print("Opción no válida.\n")


# --- Punto de entrada --------------------------------------------------------


def main():
    configurar_consola()
    parser = argparse.ArgumentParser(
        description="Simulador IA de la Guía de Simulación Fiscal 1 año del ERPGo."
    )
    parser.add_argument("--listar", action="store_true",
                        help="lista las secciones y el avance, sin interactuar")
    parser.add_argument("--paso", metavar="LETRA",
                        help="muestra una sección completa")
    parser.add_argument("--resumen", metavar="LETRA",
                        help="pide a la IA el resumen acelerado de una sección")
    parser.add_argument("--hecho", metavar="LETRA",
                        help="marca una sección como hecha")
    parser.add_argument("--reiniciar", action="store_true",
                        help="borra el avance guardado")
    parser.add_argument("--sin-ia", action="store_true",
                        help="no contactar la IA aunque esté configurada")
    parser.add_argument("--ruta-guia", type=Path, default=RUTA_GUIA_DEFECTO,
                        help="ruta alternativa de la guía")
    parser.add_argument("--timeout", type=int, default=90,
                        help="segundos de espera para la respuesta de la IA")
    argumentos = parser.parse_args()

    cfg = configuracion_ia(cargar_env(RUTA_ENV), argumentos.timeout)
    if argumentos.sin_ia:
        cfg["disponible"] = False

    pasos = extraer_pasos(leer_guia(argumentos.ruta_guia))
    progreso = cargar_progreso()

    if argumentos.reiniciar:
        RUTA_PROGRESO.unlink(missing_ok=True)
        progreso = cargar_progreso()
        print("Avance borrado: las 8 secciones vuelven a pendiente.")
        return

    if argumentos.listar:
        mostrar_avance(pasos, progreso)
        return

    if argumentos.paso:
        mostrar_paso(buscar_paso(pasos, argumentos.paso))
        return

    if argumentos.resumen:
        pedir_resumen(buscar_paso(pasos, argumentos.resumen), cfg)
        return

    if argumentos.hecho:
        marcar_hecho(buscar_paso(pasos, argumentos.hecho), progreso)
        return

    bucle_interactivo(pasos, cfg, progreso)


def buscar_paso(pasos, letra):
    paso = next((p for p in pasos if p.letra == letra.strip().upper()), None)
    if paso is None:
        raise SystemExit(f"No existe la sección '{letra}'. Letras válidas: A-H.")
    return paso


if __name__ == "__main__":
    main()
