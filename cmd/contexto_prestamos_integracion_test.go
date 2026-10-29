//go:build integration

package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	reservationapp "github.com/agustin-y2k/sgrc/internal/reservation/application"
	reservationinfra "github.com/agustin-y2k/sgrc/internal/reservation/infrastructure"
	"github.com/google/uuid"
)

func TestContextoPrestamos_ConservaCadaEntregaTrasRenombresYCierre(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `UPDATE curso SET anio=3,division='2',modalidad='Construcción' WHERE id=$1`, e.d.curso)
	ahora := time.Date(2026, 5, 4, 8, 30, 0, 0, time.UTC)
	repo := reservationinfra.NewPostgresRepo(pool)
	svc := reservationapp.NewService(repo, nil, reservationinfra.NewValidadorEquipoPostgres(pool), nil, nil, uuid.NewString, func() time.Time { return ahora }, e.bus)
	entregar := func() string {
		t.Helper()
		r, err := svc.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo}})
		if err != nil || len(r.Entregadas) != 1 {
			t.Fatalf("entrega: %+v %v", r, err)
		}
		c := r.Entregadas[0].ContextoAcademico
		if c == nil || c.Reconstruido || c.ReservaIDOriginal != e.reservaID || c.MateriaID != e.d.materia || c.CicloID != e.d.ciclo || c.CicloAnio != 2026 || c.ContenedorTipo != "CURSO" {
			t.Fatalf("contexto incompleto: %+v", c)
		}
		return r.Entregadas[0].ID
	}
	primero := entregar()
	p1, err := repo.BuscarPrestamoPorID(ctx, primero)
	if err != nil {
		t.Fatal(err)
	}
	if c := p1.ContextoAcademico; c.MateriaNombre != "Matemática" || c.ContenedorNombre != "3°2" || c.Modalidad != "Construcción" || c.CursoAnio != 3 || c.CursoDivision != "2" {
		t.Fatalf("datos de la primera entrega: %+v", c)
	}
	ahora = ahora.Add(time.Minute)
	if _, err := svc.RecibirEquipos(ctx, []string{primero}, "", "devuelta"); err != nil {
		t.Fatal(err)
	}
	academico, _ := serviciosCierre(pool, "")
	if _, err := academico.EditarMateria(ctx, e.d.materia, "Álgebra"); err != nil {
		t.Fatal(err)
	}
	if err := academico.EditarCurso(ctx, e.d.curso, 4, "3", "Informática"); err != nil {
		t.Fatal(err)
	}
	ahora = ahora.Add(time.Minute)
	segundo := entregar()
	p2, err := repo.BuscarPrestamoPorID(ctx, segundo)
	if err != nil {
		t.Fatal(err)
	}
	if c := p2.ContextoAcademico; c.MateriaNombre != "Álgebra" || c.ContenedorNombre != "4°3" || c.Modalidad != "Informática" {
		t.Fatalf("datos de la segunda entrega: %+v", c)
	}
	destino := 2027
	if _, err := academico.ArchivarYClonar(ctx, e.d.ciclo, &destino); err != nil {
		t.Fatal(err)
	}
	for _, original := range []struct {
		id       string
		contexto any
		destino  string
	}{{primero, p1.ContextoAcademico, p1.Destino}, {segundo, p2.ContextoAcademico, p2.Destino}} {
		p, err := repo.BuscarPrestamoPorID(ctx, original.id)
		if err != nil || p.ReservaID != nil || !reflect.DeepEqual(p.ContextoAcademico, original.contexto) || p.Destino != original.destino {
			t.Fatalf("el cierre perdió contexto: %+v %v", p, err)
		}
	}
	abiertos, err := svc.ListarPrestamosAbiertos(ctx)
	if err != nil || len(abiertos) != 1 || abiertos[0].Prestamo.ContextoAcademico.MateriaNombre != "Álgebra" || *abiertos[0].MateriaNombre != "Álgebra" {
		t.Fatalf("listado sin contexto: %+v %v", abiertos, err)
	}
	historial, err := svc.HistorialDeEquipo(ctx, e.d.equipo)
	if err != nil || len(historial) != 2 {
		t.Fatalf("historial: %+v %v", historial, err)
	}
	for _, h := range historial {
		if h.Prestamo.ContextoAcademico == nil || h.MateriaNombre == nil {
			t.Fatal("el historial perdió la materia")
		}
	}
	if _, err := repo.PrestamosAVigilar(ctx); err != nil {
		t.Fatalf("se rompió la vigilancia: %v", err)
	}
	if _, err := svc.RecibirEquipos(ctx, []string{segundo}, "", "recibida después del cierre"); err != nil {
		t.Fatal(err)
	}
	devuelto, err := repo.BuscarPrestamoPorID(ctx, segundo)
	if err != nil || devuelto.EstaAbierto() || !reflect.DeepEqual(devuelto.ContextoAcademico, p2.ContextoAcademico) {
		t.Fatal("devolver después del cierre alteró el contexto")
	}
}

