package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reporting/domain"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	estadoInventario   []domain.EstadoDelInventario
	fueraDeCirculacion []domain.EquipoFueraDeCirculacion
	porCategoria       []domain.ResumenPorCategoriaDeFalla
	historicoEquipo    map[string]*domain.HistoricoUsoEquipo
	historicoDocente   map[string]*domain.HistoricoUsoDocente
	usoEquipos         []domain.ResumenUsoEquipo
	usoDocentes        []domain.ResumenUsoDocente
	errCalcularEquipos error
	errCalcularDoc     error
	errGuardarEquipo   error
	errGuardarDoc      error
	incidenciasEquipo  []domain.ResumenIncidenciasEquipo
	incidenciasCarro   []domain.ResumenIncidenciasCarro
	errIncidencias     error
	borradosSinCuenta  []int
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		historicoEquipo:  make(map[string]*domain.HistoricoUsoEquipo),
		historicoDocente: make(map[string]*domain.HistoricoUsoDocente),
	}
}

func (r *fakeRepo) GuardarHistoricoUsoEquipo(ctx context.Context, h *domain.HistoricoUsoEquipo) error {
	if r.errGuardarEquipo != nil {
		return r.errGuardarEquipo
	}
	r.historicoEquipo[h.ID] = h
	return nil
}
func (r *fakeRepo) GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error {
	if r.errGuardarDoc != nil {
		return r.errGuardarDoc
	}
	r.historicoDocente[h.ID] = h
	return nil
}

func (r *fakeRepo) BorrarHistoricoDocentesSinCuenta(ctx context.Context, anio int) error {
	r.borradosSinCuenta = append(r.borradosSinCuenta, anio)
	for id, h := range r.historicoDocente {
		if h.Anio == anio && h.UsuarioID == nil {
			delete(r.historicoDocente, id)
		}
	}
	return nil
}
func (r *fakeRepo) ListarHistoricoUsoEquipoPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error) {
	var resultado []*domain.HistoricoUsoEquipo
	for _, h := range r.historicoEquipo {
		if h.Anio == anio {
			resultado = append(resultado, h)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) ListarHistoricoUsoDocentePorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error) {
	var resultado []*domain.HistoricoUsoDocente
	for _, h := range r.historicoDocente {
		if h.Anio == anio {
			resultado = append(resultado, h)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) CalcularUsoEquiposDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoEquipo, error) {
	if r.errCalcularEquipos != nil {
		return nil, r.errCalcularEquipos
	}
	return r.usoEquipos, nil
}
func (r *fakeRepo) CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	if r.errCalcularDoc != nil {
		return nil, r.errCalcularDoc
	}
	return r.usoDocentes, nil
}

// ── fakes de los puertos hacia inventory/auth ──────────────────────────

type fakeInfoEquipo struct {
	etiqueta      string
	identificador int
	carroNombre   string
	err           error
}

func (f *fakeInfoEquipo) EtiquetaYCarroDe(ctx context.Context, equipoID string) (string, int, string, error) {
	if f.err != nil {
		return "", 0, "", f.err
	}
	return f.etiqueta, f.identificador, f.carroNombre, nil
}

type fakeInfoUsuario struct {
	nombre string
	err    error
}

func (f *fakeInfoUsuario) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.nombre, nil
}

// ptr es azúcar para los campos opcionales de los resúmenes: UsuarioID es
// *string porque una cuenta eliminada deja la referencia en nil.
func ptr(s string) *string { return &s }

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

func nuevoServicioDeTest(repo Repo) *Service {
	contadorID = 0
	return NewService(repo, &fakeInfoEquipo{identificador: 27, carroNombre: "Carro 1"}, &fakeInfoUsuario{nombre: "Ada Lovelace"}, idSecuencial)
}

// ── ReporteUsoEquipos / ReporteUsoDocentes (en vivo) ────────────────────

func TestReporteUsoEquipos_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoEquipos = []domain.ResumenUsoEquipo{{EquipoID: "pc1", CantidadReservas: 5, MinutosReservados: 450}}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.ReporteUsoEquipos(context.Background(), "ciclo1", nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].EquipoID != "pc1" {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

func TestReporteUsoEquipos_ErrorDelRepo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCalcularEquipos = errors.New("base caída")
	svc := nuevoServicioDeTest(repo)

	_, err := svc.ReporteUsoEquipos(context.Background(), "ciclo1", nil, nil)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func TestReporteUsoDocentes_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoDocentes = []domain.ResumenUsoDocente{{UsuarioID: ptr("docente1"), CantidadReservas: 3, MinutosReservados: 270}}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.ReporteUsoDocentes(context.Background(), "ciclo1", nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].UsuarioID == nil || *resultado[0].UsuarioID != "docente1" {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

// ── HistoricoUsoEquipos / HistoricoUsoDocentes ──────────────────────────

func TestHistoricoUsoEquipos_SoloDelAnioPedido(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.historicoEquipo["h1"] = &domain.HistoricoUsoEquipo{ID: "h1", Anio: 2026, EquipoID: "pc1"}
	repo.historicoEquipo["h2"] = &domain.HistoricoUsoEquipo{ID: "h2", Anio: 2025, EquipoID: "pc2"}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.HistoricoUsoEquipos(context.Background(), 2026)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].ID != "h1" {
		t.Fatalf("esperaba solo h1, obtuve %+v", resultado)
	}
}

// ── ArchivarSnapshotDeCiclo ──────────────────────────────────────────────

func TestArchivarSnapshotDeCiclo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoEquipos = []domain.ResumenUsoEquipo{
		{EquipoID: "pc1", CantidadReservas: 10, MinutosReservados: 900},
		{EquipoID: "pc2", CantidadReservas: 0, MinutosReservados: 0},
	}
	repo.usoDocentes = []domain.ResumenUsoDocente{
		{UsuarioID: ptr("docente1"), CantidadReservas: 6, MinutosReservados: 540},
	}
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(repo.historicoEquipo) != 2 {
		t.Errorf("esperaba 2 históricos de PC guardados, hay %d", len(repo.historicoEquipo))
	}
	if len(repo.historicoDocente) != 1 {
		t.Errorf("esperaba 1 histórico de docente guardado, hay %d", len(repo.historicoDocente))
	}
	for _, h := range repo.historicoEquipo {
		if h.Anio != 2026 {
			t.Errorf("Anio incorrecto: %d", h.Anio)
		}
		if h.IdentificadorSnapshot != 27 || h.CarroNombreSnapshot != "Carro 1" {
			t.Errorf("snapshot de PC incorrecto: %+v", h)
		}
	}
	for _, h := range repo.historicoDocente {
		if h.NombreDocenteSnapshot != "Ada Lovelace" {
			t.Errorf("snapshot de nombre incorrecto: %+v", h)
		}
	}
}

