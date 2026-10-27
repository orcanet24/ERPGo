# Plan de corrección — Gobernanza del módulo IA (MCP interno)

**Fecha:** 27 de octubre de 2026
**Origen:** informe de revisión previo sobre la IA y el MCP interno.
**Estado:** verificado en el código y **corregido** — ver §5 «Estado de ejecución».
**Ámbito:** `interfaces/web/ai_*.go`, `migrations/schema_z_ai_*.sql`, `interfaces/web/worker.go`.

---

## 0. Veredicto de la corroboración

| # | Hueco | ¿Confirmado? | Evidencia |
|---|-------|--------------|-----------|
| 1 | «Requiere humano» es un campo muerto | **Sí** | `ai_governance.go:77` (campo), `:141` (se marca), `ai_tools.go:509` (declarado sin exportar y sin tag), `:571-572` (se escribe); ningún lector |
| 2 | Las herramientas no consultan los permisos del rol | **Sí** | cero llamadas a `hasPermission`/`canAccessModule` en `ai_*.go`; `aiGuardAction` sólo mira catálogo, tablas prohibidas y `OperatorOnly` |
| 3 | No hay llave de idempotencia | **Sí, con tres agravantes** | sin `external_ref` en migraciones ni en el camino de IA; el ticker de 5 min reencola una tarea `daily` hasta ~12 veces en su hora; `MaxAttempts: 3` reintenta y re-ejecuta; `POST /ai/tasks/run/{id}` no deduplica |

Los tres son reales tal y como se describieron. El hueco 3 es el más caro y resultó peor de lo reportado (ver §1.3).

---

## 1. Corroboración detallada

### 1.1 El campo `needsHuman` no llega a ninguna parte

El camino completo, verificado:

- `ai_governance.go:77` declara `NeedsHuman bool` en `GovernanceDecision`.
- `ai_governance.go:141` lo enciende: `NeedsHuman: tool.Risk == RiskCritical`, en la rama `Allowed`. `payment.record`, `voucher.create`, `invoice.anull` y las cuatro de nómina/pago declaradas son `RiskCritical` (`ai_tools.go:210, 224, 325, 360`, entre otras).
- `ai_tools.go:506-509` recibe la marca en el journal:

  ```go
  needsHuman bool   // sin exportar y SIN etiqueta JSON
  ```

- `ai_tools.go:571-572` la escribe: `if verdict.NeedsHuman && journal != nil { journal.needsHuman = true }`.
- `aiExecuteTask` (`ai_tasks.go`) persiste el journal con `json.Marshal(journal)`. `encoding/json` **ignora los campos no exportados**, así que `needsHuman` nunca llega al JSON; y aunque llegara, la tabla no tiene dónde guardarlo: `ai_task_run` es `id, task_id, agent_id, status, started_at, finished_at, steps, final_report, undo_journal` (`migrations/schema_z_ai_agents.sql`), sin columna de aprobación.

Búsqueda de las seis apariciones del identificador: dos en `ai_governance.go`, dos en `ai_tools.go`, y ninguna consulta SQL, ninguna ruta, ninguna plantilla. `templates/ai_task_run.html` muestra estado, informe, bitácora y el botón «Revertir»: no hay nada de aprobación.

**Consecuencia exacta:** un agente con operador activo ejecuta `payment.record` o `voucher.create`, el hecho queda auditado (`s.aiAudit` sí funciona) y la marca de «esto lo debía aprobar un humano» se pierde. Lo único que existe es `POST /ai/tasks/runs/{id}/undo`, que **deshace**, no **aprueba** — y además sólo acepta corridas con `status = 'done'`.

Contradicción documental que hay que resolver antes de codificar: `doc/bitacora-agentes-ia.md:21` registra como decisión «Operador **sin aprobación humana por acción**», mientras que `doc/# ERPGo → Plataforma SQLite Multi-Empresa.md:156` dice «Crítico | Nómina, pagos, anulaciones | **Siempre aprobación humana**». El código implementa la primera y conserva el andamiaje de la segunda.

