// Espeja los DTOs de internal/auditoria/interfaces/http/dto.go.

import type { PaginacionMeta } from "@/components/Paginador"

/** Una acción registrada: quién hizo qué, sobre qué cosa y cuándo. */
export type EntradaDeAuditoria = {
  id: string
  /**
   * Viaja SIEMPRE, también cuando hay nombre: es lo único que sigue
   * identificando a quien hizo la acción si esa cuenta después se elimina.
   */
  actorId: string
  /**
   * Falta cuando la cuenta ya no existe (RF-01.9). El backend NO lo rellena con
   * un texto de relleno a propósito: decidir cómo se dice «una cuenta
   * eliminada» es de la pantalla, y un "Desconocido" puesto del otro lado se
   * confundiría con el nombre de alguien.
   */
  actorNombre?: string
  accion: string
  entidad: string
  entidadId?: string
  /** Lo que guardó cada acción, con su propia forma. No se interpreta. */
  detalle?: unknown
  ipOrigen?: string
  creadoEn: string
}

export type RespuestaAuditoria = {
  data: EntradaDeAuditoria[]
  meta: PaginacionMeta
}

/** Los valores que HOY existen en el registro, para armar los selectores. */
export type OpcionesDeAuditoria = {
  acciones: string[]
  entidades: string[]
}

export type FiltroDeAuditoria = {
  accion?: string
  entidad?: string
  entidadId?: string
  usuarioId?: string
  desde?: string
  hasta?: string
}

/**
 * `CUENTA_APROBADA` → `Cuenta aprobada`.
 *
 * Se traduce en la PANTALLA y no en el backend, y eso es deliberado: lo
 * guardado es el nombre que la operación tenía cuando ocurrió, y el registro no
 * se reescribe nunca. Si mañana una acción se renombra, las filas viejas
 * conservan su nombre viejo y esta función simplemente no lo va a embellecer —
 * que es preferible a que el registro mienta sobre qué se hizo.
 */
export function accionLegible(accion: string): string {
  const palabras = accion.toLowerCase().replace(/_/g, " ")
  return palabras.charAt(0).toUpperCase() + palabras.slice(1)
}

/** `ciclo_lectivo` → `Ciclo lectivo`. Mismo criterio. */
export function entidadLegible(entidad: string): string {
  const palabras = entidad.toLowerCase().replace(/_/g, " ")
  return palabras.charAt(0).toUpperCase() + palabras.slice(1)
}
