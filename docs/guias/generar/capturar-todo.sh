#!/usr/bin/env bash
# Capturas y PDF reales de las guías, en una instalación efímera independiente.
set -euo pipefail
cd "$(dirname "$0")/../../.."
exec node scripts/generar-guias.mjs "$@"
