# Modelo de Datos — SGRC

El esquema completo, ejecutable, es `migrations/001_esquema_inicial.sql`, más
los archivos numerados que vengan después: la 001 es el punto de partida
—congelado— y cada migración posterior lo modifica sobre bases que ya tienen
datos (ver `docs/11-operacion.md` §5). Este documento explica **por qué** el
esquema es así: qué invariante protege cada constraint y qué pasaría sin ella.

## 1. Diagrama ER

```mermaid
erDiagram
    USUARIO ||--o{ DOCENTE_MATERIA : asignado
    USUARIO ||--o{ RESERVA_GRUPO : crea
    USUARIO ||--o{ NOTIFICACION : recibe
    USUARIO ||--o{ CODIGO_RECUPERACION : pide
    USUARIO ||--o{ HORARIO_ADMIN : declara
    USUARIO ||--o{ HORARIO_ADMIN_EXCEPCION : declara
    USUARIO ||--o{ HISTORICO_USO_DOCENTE : resume
    CARRO ||--o{ EQUIPO : contiene
    EQUIPO ||--o{ INCIDENCIA : registra
    EQUIPO ||--o{ LICENCIA_SOFTWARE : tiene
    EQUIPO ||--o{ EQUIPO_PREFERENCIA : prefiere
    EQUIPO ||--o{ PRESTAMO : sale_en
    EQUIPO ||--o{ RESERVA : ocupa
    EQUIPO ||--o{ HISTORICO_USO_EQUIPO : resume
    RESERVA ||--o{ PRESTAMO : origina
    CICLO_LECTIVO ||--o{ CURSO : contiene
    CURSO ||--o{ MATERIA : contiene
    MATERIA ||--o{ DOCENTE_MATERIA : asigna
    MATERIA ||--o{ RESERVA_GRUPO : recibe
    MATERIA ||--o{ REGLA_RECURRENCIA : tiene
    REGLA_RECURRENCIA ||--o{ RESERVA_GRUPO : materializa
    RESERVA_GRUPO ||--o{ RESERVA : contiene

    USUARIO { uuid id; string nombre; string apellido; string email; string password_hash; string google_sub; bool debe_cambiar_password; string rol; string estado; timestamptz fecha_registro; timestamptz fecha_aprobacion; uuid aprobado_por; string curso_solicitado; string materia_solicitada; string rol_solicitado; string cargo_solicitado; int version_sesion }
    CODIGO_RECUPERACION { uuid id; uuid usuario_id; string codigo_hash; timestamptz creado_en; timestamptz expira_en; timestamptz usado_en; int intentos }
    CARRO { uuid id; string nombre; string descripcion }
    EQUIPO { uuid id; uuid carro_id; int identificador; string nombre; string tipo; string numero_serie; bool freezado; string cpu; string ram; string sistema_operativo; string software_instalado; string estado; bool reservable; bool dado_de_baja; timestamptz fecha_baja; timestamptz fecha_alta }
    INCIDENCIA { uuid id; uuid equipo_id; uuid reportado_por; string descripcion; string categoria; string gravedad; timestamptz fecha; bool enviado_a_soporte; timestamptz fecha_envio_a_soporte; string estado }
    LICENCIA_SOFTWARE { uuid id; uuid equipo_id; string nombre; int dias_duracion; int dias_aviso; date fecha_vencimiento; date ultima_renovacion; uuid vencimiento_fijado_por; timestamptz vencimiento_fijado_en; date avisado_previo_para; date avisado_vencimiento_para; timestamptz creada_en }
    EQUIPO_PREFERENCIA { uuid id; uuid equipo_id; string materia_nombre; string materia_norm; int anio; string division; int prioridad; timestamptz creada_en }
    CICLO_LECTIVO { uuid id; int anio; bool activo; bool archivado }
    CURSO { uuid id; uuid ciclo_lectivo_id; string nombre; bool activo; bool archivado }
    MATERIA { uuid id; uuid curso_id; string nombre; string nombre_norm; bool activo; bool archivado }
    DOCENTE_MATERIA { uuid id; uuid usuario_id; uuid materia_id; string rol }
    REGLA_RECURRENCIA { uuid id; uuid materia_id; uuid creado_por; string dia_semana; time hora_inicio; time hora_fin; date fecha_inicio; date fecha_fin }
    RESERVA_GRUPO { uuid id; uuid materia_id; uuid creado_por; string nombre_docente_snapshot; date fecha; time hora_inicio; time hora_fin; string estado; uuid regla_recurrencia_id; timestamptz creada_en; timestamptz recordatorio_enviado_en; timestamptz aviso_sin_retirar_en }
    RESERVA { uuid id; uuid reserva_grupo_id; uuid equipo_id; uuid materia_id; string nombre_docente_snapshot; date fecha; time hora_inicio; time hora_fin; string estado; string tipo; string motivo_bloqueo; uuid creado_por; timestamptz creada_en; uuid cancelado_por; string motivo_cancelacion; timestamptz cancelada_en; timestamptz avisado_equipo_no_disponible_en }
    PRESTAMO { uuid id; uuid equipo_id; uuid reserva_id; uuid entregado_a_usuario_id; string entregado_a_nombre; string retirado_por; string motivo; timestamptz devolucion_estimada; uuid entregado_por; timestamptz entregado_en; timestamptz devuelto_en; uuid recibido_por; string observaciones; timestamptz avisado_demora_en; date avisado_cierre_para }
    NOTIFICACION { uuid id; uuid usuario_id; uuid reserva_id; uuid sobre_usuario_id; string mensaje; string tipo; string estado; timestamptz creada_en; timestamptz leida_en }
    HORARIO_ADMIN { uuid id; uuid usuario_id; string dia_semana; time hora_inicio; time hora_fin }
    HORARIO_ADMIN_EXCEPCION { uuid id; uuid usuario_id; date fecha; string tipo; time hora_inicio; time hora_fin; string motivo }
    HISTORICO_USO_EQUIPO { uuid id; int anio; uuid equipo_id; string etiqueta_snapshot; int identificador_snapshot; string carro_nombre_snapshot; int minutos_reservados; int cantidad_reservas }
    HISTORICO_USO_DOCENTE { uuid id; int anio; uuid usuario_id; string nombre_docente_snapshot; int cantidad_reservas; int minutos_totales }
    AUDIT_LOG { uuid id; uuid usuario_id; string accion; string entidad; uuid entidad_id; jsonb detalle; inet ip_origen; timestamptz creado_en }
```

> **Una reserva, varios equipos.** Un docente no reserva un equipo por vez como
> operaciones independientes: tilda los que necesita de una lista, en una sola
> operación. Eso exige separar "la reserva que hace el docente"
> (`reserva_grupo`: una materia, una fecha, un horario) de "cada equipo dentro
> de esa reserva" (`reserva`: una fila por equipo). Las cancelaciones en
> cascada actúan sobre filas `reserva` individuales — nunca sobre todo el
> grupo, salvo que terminen afectando a todos sus equipos.

## 2. Una sola base de datos: `sgrc_db`

Todo vive en la misma base y las referencias cruzadas son **foreign keys
reales**, con integridad referencial completa. `audit_log` es la única
excepción deliberada: no tiene FK a `usuario`, para que lo que hizo una cuenta
siga registrado después de eliminarla.

### Dos clases de tiempo, dos tipos distintos

El esquema distingue a propósito entre **instantes** y **hora de pared**:

- **Instantes** — el momento en que algo pasó: `creada_en`, `cancelada_en`,
  `leida_en`, `fecha_registro`, `fecha_alta`… Van en **`TIMESTAMPTZ`**.
  Postgres normaliza a UTC al escribir y devuelve el valor con zona al leer,
  así que el instante sobrevive el ida y vuelta sin depender de la
  configuración de nadie.

- **Hora de pared de la institución** — `reserva.fecha` (`DATE`) y
  `hora_inicio`/`hora_fin` (`TIME`). "De 8 a 9 del 2 de septiembre" significa
  lo mismo en enero que en julio; no es un instante hasta que se lo combina con
  la zona de la institución. Por eso **no** llevan zona, y por eso el proceso
  lee "ahora" en `APP_TIMEZONE` para compararlos (ver `cmd/main.go`).

> Guardar un instante en un `TIMESTAMP` sin zona rompe de una forma que no da
> la cara: el proceso escribe la hora de pared local, el driver la lee como si
> fuera UTC y la API la serializa con sufijo `Z`. El navegador muestra
> entonces un valor corrido tantas horas como el desfasaje de la zona, sin
> ningún error en el medio.

### `usuario`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| nombre | VARCHAR(100) | NOT NULL |
| apellido | VARCHAR(100) | NOT NULL |
| email | VARCHAR(150) | NOT NULL, UNIQUE + índice único sobre `lower(email)` |
| password_hash | VARCHAR(255) | NULL |
| google_sub | VARCHAR(255) | NULL, índice único parcial |
| debe_cambiar_password | BOOLEAN | NOT NULL DEFAULT false |
| rol | VARCHAR(10) | NOT NULL, CHECK IN ('ADMIN','DOCENTE') |
| estado | VARCHAR(20) | NOT NULL DEFAULT 'PENDIENTE', CHECK IN ('PENDIENTE','APROBADA','RECHAZADA','BAJA') |
| fecha_registro | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| fecha_aprobacion | TIMESTAMPTZ | NULL |
| aprobado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| curso_solicitado | VARCHAR(100) | NULL |
| materia_solicitada | VARCHAR(100) | NULL |
| rol_solicitado | VARCHAR(10) | NULL, CHECK IN (TITULAR, SUPLENTE) |
| cargo_solicitado | VARCHAR(20) | NULL, CHECK IN (DOCENTE, ADMIN_SISTEMA) |
| version_sesion | INTEGER | NOT NULL DEFAULT 0 |
| | | CHECK `password_hash IS NOT NULL OR google_sub IS NOT NULL` |

