//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	academicapp "github.com/agustin-y2k/sgrc/internal/academic/application"
	academicdomain "github.com/agustin-y2k/sgrc/internal/academic/domain"
	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	reportingapp "github.com/agustin-y2k/sgrc/internal/reporting/application"
	reportingdomain "github.com/agustin-y2k/sgrc/internal/reporting/domain"
	reportinginfra "github.com/agustin-y2k/sgrc/internal/reporting/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationdomain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Se ejercita la composición real: los tres módulos deben usar la MISMA
// transacción. Los errores se inyectan después de escrituras efectivas.
var errFalloDeCierre = errors.New("fallo inyectado en el cierre")

type repoAcademicConFallo struct {
	academicapp.Repo
	etapa string
}

func (r repoAcademicConFallo) ArchivarCiclo(ctx context.Context, id string) error {
	if err := r.Repo.ArchivarCiclo(ctx, id); err != nil {
		return err
	}
	if r.etapa == "archivo" {
		return errFalloDeCierre
	}
	return nil
}
func (r repoAcademicConFallo) ClonarCicloA(ctx context.Context, id string, ciclo *academicdomain.CicloLectivo) (int, int, error) {
	c, m, err := r.Repo.ClonarCicloA(ctx, id, ciclo)
	if err == nil && r.etapa == "clonado" {
		err = errFalloDeCierre
	}
	return c, m, err
}
func (r repoAcademicConFallo) GuardarCierre(ctx context.Context, id string, c *academicapp.CierreCompletado) error {
	if err := r.Repo.GuardarCierre(ctx, id, c); err != nil {
		return err
	}
	if r.etapa == "resultado" {
		return errFalloDeCierre
	}
	return nil
}

type repoReportingConFallo struct {
	reportingapp.Repo
	etapa string
}

func (r repoReportingConFallo) GuardarHistoricoUsoDocente(ctx context.Context, h *reportingdomain.HistoricoUsoDocente) error {
	if err := r.Repo.GuardarHistoricoUsoDocente(ctx, h); err != nil {
		return err
	}
	if r.etapa == "snapshot" {
		return errFalloDeCierre
	}
	return nil
}

type datosCierre struct{ ciclo, curso, espacio, materia, materiaEspacio, equipo, docente, grupo, regla string }

func ejecutarCierreSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("SQL de prueba: %v", err)
	}
}

