package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	carros      map[string]*domain.Carro
	pcs         map[string]*domain.PC
	incidencias map[string]*domain.Incidencia
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		carros:      make(map[string]*domain.Carro),
		pcs:         make(map[string]*domain.PC),
		incidencias: make(map[string]*domain.Incidencia),
	}
}

func (r *fakeRepo) CrearCarro(ctx context.Context, c *domain.Carro) error {
	r.carros[c.ID] = c
	return nil
}
func (r *fakeRepo) BuscarCarroPorID(ctx context.Context, id string) (*domain.Carro, error) {
	c, ok := r.carros[id]
	if !ok {
		return nil, ErrCarroNoEncontrado
	}
	return c, nil
}
func (r *fakeRepo) GuardarCarro(ctx context.Context, c *domain.Carro) error {
	r.carros[c.ID] = c
	return nil
}
func (r *fakeRepo) ListarCarros(ctx context.Context) ([]*domain.Carro, error) {
	var resultado []*domain.Carro
	for _, c := range r.carros {
		resultado = append(resultado, c)
	}
	return resultado, nil
}

func (r *fakeRepo) CrearPC(ctx context.Context, pc *domain.PC) error {
	for _, existente := range r.pcs {
		if existente.CarroID == pc.CarroID && existente.Identificador == pc.Identificador {
			return ErrIdentificadorDuplicado
		}
		if existente.NumeroSerie == pc.NumeroSerie {
			return ErrNumeroSerieDuplicado
		}
	}
	r.pcs[pc.ID] = pc
	return nil
}
func (r *fakeRepo) BuscarPCPorID(ctx context.Context, id string) (*domain.PC, error) {
	pc, ok := r.pcs[id]
	if !ok {
		return nil, ErrPCNoEncontrada
	}
	return pc, nil
}
func (r *fakeRepo) GuardarPC(ctx context.Context, pc *domain.PC) error {
	r.pcs[pc.ID] = pc
	return nil
}
func (r *fakeRepo) ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error) {
	var resultado []*domain.PC
	for _, pc := range r.pcs {
		if pc.CarroID == carroID {
			resultado = append(resultado, pc)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CrearIncidencia(ctx context.Context, i *domain.Incidencia) error {
	r.incidencias[i.ID] = i
	return nil
}
func (r *fakeRepo) BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error) {
	i, ok := r.incidencias[id]
	if !ok {
		return nil, ErrIncidenciaNoEncontrada
	}
	return i, nil
}
func (r *fakeRepo) GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error {
	r.incidencias[i.ID] = i
	return nil
}
func (r *fakeRepo) ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.PCID == pcID {
			resultado = append(resultado, i)
		}
	}
	return resultado, nil
}

// ── fakeValidadorReservas ───────────────────────────────────────────────

type fakeValidadorReservas struct {
	canceladas     int
	notificados    int
	err            error
	llamado        bool
	veces          int
	motivoRecibido string
	// tieneFuturas modela lo que ve el reintento: reservas todavía vivas
	// sobre una PC que ya se guardó en su nuevo estado, o sea una cascada
	// que quedó a medias.
	tieneFuturas    bool
	errTieneFuturas error
}

func (f *fakeValidadorReservas) CancelarReservasFuturasDePC(ctx context.Context, pcID, motivo string) (int, int, error) {
	f.llamado = true
	f.veces++
	f.motivoRecibido = motivo
	return f.canceladas, f.notificados, f.err
}

func (f *fakeValidadorReservas) TieneReservasFuturas(ctx context.Context, pcID string) (bool, error) {
	return f.tieneFuturas, f.errTieneFuturas
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

func nuevoServicioDeTest(repo Repo, validador ValidadorReservas) *Service {
	contadorID = 0
	return NewService(repo, validador, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	})
}

func servicioSimple(repo Repo) *Service {
	return nuevoServicioDeTest(repo, &fakeValidadorReservas{})
}

// ── Carro ───────────────────────────────────────────────────────────────

func TestCrearCarro_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	c, err := svc.CrearCarro(context.Background(), "Carro 1", "Notebooks")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c.Nombre != "Carro 1" {
		t.Errorf("nombre incorrecto: %s", c.Nombre)
	}
}

func TestCrearCarro_NombreVacio_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearCarro(context.Background(), "", "")

	if !errors.Is(err, domain.ErrNombreCarroVacio) {
		t.Fatalf("esperaba ErrNombreCarroVacio, obtuve %v", err)
	}
}

