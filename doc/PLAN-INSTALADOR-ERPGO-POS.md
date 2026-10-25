# PLAN: Instalador Autónomo ERPGo + POS

**Fecha inicio:** mañana, 2026-10-13, tarde
**Objeto:** instalador .exe aparte que deja ERPGo + POS listos para usar, sin pasos manuales.

---

## 1. Decisión de arquitectura (ya tomada)

**Un solo instalador, no dos.** Pyme y Enterprise son etiquetas de alcance inicial, no productos distintos. El instalador pregunta el alcance, configura, y el usuario puede cambiar después.

**El instalador NO pregunta:**
- Plan de cuentas detallado (se carga genérico, se ajusta después)
- Datos de RRHH (se cargan en función al uso)
- Demasiada información antes de ver el producto

**El instalador SÍ hace automáticamente:**
- Crea MySQL con el esquema completo
- Crea la empresa principal con datos básicos
- Crea el usuario admin
- Crea el plan de cuentas base SENIAT
- Deja el POS listo con primer bloque fiscal reservado

---

## 2. Alcance del instalador

### 2.1 Preguntas al usuario (mínimas)

| Pregunta | Opciones | Impacto |
|----------|----------|---------|
| Tipo de empresa | Pyme / Enterprise | Solo etiqueta de alcance inicial |
| ¿Activar contabilidad? | Sí / No | Crea plan de cuentas SENIAT |
| ¿Activar RRHH? | Sí / No | Crea estructura básica de RRHH |
| ¿Multi-empresa desde inicio? | Sí / No | Crea empresa "default" + permite crear más |
| Datos de la empresa | Nombre, RIF, dirección, teléfono | Se guardan en company_information |
| Credenciales admin | Username + password | Se hashea con bcrypt |

### 2.2 Lo que el instalador hace solo (sin preguntar)

1. Verifica/instala MySQL si no está
2. Crea la base de control (`erpgo`)
3. Crea la base del tenant (`erpgo_default`)
4. Aplica todas las migraciones (34 archivos)
5. Crea el plan de cuentas base (si contabilidad activada)
6. Crea el usuario admin con bcrypt
7. Crea la empresa principal con los datos del usuario
8. Crea el POS config (API key, terminal ID, bloque fiscal)
9. Genera el binario POS con la config embebida
10. Deja todo listo para arrancar

---

## 3. Estructura del proyecto

```
C:\Proyectos\GO\ERPGo-Installer\
├── go.mod                          # module erpgo-installer
├── main.go                         # entry point
├── cmd/
│   └── installer/
│       └── main.go                 # CLI del instalador
├── internal/
│   ├── wizard/                     # UI del instalador (preguntas)
│   │   ├── wizard.go               # flujo principal
│   │   ├── questions.go            # definición de preguntas
│   │   └── summary.go              # resumen antes de instalar
│   ├── database/                   # manejo de MySQL
│   │   ├── mysql.go                # conexión, creación de BD
│   │   ├── migrations.go           # aplica migraciones
│   │   └── seed.go                 # datos iniciales
│   ├── company/                    # creación de empresa
│   │   ├── company.go              # company_information
│   │   ├── admin_user.go           # usuario admin
│   │   └── fiscal_series.go        # series fiscales
│   ├── pos/                        # configuración POS
│   │   ├── config.go               # config.json del POS
│   │   ├── api_key.go              # genera API key
│   │   └── fiscal_block.go         # reserva primer bloque
│   └── ui/                         # interfaz de usuario
│       ├── tui.go                  # TUI (terminal UI)
│       └── steps.go                 # pasos visuales
├── templates/                      # plantillas de UI
│   └── wizard.html                 # si se hace web-based
├── assets/                         # recursos embebidos
│   ├── migrations/                 # copia de migraciones
│   ├── pos_config.json             # template config POS
│   └── erpgow.exe                  # binario ERP (opcional)
└── README.md
```

---

## 4. Flujo del instalador

