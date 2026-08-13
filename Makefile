.PHONY: test lint build docker-build run dev rebuild dev-down run-prod stop restart down logs ps migrate migrate-status psql backup seed-admin seed-datos coverage-report observabilidad observabilidad-stop

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

# Levanta el sistema MÁS Prometheus y Grafana. Sin este comando, esos dos no
# arrancan: están detrás de un profile del compose justamente para que quien
# solo quiera usar el sistema no cargue con ellos.
#
# Grafana queda en http://localhost:3000 (usuario admin, contraseña del
# .env). Desde otra máquina se llega por un túnel de SSH: publicarlo en la
# red de la institución sería dejar un panel de administración a la vista.
observabilidad:
	docker compose --profile observabilidad up -d

# Apaga solo los tableros y deja el sistema andando.
observabilidad-stop:
	docker compose --profile observabilidad stop prometheus grafana

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
	@docker compose exec -T postgres sh -c \
		'pg_dump -U "$$POSTGRES_USER" "$$POSTGRES_DB"' > backup-sgrc-$$(date +%F).sql
	@echo "Backup en backup-sgrc-$$(date +%F).sql"

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
