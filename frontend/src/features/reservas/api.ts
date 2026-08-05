import type { PaginacionMeta } from "@/components/Paginador"
import { apiFetch } from "@/lib/api-client"
import type {
  BloquearEvaluacionRequest,
  CrearReservaRecurrenteRequest,
  CrearReservaRequest,
  MateriaReservable,
  PCDisponible,
  Reserva,
  ReservaDetallada,
  ResultadoBloqueoEvaluacion,
} from "@/features/reservas/types"

type RespuestaLista<T> = { data: T[] }

/** Los listados paginados agregan `meta` (ver components/Paginador). */
type RespuestaPaginada<T> = { data: T[]; meta: PaginacionMeta }

/** RF-04.1 — en qué materias puede reservar quien está autenticado. */
export function misMaterias() {
  return apiFetch<RespuestaLista<MateriaReservable>>("/api/academic/mis-materias")
}

/** RF-04.2 — las PCs libres en esa franja, de cualquier carro. */
export function pcsDisponibles(fecha: string, horaInicio: string, horaFin: string) {
  const params = new URLSearchParams({ fecha, horaInicio, horaFin })
  return apiFetch<RespuestaLista<PCDisponible>>(
    `/api/reservation/pcs-disponibles?${params}`
  )
}

/**
 * Las reservas del usuario. El backend fuerza el filtro por creador cuando
 * quien pregunta es docente, así que "mis reservas" no necesita mandar nada.
 */
export function listarReservas(filtros?: {
  desde?: string
  hasta?: string
  incluirCanceladas?: boolean
  page?: number
}) {
  const params = new URLSearchParams()
  if (filtros?.desde) params.set("desde", filtros.desde)
  if (filtros?.hasta) params.set("hasta", filtros.hasta)
  if (filtros?.incluirCanceladas) params.set("incluirCanceladas", "true")
  if (filtros?.page && filtros.page > 1) params.set("page", String(filtros.page))
  const query = params.toString()
  return apiFetch<RespuestaPaginada<ReservaDetallada>>(
    `/api/reservation/reservas${query ? `?${query}` : ""}`
  )
}

export function crearReserva(req: CrearReservaRequest) {
  return apiFetch<{ grupo: unknown; reservas: Reserva[] }>("/api/reservation/reservas", {
    method: "POST",
    body: req,
  })
}

/** RF-04.5 — si una sola ocurrencia choca, el backend no crea ninguna. */
export function crearReservaRecurrente(req: CrearReservaRecurrenteRequest) {
  return apiFetch<{ reglaId: string; grupos: unknown[] }>(
    "/api/reservation/reservas/recurrentes",
    { method: "POST", body: req }
  )
}

/** RF-04.8 — el motivo es obligatorio si la reserva es de otra persona. */
export function cancelarReserva(reservaId: string, motivo: string) {
  return apiFetch<void>(`/api/reservation/reservas/${reservaId}/cancelar`, {
    method: "POST",
    body: { motivo },
  })
}

/**
 * RF-04.7 — bloquea PCs para una evaluación estatal y cancela en cascada
 * las reservas que se solapen.
 *
 * Es destructivo e irreversible: las reservas canceladas no se restauran
 * si después se borra el bloqueo. El backend lo hace todo en una sola
 * transacción, así que o se bloquean todas las PCs o ninguna.
 *
 * Rechaza con 409 si alguna PC no está DISPONIBLE o está dada de baja.
 */
export function bloquearParaEvaluacion(req: BloquearEvaluacionRequest) {
  return apiFetch<ResultadoBloqueoEvaluacion>("/api/reservation/bloqueos-evaluacion", {
    method: "POST",
    body: req,
  })
}

/** RF-04.6 — `soloEsta: false` cancela también las ocurrencias futuras. */
export function cancelarGrupo(grupoId: string, motivo: string, soloEsta: boolean) {
  return apiFetch<{ reservasCanceladas: number }>(
    `/api/reservation/grupos/${grupoId}/cancelar`,
    { method: "POST", body: { motivo, soloEsta } }
  )
}
