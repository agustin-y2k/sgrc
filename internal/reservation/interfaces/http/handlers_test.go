package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/authtest"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// fakeAuditor descarta toda entrada de auditoría (ver el mismo tipo en
// internal/auth/interfaces/http/handlers_test.go).
type fakeAuditor struct{}

func (fakeAuditor) Registrar(ctx context.Context, e audit.Entrada) error { return nil }

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	grupos         map[string]*domain.ReservaGrupo
	reservas       map[string]*domain.Reserva
	pcsDisponibles []application.PCDisponible
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{grupos: make(map[string]*domain.ReservaGrupo), reservas: make(map[string]*domain.Reserva)}
}

// Estos tests verifican el contrato HTTP (códigos, permisos, parseo), no
// la atomicidad — el todo-o-nada real se prueba contra Postgres en
// infrastructure/, así que acá alcanza con ejecutar fn tal cual.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	return fn(r)
}

func (r *fakeRepo) ListarReservas(ctx context.Context, f application.FiltroReservas) ([]application.ReservaDetallada, int, error) {
	var resultado []application.ReservaDetallada
	for _, res := range r.enOrden() {
		if f.CreadoPor != nil && (res.CreadoPor == nil || *res.CreadoPor != *f.CreadoPor) {
			continue
		}
		if f.PCID != nil && res.PCID != *f.PCID {
			continue
		}
		if !f.IncluirCanceladas && res.Estado == domain.ReservaCancelada {
			continue
		}
		// Los nombres los resuelve un JOIN en el repo real; acá alcanza con
		// valores estables para poder afirmar sobre el JSON de salida.
		resultado = append(resultado, application.ReservaDetallada{
			Reserva:         res,
			PCIdentificador: 7,
			CarroNombre:     "Carro 1",
			MateriaNombre:   "Matemáticas",
			CursoNombre:     "1°A",
		})
	}

	total := len(resultado)
	desde := f.Pagina.Offset()
	if desde >= total {
		return nil, total, nil
	}
	hasta := desde + f.Pagina.Limit()
	if hasta > total {
		hasta = total
	}
	return resultado[desde:hasta], total, nil
}

// enOrden recorre las reservas con un orden estable (el repo real ordena por
// fecha, hora e identificador de PC). Sobre el map pelado el orden cambia
// entre llamadas, y con LIMIT/OFFSET eso hace que un test de paginación pase
// o falle al azar.
func (r *fakeRepo) enOrden() []*domain.Reserva {
	ordenadas := make([]*domain.Reserva, 0, len(r.reservas))
	for _, res := range r.reservas {
		ordenadas = append(ordenadas, res)
	}
	sort.Slice(ordenadas, func(i, j int) bool {
		if !ordenadas[i].Fecha.Equal(ordenadas[j].Fecha) {
			return ordenadas[i].Fecha.Before(ordenadas[j].Fecha)
		}
		if ordenadas[i].HoraInicio != ordenadas[j].HoraInicio {
			return ordenadas[i].HoraInicio < ordenadas[j].HoraInicio
		}
		return ordenadas[i].ID < ordenadas[j].ID
	})
	return ordenadas
}

func (r *fakeRepo) CalendarioDePC(ctx context.Context, pcID string, desde, hasta time.Time) ([]application.BloqueCalendario, error) {
	var resultado []application.BloqueCalendario
	for _, res := range r.reservas {
		if res.PCID != pcID || res.Estado == domain.ReservaCancelada {
			continue
		}
		resultado = append(resultado, application.BloqueCalendario{
			Reserva: res, MateriaNombre: "Matemáticas", CursoNombre: "1°A",
		})
	}
	return resultado, nil
}

