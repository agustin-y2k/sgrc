# Diagramas de Secuencia — SGRC

> Nota: los "participantes" de estos diagramas (auth, academic, inventory, reservation, notification) son paquetes internos de un mismo binario Go (ver `06-arquitectura.md`), no procesos ni contenedores separados. Se mantienen como carriles separados en los diagramas porque son límites de dominio reales — la comunicación entre ellos es una llamada de función/interfaz, no HTTP ni mensajería.

## 0. Autorregistro de docente (con notificación a Admins)

```mermaid
sequenceDiagram
    actor U as Docente nuevo
    participant API as auth
    participant DB as PostgreSQL
    participant W as Worker outbox
    participant NOT as notification
    U->>API: POST /api/auth/registro
    API->>API: Validar datos, declaración y contraseña
    API->>DB: BEGIN e INSERT usuario DOCENTE/PENDIENTE
    API->>DB: Emitir evento docente.registro.pendiente en evento_pendiente
    alt Cuenta y evento guardados
        API->>DB: COMMIT
        API-->>U: 201 Cuenta pendiente de aprobación
        W->>DB: Leer evento pendiente y BEGIN de entrega
        W->>NOT: Crear avisos para admins aprobados
        NOT->>DB: INSERT notificaciones
        W->>DB: Marcar evento procesado y COMMIT
    else Email duplicado o fallo de persistencia
        API->>DB: ROLLBACK
        API-->>U: 409 por duplicado, o error de infraestructura
    end
```

La declaración de curso/materia se conserva como texto, no como asignación.
La ruta pública `GET /api/registro/opciones` ofrece nombres del ciclo activo,
pero registrarse no concede permisos para reservar ni administrar. Los avisos
internos tienen entrega durable; el correo es una copia de cortesía posterior
al commit, con un intento. Un fallo del consumidor conserva el evento para
reintentar sin confirmar avisos parciales.

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
            APP->>DB: UPDATE condicionado: password_hash, debe_cambiar_password=false, version_sesion=version_sesion+1
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

> El middleware exige HS256, expiración y cuenta vigente, con su rol y versión de sesión actuales. Un token con contraseña temporal sólo puede leer `/api/mi-perfil` y cambiar la contraseña; el resto responde 403. La restricción se aplica en el backend.

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
    FE->>RES: GET /api/equipos-disponibles?materiaId&fecha&horaInicio&horaFin
    RES->>INV: Listar equipos DISPONIBLE y reservables
    RES->>DB: SELECT reservas y bloqueos que pisan esa franja
    RES-->>FE: Libres y ocupados, con puedeTomarse y puedePedirse
    U->>FE: Tilda equipos libres o tomables por prioridad
    FE-->>U: Explica alcance, reposición y posibles faltantes
    U->>FE: Confirma la reserva
    FE->>RES: POST /api/reservas con equipoIds y equiposATomar
    RES->>RES: Verifica permiso sobre la materia (rol + docente_materia)
    RES->>DB: BEGIN y coordinación del lote
    RES->>RES: Valida jornada, fechas y conflictos actuales
    alt Sin conflictos no autorizados
        opt Hay tomas autorizadas por prioridad
            RES->>RES: Planifica reposiciones en cascada para el horario completo
            RES->>DB: Reasigna sustitutos y cancela faltantes puntuales
        end
        RES->>DB: Crea grupo y reservas, valida exclusión de solapamientos
        RES->>DB: Guarda auditoría y eventos pendientes, COMMIT
        RES-->>FE: 201 con grupo, reservas e impacto de desplazamientos
        FE-->>U: Reserva confirmada y resumen de repuestos y faltantes
    else Conflicto no autorizado o que no cumple las reglas
        RES->>DB: ROLLBACK de la operación completa
        RES-->>FE: 409 "no se pudo reservar: PC 7 ya está reservado el 13/03 de 10:00 a 12:00 (Ada Lovelace)"
        FE-->>U: El mensaje identifica el conflicto; se actualiza la disponibilidad
    end
