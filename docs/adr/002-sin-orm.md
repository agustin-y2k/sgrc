# ADR 002 — SQL escrito a mano sobre pgx, sin ORM

- **Estado:** Aceptado
- **Fecha:** 2026-08-13
- **Contexto normativo:** RNF-01 y RNF-03 (`01-requisitos.md`), desarrollado en `06-arquitectura.md` §2 y §7, y en `07-modelo-datos.md`

## Contexto

La persistencia del SGRC son hoy unas 4.600 líneas de repositorios que hablan
con PostgreSQL a través de `pgx/v5`, con el SQL escrito a mano. Cada paquete
declara la interfaz del repositorio que necesita en su capa `application/` y la
implementa en `infrastructure/`; los tests de integración levantan un Postgres
real en un contenedor efímero y le aplican las migraciones de `/migrations`.

La pregunta de si conviene un ORM —GORM es el candidato natural en Go— aparece
naturalmente al mirar ese volumen de código. Es una pregunta legítima: buena
parte de esas líneas son mecánicas (armar el `SELECT`, escanear la fila,
construir la entidad), y es exactamente el trabajo que un ORM promete borrar.

Hay dos datos del proyecto que pesan sobre la decisión. El primero es que **el
esquema no es un depósito pasivo de filas**: tiene 113 construcciones propias de
PostgreSQL —constraints de exclusión, índices parciales, `CHECK`, políticas de
borrado— y 18 archivos del backend contienen SQL que va más allá del CRUD. El
segundo es que **el sistema está terminado**: lo que queda es ponerlo a
funcionar en una institución, no explorar su arquitectura.

## Decisión

**Se mantiene el SQL escrito a mano sobre `pgx/v5`. No se incorpora un ORM.**

Tres razones, en orden de peso:

1. **Las garantías del sistema viven en el motor, y un ORM no las alcanza.** La
   regla más importante del dominio —dos personas no pueden reservar el mismo
   equipo en la misma franja— no es una validación en Go, es una constraint:

   ```sql
   ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
       EXCLUDE USING gist (equipo_id WITH =, tsrange(...) WITH &&)
       WHERE (estado = 'CONFIRMADA');
   ```

   El repositorio traduce el `23P01` que devuelve esa constraint a un error de
   dominio. La traducción de errores de GORM cubre el duplicado (`23505`) y la
   clave foránea (`23503`), pero **no** la violación de exclusión: ese caso
   habría que desarmarlo a mano contra `pgconn` igual que hoy. Lo mismo vale
   para el `COUNT(*) OVER()` que cuenta antes del recorte al paginar, el
   `ON CONFLICT` que reactiva una cuenta, la búsqueda de solapamientos que
   resuelve en una sola consulta lo que antes eran cientos de lecturas, y la
   aritmética `fecha + hora_fin` con zona horaria. Todo eso terminaría como SQL
   crudo dentro del ORM, que es lo peor de las dos opciones: se paga la
   abstracción y no se la usa.

2. **La arquitectura obliga a mapear igual, así que el ahorro se cancela.**
   Ningún paquete puede importar el `domain/` de otro y hay un test que lo
   verifica (ADR 001). Los structs anotados de un ORM tendrían que vivir en el
   dominio —rompiendo esa regla— o duplicarse en `infrastructure/` como un
   juego paralelo de modelos que se mapea a mano. Esa segunda opción es
   correcta, y es también exactamente el trabajo manual que el ORM venía a
   eliminar.

3. **El cambio cuesta mucho y no agrega nada visible.** Migrar significa
   reescribir la capa de persistencia entera y revalidar diez paquetes de tests
   de integración, tocando las consultas donde una auditoría ya encontró errores
   sutiles —un `JOIN` que dejaba fuera de los reportes a las cuentas eliminadas,
   un estado que contaba como uso sin serlo—. A cambio, ni una función nueva
   para quien usa el sistema.

## Consecuencias

### Positivas

