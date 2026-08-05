// Espeja los DTOs de internal/academic/interfaces/http/dto.go.

export type RespuestaLista<T> = { data: T[] }

export type CicloLectivo = {
  id: string
  anio: number
  activo: boolean
  archivado: boolean
}

export type Curso = {
  id: string
  cicloLectivoId: string
  nombre: string
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
 * Ojo: el backend devuelve solo `usuarioId`, sin nombre ni apellido — el
 * DTO de academic no consulta la tabla de auth (ver el comentario en
 * internal/academic/interfaces/http/dto.go). El nombre se resuelve en el
 * cliente cruzando contra GET /api/auth/usuarios.
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
 * RF-02.2: el nombre de un curso no es libre — año (1° a 6°) más división
 * (A a Z). El backend lo valida con el mismo patrón, tanto en la capa de
 * aplicación como en un CHECK de Postgres; acá se repite para avisar antes
 * de mandar el request.
 */
export const PATRON_NOMBRE_CURSO = /^[1-6]°[A-Z]$/

export function esNombreDeCursoValido(nombre: string): boolean {
  return PATRON_NOMBRE_CURSO.test(nombre)
}

/** Los años y divisiones que se pueden elegir, para armar los selectores. */
export const ANIOS_DE_CURSO = ["1", "2", "3", "4", "5", "6"] as const
export const DIVISIONES_DE_CURSO = "ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("")

/**
 * Arma el nombre canónico a partir del año y la división.
 *
 * El `°` lo pone el sistema, no la persona: escribirlo a mano obliga a
 * buscar un símbolo que no está en el teclado (y que se confunde con `º`,
 * el ordinal masculino, que NO es el mismo carácter y hacía fallar la
 * validación sin que se notara la diferencia en pantalla).
 */
export function componerNombreDeCurso(anio: string, division: string): string {
  return `${anio}°${division.toUpperCase()}`
}

/**
 * El inverso: separa un nombre existente para poder editarlo con los mismos
 * selectores. Si no matchea el patrón —una fila vieja, o cargada por API—
 * devuelve null y la pantalla cae al campo de texto libre.
 */
export function separarNombreDeCurso(
  nombre: string
): { anio: string; division: string } | null {
  const m = /^([1-6])°([A-Z])$/.exec(nombre.trim().toUpperCase())
  return m ? { anio: m[1], division: m[2] } : null
}
