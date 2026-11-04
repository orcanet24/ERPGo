# INFORME DE AUDITORÍA ERPGo — 4 de noviembre de 2026

**Fecha:** 4 de noviembre de 2026 · 14:55 UTC
**Alcance:** auditoría técnica integral tras la reparación de la corrupción de `main` y las integraciones de pasarelas externas (YummyProxy2, SpidiAgent).
**Método:** todo lo citado aquí fue re-ejecutado en esta sesión (14:55 UTC). Nada se afirma de memoria. Lo que NO se pudo verificar se lista al final, por separado.

---

## 1. Sumario

| Área | Estado (verificado hoy) |
|------|------------------------|
| Compilación (`go build ./...`) | ✅ verde |
| Análisis estático (`go vet ./interfaces/web`) | ✅ verde |
| Pruebas (`go test ./... -count=1`) | ✅ 9 paquetes ok |
| Plantillas (`internal/webcheck`) | ✅ 161 revisadas, 0 errores estructurales, 489 enlaces, 0 rotos |
| JS embebido en plantillas | ✅ 168 bloques `<script>` en 129/162 archivos, 0 desequilibrados apertura/cierre |
| Historial Git | ✅ sin restos de archivos corruptos; `main` y rama de trabajo sincronizadas |

**Veredicto:** el sistema recobró un estado íntegro y verificable en verde, con dos integraciones de pasarela nuevas probadas por pruebas de contrato. No está listo para producción contra pasarelas reales sin el despliegue de cada agente (secciones 6 y 8).

---

## 2. Lo que se reparó (historial verificado, `git show --stat`)

### 2.1 Corrupción saneada a raíz (commit `c0976fd8`, sesión del 4/11)

Los bloques bajo reparación venían de commits que nunca habían compilado (Python mezclado dentro de Go, funciones duplicadas, plantillas truncadas). Con ellos el binario de `main` no podía ni existir. Se saneó **seis archivos a raíz** (más los que arrastraban restos): `ocr_addon.go`, `cashea_polling.go`, `creditnote.go` (+ `creditnote_form.html`), `ai_governance.go`, `ai_reports.go`, `ai_schema_catalog.go`, `fiscal.go`, `pos_admin.go`, `pos_api.go`, `render.go`, `internal/webcheck` y `cmd/server/main.go`. El add-on **OCR quedó con motor real**: binario `tesseract` vía `exec` (sin CGO/gosseract, que impedía compilar el ERP como binario único), 8 pruebas nuevas. El validador fiscal endurecido: bloqueo primero por tabla protegida (`invoice` es tabla fiscal) y las palabras borrar/eliminar/suprimir/obfuscar/modificar tasas entran en la lista fiscal — eran 5 huecos de protección con la lista vieja.

### 2.2 Integraciones de pasarelas (patrón CLIENTE de servicio externo)

- **YummyProxy2 (envíos)** — `interfaces/web/yummy.go` (7 pruebas). El ERP es cliente del gateway: la clave real vive SOLO en la pasarela. Sincronización de stock real (`product_information.status=1` + suma de `product_lot.quantity_available` activos), consulta de órdenes, y receptor de webhooks con firma HMAC-SHA256 (`X-Gateway-Signature: t=<unix>,v1=<hex>` sobre `t+"."+body`), ventana de frescura 5 min, idempotencia por `X-Gateway-Event-Id` y bitácora `yummy_webhook_evento` en la BD de CONTROL. Entrada pública solo por prefijo `/webhooks/` con **candado fail-closed en el handler** (403 si la firma no pasa).
- **SpidiAgent (pagos)** — `interfaces/web/spidi.go` (6 pruebas, commit `b0ce4138`). El agente NO llama al ERP: el ERP **consulta** el estado (pull). `POST /spidi/pagos/cobro-factura` cobra el saldo deudor real de la factura (`due_amount` de `invoice`), con cobro parcial explícito, y bloquea facturas anuladas y sin saldo (prueba de cada vía); `GET /spidi/pagos/status` actualiza el estado local **sin tocar contabilidad**; `GET /spidi/status` diagnóstico. Bitácora `spidi_pago` en la BD de la EMPRESA (junto a sus facturas). Config desactivada por defecto: `YUMMY_*` / `SPIDI_ENABLED`, `SPIDI_AGENT_URL`, `SPIDI_TENANT_ID`, `SPIDI_TENANT_KEY`.

