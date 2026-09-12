// Espeja los DTOs de internal/academic/interfaces/http/dto.go.

import { sinTildes } from "@/lib/texto"

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
  /**
   * Único estado de un curso: se enciende al archivar su ciclo (RF-02.4).
   * Tenía al lado un `activo` que el servidor mandaba siempre en true y que no
   * decidía nada; se quitó con la migración 016.
   */
  archivado: boolean
}

export type Materia = {
  id: string
  cursoId: string
  nombre: string
  /** Ver Curso.archivado. */
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
 * RF-02.12 — lo creado y lo que ya estaba, separado. Son dos noticias distintas
 * para quien acaba de subir un archivo: una carga que no creó nada no falló, y
 * decirlo evita el segundo intento.
 */
export type ResultadoImportacion = {
  cursosCreados: number
  cursosExistentes: number
  materiasCreadas: number
  materiasExistentes: number
}

export type ResultadoCopia = {
  materiasCreadas: number
  materiasExistentes: number
  cursosDestino: number
}

/**
 * Lo que deja atrás renombrar una materia: las marcas de preferencia de equipo
 * (RF-03.21) se guardan por NOMBRE de materia, así que al renombrarla dejan de
 * cruzar con nada y quedan huérfanas.
 *
 * Cero es la respuesta normal y no se muestra. Un número mayor es una
 * consecuencia que el Admin no pidió y no ve desde acá —las marcas viven en
 * Inventario—, así que hay que contarla en el momento: descubrirla más tarde
 * es descubrir que el orden de equipos cambió sin motivo aparente.
 */
export type ResultadoEdicionDeMateria = {
  marcasDeEquipoAfectadas: number
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
 * Los nombres de curso que aparecen más de una vez en el ciclo, comparados sin
 * tildes ni mayúsculas.
 *
 * Un nombre repetido sólo puede venir de dos modalidades distintas —el índice
 * único lo garantiza dentro de una— y es el único caso en el que el nombre
 * solo no alcanza para saber de qué curso se habla.
 */
export function nombresRepetidos(cursos: { nombre: string }[]): Set<string> {
  const vistos = new Set<string>()
  const repetidos = new Set<string>()
  for (const c of cursos) {
    const clave = sinTildes(c.nombre)
    if (vistos.has(clave)) repetidos.add(clave)
    else vistos.add(clave)
  }
  return repetidos
}

/**
 * Lo más corto que alcanza para saber de qué curso se habla: el nombre solo, y
 * la modalidad al lado únicamente cuando ese nombre se repite.
 *
 * Es lo que se guarda como destino de una entrega (RF-08.26). La ambigüedad es
 * un hecho de los datos y no una propiedad del sistema: mostrar la modalidad
 * cuando no hace falta es tan incorrecto como esconderla cuando sí. En una
 * escuela que numera sus divisiones de corrido —4°1 es Construcción y 4°2
 * Electromecánica— el nombre ya identifica, y agregarle la modalidad sólo
 * alarga el registro.
 */
export function etiquetaCortaDeCurso(
  curso: { nombre: string; modalidad?: string },
  repetidos: Set<string>
): string {
  return repetidos.has(sinTildes(curso.nombre)) ? etiquetaDeCurso(curso) : curso.nombre
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