```sql
CREATE UNIQUE INDEX idx_usuario_email_lower  ON usuario (lower(email));
CREATE UNIQUE INDEX idx_usuario_google_sub   ON usuario (google_sub) WHERE google_sub IS NOT NULL;
CREATE INDEX        idx_usuario_estado       ON usuario (estado);
```

> **El email identifica sin distinguir mayúsculas.** El `UNIQUE` de la columna
> trata `Ana@x.edu` y `ana@x.edu` como direcciones distintas, y eso alcanza
> para que la misma persona termine con dos cuentas y una de ellas sin
> aprobar. El índice funcional sobre `lower(email)` lo impide en la base; el
> login busca por esa misma expresión.

> `debe_cambiar_password` se pone en `true` cuando un Admin resetea la
> contraseña de alguien (RF-01.6). El login sigue funcionando con la
> contraseña temporal, pero la respuesta incluye la bandera para que el
> frontend fuerce la pantalla de cambio antes de dejar entrar al resto del
> sistema; al cambiarla vuelve a `false`. Sin esta columna, "exigir el cambio
> en el próximo ingreso" no tendría ningún mecanismo que lo hiciera cumplir.

> `version_sesion` es lo que permite **cerrar las sesiones abiertas** al
> cambiar una contraseña (RF-01.11). El número viaja dentro del JWT y el
> middleware lo compara contra el de la fila en cada request — en la misma
> consulta que ya hacía para verificar el estado, así que no agrega ninguna.
>
> Es un entero y no un instante ("invalidar lo emitido antes de X") porque
> `iat` en un JWT tiene resolución de segundos: comparado contra un `now()`
> con microsegundos, el token que el propio cambio acaba de emitir se
> rechazaría a sí mismo, y redondear al segundo deja una ventana en la que las
> sesiones abiertas en ese mismo segundo sobreviven. El `DEFAULT 0` coincide
> con el claim ausente, así que una instalación existente no desloguea a
> nadie al incorporar la columna.

> `curso_solicitado` y `materia_solicitada` son **texto libre, no FKs**: al
> registrarse la persona todavía no está autenticada, así que no puede elegir
> de una lista, y lo que declara puede no existir todavía en el sistema — de
> hecho el Admin quizás lo tenga que crear al aprobarla (RF-02.6). Es una
> declaración de intención, no un vínculo.
>
> `rol_solicitado` y `cargo_solicitado` acompañan a esos dos y son la
> excepción que confirma la regla: **sí llevan CHECK**, porque son listas
> cerradas que existen siempre —"titular o suplente" es la misma de
> `docente_materia.rol`— y no nombran nada que pueda faltar. Siguen siendo
> declaraciones: el rol que rige es el que el Admin carga en `docente_materia`
> al asignar.
>
> **`cargo_solicitado` no es `rol`.** `rol` es lo que la cuenta puede hacer;
> `cargo_solicitado` es lo que la persona dijo ser al registrarse, y no otorga
> ningún permiso: quien declara `ADMIN_SISTEMA` nace igual con
> `rol = 'DOCENTE'` y `estado = 'PENDIENTE'`, y para que administre el sistema
> un Admin tiene que promoverlo aparte (RF-01.3 / RF-01.4).
>
> Es **NULL-able** aunque el registro lo exija: las cuentas anteriores a la
> columna quedaron sin cargo declarado, y un Admin creado por otro Admin
> (RF-01.4) tampoco declara ninguno — nadie se autorregistró ahí. La
> obligatoriedad es del caso de uso "registro", no del dato.

> **`BAJA` es distinto de `RECHAZADA`.** `RECHAZADA` es una solicitud de
> registro que nunca se aprobó; `BAJA` es una cuenta que **estuvo** `APROBADA`
> y se dio de baja — la distinción evita mezclar "nunca llegó a entrar" con
> "entró y se fue" en reportes y auditoría. **`BAJA` es terminal**: la API
> rechaza con 409 cualquier intento de cambiarle el estado. Si la persona
> vuelve, se registra de nuevo; y como el email es único, para reusar el suyo
> un Admin debe eliminar (hard delete) la fila vieja. `aprobado_por` usa
> `SET NULL`: si quien aprobó a otro se elimina, se pierde ese dato, no la
> cuenta aprobada.

> `google_sub` y `password_hash` son **independientes entre sí, pero no las dos
> vacías**. Una cuenta creada con Google no tiene contraseña —a quien la
> verifica es Google— y una cuenta con contraseña no tiene `google_sub`; pero
> un docente que se registró con contraseña y después entra con Google queda
> con las dos y no pierde la que tenía. De ahí dos columnas nullable con un
> `CHECK` que exige al menos una, y no un enum `proveedor`: ese enum obligaría
> a elegir una sola forma de ingreso e inventar un tercer valor `AMBAS` que no
> significa nada distinto de "las dos columnas están llenas". El `CHECK` es la
> red que impide una fila por la que no se pueda entrar de ninguna manera.
>
> Se guarda el claim `sub` del ID token y no el email porque el email de una
> cuenta de Google **puede cambiar y el `sub` no**: quien ya entró alguna vez
> sigue entrando a su misma cuenta aunque Google le haya cambiado la
> dirección. El email sigue siendo la identidad dentro del sistema; el vínculo
> con Google cuelga del `sub`.

### `codigo_recuperacion`
Códigos de un solo uso para recuperar una contraseña olvidada (RF-01.10).

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| codigo_hash | VARCHAR(255) | NOT NULL |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| expira_en | TIMESTAMPTZ | NOT NULL |
| usado_en | TIMESTAMPTZ | NULL |
| intentos | INTEGER | NOT NULL DEFAULT 0 |

```sql
CREATE INDEX idx_codigo_recuperacion_vigente
    ON codigo_recuperacion (usuario_id, creado_en DESC)
    WHERE usado_en IS NULL;
```

> **Es una tabla aparte y no dos columnas en `usuario`** por dos razones. El
> ciclo de vida no tiene nada que ver: una fila acá vive quince minutos, la de
> `usuario` vive años. Y los intentos fallidos se escriben en cada prueba de
> código — meter ese `UPDATE` sobre la fila de usuario haría que cada intento
> de recuperación tocara la fila que usa todo el resto del sistema.
>
> **`codigo_hash` guarda el hash, nunca el código.** Son seis dígitos: si la
> base se filtrara (un backup, un dump de soporte), un código en claro sería
> una cuenta abierta hasta que expire. Se usa el mismo argon2 que las
> contraseñas.
>
> **`usado_en` cubre los dos finales posibles**: el código se consumió bien, o
> se quemó al agotar los intentos. En los dos casos dejó de existir para el
> sistema, y una sola columna evita tener que preguntar por dos.
>
> Las filas viejas **no se borran**: quedan como registro de que esa persona
> pidió un código, que es justo lo que hay que poder mirar si alguien reporta
> algo raro. El índice es parcial (`WHERE usado_en IS NULL`) porque la única
> consulta en caliente es "¿tiene un código sin usar?".

### `ciclo_lectivo`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL, UNIQUE |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| archivado | BOOLEAN | NOT NULL DEFAULT false |

```sql
-- Solo puede haber un ciclo activo a la vez (RF-02.1)
CREATE UNIQUE INDEX idx_ciclo_lectivo_activo_unico ON ciclo_lectivo (activo) WHERE activo = true;
```

### `curso`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| ciclo_lectivo_id | UUID | FK → ciclo_lectivo.id, NOT NULL |
| nombre | VARCHAR(4) | NOT NULL, CHECK (nombre ~ '^[1-6]°[A-Z]$') |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| archivado | BOOLEAN | NOT NULL DEFAULT false |
| | | UNIQUE (ciclo_lectivo_id, nombre) |

> `nombre` no es libre: año (`1°`-`6°`) + división (`A`-`Z`), ej. `1°A`, `6°Z`.
> La validación vive tanto en el `CHECK` de la base como en la capa de
> aplicación, para devolver un 400 claro antes de llegar a la constraint.

### `materia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| curso_id | UUID | FK → curso.id **ON DELETE CASCADE**, NOT NULL |
| nombre | VARCHAR(100) | NOT NULL |
| nombre_norm | VARCHAR(100) | GENERATED ALWAYS AS `translate(lower(nombre), 'áéíóúüñ', 'aeiouun')` STORED |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| archivado | BOOLEAN | NOT NULL DEFAULT false |
| | | UNIQUE (curso_id, nombre) |

> `nombre_norm` es la forma canónica del nombre —sin mayúsculas ni acentos—
> contra la que se cruzan las marcas de preferencia de `equipo_preferencia`
> (RF-03.21). Se resuelve con `translate()` y **no con `unaccent()`**: esa
> función vive en una extensión, depende de un diccionario y por eso no es
> IMMUTABLE, así que no se puede usar ni en una columna generada ni en un
> índice.

> **Edición y eliminación mientras el ciclo está activo (RF-02.11):** `nombre`
> se puede editar en cualquier momento, revalidando el patrón y la unicidad. La
> eliminación (hard delete) solo se permite si no hay ninguna `reserva_grupo`
> con esa `materia_id` — para un curso, si ninguna de sus materias tiene
> reservas. Se comprueba con una consulta de existencia antes del `DELETE`, no
> con una constraint. Al eliminar un curso, sus materias se van en cascada (y
> con ellas sus `docente_materia`).

### `docente_materia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| materia_id | UUID | FK → materia.id **ON DELETE CASCADE**, NOT NULL |
| rol | VARCHAR(10) | NOT NULL, CHECK IN ('TITULAR','SUPLENTE') |
| | | UNIQUE (usuario_id, materia_id) |

```sql
CREATE INDEX idx_docente_materia_usuario ON docente_materia (usuario_id);
CREATE INDEX idx_docente_materia_materia ON docente_materia (materia_id);
CREATE INDEX idx_curso_ciclo             ON curso (ciclo_lectivo_id);
CREATE INDEX idx_materia_curso           ON materia (curso_id);
```

