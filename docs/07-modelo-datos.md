# Modelo de Datos — SGRC

## 1. Diagrama ER

```mermaid
erDiagram
    USUARIO ||--o{ DOCENTE_MATERIA : asignado
    USUARIO ||--o{ RESERVA_GRUPO : crea
    USUARIO ||--o{ NOTIFICACION : recibe
    USUARIO ||--o{ CODIGO_RECUPERACION : pide
    CARRO ||--o{ PC : contiene
    PC ||--o{ INCIDENCIA : registra
    PC ||--o{ RESERVA : recibe
    CICLO_LECTIVO ||--o{ CURSO : contiene
    CURSO ||--o{ MATERIA : contiene
    MATERIA ||--o{ DOCENTE_MATERIA : asigna
    MATERIA ||--o{ RESERVA_GRUPO : recibe
    MATERIA ||--o{ REGLA_RECURRENCIA : tiene
    REGLA_RECURRENCIA ||--o{ RESERVA_GRUPO : materializa
    RESERVA_GRUPO ||--o{ RESERVA : contiene

    USUARIO { uuid id; string nombre; string apellido; string email; string password_hash; string google_sub; string rol; string estado; timestamp fecha_registro; uuid aprobado_por; string curso_solicitado; string materia_solicitada; int version_sesion }
    CARRO { uuid id; string nombre; string descripcion }
    PC { uuid id; uuid carro_id; int identificador; bigint numero_serie; bool freezado; string cpu; string ram; string sistema_operativo; string software_instalado; string estado; bool dada_de_baja; timestamp fecha_alta }
    INCIDENCIA { uuid id; uuid pc_id; uuid reportado_por; string descripcion; string gravedad; timestamp fecha; bool enviado_dge; timestamp fecha_envio_dge; string estado }
    CICLO_LECTIVO { uuid id; int anio; bool activo; bool archivado }
    CURSO { uuid id; uuid ciclo_lectivo_id; string nombre; bool activo; bool archivado }
    MATERIA { uuid id; uuid curso_id; string nombre; bool activo; bool archivado }
    DOCENTE_MATERIA { uuid id; uuid usuario_id; uuid materia_id; string rol }
    REGLA_RECURRENCIA { uuid id; uuid materia_id; uuid creado_por; string dia_semana; time hora_inicio; time hora_fin; date fecha_inicio; date fecha_fin }
    RESERVA_GRUPO { uuid id; uuid materia_id; uuid creado_por; string nombre_docente_snapshot; date fecha; time hora_inicio; time hora_fin; string estado; uuid regla_recurrencia_id; timestamp creada_en }
    RESERVA { uuid id; uuid reserva_grupo_id; uuid pc_id; string estado; string tipo; timestamp creada_en; uuid cancelado_por; string motivo_cancelacion; timestamp cancelada_en }
    NOTIFICACION { uuid id; uuid usuario_id; uuid reserva_id; string mensaje; string estado; timestamp creada_en; timestamp leida_en }
    CODIGO_RECUPERACION { uuid id; uuid usuario_id; string codigo_hash; timestamp creado_en; timestamp expira_en; timestamp usado_en; int intentos }
```

> **Reservas agrupadas por PC:** un docente no reserva una PC por vez como operaciones independientes — selecciona varias PCs de una lista (como tildar casillas) hasta completar la cantidad que necesita para su clase, en una sola operación. Eso exige separar "la reserva que hace el docente" (`RESERVA_GRUPO`: una materia, una fecha, un horario) de "cada PC dentro de esa reserva" (`RESERVA`: una fila por PC). Cancelaciones en cascada (evaluación estatal, PC fuera de servicio) actúan sobre filas `RESERVA` individuales — nunca sobre todo el grupo salvo que termine afectando a todas sus PCs.

## 2. Una sola base de datos: `sgrc_db`

Todo vive en la misma base y las referencias cruzadas son **foreign keys reales**, con integridad referencial completa.

### Dos clases de tiempo, dos tipos distintos

El esquema distingue a propósito entre **instantes** y **hora de pared**, y confundirlos fue un bug real (ver migración `003`):

- **Instantes** — el momento en que algo pasó: `creada_en`, `cancelada_en`, `leida_en`, `fecha_registro`, `fecha_alta`… Van en **`TIMESTAMPTZ`**. Postgres normaliza a UTC al escribir y devuelve el valor con zona al leer, así que el instante sobrevive el ida y vuelta sin depender de la configuración de nadie.

- **Hora de pared de la institución** — `reserva.fecha` (`DATE`) y `hora_inicio`/`hora_fin` (`TIME`). "De 8 a 9 del 2 de septiembre" significa lo mismo en enero que en julio; no es un instante hasta que se lo combina con la zona de la escuela. Por eso **no** llevan zona, y por eso el proceso lee "ahora" en `APP_TIMEZONE` para compararlos (ver `cmd/main.go`).

