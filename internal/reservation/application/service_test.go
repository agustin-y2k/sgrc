package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── fakeRepo ────────────────────────────────────────────────────────────

type fakeRepo struct {
	grupos    map[string]*domain.ReservaGrupo
	reservas  map[string]*domain.Reserva
	reglas    map[string]*domain.ReglaRecurrencia
	prestamos map[string]*domain.Prestamo

	errCrearReserva error
	pcsDisponibles  []PCDisponible
	// pcsDadasDeBaja imita lo único que el servicio le pregunta a
	// inventory antes de entregar una máquina.
	pcsDadasDeBaja map[string]bool

	// Lo que el barrido lee por JOIN contra pc y usuario. En los tests se
	// carga a mano: acá no hay base que lo resuelva.
	identificadorDePC map[string]int
	contactoDeUsuario map[string][2]string // usuarioID → {nombre, email}
	// Las marcas del barrido, que en la base son columnas.
	recordatorioEnviado map[string]time.Time
	avisoPCNoDisponible map[string]time.Time
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		grupos:         make(map[string]*domain.ReservaGrupo),
		reservas:       make(map[string]*domain.Reserva),
		reglas:         make(map[string]*domain.ReglaRecurrencia),
		prestamos:      make(map[string]*domain.Prestamo),
		pcsDadasDeBaja: make(map[string]bool),

		identificadorDePC:   make(map[string]int),
		contactoDeUsuario:   make(map[string][2]string),
		recordatorioEnviado: make(map[string]time.Time),
		avisoPCNoDisponible: make(map[string]time.Time),
	}
}

// ── Lo que lee el barrido ───────────────────────────────────────────────

// ReservasAVigilar reproduce la consulta real: las CONFIRMADA de hoy y
// mañana, con el contacto del docente y el estado de custodia de cada PC.
//
// El cruce con los préstamos va por pc_id y no por reserva_id, igual que en
// SQL: si la máquina salió por una entrega espontánea, igual está afuera.
func (r *fakeRepo) ReservasAVigilar(ctx context.Context, hoy time.Time) ([]ReservaParaVigilar, error) {
	desde := diaDe(hoy)
	hasta := desde.AddDate(0, 0, 1)

	var resultado []ReservaParaVigilar
	for _, res := range r.enOrden() {
		if res.Estado != domain.ReservaConfirmada {
			continue
		}
		dia := diaDe(res.Fecha)
		if dia.Before(desde) || dia.After(hasta) {
			continue
		}

		v := ReservaParaVigilar{
			ReservaID:       res.ID,
			GrupoID:         res.ReservaGrupoID,
			PCID:            res.PCID,
			PCIdentificador: r.identificadorDePC[res.PCID],
			Etiqueta:        fmt.Sprintf("PC %d", r.identificadorDePC[res.PCID]),
			Fecha:           res.Fecha,
			HoraInicio:      res.HoraInicio,
			HoraFin:         res.HoraFin,
			Tipo:            res.Tipo,
			MateriaNombre:   res.MateriaID,
		}
		if res.NombreDocenteSnapshot != nil {
			v.DocenteNombre = *res.NombreDocenteSnapshot
		}
		if res.ReservaGrupoID != nil {
			if g, ok := r.grupos[*res.ReservaGrupoID]; ok {
				v.DocenteID = g.CreadoPor
				if g.CreadoPor != nil {
					if c, ok := r.contactoDeUsuario[*g.CreadoPor]; ok {
						v.DocenteNombre, v.DocenteEmail = c[0], c[1]
					}
				}
			}
			_, v.RecordatorioEnviado = r.recordatorioEnviado[*res.ReservaGrupoID]
		}
		_, v.AvisoPCNoDisponibleEnviado = r.avisoPCNoDisponible[res.ID]

		for _, p := range r.prestamos {
			if p.PCID == res.PCID && p.EstaAbierto() {
				v.PCAfuera = true
				v.PCDeboVolverA = p.DevolucionEstimada
				break
			}
		}
		resultado = append(resultado, v)
	}
	return resultado, nil
}

func (r *fakeRepo) PrestamosAVigilar(ctx context.Context) ([]PrestamoParaVigilar, error) {
	var resultado []PrestamoParaVigilar
	for _, p := range r.prestamosEnOrden() {
		if !p.EstaAbierto() {
			continue
		}
		v := PrestamoParaVigilar{
			Prestamo:        p,
			PCIdentificador: r.identificadorDePC[p.PCID],
			Etiqueta:        fmt.Sprintf("PC %d", r.identificadorDePC[p.PCID]),
		}
		if p.EntregadoAUsuarioID != nil {
			if c, ok := r.contactoDeUsuario[*p.EntregadoAUsuarioID]; ok {
				v.Email = c[1]
			}
		}
		resultado = append(resultado, v)
	}
	return resultado, nil
}

func (r *fakeRepo) ProximaReservaDePC(ctx context.Context, pcID string, desde time.Time) (*ProximaReserva, error) {
	var mejor *domain.Reserva
	for _, res := range r.enOrden() {
		if res.PCID != pcID || res.Estado != domain.ReservaConfirmada {
			continue
		}
		if mejor == nil || res.Fecha.Before(mejor.Fecha) ||
			(res.Fecha.Equal(mejor.Fecha) && res.HoraInicio < mejor.HoraInicio) {
			mejor = res
		}
	}
	if mejor == nil {
		return nil, nil
	}
	p := &ProximaReserva{Fecha: mejor.Fecha, HoraInicio: mejor.HoraInicio}
	if mejor.NombreDocenteSnapshot != nil {
		p.Nombre = *mejor.NombreDocenteSnapshot
	}
	if mejor.ReservaGrupoID != nil {
		if g, ok := r.grupos[*mejor.ReservaGrupoID]; ok && g.CreadoPor != nil {
			p.UsuarioID = *g.CreadoPor
			if c, ok := r.contactoDeUsuario[*g.CreadoPor]; ok {
				p.Nombre, p.Email = c[0], c[1]
			}
		}
	}
	return p, nil
}

func (r *fakeRepo) MarcarRecordatorioEnviado(ctx context.Context, grupoID string, ahora time.Time) error {
	r.recordatorioEnviado[grupoID] = ahora
	return nil
}

func (r *fakeRepo) MarcarAvisoPCNoDisponible(ctx context.Context, reservaID string, ahora time.Time) error {
	r.avisoPCNoDisponible[reservaID] = ahora
	return nil
}

func (r *fakeRepo) MarcarDemoraAvisada(ctx context.Context, prestamoID string, ahora time.Time) error {
	if p, ok := r.prestamos[prestamoID]; ok {
		p.AvisadoDemoraEn = &ahora
	}
	return nil
}

func (r *fakeRepo) MarcarCierreAvisado(ctx context.Context, prestamoID string, jornada time.Time) error {
	if p, ok := r.prestamos[prestamoID]; ok {
		d := diaDe(jornada)
		p.AvisadoCierrePara = &d
	}
	return nil
}

func diaDe(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// EnTransaccion imita el todo-o-nada de Postgres: saca una copia del
// estado antes de correr fn y la restaura si fn falla. Sin esto el fake
// daría verde en escenarios que en la base real dejarían filas a medias.
func (r *fakeRepo) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	gruposAntes := make(map[string]*domain.ReservaGrupo, len(r.grupos))
	for k, v := range r.grupos {
		copia := *v
		gruposAntes[k] = &copia
	}
	reservasAntes := make(map[string]*domain.Reserva, len(r.reservas))
	for k, v := range r.reservas {
		copia := *v
		reservasAntes[k] = &copia
	}
	reglasAntes := make(map[string]*domain.ReglaRecurrencia, len(r.reglas))
	for k, v := range r.reglas {
		copia := *v
		reglasAntes[k] = &copia
	}
	prestamosAntes := make(map[string]*domain.Prestamo, len(r.prestamos))
	for k, v := range r.prestamos {
		copia := *v
		prestamosAntes[k] = &copia
	}

	if err := fn(r); err != nil {
		r.grupos = gruposAntes
		r.reservas = reservasAntes
		r.reglas = reglasAntes
		r.prestamos = prestamosAntes
		return err
	}
	return nil
}

// ── Préstamos ───────────────────────────────────────────────────────────

// CrearPrestamo reproduce el índice único parcial de la migración 013: una
// PC no puede tener dos préstamos abiertos. Sin eso el fake daría verde en
// el escenario que la base real rechaza.
func (r *fakeRepo) CrearPrestamo(ctx context.Context, p *domain.Prestamo) error {
	for _, existente := range r.prestamos {
		if existente.PCID == p.PCID && existente.EstaAbierto() {
			return ErrPCYaPrestada
		}
	}
	copia := *p
	r.prestamos[p.ID] = &copia
	return nil
}

