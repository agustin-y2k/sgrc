import type { Prestamo } from "@/features/reservas/types"

/**
 * Lo que comparten la pantalla de entregas y el panel del laboratorio.
 *
 * Están acá y no duplicados en cada pantalla porque son las palabras con las
 * que el Admin lee el mostrador: si "25 min tarde" se escribiera distinto en
 * dos lugares, la misma máquina se vería diferente según por dónde se mire.
 */

export const PRESTAMOS_KEY = ["prestamos"]

/** "25 min tarde", "2 h 10 min tarde". */
export function textoDeDemora(minutos: number): string {
  if (minutos < 60) return `${minutos} min tarde`
  const horas = Math.floor(minutos / 60)
  const resto = minutos % 60
  return resto === 0 ? `${horas} h tarde` : `${horas} h ${resto} min tarde`
}

export function hora(iso: string): string {
  return new Date(iso).toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit" })
}

/**
 * Por la etiqueta que resuelve el servidor y no armándola acá: desde que se
 * prestan proyectores y cargadores (015) no todo lo que sale del laboratorio
 * tiene número, y "PC" a secas no le dice a nadie qué está devolviendo.
 */
export function nombreDePC(p: Prestamo): string {
  const equipo = p.etiqueta ?? "Equipo"
  return p.carroNombre ? `${equipo} · ${p.carroNombre}` : equipo
}

/**
 * Cada cuánto se vuelve a preguntar qué hay afuera.
 *
 * El mostrador lo atienden varios Admin a la vez: si uno recibe una máquina,
 * la pantalla del otro tiene que enterarse sin que nadie apriete recargar.
 * Un minuto es suficiente —nadie devuelve dos computadoras en menos— y no
 * castiga a la base: es una consulta sobre lo que está prestado ahora, que
 * son unas pocas filas.
 */
export const REFRESCO_DEL_MOSTRADOR = 60_000
