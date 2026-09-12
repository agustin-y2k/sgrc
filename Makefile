.PHONY: test lint build docker-build run dev rebuild dev-down run-prod levantar reconectar-tunel stop restart down logs ps migrate migrate-status psql backup seed-admin seed-datos coverage-report observabilidad observabilidad-stop capturas capturas-pdf

test:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | grep total

lint:
	golangci-lint run ./...

build:
	CGO_ENABLED=0 GOOS=linux go build -o bin/sgrc-app ./cmd

docker-build:
	docker build -t sgrc-app:latest .

# ── Operación (el detalle está en docs/11-operacion.md) ───────────────

# El overlay de desarrollo va explícito: publica 8080 y 5432 al host, que en
# producción no se exponen (ver docker-compose.dev.yml).
DEV := docker compose -f docker-compose.yml -f docker-compose.dev.yml

run:
	$(DEV) up --build

# Lo mismo pero en segundo plano: devuelve la terminal en vez de quedarse
# mostrando los logs (para eso está `make logs`).
dev:
	$(DEV) up -d --build

# Recompila y reemplaza UN servicio, sin tocar la base ni el resto.
#
#   make rebuild SERVICIO=frontend
#
# Existe por un error que se repite: después de cambiar código del frontend,
# `make restart` reinicia nginx pero NO recompila la SPA, así que el
# navegador sigue sirviendo el bundle viejo. Se prueba el cambio, no está, y
# uno sale a buscar un bug que no existe. Hace falta `--build`.
rebuild:
ifndef SERVICIO
	@echo "Falta indicar el servicio. Uso:"
	@echo "  make rebuild SERVICIO=frontend"
	@echo "  make rebuild SERVICIO=sgrc-app"
	@exit 1
else
	$(DEV) up -d --build $(SERVICIO)
endif

# Borra los contenedores de desarrollo, incluido seed-datos, que solo existe
# en el overlay: `make down` a secas no lo conoce y lo deja dado vuelta.
dev-down:
	$(DEV) down

# Levanta exactamente lo que corre en el servidor, sin puertos publicados.
run-prod:
	docker compose up --build

# ── Despliegue con un túnel que NO es el del compose ──────────────────
#
# Cuando el Cloudflare Tunnel ya existía antes que este proyecto —creado
# desde el panel, compartido con otros sitios del mismo servidor— el
# `cloudflared` del compose no sirve: vive solo en sgrc-net y no podría
# resolver lo que haya afuera. Pero `make run-prod` lo levanta igual, y sin
# un TUNNEL_TOKEN válido muere con "Provided Tunnel token is not valid".
#
# Este target levanta el sistema NOMBRANDO los servicios, que es lo que hay
# que hacer en esa instalación, y así deja de depender de que alguien se
# acuerde de la lista.
#
#   make levantar                 el sistema
#   make levantar TABLEROS=1      además Prometheus, Grafana y Dozzle
#
# Nombrar esos tres alcanza para levantarlos aunque estén en el perfil
# `observabilidad`: Compose activa el perfil de un servicio que se pide
# explícitamente.
SERVICIOS := postgres sgrc-app frontend

levantar:
	docker compose up -d --build $(SERVICIOS) $(if $(TABLEROS),prometheus grafana dozzle)
	@$(MAKE) --no-print-directory reconectar-tunel

# Un `docker compose down` borra la red y la recrea al levantar, y el túnel
# externo queda afuera: la pila entera sana y el sitio inalcanzable, sin nada
# en los logs porque el pedido no llega nunca. Esto lo reconecta.
#
# Es idempotente y no falla si no hay ningún túnel externo: en una
# instalación que usa el cloudflared del compose, no hace nada.
reconectar-tunel:
	@docker ps --format '{{.Names}}' | grep -qx cloudflared || { \
		echo "sin túnel externo (contenedor 'cloudflared'): nada que reconectar"; exit 0; }; \
	red=$$(docker inspect -f '{{range $$k, $$v := .NetworkSettings.Networks}}{{$$k}}{{end}}' \
		$$(docker compose ps -q frontend)); \
	if docker network connect $$red cloudflared 2>/dev/null; then \
		echo "túnel reconectado a $$red"; \
	else \
		echo "el túnel ya estaba conectado a $$red"; \
	fi

