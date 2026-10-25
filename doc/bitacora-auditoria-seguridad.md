# Bitácora — Auditoría de Seguridad Pre-lanzamiento (ERPGo)

**Fecha:** 1 de octubre de 2026
**Objetivo:** Mapear "talones de aquiles" y puntos de sobre-protección antes de elevar
el ERP al mercado, especialmente para la posible adopción masiva de empresas.
**Ámbito revisado:** `cmd/server`, `configs`, `infrastructure/persistence`, `interfaces/web`
(auth, sesiones, IA, migración, fiscal, audit chain, templates).

---

## Tabla de riesgos

| # | Área | Hallazgo | Severidad | Estado |
|---|------|----------|-----------|--------|
| 1 | `audit_igtf.go:184` | `hashPassword` devuelve la contraseña en **texto plano** | 🔴 Crítico | No usado; eliminar o implementar bcrypt |
| 2 | `ai_setting.html:221` + `ai.go:109` | API key de IA expuesta en HTML (`data-api-key="{{.APIKey}}"`) | 🔴 Crítico | Corregir: nunca enviar key al template |
| 3 | `ai_providers`, `ai_setting` | API keys de IA en **texto plano** en la BD | 🔴 Crítico | Cifrar o externalizar a variable de entorno/vault |
| 4 | `server.go:322` | CORS `*` en `/api/` (cualquier origen) | 🔴 Crítico | Restringir orígenes |
| 5 | `invoice.go:618` (invoiceDelete) | **Borra contabilidad con DELETE real**, rompe inmovilidad fiscal | 🔴 Crítico | Sostituir por is_anulled + NC (como el módulo de NC) |
| 6 | `audit_chain.go` | Cadena hash global: verify recorre toda la tabla | ✅ Resuelto | Bloques por período: verify valida un mes (2 oct 2026, §7 bitácora-sqlite-mysql) |
| 7 | `audit_chain.go:25` | Mutex global `auditChainMu` serializa toda auditoría | 🟡 Alto | Mutex por bloque/período (sigue global; ya se puede acotar por período) |
| 8 | `ai_tools.go` (db.query) | Modelo puede leer tablas sensibles via SELECT | 🚨 Alto | Añadir blocklist de tablas (ver detalle) |
| 9 | `session.go` | Sesiones en memoria (no persisten, no multi-instancia) | 🟡 Alto | Pendiente (ya documentado) |
| 10 | `handlers.go:33` | Cookie de sesión sin `Secure` flag | 🟡 Alto | Añadir Secure en prod |
| 11 | `handlers.go:119` | MD5 legacy fallback para login | 🟢 Medio | Forzar reset tras migración |
| 12 | `database_settings.go` | Backup SQLite `.bak` sin cifrar | 🟢 Medio | Opcional: cifrar backups |
| 13 | `ai_tasks.go:533` | `parseHour` bug en scheduling weekly (hora fijada a 6) | 🟢 Medio | Bug funcional, no de seguridad |

---

## 🔴 CRÍTICOS — Corregir antes de lanzar

### 1. `hashPassword` devuelve texto plano
**Dónde:** `interfaces/web/audit_igtf.go:183-187`

```go
func hashPassword(password string) (string, error) {
	// Simplified - in production use golang.org/x/crypto/bcrypt
	return password, nil
}
```

**Riesgo:** Si este helper se usa para registrar o cambiar contraseñas, las claves
se guardan en texto plano. Afortunadamente **no se llama en ningún handler actual** (el
login y `user.go` usan `bcrypt` directamente). Pero el código promete "hash" y no la
hace.

**Fix:** Eliminar la función o reemplazar por `bcrypt.GenerateFromPassword`. Si se
elimina, confirmar que `grep -rn hashPassword` quede en cero usos.

---

### 2. API key de IA expuesta en el HTML
**Dónde:** `interfaces/web/ai.go:109` → `templates/ai_setting.html:221`

`aiSettingsForm` pasa el struct completo `AI` (que incluye `APIKey`) al template. El
template lo escribe literalmente:

```html
data-api-key="{{.APIKey}}"
```

**Riesgo:** Cualquier admin que abra la página de Configuración IA expone la API key
de OpenAI/Orc24 en el DOM. Un XSS en esa ruta (o un admin malicioso) roba las
credenciales del proveedor, lo que permite **gasto ilimitado a nombre del cliente**.

