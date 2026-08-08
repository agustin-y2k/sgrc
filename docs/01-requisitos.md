# Documento de Requerimientos — SGRC

## 1. Descripción general
Plataforma web para gestión de inventario de equipos informáticos y reservas vinculadas a cursos y materias, para uso de **una única institución educativa**. Los usuarios tienen cuenta con rol directo (`ADMIN` o `DOCENTE`).

## 2. Jerarquía del dominio
```
Usuario (rol: ADMIN | DOCENTE, estado: PENDIENTE|APROBADA|RECHAZADA|BAJA)
├── Carro → PC (freezado: informativo) → Incidencia
└── CicloLectivo (año)
    └── Curso (formato estricto: "1°A" a "6°Z")
        └── Materia (propia del curso, no catálogo compartido)
            └── DocenteMateria (TITULAR | SUPLENTE — informativo)

ReservaGrupo (materia, fecha, horario — "la reserva" que percibe el docente)
└── Reserva (una fila por PC seleccionada dentro del grupo)
```

## 3. Actores

| Actor | Alcance | Creado por |
|---|---|---|
| `ADMIN` | Toda la institución | Otro Admin (el primero: seed) |
| `DOCENTE` | Toda la institución | Se autorregistra, queda pendiente de aprobación de un `ADMIN` |

## 4. Glosario
- **Carro**: conjunto físico de equipos. Nombre y cantidad libres.
- **PC**: equipo individual dentro de un carro. Atributo `freezado` (boolean): indica si tiene Deep Freeze (u otro software equivalente) instalado — es **metadata informativa** para inventario, sin efecto funcional sobre reservas.
- **Ciclo lectivo**: año académico. Cursos y materias pertenecen a un ciclo.
- **Curso**: nombre con formato estricto `{año}°{división}` (año `1°`-`6°`, división `A`-`Z`), ej. `1°A`, `6°Z`.
- **Materia**: asignatura propia de un curso específico (no catálogo: 1°A tiene SU Matemáticas).
- **DocenteMateria**: vínculo informativo docente↔materia con rol TITULAR/SUPLENTE. Sin incumbencia en permisos del sistema.
- **ReservaGrupo**: la reserva tal como la hace un docente — una materia, una fecha, un horario. Un docente selecciona varias PCs de una lista (tildando casillas) hasta juntar la cantidad que necesita; esa operación completa es un `ReservaGrupo`.
- **Reserva**: cada PC individual dentro de un `ReservaGrupo`. Las cancelaciones en cascada (evaluación estatal, PC fuera de servicio) actúan sobre `Reserva` puntuales, nunca sobre el grupo completo salvo que terminen afectando a todas sus PCs.
- **Bloqueo de evaluación estatal**: el admin bloquea PCs de un carro completo en un **rango horario definido**, cancelando automáticamente las `Reserva` en conflicto y notificando a los docentes afectados.
- **Horario de disponibilidad (Admin)**: patrón semanal recurrente que cada Admin carga para indicar cuándo está presente en el laboratorio. Puramente informativo — no afecta permisos ni funcionalidad.

## 5. Requerimientos funcionales

