# Por qué la suite tarda 15 minutos

Medido sobre la máquina de desarrollo, 4 de octubre de 2026. Sin esto, cualquier
mejora futura del tiempo de pruebas se hace a ciegas.

---

## 1. El reparto del tiempo

Suite completa de `interfaces/web`: **148 tests**.

| | |
|---|---|
| Suma del tiempo que declara cada test | **553 s** |
| Media por test | 3,7 s |
| Tests que tardan más de 10 s | **24** |
| Los 5 más lentos | 36 s, 35 s, 31 s, 29 s, 28 s |

Los 24 tests lentos tienen casi todos la misma causa: **cada uno levanta su
propia empresa**, y una empresa nueva significa crear una base y aplicarle las
17 migraciones.

```
crear base + 17 migraciones = 428 sentencias = 11,1 s   (medido)
```

Con 25 bases por corrida, el DDL repetido son unos 4,5 minutos. Y lo peor no es
el tiempo: es que **el trabajo es idéntico cada vez**, así que no se está
probando nada nuevo con él.

---

## 2. Lo que se intentó: clonar una base plantilla

Se aplicó el esquema **una vez** en una base plantilla
(`infrastructure/persistence/plantilla.go`) y cada base se clona con
`CREATE TABLE ... LIKE` + `INSERT IGNORE ... SELECT`.

| | |
|---|---|
| Migrar | **11,1 s** por base |
| Clonar | **3,9 s** por base |

Tres veces más rápido, con el mismo esquema y la misma semilla.

**Donde sí sirve:** en los tests de `infrastructure/persistence`, que crean su
propia base con `NuevaBaseDePrueba`. Ahí está puesto.

**Donde NO sirve, y por eso no se puso ahí:** se probó también en
`interfaces/web`, que es donde se crea el 90% de las bases. Medido:

| | |
|---|---|
| `TestSwitchingBackToOwnCodesKeepsAccounts` con migraciones | **22,8 s** |
| el mismo test con la plantilla | **25,3 s** |

Peor, no mejor. El clon paga 131 `CREATE TABLE`, que es un DDL por tabla, y las
migraciones que se ahorran ya estaban amortizadas por el camino rápido con
`multiStatements`. Comprado: DDL barato contra DDL caro.

Por eso el atajo se quedó solo donde gana. **Un rendimiento que no se mide no se
deja puesto**, por más elegante que parezca.

---

## 3. Lo que sí funciona: correr la suite en paralelo

La suite es de un solo paquete, así que `go test` la ejecuta en serie. Pero
`-run` acepta rangos, y cuatro núcleos dan cuatro procesos:

```bash
# Windows (Git Bash). Tres procesos, ~7 minutos en vez de ~16.
GOFLAGS=-p=1 go test ./interfaces/web/ -run 'Test[A-F]' -count=1 -timeout 60m -v > p1.log 2>&1 &
GOFLAGS=-p=1 go test ./interfaces/web/ -run 'Test[G-P]' -count=1 -timeout 60m -v > p2.log 2>&1 &
GOFLAGS=-p=1 go test ./interfaces/web/ -run 'Test[Q-Z]' -count=1 -timeout 60m -v > p3.log 2>&1 &
wait
grep -c '^--- FAIL' p1.log p2.log p3.log
```

`GOFLAGS=-p=1` sigue siendo obligatorio: sin él, **compilar** cuatro binarios a
la vez se come la memoria de 8 GB. La ejecución en paralelo no es lo mismo que
la compilación en paralelo.

---

## 4. Por qué las bases son el cuello de botella, y no la app

La suite crea 25 bases por corrida y **las borra al terminar** (por eso el
servidor se queda limpio). La que no se borró nunca es un fallo: se ve en
`information_schema.schemata` y se tira a mano.

Si alguna vez el servidor responde `Error 1040: Too many connections`, casi
siempre es porque una corrida anterior dejó bases detrás, no porque los tests
sean muchos.

Bases que hay que conservar:

| Base | Por qué |
|---|---|
| `erpgo` | catálogo de empresas (base de control) |
| `erpgo_default` | la empresa por defecto en desarrollo |
| `erpgow` | instalación antigua, por si hay algo que mirar |

Todo lo demás con prefijo `erpgo_t…` o `erpgo_test_…` es de un test.

---

## 5. Lo que haría más rápido de verdad

En orden de rendimiento por hora jugada:

1. **Un solo juego de migraciones para toda la suite.** Que el servidor de
   pruebas tenga un esquema ya aplicado y cada test meta sus datos en su propia
   tabla, en vez de su propia base. Es un rediseño de los tests, no un ajuste.
2. **Las 24 pruebas que crean empresas, agrupadas.** Si 8 tests comparten una
   base y se limpian entre ellos, son 8 bases en vez de 8.
3. **`t.Parallel()`** en los tests que no comparten estado. Go los ejecuta en
   paralelo hasta `GOMAXPROCS`. Es una línea por test, pero exige que no se
   pisen al borrar bases.