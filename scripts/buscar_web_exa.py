#!/usr/bin/env python3
# ============================================================================
# ERPGo — búsqueda web con Exa (SDK oficial exa-py)
#
# QUÉ ES. Módulo de búsqueda web para las herramientas y agentes del ERP: dada
# una consulta devuelve las páginas más relevantes de la web, cada una con sus
# `highlights` (los fragmentos del texto que responden a la consulta). Sirve
# para que un informe o un agente pueda contrastar datos actuales —normativa,
# tasas, proveedores— sin que nadie copie y pegue resultados a mano.
#
# POR QUÉ EXA Y NO UNA BÚSQUEDA PROPIA. El ERP es Go y Exa no publica SDK para
# Go: sus SDK oficiales son exa-js (Node) y exa-py (Python). Este módulo usa el
# oficial de Python, que es el lenguaje en que ya están las herramientas de
# operación del proyecto (scripts/), en vez de reimplementar el protocolo.
#
# CÓMO SE USA.
#   Como consulta desde la línea de órdenes:
#     EXA_API_KEY='...' .venv/Scripts/python.exe scripts/buscar_web_exa.py "consulta" --limite 5
#   Desde otro script o agente:
#     from buscar_web_exa import buscar_web
#     paginas = buscar_web("consulta", limite=5)
#
# La clave se lee del entorno del proceso (EXA_API_KEY) y NO se escribe, no se
# imprime y no viaja en los resultados ni en los mensajes de error. Sin clave,
# la búsqueda queda apagada con un aviso claro: no hay resultado inventado.
#
# Dependencia: exa-py (ver scripts/requerimientos-exa.txt). Python 3.9+.
# ============================================================================

import argparse
import json
import os
import sys
from typing import Any

VARIABLE_CLAVE = "EXA_API_KEY"
LIMITE_POR_DEFECTO = 5
LIMITE_MAXIMO = 100  # tope público del endpoint /search


class ExaSinClave(RuntimeError):
    """No hay EXA_API_KEY en el entorno: la búsqueda web está apagada."""


def clave_exa() -> str:
    """Devuelve la clave del proceso, o cadena vacía si no está configurada."""
    return os.environ.get(VARIABLE_CLAVE, "").strip()


def buscar_web(consulta: str, limite: int = LIMITE_POR_DEFECTO) -> list[dict[str, Any]]:
    """Busca `consulta` en la web con Exa y devuelve las páginas con highlights.

    Cada resultado es un diccionario con `titulo`, `url`, `publicado`, `autor` y
    `fragmentos` (la lista de highlights). La lista viene vacía si Exa no
    encontró nada; los errores de la API se propagan tal cual para que quien
    llama decida qué hacer (un informe sin red no debe fingir datos).
    """
    consulta = (consulta or "").strip()
    if not consulta:
        raise ValueError("La consulta de búsqueda no puede estar vacía.")

    clave = clave_exa()
    if not clave:
        raise ExaSinClave(
            f"Falta {VARIABLE_CLAVE} en el entorno. Define la clave en el "
            "servidor (por ejemplo en su .env) y vuelve a intentarlo."
        )

    # Importación diferida: el módulo se puede importar sin el SDK instalado
    # (por ejemplo desde una prueba que solo comprueba el aviso de clave).
    from exa_py import Exa

    limite = max(1, min(int(limite), LIMITE_MAXIMO))
    exa = Exa(api_key=clave)
    respuesta = exa.search(
        consulta,
        type="auto",
        num_results=limite,
        contents={"highlights": True},
    )

    paginas: list[dict[str, Any]] = []
    for resultado in getattr(respuesta, "results", None) or []:
        paginas.append(
            {
                "titulo": getattr(resultado, "title", None) or "",
                "url": getattr(resultado, "url", None) or "",
                "publicado": (
                    getattr(resultado, "published_date", None)
                    or getattr(resultado, "publishedDate", None)
                ),
                "autor": getattr(resultado, "author", None),
                "fragmentos": list(getattr(resultado, "highlights", None) or []),
            }
        )
    return paginas


def _main(argv: list[str]) -> int:
    analizador = argparse.ArgumentParser(
        description="Busca en la web con Exa y devuelve las páginas con highlights.",
    )
    analizador.add_argument("consulta", nargs="+", help="consulta de búsqueda")
    analizador.add_argument(
        "--limite",
        type=int,
        default=LIMITE_POR_DEFECTO,
        help=f"número de páginas a devolver (1-{LIMITE_MAXIMO}, por defecto "
        f"{LIMITE_POR_DEFECTO})",
    )
    opciones = analizador.parse_args(argv[1:])
    consulta = " ".join(opciones.consulta)

    try:
        paginas = buscar_web(consulta, limite=opciones.limite)
    except ExaSinClave as error:
        print(str(error), file=sys.stderr)
        return 2
    except ValueError as error:
        print(str(error), file=sys.stderr)
        return 2
    except Exception as error:  # se informa sin exponer la clave ni la cabecera
        print(f"Exa no respondió: {type(error).__name__}: {error}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            {"consulta": consulta, "resultados": paginas},
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(_main(sys.argv))
