/** Lo que devuelve la API para un pedido de materia. */
export type EstadoPedido = "PENDIENTE" | "APROBADO" | "RECHAZADO"

export type PedidoDeMateria = {
  id: string
  usuarioId: string
  materiaId?: string
  cursoSolicitado?: string
  materiaSolicitada?: string
  /** La materia no existe todavía: al aprobar hay que elegir en qué curso crearla. */
  esMateriaNueva: boolean
  motivo: string
  estado: EstadoPedido
  respuesta?: string
  resueltoPor?: string
  resueltoEn?: string
  creadoEn: string
}

export type RespuestaLista<T> = { data: T[] }

/** Cómo se lee cada estado en pantalla. */
export const ETIQUETA_ESTADO_PEDIDO: Record<EstadoPedido, string> = {
  PENDIENTE: "Esperando respuesta",
  APROBADO: "Aprobado",
  RECHAZADO: "No aprobado",
}

/** Qué materia pidió, dicho en una frase. */
export function materiaDelPedido(p: PedidoDeMateria, nombreDeLaLista?: string) {
  if (!p.esMateriaNueva) return nombreDeLaLista ?? "la materia que elegiste"
  const curso = p.cursoSolicitado?.trim()
  return curso ? `${p.materiaSolicitada} de ${curso}` : (p.materiaSolicitada ?? "")
}
