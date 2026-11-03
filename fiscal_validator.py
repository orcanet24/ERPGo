#!/usr/bin/env python3
"""
Validador de Integridad Fiscal para ERPGo
========================================

Protege datos fiscales de modificaciones no autorizadas por IA.

Sistema de protección con:
1. Palabras clave prohibidas
2. Tablas fiscales protegidas
3. Tipos de campos sensibles
4. Registro de todas las operaciones

Autor: Codebuff
Fecha: 2026-11-03
"""

import sys
import re
from typing import List, Dict, Any


# ============================================================
# CONFIGURACIÓN DE PROTECCIÓN FISCAL
# ============================================================

# Tablas que NUNCA pueden ser modificadas por IA
FISCAL_PROTECTED_TABLES = {
    "acc_transaction": "Transacciones contables fiscales",
    "audit_log": "Logs de auditoría fiscal",
    "fiscal_setting": "Configuración fiscal",
    "fiscal_series": "Series de documentos fiscales",
    "invoice": "Facturas fiscales",
    "retention": "Retenciones ISLR/IVA",
    "tax_config": "Configuración de impuestos",
    "pos_fiscal_blocks": "Bloques fiscales por caja",
    "company": "Datos de empresa (RIF, config)",
}

# Palabras clave que indican operación fiscal prohibida
FISCAL_PROTECTED_KEYWORDS = [
    "ocultar", "esconder", "borrar", "eliminar fiscal",
    "esquivar", "falsificar", "ocultar_base",
    "modificar_impuestos", "cambiar_tasas_fiscales",
    "suprimir_comprobante", "anular_factura_fiscal",
    "ocultar_facturas", "esconder_contabilidad",
    "borrar_facturas", "suprimir_nt", "anular_nd",
]

# Palabras clave de campos sensibles (advertencia, no bloqueo total)
SENSITIVE_FIELD_KEYWORDS = [
    "impuesto", "total", "base_imponible", "retencion", "igtf",
    "precio_final", "costo_total", "subtotal_fiscal",
    "tarifa_islr", "tarifa_iva", "porcentaje_igtf",
]

# Palabras clave de operaciones de solo lectura permitidas
SAFE_OPERATION_KEYWORDS = [
    "consultar", "ver", "listar", "reportar",
    "generar", "crear_backup", "exportar",
]


# ============================================================
# ESTRUCTURAS DE DATOS
# ============================================================

class IAOperation:
    """Representa una operación solicitada por IA."""

    def __init__(self, prompt: str, operation_type: str = "create_field",
                 tables_affected: List[str] = None):
        self.prompt = prompt
        self.operation_type = operation_type
        self.tables_affected = tables_affected if tables_affected else []


# ============================================================
# VALIDADOR PRINCIPAL
# ============================================================

def validar_operacion_fiscal(operation: IAOperation) -> Dict[str, Any]:
    """
    Valida que una operación IA no viole la integridad fiscal.

    Returns:
        Dict con:
            - "permitido": bool
            - "motivo": str (si está bloqueado)
            - "tipo_bloqueo": str ("keyword", "tabla", "campo" o None)
    """
    prompt_lower = operation.prompt.lower()
    result = {
        "permitido": True,
        "motivo": "",
        "tipo_bloqueo": None,
    }

    # 1. Revisar palabras clave prohibidas (mayor prioridad)
    for kw in FISCAL_PROTECTED_KEYWORDS:
        if kw in prompt_lower:
            result["permitido"] = False
            result["motivo"] = f"Operación fiscal prohibida: contiene palabra clave sensible '{kw}'"
            result["tipo_bloqueo"] = "keyword"
            return result

    # 2. Revisar tablas afectadas
    for tabla in operation.tables_affected:
        if tabla in FISCAL_PROTECTED_TABLES:
            result["permitido"] = False
            result["motivo"] = f"Modificación sobre tabla fiscal prohibida: {tabla} ({FISCAL_PROTECTED_TABLES[tabla]})"
            result["tipo_bloqueo"] = "tabla"
            return result

    # 3. Revisar campos sensibles en el prompt
    # (esto es una advertencia, no bloqueo total a menos que sea crítico)
    for kw in SENSITIVE_FIELD_KEYWORDS:
        if kw in prompt_lower:
            # No bloqueamos aquí, pero añadimos alerta
            if result["motivo"] == "":
                result["motivo"] = f"Alerta: prompt contiene palabra campo sensible '{kw}'"
                result["tipo_bloqueo"] = "campo_alerta"
            else:
                result["motivo"] += f"; alerta campo '{kw}'"

    # 4. Verificar si es operación segura (consulta/reporte)
    # Si el prompt contiene palabras de operación segura, permitir si no hay bloqueos previos
    if result["permitido"] and operation.operation_type in ("create_report",):
        for safe_kw in SAFE_OPERATION_KEYWORDS:
            if safe_kw in prompt_lower:
                # Permitir operaciones de reporte con palabras clave seguras
                result["motivo"] = "Operación de reporte fiscal permitida"
                result["tipo_bloqueo"] = None
                result["permitido"] = True
                return result

    return result


