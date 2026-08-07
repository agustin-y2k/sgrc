.PHONY: test lint build docker-build run dev rebuild dev-down run-prod stop restart down logs ps migrate backup seed-admin seed-datos coverage-report

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

# Aplica una migración sobre una base que ya existe: las de /migrations solo
# corren solas la primera vez, con el volumen vacío.
#
#   make migrate ARCHIVO=migrations/005_dia_semana_lectivo.sql
#
# ON_ERROR_STOP hace que psql devuelva un código de salida distinto de cero
# si la migración aborta; sin eso, una migración que se corta a propósito
# (porque encontró datos que la regla nueva dejaría afuera) parecía exitosa.
#
# El usuario y la base salen del ENTORNO DEL CONTENEDOR de Postgres, que ya
# los tiene (el compose se los pasa por `environment:`). Antes esta receta
# hacía `. ./.env`, o sea ejecutaba el archivo como un script de shell, y
# eso se rompe con cualquier valor que lleve espacios y no esté entrecomillado
# — que es exactamente la forma en que Google entrega las contraseñas de
# aplicación para SMTP ("abcd efgh ijkl mnop"). El resultado era un
# `qdnm: not found` mientras se intentaba aplicar una migración: un error que
# no menciona ni el .env ni la línea culpable. Compose parsea ese archivo con
# sus propias reglas y tolera los espacios, así que el sistema arrancaba
# perfecto y solo fallaban estos dos comandos.
migrate:
ifndef ARCHIVO
	@echo "Falta indicar el archivo. Uso:"
	@echo "  make migrate ARCHIVO=migrations/005_dia_semana_lectivo.sql"
	@echo ""
	@echo "Migraciones disponibles:"
	@ls -1 migrations/*.sql | sed 's/^/  /'
	@exit 1
else
	@docker compose exec -T postgres sh -c \
		'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < $(ARCHIVO)
	@echo "Aplicada: $(ARCHIVO)"
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
