# Informe — Participación real de la IA en ERPGo y arquitectura para expandir el sistema con IA (noviembre 2026)

**Fecha:** 2 de noviembre de 2026
**Solicitado por:** el propietario del proyecto, tras observar «fallas constantes y errores» del agente de IA dentro del sistema.
**Método:** todo lo que se afirma aquí está verificado contra el código fuente y contra el historial de commits en esta sesión. No hay memoria de conversaciones anteriores: cada punto lleva su evidencia (archivo, línea o commit).
**Compañeros de lectura en `doc/`:** `PLAN-CORRECCION-GOBERNANZA-IA.md` (el plan que ya se ejecutó, véase su §5 con estado), `ERPGo  Agentes IA.md` (el diseño original de agentes), `bitacora-agentes-ia.md` (decisiones).

---

## 0. Resumen ejecutivo

1. **La IA ya es un usuario del sistema, no un agregado externo.** Cada agente tiene identidad real (`users` + `user_login` + `sec_userrole`), un rol RBAC y un departamento que acota lo que puede ver; sus acciones quedan en `audit_log` con `ip = 'ia'` ([ai_agents.go](../interfaces/web/ai_agents.go)).
2. **El «MCP» de ERPGo es un registro interno de herramientas, no un protocolo externo.** Todo lo que la IA puede hacer pasa por una herramienta `aiToolDef` declarada con su nivel de riesgo y su permiso; lo no declarado se rechaza por omisión. Esa frontera es diseño deliberado y es la razón de que el conjunto sea seguro.
3. **Las fallas observadas fueron, en su gran mayoría, defectos de ingeniería ya corregidos con prueba según el historial**, no consecuencias del diseño: cortes por presupuesto de tokens, timeouts cortos, compresión que se comía el stream, SQL con parámetros no declarados, guardián DNS demasiado estricto. Corregidos en `7974711`, `22c6317`, `89f62f0`, `b42910a`, `e611dc4`, entre otros (§3.1).
4. **La visión pedida —sistema polimórfico con núcleo sellado, IA exclusiva, add-ons creados por IA, parámetros fiscales intocables— es factible, no dañina y reversible con la arquitectura que ya existe** (journal de compensaciones, add-ons firmados, catálogos versionados). Lo comercialmente atractivo depende de condiciones concretas que el informe cuantifica en §7.
5. **El eslabón débil que queda es la calidad de generación de informes con modelos que razonan largo.** Hay trabajo a medio hacer en el árbol de trabajo (§3.6) que conviene decidir antes de seguir agregando funcionalidad encima.

---

## 1. Mapa de la superficie real de la IA en ERPGo

### 1.1 Proveedores (de dónde viene la inteligencia)

Verificado en [ai_providers.go](../interfaces/web/ai_providers.go), [ai_providers_tools.go](../interfaces/web/ai_providers_tools.go), [ai_dns_guard.go](../interfaces/web/ai_dns_guard.go), [ai_respan.go](../interfaces/web/ai_respan.go) y sus pruebas:

- **La URL base siempre viene validada contra una whitelist compilada** (`aiVendorURL` en `ai_rules.go`): el binario no contiene URLs de proveedores, pero tampoco acepta una de un `vendor` fuera de la lista. La prueba [ai_proveedores_oficiales_test.go](../interfaces/web/ai_proveedores_oficiales_test.go) fija la combinación correcta: el `vendor` sale de la lista cerrada y la URL del registro del proveedor, validada contra esa lista.
- **Un único proveedor activo.** Todos los agentes y todas las pantallas trabajan con el activo de Configuración IA ([ai_agents.go](../interfaces/web/ai_agents.go), `aiProveedorActivoParaAgentes`); el valor que traiga un formulario viejo se ignora. Esto evita la plaga clásica de «el agente quedó amarrado a un proveedor que nadie usa ya».
- **El modelo lo elige el proveedor, no el agente.** Al guardar un agente se vacía `model_override` siempre: cambiar de modelo es un acto de Configuración IA, no una edición por agente.
- **Pasarela opcional (Respan).** Si existe `RESPAN_API_KEY`, las llamadas van por la pasarela configurada en el proceso, no directamente a la del proveedor ([ai_respan.go](../interfaces/web/ai_respan.go)).
- **Guardián DNS.** Toda resolución del host del proveedor se valida contra resolvers públicos (DoH) y la conexión se fija a las IPs verificadas; el caso de un proveedor detrás de un CDN geográfico se resolvió comparando contra la **unión** de los resolvers y no contra la intersección (`e611dc4`). Un falso positivo admite desactivar el guardián con `ERPGO_AI_DNS_GUARD=off`, decisión consciente y documentada en el propio código.
- **Claves de API guardadas en la base de la empresa** (`ai_provider.api_key`), sin valor en el binario y fuera de los mensajes al usuario. En una operación SaaS real conviene moverlas a un almacén de secretos (§6, hallazgo 2).

