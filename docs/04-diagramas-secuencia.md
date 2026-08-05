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

    U->>FE: nombre, apellido, email, password<br/>+ qué curso y materia va a dictar (opcional)
    FE->>AUTH: POST /api/auth/registro
    AUTH->>DB: SELECT usuario WHERE lower(email) = lower($1)
    alt Email libre
        AUTH->>DB: INSERT usuario (estado=PENDIENTE,<br/>curso_solicitado, materia_solicitada)
        AUTH->>EB: Publish("docente.registro.pendiente", { usuarioId, nombre, apellido })
        EB->>NOTIF: Subscribe handler ejecuta en la misma goroutine/worker
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

## 2. Reservar PCs (una o varias, en un solo grupo)

```mermaid
sequenceDiagram
    actor U as Docente/Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant INV as inventory (paquete)
    participant DB as sgrc_db

    U->>FE: Selecciona materia, fecha, horario
    FE->>RES: GET /api/inventory/pcs/disponibles?fecha&horaInicio&horaFin
    RES->>INV: Listar PCs DISPONIBLE sin solapamiento en ese horario
    INV-->>FE: Lista de PCs para tildar
    U->>FE: Tilda las PCs que necesita (una o varias)
    FE->>RES: POST /api/reservations { materiaId, fecha, horaInicio, horaFin, pcIds: [...] }
    RES->>RES: Verifica permiso sobre la materia (rol + docente_materia)
    RES->>DB: INSERT reserva_grupo (CONFIRMADA)
    loop por cada pcId
        RES->>DB: INSERT reserva (EXCLUDE constraint valida solapamiento por PC)
    end
    alt Todas las PCs sin solapamiento
        DB-->>RES: OK
        RES-->>FE: 201 { reservaGrupoId, pcsConfirmadas: N }
        FE-->>U: ✅ Reserva confirmada (N PCs)
    else Alguna PC con solapamiento
        DB-->>RES: Constraint violation en esa PC puntual
        RES->>DB: SELECT reserva en conflicto para esa PC
        RES->>DB: ROLLBACK — no se confirma ninguna PC del grupo
        RES-->>FE: 409 { conflictos: [{ pcId, docente, materia, horario }] }
        FE-->>U: ❌ PC-07 ocupada por [nombre] - destildá esa PC y reintentá
    end
```

> La creación es transaccional: si una sola PC del grupo tiene solapamiento, no se confirma ninguna — el usuario ve exactamente cuál PC falló y puede destildarla sin perder el resto de la selección.

## 3. Reserva recurrente (una o varias PCs)

```mermaid
sequenceDiagram
    actor U as Docente/Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant DB as sgrc_db

    U->>FE: Materia, PCs elegidas, día semana, horario, rango fechas
    FE->>RES: POST /api/reservations/recurrentes { materiaId, pcIds: [...], diaSemana, ... }
    RES->>RES: Calcula todas las ocurrencias (cada fecha × cada PC elegida)
    RES->>DB: SELECT solapamientos para TODAS las fechas x PCs
    alt Sin conflictos
        DB-->>RES: []
        RES->>DB: INSERT regla_recurrencia
        loop por cada fecha calculada
            RES->>DB: INSERT reserva_grupo (CONFIRMADA)
            loop por cada PC elegida
                RES->>DB: INSERT reserva (CONFIRMADA)
            end
        end
        RES-->>FE: 201 { reglaRecurrenciaId, fechasCreadas: [...] }
        FE-->>U: ✅ N fechas × M PCs creadas
    else Con conflictos
        DB-->>RES: [{ fecha, pcId, docente, materia, horario }, ...]
        RES-->>FE: 409 { conflictos: [...] }
        FE-->>U: ❌ Tabla de conflictos (por fecha y PC) — resolver antes de continuar
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
        FE->>RES: DELETE /api/reservations/grupos/{reservaGrupoId}
        RES->>DB: UPDATE reserva SET estado=CANCELADA WHERE reserva_grupo_id={id}
        RES->>DB: UPDATE reserva_grupo SET estado=CANCELADA WHERE id={id}
    else Esta fecha y siguientes
        U->>FE: "Esta y siguientes"
        FE->>RES: DELETE /api/reservations/recurrentes/{reglaId}?desde={fecha}
        RES->>DB: SELECT reserva_grupo WHERE regla_recurrencia_id={reglaId} AND fecha >= {fecha}
        RES->>DB: UPDATE reserva SET estado=CANCELADA WHERE reserva_grupo_id IN (...)
        RES->>DB: UPDATE reserva_grupo SET estado=CANCELADA WHERE id IN (...)
        RES-->>FE: 200 { gruposCancelados: N }
    end
    FE-->>U: ✅ Cancelada(s)
```

