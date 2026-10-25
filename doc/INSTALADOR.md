# El instalador

`instalar.bat`, en la raíz del programa. Un doble clic.

---

## Qué hace, en orden

| Paso | Qué hace | Si falla |
|---|---|---|
| **1** Configuración | Si no hay `.env`, lo genera apuntando al MySQL que va con el programa (puerto 3307, sin clave). Si ya hay, **no lo toca** | — |
| **2** Puerto | Busca el primer puerto libre entre 8080 y 8099, detiene el ERP anterior, compila y comprueba que hay Go | Dice que falta Go, con la versión |
| **3** Base de datos | `erpgow.exe --migrate`: arranca el MySQL incluido si no hay ninguno escuchando, crea la base de control, aplica las 17 migraciones (428 sentencias) y sale | Explica las dos causas habituales: clave que no es la de `DB_PASSWORD`, o puerto ocupado por algo que no es MySQL |
| **4** Servidor | Abre el navegador en el puerto elegido y arranca el ERP | — |

El puerto se pasa por **el entorno**, no escribiendo en el `.env`: así el
`.env` de una instalación no queda con un puerto que solo vale en el equipo
donde se instaló, y el programa lo lee del entorno por delante del fichero.

---

## Verificado de punta a punta

No basta con que el script se vea bien. Se ejecutó sobre una copia limpia del
programa en un directorio vacío, contra un MySQL de verdad:

```
[1/4] Configuración: se usa el .env que ya existe.
      Servidor en el puerto 8080

[2/4] Compilando...
      Compilacion OK

[3/4] Preparando la base de datos...
Database: MySQL en 127.0.0.1:3306 (base de control: erpgo_instalprueba)
Migrations (MySQL): 17 archivos aplicados, 0 ya aplicados, 428 sentencias, 0 avisos
      Base de datos lista

[4/4] Iniciando el servidor...
2026/10/04 08:43:17 orquestador de escrituras activo para default
ERPGo server starting on http://127.0.0.1:8080
```

Y con el servidor ya en pie, el login por HTTP:
`POST /login` con `admin` / `Admin@2026` → **302 a `/`**, y `GET /login` → **200**.

---

## Lo que estaba roto y se arregló

1. **`instalar.bat` no arrancaba nada.** El último paso era
   `erpgow.exe --install`, y ese flag ya no existe (hace años que el esquema se
   aplica al arrancar). El programa de Go, al ver un flag desconocido, imprime
   un error y sale con código 2: el instalador terminaba en un error sin
   explicación.
2. **Paréntesis dentro de un bloque `if (...)`.** El texto
   `# (Laragon, XAMPP, el de la empresa)` estaba dentro del bloque que genera el
   `.env`, y `cmd` lo leía como un cierre de grupo:
   `No se esperaba los en este momento`. El script no llegaba ni al paso 1.
3. **Dependencias del PATH.** `find`, `findstr` y `timeout` se llamaban por su
   nombre corto, que en un entorno con Unix en el PATH resuelven a las
   herramientas de Unix, no a las de Windows. Ahora se llaman por su ruta
   completa dentro de `%SystemRoot%\System32`, y el binario del ERP como
   `.\erpgow.exe`.
4. **`>nul` convertido en `>/dev/null`.** En un `.bat`, `> /dev/null` no
   descarta nada: crea un fichero. Ahora el fichero está en CRLF y se verificó
   que no le queda ninguna ruta de Unix.

---

## Lo que NO hace el instalador (y hay que decirlo)

- **No trae el `mysqld` dentro.** El programa busca un MySQL 8 en su carpeta
  `mysql\`, en `C:\xampp\mysql\bin`, en `C:\laragon\bin\mysql` y en las rutas
  habituales, y arranca **ese** con los datos del ERP. En una máquina limpia sin
  MySQL, el instalador se detiene en el paso 3 con un mensaje que lo dice y
  ofrece dos salidas: instalar MySQL 8, o apuntar `DB_HOST`/`DB_PORT` a uno que
  ya exista. Empaquetar un `mysqld.exe` (250 MB) dentro del instalador es una
  decisión de distribución que no está tomada.
- **No hace copia de seguridad de nada.** Se supone que no hay nada que
  respaldar en una instalación nueva.
- **No deja el programa instalado como servicio.** El servidor arranca en la
  ventana de consola y se detiene al cerrarla. Para que arranque con Windows
  hace falta un servicio, y es otro trabajo.

---

## Para quien instala en la máquina de un cliente

1. Copiar la carpeta completa.
2. Doble clic en `instalar.bat`.
3. Entrar en `http://localhost:8080` con `admin` / `Admin@2026` y **cambiar esa
   clave en el primer minuto**.
4. Si el cliente ya tenía MySQL, poner sus datos en el `.env` **antes** de
   ejecutar el instalador. Con `DB_AUTO_START=0`, el programa no arranca ningún
   servidor: usa el de la empresa y solo lo abre.