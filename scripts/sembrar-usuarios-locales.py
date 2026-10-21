#!/usr/bin/env python3
"""Crea cuentas ficticias y asignaciones mediante la API de la copia local."""

import json
import os
import re
import sys
import unicodedata
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen


def leer_configuracion(path):
    values = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        values[key] = value
    return values


def normalizar(value):
    return "".join(
        char for char in unicodedata.normalize("NFD", value.lower())
        if not unicodedata.combining(char)
    )


def main():
    repo = Path(__file__).resolve().parents[1]
    cfg = leer_configuracion(repo / ".env.local")
    base = os.environ.get("API_LOCAL", "http://localhost:8081").rstrip("/")
    if urlsplit(base).hostname not in {"localhost", "127.0.0.1"}:
        raise RuntimeError("La siembra sólo admite localhost o 127.0.0.1")
    if not cfg["SEED_ADMIN_EMAIL"].endswith("@local.test"):
        raise RuntimeError("La configuración debe usar cuentas ficticias @local.test")

    def api(method, path, data=None, token=None, aceptar=()):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = Request(base + path, method=method, headers=headers,
                          data=None if data is None else json.dumps(data).encode())
        try:
            with urlopen(request, timeout=30) as response:
                body = response.read()
                return json.loads(body) if body and "application/json" in response.headers.get("Content-Type", "") else None
        except HTTPError as error:
            if error.code in aceptar:
                return None
            raise RuntimeError(f"{method} {path}: HTTP {error.code}") from None

    session = api("POST", "/api/auth/login", {
        "email": cfg["SEED_ADMIN_EMAIL"], "password": cfg["SEED_ADMIN_PASSWORD"],
    })
    token = session["token"]
    if session["debeCambiarPassword"]:
        raise RuntimeError("El administrador local debe actualizar su contraseña primero")
    usuarios = api("GET", "/api/auth/usuarios?pageSize=200", token=token)["data"]
    por_email = {user["email"]: user for user in usuarios}
    if cfg["ADMIN_AUX_EMAIL"] not in por_email:
        api("POST", "/api/auth/admins", {
            "nombre": "Administrador", "apellido": "Prueba",
            "email": cfg["ADMIN_AUX_EMAIL"], "password": cfg["ADMIN_AUX_PASSWORD"],
        }, token)

    ciclos = api("GET", "/api/ciclos", token=token)["data"]
    activo = next((c for c in ciclos if c["activo"]), None)
    if activo is None:
        raise RuntimeError("Falta un ciclo activo para asignar docentes")
    cursos = api("GET", f"/api/ciclos/{activo['id']}/cursos", token=token)["data"]
    materias = []
    for curso in cursos:
        for materia in api("GET", f"/api/cursos/{curso['id']}/materias", token=token)["data"]:
            materias.append((curso, materia))
    if not cursos:
        raise RuntimeError("Faltan cursos en el ciclo local")
    docentes = [
        (cfg["DOCENTE_EMAIL"], "Docente", "TIC", ["tic", "tecnologias de la informacion", "tecnologia de la informacion", "programacion", "informatica"]),
        ("dibujo@local.test", "Docente", "Dibujo", ["dibujo tecnico"]),
        ("tecnologia@local.test", "Docente", "Tecnología", ["educacion tecnologica", "tecnologia"]),
        ("matematica@local.test", "Docente", "Matemática", ["matematica"]),
        ("construccion@local.test", "Docente", "Construcción", ["construcciones", "construccion", "sistemas constructivos"]),
    ]
    destino = repo.parent / "sgrc-datos" / "usuarios-locales.json"
    destino.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    anteriores = json.loads(destino.read_text()) if destino.exists() else []
    asignaciones = []
    for email, nombre, apellido, opciones in docentes:
        user = por_email.get(email)
        if user is None:
            user = api("POST", "/api/auth/registro", {
                "nombre": nombre, "apellido": apellido, "email": email,
                "password": cfg["DOCENTE_PASSWORD"],
                "cargoSolicitado": "DOCENTE", "rolSolicitado": "TITULAR",
            })
            if user is None:
                registrados = api("GET", "/api/auth/usuarios?pageSize=200", token=token)["data"]
                user = next(u for u in registrados if u["email"] == email)
        if user["estado"] != "APROBADA":
            api("PATCH", f"/api/auth/usuarios/{user['id']}/estado", {"estado": "APROBADA"}, token)
        elegida = next((pair for opcion in opciones for pair in materias
                        if (re.search(r"\b" + re.escape(opcion) + r"\b", normalizar(pair[1]["nombre"]))
                            if len(opcion) <= 3 else opcion in normalizar(pair[1]["nombre"]))), None)
        if elegida is None:
            curso = cursos[0]
            materia = api("POST", f"/api/cursos/{curso['id']}/materias", {"nombre": apellido}, token)
        else:
            curso, materia = elegida
        for anterior in anteriores:
            if anterior["email"] == email and anterior["materiaId"] != materia["id"]:
                vinculos = api("GET", f"/api/materias/{anterior['materiaId']}/docentes", token=token)["data"]
                for vinculo in vinculos:
                    if vinculo["usuarioId"] == user["id"]:
                        api("DELETE", f"/api/materias/{anterior['materiaId']}/docentes/{vinculo['id']}", token=token)
        vinculos = api("GET", f"/api/materias/{materia['id']}/docentes", token=token)["data"]
        if not any(v["usuarioId"] == user["id"] for v in vinculos):
            api("POST", f"/api/materias/{materia['id']}/docentes", {
                "usuarioId": user["id"], "rol": "TITULAR",
            }, token)
        asignaciones.append({"email": email, "usuarioId": user["id"], "materiaId": materia["id"],
                             "materia": materia["nombre"], "curso": curso["nombre"]})
        print(f"Docente preparado: {email} — {materia['nombre']} — {curso['nombre']}")
    for email, password in [(cfg["ADMIN_AUX_EMAIL"], cfg["ADMIN_AUX_PASSWORD"])] + [
        (d[0], cfg["DOCENTE_PASSWORD"]) for d in docentes
    ]:
        login = api("POST", "/api/auth/login", {"email": email, "password": password})
        if not login.get("token"):
            raise RuntimeError("No se emitió sesión para una cuenta de prueba")
    fd = os.open(destino, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as output:
        json.dump(asignaciones, output, ensure_ascii=False, indent=2)
    print("Listos: dos administradores y cinco docentes. Contraseñas en .env.local.")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, KeyError) as error:
        sys.exit(str(error))
