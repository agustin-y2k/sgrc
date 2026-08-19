package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
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
)

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
}

// EntregarPorReserva registra que las máquinas de una reserva salieron.
func (s *Service) EntregarPorReserva(ctx context.Context, params EntregaPorReservaParams) (*ResultadoEntrega, error) {
	if err := verificarCantidadDeEquipos(params.ReservaIDs); err != nil {
		return nil, err
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
			resultado.NoEntregadas = append(resultado.NoEntregadas, EquipoNoEntregado{
				EquipoID: reserva.EquipoID,
				Razon:    NoEntregadaReservaCancelada,
				Detalle:  "esa reserva está cancelada",
			})
			continue
		}

		vence := domain.InstanteDePared(reserva.Fecha, reserva.HoraFin, ahora.Location())

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

		prestamo, noEntregada, err := s.registrarEntrega(ctx, datos, ahora)
		if err != nil {
			return nil, err
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
			if aviso := s.reservaProximaDe(ctx, reserva.EquipoID, ahora); aviso != nil {
				resultado.Avisos = append(resultado.Avisos, *aviso)
			}
		}
	}

	return resultado, nil
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
	Motivo      string
	// DevolucionEstimada opcional: "vengo en un rato" es la respuesta
	// honesta, y una hora inventada solo generaría reclamos falsos.
	DevolucionEstimada *time.Time
	EntregadoPor       string
}

func (s *Service) EntregarSuelta(ctx context.Context, params EntregaSueltaParams) (*ResultadoEntrega, error) {
	if err := verificarCantidadDeEquipos(params.EquipoIDs); err != nil {
		return nil, err
	}

	ahora := s.ahora()
	resultado := &ResultadoEntrega{}

	for _, equipoID := range params.EquipoIDs {
		datos := domain.DatosDeEntrega{
			EquipoID:           equipoID,
			UsuarioID:          params.UsuarioID,
			Nombre:             params.Nombre,
			RetiradoPor:        params.RetiradoPor,
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

		if aviso := s.reservaProximaDe(ctx, equipoID, ahora); aviso != nil {
			resultado.Avisos = append(resultado.Avisos, *aviso)
		}
	}

	return resultado, nil
}

// registrarEntrega es el paso común: validar que la máquina esté en el
// inventario y crearla, traduciendo los dos rechazos esperables a una razón
// que la pantalla pueda mostrar.
func (s *Service) registrarEntrega(ctx context.Context, datos domain.DatosDeEntrega, ahora time.Time) (*domain.Prestamo, *EquipoNoEntregado, error) {
	enInventario, err := s.validadorEquipo.EquipoEstaEnInventario(ctx, datos.EquipoID)
	if err != nil {
		return nil, nil, fmt.Errorf("verificando el equipo %s: %w", datos.EquipoID, err)
	}
	if !enInventario {
		return nil, &EquipoNoEntregado{
			EquipoID: datos.EquipoID,
			Razon:    NoEntregadaFueraDelInventario,
			Detalle:  ErrEquipoDadoDeBaja.Error(),
		}, nil
	}

	prestamo, err := domain.NuevoPrestamo(s.nuevoID(), datos, ahora)
	if err != nil {
		// Un nombre vacío o larguísimo es igual para todo el lote, así que
		// no tiene sentido seguir con las demás máquinas.
		return nil, nil, err
	}

	if err := s.repo.CrearPrestamo(ctx, prestamo); err != nil {
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
func (s *Service) reservaProximaDe(ctx context.Context, equipoID string, ahora time.Time) *ReservaProxima {
	futuras, err := s.repo.ListarReservasFuturasDeEquipo(ctx, equipoID, ahora)
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
	if err := verificarCantidadDeEquipos(prestamoIDs); err != nil {
		return nil, err
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
				resultado.NoRecibidos = append(resultado.NoRecibidos, EquipoNoRecibido{
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
