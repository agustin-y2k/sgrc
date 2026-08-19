// Package application orquesta los casos de uso de RF-04 (reservas).
package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"strings"
)

type Service struct {
	repo             Repo
	validadorMateria ValidadorMateria
	validadorEquipo  ValidadorEquipo
	validadorJornada ValidadorJornada
	obtenedorNombre  ObtenedorNombreDocente
	nuevoID          IDGenerator
	ahora            func() time.Time
	bus              eventbus.EventBus
}

func NewService(repo Repo, validadorMateria ValidadorMateria, validadorEquipo ValidadorEquipo, validadorJornada ValidadorJornada, obtenedorNombre ObtenedorNombreDocente, nuevoID IDGenerator, ahora func() time.Time, bus eventbus.EventBus) *Service {
	return &Service{
		repo: repo, validadorMateria: validadorMateria, validadorEquipo: validadorEquipo,
		validadorJornada: validadorJornada,
		obtenedorNombre:  obtenedorNombre, nuevoID: nuevoID, ahora: ahora, bus: bus,
	}
}

// verificarDentroDeLaJornada rechaza lo que cae fuera del horario declarado
// por la institución (RF-04.2 y RF-04.5).
func (s *Service) verificarDentroDeLaJornada(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) error {
	permite, err := s.validadorJornada.PermiteReserva(ctx, fecha, horaInicio, horaFin)
	if err != nil {
		return err
	}
	if !permite {
		return ErrFueraDeJornada
	}
	return nil
}

// cancelacionPendiente es una notificación que quedó lista para publicarse
// pero todavía no se publicó, porque la transacción que la originó no
// commiteó.
type cancelacionPendiente struct {
	reserva *domain.Reserva
	motivo  string
}

// destinatario es la clave de agrupación de los avisos: a quién y por qué.
type destinatario struct {
	usuarioID string
	motivo    string
}

// publicarCancelaciones emite los eventos acumulados durante una transacción
// — se llama SIEMPRE después del commit (RF-05.1/05.2/05.3 son, de punta a
// punta, el mismo evento con distinto motivo según de dónde vino la cascada:
// cancelación manual, bloqueo administrativo, cambio de estado de PC).
func (s *Service) publicarCancelaciones(ctx context.Context, pendientes []cancelacionPendiente) {
	// El orden de los lotes sigue el de la primera cancelación de cada grupo:
	// sobre un map, el orden de publicación cambiaría entre corridas y los tests
	// no podrían afirmar nada.
	orden := make([]destinatario, 0, len(pendientes))
	grupos := make(map[destinatario][]*domain.Reserva, len(pendientes))

	for _, p := range pendientes {
		if p.reserva.CreadoPor == nil {
			continue
		}
		// Nadie necesita que le avisen de algo que acaba de hacer.
		if p.reserva.CanceladoPor != nil && *p.reserva.CanceladoPor == *p.reserva.CreadoPor {
			continue
		}
		k := destinatario{usuarioID: *p.reserva.CreadoPor, motivo: p.motivo}
		if _, visto := grupos[k]; !visto {
			orden = append(orden, k)
		}
		grupos[k] = append(grupos[k], p.reserva)
	}
	if len(orden) == 0 {
		return
	}

	etiquetas := s.etiquetasDeLosEquipos(ctx, grupos)

	for _, k := range orden {
		reservas := grupos[k]
		detalle := make([]eventbus.ReservaCancelada, 0, len(reservas))
		for _, r := range reservas {
			detalle = append(detalle, eventbus.ReservaCancelada{
				ReservaID: r.ID,
				Etiqueta:  etiquetas[r.EquipoID],
				Fecha:     r.Fecha,
			})
		}
		s.bus.Publish(eventbus.Evento{
			Tipo: "reserva.cancelada",
			Payload: eventbus.CancelacionesDeUsuario{
				UsuarioID: k.usuarioID,
				Motivo:    k.motivo,
				Reservas:  detalle,
			},
		})
	}
}

// etiquetasDeLosEquipos resuelve cómo se llama cada equipo ("PC 7",
// "Proyector Epson") a partir de su UUID, para que el aviso diga cuáles
// fueron.
func (s *Service) etiquetasDeLosEquipos(ctx context.Context, grupos map[destinatario][]*domain.Reserva) map[string]string {
	vistas := map[string]bool{}
	var equipoIDs []string
	for _, reservas := range grupos {
		for _, r := range reservas {
			if !vistas[r.EquipoID] {
				vistas[r.EquipoID] = true
				equipoIDs = append(equipoIDs, r.EquipoID)
			}
		}
	}

	etiquetas, err := s.validadorEquipo.EtiquetasDeEquipos(ctx, equipoIDs)
	if err != nil {
		log.Printf("reservation: no se pudieron resolver las etiquetas de los equipos para el aviso: %v", err)
		return map[string]string{}
	}
	return etiquetas
}

// MaxEquiposPorOperacion es el tope de equipos que puede llevar un solo
// pedido —reservar, reservar en serie, bloquear equipos, entregar—.
const MaxEquiposPorOperacion = 200

