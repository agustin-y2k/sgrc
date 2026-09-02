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

# 0. SOBRE UNA BASE RECIÉN CREADA, esto va primero: crea el ciclo lectivo, el
#    curso 1°A, la materia, un docente y un carro con 8 equipos. Sin esto, el
#    paso 1 muere con "jq: error ... Cannot index object with number", que no
#    dice en ningún lado que lo que falta es el ciclo.
#
#    Ojo con los nombres de las variables: este script NO lee las GUIA_* ni las
#    SEED_ADMIN_* del .env, lee ADMIN_EMAIL y ADMIN_PASSWORD.
ADMIN_EMAIL="$GUIA_ADMIN_EMAIL" ADMIN_PASSWORD="$GUIA_ADMIN_PASSWORD" \
  ./scripts/sembrar-datos-de-prueba.sh

# 1. Datos de demostración: docente, materias, reservas, licencias con
#    vencimiento, una entrega en curso y una conversación de soporte.
#    Da por sentado que el ciclo y el curso "1°A" ya existen (paso 0).
./docs/guias/generar/datos-de-demostracion.sh

# 2. Capturas. Escriben todas en $SALIDA; se corren desde donde quieras.
export SALIDA=/tmp/capturas-sgrc
node docs/guias/generar/capturar-jornada.mjs      # PRIMERO: ver abajo por qué
node docs/guias/generar/capturar-guia.mjs         # pantallas completas
node docs/guias/generar/capturar-pasos.mjs        # diálogos y formularios en uso
node docs/guias/generar/capturar-pasos-2.mjs      # los que necesitan otro camino
node docs/guias/generar/capturar-admin.mjs        # pantallas de Admin desplegadas
node docs/guias/generar/capturar-formularios.mjs  # login, registro y recuperación
node docs/guias/generar/capturar-cuentas.mjs      # las cuentas de un equipo, con las dos sesiones
node docs/guias/generar/capturar-nuevas.mjs       # cerrar el año, los calendarios y el perfil del Admin
node docs/guias/generar/capturar-marcas.mjs       # las que llevan números en rojo
node docs/guias/generar/capturar-readme.mjs       # las del README (otro encuadre)

# Las del README no pasan por preparar-imagenes.py —van sin recorte ni
# numeración— así que se copian a mano, con el mismo nombre:
#   for f in docs/capturas/*.png; do cp "$SALIDA/$(basename $f)" "$f"; done

# 3. Numerar en rojo lo que la guía explica, y preparar las imágenes
#    (recorte del vacío, tope de alto, borde).
python3 docs/guias/generar/marcar.py /tmp/capturas-sgrc
python3 docs/guias/generar/preparar-imagenes.py /tmp/capturas-sgrc docs/guias/imagenes

# 4. Los PDF.
node docs/guias/generar/hacer-pdf.mjs \
  docs/guias/guia-docentes.html docs/guias/SGRC-guia-docentes.pdf \
  docs/guias/guia-admins.html   docs/guias/SGRC-guia-administradores.pdf
```

## `capturar-jornada.mjs` va primero, y no es un detalle de orden

Los dos motivos están escritos arriba del script, pero conviene tenerlos acá:

1. **La primera captura necesita que la escuela NO tenga jornada declarada.**
   Es lo que ve un Admin al entrar a un sistema recién instalado, y cualquier
   script que corra antes y declare una la deja imposible de tomar.
2. **Al terminar, deja la jornada declarada, y los demás la necesitan.** Sin
   ningún tramo cargado el sistema le pide a cada Admin que la declare y no lo
   deja navegar: todas las capturas de Admin que vengan después mostrarían el
   asistente en vez de su pantalla.

La confirmación del impacto se toma **sin aplicarla**: se propone un horario
que deja clases afuera, se fotografía la pregunta y se sale con «Volver sin
cambiar nada», así los datos de demostración quedan intactos.

## Correr solo una parte, que es lo normal

Cada script de captura es independiente y `preparar-imagenes.py` saltea con
`✗ falta` los orígenes que no encuentra, así que **regenerar solo el que toca
la pantalla que cambió deja las demás imágenes intactas**. Dos cosas a tener
en cuenta cuando se hace así:

- **`capturar-jornada.mjs` deja la jornada declarada, y los demás la
  necesitan.** Si la base es nueva y se corre cualquier otro script primero,
  el Admin queda atrapado en el asistente de la primera jornada y todas sus
  capturas salen mostrando eso.
- **Un `✗ falta` de `preparar-imagenes.py` no siempre es intencional.** Ese
  mensaje es el mismo cuando se salteó un script a propósito —lo normal, y para
  eso está— que cuando se olvidó uno: en los dos casos la imagen vieja queda en
  `imagenes/` y viaja al PDF sin que nada la señale. Si estás regenerando
  TODO, un `✗ falta` es un script que no corriste.
- **El login falla de a ratos con `waitForURL: Timeout`** cuando se encadenan
  varias corridas seguidas. No es la contraseña: volver a correr el mismo
  script alcanza.

## Decisiones que conviene no deshacer

- **Las credenciales van por entorno.** Este repositorio es público y la
  instalación de cada escuela tiene las suyas. Ningún script las trae adentro.
- **Los datos de demostración son neutros a propósito** —Ana Gómez, Carro 1,
  1°A— y ninguna captura muestra correos ni nombres reales. Si vas a
  regenerarlas contra una base con datos de verdad, revisá antes qué queda a la
  vista, sobre todo en Usuarios y en Avisos.
- **El Admin y el docente de prueba salen de TU `.env`, y terminan en los
  PDF.** `SEED_ADMIN_EMAIL` es la cuenta que se ve en Usuarios, en Mi perfil y
  en el pie de los avisos; `DOCENTE_EMAIL` es la que siembra el overlay de
  desarrollo. Si en tu instalación local son tu correo personal, las capturas
  lo publican. Antes de regenerar, poné valores neutros
  —`admin@escuela.edu.ar`, `docente@escuela.edu.ar`—, recreá la base con
  `docker compose -f docker-compose.yml -f docker-compose.dev.yml down -v` y
  volvé a levantar; después restaurá tu `.env`. Conviene apagar también
  `SMTP_HOST` mientras dure: aprobar cuentas y cancelar reservas manda correos
  de verdad.
- **Las capturas son imágenes, así que buscar texto en el PDF no alcanza para
  auditarlas.** Un `pdftotext` no ve lo que dice una pantalla fotografiada: si
  hace falta confirmar que no quedó nada personal, hay que mirar las imágenes
  o revisar la base con `make psql` antes de capturar.
- **`preparar-imagenes.py` corta el vacío de abajo salteando el pie de la
  página web.** Sin eso, el hueco entre lo último que importa y el
  «SGRC v1.12.0 — software libre…» viaja al PDF como media hoja en blanco.
- **Las capturas numeradas se arman solas.** `capturar-marcas.mjs` guarda las
  coordenadas de cada elemento en `marcas.json` y `marcar.py` dibuja los globos
  rojos encima. Si movés un rótulo en la interfaz, el número lo sigue: no hay
  ninguna coordenada escrita a mano.
- **El texto de las guías está en los `.html`.** El PDF se arma con el Chromium
  de Playwright, que es lo que permite tener numeración de páginas al pie.
