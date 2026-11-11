# Simulador IA — Guía de Simulación Fiscal 1 año (ERPGo)

Acompaña al guía durante la ejecución de `docs/GUIA_SIMULACION_1_ANO_FISCAL.md`
(empresa simulada "Carnicería La Central"): extrae las secciones A-H, pide a la
IA (LongCat) un resumen acelerado de cada una y lleva registro del avance.

## Qué usa

- **Python 3.10+** con **solo biblioteca estándar**: no hay `pip install` que hacer.
- **La guía oficial** `../GUIA_SIMULACION_1_ANO_FISCAL.md`. Las secciones por
  letra (A-H) son la versión vigente; la parte numerada (1-5) del archivo es una
  versión antigua duplicada y el simulador la ignora a propósito.
- **La IA** vía variables del `.env` (API compatible con OpenAI):

  | Variable | Valor |
  |---|---|
  | `LONGCAT_API_KEY` | tu clave (se pega solo en `.env`, nunca en el código) |
  | `LONGCAT_BASE_URL` | `https://api.longcat.ai/openai/v1` |
  | `LONGCAT_MODEL` | `LongCat-2.0` |

  Para configurarla: `copy .env.example .env` y completar la clave.

## Cómo se usa

```bat
cd docs\simulador

python simulador_ia.py --listar          :: ver las 8 secciones y el avance
python simulador_ia.py                   :: modo interactivo (recomendado)
python simulador_ia.py --paso B          :: ver la sección B completa
python simulador_ia.py --resumen B       :: resumen acelerado por la IA
python simulador_ia.py --hecho B         :: marcar la sección B como hecha
python simulador_ia.py --reiniciar       :: borrar el avance
python simulador_ia.py --sin-ia          :: modo sin red (solo texto local)
```

El flujo interactivo típico, sección por sección:

1. Muestra la siguiente sección pendiente (A→H).
2. `r` pide a la IA el resumen acelerado (checklist + datos clave + qué debe verse).
3. Ejecutas esa sección contra tu ERPGo ya instalado.
4. `m` vuelve a mostrar la sección completa para cotejar los datos de la IA.
5. `h` marca la sección como hecha y pasa a la siguiente.
6. El avance queda en `progreso.json` (local, no se sube al repositorio).

## Límites que debes conocer

- **La IA solo resume; no inventa tareas.** Su resumen debe cotejarse con la
  sección completa (`m` / `--paso`). Los montos, IDs y endpoints que actúan son
  los de la guía, no los de la IA.
- **La ejecución es manual:** el simulador no toca tu ERPGo ni la base de datos;
  tú cargas los datos por los handlers oficiales (`POST /purchase/save`,
  `POST /invoiceSave`, `POST /payroll/run`, ...), nunca por INSERT directo.
- **La respuesta de la IA depende del servicio externo:** si falla (red, clave,
  cuota), el simulador te lo dice y sigue usable sin IA con `--sin-ia`.

## Seguridad

- `.env` (la clave real) está en `.gitignore` y **no se sube a GitHub**.
- Si la clave llegó a pegarse en un chat o correo, rótala desde el panel del proveedor.
