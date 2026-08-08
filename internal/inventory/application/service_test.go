package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	carros      map[string]*domain.Carro
	equipos     map[string]*domain.Equipo
	incidencias map[string]*domain.Incidencia
	licencias   map[string]*domain.LicenciaSoftware
	// errAlCrearLicenciaEnEquipo fuerza un fallo que NO es un duplicado, para
	// probar que el lote corta ahí en vez de seguir como si nada.
	errAlCrearLicenciaEnEquipo map[string]error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		carros:                     make(map[string]*domain.Carro),
		equipos:                    make(map[string]*domain.Equipo),
		incidencias:                make(map[string]*domain.Incidencia),
		licencias:                  make(map[string]*domain.LicenciaSoftware),
		errAlCrearLicenciaEnEquipo: make(map[string]error),
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

func (r *fakeRepo) CrearEquipo(ctx context.Context, equipo *domain.Equipo) error {
	for _, existente := range r.equipos {
		if existente.CarroID == equipo.CarroID && existente.Identificador == equipo.Identificador {
			return ErrIdentificadorDuplicado
		}
		if existente.NumeroSerie == equipo.NumeroSerie {
			return ErrNumeroSerieDuplicado
		}
	}
	r.equipos[equipo.ID] = equipo
	return nil
}
func (r *fakeRepo) BuscarEquipoPorID(ctx context.Context, id string) (*domain.Equipo, error) {
	equipo, ok := r.equipos[id]
	if !ok {
		return nil, ErrEquipoNoEncontrado
	}
	return equipo, nil
}
func (r *fakeRepo) GuardarEquipo(ctx context.Context, equipo *domain.Equipo) error {
	r.equipos[equipo.ID] = equipo
	return nil
}
func (r *fakeRepo) ListarEquiposPorCarro(ctx context.Context, carroID string) ([]*domain.Equipo, error) {
	var resultado []*domain.Equipo
	for _, equipo := range r.equipos {
		if equipo.CarroID == carroID {
			resultado = append(resultado, equipo)
		}
	}
	return resultado, nil
}