func TestEditarCarro_SoloNombre(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["c1"] = &domain.Carro{ID: "c1", Nombre: "Viejo", Descripcion: "Original"}
	svc := servicioSimple(repo)

	nuevoNombre := "Nuevo"
	err := svc.EditarCarro(context.Background(), "c1", &nuevoNombre, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.carros["c1"].Nombre != "Nuevo" || repo.carros["c1"].Descripcion != "Original" {
		t.Errorf("edición parcial incorrecta: %+v", repo.carros["c1"])
	}
}

func TestEditarCarro_NoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())
	nombre := "Nuevo"

	err := svc.EditarCarro(context.Background(), "no-existe", &nombre, nil)

	if !errors.Is(err, ErrCarroNoEncontrado) {
		t.Fatalf("esperaba ErrCarroNoEncontrado, obtuve %v", err)
	}
}

// ── PC ──────────────────────────────────────────────────────────────────

func TestCrearPC_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	pc, err := svc.CrearPC(context.Background(), "c1", 27, 123456, true, "i5", "8GB", "Windows 11", "Office")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pc.Identificador != 27 || pc.Estado != domain.EstadoDisponible {
		t.Errorf("PC incorrecta: %+v", pc)
	}
}

func TestCrearPC_IdentificadorDuplicadoEnMismoCarro_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	_, err := svc.CrearPC(context.Background(), "c1", 27, 111, false, "", "", "", "")
	if err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}

	_, err = svc.CrearPC(context.Background(), "c1", 27, 222, false, "", "", "", "")
	if !errors.Is(err, ErrIdentificadorDuplicado) {
		t.Fatalf("esperaba ErrIdentificadorDuplicado, obtuve %v", err)
	}
}

func TestCrearPC_MismoIdentificadorOtroCarro_OK(t *testing.T) {
	// Confirma la regla de negocio: el identificador se repite entre
	// carros distintos sin problema.
	svc := servicioSimple(nuevoFakeRepo())

	_, err1 := svc.CrearPC(context.Background(), "c1", 27, 111, false, "", "", "", "")
	_, err2 := svc.CrearPC(context.Background(), "c2", 27, 222, false, "", "", "", "")

	if err1 != nil || err2 != nil {
		t.Fatalf("ninguna debería fallar: err1=%v err2=%v", err1, err2)
	}
}

func TestEditarPC_MoverDeCarro(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", CarroID: "c1", Identificador: 1}
	svc := servicioSimple(repo)

	nuevoCarro := "c2"
	err := svc.EditarPC(context.Background(), "pc1", EditarPCParams{CarroID: &nuevoCarro})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.pcs["pc1"].CarroID != "c2" {
		t.Errorf("no se movió de carro: %s", repo.pcs["pc1"].CarroID)
	}
}

// ── CambiarEstadoPC + cascada ───────────────────────────────────────────

func TestCambiarEstadoPC_AMantenimiento_DisparaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{canceladas: 3, notificados: 2}
	svc := nuevoServicioDeTest(repo, validador)

	res, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !validador.llamado {
		t.Fatal("esperaba que se dispare la cascada de cancelación")
	}
	if res.ReservasCanceladas != 3 || res.DocentesNotificados != 2 {
		t.Errorf("resultado de cascada incorrecto: %+v", res)
	}
	if repo.pcs["pc1"].Estado != domain.EstadoEnMantenimiento {
		t.Errorf("estado final incorrecto: %s", repo.pcs["pc1"].Estado)
	}
}

func TestCambiarEstadoPC_ADisponible_NoDisparaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.llamado {
		t.Error("volver a DISPONIBLE no debería disparar ninguna cascada de cancelación")
	}
}

func TestCambiarEstadoPC_TransicionInvalida_NoLlegaALaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoPCInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoPCInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("una transición inválida no debería disparar ninguna cascada")
	}
}

func TestCambiarEstadoPC_MotivoPersonalizado_SeUsaEnLaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	motivo := "Falla eléctrica reportada por el docente"
	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoFueraDeServicio, &motivo)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.motivoRecibido != motivo {
		t.Errorf("motivo incorrecto: %q", validador.motivoRecibido)
	}
}