func (r *fakeRepo) BuscarPrestamoPorID(ctx context.Context, id string) (*domain.Prestamo, error) {
	p, ok := r.prestamos[id]
	if !ok {
		return nil, ErrPrestamoNoEncontrado
	}
	copia := *p
	return &copia, nil
}

func (r *fakeRepo) GuardarPrestamo(ctx context.Context, p *domain.Prestamo) error {
	if _, ok := r.prestamos[p.ID]; !ok {
		return ErrPrestamoNoEncontrado
	}
	copia := *p
	r.prestamos[p.ID] = &copia
	return nil
}

func (r *fakeRepo) BuscarPrestamoAbiertoDePC(ctx context.Context, pcID string) (*domain.Prestamo, error) {
	for _, p := range r.prestamos {
		if p.PCID == pcID && p.EstaAbierto() {
			copia := *p
			return &copia, nil
		}
	}
	return nil, ErrPrestamoNoEncontrado
}

func (r *fakeRepo) ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error) {
	var resultado []*PrestamoDetallado
	for _, p := range r.prestamosEnOrden() {
		if p.EstaAbierto() {
			resultado = append(resultado, &PrestamoDetallado{Prestamo: p})
		}
	}
	return resultado, nil
}

func (r *fakeRepo) ListarPrestamosDePC(ctx context.Context, pcID string, limite int) ([]*PrestamoDetallado, error) {
	var resultado []*PrestamoDetallado
	for _, p := range r.prestamosEnOrden() {
		if p.PCID == pcID && len(resultado) < limite {
			resultado = append(resultado, &PrestamoDetallado{Prestamo: p})
		}
	}
	return resultado, nil
}

// prestamosEnOrden da un recorrido estable: iterar un map en Go es aleatorio
// y haría que un test que compara listas falle una vez cada tanto.
func (r *fakeRepo) prestamosEnOrden() []*domain.Prestamo {
	ids := make([]string, 0, len(r.prestamos))
	for id := range r.prestamos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	resultado := make([]*domain.Prestamo, 0, len(ids))
	for _, id := range ids {
		resultado = append(resultado, r.prestamos[id])
	}
	return resultado
}

func (r *fakeRepo) ListarReservas(ctx context.Context, f FiltroReservas) ([]ReservaDetallada, int, error) {
	var resultado []ReservaDetallada
	for _, res := range r.enOrden() {
		if f.CreadoPor != nil && (res.CreadoPor == nil || *res.CreadoPor != *f.CreadoPor) {
			continue
		}
		if f.PCID != nil && res.PCID != *f.PCID {
			continue
		}
		if f.MateriaID != nil && (res.MateriaID == nil || *res.MateriaID != *f.MateriaID) {
			continue
		}
		if !f.IncluirCanceladas && res.Estado == domain.ReservaCancelada {
			continue
		}
		// Los nombres los resuelve un JOIN en el repo real; acá alcanza con
		// valores estables para que los tests puedan afirmar sobre ellos.
		resultado = append(resultado, ReservaDetallada{
			Reserva:         res,
			PCIdentificador: 1,
			CarroNombre:     "Carro de test",
			MateriaNombre:   "Matemáticas",
			CursoNombre:     "1°A",
		})
	}
	total := len(resultado)
	return paginar(resultado, f.Pagina), total, nil
}

// enOrden recorre las reservas con un orden estable (el repo real ordena por
// fecha, hora e identificador de PC): sobre el map pelado, dos llamadas
// devolvían las filas en distinto orden y con LIMIT/OFFSET eso convierte
// cualquier test de paginación en una moneda al aire.
func (r *fakeRepo) enOrden() []*domain.Reserva {
	ordenadas := make([]*domain.Reserva, 0, len(r.reservas))
	for _, res := range r.reservas {
		ordenadas = append(ordenadas, res)
	}
	sort.Slice(ordenadas, func(i, j int) bool {
		if !ordenadas[i].Fecha.Equal(ordenadas[j].Fecha) {
			return ordenadas[i].Fecha.Before(ordenadas[j].Fecha)
		}
		if ordenadas[i].HoraInicio != ordenadas[j].HoraInicio {
			return ordenadas[i].HoraInicio < ordenadas[j].HoraInicio
		}
		return ordenadas[i].ID < ordenadas[j].ID
	})
	return ordenadas
}

// paginar aplica la misma ventana que el LIMIT/OFFSET del repo real.
func paginar(reservas []ReservaDetallada, p paginacion.Pagina) []ReservaDetallada {
	if p.Tamanio <= 0 {
		return reservas
	}
	desde := p.Offset()
	if desde >= len(reservas) {
		return nil
	}
	hasta := desde + p.Limit()
	if hasta > len(reservas) {
		hasta = len(reservas)
	}
	return reservas[desde:hasta]
}

func (r *fakeRepo) CalendarioDePC(ctx context.Context, pcID string, desde, hasta time.Time) ([]BloqueCalendario, error) {
	var resultado []BloqueCalendario
	for _, res := range r.reservas {
		if res.PCID != pcID || res.Estado == domain.ReservaCancelada {
			continue
		}
		if res.Fecha.Before(desde) || res.Fecha.After(hasta) {
			continue
		}
		resultado = append(resultado, BloqueCalendario{Reserva: res, MateriaNombre: "Matemáticas", CursoNombre: "1°A"})
	}
	return resultado, nil
}

func (r *fakeRepo) CrearReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	r.grupos[g.ID] = g
	return nil
}
func (r *fakeRepo) BuscarReservaGrupoPorID(ctx context.Context, id string) (*domain.ReservaGrupo, error) {
	g, ok := r.grupos[id]
	if !ok {
		return nil, ErrReservaGrupoNoEncontrado
	}
	return g, nil
}
func (r *fakeRepo) GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	r.grupos[g.ID] = g
	return nil
}
func (r *fakeRepo) CrearReserva(ctx context.Context, res *domain.Reserva) error {
	if r.errCrearReserva != nil {
		return r.errCrearReserva
	}
	r.reservas[res.ID] = res
	return nil
}
func (r *fakeRepo) BuscarReservaPorID(ctx context.Context, id string) (*domain.Reserva, error) {
	res, ok := r.reservas[id]
	if !ok {
		return nil, ErrReservaNoEncontrada
	}
	return res, nil
}
func (r *fakeRepo) GuardarReserva(ctx context.Context, res *domain.Reserva) error {
	r.reservas[res.ID] = res
	return nil
}
func (r *fakeRepo) ListarReservasPorGrupo(ctx context.Context, reservaGrupoID string) ([]*domain.Reserva, error) {
	var resultado []*domain.Reserva
	for _, res := range r.reservas {
		if res.ReservaGrupoID != nil && *res.ReservaGrupoID == reservaGrupoID {
			resultado = append(resultado, res)
		}
	}
	return resultado, nil
}

// ListarReservasFuturasDePC devuelve ORDENADO por fecha y hora, como el repo
// real: quien llama puede necesitar LA PRÓXIMA, no una cualquiera. Iterar el
// map sin ordenar daba un resultado distinto en cada corrida y escondía esa
// dependencia.
//
// El filtro temporal (que la reserva no haya terminado) NO se reproduce acá
// —lo verifica el test de infrastructure contra Postgres—, así que los tests
// de este paquete usan fechas futuras a propósito.
func (r *fakeRepo) ListarReservasFuturasDePC(ctx context.Context, pcID string, desde time.Time) ([]*domain.Reserva, error) {
	var resultado []*domain.Reserva
	for _, res := range r.reservas {
		if res.PCID == pcID {
			resultado = append(resultado, res)
		}
	}
	sort.Slice(resultado, func(i, j int) bool {
		if !resultado[i].Fecha.Equal(resultado[j].Fecha) {
			return resultado[i].Fecha.Before(resultado[j].Fecha)
		}
		return resultado[i].HoraInicio < resultado[j].HoraInicio
	})
	return resultado, nil
}
func (r *fakeRepo) ListarReservasFuturasDeMateria(ctx context.Context, materiaID string, desde time.Time) ([]*domain.Reserva, error) {
	var resultado []*domain.Reserva
	for _, res := range r.reservas {
		if res.MateriaID != nil && *res.MateriaID == materiaID {
			resultado = append(resultado, res)
		}
	}
	return resultado, nil
}
func (r *fakeRepo) EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (int, int, error) {
	// El fake no modela la relación ciclo→materia→grupo/reserva (viviría
	// del lado de academic), así que solo se usa para confirmar que el
	// método existe y es invocable desde los tests que ejercitan la
	// cascada — el comportamiento real se prueba en infrastructure/
	// contra Postgres de verdad, donde sí existen esas tablas.
	return 0, 0, nil
}
func (r *fakeRepo) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	return nil, nil
}
func (r *fakeRepo) ListarPCsDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]PCDisponible, error) {
	return r.pcsDisponibles, nil
}

