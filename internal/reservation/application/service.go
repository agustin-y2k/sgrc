// Package application orquesta los casos de uso de RF-04 (reservas). Esta
// primera parte cubre la reserva simple (RF-04.1) y la cancelación
// individual (RF-04.4) — recurrencia, bloqueo por evaluación y el job de
// vencimiento se agregan en pasos siguientes dentro de la misma capa.
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
	obtenedorNombre  ObtenedorNombreDocente
	nuevoID          IDGenerator
	ahora            func() time.Time
	bus              eventbus.EventBus
}

func NewService(repo Repo, validadorMateria ValidadorMateria, validadorEquipo ValidadorEquipo, obtenedorNombre ObtenedorNombreDocente, nuevoID IDGenerator, ahora func() time.Time, bus eventbus.EventBus) *Service {
	return &Service{
		repo: repo, validadorMateria: validadorMateria, validadorEquipo: validadorEquipo,
		obtenedorNombre: obtenedorNombre, nuevoID: nuevoID, ahora: ahora, bus: bus,
	}
}

// cancelacionPendiente es una notificación que quedó lista para publicarse
// pero todavía no se publicó, porque la transacción que la originó no
// commiteó. Sin esta demora, un rollback dejaría al docente con un aviso
// de "tu reserva fue cancelada" sobre una cancelación que nunca ocurrió.
type cancelacionPendiente struct {
	reserva *domain.Reserva
	motivo  string
}

// destinatario es la clave de agrupación de los avisos: a quién y por qué.
// Dos cancelaciones con motivos distintos no se juntan en un solo mensaje —
// habría que enumerar las razones adentro y el aviso dejaría de leerse de
// un vistazo.
type destinatario struct {
	usuarioID string
	motivo    string
}

