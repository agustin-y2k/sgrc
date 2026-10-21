package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/gofiber/fiber/v2"
)

func resolverPedidoHTTP(t *testing.T, app *fiber.App, req resolverPedidoRequest) (int, []byte) {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/pedidos-de-materia/p1/resolver", jsonBody(req))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+tokenPara("admin1", "ADMIN"))
	resp, err := app.Test(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cuerpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, cuerpo
}

func TestHTTP_ResolverPedido_ConstanciaVinculoYConflicto(t *testing.T) {
	for _, aprobar := range []bool{true, false} {
		t.Run(map[bool]string{true: "aprobar", false: "rechazar"}[aprobar], func(t *testing.T) {
			repo := nuevoFakeRepo()
			p, err := domain.NuevoPedidoDeMateria("p1", "docente1", nil, "1°A", "Robótica", "Cargo nuevo", relojDeTest())
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CrearPedido(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			app := nuevaAppDeTest(repo)
			req := resolverPedidoRequest{Aprobar: aprobar, Respuesta: "Respuesta original", CursoID: "c1"}
			codigo, cuerpo := resolverPedidoHTTP(t, app, req)
			if codigo != fiber.StatusOK {
				t.Fatalf("primera resolución: %d %s", codigo, cuerpo)
			}
			var original pedidoResponse
			if err := json.Unmarshal(cuerpo, &original); err != nil {
				t.Fatal(err)
			}
			if aprobar && (original.MateriaCreadaID == nil || original.MateriaSolicitada != "Robótica" || !original.EsMateriaNueva) {
				t.Fatalf("respuesta sin vínculo o texto original: %+v", original)
			}
			req.Respuesta = "Otro texto"
			req.CursoID = ""
			req.Rol = "SUPLENTE"
			codigo, cuerpo = resolverPedidoHTTP(t, app, req)
			if codigo != fiber.StatusOK {
				t.Fatalf("reintento: %d %s", codigo, cuerpo)
			}
			var repetida pedidoResponse
			if err := json.Unmarshal(cuerpo, &repetida); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, repetida) {
				t.Fatalf("reintento cambió la constancia: %+v %+v", original, repetida)
			}
			req.Aprobar = !aprobar
			codigo, cuerpo = resolverPedidoHTTP(t, app, req)
			if codigo != fiber.StatusConflict {
				t.Fatalf("decisión contraria: %d %s", codigo, cuerpo)
			}
			esperadas := 0
			if aprobar {
				esperadas = 1
			}
			if len(repo.materias) != esperadas || len(repo.docentesMateria) != esperadas {
				t.Fatal("se duplicaron o alteraron las escrituras")
			}
		})
	}
}

func TestHTTP_ResolverPedido_InexistenteDevuelve404(t *testing.T) {
	codigo, cuerpo := resolverPedidoHTTP(t, nuevaAppDeTest(nuevoFakeRepo()), resolverPedidoRequest{Aprobar: true, CursoID: "c1"})
	if codigo != fiber.StatusNotFound {
		t.Fatalf("pedido inexistente: %d %s", codigo, cuerpo)
	}
}
