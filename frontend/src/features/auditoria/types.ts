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

/**
 * Cómo se llama en pantalla cada cosa que el registro anota, con su género.
 *
 * Es una tabla y no un `replace("_", " ")` por dos razones que se vieron recién
 * en una captura de la guía: el nombre guardado no lleva tildes
 * (`jornada_institucion`) y el género no se puede adivinar del texto, así que
 * un «esta» fijo producía «Ver todo lo de esta usuario».
 *
 * Es un conjunto cerrado —lo que existe es lo que el backend audita— pero una
 * entidad nueva no tiene por qué esperar a esta tabla: cae en el respaldo de
 * abajo, que es neutro y siempre está bien escrito.
 */
const ENTIDADES: Record<string, { nombre: string; genero: "el" | "la" }> = {
  carro: { nombre: "carro", genero: "el" },
  ciclo_lectivo: { nombre: "ciclo lectivo", genero: "el" },
  curso: { nombre: "curso", genero: "el" },
  docente_materia: { nombre: "asignación de docente", genero: "la" },
  equipo: { nombre: "equipo", genero: "el" },
  pc: { nombre: "equipo", genero: "el" },
  equipo_cuenta: { nombre: "cuenta de equipo", genero: "la" },
  jornada_institucion: { nombre: "jornada de la escuela", genero: "la" },
  licencia: { nombre: "licencia", genero: "la" },
  materia: { nombre: "materia", genero: "la" },
  pedido_de_materia: { nombre: "pedido de materia", genero: "el" },
  reserva: { nombre: "reserva", genero: "la" },
  usuario: { nombre: "usuario", genero: "el" },
}

/** `ciclo_lectivo` → `Ciclo lectivo`. */
export function entidadLegible(entidad: string): string {
  const conocida = ENTIDADES[entidad.toLowerCase()]
  const palabras = conocida
    ? conocida.nombre
    : entidad.toLowerCase().replace(/_/g, " ")
  return palabras.charAt(0).toUpperCase() + palabras.slice(1)
}

/**
 * Cómo se la nombra dentro de una frase: «este usuario», «esta cuenta de
 * equipo». Lo que no está en la tabla se dice «esta ficha», que no es tan
 * preciso pero nunca está mal escrito.
 */
export function entidadEnFrase(entidad: string): string {
  const conocida = ENTIDADES[entidad.toLowerCase()]
  if (!conocida) return "esta ficha"
  return `${conocida.genero === "el" ? "este" : "esta"} ${conocida.nombre}`
}
