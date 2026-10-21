//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	notificaciones "github.com/agustin-y2k/sgrc/internal/notification/application"
	notificacionespg "github.com/agustin-y2k/sgrc/internal/notification/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/secretos"
	"github.com/jackc/pgx/v5/pgxpool"
)

type escenarioReposicion struct {
	pool                                         *pgxpool.Pool
	repo                                         *PostgresRepo
	dibujo, tic, matematica                      string
	docenteDibujo, docenteTIC, docenteMatematica string
	moderna, vieja                               string
	fecha                                        time.Time
	bus                                          *eventbus.DurableBus
	svc                                          *application.Service
	avisos                                       []eventbus.Evento
}

func escenarioDeReposicion(t *testing.T) *escenarioReposicion {
	t.Helper()
	f := &escenarioReposicion{pool: levantarPostgresDeTest(t), fecha: time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)}
	f.repo = NewPostgresRepo(f.pool)
	f.dibujo = materiaEnCursoDeTest(t, f.pool, "Dibujo Técnico", "3°B")
	f.tic = materiaEnCursoDeTest(t, f.pool, "TIC", "5°A")
	f.matematica = materiaEnCursoDeTest(t, f.pool, "Matemática", "4°C")
	f.docenteDibujo = crearUsuarioDeTest(t, f.pool, "DOCENTE", "APROBADA")
	f.docenteTIC = crearUsuarioDeTest(t, f.pool, "DOCENTE", "APROBADA")
	f.docenteMatematica = crearUsuarioDeTest(t, f.pool, "DOCENTE", "APROBADA")
	asignarDocenteAMateria(t, f.pool, f.docenteDibujo, f.dibujo)
	f.moderna = computadoraDeReposicion(t, f.pool, "AutoCAD 2026")
	f.vieja = computadoraDeReposicion(t, f.pool, "AutoCAD 2007")
	marcarPreferencia(t, f.pool, f.moderna, "Dibujo Técnico", nil, nil, nil, 1)
	marcarPreferencia(t, f.pool, f.moderna, "TIC", nil, nil, nil, 2)
	marcarPreferencia(t, f.pool, f.vieja, "TIC", nil, nil, nil, 1)
	marcarPreferencia(t, f.pool, f.vieja, "Matemática", nil, nil, nil, 3)
	cifrador, err := secretos.Nuevo("clave-estable-de-prueba-para-reposicion-segura")
	if err != nil {
		t.Fatal(err)
	}
	f.bus = eventbus.NewDurableBus(f.pool, cifrador)
	notifSvc := notificaciones.NewService(notificacionespg.NewPostgresRepo(f.pool), notificacionespg.NewListadorAdminsPostgres(f.pool), notificacionespg.NewPreferenciasEmailPostgres(f.pool), NuevoID, time.Now)
	notificaciones.RegisterEventHandlersSincronos(f.bus, notifSvc)
	f.bus.Subscribe("reserva.reubicada", func(e eventbus.Evento) { f.avisos = append(f.avisos, e) })
	f.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) { f.avisos = append(f.avisos, e) })
	f.svc = application.NewService(f.repo, NewValidadorMateriaPostgres(f.pool), NewValidadorEquipoPostgres(f.pool), jornadaLibre{}, NewObtenedorNombrePostgres(f.pool), NuevoID, func() time.Time { return f.fecha.AddDate(0, 0, -1) }, f.bus)
	return f
}

func computadoraDeReposicion(t *testing.T, pool *pgxpool.Pool, software string) string {
	t.Helper()
	id := crearEquipoDeCarroDeTest(t, pool)
	if _, err := pool.Exec(context.Background(), `UPDATE equipo SET es_computadora=true,software_instalado=$2 WHERE id=$1`, id, software); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *escenarioReposicion) tomar(t *testing.T, equipos []string) (application.ImpactoDelRobo, error) {
	t.Helper()
	_, _, impacto, err := f.svc.CrearReserva(context.Background(), f.dibujo, f.docenteDibujo, false, f.fecha, 11*time.Hour, 12*time.Hour, equipos, []string{f.moderna})
	return impacto, err
}

