#!/bin/sh
#
# Siembra datos mínimos para poder USAR el sistema recién levantado:
# un ciclo lectivo, un curso, una materia, un docente aprobado y asignado
# a esa materia, y un carro con ocho PCs.
#
# Para qué hace falta: `cmd/main.go` siembra el primer Admin y nada más
# (ver `make seed-admin`). Sin ciclo no hay cursos, sin cursos no hay
# materias y sin materias nadie puede reservar, así que un despliegue
# limpio arranca sin nada que probar. Esto también es lo que necesitan los
# E2E de `frontend/e2e/` para no auto-skipearse.
#
# SOLO PARA DESARROLLO LOCAL. Le pega a la API como cualquier cliente
# —no toca la base por debajo—, así que todo lo que crea pasa por las
# mismas validaciones que usaría una persona. Crea un docente con una
# contraseña conocida: por eso se niega a correr contra algo que parezca
# el servidor de la escuela (ver "Guardas" más abajo).
#
#   ./scripts/sembrar-datos-de-prueba.sh
#   API=http://localhost:8080 ANIO=2027 ./scripts/sembrar-datos-de-prueba.sh
#
# Es idempotente: reutiliza lo que ya exista con el mismo nombre en vez de
# duplicarlo, así que se puede correr todas las veces que haga falta. Por
# eso el overlay de desarrollo lo corre solo en cada `make run`.
#
# Está en `sh` (no bash) a propósito: lo corre también un contenedor
# mínimo con busybox, que no trae bash.
set -eu

API="${API:-http://localhost:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@tuinstitucion.edu.ar}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-cambiar_inmediatamente}"
DOCENTE_EMAIL="${DOCENTE_EMAIL:-docente@escuela.edu.ar}"
DOCENTE_PASSWORD="${DOCENTE_PASSWORD:-docente_password_123}"
ANIO="${ANIO:-$(date +%Y)}"
CANTIDAD_PCS="${CANTIDAD_PCS:-8}"

# ── Guardas ───────────────────────────────────────────────────────────
#
# Este script crea una cuenta de docente con una contraseña que está
# escrita acá arriba. Contra el servidor de la escuela eso es una cuenta
# real con una contraseña pública, así que se corta antes de intentarlo.
case "$API" in
  http://localhost:*|http://127.0.0.1:*|http://sgrc-app:*)
    ;;
  *)
    echo "Este script es solo para desarrollo y API apunta a $API." >&2
    echo "Si de verdad querés sembrar datos de prueba ahí, exportá" >&2
    echo "SEMBRAR_IGUAL=1 y volvé a correrlo." >&2
    [ "${SEMBRAR_IGUAL:-}" = "1" ] || exit 1
    ;;
esac

TOKEN=""

api() { # api METODO RUTA [BODY]
  metodo=$1
  ruta=$2
  body=${3:-}
  if [ -n "$body" ]; then
    curl -sS -X "$metodo" "$API$ruta" \
      -H "Content-Type: application/json" \
      ${TOKEN:+-H "Authorization: Bearer $TOKEN"} -d "$body"
  else
    curl -sS -X "$metodo" "$API$ruta" \
      ${TOKEN:+-H "Authorization: Bearer $TOKEN"}
  fi
}

# El primer valor de una clave en la respuesta. Alcanza para estos DTOs y
# evita depender de jq, que no está garantizado en todas las máquinas.
campo() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

# id_por_nombre NOMBRE — de un listado {"data":[{...},{...}]}, el id del
# objeto que se llame así. Parte el JSON en un objeto por línea y busca la
# línea que tenga ese nombre; en todos estos DTOs el "id" va primero, así
# que sale de la misma línea. Es lo que hace al script idempotente: si el
# curso "1°A" ya existe, se reutiliza en vez de crear otro igual.
id_por_nombre() {
  tr '{' '\n' | grep "\"nombre\":\"$1\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4
}

# reusar_o_crear NOMBRE RUTA_LISTADO RUTA_CREAR CUERPO
reusar_o_crear() {
  nombre=$1
  ruta_listado=$2
  ruta_crear=$3
  cuerpo=$4

  existente=$(api GET "$ruta_listado" | id_por_nombre "$nombre")
  if [ -n "$existente" ]; then
    echo "$existente"
    return
  fi
  api POST "$ruta_crear" "$cuerpo" | campo id
}

