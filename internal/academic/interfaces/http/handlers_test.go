package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

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
	pedidos               map[string]*domain.PedidoDeMateria
	nombresDeUsuario      map[string]string
	ciclos                map[string]*domain.CicloLectivo
	cursos                map[string]*domain.Curso
	materias              map[string]*domain.Materia
	docentesMateria       map[string]*domain.DocenteMateria
	materiasReservables   []application.MateriaReservable
	filtroDocenteRecibido *string
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		ciclos:           make(map[string]*domain.CicloLectivo),
		cursos:           make(map[string]*domain.Curso),
		materias:         make(map[string]*domain.Materia),
		docentesMateria:  make(map[string]*domain.DocenteMateria),
		nombresDeUsuario: make(map[string]string),
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
func (r *fakeRepo) GuardarDocenteMateria(ctx context.Context, dm *domain.DocenteMateria) error {
	if _, ok := r.docentesMateria[dm.ID]; !ok {
		return application.ErrDocenteMateriaNoEncontrado
	}
	r.docentesMateria[dm.ID] = dm
	return nil
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
		&fakeArchivadorHistorico{}, &fakeCanceladorReservas{}, &fakeDatosDeUsuario{}, idSecuencial,
		relojDeTest, eventbus.NewInMemoryEventBus())
	h := NewHandler(svc, fakeAuditor{})

	app := fiber.New()
	RegisterRoutes(app, h, registroDePrueba.Autenticacion(testSecret))
	return app
}

// registroDePrueba hace de tabla usuario para el middleware de autenticación:
// Token() deja registrado el rol de cada ID, y Autenticacion() se lo devuelve
// al middleware igual que lo haría la base.
var registroDePrueba = authtest.Nuevo()

// tokenPara genera un JWT válido para un usuario de prueba — reusa
// exactamente el mismo formato que produce infrastructure.JWTFirmador, para
// que estos tests ejerciten el middleware de autenticación real.
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
// encontrado probando el servidor a mano: un ID sin formato UUID (ej.
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

func TestHTTP_CambiarRolDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "u1", MateriaID: "m1", Rol: domain.RolTitular}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/academic/materias/m1/docentes/dm1",
		jsonBody(cambiarRolDocenteRequest{Rol: "SUPLENTE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body docenteMateriaResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Rol != "SUPLENTE" {
		t.Errorf("rol = %q, esperaba SUPLENTE", body.Rol)
	}
}