> `usuario_id` en `CASCADE`: si el usuario se elimina definitivamente, el
> vínculo pierde todo sentido — no hay nada que preservar acá, a diferencia de
> las reservas, que tienen `nombre_docente_snapshot`. En la práctica casi nunca
> dispara solo, porque dar de baja a un docente (RF-02.8) ya elimina sus filas
> como parte del mismo flujo.
>
> **Solo pueden asignarse docentes con `estado = APROBADA`.** Lo valida la capa
> de aplicación: Postgres no tiene un CHECK que mire otra tabla.

### `carro`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| nombre | VARCHAR(100) | NOT NULL, UNIQUE |
| descripcion | TEXT | NULL |

> **Qué es un carro, literalmente**: un mueble metálico con ruedas y zócalos
> numerados donde las notebooks se guardan y se cargan cuando no se usan. Está
> siempre en el laboratorio de informática.
>
> **La cantidad de zócalos varía de un carro a otro** y el modelo no la
> presupone en ningún lado: no hay columna de capacidad, ni tope en el
> `identificador` —solo que sea un entero—, ni constraint que cuente equipos
> por carro.
>
> Eso explica el modelo mejor que cualquier otra cosa: `equipo.identificador`
> **es el número del zócalo**. Por eso `UNIQUE (carro_id, identificador)` y no
> un único global —el zócalo 7 existe en cada carro— y por eso la etiqueta
> "PC 7" le sirve a alguien parado frente al mueble buscando cuál sacar.
>
> El `ADMIN` edita `nombre`/`descripcion` en cualquier momento. No hay
> "eliminar carro": un carro se vacía dando de baja sus equipos.

### `equipo`
Todo lo que la institución presta, en una sola tabla: las computadoras de un
carro y también proyectores, cargadores o notebooks sueltas (RF-03.15).

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| carro_id | UUID | FK → carro.id, NULL en los equipos sueltos |
| identificador | INTEGER | NULL en los equipos sueltos |
| nombre | VARCHAR(100) | NULL en los equipos de carro, CHECK no vacío ni con espacios al borde |
| tipo | VARCHAR(50) | NOT NULL DEFAULT 'PC', CHECK no vacío ni con espacios al borde |
| numero_serie | VARCHAR(50) | UNIQUE, NULL, CHECK no vacío y en forma canónica |
| freezado | BOOLEAN | NOT NULL DEFAULT false |
| cpu | VARCHAR(100) | NULL |
| ram | VARCHAR(20) | NULL |
| sistema_operativo | VARCHAR(50) | NULL |
| software_instalado | TEXT | NULL |
| estado | VARCHAR(25) | NOT NULL DEFAULT 'DISPONIBLE', CHECK IN ('DISPONIBLE','EN_MANTENIMIENTO','FUERA_DE_SERVICIO') |
| reservable | BOOLEAN | NOT NULL DEFAULT true |
| dado_de_baja | BOOLEAN | NOT NULL DEFAULT false |
| fecha_baja | TIMESTAMPTZ | NULL |
| fecha_alta | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| | | UNIQUE (carro_id, identificador) |
| | | CHECK `chk_equipo_identificable` |

```sql
-- Las dos formas de existir: en un carro con número, o suelto con nombre.
CONSTRAINT chk_equipo_identificable CHECK (
    (carro_id IS NOT NULL AND identificador IS NOT NULL)
    OR
    (carro_id IS NULL AND nombre IS NOT NULL)
)

CREATE INDEX idx_equipo_carro_estado ON equipo (carro_id, estado);
CREATE INDEX idx_equipo_sueltos      ON equipo (tipo, nombre) WHERE carro_id IS NULL;

-- Entre los equipos sueltos el nombre es lo único que los distingue.
-- Excluye los dados de baja: el nombre es un apodo y se reutiliza al
-- reemplazar el equipo, a diferencia de un número de serie.
CREATE UNIQUE INDEX ux_equipo_suelto_nombre
    ON equipo (lower(nombre)) WHERE carro_id IS NULL AND dado_de_baja = false;
```

> **Una sola tabla para todo lo prestable, y no es un detalle de
> implementación.** "Qué hay afuera del laboratorio" (RF-08) tiene que ser una
> sola lista: con dos clases de cosa, el préstamo necesitaría dos referencias,
> el mostrador dos consultas y el barrido dos recorridos. Compartiendo entidad,
> un proyector queda prestable, reclamable, liberable y —si es reservable—
> reservable, sin una línea nueva en ninguno de esos flujos.
>
> El `CHECK` de identificabilidad es lo que impide la fila sin sentido: un
> equipo sin carro y sin nombre no se puede nombrar en ninguna pantalla.

> `identificador` es el número del zócalo, único **dentro de su carro** — dos
> carros pueden tener cada uno una "PC 27". `numero_serie` es distinto: es el
> de fábrica, **único en toda la tabla**. Y a pesar del nombre **es texto**,
> porque los códigos de fábrica llevan letras (`5CD1234ABC`); un tipo numérico
> haría imposible cargar el número que dice la etiqueta. Se guarda normalizado
> (mayúsculas, sin espacios al borde) y la base lo exige con un CHECK: sin
> forma canónica, la misma máquina cargada dos veces con distinta caja son dos
> filas distintas para el `UNIQUE`.

> `tipo` es **texto libre y no un enum**: la lista de cosas que presta una
> institución no es la de otra, y agregar una categoría nueva no puede exigir
> tocar el sistema. El formulario sugiere los tipos ya cargados para que no
> convivan "PROYECTOR" y "Proyector".

> `freezado` indica si el equipo tiene Deep Freeze (u otro software
> equivalente) instalado. Es **metadata informativa**: no restringe reservas ni
> ningún otro flujo. Se muestra a todos los usuarios autenticados, junto con
> `software_instalado` y `estado`, porque un docente los necesita para elegir
> qué reservar (RF-03.7).

> `dado_de_baja` / `fecha_baja`: el "eliminar" de la interfaz es un **soft
> delete**. La fila permanece porque `incidencia`, `reserva` y `prestamo` la
> referencian por FK, y esa historia no se pierde. Un hard delete exigiría
> borrar en cascada todo ese historial.

### `incidencia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| equipo_id | UUID | FK → equipo.id, NOT NULL |
| reportado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| descripcion | TEXT | NOT NULL |
| categoria | VARCHAR(50) | NULL, CHECK (NULL, o no vacía y sin espacios al borde) |
| gravedad | VARCHAR(10) | NOT NULL, CHECK IN ('LEVE','MODERADA','GRAVE') |
| fecha | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| enviado_a_soporte | BOOLEAN | NOT NULL DEFAULT false |
| fecha_envio_a_soporte | TIMESTAMPTZ | NULL |
| estado | VARCHAR(20) | NOT NULL DEFAULT 'ABIERTA', CHECK IN ('ABIERTA','EN_REPARACION','ENVIADA_A_SOPORTE','RESUELTA') |

```sql
CREATE INDEX idx_incidencia_equipo ON incidencia (equipo_id);

-- Agrupar por tipo de falla sin distinguir mayúsculas (RF-06.7). Parcial
-- porque las no clasificadas se cuentan por separado.
CREATE INDEX idx_incidencia_categoria ON incidencia (lower(categoria)) WHERE categoria IS NOT NULL;

-- "La última incidencia de este equipo", que es lo que pide el listado de
-- equipos fuera de circulación (RF-06.6) por cada fila que muestra.
CREATE INDEX idx_incidencia_equipo_fecha ON incidencia (equipo_id, fecha DESC);
```

> `reportado_por` en `SET NULL`: el historial de la incidencia vale por sí
> mismo aunque se pierda el dato de quién la reportó.

> `categoria` es **texto libre y no un enum**, por el mismo criterio que
> `equipo.tipo`: cada institución rompe cosas distintas, y una lista cerrada
> haría que la primera falla no prevista pidiera un cambio de esquema para
> poder anotarse. Es una decisión con un costo asumido —"Batería" y "batería"
> son dos cadenas— que se compensa fuera de la base: el formulario sugiere las
> ya usadas y los reportes agrupan por `lower(categoria)`. El `CHECK` solo
> impide las dos formas de escribir "nada" que no son `NULL` (la cadena vacía
> y los espacios), para que exista **un** valor que signifique sin clasificar y
> no tres.
>
> `NULL` no es un dato faltante sino un estado real: una máquina que no
> enciende y que nadie diagnosticó todavía tiene una falla y ninguna
> categoría. Los reportes la cuentan aparte en vez de descartarla (RF-06.7).

### `licencia_software`
Licencias de software con vencimiento periódico, una por (equipo, software).
Ver RF-03.11 a RF-03.14.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| equipo_id | UUID | FK → equipo.id **ON DELETE CASCADE**, NOT NULL |
| nombre | VARCHAR(100) | NOT NULL, CHECK no vacío y sin espacios al borde |
| dias_duracion | INTEGER | NOT NULL, CHECK entre 1 y 3650 |
| dias_aviso | INTEGER | NOT NULL DEFAULT 1, CHECK entre 0 y 365 |
| fecha_vencimiento | DATE | **NULL** = a verificar |
| ultima_renovacion | DATE | NULL |
| vencimiento_fijado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| vencimiento_fijado_en | TIMESTAMPTZ | NULL |
| avisado_previo_para | DATE | NULL |
| avisado_vencimiento_para | DATE | NULL |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

```sql
CREATE UNIQUE INDEX ux_licencia_equipo_nombre ON licencia_software (equipo_id, lower(nombre));
CREATE INDEX idx_licencia_vencimiento ON licencia_software (fecha_vencimiento)
    WHERE fecha_vencimiento IS NOT NULL;
```

