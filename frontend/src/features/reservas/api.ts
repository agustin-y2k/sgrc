import type { PaginacionMeta } from "@/components/Paginador"
import { apiFetch } from "@/lib/api-client"
import type {
  BloquearRequest,
  CrearReservaRecurrenteRequest,
  CrearReservaRequest,
  MateriaReservable,
  EquipoDisponible,
  Prestamo,
  Reserva,
  ReservaDetallada,
  ResultadoBloqueo,
  ResultadoDevolucion,
  ResultadoEntrega,
} from "@/features/reservas/types"

type RespuestaLista<T> = { data: T[] }

/** Los listados paginados agregan `meta` (ver components/Paginador). */
type RespuestaPaginada<T> = { data: T[]; meta: PaginacionMeta }

/** RF-04.1 — en qué materias puede reservar quien está autenticado. */
export function misMaterias() {
  return apiFetch<RespuestaLista<MateriaReservable>>("/api/academic/mis-materias")
}

/** RF-04.2 — los equipos libres en esa franja, de cualquier carro. */
export function equiposDisponibles(fecha: string, horaInicio: string, horaFin: string) {
  const params = new URLSearchParams({ fecha, horaInicio, horaFin })
  return apiFetch<RespuestaLista<EquipoDisponible>>(
    `/api/reservation/equipos-disponibles?${params}`
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
  /**
   * El backend pagina de a 50 si no se pide nada, con un techo de 200. Un
   * día con ocho clases de ocho máquinas son 64 reservas, así que la
   * pantalla de entregas tiene que pedir el máximo: con el default se
   * quedaba sin ver las últimas, y sin ningún aviso de que faltaban.
   */
  pageSize?: number
}) {
  const params = new URLSearchParams()
  if (filtros?.desde) params.set("desde", filtros.desde)
  if (filtros?.hasta) params.set("hasta", filtros.hasta)
  if (filtros?.incluirCanceladas) params.set("incluirCanceladas", "true")
  if (filtros?.page && filtros.page > 1) params.set("page", String(filtros.page))
  if (filtros?.pageSize) params.set("pageSize", String(filtros.pageSize))
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
 * RF-04.7 — toma equipos para otra cosa y cancela en cascada
 * las reservas que se solapen.
 *
 * Es destructivo e irreversible: las reservas canceladas no se restauran
 * si después se borra el bloqueo. El backend lo hace todo en una sola
 * transacción, así que o se bloquean todas los equipos o ninguna.
 *
 * Rechaza con 409 si alguna Equipo no está DISPONIBLE o está dada de baja.
 */
export function bloquearEquipos(req: BloquearRequest) {
  return apiFetch<ResultadoBloqueo>("/api/reservation/bloqueos", {
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

// ── Entregas y devoluciones (RF-08) ───────────────────────────────────
//
// Todo solo Admin: quien entrega y recibe las máquinas es quien hoy escribe
// el papel que esto reemplaza.

/** Qué hay afuera ahora mismo. Lo más atrasado viene primero. */
export function listarPrestamosAbiertos() {
  return apiFetch<RespuestaLista<Prestamo>>("/api/reservation/prestamos")
}

/** El historial de entregas de una máquina, de lo más reciente a lo más viejo. */
export function historialDePrestamosDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Prestamo>>(`/api/reservation/equipos/${equipoId}/prestamos`)
}

/**
 * Entregar las máquinas de una reserva. Se mandan las reservas puntuales
 * (una por Equipo), no el grupo: el docente puede llevarse tres de las cinco.
 *
 * La hora de devolución no se manda — sale del fin de la reserva.
 *
 * Responde 200 aunque alguna Equipo no haya salido; qué pasó con cada una está
 * en `noEntregadas`.
 */
export function entregarPorReserva(req: {
  reservaIds: string[]
  /**
   * Quién vino a buscarlas, si no fue el docente de la reserva. Se anota AL
   * LADO del docente y no en su lugar: él reservó y él responde (RF-08.19).
   */
  retiradoPor?: string
}) {
  return apiFetch<ResultadoEntrega>("/api/reservation/prestamos/por-reserva", {
    method: "POST",
    body: req,
  })
}

/** Entrega espontánea, sin reserva detrás: "necesito una compu para un trámite". */
export function entregarSuelta(req: {
  equipoIds: string[]
  nombre: string
  usuarioId?: string
  motivo?: string
  /** ISO 8601. Opcional: "vengo en un rato" es una respuesta válida. */
  devolucionEstimada?: string
}) {
  return apiFetch<ResultadoEntrega>("/api/reservation/prestamos", {
    method: "POST",
    body: req,
  })
}

/**
 * Las máquinas volvieron. `observaciones` vale para todo el lote, así que si
 * hay algo puntual que anotar sobre una —"volvió sin el cargador"— conviene
 * recibir esa sola.
 */
export function recibirEquipos(req: { prestamoIds: string[]; observaciones?: string }) {
  return apiFetch<ResultadoDevolucion>("/api/reservation/prestamos/recibir", {
    method: "POST",
    body: req,
  })
}

/**
 * RF-08.14 — cambiar una reserva de máquina sin partir la clase en dos.
 *
 * Sirve cuando el sistema avisa que un equipo no volvió al laboratorio: la
 * alternativa era cancelar esa reserva y crear otra, que arma un grupo nuevo
 * y deja la misma clase mostrada como dos tarjetas separadas.
 *
 * Es de quien tenga la reserva, o de un Admin.
 */
export function cambiarEquipoDeReserva(reservaId: string, equipoId: string) {
  return apiFetch<Reserva>(`/api/reservation/reservas/${reservaId}/equipo`, {
    method: "PATCH",
    body: { equipoId },
  })
}
