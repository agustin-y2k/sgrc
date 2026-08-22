package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
	"github.com/ramiro/sgrc/internal/shared/authtest"
)

// ── fakeRepo — misma semántica de titularidad-acotada-por-usuario_id que
// application/service_test.go, reimplementada acá porque interfaces/http no
// puede importar los tipos no exportados de otro paquete de test.

type fakeRepo struct {
	jornada         map[string]*domain.BloqueJornada
	jornadaDefinida bool

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
	svc := application.NewService(repo, &fakeListadorAdmins{}, idSecuencial(), func() time.Time {
		// lunes 9-mar-2026, 10:00 — coherente con los bloques LUNES 08-12
		// que usan los tests de DisponibilidadDeAdmins.
		return time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
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

	svc := application.NewService(repo, &fakeListadorAdmins{admins: []application.AdminInfo{{ID: "admin1", Nombre: "Ada", Apellido: "Lovelace"}}}, idSecuencial(), func() time.Time {
		return time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
	})
	app := fiber.New()
	RegisterRoutes(app, NewHandler(svc), registroDePrueba.Autenticacion(testSecret))

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
	if !repo.jornadaDefinida {
		t.Error("guardar la jornada es decidirla")
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
	if !repo.jornadaDefinida {
		t.Error("dejarla libre también es decidir, y por eso no se vuelve a preguntar")
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

// El GET lleva la bandera al lado de los tramos: sin ella, una lista vacía no
// dice si la escuela todavía no declaró su jornada o si eligió dejarla libre.
func TestHTTP_Jornada_InformaSiYaFueDefinida(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	pedir := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/jornada", nil)
		req.Header.Set("Authorization", "Bearer "+tokenPara("docente1", "DOCENTE"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		var cuerpo map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
			t.Fatalf("respuesta ilegible: %v", err)
		}
		return cuerpo
	}

	if definida := pedir()["definida"]; definida != false {
		t.Errorf("una instalación nueva todavía no decidió: %v", definida)
	}

	repo.jornadaDefinida = true

	if definida := pedir()["definida"]; definida != true {
		t.Errorf("ya decidió: %v", definida)
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
	r.jornadaDefinida = true
	return nil
}

func (r *fakeRepo) JornadaDefinida(_ context.Context) (bool, error) {
	return r.jornadaDefinida, nil
}
