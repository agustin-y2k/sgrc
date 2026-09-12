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
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/secretos"
	"github.com/ramiro/sgrc/internal/shared/texto"
)

// fakeAuditor descarta toda entrada de auditoría (ver el mismo tipo en
// internal/auth/interfaces/http/handlers_test.go).
type fakeAuditor struct{}

func (fakeAuditor) Registrar(ctx context.Context, e audit.Entrada) error { return nil }

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	preferenciasHuerfanas []*application.PreferenciaHuerfana
	carros                map[string]*domain.Carro
	equipos               map[string]*domain.Equipo
	incidencias           map[string]*domain.Incidencia
	licencias             map[string]*domain.LicenciaSoftware
	preferencias          map[string]*domain.PreferenciaDeEquipo
	cuentas               map[string]*domain.CuentaDeEquipo
	// nombresDeMateria es lo que ofrece el selector del formulario de marcas.
	nombresDeMateria []string
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		carros:       make(map[string]*domain.Carro),
		equipos:      make(map[string]*domain.Equipo),
		incidencias:  make(map[string]*domain.Incidencia),
		licencias:    make(map[string]*domain.LicenciaSoftware),
		preferencias: make(map[string]*domain.PreferenciaDeEquipo),
		cuentas:      make(map[string]*domain.CuentaDeEquipo),
	}
}

// ── fakeRepo: cuentas de equipo (RF-03.22) ──────────────────────────────

func (r *fakeRepo) CrearCuentaDeEquipo(_ context.Context, c *domain.CuentaDeEquipo) error {
	for _, existente := range r.cuentas {
		if existente.EquipoID == c.EquipoID && strings.EqualFold(existente.Usuario, c.Usuario) {
			return application.ErrCuentaDeEquipoDuplicada
		}
	}
	r.cuentas[c.ID] = c
	return nil
}

func (r *fakeRepo) BuscarCuentaDeEquipoPorID(_ context.Context, id string) (*domain.CuentaDeEquipo, error) {
	c, ok := r.cuentas[id]
	if !ok {
		return nil, application.ErrCuentaDeEquipoNoEncontrada
	}
	return c, nil
}

func (r *fakeRepo) GuardarCuentaDeEquipo(_ context.Context, c *domain.CuentaDeEquipo) error {
	if _, ok := r.cuentas[c.ID]; !ok {
		return application.ErrCuentaDeEquipoNoEncontrada
	}
	r.cuentas[c.ID] = c
	return nil
}

func (r *fakeRepo) BorrarCuentaDeEquipo(_ context.Context, id string) error {
	if _, ok := r.cuentas[id]; !ok {
		return application.ErrCuentaDeEquipoNoEncontrada
	}
	delete(r.cuentas, id)
	return nil
}

func (r *fakeRepo) ListarCuentasDeEquipo(_ context.Context, equipoID string) ([]*domain.CuentaDeEquipo, error) {
	var resultado []*domain.CuentaDeEquipo
	for _, c := range r.cuentas {
		if c.EquipoID == equipoID {
			resultado = append(resultado, c)
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].Usuario < resultado[j].Usuario })
	return resultado, nil
}