> Originalmente los instantes eran `TIMESTAMP` sin zona. Como el proceso lee la hora en la zona de la escuela, lo que quedaba guardado era la hora de pared local; al leerla, pgx la interpretaba como UTC y la API la serializaba con sufijo `Z`. Resultado: un instante real de las 03:31 `-03:00` viajaba como `03:31Z` y el navegador lo mostraba a las 00:31 — tres horas antes. Nadie lo había notado porque ninguna pantalla mostraba marcas de tiempo hasta el listado de notificaciones (RF-05).

### `usuario`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| nombre | VARCHAR(100) | NOT NULL |
| apellido | VARCHAR(100) | NOT NULL |
| email | VARCHAR(150) | UNIQUE, NOT NULL |
| password_hash | VARCHAR(255) | NULL desde la migración `008` — ver más abajo |
| debe_cambiar_password | BOOLEAN | NOT NULL DEFAULT false |
| rol | VARCHAR(10) | NOT NULL, CHECK IN ('ADMIN','DOCENTE') |
| estado | VARCHAR(20) | NOT NULL DEFAULT 'PENDIENTE', CHECK IN ('PENDIENTE','APROBADA','RECHAZADA','BAJA') |
| fecha_registro | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| fecha_aprobacion | TIMESTAMPTZ | NULL |
| aprobado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| curso_solicitado | VARCHAR(100) | NULL — lo que declaró al registrarse (migración `006`) |
| materia_solicitada | VARCHAR(100) | NULL — ídem |
| google_sub | VARCHAR(255) | NULL, UNIQUE parcial — identidad de Google (migración `008`) |
| version_sesion | INTEGER | NOT NULL DEFAULT 0 — revocación de sesiones (migración `010`) |
| | | CHECK `password_hash IS NOT NULL OR google_sub IS NOT NULL` |

> `debe_cambiar_password`: se pone en `true` cuando un Admin resetea la contraseña de alguien (RF-01.6). El login sigue funcionando con la contraseña temporal, pero la respuesta incluye este flag para que el frontend fuerce la pantalla de cambio antes de dejar entrar al resto del sistema; al cambiarla exitosamente (`POST /api/auth/cambiar-password`), vuelve a `false`. Sin esta columna, "exigir cambio en el próximo login" (como decía `01-requisitos.md` RF-01.6) no tenía ningún mecanismo real que lo hiciera cumplir.

> `version_sesion` (migración `010`) es lo que permite **cerrar las sesiones abiertas** al cambiar una contraseña (RF-01.11). El número viaja dentro del JWT y el middleware lo compara contra el de la fila en cada request — en la misma consulta que ya hacía para verificar el estado, así que no agrega ninguna. Cambiar la contraseña lo incrementa y todo token anterior deja de valer.
>
> Es un entero y no un timestamp ("invalidar lo emitido antes de X") porque `iat` en un JWT tiene resolución de segundos: comparado contra un `now()` con microsegundos, el token que el propio cambio acaba de emitir se rechazaría a sí mismo, y redondear al segundo deja una ventana en la que las sesiones abiertas en ese mismo segundo sobreviven. El `DEFAULT 0` coincide con el claim ausente, así que aplicar la migración no desloguea a nadie.

> `curso_solicitado` y `materia_solicitada` son **texto libre, no FKs** (migración `006`): al registrarse la persona todavía no está autenticada, así que no puede elegir de una lista, y lo que declara puede no existir todavía en el sistema — de hecho el Admin quizás lo tenga que crear al aprobarla (RF-02.6). Es una declaración de intención para que el Admin sepa a qué asignarla, no un vínculo.

> Se agrega el estado `BAJA`, distinto de `RECHAZADA`. `RECHAZADA` es para una solicitud de registro que nunca se aprobó; `BAJA` es para una cuenta que **estuvo** `APROBADA` y luego se dio de baja — la distinción importa para no mezclar "nunca llegó a entrar" con "entró y se fue" en reportes y auditoría. **`BAJA` es terminal: no hay transición de vuelta a `APROBADA`** (la API rechaza con 409 cualquier intento de cambiar el estado de una cuenta que ya está en `BAJA`). Si la persona vuelve a la institución, se registra como cuenta nueva — no se reactiva la vieja. Como `email` es `UNIQUE`, si quiere reusar el mismo email, el `ADMIN` debe primero eliminar (hard delete) la fila `usuario` en `BAJA`; el sistema no lo hace automáticamente para no perder la referencia de auditoría sin que un admin lo decida explícitamente. `aprobado_por` usa `SET NULL` porque si el usuario que aprobó a otro es eliminado más adelante, no tiene sentido bloquear ni arrastrar ese borrado — se pierde solo el dato de "quién aprobó", no la cuenta aprobada.