func prepararCierre(t *testing.T, pool *pgxpool.Pool) datosCierre {
	t.Helper()
	ejecutarCierreSQL(t, pool, `TRUNCATE usuario,ciclo_lectivo,carro,equipo,historico_uso_equipo,historico_uso_docente CASCADE`)
	d := datosCierre{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	ejecutarCierreSQL(t, pool, `INSERT INTO ciclo_lectivo(id,anio,activo) VALUES($1,2026,true)`, d.ciclo)
	ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division) VALUES($1,$2,1,'A')`, d.curso, d.ciclo)
	ejecutarCierreSQL(t, pool, `INSERT INTO espacio(id,ciclo_lectivo_id,nombre) VALUES($1,$2,'Biblioteca')`, d.espacio, d.ciclo)
	ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,'Matemática')`, d.materia, d.curso)
	ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,espacio_id,nombre) VALUES($1,$2,'Biblioteca')`, d.materiaEspacio, d.espacio)
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Notebook de prueba','Computadora')`, d.equipo)
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Ada','Docente','ada@test.local','hash','DOCENTE','APROBADA')`, d.docente)
	ejecutarCierreSQL(t, pool, `INSERT INTO docente_materia(usuario_id,materia_id,rol) VALUES($1,$2,'TITULAR')`, d.docente, d.materia)
	ejecutarCierreSQL(t, pool, `INSERT INTO regla_recurrencia(id,materia_id,creado_por,dia_semana,hora_inicio,hora_fin,fecha_inicio,fecha_fin) VALUES($1,$2,$3,'LUNES','08:00','09:00','2026-05-04','2026-05-18')`, d.regla, d.materia, d.docente)
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,regla_recurrencia_id) VALUES($1,$2,$3,'Ada Docente','2026-05-04','08:00','09:00',$4)`, d.grupo, d.materia, d.docente, d.regla)
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva(reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,estado) VALUES($1,$2,$3,$4,'Ada Docente','2026-05-04','08:00','09:00','FINALIZADA')`, d.grupo, d.equipo, d.materia, d.docente)
	// Los bloqueos aportan uso por equipo, pero no pertenecen a docentes.
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva(equipo_id,fecha,hora_inicio,hora_fin,tipo,motivo_bloqueo,estado) VALUES($1,'2026-05-04','10:00','10:30','BLOQUEO','Evaluación','FINALIZADA')`, d.equipo)
	return d
}

func serviciosCierre(pool *pgxpool.Pool, etapa string) (*academicapp.Service, *reportingapp.Service) {
	report := reportingapp.NewService(repoReportingConFallo{reportinginfra.NewPostgresRepo(pool), etapa}, reportinginfra.NewInfoEquipoPostgres(pool), reportinginfra.NewInfoUsuarioPostgres(pool), uuid.NewString)
	reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, nil, nil, nil, uuid.NewString, time.Now, nil)
	adaptador := &academicArchivadorHistoricoAdapter{reportingSvc: report, reservationSvc: reservas}
	academico := academicapp.NewService(repoAcademicConFallo{academicinfra.NewPostgresRepo(pool), etapa}, nil, nil, adaptador, nil, nil, nil, uuid.NewString, time.Now, nil)
	return academico, report
}

// Todas estas filas deben volver al estado original si cualquier etapa falla.
func verificarCierreRevertido(t *testing.T, pool *pgxpool.Pool, d datosCierre) {
	t.Helper()
	var activo, archivado bool
	if err := pool.QueryRow(context.Background(), `SELECT activo,archivado FROM ciclo_lectivo WHERE id=$1`, d.ciclo).Scan(&activo, &archivado); err != nil {
		t.Fatal(err)
	}
	if !activo || archivado {
		t.Fatalf("ciclo modificado tras el fallo: activo=%v archivado=%v", activo, archivado)
	}
	for tabla, cantidad := range map[string]int{"ciclo_lectivo": 1, "curso": 1, "espacio": 1, "materia": 2, "reserva_grupo": 1, "reserva": 2, "regla_recurrencia": 1, "historico_uso_equipo": 0, "historico_uso_docente": 0, "snapshot_ciclo": 0, "cierre_ciclo": 0} {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM `+tabla).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != cantidad {
			t.Errorf("%s: quedaron %d filas, esperaba %d", tabla, n, cantidad)
		}
	}
	var archivadas int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM curso WHERE archivado)+(SELECT COUNT(*) FROM espacio WHERE archivado)+(SELECT COUNT(*) FROM materia WHERE archivado)`).Scan(&archivadas); err != nil {
		t.Fatal(err)
	}
	if archivadas != 0 {
		t.Fatal("quedó estructura archivada después del rollback")
	}
}

func verificarCierreCompleto(t *testing.T, pool *pgxpool.Pool, d datosCierre, res *academicapp.ResultadoArchivado, clonado bool) {
	t.Helper()
	if clonado && (res.NuevoCicloID == nil || res.CursosClonados != 1 || res.MateriasClonadas != 2) {
		t.Fatalf("resultado de clonado incorrecto: %+v", res)
	}
	for tabla := range map[string]bool{"reserva": true, "reserva_grupo": true, "regla_recurrencia": true} {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM `+tabla).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("quedaron %d filas en %s", n, tabla)
		}
	}
	var activo, archivado bool
	if err := pool.QueryRow(context.Background(), `SELECT activo,archivado FROM ciclo_lectivo WHERE id=$1`, d.ciclo).Scan(&activo, &archivado); err != nil {
		t.Fatal(err)
	}
	if activo || !archivado {
		t.Fatal("el ciclo no quedó archivado")
	}
	var eqReservas, eqMinutos, docReservas, docMinutos, estructura int
	if err := pool.QueryRow(context.Background(), `SELECT cantidad_reservas,minutos_reservados FROM historico_uso_equipo WHERE anio=2026`).Scan(&eqReservas, &eqMinutos); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT cantidad_reservas,minutos_totales FROM historico_uso_docente WHERE anio=2026`).Scan(&docReservas, &docMinutos); err != nil {
		t.Fatal(err)
	}
	if eqReservas != 2 || eqMinutos != 90 || docReservas != 1 || docMinutos != 60 {
		t.Fatalf("totales incorrectos: equipo %d/%d, docente %d/%d", eqReservas, eqMinutos, docReservas, docMinutos)
	}
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM curso WHERE archivado)+(SELECT COUNT(*) FROM espacio WHERE archivado)+(SELECT COUNT(*) FROM materia WHERE archivado)`).Scan(&estructura); err != nil {
		t.Fatal(err)
	}
	if estructura != 4 {
		t.Fatalf("archivo incompleto de cursos, espacios y materias: %d", estructura)
	}
	if clonado {
		var cursos, espacios, materias, asignaciones int
		if err := pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM curso WHERE ciclo_lectivo_id=$1 AND NOT archivado),(SELECT COUNT(*) FROM espacio WHERE ciclo_lectivo_id=$1 AND NOT archivado),(SELECT COUNT(*) FROM contenedor_de_materia WHERE ciclo_lectivo_id=$1 AND NOT archivado),(SELECT COUNT(*) FROM docente_materia dm JOIN contenedor_de_materia c ON c.materia_id=dm.materia_id WHERE c.ciclo_lectivo_id=$1)`, *res.NuevoCicloID).Scan(&cursos, &espacios, &materias, &asignaciones); err != nil {
			t.Fatal(err)
		}
		if cursos != 1 || espacios != 1 || materias != 2 || asignaciones != 0 {
			t.Fatalf("estructura clonada incorrecta: %d/%d/%d/%d", cursos, espacios, materias, asignaciones)
		}
	}
}

func TestCierreAnual_AtomicoYReintentable(t *testing.T) {
	pool := baseVacia(t)
	for _, etapa := range []string{"snapshot", "archivo", "limpieza-grupos", "limpieza-reglas", "limpieza-bloqueos", "clonado", "resultado"} {
		t.Run(etapa, func(t *testing.T) {
			d := prepararCierre(t, pool)
			if etapa == "limpieza-grupos" || etapa == "limpieza-reglas" || etapa == "limpieza-bloqueos" {
				tabla := "regla_recurrencia"
				if etapa == "limpieza-grupos" {
					tabla = "reserva_grupo"
				}
				if etapa == "limpieza-bloqueos" {
					tabla = "reserva"
				}
				ejecutarCierreSQL(t, pool, `CREATE FUNCTION fallar_borrado_cierre() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo inyectado de limpieza'; END $$`)
				condicion := ""
				if etapa == "limpieza-bloqueos" {
					condicion = "WHEN (OLD.tipo='BLOQUEO')"
				}
				ejecutarCierreSQL(t, pool, fmt.Sprintf(`CREATE TRIGGER fallo_de_limpieza BEFORE DELETE ON %s FOR EACH ROW %s EXECUTE FUNCTION fallar_borrado_cierre()`, tabla, condicion))
				t.Cleanup(func() {
					ejecutarCierreSQL(t, pool, fmt.Sprintf(`DROP TRIGGER IF EXISTS fallo_de_limpieza ON %s`, tabla))
					ejecutarCierreSQL(t, pool, `DROP FUNCTION IF EXISTS fallar_borrado_cierre()`)
				})
			}
			svc, _ := serviciosCierre(pool, etapa)
			anio := 2027
			if res, err := svc.ArchivarYClonar(context.Background(), d.ciclo, &anio); err == nil || res != nil {
				t.Fatalf("esperaba fallo sin resultado: %+v %v", res, err)
			}
			verificarCierreRevertido(t, pool, d)
			if etapa == "limpieza-grupos" || etapa == "limpieza-reglas" || etapa == "limpieza-bloqueos" {
				tabla := "regla_recurrencia"
				if etapa == "limpieza-grupos" {
					tabla = "reserva_grupo"
				}
				if etapa == "limpieza-bloqueos" {
					tabla = "reserva"
				}
				ejecutarCierreSQL(t, pool, fmt.Sprintf(`DROP TRIGGER fallo_de_limpieza ON %s`, tabla))
			}
			svc, _ = serviciosCierre(pool, "")
			res, err := svc.ArchivarYClonar(context.Background(), d.ciclo, &anio)
			if err != nil {
				t.Fatalf("reintento: %v", err)
			}
			verificarCierreCompleto(t, pool, d, res, true)
		})
	}
}

