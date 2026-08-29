package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	ciclos           map[string]*domain.CicloLectivo
	cursos           map[string]*domain.Curso
	materias         map[string]*domain.Materia
	docentesMateria  map[string]*domain.DocenteMateria
	pedidos          map[string]*domain.PedidoDeMateria
	nombresDeUsuario map[string]string

	errCrearCiclo     error
	errCrearCurso     error
	errCrearMateria   error
	errAsignarDocente error
	errArchivar       error
	errClonar         error

	archivarCicloLlamado  bool
	materiasReservables   []MateriaReservable
	filtroDocenteRecibido *string
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		ciclos:          make(map[string]*domain.CicloLectivo),
		cursos:          make(map[string]*domain.Curso),
		materias:        make(map[string]*domain.Materia),
		docentesMateria: make(map[string]*domain.DocenteMateria),
	}
}

func (r *fakeRepo) CrearCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	if r.errCrearCiclo != nil {
		return r.errCrearCiclo
	}
	r.ciclos[c.ID] = c
	return nil
}

func (r *fakeRepo) BuscarCicloActivo(ctx context.Context) (*domain.CicloLectivo, error) {
	for _, c := range r.ciclos {
		if c.Activo {
			return c, nil
		}
	}
	return nil, ErrCicloNoEncontrado
}

func (r *fakeRepo) BuscarCicloPorID(ctx context.Context, id string) (*domain.CicloLectivo, error) {
	c, ok := r.ciclos[id]
	if !ok {
		return nil, ErrCicloNoEncontrado
	}
	return c, nil
}

func (r *fakeRepo) GuardarCiclo(ctx context.Context, c *domain.CicloLectivo) error {
	r.ciclos[c.ID] = c
	return nil
}

func (r *fakeRepo) ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error) {
	var resultado []*domain.CicloLectivo
	for _, c := range r.ciclos {
		if filtroArchivado != nil && c.Archivado != *filtroArchivado {
			continue
		}
		resultado = append(resultado, c)
	}
	return resultado, nil
}

func (r *fakeRepo) CrearCurso(ctx context.Context, c *domain.Curso) error {
	if r.errCrearCurso != nil {
		return r.errCrearCurso
	}
	r.cursos[c.ID] = c
	return nil
}

func (r *fakeRepo) BuscarCursoPorID(ctx context.Context, id string) (*domain.Curso, error) {
	c, ok := r.cursos[id]
	if !ok {
		return nil, ErrCursoNoEncontrado
	}
	return c, nil
}

func (r *fakeRepo) GuardarCurso(ctx context.Context, c *domain.Curso) error {
	r.cursos[c.ID] = c
	return nil
}

func (r *fakeRepo) EliminarCurso(ctx context.Context, id string) error {
	if _, ok := r.cursos[id]; !ok {
		return ErrCursoNoEncontrado
	}
	delete(r.cursos, id)
	return nil
}