func (r *fakeRepo) CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error {
	r.reglas[regla.ID] = regla
	return nil
}
func (r *fakeRepo) AsignarPCsARegla(ctx context.Context, reglaID string, pcIDs []string) error {
	return nil
}
func (r *fakeRepo) ListarGruposFuturosDeRegla(ctx context.Context, reglaID string, desde time.Time) ([]*domain.ReservaGrupo, error) {
	var resultado []*domain.ReservaGrupo
	for _, g := range r.grupos {
		if g.ReglaRecurrenciaID != nil && *g.ReglaRecurrenciaID == reglaID && g.Fecha.After(desde) {
			resultado = append(resultado, g)
		}
	}
	return resultado, nil
}

// ── fakes de los puertos ────────────────────────────────────────────────

type fakeValidadorMateria struct {
	asignado bool
	// archivada simula una materia de un ciclo ya cerrado (RF-04.1).
	archivada bool
}

func (f *fakeValidadorMateria) MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error) {
	return !f.archivada, nil
}

func (f *fakeValidadorMateria) DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error) {
	return f.asignado, nil
}

type fakeValidadorPC struct {
	disponible         bool
	errIdentificadores error
	// fueraDelInventario: PCs dadas de baja. Es lo único que distingue
	// "no se puede reservar" de "no se puede ni entregar".
	fueraDelInventario map[string]bool
}

func (f *fakeValidadorPC) PCDisponibleParaReservar(ctx context.Context, pcID string) (bool, error) {
	return f.disponible, nil
}

// PCEstaEnInventario es más laxo: una PC en mantenimiento no se puede
// reservar pero sí se le puede entregar al técnico.
func (f *fakeValidadorPC) PCEstaEnInventario(ctx context.Context, pcID string) (bool, error) {
	return !f.fueraDelInventario[pcID], nil
}

// EtiquetasDeEquipos: en los tests las PCs se llaman "pc1", "pc2"… así que
// el número visible sale del sufijo. Alcanza para verificar que el aviso
// nombre los equipos correctos.
func (f *fakeValidadorPC) EtiquetasDeEquipos(ctx context.Context, pcIDs []string) (map[string]string, error) {
	if f.errIdentificadores != nil {
		return nil, f.errIdentificadores
	}
	m := make(map[string]string, len(pcIDs))
	for _, id := range pcIDs {
		var n int
		if _, err := fmt.Sscanf(id, "pc%d", &n); err == nil {
			m[id] = fmt.Sprintf("PC %d", n)
		}
	}
	return m, nil
}

type fakeObtenedorNombre struct{ nombre string }

func (f *fakeObtenedorNombre) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	return f.nombre, nil
}

var contadorID int

func idSecuencial() string {
	contadorID++
	return fmt.Sprintf("id-%d", contadorID)
}

func nuevoServicioDeTest(repo Repo) *Service {
	contadorID = 0
	return NewService(
		repo,
		&fakeValidadorMateria{asignado: true},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada Lovelace"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) }, // un lunes
		eventbus.NewInMemoryEventBus(),                                           // el bus real — ya está probado, no hace falta fakearlo
	)
}

func fecha(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

// ── CrearReserva ────────────────────────────────────────────────────────

func TestCrearReserva_OK(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if grupo.Estado != domain.GrupoConfirmada {
		t.Errorf("estado inicial incorrecto: %s", grupo.Estado)
	}
	if len(reservas) != 2 {
		t.Fatalf("esperaba 2 reservas, obtuve %d", len(reservas))
	}
	for _, r := range reservas {
		if r.NombreDocenteSnapshot == nil || *r.NombreDocenteSnapshot != "Ada Lovelace" {
			t.Errorf("snapshot de nombre incorrecto: %+v", r)
		}
	}
}

// La semana lectiva es de lunes a viernes: no se reserva el fin de semana.
// El 2026-03-14 es sábado y el 2026-03-15 domingo.
func TestCrearReserva_FinDeSemana_Error(t *testing.T) {
	for nombre, dia := range map[string]time.Time{
		"sábado":  fecha(2026, 3, 14),
		"domingo": fecha(2026, 3, 15),
	} {
		svc := nuevoServicioDeTest(nuevoFakeRepo())

		_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
			dia, 8*time.Hour, 9*time.Hour, []string{"pc1"})

		if !errors.Is(err, domain.ErrDiaNoLectivo) {
			t.Errorf("%s: esperaba ErrDiaNoLectivo, obtuve %v", nombre, err)
		}
	}
}

// El reloj de nuevoServicioDeTest es el lunes 2/3/2026 a las 12:00, así que
// todo lo que termine antes de esa hora ese día ya pasó.
func TestCrearReserva_EnElPasado_Error(t *testing.T) {
	casos := map[string]struct {
		dia         time.Time
		inicio, fin time.Duration
	}{
		"un día que ya pasó":       {fecha(2026, 2, 27), 8 * time.Hour, 9 * time.Hour},
		"hoy pero ya terminó":      {fecha(2026, 3, 2), 8 * time.Hour, 9 * time.Hour},
		"hoy, termina justo ahora": {fecha(2026, 3, 2), 10 * time.Hour, 12 * time.Hour},
	}
	for nombre, c := range casos {
		svc := nuevoServicioDeTest(nuevoFakeRepo())

		_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
			c.dia, c.inicio, c.fin, []string{"pc1"})

		if !errors.Is(err, domain.ErrReservaEnElPasado) {
			t.Errorf("%s: esperaba ErrReservaEnElPasado, obtuve %v", nombre, err)
		}
	}
}

// La clase que arrancó hace media hora sigue siendo reservable: el criterio
// es "todavía no terminó", el mismo que usa la cascada de cancelación, no
// "todavía no empezó".
func TestCrearReserva_HoyEnCurso_SeAcepta(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 2), 11*time.Hour+30*time.Minute, 13*time.Hour, []string{"pc1"})

	if err != nil {
		t.Fatalf("una reserva en curso debería aceptarse: %v", err)
	}
}

func TestCrearReserva_DuracionExcesiva_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 0, 23*time.Hour+59*time.Minute, []string{"pc1"})

	if !errors.Is(err, domain.ErrDuracionExcesiva) {
		t.Fatalf("esperaba ErrDuracionExcesiva, obtuve %v", err)
	}
}

func TestCrearReserva_SinPCs_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, nil)

	if !errors.Is(err, ErrSinPCs) {
		t.Fatalf("esperaba ErrSinPCs, obtuve %v", err)
	}
}

func TestCrearReserva_DocenteNoAsignado_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: false}, &fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrDocenteNoAsignado) {
		t.Fatalf("esperaba ErrDocenteNoAsignado, obtuve %v", err)
	}
}

func TestCrearReserva_PCNoDisponible_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: true}, &fakeValidadorPC{disponible: false},
		&fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrPCNoDisponible) {
		t.Fatalf("esperaba ErrPCNoDisponible, obtuve %v", err)
	}
}

func TestCrearReserva_Solapamiento_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	existente := &domain.Reserva{
		ID: "existente", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour, Estado: domain.ReservaConfirmada,
	}
	repo.reservas[existente.ID] = existente
	svc := nuevoServicioDeTest(repo)

	// Pide el mismo horario, misma PC, misma fecha — debería solapar.
	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour+30*time.Minute, 9*time.Hour+30*time.Minute, []string{"pc1"})

	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba ErrSolapamiento, obtuve %v", err)
	}
}

func TestCrearReserva_MismaPCOtroDia_NoSolapaAunqueMismoHorario(t *testing.T) {
	repo := nuevoFakeRepo()
	existente := &domain.Reserva{
		ID: "existente", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour, Estado: domain.ReservaConfirmada,
	}
	repo.reservas[existente.ID] = existente
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 16), 8*time.Hour, 9*time.Hour, []string{"pc1"}) // otro lunes

	if err != nil {
		t.Fatalf("otro día, mismo horario, no debería solapar: %v", err)
	}
}