func (r *fakeRepo) CrearReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	r.grupos[g.ID] = g
	return nil
}
func (r *fakeRepo) BuscarReservaGrupoPorID(ctx context.Context, id string) (*domain.ReservaGrupo, error) {
	g, ok := r.grupos[id]
	if !ok {
		return nil, application.ErrReservaGrupoNoEncontrado
	}
	return g, nil
}
func (r *fakeRepo) GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	r.grupos[g.ID] = g
	return nil
}
func (r *fakeRepo) CrearReserva(ctx context.Context, res *domain.Reserva) error {
	r.reservas[res.ID] = res
	return nil
}
func (r *fakeRepo) BuscarReservaPorID(ctx context.Context, id string) (*domain.Reserva, error) {
	res, ok := r.reservas[id]
	if !ok {
		return nil, application.ErrReservaNoEncontrada
	}
	return res, nil
}
func (r *fakeRepo) GuardarReserva(ctx context.Context, res *domain.Reserva) error {
	r.reservas[res.ID] = res
	return nil
}
func (r *fakeRepo) ListarReservasPorGrupo(ctx context.Context, reservaGrupoID string) ([]*domain.Reserva, error) {
	var resultado []*domain.Reserva
	for _, res := range r.reservas {
		if res.ReservaGrupoID != nil && *res.ReservaGrupoID == reservaGrupoID {
			resultado = append(resultado, res)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) ListarReservasFuturasDePC(ctx context.Context, pcID string, desde time.Time) ([]*domain.Reserva, error) {
	return nil, nil
}
func (r *fakeRepo) ListarReservasFuturasDeMateria(ctx context.Context, materiaID string, desde time.Time) ([]*domain.Reserva, error) {
	return nil, nil
}
func (r *fakeRepo) EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (int, int, error) {
	return 0, 0, nil
}
func (r *fakeRepo) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	return nil, nil
}
func (r *fakeRepo) ListarPCsDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]application.PCDisponible, error) {
	return r.pcsDisponibles, nil
}

func (r *fakeRepo) CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error {
	return nil
}
func (r *fakeRepo) AsignarPCsARegla(ctx context.Context, reglaID string, pcIDs []string) error {
	return nil
}
func (r *fakeRepo) ListarGruposFuturosDeRegla(ctx context.Context, reglaID string, desde time.Time) ([]*domain.ReservaGrupo, error) {
	return nil, nil
}

type fakeValidadorMateria struct {
	asignado bool
	// archivada simula una materia de un ciclo ya cerrado (RF-04.1).
	archivada bool
}

func (f *fakeValidadorMateria) MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error) {
	return !f.archivada, nil
}

func (f *fakeValidadorMateria) DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error) {
	return f.asignado, nil
}

type fakeValidadorPC struct {
	disponible         bool
	errIdentificadores error
}

func (f *fakeValidadorPC) PCDisponibleParaReservar(ctx context.Context, pcID string) (bool, error) {
	return f.disponible, nil
}

// IdentificadoresDePCs: en los tests las PCs se llaman "pc1", "pc2"… así que
// el número visible sale del sufijo. Alcanza para verificar que el aviso
// nombre las PCs correctas.
func (f *fakeValidadorPC) IdentificadoresDePCs(ctx context.Context, pcIDs []string) (map[string]int, error) {
	if f.errIdentificadores != nil {
		return nil, f.errIdentificadores
	}
	m := make(map[string]int, len(pcIDs))
	for _, id := range pcIDs {
		var n int
		if _, err := fmt.Sscanf(id, "pc%d", &n); err == nil {
			m[id] = n
		}
	}
	return m, nil
}

type fakeObtenedorNombre struct{}

func (f *fakeObtenedorNombre) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	return "Ada Lovelace", nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-generado"
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeValidadorMateria{asignado: true}, &fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{}, idSecuencial, func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus())
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

// ── CrearReserva ────────────────────────────────────────────────────────

func TestHTTP_CrearReserva_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/reservas", jsonBody(crearReservaRequest{
		MateriaID: "materia1", Fecha: "2026-03-09", HoraInicio: "08:00", HoraFin: "09:00", PCIDs: []string{"pc1"},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearReserva_FechaInvalida_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/reservas", jsonBody(crearReservaRequest{
		MateriaID: "materia1", Fecha: "09-03-2026", HoraInicio: "08:00", HoraFin: "09:00", PCIDs: []string{"pc1"},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearReserva_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/reservas", jsonBody(crearReservaRequest{
		MateriaID: "materia1", Fecha: "2026-03-09", HoraInicio: "08:00", HoraFin: "09:00", PCIDs: []string{"pc1"},
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

