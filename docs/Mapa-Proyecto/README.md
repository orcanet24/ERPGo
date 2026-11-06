# Mapa del proyecto ERPGo

Mapa estructural del proyecto generado con [Archify](https://github.com/tt-a1i/archify).
Agrupa los 209 archivos de `interfaces/web` por **módulos de negocio** (facturación,
inventario, compras, contabilidad, RRHH, IA, CRM, reportes, add-ons, administración y
plataforma) y deja el resto de capas con sus paquetes reales.

Es la vista **estructural**; el detalle archivo a archivo (con la descripción de cada
función) sigue en el [Diccionario de Scripts](../Diccionario/index.html).

## Archivos

| Archivo | Qué es |
| --- | --- |
| `mapa-proyecto.html` | El mapa interactivo, listo para abrir en el navegador. |
| `candidate.json` | La fuente del diagrama (IR de Archify). No editar a mano. |
| `generar-mapa.js` | Genera `candidate.json` a partir del diccionario. |

## Cómo se regenera

1. Regenera el diccionario si cambió el código (desde `docs/Diccionario`):

   ```bash
   python generar_diccionario.py
   ```

2. Regenera el mapa:

   ```bash
   node docs/Mapa-Proyecto/generar-mapa.js
   ```

3. Validación y entrega con Archify:

   ```bash
   node <ruta-archify>/bin/archify.mjs finalize tree docs/Mapa-Proyecto/candidate.json docs/Mapa-Proyecto/mapa-proyecto.html --quality showcase --json
   ```

El skill de Archify se instala en `~/.agents/skills/archify` (global, para OpenCode).

## Cómo está clasificado

`generar-mapa.js` contiene las reglas de clasificación en dos listas:

- `MODULOS_WEB`: los 11 módulos de negocio de `interfaces/web`.
- `REGLAS_WEB`: prefijos de archivo → módulo, evaluados en orden.

Si aparece `ARCHIVOS SIN CLASIFICAR` al generar, el script termina en error: hay que
añadir la regla correspondiente y volver a generar.

Las capas pequeñas también se agrupan para no ensanchar el mapa:

- `migrations` → esquema base, reconstrucción, ampliaciones z y ajustes finales.
- `scripts` → `deploy/` y `build y verificación`.
- `configs` → `configuración de tenants`.