func TestCambiarEstadoPC_SinMotivo_UsaMensajePorDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.motivoRecibido == "" {
		t.Error("esperaba un mensaje generado por defecto, no vacío")
	}
	// Es la RAZÓN, no el aviso entero: el "Tu reserva fue cancelada:" lo
	// antepone notification. Nombra la PC porque el docente recibe un
	// aviso por cada una y sin el identificador no sabe cuál se le cayó
	// (RF-05.3).
	esperado := "la PC 27 pasó a FUERA_DE_SERVICIO"
	if validador.motivoRecibido != esperado {
		t.Errorf("motivo incorrecto:\n  esperado %q\n  obtenido %q", esperado, validador.motivoRecibido)
	}
}

func TestCambiarEstadoPC_ErrorEnCascada_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{err: errors.New("notification caído")}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if err == nil {
		t.Fatal("esperaba que el error de la cascada se propague")
	}
}

// ── DarDeBajaPC ─────────────────────────────────────────────────────────

func TestDarDeBajaPC_DisparaLaMismaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{canceladas: 1, notificados: 1}
	svc := nuevoServicioDeTest(repo, validador)

	res, err := svc.DarDeBajaPC(context.Background(), "pc1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !validador.llamado {
		t.Fatal("dar de baja debería disparar la misma cascada que FUERA_DE_SERVICIO")
	}
	if res.ReservasCanceladas != 1 {
		t.Errorf("resultado incorrecto: %+v", res)
	}
	if !repo.pcs["pc1"].DadaDeBaja {
		t.Error("la PC debería quedar marcada como dada de baja")
	}
}

func TestDarDeBajaPC_DosVeces_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", DadaDeBaja: true}
	svc := servicioSimple(repo)

	_, err := svc.DarDeBajaPC(context.Background(), "pc1")

	if !errors.Is(err, domain.ErrPCYaDadaDeBaja) {
		t.Fatalf("esperaba ErrPCYaDadaDeBaja, obtuve %v", err)
	}
}

// ── Incidencia ──────────────────────────────────────────────────────────

func TestCrearIncidencia_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	i, err := svc.CrearIncidencia(context.Background(), "pc1", "usuario1", "No enciende", domain.GravedadGrave)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if i.Estado != domain.IncidenciaAbierta {
		t.Errorf("estado inicial incorrecto: %s", i.Estado)
	}
}

func TestCrearIncidencia_DescripcionVacia_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearIncidencia(context.Background(), "pc1", "usuario1", "", domain.GravedadLeve)

	if !errors.Is(err, domain.ErrDescripcionVacia) {
		t.Fatalf("esperaba ErrDescripcionVacia, obtuve %v", err)
	}
}

func TestEditarIncidencia_MarcarEnviadaDGE(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.incidencias["i1"] = &domain.Incidencia{ID: "i1", Estado: domain.IncidenciaEnReparacion}
	svc := servicioSimple(repo)

	err := svc.EditarIncidencia(context.Background(), "i1", EditarIncidenciaParams{MarcarEnviadaDGE: true})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !repo.incidencias["i1"].EnviadoDGE {
		t.Error("EnviadoDGE debería quedar true")
	}
	if repo.incidencias["i1"].Estado != domain.IncidenciaEnviadaDGE {
		t.Errorf("estado incorrecto: %s", repo.incidencias["i1"].Estado)
	}
}

func TestEditarIncidencia_SoloEstado(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.incidencias["i1"] = &domain.Incidencia{ID: "i1", Estado: domain.IncidenciaAbierta}
	svc := servicioSimple(repo)

	resuelta := domain.IncidenciaResuelta
	err := svc.EditarIncidencia(context.Background(), "i1", EditarIncidenciaParams{Estado: &resuelta})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.incidencias["i1"].Estado != domain.IncidenciaResuelta {
		t.Errorf("estado incorrecto: %s", repo.incidencias["i1"].Estado)
	}
}

func TestEditarIncidencia_NoExiste_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	err := svc.EditarIncidencia(context.Background(), "no-existe", EditarIncidenciaParams{})

	if !errors.Is(err, ErrIncidenciaNoEncontrada) {
		t.Fatalf("esperaba ErrIncidenciaNoEncontrada, obtuve %v", err)
	}
}

// ── Cascada a medias: el reintento tiene que poder terminarla ───────────
//
// La cascada de RF-03.8/03.9 no es atómica con el guardado de la PC (cruza a
// reservation, que abre su propia transacción). Si falla el segundo paso, la
// PC queda guardada en su nuevo estado y sus reservas siguen CONFIRMADA.
// Estos tests cubren que el reintento la completa: sin eso rebotaría con un
// 409 y no habría forma de terminarla desde la API.

