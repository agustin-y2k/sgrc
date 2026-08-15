// Espeja internal/reservation/interfaces/http/dto.go y el
// materiaReservableResponse de academic.

export type EstadoReserva =
  | "CONFIRMADA"
  | "CANCELADA"
  | "FINALIZADA"
  /**
   * RF-08.10 — pasaron los minutos de gracia y nadie vino a buscar esa
   * máquina, así que dejó de estar reservada.
   *
   * No es una cancelación: nadie la decidió. Y liberar no es prohibir — si
   * la computadora sigue en el laboratorio, un Admin se la entrega igual.
   */
  | "NO_RETIRADA"
/**
 * NORMAL: la reserva de un docente para su clase.
 * BLOQUEO: un Admin se tomó el equipo para otra cosa y canceló lo que
 * hubiera encima. El motivo va en texto libre: se bloquea por una
 * evaluación, una jornada docente, una capacitación o una obra en el aula, y
 * el sistema no puede prever la lista.
 */
export type TipoReserva = "NORMAL" | "BLOQUEO"

// Los siete días. El sistema no supone qué días abre la institución: hay
// escuelas de jornada extendida y albergue que dictan el fin de semana, y
// antes ni siquiera podían expresar "todos los sábados".
export type DiaSemana =
  | "LUNES"
  | "MARTES"
  | "MIERCOLES"
  | "JUEVES"
  | "VIERNES"
  | "SABADO"
  | "DOMINGO"

export const DIAS_SEMANA: { valor: DiaSemana; etiqueta: string }[] = [
  { valor: "LUNES", etiqueta: "Lunes" },
  { valor: "MARTES", etiqueta: "Martes" },
  { valor: "MIERCOLES", etiqueta: "Miércoles" },
  { valor: "JUEVES", etiqueta: "Jueves" },
  { valor: "VIERNES", etiqueta: "Viernes" },
  { valor: "SABADO", etiqueta: "Sábado" },
  { valor: "DOMINGO", etiqueta: "Domingo" },
]

/**
 * Hoy en formato YYYY-MM-DD, para el `min` de los inputs de fecha: el
 * backend rechaza un bloque que ya terminó (domain.ErrReservaEnElPasado).
 *
 * Se arma con los componentes locales por la misma razón que esDiaLectivo:
 * `toISOString()` pasa por UTC y al oeste de Greenwich devuelve el día
 * siguiente a partir de las 21:00.
 */
export function hoyISO(): string {
  const hoy = new Date()
  const mes = String(hoy.getMonth() + 1).padStart(2, "0")
  const dia = String(hoy.getDate()).padStart(2, "0")
  return `${hoy.getFullYear()}-${mes}-${dia}`
}

/** Espeja domain.MaxDuracionReserva: un turno completo. */
export const MAX_HORAS_RESERVA = 8

/**
 * Avisa en el formulario en vez de esperar el 400 de
 * domain.ErrDuracionExcesiva. No aplica a los bloqueos administrativos
 * estatal, que están exceptuados del tope.
 */
export function excedeDuracionMaxima(horaInicio: string, horaFin: string): boolean {
  const minutos = (hhmm: string): number | null => {
    const [h, m] = hhmm.split(":").map(Number)
    return Number.isFinite(h) && Number.isFinite(m) ? h * 60 + m : null
  }
  const inicio = minutos(horaInicio)
  const fin = minutos(horaFin)
  if (inicio === null || fin === null) return false // incompleto: que valide el backend
  return fin - inicio > MAX_HORAS_RESERVA * 60
}

export type Reserva = {
  id: string
  reservaGrupoId?: string
  equipoId: string
  materiaId?: string
  nombreDocenteSnapshot?: string
  /** YYYY-MM-DD */
  fecha: string
  /** HH:MM */
  horaInicio: string
  horaFin: string
  estado: EstadoReserva
  tipo: TipoReserva
  /**
   * Por qué se tomaron los equipos. Solo viene en los BLOQUEO, y ahí viene
   * siempre: es obligatorio al crearlos porque cancelan clases ajenas.
   */
  motivoBloqueo?: string
  creadoPor?: string
  canceladoPor?: string
  motivoCancelacion?: string
  canceladaEn?: string
}

