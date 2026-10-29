//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationdomain "github.com/agustin-y2k/sgrc/internal/reservation/domain"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func materiaDePrioridadCustodia(t *testing.T, pool *pgxpool.Pool, curso, nombre string) (materia, docente string) {
	t.Helper()
	materia, docente = uuid.NewString(), uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO materia(id,curso_id,nombre) VALUES($1,$2,$3)`, materia, curso, nombre)
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado)
		VALUES($1,'Docente','De prueba',$2,'hash','DOCENTE','APROBADA')`, docente, docente+"@escuela.test")
	ejecutarCierreSQL(t, pool, `INSERT INTO docente_materia(usuario_id,materia_id,rol) VALUES($1,$2,'TITULAR')`, docente, materia)
	return materia, docente
}

func marcaDePrioridadCustodia(t *testing.T, pool *pgxpool.Pool, equipo, materia string, prioridad int) {
	t.Helper()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo_preferencia(id,equipo_id,materia_nombre,prioridad) VALUES($1,$2,$3,$4)`, uuid.NewString(), equipo, materia, prioridad)
}

func equipoDePrioridadCustodia(t *testing.T, pool *pgxpool.Pool, nombre string) string {
	t.Helper()
	id := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,$2,'Computadora')`, id, nombre)
	return id
}

