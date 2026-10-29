# Informe de auditoría técnica de ERPGo

**Fecha:** 29 de octubre de 2026
**Alcance:** revisión estática del código fuente disponible en este workspace. No se contó con una instancia autenticada del ERP, una base de datos de negocio ni un navegador; por tanto, las conclusiones describen defectos y riesgos observables en código, no resultados de pruebas en producción.
**Método:** revisión focalizada de formularios y rutas de guardado, CSRF, middleware HTTP, reportes fiscales e internos, conversiones monetarias, zona horaria, dependencias y activos estáticos.

## Resumen ejecutivo

ERPGo ya tiene controles valiosos: token CSRF de sesión, middleware de permisos, cookies de sesión `HttpOnly` y `SameSite=Lax`, auditoría, URLs versionadas para recursos estáticos, gzip configurable, respaldos automáticos y mecanismos específicos de integridad para compras y operaciones de IA. No se recomienda sustituir esos controles ni reescribir la aplicación como una SPA.

Sí hay problemas concretos que pueden explicar que algunos botones parezcan no responder:

1. **[RESUELTO 2026-10-29] CSRF en `fetch`:** las vistas de reposición, conteo cíclico y tasas enviaban POST sin `X-CSRF-Token`. El servidor los rechazaba con 403. Se centralizó el token en `ERPGo.csrfFetch` y las siete mutaciones afectadas lo usan; la protección del servidor no se relajó.
2. **[RESUELTO 2026-10-29] Errores ocultos al guardar tasas:** ahora se comprueba el estado HTTP y la confirmación JSON, se muestra `error`, se informa de fallos de red, se restaura el botón y se respeta el redirect del servidor y el botón pulsado.
3. **`Guardar`/`Guardar y Salir`:** varios handlers de formularios serializan con jQuery, lo que omite el botón pulsado. En esas vistas el JavaScript conserva una decisión local, pero no siempre la manda al servidor; el redirect del servidor puede no concordar con lo que el usuario seleccionó.
4. **Reportes con errores SQL silenciados:** lectores fiscales e internos omiten errores de fila/iteración o convierten fallos de consulta en resultados vacíos/parciales.
5. **Criterios fiscales distintos:** el libro de ventas excluye facturas anuladas; el reporte general de ventas y el de impuestos no aplican el mismo filtro.
6. **CORS demasiado amplio:** el middleware da `Access-Control-Allow-Origin: *` a todas las rutas `/api/`. Esto permite que cualquier origen lea respuestas de solicitudes cross-origin sin credenciales cuando el endpoint lo permita; no concede por sí solo lectura de APIs que requieren cookie con credenciales, pero la política debe definirse por tipo de cliente y no abrirse globalmente.

**Dictamen:** no es correcto decir que todos los botones de guardar estén rotos. El código tiene un flujo común correcto para muchas vistas y excepciones particulares con defectos identificables. Para verificar una queja concreta en producción se necesita la ruta, el rol/usuario, la petición HTTP de Network y el resultado de auditoría. El estado de cada hallazgo y su evidencia se mantienen en el [registro de seguimiento](seguimiento-auditoria-tecnica-2026-10-29.md).

## 1. Botones Guardar / Guardar y Salir

### Flujo general observado

- `templates/layout.html` carga `csrf.js` y `guardado.js` globalmente después de jQuery.
- `csrf.js` añade el token a las peticiones jQuery AJAX, formularios HTML normales y mutaciones `fetch` del mismo origen mediante `ERPGo.csrfFetch`.
- `guardado.js` intercepta formularios POST que no tienen un listener propio, manda el submitter y token CSRF, y procesa el JSON de éxito/error y su `redirect`.
- Los formularios con `preventDefault()` propio quedan fuera del interceptor global. AJAX mantiene igualmente los permisos, la validación y la auditoría que ejecuta el servidor.

### Hallazgos concretos

