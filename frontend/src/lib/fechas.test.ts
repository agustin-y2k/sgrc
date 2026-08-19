import {
  etiquetaDeDia,
  formatearFechaCortaYHora,
  formatearFechaLarga,
  formatearFechaYHora,
  formatearHora,
} from "@/lib/fechas"

/**
 * Un instante conocido, escrito en hora local para que el test diga lo mismo
 * corra donde corra.
 */
function instante(anio: number, mes: number, dia: number, hora: number, min: number) {
  return new Date(anio, mes - 1, dia, hora, min).toISOString()
}

describe("formatearHora", () => {
  // El defecto que motiva estos tests: el `es-AR` de un navegador resuelve a
  // 12 horas por defecto, así que las 21:29 se veían "09:29 p.
  it("usa 24 horas, no a. m. / p. m.", () => {
    expect(formatearHora(instante(2026, 8, 18, 21, 29))).toBe("21:29")
  })

  it("la tarde no se confunde con la mañana", () => {
    expect(formatearHora(instante(2026, 8, 18, 13, 5))).toBe("13:05")
    expect(formatearHora(instante(2026, 8, 18, 1, 5))).toBe("01:05")
  })

  // La otra trampa: pedir "no 12 horas" puede dar el ciclo h24, que escribe
  // la medianoche "24:00" y la deja como la última hora del día anterior.
  it("la medianoche es 00:00 y no 24:00", () => {
    expect(formatearHora(instante(2026, 8, 18, 0, 0))).toBe("00:00")
    expect(formatearHora(instante(2026, 8, 18, 0, 30))).toBe("00:30")
  })

  it("el mediodía es 12:00", () => {
    expect(formatearHora(instante(2026, 8, 18, 12, 0))).toBe("12:00")
  })

  // Un timestamp roto no puede terminar en "Invalid Date" en la pantalla.
  it("un ISO que no se entiende se devuelve tal cual", () => {
    expect(formatearHora("no-es-una-fecha")).toBe("no-es-una-fecha")
  })
})

describe("formatearFechaCortaYHora", () => {
  // Sin año, el locale escribe el mes sin el cero adelante ("18/8" y no
  // "18/08"), a diferencia de la variante con año.
  it("día, mes y hora en 24", () => {
    expect(formatearFechaCortaYHora(instante(2026, 8, 18, 21, 29))).toBe("18/8, 21:29")
  })

  it("la medianoche no adelanta ni atrasa el día", () => {
    expect(formatearFechaCortaYHora(instante(2026, 8, 18, 0, 0))).toBe("18/8, 00:00")
  })
})

describe("formatearFechaYHora", () => {
  it("lleva el año, para un historial viejo", () => {
    expect(formatearFechaYHora(instante(2026, 8, 18, 21, 29))).toBe("18/08/2026, 21:29")
  })

  it("la medianoche es 00:00", () => {
    expect(formatearFechaYHora(instante(2026, 8, 18, 0, 17))).toBe("18/08/2026, 00:17")
  })
})

// Los que ya existían no tenían test propio; van acá porque comparten el
// mismo criterio de "una fecha que no se entiende se muestra sin romper".
describe("fechas del calendario", () => {
  it("formatearFechaLarga escribe el día de la semana", () => {
    expect(formatearFechaLarga("2026-08-18")).toMatch(/martes.*18.*agosto/i)
  })

  it("etiquetaDeDia abrevia los dos días que se miran", () => {
    expect(etiquetaDeDia("2026-08-18", "2026-08-18")).toBe("Hoy")
    expect(etiquetaDeDia("2026-08-19", "2026-08-18")).toBe("Mañana")
    expect(etiquetaDeDia("2026-08-25", "2026-08-18")).toMatch(/martes.*25/i)
  })
})