> `google_sub` y `password_hash` son **independientes entre sí, pero no las dos vacías** (migración `008`). Una cuenta creada con Google no tiene contraseña —a quien la verifica es Google— y una cuenta con contraseña no tiene `google_sub`; pero un docente que se registró con contraseña y después entra con Google queda con las dos y no pierde la que tenía. Por eso son dos columnas nullable con un `CHECK` que exige al menos una, y no un enum `proveedor`: ese enum obligaría a elegir una sola forma de ingreso e inventar un tercer valor `AMBAS` que no significa nada distinto de "las dos columnas están llenas". El `CHECK` es la red que impide una fila por la que no se pueda entrar de ninguna manera.
>
> Se guarda el claim `sub` del ID token y no el email porque el email de una cuenta de Google **puede cambiar y el `sub` no**: quien ya entró alguna vez sigue entrando a su misma cuenta aunque Google le haya cambiado la dirección. El email sigue siendo la identidad dentro del sistema (es por donde el Admin reconoce a la persona); el vínculo con Google cuelga del `sub`.
>
> El índice único es **parcial** (`WHERE google_sub IS NOT NULL`). Un `UNIQUE` común también dejaría pasar cualquier cantidad de `NULL`, así que las dos formas funcionan; el parcial no indexa las filas de las cuentas con contraseña, que hoy son todas.

```sql
CREATE INDEX idx_usuario_estado ON usuario(estado);
CREATE UNIQUE INDEX idx_usuario_google_sub ON usuario (google_sub) WHERE google_sub IS NOT NULL;
```

### `ciclo_lectivo`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL, UNIQUE |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| archivado | BOOLEAN | NOT NULL DEFAULT false |

```sql
-- Solo puede haber un ciclo activo a la vez (RF-02.1)
CREATE UNIQUE INDEX idx_ciclo_lectivo_activo_unico ON ciclo_lectivo(activo) WHERE activo = true;
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

> `nombre` no es libre: año (`1°`-`6°`) + división (`A`-`Z`), ej. `1°A`, `6°Z`. La validación vive tanto en el `CHECK` de la DB como en la capa de aplicación (para devolver un 400 claro antes de llegar a la constraint).

### `materia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| curso_id | UUID | FK → curso.id **ON DELETE CASCADE**, NOT NULL |
| nombre | VARCHAR(100) | NOT NULL |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| archivado | BOOLEAN | NOT NULL DEFAULT false |
| | | UNIQUE (curso_id, nombre) |

> **Edición y eliminación de `curso`/`materia` mientras el ciclo está activo (RF-02.11):** `nombre` se puede editar en cualquier momento (validando de nuevo el patrón `^[1-6]°[A-Z]$` para `curso`, y la unicidad `(ciclo_lectivo_id, nombre)` / `(curso_id, nombre)` — devolver 400/409 según corresponda). La eliminación (hard delete) solo se permite si no hay ninguna `reserva_grupo` con esa `materia_id` (para `curso`, si ninguna de sus materias tiene reservas) — de lo contrario, la única forma de "sacarlo de circulación" es esperar al archivado del ciclo completo (RF-02.4). Se valida con una query de existencia antes del `DELETE`, no con una constraint de DB. Al eliminar un `curso`, sus `materia` se eliminan en cascada (y con ellas, sus `docente_materia` — ver más abajo); no hace falta borrar materia por materia primero.

### `docente_materia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| materia_id | UUID | FK → materia.id **ON DELETE CASCADE**, NOT NULL |
| rol | VARCHAR(10) | NOT NULL, CHECK IN ('TITULAR','SUPLENTE') |
| | | UNIQUE (usuario_id, materia_id) |

> `usuario_id` en `CASCADE`: si el usuario se elimina definitivamente, el vínculo pierde todo sentido — no hay nada que "preservar" acá como sí pasa con las reservas (que tienen `nombre_docente_snapshot`). En la práctica esto casi nunca dispara solo, porque dar de baja a un docente (RF-02.8) ya elimina explícitamente sus filas `docente_materia` como parte del mismo flujo, antes de que la cuenta llegue a borrarse. `materia_id` en `CASCADE` por la misma razón: al eliminar una materia (RF-02.11) o un curso completo, sus asignaciones de docentes se van con ella.
>
> **Solo pueden asignarse docentes con `estado = APROBADA`** — un `INSERT` en esta tabla valida el estado del `usuario_id` en la capa de aplicación (no hay CHECK cross-tabla en Postgres para esto); asignar a alguien `PENDIENTE`, `RECHAZADA` o `BAJA` no tiene sentido operativo.

```sql
CREATE INDEX idx_docente_materia_materia ON docente_materia(materia_id);
CREATE INDEX idx_docente_materia_usuario ON docente_materia(usuario_id);
CREATE INDEX idx_curso_ciclo ON curso(ciclo_lectivo_id);
CREATE INDEX idx_materia_curso ON materia(curso_id);
```

