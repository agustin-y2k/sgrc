//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestPrioridadCursoExacto_AusenciaDeAtributosNoEsComodin(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	sinAtributos := materiaEnModalidadDeTest(t, pool, 2099, "Matemática", 3, "", nil)
	modalidad := "Construcción"
	otraModalidad := materiaEnModalidadDeTest(t, pool, 2099, "Matemática", 3, "", &modalidad)
	otraDivision := materiaEnModalidadDeTest(t, pool, 2099, "Matemática", 3, "2", nil)
	otraMateria := materiaEnModalidadDeTest(t, pool, 2099, "TIC", 3, "2", nil)
	pc := crearEquipoDeCarroDeTest(t, pool)
	fecha := time.Date(2099, 3, 9, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO equipo_preferencia (equipo_id,materia_nombre,anio,prioridad,curso_exacto) VALUES ($1,'Matemática',3,1,true)`, pc); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		materia string
		tramo   application.TramoPreferencia
	}{
		{sinAtributos, application.TramoPreferente},
		{otraModalidad, application.TramoDeOtraMateria},
		{otraDivision, application.TramoDeOtraMateria},
	} {
		disponibles, err := repo.ListarEquiposDisponiblesEn(ctx, fecha, 10*time.Hour, 11*time.Hour, tc.materia)
		if err != nil || len(disponibles) != 1 || disponibles[0].Tramo != tc.tramo {
			t.Fatalf("alcance de %s: %+v, %v", tc.materia, disponibles, err)
		}
	}
	grupo := nuevoReservaGrupoDeTest(t, otraMateria, fecha, 10*time.Hour, 11*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		t.Fatal(err)
	}
	reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, pc, otraMateria, "Docente TIC", nil, fecha, 10*time.Hour, 11*time.Hour, ahoraDeTest())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearReserva(ctx, reserva); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		materia string
		robable bool
	}{{sinAtributos, true}, {otraModalidad, false}, {otraDivision, false}} {
		robables, err := repo.ReservasRobablesPor(ctx, tc.materia, []string{reserva.ID})
		if err != nil || robables[reserva.ID] != tc.robable {
			t.Fatalf("robo por %s: %+v, %v", tc.materia, robables, err)
		}
	}
	// Una regla general anterior sigue abarcando las otras modalidades.
	marcarPreferencia(t, pool, pc, "Matemática", nil, nil, nil, 1)
	// La marca concreta tiene precedencia incluso con un número más débil.
	if _, err := pool.Exec(ctx, `UPDATE equipo_preferencia SET prioridad=3 WHERE equipo_id=$1 AND curso_exacto`, pc); err != nil {
		t.Fatal(err)
	}
	marcarPreferencia(t, pool, pc, "TIC", nil, nil, nil, 2)
	for _, tc := range []struct {
		materia string
		robable bool
	}{{sinAtributos, false}, {otraModalidad, true}, {otraDivision, true}} {
		robables, err := repo.ReservasRobablesPor(ctx, tc.materia, []string{reserva.ID})
		if err != nil || robables[reserva.ID] != tc.robable {
			t.Fatalf("especificidad por %s: %+v, %v", tc.materia, robables, err)
		}
	}
}