```

> La creación es transaccional: un conflicto no autorizado o que no cumple las reglas de prioridad impide confirmar el lote. Una toma autorizada puede reponer otras reservas en cascada o cancelar faltantes puntuales; el conjunto se confirma o revierte junto. El cuerpo del 409 es texto plano y nombra el choque: equipo, día, franja y responsable.

> **La misma consulta responde las dos mitades.** Saber qué está libre en una franja exige leer las reservas y bloqueos que la pisan; devolver también quién tiene cada equipo tomado no agrega ninguna ida a la base, solo deja de descartar lo que ya se leyó (RF-04.11).

## 2b. Pedirle equipos a quien los tiene reservados (RF-04.12)

```mermaid
sequenceDiagram
    actor U as Solicitante autenticado
    participant RES as reservation
    participant DB as PostgreSQL
    participant W as Worker outbox
    participant NOT as notification
    U->>RES: POST /api/reservas/{id}/pedido-de-liberacion
    RES->>DB: BEGIN y bloquear reserva/grupo
    RES->>RES: Verificar reserva ajena, confirmada y todavía no iniciada
    RES->>DB: INSERT pedido_liberacion (reserva, solicitante, día)
    alt Reclamo válido y único
        RES->>DB: Persistir evento reserva.pedido-de-liberacion
        RES->>DB: COMMIT
        RES-->>U: 202 Pedido enviado
        W->>NOT: Entregar evento pendiente al dueño de la reserva
        NOT->>DB: Confirmar aviso y evento procesado juntos
    else Repetido o reserva no habilitada
        RES->>DB: ROLLBACK
        RES-->>U: Error de negocio, sin aviso nuevo
    end
