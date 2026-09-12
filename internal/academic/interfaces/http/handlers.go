package http

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/middleware"
	"github.com/ramiro/sgrc/internal/shared/respuesta"
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

// POST /api/ciclos (Admin)
func (h *Handler) CrearCiclo(c *fiber.Ctx) error {
	var req crearCicloRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	ciclo, err := h.svc.CrearCiclo(c.UserContext(), req.Anio)
	if err != nil {
		return mapearError(err)
	}
	return respuesta.Creado(c, "/api/ciclos", ciclo.ID, toCicloResponse(ciclo))
}

// GET /api/ciclos (cualquier usuario autenticado)
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

// POST /api/ciclos/{id}/archivar (Admin)
// PATCH /api/ciclos/{id} (Admin) — corregir el año.
//
// El valor viejo va en la auditoría junto al nuevo porque el año ES el nombre
// del ciclo en pantalla: sin él, una entrada anterior que hable del «ciclo
// 2026» se leería después como si fuera sobre otro ciclo.
func (h *Handler) CorregirCiclo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	var req corregirCicloRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	anterior, err := h.svc.ObtenerCiclo(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}

	if err := h.svc.CorregirAnioDeCiclo(c.UserContext(), id, req.Anio); err != nil {
		return mapearError(err)
	}

	if anterior.Anio != req.Anio {
		h.auditar(c, claims.UserID, audit.CicloAnioCorregido, "ciclo_lectivo", &id, map[string]any{
			"anioAnterior": anterior.Anio,
			"anioNuevo":    req.Anio,
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/ciclos/{id} (Admin) — eliminar un ciclo vacío.
//
// Se audita con el año adentro y no sólo con el id: después del borrado el id
// no se puede resolver contra nada, así que sin el año la entrada diría que
// alguien eliminó un ciclo sin decir cuál.
func (h *Handler) EliminarCiclo(c *fiber.Ctx) error {
	id := c.Params("id")
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	ciclo, err := h.svc.ObtenerCiclo(c.UserContext(), id)
	if err != nil {
		return mapearError(err)
	}

	if err := h.svc.EliminarCiclo(c.UserContext(), id); err != nil {
		return mapearError(err)
	}

	h.auditar(c, claims.UserID, audit.CicloEliminado, "ciclo_lectivo", &id, map[string]any{
		"anio": ciclo.Anio,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

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

// POST /api/ciclos/{cicloId}/cursos (Admin)
func (h *Handler) CrearCurso(c *fiber.Ctx) error {
	cicloID := c.Params("cicloId")

	var req crearCursoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	curso, err := h.svc.CrearCurso(c.UserContext(), cicloID, req.Anio, req.Division, req.Modalidad)
	if err != nil {
		return mapearError(err)
	}
	return respuesta.Creado(c, "/api/cursos", curso.ID, toCursoResponse(curso))
}

// GET /api/ciclos/{cicloId}/cursos (cualquier usuario autenticado)
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

// PATCH /api/cursos/{id} (Admin)
func (h *Handler) EditarCurso(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarCursoRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	if err := h.svc.EditarCurso(c.UserContext(), id, req.Anio, req.Division, req.Modalidad); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

// DELETE /api/cursos/{id} (Admin)
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

// POST /api/cursos/{cursoId}/materias (Admin)
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
	return respuesta.Creado(c, "/api/materias", materia.ID, toMateriaResponse(materia))
}

// GET /api/cursos/{cursoId}/materias (cualquier usuario autenticado)
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

// PATCH /api/materias/{id} (Admin)
func (h *Handler) EditarMateria(c *fiber.Ctx) error {
	id := c.Params("id")

	var req editarMateriaRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cuerpo de la petición inválido")
	}

	// Renombrar devuelve cuántas marcas de preferencia de equipo dejaron de
	// aplicar: la marca se vincula a la materia por NOMBRE (RF-03.21), así que
	// un renombre las deja apuntando a algo que ya no está. Antes eso pasaba en
	// silencio y la marca seguía viéndose como si valiera.
	//
	// Es un 200 con un dato, no un error: corregir un nombre mal escrito es
	// legítimo y no puede depender de cuántas máquinas quedaron marcadas.
	marcasAfectadas, err := h.svc.EditarMateria(c.UserContext(), id, req.Nombre)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(editarMateriaResponse{MarcasDeEquipoAfectadas: marcasAfectadas})
}

// DELETE /api/materias/{id} (Admin)
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

// POST /api/materias/{materiaId}/docentes (Admin)
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

// GET /api/materias/{materiaId}/docentes (cualquier usuario autenticado)
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

// PATCH /api/materias/{materiaId}/docentes/{docenteMateriaId}
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

// DELETE /api/materias/{materiaId}/docentes/{docenteMateriaId} (Admin)
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

// GET /api/mis-materias — RF-04.1: las materias en las que el
// usuario autenticado puede reservar.
//
// `?asignadas=true` cambia la pregunta: en qué materias está ASIGNADA esa
// persona. Para un docente las dos respuestas coinciden; para un Admin no,
// porque puede reservar en todas y normalmente no dicta ninguna.
//
// Existe porque el perfil las mostraba bajo el título "Las materias que das" y
// a un Admin le listaba las ocho del sistema. Son dos preguntas distintas y
// hacían falta las dos: el selector de una reserva nueva quiere la primera, el
// perfil la segunda.
func (h *Handler) ListarMisMaterias(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	// Con `asignadas=true` se responde igual para cualquier rol: filtrando por
	// la persona. Es exactamente lo que ya hace el camino del docente.
	comoAdmin := claims.Rol == "ADMIN" && c.Query("asignadas") != "true"

	materias, err := h.svc.ListarMateriasReservables(c.UserContext(), claims.UserID, comoAdmin)
	if err != nil {
		return mapearError(err)
	}

	data := make([]materiaReservableResponse, len(materias))
	for i, m := range materias {
		data[i] = materiaReservableResponse{
			MateriaID: m.MateriaID, MateriaNombre: m.MateriaNombre,
			CursoID: m.CursoID, CursoNombre: m.CursoNombre,
			CursoModalidad: m.CursoModalidad,
			CicloID:        m.CicloID, CicloAnio: m.CicloAnio,
		}
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/ciclos/{cicloId}/asignaciones (Admin) — quién dicta qué en
// ese ciclo, en una sola respuesta.
//
// Las dos pantallas que muestran esta relación la leían a medias: las materias
// de un curso mostraban sus docentes sólo al desplegar una por una, y el
// listado de usuarios no decía qué dictaba cada quien. Las dos la necesitan
// entera, así que se pide una vez y cada una la mira desde su punta.
func (h *Handler) ListarAsignaciones(c *fiber.Ctx) error {
	asignaciones, err := h.svc.ListarAsignaciones(c.UserContext(), c.Params("cicloId"))
	if err != nil {
		return mapearError(err)
	}

	data := make([]asignacionDocenteResponse, len(asignaciones))
	for i, a := range asignaciones {
		data[i] = asignacionDocenteResponse{
			ID: a.ID, UsuarioID: a.UsuarioID, DocenteNombre: a.DocenteNombre,
			Rol: a.Rol, MateriaID: a.MateriaID, MateriaNombre: a.MateriaNombre,
			CursoID: a.CursoID, CursoNombre: a.CursoNombre,
			CursoModalidad: a.CursoModalidad,
		}
	}
	return c.JSON(fiber.Map{"data": data})
}

// GET /api/docentes/{usuarioId}/materias (Admin) — en qué materias
// está asignada OTRA persona.
//
// Es la misma pregunta que `mis-materias?asignadas=true`, hecha sobre alguien
// más, y la hace el mostrador: cuando quien viene a buscar un equipo tiene
// cuenta, sus cursos son el destino más probable de esa entrega (RF-08.23), y
// el Admin no tiene por qué acordarse de qué dicta cada docente.
//
// Sólo Admin: es el único rol que opera el mostrador, y saber qué dicta otro
// no le hace falta a nadie más.
func (h *Handler) ListarMateriasDeDocente(c *fiber.Ctx) error {
	usuarioID := c.Params("usuarioId")

	// esAdmin=false a propósito, aunque quien pregunta sea Admin: lo que se
	// pide es lo que dicta ESA persona, no lo que podría reservar.
	materias, err := h.svc.ListarMateriasReservables(c.UserContext(), usuarioID, false)
	if err != nil {
		return mapearError(err)
	}

	data := make([]materiaReservableResponse, len(materias))
	for i, m := range materias {
		data[i] = materiaReservableResponse{
			MateriaID: m.MateriaID, MateriaNombre: m.MateriaNombre,
			CursoID: m.CursoID, CursoNombre: m.CursoNombre,
			CursoModalidad: m.CursoModalidad,
			CicloID:        m.CicloID, CicloAnio: m.CicloAnio,
		}
	}
	return c.JSON(fiber.Map{"data": data})
}

// ── GET de un recurso solo ──────────────────────────────────────────────
//
// Los tres se podían editar y eliminar sin poder pedirlos. El permiso es el de
// su listado: cualquier usuario autenticado, porque un docente necesita ver la
// estructura académica para saber sobre qué reserva.

// GET /api/ciclos/{id}
func (h *Handler) ObtenerCiclo(c *fiber.Ctx) error {
	ciclo, err := h.svc.ObtenerCiclo(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toCicloResponse(ciclo))
}

// GET /api/cursos/{id}
func (h *Handler) ObtenerCurso(c *fiber.Ctx) error {
	curso, err := h.svc.ObtenerCurso(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toCursoResponse(curso))
}

// GET /api/materias/{id}
func (h *Handler) ObtenerMateria(c *fiber.Ctx) error {
	materia, err := h.svc.ObtenerMateria(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(toMateriaResponse(materia))
}