func TestHTTP_CambiarRolDocente_RolInvalido_400(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "u1", MateriaID: "m1", Rol: domain.RolTitular}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/academic/materias/m1/docentes/dm1",
		jsonBody(cambiarRolDocenteRequest{Rol: "PROFESOR"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarRolDocente_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/academic/materias/m1/docentes/dm1",
		jsonBody(cambiarRolDocenteRequest{Rol: "SUPLENTE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// RF-02.8: la respuesta tiene que decir qué se llevó puesto la cascada.
func TestHTTP_RemoverDocenteMateria_DevuelveLasReservasCanceladas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "d1", MateriaID: "m1"}
	svc := application.NewService(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{},
		&fakeArchivadorHistorico{}, &fakeCanceladorReservas{canceladas: 3}, &fakeDatosDeUsuario{},
		idSecuencial, relojDeTest, eventbus.NewInMemoryEventBus())
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

// ── Puertos que sumó el pedido para dictar una materia ──────────────────

type fakeDatosDeUsuario struct{}

func (fakeDatosDeUsuario) Contacto(_ context.Context, usuarioID string) (application.ContactoDeDocente, error) {
	return application.ContactoDeDocente{UsuarioID: usuarioID, Nombre: "Docente de prueba",
		Email: usuarioID + "@escuela.edu.ar"}, nil
}

func (fakeDatosDeUsuario) Contactos(_ context.Context, ids []string) ([]application.ContactoDeDocente, error) {
	var r []application.ContactoDeDocente
	for _, id := range ids {
		r = append(r, application.ContactoDeDocente{UsuarioID: id, Nombre: "Otro docente",
			Email: id + "@escuela.edu.ar"})
	}
	return r, nil
}

func relojDeTest() time.Time {
	return time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
}

func (r *fakeRepo) CrearPedido(_ context.Context, p *domain.PedidoDeMateria) error {
	if r.pedidos == nil {
		r.pedidos = map[string]*domain.PedidoDeMateria{}
	}
	r.pedidos[p.ID] = p
	return nil
}

func (r *fakeRepo) BuscarPedidoPorID(_ context.Context, id string) (*domain.PedidoDeMateria, error) {
	p, ok := r.pedidos[id]
	if !ok {
		return nil, domain.ErrPedidoNoExiste
	}
	return p, nil
}

func (r *fakeRepo) GuardarPedido(_ context.Context, p *domain.PedidoDeMateria) error {
	if r.pedidos == nil {
		r.pedidos = map[string]*domain.PedidoDeMateria{}
	}
	r.pedidos[p.ID] = p
	return nil
}

// detallar hace lo mismo que el LEFT JOIN del repositorio real: le pega el
// nombre de la materia y el del curso, y los deja vacíos cuando la materia
// todavía no existe.
func (r *fakeRepo) detallar(p *domain.PedidoDeMateria) *application.PedidoDetallado {
	d := &application.PedidoDetallado{Pedido: p}
	d.DocenteNombre = r.nombresDeUsuario[p.UsuarioID]
	if p.MateriaID == nil {
		return d
	}
	m, hay := r.materias[*p.MateriaID]
	if !hay {
		return d
	}
	d.MateriaNombre = m.Nombre
	if c, hay := r.cursos[m.CursoID]; hay {
		d.CursoNombre = c.Nombre
	}
	return d
}

func (r *fakeRepo) ListarPedidos(_ context.Context, soloPendientes bool) ([]*application.PedidoDetallado, error) {
	var out []*application.PedidoDetallado
	for _, p := range r.pedidos {
		if soloPendientes && p.Estado != domain.PedidoPendiente {
			continue
		}
		out = append(out, r.detallar(p))
	}
	return out, nil
}

func (r *fakeRepo) ListarPedidosDeUsuario(_ context.Context, usuarioID string) ([]*application.PedidoDetallado, error) {
	var out []*application.PedidoDetallado
	for _, p := range r.pedidos {
		if p.UsuarioID == usuarioID {
			out = append(out, r.detallar(p))
		}
	}
	return out, nil
}

func (r *fakeRepo) ContarPedidosPendientes(_ context.Context) (int, error) {
	n := 0
	for _, p := range r.pedidos {
		if p.Estado == domain.PedidoPendiente {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) TienePedidoAbierto(_ context.Context, usuarioID, materiaID string) (bool, error) {
	for _, p := range r.pedidos {
		if p.UsuarioID == usuarioID && p.Estado == domain.PedidoPendiente &&
			p.MateriaID != nil && *p.MateriaID == materiaID {
			return true, nil
		}
	}
	return false, nil
}

// ── Pedidos para dictar una materia ─────────────────────────────────────

// TestHTTP_ListarPedidos_NombraLaMateriaPedida deja fijado el arreglo de un
// bug real: el pedido guarda `materia_id` y nada más, así que la respuesta no
// tenía con qué decir QUÉ se pidió y las dos pantallas que la muestran caían a
// un texto de relleno ("Una materia existente"). El Admin aprobaba a ciegas.
//
// Con una sola materia cargada el bug es invisible, que es por lo que
// sobrevivió: aparece recién cuando hay varias y dos pedidos distintos se ven
// idénticos.
func TestHTTP_ListarPedidos_NombraLaMateriaPedida(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.cursos["cur1"] = &domain.Curso{ID: "cur1", Nombre: "5°A"}
	repo.materias["m1"] = &domain.Materia{ID: "m1", CursoID: "cur1", Nombre: "Programación"}
	materiaID := "m1"
	repo.nombresDeUsuario["docente1"] = "Marisa Colombo"
	repo.pedidos = map[string]*domain.PedidoDeMateria{
		"ped1": {
			ID: "ped1", UsuarioID: "docente1", MateriaID: &materiaID,
			Motivo: "Tomo el segundo turno", Estado: domain.PedidoPendiente,
			CreadoEn: relojDeTest(),
		},
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/academic/pedidos-de-materia", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var cuerpo struct {
		Data []pedidoResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if len(cuerpo.Data) != 1 {
		t.Fatalf("esperaba 1 pedido, obtuve %d", len(cuerpo.Data))
	}
	if cuerpo.Data[0].MateriaNombre != "Programación" {
		t.Errorf("materiaNombre: esperaba \"Programación\", obtuve %q", cuerpo.Data[0].MateriaNombre)
	}
	if cuerpo.Data[0].CursoNombre != "5°A" {
		t.Errorf("cursoNombre: esperaba \"5°A\", obtuve %q", cuerpo.Data[0].CursoNombre)
	}
	// Aprobar es asignar a ESA persona a la materia: sin el nombre, el Admin
	// decide sobre un pedido que no sabe de quién es.
	if cuerpo.Data[0].DocenteNombre != "Marisa Colombo" {
		t.Errorf("docenteNombre: esperaba \"Marisa Colombo\", obtuve %q", cuerpo.Data[0].DocenteNombre)
	}
}

// Una materia que todavía no existe no tiene nada que resolver por JOIN: lo
// pedido es el texto que escribió la persona, y los dos campos nuevos van
// vacíos para que la pantalla sepa cuál de las dos formas mostrar.
func TestHTTP_ListarPedidos_MateriaNueva_SinNombreResuelto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pedidos = map[string]*domain.PedidoDeMateria{
		"ped1": {
			ID: "ped1", UsuarioID: "docente1",
			CursoSolicitado: "6°A", MateriaSolicitada: "Laboratorio de Redes",
			Motivo: "Es materia nueva del plan", Estado: domain.PedidoPendiente,
			CreadoEn: relojDeTest(),
		},
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/academic/pedidos-de-materia", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	var cuerpo struct {
		Data []pedidoResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if len(cuerpo.Data) != 1 {
		t.Fatalf("esperaba 1 pedido, obtuve %d", len(cuerpo.Data))
	}
	if !cuerpo.Data[0].EsMateriaNueva {
		t.Error("esperaba esMateriaNueva = true")
	}
	if cuerpo.Data[0].MateriaNombre != "" || cuerpo.Data[0].CursoNombre != "" {
		t.Errorf("esperaba los dos nombres vacíos, obtuve %q / %q",
			cuerpo.Data[0].MateriaNombre, cuerpo.Data[0].CursoNombre)
	}
}

// TestHTTP_MisMaterias_Asignadas_UnAdminNoDictaNinguna fija el arreglo de un
// bug de pantalla con raíz acá: el perfil mostraba "Las materias que das" con
// esta respuesta, y a un Admin le listaba TODAS las del sistema.
//
// No estaba mal el endpoint —un Admin puede reservar en cualquier materia
// (RF-04.1)— sino que son dos preguntas distintas y había una sola forma de
// hacerlas. Con `asignadas=true` se responde por asignación, para cualquier rol.
func TestHTTP_MisMaterias_Asignadas_UnAdminNoDictaNinguna(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materiasReservables = []application.MateriaReservable{
		{MateriaID: "m1", MateriaNombre: "Programación", CursoNombre: "5°A", CicloAnio: 2026},
	}
	app := nuevaAppDeTest(repo)

	pedir := func(ruta string) int {
		req := httptest.NewRequest("GET", ruta, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		var cuerpo struct {
			Data []materiaReservableResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
			t.Fatalf("no se pudo leer la respuesta: %v", err)
		}
		return len(cuerpo.Data)
	}

	// Sin el parámetro sigue valiendo lo de siempre: el Admin reserva en todas,
	// y el fake devuelve su lista completa porque no le llega filtro.
	if n := pedir("/api/academic/mis-materias"); n != 1 {
		t.Fatalf("esperaba la lista completa, obtuve %d", n)
	}
	if repo.filtroDocenteRecibido != nil {
		t.Errorf("sin el parámetro no tiene que filtrar por persona, filtró por %q", *repo.filtroDocenteRecibido)
	}

	// Con el parámetro pregunta por asignación, y ahí el rol no cambia nada.
	pedir("/api/academic/mis-materias?asignadas=true")
	if repo.filtroDocenteRecibido == nil {
		t.Fatal("con asignadas=true tiene que filtrar por la persona, no filtró")
	}
	if *repo.filtroDocenteRecibido != "admin1" {
		t.Errorf("filtró por %q, esperaba admin1", *repo.filtroDocenteRecibido)
	}
}
