# Operación — SGRC

Cómo se pone en marcha el sistema, cómo se para, cómo se reinicia y qué hacer
cuando algo no anda. Todo desde el servidor de la institución, con Docker
Compose — no hace falta tener Go ni Node instalados para operarlo.

Los comandos largos tienen su atajo en el `Makefile`; abajo aparecen los dos,
porque el atajo sirve mientras estés parado en la carpeta del proyecto y el
comando completo sirve siempre.

---

## 0. Lo que se busca a las apuradas

Todos los comandos se corren **parado en la carpeta del proyecto**.

| Quiero… | En mi máquina (desarrollo) | En el servidor (producción) |
|---|---|---|
| **Levantar todo** | `make dev` (en segundo plano) o `make run` (mostrando los logs) | `make run-prod` |
| **Entrar** | `http://localhost:8081` | El dominio del túnel |
| **Pegarle a la API sola** | `http://localhost:8080` | No se publica: solo se entra por el frontend |
| **Parar** | `make stop` | `make stop` |
| **Reiniciar sin recompilar** | `make restart` | `make restart` |
| **Aplicar cambios de código** | `make dev` de nuevo, o `make rebuild SERVICIO=frontend` | `make run-prod` de nuevo |
| **Ver los logs** | `make logs` | `make logs` |
| **Empezar de cero** | `docker compose down -v && make run` | Nunca sin backup (§6) |
| **Mirar la base** | `make psql` | `make psql` |

**Cuidado con "reiniciar" y "aplicar cambios": no son lo mismo.** `make
restart` levanta de nuevo los contenedores con la imagen que ya estaba
compilada. Si tocaste código, hace falta `--build` — que es lo que hacen
`make dev`, `make rebuild` y `make run-prod`. El caso que más confunde es el
frontend: `restart` reinicia nginx pero la SPA sigue siendo el bundle viejo,
así que probás el cambio, no está, y salís a buscar un bug que no existe.

### Usuarios para probar (solo desarrollo)

Los crea `make run` / `make dev` mediante el servicio `seed-datos` (§7).

| Rol | Email | Contraseña |
|---|---|---|
| Admin | el `SEED_ADMIN_EMAIL` del `.env` (por defecto `admin@tuinstitucion.edu.ar`) | el `SEED_ADMIN_PASSWORD` del `.env` |
| Docente | `docente@escuela.edu.ar` | `docente_password_123` |

Esa contraseña de docente está escrita en `scripts/sembrar-datos-de-prueba.sh`
y es pública. Por eso el script **se niega a correr contra algo que no sea
local**: en el servidor de producción ese usuario no existe.

### Y si compré el dominio, ¿qué toco?

**Una sola línea, en un solo archivo: `FRONTEND_ORIGIN`, en tu `.env`.**
Ver §9.3.

No hay ningún `localhost` que reemplazar por el dominio, aunque lo parezca:
los `localhost` del repo son de desarrollo y no corren en producción, y lo
que ves en `nginx.conf` son nombres de contenedor de la red interna de
Docker. Poner el dominio ahí arma un bucle. El porqué está en **§9.4**.

### ¿Y si necesito otro puerto, o salir sin Cloudflare Tunnel?

Ninguna de las dos cosas hace falta en un despliegue normal. Si igual las
necesitás: **§10** para los puertos (ojo con la trampa de `APP_PORT`) y
**§11** para exponer el sistema por el router o detrás de otro proxy.

---

## 1. Puesta en marcha (una sola vez)

### 1.1 Configurar el `.env`

```bash
cp .env.example .env
```

Después hay que **completar los valores reales**. Los cuatro que no se pueden
dejar como vienen:

| Variable | Qué poner | Cómo generarla |
|---|---|---|
| `POSTGRES_PASSWORD` | La contraseña de la base | `openssl rand -base64 24` |
| `JWT_SECRET` | El secreto que firma las sesiones | `openssl rand -base64 48` |
| `SEED_ADMIN_PASSWORD` | La contraseña del primer Admin | La elegís vos, mínimo 8 caracteres |
| `TUNNEL_TOKEN` | El token del túnel | Lo da el panel de Cloudflare |
| `GRAFANA_PASSWORD` | La contraseña de `admin` en Grafana, solo si vas a levantar los tableros | `openssl rand -base64 24` |

> `GRAFANA_PASSWORD` se siembra **una sola vez, en el primer arranque de
> Grafana**: después el usuario vive en el volumen `grafana-datos` y editar el
> `.env` ya no la cambia. Si hay que cambiarla más tarde,
> `docker compose exec grafana grafana cli admin reset-admin-password '…'`.

El resto tiene valores razonables por defecto, salvo dos que dependen del
dominio:

- **`FRONTEND_ORIGIN`**: el dominio público desde el que se sirve el sistema,
  con `https://` y **sin barra final** (ej. `https://sgrc.tuescuela.edu.ar`).
  Si está mal, el proceso no arranca y lo dice en el log — es a propósito:
  con este valor mal puesto el navegador rechazaría todos los requests sin
  ninguna explicación visible.
- **`VITE_API_URL`**: va **vacío**. El navegador pide `/api/...` al mismo
  host que le sirvió la página (ver README, "Cómo entra el tráfico").

> El `.env` no se comparte ni se publica: tiene la contraseña de la base y el
> secreto de las sesiones.

#### Poner al día un `.env` que ya está en uso

Un `.env` de servidor envejece de una manera particular: los valores están
bien, pero le faltan los comentarios que explican cada variable y las que se
agregaron en versiones posteriores. Para volver a juntarlo con el ejemplo, sin
tocar ningún valor:

```bash
umask 077                                   # el archivo nuevo, solo para vos
./scripts/env-con-comentarios.sh > .env.nuevo
diff .env .env.nuevo                        # mirar antes de reemplazar
cp .env .env.respaldo && mv .env.nuevo .env
```

Toma la estructura y los comentarios de `.env.example`, y para cada variable
usa **el valor que ya tenías**. Lo que el ejemplo trae y tu instalación no
tiene queda con el valor de ejemplo y se avisa por pantalla; lo que tenés vos y
el ejemplo no conoce se conserva al final del archivo, en su propia sección.
Nada se descarta en silencio.

