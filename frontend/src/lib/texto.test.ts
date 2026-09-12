import { canonizarEspacios, claveDeNombre } from "@/lib/texto"

describe("canonizarEspacios", () => {
  it("recorta los bordes y colapsa lo de adentro", () => {
    expect(canonizarEspacios("  Educación  Física  ")).toBe("Educación Física")
    expect(canonizarEspacios("Ciencias   Sociales:  Geografía")).toBe(
      "Ciencias Sociales: Geografía"
    )
  })

  // El que viaja al copiar desde una página web, y el más difícil de ver: se
  // imprime igual que un espacio común y no lo es.
  it("unifica el espacio duro", () => {
    expect(canonizarEspacios("Educación Física")).toBe("Educación Física")
    expect(canonizarEspacios("Educación  Física")).toBe("Educación Física")
  })

  it("deja en paz un texto que ya está bien", () => {
    expect(canonizarEspacios("Educación Física")).toBe("Educación Física")
  })
})

describe("claveDeNombre", () => {
  // Es la pregunta «¿esta materia ya está cargada?», no «¿se escriben igual?».
  it("da la misma clave para lo que es la misma materia", () => {
    const clave = claveDeNombre("Educación Física")
    for (const variante of [
      "EDUCACION FISICA",
      "educación física",
      "Educación  Física",
      "  Educación   Física  ",
      "Educación Física",
    ]) {
      expect(claveDeNombre(variante)).toBe(clave)
    }
  })

  it("distingue lo que de verdad es otra materia", () => {
    expect(claveDeNombre("Educación Física I")).not.toBe(
      claveDeNombre("Educación Física")
    )
  })
})