> **No hay ninguna columna con "los días que faltan".** El contador es
> `fecha_vencimiento - hoy`, calculado al leer (RF-03.12). Guardarlo obligaría
> a un job que lo decremente todos los días, y bastaría con que el servidor
> estuviera apagado uno para que quedara mal para siempre.

> **`fecha_vencimiento` NULL significa "todavía no se verificó", no "no vence
> nunca".** Es el estado real de una licencia cargada antes de poder sentarse
> delante de la máquina. Con la columna `NOT NULL`, la única salida sería
> inventar una fecha — y la tentación es "se renovó hoy", que falla en la
> dirección peligrosa: si en realidad vencía en tres días, el sistema regala
> treinta de silencio justo cuando tendría que avisar.

> **Las dos marcas de aviso guardan una FECHA, no un booleano:** la fecha de
> vencimiento para la que ya salió cada aviso. Eso hace idempotente al barrido
> sin estado extra ni resets — al renovar cambia `fecha_vencimiento`, las
> marcas dejan de coincidir solas y el ciclo nuevo vuelve a avisar. Reiniciar
> el proceso diez veces en un día manda un solo aviso.

> **`vencimiento_fijado_en` no es `ultima_renovacion`.** La primera es cuándo
> se escribió en el sistema; la segunda, cuándo se renovó de verdad. La
> diferencia cubre el caso habitual: "la renové el martes y lo cargué el
> jueves". `ultima_renovacion` queda `NULL` cuando el vencimiento se fijó por
> otro camino (por los días que faltan, o escribiendo la fecha), porque
> deducirla sería inventar un dato.

> **La unicidad es un índice funcional sobre `lower(nombre)` y no una columna
> normalizada**: acá el nombre se muestra en pantalla y en los correos, y
> pasarlo todo a minúsculas dejaría "autocad 2027" a la vista.

> **`vencimiento_fijado_por` en `SET NULL`** por lo mismo que
> `regla_recurrencia.creado_por`: sin política de borrado, eliminar
> definitivamente a un usuario (RF-01.9) muere con un 500 arrastrado por esta
> FK.

### `equipo_preferencia`
Qué materia prefiere cada equipo (RF-03.21). **Sólo ordena la lista al
reservar**: no es un permiso, no oculta nada y no afecta ninguna reserva.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| equipo_id | UUID | FK → equipo.id **ON DELETE CASCADE**, NOT NULL |
| materia_nombre | VARCHAR(100) | NOT NULL, CHECK no vacío y sin espacios en los bordes |
| materia_norm | VARCHAR(100) | GENERATED ALWAYS AS `translate(lower(materia_nombre), …)` STORED |
| anio | SMALLINT | NULL, CHECK entre 1 y 6 |
| division | CHAR(1) | NULL, CHECK `^[A-Z]$` |
| prioridad | SMALLINT | NOT NULL DEFAULT 1, CHECK entre 1 y 9 |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| | | CHECK `division IS NULL OR anio IS NOT NULL` |
| | | UNIQUE **NULLS NOT DISTINCT** (equipo_id, materia_norm, anio, division) |

```sql
CREATE INDEX idx_equipo_preferencia_materia ON equipo_preferencia (materia_norm);
CREATE INDEX idx_equipo_preferencia_equipo  ON equipo_preferencia (equipo_id);
```

> **El vínculo es por nombre y no una FK a `materia`.** `materia` es por curso y
> `ArchivarYClonar` la recrea con UUID nuevo cada año (RF-02.5); `curso`
> también. Una FK a cualquiera de las dos borraría todas las marcas el 31/12,
> que es justo cuando el Admin espera que sigan puestas. Por lo mismo el año y
> la división son **datos literales** y no una referencia a `curso`.
>
> El riesgo obvio de guardar texto —que el nombre se fragmente— se ataca por
> dos lados: el Admin **elige de la lista de nombres que ya existen** en vez de
> tipear (es el único que los escribe, porque quien se registra sólo los
> sugiere), y el cruce va por la columna normalizada, así que "Matemática" y
> "matematica" son la misma materia.

> **`NULLS NOT DISTINCT` no es un detalle de estilo.** Con el `UNIQUE` normal de
> SQL dos filas `(equipo, materia, NULL, NULL)` se consideran distintas entre
> sí, así que la misma marca sin curso se podría cargar infinitas veces.
> Disponible desde Postgres 15; este proyecto corre 16.

> **Los tres alcances y cuál gana.** `(NULL, NULL)` vale para toda materia con
> ese nombre; `(3, NULL)` para todo tercer año; `(3, 'B')` sólo para 3°B. Un
> mismo equipo puede tener los tres, y al reservar **gana el más específico**;
> a igual especificidad, la prioridad más fuerte. El rango de `anio` y el patrón
> de `division` son los mismos del CHECK de `curso.nombre` (`^[1-6]°[A-Z]$`):
> una institución con otra nomenclatura cambia los tres juntos.

> **`ON DELETE CASCADE`** porque una marca no significa nada sin su equipo: dar
> de baja la máquina se lleva sus marcas y no hay nada que preservar.

### `regla_recurrencia`
El **patrón** temporal de una reserva que se repite: materia + día de semana +
horario + rango de fechas. No guarda los equipos — eso vive en los
`reserva_grupo` que la regla materializa, uno por ocurrencia.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| materia_id | UUID | FK → materia.id, NOT NULL |
| creado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| dia_semana | VARCHAR(10) | NOT NULL, CHECK (`LUNES`…`VIERNES`) |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin <> hora_inicio) |
| fecha_inicio | DATE | NOT NULL |
| fecha_fin | DATE | NOT NULL, CHECK (fecha_fin >= fecha_inicio) |

### Bloques que cruzan la medianoche

Las escuelas nocturnas dictan de 22:00 a 01:00. Eso era inexpresable mientras el esquema exigía `hora_fin > hora_inicio`: había que partir la clase en dos reservas —una hasta las 23:59 y otra desde las 00:00 del día siguiente— que el sistema trataba como cosas sin relación. Cancelar una dejaba la otra viva, los reportes contaban dos clases donde hubo una, y el equipo figuraba devuelto a medianoche.

La regla es la que cualquiera lee sin que se la expliquen:

| | Significa |
|---|---|
| `hora_fin > hora_inicio` | termina el mismo día |
| `hora_fin < hora_inicio` | **termina al día siguiente** |
| `hora_fin = hora_inicio` | inválido — lo rechaza el CHECK |

La igualdad podría querer decir "veinticuatro horas" y se rechaza igual: nadie escribe 08:00–08:00 buscando un día entero, y dejarlo pasar haría que el tope de duración contestara con un mensaje sobre las horas que no explica el verdadero problema, que es un tipeo.

> **Dónde vive la regla.** En la función `fin_de_pared(fecha, hora_inicio, hora_fin)`, que usan la constraint `EXCLUDE` de `reserva`, los barridos y todos los listados. Es `IMMUTABLE` porque sin eso Postgres no la acepta dentro del índice de la constraint. Su gemelo en Go es `domain.FinDePared`, y **las dos tienen que decir lo mismo**: si divergen, la aplicación acepta reservas que la base rechaza o —peor— deja pasar solapamientos que creía haber chequeado.

> **Lo que NO cruza la medianoche es `horario_admin`**, el horario de presencia de cada Admin. Es informativo y el cálculo de "¿hay alguien ahora?" es de un solo día; un Admin del turno noche declara dos tramos. La jornada de la institución sí cruza, porque contra ella se validan las reservas.

### `jornada_institucion`

Qué días y entre qué horas abre la escuela. Es la única tabla del sistema **sin dueño**: describe a la institución entera, no a una persona.

| Columna | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| dia_semana | VARCHAR(10) | NOT NULL, CHECK entre los siete días |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin <> hora_inicio) |

> **Tabla vacía y día sin filas significan cosas opuestas.** Vacía = no hay horario declarado, y entonces no hay restricción. Con filas cargadas, un día sin filas es un día en que la escuela no abre. Por eso la validación lee la tabla completa y no solo el día que le preguntan (ver `PermiteReserva` en `availability/domain`).
>
> **Se reemplaza entera, en una transacción.** No hay alta ni baja de una fila suelta: la jornada es una sola decisión de siete días, y mientras se aplicaba por partes quedaba a la vista una jornada a medias que `PermiteReserva` ya estaba usando para aceptar o rechazar reservas. Los ids son nuevos en cada guardado, y eso no molesta a nadie: ninguna otra tabla los referencia.
>
> **De esta tabla también sale el corte de fin de jornada** del barrido de entregas: el fin más tardío del día, más una hora. Sin filas, se cae a `CIERRE_JORNADA` (ver `docs/11-operacion.md`).

> **Varias filas por día a propósito**: turno mañana y turno noche, con el mediodía afuera. El solapamiento se rechaza en la aplicación —igual que en `horario_admin` y por la misma razón: tabla chica, escritura casi nula, y una constraint `EXCLUDE` sobre `TIME` exigiría un tipo de rango que Postgres no trae.

> `dia_semana` admite los siete días. Lo sostiene un `CHECK` en la base y no
> solo el enum de Go: sin él, cualquier `INSERT` que no pase por la aplicación
> entra igual. Ojo con qué fija ese CHECK — el **vocabulario** del enum, no
> los días que la escuela abre. Eso último se declara aparte y se valida en la
> aplicación, porque cambia de una institución a otra y una constraint no es
> el lugar para un dato de configuración.

> **`creado_por` es nullable a propósito.** RF-01.9 permite eliminar
> definitivamente una cuenta para liberar su email, y lo asociado a ella pierde
> la referencia en vez de borrarse. Con esta FK `NOT NULL` y sin política de
> borrado, cualquier docente que hubiera creado una reserva recurrente quedaba
> imposible de eliminar: el `DELETE` moría con violación de FK y la API
> devolvía 500.

