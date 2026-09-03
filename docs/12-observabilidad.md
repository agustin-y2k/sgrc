# Observabilidad — Prometheus, Grafana y Dozzle

Cómo mirar lo que el sistema está haciendo por dentro: qué se pide, qué
tarda, qué se rompe, qué dijo cuando se rompió y si los procesos de fondo
siguen corriendo.

Es **opcional**. El sistema funciona igual sin nada de esto, y quien clone el
repositorio para probarlo no necesita levantarlo.

---

## 1. Tres capas que no se reemplazan entre sí

Es la distinción más importante de este documento, porque las tres se
confunden con "monitoreo" y cubren cosas distintas:

| Capa | Qué contesta | Dónde vive | Si el servidor se apaga |
|---|---|---|---|
| **Monitor externo** (§4 de `11-operacion.md`) | ¿El sitio responde desde afuera? | fuera de la institución | **avisa** |
| **Aviso de vida de los barridos** (ídem) | ¿Los procesos de fondo siguen corriendo? | el sistema avisa, un servicio externo alerta | **avisa** |
| **Esto: Prometheus, Grafana y Dozzle** | ¿Por qué está lento? ¿Qué se rompe? ¿Desde cuándo? ¿Qué dijo el log? | el mismo servidor | **se apaga con él** |

De ahí la regla para no llevarse una decepción: **esto no sirve para
enterarse de que el sistema se cayó** —se cae junto con él—, sirve para
entender qué está pasando cuando algo anda mal, y para ver venir lo que
todavía no se rompió.

---

## 2. Levantarlo

```bash
make observabilidad        # el sistema + Prometheus + Grafana + Dozzle
make observabilidad-stop   # apaga los paneles, el sistema sigue
```

Sin ese comando, los tres contenedores no existen: están detrás de un
*profile* del compose (`docker compose --profile observabilidad up -d` es lo
que hace el atajo). Un `make run-prod` normal no los levanta.

**Lo que cuesta**: unos 400 MB de memoria entre Prometheus y Grafana, más
unas pocas decenas para Dozzle, y hasta 1 GB de disco para el histórico de
métricas. En un servidor compartido con otros usos, es lo primero a apagar si
falta memoria — y si hay que quedarse con uno solo, Dozzle es el barato.

Antes de levantarlo, poné `GRAFANA_PASSWORD` en el `.env`: es la contraseña
del usuario `admin`. Dozzle no pide ninguna variable.

---

## 3. Llegar desde otra máquina: el túnel de SSH

Ni Grafana ni Dozzle salen a la red: los dos se publican en `127.0.0.1` del
servidor, así que desde otra computadora no se llega escribiendo su dirección
IP. Se llega con un túnel de SSH, y con **un solo comando alcanza para los
dos**:

```bash
ssh -N -L 3000:localhost:3000 -L 8888:localhost:8888 usuario@servidor
```

Mientras ese comando siga corriendo, en el navegador **de tu propia máquina**:

| Dirección | Qué es |
|---|---|
| `http://localhost:3000` | Grafana (§4) |
| `http://localhost:8888` | Dozzle, los logs (§5) |

Se corta con `Ctrl+C`, o al cerrar la terminal.

### Qué dice ese comando

`-L 3000:localhost:3000` se lee de izquierda a derecha: *abrí el puerto 3000
en mi computadora y mandá lo que llegue ahí, por adentro de la conexión SSH,
al puerto 3000 de `localhost`*.

La palabra del medio es la clave y es la que más se malinterpreta: ese
`localhost` **se resuelve en el servidor, no en tu máquina**. Por eso el
túnel funciona aunque los paneles estén publicados solo en `127.0.0.1` y no
haya ningún puerto abierto a la red. Desde el punto de vista de Docker, quien
pide la página es el propio servidor.

El `-N` es opcional y conviene: dice "no me abras una shell, solo el túnel",
así esa terminal queda dedicada a esto y no hay riesgo de tipear algo por
error en el servidor.

**El número de la izquierda es tuyo y podés cambiarlo.** Si ya tenés algo
usando el 3000 en tu computadora:

