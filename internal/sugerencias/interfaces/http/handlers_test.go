package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/agustin-y2k/sgrc/internal/sugerencias/application"
	"github.com/agustin-y2k/sgrc/internal/sugerencias/domain"
)

type repoHTTP struct {
	hilos          map[string]*domain.Sugerencia
	usuarioListado string
	pagina         paginacion.Pagina
	soloAbiertas   bool
}

func copiaHilo(s *domain.Sugerencia) *domain.Sugerencia {
	copia := *s
	copia.Mensajes = append([]domain.Mensaje(nil), s.Mensajes...)
	return &copia
}
func (r *repoHTTP) Crear(_ context.Context, s *domain.Sugerencia) error {
	r.hilos[s.ID] = copiaHilo(s)
	return nil
}
func (r *repoHTTP) BuscarPorID(_ context.Context, id string) (*domain.Sugerencia, error) {
	if s, ok := r.hilos[id]; ok {
		return copiaHilo(s), nil
	}
	return nil, domain.ErrSugerenciaNoExist
}
func (r *repoHTTP) AgregarMensaje(ctx context.Context, s *domain.Sugerencia, _ domain.Mensaje) error {
	return r.Crear(ctx, s)
}
func (r *repoHTTP) GuardarEstado(ctx context.Context, s *domain.Sugerencia) error {
	return r.Crear(ctx, s)
}
func (r *repoHTTP) ListarTodas(_ context.Context, abiertas bool, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	r.soloAbiertas, r.pagina = abiertas, p
	return []*domain.Sugerencia{}, 0, nil
}
func (r *repoHTTP) ListarDeUsuario(_ context.Context, id string, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	r.usuarioListado, r.pagina = id, p
	return []*domain.Sugerencia{}, 0, nil
}

type contactoHTTP struct{}

func (contactoHTTP) NombreYEmail(context.Context, string) (string, string, error) {
	return "Docente", "docente@test.local", nil
}

type entornoHTTP struct {
	app                  *fiber.App
	repo                 *repoHTTP
	registro             *authtest.Registro
	secret               []byte
	docente, otro, admin string
	eventos              []string
}

func nuevoEntornoHTTP(t *testing.T) *entornoHTTP {
	t.Helper()
	e := &entornoHTTP{repo: &repoHTTP{hilos: map[string]*domain.Sugerencia{}}, registro: authtest.Nuevo(), secret: []byte("clave-para-pruebas-http-de-soporte")}
	e.docente = e.registro.Token(e.secret, "docente", "DOCENTE")
	e.otro = e.registro.Token(e.secret, "otro", "DOCENTE")
	e.admin = e.registro.Token(e.secret, "admin", "ADMIN")
	bus := eventbus.NewInMemoryEventBus()
	for _, tipo := range []string{"sugerencia.nueva", "sugerencia.respondida", "sugerencia.seguimiento", "sugerencia.resuelta"} {
		bus.Subscribe(tipo, func(ev eventbus.Evento) { e.eventos = append(e.eventos, ev.Tipo) })
	}
	svc := application.NewService(e.repo, contactoHTTP{}, uuid.NewString, time.Now, bus)
	e.app = fiber.New()
	RegisterRoutes(e.app, NewHandler(svc), e.registro.Autenticacion(e.secret))
	return e
}
func (e *entornoHTTP) pedir(t *testing.T, metodo, ruta, token, cuerpo string, estado int) map[string]json.RawMessage {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != estado {
		t.Fatalf("%s %s: estado=%d, esperado=%d", metodo, ruta, resp.StatusCode, estado)
	}
	if estado != 200 && estado != 201 {
		return nil
	}
	var respuesta map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&respuesta); err != nil {
		t.Fatal(err)
	}
	return respuesta
}