func (r *fakeRepo) ListarCursosPorCiclo(ctx context.Context, cicloID string) ([]*domain.Curso, error) {
	var resultado []*domain.Curso
	for _, c := range r.cursos {
		if c.CicloLectivoID == cicloID {
			resultado = append(resultado, c)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CrearMateria(ctx context.Context, m *domain.Materia) error {
	if r.errCrearMateria != nil {
		return r.errCrearMateria
	}
	r.materias[m.ID] = m
	return nil
}

func (r *fakeRepo) BuscarMateriaPorID(ctx context.Context, id string) (*domain.Materia, error) {
	m, ok := r.materias[id]
	if !ok {
		return nil, ErrMateriaNoEncontrada
	}
	return m, nil
}

func (r *fakeRepo) GuardarMateria(ctx context.Context, m *domain.Materia) error {
	r.materias[m.ID] = m
	return nil
}

func (r *fakeRepo) EliminarMateria(ctx context.Context, id string) error {
	if _, ok := r.materias[id]; !ok {
		return ErrMateriaNoEncontrada
	}
	delete(r.materias, id)
	return nil
}

func (r *fakeRepo) ListarMateriasPorCurso(ctx context.Context, cursoID string) ([]*domain.Materia, error) {
	var resultado []*domain.Materia
	for _, m := range r.materias {
		if m.CursoID == cursoID {
			resultado = append(resultado, m)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) AsignarDocente(ctx context.Context, dm *domain.DocenteMateria) error {
	if r.errAsignarDocente != nil {
		return r.errAsignarDocente
	}
	r.docentesMateria[dm.ID] = dm
	return nil
}

func (r *fakeRepo) BuscarDocenteMateria(ctx context.Context, id string) (*domain.DocenteMateria, error) {
	dm, ok := r.docentesMateria[id]
	if !ok {
		return nil, ErrDocenteMateriaNoEncontrado
	}
	return dm, nil
}

func (r *fakeRepo) GuardarDocenteMateria(ctx context.Context, dm *domain.DocenteMateria) error {
	if _, ok := r.docentesMateria[dm.ID]; !ok {
		return ErrDocenteMateriaNoEncontrado
	}
	r.docentesMateria[dm.ID] = dm
	return nil
}

func (r *fakeRepo) RemoverDocenteMateria(ctx context.Context, id string) error {
	if _, ok := r.docentesMateria[id]; !ok {
		return ErrDocenteMateriaNoEncontrado
	}
	delete(r.docentesMateria, id)
	return nil
}

func (r *fakeRepo) ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error) {
	var resultado []*domain.DocenteMateria
	for _, dm := range r.docentesMateria {
		if dm.MateriaID == materiaID {
			resultado = append(resultado, dm)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) ListarMateriasReservables(ctx context.Context, soloDelDocente *string) ([]MateriaReservable, error) {
	r.filtroDocenteRecibido = soloDelDocente
	return r.materiasReservables, nil
}

func (r *fakeRepo) ArchivarCiclo(ctx context.Context, cicloID string) error {
	r.archivarCicloLlamado = true
	if r.errArchivar != nil {
		return r.errArchivar
	}
	c, ok := r.ciclos[cicloID]
	if !ok {
		return ErrCicloNoEncontrado
	}
	c.Activo = false
	c.Archivado = true
	for _, curso := range r.cursos {
		if curso.CicloLectivoID == cicloID {
			curso.Archivado = true
		}
	}
	return nil
}

func (r *fakeRepo) ClonarCicloA(ctx context.Context, cicloOrigenID string, nuevoCiclo *domain.CicloLectivo) (int, int, error) {
	if r.errClonar != nil {
		return 0, 0, r.errClonar
	}
	r.ciclos[nuevoCiclo.ID] = nuevoCiclo

	cursosClonados := 0
	materiasClonadas := 0
	for _, curso := range r.cursos {
		if curso.CicloLectivoID == cicloOrigenID {
			cursosClonados++
			for _, m := range r.materias {
				if m.CursoID == curso.ID {
					materiasClonadas++
				}
			}
		}
	}
	return cursosClonados, materiasClonadas, nil
}

// ── fakeValidadorUsuario / fakeValidadorReservas ───────────────────────

type fakeValidadorUsuario struct {
	valido bool
	err    error
	// validoPorUsuario permite distinguir un docente de otro, que es lo que
	// necesita la cascada de RF-02.8: "queda otro docente" solo cuenta a los que
	// siguen APROBADA. Si está en nil se usa `valido` para todos.
	validoPorUsuario map[string]bool
}

func (f *fakeValidadorUsuario) ExisteYAprobado(ctx context.Context, usuarioID string) (bool, error) {
	if f.validoPorUsuario != nil {
		return f.validoPorUsuario[usuarioID], f.err
	}
	return f.valido, f.err
}

type fakeValidadorReservas struct {
	tieneReservasCurso   bool
	tieneReservasMateria bool
	tieneReservasCiclo   bool
	err                  error
}

func (f *fakeValidadorReservas) TieneReservasCurso(ctx context.Context, cursoID string) (bool, error) {
	return f.tieneReservasCurso, f.err
}
func (f *fakeValidadorReservas) TieneReservasMateria(ctx context.Context, materiaID string) (bool, error) {
	return f.tieneReservasMateria, f.err
}
func (f *fakeValidadorReservas) TieneReservasDeCiclo(ctx context.Context, cicloID string) (bool, error) {
	return f.tieneReservasCiclo, f.err
}

type fakeArchivadorHistorico struct {
	err                error
	errAlEliminar      error
	llamadoConCicloID  string
	llamadoConAnio     int
	vecesLlamado       int
	reservasEliminadas int
	// pasos registra el orden real de la cascada, para poder afirmar que
	// el borrado irreversible va último.
	pasos []string
}

func (f *fakeArchivadorHistorico) GuardarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error {
	f.vecesLlamado++
	f.llamadoConCicloID = cicloID
	f.llamadoConAnio = anio
	f.pasos = append(f.pasos, "snapshot")
	return f.err
}

func (f *fakeArchivadorHistorico) EliminarReservasDeCiclo(ctx context.Context, cicloID string) error {
	f.pasos = append(f.pasos, "eliminar-reservas")
	if f.errAlEliminar != nil {
		return f.errAlEliminar
	}
	f.reservasEliminadas++
	return nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

// fakeDatosDeUsuario resuelve contactos de mentira: los tests de pedidos
// verifican qué evento sale, no de dónde salió el nombre.
type fakeDatosDeUsuario struct {
	contactos map[string]ContactoDeDocente
}

func (f *fakeDatosDeUsuario) Contacto(_ context.Context, usuarioID string) (ContactoDeDocente, error) {
	if c, ok := f.contactos[usuarioID]; ok {
		return c, nil
	}
	return ContactoDeDocente{UsuarioID: usuarioID, Nombre: "Docente " + usuarioID,
		Email: usuarioID + "@escuela.edu.ar"}, nil
}

func (f *fakeDatosDeUsuario) Contactos(ctx context.Context, ids []string) ([]ContactoDeDocente, error) {
	var r []ContactoDeDocente
	for _, id := range ids {
		c, _ := f.Contacto(ctx, id)
		r = append(r, c)
	}
	return r, nil
}

// relojDeTest: los pedidos llevan fecha, y una fija hace que los tests no
// dependan de cuándo se corren.
func relojDeTest() time.Time {
	return time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
}

func nuevoServicioDeTest(repo Repo, validadorUsuario ValidadorUsuario, validadorReservas ValidadorReservas) *Service {
	contadorID = 0
	return NewService(repo, validadorUsuario, validadorReservas, &fakeArchivadorHistorico{},
		&fakeCanceladorReservas{}, &fakeDatosDeUsuario{}, idSecuencial, relojDeTest,
		eventbus.NewInMemoryEventBus())
}

func nuevoServicioConArchivador(repo Repo, validadorUsuario ValidadorUsuario, validadorReservas ValidadorReservas, archivador ArchivadorHistorico) *Service {
	contadorID = 0
	return NewService(repo, validadorUsuario, validadorReservas, archivador,
		&fakeCanceladorReservas{}, &fakeDatosDeUsuario{}, idSecuencial, relojDeTest,
		eventbus.NewInMemoryEventBus())
}

// fakeCanceladorReservas registra qué materias se le pidió limpiar, que es
// justamente lo que verifica la cascada de RemoverDocenteMateria.
type fakeCanceladorReservas struct {
	materiasCanceladas []string
	motivos            []string
	canceladas         int
	err                error
}

func (f *fakeCanceladorReservas) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.materiasCanceladas = append(f.materiasCanceladas, materiaID)
	f.motivos = append(f.motivos, motivo)
	return f.canceladas, nil
}

func servicioSimple(repo Repo) *Service {
	return nuevoServicioDeTest(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{})
}

// servicioConCancelador expone el cancelador para poder verificar la
// cascada de RF-02.8 al quitar al último docente de una materia.
func servicioConCancelador(repo Repo, cancelador *fakeCanceladorReservas, validadorUsuario ValidadorUsuario) *Service {
	contadorID = 0
	return NewService(repo, validadorUsuario, &fakeValidadorReservas{}, &fakeArchivadorHistorico{},
		cancelador, &fakeDatosDeUsuario{}, idSecuencial, relojDeTest,
		eventbus.NewInMemoryEventBus())
}

// ── CrearCiclo ──────────────────────────────────────────────────────────

func TestCrearCiclo_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	c, err := svc.CrearCiclo(context.Background(), 2026)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !c.Activo {
		t.Error("un ciclo nuevo debería quedar activo")
	}
}

func TestCrearCiclo_YaHayUnoActivo_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["existente"] = &domain.CicloLectivo{ID: "existente", Anio: 2025, Activo: true}
	svc := servicioSimple(repo)

	_, err := svc.CrearCiclo(context.Background(), 2026)

	if !errors.Is(err, ErrYaHayCicloActivo) {
		t.Fatalf("esperaba ErrYaHayCicloActivo, obtuve %v", err)
	}
}

func TestCrearCiclo_AnioInvalido_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearCiclo(context.Background(), 1500)

	if !errors.Is(err, domain.ErrAnioInvalido) {
		t.Fatalf("esperaba ErrAnioInvalido, obtuve %v", err)
	}
}

// ── ArchivarYClonar ─────────────────────────────────────────────────────

func TestArchivarYClonar_SinClonar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	svc := servicioSimple(repo)

	res, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.NuevoCicloID != nil {
		t.Error("sin pedir clonar, no debería haber nuevo ciclo")
	}
	if !repo.ciclos["c1"].Archivado || repo.ciclos["c1"].Activo {
		t.Error("el ciclo original debería quedar archivado y no activo")
	}
}

func TestArchivarYClonar_ConClonar_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	repo.cursos["curso1"] = &domain.Curso{ID: "curso1", CicloLectivoID: "c1", Nombre: "1°A", Activo: true}
	repo.materias["m1"] = &domain.Materia{ID: "m1", CursoID: "curso1", Nombre: "Matemáticas", Activo: true}
	svc := servicioSimple(repo)

	anio2026 := 2026
	res, err := svc.ArchivarYClonar(context.Background(), "c1", &anio2026)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.NuevoCicloID == nil {
		t.Fatal("esperaba un nuevo ciclo clonado")
	}
	if res.CursosClonados != 1 || res.MateriasClonadas != 1 {
		t.Errorf("conteo de clonado incorrecto: %+v", res)
	}
}

func TestArchivarYClonar_CicloYaArchivado_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: false, Archivado: true}
	svc := servicioSimple(repo)

	_, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if !errors.Is(err, domain.ErrCicloYaArchivado) {
		t.Fatalf("esperaba ErrCicloYaArchivado, obtuve %v", err)
	}
}

