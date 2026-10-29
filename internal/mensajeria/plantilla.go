package mensajeria

// plantilla.go — El motor de plantillas de los avisos.
//
// POR QUÉ NO SE USA text/template. Una plantilla de aviso la escribe un humano
// en una pantalla, no un programador en el código: no puede tener acceso a
// funciones, condicionales ni bucles, y una variable desconocida no puede
// quedarse en blanco. Aquí solo hay `{{nombre}}`, y si falta un valor se
// devuelve error para que el aviso NO salga con un hueco.
//
// «Hola {{cliente}}, su saldo es {{monto}}» es todo lo que hace falta. Si algún
// día hace falta más, se amplía con una regla y su prueba, no abriendo la puerta
// a un lenguaje dentro de un mensaje.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// LargoMaximoPlantilla evita que alguien pegue un documento entero en un
// WhatsApp. No es un límite técnico del proveedor: es el límite de lo que una
// persona lee en un mensaje.
const LargoMaximoPlantilla = 2000

// VariablesDeAviso son las únicas variables que un aviso puede usar. Se declaran
// aquí para que la pantalla las ofrezca y el guardado rechace el resto.
var VariablesDeAviso = []string{"cliente", "monto", "fecha", "empresa", "referencia", "enlace"}

var reVariable = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// VariablesDe lista las variables que usa una plantilla, sin repetir y en orden.
func VariablesDe(cuerpo string) []string {
	vistas := map[string]bool{}
	var out []string
	for _, m := range reVariable.FindAllStringSubmatch(cuerpo, -1) {
		nombre := m[1]
		if !vistas[nombre] {
			vistas[nombre] = true
			out = append(out, nombre)
		}
	}
	sort.Strings(out)
	return out
}

// ValidarPlantilla comprueba el cuerpo antes de guardarlo: longitud, llaves
// bien cerradas y variables conocidas. Es mejor rechazarla al guardar que
// descubrir el hueco cuando el cliente ya recibió el mensaje.
func ValidarPlantilla(cuerpo string) error {
	cuerpo = strings.TrimSpace(cuerpo)
	if cuerpo == "" {
		return fmt.Errorf("la plantilla está vacía")
	}
	if len([]rune(cuerpo)) > LargoMaximoPlantilla {
		return fmt.Errorf("la plantilla pasa de %d caracteres; un mensaje más largo no se lee", LargoMaximoPlantilla)
	}
	if strings.Count(cuerpo, "{{") != strings.Count(cuerpo, "}}") {
		return fmt.Errorf("hay una variable sin cerrar: revise las llaves {{ }}")
	}
	permitidas := map[string]bool{}
	for _, v := range VariablesDeAviso {
		permitidas[v] = true
	}
	for _, v := range VariablesDe(cuerpo) {
		if !permitidas[v] {
			return fmt.Errorf("la variable «%s» no existe. Use: %s", v, strings.Join(VariablesDeAviso, ", "))
		}
	}
	// Un `{{` que no case con el patrón (espacios raros, acentos) se queda en
	// el texto y saldría tal cual al cliente.
	if restante := reVariable.ReplaceAllString(cuerpo, ""); strings.Contains(restante, "{{") {
		return fmt.Errorf("hay una variable con un nombre no válido; use letras, números y guion bajo")
	}
	return nil
}

// Render sustituye las variables. Si falta una o está vacía, devuelve error: un
// aviso con «Hola , su saldo es » es peor que un aviso que no sale.
func Render(cuerpo string, variables map[string]string) (string, error) {
	if err := ValidarPlantilla(cuerpo); err != nil {
		return "", err
	}
	var faltantes []string
	salida := reVariable.ReplaceAllStringFunc(cuerpo, func(trozo string) string {
		m := reVariable.FindStringSubmatch(trozo)
		if len(m) < 2 {
			return trozo
		}
		valor, ok := variables[m[1]]
		valor = strings.TrimSpace(valor)
		if !ok || valor == "" {
			faltantes = append(faltantes, m[1])
			return trozo
		}
		return valor
	})
	if len(faltantes) > 0 {
		sort.Strings(faltantes)
		return "", fmt.Errorf("faltan datos para la plantilla: %s", strings.Join(faltantes, ", "))
	}
	return salida, nil
}
