package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/notification/application"
	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/authtest"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type fakeRepo struct {
	notificaciones map[string]*domain.Notificacion
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{notificaciones: make(map[string]*domain.Notificacion)}
}
func (r *fakeRepo) Crear(ctx context.Context, n *domain.Notificacion) error {
	r.notificaciones[n.ID] = n
	return nil
}
func (r *fakeRepo) BuscarPorID(ctx context.Context, id string) (*domain.Notificacion, error) {
	n, ok := r.notificaciones[id]
	if !ok {
		return nil, application.ErrNotificacionNoEncontrada
	}
	return n, nil
}
func (r *fakeRepo) Guardar(ctx context.Context, n *domain.Notificacion) error {
	r.notificaciones[n.ID] = n
	return nil
}
func (r *fakeRepo) ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error) {
	var resultado []*domain.Notificacion
	for _, id := range r.idsOrdenados() {
		n := r.notificaciones[id]
		if n.UsuarioID != usuarioID {
			continue
		}
		if filtroEstado != nil && n.Estado != *filtroEstado {
			continue
		}
		resultado = append(resultado, n)
	}

	total := len(resultado)
	desde := pagina.Offset()
	if desde >= total {
		return nil, total, nil
	}
	hasta := desde + pagina.Limit()
	if hasta > total {
		hasta = total
	}
	return resultado[desde:hasta], total, nil
}

// idsOrdenados da un orden estable donde el repo real ordena por fecha: sobre
// el map pelado, LIMIT/OFFSET devolvería una página distinta en cada corrida.
func (r *fakeRepo) idsOrdenados() []string {
	ids := make([]string, 0, len(r.notificaciones))
	for id := range r.notificaciones {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *fakeRepo) ListarNoLeidasSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) ([]*domain.Notificacion, error) {
	// Estos tests verifican el contrato HTTP; el cierre de avisos se prueba
	// en application/.
	return nil, nil
}

type fakeListadorAdmins struct{}

func (f *fakeListadorAdmins) IDsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeListadorAdmins) EmailsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	return nil, nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeListadorAdmins{}, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	})
	h := NewHandler(svc)

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

// ── ListarPropias ───────────────────────────────────────────────────────

func TestHTTP_ListarPropias_SoloLasDeUnoMismo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "u1", Mensaje: "mía"}
	repo.notificaciones["n2"] = &domain.Notificacion{ID: "n2", UsuarioID: "u2", Mensaje: "de otro"}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/notifications/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body struct {
		Data []notificacionResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Data) != 1 || body.Data[0].ID != "n1" {
		t.Fatalf("esperaba solo n1, obtuve %+v", body.Data)
	}
}

// Cada cancelación en cascada le deja una fila a cada docente afectado y
// nada las borra, así que la campana crece sola durante todo el ciclo.
func TestHTTP_ListarPropias_PaginaYTotal(t *testing.T) {
	repo := nuevoFakeRepo()
	for _, id := range []string{"n1", "n2", "n3", "n4", "n5"} {
		repo.notificaciones[id] = &domain.Notificacion{ID: id, UsuarioID: "u1", Mensaje: id}
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/notifications/?page=3&pageSize=2", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body listarNotificacionesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "n5" {
		t.Fatalf("esperaba solo n5 en la última página, obtuve %+v", body.Data)
	}
	if body.Meta.Total != 5 || body.Meta.Page != 3 || body.Meta.PageSize != 2 {
		t.Errorf("meta incorrecta: %+v", body.Meta)
	}
}

func TestHTTP_ListarPropias_PaginacionInvalida_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	for _, query := range []string{"?page=-1", "?pageSize=0", "?pageSize=100000"} {
		req := httptest.NewRequest("GET", "/api/notifications/"+query, nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

		resp, _ := app.Test(req)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("%s: esperaba 400, obtuve %d", query, resp.StatusCode)
		}
	}
}

func TestHTTP_ListarPropias_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/notifications/", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ListarPropias_FiltroEstadoInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/notifications/?estado=ARCHIVADA", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

// ── MarcarLeida — el foco: titularidad ──────────────────────────────────

func TestHTTP_MarcarLeida_Propietario_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "u1", Estado: domain.NoLeida}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/notifications/n1/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.notificaciones["n1"].Estado != domain.Leida {
		t.Error("la notificación debería quedar marcada como leída")
	}
}

func TestHTTP_MarcarLeida_OtroUsuario_403(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "dueño", Estado: domain.NoLeida}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/notifications/n1/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro-usuario", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
	if repo.notificaciones["n1"].Estado != domain.NoLeida {
		t.Error("no debería haberse marcado como leída")
	}
}

func TestHTTP_MarcarLeida_ComoAdmin_NoTieneExcepcion_403(t *testing.T) {
	// A diferencia de otras rutas del proyecto, acá un Admin NO tiene pase libre
	// — no hay ninguna razón de negocio para que un Admin marque como leída la
	// notificación de otra persona.
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "dueño", Estado: domain.NoLeida}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/notifications/n1/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403 incluso para un Admin, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_MarcarLeida_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/notifications/no-existe/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_MarcarLeida_YaLeida_409(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "u1", Estado: domain.Leida}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PATCH", "/api/notifications/n1/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}
}

func (r *fakeRepo) MarcarTodasLeidasDe(ctx context.Context, usuarioID string, ahora time.Time) (int, error) {
	n := 0
	for _, notif := range r.notificaciones {
		if notif.UsuarioID == usuarioID && notif.Estado == domain.NoLeida {
			if err := notif.MarcarLeida(ahora); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}
