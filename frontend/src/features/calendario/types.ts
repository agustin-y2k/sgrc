// Espeja calendarioEquipoResponse de
// internal/reservation/interfaces/http/dto.go (RF-04.4).

export type TipoBloque = "NORMAL" | "BLOQUEO"

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
  /** Vacío en un bloqueo administrativo, que no tiene materia. */
  materiaNombre?: string
  cursoNombre?: string
  /** Por qué se tomó el equipo. Solo en los bloqueos, y ahí va siempre. */
  motivoBloqueo?: string
}

export type CalendarioEquipo = {
  equipoId: string
  /**
   * Cómo se llama el equipo, con su carro: "PC 7 del Carro 2". Lo resuelve el
   * servidor con el mismo criterio que los avisos. Vacía si no se pudo
   * resolver, y ahí la pantalla se queda con su título genérico.
   */
  etiqueta?: string
  desde: string
  hasta: string
  bloques: BloqueCalendario[]
}
