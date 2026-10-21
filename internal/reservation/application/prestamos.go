package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

// Entregas y devoluciones de PCs (RF-08).

// maxHistorialDeEquipo acota el historial de entregas de una máquina. Es una
// pantalla para mirar los últimos movimientos, no un reporte.
const maxHistorialDeEquipo = 50

// RazonNoEntregada es por qué una PC del lote quedó afuera.
type RazonNoEntregada string

const (
	// NoEntregadaYaPrestada: la máquina ya está en manos de otra persona.
	NoEntregadaYaPrestada RazonNoEntregada = "YA_ENTREGADA"
	// NoEntregadaFueraDelInventario: dada de baja o inexistente.
	NoEntregadaFueraDelInventario RazonNoEntregada = "FUERA_DEL_INVENTARIO"
	// NoEntregadaReservaCancelada: se intentó entregar contra una reserva
	// que ya no está vigente.
	NoEntregadaReservaCancelada RazonNoEntregada = "RESERVA_CANCELADA"
	// NoEntregadaSinDestinatario: la reserva no dice a nombre de quién entregar.
	NoEntregadaSinDestinatario RazonNoEntregada = "SIN_DESTINATARIO"
	// NoEntregadaFueraDeCirculacion: el equipo está en mantenimiento o fuera
	// de servicio. Está físicamente acá y no se le da a nadie (RF-08.17).
	NoEntregadaFueraDeCirculacion RazonNoEntregada = "FUERA_DE_CIRCULACION"
)

// EstadoEquipoDisponible es el único estado de inventory con el que un equipo
// sale del laboratorio en una entrega normal. Se compara como texto: el
// puerto ya devuelve la cadena, y son dos contextos distintos.
const EstadoEquipoDisponible = "DISPONIBLE"

// porQueNoCircula traduce el estado de inventory a por qué esa máquina no se
// le da a nadie. Lo lee quien está en el mostrador con alguien esperando
// enfrente, así que dice el hecho y no el código.
var porQueNoCircula = map[string]string{
	"EN_MANTENIMIENTO":  "ese equipo está en mantenimiento",
	"FUERA_DE_SERVICIO": "ese equipo está fuera de servicio",
}

// ErrSalidaAReparacionSinDestino: sacar del laboratorio algo que no está en
// condiciones de prestarse necesita decir a dónde va. Es la constancia, y es
// lo único que distingue esa salida de un préstamo común mal cargado.
var ErrSalidaAReparacionSinDestino = errors.New("una salida a reparación tiene que decir a dónde va el equipo")

// EquipoNoEntregado explica por qué una PC del lote no salió.
type EquipoNoEntregado struct {
	EquipoID string
	Razon    RazonNoEntregada
	Detalle  string
}

// ReservaProxima avisa que una máquina que se acaba de entregar tiene una
// reserva encima.
type ReservaProxima struct {
	EquipoID string
	Fecha    time.Time
	Inicio   time.Duration
	Fin      time.Duration
	Docente  string
}

// ResultadoEntrega es qué salió y qué no.
type ResultadoEntrega struct {
	Entregadas   []*domain.Prestamo
	NoEntregadas []EquipoNoEntregado
	// Avisos: "ojo que este equipo tiene una reserva encima".
	Avisos []ReservaProxima
	// Liberadas: las que el docente no se llevó y dejaron de estar guardadas
	// para él, en una entrega parcial declarada. Se devuelven para poder
	// decirlo en pantalla: soltar una máquina en silencio es lo que hace que
	// después nadie entienda por qué figura libre.
	Liberadas []string
}

// ── Entrega contra una reserva ──────────────────────────────────────────

