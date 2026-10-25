# Diccionario Vivo del Proyecto (DVP)

> Método para entender **cualquier** proyecto (Go, Rust, PHP, Python…) dejando un
> documento que se mantiene al día con cada cambio. Nace de la práctica real
> (ERPGo, 27/09/2026): un bug de menú → causa raíz → fix → prueba E2E → documentación.

## 1. ¿Cómo se llama?

**El proceso:** `Cartografía de Reconocimiento`.
**El entregable:** `Diccionario Vivo del Proyecto` (DVP) — el "diccionario" son los conceptos
(módulos, entidades, rutas, pantallas, reglas); "vivo" porque se regenera con cada sesión,
no es un documento que se escribe una vez y muere.

Alternativas válidas según el tono del equipo:

| Nombre | Cuándo usarlo |
|---|---|
| **Diccionario Vivo del Proyecto (DVP)** | Equipos que ya conocen el código;empsa el glosario de negocio |
| **Mapa de Conocimiento** | Cuando el objetivo es onboarding de gente nueva |
| **Codex del Proyecto** | Proyectos con muchas decisiones de diseño que explicar |
| **Bitácora Técnica Viva** | Cuando se quierePredicate el historial de hallazgos y fixes |
| **Ingeniería Inversa Didáctica** | Presentaciones/-tutoriales: "te enseño a leer el código ajeno" |

**Regla de nombre útil:** `Sustantivo (qué es) + Adjetivo (cómo se mantiene)`.
Si no puedes mantenerlo, no es un diccionario: es un documento muerto.

## 2. El método en 8 fases (manual, sin IA)

Cada fase produce una evidencia (comando + salida), nunca una suposición.

### Fase 0 — Encuadre (5 min)
- Nombre, propósito en una frase, quién lo usa.
- Anota el **stack exacto** y la **versión** (go.mod / Cargo.toml / composer.json / requirements.txt).
- Fija el criterio de "listo": ¿compila? ¿tests? ¿la app levanta?

### Fase 1 — Inventario (qué hay)
```bash
# estructura (el "mapa" del proyecto)
find . -maxdepth 2 -type d -not -path '*/\.*' -not -path '*/node_modules*'
# archivos clave por lenguaje
git ls-files | sed 's|/[^/]*$||' | sort | uniq -c | sort -rn   # dónde vive el código
```
- Lista directorios y su **responsabilidad única** (una frase cada uno).
- Marca qué es *vendor/legacy* (en ERPGo: `static/js/html/...` = PHP residual).

### Fase 2 — Arranque (cómo vive)
- Localiza el `main`/`index.php`/`__main__.py` y **en este orden**: config → conexión → migraciones → seed → servidor.
- Documenta el ciclo de vida real, no el documentado: lee el código del arranque.
- Anota los **modos/flags** (`--migrate`, `--install`, `--seed`…).

### Fase 3 — Datos (dónde vive el estado)
- Lista de migraciones: nombre + qué agregan + si son idempotentes.
- El mapa entidad→tabla→handler→pantalla (el corazón del diccionario).
- Secretos: dónde se configuran (nunca en el repo).

### Fase 4 — Interfaz (qué ve el usuario)
- Rutas registradas: extrae el mapa ruta→handler→template.
- Cada template: qué plantilla base usa, qué datos recibe, qué JS ejecuta.
- Reglas de permisos: qué condición decide si un usuario ve algo.

### Fase 5 — La prueba de verdad (E2E)
- Arranca la app de verdad, con datos reales o Semilla mínima.
- Recorre los puntos de entrada: login, home, y **un caso de escritura** (crear→leer→borrar).
- Registra código HTTP de cada ruta. Un 500 es un hallazgo, no un detalle.

### Fase 6 — Hallazgos (el diccionario se escribe)
Por cada problema, el bloque de 5 líneas (ver plantilla abajo). **Ordena por impacto**, no por orden de descubrimiento.

