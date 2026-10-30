# PLAN-MEMO — Comunicación interna, diario de trabajo y tareas para plantillas grandes

## Por qué existe este documento

El dueño del sistema pidió un módulo donde la gente **cargue notas, mensajes
internos y archivos** (fotos, notas de audio), **comparta con grupos o con
personas concretas**, con **chat**, que sirva además de **diario de trabajo**,
y con **calendario de tareas** — tomando como punto de partida el proyecto
abierto [usememos/memos](https://github.com/usememos/memos), pero orientado a
empresas con plantillas grandes de empleados.

Este documento es el contrato de ese módulo: qué se toma de Memos, qué NO se
toma, cómo se modela y en qué fases se construye. No es una lluvia de ideas:
cada fase cierra con pruebas, al estilo de `docs/PLAN-ADDONS.md`.

**Regla que manda sobre todas:** el módulo memo **es un addon**. Vive en
`addons/memo/` con su propio binario, sus templates, sus tablas (`memo_*`) y su
propio ciclo de vida. El núcleo del ERP no se toca para agregarle una función,
ni para arreglar su pantalla: esa es la frontera que `PLAN-ADDONS.md` ya fijó,
y este módulo es el primer caso de negocio real que la pone a prueba.

---

## 1. Qué se toma de usememos/memos (y con qué fuente)

Memos es Go + React, con una API gRPC/HTTP y un store que soporta SQLite,
MySQL y PostgreSQL. Lo que vale la pena copiar es el **modelo**, no el código
(licencias aparte, y su stack no es el nuestro):

| Idea de Memos | Cómo aterriza aquí | Fuente |
|---|---|---|
| La unidad es una **nota fechada con autor y visibilidad** | `memo_notas`: texto, autor, empresa, estado, fijada | [store/memo.go](https://github.com/usememos/memos/blob/main/store/memo.go) |
| **Audiencia separada de la ubicación**: pertenecer a un espacio no comparte nada por sí mismo | `visibilidad` (`privada` / `empresa` / `personas`) + tabla aparte `memo_destinatarios`. Mover una nota de área no cambia quién la lee | [Spaces](https://usememos.com/docs/usage/spaces) |
| **Comentarios como objetos relacionados**, con autor y permiso propios | `memo_comentarios`: filas apuntando a la nota, no texto incrustado | [Reactions & Comments](https://usememos.com/docs/usage/reactions-comments) |
| **Inbox derivado de eventos**, con no leídas y archivadas | `memo_inbox`: mención, comentario, invitación, tarea asignada | [Inbox](https://usememos.com/docs/usage/inbox) |
| **Tags** escritos en el texto (`#proyecto`) | Se conservan como texto dentro de la nota en la fase A; índice propio si engancha | [Tags](https://usememos.com/docs/usage/tags) |
| **Adjuntos** con metadatos separados del binario | `memo_adjuntos` en la base + archivo en la carpeta del addon | [Attachments](https://usememos.com/docs/usage/attachments) |
| **Vistas guardadas** (filtros reutilizables) | Filtros de la línea de tiempo guardados en `memo_vistas` (fase E) | [Views](https://usememos.com/docs/usage/views) |
| **Archivar** retira sin borrar; **fijar** destaca | `estado` = `activo` / `archivado`, columna `fijada` | [Search & Archive](https://usememos.com/docs/usage/search-archive) |

## 2. Qué NO se toma de Memos (porque allí no existe o no sirve aquí)

La investigación de la fuente es clara en tres ausencias que el encargo SÍ
pide; copiar Memos «tal cual» nos dejaría a medias:

1. **No hay chat ni mensajes directos.** Memos tiene comentarios encadenados,
   menciones e inbox; no conversación en tiempo real. El chat se modela
   aparte (`memo_hilos` / `memo_mensajes`) y se enlaza con notas y personas,
   sin pretender que los comentarios lo son.
2. **No hay calendario de tareas con vencimiento.** Las checklist de Memos no
   tienen fecha ni responsable. Las tareas son entidad propia (`memo_tareas`)
   con vencimiento, responsable, prioridad y estado.
3. **No hay grupos de usuarios genéricos** (son Spaces con membresía y roles).
   Aquí el «grupo» nace de lo que la empresa ya tiene: áreas, departamentos y
   roles del ERP, no una estructura paralela que haya que mantener a mano.

Tampoco se copia su frontend React: la pantalla del addon se escribe con el
mismo HTML/CSS/JS nativo que ya usa el ERP, dentro del binario del addon.

---

## 3. Frontera: qué pide el manifiesto y qué le corresponde

El addon `memo` declara (versión fase B):

```json
{
  "id": "memo",
  "routes": ["/memo"],
  "menu":  [{ "label": "Memo", "url": "/memo", "perms": ["memo_ver"], ... }],
  "perms": [
    { "code": "memo_ver",      "label": "Ver el muro y el diario" },
    { "code": "memo_publicar", "label": "Publicar notas y mensajes" },
    { "code": "memo_grupos",   "label": "Administrar grupos de trabajo" },
    { "code": "memo_tareas",   "label": "Crear y asignar tareas" }
  ],
  "db_read":  ["users", "memo_notas", "memo_destinatarios", "memo_grupos",
               "memo_grupo_miembros", "memo_inbox"],
  "db_write": [{ "verb": "nota.crear",    "perm": "memo_publicar" },
               { "verb": "grupo.crear",    "perm": "memo_grupos" },
               { "verb": "grupo.miembros", "perm": "memo_grupos" }],
  "tables_own": ["memo_notas", "memo_destinatarios", "memo_grupos",
                 "memo_grupo_miembros", "memo_inbox"]
}
```

- **Lectura granular** (`db_read`): las tablas propias del módulo van en
  `db_read` para que el addon pueda leer lo que él mismo escribió, y
  `users` —la base de identidad, servida con `X-ERP-Auth-DB`— es la fuente de
  nombres y del selector de gente. Los verbos de escritura son los únicos que
  tocan la base, y bajan por el orquestador.
- **`hr_employee` NO alimenta el selector** (decisión de la fase B, contra la
  letra de §6): la tabla de empleados no tiene enlace a usuarios de login —
  `migrations/schema_z_hr.sql` lo dice con todas las letras: «un empleado no
  es necesariamente un usuario del sistema»—, y el id que se comparte tiene
  que ser el `X-ERP-User` con el que compara la regla de visibilidad. Meter
  ids de empleado en `memo_destinatarios` sería repartir con quien no puede
  entrar. Si algún día aparece el enlace, el selector se cambia en un sitio.
- **Identidad de las notas**: `memo_notas.autor` guarda el `X-ERP-User`
  (`users.user_id`), nunca el nombre de login. La fase A guardaba el nombre
  de login «porque queda más legible» y con eso el autor dejaba de ver sus
  propias notas; la fase B lo corrigió con su prueba.
- **Sin hooks todavía**: la frontera está en fase 2 (sin `hooks_out`). Las
  notificaciones se generan dentro del propio módulo cuando alguien comenta,
  menciona o asigna. Cuando la fase 3 de la frontera llegue, los eventos del
  ERP (`invoice.emitted`, `hr.payroll.posted`) podrán avisar al módulo sin
  tocarlo.
- **Medios (fotos y audio)**: el addon guarda los archivos en SU carpeta
  (`addons/memo/datos/<empresa>/…`) y los sirve por sus propias rutas, con la
  sesión del ERP validada por el proxy. La base solo guarda metadatos. Nada de
  subir binarios a MySQL ni exponer la carpeta estática del ERP.

---

## 4. Modelo de datos (tablas propias, prefijo `memo_`)

Todas llevan `empresa` (aislamiento por compañía) y se crean con
`CREATE TABLE IF NOT EXISTS` en el `schema.sql` del addon.

| Tabla | Papel | Columnas clave |
|---|---|---|
| `memo_notas` | La unidad de contenido (nota, mensaje interno, entrada de diario) | `id`, `empresa`, `autor`, `titulo`, `texto`, `visibilidad` (`privada`/`empresa`/`personas`/`grupo`), `tipo` (`nota`/`diario`/`aviso`), `estado` (`activo`/`archivado`), `fijada`, `creada`, `actualizada` |
| `memo_destinatarios` | Con quién se comparte cuando la visibilidad es `personas` o `grupo` | `nota_id`, `destino_tipo` (`persona`/`grupo`), `destino_id`, `empresa` |
| `memo_comentarios` | Respuestas a una nota, con autor propio | `id`, `nota_id`, `autor`, `texto`, `empresa`, `creada` |
| `memo_grupos` / `memo_grupo_miembros` | Grupos de trabajo (área, proyecto, turno) | `id`, `nombre`, `empresa` / `grupo_id`, `persona_id`, `rol` |
| `memo_adjuntos` | Fotos, documentos y notas de audio | `id`, `nota_id`, `tipo` (`imagen`/`audio`/`documento`), `nombre`, `bytes`, `ruta`, `empresa`, `creada` |
| `memo_hilos` / `memo_mensajes` | El chat (fase D) | `id`, `titulo`, `empresa`, `creado_por` / `hilo_id`, `autor`, `texto`, `creada`, `leida_por` |
| `memo_tareas` | El calendario (fase E) | `id`, `titulo`, `descripcion`, `responsable`, `vencimiento`, `prioridad`, `estado`, `nota_id` (contexto), `empresa` |
| `memo_inbox` | Notificaciones no leídas | `id`, `para`, `tipo`, `ref_id`, `texto`, `leida`, `empresa`, `creada` |
| `memo_vistas` | Filtros guardados de la línea de tiempo (fase E) | `id`, `nombre`, `para`, `filtro`, `empresa` |

**Identidad.** `autor` y `responsable` guardan el `X-ERP-User` que el proxy
entrega; el nombre se resuelve al pintar con `hr_employee` (lectura granular).
Nunca se guarda una copia del usuario que después se desincronice.

---

## 5. Verbos de escritura (los que bajan por el orquestador)

Un verbo por operación de negocio, con su permiso y su prueba:

| Verbo | Permiso | Qué hace |
|---|---|---|
| `nota.crear` | `memo_publicar` | Crea la nota y su reparto (personas o grupos) y avisa en la bandeja de cada destinatario |
| `nota.actualizar` | `memo_publicar` | Editar texto/título; solo el autor |
| `nota.archivar` | `memo_publicar` | Archiva/restaura; no borra |
| `nota.comentar` | `memo_publicar` | Comentario sobre una nota |
| `grupo.crear` | `memo_grupos` | Crea un grupo de trabajo con sus miembros (fase B) |
| `grupo.miembros` | `memo_grupos` | Reescribe los miembros de un grupo: quien sale, deja de ver (fase B) |
| `adjunto.registrar` | `memo_publicar` | Registra los metadatos tras guardar el archivo en la carpeta del addon |
| `mensaje.enviar` | `memo_publicar` | Mensaje de chat en un hilo (fase D) |
| `tarea.crear` / `tarea.actualizar` | `memo_tareas` | Calendario (fase E) |
| `inbox.leer` | `memo_ver` | Marca notificaciones como leídas (fase F) |

---

## 6. Fases de construcción y criterios de cierre

### Fase A — Notas y visibilidad ✅ construida (la primera; todo lo demás se apoya en ella)
- `addons/memo/`: manifiesto, `schema.sql` (`memo_notas`, `memo_destinatarios`) y pantalla con línea de tiempo.
- Verbo `nota.crear` por el orquestador, con visibilidad `privada` / `empresa` / `personas`.
- El muro muestra lo que el usuario puede ver: suyas, las de la empresa y las que le comparten **de otra persona** (nadie ve notas ajenas privadas).
- Menú «Memo» con su permiso y toggle en Configuración → Módulos.
- **Cierre:** pruebas del escáner sobre el bundle real, del registro en `modules`/`menu`, del verbo (validación + escritura por orquestador) y del **aislamiento de visibilidad** (un usuario no ve la nota privada de otro). `go build`, `go vet`, `go test` en verde.

### Fase B — Compartir con grupos y personas ✅ construida
- Grupos de trabajo (`memo_grupos`, `memo_grupo_miembros`) con su verbo
  `grupo.crear` y con `grupo.miembros` para cambiar la plantilla — sin esta
  segunda puerta, «dejar de ser miembro» solo existiría en las pruebas.
- `nota.crear` admite `visibilidad = "grupo"`; el destinatario se guarda como
  `destino_tipo = 'grupo'` y NO se expande a personas, para que la vista se
  decida en cada lectura.
- Al compartir, cada destinatario recibe su entrada en `memo_inbox` (los
  miembros del grupo se expanden en ese momento; el autor no se avisa a sí
  mismo).
- **Cierre (probado en `addons/memo/visibilidad_test.go` y
  `interfaces/web/addon_memo_test.go`):** una nota compartida con un grupo se
  ve para sus miembros y no para el resto; borrar la fila de miembro quita la
  vista sin tocar la nota; un grupo sin miembros no reparte nada y se rechaza
  antes de escribir; los avisos caen en la bandeja de cada persona.
- **Decisión sobre los grupos:** viven en tablas `memo_*` (como dice §4) y no
  se derivan todavía de áreas/departamentos del ERP (§2.3). Esas estructuras
  siguen existiendo; engancharlas a este selector es trabajo de la fase E,
  cuando el diario muestre el área.

### Fase C — Adjuntos: fotos, documentos y notas de audio
- Subida a la carpeta del addon por empresa, con límite de tamaño y tipos admitidos (`image/*`, `audio/*`, `application/pdf`…), nombre aleatorio y extensión validada.
- Descarga por la ruta del addon, autenticada por la sesión del proxy; jamás servida desde una carpeta estática abierta.
- Audio: se sube como archivo y se reproduce en línea (grabación en el navegador); transcripción queda para cuando haya proveedor de IA configurado.
- **Cierre:** una foto y una nota de audio se suben, se listan y se descargan; un archivo con extensión rechazada no entra; borrar la nota no deja huérfanos registrados.

### Fase D — Chat
- Hilos directos y de grupo (`memo_hilos` / `memo_mensajes`), ventana de conversación con sondeo corto (3–5 s) sobre su propia ruta.
- Enlaza mensajes con notas y tareas (un mensaje puede citar una nota).
- **Cierre:** dos sesiones intercambian mensajes; el sondeo no duplica ni reordena; un hilo ajeno no es legible sin pertenecer a él.
- *Abierto:* si el volumen lo exige, la fase 3 de la frontera (hooks + tiempo real) sustituye el sondeo por avance por eventos.

### Fase E — Diario de trabajo y calendario de tareas
- Diario: vista por día con las notas `tipo=diario` del usuario y de su área, y plantillas de entrada (qué hice / qué sigue / bloqueos).
- Calendario mensual sobre `memo_tareas`: vencimiento, responsable, prioridad y estado, con enlace a la nota que le dio contexto.
- Vistas guardadas (`memo_vistas`) para filtrar por persona, área, tag o fecha.
- **Cierre:** una tarea creada desde el calendario aparece en la fecha correcta, cambia de estado y deja rastro; el diario de un día solo muestra ese día.

### Fase F — Notificaciones y avisos
- `memo_inbox` con contador de no leídas en el menú.
- Cuando la frontera llegue a fase 3, `hooks_out` del ERP (`invoice.emitted`, `hr.payroll.posted`, `purchase.registered`) para que una factura o una nómina puedan generar una nota o una tarea sin que el núcleo conozca al módulo.
- **Cierre:** mencionar, comentar y asignar generan entradas de inbox; el contador se limpia al leer.

---

## 7. Qué NO hace este módulo (para que nadie le pida lo que no puede dar)

- **No escribe en las tablas del ERP.** Ni empleados, ni usuarios, ni
  facturas: solo lee lo que `db_read` concede y escribe en `memo_*`.
- **No implementa su propio login.** La sesión, el menú y los candados son
  del ERP; el addon recibe `X-ERP-User` / `X-ERP-Tenant` / `X-ERP-Perms` /
  `X-ERP-DB` ya validados.
- **No pretende ser un correo.** Es contexto de trabajo: lo que se dice aquí
  queda fechado, ubicado y buscable, no empujado a una bandeja.
- **No comparte vistas con el resto del ERP.** Un fallo del módulo deja su
  pantalla fuera de servicio (502 con mensaje), nunca al ERP.

---

## 8. Estado del proceso

- **Fase A ✅** (`addons/memo/`: manifiesto, schema, pantalla y verbo
  `nota.crear` con sus pruebas de visibilidad).
- **Fase B ✅** (grupos con miembros, reparto por grupo, bandeja al compartir,
  y la corrección de `autor` al espacio de id de `X-ERP-User`).
- Fases C a F: pendientes, en el orden de arriba.
- Cada fase se cierra con su commit y su empuje, como manda `AGENTS.md`.
