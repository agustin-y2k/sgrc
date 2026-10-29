//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	reportingdomain "github.com/agustin-y2k/sgrc/internal/reporting/domain"
	infra "github.com/agustin-y2k/sgrc/internal/reporting/infrastructure"
	app "github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

func TestActividadReportes_RetiroTardioYPrestamoIndependiente(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET estado='NO_RETIRADA' WHERE id=$1`, e.reservaID)
	reportes := infra.NewPostgresRepo(pool)
	a, err := reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Totales.ReservasNoRetiradas != 1 || a.Totales.ReservasAtendidas != 0 {
		t.Fatalf("no retirada: %+v", a.Totales)
	}
	// Mismo docente y misma PC: un préstamo sin reserva sigue siendo independiente.
	suelta, err := e.reservas.EntregarSuelta(ctx, app.EntregaSueltaParams{EquipoIDs: []string{e.d.equipo}, Nombre: "Ada", UsuarioID: &e.d.docente})
	if err != nil {
		t.Fatal(err)
	}
	a, err = reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Totales.ReservasNoRetiradas != 1 || a.Totales.ReservasAtendidas != 0 || a.Totales.PrestamosSinReserva != 1 || a.Totales.PrestamosConReserva != 0 {
		t.Fatalf("se vinculó por coincidencia: %+v", a.Totales)
	}
	if _, err = e.reservas.RecibirEquipos(ctx, []string{suelta.Entregadas[0].ID}, "", ""); err != nil {
		t.Fatal(err)
	}
	// La entrega vinculada se registra después, conservando el estado terminal.
	ret, err := e.entregar(ctx, e.reservas, false)
	if err != nil || len(ret.Entregadas) != 1 {
		t.Fatalf("retiro tardío: %+v %v", ret, err)
	}
	if _, err = e.reservas.RecibirEquipos(ctx, []string{ret.Entregadas[0].ID}, "", ""); err != nil {
		t.Fatal(err)
	}
	ret, err = e.entregar(ctx, e.reservas, false)
	if err != nil || len(ret.Entregadas) != 1 {
		t.Fatalf("segunda tanda: %+v %v", ret, err)
	}
	a, err = reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Totales.ReservasAtendidas != 1 || a.Totales.ReservasNoRetiradas != 0 || a.Totales.PrestamosConReserva != 2 || a.Totales.PrestamosSinReserva != 1 || a.Totales.PrestamosAbiertos != 1 || a.Totales.PrestamosDevueltos != 2 {
		t.Fatalf("actividad incorrecta: %+v", a.Totales)
	}
	ds, err := reportes.CalcularUsoDocentesDeCiclo(ctx, e.d.ciclo, nil, nil)
	if err != nil || len(ds) != 1 || ds[0].CantidadReservas != 1 {
		t.Fatalf("uso docente perdió o duplicó la tardía: %+v %v", ds, err)
	}
	es, err := reportes.CalcularUsoEquiposDeCiclo(ctx, e.d.ciclo, nil, nil)
	if err != nil || len(es) != 1 || es[0].CantidadReservas != 2 {
		t.Fatalf("uso equipo (normal y bloqueo finalizado): %+v %v", es, err)
	}
	tot, err := reportes.TotalesUsoDeCiclo(ctx, e.d.ciclo)
	if err != nil || tot.ReservasDocentes != 1 || tot.ReservasEquipos != 2 {
		t.Fatalf("totales: %+v %v", tot, err)
	}
	var estado string
	if err = pool.QueryRow(ctx, `SELECT estado FROM reserva WHERE id=$1`, e.reservaID).Scan(&estado); err != nil || estado != "NO_RETIRADA" {
		t.Fatalf("reabrió la reserva: %s %v", estado, err)
	}
	// El cierre conserva estadísticas y procedencia aunque elimine las reservas.
	academico, _ := serviciosCierre(pool, "")
	if _, err = academico.ArchivarYClonar(ctx, e.d.ciclo, nil); err != nil {
		t.Fatal(err)
	}
	h, err := reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil || !h.Historico || !h.Disponible || h.Totales != a.Totales {
		t.Fatalf("snapshot: %+v %v", h, err)
	}
	if _, err = e.reservas.RecibirEquipos(ctx, []string{ret.Entregadas[0].ID}, "", ""); err != nil {
		t.Fatal(err)
	}
	h2, err := reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil || h2.Totales != h.Totales {
		t.Fatalf("se reescribió el histórico: %+v %v", h2, err)
	}
	hs, err := reportes.ListarHistoricoUsoDocentePorAnio(ctx, 2026)
	if err != nil || len(hs) != 1 || hs[0].CantidadReservas != 1 {
		t.Fatalf("histórico docente: %+v %v", hs, err)
	}
}
func TestActividadReportes_LegadoYFiltros(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	reportes := infra.NewPostgresRepo(pool)
	ejecutarCierreSQL(t, pool, `INSERT INTO prestamo(equipo_id,entregado_a_nombre,entregado_en,devuelto_en) VALUES($1,'Persona sin cuenta','2026-05-04 01:30Z','2026-05-04 02:00Z')`, e.d.equipo)
	loc, _ := time.LoadLocation("America/Argentina/Buenos_Aires")
	reportes = infra.NewPostgresRepo(pool, loc)
	desde := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	a, err := reportes.CalcularActividad(ctx, e.d.ciclo, &desde, &desde)
	if err != nil {
		t.Fatal(err)
	}
	if a.Totales.PrestamosOrigenNoDeterminado != 0 {
		t.Fatal("no respetó el día local del movimiento")
	}
	a, err = reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil || a.Totales.PrestamosOrigenNoDeterminado != 1 || a.Totales.PrestamosSinReserva != 0 {
		t.Fatalf("infirió el origen antiguo: %+v %v", a, err)
	}
	ejecutarCierreSQL(t, pool, `INSERT INTO snapshot_ciclo(ciclo_id,anio) VALUES($1,2026)`, e.d.ciclo)
	ejecutarCierreSQL(t, pool, `UPDATE ciclo_lectivo SET archivado=true WHERE id=$1`, e.d.ciclo)
	a, err = reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil || a.Disponible || !a.Historico {
		t.Fatalf("inventó un histórico anterior: %+v %v", a, err)
	}
}

func TestActividadReportes_LaLecturaEsperaElCierre(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	reportes := infra.NewPostgresRepo(pool)
	antes, err := reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	academico, _ := serviciosCierre(pool, "")
	pausa := nuevaPausaInstitucional(t)
	cierre := make(chan error, 1)
	go func() {
		cierre <- pgtransaccion.Coordinar(ctx, pool, true, func(ctx context.Context) error {
			if _, err := academico.ArchivarYClonar(ctx, e.d.ciclo, nil); err != nil {
				return err
			}
			return pausa.detener(ctx)
		})
	}()
	pausa.esperar(t, ctx)
	type resultado struct {
		a   reportingdomain.Actividad
		err error
	}
	leida := make(chan resultado, 1)
	go func() { a, err := reportes.CalcularActividad(ctx, e.d.ciclo, nil, nil); leida <- resultado{a, err} }()
	esperarOperacionInstitucional(t, ctx, pool)
	close(pausa.continuar)
	if err := <-cierre; err != nil {
		t.Fatal(err)
	}
	r := <-leida
	if r.err != nil || !r.a.Historico || !r.a.Disponible || r.a.Totales != antes.Totales {
		t.Fatalf("lectura inconsistente durante el cierre: %+v %v", r.a, r.err)
	}
}