// ── CancelarReserva — el foco: titularidad ─────────────────────────────

func TestHTTP_CancelarReserva_Propietario_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	dueño := "docente1"
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, CreadoPor: &dueño}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar", jsonBody(cancelarReservaRequest{Motivo: "no puedo dar clase"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CancelarReserva_OtroDocente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	dueño := "docente-dueño"
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, CreadoPor: &dueño}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar", jsonBody(cancelarReservaRequest{Motivo: "motivo"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro-docente", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("un docente no debería poder cancelar la reserva de otro (403), obtuve %d", resp.StatusCode)
	}
	if repo.reservas["r1"].Estado != domain.ReservaConfirmada {
		t.Error("la reserva no debería haberse cancelado")
	}
}

func TestHTTP_CancelarReserva_ComoAdmin_PuedeCancelarLaDeOtro(t *testing.T) {
	repo := nuevoFakeRepo()
	dueño := "docente-dueño"
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, CreadoPor: &dueño}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar", jsonBody(cancelarReservaRequest{Motivo: "PC en mantenimiento"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("un Admin debería poder cancelar la reserva de cualquiera (200), obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CancelarReserva_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/reservas/no-existe/cancelar", jsonBody(cancelarReservaRequest{Motivo: "motivo"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

// ── CancelarOcurrenciaRecurrente — mismo criterio de titularidad ───────

func TestHTTP_CancelarOcurrenciaRecurrente_OtroDocente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	dueño := "docente-dueño"
	repo.grupos["g1"] = &domain.ReservaGrupo{ID: "g1", Estado: domain.GrupoConfirmada, CreadoPor: &dueño}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/grupos/g1/cancelar", jsonBody(cancelarOcurrenciaRequest{Motivo: "motivo", SoloEsta: true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro-docente", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// ── BloquearParaEvaluacion — solo Admin ─────────────────────────────────

func TestHTTP_BloquearParaEvaluacion_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/bloqueos-evaluacion", jsonBody(bloquearEvaluacionRequest{
		PCIDs: []string{"pc1"}, Fecha: "2026-03-09", HoraInicio: "10:00", HoraFin: "12:00", Motivo: "Evaluación",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_BloquearParaEvaluacion_ComoAdmin_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/reservation/bloqueos-evaluacion", jsonBody(bloquearEvaluacionRequest{
		PCIDs: []string{"pc1"}, Fecha: "2026-03-09", HoraInicio: "10:00", HoraFin: "12:00", Motivo: "Evaluación",
	}))
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

// ── ObtenerReservaGrupo ─────────────────────────────────────────────────

func grupoDeTest(repo *fakeRepo, id, creadoPor string) {
	repo.grupos[id] = &domain.ReservaGrupo{
		ID: id, MateriaID: "materia1", CreadoPor: &creadoPor, Estado: domain.GrupoConfirmada,
	}
}

func pedirGrupo(t *testing.T, app *fiber.App, grupoID, usuarioID, rol string) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/reservation/grupos/"+grupoID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara(usuarioID, rol))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return resp.StatusCode
}

func TestHTTP_ObtenerReservaGrupo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	grupoDeTest(repo, "g1", "docente1")
	app := nuevaAppDeTest(repo)

	if got := pedirGrupo(t, app, "g1", "docente1", "DOCENTE"); got != fiber.StatusOK {
		t.Fatalf("esperaba 200 sobre la reserva propia, obtuve %d", got)
	}
}

