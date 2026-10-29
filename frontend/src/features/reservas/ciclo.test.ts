import { errorDeCicloLectivo } from "./ciclo"

test.each([
  ["2026-12-31", "2026-12-31", "23:00", "00:00", undefined, false],
  ["2026-12-31", "2026-12-31", "23:00", "01:00", undefined, true],
  ["2027-01-01", "2027-01-01", "08:00", "09:00", undefined, true],
  ["2025-12-31", "2025-12-31", "08:00", "09:00", undefined, true],
  ["2026-12-23", "2026-12-31", "23:00", "01:00", "MIERCOLES", false],
  ["2026-12-23", "2026-12-31", "23:00", "01:00", "JUEVES", true],
  ["2026-12-28", "2027-01-01", "08:00", "09:00", "LUNES", true],
] as const)(
  "el ciclo valida %s a %s, %s–%s, %s",
  (desde, hasta, inicio, fin, dia, invalido) => {
    const error = errorDeCicloLectivo(2026, desde, hasta, inicio, fin, dia)
    expect(!!error).toBe(invalido)
    if (error) expect(error).toContain("ciclo 2026")
  }
)
