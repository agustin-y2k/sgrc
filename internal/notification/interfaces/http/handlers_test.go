package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sort"
	"strings"
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

func (f *fakeListadorAdmins) EmailsDeAdminsSuscriptos(ctx context.Context, categoria domain.CategoriaEmail) ([]string, error) {
	return nil, nil
}

// fakePreferencias hace de tabla preferencia_email: guarda las decisiones
// explícitas, y lo que no está es lo que nadie eligió todavía.
type fakePreferencias struct {
	porUsuario map[string]map[domain.CategoriaEmail]bool
	// porEmail es lo mismo indexado como lo consulta el envío. Vacío = nadie
	// eligió nada, así que mandan los valores por defecto.
	porEmail map[string]map[domain.CategoriaEmail]bool
	err      error
}

func (f *fakePreferencias) ElegidasDe(ctx context.Context, usuarioID string) (map[domain.CategoriaEmail]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.porUsuario[usuarioID], nil
}

func (f *fakePreferencias) Reemplazar(ctx context.Context, usuarioID string, decisiones map[domain.CategoriaEmail]bool) error {
	if f.err != nil {
		return f.err
	}
	if f.porUsuario == nil {
		f.porUsuario = map[string]map[domain.CategoriaEmail]bool{}
	}
	f.porUsuario[usuarioID] = decisiones
	return nil
}

func (f *fakePreferencias) RecibePorEmail(ctx context.Context, email string, categoria domain.CategoriaEmail) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if activa, decidio := f.porEmail[email][categoria]; decidio {
		return activa, nil
	}
	return categoria.ActivaPorDefecto(), nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

var testSecret = []byte("un-secreto-de-test-bastante-largo")

func nuevaAppDeTest(repo *fakeRepo) *fiber.App {
	return nuevaAppConPreferencias(repo, &fakePreferencias{})
}

func nuevaAppConPreferencias(repo *fakeRepo, prefs application.PreferenciasEmail) *fiber.App {
	contadorID = 0
	svc := application.NewService(repo, &fakeListadorAdmins{}, prefs, idSecuencial, func() time.Time {
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
		resp.Body.Close()
	}
}

func TestHTTP_ListarPropias_SinToken_401(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/notifications/", nil))
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("esperaba 401, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_ListarPropias_FiltroEstadoInvalido_400(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("GET", "/api/notifications/?estado=ARCHIVADA", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("esperaba 403 incluso para un Admin, obtuve %d", resp.StatusCode)
	}
}

func TestHTTP_MarcarLeida_NoExiste_404(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	req := httptest.NewRequest("PATCH", "/api/notifications/no-existe/leida", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara("u1", "DOCENTE"))

	resp, _ := app.Test(req)
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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

// ── Preferencias de correo (RF-05.13) ───────────────────────────────────

func leerPreferencias(t *testing.T, app *fiber.App, usuarioID, rol string) []preferenciaEmailResponse {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/notifications/preferencias-email", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPara(usuarioID, rol))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", resp.StatusCode)
	}
	var body preferenciasEmailResponse
	json.NewDecoder(resp.Body).Decode(&body)
	return body.Data
}

