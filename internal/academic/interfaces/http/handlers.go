package http

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
)

type Handler struct {
	svc     *application.Service
	auditor audit.Auditor
}

func NewHandler(svc *application.Service, auditor audit.Auditor) *Handler {
	return &Handler{svc: svc, auditor: auditor}
}

// claimsDelContexto es el único punto donde un handler protegido lee el
// usuario autenticado — mismo patrón que internal/auth/interfaces/http.
func claimsDelContexto(c *fiber.Ctx) (*middleware.Claims, error) {
	claims := middleware.ClaimsFromCtx(c)
	if claims == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "no autenticado")
	}
	return claims, nil
}

// auditar registra una entrada de auditoría sin abortar la respuesta HTTP
// si falla (ver internal/auth/interfaces/http.Handler.auditar).
func (h *Handler) auditar(c *fiber.Ctx, actorID, accion, entidad string, entidadID *string, detalle map[string]any) {
	if err := h.auditor.Registrar(c.UserContext(), audit.Entrada{
		UsuarioID: actorID,
		Accion:    accion,
		Entidad:   entidad,
		EntidadID: entidadID,
		Detalle:   detalle,
		IPOrigen:  middleware.IPCliente(c),
	}); err != nil {
		log.Printf("auditoría: no se pudo registrar %s sobre %s: %v", accion, entidad, err)
	}
}

// ── Ciclo lectivo ───────────────────────────────────────────────────────

// POST /api/academic/ciclos (Admin)
func (h *Handler) CrearCiclo(c *fiber.Ctx) error {
	var req crearCicloRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	ciclo, err := h.svc.CrearCiclo(c.UserContext(), req.Anio)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toCicloResponse(ciclo))
}

// GET /api/academic/ciclos (cualquier usuario autenticado)
func (h *Handler) ListarCiclos(c *fiber.Ctx) error {
	var filtroArchivado *bool
	if v := c.Query("archivado"); v != "" {
		b := v == "true"
		filtroArchivado = &b
	}

	ciclos, err := h.svc.ListarCiclos(c.UserContext(), filtroArchivado)
	if err != nil {
		return mapearError(err)
	}

	data := make([]cicloLectivoResponse, len(ciclos))
	for i, cl := range ciclos {
		data[i] = toCicloResponse(cl)
	}
	return c.JSON(fiber.Map{"data": data})
}

// POST /api/academic/ciclos/{id}/archivar (Admin)
func (h *Handler) ArchivarCiclo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req archivarCicloRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	resultado, err := h.svc.ArchivarYClonar(c.UserContext(), id, req.ClonarA)
	if err != nil {
		return mapearError(err)
	}

	h.auditar(c, claims.UserID, audit.CicloArchivadoReservasElim, "ciclo_lectivo", &id, nil)
	if resultado.NuevoCicloID != nil {
		h.auditar(c, claims.UserID, audit.CicloClonado, "ciclo_lectivo", resultado.NuevoCicloID, map[string]any{
			"cicloOrigenId":    id,
			"cursosClonados":   resultado.CursosClonados,
			"materiasClonadas": resultado.MateriasClonadas,
		})
	}
	return c.JSON(toArchivarCicloResponse(resultado))
}

// ── Curso ───────────────────────────────────────────────────────────────

// POST /api/academic/ciclos/{cicloId}/cursos (Admin)
func (h *Handler) CrearCurso(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	var req crearCursoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	curso, err := h.svc.CrearCurso(c.UserContext(), cicloID, req.Nombre)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toCursoResponse(curso))
}