// verificarCantidadDeEquipos aplica las dos cotas del lote: que haya al menos
// uno y que no sean absurdos.
func verificarCantidadDeEquipos(equipoIDs []string) error {
	if len(equipoIDs) == 0 {
		return ErrSinEquipos
	}
	if len(equipoIDs) > MaxEquiposPorOperacion {
		return ErrDemasiadosEquipos
	}
	return nil
}

// CrearReserva implementa RF-04.1: un docente reserva una o más PCs para
// su materia, en una fecha y horario puntual.
func (s *Service) CrearReserva(ctx context.Context, materiaID, usuarioID string, esAdmin bool, fecha time.Time, horaInicio, horaFin time.Duration, equipoIDs []string) (*domain.ReservaGrupo, []*domain.Reserva, error) {
	if err := verificarCantidadDeEquipos(equipoIDs); err != nil {
		return nil, nil, err
	}

	if err := domain.ValidarVentanaTemporal(fecha, horaInicio, horaFin, s.ahora()); err != nil {
		return nil, nil, err
	}

	if err := s.verificarDentroDeLaJornada(ctx, fecha, horaInicio, horaFin); err != nil {
		return nil, nil, err
	}

	if err := s.verificarPuedeReservar(ctx, materiaID, usuarioID, esAdmin); err != nil {
		return nil, nil, err
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, nil, err
	}

	if err := s.verificarSinSolapamiento(ctx, equipoIDs, []time.Time{fecha}, horaInicio, horaFin); err != nil {
		return nil, nil, err
	}

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, nil, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	// Todo el lote va en una sola transacción: si la PC número 3 de 5 choca
	// contra la constraint EXCLUDE (otro pedido simultáneo se adelantó), el
	// grupo y las dos reservas anteriores se deshacen en vez de quedar
	// commiteadas a medias (RF-04.5).
	var grupo *domain.ReservaGrupo
	var reservas []*domain.Reserva
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		var err error
		grupo, reservas, err = s.materializarGrupo(ctx, repo, materiaID, &usuarioID, nombreDocente, fecha, horaInicio, horaFin, equipoIDs, nil)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return grupo, reservas, nil
}

// verificarPuedeReservar implementa RF-04.1 completo: la materia no puede
// estar archivada, y quien reserva tiene que ser un docente asignado a ella O
// un Admin ("pueden reservar (…) docentes asignados a ella y cualquier
// ADMIN").
func (s *Service) verificarPuedeReservar(ctx context.Context, materiaID, usuarioID string, esAdmin bool) error {
	aceptaReservas, err := s.validadorMateria.MateriaAceptaReservas(ctx, materiaID)
	if err != nil {
		return fmt.Errorf("validando estado de la materia: %w", err)
	}
	if !aceptaReservas {
		return ErrMateriaArchivada
	}

	if esAdmin {
		return nil
	}

	asignado, err := s.validadorMateria.DocenteEstaAsignado(ctx, materiaID, usuarioID)
	if err != nil {
		return fmt.Errorf("validando asignación docente-materia: %w", err)
	}
	if !asignado {
		return ErrDocenteNoAsignado
	}
	return nil
}

// verificarEquiposReservables valida de una sola vez que todo lo pedido se
// pueda reservar, y nombra lo que no.
func (s *Service) verificarEquiposReservables(ctx context.Context, equipoIDs []string) error {
	noReservables, err := s.validadorEquipo.EquiposNoReservables(ctx, equipoIDs)
	if err != nil {
		return fmt.Errorf("validando los equipos pedidos: %w", err)
	}
	if len(noReservables) == 0 {
		return nil
	}

	// Se nombra cuáles fallaron: con "alguno no se puede" el docente tiene que
	// adivinar a cuál destildar.
	etiquetas, errEtiquetas := s.validadorEquipo.EtiquetasDeEquipos(ctx, noReservables)
	if errEtiquetas != nil {
		return ErrEquipoNoDisponible
	}
	nombres := make([]string, 0, len(noReservables))
	for _, id := range noReservables {
		if etiqueta, hay := etiquetas[id]; hay {
			nombres = append(nombres, etiqueta)
		}
	}
	if len(nombres) == 0 {
		return ErrEquipoNoDisponible
	}
	return fmt.Errorf("%w: %s", ErrEquipoNoDisponible, strings.Join(nombres, ", "))
}

// materializarGrupo crea un ReservaGrupo + una Reserva por cada PC — lo
// reusan tanto CrearReserva (una sola fecha) como la recurrencia (una llamada
// por cada fecha generada por ReglaRecurrencia.GenerarFechas).
func (s *Service) materializarGrupo(ctx context.Context, repo Repo, materiaID string, usuarioID *string, nombreDocente string, fecha time.Time, horaInicio, horaFin time.Duration, equipoIDs []string, reglaRecurrenciaID *string) (*domain.ReservaGrupo, []*domain.Reserva, error) {
	ahora := s.ahora()
	grupo, err := domain.NuevoReservaGrupo(s.nuevoID(), materiaID, usuarioID, nombreDocente, fecha, horaInicio, horaFin, reglaRecurrenciaID, ahora)
	if err != nil {
		return nil, nil, err
	}
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		return nil, nil, fmt.Errorf("creando reserva grupo: %w", err)
	}

	reservas := make([]*domain.Reserva, 0, len(equipoIDs))
	for _, equipoID := range equipoIDs {
		r, err := domain.NuevaReservaNormal(s.nuevoID(), grupo.ID, equipoID, materiaID, nombreDocente, usuarioID, fecha, horaInicio, horaFin, ahora)
		if err != nil {
			return nil, nil, err
		}
		if err := repo.CrearReserva(ctx, r); err != nil {
			if errors.Is(err, ErrSolapamiento) {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("creando reserva de PC %s: %w", equipoID, err)
		}
		reservas = append(reservas, r)
	}

	return grupo, reservas, nil
}