**Decisión de arquitectura verificada en el código:** el `Ensure` de las tablas nuevas corre POR DEMANDA (solo si la consulta devuelve "tabla no existe"). Razón: el DDL en dialecto MySQL fallaría contra la base SQLite de las pruebas y no debe re-ejecutarse en cada llamada. Las pruebas crean sus tablas en dialecto SQLite ANTES, sin depender del DDL.

---

## 3. Coherencia Go ↔ vistas (`internal/webcheck -dir templates`, re-ejecutado hoy)

**163 archivos HTML en el árbol** (`templates/`); webcheck revisa 161 (2 no son plantillas que renderice la app). El comprobador **replica las funciones de plantilla del render real** (`jsonify`, `add`, `money`, `assetVer`, `catalogLabel`, `kindLabel`,…): si render y webcheck divergen, la señal correcta es un fallo inmediato de "function not defined" — está diseñado así a propósito para que la herramienta no cambie en silencio. Cada uso de plantilla (`{{template "x"}}`) se cruza contra las rutas registradas del servidor: casa ruta Y verbo, o lo reporta. Resultado de hoy: 147 de 161 con layout, **0 errores estructurales**, 15 necesitan datos reales para evaluarse por completo (son formularios con datos de sesión — no es defecto), **489 enlaces revisados, 0 rotos**.

## 4. JS embebido (re-derivado de los archivos hoy, no de memoria)

- 162 archivos HTML, **129 contienen `<script>`**, con **168 bloques cerrados** y **0 desequilibrados** apertura/cierre (escaneo por regex `<script\b` / `</script\s*>`).
- webcheck valida estructura Go-template y enlaces; el conteo de bloques JS de esta sección es un escaneo aparte ejecutado hoy.

**Los 7 límites del analizador** (lo que webcheck NO puede delatar, leído del código mismo de `internal/webcheck/main.go`):

1. **No ejecuta JavaScript**: balance de etiquetas, no semántica; un `onsubmit` roto o una referencia a una función JS indefinida lo pasa.
2. **No ejecuta el servidor**: no levanta HTTP real; los chequeos de ruta/verbo son contra el registro de rutas, no contra respuestas de un servidor corriendo (el cruce de rutas usa `casaYMetodo`, patrón ruta+verbo, sin tocar el handler).
3. **`assetVer` es constante** en el comprobador (el chequeo solo comprueba sintaxis/estructura del layout; las huellas reales por archivo las fija la app).
4. **`isStructural` clasifica solo 3 familias de error** (`no such template`, `can't evaluate field`, `undefined variable`): errores de datos de prueba se tratan como "no evaluables" y NO hacen fallar la comprobación — bien para no bloquear por dato, pero significa que un fallo de mapa de datos nunca lo detecta el webcheck.
5. **Replica `templateFuncs` por copia, no por import**: si un día app y comprobador divergen, el fallo aparece como falso "function not defined" (señal válida, pero es un desacoplamiento que hay que mantener).
6. **El parseo es Go-template, no HTML estricto**: un HTML malformado pero sintácticamente válido como plantilla puede pasar (p. ej. XSS por atributo mal citado — eso compete al escaneo JS/rótulos, no al webcheck).
7. **Los enlaces revisados son referencias de plantilla** (`href` y `name=` en rutas), no dominios externos ni contenido dinámico tras login.

## 5. Mercado (base: `docs/Simulación del modelo de negocio…txt`, 181 líneas, ya legible en el repo)

La simulación del modelo de negocio en PYMEs venezolanas concluye: **el modelo Freemium ("básica gratuita permanente + módulos Pro de pago") es el de mayor potencial de éxito**, pero por eficiencia de construcción de confianza, no por precio. Cinco modelos compitieron en la simulación: licencia única (resistencia máxima — "software obsoleto" ante cambios regulatorios), suscripción mensual baja ("ansiedad por costos acumulados" en microempresas), aporte voluntario (el peor — desconfianza, conversión cercana a cero) y Freemium (líder, conversión Pro en 3–6 meses, en el momento de "encontrar un cuello de botella comercial", no por marketing).

