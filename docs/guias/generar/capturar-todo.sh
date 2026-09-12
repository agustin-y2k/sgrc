#!/usr/bin/env bash
#
# Regenera TODAS las capturas y los dos PDF, de punta a punta y sin preguntar
# nada. Es lo que corre `make capturas` y lo que corre el job de CI.
#
# Existe porque el pipeline siempre estuvo automatizado y aun así las capturas
# envejecían: eran doce comandos en un README, en un orden que importaba, con
# una pila de Docker aparte que había que levantar y bajar a mano. Nadie corre
# eso "por las dudas" después de mover un botón. Un solo comando sí.
#
#   ./docs/guias/generar/capturar-todo.sh
#
# Levanta su propia pila en el proyecto `sgrc-capturas` —base recién creada,
# datos neutros— y la baja al terminar, pase lo que pase. La base de desarrollo
# no se toca: es otro proyecto de Compose y por lo tanto otro volumen.
set -euo pipefail

cd "$(dirname "$0")/../../.."
RAIZ="$PWD"

# --solo-pdf rehace únicamente los dos documentos, sin levantar nada. Es el
# caso de todos los días: cambió el texto de una guía o subió la versión (la
# portada es parte del documento), y las capturas no tienen por qué cambiar.
SOLO_PDF=no
[ "${1:-}" = "--solo-pdf" ] && SOLO_PDF=si

PROYECTO=sgrc-capturas
ENV_CAPTURAS="${ENV_CAPTURAS:-.env.capturas}"
SALIDA="${SALIDA:-/tmp/capturas-sgrc}"
BASE_URL=http://localhost:8081

compose() {
  docker compose --env-file "$ENV_CAPTURAS" -p "$PROYECTO" \
    -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.capturas.yml "$@"
}

# ── El .env de las capturas ──────────────────────────────────────────────
#
# No va al repositorio (.gitignore cubre .env.*) porque es un archivo de
# configuración local, pero el ejemplo sí: sin él, este script no se puede
# correr en una máquina limpia ni en CI, que es justamente lo que se busca.
if [ "$SOLO_PDF" = no ] && [ ! -f "$ENV_CAPTURAS" ]; then
  echo "No existe $ENV_CAPTURAS. Copialo del ejemplo y revisalo:"
  echo "  cp .env.capturas.example $ENV_CAPTURAS"
  exit 1
fi

# Se LEE, no se ejecuta. Un `.env` admite valores con espacios sin comillas
# —`SMTP_FROM_NAME=Escuela Técnica N°1` es válido para Docker Compose— y
# hacerle `source` a eso intenta correr «Técnica» como un comando. Acá sólo
# hacen falta cuatro valores, así que se sacan con sed.
leer_env() {
  sed -nE "s/^[[:space:]]*$1=//p" "$ENV_CAPTURAS" | tail -1 | sed -E 's/^"(.*)"$/\1/; s/^'"'"'(.*)'"'"'$/\1/'
}

if [ "$SOLO_PDF" = no ]; then
  SEED_ADMIN_EMAIL=$(leer_env SEED_ADMIN_EMAIL)
  SEED_ADMIN_PASSWORD=$(leer_env SEED_ADMIN_PASSWORD)
  DOCENTE_EMAIL=$(leer_env DOCENTE_EMAIL)
fi

# ── Cómo se corre Node ───────────────────────────────────────────────────
#
# En esta máquina no hay Node instalado, así que los scripts corren dentro de
# la imagen oficial de Playwright, que ya trae el navegador y sus dependencias
# de sistema. La versión tiene que ser la MISMA que la de @playwright/test en
# frontend/package.json, o el navegador no coincide con el que resuelve el
# `createRequire` de cada script.
VERSION_PW=$(sed -nE 's/.*"@playwright\/test": "\^?([0-9.]+)".*/\1/p' frontend/package.json)
if command -v node >/dev/null 2>&1; then
  nodo() { node "$@"; }
else
  echo "Sin Node local: se usa mcr.microsoft.com/playwright:v${VERSION_PW}-noble"
  nodo() {
    docker run --rm --network host --user "$(id -u):$(id -g)" \
      -v "$RAIZ":/repo -e HOME=/tmp -e SALIDA=/salida -v "$SALIDA":/salida \
      -e GUIA_ADMIN_EMAIL -e GUIA_ADMIN_PASSWORD -e GUIA_DOCENTE_EMAIL -e GUIA_DOCENTE_PASSWORD \
      -w /repo "mcr.microsoft.com/playwright:v${VERSION_PW}-noble" node "$@"
  }
