#!/bin/bash
# Datos de demostración para las capturas de las guías. Solo local.
set -eu
API=http://localhost:8080
ADMIN_EMAIL="${GUIA_ADMIN_EMAIL:-admin@escuela.edu.ar}"
ADMIN_PASSWORD="${GUIA_ADMIN_PASSWORD:?exportá GUIA_ADMIN_PASSWORD con la contraseña del Admin del .env}"
DOC_EMAIL="${GUIA_DOCENTE_EMAIL:-ana.gomez@escuela.edu.ar}"
DOC_PASS="${GUIA_DOCENTE_PASSWORD:-guia.demo.2026}"

api() { # api TOKEN METODO RUTA [BODY]
  local tk=$1 m=$2 r=$3 b=${4:-}
  if [ -n "$b" ]; then
    curl -sS -X "$m" "$API$r" -H "Content-Type: application/json" ${tk:+-H "Authorization: Bearer $tk"} -d "$b"
  else
    curl -sS -X "$m" "$API$r" ${tk:+-H "Authorization: Bearer $tk"}
  fi
}

AT=$(api "" POST /api/auth/login "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .token)
[ "$AT" != null ] || { echo "login admin falló"; exit 1; }
echo "→ admin ok"

CICLO=$(api "$AT" GET /api/academic/ciclos | jq -r '.data[0].id // .[0].id')
CURSO=$(api "$AT" GET "/api/academic/ciclos/$CICLO/cursos" | jq -r '.data[]? // .[]? | select(.nombre=="1°A") | .id' | head -1)
MAT_PROG=$(api "$AT" GET "/api/academic/cursos/$CURSO/materias" | jq -r '.data[]? // .[]? | select(.nombre=="Programación") | .id' | head -1)
echo "→ ciclo $CICLO curso $CURSO materia $MAT_PROG"

# Segunda materia, para que el selector de la reserva tenga más de una opción.
MAT_MAT=$(api "$AT" GET "/api/academic/cursos/$CURSO/materias" | jq -r '.data[]? // .[]? | select(.nombre=="Matemática") | .id' | head -1)
if [ -z "$MAT_MAT" ]; then
  MAT_MAT=$(api "$AT" POST "/api/academic/cursos/$CURSO/materias" '{"nombre":"Matemática"}' | jq -r .id)
fi
echo "→ materia Matemática $MAT_MAT"

# Docente de la guía.
api "" POST /api/auth/registro \
  "{\"nombre\":\"Ana\",\"apellido\":\"Gómez\",\"email\":\"$DOC_EMAIL\",\"password\":\"$DOC_PASS\"}" >/dev/null || true
DOC=$(api "$AT" GET "/api/auth/usuarios?rol=DOCENTE&pageSize=200" | jq -r ".data[] | select(.email==\"$DOC_EMAIL\") | .id")
api "$AT" PATCH "/api/auth/usuarios/$DOC/estado" '{"estado":"APROBADA"}' >/dev/null || true
api "$AT" POST "/api/academic/materias/$MAT_PROG/docentes" "{\"usuarioId\":\"$DOC\",\"rol\":\"TITULAR\"}" >/dev/null || true
api "$AT" POST "/api/academic/materias/$MAT_MAT/docentes" "{\"usuarioId\":\"$DOC\",\"rol\":\"TITULAR\"}" >/dev/null || true
echo "→ docente Ana Gómez $DOC"

DT=$(api "" POST /api/auth/login "{\"email\":\"$DOC_EMAIL\",\"password\":\"$DOC_PASS\"}" | jq -r .token)
echo "→ docente logueado"

# Dos reservas próximas, en días hábiles.
reservar() { # reservar FECHA HORA_INI HORA_FIN MATERIA CANT
  local f=$1 hi=$2 hf=$3 mat=$4 cant=$5
  local ids
  ids=$(api "$DT" GET "/api/reservation/equipos-disponibles?fecha=$f&horaInicio=$hi&horaFin=$hf&materiaId=$mat" \
        | jq -c "[.data[0:$cant][].equipoId]")
  [ "$ids" != "[]" ] || { echo "   sin equipos libres el $f"; return; }
  api "$DT" POST /api/reservation/reservas \
    "{\"equipoIds\":$ids,\"materiaId\":\"$mat\",\"fecha\":\"$f\",\"horaInicio\":\"$hi\",\"horaFin\":\"$hf\"}" \
    | jq -r 'if .id then "   reserva \(.fecha) \(.horaInicio) ok" else "   " + (.mensaje // .error // tostring) end'
}

D1=$(date -d "next monday" +%F)
D2=$(date -d "next wednesday" +%F)
reservar "$D1" 10:00 11:30 "$MAT_PROG" 6
reservar "$D2" 08:00 09:30 "$MAT_MAT" 3

# Licencias: una tranquila y otra por vencer, para que se vea el semáforo.
EQ=$(api "$AT" GET "/api/inventory/equipos?pageSize=200" | jq -c '[.data[0:4][].id]')
EQ2=$(api "$AT" GET "/api/inventory/equipos?pageSize=200" | jq -c '[.data[4:6][].id]')
api "$AT" POST /api/inventory/licencias "{\"equipoIds\":$EQ,\"nombre\":\"AutoCAD 2027\",\"diasDuracion\":180,\"diasAviso\":15}" >/dev/null || true
api "$AT" POST /api/inventory/licencias "{\"equipoIds\":$EQ2,\"nombre\":\"Office 365\",\"diasDuracion\":4,\"diasAviso\":7}" >/dev/null || true
echo "→ licencias cargadas"

# Una entrega en curso, para que Entregas no esté vacía.
EQ3=$(api "$AT" GET "/api/inventory/equipos?pageSize=200" | jq -c '[.data[7].id]')
api "$AT" POST /api/reservation/prestamos \
  "{\"equipoIds\":$EQ3,\"nombre\":\"Secretaría\",\"motivo\":\"inscripciones\",\"devolucionEstimada\":\"$(date -d '+2 hours' -u +%FT%TZ)\"}" >/dev/null || true
echo "→ entrega registrada"

# Una conversación de soporte con respuesta, para la captura del buzón.
SUG=$(api "$DT" POST /api/sugerencias/ \
  '{"tipo":"AYUDA","asunto":"No sé cómo cancelar una sola computadora","texto":"Reservé seis máquinas para el martes y necesito devolver dos, pero no encuentro dónde se hace. ¿Me ayudan?","pantalla":"/reservas","version":"1.11.0"}' | jq -r .id)
if [ -n "$SUG" ] && [ "$SUG" != null ]; then
  api "$AT" POST "/api/sugerencias/$SUG/mensajes" \
    '{"texto":"Hola Ana. Entrá a Reservas, abrí la reserva del martes y vas a ver cada computadora con su botón «Cancelar». Cancelá solo esas dos: las otras cuatro te quedan reservadas."}' >/dev/null || true
  echo "→ conversación de soporte $SUG"
fi

echo
echo "Listo. Para las capturas:"
echo "  Admin    $ADMIN_EMAIL / $ADMIN_PASSWORD"
echo "  Docente  $DOC_EMAIL / $DOC_PASS"