### 1.2 Agentes (cómo se configura como usuario)

Verificado en [ai_agents.go](../interfaces/web/ai_agents.go) y [ai_schema_introspect.go](../interfaces/web/ai_schema_introspect.go):

- **Identidad real.** Al crear un agente se abre una transacción en la base de CONTROL que escribe `users`, `user_login` y `sec_userrole` con su `sec_role`, y después la fila `ai_agent` en la misma transacción. Su nombre de usuario es `agente.<slug>` y su contraseña es un secreto aleatorio con `status=0`: nadie la conoce y nadie la usa — el agente actúa a través del runner del servidor, no inicia sesión.
- **El nivel de permiso es el de su rol.** El administrador lo crea como un operador más; el agente trabaja con los permisos de su `sec_role` en las pantallas web y con esos mismos permisos en el guardián de herramientas (§1.3).
- **Departamento efectivo, con dos fuentes.** La columna `ai_agent.department` manda si tiene valor; si está vacía se deduce del nombre del rol. El comentario del código deja claro el por qué: un agente de cobranza con un rol con nombre propio caía en IT y veía la nómina. El diseño permite distinguir también «IT explícito» de «agente viejo sin tocar».
- **Límites acotados en el servidor:** `max_steps` (tope 40, por defecto 8) y `max_rows` (tope `aiHardMaxRows`, por defecto 200).
- **Prompt de comportamiento por agente** (`config_prompt`), que se aplica después de las reglas ocultas. Las reglas ocultas no son editables ni por el administrador ni por el agente.

### 1.3 El «MCP»: el registro interno de herramientas con riesgo

No es la librería MCP. Es el conjunto `interfaces/web/ai_tools.go` + `ai_tools_mcp.go`, donde **el registro es la frontera**:

- Cada herramienta (`aiToolDef`) lleva `Name`, `Risk` (cuatro niveles: read/draft/execute/critical) y `Permission` (directorio + acción de RBAC), declarados **juntos** a propósito: ese diseño ya evitó un bug real documentado en el comentario de `ai_governance.go` (una herramienta se clasificó tarde y el guardián la bloqueó; el catálogo y el guardián comparten la misma fila de registro).
- **Lo no declarado no existe:** una herramienta que no esté en el registro se rechaza por omisión. Con una whitelist el defecto nace cerrado, no abierto.
- **El catálogo anunciado y el guardián comparten el vocabulario:** al modelo solo se le ofrece lo que de verdad pasa el guardián (con permiso y con operador cuando aplica), para no gastar pasos ni tokens en herramientas que serán rechazadas.
- **La calificación de riesgo viaja con la herramienta, no con el modelo:** una herramienta `critical` dispara por sí misma la marca `needs_human`, y las herramientas `OperatorOnly` exigen `operator_enabled=1` en el agente.

### 1.4 La gobernanza ya ejecutada

De `doc/PLAN-CORRECCION-GOBERNANZA-IA.md` §5 — implementado y verificado con la suite en verde (17 pruebas nuevas del módulo IA):

| Hueco | Qué se hizo | Dónde |
|-------|-------------|-------|
| 1 — «requiere humano» era un campo muerto | `NeedsHuman` exportado con etiqueta `needs_human`, columna en `ai_task_run`, distintivo en la corrida y en el listado, y acto de revisión (`POST /ai/tasks/runs/{id}/review`, admin/master, auditado) | `ai_tools.go`, `ai_tasks.go`, `ai_run_slot.go`, plantillas |
| 2 — el guardián no consultaba el rol | `aiToolDef.Permission` junto al riesgo; guardián que lo exige; catálogo que no anuncia lo que el guardián rechazará; resolución del rol sin `*http.Request` | `ai_permissions.go`, `ai_governance.go`, `ai_tools.go` |
| 3 — sin idempotencia, con tres multiplicadores | Ranura de período por tarea (tabla `ai_run_slot`), libro de idempotencia `ai_tool_execution` con la clave reclamada **dentro de la transacción** del efecto (el patrón existente de `pos_sale_idempotency`, copiado y no inventado), `external_ref` en las herramientas de escritura, claves liberadas al revertir y reversión de corridas fallidas | `ai_idempotency.go`, `ai_run_slot.go`, `ai_tasks.go`, `worker.go`, `migrations/schema_z_ai_governance.sql` |

