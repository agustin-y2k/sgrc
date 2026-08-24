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
  "{\"nombre\":\"Ana\",\"apellido\":\"Gómez\",\"email\":\"$DOC_EMAIL\",\"password\":\"$DOC_PASS\",\"cargoSolicitado\":\"DOCENTE\",\"rolSolicitado\":\"TITULAR\"}" >/dev/null || true
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

# Dos equipos sueltos, que son los que hacen visible la sección "Otros
# equipos": un proyector que se puede reservar con anticipación y un cargador
# que se presta en el momento. El número de serie va en el proyector y no en
# el cargador, que es la diferencia que la guía explica (RF-03.15).
suelto() { # suelto JSON
  api "$AT" POST /api/inventory/equipos "$1" | jq -e .id >/dev/null 2>&1 || true
}
suelto '{"tipo":"Proyector","nombre":"Proyector 1","numeroSerie":"PRY-2024-118","reservable":true}'
suelto '{"tipo":"Cargador","nombre":"Cargador 1","reservable":false}'
echo "→ equipos sueltos cargados"

# Las cuentas de un equipo (RF-03.22), en los cuatro estados que la guía
# explica: la que entra sin contraseña, la pública con contraseña anotada, la
# reservada a administración y la que pide una contraseña que nadie anotó.
# Sin las cuatro, la captura no muestra ninguna de las marcas.
#
# Guardar contraseñas necesita CUENTAS_SECRET en el .env; sin ella las dos
# cuentas que la llevan salen con 503 y el aviso queda en pantalla.
# La primera computadora DE UN CARRO, no la primera de la lista: el listado
# devuelve antes los equipos sueltos, y las capturas de la guía abren el carro
# y aprietan "Cómo entrar" en la primera PC.
PC1=$(api "$AT" GET "/api/inventory/equipos?pageSize=200" | jq -r '[.data[] | select(.carroId != null)][0].id')
cuenta() { # cuenta JSON
  local r
  r=$(api "$AT" POST "/api/inventory/equipos/$PC1/cuentas" "$1")
  # Se avisa solo cuando la cuenta NO quedó: correr el script dos veces sobre
  # la misma base es normal, y "ya tiene una cuenta con ese nombre" no es una
  # falla. Lo que sí hay que ver es un 503 por CUENTAS_SECRET sin configurar.
  case "$r" in
    *'"id"'* | *"ya tiene una cuenta"*) : ;;
    *) echo "   cuenta no cargada: $r" ;;
  esac
}
cuenta '{"usuario":"alumno","clase":"Local","privilegio":"COMUN","visibilidad":"PUBLICA","tienePassword":false,"notas":"La que usan los chicos"}'
cuenta '{"usuario":"taller","clase":"Local","privilegio":"ADMINISTRADOR","visibilidad":"PUBLICA","tienePassword":true,"password":"Taller.2026","notas":"Para instalar programas en el aula"}'
cuenta '{"usuario":"soporte","clase":"Microsoft","privilegio":"ADMINISTRADOR","visibilidad":"SOLO_ADMIN","tienePassword":true,"password":"S0porte.2026"}'
cuenta '{"usuario":"profesor","clase":"Linux","privilegio":"COMUN","visibilidad":"PUBLICA","tienePassword":true,"notas":"Viene del equipo anterior"}'
echo "→ cuentas de la PC 1 cargadas"

# Una entrega en curso, para que Entregas no esté vacía.
EQ3=$(api "$AT" GET "/api/inventory/equipos?pageSize=200" | jq -c '[.data[7].id]')
api "$AT" POST /api/reservation/prestamos \
  "{\"equipoIds\":$EQ3,\"nombre\":\"Secretaría\",\"motivo\":\"inscripciones\",\"devolucionEstimada\":\"$(date -d '+2 hours' -u +%FT%TZ)\"}" >/dev/null || true
echo "→ entrega registrada"

# Una conversación de soporte con respuesta, para la captura del buzón.
SUG=$(api "$DT" POST /api/sugerencias/ \
  '{"tipo":"AYUDA","asunto":"No sé cómo cancelar una sola computadora","texto":"Reservé seis máquinas para el martes y necesito devolver dos, pero no encuentro dónde se hace. ¿Me ayudan?","pantalla":"/reservas","version":"1.12.0"}' | jq -r .id)
if [ -n "$SUG" ] && [ "$SUG" != null ]; then
  api "$AT" POST "/api/sugerencias/$SUG/mensajes" \
    '{"texto":"Hola Ana. Entrá a Reservas, abrí la reserva del martes y vas a ver cada computadora con su botón «Cancelar». Cancelá solo esas dos: las otras cuatro te quedan reservadas."}' >/dev/null || true
  echo "→ conversación de soporte $SUG"
fi

# Una cuenta PENDIENTE que declaró cargo de administración, para que la
# pantalla de aprobación no salga vacía en las capturas y para que se vea la
# ficha con "Se registró como Administrador de Sistema" (RF-01.4).
#
# Lleva rolSolicitado aunque no vaya a dar clase: titular/suplente es una
# pregunta para los dos cargos, y el registro lo rechaza sin eso.
api "" POST /api/auth/registro \
  '{"nombre":"Marcos","apellido":"Ruiz","email":"marcos.ruiz@escuela.edu.ar","password":"guia.demo.2026","cargoSolicitado":"ADMIN_SISTEMA","rolSolicitado":"TITULAR"}' >/dev/null || true
echo "→ cuenta pendiente con cargo declarado"

# Una cuenta RECHAZADA, que es el único estado donde aparece «Eliminar
# definitivamente»: sin esto, esa acción no se puede mostrar en la guía.
api "" POST /api/auth/registro \
  '{"nombre":"Cuenta","apellido":"Duplicada","email":"duplicada@escuela.edu.ar","password":"guia.demo.2026","cargoSolicitado":"DOCENTE","rolSolicitado":"TITULAR"}' >/dev/null || true
DUP=$(api "$AT" GET "/api/auth/usuarios?rol=DOCENTE&pageSize=200" | jq -r '.data[] | select(.email=="duplicada@escuela.edu.ar") | .id')
if [ -n "$DUP" ] && [ "$DUP" != null ]; then
  api "$AT" PATCH "/api/auth/usuarios/$DUP/estado" '{"estado":"RECHAZADA"}' >/dev/null || true
  echo "→ cuenta rechazada (para la captura de eliminar definitivamente)"
fi

echo
echo "Listo. Para las capturas:"
echo "  Admin    $ADMIN_EMAIL / $ADMIN_PASSWORD"
echo "  Docente  $DOC_EMAIL / $DOC_PASS"
