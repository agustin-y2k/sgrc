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
	contactos := s.contactosDeLosAfectados(ctx, orden)

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
				Nombre:    contactos[k.usuarioID].Nombre,
				Email:     contactos[k.usuarioID].Email,
				Motivo:    k.motivo,
				Reservas:  detalle,
			},
		})
	}
}

// contactosDeLosAfectados resuelve a quién escribirle, en una sola consulta.
// Si falla, el aviso interno sale igual y sin correo: perder la notificación
// por no haber podido leer un email sería mucho peor que no mandar el mail.
func (s *Service) contactosDeLosAfectados(ctx context.Context, orden []destinatario) map[string]Contacto {
	vistos := map[string]bool{}
	ids := make([]string, 0, len(orden))
	for _, k := range orden {
		if !vistos[k.usuarioID] {
			vistos[k.usuarioID] = true
			ids = append(ids, k.usuarioID)
		}
	}

	contactos, err := s.obtenedorNombre.ContactosDe(ctx, ids)
	if err != nil {
		log.Printf("cancelaciones: no se pudieron resolver los contactos (el aviso sale sin correo): %v", err)
		return map[string]Contacto{}
	}
	return contactos
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

// ImpactoDelRobo es el saldo del desplazamiento (RF-03.21): cuántas reservas
// ajenas hubo que cancelar para poder crear ésta, y a cuántos docentes les
// tocó. Queda en cero en la enorme mayoría de las reservas, que no desplazan a
// nadie.
//
// Quien reserva no se cuenta a sí mismo: un docente que se corre entre dos de
// sus cursos cancela una reserva y afecta a CERO docentes. Es coherente con
// que tampoco reciba ningún aviso — si no se le avisó a nadie, no hay nadie
// afectado.
type ImpactoDelRobo struct {
	ReservasCanceladas int
	DocentesAfectados  int
}

// CrearReserva implementa RF-04.1: un docente reserva una o más PCs para
// su materia, en una fecha y horario puntual.
//
// equiposATomar es el subconjunto de equipoIDs que quien reserva eligió
// SABIENDO que estaba ocupado, y es lo único que autoriza a desplazar a otro
// (RF-03.21). Vacío —lo que manda hoy toda la aplicación— la creación se
// comporta exactamente como antes del robo: cualquier solapamiento la frena.
// No es un flag: es el pedido del docente, y por eso viaja como dato.
func (s *Service) CrearReserva(ctx context.Context, materiaID, usuarioID string, esAdmin bool, fecha time.Time, horaInicio, horaFin time.Duration, equipoIDs, equiposATomar []string) (*domain.ReservaGrupo, []*domain.Reserva, ImpactoDelRobo, error) {
	var sinRobo ImpactoDelRobo

	if err := verificarCantidadDeEquipos(equipoIDs); err != nil {
		return nil, nil, sinRobo, err
	}

	if err := domain.ValidarVentanaTemporal(fecha, horaInicio, horaFin, s.ahora()); err != nil {
		return nil, nil, sinRobo, err
	}

	if err := s.verificarDentroDeLaJornada(ctx, fecha, horaInicio, horaFin); err != nil {
		return nil, nil, sinRobo, err
	}

	if err := s.verificarPuedeReservar(ctx, materiaID, usuarioID, esAdmin); err != nil {
		return nil, nil, sinRobo, err
	}

	if err := s.verificarEquiposReservables(ctx, equipoIDs); err != nil {
		return nil, nil, sinRobo, err
	}

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, nil, sinRobo, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	nombreMateria, err := s.nombreParaElMotivoDelRobo(ctx, materiaID)
	if err != nil {
		return nil, nil, sinRobo, err
	}

	// Todo el lote va en una sola transacción: si la PC número 3 de 5 choca
	// contra la constraint EXCLUDE (otro pedido simultáneo se adelantó), el
	// grupo y las dos reservas anteriores se deshacen en vez de quedar
	// commiteadas a medias (RF-04.5). Desde que reservar puede DESPLAZAR a
	// otro, adentro de esa misma transacción viajan además las cancelaciones:
	// una falla posterior tiene que devolverle la clase al desplazado.
	var grupo *domain.ReservaGrupo
	var reservas []*domain.Reserva
	var impacto ImpactoDelRobo
	var pendientes []cancelacionPendiente
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		var err error
		impacto, pendientes, err = s.despejarConflictos(ctx, repo, materiaID, nombreMateria, &usuarioID,
			equipoIDs, equiposATomar, []time.Time{fecha}, horaInicio, horaFin)
		if err != nil {
			return err
		}
		grupo, reservas, err = s.materializarGrupo(ctx, repo, materiaID, &usuarioID, nombreDocente, fecha, horaInicio, horaFin, equipoIDs, nil)
		return err
	})
	if err != nil {
		return nil, nil, sinRobo, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return grupo, reservas, impacto, nil
}

