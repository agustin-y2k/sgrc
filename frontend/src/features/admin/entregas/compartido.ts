import type { Prestamo } from "@/features/reservas/types"
import { formatearHora } from "@/lib/fechas"

/** Lo que comparten la pantalla de entregas y el panel del laboratorio. */

export const PRESTAMOS_KEY = ["prestamos"]

/** "25 min tarde", "2 h 10 min tarde". */
export function textoDeDemora(minutos: number): string {
  if (minutos < 60) return `${minutos} min tarde`
  const horas = Math.floor(minutos / 60)
  const resto = minutos % 60
  return resto === 0 ? `${horas} h tarde` : `${horas} h ${resto} min tarde`
}

export function hora(iso: string): string {
  return formatearHora(iso)
}

/**
 * Por la etiqueta que resuelve el servidor y no armándola acá: desde que se
 * prestan proyectores y cargadores no todo lo que sale del laboratorio tiene
 * número, y "Equipo" a secas no le dice a nadie qué está devolviendo.
 */
export function nombreDeEquipo(p: Prestamo): string {
  const equipo = p.etiqueta ?? "Equipo"
  return p.carroNombre ? `${equipo} · ${p.carroNombre}` : equipo
}

/** Cada cuánto se vuelve a preguntar qué hay afuera. */
export const REFRESCO_DEL_MOSTRADOR = 60_000
