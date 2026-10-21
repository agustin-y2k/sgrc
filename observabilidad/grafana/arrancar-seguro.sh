#!/bin/sh
# El chequeo vive también en el contenedor: docker compose directo no debe
# saltarse la validación de make observabilidad. Nunca imprimir la contraseña.
set -eu
clave=${GF_SECURITY_ADMIN_PASSWORD:-}
case "$clave" in
  ''|admin|cambiar_por_una_contrasena_larga|sin-configurar-ver-make-observabilidad)
    echo 'GRAFANA_PASSWORD no está configurada con una contraseña propia.' >&2
    exit 1
    ;;
esac
if [ "${#clave}" -lt 16 ]; then
  echo 'GRAFANA_PASSWORD debe tener al menos 16 caracteres.' >&2
  exit 1
fi
exec "$@"
