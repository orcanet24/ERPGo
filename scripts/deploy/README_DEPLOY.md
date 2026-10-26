# Cómo hacer que el servidor tome el commit de GitHub más rápido

Hoy existe una tarea programada que baja el código **cada hora**: por eso ves
ese retraso. Aquí van las tres formas de arreglarlo, de la más simple a la más
automática. Las tres usan el mismo corazón: `actualizar_desde_git.sh` (Linux)
o `actualizar_desde_git.bat` (Windows) — que solo compila y reinicia cuando
REALMENTE hay commit nuevo, y nunca toca el binario que corre si algo falla.

## Requisito común (una vez, en el servidor)

```bash
# Linux                          # Windows (Git Bash / cmd)
git clone https://github.com/orcanet24/ERPGo.git /opt/erpgow
# o si ya está clonado:
cd /opt/erpgow && git remote set-url origin https://github.com/orcanet24/ERPGo.git
```

Y ajusta en el script cómo se reinicia el ERP: `RESETYPE` y `SERVICE`
(`systemctl restart erpgo`, `nssm restart erpgo`, `schtasks`…).

## Opción A — cron cada 5 minutos (la más simple, 2 minutos de trabajo)

```cron
# Linux: crontab -e
*/5 * * * * ERPDIR=/opt/erpgow RESETYPE=systemctl SERVICE=erpgo /opt/erpgow/scripts/deploy/actualizar_desde_git.sh >> /var/log/erpgo-deploy.log 2>&1
```

```bat
# Windows: Programador de tareas — reemplaza tu tarea horaria por esta:
# schtasks /create /tn "ERPGo-Deploy" /sc minute /mo 5 /tr "cmd /c C:\erpgow\scripts\deploy\actualizar_desde_git.bat"
```

`git fetch` es barato (KB): cada 5 minutos no carga nada. Retraso máximo: 5 min.

## Opción B — webhook de GitHub (instantáneo: segundos después del push)

1. En el servidor, deja corriendo el receptor (Python estándar, sin dependencias):

   ```bash
   GITHUB_WEBHOOK_SECRET='pon-una-cadena-larga-aleatoria-aqui' \
   python3 /opt/erpgow/scripts/deploy/webhook_github.py
   # Windows: python webhook_github.py (con la variable en el entorno)
   # En producción: como servicio (systemd unit o nssm) para que sobreviva reinicios.
   ```

2. Expone el puerto 9091 al exterior. La vía limpia con Caddy (que ya usas):

   ```caddyfile
   # en tu Caddyfile existente, junto a erp.orcanet1724.click
   erp.orcanet1724.click {
       # ... lo que ya tienes ...
       handle_path /deploy-github/* {
           reverse_proxy 127.0.0.1:9091
       }
   }
   ```
   y en el receptor: `WEBHOOK_PORT=9091` escucha en 127.0.0.1 cambia la línea
   `HTTPServer(('0.0.0.0', PORT)...)` por `('127.0.0.1', PORT)`.

3. En GitHub → `orcanet24/ERPGo` → Settings → Webhooks → Add webhook:
   - URL: `https://erp.orcanet1724.click/deploy-github/deploy`
   - Content type: `application/json`
   - Secret: la MISMA cadena del paso 1
   - Eventos: **Just the push event**

4. Prueba: `redeliver` el ping en la página del webhook → debe responder 200.

Cada push (incluidos los que yo haga desde aquí) dispara el actualizador:
**segundos**, no minutos. Con firma HMAC validada; sin secret no despliega.

## Opción C — a mano (para controlarse)

Tras cada commit mío, en el servidor:

```bash
/opt/erpgow/scripts/deploy/actualizar_desde_git.sh     # Linux
C:\erpgow\scripts\deploy\actualizar_desde_git.bat     # Windows
```

Retraso: lo que tardes en correrlo. Útil mientras pruebes las otras opciones.

## Reglas de seguridad que respetan los tres caminos

1. **Servicio y habitante separados**: el script nunca mata el ERP para
   compilar; compila `erpgow.binario.nuevo` y solo reemplaza al final.
2. **`git pull --ff-only`**: si alguien metió un commit conflictivo, no
   rompe el server; falla y lo ves en el log.
3. **Salud `/health` después del reinicio**: si el ERP no levanta en 60 s,
   el script lo grita (exit 1). No hay rollback automático todavía — el
   binario del commit anterior no se archiva; recomendación: copiar el
   binario viejo a `erpgow.previo` antes del replace (linea a añadir si quieres).
4. **Webhook**: sin secret = solo loguea (nunca despliega); solo la rama
   `main` dispara; cooldown de 60 s evita re-push en cascada.
5. **El chequeo de la auditoría sigue siendo del binario**: nada de esto toca
   la base; los backups automáticos diarios (backup/) siguen corriendo.

## Recomendación

- **Hoy mismo**: Opción A (cron 5 min) — te quita la hora en 2 minutos.
- **Esta semana**: migrar a Opción B (webhook) para segundos.
- Mantener Opción C como plan B documentado (si GitHub/Internet falla,
  siempre hay camino manual).

## Opción B detallada — systemd (servidor Linux, es la recomendada)

Las unidades ya vienen listas en `scripts/deploy/systemd/`. Instalación:

```bash
# 0) (una vez por servidor) donde vive el código:
git clone https://github.com/orcanet24/ERPGo.git /opt/erpgow
# Si ya está: cd /opt/erpgow && git pull origin main

# 1) Ajusta el nombre de TU servicio del ERP dentro de las unidades:
#    erpgow-deploy.service → Environment=SERVICE=<tu servicio, ej. erpgo>
#    (verifica el nombre: systemctl list-units --type=service | grep -i erp)

# 2) Deploy por CRON cada 5 min (opción conservadora):
sudo cp scripts/deploy/systemd/erpgow-deploy.service /etc/systemd/system/
sudo cp scripts/deploy/systemd/erpgow-deploy.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now erpgow-deploy.timer
journalctl -fu erpgow-deploy          # ver cada intento en vivo
systemctl list-timers | grep erpgow   # ver el estado del timer

# 3) Deploy INSTANTÁNEO por webhook (opción definitiva):
sudo cp scripts/deploy/systemd/erpgow-webhook.service /etc/systemd/system/
sudo nano /etc/systemd/system/erpgow-webhook.service   # pon tu SECRET real
sudo systemctl daemon-reload
sudo systemctl enable --now erpgow-webhook
# PRUEBA rápida en el server: curl -X POST http://127.0.0.1:9091/deploy
#  (que responda 403 es señal BUENA: el receptor está vivo y exige firma HMAC)
```

Y en NGINX (el proxy que ya usa el servidor):

```nginx
location /webhook/github {
    proxy_pass http://127.0.0.1:9090/deploy;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
sudo nginx -t && sudo systemctl reload nginx
```

GitHub → Settings → Webhooks → Add webhook:
- Payload URL: `https://orcanet1724.click/webhook/github`
- Content type: `application/json`
- Secret: el MISMO que quedó en `erpgow-webhook.service` (nunca en el repo)
- Eventos: *Just the push event*

Desde ese momento: cada push (los que hago yo o los tuyos) despliega en
segundos. El timer de 5 min queda como red de respaldo por si el webhook
falla (red, GitHub caído, etc.).
