// Package application orquesta los casos de uso de RF-07 (disponibilidad
// de Admins). Ver ports.go para los puertos hacia infrastructure/ y auth.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/availability/domain"
)

type Service struct {
	repo           Repo
	listadorAdmins ListadorAdmins
	nuevoID        IDGenerator
	ahora          func() time.Time
}

func NewService(repo Repo, listadorAdmins ListadorAdmins, nuevoID IDGenerator, ahora func() time.Time) *Service {
	return &Service{repo: repo, listadorAdmins: listadorAdmins, nuevoID: nuevoID, ahora: ahora}
}

// MiHorario devuelve el patrón semanal propio del Admin autenticado
// (GET /mi-horario).
func (s *Service) MiHorario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error) {
	return s.repo.ListarBloquesDeUsuario(ctx, usuarioID)
}

// AgregarBloque (RF-07.1/07.3) — aplica de inmediato para todas las
// semanas futuras, sin ninguna acción extra de "propagar" (no hay nada
// materializado que actualizar, es un patrón evaluado en el momento de
// cada consulta).
func (s *Service) AgregarBloque(ctx context.Context, usuarioID string, dia domain.DiaSemana, horaInicio, horaFin time.Duration) (*domain.BloqueHorario, error) {
	b, err := domain.NuevoBloqueHorario(s.nuevoID(), usuarioID, dia, horaInicio, horaFin)
	if err != nil {
		return nil, err
	}
	if err := s.verificarSinSolape(ctx, b); err != nil {
		return nil, err
	}
	if err := s.repo.CrearBloque(ctx, b); err != nil {
		return nil, fmt.Errorf("creando bloque de horario: %w", err)
	}
	return b, nil
}

// verificarSinSolape rechaza un bloque que pise a otro del mismo día.
//
// La base no puede garantizarlo sola —haría falta una constraint EXCLUDE con
// btree_gist sobre un rango de TIME, que es bastante maquinaria para algo
// puramente informativo—, así que la regla vive acá. Es una lectura por
// usuario: son unos pocos bloques por persona.
//
// El error nombra el bloque que estorba. Decir "se pisa con otro" y no cuál
// obliga a mirar la lista y compararlos a mano, que es justo lo que la
// pantalla no ayuda a hacer cuando hay renglones parecidos.
func (s *Service) verificarSinSolape(ctx context.Context, b *domain.BloqueHorario) error {
	existentes, err := s.repo.ListarBloquesDeUsuario(ctx, b.UsuarioID)
	if err != nil {
		return fmt.Errorf("verificando si el horario se pisa con otro: %w", err)
	}
	if choca := b.PrimeroQueSeSolapa(existentes); choca != nil {
		return fmt.Errorf("%w: %s de %s a %s", domain.ErrBloqueSolapado,
			choca.DiaSemana, formatearHora(choca.HoraInicio), formatearHora(choca.HoraFin))
	}
	return nil
}