// El caso que tenía consecuencias de verdad: el Admin pide clonar a un año
// que ya existe.
func TestArchivarYClonar_AnioDestinoOcupado_FallaSinTocarNada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	repo.ciclos["c2"] = &domain.CicloLectivo{ID: "c2", Anio: 2026, Activo: false}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	anio2026 := 2026
	_, err := svc.ArchivarYClonar(context.Background(), "c1", &anio2026)

	if !errors.Is(err, ErrCicloYaTieneAnio) {
		t.Fatalf("esperaba ErrCicloYaTieneAnio, obtuve %v", err)
	}
	// Lo que importa no es el error, es que no se haya destruido nada.
	if repo.ciclos["c1"].Archivado {
		t.Error("el ciclo viejo NO tiene que quedar archivado si el clonado no puede hacerse")
	}
	if len(archivador.pasos) != 0 {
		t.Errorf("no se tendría que haber tocado nada, pero corrió: %v", archivador.pasos)
	}
}

// Un año fuera de rango tiene el mismo problema: se validaba al final.
func TestArchivarYClonar_AnioInvalido_FallaSinTocarNada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	disparatado := 99999
	_, err := svc.ArchivarYClonar(context.Background(), "c1", &disparatado)

	if !errors.Is(err, domain.ErrAnioInvalido) {
		t.Fatalf("esperaba ErrAnioInvalido, obtuve %v", err)
	}
	if repo.ciclos["c1"].Archivado || len(archivador.pasos) != 0 {
		t.Error("no se tendría que haber tocado nada")
	}
}

