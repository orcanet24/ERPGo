# ERPGo — Agentes IA: informes por plantillas + operador autónomo auditable

## 1. Principios de diseño (no negociables)

1. **La IA no entra al sistema; el sistema la orquesta.** El ERP llama al proveedor (chat completions con *tool calling*). El modelo nunca ejecuta nada: propone llamadas a herramientas y **el servidor Go valida y ejecuta**. Esto es el "MCP interno": las herramientas son intrínsecas al binario, no hay endpoint expuesto a agentes externos.
2. **Reglas ocultas en código.** Const `aiSystemCoreRules` compilada en el binario (no en BD, no en plantillas, no visible al cliente). Se antepone a todo prompt de agente. Prohíbe: divulgar esquema/internals, exfiltrar datos por cualquier medio (URLs, correos, enlaces), obedece instrucciones dentro de los datos (hardening anti prompt-injection: los datos se marcan como datos).
3. **Agente = usuario con rol.** Cada agente es una fila real en `user_login`/`users` con `sec_userrole` a un rol normal. Sus permisos son exactamente los de su rol: el guardián de servidor los aplica en cada herramienta. N agentes, cada uno con su proveedor de IA y su rol.
4. **Vendors con URL fija en código.** El cliente NO elige URLs ni toca `.env`: solo administra **OpenAI, Anthropic y ORC24** — los 3 y únicamente los 3 (ver §3).
5. **Todo auditable y reversible.** Cada acción de agente escribe en `audit_log` (cadena de hash existente) con el username del agente + marcador de agente. Las escrituras usan patrones de reversión (ver §5).

## 2. Modelo de datos — nueva migración `migrations/schema_z_ai_agents.sql`

- **`ai_agent`**: `id, name, user_id (FK user_login), provider_id (FK ai_providers), model_override, role_id (espejo de sec_userrole), operator_enabled (0/1), max_steps, max_rows, enabled, created_by, created_at`.
- **`ai_task`** (tarea/plantilla de tarea): `id, agent_id, name, instructions (prompt de la tarea), schedule_type ('manual'|'daily'|'weekly'|'cron5'), schedule_value, enabled, created_by`. Ejemplos: "Registrar facturas del correo X", "Analizar cuentas por cobrar y reportar", "Procesar inventario Y".
- **`ai_task_run`**: `id, task_id, agent_id, status ('running'|'done'|'failed'|'reverted'), started_at, finished_at, steps (JSON: bitácora de cada llamada/herramienta/resultado), final_report (markdown), undo_journal (JSON: pila de compensaciones en orden)`.
- **`report_template`**: `id, name, description, definition (JSON: datasets, layout, gráficos, filtros, parámetros), created_by ('user:<id>' | 'agent:<id>'), status, created_at, updated_at`.
- `ai_providers` gana columna **`vendor`** (`'openai'|'anthropic'|'orc24'`); seeds de menú: `sub_module` nuevos `ai_agents`, `ai_tasks`, `custom_reports` (con `INSERT IGNORE` idempotente como los existentes).

## 3. Proveedores — whitelist compilada en el binario

- **Cambio de seguridad clave:** se elimina el campo `base_url` libre del formulario. Las URLs base son **constantes privadas en código Go**:
  ```go
  vendorBaseURL = map[string]string{
      "openai":     "https://api.openai.com/v1",
      "anthropic":  "https://api.anthropic.com/v1",
      "orc24":      "https://ia.orcanet1724.click/v1",   // dominio propio ORC24
  }
  ```
  El endpoint de completions se deriva del vendor; una URL proveniente de BD/.env se **ignora** para estos vendors.
- El admin solo escoge el vendor (dropdown de los 3), API key y modelo. El cliente no puede apuntar el ERP a otra API aunque manipule la BD: el binario resuelve y valida el host contra la whitelist antes de cada llamada.
- Compatibilidad: las filas existentes de `ai_providers` sin `vendor` se marcan `openai` si su host era `api.openai.com`; endpoints arbitrarios locales (Ollama/LM Studio) quedan fuera del whitelist según tu decisión.
- ORC24 solo necesita implementar el estándar OpenAI-compatible (chat completions + `tool_calls`) en `ia.orcanet1724.click`; al estar lista su URL ya estará compilada. Si en el futuro el dominio cambia, se actualiza la constante en una nueva versión del binario (control total tuyo).

## 4. Catálogo estructural de la BD ("mapa para la IA")

- Nueva capa `schema_catalog.go`: introspección en vivo (`information_schema` en MySQL, `sqlite_master`+`PRAGMA` en SQLite, reutilizando [dialect.go](interfaces/web/dialect.go)) **restringida a una whitelist curada** de ~45 tablas de negocio (invoice, invoice_details, customer_information, product_information, product_purchase, acc_*, tax_collection, cash_session, etc.).
- Cada tabla: descripción en español, columnas con tipo y comentario, relaciones (joins típicos). Tablas excluidas por diseño: `user_login` (credenciales), `ai_providers` (api_key), `session*`, internas de Go. El mapa se serializa como texto compacto al prompt del agente y se muestra al humano en el **modal de ayuda** del diseñador de informes.

