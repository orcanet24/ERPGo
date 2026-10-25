# Prompt maestro — Manual del Programador para cualquier proyecto

Este prompt es **agnóstico del lenguaje**: descríbelo al agente de IA que
quieras cambiando solo el bloque `{{...}}`. Produce la misma arquitectura que
usa `cmd/docgen` en este repo.

**Cuándo usarlo** al empezar un proyecto nuevo, o cuando uno existente se vuelve
demasiado grande para leerlo entero. No lo uses para documentar una función o un
módulo: para eso ya están el doc-comment y el README.

## El prompt
````text
Necesito que construyas un "Manual del Programador" para este repositorio: una
web navegable, autocontenida, que le permita a alguien nuevo entender la
estructura del sistema sin leer todo el código.

═══════════════════════════════════════════════════════════════════════
VARIABLES — rellena antes de usar el prompt
═══════════════════════════════════════════════════════════════════════
{{LENGUAJE}}     → Go, Rust, Python, TypeScript/Node, PHP, Java, C#, …
{{RAÍZ}}        → raíz del repositorio a documentar
{{SALIDA}}      → p. ej. doc/developer/index.html
{{OVERLAY}}     → p. ej. doc/developer/overlay.json
{{SEGMENTOS}}   → lista de pestañas, ej. "ingesta, dominio, api, persistencia"
{{REGLAS}}      → las reglas inmutables del proyecto, si existen
{{PALABRA_IR}}  → sustituto de "overlay" si tu proyecto usa otro nombre

═══════════════════════════════════════════════════════════════════════
OBJETIVO
═══════════════════════════════════════════════════════════════════════
Un ÚNICO archivo HTML autocontenido (sin red, sin CDN, se abre con doble clic)
con:

1. PESTAÑAS por segmento del proyecto. Cada pestaña tiene:
   - resumen táctico del segmento,
   - árbol de archivos colapsable,
   - listado de archivos con tipo, líneas y nº de símbolos.
2. FICHA POR ARCHIVO al hacer clic: qué hace, cómo opera, con qué se relaciona,
   qué reglas toca, qué importa y quién lo importa, símbolos exportados.
3. FUNCIONES DESPLEGABLES dentro de la ficha: firma, líneas, doc-comment,
   "cómo opera" en pasos, "llama →" y "← llamado por", interfaces que
   implementa, tests que la cubren. Todo enlazado y clicable.
4. MAPA MENTAL (SVG propio, layout determinista) con foco a N saltos y filtros
   de arista.
5. Buscador global, migas de pan, deep-links por URL y al menos un recorrido
   guiado del flujo principal.

═══════════════════════════════════════════════════════════════════════
REGLA DE ORO — NO INVENTES NADA
═══════════════════════════════════════════════════════════════════════
Esta es la restricción más importante del proyecto y no es negociable.

El programa separa DOS fuentes de información:

  A) HECHOS, extraídos automáticamente del código por el parser.
     Paquetes, archivos, funciones, firmas, líneas, imports, símbolos.

  B) NARRATIVA, escrita por una persona en {{OVERLAY}}.
     Por qué existe algo, cómo se opera, con qué se relaciona.

Reglas inviolables:

1. NUNCA afirmes una relación que no puedas demostrar con el parser. Si no
   puedes probar que A llama a B, no digas que lo hace.
2. Cada arista del grafo lleva un nivel de confianza explícito, visible en la
   UI. Vocabulario:
     - "directa"    mismo archivo/paquete, certeza total
     - "import"     resolución por import explícito
     - "estructura" coincidencia de nombre/receptor (puede ser falso positivo)
3. Lo que no se puede resolver se CUENTA como no resuelto y se muestra como
   contador. Ocultar lo que no sabes es peor que mostrarlo.
4. Todo lo que venga del {{PALABRA_IR}} se marca como "curado"; lo automático,
   como "auto".
5. Muestra un contador de cobertura (X de Y archivos con narrativa curada). El
   manual debe ser honesto sobre lo que no cubre.