### `pc`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| carro_id | UUID | FK → carro.id, NOT NULL |
| identificador | INTEGER | NOT NULL |
| numero_serie | BIGINT | UNIQUE, NOT NULL |
| freezado | BOOLEAN | NOT NULL DEFAULT false |
| cpu | VARCHAR(100) | NULL |
| ram | VARCHAR(20) | NULL |
| sistema_operativo | VARCHAR(50) | NULL |
| software_instalado | TEXT | NULL |
| estado | VARCHAR(25) | NOT NULL DEFAULT 'DISPONIBLE', CHECK IN ('DISPONIBLE','EN_MANTENIMIENTO','FUERA_DE_SERVICIO') |
| dada_de_baja | BOOLEAN | NOT NULL DEFAULT false |
| fecha_baja | TIMESTAMPTZ | NULL |
| fecha_alta | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| | | UNIQUE (carro_id, identificador) |

> `identificador` es un número entero (ej: "PC 27"), único **dentro de su carro** — puede repetirse entre carros distintos (el Carro 1 y el Carro 2 pueden tener cada uno una "PC 27"; lo que las distingue es la combinación carro+identificador). `numero_serie` es distinto: es el número de serie real de fábrica, entero largo, **único en toda la tabla** sin importar el carro — dos PCs nunca pueden compartir uno.
>
> `freezado`: indica si la PC tiene Deep Freeze (u otro software equivalente) instalado. Es **metadata informativa** para el admin/técnico — no restringe reservas ni ningún otro flujo. Se muestra a **todos los usuarios autenticados** (no solo Admin): un docente necesita saber, por ejemplo, qué PCs tienen AutoCAD 2007 vs AutoCAD 2027 antes de elegir cuáles reservar — ese dato vive en `software_instalado` (texto libre), y `software_instalado` + `freezado` + `estado` son visibles al listar PCs para cualquier rol.
>
> `dada_de_baja` / `fecha_baja`: el "eliminar" una PC desde la UI del Admin es en realidad un **soft delete** — se oculta de los listados activos y no puede reservarse, pero la fila permanece porque `incidencia` y `reserva` la referencian por FK y esa historia no se pierde. Un hard delete real requeriría borrar en cascada su historial de incidencias y reservas, lo cual no es deseable.

### `carro`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| nombre | VARCHAR(100) | NOT NULL, UNIQUE |
| descripcion | TEXT | NULL |

> `freezado` no es un atributo del carro, es de cada PC individual (ver `pc` arriba) — cada PC de un mismo carro puede tener o no Deep Freeze instalado. El `ADMIN` puede editar `nombre`/`descripcion` de un carro en cualquier momento (`PATCH`); no hay "eliminar carro" en el alcance actual — se elimina indirectamente dando de baja todas sus PCs.

### `incidencia`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| pc_id | UUID | FK → pc.id, NOT NULL |
| reportado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| descripcion | TEXT | NOT NULL |
| gravedad | VARCHAR(10) | NOT NULL, CHECK IN ('LEVE','MODERADA','GRAVE') |
| fecha | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| enviado_dge | BOOLEAN | NOT NULL DEFAULT false |
| fecha_envio_dge | TIMESTAMPTZ | NULL |
| estado | VARCHAR(20) | NOT NULL DEFAULT 'ABIERTA', CHECK IN ('ABIERTA','EN_REPARACION','ENVIADA_DGE','RESUELTA') |

> `reportado_por` en `SET NULL`: el historial de la incidencia (`descripcion`, `gravedad`, fechas) vale por sí mismo aunque se pierda el dato de quién la reportó si esa cuenta se elimina definitivamente más adelante.

```sql
CREATE INDEX idx_pc_carro_estado ON pc(carro_id, estado);
CREATE INDEX idx_incidencia_pc ON incidencia(pc_id);
```

### `regla_recurrencia`
Representa el **patrón** temporal (materia + día de semana + horario + rango de fechas). No guarda las PCs: la relación con las PCs vive en los `reserva_grupo` que la regla materializa, uno por ocurrencia.

`dia_semana` solo admite `LUNES` a `VIERNES` — la semana lectiva de la escuela (ver `01-requisitos.md` RF-04). Lo sostiene un `CHECK` en la base (migración `005`) y no solo el enum de Go: sin él, cualquier `INSERT` que no pase por la aplicación entra igual.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| materia_id | UUID | FK → materia.id, NOT NULL |
| creado_por | UUID | FK → usuario.id, **ON DELETE SET NULL** |
| dia_semana | VARCHAR(10) | NOT NULL, CHECK (`LUNES`…`VIERNES`) |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin > hora_inicio) |
| fecha_inicio | DATE | NOT NULL |
| fecha_fin | DATE | NOT NULL, CHECK (fecha_fin >= fecha_inicio) |