func (f *escenarioReposicion) comprobar(t *testing.T, original *domain.Reserva, equipo string, estado domain.EstadoReserva) {
	t.Helper()
	r, err := f.repo.BuscarReservaPorID(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.EquipoID != equipo || r.Estado != estado || r.HoraInicio != original.HoraInicio || r.HoraFin != original.HoraFin || !r.Fecha.Equal(original.Fecha) || *r.ReservaGrupoID != *original.ReservaGrupoID || *r.CreadoPor != *original.CreadoPor {
		t.Fatalf("reserva modificada incorrectamente: %+v", r)
	}
	if equipo != original.EquipoID && estado == domain.ReservaConfirmada {
		var anterior, nuevo string
		err := f.pool.QueryRow(context.Background(), `SELECT detalle->'antes'->>'equipo_id', detalle->'despues'->>'equipo_id' FROM audit_log WHERE entidad='reserva' AND entidad_id=$1 AND detalle->'despues'->>'equipo_id'=$2 ORDER BY creado_en DESC LIMIT 1`, original.ID, equipo).Scan(&anterior, &nuevo)
		if err != nil || anterior != original.EquipoID || nuevo != equipo {
			t.Fatalf("auditoría del reemplazo: %s -> %s: %v", anterior, nuevo, err)
		}
	}

}

func TestReposicionPrioridad_Cascada(t *testing.T) {
	for _, caso := range []string{"completa", "faltante", "empate", "bloqueo", "inicio protegido", "prefiere libre", "fallo revierte todo"} {
		t.Run(caso, func(t *testing.T) {
			f := escenarioDeReposicion(t)
			ctx := context.Background()
			_, rsTIC := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
			inicio := 10 * time.Hour
			if caso == "inicio protegido" {
				inicio = 9 * time.Hour
				f.svc = application.NewService(f.repo, NewValidadorMateriaPostgres(f.pool), NewValidadorEquipoPostgres(f.pool), jornadaLibre{}, NewObtenedorNombrePostgres(f.pool), NuevoID, func() time.Time { return f.fecha.Add(8*time.Hour + 30*time.Minute) }, f.bus)
			}
			gMath, rsMath := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha, inicio, 13*time.Hour, nil)
			var extra string
			if caso == "completa" || caso == "fallo revierte todo" {
				extra = computadoraDeReposicion(t, f.pool, "")
				marcarPreferencia(t, f.pool, extra, "Matemática", nil, nil, nil, 1)
			}
			if caso == "prefiere libre" {
				extra = computadoraDeReposicion(t, f.pool, "AutoCAD 2007")
				marcarPreferencia(t, f.pool, extra, "TIC", nil, nil, nil, 9)
			}
			if caso == "empate" {
				if _, err := f.pool.Exec(ctx, `UPDATE equipo_preferencia SET prioridad=1 WHERE equipo_id=$1 AND materia_nombre='Matemática'`, f.vieja); err != nil {
					t.Fatal(err)
				}
			}
			if caso == "bloqueo" {
				if _, err := f.pool.Exec(ctx, `UPDATE reserva SET tipo='BLOQUEO',reserva_grupo_id=NULL,materia_id=NULL,motivo_bloqueo='Evaluación' WHERE id=$1`, rsMath[0].ID); err != nil {
					t.Fatal(err)
				}
			}
			equipos := []string{f.moderna}
			if caso == "fallo revierte todo" {
				equipos = append(equipos, f.moderna)
			}
			impacto, err := f.tomar(t, equipos)
			if caso == "fallo revierte todo" {
				if err == nil {
					t.Fatal("debió fallar el lote duplicado")
				}
				f.comprobar(t, rsTIC[0], f.moderna, domain.ReservaConfirmada)
				f.comprobar(t, rsMath[0], f.vieja, domain.ReservaConfirmada)
				var n int
				if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM evento_pendiente`).Scan(&n); err != nil || n != 0 {
					t.Fatalf("eventos tras rollback: %d, %v", n, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch caso {
			case "completa":
				f.comprobar(t, rsTIC[0], f.vieja, domain.ReservaConfirmada)
				f.comprobar(t, rsMath[0], extra, domain.ReservaConfirmada)
				if impacto.ReservasReubicadas != 2 || impacto.ReservasCanceladas != 0 || impacto.DocentesAfectados != 2 {
					t.Fatalf("impacto %+v", impacto)
				}
			case "faltante":
				f.comprobar(t, rsTIC[0], f.vieja, domain.ReservaConfirmada)
				f.comprobar(t, rsMath[0], f.vieja, domain.ReservaCancelada)
				if impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 1 || impacto.DocentesAfectados != 2 {
					t.Fatalf("impacto %+v", impacto)
				}
				g, err := f.repo.BuscarReservaGrupoPorID(ctx, gMath.ID)
				if err != nil || g.Estado != domain.GrupoCancelada {
					t.Fatalf("grupo final: %+v, %v", g, err)
				}
			case "prefiere libre":
				f.comprobar(t, rsTIC[0], extra, domain.ReservaConfirmada)
				f.comprobar(t, rsMath[0], f.vieja, domain.ReservaConfirmada)
				if impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 0 || impacto.DocentesAfectados != 1 {
					t.Fatalf("impacto %+v", impacto)
				}
			default:
				f.comprobar(t, rsTIC[0], f.moderna, domain.ReservaCancelada)
				if caso != "bloqueo" {
					f.comprobar(t, rsMath[0], f.vieja, domain.ReservaConfirmada)
				}
				if impacto.ReservasReubicadas != 0 || impacto.ReservasCanceladas != 1 || impacto.DocentesAfectados != 1 {
					t.Fatalf("impacto %+v", impacto)
				}
			}
			procesados, err := f.bus.Despachar(ctx, 20)
			if err != nil {
				t.Fatal(err)
			}
			if procesados != impacto.DocentesAfectados || len(f.avisos) != procesados {
				t.Fatalf("avisos=%d procesados=%d impacto=%+v", len(f.avisos), procesados, impacto)
			}
		})
	}
}

func TestReposicionPrioridad_RecurrenciaYHorarioCompleto(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	regla, err := domain.NuevaReglaRecurrencia(NuevoID(), f.tic, f.docenteTIC, domain.Lunes, 10*time.Hour, 13*time.Hour, f.fecha, f.fecha.AddDate(0, 0, 14))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.CrearReglaRecurrencia(ctx, regla); err != nil {
		t.Fatal(err)
	}
	var originales []*domain.Reserva
	for _, fecha := range regla.GenerarFechas() {
		_, rs := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, fecha, 10*time.Hour, 13*time.Hour, &regla.ID)
		originales = append(originales, rs[0])
	}
	// La vieja sólo choca fuera del horario ganador, pero dentro del de TIC.
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha, 12*time.Hour, 13*time.Hour, nil)
	extra := computadoraDeReposicion(t, f.pool, "")
	marcarPreferencia(t, f.pool, extra, "Matemática", nil, nil, nil, 1)
	res, err := f.svc.CrearReservaRecurrente(ctx, f.dibujo, f.docenteDibujo, false, domain.Lunes, 11*time.Hour, 12*time.Hour, f.fecha, f.fecha.AddDate(0, 0, 7), []string{f.moderna}, []string{f.moderna})
	if err != nil {
		t.Fatal(err)
	}
	if res.Impacto.ReservasReubicadas != 3 || res.Impacto.ReservasCanceladas != 0 {
		t.Fatalf("impacto %+v", res.Impacto)
	}
	f.comprobar(t, originales[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, originales[1], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, originales[2], f.moderna, domain.ReservaConfirmada)
	f.comprobar(t, math[0], extra, domain.ReservaConfirmada)
	for _, r := range originales {
		g, err := f.repo.BuscarReservaGrupoPorID(ctx, *r.ReservaGrupoID)
		if err != nil || g.Estado != domain.GrupoConfirmada || g.ReglaRecurrenciaID == nil || *g.ReglaRecurrenciaID != regla.ID {
			t.Fatalf("grupo/serie: %+v %v", g, err)
		}
	}
	if n, err := f.bus.Despachar(ctx, 20); err != nil || n != 2 {
		t.Fatalf("avisos agrupados: %d %v", n, err)
	}
}

func TestReposicionPrioridad_NoDuplicaSustitutosNiRecorreCiclos(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	segunda := computadoraDeReposicion(t, f.pool, "Otro software")
	marcarPreferencia(t, f.pool, segunda, "Dibujo Técnico", nil, nil, nil, 1)
	marcarPreferencia(t, f.pool, segunda, "TIC", nil, nil, nil, 2)
	g, originales := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna, segunda}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	_, _, impacto, err := f.svc.CrearReserva(ctx, f.dibujo, f.docenteDibujo, false, f.fecha, 11*time.Hour, 12*time.Hour, []string{f.moderna, segunda}, []string{f.moderna, segunda})
	if err != nil {
		t.Fatal(err)
	}
	if impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 2 || impacto.DocentesAfectados != 2 {
		t.Fatalf("impacto %+v", impacto)
	}
	var confirmadas, canceladas int
	for _, r := range originales {
		actual, err := f.repo.BuscarReservaPorID(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if actual.Estado == domain.ReservaConfirmada {
			confirmadas++
			f.comprobar(t, r, f.vieja, domain.ReservaConfirmada)
		} else {
			canceladas++
		}
	}
	if confirmadas != 1 || canceladas != 1 {
		t.Fatalf("reservas duplicadas o perdidas: %d/%d", confirmadas, canceladas)
	}
	f.comprobar(t, math[0], f.vieja, domain.ReservaCancelada)
	actual, err := f.repo.BuscarReservaGrupoPorID(ctx, g.ID)
	if err != nil || actual.Estado != domain.GrupoParcialmenteCancelada {
		t.Fatalf("grupo: %+v %v", actual, err)
	}
}

func TestReposicionPrioridad_SinMarcasSoloUsaLibresDeLaMismaClase(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM equipo_preferencia WHERE materia_nombre IN ('TIC','Matemática')`); err != nil {
		t.Fatal(err)
	}
	_, originales := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	// El proyector está libre pero una materia sin marcas no lo recibe como PC.
	proyector := crearEquipoDeCarroDeTest(t, f.pool)
	if _, err := f.pool.Exec(ctx, `UPDATE equipo SET carro_id=NULL,identificador=NULL,tipo='PROYECTOR',nombre='Proyector libre' WHERE id=$1`, proyector); err != nil {
		t.Fatal(err)
	}
	extra := computadoraDeReposicion(t, f.pool, "Distinto software")
	impacto, err := f.tomar(t, []string{f.moderna})
	if err != nil {
		t.Fatal(err)
	}
	f.comprobar(t, originales[0], extra, domain.ReservaConfirmada)
	f.comprobar(t, math[0], f.vieja, domain.ReservaConfirmada)
	if impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 0 {
		t.Fatalf("impacto %+v", impacto)
	}
}

func TestReposicionPrioridad_NochesYFechasAdyacentes(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	_, tic := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, f.fecha, 22*time.Hour, 2*time.Hour, nil)
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha.AddDate(0, 0, 1), 0, 3*time.Hour, nil)
	extra := computadoraDeReposicion(t, f.pool, "")
	marcarPreferencia(t, f.pool, extra, "Matemática", nil, nil, nil, 1)
	_, _, impacto, err := f.svc.CrearReserva(ctx, f.dibujo, f.docenteDibujo, false, f.fecha, 23*time.Hour, time.Hour, []string{f.moderna}, []string{f.moderna})
	if err != nil {
		t.Fatal(err)
	}
	f.comprobar(t, tic[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, math[0], extra, domain.ReservaConfirmada)
	if impacto.ReservasReubicadas != 2 || impacto.ReservasCanceladas != 0 {
		t.Fatalf("impacto %+v", impacto)
	}
}

func TestReposicionPrioridad_DosDesplazadosCompitenPorElMismoSustituto(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	segunda := computadoraDeReposicion(t, f.pool, "AutoCAD 2026")
	marcarPreferencia(t, f.pool, segunda, "Dibujo Técnico", nil, nil, nil, 1)
	marcarPreferencia(t, f.pool, segunda, "TIC", nil, nil, nil, 2)
	marcarPreferencia(t, f.pool, f.moderna, "Matemática", nil, nil, nil, 3)
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.moderna}, f.docenteMatematica, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	_, tic := reservaConDuenoDeTest(t, f.pool, f.tic, []string{segunda}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	// Matemática entra primero al plan y obtiene provisionalmente la vieja.
	// TIC debe poder tomarla igualmente: gana su marca sobre ese dispositivo.
	for i, r := range []*domain.Reserva{math[0], tic[0]} {
		id := []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}[i]
		if _, err := f.pool.Exec(ctx, `UPDATE reserva SET id=$2 WHERE id=$1`, r.ID, id); err != nil {
			t.Fatal(err)
		}
		r.ID = id
	}
	_, _, impacto, err := f.svc.CrearReserva(ctx, f.dibujo, f.docenteDibujo, false, f.fecha, 11*time.Hour, 12*time.Hour, []string{f.moderna, segunda}, []string{f.moderna, segunda})
	if err != nil {
		t.Fatal(err)
	}
	f.comprobar(t, tic[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, math[0], f.moderna, domain.ReservaCancelada)
	if impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 1 || impacto.DocentesAfectados != 2 {
		t.Fatalf("impacto %+v", impacto)
	}
}

func TestReposicionPrioridad_UsaEquipoGanadorFueraDeSuHorario(t *testing.T) {
	f := escenarioDeReposicion(t)
	marcarPreferencia(t, f.pool, f.moderna, "Matemática", nil, nil, nil, 3)
	_, tic := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.vieja}, f.docenteMatematica, f.fecha, 12*time.Hour, 13*time.Hour, nil)
	impacto, err := f.tomar(t, []string{f.moderna})
	if err != nil {
		t.Fatal(err)
	}
	f.comprobar(t, tic[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, math[0], f.moderna, domain.ReservaConfirmada)
	if impacto.ReservasReubicadas != 2 || impacto.ReservasCanceladas != 0 {
		t.Fatalf("impacto %+v", impacto)
	}
}

func TestReposicionPrioridad_CambioConcurrenteEsperaLaCadena(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	_, tic := reservaConDuenoDeTest(t, f.pool, f.tic, []string{f.moderna}, f.docenteTIC, f.fecha, 10*time.Hour, 13*time.Hour, nil)
	extra := computadoraDeReposicion(t, f.pool, "")
	pausa := nuevaPausa(f.repo, true)
	svc := application.NewService(pausa, NewValidadorMateriaPostgres(f.pool), NewValidadorEquipoPostgres(f.pool), jornadaLibre{}, NewObtenedorNombrePostgres(f.pool), NuevoID, func() time.Time { return f.fecha.AddDate(0, 0, -1) }, f.bus)
	creacion := make(chan error, 1)
	go func() {
		_, _, _, err := svc.CrearReserva(ctx, f.dibujo, f.docenteDibujo, false, f.fecha, 11*time.Hour, 12*time.Hour, []string{f.moderna}, []string{f.moderna})
		creacion <- err
	}()
	esperarPausa(t, ctx, pausa)
	cambio := make(chan error, 1)
	go func() {
		_, err := f.svc.CambiarEquipoDeReserva(ctx, tic[0].ID, extra, f.docenteTIC, false, true)
		cambio <- err
	}()
	for {
		var bloqueada bool
		if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%pg_advisory_xact_lock_shared%')`).Scan(&bloqueada); err != nil {
			t.Fatal(err)
		}
		if bloqueada {
			break
		}
		select {
		case err := <-cambio:
			t.Fatalf("cambió a mitad de la cadena: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(pausa.seguir)
	if err := <-creacion; err != nil {
		t.Fatal(err)
	}
	if err := <-cambio; err != nil {
		t.Fatal(err)
	}
	antesDelCambio := *tic[0]
	antesDelCambio.EquipoID = f.vieja
	f.comprobar(t, &antesDelCambio, extra, domain.ReservaConfirmada)
}

func TestReposicionPrioridad_RevisaFaltantesCuandoUnaFranjaSeLibera(t *testing.T) {
	f := escenarioDeReposicion(t)
	ctx := context.Background()
	construccion := materiaEnCursoDeTest(t, f.pool, "Construcción", "6°A")
	docente := crearUsuarioDeTest(t, f.pool, "DOCENTE", "APROBADA")
	marcarPreferencia(t, f.pool, f.moderna, "Matemática", nil, nil, nil, 3)
	segunda := computadoraDeReposicion(t, f.pool, "")
	tercera := computadoraDeReposicion(t, f.pool, "")
	for _, id := range []string{segunda, tercera} {
		marcarPreferencia(t, f.pool, id, "Dibujo Técnico", nil, nil, nil, 1)
	}
	marcarPreferencia(t, f.pool, segunda, "TIC", nil, nil, nil, 2)
	marcarPreferencia(t, f.pool, tercera, "Construcción", nil, nil, nil, 2)
	marcarPreferencia(t, f.pool, f.vieja, "Construcción", nil, nil, nil, 1)
	if _, err := f.pool.Exec(ctx, `UPDATE equipo_preferencia SET prioridad=2 WHERE equipo_id=$1 AND materia_nombre='TIC'`, f.vieja); err != nil {
		t.Fatal(err)
	}
	_, math := reservaConDuenoDeTest(t, f.pool, f.matematica, []string{f.moderna}, f.docenteMatematica, f.fecha, 9*time.Hour, 11*time.Hour, nil)
	_, tic := reservaConDuenoDeTest(t, f.pool, f.tic, []string{segunda}, f.docenteTIC, f.fecha, 10*time.Hour, 12*time.Hour, nil)
	_, cons := reservaConDuenoDeTest(t, f.pool, construccion, []string{tercera}, docente, f.fecha, 11*time.Hour, 13*time.Hour, nil)
	ids := []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003"}
	// Orden por inicio: Matemática, TIC, Construcción. La vieja pasa de
	// 9–11 a 10–12 a 11–13, y al final vuelve a admitir Matemática 9–11.
	for i, r := range []*domain.Reserva{math[0], tic[0], cons[0]} {
		if _, err := f.pool.Exec(ctx, `UPDATE reserva SET id=$2 WHERE id=$1`, r.ID, ids[i]); err != nil {
			t.Fatal(err)
		}
		r.ID = ids[i]
	}
	equipos := []string{f.moderna, segunda, tercera}
	_, _, impacto, err := f.svc.CrearReserva(ctx, f.dibujo, f.docenteDibujo, false, f.fecha, 10*time.Hour+30*time.Minute, 11*time.Hour+30*time.Minute, equipos, equipos)
	if err != nil {
		t.Fatal(err)
	}
	f.comprobar(t, math[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, cons[0], f.vieja, domain.ReservaConfirmada)
	f.comprobar(t, tic[0], segunda, domain.ReservaCancelada)
	if impacto.ReservasReubicadas != 2 || impacto.ReservasCanceladas != 1 || impacto.DocentesAfectados != 3 {
		t.Fatalf("impacto %+v", impacto)
	}
}