### 1.2 El guardián no mira los permisos del rol

`aiGuardAction` (`ai_governance.go`) es el único punto por el que pasa toda acción de la IA, y comprueba sólo tres cosas: que la herramienta esté en el catálogo, que no toque una tabla prohibida y que `OperatorOnly` tenga operador activo. Cero coincidencias de `hasPermission` o `canAccessModule` en ningún `ai_*.go` (verificado con `rg -g ai_*.go`).

Lo único derivado del rol es `aiDeptDelAgente` (`ai_agents.go:88,103`), y se usa para **acotar lecturas**: el scope de `db.query` (`ai_tools.go`), `describe_schema` y el prompt. El departamento no limita escrituras.

Los documentos prometen otra cosa:

- `doc/ERPGo  Agentes IA.md:46` → «Fase 2 — Modo operador (requiere `operator_enabled=1` en el agente **Y permisos CRUD del rol**)».
- `doc/bitacora-agentes-ia.md:103` → idéntico.
- `ai_agents.go:5` → «sus permisos son exactamente los de su rol».

Hoy eso es cierto para la web y falso para las herramientas.

**Nota de implementación (obstáculo real, no teórico):** `hasPermission` (`render.go:466`) y `roleDBs` (`permissions.go:302`) reciben un `*http.Request`. El runner no tiene petición: corre desde el ticker y desde la cola, con una sesión sintética (`&SessionData{UserID: agent.UserID, ...}`). Por eso la comprobación **no** se resuelve llamando a `hasPermission`: hace falta una variante sin `*http.Request`. La pieza reutilizable ya existe: `canAccessModule(dbs []*sql.DB, userID, level string, departmentID int, action string)` (`permissions.go:251`), que además ya resuelve el detalle de consultar **las dos bases** (empresa y control), que es donde vive la matriz de permisos.

### 1.3 No hay llave de idempotencia — y hay tres multiplicadores

No existe `external_ref` ni equivalente: cero coincidencias en `migrations/` y ningún parámetro de referencia externa en las herramientas de escritura. `invoice.create_draft` numera con `SELECT IFNULL(MAX(invoice),999)+1` dentro de la transacción, y la tabla `invoice` (`migrations/schema.sql:656`) **no declara ninguna clave única**: ni para el número, ni para una referencia documental.

Tres mecanismos re-ejecutan el mismo trabajo, y el más grave no está en el informe original:

1. **El planificador duplica tareas `daily` y `weekly`.** `aiScheduleDue` (`ai_tasks.go`) resuelve `daily` como `nowT.Hour() == parseHour(...)`: eso es verdadero en **las doce vueltas** del ticker de 5 minutos dentro de esa hora. El único freno es `enqueueDueAITasksOf` (`worker.go:169`), que omite si la última corrida de los últimos 6 minutos sigue en `running`. Una tarea que termina rápido en estado `done` se vuelve a encolar cada 5 minutos hasta que cambia la hora: hasta ~12 corridas por día para una tarea «diaria».
2. **La cola reintenta y re-ejecuta.** `queue.Config{MaxAttempts: 3}` (`worker.go`, `internal/queue/queue.go:84`) sobre `runQueuedAITask` → `aiExecuteTask`, que inserta una corrida **nueva** con journal **nuevo**. Un fallo a mitad de trabajo reintenta las escrituras ya hechas.
3. **«Ejecutar ahora» no deduplica.** `POST /ai/tasks/run/{id}` sólo verifica que la tarea esté habilitada y que el runner global esté libre (`aiRunnerRunning`), que es un cerrojo de concurrencia, no de repetición.

Agravante de reversión: `aiUndoLastRun` sólo acepta `status = 'done'`, así que las escrituras parciales de una corrida `failed` **no se pueden deshacer desde la UI**. Y si tres corridas duplicaron documentos, revertir una sola compensa una parte: el journal es una pila de compensaciones, no un libro de deduplicación.