### Fase 7 — Mantenimiento (la parte que casi nadie hace)
- Al cerrar cada sesión con cambios: actualiza el DVP (mapa y hallazgos).
- Regla: "si el DVP ya no describe el código, se corrige en el mismo commit".
- Revisión trimestral: ¿los comandos de la Fase 5 siguen funcionando?

## 3. Las 10 trampas que aprendimos en la práctica

1. **Síntoma ≠ causa raíz**: "el menú no sale" era una fila faltante en `sub_module`; el 500 de otra pantalla era una función de template no definida (`{{charge}}` en vez de `{{.charge}}`).
2. **Un error de parseo tumba la página completa**: en templates, un solo token inválido devuelve 500 en toda la vista.
3. **CRLF vs LF es un bug real**: dividir por `";\n"` sobre un archivo con `";\r\n"` ejecuta el archivo entero como una sola sentencia y "no pasa nada".
4. **Los contadores mienten**: "246 applied" no prueba que la fila faltante esté; verifica con una lectura real.
5. **Idempotencia = sobrevive al reintento**: `INSERT IGNORE` + claves primarias explícitas evitan que dos sembradores se pisen.
6. **El orden de arranque importa**: migraciones antes del seed ⇒ clave duplicada ⇒ rollback silencioso y sistema sin admin.
7. **Herramienta ≠ defecto**: falsos fallos de PowerShell que parecen defectos del producto (`$HOME` reservado, sesión destruida por `/logout`).
8. **Envoltorios JSON pesan**: una API con `rowsAffected` (`"status":0`) frente a salida directa de la BD.
9. **Prueba con datos, no con vacío**: un array vacío puede ser template roto *o* tabla vacía; crea un registro temporal y bórralo.
10. **Limpia los artefactos**: cookies de prueba, scripts temporales, volcados con datos sensibles. La instalación debe quedar como la encontraste (+ tus cambios).

## 4. Plantilla del bloque de hallazgo (5 líneas)

```md
## [ESTADO] (fecha) título corto
**Síntoma:** qué se ve (error, pantalla, dato).
**Causa raíz:** por qué ocurre (mecánica, no teoría).
**Fix:** archivo:línea → qué cambió.
**Verificación:** prueba ejecutada + resultado (E2E, no "debería").
## 5. PROMPT MAESTRO (copiar y sustituir `{{}}`)

```
Actúa como Analista de Sistemas Senior. Tu entregable es el «Diccionario Vivo» (DVP) de este proyecto.

CONTEXTO
- Ruta del proyecto: {{RUTA}}
- Stack: {{STACK}} (Go / Rust / PHP / Python / Node…)
- Objetivo de quien pregunta: {{QUÉ QUIERO SABER}}
- Archivo de salida: doc/DICCIONARIO-PROYECTO.md (en la raíz del repo)

REGLAS (no negociables)
1. Evidencia o nada: cada afirmación cita archivo:línea o la salida de un comando que ejecutaste.
   Si no lo verificaste, etiquétalo como [suposición].
2. Lee antes de concluir: el nombre de un archivo no prueba su comportamiento.
3. Nada de "debería funcionar": ejecuta build/tests y prueba la app de verdad
   (login + un caso de escritura crear → leer → borrar).
4. No inventes rutas, tablas, variables ni comandos: si no los encuentras, dilo y sigue.
5. Si la tarea es documentar, NO edites código. Si encuentras un bug, descríbelo y
   propón el fix; no lo apliques sin que te lo pidan.
6. Documento en español; comandos y código en su idioma original.

FASES (muestra un avance corto al cerrar cada una)
F1 Encuadre: propósito en una frase, stack con versiones, criterio de "listo".
F2 Inventario: árbol de directorios + responsabilidad de cada uno (1 frase) + qué es legacy/vendor.
F3 Arranque: main/index.php → config → conexión → migraciones → seed → servidor.
    Documenta el orden REAL que ejecuta el código y los flags/modos disponibles.
