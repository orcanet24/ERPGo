# ia_dev/finetune — Generador del dataset de informes para fine-tuning

Convierte el material real de ERPGo (plantillas de `report_template` de la base
empresa + catálogo de módulos del MCP + esquema real de la base) en el dataset
JSONL del modelo especialista en informes (QLoRA sobre Gemma):
`ia_dev/finetune/salida/`.

## Qué produce

| Archivo           | Contenido                                                              |
|-------------------|------------------------------------------------------------------------|
| `finetune.jsonl`  | Filas de entrenamiento formato `messages` (system/user/assistant).     |
| `plantillas.jsonl`| Semilla: TODA plantilla exportada, con módulo y motivo de exclusión.    |
| `modulos.jsonl`   | Catálogo de módulos de la única fuente (`ai_schema_modules.go`).        |
| `esquema.jsonl`   | Esquema real por módulo introspectado en vivo desde la base empresa.   |

Cada fila de `finetune.jsonl` imita el prompt REAL de producción de
`ai_reports.go` (`iaPromptInforme`): system = `aiContratoInforme` extraído del
archivo fuente ese mismo día; user = solicitud + esquema del módulo; assistant
= el JSON EXACTO guardado en `report_template.definition`.

## Cómo correr

```bash
# SQLite (lo normal en instalación local)
go run ./ia_dev/finetune -db data/empresas/default.sqlite3

# MySQL
go run ./ia_dev/finetune -dsn "usuario:clave@tcp(127.0.0.1)/erpgo_empresa"

# Variantes de pregunta por plantilla (1-3, defecto 2) y semilla determinista
go run ./ia_dev/finetune -db data/empresas/default.sqlite3 -variantes 3 -semilla 42
```

## Candados que no se negocian

1. **El contrato no se copia.** Se extrae de `interfaces/web/ai_reports.go`
   (`aiContratoInforme`): si el contrato cambia en producción, la próxima
   corrida del generador produce el dataset con el nuevo. La prueba
   `TestContratoDelRepoReal` lo verifica contra el archivo real del repo.
2. **Las tablas fiscales no van al dataset.** La lista de
   `validateReportSQLwithSchema` (tablas fiscales prohibidas y palabras
   fiscales sin agregación) se replica en `exportar.go` `sancionarSQL`, y la
   prueba `TestTablasFiscalesAlineadasConElGuardianReal` comprueba contra el
   archivo REAL que el generador y el guardián siguen alineados. Lo que el
   guardián rechazaría NO entra a `finetune.jsonl`: va a `plantillas.jsonl`
   con su motivo, para decisión humana.
3. **Definiciones nulas no se exportan** (tal como `validateReportDefinition`
   se aplica al guardar): SQL presente, ≥1 columna, totals dentro de columns,
   chart que lea solo columnas declaradas.

## Límites documentados

- En SQLite no existen comentarios de columna, así que `esquema.jsonl` solo
  trae nombre/tipo; en MySQL llegan con la documentación de negocio
  (information_schema).
- Las preguntas se arman SOLO con palabras ya presentes en la fila (nombre,
  descripción, módulo). La generación de paráfrasis más ricas con LLM grande
  es la fase posterior del plan, con estas plantillas como material fuente.
- El generador NO ejecuta el guardián SQL completo del servidor en vivo (ese
  vive en `interfaces/web` sin exportar): la barrera completa ocurre en
  producción, al ejecutar cada informe; este proyecto añade los tres
  invariantes del guardián estructural antes de dejar algo en el dataset.

## Operación prevista

- Generar con la base empresa de cada tenant; cada empresa pequeña aporta
  sus propias plantillas (el dataset se combina entre tenants al aire).
- Añadir plantillas manuales de calidad a `plantillas.jsonl` y regenerar, o
  usar la semilla con el LLM grande de Configuración IA para expandir
  variantes descritas antes de entrenar.
- El subconjunto de TEST (10-15%) sale de cortar `finetune.jsonl` en el
  pipeline de entrenamiento (queda documentado en el plan del proyecto).