fi

if [ "$SOLO_PDF" = no ]; then

# ── El control que existe porque ya falló dos veces ──────────────────────
#
# La identidad del Admin sale de SEED_ADMIN_EMAIL y la del docente de
# DOCENTE_EMAIL, y las dos se VEN en las capturas de Usuarios y Mi perfil. Con
# el .env de desarrollo copiado sin mirar, eso publica el correo personal de
# quien generó las guías en un repositorio público — y no hay forma de
# detectarlo después con `pdftotext`, porque las pantallas son imágenes.
#
# Por eso se frena acá y no se avisa: un aviso en medio de un log de diez
# minutos es un aviso que nadie lee.
for var in SEED_ADMIN_EMAIL DOCENTE_EMAIL; do
  valor="${!var:-}"
  case "$valor" in
    "")
      echo "Falta $var en $ENV_CAPTURAS: es la cuenta que se ve en las capturas."
      exit 1
      ;;
    *@gmail.com | *@hotmail.com | *@outlook.com | *@yahoo.com)
      echo "$var es una casilla personal ($valor) y aparece en las capturas de"
      echo "Usuarios y Mi perfil, que van a un repositorio público."
      echo "Poné algo neutro en $ENV_CAPTURAS (admin@escuela.edu.ar)."
      exit 1
      ;;
  esac
done

# Los dos correos tienen que ser distintos: Ada Lovelace la siembra el overlay
# de desarrollo y pide una materia; Ana Gómez la crea datos-de-demostracion.sh
# y reserva. Con el mismo correo, la segunda reusa la cuenta de la primera y el
# paso del pedido de materia muere con un `jq: parse error` que no dice nada.
if [ "$SEED_ADMIN_EMAIL" = "$DOCENTE_EMAIL" ]; then
  echo "SEED_ADMIN_EMAIL y DOCENTE_EMAIL no pueden ser el mismo correo."
  exit 1
fi

export GUIA_ADMIN_EMAIL="$SEED_ADMIN_EMAIL"
export GUIA_ADMIN_PASSWORD="$SEED_ADMIN_PASSWORD"

mkdir -p "$SALIDA"
export SALIDA

# ── Los puertos ──────────────────────────────────────────────────────────
#
# La pila de capturas usa los mismos 8080/8081/5432 que la de desarrollo —los
# scripts tienen localhost:8081 escrito adentro— así que las dos no conviven.
# Sin este control, `compose up` muere con un "bind: address already in use"
# que no dice cuál de las dos pilas sobra ni que parar una es seguro.
#
# `make stop` para la de desarrollo sin borrar su volumen: el volcado del
# servidor sobrevive.
ocupados=""
for puerto in 8080 8081 5432; do
  if (exec 3<>"/dev/tcp/127.0.0.1/$puerto") 2>/dev/null; then
    ocupados="$ocupados $puerto"
    exec 3<&- 3>&-
  fi
done
if [ -n "$ocupados" ]; then
  echo "Los puertos$ocupados ya están ocupados, seguramente por la pila de"
  echo "desarrollo. La de capturas usa los mismos y no pueden convivir."
  echo
  echo "  make stop      # para la de desarrollo, sin tocar su base"
  echo "  make capturas"
  echo "  make dev       # y la volvés a levantar después"
  exit 1
fi

