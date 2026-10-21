import { filasACSV, leerCSV } from "@/lib/csv"

describe("filasACSV", () => {
  it("separa con punto y coma y termina las filas con CRLF", () => {
    expect(
      filasACSV([
        ["Equipo", "Reservas"],
        ["PC 7", 3],
      ])
    ).toBe("Equipo;Reservas\r\nPC 7;3")
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

  it.each([
    "=1+1",
    "+SUM(A1:A2)",
    "-1+1",
    "@SUM(A1:A2)",
    "\t=1+1",
    "\r=1+1",
    "\n=1+1",
    "  =1+1",
    "＝1+1",
    "＋1+1",
    "－1+1",
    "＠SUM(A1:A2)",
  ])("exporta como texto el valor que podría ser fórmula: %j", (valor) => {
    const csv = filasACSV([[valor, "columna intacta"]])
    expect(csv.startsWith("\"'")).toBe(true)
    expect(leerCSV(csv)).toEqual([[`'${valor}`, "columna intacta"]])
  })

  it("una fórmula con separadores y comillas no crea otra celda", () => {
    const valor = '=1+1";=1+1'
    expect(leerCSV(filasACSV([[valor, "segunda"]]))).toEqual([[`'${valor}`, "segunda"]])
  })

  it("preserva los valores numéricos negativos como números", () => {
    expect(filasACSV([[-3, "texto normal"]])).toBe("-3;texto normal")
  })
})
