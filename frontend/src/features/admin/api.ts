import { apiFetch } from "@/lib/api-client"
import type {
  CrearAdminRequest,
  Estado,
  ListarUsuariosResponse,
  Rol,
} from "@/features/auth/types"
import type {
  AltaDePreferencias,
  AltaMasivaLicencias,
  Carro,
  Incidencia,
  Licencia,
  Equipo,
  PreferenciaDeEquipo,
  PreferenciaHuerfana,
  RenovacionLicencias,
  RespuestaLista,
  VencimientoDeclarado,
} from "@/features/inventory/types"
import type {
  EquipoFueraDeCirculacion,
  EstadoDelInventario,
  HistoricoUsoDocente,
  HistoricoUsoEquipo,
  ResultadoCascada,
  ResumenPorCategoriaDeFalla,
  ResumenIncidenciasCarro,
  ResumenIncidenciasEquipo,
  ResumenUsoDocente,
  ResumenUsoEquipo,
} from "@/features/admin/types"

// ── Usuarios (RF-01.x / RF-02.x) ──────────────────────────────────────

export function listarUsuarios(filtros?: {
  estado?: Estado
  rol?: Rol
  page?: number
  /** Hasta 200. Para listas que se recorren enteras, no de a páginas. */
  pageSize?: number
}) {
  const params = new URLSearchParams()
  if (filtros?.estado) params.set("estado", filtros.estado)
  if (filtros?.rol) params.set("rol", filtros.rol)
  if (filtros?.page && filtros.page > 1) params.set("page", String(filtros.page))
  if (filtros?.pageSize) params.set("pageSize", String(filtros.pageSize))
  const query = params.toString()
  return apiFetch<ListarUsuariosResponse>(`/api/auth/usuarios${query ? `?${query}` : ""}`)
}

export function cambiarEstadoUsuario(id: string, estado: Estado) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/estado`, {
    method: "PATCH",
    body: { estado },
  })
}

/** RF-01.6 — devuelve la contraseña temporal para comunicársela a mano. */
export function resetearPassword(id: string) {
  return apiFetch<{ passwordTemporal: string }>(
    `/api/auth/usuarios/${id}/reset-password`,
    { method: "POST" }
  )
}

/**
 * Lo que el borrado definitivo se llevó en cascada. Sólo lo que DESAPARECE y
 * el Admin no espera: el resto (reservas, préstamos, incidencias) sobrevive
 * perdiendo la referencia al usuario, así que no se cuenta.
 */
export interface ArrastreDeEliminacion {
  hilosDeSoporte: number
  mensajesDeSoporte: number
  /** El que más consecuencias tiene: sin tramos de guardia, el barrido cambia. */
  bloquesDeGuardia: number
  notificaciones: number
  pedidosDeMateria: number
}

/** RF-01.9 — hard delete, permitido desde BAJA o RECHAZADA. Libera el email. */
export function eliminarUsuario(id: string) {
  return apiFetch<ArrastreDeEliminacion>(`/api/auth/usuarios/${id}`, { method: "DELETE" })
}

/** Le da rol ADMIN a un docente ya aprobado. */
export function promoverAAdmin(id: string) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/promover-a-admin`, { method: "POST" })
}

/**
 * La inversa: le quita el rol ADMIN a un Admin y lo deja como docente, sin
 * cerrarle la cuenta.
 */
export function degradarADocente(id: string) {
  return apiFetch<void>(`/api/auth/usuarios/${id}/degradar-a-docente`, { method: "POST" })
}

/** RF-01.4 — un Admin puede crear otros Admin (quedan auto-aprobados). */
/** RF-01.4 — el Admin nuevo queda APROBADA, sin pasar por PENDIENTE. */
export function crearAdmin(req: CrearAdminRequest) {
  return apiFetch<void>("/api/auth/admins", { method: "POST", body: req })
}

// ── Inventario (RF-03.x) ──────────────────────────────────────────────

export function crearCarro(req: { nombre: string; descripcion?: string }) {
  return apiFetch<Carro>("/api/carros", { method: "POST", body: req })
}

export function editarCarro(id: string, req: { nombre?: string; descripcion?: string }) {
  return apiFetch<void>(`/api/carros/${id}`, { method: "PATCH", body: req })
}

export function crearEquipoDeCarro(
  carroId: string,
  req: {
    identificador: number
    numeroSerie: string
    freezado: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
  }
) {
  return apiFetch<Equipo>(`/api/carros/${carroId}/equipos`, {
    method: "POST",
    body: req,
  })
}

/**
 * RF-03.15 — dar de alta algo prestable que no es una computadora de un
 * carro.
 */
export function crearEquipoSuelto(req: {
  tipo: string
  nombre: string
  /** Opcional para cualquier tipo: un proyector tiene serie, un cargador no. */
  numeroSerie?: string
  reservable: boolean
  /** Habilita los cinco de abajo, que solo tienen sentido en una computadora. */
  esComputadora?: boolean
  freezado?: boolean
  cpu?: string
  ram?: string
  sistemaOperativo?: string
  softwareInstalado?: string
}) {
  // A qué colección se hace POST decide dónde nace el equipo: acá nace
  // suelto, en /carros/{id}/equipos nace adentro de ese carro.
  return apiFetch<Equipo>("/api/equipos", { method: "POST", body: req })
}