// Si el clonado falla por algo transitorio, el archivado ya se consumó.
func TestArchivarYClonar_ReintentoCompletaElClonadoPendiente(t *testing.T) {
	repo := nuevoFakeRepo()
	// Como quedó tras un primer intento que archivó y borró, pero no clonó.
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: false, Archivado: true}
	repo.cursos["curso1"] = &domain.Curso{ID: "curso1", CicloLectivoID: "c1", Nombre: "1°A", Activo: true}
	repo.materias["m1"] = &domain.Materia{ID: "m1", CursoID: "curso1", Nombre: "Matemáticas", Activo: true}
	// Sin limpieza pendiente: el borrado del intento anterior sí terminó.
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true},
		&fakeValidadorReservas{}, &fakeArchivadorHistorico{})

	anio2026 := 2026
	res, err := svc.ArchivarYClonar(context.Background(), "c1", &anio2026)

	if err != nil {
		t.Fatalf("el reintento tiene que completar el clonado: %v", err)
	}
	if res.NuevoCicloID == nil || res.CursosClonados != 1 || res.MateriasClonadas != 1 {
		t.Errorf("esperaba el clonado completo, obtuve %+v", res)
	}
}

// Y sin clonado pendiente, archivar dos veces sigue siendo un error: el
// reintento habilita terminar lo que faltó, no repetir la operación.
func TestArchivarYClonar_YaArchivadoSinNadaPendiente_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: false, Archivado: true}
	svc := servicioSimple(repo)

	_, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if !errors.Is(err, domain.ErrCicloYaArchivado) {
		t.Fatalf("esperaba ErrCicloYaArchivado, obtuve %v", err)
	}
}

func TestArchivarYClonar_CicloInexistente_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.ArchivarYClonar(context.Background(), "no-existe", nil)

	if !errors.Is(err, ErrCicloNoEncontrado) {
		t.Fatalf("esperaba ErrCicloNoEncontrado, obtuve %v", err)
	}
}

func TestArchivarYClonar_LlamaAlArchivadorConCicloIDYAnio(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	_, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if archivador.vecesLlamado != 1 {
		t.Fatalf("esperaba que se llame una vez, se llamó %d", archivador.vecesLlamado)
	}
	if archivador.llamadoConCicloID != "c1" || archivador.llamadoConAnio != 2025 {
		t.Errorf("se llamó con cicloID=%q anio=%d, esperaba c1/2025", archivador.llamadoConCicloID, archivador.llamadoConAnio)
	}
}

