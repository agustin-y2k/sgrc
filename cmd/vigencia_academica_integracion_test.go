//go:build integration

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	academicapp "github.com/agustin-y2k/sgrc/internal/academic/application"
	academicdomain "github.com/agustin-y2k/sgrc/internal/academic/domain"
	academicinfra "github.com/agustin-y2k/sgrc/internal/academic/infrastructure"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
)

type vigenciaEnPausa struct {
	academicapp.Repo
	pausa *pausaInstitucional
}

func (r vigenciaEnPausa) VerificarVigencia(ctx context.Context, tipo, id string) error {
	if err := r.Repo.VerificarVigencia(ctx, tipo, id); err != nil {
		return err
	}
	return r.pausa.detener(ctx)
}

type cierreEnPausa struct {
	academicapp.Repo
	pausa *pausaInstitucional
}

func (r cierreEnPausa) GuardarCierre(ctx context.Context, id string, cierre *academicapp.CierreCompletado) error {
	if err := r.Repo.GuardarCierre(ctx, id, cierre); err != nil {
		return err
	}
	return r.pausa.detener(ctx)
}
func TestVigenciaAcademica_ArchivoProtegeTodosLosCaminos(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	d := prepararCierre(t, pool)
	svc, _ := serviciosCierre(pool, "")
	vinculo := vinculoReal(t, pool, d.docente, d.materia)
	if _, err := svc.ArchivarYClonar(ctx, d.ciclo, nil); err != nil {
		t.Fatal(err)
	}
	casos := map[string]func() error{
		"curso-alta":      func() error { _, err := svc.CrearCurso(ctx, d.ciclo, 2, "B", ""); return err },
		"curso-edicion":   func() error { return svc.EditarCurso(ctx, d.curso, 2, "B", "") },
		"curso-borrado":   func() error { return svc.EliminarCurso(ctx, d.curso) },
		"materia-alta":    func() error { _, err := svc.CrearMateria(ctx, d.curso, "Nueva"); return err },
		"materia-edicion": func() error { _, err := svc.EditarMateria(ctx, d.materia, "Nuevo nombre"); return err },
		"materia-borrado": func() error { return svc.EliminarMateria(ctx, d.materia) },
		"espacio-alta":    func() error { _, err := svc.CrearEspacio(ctx, d.ciclo, "Nuevo espacio"); return err },
		"espacio-edicion": func() error { return svc.EditarEspacio(ctx, d.espacio, "Sala nueva") },
		"espacio-borrado": func() error { return svc.EliminarEspacio(ctx, d.espacio) },
		"materia-espacio": func() error { _, err := svc.EditarMateria(ctx, d.materiaEspacio, "Nuevo"); return err },
		"asignacion": func() error {
			_, err := svc.AsignarDocente(ctx, d.materia, d.docente, academicdomain.RolTitular)
			return err
		},
		"rol":      func() error { _, err := svc.CambiarRolDocente(ctx, vinculo, academicdomain.RolSuplente); return err },
		"remocion": func() error { _, err := svc.RemoverDocenteMateria(ctx, vinculo); return err },
		"importacion": func() error {
			_, err := svc.ImportarEstructura(ctx, d.ciclo, []academicapp.CursoConMaterias{{Anio: 2, Division: "B", Materias: []string{"Nueva"}}})
			return err
		},
		"copia": func() error { _, err := svc.CopiarMaterias(ctx, d.curso, []string{d.curso}); return err },
	}
	for nombre, fn := range casos {
		t.Run(nombre, func(t *testing.T) {
			if err := fn(); !errors.Is(err, academicapp.ErrCicloArchivado) {
				t.Fatalf("escritura del histórico permitida: %v", err)
			}
		})
	}
}
func TestVigenciaAcademica_CierreYEscrituraConcurrentes(t *testing.T) {
	pool := baseVacia(t)
	for _, primero := range []string{"escritura", "cierre"} {
		t.Run(primero, func(t *testing.T) {
			ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelar()
			d := prepararCierre(t, pool)
			svc, _ := serviciosCierre(pool, "")
			pausa := nuevaPausaInstitucional(t)
			primeroResultado, segundoResultado := make(chan error, 1), make(chan error, 1)
			repo := academicapp.Repo(academicinfra.NewPostgresRepo(pool))
			if primero == "escritura" {
				repo = vigenciaEnPausa{repo, pausa}
			} else {
				repo = cierreEnPausa{repo, pausa}
			}
			_, report := serviciosCierre(pool, "")
			reservas := reservationapp.NewService(reservationinfra.NewPostgresRepo(pool), nil, nil, nil, nil, uuid.NewString, time.Now, nil)
			svcPausado := academicapp.NewService(repo, nil, nil, &academicArchivadorHistoricoAdapter{reportingSvc: report, reservationSvc: reservas}, nil, nil, nil, uuid.NewString, time.Now, nil)
			escribir := func(s *academicapp.Service, ch chan<- error) {
				_, err := s.CrearMateria(ctx, d.curso, "Nueva durante cierre")
				ch <- err
			}
			cerrar := func(s *academicapp.Service, ch chan<- error) {
				_, err := s.ArchivarYClonar(ctx, d.ciclo, nil)
				ch <- err
			}
			if primero == "escritura" {
				go escribir(svcPausado, primeroResultado)
			} else {
				go cerrar(svcPausado, primeroResultado)
			}
			pausa.esperar(t, ctx)
			if primero == "escritura" {
				go cerrar(svc, segundoResultado)
			} else {
				go escribir(svc, segundoResultado)
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			if err := recibirErrorAsignaciones(t, ctx, primeroResultado); err != nil {
				t.Fatal(err)
			}
			err := recibirErrorAsignaciones(t, ctx, segundoResultado)
			if primero == "cierre" {
				if !errors.Is(err, academicapp.ErrCicloArchivado) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var vivas int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM materia m JOIN contenedor_de_materia c ON c.materia_id=m.id WHERE c.ciclo_lectivo_id=$1 AND NOT m.archivado`, d.ciclo).Scan(&vivas); err != nil {
				t.Fatal(err)
			}
			if vivas != 0 {
				t.Fatalf("quedaron %d materias sin archivar", vivas)
			}
		})
	}
}
