import type { CursoConMaterias } from "@/features/academico/estructuraCSV"
import type {
  AsignacionDocente,
  CicloLectivo,
  Curso,
  DocenteMateria,
  Materia,
  RespuestaLista,
  ResultadoArchivado,
  ResultadoCopia,
  ResultadoImportacion,
  RolDocente,
} from "@/features/academico/types"
import type { MateriaReservable } from "@/features/reservas/types"
import { apiFetch } from "@/lib/api-client"

// ── Ciclo lectivo (RF-02.1) ───────────────────────────────────────────

export function listarCiclos() {
  return apiFetch<RespuestaLista<CicloLectivo>>("/api/ciclos")
}

/** Solo puede haber un ciclo activo: el backend rechaza con 409 si ya hay otro. */
export function crearCiclo(anio: number) {
  return apiFetch<CicloLectivo>("/api/ciclos", {
    method: "POST",
    body: { anio },
  })
}

// ── Curso (RF-02.2, RF-02.11) ─────────────────────────────────────────

/**
 * Corregir el año de un ciclo creado con el año equivocado.
 *
 * Falla con 409 si el ciclo ya tiene reservas —sus fechas quedarían en el año
 * viejo—, si el año de destino ya tiene bloqueos administrativos —se atribuyen
 * a un ciclo por el año de su fecha—, si ese año ya lo tiene otro ciclo, o si
 * el ciclo está archivado.
 */
export function corregirCiclo(cicloId: string, anio: number) {
  return apiFetch<void>(`/api/ciclos/${cicloId}`, {
    method: "PATCH",
    body: { anio },
  })
}

/** Eliminar un ciclo vacío. Uno con cursos cargados se archiva, no se borra. */
export function eliminarCiclo(cicloId: string) {
  return apiFetch<void>(`/api/ciclos/${cicloId}`, { method: "DELETE" })
}

export function listarCursos(cicloId: string) {
  return apiFetch<RespuestaLista<Curso>>(`/api/ciclos/${cicloId}/cursos`)
}

/**
 * Quién dicta qué en un ciclo, con los nombres resueltos y en una sola
 * consulta (solo Admin). Lo usan las dos pantallas que muestran la relación:
 * las materias de un curso y el listado de usuarios.
 */
export function listarAsignaciones(cicloId: string) {
  return apiFetch<RespuestaLista<AsignacionDocente>>(
    `/api/ciclos/${cicloId}/asignaciones`
  )
}

/**
 * En qué materias está asignada OTRA persona (solo Admin). Lo pregunta el
 * mostrador: cuando quien viene a buscar un equipo tiene cuenta, sus cursos
 * son el destino más probable de esa entrega (RF-08.23).
 */
export function materiasDeDocente(usuarioId: string) {
  return apiFetch<RespuestaLista<MateriaReservable>>(
    `/api/docentes/${usuarioId}/materias`
  )
}

/**
 * Los tres datos del curso (RF-02.2): el año y los dos opcionales. El nombre
 * no viaja — lo calcula la base con el año y la división.
 */
export type DatosDeCursoAPI = {
  anio: number
  division?: string
  modalidad?: string
}

export function crearCurso(cicloId: string, datos: DatosDeCursoAPI) {
  return apiFetch<Curso>(`/api/ciclos/${cicloId}/cursos`, {
    method: "POST",
    body: datos,
  })
}

export function editarCurso(id: string, datos: DatosDeCursoAPI) {
  return apiFetch<void>(`/api/cursos/${id}`, {
    method: "PATCH",
    body: datos,
  })
}

/** RF-02.11 — el backend rechaza con 409 si el curso tiene reservas. */
export function eliminarCurso(id: string) {
  return apiFetch<void>(`/api/cursos/${id}`, { method: "DELETE" })
}

// ── Materia (RF-02.3, RF-02.11) ───────────────────────────────────────