// verificarSinSolapamiento es una validación anticipada (mejor mensaje de
// error que esperar la constraint EXCLUDE de la base) — no reemplaza esa
// constraint, que sigue siendo la garantía real ante condiciones de carrera
// entre dos pedidos simultáneos.
func (s *Service) verificarSinSolapamiento(ctx context.Context, equipoIDs []string, fechas []time.Time, horaInicio, horaFin time.Duration) error {
	conflictos, err := s.repo.BuscarSolapamientos(ctx, equipoIDs, fechas, horaInicio, horaFin)
	if err != nil {
		return fmt.Errorf("verificando solapamiento: %w", err)
	}
	if len(conflictos) == 0 {
		return nil
	}
	return &ErrorDeSolapamiento{Conflictos: conflictos}
}

func mismaFecha(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// ObtenerReserva es un passthrough directo al repo — usado por
// interfaces/http para verificar la titularidad de una reserva antes de
// dejarla cancelar (un docente solo puede cancelar las suyas; un Admin puede
// cancelar cualquiera — esa verificación de rol vive en http/, acá solo se
// expone el dato).
func (s *Service) ObtenerReserva(ctx context.Context, id string) (*domain.Reserva, error) {
	return s.repo.BuscarReservaPorID(ctx, id)
}

// CancelarReserva implementa RF-04.4: cancelación manual de una PC puntual
// dentro de un grupo (motivo obligatorio).
func (s *Service) CancelarReserva(ctx context.Context, reservaID string, canceladoPor *string, motivo string) error {
	var pendientes []cancelacionPendiente

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		r, err := repo.BuscarReservaPorID(ctx, reservaID)
		if err != nil {
			return err
		}

		if err := r.Cancelar(canceladoPor, motivo, s.ahora()); err != nil {
			return err
		}
		if err := repo.GuardarReserva(ctx, r); err != nil {
			return err
		}
		pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: motivo})

		if r.ReservaGrupoID != nil {
			// La reserva y el estado recalculado de su grupo padre van juntos: un
			// grupo que quedó PARCIALMENTE_CANCELADA sin que la reserva llegara a
			// cancelarse (o al revés) sería un estado incoherente visible desde la
			// API.
			if err := s.actualizarEstadoGrupo(ctx, repo, *r.ReservaGrupoID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return nil
}

// actualizarEstadoGrupo recalcula CONFIRMADA/PARCIALMENTE_CANCELADA/
// CANCELADA de un ReservaGrupo según el estado de sus Reserva hijas.
func (s *Service) actualizarEstadoGrupo(ctx context.Context, repo Repo, grupoID string) error {
	grupo, err := repo.BuscarReservaGrupoPorID(ctx, grupoID)
	if err != nil {
		return err
	}
	if grupo.Estado == domain.GrupoFinalizada || grupo.Estado == domain.GrupoCancelada {
		return nil // terminal, no hay nada que recalcular
	}

	reservas, err := repo.ListarReservasPorGrupo(ctx, grupoID)
	if err != nil {
		return err
	}

	todasCanceladas := true
	algunaCancelada := false
	for _, r := range reservas {
		if r.Estado == domain.ReservaCancelada {
			algunaCancelada = true
		} else {
			todasCanceladas = false
		}
	}

	var nuevoEstado domain.EstadoReservaGrupo
	switch {
	case todasCanceladas:
		nuevoEstado = domain.GrupoCancelada
	case algunaCancelada:
		nuevoEstado = domain.GrupoParcialmenteCancelada
	default:
		return nil
	}

	if grupo.Estado == nuevoEstado {
		return nil
	}
	if err := grupo.CambiarEstado(nuevoEstado); err != nil {
		return err
	}
	// repo, NO s.repo: esta función corre SIEMPRE dentro de la transacción que
	// abrió quien la llama.
	return repo.GuardarReservaGrupo(ctx, grupo)
}

// ListarReservas devuelve una página de las reservas que matcheen el filtro,
// junto con el total de filas que matchean.
func (s *Service) ListarReservas(ctx context.Context, f FiltroReservas) ([]ReservaDetallada, int, error) {
	if f.Pagina.Tamanio <= 0 || f.Pagina.Numero <= 0 {
		f.Pagina = paginacion.PorDefecto()
	}
	return s.repo.ListarReservas(ctx, f)
}

// CalendarioDeEquipo implementa RF-04.4: cualquier usuario autenticado puede
// ver los bloques ocupados de una PC, con docente, materia y horario.
func (s *Service) CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) ([]BloqueCalendario, error) {
	if hasta.Before(desde) {
		return nil, domain.ErrRangoFechasInvalido
	}
	return s.repo.CalendarioDeEquipo(ctx, equipoID, desde, hasta)
}

