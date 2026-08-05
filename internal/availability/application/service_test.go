package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/availability/domain"
)

// ── fakeRepo ────────────────────────────────────────────────────────────
//
// Replica en memoria el criterio real de titularidad acotada por
// usuario_id (ver ports.go): BuscarBloqueDeUsuario, GuardarBloque y
// EliminarBloqueDeUsuario tratan "existe pero es de otro usuario" igual
// que "no existe".

type fakeRepo struct {
	bloques     map[string]*domain.BloqueHorario
	excepciones map[string]*domain.Excepcion // clave: usuarioID + "|" + fecha AAAA-MM-DD

	llamadasBloquesEnLote     int
	llamadasExcepcionesEnLote int
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		bloques:     make(map[string]*domain.BloqueHorario),
		excepciones: make(map[string]*domain.Excepcion),
	}
}

func claveExcepcion(usuarioID string, fecha time.Time) string {
	return usuarioID + "|" + fecha.Format("2006-01-02")
}

func (r *fakeRepo) ListarBloquesDeUsuario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error) {
	var resultado []*domain.BloqueHorario
	for _, b := range r.bloques {
		if b.UsuarioID == usuarioID {
			resultado = append(resultado, b)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CrearBloque(ctx context.Context, b *domain.BloqueHorario) error {
	r.bloques[b.ID] = b
	return nil
}

func (r *fakeRepo) BuscarBloqueDeUsuario(ctx context.Context, id, usuarioID string) (*domain.BloqueHorario, error) {
	b, ok := r.bloques[id]
	if !ok || b.UsuarioID != usuarioID {
		return nil, ErrBloqueNoEncontrado
	}
	return b, nil
}

func (r *fakeRepo) GuardarBloque(ctx context.Context, b *domain.BloqueHorario) error {
	existente, ok := r.bloques[b.ID]
	if !ok || existente.UsuarioID != b.UsuarioID {
		return ErrBloqueNoEncontrado
	}
	r.bloques[b.ID] = b
	return nil
}

func (r *fakeRepo) EliminarBloqueDeUsuario(ctx context.Context, id, usuarioID string) error {
	b, ok := r.bloques[id]
	if !ok || b.UsuarioID != usuarioID {
		return ErrBloqueNoEncontrado
	}
	delete(r.bloques, id)
	return nil
}

func (r *fakeRepo) BuscarExcepcionDeFecha(ctx context.Context, usuarioID string, fecha time.Time) (*domain.Excepcion, error) {
	e, ok := r.excepciones[claveExcepcion(usuarioID, fecha)]
	if !ok {
		return nil, nil
	}
	return e, nil
}

// Las versiones en lote se implementan reusando las individuales: lo que se
// prueba en application/ es que el servicio arme bien el resultado, no el
// SQL —eso vive en infrastructure/ y va contra Postgres real—. Cuentan las
// llamadas para poder verificar que el listado dejó de hacer 2N viajes.
func (r *fakeRepo) ListarBloquesDeUsuarios(ctx context.Context, usuarioIDs []string) (map[string][]*domain.BloqueHorario, error) {
	r.llamadasBloquesEnLote++
	resultado := make(map[string][]*domain.BloqueHorario, len(usuarioIDs))
	for _, id := range usuarioIDs {
		bloques, err := r.ListarBloquesDeUsuario(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(bloques) > 0 {
			resultado[id] = bloques
		}
	}
	return resultado, nil
}

func (r *fakeRepo) BuscarExcepcionesDeFecha(ctx context.Context, usuarioIDs []string, fecha time.Time) (map[string]*domain.Excepcion, error) {
	r.llamadasExcepcionesEnLote++
	resultado := make(map[string]*domain.Excepcion, len(usuarioIDs))
	for _, id := range usuarioIDs {
		e, err := r.BuscarExcepcionDeFecha(ctx, id, fecha)
		if err != nil {
			return nil, err
		}
		if e != nil {
			resultado[id] = e
		}
	}
	return resultado, nil
}

func (r *fakeRepo) GuardarExcepcion(ctx context.Context, e *domain.Excepcion) error {
	r.excepciones[claveExcepcion(e.UsuarioID, e.Fecha)] = e
	return nil
}

// ── fakeListadorAdmins ──────────────────────────────────────────────────

type fakeListadorAdmins struct {
	admins []AdminInfo
	err    error
}

func (f *fakeListadorAdmins) AdminsAprobados(ctx context.Context) ([]AdminInfo, error) {
	return f.admins, f.err
}

func idSecuencial() func() string {
	contador := 0
	return func() string {
		contador++
		return "id-" + string(rune('0'+contador))
	}
}

func ahoraFija(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// ── AgregarBloque ───────────────────────────────────────────────────────

func TestAgregarBloque_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))

	b, err := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if b.UsuarioID != "admin1" || b.DiaSemana != domain.Lunes {
		t.Errorf("bloque incorrecto: %+v", b)
	}
	if len(repo.bloques) != 1 {
		t.Errorf("esperaba 1 bloque persistido, hay %d", len(repo.bloques))
	}
}

func TestAgregarBloque_RangoInvalido_PropagaErrorDeDominio(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))

	_, err := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 12*time.Hour, 8*time.Hour)

	if !errors.Is(err, domain.ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
	if len(repo.bloques) != 0 {
		t.Error("no debería haber persistido nada")
	}
}

