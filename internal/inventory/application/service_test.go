package application

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/secretos"
	"github.com/ramiro/sgrc/internal/shared/texto"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	preferenciasHuerfanas []*PreferenciaHuerfana
	carros                map[string]*domain.Carro
	equipos               map[string]*domain.Equipo
	incidencias           map[string]*domain.Incidencia
	licencias             map[string]*domain.LicenciaSoftware
	preferencias          map[string]*domain.PreferenciaDeEquipo
	cuentas               map[string]*domain.CuentaDeEquipo
	// nombresDeMateria es lo que el selector del inventario ofrece.
	nombresDeMateria []string
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
		preferencias:               make(map[string]*domain.PreferenciaDeEquipo),
		cuentas:                    make(map[string]*domain.CuentaDeEquipo),
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
func (r *fakeRepo) ListarCarros(ctx context.Context, incluirRetirados bool) ([]*domain.Carro, error) {
	var resultado []*domain.Carro
	for _, c := range r.carros {
		if c.DadoDeBaja && !incluirRetirados {
			continue
		}
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

// ListarEquipos: el inventario, o solo lo que no está en ningún carro.
func (r *fakeRepo) ListarEquipos(ctx context.Context, soloSueltos bool) ([]*domain.Equipo, error) {
	var resultado []*domain.Equipo
	for _, equipo := range r.equipos {
		if soloSueltos && equipo.EstaEnUnCarro() {
			continue
		}
		resultado = append(resultado, equipo)
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
func (r *fakeRepo) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string, limite int) ([]*domain.Incidencia, error) {
	var resultado []*domain.Incidencia
	for _, i := range r.incidencias {
		if i.EquipoID == equipoID {
			resultado = append(resultado, i)
		}
	}
	// El recorte se imita para que el test del tope pueda probar algo. El orden
	// de la de verdad lo pone el ORDER BY; acá alcanza con no devolver más de lo
	// pedido.
	if limite > 0 && len(resultado) > limite {
		resultado = resultado[:limite]
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

// CrearLicencia devuelve `false` cuando ya estaba, igual que el repo real: el
// duplicado no es un error porque una sentencia fallida abortaría la
// transacción del lote entero.
func (r *fakeRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) (bool, error) {
	if err := r.errAlCrearLicenciaEnEquipo[l.EquipoID]; err != nil {
		return false, err
	}
	// Mismo criterio que el índice funcional ux_licencia_equipo_nombre: única
	// por equipo, comparando con la clave de texto del sistema (RF-00.1).
	for _, existente := range r.licencias {
		if existente.EquipoID == l.EquipoID && texto.SonElMismo(existente.Nombre, l.Nombre) {
			return false, nil
		}
	}
	r.licencias[l.ID] = l
	return true, nil
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

// ── Preferencias de materia (RF-03.21) ────────────────────────────────

// mismaMarca replica el UNIQUE de la tabla: mismo equipo, misma materia
// (sin distinguir acentos ni mayúsculas) y mismo alcance.
func mismaMarca(a, b *domain.PreferenciaDeEquipo) bool {
	igualPtr := func(x, y *int) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return *x == *y
	}
	igualStr := func(x, y *string) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return *x == *y
	}
	return a.EquipoID == b.EquipoID &&
		strings.EqualFold(a.MateriaNombre, b.MateriaNombre) &&
		igualPtr(a.Anio, b.Anio) &&
		igualStr(a.Modalidad, b.Modalidad) &&
		igualStr(a.Division, b.Division)
}

// EnTransaccion corre fn contra el mismo fake y, si devuelve error, DESHACE lo
// que se haya escrito adentro.
//
// Deshacer de verdad —y no sólo llamar a fn— es lo que permite que un test de
// este paquete verifique que un lote que falla en el medio no deja nada: con un
// paso a través, el fake mentiría diciendo que sí quedaron escritas las
// anteriores, que es justo el bug que esto vino a cerrar.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	licencias := maps.Clone(r.licencias)
	preferencias := maps.Clone(r.preferencias)

	if err := fn(r); err != nil {
		r.licencias = licencias
		r.preferencias = preferencias
		return err
	}
	return nil
}

func (r *fakeRepo) CrearPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) (bool, error) {
	for _, existente := range r.preferencias {
		if mismaMarca(existente, p) {
			return false, nil
		}
	}
	r.preferencias[p.ID] = p
	return true, nil
}

func (r *fakeRepo) GuardarPreferencia(ctx context.Context, p *domain.PreferenciaDeEquipo) error {
	if _, ok := r.preferencias[p.ID]; !ok {
		return domain.ErrPreferenciaNoEncontr
	}
	r.preferencias[p.ID] = p
	return nil
}

func (r *fakeRepo) BuscarPreferenciaPorID(ctx context.Context, id string) (*domain.PreferenciaDeEquipo, error) {
	p, ok := r.preferencias[id]
	if !ok {
		return nil, domain.ErrPreferenciaNoEncontr
	}
	return p, nil
}

func (r *fakeRepo) BorrarPreferencia(ctx context.Context, id string) error {
	if _, ok := r.preferencias[id]; !ok {
		return domain.ErrPreferenciaNoEncontr
	}
	delete(r.preferencias, id)
	return nil
}

func (r *fakeRepo) ListarPreferenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.PreferenciaDeEquipo, error) {
	var resultado []*domain.PreferenciaDeEquipo
	for _, p := range r.preferencias {
		if p.EquipoID == equipoID {
			resultado = append(resultado, p)
		}
	}
	return resultado, nil
}

func (r *fakeRepo) NombresDeMateriaEnUso(ctx context.Context) ([]string, error) {
	return r.nombresDeMateria, nil
}

func (r *fakeRepo) ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error) {
	var resultado []*LicenciaConUbicacion
	for _, l := range r.licencias {
		resultado = append(resultado, r.conUbicacion(l))
	}
	return resultado, nil
}

