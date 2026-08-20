# Diagramas de Secuencia — SGRC

> Nota: los "participantes" de estos diagramas (auth, academic, inventory, reservation, notification) son paquetes internos de un mismo binario Go (ver `06-arquitectura.md`), no procesos ni contenedores separados. Se mantienen como carriles separados en los diagramas porque son límites de dominio reales — la comunicación entre ellos es una llamada de función/interfaz, no HTTP ni mensajería.

## 0. Autorregistro de docente (con notificación a Admins)

```mermaid
sequenceDiagram
    actor U as Docente nuevo
    participant FE as Frontend
    participant AUTH as auth (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    U->>FE: nombre, apellido, email, password<br/>+ cargo y titular/suplente (obligatorios)<br/>+ qué curso y materia va a dictar (opcional, solo docentes)
    FE->>AUTH: POST /api/auth/registro
    AUTH->>DB: SELECT usuario WHERE lower(email) = lower($1)
    alt Email libre
        AUTH->>DB: INSERT usuario (rol=DOCENTE, estado=PENDIENTE,<br/>cargo_solicitado, rol_solicitado,<br/>curso_solicitado, materia_solicitada)
        AUTH->>EB: Publish("docente.registro.pendiente", { usuarioId, nombre, apellido })
        EB->>NOTIF: Publish es sincrónico; el handler se va a su propia goroutine
        NOTIF->>DB: SELECT usuario WHERE rol=ADMIN AND estado=APROBADA
        NOTIF->>DB: INSERT notificación tipo=DOCENTE_PENDIENTE para cada Admin (RF-05.6)
        AUTH-->>FE: 201 Cuenta creada, pendiente de aprobación
    else Email pertenece a una cuenta en BAJA
        AUTH-->>FE: 409 "Este email pertenece a una cuenta dada de baja — pedile a un Admin que la elimine"
    else Email ya registrado (cuenta activa)
        AUTH-->>FE: 409 Email ya registrado
    end
    FE-->>U: Mensaje según corresponda
```

> **Lo que declara al registrarse no es una referencia.** `curso_solicitado`
> y `materia_solicitada` son texto libre: la persona todavía no está
> autenticada, así que no puede elegir de una lista, y lo que escribe puede
> no existir en el sistema — de hecho el Admin quizás lo tenga que crear al
> aprobarla. Es lo que la pantalla de aprobación muestra para que sepa a qué
> asignarla (RF-02.6).

> **La notificación lleva `tipo`**, no solo texto: es lo que le permite a la
> pantalla del Admin ofrecer "ir a aprobar" sin interpretar el mensaje. Si la
> acción dependiera de la redacción, cambiarle una palabra al aviso rompería
> el botón.

## 1. Login

```mermaid
sequenceDiagram
    actor U as Usuario
    participant FE as Frontend
    participant APP as sgrc-app
    participant DB as sgrc_db

    U->>FE: email + password
    FE->>APP: POST /api/auth/login
    APP->>DB: SELECT usuario WHERE lower(email) = lower($1)
    alt El email no existe
        APP->>APP: argon2id contra un hash de descarte
        Note over APP: Tarda lo mismo que un login real:<br/>si no, el tiempo revela qué cuentas existen
        APP-->>FE: 401 credenciales inválidas
    end
    APP->>APP: Verificar argon2id hash
    alt Cuenta APROBADA
        APP-->>FE: JWT { userId, rol, nombre, apellido } + debeCambiarPassword
        alt debeCambiarPassword == true
            FE-->>U: Pantalla forzada de cambio de contraseña (no deja operar el resto del sistema)
            U->>FE: passwordActual + passwordNueva
            FE->>APP: POST /api/auth/cambiar-password
            APP->>DB: UPDATE usuario SET password_hash=..., debe_cambiar_password=false
            APP-->>FE: 200 + JWT nuevo (el anterior lleva dcp=true congelado)
            FE-->>U: Dashboard
        else debeCambiarPassword == false
            FE-->>U: Dashboard
        end
    else Cuenta PENDIENTE, RECHAZADA o BAJA
        APP-->>FE: 403 Cuenta no habilitada
        FE-->>U: Mensaje según el estado
    end
```

> **Cada request posterior vuelve a mirar la cuenta.** El token prueba la
> identidad, pero el middleware consulta el estado y el rol en la base antes
> de dejar pasar: una cuenta dada de baja pierde el acceso en el momento, no
> cuando expira su token (ver `09-seguridad-rbac.md` §1). El rol que vale es
> el de la base, no el del token.

