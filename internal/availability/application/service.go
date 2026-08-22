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
	reservas       ReservasDeLaInstitucion
	nuevoID        IDGenerator
	ahora          func() time.Time
}

func NewService(repo Repo, listadorAdmins ListadorAdmins, reservas ReservasDeLaInstitucion, nuevoID IDGenerator, ahora func() time.Time) *Service {
	return &Service{
		repo: repo, listadorAdmins: listadorAdmins, reservas: reservas,
		nuevoID: nuevoID, ahora: ahora,
	}
}

// MiHorario devuelve el patrón semanal propio del Admin autenticado
// (GET /mi-horario).
func (s *Service) MiHorario(ctx context.Context, usuarioID string) ([]*domain.BloqueHorario, error) {
	return s.repo.ListarBloquesDeUsuario(ctx, usuarioID)
}

// AgregarBloque (RF-07.1/07.3) — aplica de inmediato para todas las semanas
// futuras, sin ninguna acción extra de "propagar" (no hay nada materializado
// que actualizar, es un patrón evaluado en el momento de cada consulta).
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

// EditarBloque acepta campos opcionales (PATCH parcial) y revalida el rango
// horario resultante contra domain, aunque solo se edite un extremo.
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
	// Editar también puede crear un solape: mover el fin de las 12 a las 15 pisa
	// el bloque de la tarde.
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