func TestCambiarEstadoPC_ReintentoConCascadaPendiente_LaCompleta(t *testing.T) {
	repo := nuevoFakeRepo()
	// El estado de una PC cuyo intento anterior murió a mitad de camino: ya
	// está EN_MANTENIMIENTO, pero le quedaron reservas vivas.
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{tieneFuturas: true, canceladas: 3, notificados: 2}
	svc := nuevoServicioDeTest(repo, validador)

	resultado, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if err != nil {
		t.Fatalf("el reintento debería completar la cascada, no fallar: %v", err)
	}
	if !validador.llamado {
		t.Fatal("el reintento tiene que llegar a la cascada")
	}
	if resultado.ReservasCanceladas != 3 || resultado.DocentesNotificados != 2 {
		t.Errorf("resultado inesperado: %+v", resultado)
	}
}

// El reverso: sin nada pendiente, repetir la operación sigue siendo un error
// (RF-03.8 no se aplica dos veces).
func TestCambiarEstadoPC_MismoEstadoSinNadaPendiente_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{tieneFuturas: false}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoPCInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoPCInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("sin cascada pendiente no hay que volver a cancelar nada")
	}
}

// La excepción es solo para repetir la MISMA transición. Un estado terminal
// sigue siendo terminal aunque queden reservas vivas: si no, esto se
// convertiría en una puerta trasera para salir de FUERA_DE_SERVICIO.
func TestCambiarEstadoPC_DesdeTerminalConReservasVivas_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{tieneFuturas: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoPCInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoPCInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("una transición inválida no debería disparar ninguna cascada")
	}
}

// Volver a DISPONIBLE no dispara cascada, así que tampoco hay nada que
// reintentar: repetirlo es un error a secas.
func TestCambiarEstadoPC_MismoEstadoQueNoDisparaCascada_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{tieneFuturas: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoPCInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoPCInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("DISPONIBLE no saca la PC de circulación: no hay cascada que completar")
	}
}

func TestDarDeBajaPC_ReintentoConCascadaPendiente_LaCompleta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Identificador: 27, DadaDeBaja: true}
	validador := &fakeValidadorReservas{tieneFuturas: true, canceladas: 5, notificados: 1}
	svc := nuevoServicioDeTest(repo, validador)

	resultado, err := svc.DarDeBajaPC(context.Background(), "pc1")

	if err != nil {
		t.Fatalf("el reintento debería completar la cascada, no fallar: %v", err)
	}
	if resultado.ReservasCanceladas != 5 {
		t.Errorf("esperaba 5 reservas canceladas, obtuve %d", resultado.ReservasCanceladas)
	}
	// El motivo tiene que seguir nombrando la PC aunque sea un reintento —
	// es lo que el docente lee en la notificación (RF-05.3).
	if validador.motivoRecibido != "la PC 27 fue dada de baja del inventario" {
		t.Errorf("motivo inesperado en el reintento: %q", validador.motivoRecibido)
	}
}

// Un fallo al consultar si quedó algo pendiente no puede confundirse con
// "no quedó nada": se propaga.
func TestCambiarEstadoPC_ErrorAlVerificarPendiente_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoEnMantenimiento}
	fallo := errors.New("postgres no responde")
	validador := &fakeValidadorReservas{errTieneFuturas: fallo}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, fallo) {
		t.Fatalf("esperaba que se propagara el error de la consulta, obtuve %v", err)
	}
}

// El error del segundo paso tiene que decir que el primero sí se aplicó y
// que reintentar completa lo que falta — si no, quien lo lee no sabe en qué
// estado quedó el sistema.
func TestCambiarEstadoPC_ErrorEnCascada_ElMensajeExplicaComoSeguir(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.pcs["pc1"] = &domain.PC{ID: "pc1", Estado: domain.EstadoDisponible}
	fallo := errors.New("postgres no responde")
	svc := nuevoServicioDeTest(repo, &fakeValidadorReservas{err: fallo})

	_, err := svc.CambiarEstadoPC(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if !errors.Is(err, fallo) {
		t.Fatalf("el error original tiene que seguir envuelto, obtuve %v", err)
	}
	if !strings.Contains(err.Error(), "reintentar") {
		t.Errorf("el error debería explicar que reintentar completa la cascada: %q", err)
	}
	// Y la PC quedó guardada en su nuevo estado: es justamente el estado que
	// el reintento va a encontrar.
	if repo.pcs["pc1"].Estado != domain.EstadoFueraDeServicio {
		t.Errorf("la PC debería haber quedado guardada, está en %s", repo.pcs["pc1"].Estado)
	}
}
