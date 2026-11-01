# Instrucciones del proyecto ERPGo

Estas reglas son de obligado cumplimiento para cualquier agente que trabaje en
este repositorio. No son consejos: son el procedimiento.

## Cierre de cada adecuación: probar, commitear y empujar

Siempre que termine una adecuación (funcionalidad nueva, corrección,
refactorización):

1. **Verificar antes de cerrar**, desde la raíz del proyecto:
   ```bash
   go build ./...
   go vet ./interfaces/web
   go test ./...
   go run ./internal/webcheck -dir templates   # si se tocó una plantilla
   ```
   Todo debe quedar en verde. Un test que falla se arregla — nunca se salta, no
   se desactiva y no se «aplaza» al commit.

2. **Commitear con mensaje descriptivo en español**, al estilo del historial
   (sin prefijos genéricos): una línea que diga QUÉ cambió y POR QUÉ, y si el
   cambio es grande, cuerpo con el detalle. Ejemplos reales del repo:
   «Se elimina assets/, la copia heredada que ya no embebía ni leía nadie»,
   «Cuentas por Cobrar y Cuentas por Pagar pasan al primer nivel del menú».

3. **Empujar a GitHub** (`git push`) en la misma sesión, sin esperar a que lo
   pidan: el trabajo verde que no está en GitHub no existe para el resto.
   Si el push es rechazado por divergencia, NO se fuerza: se integra el historial
   y se vuelve a intentar.

## Forma de trabajo: sin parches, transparencia total, honestidad absoluta

Estas obligaciones rigen CADA modificación de CUALQUIER archivo (código Go,
plantilla, migración, script, documentación). No son aspiraciones: son la forma
de trabajo.

- **No parches.** Un archivo se corrige de una vez, completo: si hay que tocar
  varios puntos de él, se reescribe entero o se edita con vista de todo su
  contenido. Quedan PROHIBIDAS las ediciones a ciegas que generen fragmentos
  huérfanos, código duplicado, funciones a medio remendar o restos de intentos
  anteriores. Antes de decir que está listo: `gofmt` limpio del archivo tocado
  y una relectura de lo que quedó. Si se detecta un resto propio, se nombra
  como tal y se corrige en el mismo acto — nunca «luego lo limpio».
- **Transparencia total.** Cada cierre se reporta con lo que se VERIFICÓ y lo
  que NO se pudo verificar, por separado. Prohibito decir «todo perfecto» o
  «todo en verde» cuando hay fallos: se enumeran (aunque sean del entorno o
  preexistentes, con la prueba de que lo son) y no se oculta ninguno. Los
  avisos y límites de lo implementado se dicen antes de que los descubra el
  usuario.
- **Honestidad absoluta.** Lo que el usuario no pidió no se inventa; lo que se
  rompió no se disfraza; la culpa de un error propio se reconoce en el mensaje
  del paso donde ocurre, no en un párrafo final diplomático. Cada afirmación
  técnica («funciona», «está probado», «el problema es otro») va respaldada por
  comando, prueba o evidencia ejecutada en esa sesión — no por memoria ni por
  intención.
- **Eficiencia total: causa raíz, no síntoma.** Antes de tocar código, se
  identifica el defecto real (con evidencia), se corrige una vez y se cierra
  con una prueba que lo detala, para que no vuelva. Queda prohibido apilar
  trabazones hacia arriba para esconder un síntoma, o «arreglar» con parche
  condicional lo que hay que definir bien de raíz. Si el defecto revela una
  norma del proyecto que no existe, se propone aquí.
- **Cada archivo tocado, responsable único.** El que modifica un archivo
  responde por el estado completo en que lo deja, no por «mi parte». Conforme
  se toca un archivo y queda en rojo, se arregla ANTES del commit — el commit
  nunca es el camino para esconder un estado a medio hacer.

## Git: proteger el trabajo sin commitear

- **NUNCA usar `git reset --hard`, `git checkout -- .` ni `git restore` sobre
  archivos modificados.** Esos comandos destruyen cambios sin commitear y no se
  pueden recuperar (ya pasó aquí y costó reconstruir a mano toda una fase).
- Para deshacer un commit recién creado sin perder nada: `git reset --soft HEAD~1`.
- Un commit de prueba vacío no se crea «para verificar» nada; si se crea, se
  retira con `--soft`.

## Estilo del código

- Español en comentarios, mensajes al usuario y nombres de pruebas: el sistema
  se escribe como habla su dueño.
- Cada pantalla, candado o regla de negocio lleva su prueba junto a él
  (patrón `*_test.go` de `interfaces/web`). Si algo se rompe en producción, la
  prueba es lo que lo delata.
- Comentarios que expliquen el POR QUÉ (la regla que protege), no el QUÉ.
