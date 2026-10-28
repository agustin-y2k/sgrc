//go:build integration

package main

import (
	"context"
	"errors"
	inventoryapp "github.com/agustin-y2k/sgrc/internal/inventory/application"
	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func TestRotulo_ConservaReservasPrestamosYPrioridades(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `UPDATE curso SET modalidad='Construcción' WHERE id=$1`, e.d.curso)
	ctx := context.Background()
	carro := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO carro(id,nombre) VALUES($1,'Carro de prueba')`, carro)
	ejecutarCierreSQL(t, pool, `UPDATE equipo SET carro_id=$2,identificador=4,nombre=NULL,numero_serie='SERIE-FISICA',es_computadora=true WHERE id=$1`, e.d.equipo, carro)
	prestamo := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO prestamo(id,equipo_id,reserva_id,entregado_a_nombre) VALUES($1,$2,$3,'Docente de prueba')`, prestamo, e.d.equipo, e.reservaID)
	preferencia := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,prioridad) VALUES($1,$2,'Matemática',1)`, preferencia, e.d.equipo)
	licencia := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO licencia_software(id,equipo_id,nombre,dias_duracion) VALUES($1,$2,'Software de prueba',365)`, licencia, e.d.equipo)
	if _, err := e.inventario.RenumerarEquipo(ctx, e.d.equipo, 4, 18, "Reemplazo del rótulo"); err != nil {
		t.Fatal(err)
	}
	var serie string
	var numero int
	if err := pool.QueryRow(ctx, `SELECT numero_serie,identificador FROM equipo WHERE id=$1`, e.d.equipo).Scan(&serie, &numero); err != nil {
		t.Fatal(err)
	}
	if serie != "SERIE-FISICA" || numero != 18 {
		t.Fatalf("serie=%s numero=%d", serie, numero)
	}
	for _, tc := range []struct{ tabla, id string }{{"reserva", e.reservaID}, {"prestamo", prestamo}, {"equipo_preferencia", preferencia}, {"licencia_software", licencia}} {
		var equipo string
		if err := pool.QueryRow(ctx, `SELECT equipo_id FROM `+tc.tabla+` WHERE id=$1`, tc.id).Scan(&equipo); err != nil {
			t.Fatal(err)
		}
		if equipo != e.d.equipo {
			t.Fatalf("%s cambió de PC", tc.tabla)
		}
	}
	desde := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	hasta := desde.AddDate(0, 0, 6)
	filas, _, err := e.reservas.ListarCalendario(ctx, reservationapp.FiltroReservas{Desde: &desde, Hasta: &hasta, Pagina: paginacion.PorDefecto()}, e.d.docente, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, fila := range filas {
		if fila.Reserva.Tipo == "NORMAL" && fila.CursoModalidad != "Construcción" {
			t.Fatalf("modalidad no resuelta: %+v", fila)
		}
		if fila.Reserva.EquipoID == e.d.equipo && fila.Etiqueta != "PC 18" {
			t.Fatalf("rótulo anterior en calendario: %+v", fila)
		}
	}
	prestamos, err := e.reservas.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(prestamos) != 1 || prestamos[0].Etiqueta != "PC 18" {
		t.Fatalf("rótulo anterior en préstamo: %+v", prestamos)
	}

	ocupado := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,carro_id,identificador,numero_serie,tipo,es_computadora) VALUES($1,$2,19,'SERIE-OTRA','PC',true)`, ocupado, carro)
	if _, err := e.inventario.RenumerarEquipo(ctx, e.d.equipo, 18, 19, "Número ocupado"); !errors.Is(err, inventoryapp.ErrIdentificadorDuplicado) {
		t.Fatalf("se aceptó duplicado: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT identificador FROM equipo WHERE id=$1`, e.d.equipo).Scan(&numero); err != nil {
		t.Fatal(err)
	}
	if numero != 18 {
		t.Fatal("el conflicto modificó el equipo")
	}

	var grupo sync.WaitGroup
	resultados := make(chan error, 2)
	for _, n := range []int{20, 21} {
		grupo.Add(1)
		go func(numero int) {
			defer grupo.Done()
			_, err := e.inventario.RenumerarEquipo(ctx, e.d.equipo, 18, numero, "Cambio concurrente")
			resultados <- err
		}(n)
	}
	grupo.Wait()
	close(resultados)
	exitos, conflictos := 0, 0
	for err := range resultados {
		if err == nil {
			exitos++
		} else if errors.Is(err, inventoryapp.ErrRotuloDesactualizado) {
			conflictos++
		} else {
			t.Fatal(err)
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("exitos=%d conflictos=%d", exitos, conflictos)
	}
}
