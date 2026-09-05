// Espeja los DTOs de internal/academic/interfaces/http/dto.go.

export type RespuestaLista<T> = { data: T[] }

export type CicloLectivo = {
  id: string
  anio: number
  activo: boolean
  archivado: boolean
}

/**
 * RF-02.2 — un curso es un AÑO más dos datos opcionales. El año lo tienen
 * todos los ámbitos —primaria, secundaria, terciario, universidad—; la
 * división y la modalidad no: una universidad no divide sus cursos y una
 * primaria no tiene modalidades.
 */
export type Curso = {
  id: string
  cicloLectivoId: string
  /** De sólo lectura: lo calcula la base con el año y la división ("4°2", "2°"). */
  nombre: string
  anio: number
  /** "A", "2", "1ra". Ausente = la institución no divide sus cursos. */
  division?: string
  /** Modalidad, orientación o carrera. Ausente = este curso no está en ninguna. */
  modalidad?: string
  activo: boolean
  archivado: boolean
}

export type Materia = {
  id: string
  cursoId: string
  nombre: string
  activo: boolean
  archivado: boolean
}

/** RF-02.6 — el rol es informativo, no cambia permisos. */
export type RolDocente = "TITULAR" | "SUPLENTE"

/**
 * Ojo: el backend devuelve solo `usuarioId`, sin nombre ni apellido — el DTO
 * de academic no consulta la tabla de auth (ver el comentario en
 * internal/academic/interfaces/http/dto.go).
 */
export type DocenteMateria = {
  id: string
  usuarioId: string
  rol: RolDocente
}

export type ResultadoArchivado = {
  archivado: boolean
  nuevoCicloId?: string
  cursosClonados: number
  materiasClonadas: number
}

/**
 * RF-02.6 — una asignación docente-materia con los nombres de las dos puntas
 * ya resueltos. Es la relación mirada para MOSTRARLA; `DocenteMateria` es la
 * misma fila para administrarla.
 */
export type AsignacionDocente = {
  id: string
  usuarioId: string
  /** "Nombre Apellido". */
  docenteNombre: string
  rol: RolDocente
  materiaId: string
  materiaNombre: string
  cursoId: string
  cursoNombre: string
  cursoModalidad?: string
}

/** Lo que entra en cada campo, igual que los topes del dominio. */
export const MAX_LARGO_DIVISION = 12
export const MAX_LARGO_MODALIDAD = 80
export const MIN_ANIO_CURSO = 1
export const MAX_ANIO_CURSO = 15

/**
 * El nombre de un curso: año + grado + división ("4°2", "2°", "1°A"). Es la
 * misma expresión que calcula la columna generada `curso.nombre`, y existe de
 * este lado para poder mostrar cómo va a quedar antes de guardarlo.
 */
export function componerNombreDeCurso(anio: number | "", division: string): string {
  return `${anio === "" ? "" : anio}°${division.trim()}`
}

/**
 * Cómo se nombra un curso cuando hay que distinguirlo de otro: "1°A ·
 * Enfermería". El nombre puede repetirse entre dos carreras, así que donde se
 * elige o se busca un curso hace falta la modalidad al lado.
 *
 * Mismo criterio que nombreDeEquipo() en el mostrador, donde "PC 1" hay una
 * por carro.
 */
export function etiquetaDeCurso(curso: { nombre: string; modalidad?: string }): string {
  return curso.modalidad ? `${curso.nombre} · ${curso.modalidad}` : curso.nombre
}

/**
 * Adapta un listado que trae el curso resuelto por JOIN —una materia
 * reservable, una asignación docente— a lo que espera etiquetaDeCurso.
 */
export function cursoDe(fila: { cursoNombre: string; cursoModalidad?: string }): {
  nombre: string
  modalidad?: string
} {
  return { nombre: fila.cursoNombre, modalidad: fila.cursoModalidad }
}

/**
 * Las divisiones y las modalidades que la institución ya usa, sin repetir. Son
 * lo que se ofrece como sugerencia: nadie tiene que acordarse de si acá se
 * escribió "Electromecánica" o "Electromecanica", que serían dos.
 */
export function divisionesEnUso(cursos: { division?: string }[]): string[] {
  return sinRepetir(cursos.map((c) => c.division))
}

export function modalidadesEnUso(cursos: { modalidad?: string }[]): string[] {
  return sinRepetir(cursos.map((c) => c.modalidad))
}

function sinRepetir(valores: (string | undefined)[]): string[] {
  const vistos = new Set<string>()
  for (const v of valores) {
    if (v) vistos.add(v)
  }
  return [...vistos].sort((a, b) => a.localeCompare(b, "es"))
}
