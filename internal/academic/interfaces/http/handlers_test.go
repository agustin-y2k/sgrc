package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/authtest"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// fakeAuditor descarta toda entrada de auditoría (ver el mismo tipo en
// internal/auth/interfaces/http/handlers_test.go).
type fakeAuditor struct{}

func (fakeAuditor) Registrar(ctx context.Context, e audit.Entrada) error { return nil }

// ── fakeRepo (mismo patrón que application/service_test.go) ───────────

type fakeRepo struct {
	ciclos                map[string]*domain.CicloLectivo
	cursos                map[string]*domain.Curso
	materias              map[string]*domain.Materia
	docentesMateria       map[string]*domain.DocenteMateria
	materiasReservables   []application.MateriaReservable
	filtroDocenteRecibido *string
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		ciclos:          make(map[string]*domain.CicloLectivo),
		cursos:          make(map[string]*domain.Curso),
		materias:        make(map[string]*domain.Materia),
		docentesMateria: make(map[string]*domain.DocenteMateria),
	}
}

func (r *fakeRepo) CrearCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	r.ciclos[c.ID] = c
	return nil
}
func (r *fakeRepo) BuscarCicloActivo(ctx context.Context) (*domain.CicloLectivo, error) {
	for _, c := range r.ciclos {
		if c.Activo {
			return c, nil
		}
	}
	return nil, application.ErrCicloNoEncontrado
}
func (r *fakeRepo) BuscarCicloPorID(ctx context.Context, id string) (*domain.CicloLectivo, error) {
	c, ok := r.ciclos[id]
	if !ok {
		return nil, application.ErrCicloNoEncontrado
	}
	return c, nil
}
func (r *fakeRepo) GuardarCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	r.ciclos[c.ID] = c
	return nil
}
func (r *fakeRepo) ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error) {
	var resultado []*domain.CicloLectivo
	for _, c := range r.ciclos {
		if filtroArchivado != nil && c.Archivado != *filtroArchivado {
			continue
		}
		resultado = append(resultado, c)
	}
	return resultado, nil
}
func (r *fakeRepo) CrearCurso(ctx context.Context, c *domain.Curso) error {
	r.cursos[c.ID] = c
	return nil
}
func (r *fakeRepo) BuscarCursoPorID(ctx context.Context, id string) (*domain.Curso, error) {
	c, ok := r.cursos[id]
	if !ok {
		return nil, application.ErrCursoNoEncontrado
	}
	return c, nil
}
func (r *fakeRepo) GuardarCurso(ctx context.Context, c *domain.Curso) error {
	r.cursos[c.ID] = c
	return nil
}
func (r *fakeRepo) EliminarCurso(ctx context.Context, id string) error {
	if _, ok := r.cursos[id]; !ok {
		return application.ErrCursoNoEncontrado
	}
	delete(r.cursos, id)
	return nil
}
func (r *fakeRepo) ListarCursosPorCiclo(ctx context.Context, cicloID string) ([]*domain.Curso, error) {
	var resultado []*domain.Curso
	for _, c := range r.cursos {
		if c.CicloLectivoID == cicloID {
			resultado = append(resultado, c)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) CrearMateria(ctx context.Context, m *domain.Materia) error {
	r.materias[m.ID] = m
	return nil
}
func (r *fakeRepo) BuscarMateriaPorID(ctx context.Context, id string) (*domain.Materia, error) {
	m, ok := r.materias[id]
	if !ok {
		return nil, application.ErrMateriaNoEncontrada
	}
	return m, nil
}
func (r *fakeRepo) GuardarMateria(ctx context.Context, m *domain.Materia) error {
	r.materias[m.ID] = m
	return nil
}
func (r *fakeRepo) EliminarMateria(ctx context.Context, id string) error {
	if _, ok := r.materias[id]; !ok {
		return application.ErrMateriaNoEncontrada
	}
	delete(r.materias, id)
	return nil
}
func (r *fakeRepo) ListarMateriasPorCurso(ctx context.Context, cursoID string) ([]*domain.Materia, error) {
	var resultado []*domain.Materia
	for _, m := range r.materias {
		if m.CursoID == cursoID {
			resultado = append(resultado, m)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) AsignarDocente(ctx context.Context, dm *domain.DocenteMateria) error {
	r.docentesMateria[dm.ID] = dm
	return nil
}
func (r *fakeRepo) BuscarDocenteMateria(ctx context.Context, id string) (*domain.DocenteMateria, error) {
	dm, ok := r.docentesMateria[id]
	if !ok {
		return nil, application.ErrDocenteMateriaNoEncontrado
	}
	return dm, nil
}
func (r *fakeRepo) RemoverDocenteMateria(ctx context.Context, id string) error {
	if _, ok := r.docentesMateria[id]; !ok {
		return application.ErrDocenteMateriaNoEncontrado
	}
	delete(r.docentesMateria, id)
	return nil
}
func (r *fakeRepo) ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error) {
	var resultado []*domain.DocenteMateria
	for _, dm := range r.docentesMateria {
		if dm.MateriaID == materiaID {
			resultado = append(resultado, dm)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) ListarMateriasReservables(ctx context.Context, soloDelDocente *string) ([]application.MateriaReservable, error) {
	r.filtroDocenteRecibido = soloDelDocente
	return r.materiasReservables, nil
}

func (r *fakeRepo) ArchivarCiclo(ctx context.Context, cicloID string) error {
	c, ok := r.ciclos[cicloID]
	if !ok {
		return application.ErrCicloNoEncontrado
	}
	c.Activo = false
	c.Archivado = true
	return nil
}
func (r *fakeRepo) ClonarCicloA(ctx context.Context, cicloOrigenID string, nuevoCiclo *domain.CicloLectivo) (int, int, error) {
	r.ciclos[nuevoCiclo.ID] = nuevoCiclo
	return 0, 0, nil
}

type fakeValidadorUsuario struct{ valido bool }

func (f *fakeValidadorUsuario) ExisteYAprobado(ctx context.Context, usuarioID string) (bool, error) {
	return f.valido, nil
}

type fakeValidadorReservas struct{}

func (f *fakeValidadorReservas) TieneReservasCurso(ctx context.Context, cursoID string) (bool, error) {
	return false, nil
}
func (f *fakeValidadorReservas) TieneReservasMateria(ctx context.Context, materiaID string) (bool, error) {
	return false, nil
}
func (f *fakeValidadorReservas) TieneReservasDeCiclo(ctx context.Context, cicloID string) (bool, error) {
	return false, nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

// fakeArchivadorHistorico: los tests HTTP de este archivo prueban ruteo y
// RBAC, no el detalle de la cascada de archivado (eso ya está cubierto en
// application/service_test.go) — así que este fake es no-op.
type fakeArchivadorHistorico struct{}

func (f *fakeArchivadorHistorico) GuardarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error {
	return nil
}

func (f *fakeArchivadorHistorico) EliminarReservasDeCiclo(ctx context.Context, cicloID string) error {
	return nil
}

type fakeCanceladorReservas struct{ canceladas int }

func (f *fakeCanceladorReservas) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	return f.canceladas, nil
}

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{},
		&fakeArchivadorHistorico{}, &fakeCanceladorReservas{}, idSecuencial, eventbus.NewInMemoryEventBus())
	h := NewHandler(svc, fakeAuditor{})

	app := fiber.New()
	RegisterRoutes(app, h, registroDePrueba.Autenticacion(testSecret))
	return app
}

// registroDePrueba hace de tabla usuario para el middleware de
// autenticación: Token() deja registrado el rol de cada ID, y
// Autenticacion() se lo devuelve al middleware igual que lo haría la base.
var registroDePrueba = authtest.Nuevo()

// tokenPara genera un JWT válido para un usuario de prueba — reusa
// exactamente el mismo formato que produce infrastructure.JWTFirmador,
// para que estos tests ejerciten el middleware de autenticación real.
func tokenPara(id, rol string) string {
	return registroDePrueba.Token(testSecret, id, rol)
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// ── Ciclo lectivo ───────────────────────────────────────────────────────

func TestHTTP_CrearCiclo_ComoAdmin_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/ciclos", jsonBody(crearCicloRequest{Anio: 2026}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearCiclo_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/ciclos", jsonBody(crearCicloRequest{Anio: 2026}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearCiclo_AnioInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/ciclos", jsonBody(crearCicloRequest{Anio: 1500}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ListarCiclos_ComoDocente_OK(t *testing.T) {
	// A diferencia de crear, listar es para cualquier usuario autenticado.
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2026, Activo: true}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/academic/ciclos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ArchivarCiclo_YaArchivado_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Archivado: true}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/academic/ciclos/c1/archivar", jsonBody(archivarCicloRequest{}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

// ── Curso ───────────────────────────────────────────────────────────────

func TestHTTP_CrearCurso_NombreInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/ciclos/c1/cursos", jsonBody(crearCursoRequest{Nombre: "primero A"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearCurso_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/ciclos/c1/cursos", jsonBody(crearCursoRequest{Nombre: "1°A"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_EliminarCurso_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("DELETE", "/api/academic/cursos/no-existe", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

// TestHTTP_IDInvalido_ContratoDocumentado deja constancia del bug real
// encontrado probando el servidor a mano: un ID sin formato UUID (ej. un
// placeholder de ejemplo pegado sin reemplazar en la URL) generaba un 500
// en vez de un 400. El fakeRepo de este archivo no reproduce el error de
// Postgres (22P02) porque no hay Postgres detrás — la cobertura real de
// esIDInvalido está en los tests de integración de
// internal/academic/infrastructure. Este test queda como documentación
// del contrato esperado, no como prueba del fix en sí.
func TestHTTP_IDInvalido_ContratoDocumentado(t *testing.T) {
	t.Skip("cobertura real en internal/academic/infrastructure (integration) — ver esIDInvalido")
}

// ── Materia ─────────────────────────────────────────────────────────────

func TestHTTP_CrearMateria_NombreVacio_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/academic/cursos/curso1/materias", jsonBody(crearMateriaRequest{Nombre: "   "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

// ── DocenteMateria ──────────────────────────────────────────────────────

func TestHTTP_AsignarDocente_RolInvalido_400(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/academic/materias/m1/docentes",
		jsonBody(asignarDocenteRequest{UsuarioID: "u1", Rol: "PROFESOR"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_AsignarDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/academic/materias/m1/docentes",
		jsonBody(asignarDocenteRequest{UsuarioID: "u1", Rol: "TITULAR"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

// RF-02.8: la respuesta tiene que decir qué se llevó puesto la cascada. El
// endpoint devolvía 200 vacío, así que el Admin quitaba al último docente de
// una materia y no se enteraba de que había cancelado clases.
func TestHTTP_RemoverDocenteMateria_DevuelveLasReservasCanceladas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "d1", MateriaID: "m1"}
	svc := application.NewService(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{},
		&fakeArchivadorHistorico{}, &fakeCanceladorReservas{canceladas: 3}, idSecuencial,
		eventbus.NewInMemoryEventBus())
	app := fiber.New()
	RegisterRoutes(app, NewHandler(svc, fakeAuditor{}), registroDePrueba.Autenticacion(testSecret))

	req := httptest.NewRequest("DELETE", "/api/academic/materias/m1/docentes/dm1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body removerDocenteResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ReservasCanceladas != 3 {
		t.Errorf("reservasCanceladas = %d, esperaba 3", body.ReservasCanceladas)
	}
}

func TestHTTP_RemoverDocenteMateria_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("DELETE", "/api/academic/materias/m1/docentes/dm1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}