// EntregaPorReservaParams — se entregan las Reserva puntuales (una por PC),
// no el grupo entero, porque el retiro es máquina por máquina: puede llevarse
// tres de las cinco que reservó.
type EntregaPorReservaParams struct {
	ReservaIDs []string
	// RetiradoPor: quién vino a buscarlas, si no fue el docente de la reserva.
	RetiradoPor  string
	EntregadoPor string
	// LiberarNoEntregadas es la ENTREGA PARCIAL declarada: el Admin dice que
	// esto completa el retiro. Se liberan las reservas confirmadas que nunca
	// tuvieron una entrega; lo retirado en tandas anteriores se conserva.
	//
	// Antes esto lo decidía un reloj —quince minutos desde la última entrega,
	// RETIRO_PARCIAL_GRACIA_MINUTOS— porque el sistema no tenía cómo saber si
	// el docente iba a volver por las otras dos. Preguntárselo a quien está en
	// el mostrador es más barato y más exacto que adivinarlo: él lo tiene
	// enfrente. Un cuarto de hora en que la máquina figura ocupada sin estarlo
	// es, en la práctica, la hora siguiente perdida para otro curso.
	LiberarNoEntregadas bool
}

// EntregarPorReserva registra que las máquinas de una reserva salieron.
func (s *Service) EntregarPorReserva(ctx context.Context, params EntregaPorReservaParams) (*ResultadoEntrega, error) {
	var resultado *ResultadoEntrega
	err := s.repo.Coordinar(ctx, false, func(ctx context.Context) error {
		var err error
		resultado, err = s.entregarPorReservaCoordinada(ctx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) entregarPorReservaCoordinada(ctx context.Context, params EntregaPorReservaParams) (*ResultadoEntrega, error) {
	if err := verificarCantidadDeEquipos(params.ReservaIDs); err != nil {
		return nil, err
	}

	ahora := s.ahora()
	resultado := &ResultadoEntrega{}

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		resultado = &ResultadoEntrega{}
		// Las reservas del lote y sus hermanas se bloquean juntas en orden estable.
		// Así otra entrega del grupo no puede entrar entre consulta de custodia y liberación.
		ids := idsDeLoteSinRepetir(params.ReservaIDs)
		// Las reservas del lote en UNA consulta. Se recorre la lista de IDs y no
		// el mapa: el resultado se arma en el orden en que la pantalla mandó las
		// máquinas, y el recorrido de un mapa en Go es aleatorio entre corridas.
		reservas, err := repo.BuscarReservasParaEntregar(ctx, ids)
		if err != nil {
			return err
		}

		// Validar todos los IDs antes de registrar la primera entrega.
		for _, id := range ids {
			if reservas[id] == nil {
				return ErrReservaNoEncontrada
			}
		}
		for _, reservaID := range ids {
			reserva := reservas[reservaID]

			if reserva.Estado == domain.ReservaCancelada {
				resultado.NoEntregadas = append(resultado.NoEntregadas, EquipoNoEntregado{
					EquipoID: reserva.EquipoID,
					Razon:    NoEntregadaReservaCancelada,
					Detalle:  "esa reserva está cancelada",
				})
				continue
			}

			vence := domain.FinDePared(reserva.Fecha, reserva.HoraInicio, reserva.HoraFin, ahora.Location())

			// Quien responde es SIEMPRE el docente de la reserva: él la hizo y a él se
			// le reclama.
			nombre := ""
			if reserva.NombreDocenteSnapshot != nil {
				nombre = *reserva.NombreDocenteSnapshot
			}
			retiradoPor := params.RetiradoPor
			// Un bloqueo administrativo no tiene docente, así que no hay a quién hacer
			// responsable: ahí el nombre que se escribe a mano SÍ es el responsable, y
			// no queda nadie "al lado" a quien anotar.
			if nombre == "" {
				nombre, retiradoPor = retiradoPor, ""
			}
			// Un bloqueo administrativo no tiene docente: lo crea un Admin sobre PCs
			// sueltas y NombreDocenteSnapshot queda en nil.
			if nombre == "" {
				resultado.NoEntregadas = append(resultado.NoEntregadas, EquipoNoEntregado{
					EquipoID: reserva.EquipoID,
					Razon:    NoEntregadaSinDestinatario,
					Detalle:  "esa reserva no tiene docente (es un bloqueo administrativo): escribí a nombre de quién se entrega",
				})
				continue
			}

			// UsuarioID se queda apuntando al docente aunque las haya retirado otro:
			// los avisos de "no volvieron" tienen que llegarle a quien responde, y
			// quien retira muchas veces ni cuenta tiene.
			datos := domain.DatosDeEntrega{
				EquipoID:           reserva.EquipoID,
				ReservaID:          &reserva.ID,
				UsuarioID:          reserva.CreadoPor,
				Nombre:             nombre,
				RetiradoPor:        retiradoPor,
				DevolucionEstimada: &vence,
				EntregadoPor:       params.EntregadoPor,
			}

			// Una reserva nunca autoriza sacar un equipo roto: si la máquina dejó
			// de estar disponible, sus reservas se cancelaron en cascada (RF-03.8)
			// y esta entrega llega tarde.
			prestamo, noEntregada, err := s.registrarEntrega(ctx, repo, datos, ahora, false)
			if err != nil {
				return err
			}
			if noEntregada != nil {
				resultado.NoEntregadas = append(resultado.NoEntregadas, *noEntregada)
				continue
			}
			resultado.Entregadas = append(resultado.Entregadas, prestamo)

			// Entregar contra una reserva LIBERADA es legítimo —el docente llegó tarde
			// y la máquina seguía ahí—, pero en el rato que estuvo libre otro pudo
			// haberla reservado.
			if reserva.Estado == domain.ReservaNoRetirada {
				if aviso := s.reservaProximaDe(ctx, repo, reserva.EquipoID, ahora); aviso != nil {
					resultado.Avisos = append(resultado.Avisos, *aviso)
				}
			}
		}

		if params.LiberarNoEntregadas {
			liberadas, err := s.liberarElResto(ctx, repo, reservas, resultado.Entregadas)
			if err != nil {
				return err
			}
			resultado.Liberadas = liberadas
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resultado, nil
}

// liberarElResto suelta lo que el docente NO se llevó, y sólo eso.
//
// El alcance es el GRUPO de las reservas entregadas — una clase, un día, un
// horario— y ahí termina. Una serie que se repite todos los martes tiene un
// grupo por fecha, con sus propias reservas: liberar las de hoy no puede tocar
// las del martes que viene ni por descuido, porque el martes que viene es otro
// grupo y esta función nunca lo lee. Es la garantía por construcción, no por
// cuidado al escribir la consulta.
func (s *Service) liberarElResto(ctx context.Context, repo Repo, reservas map[string]*domain.Reserva, entregadas []*domain.Prestamo) ([]string, error) {
	grupos := map[string]bool{}
	for _, p := range entregadas {
		if p.ReservaID != nil {
			if r := reservas[*p.ReservaID]; r != nil && r.ReservaGrupoID != nil {
				grupos[*r.ReservaGrupoID] = true
			}
		}
	}
	if len(grupos) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(reservas))
	for id := range reservas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// Incluye préstamos ya devueltos: NO_RETIRADA no describe una máquina que
	// efectivamente se retiró, aunque haya vuelto antes de completar la tanda.
	conEntrega, err := repo.ReservasConPrestamos(ctx, ids)
	if err != nil {
		return nil, err
	}
	var liberadas []string
	for _, id := range ids {
		r := reservas[id]
		if r.ReservaGrupoID == nil || !grupos[*r.ReservaGrupoID] || conEntrega[id] || r.Estado != domain.ReservaConfirmada {
			continue
		}
		if err := r.Liberar(); err != nil {
			return nil, err
		}
		if err := repo.GuardarReserva(ctx, r); err != nil {
			return nil, err
		}
		liberadas = append(liberadas, id)
	}
	grupoIDs := make([]string, 0, len(grupos))
	for id := range grupos {
		grupoIDs = append(grupoIDs, id)
	}
	sort.Strings(grupoIDs)
	for _, id := range grupoIDs {
		if err := recalcularGrupo(ctx, repo, id); err != nil {
			return nil, err
		}
	}
	return liberadas, nil
}

// ── Entrega espontánea ──────────────────────────────────────────────────

// EntregaSueltaParams es el préstamo sin reserva detrás: "necesito una
// compu para hacer un trámite".
type EntregaSueltaParams struct {
	EquipoIDs []string
	// Nombre es obligatorio; UsuarioID solo si esa persona tiene cuenta.
	Nombre    string
	UsuarioID *string
	// RetiradoPor: si la pide una persona y la viene a buscar otra.
	RetiradoPor string
	// Destino es a dónde va el equipo. Opcional en la entrega del día a día
	// y obligatorio en una salida a reparación.
	Destino string
	// DevolucionEstimada opcional: "vengo en un rato" es la respuesta
	// honesta, y una hora inventada solo generaría reclamos falsos.
	DevolucionEstimada *time.Time
	EntregadoPor       string
	// SalidaAReparacion autoriza sacar un equipo que NO está DISPONIBLE. Es
	// el único camino: sin esta marca, una máquina en mantenimiento o fuera
	// de servicio no sale, y con ella sale dejando constancia de a dónde fue.
	// La pantalla que la manda es un panel aparte del mostrador — entregar un
	// equipo roto no es una variante de la entrega del día a día.
	SalidaAReparacion bool
}

func (s *Service) EntregarSuelta(ctx context.Context, params EntregaSueltaParams) (*ResultadoEntrega, error) {
	var resultado *ResultadoEntrega
	err := s.repo.Coordinar(ctx, false, func(ctx context.Context) error {
		var err error
		resultado, err = s.entregarSueltaCoordinada(ctx, params)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) entregarSueltaCoordinada(ctx context.Context, params EntregaSueltaParams) (*ResultadoEntrega, error) {
	if err := verificarCantidadDeEquipos(params.EquipoIDs); err != nil {
		return nil, err
	}
	if params.SalidaAReparacion && strings.TrimSpace(params.Destino) == "" {
		return nil, ErrSalidaAReparacionSinDestino
	}

	ahora := s.ahora()
	resultado := &ResultadoEntrega{}

	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		resultado = &ResultadoEntrega{}
		for _, equipoID := range idsDeLoteSinRepetir(params.EquipoIDs) {
			datos := domain.DatosDeEntrega{
				EquipoID:           equipoID,
				UsuarioID:          params.UsuarioID,
				Nombre:             params.Nombre,
				RetiradoPor:        params.RetiradoPor,
				Destino:            params.Destino,
				DevolucionEstimada: params.DevolucionEstimada,
				EntregadoPor:       params.EntregadoPor,
			}

			prestamo, noEntregada, err := s.registrarEntrega(ctx, repo, datos, ahora, params.SalidaAReparacion)
			if err != nil {
				return err
			}
			if noEntregada != nil {
				resultado.NoEntregadas = append(resultado.NoEntregadas, *noEntregada)
				continue
			}
			resultado.Entregadas = append(resultado.Entregadas, prestamo)

			if aviso := s.reservaProximaDe(ctx, repo, equipoID, ahora); aviso != nil {
				resultado.Avisos = append(resultado.Avisos, *aviso)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return resultado, nil
}

// registrarEntrega es el paso común: validar que la máquina pueda salir del
// laboratorio y crear el préstamo, traduciendo los rechazos esperables a una
// razón que la pantalla pueda mostrar.
//
// salidaAReparacion levanta la única condición que se puede levantar: la de
// que el equipo esté DISPONIBLE. Lo dado de baja no sale ni así — ya no es
// parte del parque, y lo que se saca del laboratorio es lo que después hay
// que esperar de vuelta.
func (s *Service) registrarEntrega(ctx context.Context, repo Repo, datos domain.DatosDeEntrega, ahora time.Time, salidaAReparacion bool) (*domain.Prestamo, *EquipoNoEntregado, error) {
	condicion, err := s.validadorEquipo.CondicionParaEntregar(ctx, datos.EquipoID)
	if err != nil {
		return nil, nil, fmt.Errorf("verificando el equipo %s: %w", datos.EquipoID, err)
	}
	if !condicion.EnInventario {
		return nil, &EquipoNoEntregado{
			EquipoID: datos.EquipoID,
			Razon:    NoEntregadaFueraDelInventario,
			Detalle:  ErrEquipoDadoDeBaja.Error(),
		}, nil
	}
	// RF-08.17: lo que está en mantenimiento o fuera de servicio está
	// físicamente acá y no se le da a nadie. Antes salía igual, y el mostrador
	// dejaba prestar una máquina que un Admin acababa de marcar como rota.
	if !salidaAReparacion && condicion.Estado != EstadoEquipoDisponible {
		detalle, conocido := porQueNoCircula[condicion.Estado]
		if !conocido {
			detalle = "ese equipo no está disponible"
		}
		return nil, &EquipoNoEntregado{
			EquipoID: datos.EquipoID,
			Razon:    NoEntregadaFueraDeCirculacion,
			Detalle:  detalle + ": para sacarlo del laboratorio, registrá una salida a reparación",
		}, nil
	}

	prestamo, err := domain.NuevoPrestamo(s.nuevoID(), datos, ahora)
	if err != nil {
		// Un nombre vacío o larguísimo es igual para todo el lote, así que
		// no tiene sentido seguir con las demás máquinas.
		return nil, nil, err
	}

	if err := repo.CrearPrestamo(ctx, prestamo); err != nil {
		if errors.Is(err, ErrEquipoYaPrestado) {
			return nil, &EquipoNoEntregado{
				EquipoID: datos.EquipoID,
				Razon:    NoEntregadaYaPrestada,
				Detalle:  ErrEquipoYaPrestado.Error(),
			}, nil
		}
		return nil, nil, err
	}
	return prestamo, nil, nil
}

// reservaProximaDe busca la primera reserva vigente de esa PC de acá en
// adelante.
func (s *Service) reservaProximaDe(ctx context.Context, repo Repo, equipoID string, ahora time.Time) *ReservaProxima {
	futuras, err := repo.ListarReservasFuturasDeEquipo(ctx, equipoID, ahora)
	if err != nil || len(futuras) == 0 {
		return nil
	}
	proxima := futuras[0]
	aviso := ReservaProxima{
		EquipoID: equipoID,
		Fecha:    proxima.Fecha,
		Inicio:   proxima.HoraInicio,
		Fin:      proxima.HoraFin,
	}
	if proxima.NombreDocenteSnapshot != nil {
		aviso.Docente = *proxima.NombreDocenteSnapshot
	}
	return &aviso
}

// EstaPrestado responde "¿este equipo está afuera del laboratorio ahora?".
func (s *Service) EstaPrestado(ctx context.Context, equipoID string) (bool, error) {
	_, err := s.repo.BuscarPrestamoAbiertoDeEquipo(ctx, equipoID)
	if errors.Is(err, ErrPrestamoNoEncontrado) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ── Devolución ──────────────────────────────────────────────────────────

// EquipoNoRecibido: la única razón posible es que ya figurara devuelta —dos
// Admin en el mostrador, o un doble clic—.
type EquipoNoRecibido struct {
	PrestamoID string
	Detalle    string
}

type ResultadoDevolucion struct {
	Recibidos   []*domain.Prestamo
	NoRecibidos []EquipoNoRecibido
}

// RecibirEquipos marca la vuelta de una o varias máquinas.
func (s *Service) RecibirEquipos(ctx context.Context, prestamoIDs []string, recibidoPor, observaciones string) (*ResultadoDevolucion, error) {
	var resultado *ResultadoDevolucion
	// El contador de cierre cruza préstamos distintos: las devoluciones deben
	// observar los commits anteriores para que la última emita pendientes=0.
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		var err error
		resultado, err = s.recibirEquiposCoordinada(ctx, prestamoIDs, recibidoPor, observaciones)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) recibirEquiposCoordinada(ctx context.Context, prestamoIDs []string, recibidoPor, observaciones string) (*ResultadoDevolucion, error) {
	if err := verificarCantidadDeEquipos(prestamoIDs); err != nil {
		return nil, err
	}

	ahora := s.ahora()
	resultado := &ResultadoDevolucion{}

	// Ídem EntregarPorReserva: una consulta para el lote, y el orden lo pone la
	// lista de entrada.
	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		resultado = &ResultadoDevolucion{}
		ids := idsDeLoteSinRepetir(prestamoIDs)
		prestamos, err := repo.BuscarPrestamosParaActualizar(ctx, ids)
		if err != nil {
			return err
		}

		for _, id := range ids {
			if prestamos[id] == nil {
				return ErrPrestamoNoEncontrado
			}
		}
		for _, id := range ids {
			prestamo := prestamos[id]

			if err := prestamo.Devolver(recibidoPor, observaciones, ahora); err != nil {
				if errors.Is(err, domain.ErrPrestamoYaDevuelto) {
					resultado.NoRecibidos = append(resultado.NoRecibidos, EquipoNoRecibido{
						PrestamoID: id,
						Detalle:    err.Error(),
					})
					continue
				}
				return err
			}
			if err := repo.GuardarPrestamo(ctx, prestamo); err != nil {
				return fmt.Errorf("registrando la devolución %s: %w", id, err)
			}
			resultado.Recibidos = append(resultado.Recibidos, prestamo)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(resultado.Recibidos) > 0 {
		eventbus.AlConfirmar(ctx, s.bus, func(ctx context.Context) { s.avisarSiVolvioTodoLoDelCierre(ctx) })
	}
	return resultado, nil
}

// avisarSiVolvioTodoLoDelCierre publica cuántos equipos del aviso de cierre
// siguen afuera después de esta devolución. Con cero, el aviso de la campana
// se cierra para todos los Admin (ver notification).
//
// Un fallo al contar no puede voltear la devolución, que ya está registrada:
// se loguea y sigue. El peor caso es un aviso que queda abierto hasta la
// próxima devolución, no una máquina que figura afuera estando adentro.
func (s *Service) avisarSiVolvioTodoLoDelCierre(ctx context.Context) {
	afuera, err := s.repo.ContarAvisadosSinDevolver(ctx)
	if err != nil {
		log.Printf("entregas: no se pudo contar qué queda afuera del aviso de cierre "+
			"(el aviso puede quedar abierto de más): %v", err)
		return
	}
	eventbus.Emitir(ctx, s.bus, eventbus.Evento{
		Tipo:    "prestamo.cierre.pendientes",
		Payload: eventbus.PendientesDelCierre{Pendientes: afuera},
	})
}

// ── Lecturas ────────────────────────────────────────────────────────────

// ListarPrestamosAbiertos es "qué hay afuera ahora mismo".
func (s *Service) ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error) {
	return s.repo.ListarPrestamosAbiertos(ctx)
}

// HistorialDeEquipo son los últimos movimientos de una máquina: quién se la
// llevó, cuándo volvió, qué se anotó al recibirla.
func (s *Service) HistorialDeEquipo(ctx context.Context, equipoID string) ([]*PrestamoDetallado, error) {
	return s.repo.ListarPrestamosDeEquipo(ctx, equipoID, maxHistorialDeEquipo)
}

// Ahora expone el reloj del servicio para que la capa HTTP pueda calcular los
// campos derivados de un préstamo (si está demorado, cuántos minutos) contra
// el mismo instante que usa el resto del sistema.
func (s *Service) Ahora() time.Time {
	return s.ahora()
}

// Conservar la primera aparición evita respuestas contradictorias para un ID
// repetido sin cambiar el orden pedido por la pantalla.
func idsDeLoteSinRepetir(ids []string) []string {
	vistos := make(map[string]bool, len(ids))
	resultado := make([]string, 0, len(ids))
	for _, id := range ids {
		if !vistos[id] {
			vistos[id] = true
			resultado = append(resultado, id)
		}
	}
	return resultado
}
