package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

var errPasoDelPedido = errors.New("fallo inyectado en resolución")

type repoPedidoConFallo struct {
	*fakeRepo
	paso string
}

func (r *repoPedidoConFallo) CrearMateria(ctx context.Context, m *domain.Materia) error {
	if err := r.fakeRepo.CrearMateria(ctx, m); err != nil {
		return err
	}
	if r.paso == "materia" {
		return errPasoDelPedido
	}
	return nil
}
func (r *repoPedidoConFallo) AsignarDocente(ctx context.Context, dm *domain.DocenteMateria) error {
	if err := r.fakeRepo.AsignarDocente(ctx, dm); err != nil {
		return err
	}
	if r.paso == "asignacion" {
		return errPasoDelPedido
	}
	return nil
}
func (r *repoPedidoConFallo) GuardarPedido(ctx context.Context, p *domain.PedidoDeMateria) error {
	if err := r.fakeRepo.GuardarPedido(ctx, p); err != nil {
		return err
	}
	if r.paso == "resolucion" {
		return errPasoDelPedido
	}
	return nil
}

func (r *repoPedidoConFallo) ListarDocentesDeMateria(ctx context.Context, id string) ([]*domain.DocenteMateria, error) {
	if r.paso == "lectura-rol" {
		return nil, errPasoDelPedido
	}
	return r.fakeRepo.ListarDocentesDeMateria(ctx, id)
}

func (r *repoPedidoConFallo) BuscarPedidoPorID(ctx context.Context, id string) (*domain.PedidoDeMateria, error) {
	p, err := r.fakeRepo.BuscarPedidoPorID(ctx, id)
	if err == nil && r.paso == "relectura" && p.Estado != domain.PedidoPendiente {
		return nil, errPasoDelPedido
	}
	return p, err
}

