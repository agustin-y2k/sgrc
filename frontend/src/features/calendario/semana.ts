// Helpers de la vista semanal. Se trabaja con fechas "planas" (YYYY-MM-DD)
// y horas "planas" (HH:MM) igual que el backend: las columnas de la base
// son DATE y TIME sin zona, y representan la hora de pared de la escuela
// (ver zonaHorariaDeLaEscuela en cmd/main.go). Meter Date con husos en el
// medio solo agrega oportunidades de correr todo un día o unas horas.

// Los siete días. El sistema no supone cuáles usa la institución: las
// escuelas de jornada extendida o albergue dictan el fin de semana, y la
// vista semanal tiene que poder mostrar lo que ahí se reserve.
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

/**
 * Lunes de la semana que contiene a `fecha`. El domingo se considera parte
 * de la semana que termina, no de la que empieza: es la convención local, y
 * ahora importa de verdad — el domingo es una columna más del calendario, y
 * de este cálculo depende que caiga al final de su semana y no al principio
 * de la siguiente.
 */
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

/**
 * "Lunes", "Sábado"… para una fecha "YYYY-MM-DD".
 *
 * Existe porque la cabecera del calendario dejó de poder indexar DIAS_SEMANA
 * por posición: ahora se dibujan solo los días que la escuela declaró, así
 * que la tercera columna no es necesariamente el miércoles. La etiqueta sale
 * de la fecha misma, que es la única fuente que no se desalinea.
 *
 * El `(getDay() + 6) % 7` corre el origen del domingo (0 en JS) al lunes, que
 * es donde empieza DIAS_SEMANA.
 */
export function etiquetaDeDia(fechaISO: string): string {
  return DIAS_SEMANA[(desdeFechaISO(fechaISO).getDay() + 6) % 7]
}
