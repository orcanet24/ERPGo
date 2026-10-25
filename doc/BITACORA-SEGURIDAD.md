# Bitácora de seguridad

Registro de lo que se ha hecho y lo que queda por hacer en el terreno de
seguridad del binario y los secretos. **Una entrada por cambio**, con fecha, qué
se tocó, cómo se comprobó y qué se rompió.

El diseño y las decisiones están en [SEGURIDAD.md](SEGURIDAD.md). Este archivo
es el historial: qué se hizo, cuándo y con qué resultado.

**Regla de esta bitácora**: una entrada no se escribe cuando algo "parece"
funcionar, sino cuando se ha **verificado**. Si no se ha podido comprobar, se
anota como tal, en lugar de darlo por bueno.

---

## Estado

| Id | Medida | Estado |
|---|---|---|
| S-0 | Contraseñas con bcrypt | **Pendiente — hallazgo crítico** |
| S-1 | Cifrar `api_key` con llave embebida | **Pendiente — decisión previa requerida** |
| S-2 | garble sin `-tiny` | **Script escrito y verificado. Pendiente la ejecución final** |
| S-3 | Verificación de integridad del binario | **Script escrito. Pendiente firma real (Ed25519)** |
| S-4 | SQLCipher en la base de datos | **Decidido por el usuario. Documentado, no implementado** |

---

## Threat model acordado (03-10-2026)

Quedó definido por el usuario, y es un objetivo legítimo:

1. **Que nadie abra el `.sqlite3` con un cliente externo** (DB Browser for
   SQLite, un editor, una copia) para leer o bloquear la base.
2. **Que descompilar el binario sea caro.**

Esto cambia el encuadre: **no se busca que sea imposible obtener la clave**,
 sino subir el coste. Es una meta alcanzable y honesta, y así queda escrito.

### Decisiones tomadas

- **D-2**: cifrado de la base de datos, con clave embebida en el binario. Se
  acepta que quien tenga el `.exe` puede extraerla: es el precio de que el
  programa pueda abrir la base sin pedirle nada al usuario.
- **D-3**: garble se aplica **al final del proyecto**, no ahora.
- Compilación **tradicional** para desarrollo y pruebas; la protegida solo para
  el binario que se entrega.
- **D-1 y D-4 siguen abiertas** (qué clave exacta, y si la integridad será
  auto-verificación o Authenticode).

### Aviso que sigue siendo cierto, y que no se ha retirado

Que la llave quede dentro del binario sube el coste de extraerla; no lo
impide. Se dejó escrito en la bitácora a propósito, para que dentro de unos
meses nadie lea "cifrado" y asuma "irrompible". El bcrypt de S-0 sigue siendo
la pieza que más falta hace y la más barata.

---

## 2026-10-03 — Reconocimiento

**Qué se hizo**: lectura del terreno antes de proponer nada.

**Hallazgos verificados**:

- `grep` no encuentra `FieldKey` en el código. El nombre viene de otro
  proyecto; hay que decidir qué es la clave que se cifra. Lo que existe y tiene
  valor real son `ai_setting.api_key` y `ai_providers.api_key`.
