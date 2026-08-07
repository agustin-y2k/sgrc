package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
)

// Entregas y devoluciones de PCs (RF-08).
//
// Reemplaza el papel en el que los Admin anotan qué máquinas se lleva cada
// docente y cuáles devuelve. Están en su propio archivo porque son un eje
// distinto del resto del paquete: reservar es planificar, entregar es lo que
// pasa en el mostrador.
//
// Las operaciones son en LOTE porque así ocurren: un docente se lleva cinco
// máquinas de una vez y las devuelve todas juntas. Y ninguna aborta el lote
// cuando una PC falla: el Admin tiene las otras cuatro en la mano, y hacerlo
// empezar de nuevo por una sola sería peor que informarle cuál fue.

// maxHistorialDePC acota el historial de entregas de una máquina. Es una
// pantalla para mirar los últimos movimientos, no un reporte.
const maxHistorialDePC = 50

// RazonNoEntregada es por qué una PC del lote quedó afuera. Va como código y
// no como texto suelto para que la pantalla pueda decidir qué ofrecer —"ver
// quién la tiene" no es la misma acción que "revisá el inventario".
type RazonNoEntregada string

const (
	// NoEntregadaYaPrestada: la máquina ya está en manos de otra persona.
	NoEntregadaYaPrestada RazonNoEntregada = "YA_ENTREGADA"
	// NoEntregadaFueraDelInventario: dada de baja o inexistente.
	NoEntregadaFueraDelInventario RazonNoEntregada = "FUERA_DEL_INVENTARIO"
	// NoEntregadaReservaCancelada: se intentó entregar contra una reserva
	// que ya no está vigente.
	NoEntregadaReservaCancelada RazonNoEntregada = "RESERVA_CANCELADA"
	// NoEntregadaSinDestinatario: la reserva no dice a nombre de quién
	// entregar. Pasa con los bloqueos por evaluación estatal (RF-04.7), que
	// no tienen docente: los crea un Admin sobre PCs sueltas. Se resuelve
	// escribiendo el nombre de quien las retira.
	NoEntregadaSinDestinatario RazonNoEntregada = "SIN_DESTINATARIO"
)

// PCNoEntregada explica por qué una PC del lote no salió.
type PCNoEntregada struct {
	PCID    string
	Razon   RazonNoEntregada
	Detalle string
}

// ReservaProxima avisa que una máquina que se acaba de entregar tiene una
// reserva encima. No impide nada: es información para que el Admin decida
// —entregar otra, avisarle al docente, o entregarla igual porque vuelve
// antes—. Decidir por él sería peor: el sistema no sabe cuánto va a durar un
// trámite.
type ReservaProxima struct {
	PCID    string
	Fecha   time.Time
	Inicio  time.Duration
	Fin     time.Duration
	Docente string
}

// ResultadoEntrega es qué salió y qué no.
type ResultadoEntrega struct {
	Entregadas   []*domain.Prestamo
	NoEntregadas []PCNoEntregada
	// Avisos solo se llena en las entregas espontáneas: en las que salen
	// contra una reserva, la reserva ES el permiso.
	Avisos []ReservaProxima
}

// ── Entrega contra una reserva ──────────────────────────────────────────

// EntregaPorReservaParams — se entregan las Reserva puntuales (una por PC),
// no el grupo entero, porque el retiro es máquina por máquina: puede
// llevarse tres de las cinco que reservó.
type EntregaPorReservaParams struct {
	ReservaIDs []string
	// NombreAlternativo: quién se las llevó, si no fue el docente de la
	// reserva. Pasa seguido —manda a un alumno o a un colega— y el papel lo
	// anota, así que el sistema también. Vacío = el docente de la reserva.
	NombreAlternativo string
	EntregadoPor      string
}