// Devuelve el estado y el cuerpo ya leído, en vez de la respuesta: así el
// cuerpo se cierra acá y no queda librado a que cada test se acuerde. Los
// pedidos que terminan en 4xx no traen la lista, y el cuerpo vacío es
// justamente lo que esos tests esperan.
func guardarPreferencias(t *testing.T, app *fiber.App, usuarioID, rol, cuerpo string) (int, preferenciasEmailResponse) {
	t.Helper()
	req := httptest.NewRequest("PUT", "/api/notifications/preferencias-email", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara(usuarioID, rol))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	defer resp.Body.Close()

	var body preferenciasEmailResponse
	json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestHTTP_ListarPreferenciasEmail_CadaUnaConSuValorPorDefecto(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	data := leerPreferencias(t, app, "admin1", "ADMIN")

	if len(data) != len(domain.CategoriasDeEmail()) {
		t.Fatalf("el Admin tendría que ver todas (%d), vinieron %d",
			len(domain.CategoriasDeEmail()), len(data))
	}
	for _, p := range data {
		categoria, err := domain.ParseCategoriaEmail(p.Categoria)
		if err != nil {
			t.Fatalf("categoría desconocida en la respuesta: %v", err)
		}
		if p.Activa != categoria.ActivaPorDefecto() {
			t.Errorf("%s vino activa=%t y se esperaba %t", p.Categoria, p.Activa, categoria.ActivaPorDefecto())
		}
		if p.Fija != categoria.EsFija() {
			t.Errorf("%s vino fija=%t y se esperaba %t", p.Categoria, p.Fija, categoria.EsFija())
		}
		// Sin etiqueta la casilla no dice de qué avisa, que es lo único que
		// permite decidir si se quiere.
		if p.Etiqueta == "" || p.Descripcion == "" {
			t.Errorf("%s viene sin texto para mostrar: %+v", p.Categoria, p)
		}
		if p.Grupo == "" {
			t.Errorf("%s viene sin grupo: la pantalla no sabría dónde ponerla", p.Categoria)
		}
	}
}

// El docente ve sus correos y los de su cuenta, y ninguna casilla de
// administración: esos avisos no le llegan.
func TestHTTP_ListarPreferenciasEmail_ElDocenteNoVeLasDeAdministracion(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	data := leerPreferencias(t, app, "docente1", "DOCENTE")

	if len(data) != len(domain.CategoriasPara(false)) {
		t.Fatalf("esperaba %d casillas, vinieron %d", len(domain.CategoriasPara(false)), len(data))
	}
	for _, p := range data {
		if p.Grupo == string(domain.GrupoAdministracion) {
			t.Errorf("un docente no debería ver %s", p.Categoria)
		}
	}
}

// Las de la cuenta se muestran tildadas y marcadas como fijas: están para que
// se vea que existen, no para elegirlas.
func TestHTTP_ListarPreferenciasEmail_LasDeLaCuentaVienenFijasYActivas(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	fijas := 0
	for _, p := range leerPreferencias(t, app, "docente1", "DOCENTE") {
		if p.Grupo != string(domain.GrupoCuenta) {
			continue
		}
		fijas++
		if !p.Fija || !p.Activa {
			t.Errorf("%s tendría que venir fija y activa: %+v", p.Categoria, p)
		}
	}
	if fijas != 2 {
		t.Errorf("esperaba las dos de la cuenta, vinieron %d", fijas)
	}
}

func TestHTTP_GuardarPreferenciasEmail_GuardaYDevuelveComoQuedo(t *testing.T) {
	prefs := &fakePreferencias{}
	app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

	estado, body := guardarPreferencias(t, app, "admin1", "ADMIN",
		`{"categorias":["SUGERENCIA","CUENTA_PENDIENTE"]}`)
	if estado != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", estado)
	}

	activas := map[string]bool{}
	for _, p := range body.Data {
		if p.Activa {
			activas[p.Categoria] = true
		}
	}
	if !activas["SUGERENCIA"] || !activas["CUENTA_PENDIENTE"] {
		t.Errorf("faltan las dos que tildó: %v", activas)
	}
	// Lo que no tildó quedó apagado, incluido lo que venía encendido.
	if activas["EQUIPO_NO_DISPONIBLE"] || activas["PEDIDO_DE_LIBERACION"] {
		t.Errorf("quedó encendido algo que no tildó: %v", activas)
	}
	// Y las de la cuenta siguen ahí, que no dependen de lo que mande.
	if !activas["RECUPERACION_DE_CUENTA"] || !activas["CUENTA_APROBADA"] {
		t.Errorf("se apagó un correo de la cuenta: %v", activas)
	}

	if !prefs.porUsuario["admin1"][domain.CatSugerencia] {
		t.Errorf("no se guardó lo que tildó: %v", prefs.porUsuario["admin1"])
	}
	if len(prefs.porUsuario["admin1"]) != len(domain.Configurables(true)) {
		t.Errorf("esperaba una decisión por categoría configurable, hay %d",
			len(prefs.porUsuario["admin1"]))
	}
}

