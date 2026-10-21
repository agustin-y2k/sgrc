import { aFechaLocal } from "@/lib/fechas"

export function diaAnteriorISO(fecha: string): string {
  const dia = aFechaLocal(fecha)!
  dia.setDate(dia.getDate() - 1)
  return `${dia.getFullYear()}-${String(dia.getMonth() + 1).padStart(2, "0")}-${String(dia.getDate()).padStart(2, "0")}`
}

/** El fin de una franja nocturna pertenece al siguiente día civil. */
export function intervaloDeReserva(reserva: {
  fecha: string
  horaInicio: string
  horaFin: string
}): { inicio: number; fin: number } {
  const inicio = aFechaLocal(reserva.fecha)!
  const fin = new Date(inicio)
  const [hi, mi] = reserva.horaInicio.split(":").map(Number)
  const [hf, mf] = reserva.horaFin.split(":").map(Number)
  inicio.setHours(hi, mi, 0, 0)
  if (reserva.horaFin < reserva.horaInicio) fin.setDate(fin.getDate() + 1)
  fin.setHours(hf, mf, 0, 0)
  return { inicio: inicio.getTime(), fin: fin.getTime() }
}