export function editarEquipo(
  id: string,
  req: {
    carroId?: string
    freezado?: boolean
    cpu?: string
    ram?: string
    sistemaOperativo?: string
    softwareInstalado?: string
    tipo?: string
    nombre?: string
    reservable?: boolean
    esComputadora?: boolean
    /** Cadena vacía borra la serie; el backend solo lo acepta fuera de un carro. */
    numeroSerie?: string
  }
) {
  return apiFetch<void>(`/api/equipos/${id}`, { method: "PATCH", body: req })
}

/**
 * RF-03.8 — pasar un equipo a EN_MANTENIMIENTO o FUERA_DE_SERVICIO cancela en
 * cascada sus reservas futuras.
 */
export function cambiarEstadoEquipo(
  id: string,
  estado: Equipo["estado"],
  motivo?: string
) {
  return apiFetch<ResultadoCascada>(`/api/equipos/${id}/estado`, {
    method: "PATCH",
    body: { estado, motivo },
  })
}

/** RF-03.9 — dar de baja dispara la misma cascada que RF-03.8. */
export function darDeBajaEquipo(id: string) {
  return apiFetch<ResultadoCascada>(`/api/equipos/${id}`, { method: "DELETE" })
}

/**
 * Deshacer una baja. No devuelve cascada porque no hay ninguna que deshacer:
 * las reservas que la baja canceló quedan canceladas y los avisos ya salieron.
 *
 * Puede fallar con 409 si mientras el equipo estuvo afuera otro se quedó con su
 * lugar en el carro, con su nombre o con su número de serie — los tres se
 * liberan al dar de baja. El mensaje del servidor dice cuál de los tres es.
 */
export function reactivarEquipo(id: string) {
  return apiFetch<void>(`/api/equipos/${id}/reactivar`, { method: "POST" })
}

/**
 * Retira un carro de circulación. Solo se puede si no le queda ningún equipo
 * activo adentro: el backend rechaza con 409 y el mensaje dice qué hacer
 * —moverlos a otro carro o darlos de baja— porque la salida no es obvia.
 *
 * El nombre se libera: el índice único excluye a los retirados, así que otro
 * carro puede pasar a llamarse «Carro 1» mientras éste está afuera.
 */
export function darDeBajaCarro(id: string) {
  return apiFetch<void>(`/api/carros/${id}`, { method: "DELETE" })
}

export function reactivarCarro(id: string) {
  return apiFetch<void>(`/api/carros/${id}/reactivar`, { method: "POST" })
}

// Listar y reportar incidencias viven en features/inventory/api.ts: las
// puede hacer cualquier usuario autenticado. Editarlas es solo de Admin.
export function editarIncidencia(
  id: string,
  req: { estado?: Incidencia["estado"]; marcarEnviadaASoporte?: boolean }
) {
  return apiFetch<void>(`/api/incidencias/${id}`, {
    method: "PATCH",
    body: {
      estado: req.estado,
      marcarEnviadaASoporte: req.marcarEnviadaASoporte ?? false,
    },
  })
}

// ── Reportes (RF-06.x) ────────────────────────────────────────────────

function conRango(base: string, desde?: string, hasta?: string) {
  const params = new URLSearchParams()
  if (desde) params.set("desde", desde)
  if (hasta) params.set("hasta", hasta)
  const query = params.toString()
  return query ? `${base}?${query}` : base
}

/** RF-06.1 — uso por equipo del ciclo, filtrable por rango de fechas. */
export function reporteUsoEquipos(cicloId: string, desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenUsoEquipo>>(
    conRango(`/api/ciclos/${cicloId}/uso-equipos`, desde, hasta)
  )
}

/** RF-06.2 */
export function reporteUsoDocentes(cicloId: string, desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenUsoDocente>>(
    conRango(`/api/ciclos/${cicloId}/uso-docentes`, desde, hasta)
  )
}

/**
 * RF-06.4 — el snapshot anual, que es lo único que queda de un ciclo
 * archivado: sus reservas se borran físicamente al archivarlo (RF-02.4).
 */
export function historicoUsoEquipos(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoEquipo>>(
    `/api/historico/${anio}/uso-equipos`
  )
}

export function historicoUsoDocentes(anio: number) {
  return apiFetch<RespuestaLista<HistoricoUsoDocente>>(
    `/api/historico/${anio}/uso-docentes`
  )
}

/** RF-06.3 — no depende del ciclo: Incidencia sobrevive al archivado. */
export function reporteIncidenciasPorEquipo(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenIncidenciasEquipo>>(
    conRango("/api/incidencias/equipos", desde, hasta)
  )
}

export function reporteIncidenciasPorCarro(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenIncidenciasCarro>>(
    conRango("/api/incidencias/carros", desde, hasta)
  )
}