| Severidad | Hallazgo | Evidencia y efecto probable |
|---|---|---|
| Alta | **RESUELTO 2026-10-29:** POST por `fetch` sin token CSRF | `templates/inventory_reorder.html` enviaba POST a `/inventory/reorder/recalc`, `/inventory/reorder/suggestions` y `/inventory/reorder/policy` sin token; `templates/inventory_cycle_count.html` a `/count`, `/open` y `/close`; y `templates/exchange_rate_paralelo.html` al guardar tasas. El nuevo helper global `ERPGo.csrfFetch` adjunta la cabecera `X-CSRF-Token` solo en mutaciones del mismo origen. El middleware CSRF y la validación de servidor permanecen intactos. Hay test de cobertura de las siete llamadas y comprobación JS del helper. |
| Alta | **RESUELTO 2026-10-29:** errores no visibles al guardar tasas | `guardar()` valida `response.ok`, interpreta `{error}`, muestra el éxito, maneja errores de red, reactiva el botón y respeta `redirect`. Los tres formularios declaran POST/action para tener fallback funcional. Se transmite el submitter para distinguir Guardar/Guardar y Salir; test y parseo JS cubren el contrato. |
| Media | **EN CURSO — punto 3 del seguimiento:** se omite el botón pulsado al serializar | `$(form).serialize()` no incluye `name/value` del submitter. El inventario identificó 29 vistas con botones `action=save/save_exit` y serialización. Varias deciden el destino en una variable local del navegador, sin mandar la acción al handler, así que la respuesta del servidor y la navegación pueden divergir. Se validará vista por vista antes de marcar resuelto. |
| Media | Formularios de tasas solo funcionaban con JavaScript | `formBCV`, `formParalelo` y `formFuente` no declaraban `method=POST` ni `action`, por lo que dependían enteramente de JavaScript. **Resuelto 2026-10-29:** ahora declaran método y destino POST y sus respuestas tienen tratamiento robusto; véase el punto 2 del registro de seguimiento. |
| Media | Excepción CSRF amplia a `/api/` | `enforceCSRF` exime toda ruta `/api/`, incluido `/api/login`, y todas las peticiones sin sesión. Las APIs POS llevan autenticación propia, pero conviene fijar explícitamente la protección de login CSRF y el diseño de origen/CORS para cada consumidor. No se recomienda cerrar todas las APIs sin inventariar el POS y las integraciones. |
| Baja | La recarga blanda es una capa compleja | `guardado.js` sustituye `.content`, vuelve a ejecutar scripts y puede caer en recarga completa. No se verificó en navegador; es una zona de regresión potencial en vistas con DataTables, modales y scripts que inicializan listeners globales. |

### Diagnóstico para un caso reportado

1. DevTools → Network: confirmar si se envió POST, ruta, estado HTTP y JSON.
2. 403/token: fetch/AJAX sin CSRF, sesión vencida o token desactualizado.
3. 400: validación; conservar los datos ingresados y mostrar el mensaje.
4. 500: error DB/handler; buscarlo con ID de operación, sin registrar secretos.
5. 200 con `status:1`: confirmar fila y auditoría; luego revisar si el botón debía volver al formulario o al listado.
6. Sin petición: consola JS y carga del listener.

## 2. Seguridad

### Controles presentes

- Cookie de sesión `erpgow_session` `HttpOnly`, `SameSite=Lax`; los `Set-Cookie` de login revisados no fijan `Secure`. La cookie CSRF sí usa `Secure` cuando `r.TLS != nil`.
- Token CSRF aleatorio por sesión, comparación en tiempo constante y validación para métodos no seguros salvo `/api/` y ausencia de sesión. El helper de fetch solo adjunta token para mismo origen; no altera esas excepciones.
- Middleware de autorización por módulos/acciones, contexto multi-tenant, auditoría y mecanismos de integridad para operaciones financieras.
- `Cache-Control: no-store` por defecto; estáticos versionados con caché inmutable; gzip configurable.
- `.env` y `.dev.vars` están excluidos de Git; no se hallaron claves en los archivos rastreados examinados.
- Los endpoints POS protegidos usan autenticación POS específica.

### Riesgos y acciones