Nota honesta del propio plan: la decisión tomada fue **marca real con revisión posterior** (la corrida ejecuta y el humano revisa, con compensación exacta en el journal), no la compuerta que bloquea antes de ejecutar. Es una decisión de producto que conviene revisar cuando el sistema sea exclusivo del propietario y exista IA propia (§7.3, Fase 3).

### 1.5 Herramientas del catálogo con su riesgo actual

De `ai_tools.go` y `ai_tools_mcp.go`:

| Riesgo | Herramientas |
|--------|--------------|
| **read** | `db.query`, `report.list_templates`, `report.run_preview`, `data.summary`, herramientas de lectura de compras |
| **draft** | `report.save_template`, `invoice.create_draft`, `account.propose_transaction`, `invoice.propose`, `payroll.propose`, `customer.upsert`, `supplier.upsert`, `product.upsert` |
| **execute** | `invoice.post`, `account.post_voucher`, `purchase.create` |
| **critical** | `payment.record`, `voucher.create`, `invoice.anull` |

**La sola existencia de las herramientas «propuesta» codifica el principio que la visión pide** — la IA como empleado supervisado: propone, un humano aplica. El estándar vive en el propio código: `aiProposal.applied` es `false` siempre, y `ai_tools_mcp.go` lo justifica: «Convertir una propuesta en una acción es un acto consciente de alguien, y ese alguien queda en la auditoría».

### 1.6 El rol de IT y los informes a medida

Verificado en [ai_reports.go](../interfaces/web/ai_reports.go), [report_engine.go](../interfaces/web/report_engine.go), [ai_schema_introspect.go](../interfaces/web/ai_schema_introspect.go):

- El asistente de informes personalizados y el refinado comparten el núcleo `iaPreviewDefinition`.
- El camino real de las corridas (medido en las pruebas y en los commits):
  1. **Fase 1 — clasificación de módulos:** una llamada directa y liviana (~1.5K tokens) decide qué módulos de negocio tocan al pedido (`def7e9e`). Esa fase ya acotó el mapa de la Fase 2 de ~15K a ~4K tokens.
  2. **Fase 2 — generación del SQL y de la definición** (en HEAD aún por agente con herramientas; pendiente la conversión a «dos pasos directos», véase §3.6).
  3. **Validación del SQL por el guardián** (`aiValidateSQLSelect`): solo SELECT, contra tablas del catálogo, con parámetros que el motor resuelva; el parámetro no resuelto se rechaza con error claro (corregido en `b42910a` para no perder corridas).
- **Todo lo que produce la IA para informes nace en `report.save_template` (riesgo draft):** se guarda como plantilla revisable, nunca queda escrito como efecto inmediato en la base.
- El catálogo de módulos vive de la whitelist de tablas asignadas por módulo ([ai_schema_modules.go](../interfaces/web/ai_schema_modules.go)), con una prueba que falla si una tabla del whitelist queda sin módulo.

### 1.7 GOti, la guía en vivo

Verificado en [guia_goti.go](../interfaces/web/guia_goti.go), [guia_test.go](../interfaces/web/guia_test.go) y [guia_dialect.go](../interfaces/web/guia_dialect.go):

- La guía de bienvenida (GOti) guarda el progreso en la base (sobrevive a un reinicio del ERP), respeta `hidden` para los pasos ya vencidos, exige CSRF y usa el rol real del usuario en los tours (`1a10aa6`). No es la IA, pero es la primera cara de «expansión sin técnicos» que un usuario nuevo toca.

### 1.8 Add-ons: donde la visión pide «creados por IA»

Verificado en [addons.go](../interfaces/web/addons.go), [addons_admin.go](../interfaces/web/addons_admin.go), [addon_hooks.go](../interfaces/web/addon_hooks.go), [tools/firmaraddon](../tools/firmaraddon/main.go) y [docs/PLAN-ADDONS.md](../docs/PLAN-ADDONS.md):

- **Ciclo de firma completo en el código:** `tools/firmaraddon` firma el addon con la clave privada; el host valida contra `pubkey.ed25519`; está documentada la rotación de la clave pública y existe el modo `ADDONS_UNSIGNED` explícito para desarrollo.
- **El addon no es un plugin dinámico:** es un **binario separado** que el host lanza. Los arreglos recientes sobre Windows son rutas absolutas sin excepción (CreateProcess resuelve el ejecutable sobre el directorio del hijo) y buscar binarios sin depender de los bits de ejecución de `os.Stat`, que en Windows no reportan el bit X (`1e730dc`).
- **Los hooks de fase 3 ya tienen prueba:** la fila de hook nace pendiente en la base de control y el orquestador la entrega al addon (batería de hooks, `07a69bd`).
- **El activado/desactivado lo hace el administrador, no el addon.** La instalación pasa por el panel de administración con firma verificada: es exactamente la frontera «activados y desactivados por el superadmin» que la visión pide, **ya implementada**.

