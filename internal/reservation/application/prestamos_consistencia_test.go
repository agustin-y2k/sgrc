package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func loteReservado(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	for _, id := range []string{"a", "b", "c"} {
		reservaDeTest(t, repo, id, "pc-"+id)
	}
	repo.grupos["grupo1"] = &domain.ReservaGrupo{ID: "grupo1", Estado: domain.GrupoConfirmada}
	return nuevoServicioDeTest(repo)
}

func TestEntregaParcial_SegundaTandaConservaTodasLasEntregasPrevias(t *testing.T) {
	for _, devuelta := range []bool{false, true} {
		t.Run(map[bool]string{false: "abierta", true: "devuelta"}[devuelta], func(t *testing.T) {
			repo := nuevoFakeRepo()
			svc := loteReservado(t, repo)
			primera, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"a"})})
			if err != nil {
				t.Fatal(err)
			}
			if devuelta {
				if _, err := svc.RecibirEquipos(context.Background(), []string{primera.Entregadas[0].ID}, "", ""); err != nil {
					t.Fatal(err)
				}
			}
			segunda, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"b"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"b"}), LiberarNoEntregadas: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(segunda.Liberadas, []string{"c"}) {
				t.Fatalf("liberó equipos ya retirados: %v", segunda.Liberadas)
			}
			for _, id := range []string{"a", "b"} {
				if repo.reservas[id].Estado != domain.ReservaConfirmada {
					t.Fatalf("%s quedó %s pese a su entrega", id, repo.reservas[id].Estado)
				}
			}
		})
	}
}

func TestEntregaParcial_SinEntregasEfectivasNoLiberaElGrupo(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	repo.reservas["a"].Estado = domain.ReservaCancelada
	res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"a"}), LiberarNoEntregadas: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Liberadas) != 0 || repo.reservas["b"].Estado != domain.ReservaConfirmada {
		t.Fatal("un rechazo liberó reservas de una clase sin entregas")
	}
}

func TestEntregaParcial_UsaLasEntregasEfectivasDelLote(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	// Se pidió b, pero quedó en el laboratorio: debe poder liberarse.
	svc.validadorEquipo = &fakeValidadorEquipo{fueraDelInventario: map[string]bool{"pc-b": true}}
	res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "b"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"a", "b"}), LiberarNoEntregadas: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entregadas) != 1 || len(res.NoEntregadas) != 1 || len(res.Liberadas) != 2 || repo.reservas["b"].Estado != domain.ReservaNoRetirada {
		t.Fatalf("se confundió pedido con entrega efectiva: %+v", res)
	}
}

func TestEntregaLote_IDInexistenteNoConfirmaPrestamos(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "no-existe"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"a", "no-existe"}), LiberarNoEntregadas: true})
	if !errors.Is(err, ErrReservaNoEncontrada) || res != nil {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	if len(repo.prestamos) != 0 {
		t.Fatal("se confirmó una entrega sin informar su resultado")
	}
	for _, r := range repo.reservas {
		if r.Estado != domain.ReservaConfirmada {
			t.Fatal("se modificaron reservas de un lote inválido")
		}
	}
}