**El patrón correcto ya existe en el repo y hay que copiarlo, no inventarlo:** `pos_sale_idempotency` con `UNIQUE(terminal_id, offline_id)`, reclamado **dentro** de la transacción (`internal/pos/grpc_server.go:590-594, 694`; `migrations/schema_z_correlativos.sql:43`) y la «clave de idempotencia» del orquestador con clave única (`internal/orchestrator/orchestrator.go:60, 768`).

---

## 2. Decisiones que hay que tomar antes de tocar código

1. **¿Marcar o bloquear?** El hueco 1 admite dos arreglos y no son el mismo producto:
   - **(a) Marca real:** persistir `needs_human`, mostrarlo en la corrida, listarlo y notificarlo. Sigue ejecutando; el humano se entera después. Coherente con «operador sin aprobación humana por acción» y con el diseño de compensación.
   - **(b) Compuerta real:** la herramienta crítica **no** se ejecuta; queda en `pending_approval` y un humano la aprueba. Coherente con el otro documento, e incompatible con la autonomía tal como está vendida hoy.
   - **Recomendación:** (a) ahora, como arreglo del defecto reportado, y (b) sólo para el subconjunto **irreversible** (`invoice.anull`, `payroll.post` y cualquier anulación futura) cuando se implementen. Pagos y vouchers tienen compensación exacta en el journal, que es justamente la razón por la que la autonomía se consideró aceptable.
2. **¿Qué pasa con los agentes que hoy operan sin el permiso?** Al aplicar el hueco 2, un agente con `operator_enabled=1` y un rol sin permiso de escritura pasará de ejecutar a ser rechazado. Hay que auditar antes (§3, paso 0) y decidir si el endurecimiento entra de golpe o con una ventana de aviso.
3. **¿Quién aprueba?** Para (b) hace falta un rol/permiso de aprobación (`ai_approvals`), no sólo `admin`, y la aprobación debe quedar en la cadena de auditoría con el usuario real.

---

## 3. Plan de acción

Ordenado por riesgo y coste. Cada fase es desplegable sola y deja el sistema en un estado coherente.

### Fase 0 — Auditoría previa (sin cambios de código)

Antes de endurecer nada, saber a quién afecta:

- Lista de agentes con `operator_enabled = 1` cuyo rol no tiene el permiso de escritura del directorio correspondiente (la misma consulta de `hasPermission`, pero por `user_id` del agente).
- Lista de tareas con `schedule_type IN ('daily','weekly')` y cuántas corridas `done` tienen por día: confirma la magnitud de la duplicación del ticker.
- Lista de directorios válidos a usar como permiso por herramienta: `SELECT directory FROM sub_module WHERE status = 1`. **No inventar** los nombres; se derivan de aquí, igual que hace `routeDirectory`.

### Fase 1 — El permiso viaja en el mismo registro que el riesgo (hueco 2)

El registro `aiToolDef` (`ai_tools.go`) es la única lista de herramientas, y ese diseño ya evitó un bug real (el comentario de `ai_governance.go:44` lo documenta: `payment.record` se clasificó tarde y el guardián la bloqueó). El permiso debe vivir ahí, no en un mapa paralelo.