### 1.9 La auditoría de las acciones de IA

Cada acción de agente va al rastro de la empresa con `ip='ia'` ([ai_agents.go](../interfaces/web/ai_agents.go), `aiAudit`), y la creación, edición y borrado de agentes queda auditada con el creador real. Esta es la base sobre la que la expansión sin técnicos puede auditarse después.

---

## 2. Dónde NO está la IA hoy

Conviene por escrito porque la ausencia también es diseño:

1. **El asiento contable, la nómina y el cálculo fiscal no los calcula la IA nunca:** `payroll.propose` devuelve el período y su estado, y las cifras las calcula el servidor cuando alguien lanza la corrida con las tasas vigentes de esa fecha. El propio `ai_tools_mcp.go` lo dice: «Devolver un monto calculado por el modelo sería inventar una nómina».
2. **Las tasas fiscales y el catálogo de conceptos de nómina valen por datos versionados y validados por humanos** (la pantalla de tasas: «Mientras una tasa no esté validada, el sistema no publica nóminas»). La IA los consulta por la herramienta `legal_summary` — que además informa explícitamente las normas sin verificar — pero jamás los crea ni los altera.
3. **No existe generación de add-ons por IA.** Existe el canal completo para que una IA externa (fuera del ERP) construya, firme e instale un addon (§1.8). El paso que falta es el de generación de código, y es decisión de diseño, no deuda de código (§4.5).
4. **No existe conexión MCP ingresante.** El ERP habla OpenAI-compatible con el proveedor activo. El nombre «MCP» del código se refiere al registro interno de herramientas; retitular la documentación para que nadie lo confunda es una adecuación documental pendiente (§6, hallazgo 5).

---

## 3. Análisis de las fallas constantes: por diseño o por ingeniería

La observación del propietario — «fallas constantes y errores, y creo muy fuertemente que es por diseño» — se responde aquí con la cadena de correcciones del historial, sintomático por sintomático.

### 3.1 No eran por diseño (defectos de ingeniería ya corregidos con prueba)

| Síntoma visto | Causa raíz y evidencia | Corregido en |
|---------------|------------------------|--------------|
| El informe se pierde con «context deadline exceeded» | El plazo de llamada a la IA era de 60 s: un razonamiento largo no cabía en él. | `22c6317` (60 s → 180 s; el SSE renueva el WriteTimeout del servidor por evento) |
| La corrida del asistente se cortaba siempre | Presupuesto de tokens con suelo 2K y tope 32K: LongCat-2.0 razona largo antes de escribir el JSON, se cortaba y el reintento llegaba tarde. | `7974711` (suelo 2K→8K, tope 32K→131K, plazo por llamada 600 s) |
| El asistente nunca avisaba el fin (SSE colgado) | El gzip absorbía la respuesta que transmite en vivo y el plazo de escritura expiraba sin evento final. | `89f62f0` (gzip no comprime SSE, y el plazo se renueva por evento) |
| El informe cadaba con Error 1064 y se perdía la corrida | El SQL generado llegó con `:agrupar` literal: el guardián dejaba pasar un parámetro que el motor no sabe resolver. | `b42910a` (el guardián lo detecta y el contrato del prompt lo avisa) |
| El proveedor de IA se marcaba inaccesible sin ser culpa suya | El guardián DNS comprobaba la intersección de two resolvers; con un CDN geográfico la intersección era vacía y se rechazaban proveedores legítimos. | `e611dc4` (valida contra la unión,яется la conexión a las IPs ya verificadas) |
| El agente pedía una tabla y recibía SQL como contenido | `describe_schema` ponía el TEXTO de la consulta de columnas en el campo `columns` en vez del resultado. | Corregido en `ai_tools_mcp.go` (sa columna sale del introspector; el comentario documenta la corrida medida) |
| La pantalla «Agentes IA» mostraba la tabla vacía fuera de la empresa maestra | DataTables esperaba la matriz dentro de `{"data": ...}` y el JSON iba suelto en la raíz. | Mismo arreglo documentado en `agentListData` (envoltura como `/user/list/data`, con prueba de regresión) |
| Una tarea diaria corría varias veces el mismo día | El planificador consideraba «digita» una igualdad de hora en un ticker de cinco minutos: hasta 12 corridas dentro de la hora. | Gobernanza hueco 3: ranura de período `ai_run_slot` por tarea y tipo |
| Documentos duplicados al reintentar la cola | No había llave de idempotencia; `MaxAttempts` re-ejecutaba el trabajo ya hecho. | Libro `ai_tool_execution` con clave UNIQUE reclamada dentro de la transacción |
| La marca «requiere humano» se perdía silenciosamente | El campo del journal era NO exportado: `json.Marshal` lo ignoraba y el audit trail no mostraba nunca la marca. | Columna `needs_human` persistida, con prueba de regresión y acto de revisión |