func TestDevolucionLote_IDInexistenteNoConfirmaLaPrimera(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{EquipoIDs: []string{"pc1"}, Nombre: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	id := entrega.Entregadas[0].ID
	res, err := svc.RecibirEquipos(context.Background(), []string{id, "no-existe"}, "", "")
	if !errors.Is(err, ErrPrestamoNoEncontrado) || res != nil {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	if !repo.prestamos[id].EstaAbierto() {
		t.Fatal("se confirmó una devolución sin informar su resultado")
	}
}

var errFalloDelLote = errors.New("fallo después de una escritura del lote")

type repoConFalloDeLote struct {
	*fakeRepo
	operacion  string
	escrituras int
}

func (r *repoConFalloDeLote) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return r.fakeRepo.EnTransaccion(ctx, func(Repo) error { return fn(r) })
}
func (r *repoConFalloDeLote) CrearPrestamo(ctx context.Context, p *domain.Prestamo) error {
	if err := r.fakeRepo.CrearPrestamo(ctx, p); err != nil {
		return err
	}
	if r.operacion == "entrega" {
		r.escrituras++
		if r.escrituras == 2 {
			return errFalloDelLote
		}
	}
	return nil
}
func (r *repoConFalloDeLote) GuardarPrestamo(ctx context.Context, p *domain.Prestamo) error {
	if err := r.fakeRepo.GuardarPrestamo(ctx, p); err != nil {
		return err
	}
	if r.operacion == "devolucion" {
		r.escrituras++
		if r.escrituras == 2 {
			return errFalloDelLote
		}
	}
	return nil
}
func (r *repoConFalloDeLote) GuardarReserva(ctx context.Context, p *domain.Reserva) error {
	if err := r.fakeRepo.GuardarReserva(ctx, p); err != nil {
		return err
	}
	if r.operacion == "liberacion" {
		return errFalloDelLote
	}
	return nil
}

func TestEntregaLote_ErrorDeEscrituraRevierteTodasLasEntregas(t *testing.T) {
	for _, conReserva := range []bool{false, true} {
		t.Run(map[bool]string{false: "suelta", true: "reserva"}[conReserva], func(t *testing.T) {
			base := nuevoFakeRepo()
			svc := loteReservado(t, base)
			repo := &repoConFalloDeLote{fakeRepo: base, operacion: "entrega"}
			svc.repo = repo
			var res *ResultadoEntrega
			var err error
			if conReserva {
				res, err = svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "b"}, EquiposEsperados: equiposEsperadosPrueba(base, []string{"a", "b"})})
			} else {
				res, err = svc.EntregarSuelta(context.Background(), EntregaSueltaParams{EquipoIDs: []string{"pc1", "pc2"}, Nombre: "Ada"})
			}
			if !errors.Is(err, errFalloDelLote) || res != nil {
				t.Fatalf("resultado inesperado: %+v %v", res, err)
			}
			if len(base.prestamos) != 0 {
				t.Fatal("quedó una entrega oculta tras el fallo")
			}
		})
	}
}

func TestEntregaParcial_ErrorDeLiberacionReviertePrestamosYReservas(t *testing.T) {
	base := nuevoFakeRepo()
	svc := loteReservado(t, base)
	svc.repo = &repoConFalloDeLote{fakeRepo: base, operacion: "liberacion"}
	res, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a"}, EquiposEsperados: equiposEsperadosPrueba(base, []string{"a"}), LiberarNoEntregadas: true})
	if !errors.Is(err, errFalloDelLote) || res != nil {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	if len(base.prestamos) != 0 {
		t.Fatal("el préstamo se confirmó pese a fallar la liberación")
	}
	for _, r := range base.reservas {
		if r.Estado != domain.ReservaConfirmada {
			t.Fatal("quedó una reserva liberada tras el fallo")
		}
	}
}

func TestDevolucionLote_ErrorDeEscrituraRevierteTodo(t *testing.T) {
	base := nuevoFakeRepo()
	svc := nuevoServicioDeTest(base)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{EquipoIDs: []string{"pc1", "pc2"}, Nombre: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	svc.repo = &repoConFalloDeLote{fakeRepo: base, operacion: "devolucion"}
	res, err := svc.RecibirEquipos(context.Background(), []string{entrega.Entregadas[0].ID, entrega.Entregadas[1].ID}, "", "")
	if !errors.Is(err, errFalloDelLote) || res != nil {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	for _, p := range base.prestamos {
		if !p.EstaAbierto() {
			t.Fatal("quedó una devolución oculta tras el fallo")
		}
	}
}

func TestPrestamosLote_IDsRepetidosSeProcesanUnaVez(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := loteReservado(t, repo)
	r, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{ReservaIDs: []string{"a", "a", "b"}, EquiposEsperados: equiposEsperadosPrueba(repo, []string{"a", "a", "b"})})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entregadas) != 2 || len(r.NoEntregadas) != 0 || *r.Entregadas[0].ReservaID != "a" || *r.Entregadas[1].ReservaID != "b" {
		t.Fatalf("respuesta contradictoria o desordenada: %+v", r)
	}
	d, err := svc.RecibirEquipos(context.Background(), []string{r.Entregadas[0].ID, r.Entregadas[0].ID}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Recibidos) != 1 || len(d.NoRecibidos) != 0 {
		t.Fatalf("doble respuesta para un mismo ID: %+v", d)
	}
	s, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{EquipoIDs: []string{"otra", "otra"}, Nombre: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entregadas) != 1 || len(s.NoEntregadas) != 0 {
		t.Fatalf("se procesó dos veces el mismo equipo: %+v", s)
	}
}
