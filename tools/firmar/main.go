// tools/firmar/main.go — Calcula SHA-256 del binario y lo firma con Ed25519.
//
// Uso:
//   go run ./tools/firmar --binario erpgo-lanzamiento.exe
//
// Genera (si no existen) las llaves Ed25519 en %USERPROFILE%\.erpgo\ed25519.pem
// y %USERPROFILE%\.erpgo\ed25519-pub.pem. Luego crea:
//   <binario>.sha256  → hash SHA-256 en hex
//   <binario>.sig     → firma Ed25519 del hash, en base64
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	binario := flag.String("binario", "", "Ruta al binario a firmar")
	flag.Parse()

	if *binario == "" {
		fmt.Fprintln(os.Stderr, "Uso: go run ./tools/firmar --binario <ruta>")
		os.Exit(1)
	}

	// 1. Calcular SHA-256 del binario.
	datos, err := os.ReadFile(*binario)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo leer el binario: %v\n", err)
		os.Exit(1)
	}
	suma := sha256.Sum256(datos)
	hashHex := hex.EncodeToString(suma[:])

	// 2. Cargar o generar llaves Ed25519.
	privKey, pubKey, err := cargarLlaves()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error con las llaves Ed25519: %v\n", err)
		os.Exit(1)
	}

	// 3. Firmar el hash con la privada.
	firma := ed25519.Sign(privKey, []byte(hashHex))
	firmaB64 := base64.StdEncoding.EncodeToString(firma)

	// 4. Guardar SHA-256 y firma.
	dir := filepath.Dir(*binario)
	base := filepath.Base(*binario)
	ext := filepath.Ext(base)
	nombre := base[:len(base)-len(ext)]

	sello := filepath.Join(dir, nombre+".sha256")
	sig := filepath.Join(dir, nombre+".sig")

	if err := os.WriteFile(sello, []byte(fmt.Sprintf("%s  %s\n", hashHex, base)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo escribir el sello: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(sig, []byte(firmaB64+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo escribir la firma: %v\n", err)
		os.Exit(1)
	}

	// 5. Mostrar la pública embebida (para el binario).
	pubB64 := base64.StdEncoding.EncodeToString(pubKey)
	fmt.Printf("Binario : %s\n", *binario)
	fmt.Printf("SHA-256 : %s\n", hashHex)
	fmt.Printf("Firma   : %s\n", firmaB64)
	fmt.Printf("Pública : %s\n", pubB64)
	fmt.Printf("\nSello: %s\n", sello)
	fmt.Printf("Firma: %s\n", sig)
}

// cargarLlaves carga el par Ed25519 de ~/.erpgo o lo genera una sola vez.
func cargarLlaves() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	dir := filepath.Join(os.Getenv("USERPROFILE"), ".erpgo")
	privFile := filepath.Join(dir, "ed25519.pem")
	pubFile := filepath.Join(dir, "ed25519-pub.pem")

	// Cargar si existen.
	if data, err := os.ReadFile(privFile); err == nil {
		priv, err := parsePrivateKey(data)
		if err != nil {
			return nil, nil, fmt.Errorf("leyendo privada: %w", err)
		}
		pubData, err := os.ReadFile(pubFile)
		if err != nil {
			return nil, nil, fmt.Errorf("leyendo pública: %w", err)
		}
		return priv, parsePublicKey(pubData), nil
	}

	// Generar par nuevo.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generando llaves: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}

	// Guardar privada en formato simple (hex crudo, 64 bytes).
	privHex := hex.EncodeToString(priv.Seed())
	if err := os.WriteFile(privFile, []byte(privHex+"\n"), 0o600); err != nil {
		return nil, nil, err
	}
	pubHex := hex.EncodeToString(pub)
	if err := os.WriteFile(pubFile, []byte(pubHex+"\n"), 0o644); err != nil {
		return nil, nil, err
	}

	fmt.Printf("Llaves Ed25519 generadas en %s\n", dir)
	return priv, pub, nil
}

func parsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	txt := string(data) // puede tener salto final
	hexStr := ""
	for _, line := range splitLines(txt) {
		if line != "" {
			hexStr = line
			break
		}
	}
	seed, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("semilla de tamaño incorrecto: %d", len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func parsePublicKey(data []byte) ed25519.PublicKey {
	txt := string(data)
	hexStr := ""
	for _, line := range splitLines(txt) {
		if line != "" {
			hexStr = line
			break
		}
	}
	pub, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil
	}
	return pub
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
