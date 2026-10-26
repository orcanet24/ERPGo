#!/usr/bin/env python3
"""Menú móvil: probar toggle con frame.js cargado 1 vez vs 2 veces (como el layout)."""
import http.server, socketserver, threading, functools
from playwright.sync_api import sync_playwright

PORT = 8088
Handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory="/workspace/chats/932c1a9cafe7d224bcd56742")
socketserver.TCPServer.allow_reuse_address = True
httpd = socketserver.TCPServer(("127.0.0.1", PORT), Handler)
threading.Thread(target=httpd.serve_forever, daemon=True).start()

PAGE = """<!DOCTYPE html><html><head>
<link href="/static/css/bootstrap.min.css" rel="stylesheet">
<link href="/static/css/custom.min.css" rel="stylesheet">
<link href="/static/css/style.css" rel="stylesheet">
</head><body class="hold-transition sidebar-mini">
<div class="wrapper">
  <header class="main-header">
    <nav class="navbar navbar-static-top">
      <a href="#" class="sidebar-toggle" data-toggle="offcanvas" role="button"><span class="pe-7s-keypad"></span></a>
    </nav>
  </header>
  <aside class="main-sidebar"><div class="sidebar">
    <ul class="sidebar-menu">
      <li class="treeview"><a href="javascript:void(0)"><i class="fa fa-cog"></i> <span>Config</span></a>
        <ul class="treeview-menu"><li><a href="/settings/fiscal">Fiscal</a></li></ul>
      </li>
    </ul>
  </div></aside>
  <div class="content-wrapper"><div class="content">contenido</div></div>
</div>
<script src="/static/js/jquery-3.4.1.min.js"></script>
<!-- FRAME_SCRIPTS -->
</body></html>"""

def run(page, n_frame, label):
    html = PAGE.replace("<!-- FRAME_SCRIPTS -->",
                        '<script src="/static/js/frame.js"></script>' * n_frame)
    open("/workspace/chats/932c1a9cafe7d224bcd56742/artifacts/t_movil.html", "w").write(html)
    page.goto(f"http://127.0.0.1:{PORT}/artifacts/t_movil.html")
    page.wait_for_timeout(300)
    page.set_viewport_size({"width": 375, "height": 667})
    # 1) hamburguesa
    page.click(".sidebar-toggle")
    page.wait_for_timeout(150)
    open_sidebar = page.evaluate("document.body.classList.contains('sidebar-open')")
    # 2) abrir submenú treeview
    page.click(".sidebar-menu .treeview > a")
    page.wait_for_timeout(150)
    tree_open = page.evaluate("!!document.querySelector('.treeview-menu') && getComputedStyle(document.querySelector('.treeview-menu')).display !== 'none'")
    print(f"{label}: sidebar-open={open_sidebar} treeview-abierto={tree_open}")

with sync_playwright() as p:
    b = p.chromium.launch()
    page = b.new_page(viewport={"width": 375, "height": 667})
    run(page, 1, "frame.js x1 (arreglado)   ")
    run(page, 2, "frame.js x2 (layout actual)")
    b.close()