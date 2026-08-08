# Operación — SGRC

Cómo se pone en marcha el sistema, cómo se para, cómo se reinicia y qué
hacer cuando algo no anda. Todo desde el servidor de la escuela, con Docker
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
local**: en el servidor de la escuela ese usuario no existe.

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

### 1.1.b Configurar el correo (opcional, pero conviene)

Sin esto el sistema funciona igual: los avisos siguen llegando a la campana de
notificaciones. Lo que **no** funciona es el "olvidé mi contraseña" — el
enlace ni aparece en la pantalla de ingreso, y hay que resetear a mano desde
`/admin/usuarios` cada vez que un docente se olvide la suya.

Se usa el Gmail de la escuela, con una **contraseña de aplicación**, que no es
la contraseña con la que se entra a Gmail:

1. En la cuenta de la escuela, activá la verificación en dos pasos:
   <https://myaccount.google.com/signinoptions/twosv>. Sin eso el paso
   siguiente no existe.
2. Entrá a <https://myaccount.google.com/apppasswords> y creá una. Google la
   muestra como `abcd efgh ijkl mnop`; se puede pegar con los espacios.
3. En el `.env`:

   ```
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USER=la-cuenta-de-la-escuela@gmail.com
   SMTP_PASSWORD=abcd efgh ijkl mnop
   SMTP_FROM=la-cuenta-de-la-escuela@gmail.com
   SMTP_FROM_NAME=Nombre de la escuela
   ```

4. `make restart` y mirá el log: tiene que decir
   `correo saliente habilitado vía smtp.gmail.com`.

Es gratis y el límite es de ~500 destinatarios por día, de sobra. Lo que
cambió en 2022 es que Gmail exige contraseña de aplicación, no que el SMTP
haya dejado de ser gratuito.

> **Ojo con dejarlo a medias.** Con `SMTP_HOST` puesto pero sin `SMTP_FROM`/
> `SMTP_USER` o sin `SMTP_PASSWORD`, el proceso **no arranca** y dice qué
> falta. Es deliberado: si arrancara, cada envío fallaría en silencio dentro
> de una goroutine y el único síntoma sería "no me llegan los mails".

Para probarlo de punta a punta sin tocar ninguna cuenta real: entrá a
`/recuperar-password`, poné tu propio email y fijate si llega el código
(revisá spam la primera vez).

### 1.2 Levantar todo

```bash
make run-prod          # docker compose up --build
```

La primera vez tarda unos minutos: compila el binario Go, compila la SPA y
crea la base aplicando todo lo de `migrations/`.

Cuando termina, el primer Admin ya está creado con el `SEED_ADMIN_EMAIL` y el
`SEED_ADMIN_PASSWORD` del `.env`. Entrá con esa cuenta y **cambiale la
contraseña**.

### 1.3 Comprobar que quedó sano

```bash
docker compose ps
```

`sgrc-app` tiene que figurar **`healthy`**. No es un adorno: ese estado sale
de que el proceso consultó Postgres y respondió. Si dice `unhealthy` o
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

Si la actualización trae **migraciones nuevas** (archivos nuevos en
`migrations/`), hay que aplicarlas a mano: ver §5.

---

## 4. Ver qué está pasando

```bash
make logs                          # todo, en vivo
docker compose logs -f sgrc-app    # solo la API
docker compose logs --tail=100 postgres
```

Qué buscar en el arranque de `sgrc-app`:

```
zona horaria de la escuela: America/Argentina/Buenos_Aires (ahora: ...)
conectado a sgrc_db
admin inicial: cuenta ... lista            ← solo si hizo falta sembrarlo
correo saliente habilitado vía smtp.gmail.com
```

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

## 5. Migraciones

Los archivos de `migrations/` se aplican **solos la primera vez**, cuando la
base se crea vacía. Sobre una base que ya existe **no corren solas**: hay que
aplicarlas a mano, en orden, cuando llega una nueva.

```bash
docker compose exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < migrations/005_dia_semana_lectivo.sql
```

o, más corto:

```bash
make migrate ARCHIVO=migrations/005_dia_semana_lectivo.sql
```

Cada migración es transaccional: o se aplica entera o no se aplica nada. Las
que endurecen una regla revisan primero si hay datos que quedarían afuera y
**abortan diciendo exactamente qué filas son**, sin tocar nada — la decisión
de qué hacer con esos datos es de quien administra, no de la migración.

Para ver cuáles se aplicaron hay que mirar la base; el proyecto no lleva una
tabla de versiones de esquema (a esta escala, la lista de archivos y el
orden alcanzan).

### Entregas y devoluciones

La **013** agrega `prestamo` (RF-08). No toca datos existentes ni puede
abortar: crea una tabla vacía.

La **014** agrega el estado `NO_RETIRADA` y las marcas de los avisos. Tampoco
toca datos: amplía CHECK y agrega columnas nulas.

