/** Lo que devuelve la API para un pedido de materia. */
export type EstadoPedido = "PENDIENTE" | "APROBADO" | "RECHAZADO"

export type PedidoDeMateria = {
  id: string
  usuarioId: string
  materiaId?: string
  cursoSolicitado?: string
  materiaSolicitada?: string
  /**
   * Cómo se llama la materia que ya existe, y en qué curso. Los resuelve el
   * servidor: el pedido guarda solo `materiaId`, así que sin esto no había
   * forma de nombrar lo que se pidió.
   */
  materiaNombre?: string
  cursoNombre?: string
  /**
   * Quién lo pidió. Solo lo dibuja la bandeja del Admin: en el perfil de quien
   * pidió sobra, y el pedido guarda un UUID que no le dice nada a nadie.
   */
  docenteNombre?: string
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

/**
 * Qué materia pidió, dicho en una frase: "Programación de 5°A".
 *
 * Sale igual para las dos formas de pedir —una materia de la lista o una que
 * todavía no existe— porque quien lee la frase necesita lo mismo en los dos
 * casos. El respaldo genérico quedó solo para un pedido viejo cuya materia se
 * eliminó después: ahí el JOIN no trae nombre y no hay nada que decir.
 */
export function materiaDelPedido(
  p: PedidoDeMateria,
  siNoSeSabe = "Una materia que ya no existe"
) {
  const nombre = p.esMateriaNueva ? p.materiaSolicitada : p.materiaNombre
  const curso = (p.esMateriaNueva ? p.cursoSolicitado : p.cursoNombre)?.trim()
  if (!nombre?.trim()) return siNoSeSabe
  return curso ? `${nombre} de ${curso}` : nombre
}