El script escribe a la salida estándar y nunca pisa el `.env` en uso: el
reemplazo es una decisión tuya, después de leer el `diff`.

### 1.1.b Configurar el correo (opcional, pero conviene)

Sin esto el sistema funciona igual: los avisos siguen llegando a la campana de
notificaciones. Lo que **no** funciona es el "olvidé mi contraseña" — el
enlace ni aparece en la pantalla de ingreso, y hay que resetear a mano desde
`/admin/usuarios` cada vez que un docente se olvide la suya.

Sirve cualquier servidor SMTP que hable STARTTLS en el puerto 587. El ejemplo
usa una cuenta de Gmail institucional con una **contraseña de aplicación**, que
no es la contraseña con la que se entra a Gmail:

1. En la cuenta de la institución, activá la verificación en dos pasos:
   <https://myaccount.google.com/signinoptions/twosv>. Sin eso el paso
   siguiente no existe.
2. Entrá a <https://myaccount.google.com/apppasswords> y creá una. Google la
   muestra como `abcd efgh ijkl mnop`; se puede pegar con los espacios.
3. En el `.env`:

   ```
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USER=la-cuenta-de-la-institucion@gmail.com
   SMTP_PASSWORD=abcd efgh ijkl mnop
   SMTP_FROM=la-cuenta-de-la-institucion@gmail.com
   SMTP_FROM_NAME=Nombre de la institución
   ```

4. `make restart` y mirá el log: tiene que decir
   `correo saliente habilitado vía smtp.gmail.com`.

Cada persona elige después, desde la pantalla de notificaciones, qué avisos
quiere recibir por correo (RF-05.13). De fábrica salen los de la cuenta —el
código de recuperación y el "ya podés entrar", que no se pueden apagar—, los
que traen noticias de algo que hizo otro, y para los Admin, la cuenta que
espera aprobación. El resto se prende a mano. Nada de lo que se apague se
pierde: esos mismos avisos siguen apareciendo en la campana, para todos.

El límite de Gmail es de unos 500 destinatarios por día, de sobra para avisos.
Gmail exige contraseña de aplicación desde 2022; el envío por SMTP en sí sigue
siendo gratuito.

> **Ojo con dejarlo a medias.** Con `SMTP_HOST` puesto pero sin `SMTP_FROM`/
> `SMTP_USER` o sin `SMTP_PASSWORD`, el proceso **no arranca** y dice qué
> falta. Es deliberado: si arrancara, cada envío fallaría en silencio dentro
> de una goroutine y el único síntoma sería "no me llegan los mails".

Para probarlo de punta a punta sin tocar ninguna cuenta real: entrá a
`/recuperar-password`, poné tu propio email y fijate si llega el código
(revisá spam la primera vez).

> **Que los correos no caigan en spam depende de tres cosas, y dos son de
> despliegue.** La primera ya está resuelta por salir a través de una casilla
> del proveedor: SPF, DKIM y DMARC los firma él, y la reputación de la IP
> saliente es suya y no del servidor de la institución. Las otras dos hay que
> cuidarlas acá:
>
> - **`FRONTEND_ORIGIN` tiene que ser un dominio con HTTPS.** Va tal cual
>   adentro del cuerpo de cada correo, y un enlace a una IP cruda sobre HTTP
>   (`http://192.168.1.50:8081`) es de los disparadores más directos que tiene
>   un filtro de spam. Si el sistema solo se usa dentro de la red interna,
>   conviene dejar la variable vacía: sin ella los correos salen sin enlace, que
>   es mejor que salir con uno que los hunde.
> - **Los rebotes duros se corrigen, no se acumulan.** Una cuenta creada con la
>   dirección mal escrita (RF-01.3 la deja escribir a mano) rebota en cada
>   aviso, y acumular rebotes es de las pocas cosas que arruinan la reputación
>   de un remitente. Cuando el log diga `destinatario rechazado`, corregí esa
>   dirección en la cuenta en vez de esperar que se resuelva sola.

### 1.2 Levantar todo

```bash
make run-prod          # docker compose up --build
```

La primera vez tarda unos minutos: compila el binario Go, compila la SPA y
crea la base aplicando el esquema de `migrations/`.

Cuando termina, el primer Admin ya está creado con el `SEED_ADMIN_EMAIL` y el
`SEED_ADMIN_PASSWORD` del `.env`. Entrá con esa cuenta y **cambiale la
contraseña**.

### 1.3 Comprobar que quedó sano

```bash
docker compose ps
```

`sgrc-app` tiene que figurar **`healthy`**. No es un adorno: ese estado sale de
que el proceso consultó Postgres y respondió. Lo ejecuta el propio binario
(`sgrc-app healthcheck`), porque la imagen es `scratch` y no trae `curl`. Si dice `unhealthy` o
`starting` por más de un minuto, mirá los logs (§4).

### 1.4 Cargar el inventario

El sistema arranca vacío a propósito: no inventa carros ni equipos. Desde la
interfaz, con la cuenta de Admin:

1. **Ciclo lectivo** (`/admin/academico`) → sin ciclo no hay cursos ni
   materias, y sin materias nadie puede reservar.
2. **Cursos y materias**, y asignar los docentes a cada materia.
3. **Carros y equipos** (`/admin/inventario`) → sin equipos cargados no hay nada que
   reservar.

Los docentes se autorregistran y quedan pendientes hasta que un Admin los
aprueba en `/admin/aprobacion`.

---

## 2. Arranque, parada y reinicio

Todos estos comandos se corren **parado en la carpeta del proyecto**.

| Qué querés hacer | Comando | Qué pasa con los datos |
|---|---|---|
| Arrancar | `make run-prod` | — |
| Arrancar en segundo plano | `docker compose up -d --build` | — |
| **Parar** (apagar el sistema) | `make stop` | Se conservan |
| **Reiniciar** sin cambios de código | `make restart` | Se conservan |
| **Reiniciar aplicando cambios** | `make run-prod` de nuevo | Se conservan |
| Parar y borrar los contenedores | `make down` | Se conservan (viven en el volumen) |
| Parar y **borrar la base entera** | `docker compose down -v` | **Se pierden todos** |

