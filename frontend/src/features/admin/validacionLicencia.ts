import type { CamposLicencia } from "./FormularioLicencia"

export function fechaDeLicenciaValida(fecha: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(fecha) || fecha < "0001-01-01") return false
  const valor = new Date(`${fecha}T00:00:00Z`)
  return Number.isFinite(valor.getTime()) && valor.toISOString().slice(0, 10) === fecha
}

export function licenciaValida(c: CamposLicencia): boolean {
  const entero = (s: string, min: number, max: number) =>
    s.trim() !== "" && Number.isInteger(Number(s)) && Number(s) >= min && Number(s) <= max
  if (
    !c.nombre.trim() ||
    [...c.nombre.trim()].length > 100 ||
    !entero(c.diasDuracion, 1, 3650) ||
    !entero(c.diasAviso, 0, 365)
  )
    return false
  const v = c.vencimiento
  switch (v.forma) {
    case "sin-fecha":
      return true
    case "quedan-dias":
      return entero(v.quedanDias, 0, 3650)
    case "renovada-el":
      return fechaDeLicenciaValida(v.renovadaEl)
    case "vence-el":
      return fechaDeLicenciaValida(v.venceEl)
  }
}
