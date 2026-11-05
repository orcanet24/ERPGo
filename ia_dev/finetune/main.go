package main

// ia_dev/finetune — Generador del dataset para el fine-tuning (LoRA/QLoRA) del
// modelo especialista en informes de ERPGo.
//
// Qué produce (en el directorio -salida):
//
//   finetune.jsonl   filas de entrenamiento formato messages (system/user/
//                    assistant), imitando el prompt REAL que ERPGo le envía al
//                    proveedor (contrato + solicitud + esquema del módulo).
//                    Cada plantilla real de report_template genera hasta
//                    -variantes preguntas distintas.
//   plantillas.jsonl semilla de pares pregunta↔definición con metadatos
//                    (módulo, creación y motivo de exclusión si lo hubo). Es
//                    el insumo para revisión humana y para la fase posterior
//                    de paráfrasis con LLM grande.
//   modulos.jsonl    catálogo de módulos leído de la FUENTE ÚNICA
//                    (interfaces/web/ai_schema_modules.go), no una copia.
//   esquema.jsonl    esquema real de cada módulo leído en vivo de la base
//                    empresa (SQLite: PRAGMA table_info; MySQL:
//                    information_schema, con comentarios documentados).
//
// Dos reglas evitan enseñarle al modelo lo que el guardián rechazará:
//
//   1. El contrato NO se copia: se extrae de ai_reports.go (aiContratoInforme)
//      y el catálogo de ai_schema_modules.go, igual que lo que producción usa.
//   2. Las plantillas que el guardián rechazaría (tablas fiscales o palabras
//      fiscales sin agregación) NO entran a finetune.jsonl: van a
//      plantillas.jsonl con el motivo, para decisión humana.
//
// Uso:
//
//   go run ./ia_dev/finetune -db data/empresas/default.sqlite3
//   go run ./ia_dev/finetune -dsn "usuario:clave@tcp(127.0.0.1)/erpgo_empresa"

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configCorrida integra los parámetros de una corrida (typedef para pruebas).
type configCorrida struct {
	rutaBD           string // ruta del .sqlite3 de la empresa (si no hay -dsn)
	dsnMySQL         string // DSN MySQL; vacío = usar SQLite
	repoRoot         string // raíz del repo: dónde están interfaces/web
	directorioSalida string // directorio de salida de los .jsonl
	variantes        int    // nº de filas de entrenamiento por plantilla (1-3)
	semilla          int64  // semilla determinista de las variantes
}