La diferencia que importa:

- **`stop`** apaga los contenedores pero los deja creados. Es lo que querés
  para apagar el sistema un fin de semana largo: `make run-prod` lo vuelve a
  levantar con todo adentro.
- **`down`** además borra los contenedores. Los datos igual sobreviven,
  porque viven en el volumen `pgdata`, no adentro del contenedor.
- **`down -v`** borra también el volumen: eso **destruye la base de datos**,
  con usuarios, reservas e inventario. Es útil solo para empezar de cero en
  desarrollo. No hay forma de deshacerlo sin un backup (§6).

### Si el túnel no es el del compose

En una instalación donde el Cloudflare Tunnel ya existía antes que este
proyecto —creado desde el panel, compartido con otros sitios del mismo
servidor— el servicio `cloudflared` del compose **no sirve**: vive solo en
`sgrc-net` y no podría resolver nada de afuera. Y `make run-prod` lo levanta
igual, así que muere en cada arranque con «Provided Tunnel token is not valid».

Para esos casos:

```bash
make levantar              # postgres, sgrc-app y frontend, nada más
make levantar TABLEROS=1   # además Prometheus y Grafana
```

Levanta nombrando los servicios y, al terminar, **reconecta el túnel externo a
la red** si encuentra un contenedor llamado `cloudflared`. Eso último importa
después de cualquier `docker compose down`: la red se recrea y el túnel queda
afuera, con el síntoma más difícil de diagnosticar que da este sistema —la pila
entera sana y el sitio inalcanzable, sin una sola línea en los logs, porque el
pedido nunca llega—. Si el túnel ya estaba conectado, lo dice y no hace nada;
si no hay ninguno, tampoco falla.

También se puede correr suelto, cuando el sitio dejó de responder sin motivo
aparente:

```bash
make reconectar-tunel
```

El apagado es ordenado: al recibir la señal, la API deja de aceptar pedidos
nuevos, termina los que estaban en curso (hasta 15 segundos), espera a que
salgan las notificaciones pendientes y recién ahí cierra la conexión a la
base. Cortar el proceso a lo bruto podía dejar una reserva a medio confirmar
que el docente veía como un error de red.

### Reiniciar un solo servicio

```bash
docker compose restart sgrc-app     # solo la API
docker compose restart frontend     # solo nginx + la SPA
```

---

## 3. Actualizar el sistema

Con los archivos nuevos ya en el servidor:

```bash
make run-prod          # reconstruye las imágenes y reemplaza los contenedores
```

Si la actualización trae **cambios en el esquema** (archivos nuevos en
`migrations/`), no hay nada extra que hacer: el binario los aplica solo al
arrancar. Conviene igual mirar el log —la línea empieza con `goose:`— y, si
querés confirmarlo después, `make migrate-status`. El detalle está en §5.

---

## 4. Ver qué está pasando

```bash
make logs                          # todo, en vivo
docker compose logs -f sgrc-app    # solo la API
docker compose logs --tail=100 postgres
```

Qué buscar en el arranque de `sgrc-app`:

```
zona horaria: America/Argentina/Buenos_Aires (ahora: ...)
conectado a sgrc_db
goose: successfully migrated database to version: 1   ← o "no migrations to run"
admin inicial: cuenta ... lista            ← solo si hizo falta sembrarlo
correo saliente habilitado vía smtp.gmail.com
aviso de vida configurado para: ...        ← si se configuró (ver abajo)
```

### Enterarse cuando algo deja de correr

El sistema tiene tres barridos de fondo —vencimiento de reservas, entregas y
devoluciones, aviso de licencias— que corren en goroutines del mismo proceso.
Si una **muere o se cuelga**, el proceso sigue vivo, la web responde y el
healthcheck da verde: lo que deja de pasar se descubre semanas más tarde,
cuando alguien pregunta por qué su reserva sigue abierta.

Un aviso "cuando algo falla" no sirve para esto, porque una goroutine muerta
tampoco puede avisar. Va al revés: **cada barrido le pega a una URL cada vez
que termina bien, y el servicio externo alerta cuando ese aviso deja de
llegar.** El silencio es la señal.

Se activa poniendo las tres `PING_URL_*` del `.env` (ver `.env.example`, que
detalla qué período configurar en cada una). Sirve cualquier servicio de
*heartbeat* o *cron monitoring* que entregue una URL por chequeo; los hay
gratuitos. Sin configurar, el sistema arranca igual y lo dice en el log.

> **Esto no reemplaza un monitor externo del sitio**, y conviene tener los
> dos. Si el túnel se cae, el sistema queda inalcanzable desde afuera con
> todo sano adentro — y los avisos de los barridos van a seguir llegando
> puntualmente, porque los barridos siguen corriendo. Para ese caso hace
> falta algo que consulte el dominio **desde afuera de la institución**.

### Enterarse cuando el sitio deja de responder

No requiere configurar nada en el sistema: la aplicación ya expone lo que un
monitor necesita, y cualquier servicio de *uptime* sirve.

**El monitor no es parte del sistema y no hay que desplegar nada.** Es algo
que, desde afuera, le pide una página al dominio cada cinco minutos y anota
si contestó — como llamar a un teléfono para ver si suena. Quien llama no
necesita una copia del teléfono. En la variante contratada, todo se hace
llenando un formulario en la web del servicio: no se instala ni se levanta
nada en ningún lado.

Lo único imprescindible es que la consulta **salga desde fuera de la red de
la institución**. Un monitor instalado en el mismo servidor no cubre este
caso: si lo que se cae es el servidor —o el túnel—, el monitor se cae con
él y el silencio se confunde con normalidad.

Qué configurar en el servicio que se elija:

| | |
|---|---|
| **URL** | `https://<el-dominio>/health` |
| **Espera** | código `200`, y si permite verificar el cuerpo, que contenga `"status":"ok"` |
| **Frecuencia** | cada 5 minutos alcanza y sobra; es lo que suelen dar los planes gratuitos |
| **Alertar tras** | 2 fallos seguidos, no 1 |