/**
 * RF-06.5 — los dos primeros describen la situación de AHORA y por eso no
 * aceptan rango de fechas: "cuántas estaban rotas en marzo" no se puede
 * responder con el estado actual.
 */
export function reporteEstadoDelInventario() {
  return apiFetch<RespuestaLista<EstadoDelInventario>>("/api/inventario/estado")
}

export function reporteEquiposFueraDeCirculacion() {
  return apiFetch<RespuestaLista<EquipoFueraDeCirculacion>>(
    "/api/inventario/fuera-de-circulacion"
  )
}

/** Este sí acepta fechas: la pregunta es qué se rompió en un período. */
export function reporteIncidenciasPorCategoria(desde?: string, hasta?: string) {
  return apiFetch<RespuestaLista<ResumenPorCategoriaDeFalla>>(
    conRango("/api/incidencias/categorias", desde, hasta)
  )
}

export function listarCiclos() {
  return apiFetch<
    RespuestaLista<{ id: string; anio: number; activo: boolean; archivado: boolean }>
  >("/api/ciclos")
}

// ── Licencias de software (RF-03.11 a RF-03.14) ─────────────────────── Todo
// solo-Admin, incluidas las lecturas: el docente elige Equipo por
// `softwareInstalado`, que ya ve en la pantalla de reserva; cuándo vence una
// licencia es trabajo administrativo.

export function listarLicencias() {
  return apiFetch<RespuestaLista<Licencia>>("/api/licencias")
}

export function listarLicenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Licencia>>(
    `/api/equipos/${equipoId}/licencias`
  )
}

/**
 * Alta de la MISMA licencia en varios equipos de una vez: el caso real es
 * "AutoCAD, 30 días, en estas ocho máquinas".
 */
export function crearLicencias(
  req: {
    equipoIds: string[]
    nombre: string
    diasDuracion: number
    diasAviso?: number
  } & VencimientoDeclarado
) {
  return apiFetch<AltaMasivaLicencias>("/api/licencias", {
    method: "POST",
    body: req,
  })
}

/**
 * `renovadaEl` ausente significa "hoy", que es el botón que se aprieta el 99%
 * de las veces.
 */
export function renovarLicencias(req: { licenciaIds: string[]; renovadaEl?: string }) {
  return apiFetch<RenovacionLicencias>("/api/licencias/renovar", {
    method: "POST",
    body: req,
  })
}

/**
 * El "editar el contador en cualquier momento": corregir la fecha, cambiar la
 * duración de 30 a 60 días, o cargar el vencimiento de una licencia que se
 * dio de alta sin él.
 */
export function editarLicencia(
  id: string,
  req: {
    nombre?: string
    diasDuracion?: number
    diasAviso?: number
  } & VencimientoDeclarado
) {
  return apiFetch<void>(`/api/licencias/${id}`, { method: "PATCH", body: req })
}

export function borrarLicencia(id: string) {
  return apiFetch<void>(`/api/licencias/${id}`, { method: "DELETE" })
}

// ── Preferencia de materia por equipo (RF-03.21) ─────────────────────── La
// marca dice que una máquina es preferente para una materia.

export function listarPreferenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<PreferenciaDeEquipo>>(
    `/api/equipos/${equipoId}/preferencias`
  )
}

/**
 * Los nombres de materia que ya existen, para que el Admin ELIJA en vez de
 * tipear: la marca se guarda como texto, y este selector es lo único que
 * impide que "Matemática" y "Matematica" nazcan como dos marcas distintas.
 */
export function materiasEnUso() {
  return apiFetch<RespuestaLista<string>>("/api/materias-en-uso")
}

/**
 * La misma marca en varios equipos de una vez: el caso real es "estas ocho
 * PCs son las de Dibujo Técnico".
 */
export function marcarPreferencia(req: {
  equipoIds: string[]
  materiaNombre: string
  modalidad?: string
  anio?: number
  division?: string
  prioridad?: number
}) {
  return apiFetch<AltaDePreferencias>("/api/preferencias", {
    method: "POST",
    body: req,
  })
}

/** La materia no se edita: apuntar a otra es otra marca. */
export function editarPreferencia(
  id: string,
  req: { modalidad?: string; anio?: number; division?: string; prioridad?: number }
) {
  return apiFetch<PreferenciaDeEquipo>(`/api/preferencias/${id}`, {
    method: "PATCH",
    body: req,
  })
}

export function borrarPreferencia(id: string) {
  return apiFetch<void>(`/api/preferencias/${id}`, { method: "DELETE" })
}

/**
 * Las marcas que quedaron apuntando a una materia inexistente. Es una consulta
 * de todo el inventario y no de un equipo: la huérfana la produce renombrar una
 * materia, que no pasa por ninguna máquina en particular.
 *
 * Va antes que `/preferencias/{id}` en el router del backend, para que
 * "huerfanas" no se lea como un id.
 */
export function listarPreferenciasHuerfanas() {
  return apiFetch<RespuestaLista<PreferenciaHuerfana>>("/api/preferencias/huerfanas")
}