// ListarEquiposSueltos: lo prestable que no está en ningún carro (015).
func (r *fakeRepo) ListarEquiposSueltos(ctx context.Context) ([]*domain.Equipo, error) {
	var resultado []*domain.Equipo
	for _, equipo := range r.equipos {
		if !equipo.EstaEnUnCarro() {
			resultado = append(resultado, equipo)
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
func (r *fakeRepo) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.EquipoID == equipoID {
			resultado = append(resultado, i)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) CategoriasDeFallaUsadas(ctx context.Context) ([]string, error) {
	vistas := map[string]bool{}
	var resultado []string
	for _, i := range r.incidencias {
		if i.Categoria == "" || vistas[strings.ToLower(i.Categoria)] {
			continue
		}
		vistas[strings.ToLower(i.Categoria)] = true
		resultado = append(resultado, i.Categoria)
	}
	sort.Strings(resultado)
	return resultado, nil
}

func (r *fakeRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	if err := r.errAlCrearLicenciaEnEquipo[l.EquipoID]; err != nil {
		return err
	}
	// Mismo criterio que el índice funcional de la migración 012: única por
	// PC sin distinguir mayúsculas.
	for _, existente := range r.licencias {
		if existente.EquipoID == l.EquipoID && strings.EqualFold(existente.Nombre, l.Nombre) {
			return ErrLicenciaDuplicada
		}
	}
	r.licencias[l.ID] = l
	return nil
}

func (r *fakeRepo) BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error) {
	l, ok := r.licencias[id]
	if !ok {
		return nil, ErrLicenciaNoEncontrada
	}
	return l, nil
}

func (r *fakeRepo) GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	if _, ok := r.licencias[l.ID]; !ok {
		return ErrLicenciaNoEncontrada
	}
	r.licencias[l.ID] = l
	return nil
}

func (r *fakeRepo) BorrarLicencia(ctx context.Context, id string) error {
	if _, ok := r.licencias[id]; !ok {
		return ErrLicenciaNoEncontrada
	}
	delete(r.licencias, id)
	return nil
}

func (r *fakeRepo) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	var resultado []*domain.LicenciaSoftware
	for _, l := range r.licencias {
		if l.EquipoID == equipoID {
			resultado = append(resultado, l)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error) {
	var resultado []*LicenciaConUbicacion
	for _, l := range r.licencias {
		resultado = append(resultado, r.conUbicacion(l))
	}
	return resultado, nil
}

func (r *fakeRepo) ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*LicenciaConUbicacion, error) {
	// El fake no reproduce el filtro grueso del SQL —eso se verifica contra
	// Postgres real en infrastructure— pero sí lo único que cambiaría el
	// resultado de un aviso: las PCs dadas de baja no cuentan.
	var resultado []*LicenciaConUbicacion
	for _, l := range r.licencias {
		u := r.conUbicacion(l)
		if u.EquipoDadoDeBaja {
			continue
		}
		resultado = append(resultado, u)
	}
	return resultado, nil
}

func (r *fakeRepo) MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error {
	guardada, ok := r.licencias[l.ID]
	if !ok {
		return ErrLicenciaNoEncontrada
	}
	guardada.AvisadoPrevioPara = l.AvisadoPrevioPara
	guardada.AvisadoVencimientoPara = l.AvisadoVencimientoPara
	return nil
}

func (r *fakeRepo) conUbicacion(l *domain.LicenciaSoftware) *LicenciaConUbicacion {
	u := &LicenciaConUbicacion{Licencia: l}
	if equipo, ok := r.equipos[l.EquipoID]; ok {
		u.Identificador = equipo.Identificador
		u.EquipoDadoDeBaja = equipo.DadoDeBaja
		u.CarroID = equipo.CarroID
		if carro, ok := r.carros[equipo.CarroID]; ok {
			u.CarroNombre = carro.Nombre
		}
	}
	return u
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
	// prestado: el equipo está afuera del laboratorio. Da de baja no puede
	// pisar eso.
	prestado    bool
	errPrestado error
}

func (f *fakeValidadorReservas) EstaPrestado(ctx context.Context, equipoID string) (bool, error) {
	return f.prestado, f.errPrestado
}

func (f *fakeValidadorReservas) CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID, motivo string) (int, int, error) {
	f.llamado = true
	f.veces++
	f.motivoRecibido = motivo
	return f.canceladas, f.notificados, f.err
}

func (f *fakeValidadorReservas) TieneReservasFuturas(ctx context.Context, equipoID string) (bool, error) {
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

func TestCrearEquipo_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	equipo, err := svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "5CD1234ABC", true, "i5", "8GB", "Windows 11", "Office")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.Identificador != 27 || equipo.Estado != domain.EstadoDisponible {
		t.Errorf("PC incorrecta: %+v", equipo)
	}
}

func TestCrearEquipo_IdentificadorDuplicadoEnMismoCarro_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioSimple(repo)

	_, err := svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "SERIE-111", false, "", "", "", "")
	if err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}

	_, err = svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "SERIE-222", false, "", "", "", "")
	if !errors.Is(err, ErrIdentificadorDuplicado) {
		t.Fatalf("esperaba ErrIdentificadorDuplicado, obtuve %v", err)
	}
}

func TestCrearEquipo_MismoIdentificadorOtroCarro_OK(t *testing.T) {
	// Confirma la regla de negocio: el identificador se repite entre
	// carros distintos sin problema.
	svc := servicioSimple(nuevoFakeRepo())

	_, err1 := svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "SERIE-111", false, "", "", "", "")
	_, err2 := svc.CrearEquipoDeCarro(context.Background(), "c2", 27, "SERIE-222", false, "", "", "", "")

	if err1 != nil || err2 != nil {
		t.Fatalf("ninguna debería fallar: err1=%v err2=%v", err1, err2)
	}
}

func TestEditarEquipo_MoverDeCarro(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 1}
	svc := servicioSimple(repo)

	nuevoCarro := "c2"
	err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{CarroID: &nuevoCarro})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["pc1"].CarroID != "c2" {
		t.Errorf("no se movió de carro: %s", repo.equipos["pc1"].CarroID)
	}
}