// AdminDisponibilidad es el resultado combinado por Admin para RF-07.2 — vive
// en application/ porque combina datos de varias fuentes (auth vía
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
// usuario autenticado.
func (s *Service) DisponibilidadDeTodosLosAdmins(ctx context.Context) ([]AdminDisponibilidad, error) {
	admins, err := s.listadorAdmins.AdminsAprobados(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando admins aprobados: %w", err)
	}

	ahora := s.ahora()
	diaActual, horaActual := domain.DiaYHoraDe(ahora)
	hoy := domain.FechaSolo(ahora)

	// Dos consultas en total, no dos por Admin: resolverlo dentro del for serían
	// 2N viajes a la base para armar una pantalla que mira cualquier docente.
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
		// Faltar en el mapa es el caso normal: un Admin sin horario cargado y sin
		// excepción para hoy.
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
// Vive en este paquete y no en uno propio porque es el mismo concepto que
// availability ya modela —días de la semana y tramos horarios— y porque
// comparte con él las conversiones de hora de pared.

// Jornada devuelve la jornada declarada, completa.
func (s *Service) Jornada(ctx context.Context) ([]*domain.BloqueJornada, error) {
	return s.repo.ListarJornada(ctx)
}

// JornadaDefinida dice si la institución ya decidió su jornada. Una lista de
// tramos vacía no alcanza para saberlo: puede ser que todavía no la declaren
// o que hayan elegido no restringir nada, y a una hay que preguntarle y a la
// otra no.
func (s *Service) JornadaDefinida(ctx context.Context) (bool, error) {
	return s.repo.JornadaDefinida(ctx)
}

// TramoDeJornada es un tramo pedido, todavía sin ID: los IDs los pone el
// servicio porque la jornada se reemplaza entera y las filas viejas se van.
type TramoDeJornada struct {
	DiaSemana  domain.DiaSemana
	HoraInicio time.Duration
	HoraFin    time.Duration
}

// MotivoCambioDeJornada llega TAL CUAL al correo del docente, después de "Tu
// reserva fue cancelada: " que antepone notification. Por eso arranca en
// minúscula y explica la causa en vez de nombrar una categoría: quien lo lee
// no sabe qué es una jornada institucional, sabe que perdió su clase.
const MotivoCambioDeJornada = "la escuela cambió su horario de apertura y ese día y hora quedaron fuera"

// ImpactoDeJornada es lo que un cambio de jornada dejaría afuera.
type ImpactoDeJornada struct {
	// Reservas son las futuras que ya no entrarían. Se cuentan de a
	// ocurrencias y no de a series: una recurrencia de quince lunes son
	// quince, y decir "una serie" escondería el tamaño de lo que se cancela.
	Reservas []ReservaFutura
	// Prestamos son las máquinas que están afuera con la devolución pactada
	// fuera del horario nuevo. Van solo para que se vean: no se cancelan
	// nunca, porque el equipo está físicamente afuera y marcar el préstamo
	// como cancelado sería perder el rastro de quién lo tiene.
	Prestamos []PrestamoAbierto
	// TotalDeReservas es cuántas reservas futuras hay en total, afectadas o
	// no. Sirve para leer el número de arriba: veinte cancelaciones sobre
	// veinticinco reservas no es lo mismo que veinte sobre trescientas, y la
	// primera huele a error de carga.
	TotalDeReservas int
}

func (i *ImpactoDeJornada) HayAlgo() bool {
	return len(i.Reservas) > 0 || len(i.Prestamos) > 0
}

// ResultadoDeJornada es qué pasó al intentar cambiar la jornada.
type ResultadoDeJornada struct {
	// Bloques es la jornada que quedó; vacío si no se guardó nada.
	Bloques []*domain.BloqueJornada
	// Impacto es lo que quedaba (o quedó) afuera.
	Impacto *ImpactoDeJornada
	// Guardada en false significa que el cambio NO se aplicó porque hay
	// impacto y falta confirmarlo.
	Guardada           bool
	ReservasCanceladas int
}

// ReemplazarJornada deja la jornada exactamente igual a los tramos pedidos y
// marca que la institución ya decidió.
//
// Reemplazar entera —en vez de sumar, editar y borrar de a un tramo— es lo
// que hace que la jornada se pueda validar como el conjunto que es: los
// solapes se buscan sobre lo que va a quedar, no sobre un estado intermedio
// que depende del orden en que se hayan mandado los cambios.
//
// Una lista vacía es válida: es la institución diciendo que no quiere
// restringir días ni horarios. Queda igual que antes para reservar —sin
// tramos no hay restricción— pero ya no se le vuelve a preguntar.
func (s *Service) ReemplazarJornada(ctx context.Context, tramos []TramoDeJornada, confirmado bool) (*ResultadoDeJornada, error) {
	bloques := make([]*domain.BloqueJornada, 0, len(tramos))
	for _, t := range tramos {
		b, err := domain.NuevoBloqueJornada(s.nuevoID(), t.DiaSemana, t.HoraInicio, t.HoraFin)
		if err != nil {
			return nil, err
		}
		bloques = append(bloques, b)
	}

	if err := verificarJornadaSinSolape(bloques); err != nil {
		return nil, err
	}

	impacto, err := s.impactoDe(ctx, bloques)
	if err != nil {
		return nil, err
	}

	// Sin confirmar no se toca nada. El pedido sin confirmación ES la
	// previsualización: no hay un endpoint aparte que calcule el impacto,
	// justamente para que no pueda decir una cosa distinta de la que después
	// se aplica.
	if impacto.HayAlgo() && !confirmado {
		return &ResultadoDeJornada{Impacto: impacto}, nil
	}

	if err := s.repo.ReemplazarJornada(ctx, bloques); err != nil {
		return nil, fmt.Errorf("guardando la jornada: %w", err)
	}

	resultado := &ResultadoDeJornada{Bloques: bloques, Guardada: true, Impacto: impacto}

	// Se cancela DESPUÉS de guardar, y son dos transacciones distintas porque
	// viven en módulos distintos. El orden es el que deja el peor caso más
	// benigno: si esto falla, la jornada nueva quedó puesta y sobreviven unas
	// reservas fuera de horario —que es el estado que el sistema ya toleraba
	// antes de todo esto—. Al revés se cancelarían clases por un cambio que
	// después no se guarda.
	if len(impacto.Reservas) > 0 {
		ids := make([]string, len(impacto.Reservas))
		for i, r := range impacto.Reservas {
			ids[i] = r.ID
		}
		canceladas, err := s.reservas.CancelarReservas(ctx, ids, MotivoCambioDeJornada)
		if err != nil {
			return resultado, fmt.Errorf("%w: %w", ErrCascadaDeJornada, err)
		}
		resultado.ReservasCanceladas = canceladas
	}

	return resultado, nil
}

// impactoDe calcula qué queda afuera si la jornada pasa a ser `bloques`.
//
// Una jornada vacía no deja nada afuera: sin tramos declarados no hay
// restricción, así que ampliar hasta el extremo de no restringir nada nunca
// cancela una clase.
func (s *Service) impactoDe(ctx context.Context, bloques []*domain.BloqueJornada) (*ImpactoDeJornada, error) {
	impacto := &ImpactoDeJornada{}
	if len(bloques) == 0 {
		return impacto, nil
	}

	desde := s.ahora()
	reservas, err := s.reservas.ReservasFuturas(ctx, desde)
	if err != nil {
		return nil, fmt.Errorf("leyendo las reservas que podrían quedar fuera: %w", err)
	}
	impacto.TotalDeReservas = len(reservas)
	for _, r := range reservas {
		dia, _ := domain.DiaYHoraDe(r.Fecha)
		if !domain.PermiteReserva(bloques, dia, r.HoraInicio, r.HoraFin) {
			impacto.Reservas = append(impacto.Reservas, r)
		}
	}

	prestamos, err := s.reservas.PrestamosAbiertos(ctx)
	if err != nil {
		return nil, fmt.Errorf("leyendo los préstamos abiertos: %w", err)
	}
	for _, p := range prestamos {
		// Sin hora pactada no hay nada que comparar: ese préstamo no queda
		// "fuera de la jornada" porque nunca tuvo un momento previsto.
		if p.DevolucionEstimada == nil {
			continue
		}
		dia, hora := domain.DiaYHoraDe(*p.DevolucionEstimada)
		// La devolución es un instante y no un rango, así que se pregunta por
		// un tramo de duración cero: cae adentro o no cae.
		if !domain.PermiteReserva(bloques, dia, hora, hora) {
			impacto.Prestamos = append(impacto.Prestamos, p)
		}
	}

	return impacto, nil
}

// verificarJornadaSinSolape rechaza dos tramos del mismo día que se pisen,
// nombrando cuál. Compara todos contra todos sobre la jornada propuesta: no
// hace falta ir a la base porque lo que se guarda es exactamente esta lista.
func verificarJornadaSinSolape(bloques []*domain.BloqueJornada) error {
	for i, b := range bloques {
		for _, otro := range bloques[i+1:] {
			if b.SolapaCon(otro) {
				return fmt.Errorf("%w: %s de %s a %s", domain.ErrBloqueJornadaSolapado,
					otro.DiaSemana, formatearHora(otro.HoraInicio), formatearHora(otro.HoraFin))
			}
		}
	}
	return nil
}

// PermiteReserva responde si un bloque cae dentro de la jornada declarada.
func (s *Service) PermiteReserva(ctx context.Context, fecha time.Time, horaInicio, horaFin time.Duration) (bool, error) {
	jornada, err := s.repo.ListarJornada(ctx)
	if err != nil {
		return false, fmt.Errorf("leyendo la jornada de la institución: %w", err)
	}
	dia, _ := domain.DiaYHoraDe(fecha)
	return domain.PermiteReserva(jornada, dia, horaInicio, horaFin), nil
}
