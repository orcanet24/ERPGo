#!/usr/bin/env python3
"""Replica el checkbox de /settings/fiscal con la MISMA cadena de CSS
del layout y diagnostica por qué no se marca."""
import time, http.server, socketserver, threading, functools
from playwright.sync_api import sync_playwright

PORT = 8079
Handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory="/workspace/chats/932c1a9cafe7d224bcd56742")
httpd = socketserver.TCPServer(("127.0.0.1", PORT), Handler)
threading.Thread(target=httpd.serve_forever, daemon=True).start()

css_chain = ["/static/css/bootstrap.min.css", "/static/css/custom.min.css",
             "/static/css/style.css", "/static/css/erpgow-refined.css", "/static/css/erpgow-tema.css"]
links = "\n".join(f'<link href="{c}" rel="stylesheet">' for c in css_chain)

page_html = f"""<!DOCTYPE html><html><head>
<script src="/static/js/jquery-3.4.1.min.js"></script>
<script src="/static/js/bootstrap.min.js"></script>
{links}
</head><body><div class="content-wrapper"><div class="panel-body">
<form id="fsForm">
<div class="checkbox"><label>
<input type="checkbox" name="igtf_enabled" value="1">Percibir IGTF automáticamente
</label></div>
</form></div></div></body></html>"""

with open("/workspace/chats/932c1a9cafe7d224bcd56742/artifacts/test_checkbox.html", "w") as f:
    f.write(page_html)

with sync_playwright() as p:
    b = p.chromium.launch()
    page = b.new_page()
    errors = []
    page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    page.goto(f"http://127.0.0.1:{PORT}/artifacts/test_checkbox.html")
    page.wait_for_timeout(500)

    cb = page.locator('input[name=igtf_enabled]')
    box = cb.bounding_box()
    print("bounding input:", box)
    before_checked = page.evaluate("document.querySelector('input[name=igtf_enabled]').checked")
    print("checked antes:", before_checked)

    # Click nativo de playwright (activación estándar)
    cb.click()
    page.wait_for_timeout(200)
    print("checked tras click input:", page.evaluate("document.querySelector('input[name=igtf_enabled]').checked"))

    # ¿Qué hay encima del CENTRO del input? (colador de hit-testing)
    hit = page.evaluate(f"""() => {{
        const el = document.querySelector('input[name=igtf_enabled]');
        const r = el.getBoundingClientRect();
        const pts = {{}} ;
        for (const [k,x,y] of [['centro', r.left+r.width/2, r.top+r.height/2],
                               ['esquina-sup-l', r.left+3, r.top+3],
                               ['centro-x-1', r.left+1, r.top+r.height/2]]) {{
            const top = document.elementFromPoint(x,y);
            pts[k] = top ? top.tagName + '.' + (top.className||'') + ' (op:' + getComputedStyle(top).opacity + ')' : 'null';
        }}
        return pts;
    }}""")
    print("hit-testing:", hit)

    # Estilos computados del input y del label
    st = page.evaluate("""() => {
        const el = document.querySelector('input[name=igtf_enabled]');
        const lab = el.closest('label');
        const labrev = getComputedStyle(lab);
        return {
           input_opacity: getComputedStyle(el).opacity,
           input_pos: getComputedStyle(el).position, input_w: el.offsetWidth, input_h: el.offsetHeight,
           label_pos: labrev.position, label_padleft: labrev.paddingLeft,
           before_ml: labrev.getPropertyValue('margin-left'),
        };
    }""")
    print("computed:", st)
    if errors:
        print("ERRORES JS:", errors[:5])
    b.close()
