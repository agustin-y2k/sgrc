import { describe, expect, it } from "vitest"

import { agruparTramos, etiquetaDeDias, ordenarDias } from "./jornada"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"

function bloque(
  diaSemana: DiaSemana,
  horaInicio: string,
  horaFin: string
): BloqueHorario {
  return { id: `${diaSemana}-${horaInicio}`, diaSemana, horaInicio, horaFin }
}

describe("ordenarDias", () => {
  it("los pone como transcurre la semana", () => {
    expect(ordenarDias(["DOMINGO", "MIERCOLES", "LUNES"])).toEqual([
      "LUNES",
      "MIERCOLES",
      "DOMINGO",
    ])
  })
})

describe("etiquetaDeDias", () => {
  it("los siete días son todos los días", () => {
    const todos: DiaSemana[] = [
      "LUNES",
      "MARTES",
      "MIERCOLES",
      "JUEVES",
      "VIERNES",
      "SABADO",
      "DOMINGO",
    ]
    expect(etiquetaDeDias(todos)).toBe("Todos los días")
  })

  it("tres o más días seguidos se dicen como un rango", () => {
    expect(etiquetaDeDias(["LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES"])).toBe(
      "Lunes a viernes"
    )
    expect(etiquetaDeDias(["MIERCOLES", "JUEVES", "VIERNES"])).toBe("Miércoles a viernes")
  })

  // "Sábados a domingos" es más largo que enumerarlos y suena a error.
  it("dos días, aunque sean seguidos, se enumeran", () => {
    expect(etiquetaDeDias(["SABADO", "DOMINGO"])).toBe("Sábado y domingo")
  })

  it("los días sueltos se enumeran en orden, sin importar cómo lleguen", () => {
    expect(etiquetaDeDias(["VIERNES", "LUNES", "MIERCOLES"])).toBe(
      "Lunes, miércoles y viernes"
    )
  })

  it("un solo día es su nombre", () => {
    expect(etiquetaDeDias(["SABADO"])).toBe("Sábado")
  })
})

describe("agruparTramos", () => {
  // El caso que motivó todo: la semana entera cargada con el mismo horario
  // es una línea, no cinco.
  it("junta los días que comparten horario", () => {
    const tramos = agruparTramos([
      bloque("LUNES", "07:30", "12:30"),
      bloque("MARTES", "07:30", "12:30"),
      bloque("MIERCOLES", "07:30", "12:30"),
    ])

    expect(tramos).toHaveLength(1)
    expect(tramos[0].dias).toEqual(["LUNES", "MARTES", "MIERCOLES"])
    expect(tramos[0].bloques.map((b) => b.id)).toEqual([
      "LUNES-07:30",
      "MARTES-07:30",
      "MIERCOLES-07:30",
    ])
  })

  it("no junta horarios distintos aunque sean del mismo día", () => {
    const tramos = agruparTramos([
      bloque("LUNES", "18:00", "22:00"),
      bloque("LUNES", "07:30", "12:30"),
    ])

    expect(tramos.map((t) => [t.horaInicio, t.horaFin])).toEqual([
      ["07:30", "12:30"],
      ["18:00", "22:00"],
    ])
  })

  // Contiguos no es lo mismo que uno solo: son dos cosas que alguien cargó
  // por separado y va a querer editar por separado.
  it("no fusiona tramos que se tocan", () => {
    const tramos = agruparTramos([
      bloque("LUNES", "07:00", "12:00"),
      bloque("LUNES", "12:00", "18:00"),
    ])

    expect(tramos).toHaveLength(2)
  })

  it("ordena por el día más temprano de cada grupo y después por hora", () => {
    const tramos = agruparTramos([
      bloque("SABADO", "08:00", "12:00"),
      bloque("LUNES", "18:00", "22:00"),
      bloque("VIERNES", "18:00", "22:00"),
      bloque("LUNES", "07:30", "12:30"),
    ])

    expect(tramos.map((t) => `${etiquetaDeDias(t.dias)} ${t.horaInicio}`)).toEqual([
      "Lunes 07:30",
      "Lunes y viernes 18:00",
      "Sábado 08:00",
    ])
  })

  it("sin bloques no hay tramos", () => {
    expect(agruparTramos([])).toEqual([])
  })

  // Una escuela real: doble turno casi toda la semana, un día corrido
  // distinto, y el fin de semana cerrado.
  it("doble turno con un día que difiere se lee en tres líneas", () => {
    const casiTodos: DiaSemana[] = ["LUNES", "MIERCOLES", "JUEVES", "VIERNES"]
    const jornada = [
      ...casiTodos.flatMap((d) => [
        bloque(d, "08:00", "12:00"),
        bloque(d, "13:00", "18:00"),
      ]),
      bloque("MARTES", "08:00", "14:00"),
    ]

    expect(
      agruparTramos(jornada).map(
        (t) => `${etiquetaDeDias(t.dias)} de ${t.horaInicio} a ${t.horaFin}`
      )
    ).toEqual([
      "Lunes, miércoles, jueves y viernes de 08:00 a 12:00",
      "Lunes, miércoles, jueves y viernes de 13:00 a 18:00",
      "Martes de 08:00 a 14:00",
    ])
  })
})
