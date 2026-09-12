/**
 * RF-02.12 — la planilla de cursos y materias, en las dos direcciones.
 *
 * El formato es una fila por curso y las materias de ese curso en UNA celda,
 * separadas por comas:
 *
 *     Año;Modalidad;División;Materias
 *     1°;;1;"Lengua, Matemática, Educación Física"
 *
 * Es la forma en que una planilla de horarios ya está escrita, y por eso se
 * eligió: la alternativa —una fila por materia— es correcta y obliga a repetir
 * el curso en cada renglón, que es justo lo que hace que nadie mantenga el
 * archivo.
 *
 * El parseo vive en el navegador y no en el servidor a propósito. Leer un CSV
 * es separador, comillas, BOM y codificación —un problema de presentación— y
 * quien tiene el archivo en la mano es el único que puede mostrar la vista
 * previa antes de mandar nada. La API habla de cursos y materias, no de
 * archivos.
 */

import { leerCSV } from "@/lib/csv"
import { canonizarEspacios, sinTildes } from "@/lib/texto"

import { MAX_ANIO_CURSO, MIN_ANIO_CURSO } from "@/features/academico/types"

/** Un curso con sus materias — la forma que viaja a la API en las dos direcciones. */
export type CursoConMaterias = {
  anio: number
  division?: string
  modalidad?: string
  materias: string[]
}

/** Una fila que no se pudo usar, con el número de línea del archivo. */
export type FilaRechazada = {
  linea: number
  motivo: string
}

export type ResultadoLectura = {
  cursos: CursoConMaterias[]
  rechazadas: FilaRechazada[]
}

/**
 * Los encabezados que se reconocen para cada columna, comparados sin tildes ni
 * mayúsculas. Hay varios por columna porque el mismo dato se llama distinto en
 * cada ámbito: lo que una secundaria llama «División» un terciario lo llama
 * «Comisión» y una universidad «Sección».
 */
const ENCABEZADOS = {
  anio: ["ano", "anio", "año", "curso", "nivel", "grado"],
  modalidad: [
    "modalidad",
    "modalidad / carrera",
    "modalidad/carrera",
    "carrera",
    "orientacion",
    "especialidad",
  ],
  division: ["division", "comision", "seccion", "grupo"],
  materias: ["materias", "materia", "asignaturas", "asignatura", "espacios curriculares"],
} as const

/** Las columnas del archivo, por posición. `-1` = la columna no está. */
type Columnas = { anio: number; modalidad: number; division: number; materias: number }

function ubicarColumnas(encabezado: string[]): Columnas | null {
  const normalizado = encabezado.map((c) => sinTildes(c.trim()))
  const buscar = (nombres: readonly string[]) =>
    normalizado.findIndex((c) => nombres.includes(c))

  const columnas: Columnas = {
    anio: buscar(ENCABEZADOS.anio),
    modalidad: buscar(ENCABEZADOS.modalidad),
    division: buscar(ENCABEZADOS.division),
    materias: buscar(ENCABEZADOS.materias),
  }
  // El año y las materias son las dos que no se pueden adivinar por posición:
  // sin ellas no hay archivo que leer. La división y la modalidad pueden
  // faltar, y de hecho faltan en cualquier institución que no las use.
  return columnas.anio === -1 || columnas.materias === -1 ? null : columnas
}

/**
 * El año llega escrito como lo escribe una persona: «1°», «1', «1º», «Primero
 * no». Se toma el primer número de la celda, que es lo que todas esas formas
 * tienen en común. «Primero» no se traduce: inventar un diccionario de
 * ordinales es adivinar, y adivinar mal acá carga el curso equivocado.
 */
function leerAnio(celda: string): number | null {
  const numero = celda.match(/\d+/)
  if (!numero) return null
  const anio = Number(numero[0])
  return anio >= MIN_ANIO_CURSO && anio <= MAX_ANIO_CURSO ? anio : null
}

