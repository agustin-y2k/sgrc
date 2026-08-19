export type TipoDeMensaje = "SUGERENCIA" | "PROBLEMA"
export type EstadoSugerencia = "ABIERTA" | "RESUELTA"

export type Sugerencia = {
  id: string
  tipo: TipoDeMensaje
  texto: string
  pantalla?: string
  version?: string
  estado: EstadoSugerencia
  /** Solo viaja en el listado del Admin. */
  usuarioId?: string
  respuesta?: string
  respondidaPor?: string
  respondidaEn?: string
  creadaEn: string
}

export type RespuestaLista<T> = { data: T[]; meta?: { total: number } }

/** Cómo se nombra cada tipo en pantalla. */
export const ETIQUETA_TIPO: Record<TipoDeMensaje, string> = {
  PROBLEMA: "Algo no anda",
  SUGERENCIA: "Una idea",
}