```
┌─────────────────────────────────────────────────────────────┐
│                    INSTALADOR ERPGo + POS                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Bienvenida                                              │
│     └─> Detecta si ya hay instalación                       │
│                                                             │
│  2. Tipo de empresa                                         │
│     ├─> Pyme (1 empresa, contabilidad opcional)              │
│     └─> Enterprise (multi-empresa, contabilidad obligatoria)│
│                                                             │
│  3. Módulos                                                 │
│     ├─> Contabilidad (Sí/No)                                │
│     ├─> RRHH (Sí/No)                                        │
│     └─> Multi-empresa (Sí/No)                               │
│                                                             │
│  4. Datos de la empresa                                     │
│     ├─> Nombre                                              │
│     ├─> RIF                                                 │
│     ├─> Dirección                                           │
│     └─> Teléfono                                            │
│                                                             │
│  5. Credenciales admin                                      │
│     ├─> Username                                            │
│     └─> Password                                            │
│                                                             │
│  6. Resumen                                                 │
│     └─> Muestra todo, pide confirmación                     │
│                                                             │
│  7. Instalación                                             │
│     ├─> Crea MySQL                                          │
│     ├─> Aplica migraciones                                  │
│     ├─> Crea empresa                                        │
│     ├─> Crea admin                                          │
│     ├─> Configura POS                                       │
│     └─> Reserva bloque fiscal                               │
│                                                             │
│  8. Listo                                                   │
│     └─> Muestra credenciales y cómo arrancar                │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Decisiones técnicas

### 5.1 Lenguaje
- **Go** (mismo que ERPGo) — un solo binario, sin dependencias externas
- TUI con `bubbletea` o `termui` para la interfaz de terminal
- Opcional: web-based con `go:embed` para HTML/JS

### 5.2 MySQL
- El instalador **no instala MySQL** — asume que ya está instalado (Laragon, XAMPP, etc.)
- Si no está, da error claro con instrucciones
- Usa el driver `go-sql-driver/mysql` (ya está en el proyecto)

### 5.3 Migraciones
- El instalador **no ejecuta `erpgow.exe -migrate`**
- Aplica las migraciones directamente desde Go (lee los .sql y los ejecuta)
- Esto permite mostrar progreso paso a paso

### 5.4 POS
- El instalador **no compila el POS** — usa el binario ya compilado
- Genera el `config.json` con la API key y terminal ID
- Reserva el primer bloque fiscal vía HTTP al servidor ERP

### 5.5 Binario ERP
- Opción A: el instalador **no incluye** `erpgow.exe` — el usuario lo descarga aparte
- Opción B: el instalador **incluye** `erpgow.exe` embebido — todo en uno
- **Recomendación:** Opción B para simplicidad, pero con opción de usar un binario externo

---

## 6. Pasos de implementación (mañana tarde)

### Fase 1: Esqueleto del proyecto (1-2 horas)
1. Crear estructura de carpetas
2. `go.mod` con dependencias
3. `main.go` básico que detecta MySQL
4. TUI mínimo con bienvenida

### Fase 2: Wizard de preguntas (2-3 horas)
1. Pantalla de tipo de empresa
2. Pantalla de módulos
3. Pantalla de datos de empresa
4. Pantalla de credenciales
5. Pantalla de resumen

### Fase 3: Lógica de instalación (3-4 horas)
1. Conexión a MySQL
2. Creación de bases de datos
3. Aplicación de migraciones
4. Creación de empresa y admin
5. Configuración de POS

### Fase 4: Pruebas y ajustes (1-2 horas)
1. Instalación en limpio
2. Verificación de que todo funciona
3. Ajustes de UI
4. Documentación

---

## 7. Riesgos y mitigaciones

| Riesgo | Mitigación |
|--------|------------|
| MySQL no está instalado | Detectar y dar error claro con instrucciones |
| Migraciones fallan | Mostrar cuál falló y permitir reintentar |
| POS no se puede configurar | Permitir omitir y configurar después |
| Usuario no sabe qué responder | Valores por defecto inteligentes |
| Instalación a medias | Transacciones MySQL, rollback si falla |

---

## 8. Preguntas pendientes de responder mañana

1. ¿El instalador será TUI (terminal) o web-based?
2. ¿Incluye el binario ERP o el usuario lo descarga aparte?
3. ¿El POS se instala en la misma máquina o en otra?
4. ¿Se necesita soporte para instalación en red (servidor remoto)?

---

## 9. Estimación de tiempo

- **Fase 1 (esqueleto):** 1-2 horas
- **Fase 2 (wizard):** 2-3 horas
- **Fase 3 (instalación):** 3-4 horas
- **Fase 4 (pruebas):** 1-2 horas
- **Total:** 7-11 horas (1-2 días de trabajo)

---

## 10. Criterios de éxito

- [ ] Instalación completa en < 10 minutos
- [ ] No requiere conocimiento técnico del usuario
- [ ] Deja ERPGo listo para arrancar
- [ ] Deja POS listo para facturar
- [ ] Maneja errores con mensajes claros
- [ ] Permite reinstalar sin romper nada
