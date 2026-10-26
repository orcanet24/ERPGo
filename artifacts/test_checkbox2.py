#!/usr/bin/env python3
"""Sondeo fino del checkbox estilo por estilo."""
import http.server, socketserver, threading, functools
from playwright.sync_api import sync_playwright

PORT = 8083
Handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory="/workspace/chats/932c1a9cafe7d224bcd56742")
socketserver.TCPServer.allow_reuse_address = True
httpd = socketserver.TCPServer(("127.0.0.1", PORT), Handler)
threading.Thread(target=httpd.serve_forever, daemon=True).start()

page_html = """<!DOCTYPE html><html><head>
<script src="/static/js/jquery-3.4.1.min.js"></script>
<script src="/static/js/bootstrap.min.js"></script>
<link href="/static/css/bootstrap.min.css" rel="stylesheet">
<link href="/static/css/custom.min.css" rel="stylesheet">
<link href="/static/css/style.css" rel="stylesheet">
<link href="/static/css/erpgow-refined.css" rel="stylesheet">
<link href="/static/css/erpgow-tema.css" rel="stylesheet">
</head><body><div class="content-wrapper"><div class="panel-body">
<form id="fsForm">
<div class="checkbox"><label>
<input type="checkbox" name="igtf_enabled" value="1">Percibir IGTF automáticamente
</label></div>
</form></div></div></body></html>"""
open("/workspace/chats/932c1a9cafe7d224bcd56742/artifacts/tc2.html", "w").write(page_html)

with sync_playwright() as p:
    b = p.chromium.launch()
    page = b.new_page()
    page.goto(f"http://127.0.0.1:{PORT}/artifacts/tc2.html")
    page.wait_for_timeout(300)
    st = page.evaluate("""() => {
        const el = document.querySelector('input[name=igtf_enabled]');
        const cs = getComputedStyle(el);
        const lab = el.closest('label');
        const r = el.getBoundingClientRect();
        return {
          display: cs.display, visibility: cs.visibility, position: cs.position,
          opacity: cs.opacity, width: cs.width, height: cs.height,
          rect: {x: r.x, y: r.y, w: r.width, h: r.height},
          offsetW: el.offsetWidth, offsetH: el.offsetHeight,
          matched_rules: []
        };
    }""")
    import json
    print(json.dumps(st, indent=1))
    b.close()