> **`creado_por` es nullable a propósito** (migración `002`). RF-01.9 permite eliminar definitivamente una cuenta en BAJA para liberar su email, y lo asociado a ella "pierde la referencia al usuario" en vez de borrarse. Cuando esta FK era `NOT NULL` sin política de borrado, cualquier docente que hubiera creado una reserva recurrente quedaba imposible de eliminar: el `DELETE` moría con violación de FK y la API devolvía 500.

> **`regla_recurrencia_pc` fue eliminada** en la migración `002`. Existía como tabla puente hacia `pc`, pero solo se escribía y nunca se leía: la cancelación de ocurrencias futuras (RF-04.6) resuelve por `reserva_grupo.regla_recurrencia_id`.

### `reserva_grupo` (nueva)
Es "la reserva" tal como la percibe el docente: una materia, una fecha, un horario — independientemente de cuántas PCs incluya, y **sin restricción de que todas pertenezcan al mismo carro** (un docente puede combinar PCs de carros distintos en la misma reserva si así lo necesita).

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| materia_id | UUID | FK → materia.id, NOT NULL |
| creado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| nombre_docente_snapshot | VARCHAR(200) | NOT NULL |
| fecha | DATE | NOT NULL |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin > hora_inicio) |
| estado | VARCHAR(25) | NOT NULL DEFAULT 'CONFIRMADA', CHECK IN ('CONFIRMADA','PARCIALMENTE_CANCELADA','CANCELADA','FINALIZADA') |
| regla_recurrencia_id | UUID | FK → regla_recurrencia.id, NULL |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

> `creado_por` en `SET NULL`: si el usuario se elimina definitivamente, la reserva sigue existiendo con su `nombre_docente_snapshot` intacto — el mismo patrón que ya se usaba para preservar el nombre aunque la cuenta se dé de baja, ahora también sostenido a nivel de FK para que el `DELETE` del usuario no falle.

**Regla de recálculo de `estado`** (se ejecuta cada vez que cambia el estado de una `reserva` hija):
- Todas sus `reserva` en `CANCELADA` → grupo `CANCELADA` ("la reserva completa", solo si TODAS las PCs quedaron afectadas).
- Al menos una `CANCELADA` pero no todas → grupo `PARCIALMENTE_CANCELADA`.
- Todas en `FINALIZADA` (ninguna cancelada) → grupo `FINALIZADA`.
- Ninguna cancelada ni finalizada aún → grupo `CONFIRMADA`.

```sql
CREATE INDEX idx_reserva_grupo_materia ON reserva_grupo(materia_id);
CREATE INDEX idx_reserva_grupo_creado_por ON reserva_grupo(creado_por);
CREATE INDEX idx_reserva_grupo_regla ON reserva_grupo(regla_recurrencia_id) WHERE regla_recurrencia_id IS NOT NULL;
```

### `reserva` (modificada)
Ahora es "una PC dentro de un grupo de reserva" para las reservas normales, o una fila administrativa suelta para bloqueos de evaluación estatal (que no pertenecen a ningún `reserva_grupo` de un docente).

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| reserva_grupo_id | UUID | FK → reserva_grupo.id **ON DELETE CASCADE**, NULL (ver CHECK abajo) |
| pc_id | UUID | FK → pc.id, NOT NULL |
| materia_id | UUID | FK → materia.id, NULL (ver CHECK abajo) |
| nombre_docente_snapshot | VARCHAR(200) | NULL (solo para NORMAL; ver CHECK abajo) |
| fecha | DATE | NOT NULL |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin > hora_inicio) |
| estado | VARCHAR(15) | NOT NULL DEFAULT 'CONFIRMADA', CHECK IN ('CONFIRMADA','CANCELADA','FINALIZADA') |
| tipo | VARCHAR(20) | NOT NULL DEFAULT 'NORMAL', CHECK IN ('NORMAL','EVALUACION_ESTATAL') |
| creado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| cancelado_por | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| motivo_cancelacion | TEXT | NULL |
| cancelada_en | TIMESTAMPTZ | NULL |

> `reserva_grupo_id` en `CASCADE`: al archivar un ciclo lectivo, basta con hacer `DELETE FROM reserva_grupo WHERE materia_id IN (...)` — sus filas `reserva` hijas se eliminan solas, sin necesitar dos sentencias en el orden correcto. `creado_por`/`cancelado_por` en `SET NULL` por el mismo motivo que en `reserva_grupo`: el historial no depende de que la cuenta siga existiendo.

