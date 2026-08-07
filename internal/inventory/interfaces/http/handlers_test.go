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

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/authtest"
)

// fakeAuditor descarta toda entrada de auditoría (ver el mismo tipo en
// internal/auth/interfaces/http/handlers_test.go).
type fakeAuditor struct{}

func (fakeAuditor) Registrar(ctx context.Context, e audit.Entrada) error { return nil }

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	carros      map[string]*domain.Carro
	pcs         map[string]*domain.PC
	incidencias map[string]*domain.Incidencia
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		carros:      make(map[string]*domain.Carro),
		pcs:         make(map[string]*domain.PC),
		incidencias: make(map[string]*domain.Incidencia),
	}
}

func (r *fakeRepo) CrearCarro(ctx context.Context, c *domain.Carro) error {
	r.carros[c.ID] = c
	return nil
}
func (r *fakeRepo) BuscarCarroPorID(ctx context.Context, id string) (*domain.Carro, error) {
	c, ok := r.carros[id]
	if !ok {
		return nil, application.ErrCarroNoEncontrado
	}
	return c, nil
}
func (r *fakeRepo) GuardarCarro(ctx context.Context, c *domain.Carro) error {
	r.carros[c.ID] = c
	return nil
}
func (r *fakeRepo) ListarCarros(ctx context.Context) ([]*domain.Carro, error) {
	var resultado []*domain.Carro
	for _, c := range r.carros {
		resultado = append(resultado, c)
	}
	return resultado, nil
}
func (r *fakeRepo) CrearPC(ctx context.Context, pc *domain.PC) error {
	r.pcs[pc.ID] = pc
	return nil
}
func (r *fakeRepo) BuscarPCPorID(ctx context.Context, id string) (*domain.PC, error) {
	pc, ok := r.pcs[id]
	if !ok {
		return nil, application.ErrPCNoEncontrada
	}
	return pc, nil
}
func (r *fakeRepo) GuardarPC(ctx context.Context, pc *domain.PC) error {
	r.pcs[pc.ID] = pc
	return nil
}
func (r *fakeRepo) ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error) {
	var resultado []*domain.PC
	for _, pc := range r.pcs {
		if pc.CarroID == carroID {
			resultado = append(resultado, pc)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) CrearIncidencia(ctx context.Context, i *domain.Incidencia) error {
	r.incidencias[i.ID] = i
	return nil
}
func (r *fakeRepo) BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error) {
	i, ok := r.incidencias[id]
	if !ok {
		return nil, application.ErrIncidenciaNoEncontrada
	}
	return i, nil
}
func (r *fakeRepo) GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error {
	r.incidencias[i.ID] = i
	return nil
}
func (r *fakeRepo) ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.PCID == pcID {
			resultado = append(resultado, i)
		}
	}
	return resultado, nil
}

type fakeValidadorReservas struct{}

func (f *fakeValidadorReservas) CancelarReservasFuturasDePC(ctx context.Context, pcID, motivo string) (int, int, error) {
	return 0, 0, nil
}

func (f *fakeValidadorReservas) TieneReservasFuturas(ctx context.Context, pcID string) (bool, error) {
	return false, nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeValidadorReservas{}, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	})
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

// ── Carro ───────────────────────────────────────────────────────────────

func TestHTTP_CrearCarro_ComoAdmin_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros", jsonBody(crearCarroRequest{Nombre: "Carro 1"}))
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

func TestHTTP_CrearCarro_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros", jsonBody(crearCarroRequest{Nombre: "Carro 1"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ListarCarros_ComoDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["c1"] = &domain.Carro{ID: "c1", Nombre: "Carro 1"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/inventory/carros", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── PC ──────────────────────────────────────────────────────────────────

func TestHTTP_CrearPC_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/pcs",
		jsonBody(crearPCRequest{Identificador: 27, NumeroSerie: "5CD1234ABC"}))
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

// El número de serie es texto: sin esto, cargar la primera PC con el código
// que dice la etiqueta era imposible (ver migración 011). La respuesta trae
// la forma canónica, que puede no ser lo que se tipeó.
func TestHTTP_CrearPC_NumeroSerieAlfanumerico_SeNormaliza(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/pcs",
		jsonBody(crearPCRequest{Identificador: 27, NumeroSerie: " pf2k9l3m "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var creada pcResponse
	if err := json.NewDecoder(resp.Body).Decode(&creada); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if creada.NumeroSerie != "PF2K9L3M" {
		t.Errorf("esperaba la forma canónica PF2K9L3M, obtuve %q", creada.NumeroSerie)
	}
}

func TestHTTP_CrearPC_NumeroSerieVacio_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/pcs",
		jsonBody(crearPCRequest{Identificador: 27, NumeroSerie: "   "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearPC_IdentificadorInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/pcs",
		jsonBody(crearPCRequest{Identificador: -1, NumeroSerie: "5CD1234ABC"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstadoPC_ADisponible_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 1, Estado: domain.EstadoEnMantenimiento}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/pcs/pc1/estado",
		jsonBody(cambiarEstadoPCRequest{Estado: "DISPONIBLE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstadoPC_EstadoInvalido_400(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoDisponible}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/pcs/pc1/estado",
		jsonBody(cambiarEstadoPCRequest{Estado: "ROTA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstadoPC_DesdeFueraDeServicio_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/pcs/pc1/estado",
		jsonBody(cambiarEstadoPCRequest{Estado: "DISPONIBLE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_DarDeBajaPC_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("DELETE", "/api/inventory/pcs/pc1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// ── Incidencia ──────────────────────────────────────────────────────────

func TestHTTP_CrearIncidencia_ComoDocente_OK(t *testing.T) {
	// A diferencia de las mutaciones de Carro/PC, cualquier usuario
	// autenticado puede reportar una incidencia (RF-03.5).
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/incidencias",
		jsonBody(crearIncidenciaRequest{PCID: "pc1", Descripcion: "No enciende", Gravedad: "GRAVE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearIncidencia_GravedadInvalida_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/incidencias",
		jsonBody(crearIncidenciaRequest{PCID: "pc1", Descripcion: "No enciende", Gravedad: "CRITICA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearIncidencia_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/incidencias",
		jsonBody(crearIncidenciaRequest{PCID: "pc1", Descripcion: "No enciende", Gravedad: "GRAVE"}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_EditarIncidencia_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/inventory/incidencias/i1",
		jsonBody(editarIncidenciaRequest{MarcarEnviadaDGE: true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}