### RF-01 — Usuarios y autenticación
- RF-01.1: Login con email y contraseña, en un solo paso. Si la cuenta tiene `debe_cambiar_password = true`, el login funciona igual (con la contraseña temporal) pero la respuesta lo indica explícitamente para que el frontend fuerce la pantalla de cambio antes de dejar operar el resto del sistema.
- RF-01.2: JWT contiene: `userId`, `rol`, `nombre`, `apellido`.
- RF-01.3: Un docente se autorregistra y queda con estado `PENDIENTE` hasta aprobación de un `ADMIN`. Al registrarse puede declarar **qué curso y qué materia va a dictar** (texto libre, opcional): es lo que el `ADMIN` mira al aprobarlo para saber a qué asignarlo (RF-02.6), y si eso todavía no existe, para saber que lo tiene que crear. No es una referencia a `Curso` ni a `Materia` — al registrarse la persona no está autenticada y no puede elegir de una lista. Si el email ya pertenece a una cuenta existente en estado `BAJA`, el sistema devuelve un mensaje específico ("este email pertenece a una cuenta dada de baja — pedile a un Admin que la elimine para poder registrarte de nuevo") en vez del genérico "email ya registrado", para que quien vuelve entienda que es su propia cuenta vieja y no un conflicto con otra persona.
- RF-01.4: El primer `ADMIN` se crea por seed. Un `ADMIN` puede crear o aprobar otros `ADMIN`, promover a un docente ya aprobado, y **quitarle el rol a otro `ADMIN`** dejándolo como docente sin cerrarle la cuenta. Quitar el rol conserva materias, reservas y forma de ingreso: es un cambio de permisos y no una baja. Con dos límites: nunca sobre el último `ADMIN` activo (RF-01.8) ni sobre uno mismo —quien lo hiciera perdería en el acto la pantalla desde la que lo pidió y dependería de otro `ADMIN` para volver atrás—.
- RF-01.5: Contraseñas con hash `argon2id`. JWT firmados con `HS256` (ver `06-arquitectura.md`).
- RF-01.6: Recuperación de contraseña asistida por `ADMIN`: un `ADMIN` puede resetear la contraseña de cualquier usuario a una temporal; el sistema marca `debe_cambiar_password = true` en esa cuenta (ver `07-modelo-datos.md`), y el usuario debe cambiarla antes de poder usar el resto del sistema. Es el camino de **rescate**, no el habitual (ese es RF-01.10): sirve para quien no puede recibir el mail —email mal escrito al registrarse, casilla perdida— y es el único disponible en un despliegue sin correo configurado.
- RF-01.7: Cualquier usuario autenticado puede cambiar su propia contraseña en cualquier momento (no solo cuando se lo exige un reseteo de Admin), indicando la contraseña actual y la nueva.
- RF-01.11: **Cambiar una contraseña cierra las sesiones abiertas de esa cuenta**, por cualquiera de los tres caminos (RF-01.6, RF-01.7 y RF-01.10). Quien cambia su contraseña porque sospecha que entraron a su cuenta necesita que el acceso del otro se corte ahí, no cuando venza el token que ya tenía. La sesión desde la que se hace el cambio sobrevive; el resto vuelve a la pantalla de ingreso con el motivo explicado. Ver `09-seguridad-rbac.md` §1.
- RF-01.10: **Recuperación de contraseña por autoservicio.** Quien olvidó su contraseña pide un código desde la pantalla de ingreso, le llega a su email, y con ese código elige una contraseña nueva — sin que ningún `ADMIN` intervenga ni vea nada. La contraseña de cada persona queda enteramente en sus manos.
  - El código es de 6 dígitos, vence a los **15 minutos**, sirve **una sola vez**, admite **5 intentos** fallidos y pedir uno nuevo invalida el anterior. En la base se guarda hasheado con argon2, igual que una contraseña (ver `migrations/009`).
  - Solo aplica al **ingreso local**. Una cuenta creada con Google no tiene contraseña propia: quien la verifica es Google. A esa persona le llega un correo explicándoselo, en vez de dejarla esperando un código que no existe.
  - Pedir el código responde **siempre lo mismo**, exista o no la cuenta y esté o no aprobada. Es lo que evita que el formulario sirva para averiguar qué direcciones están registradas en la escuela (ver `09-seguridad-rbac.md` §4).
  - Depende de que el despliegue tenga correo configurado (`SMTP_*`). Sin eso, el enlace no aparece en la pantalla de ingreso y la salida es RF-01.6.
- RF-01.8: El sistema **nunca permite** que quede cero `ADMIN` en estado `APROBADA`: rechaza dar de baja, rechazar o degradar al último `ADMIN` activo (HTTP 409).
- RF-01.9: `ADMIN` puede eliminar (hard delete) definitivamente una cuenta, pero **solo si está cerrada**, o sea en alguno de los dos estados terminales: `BAJA` o `RECHAZADA`. Una cuenta `APROBADA` no se elimina directamente —primero hay que darla de baja, para que corra la cascada que cancela sus reservas— y una `PENDIENTE` tampoco: hay que resolverla aprobándola o rechazándola, porque borrar en silencio a alguien que espera respuesta lo deja afuera sin que nadie se entere. El propósito principal es liberar el email para un nuevo registro (RF-02.9); las reservas, incidencias y notificaciones asociadas a esa cuenta no se eliminan, solo pierden la referencia al usuario (ver `07-modelo-datos.md`).
  - Que `RECHAZADA` habilite el borrado es lo que hace que rechazar no sea una trampa: ese estado no transiciona a ningún otro (ver `PuedeTransicionarA`), así que mientras eliminar exigió `BAJA`, una cuenta rechazada no se podía reactivar **ni** eliminar y su email quedaba tomado para siempre. Un rechazo por error dejaba a esa persona sin poder registrarse nunca más con su propia dirección.

