import { aFechaLocal } from "@/lib/fechas"

export function diaAnteriorISO(fecha: string): string {
  return desplazarDiaISO(fecha, -1)
}

export function diaSiguienteISO(fecha: string): string {
  return desplazarDiaISO(fecha, 1)
}

function desplazarDiaISO(fecha: string, dias: number): string {
  const dia = aFechaLocal(fecha)!
  dia.setDate(dia.getDate() + dias)
  return `${dia.getFullYear()}-${String(dia.getMonth() + 1).padStart(2, "0")}-${String(dia.getDate()).padStart(2, "0")}`
}

type ReservaParaEntrega = {
  id: string
  equipoId: string
  estado: string
  fecha: string
  horaInicio: string
  horaFin: string
}

/** Una salida anticipada ocupa la PC desde ahora hasta el fin de su clase. */
export function conflictoDeEntregaAnticipada<T extends ReservaParaEntrega>(
  reserva: ReservaParaEntrega,
  reservas: readonly T[],
  ahora: Date
): T | undefined {
  const retiro = intervaloDeReserva(reserva)
  const instante = ahora.getTime()
  if (instante >= retiro.inicio) return undefined
  return reservas
    .filter((otra) => {
      if (
        otra.id === reserva.id ||
        otra.equipoId !== reserva.equipoId ||
        otra.estado !== "CONFIRMADA"
      )
        return false
      const franja = intervaloDeReserva(otra)
      return franja.inicio < retiro.fin && instante < franja.fin
    })
    .sort((a, b) => intervaloDeReserva(a).inicio - intervaloDeReserva(b).inicio)[0]
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

/** Mismo día de retiro que valida la API, incluyendo la franja nocturna de ayer. */
export function puedeEntregarseReserva(
  reserva: {
    estado: string
    fecha: string
    horaInicio: string
    horaFin: string
  },
  ahora: Date
): boolean {
  return (
    (reserva.estado === "CONFIRMADA" || reserva.estado === "NO_RETIRADA") &&
    aFechaLocal(reserva.fecha)!.getTime() <= ahora.getTime() &&
    ahora.getTime() < intervaloDeReserva(reserva).fin
  )
}