func TestCierreAnual_RepeticionesYConcurrencia(t *testing.T) {
	pool := baseVacia(t)
	for _, clonar := range []bool{false, true} {
		t.Run(fmt.Sprint(clonar), func(t *testing.T) {
			d := prepararCierre(t, pool)
			svc, report := serviciosCierre(pool, "")
			var destino *int
			anio := 2027
			if clonar {
				destino = &anio
			}
			type respuesta struct {
				res *academicapp.ResultadoArchivado
				err error
			}
			inicio := make(chan struct{})
			salidas := make(chan respuesta, 2)
			for range 2 {
				go func() {
					<-inicio
					r, e := svc.ArchivarYClonar(context.Background(), d.ciclo, destino)
					salidas <- respuesta{r, e}
				}()
			}
			close(inicio)
			a, b := <-salidas, <-salidas
			if a.err != nil || b.err != nil {
				t.Fatalf("cierres simultáneos: %v / %v", a.err, b.err)
			}
			if !reflect.DeepEqual(a.res, b.res) {
				t.Fatalf("respuestas distintas: %+v / %+v", a.res, b.res)
			}
			verificarCierreCompleto(t, pool, d, a.res, clonar)
			antes, err := report.HistoricoUsoDocentes(context.Background(), 2026)
			if err != nil {
				t.Fatal(err)
			}
			if err := report.ArchivarSnapshotDeCiclo(context.Background(), d.ciclo, 2026); err != nil {
				t.Fatal(err)
			}
			despues, err := report.HistoricoUsoDocentes(context.Background(), 2026)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(antes, despues) {
				t.Fatal("se regeneró el snapshot sellado con la fuente vacía")
			}
			repetido, err := svc.ArchivarYClonar(context.Background(), d.ciclo, destino)
			if err != nil || !reflect.DeepEqual(a.res, repetido) {
				t.Fatalf("respuesta perdida: %+v %v", repetido, err)
			}
			otro := 2028
			if _, err := svc.ArchivarYClonar(context.Background(), d.ciclo, &otro); !errors.Is(err, academicapp.ErrDestinoDeCierreDiferente) {
				t.Fatalf("intención distinta: %v", err)
			}
			// La defensa final también cubre reservas administrativas sin materia.
			repo := reservationinfra.NewPostgresRepo(pool)
			bloqueo := &reservationdomain.Reserva{ID: uuid.NewString(), EquipoID: d.equipo, Fecha: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC), HoraInicio: 12 * time.Hour, HoraFin: 13 * time.Hour, Estado: reservationdomain.EstadoReserva("CONFIRMADA"), Tipo: reservationdomain.TipoReserva("BLOQUEO"), MotivoBloqueo: "Tardío", CreadaEn: time.Now()}
			if err := repo.CrearReserva(context.Background(), bloqueo); !errors.Is(err, reservationapp.ErrCicloCerrado) {
				t.Fatalf("reserva tardía: %v", err)
			}
			// La eliminación de cuenta solo desvincula la FK del histórico.
			ejecutarCierreSQL(t, pool, `DELETE FROM usuario WHERE id=$1`, d.docente)
			hist, err := report.HistoricoUsoDocentes(context.Background(), 2026)
			if err != nil {
				t.Fatal(err)
			}
			if len(hist) != 1 || hist[0].UsuarioID != nil || hist[0].DocenteIDHistorico != d.docente || hist[0].CantidadReservas != 1 {
				t.Fatalf("se perdió el histórico al eliminar la cuenta: %+v", hist)
			}
		})
	}
}