| Prioridad | Riesgo | Acción propuesta |
|---|---|---|
| Alta | **RESUELTO 2026-10-29:** fetch de mutaciones sin CSRF | Helper compartido `ERPGo.csrfFetch` agregado y aplicado a los siete POST de las tres vistas afectadas; el test asegura que no se revierta. No se retiró el requisito CSRF. |
| Media | `Access-Control-Allow-Origin: *` para todo `/api/` | Inventariar orígenes. APIs de navegador: allowlist exacta y `Vary: Origin`; clientes POS autenticados por token: CORS solo si se requiere. El wildcard no habilita cookies con credenciales en el navegador, pero tampoco debe usarse como control de acceso. |
| Alta | API keys POS aparecen en logs | `pos_admin.go` registra la clave completa al crear, revocar y borrar. Cambiar logs a ID/sucursal/acción; revisar retención/acceso de logs existentes y rotar claves que se hayan expuesto. |
| Media | Fallback débil de CSRF | `newCSRFToken` devuelve una cadena de ceros si falla `crypto/rand`. Fallar cerrado: no emitir token débil y bloquear escrituras hasta resolver el error. |
| Alta en despliegues HTTPS | Cookie de sesión de login sin `Secure` en los `Set-Cookie` revisados | Forzar HTTPS en el borde y definir `Secure` mediante configuración confiable. Detrás de proxy TLS, no confiar ciegamente en `r.TLS` ni en cabeceras proporcionadas por clientes. |
| Media | Falta de política de seguridad moderna completa | Se encontraron `X-Content-Type-Options`, `X-Frame-Options` y el legado `X-XSS-Protection`; no se encontraron CSP, `Referrer-Policy` ni HSTS. Diseñar CSP compatible con scripts/plugins antes de activarla; HSTS solo cuando todo el dominio esté servido por HTTPS. |
| Media | Excepciones API sin matriz explícita | Documentar endpoint → autenticación → tenant → permiso → rate limit, con pruebas HTTP para login, POS, integraciones y auditoría. |

**No hacer:** desactivar CSRF para “arreglar” guardar, abrir CORS general como remedio, o registrar secretos para depurar.

## 3. Activos estáticos y archivos potencialmente innecesarios

- Conteo de este checkout: **443 archivos** rastreados en `static/`, unos **26,4 MB**. DataTables pesa ~1,19 MB; hay otros paquetes JS/CSS grandes.
- `assets/` no tiene archivos rastreados aquí. Eso no significa que `static/` o `static/js/html/` sean basura.
- Hay copias idénticas de algunas imágenes de producto entre `static/image/product/` y `static/image/product-latest/`, y duplicados de logos, fuentes y temas. Pueden existir por compatibilidad o datos. No borrar sin localizar referencias y comprender la carga.
- Las huellas por archivo y URLs `?v=` ya ayudan a cachear e invalidar estáticos. Gzip está configurado.
- `internal/webcheck` y auditoría completa de referencias: todavía pendientes en esta pasada.

**Acción segura:** inventario de referencias (HTML/CSS/JS/Go), tamaño, hash y propósito; eliminar solo recursos sin referencias confirmadas, con respaldo y build/render comprobados. Optimizar imágenes por pipeline en vez de reemplazar datos existentes sin validar usos.

## 4. jQuery frente a JavaScript nativo

### Estado observado

- El layout carga jQuery **3.4.1** globalmente. jQuery UI, DataTables y otros plugins dependen de jQuery.
- Se contaron **117/159** plantillas con `$()`/`jQuery()` y 35 archivos/plantillas con referencias textuales; jQuery es una dependencia transversal.
- Reescribir algunas llamadas no elimina el bundle mientras DataTables, UI u otros plugins lo necesiten. DataTables minificado es más de diez veces el peso de jQuery minificado.
- Vanilla JS y jQuery corren en el mismo motor. Cambiar sintaxis no garantiza por sí solo una mejora de velocidad; consultas DB, payload, plugins y re-render pueden dominar.

**Alcance acordado:** no migrar de jQuery a Vanilla JS. Primero se repararán las funciones, plugins y scripts jQuery rotos o con defectos de ingeniería; cualquier actualización de versión se hará como cambio separado y con sus propias pruebas. La migración no forma parte de este plan.

## 5. Velocidad y estabilidad de vistas

### Ayudas existentes

