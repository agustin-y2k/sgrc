package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

type repoSeriePorFecha struct {
	*fakeRepo
	fechasConsultadas []string
	bloqueado         bool
}

func (r *repoSeriePorFecha) ListarEquiposDisponiblesEn(_ context.Context, fecha time.Time, _, _ time.Duration, _ string) ([]EquipoDisponible, error) {
	r.fechasConsultadas = append(r.fechasConsultadas, fecha.Format("2006-01-02"))
	if fecha.Day() == 9 {
		return []EquipoDisponible{{EquipoID: "pc1", Etiqueta: "PC 1"}}, nil
	}
	return []EquipoDisponible{}, nil
}
func (r *repoSeriePorFecha) ListarEquiposOcupadosEn(_ context.Context, fecha time.Time, _, _ time.Duration) ([]EquipoOcupado, error) {
	if fecha.Day() == 9 {
		return nil, nil
	}
	return []EquipoOcupado{{EquipoID: "pc1", Etiqueta: "PC 1", ReservaID: "r1", EsBloqueo: r.bloqueado, Fecha: fecha, HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour}}, nil
}
func TestPrevisualizarSerie_TodasLasFechasSinEscrituras(t *testing.T) {
	for _, bloqueado := range []bool{false, true} {
		t.Run(map[bool]string{false: "prioridad futura", true: "bloqueo futuro"}[bloqueado], func(t *testing.T) {
			repo := &repoSeriePorFecha{fakeRepo: nuevoFakeRepo(), bloqueado: bloqueado}
			repo.robablesPor = map[string]bool{"r1": !bloqueado}
			svc := nuevoServicioDeTest(repo)
			res, err := svc.PrevisualizarReservaRecurrente(context.Background(), "m1", "docente1", false, domain.Lunes, 8*time.Hour, 9*time.Hour, fecha(2026, 3, 9), fecha(2026, 3, 23), []string{"pc1"})
			if err != nil {
				t.Fatal(err)
			}
			if res.CantidadClases != 3 || len(repo.fechasConsultadas) != 3 || res.Fechas[0].Equipos[0].Estado != "LIBRE" {
				t.Fatalf("no consultó la serie completa: %+v", res)
			}
			if res.PuedeCrear == bloqueado {
				t.Fatalf("disponibilidad incorrecta: %+v", res)
			}
			if !bloqueado && (len(res.EquiposATomar) != 1 || res.EquiposATomar[0] != "pc1") {
				t.Fatal("faltó consentimiento para conflicto futuro")
			}
			if len(repo.grupos)+len(repo.reservas)+len(repo.reglas)+len(repo.prestamos) != 0 {
				t.Fatal("la consulta persistió datos")
			}
		})
	}
}
func TestPrevisualizarSerie_ConservaLimitesYPermisos(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	_, err := svc.PrevisualizarReservaRecurrente(context.Background(), "m1", "u1", false, domain.Lunes, 8*time.Hour, 9*time.Hour, fecha(2026, 3, 9), fecha(2030, 3, 9), []string{"pc1"})
	if !errors.Is(err, ErrDemasiadasOcurrencias) {
		t.Fatalf("límite: %v", err)
	}
	svc.validadorMateria = &fakeValidadorMateria{asignado: false}
	_, err = svc.PrevisualizarReservaRecurrente(context.Background(), "m1", "u1", false, domain.Lunes, 8*time.Hour, 9*time.Hour, fecha(2026, 3, 9), fecha(2026, 3, 23), []string{"pc1"})
	if err == nil {
		t.Fatal("aceptó docente no asignado")
	}
}