1. Añadir a `aiToolDef` un campo `Permission` (p. ej. `struct{ Directory, Action string }`), declarado **junto** a `Risk` y `OperatorOnly` en cada entrada del registro. Herramientas de escritura declaradas sin permiso → el guardián las rechaza por omisión (mismo criterio que «no declarada = rechazada»).
2. Añadir el helper sin `*http.Request`: `hasPermissionFor(sess *SessionData, userID, directory, action string) bool`, que resuelva el nivel con `loadUserRole`, la lista de bases con el equivalente de `roleDBs` para sesión y delegue en `canAccessModule` (`permissions.go:251`). Debe cubrir el caso admin/master y la regla «empleado sólo lectura» que ya aplica la web.
3. Comprobar el permiso en `aiGuardAction` (o inmediatamente después, en `aiDispatchTool`), devolviendo `aiToolBlocked` con `Rule: "permisos"` — no un `aiToolError`: el bloqueo por regla no se reintenta, y el agente debe distinguirlo.
4. Derivar el catálogo anunciado del permiso real: `aiCatalogForPhase` ya filtra por `OperatorOnly`; debe filtrar además por permiso, para no ofrecer al modelo herramientas que le serán rechazadas y gastar pasos y tokens.
5. **Aceptación:**
   - Test: agente con rol sin `invoice.create` → `invoice.create_draft` devuelve `blocked` con `rule: "permisos"` y no crea fila en `invoice`.
   - Test: el mismo agente con el permiso concedido → la herramienta ejecuta.
   - Test: la lista del catálogo anunciado no incluye las herramientas sin permiso.
   - Test: un agente `admin`/`master` no queda bloqueado (paridad con `hasPermission`).
   - Prueba de paridad explícita contra la web: la misma acción por la interfaz y por la herramienta dan el mismo veredicto para el mismo rol.

### Fase 2 — La llave de idempotencia (hueco 3)

Sub-fases independientes; la 2.1 es la de mayor retorno inmediato.

**2.1 El planificador deja de repetir (el arreglo más barato y el más visible).**
- En `aiScheduleDue`/`enqueueDueAITasksOf`, calcular una **clave de ranura** por período en lugar de comparar la hora del reloj: p. ej. `task_id + "|" + tipo + "|" + (AAAA-MM-DD para daily / AAAA-Www para weekly / el minuto del tick para cron5)`.
- Índice único en `ai_task_run` sobre esa clave (columna nueva `slot_key`), con `INSERT IGNORE`/conflicto tratado como «ya corrió en este período». El ticker puede seguir siendo de 5 minutos sin duplicar nada.
- Aceptación: una tarea `daily` encolada 12 veces en su hora produce **una** corrida. Test que simula las doce vueltas del tick con reloj fijo.

**2.2 La llave de idempotencia de las herramientas de escritura.**
- Columna `idem_key varchar(120) NOT NULL DEFAULT ''` + `UNIQUE (idem_key)` (o `UNIQUE(tool, idem_key)`) en un libro de ejecución nuevo, p. ej. `ai_tool_execution`, con `result_json`, `document_ref`, `run_id`, `created_at`.
- La llave se reclama **dentro de la misma transacción** que el efecto, exactamente como `processSale` con `pos_sale_idempotency`: si el `INSERT` choca con la clave única, se devuelve el resultado ya guardado con `duplicate: true` y **no** se vuelve a escribir.
- Origen de la llave, en orden de preferencia: (a) `external_ref` explícito del documento de origen; (b) derivada por el servidor de `run_scope + tool + argumentos normalizados` (hash), para que un reintento de la cola produzca la misma llave. El modelo **nunca** inventa la llave: la deriva el servidor, o viene del documento.
- Añadir `external_ref` al vocabulario de escritura: columna en `invoice` (y en `purchase` cuando exista herramienta de compras) con índice no único, y exigirla en las herramientas que nacen de un documento externo («cargar compras desde un listado»). Sin referencia externa no hay deduplicación posible entre corridas distintas: eso es un requisito de la herramienta, no un detalle.
- Reintentos de la cola: pasar la llave en el `aiTaskPayload` (`worker.go`) para que el intento 2 reuse la del intento 1; y no crear una corrida nueva por intento (`aiExecuteTask` ya inserta `running`; el reintento debe reutilizar la corrida o marcarla `retried`).
- Aceptación:
  - Test: dos corridas con la misma llave → **un** documento, y la segunda responde `duplicate`.
  - Test: fallo simulado a mitad de la corrida + reintento de la cola → un solo documento.
  - Test: la misma tarea disparada dos veces con argumentos idénticos y sin `external_ref` → un solo documento (llave derivada).
  - Test: `INSERT` de la llave y efecto en la misma transacción → si el efecto falla, la llave no queda reclamada (nada de «reclamada sin efecto», que es un no-op silencioso).