```bash
ssh -N -L 3300:localhost:3000 usuario@servidor    # y entrás por localhost:3300
```

### Para no escribirlo cada vez

En `~/.ssh/config` de tu máquina:

```
Host paneles
    HostName servidor.o.la.ip
    User usuario
    LocalForward 3000 localhost:3000
    LocalForward 8888 localhost:8888
    ExitOnForwardFailure yes
    RequestTTY no
```

Después alcanza con `ssh -N paneles`.

> **Si ya tenés un `Host` para ese servidor, las líneas van adentro de ese
> bloque**, no en uno nuevo. Es el error que más cuesta ver: cuando el bloque
> que ya existe reenvía uno de los puertos, agregar el mismo `-L` en la línea
> de comandos hace que la conexión nueva choque contra la anterior — y falla
> por un puerto que aparentemente no está usando nadie, cuando en realidad lo
> tiene tomado tu propia sesión.

`ExitOnForwardFailure yes` hace que la conexión falle entera si algún reenvío
no se pudo abrir. Sin eso, ssh avisa del que falló pero se conecta igual, y
te quedás con medio túnel: un panel abre y el otro no, sin ninguna razón
visible.

### Desde el celular

Cualquier cliente de SSH con reenvío de puertos sirve (Termius, JuiceSSH y
similares): se configura el mismo par de puertos y se abre
`http://localhost:8888` en el navegador del teléfono. Es la forma práctica de
mirar los logs cuando alguien avisa que algo anda mal y no estás frente a una
computadora.

### Cuando no anda

| Lo que ves | Qué pasa |
|---|---|
| `bind: Address already in use` | El puerto de la izquierda ya está ocupado en **tu** máquina, y casi siempre es **otra sesión de SSH tuya** que ya lo abrió: una terminal que quedó colgada, o un `~/.ssh/config` que ya reenvía ese puerto. `ss -ltnp \| grep 3000` dice qué proceso lo tiene. Si es algo que no conviene cerrar, cambiá el número de la izquierda (`-L 3300:localhost:3000`). |
| `channel N: open failed: connect failed: Connection refused` | El túnel está bien; **el contenedor no está levantado**. Los paneles están detrás del perfil: `make observabilidad` en el servidor. |
| La página no carga y no hay ningún error | Casi siempre es el navegador yendo a `https://`. Es `http://`, sin la ese. |

### Por qué esto y no publicarlos por el túnel de Cloudflare

Se evaluó darles un subdominio con Cloudflare Access adelante y se decidió
que no. Dozzle no tiene usuario ni contraseña propios, así que un error de
política en Access —una regla de prueba que quedó, un bypass mal puesto—
dejaría los logs, con nombres y direcciones de correo de gente real, servidos
en internet. Además obligaría a dejar los paneles prendidos todo el tiempo
para que el hostname no devuelva 502, y no mejoraría lo único que de verdad
importa: siguen viviendo en el mismo servidor, así que se apagan junto con lo
que habría que vigilar (§1).

El túnel de SSH da el mismo acceso remoto, no agrega ninguna superficie
nueva, y se corta solo cuando cerrás la terminal.

---

## 4. Entrar a Grafana

Queda en `http://localhost:3000` **del propio servidor**, publicado solo en
`127.0.0.1` a propósito: es un panel de administración y no tiene por qué
estar a la vista del resto de la red de la institución. Desde otra
computadora se llega con el túnel de SSH de §3. Usuario `admin`, contraseña
la del `.env`.

El tablero **SGRC — funcionamiento** ya viene cargado. No hay que configurar
el origen de datos: se aprovisiona solo al arrancar
(`observabilidad/grafana/`).

> Los cambios hechos desde la interfaz de Grafana **se pierden al
> reiniciar**: la fuente de verdad es el JSON del repositorio. Si un panel
> nuevo vale la pena, se exporta y se commitea. Es a propósito — un tablero
> que solo existe en el disco de un servidor se pierde con el servidor.

---

## 5. Los logs en el navegador (Dozzle)