// ContarPendientesDeRenovar cuenta las que hoy están por vencer o vencidas,
// ya se haya avisado de ellas o no — es lo que decide si el aviso de la
// campana se cierra. El fake reproduce la misma condición que el SQL: fecha
// cargada, equipo activo, y dentro de la ventana de aviso.
func (r *fakeRepo) ContarPendientesDeRenovar(ctx context.Context, hoy time.Time) (int, error) {
	n := 0
	for _, l := range r.licencias {
		if r.conUbicacion(l).EquipoDadoDeBaja || l.FechaVencimiento == nil {
			continue
		}
		switch l.Estado(hoy) {
		case domain.LicenciaPorVencer, domain.LicenciaVencida:
			n++
		}
	}
	return n, nil
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
	// tieneFuturas modela lo que ve el reintento: reservas todavía vivas sobre
	// una PC que ya se guardó en su nuevo estado, o sea una cascada que quedó a
	// medias.
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
	}, cifradorDeTest(), eventbus.NewInMemoryEventBus())
}

// cifradorDeTest usa una clave fija: los tests no prueban el cifrado —eso lo
// hace internal/shared/secretos— sino que el servicio cifre antes de guardar y
// descifre al revelar.
func cifradorDeTest() *secretos.Cifrador {
	c, err := secretos.Nuevo("clave-de-test-para-las-cuentas-de-equipo")
	if err != nil {
		panic(err)
	}
	return c
}

// ── fakeRepo: cuentas de equipo ─────────────────────────────────────────

func (r *fakeRepo) CrearCuentaDeEquipo(_ context.Context, c *domain.CuentaDeEquipo) error {
	for _, existente := range r.cuentas {
		if existente.EquipoID == c.EquipoID && strings.EqualFold(existente.Usuario, c.Usuario) {
			return ErrCuentaDeEquipoDuplicada
		}
	}
	r.cuentas[c.ID] = c
	return nil
}