// formatearHora pasa una duración desde medianoche a "08:30", que es como la
// persona escribió el horario y como lo ve en pantalla.
func formatearHora(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// EditarBloque acepta campos opcionales (PATCH parcial) y revalida el
// rango horario resultante contra domain, aunque solo se edite un
// extremo. La titularidad se resuelve en BuscarBloqueDeUsuario, acotada
// por usuarioID — intentar editar el bloque de otro Admin da
// ErrBloqueNoEncontrado, igual que un ID inexistente.
func (s *Service) EditarBloque(ctx context.Context, id, usuarioID string, dia *domain.DiaSemana, horaInicio, horaFin *time.Duration) (*domain.BloqueHorario, error) {
	actual, err := s.repo.BuscarBloqueDeUsuario(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}

	nuevoDia := actual.DiaSemana
	if dia != nil {
		nuevoDia = *dia
	}
	nuevoInicio := actual.HoraInicio
	if horaInicio != nil {
		nuevoInicio = *horaInicio
	}
	nuevoFin := actual.HoraFin
	if horaFin != nil {
		nuevoFin = *horaFin
	}

	actualizado, err := domain.NuevoBloqueHorario(actual.ID, actual.UsuarioID, nuevoDia, nuevoInicio, nuevoFin)
	if err != nil {
		return nil, err
	}
	// Editar también puede crear un solape: mover el fin de las 12 a las 15
	// pisa el bloque de la tarde. PrimeroQueSeSolapa se ignora a sí mismo por
	// ID, así que guardar sin mover nada no choca con su propia versión.
	if err := s.verificarSinSolape(ctx, actualizado); err != nil {
		return nil, err
	}
	if err := s.repo.GuardarBloque(ctx, actualizado); err != nil {
		return nil, fmt.Errorf("actualizando bloque de horario: %w", err)
	}
	return actualizado, nil
}

// EliminarBloque (DELETE /mi-horario/{id}) — titularidad acotada en el
// repo, mismo criterio que EditarBloque.
func (s *Service) EliminarBloque(ctx context.Context, id, usuarioID string) error {
	return s.repo.EliminarBloqueDeUsuario(ctx, id, usuarioID)
}

// CargarExcepcion (RF-07.4) — reemplaza la excepción existente para esa
// fecha si ya había una (upsert en el repo, UNIQUE(usuario_id, fecha)).
func (s *Service) CargarExcepcion(ctx context.Context, usuarioID string, fecha time.Time, tipo domain.TipoExcepcion, horaInicio, horaFin *time.Duration, motivo *string) (*domain.Excepcion, error) {
	e, err := domain.NuevaExcepcion(s.nuevoID(), usuarioID, fecha, tipo, horaInicio, horaFin, motivo)
	if err != nil {
		return nil, err
	}
	if err := s.repo.GuardarExcepcion(ctx, e); err != nil {
		return nil, fmt.Errorf("guardando excepción: %w", err)
	}
	return e, nil
}

// MarcarNoDisponibleAhora (RF-07.5) — atajo de un solo paso, equivalente a
// cargar una excepción NO_DISPONIBLE para la fecha de hoy.
func (s *Service) MarcarNoDisponibleAhora(ctx context.Context, usuarioID string) (*domain.Excepcion, error) {
	hoy := domain.FechaSolo(s.ahora())
	return s.CargarExcepcion(ctx, usuarioID, hoy, domain.NoDisponible, nil, nil, nil)
}

// AdminDisponibilidad es el resultado combinado por Admin para RF-07.2 —
// vive en application/ porque combina datos de varias fuentes (auth vía
// ListadorAdmins + los propios repos de bloques/excepciones), no es una
// entidad persistida por sí misma.
type AdminDisponibilidad struct {
	UsuarioID       string
	Nombre          string
	Apellido        string
	DisponibleAhora bool
	ExcepcionHoy    *domain.Excepcion
	HorarioSemanal  []*domain.BloqueHorario
}

// DisponibilidadDeTodosLosAdmins (GET /admins, RF-07.2) — para cualquier
// usuario autenticado. disponibleAhora se calcula en el momento contra el
// horario semanal y la excepción de hoy de cada Admin (la excepción,
// cuando existe, siempre tiene prioridad — ver domain.DisponibleAhora).
func (s *Service) DisponibilidadDeTodosLosAdmins(ctx context.Context) ([]AdminDisponibilidad, error) {
	admins, err := s.listadorAdmins.AdminsAprobados(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando admins aprobados: %w", err)
	}

	ahora := s.ahora()
	diaActual, horaActual := domain.DiaYHoraDe(ahora)
	hoy := domain.FechaSolo(ahora)

	// Dos consultas en total, no dos por Admin: resolverlo dentro del for
	// serían 2N viajes a la base para armar una pantalla que mira cualquier
	// docente.
	ids := make([]string, len(admins))
	for i, admin := range admins {
		ids[i] = admin.ID
	}

	bloquesPorAdmin, err := s.repo.ListarBloquesDeUsuarios(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listando horarios de los admins: %w", err)
	}
	excepcionesPorAdmin, err := s.repo.BuscarExcepcionesDeFecha(ctx, ids, hoy)
	if err != nil {
		return nil, fmt.Errorf("buscando las excepciones de hoy: %w", err)
	}

	resultado := make([]AdminDisponibilidad, 0, len(admins))
	for _, admin := range admins {
		// Faltar en el mapa es el caso normal: un Admin sin horario cargado
		// y sin excepción para hoy. domain.DisponibleAhora ya trata el nil
		// como "sin bloques" / "sin excepción".
		bloques := bloquesPorAdmin[admin.ID]
		excepcion := excepcionesPorAdmin[admin.ID]

		resultado = append(resultado, AdminDisponibilidad{
			UsuarioID:       admin.ID,
			Nombre:          admin.Nombre,
			Apellido:        admin.Apellido,
			DisponibleAhora: domain.DisponibleAhora(bloques, excepcion, diaActual, horaActual),
			ExcepcionHoy:    excepcion,
			HorarioSemanal:  bloques,
		})
	}
	return resultado, nil
}

// ═══════════════════════════════════════════════════════════════════════
// Jornada de la institución
// ═══════════════════════════════════════════════════════════════════════
//
// Vive en este paquete y no en uno propio porque es el mismo concepto que
// availability ya modela —días de la semana y tramos horarios— y porque
// comparte con él las conversiones de hora de pared. Lo que NO comparte es
// el dueño: el horario de un Admin es de esa persona, la jornada es de la
// escuela. Por eso ninguna de estas funciones recibe usuarioID; quién puede
// tocarla se resuelve en la ruta, que exige rol ADMIN.

// Jornada devuelve la jornada declarada, completa. Una lista vacía significa
// que la institución todavía no la declaró, y eso NO es lo mismo que una
// escuela cerrada: sin jornada declarada no hay restricción de horario.
func (s *Service) Jornada(ctx context.Context) ([]*domain.BloqueJornada, error) {
	return s.repo.ListarJornada(ctx)
}

// AgregarBloqueDeJornada suma un tramo abierto a un día. Varios por día es
// el caso previsto: turno mañana y turno noche, con el mediodía afuera.
func (s *Service) AgregarBloqueDeJornada(ctx context.Context, dia domain.DiaSemana, horaInicio, horaFin time.Duration) (*domain.BloqueJornada, error) {
	b, err := domain.NuevoBloqueJornada(s.nuevoID(), dia, horaInicio, horaFin)
	if err != nil {
		return nil, err
	}
	if err := s.verificarJornadaSinSolape(ctx, b); err != nil {
		return nil, err
	}
	if err := s.repo.CrearBloqueJornada(ctx, b); err != nil {
		return nil, fmt.Errorf("creando bloque de jornada: %w", err)
	}
	return b, nil
}

// EditarBloqueDeJornada acepta campos opcionales (PATCH parcial) y revalida
// el rango resultante aunque se edite un solo extremo, igual que el horario
// de los Admin.
func (s *Service) EditarBloqueDeJornada(ctx context.Context, id string, dia *domain.DiaSemana, horaInicio, horaFin *time.Duration) (*domain.BloqueJornada, error) {
	actual, err := s.repo.BuscarBloqueJornada(ctx, id)
	if err != nil {
		return nil, err
	}

	nuevo := *actual
	if dia != nil {
		nuevo.DiaSemana = *dia
	}
	if horaInicio != nil {
		nuevo.HoraInicio = *horaInicio
	}
	if horaFin != nil {
		nuevo.HoraFin = *horaFin
	}
	// Solo iguales es inválido, igual que en NuevoBloqueJornada: en la
	// jornada, hora_fin menor que hora_inicio significa que el tramo termina
	// al día siguiente. Acá decía `<=`, copiado del horario de los Admin —que
	// no cruza la medianoche a propósito—, y eso dejaba crear el tramo de una
	// nocturna (20:00–01:00) pero no editarlo nunca más.
	if nuevo.HoraFin == nuevo.HoraInicio {
		return nil, domain.ErrRangoHorarioInvalido
	}
	if err := s.verificarJornadaSinSolape(ctx, &nuevo); err != nil {
		return nil, err
	}

	if err := s.repo.GuardarBloqueJornada(ctx, &nuevo); err != nil {
		return nil, fmt.Errorf("guardando bloque de jornada: %w", err)
	}
	return &nuevo, nil
}

func (s *Service) EliminarBloqueDeJornada(ctx context.Context, id string) error {
	return s.repo.EliminarBloqueJornada(ctx, id)
}

// verificarJornadaSinSolape rechaza un tramo que pise a otro del mismo día,
// nombrando cuál. Mismo criterio y mismo motivo que verificarSinSolape.
//
// Se excluye el propio bloque de la comparación: al editar, el tramo se pisa
// consigo mismo y sin esto no habría forma de mover un extremo.
func (s *Service) verificarJornadaSinSolape(ctx context.Context, b *domain.BloqueJornada) error {
	existentes, err := s.repo.ListarJornada(ctx)
	if err != nil {
		return fmt.Errorf("verificando si el tramo se pisa con otro: %w", err)
	}
	for _, otro := range existentes {
		if otro.ID == b.ID {
			continue
		}
		if b.SolapaCon(otro) {
			return fmt.Errorf("%w: %s de %s a %s", domain.ErrBloqueJornadaSolapado,
				otro.DiaSemana, formatearHora(otro.HoraInicio), formatearHora(otro.HoraFin))
		}
	}
	return nil
}

// PermiteReserva responde si un bloque cae dentro de la jornada declarada.
// Lo consume reservation a través de un puerto (ver cmd/wiring_adapters.go):
// availability no sabe que existen las reservas.
//
// `fecha` llega como el DATE de la reserva —medianoche, sin zona— y de ahí
// sale el día de la semana. Las horas van aparte, como duraciones desde
// medianoche, que es como las guarda la columna TIME.
func (s *Service) PermiteReserva(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) (bool, error) {
	jornada, err := s.repo.ListarJornada(ctx)
	if err != nil {
		return false, fmt.Errorf("leyendo la jornada de la institución: %w", err)
	}
	dia, _ := domain.DiaYHoraDe(fecha)
	return domain.PermiteReserva(jornada, dia, horaInicio, horaFin), nil
}