═══════════════════════════════════════════════════════════════════════
ARQUITECTURA OBLIGATORIA (4 capas, sin atajos)
═══════════════════════════════════════════════════════════════════════
Crea un generador en un subdirectorio tipo `cmd/docgen/` (o `tools/docgen/`,
`scripts/docgen/`), dividido así:

  scan.go      Parser: recorre el repo y extrae hechos.
  graph.go     Resuelve relaciones y sus índices inversos.
  overlay.go   Carga y valida {{OVERLAY}}.
  render.go    Ensambla el HTML final.
  assets/      manual.css, manual.js, manual.html.tmpl (embebidos con
               //go:embed, o equivalentes en tu lenguaje).

REGLAS DE ARQUITECTURA:

  - El generador NO escribe HTML a mano: usa plantillas y un modelo de datos
    intermedio tipado.
  - El modelo de datos es un ÚNICO objeto serializado a JSON que se inyecta en
    el HTML como isla `<script type="application/json">`. El JS del navegador
    solo consume ese JSON.
  - El CSS y el JS se embeben en el propio HTML. Cero peticiones de red.
  - El generador es DETERMINISTA: mismos fuentes → mismos bytes. Sin
    timestamps, sin aleatoriedad, todos los slices ordenados. Esto es lo que
    hace posible la compuerta anti-drift del punto 6.
  - Idioma de los comentarios: inglés. Idioma de los mensajes de error de
    consola: español. Sigue la convención del repo si difiere.

═══════════════════════════════════════════════════════════════════════
FASE 1 — El parser (lo más difícil; empieza aquí)
═══════════════════════════════════════════════════════════════════════
Antes de escribir UI, haz que el scanner funcione y pruébalo.

Tu parser debe extraer, por cada archivo:
  - ruta, paquete/módulo, tipo (código, test, config, migración, doc)
  - líneas totales y líneas de código
  - comentario de cabecera / doc del módulo
  - imports (internos y externos) y sus alias
  - funciones y métodos: firma, líneas inicial/final, exportada/no, doc,
    conteo aproximado de ramas (proxy de complejidad)
  - tipos declarados (struct, interface, enum, trait, class…)
  - los "asserts" de contrato: `var _ Iface = (*T)(nil)` y equivalentes
  - identificadores de reglas del proyecto citados en el archivo (p. ej.
    "REG-01"), y referencias a fases del plan

Y por cada función, los SITIOS DE LLAMADA (CallExpr) con suficiente información
para resolverlos después: nombre del símbolo, identificador izquierdo, número
de selectores de la cadena, y si el receptor es el de la propia función.

IMPORTANTE — resolución de imports: usa el NOMBRE del paquete/módulo tal y
como se escribe en el código, no el último elemento de la ruta. En la mayoría
de lenguajes el nombre del módulo y su ruta no coinciden, y ese detalle es la
causa más común de que el grafo salga vacío.

Criterio de éxito de la Fase 1: un test que verifica que el parser extrae
funciones, tipos e imports de un mini-árbol de ficheros en un directorio
temporal. Nada de depender del repo real en los tests.

═══════════════════════════════════════════════════════════════════════
FASE 2 — El grafo
═══════════════════════════════════════════════════════════════════════
Construye índices en AMBOS sentidos:
  - llama →      y      ← llamado por
  - importa →    y      ← importado por
  - implementa (tipo → interfaz)  y  implementadores
  - test que cubre la función

Cada arista lleva su confianza (ver "REGLA DE ORO"). Resuelve las llamadas de
forma conservadora:

  1. Si es una llamada sin calificar dentro del mismo archivo → "directa".
  2. Si el identificador izquierdo es un import interno y el símbolo existe en
     ese módulo → "import".
  3. Si es un método sobre el receptor conocido → "estructura".
  4. Si el nombre de método es único en el módulo → "estructura" (con aviso).
  5. Selector de profundidad ≥ 2 (`a.b.Método()`) → NO resuelvas a nivel de
     función. La dependencia ya está en el grafo de imports; afirmar la función
═══════════════════════════════════════════════════════════════════════
FASE 3 — Overlay y validación
═══════════════════════════════════════════════════════════════════════
Formato del {{OVERLAY}} (usa JSON, con estos campos):

  {
    "meta":    { "title", "product", "tagline" },
    "segments": [ { "id", "title", "tagline", "summary",
                    "responsibilities": [], "invariants": [],
                    "paths": [],      // prefijos de ruta que cubre
                    "files": [] } ],  // ids exactos para archivos sueltos
    "files":   { "<ruta>": { "role", "how", "relations", "watchouts" } },
    "funcs":   { "<ruta>#<Nombre>": { "what", "how": [], "relations" } },
    "types":   { "<ruta>#<Nombre>": { "how", "relations" } },
    "rules":   [ { "id", "title", "text" } ],
    "tours":   [ { "id", "title", "intro",
                    "steps": [ { "n", "anchor", "title", "note" } ] } ]
  }

REGLA DE PRECEDENCIA:  overlay > doc-comment > nada.

VALIDACIÓN OBLIGATORIA (esto es lo que convierte la doc en fiable):
  - Si el overlay apunta a un archivo, función o tipo que ya NO EXISTE en el
    código → el generador FALLA con error, listando las claves problemáticas.
    La deriva narrativa es un error, no una pérdida silenciosa.
  - Si un segmento no engancha ningún archivo → falla.
  - Si hay segmentos o reglas sin id/título → falla.
  - Si un archivo del repo no está asignado a ningún segmento → falla.
  - Claves que empiezan por "_" están reservadas para notas pendientes y no
    rompen la compuerta (útil para no bloquear el trabajo).

═══════════════════════════════════════════════════════════════════════
FASE 4 — La web
═══════════════════════════════════════════════════════════════════════
Vanilla JS, sin framework ni build step. Tema oscuro coherente con el resto del
proyecto si existe (si no, un tema oscuro neutro accesible).

  - Render de pestañas, árbol y fichas a partir del JSON inyectado.
  - La insignia "curado"/"auto" en cada tarjeta, y el contador de cobertura
    visible en la cabecera.
  - Mapa mental en SVG: layout determinista por anillos (distancia en saltos) y
    orden alfabético dentro de cada anillo. Zoom y pan. Sin librerías.
═══════════════════════════════════════════════════════════════════════
FASE 5 — El {{PALABRA_IR}} curado (la parte humana)
═══════════════════════════════════════════════════════════════════════
El generador no inventa la narrativa. Escribe tú el {{OVERLAY}} con criterio:

  - Los 10-20 archivos más importantes del proyecto, con narrativa real.
  - Las 40-80 funciones que un nuevo desarrollador necesita entender primero,
    explicadas como "qué es" + "cómo opera, paso a paso" + "con qué se relaciona".
  - Los segmentos con su resumen táctico y sus responsabilidades.
  - Las reglas inmutables del proyecto y dónde se aplican.
  - Al menos un recorrido guiado del flujo principal de extremo a extremo.

  REGLA DE ESTILO: escribe como si le explicaras el código a un colega
  inteligente que acaba de llegar. Sin marketing, sin repetir lo obvio que ya
  está en el nombre de la función. Si algo no lo sabes, no lo escribas.

═══════════════════════════════════════════════════════════════════════
FASE 6 — Compuerta anti-deriva (no la saltees)
═══════════════════════════════════════════════════════════════════════
El manual se commitea en el repo. Por tanto puede quedar desfasado. Solución:

  - target `docs`: regenera el HTML.
  - target `docs-check`: regenera a un fichero temporal, compara byte a byte con
    el commiteado y FALLA si difieren. Igual que una compuerta de tests.
  - CI ejecuta `docs-check`. Si alguien cambia el código y no regenera, falla.

  Esto solo es posible si el generador es determinista. Si al regenerar dos veces
  seguidas obtienes ficheros distintos, tienes un bug de determinismo (mapas sin
  ordenar, goroutines, time.Now()): arréglalo antes de seguir.

═══════════════════════════════════════════════════════════════════════
ENTREGABLES
═══════════════════════════════════════════════════════════════════════
  1. El generador, en 4-5 ficheros + assets, con tests propios.
  2. El {{OVERLAY}} curado.
  3. El HTML generado, commiteado.
  4. Un README corto en la carpeta de salida: qué es, cómo se regenera, cómo se
     edita la narrativa.
  5. Los targets docs/docs-check y el paso de CI.

═══════════════════════════════════════════════════════════════════════
ORDEN DE EJECUCIÓN (importante)
═══════════════════════════════════════════════════════════════════════
  1. Parser + su test.               → valida que sabes leer el código.
  2. Grafo + sus tests.              → valida que no inventas relaciones.
  3. Overlay de prueba + validación. → valida que la deriva se detecta.
  4. Render mínimo que funcione.     → valida el circuito completo.
  5. Overlay real, escrito.          → aquí es donde gastas el tiempo.
  6. UI y mapa mental.               → lo visible, lo menos importante.
  7. README, compuertas, CI.

  No inviertas el orden. El riesgo de este proyecto está en los pasos 1-3, no en
  el 6. Si el grafo miente, por muy bonito que sea el mapa, el manual es peor
  que no tener nada: te hace perder el tiempo con seguridad.

═══════════════════════════════════════════════════════════════════════
VERIFICACIÓN ANTES DE DECIR "LISTO"
═══════════════════════════════════════════════════════════════════════
  [ ] El generador compila y su lint pasa con 0 incidencias.
  [ ] Los tests del generador pasan.
  [ ] Regenerar dos veces seguidas produce ficheros IDÉNTICOS byte a byte.
  [ ] El HTML abre con doble clic y no hace ninguna petición de red.
  [ ] Consola del navegador sin errores.
  [ ] Cada pestaña tiene archivos; ningún archivo está sin segmento.
  [ ] El contador de cobertura es visible.
  [ ] `docs-check` falla cuando tocas un fichero de código sin regenerar.
  [ ] El mapa mental se dibuja y las aristas distinguen su confianza.
````

## Cómo adaptarlo a tu lenguaje

| Lenguaje | Parser recomendado | Notas |
|---|---|---|
| **Go** | `go/parser` + `go/ast` (stdlib) | Lo que usa este repo. Cero dependencias. |
| **Python** | `ast` (stdlib) o `astroid` | Ojo: el `import` dentro de funciones rompe el análisis estático simple. |
| **TypeScript/JS** | `ts-morph` (envolvente de la API del compilador) | Da resolución de tipos real: el grafo deja de ser heurístico. |
| **Rust** | `syn` como crate, o `tree-sitter` | `cargo rustdoc --output-format json` da info limpia, pero es **nightly e inestable**: no lo uses como dependencia de CI. |
| **PHP** | `nikic/php-parser` o `tree-sitter-php` | Fíjate en el autoload: los "imports" son `use`. |
| **Java / C#** | `tree-sitter-java` / Roslyn | En C# los `partial` complican el conteo de líneas. |
| **Cualquiera** | `tree-sitter` + el pack de gramáticas | El plan B universal: un solo motor para todos los lenguajes. |

**La columna que importa** es la tercera. En Go, Python y Node el grafo es
*heurístico* (nombre + import), porque el parser no resuelve tipos. En
TypeScript con `ts-morph` puedes resolver de verdad y subir todas las aristas a
"directa": es una diferencia grande de calidad. Si tu lenguaje tiene resolución
de tipos nativa, úsala antes de inventar heurísticas.

## Tres cosas que el prompt no puede darte

1. **La narrativa curada.** El generador extrae hechos; el "por qué" lo escribes
   tú (o la IA, revisando). Es el 70% del esfuerzo real.
2. **El buen criterio de segmentación.** Nadie sabe cómo dividir tu proyecto en
   12 pestañas hasta que las ve. Empieza por lo que un dev nuevo necesita
   aprender en orden.
3. **Mantenerlo vivo.** Un manual sin dueño se pudre en tres meses. La compuerta
   anti-deriva solo garantiza que el *HTML* esté fresco, no que la *narrativa*
   sea cierta.

  - Deep-links: #s/<segmento>/<archivo>/<función>. Al abrir un enlace se
    restaura el estado completo.
  - Atajos de teclado: "/" buscar, "Esc" cerrar, flechas para cambiar de pestaña.
  - Debe funcionar abriendo el fichero con file:// — nada de fetch, nada de CDN.
  - Accesibilidad: HTML semántico, roles ARIA en pestañas y acordeones,
    navegación por teclado, foco visible.


     sería inventar.
  6. Todo lo demás → cuenta como "no resuelto" y muéstralo.

Para implementaciones de interfaces, ordena la evidencia así:
  1. Assert de compilación explícito. Es la más fuerte.
  2. El conjunto de métodos coincide y está en el mismo módulo.
  3. Los módulos se co-importan desde un tercero.
  Si ninguna se cumple, no lo afirmes.



