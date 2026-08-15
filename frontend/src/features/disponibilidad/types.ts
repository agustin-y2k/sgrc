// Espeja los DTOs de internal/availability/interfaces/http/dto.go.

export type RespuestaLista<T> = { data: T[] }

/**
 * Se declara acá y no se importa de features/reservas, igual que el backend
 * declara su propio domain.DiaSemana en cada paquete en vez de compartirlo
 * (docs/06-arquitectura.md §3): son dos conceptos que hoy coinciden pero no
 * tienen por qué moverse juntos.
 *
 * Los siete días, igual que el enum del backend y el CHECK de la columna. Un
 * Admin de una escuela que abre el sábado tiene que poder publicar que ese
 * día está: antes el sistema no admitía nombrar el día, así que la respuesta
 * "no hay nadie" era estructural y no había forma de corregirla.
 */
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
 *
 * `disponibleAhora` no se recalcula en el cliente a propósito: la hora que
 * vale es la de la escuela (APP_TIMEZONE), no la del navegador de quien
 * mira.
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

/**
 * Ordena los bloques como transcurre la semana.
 *
 * El backend los trae con `ORDER BY dia_semana, hora_inicio`, que sobre un
 * VARCHAR ordena alfabéticamente: JUEVES, LUNES, MARTES, MIERCOLES,
 * VIERNES. Leído así, un horario semanal no se entiende. Se ordena acá y no
 * en el SQL porque es una decisión de presentación.
 */
export function ordenarPorDia(bloques: BloqueHorario[]): BloqueHorario[] {
  return [...bloques].sort((a, b) => {
    const dia = ORDEN_DIA[a.diaSemana] - ORDEN_DIA[b.diaSemana]
    return dia !== 0 ? dia : a.horaInicio.localeCompare(b.horaInicio)
  })
}
