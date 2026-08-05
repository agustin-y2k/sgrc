import { filasACSV } from "@/lib/csv"

describe("filasACSV", () => {
  it("separa con punto y coma y termina las filas con CRLF", () => {
    expect(
      filasACSV([
        ["PC", "Reservas"],
        ["PC 7", 3],
      ])
    ).toBe("PC;Reservas\r\nPC 7;3")
  })

  // El caso que rompe un CSV armado a mano: un nombre de materia con punto
  // y coma partiría la fila en dos columnas.
  it("entrecomilla lo que lleva el separador adentro", () => {
    expect(filasACSV([["Lengua; Literatura", 1]])).toBe('"Lengua; Literatura";1')
  })

  it("duplica las comillas de adentro", () => {
    expect(filasACSV([['El carro "nuevo"']])).toBe('"El carro ""nuevo"""')
  })

  it("no entrecomilla lo que no hace falta", () => {
    expect(filasACSV([["Matemática", 42]])).toBe("Matemática;42")
  })

  it("entrecomilla los saltos de línea", () => {
    expect(filasACSV([["no arranca\nni con el cargador"]])).toBe(
      '"no arranca\nni con el cargador"'
    )
  })
})