// publicarCancelaciones emite los eventos acumulados durante una
// transacción — se llama SIEMPRE después del commit (RF-05.1/05.2/05.3 son,
// de punta a punta, el mismo evento con distinto motivo según de dónde vino
// la cascada: cancelación manual, bloqueo por evaluación, cambio de estado
// de PC). Una reserva sin docente asociado (un bloqueo de evaluación
// cancelado, caso raro) no tiene a quién notificar y se saltea.
//
// Emite UN evento por docente y motivo, no uno por Reserva. Bloquear tres
// PCs de una misma clase para una evaluación le dejaba al docente tres
// avisos idénticos en la campana: para él es una sola cosa —"me sacaron la
// clase"— y lo que necesita saber es qué equipos, no cuántas filas se tocaron.
// El armado de la frase vive en notification; acá solo se junta el dato.
func (s *Service) publicarCancelaciones(ctx context.Context, pendientes []cancelacionPendiente) {
	// El orden de los lotes sigue el de la primera cancelación de cada
	// grupo: sobre un map, el orden de publicación cambiaría entre
	// corridas y los tests no podrían afirmar nada.
	orden := make([]destinatario, 0, len(pendientes))
	grupos := make(map[destinatario][]*domain.Reserva, len(pendientes))

	for _, p := range pendientes {
		if p.reserva.CreadoPor == nil {
			continue
		}
		// Nadie necesita que le avisen de algo que acaba de hacer. RF-05.1
		// habla de la reserva propia cancelada POR UN ADMIN; cuando el
		// docente cancela la suya, el motivo además es opcional (RF-04.8),
		// así que el aviso salía como "Tu reserva fue cancelada: " con el
		// dos puntos colgando. Las cascadas no entran acá: van sin
		// canceladoPor, porque las dispara el sistema.
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
//
// Devuelve etiquetas y no números desde la 015: lo que se reserva puede no
// tener número, y "PC 0" es lo que sale de formatear uno que no existe.
//
// Si la consulta falla NO se aborta nada: el evento sale igual sin el
// detalle. Esto corre después del commit, sobre una cancelación que ya
// ocurrió — quedarse sin avisar por no poder adornar el mensaje sería el
// peor de los dos errores.
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
// pedido —reservar, reservar en serie, bloquear para evaluación, entregar—.
//
// Doscientos no sale de ninguna regla de la escuela: sale de que el número
// tiene que estar MUY por encima de cualquier uso real y aun así ser finito.
// El inventario acá son sesenta y pico de equipos y el pedido más grande que
// existe es bloquear un carro entero, así que nadie lo va a rozar; lo que
// frena es un cliente pidiendo diez mil, que sin tope se traduce en diez mil
// filas en una transacción.
const MaxEquiposPorOperacion = 200

// verificarCantidadDeEquipos aplica las dos cotas del lote: que haya al
// menos uno y que no sean absurdos. Va primero en cada caso de uso, antes de
// cualquier consulta: validar el tamaño después de haber ido a la base por
// cada elemento sería llegar tarde.
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

	if !domain.EsDiaLectivo(fecha) {
		return nil, nil, domain.ErrDiaNoLectivo
	}

	if err := domain.ValidarVentanaTemporal(fecha, horaInicio, horaFin, s.ahora()); err != nil {
		return nil, nil, err
	}

	if err := s.verificarPuedeReservar(ctx, materiaID, usuarioID, esAdmin); err != nil {
		return nil, nil, err
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, nil, err
	}

	if err := s.verificarSinSolapamiento(ctx, equipoIDs, fecha, horaInicio, horaFin); err != nil {
		return nil, nil, err
	}

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, nil, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	// Todo el lote va en una sola transacción: si la PC número 3 de 5
	// choca contra la constraint EXCLUDE (otro pedido simultáneo se
	// adelantó), el grupo y las dos reservas anteriores se deshacen en vez
	// de quedar commiteadas a medias (RF-04.5).
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
// estar archivada, y quien reserva tiene que ser un docente asignado a ella
// O un Admin ("pueden reservar (…) docentes asignados a ella y cualquier
// ADMIN"). Las dos condiciones son independientes: un Admin tampoco puede
// reservar sobre una materia archivada.
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
//
// Antes era un bucle con una consulta por equipo. Con ocho máquinas daba
// igual; con un bloqueo por evaluación sobre un carro entero eran 64 idas a
// la base antes de escribir una sola fila, y eso lo dispara el uso normal.
func (s *Service) verificarEquiposReservables(ctx context.Context, equipoIDs []string) error {
	noReservables, err := s.validadorEquipo.EquiposNoReservables(ctx, equipoIDs)
	if err != nil {
		return fmt.Errorf("validando los equipos pedidos: %w", err)
	}
	if len(noReservables) == 0 {
		return nil
	}

	// Se nombra cuáles fallaron: con "alguno no se puede" el docente tiene
	// que adivinar a cuál destildar. Si no se pueden resolver las etiquetas,
	// sale el error genérico — quedarse sin aviso sería peor.
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
// reusan tanto CrearReserva (una sola fecha) como la recurrencia (una
// llamada por cada fecha generada por ReglaRecurrencia.GenerarFechas).
// Recibe el repo por parámetro (en vez de usar s.repo) para poder correr
// dentro de la transacción que abre quien la llama.
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
// constraint, que sigue siendo la garantía real ante condiciones de
// carrera entre dos pedidos simultáneos.
func (s *Service) verificarSinSolapamiento(ctx context.Context, equipoIDs []string, fecha time.Time, horaInicio, horaFin time.Duration) error {
	for _, equipoID := range equipoIDs {
		futuras, err := s.repo.ListarReservasFuturasDeEquipo(ctx, equipoID, fecha)
		if err != nil {
			return fmt.Errorf("verificando solapamiento: %w", err)
		}
		for _, f := range futuras {
			if f.Estado != domain.ReservaConfirmada {
				continue
			}
			if !mismaFecha(f.Fecha, fecha) {
				continue
			}
			if f.SolapaCon(horaInicio, horaFin) {
				return ErrSolapamiento
			}
		}
	}
	return nil
}

func mismaFecha(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// ObtenerReserva es un passthrough directo al repo — usado por
// interfaces/http para verificar la titularidad de una reserva antes de
// dejarla cancelar (un docente solo puede cancelar las suyas; un Admin
// puede cancelar cualquiera — esa verificación de rol vive en http/, acá
// solo se expone el dato).
func (s *Service) ObtenerReserva(ctx context.Context, id string) (*domain.Reserva, error) {
	return s.repo.BuscarReservaPorID(ctx, id)
}

// CancelarReserva implementa RF-04.4: cancelación manual de una PC
// puntual dentro de un grupo (motivo obligatorio). Actualiza el estado
// del ReservaGrupo padre según cuántas de sus reservas sigan confirmadas.
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
			// La reserva y el estado recalculado de su grupo padre van
			// juntos: un grupo que quedó PARCIALMENTE_CANCELADA sin que la
			// reserva llegara a cancelarse (o al revés) sería un estado
			// incoherente visible desde la API.
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
// FINALIZADA no se toca acá — esa transición es responsabilidad exclusiva
// del job de vencimiento (RF-04.9).
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
	// repo, NO s.repo: esta función corre SIEMPRE dentro de la transacción
	// que abrió quien la llama. Escribir por s.repo usaba otra conexión del
	// pool, así que el estado del grupo se commiteaba solo — un rollback
	// posterior (ej. el INSERT del bloqueo de evaluación choca contra la
	// constraint EXCLUDE en la PC 3 de 5) devolvía las Reserva a CONFIRMADA
	// y dejaba sus ReservaGrupo marcados CANCELADA para siempre.
	return repo.GuardarReservaGrupo(ctx, grupo)
}

// ListarReservas devuelve una página de las reservas que matcheen el filtro,
// junto con el total de filas que matchean. La verificación de quién puede
// ver qué vive en interfaces/http (un docente solo puede pedir las suyas; un
// Admin, cualquiera).
//
// Una Pagina en cero no significa "traeme todo": es un filtro sin
// inicializar, y el LIMIT 0 que saldría de ahí devolvería una lista vacía
// sin ningún error. Se completa con la ventana por defecto.
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

// ListarEquiposDisponiblesEn implementa la selección de PCs de RF-04.2.
func (s *Service) ListarEquiposDisponiblesEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) ([]EquipoDisponible, error) {
	if horaFin <= horaInicio {
		return nil, domain.ErrRangoHorarioInvalido
	}
	return s.repo.ListarEquiposDisponiblesEn(ctx, fecha, horaInicio, horaFin)
}

// ObtenerReservaGrupo es un passthrough directo al repo — mismo criterio
// que ObtenerReserva, para verificar titularidad antes de
// CancelarOcurrenciaRecurrente.
func (s *Service) ObtenerReservaGrupo(ctx context.Context, id string) (*domain.ReservaGrupo, error) {
	return s.repo.BuscarReservaGrupoPorID(ctx, id)
}

// ── Recurrencia (RF-04.2) ───────────────────────────────────────────────

// ResultadoRecurrencia agrupa lo que devuelve CrearReservaRecurrente: la
// regla en sí, y un ReservaGrupo (con sus Reserva) por cada fecha
// generada.
type ResultadoRecurrencia struct {
	Regla  *domain.ReglaRecurrencia
	Grupos []*domain.ReservaGrupo
}

// maxOcurrenciasRecurrencia acota cuántas fechas puede materializar una
// sola serie recurrente.
//
// 45 son algo más de un año lectivo de clases semanales (la escuela tiene
// ~40 semanas de cursada), así que el caso de uso real de RF-04.2 —"todos
// los martes hasta que termine el año"— entra holgado. Lo que queda afuera
// es el pedido que no describe ninguna clase real: un fechaFin en 2099
// generaba 3.863 ReservaGrupo y otras tantas Reserva en un único request,
// con ~7.700 consultas de pre-chequeo y una respuesta JSON de 1,2 MB.
//
// El tope es sobre las ocurrencias y no sobre el rango de fechas porque es
// lo que realmente cuesta: son las filas que se insertan, las validaciones
// que se corren y el tamaño de la respuesta.
const maxOcurrenciasRecurrencia = 45

// CrearReservaRecurrente implementa RF-04.2: crea la ReglaRecurrencia y
// materializa un ReservaGrupo (con sus Reserva) por cada fecha que genera
// domain.ReglaRecurrencia.GenerarFechas(). Las mismas validaciones de
// CrearReserva (docente asignado, PCs disponibles, sin solapamiento)
// corren para CADA fecha generada — si una sola fecha falla (ej. una PC ya
// está ocupada ese día puntual por otra cosa), toda la operación se aborta
// sin dejar reglas ni grupos a medio crear.
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

	// El tope se aplica ANTES del pre-chequeo de solapamiento: ese chequeo
	// hace una consulta por PC y por fecha, así que dejarlo correr sobre un
	// rango sin acotar ya era el grueso del problema, aunque después no se
	// insertara nada.
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
	// serie. La serie se rechaza entera en vez de saltearle las ocurrencias
	// vencidas: quien pidió "todos los martes desde marzo" a mitad de año
	// espera que le digan que marzo ya pasó, no recibir una serie con menos
	// clases de las que pidió y sin explicación.
	if err := domain.ValidarVentanaTemporal(fechas[0], horaInicio, horaFin, s.ahora()); err != nil {
		if errors.Is(err, domain.ErrReservaEnElPasado) {
			return nil, fmt.Errorf("%w: la serie arranca el %s", err, fechas[0].Format("2006-01-02"))
		}
		return nil, err
	}

	// Se verifica el solapamiento de TODAS las fechas antes de crear nada
	// — así una falla en la fecha 5 de 10 no deja 4 grupos ya creados.
	for _, fecha := range fechas {
		if err := s.verificarSinSolapamiento(ctx, equipoIDs, fecha, horaInicio, horaFin); err != nil {
			return nil, err
		}
	}

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	// La regla, sus PCs y las N ocurrencias van en una sola transacción.
	// La verificación anticipada de arriba reduce las chances de conflicto
	// pero no las elimina (otro pedido puede colarse entre la lectura y la
	// escritura): si la fecha 7 de 40 falla, se deshacen las 6 anteriores y
	// la regla, en vez de dejar media serie creada.
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
// puntual de una serie recurrente. Si soloEsta es true, cancela
// únicamente ese grupo (RF-04.6 "solo esta semana"); si es false, cancela
// ese grupo y todos los futuros de la misma regla a partir de esa fecha
// (RF-04.6 "esta y las siguientes").
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

// ── Bloqueo por evaluación estatal (RF-04.7) ───────────────────────────

// ResultadoBloqueoEvaluacion agrupa lo que devuelve BloquearParaEvaluacion.
type ResultadoBloqueoEvaluacion struct {
	Bloqueos            []*domain.Reserva
	ReservasCanceladas  int
	DocentesNotificados int
}

// BloquearParaEvaluacion implementa RF-04.7: un Admin bloquea una o más
// PCs en un rango horario definido para una evaluación estatal. Cualquier
// Reserva NORMAL que se superponga con ese rango, en esas PCs, se cancela
// en cascada (nunca cancela otro bloqueo de evaluación existente — dos
// evaluaciones no deberían solaparse en la práctica, y si pasara, no es
// este método el que debe resolverlo).
func (s *Service) BloquearParaEvaluacion(ctx context.Context, equipoIDs []string, creadoPor *string, fecha time.Time, horaInicio, horaFin time.Duration, motivoBloqueo string) (*ResultadoBloqueoEvaluacion, error) {
	if err := verificarCantidadDeEquipos(equipoIDs); err != nil {
		return nil, err
	}

	ahora := s.ahora()

	// Un bloqueo sobre un horario que ya terminó no bloquea nada: no cancela
	// reservas (la cascada de abajo solo mira las que siguen vivas) y deja una
	// fila que solo sirve para ensuciar los reportes.
	//
	// El tope de duración, en cambio, NO se aplica acá — mismo criterio que
	// EsDiaLectivo, que tampoco se le impone a RF-04.7: una evaluación estatal
	// es excepcional por naturaleza y es el Admin quien decide cuánto dura, así
	// que tomarse el laboratorio un día entero es una decisión suya, no un
	// error. Lo que sí sigue valiendo es que la hora de fin sea posterior a la
	// de inicio (domain.NuevaReservaEvaluacion).
	if domain.YaTermino(fecha, horaFin, ahora) {
		return nil, domain.ErrReservaEnElPasado
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, err
	}

	docentesAfectados := map[string]bool{}
	reservasCanceladas := 0
	bloqueos := make([]*domain.Reserva, 0, len(equipoIDs))
	var pendientes []cancelacionPendiente

	// Todo el bloqueo es una sola transacción. Es especialmente importante
	// acá: las cancelaciones que dispara esta cascada NO se restauran
	// automáticamente (RF-03.8), así que fallar después de haber cancelado
	// las reservas de las primeras PCs dejaría a esos docentes sin su
	// reserva y sin bloqueo que lo justifique.
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

				// Solo la razón: la frase "Tu reserva fue cancelada" la
				// pone el suscriptor de notification. Este texto también
				// es el que se guarda en motivo_cancelacion y el que
				// "Mis reservas" muestra al lado de la PC afectada.
				motivoCascada := fmt.Sprintf("bloqueo por evaluación estatal (%s)", motivoBloqueo)
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

			bloqueo, err := domain.NuevaReservaEvaluacion(s.nuevoID(), equipoID, creadoPor, fecha, horaInicio, horaFin, ahora)
			if err != nil {
				return err
			}
			if err := repo.CrearReserva(ctx, bloqueo); err != nil {
				if errors.Is(err, ErrSolapamiento) {
					return err
				}
				return fmt.Errorf("creando bloqueo de evaluación para el equipo %s: %w", equipoID, err)
			}
			bloqueos = append(bloqueos, bloqueo)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.publicarCancelaciones(ctx, pendientes)

	return &ResultadoBloqueoEvaluacion{
		Bloqueos:            bloqueos,
		ReservasCanceladas:  reservasCanceladas,
		DocentesNotificados: len(docentesAfectados),
	}, nil
}

// ── Job de vencimiento (RF-04.9) ────────────────────────────────────────

const (
	// loteFinalizarVencidas es cuántas reservas procesa cada transacción del
	// job. Sin cota, el job leía TODO lo vencido en memoria y lo actualizaba
	// en una única transacción — y "todo lo vencido" no es una cantidad
	// acotada: crece con cada hora que el proceso haya estado caído, y
	// después de un fin de semana largo puede ser el atraso de varios días.
	loteFinalizarVencidas = 500

	// maxLotesPorCiclo acota cuánto puede durar UNA corrida del job. Con el
	// ticker de 5 minutos, 20 lotes son 10.000 reservas por ciclo: más que
	// suficiente para cualquier atraso real de una escuela, y si alguna vez
	// no alcanzara, el ciclo siguiente sigue por donde quedó. Es la
	// diferencia entre un job que tarda de más y uno que no termina nunca.
	maxLotesPorCiclo = 20
)

// FinalizarVencidas implementa RF-04.9 — se llama periódicamente desde un
// goroutine + time.Ticker en cmd/main.go. Marca FINALIZADA cada Reserva
// CONFIRMADA cuya fecha+horaFin ya pasó, y recalcula el estado de cada
// ReservaGrupo afectado.
//
// Procesa de a lotes, cada uno en su propia transacción. Un lote que
// commitea es trabajo que ya no hay que rehacer: con una transacción única,
// un fallo sobre la reserva 9.000 tiraba abajo las 8.999 anteriores y el
// ciclo siguiente volvía a empezar de cero, con la misma chance de fallar.
func (s *Service) FinalizarVencidas(ctx context.Context) (int, error) {
	total := 0

	for lote := 0; lote < maxLotesPorCiclo; lote++ {
		leidas, finalizadas, err := s.finalizarLoteVencidas(ctx)
		total += finalizadas
		if err != nil {
			// Lo finalizado hasta acá ya está commiteado: se devuelve junto
			// con el error para que el log del job no diga "0" cuando en
			// realidad avanzó.
			return total, err
		}
		if leidas < loteFinalizarVencidas {
			break // no queda nada más vencido
		}
		if finalizadas == 0 {
			// Se leyó un lote completo y no avanzó ninguna: son reservas que
			// el repo devuelve como vencidas pero que no pueden transicionar.
			// Seguir pidiendo el mismo lote sería un bucle infinito.
			break
		}
	}

	return total, nil
}

// finalizarLoteVencidas procesa un lote en una sola transacción y devuelve
// cuántas filas leyó y cuántas efectivamente finalizó. Las dos cifras no
// coinciden si alguna reserva no pudo transicionar, y esa diferencia es lo
// que le dice al llamador que insistir no va a servir.
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

// finalizarGrupoSiCorresponde marca un ReservaGrupo como FINALIZADA
// cuando ninguna de sus Reserva sigue CONFIRMADA (ya sea porque todas
// finalizaron, o una mezcla de finalizadas+canceladas). Si el grupo ya es
// CANCELADA o FINALIZADA, no hace nada.
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
//
// Estos dos métodos son lo que reservation expone para que inventory y
// auth puedan disparar una cascada de cancelación sin reimplementar la
// máquina de estados de Reserva/ReservaGrupo con SQL crudo — a diferencia
// de los validadores de solo-lectura (que sí van directo por SQL en cada
// paquete), cancelar es una ACCIÓN con reglas de negocio reales (una
// reserva ya cancelada no se puede volver a cancelar, el ReservaGrupo
// padre tiene que recalcular su estado, etc.) que solo debe existir en un
// lugar. Por eso cmd/main.go (el único punto que conoce todos los
// paquetes a la vez) envuelve estos métodos en un adaptador que satisface
// los puertos ValidadorReservas de inventory/auth, en vez de que esos
// paquetes reimplementen la lógica por su cuenta.

// CancelarReservasFuturasDeEquipo implementa el lado "reservation" de la
// cascada que dispara inventory (RF-03.8/03.9: cambio de estado o baja de
// una PC). Cancela toda Reserva CONFIRMADA de esa PC desde ahora en
// adelante, y recalcula el estado de cada ReservaGrupo afectado.
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

// TieneReservasFuturasDeEquipo responde si a esa PC todavía le quedan reservas
// CONFIRMADA sin terminar. Es el equivalente para inventory de lo que
// TieneReservasDeCiclo es para academic: distingue dos situaciones que desde
// afuera se ven igual —una PC que ya está fuera de servicio o dada de baja—
// pero que merecen respuestas opuestas al pedir la operación de nuevo.
//
//   - sin reservas futuras: la cascada de RF-03.8/03.9 ya terminó. Pedirlo
//     otra vez es un error y devuelve 409.
//   - CON reservas futuras: un intento anterior murió entre el guardado de
//     la PC y la cancelación. Ahí el 409 era el problema y no la protección
//     — dejaba reservas vivas sobre una PC que ya no existe, sin ninguna
//     forma de completar la cascada desde la API.
//
// Es una lectura, no una acción, pero vive acá y no como SQL directo en
// inventory/infrastructure porque "reserva futura" ya tiene una definición
// no trivial en este paquete (fecha + hora_fin contra el instante actual,
// ver condicionNoTerminada) y duplicarla en otro paquete es exactamente
// cómo dos criterios terminan divergiendo.
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
// cascada que dispara auth (RF-02.8: dar de baja al único docente
// asignado a una materia). Cancela toda Reserva CONFIRMADA vinculada a
// esa materia desde ahora en adelante, y recalcula el estado de cada
// ReservaGrupo afectado.
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

// cancelarEnCascada centraliza lo que ya hacían por separado
// BloquearParaEvaluacion y estos dos métodos: cancelar cada reserva
// CONFIRMADA de la lista (sin "cancelado por" — son cascadas disparadas
// por el sistema, no por un click puntual de un usuario sobre esa reserva
// en particular) y recalcular el ReservaGrupo padre de cada una.
// Corre siempre dentro de una transacción abierta por quien la llama, y
// devuelve las notificaciones a emitir DESPUÉS del commit en vez de
// publicarlas sobre la marcha.
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

// EliminarReservasDeCiclo implementa el lado "reservation" de la cascada
// de archivado de academic (RF-02.4) — borra físicamente todas las
// Reserva/ReservaGrupo de ese ciclo lectivo, sin importar su estado (a
// diferencia de Cancelar*, que solo cambia el estado, esto elimina las
// filas de verdad). Se llama DESPUÉS de que reporting ya calculó y
// guardó el snapshot histórico (ver ArchivarSnapshotDeCiclo en
// reporting/application) — el orden se coordina desde cmd/main.go, no
// acá.
func (s *Service) EliminarReservasDeCiclo(ctx context.Context, cicloID string) (gruposEliminados int, reservasEliminadas int, err error) {
	return s.repo.EliminarReservasYGruposDeCiclo(ctx, cicloID)
}

// ErrReservaNoModificable: solo una reserva CONFIRMADA se puede cambiar de
// máquina. Una cancelada, finalizada o liberada por no retiro ya no reserva
// nada, así que cambiarle la PC no significaría nada.
var ErrReservaNoModificable = errors.New("esa reserva ya no está vigente")

// CambiarEquipoDeReserva mueve una reserva a otra máquina sin partir la clase en
// dos (RF-08.14).
//
// Hace falta porque el barrido puede avisar que una PC no volvió, y hasta
// ahora la única salida era cancelar esa reserva y crear otra — que arma un
// ReservaGrupo nuevo, así que el docente terminaba con la misma clase
// mostrada como dos tarjetas separadas en "Mis reservas".
//
// Se apoya en la constraint EXCLUDE para el anti-solapamiento, igual que
// crear: se valida antes para dar un mensaje mejor, pero la garantía real
// ante dos pedidos simultáneos es la de la base.
func (s *Service) CambiarEquipoDeReserva(ctx context.Context, reservaID, pcNuevoID string, quien string, esAdmin bool) (*domain.Reserva, error) {
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
		if err := s.verificarSinSolapamiento(ctx, []string{pcNuevoID}, r.Fecha, r.HoraInicio, r.HoraFin); err != nil {
			return err
		}

		r.EquipoID = pcNuevoID
		if err := repo.GuardarReserva(ctx, r); err != nil {
			return err
		}
		cambiada = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cambiada, nil
}
