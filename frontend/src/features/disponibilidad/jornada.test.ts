import { describe, expect, it } from "vitest"

import { dentroDeLaJornada, diaDeLaFecha, diasDeLaJornada } from "./types"
import type { BloqueHorario, DiaSemana } from "./types"

function tramo(diaSemana: DiaSemana, horaInicio: string, horaFin: string): BloqueHorario {
  return { id: `${diaSemana}-${horaInicio}`, diaSemana, horaInicio, horaFin }
}

describe("diaDeLaFecha", () => {
  // El 8 de agosto de 2026 es sábado. Es el caso que rompía antes: con
  // `new Date("2026-08-08")` la fecha se lee como medianoche UTC y al oeste
  // de Greenwich cae el día anterior, así que un sábado se leía viernes.
  it("lee la fecha en hora local, no en UTC", () => {
    expect(diaDeLaFecha("2026-08-08")).toBe("SABADO")
    expect(diaDeLaFecha("2026-08-09")).toBe("DOMINGO")
    expect(diaDeLaFecha("2026-08-10")).toBe("LUNES")
  })

  it("devuelve null si la fecha está incompleta", () => {
    expect(diaDeLaFecha("")).toBeNull()
    expect(diaDeLaFecha("2026-08")).toBeNull()
  })
})

describe("diasDeLaJornada", () => {
  it("sin jornada declarada muestra los siete días", () => {
    expect(diasDeLaJornada([])).toHaveLength(7)
  })

  it("con jornada declarada muestra solo los días con tramos, en orden de semana", () => {
    const jornada = [
      tramo("SABADO", "08:00", "13:00"),
      tramo("LUNES", "07:00", "12:00"),
      tramo("LUNES", "18:00", "23:00"),
    ]
    expect(diasDeLaJornada(jornada)).toEqual(["LUNES", "SABADO"])
  })
})

describe("dentroDeLaJornada", () => {
  // La distinción que sostiene el diseño: sin jornada cargada el sistema no
  // restringe nada. Si esto devolviera false, instalar el sistema y no
  // configurar nada dejaría a todos sin poder reservar.
  it("sin jornada declarada permite cualquier día y hora", () => {
    expect(dentroDeLaJornada([], "2026-08-09", "03:00", "05:00")).toBe(true)
  })

  it("rechaza un día sin tramos declarados", () => {
    const jornada = [tramo("LUNES", "07:00", "12:00")]
    // 2026-08-08 es sábado.
    expect(dentroDeLaJornada(jornada, "2026-08-08", "08:00", "09:00")).toBe(false)
  })

  it("exige que la reserva entre entera en un tramo", () => {
    const jornada = [tramo("SABADO", "08:00", "13:00")]
    expect(dentroDeLaJornada(jornada, "2026-08-08", "09:00", "12:00")).toBe(true)
    expect(dentroDeLaJornada(jornada, "2026-08-08", "08:00", "13:00")).toBe(true)
    expect(dentroDeLaJornada(jornada, "2026-08-08", "07:00", "12:00")).toBe(false)
    expect(dentroDeLaJornada(jornada, "2026-08-08", "12:00", "14:00")).toBe(false)
  })

  // Turno mañana y turno noche: el mediodía está cerrado y una reserva no
  // puede cruzarlo.
  it("no deja cruzar el hueco entre dos turnos", () => {
    const jornada = [tramo("LUNES", "07:00", "12:00"), tramo("LUNES", "18:00", "23:00")]
    expect(dentroDeLaJornada(jornada, "2026-08-10", "08:00", "11:00")).toBe(true)
    expect(dentroDeLaJornada(jornada, "2026-08-10", "19:00", "22:00")).toBe(true)
    expect(dentroDeLaJornada(jornada, "2026-08-10", "11:00", "19:00")).toBe(false)
  })

  // Dos tramos que se tocan describen un día abierto de punta a punta. Sin
  // fusionarlos, una reserva que cruza la juntura se rechazaría aunque en la
  // pantalla el día figure abierto de 7 a 18.
  it("fusiona tramos contiguos", () => {
    const jornada = [tramo("LUNES", "12:00", "18:00"), tramo("LUNES", "07:00", "12:00")]
    expect(dentroDeLaJornada(jornada, "2026-08-10", "11:00", "13:00")).toBe(true)
  })

  it("con la fecha incompleta no se adelanta: valida el backend", () => {
    const jornada = [tramo("LUNES", "07:00", "12:00")]
    expect(dentroDeLaJornada(jornada, "", "08:00", "09:00")).toBe(true)
  })
})
