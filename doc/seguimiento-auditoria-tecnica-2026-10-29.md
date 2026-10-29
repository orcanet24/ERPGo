# Seguimiento de auditoría técnica de ERPGo

**Fecha de apertura:** 29 de octubre de 2026
**Base:** [informe de auditoría técnica](informe-auditoria-tecnica-2026-10-29.md)

## Regla de trabajo

Se trabaja en orden numérico: completar un punto, verificarlo con pruebas apropiadas y actualizar su estado con evidencia antes de comenzar el siguiente. Ningún punto se declara listo por prioridad, por adelantar otros asuntos o por corregir solo una parte. Los puntos que exigen validación fiscal, infraestructura o producción quedan abiertos hasta contar con evidencia autorizada. No se hará migración de jQuery a Vanilla JS; el alcance es reparar código, plugins y scripts jQuery con fallos comprobados.

## Lista maestra

| N.º | Estado al 29-10-2026 | Hallazgo y condición para cerrar |
|---|---|---|
| 1 | **RESUELTO** | CSRF ausente en mutaciones `fetch` de reposición, conteo cíclico y tasas. Se añadió `ERPGo.csrfFetch` y se aplicó a siete POST. Test Go cubre plantillas; prueba Node verifica token same-origin en POST y ausencia en GET/cross-origin. El middleware conserva su exigencia CSRF. |
| 2 | **RESUELTO** | Guardado de tasas: interpretar estado HTTP/JSON `{error}`, mostrar éxito/error, capturar red, reactivar botón, respetar redirect y submitter, y definir fallback POST/action. Test del contrato, sintaxis JS y `webcheck` de plantillas aprobados. |
| 3 | **SIGUIENTE** | Formularios dobles con serialización: inventario estático detectó 29 plantillas con `save_exit` y serialización; varias guardan la selección solo en una variable local. Validar alcance de endpoints y controlar cada vista para que el submitter llegue al servidor y el destino sea correcto; reparar y cubrir todas las desviaciones identificadas. |
| 4 | **PENDIENTE** | Diferencia de criterio de facturas anuladas entre reporte de ventas/impuestos y libro de ventas. Validar regla fiscal y de negocio, conservar trazabilidad y probar conciliación. |
| 5 | **PENDIENTE** | Errores `Query`, `Scan` y `rows.Err()` silenciados en reportes financieros/fiscales; entregar error explícito y jamás un resultado parcial como si fuera completo. |
| 6 | **PENDIENTE** | Límites mensuales con día fijo 31. Cambiar los lugares identificados a intervalos semiabiertos y probar febrero, meses de 30/31 días y ambos bordes en motores soportados. |
| 7 | **PENDIENTE** | `parseMoney` y conversiones silencian formatos inválidos. Definir formatos aceptados (incluido venezolano), rechazar NaN/Inf/rangos indebidos y proteger invariantes monetarias con pruebas. |
| 8 | **PENDIENTE** | API keys POS llegan a logs. Retirar secretos, revisar exposición histórica/retención y gestionar rotación de las claves afectadas de forma segura. |
| 9 | **PENDIENTE** | Token CSRF de respaldo fijo ante fallo de `crypto/rand`. Fallar cerrado y probar que no se emite token ni se autoriza escritura cuando falla el generador. |
| 10 | **PENDIENTE** | CORS wildcard y excepción CSRF general para `/api/`. Inventariar clientes POS e integraciones, definir política endpoint por endpoint y probar seguridad/compatibilidad antes de ajustar. |
| 11 | **PENDIENTE** | Cookie de sesión sin `Secure` en login. Resolver TLS/proxy con configuración confiable y probar despliegue HTTPS y desarrollo local. |
| 12 | **PENDIENTE** | Evaluar CSP, `Referrer-Policy` y HSTS sin romper plugins/scripts; HSTS solo en dominios totalmente HTTPS y con pruebas de regresión. |
| 13 | **PENDIENTE** | Fallo del journal/orquestador: definir y probar garantías para que escrituras críticas no queden sin traza silenciosamente. |
| 14 | **PENDIENTE** | Verificar ejecución, permisos, integridad y restauración de backups en ambiente autorizado; el código por sí solo no acredita recuperación. |
| 15 | **PENDIENTE** | Usar `America/Caracas` en fechas civiles de negocio y UTC en instantes técnicos; probar límites de día/mes sin depender de la zona del host. |
| 16 | **PENDIENTE** | Comprobar recálculo del servidor para renglones, descuentos, IVA, IGTF, pagos y saldos; ampliar pruebas por documentos y alícuotas. |
| 17 | **PENDIENTE** | Auditar estabilidad de jQuery/plugins (incluido DataTables), refresco parcial, eventos duplicados y scripts rotos; reparar y probar hallazgos concretos. **No migrar a Vanilla JS.** |
| 18 | **PENDIENTE** | Medir latencia, consultas y payload antes de optimizar; revisar N+1/índices con datos anonimizados y comparar medidas. |
| 19 | **PENDIENTE** | Identificar activos estáticos sin uso con referencias y hashes; no borrar sin demostrar ausencia de consumidores y validar build. |
| 20 | **PENDIENTE** | Historial de auditoría limitado a 1000 filas sin señal clara. Añadir total/paginación/aviso de truncamiento y probar exportaciones. |
| 21 | **PENDIENTE** | Revisión pantalla por pantalla de errores visibles, conservación de entrada, estado de guardado, doble envío, filtros/paginación y actualización de saldos. Cierre solo con criterios y pruebas por pantalla. |

## Evidencia de los puntos cerrados

### Punto 1 — CSRF en fetch

- Helper `ERPGo.csrfFetch` centraliza cabecera CSRF para mutaciones same-origin sin enviar el token a otras fuentes.
- Aplicado a tres acciones de reposición, tres de conteo cíclico y guardado de tasas.
- Tests Go enfocados: **OK**.
- Prueba Node de POST same-origin / GET / POST cross-origin: **OK**.

### Punto 2 — Guardado de tipos de cambio

- Los tres formularios especifican `method=post` y `action` como fallback.
- La respuesta valida HTTP y `{status:1}`, muestra `{error}` y errores de red, reactiva el submitter y respeta redirect.
- Test del contrato: **OK**; `node --check` de scripts: **OK**; `webcheck`: **159 plantillas, 0 errores estructurales**.

## Verificación global

- `go build ./...`: **OK**.
- `go vet ./interfaces/web`: **OK**.
- `go test ./...`: **OK**.
- `go run ./internal/webcheck -dir templates`: **159 plantillas, 0 errores estructurales** (15 requieren datos reales para evaluarse completamente).
- Sintaxis de JavaScript de los scripts cambiados y prueba ejecutable del helper CSRF: **OK**.
- `git diff --check`: **OK**.
- No hubo prueba visual en navegador ni benchmark; no se afirma verificación de producción ni certificación fiscal.
