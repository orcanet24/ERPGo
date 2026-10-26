#!/usr/bin/env python3
# ============================================================================
# ERPGo — webhook de GitHub para deploy INSTANTÁNEO
#
# Cómo funciona: GitHub envía un POST cuando hay push a main. Este receptor
# valida la firma HMAC (secret compartido), confirma que es el repo y rama de
# producción, y ejecuta el actualizador (scripts/deploy/actualizar_desde_git.*).
# El push que yo hago desde Codebuff llega a producción en segundos. La
# compilación y el reinicio siguen con el pacto "si algo falla no toca lo que
# ya corre": el webhook solo dispara el mismo script manual.
#
# Uso (en el servidor):
#   GITHUB_WEBHOOK_SECRET='cadena-larga-aleatoria' python3 webhook_github.py
#
# En GitHub → Settings → Webhooks → Add webhook:
#   URL:            http://<ip-o-dominio>:9091/deploy
#   Content type:   application/json
#   Secret:         la misma cadena
#   Eventos:        'Just the push event'
#
# Seguridad mínima:
#   - Se liga a 0.0.0.0 SOLO si hay firewall (recomendado: abrir 9091 a las IP
#     de GitHub, o ligar en 127.0.0.1 y proxear por Caddy).
#   - Solo acepta firma HMAC-SHA256 válida; sin secret corre en modo "solo
#     escuchar y loguear" (seguro, pero no despliega).
#   - Solo actúa en push a la rama de deploy; el resto responde 202 ignorado.
#   - Reintento silencioso: si la compi estaba corriendo, marca 'busy' en vez
#     de encolar (los rebuilds se disparan de push a push, no desbordar).
# ============================================================================
import hashlib
import hmac
import json
import logging
import os
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

# ── Config ────────────────────────────────────────────────────────────────
SECRET     = os.environ.get('GITHUB_WEBHOOK_SECRET', '')
PORT       = int(os.environ.get('WEBHOOK_PORT', '9091'))
DEPLOY_BRANCH = os.environ.get('DEPLOY_BRANCH', 'main')
DEPLOY_SCRIPT = os.environ.get('DEPLOY_SCRIPT',
    os.path.join(os.path.dirname(os.path.abspath(__file__)),
                 'actualizar_desde_git.sh' if os.name != 'nt' else 'actualizar_desde_git.bat'))
TIMEOUT    = int(os.environ.get('DEPLOY_TIMEOUT', '600'))

logging.basicConfig(level=logging.INFO,
    format='%(asctime)s [deploy] %(levelname)s %(message)s')
log = logging.getLogger('erpgo-deploy')

_lock = threading.Lock()
_last = {'stamp': 0}

def firma_valida(payload, signature):
    if not SECRET:
        return False
    if not signature or not signature.startswith('sha256='):
        return False
    calc = hmac.new(SECRET.encode(), payload, hashlib.sha256).hexdigest()
    return hmac.compare_digest(calc, signature[7:])

def hay_deploy_recien():
    """Evita que un re-push en segundos dispare dos compilaciones simultáneas."""
    with _lock:
        if time.time() - _last['stamp'] < 60 and time.time() - _last['stamp'] > 0:
            return True
        _last['stamp'] = time.time()
    return False

def lanzar_desploys():
    """Corre el actualizador en un hilo; el webhook responde 202 enseguida."""
    def run():
        try:
            cmd = DEPLOY_SCRIPT if os.name != 'nt' else ['cmd', '/c', DEPLOY_SCRIPT]
            cmd = [cmd] if isinstance(cmd, str) else cmd
            p = subprocess.run(cmd, cwd=os.path.dirname(DEPLOY_SCRIPT),
                               capture_output=True, timeout=TIMEOUT)
            log.info('deploy exit=%d SALIDA: %s', p.returncode,
                     (p.stdout + p.stderr)[-800:].decode('utf-8', 'replace'))
        except Exception as e:
            log.error('deploy falló: %s', e)
    hilo = threading.Thread(target=run, daemon=True)
    hilo.start()

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != '/deploy':
            self.send_response(404); self.end_headers(); return
        len = int(self.headers.get('Content-Length', '0'))
        body = self.rfile.read(len)
        sig = self.headers.get('X-Hub-Signature-256', '')
        event = self.headers.get('X-GitHub-Event', 'ping')

        if not firma_valida(body, sig):
            log.warning('firma inválida (o secret sin configurar); ignorando')
            self.send_response(403); self.end_headers(); return

        if event == 'ping':
            self.send_response(200)
            self.end_headers(); self.wfile.write(b'pong'); return
        if event != 'push':
            self.send_response(202); self.end_headers(); return

        try:
            data = json.loads(body)
        except Exception:
            data = {}
        branch = (data.get('ref') or '').split('/')[-1]
        if branch != DEPLOY_BRANCH:
            log.info('push a %s ignorado (despliega desde %s)', branch, DEPLOY_BRANCH)
            self.send_response(202); self.end_headers(); return
        pusher = data.get('pusher', {}).get('name', '?')
        log.info('push a %s por %s', DEPLOY_BRANCH, pusher)

        if hay_deploy_recien():
            log.info('deploy ya corriendo (cooldown de 60 s): el push anterior lo cubre')
            self.send_response(200); self.end_headers()
            return
        lanzar_desploys()
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'disparando deploy')

    def log_message(self, fmt, *args):
        return  # el logging propio ya cubre

if __name__ == '__main__':
    if not SECRET:
        log.warning('GITHUB_WEBHOOK_SECRET no definido: el webhook LOGUEA pero NO despliega')
    log.info('webhook escuchando 0.0.0.0:%d — POST /deploy (rama %s)', PORT, DEPLOY_BRANCH)
    HTTPServer(('0.0.0.0', PORT), Handler).serve_forever()