func TestArchivarYClonar_ErrorEnCascada_NoArchivaElCiclo(t *testing.T) {
	// Si la cascada hacia reporting/reservation falla, la persistencia real
	// (repo.ArchivarCiclo) no debe llegar a invocarse — mejor no archivar que
	// archivar sin el histórico guardado.
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: true}
	archivador := &fakeArchivadorHistorico{err: errors.New("reporting/reservation caídos")}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	_, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if err == nil {
		t.Fatal("esperaba que el error de la cascada se propague")
	}
	if repo.archivarCicloLlamado {
		t.Error("la persistencia real (ArchivarCiclo) no debería haberse invocado si la cascada falló antes")
	}
}

// ── Curso ───────────────────────────────────────────────────────────────

func TestCrearCurso_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	c, err := svc.CrearCurso(context.Background(), "ciclo1", "1°A")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Nombre != "1°A" {
		t.Errorf("nombre incorrecto: %s", c.Nombre)
	}
}

func TestCrearCurso_NombreInvalido_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearCurso(context.Background(), "ciclo1", "primero A")

	if !errors.Is(err, domain.ErrNombreCursoInvalido) {
		t.Fatalf("esperaba ErrNombreCursoInvalido, obtuve %v", err)
	}
}

func TestEditarCurso_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.cursos["curso1"] = &domain.Curso{ID: "curso1", Nombre: "1°A", Activo: true}
	svc := servicioSimple(repo)

	err := svc.EditarCurso(context.Background(), "curso1", "2°B")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.cursos["curso1"].Nombre != "2°B" {
		t.Errorf("el nombre no se actualizó: %s", repo.cursos["curso1"].Nombre)
	}
}

func TestEditarCurso_NoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	err := svc.EditarCurso(context.Background(), "no-existe", "2°B")

	if !errors.Is(err, ErrCursoNoEncontrado) {
		t.Fatalf("esperaba ErrCursoNoEncontrado, obtuve %v", err)
	}
}

func TestEliminarCurso_SinReservas_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.cursos["curso1"] = &domain.Curso{ID: "curso1", Nombre: "1°A"}
	svc := nuevoServicioDeTest(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{tieneReservasCurso: false})

	err := svc.EliminarCurso(context.Background(), "curso1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestEliminarCurso_ConReservas_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.cursos["curso1"] = &domain.Curso{ID: "curso1", Nombre: "1°A"}
	svc := nuevoServicioDeTest(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{tieneReservasCurso: true})

	err := svc.EliminarCurso(context.Background(), "curso1")

	if !errors.Is(err, ErrCursoConReservas) {
		t.Fatalf("esperaba ErrCursoConReservas, obtuve %v", err)
	}
	if _, existe := repo.cursos["curso1"]; !existe {
		t.Error("el curso no debería haberse eliminado si tiene reservas")
	}
}

// ── Materia ─────────────────────────────────────────────────────────────

func TestCrearMateria_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	m, err := svc.CrearMateria(context.Background(), "curso1", "Matemáticas")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if m.Nombre != "Matemáticas" {
		t.Errorf("nombre incorrecto: %s", m.Nombre)
	}
}

func TestEliminarMateria_ConReservas_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	svc := nuevoServicioDeTest(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{tieneReservasMateria: true})

	err := svc.EliminarMateria(context.Background(), "m1")

	if !errors.Is(err, ErrMateriaConReservas) {
		t.Fatalf("esperaba ErrMateriaConReservas, obtuve %v", err)
	}
}

// ── DocenteMateria ──────────────────────────────────────────────────────

func TestAsignarDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	svc := servicioSimple(repo)

	dm, err := svc.AsignarDocente(context.Background(), "m1", "usuario1", domain.RolTitular)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if dm.UsuarioID != "usuario1" || dm.Rol != domain.RolTitular {
		t.Errorf("datos incorrectos: %+v", dm)
	}
}

func TestAsignarDocente_MateriaNoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.AsignarDocente(context.Background(), "no-existe", "usuario1", domain.RolTitular)

	if !errors.Is(err, ErrMateriaNoEncontrada) {
		t.Fatalf("esperaba ErrMateriaNoEncontrada, obtuve %v", err)
	}
}

func TestAsignarDocente_UsuarioNoValido_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	svc := nuevoServicioDeTest(repo, &fakeValidadorUsuario{valido: false}, &fakeValidadorReservas{})

	_, err := svc.AsignarDocente(context.Background(), "m1", "usuario-pendiente", domain.RolTitular)

	if !errors.Is(err, ErrUsuarioNoValidoParaAsignar) {
		t.Fatalf("esperaba ErrUsuarioNoValidoParaAsignar, obtuve %v", err)
	}
}