El barrido corre dentro del mismo proceso, cada cinco minutos, sin cron ni
nada que instalar. Tres variables opcionales lo ajustan:
`RETIRO_GRACIA_MINUTOS` (40), `DEVOLUCION_DEMORA_MINUTOS` (10) y
`CIERRE_JORNADA` (18). Un valor mal escrito **impide levantar**, a propósito:
descubrirlo tres horas después porque un aviso no salió es peor.

**Ningún aviso depende de que el barrido corra a una hora exacta.** Cada uno
deja su marca en la fila, así que reiniciar el contenedor o estar caído dos
horas cambia *cuándo* sale, nunca *cuántas veces*. En el log se ve una línea
por barrida que hizo algo:

```
barrido: 2 recordatorios, 1 reservas liberadas, 0 avisos de equipo faltante, ...
```

Si al entregar aparece "ese equipo ya figura entregado y todavía no
volvió", no es un error del sistema: es el índice único haciendo su trabajo.
La máquina figura afuera, y lo que corresponde es recibirla primero — o
averiguar quién la tiene, que la pantalla de entregas lo dice.

### Aviso de licencias de software

La **012** agrega `licencia_software` (RF-03.11 a RF-03.14). No toca datos
existentes ni puede abortar: crea una tabla vacía y amplía el `CHECK` de
`notificacion.tipo`.

El aviso corre dentro del mismo proceso, sin cron ni nada que instalar. Sale
a partir de `LICENCIAS_HORA_AVISO` (0-23, hora de la escuela; por defecto
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
docker compose exec -T postgres \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > backup-sgrc-$(date +%F).sql
```

**Restaurar** (sobre una base vacía; borra lo que haya):

```bash
docker compose exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" < backup-sgrc-2026-08-03.sql
```

Conviene sacar un backup **antes de actualizar** y **antes de aplicar una
migración**. Un archivo por semana guardado fuera del servidor cubre el caso
que importa: que el servidor deje de arrancar.

---

## 7. La base en desarrollo y en producción

Son **la misma base con distinto contenido**, no dos esquemas distintos. Lo
que cambia es qué datos hay adentro:

|  | Producción (servidor de la escuela) | Desarrollo (`make run`) |
|---|---|---|
| Tablas | Las crea Postgres al arrancar con el volumen vacío, aplicando `migrations/` | Igual |
| Primer Admin | Lo siembra la app (`SEED_ADMIN_*` del `.env`) | Igual |
| Datos | **Ninguno**: ni ciclo, ni carros, ni equipos. El Admin arma todo desde la interfaz | Ciclo, curso, materia, un docente aprobado y un carro con 8 PCs |

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

- **Vive solo en el overlay de desarrollo.** En el servidor no existe:
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

En el servidor de la escuela ese `-v` no se usa nunca sin un backup (§6).

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
tu máquina y el servidor de la escuela es eso.

### 9.1 Qué cambia, exactamente

| | Desarrollo | Producción |
|---|---|---|
| Comando | `make dev` / `make run` | `make run-prod` |
| Overlay | `docker-compose.dev.yml` | Ninguno |
| Puertos al host | 8081 (SPA), 8080 (API), 5432 (base) | **Ninguno.** Se entra solo por el túnel |
| Datos de prueba | Los carga `seed-datos` | No existe ese servicio |
| Entrada | `http://localhost:8081` | El dominio, vía Cloudflare Tunnel |

Que en producción no se publique ningún puerto es a propósito: publicar el
5432 abriría la base a toda la LAN de la escuela, y publicar el 80 del
frontend daría un camino que se saltea el túnel — y con él, la posibilidad de
falsificar el header con la IP del cliente.

### 9.2 Lista de control antes de desplegar

1. **`.env` propio del servidor**, con los cuatro valores reales (§1.1):
   `POSTGRES_PASSWORD`, `JWT_SECRET`, `SEED_ADMIN_PASSWORD`, `TUNNEL_TOKEN`.
   Los de `.env.example` dicen `cambiar_...` y el backend **se niega a
   arrancar** con un `JWT_SECRET` de menos de 32 bytes.
2. **`FRONTEND_ORIGIN`** con el dominio real (§9.3).
3. **`APP_ENV=production`**.
4. **`VITE_API_URL` vacío.** Se parece a `FRONTEND_ORIGIN` pero no se comporta
   igual: vacío es lo correcto, porque el navegador pide `/api/...` al mismo
   host que le sirvió la página.
5. **El ingress del túnel apunta a `http://frontend:80`** — se configura en el
   panel de Cloudflare, no en el repo.
6. `make run-prod`, y comprobar que `sgrc-app` queda `healthy` (§1.3).
7. Entrar con el Admin sembrado y **cambiarle la contraseña**.

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
