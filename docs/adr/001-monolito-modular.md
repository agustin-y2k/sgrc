# ADR 001 — Monolito modular en vez de microservicios

- **Estado:** Aceptado
- **Fecha:** 2026-07-22
- **Contexto normativo:** RNF-03 (`01-requisitos.md`), desarrollado en `06-arquitectura.md` §1, §3 y §7

## Contexto

El SGRC atiende a **una sola institución educativa**: decenas de usuarios
(docentes y un puñado de Admins), un inventario de carros y PCs del orden de
las centenas, y una carga de reservas concentrada en el horario escolar. Corre
sobre un **único servidor**, típicamente hardware que la institución ya tiene
y comparte con otros usos —del orden de unos pocos cores y algunos GB de RAM—,
expuesto mediante un túnel inverso. Y **no hay un equipo de DevOps** detrás: el
mantenimiento queda en manos de quien desarrolla.

Al mismo tiempo, el sistema tiene dominios razonablemente separables —
autenticación, gestión académica, inventario, reservas, notificaciones y
reportes — con reglas de negocio propias en cada uno. Esa separación hace
tentador el salto a microservicios, y existe la posibilidad concreta (no
inmediata, pero tampoco descartable) de que el proyecto se extienda a más de
una escuela.

La decisión es cómo estructurar el sistema para que esa separación de dominios
sea real y verificable, sin pagar por adelantado la complejidad operativa de
distribuirlo.

## Decisión

**Un monolito modular: un solo binario Go y un solo PostgreSQL, organizados en
paquetes internos con límites de dominio explícitos.**

Las tres reglas que definen la decisión:

1. **Ningún paquete de `internal/` importa el `domain/` de otro.** La
   comunicación entre paquetes pasa siempre por una interfaz pequeña
   (un *puerto*) declarada en la capa `application/` del paquete consumidor,
   e implementada por un adaptador que se inyecta desde `cmd/main.go`. Es el
   mismo contrato que tendría una llamada REST entre servicios separados, solo
   que resuelto en compile-time y sin latencia de red.

2. **La comunicación asincrónica va por un event bus in-process** con interfaz
   `Publish`/`Subscribe` (`internal/shared/eventbus`), no por llamadas directas
   entre paquetes. `reservation` publica eventos como `reserva.cancelada`;
   `notification` y `reporting` se suscriben en el arranque.

3. **El límite se verifica automáticamente, no por disciplina.**
   `internal/shared/archtest` parsea todos los `.go` de `internal/` y falla el
   build si encuentra un import de `internal/<otro>/domain`. Sin ese test la
   regla 1 se erosiona sola: un import de más compila perfecto y nadie lo revisa
   a mano en cada PR.

`internal/shared/` queda exceptuado de la regla 1 por ser transversal por
diseño (middleware, event bus, primitivas de seguridad) y no tener `domain/`
propio.

## Consecuencias

### Positivas

- **Complejidad operativa mínima.** Tres contenedores (`sgrc-app`, `postgres`,
  `cloudflared`), ~150–200 MB de RAM, un `docker compose up`. Sin service mesh,
  sin descubrimiento de servicios, sin tracing distribuido, sin un broker que
  mantener.
- **Integridad referencial real.** Al haber una sola base, toda referencia entre
  tablas es una foreign key de verdad. En un esquema distribuido, media docena
  de esas relaciones se convertirían en consistencia eventual gestionada a mano.
- **Transacciones locales.** Operaciones como "crear una reserva agrupada de N
  PCs, o ninguna" son un `BEGIN`/`COMMIT`, no una saga con compensaciones.
- **Refactor barato.** Mover un límite mal trazado entre dos paquetes es mover
  archivos y ajustar una interfaz, no versionar y desplegar dos servicios.
- **La puerta a microservicios queda abierta.** Extraer un paquete significa
  reemplazar el adaptador in-memory que cumple su puerto por un cliente HTTP que
  cumpla el mismo puerto, sin tocar la lógica de dominio. El event bus se cambia
  por NATS o Kafka sin tocar quién publica ni quién se suscribe.

### Negativas (aceptadas)

- **No hay aislamiento de fallos ni escalado independiente.** Si el proceso
  muere, muere todo; si `reporting` consume CPU, la consume de todos. A esta
  escala es un intercambio favorable: el modo de falla realista es que se caiga
  el único servidor, en cuyo caso ningún reparto de procesos habría ayudado.
- **La entrega de eventos no tiene garantías.** El bus es en memoria: sin
  persistencia de mensajes ni at-least-once. Es aceptable justamente porque hay
  un solo proceso — si se cae, publicadores y suscriptores se reinician juntos,
  así que no existe el escenario de "el evento se emitió pero el consumidor
  estaba caído".
- **Un despliegue redeploya todo.** Un cambio en una notificación reinicia
  también las reservas. Con un binario que arranca en milisegundos y una
  ventana de uso acotada al horario escolar, el costo es despreciable.
- **Los límites dependen de que el test los sostenga.** Si `archtest` se
  desactiva o se le agregan excepciones a la ligera, esto degrada silenciosamente
  a un monolito plano y la ventaja principal de la decisión desaparece. El test
  no es opcional: es lo que hace que "modular" signifique algo.

## Alternativas consideradas

### Microservicios desde el principio

**Descartada.** El costo se paga desde el día uno — un repo o carpeta por
servicio, bases separadas, un broker, orquestación, observabilidad distribuida,
consistencia eventual en relaciones que hoy son FKs — y el beneficio
(aislamiento de fallos, escalado independiente, despliegue por equipo) no aplica
a una escuela con decenas de usuarios, un servidor y una sola persona
desarrollando. Sería complejidad accidental sin un problema que la justifique.

### Monolito plano (sin límites de paquete)

**Descartada.** Es más simple de escribir al principio, pero deja que cualquier
parte del código toque las entidades de cualquier otra. En un sistema con reglas
de negocio densas y entrelazadas (solapamiento, recurrencias, cascadas de
cancelación, archivado de ciclos) eso convierte cualquier cambio en un riesgo
difícil de acotar, y hace irreversible la posibilidad de dividir el sistema más
adelante. El sobrecosto del monolito modular frente al plano — declarar un
puerto en vez de importar una struct — es bajo y se paga una sola vez por
límite.

### Monolito modular con base de datos por módulo (schemas separados)

**Descartada.** Refuerza el límite también en la capa de datos, pero renuncia a
las foreign keys entre módulos justo donde más valen (una reserva referencia una
PC, un curso y un docente), sin traer a cambio ninguna de las ventajas reales de
distribuir. Es el costo de los microservicios sin sus beneficios.

## Revisión

Reconsiderar esta decisión si se cumple alguna de estas condiciones:

- El sistema pasa a atender **más de una institución** con datos aislados entre
  sí.
- Algún paquete desarrolla una carga que justifique escalarlo por separado
  (el candidato natural sería `reporting`).
- El equipo crece a un punto donde los despliegues independientes por área
  dejan de ser una comodidad y pasan a ser un cuello de botella.

Mientras ninguna se cumpla, la extracción a servicios es trabajo sin retorno.