> El JWT se emite igual con la contraseña temporal — `debeCambiarPassword` es una bandera que el frontend usa para bloquear la navegación hasta que se cambie, no una restricción a nivel de token.

## 2. Reservar equipos (uno o varios, en un solo grupo)

```mermaid
sequenceDiagram
    actor U as Docente/Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant INV as inventory (paquete)
    participant DB as sgrc_db

    U->>FE: Selecciona materia, fecha, horario
    Note over FE: Antes de la franja no hay lista que mostrar
    FE->>RES: GET /api/reservation/equipos-disponibles?fecha&horaInicio&horaFin
    RES->>INV: Listar equipos DISPONIBLE y reservables
    RES->>DB: SELECT reservas y bloqueos que pisan esa franja
    RES-->>FE: { data: libres para tildar, ocupados: con docente/materia/franja o motivo }
    U->>FE: Tilda los equipos que necesita (uno o varios)
    FE->>RES: POST /api/reservation/reservas { materiaId, fecha, horaInicio, horaFin, equipoIds: [...] }
    RES->>RES: Verifica permiso sobre la materia (rol + docente_materia)
    RES->>DB: INSERT reserva_grupo (CONFIRMADA)
    loop por cada equipoId
        RES->>DB: INSERT reserva (EXCLUDE constraint valida solapamiento por equipo)
    end
    alt Todos los equipos sin solapamiento
        DB-->>RES: OK
        RES-->>FE: 201 { reservaGrupoId, equiposConfirmados: N }
        FE-->>U: ✅ Reserva confirmada (N equipos)
    else Algún equipo con solapamiento
        DB-->>RES: Constraint violation en ese equipo puntual
        RES->>DB: SELECT reserva en conflicto para ese equipo
        RES->>DB: ROLLBACK — no se confirma ningún equipo del grupo
        RES-->>FE: 409 "no se pudo reservar: PC 7 ya está reservado el 13/03 de 10:00 a 12:00 (Ada Lovelace)"
        FE-->>U: ❌ El mensaje dice cuál destildar; se recarga la lista de libres
    end
```

> La creación es transaccional: si un solo equipo del grupo tiene solapamiento, no se confirma ninguno. El cuerpo del 409 es texto plano —el backend usa el `ErrorHandler` por defecto de Fiber— pero ese texto **nombra el choque**: qué equipo, qué día, qué franja y con quién. El dato lo trae la misma consulta que lo detectó, así que no cuesta nada más.

> **La misma consulta responde las dos mitades.** Saber qué está libre en una franja exige leer las reservas y bloqueos que la pisan; devolver también quién tiene cada equipo tomado no agrega ninguna ida a la base, solo deja de descartar lo que ya se leyó (RF-04.11).

## 2b. Pedirle equipos a quien los tiene reservados (RF-04.12)

```mermaid
sequenceDiagram
    actor U as Docente que necesita el equipo
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant EB as eventbus
    participant NOT as notification (paquete)
    actor D as Docente dueño de la reserva

    U->>FE: En la lista de la franja, "pedir" sobre un equipo tomado
    FE->>RES: POST /api/reservation/reservas/{id}/pedido-de-liberacion { mensaje? }
    RES->>RES: ¿La reserva está CONFIRMADA y su franja todavía no empezó?
    RES->>NOT: ¿Ya hay un pedido de este solicitante por esta reserva hoy?
    alt Ya pidió hoy, o la reserva no admite pedido
        RES-->>FE: 409 "ya le pediste esos equipos hoy" / "esa reserva ya empezó"
        FE-->>U: ❌ No se manda nada
    else Pedido válido
        RES->>EB: reserva.pedido-de-liberacion
        EB->>NOT: notificación interna al dueño (PEDIDO_DE_LIBERACION)
        EB->>NOT: copia por correo (fuera del request)
        NOT-->>D: 🔔 + ✉️ "Fulano necesita PC 3 y PC 4 el jueves de 10 a 12"
        RES-->>FE: 202 Pedido enviado
        FE-->>U: ✅ "Le avisamos. La reserva sigue siendo de él hasta que decida"
    end
```

> **El pedido no escribe nada sobre las reservas.** No hay tabla propia, no hay estado que resolver y quien pidió no queda con prioridad sobre el equipo: si el dueño lo libera, vuelve a la lista de libres para todos. La única fila que se crea es la notificación, y de paso es la que sostiene la regla de "un pedido por reserva, por solicitante y por día".

