# Cómo se generan las guías

Las dos guías en PDF (`../SGRC-guia-docentes.pdf` y
`../SGRC-guia-administradores.pdf`) no se escriben a mano: el texto vive en los
dos `.html` de la carpeta de arriba y el PDF sale de ahí.

```bash
node docs/guias/generar/hacer-pdf.mjs \
  docs/guias/guia-docentes.html docs/guias/SGRC-guia-docentes.pdf \
  docs/guias/guia-admins.html   docs/guias/SGRC-guia-administradores.pdf
```

Hace falta Node con las dependencias del frontend instaladas: el script usa el
Playwright que ya está en `frontend/node_modules` y lo resuelve por ruta, así que
corre desde cualquier directorio. Sin Node en la máquina, anda igual dentro de la
imagen oficial de Playwright, que ya trae el navegador:

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD":/repo -e HOME=/tmp -w /repo \
  mcr.microsoft.com/playwright:v1.56.0-noble \
  node docs/guias/generar/hacer-pdf.mjs \
    docs/guias/guia-docentes.html docs/guias/SGRC-guia-docentes.pdf \
    docs/guias/guia-admins.html   docs/guias/SGRC-guia-administradores.pdf
```

## Las guías no llevan capturas

Hasta la 1.21.0 llevaban 45, generadas por un pipeline de scripts que levantaba
el sistema en local, cargaba datos de demostración, sacaba las pantallas, las
recortaba y las numeraba. **Se quitaron, y con ellas todo ese pipeline.**

El motivo es el costo de mantenerlas. Una captura queda desactualizada con
cualquier cambio de la pantalla que muestra, y una guía con capturas viejas es
peor que una sin ninguna: el docente busca un botón que ya no se llama así y
concluye que la guía —o el sistema— está mal. Mantenerlas al día obligaba a
levantar el sistema, sembrar datos y regenerar imágenes en cada cambio de
interfaz, y eso no pasaba.

El texto no dependía de ellas: ninguna instrucción decía «como se ve en la
imagen», así que los pasos se leen igual sin ellas. Lo único que se perdió es el
pie de cada figura, que describía la imagen y no agregaba nada.

Las capturas del README (`../../capturas/`) son otra cosa y siguen en pie: son la
vitrina del repositorio, no instrucciones.
