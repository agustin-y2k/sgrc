package application

import (
	"context"
	"errors"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

func TestBajaRecuperable_ReintentaCancelacionYRemocionSinRepetirAvisos(t *testing.T) {
	for _, paso := range []string{"cancelacion", "remocion"} {
		t.Run(paso, func(t *testing.T) {
			repo := nuevoFakeRepo()
			repo.usuarios["d1"] = &domain.Usuario{ID: "d1", Rol: domain.RolDocente, Estado: domain.EstadoAprobada}
			gestor := nuevoFakeGestorMaterias()
			gestor.materias["d1"] = []string{"materia-a", "materia-b"}
			cancelador := nuevoFakeCanceladorReservas()
			if paso == "cancelacion" {
				cancelador.errPorMateria = map[string]error{"materia-b": errors.New("fallo")}
			} else {
				gestor.errRemover = errors.New("fallo")
			}
			svc := nuevoServicioConCascada(repo, gestor, cancelador)
			avisos := map[string]int{}
			svc.bus.Subscribe("docente.baja.materia-huerfana", func(e eventbus.Evento) { avisos[e.Payload.(map[string]any)["materiaId"].(string)]++ })
			if err := svc.DarDeBaja(context.Background(), "d1"); err == nil || repo.usuarios["d1"].Estado != domain.EstadoBaja {
				t.Fatalf("no quedó la baja con tareas pendientes: %v", err)
			}
			gestor.errRemover = nil
			cancelador.errPorMateria = nil
			if err := svc.DarDeBaja(context.Background(), "d1"); err != nil {
				t.Fatalf("no se pudo completar la baja: %v", err)
			}
			if err := svc.DarDeBaja(context.Background(), "d1"); err != nil {
				t.Fatalf("repetir baja completa: %v", err)
			}
			if len(gestor.removidoDe) != 1 || avisos["materia-a"] != 1 || avisos["materia-b"] != 1 {
				t.Fatalf("efectos repetidos: remociones=%v avisos=%v", gestor.removidoDe, avisos)
			}
		})
	}
}

func TestBajaRecuperable_AdminConAsignacionesTambienCompletaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.adminsAprobadosCount = 2
	repo.usuarios["a1"] = &domain.Usuario{ID: "a1", Rol: domain.RolAdmin, Estado: domain.EstadoAprobada}
	gestor := nuevoFakeGestorMaterias()
	gestor.materias["a1"] = []string{"materia-a"}
	cancelador := nuevoFakeCanceladorReservas()
	svc := nuevoServicioConCascada(repo, gestor, cancelador)
	if err := svc.DarDeBaja(context.Background(), "a1"); err != nil {
		t.Fatal(err)
	}
	if len(cancelador.llamadoParaMateria) != 1 || len(gestor.removidoDe) != 1 {
		t.Fatalf("el rol ADMIN dejó asignaciones sin procesar: %v %v", cancelador.llamadoParaMateria, gestor.removidoDe)
	}
}
