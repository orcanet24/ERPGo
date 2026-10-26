#!/usr/bin/env python3
"""
auditar_rutas.py — Verifica que las ventanas prometidas EXISTAN.

Cruza tres fuentes del código:
  A) Rutas realmente registradas en Go  (s.mux.HandleFunc / s.mux.Handle, *_FILES)
  B) URLs que los templates piden       (xhr.open / fetch / action= / href="/...")
  C) URLs del menú                      (INSERTs del menú en migrations/*.sql)

Y reporta:
  1. Menú → ruta no registrada      (muestre una ventana que responde 404)
  2. Template → ruta no registrada  (XHR/fetch que muere en silencio, el caso
                                     del gestor de menús antes del 26-10)
  3. Ruta registrada que nadie usa  (informativo; no es error)

Uso:  python3 scripts/auditar_rutas.py            (salida en pantalla)
      python3 scripts/auditar_rutas.py --json     (salida machine-readable)
"""
import json
import re
import sys
from pathlib import Path

RAIZ = Path(__file__).resolve().parent.parent
DIR_WEB = RAIZ / "interfaces" / "web"
DIR_TPL = RAIZ / "templates"
DIR_MIG = RAIZ / "migrations"

# ── A) Rutas registradas en Go ─────────────────────────────────────────────
RE_RUTA = re.compile(r"\bs\.mux\.HandleFunc\(\s*\"([A-Z]+ )?/?([^\"]+)\"")


def rutas_registradas():
    """Devuelve dict de ruta (sin método) → set de métodos ('*' si no declara)."""
    out = {}
    # Cualquier .mux.HandleFunc(/Handle) cuyo primer argumento sea un string,
    # con Método opcional delante y en una o varias líneas.
    pat = re.compile(r"\.mux\.Handle(?:Func)?\(\s*\"([^\"]+)\"")
    for go in RAIZ.rglob("*.go"):
        parts = go.parts
        if any(p in (".external", ".git", "doc", "static") for p in parts):
            continue
        try:
            src = go.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        for m in pat.finditer(src):
            bruto = m.group(1).strip()
            if bruto.startswith("/"):
                metodo, ruta = "*", bruto
            else:
                secciones = bruto.split(" ", 1)
                if len(secciones) != 2:
                    continue
                metodo, ruta = secciones[0], secciones[1]
            if not ruta.startswith("/") or "$" in ruta or "{{" in ruta:
                continue
            if ruta.startswith("/debug"):
                continue
            ruta_norm = re.sub(r"\{[^}]+\}", "{}", ruta).rstrip("/") or "/"
            out.setdefault(ruta_norm, set()).add(metodo)
    return out


def normalizar(u):
    u = re.sub(r"\{[^}]+\}", "{}", u)
    return ("/" + u.strip("/")).rstrip("/") or "/"


# ── B) URLs pedidas desde templates ───────────────────────────────────────
RE_XHR = re.compile(r"xhr\.open\(\s*'(GET|POST|PUT|DELETE)'\s*,\s*'([^']+)'")
RE_FETCH = re.compile(r"fetch\(\s*'(/[^'?]+[^']*?)'")
RE_ACTION = re.compile(r"action=['\"](/[^\"]+)['\"]")

def urls_pedidas_template():
    res = []
    for tpl in DIR_TPL.rglob("*.html"):
        try:
            src = tpl.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        for m in RE_XHR.finditer(src):
            res.append((str(tpl.relative_to(RAIZ)), m.group(1), m.group(2)))
        for m in RE_FETCH.finditer(src):
            if "${" not in m.group(1) and "{{" not in m.group(1):
                res.append((str(tpl.relative_to(RAIZ)), "GET", m.group(1).split("'", 1)[0]))
        for m in RE_ACTION.finditer(src):
            ruta = m.group(1).replace("{{.AssetVer}}", "").strip()
            if ruta.startswith("/"):
                res.append((str(tpl.relative_to(RAIZ)), "POST?", ruta))
    return res


# ── C) URLs del menú en migraciones ──────────────────────────────────────
RE_MENU = re.compile(r"\(?\s*(?:\}\s*\)|)?\s*(\d+),\s*(?:\d+|NULL),\s*'[^']*',\s*'(/[^']*)'")

def urls_menu():
    urls = {}
    pat = re.compile(r"\((\d+)\s*,\s*(?:\d+|NULL)\s*,\s*'([^']*)'\s*,\s*'(/[^']*)'")
    for mig in sorted(DIR_MIG.glob("*.sql")):
        for m in pat.finditer(mig.read_text(encoding="utf-8", errors="ignore")):
            mid, label, url = m.group(1), m.group(2), m.group(3)
            if url in ("#",) or url.startswith("//"):
                continue
            urls.setdefault(normalizar(url), []).append((mig.name, mid, label))
    return urls


def coincide(rutas, buscada):
    """Coincidencia tolerante: normaliza y admite prefijos de proxy (/mod/...)."""
    b = normalizar(buscada)
    if b in rutas:
        return True
    # patrón con parámetros registrado: /x/{id}
    for r in rutas:
        if "{}" in r:
            re_pat = "^" + re.escape(r).replace(r"\{\}", "[^/]+") + "$"
            if re.match(re_pat, b):
                return True
    # prefijo capturado por un handler /x/ (Handle sin método exacto)
    prefijo = b.rsplit("/", 1)[0] + "/"
    return prefijo in rutas or prefijo in rutas.get("*", set()) or any(
        r.rstrip("/") + "/" in buscada or buscada.startswith(r + "/") for r in rutas
    )


def main():
    modo_json = "--json" in sys.argv
    rutas = rutas_registradas()
    menu = urls_menu()
    pedidas = urls_pedidas_template()

    dead_menu = []
    for url, orígenes in sorted(menu.items()):
        if not coincide(rutas, url):
            dead_menu.append({"url": url, "menu": orígenes})
    dead_templates = []
    for tpl, metodo, url in sorted(set(pedidas)):
        if "${" in url or "{{" in url or url in ("#",) or url.startswith("//"):
            continue
        b = normalizar(url.split("?")[0])
        if not coincide(rutas, b):
            dead_templates.append({"template": tpl, "metodo": metodo, "url": b})

    resumen = {
        "rutas_registradas": len(rutas),
        "urls_menu": len(menu),
        "llamadas_templates": len(set(pedidas)),
        "menu_muerto": dead_menu,
        "llamadas_mutuas": dead_templates,
    }
    if modo_json:
        print(json.dumps(resumen, indent=2, ensure_ascii=False))
        return
    print(f"Rutas registradas: {resumen['rutas_registradas']}  |  URLs de menú: {resumen['urls_menu']}  |  llamadas de templates: {resumen['llamadas_templates']}")
    print(f"\n=== 1) ENTRADAS DE MENÚ SIN RUTA (404 al abrir la ventana): {len(dead_menu)}")
    for d in dead_menu:
        for _, mid, label in d["menu"]:
            print(f"  [{mid}] {label:<40} {d['url']}")
    print(f"\n=== 2) LLAMADAS DESDE VENTANAS QUE EL SERVER NO ATENDE: {len(dead_templates)}")
    for d in dead_templates:
        print(f"  {d['template']:<40} {d['metodo']:<6} {d['url']}")
    if not dead_menu and not dead_templates:
        print("\nSIN HUÉRFANAS: todo el menú y todas las llamadas responden en el server.")


if __name__ == "__main__":
    main()
