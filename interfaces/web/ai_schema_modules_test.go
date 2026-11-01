package web

// ai_schema_modules_test.go — El MCP inteligente agrupa, clasifica y scopa.

import (
	"sort"
	"strings"
	"testing"
)

// TestSchemaModulesCubrenTodasLasTablas: las 100 tablas del whitelist aparecen en
// al menos un módulo o en el conjunto transversal. Las tablas transversales
// (productos, clientes) se añaden automáticamente en tiempo de ejecución, así que
// se excluyen de esta cobertura estática.
func TestSchemaModulesCubrenTodasLasTablas(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range aiSchemaModules {
		for _, tt := range m.Tables {
			seen[tt] = true
		}
	}
	// Transversales: se añaden automáticamente en aiSchemaTablesForModules.
	transversales := map[string]bool{"customer_information": true, "product_information": true, "product_category": true, "product_service": true}
	faltantes := []string{}
	for tbl := range aiCatalogWhitelist {
		if !seen[tbl] && !transversales[tbl] {
			faltantes = append(faltantes, tbl)
		}
	}
	sort.Strings(faltantes)
	if len(faltantes) > 0 {
		t.Errorf("tablas del whitelist sin asignar a ningún módulo (%d): %s",
			len(faltantes), strings.Join(faltantes, ", "))
	}
}

// TestSchemaModulesPromptTextLiviano: el texto de clasificación es pequeño
// (debe ser ~1.5K tokens, no ~15K). Si crece mucho, algo se está metiendo de más.
func TestSchemaModulesPromptTextLiviano(t *testing.T) {
	txt := aiSchemaModulesPromptText()
	tokens := len(txt) / 4
	if tokens > 2000 {
		t.Errorf("el prompt de clasificación es muy grande: ~%d tokens (máximo esperado ~1500)", tokens)
	}
	if !strings.Contains(txt, "ventas") || !strings.Contains(txt, "cxc") {
		t.Errorf("el prompt de clasificación debe mencionar módulos clave")
	}
}

// TestSchemaModulesFromText: parsea claves separadas por comas, espacios, etc.
func TestSchemaModulesFromText(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"ventas,cxc", []string{"ventas", "cxc"}},
		{"ventas, cxc, contabilidad", []string{"ventas", "cxc", "contabilidad"}},
		{"ventas cxc", []string{"ventas", "cxc"}},
		{"VENTAS, CXC", []string{"ventas", "cxc"}}, // case-insensitive
		{"ventas, modulo_inexistente, cxc", []string{"ventas", "cxc"}}, // ignora desconocidos
		{"", []string{}},
		{"   ", []string{}},
		{"nada_valido", []string{}},
	}
	for _, c := range cases {
		got := aiSchemaModulesFromText(c.in)
		if len(got) != len(c.want) {
			t.Errorf("aiSchemaModulesFromText(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("aiSchemaModulesFromText(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestSchemaTablesForModulesConTransversales: al elegir un módulo de negocio,
// las tablas transversales (productos, clientes) se añaden automáticamente.
func TestSchemaTablesForModulesConTransversales(t *testing.T) {
	tables := aiSchemaTablesForModules([]string{"ventas"})
	hasTbl := func(tbl string) bool {
		for _, tt := range tables {
			if tt == tbl {
				return true
			}
		}
		return false
	}
	if !hasTbl("invoice") {
		t.Errorf("módulo ventas debe incluir invoice")
	}
	if !hasTbl("product_information") {
		t.Errorf("tablas transversales deben añadirse automáticamente (product_information)")
	}
	if !hasTbl("customer_information") {
		t.Errorf("tablas transversales deben añadirse automáticamente (customer_information)")
	}
}

// TestSchemaTablesForModulesContabilidadSinTransversales: un módulo puro de
// datos (contabilidad, fiscal) no arrastra productos/clientes porque no los
// necesita.
func TestSchemaTablesForModulesContabilidadSinTransversales(t *testing.T) {
	tables := aiSchemaTablesForModules([]string{"contabilidad"})
	for _, tt := range tables {
		if tt == "product_information" || tt == "customer_information" {
			t.Errorf("módulo contabilidad no debe arrastras transversales: encontró %s", tt)
		}
	}
}

// TestAllModuleKeysFallback: el fallback devuelve todos los módulos.
func TestAllModuleKeysFallback(t *testing.T) {
	all := allModuleKeys()
	if len(all) != len(aiSchemaModules) {
		t.Errorf("allModuleKeys() devolvió %d, esperaba %d", len(all), len(aiSchemaModules))
	}
}