func (r *fakeRepo) BuscarCuentaDeEquipoPorID(_ context.Context, id string) (*domain.CuentaDeEquipo, error) {
	c, ok := r.cuentas[id]
	if !ok {
		return nil, ErrCuentaDeEquipoNoEncontrada
	}
	return c, nil
}

func (r *fakeRepo) GuardarCuentaDeEquipo(_ context.Context, c *domain.CuentaDeEquipo) error {
	if _, ok := r.cuentas[c.ID]; !ok {
		return ErrCuentaDeEquipoNoEncontrada
	}
	r.cuentas[c.ID] = c
	return nil
}

func (r *fakeRepo) BorrarCuentaDeEquipo(_ context.Context, id string) error {
	if _, ok := r.cuentas[id]; !ok {
		return ErrCuentaDeEquipoNoEncontrada
	}
	delete(r.cuentas, id)
	return nil
}

func (r *fakeRepo) ListarCuentasDeEquipo(_ context.Context, equipoID string) ([]*domain.CuentaDeEquipo, error) {
	var resultado []*domain.CuentaDeEquipo
	for _, c := range r.cuentas {
		if c.EquipoID == equipoID {
			resultado = append(resultado, c)
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].Usuario < resultado[j].Usuario })
	return resultado, nil
}

func (r *fakeRepo) ClasesDeCuentaUsadas(_ context.Context) ([]string, error) {
	vistas := map[string]bool{}
	var resultado []string
	for _, c := range r.cuentas {
		if !vistas[c.Clase] {
			vistas[c.Clase] = true
			resultado = append(resultado, c.Clase)
		}
	}
	sort.Strings(resultado)
	return resultado, nil
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

// repoConCarros deja creados los carros que estos tests dan por existentes.
//
// Hizo falta al validar el destino: antes se creaban equipos en carros que no
// existían, algo que la base real rechazaba igual por la clave foránea. El
// armado estaba describiendo un mundo imposible.
func repoConCarros(nombres ...string) *fakeRepo {
	repo := nuevoFakeRepo()
	for _, id := range nombres {
		repo.carros[id] = &domain.Carro{ID: id, Nombre: "Carro " + id}
	}
	return repo
}

func TestCrearEquipo_OK(t *testing.T) {
	svc := servicioSimple(repoConCarros("c1"))

	equipo, err := svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "5CD1234ABC", true, "i5", "8GB", "Windows 11", "Office")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if equipo.Identificador != 27 || equipo.Estado != domain.EstadoDisponible {
		t.Errorf("PC incorrecta: %+v", equipo)
	}
}

func TestCrearEquipo_IdentificadorDuplicadoEnMismoCarro_Error(t *testing.T) {
	repo := repoConCarros("c1")
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
	svc := servicioSimple(repoConCarros("c1", "c2"))

	_, err1 := svc.CrearEquipoDeCarro(context.Background(), "c1", 27, "SERIE-111", false, "", "", "", "")
	_, err2 := svc.CrearEquipoDeCarro(context.Background(), "c2", 27, "SERIE-222", false, "", "", "", "")

	if err1 != nil || err2 != nil {
		t.Fatalf("ninguna debería fallar: err1=%v err2=%v", err1, err2)
	}
}

// El bug que cerró esto: mover un equipo a un carro RETIRADO devolvía 200 y
// dejaba la máquina en un contenedor que ninguna pantalla lista —los listados
// de carros traen sólo los vivos—, sin estar ella misma dada de baja.
//
// La clave foránea no lo impedía: un carro dado de baja sigue siendo una fila
// válida.
func TestEditarEquipo_NoSePuedeMoverAUnCarroDadoDeBaja(t *testing.T) {
	repo := repoConCarros("c1", "c2")
	repo.carros["c2"].DadoDeBaja = true
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 1}
	svc := servicioSimple(repo)

	destino := "c2"
	_, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{CarroID: &destino})

	if !errors.Is(err, ErrCarroDadoDeBaja) {
		t.Fatalf("esperaba ErrCarroDadoDeBaja, obtuve %v", err)
	}
	if repo.equipos["pc1"].CarroID != "c1" {
		t.Errorf("el equipo se movió igual: quedó en %s", repo.equipos["pc1"].CarroID)
	}
}