func TestCrearReserva_ReservaCanceladaNoCuentaParaSolapamiento(t *testing.T) {
	repo := nuevoFakeRepo()
	cancelada := &domain.Reserva{
		ID: "cancelada", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour, Estado: domain.ReservaCancelada,
	}
	repo.reservas[cancelada.ID] = cancelada
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if err != nil {
		t.Fatalf("una reserva cancelada no debería bloquear una nueva: %v", err)
	}
}

// ── CancelarReserva ─────────────────────────────────────────────────────

func TestCancelarReserva_UnaDeVarias_GrupoQuedaParcial(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	canceladoPor := "admin1"
	err = svc.CancelarReserva(context.Background(), reservas[0].ID, &canceladoPor, "PC rota")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	grupoRecargado, _ := repo.BuscarReservaGrupoPorID(context.Background(), grupo.ID)
	if grupoRecargado.Estado != domain.GrupoParcialmenteCancelada {
		t.Errorf("esperaba PARCIALMENTE_CANCELADA, obtuve %s", grupoRecargado.Estado)
	}
}

func TestCancelarReserva_TodasLasDelGrupo_GrupoQuedaCancelado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	canceladoPor := "admin1"
	for _, r := range reservas {
		if err := svc.CancelarReserva(context.Background(), r.ID, &canceladoPor, "Bloqueo evaluación"); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
	}

	grupoRecargado, _ := repo.BuscarReservaGrupoPorID(context.Background(), grupo.ID)
	if grupoRecargado.Estado != domain.GrupoCancelada {
		t.Errorf("esperaba CANCELADA, obtuve %s", grupoRecargado.Estado)
	}
}

func TestCancelarReserva_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	err := svc.CancelarReserva(context.Background(), "no-existe", nil, "motivo")

	if !errors.Is(err, ErrReservaNoEncontrada) {
		t.Fatalf("esperaba ErrReservaNoEncontrada, obtuve %v", err)
	}
}

func TestCancelarReserva_YaCancelada_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaCancelada}
	svc := nuevoServicioDeTest(repo)

	err := svc.CancelarReserva(context.Background(), "r1", nil, "motivo")

	if !errors.Is(err, domain.ErrTransicionReservaInvalida) {
		t.Fatalf("esperaba ErrTransicionReservaInvalida, obtuve %v", err)
	}
}

func TestCancelarReserva_DeBloqueoEvaluacion_NoTocaNingunGrupo(t *testing.T) {
	// Un bloqueo de evaluación no pertenece a ningún ReservaGrupo — cancelarlo
	// no debe intentar buscar/actualizar ningún grupo (ni panickear).
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, Tipo: domain.TipoEvaluacionEstatal}
	svc := nuevoServicioDeTest(repo)

	err := svc.CancelarReserva(context.Background(), "r1", nil, "motivo")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestCancelarReserva_PublicaEventoReservaCancelada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		recibido <- e
	})

	canceladoPor := "admin1"
	if err := svc.CancelarReserva(context.Background(), reservas[0].ID, &canceladoPor, "PC rota"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		payload := e.Payload.(eventbus.CancelacionesDeUsuario)
		if payload.UsuarioID != "docente1" || payload.Motivo != "PC rota" {
			t.Errorf("payload incorrecto: %+v", payload)
		}
		if len(payload.Reservas) != 1 || payload.Reservas[0].ReservaID != reservas[0].ID {
			t.Errorf("esperaba la reserva cancelada en el detalle: %+v", payload.Reservas)
		}
		if payload.Reservas[0].Etiqueta != "PC 1" {
			t.Errorf("el aviso tiene que poder nombrar la PC: %+v", payload.Reservas[0])
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento reserva.cancelada")
	}
}

// RF-05.1/05.2/05.3: bloquear tres PCs de una misma reserva le dejaba al
// docente tres avisos idénticos. Es una sola noticia para él —"me sacaron la
// clase"— así que sale un evento con las tres PCs adentro.
func TestBloquearParaEvaluacion_UnSoloEventoPorDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, []string{"pc1", "pc2", "pc3"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	var eventos []eventbus.Evento
	var mu sync.Mutex
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		mu.Lock()
		defer mu.Unlock()
		eventos = append(eventos, e)
	})

	admin := "admin1"
	res, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1", "pc2", "pc3"}, &admin,
		fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, "Mesa de examen")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.ReservasCanceladas != 3 {
		t.Fatalf("esperaba 3 reservas canceladas, obtuve %d", res.ReservasCanceladas)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(eventos) != 1 {
		t.Fatalf("esperaba UN evento para el docente, obtuve %d", len(eventos))
	}
	payload := eventos[0].Payload.(eventbus.CancelacionesDeUsuario)
	if payload.UsuarioID != "docente1" {
		t.Errorf("destinatario incorrecto: %s", payload.UsuarioID)
	}
	if len(payload.Reservas) != 3 {
		t.Fatalf("el aviso tiene que traer las 3 PCs, trae %d", len(payload.Reservas))
	}
	pcs := map[string]bool{}
	for _, r := range payload.Reservas {
		pcs[r.Etiqueta] = true
	}
	for _, esperada := range []string{"PC 1", "PC 2", "PC 3"} {
		if !pcs[esperada] {
			t.Errorf("falta la %s en el detalle: %+v", esperada, payload.Reservas)
		}
	}
}

// Dos docentes afectados por el mismo bloqueo reciben un aviso cada uno, no
// uno con las reservas del otro adentro.
func TestBloquearParaEvaluacion_UnEventoPorCadaDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	for _, docente := range []string{"docente1", "docente2"} {
		pc := "pc1"
		if docente == "docente2" {
			pc = "pc2"
		}
		if _, _, err := svc.CrearReserva(context.Background(), "materia1", docente, false,
			fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, []string{pc}); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
	}

	var eventos []eventbus.Evento
	var mu sync.Mutex
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		mu.Lock()
		defer mu.Unlock()
		eventos = append(eventos, e)
	})

	admin := "admin1"
	if _, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1", "pc2"}, &admin,
		fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, "Mesa de examen"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(eventos) != 2 {
		t.Fatalf("esperaba un evento por docente (2), obtuve %d", len(eventos))
	}
	destinatarios := map[string]int{}
	for _, e := range eventos {
		p := e.Payload.(eventbus.CancelacionesDeUsuario)
		destinatarios[p.UsuarioID] = len(p.Reservas)
	}
	if destinatarios["docente1"] != 1 || destinatarios["docente2"] != 1 {
		t.Errorf("cada docente debería recibir solo lo suyo: %+v", destinatarios)
	}
}

// Si no se pueden resolver los identificadores, el aviso sale igual sin el
// detalle: quedarse sin notificar por no poder adornar el mensaje sería
// mucho peor que un mensaje menos específico.
func TestPublicarCancelaciones_SinIdentificadores_ElAvisoSaleIgual(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeValidadorMateria{asignado: true},
		&fakeValidadorPC{disponible: true, errIdentificadores: errors.New("inventory caído")},
		&fakeObtenedorNombre{nombre: "Ada"}, idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus())

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) { recibido <- e })

	canceladoPor := "admin1"
	if err := svc.CancelarReserva(context.Background(), reservas[0].ID, &canceladoPor, "PC rota"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		payload := e.Payload.(eventbus.CancelacionesDeUsuario)
		if len(payload.Reservas) != 1 || payload.Reservas[0].Etiqueta != "" {
			t.Errorf("esperaba el aviso sin etiqueta: %+v", payload.Reservas)
		}
	case <-time.After(time.Second):
		t.Fatal("el evento tenía que publicarse igual")
	}
}

// RF-05.1 es "tu reserva fue cancelada POR UN ADMIN". Cancelar lo propio
// es una acción deliberada del docente: avisarle de algo que acaba de
// hacer no aporta nada, y como el motivo es opcional cuando la reserva es
// propia (RF-04.8), el aviso salía además como "Tu reserva fue cancelada: "
// con el dos puntos colgando.
func TestCancelarReserva_ElDocenteCancelaLaPropia_NoPublicaEvento(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	publicado := false
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		publicado = true
	})

	elMismoDocente := "docente1"
	if err := svc.CancelarReserva(context.Background(), reservas[0].ID, &elMismoDocente, ""); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if publicado {
		t.Error("no hay que notificarle al docente una cancelación que hizo él mismo")
	}
}

