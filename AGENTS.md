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
