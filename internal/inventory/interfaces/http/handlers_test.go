package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
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
	equipos     map[string]*domain.Equipo
	incidencias map[string]*domain.Incidencia
	licencias   map[string]*domain.LicenciaSoftware
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		carros:      make(map[string]*domain.Carro),
		equipos:     make(map[string]*domain.Equipo),
		incidencias: make(map[string]*domain.Incidencia),
		licencias:   make(map[string]*domain.LicenciaSoftware),
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
func (r *fakeRepo) CrearEquipo(ctx context.Context, equipo *domain.Equipo) error {
	r.equipos[equipo.ID] = equipo
	return nil
}
func (r *fakeRepo) BuscarEquipoPorID(ctx context.Context, id string) (*domain.Equipo, error) {
	equipo, ok := r.equipos[id]
	if !ok {
		return nil, application.ErrEquipoNoEncontrado
	}
	return equipo, nil
}
func (r *fakeRepo) GuardarEquipo(ctx context.Context, equipo *domain.Equipo) error {
	r.equipos[equipo.ID] = equipo
	return nil
}
func (r *fakeRepo) ListarEquiposPorCarro(ctx context.Context, carroID string) ([]*domain.Equipo, error) {
	var resultado []*domain.Equipo
	for _, equipo := range r.equipos {
		if equipo.CarroID == carroID {
			resultado = append(resultado, equipo)
		}
	}
	return resultado, nil
}

// ListarEquipos: el inventario, o solo lo que no está en ningún carro.
// El filtro se aplica acá igual que en la base: un fake más permisivo que el
// repositorio real hace pasar en la máquina lo que falla en producción.
func (r *fakeRepo) ListarEquipos(ctx context.Context, soloSueltos bool) ([]*domain.Equipo, error) {
	var resultado []*domain.Equipo
	for _, equipo := range r.equipos {
		if soloSueltos && equipo.EstaEnUnCarro() {
			continue
		}
		resultado = append(resultado, equipo)
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
func (r *fakeRepo) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.EquipoID == equipoID {
			resultado = append(resultado, i)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CategoriasDeFallaUsadas(ctx context.Context) ([]string, error) {
	vistas := map[string]bool{}
	var resultado []string
	for _, i := range r.incidencias {
		if i.Categoria == "" || vistas[strings.ToLower(i.Categoria)] {
			continue
		}
		vistas[strings.ToLower(i.Categoria)] = true
		resultado = append(resultado, i.Categoria)
	}
	sort.Strings(resultado)
	return resultado, nil
}

func (r *fakeRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	for _, existente := range r.licencias {
		if existente.EquipoID == l.EquipoID && strings.EqualFold(existente.Nombre, l.Nombre) {
			return application.ErrLicenciaDuplicada
		}
	}
	r.licencias[l.ID] = l
	return nil
}
func (r *fakeRepo) BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error) {
	l, ok := r.licencias[id]
	if !ok {
		return nil, application.ErrLicenciaNoEncontrada
	}
	return l, nil
}
func (r *fakeRepo) GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	if _, ok := r.licencias[l.ID]; !ok {
		return application.ErrLicenciaNoEncontrada
	}
	r.licencias[l.ID] = l
	return nil
}
func (r *fakeRepo) BorrarLicencia(ctx context.Context, id string) error {
	if _, ok := r.licencias[id]; !ok {
		return application.ErrLicenciaNoEncontrada
	}
	delete(r.licencias, id)
	return nil
}
func (r *fakeRepo) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	var resultado []*domain.LicenciaSoftware
	for _, l := range r.licencias {
		if l.EquipoID == equipoID {
			resultado = append(resultado, l)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) ListarLicencias(ctx context.Context) ([]*application.LicenciaConUbicacion, error) {
	var resultado []*application.LicenciaConUbicacion
	for _, l := range r.licencias {
		resultado = append(resultado, r.conUbicacion(l))
	}
	return resultado, nil
}
func (r *fakeRepo) ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*application.LicenciaConUbicacion, error) {
	return nil, nil
}
func (r *fakeRepo) MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error {
	return nil
}
func (r *fakeRepo) conUbicacion(l *domain.LicenciaSoftware) *application.LicenciaConUbicacion {
	u := &application.LicenciaConUbicacion{Licencia: l}
	if equipo, ok := r.equipos[l.EquipoID]; ok {
		u.Identificador = equipo.Identificador
		u.EquipoDadoDeBaja = equipo.DadoDeBaja
		u.CarroID = equipo.CarroID
		if carro, ok := r.carros[equipo.CarroID]; ok {
			u.CarroNombre = carro.Nombre
		}
	}
	return u
}

type fakeValidadorReservas struct{}

func (f *fakeValidadorReservas) CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID, motivo string) (int, int, error) {
	return 0, 0, nil
}

func (f *fakeValidadorReservas) TieneReservasFuturas(ctx context.Context, equipoID string) (bool, error) {
	return false, nil
}

func (f *fakeValidadorReservas) EstaPrestado(ctx context.Context, equipoID string) (bool, error) {
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

func TestHTTP_CrearEquipo_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/equipos",
		jsonBody(crearEquipoDeCarroRequest{Identificador: 27, NumeroSerie: "5CD1234ABC"}))
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

// El número de serie es texto: con un tipo numérico no se podría cargar el
// código que dice la etiqueta. La respuesta trae la forma canónica, que
// puede no ser lo que se tipeó.
func TestHTTP_CrearEquipo_NumeroSerieAlfanumerico_SeNormaliza(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/equipos",
		jsonBody(crearEquipoDeCarroRequest{Identificador: 27, NumeroSerie: " pf2k9l3m "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var creada equipoResponse
	if err := json.NewDecoder(resp.Body).Decode(&creada); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if creada.NumeroSerie != "PF2K9L3M" {
		t.Errorf("esperaba la forma canónica PF2K9L3M, obtuve %q", creada.NumeroSerie)
	}
}

func TestHTTP_CrearEquipo_NumeroSerieVacio_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/equipos",
		jsonBody(crearEquipoDeCarroRequest{Identificador: 27, NumeroSerie: "   "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearEquipo_IdentificadorInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/inventory/carros/c1/equipos",
		jsonBody(crearEquipoDeCarroRequest{Identificador: -1, NumeroSerie: "5CD1234ABC"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstadoEquipo_ADisponible_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 1, Estado: domain.EstadoEnMantenimiento}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "DISPONIBLE"}))
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

func TestHTTP_CambiarEstadoEquipo_EstadoInvalido_400(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoDisponible}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "ROTA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CambiarEstadoEquipo_DesdeFueraDeServicio_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/inventory/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "DISPONIBLE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_DarDeBajaEquipo_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("DELETE", "/api/inventory/equipos/pc1", nil)
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
		jsonBody(crearIncidenciaRequest{EquipoID: "pc1", Descripcion: "No enciende", Gravedad: "GRAVE"}))
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
		jsonBody(crearIncidenciaRequest{EquipoID: "pc1", Descripcion: "No enciende", Gravedad: "CRITICA"}))
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
		jsonBody(crearIncidenciaRequest{EquipoID: "pc1", Descripcion: "No enciende", Gravedad: "GRAVE"}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_EditarIncidencia_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/inventory/incidencias/i1",
		jsonBody(editarIncidenciaRequest{MarcarEnviadaASoporte: true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}
