import { DIAS_SEMANA, type DiaSemana } from "@/features/reservas/types"

export const MAX_OCURRENCIAS = 45
const DIA_MS = 24 * 60 * 60 * 1000

/** Días civiles en UTC para contar semanas sin que DST altere la duración. */
export function calcularRecurrencia(
  desde: string,
  hasta: string,
  dia: DiaSemana
): { primeraFecha: string | null; cantidad: number } | null {
  if (!desde || !hasta) return null
  const inicio = Date.parse(`${desde}T00:00:00Z`)
  const fin = Date.parse(`${hasta}T00:00:00Z`)
  if (!Number.isFinite(inicio) || !Number.isFinite(fin)) return null
  const diaElegido = (DIAS_SEMANA.findIndex((d) => d.valor === dia) + 1) % 7
  const desplazamiento = (diaElegido - new Date(inicio).getUTCDay() + 7) % 7
  const primera = inicio + desplazamiento * DIA_MS
  if (primera > fin) return { primeraFecha: null, cantidad: 0 }
  return {
    primeraFecha: new Date(primera).toISOString().slice(0, 10),
    cantidad: 1 + Math.floor((fin - primera) / (7 * DIA_MS)),
  }
}