func TestContextoPrestamos_EspaciosYEntregasSinReserva(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	ejecutarCierreSQL(t, pool, `UPDATE reserva SET materia_id=$2 WHERE id=$1`, e.reservaID, e.d.materiaEspacio)
	ejecutarCierreSQL(t, pool, `UPDATE reserva_grupo SET materia_id=$2 WHERE id=$1`, e.d.grupo, e.d.materiaEspacio)
	entrega, err := e.reservas.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo}})
	if err != nil || len(entrega.Entregadas) != 1 {
		t.Fatalf("espacio: %+v %v", entrega, err)
	}
	p := entrega.Entregadas[0]
	if c := p.ContextoAcademico; c == nil || c.ContenedorTipo != "ESPACIO" || c.ContenedorNombre != "Biblioteca" || c.CursoAnio != 0 || c.CursoDivision != "" || c.Modalidad != "" {
		t.Fatalf("se inventó un curso para el espacio: %+v", c)
	}
	if _, err := e.reservas.RecibirEquipos(ctx, []string{p.ID}, "", ""); err != nil {
		t.Fatal(err)
	}
	suelta, err := e.reservas.EntregarSuelta(ctx, reservationapp.EntregaSueltaParams{EquipoIDs: []string{e.d.equipo}, Nombre: "Docente de prueba", Destino: "Secretaría"})
	if err != nil || len(suelta.Entregadas) != 1 || suelta.Entregadas[0].ContextoAcademico != nil || suelta.Entregadas[0].Destino != "Secretaría" {
		t.Fatalf("se inventó contexto para la entrega espontánea: %+v %v", suelta, err)
	}
}

func TestContextoPrestamos_UnFalloRevierteContextoYTodoElLote(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	e := prepararInstitucion(t, pool)
	otraReserva, otraPc := uuid.NewString(), uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO equipo(id,nombre,tipo) VALUES($1,'Otra computadora','Computadora')`, otraPc)
	ejecutarCierreSQL(t, pool, `INSERT INTO reserva(id,reserva_grupo_id,equipo_id,materia_id,nombre_docente_snapshot,creado_por,fecha,hora_inicio,hora_fin,estado)
 SELECT $2,reserva_grupo_id,$3,materia_id,nombre_docente_snapshot,creado_por,fecha,hora_inicio,hora_fin,estado FROM reserva WHERE id=$1`, e.reservaID, otraReserva, otraPc)
	quitar := falloBajaSQL(t, pool, "prestamo_contexto_academico", "INSERT", fmt.Sprintf("WHEN (NEW.reserva_id_original='%s'::uuid)", otraReserva))
	defer quitar()
	r, err := e.reservas.EntregarPorReserva(ctx, reservationapp.EntregaPorReservaParams{ReservaIDs: []string{e.reservaID, otraReserva}, EquiposEsperados: map[string]string{e.reservaID: e.d.equipo, otraReserva: otraPc}})
	if err == nil || r != nil {
		t.Fatalf("confirmó una entrega sin contexto: %+v %v", r, err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM prestamo`)
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM prestamo_contexto_academico`)
}
