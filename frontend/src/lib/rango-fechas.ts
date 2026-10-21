/** Valida días civiles sin convertirlos a la zona horaria del navegador. */
export function errorRangoFechas(desde = "", hasta = "", maxDias?: number): string {
  const instante = (fecha: string) => Date.parse(`${fecha}T00:00:00Z`)
  for (const fecha of [desde, hasta]) {
    if (
      fecha &&
      (!/^\d{4}-\d{2}-\d{2}$/.test(fecha) ||
        !Number.isFinite(instante(fecha)) ||
        new Date(instante(fecha)).toISOString().slice(0, 10) !== fecha)
    ) {
      return "Ingresá fechas válidas con un año de cuatro cifras."
    }
  }
  if (desde && hasta) {
    if (desde > hasta) return "La fecha Desde no puede ser posterior a Hasta."
    if (
      maxDias !== undefined &&
      instante(hasta) - instante(desde) > maxDias * 86_400_000
    ) {
      return `El intervalo entre las fechas no puede superar ${maxDias} días.`
    }
  }
  return ""
}