// GET /api/academic/ciclos/{cicloId}/cursos (cualquier usuario autenticado)
func (h *Handler) ListarCursos(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	cursos, err := h.svc.ListarCursos(c.UserContext(), cicloID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]cursoResponse, len(cursos))
	for i, cu := range cursos {
		data[i] = toCursoResponse(cu)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/academic/cursos/{id} (Admin)
func (h *Handler) EditarCurso(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarCursoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	if err := h.svc.EditarCurso(c.UserContext(), id, req.Nombre); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/academic/cursos/{id} (Admin)
func (h *Handler) EliminarCurso(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.EliminarCurso(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.CursoEliminado, "curso", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// ── Materia ─────────────────────────────────────────────────────────────

// POST /api/academic/cursos/{cursoId}/materias (Admin)
func (h *Handler) CrearMateria(c *fiber.Ctx) error {
	cursoID := c.Params("cursoId")

	var req crearMateriaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	materia, err := h.svc.CrearMateria(c.UserContext(), cursoID, req.Nombre)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toMateriaResponse(materia))
}

// GET /api/academic/cursos/{cursoId}/materias (cualquier usuario autenticado)
func (h *Handler) ListarMaterias(c *fiber.Ctx) error {
	cursoID := c.Params("cursoId")

	materias, err := h.svc.ListarMaterias(c.UserContext(), cursoID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]materiaResponse, len(materias))
	for i, m := range materias {
		data[i] = toMateriaResponse(m)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/academic/materias/{id} (Admin)
func (h *Handler) EditarMateria(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarMateriaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	if err := h.svc.EditarMateria(c.UserContext(), id, req.Nombre); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/academic/materias/{id} (Admin)
func (h *Handler) EliminarMateria(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	if err := h.svc.EliminarMateria(c.UserContext(), id); err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.MateriaEliminada, "materia", &id, nil)
	return c.SendStatus(fiber.StatusOK)
}

// ── DocenteMateria ──────────────────────────────────────────────────────

// POST /api/academic/materias/{materiaId}/docentes (Admin)
func (h *Handler) AsignarDocente(c *fiber.Ctx) error {
	materiaID := c.Params("materiaId")

	var req asignarDocenteRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	rol, err := domain.ParseRolDocente(req.Rol)
	if err != nil {
		return mapearError(err)
	}

	dm, err := h.svc.AsignarDocente(c.UserContext(), materiaID, req.UsuarioID, rol)
	if err != nil {
		return mapearError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(toDocenteMateriaResponse(dm))
}

// GET /api/academic/materias/{materiaId}/docentes (cualquier usuario autenticado)
func (h *Handler) ListarDocentesDeMateria(c *fiber.Ctx) error {
	materiaID := c.Params("materiaId")

	docentes, err := h.svc.ListarDocentesDeMateria(c.UserContext(), materiaID)
	if err != nil {
		return mapearError(err)
	}

	data := make([]docenteMateriaResponse, len(docentes))
	for i, dm := range docentes {
		data[i] = toDocenteMateriaResponse(dm)
	}
	return c.JSON(fiber.Map{"data": data})
}

// PATCH /api/academic/materias/{materiaId}/docentes/{docenteMateriaId}
// (Admin) Es el único camino para corregir un rol.
func (h *Handler) CambiarRolDocente(c *fiber.Ctx) error {
	materiaID := c.Params("materiaId")
	id := c.Params("docenteMateriaId")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req cambiarRolDocenteRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	rol, err := domain.ParseRolDocente(req.Rol)
	if err != nil {
		return mapearError(err)
	}

	dm, err := h.svc.CambiarRolDocente(c.UserContext(), id, rol)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.DocenteRolCambiado, "docente_materia", &id, map[string]any{
		"materiaId": materiaID,
		"usuarioId": dm.UsuarioID,
		"rol":       string(dm.Rol),
	})
	return c.JSON(toDocenteMateriaResponse(dm))
}

// DELETE /api/academic/materias/{materiaId}/docentes/{docenteMateriaId} (Admin)
func (h *Handler) RemoverDocenteMateria(c *fiber.Ctx) error {
	materiaID := c.Params("materiaId")
	id := c.Params("docenteMateriaId")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	canceladas, err := h.svc.RemoverDocenteMateria(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}
	h.auditar(c, claims.UserID, audit.DocenteRemovidoDeMateria, "docente_materia", &id, map[string]any{
		"materiaId":          materiaID,
		"reservasCanceladas": canceladas,
	})
	// Se devuelve cuántas reservas se llevó puesta la cascada (RF-02.8): es
	// una operación destructiva y el Admin no tenía forma de enterarse.
	return c.JSON(removerDocenteResponse{ReservasCanceladas: canceladas})
}

// GET /api/academic/mis-materias — RF-04.1: las materias en las que el
// usuario autenticado puede reservar.
func (h *Handler) ListarMisMaterias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	materias, err := h.svc.ListarMateriasReservables(c.UserContext(), claims.UserID, claims.Rol == "ADMIN")
	if err != nil {
		return mapearError(err)
	}

	data := make([]materiaReservableResponse, len(materias))
	for i, m := range materias {
		data[i] = materiaReservableResponse{
			MateriaID: m.MateriaID, MateriaNombre: m.MateriaNombre,
			CursoID: m.CursoID, CursoNombre: m.CursoNombre,
			CicloID: m.CicloID, CicloAnio: m.CicloAnio,
		}
	}
	return c.JSON(fiber.Map{"data": data})
}
