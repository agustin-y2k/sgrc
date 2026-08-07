import type { PaginacionMeta } from "@/components/Paginador"

// Espeja internal/notification/interfaces/http/dto.go.

export type EstadoNotificacion = "NO_LEIDA" | "LEIDA"

/**
 * De qué se trata el aviso. Existe para poder ofrecer la acción que
 * corresponde sin interpretar el texto del mensaje: el mensaje está escrito
 * para una persona y cambiar su redacción no debería romper un botón.
 */
export type TipoNotificacion =
  | "GENERAL"
  | "DOCENTE_PENDIENTE"
  | "RESERVA_CANCELADA"
  | "LICENCIA_POR_VENCER"

export type Notificacion = {
  id: string
  /** Presente cuando la notificación es sobre una Reserva puntual. */
  reservaId?: string
  mensaje: string
  tipo: TipoNotificacion
  estado: EstadoNotificacion
  /** ISO 8601 */
  creadaEn: string
  leidaEn?: string
}

export type RespuestaLista<T> = { data: T[] }

/** El listado de notificaciones está paginado (ver components/Paginador). */
export type RespuestaPaginada<T> = { data: T[]; meta: PaginacionMeta }