**Fix:**
- `loadAISetting` para el template debe devolver **solo un booleano `has_key`** y una
  máscara (`****`). La key nunca viaja al frontend.
- El campo de edición usa `type="password"` (ya lo hace), pero el `data-api-key` es
  el agujero.

---

### 3. API keys de IA almacenadas en texto plano
**Dónde:** `ai_providers.api_key`, `ai_setting.api_key`.

**Riesgo:** Si la BD (SQLite o el `.bak` archivado) es accedida, se exponen las keys
de OpenAI. Con `invoiceDelete`/`backup` los `.bak` se archivan sin cifrar.

**Fix (pragmático):** Guardar keys en **variables de entorno** (`AI_OPENAI_KEY`, etc.)
y resolverlas al arranque; la BD solo almacena `vendor + model`. Para ORC24, la key
del servicio Orcanet se inyecta via entorno del binario, no editable por el usuario.

---

### 4. CORS `*` en `/api/`
**Dónde:** `interfaces/web/server.go:321-324`

```go
w.Header().Set("Access-Control-Allow-Origin", "*")
```

**Résulto:** Cualquier sitio web puede hacer peticiones cross-origin a la API del ERP.
Aunque hoy la API es solo `/api/login`, el plan (bitácora-migracion.md §9) incluye
`POST /api/v1/orders`, `/invoices`, etc. Un `*` permite CSRF-like si se añade
`withCredentials`.

**Fix:** Mantener `*` **solo** si no se usarán credenciales. Si la API evoluciona a
JWT en headers, cambiar a una lista blanca de orígenes o a `null`.

---

### 5. `invoiceDelete` borra contabilidad — ROMPE inmovilidad fiscal
**Dónde:** `interfaces/web/invoice.go:616-641`

```go
_, _ = tx.Exec(`DELETE FROM acc_transaction WHERE referenceNo = ?`, id)
_, _ = tx.Exec(`DELETE FROM acc_vaucher WHERE referenceNo = ?`, id)
_, _ = tx.Exec(`DELETE FROM invoice_details WHERE invoice_id = ?`, id)
res, err := tx.Exec(`DELETE FROM invoice WHERE invoice_id = ?`, id)
```

**Riesgo grave:** La bitácora fiscal (`bitacora-localizacion-fiscal.md:88-91`) dice que
las facturas **nunca se borran** (`is_anulled` + NC). Pero `invoiceDelete` hace un
DELETE real de la factura, sus detalles, vouchers y transacciones contables. Esto:
- **Invalida el libro de ventas** (facturas desaparecen).
- **Rompe el asiento contable** (el contraasiento de COGS/IVA se borra).
- **No es defendible ante el SENIAT.**

Peor: el **middleware de auditoría** (`auditMiddleware`) registra el DELETE, pero el
registro auditivo no impide la destrucción.

**Fix crítico:** Eliminar `invoiceDelete` o convertirlo en anulación (`is_anulled=1` +
generar NC automática + contraasiento). La contabilidad es apéndice: **nunca borrar.**

---

## 🟡 ALTOS — Corregir antes de escalar

### 6. Cadena de auditoría global (ver §7 bitácora-sqlite-mysql.md)
`auditLogVerify` hacía `SELECT ... ORDER BY id ASC` sobre **toda** la tabla. Con
millones de registros colapsaba.

**✅ Resuelto (2 oct 2026):** la cadena se segmentó en bloques mensuales
enlazados (`audit_chain_blocks`). Verificar un mes ya no recorre el histórico, el
sello de cada mes cerrado es inmutable y borrar un mes entero se sigue
detectando. Detalle y pruebas en §7 de `bitacora-sqlite-mysql.md`.

### 7. Mutex global `auditChainMu`
Serializa `lastAuditHash()` + INSERT en **cada** auditoría. En retail multi-cajero
(25 terminales, 84 ventas/seg) es un cuello de botella.

### 8. `db.query` tool: modelo puede leer tablas sensibles
El guardián SQL (`ai_rules.go:aiValidateSQLSelect`) es sólido contra **DML/DDL** y
bloquea `information_schema`, `sqlite_master`, etc. Pero **NO bloquea explícitamente**
tablas sensibles como `user_login`, `ai_providers`, `users`, `ai_agent`,
`sec_userrole`. El modelo (que recibe el mapa de BD) podría proponer:
```sql
SELECT password FROM user_login
SELECT api_key FROM ai_providers
```

