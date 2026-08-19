package application

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

// ── Dobles ──────────────────────────────────────────────────────────────

type fakeRepo struct {
	hilos map[string]*domain.Sugerencia
	err   error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{hilos: map[string]*domain.Sugerencia{}}
}

func (f *fakeRepo) Crear(_ context.Context, s *domain.Sugerencia) error {
	if f.err != nil {
		return f.err
	}
	f.hilos[s.ID] = s
	return nil
}

func (f *fakeRepo) BuscarPorID(_ context.Context, id string) (*domain.Sugerencia, error) {
	s, ok := f.hilos[id]
	if !ok {
		return nil, domain.ErrSugerenciaNoExist
	}
	return s, nil
}

func (f *fakeRepo) AgregarMensaje(_ context.Context, s *domain.Sugerencia, _ domain.Mensaje) error {
	if f.err != nil {
		return f.err
	}
	f.hilos[s.ID] = s
	return nil
}

func (f *fakeRepo) GuardarEstado(_ context.Context, s *domain.Sugerencia) error {
	if f.err != nil {
		return f.err
	}
	f.hilos[s.ID] = s
	return nil
}

func (f *fakeRepo) ListarTodas(context.Context, bool, paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) ListarDeUsuario(context.Context, string, paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	return nil, 0, nil
}

func (f *fakeRepo) ContarAbiertas(context.Context) (int, error) { return 0, nil }

type fakeUsuario struct {
	nombre, email string
	err           error
}

func (f *fakeUsuario) NombreYEmail(context.Context, string) (string, string, error) {
	return f.nombre, f.email, f.err
}

// busDePrueba junta lo publicado para poder afirmar sobre los eventos, que es
// lo único que este servicio le cuenta al resto del sistema.
type busDePrueba struct{ eventos []eventbus.Evento }

func (b *busDePrueba) Publish(e eventbus.Evento)               { b.eventos = append(b.eventos, e) }
func (b *busDePrueba) Subscribe(string, func(eventbus.Evento)) {}

func (b *busDePrueba) ultimo() eventbus.Evento {
	if len(b.eventos) == 0 {
		return eventbus.Evento{}
	}
	return b.eventos[len(b.eventos)-1]
}

var ahoraDePrueba = time.Date(2026, time.August, 19, 10, 0, 0, 0, time.UTC)

func servicioDePrueba() (*Service, *fakeRepo, *busDePrueba) {
	repo := nuevoFakeRepo()
	bus := &busDePrueba{}
	var n int
	nuevoID := func() string {
		n++
		return "id-" + strconv.Itoa(n)
	}
	svc := NewService(repo, &fakeUsuario{nombre: "Ada Lovelace", email: "ada@escuela.edu.ar"},
		nuevoID, func() time.Time { return ahoraDePrueba }, bus)
	return svc, repo, bus
}

// ── Escribir ────────────────────────────────────────────────────────────

func TestEscribir_GuardaYAvisaALosAdmins(t *testing.T) {
	svc, repo, bus := servicioDePrueba()

	sug, err := svc.Escribir(context.Background(), "u1", "AYUDA",
		"No arranca la PC 3", "la enciendo y no pasa nada", "/reservas", "1.10.0")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if len(repo.hilos) != 1 || len(sug.Mensajes) != 1 {
		t.Fatalf("esperaba un hilo con su primer mensaje: %+v", sug)
	}

	e := bus.ultimo()
	if e.Tipo != "sugerencia.nueva" {
		t.Fatalf("esperaba sugerencia.nueva, obtuve %q", e.Tipo)
	}
	payload, ok := e.Payload.(eventbus.SugerenciaNueva)
	if !ok {
		t.Fatalf("payload inesperado: %+v", e.Payload)
	}
	// El tipo viaja porque de él depende que el correo se pueda desactivar.
	if payload.Tipo != "AYUDA" || payload.Asunto != "No arranca la PC 3" {
		t.Errorf("el aviso no lleva de qué se trata: %+v", payload)
	}
	if payload.Quien != "Ada Lovelace" || payload.Texto != "la enciendo y no pasa nada" {
		t.Errorf("el aviso no dice quién ni qué: %+v", payload)
	}
}

// Que no se pueda resolver el nombre no puede hacer perder el mensaje: ya
// está guardado, que es lo que no se recupera.
func TestEscribir_SinNombre_IgualGuardaYAvisa(t *testing.T) {
	repo := nuevoFakeRepo()
	bus := &busDePrueba{}
	svc := NewService(repo, &fakeUsuario{err: errors.New("la base no está")},
		func() string { return "id-1" }, func() time.Time { return ahoraDePrueba }, bus)

	if _, err := svc.Escribir(context.Background(), "u1", "PROBLEMA", "Algo", "no anda", "", ""); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if len(repo.hilos) != 1 || len(bus.eventos) != 1 {
		t.Errorf("el mensaje y el aviso tenían que salir igual: %d hilos, %d eventos",
			len(repo.hilos), len(bus.eventos))
	}
}