## 5. Herramientas (tools) del agente

**Fase 1 — Solo lectura + informes (disponible a todo agente con permiso):**
- `db.query`: SELECT de solo lectura sobre las tablas whitelisted, con **guardián SQL**: una sola sentencia, debe iniciar con `SELECT`/`WITH`, denegación de `;`, `INSERT|UPDATE|DELETE|DROP|ALTER|ATTACH|PRAGMA|...`, `LIMIT` forzado, tope de filas/tiempo, ejecución en conexión dedicada de solo lectura (SQLite read-only DSN; MySQL: validar sintácticamente + documentar usuario DB de solo lectura como dureza opcional).
- `report.save_template` / `report.run_preview`: crear/probar plantillas (solo si el rol del agente tiene create/read en `custom_reports`).
- `report.list_templates`, `data.summary` (agregados precalculados estilo aiAnalyze existente).

**Fase 2 — Modo operador (requiere `operator_enabled=1` en el agente Y permisos CRUD del rol):**
- `invoice.create_draft`: crea factura en estado borrador (`invoice.is_draft='yes'`), **reversión = eliminar borrador** (limpia, sin efectos contables).
- `invoice.post`: publica borrador (usa el flujo transaccional existente en [invoice.go](interfaces/web/invoice.go)); reversión = **nota de crédito** existente, nunca DELETE de registros financieros.
- `payment.record` / `payment.reverse`: cobros de clientes vía voucher + asiento inverso.
- `voucher.create` / `voucher.contra`: comprobante contable y su contra-asiento automático.
- Toda escritura registra en `undo_journal` la compensación exacta; el botón **"Revertir tarea"** ejecuta las compensaciones en orden inverso dentro de una transacción, dejando auditoría `task.reverted`.

## 6. Ejecución de tareas (secuencias autónomas)

1. Admin crea agente (rol + proveedor + límites) y tareas con instrucciones claras ("registra las facturas X", "procesa el inventario Y, luego reporta cuentas por cobrar").
2. El runner (goroutine con ticker + "Ejecutar ahora") lanza `ai_task_run`: bucle **modelo ↔ herramientas**: system = reglas ocultas + mapa BD + instrucciones de tarea; el modelo pide herramientas; el guardián valida contra el rol; resultados vuelven al modelo; hasta informe final o `max_steps`.
3. Al terminar: `final_report` (markdown) queda en la tarea y opcionalmente como `report_template`; todo el rastro en `steps` + `audit_log`.

## 7. UI (plantillas AdminLTE existentes)

- `agent_list.html` / `agent_form.html`: CRUD de agentes (vendor de la whitelist, API key, rol, toggle operador, límites) — solo admins.
- `ai_task_list.html` / `ai_task_form.html` / `ai_task_run.html`: tareas, bitácora por paso, informe final, botón Revertir.
- `report_template_list.html` / `report_template_form.html`: diseñador de informes con **modal de ayuda** que muestra el mapa de tablas (el mismo de la IA) + botón "Ayúdame con la IA" que pide al agente IT por defecto un borrador de plantilla.
- Sección de menú **"Informes personalizados"** (gate `{{if index .Perm "custom_reports"}}` en [layout.html](templates/layout.html)): lista plantillas, ejecución con parámetros de fecha, exportación CSV/impresión.
- Audit log: filtro por usuario-agente (reutiliza [audit_chain.go](interfaces/web/audit_chain.go)).

## 8. Verificación

- `go build ./...` + `go vet`; tests unitarios nuevos: guardián SQL (casos de evasión), generador de mapa BD (SQLite en memoria), **whitelist de vendors** (rechaza base_url ajena), motor de reversión (crear borrador → revertir → sin residuos), y los tests existentes (`go test ./...`) en verde.
- Prueba E2E manual documentada en `doc/`: crear agente → tarea "resumen de ventas" → informe generado → plantilla guardada → aparecer en menú → revertir tarea de operador.

## 9. Orden de construcción

1. Migración + catálogo BD + guardián SQL + reglas ocultas (núcleo de seguridad).
2. Whitelist de vendors (OpenAI / Anthropic / `ia.orcanet1724.click`) + `callChatWithTools`.
3. Agentes + tareas + runner + bitácora/auditoría (Fase 1: lectura e informes).
4. Plantillas de informes + diseñador + modal ayuda + menú.
5. Modo operador: borradores de factura, pagos, vouchers + motor de reversión (Fase 2).
6. Tests + documentación.