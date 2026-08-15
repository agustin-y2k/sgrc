import { describe, expect, it } from "vitest"

import { cruzaMedianoche, duracionEnMinutos, excedeDuracionMaxima } from "./types"

// Espejo de los tests de domain: una escuela nocturna dicta de 22:00 a 01:00 y
// el formulario tiene que entenderlo igual que el backend.

describe("duracionEnMinutos", () => {
  it("mide bien una franja normal", () => {
    expect(duracionEnMinutos("08:00", "12:00")).toBe(240)
  })

  // Una resta cruda daba −1260: negativo, así que nunca superaba ningún tope.
  it("mide bien una franja que cruza la medianoche", () => {
    expect(duracionEnMinutos("22:00", "01:00")).toBe(180)
    expect(duracionEnMinutos("23:30", "00:30")).toBe(60)
  })

  it("con la hora incompleta no inventa nada", () => {
    expect(duracionEnMinutos("", "01:00")).toBeNull()
  })
})

describe("cruzaMedianoche", () => {
  it("distingue las tres situaciones", () => {
    expect(cruzaMedianoche("08:00", "12:00")).toBe(false)
    expect(cruzaMedianoche("22:00", "01:00")).toBe(true)
    expect(cruzaMedianoche("08:00", "08:00")).toBe(false)
  })
})

describe("excedeDuracionMaxima", () => {
  it("una nocturna corta entra en el tope", () => {
    expect(excedeDuracionMaxima("22:00", "01:00")).toBe(false)
  })

  it("una nocturna larga lo supera", () => {
    expect(excedeDuracionMaxima("20:00", "06:00")).toBe(true)
  })
})