> **La relación con los equipos no se guarda acá.** Una tabla puente
> `regla_recurrencia ↔ equipo` sería información duplicada que solo se
> escribiría: cancelar "esta y las siguientes" (RF-04.6) se resuelve por
> `reserva_grupo.regla_recurrencia_id`, y qué equipos toca cada ocurrencia ya
> está en sus filas `reserva`.

### `reserva_grupo`
"La reserva" tal como la percibe el docente: una materia, una fecha, un
horario — independientemente de cuántos equipos incluya, y **sin restricción de
que todos pertenezcan al mismo carro**.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| materia_id | UUID | FK → materia.id, NOT NULL |
| creado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| nombre_docente_snapshot | VARCHAR(200) | NOT NULL |
| fecha | DATE | NOT NULL |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin <> hora_inicio) |
| estado | VARCHAR(25) | NOT NULL DEFAULT 'CONFIRMADA', CHECK IN ('CONFIRMADA','PARCIALMENTE_CANCELADA','CANCELADA','FINALIZADA','NO_RETIRADA') |
| regla_recurrencia_id | UUID | FK → regla_recurrencia.id **ON DELETE SET NULL**, NULL |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| recordatorio_enviado_en | TIMESTAMPTZ | NULL |
| aviso_sin_retirar_en | TIMESTAMPTZ | NULL |

```sql
CREATE INDEX idx_reserva_grupo_materia    ON reserva_grupo (materia_id);
CREATE INDEX idx_reserva_grupo_creado_por ON reserva_grupo (creado_por);
CREATE INDEX idx_reserva_grupo_regla      ON reserva_grupo (regla_recurrencia_id) WHERE regla_recurrencia_id IS NOT NULL;

-- El barrido de recordatorios: las que todavía no se avisaron (RF-08.11).
CREATE INDEX idx_grupo_sin_recordar ON reserva_grupo (fecha, hora_inicio)
    WHERE recordatorio_enviado_en IS NULL AND estado = 'CONFIRMADA';

-- El aviso de "todavía no las retiraste" (RF-08.20). Mismo patrón que el
-- anterior y por la misma razón: el barrido corre cada cinco minutos y no
-- puede recorrer la tabla entera para descartar las que ya avisó.
CREATE INDEX idx_grupo_sin_aviso_retiro ON reserva_grupo (fecha, hora_inicio)
    WHERE aviso_sin_retirar_en IS NULL AND estado = 'CONFIRMADA';
```

> `creado_por` en `SET NULL`: si el usuario se elimina definitivamente, la
> reserva sigue existiendo con su `nombre_docente_snapshot` intacto.

> `recordatorio_enviado_en` es un **instante y no un booleano** porque lo
> primero que se pregunta cuando alguien dice "no me llegó" es a qué hora
> salió. Vive en el grupo y no en cada `reserva`: el recordatorio es uno por
> clase, no uno por equipo.

> `aviso_sin_retirar_en` es la marca del aviso de los 15 minutos (RF-08.20) y
> sigue el mismo criterio, por las mismas dos razones. Son **dos columnas y no
> un contador** porque son dos avisos distintos con condiciones distintas: el
> recordatorio sale siempre, una hora antes; este solo si a los quince minutos
> del inicio **no salió ninguna** máquina de esa reserva. Que la liberación
> (RF-08.10) no tenga marca propia no es un olvido: no avisa nada, y su
> idempotencia ya la da el estado `NO_RETIRADA`, que es terminal.

**Regla de recálculo de `estado`** (se ejecuta cada vez que cambia el estado de
una `reserva` hija):

- Todas sus `reserva` en `CANCELADA` → grupo `CANCELADA`.
- Al menos una `CANCELADA` pero no todas → grupo `PARCIALMENTE_CANCELADA`.
- Todas en `FINALIZADA` (ninguna cancelada) → grupo `FINALIZADA`.
- **Ninguna** retirada dentro del plazo de gracia → grupo `NO_RETIRADA`; si se
  retiró alguna, el grupo no se marca (el docente vino).
- Ninguna cancelada ni finalizada aún → grupo `CONFIRMADA`.

### `reserva`
La ocupación de **un** equipo en **una** franja: es la unidad que protege la
constraint de anti-solapamiento. Para las reservas normales es "un equipo
dentro de un grupo"; para un bloqueo administrativo es una fila suelta, sin
grupo ni materia.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| reserva_grupo_id | UUID | FK → reserva_grupo.id **ON DELETE CASCADE**, NULL en los BLOQUEO |
| equipo_id | UUID | FK → equipo.id, NOT NULL |
| materia_id | UUID | FK → materia.id, NULL en los BLOQUEO |
| nombre_docente_snapshot | VARCHAR(200) | NULL en los BLOQUEO |
| fecha | DATE | NOT NULL |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin <> hora_inicio) |
| estado | VARCHAR(15) | NOT NULL DEFAULT 'CONFIRMADA', CHECK IN ('CONFIRMADA','CANCELADA','FINALIZADA','NO_RETIRADA') |
| tipo | VARCHAR(20) | NOT NULL DEFAULT 'NORMAL', CHECK IN ('NORMAL','BLOQUEO') |
| motivo_bloqueo | TEXT | NULL en las NORMAL; NOT NULL y no vacío en las BLOQUEO |
| creado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| cancelado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| motivo_cancelacion | TEXT | NULL |
| cancelada_en | TIMESTAMPTZ | NULL |
| avisado_equipo_no_disponible_en | TIMESTAMPTZ | NULL |

```sql
-- Invariante: NORMAL siempre pertenece a un grupo y a una materia;
-- BLOQUEO nunca pertenece a un grupo ni a una materia, y siempre dice por qué.
ALTER TABLE reserva ADD CONSTRAINT chk_reserva_tipo_coherente CHECK (
  (tipo = 'NORMAL' AND reserva_grupo_id IS NOT NULL AND materia_id IS NOT NULL
   AND motivo_bloqueo IS NULL)
  OR
  (tipo = 'BLOQUEO' AND reserva_grupo_id IS NULL AND materia_id IS NULL
   AND motivo_bloqueo IS NOT NULL AND btrim(motivo_bloqueo) <> '')
);

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- La garantía que sostiene todo el sistema: dos reservas confirmadas no
-- pueden pisarse sobre el mismo equipo. Es una constraint y no una
-- validación en Go a propósito — una validación se puede ganar por carrera y
-- esto no, y vale además contra cualquier cosa que escriba directo en la base.
--
-- Se usa `fecha + hora_inicio` (aritmética DATE + TIME, pura) en vez de
-- concatenar texto y castear: un EXCLUDE es un índice GiST por debajo y
-- Postgres exige que sus expresiones sean IMMUTABLE. El cast de texto a
-- timestamp depende de DateStyle, así que es STABLE y la constraint se
-- rechaza con "functions in index expression must be marked IMMUTABLE".
--
-- El WHERE deja fuera las canceladas, finalizadas y no retiradas: una franja
-- que se liberó tiene que poder volver a reservarse.
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
  EXCLUDE USING gist (
    equipo_id WITH =,
    tsrange(fecha + hora_inicio, fecha + hora_fin) WITH &&
  )
  WHERE (estado = 'CONFIRMADA');

CREATE INDEX idx_reserva_equipo_fecha ON reserva (equipo_id, fecha);
CREATE INDEX idx_reserva_creado_por   ON reserva (creado_por);
CREATE INDEX idx_reserva_grupo        ON reserva (reserva_grupo_id) WHERE reserva_grupo_id IS NOT NULL;
CREATE INDEX idx_reserva_materia      ON reserva (materia_id) WHERE materia_id IS NOT NULL;

-- El mostrador arma su pantalla con las confirmadas de hoy (RF-08.15).
CREATE INDEX idx_reserva_confirmadas_del_dia ON reserva (fecha, hora_inicio) WHERE estado = 'CONFIRMADA';
```

> `fecha`/`hora_inicio`/`hora_fin` se duplican de `reserva_grupo` hacia
> `reserva` a propósito: el `EXCLUDE` necesita filtrar por `equipo_id` + rango
> horario sin depender de un JOIN.

> `reserva_grupo_id` en `CASCADE`: al archivar un ciclo lectivo basta con
> `DELETE FROM reserva_grupo WHERE materia_id IN (...)` — las filas `reserva`
> hijas se van solas, sin necesitar dos sentencias en el orden correcto.

> **Por qué `BLOQUEO` no lleva `materia_id` ni `reserva_grupo_id`:** no es la
> reserva de un docente para dar clase, es un Admin tomando equipos concretos
> en un rango horario. Forzarlo a pertenecer a una materia no significaría
> nada.
>
> **Y por qué en cambio SÍ lleva `motivo_bloqueo`, obligatorio:** ese es el
> lugar donde una reserva normal tiene su materia. Sin él, un bloqueo es un
> rato ocupado sin explicación — y el caso más común es bloquear con
> anticipación, cuando todavía no hay ninguna reserva que cancelar y por lo
> tanto ningún `motivo_cancelacion` donde el porqué pudiera quedar escrito. El
> `CHECK` lo exige en los dos sentidos: obligatorio en los `BLOQUEO`,
> prohibido en las `NORMAL`, para que no haya dos lugares donde decir para qué
> es una franja.

> `avisado_equipo_no_disponible_en` va en `reserva` y no en `prestamo`, y **es
> la diferencia que importa**: la misma máquina demorada toda la mañana le
> falta a la clase de las 10 y también a la de las 12, y a las dos hay que
> avisarles. Con la marca del lado del préstamo, solo se enteraría la primera.

