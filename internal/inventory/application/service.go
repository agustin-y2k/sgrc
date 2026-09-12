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

// DarDeBajaCarro retira un carro de circulación (RF-03.1).
//
// Es una baja LÓGICA: el nombre del carro vive congelado en el histórico de uso
// de cada equipo que tuvo adentro, y borrarlo dejaría ese histórico hablando de
// algo que el sistema ya no puede explicar. Lo que sí se libera es el nombre,
// para que el carro que lo reemplaza pueda llamarse igual.
//
// Sólo se permite con el carro VACÍO. Dar de baja uno con máquinas adentro las
// dejaría en un contenedor que para el sistema ya no existe: no aparecerían en
// ningún selector de carro y sólo se las podría encontrar buscándolas sueltas.
// Moverlas o darlas de baja es una decisión de quien conoce dónde fueron a
// parar, no algo que se pueda deducir acá.
func (s *Service) DarDeBajaCarro(ctx context.Context, carroID string) error {
	c, err := s.repo.BuscarCarroPorID(ctx, carroID)
	if err != nil {
		return err
	}

	equipos, err := s.repo.ListarEquiposPorCarro(ctx, carroID)
	if err != nil {
		return fmt.Errorf("verificando si el carro está vacío: %w", err)
	}
	for _, e := range equipos {
		if !e.DadoDeBaja {
			return ErrCarroConEquipos
		}
	}

	if err := c.DarDeBaja(s.ahora()); err != nil {
		return err
	}
	return s.repo.GuardarCarro(ctx, c)
}

// ReactivarCarro deshace la baja de un carro.
//
// Lo único que puede impedirlo es que su nombre ya no esté libre: el índice
// único excluye a los retirados, así que mientras el carro estuvo afuera otro
// pudo haberse quedado con «Carro 1». No lo comprueba este servicio sino la
// base, y el error que devuelve es el mismo que da el alta —que es justo lo que
// el Admin necesita leer: no es que no se pueda reactivar, es que el nombre está
// ocupado y hay que renombrar a uno de los dos.
func (s *Service) ReactivarCarro(ctx context.Context, carroID string) error {
	c, err := s.repo.BuscarCarroPorID(ctx, carroID)
	if err != nil {
		return err
	}
	if err := c.Reactivar(); err != nil {
		return err
	}
	return s.repo.GuardarCarro(ctx, c)
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
		c.CambiarDescripcion(*descripcion)
	}
	return s.repo.GuardarCarro(ctx, c)
}

func (s *Service) ListarCarros(ctx context.Context, incluirRetirados bool) ([]*domain.Carro, error) {
	return s.repo.ListarCarros(ctx, incluirRetirados)
}

// ── PC ──────────────────────────────────────────────────────────────────

// verificarCarroDisponible falla si el carro no existe o está dado de baja.
//
// Lo segundo NO lo cubre la clave foránea —un carro retirado sigue siendo una
// fila válida— y es justo el caso que deja una máquina invisible: los listados
// de carros traen sólo los vivos, así que el equipo queda adentro de algo que
// ninguna pantalla muestra, sin estar él mismo dado de baja.
func (s *Service) verificarCarroDisponible(ctx context.Context, carroID string) error {
	c, err := s.repo.BuscarCarroPorID(ctx, carroID)
	if err != nil {
		return err
	}
	if c.DadoDeBaja {
		return ErrCarroDadoDeBaja
	}
	return nil
}

