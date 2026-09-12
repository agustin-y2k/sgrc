"""¿Alguna captura commiteada dejó de mostrar lo que muestra el sistema?

Compara lo que el pipeline acaba de generar contra lo que está en git y falla si
alguna cambió DE VERDAD. Lo usa el job de CI; a mano no hace falta, porque
`git status` ya dice qué se movió.

    python3 docs/guias/generar/comparar-capturas.py [--umbral 2.0]

**Por qué un umbral y no igualdad exacta.** Dos corridas del pipeline sobre el
mismo código no dan imágenes idénticas: las pantallas muestran fechas y horas
—los datos se siembran con `now()`— y la de inicio saluda según la hora. Esa
diferencia existe siempre y no dice nada sobre la interfaz.

El primer intento fue una lista de pantallas a ignorar. No funciona: habría que
ir agregándole una pantalla por cada corrida en rojo, y cada agregado apaga el
control justo donde acaba de fallar. El umbral sale de medir, no de estimar.
Comparando una corrida local contra una de CI, con el mismo commit:

    3 imágenes  > 5%      el botón de «Acceder con Google», que aparece o no
                          según la configuración — una diferencia real
    26 imágenes 0,01-0,6% el reloj y el saludo
    31 imágenes 0%        idénticas

Entre 0,6% y 5% no hay nada, y no es casualidad: mover un rótulo, agregar un
botón o cambiar una sección repinta bastante más que una línea de texto. El
umbral vive en ese hueco.

Lo que esto NO atrapa es un cambio de una palabra en una pantalla larga. Para
eso está el otro control, que corre siempre y cubre las sesenta: el pipeline
falla —`preparar-imagenes.py --estricto`— si alguna captura no se generó, que es
como estas cosas se rompían de verdad (un selector que dejó de encontrar su
botón, en silencio, durante semanas).
"""

import subprocess
import sys
from io import BytesIO

from PIL import Image, ImageChops

UMBRAL_POR_DEFECTO = 2.0
CARPETAS = ["docs/guias/imagenes", "docs/capturas"]


def umbral_pedido(argv):
    if "--umbral" in argv:
        return float(argv[argv.index("--umbral") + 1])
    return UMBRAL_POR_DEFECTO


def version_en_git(ruta):
    """El PNG tal como está commiteado, o None si es un archivo nuevo."""
    r = subprocess.run(["git", "show", f"HEAD:{ruta}"], capture_output=True)
    return r.stdout if r.returncode == 0 else None


def porcentaje_distinto(antes, ahora):
    a = Image.open(BytesIO(antes)).convert("RGB")
    b = Image.open(ahora).convert("RGB")
    if a.size != b.size:
        return 100.0
    dif = ImageChops.difference(a, b)
    distintos = sum(1 for p in dif.getdata() if p != (0, 0, 0))
    return 100 * distintos / (a.size[0] * a.size[1])


def main():
    umbral = umbral_pedido(sys.argv)
    cambiadas = subprocess.run(
        ["git", "status", "--porcelain", "--"] + CARPETAS,
        capture_output=True, text=True, check=True,
    ).stdout.split("\n")

    reales, ruido = [], []
    for linea in cambiadas:
        if not linea.strip():
            continue
        ruta = linea.split()[-1]
        antes = version_en_git(ruta)
        if antes is None:
            # Una captura nueva no tiene contra qué compararse. No es un
            # problema: es una pantalla que antes no estaba documentada.
            reales.append((ruta, None))
            continue
        pct = porcentaje_distinto(antes, ruta)
        (reales if pct > umbral else ruido).append((ruta, pct))

    if ruido:
        print(f"Cambios por el reloj y el saludo (por debajo de {umbral}%):")
        for ruta, pct in sorted(ruido, key=lambda x: -x[1]):
            print(f"  {pct:5.2f}%  {ruta}")
        print()

    if reales:
        print("Estas capturas ya no muestran lo que muestra el sistema:")
        for ruta, pct in sorted(reales, key=lambda x: -(x[1] or 100)):
            cuanto = "nueva" if pct is None else f"{pct:.2f}%"
            print(f"  {cuanto:>7}  {ruta}")
        print()
        print("Regeneralas y commitealas:")
        print("  make capturas")
        return 1

    print(f"Las capturas están al día (nada por encima de {umbral}%).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
