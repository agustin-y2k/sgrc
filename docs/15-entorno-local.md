# Entorno local independiente

Esta guía sirve para cualquier clon del repositorio. El entorno local usa el
proyecto Docker Compose `sgrc-local`, su propio volumen `sgrc-local_pgdata` y
los archivos privados `.env.local` y `docker-compose.local.yml`.
No requiere una instalación de producción, un respaldo ni inventario previo.

## Preparación

Requisitos: Docker con Compose **2.24.4 o posterior** (por `!override`), Make y
Python 3. No hacen falta Go, Node ni PostgreSQL en el host para levantar los
servicios compilados en contenedores.

Desde la raíz del repositorio:

```bash
make local-init
make local
```

`make local-init` crea los dos archivos privados a partir de
[.env.local.example](../.env.local.example) y
[docker-compose.local.example.yml](../docker-compose.local.example.yml).
Genera secretos y contraseñas aleatorios, independientes para cada función, y
crea los archivos con permisos `0600`. No imprime las claves ni reemplaza
archivos existentes. `make local` también ejecuta esa preparación si hace falta.

La plantilla de Compose no monta respaldos ni carpetas de una institución.
Todos los puertos publicados escuchan sólo en `127.0.0.1`; el túnel permanece
fuera de los servicios que inicia `make local`. El perfil opcional se llama
`tunel` en todos los entornos y hereda `restart: unless-stopped` del Compose base.

| Servicio | Dirección local |
|---|---|
| Aplicación | http://localhost:8081 |
| API | http://localhost:8080 |
| PostgreSQL | localhost:15432 |
| Grafana, opcional | http://localhost:13000 |
| Prometheus, opcional | http://localhost:19090 |
| Logs, opcional | http://localhost:18888 |

Las claves se consultan en `.env.local`. El administrador inicial se identifica
con `SEED_ADMIN_EMAIL` y `SEED_ADMIN_PASSWORD`; el correo de la plantilla es
`admin@local.test`. Cambiar el archivo no modifica la contraseña de una cuenta
que ya existe en la base.

## Variables por servicio y copias anteriores

`make local` usa `--env-file .env.local` para interpolar el Compose. El backend,
PostgreSQL, el túnel opcional y Grafana reciben las variables declaradas para cada
servicio; el archivo completo no se entrega a ningún contenedor.

Si la copia local se creó con una plantilla anterior, quitar de
`docker-compose.local.yml` las entradas `env_file` de `sgrc-app` y `cloudflared`.
Retirar también la entrada antigua `cloudflared: profiles: [tunel-local]`; el
perfil `tunel` ya está definido en el Compose base. Si esa era la única opción
de `cloudflared`, quitar su bloque del overlay. Conservar los puertos, redes,
volúmenes y demás ajustes propios. El generador
conserva los archivos existentes y no aplica esa actualización por su cuenta.
La plantilla pública actual ya incorpora la separación. Las variables nuevas se
aplican al recrear los contenedores; `restart` conserva las anteriores.

## Primera carga

Una base nueva contiene el esquema y el administrador inicial. Desde la
interfaz se cargan el ciclo lectivo, los cursos, las materias, la jornada y el
inventario. No se distribuyen datos de ninguna institución con el proyecto.

Después de crear al menos un ciclo activo y un curso, `make local-usuarios`
permite preparar dos administradores y cinco docentes ficticios con materias
de ejemplo. Usa las credenciales de `.env.local` y la API local. Su resumen se
guarda en `../sgrc-datos/usuarios-locales.json`, fuera del repositorio. No es
necesario para operar el sistema ni restaura inventario.

Para una demostración sembrada automáticamente existe también el entorno de
[desarrollo](11-operacion.md), con `docker-compose.dev.yml`. Elegir un entorno
por vez si se usan los mismos puertos de API e interfaz.

## Comandos

```bash
make local                 # preparar configuración e iniciar o recompilar
make local-observabilidad  # iniciar métricas, tableros y logs
make local-ps              # ver estado
make local-logs            # seguir registros de backend y frontend
make local-stop            # detener conservando los datos
make local-usuarios        # preparar usuarios ficticios después de la primera carga
```

No usar `docker compose down -v` sobre una copia con datos que se quieran
conservar: elimina el volumen de ese proyecto.

## Personalización privada

Los cambios de puertos, subred, rutas de respaldo y credenciales se hacen en
`docker-compose.local.yml` y `.env.local`. Si se cambia la subred, ajustar
`TRUSTED_PROXIES`; si se cambia el puerto web, ajustar `FRONTEND_ORIGIN` y la
URL de las herramientas que consulten la API. Estos archivos están ignorados
por Git y excluidos de los contextos Docker.

Los respaldos se guardan fuera del repositorio o en `datos-locales/`,
`sgrc-datos/` o `backups/`, todas privadas. La importación se decide y ejecuta
por separado: el arranque no restaura archivos ni sobrescribe la base.
Los detalles de una instalación concreta tampoco se agregan a esta guía.

Las plantillas locales dejan deshabilitados los proveedores externos. Su
configuración, cuando se necesite, corresponde a la copia privada.

Ver [Qué se publica y qué se mantiene privado](16-publicacion-repositorio.md)
para distinguir plantillas, configuración compartida y archivos de cada sitio.

## Actualizar una instalación local

Usar siempre `--env-file .env.local` y la combinación local indicada arriba.
La versión `v1.22.0` incorpora las migraciones 030–035 respecto de `v1.21.0`.
Un respaldo o ensayo sobre otra base no significa que el entorno habitual ya esté
actualizado. Identificar el proyecto y el volumen antes de reemplazar servicios,
respaldar sus datos y verificar salud y versión del esquema después del arranque.

El entorno local puede funcionar sin cuentas externas configuradas. Los archivos
activos, credenciales, inventario y evidencias de pruebas permanecen privados.
Las guías se generan en una base efímera diferente, exclusivamente con datos
ficticios; no necesitan copiar este inventario ni sus usuarios.
