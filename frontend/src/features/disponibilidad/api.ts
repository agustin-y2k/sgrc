import type {
  AdminDisponibilidad,
  BloqueHorario,
  DiaSemana,
  Excepcion,
  RespuestaJornada,
  RespuestaLista,
  TipoExcepcion,
  TramoDeJornada,
} from "@/features/disponibilidad/types"
import { apiFetch } from "@/lib/api-client"

// ── Consulta pública dentro del sistema (RF-07.2) ─────────────────────

/** Cualquier usuario autenticado, no solo Admins. */
export function listarDisponibilidadDeAdmins() {
  return apiFetch<RespuestaLista<AdminDisponibilidad>>("/api/availability/admins")
}

// ── Horario semanal propio (RF-07.1, RF-07.3) ─────────────────────────

export function miHorario() {
  return apiFetch<RespuestaLista<BloqueHorario>>("/api/availability/mi-horario")
}

/**
 * RF-07.3 — el cambio rige desde ya y hacia adelante, sin ninguna acción de
 * "propagar": el horario es un patrón que se evalúa en cada consulta, no una
 * serie de semanas materializadas.
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
 * RF-07.5 — atajo de un paso equivalente a cargar una excepción NO_DISPONIBLE
 * para hoy.
 */
export function marcarNoDisponibleAhora() {
  return apiFetch<Excepcion>("/api/availability/no-disponible-ahora", {
    method: "POST",
  })
}

// ── Jornada de la institución ───────────────────────────────────────── Vive
// en este archivo porque comparte el tipo BloqueHorario y las mismas
// conversiones de "HH:MM", pero cuelga de /api/jornada y no de
// /api/availability: es un dato de la escuela, no la disponibilidad de una
// persona.

/** La jornada declarada por la institución: qué días y en qué horas abre. */
/** Clave de react-query de la jornada. */
export const JORNADA_KEY = ["jornada"]

export function jornadaDeLaInstitucion() {
  return apiFetch<RespuestaJornada>("/api/jornada")
}

/**
 * Reemplaza la jornada ENTERA: lo que se manda es lo que queda.
 *
 * No hay alta, edición ni baja de un tramo suelto. La jornada es una sola
 * decisión de siete días, y mandarla de a partes dejaba a la vista un estado
 * intermedio que ya decidía qué reservas se aceptaban.
 *
 * Una lista vacía es válida y no es lo mismo que no llamar: es la institución
 * eligiendo no restringir nada, y deja de preguntársele cuál es su jornada.
 */
export function reemplazarJornada(tramos: TramoDeJornada[], confirmado = false) {
  return apiFetch<RespuestaJornada>("/api/jornada", {
    method: "PUT",
    body: { tramos, confirmado },
  })
}
