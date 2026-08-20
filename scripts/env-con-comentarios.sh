#!/bin/sh
#
# Rearma un `.env` con TODOS los comentarios de `.env.example`, conservando
# los valores que ya están configurados en esta instalación.
#
# Para qué existe: `.env.example` es donde vive la explicación de cada
# variable —qué hace, qué pasa si la dejás vacía, con qué otra tiene que ser
# coherente—. Un `.env` de servidor, en cambio, se va escribiendo a mano
# durante los despliegues y termina con los valores correctos pero sin la
# mitad de esas explicaciones, y sin las variables que se agregaron después.
# Esto junta las dos cosas: el texto del ejemplo, los valores de acá.
#
#   ./scripts/env-con-comentarios.sh                 # lo muestra por pantalla
#   ./scripts/env-con-comentarios.sh > .env.nuevo    # lo escribe para revisar
#
# NO pisa el `.env` en uso: escribe en la salida estándar a propósito. El
# reemplazo se hace a mano, después de mirar el diff:
#
#   diff .env .env.nuevo
#   cp .env .env.respaldo && mv .env.nuevo .env
#
# Y como el archivo tiene contraseñas, el resultado se guarda con permisos
# 600 si se lo redirige a un archivo con `umask 077` (ver el README de
# operación).
set -eu

EJEMPLO="${EJEMPLO:-.env.example}"
ACTUAL="${ACTUAL:-.env}"

[ -f "$EJEMPLO" ] || { echo "No encuentro $EJEMPLO. Correlo desde la raíz del proyecto." >&2; exit 1; }
[ -f "$ACTUAL" ] || { echo "No encuentro $ACTUAL." >&2; exit 1; }

# valor_actual CLAVE — la primera definición de esa clave en el .env de acá.
# Se corta en el primer '=' y no se toca el resto: hay valores con '=' adentro
# (las contraseñas generadas en base64 terminan en '=' bastante seguido).
valor_actual() {
  sed -n "s/^$1=//p" "$ACTUAL" | head -1
}

# 1. El ejemplo, línea por línea, cambiando solo los valores.
faltantes=""
while IFS= read -r linea; do
  case "$linea" in
    [A-Z]*=*)
      clave=${linea%%=*}
      if grep -q "^$clave=" "$ACTUAL"; then
        printf '%s=%s\n' "$clave" "$(valor_actual "$clave")"
      else
        # Variable que el ejemplo trae y esta instalación no tiene: va con el
        # valor del ejemplo y queda anotada para avisar al final.
        printf '%s\n' "$linea"
        faltantes="$faltantes $clave"
      fi
      ;;
    *)
      printf '%s\n' "$linea"
      ;;
  esac
done < "$EJEMPLO"

# 2. Lo que tiene esta instalación y el ejemplo no conoce. No se descarta en
#    silencio: si alguien agregó una variable a mano, perderla al "ordenar" el
#    archivo sería exactamente el accidente que este script tiene que evitar.
sobrantes=$(grep -oE '^[A-Z_]+' "$ACTUAL" | sort -u | while read -r clave; do
  grep -q "^$clave=" "$EJEMPLO" || echo "$clave"
done)

if [ -n "$sobrantes" ]; then
  printf '\n# ── Variables de esta instalación que no están en .env.example ──\n'
  printf '#\n# Se conservan tal cual. Si alguna ya no la lee nadie, se puede\n'
  printf '# borrar; comprobalo con: grep -rn NOMBRE cmd/ internal/\n'
  for clave in $sobrantes; do
    printf '%s=%s\n' "$clave" "$(valor_actual "$clave")"
  done
fi

# 3. El resumen va a stderr para no ensuciar el archivo generado.
if [ -n "$faltantes" ]; then
  printf '\nVariables NUEVAS, con el valor de ejemplo (revisalas):%s\n' "$faltantes" >&2
fi
if [ -n "$sobrantes" ]; then
  printf 'Variables que solo existen acá, conservadas al final: %s\n' "$(echo "$sobrantes" | tr '\n' ' ')" >&2
fi