// Y tampoco se puede crear uno adentro.
func TestCrearEquipoDeCarro_EnUnCarroDadoDeBaja(t *testing.T) {
	repo := repoConCarros("c1")
	repo.carros["c1"].DadoDeBaja = true
	svc := servicioSimple(repo)

	_, err := svc.CrearEquipoDeCarro(context.Background(), "c1", 1, "SERIE-X", false, "", "", "", "")

	if !errors.Is(err, ErrCarroDadoDeBaja) {
		t.Fatalf("esperaba ErrCarroDadoDeBaja, obtuve %v", err)
	}
}

func TestEditarEquipo_MoverDeCarro(t *testing.T) {
	repo := repoConCarros("c1", "c2")
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 1}
	svc := servicioSimple(repo)

	nuevoCarro := "c2"
	_, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{CarroID: &nuevoCarro})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["pc1"].CarroID != "c2" {
		t.Errorf("no se movió de carro: %s", repo.equipos["pc1"].CarroID)
	}
}

// Editar es la otra puerta por la que se escriben tipo y nombre, además del
// alta.
func TestEditarEquipo_EquipoSueltoNoPuedeQuedarSinNombre(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "PROYECTOR", Nombre: "Proyector Epson"}
	svc := servicioSimple(repo)

	vacio := ""
	_, err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{Nombre: &vacio})

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
	_, err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{Tipo: &espacios})

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
	if _, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{Nombre: &vacio}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["pc1"].Nombre != "" {
		t.Errorf("no se limpió: %q", repo.equipos["pc1"].Nombre)
	}
}

// ── El número de serie que se carga después ─────────────────────────────
//
// Los equipos sueltos que ya estaban cargados no tienen serie: sin poder
// editarla habría que darlos de baja y recrearlos solo para anotarla,
// perdiendo su historial de préstamos e incidencias.

func TestEditarEquipo_CargarNumeroDeSerieAUnSuelto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "NOTEBOOK", Nombre: "Notebook Dirección"}
	svc := servicioSimple(repo)

	serie := "  abc-123x "
	if _, err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{NumeroSerie: &serie}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.equipos["eq1"].NumeroSerie != "ABC-123X" {
		t.Fatalf("esperaba ABC-123X, obtuve %q", repo.equipos["eq1"].NumeroSerie)
	}
}

// El caso de haberla anotado mal: fuera de un carro se puede dejar sin serie.
func TestEditarEquipo_UnSueltoSiPuedeQuedarSinNumeroDeSerie(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "CARGADOR", Nombre: "Cargador 1", NumeroSerie: "XYZ-9"}
	svc := servicioSimple(repo)

	vacio := ""
	if _, err := svc.EditarEquipo(context.Background(), "eq1", EditarEquipoParams{NumeroSerie: &vacio}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["eq1"].NumeroSerie != "" {
		t.Errorf("no se limpió: %q", repo.equipos["eq1"].NumeroSerie)
	}
}