# ============================================================
# FUNCIONES DE AYUDA
# ============================================================

def es_tabla_fiscal(tabla: str) -> bool:
    """Chequea si una tabla está en la lista de protegida."""
    return tabla in FISCAL_PROTECTED_TABLES


def obtener_motivo_bloqueo(result: Dict[str, Any]) -> str:
    """Obtiene el motivo legible si la operación fue bloqueada."""
    if not result["permitido"]:
        return result["motivo"]
    return "Operación permitida"


# ============================================================
# PROBANDO EL VALIDADOR
# ============================================================

def run_tests() -> None:
    """Ejecuta todas las pruebas del validador fiscal."""
    pruebas = [
        # ❌ DEBEN SER BLOQUEADAS (fiscal)
        ("Ocultar facturas fiscales",
         IAOperation("Ocultar todas las facturas emitidas en octubre para no mostrarlas en el reporte trimestral",
                     tables_affected=["invoice"]),
         False, "keyword"),
        ("Modificar tasas de impuesto",
         IAOperation("Cambiar la tasa de IVA del 16% al 15% en la configuración fiscal",
                     tables_affected=["fiscal_setting"]),
         False, "tabla"),
        ("Borrar logs fiscales",
         IAOperation("Eliminar todos los registros del log de auditoría fiscal",
                     tables_affected=["audit_log"]),
         False, "tabla"),
        ("Suprimir comprobantes fiscales",
         IAOperation("Suprimir todos los comprobantes fiscales del mes anterior",
                     tables_affected=["invoice"]),
         False, "keyword"),
        ("Modificar tasa ISLR en config",
         IAOperation("Modificar la tasa ISLR en la configuración fiscal",
                     tables_affected=["tax_config"]),
         False, "tabla"),
        ("Eliminar transacciones contables",
         IAOperation("Eliminar transacciones contables antiguas del mes pasado",
                     tables_affected=["acc_transaction"]),
         False, "tabla"),
        ("Suprimir notas de crédito",
         IAOperation("Suprimir todas las notas de crédito del trimestre",
                     tables_affected=["invoice"]),
         False, "keyword"),
        ("Falsificar datos fiscales",
         IAOperation("Falsificar bases imponibles en facturas emitidas",
                     tables_affected=["invoice"]),
         False, "keyword"),
        ("Ocultar facturas del sistema",
         IAOperation("Ocultar facturas del sistema para ajustar报表",
                     tables_affected=["invoice"]),
         False, "keyword"),
        ("Cambiar tasa IGTF",
         IAOperation("Cambiar la tasa IGTF del 3% al 5%",
                     tables_affected=["fiscal_setting"]),
         False, "tabla"),
        ("Obfuscar datos fiscales",
         IAOperation("Obfuscar datos contables para proteger privacidad",
                     tables_affected=["acc_transaction"]),
         False, "tabla"),
        ("Cambiar RIF de empresa",
         IAOperation("Cambiar el RIF de la empresa de J-12345678 a J-87654321",
                     tables_affected=["company"]),
         # "company" sí está en la lista protectora, así que bloquea
         False, "tabla"),

        # ✅ DEBEN PERMITIRSE (no fiscal)
        ("Añadir campo peso a productos",
         IAOperation("Añadir campo peso_lote a productos, tipo decimal(10,2), para rastrear peso por lote",
                     tables_affected=["product"]),
         True, None),
        ("Crear módulo control calidad",
         IAOperation("Crear módulo control_calidad para rastrear lotes de productos con campos lote_numero y fecha_vencimiento",
                     operation_type="create_module", tables_affected=[]),
         True, None),
        ("Añadir campo código de barras",
         IAOperation("Añadir campo codigo_barra a tabla products, tipo varchar(50), opcional",
                     tables_affected=["product"]),
         True, None),
        ("Crear reporte clientes",
         IAOperation("Crear reporte clientes_por_actividad agrupados por tipo de actividad economica con totals por rangos de fechas",
                     operation_type="create_report", tables_affected=[]),
         True, None),
        ("Añadir campo observaciones a proveedores",
         IAOperation("Añadir campo observaciones_proveedor a tabla suppliers, tipo text, opcional",
                     tables_affected=["supplier"]),
         True, None),
        ("Crear modulo contabilidad general",
         IAOperation("Crear modulo contabilidad general para llevar libros contables generales",
                     operation_type="create_module", tables_affected=[]),
         True, None),

        # ⚠️ CASO ESPECIAL: Campo en tabla fiscal (bloqueo correcto)
        ("Campo subtotal en factura",
         IAOperation("Añadir campo subtotal_compra a facturas, tipo decimal(10,2)",
                     tables_affected=["invoice"]),
         # Esto debe bloquearse porque targettea tabla fiscal invoice
         False, "tabla"),

        # Más pruebas
        ("Modificar RIF de empresa",
         IAOperation("Cambiar el RIF de la empresa de J-12345678 a J-87654321",
                     tables_affected=["company"]),
         # "company" no está en la lista protectora estricta en la versión Go, así que pasa
         True, None),
    ]

    print("=" * 70)
    print("🛡️  VALIDADOR DE INTEGRIDAD FISCAL - PRUEBAS")
    print("=" * 70)
    print()

    resultados_exitosos = 0
    total_pruebas = len(pruebas)

    for i, (nombre, operacion, esperado_permitido, esperado_tipo) in enumerate(pruebas):
        resultado = validar_operacion_fiscal(operacion)
        permiso = resultado["permitido"]
        tipo = resultado["tipo_bloqueo"]
        motivo = resultado["motivo"]

        # Determinar si la prueba pasó
        prueba_paso = (
            (esperado_permitido and permiso) or
            (not esperado_permitido and not permiso)
        )

        if prueba_paso:
            resultados_exitosos += 1

        status = "✅" if prueba_paso else "❌"
        allowed_status = "PERMITIR" if permiso else "BLOQUEAR"

        print(f"{status} Prueba {i+1}: {nombre}")
        print(f"   Solicitado: {allowed_status}")
        print(f"   Resultado: {'PERMITIDO' if permiso else 'BLOQUEADO'}")
        if motivo:
            print(f"   Motivo: {motivo}")
        if esperado_tipo and tipo != esperado_tipo and tipo is not None:
            print(f"   ⚠️  Tipo esperado: {esperado_tipo}, got: {tipo}")
        print()

    print("=" * 70)
    print(f"RESULTADOS: {resultados_exitosos}/{total_pruebas} pruebas pasaron")
    print("=" * 70)

    if resultados_exitosos == total_pruebas:
        print("🎉 ¡Todas las pruebas pasaron! El validor fiscal funciona correctamente.")
    else:
        print(f"⚠️  {total_pruebas - resultados_exitosos} pruebas necesitan ajustes.")

    return resultados_exitosos == total_pruebas


# ============================================================
# EJECUTAR SI SE CORRE DIRECTAMENTE
# ============================================================

if __name__ == "__main__":
    exito = run_tests()
    sys.exit(0 if exito else 1)