# Apaga los contenedores sin borrarlos: los datos siguen ahí y run-prod
# vuelve a levantar todo tal cual estaba.
stop:
	docker compose stop

# Reinicia sin reconstruir. Para aplicar cambios de código va run-prod, que
# además recompila.
restart:
	docker compose restart

# Borra los contenedores. Los datos sobreviven porque viven en el volumen
# pgdata — borrarlos también requiere `docker compose down -v`, que NO tiene
# atajo acá a propósito: destruye la base entera y no se puede deshacer.
down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

# ── Observabilidad (el detalle está en docs/12-observabilidad.md) ─────

# Levanta el sistema MÁS Prometheus, Grafana y Dozzle. Sin este comando, esos
# tres no arrancan: están detrás de un profile del compose justamente para que
# quien solo quiera usar el sistema no cargue con ellos.
#
# Grafana queda en http://localhost:3000 (usuario admin, contraseña del .env)
# y Dozzle —los logs en el navegador— en http://localhost:8888. Desde otra
# máquina se llega por un túnel de SSH: publicarlos en la red de la
# institución sería dejar dos paneles de administración a la vista.
observabilidad:
# Antes de levantar nada, la contraseña de Grafana. El compose tiene un valor
# por defecto —lo necesita para que el archivo se pueda interpretar incluso en
# un despliegue que nunca use este perfil— y ese valor está escrito en el
# repositorio, así que si nadie lo cambia el panel queda con una contraseña
# pública. Este chequeo es la puerta: el perfil se levanta desde acá, y desde
# acá se puede mirar el .env y negarse.
	@clave=$$(grep -E '^GRAFANA_PASSWORD=' .env 2>/dev/null | cut -d= -f2-); \
	if [ -z "$$clave" ] || [ "$$clave" = "cambiar_por_una_contrasena_larga" ]; then \
		echo "GRAFANA_PASSWORD no está definida en el .env (o quedó con el valor de ejemplo)."; \
		echo "Grafana queda con una contraseña que está publicada en el repositorio."; \
		echo "Generá una con \`openssl rand -base64 24\` y ponela en el .env."; \
		exit 1; \
	fi
	docker compose --profile observabilidad up -d

# Apaga solo los tableros y deja el sistema andando.
observabilidad-stop:
	docker compose --profile observabilidad stop prometheus grafana dozzle

# ── Esquema de la base ────────────────────────────────────────────────
#
# Normalmente no hay nada que correr a mano: sgrc-app aplica las migraciones
# pendientes cada vez que arranca, con goose, y anota lo aplicado en la tabla
# goose_db_version (ver cmd/migrate.go). Después de un `make run-prod` la base
# ya está al día.
#
# Estos dos comandos son para mirar y para actuar sin reiniciar la aplicación.
# Los ejecuta el propio binario adentro del contenedor: la imagen es `scratch`
# y no tiene psql ni shell, así que `docker compose exec` invoca directamente
# a /sgrc-app, que sí conoce las variables de conexión del entorno.

# Qué migraciones están aplicadas y cuáles faltan.
migrate-status:
	@docker compose exec sgrc-app /sgrc-app migrate status

# Aplica las pendientes contra la base en marcha. Sirve para poner al día una
# instalación sin esperar al próximo reinicio; el arranque hace lo mismo.
#
# No hay atajo para revertir: `goose down` sobre el esquema inicial borra las
# tablas y con ellas los datos. El mismo criterio que con `docker compose
# down -v`, que tampoco lo tiene.
migrate:
	@docker compose exec sgrc-app /sgrc-app migrate up

