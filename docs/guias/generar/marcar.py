import json, sys
from PIL import Image, ImageDraw, ImageFont

CAPTURAS = sys.argv[1]  # la carpeta donde escribieron los capturar-*.mjs
ESCALA = 2  # deviceScaleFactor
ROJO = (220, 38, 38)
FUENTE = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

marcas = json.load(open(f"{CAPTURAS}/marcas.json"))
for nombre, items in marcas.items():
    img = Image.open(f"{CAPTURAS}/{nombre}.png").convert("RGB")
    d = ImageDraw.Draw(img)
    r = 26  # radio del círculo
    fuente = ImageFont.truetype(FUENTE, 30)
    usados = []
    n = 0
    for it in items:
        c = it["caja"]
        if not c:
            continue
        n += 1
        x, y = c["x"] * ESCALA, c["y"] * ESCALA
        w, h = c["w"] * ESCALA, c["h"] * ESCALA
        # recuadro del elemento
        d.rounded_rectangle([x - 4, y - 4, x + w + 4, y + h + 4], radius=8, outline=ROJO, width=3)
        # círculo numerado, arriba a la izquierda del recuadro
        # el globo va AFUERA del recuadro, a la izquierda: encima tapa el texto
        cx, cy = x - r - 14, y + h / 2
        if cx < r + 2:
            cx, cy = x + w + r + 14, y + h / 2
        cx = min(max(r + 2, cx), img.width - r - 2)
        cy = min(max(r + 2, cy), img.height - r - 2)
        d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=ROJO, outline=(255, 255, 255), width=3)
        t = str(n)
        bb = d.textbbox((0, 0), t, font=fuente)
        d.text((cx - (bb[2] - bb[0]) / 2, cy - (bb[3] - bb[1]) / 2 - bb[1]), t, font=fuente, fill=(255, 255, 255))
        usados.append((x, y, x + w, y + h))
    # recorte: todo lo marcado, con aire
    if usados:
        x0 = max(0, min(u[0] for u in usados) - 110)
        y0 = max(0, min(u[1] for u in usados) - 90)
        x1 = min(img.width, max(u[2] for u in usados) + 90)
        y1 = min(img.height, max(u[3] for u in usados) + 120)
        img = img.crop((int(x0), int(y0), int(x1), int(y1)))
    img.save(f"{CAPTURAS}/{nombre}-marcada.png")
    print("  ✓", nombre, f"{n} marcas", img.size)
