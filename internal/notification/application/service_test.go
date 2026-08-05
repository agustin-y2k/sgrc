package application

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	notificaciones map[string]*domain.Notificacion
	errCrear       error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{notificaciones: make(map[string]*domain.Notificacion)}
}

func (r *fakeRepo) Crear(ctx context.Context, n *domain.Notificacion) error {
	if r.errCrear != nil {
		return r.errCrear
	}
	r.notificaciones[n.ID] = n
	return nil
}

func (r *fakeRepo) BuscarPorID(ctx context.Context, id string) (*domain.Notificacion, error) {
	n, ok := r.notificaciones[id]
	if !ok {
		return nil, ErrNotificacionNoEncontrada
	}
	return n, nil
}

func (r *fakeRepo) Guardar(ctx context.Context, n *domain.Notificacion) error {
	r.notificaciones[n.ID] = n
	return nil
}

func (r *fakeRepo) ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error) {
	var resultado []*domain.Notificacion
	for _, id := range r.idsOrdenados() {
		n := r.notificaciones[id]
		if n.UsuarioID != usuarioID {
			continue
		}
		if filtroEstado != nil && n.Estado != *filtroEstado {
			continue
		}
		resultado = append(resultado, n)
	}

	total := len(resultado)
	desde := pagina.Offset()
	if desde >= total {
		return nil, total, nil
	}
	hasta := desde + pagina.Limit()
	if hasta > total {
		hasta = total
	}
	return resultado[desde:hasta], total, nil
}

// idsOrdenados da un orden estable donde el repo real ordena por fecha:
// sobre el map pelado, LIMIT/OFFSET devolvería una página distinta en cada
// corrida.
func (r *fakeRepo) idsOrdenados() []string {
	ids := make([]string, 0, len(r.notificaciones))
	for id := range r.notificaciones {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *fakeRepo) ListarNoLeidasSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) ([]*domain.Notificacion, error) {
	var resultado []*domain.Notificacion
	for _, id := range r.idsOrdenados() {
		n := r.notificaciones[id]
		if n.SobreUsuarioID != nil && *n.SobreUsuarioID == sobreUsuarioID &&
			n.Tipo == tipo && n.Estado == domain.NoLeida {
			resultado = append(resultado, n)
		}
	}
	return resultado, nil
}

// ── fakeListadorAdmins ──────────────────────────────────────────────────

type fakeListadorAdmins struct {
	adminIDs []string
	err      error
}

func (f *fakeListadorAdmins) IDsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.adminIDs, nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

func nuevoServicioDeTest(repo Repo, listador ListadorAdmins) *Service {
	contadorID = 0
	return NewService(repo, listador, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	})
}

// ── NotificarUsuario ────────────────────────────────────────────────────

func TestNotificarUsuario_OK(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo(), &fakeListadorAdmins{})

	n, err := svc.NotificarUsuario(context.Background(), "usuario1", "Tu reserva fue cancelada", domain.TipoReservaCancelada, domain.Referencias{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.Estado != domain.NoLeida {
		t.Errorf("estado inicial incorrecto: %s", n.Estado)
	}
}

func TestNotificarUsuario_MensajeVacio_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo(), &fakeListadorAdmins{})

	_, err := svc.NotificarUsuario(context.Background(), "usuario1", "", domain.TipoGeneral, domain.Referencias{})

	if !errors.Is(err, domain.ErrMensajeVacio) {
		t.Fatalf("esperaba ErrMensajeVacio, obtuve %v", err)
	}
}

func TestNotificarUsuario_ErrorDelRepo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCrear = errors.New("la base está caída")
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	_, err := svc.NotificarUsuario(context.Background(), "usuario1", "mensaje", domain.TipoGeneral, domain.Referencias{})

	if err == nil {
		t.Fatal("esperaba que el error del repo se propague")
	}
}

// ── NotificarATodosLosAdmins ────────────────────────────────────────────

func TestNotificarATodosLosAdmins_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2", "admin3"}}
	svc := nuevoServicioDeTest(repo, listador)

	creadas, err := svc.NotificarATodosLosAdmins(context.Background(), "Docente pendiente de aprobación", domain.TipoDocentePendiente, domain.Referencias{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if creadas != 3 {
		t.Errorf("esperaba 3 notificaciones creadas, obtuve %d", creadas)
	}
	if len(repo.notificaciones) != 3 {
		t.Errorf("esperaba 3 notificaciones en el repo, hay %d", len(repo.notificaciones))
	}
}

func TestNotificarATodosLosAdmins_SinAdmins_NoCreaNada(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo(), &fakeListadorAdmins{adminIDs: nil})

	creadas, err := svc.NotificarATodosLosAdmins(context.Background(), "mensaje", domain.TipoGeneral, domain.Referencias{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if creadas != 0 {
		t.Errorf("esperaba 0, obtuve %d", creadas)
	}
}

func TestNotificarATodosLosAdmins_ErrorListandoAdmins_SePropaga(t *testing.T) {
	listador := &fakeListadorAdmins{err: errors.New("auth caído")}
	svc := nuevoServicioDeTest(nuevoFakeRepo(), listador)

	_, err := svc.NotificarATodosLosAdmins(context.Background(), "mensaje", domain.TipoGeneral, domain.Referencias{})

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

// ── MarcarLeida ─────────────────────────────────────────────────────────

func TestMarcarLeida_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", Estado: domain.NoLeida}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	err := svc.MarcarLeida(context.Background(), "n1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.notificaciones["n1"].Estado != domain.Leida {
		t.Errorf("estado final incorrecto: %s", repo.notificaciones["n1"].Estado)
	}
}

func TestMarcarLeida_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo(), &fakeListadorAdmins{})

	err := svc.MarcarLeida(context.Background(), "no-existe")

	if !errors.Is(err, ErrNotificacionNoEncontrada) {
		t.Fatalf("esperaba ErrNotificacionNoEncontrada, obtuve %v", err)
	}
}