Los dos detalles que hacen la diferencia:

- **`/health` y no la raíz del sitio.** nginx sirve la interfaz aunque el
  backend esté muerto, así que `/` devuelve 200 con la base caída: un verde
  que miente. `/health` hace un ping real a Postgres y responde 503 si no
  contesta (ver `cmd/main.go`), así que cubre los tres eslabones de una vez
  —túnel, nginx y base—.
- **Dos fallos antes de alertar.** Actualizar el sistema reemplaza los
  contenedores y deja unos segundos sin respuesta; con alerta al primer
  fallo, cada despliegue manda una alarma y en un mes nadie las mira.

Hay dos caminos, y el sistema no depende de ninguno en particular porque lo
único que se necesita es que alguien consulte una URL:

- **Un servicio contratado** (varios tienen plan gratuito suficiente para
  esto). No se instala nada: se crea una cuenta, se carga la URL y listo. Los
  chequeos salen de los servidores de ese servicio, que ya están fuera de la
  institución.
- **Autogestionado**, con alguna herramienta de código abierto de monitoreo.
  Necesita **una máquina cualquiera que esté prendida y fuera de la red de la
  escuela** —una computadora en una casa, una placa de bajo consumo, un
  servidor virtual barato—. Ahí corre **solo el monitor**: unos pocos MB, sin
  base de datos ni nada del SGRC. Lo que no sirve es instalarlo en el mismo
  servidor que la aplicación.

### Entender por qué algo anda mal

Las dos secciones anteriores sirven para **enterarse** de que pasó algo. Para
investigarlo hay una tercera pieza, opcional y también apagada por defecto:
tableros de Prometheus y Grafana, en
[`12-observabilidad.md`](12-observabilidad.md). Contestan qué ruta está
lenta, qué se está rompiendo y desde cuándo.

No reemplaza a las otras dos: corre en el mismo servidor, así que se apaga
junto con lo que habría que vigilar.

---

| Síntoma | Causa habitual |
|---|---|
| `FRONTEND_ORIGIN está vacío` y el proceso no arranca | Falta completar esa variable en el `.env` (§1.1) |
| `JWT_SECRET tiene N bytes: hacen falta al menos 32` | El secreto quedó con el valor de ejemplo |
| `health: postgres no responde` repetido | Postgres no levantó; mirá sus logs |
| `sgrc-app` en `unhealthy` | La API no llega a la base — casi siempre credenciales mal en el `.env` |
| El navegador no carga nada | Revisá que el ingress del túnel apunte a `http://frontend:80` |
| `configuración de correo inválida` y el proceso no arranca | El bloque `SMTP_*` quedó a medias (§1.1.b) |
| No aparece "Olvidé mi contraseña" en el login | No hay correo configurado. El log lo dice al arrancar: `correo saliente deshabilitado` |

### Cuando no llegan los mails

El envío es de **mejor esfuerzo**: si falla, se loguea y el sistema sigue. Los
avisos internos (la campana) llegan igual, así que no se pierde nada — pero
conviene mirar el log:

```bash
docker compose logs sgrc-app | grep -i "correo\|email"
```

| Lo que dice el log | Qué significa |
|---|---|
| `email: SMTP no configurado, no se envía ...` | Falta el bloque `SMTP_*` (§1.1.b) |
| `autenticando contra smtp.gmail.com: ... Username and Password not accepted` | La contraseña de aplicación está mal, o se usó la contraseña normal de Gmail |
| `conectando a smtp.gmail.com:587: ... timeout` | El servidor no tiene salida al puerto 587 |
| `el servidor ... no ofrece STARTTLS` | `SMTP_PORT` mal: tiene que ser 587, no 465 |
| `destinatario rechazado` | La dirección de esa persona no existe. Corregila en su cuenta |
| Nada en el log y tampoco llega | Revisá spam. Si es la primera vez, marcá el remitente como conocido |

---

## 5. El esquema de la base