### 3.2 Sí son por diseño — y no debe cambiar

1. **La IA propone, el humano aplica.** Herramientas de propuesta con `applied=false` siempre y riesgo critical naccessiblemente disponible sin revisión. Cambiar eso por «la IA escribe y el sistema confía» es la única decisión del informe que NO se recomienda.
2. **El SQL de la IA nace solo SELECT y acotado.** Sin el guardián de SQL no existe la construcción segura de informes ni el rol de IT que la visión presupone.
3. **El catálogo fiscal vive fuera del alcance de la IA.** Tasas y períodos se cierran con validación humana; la IA consulta (`legal_summary`, con aviso explícito de normas sin verificar) pero jamás escribe.
4. **El addon es un binario firmado, separado del host.** Descarta cualquier ideal de «hot code loading» dentro del proceso del ERP: es una decisión de aislamiento, y es la que hace reversible el sistema.

### 3.3 La categoría que queda abierta: calidad de generación con modelos que razonan

Además de la plomería (§3.1), hay una cola de incidentes de **calidad de generación** (modelo que no llega a escribir el JSON, planificación por módulos incompleta, comentarios de código que salen con acentos corruptos). El caso más adelantado es el de la generación de informes: el trabajo a medio hacer (§3.6) ya diagnostica el por qué — los modelos que razonan se pierden en el esquema + herramientas — y la mejor candidata para cerrarlo es la conversión a dos pasos directos (§8, Fase 0).

### 3.4 Conclusión del capítulo

Las fallas observadas comparten un enemigo común y una norma que lo vence: la del AGENTS.md colocada en `59f15c9` («sin parches, transparencia total, honestidad absoluta»). Cada defecto se arregla de raíz, con prueba en verde, y el commit lo cuenta. El diseño es una caja fuerte, no una jaula: lo que se veía roto casi siempre fue plomería.

### 3.5 Una regla nueva que conviene formalizar: el estado mutable de la IA

La mayoría de los defectos de §3.1 tuvieron una raíz común: condiciones de carrera o estado compartido (journal, cache de operación, ranura de corrida, idempotencia). Es la única área del sistema que no era determinista al inicio de cada corrección. La consecuencia arquitectónica está en §5: **el estado mutable de la IA debe vivir en tablas versionadas de la base de control**, nunca en memoria de proceso ni en binario.

### 3.6 Trabajo a medio hacer detectado en el árbol (no tocado en este informe)

- `interfaces/web/ai_reports.go` tiene cambios sin commit que convierten `iaPreviewDefinition` del enfoque de agente-con-herramientas a **dos pasos directos** (1: SQL; 2: columnas/totales). Su propio comentario documenta el por qué: «Los modelos que razonan se perdían en el razonamiento sobre el esquema y las herramientas, consumían todo el presupuesto de tokens y nunca llegaban a escribir el JSON del informe».
- **Estado:** el cambio no se evalúa ni complete aquí; queda pendiente de decidir por el propietario. Es el candidata natural de la Fase 0 de la hoja de ruta (§8).
- Además, ese cambio trae los comentarios en español con caracteres acentuados rotos (`generaci�n`, `clasificaci�n`): si se conserva, hay que reescribir el archivo completo antes de commitear, como exige el AGENTS.md.

---

## 4. La visión pedida, pieza por pieza, con su dictamen

### 4.1 «La IA es un usuario más del sistema» — hecho

Verificado en §1.1–§1.2 y §1.9. Pendiente (decisión futura, no hueco): que el agente participe de la conversación entre usuarios (menciones, hilos).

### 4.2 «El rol de IT capaz de crear reportes a medida» — funcional, con un pendiente de calidad

Funcional hoy mediante el asistente de informes con guardián SQL (§1.6), pero la calidad observada (§3.3) es la mayor deuda de mejora. Sin resolverla, el rol de IT no sostiene el caso comercial más visible.

