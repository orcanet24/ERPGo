// Package fiscal_validator_test - Pruebas del validador de integridad fiscal
package fiscal_validator_test

import (
	"errors"
	"strings"
	"testing"
)

// fiscalProtectedTables son tablas que nunca pueden ser modificadas por IA
var fiscalProtectedTables = map[string]bool{
	"acc_transaction": true,    // transacciones contables fiscales
	"audit_log":       true,    // logs de auditoría fiscal
	"fiscal_setting":  true,    // configuración fiscal
	"fiscal_series":   true,    // series de documentos fiscales
	"invoice":         true,    // facturas fiscales
	"retention":       true,    // retenciones ISLR/IVA
	"tax_config":      true,    // configuración de impuestos
}

// fiscalProtectedKeywords son palabras clave que indican operación fiscal prohibida
var fiscalProtectedKeywords = []string{
	"ocultar", "esconder", "borrar", "eliminar fiscal",
	"esquivar", "falsificar", "ocultar_base",
	"modificar_impuestos", " cambiar_tasas_fiscales",
	"suprimir_comprobante", "anular_factura_fiscal",
	"ocultar_facturas", "esconder_contabilidad",
}

// OperationType es el tipo de operación IA
type OperationType string

const (
	// create_field crea un nuevo campo en la BD
	create_field OperationType = "create_field"
	// create_module crea un nuevo módulo
	create_module OperationType = "create_module"
	// create_report crea un nuevo reporte
	create_report OperationType = "create_report"
	// modify_field modifica un campo existente
	modify_field OperationType = "modify_field"
)

// IAOperation representa una operación solicitada por IA
type IAOperation struct {
	Prompt      string
	Operation   OperationType
	TablesAffected []string
}

// validarOperacionFiscal valida que una operación IA no viole la integridad fiscal
func validarOperacionFiscal(op IAOperation) error {
	// 1. Revisar prompt por palabras clave prohibidas
	promptLower := strings.ToLower(op.Prompt)
	for _, kw := range fiscalProtectedKeywords {
		if strings.Contains(promptLower, kw) {
			return errors.New("Operación fiscal prohibida: contiene palabra clave sensible fiscal (" + kw + ")")
		}
	}

	// 2. Revisar tablas afectadas
	for _, tabla := range op.TablesAffected {
		if fiscalProtectedTables[tabla] {
			return errors.New("Modificación sobre tabla fiscal prohibida: " + tabla)
		}
	}

	// 3. Revisar por tipo de operación y contexto fiscal
	if op.Operation == create_field || op.Operation == modify_field {
		// Campos que tocan totales o impuestos están prohibidos
		fiscalFieldKeywords := []string{"impuesto", "total", "base_imponible", "retencion", "igtf"}
		for _, kw := range fiscalFieldKeywords {
			if strings.Contains(promptLower, kw) {
				return errors.New("Campo fiscal prohibido: " + kw + " no puede ser modificado por IA")
			}
		}
	}

	return nil
}