- Listados por AJAX y `ERPGo.refrescar()` para varias mutaciones.
- Assets versionados con caché, gzip configurable y medición/log de peticiones lentas en ciertos modos.

### Riesgos

- El refresco parcial sustituye DOM y ejecuta scripts otra vez. DataTables, modales y listeners globales merecen verificación de inicialización única.
- DataTables es grande y algunas pantallas recrean tablas tras consultar; listados numerosos deberían paginar en servidor.
- Errores SQL de lectura se silencian en varios handlers. El resultado incompleto puede parecer “correcto”. `serviceListData` es un ejemplo de consultas `COUNT` y `Scan` ignoradas.

### Mejoras recomendadas

1. Un solo contrato de guardado seguro: token, validación, estado visual, mensaje de éxito/error y prevención de doble envío.
2. Actualizar el componente afectado sin resetear la página/filtros; mantener fallback a navegación normal.
3. Medir por endpoint latencia, tiempo DB, tamaño de respuesta y red, evitando datos personales.
4. Revisar N+1 e índices con `EXPLAIN` sobre un clon anonimizado; paginar grandes listados.
5. Revisar timeouts/contextos en consultas y llamadas externas.

**No se afirma una mejora de rendimiento:** no se ejecutó benchmark.

## 6. Zona horaria Venezuela

- Se lee `timezone` de `web_setting` en `AppSettings`, pero la búsqueda de uso no encontró que configuren `time.Local` o carguen una ubicación explícita para fechas de negocio.
- `today()` y muchas fechas usan `time.Now()` del proceso. Libros IVA/reportes usan `time.Now().Month()/Year()`; sesiones usan `time.ParseInLocation(..., time.Local)`.
- Si el proceso corre en UTC, una petición cerca de medianoche puede caer en otro día civil venezolano. La VM de esta auditoría reportó `America/Caracas` UTC−04, lo que no valida producción.

**Acción:** zona IANA `America/Caracas` explícita para fechas de negocio; UTC para instantes técnicos; cargar tzdata y probar límites de día/período. No cambiar globalmente `time.Local` sin análisis del impacto en toda la aplicación.

## 7. Matemáticas y dinero

### Controles

- `round2` centraliza redondeo a dos decimales en varias operaciones fiscales.
- Hay lógica transaccional para correlativos y reglas de cálculo en ventas/compras.
- Hay tests de regresión para retenciones y componentes de libros IVA.

### Riesgos

- `parseMoney` elimina comas, usa `fmt.Sscanf` e ignora errores. Entrada inválida puede resultar cero o aceptar un prefijo numérico; la coma decimal local puede interpretarse mal. El helper se usa en rutas financieras extensas.
- `round2` usa `float64`; cálculos encadenados pueden acumular error binario. Cambiar tipo global sin especificar política por línea/documento sería arriesgado.
- Varias conversiones `ParseFloat` ignoran errores (incluidas tasas); algunos handlers después rechazan cero, otros pueden guardar defaults silenciosos. Revisar también pagos, importes negativos, NaN/Inf y rangos.
- `invoice.go` recibe `grand_total` del cliente y lee renglones: se debe verificar sistemáticamente que el servidor recalcule subtotales, impuestos, descuentos, IGTF, pagos y saldos en todas las rutas, sin confiar en totales fiscales enviados desde navegador.
- El libro de compras tiene test para exentos, pero quedan combinaciones por probar: descuentos, alícuotas mixtas, factura/orden, moneda y anulaciones.

**Acción:** parser estricto para formatos aceptados, rechazo de errores/no finitos y validación de rango; precisión en unidades menores; redondeo fiscal explícito; pruebas de invariantes (sumas de renglón/impuesto/total, pago + saldo, conversión).

## 8. Informes fiscales e internos