// ListarEquiposDisponiblesEn implementa la selección de PCs de RF-04.2,
// ordenada para la materia que se está reservando (RF-03.21).
func (s *Service) ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, materiaID string) ([]EquipoDisponible, error) {
	if horaFin == horaInicio {
		return nil, domain.ErrRangoHorarioInvalido
	}
	return s.repo.ListarEquiposDisponiblesEn(ctx, fecha, horaInicio, horaFin, materiaID)
}

// ListarEquiposOcupadosEn devuelve la otra mitad de la franja (RF-04.11): lo
// que ya tiene alguien, con quién lo tiene, para que "no hay nada libre" y
// "los tiene alguien con quien puedo hablar" dejen de verse igual.
func (s *Service) ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, quien string) ([]EquipoOcupado, error) {
	if horaFin == horaInicio {
		return nil, domain.ErrRangoHorarioInvalido
	}
	ocupados, err := s.repo.ListarEquiposOcupadosEn(ctx, fecha, horaInicio, horaFin)
	if err != nil {
		return nil, err
	}

	ahora := s.ahora()
	for i := range ocupados {
		oc := &ocupados[i]
		oc.PuedePedirse = !oc.EsBloqueo &&
			oc.DocenteID != nil && *oc.DocenteID != quien &&
			!domain.YaEmpezo(fecha, oc.HoraInicio, ahora)
	}
	return ocupados, nil
}

// ListarEquiposLibresEnLaSerie: los equipos libres en todas las fechas que le
// quedan a la serie, para cambiar la máquina de una recurrencia entera
// (RF-08.14).
func (s *Service) ListarEquiposLibresEnLaSerie(ctx context.Context, grupoID string) ([]EquipoDisponible, error) {
	return s.repo.ListarEquiposLibresEnLaSerie(ctx, grupoID)
}

// PedirLiberacionDeReserva le avisa al dueño de una reserva que otro docente
// necesita ese equipo (RF-04.12).
func (s *Service) PedirLiberacionDeReserva(ctx context.Context, reservaID, solicitanteID, mensaje string) error {
	pedido, err := s.repo.DatosParaPedirLiberacion(ctx, reservaID)
	if err != nil {
		return err
	}

	if pedido.Estado != domain.ReservaConfirmada {
		return ErrReservaNoModificable
	}
	if pedido.EsBloqueo || pedido.DuenoID == nil {
		return ErrReservaSinDueño
	}
	if *pedido.DuenoID == solicitanteID {
		return ErrReservaPropia
	}
	if domain.YaEmpezo(pedido.Fecha, pedido.HoraInicio, s.ahora()) {
		return ErrReservaYaEmpezada
	}

	yaPidio, err := s.repo.YaPidioLiberacionHoy(ctx, reservaID, solicitanteID, s.ahora())
	if err != nil {
		return err
	}
	if yaPidio {
		return ErrPedidoRepetido
	}

	nombreSolicitante, err := s.obtenedorNombre.NombreCompletoDe(ctx, solicitanteID)
	if err != nil {
		// El nombre de quien pide es el dato central del aviso: sin él, al
		// dueño le llega un pedido anónimo y no sabe con quién hablar.
		return fmt.Errorf("resolviendo el nombre de quien pide: %w", err)
	}

	s.bus.Publish(eventbus.Evento{Tipo: "reserva.pedido-de-liberacion", Payload: eventbus.PedidoDeLiberacion{
		UsuarioID:         *pedido.DuenoID,
		Email:             pedido.DuenoEmail,
		Nombre:            pedido.DuenoNombre,
		SolicitanteID:     solicitanteID,
		SolicitanteNombre: nombreSolicitante,
		ReservaID:         reservaID,
		Etiqueta:          pedido.Etiqueta,
		MateriaNombre:     pedido.MateriaNombre,
		Fecha:             pedido.Fecha,
		HoraInicio:        pedido.HoraInicio,
		HoraFin:           pedido.HoraFin,
		Mensaje:           mensaje,
	}})
	return nil
}

// ObtenerReservaGrupo es un passthrough directo al repo — mismo criterio que
// ObtenerReserva, para verificar titularidad antes de
// CancelarOcurrenciaRecurrente.
func (s *Service) ObtenerReservaGrupo(ctx context.Context, id string) (*domain.ReservaGrupo, error) {
	return s.repo.BuscarReservaGrupoPorID(ctx, id)
}

// ── Recurrencia (RF-04.2) ───────────────────────────────────────────────

// ResultadoRecurrencia agrupa lo que devuelve CrearReservaRecurrente: la
// regla en sí, y un ReservaGrupo (con sus Reserva) por cada fecha generada.
type ResultadoRecurrencia struct {
	Regla  *domain.ReglaRecurrencia
	Grupos []*domain.ReservaGrupo
}

// maxOcurrenciasRecurrencia acota cuántas fechas puede materializar una sola
// serie recurrente.
const maxOcurrenciasRecurrencia = 45

