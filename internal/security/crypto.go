// Package security proporciona primitivas de encriptación para ERPGo.
//
// Las contraseñas de las bases de datos de cada empresa se encriptan con
// AES-256-GCM antes de guardarlas en el catálogo. La llave maestra se deriva
// de JWT_SECRET (que ya existe y nunca toca el control de versiones), de modo
// que la encriptación es determinista por instancia pero irreversible sin
// la clave original.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// DeriveKey deriva una llave AES-256 (32 bytes) desde un secreto arbitrario.
// El sufijo fijo separa este uso de otros usos del mismo secreto (JWT).
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret + "::erpgo-db-key"))
	return h[:]
}

// Encrypt encripta un texto en claro con AES-256-GCM y lo devuelve en base64.
// El nonce se genera aleatoriamente y se prepende al ciphertext.
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("clave invalida: se requieren 32 bytes para AES-256, se recibieron %d", len(key))
	}
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt invierte Encrypt: decodifica base64, extrae el nonce y descifra.
func Decrypt(encoded string, key []byte) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if len(key) != 32 {
		return "", fmt.Errorf("clave invalida: se requieren 32 bytes para AES-256, se recibieron %d", len(key))
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("texto cifrado demasiado corto")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("descifrado (clave incorrecta o datos corruptos): %w", err)
	}
	return string(plaintext), nil
}