// ── EditarBloque ────────────────────────────────────────────────────────

func TestEditarBloque_ActualizaSoloElCampoIndicado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	original, _ := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	nuevoDia := domain.Martes
	actualizado, err := svc.EditarBloque(context.Background(), original.ID, "admin1", &nuevoDia, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if actualizado.DiaSemana != domain.Martes {
		t.Errorf("día no se actualizó: %s", actualizado.DiaSemana)
	}
	// Las horas, no tocadas, deben conservarse.
	if actualizado.HoraInicio != 8*time.Hour || actualizado.HoraFin != 12*time.Hour {
		t.Errorf("las horas no deberían haber cambiado: %v-%v", actualizado.HoraInicio, actualizado.HoraFin)
	}
}

func TestEditarBloque_RangoResultanteInvalido_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	original, _ := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	nuevaHoraInicio := 13 * time.Hour // queda después de HoraFin (12:00), que no se toca

	_, err := svc.EditarBloque(context.Background(), original.ID, "admin1", nil, &nuevaHoraInicio, nil)

	if !errors.Is(err, domain.ErrRangoHorarioInvalido) {
		t.Fatalf("esperaba ErrRangoHorarioInvalido, obtuve %v", err)
	}
}

func TestEditarBloque_DeOtroUsuario_ErrBloqueNoEncontrado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	original, _ := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	nuevoDia := domain.Martes
	_, err := svc.EditarBloque(context.Background(), original.ID, "admin2-intruso", &nuevoDia, nil, nil)

	if !errors.Is(err, ErrBloqueNoEncontrado) {
		t.Fatalf("esperaba ErrBloqueNoEncontrado (titularidad ajena), obtuve %v", err)
	}
}

func TestEditarBloque_NoExiste_ErrBloqueNoEncontrado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))

	nuevoDia := domain.Martes
	_, err := svc.EditarBloque(context.Background(), "no-existe", "admin1", &nuevoDia, nil, nil)

	if !errors.Is(err, ErrBloqueNoEncontrado) {
		t.Fatalf("esperaba ErrBloqueNoEncontrado, obtuve %v", err)
	}
}

// ── EliminarBloque ──────────────────────────────────────────────────────

func TestEliminarBloque_Propio_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	b, _ := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	err := svc.EliminarBloque(context.Background(), b.ID, "admin1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(repo.bloques) != 0 {
		t.Error("el bloque debería haberse eliminado")
	}
}

func TestEliminarBloque_DeOtroUsuario_ErrBloqueNoEncontrado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	b, _ := svc.AgregarBloque(context.Background(), "admin1", domain.Lunes, 8*time.Hour, 12*time.Hour)

	err := svc.EliminarBloque(context.Background(), b.ID, "admin2-intruso")

	if !errors.Is(err, ErrBloqueNoEncontrado) {
		t.Fatalf("esperaba ErrBloqueNoEncontrado, obtuve %v", err)
	}
	if len(repo.bloques) != 1 {
		t.Error("el bloque de admin1 no debería haberse tocado")
	}
}

// ── CargarExcepcion / MarcarNoDisponibleAhora ──────────────────────────

