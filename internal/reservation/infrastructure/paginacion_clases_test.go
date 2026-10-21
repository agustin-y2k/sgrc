//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
)

func TestPostgresRepo_ListarReservas_PorClasesCompletas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materia := crearMateriaDeTest(t, pool)
	docente := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	otro := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	fecha := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	ahora := fecha.AddDate(0, -1, 0)
	equipos := make([]string, 70)
	for i := range equipos {
		equipos[i] = crearEquipoDeCarroDeTest(t, pool)
	}
	crearClase := func(dia time.Time, cantidad int, cancelarTodas bool) *domain.ReservaGrupo {
		t.Helper()
		g := nuevoReservaGrupoDeTest(materia, dia, 9*time.Hour, 10*time.Hour)
		g.CreadoPor = &docente
		if err := repo.CrearReservaGrupo(ctx, g); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < cantidad; i++ {
			r, err := domain.NuevaReservaNormal(NuevoID(), g.ID, equipos[i], materia, "Docente de prueba", &docente, dia, 9*time.Hour, 10*time.Hour, ahora)
			if err != nil {
				t.Fatal(err)
			}
			if cancelarTodas || i == 0 {
				if err := r.Cancelar(&docente, "Cancelación de prueba", ahora); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.CrearReserva(ctx, r); err != nil {
				t.Fatal(err)
			}
		}
		return g
	}
	primera := crearClase(fecha, 70, false)
	segunda := crearClase(fecha.AddDate(0, 0, 1), 30, false)
	crearClase(fecha.AddDate(0, 0, 2), 2, true)
	for i := 0; i < 50; i++ {
		actor := &docente
		if i >= 25 {
			actor = &otro
		}
		r, err := domain.NuevaReservaBloqueo(NuevoID(), equipos[i], actor, fecha.AddDate(0, 0, 3), 9*time.Hour, 10*time.Hour, "Bloqueo de prueba", ahora)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.CrearReserva(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	filtro := application.FiltroReservas{CreadoPor: &docente, PorClases: true, Pagina: paginacion.Pagina{Numero: 1, Tamanio: 1}}
	filas, total, err := repo.ListarReservas(ctx, filtro)
	if err != nil {
		t.Fatal(err)
	}
	if len(filas) != 70 || total != 3 {
		t.Fatalf("clase grande recortada: filas=%d, total=%d", len(filas), total)
	}
	canceladas := 0
	for _, f := range filas {
		if f.Reserva.ReservaGrupoID == nil || *f.Reserva.ReservaGrupoID != primera.ID {
			t.Fatal("se mezclaron clases")
		}
		if f.Reserva.Estado == domain.ReservaCancelada {
			canceladas++
		}
	}
	if canceladas != 1 {
		t.Fatal("se perdió el equipo cancelado de una clase vigente")
	}
	filtro.Pagina.Numero = 2
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil {
		t.Fatal(err)
	}
	if len(filas) != 30 || total != 3 || *filas[0].Reserva.ReservaGrupoID != segunda.ID {
		t.Fatalf("segunda clase incorrecta: filas=%d, total=%d", len(filas), total)
	}
	filtro.Pagina.Numero = 3
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil {
		t.Fatal(err)
	}
	if len(filas) != 25 || total != 3 {
		t.Fatal("el bloqueo no se devolvió completo")
	}
	for _, f := range filas {
		if f.Reserva.Tipo != domain.TipoBloqueo || f.Reserva.CreadoPor == nil || *f.Reserva.CreadoPor != docente {
			t.Fatal("se incluyeron equipos de otro actor")
		}
	}
	filtro.Pagina.Numero = 9
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil || len(filas) != 0 || total != 3 {
		t.Fatalf("página fuera de rango: total=%d err=%v", total, err)
	}
	filtro.IncluirCanceladas = true
	filtro.Pagina.Numero = 3
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil || len(filas) != 2 || total != 4 {
		t.Fatalf("clase cancelada: total=%d filas=%d err=%v", total, len(filas), err)
	}
	// Un filtro por equipo identifica su clase, sin recortar sus compañeros.
	filtro.IncluirCanceladas = false
	filtro.EquipoID = &equipos[1]
	filtro.Desde = &fecha
	filtro.Hasta = &fecha
	filtro.Pagina.Numero = 1
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil || len(filas) != 70 || total != 1 {
		t.Fatalf("filtro por equipo recortó la clase: total=%d filas=%d err=%v", total, len(filas), err)
	}
	// Sin filtro de actor los bloqueos coincidentes conservan sus dos tarjetas.
	filtro = application.FiltroReservas{PorClases: true, Pagina: paginacion.Pagina{Numero: 1, Tamanio: 20}}
	filas, total, err = repo.ListarReservas(ctx, filtro)
	if err != nil || len(filas) != 150 || total != 4 {
		t.Fatalf("bloqueos de dos actores: total=%d filas=%d err=%v", total, len(filas), err)
	}
	t.Run("buscar por materia sin tildes mantiene clases completas", func(t *testing.T) {
		f := application.FiltroReservas{CreadoPor: &docente, PorClases: true, Busqueda: "MATEMATICAS", Pagina: paginacion.Pagina{Numero: 1, Tamanio: 1}}
		filas, total, err := repo.ListarReservas(ctx, f)
		if err != nil || total != 2 || len(filas) != 70 {
			t.Fatalf("búsqueda: total=%d filas=%d err=%v", total, len(filas), err)
		}
		for _, fila := range filas {
			if fila.Reserva.CreadoPor == nil || *fila.Reserva.CreadoPor != docente {
				t.Fatal("se perdió el filtro de identidad")
			}
		}
		f.Busqueda = "%"
		filas, total, err = repo.ListarReservas(ctx, f)
		if err != nil || total != 0 || len(filas) != 0 {
			t.Fatalf("búsqueda literal: total=%d filas=%d err=%v", total, len(filas), err)
		}
	})
	t.Run("estado cancelado no recorta clases mixtas", func(t *testing.T) {
		estado := domain.ReservaCancelada
		f := application.FiltroReservas{CreadoPor: &docente, PorClases: true, Estado: &estado, Pagina: paginacion.Pagina{Numero: 1, Tamanio: 1}}
		filas, total, err := repo.ListarReservas(ctx, f)
		if err != nil || total != 3 || len(filas) != 70 {
			t.Fatalf("estado: total=%d filas=%d err=%v", total, len(filas), err)
		}
		confirmadas := 0
		for _, fila := range filas {
			if fila.Reserva.Estado == domain.ReservaConfirmada {
				confirmadas++
			}
		}
		if confirmadas != 69 {
			t.Fatalf("se perdieron equipos confirmados de la clase: %d", confirmadas)
		}
	})
}
