//go:build integration

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	academicapp "github.com/agustin-y2k/sgrc/internal/academic/application"
	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	"github.com/google/uuid"
)

func TestCursoPrioridades_ImpactoConservacionYAtomicidad(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	for _, caso := range []string{"anio", "division", "modalidad", "compartida", "destino-igual", "destino-distinto", "aceptar", "obsoleto", "curso-obsoleto", "duplicado", "generales", "normalizacion", "concurrente"} {
		t.Run(caso, func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			ejecutarCierreSQL(t, pool, `UPDATE curso SET anio=3,division='2',modalidad='Construcción' WHERE id=$1`, e.d.curso)
			marca := uuid.NewString()
			ejecutarCierreSQL(t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,anio,division,modalidad,prioridad,curso_exacto) VALUES($1,$2,'matematica',3,'2','Construcción',1,true)`, marca, e.d.equipo)
			svc := academicapp.NewService(academicinfra.NewPostgresRepo(pool), nil, nil, nil, nil, nil, &academicMarcasAdapter{e.inventario}, uuid.NewString, time.Now, nil)
			anio, division, modalidad := 3, "2", "Construcción"
			switch caso {
			case "anio":
				anio = 4
			case "division":
				division = "3"
			case "normalizacion":
				modalidad = "CONSTRUCCION"
			default:
				modalidad = "Electrónica"
			}
			if caso == "compartida" || caso == "destino-igual" || caso == "destino-distinto" {
				ciclo, curso := uuid.NewString(), uuid.NewString()
				ejecutarCierreSQL(t, pool, `INSERT INTO ciclo_lectivo(id,anio,activo,archivado) VALUES($1,2025,false,true)`, ciclo)
				ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division,modalidad,archivado) VALUES($1,$2,3,'2','Construcción',true)`, curso, ciclo)
				ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre,archivado) VALUES($1,$2,'Matemática',true)`, uuid.NewString(), curso)
			}
			if caso == "destino-igual" || caso == "destino-distinto" {
				prioridad := 1
				if caso == "destino-distinto" {
					prioridad = 4
				}
				ejecutarCierreSQL(t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,anio,division,modalidad,prioridad,curso_exacto) VALUES($1,$2,'Matemática',3,'2','Electrónica',$3,true)`, uuid.NewString(), e.d.equipo, prioridad)
			}
			if caso == "duplicado" {
				ejecutarCierreSQL(t, pool, `INSERT INTO curso(id,ciclo_lectivo_id,anio,division,modalidad) VALUES($1,$2,3,'2','Electrónica')`, uuid.NewString(), e.d.ciclo)
			}
			if caso == "generales" {
				ejecutarCierreSQL(t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,modalidad,prioridad) VALUES($1,$2,'Matemática','Construcción',5),($3,$2,'Matemática','Electrónica',7),($4,$2,'Matemática',NULL,9)`, uuid.NewString(), e.d.equipo, uuid.NewString(), uuid.NewString())
			}
			var reservaAntes string
			if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, e.reservaID).Scan(&reservaAntes); err != nil {
				t.Fatal(err)
			}
			impacto, err := svc.PrevisualizarEdicionCurso(ctx, e.d.curso, anio, division, modalidad)
			if err != nil {
				t.Fatal(err)
			}
			if caso == "normalizacion" {
				if len(impacto.Cambios) != 0 {
					t.Fatalf("la normalización cambió el alcance: %+v", impacto)
				}
				if err := svc.EditarCurso(ctx, e.d.curso, anio, division, modalidad); err != nil {
					t.Fatal(err)
				}
				return
			}
			if len(impacto.Cambios) == 0 {
				t.Fatal("no detectó las marcas")
			}
			if err := svc.EditarCurso(ctx, e.d.curso, anio, division, modalidad); !errors.Is(err, academicapp.ErrRevisarPrioridadesCurso) {
				t.Fatalf("edición sin revisión: %v", err)
			}
			decision := academicapp.ConservarExactas
			if caso == "aceptar" {
				decision = academicapp.AceptarAlcance
			}
			if caso == "obsoleto" {
				ejecutarCierreSQL(t, pool, `UPDATE equipo_preferencia SET prioridad=2 WHERE id=$1`, marca)
			}
			if caso == "curso-obsoleto" {
				ejecutarCierreSQL(t, pool, `UPDATE curso SET division='4' WHERE id=$1`, e.d.curso)
			}
			if caso == "concurrente" {
				resultados := make(chan error, 2)
				for range 2 {
					go func() {
						resultados <- svc.EditarCurso(ctx, e.d.curso, anio, division, modalidad, academicapp.ConfirmacionCurso{Version: impacto.Version, Decision: decision})
					}()
				}
				exitos, obsoletos := 0, 0
				for range 2 {
					err := <-resultados
					if err == nil {
						exitos++
					} else if errors.Is(err, academicapp.ErrImpactoCursoDesactualizado) {
						obsoletos++
					} else {
						t.Fatal(err)
					}
				}
				if exitos != 1 || obsoletos != 1 {
					t.Fatalf("concurrencia: exitos=%d obsoletos=%d", exitos, obsoletos)
				}
			} else {
				err = svc.EditarCurso(ctx, e.d.curso, anio, division, modalidad, academicapp.ConfirmacionCurso{Version: impacto.Version, Decision: decision})
			}
			var esperado error
			switch caso {
			case "destino-distinto":
				esperado = academicapp.ErrConflictoPrioridadesCurso
			case "obsoleto", "curso-obsoleto":
				esperado = academicapp.ErrImpactoCursoDesactualizado
			case "duplicado":
				esperado = academicapp.ErrCursoNombreDuplicado
			}
			if !errors.Is(err, esperado) {
				t.Fatalf("resultado: %v esperado: %v", err, esperado)
			}
			if esperado != nil {
				var original int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM equipo_preferencia WHERE id=$1 AND modalidad='Construcción' AND anio=3 AND division='2'`, marca).Scan(&original); err != nil || original != 1 {
					t.Fatalf("se perdieron marcas tras el rechazo: %d %v", original, err)
				}
				if caso == "destino-distinto" {
					if err := svc.EditarCurso(ctx, e.d.curso, anio, division, modalidad, academicapp.ConfirmacionCurso{Version: impacto.Version, Decision: academicapp.AceptarAlcance}); err != nil {
						t.Fatal(err)
					}
					var prioridad int
					if err := pool.QueryRow(ctx, `SELECT ep.prioridad FROM equipo_preferencia ep JOIN curso c ON c.id=$1 WHERE ep.equipo_id=$2 AND ep.curso_exacto AND ep.anio=c.anio AND ep.division_norm=c.division_norm AND ep.modalidad_norm=c.modalidad_norm`, e.d.curso, e.d.equipo).Scan(&prioridad); err != nil || prioridad != 4 {
						t.Fatalf("se sobrescribió la prioridad de destino: %d %v", prioridad, err)
					}
				}
				if caso == "duplicado" {
					var n int
					_ = pool.QueryRow(ctx, `SELECT count(*) FROM equipo_preferencia WHERE equipo_id=$1 AND modalidad='Electrónica'`, e.d.equipo).Scan(&n)
					if n != 0 {
						t.Fatal("la marca se trasladó pese al curso duplicado")
					}
				}
				return
			}
			var aplicables, originales, generales int
			if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE ep.curso_exacto AND ep.anio=c.anio AND ep.division_norm=c.division_norm AND ep.modalidad_norm=c.modalidad_norm),count(*) FILTER(WHERE ep.curso_exacto AND ep.modalidad='Construcción' AND ep.anio=3 AND ep.division='2'),count(*) FILTER(WHERE NOT ep.curso_exacto) FROM equipo_preferencia ep CROSS JOIN curso c WHERE c.id=$1 AND ep.equipo_id=$2`, e.d.curso, e.d.equipo).Scan(&aplicables, &originales, &generales); err != nil {
				t.Fatal(err)
			}
			if caso == "aceptar" {
				if aplicables != 0 || originales != 1 {
					t.Fatal("no mantuvo el alcance original")
				}
			} else if aplicables != 1 {
				t.Fatalf("prioridad no conservada: %d", aplicables)
			}
			if caso == "compartida" || caso == "destino-igual" {
				if originales != 1 {
					t.Fatal("quitó la prioridad de otro ciclo")
				}
			}
			if caso == "generales" {
				if len(impacto.Cambios) != 3 || generales != 3 {
					t.Fatalf("reglas generales modificadas: %+v %d", impacto.Cambios, generales)
				}
			}
			var reservaDespues string
			if err := pool.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM reserva r WHERE id=$1`, e.reservaID).Scan(&reservaDespues); err != nil || reservaAntes != reservaDespues {
				t.Fatal("modificó la reserva existente")
			}
		})
	}
}
