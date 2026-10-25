# Tertulia #3 - Modularidad en Go vs PHP: Arquitectura para Distribución Selectiva

**Fecha**: 2026-10-04
**Relacionado**: [Tertulia #1 - Arquitectura Hexagonal](2026-10-04-arquitectura-hexagonal-mysql.md), [Tertulia #2 - Plan Sistema Costos](2026-10-04-plan-sistema-costos.md)

## Conversación

Se discutió la necesidad de modularizar ERPGo para permitir distribución selectiva de módulos (facturación, inventario, contabilidad, ecommerce) sin requerir que el cliente instale todo el sistema. El problema central es que Go compila a un binario monolítico que incluye todo el código, mientras que PHP permite carga selectiva de módulos pero es más lento para cálculos complejos.

## Problema Técnico

### Go - Un Binario, Todo Incluido
```
$ go build -o erpgo
$ ./erpgo --puedes-quitar-contabilidad? no
```
- Todo el código de costos, contabilidad, IA, ecommerce se compila en un único binario
- No existe mecanismo nativo de "plugin" confiable entre plataformas
- El cliente descarga el monolito completo aunque solo use facturación
- Peso del binario crece con cada módulo añadido

### PHP - Flexibilidad de Carga
```php
include('modules/contabilidad.php'); // Se carga solo si se necesita
```
- Carga selectiva de archivos/módulos
- Fácil de "desconectar" módulos
- Pero: rendimiento insuficiente para cálculos de costeo, IA, concurrencia

## Estado Actual del Código

```
cmd/
  server/          ← Binario principal (todo incluido)
  docgen/          ← Herramienta separada (existe!)
  loadtest/        ← Test de carga
  limpiar/         ← Limpieza de datos
  estress/         ← Test de estrés

interfaces/web/    ← Rutas HTTP (todo mezclado)
infrastructure/    ← Adaptadores (SQLite, MySQL)
migrations/        ← Esquemas base de datos
```

## Propuesta #1: Microservicios Internos (Recomendada para Go)

### Arquitectura
```
Instalación ERPGo-Core (Go binario)           ← Sistema base
├── erpgo-server (Go binario)                 ← Core: usuarios, auth, config
├── erpgo-facturacion (Go binario)             ← Sólo facturación + clientes
├── erpgo-inventario (Go binario)             ← Inventario + costos
├── erpgo-contabilidad (Go binario)           ← Contabilidad y libros
├── erpgo-ecommerce (Go binario)              ← API ecommerce
└── erpgo-ia (Go binario)                     ← Agentes IA opcional

MySQL (instancia única compartida)
```

### Comunicación entre servicios
```go
// gRPC entre microservicios
import "google.golang.org/grpc"

// Registro de servicios
services := map[string]string{
    "facturacion": "localhost:8081",
    "inventario":  "localhost:8082", 
    "contabilidad": "localhost:8083",
}

// El core consulta qué servicios están activos
// y muestra menús dinámicamente
```

### Ventajas
- Cada módulo es un binario independiente
- Escalabilidad individual (inventario bajo alta carga, facturación baja)
- Deploy selectivo: instalar solo lo necesario
- Tests aislados por servicio
- Comparten DB y config

### Desventajas
- Arquitectura de red más compleja
- Necesita orquestación (docker-compose o systemd)
- Latencia entre servicios

## Propuesta #2: Go + PHP Híbrido

### Arquitectura
```
ERPGo-Serve (Go binario - Core y rendimiento)
├── core/                   ← PHP para UI ligera
│   ├── modules/
│   │   ├── facturacion/    ← PHP + JS
│   │   └── inventario/     ← PHP + JS  
│   └── erpgo-apache.exe    ← Apache integrado
├── erpgo-core (Go binario) ← Lógica de negocio y cálculos
├── erpgo-costos (Go binario) ← Cálculos de costeo (pesado)
└── erpgo-ia (Go binario)   ← Agentes IA (pesado)
```

### Ventajas
- Go: rendimiento para cálculos complejos
- PHP: carga selectiva de módulos UI
- Instalador descarga PHP + Go según licencia
- Fácil de empaquetar (Apache + PHP embebido)

### Desventajas
- Complejidad técnica mixta
- Mantener 2 stacks tecnológicos

## Propuesta #3: Plugins con WebAssembly (Futuro)

### Arquitectura
```
erpgo-core (Go binario con WASM runtime)
├── facturacion.wasm        ← Módulo cargado dinámicamente
├── contabilidad.wasm       ← Opcional, cargado bajo demanda
├── inventario.wasm         ← Pesado, casi siempre presente
└── ecommerce.wasm          ← Solo para ecommerce
```

### Comunicación
```javascript
// En la UI
const module = await WebAssembly.instantiateStreaming(fetch('costos.wasm'));
module.instance.exports.calculateCost(...); // Llamada a Go compilado a WASM
```

### Ventajas
- Modules descargables bajo demanda
- Aísla lógica de negocio
- Potencial para cálculos paralelos en browser

### Desventajas
- WASM aún madura poco en ecosistema Go
- Debugging complejo
- Límite de recursos en browser

## Recomendación para Discussion

### Opción A: Comenzar con Microservicios Internos (Go)
```bash
# Instalador descarga según licencia:
erpgo-installer.sh --modules=facturacion,inventario
# Descarga:
# - erpgo-server (core)
# - erpgo-facturacion
# - erpgo-inventario
# Omite: contabilidad, ecommerce, ia
```

### Opción B: Go monolítico + Feature Flags (Práctico)
```go
// en lugar de modularizar binarios, modularizar lógica
type ModuleConfig struct {
    Accounting     bool `env:"ERP_ACCOUNTING"`
    Inventory      bool `env:"ERP_INVENTORY"`
    Ecommerce      bool `env:"ERP_ECOMMERCE"`
    AI             bool `env:"ERP_AI"`
}

// UI muestra menús condicionalmente
if config.Accounting {
    router.HandleFunc("/accounting/*", accountingHandler)
}
```

**Ventaja**: Mantiene binario único pero código está modularizado
**Desventaja**: Todo el código va incluido aunque no se use

## Consideraciones Comerciales

| Modelo | Instalación cliente | Pesabilidad | Escalabilidad | Complejidad |
|--------|-------------------|-------------|---------------|-------------|
| Monolítico | Simple (1 binario) | Baja | Limitada | Baja |
| Microservicios | Media (orquestación) | Controlada | Alta | Alta |
| Go+PHP | Media | Baja-Media | Alta | Media-Alta |
| WebAssembly | Simple (1 binario + .wasm) | Baja | Media-Alta | Alta (futuro) |

## Preguntas para Discussion del Equipo

1. **¿Los clientes realmente piden módulos separados?**
   - Muchos ERP venden "módulos" pero instalan monolitos
   - La percepción de modularidad puede ser UI, no arquitectura

2. **¿Qué tan complejo es el deployment actual?**
   - Si ya usan docker, microservicios son naturales
   - Si instalan binario directamente, monolítico es más simple

3. **¿Costos operacionales de múltiples binarios?**
   - Más binarios = más testing, más releases
   - Cada microservicio necesita logging, monitoring

4. **¿Performance vs simplicidad?**
   - Go brillante en cálculos de costeo e IA
   - ¿Justifica complejidad de microservicios?

## Conclusión Provisional

**Recomendación**: Mantener monolítico por ahora (Go binario único) pero con arquitectura hexagonal limpia que facilite futuro split. La verdadera modularidad es posible sin microservicios si:
1. Dominio bien aislado (hexagonal)
2. Feature flags para módulos
3. UI dinámica según módulos activos
4. Una sola DB con esquemas modulares

Esto deja abierta la puerta a microservicios sin el costo operacional inmediato.

## Próximos Pasos

1. Decidir: monolítico con feature flags vs microservicios desde el inicio
2. Si microservicios: definir boundary entre módulos (qué va en cada binario)
3. Si monolítico: implementar registro dinámico de rutas/menús
4. Diseñar instalador que descargue binario único + config de módulos