#!/usr/bin/env python3
"""Aislamiento por eliminación: CSS y JS del layout uno por uno."""
import http.server, socketserver, threading, functools, json
from playwright.sync_api import sync_playwright

PORT = 8084
Handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory="/workspace/chats/932c1a9cafe7d224bcd56742")
socketserver.TCPServer.allow_reuse_address = True
httpd = socketserver.TCPServer(("127.0.0.1", PORT), Handler)
threading.Thread(target=httpd.serve_forever, daemon=True).start()

CSS = ["/static/css/jquery-ui.min.css", "/static/css/bootstrap.min.css", "/static/css/wickedpicker.min.css",
       "/static/css/jquery-ui-timepicker-addon.min.css", "/static/css/select2.min.css",
       "/static/datatables/dataTables.min.css", "/static/css/pe-icon-7-stroke.css", "/static/css/themify-icons.css",
       "/static/plugins/toastr/toastr.css", "/static/css/bootstrap-toggle.min.css", "/static/css/custom.min.css",
       "/static/css/style.css", "/static/js/sweetalert/sweetalert.css", "/static/css/custom-rtl.css",
       "/static/css/print.css", "/static/css/erpgow-refined.css", "/static/css/erpgow-tema.css"]
JS = ["/static/js/jquery-3.4.1.min.js", "/static/js/jquery.validate.min.js", "/static/js/jquery-ui.min.js",
      "/static/js/bootstrap.min.js", "/static/js/pace.min.js", "/static/js/jquery.slimscroll.min.js",
      "/static/js/jquery-ui-timepicker-addon.min.js", "/static/js/select2.min.js",
      "/static/datatables/dataTables.min.js", "/static/js/tableHeadFixer.js", "/static/js/frame.js",
      "/static/js/bootstrap-toggle.min.js", "/static/plugins/toastr/toastr.min.js",
      "/static/js/sweetalert/sweetalert.min.js", "/static/js/custom.js", "/static/js/csrf.js",
      "/static/js/viewpref.js", "/static/js/appearance.js"]

page_html = """<!DOCTYPE html><html><head>
<link href="/static/js/sweetalert/sweetalert.css" rel="stylesheet">
<!-- CSS_HERE -->
</head><body>
<!-- JS_HERE -->
<div class="content-wrapper"><div class="panel-body">
<form id="fsForm">
<div class="checkbox"><label>
<input type="checkbox" name="igtf_enabled" value="1">Percibir IGTF
</label></div>
</form></div></div></body></html>"""

def make_file(css, js):
    cssl = "\n".join(f'<link href="{c}" rel="stylesheet">' for c in css)
    jsl = "\n".join(f'<script src="{j}"></script>' for j in js)
    html = page_html.replace("<!-- CSS_HERE -->", cssl).replace("<!-- JS_HERE -->", jsl)
    open("/workspace/chats/932c1a9cafe7d224bcd56742/artifacts/tc3.html", "w").write(html)

def test(page):
    page.goto(f"http://127.0.0.1:{PORT}/artifacts/tc3.html")
    page.wait_for_timeout(400)
    got = page.evaluate("""() => {
        const el = document.querySelector('input[name=igtf_enabled]');
        if (!el) return {gone: true};
        el.click();
        return {checked: el.checked,
                w: el.offsetWidth, h: el.offsetHeight,
                top: (() => { const r = el.getBoundingClientRect();
                    const t = document.elementFromPoint(r.x + r.width/2, r.y + r.height/2);
                    return t ? t.tagName + ((t.className && t.className.baseVal!==undefined) ? t.className : ' ' + t.className) : 'none'; })()};
    }""")
    return got

with sync_playwright() as p:
    b = p.chromium.launch()
    page = b.new_page()

    print("== base(sin css, sin js) ==")
    make_file(CSS[:3], [])
    print(test(page))

    print("== CSS acumulativo ==")
    for i, c in enumerate(CSS[:3] + CSS[3:], start=0):
        make_file(CSS[:i+1], [])
        r = test(page)
        state = (r or {}).get("checked", (r or {}).get("gone", "?"))
        if state is not True:
            print("FALLA con:", c, "->", r)

    print("== JS acumulativo (con todo el CSS) ==")
    make_file(CSS, [])
    base = test(page)
    print("con solo css:", base)
    for i, j in enumerate(JS):
        make_file(CSS, JS[:i+1])
        r = test(page)
        state = (r or {}).get("checked", "n/a")
        if state is not True:
            print("FALLA con:", j, "->", r)
    b.close()