> **El acuerdo ocurre afuera.** Lo que el sistema aporta es que el pedido llegue a alguien que quizá no se cruce con quien pide; lo que sigue —negociar, ceder, decir que no— no tiene por qué pasar por acá, y modelarlo costaría un flujo de aceptación con caducidad, carrera contra terceros y una pantalla de pendientes.

## 3. Reserva recurrente (uno o varios equipos)

```mermaid
sequenceDiagram
    actor U as Docente/Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant DB as sgrc_db

    U->>FE: Materia, equipos elegidos, día de semana, horario, rango de fechas
    FE->>RES: POST /api/reservation/reservas/recurrentes { materiaId, equipoIds: [...], diaSemana, ... }
    RES->>RES: Calcula todas las ocurrencias (cada fecha × cada equipo elegido)
    RES->>DB: UNA consulta: todos los equipos × todas las fechas, un rango horario
    alt Sin conflictos
        DB-->>RES: []
        RES->>DB: INSERT regla_recurrencia
        loop por cada fecha calculada
            RES->>DB: INSERT reserva_grupo (CONFIRMADA)
            loop por cada equipo elegido
                RES->>DB: INSERT reserva (CONFIRMADA)
            end
        end
        RES-->>FE: 201 { reglaRecurrenciaId, fechasCreadas: [...] }
        FE-->>U: ✅ N fechas × M equipos creados
    else Con conflictos
        DB-->>RES: [{ equipo, fecha, horario, docente }, ...]
        RES-->>FE: 409 nombrando qué equipo choca y en qué fecha
        FE-->>U: ❌ No se creó ninguna ocurrencia
    end
```

## 4. Cancelar ocurrencia recurrente

```mermaid
sequenceDiagram
    actor U as Usuario
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant DB as sgrc_db

    U->>FE: Clic en el grupo de reserva recurrente del calendario
    FE-->>U: Popup "¿Solo esta fecha o esta y siguientes?"
    alt Solo esta fecha
        U->>FE: "Solo esta"
        FE->>RES: POST /api/reservation/grupos/{reservaGrupoId}/cancelar { soloEsta: true }
        RES->>DB: UPDATE reserva SET estado=CANCELADA WHERE reserva_grupo_id={id}
        RES->>DB: UPDATE reserva_grupo SET estado=CANCELADA WHERE id={id}
    else Esta fecha y siguientes
        U->>FE: "Esta y siguientes"
        FE->>RES: POST /api/reservation/grupos/{reservaGrupoId}/cancelar { soloEsta: false }
        RES->>DB: SELECT reserva_grupo WHERE regla_recurrencia_id={reglaId} AND fecha >= {fecha}
        RES->>DB: UPDATE reserva SET estado=CANCELADA WHERE reserva_grupo_id IN (...)
        RES->>DB: UPDATE reserva_grupo SET estado=CANCELADA WHERE id IN (...)
        RES-->>FE: 200 { reservasCanceladas: N }
    end
    FE-->>U: ✅ Cancelada(s)
```

