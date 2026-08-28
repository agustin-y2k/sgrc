import type { PaginacionMeta } from "@/components/Paginador"
import { apiFetch } from "@/lib/api-client"
import type {
  BloquearRequest,
  CrearReservaRecurrenteRequest,
  CrearReservaRequest,
  MateriaReservable,
  EquipoDisponible,
  EquipoOcupado,
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

/**
 * En qué materias está ASIGNADA la persona. Para un docente es lo mismo que
 * misMaterias(); para un Admin no, porque puede reservar en todas y
 * normalmente no dicta ninguna. Es la que va donde se dice "las materias que
 * das", no donde se elige para qué materia reservar.
 */
export function misMateriasAsignadas() {
  return apiFetch<RespuestaLista<MateriaReservable>>(
    "/api/academic/mis-materias?asignadas=true"
  )
}

/**
 * RF-04.2 y RF-04.11 — las dos mitades de la franja: los equipos libres para
 * tildar y los que ya tiene alguien, con quién los tiene.
 */
export function equiposDisponibles({
  fecha,
  horaInicio,
  horaFin,
  serieDesdeGrupoId,
  materiaId,
}: {
  fecha: string
  horaInicio: string
  horaFin: string
  serieDesdeGrupoId?: string
  /** RF-03.21: para qué materia se ordena la lista. */
  materiaId?: string
}) {
  const params = new URLSearchParams({ fecha, horaInicio, horaFin })
  if (serieDesdeGrupoId) params.set("serieDesdeGrupoId", serieDesdeGrupoId)
  if (materiaId) params.set("materiaId", materiaId)
  return apiFetch<RespuestaLista<EquipoDisponible> & { ocupados?: EquipoOcupado[] }>(
    `/api/reservation/equipos-disponibles?${params}`
  )
}

/**
 * RF-04.12 — pedirle al docente que tiene esa reserva que libere el equipo.
 */
export function pedirLiberacion(reservaId: string, mensaje: string) {
  return apiFetch<void>(`/api/reservation/reservas/${reservaId}/pedido-de-liberacion`, {
    method: "POST",
    body: { mensaje },
  })
}

/** Las reservas del usuario. */
export function listarReservas(filtros?: {
  desde?: string
  hasta?: string
  incluirCanceladas?: boolean
  page?: number
  /**
   * El backend pagina de a 50 si no se pide nada, con un techo de 200. Un día
   * con ocho clases de ocho máquinas son 64 reservas, así que la pantalla de
   * entregas tiene que pedir el máximo: con el default se quedaba sin ver las
   * últimas, y sin ningún aviso de que faltaban.
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
 * RF-04.7 — toma equipos para otra cosa y cancela en cascada las reservas que
 * se solapen.
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

// ── Entregas y devoluciones (RF-08) ─────────────────────────────────── Todo
// solo Admin: quien entrega y recibe las máquinas es quien hoy escribe el
// papel que esto reemplaza.

/** Qué hay afuera ahora mismo. Lo más atrasado viene primero. */
export function listarPrestamosAbiertos() {
  return apiFetch<RespuestaLista<Prestamo>>("/api/reservation/prestamos")
}

/** El historial de entregas de una máquina, de lo más reciente a lo más viejo. */
export function historialDePrestamosDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Prestamo>>(
    `/api/reservation/equipos/${equipoId}/prestamos`
  )
}

/** Entregar las máquinas de una reserva. */
export function entregarPorReserva(req: {
  reservaIds: string[]
  /** Quién vino a buscarlas, si no fue el docente de la reserva. */
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
  /**
   * El equipo NO está disponible y sale igual, camino al técnico. Es el único
   * modo de sacar del laboratorio algo en mantenimiento o fuera de servicio,
   * y obliga a mandar `motivo`.
   */
  salidaAReparacion?: boolean
}) {
  return apiFetch<ResultadoEntrega>("/api/reservation/prestamos", {
    method: "POST",
    body: req,
  })
}

/** Las máquinas volvieron. */
export function recibirEquipos(req: { prestamoIds: string[]; observaciones?: string }) {
  return apiFetch<ResultadoDevolucion>("/api/reservation/prestamos/recibir", {
    method: "POST",
    body: req,
  })
}

/** RF-08.14 — cambiar una reserva de máquina sin partir la clase en dos. */
export function cambiarEquipoDeReserva(
  reservaId: string,
  equipoId: string,
  soloEsta = true
) {
  return apiFetch<Reserva>(`/api/reservation/reservas/${reservaId}/equipo`, {
    method: "PATCH",
    body: { equipoId, soloEsta },
  })
}
