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
	pcsDisponibles  []EquipoDisponible
	// materiaRecibidaAlListar: para qué materia se pidió la lista (RF-03.21).
	materiaRecibidaAlListar string
	// pcsDadasDeBaja imita lo único que el servicio le pregunta a
	// inventory antes de entregar una máquina.
	pcsDadasDeBaja map[string]bool

	// Lo que el barrido lee por JOIN contra equipo y usuario. En los tests se
	// carga a mano: acá no hay base que lo resuelva.
	identificadorDeEquipo map[string]int
	contactoDeUsuario     map[string][2]string // usuarioID → {nombre, email}
	// Las marcas del barrido, que en la base son columnas.
	recordatorioEnviado map[string]time.Time
	pcsOcupadas         []EquipoOcupado
	pedidosDeLiberacion map[string]bool

	errBuscarSolapamientos error
}

func nuevoFakeRepo() *fakeRepo {
	return &fakeRepo{
		grupos:         make(map[string]*domain.ReservaGrupo),
		reservas:       make(map[string]*domain.Reserva),
		reglas:         make(map[string]*domain.ReglaRecurrencia),
		prestamos:      make(map[string]*domain.Prestamo),
		pcsDadasDeBaja: make(map[string]bool),

		identificadorDeEquipo: make(map[string]int),
		contactoDeUsuario:     make(map[string][2]string),
		recordatorioEnviado:   make(map[string]time.Time),
		pedidosDeLiberacion:   make(map[string]bool),
	}
}

// ── Lo que lee el barrido ───────────────────────────────────────────────

// ReservasAVigilar reproduce la consulta real: las CONFIRMADA de hoy y
// mañana, con el contacto del docente y el estado de custodia de cada PC. El
// cruce con los préstamos va por equipo_id y no por reserva_id, igual que en
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
			ReservaID:     res.ID,
			GrupoID:       res.ReservaGrupoID,
			EquipoID:      res.EquipoID,
			Identificador: r.identificadorDeEquipo[res.EquipoID],
			Etiqueta:      fmt.Sprintf("PC %d", r.identificadorDeEquipo[res.EquipoID]),
			Fecha:         res.Fecha,
			HoraInicio:    res.HoraInicio,
			HoraFin:       res.HoraFin,
			Tipo:          res.Tipo,
			MateriaNombre: res.MateriaID,
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

		for _, p := range r.prestamos {
			if p.EquipoID == res.EquipoID && p.EstaAbierto() {
				v.EquipoAfuera = true
				v.EquipoDebioVolverA = p.DevolucionEstimada
				break
			}
		}
		// La última entrega CONTRA ESTA RESERVA (por reserva_id, no por
		// equipo) y sin filtrar por devuelta: reproduce la subconsulta real.
		v.UltimaEntregaDelGrupo = r.ultimaEntregaDelGrupo(res.ReservaGrupoID)
		resultado = append(resultado, v)
	}
	return resultado, nil
}

// ultimaEntregaDelGrupo: de todas las reservas del grupo, cuándo se entregó
// por última vez alguna.
func (r *fakeRepo) ultimaEntregaDelGrupo(grupoID *string) *time.Time {
	if grupoID == nil {
		return nil
	}
	var ultima *time.Time
	for _, p := range r.prestamos {
		if p.ReservaID == nil {
			continue
		}
		res, ok := r.reservas[*p.ReservaID]
		if !ok || res.ReservaGrupoID == nil || *res.ReservaGrupoID != *grupoID {
			continue
		}
		if ultima == nil || p.EntregadoEn.After(*ultima) {
			entregado := p.EntregadoEn
			ultima = &entregado
		}
	}
	return ultima
}