/**
 * Una Reserva con los nombres que resuelve el JOIN de `GET
 * /api/reservation/reservas` (espeja reservaDetalladaResponse). Sin ellos
 * la pantalla solo tendría UUIDs y no podría decir de qué Equipo ni de qué
 * materia es cada reserva.
 *
 * Es un tipo aparte y no campos opcionales de `Reserva` porque los otros
 * endpoints que devuelven reservas —crear una, crear un bloqueo por
 * evaluación— responden `reservaResponse` pelado: declararlos ahí sería
 * prometer datos que no llegan.
 */
export type ReservaDetallada = Reserva & {
  /** Cómo se nombra el equipo: "PC 3" o "Proyector Epson". */
  etiqueta: string
  /** 0 en un equipo suelto; el carro, vacío. Lo que se muestra es `etiqueta`. */
  identificador: number
  carroNombre: string
  /** Vacío en los bloqueos administrativos, que no tienen materia. */
  materiaNombre?: string
  cursoNombre?: string
  /**
   * Presente solo si la reserva es parte de una serie recurrente. No
   * confundir con `reservaGrupoId`, que tienen TODAS las reservas normales
   * (es el grupo de Equipos de una misma fecha).
   */
  reglaRecurrenciaId?: string
}

/**
 * Las Reserva de un mismo ReservaGrupo, juntas.
 *
 * El glosario define ReservaGrupo como "la reserva tal como la percibe el
 * docente": una materia, una fecha, un horario, con N equipos adentro. La API
 * devuelve las filas sueltas (una por Equipo), así que el agrupado se arma acá.
 */
export type GrupoDeReservas = {
  /** Ausente si la reserva no pertenece a ningún grupo. */
  grupoId?: string
  /**
   * Un bloqueo administrativo (RF-04.7). No tiene ReservaGrupo en la base
   * —no es la reserva de nadie— pero sí es UNA operación del Admin, así que
   * se junta en una sola tarjeta.
   */
  esBloqueo: boolean
  /** Por qué se tomaron los equipos. Solo en los bloqueos. */
  motivoBloqueo?: string
  fecha: string
  horaInicio: string
  horaFin: string
  materiaNombre?: string
  cursoNombre?: string
  nombreDocenteSnapshot?: string
  creadoPor?: string
  esRecurrente: boolean
  reservas: ReservaDetallada[]
}

/**
 * La clave por la que dos filas `reserva` caen en la misma tarjeta.
 *
 * Una reserva normal trae su `reservaGrupoId`: es lo que el docente vivió
 * como "una reserva". Un bloqueo administrativo NO tiene grupo en la base
 * —no pertenece a nadie ni a ninguna materia— pero para el Admin que lo
 * creó sí fue una sola operación: eligió varias equipos, una fecha y un
 * horario, y apretó confirmar una vez. Mostrarlo como una tarjeta por Equipo
 * hacía que bloquear ocho equipos se viera como ocho bloqueos distintos.
 *
 * Se agrupa por quién lo creó, la fecha y el horario, que es exactamente lo
 * que define una operación de bloqueo. Dos bloqueos distintos con los mismos
 * tres datos se juntarían en una tarjeta, pero desde afuera son
 * indistinguibles: son las mismas Equipos, el mismo día y la misma franja.
 */
function claveDeAgrupacion(r: ReservaDetallada): string {
  if (r.reservaGrupoId) return r.reservaGrupoId
  if (r.tipo === "BLOQUEO") {
    return `bloqueo:${r.creadoPor ?? "sistema"}:${r.fecha}:${r.horaInicio}:${r.horaFin}`
  }
  return `sin-grupo:${r.id}`
}

