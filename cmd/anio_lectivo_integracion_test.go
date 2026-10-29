//go:build integration

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestAnioLectivo_ReservaYSerieNoPasanDeDiciembre(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	var anteriores int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva`).Scan(&anteriores); err != nil {
		t.Fatal(err)
	}
	diciembre := time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC)
	enero := time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)
	_, _, _, err := e.reservas.CrearReserva(ctx, e.d.materia, e.d.docente, true, enero, 8*time.Hour, 9*time.Hour, []string{e.d.equipo}, nil)
	if !errors.Is(err, domain.ErrReservaFueraDelCiclo) {
		t.Fatalf("puntual: %v", err)
	}
	_, err = e.reservas.CrearReservaRecurrente(ctx, e.d.materia, e.d.docente, true, domain.Lunes, 8*time.Hour, 9*time.Hour, diciembre, enero, []string{e.d.equipo}, nil)
	if !errors.Is(err, domain.ErrReservaFueraDelCiclo) {
		t.Fatalf("serie: %v", err)
	}
	_, err = e.reservas.PrevisualizarReservaRecurrente(ctx, e.d.materia, e.d.docente, true, domain.Lunes, 8*time.Hour, 9*time.Hour, diciembre, enero, []string{e.d.equipo})
	if !errors.Is(err, domain.ErrReservaFueraDelCiclo) {
		t.Fatalf("previsualización: %v", err)
	}
	var actuales int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva`).Scan(&actuales); err != nil || anteriores != actuales {
		t.Fatal("quedaron reservas parciales")
	}
	ultimo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	_, _, _, err = e.reservas.CrearReserva(ctx, e.d.materia, e.d.docente, true, ultimo, 23*time.Hour, time.Hour, []string{e.d.equipo}, nil)
	if !errors.Is(err, domain.ErrReservaFueraDelCiclo) {
		t.Fatalf("nocturna: %v", err)
	}
	_, reservas, _, err := e.reservas.CrearReserva(ctx, e.d.materia, e.d.docente, true, ultimo, 23*time.Hour, 0, []string{e.d.equipo}, nil)
	if err != nil || len(reservas) != 1 {
		t.Fatalf("fin exclusivo en medianoche: %v", err)
	}
}