// EntregarPorReserva registra que las máquinas de una reserva salieron.
//
// La hora en que deben volver sale del fin de la reserva, no se pide: es el
// dato que ya está y que el Admin no tiene por qué volver a tipear.
func (s *Service) EntregarPorReserva(ctx context.Context, params EntregaPorReservaParams) (*ResultadoEntrega, error) {
	if len(params.ReservaIDs) == 0 {
		return nil, ErrSinPCs
	}

	ahora := s.ahora()
	resultado := &ResultadoEntrega{}

	for _, reservaID := range params.ReservaIDs {
		reserva, err := s.repo.BuscarReservaPorID(ctx, reservaID)
		if err != nil {
			// Un ID que no existe es un error del cliente, no una PC que no
			// se pudo entregar: la pantalla mandó algo que no corresponde.
			return nil, err
		}

		if reserva.Estado == domain.ReservaCancelada {
			resultado.NoEntregadas = append(resultado.NoEntregadas, PCNoEntregada{
				PCID:    reserva.PCID,
				Razon:   NoEntregadaReservaCancelada,
				Detalle: "esa reserva está cancelada",
			})
			continue
		}

		vence := domain.InstanteDePared(reserva.Fecha, reserva.HoraFin, ahora.Location())
		nombre := params.NombreAlternativo
		if nombre == "" && reserva.NombreDocenteSnapshot != nil {
			nombre = *reserva.NombreDocenteSnapshot
		}
		// Un bloqueo por evaluación estatal no tiene docente: lo crea un
		// Admin sobre PCs sueltas y NombreDocenteSnapshot queda en nil. Sin
		// esta rama, NuevoPrestamo devolvía ErrNombreDestinatarioVacio, y ese
		// error corta el lote entero — así que entregar cinco máquinas
		// fallaba con un 400 sin entregar ninguna porque una era un bloqueo.
		//
		// Se informa por PC en vez de cortar, y con nombre a mano sí se
		// entrega: alguien tiene que retirar las máquinas de una mesa de
		// examen.
		if nombre == "" {
			resultado.NoEntregadas = append(resultado.NoEntregadas, PCNoEntregada{
				PCID:    reserva.PCID,
				Razon:   NoEntregadaSinDestinatario,
				Detalle: "esa reserva no tiene docente (es un bloqueo por evaluación): escribí a nombre de quién se entrega",
			})
			continue
		}

		datos := domain.DatosDeEntrega{
			PCID:               reserva.PCID,
			ReservaID:          &reserva.ID,
			UsuarioID:          reserva.CreadoPor,
			Nombre:             nombre,
			DevolucionEstimada: &vence,
			EntregadoPor:       params.EntregadoPor,
		}
		// Si se la llevó otra persona, el usuario de la reserva ya no es
		// quien tiene la máquina: dejarlo apuntando al docente haría que los
		// avisos de devolución le lleguen a quien no la tiene.
		if params.NombreAlternativo != "" {
			datos.UsuarioID = nil
		}

		prestamo, noEntregada, err := s.registrarEntrega(ctx, datos, ahora)
		if err != nil {
			return nil, err
		}
		if noEntregada != nil {
			resultado.NoEntregadas = append(resultado.NoEntregadas, *noEntregada)
			continue
		}
		resultado.Entregadas = append(resultado.Entregadas, prestamo)
	}

	return resultado, nil
}

// ── Entrega espontánea ──────────────────────────────────────────────────

// EntregaSueltaParams es el préstamo sin reserva detrás: "necesito una
// compu para hacer un trámite".
type EntregaSueltaParams struct {
	PCIDs []string
	// Nombre es obligatorio; UsuarioID solo si esa persona tiene cuenta.
	// Quien viene a pedir una máquina para un trámite muchas veces no la
	// tiene —secretaría, preceptoría, un alumno— y obligar a que la tenga
	// significaría anotar el préstamo a nombre de otro.
	Nombre    string
	UsuarioID *string
	Motivo    string
	// DevolucionEstimada opcional: "vengo en un rato" es la respuesta
	// honesta, y una hora inventada solo generaría reclamos falsos.
	DevolucionEstimada *time.Time
	EntregadoPor       string
}

func (s *Service) EntregarSuelta(ctx context.Context, params EntregaSueltaParams) (*ResultadoEntrega, error) {
	if len(params.PCIDs) == 0 {
		return nil, ErrSinPCs
	}

	ahora := s.ahora()
	resultado := &ResultadoEntrega{}

	for _, pcID := range params.PCIDs {
		datos := domain.DatosDeEntrega{
			PCID:               pcID,
			UsuarioID:          params.UsuarioID,
			Nombre:             params.Nombre,
			Motivo:             params.Motivo,
			DevolucionEstimada: params.DevolucionEstimada,
			EntregadoPor:       params.EntregadoPor,
		}

		prestamo, noEntregada, err := s.registrarEntrega(ctx, datos, ahora)
		if err != nil {
			return nil, err
		}
		if noEntregada != nil {
			resultado.NoEntregadas = append(resultado.NoEntregadas, *noEntregada)
			continue
		}
		resultado.Entregadas = append(resultado.Entregadas, prestamo)

		if aviso := s.reservaProximaDe(ctx, pcID, ahora); aviso != nil {
			resultado.Avisos = append(resultado.Avisos, *aviso)
		}
	}

	return resultado, nil
}