func TestSoporteHTTP_ConversacionPrivadaCierreYReapertura(t *testing.T) {
	e := nuevoEntornoHTTP(t)
	alta := e.pedir(t, "POST", "/api/sugerencias", e.docente, `{"tipo":"AYUDA","asunto":"Consulta","texto":"Mensaje inicial","usuarioId":"otro"}`, 201)
	var id string
	if err := json.Unmarshal(alta["id"], &id); err != nil {
		t.Fatal(err)
	}
	if e.repo.hilos[id].UsuarioID != "docente" {
		t.Fatal("el body pudo elegir otro propietario")
	}
	if _, existe := alta["usuarioId"]; existe {
		t.Fatal("respuesta propia expone usuarioId")
	}
	mensajes := "/api/sugerencias/" + id + "/mensajes"
	resolver := "/api/sugerencias/" + id + "/resolver"
	e.pedir(t, "POST", mensajes, e.otro, `{"texto":"Intrusión"}`, 403)
	e.pedir(t, "POST", resolver, e.docente, `{}`, 403)
	if len(e.repo.hilos[id].Mensajes) != 1 || len(e.eventos) != 1 {
		t.Fatal("acceso denegado cambió la conversación o publicó eventos")
	}
	respuesta := e.pedir(t, "POST", mensajes, e.admin, `{"texto":"Respuesta del admin"}`, 200)
	if string(respuesta["estado"]) != `"ABIERTA"` || string(respuesta["esperaRespuesta"]) != "false" || string(respuesta["usuarioId"]) != `"docente"` {
		t.Fatalf("respuesta=%v", respuesta)
	}
	e.pedir(t, "POST", resolver, e.admin, `{}`, 200)
	e.pedir(t, "POST", resolver, e.admin, `{}`, 409)
	reapertura := e.pedir(t, "POST", mensajes, e.docente, `{"texto":"Otra consulta","deAdmin":true}`, 200)
	if string(reapertura["estado"]) != `"ABIERTA"` || string(reapertura["esperaRespuesta"]) != "true" {
		t.Fatalf("reapertura=%v", reapertura)
	}
	if _, existe := reapertura["usuarioId"]; existe {
		t.Fatal("seguimiento propio expone usuarioId")
	}
	var mensajesJSON []mensajeResponse
	if err := json.Unmarshal(reapertura["mensajes"], &mensajesJSON); err != nil {
		t.Fatal(err)
	}
	if len(mensajesJSON) != 3 || mensajesJSON[0].DeAdmin || !mensajesJSON[1].DeAdmin || mensajesJSON[2].DeAdmin || mensajesJSON[2].Texto != "Otra consulta" {
		t.Fatalf("mensajes=%+v", mensajesJSON)
	}
	if strings.Join(e.eventos, ",") != "sugerencia.nueva,sugerencia.respondida,sugerencia.resuelta,sugerencia.seguimiento" {
		t.Fatalf("eventos=%v", e.eventos)
	}
}

func TestSoporteHTTP_ListadosPermisosIdentidadYPaginacion(t *testing.T) {
	e := nuevoEntornoHTTP(t)
	e.pedir(t, "GET", "/api/mis-sugerencias", "", "", 401)
	e.pedir(t, "GET", "/api/sugerencias", e.docente, "", 403)
	propias := e.pedir(t, "GET", "/api/mis-sugerencias?usuarioId=otro&page=2&pageSize=3", e.docente, "", 200)
	if e.repo.usuarioListado != "docente" || e.repo.pagina != (paginacion.Pagina{Numero: 2, Tamanio: 3}) {
		t.Fatal("el listado propio no respetó la identidad/página")
	}
	if string(propias["data"]) != "[]" || string(propias["meta"]) != `{"total":0,"page":2,"pageSize":3}` {
		t.Fatalf("listado=%v", propias)
	}
	e.pedir(t, "GET", "/api/sugerencias?abiertas=true&page=3&pageSize=2", e.admin, "", 200)
	if !e.repo.soloAbiertas || e.repo.pagina != (paginacion.Pagina{Numero: 3, Tamanio: 2}) {
		t.Fatal("filtro administrativo alterado")
	}
	e.registro.DarDeBaja("docente")
	e.pedir(t, "GET", "/api/mis-sugerencias", e.docente, "", 401)
}

