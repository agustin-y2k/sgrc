import type { PaginacionMeta } from "@/components/Paginador"

// Espeja internal/notification/interfaces/http/dto.go.

export type EstadoNotificacion = "NO_LEIDA" | "LEIDA"

/** De qué se trata el aviso. */
export type TipoNotificacion =
  | "GENERAL"
  | "DOCENTE_PENDIENTE"
  | "RESERVA_CANCELADA"
  | "LICENCIA_POR_VENCER"
  | "RESERVA_POR_COMENZAR"
  | "RESERVA_NO_RETIRADA"
  | "EQUIPO_SIN_DEVOLVER"

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
