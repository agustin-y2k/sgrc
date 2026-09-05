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
 *
 * El carro NO es opcional cuando existe: el identificador es el número del
 * zócalo, así que "PC 1" hay uno por carro. En estas pantallas alguien va
 * FÍSICAMENTE a buscar la máquina, y tres chips que dicen "PC 1" no le dicen a
 * dónde ir.
 *
 * Recibe la forma y no el tipo entero porque un préstamo y una reserva traen
 * estos dos campos del mismo JOIN y se nombran igual en pantalla.
 */
export function nombreDeEquipo(p: { etiqueta?: string; carroNombre?: string }): string {
  const equipo = p.etiqueta ?? "Equipo"
  return p.carroNombre ? `${equipo} · ${p.carroNombre}` : equipo
}

/** Cada cuánto se vuelve a preguntar qué hay afuera. */
export const REFRESCO_DEL_MOSTRADOR = 60_000

/**
 * Los lugares de una escuela a los que se lleva un equipo y que no son un
 * curso. Se ofrecen como sugerencia junto a los cursos del ciclo, y el campo
 * sigue siendo libre: ninguna lista de destinos se puede cerrar sin que el
 * primer caso no previsto quede sin poder anotarse.
 */
export const LUGARES_DE_LA_ESCUELA = [
  "Biblioteca",
  "Dirección",
  "Secretaría",
  "Sección Alumnos",
  "Preceptoría",
  "Sala de profesores",
  "Laboratorio",
  "Taller",
]
