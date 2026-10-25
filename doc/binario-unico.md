# ERPGo — Binario Único con go:embed

**Fecha:** 27 de septiembre de 2026

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

Las carpetas raíz `templates/`, `static/` y `migrations/` son la FUENTE.
Después de editarlas hay que copiarlas a `assets/` y recompilar:

    cp -r templates/. assets/templates/
    cp -r static/. assets/static/
    cp -r migrations/. assets/migrations/
    go build -o erpgow.exe ./cmd/server

(Las carpetas en assets/ se duplican porque go:embed no puede subir
directorios padre: solo embebe lo que está dentro de su propio paquete.)

## Verificado

- Exe standalone en carpeta limpia (solo .env): todas las páginas 200
- MIME: css → text/css, js → application/javascript (bug resuelto)
- Venta POS end-to-end desde el binario standalone (factura 1006)
- Migraciones embebidas: `erpgow.exe -migrate` funciona sin archivos sueltos
- Backup: genera en data/backup/ y descarga vía /data/backup/{name}
- Auditoría: cadena SHA-256 íntegra (8 registros)