// Adentro de un carro no: el alta la exige, y dejar que una edición se la
// saque abriría por la puerta de atrás un estado que el alta prohíbe.
func TestEditarEquipo_UnaDeCarroNoPuedeQuedarSinNumeroDeSerie(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 3, NumeroSerie: "ABC-1"}
	svc := servicioSimple(repo)

	vacio := ""
	_, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{NumeroSerie: &vacio})

	if !errors.Is(err, domain.ErrNumeroSerieInvalido) {
		t.Fatalf("esperaba ErrNumeroSerieInvalido, obtuve %v", err)
	}
	if repo.equipos["pc1"].NumeroSerie != "ABC-1" {
		t.Errorf("no debería haberse tocado: %q", repo.equipos["pc1"].NumeroSerie)
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

// Un equipo fuera de servicio que se arregla vuelve a circulación, y volver
// nunca cancela nada: la cascada es de ida. El caso real fue una máquina que
// pasó a fuera de servicio por no tener batería y volvió cuando apareció una.
func TestCambiarEstadoEquipo_DesdeFueraDeServicio_VuelveSinCascada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoDisponible, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["pc1"].Estado != domain.EstadoDisponible {
		t.Errorf("estado = %s, esperaba DISPONIBLE", repo.equipos["pc1"].Estado)
	}
	if validador.llamado {
		t.Error("volver a circulación no debería disparar ninguna cascada")
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
	// antepone notification, que además ya nombró la máquina. Por eso el motivo
	// NO la nombra — ver motivoPorDefecto.
	esperado := "el equipo quedó fuera de servicio"
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
// de baja y lo único que falta es terminar de cancelar sus reservas.
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

func TestEditarIncidencia_MarcarEnviadaASoporte(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.incidencias["i1"] = &domain.Incidencia{ID: "i1", Estado: domain.IncidenciaEnReparacion}
	svc := servicioSimple(repo)

	err := svc.EditarIncidencia(context.Background(), "i1", EditarIncidenciaParams{MarcarEnviadaASoporte: true})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !repo.incidencias["i1"].EnviadoASoporte {
		t.Error("EnviadoASoporte debería quedar true")
	}
	if repo.incidencias["i1"].Estado != domain.IncidenciaEnviadaASoporte {
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

// ── Cascada a medias: el reintento tiene que poder terminarla ─────────── La
// cascada de RF-03.8/03.9 no es atómica con el guardado de la PC (cruza a
// reservation, que abre su propia transacción).

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

// Cambiar el diagnóstico de un equipo que ya está fuera de circulación sí es
// una transición, y dispara la cascada como cualquier otra salida de
// DISPONIBLE: si le quedaban reservas vivas, se cancelan.
func TestCambiarEstadoEquipo_DeFueraDeServicioAMantenimiento_CancelaLoQueQuede(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Estado: domain.EstadoFueraDeServicio}
	validador := &fakeValidadorReservas{tieneFuturas: true}
	svc := nuevoServicioDeTest(repo, validador)

	_, err := svc.CambiarEstadoEquipo(context.Background(), "pc1", domain.EstadoEnMantenimiento, nil)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !validador.llamado {
		t.Error("salir de circulación siempre cancela las reservas futuras que queden")
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
	// El reintento tiene que mandar el mismo motivo que el primer intento — es
	// lo que el docente lee en la notificación (RF-05.3).
	if validador.motivoRecibido != "el equipo fue dado de baja del inventario" {
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

// El error del segundo paso tiene que decir que el primero sí se aplicó y que
// reintentar completa lo que falta — si no, quien lo lee no sabe en qué
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

// TestCambiarEstado_ElMotivoNoRepiteElNombreDelEquipo fija la segunda mitad de
// un arreglo real. El aviso completo se arma en notification y YA nombra la
// máquina; cuando el motivo la nombraba también, el docente leía "Tu reserva
// del 28/08 (PC 7 del Carro 1) fue cancelada: PC 7 del Carro 1 pasó a
// FUERA_DE_SERVICIO", y en el correo lo mismo dos renglones seguidos.
//
// Quién es la máquina lo resuelve reservation al armar el aviso
// (EtiquetasDeEquipos, que sí trae el carro): acá alcanza con la razón.
func TestCambiarEstado_ElMotivoNoRepiteElNombreDelEquipo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro EDUTEC"}
	repo.equipos["eq1"] = &domain.Equipo{
		ID: "eq1", CarroID: "carro1", Identificador: 7, Estado: domain.EstadoDisponible,
	}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	if _, err := svc.CambiarEstadoEquipo(context.Background(), "eq1", domain.EstadoFueraDeServicio, nil); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if validador.motivoRecibido != "el equipo quedó fuera de servicio" {
		t.Errorf("obtuve: %q", validador.motivoRecibido)
	}
	if strings.Contains(validador.motivoRecibido, "PC 7") {
		t.Errorf("el motivo no tiene que nombrar la máquina, obtuve: %q", validador.motivoRecibido)
	}
}

// Un motivo escrito por el Admin manda sobre el de por defecto: es lo único
// del aviso que escribió una persona.
func TestCambiarEstado_MotivoDelAdmin_MandaSobreElDeDefecto(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{
		ID: "eq1", Nombre: "Proyector Benq", Estado: domain.EstadoDisponible,
	}
	validador := &fakeValidadorReservas{}
	svc := nuevoServicioDeTest(repo, validador)

	suyo := "se lo llevó el service, vuelve la semana que viene"
	if _, err := svc.CambiarEstadoEquipo(context.Background(), "eq1", domain.EstadoFueraDeServicio, &suyo); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if validador.motivoRecibido != suyo {
		t.Errorf("obtuve: %q", validador.motivoRecibido)
	}
}

func (r *fakeRepo) BuscarLicenciasPorIDs(ctx context.Context, ids []string) (map[string]*domain.LicenciaSoftware, error) {
	licencias := make(map[string]*domain.LicenciaSoftware, len(ids))
	for _, id := range ids {
		if l, existe := r.licencias[id]; existe {
			licencias[id] = l
		}
	}
	return licencias, nil
}

func (r *fakeRepo) ListarPreferenciasHuerfanas(ctx context.Context) ([]*PreferenciaHuerfana, error) {
	// Huérfana = su nombre de materia no cruza con ninguna materia cargada. El
	// fake no tiene tabla de materias, así que devuelve lo que el test le dejó
	// preparado.
	return r.preferenciasHuerfanas, nil
}

func (r *fakeRepo) ContarPreferenciasQueDejarianDeAplicar(ctx context.Context, materiaNombre string) (int, error) {
	n := 0
	for _, p := range r.preferencias {
		if texto.SonElMismo(p.MateriaNombre, materiaNombre) {
			n++
		}
	}
	return n, nil
}

// ── Reactivar: deshacer una baja ────────────────────────────────────────
//
// Los conflictos de unicidad —el zócalo, el nombre suelto y el número de serie
// que otro pudo haberse llevado mientras el equipo estaba afuera— no se prueban
// acá: los decide un índice parcial de la base y el fake no los tiene. Viven en
// infrastructure/reactivar_test.go, contra Postgres de verdad.

func TestReactivarEquipo_LoDevuelveAlInventarioYLimpiaLaFecha(t *testing.T) {
	repo := nuevoFakeRepo()
	baja := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro 1"}
	repo.equipos["pc1"] = &domain.Equipo{
		ID: "pc1", CarroID: "carro1", Identificador: 7,
		Estado: domain.EstadoFueraDeServicio, DadoDeBaja: true, FechaBaja: &baja,
	}
	svc := servicioSimple(repo)

	if err := svc.ReactivarEquipo(context.Background(), "pc1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	pc := repo.equipos["pc1"]
	if pc.DadoDeBaja {
		t.Error("el equipo tenía que volver al inventario")
	}
	if pc.FechaBaja != nil {
		t.Errorf("la fecha de baja tenía que limpiarse, quedó %v", pc.FechaBaja)
	}
	// El estado NO se toca: la baja no lo había cambiado, así que la máquina
	// vuelve rota si estaba rota. Devolverla DISPONIBLE sería inventar que
	// alguien la arregló.
	if pc.Estado != domain.EstadoFueraDeServicio {
		t.Errorf("el estado no tenía que cambiar, quedó %s", pc.Estado)
	}
}

func TestReactivarEquipo_QueNoEstabaDeBaja_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro 1"}
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "carro1", Identificador: 7}
	svc := servicioSimple(repo)

	err := svc.ReactivarEquipo(context.Background(), "pc1")

	if !errors.Is(err, domain.ErrEquipoNoEstaDadoDeBaja) {
		t.Fatalf("esperaba ErrEquipoNoEstaDadoDeBaja, obtuve %v", err)
	}
}

// El caso que la base no puede ver: el carro del equipo se retiró mientras la
// máquina estaba de baja. Reactivarla ahí adentro la dejaría invisible —ningún
// selector lista un carro retirado— sin estar dada de baja, que es peor que
// seguir de baja.
func TestReactivarEquipo_ConSuCarroRetirado_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro 1", DadoDeBaja: true}
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "carro1", Identificador: 7, DadoDeBaja: true}
	svc := servicioSimple(repo)

	err := svc.ReactivarEquipo(context.Background(), "pc1")

	if !errors.Is(err, ErrCarroDadoDeBaja) {
		t.Fatalf("esperaba ErrCarroDadoDeBaja, obtuve %v", err)
	}
	if !repo.equipos["pc1"].DadoDeBaja {
		t.Error("el equipo no tenía que reactivarse")
	}
}

// Un equipo suelto no tiene carro, así que no hay nada que verificar antes.
func TestReactivarEquipo_Suelto_NoMiraNingunCarro(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["proyector"] = &domain.Equipo{
		ID: "proyector", Tipo: "PROYECTOR", Nombre: "Proyector 1", DadoDeBaja: true,
	}
	svc := servicioSimple(repo)

	if err := svc.ReactivarEquipo(context.Background(), "proyector"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.equipos["proyector"].DadoDeBaja {
		t.Error("el equipo suelto tenía que volver al inventario")
	}
}

func TestReactivarCarro_LoDevuelveACirculacion(t *testing.T) {
	repo := nuevoFakeRepo()
	baja := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro 1", DadoDeBaja: true, FechaBaja: &baja}
	svc := servicioSimple(repo)

	if err := svc.ReactivarCarro(context.Background(), "carro1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	c := repo.carros["carro1"]
	if c.DadoDeBaja || c.FechaBaja != nil {
		t.Errorf("el carro tenía que volver a circulación sin fecha de baja: %+v", c)
	}
}

func TestReactivarCarro_QueNoEstabaDeBaja_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["carro1"] = &domain.Carro{ID: "carro1", Nombre: "Carro 1"}
	svc := servicioSimple(repo)

	err := svc.ReactivarCarro(context.Background(), "carro1")

	if !errors.Is(err, domain.ErrCarroNoEstaDadoDeBaja) {
		t.Fatalf("esperaba ErrCarroNoEstaDadoDeBaja, obtuve %v", err)
	}
}

// Los retirados sólo salen si se los pide: el selector de "dónde va este
// equipo" se llena con esta misma lista.
func TestListarCarros_LosRetiradosSoloSiSePiden(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["vivo"] = &domain.Carro{ID: "vivo", Nombre: "Carro 1"}
	repo.carros["retirado"] = &domain.Carro{ID: "retirado", Nombre: "Carro viejo", DadoDeBaja: true}
	svc := servicioSimple(repo)

	soloVivos, err := svc.ListarCarros(context.Background(), false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(soloVivos) != 1 || soloVivos[0].ID != "vivo" {
		t.Errorf("esperaba sólo el carro en circulación, obtuve %+v", soloVivos)
	}

	todos, err := svc.ListarCarros(context.Background(), true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("esperaba los dos carros, obtuve %d", len(todos))
	}
}

// ── Qué deja registrado una edición ─────────────────────────────────────
//
// Hasta que esto existió, lo único auditado de una edición era el cambio de
// carro: el nombre, el tipo, el número de serie y si el equipo es reservable
// cambiaban sin dejar rastro, contra lo que dice RF-00.2.

func TestEditarEquipo_DevuelveLoQueCambioConSusDosPuntas(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{
		ID: "pc1", Tipo: "PROYECTOR", Nombre: "Proyector viejo",
		NumeroSerie: "ABC123", Reservable: true, RAM: "8GB",
	}
	svc := servicioSimple(repo)

	nombre, reservable := "Proyector del SUM", false
	cambios, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{
		Nombre: &nombre, Reservable: &reservable,
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(cambios) != 2 {
		t.Fatalf("esperaba 2 campos cambiados, obtuve %d: %+v", len(cambios), cambios)
	}
	if c := cambios["nombre"]; c.Antes != "Proyector viejo" || c.Despues != "Proyector del SUM" {
		t.Errorf("el cambio de nombre tenía que traer las dos puntas: %+v", c)
	}
	if c := cambios["reservable"]; c.Antes != true || c.Despues != false {
		t.Errorf("el cambio de reservable tenía que traer las dos puntas: %+v", c)
	}
	// Lo que no se tocó no aparece: el registro dice qué cambió, no qué se mandó.
	if _, hay := cambios["ram"]; hay {
		t.Error("la RAM no cambió y no tenía que figurar")
	}
}

// Un PATCH que manda los mismos datos no es una edición. Sin esto, cada
// guardado sin cambios dejaría una entrada que dice «editó» sin decir qué.
func TestEditarEquipo_SinCambiosNoRegistraNada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", Tipo: "PROYECTOR", Nombre: "Proyector", Reservable: true}
	svc := servicioSimple(repo)

	mismoNombre, mismoReservable := "Proyector", true
	cambios, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{
		Nombre: &mismoNombre, Reservable: &mismoReservable,
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cambios) != 0 {
		t.Errorf("nada cambió y sin embargo registró: %+v", cambios)
	}
}

// El campo que alguien agregue mañana a la edición entra solo en la auditoría,
// porque el registro sale de comparar dos fotos y no de anotar en cada rama.
// Esto lo fija: la ficha técnica nunca se anotó a mano y aun así se registra.
func TestEditarEquipo_RegistraTambienLaFichaTecnica(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["c1"] = &domain.Carro{ID: "c1", Nombre: "Carro 1"}
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 3, RAM: "4GB", CPU: "i3"}
	svc := servicioSimple(repo)

	ram := "8GB"
	cambios, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{RAM: &ram})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if c := cambios["ram"]; c.Antes != "4GB" || c.Despues != "8GB" {
		t.Errorf("el cambio de RAM tenía que quedar registrado: %+v", cambios)
	}
}

// El cambio de carro tiene su propia acción en el catálogo, así que no se
// cuenta dos veces.
func TestEditarEquipo_ElCarroNoSeDuplicaEnElDetalle(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.carros["c1"] = &domain.Carro{ID: "c1", Nombre: "Carro 1"}
	repo.carros["c2"] = &domain.Carro{ID: "c2", Nombre: "Carro 2"}
	repo.equipos["pc1"] = &domain.Equipo{ID: "pc1", CarroID: "c1", Identificador: 3}
	svc := servicioSimple(repo)

	destino := "c2"
	cambios, err := svc.EditarEquipo(context.Background(), "pc1", EditarEquipoParams{CarroID: &destino})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cambios) != 0 {
		t.Errorf("el carro lo registra EQUIPO_MOVIDO_DE_CARRO, no este detalle: %+v", cambios)
	}
}

// El historial de fallas de una máquina tiene tope, con el mismo criterio y el
// mismo número que el de entregas: es una pantalla para mirar los últimos
// movimientos, no un reporte. Sin él devolvía todo lo acumulado desde que el
// equipo entró al inventario.
func TestListarIncidenciasPorEquipo_TieneTope(t *testing.T) {
	repo := nuevoFakeRepo()
	for i := 0; i < maxIncidenciasDeEquipo+20; i++ {
		id := fmt.Sprintf("i%d", i)
		repo.incidencias[id] = &domain.Incidencia{
			ID: id, EquipoID: "pc1", Descripcion: "No arranca",
			Gravedad: domain.GravedadLeve, Estado: domain.IncidenciaAbierta,
		}
	}
	svc := servicioSimple(repo)

	incidencias, err := svc.ListarIncidenciasPorEquipo(context.Background(), "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(incidencias) != maxIncidenciasDeEquipo {
		t.Errorf("esperaba el tope de %d, obtuve %d", maxIncidenciasDeEquipo, len(incidencias))
	}
}