Queda en `http://localhost:8888` **del propio servidor**, con las mismas
condiciones que Grafana: publicado solo en `127.0.0.1`, y desde otra máquina
por el túnel de SSH de §3.

Muestra los logs de los contenedores en vivo, sin usuario ni contraseña
—cualquiera que llegue al puerto entra—, y por eso el puerto no sale de
`127.0.0.1`: en ese servidor, llegar ahí ya supone tener una sesión de SSH.

**No reemplaza a `docker compose logs -f`**, que sigue siendo el camino corto
para una mirada rápida (§4 de [`11-operacion.md`](11-operacion.md)). Sirve
para lo que en la terminal cuesta: seguir los cuatro servicios a la vez en
columnas, buscar un texto en lo que ya pasó sin volver a pedir el log entero,
o mirarlo desde el celular mientras alguien reporta algo por teléfono.

### Lo que no hace

- **No guarda nada.** Lee lo que Docker tiene en disco para cada contenedor.
  Si el demonio rota los logs, o si un `up --build` recrea el contenedor, lo
  anterior no está y Dozzle tampoco lo tiene. Para conservar historia hay que
  juntarla en otro lado, y eso no está incluido.
- **No toca los contenedores.** Reiniciarlos desde el navegador y abrir una
  terminal adentro son dos funciones que Dozzle trae y que acá quedan
  apagadas (`DOZZLE_ENABLE_ACTIONS` y `DOZZLE_ENABLE_SHELL`). Que sea solo un
  visor es la mitad de la razón por la que se puede dejar prendido.
- **No sale a internet.** Ni analítica anónima ni consultas al registro para
  avisar de versiones nuevas: la versión se sube a mano en el compose.

### Solo muestra los contenedores de este proyecto

El filtro es una etiqueta, `sgrc.logs=si`, que el `docker-compose.yml` le
pone a sus servicios. Un contenedor que no la lleve no aparece: en un
servidor que además corre otras cosas, abrir Dozzle no destapa los logs de
todo lo que haya en la máquina.

El costo de esa decisión es que **un contenedor ajeno al compose tampoco se
ve** — el caso típico es un Cloudflare Tunnel creado desde el panel, fuera de
este proyecto (§2 de [`11-operacion.md`](11-operacion.md)). Para verlo hay
dos caminos: recrearlo con la etiqueta, o sacar la línea `DOZZLE_FILTER` del
compose y aceptar que se vea todo el demonio.

### El socket de Docker

Dozzle lee los logs por el socket del demonio, que le montamos como solo
lectura. Conviene saber exactamente qué significa eso: **el `:ro` protege el
archivo, no la API que hay del otro lado**, y quien habla con ese socket
tiene el poder de root en el host. No es un detalle de Dozzle, es cómo
funciona cualquier visor de este tipo.

Lo que hace que valga la pena igual: el contenedor solo existe cuando se lo
pide, no publica su puerto fuera de `127.0.0.1`, y no tiene habilitada
ninguna acción sobre los contenedores. Si eso no alcanzara, el paso
siguiente es un *socket proxy* de solo lectura delante — una pieza más, que
no se incluyó porque en un servidor de una sola persona no cambia nada.

---

## 6. Qué se mide y qué pregunta contesta

| Métrica | La pregunta detrás |
|---|---|
| `sgrc_peticiones_http_total{metodo,ruta,codigo}` | ¿Cuánto se usa? ¿Qué proporción falla, y en qué pantalla? |
| `sgrc_peticion_http_duracion_segundos{metodo,ruta}` | ¿Está lento de verdad o es una impresión? ¿Qué ruta? |
| `sgrc_barrido_ejecuciones_total{barrido,resultado}` | ¿Los barridos corren? ¿Fallan seguido? |
| `sgrc_barrido_ultimo_exito_timestamp{barrido}` | ¿Hace cuánto que uno no termina bien? |
| `sgrc_pool_conexiones_*` | ¿Alcanzan las conexiones a Postgres, o hay consultas haciendo cola? |
| `go_*`, `process_*` | ¿Se está comiendo la memoria del servidor? ¿Hay una fuga? |

