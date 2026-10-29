package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"testing"
)

func (r *fakeRepo) BuscarBloqueoParaActualizar(_ context.Context, operacionID string) ([]*domain.Reserva, error) {
	var filas []*domain.Reserva
	for _, reserva := range r.enOrden() {
		if reserva.Tipo == domain.TipoBloqueo && reserva.BloqueoOperacionID != nil && *reserva.BloqueoOperacionID == operacionID {
			filas = append(filas, reserva)
		}
	}
	return filas, nil
}
func (r *fakeRepo) RegistrarLevantamientoDeBloqueo(context.Context, LevantarBloqueoParams, int) error {
	return nil
}
func TestLevantarBloqueo_ConservaLaOperacionYLaHistoria(t *testing.T) {
	repo := nuevoFakeRepo()
	creador := "admin"
	operacion := "operacion"
	for _, id := range []string{"a", "b", "otra"} {
		r := reservaDeTest(t, repo, id, "pc-"+id)
		r.Tipo = domain.TipoBloqueo
		r.ReservaGrupoID = nil
		r.CreadoPor = &creador
		r.BloqueoOperacionID = &operacion
	}
	otra := "otra-operacion"
	repo.reservas["otra"].BloqueoOperacionID = &otra
	svc := nuevoServicioDeTest(repo)
	p := LevantarBloqueoParams{OperacionID: operacion, SolicitanteID: creador, EsAdmin: true, Motivo: "Terminó la actividad"}
	res, err := svc.LevantarBloqueo(context.Background(), p)
	if err != nil || res.ReservasCanceladas != 2 || repo.reservas["otra"].Estado != domain.ReservaConfirmada {
		t.Fatalf("levantamiento: %+v %v", res, err)
	}
	p.Motivo = "Otro motivo"
	res, err = svc.LevantarBloqueo(context.Background(), p)
	if err != nil || res.ReservasCanceladas != 0 || res.ReservasYaCanceladas != 2 || *repo.reservas["a"].MotivoCancelacion != "Terminó la actividad" {
		t.Fatalf("repetición: %+v %v", res, err)
	}
}
