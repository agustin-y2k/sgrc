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

// ── Preferencias de correo (RF-05.13) ───────────────────────────────────

/**
 * De qué avisa cada correo. Todo esto se ve igual en la campana: acá se
 * elige el canal, no la información.
 */
export type CategoriaEmail =
  // De la cuenta: salen siempre, no se pueden apagar.
  | "RECUPERACION_DE_CUENTA"
  | "CUENTA_APROBADA"
  // Personales: las tiene cualquiera por sus reservas y pedidos.
  | "RESERVA_CANCELADA"
  | "EQUIPO_NO_DISPONIBLE"
  | "PEDIDO_DE_LIBERACION"
  | "PEDIDO_DE_MATERIA_RESUELTO"
  | "PEDIDO_SOBRE_MI_MATERIA"
  | "SUGERENCIA_RESPONDIDA"
  | "RECORDATORIO_DE_RESERVA"
  | "RESERVA_SIN_RETIRAR"
  | "DEVOLUCION_PENDIENTE"
  // De administración: los avisos que van a todos los Admin.
  | "CUENTA_PENDIENTE"
  | "DEVOLUCION_DEMORADA"
  | "CIERRE_SIN_DEVOLVER"
  | "LICENCIA_POR_VENCER"
  | "PEDIDO_DE_MATERIA"
  | "SUGERENCIA"

export type GrupoDeCategoria = "CUENTA" | "PERSONAL" | "ADMINISTRACION"

/**
 * La etiqueta, la descripción y el grupo vienen del backend: la lista de
 * categorías la define quien manda los correos, no esta pantalla.
 */
export type PreferenciaEmail = {
  categoria: CategoriaEmail
  grupo: GrupoDeCategoria
  etiqueta: string
  descripcion: string
  activa: boolean
  /** Sale siempre y no admite preferencia: se muestra sin casilla que tocar. */
  fija: boolean
}
