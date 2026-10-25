// Package setup proporciona el wizard de instalación --setup de ERPGo.
package setup

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// DefaultPorts son los puertos por defecto de cada componente.
var DefaultPorts = struct {
	ERP   int
	MySQL int
	GRPC  int
}{
	ERP:   8080,
	MySQL: 3307,
	GRPC:  9095,
}

// IsPortOpen verifica si un puerto está disponible en el host dado.
// Retorna true si el puerto está libre (se puede usar).
func IsPortOpen(host string, port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// FindFreePort busca un puerto libre en un rango.
// Retorna el primer puerto libre encontrado o -1 si no hay ninguno.
func FindFreePort(host string, start, end int) int {
	for port := start; port <= end; port++ {
		if IsPortOpen(host, port) {
			return port
		}
	}
	return -1
}

// ValidatePort verifica si un puerto es válido y está disponible.
// Retorna un mensaje de error si no es válido, o "" si está bien.
func ValidatePort(host string, port int, name string) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s: puerto %d fuera de rango (1-65535)", name, port)
	}
	if !IsPortOpen(host, port) {
		return fmt.Errorf("%s: puerto %d en uso, elija otro", name, port)
	}
	return nil
}

// PortRecommendation retorna una lista de puertos recomendados para un servicio.
func PortRecommendation(host string, defaultPort int, name string) []int {
	var recommendations []int

	// Primero el puerto por defecto
	if IsPortOpen(host, defaultPort) {
		recommendations = append(recommendations, defaultPort)
	}

	// Buscar puertos cercanos
	for offset := 1; offset <= 10; offset++ {
		if IsPortOpen(host, defaultPort+offset) {
			recommendations = append(recommendations, defaultPort+offset)
		}
		if defaultPort-offset > 0 && IsPortOpen(host, defaultPort-offset) {
			recommendations = append(recommendations, defaultPort-offset)
		}
		if len(recommendations) >= 5 {
			break
		}
	}

	return recommendations
}

// DetectConflicts detecta conflictos entre los puertos configurados.
// Retorna un mapa de servicio -> error si hay conflicto.
func ValidateAllPorts(host string, erpPort, mysqlPort, grpcPort int) map[string]string {
	conflicts := make(map[string]string)

	if err := ValidatePort(host, erpPort, "ERP"); err != nil {
		conflicts["erp"] = err.Error()
	}
	if err := ValidatePort(host, mysqlPort, "MySQL"); err != nil {
		conflicts["mysql"] = err.Error()
	}
	if err := ValidatePort(host, grpcPort, "gRPC"); err != nil {
		conflicts["grpc"] = err.Error()
	}

	// Verificar duplicados entre sí
	ports := map[int]string{erpPort: "erp", mysqlPort: "mysql", grpcPort: "grpc"}
	for p1, n1 := range ports {
		for p2, n2 := range ports {
			if p1 == p2 && n1 != n2 && p1 > 0 {
				conflicts[strings.ToLower(n1)] = fmt.Errorf("puerto %d duplicado con %s", p1, n2).Error()
			}
		}
	}

	return conflicts
}
