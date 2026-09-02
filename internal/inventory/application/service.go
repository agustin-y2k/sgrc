// Package application orquesta los casos de uso de RF-03 (inventario:
// carros, PCs, incidencias).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/secretos"
)

type Service struct {
	repo              Repo
	validadorReservas ValidadorReservas
	nuevoID           IDGenerator
	ahora             func() time.Time
	// cifrador guarda y recupera las contraseñas de las cuentas de cada equipo
	// (RF-03.22). Puede ser nil: el despliegue que no configuró CUENTAS_SECRET
	// registra cuentas igual, solo que sin contraseñas. Todos sus métodos
	// toleran el nil y responden ErrSinClave.
	cifrador *secretos.Cifrador
	// bus publica que una licencia dejó de estar pendiente, para que el aviso
	// de la campana se cierre solo cuando ya no queda ninguna por renovar.
	bus eventbus.EventBus
}

func NewService(repo Repo, validadorReservas ValidadorReservas, nuevoID IDGenerator, ahora func() time.Time, cifrador *secretos.Cifrador, bus eventbus.EventBus) *Service {
	return &Service{repo: repo, validadorReservas: validadorReservas, nuevoID: nuevoID,
		ahora: ahora, cifrador: cifrador, bus: bus}
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

func (s *Service) CrearEquipoDeCarro(ctx context.Context, carroID string, identificador int, numeroSerie string, freezado bool, cpu, ram, sistemaOperativo, softwareInstalado string) (*domain.Equipo, error) {
	pc, err := domain.NuevoEquipoDeCarro(s.nuevoID(), carroID, identificador, numeroSerie, freezado, s.ahora())
	if err != nil {
		return nil, err
	}
	pc.CPU = cpu
	pc.RAM = ram
	pc.SistemaOperativo = sistemaOperativo
	pc.SoftwareInstalado = softwareInstalado

	if err := s.repo.CrearEquipo(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// EditarEquipoParams agrupa los campos editables de una PC — todos punteros,
// nil significa "no tocar ese campo" (RF-03.4, RF-03.10 para CarroID).
type EditarEquipoParams struct {
	CarroID           *string
	Freezado          *bool
	CPU               *string
	RAM               *string
	SistemaOperativo  *string
	SoftwareInstalado *string
	// Los tres de un equipo suelto. Tipo y Nombre solo tienen sentido en un equipo
	// suelto; Reservable, en cualquiera.
	Tipo       *string
	Nombre     *string
	Reservable *bool
	// EsComputadora: corregir de qué se trata el equipo. Desmarcarlo NO borra
	// la ficha técnica que ya tenía —deja de mostrarse y vuelve intacta si se
	// vuelve a marcar—, porque lo normal es que se esté corrigiendo un error de
	// carga y perder lo escrito por una casilla mal tildada es peor que
	// guardar un dato que nadie mira.
	EsComputadora *bool
	// NumeroSerie se edita porque los equipos que ya estaban cargados no lo
	// tienen: sin esto habría que dar de baja la notebook y volver a crearla
	// solo para anotarle la serie, perdiendo su historial.
	NumeroSerie *string
}

func (s *Service) EditarEquipo(ctx context.Context, equipoID string, params EditarEquipoParams) error {
	pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
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
	if params.Tipo != nil {
		tipo, err := domain.TipoDeEquipoValido(*params.Tipo)
		if err != nil {
			return err
		}
		pc.Tipo = tipo
	}
	if params.Nombre != nil {
		nombre, err := domain.NombreDeEquipoValido(*params.Nombre)
		// Un equipo suelto no puede quedarse sin nombre: es lo único que lo
		// distingue, y el índice `ux_equipo_suelto_nombre` lo exige en la base.
		if err != nil && (*params.Nombre != "" || !pc.EstaEnUnCarro()) {
			return err
		}
		pc.Nombre = nombre
	}
	if params.Reservable != nil {
		pc.Reservable = *params.Reservable
	}
	if params.EsComputadora != nil {
		pc.EsComputadora = *params.EsComputadora
	}
	if params.NumeroSerie != nil {
		serie, err := domain.NumeroSerieOpcionalValido(*params.NumeroSerie)
		if err != nil {
			return err
		}
		// Vaciarlo solo se permite fuera de un carro. Una computadora de
		// laboratorio nace con serie obligatoria, y dejar que una edición se la
		// saque abriría por la puerta de atrás un estado que el alta prohíbe.
		if serie == "" && pc.EstaEnUnCarro() {
			return domain.ErrNumeroSerieInvalido
		}
		pc.NumeroSerie = serie
	}

	return s.repo.GuardarEquipo(ctx, pc)
}

// ResultadoCascada es lo que el handler HTTP necesita para armar la respuesta
// de RF-03.8/03.9 (cuántas reservas se cancelaron, a cuántos docentes se
// notificó).
type ResultadoCascada struct {
	ReservasCanceladas  int
	DocentesNotificados int
}

// disparaCascada dice si ese estado saca a la PC de circulación y, por lo
// tanto, obliga a cancelar sus reservas futuras (RF-03.8).
func disparaCascada(estado domain.EstadoEquipo) bool {
	return estado == domain.EstadoEnMantenimiento || estado == domain.EstadoFueraDeServicio
}

// cascadaPendiente distingue "esta operación ya se hizo" de "esta operación
// se hizo a medias y hay que terminarla".
func (s *Service) cascadaPendiente(ctx context.Context, equipoID string) (bool, error) {
	pendiente, err := s.validadorReservas.TieneReservasFuturas(ctx, equipoID)
	if err != nil {
		return false, fmt.Errorf("verificando si quedó una cascada pendiente sobre el equipo: %w", err)
	}
	return pendiente, nil
}

// errCascada envuelve el fallo del segundo paso dejando dicho que el primero
// SÍ se aplicó y que reintentar la misma operación completa lo que falta —
// mismo criterio que el error de ArchivarYClonar en academic.
func errCascada(err error) error {
	return fmt.Errorf("el equipo quedó guardado en su nuevo estado pero no se pudieron cancelar sus reservas futuras "+
		"(reintentar la misma operación completa la cascada): %w", err)
}

// CambiarEstadoEquipo implementa RF-03.8: al pasar a EN_MANTENIMIENTO o
// FUERA_DE_SERVICIO, cancela en cascada las reservas futuras de esa PC.
func (s *Service) CambiarEstadoEquipo(ctx context.Context, equipoID string, nuevo domain.EstadoEquipo, motivo *string) (*ResultadoCascada, error) {
	pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
	if err != nil {
		return nil, err
	}

	if errTransicion := pc.CambiarEstado(nuevo); errTransicion != nil {
		// Repetir la transición sigue siendo un error — salvo que la PC ya esté en
		// el estado pedido Y le queden reservas futuras vivas, que es la huella que
		// deja un intento anterior cortado entre el guardado y la cascada.
		if pc.Estado != nuevo || !disparaCascada(nuevo) {
			return nil, errTransicion
		}
		pendiente, err := s.cascadaPendiente(ctx, equipoID)
		if err != nil {
			return nil, err
		}
		if !pendiente {
			return nil, errTransicion
		}
	} else if err := s.repo.GuardarEquipo(ctx, pc); err != nil {
		return nil, err
	}

	resultado := &ResultadoCascada{}
	if disparaCascada(nuevo) {
		motivoTexto := motivoPorDefecto(nuevo, motivo)
		canceladas, notificados, err := s.validadorReservas.CancelarReservasFuturasDeEquipo(ctx, equipoID, motivoTexto)
		if err != nil {
			return nil, errCascada(err)
		}
		resultado.ReservasCanceladas = canceladas
		resultado.DocentesNotificados = notificados
	}

	return resultado, nil
}

// DarDeBajaEquipo implementa RF-03.4/03.9: soft delete + misma cascada de
// cancelación que CambiarEstadoEquipo (RF-03.9 dice explícitamente que dar de
// baja dispara la misma cascada que pasar a FUERA_DE_SERVICIO), incluido el
// mismo reintento cuando la cascada quedó a medias.
func (s *Service) DarDeBajaEquipo(ctx context.Context, equipoID string) (*ResultadoCascada, error) {
	pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
	if err != nil {
		return nil, err
	}

	// Solo cuando se está dando de baja de verdad.
	if !pc.DadoDeBaja {
		prestado, err := s.validadorReservas.EstaPrestado(ctx, equipoID)
		if err != nil {
			return nil, fmt.Errorf("verificando si el equipo está prestado: %w", err)
		}
		if prestado {
			return nil, ErrEquipoPrestado
		}
	}

	if errBaja := pc.DarDeBaja(s.ahora()); errBaja != nil {
		if !errors.Is(errBaja, domain.ErrEquipoYaDadoDeBaja) {
			return nil, errBaja
		}
		pendiente, err := s.cascadaPendiente(ctx, equipoID)
		if err != nil {
			return nil, err
		}
		if !pendiente {
			return nil, errBaja
		}
	} else if err := s.repo.GuardarEquipo(ctx, pc); err != nil {
		return nil, err
	}

	// Minúscula y sin prefijo: esto se lee después de "Tu reserva fue cancelada:
	// " (ver motivoPorDefecto).
	motivo := "el equipo fue dado de baja del inventario"
	canceladas, notificados, err := s.validadorReservas.CancelarReservasFuturasDeEquipo(ctx, equipoID, motivo)
	if err != nil {
		return nil, errCascada(err)
	}

	return &ResultadoCascada{ReservasCanceladas: canceladas, DocentesNotificados: notificados}, nil
}

// motivoPorDefecto arma la RAZÓN de la cancelación, no el aviso completo: el
// "Tu reserva fue cancelada:" lo antepone el suscriptor de notification (ver
// internal/notification/application/subscribers.go).
//
// NO nombra el equipo, y esa es la regla que comparte con los otros dos
// motivos del sistema ("la escuela cambió su horario de apertura…" en
// availability, "Se quitó al único docente…" en academic). Quien arma el aviso
// ya nombró la máquina —las tres formas del mensaje lo hacen, y el correo la
// lista arriba del renglón "Motivo:"—, así que nombrarla de nuevo acá salía
// como "Tu reserva del 28/08 (PC 7 del Carro 1) fue cancelada: PC 7 del Carro
// 1 pasó a FUERA_DE_SERVICIO".
//
// Dice "el equipo" y no arranca directo en el verbo para que se lea igual de
// bien solo, que es como aparece en el correo. Y "quedó" y no "pasó a" por el
// estado del medio: "pasó a en mantenimiento" no se puede leer, "quedó en
// mantenimiento" sí, y la misma frase sirve para los tres.
func motivoPorDefecto(nuevo domain.EstadoEquipo, motivo *string) string {
	if motivo != nil && *motivo != "" {
		return *motivo
	}
	return fmt.Sprintf("el equipo quedó %s", nuevo.Legible())
}

func (s *Service) ListarEquiposPorCarro(ctx context.Context, carroID string) ([]*domain.Equipo, error) {
	return s.repo.ListarEquiposPorCarro(ctx, carroID)
}

// ── Equipos que no están en ningún carro (RF-03.15) ─────────────────────

// CrearEquipo da de alta algo prestable que no es una computadora de un
// carro: un proyector, un cargador, una notebook suelta.
// CrearEquipoSueltoParams son los datos del alta de algo prestable que no está
// en un carro (RF-03.15). Es un struct y no una lista de argumentos porque los
// cinco últimos son la ficha técnica, y son cinco justamente para no tener que
// elegir cuáles de los datos de una máquina merecen anotarse.
//
// La ficha se completa cuando EsComputadora: la pantalla no la ofrece para un
// cargador. El servidor no la descarta si igual llega —guardar un dato que
// nadie va a mirar es más barato que una regla escondida que borra lo que
// alguien escribió—, simplemente no se muestra.
type CrearEquipoSueltoParams struct {
	Tipo        string
	Nombre      string
	NumeroSerie string
	Reservable  bool

	EsComputadora     bool
	Freezado          bool
	CPU               string
	RAM               string
	SistemaOperativo  string
	SoftwareInstalado string
}

func (s *Service) CrearEquipo(ctx context.Context, params CrearEquipoSueltoParams) (*domain.Equipo, error) {
	equipo, err := domain.NuevoEquipoSuelto(s.nuevoID(), params.Tipo, params.Nombre, params.NumeroSerie,
		params.Reservable, s.ahora())
	if err != nil {
		return nil, err
	}
	equipo.EsComputadora = params.EsComputadora
	equipo.Freezado = params.Freezado
	equipo.CPU = params.CPU
	equipo.RAM = params.RAM
	equipo.SistemaOperativo = params.SistemaOperativo
	equipo.SoftwareInstalado = params.SoftwareInstalado

	if err := s.repo.CrearEquipo(ctx, equipo); err != nil {
		return nil, err
	}
	return equipo, nil
}

func (s *Service) ListarEquipos(ctx context.Context, soloSueltos bool) ([]*domain.Equipo, error) {
	return s.repo.ListarEquipos(ctx, soloSueltos)
}

// ── Incidencia ──────────────────────────────────────────────────────────

func (s *Service) CrearIncidencia(ctx context.Context, equipoID, reportadoPor, descripcion, categoria string, gravedad domain.Gravedad) (*domain.Incidencia, error) {
	i, err := domain.NuevaIncidencia(s.nuevoID(), equipoID, reportadoPor, descripcion, categoria, gravedad, s.ahora())
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
	Estado                *domain.EstadoIncidencia
	MarcarEnviadaASoporte bool
	// Categoria se puede completar DESPUÉS, y ese es su caso principal: la falla
	// se reporta el día que aparece —"no enciende"— y el diagnóstico llega
	// cuando alguien pudo abrirla.
	Categoria *string
}

func (s *Service) EditarIncidencia(ctx context.Context, incidenciaID string, params EditarIncidenciaParams) error {
	i, err := s.repo.BuscarIncidenciaPorID(ctx, incidenciaID)
	if err != nil {
		return err
	}

	if params.MarcarEnviadaASoporte {
		i.MarcarEnviadaASoporte(s.ahora())
	} else if params.Estado != nil {
		i.Estado = *params.Estado
	}

	if params.Categoria != nil {
		categoria, err := domain.CategoriaDeFallaValida(*params.Categoria)
		if err != nil {
			return err
		}
		i.Categoria = categoria
	}

	return s.repo.GuardarIncidencia(ctx, i)
}

func (s *Service) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error) {
	return s.repo.ListarIncidenciasPorEquipo(ctx, equipoID)
}

func (s *Service) CategoriasDeFallaUsadas(ctx context.Context) ([]string, error) {
	return s.repo.CategoriasDeFallaUsadas(ctx)
}