### RF-02 — Ciclo lectivo, cursos y materias
- RF-02.1: `ADMIN` crea ciclos lectivos (año). Solo puede haber **un ciclo lectivo `activo` a la vez** — el sistema rechaza crear uno nuevo si ya existe otro activo sin archivar.
- RF-02.2: Dentro de cada ciclo se crean cursos. El nombre **no es libre**: se compone de dos partes obligatorias — año (`1°` a `6°`) y división (`A` a `Z`), formato `{año}°{división}` (ej: `1°A`, `6°Z`). El sistema rechaza cualquier nombre que no cumpla el patrón `^[1-6]°[A-Z]$`.
- RF-02.3: Dentro de cada curso se crean materias (nombre libre, propias del curso).
- RF-02.4: Al cerrar un ciclo, el `ADMIN` archiva cursos y materias (`archivado=true`, se preservan — esto es lo que evita tener que recrearlos el año siguiente y volver a asignar docentes). **Todas las reservas** (`ReservaGrupo`, `Reserva`, `ReglaRecurrencia`) de las materias de ese ciclo **se eliminan físicamente** en el mismo paso — no quedan como historial consultable en detalle. Antes de eliminarlas, el sistema calcula y guarda un snapshot agregado de estadísticas del año (uso por PC, uso por docente) en tablas históricas permanentes, para mantener registro año a año sin conservar cada reserva individual. `Incidencia` no se ve afectada (pertenece a la PC, no al ciclo lectivo).
- RF-02.5: Al archivar, el sistema ofrece clonar al nuevo ciclo: copia cursos + materias (nuevas filas, sin `archivado`) sin asignaciones de docentes.
- RF-02.6: `ADMIN` asigna docentes a materias con rol TITULAR o SUPLENTE (informativo). Una materia puede tener más de un docente asignado (ej: titular + suplente, o varios simultáneos). Solo se puede asignar a un usuario con estado `APROBADA` — no tiene sentido operativo asignar a alguien `PENDIENTE`, `RECHAZADA` o `BAJA`.
- RF-02.7: Un docente puede estar asignado a múltiples materias en distintos cursos.
- RF-02.8: Al dar de baja a un docente (estado `BAJA`), el sistema, para cada materia donde tenía `DocenteMateria`: primero verifica si queda al menos otro docente `APROBADA` asignado — si es así, no cancela ninguna reserva (la reserva pertenece a la materia, no al docente puntual) y solo genera un aviso informativo a todos los `ADMIN` listando las reservas futuras del docente dado de baja; si la materia queda **sin ningún docente activo**, cancela los `ReservaGrupo` futuros de esa materia y notifica a todos los `ADMIN` (no hay un docente al cual avisar). **Después** de esa revisión, el sistema elimina todos los vínculos `DocenteMateria` del docente dado de baja (ya no está asignado a ninguna materia) — el orden importa: primero se decide el destino de las reservas mirando quién más queda asignado, recién después se borra el vínculo del que se va.
- RF-02.9: La baja de un docente es **permanente**: no existe una acción de "reactivar" cuenta — la API rechaza cualquier intento de cambiar el estado de una cuenta que ya está en `BAJA`. Si la persona vuelve a la institución, debe autorregistrarse de nuevo como cuenta nueva (RF-01.3). Si quiere reutilizar el mismo email de su cuenta anterior, el `ADMIN` puede eliminar (hard delete) esa cuenta en `BAJA` — sus reservas, incidencias y notificaciones no se borran con ella (ver `07-modelo-datos.md` para el detalle de qué se preserva y qué se limpia).
- RF-02.10: Remover a un docente de **una sola materia** puntual (sin darlo de baja del sistema) sigue la misma política que RF-02.8: nunca cancela reservas existentes — la reserva pertenece a la materia, y si queda al menos otro docente asignado, las reservas del que se desvincula siguen siendo válidas para la materia. Solo se cancelan si esa materia queda sin ningún docente asignado tras la remoción, con el mismo aviso a todos los `ADMIN`. La única diferencia con RF-02.8 es que el docente conserva su cuenta y sus demás materias intactas.
- RF-02.11: Mientras el ciclo lectivo esté activo (sin archivar), `ADMIN` puede editar el nombre de un curso o materia, y eliminarlo (hard delete) **solo si no tiene ninguna reserva asociada** — para corregir errores de carga sin esperar al archivado anual. Un curso/materia con reservas ya no puede eliminarse, solo archivarse junto con su ciclo.

### RF-03 — Inventario
- RF-03.1: `ADMIN` crea y edita carros (nombre, descripción).
- RF-03.2: `ADMIN` registra PCs en un carro con: identificador (número entero, único dentro del carro — puede repetirse en carros distintos, ej. "PC 27" existe en el Carro 1 y en el Carro 2), número de serie (texto alfanumérico de hasta 50 caracteres, único en toda la institución, es el de fábrica: casi siempre lleva letras, ej. `5CD1234ABC`. Se guarda en mayúsculas y sin espacios al borde, para que la misma máquina no entre dos veces con distinta caja), `freezado` (boolean, informativo), CPU, RAM, SO, software instalado (texto libre — incluye, por ejemplo, versión de AutoCAD u otro software específico instalado).
- RF-03.3: Estado de PC: `DISPONIBLE`, `EN_MANTENIMIENTO`, `FUERA_DE_SERVICIO`. PCs no disponibles no pueden reservarse.
- RF-03.4: `ADMIN` edita los datos de una PC (identificador, número de serie, freezado, CPU, RAM, SO, software instalado) y puede darla de baja del inventario (soft delete: deja de listarse como disponible para reservar, pero su historial de incidencias y reservas pasadas se conserva).
- RF-03.5: `ADMIN` registra y gestiona incidencias. Docentes solo pueden reportarlas.
- RF-03.6: El sistema permite registrar envío de PC a soporte técnico DGE con fecha.
- RF-03.7: Cualquier usuario autenticado (no solo `ADMIN`) puede ver el listado de carros y PCs con su `estado`, `freezado` y `software_instalado` — un docente necesita ver, por ejemplo, qué PCs tienen instalado AutoCAD 2007 frente a las que tienen AutoCAD 2027, antes de elegir cuáles reservar para su clase.
- RF-03.8: Al cambiar el estado de una PC a `EN_MANTENIMIENTO` o `FUERA_DE_SERVICIO`, el sistema cancela automáticamente **solo la `Reserva` de esa PC puntual** dentro de cada `ReservaGrupo` afectado — nunca el grupo completo, salvo que sea la única PC del grupo o que las demás ya estuvieran canceladas por otro motivo. El docente recibe una notificación por cada PC afectada. El admin puede indicar un motivo opcional; si no lo indica, el sistema genera un mensaje por defecto (ej: "Tu reserva de la PC {identificador} del {fecha} {hora} fue cancelada: la PC pasó a FUERA_DE_SERVICIO"). Esta condición es **indefinida** (no tiene fecha de fin conocida): cuando la PC vuelve a `DISPONIBLE`, las reservas canceladas **no se restauran automáticamente** — quien las necesite debe volver a reservar.
- RF-03.9: Dar de baja una PC (RF-03.4) dispara la **misma cascada de cancelación** que RF-03.8 sobre sus reservas futuras — no tendría sentido eliminarla del inventario y dejar reservas colgadas sobre un equipo que ya no está disponible.
- RF-03.10: `ADMIN` puede mover una PC de un carro a otro (reorganización de inventario). El identificador debe seguir siendo único dentro del carro destino.

