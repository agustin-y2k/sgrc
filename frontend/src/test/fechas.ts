/** Fechas relativas a hoy para los tests de formularios. */

function aISO(d: Date): string {
  const mes = String(d.getMonth() + 1).padStart(2, "0")
  const dia = String(d.getDate()).padStart(2, "0")
  return `${d.getFullYear()}-${mes}-${dia}`
}

/** Una fecha dentro de `dias` días. */
export function fechaFuturaEnDias(dias: number): string {
  const d = new Date()
  d.setDate(d.getDate() + dias)
  return aISO(d)
}
