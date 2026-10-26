### Motor de datos: MySQL (04-10, 2.ª y definitiva) — decisión vigente

> **Esta sección SUSTITUYE a "Motor de datos: SE QUEDA SQLite (04-10)".**
> Aquella decisión se tomó con un dato equivocado y está anulada. Se conserva el
> texto viejo más abajo solo como registro de por qué se cambió; **no manda**.

**Por qué se cambió.** El argumento de "SQLite no puede hacer deadlock" era
cierto y no era la pregunta. La pregunta era cuál era el coste de cambiar, y la
respuesta está medida en las 415 sentencias y los 16 ficheros `.sql` que se
aplican tal cual: **el esquema NACE de MySQL.** `migrations/schema.sql` dice
"Generated from SalesERP v12.13 install.sql" y tiene 74 apariciones de
`ENGINE=InnoDB`. Todo eso hoy pasaba por `sqlite_convert.go`, 361 líneas y 8
funciones que traducen MySQL→SQLite. Ir a MySQL **borra** esa traducción; ir a
PostgreSQL obligaría a escribir **otra** traducción, en sentido contrario. El
cambio se hizo en 4 días y salió más barato de lo que se temía.

#### Y la pregunta abierta: ¿MySQL o PostgreSQL con una sola base?

La pregunta que quedó sobre la mesa ("¿tú dices postgres y es una sola bd y ya?")
tiene respuesta, y no es una preferencia:

| | MySQL, una base por empresa | PostgreSQL, una base y esquemas |
|---|---|---|
| aislamiento por empresa | **base completa** (`erpgo_<codigo>`) | un `search_path` por conexión |
| un usuario estropea una empresa | el dano se queda en **una base**, el resto sigue entero | con `search_path` mal puesto, escribe en la de al lado |
| respaldo de una empresa | `mysqldump` de **una base** | volcado por esquema, con los objetos compartidos como problema |
| borrar una empresa | `DROP DATABASE` | se queda el esquema o hay que acordarse de los objetos sueltos |
| traducción del esquema | **ninguna** | otra capa entera, en sentido contrario |

La columna que decide es la segunda. En PostgreSQL, el aislamiento por empresa
depende de que **cada** conexión abra con el `search_path` correcto; basta un
`SET` olvidado en una ruta de código para que una factura acabe en la base de
otra empresa. Con una base por empresa, ese error **no tiene forma de ocurrir**:
el handle apunta a una base y solo puede tocar esa.

El único argumento de PostgreSQL —que se esoyan "una sola bd"— no compensa: el
ahorro es administrativo (un `CREATE DATABASE` en vez de varios) y el coste es
que el aislamiento deje de ser una propiedad estructural y pase a depender de que
nadie se equivoque.

#### Cómo queda instalado (REVISADO: el motor ya no se instala ni se arranca)

> **Esta parte SUSTITUYE al diseño del «MySQL incluido».** Aquel diseño —buscar
> un `mysqld`, inicializar un datadir con `--initialize-insecure`, arrancarlo en
> el 3307 y apagarlo al salir— **se eliminó del producto**. Dos motivos:
>
> 1. El asistente llegó a ejecutar `apt-get install mariadb-server` / `yum` /
>    `dnf` / un MSI desde el binario. Eso modifica paquetes, servicios y el
>    datadir del sistema operativo anfitrión desde una aplicación de negocio, y
>    en producción rompió servidores que ya estaban sanos.
> 2. Aun sin instalar paquetes, inicializar y levantar un motor propio crea una
>    instalación de base de datos a espaldas de quien administra la máquina.
>
> **Regla vigente: el servidor debe tener su propio MySQL o MariaDB instalado y
> arrancado como servicio. ERPGo solo se conecta.** No instala, no inicializa, no
> arranca y no apaga ningún motor. Si no responde en `DB_HOST:DB_PORT`, el
> arranque se detiene con un mensaje que explica el requisito y no toca nada del
> sistema.

1. El motor (MySQL 8+ o MariaDB 10.6+) se instala como paquete o servicio del
   sistema operativo y queda arrancado. Puerto estándar: **3306**.
2. `--setup` pide host, puerto, usuario y contraseña, **comprueba la conexión** y
   solo entonces escribe el `.env`. Si la conexión falla, permite reintentar sin
   volver a empezar.
3. El ERP crea la base de control (`erpgo`) y una por empresa
   (`erpgo_<codigo>`) con el usuario de provisión. Ese usuario necesita
   `CREATE DATABASE`, `CREATE USER` y los permisos sobre `erpgo_%`; si no los
   tiene, el error de MySQL se muestra tal cual en vez de disfrazarse.
4. El ERP **no** apaga el motor al terminar: no es suyo.

`DB_AUTO_START`, `DB_MYSQL_HOME`, `DB_MYSQL_DATA` y
`DB_MYSQL_BUFFER_POOL_MB` ya no existen; si quedan en un `.env` viejo, se
ignoran.