func TestCargarExcepcion_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	e, err := svc.CargarExcepcion(context.Background(), "admin1", fecha, domain.NoDisponible, nil, nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if e.UsuarioID != "admin1" || e.Tipo != domain.NoDisponible {
		t.Errorf("excepción incorrecta: %+v", e)
	}
}

func TestCargarExcepcion_SegundaCargaMismaFecha_Reemplaza(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	_, err := svc.CargarExcepcion(context.Background(), "admin1", fecha, domain.NoDisponible, nil, nil, nil)
	if err != nil {
		t.Fatalf("primera carga no debería fallar: %v", err)
	}

	horaInicio, horaFin := 9*time.Hour, 11*time.Hour
	segunda, err := svc.CargarExcepcion(context.Background(), "admin1", fecha, domain.HorarioModificado, &horaInicio, &horaFin, nil)
	if err != nil {
		t.Fatalf("segunda carga no debería fallar: %v", err)
	}

	guardada, _ := repo.BuscarExcepcionDeFecha(context.Background(), "admin1", fecha)
	if guardada.Tipo != domain.HorarioModificado || guardada.ID != segunda.ID {
		t.Errorf("la segunda carga debería haber reemplazado la primera: %+v", guardada)
	}
	if len(repo.excepciones) != 1 {
		t.Errorf("debería haber una sola excepción para (admin1, esa fecha), hay %d", len(repo.excepciones))
	}
}

func TestCargarExcepcion_Incoherente_PropagaError(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(time.Now()))
	fecha := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)

	_, err := svc.CargarExcepcion(context.Background(), "admin1", fecha, domain.HorarioModificado, nil, nil, nil)

	if !errors.Is(err, domain.ErrExcepcionIncoherente) {
		t.Fatalf("esperaba ErrExcepcionIncoherente, obtuve %v", err)
	}
}

func TestMarcarNoDisponibleAhora_CreaExcepcionNoDisponibleParaHoy(t *testing.T) {
	repo := nuevoFakeRepo()
	ahora := time.Date(2026, time.March, 9, 15, 30, 0, 0, time.UTC)
	svc := NewService(repo, &fakeListadorAdmins{}, idSecuencial(), ahoraFija(ahora))

	e, err := svc.MarcarNoDisponibleAhora(context.Background(), "admin1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if e.Tipo != domain.NoDisponible {
		t.Errorf("esperaba NO_DISPONIBLE, obtuve %s", e.Tipo)
	}
	if !e.Fecha.Equal(domain.FechaSolo(ahora)) {
		t.Errorf("la fecha debería ser la de hoy: %v", e.Fecha)
	}
}

// ── DisponibilidadDeTodosLosAdmins ─────────────────────────────────────

func TestDisponibilidadDeTodosLosAdmins_CombinaBloquesYExcepciones(t *testing.T) {
	repo := nuevoFakeRepo()
	// 9-mar-2026 es lunes, 10:00.
	ahora := time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)

	admins := []AdminInfo{
		{ID: "admin1", Nombre: "Ada", Apellido: "Lovelace"},
		{ID: "admin2", Nombre: "Alan", Apellido: "Turing"},
	}
	svc := NewService(repo, &fakeListadorAdmins{admins: admins}, idSecuencial(), ahoraFija(ahora))

	// admin1: tiene un bloque LUNES 08-12 que lo cubre a las 10:00 → disponible.
	repo.CrearBloque(context.Background(), &domain.BloqueHorario{
		ID: "b1", UsuarioID: "admin1", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
	})
	// admin2: el mismo bloque, pero además cargó una excepción NO_DISPONIBLE
	// para hoy → debe pisar el bloque y quedar no disponible.
	repo.CrearBloque(context.Background(), &domain.BloqueHorario{
		ID: "b2", UsuarioID: "admin2", DiaSemana: domain.Lunes, HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
	})
	repo.GuardarExcepcion(context.Background(), &domain.Excepcion{
		ID: "e1", UsuarioID: "admin2", Fecha: domain.FechaSolo(ahora), Tipo: domain.NoDisponible,
	})

	resultado, err := svc.DisponibilidadDeTodosLosAdmins(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 2 {
		t.Fatalf("esperaba 2 admins, obtuve %d", len(resultado))
	}

	porID := make(map[string]AdminDisponibilidad, 2)
	for _, r := range resultado {
		porID[r.UsuarioID] = r
	}

	if !porID["admin1"].DisponibleAhora {
		t.Error("admin1 debería figurar disponible (dentro de su bloque semanal)")
	}
	if porID["admin1"].ExcepcionHoy != nil {
		t.Error("admin1 no cargó ninguna excepción para hoy")
	}
	if porID["admin2"].DisponibleAhora {
		t.Error("admin2 debería figurar NO disponible (la excepción de hoy pisa el bloque)")
	}
	if porID["admin2"].ExcepcionHoy == nil {
		t.Error("admin2 sí cargó una excepción para hoy, debería venir en la respuesta")
	}
	if porID["admin1"].Nombre != "Ada" || porID["admin1"].Apellido != "Lovelace" {
		t.Errorf("nombre/apellido de admin1 incorrectos: %+v", porID["admin1"])
	}
}

