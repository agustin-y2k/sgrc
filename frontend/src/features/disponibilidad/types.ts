// Espeja los DTOs de internal/availability/interfaces/http/dto.go.

import { ApiError } from "@/lib/api-client"

export type RespuestaLista<T> = { data: T[] }

/**
 * Un tramo de la jornada tal como se manda: sin id, porque la jornada se
 * reemplaza entera y los ids los pone el backend.
 */
export type TramoDeJornada = {
  diaSemana: DiaSemana
  horaInicio: string
  horaFin: string
}

/**
 * La jornada de la institución con su bandera al lado.
 *
 * `definida` es lo que separa "todavía no la declararon" de "eligieron
 * dejarla libre": las dos llegan con `data` vacío y piden cosas distintas —a
 * la primera hay que preguntarle cuál es su jornada, a la segunda no.
 */
export type RespuestaJornada = RespuestaLista<BloqueHorario> & {
  definida: boolean
  /** Cuántas reservas se cancelaron al guardar, si hubo alguna. */
  reservasCanceladas?: number
}

/** Una reserva que quedaría fuera de la jornada propuesta. */
export type ReservaAfectada = {
  id: string
  fecha: string
  horaInicio: string
  horaFin: string
  equipo: string
  materia: string
  docente: string
}

/**
 * Una máquina YA ENTREGADA contra una de las reservas que se van a cancelar.
 *
 * La jornada no restringe los préstamos —una máquina se entrega cualquier día
 * y a cualquier hora mientras esté en el laboratorio—, así que lo que importa
 * no es su horario sino que su clase deje de existir mientras el docente la
 * tiene en la mano.
 */
export type PrestamoAfectado = {
  id: string
  equipo: string
  quien: string
}

/** Lo que un cambio de jornada dejaría afuera. */
export type ImpactoDeJornada = {
  /**
   * Viene RECORTADA por el backend. Lo que se va a cancelar es
   * `totalAfectadas`, no `reservas.length`: mostrar el largo de la lista haría
   * que el Admin confirme creyendo que cancela muchas menos.
   */
  reservas: ReservaAfectada[]
  prestamos: PrestamoAfectado[]
  /** Cuántas se van a cancelar de verdad. */
  totalAfectadas: number
  /** Cuántas reservas futuras hay en total, afectadas o no. */
  totalDeReservas: number
}

/**
 * El backend rechaza con 409 un cambio que deja algo afuera, y manda el
 * detalle en el mismo cuerpo. Esta función lo saca del ApiError.
 *
 * Devuelve null si el error es cualquier otra cosa: un 409 por solape de
 * tramos, por ejemplo, no trae impacto y se muestra como un error normal.
 */
export function impactoDelError(err: unknown): ImpactoDeJornada | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null
  const cuerpo = err.cuerpo
  if (typeof cuerpo !== "object" || cuerpo === null || !("impacto" in cuerpo)) {
    return null
  }
  return (cuerpo as { impacto: ImpactoDeJornada }).impacto
}

/**
 * Se declara acá y no se importa de features/reservas, igual que el backend
 * declara su propio domain.DiaSemana en cada paquete en vez de compartirlo
 * (docs/06-arquitectura.md §3): son dos conceptos que hoy coinciden pero no
 * tienen por qué moverse juntos.
 */
export type DiaSemana =
  "LUNES" | "MARTES" | "MIERCOLES" | "JUEVES" | "VIERNES" | "SABADO" | "DOMINGO"

export const DIAS_SEMANA: { valor: DiaSemana; etiqueta: string }[] = [
  { valor: "LUNES", etiqueta: "Lunes" },
  { valor: "MARTES", etiqueta: "Martes" },
  { valor: "MIERCOLES", etiqueta: "Miércoles" },
  { valor: "JUEVES", etiqueta: "Jueves" },
  { valor: "VIERNES", etiqueta: "Viernes" },
  { valor: "SABADO", etiqueta: "Sábado" },
  { valor: "DOMINGO", etiqueta: "Domingo" },
]

const ETIQUETAS_DIA: Record<DiaSemana, string> = Object.fromEntries(
  DIAS_SEMANA.map((d) => [d.valor, d.etiqueta])
) as Record<DiaSemana, string>

export function etiquetaDia(dia: DiaSemana): string {
  return ETIQUETAS_DIA[dia]
}

export type BloqueHorario = {
  id: string
  diaSemana: DiaSemana
  horaInicio: string
  horaFin: string
}

