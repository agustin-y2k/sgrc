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
	estadoInventario   []domain.EstadoDelInventario
	fueraDeCirculacion []domain.EquipoFueraDeCirculacion
	porCategoria       []domain.ResumenPorCategoriaDeFalla
	usoEquipos         []domain.ResumenUsoEquipo
	usoDocentes        []domain.ResumenUsoDocente
	historicoEquipo    []*domain.HistoricoUsoEquipo
	incidenciasEquipo  []domain.ResumenIncidenciasEquipo
	incidenciasCarro   []domain.ResumenIncidenciasCarro
}

func (r *fakeRepo) GuardarHistoricoUsoEquipo(ctx context.Context, h *domain.HistoricoUsoEquipo) error {
	return nil
}
func (r *fakeRepo) GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error {
	return nil
}
func (r *fakeRepo) BorrarHistoricoDocentesSinCuenta(ctx context.Context, anio int) error {
	return nil
}
func (r *fakeRepo) ListarHistoricoUsoEquipoPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error) {
	return r.historicoEquipo, nil
}
func (r *fakeRepo) ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error) {
	return nil, nil
}
func (r *fakeRepo) CalcularUsoEquiposDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoEquipo, error) {
	return r.usoEquipos, nil
}
func (r *fakeRepo) CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	return r.usoDocentes, nil
}

type fakeInfoEquipo struct{}

func (f *fakeInfoEquipo) EtiquetaYCarroDe(ctx context.Context, equipoID string) (string, int, string, error) {
	return "PC 1", 1, "Carro 1", nil
}

type fakeInfoUsuario struct{}

func (f *fakeInfoUsuario) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	return "Ada Lovelace", nil
}

// ptr es azúcar para los campos opcionales de los resúmenes: UsuarioID es
// *string porque una cuenta eliminada deja la referencia en nil.
func ptr(s string) *string { return &s }

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	return montarEn(fiber.New(), repo)
}

// montarEn registra reporting sobre una app que ya puede tener otras rutas —
// que es como corre de verdad: en cmd/main.go conviven los nueve módulos.
func montarEn(app *fiber.App, repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeInfoEquipo{}, &fakeInfoUsuario{}, idSecuencial)
	h := NewHandler(svc)

	RegisterRoutes(app, h, registroDePrueba.Autenticacion(testSecret))
	return app
}

// Los tres resúmenes de incidencias, con una ruta `:id` de la misma forma
// registrada ANTES. Es la reproducción exacta de un bug que estuvo vivo:
// `GET /api/incidencias/{id}` lo sirve `inventory`, que en cmd/main.go se
// registra antes que `reporting`, y con los reportes colgando de
// `/incidencias/<algo>` la palabra "equipos" entraba como id — no era un UUID,
// y las tres respondían 400 «el ID indicado no tiene un formato válido». La
// pestaña de Reportes mostraba ese error y estos handlers no corrían nunca.
//
// El test registra el intruso PRIMERO a propósito: si alguien vuelve a acortar
// estas rutas a un solo segmento, acá se ve, y no en producción.
func TestHTTP_ResumenDeIncidencias_NoLasComeLaRutaConParametro(t *testing.T) {
	app := fiber.New()
	// El de inventory, tal como se registra en cmd/main.go: antes que reporting.
	app.Group("/api").Get("/incidencias/:id", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "el ID indicado no tiene un formato válido")
	})
	montarEn(app, &fakeRepo{})

	for _, ruta := range []string{
		"/api/incidencias/resumen/por-equipo",
		"/api/incidencias/resumen/por-carro",
		"/api/incidencias/resumen/por-categoria",
	} {
		req := httptest.NewRequest("GET", ruta, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: error inesperado: %v", ruta, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("%s: esperaba 200 del reporte, obtuve %d — se la comió la ruta con parámetro",
				ruta, resp.StatusCode)
		}
	}
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

// ── ReporteUsoEquipos / ReporteUsoDocentes ──────────────────────────────

func TestHTTP_ReporteUsoEquipos_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{usoEquipos: []domain.ResumenUsoEquipo{{EquipoID: "pc1", CantidadReservas: 5, MinutosReservados: 300}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/ciclos/ciclo1/uso-equipos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoEquipos_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/ciclos/ciclo1/uso-equipos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoEquipos_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/ciclos/ciclo1/uso-equipos", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReporteUsoDocentes_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{usoDocentes: []domain.ResumenUsoDocente{{UsuarioID: ptr("docente1"), CantidadReservas: 3, MinutosReservados: 180}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/ciclos/ciclo1/uso-docentes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── HistoricoUsoEquipos / HistoricoUsoDocentes ──────────────────────────

func TestHTTP_HistoricoUsoEquipos_ComoAdmin_OK(t *testing.T) {
	repo := &fakeRepo{historicoEquipo: []*domain.HistoricoUsoEquipo{{ID: "h1", Anio: 2025, EquipoID: "pc1"}}}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/historico/2025/uso-equipos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_HistoricoUsoEquipos_AnioInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/historico/no-es-un-numero/uso-equipos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_HistoricoUsoDocentes_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(&fakeRepo{})

	req := httptest.NewRequest("GET", "/api/historico/2025/uso-docentes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func (r *fakeRepo) CalcularIncidenciasPorEquipo(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasEquipo, error) {
	return r.incidenciasEquipo, nil
}

func (r *fakeRepo) CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	return r.incidenciasCarro, nil
}

func (r *fakeRepo) EstadoDelInventario(ctx context.Context) ([]domain.EstadoDelInventario, error) {
	return r.estadoInventario, nil
}

func (r *fakeRepo) EquiposFueraDeCirculacion(ctx context.Context) ([]domain.EquipoFueraDeCirculacion, error) {
	return r.fueraDeCirculacion, nil
}

func (r *fakeRepo) CalcularIncidenciasPorCategoria(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenPorCategoriaDeFalla, error) {
	return r.porCategoria, nil
}
