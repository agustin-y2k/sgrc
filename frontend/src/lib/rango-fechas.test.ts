import { errorRangoFechas } from "./rango-fechas"

it.each([
  ["", ""],
  ["2026-08-05", ""],
  ["", "2026-08-05"],
  ["2024-02-29", "2024-02-29"],
  ["2026-03-08", "2027-03-09"],
])("acepta límites opcionales y días civiles válidos: %s a %s", (desde, hasta) => {
  expect(errorRangoFechas(desde, hasta, 366)).toBe("")
})

it.each(["2026-02-29", "2026-04-31", "fecha", "20260-01-01"])(
  "rechaza una fecha que la API no admite: %s",
  (fecha) => {
    expect(errorRangoFechas(fecha)).not.toBe("")
  }
)

it("la auditoría tiene límite de intervalo y los reportes permiten períodos mayores", () => {
  expect(errorRangoFechas("2026-01-01", "2025-12-31")).toMatch(/posterior/)
  expect(errorRangoFechas("2026-01-01", "2027-01-03", 366)).toMatch(/366 días/)
  expect(errorRangoFechas("2026-01-01", "2027-01-03")).toBe("")
})
