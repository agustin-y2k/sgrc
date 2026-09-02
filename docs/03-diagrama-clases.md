# Diagrama de Clases (Modelo de Dominio) — SGRC

Las entidades tal como viven en `internal/*/domain/`. La correspondencia con
las tablas está en `07-modelo-datos.md`; acá interesa el comportamiento —
qué sabe hacer cada entidad por sí sola.

```mermaid
classDiagram
    class Usuario {
        +UUID id
        +string nombre
        +string apellido
        +string email
        +string passwordHash
        +string googleSub
        +boolean debeCambiarPassword
        +RolUsuario rol
        +EstadoCuenta estado
        +DateTime fechaRegistro
        +DateTime fechaAprobacion
        +UUID aprobadoPor
        +string cursoSolicitado
        +string materiaSolicitada
        +int versionSesion
        +puedeTransicionarA(estado) boolean
    }

    class CodigoRecuperacion {
        +UUID id
        +UUID usuarioId
        +string codigoHash
        +DateTime creadoEn
        +DateTime expiraEn
        +DateTime usadoEn
        +int intentos
        +estaVigente(ahora) boolean
    }

    class Carro {
        +UUID id
        +string nombre
        +string descripcion
    }

    class Equipo {
        +UUID id
        +UUID carroId
        +int identificador
        +string nombre
        +string tipo
        +string numeroSerie
        +boolean reservable
        +boolean esComputadora
        +boolean freezado
        +string cpu
        +string ram
        +string sistemaOperativo
        +string softwareInstalado
        +EstadoEquipo estado
        +boolean dadoDeBaja
        +DateTime fechaBaja
        +DateTime fechaAlta
        +etiqueta() string
        +estaEnUnCarro() boolean
    }

    %% Equipo no es solo una computadora: también un proyector o un cargador,
    %% que no cuelgan de ningún carro (carroId e identificador quedan vacíos y
    %% los nombra `nombre`; numeroSerie es opcional en ellos y obligatorio en
    %% una computadora de carro). Comparten entidad para que "qué hay afuera
    %% del laboratorio" sea una sola lista. `etiqueta()` resuelve cómo se lo
    %% nombra en pantallas y correos: "PC 3" o el nombre.
    %%
    %% `esComputadora` es lo que decide qué se le pide y qué se le puede
    %% anotar: los cuatro campos de la ficha (cpu, ram, sistemaOperativo,
    %% softwareInstalado), `freezado` y las CuentaDeEquipo. Un cargador no
    %% tiene nada de eso; una notebook suelta lo tiene todo. No reemplaza a
    %% `tipo`, que dice QUÉ ES y sigue siendo texto libre: este dice QUÉ SE LE
    %% PREGUNTA. Lo que está en un carro lo es siempre.

    class CuentaDeEquipo {
        +UUID id
        +UUID equipoId
        +string usuario
        +string clase
        +PrivilegioDeCuenta privilegio
        +boolean tienePassword
        +string passwordCifrada
        +VisibilidadDeCuenta visibilidad
        +string notas
        +DateTime creadaEn
        +DateTime actualizadaEn
        +puedeVerLaPassword(esAdmin) boolean
        +hayPasswordParaVer() boolean
    }

    %% Con qué cuenta se entra a la máquina (RF-03.22). `tienePassword` y
    %% `passwordCifrada` son dos cosas distintas porque hay TRES estados: la
    %% cuenta libre, la que pide una contraseña que tenemos anotada, y la que
    %% pide una que no sabemos. `visibilidad` decide quién ve la CONTRASEÑA y
    %% es independiente del privilegio: hay cuentas de administrador de uso
    %% común y cuentas comunes reservadas.

    class LicenciaSoftware {
        +UUID id
        +UUID equipoId
        +string nombre
        +int diasDuracion
        +int diasAviso
        +Date fechaVencimiento
        +Date ultimaRenovacion
        +UUID vencimientoFijadoPor
        +DateTime vencimientoFijadoEn
        +Date avisadoPrevioPara
        +Date avisadoVencimientoPara
        +diasRestantes(hoy) int
        +estado(hoy) EstadoLicencia
        +renovar(fecha, por, ahora)
    }

    class Prestamo {
        +UUID id
        +UUID equipoId
        +UUID reservaId
        +UUID entregadoAUsuarioId
        +string entregadoANombre
        +string retiradoPor
        +string motivo
        +DateTime devolucionEstimada
        +UUID entregadoPor
        +DateTime entregadoEn
        +DateTime devueltoEn
        +UUID recibidoPor
        +string observaciones
        +Date avisadoCierrePara
        +estaAbierto() bool
        +demorado(ahora) bool
        +devolver(quien, obs, ahora)
    }

    class Incidencia {
        +UUID id
        +UUID equipoId
        +UUID reportadoPor
        +string descripcion
        +string categoria
        +Gravedad gravedad
        +DateTime fecha
        +boolean enviadoASoporte
        +DateTime fechaEnvioASoporte
        +EstadoIncidencia estado
    }

    class CicloLectivo {
        +UUID id
        +int anio
        +boolean activo
        +boolean archivado
    }

    class Curso {
        +UUID id
        +UUID cicloLectivoId
        +string nombre
        +boolean activo
        +boolean archivado
    }

    class Materia {
        +UUID id
        +UUID cursoId
        +string nombre
        +boolean activo
        +boolean archivado
    }

    class DocenteMateria {
        +UUID id
        +UUID usuarioId
        +UUID materiaId
        +RolDocente rol
    }

    class ReglaRecurrencia {
        +UUID id
        +UUID materiaId
        +UUID creadoPor
        +DiaSemana diaSemana
        +Time horaInicio
        +Time horaFin
        +Date fechaInicio
        +Date fechaFin
        +ocurrencias() Date[]
    }

    class ReservaGrupo {
        +UUID id
        +UUID materiaId
        +UUID creadoPor
        +string nombreDocenteSnapshot
        +Date fecha
        +Time horaInicio
        +Time horaFin
        +EstadoReservaGrupo estado
        +UUID reglaRecurrenciaId
        +DateTime creadaEn
        +DateTime recordatorioEnviadoEn
        +recalcularEstado(reservas)
    }

    class Reserva {
        +UUID id
        +UUID reservaGrupoId
        +UUID equipoId
        +UUID materiaId
        +string nombreDocenteSnapshot
        +Date fecha
        +Time horaInicio
        +Time horaFin
        +EstadoReserva estado
        +TipoReserva tipo
        +string motivoBloqueo
        +UUID creadoPor
        +DateTime creadaEn
        +UUID canceladoPor
        +string motivoCancelacion
        +DateTime canceladaEn
        +DateTime avisadoEquipoNoDisponibleEn
        +seSolapaCon(otra) boolean
        +cancelar(por, motivo, ahora)
    }

    class Notificacion {
        +UUID id
        +UUID usuarioId
        +UUID reservaId
        +UUID sobreUsuarioId
        +string mensaje
        +TipoNotif tipo
        +EstadoNotif estado
        +DateTime creadaEn
        +DateTime leidaEn
    }

    class HorarioAdmin {
        +UUID id
        +UUID usuarioId
        +DiaSemana diaSemana
        +Time horaInicio
        +Time horaFin
    }

    class HorarioAdminExcepcion {
        +UUID id
        +UUID usuarioId
        +Date fecha
        +TipoExcepcionHorario tipo
        +Time horaInicio
        +Time horaFin
        +string motivo
    }

    class HistoricoUsoEquipo {
        +UUID id
        +int anio
        +UUID equipoId
        +string etiquetaSnapshot
        +int identificadorSnapshot
        +string carroNombreSnapshot
        +int minutosReservados
        +int cantidadReservas
    }

    class HistoricoUsoDocente {
        +UUID id
        +int anio
        +UUID usuarioId
        +string nombreDocenteSnapshot
        +int cantidadReservas
        +int minutosTotales
    }

    Carro "1" --> "N" Equipo
    Equipo "1" --> "N" Incidencia
    Equipo "1" --> "N" LicenciaSoftware
    Equipo "1" --> "N" CuentaDeEquipo
    Equipo "1" --> "N" Prestamo
    Equipo "1" --> "N" Reserva
    Equipo "1" --> "N" HistoricoUsoEquipo
    Reserva "1" --> "N" Prestamo
    CicloLectivo "1" --> "N" Curso
    Curso "1" --> "N" Materia
    Materia "1" --> "N" DocenteMateria
    Materia "1" --> "N" ReservaGrupo
    Materia "1" --> "N" ReglaRecurrencia
    Usuario "1" --> "N" DocenteMateria
    Usuario "1" --> "N" ReservaGrupo : creadoPor
    Usuario "1" --> "N" Notificacion
    Usuario "1" --> "N" CodigoRecuperacion
    Usuario "1" --> "N" HorarioAdmin
    Usuario "1" --> "N" HorarioAdminExcepcion
    Usuario "1" --> "N" HistoricoUsoDocente
    ReglaRecurrencia "1" --> "N" ReservaGrupo : materializa
    ReservaGrupo "1" --> "N" Reserva : contiene (una por equipo)
```

