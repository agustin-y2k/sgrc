import {
  conflictoDeEntregaAnticipada,
  diaAnteriorISO,
  diaSiguienteISO,
  puedeEntregarseReserva,
} from "./intervalo"

describe("ventana de entrega", () => {
  const r = {
    fecha: "2026-03-02",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
  }
  it.each(["CONFIRMADA", "NO_RETIRADA"])(
    "permite %s vigente, incluido el retiro anticipado del día",
    (estado) => {
      expect(puedeEntregarseReserva({ ...r, estado }, new Date(2026, 2, 2, 7, 30))).toBe(
        true
      )
      expect(puedeEntregarseReserva({ ...r, estado }, new Date(2026, 2, 2, 8, 30))).toBe(
        true
      )
    }
  )
  it.each(["CANCELADA", "FINALIZADA"])(
    "rechaza %s aunque el horario siga en curso",
    (estado) => {
      expect(puedeEntregarseReserva({ ...r, estado }, new Date(2026, 2, 2, 8, 30))).toBe(
        false
      )
    }
  )
  it("rechaza al llegar al fin, después del fin y en otro día", () => {
    for (const ahora of [
      new Date(2026, 2, 2, 9),
      new Date(2026, 2, 2, 12),
      new Date(2026, 2, 1, 23),
      new Date(2026, 2, 3, 0),
    ])
      expect(puedeEntregarseReserva(r, ahora)).toBe(false)
  })
  it("mantiene la nocturna de ayer hasta su fin exclusivo", () => {
    const nocturna = {
      ...r,
      horaInicio: "22:00",
      horaFin: "01:00",
      estado: "NO_RETIRADA",
    }
    expect(puedeEntregarseReserva(nocturna, new Date(2026, 2, 3, 0, 30))).toBe(true)
    expect(puedeEntregarseReserva(nocturna, new Date(2026, 2, 3, 1))).toBe(false)
  })
})

describe("conflictos de entrega anticipada", () => {
  const tarde = {
    id: "tarde",
    equipoId: "pc1",
    estado: "CONFIRMADA",
    fecha: "2026-03-02",
    horaInicio: "15:00",
    horaFin: "16:00",
  }
  const previa = { ...tarde, id: "previa", horaInicio: "13:00", horaFin: "14:00" }
  it("incluye reservas y bloqueos intermedios y excluye la propia reserva", () => {
    const ahora = new Date(2026, 2, 2, 12)
    expect(conflictoDeEntregaAnticipada(tarde, [tarde, previa], ahora)).toEqual(previa)
    expect(conflictoDeEntregaAnticipada(tarde, [tarde], ahora)).toBeUndefined()
  })
  it.each(["CANCELADA", "FINALIZADA", "NO_RETIRADA"])("ignora reservas %s", (estado) => {
    expect(
      conflictoDeEntregaAnticipada(
        tarde,
        [{ ...previa, estado }],
        new Date(2026, 2, 2, 12)
      )
    ).toBeUndefined()
  })
  it("permite al terminar la intermedia, otro equipo y reservas posteriores a la devolución", () => {
    expect(
      conflictoDeEntregaAnticipada(tarde, [previa], new Date(2026, 2, 2, 14))
    ).toBeUndefined()
    expect(
      conflictoDeEntregaAnticipada(
        tarde,
        [{ ...previa, equipoId: "otra" }],
        new Date(2026, 2, 2, 12)
      )
    ).toBeUndefined()
    expect(
      conflictoDeEntregaAnticipada(
        tarde,
        [{ ...previa, horaInicio: "16:00", horaFin: "17:00" }],
        new Date(2026, 2, 2, 12)
      )
    ).toBeUndefined()
  })
  it("incluye la madrugada siguiente aunque el retiro ocupe más de 24 horas", () => {
    const noche = { ...tarde, horaInicio: "23:00", horaFin: "07:00" }
    const madrugada = {
      ...previa,
      fecha: "2026-03-03",
      horaInicio: "02:00",
      horaFin: "03:00",
    }
    expect(
      conflictoDeEntregaAnticipada(noche, [madrugada], new Date(2026, 2, 2, 0, 30))
    ).toEqual(madrugada)
    expect(diaSiguienteISO("2026-12-31")).toBe("2027-01-01")
    expect(diaAnteriorISO("2026-03-01")).toBe("2026-02-28")
  })
  it("no cambia el retiro tardío que ya comenzó", () => {
    expect(
      conflictoDeEntregaAnticipada(tarde, [previa], new Date(2026, 2, 2, 15))
    ).toBeUndefined()
  })
})