func TestAsignarDocente_ErrorDelValidador_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.materias["m1"] = &domain.Materia{ID: "m1", Nombre: "Matemáticas"}
	svc := nuevoServicioDeTest(repo, &fakeValidadorUsuario{err: errors.New("auth caído")}, &fakeValidadorReservas{})

	_, err := svc.AsignarDocente(context.Background(), "m1", "usuario1", domain.RolTitular)

	if err == nil {
		t.Fatal("esperaba que el error del validador se propague")
	}
}

func TestCambiarRolDocente_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1", Rol: domain.RolTitular}
	svc := servicioSimple(repo)

	dm, err := svc.CambiarRolDocente(context.Background(), "dm1", domain.RolSuplente)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if dm.Rol != domain.RolSuplente {
		t.Errorf("esperaba SUPLENTE, obtuve %s", dm.Rol)
	}
	if repo.docentesMateria["dm1"].Rol != domain.RolSuplente {
		t.Error("el cambio no se persistió")
	}
}

func TestCambiarRolDocente_NoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CambiarRolDocente(context.Background(), "no-existe", domain.RolSuplente)

	if !errors.Is(err, ErrDocenteMateriaNoEncontrado) {
		t.Fatalf("esperaba ErrDocenteMateriaNoEncontrado, obtuve %v", err)
	}
}

// El motivo por el que CambiarRolDocente existe: el otro camino para corregir
// un rol —quitar y volver a asignar— pasa por la cascada de RF-02.8 y, si el
// docente es el único de la materia, le cancela las reservas futuras.
func TestCambiarRolDocente_NoDisparaLaCascadaDeReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1", Rol: domain.RolTitular}
	cancelador := &fakeCanceladorReservas{canceladas: 4}
	svc := servicioConCancelador(repo, cancelador, &fakeValidadorUsuario{valido: true})

	if _, err := svc.CambiarRolDocente(context.Background(), "dm1", domain.RolSuplente); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(cancelador.materiasCanceladas) != 0 {
		t.Errorf("no debería haber cancelado reservas, canceló las de %v", cancelador.materiasCanceladas)
	}
	if _, existe := repo.docentesMateria["dm1"]; !existe {
		t.Error("la asignación tiene que seguir existiendo")
	}
}

func TestRemoverDocenteMateria_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1"}
	svc := servicioSimple(repo)

	_, err := svc.RemoverDocenteMateria(context.Background(), "dm1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if _, existe := repo.docentesMateria["dm1"]; existe {
		t.Error("la asignación debería haberse removido")
	}
}

// ── Cascada al quitar al último docente (RF-02.8) ──────────────────────
// auth.DarDeBaja ya hacía todo esto; quitar la asignación llegaba al mismo
// estado —materia sin nadie a cargo, con reservas futuras vivas— sin ninguna
// de las consecuencias.

func TestRemoverDocenteMateria_EraElUnico_CancelaLasReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1"}
	cancelador := &fakeCanceladorReservas{canceladas: 4}
	svc := servicioConCancelador(repo, cancelador, &fakeValidadorUsuario{valido: true})

	canceladas, err := svc.RemoverDocenteMateria(context.Background(), "dm1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 4 {
		t.Errorf("esperaba 4 reservas canceladas, obtuve %d", canceladas)
	}
	if len(cancelador.materiasCanceladas) != 1 || cancelador.materiasCanceladas[0] != "m1" {
		t.Fatalf("esperaba la cascada sobre m1, obtuve %v", cancelador.materiasCanceladas)
	}
	if _, existe := repo.docentesMateria["dm1"]; existe {
		t.Error("la asignación debería haberse removido igual")
	}
}

func TestRemoverDocenteMateria_QuedaOtroDocente_NoCancelaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1"}
	repo.docentesMateria["dm2"] = &domain.DocenteMateria{ID: "dm2", UsuarioID: "docente2", MateriaID: "m1"}
	cancelador := &fakeCanceladorReservas{canceladas: 4}
	svc := servicioConCancelador(repo, cancelador, &fakeValidadorUsuario{valido: true})

	canceladas, err := svc.RemoverDocenteMateria(context.Background(), "dm1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 0 || len(cancelador.materiasCanceladas) != 0 {
		t.Fatalf("la materia sigue teniendo docente: no debería cancelarse nada (%d, %v)",
			canceladas, cancelador.materiasCanceladas)
	}
}

// Un docente que quedó en BAJA sigue teniendo su fila docente_materia hasta
// que la cascada de auth la borre.
func TestRemoverDocenteMateria_ElOtroDocenteNoEstaActivo_CancelaIgual(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1"}
	repo.docentesMateria["dm2"] = &domain.DocenteMateria{ID: "dm2", UsuarioID: "docenteDeBaja", MateriaID: "m1"}
	cancelador := &fakeCanceladorReservas{canceladas: 2}
	// Solo docente1 sigue aprobado.
	svc := servicioConCancelador(repo, cancelador,
		&fakeValidadorUsuario{validoPorUsuario: map[string]bool{"docente1": true}})

	canceladas, err := svc.RemoverDocenteMateria(context.Background(), "dm1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 2 {
		t.Errorf("esperaba 2 reservas canceladas, obtuve %d", canceladas)
	}
}

