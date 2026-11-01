# ia_dev — Mapas de módulos

Metodología obligatoria para evitar desalineaciones entre el agente y el código
existente. Norma del proyecto (AGENTS.md, sección "Forma de trabajo").

## Regla

ANTES de tocar cualquier archivo de un módulo, el agente DEBE leer el JSON de
mapa correspondiente en este directorio. El mapa contiene: cada archivo, cada
función con su propósito exacto, rutas HTTP, tablas de BD tocadas y endpoints JS
que lo invocarán. Con el mapa en mano, ningún cambio parte de una suposición.

## Flujo

1. Usuario pide un cambio en un módulo.
2. Agente lee `ia_dev/<modulo>/<modulo>.map.json` completo.
3. Agente identifica las funciones/rutas exactas que el cambio afecta.
4. Agente propone el cambio citando funciones y líneas del mapa.
5. Usuario valida. Agente ejecuta.
6. Si el cambio añade funciones/rutas nuevas, el agente ACTUALIZA el mapa.

## Directorios

- `reportes/modulo_reportes.map.json` — módulo de informes (CRUD, motor, IA,
  PDF, export, reportes clásicos). Cada entrada documenta propósito, tablas y
  endpoints.

## Formato del mapa

Cada archivo es un objeto JSON con:
- `archivo`: ruta relativa.
- `descripcion`: qué hace el archivo en una frase.
- `rutas_http`: endpoints que registra (HandleFunc).
- `funciones`: lista con nombre, línea, propósito, tablas_bd.
- `endpoints_js`: rutas que llama desde el frontend.
- `tablas_bd_principales`: tablas que toca.

## Construcción del mapa

Para generar/regenerar un mapa se puede usar el script existente
`docs/Diccionario/generar_diccionario.py`, que escanea el proyecto entero y
extrae funciones con sus comentarios. El resultado se focaliza luego en el
módulo de interés.