// CrearReservaRecurrente implementa RF-04.2: crea la ReglaRecurrencia y
// materializa un ReservaGrupo (con sus Reserva) por cada fecha que genera
// domain.ReglaRecurrencia.GenerarFechas().
func (s *Service) CrearReservaRecurrente(ctx context.Context, materiaID, usuarioID string, esAdmin bool, diaSemana domain.DiaSemana, horaInicio, horaFin time.Duration, fechaInicio, fechaFin time.Time, equipoIDs []string) (*ResultadoRecurrencia, error) {
	if err := verificarCantidadDeEquipos(equipoIDs); err != nil {
		return nil, err
	}

	if err := s.verificarPuedeReservar(ctx, materiaID, usuarioID, esAdmin); err != nil {
		return nil, err
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, err
	}

	regla, err := domain.NuevaReglaRecurrencia(s.nuevoID(), materiaID, usuarioID, diaSemana, horaInicio, horaFin, fechaInicio, fechaFin)
	if err != nil {
		return nil, err
	}

	fechas := regla.GenerarFechas()

	// La jornada se chequea una sola vez y no por ocurrencia: todas caen en el
	// mismo día de la semana y en el mismo horario, así que o entran todas o no
	// entra ninguna.
	if len(fechas) > 0 {
		if err := s.verificarDentroDeLaJornada(ctx, fechas[0], horaInicio, horaFin); err != nil {
			return nil, err
		}
	}

	// El tope se aplica ANTES del pre-chequeo de solapamiento: ese chequeo hace
	// una consulta por PC y por fecha, así que dejarlo correr sobre un rango sin
	// acotar ya era el grueso del problema, aunque después no se insertara nada.
	if len(fechas) == 0 {
		return nil, ErrSinOcurrencias
	}
	if len(fechas) > maxOcurrenciasRecurrencia {
		return nil, fmt.Errorf("%w: son %d clases y el máximo es %d",
			ErrDemasiadasOcurrencias, len(fechas), maxOcurrenciasRecurrencia)
	}

	// La ventana temporal se valida sobre la PRIMERA ocurrencia: GenerarFechas
	// las devuelve en orden ascendente, así que si esa no quedó en el pasado
	// ninguna de las siguientes tampoco, y el horario es el mismo para toda la
	// serie.
	if err := domain.ValidarVentanaTemporal(fechas[0], horaInicio, horaFin, s.ahora()); err != nil {
		if errors.Is(err, domain.ErrReservaEnElPasado) {
			return nil, fmt.Errorf("%w: la serie arranca el %s", err, fechas[0].Format("2006-01-02"))
		}
		return nil, err
	}

	// Se verifica el solapamiento de TODAS las fechas antes de crear nada — así
	// una falla en la fecha 5 de 10 no deja 4 grupos ya creados.
	if err := s.verificarSinSolapamiento(ctx, equipoIDs, fechas, horaInicio, horaFin); err != nil {
		return nil, err
	}

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	// La regla, sus PCs y las N ocurrencias van en una sola transacción.
	grupos := make([]*domain.ReservaGrupo, 0, len(fechas))
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		grupos = grupos[:0]

		if err := repo.CrearReglaRecurrencia(ctx, regla); err != nil {
			return fmt.Errorf("creando regla de recurrencia: %w", err)
		}

		for _, fecha := range fechas {
			grupo, _, err := s.materializarGrupo(ctx, repo, materiaID, &usuarioID, nombreDocente, fecha, horaInicio, horaFin, equipoIDs, &regla.ID)
			if err != nil {
				return fmt.Errorf("materializando ocurrencia del %s: %w", fecha.Format("2006-01-02"), err)
			}
			grupos = append(grupos, grupo)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ResultadoRecurrencia{Regla: regla, Grupos: grupos}, nil
}

// CancelarOcurrenciaRecurrente implementa RF-04.6: cancela un ReservaGrupo
// puntual de una serie recurrente.
func (s *Service) CancelarOcurrenciaRecurrente(ctx context.Context, reservaGrupoID string, canceladoPor *string, motivo string, soloEsta bool) (int, error) {
	totalCancelado := 0
	var pendientes []cancelacionPendiente

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		totalCancelado = 0
		pendientes = nil

		grupo, err := repo.BuscarReservaGrupoPorID(ctx, reservaGrupoID)
		if err != nil {
			return err
		}

		gruposACancelar := []*domain.ReservaGrupo{grupo}

		if !soloEsta && grupo.ReglaRecurrenciaID != nil {
			futuros, err := repo.ListarGruposFuturosDeRegla(ctx, *grupo.ReglaRecurrenciaID, grupo.Fecha)
			if err != nil {
				return fmt.Errorf("listando ocurrencias futuras de la regla: %w", err)
			}
			gruposACancelar = append(gruposACancelar, futuros...)
		}

		for _, g := range gruposACancelar {
			reservas, err := repo.ListarReservasPorGrupo(ctx, g.ID)
			if err != nil {
				return fmt.Errorf("listando reservas del grupo %s: %w", g.ID, err)
			}
			for _, r := range reservas {
				if r.Estado != domain.ReservaConfirmada {
					continue // ya cancelada o finalizada, no hay nada que hacer
				}
				if err := r.Cancelar(canceladoPor, motivo, s.ahora()); err != nil {
					return err
				}
				if err := repo.GuardarReserva(ctx, r); err != nil {
					return err
				}
				pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: motivo})
				totalCancelado++
			}
			if err := s.actualizarEstadoGrupo(ctx, repo, g.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return totalCancelado, nil
}

// ── Bloqueo administrativo de equipos (RF-04.7) ────────────────────────

// ResultadoBloqueo agrupa lo que devuelve BloquearEquipos.
type ResultadoBloqueo struct {
	Bloqueos            []*domain.Reserva
	ReservasCanceladas  int
	DocentesNotificados int
}

// BloquearEquipos implementa RF-04.7: un Admin toma una o más máquinas en un
// rango horario definido, por el motivo que sea —una evaluación, una jornada
// docente, una obra en el aula—.
func (s *Service) BloquearEquipos(ctx context.Context, equipoIDs []string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, motivoBloqueo string) (*ResultadoBloqueo, error) {
	if err := verificarCantidadDeEquipos(equipoIDs); err != nil {
		return nil, err
	}

	ahora := s.ahora()

	// Un bloqueo sobre un horario que ya terminó no bloquea nada: no cancela
	// reservas (la cascada de abajo solo mira las que siguen vivas) y deja una
	// fila que solo sirve para ensuciar los reportes.
	if domain.YaTermino(fecha, horaInicio, horaFin, ahora) {
		return nil, domain.ErrReservaEnElPasado
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, err
	}

	docentesAfectados := map[string]bool{}
	reservasCanceladas := 0
	bloqueos := make([]*domain.Reserva, 0, len(equipoIDs))
	var pendientes []cancelacionPendiente

	// Todo el bloqueo es una sola transacción.
	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		docentesAfectados = map[string]bool{}
		reservasCanceladas = 0
		bloqueos = bloqueos[:0]
		pendientes = nil

		for _, equipoID := range equipoIDs {
			futuras, err := repo.ListarReservasFuturasDeEquipo(ctx, equipoID, fecha)
			if err != nil {
				return fmt.Errorf("listando reservas del equipo %s: %w", equipoID, err)
			}

			for _, r := range futuras {
				if r.Estado != domain.ReservaConfirmada || r.Tipo != domain.TipoNormal {
					continue
				}
				if !mismaFecha(r.Fecha, fecha) || !r.SolapaCon(horaInicio, horaFin) {
					continue
				}

				// Solo la razón: la frase "Tu reserva fue cancelada" la pone el
				// suscriptor de notification.
				motivoCascada := fmt.Sprintf("los equipos quedaron bloqueados: %s", motivoBloqueo)
				if err := r.Cancelar(creadoPor, motivoCascada, ahora); err != nil {
					return err
				}
				if err := repo.GuardarReserva(ctx, r); err != nil {
					return err
				}
				pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: motivoCascada})
				reservasCanceladas++
				if r.CreadoPor != nil {
					docentesAfectados[*r.CreadoPor] = true
				}
				if r.ReservaGrupoID != nil {
					if err := s.actualizarEstadoGrupo(ctx, repo, *r.ReservaGrupoID); err != nil {
						return err
					}
				}
			}

			bloqueo, err := domain.NuevaReservaBloqueo(s.nuevoID(), equipoID, creadoPor, fecha, horaInicio, horaFin, motivoBloqueo, ahora)
			if err != nil {
				return err
			}
			if err := repo.CrearReserva(ctx, bloqueo); err != nil {
				if errors.Is(err, ErrSolapamiento) {
					return err
				}
				return fmt.Errorf("creando el bloqueo del equipo %s: %w", equipoID, err)
			}
			bloqueos = append(bloqueos, bloqueo)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.publicarCancelaciones(ctx, pendientes)

	return &ResultadoBloqueo{
		Bloqueos:            bloqueos,
		ReservasCanceladas:  reservasCanceladas,
		DocentesNotificados: len(docentesAfectados),
	}, nil
}