### `prestamo`
La custodia física de un equipo: quién lo tiene **ahora**. Ver RF-08.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| equipo_id | UUID | FK → equipo.id, NOT NULL |
| reserva_id | UUID | FK → reserva.id **ON DELETE SET NULL**, NULL = préstamo espontáneo |
| entregado_a_usuario_id | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| entregado_a_nombre | VARCHAR(200) | NOT NULL, CHECK no vacío y sin espacios al borde |
| retirado_por | VARCHAR(200) | NULL, CHECK no vacío y sin espacios al borde |
| motivo | TEXT | NULL |
| devolucion_estimada | TIMESTAMPTZ | NULL = no se pactó hora |
| entregado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| entregado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| devuelto_en | TIMESTAMPTZ | **NULL = la máquina sigue afuera**, CHECK ≥ `entregado_en` |
| recibido_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| observaciones | TEXT | NULL |
| avisado_demora_en | TIMESTAMPTZ | NULL |
| avisado_cierre_para | DATE | NULL |

```sql
CREATE UNIQUE INDEX ux_prestamo_abierto ON prestamo (equipo_id) WHERE devuelto_en IS NULL;
CREATE INDEX idx_prestamo_abiertos ON prestamo (entregado_en) WHERE devuelto_en IS NULL;
CREATE INDEX idx_prestamo_equipo   ON prestamo (equipo_id, entregado_en DESC);
CREATE INDEX idx_prestamo_reserva  ON prestamo (reserva_id) WHERE reserva_id IS NOT NULL;
```

> **`prestamo` no es `reserva`, y esa es la razón de que exista la tabla.** La
> reserva es el derecho a usar un equipo en una franja; el préstamo es dónde
> está la máquina. Existen por separado: hay reservas que nadie vino a buscar,
> préstamos sin reserva (alguien pide una máquina para un trámite) y préstamos
> que sobreviven a su reserva (la clase terminó y el equipo no volvió).

> **El índice único parcial es la garantía que el papel no puede dar:** un
> equipo no puede tener dos préstamos abiertos. Es parcial
> (`WHERE devuelto_en IS NULL`) para conservar el historial completo — la misma
> máquina prestada cien veces son cien filas, pero como mucho una abierta.

> **No hay ninguna columna en `equipo` que diga "prestado".** El estado se
> deriva de si existe un préstamo abierto, por la misma razón que el contador
> de licencias no se guarda: lo que se duplica se desincroniza.

> **`entregado_a_nombre` va siempre, aunque haya `entregado_a_usuario_id`.** Es
> un snapshot, igual que `reserva_grupo.nombre_docente_snapshot`: si la cuenta
> se elimina definitivamente (RF-01.9), el registro tiene que seguir diciendo
> quién se llevó la máquina. Y el usuario es opcional porque quien pide un
> equipo para un trámite puede no tener cuenta.

> **`retirado_por` se guarda al lado de `entregado_a_nombre`, no en su lugar**
> (RF-08.19). Quien responde por el equipo es el docente que reservó; que mande
> a un alumno o a un colega a buscarlo es lo habitual y se anota aparte. Es
> texto libre y no una FK porque casi nunca es alguien con cuenta.

> **`reserva_id` en `SET NULL` y no `CASCADE`:** al archivar un ciclo lectivo
> se borran físicamente sus reservas (§3), y el registro de que alguien se
> llevó una máquina vale por sí mismo aunque la reserva que lo originó ya no
> exista. Mismo criterio que `notificacion.reserva_id`.

> **Las marcas del barrido.** Las dos existen para que un aviso salga una sola
> vez por préstamo: `avisado_demora_en` para el reclamo de devolución y
> `avisado_cierre_para` para el corte de fin de jornada. Lo único que importa
> es si están puestas o no; el instante que guardan sirve para saber cuándo
> salió, no para decidir si vuelve a salir.

### `notificacion`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| reserva_id | UUID | FK → reserva.id **ON DELETE SET NULL**, NULL |
| sobre_usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NULL |
| mensaje | TEXT | NOT NULL |
| tipo | VARCHAR(30) | NOT NULL DEFAULT 'GENERAL', CHECK IN ('GENERAL','DOCENTE_PENDIENTE','RESERVA_CANCELADA','LICENCIA_POR_VENCER','RESERVA_POR_COMENZAR','RESERVA_NO_RETIRADA','EQUIPO_SIN_DEVOLVER','PEDIDO_DE_LIBERACION') |
| estado | VARCHAR(10) | NOT NULL DEFAULT 'NO_LEIDA', CHECK IN ('NO_LEIDA','LEIDA') |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| leida_en | TIMESTAMPTZ | NULL |

```sql
CREATE INDEX idx_notif_usuario_estado ON notificacion (usuario_id, estado);
CREATE INDEX idx_notif_sobre_usuario  ON notificacion (sobre_usuario_id, tipo) WHERE sobre_usuario_id IS NOT NULL;
```

> `usuario_id` en `CASCADE`: acá el destinatario es la razón de ser de la fila
> — si su cuenta se elimina, sus notificaciones no le sirven a nadie más.
>
> `sobre_usuario_id` es **de quién habla** el aviso, no a quién va: un Admin
> recibe "hay una cuenta esperando aprobación" y esta columna dice cuál. Es lo
> que permite cerrar ese aviso cuando la cuenta se resuelve, sin leer el texto
> del mensaje. `CASCADE` por lo mismo: si esa cuenta se elimina, el aviso sobre
> ella deja de tener sentido.
>
> `reserva_id` apunta a la fila `reserva` (un equipo puntual), no al grupo —
> así el aviso puede ser específico aunque el resto del grupo siga confirmado.
> Va en `SET NULL` **por necesidad, no por prolijidad**: al archivar un ciclo
> sus `reserva` se eliminan físicamente (§3), y sin esta política el `DELETE`
> fallaría por violación de FK. El `mensaje` ya tiene el texto completo, así
> que la notificación sigue siendo legible con la referencia en `NULL`.

> `tipo` **sí** es un enum, a diferencia del tipo de equipo o la categoría de
> falla: sobre él decide el sistema —qué acción ofrecer, qué avisos
> deduplicar—, no la institución. La pantalla elige el botón por este campo y
> nunca leyendo el mensaje: cambiar una redacción no puede romper una acción.

> **`RESERVA_NO_RETIRADA` cambió de momento, no de nombre.** Antes constataba una
> liberación ya hecha; ahora es el aviso de los 15 minutos que advierte que va a
> pasar (RF-08.20). El nombre sigue describiendo el hecho —esa reserva no se
> retiró— y la pantalla lo lleva al mismo lugar, así que reusar el valor evita
> una migración del `CHECK` y, sobre todo, evita dejar un valor huérfano que
> ninguna fila nueva volvería a usar. Las notificaciones viejas conservan su
> texto: el `mensaje` es lo que la persona lee, y no se reescribe.

> **El pedido de liberación (RF-04.12) no tiene tabla propia, y esta es.** Un
> pedido es un mensaje: no cambia ninguna reserva, no espera respuesta dentro
> del sistema y no le da prioridad a nadie sobre ningún equipo. Modelarlo como
> entidad obligaría a inventarle estados y caducidad para representar un acuerdo
> que se cierra hablando. La fila que ya se escribe —la notificación del dueño—
> alcanza para las dos cosas que sí hacen falta: dejar constancia de que se pidió
> y sostener la regla de **un pedido por reserva, por solicitante y por día**, que
> se verifica preguntando si existe una notificación `PEDIDO_DE_LIBERACION` con
> este `reserva_id`, este `sobre_usuario_id` y `creada_en` de hoy. La consulta cae
> sobre `idx_notif_sobre_usuario`, que ya existe: no hace falta ningún índice
> nuevo.
>
> `sobre_usuario_id` es de quién **habla** el aviso, y acá eso es **quien pide**,
> no el destinatario: el aviso le llega al dueño y trata sobre el otro docente.
> Es el mismo uso que en "hay una cuenta esperando aprobación".

### `preferencia_email`
Qué copias por correo quiere recibir cada persona (RF-05.13). Guarda el canal,
nunca el aviso: lo que se apaga acá sigue apareciendo en la campana.

| Campo | Tipo | Restricciones |
|---|---|---|
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL, PK (con categoria) |
| categoria | VARCHAR(30) | NOT NULL, PK (con usuario_id), CHECK con las 15 configurables (9 personales + 6 de administración) |
| activa | BOOLEAN | NOT NULL |

> **La ausencia de fila no es "apagado": es "todavía no eligió".** Ahí manda el
> valor por defecto de la categoría, que vive en el código
> (`domain.CategoriaEmail.ActivaPorDefecto`) y no en la base — es una decisión
> de producto que se lee mejor al lado de la lista de categorías que repartida
> en un `DEFAULT` por columna.
>
> Por eso se guardan también las **apagadas**. Si la tabla tuviera únicamente
> lo tildado, destildar un aviso que arranca encendido sería indistinguible de
> no haber abierto nunca el panel, y volvería a encenderse en la lectura
> siguiente. Guardar el panel escribe una fila por categoría de una vez.
>
> **Los dos correos de la cuenta no están en el `CHECK` y no pueden estar.** El
> código de recuperación y el "ya podés entrar" salen siempre; se muestran en
> el panel tildados y sin casilla, y que la base rechace la fila es la última
> garantía de que nadie los apague por accidente.
>
> **Solo se escriben las categorías que esa persona podía elegir.** A un
> docente no se le guarda una fila apagada por cada aviso de administración:
> el día que lo asciendan tiene que recibir los defaults de esas categorías, y
> una fila en `false` se lo impediría para siempre sin que nadie recuerde por
> qué.
>
> Sin índice propio: la PK `(usuario_id, categoria)` sirve a las tres consultas
> que existen —las de una persona, el `LEFT JOIN` por categoría con el que se
> arma la lista de Admin destinatarios, y la búsqueda por email con la que cada
> correo personal se pregunta si sale—.
>
> `CASCADE` por lo mismo que `notificacion`: la preferencia no significa nada
> sin la cuenta que la eligió.

