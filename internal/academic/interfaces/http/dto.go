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
	Activo    bool   `json:"activo"`
	Archivado bool   `json:"archivado"`
}

func toCursoResponse(c *domain.Curso) cursoResponse {
	return cursoResponse{
		ID: c.ID, CicloLectivoID: c.CicloLectivoID, Nombre: c.Nombre,
		Anio: c.Anio, Division: c.Division, Modalidad: c.Modalidad,
		Activo: c.Activo, Archivado: c.Archivado,
	}
}

type materiaResponse struct {
	ID        string `json:"id"`
	CursoID   string `json:"cursoId"`
	Nombre    string `json:"nombre"`
	Activo    bool   `json:"activo"`
	Archivado bool   `json:"archivado"`
}

func toMateriaResponse(m *domain.Materia) materiaResponse {
	return materiaResponse{ID: m.ID, CursoID: m.CursoID, Nombre: m.Nombre, Activo: m.Activo, Archivado: m.Archivado}
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