```sql
-- Invariante: NORMAL siempre pertenece a un grupo y a una materia;
-- EVALUACION_ESTATAL nunca pertenece a un grupo de docente ni a una materia.
ALTER TABLE reserva ADD CONSTRAINT chk_reserva_tipo_coherente CHECK (
  (tipo = 'NORMAL' AND reserva_grupo_id IS NOT NULL AND materia_id IS NOT NULL)
  OR
  (tipo = 'EVALUACION_ESTATAL' AND reserva_grupo_id IS NULL AND materia_id IS NULL)
);

-- fecha/hora se duplican de reserva_grupo hacia reserva a propósito:
-- el EXCLUDE de solapamiento necesita filtrar por pc_id + rango horario
-- sin depender de un JOIN.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Se usa `fecha + hora_inicio` (aritmética DATE + TIME = timestamp, pura)
-- en vez de concatenar texto y castear — Postgres exige que las
-- expresiones de un índice (un EXCLUDE es un índice GiST por debajo) sean
-- IMMUTABLE, y el cast de texto a timestamp depende de la configuración
-- regional del servidor (DateStyle), por lo que Postgres lo trata como
-- STABLE y rechaza la constraint con "functions in index expression must
-- be marked IMMUTABLE". La suma date+time no tiene ese problema.
ALTER TABLE reserva ADD CONSTRAINT no_solapamiento
  EXCLUDE USING gist (
    pc_id WITH =,
    tsrange(fecha + hora_inicio, fecha + hora_fin) WITH &&
  )
  WHERE (estado = 'CONFIRMADA');

CREATE INDEX idx_reserva_pc_fecha ON reserva(pc_id, fecha);
CREATE INDEX idx_reserva_grupo ON reserva(reserva_grupo_id) WHERE reserva_grupo_id IS NOT NULL;
CREATE INDEX idx_reserva_materia ON reserva(materia_id) WHERE materia_id IS NOT NULL;
CREATE INDEX idx_reserva_creado_por ON reserva(creado_por);
```

> **Por qué `EVALUACION_ESTATAL` no lleva `materia_id` ni `reserva_grupo_id`:** un bloqueo de evaluación no es la reserva de un docente para dar clase — es un bloqueo administrativo sobre PCs concretas en un rango horario. No tiene sentido forzarlo a pertenecer a una materia.

### `notificacion`
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| reserva_id | UUID | FK → reserva.id **ON DELETE SET NULL**, NULL |
| mensaje | TEXT | NOT NULL |
| estado | VARCHAR(10) | NOT NULL DEFAULT 'NO_LEIDA', CHECK IN ('NO_LEIDA','LEIDA') |
| creada_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| leida_en | TIMESTAMPTZ | NULL |

```sql
CREATE INDEX idx_notif_usuario_estado ON notificacion(usuario_id, estado);
```

> `usuario_id` en `CASCADE`: a diferencia de `reserva_id` (que preserva el mensaje aunque la reserva ya no exista), acá el destinatario es la razón de ser de la fila — si su cuenta se elimina definitivamente, sus propias notificaciones no le sirven a nadie más y se van con ella.
>
> `reserva_id` sigue apuntando a la fila `reserva` (PC puntual), no a `reserva_grupo` — así la notificación puede ser específica ("tu reserva de la PC-07 del 12/08 10:00 fue cancelada") aunque el resto del grupo siga confirmado.
>
> **`ON DELETE SET NULL` en `reserva_id` es necesario, no cosmético:** cuando se archiva un ciclo lectivo, sus `reserva` se eliminan físicamente (ver §3), pero las notificaciones ya enviadas sobre esas reservas no deberían desaparecer ni romperse — `mensaje` ya tiene el texto completo guardado, así que la notificación sigue siendo legible aunque `reserva_id` quede en `NULL`. Sin este `ON DELETE SET NULL` (o un `CASCADE`), el `DELETE` de `reserva` fallaría por violación de FK.

### `codigo_recuperacion`
Códigos de un solo uso para recuperar una contraseña olvidada (RF-01.10, migración `009`).

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

> **Es una tabla aparte y no dos columnas en `usuario`** por dos razones. El ciclo de vida no tiene nada que ver: una fila acá vive quince minutos, la de `usuario` vive años. Y los intentos fallidos se escriben en cada prueba de código — meter ese `UPDATE` sobre la fila de usuario haría que cada intento de recuperación tocara la fila que usa todo el resto del sistema.
>
> **`codigo_hash` guarda el hash, nunca el código.** Son seis dígitos: si la base se filtrara (un backup, un dump de soporte), un código en claro sería una cuenta abierta hasta que expire. Se usa el mismo argon2 que las contraseñas.
>
> **`usado_en` cubre los dos finales posibles**: el código se consumió bien, o se quemó al agotar los cinco intentos. En los dos casos dejó de existir para el sistema, y una sola columna evita tener que preguntar por dos.
>
> Las filas viejas **no se borran**: quedan como registro de que esa persona pidió un código, que es justo lo que hay que poder mirar si alguien reporta algo raro. El índice es parcial (`WHERE usado_en IS NULL`) porque la única consulta en caliente es "¿tiene un código sin usar?", y las filas viejas —que con el tiempo son todas— no hace falta indexarlas. Si algún día molestaran, se limpian con un `DELETE` por `creado_en`; a esta escala no hace falta un job.
>
> `ON DELETE CASCADE` por lo mismo que en `notificacion`: sin la cuenta, el código no le sirve a nadie.