func entregarDePrioridadCustodia(t *testing.T, e escenarioInstitucional, devuelta bool) string {
	t.Helper()
	ret, err := e.entregar(context.Background(), e.reservas, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret.Entregadas) != 1 || len(ret.NoEntregadas) != 0 {
		t.Fatalf("entrega inicial: %+v", ret)
	}
	if devuelta {
		if _, err := e.reservas.RecibirEquipos(context.Background(), []string{ret.Entregadas[0].ID}, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	return ret.Entregadas[0].ID
}

func comprobarProtegidaPorEntrega(t *testing.T, pool *pgxpool.Pool, e escenarioInstitucional, prestamo string, devuelta bool) {
	t.Helper()
	var estado, equipo, equipoPrestado, reservaPrestada string
	var fueDevuelto bool
	err := pool.QueryRow(context.Background(), `SELECT r.estado,r.equipo_id,p.equipo_id,p.reserva_id,p.devuelto_en IS NOT NULL
		FROM reserva r JOIN prestamo p ON p.id=$2 WHERE r.id=$1`, e.reservaID, prestamo).
		Scan(&estado, &equipo, &equipoPrestado, &reservaPrestada, &fueDevuelto)
	if err != nil {
		t.Fatal(err)
	}
	if estado != "CONFIRMADA" || equipo != e.d.equipo || equipoPrestado != equipo || reservaPrestada != e.reservaID || fueDevuelto != devuelta {
		t.Fatalf("se alteró la reserva o su préstamo: estado=%s equipo=%s préstamo=%s reserva=%s devuelto=%v", estado, equipo, equipoPrestado, reservaPrestada, fueDevuelto)
	}
}

func TestPrioridadCustodia_EntregaImpideTomaYPrevisualizacion(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	fecha := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	for _, devuelta := range []bool{false, true} {
		for _, sustituto := range []bool{false, true} {
			t.Run(fmt.Sprintf("devuelta-%v/sustituto-%v", devuelta, sustituto), func(t *testing.T) {
				e := prepararInstitucion(t, pool)
				if sustituto {
					equipoDePrioridadCustodia(t, pool, "PC de reemplazo")
				}
				materia, docente := materiaDePrioridadCustodia(t, pool, e.d.curso, "Dibujo Técnico")
				marcaDePrioridadCustodia(t, pool, e.d.equipo, "Dibujo Técnico", 1)
				ocupados, err := e.reservas.ListarEquiposOcupadosEn(ctx, fecha, 8*time.Hour, 9*time.Hour, docente, materia)
				if err != nil || len(ocupados) != 1 || !ocupados[0].PuedeTomarse {
					t.Fatalf("la reserva sin entrega debe participar de la prioridad: %+v, %v", ocupados, err)
				}
				prestamo := entregarDePrioridadCustodia(t, e, devuelta)
				antes := huellasBase(t, ctx, pool)
				ocupados, err = e.reservas.ListarEquiposOcupadosEn(ctx, fecha, 8*time.Hour, 9*time.Hour, docente, materia)
				if err != nil || len(ocupados) != 1 || ocupados[0].PuedeTomarse {
					t.Fatalf("se ofreció tomar una reserva que ya tuvo entrega: %+v, %v", ocupados, err)
				}
				previa, err := e.reservas.PrevisualizarReservaRecurrente(ctx, materia, docente, false, reservationdomain.Lunes,
					8*time.Hour, 9*time.Hour, fecha, fecha.AddDate(0, 0, 7), []string{e.d.equipo})
				if err != nil {
					t.Fatal(err)
				}
				if previa.PuedeCrear || len(previa.EquiposATomar) != 0 || previa.Fechas[0].Equipos[0].Estado != "NO_DISPONIBLE" || previa.Fechas[1].Equipos[0].Estado != "LIBRE" {
					t.Fatalf("previsualización incoherente con la entrega: %+v", previa)
				}
				_, _, impacto, err := e.reservas.CrearReserva(ctx, materia, docente, false, fecha, 8*time.Hour, 9*time.Hour,
					[]string{e.d.equipo}, []string{e.d.equipo})
				if !errors.Is(err, reservationapp.ErrSolapamiento) || impacto != (reservationapp.ImpactoDelRobo{}) {
					t.Fatalf("toma puntual: impacto=%+v error=%v", impacto, err)
				}
				_, err = e.reservas.CrearReservaRecurrente(ctx, materia, docente, false, reservationdomain.Lunes, 8*time.Hour, 9*time.Hour,
					fecha, fecha.AddDate(0, 0, 7), []string{e.d.equipo}, []string{e.d.equipo})
				if !errors.Is(err, reservationapp.ErrSolapamiento) {
					t.Fatalf("toma recurrente: %v", err)
				}
				if despues := huellasBase(t, ctx, pool); !reflect.DeepEqual(antes, despues) {
					t.Fatal("el rechazo dejó escrituras en reservas, préstamos, auditoría u otras tablas")
				}
				comprobarProtegidaPorEntrega(t, pool, e, prestamo, devuelta)
			})
		}
	}
}

func TestPrioridadCustodia_ProtegeSoloLaOcurrenciaEntregada(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ctx := context.Background()
	materia, docente := materiaDePrioridadCustodia(t, pool, e.d.curso, "Dibujo Técnico")
	marcaDePrioridadCustodia(t, pool, e.d.equipo, "Dibujo Técnico", 1)
	fecha := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	for _, dias := range []int{7, 14} {
		grupo := uuid.NewString()
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva_grupo(id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin,regla_recurrencia_id)
			VALUES($1,$2,$3,'Ada Docente',$4,'08:00','09:00',$5)`, grupo, e.d.materia, e.d.docente, fecha.AddDate(0, 0, dias), e.d.regla)
		ejecutarCierreSQL(t, pool, `INSERT INTO reserva(reserva_grupo_id,equipo_id,materia_id,creado_por,nombre_docente_snapshot,fecha,hora_inicio,hora_fin)
			VALUES($1,$2,$3,$4,'Ada Docente',$5,'08:00','09:00')`, grupo, e.d.equipo, e.d.materia, e.d.docente, fecha.AddDate(0, 0, dias))
	}
	prestamo := entregarDePrioridadCustodia(t, e, false)
	antes := huellasBase(t, ctx, pool)
	_, err := e.reservas.CrearReservaRecurrente(ctx, materia, docente, false, reservationdomain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha, fecha.AddDate(0, 0, 14), []string{e.d.equipo}, []string{e.d.equipo})
	if !errors.Is(err, reservationapp.ErrSolapamiento) || !reflect.DeepEqual(antes, huellasBase(t, ctx, pool)) {
		t.Fatalf("la serie con una ocurrencia protegida debe rechazarse entera: %v", err)
	}
	res, err := e.reservas.CrearReservaRecurrente(ctx, materia, docente, false, reservationdomain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha.AddDate(0, 0, 7), fecha.AddDate(0, 0, 14), []string{e.d.equipo}, []string{e.d.equipo})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Grupos) != 2 || res.Impacto.ReservasCanceladas != 2 || res.Impacto.ReservasReubicadas != 0 {
		t.Fatalf("la entrega previa no debe proteger fechas sin entrega: %+v", res)
	}
	var canceladas int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reserva WHERE materia_id=$1 AND fecha>$2 AND estado='CANCELADA'`, e.d.materia, fecha).Scan(&canceladas); err != nil {
		t.Fatal(err)
	}
	if canceladas != 2 {
		t.Fatalf("ocurrencias futuras canceladas: %d", canceladas)
	}
	comprobarProtegidaPorEntrega(t, pool, e, prestamo, false)
}

func TestPrioridadCustodia_CascadaConservaEntregasSecundarias(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	fecha := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	for _, devuelta := range []bool{false, true} {
		for _, repuesto := range []string{"ninguno", "para-protegida", "para-desplazada"} {
			t.Run(fmt.Sprintf("devuelta-%v/repuesto-%s", devuelta, repuesto), func(t *testing.T) {
				e := prepararInstitucion(t, pool)
				dibujo, docenteDibujo := materiaDePrioridadCustodia(t, pool, e.d.curso, "Dibujo Técnico")
				tic, docenteTIC := materiaDePrioridadCustodia(t, pool, e.d.curso, "TIC")
				moderna := equipoDePrioridadCustodia(t, pool, "PC moderna")
				marcaDePrioridadCustodia(t, pool, moderna, "Dibujo Técnico", 1)
				marcaDePrioridadCustodia(t, pool, moderna, "TIC", 2)
				marcaDePrioridadCustodia(t, pool, e.d.equipo, "TIC", 1)
				marcaDePrioridadCustodia(t, pool, e.d.equipo, "Matemática", 3)
				var sustituto string
				if repuesto != "ninguno" {
					sustituto = equipoDePrioridadCustodia(t, pool, "PC sustituta")
					nombre := "Matemática"
					if repuesto == "para-desplazada" {
						nombre = "TIC"
					}
					marcaDePrioridadCustodia(t, pool, sustituto, nombre, 1)
				}
				_, rs, _, err := e.reservas.CrearReserva(ctx, tic, docenteTIC, false, fecha, 8*time.Hour, 9*time.Hour, []string{moderna}, nil)
				if err != nil {
					t.Fatal(err)
				}
				prestamo := entregarDePrioridadCustodia(t, e, devuelta)
				_, ganadoras, impacto, err := e.reservas.CrearReserva(ctx, dibujo, docenteDibujo, false, fecha, 8*time.Hour, 9*time.Hour,
					[]string{moderna}, []string{moderna})
				if err != nil {
					t.Fatal(err)
				}
				if len(ganadoras) != 1 || ganadoras[0].EquipoID != moderna {
					t.Fatalf("reserva ganadora incorrecta: %+v", ganadoras)
				}
				var estado, equipo string
				if err := pool.QueryRow(ctx, `SELECT estado,equipo_id FROM reserva WHERE id=$1`, rs[0].ID).Scan(&estado, &equipo); err != nil {
					t.Fatal(err)
				}
				if repuesto == "para-desplazada" {
					if estado != "CONFIRMADA" || equipo != sustituto || impacto.ReservasReubicadas != 1 || impacto.ReservasCanceladas != 0 {
						t.Fatalf("TIC debe usar el repuesto libre: %s %s %+v", estado, equipo, impacto)
					}
				} else if estado != "CANCELADA" || equipo != moderna || impacto.ReservasCanceladas != 1 || impacto.ReservasReubicadas != 0 {
					t.Fatalf("la cascada debe detenerse ante la reserva entregada: %s %s %+v", estado, equipo, impacto)
				}
				comprobarProtegidaPorEntrega(t, pool, e, prestamo, devuelta)
			})
		}
	}
}

func TestPrioridadCustodia_EntregaYTomaConcurrentes(t *testing.T) {
	pool := baseVacia(t)
	for _, entregaPrimero := range []bool{true, false} {
		t.Run(fmt.Sprintf("entrega-primero-%v", entregaPrimero), func(t *testing.T) {
			e := prepararInstitucion(t, pool)
			materia, docente := materiaDePrioridadCustodia(t, pool, e.d.curso, "Dibujo Técnico")
			marcaDePrioridadCustodia(t, pool, e.d.equipo, "Dibujo Técnico", 1)
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			pausa := nuevaPausaInstitucional(t)
			entregador, tomador := e.reservas, e.reservas
			validador := equipoValidadoEnPausa{reservationinfra.NewValidadorEquipoPostgres(pool), pausa}
			if entregaPrimero {
				entregador = e.reservasCon(pool, validador)
			} else {
				tomador = e.reservasCon(pool, validador)
			}
			type resultadoEntrega struct {
				resultado *reservationapp.ResultadoEntrega
				err       error
			}
			entregas := make(chan resultadoEntrega, 1)
			tomas := make(chan error, 1)
			entregar := func() { r, err := e.entregar(ctx, entregador, false); entregas <- resultadoEntrega{r, err} }
			tomar := func() {
				_, _, _, err := tomador.CrearReserva(ctx, materia, docente, false, time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
					8*time.Hour, 9*time.Hour, []string{e.d.equipo}, []string{e.d.equipo})
				tomas <- err
			}
			if entregaPrimero {
				go entregar()
				pausa.esperar(t, ctx)
				go tomar()
			} else {
				go tomar()
				pausa.esperar(t, ctx)
				go entregar()
			}
			esperarOperacionInstitucional(t, ctx, pool)
			close(pausa.continuar)
			toma := <-tomas
			entrega := <-entregas
			if entrega.err != nil {
				t.Fatal(entrega.err)
			}
			if entregaPrimero {
				if !errors.Is(toma, reservationapp.ErrSolapamiento) || len(entrega.resultado.Entregadas) != 1 {
					t.Fatalf("debe conservarse la entrega: toma=%v entrega=%+v", toma, entrega.resultado)
				}
				comprobarProtegidaPorEntrega(t, pool, e, entrega.resultado.Entregadas[0].ID, false)
			} else {
				if toma != nil || len(entrega.resultado.Entregadas) != 0 || len(entrega.resultado.NoEntregadas) != 1 || entrega.resultado.NoEntregadas[0].Razon != reservationapp.NoEntregadaReservaCancelada {
					t.Fatalf("la entrega debe releer la cancelación confirmada: toma=%v entrega=%+v", toma, entrega.resultado)
				}
			}
		})
	}
}