func (r *fakeRepo) PrestamosAVigilar(ctx context.Context) ([]PrestamoParaVigilar, error) {
	var resultado []PrestamoParaVigilar
	for _, p := range r.prestamosEnOrden() {
		if !p.EstaAbierto() {
			continue
		}
		v := PrestamoParaVigilar{
			Prestamo:      p,
			Identificador: r.identificadorDeEquipo[p.EquipoID],
			Etiqueta:      fmt.Sprintf("PC %d", r.identificadorDeEquipo[p.EquipoID]),
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

func (r *fakeRepo) ProximaReservaDeEquipo(ctx context.Context, equipoID string, desde time.Time) (*ProximaReserva, error) {
	var mejor *domain.Reserva
	for _, res := range r.enOrden() {
		if res.EquipoID != equipoID || res.Estado != domain.ReservaConfirmada {
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

func (r *fakeRepo) MarcarCierreAvisado(ctx context.Context, prestamoID string, jornada time.Time) error {
	if p, ok := r.prestamos[prestamoID]; ok {
		d := diaDe(jornada)
		p.AvisadoCierrePara = &d
	}
	return nil
}

func (r *fakeRepo) ContarAvisadosSinDevolver(ctx context.Context) (int, error) {
	n := 0
	for _, p := range r.prestamosEnOrden() {
		if p.AvisadoCierrePara != nil && p.DevueltoEn == nil {
			n++
		}
	}
	return n, nil
}

func diaDe(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// EnTransaccion imita el todo-o-nada de Postgres: saca una copia del estado
// antes de correr fn y la restaura si fn falla.
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

// CrearPrestamo reproduce el índice único parcial ux_prestamo_abierto: un
// equipo no puede tener dos préstamos abiertos.
func (r *fakeRepo) CrearPrestamo(ctx context.Context, p *domain.Prestamo) error {
	for _, existente := range r.prestamos {
		if existente.EquipoID == p.EquipoID && existente.EstaAbierto() {
			return ErrEquipoYaPrestado
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

func (r *fakeRepo) BuscarPrestamoAbiertoDeEquipo(ctx context.Context, equipoID string) (*domain.Prestamo, error) {
	for _, p := range r.prestamos {
		if p.EquipoID == equipoID && p.EstaAbierto() {
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

func (r *fakeRepo) ListarPrestamosDeEquipo(ctx context.Context, equipoID string, limite int) ([]*PrestamoDetallado, error) {
	var resultado []*PrestamoDetallado
	for _, p := range r.prestamosEnOrden() {
		if p.EquipoID == equipoID && len(resultado) < limite {
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
		if f.EquipoID != nil && res.EquipoID != *f.EquipoID {
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
			Reserva:       res,
			Identificador: 1,
			CarroNombre:   "Carro de test",
			MateriaNombre: "Matemáticas",
			CursoNombre:   "1°A",
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

func (r *fakeRepo) CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) ([]BloqueCalendario, error) {
	var resultado []BloqueCalendario
	for _, res := range r.reservas {
		if res.EquipoID != equipoID || res.Estado == domain.ReservaCancelada {
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

// ListarReservasFuturasDeEquipo devuelve ORDENADO por fecha y hora, como el
// repo real: quien llama puede necesitar LA PRÓXIMA, no una cualquiera.
func (r *fakeRepo) BuscarSolapamientos(ctx context.Context, equipoIDs []string, fechas []time.Time, horaInicio, horaFin time.Duration) ([]Solapamiento, error) {
	if r.errBuscarSolapamientos != nil {
		return nil, r.errBuscarSolapamientos
	}
	pedidos := map[string]bool{}
	for _, id := range equipoIDs {
		pedidos[id] = true
	}

	var resultado []Solapamiento
	for _, res := range r.reservas {
		if !pedidos[res.EquipoID] || res.Estado != domain.ReservaConfirmada {
			continue
		}
		enAlgunaFecha := false
		for _, f := range fechas {
			if mismaFecha(res.Fecha, f) {
				enAlgunaFecha = true
				break
			}
		}
		if !enAlgunaFecha || !res.SolapaCon(horaInicio, horaFin) {
			continue
		}
		sol := Solapamiento{
			EquipoID: res.EquipoID, Etiqueta: "PC " + res.EquipoID,
			Fecha: res.Fecha, HoraInicio: res.HoraInicio, HoraFin: res.HoraFin,
			MotivoBloqueo: res.MotivoBloqueo,
		}
		if res.NombreDocenteSnapshot != nil {
			sol.Docente = *res.NombreDocenteSnapshot
		}
		resultado = append(resultado, sol)
	}
	sort.Slice(resultado, func(i, j int) bool {
		if !resultado[i].Fecha.Equal(resultado[j].Fecha) {
			return resultado[i].Fecha.Before(resultado[j].Fecha)
		}
		return resultado[i].EquipoID < resultado[j].EquipoID
	})
	return resultado, nil
}

func (r *fakeRepo) ListarReservasFuturasDeEquipo(ctx context.Context, equipoID string, desde time.Time) ([]*domain.Reserva, error) {
	var resultado []*domain.Reserva
	for _, res := range r.reservas {
		if res.EquipoID != equipoID || res.Estado != domain.ReservaConfirmada {
			continue
		}
		if domain.YaTermino(res.Fecha, res.HoraInicio, res.HoraFin, desde) {
			continue
		}
		resultado = append(resultado, res)
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

// ListarReservasFuturas: solo las de clase, como en la consulta real. Los
// bloqueos administrativos nunca estuvieron sujetos a la jornada.
func (r *fakeRepo) ListarReservasFuturas(ctx context.Context, desde time.Time) ([]ReservaDetallada, error) {
	var resultado []ReservaDetallada
	for _, res := range r.reservas {
		if res.Estado != domain.ReservaConfirmada || res.Tipo != domain.TipoNormal {
			continue
		}
		resultado = append(resultado, ReservaDetallada{Reserva: res, Identificador: r.identificadorDeEquipo[res.EquipoID],
			Etiqueta: fmt.Sprintf("PC %d", r.identificadorDeEquipo[res.EquipoID])})
	}
	return resultado, nil
}

func (r *fakeRepo) EliminarReservasYGruposDeCiclo(ctx context.Context, cicloID string) (int, int, error) {
	// El fake no modela la relación ciclo→materia→grupo/reserva (viviría del
	// lado de academic), así que solo se usa para confirmar que el método existe
	// y es invocable desde los tests que ejercitan la cascada — el
	// comportamiento real se prueba en infrastructure/ contra Postgres de
	// verdad, donde sí existen esas tablas.
	return 0, 0, nil
}

// ListarReservasConfirmadasVencidas reproduce la consulta real: las
// CONFIRMADA cuya franja ya terminó. Devolvía nil, y con eso ningún test podía
// ver qué le pasa a una reserva que nadie tocó cuando se le acaba el horario
// —que es justo lo que hay que saber el día que no hubo nadie en el mostrador
// (RF-07.6)—.
func (r *fakeRepo) ListarReservasConfirmadasVencidas(ctx context.Context, ahora time.Time, limite int) ([]*domain.Reserva, error) {
	var vencidas []*domain.Reserva
	for _, res := range r.enOrden() {
		if res.Estado != domain.ReservaConfirmada {
			continue
		}
		if domain.YaTermino(res.Fecha, res.HoraInicio, res.HoraFin, ahora) {
			vencidas = append(vencidas, res)
		}
		if len(vencidas) >= limite {
			break
		}
	}
	return vencidas, nil
}
func (r *fakeRepo) ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, materiaID string) ([]EquipoDisponible, error) {
	r.materiaRecibidaAlListar = materiaID
	return r.pcsDisponibles, nil
}

func (r *fakeRepo) ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]EquipoOcupado, error) {
	return r.pcsOcupadas, nil
}

func (r *fakeRepo) ListarEquiposLibresEnLaSerie(ctx context.Context, grupoID string) ([]EquipoDisponible, error) {
	return r.pcsDisponibles, nil
}

// ReservasDeLaSerieDesde reproduce la consulta real: la misma máquina, en las
// ocurrencias que le quedan a la serie.
func (r *fakeRepo) ReservasDeLaSerieDesde(ctx context.Context, reservaID string) ([]*domain.Reserva, error) {
	origen, ok := r.reservas[reservaID]
	if !ok || origen.ReservaGrupoID == nil {
		return nil, nil
	}
	grupoOrigen, ok := r.grupos[*origen.ReservaGrupoID]
	if !ok || grupoOrigen.ReglaRecurrenciaID == nil {
		return nil, nil
	}

	var resultado []*domain.Reserva
	for _, res := range r.enOrden() {
		if res.Estado != domain.ReservaConfirmada || res.EquipoID != origen.EquipoID ||
			res.ReservaGrupoID == nil {
			continue
		}
		g, ok := r.grupos[*res.ReservaGrupoID]
		if !ok || g.ReglaRecurrenciaID == nil ||
			*g.ReglaRecurrenciaID != *grupoOrigen.ReglaRecurrenciaID {
			continue
		}
		if g.Fecha.Before(grupoOrigen.Fecha) {
			continue
		}
		resultado = append(resultado, res)
	}
	return resultado, nil
}

func (r *fakeRepo) DatosParaPedirLiberacion(ctx context.Context, reservaID string) (*ReservaParaPedido, error) {
	res, ok := r.reservas[reservaID]
	if !ok {
		return nil, ErrReservaNoEncontrada
	}
	p := &ReservaParaPedido{
		Estado:     res.Estado,
		EsBloqueo:  res.Tipo == domain.TipoBloqueo,
		DuenoID:    res.CreadoPor,
		Etiqueta:   fmt.Sprintf("PC %d", r.identificadorDeEquipo[res.EquipoID]),
		Fecha:      res.Fecha,
		HoraInicio: res.HoraInicio,
		HoraFin:    res.HoraFin,
	}
	if res.MateriaID != nil {
		p.MateriaNombre = *res.MateriaID
	}
	if res.CreadoPor != nil {
		if c, ok := r.contactoDeUsuario[*res.CreadoPor]; ok {
			p.DuenoNombre, p.DuenoEmail = c[0], c[1]
		}
	}
	return p, nil
}

func (r *fakeRepo) YaPidioLiberacionHoy(ctx context.Context, reservaID, solicitanteID string, dia time.Time) (bool, error) {
	return r.pedidosDeLiberacion[reservaID+"/"+solicitanteID+"/"+diaDe(dia).Format("2006-01-02")], nil
}

func (r *fakeRepo) CrearReglaRecurrencia(ctx context.Context, regla *domain.ReglaRecurrencia) error {
	r.reglas[regla.ID] = regla
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

type fakeValidadorEquipo struct {
	disponible         bool
	errIdentificadores error
	// fueraDelInventario: PCs dadas de baja. Es lo único que distingue
	// "no se puede reservar" de "no se puede ni entregar".
	fueraDelInventario map[string]bool
	// estados: el estado de circulación de cada equipo. Vacío es DISPONIBLE,
	// que es el caso de casi todos los tests.
	estados map[string]string
	// Los dos contadores existen para poder afirmar que la validación de un
	// lote es UNA consulta y no una por equipo.
	vecesNoReservables    int
	vecesDisponibleDeAUna int
}

func (f *fakeValidadorEquipo) EquipoDisponibleParaReservar(ctx context.Context, equipoID string) (bool, error) {
	f.vecesDisponibleDeAUna++
	return f.disponible, nil
}

// EquiposNoReservables: la versión de lote, coherente con la de a una.
func (f *fakeValidadorEquipo) EquiposNoReservables(ctx context.Context, equipoIDs []string) ([]string, error) {
	f.vecesNoReservables++
	if f.disponible {
		return nil, nil
	}
	return equipoIDs, nil
}

// EquipoEstaEnInventario es más laxo que reservar: una PC en mantenimiento no
// se puede reservar y su calendario sí se mira.
func (f *fakeValidadorEquipo) EquipoEstaEnInventario(ctx context.Context, equipoID string) (bool, error) {
	return !f.fueraDelInventario[equipoID], nil
}

// CondicionParaEntregar: en el inventario si no lo sacaron, y DISPONIBLE
// salvo que el test diga otra cosa.
func (f *fakeValidadorEquipo) CondicionParaEntregar(ctx context.Context, equipoID string) (CondicionDeEquipo, error) {
	estado := f.estados[equipoID]
	if estado == "" {
		estado = "DISPONIBLE"
	}
	return CondicionDeEquipo{
		EnInventario: !f.fueraDelInventario[equipoID],
		Estado:       estado,
	}, nil
}

// EtiquetasDeEquipos: en los tests los equipos se llaman "pc1", "pc2"… así
// que el número visible sale del sufijo.
func (f *fakeValidadorEquipo) EtiquetasDeEquipos(ctx context.Context, equipoIDs []string) (map[string]string, error) {
	if f.errIdentificadores != nil {
		return nil, f.errIdentificadores
	}
	m := make(map[string]string, len(equipoIDs))
	for _, id := range equipoIDs {
		var n int
		if _, err := fmt.Sscanf(id, "pc%d", &n); err == nil {
			m[id] = fmt.Sprintf("PC %d", n)
		}
	}
	return m, nil
}

type fakeObtenedorNombre struct {
	nombre string
	// contactos, si está, es lo que devuelve ContactosDe; en nil, arma uno
	// con el nombre de arriba para cada id que le pidan.
	contactos map[string]Contacto
}

func (f *fakeObtenedorNombre) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	return f.nombre, nil
}

func (f *fakeObtenedorNombre) ContactosDe(ctx context.Context, usuarioIDs []string) (map[string]Contacto, error) {
	if f.contactos != nil {
		return f.contactos, nil
	}
	contactos := make(map[string]Contacto, len(usuarioIDs))
	for _, id := range usuarioIDs {
		contactos[id] = Contacto{Nombre: f.nombre, Email: id + "@escuela.edu.ar"}
	}
	return contactos, nil
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
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada Lovelace"},
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

// El fin de semana dejó de estar prohibido por el código.
func TestCrearReserva_FinDeSemana_SePuede(t *testing.T) {
	for nombre, dia := range map[string]time.Time{
		"sábado":  fecha(2026, 3, 14),
		"domingo": fecha(2026, 3, 15),
	} {
		svc := nuevoServicioDeTest(nuevoFakeRepo())

		grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
			dia, 8*time.Hour, 9*time.Hour, []string{"pc1"})

		if err != nil {
			t.Errorf("%s: esperaba que se pudiera reservar, obtuve %v", nombre, err)
			continue
		}
		if grupo == nil || len(reservas) != 1 {
			t.Errorf("%s: esperaba un grupo con una reserva, obtuve grupo=%v reservas=%d",
				nombre, grupo, len(reservas))
		}
	}
}

// Con jornada declarada, lo que cae afuera se rechaza.
func TestCrearReserva_FueraDeLaJornada_Error(t *testing.T) {
	svc := NewService(
		nuevoFakeRepo(),
		&fakeValidadorMateria{asignado: true},
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: false},
		&fakeObtenedorNombre{nombre: "Ada Lovelace"},
		idSecuencial,
		func() time.Time { return fecha(2026, 3, 2) },
		eventbus.NewInMemoryEventBus(),
	)

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrFueraDeJornada) {
		t.Fatalf("esperaba ErrFueraDeJornada, obtuve %v", err)
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

func TestCrearReserva_SinEquipos_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, nil)

	if !errors.Is(err, ErrSinEquipos) {
		t.Fatalf("esperaba ErrSinEquipos, obtuve %v", err)
	}
}

func TestCrearReserva_DocenteNoAsignado_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: false}, &fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrDocenteNoAsignado) {
		t.Fatalf("esperaba ErrDocenteNoAsignado, obtuve %v", err)
	}
}

func TestCrearReserva_EquipoNoDisponible_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: true}, &fakeValidadorEquipo{disponible: false},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"})

	if !errors.Is(err, ErrEquipoNoDisponible) {
		t.Fatalf("esperaba ErrEquipoNoDisponible, obtuve %v", err)
	}
}

func TestCrearReserva_Solapamiento_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	existente := &domain.Reserva{
		ID: "existente", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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

func TestCrearReserva_MismaEquipoOtroDia_NoSolapaAunqueMismoHorario(t *testing.T) {
	repo := nuevoFakeRepo()
	existente := &domain.Reserva{
		ID: "existente", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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
		ID: "cancelada", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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
		if err := svc.CancelarReserva(context.Background(), r.ID, &canceladoPor, "Bloqueo administrativo"); err != nil {
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

func TestCancelarReserva_DeBloqueoAdministrativo_NoTocaNingunGrupo(t *testing.T) {
	// Un bloqueo administrativo no pertenece a ningún ReservaGrupo — cancelarlo
	// no debe intentar buscar/actualizar ningún grupo (ni panickear).
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, Tipo: domain.TipoBloqueo}
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
// docente tres avisos idénticos.
func TestBloquearEquipos_UnSoloEventoPorDocente(t *testing.T) {
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
	res, err := svc.BloquearEquipos(context.Background(), []string{"pc1", "pc2", "pc3"}, &admin,
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
	equipos := map[string]bool{}
	for _, r := range payload.Reservas {
		equipos[r.Etiqueta] = true
	}
	for _, esperada := range []string{"PC 1", "PC 2", "PC 3"} {
		if !equipos[esperada] {
			t.Errorf("falta la %s en el detalle: %+v", esperada, payload.Reservas)
		}
	}
}

// Dos docentes afectados por el mismo bloqueo reciben un aviso cada uno, no
// uno con las reservas del otro adentro.
func TestBloquearEquipos_UnEventoPorCadaDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	for _, docente := range []string{"docente1", "docente2"} {
		equipo := "pc1"
		if docente == "docente2" {
			equipo = "pc2"
		}
		if _, _, err := svc.CrearReserva(context.Background(), "materia1", docente, false,
			fecha(2026, 3, 9), 10*time.Hour, 12*time.Hour, []string{equipo}); err != nil {
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
	if _, err := svc.BloquearEquipos(context.Background(), []string{"pc1", "pc2"}, &admin,
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
// detalle: quedarse sin notificar por no poder adornar el mensaje sería mucho
// peor que un mensaje menos específico.
func TestPublicarCancelaciones_SinIdentificadores_ElAvisoSaleIgual(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo, &fakeValidadorMateria{asignado: true},
		&fakeValidadorEquipo{disponible: true, errIdentificadores: errors.New("inventory caído")},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"}, idSecuencial,
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

// RF-05.1 es "tu reserva fue cancelada POR UN ADMIN".
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

// El motivo viaja sin el "Tu reserva fue cancelada:" — esa frase la pone el
// suscriptor de notification.
func TestBloquearEquipos_ElMotivoNoTraeElPrefijoDelAviso(t *testing.T) {
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
	if _, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, &admin,
		fecha(2026, 3, 9), 9*time.Hour, 11*time.Hour, "Aprender 2026"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	select {
	case e := <-recibido:
		motivo := e.Payload.(eventbus.CancelacionesDeUsuario).Motivo
		if strings.Contains(motivo, "Tu reserva fue cancelada") {
			t.Errorf("el motivo no debe traer el prefijo del aviso: %q", motivo)
		}
		// El motivo del Admin va tal cual, sin envolverlo en ninguna categoría: si
		// escribió "jornada docente", el docente cancelado tiene que leer
		// exactamente eso.
		if motivo != "los equipos quedaron bloqueados: Aprender 2026" {
			t.Errorf("motivo inesperado: %q", motivo)
		}
	case <-time.After(time.Second):
		t.Fatal("nunca se publicó el evento reserva.cancelada")
	}
}

func TestCancelarReserva_BloqueoAdministrativoCancelado_NoPublicaEvento(t *testing.T) {
	// Un bloqueo administrativo no tiene CreadoPor de un docente afectado que
	// notificar de la misma forma — no debería publicar nada (o al menos no
	// debería panickear al no tener a quién avisar).
	repo := nuevoFakeRepo()
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", Estado: domain.ReservaConfirmada, Tipo: domain.TipoBloqueo, CreadoPor: nil}
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

	// Marzo 2026: los lunes son 2, 9, 16, 23, 30 — el mock "ahora" es lunes 2/3
	// al mediodía, así que arrancamos la regla desde ahí, a la tarde: la primera
	// ocurrencia es hoy pero todavía no empezó.
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
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: false}, &fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

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
		ID: "existente", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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

// ── BloquearEquipos ──────────────────────────────────────────────

func TestBloquearEquipos_SinConflictos_OK(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	res, err := svc.BloquearEquipos(context.Background(), []string{"pc1", "pc2"}, &creadoPor,
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
		if b.Tipo != domain.TipoBloqueo {
			t.Errorf("tipo incorrecto: %s", b.Tipo)
		}
	}
}

func TestBloquearEquipos_EnElPasado_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	_, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 2, 27), 10*time.Hour, 12*time.Hour, "Evaluación provincial")

	if !errors.Is(err, domain.ErrReservaEnElPasado) {
		t.Fatalf("esperaba ErrReservaEnElPasado, obtuve %v", err)
	}
}

// El tope de duración no alcanza a RF-04.7: si el Admin necesita el
// laboratorio el día entero para una evaluación, es su decisión — mismo
// criterio que la exención de EsDiaLectivo.
func TestBloquearEquipos_DiaEntero_SeAcepta(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	creadoPor := "admin1"

	res, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 3, 9), 0, 23*time.Hour+59*time.Minute, "Evaluación provincial")

	if err != nil {
		t.Fatalf("un bloqueo de día completo debería aceptarse: %v", err)
	}
	if len(res.Bloqueos) != 1 {
		t.Fatalf("esperaba 1 bloqueo, obtuve %d", len(res.Bloqueos))
	}
}

func TestBloquearEquipos_CancelaReservaQueSeSolapa(t *testing.T) {
	repo := nuevoFakeRepo()
	docenteAfectado := "docente-afectado"
	repo.reservas["existente"] = &domain.Reserva{
		ID: "existente", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 10 * time.Hour, HoraFin: 11 * time.Hour,
		Estado: domain.ReservaConfirmada, Tipo: domain.TipoNormal, CreadoPor: &docenteAfectado,
	}
	svc := nuevoServicioDeTest(repo)
	creadoPor := "admin1"

	res, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, &creadoPor,
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

func TestBloquearEquipos_NoCancelaOtroBloqueoAdministrativo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.reservas["otro-bloqueo"] = &domain.Reserva{
		ID: "otro-bloqueo", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
		HoraInicio: 10 * time.Hour, HoraFin: 11 * time.Hour,
		Estado: domain.ReservaConfirmada, Tipo: domain.TipoBloqueo,
	}
	svc := nuevoServicioDeTest(repo)
	creadoPor := "admin1"

	res, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, &creadoPor,
		fecha(2026, 3, 9), 9*time.Hour, 12*time.Hour, "Evaluación provincial")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if res.ReservasCanceladas != 0 {
		t.Error("un bloqueo administrativo existente no debería cancelarse por otro bloqueo")
	}
	if repo.reservas["otro-bloqueo"].Estado != domain.ReservaConfirmada {
		t.Error("el otro bloqueo debería seguir confirmado")
	}
}

func TestBloquearEquipos_EquipoNoDisponible_Error(t *testing.T) {
	svc := NewService(nuevoFakeRepo(), &fakeValidadorMateria{asignado: true}, &fakeValidadorEquipo{disponible: false},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"}, idSecuencial, func() time.Time { return fecha(2026, 3, 2) }, eventbus.NewInMemoryEventBus())

	_, err := svc.BloquearEquipos(context.Background(), []string{"pc1"}, nil,
		fecha(2026, 3, 9), 9*time.Hour, 12*time.Hour, "motivo")

	if !errors.Is(err, ErrEquipoNoDisponible) {
		t.Fatalf("esperaba ErrEquipoNoDisponible, obtuve %v", err)
	}
}

// ── FinalizarVencidas ───────────────────────────────────────────────────

type fakeRepoConVencidas struct {
	*fakeRepo
	vencidas []*domain.Reserva
}

// Imita al repo real en las dos cosas de las que depende el job por lotes:
// solo devuelve lo que sigue CONFIRMADA (lo ya finalizado sale del conjunto)
// y respeta el límite.
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
// ListarReservasConfirmadasVencidas.
func (f *fakeRepoConVencidas) EnTransaccion(ctx context.Context, fn func(Repo) error) error {
	return fn(f)
}

// El job leía TODO lo vencido en una sola transacción, y "todo lo vencido"
// crece con cada hora que el proceso haya estado caído.
func TestFinalizarVencidas_AtrasoMayorQueUnLote_LoProcesaEntero(t *testing.T) {
	base := nuevoFakeRepo()
	cantidad := loteFinalizarVencidas*2 + 37

	vencidas := make([]*domain.Reserva, 0, cantidad)
	for i := 0; i < cantidad; i++ {
		id := fmt.Sprintf("r%d", i)
		r := &domain.Reserva{
			ID: id, EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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
// transicionar se saltea, y el lote entero no avanza.
func TestFinalizarVencidas_LoteSinProgreso_NoSeQuedaEnBucle(t *testing.T) {
	base := nuevoFakeRepo()

	// Estado terminal: ListarReservasConfirmadasVencidas del repo real nunca
	// la devolvería, pero el fake la fuerza para simular ese estado imposible.
	rara := &domain.Reserva{
		ID: "rara", EquipoID: "pc1", Fecha: fecha(2026, 3, 9),
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
// haya pasado antes — es el escenario que hace falta para probar el corte por
// falta de progreso.
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
	repo.reservas["r1"] = &domain.Reserva{ID: "r1", EquipoID: "pc1"}
	svc := nuevoServicioDeTest(repo)

	r, err := svc.ObtenerReserva(context.Background(), "r1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if r.EquipoID != "pc1" {
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

// ── CancelarReservasFuturasDeEquipo (cascada hacia inventory) ───────────────

func TestCancelarReservasFuturasDeEquipo_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, 3, 9), 8*time.Hour, 9*time.Hour, []string{"pc1", "pc2"})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	canceladas, notificados, err := svc.CancelarReservasFuturasDeEquipo(context.Background(), "pc1", "PC dada de baja")

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

func TestCancelarReservasFuturasDeEquipo_SinReservas_NoHaceNada(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	canceladas, notificados, err := svc.CancelarReservasFuturasDeEquipo(context.Background(), "equipo-sin-reservas", "motivo")

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
// DocenteMateria) Y CUALQUIER ADMIN".
func TestCrearReserva_UnAdminNoAsignadoPuedeReservar(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: false},
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Admin Inicial"},
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
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"},
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
// cerrado no admite reservas nuevas aunque el registro se conserve".
func TestCrearReserva_MateriaArchivada_NoAdmiteReservas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := NewService(repo,
		&fakeValidadorMateria{asignado: true, archivada: true},
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"},
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
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Admin"},
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
		&fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Ada"},
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

func TestBloquearEquipos_SiFallaElBloqueoNoQuedaNingunGrupoTocado(t *testing.T) {
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
	_, err = svc.BloquearEquipos(context.Background(), []string{"pc1"}, &admin,
		fecha(2026, 3, 9), 8*time.Hour, 10*time.Hour, "prueba estatal")
	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba ErrSolapamiento, obtuve %v", err)
	}

	if estado := base.reservas[reservas[0].ID].Estado; estado != domain.ReservaConfirmada {
		t.Errorf("la reserva debería seguir CONFIRMADA tras el rollback, quedó %s", estado)
	}
	// Ojo: este test NO detecta por sí solo una escritura fuera de la
	// transacción — el fakeRepo restaura sus mapas enteros, sin importar quién
	// los escribió.
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

// ── TieneReservasFuturasDeEquipo (lo que usa el reintento de inventory) ─

func TestTieneReservasFuturasDeEquipo_ConReservaConfirmada_True(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("preparando la reserva: %v", err)
	}

	tiene, err := svc.TieneReservasFuturasDeEquipo(context.Background(), "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !tiene {
		t.Error("la PC tiene una reserva confirmada por delante: esperaba true")
	}
}

func TestTieneReservasFuturasDeEquipo_SinReservas_False(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	tiene, err := svc.TieneReservasFuturasDeEquipo(context.Background(), "equipo-sin-reservas")
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
func TestTieneReservasFuturasDeEquipo_DespuesDeLaCascada_False(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 9), 8*time.Hour, 9*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("preparando la reserva: %v", err)
	}
	if _, _, err := svc.CancelarReservasFuturasDeEquipo(context.Background(), "pc1", "PC dada de baja"); err != nil {
		t.Fatalf("cascada: %v", err)
	}

	tiene, err := svc.TieneReservasFuturasDeEquipo(context.Background(), "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if tiene {
		t.Error("después de la cascada no queda nada pendiente: esperaba false")
	}
}

// ── Tamaño del lote ─────────────────────────────────────────────────────

// El pedido lo arma el cliente, así que el tamaño del lote es entrada como
// cualquier otra.
func TestCrearReserva_DemasiadosEquipos_SeRechaza(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	muchos := make([]string, MaxEquiposPorOperacion+1)
	for i := range muchos {
		muchos[i] = fmt.Sprintf("pc%d", i)
	}

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 3), 8*time.Hour, 9*time.Hour, muchos)

	if !errors.Is(err, ErrDemasiadosEquipos) {
		t.Fatalf("esperaba ErrDemasiadosEquipos, obtuve %v", err)
	}
	// Se corta ANTES de tocar la base: validar el tamaño después de haber
	// consultado por cada elemento sería llegar tarde.
	if len(repo.reservas) != 0 {
		t.Error("no tendría que haber creado ninguna reserva")
	}
}

// El tope no puede molestar al uso real: un carro entero tiene que entrar.
func TestCrearReserva_UnCarroEnteroEntraSinProblema(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	carro := make([]string, 64)
	for i := range carro {
		carro[i] = fmt.Sprintf("pc%d", i)
	}

	_, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 3), 8*time.Hour, 9*time.Hour, carro)

	if err != nil {
		t.Fatalf("64 equipos tienen que entrar: %v", err)
	}
	if len(reservas) != 64 {
		t.Errorf("esperaba 64 reservas, obtuve %d", len(reservas))
	}
}

// La validación de disponibilidad pasó a ser una sola consulta de lote: con
// un bucle, un bloqueo sobre un carro entero eran 64 idas a la base antes de
// escribir la primera fila.
func TestCrearReserva_ValidaLaDisponibilidadEnUnaSolaConsulta(t *testing.T) {
	repo := nuevoFakeRepo()
	validador := &fakeValidadorEquipo{disponible: true}
	svc := servicioConValidador(repo, validador)

	carro := make([]string, 64)
	for i := range carro {
		carro[i] = fmt.Sprintf("pc%d", i)
	}

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 3), 8*time.Hour, 9*time.Hour, carro); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if validador.vecesNoReservables != 1 {
		t.Errorf("esperaba 1 consulta de lote, hubo %d", validador.vecesNoReservables)
	}
	if validador.vecesDisponibleDeAUna != 0 {
		t.Errorf("no tendría que preguntar de a una: %d veces", validador.vecesDisponibleDeAUna)
	}
}

// El 409 tiene que decir QUÉ chocó.
func TestCrearReserva_Solapamiento_ElMensajeNombraLoQueChoco(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	dia := fecha(2026, 3, 9)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		dia, 10*time.Hour, 12*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("la primera reserva no debería fallar: %v", err)
	}

	_, _, err := svc.CrearReserva(context.Background(), "materia1", "docente2", true,
		dia, 11*time.Hour, 13*time.Hour, []string{"pc1", "pc2"})

	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba un solapamiento, obtuve %v", err)
	}
	var detallado *ErrorDeSolapamiento
	if !errors.As(err, &detallado) {
		t.Fatalf("esperaba un *ErrorDeSolapamiento, obtuve %T", err)
	}
	if len(detallado.Conflictos) != 1 || detallado.Conflictos[0].EquipoID != "pc1" {
		t.Fatalf("esperaba el conflicto de pc1, obtuve %+v", detallado.Conflictos)
	}
	// Lo que llega al docente: el equipo, el día, la franja y quién la tiene.
	mensaje := err.Error()
	for _, esperado := range []string{"PC pc1", "10:00", "12:00", "Ada Lovelace"} {
		if !strings.Contains(mensaje, esperado) {
			t.Errorf("el mensaje %q no menciona %q", mensaje, esperado)
		}
	}
}

// Los bordes que se tocan no se pisan: una clase de 8 a 10 y otra de 10 a 12
// sobre el mismo equipo es el caso más común que existe.
func TestCrearReserva_HorariosContiguos_NoSolapan(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())
	dia := fecha(2026, 3, 9)

	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		dia, 8*time.Hour, 10*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if _, _, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		dia, 10*time.Hour, 12*time.Hour, []string{"pc1"}); err != nil {
		t.Fatalf("una reserva contigua no debería chocar: %v", err)
	}
}

// ── Cambiar la máquina de una serie (RF-08.14) ──────────────────────────

// ptr: los campos de titularidad son *string porque una cuenta eliminada los
// deja en NULL. En los tests siempre hay dueño.
func ptr(s string) *string { return &s }

// serieDeCincoLunes deja creada una recurrencia de marzo sobre pc1 y devuelve
// las reservas en orden de fecha.
func serieDeCincoLunes(t *testing.T, svc *Service, repo *fakeRepo) []*domain.Reserva {
	t.Helper()
	res, err := svc.CrearReservaRecurrente(context.Background(), "materia1", "docente1", false,
		domain.Lunes, 14*time.Hour, 15*time.Hour,
		fecha(2026, time.March, 2), fecha(2026, time.March, 30), []string{"pc1"})
	if err != nil {
		t.Fatalf("armando la serie: %v", err)
	}
	if len(res.Grupos) != 5 {
		t.Fatalf("esperaba 5 ocurrencias, obtuve %d", len(res.Grupos))
	}

	var reservas []*domain.Reserva
	for _, g := range res.Grupos {
		delGrupo, err := repo.ListarReservasPorGrupo(context.Background(), g.ID)
		if err != nil {
			t.Fatalf("leyendo el grupo: %v", err)
		}
		reservas = append(reservas, delGrupo...)
	}
	return reservas
}

func TestCambiarEquipoDeReserva_SoloEsta_NoTocaLasDemas(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	reservas := serieDeCincoLunes(t, svc, repo)

	_, err := svc.CambiarEquipoDeReserva(context.Background(), reservas[0].ID, "pc9",
		"docente1", false, true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.reservas[reservas[0].ID].EquipoID != "pc9" {
		t.Error("la ocurrencia elegida tiene que cambiar")
	}
	for _, r := range reservas[1:] {
		if repo.reservas[r.ID].EquipoID != "pc1" {
			t.Errorf("la ocurrencia del %s no tenía que cambiar", r.Fecha.Format("2006-01-02"))
		}
	}
}

func TestCambiarEquipoDeReserva_EstaYLasSiguientes(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	reservas := serieDeCincoLunes(t, svc, repo)

	// Desde la tercera en adelante.
	_, err := svc.CambiarEquipoDeReserva(context.Background(), reservas[2].ID, "pc9",
		"docente1", false, false)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Las dos primeras no se tocan: lo que ya se dio no se reescribe.
	for _, r := range reservas[:2] {
		if repo.reservas[r.ID].EquipoID != "pc1" {
			t.Errorf("la ocurrencia del %s es anterior: no se toca", r.Fecha.Format("2006-01-02"))
		}
	}
	for _, r := range reservas[2:] {
		if repo.reservas[r.ID].EquipoID != "pc9" {
			t.Errorf("la ocurrencia del %s tenía que cambiar", r.Fecha.Format("2006-01-02"))
		}
	}
}

// El caso que justifica validar todo antes de tocar nada: si el equipo nuevo
// choca en una sola de las fechas, no se cambia ninguna.
func TestCambiarEquipoDeReserva_EnSerie_UnChoqueNoCambiaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	reservas := serieDeCincoLunes(t, svc, repo)

	// Otro docente ya tiene pc9 el cuarto lunes, en la misma franja.
	ocupada, err := domain.NuevaReservaNormal("ajena", "grupo-ajeno", "pc9", "materia2",
		"Otro", ptr("otro-docente"), reservas[3].Fecha, 14*time.Hour, 15*time.Hour,
		fecha(2026, time.March, 1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[ocupada.ID] = ocupada

	_, err = svc.CambiarEquipoDeReserva(context.Background(), reservas[0].ID, "pc9",
		"docente1", false, false)

	if err == nil {
		t.Fatal("tenía que rechazar el cambio")
	}
	if !errors.Is(err, ErrSolapamiento) {
		t.Fatalf("esperaba un solapamiento, obtuve: %v", err)
	}
	for _, r := range reservas {
		if repo.reservas[r.ID].EquipoID != "pc1" {
			t.Errorf("no tenía que cambiar ninguna, y cambió la del %s",
				r.Fecha.Format("2006-01-02"))
		}
	}
}

// Una reserva suelta no tiene serie: "esta y las siguientes" no significa
// nada distinto de "solo esta", y rechazar el pedido por eso sería inventar
// un error para el caso más común.
func TestCambiarEquipoDeReserva_SinSerie_ElAlcanceNoCambiaNada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	grupo, reservas, err := svc.CrearReserva(context.Background(), "materia1", "docente1", false,
		fecha(2026, time.March, 3), 14*time.Hour, 15*time.Hour, []string{"pc1"})
	if err != nil {
		t.Fatalf("creando la reserva: %v", err)
	}
	_ = grupo

	if _, err := svc.CambiarEquipoDeReserva(context.Background(), reservas[0].ID, "pc9",
		"docente1", false, false); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if repo.reservas[reservas[0].ID].EquipoID != "pc9" {
		t.Error("la reserva suelta tiene que cambiar igual")
	}
}

// ── Pedir la liberación de una reserva ajena (RF-04.12) ─────────────────

func TestPedirLiberacionDeReserva_PublicaElAviso(t *testing.T) {
	repo := nuevoFakeRepo()
	bus := &busEspia{}
	svc := NewService(repo, &fakeValidadorMateria{asignado: true}, &fakeValidadorEquipo{disponible: true},
		&fakeValidadorJornada{permite: true}, &fakeObtenedorNombre{nombre: "Grace Hopper"}, idSecuencial,
		func() time.Time { return fecha(2026, 3, 2) }, bus)

	repo.contactoDeUsuario["otro-docente"] = [2]string{"Ada Lovelace", "ada@escuela.edu.ar"}
	repo.identificadorDeEquipo["pc1"] = 3
	r, err := domain.NuevaReservaNormal("res1", "grupo1", "pc1", "materia1", "Ada Lovelace",
		ptr("otro-docente"), fecha(2026, time.March, 4), 10*time.Hour, 12*time.Hour,
		fecha(2026, time.March, 1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[r.ID] = r

	if err := svc.PedirLiberacionDeReserva(context.Background(), "res1", "docente1",
		"La necesito para una evaluación"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	eventos := bus.de("reserva.pedido-de-liberacion")
	if len(eventos) != 1 {
		t.Fatalf("esperaba 1 evento, hubo %d", len(eventos))
	}
	pedido := eventos[0].Payload.(eventbus.PedidoDeLiberacion)
	// Va al DUEÑO y habla de quien pide: sin el nombre, el aviso llega
	// anónimo y no sabe con quién hablar.
	if pedido.UsuarioID != "otro-docente" || pedido.Email != "ada@escuela.edu.ar" {
		t.Errorf("el aviso tiene que ir al dueño: %+v", pedido)
	}
	if pedido.SolicitanteNombre != "Grace Hopper" || pedido.SolicitanteID != "docente1" {
		t.Errorf("falta quién pide: %+v", pedido)
	}
	if pedido.Mensaje != "La necesito para una evaluación" {
		t.Errorf("el texto libre viaja tal cual: %q", pedido.Mensaje)
	}

	// Y lo más importante: no tocó nada.
	if repo.reservas["res1"].Estado != domain.ReservaConfirmada ||
		repo.reservas["res1"].EquipoID != "pc1" {
		t.Error("el pedido no puede cambiar la reserva")
	}
}

// Una clase en curso ya no se puede liberar: el docente está usando esas
// máquinas.
func TestPedirLiberacionDeReserva_FranjaYaEmpezada(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo) // el reloj está en el lunes 2/3 al mediodía

	r, err := domain.NuevaReservaNormal("res1", "grupo1", "pc1", "materia1", "Ada",
		ptr("otro-docente"), fecha(2026, time.March, 2), 8*time.Hour, 14*time.Hour,
		fecha(2026, time.March, 1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[r.ID] = r

	err = svc.PedirLiberacionDeReserva(context.Background(), "res1", "docente1", "")

	if !errors.Is(err, ErrReservaYaEmpezada) {
		t.Fatalf("esperaba ErrReservaYaEmpezada, obtuve: %v", err)
	}
}

// fakeValidadorJornada hace de la jornada declarada por la institución.
//
// `cierre` en nil = la institución no declaró jornada, que es el caso de casi
// todos estos tests: ahí el corte del barrido cae a la hora configurada.
type fakeValidadorJornada struct {
	permite bool
	cierre  func(fecha time.Time) CierreDeJornada
}

func (f *fakeValidadorJornada) PermiteReserva(_ context.Context, _ time.Time, _, _ time.Duration) (bool, error) {
	return f.permite, nil
}

func (f *fakeValidadorJornada) CierreDeLaJornada(_ context.Context, fecha time.Time) (CierreDeJornada, error) {
	if f.cierre == nil {
		return CierreDeJornada{}, nil
	}
	return f.cierre(fecha), nil
}

// fakeValidadorMostrador dice si había alguien operando el sistema (RF-07.6).
//
// El cero value es "en esta escuela nadie declaró horarios": Declarado en
// false, que es lo que hace que Opera() dé true. Así, los tests que no dicen
// nada del mostrador siguen viendo el barrido de siempre, y solo los que
// importan tienen que hablar del tema.
type fakeValidadorMostrador struct {
	// declarado + atendido: los dos juntos son "hay horarios cargados y
	// ninguno cubre este momento", el caso que apaga las tres pasadas.
	declarado bool
	atendido  bool
	// err para probar que un fallo no se traduce en liberar por las dudas.
	err error
	// eseDia permite que el corte de jornada responda distinto del instante,
	// que es justamente para lo que existe MostradorEseDia.
	eseDia *bool
}

func (f *fakeValidadorMostrador) MostradorEn(_ context.Context, _ time.Time) (MostradorAtendido, error) {
	if f.err != nil {
		return MostradorAtendido{}, f.err
	}
	return MostradorAtendido{Atendido: f.atendido, Declarado: f.declarado}, nil
}

func (f *fakeValidadorMostrador) MostradorEseDia(_ context.Context, _ time.Time) (MostradorAtendido, error) {
	if f.err != nil {
		return MostradorAtendido{}, f.err
	}
	atendido := f.atendido
	if f.eseDia != nil {
		atendido = *f.eseDia
	}
	return MostradorAtendido{Atendido: atendido, Declarado: f.declarado}, nil
}

// mostradorAtendido es el validador de los tests que necesitan que el barrido
// opere con horarios efectivamente declarados.
func mostradorAtendido() *fakeValidadorMostrador {
	return &fakeValidadorMostrador{declarado: true, atendido: true}
}

// mostradorSinAtender es el día que el Admin faltó y lo cubrió alguien que no
// usa el sistema.
func mostradorSinAtender() *fakeValidadorMostrador {
	return &fakeValidadorMostrador{declarado: true, atendido: false}
}