func main() {
	cfg := configCorrida{}
	flag.StringVar(&cfg.rutaBD, "db", "data/empresas/default.sqlite3", "ruta del archivo .sqlite3 de la empresa")
	flag.StringVar(&cfg.dsnMySQL, "dsn", "", "DSN MySQL (usuario:clave@tcp(host)/base): si se da, -db se ignora")
	flag.StringVar(&cfg.repoRoot, "repo", ".", "raíz del repositorio ERPGo (para leer ai_reports.go y ai_schema_modules.go)")
	flag.StringVar(&cfg.directorioSalida, "salida", "ia_dev/finetune/salida", "directorio de salida de los .jsonl")
	flag.IntVar(&cfg.variantes, "variantes", 2, "nº de filas de entrenamiento por plantilla (1-3)")
	flag.Int64Var(&cfg.semilla, "semilla", 7, "semilla determinista del orden de las variantes")
	flag.Parse()

	if cfg.variantes < 1 || cfg.variantes > 3 {
		fmt.Fprintln(os.Stderr, "-variantes debe estar entre 1 y 3")
		os.Exit(2)
	}
	if cfg.dsnMySQL == "" && cfg.rutaBD == "" {
		fmt.Fprintln(os.Stderr, "hace falta -db (SQLite) o -dsn (MySQL)")
		os.Exit(2)
	}

	resumen, err := ejecutar(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	fmt.Println(resumen)
}

// ejecutar enciende todo el pipeline. Devuelve el resumen legible de la
// corrida. Está separada de main() para que las pruebas la llamen directo con
// una base de prueba propia.
func ejecutar(cfg configCorrida) (string, error) {
	// 1. Contrato y catálogo: SIEMPRE de los archivos fuente del repo, nunca
	// de copias en este directorio (una copia diverge sin que nadie lo sepa).
	contrato, err := extraerContrato(cfg.repoRoot)
	if err != nil {
		return "", err
	}
	modulos, err := leerCatalogoModulos(cfg.repoRoot)
	if err != nil {
		return "", err
	}

	// 2. La base real de la empresa.
	db, motor, cerrar, err := abrirDB(cfg)
	if err != nil {
		return "", err
	}
	defer cerrar()

	plantillas, avisos, err := exportarPlantillas(db, modulos)
	if err != nil {
		return "", err
	}

	// 3. Esquema real de cada módulo con tablas presentes en ESTA base.
	tablasBD, err := listarTablasBD(db, motor)
	if err != nil {
		return "", err
	}
	esquemas, err := esquemaDeModulos(db, tablasBD, modulos, motor)
	if err != nil {
		return "", err
	}
	// El esquema que va DENTRO de cada user prompt, en JSON compacto, por
	// módulo. Se arma UNA vez y se comparte: el modelo aprende a leer el
	// esquema como lo leerá en producción (mismo canal, misma sintaxis).
	esquemaDePrompt, err := esquemasComoPromptJSON(esquemas)
	if err != nil {
		return "", err
	}

	// 4. Escribir los cuatro .jsonl.
	if err := os.MkdirAll(cfg.directorioSalida, 0o755); err != nil {
		return "", fmt.Errorf("crear directorio de salida: %w", err)
	}
	nAceptadas, nExcluidas, nEntreno := 0, 0, 0

	plantillasF, err := os.Create(filepath.Join(cfg.directorioSalida, "plantillas.jsonl"))
	if err != nil {
		return "", fmt.Errorf("crear plantillas.jsonl: %w", err)
	}
	entrenoF, err := os.Create(filepath.Join(cfg.directorioSalida, "finetune.jsonl"))
	if err != nil {
		plantillasF.Close()
		return "", fmt.Errorf("crear finetune.jsonl: %w", err)
	}
	modulosF, err := os.Create(filepath.Join(cfg.directorioSalida, "modulos.jsonl"))
	if err != nil {
		plantillasF.Close()
		entrenoF.Close()
		return "", fmt.Errorf("crear modulos.jsonl: %w", err)
	}
	esquemasF, err := os.Create(filepath.Join(cfg.directorioSalida, "esquema.jsonl"))
	if err != nil {
		plantillasF.Close()
		entrenoF.Close()
		modulosF.Close()
		return "", fmt.Errorf("crear esquema.jsonl: %w", err)
	}
	defer plantillasF.Close()
	defer entrenoF.Close()
	defer modulosF.Close()
	defer esquemasF.Close()

	for _, m := range modulos {
		fila := map[string]interface{}{
			"key":         m.Key,
			"etiqueta":    m.Label,
			"descripcion": m.Description,
			"tablas":      m.Tables,
		}
		if err := escribirJSONL(modulosF, fila); err != nil {
			return "", err
		}
	}
	for _, e := range esquemas {
		// La nota de limitación solo va cuando ES SQLite: en MySQL los
		// comentarios de campo existen y llegan, y una nota que no aplica
		// solo siembra ruido en el dataset.
		fila := map[string]interface{}{
			"modulo":   e.Modulo,
			"etiqueta": e.Etiqueta,
			"tablas":   e.Tablas,
		}
		if motor == "sqlite" {
			fila["nota_sqlite"] = notaLimitacionSQLite
		}
		if err := escribirJSONL(esquemasF, fila); err != nil {
			return "", err
		}
	}

	for _, p := range plantillas {
		// DE TODA plantilla exportada va la semilla; de las aceptadas, además,
		// su definición completa para revisión y paráfrasis posterior.
		fila := map[string]interface{}{
			"id":          p.ID,
			"nombre":      p.Nombre,
			"descripcion": p.Descripcion,
			"modulo":      p.Modulo,
			"creada":      p.Creada,
		}
		if p.MotivoExclusion != "" {
			fila["exclusion"] = p.MotivoExclusion
			nExcluidas++
		} else {
			fila["definicion"] = p.DefinicionRaw
			nAceptadas++
		}
		if err := escribirJSONL(plantillasF, fila); err != nil {
			return "", err
		}
		if p.MotivoExclusion != "" {
			continue // lo que el guardián rechaza NO entra al entrenamiento
		}
		filas := variantesDePregunta(p, cfg.semilla, cfg.variantes)
		for _, pregunta := range filas {
			if err := escribirJSONL(entrenoF, filaEntreno(contrato, esquemaDePrompt[p.Modulo], pregunta, p.DefinicionRaw)); err != nil {
				return "", err
			}
			nEntreno++
		}
	}

	var res strings.Builder
	res.WriteString(fmt.Sprintf(
		"Plantillas totales: %d · aceptadas: %d · excluidas por el guardián: %d · filas de entrenamiento: %d\n",
		len(plantillas), nAceptadas, nExcluidas, nEntreno))
	res.WriteString(fmt.Sprintf("Módulos del catálogo: %d · tablas en la base: %d · esquemas con tablas en esta base: %d\n", len(modulos), len(tablasBD), len(esquemas)))
	if strings.TrimSpace(avisos) != "" {
		res.WriteString("Avisos de exclusión:\n" + avisos + "\n")
	}
	return strings.TrimRight(res.String(), "\n"), nil
}

// esquemasComoPromptJSON deja el esquema de cada módulo en JSON compacto,
// listo para inyectarse en el user prompt. Por cada módulo con tablas se
// produce {modulo, etiqueta, tablas:[{tabla, campos:[...]}]}.
func esquemasComoPromptJSON(esquemas []esquemaModuloJSON) (map[string]string, error) {
	out := map[string]string{}
	for _, e := range esquemas {
		raw, err := json.Marshal(e)
		if err != nil {
			return nil, fmt.Errorf("esquema del módulo %s: %w", e.Modulo, err)
		}
		out[e.Modulo] = string(raw)
	}
	return out, nil
}

const notaLimitacionSQLite = `SQLite no guarda comentarios de campo: la documentación de negocio del esquema solo está disponible en MySQL (information_schema).`