> **Las categorías no son los `notificacion.tipo`.** Son dos listas parecidas y
> conviene no confundirlas: el `tipo` clasifica un aviso interno para decidir
> qué botón ofrece la pantalla, y la `categoria` agrupa por "de qué me avisa
> este mail", que es lo que una persona puede querer tildar. Un solo correo
> —el corte de jornada— resume varios avisos; varios `tipo` no tienen correo
> ninguno; y dos correos de esta tabla (los de la cuenta) no tienen `tipo`
> porque no generan aviso interno.

### `horario_admin`
Patrón semanal recurrente de presencia en el laboratorio — puramente
informativo (RF-07), no afecta permisos ni reservas.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| dia_semana | VARCHAR(10) | NOT NULL, CHECK (`LUNES`…`VIERNES`) |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin > hora_inicio) |

```sql
CREATE INDEX idx_horario_admin_usuario ON horario_admin (usuario_id);
```

> Sin versionado ni `vigente_desde`/`vigente_hasta`: a diferencia de
> `regla_recurrencia` —que materializa una fila por ocurrencia, porque cada una
> puede reservarse o cancelarse individualmente— acá no hay nada que
> materializar. Es un patrón que se evalúa contra el día y la hora de la
> consulta, así que editar un bloque cambia el patrón de inmediato para todas
> las semanas futuras.

> **Dos bloques del mismo Admin no pueden pisarse el mismo día** (RF-07.7), y
> esa regla vive en el servicio y no en la base: garantizarla con una
> constraint pediría `btree_gist` sobre un rango de `TIME`, que es mucha
> maquinaria para algo puramente informativo. Dos bloques que se tocan en el
> borde —uno termina 12:00, el otro empieza 12:00— no se pisan: es el caso más
> común, el turno de la mañana y el de la tarde.

### `horario_admin_excepcion`
Cubre tanto una excepción planificada (horario distinto un día puntual) como el
botón rápido "marcarme no disponible ahora" — son la misma fila, con
`fecha = hoy` en el segundo caso.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| fecha | DATE | NOT NULL |
| tipo | VARCHAR(20) | NOT NULL, CHECK IN ('NO_DISPONIBLE','HORARIO_MODIFICADO') |
| hora_inicio | TIME | NULL (solo si `tipo = HORARIO_MODIFICADO`) |
| hora_fin | TIME | NULL (solo si `tipo = HORARIO_MODIFICADO`, CHECK hora_fin > hora_inicio) |
| motivo | TEXT | NULL |
| | | UNIQUE (usuario_id, fecha) |

```sql
ALTER TABLE horario_admin_excepcion ADD CONSTRAINT chk_excepcion_horario_coherente CHECK (
  (tipo = 'NO_DISPONIBLE'      AND hora_inicio IS NULL     AND hora_fin IS NULL)
  OR
  (tipo = 'HORARIO_MODIFICADO' AND hora_inicio IS NOT NULL AND hora_fin IS NOT NULL)
);

CREATE INDEX idx_horario_excepcion_usuario_fecha ON horario_admin_excepcion (usuario_id, fecha);
```

**Cálculo de "¿disponible ahora?"** (resuelto en el momento de la consulta, no
almacenado):

1. Buscar la excepción de ese Admin con `fecha = hoy`.
   - `tipo = NO_DISPONIBLE` → no disponible, sin importar el patrón semanal.
   - `tipo = HORARIO_MODIFICADO` → comparar la hora actual contra el horario de
     la excepción.
2. Si no hay excepción para hoy → comparar la hora actual contra los bloques de
   `horario_admin` de ese día de semana; alcanza con caer dentro de alguno.
3. `UNIQUE(usuario_id, fecha)` garantiza que no haya ambigüedad: como máximo
   una excepción por Admin por día.

### `audit_log`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | NOT NULL, **sin FK** |
| accion | VARCHAR(100) | NOT NULL |
| entidad | VARCHAR(50) | NOT NULL |
| entidad_id | UUID | NULL |
| detalle | JSONB | NULL |
| ip_origen | INET | NULL |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

```sql
CREATE INDEX idx_audit_usuario ON audit_log (usuario_id, creado_en DESC);
```

> **Sin FK a `usuario`, a propósito**: si una cuenta se elimina, lo que hizo
> tiene que seguir registrado. Y `accion` es texto libre y no un enum porque
> los valores guardados son el nombre que tenía una operación **en su
> momento**: reescribir un registro de auditoría es precisamente lo que un
> registro de auditoría no debe permitir. El catálogo de acciones está en
> `09-seguridad-rbac.md` §5.

### `foto_de_perfil`
| Campo | Tipo | Restricciones |
|---|---|---|
| usuario_id | UUID | PK, FK → usuario ON DELETE CASCADE |
| contenido | BYTEA | NOT NULL |
| tipo | VARCHAR(20) | NOT NULL, CHECK image/webp \| image/jpeg \| image/png |
| actualizada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

> **En una tabla aparte y no como columna de `usuario`**: la imagen pesa
> cientos de veces más que el resto de la fila y se lee en una sola pantalla.
> Adentro, cada listado de usuarios y cada JOIN que resuelve el nombre de un
> docente se la habría llevado puesta.
>
> **El tipo se deduce de los bytes del archivo, no de lo que declare quien
> sube** (ver `internal/auth/domain/foto_de_perfil.go`). La lista es cerrada y
> sin SVG: un SVG puede traer JavaScript adentro y se serviría desde el mismo
> origen que la aplicación, o sea con acceso a la sesión de quien lo mire.
>
> El `ON DELETE CASCADE` es lo que hace que borrar una cuenta se lleve su
> foto. Guardarlas en disco dejaría archivos huérfanos, fuera de la copia de
> seguridad —que hoy es solo de Postgres— y sin forma de saber de quién eran.

### `pedido_de_materia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | NOT NULL, FK → usuario ON DELETE CASCADE |
| materia_id | UUID | NULL, FK → materia ON DELETE CASCADE |
| curso_solicitado | VARCHAR(100) | NULL |
| materia_solicitada | VARCHAR(100) | NULL |
| motivo | TEXT | NOT NULL, no vacío |
| estado | VARCHAR(20) | NOT NULL, CHECK PENDIENTE \| APROBADO \| RECHAZADO |
| respuesta | TEXT | NULL |
| resuelto_por | UUID | NULL, FK → usuario ON DELETE SET NULL |
| resuelto_en | TIMESTAMPTZ | NULL |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

```sql
CREATE UNIQUE INDEX idx_pedido_materia_abierto
    ON pedido_de_materia (usuario_id, materia_id)
    WHERE estado = 'PENDIENTE' AND materia_id IS NOT NULL;
```

Un docente pide poder dictar —y por lo tanto reservar computadoras para— una
materia más. Al registrarse ya se podía decir qué materia se dicta
(`usuario.materia_solicitada`); esto es lo mismo pero repetible, porque a
mitad de año a alguien le asignan una materia nueva.

> **Dos formas de pedir, excluyentes** (`chk_pedido_una_forma`): o se elige
> una materia que existe (`materia_id`), o se escribe una que todavía no está
> cargada (`materia_solicitada` + `curso_solicitado`, texto libre igual que en
> el registro). Con las dos —o con ninguna— no hay forma de saber qué se pidió.
>
> **El índice único es parcial**: vale solo mientras el pedido esté sin
> resolver. Apretar dos veces el botón mandaba dos avisos a todos los Admin
> por lo mismo; volver a pedir el año que viene es legítimo.
>
> **La aprobación es una decisión humana y el sistema no la automatiza.**
> Aceptar habilita a reservar los mismos equipos que usa quien ya dicta esa
> materia (tocarle las reservas no puede: eso está prohibido en
> `reservation`). Si el pedido corresponde se sabe hablando con la persona o
> con los directivos. El sistema deja el pedido escrito con su motivo, le
> avisa a quien ya la dicta para que no se entere tarde, y guarda en
> `audit_log` quién resolvió qué.

### `sugerencia` (el hilo)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | NOT NULL, FK → usuario ON DELETE CASCADE |
| tipo | VARCHAR(20) | NOT NULL, CHECK AYUDA \| PROBLEMA \| SUGERENCIA |
| asunto | VARCHAR(150) | NOT NULL, no vacío |
| pantalla | VARCHAR(200) | NULL |
| version | VARCHAR(20) | NULL |
| estado | VARCHAR(20) | NOT NULL, CHECK ABIERTA \| RESUELTA |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| ultima_actividad_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

### `sugerencia_mensaje` (cada intervención)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| sugerencia_id | UUID | NOT NULL, FK → sugerencia ON DELETE CASCADE |
| autor_id | UUID | NULL, FK → usuario ON DELETE SET NULL |
| de_admin | BOOLEAN | NOT NULL |
| texto | TEXT | NOT NULL, no vacío |
| escrito_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

La bandeja de soporte (RF-09): por acá se pide ayuda, se cuenta que algo del
sistema no anda y se proponen cambios. Es **una conversación**, no un mensaje
con una respuesta.

> **No hay `texto` en el hilo.** El mensaje inicial es la primera fila de
> `sugerencia_mensaje` y no tiene nada de distinto salvo ser el primero.
> Guardarlo aparte obligaría a leer dos lugares para mostrar una conversación,
> y a decidir en cuál de los dos vive una respuesta.
>
> **`de_admin` se guarda, no se deduce del rol del autor.** Si a un docente lo
> ascienden a `ADMIN`, lo que escribió antes lo escribió como docente y el hilo
> tiene que seguir leyéndose igual. `autor_id` en `SET NULL` porque borrar
> media conversación deja la otra media sin sentido.
>
> **`ultima_actividad_en` es por lo que se ordena la bandeja**, y por eso
> existe además de `creada_en`: un hilo de la semana pasada al que le acaban de
> escribir es el que tiene a alguien esperando.
>
> **El `tipo` decide algo más que el orden**: los correos de un `AYUDA` no se
> pueden desactivar (RF-09.5), los de los otros dos sí.
>
> **No confundir con `incidencia`**: aquella es una computadora que no
> arranca —marca el equipo y lo saca de circulación—; esto es una conversación
> con una persona. Lo resuelve gente distinta.
>
> **`pantalla` y `version` las completa la aplicación, no la persona.** Un "no
> anda" sin saber desde dónde se escribió obliga a ir a buscar a quien lo
> escribió para preguntarle qué estaba haciendo, y con alguien que ya se
> sintió torpe usando el sistema esa conversación no vuelve a pasar.
>
> **Contestar no cierra el hilo**, a diferencia de como era antes: cerrar es un
> acto aparte y un mensaje de quien preguntó lo reabre. Una respuesta que
> cerraba dejaba sin manera de decir "ya probé y no anda".