Dos decisiones del diseño que conviene conocer antes de agregar métricas:

- **La etiqueta `ruta` es el patrón, no la URL.** Se cuenta
  `/api/reservation/:id`, no `/api/reservation/3f9d…`. Con la URL cruda, cada
  identificador —y cada dirección inventada que pruebe un escaneo
  automático— crearía su propia serie, que es la forma clásica de que
  Prometheus termine consumiendo toda la memoria del servidor. Las rutas que
  no existen se agrupan como `desconocida`.
- **Las series de los barridos se crean al arrancar**, antes de la primera
  corrida. En Prometheus la ausencia de una métrica no dispara alertas: sin
  esto, una goroutine que muere al arrancar —el caso más grave— sería
  justamente el único que no alertaría.

---

## 7. `/metrics` no se publica a internet

El backend expone las métricas en `/metrics`, y **eso no sale por el túnel**:
nginx solo proxea `/api` y `/health`; cualquier otra ruta cae en la interfaz
(`frontend/nginx.conf`). Pedir `/metrics` desde afuera devuelve la página de
la aplicación, no las métricas.

Quien las consulta es Prometheus, por la red interna de Docker. No hay
ningún puerto publicado para eso.

---

## 8. Alertas: se ven, pero no notifican

`observabilidad/alertas.yml` define cinco reglas —barrido detenido, errores
5xx sostenidos, pool saturado, backend sin responder—. **Se disparan y se ven
en la pantalla de Prometheus, pero no mandan ningún mensaje.** Para eso hace
falta otra pieza: Alertmanager, o las alertas propias de Grafana con un SMTP
configurado.

No se incluye ninguna de las dos a propósito. Lo que hay que saber sí o sí
—el sistema no responde, un barrido dejó de correr— ya avisa por afuera, sin
depender de que este servidor esté sano (§1). Sumar acá un notificador sería
duplicar el aviso y, peor, apoyarlo en la máquina que puede ser justamente la
que falló.

---

## 9. Tableros de uso (reservas por mes, equipo más pedido)

Para eso **no hace falta Prometheus**: esos datos ya están en Postgres, con
tablas históricas que sobreviven al cierre del ciclo lectivo
(`historico_uso_equipo`, `historico_uso_docente`). Se agrega Postgres como
segundo origen de datos de Grafana y se consulta con SQL.

No viene aprovisionado porque necesita un usuario de solo lectura, que hay
que crear una vez. Abrí la consola con `make psql` y pegá esto adentro:

```sql
CREATE ROLE grafana LOGIN PASSWORD 'cambiar_por_una_contrasena_larga';
GRANT CONNECT ON DATABASE sgrc_db TO grafana;
GRANT USAGE ON SCHEMA public TO grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO grafana;
```

> En la consola y no con `make psql SQL="…"`: esa variante pasa la consulta
> entre comillas simples, así que la contraseña —que en SQL va sí o sí entre
> comillas simples— rompe el comando antes de llegar a Postgres.

Después, en Grafana: *Connections → Add new connection → PostgreSQL*, con
host `postgres:5432`, base `sgrc_db`, usuario `grafana` y TLS deshabilitado
(el tráfico no sale del servidor).

**Solo lectura, y no el usuario de la aplicación**: un tablero mal escrito no
tiene por qué poder borrar nada, y alcanza con que alguien deje una consulta
pesada corriendo para que se note en el sistema. Un ejemplo para empezar:

```sql
SELECT date_trunc('month', fecha) AS mes, count(*) AS reservas
FROM reserva
WHERE estado <> 'CANCELADA'
GROUP BY 1 ORDER BY 1;
```

---

## 10. Cuánto guarda

Prometheus está limitado a **30 días o 1 GB**, lo que pase primero. Los dos
topes existen porque esto corre en el servidor de una institución y no en
infraestructura dedicada: sin ellos, el histórico crece hasta llenar el
disco, y el primero en enterarse sería Postgres, que dejaría de poder
escribir.

Si hiciera falta más historia, conviene subir el tope de tamaño antes que el
de días: es el que realmente protege el disco.
