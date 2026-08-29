/** Fechas para leer, no para parsear. */

const LARGA = new Intl.DateTimeFormat("es-AR", {
  weekday: "long",
  day: "numeric",
  month: "long",
})

/** Las horas van en 24 h en todo el sistema, y de acá salen todas. */
const HORA = new Intl.DateTimeFormat("es-AR", {
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
})

const FECHA_CORTA_Y_HORA = new Intl.DateTimeFormat("es-AR", {
  day: "2-digit",
  month: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
})

const FECHA_Y_HORA = new Intl.DateTimeFormat("es-AR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
})

export function aFechaLocal(iso: string): Date | null {
  const [anio, mes, dia] = iso.split("-").map(Number)
  if (!anio || !mes || !dia) return null
  return new Date(anio, mes - 1, dia)
}

/**
 * "martes 4 de agosto", a partir de una FECHA ("2026-08-04"): lo que en la
 * base es una columna DATE, como `reserva.fecha`.
 *
 * NO recibe un instante. Para eso está formatearFechaLargaDeInstante: cortarle
 * los diez primeros caracteres a un TIMESTAMPTZ toma su fecha en UTC, y a las
 * 23:40 de Argentina eso ya es el día siguiente.
 */
export function formatearFechaLarga(iso: string): string {
  const fecha = aFechaLocal(iso)
  return fecha ? LARGA.format(fecha) : iso
}

/** Igual, pero con la primera letra en mayúscula, para empezar una línea. */
export function formatearFechaLargaCapitalizada(iso: string): string {
  const texto = formatearFechaLarga(iso)
  return texto.charAt(0).toUpperCase() + texto.slice(1)
}

/** "Hoy", "Mañana", o la fecha larga. */
export function etiquetaDeDia(iso: string, hoy: string): string {
  if (iso === hoy) return "Hoy"
  const referencia = aFechaLocal(hoy)
  if (referencia) {
    const manana = new Date(
      referencia.getFullYear(),
      referencia.getMonth(),
      referencia.getDate() + 1
    )
    const mes = String(manana.getMonth() + 1).padStart(2, "0")
    const dia = String(manana.getDate()).padStart(2, "0")
    if (iso === `${manana.getFullYear()}-${mes}-${dia}`) return "Mañana"
  }
  return formatearFechaLargaCapitalizada(iso)
}

/**
 * Los tres formatos de un instante (`TIMESTAMPTZ` del backend), en la zona
 * del navegador y en 24 horas.
 */
function conFormato(iso: string, formato: Intl.DateTimeFormat): string {
  const fecha = new Date(iso)
  return Number.isNaN(fecha.getTime()) ? iso : formato.format(fecha)
}

/** "21:29" — solo la hora. */
export function formatearHora(iso: string): string {
  return conFormato(iso, HORA)
}

/** "18/08, 21:29" — sin año, para lo que se mira dentro de la misma semana. */
export function formatearFechaCortaYHora(iso: string): string {
  return conFormato(iso, FECHA_CORTA_Y_HORA)
}

/** "18/08/2026, 21:29" — con año, para un historial que puede ser viejo. */
export function formatearFechaYHora(iso: string): string {
  return conFormato(iso, FECHA_Y_HORA)
}

/**
 * "martes 4 de agosto" a partir de un INSTANTE (`TIMESTAMPTZ`), en la zona del
 * navegador. Es la que va cuando lo que se muestra es cuándo pasó algo y no
 * una fecha de calendario.
 */
export function formatearFechaLargaDeInstante(iso: string): string {
  return conFormato(iso, LARGA)
}