func TestCierreAnual_IdentidadHistoricaDelDocente(t *testing.T) {
	pool := baseVacia(t)
	d := prepararCierre(t, pool)
	// La misma persona reservó con nombres distintos, incluso en un espacio.
	grupo2 := uuid.NewString()
	ejecutarCierreSQL(t, pool, `UPDATE usuario SET nombre='Ada Renombrada' WHERE id=$1`, d.docente)
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Ada Renombrada Docente','2026-05-05','22:00','01:00')`, grupo2, d.materiaEspacio, d.docente)
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva(reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,estado) VALUES($1,$2,$3,$4,'Ada Renombrada Docente','2026-05-05','22:00','01:00','FINALIZADA')`, grupo2, d.equipo, d.materiaEspacio, d.docente)
	// Dos homónimos eliminados conservan identidades diferentes.
	for i := range 2 {
		usuario, grupo := uuid.NewString(), uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Alex','Igual',$2,'hash','DOCENTE','APROBADA')`, usuario, usuario+"@test.local")
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Alex Igual',$4,'08:00','09:00')`, grupo, d.materia, usuario, time.Date(2026, 5, 6+i, 0, 0, 0, 0, time.UTC))
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva(reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,estado) VALUES($1,$2,$3,$4,'Alex Igual',$5,'08:00','09:00','FINALIZADA')`, grupo, d.equipo, d.materia, usuario, time.Date(2026, 5, 6+i, 0, 0, 0, 0, time.UTC))
		ejecutarCierreSQL(t, pool, `DELETE FROM usuario WHERE id=$1`, usuario)
	}
	svc, report := serviciosCierre(pool, "")
	vivo, err := report.ReporteUsoDocentes(context.Background(), d.ciclo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vivo) != 3 {
		t.Fatalf("el reporte divide nombres o fusiona homónimos: %+v", vivo)
	}
	if _, err := svc.ArchivarYClonar(context.Background(), d.ciclo, nil); err != nil {
		t.Fatal(err)
	}
	hist, err := report.HistoricoUsoDocentes(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("identidades históricas: %+v", hist)
	}
	var reservas, minutos int
	identidades := map[string]bool{}
	for _, h := range hist {
		reservas += h.CantidadReservas
		minutos += h.MinutosTotales
		if identidades[h.DocenteIDHistorico] {
			t.Fatal("identidad duplicada")
		}
		identidades[h.DocenteIDHistorico] = true
		if h.UsuarioID != nil && (h.CantidadReservas != 2 || h.MinutosTotales != 240 || h.NombreDocenteSnapshot != "Ada Renombrada Docente") {
			t.Fatalf("uso dividido por cambio de nombre: %+v", h)
		}
	}
	if reservas != 4 || minutos != 360 {
		t.Fatalf("histórico perdió uso: %d / %d", reservas, minutos)
	}
}

// Un escritor que ya tomó el bloqueo compartido debe completar primero. El
// cierre espera, vuelve a leer si hay conflicto y captura también ese uso.
func TestCierreAnual_ReservaEnCursoDeEscritura(t *testing.T) {
	pool := baseVacia(t)
	d := prepararCierre(t, pool)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	grupo := uuid.NewString()
	if _, err := tx.Exec(context.Background(), `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin) VALUES($1,$2,$3,'Ada Docente','2026-05-05','08:00','09:00')`, grupo, d.materia, d.docente); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `INSERT INTO reserva(reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,estado) VALUES($1,$2,$3,$4,'Ada Docente','2026-05-05','08:00','09:00','FINALIZADA')`, grupo, d.equipo, d.materia, d.docente); err != nil {
		t.Fatal(err)
	}
	svc, report := serviciosCierre(pool, "")
	termino := make(chan error, 1)
	go func() { _, err := svc.ArchivarYClonar(context.Background(), d.ciclo, nil); termino <- err }()
	// Esperar el bloqueo real evita depender del tiempo del planificador.
	limite := time.Now().Add(5 * time.Second)
	for {
		var espera bool
		if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT id FROM ciclo_lectivo%')`).Scan(&espera); err != nil {
			t.Fatal(err)
		}
		if espera {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("el cierre no esperó al escritor de reservas")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-termino; err != nil {
		t.Fatalf("cierre concurrente: %v", err)
	}
	hist, err := report.HistoricoUsoDocentes(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].CantidadReservas != 2 || hist[0].MinutosTotales != 120 {
		t.Fatalf("se perdió la escritura concurrente: %+v", hist)
	}
}