/**
 * Las materias de una celda. Se parten por coma y también por punto y coma:
 * quien escribe la lista a mano usa la que tenga más cerca, y un archivo
 * separado por punto y coma obliga a que la lista de adentro use coma.
 */
function leerMaterias(celda: string): string[] {
  return (
    celda
      .split(/[,;\n]/)
      // Canonizadas y no sólo recortadas: una celda de planilla trae dobles
      // espacios que nadie ve, y «Educación  Física» entraría como una materia
      // distinta de «Educación Física».
      .map(canonizarEspacios)
      .filter((m) => m !== "")
  )
}

/**
 * Lee la planilla. Nunca tira: una fila mala se devuelve en `rechazadas` con su
 * número de línea y el resto del archivo se aprovecha igual.
 *
 * Frenar todo por un renglón es lo que convierte un archivo de treinta cursos
 * en media hora de prueba y error; con las rechazadas a la vista, se corrigen
 * las tres que fallaron y se vuelve a subir, que no duplica nada.
 */
export function leerEstructuraCSV(texto: string): ResultadoLectura {
  const filas = leerCSV(texto)
  if (filas.length === 0) return { cursos: [], rechazadas: [] }

  const columnas = ubicarColumnas(filas[0])
  if (!columnas) {
    return {
      cursos: [],
      rechazadas: [
        {
          linea: 1,
          motivo:
            "No se reconoció el encabezado. La primera fila tiene que nombrar al menos las columnas «Año» y «Materias».",
        },
      ],
    }
  }

  const cursos: CursoConMaterias[] = []
  const rechazadas: FilaRechazada[] = []

  for (let i = 1; i < filas.length; i++) {
    const fila = filas[i]
    // +1 porque la fila 0 es el encabezado y quien mira el archivo cuenta
    // desde 1.
    const linea = i + 1
    const celda = (indice: number) => (indice === -1 ? "" : (fila[indice] ?? "").trim())

    const anio = leerAnio(celda(columnas.anio))
    if (anio === null) {
      rechazadas.push({
        linea,
        motivo: `«${celda(columnas.anio)}» no es un año entre ${MIN_ANIO_CURSO} y ${MAX_ANIO_CURSO}.`,
      })
      continue
    }

    const materias = leerMaterias(celda(columnas.materias))
    const division = celda(columnas.division)
    const modalidad = celda(columnas.modalidad)

    cursos.push({
      anio,
      // Ausentes y no cadena vacía: en la base son NULL, que es lo que
      // significa «este curso no tiene ese dato».
      ...(division ? { division } : {}),
      ...(modalidad ? { modalidad } : {}),
      materias,
    })
  }

  return { cursos, rechazadas }
}

/** El encabezado que se escribe al descargar. */
const ENCABEZADO_SALIDA = ["Año", "Modalidad / Carrera", "División", "Materias"]

/**
 * La planilla lista para descargar, en filas.
 *
 * Sale con el MISMO formato que entra, y eso es el punto: lo que se baja se
 * corrige en una planilla y se vuelve a subir, o se sube sobre el ciclo del año
 * siguiente. Un formato de salida más lindo que no se pueda volver a cargar
 * convierte la descarga en una foto.
 */
export function estructuraAFilas(cursos: CursoConMaterias[]): string[][] {
  return [
    ENCABEZADO_SALIDA,
    ...cursos.map((c) => [
      `${c.anio}°`,
      c.modalidad ?? "",
      c.division ?? "",
      // Coma y espacio: el separador del archivo es el punto y coma, así que
      // la lista de adentro puede usar coma sin entrecomillar de más.
      c.materias.join(", "),
    ]),
  ]
}

/** «Cursos y materias 2026.csv» — el año en el nombre, para no pisar el anterior. */
export function nombreDeArchivo(anio: number): string {
  return `cursos-y-materias-${anio}.csv`
}
