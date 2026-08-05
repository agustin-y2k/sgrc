import type {
  CicloLectivo,
  Curso,
  DocenteMateria,
  Materia,
  RespuestaLista,
  ResultadoArchivado,
  RolDocente,
} from "@/features/academico/types"
import { apiFetch } from "@/lib/api-client"

// ── Ciclo lectivo (RF-02.1) ───────────────────────────────────────────

export function listarCiclos() {
  return apiFetch<RespuestaLista<CicloLectivo>>("/api/academic/ciclos")
}

/** Solo puede haber un ciclo activo: el backend rechaza con 409 si ya hay otro. */
export function crearCiclo(anio: number) {
  return apiFetch<CicloLectivo>("/api/academic/ciclos", {
    method: "POST",
    body: { anio },
  })
}

// ── Curso (RF-02.2, RF-02.11) ─────────────────────────────────────────

export function listarCursos(cicloId: string) {
  return apiFetch<RespuestaLista<Curso>>(`/api/academic/ciclos/${cicloId}/cursos`)
}

export function crearCurso(cicloId: string, nombre: string) {
  return apiFetch<Curso>(`/api/academic/ciclos/${cicloId}/cursos`, {
    method: "POST",
    body: { nombre },
  })
}

export function editarCurso(id: string, nombre: string) {
  return apiFetch<void>(`/api/academic/cursos/${id}`, {
    method: "PATCH",
    body: { nombre },
  })
}

/** RF-02.11 — el backend rechaza con 409 si el curso tiene reservas. */
export function eliminarCurso(id: string) {
  return apiFetch<void>(`/api/academic/cursos/${id}`, { method: "DELETE" })
}

// ── Materia (RF-02.3, RF-02.11) ───────────────────────────────────────

export function listarMaterias(cursoId: string) {
  return apiFetch<RespuestaLista<Materia>>(`/api/academic/cursos/${cursoId}/materias`)
}

export function crearMateria(cursoId: string, nombre: string) {
  return apiFetch<Materia>(`/api/academic/cursos/${cursoId}/materias`, {
    method: "POST",
    body: { nombre },
  })
}

export function editarMateria(id: string, nombre: string) {
  return apiFetch<void>(`/api/academic/materias/${id}`, {
    method: "PATCH",
    body: { nombre },
  })
}

/** RF-02.11 — el backend rechaza con 409 si la materia tiene reservas. */
export function eliminarMateria(id: string) {
  return apiFetch<void>(`/api/academic/materias/${id}`, { method: "DELETE" })
}

// ── DocenteMateria (RF-02.6, RF-02.10) ────────────────────────────────

export function listarDocentesDeMateria(materiaId: string) {
  return apiFetch<RespuestaLista<DocenteMateria>>(
    `/api/academic/materias/${materiaId}/docentes`
  )
}

/** Solo se puede asignar a una cuenta APROBADA; el backend lo valida. */
export function asignarDocente(materiaId: string, usuarioId: string, rol: RolDocente) {
  return apiFetch<DocenteMateria>(`/api/academic/materias/${materiaId}/docentes`, {
    method: "POST",
    body: { usuarioId, rol },
  })
}

/**
 * RF-02.10 — si la materia queda sin ningún docente, el backend cancela
 * sus reservas futuras y avisa a todos los Admin. Si queda otro asignado,
 * no cancela nada.
 */
export function removerDocenteMateria(materiaId: string, docenteMateriaId: string) {
  return apiFetch<{ reservasCanceladas: number }>(
    `/api/academic/materias/${materiaId}/docentes/${docenteMateriaId}`,
    { method: "DELETE" }
  )
}

// ── Archivado y clonado (RF-02.4, RF-02.5) ────────────────────────────

/**
 * Archiva el ciclo y, si se pasa `clonarA`, crea el del año siguiente
 * copiando cursos y materias sin las asignaciones de docentes.
 *
 * Es destructivo e irreversible: elimina físicamente todas las reservas
 * del ciclo, después de guardar el snapshot histórico agregado.
 */
export function archivarCiclo(cicloId: string, clonarA?: number) {
  return apiFetch<ResultadoArchivado>(`/api/academic/ciclos/${cicloId}/archivar`, {
    method: "POST",
    body: clonarA !== undefined ? { clonarA } : {},
  })
}
