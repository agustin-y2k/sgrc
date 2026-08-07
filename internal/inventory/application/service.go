// Package application orquesta los casos de uso de RF-03 (inventario:
// carros, PCs, incidencias).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

type Service struct {
	repo              Repo
	validadorReservas ValidadorReservas
	nuevoID           IDGenerator
	ahora             func() time.Time
}

func NewService(repo Repo, validadorReservas ValidadorReservas, nuevoID IDGenerator, ahora func() time.Time) *Service {
	return &Service{repo: repo, validadorReservas: validadorReservas, nuevoID: nuevoID, ahora: ahora}
}

// ── Carro ───────────────────────────────────────────────────────────────

func (s *Service) CrearCarro(ctx context.Context, nombre, descripcion string) (*domain.Carro, error) {
	c, err := domain.NuevoCarro(s.nuevoID(), nombre, descripcion)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearCarro(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// EditarCarro actualiza nombre y/o descripción — nil significa "no tocar
// ese campo" (RF-03.1: edición parcial).
func (s *Service) EditarCarro(ctx context.Context, carroID string, nombre, descripcion *string) error {
	c, err := s.repo.BuscarCarroPorID(ctx, carroID)
	if err != nil {
		return err
	}
	if nombre != nil {
		if err := c.RenombrarA(*nombre); err != nil {
			return err
		}
	}
	if descripcion != nil {
		c.Descripcion = *descripcion
	}
	return s.repo.GuardarCarro(ctx, c)
}

func (s *Service) ListarCarros(ctx context.Context) ([]*domain.Carro, error) {
	return s.repo.ListarCarros(ctx)
}

// ── PC ──────────────────────────────────────────────────────────────────

func (s *Service) CrearPC(ctx context.Context, carroID string, identificador int, numeroSerie string, freezado bool, cpu, ram, sistemaOperativo, softwareInstalado string) (*domain.PC, error) {
	pc, err := domain.NuevaPC(s.nuevoID(), carroID, identificador, numeroSerie, freezado, s.ahora())
	if err != nil {
		return nil, err
	}
	pc.CPU = cpu
	pc.RAM = ram
	pc.SistemaOperativo = sistemaOperativo
	pc.SoftwareInstalado = softwareInstalado

	if err := s.repo.CrearPC(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// EditarPCParams agrupa los campos editables de una PC — todos punteros,
// nil significa "no tocar ese campo" (RF-03.4, RF-03.10 para CarroID).
type EditarPCParams struct {
	CarroID           *string
	Freezado          *bool
	CPU               *string
	RAM               *string
	SistemaOperativo  *string
	SoftwareInstalado *string
}

func (s *Service) EditarPC(ctx context.Context, pcID string, params EditarPCParams) error {
	pc, err := s.repo.BuscarPCPorID(ctx, pcID)
	if err != nil {
		return err
	}

	if params.CarroID != nil {
		pc.MoverACarro(*params.CarroID)
	}
	if params.Freezado != nil {
		pc.Freezado = *params.Freezado
	}
	if params.CPU != nil {
		pc.CPU = *params.CPU
	}
	if params.RAM != nil {
		pc.RAM = *params.RAM
	}
	if params.SistemaOperativo != nil {
		pc.SistemaOperativo = *params.SistemaOperativo
	}
	if params.SoftwareInstalado != nil {
		pc.SoftwareInstalado = *params.SoftwareInstalado
	}

	return s.repo.GuardarPC(ctx, pc)
}

// ResultadoCascada es lo que el handler HTTP necesita para armar la
// respuesta de RF-03.8/03.9 (cuántas reservas se cancelaron, a cuántos
// docentes se notificó).
type ResultadoCascada struct {
	ReservasCanceladas  int
	DocentesNotificados int
}

// disparaCascada dice si ese estado saca a la PC de circulación y, por lo
// tanto, obliga a cancelar sus reservas futuras (RF-03.8).
func disparaCascada(estado domain.EstadoPC) bool {
	return estado == domain.EstadoEnMantenimiento || estado == domain.EstadoFueraDeServicio
}

// cascadaPendiente distingue "esta operación ya se hizo" de "esta operación
// se hizo a medias y hay que terminarla".
//
// Hace falta porque la cascada de RF-03.8/03.9 NO puede ser atómica con el
// guardado de la PC: cruza a reservation por un puerto, con su propia
// transacción, y meterlas en una sola rompería el límite de dominio
// (docs/06-arquitectura.md §3). Dado eso, lo que se puede elegir no es que
// nunca falle a la mitad sino qué queda cuando falla — mismo razonamiento
// que en auth.DarDeBaja.
//
// Sin esta distinción, lo que queda tras un fallo a mitad de camino es
// irrecuperable: la PC guardada en su nuevo estado, sus reservas todavía
// CONFIRMADA, los docentes sin aviso, y el reintento rebotando con 409 ("de
// EN_MANTENIMIENTO a EN_MANTENIMIENTO", "la PC ya está dada de baja")
// porque la máquina de estados rechaza repetir la transición. La única
// salida sería SQL a mano contra producción.
func (s *Service) cascadaPendiente(ctx context.Context, pcID string) (bool, error) {
	pendiente, err := s.validadorReservas.TieneReservasFuturas(ctx, pcID)
	if err != nil {
		return false, fmt.Errorf("verificando si quedó una cascada pendiente sobre la PC: %w", err)
	}
	return pendiente, nil
}

// errCascada envuelve el fallo del segundo paso dejando dicho que el primero
// SÍ se aplicó y que reintentar la misma operación completa lo que falta —
// mismo criterio que el error de ArchivarYClonar en academic.
func errCascada(err error) error {
	return fmt.Errorf("la PC quedó guardada en su nuevo estado pero no se pudieron cancelar sus reservas futuras "+
		"(reintentar la misma operación completa la cascada): %w", err)
}

// CambiarEstadoPC implementa RF-03.8: al pasar a EN_MANTENIMIENTO o
// FUERA_DE_SERVICIO, cancela en cascada las reservas futuras de esa PC.
func (s *Service) CambiarEstadoPC(ctx context.Context, pcID string, nuevo domain.EstadoPC, motivo *string) (*ResultadoCascada, error) {
	pc, err := s.repo.BuscarPCPorID(ctx, pcID)
	if err != nil {
		return nil, err
	}

	if errTransicion := pc.CambiarEstado(nuevo); errTransicion != nil {
		// Repetir la transición sigue siendo un error — salvo que la PC ya
		// esté en el estado pedido Y le queden reservas futuras vivas, que es
		// la huella que deja un intento anterior cortado entre el guardado y
		// la cascada. En ese caso esto no es "cambiar de estado de nuevo"
		// sino terminar lo que quedó a medias, y el paso que falta es
		// justamente el que sigue.
		//
		// Se compara contra pc.Estado y no se acepta cualquier error: pasar
		// de FUERA_DE_SERVICIO (terminal) a EN_MANTENIMIENTO tiene que
		// seguir siendo 409.
		if pc.Estado != nuevo || !disparaCascada(nuevo) {
			return nil, errTransicion
		}
		pendiente, err := s.cascadaPendiente(ctx, pcID)
		if err != nil {
			return nil, err
		}
		if !pendiente {
			return nil, errTransicion
		}
	} else if err := s.repo.GuardarPC(ctx, pc); err != nil {
		return nil, err
	}

	resultado := &ResultadoCascada{}
	if disparaCascada(nuevo) {
		motivoTexto := motivoPorDefecto(pc, nuevo, motivo)
		canceladas, notificados, err := s.validadorReservas.CancelarReservasFuturasDePC(ctx, pcID, motivoTexto)
		if err != nil {
			return nil, errCascada(err)
		}
		resultado.ReservasCanceladas = canceladas
		resultado.DocentesNotificados = notificados
	}

	return resultado, nil
}

// DarDeBajaPC implementa RF-03.4/03.9: soft delete + misma cascada de
// cancelación que CambiarEstadoPC (RF-03.9 dice explícitamente que dar de
// baja dispara la misma cascada que pasar a FUERA_DE_SERVICIO), incluido el
// mismo reintento cuando la cascada quedó a medias.
func (s *Service) DarDeBajaPC(ctx context.Context, pcID string) (*ResultadoCascada, error) {
	pc, err := s.repo.BuscarPCPorID(ctx, pcID)
	if err != nil {
		return nil, err
	}

	if errBaja := pc.DarDeBaja(s.ahora()); errBaja != nil {
		if !errors.Is(errBaja, domain.ErrPCYaDadaDeBaja) {
			return nil, errBaja
		}
		pendiente, err := s.cascadaPendiente(ctx, pcID)
		if err != nil {
			return nil, err
		}
		if !pendiente {
			return nil, errBaja
		}
	} else if err := s.repo.GuardarPC(ctx, pc); err != nil {
		return nil, err
	}

	// Minúscula y sin prefijo: esto se lee después de "Tu reserva fue
	// cancelada: " (ver motivoPorDefecto).
	motivo := fmt.Sprintf("la PC %d fue dada de baja del inventario", pc.Identificador)
	canceladas, notificados, err := s.validadorReservas.CancelarReservasFuturasDePC(ctx, pcID, motivo)
	if err != nil {
		return nil, errCascada(err)
	}

	return &ResultadoCascada{ReservasCanceladas: canceladas, DocentesNotificados: notificados}, nil
}

// motivoPorDefecto arma la RAZÓN de la cancelación, no el aviso completo:
// el "Tu reserva fue cancelada:" lo antepone el suscriptor de notification
// (ver internal/notification/application/subscribers.go). Nombra la PC
// porque el docente recibe un aviso por cada una y sin el identificador no
// puede saber cuál se le cayó (RF-05.3).
func motivoPorDefecto(pc *domain.PC, nuevo domain.EstadoPC, motivo *string) string {
	if motivo != nil && *motivo != "" {
		return *motivo
	}
	return fmt.Sprintf("la PC %d pasó a %s", pc.Identificador, nuevo)
}

func (s *Service) ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error) {
	return s.repo.ListarPCsPorCarro(ctx, carroID)
}

// ── Incidencia ──────────────────────────────────────────────────────────

func (s *Service) CrearIncidencia(ctx context.Context, pcID, reportadoPor, descripcion string, gravedad domain.Gravedad) (*domain.Incidencia, error) {
	i, err := domain.NuevaIncidencia(s.nuevoID(), pcID, reportadoPor, descripcion, gravedad, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearIncidencia(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

// EditarIncidenciaParams — nil significa "no tocar ese campo".
type EditarIncidenciaParams struct {
	Estado           *domain.EstadoIncidencia
	MarcarEnviadaDGE bool
}

func (s *Service) EditarIncidencia(ctx context.Context, incidenciaID string, params EditarIncidenciaParams) error {
	i, err := s.repo.BuscarIncidenciaPorID(ctx, incidenciaID)
	if err != nil {
		return err
	}

	if params.MarcarEnviadaDGE {
		i.MarcarEnviadaDGE(s.ahora())
	} else if params.Estado != nil {
		i.Estado = *params.Estado
	}

	return s.repo.GuardarIncidencia(ctx, i)
}

func (s *Service) ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error) {
	return s.repo.ListarIncidenciasPorPC(ctx, pcID)
}