### `horario_admin`
Patrón semanal recurrente de presencia en el laboratorio — puramente informativo (RF-07), no afecta permisos ni reservas.

`dia_semana` sigue la misma regla que `regla_recurrencia`: `LUNES` a `VIERNES`, con `CHECK` desde la migración `005`. Hasta entonces este era el único lugar del sistema que todavía aceptaba `SABADO`.

| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| usuario_id | UUID | FK → usuario.id **ON DELETE CASCADE**, NOT NULL |
| dia_semana | VARCHAR(10) | NOT NULL, CHECK (`LUNES`…`VIERNES`) |
| hora_inicio | TIME | NOT NULL |
| hora_fin | TIME | NOT NULL, CHECK (hora_fin > hora_inicio) |

```sql
CREATE INDEX idx_horario_admin_usuario ON horario_admin(usuario_id);
```

> Sin versionado ni `vigente_desde`/`vigente_hasta`: a diferencia de `regla_recurrencia` (que materializa una fila `reserva_grupo` por cada ocurrencia, porque cada una puede reservarse o cancelarse individualmente), acá no hay nada que materializar — es un patrón que se evalúa en el momento de la consulta contra el día/hora actual. Editar (`PATCH`) un bloque cambia el patrón de inmediato para todas las semanas futuras; no hace falta ninguna acción para que se "propague".

### `horario_admin_excepcion`
Cubre tanto una excepción planificada (horario distinto un día puntual) como el botón rápido "marcarme no disponible ahora" — son la misma fila, con `fecha = hoy` en el segundo caso.

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
  (tipo = 'NO_DISPONIBLE' AND hora_inicio IS NULL AND hora_fin IS NULL)
  OR
  (tipo = 'HORARIO_MODIFICADO' AND hora_inicio IS NOT NULL AND hora_fin IS NOT NULL)
);

