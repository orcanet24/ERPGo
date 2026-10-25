# Tertulia #1 - Arquitectura Hexagonal y Migración a MySQL

**Fecha**: 2026-10-04

## Conversación

Se discutió el paso de SQLite a MySQL en el proyecto ERPGo. Originalmente se usaba SQLite con WAL + un encolador, lo cual era "brutal" (brutalmente bueno) para performance en tests y desarrollo interno. Sin embargo, para soportar múltiples empresas (multi-tenant) y futuras APIs/ecommerce, se decidió migrar a MySQL.

## Problemas Identificados

- SQLite + WAL + encolador funciona bien en tests internos pero no escala bien para varias empresas
- La migración a MySQL resultó compleja porque el código no estaba preparado para cambios de infraestructura
- La arquitectura actual acopla la lógica de negocio con la base de datos específica

## Solución Propuesta

Adopción de arquitectura hexagonal (puertos y adaptadores):

1. **Aislamiento de la base de datos**: con un puerto/diseñador para repositorios, cambiar entre SQLite y MySQL sería cuestión de cambiar el adaptador
2. **Testabilidad**: usar SQLite + WAL + encolador en tests sin afectar producción
3. **Escalabilidad multi-tenant**: estrategias de particionamiento en adaptadores sin tocar el dominio
4. **Transición limpia**: puertos HTTP/gRPC para futuras APIs sin acoplar lógica de negocio

## Próximos Pasos

- Definir el dominio principal (empresas, usuarios, productos, órdenes, etc.) y qué no cambia entre backend.
- Identificar los **puertos de salida**: persistencia, encolador, storage de archivos, pagos.
- Crear el primer **adaptador de persistencia** con interfaz genérica (repository pattern).
- Usar SQLite como DB de desarrollo/testing y MySQL como productivo con el mismo contrato.

## Cuestiónes Adicionales

SQLite es una excelente opción incluso para enterprise (Apple, Meta, etc. lo usan). El problema no es SQLite, es que el código debe estar preparado para cambiar fácilmente entre una BD y otra.

La arquitectura hexagonal es la solución: con la interfaz `repository` (puerto), cambiar entre SQLite, MySQL, PostgreSQL o incluso Mongo, es cuestión de implementar el adaptador correspondiente. El dominio de negocio no cambia ni un bit.

## Plan de Acción Futuro

Objetivo: pasar ERPGo a arquitectura hexagonal para poder usar SQLite, MySQL, PostgreSQL, o bases documentales según necesite cada cliente.

1. **Fase 1 - Dominio**
   - Definir el modelo de dominio principal (entidades: Company, User, Product, Order, etc.)
   - Definir puertos de salida (interfaces): Repository, Queue, Storage, PaymentGateway

2. **Fase 2 - Adaptadores SQLite**
   - Implementar repositorios para SQLite manteniendo el contrato del puerto
   - Mantener compatibilidad con código existente

3. **Fase 3 - Adaptadores MySQL/PostgreSQL**
   - Implementar adaptadores para MySQL y PostgreSQL usando el mismo contrato
   - Permite producción multi-tenant y clustering

4. **Fase 4 - Adaptadores documentales**
   - Evaluar MongoDB o similar para casos específicos (ecommerce, APIs)
   - Implementar adaptador según necesidad del cliente

## Opinión Técnica - Viabilidad

**Estimado de tiempo**: 2-3 semanas para fase 1 (dominio + puertos), 1-2 semanas por adaptador (SQLite, MySQL, PostgreSQL). Total: 4-7 semanas según complejidad del modelo.

**Viabilidad**: Alta. Es aplicable y recomendable. La clave es:
- No refactorizar todo de golpe, ir por entidades/paquetes uno a uno
- Mantener los adaptadores existentes funcionando mientras migras
- Usar el patrón "Strangler Fig": nuevas funcionalidades usan hexagonal, legacy se migra gradualmente

**Beneficios a largo plazo**:
- Cambios de BD sin tocar el core = imposible meter la pata en producción
- Tests rápidos con SQLite, prod con MySQL = mejor calidad
- Multi-tenant escalable = crece sin límites
- Clientes con diferentes necesidades BD = flexibilidad comercial

**Riesgo**: La curva de aprendizaje si el equipo nunca usó hexagonal. Pero el beneficio supera con creces el esfuerzo inicial.

## Visión de Futuro

- Arquitectura preparada para multi-tenant con particionamiento por empresa
- Infraestructura desacoplada: fácil cambio entre SQLite (dev/test) y MySQL (producción)
- Soporte para APIs externas y ecommerce futuro
- Tests rápidos y aislados usando SQLite + WAL + encolador simulado
- Código maduro y preparado para crecer sin refactorizaciones grandes