// El motivo viaja sin el "Tu reserva fue cancelada:" — esa frase la pone
// el suscriptor de notification. Si el servicio también la pusiera, el
// aviso saldría con el prefijo repetido.
func TestBloquearParaEvaluacion_ElMotivoNoTraeElPrefijoDelAviso(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 10*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	recibido := make(chan eventbus.Evento, 1)
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) { recibido <- e })

	admin := "admin1"
	if _, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &admin,
		fecha(2026, 3, 9), 9*time.Hour, 11*time.Hour, "Aprender 2026"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		motivo := e.Payload.(eventbus.CancelacionesDeUsuario).Motivo
		if strings.Contains(motivo, "Tu reserva fue cancelada") {
			t.Errorf("el motivo no debe traer el prefijo del aviso: %q", motivo)
		}
		if motivo != "bloqueo por evaluación estatal (Aprender 2026)" {
			t.Errorf("motivo inesperado: %q", motivo)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento reserva.cancelada")
	}
}

func TestCancelarReserva_BloqueoEvaluacionCancelado_NoPublicaEvento(t *testing.T) {
	// Un bloqueo de evaluación no tiene CreadoPor de un docente afectado
	// que notificar de la misma forma — no debería publicar nada (o al
	// menos no debería panickear al no tener a quién avisar).
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, Tipo: domain.TipoEvaluacionEstatal, CreadoPor: nil}
	svc := nuevoServicioDeTest(repo)

	publicado := false
	svc.bus.Subscribe("reserva.cancelada", func(e eventbus.Evento) {
		publicado = true
	})

	if err := svc.CancelarReserva(context.Background(), "r1", nil, "motivo"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if publicado {
		t.Error("no debería publicarse nada si la reserva no tiene CreadoPor")
	}
}

// ── CrearReservaRecurrente ──────────────────────────────────────────────

func TestCrearReservaRecurrente_OK(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	// Marzo 2026: los lunes son 2, 9, 16, 23, 30 — el mock "ahora" es
	// lunes 2/3 al mediodía, así que arrancamos la regla desde ahí, a la
	// tarde: la primera ocurrencia es hoy pero todavía no empezó.
	res, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 14*time.Hour, 15*time.Hour,
		fecha(2026, time.March, 2), fecha(2026, time.March, 30), []string{"pc1"})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(res.Grupos) != 5 {
		t.Fatalf("esperaba 5 ocurrencias (lunes de marzo), obtuve %d", len(res.Grupos))
	}
	for _, g := range res.Grupos {
		if g.ReglaRecurrenciaID == nil || *g.ReglaRecurrenciaID != res.Regla.ID {
			t.Errorf("grupo no vinculado a la regla: %+v", g)
		}
	}
}

func TestCrearReservaRecurrente_DocenteNoAsignado_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: false}, &fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 14*time.Hour, 15*time.Hour, fecha(2026, 3, 2), fecha(2026, 3, 9), []string{"pc1"})

	if !errors.Is(err, ErrDocenteNoAsignado) {
		t.Fatalf("esperaba ErrDocenteNoAsignado, obtuve %v", err)
	}
}

func TestCrearReservaRecurrente_SolapamientoEnUnaFecha_AbortaTodo(t *testing.T) {
	repo := nuevoFakeRepo()
	// Una reserva existente choca justo con la segunda ocurrencia (9 de marzo).
	repo.reservas["existente"] = &domain.Reserva{
		ID: "existente", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 14 * time.Hour, HoraFin: 15 * time.Hour, Estado: domain.ReservaConfirmada,
	}
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 14*time.Hour, 15*time.Hour, fecha(2026, 3, 2), fecha(2026, 3, 16), []string{"pc1"})

	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba ErrSolapamiento, obtuve %v", err)
	}
	// Nada debería haberse creado — ni la regla ni ningún grupo.
	if len(repo.reglas) != 0 {
		t.Error("no debería haberse creado ninguna regla si una fecha solapa")
	}
	if len(repo.grupos) != 0 {
		t.Error("no debería haberse creado ningún grupo si una fecha solapa")
	}
}

func TestCrearReservaRecurrente_SerieQueArrancaEnElPasado_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	// Los lunes de marzo, pero a las 8 de la mañana: la primera ocurrencia
	// (lunes 2, el día del reloj de test) ya terminó al mediodía.
	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, 3, 2), fecha(2026, 3, 30), []string{"pc1"})

	if !errors.Is(err, domain.ErrReservaEnElPasado) {
		t.Fatalf("esperaba ErrReservaEnElPasado, obtuve %v", err)
	}
	// La serie se rechaza entera, no se materializa recortada.
	if len(repo.reglas) != 0 || len(repo.grupos) != 0 {
		t.Errorf("no debería haberse creado nada: %d reglas, %d grupos", len(repo.reglas), len(repo.grupos))
	}
}

// ── CancelarOcurrenciaRecurrente ────────────────────────────────────────

func crearSerieDeTest(t *testing.T, repo *fakeRepo, svc *Service) *ResultadoRecurrencia {
	t.Helper()
	res, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 14*time.Hour, 15*time.Hour,
		fecha(2026, time.March, 2), fecha(2026, time.March, 30), []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar armando la serie de test: %v", err)
	}
	return res
}

func TestCancelarOcurrenciaRecurrente_SoloEsta_NoAfectaLasDemas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	res := crearSerieDeTest(t, repo, svc)

	// Cancelamos solo la segunda ocurrencia (9 de marzo).
	segunda := res.Grupos[1]
	canceladoPor := "admin1"
	n, err := svc.CancelarOcurrenciaRecurrente(context.Background(), segunda.ID, &canceladoPor, "no hay clase", true)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 1 {
		t.Errorf("esperaba 1 reserva cancelada, obtuve %d", n)
	}

	segundaRecargada, _ := repo.BuscarReservaGrupoPorID(context.Background(), segunda.ID)
	if segundaRecargada.Estado != domain.GrupoCancelada {
		t.Errorf("la ocurrencia cancelada debería quedar CANCELADA: %s", segundaRecargada.Estado)
	}

	// Las demás (incluida la tercera, posterior) deben seguir CONFIRMADA.
	tercera := res.Grupos[2]
	terceraRecargada, _ := repo.BuscarReservaGrupoPorID(context.Background(), tercera.ID)
	if terceraRecargada.Estado != domain.GrupoConfirmada {
		t.Errorf("una ocurrencia no tocada debería seguir CONFIRMADA: %s", terceraRecargada.Estado)
	}
}

func TestCancelarOcurrenciaRecurrente_EstaYSiguientes_CancelaElRestoNoLasAnteriores(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	res := crearSerieDeTest(t, repo, svc) // 5 ocurrencias: 2,9,16,23,30 de marzo

	tercera := res.Grupos[2] // 16 de marzo
	canceladoPor := "admin1"
	n, err := svc.CancelarOcurrenciaRecurrente(context.Background(), tercera.ID, &canceladoPor, "docente de licencia", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 3 { // 16, 23 y 30 de marzo
		t.Errorf("esperaba 3 reservas canceladas (esta y las siguientes), obtuve %d", n)
	}

	// Las dos primeras (2 y 9 de marzo) no deberían haberse tocado.
	for i := 0; i < 2; i++ {
		g, _ := repo.BuscarReservaGrupoPorID(context.Background(), res.Grupos[i].ID)
		if g.Estado != domain.GrupoConfirmada {
			t.Errorf("ocurrencia anterior %d no debería haberse cancelado: %s", i, g.Estado)
		}
	}
	// La tercera en adelante sí.
	for i := 2; i < 5; i++ {
		g, _ := repo.BuscarReservaGrupoPorID(context.Background(), res.Grupos[i].ID)
		if g.Estado != domain.GrupoCancelada {
			t.Errorf("ocurrencia %d debería haberse cancelado: %s", i, g.Estado)
		}
	}
}

func TestCancelarOcurrenciaRecurrente_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.CancelarOcurrenciaRecurrente(context.Background(), "no-existe", nil, "motivo", true)

	if !errors.Is(err, ErrReservaGrupoNoEncontrado) {
		t.Fatalf("esperaba ErrReservaGrupoNoEncontrado, obtuve %v", err)
	}
}

// ── BloquearParaEvaluacion ──────────────────────────────────────────────

func TestBloquearParaEvaluacion_SinConflictos_OK(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	res, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1", "pc2"}, &creadoPor,
		fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, "Evaluación provincial")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(res.Bloqueos) != 2 {
		t.Fatalf("esperaba 2 bloqueos, obtuve %d", len(res.Bloqueos))
	}
	if res.ReservasCanceladas != 0 || res.DocentesNotificados != 0 {
		t.Errorf("sin conflictos no debería cancelar nada: %+v", res)
	}
	for _, b := range res.Bloqueos {
		if b.Tipo != domain.TipoEvaluacionEstatal {
			t.Errorf("tipo incorrecto: %s", b.Tipo)
		}
	}
}