#### Equipos que no son computadoras de un carro

> La escuela también presta un proyector, dos cargadores y dos notebooks de otro modelo. De todo eso, solo el proyector podría llegar a reservarse. Y la lista cambia de escuela en escuela: otra tiene proyector pero quizá ni cargadores ni notebooks sueltas.

- RF-03.15: `ADMIN` registra **equipos prestables que no están en ningún carro**: un proyector, un cargador, lo que sea. No tienen identificador ni número de serie —"PC 3" no significa nada para un cargador, y un cargador puede no traer serie de fábrica—, así que lo que los identifica es un **nombre**, obligatorio y único entre ellos. El **tipo** es texto libre, con sugerencias de los que ya existen: con una lista cerrada, agregar "impresora 3D" pediría tocar el sistema, y otra escuela con otras cosas no podría usarlo.
- RF-03.16: Cada equipo tiene una marca de **reservable**. El proyector sí; un cargador se presta en el momento y nadie planifica con él. Lo no reservable **no aparece** en la lista de equipos libres al reservar, ni se puede reservar por un pedido armado a mano. Sí se presta, que es su caso principal.
- RF-03.17: En cualquier pantalla o correo, un equipo se nombra por su **etiqueta**: "PC 3" si está en un carro, su nombre si no. La resuelve el servidor para que la misma cosa no se vea distinta según dónde se la mire. Aplica a **todo** lo que nombra un equipo: la lista al reservar, "Mis reservas", el mostrador, los avisos de licencias, los reportes de uso e incidencias y el histórico archivado. Ahí donde una pantalla arme el rótulo por su cuenta con el identificador y el carro, un proyector se lee "PC 0 · ".

> **Todo esto vive en la misma entidad que las PCs, y no es un detalle de implementación.** "Qué hay afuera del laboratorio" tiene que ser UNA sola lista (RF-08): con dos clases de cosa, el préstamo necesitaría dos referencias, el mostrador dos consultas y el barrido dos recorridos. Compartiendo entidad, el proyector queda prestable, reclamable, liberable y —si es reservable— reservable, sin una línea nueva en ninguno de esos flujos.
>
> El costo es que la tabla se llama `pc` y tiene un proyector adentro. Es la segunda imprecisión del modelo, después de `carro`. Se aceptó a propósito: renombrar a `equipo` toca 419 sitios y mezclarlo con este cambio daría un diff imposible de revisar. Va aparte.

#### Licencias de software con vencimiento

> Motivo: una de las PCs tiene AutoCAD con licencia que vence cada 30 días. Cuando vence, el programa deja de abrir, y sin un contador el `ADMIN` se entera el día que un docente no puede dar la clase. No reemplaza a `software_instalado` de RF-03.2/RF-03.7 (texto libre que el docente ve para elegir PC): ese describe qué hay en la máquina, esto lleva el vencimiento y es solo de `ADMIN`.

- RF-03.11: `ADMIN` registra licencias de software con vencimiento periódico. Hay **una licencia por (PC, software)**: el mismo AutoCAD instalado en las ocho PCs de un carro son ocho licencias, cada una con su propio contador. Es a propósito — el caso a cubrir es que una máquina quede sin renovar mientras las demás sí. Cada licencia tiene nombre del software, días que dura una renovación (30, 60, o los que sean: **puede cambiar en cualquier momento**), y con cuántos días de anticipación avisar. Puede haber varias licencias por PC, y la misma licencia en PCs de carros distintos.
- RF-03.12: **Los días que faltan no se guardan**: se calculan como `fecha_vencimiento − hoy` cada vez que se consulta. No hay ningún contador que decrementar, así que un servidor apagado varios días no descuadra nada (mismo criterio que RF-04.9).
- RF-03.13: El vencimiento de una licencia se puede **cargar y corregir en cualquier momento**, de tres formas —la fecha en que se renovó (se le suman los días de duración), los días que le quedan según la propia máquina, o la fecha de vencimiento directa— y la renovación se puede registrar con **fecha pasada**: el caso normal es que alguien renueve el martes y lo cargue el jueves, o que otro `ADMIN` tenga que corregirlo porque el primero se olvidó. La fecha de vencimiento puede quedar **sin cargar**, que significa "todavía no se verificó contra la máquina" y **no** "no vence nunca": esas licencias no disparan ningún aviso y se listan primero. Cambiar los días de duración **no mueve** el vencimiento vigente (aplica a la próxima renovación); recalcularlo es una acción explícita aparte. Queda registrado quién fijó el vencimiento y cuándo lo cargó, que no es lo mismo que cuándo se renovó.
- RF-03.14: El sistema avisa **dos veces por ciclo** a todos los `ADMIN`: con la anticipación configurada en esa licencia (por defecto, el día anterior) y el día en que vence. Después se calla — no insiste a diario sobre una licencia vencida. Cada aviso sale **una sola vez** aunque el proceso se reinicie: la marca de "ya avisé" apunta a la fecha de vencimiento para la que salió, así que renovar reabre el ciclo solo. Las licencias de PCs dadas de baja no avisan (las de PCs `FUERA_DE_SERVICIO` sí: son recuperables y la licencia les sigue corriendo). Ver RF-05.9.