**Por qué el MySQL no va dentro del `.exe`.** Un servidor de base de datos son
~200 MB de código nativo que hay que mantener, actualizar y firmar aparte.
Empaquetarlo en el binario no lo hace más portátil, solo más grande y más viejo.
Y arrancarlo desde el ERP mezcla dos responsabilidades que deben estar separadas:
la aplicación no es el administrador de la infraestructura.

**SQLite queda en el repositorio** como el proyecto anterior: `database.go`,
`sqlite_convert.go` y el camino legacy siguen ahí, y se pueden volver a usar
poniendo `DB_DRIVER=sqlite`. No es código muerto: es el historial del producto y
sirve para comparar. Lo que no se hace es mantenerlo vivo a la vez.

#### Lo que costó de verdad, medido

- **415 sentencias en 16 ficheros**, aplicadas tal cual: 0 avisos, 121 tablas
  InnoDB. Segunda corrida: **0 sentencias** (idempotente por registro con
  checksum).
- Crear una base de empresa de cero: **~9 s**. Medido que **no** es la red: el
  cliente `mysql` por línea de comandos, que va sentencia a sentencia, tarda lo
  mismo (7,4 s solo `schema.sql`). Es InnoDB creando 121 tablas a ~75 ms cada
  una. El camino rápido con `multiStatements` (16 viajes en vez de 415) **sí se
  usa** —se comprueba en el log con la línea "lote rechazado"— pero en local no
  ahorra nada: ahorra 415 viajes de red, que es lo que duele cuando la base está
  en otra máquina.
- El banco de pruebas (`tools/estress`) pasa a MySQL: una base desechable por
  corrida, con nombre único y `DROP` en `defer`.

#### Los tres fallos que el cambio destapó (y que SQLite tapaba)

1. **`IFNULL(fecha, '')` es un error 1525 en MySQL.** "Sin fecha" se guarda como `NULL`; comparar una columna `DATE` con `''` no es "falso", es un error que
   tumba la pantalla entera. Apareció en 8 ficheros de producción. Ahora es
   `IFNULL(CAST(x AS CHAR),'')`. **Un banco de pruebas que no inserta filas no ve
   estos fallos**: la expresión solo se evalúa por fila.
2. **Los tests escribían en las bases reales.** `openTenantDB("default")` compone
   `erpgo_default`, que es el nombre de la base de la empresa por defecto. La
   suite estaba dando de alta "ACME C.A." en el servidor del desarrollador. Con
   SQLite no se notaba porque cada test tenía su fichero temporal; al pasar a
   MySQL, "temporal" dejó de existir. Ahora cada test lleva un prefijo único
   (`DB_TENANT_PREFIX`) y borra sus bases al terminar.
3. **El borrado de bases de prueba no borraba nada.** Buscaba el prefijo del test
   sin el `erpgo_` que antepone la regla de nombres. En dos días: **209 bases
   huérfanas** y, con ellas, `Error 1040: Too many connections` que hizo fallar
   una tanda entera de tests sin relación con bases.

Los tres son fallos **silenciosos**: el sistema "funciona" mientras destructura.
El segundo es el mas grave: los tests pasaban y lo que se ensuciaba era la base del
desarrollador.

---

<details>
<summary>Decisión anterior (04-10, ANULADA — se conserva como registro)</summary>

### ~~Motor de datos: SE QUEDA SQLite (04-10)~~ — anulada

Se planteó pasar a PostgreSQL para evitar bloqueos. **Se descarta.** Medido:

| | |
|---|---|
| ficheros con SQL directo | **64** |
| puntos de apertura de base (la costura) | **1** — `persistence/database.go:44` |
| driver | `glebarez/go-sqlite` — **puro Go, sin CGO ni gcc** |
| modelos multi-empresa | un fichero por empresa |

Un cambio de motor con esa superficie no es una configuración: es reescribir la
capa de persistencia entera, traducir el dialecto de 64 ficheros, sustituir
`sqlite_convert.go` y replantear el aislamiento por empresa. Con 5 días para la
entrega sería el cambio más arriesgado posible.

**Por qué se anuló.** El cálculo de coste estaba mal porque contó el dialecto como
traducción de MySQL→SQLite siendo que era al revés: se estaban **descartando**
las partes de MySQL del esquema, no adaptándolas. Las 30 sentencias que hubo que
reescribir (`AUTOINCREMENT`→`AUTO_INCREMENT`, `CREATE INDEX IF NOT EXISTS`→`CREATE
INDEX`, `INSERT OR IGNORE`→`INSERT IGNORE`, upserts) y las 21 columnas
`TEXT`→`VARCHAR` son **menos** que las 361 líneas que se dejaron de mantener. La
estimación estaba simplemente equivocada.

La objeción de "es arriesgado con 5 días" era correcta en su momento: no lo era
la conclusión, porque la alternativa (un ERP con el motor que su esquema no
nació para usar) es la que después cuesta.

</details>