func pedidoNuevoDeTest(t *testing.T, repo *fakeRepo) string {
	t.Helper()
	p, err := domain.NuevoPedidoDeMateria("pedido1", "docente1", nil, "1°A", "Robótica", "Dictaré esta materia", relojDeTest())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearPedido(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestResolverPedido_FalloDeCadaPasoRevierteYPermiteReintento(t *testing.T) {
	for _, paso := range []string{"materia", "asignacion", "resolucion", "lectura-rol", "relectura"} {
		t.Run(paso, func(t *testing.T) {
			repo := &repoPedidoConFallo{fakeRepo: nuevoFakeRepo(), paso: paso}
			id := pedidoNuevoDeTest(t, repo.fakeRepo)
			svc := servicioSimple(repo)
			avisos := 0
			svc.bus.Subscribe("materia.pedido.resuelto", func(eventbus.Evento) { avisos++ })
			curso := "curso1"
			res, err := svc.ResolverPedido(context.Background(), id, "admin1", true, "Aprobado", &curso, nil)
			if !errors.Is(err, errPasoDelPedido) || res != nil {
				t.Fatalf("fallo no informado: %+v %v", res, err)
			}
			if len(repo.materias) != 0 || len(repo.docentesMateria) != 0 || repo.pedidos[id].Estado != domain.PedidoPendiente || avisos != 0 {
				t.Fatalf("quedaron efectos de la operación fallida: materias=%d asignaciones=%d pedido=%+v avisos=%d", len(repo.materias), len(repo.docentesMateria), repo.pedidos[id], avisos)
			}
			repo.paso = ""
			res, err = svc.ResolverPedido(context.Background(), id, "admin1", true, "Aprobado", &curso, nil)
			if err != nil || res.Estado != domain.PedidoAprobado || len(repo.materias) != 1 || len(repo.docentesMateria) != 1 || avisos != 1 {
				t.Fatalf("reintento incorrecto: %+v %v", res, err)
			}
		})
	}
}

func TestResolverPedido_RepeticionConservaConstanciaSinDuplicarEfectos(t *testing.T) {
	for _, aprobar := range []bool{true, false} {
		t.Run(map[bool]string{true: "aprobar", false: "rechazar"}[aprobar], func(t *testing.T) {
			repo := nuevoFakeRepo()
			id := pedidoNuevoDeTest(t, repo)
			svc := servicioSimple(repo)
			avisos := 0
			svc.bus.Subscribe("materia.pedido.resuelto", func(eventbus.Evento) { avisos++ })
			curso := "curso1"
			original, err := svc.ResolverPedido(context.Background(), id, "admin1", aprobar, "Respuesta original", &curso, nil)
			if err != nil {
				t.Fatal(err)
			}
			if aprobar && (original.MateriaCreadaID == nil || repo.materias[*original.MateriaCreadaID] == nil || !original.EsMateriaNueva() || original.MateriaSolicitada != "Robótica") {
				t.Fatalf("se perdió el vínculo o la solicitud original: %+v", original)
			}
			// Una nueva fecha y otros parámetros no cambian la decisión confirmada.
			svc.ahora = func() time.Time { return relojDeTest().Add(time.Hour) }
			rol := domain.RolSuplente
			repetido, err := svc.ResolverPedido(context.Background(), id, "admin2", aprobar, "Otra respuesta", nil, &rol)
			if err != nil || !reflect.DeepEqual(original, repetido) || avisos != 1 {
				t.Fatalf("repetición cambió resultado o avisos: %+v %+v %v avisos=%d", original, repetido, err, avisos)
			}
			if res, err := svc.ResolverPedido(context.Background(), id, "admin2", !aprobar, "Contraria", &curso, nil); !errors.Is(err, domain.ErrPedidoResuelto) || res != nil {
				t.Fatalf("se aceptó la decisión contraria: %+v %v", res, err)
			}
			esperadas := 0
			if aprobar {
				esperadas = 1
			}
			if len(repo.materias) != esperadas || len(repo.docentesMateria) != esperadas {
				t.Fatalf("efectos repetidos: %d %d", len(repo.materias), len(repo.docentesMateria))
			}
		})
	}
}

func TestResolverPedido_MateriaExistenteConservaRolPorDefectoYRolExplicito(t *testing.T) {
	for _, caso := range []struct {
		nombre      string
		otroDocente bool
		rol         *domain.RolDocente
		esperado    domain.RolDocente
	}{
		{nombre: "primer-docente", esperado: domain.RolTitular},
		{nombre: "otro-docente", otroDocente: true, esperado: domain.RolSuplente},
		{nombre: "rol-explicito", otroDocente: true, rol: func() *domain.RolDocente { r := domain.RolTitular; return &r }(), esperado: domain.RolTitular},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			repo.materias["m1"] = &domain.Materia{ID: "m1", CursoID: "c1", Nombre: "Matemática"}
			if caso.otroDocente {
				repo.docentesMateria["dm1"] = domain.NuevoDocenteMateria("dm1", "docente2", "m1", domain.RolTitular)
			}
			materia := "m1"
			p, err := domain.NuevoPedidoDeMateria("p1", "docente1", &materia, "", "", "Segundo turno", relojDeTest())
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CrearPedido(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			res, err := servicioSimple(repo).ResolverPedido(context.Background(), p.ID, "admin1", true, "", nil, caso.rol)
			if err != nil || res.MateriaCreadaID != nil || *res.MateriaID != materia {
				t.Fatalf("resolución existente: %+v %v", res, err)
			}
			var asignada *domain.DocenteMateria
			for _, dm := range repo.docentesMateria {
				if dm.UsuarioID == "docente1" {
					asignada = dm
				}
			}
			if asignada == nil || asignada.Rol != caso.esperado {
				t.Fatalf("rol incorrecto: %+v", asignada)
			}
		})
	}
}

func TestResolverPedido_RespuestaInvalidaNoCreaMateriaNiAsignacion(t *testing.T) {
	repo := nuevoFakeRepo()
	id := pedidoNuevoDeTest(t, repo)
	svc := servicioSimple(repo)
	curso := "curso1"
	res, err := svc.ResolverPedido(context.Background(), id, "admin1", true, strings.Repeat("a", domain.MaxRespuesta+1), &curso, nil)
	if !errors.Is(err, domain.ErrRespuestaLarga) || res != nil || len(repo.materias) != 0 || len(repo.docentesMateria) != 0 || repo.pedidos[id].Estado != domain.PedidoPendiente {
		t.Fatalf("respuesta inválida con efectos: %+v %v materias=%d asignaciones=%d", res, err, len(repo.materias), len(repo.docentesMateria))
	}
}
