# ERPGo — Binario Único con go:embed

**Fecha:** 27 de septiembre de 2026

> **Actualización (28 de octubre de 2026).** Lo que sigue es el diseño
> original de ese día. Hoy el binario embebe desde la raíz del repositorio
> —`embed.go`, con `//go:embed templates`, `//go:embed static` y
> `//go:embed migrations`—, así que no hay ninguna carpeta intermedia que
> mantener. La copia `assets/` que este documento manda a replicar era el
> duplicado heredado de esa primera versión (658 ficheros) y se eliminó:
> nada la importaba, el binario no la contenía y `docgen` ya la saltaba al
> escanear. Los únicos `assets/` con `go:embed` que quedan en el repo son
> los de `cmd/docgen/assets/`, las piezas del propio manual, sin relación
> con esa copia.

## Qué cambió

Todo el contenido estático ahora vive DENTRO del binario (`erpgow.exe`):

| Antes (estilo PHP) | Ahora (binario único) |
|---|---|
| `templates/` sueltos en disco | `go:embed` en `assets/templates/` |
| `static/` (CSS/JS/img) en disco | `go:embed` en `assets/static/` |
| `migrations/` en disco | `go:embed` en `assets/migrations/` |
| MIME types del registro de Windows | Tabla MIME fija en el código |

**Despliegue: solo 2 archivos** — `erpgow.exe` + `.env`. Nada más.

## Fix del bug de CSS/JS bloqueado

**Síntoma:** la web se veía sin estilos; consola del navegador:
`Refused to apply style... MIME type ('application/x-css') is not a supported stylesheet MIME type`

**Causa:** en Windows, Go lee los MIME del registro; el registro mapea
`.css → application/x-css` y `.js → application/x-js`. Con el header
`X-Content-Type-Options: nosniff` activo, Chrome bloquea ambos.

**Fix:** el servidor ahora fuerza los MIME canónicos (`text/css`,
`application/javascript`, etc.) en `interfaces/web/server.go` — ya no
depende del registro del sistema.

## Estructura

```
assets/assets.go        // go:embed de templates, static, migrations
interfaces/web/server.go   // sirve static desde el FS embebido + MIME fix
interfaces/web/render.go   // parsea templates desde el FS embebido
infrastructure/persistence/database.go  // migraciones desde el FS embebido
```

## Datos en disco (runtime, no parte de la app)

- `.env` — configuración (obligatorio junto al exe)
- `uploads/` — logos y archivos subidos por el usuario
- `data/backup/` — backups generados (antes en static/data/backup)
- `server.log` — log del servidor

## Flujo de trabajo para editar templates/assets

Las carpetas raíz `templates/`, `static/` y `migrations/` son la FUENTE y
son también lo que se embebe. Se edita y se recompila: no hay paso previo.

    go build -o erpgow.exe ./cmd/server

(Ese es el único paso. La versión original de este documento mandaba antes a
copiar las tres carpetas a `assets/`, porque en ese diseño `go:embed` vivía
dentro de esa copia y no podía subir directorios padre. Esa copia es la que
se eliminó el 28 de octubre de 2026.)

## Verificado

- Exe standalone en carpeta limpia (solo .env): todas las páginas 200
- MIME: css → text/css, js → application/javascript (bug resuelto)
- Venta POS end-to-end desde el binario standalone (factura 1006)
- Migraciones embebidas: `erpgow.exe -migrate` funciona sin archivos sueltos
- Backup: genera en data/backup/ y descarga vía /data/backup/{name}
- Auditoría: cadena SHA-256 íntegra (8 registros)