`migrations/001_esquema_inicial.sql` es el esquema completo del sistema. **Lo
aplica sgrc-app al arrancar**, con [goose](https://github.com/pressly/goose):
mira qué migraciones registró la tabla `goose_db_version`, aplica las que
falten y sigue. Arrancar mil veces no cambia nada; arrancar contra una base
vacía la deja lista; arrancar contra una base vieja la pone al día.

En el log se ve así:

```
conectado a sgrc_db
goose: successfully migrated database to version: 1     ← la primera vez
goose: no migrations to run. current version: 1         ← las siguientes
```

> **Si el sistema arranca bien pero devuelve 500 en la primera pantalla que
> toca una columna que no está**, el esquema quedó atrasado: `make
> migrate-status` es la primera pregunta a hacer.

Es un archivo único y no una cadena de parches incrementales. La razón es
para quién está escrito: alguien que adopta el proyecto necesita entender qué
tablas hay y por qué son así, y eso se lee de corrido en un archivo — no
reconstruyéndolo mentalmente a partir de veinte migraciones sucesivas, la
mitad de las cuales renombran lo que hizo la otra mitad. La historia de cómo
se llegó a este esquema está en el historial de git, que es donde corresponde.

### Mirar y forzar el estado

```bash
make migrate-status   # qué está aplicado y qué falta
make migrate          # aplica lo pendiente sin esperar al próximo arranque
```

Los ejecuta el propio binario adentro del contenedor: la imagen es `scratch`,
no tiene psql ni shell, y `sgrc-app` sí conoce las variables de conexión.

No hay comando para **revertir**. Deshacer el esquema inicial borra las tablas
y con ellas los datos; existe en el archivo de migración porque tiene que
existir, pero no se llega ahí por accidente — el mismo criterio con el que
`docker compose down -v` tampoco tiene atajo.

### Agregar una migración

Un archivo nuevo en `migrations/`, numerado después del último
(`002_lo_que_sea.sql`), con las dos anotaciones de goose: la de subida
—`Up`— antes de los cambios y la de bajada —`Down`— antes de cómo se
deshacen. Dos cosas que muerden:

- **Las anotaciones no se pueden nombrar en un comentario del mismo archivo.**
  goose lee todas las líneas que llevan su marca, así que explicarlas
  escribiéndolas de nuevo rompe el archivo con un error de anotación
  duplicada.
- **El esquema viaja compilado adentro del binario** (`go:embed`, ver
  `migrations/embed.go`), porque la imagen final no tiene sistema de archivos
  donde ponerlo. Por eso un archivo nuevo exige **reconstruir**: `make
  run-prod` sí, `make restart` no.

### Una base anterior a goose

Una instalación donde el esquema se aplicó a mano —como se hacía antes— tiene
las tablas pero no `goose_db_version`. Al arrancar, goose intenta crear lo que
ya existe y el contenedor muere avisando. Hay que decirle que esa migración ya
está aplicada:

```bash
make psql SQL="CREATE TABLE IF NOT EXISTS goose_db_version (id SERIAL PRIMARY KEY, version_id BIGINT NOT NULL, is_applied BOOLEAN NOT NULL, tstamp TIMESTAMP NOT NULL DEFAULT now()); INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, true), (1, true);"
```

Antes de correrlo, **comprobá que el esquema realmente esté completo**
(`make psql SQL="\dt"` y comparalo con el archivo de migración): marcar como
aplicada una migración que no corrió entera deja el sistema en un estado que
ninguna herramienta puede reparar sola. Ante la duda, y si no hay datos que
perder, es más barato `docker compose down -v` y arrancar limpio.

### El barrido de entregas y devoluciones

El barrido corre dentro del mismo proceso, cada cinco minutos, sin cron ni
nada que instalar. Cinco variables opcionales lo ajustan:
`RETIRO_AVISO_MINUTOS` (15), `RETIRO_GRACIA_MINUTOS` (40),
`RETIRO_PARCIAL_GRACIA_MINUTOS` (15), `DEVOLUCION_DEMORA_MINUTOS` (10) y
`CIERRE_JORNADA` (18). Un valor mal escrito **impide levantar**, a propósito:
descubrirlo tres horas después porque un aviso no salió es peor.

Las tres primeras se leen juntas, porque son tres momentos de la misma clase y
dos de ellas valen 15 por defecto sin ser lo mismo:

| Variable | Desde cuándo cuenta | Qué hace |
|---|---|---|
| `RETIRO_AVISO_MINUTOS` (15) | el inicio de la clase | **Avisa** al docente que todavía no las retiró y que a los 40 quedan libres. Es el único aviso de esta etapa (RF-08.20) |
| `RETIRO_GRACIA_MINUTOS` (40) | el inicio de la clase | **Libera**, en silencio, si no se retiró ninguna |
| `RETIRO_PARCIAL_GRACIA_MINUTOS` (15) | la última **entrega** | **Libera**, en silencio, lo que el docente dejó cuando vino a buscar una parte |

Las dos de liberación no avisan nada: el correo ya salió a los 15 minutos del
inicio, cuando el docente todavía podía ir, cambiar la máquina o cancelar.
Mandar otro al liberar sería un segundo mensaje por la misma clase para contar
un hecho consumado. Si se sube `RETIRO_AVISO_MINUTOS` por encima de
`RETIRO_GRACIA_MINUTOS`, el aviso pierde su razón de ser —llegaría con la
reserva ya liberada—, así que esa combinación se rechaza al arrancar.

#### `CIERRE_JORNADA`: el corte de lo que quedó afuera

Es una **hora de reloj** (0-23, en la zona de `APP_TIMEZONE`) y **no tiene
ninguna relación con la jornada institucional** que se declara desde la
pantalla de Admin: son dos cosas con nombres parecidos que no se hablan. Lo
único que hace el barrido es preguntar, cada cinco minutos, si la hora actual
llegó a ese número; si llegó y hay equipos afuera que todavía no se avisaron
hoy, sale un aviso a todos los Admin con la lista y con a quién le va a faltar
esa máquina en su próxima reserva.

Cada préstamo queda marcado con la fecha del aviso, así que el corte sale **una
vez por día y por equipo**, y se repite al día siguiente si la máquina sigue
afuera.

De ese diseño salen dos consecuencias que conviene conocer antes de elegir el
número:

- **Pasada esa hora, el corte es casi inmediato.** Un equipo entregado a las 19
  en una instalación con `CIERRE_JORNADA=18` aparece en el aviso de la barrida
  siguiente, cinco minutos después: no espera a ningún cierre.
- **Entre la medianoche y esa hora no hay corte**, porque la comparación es
  contra la hora del reloj. En una escuela nocturna que cierra a la 01:00,
  dejar 18 no sirve —el aviso saldría antes de que la escuela abra, y todo lo
  que se entregue durante la noche se avisaría a los cinco minutos—. Lo más
  cercano es poner la hora real de cierre del laboratorio (23, por ejemplo),
  aceptando que lo entregado después se avisa enseguida. **Un turno que cruza
  la medianoche no se puede expresar con una sola hora**: es un límite conocido
  del diseño actual, el mismo que la jornada institucional sí resuelve
  permitiendo que la hora de cierre sea menor que la de apertura.

**Ningún aviso depende de que el barrido corra a una hora exacta.** Cada uno
deja su marca en la fila, así que reiniciar el contenedor o estar caído dos
horas cambia *cuándo* sale, nunca *cuántas veces*. En el log se ve una línea
por barrida que hizo algo:

```
barrido: 2 recordatorios, 3 avisos de no retiro, 1 reservas liberadas, 0 avisos de equipo faltante, ...
```

Si al entregar aparece "ese equipo ya figura entregado y todavía no
volvió", no es un error del sistema: es el índice único haciendo su trabajo.
La máquina figura afuera, y lo que corresponde es recibirla primero — o
averiguar quién la tiene, que la pantalla de entregas lo dice.

### El aviso de licencias de software

El aviso corre dentro del mismo proceso, sin cron ni nada que instalar. Sale
a partir de `LICENCIAS_HORA_AVISO` (0-23, hora local de la institución; por defecto
`7`). Es un "no antes de", no un horario exacto: el barrido pasa cada hora y
la primera pasada después de esa hora manda el mail; las siguientes no
encuentran nada porque cada licencia queda marcada con la fecha de
vencimiento para la que ya avisó.

**Reiniciar el contenedor no duplica avisos**, y tampoco hace falta que el
servidor esté prendido justo el día del vencimiento: el barrido busca "ya
entró en su ventana de aviso", no "vence exactamente hoy", así que un día
apagado hace que el aviso salga tarde en vez de no salir nunca.

Si los mails no llegan pero la campana sí, el problema es SMTP, no las
licencias: son dos suscriptores independientes del mismo evento. En el log
del contenedor se ve `job de aviso de licencias: N licencias por vencer o
vencidas` cada vez que hay algo, y el fallo de envío queda como
`notification: error notificando por mail las licencias por vencer`.

---

## 6. Copia de seguridad

La base es lo único que no se puede reconstruir: el código y las imágenes se
vuelven a compilar, los datos no.

**Hacer un backup:**

```bash
make backup                    # deja backup-sgrc-AAAA-MM-DD.sql en la carpeta
```

o el comando completo:

```bash
docker compose exec -T postgres sh -c \
  'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > backup-sgrc-$(date +%F).sql
```

**Restaurar** (sobre una base vacía; borra lo que haya):

```bash
docker compose exec -T postgres sh -c \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < backup-sgrc-2026-08-03.sql
```

Conviene sacar un backup **antes de actualizar** y **antes de aplicar una
migración**. Un archivo por semana guardado fuera del servidor cubre el caso
que importa: que el servidor deje de arrancar.

### Mirar la base cuando algo se pone raro

```bash
make psql                                  # consola interactiva
make psql SQL="SELECT count(*) FROM equipo;"   # una consulta y listo
```

Sirve para responder preguntas que la interfaz no contesta. La más útil, si
alguna vez sospechás que la base se recreó:

```bash
make psql SQL="SELECT (SELECT count(*) FROM equipo) AS equipos, (SELECT count(*) FROM usuario) AS usuarios;"
```

Si `equipos` da **cero**, la base arrancó de nuevo y hay que restaurar el
backup: el sistema no inventa inventario, así que un cero ahí solo puede
significar que se perdió lo que había.

### Tres tablas que crecen y nunca se limpian

`notificacion`, `audit_log` y `codigo_recuperacion` **no se borran nunca**, a
propósito. Cada aviso, cada acción sensible y cada pedido de
recuperación de contraseña queda para siempre, y el backup los arrastra.

No hay ninguna purga automática, y es deliberado: la auditoría existe
justamente para poder reconstruir qué pasó cuando alguien reclama algo de
hace meses, y un proceso que borra sin que nadie mire es peor que una tabla
grande. A la escala de una institución educativa esto crece muy despacio: son filas de
texto corto, no archivos.

Para ver cuánto pesan:

```bash
make psql SQL="SELECT relname AS tabla, n_live_tup AS filas, pg_size_pretty(pg_total_relation_size(relid)) AS tamanio FROM pg_stat_user_tables ORDER BY pg_total_relation_size(relid) DESC LIMIT 10;"
```

**Cuándo preocuparse:** cuando el backup empiece a tardar o a molestar por
tamaño. Hasta entonces no hay nada que hacer. Si llega ese momento, lo
razonable es archivar las filas viejas —volcarlas a un `.sql` aparte y
guardarlo fuera del servidor— antes de borrar nada, y decidir el corte con
un criterio explícito (por ejemplo, notificaciones leídas de más de dos
ciclos lectivos atrás). No lo hagas a ojo sobre producción.

---

## 7. La base en desarrollo y en producción

Son **la misma base con distinto contenido**, no dos esquemas distintos. Lo
que cambia es qué datos hay adentro:

|  | Producción | Desarrollo (`make run`) |
|---|---|---|
| Tablas | Las crea Postgres al arrancar con el volumen vacío, aplicando el esquema de `migrations/` | Igual |
| Primer Admin | Lo siembra la app (`SEED_ADMIN_*` del `.env`) | Igual |
| Datos | **Ninguno**: ni ciclo, ni carros, ni equipos. El Admin arma todo desde la interfaz | Un ciclo, un curso, una materia, un docente aprobado y un carro con equipos |

El sistema no inventa datos en producción a propósito: un carro llamado
"Carro 1" que nadie cargó es peor que la pantalla vacía, porque parece real.

### En desarrollo se siembra solo

`docker-compose.dev.yml` agrega un servicio `seed-datos` que corre después de
que la API queda `healthy` y carga los datos de prueba. Con

```bash
docker compose down -v && make run
```

tenés una base recién creada y usable, sin pasos extra.

Tres cosas de ese servicio:

- **Vive solo en el overlay de desarrollo.** En producción no existe:
  `make run-prod` no lo levanta.
- **Le pega a la API como cualquier cliente**, no escribe en la base por
  debajo. Todo lo que crea pasa por las mismas validaciones que usaría una
  persona, así que no puede dejar datos que el sistema no podría haber
  producido.
- **Es idempotente**: reutiliza lo que ya exista con el mismo nombre. Correr
  `make run` diez veces deja un curso, una materia y un carro, no diez.

El script se puede correr a mano igual, con el sistema levantado:

```bash
make seed-datos
```

Crea un docente con una contraseña conocida (`docente@escuela.edu.ar`), así
que **se niega a correr contra algo que no sea local**. Para forzarlo hay que
exportar `SEMBRAR_IGUAL=1`, que es lo bastante incómodo como para que no
pase por accidente.

### Empezar de cero

```bash
docker compose down -v   # borra el volumen: se pierde TODO
make run                 # tablas nuevas + Admin + datos de prueba
```

En producción ese `-v` no se usa nunca sin un backup (§6).

### Dónde se abre en local

`make run` publica dos cosas al host:

| URL | Qué es | Para qué sirve |
|---|---|---|
| `http://localhost:8081` | La SPA compilada servida por **el mismo nginx que corre en el servidor** | Probar lo que realmente se despliega: el build de producción y el ruteo same-origin |
| `http://localhost:8080` | La API directa | Pegarle con `curl`, mirar respuestas crudas |

Para **desarrollar** el frontend conviene Vite (`cd frontend && npm run dev`,
en `:5173`), que recarga al guardar y proxea `/api` a `localhost:8080`. Pero
antes de desplegar hay que mirar el `:8081`: Vite no usa el `nginx.conf` ni el
build de producción, que son justamente las piezas que fallan en un primer
deploy.

---

## 8. Cuentas de Admin

- El primer Admin lo siembra el arranque, de forma idempotente: si ya hay un
  Admin **en estado `APROBADA`**, no hace nada.
- Los demás se crean desde la interfaz, en `/admin/usuarios` → "Crear otro
  Admin". Quedan aprobados de entrada.
- **Si te quedás sin ningún Admin activo** (por ejemplo, se dio de baja al
  único, o se restauró un backup viejo), el arranque lo detecta y vuelve a
  dejar lista la cuenta del `SEED_ADMIN_EMAIL` con la contraseña que tenga el
  `.env` en ese momento. Alcanza con reiniciar:

  ```bash
  make restart
  ```

  Queda registrado en el log y la cuenta arranca pidiendo cambiar la
  contraseña.

---

## 9. Pasar a producción

No hay dos versiones del proyecto ni una rama distinta: **es el mismo código
con otro `.env` y sin el overlay de desarrollo**. Lo único que cambia entre
tu máquina y el servidor es eso.

### 9.1 Qué cambia, exactamente

| | Desarrollo | Producción |
|---|---|---|
| Comando | `make dev` / `make run` | `make run-prod` |
| Overlay | `docker-compose.dev.yml` | Ninguno |
| Puertos al host | 8081 (SPA), 8080 (API), 5432 (base) | **Ninguno.** Se entra solo por el túnel |
| Datos de prueba | Los carga `seed-datos` | No existe ese servicio |
| Entrada | `http://localhost:8081` | El dominio, vía Cloudflare Tunnel |

Que en producción no se publique ningún puerto es a propósito: publicar el
5432 abriría la base a toda la LAN de la institución, y publicar el 80 del
frontend daría un camino que se saltea el túnel — y con él, la posibilidad de
falsificar el header con la IP del cliente.

### 9.2 Lista de control antes de desplegar

1. **`.env` propio del servidor**, con los cuatro valores reales (§1.1):
   `POSTGRES_PASSWORD`, `JWT_SECRET`, `SEED_ADMIN_PASSWORD`, `TUNNEL_TOKEN`.
   Los de `.env.example` dicen `cambiar_...` y el backend **se niega a
   arrancar** con un `JWT_SECRET` de menos de 32 bytes.
2. **`FRONTEND_ORIGIN`** con el dominio real (§9.3).
3. **`VITE_API_URL` vacío.** Se parece a `FRONTEND_ORIGIN` pero no se comporta
   igual: vacío es lo correcto, porque el navegador pide `/api/...` al mismo
   host que le sirvió la página.
4. **El ingress del túnel apunta a `http://frontend:80`** — se configura en el
   panel de Cloudflare, no en el repo.
5. `make run-prod`, y comprobar que `sgrc-app` queda `healthy` (§1.3).
6. Entrar con el Admin sembrado y **cambiarle la contraseña**.

### 9.3 Cuando tengas el dominio comprado

**Se toca una sola línea, en un solo archivo.** El dominio no está escrito en
ningún otro lado del proyecto:

```bash
# .env  (en el servidor — el .env.example es solo la plantilla)
FRONTEND_ORIGIN=https://sgrc.tuescuela.edu.ar
```

Con esquema (`https://`) y **sin barra final**. El backend valida las dos
cosas al arrancar y, si están mal, no arranca y lo dice en el log. Es a
propósito: con este valor mal puesto el navegador rechazaría todos los pedidos
por CORS sin ninguna explicación visible, y el síntoma sería "el sistema no
anda" sin más pistas.

Después de tocarlo:

```bash
make run-prod          # el .env se lee al arrancar el contenedor
```

Lo que **no** hay que tocar:

- **`frontend/nginx.conf`** — usa `server_name _`, que acepta cualquier
  hostname a propósito: quien decide el dominio es el túnel.
- **`docker-compose.yml`** — `cloudflared` se configura con el
  `TUNNEL_TOKEN`; el dominio y el ingress viven en el panel de Cloudflare.
- **El código del frontend** — nunca arma URLs absolutas: todas las llamadas
  salen como `/api/...` contra el mismo origen.

Si algún día la SPA se sirviera desde **otro** host que la API, ahí sí habría
que setear `VITE_API_URL` y recompilar el frontend — es una variable de
compilación, no de runtime, así que un `restart` no alcanza.

### 9.4 No hay ningún `localhost` que reemplazar por el dominio

Es la confusión más natural al desplegar por primera vez, y hacerla rompe el
sistema de una forma difícil de diagnosticar. Vale la pena entender por qué.

En el proyecto entero, `localhost` aparece en **dos lugares funcionales**, y
ninguno de los dos corre en producción:

| Archivo | Qué es | Cuándo corre |
|---|---|---|
| `frontend/vite.config.ts` | El proxy del servidor de desarrollo de Vite | Solo con `npm run dev`, en la máquina de quien programa |
| `frontend/playwright.config.ts` | La URL base de los tests de punta a punta | Solo al correr los tests (y ya se pisa con `E2E_BASE_URL`) |

En producción no hay Vite: la interfaz es un build estático que sirve nginx.

Lo que sí hay son **nombres de contenedor**, que se parecen a direcciones
pero no lo son:

```
navegador → tudominio.com → Cloudflare → túnel → frontend:80  (nginx)
                                                     └── /api → sgrc-app:8080
```

`frontend` y `sgrc-app` son los nombres de los servicios en
`docker-compose.yml`. Docker les da una IP en la red interna `sgrc-net` y los
resuelve por su DNS propio — por eso `frontend/nginx.conf` declara
`resolver 127.0.0.11`, que es ese DNS. Son direcciones privadas que solo
existen adentro del servidor.

> **Si reemplazaras `sgrc-app:8080` por tu dominio en el `proxy_pass` de
> `nginx.conf`, armarías un bucle**: nginx mandaría cada pedido de `/api` de
> vuelta a internet, daría toda la vuelta por Cloudflare y volvería a entrar
> por el mismo túnel, a sí mismo. El síntoma es un timeout o un 502, y la
> causa no se ve en ningún log.

El dominio vive en **un solo lugar del repo**: `FRONTEND_ORIGIN`, en el
`.env` (§9.3). Y en un solo lugar fuera del repo: el ingress del túnel, en el
panel de Cloudflare.

---

## 10. Cambiar los puertos

Nada de esto hace falta para un despliegue normal: con Cloudflare Tunnel los
puertos son internos y nadie los ve. Esta sección es para quien necesite
convivir con otro servicio que ya ocupa el 8080, o exponer el sistema de otra
manera.

### 10.1 El puerto del backend

El puerto sale de `APP_PORT` en el `.env` (`cmd/main.go`, con 8080 por
defecto), **pero cambiarlo ahí solo no alcanza**:

| Archivo | Dónde | Qué dice | ¿Hay que tocarlo? |
|---|---|---|---|
| `.env` | `APP_PORT` | El puerto en el que escucha Go | **Sí** |
| `frontend/nginx.conf` | Dos veces: en `location /api/` y en `location = /health` | `set $sgrc_app http://sgrc-app:8080;` | **Sí — es la parte que se olvida** |
| `Dockerfile` | `EXPOSE 8080` | Documenta el puerto de la imagen | Por prolijidad; en una red de Docker no restringe nada |
| `docker-compose.dev.yml` | `"8080:8080"` | Publica la API al host | Solo si le pegás con `curl` desde tu máquina |
| `frontend/vite.config.ts` | `target: "http://localhost:8080"` | El proxy de desarrollo | Solo si usás `npm run dev` |

> **La trampa.** Si cambiás `APP_PORT` y no tocás `nginx.conf`, Go escucha en
> el puerto nuevo y nginx sigue buscándolo en el 8080. Todo `/api` devuelve
> **502**, la interfaz carga pero ningún dato aparece — y el contenedor de la
> API figura **`healthy`**, porque el autochequeo sí acompaña a `APP_PORT`
> (`cmd/main.go` le pasa el puerto configurado). Un sistema "sano" que no
> funciona es el peor síntoma posible: parece un problema de la base o de la
> red, y no es ninguno de los dos.

Después de cambiarlo hay que **reconstruir el frontend**, no solo reiniciarlo
— el `nginx.conf` se copia adentro de la imagen:

```bash
make rebuild SERVICIO=frontend
make rebuild SERVICIO=sgrc-app
```

### 10.2 El puerto del frontend

| Archivo | Dónde | Qué dice |
|---|---|---|
| `frontend/nginx.conf` | Primera línea del bloque `server` | `listen 80;` |
| `frontend/Dockerfile` | `EXPOSE 80` | Documenta el puerto de la imagen |
| `docker-compose.dev.yml` | `"8081:80"` | Publica la interfaz al host, solo en desarrollo |
| Panel de Cloudflare | Ingress del túnel | `http://frontend:80` |

Si movés el `listen`, **el ingress del túnel tiene que apuntar al puerto
nuevo**, o el túnel entrega a una puerta cerrada.

Cambiar el `"8081:80"` de desarrollo es inofensivo: es solo por qué puerta
entrás desde tu propia máquina. El número de la izquierda es el del host, el
de la derecha el del contenedor; tocá siempre el de la izquierda.

---

## 11. Salir a internet sin Cloudflare Tunnel

El proyecto está armado alrededor del túnel, y es la forma recomendada: no
expone ningún puerto del servidor, resuelve el certificado y protege el
origen. Pero se puede desplegar de otra manera —abriendo un puerto en el
router, o detrás de un proxy inverso que ya exista en la institución— y estos
son los tres puntos con los que hay que lidiar.

### 11.1 Publicar el puerto

`docker-compose.yml` **no publica ningún puerto al host a propósito**. Con el
túnel eso es una ventaja; sin él significa que no hay nada a donde apuntar el
router. Hace falta agregarle al servicio `frontend`:

```yaml
frontend:
  ports:
    - "80:80"   # el de la izquierda es el puerto del servidor
```

Conviene ponerlo en un archivo aparte (por ejemplo `docker-compose.prod.yml`)
en vez de editar el compose base, para no perder el cambio al actualizar y
para que quede explícito que ese despliegue expone puertos.

Y el servicio `cloudflared` deja de tener sentido: sin `TUNNEL_TOKEN` el
contenedor arranca, falla y queda en `Exited (255)`. No molesta —el compose
no define política de reinicio, así que no lo reintenta— pero conviene
sacarlo para que `docker compose ps` no muestre siempre un servicio caído y
nadie pierda tiempo investigándolo.

### 11.2 El certificado

nginx escucha en **HTTP plano**: el HTTPS lo ponía Cloudflare. Sin el túnel
hay que resolverlo por cuenta propia — lo habitual es Let's Encrypt con
`certbot`, o un proxy inverso adelante (Caddy, Traefik, nginx del sistema)
que termine el TLS y reenvíe al contenedor.

No es opcional. El sistema maneja contraseñas y sesiones: por HTTP viajan en
claro por toda la red de la institución.

### 11.3 Que `FRONTEND_ORIGIN` coincida con la realidad

Es donde falla este camino. `FRONTEND_ORIGIN` tiene que ser **exactamente**
el origen desde el que el navegador carga la página: mismo esquema, mismo
host, mismo puerto si no es el estándar.

| Cómo entra la gente | Qué va en `FRONTEND_ORIGIN` |
|---|---|
| `https://sgrc.tuescuela.edu.ar` | `https://sgrc.tuescuela.edu.ar` |
| `http://192.168.1.50:8081` (red interna, sin certificado) | `http://192.168.1.50:8081` |

Si dice `https://` y la gente entra por `http://`, el navegador rechaza todos
los pedidos por CORS y el sistema no anda, sin ningún error visible en el
servidor. El backend valida el formato al arrancar (§9.3) pero no puede
adivinar por dónde entra la gente: esa coherencia la ponés vos.