### RF-04 — Reservas

> **Semana lectiva: lunes a viernes.** El sistema rechaza reservar un sábado o un domingo, tanto puntual (RF-04.2) como recurrente (RF-04.5) — el selector de día de una recurrencia solo ofrece de lunes a viernes. Los **feriados y el receso de invierno no se modelan**: si alguien reserva un feriado, se cancela a mano. Los bloqueos por evaluación estatal (RF-04.7) quedan exceptuados de esta restricción: son excepcionales por naturaleza y es el Admin quien decide cuándo.

- RF-04.1: Pueden reservar para una materia: docentes asignados a ella (vía DocenteMateria) y cualquier `ADMIN`, siempre que la materia **no esté archivada** (`archivado=false`) — una materia de un ciclo ya cerrado no admite reservas nuevas aunque el registro se conserve.
- RF-04.2: Un docente reserva **una o varias PCs en una sola operación**: selecciona PCs de una lista (como tildar casillas) hasta juntar la cantidad que necesita para su clase — la lista no está restringida a un solo carro, puede combinar PCs de carros distintos en la misma reserva. El sistema crea un `ReservaGrupo` (materia, fecha, horario) con una `Reserva` por cada PC elegida, sin importar de qué carro venga cada una.
- RF-04.3: El sistema rechaza cualquier PC del grupo que tenga solapamiento de horario — informa cuáles PCs específicas están ocupadas (con quién y en qué materia) sin bloquear las que sí están libres, para que el docente pueda ajustar su selección antes de confirmar.
- RF-04.4: Cualquier usuario autenticado puede ver el calendario completo de una PC (bloques con nombre del docente, materia y horario).
- RF-04.5: Un `ReservaGrupo` puede ser recurrente (mismo día/horario/conjunto de PCs en un rango de fechas). El sistema valida todas las ocurrencias (todas las fechas × todas las PCs elegidas) antes de crear alguna. Si hay conflicto, informa cuáles son y no crea ninguna.
- RF-04.6: Al cancelar un `ReservaGrupo` recurrente, el usuario elige: "solo esta fecha" o "esta fecha y todas las siguientes" — aplicado a todas las PCs del grupo en esa fecha (o rango).
- RF-04.7: `ADMIN` puede bloquear PCs de cualquier carro para evaluación estatal en un **rango horario definido** (fecha + hora inicio + hora fin conocidos de antemano). Las `Reserva` puntuales en conflicto se cancelan automáticamente — el `ReservaGrupo` al que pertenecían pasa a `PARCIALMENTE_CANCELADA` si conserva otras PCs confirmadas, o a `CANCELADA` si todas sus PCs quedaron afectadas. Los docentes reciben notificación interna detallando qué PCs puntuales se cancelaron.
- RF-04.8: Al cancelar **manualmente** una `Reserva` puntual ajena, el admin ingresa motivo obligatorio (texto libre) y el docente recibe notificación interna. Si esa era la única PC confirmada del grupo, el grupo pasa a `CANCELADA`; si no, a `PARCIALMENTE_CANCELADA`.
- RF-04.9: Historial de reservas: dentro de un ciclo lectivo activo, nunca se eliminan — se marcan `CANCELADA` o `FINALIZADA` (a nivel `Reserva` y `ReservaGrupo`). Se eliminan físicamente únicamente cuando se archiva el ciclo lectivo de su materia (ver RF-02.4), y solo después de calcular el snapshot histórico agregado.

> **Nota — tres mecanismos de cancelación, todos a nivel de PC puntual, no confundir:**
> 1. **Cancelación manual de una PC puntual** (RF-04.8): el admin elige una `Reserva` específica y tipea un motivo obligatorio.
> 2. **Cascada por bloqueo de evaluación estatal** (RF-04.7): rango horario **definido** de antemano. Motivo generado por el sistema, no requiere texto libre. Solo afecta las PCs y fechas dentro de ese rango — el resto de una recurrencia sigue viva.
> 3. **Cascada por PC individual fuera de servicio o dada de baja** (RF-03.8/RF-03.9): duración **indefinida** (no se sabe cuándo, ni si, la PC vuelve a estar disponible). Cancela todas las reservas futuras de esa PC puntual, sin fecha de corte, y no las restaura automáticamente al volver a `DISPONIBLE`.
>
> En los tres casos, la cancelación es siempre a nivel de una `Reserva` (PC + fecha puntual) — el `ReservaGrupo` solo se marca `CANCELADA` como consecuencia de que **todas** sus PCs quedaron afectadas, nunca como acción directa.