## Enumeraciones

| Enum | Valores |
|---|---|
| `RolUsuario` | `ADMIN`, `DOCENTE` |
| `EstadoCuenta` | `PENDIENTE`, `APROBADA`, `RECHAZADA`, `BAJA` |
| `RolDocente` | `TITULAR`, `SUPLENTE` |
| `EstadoEquipo` | `DISPONIBLE`, `EN_MANTENIMIENTO`, `FUERA_DE_SERVICIO` |
| `EstadoReservaGrupo` | `CONFIRMADA`, `PARCIALMENTE_CANCELADA`, `CANCELADA`, `FINALIZADA`, `NO_RETIRADA` |
| `EstadoReserva` (por equipo) | `CONFIRMADA`, `CANCELADA`, `FINALIZADA`, `NO_RETIRADA` |
| `TipoReserva` | `NORMAL`, `BLOQUEO` |
| `Gravedad` | `LEVE`, `MODERADA`, `GRAVE` |
| `EstadoIncidencia` | `ABIERTA`, `EN_REPARACION`, `ENVIADA_A_SOPORTE`, `RESUELTA` |
| `EstadoNotif` | `NO_LEIDA`, `LEIDA` |
| `TipoNotif` | `GENERAL`, `DOCENTE_PENDIENTE`, `RESERVA_CANCELADA`, `LICENCIA_POR_VENCER`, `EQUIPO_SIN_DEVOLVER`, `PEDIDO_DE_LIBERACION`, `PEDIDO_DE_MATERIA`, `PEDIDO_DE_MATERIA_RESUELTO`, `SUGERENCIA`, `SUGERENCIA_RESPONDIDA` |
| `PrivilegioDeCuenta` | `COMUN`, `ADMINISTRADOR` — qué puede hacer esa cuenta en la máquina |
| `VisibilidadDeCuenta` | `PUBLICA`, `SOLO_ADMIN` — quién puede ver su **contraseña**; la cuenta y su privilegio se listan siempre |
| `EstadoLicencia` | `SIN_FECHA`, `VENCIDA`, `POR_VENCER`, `VIGENTE` — **derivado**, nunca una columna: se calcula contra la fecha de hoy |
| `DiaSemana` | `LUNES`…`DOMINGO` (los siete días; qué días opera la institución se declara, no se supone) |
| `TipoExcepcionHorario` | `NO_DISPONIBLE`, `HORARIO_MODIFICADO` |

