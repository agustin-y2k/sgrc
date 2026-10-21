#!/usr/bin/env python3
"""Prepara configuración local privada sin reemplazar archivos existentes."""

import os
import secrets
from pathlib import Path


SECRETOS = {
    "POSTGRES_PASSWORD", "JWT_SECRET", "OUTBOX_SECRET", "CUENTAS_SECRET",
    "SEED_ADMIN_PASSWORD", "ADMIN_AUX_PASSWORD", "DOCENTE_PASSWORD",
    "GRAFANA_PASSWORD",
}


def crear_si_falta(destino, contenido):
    try:
        fd = os.open(destino, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        if not destino.is_file():
            raise RuntimeError(f"{destino.name} existe pero no es un archivo")
        print(f"Se conserva {destino.name}")
        return
    with os.fdopen(fd, "w") as archivo:
        archivo.write(contenido)
    print(f"Creado {destino.name} (privado, permisos 0600)")


def main():
    raiz = Path(__file__).resolve().parents[1]
    lineas = (raiz / ".env.local.example").read_text().splitlines()
    encontradas = set()
    for i, linea in enumerate(lineas):
        clave, separador, valor = linea.partition("=")
        if separador and clave in SECRETOS:
            if valor != "cambiar":
                raise RuntimeError(f"La plantilla debe conservar el marcador de {clave}")
            encontradas.add(clave)
            lineas[i] = f"{clave}=LocalA1!{secrets.token_urlsafe(32)}"
    if encontradas != SECRETOS:
        raise RuntimeError("La plantilla local no contiene todos los secretos esperados")
    # Las plantillas deben estar completas antes de crear cualquier archivo.
    compose = (raiz / "docker-compose.local.example.yml").read_text()
    inicio = next(i for i, linea in enumerate(lineas) if linea and not linea.startswith("#"))
    configuracion = (
        "# CONFIGURACIÓN PRIVADA LOCAL. Contiene claves generadas: no publicar.\n"
        "# make local-init conserva este archivo si ya existe.\n\n"
        + "\n".join(lineas[inicio:]) + "\n"
    )
    inicio_compose = next(i for i, linea in enumerate(compose.splitlines())
                          if linea and not linea.startswith("#"))
    compose_privado = (
        "# CONFIGURACIÓN PRIVADA LOCAL. No publicar ajustes de esta instalación.\n"
        "# Los cambios reutilizables se incorporan a docker-compose.local.example.yml.\n"
        + "\n".join(compose.splitlines()[inicio_compose:]) + "\n"
    )
    crear_si_falta(raiz / ".env.local", configuracion)
    crear_si_falta(raiz / "docker-compose.local.yml", compose_privado)
    print("Configuración lista. Las contraseñas se consultan en .env.local.")


if __name__ == "__main__":
    main()