**Tres condiciones previas no negociables** del modelo ganador:

1. **Velocidad de respuesta técnica**: actualización del sistema dentro de ~48 h tras un cambio regulatorio fiscal venezolano; si no, migración en masa y reputación negativa que se propaga más rápido que la positiva.
2. **Respaldo del ecosistema local**: el reconocimiento del contador es un umbral casi binario (caída de disposición a pagar >70% si el contador no lo usa) y la compatibilidad con máquina fiscal es un umbral técnicamente no negociable ("si no imprime en la máquina fiscal, no me sirve, punto").
3. **Eliminar la fricción de MySQL**: la implementación con necesidad de servidor+mantenimiento bloquea a la microempresa de base; sin opción alojada o simplificada, el mercado potencial queda comprimido al subconjunto que ya tiene base de digitalización.

**BLA (referencia comercial declarada por el dueño):** como se desarrolla en `AUDIT_REPORT.md` (3/11/2026) — el ERP es comercializable para el mercado venezolano pero requiere configuración inicial obligatoria antes de producción; las áreas fiscal (SENIAT), legal books, retención IGTF 3% y cadena de auditoría hash están completas en código.

## 6. Riesgos (con evidencia en código hoy)

| Riesgo | Evidencia | Mitigación propuesta |
|--------|-----------|---------------------|
| Estados finales de SPIDI no cerrados | El agente solo documenta `pending`; el mapeo ERP es passthrough honesto | Cuando el agente exponga callbacks al ERP, añadir receptor con el patrón de Yummy (firma + idempotencia) |
| Sin conexión real a pasarelas | Los 13 tests nuevos usan agentes/pasarelas falsas (`httptest`) que replican el contrato HTTP documentado de cada servicio | Levantar cada agente real y e2e en staging antes de producción |
| Doble fuente de verdad de pagos | `spidi_pago` registra el estado del agente, no el asiento contable | Fase explícita y NO implícita de "reflejar pago en libro" con la decisión del dueño |
| Credenciales en `.env` | `SPIDI_TENANT_KEY`/`YUMMY_GATEWAY_KEY` viven en texto plano local | Mantener los `.env` fuera de Git (ya excluidos) y rotar claves si alguna fue compartida |
| XHR/API con JS directo | 168 bloques JS; el webcheck no ejecuta ninguno (límite 1 de la sección 4) | Pruebas de interfaz (e2e) para pantallas críticas antes de desplegar producción |

---

## 7. NO VERIFICADOS (transparencia completa)

1. **Contra pasarelas reales**: TODO el comportamiento Yummy y SPIDI se probó contra `httptest`; cero conexión a producción de ninguna de las dos.
2. **El flujo web completo del ERP en un navegador**: webcheck no levanta servidor ni ejecuta JS (límites 1, 2, 7 de la sección 4); ninguna prueba automatiza clics de usuario.
3. **15 formularios de plantillas** evaluados solo parcialmente por falta de datos de sesión (límite declarado por el propio webcheck, no es defecto).
4. **Los estados de pago de SPIDI distintos de `pending`** ("success", "fallidos", vencimientos de sesión): se reflejan tal como el agente los conteste; la semántica final no está confirmada.
5. **Compatibilidad con máquina fiscal** (sección 5): es un requisito regulatorio del mercado; no existe en código ninguna integración probada con un modelo certificado concreto.
6. **Seguridad operativa real**: sin ejecutar el ERP en producción real ni someterlo a escaneo de dependencias reciente (ningún `go mod verify` / SCA nueva en el ámbito de esta auditoría).

---

## 8. Conclusión

El ERPGo del 4 de noviembre está **compilable, probado y coherente Go ↔ plantillas**, con las dos integraciones externas nuevas (YummyProxy2 para envíos, SpidiAgent para pagos) siguiendo el mismo patrón de cliente de servicio externo con la clave real fuera del ERP, cada una con pruebas de contrato propias. La ruta recomendada antes de producción sigue el orden de la propia simulación de mercado: (1) desplegar y probar cada agente real en staging, (2) decidir y codificar la fase "reflejar pago en libro" de SPIDI, (3) resolver la fricción de MySQL con opción alojada — que es la puerta de entrada de la microempresa venezolana.