func (s *Service) CrearEquipoDeCarro(ctx context.Context, carroID string, identificador int, numeroSerie string, freezado bool, cpu, ram, sistemaOperativo, softwareInstalado string) (*domain.Equipo, error) {
	if err := s.verificarCarroDisponible(ctx, carroID); err != nil {
		return nil, err
	}

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

// CambioDeCampo es un dato que cambió, con sus dos puntas. La auditoría
// necesita las dos: sin el valor anterior, «alguien editó el nombre» no
// contesta la pregunta que se hace meses después, que es qué decía antes.
type CambioDeCampo struct {
	Antes   any `json:"antes"`
	Despues any `json:"despues"`
}

// cambiosDeEquipo compara dos fotos del mismo equipo y devuelve sólo lo que se
// movió.
//
// Se hace por comparación y no anotando en cada rama de la edición a propósito:
// las ramas son once y crecen, y la que alguien agregue mañana entra sola en la
// auditoría en vez de quedarse afuera en silencio — que es exactamente cómo
// llegamos a que sólo el cambio de carro dejara rastro.
//
// `carroId` queda afuera porque tiene su propia acción, EQUIPO_MOVIDO_DE_CARRO:
// registrarlo en las dos sería contar el mismo movimiento dos veces.
func cambiosDeEquipo(antes, despues domain.Equipo) map[string]CambioDeCampo {
	campos := []struct {
		nombre         string
		antes, despues any
	}{
		{"identificador", antes.Identificador, despues.Identificador},
		{"tipo", antes.Tipo, despues.Tipo},
		{"nombre", antes.Nombre, despues.Nombre},
		{"numeroSerie", antes.NumeroSerie, despues.NumeroSerie},
		{"reservable", antes.Reservable, despues.Reservable},
		{"esComputadora", antes.EsComputadora, despues.EsComputadora},
		{"freezado", antes.Freezado, despues.Freezado},
		{"cpu", antes.CPU, despues.CPU},
		{"ram", antes.RAM, despues.RAM},
		{"sistemaOperativo", antes.SistemaOperativo, despues.SistemaOperativo},
		{"softwareInstalado", antes.SoftwareInstalado, despues.SoftwareInstalado},
	}

	cambios := map[string]CambioDeCampo{}
	for _, c := range campos {
		if c.antes != c.despues {
			cambios[c.nombre] = CambioDeCampo{Antes: c.antes, Despues: c.despues}
		}
	}
	return cambios
}

// EditarEquipo devuelve qué cambió, para que el handler lo audite. Un mapa
// vacío significa que el pedido no movió nada — y entonces no se audita: una
// entrada que dice «editó» sin decir qué es ruido en el registro.
func (s *Service) EditarEquipo(ctx context.Context, equipoID string, params EditarEquipoParams) (map[string]CambioDeCampo, error) {
	pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
	if err != nil {
		return nil, err
	}

	// La foto de antes. Se copia por valor: los campos que se comparan son
	// todos escalares.
	antes := *pc

	if params.CarroID != nil {
		// El destino se valida ANTES de tocar el equipo: mover a un carro
		// retirado devolvía 200 y dejaba la máquina en un contenedor que ninguna
		// pantalla lista.
		if err := s.verificarCarroDisponible(ctx, *params.CarroID); err != nil {
			return nil, err
		}
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
			return nil, err
		}
		pc.Tipo = tipo
	}
	if params.Nombre != nil {
		nombre, err := domain.NombreDeEquipoValido(*params.Nombre)
		// Un equipo suelto no puede quedarse sin nombre: es lo único que lo
		// distingue, y el índice `ux_equipo_suelto_nombre` lo exige en la base.
		if err != nil && (*params.Nombre != "" || !pc.EstaEnUnCarro()) {
			return nil, err
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
			return nil, err
		}
		// Vaciarlo solo se permite fuera de un carro. Una computadora de
		// laboratorio nace con serie obligatoria, y dejar que una edición se la
		// saque abriría por la puerta de atrás un estado que el alta prohíbe.
		if serie == "" && pc.EstaEnUnCarro() {
			return nil, domain.ErrNumeroSerieInvalido
		}
		pc.NumeroSerie = serie
	}

	if err := s.repo.GuardarEquipo(ctx, pc); err != nil {
		return nil, err
	}
	return cambiosDeEquipo(antes, *pc), nil
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

// ReactivarEquipo deshace la baja de un equipo (RF-03.4).
//
// Tres cosas pueden impedirlo, y las tres son "alguien se quedó con lo tuyo
// mientras no estabas", porque los índices únicos que sostienen esos tres datos
// excluyen a los dados de baja:
//
//   - su zócalo en el carro (PC 7 del Carro 1 ahora es otra máquina),
//   - su nombre, si es un equipo suelto,
//   - su número de serie.
//
// Ninguna la comprueba este servicio: las decide la base y el repositorio
// traduce cuál de las tres fue, que es lo que hay que decirle al Admin para que
// sepa qué renombrar.
//
// La que sí va acá es la cuarta, porque la base no la puede ver: el carro al que
// el equipo pertenece pudo haberse retirado en el medio. Reactivar ahí adentro
// devolvería la máquina a un contenedor que ninguna pantalla lista —es el mismo
// caso que verificarCarroDisponible cubre al crear y al mover—, y el equipo
// quedaría invisible sin estar dado de baja, que es peor que seguir de baja.
func (s *Service) ReactivarEquipo(ctx context.Context, equipoID string) error {
	pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
	if err != nil {
		return err
	}
	if pc.EstaEnUnCarro() {
		if err := s.verificarCarroDisponible(ctx, pc.CarroID); err != nil {
			return err
		}
	}
	if err := pc.Reactivar(); err != nil {
		return err
	}
	return s.repo.GuardarEquipo(ctx, pc)
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

// maxIncidenciasDeEquipo acota el historial de fallas de una máquina, con el
// mismo criterio y el mismo número que el de entregas (maxHistorialDeEquipo en
// reservation): son las dos la misma pantalla —los últimos movimientos de un
// equipo— y la decisión estaba tomada para una sola de las dos.
const maxIncidenciasDeEquipo = 50

func (s *Service) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error) {
	return s.repo.ListarIncidenciasPorEquipo(ctx, equipoID, maxIncidenciasDeEquipo)
}

func (s *Service) CategoriasDeFallaUsadas(ctx context.Context) ([]string, error) {
	return s.repo.CategoriasDeFallaUsadas(ctx)
}

// ── Obtener uno solo ────────────────────────────────────────────────────
//
// Cada uno de estos recursos se podía editar y dar de baja, pero no PEDIR: la
// API tenía PATCH y DELETE de /equipos/{id} y ningún GET. Funcionaba porque la
// pantalla trae todo de los listados y se guarda el objeto en memoria;
// cualquier otro consumidor —un script, una integración, la propia pantalla
// después de recargar en una dirección profunda— tenía que traerse la colección
// entera para encontrar uno.
//
// El permiso de cada uno es el mismo que el de su listado: si ya se podía ver
// en la lista, se puede ver solo.

func (s *Service) ObtenerCarro(ctx context.Context, id string) (*domain.Carro, error) {
	return s.repo.BuscarCarroPorID(ctx, id)
}

func (s *Service) ObtenerEquipo(ctx context.Context, id string) (*domain.Equipo, error) {
	return s.repo.BuscarEquipoPorID(ctx, id)
}

func (s *Service) ObtenerIncidencia(ctx context.Context, id string) (*domain.Incidencia, error) {
	return s.repo.BuscarIncidenciaPorID(ctx, id)
}
