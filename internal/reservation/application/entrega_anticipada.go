package application

import (
	"context"
	"fmt"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

// El préstamo empieza al retirar la PC, aunque su clase sea más tarde.
// Comparar instantes completos permite cubrir también retiros cuyo fin cae mañana.
func reservaQueImpideAnticiparEntrega(ctx context.Context, repo Repo, reserva *domain.Reserva, ahora time.Time) (*domain.Reserva, error) {
	if !ahora.Before(domain.InicioDePared(reserva.Fecha, reserva.HoraInicio, ahora.Location())) {
		return nil, nil
	}
	reservas, err := repo.ListarReservasFuturasDeEquipo(ctx, reserva.EquipoID, ahora)
	if err != nil {
		return nil, fmt.Errorf("comprobar reservas antes de adelantar la entrega: %w", err)
	}
	vence := domain.FinDePared(reserva.Fecha, reserva.HoraInicio, reserva.HoraFin, ahora.Location())
	for _, otra := range reservas {
		if otra.ID == reserva.ID || otra.EquipoID != reserva.EquipoID || otra.Estado != domain.ReservaConfirmada {
			continue
		}
		inicio := domain.InicioDePared(otra.Fecha, otra.HoraInicio, ahora.Location())
		fin := domain.FinDePared(otra.Fecha, otra.HoraInicio, otra.HoraFin, ahora.Location())
		if inicio.Before(vence) && ahora.Before(fin) {
			return otra, nil
		}
	}
	return nil, nil
}

func detalleDeReservaIntermedia(reserva *domain.Reserva) string {
	tipo := "otra reserva"
	if reserva.Tipo == domain.TipoBloqueo {
		tipo = "un bloqueo administrativo"
	}
	fin := ""
	if reserva.HoraFin < reserva.HoraInicio {
		fin = " del día siguiente"
	}
	return fmt.Sprintf("no se puede adelantar la entrega: hay %s el %s de %02d:%02d a %02d:%02d%s; la reserva para retirar después se mantiene",
		tipo, reserva.Fecha.Format("02/01/2006"),
		int(reserva.HoraInicio.Hours()), int(reserva.HoraInicio.Minutes())%60,
		int(reserva.HoraFin.Hours()), int(reserva.HoraFin.Minutes())%60, fin)
}