// registrarEntrega es el paso común: validar que la máquina esté en el
// inventario y crearla, traduciendo los dos rechazos esperables a una razón
// que la pantalla pueda mostrar. Devuelve error solo ante fallas reales.
func (s *Service) registrarEntrega(ctx context.Context, datos domain.DatosDeEntrega, ahora time.Time) (*domain.Prestamo, *PCNoEntregada, error) {
	enInventario, err := s.validadorPC.PCEstaEnInventario(ctx, datos.PCID)
	if err != nil {
		return nil, nil, fmt.Errorf("verificando la PC %s: %w", datos.PCID, err)
	}
	if !enInventario {
		return nil, &PCNoEntregada{
			PCID:    datos.PCID,
			Razon:   NoEntregadaFueraDelInventario,
			Detalle: ErrPCDadaDeBaja.Error(),
		}, nil
	}

	prestamo, err := domain.NuevoPrestamo(s.nuevoID(), datos, ahora)
	if err != nil {
		// Un nombre vacío o larguísimo es igual para todo el lote, así que
		// no tiene sentido seguir con las demás máquinas.
		return nil, nil, err
	}

	if err := s.repo.CrearPrestamo(ctx, prestamo); err != nil {
		if errors.Is(err, ErrPCYaPrestada) {
			return nil, &PCNoEntregada{
				PCID:    datos.PCID,
				Razon:   NoEntregadaYaPrestada,
				Detalle: ErrPCYaPrestada.Error(),
			}, nil
		}
		return nil, nil, err
	}
	return prestamo, nil, nil
}

// reservaProximaDe busca la primera reserva vigente de esa PC de acá en
// adelante. Es informativo: si falla, la entrega ya se registró y no tiene
// sentido tirarla abajo por no haber podido mostrar un aviso.
func (s *Service) reservaProximaDe(ctx context.Context, pcID string, ahora time.Time) *ReservaProxima {
	futuras, err := s.repo.ListarReservasFuturasDePC(ctx, pcID, ahora)
	if err != nil || len(futuras) == 0 {
		return nil
	}
	proxima := futuras[0]
	aviso := ReservaProxima{
		PCID:   pcID,
		Fecha:  proxima.Fecha,
		Inicio: proxima.HoraInicio,
		Fin:    proxima.HoraFin,
	}
	if proxima.NombreDocenteSnapshot != nil {
		aviso.Docente = *proxima.NombreDocenteSnapshot
	}
	return &aviso
}

// ── Devolución ──────────────────────────────────────────────────────────

// PCNoRecibida: la única razón posible es que ya figurara devuelta —dos
// Admin en el mostrador, o un doble clic—. Se informa en vez de fallar,
// porque el resultado que el Admin quería (que la máquina figure adentro)
// ya está.
type PCNoRecibida struct {
	PrestamoID string
	Detalle    string
}

type ResultadoDevolucion struct {
	Recibidos   []*domain.Prestamo
	NoRecibidos []PCNoRecibida
}

// RecibirPCs marca la vuelta de una o varias máquinas.
//
// `observaciones` es una sola para todo el lote y normalmente va vacía. Si
// hay algo puntual que anotar sobre una máquina —"volvió sin el cargador"—
// se recibe esa sola: atarle una observación a cinco filas diría de las
// otras cuatro algo que no pasó.
func (s *Service) RecibirPCs(ctx context.Context, prestamoIDs []string, recibidoPor, observaciones string) (*ResultadoDevolucion, error) {
	if len(prestamoIDs) == 0 {
		return nil, ErrSinPCs
	}

	ahora := s.ahora()
	resultado := &ResultadoDevolucion{}

	for _, id := range prestamoIDs {
		prestamo, err := s.repo.BuscarPrestamoPorID(ctx, id)
		if err != nil {
			return nil, err
		}

		if err := prestamo.Devolver(recibidoPor, observaciones, ahora); err != nil {
			if errors.Is(err, domain.ErrPrestamoYaDevuelto) {
				resultado.NoRecibidos = append(resultado.NoRecibidos, PCNoRecibida{
					PrestamoID: id,
					Detalle:    err.Error(),
				})
				continue
			}
			return nil, err
		}
		if err := s.repo.GuardarPrestamo(ctx, prestamo); err != nil {
			return nil, fmt.Errorf("registrando la devolución %s: %w", id, err)
		}
		resultado.Recibidos = append(resultado.Recibidos, prestamo)
	}

	return resultado, nil
}

// ── Lecturas ────────────────────────────────────────────────────────────

// ListarPrestamosAbiertos es "qué hay afuera ahora mismo".
func (s *Service) ListarPrestamosAbiertos(ctx context.Context) ([]*PrestamoDetallado, error) {
	return s.repo.ListarPrestamosAbiertos(ctx)
}

// HistorialDePC son los últimos movimientos de una máquina: quién se la
// llevó, cuándo volvió, qué se anotó al recibirla.
func (s *Service) HistorialDePC(ctx context.Context, pcID string) ([]*PrestamoDetallado, error) {
	return s.repo.ListarPrestamosDePC(ctx, pcID, maxHistorialDePC)
}

// Ahora expone el reloj del servicio para que la capa HTTP pueda calcular
// los campos derivados de un préstamo (si está demorado, cuántos minutos)
// contra el mismo instante que usa el resto del sistema.
//
// Es el mismo criterio que Service.Hoy() en inventory: la regla vive en el
// dominio, pero quien arma la respuesta necesita el "cuándo".
func (s *Service) Ahora() time.Time {
	return s.ahora()
}