F4 Datos: lista de migraciones (qué agregan, si son idempotentes) + mapa
    entidad → tabla → handler → pantalla.
F5 Interfaz: mapa ruta → handler → template → JS + regla de permisos
    (qué condición decide si un usuario ve una opción del menú).
F6 E2E: arranca la app, haz login y recorre las rutas listando el código HTTP de cada una;
    crea, lee y borra un registro de prueba y deja la base como estaba.
F7 Hallazgos: por cada problema usa el bloque de 5 líneas
    (Síntoma / Causa raíz / Fix archivo:línea / Verificación ejecutada / Regla anti-regresión).
F8 Mantenimiento: qué parte del DVP hay que actualizar con cada cambio futuro.

FORMATO DE SALIDA
- Tabla de contenidos.
- Una sección por fase; máximo ~3 pantallas por sección (usa tablas).
- Código solo si es ejecutable; incluye el comando exacto que usaste para verificar.
- Cierra con "Lista de verificación" ([x]/[ ] por fase) y con los comandos para reproducir tus pruebas.
```

## 6. Variantes del prompt (casos de uso diario)

**a) Auditar un bug (el flujo que usamos en ERPGo)**
```
Proyecto en {{RUTA}} (stack {{STACK}}). Falla: {{SINTOMA}} al hacer {{PASOS}}.
1. Reproduce y copia el error literal.
2. Encuentra la CAUSA RAÍZ con evidencia (archivo:línea), no el síntoma.
3. Propón el fix mínimo y espera mi confirmación antes de tocar código.
4. Aplícalo, reconstruye y prueba el flujo completo (no solo el caso que fallaba).
5. Actualiza doc/DICCIONARIO-PROYECTO.md con el bloque de 5 líneas + regla anti-regresión.
6. Limpia los artefactos que creaste (scripts, cookies, volcados).
```

**b) Onboarding de 30 minutos (aprende el proyecto)**
```
Eres mi profesor. Enséñame {{RUTA}} ({{STACK}}) como si nunca hubiera visto este código.
Plan: 1) qué hace el sistema en una frase, 2) cómo arranca, 3) dónde están los módulos,
4) cómo se guardan los datos, 5) cómo se agrega un módulo nuevo de punta a punta
(archivos a tocar + rutas + plantilla + migración), 6) los 5 dangers del proyecto.
Termina con 3 exercises y sus respuestas.
```

**c) Auditar coherencia de datos (sembradores/migraciones que se pisan)**
```
Revisa cómo este proyecto crea sus datos iniciales (migraciones .sql, seeds en código,
fixtures). Comprueba: idempotencia, orden de ejecución en el arranque, colisiones de claves
primarias/únicas entre sembradores y valores coherentes con los esquemas reales.
Ejecuta el seed/migración dos veces y en una base vacía; reporta hallazgos con evidencia.
```

**d) Rutina de cierre de sesión (mantener el diccionario vivo)**
```
Revisa los cambios que hice en esta sesión (git diff / archivos tocados) y actualiza
doc/DICCIONARIO-PROYECTO.md: mapa de módulos, rutas, migraciones y hallazgos.
Marca cada hallazgo con su estado (ABIERTO / RESUELTO + fecha) y deja la lista de
comandos para reproducir la verificación. No inventes: cita lo que ejecutes.
```

## 7. Cómo se aplica a todos tus proyectos (ritual único)

1. Copia `doc/DICCIONARIO-VIVO.md` (o este prompt) a la raíz del proyecto nuevo.
2. Lanza el **PROMPT MAESTRO** una vez → queda el DVP inicial.
3. En cada sesión con IA: cierra con la **variante d)** → el DVP se mantiene vivo.
4. Cuando arrives a un proyecto heredado: usa la **variante b)** para entenderlo en 30 min.

Resultado: un proyecto con "diccionario dinámico" propio, independiente de la IA que lo
tocó, que responde las preguntas de cualquier devs nuevo en 10 minutos.