CREATE INDEX idx_horario_excepcion_usuario_fecha ON horario_admin_excepcion(usuario_id, fecha);
```

**Cálculo de "¿disponible ahora?"** (resuelto en el momento de la consulta, no almacenado):
1. Buscar `horario_admin_excepcion` para ese Admin con `fecha = hoy`.
   - Si existe y `tipo = NO_DISPONIBLE` → no disponible, sin importar el patrón semanal.
   - Si existe y `tipo = HORARIO_MODIFICADO` → comparar la hora actual contra `hora_inicio`/`hora_fin` de la excepción.
2. Si no hay excepción para hoy → comparar la hora actual contra los bloques de `horario_admin` de ese día de semana (puede haber más de uno; alcanza con que la hora actual caiga dentro de alguno).
3. `UNIQUE(usuario_id, fecha)` garantiza que no haya ambigüedad: como máximo una excepción por Admin por día.

## 3. Archivado de ciclo lectivo: qué se borra y qué se preserva

Archivar un ciclo lectivo **no es un soft-delete de las reservas** — es un borrado real. El propósito de "archivar" es evitar tener que recrear cursos, materias y las asignaciones docente↔materia el año siguiente; **no** es preservar el detalle de cada reserva.

**Al archivar un ciclo lectivo:**
1. Se preservan (`archivado = true`, sin borrar): `curso`, `materia`, `docente_materia` — esto es lo que evita recrear "1°A" + "Matemáticas" + "el titular es Fulano" el año que viene.
2. Antes de borrar nada, se calcula y persiste un **snapshot histórico agregado** (ver `historico_uso_pc` / `historico_uso_docente` abajo) con las estadísticas del año que termina.
3. Se **eliminan físicamente** (`DELETE`, no `UPDATE archivado=true`): todos los `reserva_grupo`, `reserva`, `regla_recurrencia` cuya `materia_id` pertenece a un curso de ese ciclo, más los bloqueos por evaluación estatal (`reserva` con `tipo = EVALUACION_ESTATAL`) del año de ese ciclo — no tienen materia, así que hay que ubicarlos por año.
4. `incidencia` **no se toca** — pertenece a la `pc`, no al ciclo lectivo ni a ninguna materia; el historial de incidencias es independiente del calendario académico.

### `historico_uso_pc` (nueva — permanente, uno por PC por año)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL |
| pc_id | UUID | FK → pc.id, NOT NULL |
| identificador_snapshot | INTEGER | NOT NULL |
| carro_nombre_snapshot | VARCHAR(100) | NOT NULL |
| minutos_reservados | INTEGER | NOT NULL |
| cantidad_reservas | INTEGER | NOT NULL |
| | | UNIQUE (anio, pc_id) |

### `historico_uso_docente` (nueva — permanente, uno por docente por año)
| Campo | Tipo | Restricciones |
|---|---|---|
| id | UUID | PK |
| anio | INTEGER | NOT NULL |
| usuario_id | UUID | FK → usuario.id **ON DELETE SET NULL**, NULL |
| nombre_docente_snapshot | VARCHAR(200) | NOT NULL |
| cantidad_reservas | INTEGER | NOT NULL |
| minutos_totales | INTEGER | NOT NULL |
| | | UNIQUE (anio, usuario_id) |

> `usuario_id` queda en NULL si esa cuenta se elimina físicamente más adelante (RF-01.9); la fila sobrevive y sigue siendo legible por `nombre_docente_snapshot`. La política `SET NULL` se agregó en la migración `002`: originalmente la FK no tenía ninguna, así que el hard delete de un docente que figurara en cualquier snapshot histórico fallaba con violación de FK.

```sql
CREATE INDEX idx_historico_pc_anio ON historico_uso_pc(anio);
CREATE INDEX idx_historico_docente_anio ON historico_uso_docente(anio);
```

> Estas dos tablas **sí** son un read-model permanente (a diferencia de la decisión de "sin CQRS" para reportes del ciclo activo, §4) — pero se calculan **una sola vez, al archivar**, no se sincronizan continuamente por eventos. Son baratas de mantener y son la única fuente de verdad para años ya cerrados, porque el detalle (`reserva`/`reserva_grupo`) de esos años ya no existe.
>
> **`incidencia` no necesita una tabla histórica equivalente**: a diferencia de `reserva`/`reserva_grupo`, nunca se elimina al archivar un ciclo (no depende de ningún ciclo lectivo — pertenece a la `pc`). El reporte de incidencias (RF-06.3) siempre puede resolverse con una query directa sobre `incidencia`, sin importar cuántos ciclos se hayan archivado desde entonces.

## 4. Reportes: queries agregadas para el ciclo activo, tablas históricas para años cerrados

A esta escala no hace falta un read-model continuo sincronizado por eventos — pero sí hace falta uno puntual al archivar (§3), ya que el detalle se borra. Los reportes de RF-06 se resuelven así:

- **Ciclo lectivo activo** (el año en curso, con `reserva`/`reserva_grupo` todavía en la base): queries directas.
- **Ciclos ya archivados**: se leen de `historico_uso_pc` / `historico_uso_docente`.

```sql
-- Uso por PC en un rango de fechas (ciclo activo)
SELECT p.id AS pc_id, p.identificador, c.nombre AS carro_nombre,
       SUM(EXTRACT(EPOCH FROM (r.hora_fin - r.hora_inicio)) / 60)::int AS minutos_reservados,
       COUNT(*) AS cantidad_reservas
FROM reserva r
JOIN pc p ON p.id = r.pc_id
JOIN carro c ON c.id = p.carro_id
WHERE r.fecha BETWEEN $1 AND $2 AND r.estado != 'CANCELADA'
GROUP BY p.id, p.identificador, c.nombre;

-- Uso por docente en un mes (ciclo activo, vía reserva_grupo)
SELECT rg.creado_por AS usuario_id, rg.nombre_docente_snapshot,
       COUNT(DISTINCT rg.id) AS cantidad_reservas,
       SUM(EXTRACT(EPOCH FROM (r.hora_fin - r.hora_inicio)) / 60)::int AS minutos_totales
FROM reserva r
JOIN reserva_grupo rg ON rg.id = r.reserva_grupo_id
WHERE to_char(rg.fecha, 'YYYY-MM') = $1 AND r.estado != 'CANCELADA'
GROUP BY rg.creado_por, rg.nombre_docente_snapshot;

-- Uso por PC en un año ya archivado
SELECT pc_id, identificador_snapshot, carro_nombre_snapshot, minutos_reservados, cantidad_reservas
FROM historico_uso_pc
WHERE anio = $1;
```

## 5. Notas de diseño

- `pc.freezado` es informativo (Deep Freeze instalado), sin efecto funcional sobre reservas.
- `pc.dada_de_baja` / `pc.fecha_baja`: soft delete de inventario — la fila se conserva para no perder el historial de incidencias y reservas ya asociadas.
- `usuario.estado` incluye `BAJA` como estado terminal (sin reactivación) — distinto de `RECHAZADA`, que es para una solicitud de registro que nunca se aprobó.
- `reserva_grupo` / `reserva` / `regla_recurrencia` no persisten indefinidamente: se **eliminan físicamente** al archivar el ciclo lectivo de su materia (ver §3), a diferencia de `curso`/`materia`/`docente_materia`, que solo se marcan `archivado=true` y se preservan.
