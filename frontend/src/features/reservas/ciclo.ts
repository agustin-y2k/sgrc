import { calcularRecurrencia } from "./recurrencias"
import type { DiaSemana } from "./types"

export function errorDeCicloLectivo(
  anio: number | undefined,
  desde: string,
  hasta: string,
  inicio: string,
  fin: string,
  dia?: DiaSemana
): string | null {
  if (!anio) return null
  const mensaje = `Las reservas del ciclo ${anio} deben quedar entre el 01/01/${anio} y el 31/12/${anio}.`
  if ([desde, hasta].some((fecha) => fecha && Number(fecha.slice(0, 4)) !== anio))
    return mensaje
  let ultima = desde
  if (dia) {
    const serie = calcularRecurrencia(desde, hasta, dia)
    if (!serie?.primeraFecha || !serie.cantidad) return null
    const fecha = new Date(`${serie.primeraFecha}T00:00:00Z`)
    fecha.setUTCDate(fecha.getUTCDate() + (serie.cantidad - 1) * 7)
    ultima = fecha.toISOString().slice(0, 10)
  }
  if (ultima === `${anio}-12-31` && inicio && fin && fin < inicio && fin !== "00:00")
    return mensaje
  return null
}
