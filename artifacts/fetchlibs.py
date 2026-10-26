#!/usr/bin/env python3
"""Descarga .deb de archive.ubuntu.com y extrae binarios, sinsudo."""
import re, sys, urllib.request, subprocess, os

BASE = "https://archive.ubuntu.com/ubuntu/pool/main/"
DST = os.path.expanduser("~/chromlibs")

def pool_ok(pool):
    try:
        with urllib.request.urlopen(BASE + pool + "/") as r:
            return r.read().decode()
    except Exception as e:
        return ""

def fetch(pool, prefix):
    html = pool_ok(pool)
    if not html:
        print(f"MISS pool {pool}")
        return
    urls = sorted(set(re.findall(prefix + r'[^"\'>]*_amd64\.deb', html)), key=lambda s: [int(x) if x.isdigit() else 0 for x in re.split(r'[._+-]', s)])
    # orden natural: tomar el último
    url = re.findall(prefix + r'[^"\'>]*_amd64\.deb', html)
    cand = sorted(set(url))[-1]
    print("GET", cand)
    urllib.request.urlretrieve(BASE + pool + "/" + cand, os.path.join(DST, cand))
    subprocess.run(["dpkg", "-x", os.path.join(DST, cand), DST], check=True)

for pool, prefix in [
    ("a/at-spi2-core/", "libatspi2.0-0t64_"),
    ("g/glib2.0/", "libglib2.0-0t64_"),
    ("d/dbus/", "libdbus-1-3_"),
    ("e/expat/", "libexpat1_"),
    ("p/pango1.0/", "libpango-1.0-0_"),
    ("p/pango1.0/", "libpangocairo-1.0-0_"),
    ("p/pango1.0/", "libpangoft2-1.0-0_"),
    ("c/cairo2/", "libcairo2_"),
    ("f/freetype/", "libfreetype6_"),
    ("f/fontconfig/", "libfontconfig1_"),
    ("h/harfbuzz/", "libharfbuzz0b_"),
    ("libt/libthai/", "libthai0t64_"),
    ("g/graphite2/", "libgraphite2-3_"),
    ("libp/libpng1.6/", "libpng16-16t64_"),
    ("p/pcre2/", "libpcre2-8-0t64_"),
    ("b/brotli/", "libbrotli1_"),
    ("libw/libwayland/", "libwayland-client0_"),
]:
    try:
        fetch(pool.rstrip('/'), prefix)
    except Exception as e:
        print("ERR", pool, e)
