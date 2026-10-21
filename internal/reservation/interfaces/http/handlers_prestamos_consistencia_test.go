package http

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestHTTP_EntregaParcial_SegundaTandaInformaSoloLoNuncaRetirado(t *testing.T) {
	repo := nuevoFakeRepo()
	for i, id := range []string{"res1", "res2", "res3"} {
		reservaEnRepo(t, repo, id, []string{"pc1", "pc2", "pc3"}[i], "docente1")
	}
	repo.grupos["grupo1"] = &domain.ReservaGrupo{ID: "grupo1", Estado: domain.GrupoConfirmada}
	app := nuevaAppDeTest(repo)
	if codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos/por-reserva",
		entregarPorReservaRequest{ReservaIDs: []string{"res1"}}, "ADMIN"); codigo != fiber.StatusOK {
		t.Fatalf("primera tanda: %d %s", codigo, cuerpo)
	}
	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos/por-reserva",
		entregarPorReservaRequest{ReservaIDs: []string{"res2"}, LiberarNoEntregadas: true}, "ADMIN")
	if codigo != fiber.StatusOK {
		t.Fatalf("segunda tanda: %d %s", codigo, cuerpo)
	}
	var resultado resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &resultado); err != nil {
		t.Fatal(err)
	}
	if len(resultado.Entregadas) != 1 || len(resultado.NoEntregadas) != 0 || len(resultado.Liberadas) != 1 || resultado.Liberadas[0] != "res3" {
		t.Fatalf("resultado incoherente: %+v", resultado)
	}
	if len(repo.prestamos) != 2 || repo.reservas["res1"].Estado != domain.ReservaConfirmada || repo.reservas["res3"].Estado != domain.ReservaNoRetirada {
		t.Fatalf("el estado persistido no coincide con la respuesta: %+v", repo.reservas)
	}
}

func TestHTTP_Prestamos_IDInexistenteNoOcultaCambios(t *testing.T) {
	t.Run("entrega", func(t *testing.T) {
		repo := nuevoFakeRepo()
		reservaEnRepo(t, repo, "res1", "pc1", "docente1")
		app := nuevaAppDeTest(repo)
		codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos/por-reserva",
			entregarPorReservaRequest{ReservaIDs: []string{"res1", "inexistente"}}, "ADMIN")
		if codigo != fiber.StatusNotFound || len(repo.prestamos) != 0 {
			t.Fatalf("el 404 dejó una entrega oculta: %d %s, préstamos=%d", codigo, cuerpo, len(repo.prestamos))
		}
	})
	t.Run("devolucion", func(t *testing.T) {
		repo := nuevoFakeRepo()
		app := nuevaAppDeTest(repo)
		codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/prestamos",
			entregarSueltaRequest{EquipoIDs: []string{"pc1"}, Nombre: "Ada"}, "ADMIN")
		var entrega resultadoEntregaResponse
		if codigo != fiber.StatusCreated {
			t.Fatalf("preparando entrega: %d %s", codigo, cuerpo)
		}
		if err := json.Unmarshal(cuerpo, &entrega); err != nil {
			t.Fatal(err)
		}
		id := entrega.Entregadas[0].ID
		codigo, cuerpo = pedirPrestamos(t, app, "POST", "/api/prestamos/recibir",
			recibirRequest{PrestamoIDs: []string{id, "inexistente"}}, "ADMIN")
		if codigo != fiber.StatusNotFound || repo.prestamos[id].DevueltoEn != nil {
			t.Fatalf("el 404 dejó una devolución oculta: %d %s, préstamo=%+v", codigo, cuerpo, repo.prestamos[id])
		}
	})
}
