// Package logger proporciona logging configurable por entorno.
//
// Variables de entorno:
//   - LOG_ENABLED: activa/desactiva logs (0=off, 1=on). Default: 0
//   - LOG_LEVEL: nivel mínimo (debug, info, warn, error). Default: info
//   - LOG_FILE: ruta del archivo de log. Default: logs/erpgow.log
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelNone
)

var (
	mu         sync.RWMutex
	level      = LevelInfo
	enabled    = false
	outputFile string
	file       *os.File
	logger     *log.Logger
)

// Init inicializa el logger desde variables de entorno.
func Init(enabled bool, levelStr, filePath string) {
	mu.Lock()
	defer mu.Unlock()

	if !enabled {
		enabled = false
		level = LevelInfo
		logger = nil
		return
	}

	enabled = true
	level = parseLevel(levelStr)

	if filePath == "" {
		filePath = "logs/erpgow.log"
	}
	outputFile = filePath

	// Crear directorio si no existe
	if dir := filepath.Dir(filePath); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	// Abrir archivo
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Fallback a stderr
		file = nil
		logger = log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)
		return
	}
	file = f

	// Salida dual: archivo y consola
	mw := io.MultiWriter(f, os.Stdout)
	logger = log.New(mw, "", log.LstdFlags|log.Lshortfile)

	// Redirigir log estándar
	log.SetOutput(mw)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	logger.Printf("[LOGGER] inicializado → %s (nivel: %s)\n", filePath, levelStr)
}

// Close cierra el archivo de log.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

// parseLevel convierte string a nivel.
func parseLevel(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	}
	return LevelInfo
}

// logf escribe si el nivel es suficiente y el logger está activo.
func logf(lvl int, format string, v ...interface{}) {
	mu.RLock()
	l := logger
	en := enabled
	cur := level
	mu.RUnlock()

	if !en || lvl < cur || l == nil {
		return
	}

	prefix := map[int]string{
		LevelDebug: "[DBG]",
		LevelInfo:  "[INF]",
		LevelWarn:  "[WRN]",
		LevelError: "[ERR]",
	}[lvl]

	// Formato con timestamp legible
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, v...)
	l.Output(3, fmt.Sprintf("%s %s %s", ts, prefix, msg))
}

// Debug logs a debug-level message.
func Debug(format string, v ...interface{}) { logf(LevelDebug, format, v...) }

// Info logs an info-level message.
func Info(format string, v ...interface{}) { logf(LevelInfo, format, v...) }

// Warn logs a warning-level message.
func Warn(format string, v ...interface{}) { logf(LevelWarn, format, v...) }

// Error logs an error-level message.
func Error(format string, v ...interface{}) { logf(LevelError, format, v...) }

// Println writes an unleveled message (always if enabled).
func Println(v ...interface{}) {
	mu.RLock()
	l := logger
	en := enabled
	mu.RUnlock()
	if !en || l == nil {
		return
	}
	l.Output(2, fmt.Sprintln(v...))
}

// Printf writes an unleveled formatted message.
func Printf(format string, v ...interface{}) {
	mu.RLock()
	l := logger
	en := enabled
	mu.RUnlock()
	if !en || l == nil {
		return
	}
	l.Output(2, fmt.Sprintf(format, v...))
}

// Level devuelve el nivel actual como string.
func Level() string {
	mu.RLock()
	defer mu.RUnlock()
	return map[int]string{
		LevelDebug: "debug",
		LevelInfo:  "info",
		LevelWarn:  "warn",
		LevelError: "error",
	}[level]
}

// IsEnabled indica si el logger está activo.
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}