// Faltaba el chequeo de titularidad: alcanzaba con estar autenticado para
// leer el grupo de cualquier otra persona, con su nombre y su materia
// adentro. Es el mismo criterio que ya aplicaban ListarReservas y las dos
// cancelaciones.
func TestHTTP_ObtenerReservaGrupo_DeOtroDocente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	grupoDeTest(repo, "g1", "otroDocente")
	app := nuevaAppDeTest(repo)

	if got := pedirGrupo(t, app, "g1", "docente1", "DOCENTE"); got != fiber.StatusForbidden {
		t.Fatalf("esperaba 403 sobre la reserva ajena, obtuve %d", got)
	}
}

func TestHTTP_ObtenerReservaGrupo_ComoAdmin_VeLaDeCualquiera(t *testing.T) {
	repo := nuevoFakeRepo()
	grupoDeTest(repo, "g1", "otroDocente")
	app := nuevaAppDeTest(repo)

	if got := pedirGrupo(t, app, "g1", "admin1", "ADMIN"); got != fiber.StatusOK {
		t.Fatalf("un Admin debería poder verla, obtuve %d", got)
	}
}

// Un bloqueo por evaluación no tiene ReservaGrupo, pero si alguna vez
// existiera un grupo sin creador, "de nadie" no puede significar "de todos".
func TestHTTP_ObtenerReservaGrupo_SinCreador_403ParaDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.grupos["g1"] = &domain.ReservaGrupo{ID: "g1", MateriaID: "materia1", Estado: domain.GrupoConfirmada}
	app := nuevaAppDeTest(repo)

	if got := pedirGrupo(t, app, "g1", "docente1", "DOCENTE"); got != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", got)
	}
}

// ── ListarReservas / Calendario / motivo obligatorio ───────────────────

func reservaDeTest(repo *fakeRepo, id, pcID, creadoPor string) *domain.Reserva {
	grupoID := "grupo-" + id
	materiaID := "materia1"
	nombre := "Ada Lovelace"
	repo.grupos[grupoID] = &domain.ReservaGrupo{
		ID: grupoID, MateriaID: materiaID, CreadoPor: &creadoPor,
		Fecha: time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), Estado: domain.GrupoConfirmada,
	}
	res := &domain.Reserva{
		ID: id, ReservaGrupoID: &grupoID, PCID: pcID, MateriaID: &materiaID,
		NombreDocenteSnapshot: &nombre, CreadoPor: &creadoPor,
		Fecha:      time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
		HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour,
		Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal,
	}
	repo.reservas[id] = res
	return res
}

// Sin este endpoint un docente no tenía forma de recuperar sus reservas:
// solo podía pedir un grupo puntual si se acordaba del UUID.
func TestHTTP_ListarReservas_UnDocenteSoloVeLasSuyas(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	reservaDeTest(repo, "r2", "pc2", "otroDocente")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/reservas", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body listarReservasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "r1" {
		t.Fatalf("un docente solo debe ver sus propias reservas, obtuve %+v", body.Data)
	}
}

// El listado tiene que traer los nombres resueltos: antes devolvía pc_id y
// materia_id como UUIDs, así que "Mis reservas" no podía decir de qué PC ni
// de qué materia era cada tarjeta — una reserva de ocho PCs se veía como
// ocho filas idénticas.
func TestHTTP_ListarReservas_TraeNombresResueltos(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/reservas", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	var body listarReservasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("esperaba 1 reserva, obtuve %d", len(body.Data))
	}
	got := body.Data[0]
	if got.PCIdentificador != 7 || got.CarroNombre != "Carro 1" {
		t.Errorf("faltan los datos de la PC: identificador=%d carro=%q", got.PCIdentificador, got.CarroNombre)
	}
	if got.MateriaNombre != "Matemáticas" || got.CursoNombre != "1°A" {
		t.Errorf("faltan los datos de la materia: materia=%q curso=%q", got.MateriaNombre, got.CursoNombre)
	}
}

func TestHTTP_ListarReservas_UnAdminLasVeTodas(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	reservaDeTest(repo, "r2", "pc2", "otroDocente")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/reservas", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	var body listarReservasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("un Admin debe ver todas, obtuve %d", len(body.Data))
	}
}