| Severidad | Observación | Consecuencia |
|---|---|---|
| Alta | `reportSalesData` y `reportTax` filtran borradores, pero no `is_anulled=0`; libro de ventas y su impresión sí excluyen anuladas. | Facturas anuladas podrían aparecer en ventas/impuestos y no en el libro. Un período difiere según pantalla. Confirmar criterio legal y registrar anulaciones; no borrarlas. |
| Alta | `reportTax` oculta error de consulta, ignora `Scan` y `rows.Err()`. | Una falla de base aparece vacía o parcial sin avisar, afectando decisiones y evidencia de auditoría. |
| Media | Reportes de ventas/compras y libros ignoran errores individuales de `Scan`; algunos bucles no comprueban `rows.Err()`. | Filas omitidas pueden producir totales descuadrados sin señal. |
| Media | Límites mensuales se forman con `YYYY-MM-31`, una fecha que no existe. | Con texto ISO puede incluir todas las fechas válidas del mes por orden lexical; con columnas DATE/timestamps y distintos modos de SQL el literal inválido puede normalizarse o producir advertencia/error. No se observa aquí una exclusión concreta de filas; es una construcción frágil y dependiente del motor. Usar rango semiabierto de inicio del mes a inicio del mes siguiente. |
| Media | `auditLogData` limita a las últimas 1000 filas sin indicador visible de truncamiento en el handler revisado. | Una extracción puede confundirse con un conjunto completo. Informar total/paginación/límite. |

**Principio:** un reporte debe distinguir sin datos, datos parciales y fallo de consulta; mostrar período, criterios, moneda, excluidos y conteos; y conciliar contra la fuente. No convertir errores SQL en “sin movimientos”.

## 9. Estabilidad y fiabilidad de datos

- Hay candado anti-duplicado de compra con SHA-256/índice único y compatibilidad heredada; clasificación de factura/orden/nota de entrega; idempotencia en acciones de IA y correlativos transaccionales.
- Sesiones persistentes y limpieza de expiradas; cola IA con backpressure y estado.
- La tolerancia a errores SQL es excesiva en varios lectores; en balances, libros, informes de auditoría y datos financieros debe indicarse error en vez de aceptar parcialidad silenciosa.
- El orquestador de escrituras/auditoría puede no iniciar y queda un aviso. Verificar que operaciones críticas se bloqueen o alerten inequívocamente si la auditoría necesaria no está disponible.
- Hay código para backups, pero no se verificó que producción los ejecute, permisos, restauración ni copia externa.

**Buenas prácticas:** restauraciones periódicas aisladas, copia fuera del host, comprobación de integridad, conciliación de stock/CxC/CxP/libro mayor y anulaciones trazables.

## 10. Usabilidad

1. Mostrar errores junto al formulario y conservar valores ante 400/403/500.
2. Deshabilitar botón mientras se procesa; reactivarlo ante error. Prevenir duplicados sin ocultar rechazos.
3. Confirmar guardado y hacer navegación acorde a Guardar/Guardar y Salir, sin temporizador arbitrario.
4. Mantener filtros/paginación en tablas y actualizar saldos al modificar una fila.
5. Proporcionar ID de operación a soporte para correlacionar petición, auditoría y error sin exponer datos.
6. En CxC/CxP mostrar fecha de actualización y acceso al documento origen.
7. En informes distinguir sin movimientos, error y resultado parcial.

## 11. Registro de hallazgos y correcciones

El trabajo se hará estrictamente en el orden de esta tabla: se termina un punto, se verifica y se actualiza su estado con la evidencia; solo entonces comienza el siguiente. Ningún hallazgo se declarará resuelto por prioridad, por avanzar a otra área ni por una revisión parcial. Los puntos que requieren validación fiscal, de infraestructura o de producción permanecerán abiertos hasta obtener evidencia adecuada.