### 4.3 «El uso eficiente del MCP» — buena arquitectura con dos afinaciones

La eficiencia mejoró de forma medible (Fase 1 liviana, catálogo filtrado por permiso, cache de plantillas de operación `7d4d6d7`). Las dos afinaciones:

1. **Un «MCP» que no es el protocolo MCP.** Si el producto comercial desea exponer las herramientas del ERP a un cliente MCP estándar, la pieza correcta es un *bridge* del registro `aiToolDef` hacia el protocolo real, sin duplicar la lógica (el catálogo y el guardián ya están hechos). No se recomienda implementarlo hoy; se recomienda dejar mencionada la vía (§6, hallazgo 5).
2. **`max_steps` por rol además de por agente.** Un agente de IT con pasos altos es legítimo; un agente de consulta debe tener el presupuesto bajo por defecto. Hoy es un ajuste manual por agente (§6, hallazgo 6).

### 4.4 «Usuarios cualquiera con una IA que será exclusiva (más adelante)» — factible, sin romper nada

El sistema ya cruza «qué modelo» con «quién lo activa»: un único proveedor activo, elegido en Configuración IA por el superadmin, y el modelo lo decide el proveedor (§1.1). Cuando la IA sea exclusiva del propietario, será una fila de configuración más (una pasarela propia o un proveedor con perfil cerrado) y la decisión de activarla seguirá siendo del superadmin, no del agente. La whitelist compilada y la pasarela son las piezas de futuro aquí; nada obliga a tocarlas hasta que llegue.

### 4.5 «Add-ons creados por IA, activados y desactivados por el superadmin» — factible, y es la pieza correcta del polimorfismo

El ciclo de hoy ya es desplegable con seguridad, y el que falta no requiere que el ERP compile nada:

1. **Hoy:** addon = binario Go firmado con `firmaraddon`, hooks de fase 3, instalación por el panel, activado por el superadmin. Reversible: se desactiva, la fila de hook nace pendiente y el journal deja los documentos compensables.
2. **A construir:** la generación de código del addon la hace una IA **fuera del ERP** (editor + agente de desarrollo como los usados en este proyecto), con un *scaffold* interno del repo como base. El ERP solo ve el artefacto: binario firmado, verificación de firma, prueba en la base demo y aceptación del superadmin. Revertible al 100% con los mecanismos existentes.
3. **Nunca:** el ERP no compila, no firma ni carga código dentro de su proceso. Esa condición es la que hace el sistema no dañino y comercialmente confiable: el comprador audita un binario instalado y firmado, no la conversación de un agente.

### 4.6 «Reconfigurar, nuevos campos, adecuaciones de procesos» — factible con dos límites

1. **Campos de negocio nuevos:** la pieza correcta es la migración idempotente asistida (el patrón `asegurarColumna` del repo ya es la práctica), **con revisión humana obligatoria**. Ningún agente ejecuta `ALTER` sobre la base productiva en runtime.
2. **Adecuaciones de proceso (flujos, formularios):** la que el núcleo no debe absorber a la ligera. La pieza correcta es el addon (§4.5), que se apoya en los hooks ya definidos; si el proceso pide algo que ningún hook ofrece, primero se define el hook en el núcleo (código fuente + prueba) y después lo consume el addon.

### 4.7 «Núcleo sólido no modificable sin código fuente» — ya es el diseño; falta formalizarlo

El ERP es un binario Go: el núcleo no admite intercambio en caliente. Lo que falta es que la **política de tres capas** (§5) y las reglas de qué puede tocar cada capa queden escritas en el AGENTS.md como norma.

### 4.8 «Parámetros fiscales ultra seguros» — reforzar, no simplificar

Hoy: tasas versionadas con vigencia y verificación humana obligatoria, períodos cerrados, catálogos protegidos y fuera del catálogo de herramientas de la IA. Para la visión conviene un **guardián fiscal** con dos reglas que hoy no son explícitas en el código:

1. **Lista de tablas prohibidas por categoría** en `ai_governance.go` (tasas, períodos, `ai_provider`, `ai_agent`, catálogos contables críticos): ninguna herramienta del registro ni ningún hook de addon puede escribirlas. Hoy el control es parcial (whitelist de tablas + permisos); hacerlo explícito lo vuelve comprobable con una prueba.
2. **Validación en la instalación del hook del addon** de que no escriba esas tablas: la firma protege contra un artesano malintencionado, no contra un addon legítimo pero mal escrito; el host es quien debe negar el hook.

