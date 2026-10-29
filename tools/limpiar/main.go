// Command limpiar borra del servidor MySQL las bases de datos que dejaron los
// tests al morir a mitad de una corrida.
//
// POR QUÉ EXISTE ESTA HERRAMIENTA, Y POR QUÉ NO ES UN SCRIPT DE MySQL.
//
// Los tests ya borran sus bases al terminar, con t.Cleanup. Ese borrado falla en
// un caso concreto y frecuente: la corrida que no llega al final. Si el binario
// se mata, si MySQL se reinicia, si el portapapeles se cierra, los t.Cleanup no
// se ejecutan y las bases se quedan. Passó aquí: siete corridas incompletas
// dejaron 97 bases, y el efecto fue el ERROR 1040 "Too many connections" en
// una tanda de tests que no tenía nada que ver con bases.
//
// La regla de qué es una base de prueba y cuál es una empresa de una persona
// la tiene persistence, que es el paquete que inventó los nombres. Duplicar esa
// regla en un script es exactamente la clase de duplicación que hace que un
// día el script borre la base equivocada y nadie sepa por qué. Aquí se llama a
// la función, y punto.
//
// LO QUE NO HACE.
//
// No toca bases de empresa. El patrón que reconoce es `erpgo_test_...` y
// `erpgo_t<contador>_<sello>_...`, los dos con un número y una marca de tiempo
// que este repositorio pone y que ningún nombre de empresa puede llevar.
//
// USO.
//
//	go run ./tools/limpiar          borra las huérfanas
//	go run ./tools/limpiar -listar  solo las muestra, no borra nada
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"erpgo/infrastructure/persistence"
)

func main() {
	listar := flag.Bool("listar", false, "solo listar las bases huérfanas, sin borrar ninguna")
	flag.Parse()

	if !*listar {
		fmt.Println("Buscando bases de prueba abandonadas...")
		encontradas, borradas, err := persistence.BorrarBasesDePruebaHuerfanas()
		if err != nil {
			fmt.Fprintf(os.Stderr, "No se pudo consultar el servidor MySQL: %v\n", err)
			fmt.Fprintln(os.Stderr, "¿Está corriendo? Sin MySQL no hay nada que limpiar.")
			os.Exit(1)
		}
		fmt.Printf("Bases de prueba abandonadas encontradas: %d\n", encontradas)
		fmt.Printf("Bases borradas: %d\n", borradas)
		if encontradas > borradas {
			// No es un error del programa: es una base con conexiones
			// abiertas, que en Windows no se puede eliminar. Se dice cuál
			// quedó y por qué, en lugar de dejar que se descubra la próxima
			// vez que falten conexiones.
			fmt.Printf("Quedaron %d sin borrar. Suele ser una base con conexiones "+
				"abiertas (el ERP o un test todavía vivo): ciérralo y repite.\n",
				encontradas-borradas)
			quedan, err := persistence.BasesDePruebaHuerfanas()
			if err == nil {
				sort.Strings(quedan)
				for _, n := range quedan {
					fmt.Println("  -", n)
				}
			}
		}
		if borradas > 0 {
			fmt.Printf("Liberadas %d bases a las %s.\n", borradas, time.Now().Format("15:04:05"))
		}
		return
	}

	huerfanas, err := persistence.BasesDePruebaHuerfanas()
	if err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo consultar el servidor MySQL: %v\n", err)
		os.Exit(1)
	}
	if len(huerfanas) == 0 {
		fmt.Println("No hay bases de prueba abandonadas.")
		return
	}
	sort.Strings(huerfanas)
	fmt.Printf("Bases de prueba abandonadas: %d\n", len(huerfanas))
	for _, n := range huerfanas {
		fmt.Println("  -", n)
	}
	fmt.Println("\nPara borrarlas: go run ./tools/limpiar")
}