## 3. Archivado de ciclo lectivo: qué se borra y qué se preserva

Archivar un ciclo lectivo **no es un soft-delete de las reservas** — es un
borrado real. El propósito de archivar es evitar recrear cursos, materias y
asignaciones docente↔materia el año siguiente; **no** es preservar el detalle
de cada reserva.

1. Se preservan (`archivado = true`, sin borrar): `curso`, `materia`,
   `docente_materia`.
2. Antes de borrar nada, se calcula y persiste un **snapshot histórico
   agregado** (`historico_uso_equipo` / `historico_uso_docente`) con las
   estadísticas del año que termina.
3. Se **eliminan físicamente**: todos los `reserva_grupo`, `reserva` y
   `regla_recurrencia` cuya `materia_id` pertenece a un curso de ese ciclo, más
   los bloqueos administrativos (`reserva` con `tipo = BLOQUEO`) del año de ese
   ciclo — no tienen materia, así que se ubican por año.
4. `incidencia` **no se toca**: pertenece al equipo, no al ciclo lectivo.
   `prestamo` tampoco — su `reserva_id` queda en `NULL` y el registro de que
   alguien se llevó la máquina sobrevive.

### `historico_uso_equipo` (permanente, uno por equipo por año)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL |
| equipo_id | UUID | FK → equipo.id, NOT NULL |
| etiqueta_snapshot | VARCHAR(100) | NOT NULL |
| identificador_snapshot | INTEGER | NULL |
| carro_nombre_snapshot | VARCHAR(100) | NULL |
| minutos_reservados | INTEGER | NOT NULL |
| cantidad_reservas | INTEGER | NOT NULL |
| | | UNIQUE (anio, equipo_id) |

> **`etiqueta_snapshot` es el nombre que se muestra**, congelado: "PC 7" o el
> nombre del equipo suelto. El identificador y el carro son nullable porque un
> equipo que no está en ningún carro no tiene ninguno de los dos — armar el
> rótulo a partir de ellos dejaría al proyector archivado como "PC 0 ()".

### `historico_uso_docente` (permanente, uno por docente por año)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL |
| usuario_id | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| nombre_docente_snapshot | VARCHAR(200) | NOT NULL |
| cantidad_reservas | INTEGER | NOT NULL |
| minutos_totales | INTEGER | NOT NULL |
| | | UNIQUE (anio, usuario_id) |

```sql
CREATE INDEX idx_historico_equipo_anio  ON historico_uso_equipo (anio);
CREATE INDEX idx_historico_docente_anio ON historico_uso_docente (anio);
```

> `usuario_id` queda en `NULL` si esa cuenta se elimina físicamente (RF-01.9);
> la fila sobrevive y sigue siendo legible por `nombre_docente_snapshot`. Sin
> esa política, el hard delete de un docente que figurara en cualquier snapshot
> fallaría con violación de FK.

> Estas dos tablas **sí** son un read-model permanente (a diferencia de la
> decisión de "sin CQRS" para reportes del ciclo activo, §4), pero se calculan
> **una sola vez, al archivar**, y no se sincronizan continuamente por eventos.
> Son la única fuente de verdad para años ya cerrados, porque el detalle de
> esos años ya no existe.
>
> **`incidencia` no necesita una tabla histórica equivalente**: nunca se
> elimina al archivar, así que el reporte de incidencias (RF-06.3) siempre se
> resuelve con una consulta directa.

## 4. Reportes: consultas agregadas para el ciclo activo, tablas históricas para años cerrados

A esta escala no hace falta un read-model continuo sincronizado por eventos —
pero sí uno puntual al archivar (§3), ya que el detalle se borra. Los reportes
de RF-06 se resuelven así:

- **Ciclo lectivo activo** (con `reserva`/`reserva_grupo` todavía en la base):
  consultas directas.
- **Ciclos ya archivados**: se leen de `historico_uso_equipo` /
  `historico_uso_docente`.
- **Los que no son de un ciclo** (estado del inventario, equipos fuera de
  circulación, incidencias por categoría — RF-06.5 a RF-06.7): consultas
  directas sobre `equipo` e `incidencia`, siempre. Ninguna de las dos tablas se
  archiva, así que no hay un "año cerrado" del que leerlas ni razón para
  materializarlas: describen lo que hay hoy, no lo que pasó en un período.

```sql
-- Uso por equipo en un rango de fechas (ciclo activo).
-- El JOIN a carro va LEFT: un equipo suelto no tiene carro, y con INNER
-- desaparecería del reporte en vez de aparecer sin carro.
SELECT e.id AS equipo_id, e.identificador, c.nombre AS carro_nombre,
       SUM(EXTRACT(EPOCH FROM (r.hora_fin - r.hora_inicio)) / 60)::int AS minutos_reservados,
       COUNT(*) AS cantidad_reservas
FROM reserva r
JOIN equipo e ON e.id = r.equipo_id
LEFT JOIN carro c ON c.id = e.carro_id
WHERE r.fecha BETWEEN $1 AND $2 AND r.estado NOT IN ('CANCELADA','NO_RETIRADA')
GROUP BY e.id, e.identificador, c.nombre;

-- Uso por docente (ciclo activo).
-- LEFT JOIN y sin filtrar por creado_por: una cuenta eliminada (RF-01.9)
-- deja esa columna en NULL, y sus horas tienen que seguir contando.
SELECT r.creado_por AS usuario_id,
       COALESCE(MAX(u.nombre || ' ' || u.apellido), r.nombre_docente_snapshot, '') AS docente,
       COUNT(*) AS cantidad_reservas,
       SUM(EXTRACT(EPOCH FROM (r.hora_fin - r.hora_inicio)) / 60)::int AS minutos_totales
FROM reserva r
LEFT JOIN usuario u ON u.id = r.creado_por
WHERE r.fecha BETWEEN $1 AND $2 AND r.estado NOT IN ('CANCELADA','NO_RETIRADA')
GROUP BY r.creado_por, r.nombre_docente_snapshot;

-- Uso por equipo en un año ya archivado
SELECT equipo_id, etiqueta_snapshot, minutos_reservados, cantidad_reservas
FROM historico_uso_equipo
WHERE anio = $1;
```

> **`cantidad_reservas` cuenta filas `reserva`, no clases.** Un docente que
> reservó ocho equipos para una clase suma ocho, no una. Es coherente con el
> reporte por equipo —que es de filas— pero no con lo que el docente llama
> "una reserva"; si alguna vez se quiere el número de clases, es
> `COUNT(DISTINCT reserva_grupo_id)`.

> **`NO_RETIRADA` no cuenta como uso** (RF-08.10). Una clase que nadie vino a
> buscar no es una clase dada, y contarla infla justamente el número con el
> que se justifica una compra: un carro que nadie retira figuraría como el
> más usado de la institución.

> **Una cuenta eliminada sigue contando.** El `JOIN` a `usuario` es LEFT y no
> se filtra por `creado_por IS NOT NULL`: al eliminar definitivamente una
> cuenta (RF-01.9) esa columna queda en NULL, y con INNER las horas de esa
> persona desaparecían del año. El nombre sale entonces de
> `nombre_docente_snapshot`, que existe para eso. Por lo mismo se agrupa
> también por esa columna: en un `GROUP BY` los NULL se juntan entre sí, así
> que sin ella todas las cuentas borradas caerían en un único renglón.
>
> En el snapshot, esas filas quedan con `usuario_id` NULL. Como el `UNIQUE
> (anio, usuario_id)` no las deduplica —Postgres no considera iguales a dos
> NULL— el archivado las borra antes de reescribirlas, para que reintentarlo
> no las acumule.

> **Los bloqueos administrativos cuentan.** No tienen materia, así que el
> filtro por ciclo no los alcanza, y se los ata al ciclo por el año de la
> fecha: igual ocupan el equipo, y sin ellos una máquina muy usada para
> exámenes figuraría como poco usada.

> **Toda consulta sobre `equipo` une a `carro` con `LEFT JOIN`**, salvo que la
> pregunta sea explícitamente *por carro* (las incidencias agrupadas por carro
> de RF-06.3). Con `INNER`, un equipo suelto no queda sin nombre: **desaparece
> de la consulta**, que es un modo de fallar mucho peor.

## 5. Notas de diseño

- `equipo.freezado` es informativo (Deep Freeze instalado), sin efecto
  funcional sobre reservas.
- `equipo.dado_de_baja` / `equipo.fecha_baja`: soft delete de inventario — la
  fila se conserva para no perder el historial asociado.
- `usuario.estado` incluye `BAJA` como estado terminal (sin reactivación),
  distinto de `RECHAZADA`.
- `reserva_grupo` / `reserva` / `regla_recurrencia` no persisten
  indefinidamente: se **eliminan físicamente** al archivar el ciclo lectivo de
  su materia (§3), a diferencia de `curso`/`materia`/`docente_materia`, que
  solo se marcan `archivado = true`.
- Lo que la institución escribe es texto libre (`equipo.tipo`,
  `incidencia.categoria`, `reserva.motivo_bloqueo`); lo que el sistema
  interpreta es un enum con `CHECK` (estados, roles, `notificacion.tipo`).
</content>
</invoke>