// ── Job de vencimiento (RF-04.9) ────────────────────────────────────────

const (
	// loteFinalizarVencidas es cuántas reservas procesa cada transacción del
	// job.
	loteFinalizarVencidas = 500

	// maxLotesPorCiclo acota cuánto puede durar UNA corrida del job.
	maxLotesPorCiclo = 20
)

// FinalizarVencidas implementa RF-04.9 — se llama periódicamente desde un
// goroutine + time.Ticker en cmd/main.go.
func (s *Service) FinalizarVencidas(ctx context.Context) (int, error) {
	total := 0

	for lote := 0; lote < maxLotesPorCiclo; lote++ {
		leidas, finalizadas, err := s.finalizarLoteVencidas(ctx)
		total += finalizadas
		if err != nil {
			// Lo finalizado hasta acá ya está commiteado: se devuelve junto con el
			// error para que el log del job no diga "0" cuando en realidad avanzó.
			return total, err
		}
		if leidas < loteFinalizarVencidas {
			break // no queda nada más vencido
		}
		if finalizadas == 0 {
			// Se leyó un lote completo y no avanzó ninguna: son reservas que el repo
			// devuelve como vencidas pero que no pueden transicionar.
			break
		}
	}

	return total, nil
}

// finalizarLoteVencidas procesa un lote en una sola transacción y devuelve
// cuántas filas leyó y cuántas efectivamente finalizó.
func (s *Service) finalizarLoteVencidas(ctx context.Context) (leidas int, finalizadas int, err error) {
	ahora := s.ahora()

	// En una transacción: si algo falla a mitad, el ticker reintenta en el
	// próximo ciclo desde un estado consistente, en vez de dejar reservas
	// finalizadas cuyos grupos padre nunca se recalcularon.
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		leidas, finalizadas = 0, 0

		vencidas, err := repo.ListarReservasConfirmadasVencidas(ctx, ahora, loteFinalizarVencidas)
		if err != nil {
			return fmt.Errorf("listando reservas vencidas: %w", err)
		}
		leidas = len(vencidas)

		gruposAfectados := map[string]bool{}

		for _, r := range vencidas {
			if err := r.Finalizar(); err != nil {
				// No debería pasar (ya vino filtrada como CONFIRMADA desde el
				// repo), pero una reserva rara no debe abortar el job entero.
				continue
			}
			if err := repo.GuardarReserva(ctx, r); err != nil {
				return fmt.Errorf("guardando reserva finalizada %s: %w", r.ID, err)
			}
			finalizadas++
			if r.ReservaGrupoID != nil {
				gruposAfectados[*r.ReservaGrupoID] = true
			}
		}

		for grupoID := range gruposAfectados {
			if err := s.finalizarGrupoSiCorresponde(ctx, repo, grupoID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return leidas, finalizadas, nil
}

// finalizarGrupoSiCorresponde marca un ReservaGrupo como FINALIZADA cuando
// ninguna de sus Reserva sigue CONFIRMADA (ya sea porque todas finalizaron, o
// una mezcla de finalizadas+canceladas).
func (s *Service) finalizarGrupoSiCorresponde(ctx context.Context, repo Repo, grupoID string) error {
	grupo, err := repo.BuscarReservaGrupoPorID(ctx, grupoID)
	if err != nil {
		return err
	}
	if grupo.Estado == domain.GrupoCancelada || grupo.Estado == domain.GrupoFinalizada {
		return nil
	}

	reservas, err := repo.ListarReservasPorGrupo(ctx, grupoID)
	if err != nil {
		return err
	}

	for _, r := range reservas {
		if r.Estado == domain.ReservaConfirmada {
			return nil // todavía queda alguna viva, no finalizar el grupo
		}
	}

	if err := grupo.CambiarEstado(domain.GrupoFinalizada); err != nil {
		return err
	}
	return repo.GuardarReservaGrupo(ctx, grupo)
}

// ── Cascadas hacia otros paquetes (inventory, auth) ────────────────────
// Estos dos métodos son lo que reservation expone para que inventory y auth
// puedan disparar una cascada de cancelación sin reimplementar la máquina de
// estados de Reserva/ReservaGrupo con SQL crudo — a diferencia de los
// validadores de solo-lectura (que sí van directo por SQL en cada paquete),
// cancelar es una ACCIÓN con reglas de negocio reales (una reserva ya
// cancelada no se puede volver a cancelar, el ReservaGrupo padre tiene que
// recalcular su estado, etc.) que solo debe existir en un lugar.

// CancelarReservasFuturasDeEquipo implementa el lado "reservation" de la
// cascada que dispara inventory (RF-03.8/03.9: cambio de estado o baja de una
// PC).
func (s *Service) CancelarReservasFuturasDeEquipo(ctx context.Context, equipoID, motivo string) (int, int, error) {
	ahora := s.ahora()
	var canceladas, docentes int
	var pendientes []cancelacionPendiente

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		futuras, err := repo.ListarReservasFuturasDeEquipo(ctx, equipoID, ahora)
		if err != nil {
			return fmt.Errorf("listando reservas futuras del equipo: %w", err)
		}
		canceladas, docentes, pendientes, err = s.cancelarEnCascada(ctx, repo, futuras, motivo, ahora)
		return err
	})
	if err != nil {
		return 0, 0, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return canceladas, docentes, nil
}

// TieneReservasFuturasDeEquipo responde si a esa PC todavía le quedan
// reservas CONFIRMADA sin terminar.
func (s *Service) TieneReservasFuturasDeEquipo(ctx context.Context, equipoID string) (bool, error) {
	futuras, err := s.repo.ListarReservasFuturasDeEquipo(ctx, equipoID, s.ahora())
	if err != nil {
		return false, fmt.Errorf("verificando reservas futuras del equipo: %w", err)
	}
	for _, r := range futuras {
		if r.Estado == domain.ReservaConfirmada {
			return true, nil
		}
	}
	return false, nil
}

// CancelarReservasFuturasDeMateria implementa el lado "reservation" de la
// cascada que dispara auth (RF-02.8: dar de baja al único docente asignado a
// una materia).
func (s *Service) CancelarReservasFuturasDeMateria(ctx context.Context, materiaID, motivo string) (int, error) {
	ahora := s.ahora()
	var canceladas int
	var pendientes []cancelacionPendiente

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		futuras, err := repo.ListarReservasFuturasDeMateria(ctx, materiaID, ahora)
		if err != nil {
			return fmt.Errorf("listando reservas futuras de la materia: %w", err)
		}
		canceladas, _, pendientes, err = s.cancelarEnCascada(ctx, repo, futuras, motivo, ahora)
		return err
	})
	if err != nil {
		return 0, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return canceladas, nil
}