| N.º | Estado al 29-10-2026 | Hallazgo, alcance de reparación y condición para cerrar |
|---|---|---|
| 1 | **RESUELTO** | CSRF ausente en mutaciones `fetch` de reposición, conteo cíclico y tasas. Se añadió `ERPGo.csrfFetch` y se aplicó a siete POST. Test Go cubre las tres plantillas; prueba Node verifica token same-origin en POST y ausencia en GET/cross-origin. El middleware sigue exigiendo el token. |
| 2 | **RESUELTO** | Guardado de tasas: se comprueba HTTP/JSON `{error}`, se gestiona fallo de red, se muestra confirmación, se reactiva el botón, se declara POST/action de respaldo y se respeta `redirect` y submitter. Test del contrato, `node --check` del script y `webcheck` de 159 plantillas (0 errores estructurales): OK. |
| 3 | **SIGUIENTE** | Formularios dobles con serialización: inventario detectó 29 plantillas con `save_exit` y serialización. Validar handlers y corregir cada vista para enviar la acción seleccionada y respetar su destino; hacer pruebas por vista antes de cerrar. |
| 4 | **PENDIENTE** | Diferencia de criterio de facturas anuladas entre reporte de ventas/impuestos y libro de ventas. Validar regla de negocio-fiscal, mantener trazabilidad de anulaciones y probar conciliación entre pantallas. |
| 5 | **PENDIENTE** | Errores de consulta, `Scan` y `rows.Err()` silenciados en reportes financieros/fiscales, incluidos ventas, compras, stock, impuestos y cierre diario. Hacer explícitos los errores; nunca presentar datos vacíos o parciales como completos. |
| 6 | **PENDIENTE** | Límites mensuales con día fijo `31` en consultas fiscales. Cambiar los lugares identificados a rangos semiabiertos y probar febrero, meses de 30/31 días y bordes de período en motores soportados. |
| 7 | **PENDIENTE** | `parseMoney`/conversiones de montos aceptan o silencian entradas inválidas. Definir formatos permitidos (incluido formato venezolano), rechazar NaN/Inf y valores fuera de rango y probar invariantes monetarias sin regresión global. |
| 8 | **PENDIENTE** | API keys POS escritas en logs al crear/revocar/borrar. Eliminar el secreto del log, revisar acceso/retención históricos y acordar rotación de cualquier clave expuesta. |
| 9 | **PENDIENTE** | Generación CSRF devuelve token fijo de ceros si falla `crypto/rand`. Fallar cerrado, propagar el error y probar que no se emita token ni se autorice escritura ante fallo. |
| 10 | **PENDIENTE** | CORS wildcard global para `/api/` y excepción CSRF general de API/login. Inventariar clientes POS/integraciones; definir política por endpoint y probar orígenes, autenticación, CSRF y compatibilidad sin abrir datos. |
| 11 | **PENDIENTE** | Cookie de sesión sin `Secure` en login. Definir detección/configuración de HTTPS segura detrás de proxy, mantener desarrollo local funcional y probar cookies en ambos escenarios. |
| 12 | **PENDIENTE** | Falta evaluación CSP, `Referrer-Policy` y HSTS. Diseñar políticas compatibles con scripts/plugins actuales; no habilitar HSTS fuera de dominios HTTPS comprobados. Cerrar con pruebas de cabeceras y regresión de vistas. |
| 13 | **PENDIENTE** | Conducta del sistema si journal/orquestador de auditoría no puede registrar una escritura crítica. Definir garantías, hacer explícito el bloqueo/alerta requerido y probar el fallo; la operación financiera no puede quedar sin traza silenciosamente. |
| 14 | **PENDIENTE** | Backups: la revisión estática no acredita ejecución, permisos, integridad ni restaurabilidad. Comprobar procedimientos y restauración en entorno autorizado; registrar fecha y resultado sin manipular datos productivos. |
| 15 | **PENDIENTE** | Zona de negocio puede depender de la zona horaria del proceso. Aplicar `America/Caracas` a fechas civiles, UTC a instantes técnicos y probar límites de día/mes y zona configurada. |
| 16 | **PENDIENTE** | Confirmar que el servidor recalcula y valida importes de factura/compra (renglones, descuentos, IVA, IGTF, pagos y saldo), en vez de aceptar totales de navegador; cubrir tipos documentales y alícuotas con pruebas. |
| 17 | **PENDIENTE** | Estabilidad de jQuery, plugins (incluido DataTables), refresco parcial, eventos duplicados y vistas con JavaScript roto. Reparar los errores concretos y hacer pruebas de regresión. **No migrar jQuery a Vanilla JS** dentro de esta adecuación. |
| 18 | **PENDIENTE** | Rendimiento sin medición suficiente: medir latencia, consultas DB y payload por vistas; revisar N+1/índices con datos anonimizados y optimizar solo después de establecer base y comparar. |
| 19 | **PENDIENTE** | Archivos estáticos potencialmente duplicados/no usados. Inventariar referencias y hashes; no borrar nada hasta demostrar que no tiene consumidores y validar build. |
| 20 | **PENDIENTE** | Historial de auditoría limitado a 1000 filas sin indicador claro de truncamiento. Añadir conteo/paginación/advertencia de límite y probar que exportaciones no se confundan con un historial completo. |
| 21 | **PENDIENTE** | Usabilidad/robustez general: errores visibles, conservar entrada ante fallo, estado de guardado, prevención de doble envío, filtros/paginación y saldos actualizados. Revisar pantalla por pantalla, con criterios verificables; no afirmar cobertura global a partir de muestras. |

