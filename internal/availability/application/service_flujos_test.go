package application

import (
	"context"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/availability/domain"
)

func TestMostrador_ExcepcionSinPatronSemanalEsDeclaracion(t *testing.T) {
	for _, tipo := range []domain.TipoExcepcion{domain.NoDisponible, domain.HorarioModificado} {
		t.Run(string(tipo), func(t *testing.T) {
			repo := nuevoFakeRepo()
			inicio, fin := 8*time.Hour, 10*time.Hour
			excepcion := &domain.Excepcion{UsuarioID: "admin1", Fecha: domain.FechaSolo(elLunesALas(0)), Tipo: tipo}
			if tipo == domain.HorarioModificado {
				excepcion.HoraInicio, excepcion.HoraFin = &inicio, &fin
			}
			repo.excepciones[claveExcepcion("admin1", elLunesALas(0))] = excepcion
			svc := servicioConAdmins(repo, "admin1")
			for _, hora := range []int{9, 11} {
				cobertura, err := svc.MostradorEn(context.Background(), elLunesALas(hora))
				if err != nil {
					t.Fatal(err)
				}
				atendido := tipo == domain.HorarioModificado && hora == 9
				if !cobertura.Declarado || cobertura.Atendido != atendido || cobertura.Opera() != atendido {
					t.Fatalf("excepción sin patrón ignorada: hora=%d %+v", hora, cobertura)
				}
			}
			cobertura, err := svc.MostradorEseDia(context.Background(), elLunesALas(0))
			if err != nil || !cobertura.Declarado || cobertura.Opera() != (tipo == domain.HorarioModificado) {
				t.Fatalf("cobertura del día: %+v %v", cobertura, err)
			}
		})
	}
}
