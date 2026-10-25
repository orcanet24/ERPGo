# La capa de IA no funcionaba. Estos son los cinco motivos.

Fecha: 4 de octubre de 2026. Todos ellos aparecieron al hablar con un servidor
de verdad por primera vez. Ninguno lo cazó un test: el código era válido, el JSON
salía bien formado y todo compilaba.

---

## El resumen

La IA del ERP está **conectada y restringida a dos proveedores**: `openai` y
`orc24` (`aiAllowedVendors`, `ai_rules.go:55`). El proveedor por defecto es
`orc24`. Eso no se cambió.

Lo que estaba roto es el **traductor**: el ERP hablaba con el servidor con un
formato que un servidor estricto rechaza, y además descartaba lo que el modelo
le pedía.

| # | Qué pasaba | Síntoma medido |
|---|---|---|
| 1 | Catálogo de herramientas sin el envoltorio `type`/`function` | `HTTP 400: missing field 'type' at line 1 column 43348` |
| 2 | Nombres de función con punto (`db.query`) | `HTTP 400: Function at index 0 has an invalid name: "db.query"` |
| 3 | `required` ausente sale como `null` | `HTTP 400: None is not of type 'array'` |
| 4 | Traducir el nombre a ciegas rompía `describe_schema` | `herramienta 'describe.schema' no está registrada en el catálogo de riesgos` |
| 5 | **Los argumentos de la herramienta se descartaban** | `consulta vacía`, ocho veces seguidas |

El quinto es el que más pesa, y está abajo con todo el detalle.

---

## 1. El catálogo sin envoltorio

El esquema interno de una herramienta es `{name, description, parameters}` y se
mandaba tal cual. La API espera:

```json
{"type":"function","function":{"name":"db_query","description":"...","parameters":{...}}}
```

Un servidor estricto rechaza la petición **entera**, así que el agente no
ejecutaba ni una herramienta. Con `api.openai.com` el síntoma es distinto, lo
cual explica que esto estuviera escondido.

**Arreglo:** `aiToolWire` envuelve cada entrada del catálogo.

---

## 2. Los puntos en el nombre de la herramienta

`db.query`, `report.save_template`, `invoice.post`. La API solo admite letras,
dígitos, guion bajo y guion.

**Arreglo:** al serializar, el primer punto se cambia por guion bajo
(`db.query` → `db_query`); al recibir la llamada se deshace.

---

## 3. `required` como `null`

`aiParameterSchema` ponía `required` a `nil` cuando la herramienta no declaraba
parámetros obligatorios, y un `nil` sale como `null` en el JSON.

**Arreglo:** `required` es siempre un array, vacío si no hay.

---

## 4. El arreglo del 2 rompió una herramienta

Al traducir `_` → `.` a ciegas, `describe_schema` (que ya usaba guion bajo) pasó
a llamarse `describe.schema`, y el guardián la rechazó: *no está registrada en el
catálogo de riesgos*.

**Arreglo:** en vez de una regla sobre el texto, se busca el nombre en el
registro: primero tal cual llega y, si no está, con el primer guion bajo cambiado
por punto. Un registro que se consulta vale más que una regla.

---

## 5. El grave: ninguna herramienta recibía sus parámetros

Este es el que inutilizaba la IA entera, aunque todo lo demás pareciera bien.

La API devuelve los argumentos de una llamada como un objeto **plano**:

```json
{"name":"db_query","arguments":"{\"sql\":\"SELECT ...\",\"limit\":50}"}
```

El ERP los desempaquetaba contra `aiToolPayload`, que tiene la forma
`{name, args:{...}}`. Como no coinciden, **`Name` y `Args` salían vacíos** y
todas las herramientas se ejecutaban sin argumentos.

Medido, en vivo:

```
paso 1 [describe_schema]  {"table":"invoice"}                     ← esta sí
paso 2 [db_query]         {"error":"consulta vacía"}
paso 3 [db_query]         {"error":"consulta vacía"}
... (ocho veces)
paso 10 [model]           (respuesta vacía)
```

O sea: el mapa técnico llegaba al modelo, el guardián autorizaba la llamada, la
herramienta se ejecutaba… y sin argumentos no podía hacer nada. Un `SELECT` sin
texto. El agente no podía consultar la base, escribir un informe, ni crear un
borrador de factura.

**Arreglo:** `aiArgumentosDeHerramienta` lee el objeto plano y acepta también el
formato con `args` anidado, por si algún proveedor envuelve.

---

## Cómo se redirige la IA mientras tanto

El servidor propio todavía no está en pie. Para poder probar se redirige el
**endpoint**, no el proveedor:

```
AI_VENDOR_ORC24_BASE_URL=https://inference.poolside.ai/v1
```

El vendor sigue siendo `orc24`, la clave sigue siendo la de ese vendor, y el
cliente HTTP sigue siendo el mismo. Lo único que se mueve es **dónde** está ese
servidor, y solo quien controla el entorno del proceso puede hacerlo. El día que
el servidor esté en pie, se quita la variable y el producto habla con ORC24 sin
tocar el binario.

**No se añadió ningún proveedor nuevo ni ninguna clave al código.**

---

## Lo que sigue pendiente

1. **El modelo sustituto no cierra el informe.** Con el mapa técnico completo
   (43 KB de esquema) el modelo se pasa de los 12 pasos sin entregar el informe
   final, o expira el tiempo de espera de red. El producto en ambos casos
   responde bien y con un mensaje claro; lo que no converge es el modelo.
   Cuando se pruebe contra ORC24 habrá que ver si ahí sí cierra.
2. **`aiTimeout` puede ser corto** para modelos que razonan mucho.
3. El test vivo que hay ahora (`empresa_completa_ia_test.go`) comprueba la cadena
   completa cuando el agente llega a guardar la plantilla, y **se salta con el
   motivo exacto** cuando no. Un `t.Fatal` en ese punto escondería los fallos de
   verdad del código detrás de lo que el modelo decide hacer cada día.