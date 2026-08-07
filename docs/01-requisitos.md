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
- RF-05.8: **Copias por email** de algunos de estos avisos, para que no dependan de que la persona tenga el sistema abierto. Son tres: a todos los `ADMIN` cuando alguien queda `PENDIENTE` (RF-05.6 — una cuenta que nadie mira es un docente que no puede trabajar); a la persona cuando le aprueban la cuenta (RF-02); y el código de RF-01.10.
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