// ── Paginación ─────────────────────────────────────────────────────────
//
// Es el único listado que crece con el uso: una fila por PC, por clase, por
// semana. Sin cota devolvía las 3.863 filas de un año en una sola respuesta
// de 2,1 MB.

func TestHTTP_ListarReservas_PaginaYTotal(t *testing.T) {
	repo := nuevoFakeRepo()
	for i := 0; i < 5; i++ {
		reservaDeTest(repo, fmt.Sprintf("r%d", i), "pc1", "docente1")
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/reservas?page=2&pageSize=2", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body listarReservasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("esperaba 2 filas en la página, obtuve %d", len(body.Data))
	}
	// El total es de todas las que matchean el filtro, no las de la página:
	// es con lo único que el cliente puede saber que hay una siguiente.
	if body.Meta.Total != 5 || body.Meta.Page != 2 || body.Meta.PageSize != 2 {
		t.Errorf("meta incorrecta: %+v", body.Meta)
	}
	// Segunda página de un orden estable: las dos primeras quedaron atrás.
	if body.Data[0].ID != "r2" || body.Data[1].ID != "r3" {
		t.Errorf("página equivocada: %s, %s", body.Data[0].ID, body.Data[1].ID)
	}
}

// Sin parámetros el endpoint sigue funcionando igual que antes para el
// cliente, pero acotado: un cliente viejo no se trae el año entero.
func TestHTTP_ListarReservas_SinParametros_UsaLaVentanaPorDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/reservas", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	var body listarReservasResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Meta.Page != 1 || body.Meta.PageSize != paginacion.TamanioPorDefecto {
		t.Errorf("meta incorrecta: %+v", body.Meta)
	}
	if body.Meta.Total != 1 || len(body.Data) != 1 {
		t.Errorf("esperaba la única reserva: %+v", body)
	}
}

// pageSize sin techo no protege de nada: alcanza con pedir 100000 para
// volver al listado sin cota.
func TestHTTP_ListarReservas_ParametrosInvalidos_400(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	for _, query := range []string{"?page=0", "?page=abc", "?pageSize=0", "?pageSize=100000"} {
		req := httptest.NewRequest("GET", "/api/reservation/reservas"+query, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("%s: esperaba 400, obtuve %d", query, resp.StatusCode)
		}
	}
}

// RF-04.4
func TestHTTP_CalendarioDePC_DevuelveDocenteYMateria(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reservation/pcs/pc1/calendario?desde=2026-03-01&hasta=2026-03-31", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente2", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body calendarioPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Bloques) != 1 {
		t.Fatalf("esperaba 1 bloque ocupado, obtuve %d", len(body.Bloques))
	}
	b := body.Bloques[0]
	if b.Docente != "Ada Lovelace" || b.MateriaNombre != "Matemáticas" || b.HoraInicio != "08:00" {
		t.Errorf("el bloque debe traer docente, materia y horario (RF-04.4): %+v", b)
	}
}

func TestHTTP_CalendarioDePC_SinRango_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/reservation/pcs/pc1/calendario", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400 sin desde/hasta, obtuve %d", resp.StatusCode)
	}
}

// RF-04.8: el motivo es el texto que el docente recibe en la notificación,
// así que cancelar la reserva de otro sin motivo no puede pasar.
func TestHTTP_CancelarReservaAjena_SinMotivo_400(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar",
		jsonBody(cancelarReservaRequest{Motivo: "   "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400 al cancelar una reserva ajena sin motivo, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CancelarReservaAjena_ConMotivo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar",
		jsonBody(cancelarReservaRequest{Motivo: "se necesita el laboratorio para un acto"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// Cancelar la propia no exige motivo.
func TestHTTP_CancelarReservaPropia_SinMotivo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(repo, "r1", "pc1", "docente1")
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/reservation/reservas/r1/cancelar",
		jsonBody(cancelarReservaRequest{Motivo: ""}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200 al cancelar la propia sin motivo, obtuve %d", resp.StatusCode)
	}
}
