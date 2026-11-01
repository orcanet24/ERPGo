package web

// ai_dns_guard_test.go — el guardián DNS no debe cortar a proveedores legítimos.
//
// Regresión real: api.longcat.ai vive detrás de un CDN con DNS geográfico y
// Cloudflare y Google responden IPs DISTINTAS (cada uno anuncia el edge de su
// región). El consenso viejo exigía la INTERSECCIÓN de las respuestas, que
// era vacía, y el ERP rechazaba un proveedor que funcionaba perfectamente.
// La regla correcta es más simple y sigue detectando el envenenamiento: la
// resolución LOCAL debe estar presentada por ALGÚN resolver público (unión);
// una IP inyectada no estará en ninguno.

import (
	"testing"
)

// TestElGuardianADmiteResolversSinSolapamiento fija la regla nueva con las
// IPs reales observadas en el incidente de api.longcat.ai (gccdn.net).
func TestElGuardianADmiteResolversSinSolapamiento(t *testing.T) {
	cf := []string{"140.150.37.50", "2803:bc40:81ff:4::23"}  // edge de Cloudflare
	google := []string{"153.43.97.3", "2803:d760:31:1::1c"}   // edge de Google

	// Con la intersección vieja, estas dos respuestas legítimas no
	// compartían ni una IP: el guardián daba «los resolvers públicos no
	// coinciden entre sí» y cortaba un proveedor sano. Con la unión, la IP
	// que la máquina resuelve localmente (la de Google) debe validar.
	union := aiIPSetFromStrings(append(append([]string{}, cf...), google...))
	local := aiIPSetFromStrings([]string{"153.43.97.3"})
	if faltan := aiMissing(local, union); len(faltan) > 0 {
		t.Fatalf("la IP local real del incidente no puede quedar «faltante» en la unión: %v", faltan)
	}
}

// TestElGuardianCortaIPLocalNoPresentadaPublicamente: el caso que el
// guardián sigue defendiendo. Un proxy de inspección en red corporativa
// responde una IP que NINGÚN resolver público publicó → se corta.
func TestElGuardianCortaIPLocalNoPresentadaPublicamente(t *testing.T) {
	union := aiIPSetFromStrings([]string{"140.150.37.50", "153.43.97.3"})
	local := aiIPSetFromStrings([]string{"10.66.66.66"})
	if faltan := aiMissing(local, union); len(faltan) == 0 {
		t.Fatal("una IP no anunciada públicamente debe quedar «faltante» y cortar la conexión")
	}
}