func TestSoporteHTTP_PaginaConOffsetDesbordadoEs400SinConsultarElRepositorio(t *testing.T) {
	e := nuevoEntornoHTTP(t)
	numero := strconv.Itoa(int(^uint(0) >> 1))
	e.pedir(t, "GET", "/api/mis-sugerencias?page="+numero+"&pageSize=200", e.docente, "", 400)
	e.pedir(t, "GET", "/api/sugerencias?page="+numero+"&pageSize=200", e.admin, "", 400)
	if e.repo.pagina != (paginacion.Pagina{}) || e.repo.usuarioListado != "" {
		t.Fatal("una página cuyo offset no es representable llegó al repositorio")
	}
}

func TestSoporteHTTP_ValidacionYLimiteDeEscritura(t *testing.T) {
	for _, cuerpo := range []string{`{`, `{"tipo":"OTRO","asunto":"Consulta","texto":"Texto"}`, `{"tipo":"AYUDA","asunto":"","texto":"Texto"}`, `{"tipo":"AYUDA","asunto":"Consulta","texto":""}`, `{"tipo":"AYUDA","asunto":"Consulta","texto":"` + strings.Repeat("a", domain.MaxTexto+1) + `"}`} {
		t.Run(cuerpo[:min(len(cuerpo), 45)], func(t *testing.T) {
			e := nuevoEntornoHTTP(t)
			e.pedir(t, "POST", "/api/sugerencias", e.docente, cuerpo, 400)
			if len(e.repo.hilos) != 0 || len(e.eventos) != 0 {
				t.Fatal("se guardó/publicó una entrada inválida")
			}
		})
	}
	e := nuevoEntornoHTTP(t)
	e.pedir(t, "GET", "/api/mis-sugerencias?page=0", e.docente, "", 400)
	e.pedir(t, "GET", "/api/sugerencias?pageSize=201", e.admin, "", 400)
	e.pedir(t, "POST", "/api/sugerencias/inexistente/mensajes", e.docente, `{"texto":"Texto"}`, 404)
	e.pedir(t, "POST", "/api/sugerencias/inexistente/resolver", e.admin, `{}`, 404)
	for range 5 {
		e.pedir(t, "POST", "/api/sugerencias", e.docente, `{"tipo":"AYUDA","asunto":"Consulta","texto":"Texto"}`, 201)
	}
	e.pedir(t, "POST", "/api/sugerencias", e.docente, `{"tipo":"AYUDA","asunto":"Consulta","texto":"Texto"}`, 429)
	if len(e.repo.hilos) != 5 || len(e.eventos) != 5 {
		t.Fatal("la petición limitada tuvo efectos")
	}
}

func TestSoporteHTTP_DetallePrivadoSinDependerDelFiltro(t *testing.T) {
	e := nuevoEntornoHTTP(t)
	alta := e.pedir(t, "POST", "/api/sugerencias", e.docente, `{"tipo":"AYUDA","asunto":"Consulta","texto":"Mensaje privado"}`, 201)
	var id string
	if err := json.Unmarshal(alta["id"], &id); err != nil {
		t.Fatal(err)
	}
	ruta := "/api/sugerencias/" + id
	e.pedir(t, "GET", ruta, "", "", 401)
	e.pedir(t, "GET", ruta, e.otro, "", 403)
	propia := e.pedir(t, "GET", ruta, e.docente, "", 200)
	if _, ok := propia["usuarioId"]; ok {
		t.Fatal("expone propietario en detalle propio")
	}
	admin := e.pedir(t, "GET", ruta, e.admin, "", 200)
	if string(admin["usuarioId"]) != `"docente"` {
		t.Fatal("falta propietario para administración")
	}
	e.pedir(t, "POST", ruta+"/resolver", e.admin, `{}`, 200)
	resuelta := e.pedir(t, "GET", ruta, e.docente, "", 200)
	if string(resuelta["estado"]) != `"RESUELTA"` {
		t.Fatal("el detalle depende del filtro de pendientes")
	}
	e.pedir(t, "GET", "/api/sugerencias/"+uuid.NewString(), e.admin, "", 404)
	if len(e.repo.hilos[id].Mensajes) != 1 || len(e.eventos) != 2 {
		t.Fatal("la lectura provocó efectos")
	}
	e.registro.DarDeBaja("docente")
	e.pedir(t, "GET", ruta, e.docente, "", 401)
}