- **Se ve lo que se ejecuta.** Cada consulta está escrita donde se lee, con su
  plan implícito a la vista; no hay una capa que decida por su cuenta emitir una
  consulta por fila.
- **El motor se usa entero.** Constraints de exclusión, índices parciales,
  funciones de ventana y `ON CONFLICT` están disponibles sin pelear con una
  abstracción que los desconoce.
- **Los errores del motor son parte del dominio.** Un `23P01` no es una falla
  técnica que se escapa: se traduce a un error con significado y el handler lo
  convierte en la respuesta correcta.
- **Los tests prueban lo real.** Al no haber capa intermedia, un test de
  integración contra Postgres verifica la misma consulta que corre en
  producción.

### Negativas (aceptadas)

- **El escaneo de filas es repetitivo y hay que escribirlo.** Es la parte
  mecánica del código de infraestructura, y es donde aparecen los errores
  tontos: una columna agregada al `SELECT` que nadie escanea, un orden de
  argumentos cambiado. Se mitiga con los tests de integración, que fallan
  ruidosamente ante eso.
- **No hay generación automática de esquema.** El esquema se mantiene a mano en
  `/migrations`, y llevar una base existente a una versión nueva es un
  procedimiento manual que se ensaya contra una copia. Es un hueco real —y
  conocido— pero se resuelve con una herramienta de migraciones, no con un ORM.
- **Cambiar de motor sería reescribir la capa entera.** Aceptado sin reservas:
  la elección de PostgreSQL es deliberada y el sistema depende de sus garantías.
  La portabilidad no es un objetivo.
- **Hay que saber SQL para trabajar en la capa de datos.** Quien retome el
  proyecto necesita leer y escribir consultas, no solo llamar métodos.

## Alternativas consideradas

### GORM

**Descartada.** Es el ORM más usado del ecosistema Go y resuelve bien el CRUD:
menos código repetido, asociaciones con `Preload`, generación de esquema. El
problema es que este sistema no es mayormente CRUD. Lo específico —exclusión,
ventanas, conflictos, lotes transaccionales, aritmética temporal con zona—
quedaría igual en SQL crudo, y lo genérico se pagaría con una capa de mapeo
adicional impuesta por los límites de dominio. Suma dependencia y resta
control sin resolver lo difícil.

### sqlc (generar Go tipado a partir del SQL)

**Descartada por ahora, pero es la alternativa viva.** Va en la dirección
opuesta al ORM: el SQL sigue siendo la fuente de verdad y el generador produce
las structs y las funciones de escaneo, que es justamente la parte mecánica.
Conserva todo lo que hace valiosa la decisión actual. No se adopta hoy porque
agrega un generador y un paso de build a un proyecto que ya está terminado, y
porque el problema que resuelve todavía no duele. Es lo primero que hay que
mirar si empieza a doler.

### Helpers de `pgx` para el escaneo (`CollectRows`, `RowToStructByName`)

**Disponible, sin decisión de arquitectura.** Ya vienen con la dependencia
actual y se pueden adoptar consulta por consulta, sin migración ni compromiso.
Es el camino de menor costo para recortar repetición.

## Revisión

Reconsiderar esta decisión si se cumple alguna de estas condiciones:

- El escaneo manual pasa a ser una fuente **repetida** de defectos, y no un
  costo aburrido pero controlado. La respuesta en ese caso es **sqlc**, no un
  ORM: el problema sería la mecánica del escaneo, no el SQL.
- El sistema debe soportar más de un motor de base de datos, lo que hoy no es
  un objetivo.
- El trabajo pasa a ser mayoritariamente CRUD sobre tablas simples y el equipo
  crece con gente que no escribe SQL.

Que la capa de persistencia sea grande no es, por sí solo, un motivo: buena
parte de ese tamaño son las consultas específicas que sostienen las reglas del
dominio, y esas no desaparecen al cambiar de herramienta — se mudan a otro
lugar, con menos control.
