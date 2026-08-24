import sys, os
from PIL import Image, ImageOps

ORIGEN, DESTINO = sys.argv[1], sys.argv[2]

# nombre_de_origen -> nombre_publicado
MAPA = {
    # docentes
    "form-login": "docente-01-entrar",
    "form-registro": "docente-02-crear-cuenta",
    "form-recuperar": "docente-03-olvide-contrasena",
    "marca-inicio-docente-marcada": "docente-04-pantalla-de-inicio",
    "marca-nueva-reserva-marcada": "docente-05-nueva-reserva",
    "paso-03-reserva-semanal": "docente-06-reserva-semanal",
    "marca-mis-reservas-marcada": "docente-07-mis-reservas",
    "paso-04-cancelar-reserva": "docente-08-cancelar",
    "paso-05-cambiar-computadora": "docente-09-cambiar-computadora",
    "paso-08b-computadoras-desplegado": "docente-10-computadoras",
    "paso-09-avisar-falla": "docente-11-avisar-falla",
    "doc-05-notificaciones": "docente-12-avisos",
    "paso-06-conversacion": "docente-13-conversacion",
    "paso-07-copias-por-correo": "docente-14-copias-por-correo",
    "paso-10-perfil": "docente-15-perfil",
    "doc-08-cambiar-password": "docente-16-cambiar-contrasena",
    "doc-09-horario-admins": "docente-17-horario-admins",
    "mov-01-inicio": "docente-18-telefono",
    # admins
    "marca-inicio-admin-marcada": "admin-01-mostrador",
    "paso-21-menu-administracion": "admin-02-menu",
    "adm2-aprobacion": "admin-03-aprobacion",
    "adm2-academico": "admin-04-academico",
    "adm-04-usuarios": "admin-05-usuarios",
    "adm2-equipos": "admin-06-inventario",
    "adm-06-entregas": "admin-07-entregas",
    "adm2-entregar-sin-reserva": "admin-08-entregar-sin-reserva",
    "adm2-licencias": "admin-09-licencias",
    "adm2-cargar-licencia": "admin-10-cargar-licencia",
    "adm2-bloquear": "admin-11-bloquear",
    "adm-09-jornada": "admin-12-jornada",
    # La jornada: el asistente del primer arranque y la confirmación de lo que
    # se cancela al achicarla (capturar-jornada.mjs).
    "adm-18-jornada-impacto": "admin-12b-jornada-impacto",
    "adm-16-primera-jornada": "admin-12c-primera-jornada",
    "adm2-pedidos": "admin-13-pedidos-de-materia",
    "paso-22-soporte-admin": "admin-14-soporte",
    "paso-23-soporte-conversacion": "admin-15-soporte-conversacion",
    "adm2-reportes": "admin-16-reportes",
    "adm-13-disponibilidad": "admin-17-horarios-admin",
    "adm-12-notificaciones": "admin-18-avisos",
    # Las de los capítulos que se sumaron después (capturar-nuevas.mjs).
    "nue-cerrar-anio": "admin-19-cerrar-anio",
    "nue-calendario-equipo-admin": "admin-20-calendario-equipo",
    "nue-perfil-admin": "admin-21-perfil",
    "nue-calendario-equipo-docente": "docente-19-calendario-equipo",
    # Con qué cuenta se entra a cada equipo (capturar-cuentas.mjs).
    "form-cuentas-admin": "admin-22-cuentas-de-equipo",
    "form-cuenta-nueva": "admin-23-cuenta-nueva",
    "cue-03-cuentas-docente": "docente-20-como-entrar",
    "cue-04-cuentas-telefono": "docente-21-como-entrar-telefono",
}

def recortar_vacio(img, margen=40, pie=280):
    """Corta el vacío de abajo. Se saltean los últimos `pie` píxeles a
    propósito: ahí está el pie de la página web ("SGRC v1.11.0 — software
    libre…"), y si se lo cuenta como contenido, el hueco que queda entre lo
    que importa y ese pie viaja al PDF como media hoja en blanco."""
    gris = img.convert("L")
    ancho, alto = gris.size
    fondo = gris.getpixel((ancho // 2, alto - 5))
    limite = max(1, alto - pie)
    fila_util = limite
    for y in range(limite - 1, -1, -1):
        fila = gris.crop((0, y, ancho, y + 1)).getextrema()
        if abs(fila[0] - fondo) > 12 or abs(fila[1] - fondo) > 12:
            fila_util = y
            break
    return img.crop((0, 0, ancho, min(alto, fila_util + margen)))

for origen, destino in MAPA.items():
    ruta = os.path.join(ORIGEN, origen + ".png")
    if not os.path.exists(ruta):
        print("  ✗ falta", origen)
        continue
    img = Image.open(ruta).convert("RGB")
    antes = img.size
    # Las capturas de un elemento (formularios, capturas ya marcadas) no
    # tienen el pie de la página adentro: saltearlo les comería contenido.
    recorte_completo = origen.startswith("form-") or origen.startswith("marca-")
    img = recortar_vacio(img, pie=0 if recorte_completo else 280)
    # Una captura muy larga entra en la hoja como una tira ilegible. Se corta
    # por abajo: lo que la guía explica está siempre arriba.
    alto_max = int(img.width * 1.35)
    if img.height > alto_max:
        img = img.crop((0, 0, img.width, alto_max))
    # borde suave para que la captura se despegue del papel
    img = ImageOps.expand(img, border=2, fill=(210, 214, 220))
    img.save(os.path.join(DESTINO, destino + ".png"), optimize=True)
    print(f"  ✓ {destino:38s} {antes[0]}x{antes[1]} → {img.size[0]}x{img.size[1]}")