func TestBloquearParaEvaluacion_EnElPasado_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	_, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 2, 27), 10*time.Hour, 12*time.Hour, "Evaluación provincial")

	if !errors.Is(err, domain.ErrReservaEnElPasado) {
		t.Fatalf("esperaba ErrReservaEnElPasado, obtuve %v", err)
	}
}

// El tope de duración no alcanza a RF-04.7: si el Admin necesita el
// laboratorio el día entero para una evaluación, es su decisión — mismo
// criterio que la exención de EsDiaLectivo.
func TestBloquearParaEvaluacion_DiaEntero_SeAcepta(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	res, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 3, 9), 0, 23*time.Hour+59*time.Minute, "Evaluación provincial")

	if err != nil {
		t.Fatalf("un bloqueo de día completo debería aceptarse: %v", err)
	}
	if len(res.Bloqueos) != 1 {
		t.Fatalf("esperaba 1 bloqueo, obtuve %d", len(res.Bloqueos))
	}
}

func TestBloquearParaEvaluacion_CancelaReservaQueSeSolapa(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAfectado := "docente-afectado"
	repo.reservas["existente"] = &domain.Reserva{
		ID: "existente", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 10 * time.Hour, HoraFin: 11 * time.Hour,
		Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal, CreadoPor: &docenteAfectado,
	}
	svc := nuevoServicioDeTest(repo)
	creadoPor := "admin1"

	res, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 3, 9), 9*time.Hour, 12*time.Hour, "Evaluación provincial")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.ReservasCanceladas != 1 {
		t.Errorf("esperaba 1 reserva cancelada, obtuve %d", res.ReservasCanceladas)
	}
	if res.DocentesNotificados != 1 {
		t.Errorf("esperaba 1 docente notificado, obtuve %d", res.DocentesNotificados)
	}
	if repo.reservas["existente"].Estado != domain.ReservaCancelada {
		t.Error("la reserva que se solapaba debería quedar cancelada")
	}
}

func TestBloquearParaEvaluacion_NoCancelaOtroBloqueoDeEvaluacion(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.reservas["otro-bloqueo"] = &domain.Reserva{
		ID: "otro-bloqueo", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 10 * time.Hour, HoraFin: 11 * time.Hour,
		Estado: domain.ReservaConfirmada, Tipo: domain.TipoEvaluacionEstatal,
	}
	svc := nuevoServicioDeTest(repo)
	creadoPor := "admin1"

	res, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 3, 9), 9*time.Hour, 12*time.Hour, "Evaluación provincial")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.ReservasCanceladas != 0 {
		t.Error("un bloqueo de evaluación existente no debería cancelarse por otro bloqueo")
	}
	if repo.reservas["otro-bloqueo"].Estado != domain.ReservaConfirmada {
		t.Error("el otro bloqueo debería seguir confirmado")
	}
}

func TestBloquearParaEvaluacion_PCNoDisponible_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: true}, &fakeValidadorPC{disponible: false},
		&fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, err := svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, nil,
		fecha(2026, 3, 9), 9*time.Hour, 12*time.Hour, "motivo")

	if !errors.Is(err, ErrPCNoDisponible) {
		t.Fatalf("esperaba ErrPCNoDisponible, obtuve %v", err)
	}
}

// ── FinalizarVencidas ───────────────────────────────────────────────────

type fakeRepoConVencidas struct {
	*fakeRepo
	vencidas []*domain.Reserva
}

// Imita al repo real en las dos cosas de las que depende el job por lotes:
// solo devuelve lo que sigue CONFIRMADA (lo ya finalizado sale del conjunto)
// y respeta el límite. Sin esto, un test de lotes vería siempre la misma
// lista y no probaría nada.
func (f *fakeRepoConVencidas) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	var pendientes []*domain.Reserva
	for _, r := range f.vencidas {
		if r.Estado != domain.ReservaConfirmada {
			continue
		}
		pendientes = append(pendientes, r)
		if len(pendientes) == limite {
			break
		}
	}
	return pendientes, nil
}

// EnTransaccion se redefine acá a propósito: la versión promovida desde
// *fakeRepo le pasaría a fn el fake interno, perdiendo este override de
// ListarReservasConfirmadasVencidas. El repo real tiene el mismo cuidado —
// la transacción devuelve un repo del mismo tipo concreto.
func (f *fakeRepoConVencidas) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return fn(f)
}

// El job leía TODO lo vencido en una sola transacción, y "todo lo vencido"
// crece con cada hora que el proceso haya estado caído. Ahora va por lotes:
// lo que importa es que un atraso más grande que el lote se procese entero
// igual, en varias transacciones.
func TestFinalizarVencidas_AtrasoMayorQueUnLote_LoProcesaEntero(t *testing.T) {
	base := nuevoFakeRepo()
	cantidad := loteFinalizarVencidas*2 + 37

	vencidas := make([]*domain.Reserva, 0, cantidad)
	for i := 0; i < cantidad; i++ {
		id := fmt.Sprintf("r%d", i)
		r := &domain.Reserva{
			ID: id, PCID: "pc1", Fecha: fecha(2026, 3, 9),
			HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour,
			Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal,
		}
		base.reservas[id] = r
		vencidas = append(vencidas, r)
	}

	repo := &fakeRepoConVencidas{fakeRepo: base, vencidas: vencidas}
	svc := nuevoServicioDeTest(repo)

	n, err := svc.FinalizarVencidas(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != cantidad {
		t.Fatalf("finalizó %d de %d", n, cantidad)
	}
	for _, r := range vencidas {
		if r.Estado != domain.ReservaFinalizada {
			t.Fatalf("la reserva %s quedó en %s", r.ID, r.Estado)
			break
		}
	}
}

// Una reserva que el repo devuelve como vencida pero que no puede
// transicionar se saltea, y el lote entero no avanza. Sin el corte por falta
// de progreso, el job pediría el mismo lote hasta agotar maxLotesPorCiclo en
// cada corrida del ticker.
func TestFinalizarVencidas_LoteSinProgreso_NoSeQuedaEnBucle(t *testing.T) {
	base := nuevoFakeRepo()

	// Estado terminal: ListarReservasConfirmadasVencidas del repo real nunca
	// la devolvería, pero el fake la fuerza para simular ese estado imposible.
	rara := &domain.Reserva{
		ID: "rara", PCID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 8 * time.Hour, HoraFin: 9 * time.Hour,
		Estado: domain.ReservaCancelada,
	}
	base.reservas[rara.ID] = rara

	repo := &fakeRepoTerca{fakeRepo: base, siempre: []*domain.Reserva{rara}}
	svc := nuevoServicioDeTest(repo)

	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		n, err := svc.FinalizarVencidas(context.Background())
		if err != nil {
			t.Errorf("no debería fallar: %v", err)
		}
		if n != 0 {
			t.Errorf("no debería haber finalizado nada, obtuve %d", n)
		}
	}()

	select {
	case <-hecho:
	case <-time.After(5 * time.Second):
		t.Fatal("el job no terminó: se quedó pidiendo el mismo lote")
	}
}

// fakeRepoTerca devuelve SIEMPRE el mismo lote completo, sin importar lo que
// haya pasado antes — es el escenario que hace falta para probar el corte
// por falta de progreso.
type fakeRepoTerca struct {
	*fakeRepo
	siempre []*domain.Reserva
}

func (f *fakeRepoTerca) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	lote := make([]*domain.Reserva, 0, limite)
	for len(lote) < limite {
		lote = append(lote, f.siempre...)
	}
	return lote[:limite], nil
}

func (f *fakeRepoTerca) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return fn(f)
}

