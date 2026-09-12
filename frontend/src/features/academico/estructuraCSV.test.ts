import {
  estructuraAFilas,
  leerEstructuraCSV,
  nombreDeArchivo,
} from "@/features/academico/estructuraCSV"

describe("leerEstructuraCSV", () => {
  it("lee una fila por curso con las materias en una celda", () => {
    const { cursos, rechazadas } = leerEstructuraCSV(
      'Año,Modalidad / Carrera,División,Materias\n1°,,1,"Lengua, Matemática, Educación Física"'
    )

    expect(rechazadas).toEqual([])
    expect(cursos).toEqual([
      {
        anio: 1,
        division: "1",
        materias: ["Lengua", "Matemática", "Educación Física"],
      },
    ])
  })

  // El caso que rompe cualquier split ingenuo: la celda de materias tiene más
  // comas que el resto de la fila junta, así que contarlas sin mirar las
  // comillas elige el separador equivocado.
  it("no confunde las comas de adentro de la celda con el separador", () => {
    const { cursos } = leerEstructuraCSV(
      'Año;Modalidad;División;Materias\n4°;Electromecánica;1;"Física, Química, Matemática"'
    )

    expect(cursos).toHaveLength(1)
    expect(cursos[0].modalidad).toBe("Electromecánica")
    expect(cursos[0].materias).toEqual(["Física", "Química", "Matemática"])
  })

  it("acepta tabulación, que es lo que sale de copiar una tabla", () => {
    const { cursos } = leerEstructuraCSV(
      "Año\tModalidad\tDivisión\tMaterias\n2°\t\t3\tLengua"
    )

    expect(cursos).toEqual([{ anio: 2, division: "3", materias: ["Lengua"] }])
  })

  // El BOM de Excel se pega al primer encabezado y lo deja sin coincidir con
  // ninguna columna conocida: el archivo entero se lee como ilegible.
  it("ignora el BOM que escribe Excel", () => {
    const { cursos, rechazadas } = leerEstructuraCSV(
      "﻿Año,División,Materias\n1°,1,Lengua"
    )

    expect(rechazadas).toEqual([])
    expect(cursos).toHaveLength(1)
  })

  it("reconoce los encabezados sin tildes y en cualquier mayúscula", () => {
    const { cursos } = leerEstructuraCSV("ANO,DIVISION,MATERIAS\n3°,2,Física")

    expect(cursos).toEqual([{ anio: 3, division: "2", materias: ["Física"] }])
  })

  // La división y la modalidad son opcionales (RF-02.2): una universidad no
  // divide sus cursos y una primaria no tiene modalidades.
  it("deja ausentes la división y la modalidad cuando la celda está vacía", () => {
    const { cursos } = leerEstructuraCSV('Año,Modalidad,División,Materias\n1°,,,"Lengua"')

    expect(cursos[0]).not.toHaveProperty("division")
    expect(cursos[0]).not.toHaveProperty("modalidad")
  })

  it("acepta un archivo sin columna de división ni de modalidad", () => {
    const { cursos, rechazadas } = leerEstructuraCSV("Año,Materias\n1°,Lengua")

    expect(rechazadas).toEqual([])
    expect(cursos).toEqual([{ anio: 1, materias: ["Lengua"] }])
  })

  it("toma el número del año escrito como sea", () => {
    const { cursos } = leerEstructuraCSV(
      "Año,Materias\n1°,Lengua\n2º,Lengua\n3,Lengua\n 4° ,Lengua"
    )

    expect(cursos.map((c) => c.anio)).toEqual([1, 2, 3, 4])
  })

  // Una fila mala no puede frenar las otras treinta: con las rechazadas a la
  // vista se corrigen esas y se vuelve a subir, que no duplica nada.
  it("aparta las filas ilegibles y aprovecha el resto", () => {
    const { cursos, rechazadas } = leerEstructuraCSV(
      "Año,Materias\n1°,Lengua\nPrimero,Lengua\n2°,Matemática"
    )

    expect(cursos.map((c) => c.anio)).toEqual([1, 2])
    expect(rechazadas).toHaveLength(1)
    // El número de línea es lo único que permite encontrar el renglón malo en
    // una planilla de cien.
    expect(rechazadas[0].linea).toBe(3)
  })

  it("rechaza el año fuera del rango que acepta el dominio", () => {
    const { cursos, rechazadas } = leerEstructuraCSV("Año,Materias\n99°,Lengua")

    expect(cursos).toEqual([])
    expect(rechazadas[0].motivo).toMatch(/no es un año/)
  })

  it("avisa cuando el encabezado no se reconoce", () => {
    const { cursos, rechazadas } = leerEstructuraCSV("Nombre;Apellido\nJuan;Pérez")

    expect(cursos).toEqual([])
    expect(rechazadas[0].motivo).toMatch(/encabezado/)
  })

  it("no devuelve nada para un archivo vacío", () => {
    expect(leerEstructuraCSV("")).toEqual({ cursos: [], rechazadas: [] })
  })

  it("no inventa un curso con la fila en blanco del final", () => {
    const { cursos } = leerEstructuraCSV("Año,Materias\r\n1°,Lengua\r\n")

    expect(cursos).toHaveLength(1)
  })

  it("admite un curso sin ninguna materia", () => {
    const { cursos, rechazadas } = leerEstructuraCSV("Año,División,Materias\n1°,2,")

    expect(rechazadas).toEqual([])
    expect(cursos).toEqual([{ anio: 1, division: "2", materias: [] }])
  })
})

describe("estructuraAFilas", () => {
  it("escribe el mismo formato que se lee", () => {
    const filas = estructuraAFilas([
      {
        anio: 4,
        division: "1",
        modalidad: "Electromecánica",
        materias: ["Física", "Química"],
      },
    ])

    expect(filas[0]).toEqual(["Año", "Modalidad / Carrera", "División", "Materias"])
    expect(filas[1]).toEqual(["4°", "Electromecánica", "1", "Física, Química"])
  })

  // Que lo que baja se pueda volver a subir es el punto: sin eso la descarga es
  // una foto y no sirve para armar el año siguiente.
  it("lo que escribe se vuelve a leer igual", () => {
    const original = [
      { anio: 1, division: "1", materias: ["Lengua", "Matemática"] },
      {
        anio: 4,
        division: "2",
        modalidad: "Construcciones",
        materias: ["Dibujo Técnico"],
      },
      { anio: 5, materias: [] },
    ]

    const csv = estructuraAFilas(original)
      .map((fila) =>
        fila
          .map((c) => (/[";,\r\n]/.test(c) ? `"${c.replace(/"/g, '""')}"` : c))
          .join(";")
      )
      .join("\r\n")

    expect(leerEstructuraCSV(csv).cursos).toEqual(original)
  })
})

describe("nombreDeArchivo", () => {
  it("lleva el año, para no pisar el del ciclo anterior", () => {
    expect(nombreDeArchivo(2026)).toBe("cursos-y-materias-2026.csv")
  })
})