### 4.9 «La IA no puede crear módulos que llamen otras IA o alteren parámetros fiscales» — es regla de facto; falta cuñarla con prueba

Hoy el catálogo de herramientas es estático y nada generado por la IA registra herramientas nuevas: la regla prácticamente existe. Pero si mañana se implementa el *bridge* MCP de §4.3, la superficie cambia. La regla que hay que elevar a **código + prueba**:

- Ninguna herramienta nueva declarada puede apuntar a las tablas prohibidas (§4.8).
- Ninguna herramienta puede declarar/editar proveedores (`ai_provider`), agentes (`ai_agent`) ni lanzar llamadas a la propia IA (ni directo ni por la pasarela).
- La lista negra vive junto al catálogo, no en un mapa paralelo: el mismo registro `aiToolDef` que ya evitó un bug por tener dos listas desalineadas.

---

## 5. Arquitectura recomendada: núcleo sellado + capa polimórfica

Tres capas, con las reglas de movimiento de adentro hacia afuera:

```
Núcleo (binario; cada cambio exige código fuente y prueba)
  ├─ autenticación, multi-empresa, RBAC, períodos, audit_log
  ├─ catálogos fiscal/nómina versionados, motor de hooks
  ├─ runner IA: proveedor activo único, whitelists, guardián (riesgo + permisos + idempotencia)
  └─ registro de herramientas aiToolDef (fuente única de verdad del riesgo)

Capa polimórfica (configuración y datos, no código)
  ├─ reportes definidos por IA con guardián SQL (plantillas revisables)
  ├─ catálogo de módulos asignados por whitelist
  ├─ columnas de negocio nuevas por migración idempotente revisada
  └─ GOti (guía de onboarding configurable)

Capa de expansión (add-ons binarios firmados)
  ├─ hooks de fase 3 pendientes en la base de control
  ├─ instalación por el superadmin con firma verificada
  └─ reversión por journal de compensación
```

Reglas de frontera que hay que codificar como capítulo nuevo del AGENTS.md o del plan de IA:

1. Un cambio del Núcleo exige código fuente y prueba (ya es la práctica del repo); la Capa polimórfica no admite código: admite configuración y datos.
2. Los add-ons no compilan, no firman ni leen estado interno del núcleo: su contrato es el de los hooks del host.
3. La IA no escribe el Núcleo y no puede convocar otra IA (§4.9).
4. Toda expansión es reversible por journal + libro de idempotencia ya existentes.

---

## 6. Hallazgos y adecuaciones recomendadas (ordenadas)

| # | Hallazgo | Prioridad | Riesgo si se ignora |
|---|----------|-----------|----------------------|
| 1 | Decidir y culminar el trabajo a medio hacer de `iaPreviewDefinition` (§3.6): corregir la calidad de la generación de informes | Alta | El caso de uso comercial más visible sigue siendo irregular |
| 2 | Mover las API keys de proveedores fuera de la base de la empresa (variable del proceso o almacén de secretos: hoy quedan en cualquier backup SQL de la BD) | Alta | Fuga de credenciales ante un dump de la base |
| 3 | Formalizar la lista de tablas prohibidas y cuñarla con prueba (§4.8) | Alta | Un addon mal escrito o un cambio de whitelist contamina lo fiscal |
| 4 | Elevar «la IA no convoca a otra IA; no escribe `ai_provider` ni `ai_agent`» a regla con prueba (§4.9) | Media | El día que exista el bridge MCP, la regla no estará |
| 5 | Documentación: describir el «MCP interno» como registro de herramientas, y mencionar la vía de un bridge MCP sin implementarlo | Baja | Confusión pedagógica en la venta del producto |
| 6 | `max_steps`/`max_rows` también con propuestas por rol, no solo por agente (§4.3) | Baja | Disparo de costos por configuración descuidada |
| 7 | Medición de costo real por corrida de informe y por agente-día (bench con las corridas existentes) | Media | Fijar precio sin base |

---

## 7. Dictamen de factibilidad

**Funcional: sí, con grados por pieza.** Reportes a medida: funcional hoy con un pendiente de calidad (§3.3). Addons creados por IA: factible como flujo fuera del ERP sobre el canal de firma/instalación ya listo. Núcleo sellado: ya es el diseño; formalizar es política, no ingeniería. IA exclusiva: una fila de configuración del proveedor, no un cambio de arquitectura.

**No dañino: sí, con dos condiciones que el informe eleva a regla con prueba (§4.8–§4.9).** (i) La lista de tablas prohibidas explícita por categoría, y (ii) validación del hook del addon en la instalación. Sin eso, «no dañino» es una promesa; con eso, es una unidad de código con su test.