/** RF-07.4/07.5 — ausencia total ese día, u horario distinto al habitual. */
export type TipoExcepcion = "NO_DISPONIBLE" | "HORARIO_MODIFICADO"

export type Excepcion = {
  id: string
  fecha: string
  tipo: TipoExcepcion
  /** Ausentes si el tipo es NO_DISPONIBLE; obligatorias si es HORARIO_MODIFICADO. */
  horaInicio?: string
  horaFin?: string
  motivo?: string
}

/**
 * RF-07.2 — un Admin con su estado de "ahora" ya calculado por el backend
 * contra la hora del servidor, más el horario semanal de referencia.
 */
export type AdminDisponibilidad = {
  usuarioId: string
  nombre: string
  apellido: string
  disponibleAhora: boolean
  excepcionHoy?: Excepcion
  horarioSemanal: BloqueHorario[]
}

const ORDEN_DIA: Record<DiaSemana, number> = {
  LUNES: 1,
  MARTES: 2,
  MIERCOLES: 3,
  JUEVES: 4,
  VIERNES: 5,
  SABADO: 6,
  DOMINGO: 7,
}

/** Ordena los bloques como transcurre la semana. */
export function ordenarPorDia(bloques: BloqueHorario[]): BloqueHorario[] {
  return [...bloques].sort((a, b) => {
    const dia = ORDEN_DIA[a.diaSemana] - ORDEN_DIA[b.diaSemana]
    return dia !== 0 ? dia : a.horaInicio.localeCompare(b.horaInicio)
  })
}

// ── Jornada de la institución ─────────────────────────────────────────
// Espejo de domain.PermiteReserva del backend, para poder avisar en la
// pantalla en vez de esperar el 400. La regla vive en el backend, que es
// quien decide; esto solo se adelanta.

/** Índice de JS (0 = domingo) a nuestro enum. */
const DIA_DE_JS: DiaSemana[] = [
  "DOMINGO",
  "LUNES",
  "MARTES",
  "MIERCOLES",
  "JUEVES",
  "VIERNES",
  "SABADO",
]

/** Día de la semana de una fecha "YYYY-MM-DD". */
export function diaDeLaFecha(fechaISO: string): DiaSemana | null {
  const [anio, mes, dia] = fechaISO.split("-").map(Number)
  if (!anio || !mes || !dia) return null
  return DIA_DE_JS[new Date(anio, mes - 1, dia).getDay()]
}

/** Los días en que la institución declaró que abre, en orden de semana. */
export function diasDeLaJornada(jornada: BloqueHorario[]): DiaSemana[] {
  if (jornada.length === 0) return DIAS_SEMANA.map((d) => d.valor)
  const declarados = new Set(jornada.map((b) => b.diaSemana))
  return DIAS_SEMANA.map((d) => d.valor).filter((d) => declarados.has(d))
}

/** Si un bloque cae dentro de la jornada. */
export function dentroDeLaJornada(
  jornada: BloqueHorario[],
  fechaISO: string,
  horaInicio: string,
  horaFin: string
): boolean {
  if (jornada.length === 0) return true

  const dia = diaDeLaFecha(fechaISO)
  if (dia === null) return true // fecha incompleta: que valide el backend

  // Todo se mide desde la misma medianoche, así que el fin pasa de las 24
  // horas cuando el tramo cruza: una jornada nocturna de 20:00 a 01:00 es
  // [1200, 1500) en minutos.
  const tramos = jornada
    .filter((b) => b.diaSemana === dia)
    .map((b) => {
      const desde = aMinutosDelDia(b.horaInicio)
      const hasta = aMinutosDelDia(b.horaFin)
      return { desde, hasta: hasta < desde ? hasta + 24 * 60 : hasta }
    })
    .sort((a, b) => a.desde - b.desde)
  if (tramos.length === 0) return false

  const fusionados = [tramos[0]]
  for (const t of tramos.slice(1)) {
    const ultimo = fusionados[fusionados.length - 1]
    if (t.desde <= ultimo.hasta) {
      ultimo.hasta = Math.max(ultimo.hasta, t.hasta)
      continue
    }
    fusionados.push({ ...t })
  }

  const desde = aMinutosDelDia(horaInicio)
  const crudoHasta = aMinutosDelDia(horaFin)
  const hasta = crudoHasta < desde ? crudoHasta + 24 * 60 : crudoHasta
  return fusionados.some((t) => desde >= t.desde && hasta <= t.hasta)
}

function aMinutosDelDia(hora: string): number {
  const [h, m] = hora.split(":").map(Number)
  return h * 60 + m
}
