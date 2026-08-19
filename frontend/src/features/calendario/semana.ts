// Helpers de la vista semanal.

// Los siete días.
export const DIAS_SEMANA = [
  "Lunes",
  "Martes",
  "Miércoles",
  "Jueves",
  "Viernes",
  "Sábado",
  "Domingo",
] as const

/** YYYY-MM-DD de un Date, leído en hora local (no UTC). */
export function aFechaISO(d: Date): string {
  const mes = String(d.getMonth() + 1).padStart(2, "0")
  const dia = String(d.getDate()).padStart(2, "0")
  return `${d.getFullYear()}-${mes}-${dia}`
}

/** Parsea YYYY-MM-DD como fecha local (new Date("...") la leería como UTC). */
export function desdeFechaISO(iso: string): Date {
  const [anio, mes, dia] = iso.split("-").map(Number)
  return new Date(anio, mes - 1, dia)
}

/** Lunes de la semana que contiene a `fecha`. */
export function lunesDeLaSemana(fecha: Date): Date {
  const d = new Date(fecha.getFullYear(), fecha.getMonth(), fecha.getDate())
  const diaSemana = d.getDay() // 0 = domingo
  const diasHastaElLunes = diaSemana === 0 ? -6 : 1 - diaSemana
  d.setDate(d.getDate() + diasHastaElLunes)
  return d
}

export function sumarDias(fecha: Date, dias: number): Date {
  const d = new Date(fecha.getFullYear(), fecha.getMonth(), fecha.getDate())
  d.setDate(d.getDate() + dias)
  return d
}

/** Las siete fechas (lunes a domingo) de la semana de `fecha`. */
export function fechasDeLaSemana(fecha: Date): string[] {
  const lunes = lunesDeLaSemana(fecha)
  return DIAS_SEMANA.map((_, i) => aFechaISO(sumarDias(lunes, i)))
}

/** "HH:MM" → minutos desde medianoche. */
export function aMinutos(hora: string): number {
  const [h, m] = hora.split(":").map(Number)
  return h * 60 + m
}

export function formatearRangoSemana(fecha: Date): string {
  const lunes = lunesDeLaSemana(fecha)
  const domingo = sumarDias(lunes, 6)
  const fmt = (d: Date) =>
    `${String(d.getDate()).padStart(2, "0")}/${String(d.getMonth() + 1).padStart(2, "0")}`
  return `${fmt(lunes)} – ${fmt(domingo)}/${domingo.getFullYear()}`
}

/** "Lunes", "Sábado"… para una fecha "YYYY-MM-DD". */
export function etiquetaDeDia(fechaISO: string): string {
  return DIAS_SEMANA[(desdeFechaISO(fechaISO).getDay() + 6) % 7]
}