**2.3 `undo` y el libro de idempotencia.**
- `aiUndoLastRun` debe consultar el libro: una llave ya revertida no se revierte dos veces, y al revertir debe liberar o marcar sus llaves.
- Permitir revertir corridas `failed` (hoy sólo `done`), que es cuando más falta hace: son las que dejan escrituras parciales.
- Aceptación: revertir una corrida fallida deja el estado contable consistente; revertir dos veces la misma corrida es un no-op con aviso, no una doble compensación.

### Fase 3 — La marca «requiere humano» deja de evaporarse (hueco 1)

Según la decisión de §2.1.

1. Persistirla: exportar el campo con etiqueta (`NeedsHuman bool \`json:"needs_human"\``) y añadir `needs_human TINYINT DEFAULT 0` a `ai_task_run`, escribiéndola en el `UPDATE` de cierre de `aiExecuteTask` (junto a `status`, `steps`, `final_report`, `undo_journal`).
2. Mostrarla: distintivo en `templates/ai_task_run.html` y en el listado de tareas/corridas (`aiTaskListData` ya devuelve `last_status`; añadir `needs_human`).
3. Si se elige (b) compuerta: columna `approval_status ('none'|'pending'|'approved'|'rejected')`, `approved_by`, `approved_at`, y `POST /ai/tasks/runs/{id}/approve` con permiso propio, auditado en la cadena. La herramienta crítica, con compuerta activa, **no ejecuta**: registra la intención en el libro de idempotencia en estado `pending` y responde al modelo que quedó pendiente (el modelo no debe reintentar en bucle).
4. **Aceptación:**
   - Test: una corrida que llama a `payment.record` queda con `needs_human = 1` **leído de la BD**, no de una variable en memoria.
   - Test de regresión del defecto exacto: `json.Marshal` del journal + `json.Unmarshal` posterior devuelve la marca (hoy la pierde).
   - Con compuerta: test de que la herramienta no escribió nada y que la aprobación posterior sí escribe, una sola vez (encadenado con la llave de idempotencia de la Fase 2).

### Fase 4 — Cierre documental

- Corregir la contradicción entre `doc/bitacora-agentes-ia.md:21` y `doc/# ERPGo → Plataforma SQLite Multi-Empresa.md:156`.
- Actualizar `doc/bitacora-agentes-ia.md` §4 con el permiso por herramienta y la llave de idempotencia (hoy documenta «reversión por journal» como si fuera suficiente).
- Registrar el resultado en la bitácora, con la convención del repo: qué se cambió, por qué, y qué prueba lo respalda.

---

## 4. Verificación (comandos del repo)

```
go build ./...
go vet ./...
go test ./...
```

Además, por convención del proyecto (`doc/PATRON-UNICO-DE-DESARROLLO.md:327`): **la prueba de idempotencia de la migración no es opcional** — aplicar la migración dos veces debe dejar el mismo esquema y no fallar. Las columnas nuevas se añaden con la forma idempotente que ya usan las migraciones de IA (`ADD COLUMN IF NOT EXISTS` / `CREATE TABLE IF NOT EXISTS`), y el archivo debe respetar la **ordenación por nombre** (`schema_z_*`) que documenta `migrations/schema_z_ai_agents.sql`.

Tests nuevos previstos (paquete `interfaces/web`, junto a los existentes):

| Test | Qué fija |
|------|----------|
| Permiso denegado por herramienta | El rol manda en el despacho, no sólo en la web |
| Permiso concedido / admin | Paridad con `hasPermission` |
| Catálogo anunciado filtrado | No se ofrecen herramientas sin permiso |
| `needs_human` persistido y releído | Regresión del hueco 1 |
| Ranura de programación | Una tarea `daily` = una corrida por período |
| Llave de idempotencia duplicada | Un documento, segunda respuesta `duplicate` |
| Reintento de cola | No duplica escrituras |
| `undo` de corrida fallida | No deja residuos ni compensa dos veces |