// nombreParaElMotivoDelRobo resuelve cómo se llama la materia que reserva, que
// es lo único que el motivo de cancelación no tiene a mano.
//
// Se pregunta ANTES de abrir la transacción, aunque la enorme mayoría de las
// reservas no vayan a desplazar a nadie y la consulta no le sirva a nadie: el
// validador va por el pool, y pedirle una conexión nueva mientras la
// transacción tiene tomada la suya es quedarse esperando el día que el pool
// esté justo — con las filas de las víctimas ya bloqueadas.
func (s *Service) nombreParaElMotivoDelRobo(ctx context.Context, materiaID string) (string, error) {
	nombre, err := s.validadorMateria.NombreDeMateria(ctx, materiaID)
	if err != nil {
		return "", fmt.Errorf("obteniendo el nombre de la materia: %w", err)
	}
	return nombre, nil
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

// puedeCancelar decide si quien pide puede cancelar eso, y con qué requisitos.
//
// Dos reglas en una función porque van juntas y se explican juntas:
//
//   - **Lo propio se cancela sin explicar nada**: el docente decide sobre su
//     clase.
//   - **Lo ajeno sólo lo cancela un Admin, y con motivo** (RF-04.8). El motivo
//     no es burocracia: es el texto exacto que el docente recibe en el aviso
//     (RF-05.1), así que vacío lo dejaría enterándose de que le cancelaron la
//     clase sin ninguna explicación.
//
// `creadoPor` es nil cuando la reserva la creó el sistema (un bloqueo
// administrativo): ahí no hay dueño, así que sólo un Admin la toca.
func puedeCancelar(creadoPor *string, solicitanteID string, esAdmin bool, motivo string) error {
	esPropia := creadoPor != nil && *creadoPor == solicitanteID
	if esPropia {
		return nil
	}
	if !esAdmin {
		return ErrNoEsTuReserva
	}
	if strings.TrimSpace(motivo) == "" {
		return ErrMotivoObligatorio
	}
	return nil
}

// puedeVer es la mitad de puedeCancelar que pregunta sólo por la titularidad:
// es tuyo, o sos Admin.
//
// Va aparte y no reusa puedeCancelar porque aquélla suma la regla de RF-04.8 —
// el Admin que toca algo ajeno tiene que decir por qué—, y sobre una lectura
// ese motivo no existe: nadie recibe un aviso por que le hayan mirado la
// reserva.
//
// `creadoPor` nil es un bloqueo administrativo: no tiene dueño, así que sólo un
// Admin lo ve.
func puedeVer(creadoPor *string, solicitanteID string, esAdmin bool) error {
	if esAdmin {
		return nil
	}
	if creadoPor == nil || *creadoPor != solicitanteID {
		return ErrNoEsTuReserva
	}
	return nil
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

// sePuedeRobar compone las DOS mitades de la regla del robo (RF-03.21), y es
// el único lugar del sistema donde se componen: la preferencia, que contesta
// ReservasRobablesPor contra la base, y el corte de antelación, que es hora de
// pared y se resuelve en Go.
//
// Lo usan las dos puntas —la escritura, para autorizar el robo, y la lista de
// ocupados, para ofrecerlo— por el mismo motivo por el que el SQL de la regla
// está en un solo lugar: dos composiciones son dos oportunidades de que
// diverjan, y el síntoma es la pantalla ofreciendo un robo que el backend
// después rechaza.
//
// La fecha y la hora son SIEMPRE las de la reserva en disputa, nunca las de la
// franja que se consultó o se pidió, y por eso van nombradas así. No son la
// misma: sqlSolapaConLaFranja trae a propósito reservas del día anterior y del
// siguiente, porque una nocturna de 22:00 a 01:00 fechada ayer pisa la franja
// de hoy. Evaluada con la fecha consultada, esa fila parecería empezar HOY a
// las 22:00 —veintidós horas por delante, robable de sobra— cuando en realidad
// arrancó anoche y ya no se toca.
func sePuedeRobar(robables map[string]bool, reservaID string,
	fechaDeLaReserva time.Time, horaInicioDeLaReserva time.Duration, ahora time.Time) bool {
	return robables[reservaID] &&
		domain.SePuedeRobarPorHorario(fechaDeLaReserva, horaInicioDeLaReserva, ahora)
}

// despejarConflictos es el enganche de RF-03.21 del lado de la escritura:
// reemplaza el corte seco ante cualquier solapamiento por "clasificá, y si
// TODO lo que estorba se puede correr, corrélo".
//
// Corre SIEMPRE dentro de la transacción que va a crear la reserva, y con las
// filas en conflicto tomadas. Hasta acá el pre-chequeo de solapamiento era
// informativo —lo que impide dos reservas pisadas es la constraint EXCLUDE, y
// lo sigue siendo—, pero ahora además decide A QUIÉN SE LE CANCELA LA CLASE, y
// eso no se puede decidir sobre una lectura vieja: entre listar los conflictos
// y cancelarlos, otro pedido pudo haberlos tocado.
//
// El todo-o-nada se mantiene tal cual: alcanza UN conflicto que no sea robable
// para devolver el error de siempre sin haber cancelado nada. Y como lo que
// cancela vive en la misma transacción que la creación, una falla posterior
// —la EXCLUDE, por ejemplo— le devuelve la clase al desplazado.
func (s *Service) despejarConflictos(ctx context.Context, repo Repo, materiaID, nombreMateria string,
	quienRoba *string, equipoIDs, equiposATomar []string, fechas []time.Time, horaInicio, horaFin time.Duration,
) (ImpactoDelRobo, []cancelacionPendiente, error) {
	conflictos, err := repo.BuscarSolapamientos(ctx, equipoIDs, fechas, horaInicio, horaFin)
	if err != nil {
		return ImpactoDelRobo{}, nil, fmt.Errorf("verificando solapamiento: %w", err)
	}
	if len(conflictos) == 0 {
		return ImpactoDelRobo{}, nil, nil
	}

	// Sin equipos tomados a sabiendas no hay robo posible, y esta rama es
	// LITERALMENTE el comportamiento anterior a RF-03.21: el mismo error, sin
	// tomar filas ni preguntar por preferencias. Es lo que mantiene inerte al
	// sistema mientras ninguna pantalla mande la lista.
	if len(equiposATomar) == 0 {
		return ImpactoDelRobo{}, nil, &ErrorDeSolapamiento{Conflictos: conflictos}
	}

	// Sólo se desplaza lo que quien reserva eligió SABIENDO que estaba ocupado.
	// Un equipo tildado en la lista de libres no autoriza ningún robo, y en una
	// serie recurrente esa es toda la diferencia: los equipos se eligen libres
	// en la fecha de inicio y los conflictos aparecen en las fechas 2..N, así
	// que sin esto un docente cancela clases ajenas sin haberlo pedido nunca.
	//
	// La contracara también es deliberada: tomar un equipo ocupado autoriza a
	// desplazarlo en TODAS las fechas de la serie. Ahí la cascada es lo que se
	// está pidiendo, no un efecto colateral.
	tomar := make(map[string]bool, len(equiposATomar))
	for _, id := range equiposATomar {
		tomar[id] = true
	}

	ids := make([]string, 0, len(conflictos))
	for _, c := range conflictos {
		ids = append(ids, c.ReservaID)
	}

	// Primero tomar las filas, después clasificar: al revés, la clasificación
	// se haría sobre un estado que otro pedido todavía puede cambiar.
	tomadas, err := repo.BuscarReservasParaActualizar(ctx, ids)
	if err != nil {
		return ImpactoDelRobo{}, nil, fmt.Errorf("tomando las reservas en conflicto: %w", err)
	}
	robables, err := repo.ReservasRobablesPor(ctx, materiaID, ids)
	if err != nil {
		return ImpactoDelRobo{}, nil, fmt.Errorf("clasificando las reservas en conflicto: %w", err)
	}

	ahora := s.ahora()
	var aDesplazar []*domain.Reserva
	var bloqueantes []Solapamiento
	for _, c := range conflictos {
		r, tomada := tomadas[c.ReservaID]
		if !tomada {
			// La fila desapareció entre que la vimos y fuimos a tomarla. No debería
			// pasar —una reserva sólo se borra al archivar el ciclo—, y por eso
			// cuenta como bloqueante en vez de saltearse: no se roba lo que no se
			// pudo verificar.
			bloqueantes = append(bloqueantes, c)
			continue
		}
		if r.Estado != domain.ReservaConfirmada {
			// Dejó de estorbar entre que la vimos y la tomamos. No es un conflicto
			// ni es un robo: es una franja que se liberó sola.
			continue
		}
		if tomar[c.EquipoID] && sePuedeRobar(robables, c.ReservaID, r.Fecha, r.HoraInicio, ahora) {
			aDesplazar = append(aDesplazar, r)
			continue
		}
		bloqueantes = append(bloqueantes, c)
	}

	if len(bloqueantes) > 0 {
		// El error nombra SÓLO los bloqueantes: sumarle los que el sistema estaba
		// dispuesto a correr le daría al docente un problema que no tiene.
		return ImpactoDelRobo{}, nil, &ErrorDeSolapamiento{Conflictos: bloqueantes}
	}
	if len(aDesplazar) == 0 {
		return ImpactoDelRobo{}, nil, nil
	}

	// El motivo es todo lo que el desplazado recibe, y por eso nombra a la
	// materia y no a la persona: lo que lo corrió es una marca del Admin sobre
	// ese equipo, no un colega.
	motivo := "Un equipo preferente de " + nombreMateria + " lo necesitaba"

	// canceladoPor con el docente que reserva, a diferencia de las otras
	// cascadas: acá sí hay una persona atrás, y queda el rastro de quién
	// desplazó a quién en la fila misma. De paso resuelve el auto-robo, que es
	// un efecto buscado: publicarCancelaciones saltea el aviso cuando quien
	// cancela es quien reservó, así que un docente que se corre a sí mismo entre
	// dos de sus cursos no recibe ningún correo.
	canceladas, docentes, pendientes, err := s.cancelarEnCascada(ctx, repo, aDesplazar, motivo, quienRoba, ahora)
	if err != nil {
		return ImpactoDelRobo{}, nil, err
	}
	return ImpactoDelRobo{ReservasCanceladas: canceladas, DocentesAfectados: docentes}, pendientes, nil
}

func mismaFecha(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// ObtenerReserva trae una reserva, y sólo a quien le corresponde verla.
//
// Era un passthrough sin regla, y su comentario decía que la titularidad la
// verificaba interfaces/http antes de dejar cancelar. Dejó de ser cierto con el
// barrido de RF-00.3 —la regla bajó al servicio— y desde entonces esta función
// no la llamaba nadie. Ahora sostiene el GET de una reserva sola, con la misma
// regla que el grupo: es tuya, o sos Admin.
func (s *Service) ObtenerReserva(ctx context.Context, id, solicitanteID string, esAdmin bool) (*domain.Reserva, error) {
	r, err := s.repo.BuscarReservaPorID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := puedeVer(r.CreadoPor, solicitanteID, esAdmin); err != nil {
		return nil, err
	}
	return r, nil
}

// CancelarReserva implementa RF-04.4: cancelación manual de una PC puntual
// dentro de un grupo (motivo obligatorio).
func (s *Service) CancelarReserva(ctx context.Context, reservaID, solicitanteID string, esAdmin bool, motivo string) error {
	var pendientes []cancelacionPendiente
	canceladoPor := &solicitanteID

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		r, err := repo.BuscarReservaPorID(ctx, reservaID)
		if err != nil {
			return err
		}

		// Quién puede cancelar qué, y con qué requisitos, se decide ACÁ y no en
		// el handler: es una regla del dominio, y dejarla afuera significa que
		// cualquier otro llamador del servicio la saltea sin enterarse.
		if err := puedeCancelar(r.CreadoPor, solicitanteID, esAdmin, motivo); err != nil {
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
// Devuelve también CÓMO SE LLAMA el equipo, porque la pantalla que dibuja
// esto se titulaba "Calendario del equipo" sin decir cuál: se llega desde un
// botón por equipo, y abrir tres en pestañas distintas daba tres pantallas
// idénticas. El nombre sale de EtiquetasDeEquipos, el mismo que usan los
// avisos, así que dice "PC 7 del Carro 2" y no "PC 7".
func (s *Service) CalendarioDeEquipo(ctx context.Context, equipoID string, desde, hasta time.Time) (etiqueta string, bloques []BloqueCalendario, err error) {
	if hasta.Before(desde) {
		return "", nil, domain.ErrRangoFechasInvalido
	}

	// Se pregunta si el equipo existe antes de traer sus bloques. Sin esto, un
	// id inventado devolvía 200 con la lista vacía y la pantalla dibujaba la
	// semana entera: "no existe" y "está libre" se veían igual.
	//
	// EquipoEstaEnInventario y no EquipoDisponibleParaReservar: una máquina en
	// mantenimiento sí tiene calendario —lo que ya está reservado sigue ahí, y
	// es justo lo que hay que mirar para reprogramarlo—; lo que no tiene
	// calendario es algo que no está en el inventario.
	enInventario, err := s.validadorEquipo.EquipoEstaEnInventario(ctx, equipoID)
	if err != nil {
		return "", nil, fmt.Errorf("verificando el equipo del calendario: %w", err)
	}
	if !enInventario {
		return "", nil, ErrEquipoNoEncontrado
	}

	bloques, err = s.repo.CalendarioDeEquipo(ctx, equipoID, desde, hasta)
	if err != nil {
		return "", nil, err
	}

	// Si el nombre no se puede resolver, el calendario sale igual y la
	// pantalla se queda con su título genérico: perder el calendario entero
	// por no poder titularlo sería una mala transacción.
	etiquetas, err := s.validadorEquipo.EtiquetasDeEquipos(ctx, []string{equipoID})
	if err != nil {
		log.Printf("reservation: no se pudo resolver el nombre del equipo %s para el calendario: %v", equipoID, err)
		return "", bloques, nil
	}
	return etiquetas[equipoID], bloques, nil
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
//
// materiaID es la materia para la que se está reservando, y suma la segunda
// oferta posible sobre lo ocupado (RF-03.21): además de pedirlo, ¿se lo puede
// robar? Se resuelve con la MISMA regla que usa despejarConflictos al crear
// —ReservasRobablesPor + domain.SePuedeRobarPorHorario—, y a propósito: una
// segunda comparación de prioridad acá es la copia que se desincroniza, y la
// pantalla terminaría ofreciendo un robo que el backend después rechaza.
func (s *Service) ListarEquiposOcupadosEn(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration, quien, materiaID string) ([]EquipoOcupado, error) {
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
		// oc.Fecha y no `fecha`: la consulta trae a propósito reservas del día
		// anterior y del siguiente (una nocturna de ayer pisa la franja de hoy),
		// así que la fecha consultada no es la de la reserva. Con `fecha`, la
		// nocturna de anoche parecía empezar hoy y se ofrecía pedirla cuando ya
		// estaba en curso.
		oc.PuedePedirse = !oc.EsBloqueo &&
			oc.DocenteID != nil && *oc.DocenteID != quien &&
			!domain.YaEmpezo(oc.Fecha, oc.HoraInicio, ahora)
	}

	// Sin materia no hay marca propia contra la cual comparar —mismo criterio
	// que ListarEquiposDisponiblesEn con materia vacía— y ni vale la pena
	// preguntarle a la regla del robo, que sin materia no tiene con qué
	// contestar.
	if materiaID == "" || len(ocupados) == 0 {
		return ocupados, nil
	}

	ids := make([]string, len(ocupados))
	for i, oc := range ocupados {
		ids[i] = oc.ReservaID
	}
	robables, err := s.repo.ReservasRobablesPor(ctx, materiaID, ids)
	if err != nil {
		return nil, err
	}
	// La regla contesta por RESERVA y la acción es por EQUIPO, así que hay que
	// juntarlas con un AND: la creación es todo-o-nada sobre TODOS los
	// conflictos del equipo en la franja pedida. Un equipo con una reserva
	// débil de 8 a 10 y una fuerte de 10 a 12 no se puede tomar de 8 a 12,
	// aunque el primer renglón, mirado solo, diga que sí. Sin esto la pantalla
	// ofrece un robo que la creación después rechaza.
	tomableElEquipo := make(map[string]bool, len(ocupados))
	for i := range ocupados {
		oc := &ocupados[i]
		robable := sePuedeRobar(robables, oc.ReservaID, oc.Fecha, oc.HoraInicio, ahora)
		if yaVisto, hay := tomableElEquipo[oc.EquipoID]; hay {
			robable = robable && yaVisto
		}
		tomableElEquipo[oc.EquipoID] = robable
	}
	for i := range ocupados {
		ocupados[i].PuedeTomarse = tomableElEquipo[ocupados[i].EquipoID]
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

// ObtenerReservaGrupo trae una serie de reservas, y sólo a quien le
// corresponde verla.
//
// La titularidad se resuelve acá y no en el handler por lo mismo que en
// CancelarReserva: quién puede ver qué es una regla del dominio, y una
// comprobación que sólo hace el transporte la saltea cualquier llamador que no
// entre por HTTP.
func (s *Service) ObtenerReservaGrupo(ctx context.Context, id, solicitanteID string, esAdmin bool) (*domain.ReservaGrupo, error) {
	g, err := s.repo.BuscarReservaGrupoPorID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := puedeVer(g.CreadoPor, solicitanteID, esAdmin); err != nil {
		return nil, err
	}
	return g, nil
}

// ── Recurrencia (RF-04.2) ───────────────────────────────────────────────

// ResultadoRecurrencia agrupa lo que devuelve CrearReservaRecurrente: la
// regla en sí, y un ReservaGrupo (con sus Reserva) por cada fecha generada.
type ResultadoRecurrencia struct {
	Regla  *domain.ReglaRecurrencia
	Grupos []*domain.ReservaGrupo
	// Impacto: a cuántos desplazó la serie entera (RF-03.21). Una serie anual
	// roba ocurrencia por ocurrencia, así que un solo click puede correr
	// decenas de clases de varios docentes.
	Impacto ImpactoDelRobo
}

// MaxOcurrenciasRecurrencia acota cuántas fechas puede materializar una sola
// serie recurrente.
const MaxOcurrenciasRecurrencia = 45

// CrearReservaRecurrente implementa RF-04.2: crea la ReglaRecurrencia y
// materializa un ReservaGrupo (con sus Reserva) por cada fecha que genera
// domain.ReglaRecurrencia.GenerarFechas().
//
// equiposATomar es lo mismo que en CrearReserva, y acá importa el doble: los
// equipos de una serie se eligen mirando la fecha de INICIO, así que sin ese
// permiso explícito un docente que elige máquinas libres el primer día
// terminaría desplazando clases ajenas en las fechas 2..N sin haberlo pedido.
func (s *Service) CrearReservaRecurrente(ctx context.Context, materiaID, usuarioID string, esAdmin bool, diaSemana domain.DiaSemana, horaInicio, horaFin time.Duration, fechaInicio, fechaFin time.Time, equipoIDs, equiposATomar []string) (*ResultadoRecurrencia, error) {
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

	// El tope se aplica antes que nada de lo que cuesta: un rango sin acotar
	// genera cientos de fechas, y cada una es un ReservaGrupo más sus reservas
	// dentro de una sola transacción. Antes frenaba además un pre-chequeo de
	// solapamiento que hacía una consulta por PC y por fecha; ese chequeo hoy
	// vive adentro de la transacción y resuelve todo el lote de una, pero el
	// tope sigue yendo primero por el costo de materializar.
	if len(fechas) == 0 {
		return nil, ErrSinOcurrencias
	}
	if len(fechas) > MaxOcurrenciasRecurrencia {
		return nil, fmt.Errorf("%w: son %d clases y el máximo es %d",
			ErrDemasiadasOcurrencias, len(fechas), MaxOcurrenciasRecurrencia)
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

	nombreDocente, err := s.obtenedorNombre.NombreCompletoDe(ctx, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("obteniendo nombre del docente: %w", err)
	}

	nombreMateria, err := s.nombreParaElMotivoDelRobo(ctx, materiaID)
	if err != nil {
		return nil, err
	}

	// La regla, sus PCs y las N ocurrencias van en una sola transacción, y con
	// ellas el despeje de TODAS las fechas antes de crear nada — así una falla
	// en la fecha 5 de 10 no deja 4 grupos ya creados ni, ahora, cuatro
	// docentes desplazados sin que la serie exista.
	grupos := make([]*domain.ReservaGrupo, 0, len(fechas))
	var impacto ImpactoDelRobo
	var pendientes []cancelacionPendiente
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		grupos = grupos[:0]

		var err error
		impacto, pendientes, err = s.despejarConflictos(ctx, repo, materiaID, nombreMateria, &usuarioID,
			equipoIDs, equiposATomar, fechas, horaInicio, horaFin)
		if err != nil {
			return err
		}

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

	s.publicarCancelaciones(ctx, pendientes)
	return &ResultadoRecurrencia{Regla: regla, Grupos: grupos, Impacto: impacto}, nil
}

// CancelarOcurrenciaRecurrente implementa RF-04.6: cancela un ReservaGrupo
// puntual de una serie recurrente.
func (s *Service) CancelarOcurrenciaRecurrente(ctx context.Context, reservaGrupoID, solicitanteID string, esAdmin bool, motivo string, soloEsta bool) (int, error) {
	totalCancelado := 0
	var pendientes []cancelacionPendiente
	canceladoPor := &solicitanteID

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		totalCancelado = 0
		pendientes = nil

		grupo, err := repo.BuscarReservaGrupoPorID(ctx, reservaGrupoID)
		if err != nil {
			return err
		}

		// Mismo criterio que CancelarReserva: la regla vive en el servicio.
		if err := puedeCancelar(grupo.CreadoPor, solicitanteID, esAdmin, motivo); err != nil {
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

	// Las dos reglas que comparte con una reserva: que el rango diga algo y que
	// no esté ya terminado. Un bloqueo sobre un horario que pasó no bloquea
	// nada —no cancela reservas, porque la cascada de abajo sólo mira las que
	// siguen vivas— y deja una fila que sólo ensucia los reportes. Un bloqueo de
	// duración cero tampoco bloquea nada, y hasta acá llegaba a la base y salía
	// como 500.
	//
	// Lo que NO se aplica es el tope de ocho horas: un bloqueo de la jornada
	// entera es un caso legítimo.
	if err := domain.ValidarVentanaDeBloqueo(fecha, horaInicio, horaFin, ahora); err != nil {
		return nil, err
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

		// Las reservas futuras de TODOS los equipos del lote en una consulta. Un
		// bloqueo puede llegar con doscientas máquinas y antes preguntaba por
		// cada una; el índice `(equipo_id, fecha)` sirve igual a las dos formas.
		futurasPorEquipo, err := repo.ListarReservasFuturasDeEquipos(ctx, equipoIDs, fecha)
		if err != nil {
			return fmt.Errorf("listando las reservas de los equipos a bloquear: %w", err)
		}

		for _, equipoID := range equipoIDs {
			// Un equipo sin reservas futuras no está en el mapa y la lista sale
			// vacía sola.
			for _, r := range futurasPorEquipo[equipoID] {
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
		canceladas, docentes, pendientes, err = s.cancelarEnCascada(ctx, repo, futuras, motivo, nil, ahora)
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
		canceladas, _, pendientes, err = s.cancelarEnCascada(ctx, repo, futuras, motivo, nil, ahora)
		return err
	})
	if err != nil {
		return 0, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return canceladas, nil
}

// ReservasFuturas lista lo que todavía no terminó, para que availability
// pueda decidir qué quedaría fuera de una jornada nueva. No decide nada acá:
// la regla de qué entra en la jornada vive del otro lado, que es donde
// también la aplica el alta de una reserva.
func (s *Service) ReservasFuturas(ctx context.Context, desde time.Time) ([]ReservaDetallada, error) {
	return s.repo.ListarReservasFuturas(ctx, desde)
}

// CancelarReservasPorIDs cancela esas reservas puntuales con ese motivo.
//
// A diferencia de las otras cascadas, acá la lista de víctimas llega armada
// desde afuera: quien decide cuáles caen es availability, con la jornada
// nueva en la mano. Este lado se limita a cancelarlas con las mismas reglas
// que el resto —sin "cancelado por", porque no es el click de una persona
// sobre esa reserva— y a emitir los avisos.
//
// Las que ya no estén CONFIRMADA se saltean en silencio: entre que se calculó
// el impacto y el Admin confirmó, alguna pudo cancelarse sola.
func (s *Service) CancelarReservasPorIDs(ctx context.Context, reservaIDs []string, motivo string) (int, error) {
	if len(reservaIDs) == 0 {
		return 0, nil
	}

	ahora := s.ahora()
	var canceladas int
	var pendientes []cancelacionPendiente

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		// Una consulta para todo el lote. Acá el id que no está se SALTEA y no es
		// un error, a diferencia de las entregas: esta cascada la disparan otras
		// operaciones —dar de baja un equipo, quitar al último docente de una
		// materia— y una reserva que desapareció en el medio es una menos que
		// cancelar, no un pedido mal armado.
		//
		// El orden lo sigue poniendo `reservaIDs` y no el mapa.
		porID, err := repo.BuscarReservasPorIDs(ctx, reservaIDs)
		if err != nil {
			return fmt.Errorf("buscando las reservas a cancelar: %w", err)
		}
		reservas := make([]*domain.Reserva, 0, len(reservaIDs))
		for _, id := range reservaIDs {
			if r, hay := porID[id]; hay {
				reservas = append(reservas, r)
			}
		}

		canceladas, _, pendientes, err = s.cancelarEnCascada(ctx, repo, reservas, motivo, nil, ahora)
		return err
	})
	if err != nil {
		return 0, err
	}

	s.publicarCancelaciones(ctx, pendientes)
	return canceladas, nil
}

// cancelarEnCascada centraliza lo que ya hacían por separado BloquearEquipos
// y estos dos métodos: cancelar cada reserva CONFIRMADA de la lista y
// recalcular el ReservaGrupo padre de cada una.
//
// `canceladoPor` va en nil en las cascadas del sistema —baja de un equipo,
// baja de un docente, cambio de jornada, cancelación por ids—, que no son el
// click de nadie sobre esa reserva en particular. El robo de RF-03.21 es la
// excepción y pasa al docente que se llevó la máquina.
//
// Cuesta del orden de 3-4 consultas por reserva cancelada (GuardarReserva más
// actualizarEstadoGrupo), y desde el robo eso lo dispara un docente con un
// click y no un Admin dando de baja un equipo. Medido con el peor caso que el
// sistema permite —una serie de MaxOcurrenciasRecurrencia fechas, todas
// tomadas por una materia más débil— da 170-210 ms de punta a punta contra
// Postgres en contenedor local (TestCrearReservaRecurrente_RoboDeLaSerieMasLarga_CuantoTarda).
// Lejos del par de segundos que haría falta para atacar la deuda de N+1, así
// que sigue esperando; el servidor de verdad habla con la base por un túnel,
// que es lo que podría dar vuelta el número.
func (s *Service) cancelarEnCascada(ctx context.Context, repo Repo, reservas []*domain.Reserva, motivo string, canceladoPor *string, ahora time.Time) (int, int, []cancelacionPendiente, error) {
	docentesAfectados := map[string]bool{}
	canceladas := 0
	var pendientes []cancelacionPendiente

	for _, r := range reservas {
		if r.Estado != domain.ReservaConfirmada {
			continue
		}
		if err := r.Cancelar(canceladoPor, motivo, ahora); err != nil {
			return 0, 0, nil, err
		}
		if err := repo.GuardarReserva(ctx, r); err != nil {
			return 0, 0, nil, err
		}
		pendientes = append(pendientes, cancelacionPendiente{reserva: r, motivo: motivo})
		canceladas++
		// A quien cancela no se le cuenta como afectado, por lo mismo que
		// publicarCancelaciones no le manda el aviso: nadie necesita que le avisen
		// de algo que acaba de hacer. Es el docente que se corre a sí mismo entre
		// dos de sus cursos — si no salió ningún aviso, no hay ningún afectado.
		if r.CreadoPor != nil && (canceladoPor == nil || *r.CreadoPor != *canceladoPor) {
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
		// Misma regla de titularidad que ver y que cancelar: es tuya, o sos Admin.
		if err := puedeVer(r.CreadoPor, quien, esAdmin); err != nil {
			return err
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
