# Seguridad del binario y de los secretos

> **Estado: PLANIFICADO. Nada de esto está implementado todavía.**
> Documento de diseño, escrito el 3 de octubre de 2026, para aplicar en una
> etapa posterior al trabajo visual. No hay código de esto en el repositorio.

Este documento explica **qué** se quiere conseguir, **por qué** cada decisión, y
**qué riesgos** trae cada una. La ejecución deja traza en
[BITACORA-SEGURIDAD.md](BITACORA-SEGURIDAD.md).

La regla que gobierna todo lo de aquí: **esto es ofuscación, no criptografía.**
Ninguna de las tres medidas protege un secreto que el usuario pueda leer desde
el disco. Ver [Qué NO protege esto](#qué-no-protege-esto) antes de decidir.

---

## 1. El terreno real (verificado en el código el 3 de octubre de 2026)

Antes de decidir nada conviene tener los hechos, porque cambian el orden de las
medidas:

| Hecho | Dónde | Qué implica |
|---|---|---|
| **No hay repositorio git** | `git rev-parse` falla | garble **no puede** usarse en modo `reverse` sobre git, y no hay historial al que volver si una ofuscación rompe el arranque |
| Compilación con `go build` pelado | [iniciar.bat:50](../iniciar.bat#L50) | hoy no hay ofuscación, ni símbolos, ni flags de ldflags |
| `static/` va **embebido** en el binario | [embed.go:21](../embed.go#L21) | las 115 plantillas y el CSS viajan dentro del `.exe`: quien tenga el binario tiene el sistema visual entero |
| `garble` **ya está instalado** | `~/go/bin/garble` | la medida 2 no requiere instalar nada |
| Binario actual: **44.2 MB** | `erpgow.exe` | margen de sobra para el crecimiento que aporta garble |
| Go **1.26.5**, módulo `erpgow` | [go.mod](../go.mod) | versión moderna: garble la soporta |
| **Las claves de IA están en texto plano** en la BD | `ai_setting.api_key`, `ai_providers.api_key` ([schema.sql:261](../migrations/schema.sql#L261)) | este es el activo más valioso y el más expuesto |
| Contraseñas de usuario en **texto plano** | `hashPassword` devuelve la contraseña tal cual | hallazgo aparte, tratado más abajo |
| No existe `FieldKey` en el código | `grep` no encuentra el símbolo | el nombre de la medida 1 viene de otro proyecto; hay que decidir qué se cifra |

### El hallazgo que hay que decidir antes de empezar

`hashPassword` en [audit_igtf.go:183](../interfaces/web/audit_igtf.go#L183)
devuelve la contraseña **sin transformarla**: en la base se guarda tal cual. Eso
significa que cualquiera con acceso al archivo `.sqlite3` lee las contraseñas de
todos los usuarios. **No es un problema de binario: es un problema de base de
datos, y garble no lo arregla.** Es más grave que las tres medidas que pides
juntas, porque el atacante no necesita desensamblar nada: solo abrir el archivo.

Está fuera del alcance de este documento porque es otro tipo de cambio
(migración de datos existentes, decisión de compatibilidad), pero **debería ir
antes que las tres medidas**. Queda anotado en la bitácora como S-0.

---

## 2. Medida 1 — Cifrar las claves de API con una llave embebida

### Qué se quiere

Que `api_key` no aparezca en claro dentro del `.sqlite3`. Hoy se lee con un
`SELECT` y se ve la clave de OpenAI a simple vista.

### Diseño propuesto

AES-GCM con una llave de 32 bytes, no XOR. La llave se inyecta en tiempo de
compilación:

```
go build -ldflags "-X erpgow/seguridad.borradorClave=<hex de 32 bytes>" ...
```

El campo debe existir aunque no se use (Go exige que `-X` apunte a un
identificador real), y el código **solo lo lee si el valor no es el de
desarrollo**.

### Por qué AES-GCM y no XOR

XOR con llave repetida es reversible por whoever tenga la llave, y la llave
está en el binario. Pero el problema de XOR no es la llave: es que **no
autentica**. Alguien puede alterar un byte del ciphertext y el programa lo
desencripta sin avisar; con GCM el Alter alters *falla*. Para un valor que se
paga y se revoca, detectar la manipulación importa más que la longitud del
ciphertext.

### El riesgo que hay que decir en voz alta

**La llave está dentro del binario.** Quien tenga `erpgow.exe` puede
extraerla con `strings`, y con ella desencriptar la base. Esta medida:

- **sí** evita que alguien con **acceso al archivo de base de datos** (copia de
  seguridad, `.sqlite3` en un USB, un contractor) vea las claves;
- **no** evita que alguien con **el binario** las vea;
- **no** evita que alguien con acceso a la máquina las vea: el programa
  necesita desencriptar en algún momento.

Es un endurecimiento real pero limitado, y por eso está aquí y no presentado
como "los secretos quedan protegidos".

### Alternativa que hay que considerar antes de decidir

Si el objetivo real es que las claves no salgan del servidor, la solución de
verdad es **no guardarlas**: que el usuario las introduzca en cada arranque, o
que se guarden en el almacén de credenciales del sistema operativo
(Credential Manager / keyring). Eso sí protege contra alguien con el binario y
contra alguien con la base.

**Decisión pendiente: ¿se busca "que no se lea en el archivo de base de datos" o
"que no exista en disco en ningún sitio"?** La respuesta cambia la medida
entera. La primera es un cambio de dos horas; la segunda es un cambio de diseño
y toca la pantalla de ajustes de IA.

### Coste

- Migración: leer `api_key` en claro una vez, reescribirlo cifrado. Hay que
  distinguir "ya está cifrado" de "está en claro" con un prefijo de versión
  (`enc:v1:`), o el doble cifrado es silencioso.
- La llave **no se puede rotar sin volver a cifrar**: hay que decidir si existe
  rotación, y si existe, de dónde sale la llave nueva.

---

## 3. Medida 2 — garble sin `-tiny`

### Qué se quiere

Hoy el binario entrega símbolos, nombres de función y rutas de archivo sin
ofuscar: `strings erpgow.exe` devuelve `handleLogin`, `renderStandalone`,
`interfaces/web/fiscal.go`. Eso convierte el trabajo de un atacante en seguir
nombres. garble renombra identificadores y borra metadatos.

### Por qué **sin** `-tiny`

`-tiny` quita mucha información y produce un binario más pequeño y **más
rápido de compilar**, a cambio de romper cosas delicateadas: refreflection,
`go test`, y el soporte de depuración. Como el criterio de este proyecto es
**"usable, elegante, minimalista, robusto y rápido"**, la pregunta correcta no
es "¿ofuscamos más?" sino "¿qué se rompe?".

Sin `-tiny` se conserva la posibilidad de ejecutar los tests del paquete
(compila instrumentado), que es justo lo que permite verificar que la
ofuscación no rompió nada. Con `-tiny` esa verificación desaparece y te quedas
sin red.

### El problema serio: no hay git

**garble en modo normal usa el historial de git para ofuscar de forma
reproducible.** Sin repositorio, hay dos salidas y hay que elegir:

1. **Inicializar git primero** (recomendado). Además de resolver garble, da
   punto de vuelta. Una ofuscación mal aplicada puede dejar el binario sin
   arrancar, y sin git no hay forma de deshacer.
2. Compilar con `-literals=false` y asumir builds no reproducibles: dos
   compilaciones del mismo código dan binarios distintos, lo que dificulta
   verificar si un binario concreto es "elofficial".

### Verificación obligatoria

Un cambio de build que no se ha medido no se aplica. Antes y después:

```bash
go build -o erpgow-plano.exe ./cmd/server    # referencia
garble -o erpgow-obsc.exe ./cmd/server       # ofuscado
```

Y comparar: `go vet`, `go test ./interfaces/web/ -count=1`, `webcheck`, y
arrancar el binario ofuscado y **entrar al sistema**. Este último paso es el
que no se puede saltar: hay al menos un sitio donde el código usa
`reflect` sobre nombres (los `{{.Field}}` de las plantillas y el mapeo de
columnas), y la ofuscación de nombres rompe ese tipo de cosas en silencio.

---

## 4. Medida 3 — Verificación de integridad del binario

### Qué se quiere

Detectar que `erpgow.exe` ha sido **modificado**: parchado, sustituido o
manipulado para saltarse controles.

### Diseño propuesto

Un sello: hash SHA-256 de los bytes del propio ejecutable, firmado con una
llave Ed25519 cuya **pública va embebida** en el binario.

Al arrancar (o con un subcomando `erpgow verificar`), el programa calcula su
propio hash y lo compara con el esperado.

### La debilidad que hay que decir en voz alta

**Si la verificación vive dentro del binario verificado, no sirve.** Quien
parchea el binario parchea también la comprobación, en el mismo minuto. Es la
limitación clásica de la auto-verificación: protege del error y del descuido,
no del atacante con acceso de escritura al ejecutable.

Para que protegiera de verdad habría que que la comprobación **no fuera
modificable**: firma de código de Windows (Authenticode, con el certificado de
tu empresa) validada por Windows antes de ejecutar, o firma en un launcher
separado. Eso ya es otra decisión, con otro coste (certificado, coste anual).

### Lo que sí aporta, y es real

- Detecta **sustitución** del ejecutable por otro distinto, si el lanzador o el
  instalador verifican antes de arrancar.
- Detecta **corrupción** y compilación incompleta: un `.exe` truncado o
  compilado a medias no arranca, y eso se nota antes que en producción.
- Da un **punto de anclaje verificable**: una forma de decir "este binario es
  el que firmamos", que hoy no existe.

### Recomendación de orden

**No es la primera medida que conviene aplicar.** Con garble sin `-tiny` ya se
gana más de los tres, y esta es la que más infraestructura pide. Si el
objetivo es anti-tampering serio, **Authenticode** da más por el mismo esfuerzo.

---

## 5. Qué NO protege esto

Conviene dejarlo escrito para que nadie se lleve una impresión equivocada:

| Amenaza | ¿La para algo de este documento? |
|---|---|
| Alguien con **el `.exe`** extrae la llave y lee la BD | **No.** Las dos primeras son obstrucción, no confidencialidad. |
| Alguien con **acceso a la máquina** lee memoria o la BD mientras corre | **No.** |
| Alguien **parchea** el binario para saltar la verificación | **No**, si la verificación va dentro. |
| Alguien abre el `.sqlite3` y ve **contraseñas de usuario** | **No.** Eso es S-0 y no depende del binario. |
| Alguien entra por la **red** sin pasar el login | **No.** Nada de esto toca la autenticación ni las sesiones. |
| Un `.exe` **sustituido** en la carpeta de instalación | **Sí**, si se verifica antes de arrancar (medida 3). |

La conclusión honesta: **esto protege el binario y, parcialmente, el archivo de
base de datos.** No sustituye a bcrypt para las contraseñas, ni a TLS, ni a
cualquier control de la aplicación. Y para lo que realmente se busca —que el
software no se copie ni se altere— la vía que da más resultado por el mismo
esfuerzo es la licencia con verificación en el servidor, no estas tres.

---

## 6. Orden recomendado

1. **S-0 · bcrypt para las contraseñas** (fuera de este documento, pero antes
   que todo lo demás).
2. **Decidir** la pregunta de la medida 1: ¿"no legible en el archivo" o "no
   existe en disco"?
3. **git** — punto de vuelta, y condición previa para garble.
4. **garble sin `-tiny`**, medido antes y después.
5. **Cifrado de `api_key`**, con migración y prefijo de versión.
6. **Integridad**, y antes de escribir código, mirar Authenticode en serio.

Los pasos 4 y 5 son independientes y pueden ir en cualquier orden. El 6 es el
que más conviene replantear.

---

## Referencias

- [BITACORA-SEGURIDAD.md](BITACORA-SEGURIDAD.md) — registro por entradas.
- [HOJA-DE-RUTA.md](HOJA-DE-RUTA.md) — hoja de ruta única del proyecto.
- [iniciar.bat](../iniciar.bat) — proceso de compilación actual.
- [embed.go](../embed.go) — embebido de `static/` y `templates/`.