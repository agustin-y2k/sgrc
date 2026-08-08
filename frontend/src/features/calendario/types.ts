// Espeja calendarioEquipoResponse de
// internal/reservation/interfaces/http/dto.go (RF-04.4).

export type TipoBloque = "NORMAL" | "EVALUACION_ESTATAL"

export type BloqueCalendario = {
  reservaId: string
  /** YYYY-MM-DD */
  fecha: string
  /** HH:MM */
  horaInicio: string
  /** HH:MM */
  horaFin: string
  estado: string
  tipo: TipoBloque
  docente: string
  /** Vacío en un bloqueo por evaluación estatal, que no tiene materia. */
  materiaNombre?: string
  cursoNombre?: string
}

export type CalendarioEquipo = {
  equipoId: string
  desde: string
  hasta: string
  bloques: BloqueCalendario[]
}
