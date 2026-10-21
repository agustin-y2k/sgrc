//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	reportinginfra "github.com/agustin-y2k/sgrc/internal/reporting/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
)

func TestFechasLimites_DivisionesGrandesYDiaCompleto(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	for _, division := range []string{"2", "10", "999999999998", "999999999999"} {
		ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,2,$3)`, uuid.NewString(), e.d.ciclo, division)
	}
	repo := academicinfra.NewPostgresRepo(pool)
	cursos, err := repo.ListarCursosPorCiclo(ctx, e.d.ciclo)
	if err != nil {
		t.Fatal(err)
	}
	if len(cursos) != 5 || cursos[4].Division != "999999999999" {
		t.Fatalf("orden incorrecto: %+v", cursos)
	}
	if _, err := repo.ListarEstructuraDeCiclo(ctx, e.d.ciclo); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	report := reportinginfra.NewPostgresRepo(pool, loc)
	dia := time.Date(2026, 5, 4, 0, 0, 0, 0, loc)
	for _, fecha := range []time.Time{dia.Add(-time.Nanosecond), dia, dia.Add(12 * time.Hour), dia.AddDate(0, 0, 1).Add(-time.Microsecond), dia.AddDate(0, 0, 1)} {
		ejecutarCierreSQL(t, pool, `INSERT INTO incidencia(equipo_id,descripcion,gravedad,fecha,categoria) VALUES($1,'Falla','LEVE',$2,'Pantalla')`, e.d.equipo, fecha)
	}
	porEquipo, err := report.CalcularIncidenciasPorEquipo(ctx, &dia, &dia)
	if err != nil {
		t.Fatal(err)
	}
	if len(porEquipo) != 1 || porEquipo[0].Total != 3 {
		t.Fatalf("el día local no está completo: %+v", porEquipo)
	}
	categorias, err := report.CalcularIncidenciasPorCategoria(ctx, &dia, &dia)
	if err != nil {
		t.Fatal(err)
	}
	if len(categorias) != 1 || categorias[0].Total != 3 {
		t.Fatalf("rango por categoría: %+v", categorias)
	}
}
func TestFechasLimites_NocturnaEntregaVigilanciaYFinalizacion(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET hora_inicio='22:00',hora_fin='01:00' WHERE id=$1`, e.reservaID)
	ejecutarCierreSQL(t, pool, `UPDATE reserva_grupo SET hora_inicio='22:00',hora_fin='01:00' WHERE id=$1`, e.d.grupo)
	ahora := time.Date(2026, 5, 4, 22, 0, 0, 0, time.UTC)
	repo := reservationinfra.NewPostgresRepo(pool)
	svc := reservationapp.NewService(repo, nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return ahora }, e.bus)
	entregadas, err := svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(entregadas.Entregadas) != 1 {
		t.Fatalf("entrega: %+v", entregadas)
	}
	fin := time.Date(2026, 5, 5, 1, 0, 0, 0, time.UTC)
	if p := entregadas.Entregadas[0]; p.DevolucionEstimada == nil || !p.DevolucionEstimada.Equal(fin) {
		t.Fatalf("vencimiento nocturno: %+v", p)
	}
	ahora = time.Date(2026, 5, 5, 0, 30, 0, 0, time.UTC)
	vigilar, err := repo.ReservasAVigilar(ctx, ahora)
	if err != nil {
		t.Fatal(err)
	}
	encontrado := false
	for _, v := range vigilar {
		if v.ReservaID == e.reservaID {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatal("se perdió la nocturna de ayer")
	}
	if n, err := svc.FinalizarVencidas(ctx); err != nil || n != 0 {
		t.Fatalf("finalizó antes de vencer: %d %v", n, err)
	}
	ahora = fin.Add(time.Minute)
	if n, err := svc.FinalizarVencidas(ctx); err != nil || n != 1 {
		t.Fatalf("no finalizó al vencer: %d %v", n, err)
	}
}
func TestFechasLimites_BloqueoEntreDias(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	for _, caso := range []struct {
		nombre, fecha, inicio, fin string
		diaBloqueo                 int
		inicioBloqueo, finBloqueo  time.Duration
		canceladas                 int
	}{
		{"ayer", "2026-05-04", "22:00", "01:00", 5, 0, 2 * time.Hour, 1},
		{"mañana", "2026-05-05", "00:00", "01:00", 4, 22 * time.Hour, time.Hour, 1},
		{"adyacente", "2026-05-05", "01:00", "02:00", 4, 22 * time.Hour, time.Hour, 0},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			ejecutarCierreSQL(t, pool, `UPDATE reserva SET fecha=$2::date,hora_inicio=$3::time,hora_fin=$4::time WHERE id=$1`, e.reservaID, caso.fecha, caso.inicio, caso.fin)
			ejecutarCierreSQL(t, pool, `UPDATE reserva_grupo SET fecha=$2::date,hora_inicio=$3::time,hora_fin=$4::time WHERE id=$1`, e.d.grupo, caso.fecha, caso.inicio, caso.fin)
			res, err := e.reservas.BloquearEquipos(ctx, []string{e.d.equipo}, nil, time.Date(2026, 5, caso.diaBloqueo, 0, 0, 0, 0, time.UTC), caso.inicioBloqueo, caso.finBloqueo, "Mantenimiento")
			if err != nil || res.ReservasCanceladas != caso.canceladas {
				t.Fatalf("bloqueo: %+v %v", res, err)
			}
		})
	}
}
