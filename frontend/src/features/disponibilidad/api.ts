import type {
  AdminDisponibilidad,
  BloqueHorario,
  DiaSemana,
  Excepcion,
  RespuestaLista,
  TipoExcepcion,
} from "@/features/disponibilidad/types"
import { apiFetch } from "@/lib/api-client"

// ── Consulta pública dentro del sistema (RF-07.2) ─────────────────────

/**
 * Cualquier usuario autenticado, no solo Admins. `disponibleAhora` se
 * calcula en el servidor en el momento de la consulta, así que la respuesta
 * envejece: la pantalla la vuelve a pedir cada tanto.
 */
export function listarDisponibilidadDeAdmins() {
  return apiFetch<RespuestaLista<AdminDisponibilidad>>("/api/availability/admins")
}

// ── Horario semanal propio (RF-07.1, RF-07.3) ─────────────────────────

export function miHorario() {
  return apiFetch<RespuestaLista<BloqueHorario>>("/api/availability/mi-horario")
}

/**
 * RF-07.3 — el cambio rige desde ya y hacia adelante, sin ninguna acción de
 * "propagar": el horario es un patrón que se evalúa en cada consulta, no
 * una serie de semanas materializadas.
 */
export function agregarBloque(diaSemana: DiaSemana, horaInicio: string, horaFin: string) {
  return apiFetch<BloqueHorario>("/api/availability/mi-horario", {
    method: "POST",
    body: { diaSemana, horaInicio, horaFin },
  })
}

/** PATCH parcial: solo viajan los campos que se tocaron. */
export function editarBloque(
  id: string,
  cambios: { diaSemana?: DiaSemana; horaInicio?: string; horaFin?: string }
) {
  return apiFetch<BloqueHorario>(`/api/availability/mi-horario/${id}`, {
    method: "PATCH",
    body: cambios,
  })
}

/**
 * El backend acota la búsqueda por usuario_id, así que borrar el bloque de
 * otro Admin devuelve 404, no 403.
 */
export function eliminarBloque(id: string) {
  return apiFetch<void>(`/api/availability/mi-horario/${id}`, { method: "DELETE" })
}

// ── Excepciones puntuales (RF-07.4, RF-07.5) ──────────────────────────

/**
 * Pisa la excepción que ya hubiera para esa fecha (UNIQUE por usuario y
 * fecha, resuelto con upsert): cargar dos veces el mismo día no acumula.
 *
 * El backend rechaza con 400 las combinaciones incoherentes —NO_DISPONIBLE
 * con horario, o HORARIO_MODIFICADO sin él—, misma regla que el CHECK de la
 * base.
 */
export function cargarExcepcion(excepcion: {
  fecha: string
  tipo: TipoExcepcion
  horaInicio?: string
  horaFin?: string
  motivo?: string
}) {
  return apiFetch<Excepcion>("/api/availability/mi-excepcion", {
    method: "POST",
    body: excepcion,
  })
}

/**
 * RF-07.5 — atajo de un paso equivalente a cargar una excepción
 * NO_DISPONIBLE para hoy. La fecha la pone el servidor con su propia hora,
 * que es la que corresponde: la del navegador podría estar en otro día.
 */
export function marcarNoDisponibleAhora() {
  return apiFetch<Excepcion>("/api/availability/no-disponible-ahora", {
    method: "POST",
  })
}

// ── Jornada de la institución ─────────────────────────────────────────
//
// Vive en este archivo porque comparte el tipo BloqueHorario y las mismas
// conversiones de "HH:MM", pero cuelga de /api/jornada y no de
// /api/availability: es un dato de la escuela, no la disponibilidad de una
// persona.

/**
 * La jornada declarada por la institución: qué días y en qué horas abre.
 *
 * La consulta cualquier autenticado, no solo Admins — el formulario de
 * reserva la usa para avisar antes de mandar, y el calendario para saber qué
 * días dibujar.
 *
 * Lista vacía significa que todavía no la declararon, y eso NO es una
 * escuela cerrada: sin jornada no hay restricción de día ni de horario.
 */
/**
 * Clave de react-query de la jornada. Se declara acá, y no en la pantalla que
 * la edita, porque la consultan tres lugares —la pantalla del Admin, el
 * formulario de reserva y el calendario— y una clave escrita a mano en cada
 * uno se desincroniza el día que alguien la cambia en dos de los tres.
 */
export const JORNADA_KEY = ["jornada"]

export function jornadaDeLaInstitucion() {
  return apiFetch<RespuestaLista<BloqueHorario>>("/api/jornada")
}

export function agregarBloqueDeJornada(
  diaSemana: DiaSemana,
  horaInicio: string,
  horaFin: string
) {
  return apiFetch<BloqueHorario>("/api/jornada", {
    method: "POST",
    body: { diaSemana, horaInicio, horaFin },
  })
}

/** PATCH parcial, igual que el horario propio. */
export function editarBloqueDeJornada(
  id: string,
  cambios: { diaSemana?: DiaSemana; horaInicio?: string; horaFin?: string }
) {
  return apiFetch<BloqueHorario>(`/api/jornada/${id}`, {
    method: "PATCH",
    body: cambios,
  })
}

export function eliminarBloqueDeJornada(id: string) {
  return apiFetch<void>(`/api/jornada/${id}`, { method: "DELETE" })
}
