import {
  aFechaISO,
  aMinutos,
  desdeFechaISO,
  fechasDeLaSemana,
  lunesDeLaSemana,
  sumarDias,
} from "@/features/calendario/semana"

describe("aFechaISO / desdeFechaISO", () => {
  it("hacen ida y vuelta sin correrse de día", () => {
    expect(aFechaISO(new Date(2026, 2, 9))).toBe("2026-03-09")
    expect(aFechaISO(desdeFechaISO("2026-03-09"))).toBe("2026-03-09")
  })

  // new Date("2026-03-09") se parsea como UTC y en UTC-3 cae el día 8.
  // desdeFechaISO existe justamente para no pisar esa piedra.
  it("desdeFechaISO interpreta la fecha en hora local, no en UTC", () => {
    const d = desdeFechaISO("2026-03-09")
    expect(d.getDate()).toBe(9)
    expect(d.getMonth()).toBe(2)
  })
})

describe("lunesDeLaSemana", () => {
  it("desde un miércoles devuelve el lunes anterior", () => {
    // 2026-03-11 es miércoles.
    expect(aFechaISO(lunesDeLaSemana(desdeFechaISO("2026-03-11")))).toBe("2026-03-09")
  })

  it("desde el propio lunes se queda en ese lunes", () => {
    expect(aFechaISO(lunesDeLaSemana(desdeFechaISO("2026-03-09")))).toBe("2026-03-09")
  })

  it("desde el sábado devuelve el lunes de esa misma semana", () => {
    expect(aFechaISO(lunesDeLaSemana(desdeFechaISO("2026-03-14")))).toBe("2026-03-09")
  })

  // El caso que más fácil se rompe: getDay() del domingo es 0.
  it("desde el domingo devuelve el lunes de la semana que termina, no la que empieza", () => {
    expect(aFechaISO(lunesDeLaSemana(desdeFechaISO("2026-03-15")))).toBe("2026-03-09")
  })
})

describe("fechasDeLaSemana", () => {
  it("devuelve los siete días, de lunes a domingo", () => {
    expect(fechasDeLaSemana(desdeFechaISO("2026-03-11"))).toEqual([
      "2026-03-09",
      "2026-03-10",
      "2026-03-11",
      "2026-03-12",
      "2026-03-13",
      "2026-03-14",
      "2026-03-15",
    ])
  })

  it("cruza el fin de mes sin romperse", () => {
    // 2026-03-30 es lunes; la semana termina el domingo 5 de abril.
    expect(fechasDeLaSemana(desdeFechaISO("2026-03-30"))).toEqual([
      "2026-03-30",
      "2026-03-31",
      "2026-04-01",
      "2026-04-02",
      "2026-04-03",
      "2026-04-04",
      "2026-04-05",
    ])
  })
})

describe("sumarDias", () => {
  it("cruza el cambio de año", () => {
    expect(aFechaISO(sumarDias(desdeFechaISO("2026-12-31"), 1))).toBe("2027-01-01")
  })

  it("acepta días negativos", () => {
    expect(aFechaISO(sumarDias(desdeFechaISO("2026-03-09"), -7))).toBe("2026-03-02")
  })
})

describe("aMinutos", () => {
  it("convierte HH:MM a minutos desde medianoche", () => {
    expect(aMinutos("00:00")).toBe(0)
    expect(aMinutos("08:30")).toBe(510)
    expect(aMinutos("23:59")).toBe(1439)
  })
})