La investigación de la lista y las reparaciones avanzan una por una, en orden numérico. Esta lista cubre hallazgos de código y verificaciones pendientes de entorno; por ello algunos puntos solo se podrán cerrar con acceso autorizado a infraestructura/producción, sin simular esa evidencia.

## 12. Decisión sobre jQuery

No se hará migración de jQuery a Vanilla JS en esta adecuación. Se repararán sus funciones, plugins, scripts y fallos de ingeniería según hallazgos concretos; cada punto se probará antes de cerrar. Una evaluación de actualización/migración podrá proponerse después como trabajo separado. El orden y los criterios de cierre están en el [registro de seguimiento](seguimiento-auditoria-tecnica-2026-10-29.md).

## 13. Verificación y limitaciones

- Revisión estática de plantillas, handlers, middleware, reportes, matemáticas, dependencias y activos.
- Conteos de checkout: 159 plantillas HTML; 87 contienen `<form>`; 80 incluyen `type="submit"`; 117 usan `$()`/`jQuery()`; 443 activos rastreados y ~26,4 MB.
- Fecha local de la VM: America/Caracas UTC−04. No verifica servidores de producción.
- Build `go build ./...`: **OK**.
- Vet `go vet ./interfaces/web`: **OK**.
- Suite `go test ./...`: **OK**.
- `go run ./internal/webcheck -dir templates`: **159 plantillas revisadas, 0 errores estructurales** (15 requieren datos reales para evaluación completa).
- Tests enfocados para puntos 1 y 2: **OK**.
- `node --check static/js/csrf.js` y compilación sintáctica del JS de tasas/reposición/conteo: **OK**. También se ejecutó una prueba Node del helper CSRF para POST same-origin, GET y POST externo: **OK**.
- `git diff --check`: **OK**.
- El punto 3 es el siguiente; el inventario localizó 29 vistas con botón doble y serialización, pero aún no se han corregido ni verificado todas.
- No se realizó prueba visual en navegador ni benchmark. Esto no es certificación fiscal ni auditoría de producción.

## Fuentes técnicas

- Política oficial de soporte jQuery: https://jquery.com/support/ (indica que jQuery 3.x recibe solo parches críticos y recomienda la rama 4.x).
- Guía oficial de migración jQuery 4: https://jquery.com/upgrade-guide/4.0/ (recomienda Migrate, probar y resolver avisos antes de retirarlo).
- Descargas jQuery: https://jquery.com/download/
- OWASP, prevención CSRF: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html (tokens para mutaciones y verificación de origen como defensa en profundidad).
- OWASP WSTG, pruebas CORS: https://wstg.owasp.org/latest/4-Web_Application_Security_Testing/11-Client-side/07-Cross_Origin_Resource_Sharing/
- Go `time` y zonas horarias: https://pkg.go.dev/time y https://go.dev/src/time/zoneinfo.go

---

**Conclusión:** ERPGo incluye mecanismos deliberados de seguridad e integridad, pero algunos caminos de UI no cumplen sus contratos y ciertos reportes silencian errores. La respuesta correcta no es quitar seguridad ni migrar toda la UI por intuición: hay que reparar los POST, hacer visibles errores y resultados parciales, armonizar informes y medir antes de optimizar. Este informe documenta hallazgos y acciones; no certifica una instalación ni el cumplimiento fiscal.