func TestMarcarLeida_YaLeida_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", Estado: domain.Leida}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	err := svc.MarcarLeida(context.Background(), "n1")

	if !errors.Is(err, domain.ErrYaLeida) {
		t.Fatalf("esperaba ErrYaLeida, obtuve %v", err)
	}
}

// ── ListarPorUsuario ─────────────────────────────────────────────────────

func TestListarPorUsuario_SoloLasDeEseUsuario(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "usuario1"}
	repo.notificaciones["n2"] = &domain.Notificacion{ID: "n2", UsuarioID: "usuario2"}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	resultado, total, err := svc.ListarPorUsuario(context.Background(), "usuario1", nil, paginacion.PorDefecto())

	if total != 1 {
		t.Errorf("total = %d, esperaba 1", total)
	}
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].ID != "n1" {
		t.Fatalf("esperaba solo n1, obtuve %+v", resultado)
	}
}

func TestListarPorUsuario_FiltraPorEstado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "usuario1", Estado: domain.NoLeida}
	repo.notificaciones["n2"] = &domain.Notificacion{ID: "n2", UsuarioID: "usuario1", Estado: domain.Leida}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	noLeida := domain.NoLeida
	resultado, total, err := svc.ListarPorUsuario(context.Background(), "usuario1", &noLeida, paginacion.PorDefecto())

	if total != 1 {
		t.Errorf("total = %d, esperaba 1 (el filtro cuenta antes del LIMIT)", total)
	}
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].ID != "n1" {
		t.Fatalf("esperaba solo n1 (NO_LEIDA), obtuve %+v", resultado)
	}
}

// ── ObtenerNotificacion ─────────────────────────────────────────────────

func TestObtenerNotificacion_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "usuario1"}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	n, err := svc.ObtenerNotificacion(context.Background(), "n1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n.UsuarioID != "usuario1" {
		t.Errorf("notificación incorrecta: %+v", n)
	}
}

func TestObtenerNotificacion_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo(), &fakeListadorAdmins{})

	_, err := svc.ObtenerNotificacion(context.Background(), "no-existe")

	if !errors.Is(err, ErrNotificacionNoEncontrada) {
		t.Fatalf("esperaba ErrNotificacionNoEncontrada, obtuve %v", err)
	}
}

func (r *fakeRepo) MarcarTodasLeidasDe(ctx context.Context, usuarioID string, ahora time.Time) (int, error) {
	n := 0
	for _, notif := range r.notificaciones {
		if notif.UsuarioID == usuarioID && notif.Estado == domain.NoLeida {
			if err := notif.MarcarLeida(ahora); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// La página sin inicializar es un caso real: cualquier llamador que arme el
// filtro a mano y no toque Pagina daría LIMIT 0, o sea una lista vacía sin
// ningún error que lo explique.
func TestListarPorUsuario_PaginaEnCero_UsaLaVentanaPorDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.notificaciones["n1"] = &domain.Notificacion{ID: "n1", UsuarioID: "usuario1"}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	resultado, total, err := svc.ListarPorUsuario(context.Background(), "usuario1", nil, paginacion.Pagina{})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || total != 1 {
		t.Fatalf("esperaba la única notificación, obtuve %d (total %d)", len(resultado), total)
	}
}

func TestListarPorUsuario_SegundaPagina(t *testing.T) {
	repo := nuevoFakeRepo()
	for _, id := range []string{"n1", "n2", "n3"} {
		repo.notificaciones[id] = &domain.Notificacion{ID: id, UsuarioID: "usuario1"}
	}
	svc := nuevoServicioDeTest(repo, &fakeListadorAdmins{})

	resultado, total, err := svc.ListarPorUsuario(context.Background(), "usuario1", nil,
		paginacion.Pagina{Numero: 2, Tamanio: 2})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, esperaba 3 (todas las que matchean, no las de la página)", total)
	}
	if len(resultado) != 1 || resultado[0].ID != "n3" {
		t.Fatalf("esperaba solo n3 en la segunda página, obtuve %+v", resultado)
	}
}