// Mismo criterio que auth.DarDeBaja: si la cascada falla, el vínculo se
// conserva.
func TestRemoverDocenteMateria_SiFallaLaCascada_ConservaLaAsignacion(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.docentesMateria["dm1"] = &domain.DocenteMateria{ID: "dm1", UsuarioID: "docente1", MateriaID: "m1"}
	cancelador := &fakeCanceladorReservas{err: errors.New("reservation caído")}
	svc := servicioConCancelador(repo, cancelador, &fakeValidadorUsuario{valido: true})

	_, err := svc.RemoverDocenteMateria(context.Background(), "dm1")

	if err == nil {
		t.Fatal("esperaba que el error de la cascada se propague")
	}
	if _, existe := repo.docentesMateria["dm1"]; !existe {
		t.Error("la asignación tendría que conservarse para poder reintentar")
	}
}

// ── Orden de la cascada de archivado (RF-02.4) ─────────────────────────

// El borrado físico de reservas es el único paso irreversible de la cascada,
// así que tiene que ser el ÚLTIMO. Si se ejecutara antes de archivar el ciclo
// (como estaba originalmente), un fallo al archivar dejaría el año entero de
// reservas borrado y el ciclo sin archivar, sin forma de recuperarlo.
func TestArchivarYClonar_ElBorradoIrreversibleVaUltimo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo1"] = &domain.CicloLectivo{ID: "ciclo1", Anio: 2026, Activo: true}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	if _, err := svc.ArchivarYClonar(context.Background(), "ciclo1", nil); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(archivador.pasos) != 2 || archivador.pasos[0] != "snapshot" || archivador.pasos[1] != "eliminar-reservas" {
		t.Fatalf("orden inesperado de la cascada: %v", archivador.pasos)
	}
}

func TestArchivarYClonar_SiFallaElArchivado_NoBorraNingunaReserva(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo1"] = &domain.CicloLectivo{ID: "ciclo1", Anio: 2026, Activo: true}
	repo.errArchivar = errors.New("la base se cayó justo acá")
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	if _, err := svc.ArchivarYClonar(context.Background(), "ciclo1", nil); err == nil {
		t.Fatal("se esperaba el error de archivado")
	}

	for _, paso := range archivador.pasos {
		if paso == "eliminar-reservas" {
			t.Fatal("se borraron reservas pese a que el archivado falló — pérdida de datos irrecuperable")
		}
	}
}

// Si lo único que falla es el borrado, el ciclo ya quedó archivado y el
// snapshot guardado: no se perdió nada y reintentar completa la limpieza.
func TestArchivarYClonar_SiFallaElBorrado_ElCicloYaQuedoArchivadoConSuSnapshot(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo1"] = &domain.CicloLectivo{ID: "ciclo1", Anio: 2026, Activo: true}
	archivador := &fakeArchivadorHistorico{errAlEliminar: errors.New("timeout")}
	svc := nuevoServicioConArchivador(repo, &fakeValidadorUsuario{valido: true}, &fakeValidadorReservas{}, archivador)

	if _, err := svc.ArchivarYClonar(context.Background(), "ciclo1", nil); err == nil {
		t.Fatal("se esperaba el error de borrado")
	}

	if archivador.vecesLlamado != 1 {
		t.Errorf("el snapshot debería haberse guardado antes del borrado")
	}
	if !repo.ciclos["ciclo1"].Archivado {
		t.Errorf("el ciclo debería haber quedado archivado — el borrado es el único paso que falta y es reintentable")
	}
}

// ── RF-04.1: materias reservables ──────────────────────────────────────

func TestListarMateriasReservables_UnDocenteSoloVeLasSuyas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	if _, err := svc.ListarMateriasReservables(context.Background(), "docente1", false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.filtroDocenteRecibido == nil || *repo.filtroDocenteRecibido != "docente1" {
		t.Errorf("un docente debe consultarse filtrado por sí mismo, se filtró por %v", repo.filtroDocenteRecibido)
	}
}

func TestListarMateriasReservables_UnAdminLasVeTodas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	if _, err := svc.ListarMateriasReservables(context.Background(), "admin1", true); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.filtroDocenteRecibido != nil {
		t.Errorf("un Admin puede reservar para cualquier materia, no debe filtrarse por docente")
	}
}

// ── Reintento del archivado (RF-02.4) ──────────────────────────────────

