#!/usr/bin/env python3
"""Conversor mínimo markdown -> .docx para los documentos de plan de ERPGo.

Soporta: títulos (#..###), tablas markdown, listas con guion y numeradas,
negrita, código en línea, cursiva y reglas horizontales.
"""
import re
import sys

from docx import Document
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.shared import Pt, RGBColor

INLINE = re.compile(r"(\*\*.+?\*\*|`[^`]+`|\*[^*]+\*)")


def add_inline(par, text):
    for chunk in INLINE.split(text):
        if not chunk:
            continue
        if chunk.startswith("**") and chunk.endswith("**") and len(chunk) > 4:
            run = par.add_run(chunk[2:-2])
            run.bold = True
        elif chunk.startswith("`") and chunk.endswith("`") and len(chunk) > 2:
            run = par.add_run(chunk[1:-1])
            run.font.name = "Consolas"
            run.font.color.rgb = RGBColor(0xB0, 0x30, 0x30)
        elif chunk.startswith("*") and chunk.endswith("*") and len(chunk) > 2:
            # Dentro de la cursiva puede haber `código`: separarlo para no
            # dejar los backticks literales.
            for j, sub in enumerate(chunk[1:-1].split("`")):
                if not sub:
                    continue
                run = par.add_run(sub)
                run.italic = True
                if j % 2 == 1:
                    run.font.name = "Consolas"
                    run.font.color.rgb = RGBColor(0xB0, 0x30, 0x30)
        else:
            par.add_run(chunk)


def is_table_row(line):
    s = line.strip()
    return s.startswith("|") and s.endswith("|") and len(s) > 1


def table_cells(line):
    return [c.strip() for c in line.strip().strip("|").split("|")]


def main(src, dst):
    with open(src, encoding="utf-8") as fh:
        lines = fh.read().splitlines()

    doc = Document()
    style = doc.styles["Normal"]
    style.font.name = "Calibri"
    style.font.size = Pt(10.5)

    i = 0
    n = len(lines)
    while i < n:
        line = lines[i]
        stripped = line.strip()

        if not stripped:
            i += 1
            continue

        # Tablas
        if is_table_row(stripped):
            rows = []
            while i < n and is_table_row(lines[i].strip()):
                cells = table_cells(lines[i])
                # fila separadora |---|---|
                if not all(re.fullmatch(r":?-{2,}:?", c or "-") for c in cells):
                    rows.append(cells)
                i += 1
            if rows:
                width = max(len(r) for r in rows)
                table = doc.add_table(rows=len(rows), cols=width)
                table.style = "Table Grid"
                for ri, row in enumerate(rows):
                    for ci in range(width):
                        cell = table.cell(ri, ci)
                        cell.text = ""
                        par = cell.paragraphs[0]
                        add_inline(par, row[ci] if ci < len(row) else "")
                        for run in par.runs:
                            run.font.size = Pt(9)
                            if ri == 0:
                                run.bold = True
                doc.add_paragraph()
            continue

        # Títulos
        m = re.match(r"^(#{1,6})\s+(.*)$", stripped)
        if m:
            level = len(m.group(1))
            text = m.group(2).strip()
            if level == 1:
                par = doc.add_heading("", level=0)
                add_inline(par, text)
                par.alignment = WD_ALIGN_PARAGRAPH.CENTER
            else:
                par = doc.add_heading("", level=min(level - 1, 4))
                add_inline(par, text)
            i += 1
            continue

        # Regla horizontal
        if re.fullmatch(r"-{3,}|\*{3,}", stripped):
            par = doc.add_paragraph()
            par.add_run("_" * 78).font.color.rgb = RGBColor(0xBB, 0xBB, 0xBB)
            i += 1
            continue

        # Lista con guion
        if stripped.startswith("- "):
            par = doc.add_paragraph(style="List Bullet")
            add_inline(par, stripped[2:])
            i += 1
            continue

        # Lista numerada
        m = re.match(r"^(\d+)\.\s+(.*)$", stripped)
        if m:
            par = doc.add_paragraph(style="List Number")
            add_inline(par, m.group(2))
            i += 1
            continue

        # Cita
        if stripped.startswith("> "):
            par = doc.add_paragraph()
            par.paragraph_format.left_indent = Pt(18)
            add_inline(par, stripped[2:])
            for run in par.runs:
                run.italic = True
            i += 1
            continue

        # Párrafo normal
        par = doc.add_paragraph()
        add_inline(par, stripped)
        i += 1

    doc.save(dst)
    print(f"OK -> {dst}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
