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

/**
 * Las horas van en 24 h en todo el sistema, y de acá salen todas.
 *
 * `hour12: false` no es opcional ni cosmético: el `es-AR` de un navegador
 * resuelve por defecto a 12 horas, así que sin esto la misma aplicación que
 * aclara "los horarios van en formato de 24 horas" en el formulario de
 * reserva, y que muestra "07:30" en la jornada, escribía "09:29 p. m." en
 * Entregas y en Avisos. Además de inconsistente, el "p. m." con espacios que
 * produce ese locale es incómodo de leer de un vistazo en un mostrador.
 *
 * La trampa conocida es que `hour12: false` diera el ciclo h24 —01 a 24, con
 * la medianoche escrita "24:00"— en vez de h23. Verificado que no pasa: tanto
 * el Chromium con el que se corren los E2E como el Node del build resuelven
 * `hourCycle: "h23"`, y la medianoche sale "00:00".
 */
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

/**
 * "Hoy", "Mañana", o la fecha larga.
 *
 * Los dos primeros días son los que se miran, y leer "Hoy" es más rápido y
 * más seguro que leer "miércoles 12 de agosto" y compararlo mentalmente con
 * el día que uno cree que es. Del tercero en adelante la fecha sí es el
 * dato: "en tres días" obligaría a contar.
 *
 * `hoy` se pasa y no se lee del reloj acá para que la pantalla entera hable
 * del mismo día: consultada dos veces alrededor de la medianoche, una misma
 * lista podría rotular "Hoy" y "Mañana" la misma fecha.
 */
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
 *
 * Reciben el ISO completo y no una fecha suelta: convertir el instante a la
 * zona local es lo que hace que "salió 21:29" sea la hora del reloj de la
 * pared de la escuela y no la del servidor.
 *
 * Un ISO que no se entiende se devuelve tal cual en vez de mostrar
 * "Invalid Date", que no le dice nada a nadie.
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