func TestArchivarSnapshotDeCiclo_SinNadaQueAgregar_NoFalla(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo-vacio", 2026)

	if err != nil {
		t.Fatalf("un ciclo sin reservas no debería fallar: %v", err)
	}
}

func TestArchivarSnapshotDeCiclo_ErrorCalculandoEquipos_SePropagaYNoTocaDocentes(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCalcularEquipos = errors.New("base caída")
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
	if len(repo.historicoDocente) != 0 {
		t.Error("no debería haber guardado nada de docentes si falló antes")
	}
}

func TestArchivarSnapshotDeCiclo_ErrorGuardandoEquipo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoEquipos = []domain.ResumenUsoEquipo{{EquipoID: "pc1", CantidadReservas: 1, MinutosReservados: 60}}
	repo.errGuardarEquipo = errors.New("constraint violada")
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func TestArchivarSnapshotDeCiclo_ErrorObteniendoInfoEquipo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoEquipos = []domain.ResumenUsoEquipo{{EquipoID: "pc1", CantidadReservas: 1, MinutosReservados: 60}}
	contadorID = 0
	svc := NewService(repo, &fakeInfoEquipo{err: errors.New("inventory caído")}, &fakeInfoUsuario{nombre: "Ada"}, idSecuencial)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func (r *fakeRepo) CalcularIncidenciasPorEquipo(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasEquipo, error) {
	return r.incidenciasEquipo, r.errIncidencias
}

func (r *fakeRepo) CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	return r.incidenciasCarro, r.errIncidencias
}

func (r *fakeRepo) EstadoDelInventario(ctx context.Context) ([]domain.EstadoDelInventario, error) {
	return r.estadoInventario, nil
}

func (r *fakeRepo) EquiposFueraDeCirculacion(ctx context.Context) ([]domain.EquipoFueraDeCirculacion, error) {
	return r.fueraDeCirculacion, nil
}

func (r *fakeRepo) CalcularIncidenciasPorCategoria(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenPorCategoriaDeFalla, error) {
	return r.porCategoria, nil
}

// Una cuenta eliminada definitivamente (RF-01.9) deja creado_por en NULL, y
// el repositorio devuelve esa fila con UsuarioID nil y el nombre congelado.
// El snapshot la tiene que guardar igual: si no, las horas de esa persona
// desaparecen del año y los totales dejan de cerrar.
func TestArchivarSnapshotDeCiclo_DocenteSinCuenta_SeGuardaConElNombreCongelado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoDocentes = []domain.ResumenUsoDocente{
		{UsuarioID: nil, NombreDocente: "Grace Hopper", CantidadReservas: 4, MinutosReservados: 360},
	}
	svc := nuevoServicioDeTest(repo)

	if err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(repo.historicoDocente) != 1 {
		t.Fatalf("esperaba 1 histórico guardado, hay %d", len(repo.historicoDocente))
	}
	for _, h := range repo.historicoDocente {
		if h.UsuarioID != nil {
			t.Errorf("esperaba UsuarioID nil, obtuve %v", *h.UsuarioID)
		}
		// El nombre NO sale del puerto hacia auth —esa cuenta ya no existe—
		// sino del snapshot que viaja en la reserva.
		if h.NombreDocenteSnapshot != "Grace Hopper" {
			t.Errorf("esperaba el nombre congelado, obtuve %q", h.NombreDocenteSnapshot)
		}
		if h.MinutosTotales != 360 {
			t.Errorf("esperaba 360 minutos, obtuve %d", h.MinutosTotales)
		}
	}
}

// El archivado se puede reintentar (la cascada cruza tres paquetes sin una
// transacción común). Para las filas con usuario_id NULL el ON CONFLICT de
// la base no sirve —Postgres no considera iguales a dos NULL— así que el
// servicio las borra antes de reescribirlas.
func TestArchivarSnapshotDeCiclo_Reintentado_NoDuplicaDocentesSinCuenta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoDocentes = []domain.ResumenUsoDocente{
		{UsuarioID: nil, NombreDocente: "Grace Hopper", CantidadReservas: 4, MinutosReservados: 360},
	}
	svc := nuevoServicioDeTest(repo)

	for i := 0; i < 3; i++ {
		if err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026); err != nil {
			t.Fatalf("intento %d: %v", i+1, err)
		}
	}

	if len(repo.historicoDocente) != 1 {
		t.Fatalf("tres intentos tienen que dejar 1 fila, hay %d", len(repo.historicoDocente))
	}
	if len(repo.borradosSinCuenta) != 3 {
		t.Errorf("esperaba una limpieza por intento, hubo %d", len(repo.borradosSinCuenta))
	}
}
