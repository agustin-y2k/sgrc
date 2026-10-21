package application

import (
	"context"
	"sort"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func reservaParaActualizar(ctx context.Context, repo Repo, id string) (*domain.Reserva, error) {
	porID, err := repo.BuscarReservasParaActualizar(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	if r := porID[id]; r != nil {
		return r, nil
	}
	return nil, ErrReservaNoEncontrada
}

// Conserva el alcance de la selección: una reserva que cambió de equipo
// antes del bloqueo ya no pertenece a esa cascada ni al cambio de serie.
func reservasParaActualizar(ctx context.Context, repo Repo, candidatas []*domain.Reserva) ([]*domain.Reserva, error) {
	ids := make([]string, 0, len(candidatas))
	for _, r := range candidatas {
		ids = append(ids, r.ID)
	}
	porID, err := repo.BuscarReservasParaActualizar(ctx, ids)
	if err != nil {
		return nil, err
	}
	var actuales []*domain.Reserva
	vistas := map[string]bool{}
	for _, previa := range candidatas {
		r := porID[previa.ID]
		if r == nil || vistas[r.ID] || r.EquipoID != previa.EquipoID {
			continue
		}
		vistas[r.ID] = true
		actuales = append(actuales, r)
	}
	return actuales, nil
}

func clavesOrdenadas(m map[string]bool) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// El llamador ya tiene bloqueados el grupo y sus reservas. La escritura
// del hijo y esta agregación se confirman en la misma transacción.
func recalcularGrupo(ctx context.Context, repo Repo, id string) error {
	g, err := repo.BuscarReservaGrupoPorID(ctx, id)
	if err != nil {
		return err
	}
	if g.Estado == domain.GrupoCancelada || g.Estado == domain.GrupoFinalizada || g.Estado == domain.GrupoNoRetirado {
		return nil
	}
	reservas, err := repo.ListarReservasPorGrupo(ctx, id)
	if err != nil {
		return err
	}
	nuevo := domain.EstadoDeGrupo(reservas, g.Estado)
	if nuevo == g.Estado {
		return nil
	}
	if err := g.CambiarEstado(nuevo); err != nil {
		return err
	}
	return repo.GuardarReservaGrupo(ctx, g)
}
