// Espeja internal/reservation/interfaces/http/dto.go y el
// materiaReservableResponse de academic.

export type EstadoReserva = "CONFIRMADA" | "CANCELADA" | "FINALIZADA"
export type TipoReserva = "NORMAL" | "EVALUACION_ESTATAL"

// La semana lectiva es de lunes a viernes: el backend rechaza reservar un
// sábado o un domingo (domain.ErrDiaNoLectivo).
export type DiaSemana = "LUNES" | "MARTES" | "MIERCOLES" | "JUEVES" | "VIERNES"

export const DIAS_SEMANA: { valor: DiaSemana; etiqueta: string }[] = [
  { valor: "LUNES", etiqueta: "Lunes" },
  { valor: "MARTES", etiqueta: "Martes" },
  { valor: "MIERCOLES", etiqueta: "Miércoles" },
  { valor: "JUEVES", etiqueta: "Jueves" },
  { valor: "VIERNES", etiqueta: "Viernes" },
]

/**
 * Espeja domain.EsDiaLectivo del backend, para avisar en el formulario en
 * vez de esperar el 400.
 *
 * Se construye la fecha con componentes locales a propósito: `new
 * Date("2026-08-08")` la interpreta como medianoche UTC, y al oeste de
 * Greenwich eso cae el día anterior — un sábado se leería como viernes.
 */
export function esDiaLectivo(fechaISO: string): boolean {
  const [anio, mes, dia] = fechaISO.split("-").map(Number)
  if (!anio || !mes || !dia) return true // incompleta: que valide el backend
  const diaSemana = new Date(anio, mes - 1, dia).getDay()
  return diaSemana !== 0 && diaSemana !== 6
}

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
 * domain.ErrDuracionExcesiva. No aplica a los bloqueos por evaluación
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
  pcId: string
  materiaId?: string
  nombreDocenteSnapshot?: string
  /** YYYY-MM-DD */
  fecha: string
  /** HH:MM */
  horaInicio: string
  horaFin: string
  estado: EstadoReserva
  tipo: TipoReserva
  creadoPor?: string
  canceladoPor?: string
  motivoCancelacion?: string
  canceladaEn?: string
}

/**
 * Una Reserva con los nombres que resuelve el JOIN de `GET
 * /api/reservation/reservas` (espeja reservaDetalladaResponse). Sin ellos
 * la pantalla solo tendría UUIDs y no podría decir de qué PC ni de qué
 * materia es cada reserva.
 *
 * Es un tipo aparte y no campos opcionales de `Reserva` porque los otros
 * endpoints que devuelven reservas —crear una, crear un bloqueo por
 * evaluación— responden `reservaResponse` pelado: declararlos ahí sería
 * prometer datos que no llegan.
 */
export type ReservaDetallada = Reserva & {
  pcIdentificador: number
  carroNombre: string
  /** Vacío en los bloqueos por evaluación estatal, que no tienen materia. */
  materiaNombre?: string
  cursoNombre?: string
  /**
   * Presente solo si la reserva es parte de una serie recurrente. No
   * confundir con `reservaGrupoId`, que tienen TODAS las reservas normales
   * (es el grupo de PCs de una misma fecha).
   */
  reglaRecurrenciaId?: string
}

/**
 * Las Reserva de un mismo ReservaGrupo, juntas.
 *
 * El glosario define ReservaGrupo como "la reserva tal como la percibe el
 * docente": una materia, una fecha, un horario, con N PCs adentro. La API
 * devuelve las filas sueltas (una por PC), así que el agrupado se arma acá.
 */
export type GrupoDeReservas = {
  /** Ausente si la reserva no pertenece a ningún grupo. */
  grupoId?: string
  /**
   * Un bloqueo por evaluación (RF-04.7). No tiene ReservaGrupo en la base
   * —no es la reserva de nadie— pero sí es UNA operación del Admin, así que
   * se junta en una sola tarjeta.
   */
  esBloqueoEvaluacion: boolean
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
 * como "una reserva". Un bloqueo por evaluación NO tiene grupo en la base
 * —no pertenece a nadie ni a ninguna materia— pero para el Admin que lo
 * creó sí fue una sola operación: eligió varias PCs, una fecha y un
 * horario, y apretó confirmar una vez. Mostrarlo como una tarjeta por PC
 * hacía que bloquear ocho PCs se viera como ocho bloqueos distintos.
 *
 * Se agrupa por quién lo creó, la fecha y el horario, que es exactamente lo
 * que define una operación de bloqueo. Dos bloqueos distintos con los mismos
 * tres datos se juntarían en una tarjeta, pero desde afuera son
 * indistinguibles: son las mismas PCs, el mismo día y la misma franja.
 */
function claveDeAgrupacion(r: ReservaDetallada): string {
  if (r.reservaGrupoId) return r.reservaGrupoId
  if (r.tipo === "EVALUACION_ESTATAL") {
    return `evaluacion:${r.creadoPor ?? "sistema"}:${r.fecha}:${r.horaInicio}:${r.horaFin}`
  }
  return `sin-grupo:${r.id}`
}

/**
 * Agrupa las filas `reserva` en lo que cada usuario percibe como "una
 * reserva" (ver claveDeAgrupacion), conservando el orden en que vinieron
 * (el backend ordena por fecha, hora e identificador de PC).
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
      esBloqueoEvaluacion: r.tipo === "EVALUACION_ESTATAL",
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

/** RF-04.2: una PC libre en la franja consultada. */
export type PCDisponible = {
  pcId: string
  identificador: number
  carroId: string
  carroNombre: string
  freezado: boolean
  softwareInstalado?: string
}

export type CrearReservaRequest = {
  materiaId: string
  fecha: string
  horaInicio: string
  horaFin: string
  pcIds: string[]
}

export type CrearReservaRecurrenteRequest = {
  materiaId: string
  diaSemana: DiaSemana
  horaInicio: string
  horaFin: string
  fechaInicio: string
  fechaFin: string
  pcIds: string[]
}

/**
 * RF-04.7 — bloqueo por evaluación estatal.
 *
 * A diferencia de una reserva no lleva materia (el bloqueo no es de nadie)
 * y sí lleva `motivo`, que es lo que el backend intercala en el aviso a
 * cada docente afectado: "Tu reserva fue cancelada: bloqueo por evaluación
 * estatal (…)".
 */
export type BloquearEvaluacionRequest = {
  pcIds: string[]
  fecha: string
  horaInicio: string
  horaFin: string
  motivo: string
}

/**
 * Lo que hay que mostrarle al Admin después: cuántas reservas ajenas se
 * llevó puesta la cascada y a cuántos docentes se les avisó. Es la única
 * devolución que tiene de una operación destructiva.
 */
export type ResultadoBloqueoEvaluacion = {
  bloqueos: Reserva[]
  reservasCanceladas: number
  docentesNotificados: number
}
