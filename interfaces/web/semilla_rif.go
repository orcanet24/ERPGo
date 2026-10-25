package web

// semilla_rif.go — RIFoules válidos.
//
// POR QUÉ HAY UN FICHERO ENTERO DEDICADO A FABRICAR RIF.
//
// Porque `ValidateRIF` (interfaces/web/fiscal.go) exige el dígito verificador
// módulo 11, y un RIF inventado "J-40112233-7" no lo tiene: el ERP lo rechaza
// con "RIF inválido" y la factura no se guarda. Es la comprobación correcta —
// un RIF es un identificador fiscal, no una etiqueta— y obliga a que los datos
// de prueba sean RIF de verdad.
//
// Se usa el MISMO algoritmo y las MISMAS tablas de pesos que el ERP, en vez de
// una copia: si el ERP cambiara su validación, esta copia se quedaría atrás y
// volvería a fallar, y nadie sabría por qué.
//
// ESTOS RIF SON FICTICIOS PERO VÁLIDOS: el dígito verificador es correcto, pero
// los números no corresponden a ningún contribuyente real. Sirven para probar el
// ERP, no para declarar ante el SENIAT.

import (
	"fmt"
	"strings"
)

// rifCompleto calcula el RIF con su dígito verificador a partir de la letra y
// los ocho dígitos.
//
// Es `ValidateRIF` dado la vuelta: se aplica el mismo módulo 11 con los mismos
// pesos y se devuelve el dígito que hace que la validación pase.
func rifCompleto(letra string, numero int) string {
	if len(letra) != 1 {
		letra = "J"
	}
	letra = strings.ToUpper(letra)
	lv, ok := rifLetterVal[letra[0]]
	if !ok {
		lv = rifLetterVal['J']
		letra = "J"
	}

	digitos := fmt.Sprintf("%08d", numero%100000000)
	suma := lv
	for i := 0; i < 8; i++ {
		suma += int(digitos[i]-'0') * rifWeights[i]
	}
	dv := 11 - (suma % 11)
	if dv >= 10 {
		dv = 0
	}
	return fmt.Sprintf("%s-%s-%d", letra, digitos, dv)
}

// rifConFormato deja el RIF como lo escribe una persona: J-40112233-7.
func rifConFormato(letra string, numero int) string {
	return rifCompleto(letra, numero)
}
