# Cómo se generan las guías

Las dos guías en PDF (`../SGRC-guia-docentes.pdf` y
`../SGRC-guia-administradores.pdf`) no se escriben a mano: el texto vive en los
dos `.html` de la carpeta de arriba y **todas las capturas salen del sistema
corriendo en local**. Cuando una pantalla cambia, se vuelven a generar y las
guías quedan al día sin retocar imágenes a mano.

El mismo pipeline actualiza las capturas del README (`../../capturas/`).

## Lo que hace falta

- El sistema levantado en local: `make dev` (SPA en `localhost:8081`, API en
  `localhost:8080`).
- Node con las dependencias del frontend instaladas: los scripts usan el
  Playwright que ya está en `frontend/node_modules` y lo resuelven por ruta, así
  que se pueden correr desde cualquier directorio.
- Python con Pillow, para recortar y numerar las capturas.
- `jq` y `curl`, para el script de datos.

## Los pasos

```bash
export GUIA_ADMIN_PASSWORD='...'      # la de SEED_ADMIN_PASSWORD del .env
export GUIA_ADMIN_EMAIL='...'         # opcional, si tu Admin no es admin@escuela.edu.ar

# 1. Datos de demostración: docente, materias, reservas, licencias con
#    vencimiento, una entrega en curso y una conversación de soporte.
./docs/guias/generar/datos-de-demostracion.sh

# 2. Capturas. Escriben todas en $SALIDA; se corren desde donde quieras.
export SALIDA=/tmp/capturas-sgrc
node docs/guias/generar/capturar-guia.mjs         # pantallas completas
node docs/guias/generar/capturar-pasos.mjs        # diálogos y formularios en uso
node docs/guias/generar/capturar-pasos-2.mjs      # los que necesitan otro camino
node docs/guias/generar/capturar-admin.mjs        # pantallas de Admin desplegadas
node docs/guias/generar/capturar-formularios.mjs  # login, registro y recuperación
node docs/guias/generar/capturar-marcas.mjs       # las que llevan números en rojo
node docs/guias/generar/capturar-readme.mjs       # las del README (otro encuadre)

# 3. Numerar en rojo lo que la guía explica, y preparar las imágenes
#    (recorte del vacío, tope de alto, borde).
python3 docs/guias/generar/marcar.py /tmp/capturas-sgrc
python3 docs/guias/generar/preparar-imagenes.py /tmp/capturas-sgrc docs/guias/imagenes

# 4. Los PDF.
node docs/guias/generar/hacer-pdf.mjs \
  docs/guias/guia-docentes.html docs/guias/SGRC-guia-docentes.pdf \
  docs/guias/guia-admins.html   docs/guias/SGRC-guia-administradores.pdf
```

## Decisiones que conviene no deshacer

- **Las credenciales van por entorno.** Este repositorio es público y la
  instalación de cada escuela tiene las suyas. Ningún script las trae adentro.
- **Los datos de demostración son neutros a propósito** —Ana Gómez, Carro 1,
  1°A— y ninguna captura muestra correos ni nombres reales. Si vas a
  regenerarlas contra una base con datos de verdad, revisá antes qué queda a la
  vista, sobre todo en Usuarios y en Avisos.
- **`preparar-imagenes.py` corta el vacío de abajo salteando el pie de la
  página web.** Sin eso, el hueco entre lo último que importa y el
  «SGRC v1.11.0 — software libre…» viaja al PDF como media hoja en blanco.
- **Las capturas numeradas se arman solas.** `capturar-marcas.mjs` guarda las
  coordenadas de cada elemento en `marcas.json` y `marcar.py` dibuja los globos
  rojos encima. Si movés un rótulo en la interfaz, el número lo sigue: no hay
  ninguna coordenada escrita a mano.
- **El texto de las guías está en los `.html`.** El PDF se arma con el Chromium
  de Playwright, que es lo que permite tener numeración de páginas al pie.
