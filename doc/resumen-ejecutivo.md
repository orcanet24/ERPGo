# Resumen Ejecutivo — ERPGo Migración

**Fecha:** 27 de septiembre de 2026  
**Proyecto:** ERPGo (migración de SalesERP v12.13 PHP a Go + MySQL)

---

## ¿Qué es ERPGo?

Un ERP completo para PYMES venezolanas, desarrollado en Go con MySQL, que replica y mejora el sistema original SalesERP en PHP.

## Estado: OPERATIVO (95%)

| Componente | Estado |
|------------|--------|
| Build Go | ✅ Compila sin errores |
| Módulos contables | ✅ Libro Diario, Mayor, Balances |
| Facturación/POS | ✅ Con asientos contables automáticos |
| Compras | ✅ Con asientos automáticos |
| Clientes/Proveedores | ✅ CRUD + cuentas COA |
| Productos/Inventario | ✅ Stock automático |
| **Libros Legales Venezuela** | ✅ NUEVO (Kárdex, IVA, ISLR) |
| TXT/XML SENIAT | ✅ NUEVO |
| Dashboard | ✅ Info boxes |
| Usuarios/RBAC | ✅ Roles y permisos |

## Inicio Rápido

```bash
# Opción 1: .bat (recomendado)
doble clic en iniciar.bat

# Opción 2: manual
go build -o erpgow.exe ./cmd/server
.\erpgow.exe
```

**Acceso:** http://localhost:8080  
**Login:** admin / Admin@2026

## Módulos Contables (ya existían)
- Plan de Cuentas (COA) — 82 cuentas
- Comprobantes (DV, CV, JV, CONTRA)
- Libro Diario → doble partida automática
- Libro Mayor por cuenta
- Balance de Comprobación (cuadrado)
- Estado de Resultados
- Balance General

## Libros Legales Venezuela (nuevos)

### Kárdex Art. 177 RISLR
- Costo Promedio Ponderado
- 12 columnas (Fecha, Concepto, Entradas, Salidas, Saldo)
- Versión imprimible para fiscalización

### Libro de Compras (IVA)
- Reporte mensual de compras con IVA desglosado
- Base imponible, IVA, retenciones

### Libro de Ventas (IVA)
- Reporte mensual de ventas con IVA
- Base imponible, IVA, retenciones

### TXT Retenciones IVA
- Archivo de 16 columnas formato SENIAT
- Tab-delimitado, CRLF, listo para declarar

### XML Retenciones ISLR
- Estructura XML oficial SENIAT
- Códigos de concepto del Decreto 1808

## Próximos Pasos (Fase 2)

1. IGTF 3% (pagos en divisas)
2. Tasa BCV + Diferencial Cambiario
3. Auditoría e inalterabilidad
4. Nómina/Parafiscales

---

**Documentación completa:** doc/bitacora-migracion.md  
**Planes futuros:** doc/plan-nivel-erp.md, doc/plan-libros-legales-venezuela.md