/**
 * Agrupa las filas `reserva` en lo que cada usuario percibe como "una
 * reserva" (ver claveDeAgrupacion), conservando el orden en que vinieron
 * (el backend ordena por fecha, hora e identificador de Equipo).
 */
export function agruparReservas(reservas: ReservaDetallada[]): GrupoDeReservas[] {
  const grupos = new Map<string, GrupoDeReservas>()

  for (const r of reservas) {
    const clave = claveDeAgrupacion(r)
    const existente = grupos.get(clave)
    if (existente) {
      existente.reservas.push(r)
      continue
    }
    grupos.set(clave, {
      grupoId: r.reservaGrupoId,
      esBloqueo: r.tipo === "BLOQUEO",
      motivoBloqueo: r.motivoBloqueo,
      fecha: r.fecha,
      horaInicio: r.horaInicio,
      horaFin: r.horaFin,
      materiaNombre: r.materiaNombre,
      cursoNombre: r.cursoNombre,
      nombreDocenteSnapshot: r.nombreDocenteSnapshot,
      creadoPor: r.creadoPor,
      esRecurrente: r.reglaRecurrenciaId !== undefined,
      reservas: [r],
    })
  }

  return [...grupos.values()]
}

/** RF-04.1: una materia en la que el usuario autenticado puede reservar. */
export type MateriaReservable = {
  materiaId: string
  materiaNombre: string
  cursoId: string
  cursoNombre: string
  cicloId: string
  cicloAnio: number
}

/** RF-04.2: un equipo libre en la franja consultada. */
export type EquipoDisponible = {
  equipoId: string
  /** 0 en un equipo suelto. Lo que se muestra es `etiqueta`. */
  identificador?: number
  /** "PC 3" o "Proyector Epson". */
  etiqueta: string
  tipo?: string
  carroId: string
  carroNombre: string
  freezado: boolean
  softwareInstalado?: string
  /**
   * RF-03.21: en qué bloque cae el equipo para la materia que se está
   * reservando. La lista ya viene ordenada por tramo; esto permite
   * titularlos.
   *
   * No es un permiso: los tres tramos se reservan igual.
   */
  tramo: TramoPreferencia
  /** "Preferente para Matemática de 3°B". Ausente en un equipo neutral. */
  motivo?: string
}

export type TramoPreferencia = "PREFERENTE" | "NEUTRAL" | "DE_OTRA_MATERIA"

/**
 * RF-04.11: un equipo que ya tiene dueño en esa franja. No se tilda; está
 * para poder ir a hablarle o mandarle un pedido.
 *
 * De la otra persona llega el nombre y nunca el email: el correo lo manda el
 * servidor.
 */
export type EquipoOcupado = {
  equipoId: string
  etiqueta: string
  carroNombre?: string
  /** Lo que después recibe el pedido de liberación. */
  reservaId?: string
  /** Vacío en un bloqueo administrativo, que no tiene docente detrás. */
  docenteNombre?: string
  materiaNombre?: string
  /** Solo en un bloqueo: el texto que escribió el Admin (RF-04.7). */
  motivo?: string
  /** De la reserva que lo ocupa, que puede no coincidir con la franja pedida. */
  horaInicio: string
  horaFin: string
  /**
   * Lo decide el servidor: false en un bloqueo, en una reserva propia y si
   * esa franja ya empezó. La pantalla no replica la regla — dos copias de
   * una regla terminan discrepando.
   */
  puedePedirse: boolean
}

export type CrearReservaRequest = {
  materiaId: string
  fecha: string
  horaInicio: string
  horaFin: string
  equipoIds: string[]
}

export type CrearReservaRecurrenteRequest = {
  materiaId: string
  diaSemana: DiaSemana
  horaInicio: string
  horaFin: string
  fechaInicio: string
  fechaFin: string
  equipoIds: string[]
}

