# Observabilidad — Prometheus y Grafana

Cómo mirar lo que el sistema está haciendo por dentro: qué se pide, qué
tarda, qué se rompe y si los procesos de fondo siguen corriendo.

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
| **Esto: Prometheus y Grafana** | ¿Por qué está lento? ¿Qué se rompe? ¿Desde cuándo? | el mismo servidor | **se apaga con él** |

De ahí la regla para no llevarse una decepción: **esto no sirve para
enterarse de que el sistema se cayó** —se cae junto con él—, sirve para
entender qué está pasando cuando algo anda mal, y para ver venir lo que
todavía no se rompió.

---

## 2. Levantarlo

```bash
make observabilidad        # el sistema + Prometheus + Grafana
make observabilidad-stop   # apaga los tableros, el sistema sigue
```

Sin ese comando, los dos contenedores no existen: están detrás de un
*profile* del compose (`docker compose --profile observabilidad up -d` es lo
que hace el atajo). Un `make run-prod` normal no los levanta.

**Lo que cuesta**: unos 400 MB de memoria entre los dos, y hasta 1 GB de
disco para el histórico. En un servidor compartido con otros usos, es lo
primero a apagar si falta memoria.

Antes de levantarlo, poné `GRAFANA_PASSWORD` en el `.env`: es la contraseña
del usuario `admin`.

---

## 3. Entrar a Grafana

Queda en `http://localhost:3000` **del propio servidor**, publicado solo en
`127.0.0.1` a propósito: es un panel de administración y no tiene por qué
estar a la vista del resto de la red de la institución.

Desde otra computadora se llega con un túnel de SSH:

```bash
ssh -L 3000:localhost:3000 usuario@servidor
```

y después `http://localhost:3000` en el navegador propio. Usuario `admin`,
contraseña la del `.env`.

El tablero **SGRC — funcionamiento** ya viene cargado. No hay que configurar
el origen de datos: se aprovisiona solo al arrancar
(`observabilidad/grafana/`).

> Los cambios hechos desde la interfaz de Grafana **se pierden al
> reiniciar**: la fuente de verdad es el JSON del repositorio. Si un panel
> nuevo vale la pena, se exporta y se commitea. Es a propósito — un tablero
> que solo existe en el disco de un servidor se pierde con el servidor.

---

## 4. Qué se mide y qué pregunta contesta

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

## 5. `/metrics` no se publica a internet

El backend expone las métricas en `/metrics`, y **eso no sale por el túnel**:
nginx solo proxea `/api` y `/health`; cualquier otra ruta cae en la interfaz
(`frontend/nginx.conf`). Pedir `/metrics` desde afuera devuelve la página de
la aplicación, no las métricas.

Quien las consulta es Prometheus, por la red interna de Docker. No hay
ningún puerto publicado para eso.

---

## 6. Alertas: se ven, pero no notifican

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

## 7. Tableros de uso (reservas por mes, equipo más pedido)

Para eso **no hace falta Prometheus**: esos datos ya están en Postgres, con
tablas históricas que sobreviven al cierre del ciclo lectivo
(`historico_uso_equipo`, `historico_uso_docente`). Se agrega Postgres como
segundo origen de datos de Grafana y se consulta con SQL.

No viene aprovisionado porque necesita un usuario de solo lectura, que hay
que crear una vez. Abrí la consola con `make psql` y pegá esto adentro:

```sql
CREATE ROLE grafana LOGIN PASSWORD 'una-contraseña-larga';
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

## 8. Cuánto guarda

Prometheus está limitado a **30 días o 1 GB**, lo que pase primero. Los dos
topes existen porque esto corre en el servidor de una institución y no en
infraestructura dedicada: sin ellos, el histórico crece hasta llenar el
disco, y el primero en enterarse sería Postgres, que dejaría de poder
escribir.

Si hiciera falta más historia, conviene subir el tope de tamaño antes que el
de días: es el que realmente protege el disco.
