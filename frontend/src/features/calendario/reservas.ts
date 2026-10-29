import type { GrupoDeReservas, ReservaDetallada } from "@/features/reservas/types"
import { agruparReservas } from "@/features/reservas/types"
import {
  aFechaISO,
  aMinutos,
  desdeFechaISO,
  fechasDeLaSemana,
  lunesDeLaSemana,
  sumarDias,
} from "./semana"

export type VistaCalendario = "semana" | "mes" | "agenda"
export type ReservaCalendario = ReservaDetallada & {
  propia: boolean
  cursoModalidad?: string
}
export type ClaseCalendario = GrupoDeReservas & {
  clave: string
  propia: boolean
  cursoModalidad?: string
}
export type SegmentoClase = {
  clase: ClaseCalendario
  fecha: string
  inicio: number
  fin: number
  continua: boolean
}

export function fechasDelCalendario(referencia: Date, vista: VistaCalendario): string[] {
  if (vista === "semana") return fechasDeLaSemana(referencia)
  if (vista === "agenda")
    return Array.from({ length: 7 }, (_, i) => aFechaISO(sumarDias(referencia, i)))
  const primero = new Date(referencia.getFullYear(), referencia.getMonth(), 1)
  return Array.from({ length: 42 }, (_, i) =>
    aFechaISO(sumarDias(lunesDeLaSemana(primero), i))
  )
}

export function moverCalendario(
  referencia: Date,
  vista: VistaCalendario,
  sentido: number
): Date {
  return vista === "mes"
    ? new Date(referencia.getFullYear(), referencia.getMonth() + sentido, 1)
    : sumarDias(referencia, sentido * 7)
}

export function clasesDelCalendario(reservas: ReservaCalendario[]): ClaseCalendario[] {
  const propias = new Map(reservas.map((r) => [r.id, r.propia]))
  const modalidades = new Map(reservas.map((r) => [r.id, r.cursoModalidad]))
  return agruparReservas(reservas).map((grupo) => ({
    ...grupo,
    clave: grupo.grupoId ?? grupo.bloqueoOperacionId ?? grupo.reservas[0].id,
    propia: propias.get(grupo.reservas[0].id) ?? false,
    cursoModalidad: modalidades.get(grupo.reservas[0].id),
  }))
}

export function segmentosDeClase(clase: ClaseCalendario): SegmentoClase[] {
  const inicio = aMinutos(clase.horaInicio)
  const fin = aMinutos(clase.horaFin)
  if (fin > inicio) return [{ clase, fecha: clase.fecha, inicio, fin, continua: false }]
  const segmentos = [{ clase, fecha: clase.fecha, inicio, fin: 1440, continua: false }]
  if (fin > 0)
    segmentos.push({
      clase,
      fecha: aFechaISO(sumarDias(desdeFechaISO(clase.fecha), 1)),
      inicio: 0,
      fin,
      continua: true,
    })
  return segmentos
}

/** Las clases simultáneas se distribuyen en columnas dentro de cada franja. */
export function distribuirClases(segmentos: SegmentoClase[]) {
  const resultado: (SegmentoClase & { columna: number; columnas: number })[] = []
  let grupo: (SegmentoClase & { columna: number })[] = []
  let finales: number[] = []
  let finGrupo = -1
  const cerrar = () => {
    resultado.push(...grupo.map((s) => ({ ...s, columnas: finales.length })))
    grupo = []
    finales = []
    finGrupo = -1
  }
  for (const segmento of [...segmentos].sort(
    (a, b) =>
      a.inicio - b.inicio || a.fin - b.fin || a.clase.clave.localeCompare(b.clase.clave)
  )) {
    if (segmento.inicio >= finGrupo) cerrar()
    let columna = finales.findIndex((fin) => fin <= segmento.inicio)
    if (columna < 0) columna = finales.length
    finales[columna] = segmento.fin
    finGrupo = Math.max(finGrupo, segmento.fin)
    grupo.push({ ...segmento, columna })
  }
  cerrar()
  return resultado
}

export function estadoDeClase(clase: ClaseCalendario): string {
  const vigentes = clase.reservas.filter((r) => r.estado !== "CANCELADA")
  if (vigentes.length === 0) return "Cancelada"
  if (vigentes.length < clase.reservas.length) return "Con cambios de equipos"
  if (vigentes.every((r) => r.estado === "NO_RETIRADA")) return "No retirada"
  if (vigentes.every((r) => r.estado === "FINALIZADA")) return "Finalizada"
  return "Confirmada"
}

/** Da espacio a cada clase sin agrandar los huecos ni tapar la siguiente. */
export function crearEjeSemanal(
  inicio: number,
  fin: number,
  segmentos: SegmentoClase[],
  minimo: number
) {
  const puntos = [
    ...new Set([
      inicio,
      fin,
      ...Array.from({ length: (fin - inicio) / 60 }, (_, i) => inicio + i * 60),
      ...segmentos.flatMap((s) => [s.inicio, s.fin]),
    ]),
  ].sort((a, b) => a - b)
  const indices = new Map(puntos.map((punto, i) => [punto, i]))
  const iniciosPorFin = new Map<number, number[]>()
  for (const s of segmentos)
    iniciosPorFin.set(s.fin, [...(iniciosPorFin.get(s.fin) ?? []), s.inicio])
  const posiciones = [0]
  for (let i = 1; i < puntos.length; i++) {
    let posicion = posiciones[i - 1] + (puntos[i] - puntos[i - 1]) * 1.2
    for (const comienzo of iniciosPorFin.get(puntos[i]) ?? [])
      posicion = Math.max(posicion, posiciones[indices.get(comienzo)!] + minimo)
    posiciones.push(posicion)
  }
  return {
    alto: posiciones[posiciones.length - 1],
    ampliado: posiciones[posiciones.length - 1] > (fin - inicio) * 1.2 + 0.01,
    posicion(minuto: number) {
      const i = puntos.findIndex((p) => p >= minuto)
      if (i <= 0) return 0
      return (
        posiciones[i - 1] +
        ((minuto - puntos[i - 1]) / (puntos[i] - puntos[i - 1])) *
          (posiciones[i] - posiciones[i - 1])
      )
    },
  }
}