export function listarMaterias(cursoId: string) {
  return apiFetch<RespuestaLista<Materia>>(`/api/cursos/${cursoId}/materias`)
}

export function crearMateria(cursoId: string, nombre: string) {
  return apiFetch<Materia>(`/api/cursos/${cursoId}/materias`, {
    method: "POST",
    body: { nombre },
  })
}

export function editarMateria(id: string, nombre: string) {
  return apiFetch<void>(`/api/materias/${id}`, {
    method: "PATCH",
    body: { nombre },
  })
}

/** RF-02.11 — el backend rechaza con 409 si la materia tiene reservas. */
export function eliminarMateria(id: string) {
  return apiFetch<void>(`/api/materias/${id}`, { method: "DELETE" })
}

// ── DocenteMateria (RF-02.6, RF-02.10) ────────────────────────────────

export function listarDocentesDeMateria(materiaId: string) {
  return apiFetch<RespuestaLista<DocenteMateria>>(
    `/api/materias/${materiaId}/docentes`
  )
}

/** Solo se puede asignar a una cuenta APROBADA; el backend lo valida. */
export function asignarDocente(materiaId: string, usuarioId: string, rol: RolDocente) {
  return apiFetch<DocenteMateria>(`/api/materias/${materiaId}/docentes`, {
    method: "POST",
    body: { usuarioId, rol },
  })
}

/** Corrige el rol de un vínculo que ya existe. */
export function cambiarRolDocente(
  materiaId: string,
  docenteMateriaId: string,
  rol: RolDocente
) {
  return apiFetch<DocenteMateria>(
    `/api/materias/${materiaId}/docentes/${docenteMateriaId}`,
    { method: "PATCH", body: { rol } }
  )
}

/**
 * RF-02.10 — si la materia queda sin ningún docente, el backend cancela sus
 * reservas futuras y avisa a todos los Admin.
 */
export function removerDocenteMateria(materiaId: string, docenteMateriaId: string) {
  return apiFetch<{ reservasCanceladas: number }>(
    `/api/materias/${materiaId}/docentes/${docenteMateriaId}`,
    { method: "DELETE" }
  )
}

// ── Archivado y clonado (RF-02.4, RF-02.5) ────────────────────────────

/**
 * Archiva el ciclo y, si se pasa `clonarA`, crea el del año siguiente
 * copiando cursos y materias sin las asignaciones de docentes.
 */
export function archivarCiclo(cicloId: string, clonarA?: number) {
  return apiFetch<ResultadoArchivado>(`/api/ciclos/${cicloId}/archivar`, {
    method: "POST",
    body: clonarA !== undefined ? { clonarA } : {},
  })
}

// ── Carga y descarga de la estructura (RF-02.12) ──────────────────────

/**
 * Los cursos del ciclo con sus materias, en una sola consulta. Es exactamente
 * lo que acepta `importarEstructura`: lo que se descarga se vuelve a cargar sin
 * traducción.
 */
export function exportarEstructura(cicloId: string) {
  return apiFetch<{ cursos: CursoConMaterias[] }>(
    `/api/ciclos/${cicloId}/estructura`
  )
}

/**
 * Crea los cursos y materias que falten. NUNCA borra ni renombra: subir dos
 * veces el mismo archivo es inofensivo, y un archivo al que le falta algo que
 * se agregó a mano no lo hace desaparecer.
 */
export function importarEstructura(cicloId: string, cursos: CursoConMaterias[]) {
  return apiFetch<ResultadoImportacion>(`/api/ciclos/${cicloId}/estructura`, {
    method: "POST",
    body: { cursos },
  })
}

/** Copia los nombres de materia de un curso a otros del mismo ciclo. */
export function copiarMaterias(cursoOrigenId: string, cursosDestinoIds: string[]) {
  return apiFetch<ResultadoCopia>(
    `/api/cursos/${cursoOrigenId}/copiar-materias`,
    { method: "POST", body: { cursosDestinoIds } }
  )
}
