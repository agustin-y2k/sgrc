package http

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reporting/application"
	"github.com/ramiro/sgrc/internal/reporting/domain"
	"github.com/ramiro/sgrc/internal/shared/authtest"
)

type fakeRepo struct {
	usoPCs           []domain.ResumenUsoPC
	usoDocentes      []domain.ResumenUsoDocente
	historicoPC      []*domain.HistoricoUsoPC
	incidenciasPC    []domain.ResumenIncidenciasPC
	incidenciasCarro []domain.ResumenIncidenciasCarro
}

func (r *fakeRepo) GuardarHistoricoUsoPC(ctx context.Context, h *domain.HistoricoUsoPC) error {
	return nil
}
func (r *fakeRepo) GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error {
	return nil
}
func (r *fakeRepo) ListarHistoricoUsoPCPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoPC, error) {
	return r.historicoPC, nil
}
func (r *fakeRepo) ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error) {
	return nil, nil
}
func (r *fakeRepo) CalcularUsoPCsDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoPC, error) {
	return r.usoPCs, nil
}
func (r *fakeRepo) CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	return r.usoDocentes, nil
}

type fakeInfoPC struct{}

func (f *fakeInfoPC) EtiquetaYCarroDe(ctx context.Context, pcID string) (string, int, string, error) {
	return "PC 1", 1, "Carro 1", nil
}

type fakeInfoUsuario struct{}

func (f *fakeInfoUsuario) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	return "Ada Lovelace", nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeInfoPC{}, &fakeInfoUsuario{}, idSecuencial)
	h := NewHandler(svc)

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

// ── ReporteUsoPCs / ReporteUsoDocentes ──────────────────────────────────

func TestHTTP_ReporteUsoPCs_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{usoPCs: []domain.ResumenUsoPC{{PCID: "pc1", CantidadReservas: 5, MinutosReservados: 300}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reporting/ciclos/ciclo1/uso-pcs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoPCs_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/reporting/ciclos/ciclo1/uso-pcs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoPCs_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/reporting/ciclos/ciclo1/uso-pcs", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoDocentes_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{usoDocentes: []domain.ResumenUsoDocente{{UsuarioID: "docente1", CantidadReservas: 3, MinutosReservados: 180}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reporting/ciclos/ciclo1/uso-docentes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── HistoricoUsoPCs / HistoricoUsoDocentes ──────────────────────────────

func TestHTTP_HistoricoUsoPCs_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{historicoPC: []*domain.HistoricoUsoPC{{ID: "h1", Anio: 2025, PCID: "pc1"}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/reporting/historico/2025/uso-pcs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_HistoricoUsoPCs_AnioInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/reporting/historico/no-es-un-numero/uso-pcs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_HistoricoUsoDocentes_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/reporting/historico/2025/uso-docentes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func (r *fakeRepo) CalcularIncidenciasPorPC(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasPC, error) {
	return r.incidenciasPC, nil
}

func (r *fakeRepo) CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	return r.incidenciasCarro, nil
}