# Abre una consola SQL contra la base, para mirar cuando algo se pone raro.
#
# Existe porque el comando a mano tiene una trampa: POSTGRES_USER vive en el
# .env, que el shell de afuera NO lee. Escrito de la forma obvia
# —`psql -U "$POSTGRES_USER"`— la variable llega vacía y psql intenta entrar
# con el usuario del sistema, fallando con `role "root" does not exist`, que
# no menciona ni el .env ni la variable. Las comillas SIMPLES de abajo son lo
# que hace que se expanda adentro del contenedor, donde sí existe. Mismo
# truco que backup.
#
#   make psql
#   make psql SQL="SELECT count(*) FROM equipo;"
psql:
ifdef SQL
	@echo '$(SQL)' | docker compose exec -T postgres sh -c \
		'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
else
	@docker compose exec postgres sh -c \
		'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
endif

# La base es lo único que no se puede reconstruir: el código y las imágenes
# se vuelven a compilar, los datos no. Conviene correrlo antes de actualizar
# y antes de aplicar una migración.
backup:
# `umask 077` antes de la redirección: el archivo lo crea la shell de make, y
# con el umask normal queda 0644, o sea legible por cualquier usuario del
# servidor. El volcado tiene el nombre, el correo y el cargo de todo el mundo en
# texto plano —las contraseñas de las máquinas no, esas van cifradas con
# CUENTAS_SECRET, ver internal/shared/secretos—. Con 0600 lo lee solo quien lo
# generó.
	@umask 077; docker compose exec -T postgres sh -c \
		'pg_dump -U "$$POSTGRES_USER" "$$POSTGRES_DB"' > backup-sgrc-$$(date +%F).sql
	@echo "Backup en backup-sgrc-$$(date +%F).sql (solo lo podés leer vos: 0600)"

# ── Datos iniciales ───────────────────────────────────────────────────

seed-admin:
	@echo "El primer Admin lo siembra cmd/main.go al arrancar, de forma idempotente:"
	@echo "si no hay ningún ADMIN en estado APROBADA, deja lista la cuenta de"
	@echo "SEED_ADMIN_EMAIL con la contraseña del .env (ver cmd/seed_admin.go y"
	@echo "internal/shared/adminseed)."
	@echo "No hay script aparte que correr — alcanza con 'make run-prod'."
	@echo ""
	@echo "Para datos con los que probar (ciclo, materia, docente, PCs), ver 'make seed-datos'."

# Datos mínimos para poder usar el sistema recién levantado. Solo desarrollo:
# le pega a la API con el Admin sembrado, así que el backend tiene que estar
# corriendo.
seed-datos:
	./scripts/sembrar-datos-de-prueba.sh

coverage-report:
	go tool cover -html=coverage.out -o coverage.html

# ── Las guías (el detalle está en docs/guias/generar/README.md) ───────

# Regenera TODAS las capturas y los dos PDF, de punta a punta.
#
# Levanta su propia pila en el proyecto `sgrc-capturas` y la baja al terminar:
# la base de desarrollo no se toca. Tarda varios minutos y necesita los puertos
# 8080/8081/5432 libres, así que si tenés la pila de desarrollo arriba hay que
# pararla antes (`make stop`) — las dos usan los mismos puertos y no conviven.
capturas:
	./docs/guias/generar/capturar-todo.sh

# Solo los dos PDF, sin capturas, sin base y sin Docker Compose.
#
# Es lo único que hace falta cuando cambia la VERSIÓN o el texto de una guía:
# la portada es parte del documento, pero las capturas muestran a propósito el
# pie de la versión anterior —se sacan antes del bump— y regenerar cincuenta
# imágenes por un dígito no se justifica. Son dos minutos.
capturas-pdf:
	./docs/guias/generar/capturar-todo.sh --solo-pdf