func TestEscribir_TipoInvalido(t *testing.T) {
	svc, _, _ := servicioDePrueba()

	_, err := svc.Escribir(context.Background(), "u1", "QUEJA", "Asunto", "texto", "", "")

	if !errors.Is(err, domain.ErrTipoInvalido) {
		t.Errorf("esperaba ErrTipoInvalido, obtuve %v", err)
	}
}

// ── Responder ───────────────────────────────────────────────────────────

func TestResponder_DelAdmin_AvisaAQuienEscribio(t *testing.T) {
	svc, _, bus := servicioDePrueba()
	sug, _ := svc.Escribir(context.Background(), "u1", "AYUDA", "No arranca", "ayuda", "", "")

	actualizado, err := svc.Responder(context.Background(), sug.ID, "admin1", true, "vamos para allá")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if len(actualizado.Mensajes) != 2 {
		t.Fatalf("esperaba dos mensajes, hay %d", len(actualizado.Mensajes))
	}
	// Contestar no cierra: eso es MarcarResuelta.
	if actualizado.Estado != domain.Abierta {
		t.Errorf("contestar no debería cerrar el hilo: %q", actualizado.Estado)
	}

	e := bus.ultimo()
	if e.Tipo != "sugerencia.respondida" {
		t.Fatalf("esperaba sugerencia.respondida, obtuve %q", e.Tipo)
	}
	payload := e.Payload.(eventbus.SugerenciaRespondida)
	if payload.Email != "ada@escuela.edu.ar" || payload.Respuesta != "vamos para allá" {
		t.Errorf("el aviso no lleva a quién ni qué: %+v", payload)
	}
	if payload.Tipo != "AYUDA" || payload.Asunto != "No arranca" {
		t.Errorf("falta de qué hilo se trata: %+v", payload)
	}
}

func TestResponder_DelDocente_VuelveALosAdmins(t *testing.T) {
	svc, _, bus := servicioDePrueba()
	sug, _ := svc.Escribir(context.Background(), "u1", "PROBLEMA", "No me deja", "nada", "", "")
	if _, err := svc.Responder(context.Background(), sug.ID, "admin1", true, "probá de nuevo"); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if _, err := svc.Responder(context.Background(), sug.ID, "u1", false, "sigue igual"); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	e := bus.ultimo()
	if e.Tipo != "sugerencia.seguimiento" {
		t.Fatalf("esperaba sugerencia.seguimiento, obtuve %q", e.Tipo)
	}
	payload := e.Payload.(eventbus.SugerenciaSeguimiento)
	if payload.Texto != "sigue igual" || payload.Asunto != "No me deja" {
		t.Errorf("el aviso no dice qué escribió ni sobre qué: %+v", payload)
	}
}

// Una conversación es entre dos: un tercero no escribe en ella.
func TestResponder_DeUnTercero_NoLoDeja(t *testing.T) {
	svc, _, bus := servicioDePrueba()
	sug, _ := svc.Escribir(context.Background(), "u1", "AYUDA", "Asunto", "texto", "", "")
	eventosAntes := len(bus.eventos)

	_, err := svc.Responder(context.Background(), sug.ID, "otro-docente", false, "me meto")

	if !errors.Is(err, ErrNoEsTuya) {
		t.Fatalf("esperaba ErrNoEsTuya, obtuve %v", err)
	}
	if len(bus.eventos) != eventosAntes {
		t.Error("un pedido rechazado no debería publicar nada")
	}
}

func TestResponder_HiloQueNoExiste(t *testing.T) {
	svc, _, _ := servicioDePrueba()

	_, err := svc.Responder(context.Background(), "no-existe", "admin1", true, "hola")

	if !errors.Is(err, domain.ErrSugerenciaNoExist) {
		t.Errorf("esperaba ErrSugerenciaNoExist, obtuve %v", err)
	}
}

// ── Resolver ────────────────────────────────────────────────────────────

// Cerrar no manda correo: el aviso útil fue la respuesta, y un segundo mail
// diciendo "lo dimos por terminado" es ruido.
func TestMarcarResuelta_CierraSinAvisar(t *testing.T) {
	svc, _, bus := servicioDePrueba()
	sug, _ := svc.Escribir(context.Background(), "u1", "SUGERENCIA", "Una idea", "estaría bueno", "", "")
	eventosAntes := len(bus.eventos)

	cerrada, err := svc.MarcarResuelta(context.Background(), sug.ID)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if cerrada.Estado != domain.Resuelta {
		t.Errorf("esperaba resuelta, quedó %q", cerrada.Estado)
	}
	if len(bus.eventos) != eventosAntes {
		t.Errorf("cerrar no manda avisos: %+v", bus.eventos[eventosAntes:])
	}
}

func TestMarcarResuelta_DosVeces(t *testing.T) {
	svc, _, _ := servicioDePrueba()
	sug, _ := svc.Escribir(context.Background(), "u1", "SUGERENCIA", "Una idea", "texto", "", "")
	if _, err := svc.MarcarResuelta(context.Background(), sug.ID); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	_, err := svc.MarcarResuelta(context.Background(), sug.ID)

	if !errors.Is(err, domain.ErrYaResuelta) {
		t.Errorf("esperaba ErrYaResuelta, obtuve %v", err)
	}
}