Los cuatro campos que **no** son enums —`Equipo.tipo`,
`CuentaDeEquipo.clase`, `Incidencia.categoria` y `Reserva.motivoBloqueo`— son
los que escribe la institución. La regla es
simple: lo que el sistema interpreta es un enum, lo que describe una realidad
local es texto libre.

## Notas de diseño

- **`ReservaGrupo` vs `Reserva`**: un docente tilda varios equipos de una lista
  hasta juntar los que necesita, en una sola operación. `ReservaGrupo` es esa
  operación (una materia, fecha, horario); `Reserva` es cada equipo dentro de
  ella. Las cancelaciones en cascada actúan sobre filas `Reserva` puntuales — el
  grupo solo pasa a `CANCELADA` si terminan canceladas **todas**; si queda
  alguna en pie, pasa a `PARCIALMENTE_CANCELADA`.
- **`Reserva` de tipo `BLOQUEO` no pertenece a ningún `ReservaGrupo` ni
  `Materia`**: es un Admin tomando equipos puntuales, no la reserva de un
  docente para dar clase. En su lugar lleva `motivoBloqueo`, obligatorio.
- **`Equipo.etiqueta()` es el único lugar donde se decide cómo se nombra un
  equipo** (RF-03.17): "PC 3" si está en un carro, su nombre si no. Cualquier
  pantalla que arme el rótulo por su cuenta a partir del identificador y el
  carro muestra un proyector como "PC 0 · ".