// El archivado cruza tres paquetes sin una transacción que los abarque, así
// que puede quedar a mitad de camino: ciclo marcado, snapshot guardado y
// reservas todavía sin borrar.

func TestArchivarYClonar_YaArchivadoConReservasPendientes_CompletaLaLimpieza(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: false, Archivado: true}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo,
		&fakeValidadorUsuario{valido: true},
		&fakeValidadorReservas{tieneReservasCiclo: true}, // el intento anterior murió antes de borrarlas
		archivador)

	if _, err := svc.ArchivarYClonar(context.Background(), "c1", nil); err != nil {
		t.Fatalf("un archivado interrumpido tiene que poder completarse, obtuve: %v", err)
	}

	if archivador.pasos[len(archivador.pasos)-1] != "eliminar-reservas" {
		t.Errorf("el borrado de reservas debería haberse ejecutado último, pasos: %v", archivador.pasos)
	}
}

func TestArchivarYClonar_YaArchivadoYSinNadaPendiente_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["c1"] = &domain.CicloLectivo{ID: "c1", Anio: 2025, Activo: false, Archivado: true}
	archivador := &fakeArchivadorHistorico{}
	svc := nuevoServicioConArchivador(repo,
		&fakeValidadorUsuario{valido: true},
		&fakeValidadorReservas{tieneReservasCiclo: false}, // ya terminó: archivar de nuevo es un error del Admin
		archivador)

	_, err := svc.ArchivarYClonar(context.Background(), "c1", nil)

	if !errors.Is(err, domain.ErrCicloYaArchivado) {
		t.Fatalf("esperaba ErrCicloYaArchivado, obtuve %v", err)
	}
	if archivador.vecesLlamado != 0 {
		t.Errorf("no debería haber recalculado el snapshot: pisaría el bueno con los datos de un ciclo ya vaciado")
	}
}

// ── Pedidos para dictar una materia ───────────────────────────────────── El
// fake los guarda en un mapa; el orden de los listados no importa para estos
// tests, que verifican reglas y no presentación.

func (r *fakeRepo) CrearPedido(_ context.Context, p *domain.PedidoDeMateria) error {
	if r.pedidos == nil {
		r.pedidos = map[string]*domain.PedidoDeMateria{}
	}
	r.pedidos[p.ID] = p
	return nil
}

func (r *fakeRepo) BuscarPedidoPorID(_ context.Context, id string) (*domain.PedidoDeMateria, error) {
	p, ok := r.pedidos[id]
	if !ok {
		return nil, domain.ErrPedidoNoExiste
	}
	return p, nil
}

func (r *fakeRepo) GuardarPedido(_ context.Context, p *domain.PedidoDeMateria) error {
	if r.pedidos == nil {
		r.pedidos = map[string]*domain.PedidoDeMateria{}
	}
	r.pedidos[p.ID] = p
	return nil
}

// detallar hace lo mismo que el LEFT JOIN del repositorio real: le pega el
// nombre de la materia y el del curso, y los deja vacíos cuando la materia
// todavía no existe.
func (r *fakeRepo) detallar(p *domain.PedidoDeMateria) *PedidoDetallado {
	d := &PedidoDetallado{Pedido: p}
	d.DocenteNombre = r.nombresDeUsuario[p.UsuarioID]
	if p.MateriaID == nil {
		return d
	}
	m, hay := r.materias[*p.MateriaID]
	if !hay {
		return d
	}
	d.MateriaNombre = m.Nombre
	if c, hay := r.cursos[m.CursoID]; hay {
		d.CursoNombre = c.Nombre
	}
	return d
}

func (r *fakeRepo) ListarPedidos(_ context.Context, soloPendientes bool) ([]*PedidoDetallado, error) {
	var out []*PedidoDetallado
	for _, p := range r.pedidos {
		if soloPendientes && p.Estado != domain.PedidoPendiente {
			continue
		}
		out = append(out, r.detallar(p))
	}
	return out, nil
}

func (r *fakeRepo) ListarPedidosDeUsuario(_ context.Context, usuarioID string) ([]*PedidoDetallado, error) {
	var out []*PedidoDetallado
	for _, p := range r.pedidos {
		if p.UsuarioID == usuarioID {
			out = append(out, r.detallar(p))
		}
	}
	return out, nil
}

func (r *fakeRepo) ContarPedidosPendientes(_ context.Context) (int, error) {
	n := 0
	for _, p := range r.pedidos {
		if p.Estado == domain.PedidoPendiente {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) TienePedidoAbierto(_ context.Context, usuarioID, materiaID string) (bool, error) {
	for _, p := range r.pedidos {
		if p.UsuarioID == usuarioID && p.Estado == domain.PedidoPendiente &&
			p.MateriaID != nil && *p.MateriaID == materiaID {
			return true, nil
		}
	}
	return false, nil
}
