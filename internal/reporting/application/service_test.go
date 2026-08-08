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
	historicoPC      map[string]*domain.HistoricoUsoPC
	historicoDocente map[string]*domain.HistoricoUsoDocente
	usoPCs           []domain.ResumenUsoPC
	usoDocentes      []domain.ResumenUsoDocente
	errCalcularPC    error
	errCalcularDoc   error
	errGuardarPC     error
	errGuardarDoc    error
	incidenciasPC    []domain.ResumenIncidenciasPC
	incidenciasCarro []domain.ResumenIncidenciasCarro
	errIncidencias   error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		historicoPC:      make(map[string]*domain.HistoricoUsoPC),
		historicoDocente: make(map[string]*domain.HistoricoUsoDocente),
	}
}

func (r *fakeRepo) GuardarHistoricoUsoPC(ctx context.Context, h *domain.HistoricoUsoPC) error {
	if r.errGuardarPC != nil {
		return r.errGuardarPC
	}
	r.historicoPC[h.ID] = h
	return nil
}
func (r *fakeRepo) GuardarHistoricoUsoDocente(ctx context.Context, h *domain.HistoricoUsoDocente) error {
	if r.errGuardarDoc != nil {
		return r.errGuardarDoc
	}
	r.historicoDocente[h.ID] = h
	return nil
}
func (r *fakeRepo) ListarHistoricoUsoPCPorAnio(ctx context.Context, anio int) ([]*domain.HistoricoUsoPC, error) {
	var resultado []*domain.HistoricoUsoPC
	for _, h := range r.historicoPC {
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
func (r *fakeRepo) CalcularUsoPCsDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoPC, error) {
	if r.errCalcularPC != nil {
		return nil, r.errCalcularPC
	}
	return r.usoPCs, nil
}
func (r *fakeRepo) CalcularUsoDocentesDeCiclo(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	if r.errCalcularDoc != nil {
		return nil, r.errCalcularDoc
	}
	return r.usoDocentes, nil
}

// ── fakes de los puertos hacia inventory/auth ──────────────────────────

type fakeInfoPC struct {
	etiqueta      string
	identificador int
	carroNombre   string
	err           error
}

func (f *fakeInfoPC) EtiquetaYCarroDe(ctx context.Context, pcID string) (string, int, string, error) {
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

var contadorID int

func idSecuencial() string {
	contadorID++
	return "id-" + string(rune('0'+contadorID))
}

func nuevoServicioDeTest(repo Repo) *Service {
	contadorID = 0
	return NewService(repo, &fakeInfoPC{identificador: 27, carroNombre: "Carro 1"}, &fakeInfoUsuario{nombre: "Ada Lovelace"}, idSecuencial)
}

// ── ReporteUsoPCs / ReporteUsoDocentes (en vivo) ────────────────────────

func TestReporteUsoPCs_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoPCs = []domain.ResumenUsoPC{{PCID: "pc1", CantidadReservas: 5, MinutosReservados: 450}}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.ReporteUsoPCs(context.Background(), "ciclo1", nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].PCID != "pc1" {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

func TestReporteUsoPCs_ErrorDelRepo_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCalcularPC = errors.New("base caída")
	svc := nuevoServicioDeTest(repo)

	_, err := svc.ReporteUsoPCs(context.Background(), "ciclo1", nil, nil)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func TestReporteUsoDocentes_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoDocentes = []domain.ResumenUsoDocente{{UsuarioID: "docente1", CantidadReservas: 3, MinutosReservados: 270}}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.ReporteUsoDocentes(context.Background(), "ciclo1", nil, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado) != 1 || resultado[0].UsuarioID != "docente1" {
		t.Fatalf("resultado incorrecto: %+v", resultado)
	}
}

// ── HistoricoUsoPCs / HistoricoUsoDocentes ──────────────────────────────

func TestHistoricoUsoPCs_SoloDelAnioPedido(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.historicoPC["h1"] = &domain.HistoricoUsoPC{ID: "h1", Anio: 2026, PCID: "pc1"}
	repo.historicoPC["h2"] = &domain.HistoricoUsoPC{ID: "h2", Anio: 2025, PCID: "pc2"}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.HistoricoUsoPCs(context.Background(), 2026)

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
	repo.usoPCs = []domain.ResumenUsoPC{
		{PCID: "pc1", CantidadReservas: 10, MinutosReservados: 900},
		{PCID: "pc2", CantidadReservas: 0, MinutosReservados: 0},
	}
	repo.usoDocentes = []domain.ResumenUsoDocente{
		{UsuarioID: "docente1", CantidadReservas: 6, MinutosReservados: 540},
	}
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(repo.historicoPC) != 2 {
		t.Errorf("esperaba 2 históricos de PC guardados, hay %d", len(repo.historicoPC))
	}
	if len(repo.historicoDocente) != 1 {
		t.Errorf("esperaba 1 histórico de docente guardado, hay %d", len(repo.historicoDocente))
	}
	for _, h := range repo.historicoPC {
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

func TestArchivarSnapshotDeCiclo_ErrorCalculandoPCs_SePropagaYNoTocaDocentes(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.errCalcularPC = errors.New("base caída")
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
	if len(repo.historicoDocente) != 0 {
		t.Error("no debería haber guardado nada de docentes si falló antes")
	}
}

func TestArchivarSnapshotDeCiclo_ErrorGuardandoPC_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoPCs = []domain.ResumenUsoPC{{PCID: "pc1", CantidadReservas: 1, MinutosReservados: 60}}
	repo.errGuardarPC = errors.New("constraint violada")
	svc := nuevoServicioDeTest(repo)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func TestArchivarSnapshotDeCiclo_ErrorObteniendoInfoPC_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usoPCs = []domain.ResumenUsoPC{{PCID: "pc1", CantidadReservas: 1, MinutosReservados: 60}}
	contadorID = 0
	svc := NewService(repo, &fakeInfoPC{err: errors.New("inventory caído")}, &fakeInfoUsuario{nombre: "Ada"}, idSecuencial)

	err := svc.ArchivarSnapshotDeCiclo(context.Background(), "ciclo1", 2026)

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

func (r *fakeRepo) CalcularIncidenciasPorPC(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasPC, error) {
	return r.incidenciasPC, r.errIncidencias
}

func (r *fakeRepo) CalcularIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	return r.incidenciasCarro, r.errIncidencias
}