echo "→ login del admin"
TOKEN=$(api POST /api/auth/login \
  "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | campo token)
if [ -z "$TOKEN" ]; then
  echo "No se pudo autenticar contra $API." >&2
  echo "¿Está levantado el backend? ¿Coinciden SEED_ADMIN_* del .env?" >&2
  exit 1
fi

echo "→ ciclo lectivo $ANIO"
CICLO=$(api POST /api/academic/ciclos "{\"anio\":$ANIO}" | campo id)
# Si ya había un ciclo activo el POST devuelve 409: se reutiliza el que exista.
[ -n "$CICLO" ] || CICLO=$(api GET /api/academic/ciclos | campo id)

echo "→ curso 1°A"
CURSO=$(reusar_o_crear "1°A" \
  "/api/academic/ciclos/$CICLO/cursos" \
  "/api/academic/ciclos/$CICLO/cursos" \
  '{"nombre":"1°A"}')

echo "→ materia Programación"
MATERIA=$(reusar_o_crear "Programación" \
  "/api/academic/cursos/$CURSO/materias" \
  "/api/academic/cursos/$CURSO/materias" \
  '{"nombre":"Programación"}')

echo "→ docente (autorregistro + aprobación)"
TOKEN_ADMIN=$TOKEN
TOKEN=""  # el autorregistro es público, va sin token
api POST /api/auth/registro \
  "{\"nombre\":\"Ada\",\"apellido\":\"Lovelace\",\"email\":\"$DOCENTE_EMAIL\",\"password\":\"$DOCENTE_PASSWORD\"}" >/dev/null || true
TOKEN=$TOKEN_ADMIN
# Por email y no "el primer DOCENTE que aparezca": una vez que hay más de
# uno cargado, el primero puede ser cualquiera.
DOCENTE=$(api GET "/api/auth/usuarios?rol=DOCENTE&pageSize=200" \
  | tr '{' '\n' | grep "\"email\":\"$DOCENTE_EMAIL\"" \
  | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
if [ -z "$DOCENTE" ]; then
  echo "No se encontró el docente $DOCENTE_EMAIL después de registrarlo." >&2
  exit 1
fi
# Aprobar es idempotente en la práctica: si ya estaba APROBADA el backend
# responde 409 y seguimos.
api PATCH "/api/auth/usuarios/$DOCENTE/estado" '{"estado":"APROBADA"}' >/dev/null || true

echo "→ asignación del docente a la materia"
# Si ya estaba asignado, el backend responde 409 y no pasa nada.
api POST "/api/academic/materias/$MATERIA/docentes" \
  "{\"usuarioId\":\"$DOCENTE\",\"rol\":\"TITULAR\"}" >/dev/null || true

echo "→ carro y $CANTIDAD_PCS PCs"
CARRO=$(reusar_o_crear "Carro 1" \
  "/api/inventory/carros" \
  "/api/inventory/carros" \
  '{"nombre":"Carro 1","descripcion":"Laboratorio de informática"}')

PCS_EXISTENTES=$(api GET "/api/inventory/carros/$CARRO/pcs" | grep -o '"identificador":' | wc -l)
i=1
while [ "$i" -le "$CANTIDAD_PCS" ]; do
  # El identificador es único dentro del carro: si la PC ya está, el
  # backend responde 409 y se sigue con la siguiente.
  api POST "/api/inventory/carros/$CARRO/pcs" \
    "{\"identificador\":$i,\"numeroSerie\":\"5CD100${i}ABC\",\"freezado\":true,\"cpu\":\"i5\",\"ram\":\"8GB\",\"sistemaOperativo\":\"Windows 10\",\"softwareInstalado\":\"AutoCAD 2027, Office\"}" >/dev/null || true
  i=$((i + 1))
done

cat <<RESUMEN

Listo.
  Admin    $ADMIN_EMAIL / $ADMIN_PASSWORD
  Docente  $DOCENTE_EMAIL / $DOCENTE_PASSWORD
  Ciclo    $ANIO · curso 1°A · materia Programación
  Carro 1  $CANTIDAD_PCS PCs disponibles (había $PCS_EXISTENTES antes de correr esto)

La semana lectiva es de lunes a viernes: al reservar, elegí un día hábil.
RESUMEN
