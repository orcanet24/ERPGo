// tools/firmaraddon — Parejas de firma y verificación de manifiestos ed25519
// (docs/PLAN-ADDONS.md §8 fase 3).
//
// El dueño del ERP firma los manifiestos de SU carpeta addons/ y entrega la
// clave pública al núcleo:
//
//	1. Generar su pareja (una vez):
//	       go run ./tools/firmaraddon clave
//	   Guarda la clave PRIVADA en firmaraddon.key (permiso 0600, jamás al
//	   repositorio) e imprime la pública para ponerla en addons/pubkey.ed25519.
//	2. Firmar un addon:
//	       go run ./tools/firmaraddon firmar addons/hola
//	   Lee manifest.json con la privada (entorno FIRMARADDON_KEY o el archivo
//	   firmaraddon.key) y escribe manifest.sig al lado.
//	3. Verificar (lo que el núcleo hace en cada escaneo, en su mano):
//	       go run ./tools/firmaraddon verificar addons/hola
//
// firmaraddon.key es la propiedad única del dueño, como hacerlo con su VPN o
// su firma mensual: de su uso depende que el manifiesto sea exactamente lo que
// él firmó.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		salir(2, "uso: firmaraddon clave | firmar <carpeta> | verificar <carpeta>")
	}
	switch os.Args[1] {
	case "clave":
		clave()
	case "firmar":
		if len(os.Args) != 3 {
			salir(2, "uso: firmaraddon firmar <carpeta>")
		}
		firmar(os.Args[2])
	case "verificar":
		if len(os.Args) != 3 {
			salir(2, "uso: firmaraddon verificar <carpeta>")
		}
		verificar(os.Args[2])
	default:
		salir(2, "subcomando desconocido: "+os.Args[1])
	}
}

// clave genera la pareja ed25519 del dueño. La privada cae en firmaraddon.key
// (hex) con permiso 0600; la pública sale por pantalla.
func clave() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		salir(1, "no se pudo generar la clave: "+err.Error())
	}
	if err := os.WriteFile("firmaraddon.key", []byte(hex.EncodeToString(priv)+"\n"), 0o600); err != nil {
		salir(1, "no se pudo escribir firmaraddon.key: "+err.Error())
	}
	fmt.Println("Clave privada en firmaraddon.key (0600): NO la commitee.")
	fmt.Println("Ponga esta clave pública en <raíz>/addons/pubkey.ed25519:")
	fmt.Println(hex.EncodeToString(pub))
}

// privada toma la clave privada del entorno FIRMARADDON_KEY o de firmaraddon.key.
func privada() ed25519.PrivateKey {
	crudo := os.Getenv("FIRMARADDON_KEY")
	if crudo == "" {
		b, err := os.ReadFile("firmaraddon.key")
		if err != nil {
			salir(1, "sin FIRMARADDON_KEY ni firmaraddon.key en el directorio actual: use «firmaraddon clave» primero")
		}
		crudo = string(b)
	}
	seeds, err := hex.DecodeString(strings.TrimSpace(crudo))
	if err != nil || len(seeds) != ed25519.PrivateKeySize {
		salir(1, "la clave privada no es ed25519 en hex del tamaño correcto")
	}
	return ed25519.PrivateKey(seeds)
}

// firmar firma <carpeta>/manifest.json y deja <carpeta>/manifest.sig.
func firmar(carpeta string) {
	manifiesto, err := os.ReadFile(filepath.Join(carpeta, "manifest.json"))
	if err != nil {
		salir(1, "no se pudo leer el manifiesto: "+err.Error())
	}
	firma := ed25519.Sign(privada(), manifiesto)
	if err := os.WriteFile(filepath.Join(carpeta, "manifest.sig"), []byte(hex.EncodeToString(firma)+"\n"), 0o644); err != nil {
		salir(1, "no se pudo escribir manifest.sig: "+err.Error())
	}
	fmt.Printf("%s firmado (manifest.sig escrito).\n", carpeta)
}

// verificar reproduce LA política del núcleo: con la pública en el directorio
// padre (<raíz>/addons), sin firma con clave configurada no carga, y con firma
// inválida no carga en ningún modo.
func verificar(carpeta string) {
	manifiesto, err := os.ReadFile(filepath.Join(carpeta, "manifest.json"))
	if err != nil {
		salir(1, "no se pudo leer el manifiesto: "+err.Error())
	}
	crudo, err := os.ReadFile(filepath.Join(filepath.Dir(carpeta), "pubkey.ed25519"))
	if err != nil {
		salir(1, "sin pubkey.ed25519 en el directorio de addons: el núcleo corre «sin verificar»")
	}
	pub, err := hex.DecodeString(strings.TrimSpace(string(crudo)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		salir(1, "pubkey.ed25519 no es una clave pública ed25519 en hex")
	}
	sig, err := os.ReadFile(filepath.Join(carpeta, "manifest.sig"))
	if err != nil {
		salir(1, "sin manifest.sig: con la clave configurada el núcleo NO carga este addon (salvo ADDONS_UNSIGNED=1)")
	}
	firma, err := hex.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(firma) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(pub), manifiesto, firma) {
		salir(1, "la firma NO corresponde: el manifiesto cambió después de firmar y el núcleo lo rechaza")
	}
	fmt.Printf("%s: firma verificada.\n", carpeta)
}

func salir(codigo int, msg string) {
	fmt.Fprintln(os.Stderr, "firmaraddon: "+msg)
	os.Exit(codigo)
}