// cancelarEnCascada centraliza lo que ya hacían por separado BloquearEquipos
// y estos dos métodos: cancelar cada reserva CONFIRMADA de la lista (sin
// "cancelado por" — son cascadas disparadas por el sistema, no por un click
// puntual de un usuario sobre esa reserva en particular) y recalcular el
// ReservaGrupo padre de cada una.
func (s *Service) cancelarEnCascada(ctx context.Context, repo Repo, reservas []*domain.Reserva, motivo string, ahora time.Time) (int, int, []cancelacionPendiente, error) {
	docentesAfectados := map[string]bool{}
	canceladas := 0
	var pendientes []cancelacionPendiente

	for _, r := range reservas {
		if r.Estado != domain.ReservaConfirmada {
			continue
		}
		if err := r.Cancelar(nil, motivo, ahora); err != nil {
			return 0, 0, nil, err
		}
		if err := repo.GuardarReserva(ctx, r); err != nil {
			return 0, 0, nil, err
		}
		pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: motivo})
		canceladas++
		if r.CreadoPor != nil {
			docentesAfectados[*r.CreadoPor] = true
		}
		if r.ReservaGrupoID != nil {
			if err := s.actualizarEstadoGrupo(ctx, repo, *r.ReservaGrupoID); err != nil {
				return 0, 0, nil, err
			}
		}
	}

	return canceladas, len(docentesAfectados), pendientes, nil
}

