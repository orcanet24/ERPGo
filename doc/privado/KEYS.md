# CLAVES Y SECRETOS — DOCUMENTO PRIVADO

> **NO SUBIR A NINGUN REPOSITORIO. NO DISTRIBUIR CON EL BINARIO.**
> Este archivo guarda material que da acceso a los datos de los clientes.
> Vive solo en la maquina de desarrollo y en el sitio donde se entregue.

---

## FieldKey de SQLite (SQLCipher)

```
caae4c3c5cee440d10b89a315ea01bb0
```

- **32 caracteres** = 16 bytes = 128 bits.
- Es la llave con la que se cifra la base de datos de cada empresa
  (`data/empresas/<code>.sqlite3`).
- Va embebida en el binario de entrega. Ver la nota de abajo.

### Copias de seguridad

Guardar este archivo y la llave en un sitio **distinto** del que se
guarda el binario. Si se pierde la llave, las bases cifradas no se abren
y no hay recuperacion posible: no es un hash, es una llave.

---

## Que protege y que no

Decidido y no se discute mas. Queda escrito solo para que nadie lo lea
despues como si significara mas de lo que significa:

- La llave va DENTRO del binario. Quien tenga el `.exe` puede extraerla.
- Lo que se consigue es **subir el coste** de abrir la base con un cliente
  externo, no hacerlo imposible.
- El acceso de solo lectura para tecnicos se hara por el puerto tipo MySQL
  que se describe en SEGURIDAD.md, no abriendo el fichero.

---

## Que falta hacer con esta llave

1. Conectarla a SQLCipher en `infrastructure/persistence/database.go`
   (`OpenSQLite`). Hoy no recibe llave.
2. Migrar las bases que ya existen en claro. Sin perder datos.
3. **Reset de llave desde la aplicacion.** Si un cliente la pierde y no
   hay como recuperarla, se ha perdido su negocio. Es lo unico de esta
   lista que es urgente.

## Sistema de encolado de escrituras

La llave tiene que abrir tambien el binario de cola (`data/cola/`), que
por lo mismo tiene que ir cifrado y no ser legible con un editor.
Misma llave o una derivada: se decide al implementarlo.