# ── La pila, y su desmantelamiento pase lo que pase ──────────────────────
#
# El `-v` del final borra el volumen de ESTE proyecto (sgrc-capturas_pgdata) y
# no el de desarrollo. Va en un trap porque una corrida que aborta a la mitad y
# deja la pila arriba se lleva puesta la siguiente: los puertos 8080/8081/5432
# son los mismos que los de desarrollo y no pueden convivir.
limpiar() {
  echo "── Bajando la pila de capturas"
  compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap limpiar EXIT

echo "── Levantando la pila de capturas (proyecto $PROYECTO)"
compose up -d --build postgres sgrc-app frontend

echo "── Esperando a que responda $BASE_URL"
for _ in $(seq 1 60); do
  if curl -sf "$BASE_URL" >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -sf "$BASE_URL" >/dev/null

# ── Los datos ────────────────────────────────────────────────────────────
#
# Primero el ciclo lectivo, el curso y el carro; después el resto. Al revés, el
# segundo script muere con "jq: error ... Cannot index object with number", que
# en ningún lado dice que lo que falta es el ciclo.
echo "── Sembrando datos base"
ADMIN_EMAIL="$GUIA_ADMIN_EMAIL" ADMIN_PASSWORD="$GUIA_ADMIN_PASSWORD" \
  ./scripts/sembrar-datos-de-prueba.sh

echo "── Datos de demostración"
./docs/guias/generar/datos-de-demostracion.sh

# ── Las capturas ─────────────────────────────────────────────────────────
#
# El orden importa en un solo lugar y es el primero: capturar-jornada.mjs
# necesita que la escuela NO tenga jornada declarada —es lo que ve un Admin al
# entrar a un sistema recién instalado— y al terminar la deja declarada, que es
# lo que los demás necesitan para poder navegar.
#
# Cada script se reintenta UNA vez: el login falla de a ratos con
# `waitForURL: Timeout` cuando se encadenan corridas, y no es la contraseña
# —el mismo login por curl da 200—. Repetirlo alcanza. Dos fallos seguidos sí
# son un problema de verdad y cortan todo.
CAPTURAS=(
  capturar-jornada.mjs      # PRIMERO, ver arriba
  capturar-guia.mjs         # pantallas completas
  capturar-pasos.mjs        # diálogos y formularios en uso
  capturar-pasos-2.mjs      # los que necesitan otro camino
  capturar-admin.mjs        # pantallas de Admin desplegadas
  capturar-formularios.mjs  # login, registro y recuperación
  capturar-cuentas.mjs      # las cuentas de un equipo, con las dos sesiones
  capturar-nuevas.mjs       # cerrar el año, los calendarios y el perfil
  capturar-marcas.mjs       # las que llevan números en rojo
  capturar-readme.mjs       # las del README, con otro encuadre
)

for script in "${CAPTURAS[@]}"; do
  echo "── $script"
  if ! nodo "docs/guias/generar/$script"; then
    echo "   falló; se reintenta una vez"
    nodo "docs/guias/generar/$script"
  fi
done

# ── Numerar, recortar y publicar ─────────────────────────────────────────
echo "── Globos numerados"
python3 docs/guias/generar/marcar.py "$SALIDA"

echo "── Recorte y publicación"
# --estricto: si falta cualquier origen, esto corta. Es la diferencia entre una
# corrida completa y una a la que se le cayó media pantalla sin avisar.
python3 docs/guias/generar/preparar-imagenes.py "$SALIDA" docs/guias/imagenes --estricto

# Las del README van sin recorte ni numeración: son otro encuadre y otra
# función —la vitrina del repositorio, no instrucciones—, así que se copian
# tal como salieron.
echo "── Capturas del README"
for f in docs/capturas/*.png; do
  origen="$SALIDA/$(basename "$f")"
  if [ ! -f "$origen" ]; then
    echo "   falta $(basename "$f") — capturar-readme.mjs no la generó"
    exit 1
  fi
  cp "$origen" "$f"
done

fi  # fin de todo lo que --solo-pdf se saltea

# ── Los PDF ──────────────────────────────────────────────────────────────
#
# De a uno. Con los dos pares en una sola invocación, el de Admins salió
# truncado a la mitad (23 páginas de 47) y SIN error, así que el conteo de
# páginas de abajo es parte del pipeline y no una comprobación de más.
echo "── PDF de docentes"
nodo docs/guias/generar/hacer-pdf.mjs \
  docs/guias/guia-docentes.html docs/guias/SGRC-guia-docentes.pdf

echo "── PDF de administradores"
nodo docs/guias/generar/hacer-pdf.mjs \
  docs/guias/guia-admins.html docs/guias/SGRC-guia-administradores.pdf

if command -v pdftotext >/dev/null 2>&1; then
  for pdf in docs/guias/SGRC-guia-docentes.pdf docs/guias/SGRC-guia-administradores.pdf; do
    paginas=$(pdftotext "$pdf" - | grep -c $'\f' || true)
    echo "   $(basename "$pdf"): $paginas páginas"
    # Un PDF truncado es el modo de falla conocido, y las dos guías pasan
    # holgadamente de 30 páginas desde la 1.12.
    if [ "$paginas" -lt 30 ]; then
      echo "   demasiado corto: el PDF salió truncado"
      exit 1
    fi
  done
else
  echo "   (sin pdftotext: no se pudo verificar el largo de los PDF)"
fi

echo
echo "Listo. Lo que cambió de verdad lo dice git:"
echo "  git status --short docs/guias/imagenes docs/capturas docs/guias/*.pdf"