func TestFinalizarVencidas_MarcaFinalizadaYPropagaAlGrupo(t *testing.T) {
	base := nuevoFakeRepo()
	svc := nuevoServicioDeTest(base)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	repoConVencidas := &fakeRepoConVencidas{fakeRepo: base, vencidas: reservas}
	svc2 := nuevoServicioDeTest(repoConVencidas)

	n, err := svc2.FinalizarVencidas(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 1 {
		t.Errorf("esperaba 1 reserva finalizada, obtuve %d", n)
	}
	if base.reservas[reservas[0].ID].Estado != domain.ReservaFinalizada {
		t.Error("la reserva debería quedar FINALIZADA")
	}

	grupoRecargado, _ := base.BuscarReservaGrupoPorID(context.Background(), grupo.ID)
	if grupoRecargado.Estado != domain.GrupoFinalizada {
		t.Errorf("el grupo debería quedar FINALIZADA al no quedar ninguna reserva viva: %s", grupoRecargado.Estado)
	}
}

func TestFinalizarVencidas_GrupoConAlgunaCanceladaYOtraFinalizada_GrupoFinalizaIgual(t *testing.T) {
	base := nuevoFakeRepo()
	svc := nuevoServicioDeTest(base)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Cancelamos una de las dos manualmente antes de que venza la otra.
	canceladoPor := "admin1"
	if err := svc.CancelarReserva(context.Background(), reservas[0].ID, &canceladoPor, "PC rota"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// La que queda confirmada (reservas[1]) "vence".
	repoConVencidas := &fakeRepoConVencidas{fakeRepo: base, vencidas: []*domain.Reserva{reservas[1]}}
	svc2 := nuevoServicioDeTest(repoConVencidas)

	_, err = svc2.FinalizarVencidas(context.Background())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	grupoRecargado, _ := base.BuscarReservaGrupoPorID(context.Background(), grupo.ID)
	if grupoRecargado.Estado != domain.GrupoFinalizada {
		t.Errorf("el grupo debería quedar FINALIZADA (mezcla cancelada+finalizada, ninguna viva): %s", grupoRecargado.Estado)
	}
}

func TestFinalizarVencidas_SinVencidas_NoHaceNada(t *testing.T) {
	repoConVencidas := &fakeRepoConVencidas{fakeRepo: nuevoFakeRepo(), vencidas: nil}
	svc := nuevoServicioDeTest(repoConVencidas)

	n, err := svc.FinalizarVencidas(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 0 {
		t.Errorf("esperaba 0, obtuve %d", n)
	}
}

// ── ObtenerReserva / ObtenerReservaGrupo (passthroughs) ────────────────

func TestObtenerReserva_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", PCID: "pc1"}
	svc := nuevoServicioDeTest(repo)

	r, err := svc.ObtenerReserva(context.Background(), "r1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.PCID != "pc1" {
		t.Errorf("reserva incorrecta: %+v", r)
	}
}

func TestObtenerReserva_NoExiste_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.ObtenerReserva(context.Background(), "no-existe")

	if !errors.Is(err, ErrReservaNoEncontrada) {
		t.Fatalf("esperaba ErrReservaNoEncontrada, obtuve %v", err)
	}
}

func TestObtenerReservaGrupo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.grupos["g1"] = &domain.ReservaGrupo{ID: "g1", MateriaID: "materia1"}
	svc := nuevoServicioDeTest(repo)

	g, err := svc.ObtenerReservaGrupo(context.Background(), "g1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if g.MateriaID != "materia1" {
		t.Errorf("grupo incorrecto: %+v", g)
	}
}

// ── CancelarReservasFuturasDePC (cascada hacia inventory) ───────────────

func TestCancelarReservasFuturasDePC_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	canceladas, notificados, err := svc.CancelarReservasFuturasDePC(context.Background(), "pc1", "PC dada de baja")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 1 {
		t.Errorf("esperaba 1 reserva cancelada (solo la de pc1), obtuve %d", canceladas)
	}
	if notificados != 1 {
		t.Errorf("esperaba 1 docente notificado, obtuve %d", notificados)
	}
	if repo.reservas[reservas[0].ID].Estado != domain.ReservaCancelada {
		t.Error("la reserva de pc1 debería quedar cancelada")
	}
	// pc2 no debería tocarse.
	pc2Reserva := reservas[1]
	if repo.reservas[pc2Reserva.ID].Estado != domain.ReservaConfirmada {
		t.Error("la reserva de pc2 no debería haberse tocado")
	}
}

func TestCancelarReservasFuturasDePC_SinReservas_NoHaceNada(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	canceladas, notificados, err := svc.CancelarReservasFuturasDePC(context.Background(), "pc-sin-reservas", "motivo")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 0 || notificados != 0 {
		t.Errorf("esperaba 0/0, obtuve %d/%d", canceladas, notificados)
	}
}

// ── CancelarReservasFuturasDeMateria (cascada hacia auth) ───────────────

func TestCancelarReservasFuturasDeMateria_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia-huerfana", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	canceladas, err := svc.CancelarReservasFuturasDeMateria(context.Background(), "materia-huerfana", "Docente dado de baja")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if canceladas != 1 {
		t.Errorf("esperaba 1 reserva cancelada, obtuve %d", canceladas)
	}
	if repo.reservas[reservas[0].ID].Estado != domain.ReservaCancelada {
		t.Error("la reserva debería quedar cancelada")
	}
	grupoRecargado, _ := repo.BuscarReservaGrupoPorID(context.Background(), grupo.ID)
	if grupoRecargado.Estado != domain.GrupoCancelada {
		t.Errorf("el grupo debería quedar CANCELADA, quedó: %s", grupoRecargado.Estado)
	}
}

func TestCancelarReservasFuturasDeMateria_OtraMateriaNoSeToca(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, reservasOtra, err := svc.CrearReserva(context.Background(), "materia-normal", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	_, err = svc.CancelarReservasFuturasDeMateria(context.Background(), "materia-huerfana-sin-reservas", "motivo")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.reservas[reservasOtra[0].ID].Estado != domain.ReservaConfirmada {
		t.Error("una materia sin relación no debería afectar reservas de otra materia")
	}
}

// ── EliminarReservasDeCiclo (cascada hacia academic) ────────────────────

type fakeRepoConEliminarCiclo struct {
	*fakeRepo
	grupos, reservas int
	err              error
}

func (f *fakeRepoConEliminarCiclo) EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (int, int, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.grupos, f.reservas, nil
}

func TestEliminarReservasDeCiclo_OK(t *testing.T) {
	repo := &fakeRepoConEliminarCiclo{fakeRepo: nuevoFakeRepo(), grupos: 3, reservas: 7}
	svc := nuevoServicioDeTest(repo)

	grupos, reservas, err := svc.EliminarReservasDeCiclo(context.Background(), "ciclo1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if grupos != 3 || reservas != 7 {
		t.Errorf("esperaba 3/7, obtuve %d/%d", grupos, reservas)
	}
}

func TestEliminarReservasDeCiclo_ErrorDelRepo_SePropaga(t *testing.T) {
	repo := &fakeRepoConEliminarCiclo{fakeRepo: nuevoFakeRepo(), err: errors.New("fk violation")}
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.EliminarReservasDeCiclo(context.Background(), "ciclo1")

	if err == nil {
		t.Fatal("esperaba que el error se propague")
	}
}

// ── RF-04.1: quién puede reservar y sobre qué materia ──────────────────

// "Pueden reservar para una materia: docentes asignados a ella (vía
// DocenteMateria) Y CUALQUIER ADMIN". El chequeo miraba solo
// docente_materia, así que un Admin no asignado quedaba afuera.
func TestCrearReserva_UnAdminNoAsignadoPuedeReservar(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: false},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Admin Inicial"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus(),
	)

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "admin1", true,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if err != nil {
		t.Fatalf("un Admin puede reservar sin estar asignado (RF-04.1): %v", err)
	}
	if len(reservas) != 1 {
		t.Errorf("esperaba 1 reserva creada, obtuve %d", len(reservas))
	}
}

func TestCrearReserva_UnDocenteNoAsignadoSigueSinPoder(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: false},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus(),
	)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrDocenteNoAsignado) {
		t.Fatalf("esperaba ErrDocenteNoAsignado, obtuve %v", err)
	}
}

// "siempre que la materia no esté archivada — una materia de un ciclo ya
// cerrado no admite reservas nuevas aunque el registro se conserve". Nada
// validaba esto: alcanzaba con estar asignado.
func TestCrearReserva_MateriaArchivada_NoAdmiteReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: true, archivada: true},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus(),
	)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrMateriaArchivada) {
		t.Fatalf("esperaba ErrMateriaArchivada, obtuve %v", err)
	}
}

// Ni siquiera un Admin puede reservar sobre una materia archivada: las dos
// condiciones de RF-04.1 son independientes.
func TestCrearReserva_UnAdminTampocoPuedeSobreMateriaArchivada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: true, archivada: true},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Admin"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus(),
	)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "admin1", true,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrMateriaArchivada) {
		t.Fatalf("esperaba ErrMateriaArchivada, obtuve %v", err)
	}
}

func TestCrearReservaRecurrente_MateriaArchivada_NoAdmiteReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: true, archivada: true},
		&fakeValidadorPC{disponible: true},
		&fakeObtenedorNombre{nombre: "Ada"},
		idSecuencial,
		func() time.Time { return time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC) },
		eventbus.NewInMemoryEventBus(),
	)

	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, 3, 9), fecha(2026, 4, 30), []string{"pc1"})

	if !errors.Is(err, ErrMateriaArchivada) {
		t.Fatalf("esperaba ErrMateriaArchivada, obtuve %v", err)
	}
}

