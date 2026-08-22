package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
	"github.com/ramiro/sgrc/internal/shared/audit"
	"github.com/ramiro/sgrc/internal/shared/authtest"
)

// ── fakeRepo — misma semántica de titularidad-acotada-por-usuario_id que
// application/service_test.go, reimplementada acá porque interfaces/http no
// puede importar los tipos no exportados de otro paquete de test.

type fakeRepo struct {
	jornada map[string]*domain.BloqueJornada

	bloques     map[string]*domain.BloqueHorario
	excepciones map[string]*domain.Excepcion

	llamadasBloquesEnLote     int
	llamadasExcepcionesEnLote int
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		jornada:     make(map[string]*domain.BloqueJornada),
		bloques:     make(map[string]*domain.BloqueHorario),
		excepciones: make(map[string]*domain.Excepcion),
	}
}

func claveExcepcion(usuarioID string, fecha time.Time) string {
	return usuarioID + "|" + fecha.Format("2006-01-02")
}

func (r *fakeRepo) ListarBloquesDeUsuario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error) {
	var resultado []*domain.BloqueHorario
	for _, b := range r.bloques {
		if b.UsuarioID == usuarioID {
			resultado = append(resultado, b)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CrearBloque(ctx context.Context, b *domain.BloqueHorario) error {
	r.bloques[b.ID] = b
	return nil
}

func (r *fakeRepo) BuscarBloqueDeUsuario(ctx context.Context, id, usuarioID string) (*domain.BloqueHorario, error) {
	b, ok := r.bloques[id]
	if !ok || b.UsuarioID != usuarioID {
		return nil, application.ErrBloqueNoEncontrado
	}
	return b, nil
}

func (r *fakeRepo) GuardarBloque(ctx context.Context, b *domain.BloqueHorario) error {
	existente, ok := r.bloques[b.ID]
	if !ok || existente.UsuarioID != b.UsuarioID {
		return application.ErrBloqueNoEncontrado
	}
	r.bloques[b.ID] = b
	return nil
}

func (r *fakeRepo) EliminarBloqueDeUsuario(ctx context.Context, id, usuarioID string) error {
	b, ok := r.bloques[id]
	if !ok || b.UsuarioID != usuarioID {
		return application.ErrBloqueNoEncontrado
	}
	delete(r.bloques, id)
	return nil
}

func (r *fakeRepo) BuscarExcepcionDeFecha(ctx context.Context, usuarioID string, fecha time.Time) (*domain.Excepcion, error) {
	e, ok := r.excepciones[claveExcepcion(usuarioID, fecha)]
	if !ok {
		return nil, nil
	}
	return e, nil
}

// Las versiones en lote se implementan reusando las individuales: lo que se
// prueba en application/ es que el servicio arme bien el resultado, no el SQL
// —eso vive en infrastructure/ y va contra Postgres real—.
func (r *fakeRepo) ListarBloquesDeUsuarios(ctx context.Context, usuarioIDs []string) (map[string][]*domain.BloqueHorario, error) {
	r.llamadasBloquesEnLote++
	resultado := make(map[string][]*domain.BloqueHorario, len(usuarioIDs))
	for _, id := range usuarioIDs {
		bloques, err := r.ListarBloquesDeUsuario(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(bloques) > 0 {
			resultado[id] = bloques
		}
	}
	return resultado, nil
}

func (r *fakeRepo) BuscarExcepcionesDeFecha(ctx context.Context, usuarioIDs []string, fecha time.Time) (map[string]*domain.Excepcion, error) {
	r.llamadasExcepcionesEnLote++
	resultado := make(map[string]*domain.Excepcion, len(usuarioIDs))
	for _, id := range usuarioIDs {
		e, err := r.BuscarExcepcionDeFecha(ctx, id, fecha)
		if err != nil {
			return nil, err
		}
		if e != nil {
			resultado[id] = e
		}
	}
	return resultado, nil
}

func (r *fakeRepo) GuardarExcepcion(ctx context.Context, e *domain.Excepcion) error {
	r.excepciones[claveExcepcion(e.UsuarioID, e.Fecha)] = e
	return nil
}

type fakeListadorAdmins struct {
	admins []application.AdminInfo
}

func (f *fakeListadorAdmins) AdminsAprobados(ctx context.Context) ([]application.AdminInfo, error) {
	return f.admins, nil
}

func idSecuencial() func() string {
	contador := 0
	return func() string {
		contador++
		return "id-" + string(rune('0'+contador))
	}
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	return nuevaAppDeTestCon(repo, &fakeReservas{}, &auditorEspia{})
}

func nuevaAppDeTestCon(repo *fakeRepo, reservas *fakeReservas, auditor *auditorEspia) *fiber.App {
	svc := application.NewService(repo, &fakeListadorAdmins{}, reservas, idSecuencial(), func() time.Time {
		// lunes 9-mar-2026, 10:00 — coherente con los bloques LUNES 08-12
		// que usan los tests de DisponibilidadDeAdmins.
		return time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
	})
	h := NewHandler(svc, auditor)

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

func conBody(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// ── GET /admins — cualquier usuario autenticado ────────────────────────

func TestHTTP_DisponibilidadDeAdmins_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/availability/admins", nil))
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_DisponibilidadDeAdmins_ComoDocente_200(t *testing.T) {
	// RF-07.2: cualquier usuario autenticado (docentes incluidos) puede ver esta
	// lista — a diferencia del resto de las rutas, que son solo para Admin sobre
	// su propio horario.
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/availability/admins", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
}

// ── GET /mi-horario — Admin ─────────────────────────────────────────────

func TestHTTP_MiHorario_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/availability/mi-horario", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_MiHorario_ComoAdmin_SoloLosPropios(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "admin1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	repo.bloques["b2"] = &domain.BloqueHorario{ID: "b2", UsuarioID: "otro-admin", DiaSemana: domain.Martes, HoraInicio: 9 * time.Hour, HoraFin: 10 * time.Hour}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("GET", "/api/availability/mi-horario", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var body struct {
		Data []bloqueResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Data) != 1 || body.Data[0].ID != "b1" {
		t.Fatalf("esperaba solo b1, obtuve %+v", body.Data)
	}
}

// ── POST /mi-horario — Admin ─────────────────────────────────────────────

func TestHTTP_AgregarBloque_OK_201(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/availability/mi-horario",
		conBody(bloqueRequest{DiaSemana: "LUNES", HoraInicio: "08:00", HoraFin: "12:00"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var body bloqueResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.DiaSemana != "LUNES" || body.HoraInicio != "08:00" || body.HoraFin != "12:00" {
		t.Errorf("respuesta incorrecta: %+v", body)
	}
	if len(repo.bloques) != 1 {
		t.Errorf("esperaba 1 bloque persistido, hay %d", len(repo.bloques))
	}
}

func TestHTTP_AgregarBloque_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-horario",
		conBody(bloqueRequest{DiaSemana: "LUNES", HoraInicio: "08:00", HoraFin: "12:00"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_AgregarBloque_DiaInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-horario",
		conBody(bloqueRequest{DiaSemana: "FERIADO", HoraInicio: "08:00", HoraFin: "12:00"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_AgregarBloque_RangoHorarioInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-horario",
		conBody(bloqueRequest{DiaSemana: "LUNES", HoraInicio: "12:00", HoraFin: "08:00"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_AgregarBloque_HoraConFormatoInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-horario",
		conBody(bloqueRequest{DiaSemana: "LUNES", HoraInicio: "8am", HoraFin: "12:00"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

// ── PATCH /mi-horario/{id} — titularidad acotada en el repo ────────────

func TestHTTP_EditarBloque_Propio_200(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "admin1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	app := nuevaAppDeTest(repo)

	nuevoDia := "MARTES"
	req := httptest.NewRequest("PATCH", "/api/availability/mi-horario/b1",
		conBody(editarBloqueRequest{DiaSemana: &nuevoDia}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if repo.bloques["b1"].DiaSemana != domain.Martes {
		t.Errorf("no se actualizó el bloque: %+v", repo.bloques["b1"])
	}
}

func TestHTTP_EditarBloque_DeOtroAdmin_404(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "dueño", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	app := nuevaAppDeTest(repo)

	nuevoDia := "MARTES"
	req := httptest.NewRequest("PATCH", "/api/availability/mi-horario/b1",
		conBody(editarBloqueRequest{DiaSemana: &nuevoDia}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro-admin", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404 (titularidad ajena tratada como inexistente), obtuve %d", resp.StatusCode)
	}
	if repo.bloques["b1"].DiaSemana != domain.Lunes {
		t.Error("el bloque del dueño real no debería haberse tocado")
	}
}

// ── DELETE /mi-horario/{id} ──────────────────────────────────────────────

func TestHTTP_EliminarBloque_Propio_200(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "admin1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("DELETE", "/api/availability/mi-horario/b1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if _, existe := repo.bloques["b1"]; existe {
		t.Error("el bloque debería haberse eliminado")
	}
}

func TestHTTP_EliminarBloque_DeOtroAdmin_404(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "dueño", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("DELETE", "/api/availability/mi-horario/b1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("otro-admin", "ADMIN"))

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", resp.StatusCode)
	}
	if _, existe := repo.bloques["b1"]; !existe {
		t.Error("el bloque del dueño real no debería haberse eliminado")
	}
}

// ── POST /mi-excepcion — Admin ────────────────────────────────────────────

func TestHTTP_CargarExcepcion_NoDisponible_201(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-excepcion",
		conBody(excepcionRequest{Fecha: "2026-03-09", Tipo: "NO_DISPONIBLE"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CargarExcepcion_HorarioModificadoSinHoras_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-excepcion",
		conBody(excepcionRequest{Fecha: "2026-03-09", Tipo: "HORARIO_MODIFICADO"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_CargarExcepcion_HorarioModificado_201(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())
	horaInicio, horaFin := "09:00", "11:00"

	req := httptest.NewRequest("POST", "/api/availability/mi-excepcion",
		conBody(excepcionRequest{Fecha: "2026-03-09", Tipo: "HORARIO_MODIFICADO", HoraInicio: &horaInicio, HoraFin: &horaFin}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var body excepcionResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.HoraInicio == nil || *body.HoraInicio != "09:00" {
		t.Errorf("horaInicio incorrecta en la respuesta: %+v", body)
	}
}

func TestHTTP_CargarExcepcion_ComoDocente_403(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("POST", "/api/availability/mi-excepcion",
		conBody(excepcionRequest{Fecha: "2026-03-09", Tipo: "NO_DISPONIBLE"}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
}

// ── POST /no-disponible-ahora — Admin ─────────────────────────────────────

func TestHTTP_MarcarNoDisponibleAhora_201(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("POST", "/api/availability/no-disponible-ahora", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d", resp.StatusCode)
	}

	var body excepcionResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Tipo != "NO_DISPONIBLE" {
		t.Errorf("esperaba tipo NO_DISPONIBLE, obtuve %+v", body)
	}
}

// ── Integración local: la excepción de hoy pisa el bloque en /admins ────

func TestHTTP_DisponibilidadDeAdmins_ExcepcionDeHoyPisaElBloque(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.bloques["b1"] = &domain.BloqueHorario{ID: "b1", UsuarioID: "admin1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour}
	// "ahora" en nuevaAppDeTest está fijado a lunes 9-mar-2026 10:00 —
	// dentro del bloque, así que sin excepción admin1 daría disponible.
	repo.excepciones[claveExcepcion("admin1", time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC))] =
		&domain.Excepcion{ID: "e1", UsuarioID: "admin1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC), Tipo: domain.NoDisponible}

	svc := application.NewService(repo, &fakeListadorAdmins{admins: []application.AdminInfo{{ID: "admin1", Nombre: "Ada", Apellido: "Lovelace"}}}, &fakeReservas{}, idSecuencial(), func() time.Time {
		return time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
	})
	app := fiber.New()
	RegisterRoutes(app, NewHandler(svc, &auditorEspia{}), registroDePrueba.Autenticacion(testSecret))

	req := httptest.NewRequest("GET", "/api/availability/admins", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var body struct {
		Data []adminDisponibilidadResponse `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Data) != 1 {
		t.Fatalf("esperaba 1 admin, obtuve %d", len(body.Data))
	}
	if body.Data[0].DisponibleAhora {
		t.Error("la excepción NO_DISPONIBLE de hoy debería pisar el bloque semanal y dar no disponible")
	}
	if body.Data[0].ExcepcionHoy == nil {
		t.Error("debería venir la excepción de hoy en la respuesta")
	}
}

// ── Jornada de la institución ──────────────────────────────────────────

// La jornada se manda entera, así que un PUT con dos tramos deja exactamente
// esos dos: es el turno mañana y el turno noche de la misma escuela.
func TestHTTP_ReemplazarJornada_200(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.jornada["viejo"] = &domain.BloqueJornada{
		ID: "viejo", DiaSemana: domain.Sabado, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: []bloqueRequest{
		{DiaSemana: "LUNES", HoraInicio: "07:00", HoraFin: "12:00"},
		{DiaSemana: "LUNES", HoraInicio: "20:00", HoraFin: "01:00"},
	}}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if len(repo.jornada) != 2 {
		t.Fatalf("esperaba exactamente los dos tramos mandados, quedaron %d", len(repo.jornada))
	}
	// El sábado que ya no viene en el pedido tiene que haberse ido.
	for _, b := range repo.jornada {
		if b.DiaSemana == domain.Sabado {
			t.Error("lo que no viene en el PUT se borra")
		}
	}
}

// Un cuerpo sin tramos es la institución eligiendo no restringir nada. No es
// un pedido incompleto y no se rechaza.
func TestHTTP_ReemplazarJornada_Vacia_200(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.jornada["viejo"] = &domain.BloqueJornada{
		ID: "viejo", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	if len(repo.jornada) != 0 {
		t.Errorf("la jornada tenía que quedar vacía, quedó %+v", repo.jornada)
	}
}

func TestHTTP_ReemplazarJornada_Docente_403(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.jornada["j1"] = &domain.BloqueJornada{
		ID: "j1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
	}
	app := nuevaAppDeTest(repo)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: []bloqueRequest{
		{DiaSemana: "LUNES", HoraInicio: "07:00", HoraFin: "23:00"},
	}}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", resp.StatusCode)
	}
	if repo.jornada["j1"].HoraFin != 12*time.Hour {
		t.Error("la jornada no debería haberse tocado")
	}
}

// Un cambio que deja clases afuera se rechaza con el detalle adentro y sin
// tocar nada. El mismo endpoint hace de previsualización: no hay forma de que
// lo que se muestra difiera de lo que después se aplica.
func TestHTTP_ReemplazarJornada_ConImpacto_409YNoGuarda(t *testing.T) {
	repo := nuevoFakeRepo()
	reservas := &fakeReservas{futuras: []application.ReservaFutura{{
		ID: "r1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
		HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour,
		Equipo: "PC 3", Materia: "Matemáticas", Docente: "Ada Lovelace",
	}}}
	app := nuevaAppDeTestCon(repo, reservas, &auditorEspia{})

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: []bloqueRequest{
		{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"},
	}}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", resp.StatusCode)
	}

	var cuerpo struct {
		Impacto struct {
			Reservas []struct {
				ID      string `json:"id"`
				Docente string `json:"docente"`
			} `json:"reservas"`
		} `json:"impacto"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(cuerpo.Impacto.Reservas) != 1 || cuerpo.Impacto.Reservas[0].ID != "r1" {
		t.Errorf("el detalle tiene que nombrar la reserva: %+v", cuerpo.Impacto.Reservas)
	}
	// Con el nombre del docente: un número suelto no deja ver a quién le cae.
	if cuerpo.Impacto.Reservas[0].Docente != "Ada Lovelace" {
		t.Errorf("falta el docente: %+v", cuerpo.Impacto.Reservas[0])
	}
	if len(repo.jornada) != 0 {
		t.Errorf("un 409 no puede haber guardado nada: %+v", repo.jornada)
	}
	if len(reservas.canceladas) != 0 {
		t.Errorf("un 409 no puede haber cancelado nada: %v", reservas.canceladas)
	}
}

func TestHTTP_ReemplazarJornada_Confirmada_200YCancela(t *testing.T) {
	repo := nuevoFakeRepo()
	reservas := &fakeReservas{futuras: []application.ReservaFutura{{
		ID: "r1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
		HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour, Equipo: "PC 3",
	}}}
	app := nuevaAppDeTestCon(repo, reservas, &auditorEspia{})

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{
		Tramos:     []bloqueRequest{{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"}},
		Confirmado: true,
	}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}

	var cuerpo map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if cuerpo["reservasCanceladas"] != float64(1) {
		t.Errorf("tenía que informar la cancelación: %v", cuerpo["reservasCanceladas"])
	}
	if len(reservas.canceladas) != 1 {
		t.Errorf("se tenía que cancelar r1: %v", reservas.canceladas)
	}
}

// La acción administrativa de mayor alcance del sistema tiene que dejar
// rastro de quién la disparó y de cuánto se llevó puesto. Es lo mismo que
// pide docs/09-seguridad-rbac.md §5 para el archivado de un ciclo.
func TestHTTP_ReemplazarJornada_QuedaAuditada(t *testing.T) {
	repo := nuevoFakeRepo()
	reservas := &fakeReservas{futuras: []application.ReservaFutura{{
		ID: "r1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
		HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour, Equipo: "PC 3",
	}}}
	auditor := &auditorEspia{}
	app := nuevaAppDeTestCon(repo, reservas, auditor)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{
		Tramos:     []bloqueRequest{{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"}},
		Confirmado: true,
	}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	if _, err := app.Test(req); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	entradas := auditor.de("JORNADA_CAMBIADA")
	if len(entradas) != 1 {
		t.Fatalf("esperaba una entrada de auditoría, hubo %d", len(entradas))
	}
	if entradas[0].UsuarioID != "admin1" {
		t.Errorf("tiene que quedar QUIÉN lo hizo: %q", entradas[0].UsuarioID)
	}
	// El conteo es el dato que hace útil el registro: sin él la entrada dice
	// que alguien cambió el horario, no que canceló las clases de la escuela.
	if entradas[0].Detalle["reservasCanceladas"] != 1 {
		t.Errorf("tiene que quedar CUÁNTO se canceló: %+v", entradas[0].Detalle)
	}
}

// Un cambio que no se aplicó no se audita: el 409 es una pregunta, no un
// hecho, y un registro lleno de intentos esconde los cambios de verdad.
func TestHTTP_ReemplazarJornada_SinConfirmar_NoSeAudita(t *testing.T) {
	repo := nuevoFakeRepo()
	reservas := &fakeReservas{futuras: []application.ReservaFutura{{
		ID: "r1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
		HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour, Equipo: "PC 3",
	}}}
	auditor := &auditorEspia{}
	app := nuevaAppDeTestCon(repo, reservas, auditor)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: []bloqueRequest{
		{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"},
	}}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	if _, err := app.Test(req); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(auditor.de("JORNADA_CAMBIADA")) != 0 {
		t.Error("una previsualización no cambió nada: no hay qué auditar")
	}
}

// El tope existe sobre todo por lo que pasa DESPUÉS: la jornada se lee entera
// en cada alta de reserva, así que una inflada no molesta una vez sino todos
// los días.
func TestHTTP_ReemplazarJornada_DemasiadosTramos_400(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	// Tramos de un minuto, todos distintos: no se solapan, así que lo único
	// que puede frenarlos es el tope.
	muchos := make([]bloqueRequest, application.MaxTramosDeJornada+1)
	for i := range muchos {
		muchos[i] = bloqueRequest{
			DiaSemana:  "LUNES",
			HoraInicio: fmt.Sprintf("%02d:%02d", i/60, i%60),
			HoraFin:    fmt.Sprintf("%02d:%02d", (i+1)/60, (i+1)%60),
		}
	}

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: muchos}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", resp.StatusCode)
	}
	if len(repo.jornada) != 0 {
		t.Error("un pedido rechazado no puede dejar nada escrito")
	}
}

// La lista del 409 se recorta, pero el número no: es el número el que decide,
// y recortarlo haría que el Admin confirmara creyendo que cancela menos.
func TestHTTP_ReemplazarJornada_LaListaSeRecortaPeroElConteoNo(t *testing.T) {
	repo := nuevoFakeRepo()
	cuantas := MaxAfectadasEnLaRespuesta + 20
	futuras := make([]application.ReservaFutura, cuantas)
	for i := range futuras {
		futuras[i] = application.ReservaFutura{
			ID:    fmt.Sprintf("r%d", i),
			Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
			// 13:00–15:00, fuera del tramo 16:00–18:00 que se va a declarar.
			HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour, Equipo: "PC 3",
		}
	}
	app := nuevaAppDeTestCon(repo, &fakeReservas{futuras: futuras}, &auditorEspia{})

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{Tramos: []bloqueRequest{
		{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"},
	}}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var cuerpo struct {
		Impacto struct {
			Reservas       []map[string]any `json:"reservas"`
			TotalAfectadas int              `json:"totalAfectadas"`
		} `json:"impacto"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(cuerpo.Impacto.Reservas) != MaxAfectadasEnLaRespuesta {
		t.Errorf("la lista tenía que recortarse en %d, vinieron %d",
			MaxAfectadasEnLaRespuesta, len(cuerpo.Impacto.Reservas))
	}
	if cuerpo.Impacto.TotalAfectadas != cuantas {
		t.Errorf("el conteo tiene que ser el real (%d), vino %d", cuantas, cuerpo.Impacto.TotalAfectadas)
	}
}

// La cascada a medias deja la jornada nueva puesta. El mensaje tiene que
// decirlo: con un "error interno" genérico el Admin cree que no pasó nada y
// el horario de la escuela cambió sin que lo sepa.
func TestHTTP_ReemplazarJornada_CascadaAMedias_LoDiceYLoAudita(t *testing.T) {
	repo := nuevoFakeRepo()
	reservas := &fakeReservas{
		futuras: []application.ReservaFutura{{
			ID: "r1", Fecha: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
			HoraInicio: 13 * time.Hour, HoraFin: 15 * time.Hour, Equipo: "PC 3",
		}},
		errCancelar: errors.New("la base se cayó"),
	}
	auditor := &auditorEspia{}
	app := nuevaAppDeTestCon(repo, reservas, auditor)

	req := httptest.NewRequest("PUT", "/api/jornada", conBody(jornadaRequest{
		Tramos:     []bloqueRequest{{DiaSemana: "LUNES", HoraInicio: "16:00", HoraFin: "18:00"}},
		Confirmado: true,
	}))
	req.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuve %d", resp.StatusCode)
	}
	cuerpo, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(cuerpo), "la jornada se guardó") {
		t.Errorf("el mensaje tiene que decir que el cambio sí se aplicó: %q", cuerpo)
	}
	// Y queda auditado igual: el horario cambió, y eso es lo que el registro
	// no puede perderse.
	entradas := auditor.de("JORNADA_CAMBIADA")
	if len(entradas) != 1 || entradas[0].Detalle["cascadaIncompleta"] != true {
		t.Errorf("la cascada a medias también se audita: %+v", entradas)
	}
}

func (r *fakeRepo) ListarJornada(_ context.Context) ([]*domain.BloqueJornada, error) {
	var todos []*domain.BloqueJornada
	for _, b := range r.jornada {
		todos = append(todos, b)
	}
	sort.Slice(todos, func(i, j int) bool {
		if todos[i].DiaSemana != todos[j].DiaSemana {
			return todos[i].DiaSemana < todos[j].DiaSemana
		}
		return todos[i].HoraInicio < todos[j].HoraInicio
	})
	return todos, nil
}

func (r *fakeRepo) ReemplazarJornada(_ context.Context, bloques []*domain.BloqueJornada) error {
	r.jornada = make(map[string]*domain.BloqueJornada, len(bloques))
	for _, b := range bloques {
		r.jornada[b.ID] = b
	}
	return nil
}

// fakeReservas es el doble del puerto hacia reservation. Vacío = no hay
// nada que pueda quedar fuera de la jornada, que es el caso de casi todos
// estos tests.
type fakeReservas struct {
	futuras     []application.ReservaFutura
	prestamos   []application.PrestamoAbierto
	canceladas  []string
	errCancelar error
}

func (f *fakeReservas) ReservasFuturas(_ context.Context, _ time.Time) ([]application.ReservaFutura, error) {
	return f.futuras, nil
}

func (f *fakeReservas) PrestamosAbiertos(_ context.Context) ([]application.PrestamoAbierto, error) {
	return f.prestamos, nil
}

func (f *fakeReservas) CancelarReservas(_ context.Context, ids []string, _ string) (int, error) {
	if f.errCancelar != nil {
		return 0, f.errCancelar
	}
	f.canceladas = append(f.canceladas, ids...)
	return len(ids), nil
}

// auditorEspia guarda lo que se auditó, para poder afirmar que una acción
// destructiva dejó rastro.
type auditorEspia struct {
	entradas []audit.Entrada
}

func (a *auditorEspia) Registrar(_ context.Context, e audit.Entrada) error {
	a.entradas = append(a.entradas, e)
	return nil
}

func (a *auditorEspia) de(accion string) []audit.Entrada {
	var r []audit.Entrada
	for _, e := range a.entradas {
		if e.Accion == accion {
			r = append(r, e)
		}
	}
	return r
}