// Guardar el panel con todo destildado es una operación válida, no un pedido
// vacío que haya que rechazar.
func TestHTTP_GuardarPreferenciasEmail_ListaVacia_200(t *testing.T) {
	prefs := &fakePreferencias{porUsuario: map[string]map[domain.CategoriaEmail]bool{
		"admin1": {domain.CatSugerencia: true},
	}}
	app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

	estado, _ := guardarPreferencias(t, app, "admin1", "ADMIN", `{"categorias":[]}`)
	if estado != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", estado)
	}

	for categoria, activa := range prefs.porUsuario["admin1"] {
		if activa {
			t.Errorf("se destildó todo y %s quedó encendida", categoria)
		}
	}
	// Destildar tiene que quedar GUARDADO y no volver al default: si no, lo
	// que arranca encendido se reencendería solo.
	if len(prefs.porUsuario["admin1"]) != len(domain.Configurables(true)) {
		t.Errorf("esperaba una decisión explícita por categoría, hay %d",
			len(prefs.porUsuario["admin1"]))
	}
}

func TestHTTP_GuardarPreferenciasEmail_CategoriaQueNoExiste_400(t *testing.T) {
	prefs := &fakePreferencias{}
	app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

	// "GENERAL" es un Tipo de notificación, no una categoría de correo.
	estado, _ := guardarPreferencias(t, app, "admin1", "ADMIN", `{"categorias":["GENERAL"]}`)

	if estado != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", estado)
	}
	if _, guardo := prefs.porUsuario["admin1"]; guardo {
		t.Error("un pedido inválido no debería haber tocado nada")
	}
}

// Las fijas no se apagan ni mandando el pedido a mano: ni las de la cuenta ni
// las de soporte, que son fijas por otra razón.
func TestHTTP_GuardarPreferenciasEmail_NoDejaTocarLasFijas(t *testing.T) {
	for _, fija := range []domain.CategoriaEmail{
		domain.CatRecuperacionDeCuenta, domain.CatSoporteRespondido,
	} {
		t.Run(string(fija), func(t *testing.T) {
			prefs := &fakePreferencias{}
			app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

			estado, _ := guardarPreferencias(t, app, "docente1", "DOCENTE",
				`{"categorias":["`+string(fija)+`"]}`)

			if estado != fiber.StatusBadRequest {
				t.Fatalf("esperaba 400, obtuve %d", estado)
			}
			if _, guardo := prefs.porUsuario["docente1"]; guardo {
				t.Error("no tendría que haber guardado nada")
			}
		})
	}
}

// Un docente no puede encender un aviso de administración: no lo recibe, y la
// fila no haría nada más que confundir el día que lo asciendan.
func TestHTTP_GuardarPreferenciasEmail_DocenteConCategoriaDeAdmin_403(t *testing.T) {
	prefs := &fakePreferencias{}
	app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

	estado, _ := guardarPreferencias(t, app, "docente1", "DOCENTE", `{"categorias":["SUGERENCIA"]}`)

	if estado != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", estado)
	}
	if _, guardo := prefs.porUsuario["docente1"]; guardo {
		t.Error("no tendría que haber guardado nada")
	}
}

// Y sí puede guardar las suyas: el panel es de todos.
func TestHTTP_GuardarPreferenciasEmail_DocenteGuardaLasSuyas_200(t *testing.T) {
	prefs := &fakePreferencias{}
	app := nuevaAppConPreferencias(nuevoFakeRepo(), prefs)

	estado, _ := guardarPreferencias(t, app, "docente1", "DOCENTE",
		`{"categorias":["RECORDATORIO_DE_RESERVA"]}`)

	if estado != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d", estado)
	}
	if !prefs.porUsuario["docente1"][domain.CatRecordatorioDeReserva] {
		t.Errorf("no se guardó: %v", prefs.porUsuario["docente1"])
	}
	if len(prefs.porUsuario["docente1"]) != len(domain.Configurables(false)) {
		t.Errorf("esperaba solo sus categorías configurables, hay %d",
			len(prefs.porUsuario["docente1"]))
	}
}