func TestCierreAnual_LimpiezaPorSiSolaEsAtomica(t *testing.T) {
	pool := baseVacia(t)
	for _, tabla := range []string{"regla_recurrencia", "reserva"} {
		t.Run(tabla, func(t *testing.T) {
			d := prepararCierre(t, pool)
			ejecutarCierreSQL(t, pool, `CREATE FUNCTION fallar_limpieza_aislada() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fallo de limpieza'; END $$`)
			condicion := ""
			if tabla == "reserva" {
				condicion = "WHEN (OLD.tipo='BLOQUEO')"
			}
			ejecutarCierreSQL(t, pool, fmt.Sprintf(`CREATE TRIGGER fallo_de_limpieza BEFORE DELETE ON %s FOR EACH ROW %s EXECUTE FUNCTION fallar_limpieza_aislada()`, tabla, condicion))
			t.Cleanup(func() {
				ejecutarCierreSQL(t, pool, fmt.Sprintf(`DROP TRIGGER IF EXISTS fallo_de_limpieza ON %s`, tabla))
				ejecutarCierreSQL(t, pool, `DROP FUNCTION IF EXISTS fallar_limpieza_aislada()`)
			})
			repo := reservationinfra.NewPostgresRepo(pool)
			g, r, err := repo.EliminarReservasYGruposDeCiclo(context.Background(), d.ciclo)
			if err == nil || g != 0 || r != 0 {
				t.Fatalf("limpieza parcial reportada como éxito: %d/%d %v", g, r, err)
			}
			verificarCierreRevertido(t, pool, d)
			ejecutarCierreSQL(t, pool, fmt.Sprintf(`DROP TRIGGER fallo_de_limpieza ON %s`, tabla))
			g, r, err = repo.EliminarReservasYGruposDeCiclo(context.Background(), d.ciclo)
			if err != nil || g != 1 || r != 2 {
				t.Fatalf("reintento de limpieza: %d/%d %v", g, r, err)
			}
			g, r, err = repo.EliminarReservasYGruposDeCiclo(context.Background(), d.ciclo)
			if err != nil || g != 0 || r != 0 {
				t.Fatalf("limpieza repetida: %d/%d %v", g, r, err)
			}
		})
	}
}

