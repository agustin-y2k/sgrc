// Espeja los DTOs de internal/reporting.
//
// `Incidencia` NO está acá: es un concepto de inventario y la reportan los
// docentes, así que vive en features/inventory/types.ts. Lo que queda son
// los agregados de RF-06, que sí son solo para Admin.

/**
 * RF-06.1. Trae identificador y carro además del UUID: un reporte que solo
 * muestra IDs no se puede leer.
 */
export type ResumenUsoEquipo = {
  equipoId: string
  /** Cómo se nombra: "PC 3" o "Proyector Epson". Lo resuelve el servidor. */
  etiqueta: string
  /** 0 si no está en ningún carro. Lo que se muestra es `etiqueta`. */
  identificador: number
  carroNombre: string
  cantidadReservas: number
  minutosReservados: number
}

/** RF-06.2 */
export type ResumenUsoDocente = {
  usuarioId: string
  nombreDocente: string
  cantidadReservas: number
  minutosReservados: number
}

/**
 * RF-06.4 — el uso de un equipo en un año ya archivado.
 *
 * Todo lo que se muestra es un *snapshot*: al archivar el ciclo se borran
 * físicamente sus reservas (RF-02.4), así que estos números no se pueden
 * recalcular ni filtrar por fecha. El identificador y el carro son los que
 * el equipo tenía al cerrar el año — desde entonces pudo mudarse de carro
 * (RF-03.10) o darse de baja, y el reporte igual tiene que seguir
 * diciendo dónde estaba.
 */
export type HistoricoUsoEquipo = {
  id: string
  anio: number
  equipoId: string
  /** Cómo se llamaba el equipo el día que se archivó el ciclo. */
  etiquetaSnapshot: string
  /** 0 si no estaba en ningún carro. Lo que se muestra es `etiquetaSnapshot`. */
  identificadorSnapshot: number
  carroNombreSnapshot: string
  minutosReservados: number
  cantidadReservas: number
}

/**
 * RF-06.4 — el uso de un docente en un año ya archivado.
 *
 * `usuarioId` es opcional a propósito: la FK quedó en ON DELETE SET NULL,
 * así que si la cuenta se eliminó (RF-01.9) el snapshot sobrevive con el
 * nombre pero sin a quién apuntar. Ojo con el nombre del campo de tiempo:
 * acá es `minutosTotales`, no `minutosReservados` como en el reporte del
 * ciclo activo.
 */
export type HistoricoUsoDocente = {
  id: string
  anio: number
  usuarioId?: string
  nombreDocenteSnapshot: string
  cantidadReservas: number
  minutosTotales: number
}

/** RF-06.3 */
export type ResumenIncidenciasEquipo = {
  equipoId: string
  /** Ver ResumenUsoEquipo.etiqueta. */
  etiqueta: string
  identificador: number
  carroNombre: string
  total: number
  abiertas: number
  enReparacion: number
  enviadasDge: number
  resueltas: number
  graves: number
}

export type ResumenIncidenciasCarro = {
  carroId: string
  carroNombre: string
  total: number
  abiertas: number
  graves: number
}

export type Ciclo = {
  id: string
  anio: number
  activo: boolean
  archivado: boolean
}

/** Minutos → "2h 30min", que es como se lee un reporte de uso. */
export function formatearDuracion(minutos: number): string {
  const h = Math.floor(minutos / 60)
  const m = minutos % 60
  if (h === 0) return `${m}min`
  if (m === 0) return `${h}h`
  return `${h}h ${m}min`
}

/**
 * Qué parte del total representa un valor, de 0 a 100.
 *
 * Todo el reporte estaba en absolutos —"1240 minutos", "18 reservas"— y un
 * absoluto solo no se puede juzgar: nadie sabe si 1240 minutos es mucho sin
 * saber contra qué. El total de la propia tabla es el denominador que
 * tenemos sin pedirle nada nuevo al backend; no es la ocupación real del
 * laboratorio (eso necesita los equipos operativas y la franja lectiva, que hoy
 * no llegan a esta pantalla), así que se rotula como lo que es: la
 * participación de esa fila en el período consultado.
 */
export function proporcion(parte: number, total: number): number {
  return total === 0 ? 0 : (parte / total) * 100
}

/** 12.345 → "12,3%". Un decimal: más precisión acá es ruido. */
export function formatearPorcentaje(valor: number): string {
  return `${valor.toLocaleString("es-AR", { maximumFractionDigits: 1 })}%`
}

/**
 * Lo que devuelven las dos operaciones que sacan un equipo de circulación
 * (RF-03.8 y RF-03.9): cuántas reservas futuras se cancelaron en cascada y a
 * cuántos docentes se avisó.
 *
 * No es un detalle: dar de baja un proyector reservado para la semana que
 * viene cancela clases ajenas, y quien aprieta el botón tiene que enterarse
 * de que pasó.
 */
export type ResultadoCascada = {
  reservasCanceladas: number
  docentesNotificados: number
}