/**
 * RF-04.7 — bloqueo administrativo de equipos.
 *
 * A diferencia de una reserva no lleva materia (el bloqueo no es de nadie)
 * y sí lleva `motivo`, que es lo que el backend intercala en el aviso a
 * cada docente afectado: "Tu reserva fue cancelada: bloqueo administrativo
 * estatal (…)".
 */
export type BloquearRequest = {
  equipoIds: string[]
  fecha: string
  horaInicio: string
  horaFin: string
  /** Obligatorio: el bloqueo cancela las clases de otros (RF-04.7). */
  motivo: string
}

/**
 * Lo que hay que mostrarle al Admin después: cuántas reservas ajenas se
 * llevó puesta la cascada y a cuántos docentes se les avisó. Es la única
 * devolución que tiene de una operación destructiva.
 */
export type ResultadoBloqueo = {
  bloqueos: Reserva[]
  reservasCanceladas: number
  docentesNotificados: number
}

// ── Entregas y devoluciones (RF-08) ───────────────────────────────────
//
// Espeja internal/reservation/interfaces/http/dto_prestamos.go.
//
// Un `Prestamo` NO es una reserva, y la diferencia es la razón de ser de
// todo esto: la reserva es el derecho a usar un equipo en una franja, el
// préstamo es dónde está la máquina ahora. Existen por separado —hay
// reservas que nadie vino a buscar y préstamos sin reserva detrás— y por eso
// son dos cosas distintas también acá.

export type Prestamo = {
  id: string
  equipoId: string
  /** Ausente = préstamo espontáneo, sin reserva detrás. */
  reservaId?: string

  entregadoAUsuarioId?: string
  /** Quién RESPONDE por el equipo: contra una reserva, siempre el docente. */
  entregadoANombre: string
  /** Quién vino a buscarlo, si no fue quien responde. Ausente = fue él. */
  retiradoPor?: string
  motivo?: string

  /** ISO 8601. Ausente = no se pactó hora; no se le reclama nada. */
  devolucionEstimada?: string
  entregadoPor?: string
  entregadoEn: string
  /** Ausente = la máquina todavía está afuera. */
  devueltoEn?: string
  recibidoPor?: string
  observaciones?: string

  /**
   * Derivados: los calcula el backend contra su propio reloj. Vienen
   * resueltos por la misma razón que el contador de las licencias — si los
   * calculara el navegador, un reloj corrido mostraría una demora distinta
   * de la que el sistema va a reclamar.
   */
  abierto: boolean
  demorado: boolean
  minutosDeDemora?: number

  /** Ubicación. Solo viene en los listados. */
  identificador?: number
  /** "PC 3" o "Proyector Epson". La resuelve el backend. */
  etiqueta?: string
  carroNombre?: string
  /** Solo en préstamos que salieron contra una reserva. */
  materiaNombre?: string
}

/**
 * Por qué un equipo del lote no salió. El código permite ofrecer la acción que
 * corresponde: "ver quién la tiene" no es lo mismo que "revisá el
 * inventario".
 */
export type RazonNoEntregada =
  | "YA_ENTREGADA"
  | "FUERA_DEL_INVENTARIO"
  | "RESERVA_CANCELADA"
  /** La reserva no dice a nombre de quién: es un bloqueo administrativo. */
  | "SIN_DESTINATARIO"

export type EquipoNoEntregada = {
  equipoId: string
  razon: RazonNoEntregada
  detalle: string
}

/**
 * Aviso de que una máquina recién entregada tiene una reserva encima. No
 * impidió nada: es información para que el Admin decida. El sistema no sabe
 * cuánto va a durar un trámite.
 */
export type ReservaProxima = {
  equipoId: string
  fecha: string
  horaInicio: string
  horaFin: string
  docente?: string
}

export type ResultadoEntrega = {
  entregadas: Prestamo[]
  noEntregadas?: EquipoNoEntregada[]
  avisos?: ReservaProxima[]
}

export type ResultadoDevolucion = {
  recibidos: Prestamo[]
  noRecibidos?: { prestamoId: string; detalle: string }[]
}
