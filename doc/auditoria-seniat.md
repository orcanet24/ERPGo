# ERPGo — Sistema de Auditoría SENIAT (Cadena de Hash)

**Fecha:** 27 de septiembre de 2026

## Concepto: "binario inamovible e inalterable"

SENIAT requiere poder auditar todas las operaciones. La solución implementada
es una **cadena de hash SHA-256 encadenada** (misma idea que blockchain):

- Cada registro de `audit_log` incluye `prev_hash` (hash del registro anterior)
  y `record_hash` = SHA256(prev_hash + id + usuario + acción + módulo + tabla +
  registro + valor_anterior + valor_nuevo + IP + timestamp).
- **Alterar cualquier campo de cualquier registro rompe la cadena** a partir
  de ese punto: el verificador recalcula todos los hashes y señala el primer
  registro roto.
- **Eliminar un registro también rompe la cadena**: el `prev_hash` del
  siguiente ya no coincide.
- La aplicación **solo hace INSERT** en `audit_log` (append-only). El único
  UPDATE permitido es el que completa el hash del registro recién creado.

## Qué se audita automáticamente

El middleware `auditMiddleware` (audit_chain.go) registra **todo** POST/PUT/DELETE:

- Usuario (de la sesión), IP real (con soporte X-Forwarded-For), fecha/hora
- Acción (create/update/delete), módulo derivado de la URL, ruta completa
- Los handlers pueden además registrar valores old/new con `s.audit(...)`

## Interfaz: /reports/audit-log

- Filtro por rango de fechas, tabla DataTables con 1000 registros
- **Botón "Verificar Integridad"**: recorre toda la cadena y reporta
  `valid: true/false` con el ID del primer registro alterado y la causa
- **Botón "Exportar CSV"**: descarga con BOM UTF-8 (abre en Excel),
  incluye hashes para verificación externa por el auditor

## Verificación manual (para el auditor externo)

```
GET /reports/audit-log/verify   → {"valid":true,"total":N,"reason":"..."}
GET /reports/audit-log/export?from=YYYY-MM-DD&to=YYYY-MM-DD  → CSV
```

El CSV contiene `prev_hash` y `record_hash` de cada fila: el auditor puede
recalcular SHA256 en Excel/Python y confirmar que ningún registro fue
modificado desde la exportación anterior.

## Prueba de alteración (evidencia)

```
mysql> UPDATE audit_log SET user_id='hacker' WHERE id=1;
GET /reports/audit-log/verify
→ {"valid":false,"broken_at":1,"reason":"contenido alterado en registro 1:
   el hash recalculado no coincide con el almacenado"}
```

## Cambios de rendimiento

- Contador de stock del header: cacheado 60s (antes 1 query por página)
- Índices en audit_log (created_at, user_id)
- Collation unificado: 58 tablas convertidas a utf8mb4_general_ci
  (corrige "Illegal mix of collations" en búsquedas de productos)

## Archivos

- `interfaces/web/audit_chain.go` — cadena de hash, middleware, viewer, verify, CSV
- `migrations/schema_extra.sql` — columnas prev_hash/record_hash + índices
- `templates/audit_log.html` — interfaz con verificar/exportar
- `static/css/erpgow-refined.css` — refinamiento visual minimalista