// Editar es la otra puerta por la que se escriben tipo y nombre, además del
// alta. Sin validar acá, un nombre vacío llegaría hasta el CHECK de la 015 y
// volvería como un 500 en vez del 400 que corresponde — y dejaría un equipo
// suelto sin lo único que lo distingue en la lista de entregas.
func TestEditarEquipo_EquipoSueltoNoPuedeQuedarSinNombre(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "PROYECTOR", Nombre: "Proyector Epson"}
	svc := servicioSimple(repo)

	vacio := ""
	err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{Nombre: &vacio})

	if !errors.Is(err, domain.ErrNombreEquipoVacio) {
		t.Fatalf("esperaba ErrNombreEquipoVacio, obtuve %v", err)
	}
	if repo.equipos["eq1"].Nombre != "Proyector Epson" {
		t.Errorf("no debería haberse tocado: %q", repo.equipos["eq1"].Nombre)
	}
}

func TestEditarEquipo_TipoVacioSeRechaza(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "PROYECTOR", Nombre: "Proyector Epson"}
	svc := servicioSimple(repo)

	espacios := "   "
	err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{Tipo: &espacios})

	if !errors.Is(err, domain.ErrTipoEquipoVacio) {
		t.Fatalf("esperaba ErrTipoEquipoVacio, obtuve %v", err)
	}
}

// En una PC de carro el nombre no cumple ninguna función —se la nombra por
// su identificador—, así que borrarlo no rompe nada.
func TestEditarEquipo_UnaEquipoDeCarroSiPuedeQuedarSinNombre(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 3, Nombre: "sobrante"}
	svc := servicioSimple(repo)

	vacio := ""
	if err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{Nombre: &vacio}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["pc1"].Nombre != "" {
		t.Errorf("no se limpió: %q", repo.equipos["pc1"].Nombre)
	}
}

// ── CambiarEstadoEquipo + cascada ───────────────────────────────────────

func TestCambiarEstadoEquipo_AMantenimiento_DisparaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{canceladas: 3, notificados: 2}
	svc := nuevoServicioDeTest(repo, validador)

	res, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !validador.llamado {
		t.Fatal("esperaba que se dispare la cascada de cancelación")
	}
	if res.ReservasCanceladas != 3 || res.DocentesNotificados != 2 {
		t.Errorf("resultado de cascada incorrecto: %+v", res)
	}
	if repo.equipos["pc1"].Estado != domain.EstadoEnMantenimiento {
		t.Errorf("estado final incorrecto: %s", repo.equipos["pc1"].Estado)
	}
}

func TestCambiarEstadoEquipo_ADisponible_NoDisparaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.llamado {
		t.Error("volver a DISPONIBLE no debería disparar ninguna cascada de cancelación")
	}
}

func TestCambiarEstadoEquipo_TransicionInvalida_NoLlegaALaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoEquipoInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoEquipoInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("una transición inválida no debería disparar ninguna cascada")
	}
}

func TestCambiarEstadoEquipo_MotivoPersonalizado_SeUsaEnLaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	motivo := "Falla eléctrica reportada por el docente"
	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoFueraDeServicio, &motivo)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.motivoRecibido != motivo {
		t.Errorf("motivo incorrecto: %q", validador.motivoRecibido)
	}
}

func TestCambiarEstadoEquipo_SinMotivo_UsaMensajePorDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if validador.motivoRecibido == "" {
		t.Error("esperaba un mensaje generado por defecto, no vacío")
	}
	// Es la RAZÓN, no el aviso entero: el "Tu reserva fue cancelada:" lo
	// antepone notification. Nombra el equipo porque el docente recibe un
	// aviso por cada uno y sin eso no sabe cuál se le cayó (RF-05.3).
	//
	// Sin artículo a propósito: la etiqueta puede ser "PC 27" o "Proyector
	// Epson", y no hay un artículo que sirva para las dos.
	esperado := "PC 27 pasó a FUERA_DE_SERVICIO"
	if validador.motivoRecibido != esperado {
		t.Errorf("motivo incorrecto:\n  esperado %q\n  obtenido %q", esperado, validador.motivoRecibido)
	}
}