// Test cases para el validador fiscal
func TestFiscalValidator(t *testing.T) {
	tests := []struct {
		name       string
		operation  IAOperation
		wantErr    bool
		errContains string
	}{
		// ❌ Deben ser bloqueadas (fiscal)
		{
			name: "Ocultar facturas fiscales",
			operation: IAOperation{
				Prompt:       "Ocultar todas las facturas emitidas en octubre para no mostrarlas en el reporte trimestral",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Modificar tasas de impuesto",
			operation: IAOperation{
				Prompt:       "Cambiar la tasa de IVA del 16% al 15% en la configuración fiscal",
				Operation:    modify_field,
				TablesAffected: []string{"fiscal_setting"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Borrar logs fiscales",
			operation: IAOperation{
				Prompt:       "Eliminar todos los registros del log de auditoría fiscal",
				Operation:    create_field,
				TablesAffected: []string{"audit_log"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Suprimir comprobantes fiscales",
			operation: IAOperation{
				Prompt:       "Suprimir todos los comprobantes fiscales del mes anterior",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Modificar tasa ISLR en config",
			operation: IAOperation{
				Prompt:       "Modificar la tasa ISLR en la configuración fiscal",
				Operation:    modify_field,
				TablesAffected: []string{"tax_config"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Eliminar transacciones contables",
			operation: IAOperation{
				Prompt:       "Eliminar transacciones contables antiguas del mes pasado",
				Operation:    create_field,
				TablesAffected: []string{"acc_transaction"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Suprimir notas de crédito",
			operation: IAOperation{
				Prompt:       "Suprimir todas las notas de crédito del trimestre",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Falsificar datos fiscales",
			operation: IAOperation{
				Prompt:       "Falsificar bases imponibles en facturas emitidas",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Ocultar facturas del sistema",
			operation: IAOperation{
				Prompt:       "Ocultar facturas del sistema para ajustar报表",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Cambiar tasa IGTF",
			operation: IAOperation{
				Prompt:       "Cambiar la tasa IGTF del 3% al 5%",
				Operation:    modify_field,
				TablesAffected: []string{"fiscal_setting"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Palabra clave 'eliminar' en contexto fiscal",
			operation: IAOperation{
				Prompt:       "Eliminar logs del sistema que no son fiscales",
				Operation:    create_field,
				TablesAffected: []string{"system_log"},
			},
			wantErr:    false, // "system_log" no es tabla fiscal protegida
		},
		{
			name: "Campo total en factura (bloqueado)",
			operation: IAOperation{
				Prompt:       "Añadir campo total_final a facturas tipo decimal",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "Campo fiscal prohibido",
		},

		// ✅ Deben pasar (no fiscal)
		{
			name: "Añadir campo peso a productos",
			operation: IAOperation{
				Prompt:       "Añadir campo peso_lote a productos, tipo decimal(10,2), para rastrear peso por lote",
				Operation:    create_field,
				TablesAffected: []string{"product"},
			},
			wantErr:    false,
		},
		{
			name: "Crear módulo de control de calidad",
			operation: IAOperation{
				Prompt:       "Crear módulo control_calidad para rastrear lotes de productos con campos lote_numero y fecha_vencimiento",
				Operation:    create_module,
				TablesAffected: []string{},  // Módulos nuevos no tocan tablas fiscales
			},
			wantErr:    false,
		},
		{
			name: "Añadir campo código de barras",
			operation: IAOperation{
				Prompt:       "Añadir campo codigo_barra a tabla products, tipo varchar(50), opcional",
				Operation:    create_field,
				TablesAffected: []string{"product"},
			},
			wantErr:    false,
		},
		{
			name: "Crear reporte de clientes por actividad",
			operation: IAOperation{
				Prompt:       "Crear reporte clientes_por_actividad agrupados por tipo de actividad economica con totals por rangos de fechas",
				Operation:    create_report,
				TablesAffected: []string{},
			},
			wantErr:    false,
		},
		{
			name: "Añadir campo observaciones a proveedores",
			operation: IAOperation{
				Prompt:       "Añadir campo observaciones_proveedor a tabla suppliers, tipo text, opcional",
				Operation:    create_field,
				TablesAffected: []string{"supplier"},
			},
			wantErr:    false,
		},
		{
			name: "Crear modulo contabilidad",
			operation: IAOperation{
				Prompt:       "Crear modulo contabilidad general para llevar libros contables generales",
				Operation:    create_module,
				TablesAffected: []string{},
			},
			wantErr:    false,
		},

		// ⚠️ Casos borde - dependen del contexto
		{
			name: "Campo con palabra 'total' pero no fiscal",
			operation: IAOperation{
				Prompt:       "Añadir campo subtotal_compra a facturas, tipo decimal(10,2)",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			// Esto podría ser bloqueado dependiendo de la interpretación
			// Para el test, lo dejamos como paso (wantErr: false) pero podría ser true
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validarOperacionFiscal(tt.operation)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect error, got: %v", err)
				}
			}
		})
	}
}

// TestFiscalValidatorOperationTypes prueba diferentes tipos de operación
func TestFiscalValidatorOperationTypes(t *testing.T) {
	operations := []IAOperation{
		// create_field pruebas
		{Prompt: "Añadir campo descripcion a productos, tipo string", Operation: create_field, TablesAffected: []string{"product"}},
		{Prompt: "Modificar tamaño de campo nombre en tabla clientas", Operation: modify_field, TablesAffected: []string{"customer"}},

		// create_module pruebas
		{Prompt: "Crear modulo inventario con funciones de entrada/salida", Operation: create_module, TablesAffected: []string{}},
		{Prompt: "Añadir nuevo modulo reportes_vendidos", Operation: create_module, TablesAffected: []string{}},

		// TablesAffected vacío o no fiscal
		{Prompt: "Nuevo modulo estadisticas", Operation: create_module, TablesAffected: []string{}},

		// ⚠️ Campos fiscales en tablas no protegidas (alerta pero no bloqueo total)
		{Prompt: "Añadir campo impuesto_final a tabla orders", Operation: modify_field, TablesAffected: []string{"orders"}},

		// create_module pruebas adicionales
		{Prompt: "Crear modulo control calidad", Operation: create_module, TablesAffected: []string{}},
		{Prompt: "Añadir nuevo modulo reportes_vendidos", Operation: create_module, TablesAffected: []string{}},

		// TablesAffected vacío o no fiscal
		{Prompt: "Nuevo modulo estadisticas", Operation: create_module, TablesAffected: []string{}},
	}

	for _, op := range operations {
		t.Run(op.Prompt[:35], func(t *testing.T) {
			err := validarOperacionFiscal(op)
			// Para operaciones no fiscal, debería ser nil
			if len(op.TablesAffected) == 0 || !esTablaFiscal(op.TablesAffected[0]) {
				if err != nil {
					t.Logf("Operación permitida pero dio error: %v", err)
				}
			}
		})
	}
}

// TestFiscalEdgeCases prueba casos borde y nuevas protecciones
func TestFiscalEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		operation  IAOperation
		wantErr    bool
		errContains string
	}{
		// ❌ Nuevos bloqueos reforzados
		{
			name: "Modificar tasa ISLR en config",
			operation: IAOperation{
				Prompt:       "Modificar la tasa ISLR en la configuración fiscal",
				Operation:    modify_field,
				TablesAffected: []string{"tax_config"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Eliminar transacciones contables",
			operation: IAOperation{
				Prompt:       "Eliminar transacciones contables antiguas del mes pasado",
				Operation:    create_field,
				TablesAffected: []string{"acc_transaction"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Suprimir notas de crédito",
			operation: IAOperation{
				Prompt:       "Suprimir todas las notas de crédito del trimestre",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Falsificar datos fiscales",
			operation: IAOperation{
				Prompt:       "Falsificar bases imponibles en facturas emitidas",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Ocultar facturas del sistema",
			operation: IAOperation{
				Prompt:       "Ocultar facturas del sistema para ajustar报表",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Cambiar tasa IGTF",
			operation: IAOperation{
				Prompt:       "Cambiar la tasa IGTF del 3% al 5%",
				Operation:    modify_field,
				TablesAffected: []string{"fiscal_setting"},
			},
			wantErr:    true,
			errContains: "prohibida",
		},
		{
			name: "Campo total en factura (bloqueado)",
			operation: IAOperation{
				Prompt:       "Añadir campo total_final a facturas tipo decimal",
				Operation:    create_field,
				TablesAffected: []string{"invoice"},
			},
			wantErr:    true,
			errContains: "Campo fiscal prohibido",
		},

		// ✅ Deben permitirse (operaciones legítimas no fiscales)
		{
			name: "Añadir campo peso a productos",
			operation: IAOperation{
				Prompt:       "Añadir campo peso_lote a productos, tipo decimal(10,2), para rastrear peso por lote",
				Operation:    create_field,
				TablesAffected: []string{"product"},
			},
			wantErr:    false,
		},
		{
			name: "Crear módulo de control de calidad",
			operation: IAOperation{
				Prompt:       "Crear módulo control_calidad para rastrear lotes de productos con campos lote_numero y fecha_vencimiento",
				Operation:    create_module,
				TablesAffected: []string{},
			},
			wantErr:    false,
		},
		{
			name: "Añadir campo código de barras",
			operation: IAOperation{
				Prompt:       "Añadir campo codigo_barra a tabla products, tipo varchar(50), opcional",
				Operation:    create_field,
				TablesAffected: []string{"product"},
			},
			wantErr:    false,
		},
		{
			name: "Crear reporte clientes",
			operation: IAOperation{
				Prompt:       "Crear reporte clientes_por_actividad agrupados por tipo de actividad economica con totals por rangos de fechas",
				Operation:    create_report,
				TablesAffected: []string{},
			},
			wantErr:    false,
		},
		{
			name: "Añadir campo observaciones a proveedores",
			operation: IAOperation{
				Prompt:       "Añadir campo observaciones_proveedor a tabla suppliers, tipo text, opcional",
				Operation:    create_field,
				TablesAffected: []string{"supplier"},
			},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validarOperacionFiscal(tt.operation)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect error, got: %v", err)
				}
			}
		})
	}
}

// TestFiscalKeywordDetection mejora detección de palabras clave
func TestFiscalKeywordDetection(t *testing.T) {
	tests := []struct {
		prompt    string
		expectBlocked bool
	}{
		{"Ocultar facturas del mes", true},
		{"Borrar logs del sistema", false},
		{"Modificar tasa de IVA", true},
		{"Añadir campo observacion", false},
		{"Falsificar datos fiscales", true},
		{"Crear reporte ventas", false},
		{"Eliminar transacciones fiscales", true},
		{"Añadir campo fecha_adicional", false},
		{"Modificar tasa ISLR", true},
		{"Suprimir comprobantes", true},
		{"Obfuscar datos contables", true},
		{"Crear modulo contabilidad", false},
	}

	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			op := IAOperation{
				Prompt:    tt.prompt,
				Operation: create_field,
				TablesAffected: []string{"product"}, // tabla no fiscal
			}
			err := validarOperacionFiscal(op)

			if tt.expectBlocked {
				if err == nil {
					t.Error("Expected blocked but got nil")
				} else {
					t.Logf("Correctly blocked: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected allowed but got blocked: %v", err)
				} else {
					t.Logf("Correctly allowed: %v", err)
				}
			}
		})
	}
}

// esTablaFiscal chequea si una tabla está en la lista de protegida
func esTablaFiscal(tabla string) bool {
	_, protected := fiscalProtectedTables[tabla]
	return protected
}