---

## 5. Estado de ejecución (27-10-2026)

Implementado y verificado con `go build ./...`, `go vet ./...` y `go test ./...` (suite
completa en verde, 17 pruebas nuevas del módulo IA).

| Hueco | Qué se hizo | Dónde |
|-------|-------------|-------|
| 1 | `NeedsHuman` exportado con etiqueta `needs_human`, columna en `ai_task_run`, distintivo en la corrida y en el listado, y **acto de revisión** (`POST /ai/tasks/runs/{id}/review`, admin/master, auditado) | `ai_tools.go`, `ai_tasks.go`, `ai_run_slot.go`, `templates/ai_task_run.html`, `templates/ai_task_list.html` |
| 2 | `aiToolDef.Permission` (directorio + acción) junto al riesgo; guardián que lo exige; catálogo que no anuncia lo que el guardián rechazará; resolución del rol sin `*http.Request` | `ai_permissions.go`, `ai_governance.go`, `ai_tools.go` |
| 3 | Ranura de período por tarea (tabla `ai_run_slot`), libro de idempotencia `ai_tool_execution`, `external_ref` en las herramientas de escritura, claves liberadas al revertir, y reversión de corridas fallidas | `ai_idempotency.go`, `ai_run_slot.go`, `ai_tasks.go`, `worker.go`, `migrations/schema_z_ai_governance.sql` |

**Decisiones tomadas** (las de §2, resueltas):

1. Se implementó la **marca real con revisión** (opción a), no la compuerta que bloquea la
ejecución. Motivo: los pagos y vouchers tienen compensación exacta en el journal, que es lo
que hacía aceptable la autonomía, y la bitácora de agentes ya la registraba como decisión
(«operador sin aprobación humana por acción»). La compuerta queda pendiente para las
acciones **irreversibles** cuando se implementen (`invoice.anull`, `payroll.post`).
2. Los permisos se exigen sólo a las herramientas que **escriben**. Las lecturas ya están
acotadas por el departamento del rol, y doblarlas por directorio habría roto el diseñador
de informes, que usa al agente por defecto para leer.
3. La idempotencia es fuerte con `external_ref` (una sola vez, aunque pasen los días) y por
ámbito de corrida cuando no hay referencia (cubre el reintento de la cola y un segundo
«Ejecutar ahora» del mismo día). No se añadió columna `external_ref` a `invoice`: el rastro
vive en el libro y en `audit_log`, y tocar el INSERT de factura en cinco llamadores por una
columna que sólo usaría la IA no se justificaba.

**Pendiente, y se dice en voz alta:**

- **Fase 0 (auditoría previa)** no se ejecutó: hay que listar los agentes con
`operator_enabled = 1` cuyo rol no tenga el permiso de escritura correspondiente, porque
pasarán de ejecutar a recibir un bloqueo explícito. El mensaje de bloqueo ya dice qué
permiso falta y dónde se concede, así que el arreglo es una pantalla de Roles.
- La migración se validó por compilación, por orden de archivo y por ejecución de sus
sentencias contra SQLite en memoria (pruebas del libro y de la ranura). **No se aplicó
contra MySQL/MariaDB**: en esta máquina no hay motor de base de datos.
- `ai_run_slot` no se limpia por antigüedad. Cada período deja una fila; el volumen es una
por tarea y período, pero conviene una poda en la limpieza periódica que ya existe.

## 6. Lo que este plan NO incluye

- No añade herramientas nuevas (compras, nómina) ni implementa `payroll.run`, `payment.create`, `invoice.anull`, etc. Las declaradas sin `Run` siguen declaradas: el permiso y el riesgo se les asigna igual, para que nazcan protegidas.
- No cambia el modelo de proveedores ni el guardián SQL de lectura, que están fuera del informe.
- No toca el POS: `pos_sale_idempotency` es el patrón de referencia, no el objeto del cambio.
