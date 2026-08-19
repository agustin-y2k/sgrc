// Espeja internal/sugerencias/interfaces/http/dto.go.

/**
 * De qué se trata la conversación. AYUDA es el pedido de soporte y se
 * distingue de los otros dos en algo que no es cosmético: sus correos no se
 * pueden desactivar.
 */
export type TipoDeMensaje = "AYUDA" | "PROBLEMA" | "SUGERENCIA"
export type EstadoSugerencia = "ABIERTA" | "RESUELTA"

/** Una intervención del hilo. */
export type MensajeDelHilo = {
  id: string
  /** De qué lado viene: lo dice el servidor, no se deduce comparando autores. */
  deAdmin: boolean
  texto: string
  /** ISO 8601 */
  escritoEn: string
}

export type Sugerencia = {
  id: string
  tipo: TipoDeMensaje
  asunto: string
  pantalla?: string
  version?: string
  estado: EstadoSugerencia
  /** Solo viaja en el listado del Admin. */
  usuarioId?: string
  /** Está abierta y el último que habló fue quien preguntó. */
  esperaRespuesta: boolean
  mensajes: MensajeDelHilo[]
  creadaEn: string
  ultimaActividadEn: string
}

export type RespuestaLista<T> = { data: T[]; meta?: { total: number } }

/** Cómo se nombra cada tipo en pantalla. */
export const ETIQUETA_TIPO: Record<TipoDeMensaje, string> = {
  AYUDA: "Pedido de ayuda",
  PROBLEMA: "Algo no anda",
  SUGERENCIA: "Una idea",
}

/** Qué promete cada tipo, que es lo que ayuda a elegir bien. */
export const AYUDA_DEL_TIPO: Record<TipoDeMensaje, string> = {
  AYUDA:
    "Necesitás una mano ahora: el correo le llega sí o sí al equipo de administración.",
  PROBLEMA: "Algo del sistema no funciona y hay que arreglarlo.",
  SUGERENCIA: "Se te ocurre algo que el sistema podría hacer mejor.",
}