## 5. Bloqueo de PCs para evaluación estatal

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Selecciona carro, rango fecha/hora (definido)
    FE->>RES: POST /api/reservations/bloqueo-evaluacion (JWT)
    RES->>DB: SELECT PCs del carro
    RES->>DB: SELECT reserva CONFIRMADA en conflicto (solo esas PCs, ese rango exacto)
    DB-->>RES: [reserva puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva en conflicto
        RES->>DB: UPDATE reserva SET estado=CANCELADA, motivo='Evaluación estatal'
        RES->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    RES->>DB: INSERT reserva tipo EVALUACION_ESTATAL (sin materia_id ni reserva_grupo_id) por cada PC del carro
    RES->>EB: Publish("reserva.cancelada", { usuarioId, reservaId, motivo }) por reserva
    EB->>NOTIF: Subscribe handler ejecuta en la misma goroutine/worker
    NOTIF->>DB: INSERT notificación por docente, detallando qué PCs puntuales se cancelaron
    RES-->>FE: 201 { bloqueoId, reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ PCs bloqueadas. N reservas puntuales canceladas.
```

> Solo se cancelan las `Reserva` (PC + fecha) que caen dentro del rango exacto del bloqueo. El resto de una recurrencia, o del mismo `ReservaGrupo` en otro horario, sigue vigente.

> **Un solo tipo de evento para los tres orígenes.** RF-05.1, 05.2 y 05.3 son,
> de punta a punta, la misma notificación con distinto motivo: cancelación
> manual de un Admin, bloqueo por evaluación, o PC fuera de servicio. El
> motivo ya viene armado desde `reservation`, así que `notification` no
> necesita saber de dónde vino. Los eventos se publican **después del
> commit**: si la transacción se deshace, nadie recibe el aviso de una
> cancelación que no ocurrió.

## 6. Cambio de estado de PC individual → cascada de cancelación

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant EB as eventbus (in-process)
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Cambia estado de PC a EN_MANTENIMIENTO/FUERA_DE_SERVICIO (+ motivo opcional)
    FE->>INV: PATCH /api/inventory/pcs/{id}/estado
    INV->>DB: UPDATE pc SET estado=...
    INV->>DB: SELECT reserva CONFIRMADA de esa PC puntual con fecha/hora futura
    DB-->>INV: [reserva puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva afectada
        INV->>DB: UPDATE reserva SET estado=CANCELADA, cancelado_por=admin, motivo_cancelacion=(ingresado o por defecto)
        INV->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    INV->>EB: Publish("reserva.cancelada", { usuarioId, reservaId, motivo }) por reserva
    EB->>NOTIF: Subscribe handler ejecuta en la misma goroutine/worker
    NOTIF->>DB: INSERT notificación por docente, detallando que fue esa PC puntual
    INV-->>FE: 200 { estado, reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ PC actualizada. N reservas canceladas y docentes notificados.
```

> Duración **indefinida** (a diferencia del bloqueo de evaluación, que tiene rango definido): se cancelan todas las reservas futuras de esa PC puntual, sin fecha de corte. Cuando la PC vuelve a `DISPONIBLE`, nada se restaura automáticamente.

## 6b. Dar de baja una PC del inventario (soft delete)

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant DB as sgrc_db

    ADM->>FE: Elimina una PC del inventario
    FE->>INV: DELETE /api/inventory/pcs/{id}
    INV->>DB: UPDATE pc SET dada_de_baja=true, fecha_baja=now(), estado=FUERA_DE_SERVICIO
    Note over INV,DB: Soft delete: no se borra la fila — incidencia y reserva la referencian por FK
    INV-->>FE: 200 { dadaDeBaja: true }
    FE-->>ADM: ✅ PC dada de baja. Ya no aparece en listados activos ni puede reservarse.
```

> Un `DELETE` en la API es, puertas adentro, un soft delete: si se borrara la fila físicamente, se perdería la referencia de todo el historial de incidencias y reservas pasadas de esa PC. Distinto del borrado de reservas al archivar un ciclo lectivo (§7), que sí es físico porque ahí se preserva un snapshot agregado aparte.

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
    REP->>DB: INSERT historico_uso_pc, historico_uso_docente (permanentes)
    ACAD->>RES: EliminarReservasDeCiclo(cicloId)
    RES->>DB: DELETE reserva WHERE reserva_grupo_id IN (SELECT id FROM reserva_grupo WHERE materia_id IN [...])
    RES->>DB: DELETE reserva_grupo WHERE materia_id IN [...]
    RES->>DB: DELETE regla_recurrencia WHERE materia_id IN [...]
    Note over RES,DB: incidencia NO se toca — pertenece a la PC, no al ciclo
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
    EB->>NOTIF: Subscribe handlers ejecutan en la misma goroutine/worker
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
        AUTH-->>FE: 200 { eliminado: true }
    end
    FE-->>ADM: ✅ Cuenta eliminada. El email queda libre para un registro nuevo.
```

## 9. Liberar reservas vencidas (job interno)

```mermaid
sequenceDiagram
    participant CRON as Scheduler interno (goroutine + ticker)
    participant RES as reservation (paquete)
    participant DB as sgrc_db

    loop cada 5 minutos
        CRON->>RES: Trigger (time.Ticker, sin proceso externo)
        RES->>DB: UPDATE reserva SET estado=FINALIZADA WHERE estado=CONFIRMADA AND (fecha+hora_fin) < now()
        RES->>DB: Recalcular estado de reserva_grupo afectados (FINALIZADA si ninguna PC quedó cancelada)
    end
```

> El job corre como una goroutine que arranca junto con `main.go` (`time.NewTicker(5 * time.Minute)`), sin pieza de infraestructura adicional ni proceso externo.

## 10. Ver disponibilidad de Admins

```mermaid
sequenceDiagram
    actor U as Cualquier usuario autenticado
    participant FE as Frontend
    participant AVAIL as availability (paquete)
    participant DB as sgrc_db

    U->>FE: Abre la vista de disponibilidad
    FE->>AVAIL: GET /api/availability/admins
    loop por cada Admin
        AVAIL->>DB: SELECT excepción de hoy (usuario_id, fecha=hoy)
        alt Existe excepción NO_DISPONIBLE
            AVAIL->>AVAIL: disponibleAhora = false
        else Existe excepción HORARIO_MODIFICADO
            AVAIL->>AVAIL: comparar hora actual vs horario de la excepción
        else Sin excepción hoy
            AVAIL->>DB: SELECT bloques de horario_admin para el día de semana de hoy
            AVAIL->>AVAIL: comparar hora actual vs esos bloques
        end
    end
    AVAIL-->>FE: 200 [{ admin, disponibleAhora, horarioSemanal }, ...]
    FE-->>U: Lista de Admins con su estado y horario de referencia
```

> Puramente informativo (RF-07.6): este cálculo no habilita ni bloquea ninguna otra acción del sistema — es solo para que el docente sepa cuándo pasar por el laboratorio.
