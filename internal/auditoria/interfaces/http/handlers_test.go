package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/auditoria/application"
	"github.com/agustin-y2k/sgrc/internal/auditoria/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
)

type repoHTTP struct {
	filtro   domain.Filtro
	pagina   paginacion.Pagina
	llamadas int
	entradas []domain.Entrada
	total    int
	err      error
}

func (r *repoHTTP) Listar(_ context.Context, f domain.Filtro, p paginacion.Pagina) ([]domain.Entrada, int, error) {
	r.llamadas++
	r.filtro, r.pagina = f, p
	return r.entradas, r.total, r.err
}
func (r *repoHTTP) AccionesEnUso(context.Context) ([]string, error) {
	r.llamadas++
	return []string{"CAMBIO_TRANSACCIONAL"}, r.err
}
func (r *repoHTTP) EntidadesEnUso(context.Context) ([]string, error) {
	r.llamadas++
	return []string{"equipo"}, r.err
}

func pruebaAuditoria(t *testing.T) (*fiber.App, *repoHTTP, *authtest.Registro, []byte) {
	t.Helper()
	repo := &repoHTTP{}
	registro := authtest.Nuevo()
	secret := []byte("clave-para-pruebas-http-de-auditoria")
	app := fiber.New()
	RegisterRoutes(app, NewHandler(application.NewService(repo)), registro.Autenticacion(secret))
	return app, repo, registro, secret
}

func consultar(t *testing.T, app *fiber.App, ruta, token string, estado int) map[string]json.RawMessage {
	t.Helper()
	req := httptest.NewRequest("GET", ruta, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != estado {
		t.Fatalf("%s: estado=%d, esperado=%d", ruta, resp.StatusCode, estado)
	}
	if estado != 200 {
		return nil
	}
	var cuerpo map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		t.Fatal(err)
	}
	return cuerpo
}

func TestRutasAuditoria_SoloAdminVigente(t *testing.T) {
	for _, ruta := range []string{"/api/auditoria", "/api/auditoria/opciones"} {
		t.Run(ruta, func(t *testing.T) {
			app, repo, registro, secret := pruebaAuditoria(t)
			docente := registro.Token(secret, "docente", "DOCENTE")
			admin := registro.Token(secret, "admin", "ADMIN")
			consultar(t, app, ruta, "", 401)
			consultar(t, app, ruta, docente, 403)
			if repo.llamadas != 0 {
				t.Fatal("un acceso denegado consultó el registro")
			}
			consultar(t, app, ruta, admin, 200)
			llamadas := repo.llamadas
			registro.InvalidarSesiones("admin")
			consultar(t, app, ruta, admin, 401)
			if repo.llamadas != llamadas {
				t.Fatal("la sesión revocada consultó el registro")
			}
		})
	}
}

func TestListarAuditoria_FiltrosPaginacionYActorEliminado(t *testing.T) {
	app, repo, registro, secret := pruebaAuditoria(t)
	admin := registro.Token(secret, "admin", "ADMIN")
	repo.total = 12
	repo.entradas = []domain.Entrada{{ID: "entrada", UsuarioID: "actor-eliminado", Accion: "CAMBIO_TRANSACCIONAL", Entidad: "equipo", EntidadID: "equipo-id", EntidadNombre: "PC 7", Referencias: map[string]string{"carro:carro-id": "Carro 1"}, Detalle: []byte(`{"prioridad":2}`), CreadoEn: time.Date(2026, 5, 4, 23, 59, 0, 0, time.UTC)}}
	cuerpo := consultar(t, app, "/api/auditoria?accion=CAMBIO_TRANSACCIONAL&entidad=equipo&entidadId=equipo-id&usuarioId=actor-eliminado&desde=2026-05-04&hasta=2026-05-04&page=2&pageSize=5", admin, 200)
	f := repo.filtro
	if f.Accion != "CAMBIO_TRANSACCIONAL" || f.Entidad != "equipo" || f.EntidadID != "equipo-id" || f.UsuarioID != "actor-eliminado" || f.Desde == nil || f.Hasta == nil || f.Desde.Format(time.DateOnly) != "2026-05-04" || !f.Desde.Equal(*f.Hasta) {
		t.Fatalf("filtros alterados: %+v", f)
	}
	if repo.pagina != (paginacion.Pagina{Numero: 2, Tamanio: 5}) {
		t.Fatalf("página=%+v", repo.pagina)
	}
	var meta paginacion.Meta
	if err := json.Unmarshal(cuerpo["meta"], &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Total != 12 || meta.Page != 2 || meta.PageSize != 5 {
		t.Fatalf("meta=%+v", meta)
	}
	var data []map[string]json.RawMessage
	if err := json.Unmarshal(cuerpo["data"], &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || string(data[0]["actorId"]) != `"actor-eliminado"` || string(data[0]["detalle"]) != `{"prioridad":2}` {
		t.Fatalf("datos=%s", cuerpo["data"])
	}
	if _, existe := data[0]["actorNombre"]; existe {
		t.Fatal("se inventó el nombre de una cuenta eliminada")
	}
	if string(data[0]["entidadNombre"]) != `"PC 7"` || string(data[0]["referencias"]) != `{"carro:carro-id":"Carro 1"}` {
		t.Fatalf("faltan los nombres actuales: %s", cuerpo["data"])
	}
	repo.entradas, repo.total = nil, 0
	vacia := consultar(t, app, "/api/auditoria", admin, 200)
	if string(vacia["data"]) != "[]" {
		t.Fatalf("listado vacío=%s", vacia["data"])
	}
}

func TestListarAuditoria_ConsultasInvalidasNoLleganAlRepo(t *testing.T) {
	for _, query := range []string{"page=0", "pageSize=201", "page=no", "desde=2026-02-30", "hasta=ayer", "desde=2026-05-05&hasta=2026-05-04", "desde=2024-01-01&hasta=2026-01-01"} {
		t.Run(query, func(t *testing.T) {
			app, repo, registro, secret := pruebaAuditoria(t)
			consultar(t, app, "/api/auditoria?"+query, registro.Token(secret, "admin", "ADMIN"), 400)
			if repo.llamadas != 0 {
				t.Fatal("consulta inválida ejecutada")
			}
		})
	}
}

func TestAuditoria_ErroresDeRepositorio(t *testing.T) {
	for _, caso := range []struct {
		ruta   string
		err    error
		estado int
	}{
		{"/api/auditoria?usuarioId=invalido", application.ErrIDInvalido, 400},
		{"/api/auditoria", errors.New("base caída"), 500},
		{"/api/auditoria/opciones", errors.New("base caída"), 500},
	} {
		t.Run(caso.ruta, func(t *testing.T) {
			app, repo, registro, secret := pruebaAuditoria(t)
			repo.err = caso.err
			consultar(t, app, caso.ruta, registro.Token(secret, "admin", "ADMIN"), caso.estado)
		})
	}
}
