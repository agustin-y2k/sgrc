// Package http expone las rutas Fiber de academic — ver docs/08-api-spec.yaml
// para el contrato completo de cada endpoint.
package http

import (
	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// ── Requests ────────────────────────────────────────────────────────────

type crearCicloRequest struct {
	Anio int `json:"anio"`
}

// corregirCicloRequest: el año es lo único corregible de un ciclo. Lo demás
// —activo, archivado— no son datos que se editen sino el resultado de
// archivarlo.
type corregirCicloRequest struct {
	Anio int `json:"anio"`
}

type archivarCicloRequest struct {
	ClonarA *int `json:"clonarA,omitempty"`
}

// crearCursoRequest y editarCursoRequest llevan los tres datos del curso: el
// año —lo único obligatorio— y los dos opcionales. El NOMBRE no viaja: lo
// calcula la base a partir del año y la división (RF-02.2).
type crearCursoRequest struct {
	Anio      int    `json:"anio"`
	Division  string `json:"division,omitempty"`
	Modalidad string `json:"modalidad,omitempty"`
}

type editarCursoRequest struct {
	Anio      int    `json:"anio"`
	Division  string `json:"division,omitempty"`
	Modalidad string `json:"modalidad,omitempty"`
}

type crearMateriaRequest struct {
	Nombre string `json:"nombre"`
}

type editarMateriaRequest struct {
	Nombre string `json:"nombre"`
}

// importarEstructuraRequest es la carga masiva de RF-02.12. Lleva la estructura
// ya parseada y NO el archivo: leer un CSV —separador, comillas, BOM, el acento
// de la codificación— es un problema de presentación, y el cliente que lo
// resuelve es el que tiene el archivo en la mano y puede mostrar la vista previa
// antes de mandar nada. La API acepta y devuelve la misma forma, así que lo que
// se descarga se vuelve a cargar sin traducción.
type importarEstructuraRequest struct {
	Cursos []cursoConMateriasDTO `json:"cursos"`
}

type cursoConMateriasDTO struct {
	Anio      int      `json:"anio"`
	Division  string   `json:"division,omitempty"`
	Modalidad string   `json:"modalidad,omitempty"`
	Materias  []string `json:"materias"`
}

// copiarMateriasRequest: RF-02.12 — las materias de un curso a varios de una vez.
type copiarMateriasRequest struct {
	CursosDestinoIDs []string `json:"cursosDestinoIds"`
}

type asignarDocenteRequest struct {
	UsuarioID string `json:"usuarioId"`
	Rol       string `json:"rol"` // TITULAR | SUPLENTE
}

// cambiarRolDocenteRequest solo lleva el rol: el usuario y la materia de un
// vínculo no se editan (ver Service.CambiarRolDocente).
type cambiarRolDocenteRequest struct {
	Rol string `json:"rol"` // TITULAR | SUPLENTE
}

// ── Responses ───────────────────────────────────────────────────────────

type cicloLectivoResponse struct {
	ID        string `json:"id"`
	Anio      int    `json:"anio"`
	Activo    bool   `json:"activo"`
	Archivado bool   `json:"archivado"`
}

func toCicloResponse(c *domain.CicloLectivo) cicloLectivoResponse {
	return cicloLectivoResponse{ID: c.ID, Anio: c.Anio, Activo: c.Activo, Archivado: c.Archivado}
}

type cursoResponse struct {
	ID             string `json:"id"`
	CicloLectivoID string `json:"cicloLectivoId"`
	// Nombre es de sólo lectura: sale de anio + division.
	Nombre    string `json:"nombre"`
	Anio      int    `json:"anio"`
	Division  string `json:"division,omitempty"`
	Modalidad string `json:"modalidad,omitempty"`
	Archivado bool   `json:"archivado"`
}

func toCursoResponse(c *domain.Curso) cursoResponse {
	return cursoResponse{
		ID: c.ID, CicloLectivoID: c.CicloLectivoID, Nombre: c.Nombre,
		Anio: c.Anio, Division: c.Division, Modalidad: c.Modalidad,
		Archivado: c.Archivado,
	}
}

type materiaResponse struct {
	ID      string `json:"id"`
	CursoID string `json:"cursoId"`
	Nombre  string `json:"nombre"`
	// Archivado y nada más: el `activo` que acompañaba a los dos nunca se puso
	// en false y no decidía nada (migración 016).
	Archivado bool `json:"archivado"`
}

func toMateriaResponse(m *domain.Materia) materiaResponse {
	return materiaResponse{ID: m.ID, CursoID: m.CursoID, Nombre: m.Nombre, Archivado: m.Archivado}
}

// docenteMateriaResponse es la relación cruda, para administrarla: asignar,
// cambiar el rol, quitar. No trae el nombre del docente porque quien la usa ya
// tiene el listado de usuarios cargado para poder elegir.
//
// Para MOSTRAR quién dicta qué está asignacionDocenteResponse, que sí resuelve
// los nombres.
type docenteMateriaResponse struct {
	ID        string `json:"id"`
	UsuarioID string `json:"usuarioId"`
	Rol       string `json:"rol"`
}

func toDocenteMateriaResponse(dm *domain.DocenteMateria) docenteMateriaResponse {
	return docenteMateriaResponse{ID: dm.ID, UsuarioID: dm.UsuarioID, Rol: string(dm.Rol)}
}

// asignacionDocenteResponse es la misma relación que docenteMateriaResponse
// pero con los nombres de las dos puntas resueltos, para las pantallas que la
// muestran en vez de administrarla.
type asignacionDocenteResponse struct {
	ID             string `json:"id"`
	UsuarioID      string `json:"usuarioId"`
	DocenteNombre  string `json:"docenteNombre"`
	Rol            string `json:"rol"`
	MateriaID      string `json:"materiaId"`
	MateriaNombre  string `json:"materiaNombre"`
	CursoID        string `json:"cursoId"`
	CursoNombre    string `json:"cursoNombre"`
	CursoModalidad string `json:"cursoModalidad,omitempty"`
}

type archivarCicloResponse struct {
	Archivado        bool    `json:"archivado"`
	NuevoCicloID     *string `json:"nuevoCicloId,omitempty"`
	CursosClonados   int     `json:"cursosClonados"`
	MateriasClonadas int     `json:"materiasClonadas"`
}

func toArchivarCicloResponse(res *application.ResultadoArchivado) archivarCicloResponse {
	return archivarCicloResponse{
		Archivado:        true,
		NuevoCicloID:     res.NuevoCicloID,
		CursosClonados:   res.CursosClonados,
		MateriasClonadas: res.MateriasClonadas,
	}
}

// materiaReservableResponse: RF-04.1 — trae curso y año resueltos porque
// "Matemáticas" a secas no distingue la de 1°A de la de 3°B.
type materiaReservableResponse struct {
	MateriaID      string `json:"materiaId"`
	MateriaNombre  string `json:"materiaNombre"`
	CursoID        string `json:"cursoId"`
	CursoNombre    string `json:"cursoNombre"`
	CursoModalidad string `json:"cursoModalidad,omitempty"`
	CicloID        string `json:"cicloId"`
	CicloAnio      int    `json:"cicloAnio"`
}

// removerDocenteResponse: RF-02.8 — quitar al único docente de una materia
// cancela sus reservas futuras en cascada.
type removerDocenteResponse struct {
	ReservasCanceladas int `json:"reservasCanceladas"`
}

// ── Estructura del ciclo (RF-02.12) ─────────────────────────────────────

func toCursosConMateriasDTO(cursos []application.CursoConMaterias) []cursoConMateriasDTO {
	data := make([]cursoConMateriasDTO, len(cursos))
	for i, c := range cursos {
		// El array nunca sale nulo: un curso sin materias es `[]`, no `null`.
		// Quien consume esto lo recorre sin preguntar, y el JSON es el mismo que
		// después se vuelve a mandar.
		materias := c.Materias
		if materias == nil {
			materias = []string{}
		}
		data[i] = cursoConMateriasDTO{
			Anio: c.Anio, Division: c.Division, Modalidad: c.Modalidad, Materias: materias,
		}
	}
	return data
}

func deCursosConMateriasDTO(dtos []cursoConMateriasDTO) []application.CursoConMaterias {
	cursos := make([]application.CursoConMaterias, len(dtos))
	for i, d := range dtos {
		cursos[i] = application.CursoConMaterias{
			Anio: d.Anio, Division: d.Division, Modalidad: d.Modalidad, Materias: d.Materias,
		}
	}
	return cursos
}

// importarEstructuraResponse dice qué se creó y qué ya estaba, porque una
// importación que no creó nada no falló.
type importarEstructuraResponse struct {
	CursosCreados      int `json:"cursosCreados"`
	CursosExistentes   int `json:"cursosExistentes"`
	MateriasCreadas    int `json:"materiasCreadas"`
	MateriasExistentes int `json:"materiasExistentes"`
}

type copiarMateriasResponse struct {
	MateriasCreadas    int `json:"materiasCreadas"`
	MateriasExistentes int `json:"materiasExistentes"`
	CursosDestino      int `json:"cursosDestino"`
}

// editarMateriaResponse dice cuántas marcas de preferencia de equipo dejaron de
// aplicar por el renombre.
//
// Cero es la respuesta normal. Un número mayor significa que esas máquinas
// quedaron marcadas para un nombre de materia que ya no existe: se listan en
// GET /api/preferencias/huerfanas.
type editarMateriaResponse struct {
	MarcasDeEquipoAfectadas int `json:"marcasDeEquipoAfectadas"`
}