**Reversible: sí por construcción.** Tres mecanismos ya existen: journal de compensación de la gobernanza, add-ons desactivables (con la fila de hook que nace pendiente) y catálogos versionados. La reversión de corridas fallidas — cuando más falta hace — está implementada (plan de gobernanza, hueco 3).

**Comercialmente atractivo: sí, y es la razón del orden de la hoja de ruta.** El «sistema se expande sin técnicos» tiene mercado real (contadores, despachos, PYMEs) — pero la pieza de venta es **primero el rol de IT: reportes a medida** (hallazgo 1), que es el uso que un cliente paga y mide. Dos condiciones de instrumento comercial:

1. **Benchmark de costo por corrida antes de fijar precio.** El modelo de costo es el consumo del proveedor activo, medible con las correduras registradas; sin ese dato, el margen es una suposición.
2. **La IA exclusiva es ventaja competitiva pero agrega la obligación de una pasarela propia** — y las piezas para no romper nada ya existen (proveedor activo único, whitelist compilada, pasarela Respan). La IA propia entrará como una fila más de proveedor.

**Advertencia franca:** la parte más frágil de la visión no es técnica, es de producto — decidir por escrito la frontera entre «adecuaciones de proceso» que van a un addon y las que se resuelven en el núcleo. Sin esa frontera escrita, el «polimorfismo» degenera en demanda infinita de trabajo técnico, que es justo lo que la visión quiere evitar.

---

## 8. Hoja de ruta por fases (política, no código en este informe)

**Fase 0 — Cerrar la plomería pendiente.**
- Decidir y culminar los dos pasos directos de `iaPreviewDefinition` (§3.6), con sus pruebas.
- Custodia de claves de proveedor fuera de la BD (hallazgo 2).
- Reescribir documentación del «MCP» como registro de herramientas (hallazgo 5).

**Fase 1 — Cimentar las fronteras (regla + prueba cada una).**
- Lista de tablas prohibidas por categoría, con prueba (§4.8).
- Regla «la IA no convoca a la IA», con prueba (§4.9).
- Benchmark de costo por corrida (hallazgo 7).

**Fase 2 — Polimorfismo real.**
- Scaffold interno de add-ons para el agente externo + ciclo completo `firmaraddon` / instalación / journal.
- Migraciones idempotentes asistidas para campos de negocio, con revisión humana obligatoria (§4.6).
- Medición con el rol de IT activo.

**Fase 3 — Compuerta para acciones irreversibles (opcional).**
- Si la IA exclusiva entrara en producción, revisar la decisión de §1.4: pasar `invoice.anull` (y las futuras funciones de nómina irreversible) de marca a compuerta (`pending_approval`), según la opción (b) ya archivada en el plan de gobernanza.

**Fase 4 — Producto.**
- Precio sobre el benchmark real; soporte apoyado en la operación de revisión de corridas; onboarding con GOti; plantilla interna de add-ons.

---

## 9. Verificación de este informe y límites

**Verificado con evidencia en esta sesión:**
- Lectura directa del catálogo de herramientas con sus riesgos y permisos (`ai_tools.go`, `ai_tools_mcp.go`).
- Lectura del plan de gobernanza ejecutado y de su estado documentado (`doc/PLAN-CORRECCION-GOBERNANZA-IA.md` §5).
- Lectura directa de la identidad, límites y auditoría de agentes (`ai_agents.go`).
- Historial de commits consultado en la sesión para la cadena de correcciones, con identificadores reales citados.
- Lectura de la pantalla de tasas fiscales, GOti, add-ons y el plan de add-ons.

**No verificado (y por qué):**
- Corridas vivas contra un proveedor real en esta sesión: el informe se apoya en las pruebas automatizadas del módulo y en los incidentes documentados en commits; no se lanzó una corrida contra el proveedor activo para elaborarlo.
- La suite completa `go build/vet/test` no se ejecutó para este informe (no se modificó código fuente; solo se creó este documento): lo que respalda las alegaciones de gobernanza es el plan archivado con su verificación registrada en verde.
- El trabajo sin commit en `ai_reports.go` (§3.6): se consultó su diff como estado del árbol; **no se corrigió, no se descartó y no se commiteó aquí**, decisión pendiente del propietario.

**Una advertencia honesta de este propio documento:** la primera versión de este informe se corrompió (acentos y palabras dañadas en varios párrafos) y fue reescrita entera en lugar de parcheada — exactamente el defecto de generación de §3.3 en acción. Es otra razón para prioritizar el hallazgo 1.