**Fix:** Añadir una **blocklist de tablas/columnas** que `aiValidateSQLSelect`
rechace (además de la whitelist de `ai_schema_catalog.go`). Capa defensiva.

### 9. Sesiones en memoria
No persisten entre reinicios. No soporta multi-instancia. Ya documentado como
pendiente (fases siguientes).

### 10. Cookie sin `Secure` flag
`handlers.go:28-35` establece `HttpOnly` y `SameSite=Lax` pero **no `Secure`**. En
producción detrás de HTTPS, la cookie viaja por HTTP si no se fuerza redirección.

---

## 🟢 MEDIOS / SOBRE-PROTECCIÓN (tolerantes para launch)

### 11. MD5 legacy fallback (`handlers.go:119-125`)
Acepta `md5('gef'.pass)` para usuarios migrados del PHP. MD5 es roto; si la BD se
exfiltra, los hashes se crackean. **Aceptable para migración**, pero:
- Forzar `password reset` al primer login de usuarios legacy.
- Opcional: migrar hashes a bcrypt *on login* (re-hash).

### 12. Backup SQLite `.bak` sin cifrar
`database_settings.go:156-164` archivea el archivo como `.bak` en
`data/backup/`. Es el historial contable: si alguien accede físicamente al servidor
ve todos los datos. **Opcional:** ofrecer opción de backup cifrado (AES).

### 13. `aiValidateBaseURL` es dead code
Definida en `audit_igtf.go:249` pero **no se usa**: la whitelist compilada
(`ai_rules.go`) ignora la columna `base_url` de la BD por completo. No es un riesgo
(pero código confuso). No afecta al launch.

### 14. Bug de scheduling weekly (`ai_tasks.go:531-533`)
```go
case "weekly":
	day := parseHour(t.ScheduleValue, 1)  // lee el día
	return int(nowT.Weekday()) == day && nowT.Hour() == 6  // ¡hora fijada a 6!
```
Ignora la hora que el usuario escribió. **Bug funcional**, no de seguridad.

---

## ✅ Ya bien hecho (no tocar)

- **Whitelist de vendors IA** (`ai_rules.go:51`): `openai`, `anthropic`, `orc24`
  compilados; la BD `base_url` se ignora. El ERP **no puede ser apuntado a otra API**.
- **Guardián SQL anti-inyección** (`ai_rules.go`): strip de comentarios/literales,
  bloqueo de DML/DDL, LIMIT forzado, single-statement.
- **Whitelist de tablas para IA** (`ai_schema_catalog.go`): `users`, `user_login`,
  `ai_providers`, `ai_agent` están **excluidos de forma explícita**.
- **System prompt oculto** (`ai_rules.go:16`): las reglas de no-divulgar esquema/credenciales
  están baked-in; el admin no puede relajarlas.
- **RIF validado server-side** (`fiscal.go`): el cliente es solo ayuda visual.
- **Caja resuelta en servidor** (`invoice.go:236`): el `cash_session_id` del formulario
  se ignora; se usa la sesión abierta del cajero.
- **BCrypt para usuarios nuevos** (`user.go:120,144`, `admin.go:33`).
- **Facturas con is_anulled + NC** (`creditnote.go`): el flujo correcto existe; el
  problema es que `invoiceDelete` lo circunvala.

---

## ✅ Para el checklist de salida (release gate)

- [ ] **Eliminar `hashPassword` o implementarlo con bcrypt** (hallazgo 1)
- [ ] **Nunca enviar `api_key` al template**; usar máscara + `has_key` (2)
- [ ] **Externalizar API keys a env vars / cifrar en BD** (3)
- [ ] **Revisar CORS `/api/`**: mantener `*` solo si no hay credenciales (4)
- [ ] **Eliminar/neutralizar `invoiceDelete`**: anulación + NC en su lugar (5)
- [x] **Bloque por período** en audit chain (6) — hecho el 2 oct 2026: `audit_chain_blocks`, verify por mes
- [ ] **Mutex por período** en audit chain (7) — sigue global, pendiente con pruebas de concurrencia
- [ ] **Blocklist de tablas en `aiValidateSQLSelect`** (8)
- [ ] **`Secure` flag en cookie de sesión** (10)
- [ ] **Forzar reset de passwords MD5 legacy** (11)