func TestCambiarEstadoEquipo_ErrorEnCascada_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{err: errors.New("notification caído")}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if err == nil {
		t.Fatal("esperaba que el error de la cascada se propague")
	}
}

// ── DarDeBajaEquipo ─────────────────────────────────────────────────────

// Dar de baja algo que está afuera dejaba el préstamo abierto para siempre:
// el equipo desaparecía del inventario pero seguía figurando en "lo que falta
// volver", y desde ninguna pantalla se podía cerrar —la lista de equipos a
// recibir sale del inventario—.
func TestDarDeBajaEquipo_PrestadoNoSePuedeDarDeBaja(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{prestado: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.DarDeBajaEquipo(context.Background(), "eq1")

	if !errors.Is(err, ErrEquipoPrestado) {
		t.Fatalf("esperaba ErrEquipoPrestado, obtuve %v", err)
	}
	if repo.equipos["eq1"].DadoDeBaja {
		t.Error("no tenía que quedar dado de baja")
	}
	if validador.llamado {
		t.Error("tampoco tenía que cancelar reservas: la baja no ocurrió")
	}
}

// El reintento de una cascada pendiente NO se bloquea: ahí el equipo ya está
// de baja y lo único que falta es terminar de cancelar sus reservas. Si lo
// prestaron antes de darlo de baja, bloquear acá dejaría la cascada a medias
// para siempre.
func TestDarDeBajaEquipo_YaDeBajaConCascadaPendiente_NoLoFrenaElPrestamo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Identificador: 27, DadoDeBaja: true}
	validador := &fakeValidadorReservas{prestado: true, tieneFuturas: true, canceladas: 2}
	svc := nuevoServicioDeTest(repo, validador)

	res, err := svc.DarDeBajaEquipo(context.Background(), "eq1")

	if err != nil {
		t.Fatalf("el reintento tiene que poder completarse: %v", err)
	}
	if res.ReservasCanceladas != 2 {
		t.Errorf("esperaba que terminara la cascada, obtuve %+v", res)
	}
}

func TestDarDeBajaEquipo_DisparaLaMismaCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{canceladas: 1, notificados: 1}
	svc := nuevoServicioDeTest(repo, validador)

	res, err := svc.DarDeBajaEquipo(context.Background(), "pc1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !validador.llamado {
		t.Fatal("dar de baja debería disparar la misma cascada que FUERA_DE_SERVICIO")
	}
	if res.ReservasCanceladas != 1 {
		t.Errorf("resultado incorrecto: %+v", res)
	}
	if !repo.equipos["pc1"].DadoDeBaja {
		t.Error("la PC debería quedar marcada como dada de baja")
	}
}

func TestDarDeBajaEquipo_DosVeces_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", DadoDeBaja: true}
	svc := servicioSimple(repo)

	_, err := svc.DarDeBajaEquipo(context.Background(), "pc1")

	if !errors.Is(err, domain.ErrEquipoYaDadoDeBaja) {
		t.Fatalf("esperaba ErrEquipoYaDadoDeBaja, obtuve %v", err)
	}
}

// ── Incidencia ──────────────────────────────────────────────────────────

func TestCrearIncidencia_OK(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	i, err := svc.CrearIncidencia(context.Background(), "pc1", "usuario1", "No enciende", "", domain.GravedadGrave)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if i.Estado != domain.IncidenciaAbierta {
		t.Errorf("estado inicial incorrecto: %s", i.Estado)
	}
}

func TestCrearIncidencia_DescripcionVacia_Error(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearIncidencia(context.Background(), "pc1", "usuario1", "", "", domain.GravedadLeve)

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

func TestCambiarEstadoEquipo_ReintentoConCascadaPendiente_LaCompleta(t *testing.T) {
	repo := nuevoFakeRepo()
	// El estado de una PC cuyo intento anterior murió a mitad de camino: ya
	// está EN_MANTENIMIENTO, pero le quedaron reservas vivas.
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{tieneFuturas: true, canceladas: 3, notificados: 2}
	svc := nuevoServicioDeTest(repo, validador)

	resultado, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

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
func TestCambiarEstadoEquipo_MismoEstadoSinNadaPendiente_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoEnMantenimiento}
	validador := &fakeValidadorReservas{tieneFuturas: false}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoEquipoInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoEquipoInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("sin cascada pendiente no hay que volver a cancelar nada")
	}
}