```

El pedido no modifica la reserva ni concede prioridad. La tabla
`pedido_liberacion` sostiene la unicidad diaria; borrar o leer un aviso no
permite repetir el reclamo. La decisión de liberar sigue siendo del dueño.

## 3. Reserva recurrente (uno o varios equipos)

```mermaid
sequenceDiagram
    actor U as Docente/Admin
    participant FE as Frontend
    participant RES as reservation (paquete)
    participant DB as sgrc_db

    U->>FE: Materia, equipos elegidos, día de semana, horario, rango de fechas
    U->>FE: Comprobar serie
    FE->>RES: POST /api/reservas/recurrentes/previsualizar
    RES->>RES: Contar ocurrencias sin recorrer el rango y exigir 1 a 45
    RES->>DB: Consultar disponibilidad de todos los equipos y fechas
    RES-->>FE: Fechas, puedeCrear y unión de equiposATomar, sin escrituras
    alt La previsualización permite crear
        opt Alguna fecha requiere tomar por prioridad
            FE-->>U: Mostrar desplazamientos, reposición y posibles faltantes
            U->>FE: Dar consentimiento explícito para las fechas indicadas
        end
        Note over FE: Cambiar los datos invalida la comprobación y el consentimiento
        U->>FE: Confirmar serie
        FE->>RES: POST /api/reservas/recurrentes con equiposATomar autorizados
        RES->>DB: BEGIN y coordinación de la operación
        RES->>RES: Revalidar permisos, jornada y todas las fechas
        alt Sin conflictos no autorizados en la creación
            RES->>RES: Planificar reposiciones en cascada si corresponde
            RES->>DB: Guardar reasignaciones y cancelaciones por faltantes
            RES->>DB: INSERT regla_recurrencia
            loop por cada fecha calculada
                RES->>DB: INSERT reserva_grupo y sus reservas CONFIRMADA
            end
            RES->>DB: Guardar auditoría y eventos pendientes, COMMIT
            RES-->>FE: 201 con reglaId, grupos e impacto de desplazamientos
            FE-->>U: Serie creada, equipos repuestos y faltantes
        else Disponibilidad cambió o toma no permitida
            RES->>DB: ROLLBACK de la serie y la cadena
            RES-->>FE: 409 con los conflictos
            FE-->>U: No se creó ninguna ocurrencia
        end
    else Hay conflictos que impiden la serie
        FE-->>U: Mostrar fechas y equipos bloqueados, sin crear reservas
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
        FE->>RES: POST /api/grupos/{reservaGrupoId}/cancelar { soloEsta: true }
        RES->>DB: UPDATE reserva SET estado=CANCELADA WHERE reserva_grupo_id={id}
        RES->>DB: UPDATE reserva_grupo SET estado=CANCELADA WHERE id={id}
    else Esta fecha y siguientes
        U->>FE: "Esta y siguientes"
        FE->>RES: POST /api/grupos/{reservaGrupoId}/cancelar { soloEsta: false }
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
    participant EB as eventbus durable
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Selecciona los equipos, el rango fecha/hora y escribe el motivo
    FE->>RES: POST /api/bloqueos (JWT)
    RES->>DB: SELECT reserva CONFIRMADA en conflicto (solo esos equipos, ese rango exacto)
    DB-->>RES: [reservas puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva en conflicto
        RES->>DB: UPDATE reserva SET estado=CANCELADA, motivo_cancelacion='los equipos quedaron bloqueados: {motivo}'
        RES->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    RES->>DB: INSERT reserva tipo BLOQUEO con motivo_bloqueo (sin materia_id ni reserva_grupo_id) por cada equipo
    RES->>EB: Emitir reserva.cancelada en la outbox de la misma transacción
    EB->>NOTIF: Worker entrega después del commit; aviso y procesado atómicos
    NOTIF->>DB: INSERT notificación por docente, detallando qué equipos puntuales se cancelaron
    RES-->>FE: 201 { bloqueos: [...], reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ Equipos bloqueados. N reservas puntuales canceladas.
```

> Solo se cancelan las `Reserva` (equipo + fecha) que caen dentro del rango exacto del bloqueo. El resto de una recurrencia, o del mismo `ReservaGrupo` en otro horario, sigue vigente.

> **Un solo tipo de evento para los tres orígenes.** RF-05.1, 05.2 y 05.3 son,
> de punta a punta, la misma notificación con distinto motivo: cancelación
> manual de un Admin, bloqueo administrativo, o equipo fuera de servicio. El
> motivo ya viene armado desde `reservation`, así que `notification` no
> necesita saber de dónde vino. El evento se persiste **en la misma
> transacción** y el worker lo entrega después del commit. Un rollback elimina
> también el evento; un fallo de entrega conserva su reintento.

## 6. Cambio de estado de un equipo individual → cascada de cancelación

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant EB as eventbus durable
    participant NOTIF as notification (paquete)
    participant DB as sgrc_db

    ADM->>FE: Cambia estado del equipo a EN_MANTENIMIENTO/FUERA_DE_SERVICIO (+ motivo opcional)
    FE->>INV: PATCH /api/equipos/{id}/estado
    INV->>DB: UPDATE equipo SET estado=...
    INV->>DB: SELECT reserva CONFIRMADA de ese equipo puntual con fecha/hora futura
    DB-->>INV: [reservas puntuales con reserva_grupo_id, docenteId]
    loop por cada reserva afectada
        INV->>DB: UPDATE reserva SET estado=CANCELADA, cancelado_por=admin, motivo_cancelacion=(ingresado o por defecto)
        INV->>DB: Recalcular estado de reserva_grupo (PARCIALMENTE_CANCELADA o CANCELADA)
    end
    INV->>EB: Emitir reserva.cancelada en la outbox de la misma transacción
    EB->>NOTIF: Worker entrega después del commit; aviso y procesado atómicos
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
    FE->>INV: DELETE /api/equipos/{id}
    INV->>DB: SELECT: ¿está prestado y sin devolver?
    INV->>DB: UPDATE equipo SET dado_de_baja=true, fecha_baja=now()
    Note over INV,DB: Soft delete: no se borra la fila — incidencia y reserva la referencian por FK
    INV->>INV: Cancela las reservas futuras y avisa a cada docente
    INV-->>FE: 200 { reservasCanceladas: N, docentesNotificados: N }
    FE-->>ADM: ✅ Equipo dado de baja. Ya no aparece en listados activos ni puede reservarse.
```

> Un `DELETE` en la API es, puertas adentro, un soft delete: si se borrara la fila físicamente, se perdería la referencia de todo el historial de incidencias y reservas pasadas de ese equipo. Distinto del borrado de reservas al archivar un ciclo lectivo (§7), que sí es físico porque ahí se preserva un snapshot agregado aparte.

> **El `estado` no se toca.** El diagrama decía que la baja lo pasaba a `FUERA_DE_SERVICIO` y eso nunca fue cierto: `dado_de_baja` y `estado` son dos ejes distintos, y la baja sólo mueve el primero. Importa porque es lo que permite que deshacerla devuelva el equipo tal como estaba.

## 6c. Deshacer la baja de un equipo

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant INV as inventory (paquete)
    participant DB as sgrc_db

    ADM->>FE: Abre los equipos retirados del carro y devuelve uno al inventario
    FE->>INV: POST /api/equipos/{id}/reactivar
    INV->>DB: SELECT: ¿el carro del equipo sigue en circulación?
    Note over INV,DB: Si el carro se retiró en el medio, 409: el equipo quedaría<br/>dentro de algo que ninguna pantalla lista
    INV->>DB: UPDATE equipo SET dado_de_baja=false, fecha_baja=NULL
    Note over DB: Puede fallar con 409 contra uno de los tres índices únicos<br/>parciales: el zócalo, el nombre suelto o el número de serie<br/>que la baja liberó y otro equipo tomó
    INV-->>FE: 200
    FE-->>ADM: ✅ Volvió al inventario, con el estado que tenía
```

> **No revierte la cascada.** Las reservas que la baja canceló quedan canceladas y los avisos ya salieron. Lo que vuelve es la máquina; por eso esta respuesta no trae ningún contador, a diferencia de la baja.

> El carro sigue el mismo camino con `POST /api/carros/{id}/reactivar`, y ahí lo único que puede fallar es que otro carro se haya quedado con el nombre.

## 7. Archivar y clonar ciclo lectivo

```mermaid
sequenceDiagram
    actor A as Admin
    participant AC as academic
    participant RP as reporting
    participant RS as reservation
    participant DB as PostgreSQL
    A->>AC: POST /api/ciclos/{id}/archivar con clonarA opcional
    AC->>DB: BEGIN, coordinación exclusiva y ciclo FOR UPDATE
    AC->>DB: Buscar cierre_ciclo
    alt Cierre ya confirmado con el mismo destino
        DB-->>AC: Resultado persistido
        AC-->>A: 200 Mismo resultado
    else Primera ejecución
        AC->>AC: Validar ciclo y año de destino libre
        AC->>RP: GuardarSnapshotDeCiclo con contexto transaccional
        RP->>DB: Guardar históricos y sello snapshot_ciclo
        AC->>DB: Archivar ciclo, cursos, espacios y materias
        AC->>RS: EliminarReservasDeCiclo con el mismo contexto
        RS->>DB: Borrar reservas, grupos, reglas y bloqueos del año
        opt Se pidió clonar
            AC->>DB: Crear ciclo activo y clonar cursos, espacios y materias
        end
        AC->>DB: Guardar resultado en cierre_ciclo y COMMIT
        AC-->>A: 200 archivado, nuevoCicloId y conteos de clonado
    end
```

Snapshot, archivo, limpieza y clonado comparten una transacción. Un fallo
revierte todos los pasos. Repetir con otro destino responde 409; un archivo
legado sin constancia también se rechaza. Los históricos quedan sellados
antes de borrar su fuente. Se conservan estructura, asignaciones, incidencias
y préstamos; al borrar reservas, sus referencias en préstamos pueden quedar
nulas. Las asignaciones docentes no se clonan.

## 8. Dar de baja a un docente (permanente y recuperable)

```mermaid
sequenceDiagram
    actor A as Admin
    participant AU as auth
    participant AC as academic
    participant RS as reservation
    participant DB as PostgreSQL
    A->>AU: PATCH estado=BAJA
    AU->>DB: Transacción de preparación: BAJA y materias pendientes
    Note over AU,DB: La cuenta pierde acceso desde este commit
    loop Cada materia todavía pendiente
        AU->>DB: BEGIN con coordinación exclusiva y bloqueo por usuario
        AU->>AC: Verificar otros docentes aprobados
        opt La materia quedó sin docente activo
            AU->>RS: Cancelar reservas futuras con el mismo contexto
            RS->>DB: Actualizar reservas y grupos
            AU->>DB: Persistir evento docente.baja.materia-huerfana
        end
        AU->>DB: Marcar materia completada y COMMIT
    end
    alt Todas las materias completadas
        AU->>DB: Transacción final: remover vínculos y completar baja
        AU-->>A: 200
    else Una materia o la limpieza falló
        AU-->>A: 409 Baja pendiente; repetir estado=BAJA
    end
```

`baja_usuario` y `baja_usuario_materia` conservan el avance después de un
reinicio. Cada materia confirma cancelaciones, evento y avance juntos; el
reintento omite las completadas. Si queda otro docente activo no se cancela
ni avisa. La eliminación definitiva queda bloqueada hasta terminar la baja.
Los eventos confirmados se entregan por el worker durable.

Remover una asignación puntual usa otra operación exclusiva: elimina el
vínculo, comprueba docentes activos restantes y, si corresponde, cancela y
emite `docente.desasignado.materia-huerfana` en una sola transacción. Un fallo
restaura también el vínculo. El resto de materias y la cuenta se conservan.

## 8b. Eliminar definitivamente una cuenta cerrada

```mermaid
sequenceDiagram
    actor ADM as Admin
    participant FE as Frontend
    participant AUTH as auth (paquete)
    participant DB as sgrc_db

    ADM->>FE: Elimina definitivamente una cuenta en BAJA
    FE->>AUTH: DELETE /api/auth/usuarios/{id}
    AUTH->>DB: SELECT estado FROM usuario WHERE id={id}
    alt Cuenta no terminal o baja todavía pendiente
        AUTH-->>FE: 409 Solo se puede eliminar una cuenta en BAJA
    else BAJA completada o RECHAZADA
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
    participant AV as availability
    participant INV as inventory
    participant EB as eventbus durable
    participant DB as sgrc_db

    loop cada 5 minutos
        T1->>RES: FinalizarVencidas
        RES->>DB: UPDATE reserva SET estado=FINALIZADA<br/>WHERE estado=CONFIRMADA AND (fecha+hora_fin) < now()
        RES->>DB: Recalcular estado de los reserva_grupo afectados
    end

    loop cada 5 minutos
        T2->>RES: Vigilante.Barrer (RF-08.10, 08.11, 08.13)
        RES->>AV: ¿hay algún Admin de guardia? (RF-07.6)
        RES->>DB: recordatorios pendientes, corte de jornada
        RES->>EB: reserva.recordatorio,<br/>prestamo.sin-devolver.cierre
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

> **Liberar no publica nada** (RF-08.10) — ni a los 40 sin retiro, ni a los 15
> de una entrega parcial. Es el único punto del barrido donde una reserva cambia
> de estado sin que salga ningún aviso, y desde la 1.18.0 tampoco lo precede
> uno: el de los 15 minutos (RF-08.20) se retiró. Liberar le devuelve la máquina
> al resto de la escuela; no es un reproche que haya que anunciar.

> **Antes de deducir nada, el barrido pregunta si había alguien** (RF-07.6).
> Las dos pasadas que concluyen algo a partir de un registro FALTANTE —liberar
> y el corte de fin de jornada— consultan primero a `availability` si algún
> Admin tenía horario que cubriera ese momento
> y no declaró ausencia. Sin nadie atendiendo el mostrador, el barrido lee su
> propio silencio y no la realidad del laboratorio, así que no escribe ni avisa
> nada. El recordatorio es la excepción: no deduce nada de lo que falta.
>
> El corte de jornada usa la variante **por día** y no por instante, porque sale
> una hora después de que la escuela cerró: para entonces el Admin ya se fue, y
> preguntar por ese momento daría "no hay nadie" siempre.

## 10. Ver disponibilidad de Admins

```mermaid
sequenceDiagram
    actor U as Cualquier usuario autenticado
    participant FE as Frontend
    participant AVAIL as availability (paquete)
    participant DB as sgrc_db

    U->>FE: Abre la vista de disponibilidad
    FE->>AVAIL: GET /api/guardia
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

> Este cálculo **no habilita ni bloquea ninguna acción que haga una persona**
> (RF-07.6): reservar, entregar y aprobar funcionan igual esté quien esté. Pero
> desde la 1.18.0 **sí decide si el barrido automático puede sacar
> conclusiones** — es la misma consulta que responde `MostradorEn`. Un horario
> mal cargado ya no es solo una pantalla que miente.

## Contrato transaccional común

Las secuencias resumen los casos de uso; los locks y triggers transversales
se detallan en `06-arquitectura.md`. En producción, cambios sensibles y
auditoría se confirman juntos. Los eventos internos se persisten con su
operación y el worker confirma aviso/procesado juntos. El correo mantiene un
intento posterior al commit. Esto también aplica a los barridos: sus marcas
y eventos comparten transacción, sin publicar avisos de marcas revertidas.