// El listado hacía dos consultas por Admin (horario + excepción de hoy) en
// un for. Es la pantalla que mira cualquier docente que necesita ubicar a un
// Admin, así que crecía con el tamaño de la institución sin ninguna razón.
func TestDisponibilidadDeTodosLosAdmins_ConsultaEnLote_NoUnaVezPorAdmin(t *testing.T) {
	repo := nuevoFakeRepo()
	ahora := time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)

	admins := make([]AdminInfo, 0, 6)
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("admin%d", i)
		admins = append(admins, AdminInfo{ID: id, Nombre: "Admin", Apellido: id})
		repo.CrearBloque(context.Background(), &domain.BloqueHorario{
			ID: "b" + id, UsuarioID: id, DiaSemana: domain.Lunes,
			HoraInicio: 8 * time.Hour, HoraFin: 12 * time.Hour,
		})
	}
	svc := NewService(repo, &fakeListadorAdmins{admins: admins}, idSecuencial(), ahoraFija(ahora))

	resultado, err := svc.DisponibilidadDeTodosLosAdmins(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 6 {
		t.Fatalf("esperaba 6 admins, obtuve %d", len(resultado))
	}
	if repo.llamadasBloquesEnLote != 1 || repo.llamadasExcepcionesEnLote != 1 {
		t.Errorf("esperaba una consulta de cada tipo, obtuve %d de bloques y %d de excepciones",
			repo.llamadasBloquesEnLote, repo.llamadasExcepcionesEnLote)
	}
	for _, r := range resultado {
		if !r.DisponibleAhora || len(r.HorarioSemanal) != 1 {
			t.Errorf("%s quedó mal armado: %+v", r.UsuarioID, r)
		}
	}
}

// Un Admin sin horario cargado y sin excepción no aparece en ninguno de los
// dos mapas: tiene que salir en el listado igual, como no disponible.
func TestDisponibilidadDeTodosLosAdmins_AdminSinHorario_ApareceNoDisponible(t *testing.T) {
	repo := nuevoFakeRepo()
	ahora := time.Date(2026, time.March, 9, 10, 0, 0, 0, time.UTC)
	admins := []AdminInfo{{ID: "admin1", Nombre: "Ada", Apellido: "Lovelace"}}
	svc := NewService(repo, &fakeListadorAdmins{admins: admins}, idSecuencial(), ahoraFija(ahora))

	resultado, err := svc.DisponibilidadDeTodosLosAdmins(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 {
		t.Fatalf("esperaba 1 admin, obtuve %d", len(resultado))
	}
	if resultado[0].DisponibleAhora || resultado[0].ExcepcionHoy != nil || len(resultado[0].HorarioSemanal) != 0 {
		t.Errorf("un admin sin horario debería figurar no disponible y sin bloques: %+v", resultado[0])
	}
}

func TestDisponibilidadDeTodosLosAdmins_ErrorDelListador_Propaga(t *testing.T) {
	repo := nuevoFakeRepo()
	errListador := errors.New("auth caído")
	svc := NewService(repo, &fakeListadorAdmins{err: errListador}, idSecuencial(), ahoraFija(time.Now()))

	_, err := svc.DisponibilidadDeTodosLosAdmins(context.Background())

	if !errors.Is(err, errListador) {
		t.Fatalf("esperaba que el error del listador se propague, obtuve %v", err)
	}
}
