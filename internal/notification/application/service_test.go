package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	notificaciones map[string]*domain.Notificacion
	errCrear       error
	// errCrearPara falla solo para ese usuario, para poder probar el fallo
	// PARCIAL: errCrear falla siempre y no distingue "se cayó la base" de "un
	// destinatario dio problema", que es justo lo que hay que separar.
	errCrearPara map[string]error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{notificaciones: make(map[string]*domain.Notificacion)}
}

func (r *fakeRepo) Crear(ctx context.Context, n *domain.Notificacion) error {
	if r.errCrear != nil {
		return r.errCrear
	}
	if err, hay := r.errCrearPara[n.UsuarioID]; hay {
		return err
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

// idsOrdenados da un orden estable donde el repo real ordena por fecha: sobre
// el map pelado, LIMIT/OFFSET devolvería una página distinta en cada corrida.
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
	adminIDs    []string
	adminEmails []string
	// suscriptos, si está, dice quién pidió cada categoría; en nil, todos los
	// de adminEmails reciben todo, que es lo que suponen los tests que no
	// hablan de preferencias.
	suscriptos   map[domain.CategoriaEmail][]string
	err          error
	errorEnEmail error
}

func (f *fakeListadorAdmins) IDsDeAdminsAprobados(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.adminIDs, nil
}

func (f *fakeListadorAdmins) EmailsDeAdminsSuscriptos(ctx context.Context, categoria domain.CategoriaEmail) ([]string, error) {
	if f.errorEnEmail != nil {
		return nil, f.errorEnEmail
	}
	if f.suscriptos == nil {
		return f.adminEmails, nil
	}
	return f.suscriptos[categoria], nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

// fakePreferencias guarda las decisiones explícitas en memoria, igual que la
// tabla: lo que no está es lo que la persona nunca eligió.
type fakePreferencias struct {
	porUsuario map[string]map[domain.CategoriaEmail]bool
	// porEmail es lo mismo indexado como lo consulta el envío. Vacío = nadie
	// eligió nada, así que mandan los valores por defecto.
	porEmail map[string]map[domain.CategoriaEmail]bool
	// siempreSi es para los tests que van sobre el TEXTO de un correo y no
	// sobre a quién le llega: dice que sí a todo y los deja al margen de los
	// valores por defecto.
	siempreSi bool
	err       error
}

func (f *fakePreferencias) ElegidasDe(ctx context.Context, usuarioID string) (map[domain.CategoriaEmail]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.porUsuario[usuarioID], nil
}

func (f *fakePreferencias) Reemplazar(ctx context.Context, usuarioID string, decisiones map[domain.CategoriaEmail]bool) error {
	if f.err != nil {
		return f.err
	}
	if f.porUsuario == nil {
		f.porUsuario = map[string]map[domain.CategoriaEmail]bool{}
	}
	f.porUsuario[usuarioID] = decisiones
	return nil
}

func (f *fakePreferencias) RecibePorEmail(ctx context.Context, email string, categoria domain.CategoriaEmail) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.siempreSi {
		return true, nil
	}
	if activa, decidio := f.porEmail[email][categoria]; decidio {
		return activa, nil
	}
	return categoria.ActivaPorDefecto(), nil
}

func nuevoServicioDeTest(repo Repo, listador ListadorAdmins) *Service {
	return nuevoServicioConPreferencias(repo, listador, &fakePreferencias{})
}

func nuevoServicioConPreferencias(repo Repo, listador ListadorAdmins, prefs PreferenciasEmail) *Service {
	contadorID = 0
	return NewService(repo, listador, prefs, idSecuencial, func() time.Time {
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

// Un Admin que falla no puede dejar sin aviso a los que siguen: si se corta
// en el primero, el tercero nunca se entera de que hay una cuenta esperando
// aprobación o una máquina que no volvió.
func TestNotificarATodosLosAdmins_UnoFalla_LosDemasIgualSeEnteran(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCrearPara = map[string]error{"admin2": errors.New("cuenta borrada recién")}
	listador := &fakeListadorAdmins{adminIDs: []string{"admin1", "admin2", "admin3"}}
	svc := nuevoServicioDeTest(repo, listador)

	creadas, err := svc.NotificarATodosLosAdmins(context.Background(), "mensaje", domain.TipoGeneral, domain.Referencias{})

	// El error se devuelve igual: queda constancia de a quién no se le pudo.
	if err == nil {
		t.Fatal("esperaba que informara el fallo parcial")
	}
	if !strings.Contains(err.Error(), "admin2") {
		t.Errorf("el error tiene que decir a quién no se pudo: %v", err)
	}
	if creadas != 2 {
		t.Errorf("esperaba 2 notificaciones creadas, obtuve %d", creadas)
	}
	// Lo que importa: admin3 va DESPUÉS del que falló y tiene que estar.
	if !notificoA(repo, "admin3") {
		t.Error("admin3 se quedó sin el aviso por un fallo de admin2")
	}
	if !notificoA(repo, "admin1") {
		t.Error("admin1 tendría que haber recibido el aviso")
	}
}

func notificoA(repo *fakeRepo, usuarioID string) bool {
	for _, n := range repo.notificaciones {
		if n.UsuarioID == usuarioID {
			return true
		}
	}
	return false
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

// ── Preferencias de correo (RF-05.13) ───────────────────────────────────

// Contiene dice si la lista trae esa categoría.
func contiene(cats []domain.CategoriaEmail, buscada domain.CategoriaEmail) bool {
	for _, c := range cats {
		if c == buscada {
			return true
		}
	}
	return false
}

// Un Admin que nunca abrió el panel: los dos de su cuenta, las cinco
// personales que vienen encendidas, y de las de administración solo las
// cuentas esperando aprobación.
func TestCategoriasDeEmail_LoQueRecibeQuienNuncaEligio(t *testing.T) {
	svc := nuevoServicioConPreferencias(nuevoFakeRepo(), &fakeListadorAdmins{}, &fakePreferencias{})

	cats, err := svc.CategoriasDeEmail(context.Background(), "admin1", true)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !contiene(cats, domain.CatCuentaPendiente) {
		t.Errorf("las cuentas pendientes tendrían que venir encendidas: %v", cats)
	}
	for _, apagada := range []domain.CategoriaEmail{
		domain.CatLicenciaPorVencer, domain.CatSugerencia, domain.CatPedidoDeMateria,
		domain.CatDevolucionDemorada, domain.CatCierreSinDevolver,
		domain.CatRecordatorioDeReserva, domain.CatReservaSinRetirar, domain.CatDevolucionPendiente,
	} {
		if contiene(cats, apagada) {
			t.Errorf("%s tendría que arrancar apagada: %v", apagada, cats)
		}
	}
}

// Un docente no ve ni recibe las de administración, aunque la base tuviera
// una fila suya diciendo que sí.
func TestCategoriasDeEmail_ElDocenteNoRecibeLasDeAdministracion(t *testing.T) {
	prefs := &fakePreferencias{porUsuario: map[string]map[domain.CategoriaEmail]bool{
		"docente1": {domain.CatSugerencia: true},
	}}
	svc := nuevoServicioConPreferencias(nuevoFakeRepo(), &fakeListadorAdmins{}, prefs)

	cats, err := svc.CategoriasDeEmail(context.Background(), "docente1", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if contiene(cats, domain.CatSugerencia) {
		t.Errorf("le devolvió una categoría de administración: %v", cats)
	}
}

// Destildar lo que viene encendido se guarda: el default vale hasta que la
// persona se pronuncia, no vuelve a aplicarse después.
func TestGuardarCategoriasDeEmail_DestildarLoEncendido_QuedaApagado(t *testing.T) {
	prefs := &fakePreferencias{}
	svc := nuevoServicioConPreferencias(nuevoFakeRepo(), &fakeListadorAdmins{}, prefs)
	ctx := context.Background()

	if _, err := svc.GuardarCategoriasDeEmail(ctx, "admin1",
		[]domain.CategoriaEmail{domain.CatSugerencia}, true); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	cats, err := svc.CategoriasDeEmail(ctx, "admin1", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if contiene(cats, domain.CatCuentaPendiente) {
		t.Errorf("se destildó y volvió sola: %v", cats)
	}
	if !contiene(cats, domain.CatSugerencia) {
		t.Errorf("lo que tildó no quedó: %v", cats)
	}
}

// Las fijas salen siempre, tilde lo que tilde: los dos de la cuenta y la
// respuesta a un pedido de ayuda.
func TestGuardarCategoriasDeEmail_LasFijasNoSeApagan(t *testing.T) {
	prefs := &fakePreferencias{}
	svc := nuevoServicioConPreferencias(nuevoFakeRepo(), &fakeListadorAdmins{}, prefs)
	ctx := context.Background()

	cats, err := svc.GuardarCategoriasDeEmail(ctx, "docente1", nil, false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("esperaba las tres fijas que ve un docente, obtuve %v", cats)
	}
	for _, c := range cats {
		if !c.EsFija() {
			t.Errorf("%s no es fija y quedó encendida", c)
		}
	}
	// Y no se guardó ninguna decisión sobre ellas.
	for c := range prefs.porUsuario["docente1"] {
		if c.EsFija() {
			t.Errorf("guardó una decisión sobre un correo que sale siempre: %s", c)
		}
	}
}

func TestGuardarCategoriasDeEmail_ErrorDelRepo_SePropaga(t *testing.T) {
	prefs := &fakePreferencias{err: errors.New("la base no está")}
	svc := nuevoServicioConPreferencias(nuevoFakeRepo(), &fakeListadorAdmins{}, prefs)

	if _, err := svc.GuardarCategoriasDeEmail(context.Background(), "admin1", nil, true); err == nil {
		t.Error("esperaba error y no hubo")
	}
}