// La excepción es solo para repetir la MISMA transición. Un estado terminal
// sigue siendo terminal aunque queden reservas vivas: si no, esto se
// convertiría en una puerta trasera para salir de FUERA_DE_SERVICIO.
func TestCambiarEstadoEquipo_DesdeTerminalConReservasVivas_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{tieneFuturas: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoEquipoInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoEquipoInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("una transición inválida no debería disparar ninguna cascada")
	}
}

// Volver a DISPONIBLE no dispara cascada, así que tampoco hay nada que
// reintentar: repetirlo es un error a secas.
func TestCambiarEstadoEquipo_MismoEstadoQueNoDisparaCascada_SigueSiendoError(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoDisponible}
	validador := &fakeValidadorReservas{tieneFuturas: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if !errors.Is(err, domain.ErrTransicionEstadoEquipoInvalida) {
		t.Fatalf("esperaba ErrTransicionEstadoEquipoInvalida, obtuve %v", err)
	}
	if validador.llamado {
		t.Error("DISPONIBLE no saca la PC de circulación: no hay cascada que completar")
	}
}

func TestDarDeBajaEquipo_ReintentoConCascadaPendiente_LaCompleta(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Identificador: 27, DadoDeBaja: true}
	validador := &fakeValidadorReservas{tieneFuturas: true, canceladas: 5, notificados: 1}
	svc := nuevoServicioDeTest(repo, validador)

	resultado, err := svc.DarDeBajaEquipo(context.Background(), "pc1")

	if err != nil {
		t.Fatalf("el reintento debería completar la cascada, no fallar: %v", err)
	}
	if resultado.ReservasCanceladas != 5 {
		t.Errorf("esperaba 5 reservas canceladas, obtuve %d", resultado.ReservasCanceladas)
	}
	// El motivo tiene que seguir nombrando el equipo aunque sea un reintento
	// — es lo que el docente lee en la notificación (RF-05.3).
	if validador.motivoRecibido != "PC 27 fue dado de baja del inventario" {
		t.Errorf("motivo inesperado en el reintento: %q", validador.motivoRecibido)
	}
}

// Un fallo al consultar si quedó algo pendiente no puede confundirse con
// "no quedó nada": se propaga.
func TestCambiarEstadoEquipo_ErrorAlVerificarPendiente_SePropaga(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoEnMantenimiento}
	fallo := errors.New("postgres no responde")
	validador := &fakeValidadorReservas{errTieneFuturas: fallo}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if !errors.Is(err, fallo) {
		t.Fatalf("esperaba que se propagara el error de la consulta, obtuve %v", err)
	}
}

// El error del segundo paso tiene que decir que el primero sí se aplicó y
// que reintentar completa lo que falta — si no, quien lo lee no sabe en qué
// estado quedó el sistema.
func TestCambiarEstadoEquipo_ErrorEnCascada_ElMensajeExplicaComoSeguir(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoDisponible}
	fallo := errors.New("postgres no responde")
	svc := nuevoServicioDeTest(repo, &fakeValidadorReservas{err: fallo})

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoFueraDeServicio, nil)

	if !errors.Is(err, fallo) {
		t.Fatalf("el error original tiene que seguir envuelto, obtuve %v", err)
	}
	if !strings.Contains(err.Error(), "reintentar") {
		t.Errorf("el error debería explicar que reintentar completa la cascada: %q", err)
	}
	// Y la PC quedó guardada en su nuevo estado: es justamente el estado que
	// el reintento va a encontrar.
	if repo.equipos["pc1"].Estado != domain.EstadoFueraDeServicio {
		t.Errorf("la PC debería haber quedado guardada, está en %s", repo.equipos["pc1"].Estado)
	}
}