// ── Alcance de la transacción ───────────────────────────────────────────

// espiaDeTransaccion detecta escrituras que se escapan de la transacción.
//
// El fakeRepo normal no puede: su EnTransaccion le pasa a fn el MISMO
// objeto, así que escribir por `s.repo` o por el `repo` del closure es
// indistinguible. El PostgresRepo real no funciona así — devuelve un repo
// nuevo atado a la pgx.Tx, y `s.repo` sigue apuntando al pool—, y por eso
// el bug de actualizarEstadoGrupo (que guardaba el ReservaGrupo por
// `s.repo`) daba verde acá y corrompía datos en producción: la Reserva
// volvía atrás con el rollback y su grupo padre quedaba CANCELADA.
//
// Esto imita esa distinción: fn recibe una instancia marcada como "dentro
// de la transacción" y todo lo que se guarde por afuera queda contado.
type espiaDeTransaccion struct {
	*fakeRepo
	dentroDeTx     bool
	gruposPorFuera *int
}

func nuevoEspiaDeTransaccion(base *fakeRepo) *espiaDeTransaccion {
	var contador int
	return &espiaDeTransaccion{fakeRepo: base, gruposPorFuera: &contador}
}

func (e *espiaDeTransaccion) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return fn(&espiaDeTransaccion{fakeRepo: e.fakeRepo, dentroDeTx: true, gruposPorFuera: e.gruposPorFuera})
}

func (e *espiaDeTransaccion) GuardarReservaGrupo(ctx context.Context, g *domain.ReservaGrupo) error {
	if !e.dentroDeTx {
		*e.gruposPorFuera++
	}
	return e.fakeRepo.GuardarReservaGrupo(ctx, g)
}

func TestCancelarReserva_EstadoDelGrupoSeGuardaDentroDeLaTransaccion(t *testing.T) {
	base := nuevoFakeRepo()
	espia := nuevoEspiaDeTransaccion(base)
	svc := nuevoServicioDeTest(espia)

	docente := "docente1"
	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", docente, false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}
	*espia.gruposPorFuera = 0 // solo interesa lo que pase al cancelar

	// Cancelar una de las dos PCs deja el grupo PARCIALMENTE_CANCELADA:
	// obliga a actualizarEstadoGrupo a escribir.
	admin := "admin1"
	if err := svc.CancelarReserva(context.Background(), reservas[0].ID, &admin, "acto escolar"); err != nil {
		t.Fatalf("no debería fallar cancelando: %v", err)
	}

	if *espia.gruposPorFuera != 0 {
		t.Errorf("el estado del ReservaGrupo se guardó %d vez/veces fuera de la transacción; "+
			"un rollback dejaría el grupo y sus reservas en estados incoherentes", *espia.gruposPorFuera)
	}
	if g := base.grupos[*reservas[0].ReservaGrupoID]; g.Estado != domain.GrupoParcialmenteCancelada {
		t.Errorf("el grupo debería haber quedado PARCIALMENTE_CANCELADA, quedó %s", g.Estado)
	}
}

func TestBloquearParaEvaluacion_SiFallaElBloqueoNoQuedaNingunGrupoTocado(t *testing.T) {
	base := nuevoFakeRepo()
	svc := nuevoServicioDeTest(base)

	docente := "docente1"
	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", docente, false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("no debería fallar creando: %v", err)
	}
	grupoID := *reservas[0].ReservaGrupoID

	// El INSERT del bloqueo falla DESPUÉS de que la cascada ya canceló la
	// reserva del docente y recalculó su grupo — el caso exacto en que el
	// rollback tiene que deshacer las dos cosas juntas.
	base.errCrearReserva = ErrSolapamiento

	admin := "admin1"
	_, err = svc.BloquearParaEvaluacion(context.Background(), []string{"pc1"}, &admin,
		fecha(2026, 3, 9), 8*time.Hour, 10*time.Hour, "prueba estatal")
	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba ErrSolapamiento, obtuve %v", err)
	}

	if estado := base.reservas[reservas[0].ID].Estado; estado != domain.ReservaConfirmada {
		t.Errorf("la reserva debería seguir CONFIRMADA tras el rollback, quedó %s", estado)
	}
	// Ojo: este test NO detecta por sí solo una escritura fuera de la
	// transacción — el fakeRepo restaura sus mapas enteros, sin importar
	// quién los escribió. De eso se ocupa el test de arriba, con el espía.
	// Este cubre la otra mitad: que la cascada y el bloqueo vayan
	// efectivamente dentro del mismo alcance transaccional.
	if estado := base.grupos[grupoID].Estado; estado != domain.GrupoConfirmada {
		t.Errorf("el grupo debería seguir CONFIRMADA tras el rollback, quedó %s", estado)
	}
}

// ── Tope de ocurrencias (RF-04.2) ──────────────────────────────────────

// Sin este tope, un fechaFin lejano materializaba miles de ReservaGrupo en
// una sola transacción: 2026-01-01 a 2099-12-31 daba 3.863 ocurrencias, con
// una consulta de pre-chequeo por PC y por fecha antes de insertar nada.
func TestCrearReservaRecurrente_RangoEnorme_Rechazado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.January, 1), fecha(2099, time.December, 31), []string{"pc1"})

	if !errors.Is(err, ErrDemasiadasOcurrencias) {
		t.Fatalf("esperaba ErrDemasiadasOcurrencias, obtuve %v", err)
	}
	// Y no debe haber tocado la base: ni la regla, ni un solo grupo.
	if len(repo.grupos) != 0 {
		t.Errorf("no debería haber creado ningún grupo, hay %d", len(repo.grupos))
	}
	if len(repo.reglas) != 0 {
		t.Errorf("no debería haber creado ninguna regla, hay %d", len(repo.reglas))
	}
}

// El límite tiene que dejar pasar el caso de uso real de RF-04.2: "todos los
// martes hasta que termine el año lectivo".
func TestCrearReservaRecurrente_AnioLectivoCompleto_Pasa(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	// Marzo a noviembre de 2026, todos los martes: 39 clases.
	res, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Martes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 3), fecha(2026, time.November, 24), []string{"pc1"})

	if err != nil {
		t.Fatalf("un año lectivo completo debe entrar en el tope: %v", err)
	}
	if len(res.Grupos) != 39 {
		t.Fatalf("esperaba 39 martes entre marzo y noviembre, obtuve %d", len(res.Grupos))
	}
}

// Un rango válido que no contiene ningún día de la semana pedido creaba una
// regla que no materializaba nada y quedaba huérfana en la base.
func TestCrearReservaRecurrente_RangoSinNingunaOcurrencia_Rechazado(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	// Del miércoles 4 al viernes 6 de marzo de 2026 no hay ningún lunes.
	_, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 8*time.Hour, 9*time.Hour,
		fecha(2026, time.March, 4), fecha(2026, time.March, 6), []string{"pc1"})

	if !errors.Is(err, ErrSinOcurrencias) {
		t.Fatalf("esperaba ErrSinOcurrencias, obtuve %v", err)
	}
	if len(repo.reglas) != 0 {
		t.Errorf("no debería quedar una regla huérfana, hay %d", len(repo.reglas))
	}
}

// ── TieneReservasFuturasDePC (lo que usa el reintento de inventory) ─────

func TestTieneReservasFuturasDePC_ConReservaConfirmada_True(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("preparando la reserva: %v", err)
	}

	tiene, err := svc.TieneReservasFuturasDePC(context.Background(), "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tiene {
		t.Error("la PC tiene una reserva confirmada por delante: esperaba true")
	}
}

func TestTieneReservasFuturasDePC_SinReservas_False(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	tiene, err := svc.TieneReservasFuturasDePC(context.Background(), "pc-sin-reservas")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("una PC sin reservas no tiene nada pendiente")
	}
}

// Después de que la cascada corrió, la respuesta tiene que dar false — es lo
// que hace que un segundo reintento en inventory devuelva 409 en vez de
// volver a cancelar sobre lo ya cancelado.
func TestTieneReservasFuturasDePC_DespuesDeLaCascada_False(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("preparando la reserva: %v", err)
	}
	if _, _, err := svc.CancelarReservasFuturasDePC(context.Background(), "pc1", "PC dada de baja"); err != nil {
		t.Fatalf("cascada: %v", err)
	}

	tiene, err := svc.TieneReservasFuturasDePC(context.Background(), "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("después de la cascada no queda nada pendiente: esperaba false")
	}
}
