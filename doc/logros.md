# Logros técnicos de ERPGo — Registro verificable

**Titular:** Orcanet1724 C.A · **Actualizado:** 1 de octubre de 2026

Este documento registra, con evidencia de prueba, los hitos técnicos del
Sistema. No es material comercial: es el respaldo de lo que se ofrece.

---

## 1. Localización fiscal Venezuela (completa y verificada)

| Logro | Evidencia |
|-------|-----------|
| RIF validado con algoritmo oficial (módulo 11 SENIAT) en clientes, proveedores, facturas, compras y POS | Tests unitarios (`fiscal_test.go`): J31335212-8, V12345678-1 válidos; rechaza dígitos verificadores incorrectos |
| Número de control fiscal asignado por serie, atómico y consecutivo | `00-00000001`, `00-00000002`... bajo transacción con bloqueo; verificado e2e |
| Series fiscales FACT/NC/ND/RET-IVA/RET-ISLR editables desde UI | Ajustes → Fiscales (SENIAT) |
| Notas de crédito fiscales con asientos contables automáticos | Emitida e2e: NC-00000001, reversión de ingreso/IVA, impresión standalone |
| IGTF 3% automático en pagos en divisa/cripto, percibido al cliente | Venta Zelle 100 → IGTF 3.09 cobrado (cobro real 103), voucher contable por la percepción |
| 7 métodos de pago venezolanos con cuentas contables predefinidas | efectivo→Caja; punto/pago móvil/transferencia/Zelle/cripto→Banco; crédito→Clientes |
| Caja por turnos: apertura, Reporte X, Cierre Z con arqueo | Diferencia de arqueo genera comprobante contable automático |
| Libro de Ventas y Libro de Compras con RIF/control reales, ex-anuladas, NCs incluidas | Salidas verificadas contra datos e2e |
| Kárdex Art. 177 RISLR, TXT Retenciones IVA (16 columnas SENIAT), XML Retenciones ISLR | Módulos en producción |
| Seguridad: la sesión de caja se resuelve SIEMPRE en el servidor | Nunca se confía en el valor del formulario |

## 2. Soporte dual de base de datos (SQLite ↔ MySQL)

| Logro | Evidencia |
|-------|-----------|
| **Cero-configuración**: solo ejecutar el binario crea BD + admin + esquema | Arranque SQLite: 253 statements, 0 warnings, admin `admin/Admin@2026` |
| SQLite con **WAL + BEGIN IMMEDIATE + busy_timeout 10s** | Multiusuario sin bloqueos: la BD misma es la cola de escritura FIFO, sin cola en memoria que perder en un reinicio |
| Migraciones idempotentes traducidas MySQL→SQLite al vuelo | PK plegadas, KEY→CREATE INDEX, INSERT OR IGNORE, cláusulas MySQL eliminadas |
| **Migración SQLite→MySQL desde la UI** (Ajustes → Base de Datos) | e2e: 95 tablas, 248 filas copiadas; fechas exactas preservadas (fix de desfase UTC corregido y verificado) |
| El sistema escribe su propio `.env` al migrar | Reinicio siguiente arranca en MySQL automáticamente |
| Continuidad fiscal entre motores | Primera venta tras migrar: control `00-00000003` — la serie sigue sin reiniciarse |
| El archivo SQLite se **archiva, nunca se destruye** | `data/backup/erpgow.sqlite3-retired-<fecha>` — historial contable del negocio |
| Compatibilidad inversa: instalación MySQL existente no se rompe | Default inteligente: sin `.env` → SQLite; con `.env` → MySQL |

## 3. Benchmarks de concurrencia (orquestador real en Go)

Metodología: `tools/loadtest` lanza N usuarios HTTP simultáneos reales; cada
uno hace ráfagas de lecturas (POS, listados, reportes) y **ventas completas**
(transacción con factura, asientos contables y numeración fiscal). No son
consultas sueltas: es la operación real del negocio. Servidor y prueba sobre
SQLite (el modo por defecto del binario gratuito).

| Escenario | Operaciones totales | Fallos | Latencia de venta | Throughput |
|---|---|---|---|---|
| **4 cajeros simultáneos** (caso típico PYME) | 800 lecturas + 40 ventas | **0** | **12 ms** promedio · p95 36 ms · máx 43 ms | 47 ventas/seg |
| **10 terminales** | 500 lecturas + 100 ventas | **0** | 58 ms promedio · p95 137 ms · máx 458 ms | **84 ventas/seg** |
| **25 terminales** (stress) | 300 lecturas + 100 ventas | **0** | 186 ms promedio · p95 310 ms · máx 499 ms | 61 ventas/seg |

**Integridad: 240/240 ventas registradas, IDs consecutivos 1000→1239 sin
huecos — ni una venta perdida ni duplicada**, aunque 25 escritores concurrentes
se pisaran. Latencia de lectura en página: 3-4 ms promedio bajo carga.

**Conclusión:** el binario gratuito soporta con holgura el multiusuario de un
negocio pequeño (hasta ~10 terminales fluidas) y degrada gradualmente, sin
errores ni pérdidas, hasta decenas. Superado ese punto, la migración a MySQL
es un botón. Reproducible:

```bash
go run ./tools/loadtest                                  # 4 cajeros por defecto
CAJEROS=10 VENTAS=10 LECTURAS=5 go run ./tools/loadtest  # carga a medida
```

## 4. Verificación continua

- `go build ./...` · `go vet ./...` · `go test ./...` — todos en verde.
- Binario de producción `erpgow.exe` reconstruido con cada hito verificado.
- Bitácoras de fase: [bitacora-localizacion-fiscal.md](bitacora-localizacion-fiscal.md),
  [bitacora-sqlite-mysql.md](bitacora-sqlite-mysql.md).