// EliminarReservasDeCiclo implementa el lado "reservation" de la cascada de
// archivado de academic (RF-02.4) — borra físicamente todas las
// Reserva/ReservaGrupo de ese ciclo lectivo, sin importar su estado (a
// diferencia de Cancelar*, que solo cambia el estado, esto elimina las filas
// de verdad).
func (s *Service) EliminarReservasDeCiclo(ctx context.Context, cicloID string) (gruposEliminados int, reservasEliminadas int, err error) {
	return s.repo.EliminarReservasYGruposDeCiclo(ctx, cicloID)
}

// ErrReservaNoModificable: solo una reserva CONFIRMADA se puede cambiar de
// máquina.
var ErrReservaNoModificable = errors.New("esa reserva ya no está vigente")

// CambiarEquipoDeReserva mueve una reserva a otra máquina sin partir la clase
// en dos (RF-08.14).
func (s *Service) CambiarEquipoDeReserva(ctx context.Context, reservaID, pcNuevoID string, quien string, esAdmin bool, soloEsta bool) (*domain.Reserva, error) {
	var cambiada *domain.Reserva

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		r, err := repo.BuscarReservaPorID(ctx, reservaID)
		if err != nil {
			return err
		}
		if r.Estado != domain.ReservaConfirmada {
			return ErrReservaNoModificable
		}
		// Misma regla de titularidad que cancelar: es tuya, o sos Admin.
		if !esAdmin && (r.CreadoPor == nil || *r.CreadoPor != quien) {
			return ErrReservaAjena
		}
		if r.EquipoID == pcNuevoID {
			cambiada = r
			return nil // no hay nada que cambiar
		}

		disponible, err := s.validadorEquipo.EquipoDisponibleParaReservar(ctx, pcNuevoID)
		if err != nil {
			return fmt.Errorf("verificando el equipo nuevo: %w", err)
		}
		if !disponible {
			return ErrEquipoNoDisponible
		}

		aCambiar, err := s.reservasDelAlcance(ctx, repo, r, soloEsta)
		if err != nil {
			return err
		}

		// Todas las fechas contra el equipo nuevo, en una sola consulta y ANTES de
		// tocar ninguna: cambiar catorce martes y fallar en el decimoquinto dejaría
		// la serie repartida entre dos máquinas sin que nadie lo haya pedido.
		fechas := make([]time.Time, 0, len(aCambiar))
		for _, res := range aCambiar {
			fechas = append(fechas, res.Fecha)
		}
		if err := s.verificarSinSolapamiento(ctx, []string{pcNuevoID}, fechas, r.HoraInicio, r.HoraFin); err != nil {
			return err
		}

		for _, res := range aCambiar {
			res.EquipoID = pcNuevoID
			if err := repo.GuardarReserva(ctx, res); err != nil {
				return err
			}
			if res.ID == reservaID {
				cambiada = res
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cambiada, nil
}

// reservasDelAlcance traduce "solo esta" / "esta y las siguientes" a la lista
// concreta de filas a mover.
func (s *Service) reservasDelAlcance(ctx context.Context, repo Repo, r *domain.Reserva, soloEsta bool) ([]*domain.Reserva, error) {
	if soloEsta {
		return []*domain.Reserva{r}, nil
	}

	serie, err := repo.ReservasDeLaSerieDesde(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	if len(serie) == 0 {
		return []*domain.Reserva{r}, nil
	}
	return serie, nil
}
