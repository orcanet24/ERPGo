# Plantilla de módulo nuevo del ERPGo

**Para qué es esto:** para que crear un módulo no dependa de que alguien
recuerde los 9 pasos. Copia, sustituye `<MODULO>`, avanza en orden.

La norma completa, con el porqué de cada regla, está en
[PATRON-UNICO-DE-DESARROLLO.md](../../PATRON-UNICO-DE-DESARROLLO.md). Este
README es solo la mecánica.

---

## Antes de copiar nada: cuatro preguntas

Una plantilla que se copia sin pensar produce el módulo equivocado más rápido.
Estas cuatro se contestan **antes**, en el orden:

1. **¿Cuál es la regla de negocio que este módulo reemplaza o no reemplaza?**
   Si es "nada de lo que hay", es una pantalla. Si reemplaza un cálculo, es un
   módulo: y entonces el cálculo viejo tiene que quedarse funcionando igual
   hasta que el nuevo esté probado. Un módulo de costos que cambia el CPP de
   todos los productos sin querer no es un módulo nuevo, es una migración de
   datos disfrazada.

2. **¿Qué tabla del ERP va a leer y a escribir?** Anota los nombres **exactos**,
   verificados contra la base, no de memoria. Escribir `hr_salary` cuando la
   tabla se llama `hr_salary_assignment` produce un módulo que no da error y no
   encuentra nada.

3. **¿Quién mira los datos?** Si es una persona, pantalla. Si es la IA, hace
   falta el paso 7 y el 8 de la whitelist. Casi siempre son las dos cosas.

4. **¿Qué número tiene que salir bien?** Un módulo sin un número que cuadrar es
   una pantalla bonita. "El total de la factura debe coincidir con la
   contabilidad" es un criterio de aceptación; "que guarde" no lo es.

---

## Qué copiar y en qué orden

| Orden | Copiar de aquí | A | Qué hacer |
|---|---|---|---|
| 1 | `schema.sql` | `migrations/schema_z_<MODULO>.sql` | Tus tablas. Aplica **dos veces** y comprueba que las dos salen con 0 |
| 2 | `servicio.go.txt` | `interfaces/web/<MODULO>.go` | El cálculo puro primero, sus tests después |
| 3 | `modulo_test.go.txt` | `interfaces/web/<MODULO>_test.go` | Los tests puros. **Ejecútalos aquí**, antes de seguir |
| 4 | `handlers.go.txt` | `interfaces/web/<MODULO>_handlers.go` | Las rutas |
| 5 | `plantilla.html` | `templates/<MODULO>.html` | La pantalla |
| 6 | `menu_layout.html` | `templates/layout.html` | El trozo del sidebar |
| 7 | *(ya en el paso 1)* | — | El `INSERT IGNORE INTO sub_module` estaba en `schema.sql` |
| 8 | `ai_catalog.txt` | `ai_schema_catalog.go` y `ai_schema_introspect.go` | Las dos líneas de la IA |

**El paso 3 antes que el 4 es deliberado.** Los tests puros tardan
milisegundos. Si los escribes después de las rutas, la tentación es no
escribirlos porque ya hay mucho hecho, y entonces el módulo entra sin pruebas.

---

## Los 9 pasos, que no son 8 ficheros

| # | Qué | Dónde |
|---|---|---|
| 1 | Tablas | `migrations/schema_z_<MODULO>.sql` |
| 2 | Servicio y rutas | `interfaces/web/<MODULO>.go` |
| 3 | **Llamar** a `register<Modulo>Routes()` | `interfaces/web/server.go`, dentro de `NewServer` |
| 4 | Pantalla | `templates/<MODULO>.html` |
| 5 | Sección del menú | `templates/layout.html` |
| 6 | Permiso del menú | `sub_module`, en el `schema.sql` |
| 7 | **Whitelist de la IA** | `aiCatalogWhitelist` |
| 8 | **Departamento de la IA** | `aiDeptDeTabla` |
| 9 | Tests | `interfaces/web/<MODULO>_test.go` |

Los pasos **3, 6, 7 y 8** son los que no dan error si se olvidan. Por eso
existen en la lista.

---

## Antes de decir que está terminado

```bash
set -a && . ./.env && set +a      # los tests leen la configuración de aquí

go build ./...                                            # sin salida
go vet ./interfaces/web/                                 # sin salida
gofmt -l interfaces/                                     # sin salida

go test ./interfaces/web/ -run "<MODULO>" -count=1 -v
go test ./interfaces/web/ -run TestNingunDepartamentoVeTablasHuerfanas -count=1
go test ./interfaces/web/ -run TestMapaPorRolVieneRecortado -count=1
go test ./interfaces/web/ -run TestEveryTemplateRendersWithLayout -count=1
go test ./interfaces/web/ -run TestCadaPaginaSePintaEnTemaOscuro -count=1
go test ./interfaces/web/ -run TestNingunaPlantillaEscribeColorEnLinea -count=1
```

> **Sobre los pipelines:** `cmd | grep -v algo; echo $?` devuelve el estado de
> `grep`, no el de `cmd`. Un test que falla puede aparecer como verde. Usa
> `cmd | head; echo ${PIPESTATUS[0]}`.

Y la prueba que no es un comando: **abre el módulo desde el menú con un usuario
real y en tema oscuro.** Si no se llega por el menú, el paso 5 o el 6 está mal,
pase lo que pase el build.

---

## Lo que esta plantilla no puede hacer por ti

- **No verifica que el esquema que escribiste sea el que hay.** Para eso:
  `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA='erpgo_default'`
  y comparar con lo que crees.
- **No inventa los datos que el cálculo necesita.** Si tu módulo necesita un
  rendimiento, una merma medida o una unidad realmente recibida, los tests solo
  comprueban que la fórmula sea correcta. Que el número sea verdadero depende de
  que alguien haya pesado la canal.
- **No decide el método de costeo, el nombre de los campos ni el modelo de
  permisos.** Esas son decisiones de negocio y hay que tomarlas antes de
  escribir la primera línea de la migración.