### RF-05 — Notificaciones internas
- RF-05.1: Notificación cuando una `Reserva` (PC puntual) propia es cancelada manualmente por un admin (con motivo tipeado) — ver RF-04.8.
- RF-05.2: Notificación cuando una o más `Reserva` propias son canceladas por bloqueo de evaluación estatal — ver RF-04.7. El mensaje detalla qué PC(s) puntual(es) se vieron afectadas, no "toda tu reserva" si el grupo sigue parcialmente vigente.
- RF-05.3: Notificación cuando una `Reserva` propia es cancelada porque esa PC individual pasó a `EN_MANTENIMIENTO`, `FUERA_DE_SERVICIO` o fue dada de baja del inventario — ver RF-03.8/RF-03.9.

> **Un aviso por operación, no por fila.** Las cancelaciones en cascada
> (RF-05.1/05.2/05.3) se agrupan por docente: bloquear tres PCs de una misma
> clase para una evaluación genera **una** notificación que dice cuáles
> fueron ("Se cancelaron 3 de tus reservas del 13/08/2026 (PC 1, PC 2, PC 3):
> …"), no tres avisos idénticos. Para el docente es una sola noticia, y lo
> que necesita saber es qué PCs perdió.

- RF-05.4: Notificación a **todos los usuarios con rol `ADMIN` y estado `APROBADA`** (no a uno solo) cuando se da de baja a un docente con reservas futuras en materias que conservan otro docente activo (aviso informativo, no acción automática) — ver RF-02.8.
- RF-05.5: Notificación a todos los `ADMIN` con el mismo criterio que RF-05.4 cuando se remueve a un docente de una materia puntual (sin baja completa) — ver RF-02.10.
- RF-05.6: Notificación a **todos los usuarios con rol `ADMIN` y estado `APROBADA`** cuando un docente se autorregistra y queda `PENDIENTE` de aprobación (RF-01.3) — así no dependen de revisar manualmente la lista para enterarse de que hay una cuenta esperando.
- RF-05.7: Estado `NO_LEIDA` / `LEIDA`. Visibles al ingresar al sistema.
- RF-05.10: Notificaciones del barrido de reservas y entregas (RF-08.10 a RF-08.13): el recordatorio y el aviso de PC que no volvió, al docente; la reserva liberada, al docente; el reclamo de devolución y el corte de jornada, a los `ADMIN`. Son los **únicos avisos del sistema que no los dispara una persona**, y por eso el texto tiene que explicar solo por qué está llegando.
- RF-05.9: Notificación a **todos los usuarios con rol `ADMIN` y estado `APROBADA`** cuando hay licencias de software por vencer o ya vencidas (RF-03.14). Es el **único aviso del sistema que no lo dispara una persona sino el reloj**: un barrido periódico lo genera. Se agrupa igual que las cancelaciones —todas las licencias de la barrida en un solo aviso, no uno por licencia— y el aviso de la campana resume mientras que la copia por correo enumera cuáles son y en qué PC, porque se lee sin tener el sistema abierto.
- RF-05.8: **Copias por email** de algunos de estos avisos, para que no dependan de que la persona tenga el sistema abierto. Son tres: a todos los `ADMIN` cuando alguien queda `PENDIENTE` (RF-05.6 — una cuenta que nadie mira es un docente que no puede trabajar); a la persona cuando le aprueban la cuenta (RF-02); y el código de RF-01.10.
  - Desde RF-05.9 son cuatro: se suma el aviso de licencias por vencer a todos los `ADMIN`.
  - Es **opcional**: sin `SMTP_*` configurado el sistema funciona igual y los avisos internos siguen llegando a la campana.
  - El email es una **copia de cortesía**, no la fuente de verdad. Si el envío falla se loguea y nada más: la notificación interna ya está escrita, así que no se pierde nada que la persona no pueda ver entrando. Por eso tampoco hay cola de reintentos.
  - El envío ocurre **fuera del request**, en su propia goroutine: el bus de eventos publica de forma sincrónica, y abrir una conexión SMTP adentro dejaría a un docente esperando a Gmail para terminar de registrarse.
  - No se avisa por mail cuando una cuenta se **rechaza**. Es una decisión de trato, no técnica: un rechazo se conversa, no se notifica.

### RF-06 — Reportes (`ADMIN`)
- RF-06.1: Uso por PC (horas y cantidad de reservas), filtrable por rango de fechas, para el ciclo lectivo activo.
- RF-06.2: Uso por docente, para el ciclo lectivo activo.
- RF-06.3: Incidencias por equipo y por carro (no depende del ciclo lectivo). Como `Incidencia` nunca se elimina (sobrevive al archivado de cualquier ciclo, ver RF-02.4), este reporte siempre se resuelve con una query directa — no necesita snapshot histórico como RF-06.4.
- RF-06.4: Estadísticas históricas por año (uso por PC, uso por docente) para ciclos ya archivados, aunque el detalle de sus reservas ya no exista (ver RF-02.4 y `07-modelo-datos.md`).

### RF-07 — Disponibilidad de Admins (informativo)
- RF-07.1: Cada `ADMIN` carga su propio horario semanal recurrente de presencia en el laboratorio: uno o más bloques de `día de semana + hora inicio + hora fin` (cada Admin tiene días/horarios distintos).
- RF-07.2: Cualquier usuario autenticado (docentes incluidos) puede ver la lista de `ADMIN` con un estado **"disponible ahora"** calculado en el momento (según el horario cargado y la hora actual), junto con su horario semanal completo de referencia.
- RF-07.3: Un `ADMIN` puede editar su horario habitual en cualquier momento — el cambio aplica desde ese momento en adelante ("en cascada" hacia las semanas futuras), sin necesidad de una acción separada, porque el horario es un patrón recurrente, no una serie de reservas materializadas por semana.
- RF-07.4: Un `ADMIN` puede cargar una **excepción puntual** para una fecha concreta (un horario distinto ese día, o ausencia total) sin alterar su patrón semanal general — útil para un día particular distinto a lo habitual.
- RF-07.5: Un `ADMIN` puede marcarse manualmente como **"no disponible ahora"** en cualquier momento, incluso dentro de su horario habitual (ej: llegó tarde, está ausente ese día) — técnicamente es una excepción puntual para la fecha de hoy (RF-07.4), pero expuesta como una acción rápida de un solo paso.
- RF-07.6: Esta disponibilidad es **puramente informativa**: no restringe ni habilita ninguna otra funcionalidad del sistema — aprobar cuentas, reservar, cancelar, etc. funcionan exactamente igual sin importar si el Admin figura disponible o no en ese momento.

### RF-08 — Entregas y devoluciones de PCs (`ADMIN`)

> Reemplaza el papel en el que hoy los Admin anotan qué computadoras se lleva cada docente y cuáles devuelve.
>
> **La distinción que lo sostiene todo:** una **reserva** es el derecho a usar una PC en una franja; un **préstamo** es quién tiene la máquina *ahora*. No son lo mismo, y los tres casos de la escuela lo demuestran: hay reservas que nadie vino a buscar, hay préstamos sin reserva detrás (alguien pide una PC para un trámite), y hay préstamos que sobreviven a su reserva (la clase terminó y las máquinas no volvieron).

- RF-08.1: `ADMIN` registra la **entrega** de una o varias PCs, con o sin reserva detrás. Queda anotado a quién, quién la entregó y cuándo. La persona que se la lleva **no necesita tener cuenta** en el sistema: puede ser secretaría, preceptoría o un alumno, que es justo quien viene a pedir una máquina para un trámite. El nombre se guarda siempre, aunque además haya cuenta — es un snapshot, para que el registro siga diciendo quién se la llevó si esa cuenta se elimina (RF-01.9).
- RF-08.2: Una PC **no puede tener dos entregas abiertas a la vez**. Es la garantía que el papel no puede dar: que dos Admin anoten la misma máquina, o que nadie vea que ya estaba afuera, no lo detecta nadie hasta que aparece un docente sin computadora.
- RF-08.3: **"¿Dónde está la PC 3?" se deriva, no se guarda.** No hay ninguna columna en `pc` que diga "prestada": el estado sale de si existe una entrega sin devolver. Por eso no puede desincronizarse, que es exactamente lo que le pasa al papel cuando alguien devuelve una máquina y nadie tacha el renglón.
- RF-08.4: La entrega es **máquina por máquina**, no de a reserva completa: el docente puede llevarse tres de las cinco que reservó, y las otras dos siguen disponibles para otro. Al entregar contra una reserva, la hora en que deben volver sale del fin de esa reserva — no se pide.
- RF-08.5: Puede registrarse que **se las llevó otra persona** (el docente manda a un alumno o a un colega). En ese caso el aviso de devolución no queda atado al docente de la reserva, porque no es quien tiene la máquina.
- RF-08.6: En una entrega **espontánea** la hora de devolución es **opcional**: "vengo en un rato" es una respuesta honesta, y una hora inventada solo generaría reclamos falsos. Sin ella, a esa máquina no se le reclama nada. Si la PC entregada tiene una reserva próxima, el sistema **avisa pero no impide** — no sabe cuánto va a durar un trámite, así que la decisión es del Admin.
- RF-08.7: `ADMIN` registra la **devolución**, con observaciones libres ("volvió sin el cargador"). Devolver tres de cuatro no necesita nada especial: la cuarta simplemente sigue figurando afuera. Recibir dos veces la misma máquina **no es un error que corte nada** —pasa con dos Admin en el mostrador o un doble clic— y se informa.
- RF-08.8: Se conserva el **historial de entregas de cada PC**, que sobrevive al archivado del ciclo lectivo: al borrarse las reservas (RF-02.4) el préstamo queda sin reserva asociada, pero el registro de quién se llevó la máquina vale por sí mismo.
- RF-08.15: La **pantalla de inicio del `ADMIN` es el mostrador**: las clases en curso y las que siguen hoy —con cada máquina marcada como entregada, sin retirar o liberada—, lo que está fuera del laboratorio con su botón para recibirlo, y la entrega sin reserva a un clic. Se **refresca sola cada minuto**, porque el mostrador lo atienden varios Admin a la vez. La pantalla de entregas sigue existiendo como vista extendida y como destino de los avisos.
- RF-08.9: Todo esto es **solo `ADMIN`**, incluidas las lecturas. Que un docente pudiera marcarse la entrega a sí mismo convertiría el registro en una declaración en vez de en una constancia, que es justo lo que hace confiable al papel.

#### Lo que el sistema hace solo

- RF-08.10: Pasados unos **minutos de gracia** desde el inicio de la clase (por defecto 40), toda PC de esa reserva que **nadie haya retirado** deja de bloquear el horario y queda disponible para otro docente. Pasa a estado `NO_RETIRADA`, que **no es una cancelación**: nadie la decidió, y el reporte de uso (RF-06.1) deja de contarla como una clase dada. **Liberar no es prohibir**: si el docente llega más tarde y las máquinas siguen en el laboratorio, un `ADMIN` se las entrega igual — eso queda registrado como un préstamo, que es otra cosa. Una reserva **cuya PC está afuera no se libera**: si el docente vino y se la llevó, la reserva está cumplida. Y una reserva **más corta que el plazo de gracia** no se libera nunca, porque liberar los últimos minutos no le sirve a nadie.
- RF-08.11: **Una hora antes**, el docente recibe un recordatorio (campana y correo) con el horario, las PCs y **la regla de los minutos de gracia**. Se repite en cada recordatorio a propósito: sin esa línea, liberar la reserva después se lee como que el sistema se la quitó de prepo.
- RF-08.12: Pasados unos minutos de la hora de devolución (por defecto 10), se **reclama** la máquina que no volvió: a todos los `ADMIN` con la lista completa, y a quien la tiene si tiene cuenta en el sistema. El reclamo sale **una sola vez**. Un préstamo **sin hora pactada nunca se reclama** — "vengo en un rato" es una respuesta válida.
- RF-08.13: Al **cierre de la jornada** (hora configurable) se avisa qué computadoras quedaron afuera, incluidas las que salieron sin hora pactada. A diferencia del reclamo, **este aviso se repite** cada día que la máquina siga afuera. Va a los `ADMIN` y también **al docente de la próxima reserva de esa PC** — solo al siguiente, que es el único para quien es accionable.
- RF-08.14: Cualquiera puede **cambiar una PC de una reserva** por otra libre en la misma franja, sin cancelar ni volver a reservar. Sin esto, "elegí otra" partía la clase en dos `ReservaGrupo` y el docente la veía como dos reservas distintas.

> **Al docente de la próxima reserva se le avisa en `max(momento de la detección, inicio de su reserva − 1 hora)`.** No son dos reglas con una excepción, es una cuenta: si su clase es dentro de tres horas, el aviso espera; si es contigua o falta menos de una hora, sale al detectar la demora. Y lo más importante es lo que **no** hace: si la máquina vuelve antes de que llegue ese momento, el aviso **no sale nunca**. En el caso más común —alguien se demora quince minutos y devuelve— el docente de tres horas después no se entera de nada.
>
> Con una reserva contigua el correo llega tarde igual: el docente ya está yendo al laboratorio. Lo que resuelve ese caso es el reclamo al `ADMIN`, que sale a los diez minutos y lo arregla en persona.

> **Si la advertencia de una PC que no volvió cae junto con el recordatorio, viaja adentro de él.** Un solo correo por clase: mandar dos es el bombardeo que esto vino a evitar.

> **Ninguno de estos avisos depende de que el barrido corra a una hora exacta.** Cada uno deja su marca en la fila, así que correrlo cada cinco minutos, reiniciar el contenedor o estar caído dos horas cambia *cuándo* sale el aviso, nunca *cuántas veces*.

> **El barrido entero ignora los bloqueos por evaluación estatal (RF-04.7).** No los retira nadie —los crea un `ADMIN` para sacar máquinas de circulación—, así que no hay a quién recordarle ni a quién avisarle, y sobre todo **no se liberan**: hacerlo dejaría que otro docente reserve una computadora que está en una mesa de examen, con el examen en curso.

> **Un bloqueo por evaluación estatal (RF-04.7) no tiene docente**, así que no puede entregarse sin decir a nombre de quién: se informa por PC —no corta el lote— y con un nombre escrito a mano sí se entrega. Alguien tiene que retirar las máquinas de una mesa de examen.

> **Se puede entregar una PC en `EN_MANTENIMIENTO` o `FUERA_DE_SERVICIO`**, a diferencia de reservarla: llevarle una máquina rota al técnico es justamente un préstamo, y prohibirlo obligaría a sacarla del inventario para poder anotarlo. Lo único que se rechaza es entregar una PC **dada de baja**.

## 6. Requerimientos no funcionales
| ID | Requerimiento |
|---|---|
| RNF-01 | Docker sobre Huawei RH1288 V3 (Ubuntu Server 24.04) |
| RNF-02 | Exposición pública vía Cloudflare Tunnel |
| RNF-03 | **Monolito modular**: un solo binario Go, límites de dominio explícitos vía interfaces internas (ver `06-arquitectura.md`) |
| RNF-04 | HTTPS, JWT HS256, rate limiting, headers de seguridad |
| RNF-05 | Cobertura tests unitarios ≥ 80% en lógica crítica |
| RNF-06 | GitFlow + Conventional Commits |
| RNF-07 | Diseño responsive |

## 7. Fuera de alcance
- Multi-tenancy / múltiples instituciones (si el proyecto llegara a usarse en más de una escuela, ver `06-arquitectura.md` §5 sobre los límites de dominio pensados para permitir esa extensión sin reescribir la lógica de negocio)
- El email como canal principal de notificación. Hay correo saliente (RF-05.8 y RF-01.10), pero acotado: sin digest, sin preferencias por usuario y sin cola con reintentos. Los avisos internos son la fuente de verdad y el correo es una copia de cortesía.
- App móvil nativa
- Billing/facturación
- Gestión automatizada de calendario académico (feriados/días no hábiles)
- Restauración automática de reservas al volver una PC a `DISPONIBLE` (ver RF-03.8)
