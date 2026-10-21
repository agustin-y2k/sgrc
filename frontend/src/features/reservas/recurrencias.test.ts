import { calcularRecurrencia } from "@/features/reservas/recurrencias"

describe("calendario de una reserva semanal", () => {
  it("consulta el primer lunes de la serie aunque el rango empiece un martes", () => {
    expect(calcularRecurrencia("2026-05-05", "2026-05-18", "LUNES")).toEqual({
      primeraFecha: "2026-05-11",
      cantidad: 2,
    })
  })
  it("incluye ambos extremos y detecta las series de 45 y 46 clases", () => {
    expect(calcularRecurrencia("2026-01-05", "2026-11-09", "LUNES")?.cantidad).toBe(45)
    expect(calcularRecurrencia("2026-01-05", "2026-11-16", "LUNES")?.cantidad).toBe(46)
  })
  it("no inventa una ocurrencia cuando no cae el día elegido", () => {
    expect(calcularRecurrencia("2026-05-05", "2026-05-08", "LUNES")).toEqual({
      primeraFecha: null,
      cantidad: 0,
    })
  })
  it("cuenta sin depender del cambio de horario y rechaza un rango invertido", () => {
    expect(calcularRecurrencia("2026-03-01", "2026-03-29", "DOMINGO")?.cantidad).toBe(5)
    expect(calcularRecurrencia("2026-05-11", "2026-05-04", "LUNES")?.cantidad).toBe(0)
    expect(calcularRecurrencia("", "2026-05-04", "LUNES")).toBeNull()
  })
})