- **`Equipo.freezado`** es informativo (Deep Freeze instalado), sin efecto sobre
  reservas. Vive a nivel de equipo y no de `Carro`: dentro de un mismo carro
  cada máquina puede tenerlo o no.
- **`ReglaRecurrencia` no guarda sus equipos**: una recurrencia semanal reserva
  varios equipos a la vez, pero eso vive en los `ReservaGrupo` que la regla
  materializa (uno por ocurrencia), cada uno con sus `Reserva`. Cancelar "esta y
  las siguientes" (RF-04.6) se resuelve por `reservaGrupo.reglaRecurrenciaId`.
- **`Usuario.estado = BAJA`** distingue una cuenta que estuvo activa de una que
  nunca fue aprobada (`RECHAZADA`). Al dar de baja a un docente, sus reservas
  futuras **no se cancelan** salvo que la materia quede sin ningún otro docente
  asignado (RF-02.8).
- **`nombreDocenteSnapshot`** vive en `ReservaGrupo` y se copia a cada
  `Reserva`: preserva el nombre aunque la cuenta se dé de baja o se elimine.
- **Archivado de cursos/materias**: `archivado = true` los oculta del sistema
  activo, pero a diferencia de `ReservaGrupo`/`Reserva`/`ReglaRecurrencia` —que
  se **eliminan físicamente** al archivar el ciclo— `Curso`/`Materia`/
  `DocenteMateria` se preservan. Es lo que evita recrearlos el año siguiente.
- **`HistoricoUsoEquipo` / `HistoricoUsoDocente`**: snapshot agregado
  permanente, calculado una sola vez al archivar un ciclo (no sincronizado
  continuamente). Es la única fuente de estadísticas para años cerrados, porque
  el detalle de sus reservas ya no existe.
- **`Equipo.dadoDeBaja` / `Equipo.fechaBaja`**: el "eliminar" de la interfaz es
  un soft delete — el equipo se oculta de los listados activos y no puede
  reservarse, pero conserva su historial de incidencias, préstamos y reservas.
- **`HorarioAdmin` es un patrón recurrente simple, sin versionar**: a diferencia
  de `ReglaRecurrencia` (que materializa una fila por ocurrencia porque cada una
  puede reservarse o cancelarse individualmente), acá no hace falta — se evalúa
  en el momento de la consulta. Editar un bloque cambia el patrón para todas las
  semanas futuras de inmediato.
- **`Prestamo` no es `Reserva`**: la reserva es el derecho a usar un equipo en
  una franja, el préstamo es quién tiene la máquina *ahora*. Existen por
  separado —reservas que nadie retiró, préstamos sin reserva, préstamos que
  sobreviven a su reserva—, y por eso tampoco hay un estado "prestado" en
  `Equipo`: se deriva de si hay un préstamo con `devueltoEn` en `null`.
- **`Prestamo.retiradoPor` no reemplaza a `entregadoANombre`** (RF-08.19): quien
  responde por el equipo es siempre quien reservó; quién vino a buscarlo se
  anota al lado y es opcional.
- **`LicenciaSoftware` no guarda "los días que faltan"**: `diasRestantes()` los
  calcula contra la fecha de hoy cada vez (RF-03.12). Un contador guardado
  necesitaría que alguien lo decremente todos los días, y bastaría un día de
  servidor apagado para dejarlo mal para siempre. Por lo mismo,
  `fechaVencimiento` en `null` significa "todavía no se verificó contra la
  máquina" y **no** "no vence nunca": es un estado legítimo, no un dato faltante
  que haya que completar con una fecha inventada.
- **`avisadoPrevioPara` / `avisadoVencimientoPara` guardan una fecha, no un
  booleano**: la fecha de vencimiento para la que ya salió cada aviso. Es lo que
  hace idempotente al barrido sin resetear nada — al renovar cambia
  `fechaVencimiento`, las marcas dejan de coincidir solas, y el ciclo nuevo
  vuelve a avisar.
- **`HorarioAdminExcepcion`** cubre tanto una excepción planificada como el
  botón rápido "marcarme no disponible ahora" (la misma fila, con
  `tipo = NO_DISPONIBLE` y `fecha = hoy`). Si existe una excepción para la fecha
  consultada, siempre pisa al patrón semanal de `HorarioAdmin`.
</content>