func (r *fakeRepo) ClasesDeCuentaUsadas(_ context.Context) ([]string, error) {
	vistas := map[string]bool{}
	var resultado []string
	for _, c := range r.cuentas {
		if !vistas[c.Clase] {
			vistas[c.Clase] = true
			resultado = append(resultado, c.Clase)
		}
	}
	sort.Strings(resultado)
	return resultado, nil
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
func (r *fakeRepo) ListarCarros(ctx context.Context, incluirRetirados bool) ([]*domain.Carro, error) {
	var resultado []*domain.Carro
	for _, c := range r.carros {
		if c.DadoDeBaja && !incluirRetirados {
			continue
		}
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
func (r *fakeRepo) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string, limite int) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.EquipoID == equipoID {
			resultado = append(resultado, i)
		}
	}
	// El recorte se imita para que el test del tope pueda probar algo. El orden
	// de la de verdad lo pone el ORDER BY; acá alcanza con no devolver más de lo
	// pedido.
	if limite > 0 && len(resultado) > limite {
		resultado = resultado[:limite]
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

func (r *fakeRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) (bool, error) {
	for _, existente := range r.licencias {
		if existente.EquipoID == l.EquipoID && strings.EqualFold(existente.Nombre, l.Nombre) {
			return false, nil
		}
	}
	r.licencias[l.ID] = l
	return true, nil
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

// ── Preferencias de materia (RF-03.21) ────────────────────────────────

func (r *fakeRepo) CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) (bool, error) {
	r.preferencias[p.ID] = p
	return true, nil
}

// EnTransaccion corre fn derecho: lo que este fake prueba es el contrato HTTP
// —códigos, forma del JSON, qué queda auditado—, no la atomicidad, que se
// verifica contra Postgres en infrastructure/.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(application.Repo) error) error {
	return fn(r)
}

func (r *fakeRepo) GuardarPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error {
	if _, ok := r.preferencias[p.ID]; !ok {
		return domain.ErrPreferenciaNoEncontr
	}
	r.preferencias[p.ID] = p
	return nil
}

func (r *fakeRepo) BuscarPreferenciaPorID(ctx context.Context, id string) (*domain.PreferenciaDeEquipo, error) {
	p, ok := r.preferencias[id]
	if !ok {
		return nil, domain.ErrPreferenciaNoEncontr
	}
	return p, nil
}

func (r *fakeRepo) BorrarPreferencia(ctx context.Context, id string) error {
	if _, ok := r.preferencias[id]; !ok {
		return domain.ErrPreferenciaNoEncontr
	}
	delete(r.preferencias, id)
	return nil
}

func (r *fakeRepo) ListarPreferenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	var resultado []*domain.PreferenciaDeEquipo
	for _, p := range r.preferencias {
		if p.EquipoID == equipoID {
			resultado = append(resultado, p)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) NombresDeMateriaEnUso(ctx context.Context) ([]string, error) {
	return r.nombresDeMateria, nil
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

func (r *fakeRepo) ContarPendientesDeRenovar(ctx context.Context, hoy time.Time) (int, error) {
	return 0, nil
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

// repoConCarro deja creado el carro que los tests de equipos dan por
// existente. Hizo falta al validar el destino de un equipo: antes se creaban
// máquinas en carros inexistentes, algo que la base real rechazaba igual por la
// clave foránea.
func repoConCarro(id string) *fakeRepo {
	repo := nuevoFakeRepo()
	repo.carros[id] = &domain.Carro{ID: id, Nombre: "Carro de prueba"}
	return repo
}

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	cifrador, err := secretos.Nuevo("clave-de-test-para-las-cuentas-de-equipo")
	if err != nil {
		panic(err)
	}
	svc := application.NewService(repo, &fakeValidadorReservas{}, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}, cifrador, eventbus.NewInMemoryEventBus())
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

// ── Carro ───────────────────────────────────────────────────────────────

func TestHTTP_CrearCarro_ComoAdmin_OK(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/carros", jsonBody(crearCarroRequest{Nombre: "Carro 1"}))
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

	req := httptest.NewRequest("POST", "/api/carros", jsonBody(crearCarroRequest{Nombre: "Carro 1"}))
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

	req := httptest.NewRequest("GET", "/api/carros", nil)
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
	app := nuevaAppDeTest(repoConCarro("c1"))

	req := httptest.NewRequest("POST", "/api/carros/c1/equipos",
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
// código que dice la etiqueta.
func TestHTTP_CrearEquipo_NumeroSerieAlfanumerico_SeNormaliza(t *testing.T) {
	app := nuevaAppDeTest(repoConCarro("c1"))

	req := httptest.NewRequest("POST", "/api/carros/c1/equipos",
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
	app := nuevaAppDeTest(repoConCarro("c1"))

	req := httptest.NewRequest("POST", "/api/carros/c1/equipos",
		jsonBody(crearEquipoDeCarroRequest{Identificador: 27, NumeroSerie: "   "}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CrearEquipo_IdentificadorInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(repoConCarro("c1"))

	req := httptest.NewRequest("POST", "/api/carros/c1/equipos",
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

	req := httptest.NewRequest("PATCH", "/api/equipos/pc1/estado",
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

	req := httptest.NewRequest("PATCH", "/api/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "ROTA"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

// Un equipo fuera de servicio que se arregla vuelve a circulación. Antes esto
// respondía 409 y la pantalla ofrecía el botón igual: se apretaba "Confirmar
// cambio" y no pasaba nada visible.
func TestHTTP_CambiarEstadoEquipo_DesdeFueraDeServicio_Vuelve(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "DISPONIBLE"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.equipos["pc1"].Estado != domain.EstadoDisponible {
		t.Errorf("estado = %s, esperaba DISPONIBLE", repo.equipos["pc1"].Estado)
	}
}

// Repetir el estado que ya se tiene sigue siendo 409: el pedido está bien
// formado, pero no es un cambio.
func TestHTTP_CambiarEstadoEquipo_AlMismoEstado_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/equipos/pc1/estado",
		jsonBody(cambiarEstadoEquipoRequest{Estado: "FUERA_DE_SERVICIO"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_DarDeBajaEquipo_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("DELETE", "/api/equipos/pc1", nil)
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

	req := httptest.NewRequest("POST", "/api/incidencias",
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

	req := httptest.NewRequest("POST", "/api/incidencias",
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

	req := httptest.NewRequest("POST", "/api/incidencias",
		jsonBody(crearIncidenciaRequest{EquipoID: "pc1", Descripcion: "No enciende", Gravedad: "GRAVE"}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_EditarIncidencia_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/incidencias/i1",
		jsonBody(editarIncidenciaRequest{MarcarEnviadaASoporte: true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// TestHTTP_EditarEquipo_ConEstado_400 fija el arreglo de una trampa real: el
// cuerpo de PATCH /equipos/{id} no tenía campo `estado`, así que mandarlo
// devolvía 200 y se descartaba en silencio. Quien escribía un script contra la
// API veía "salió bien" y la máquina seguía en el estado viejo.
//
// El estado tiene su propia ruta porque cambiarlo cancela las reservas que
// quedan sin máquina (RF-03.8): no es un campo más del formulario.
func TestHTTP_EditarEquipo_ConEstado_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/equipos/eq1",
		bytes.NewBufferString(`{"estado":"DISPONIBLE"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
	var cuerpo bytes.Buffer
	if _, err := cuerpo.ReadFrom(resp.Body); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if !strings.Contains(cuerpo.String(), "/estado") {
		t.Errorf("el mensaje tiene que decir a qué ruta ir, obtuve: %s", cuerpo.String())
	}
}

// TestHTTP_RevelarPassword_ClaveCambiada_409 es la otra mitad del arreglo: el
// servicio ya devolvía una explicación —"hay que volver a cargarla mirando el
// equipo"— y el mapeo la tiraba, porque no tenía caso para este error y caía
// al 500 genérico. En pantalla se leía "error interno" y en el log no quedaba
// nada.
func TestHTTP_RevelarPassword_ClaveCambiada_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Nombre: "Notebook 1"}
	// Lo guardado no corresponde con la clave que corre: es lo que deja un
	// cambio de CUENTAS_SECRET.
	repo.cuentas["cu1"] = &domain.CuentaDeEquipo{
		ID: "cu1", EquipoID: "eq1", Usuario: "alumno", Clase: "Local",
		Privilegio: domain.PrivilegioComun, Visibilidad: domain.VisibilidadPublica,
		TienePassword: true, PasswordCifrada: "esto-no-lo-descifra-ninguna-clave",
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/cuentas/cu1/password", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}

	var cuerpo bytes.Buffer
	if _, err := cuerpo.ReadFrom(resp.Body); err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	if strings.Contains(cuerpo.String(), "error interno") {
		t.Errorf("volvió el mensaje genérico: %s", cuerpo.String())
	}
	// Lo que importa no es el diagnóstico sino qué hacer con él.
	if !strings.Contains(cuerpo.String(), "volver a cargarla") {
		t.Errorf("la respuesta tiene que decir cuál es la salida, obtuve: %s", cuerpo.String())
	}
}

func (r *fakeRepo) BuscarLicenciasPorIDs(ctx context.Context, ids []string) (map[string]*domain.LicenciaSoftware, error) {
	licencias := make(map[string]*domain.LicenciaSoftware, len(ids))
	for _, id := range ids {
		if l, existe := r.licencias[id]; existe {
			licencias[id] = l
		}
	}
	return licencias, nil
}

func (r *fakeRepo) ListarPreferenciasHuerfanas(ctx context.Context) ([]*application.PreferenciaHuerfana, error) {
	// Huérfana = su nombre de materia no cruza con ninguna materia cargada. El
	// fake no tiene tabla de materias, así que devuelve lo que el test le dejó
	// preparado.
	return r.preferenciasHuerfanas, nil
}

func (r *fakeRepo) ContarPreferenciasQueDejarianDeAplicar(ctx context.Context, materiaNombre string) (int, error) {
	n := 0
	for _, p := range r.preferencias {
		if texto.SonElMismo(p.MateriaNombre, materiaNombre) {
			n++
		}
	}
	return n, nil
}

// ── Reactivar: deshacer una baja ────────────────────────────────────────

func TestHTTP_ReactivarEquipo_OK(t *testing.T) {
	repo := repoConCarro("c1")
	baja := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 7, DadoDeBaja: true, FechaBaja: &baja}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/equipos/pc1/reactivar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.equipos["pc1"].DadoDeBaja {
		t.Error("el equipo tenía que volver al inventario")
	}
}

func TestHTTP_ReactivarEquipo_ComoDocente_403(t *testing.T) {
	repo := repoConCarro("c1")
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 7, DadoDeBaja: true}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/equipos/pc1/reactivar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// Reactivar algo que está en circulación es un 409, no un 200 silencioso:
// quien lo pide cree que está arreglando algo.
func TestHTTP_ReactivarEquipo_QueNoEstabaDeBaja_409(t *testing.T) {
	repo := repoConCarro("c1")
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 7}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/equipos/pc1/reactivar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ReactivarCarro_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["c1"] = &domain.Carro{ID: "c1", Nombre: "Carro 1", DadoDeBaja: true}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/carros/c1/reactivar", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.carros["c1"].DadoDeBaja {
		t.Error("el carro tenía que volver a circulación")
	}
}

// Los carros retirados sólo salen si los pide un Admin: para el docente el
// listado alimenta el selector de dónde va un equipo, y un carro que ya no
// existe no es un destino.
func TestHTTP_ListarCarros_IncluirRetirados_SoloParaAdmin(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["vivo"] = &domain.Carro{ID: "vivo", Nombre: "Carro 1"}
	repo.carros["retirado"] = &domain.Carro{ID: "retirado", Nombre: "Carro viejo", DadoDeBaja: true}
	app := nuevaAppDeTest(repo)

	casos := []struct {
		nombre   string
		rol      string
		esperado int
	}{
		{"el Admin los ve", "ADMIN", 2},
		{"el docente no", "DOCENTE", 1},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/carros?incluirRetirados=true", nil)
			req.Header.Set("Authorization", "Bearer "+tokenPara("u1", c.rol))

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
			}

			var cuerpo struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
				t.Fatalf("decodificando: %v", err)
			}
			if len(cuerpo.Data) != c.esperado {
				t.Errorf("esperaba %d carros, obtuve %d", c.esperado, len(cuerpo.Data))
			}
		})
	}
}

// ── GET de un recurso solo ──────────────────────────────────────────────

func TestHTTP_ObtenerRecursoIndividual(t *testing.T) {
	repo := repoConCarro("c1")
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 7, Estado: "DISPONIBLE"}
	repo.incidencias["i1"] = &domain.Incidencia{ID: "i1", EquipoID: "pc1", Descripcion: "No arranca", Estado: "ABIERTA", Gravedad: "GRAVE"}
	app := nuevaAppDeTest(repo)

	casos := []struct {
		nombre, ruta, rol string
	}{
		{"un carro", "/api/carros/c1", "DOCENTE"},
		{"un equipo", "/api/equipos/pc1", "DOCENTE"},
		{"una incidencia", "/api/incidencias/i1", "DOCENTE"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest("GET", c.ruta, nil)
			req.Header.Set("Authorization", "Bearer "+tokenPara("u1", c.rol))

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
			}
		})
	}
}

func TestHTTP_ObtenerEquipo_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/equipos/no-existe", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

// Las licencias son de Admin también de a una: el listado lo es, y un GET
// individual que no lo fuera sería una puerta de atrás al mismo dato.
func TestHTTP_ObtenerLicencia_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/licencias/l1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("d1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// El riesgo que introduce agregar /preferencias/{id}: `huerfanas` es un
// literal en la misma posición, y si la ruta con parámetro se registrara
// primero se lo comería. El orden de registro es lo único que lo impide, así
// que se fija acá.
func TestHTTP_PreferenciasHuerfanas_NoLaComeLaRutaConParametro(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/preferencias/huerfanas", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// Si la resolviera /preferencias/:id, "huerfanas" sería un id inexistente
	// y contestaría 404 en vez de la lista.
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200 de la lista de huérfanas, obtuve %d", resp.StatusCode)
	}
}

// El Location de un 201 sólo sirve si se puede seguir. Esto crea un carro,
// lee la cabecera y pide ESA dirección: si el prefijo que publica el handler y
// la ruta que registra el router se separan alguna vez, el segundo pedido da
// 404 y el test lo dice.
func TestHTTP_LocationDelCreado_SePuedeSeguir(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	crear := httptest.NewRequest("POST", "/api/carros",
		strings.NewReader(`{"nombre":"Carro nuevo"}`))
	crear.Header.Set("Content-Type", "application/json")
	crear.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp, err := app.Test(crear)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	ubicacion := resp.Header.Get("Location")
	if ubicacion == "" {
		t.Fatal("un 201 sin Location no dice dónde quedó lo que creó")
	}

	seguir := httptest.NewRequest("GET", ubicacion, nil)
	seguir.Header.Set("Authorization", "Bearer "+tokenPara("a1", "ADMIN"))

	resp2, err := app.Test(seguir)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("el Location %q tenía que devolver el carro, dio %d", ubicacion, resp2.StatusCode)
	}
}
