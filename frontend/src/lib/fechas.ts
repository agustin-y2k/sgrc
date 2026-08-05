/**
 * Fechas para leer, no para parsear.
 *
 * El backend manda y recibe siempre `YYYY-MM-DD`, y así viajan por toda la
 * aplicación. Pero mostrarlo tal cual —"2026-08-04 · 10:25"— obliga a
 * traducir mentalmente qué día de la semana es eso, que es justo el dato que
 * importa cuando alguien organiza sus clases.
 *
 * Se construye la fecha con componentes locales a propósito, igual que en
 * `esDiaLectivo` y `hoyISO`: `new Date("2026-08-04")` la interpreta como
 * medianoche UTC y al oeste de Greenwich se lee como el día anterior.
 */

const LARGA = new Intl.DateTimeFormat("es-AR", {
  weekday: "long",
  day: "numeric",
  month: "long",
})

export function aFechaLocal(iso: string): Date | null {
  const [anio, mes, dia] = iso.split("-").map(Number)
  if (!anio || !mes || !dia) return null
  return new Date(anio, mes - 1, dia)
}

/** "martes 4 de agosto". Si la fecha no se entiende, devuelve el ISO. */
export function formatearFechaLarga(iso: string): string {
  const fecha = aFechaLocal(iso)
  return fecha ? LARGA.format(fecha) : iso
}

/** Igual, pero con la primera letra en mayúscula, para empezar una línea. */
export function formatearFechaLargaCapitalizada(iso: string): string {
  const texto = formatearFechaLarga(iso)
  return texto.charAt(0).toUpperCase() + texto.slice(1)
}