func TestCierreAnual_SnapshotRechazaAnioAjenoSinEscriturasParciales(t *testing.T) {
	pool := baseVacia(t)
	d := prepararCierre(t, pool)
	_, report := serviciosCierre(pool, "")
	if err := report.ArchivarSnapshotDeCiclo(context.Background(), d.ciclo, 2025); err == nil {
		t.Fatal("se aceptó un año que no corresponde al ciclo")
	}
	verificarCierreRevertido(t, pool, d)
	if err := report.ArchivarSnapshotDeCiclo(context.Background(), d.ciclo, 2026); err != nil {
		t.Fatal(err)
	}
	// Una escritura individual tampoco puede agregar filas a un año sellado.
	otro := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Otra notebook','Computadora')`, otro)
	h, err := reportingdomain.NuevoHistoricoUsoEquipo(uuid.NewString(), 2026, otro, "Otra notebook", 0, "", 60, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := reportinginfra.NewPostgresRepo(pool).GuardarHistoricoUsoEquipo(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	hist, err := report.HistoricoUsoEquipos(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].EquipoID != d.equipo || hist[0].MinutosReservados != 90 {
		t.Fatal("una escritura alteró el histórico sellado")
	}
}

func TestCierreAnual_ReservaQueEsperaElCierreNoReabreElAnio(t *testing.T) {
	pool := baseVacia(t)
	d := prepararCierre(t, pool)
	svc, _ := serviciosCierre(pool, "")
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	bloqueado, continuar := make(chan struct{}), make(chan struct{})
	type respuesta struct {
		res *academicapp.ResultadoArchivado
		err error
	}
	cierre := make(chan respuesta, 1)
	go func() {
		var res *academicapp.ResultadoArchivado
		err := academicinfra.NewPostgresRepo(pool).EnCierre(ctx, d.ciclo, func(ctx context.Context) error {
			close(bloqueado)
			select {
			case <-continuar:
			case <-ctx.Done():
				return ctx.Err()
			}
			var err error
			res, err = svc.ArchivarYClonar(ctx, d.ciclo, nil)
			return err
		})
		cierre <- respuesta{res, err}
	}()
	select {
	case <-bloqueado:
	case <-ctx.Done():
		t.Fatal("el cierre no tomó el bloqueo")
	}
	escritura := make(chan error, 1)
	grupo := &reservationdomain.ReservaGrupo{ID: uuid.NewString(), MateriaID: d.materia, CreadoPor: &d.docente, NombreDocenteSnapshot: "Ada Docente", Fecha: time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC), HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour, Estado: reservationdomain.EstadoReservaGrupo("CONFIRMADA"), CreadaEn: time.Now()}
	go func() { escritura <- reservationinfra.NewPostgresRepo(pool).CrearReservaGrupo(ctx, grupo) }()
	for {
		var espera bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%INSERT INTO reserva_grupo%')`).Scan(&espera); err != nil {
			t.Fatal(err)
		}
		if espera {
			break
		}
		select {
		case err := <-escritura:
			t.Fatalf("la reserva no esperó al cierre: %v", err)
		case <-ctx.Done():
			t.Fatal("no se observó la espera")
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(continuar)
	r := <-cierre
	if r.err != nil {
		t.Fatalf("cierre: %v", r.err)
	}
	if err := <-escritura; !errors.Is(err, reservationapp.ErrCicloCerrado) {
		t.Fatalf("la reserva tardía reabrió el año: %v", err)
	}
	verificarCierreCompleto(t, pool, d, r.res, false)
}
