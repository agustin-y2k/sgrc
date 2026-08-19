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

type crearCursoRequest struct {
	Nombre string `json:"nombre"`
}

type editarCursoRequest struct {
	Nombre string `json:"nombre"`
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
	Nombre         string `json:"nombre"`
	Activo         bool   `json:"activo"`
	Archivado      bool   `json:"archivado"`
}

func toCursoResponse(c *domain.Curso) cursoResponse {
	return cursoResponse{ID: c.ID, CicloLectivoID: c.CicloLectivoID, Nombre: c.Nombre, Activo: c.Activo, Archivado: c.Archivado}
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

// docenteMateriaResponse no incluye nombre/apellido del docente todavía — eso
// requeriría consultar la tabla usuario (de auth) desde acá, lo cual no está
// en el alcance de esta pasada.
type docenteMateriaResponse struct {
	ID        string `json:"id"`
	UsuarioID string `json:"usuarioId"`
	Rol       string `json:"rol"`
}

func toDocenteMateriaResponse(dm *domain.DocenteMateria) docenteMateriaResponse {
	return docenteMateriaResponse{ID: dm.ID, UsuarioID: dm.UsuarioID, Rol: string(dm.Rol)}
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
	MateriaID     string `json:"materiaId"`
	MateriaNombre string `json:"materiaNombre"`
	CursoID       string `json:"cursoId"`
	CursoNombre   string `json:"cursoNombre"`
	CicloID       string `json:"cicloId"`
	CicloAnio     int    `json:"cicloAnio"`
}

// removerDocenteResponse: RF-02.8 — quitar al único docente de una materia
// cancela sus reservas futuras en cascada.
type removerDocenteResponse struct {
	ReservasCanceladas int `json:"reservasCanceladas"`
}