- **No hay repositorio git.** `git rev-parse` falla. Esto condiciona S-2.
- Compilación con `go build -o erpgow.exe ./cmd/server`, sin flags
  ([iniciar.bat:50](../iniciar.bat#L50)). Hoy no hay ofuscación de ningún tipo.
- `garble` **ya está instalado** en `~/go/bin/garble`. S-2 no necesita instalar.
- Binario: 44.2 MB. Go 1.26.5, módulo `erpgow`.
- `static/` va embebido ([embed.go:21](../embed.go#L21)): las 115 plantillas y
  todo el CSS viajan dentro del `.exe`.
- No hay cifrado en ninguna parte: `grep` de `aes|gcm|encrypt` sobre el código Go
  no devuelve ninguna implementación.

**Hallazgo aparte, no estaba en el encargo (S-0)**:

`hashPassword` en [audit_igtf.go:183](../interfaces/web/audit_igtf.go#L183)
devuelve la contraseña **sin transformarla**. Las contraseñas de usuario están en
texto plano en el `.sqlite3`.

Esto es más grave que las tres medidas pedidas juntas, porque no requiere
desensamblar nada: abrir el archivo basta. Y **garble no lo arregla**, porque no
tiene nada que ver con el binario.

**Pendiente de decisión**: ¿el alcance de este trabajo incluye las contraseñas,
o es solo el binario? Registrado como S-0 porque depende de la respuesta.

**Cómo se comprobó**: lectura directa de los ficheros citados, `grep` sobre el
código, y `strings`/`ls` sobre el binario. Sin cambios en el código.

---

## 2026-10-03 — S-2 y S-3: script de compilación protegida (escrito y probado)

**Qué se hizo**: [compilar-lanzamiento.bat](../compilar-lanzamiento.bat), que
produce el binario de entrega con garble + XOR + sello de integridad, dejando
intacto el de desarrollo.

**Verificado de verdad, no supuesto**:

| Comprobación | Resultado |
|---|---|
| `garble` con el Go del sistema (1.26.5) | **FALLA**: pide Go 1.27+ |
| `garble` con el Go 1.27.1 | funciona |
| Compilación ofuscada del proyecto real | **OK**, 3 min 12 s, 42,9 MB |
| Nombres `handleLogin` / `authMiddleware` / `attachScope` / `tenantDBMust` | **0 apariciones** (antes, 1 cada uno) |
| El binario ofuscado **arranca** y sirve `/login` | **HTTP 200** |
| CSS y assets servidos desde el binario ofuscado | **OK**, 54 954 bytes |

**Dos fallos reales encontrados al probar** (por eso se probó en vez de
entregar):

1. **Un `.bat` escrito con saltos LF no funciona.** cmd.exe espera CRLF; con LF
   cada línea se parte y salen errores como `"olor" no se reconoce`. El fichero
   tiene que guardarse con CRLF.
2. **La detección de Go fallaba** porque buscaba con `where`, que no lo
   encuentra si go1.27 no está en el PATH de la sesión. Ahora mira primero en
   `%USERPROFILE%\go\bin`.

**La trampa que habría fallado el día del lanzamiento**: `garble` v0.18.0 exige
**Go 1.27 o superior**, y el Go por defecto de esta máquina es el **1.26.5**.
Compilando con la instrucción habitual, garble aborta. Está escrito en la
cabecera del `.bat` y en este registro, porque no es un error evidente.

**Pendiente de ejecutar**: la compilación completa del `.bat` **no llegó a
terminar** en esta máquina. Falló por falta de memoria (`errno=1455`) al
lanzar el compilador, no por el script — que se comportó como debía: avisó del
error y **no tocó** `erpgow.exe`. Se le añadió `-p=2` para limitar los
compiladores en paralelo; falta comprobarlo en un equipo con memoria
suficiente.

**Veredicto**: *funciona, pendiente de ejecutar de principio a fin en el PC de
lanzamiento.* Lo que queda sin comprobar es el paso 6 (sello SHA-256) y el
arranque del binario producido por el script, no el de la compilación manual.

---

## Decisiones que faltan (bloquean la ejecución)

Ninguna de las tres medidas se puede aplicar tal cual sin responder esto antes:

### D-1 · ¿Qué se cifra en S-1?

`FieldKey` no existe. Las opciones son `ai_setting.api_key`,
`ai_providers.api_key`, o ambas. **Recomendación: las dos**, están en la misma
tabla y el mismo código.

### D-2 · ¿"No legible en el archivo de BD" o "no existe en disco"?

La primera es un endurecimiento: cifrar con AES-GCM y una llave embebida. La
segunda —que el usuario teclee la clave en cada arranque, o que se guarde en el
almacén de credenciales del sistema— es la que de verdad impide que alguien con
el binario la vea.

La llave embebida **no protege de quien tenga el `.exe`**. La diferencia entre
las dos opciones es la diferencia entre obstrucción y protección real.

### D-3 · ¿Se crea el repositorio git?

Sin git: garble compila de forma no reproducible y no hay punto de vuelta si la
ofuscación deja el binario sin arrancar. **Recomendación: sí**, y antes de
S-2.

### D-4 · ¿S-3 es auto-verificación o firma de código?

La auto-verificación dentro del binario no protege: quien parchea el binario
parchea también la comprobación. Si el objetivo es anti-tampering de verdad, la
vía es **Authenticode** (firmado por Windows antes de ejecutar), que es otra
decisión con otro coste.

---

## 2026-10-03 — Orquestador de escrituras (la puerta y el rastro obligatorio)

**Medida**: S-2 (rastro obligatorio) y S-3 (orden determinista)

**Qué se hizo**

`internal/orchestrator/`: la puerta única de escritura. Un journal en disco con
cabecera propia (correlativo, versión, flags) y registros cifrados con AES-GCM
usando la FieldKey; un escritor único que aplica en orden de correlativo; y el
INSERT en `audit_log` **dentro de la misma transacción** que el dato, de modo que
si el rastro no se puede escribir la escritura no entra.

Más la conexión al servidor (`interfaces/web/orchestrator.go`): un orquestador
por empresa, con su journal en `data/escrituras/<empresa>.journal`, levantado
desde `NewServer` y parado desde `Close`.

**Cómo se comprobó**

13 pruebas del paquete y 5 de la conexión, todas en verde. Además, arrancar el
binario de verdad: el log dice `orquestador de escrituras activo para default` y
el journal aparece con 32 bytes (solo la cabecera).

Lo que demostraron, por nombre:

- `TestColaLlenaNoPierdeTrabajo` — 50 escrituras con capacidad 2: ninguna se
  pierde, y las 50 dejan rastro.
- `TestElProductorSeFrenaCuandoLaColaLlena` — con la cola llena, `Submit` se
  queda esperando en vez de devolver un error.
- `TestSinRastroNoEntraElDato` — se rompe `audit_log` a propósito: el documento
  **no** entra y la escritura sigue en el journal para reintentarse.
- `TestLaMismaClaveEscribeUnaSolaVez` — 4 entregas con la misma clave: 1 fila.
- `TestLimpiarNoReiniciaElCorrelativo` — el journal queda limpio y el correlativo
  sigue; comprobado releyendo la cabecera del archivo, no la memoria.
- `TestRecuperaLoQueQuedoAMedias` — cierre duro y rearrange: 5 de 5 recuperadas
  y en orden.
- `TestElJournalVaCifrado` — la marca sensible **no** aparece en el archivo, y
  sin la FieldKey el journal ni siquiera se abre.
- `TestElHashCoincideConElDelProyecto` — el hash de la cadena es idéntico al de
  `computeAuditHash`. Si divergieran, la verificación declararía manipulado un
  bloque que está bien.

**Qué se rompió** (y cómo se corrigió)

1. **El aplicador usaba la base, no la transacción.** Se le pasaba `*sql.DB` y
   abría su propia conexión, así que el rastro y el dato se confirmaban por
   separado: exactamente el fallo que el paquete viene a cerrar. Se cambió la
   firma a `func(ctx, *sql.Tx, Entrada)`. Merece la pena, porque era el error
   más importante de todos y no lo daba ningún test.

2. **Identificadores validados demasiado tarde.** Si una tabla inválida pasaba la
   validación, se escribía en el journal, el escritor la recogía, fallaba cinco
   veces con reintentos y solo entonces se reportaba: veinte segundos y cinco
   vueltas por un error que se detecta mirando 64 bytes. Se movió la validación a
   `validar()`, en memoria. El test bajó de 20,78 s a 1,20 s.

3. **El `Aplicador` no llevaba la empresa y el journal tampoco.** Al recuperar
   trabajo a medias ya no hay un `http.Request` del que sacarlos, así que ahora
   viajan en el registro.

**Veredicto**: funciona. *Pendiente y dicho sin rodeos*: los 97 INSERT que hay
hoy en los handlers **siguen fuera de la puerta**. La puerta existe y está
probada; migrar los handlers uno a uno es el siguiente paso, y el test que lo
fuerza (que falle si aparece un INSERT fuera de la puerta en módulos de negocio)
es lo que obliga al resto.

---

## 2026-10-03 — Contraseñas, FieldKey conectada y respaldos

**Medida**: S-0 (corregido), S-2 (cifrado en disco)

**Qué se hizo**

1. **Contraseñas.** Creado `interfaces/web/password.go` con un solo punto para
   verificar y otro para crear hashes; borrada la función muerta `hashPassword`
   que prometía bcrypt y devolvía la contraseña en claro; comparación en tiempo
   constante en vez de `==`; y rehash automático a bcrypt en el momento del
   login para las filas heredadas.

2. **FieldKey conectada.** El orquestador recibía `""` como clave, así que el
   journal se creaba **en claro** aunque el binario trajera la FieldKey. Ahora
   `cmd/server/main.go` la entrega con `web.SetFieldKey()` antes de levantar el
   servidor, y el log dice si el journal va cifrado o no.

3. **Respaldos.** `respaldar.bat`: ZIP fechado con SHA-256 y listado, excluyendo
   solo `backup/` y las carpetas de compilación de Go.

4. **Limpieza.** Eliminada una base de datos de 1,1 MB que los tests creaban en
   `interfaces/web/empresas/`, **arreglando la causa** (no el síntoma) y el
   `loadtest.exe` compilado de 9 MB.

**Cómo se comprobó**

- 7 pruebas nuevas de contraseñas, todas en verde.
- Servidor real arrancado: login por HTTP con contraseña en texto plano → 302, y
  la fila pasó a `$2a$10$…`. La misma contraseña siguió entrando; una incorrecta
  fue rechazada (200).
- Binario con FieldKey: el log dice `journal CIFRADO`.
- `respaldar.bat` ejecutado: 44,5 MB, 2062 entradas, SHA-256 calculado, y
  comprobado que el ZIP **no** contiene `backup/` y **sí** las bases.
- Suite completa en verde y la carpeta `interfaces/web/empresas/` ya no aparece.

**Qué se rompió** (y cómo se corrigió)

- **El hallazgo S-0 estaba equivocado.** Decía que las contraseñas estaban en
  texto plano; la realidad es que el login ya usaba bcrypt y solo una fila
  heredada lo estaba. Documentado como corrección, no escondido.
- **La limpieza de `TempDir` fallaba** al final del test, con un error que no
  señalaba su causa: el handle de la base por empresa quedaba abierto. Se
  cerró el `Server` en el `Cleanup`.
- **El listado del ZIP fallaba** por un escape de `$` sobrante al pasar el
  comando por bash a PowerShell.

**Veredicto**: funciona. *Pendiente*: las **API keys de IA** (`ai_setting`,
`ai_providers`) siguen en texto plano en la base. Eso no lo arregla ni garble ni
el orquestador: es lo que queda por cifrar.

---

## Plantilla de entrada

Copiar para cada cambio:

```
## AAAA-MM-DD — Título corto

**Medida**: S-x

**Qué se hizo**:
**Cómo se comprobó**:
**Qué se rompió** (y cómo se corrigió):
**Veredicto**: fonctionne / no funcionó / parcial — *qué queda sin comprobar*
```