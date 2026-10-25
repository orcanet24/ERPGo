import fitz  # PyMuPDF

ruta_pdf = "GacetaOficial43484.pdf"
ruta_txt = "GacetaOficial43484.txt"

# Abrimos el PDF
documento = fitz.open(ruta_pdf)
texto_completo = ""

# Extraemos texto página por página
for num_pagina, pagina in enumerate(documento):
    texto = pagina.get_text()
    texto_completo += f"===== Página {num_pagina + 1} =====\n{texto}\n\n"

# Guardamos en TXT
with open(ruta_txt, "w", encoding="utf-8") as archivo:
    archivo.write(texto_completo)

print("✅ Listo, texto guardado en:", ruta_txt)