## 5. Bloqueo administrativo de equipos

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Selecciona los equipos, el rango fecha/hora y escribe el motivo
    FE->>RES: POST /api/reservation/bloqueos (JWT)
    RES->>DB: SELECT reserva CONFIRMADA en conflicto (solo esos equipos, ese rango exacto)
    DB-->>RES: [reservas puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva en conflicto
        RES->>DB: UPDATE reserva SET estado=CANCELADA, motivo_cancelacion='los equipos quedaron bloqueados: {motivo}'
        RES->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    RES->>DB: INSERT reserva tipo BLOQUEO con motivo_bloqueo (sin materia_id ni reserva_grupo_id) por cada equipo
    RES->>EB: Publish("reserva.cancelada", { usuarioId, reservaId, motivo }) por reserva
    EB->>NOTIF: Publish es sincrónico; el handler se va a su propia goroutine
    NOTIF->>DB: INSERT notificación por docente, detallando qué equipos puntuales se cancelaron
    RES-->>FE: 201 { bloqueos: [...], reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ Equipos bloqueados. N reservas puntuales canceladas.
```

> Solo se cancelan las `Reserva` (equipo + fecha) que caen dentro del rango exacto del bloqueo. El resto de una recurrencia, o del mismo `ReservaGrupo` en otro horario, sigue vigente.

> **Un solo tipo de evento para los tres orígenes.** RF-05.1, 05.2 y 05.3 son,
> de punta a punta, la misma notificación con distinto motivo: cancelación
> manual de un Admin, bloqueo administrativo, o equipo fuera de servicio. El
> motivo ya viene armado desde `reservation`, así que `notification` no
> necesita saber de dónde vino. Los eventos se publican **después del
> commit**: si la transacción se deshace, nadie recibe el aviso de una
> cancelación que no ocurrió.

## 6. Cambio de estado de un equipo individual → cascada de cancelación

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Cambia estado del equipo a EN_MANTENIMIENTO/FUERA_DE_SERVICIO (+ motivo opcional)
    FE->>INV: PATCH /api/inventory/equipos/{id}/estado
    INV->>DB: UPDATE equipo SET estado=...
    INV->>DB: SELECT reserva CONFIRMADA de ese equipo puntual con fecha/hora futura
    DB-->>INV: [reservas puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva afectada
        INV->>DB: UPDATE reserva SET estado=CANCELADA, cancelado_por=admin, motivo_cancelacion=(ingresado o por defecto)
        INV->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    INV->>EB: Publish("reserva.cancelada", { usuarioId, reservaId, motivo }) por reserva
    EB->>NOTIF: Publish es sincrónico; el handler se va a su propia goroutine
    NOTIF->>DB: INSERT notificación por docente, detallando que fue ese equipo puntual
    INV-->>FE: 200 { estado, reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ Equipo actualizado. N reservas canceladas y docentes notificados.
```

> Duración **indefinida** (a diferencia del bloqueo administrativo, que tiene rango definido): se cancelan todas las reservas futuras de ese equipo puntual, sin fecha de corte. Cuando el equipo vuelve a `DISPONIBLE`, nada se restaura automáticamente.

## 6b. Dar de baja un equipo del inventario (soft delete)

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant DB as sgrc_db

    ADM->>FE: Elimina un equipo del inventario
    FE->>INV: DELETE /api/inventory/equipos/{id}
    INV->>DB: UPDATE equipo SET dado_de_baja=true, fecha_baja=now(), estado=FUERA_DE_SERVICIO
    Note over INV,DB: Soft delete: no se borra la fila — incidencia y reserva la referencian por FK
    INV-->>FE: 200 { reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ Equipo dado de baja. Ya no aparece en listados activos ni puede reservarse.
```

> Un `DELETE` en la API es, puertas adentro, un soft delete: si se borrara la fila físicamente, se perdería la referencia de todo el historial de incidencias y reservas pasadas de ese equipo. Distinto del borrado de reservas al archivar un ciclo lectivo (§7), que sí es físico porque ahí se preserva un snapshot agregado aparte.

## 7. Archivar y clonar ciclo lectivo

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant ACAD as academic (paquete)
    participant RES as reservation (paquete)
    participant REP as reporting (paquete)
    participant DB as sgrc_db

    ADM->>FE: Archivar ciclo {año} y clonar a {año+1}
    FE->>ACAD: POST /api/academic/ciclos/{id}/archivar { clonarA: año+1 }
    ACAD->>REP: CalcularSnapshotAnual(cicloId) — antes de borrar nada
    REP->>DB: SELECT agregados de reserva/reserva_grupo de las materias del ciclo
    REP->>DB: INSERT historico_uso_equipo, historico_uso_docente (permanentes)
    ACAD->>RES: EliminarReservasDeCiclo(cicloId)
    RES->>DB: DELETE reserva WHERE reserva_grupo_id IN (SELECT id FROM reserva_grupo WHERE materia_id IN [...])
    RES->>DB: DELETE reserva_grupo WHERE materia_id IN [...]
    RES->>DB: DELETE regla_recurrencia WHERE materia_id IN [...]
    RES->>DB: DELETE reserva WHERE tipo='BLOQUEO' AND año(fecha) = año del ciclo
    Note over RES,DB: incidencia y prestamo NO se tocan — no dependen del ciclo
    ACAD->>DB: UPDATE curso SET archivado=true WHERE ciclo_lectivo_id
    ACAD->>DB: UPDATE materia SET archivado=true WHERE curso_id IN [...]
    ACAD->>DB: UPDATE ciclo_lectivo SET archivado=true, activo=false
    ACAD->>DB: INSERT ciclo_lectivo { anio: año+1, activo: true }
    ACAD->>DB: INSERT cursos clonados (sin archivado, nuevos ids)
    ACAD->>DB: INSERT materias clonadas (sin archivado, nuevos ids)
    Note over ACAD,DB: DocenteMateria NO se clona
    ACAD-->>FE: 200 { nuevoCicloId, cursosClonados, materiasClonadas, reservasEliminadas }
    FE-->>ADM: ✅ Nuevo ciclo listo. Estadísticas del año anterior guardadas. Asignar docentes a materias.
```

> Las reservas de las materias del ciclo archivado **se eliminan físicamente**, no quedan como historial en detalle. El snapshot histórico se calcula primero para no perder las estadísticas — `curso`/`materia`/`docente_materia` sí se preservan (`archivado=true`), porque eso es lo que evita recrearlos.

## 8. Dar de baja a un docente (permanente)

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant AUTH as auth (paquete)
    participant ACAD as academic (paquete)
    participant RES as reservation (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Da de baja a un docente
    FE->>AUTH: PATCH /api/auth/usuarios/{id}/estado { estado: BAJA }
    AUTH->>DB: UPDATE usuario SET estado=BAJA WHERE id={id}
    AUTH->>ACAD: ObtenerMateriasDeDocente(usuarioId) — llamada directa vía interfaz
    ACAD-->>AUTH: [materiaId, ...]
    loop por cada materia
        AUTH->>ACAD: ¿Queda otro DocenteMateria con usuario APROBADA en esta materia?
        alt Queda otro docente activo
            ACAD-->>AUTH: Sí
            AUTH->>EB: Publish("docente.baja.notificar_admin", { usuarioId, materiaId })
        else No queda ningún docente
            ACAD-->>AUTH: No
            AUTH->>RES: CancelarReservasFuturasDeMateria(materiaId)
            RES->>DB: UPDATE reserva SET estado=CANCELADA<br/>WHERE materia_id={materiaId} AND (fecha + hora_fin) > ahora
            AUTH->>EB: Publish("docente.baja.materia-huerfana", { usuarioId, materiaId, reservasCanceladas })
        end
    end
    AUTH->>ACAD: EliminarDocenteMateria(usuarioId) — recién ahora, después de resolver el destino de las reservas
    ACAD->>DB: DELETE FROM docente_materia WHERE usuario_id={usuarioId}
    EB->>NOTIF: Publish es sincrónico; los handlers se van a su propia goroutine
    NOTIF->>DB: INSERT notificación(es) a todos los ADMIN (aviso informativo o cancelación aplicada)
    AUTH-->>FE: 200
    FE-->>ADM: ✅ Docente dado de baja. Revisar notificaciones si hay materias afectadas.
```

> El orden importa por dos razones. `EliminarDocenteMateria` corre **después**
> del loop que revisa cada materia: si se borrara primero, la pregunta "¿queda
> otro docente activo?" ya no tendría con qué compararse. Y las cancelaciones
> van antes de borrar los vínculos, porque la cascada cruza a `reservation`
> con su propia transacción y el conjunto no puede ser atómico: si algo falla,
> los vínculos se conservan y el error dice qué materias quedaron pendientes,
> en vez de dejar reservas vivas en materias que ya nadie sabe cuáles eran.

> **RF-02.10 hace lo mismo por el otro camino.** Quitar a un docente de **una
> sola** materia (`DELETE /materias/{id}/docentes/{dmId}`) llega al mismo
> estado —materia sin nadie a cargo, con reservas futuras vivas— así que
> dispara la misma cascada, con el mismo orden (cancelar y después borrar el
> vínculo) y su propio evento, `docente.desasignado.materia-huerfana`. La
> respuesta devuelve `reservasCanceladas` para que el Admin sepa qué se llevó
> puesto.

## 8b. Eliminar definitivamente una cuenta en BAJA

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant AUTH as auth (paquete)
    participant DB as sgrc_db

    ADM->>FE: Elimina definitivamente una cuenta en BAJA
    FE->>AUTH: DELETE /api/auth/usuarios/{id}
    AUTH->>DB: SELECT estado FROM usuario WHERE id={id}
    alt estado != BAJA
        AUTH-->>FE: 409 Solo se puede eliminar una cuenta en BAJA
    else estado == BAJA
        AUTH->>DB: DELETE FROM usuario WHERE id={id}
        Note over AUTH,DB: docente_materia y notificaciones propias caen en CASCADE.<br/>reserva_grupo.creado_por, reserva.creado_por/cancelado_por,<br/>incidencia.reportado_por, usuario.aprobado_por quedan en NULL (SET NULL).
        AUTH-->>FE: 200 sin cuerpo
    end
    FE-->>ADM: ✅ Cuenta eliminada. El email queda libre para un registro nuevo.
```

## 9. Los barridos periódicos (sin proceso externo)

Hay **tres barridos**, cada uno en su propia goroutine arrancada por
`cmd/main.go` con un `time.Ticker` y registrada en un `sync.WaitGroup` para que
el apagado ordenado los espere. Ninguno necesita cron ni un proceso aparte.

```mermaid
sequenceDiagram
    participant T1 as ticker 5 min
    participant T2 as ticker 5 min
    participant T3 as ticker 1 h
    participant RES as reservation
    participant INV as inventory
    participant EB as eventbus
    participant DB as sgrc_db

    loop cada 5 minutos
        T1->>RES: FinalizarVencidas
        RES->>DB: UPDATE reserva SET estado=FINALIZADA<br/>WHERE estado=CONFIRMADA AND (fecha+hora_fin) < now()
        RES->>DB: Recalcular estado de los reserva_grupo afectados
    end

    loop cada 5 minutos
        T2->>RES: Vigilante.Barrer (RF-08.10 a 08.13, 08.20)
        RES->>DB: recordatorios pendientes, reservas sin retirar,<br/>préstamos demorados, corte de jornada
        RES->>EB: reserva.recordatorio, reserva.sin-retirar,<br/>prestamo.demorado, prestamo.sin-devolver.cierre
        RES->>DB: marcar cada fila como avisada
        RES->>DB: liberar lo que venció su plazo (sin publicar nada)
    end

    loop cada hora, solo a partir de LICENCIAS_HORA_AVISO
        T3->>INV: AvisadorDeLicencias.Barrer (RF-03.14)
        INV->>DB: SELECT licencias que entraron en su ventana de aviso
        INV->>EB: licencia.por-vencer (todas juntas, un solo aviso)
        INV->>DB: marcar avisado_previo_para / avisado_vencimiento_para
    end
```

> **`FinalizarVencidas` va por lotes**, cada uno en su propia transacción, con
> corte por falta de progreso y un tope de lotes por ciclo del ticker: el
> primer barrido de una base con años de reservas no puede quedarse tomando un
> lock encima de la tabla que usa el resto del sistema.

> **Ningún aviso depende de que el barrido corra a una hora exacta.** Cada uno
> deja su marca en la fila, así que reiniciar el proceso o estar caído dos
> horas cambia *cuándo* sale el aviso, nunca *cuántas veces* (RF-08).

> **El aviso y la liberación son dos momentos distintos** (RF-08.20 y RF-08.10).
> A los 15 minutos del inicio, si no salió ninguna máquina de esa reserva, el
> barrido publica `reserva.sin-retirar` y marca `aviso_sin_retirar_en`: ese es el
> único aviso, y llega cuando el docente todavía puede ir, cambiar la máquina o
> cancelar. **Liberar no publica nada** — ni a los 40 sin retiro, ni a los 15 de
> una entrega parcial. Es el único punto del barrido donde una reserva cambia de
> estado sin que salga un aviso, y es a propósito: el correo ya salió antes, y
> repetirlo al liberar sería un segundo mensaje por la misma clase para contar
> algo que ya no se puede cambiar.

## 10. Ver disponibilidad de Admins

```mermaid
sequenceDiagram
    actor U as Cualquier usuario autenticado
    participant FE as Frontend
    participant AVAIL as availability (paquete)
    participant DB as sgrc_db

    U->>FE: Abre la vista de disponibilidad
    FE->>AVAIL: GET /api/availability/admins
    AVAIL->>DB: SELECT admins con rol=ADMIN y estado=APROBADA
    AVAIL->>DB: SELECT bloques de horario_admin de TODOS esos ids
    AVAIL->>DB: SELECT excepciones de TODOS esos ids con fecha = hoy
    loop en memoria, por cada Admin
        AVAIL->>AVAIL: excepción de hoy si existe (siempre pisa al patrón),<br/>si no, los bloques de este día de semana
    end
    AVAIL-->>FE: 200 [{ admin, disponibleAhora, horarioSemanal }, ...]
    FE-->>U: Lista de Admins con su estado y horario de referencia
```

> **Dos consultas en total, no dos por Admin.** Resolverlo dentro del bucle
> serían 2N viajes a la base para armar una pantalla que mira cualquier
> docente; el cálculo en sí no toca la base, así que se traen los dos conjuntos
> de una vez y se cruzan en memoria.

> Puramente informativo (RF-07.6): este cálculo no habilita ni bloquea ninguna otra acción del sistema — es solo para que el docente sepa cuándo pasar por el laboratorio.
