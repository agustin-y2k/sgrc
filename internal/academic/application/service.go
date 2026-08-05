// Package application orquesta los casos de uso de RF-02 (ciclo lectivo,
// cursos, materias, docente_materia).
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Service implementa los casos de uso de academic. Depende de Repo,
// ValidadorUsuario (puerto hacia auth) y ValidadorReservas (puerto hacia
// reservation, stub por ahora) — nunca de esos paquetes directamente.
type Service struct {
	repo                Repo
	validadorUsuario    ValidadorUsuario
	validadorReservas   ValidadorReservas
	archivadorHistorico ArchivadorHistorico
	canceladorReservas  CanceladorReservasDeMateria
	nuevoID             IDGenerator
	bus                 eventbus.EventBus
}

func NewService(
	repo Repo,
	validadorUsuario ValidadorUsuario,
	validadorReservas ValidadorReservas,
	archivadorHistorico ArchivadorHistorico,
	canceladorReservas CanceladorReservasDeMateria,
	nuevoID IDGenerator,
	bus eventbus.EventBus,
) *Service {
	return &Service{
		repo:                repo,
		validadorUsuario:    validadorUsuario,
		validadorReservas:   validadorReservas,
		archivadorHistorico: archivadorHistorico,
		canceladorReservas:  canceladorReservas,
		nuevoID:             nuevoID,
		bus:                 bus,
	}
}

// ── Ciclo lectivo ───────────────────────────────────────────────────────

// CrearCiclo implementa RF-02.1: solo puede existir un ciclo activo a la
// vez. La constraint UNIQUE parcial de la base (idx_ciclo_lectivo_activo_unico)
// respalda esta misma regla como última línea de defensa ante una
// condición de carrera — igual que email único en auth.
func (s *Service) CrearCiclo(ctx context.Context, anio int) (*domain.CicloLectivo, error) {
	_, err := s.repo.BuscarCicloActivo(ctx)
	if err == nil {
		return nil, ErrYaHayCicloActivo
	}
	if !errors.Is(err, ErrCicloNoEncontrado) {
		return nil, fmt.Errorf("verificando ciclo activo: %w", err)
	}

	c, err := domain.NuevoCicloLectivo(s.nuevoID(), anio)
	if err != nil {
		return nil, err
	}

	if err := s.repo.CrearCiclo(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) ListarCiclos(ctx context.Context, filtroArchivado *bool) ([]*domain.CicloLectivo, error) {
	return s.repo.ListarCiclos(ctx, filtroArchivado)
}

// ResultadoArchivado es lo que el handler HTTP necesita para armar la
// respuesta de RF-02.4/02.5.
type ResultadoArchivado struct {
	NuevoCicloID     *string
	CursosClonados   int
	MateriasClonadas int
}

// ArchivarYClonar implementa RF-02.4/02.5.
//
// El orden de los tres pasos es deliberado y NO se puede reacomodar:
//
//  1. Snapshot histórico (reporting): tiene que ir antes del borrado, o
//     quedaría vacío.
//  2. Archivar el ciclo (transaccional dentro de este paquete).
//  3. Borrar físicamente las reservas (reservation): último a propósito,
//     porque es el ÚNICO paso irreversible de toda la cascada.
//
// La cascada cruza tres paquetes, cada uno dueño de su propio repo, así
// que no hay una transacción que los abarque a todos sin romper el límite
// de dominio (docs/06-arquitectura.md §3). Lo que sí se puede garantizar
// es que ningún fallo destruya datos: si (1) o (2) fallan, todavía no se
// borró nada; si falla (3), el ciclo queda archivado con sus reservas
// intactas y basta con reintentar el archivado para completar la limpieza.
// Con el borrado en el medio —como estaba antes— un fallo al archivar
// dejaba el año entero de reservas borrado y el ciclo sin archivar, sin
// vuelta atrás posible.
//
// Ese reintento tiene que funcionar de verdad, y hasta ahora no funcionaba:
// moría en ErrCicloYaArchivado antes de llegar al borrado, así que el único
// camino para completar la limpieza era SQL a mano contra producción. Los
// dos cambios que lo hacen posible son el reintento explícito de abajo y el
// ON CONFLICT DO NOTHING del snapshot (ver reporting/infrastructure).
func (s *Service) ArchivarYClonar(ctx context.Context, cicloID string, clonarAAnio *int) (*ResultadoArchivado, error) {
	ciclo, err := s.repo.BuscarCicloPorID(ctx, cicloID)
	if err != nil {
		return nil, err
	}

	if err := ciclo.Archivar(); err != nil {
		// Archivar dos veces sigue siendo un error (RF-02.4) — salvo que el
		// ciclo todavía tenga reservas, que es la huella que deja un intento
		// anterior interrumpido entre el paso 2 y el 3. En ese caso esto no
		// es "archivar de nuevo" sino terminar lo que quedó a medias, y los
		// pasos que faltan son justamente los que siguen.
		if !errors.Is(err, domain.ErrCicloYaArchivado) {
			return nil, err
		}
		limpiezaPendiente, errValidacion := s.validadorReservas.TieneReservasDeCiclo(ctx, cicloID)
		if errValidacion != nil {
			return nil, fmt.Errorf("verificando si quedó limpieza pendiente del ciclo: %w", errValidacion)
		}
		if !limpiezaPendiente {
			return nil, err
		}
	}

	if err := s.archivadorHistorico.GuardarSnapshotDeCiclo(ctx, cicloID, ciclo.Anio); err != nil {
		return nil, fmt.Errorf("guardando snapshot histórico: %w", err)
	}

	if err := s.repo.ArchivarCiclo(ctx, cicloID); err != nil {
		return nil, fmt.Errorf("archivando ciclo: %w", err)
	}

	if err := s.archivadorHistorico.EliminarReservasDeCiclo(ctx, cicloID); err != nil {
		return nil, fmt.Errorf("eliminando reservas del ciclo archivado (el ciclo ya quedó archivado y el snapshot guardado; reintentar el archivado completa la limpieza): %w", err)
	}

	resultado := &ResultadoArchivado{}

	if clonarAAnio != nil {
		nuevoCiclo, err := domain.NuevoCicloLectivo(s.nuevoID(), *clonarAAnio)
		if err != nil {
			return nil, err
		}

		cursosClonados, materiasClonadas, err := s.repo.ClonarCicloA(ctx, cicloID, nuevoCiclo)
		if err != nil {
			return nil, fmt.Errorf("clonando ciclo: %w", err)
		}

		resultado.NuevoCicloID = &nuevoCiclo.ID
		resultado.CursosClonados = cursosClonados
		resultado.MateriasClonadas = materiasClonadas
	}

	return resultado, nil
}

// ListarMateriasReservables implementa el selector de materias de RF-04.1.
// Un Admin puede reservar para cualquier materia no archivada; un docente,
// solo para las suyas.
func (s *Service) ListarMateriasReservables(ctx context.Context, usuarioID string, esAdmin bool) ([]MateriaReservable, error) {
	if esAdmin {
		return s.repo.ListarMateriasReservables(ctx, nil)
	}
	return s.repo.ListarMateriasReservables(ctx, &usuarioID)
}

// ── Curso ───────────────────────────────────────────────────────────────

func (s *Service) CrearCurso(ctx context.Context, cicloLectivoID, nombre string) (*domain.Curso, error) {
	c, err := domain.NuevoCurso(s.nuevoID(), cicloLectivoID, nombre)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearCurso(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// EditarCurso implementa RF-02.11: renombrar mientras el ciclo está activo.
func (s *Service) EditarCurso(ctx context.Context, cursoID, nuevoNombre string) error {
	c, err := s.repo.BuscarCursoPorID(ctx, cursoID)
	if err != nil {
		return err
	}
	if err := c.RenombrarA(nuevoNombre); err != nil {
		return err
	}
	return s.repo.GuardarCurso(ctx, c)
}

// EliminarCurso implementa RF-02.11: solo si ninguna de sus materias tiene
// reservas asociadas.
func (s *Service) EliminarCurso(ctx context.Context, cursoID string) error {
	tieneReservas, err := s.validadorReservas.TieneReservasCurso(ctx, cursoID)
	if err != nil {
		return fmt.Errorf("verificando reservas del curso: %w", err)
	}
	if tieneReservas {
		return ErrCursoConReservas
	}
	return s.repo.EliminarCurso(ctx, cursoID)
}

func (s *Service) ListarCursos(ctx context.Context, cicloID string) ([]*domain.Curso, error) {
	return s.repo.ListarCursosPorCiclo(ctx, cicloID)
}

// ── Materia ─────────────────────────────────────────────────────────────

func (s *Service) CrearMateria(ctx context.Context, cursoID, nombre string) (*domain.Materia, error) {
	m, err := domain.NuevaMateria(s.nuevoID(), cursoID, nombre)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearMateria(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) EditarMateria(ctx context.Context, materiaID, nuevoNombre string) error {
	m, err := s.repo.BuscarMateriaPorID(ctx, materiaID)
	if err != nil {
		return err
	}
	if err := m.RenombrarA(nuevoNombre); err != nil {
		return err
	}
	return s.repo.GuardarMateria(ctx, m)
}

// EliminarMateria implementa RF-02.11: solo si no tiene reservas asociadas.
func (s *Service) EliminarMateria(ctx context.Context, materiaID string) error {
	tieneReservas, err := s.validadorReservas.TieneReservasMateria(ctx, materiaID)
	if err != nil {
		return fmt.Errorf("verificando reservas de la materia: %w", err)
	}
	if tieneReservas {
		return ErrMateriaConReservas
	}
	return s.repo.EliminarMateria(ctx, materiaID)
}

func (s *Service) ListarMaterias(ctx context.Context, cursoID string) ([]*domain.Materia, error) {
	return s.repo.ListarMateriasPorCurso(ctx, cursoID)
}

// ── DocenteMateria ──────────────────────────────────────────────────────

// AsignarDocente implementa RF-02.6: solo se puede asignar a un usuario
// que existe y está en estado APROBADA (validado a través del puerto
// hacia auth, nunca importando ese paquete directamente).
func (s *Service) AsignarDocente(ctx context.Context, materiaID, usuarioID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	if _, err := s.repo.BuscarMateriaPorID(ctx, materiaID); err != nil {
		return nil, err
	}

	valido, err := s.validadorUsuario.ExisteYAprobado(ctx, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("validando usuario: %w", err)
	}
	if !valido {
		return nil, ErrUsuarioNoValidoParaAsignar
	}

	dm := domain.NuevoDocenteMateria(s.nuevoID(), usuarioID, materiaID, rol)
	if err := s.repo.AsignarDocente(ctx, dm); err != nil {
		return nil, err
	}
	return dm, nil
}

// RemoverDocenteMateria quita la asignación y, si con eso la materia se
// queda sin ningún docente activo, cancela sus reservas futuras (RF-02.8) y
// avisa a los Admin.
//
// Antes era un passthrough al repo. auth.DarDeBaja hacía todo este trabajo
// —detectar materias huérfanas, cancelar, notificar— pero quitar la
// asignación directamente llegaba al MISMO estado por otro camino y sin
// ninguna de esas consecuencias: la materia quedaba sin nadie a cargo, con
// reservas futuras vivas a nombre de un docente que ya no la tiene asignada
// y que por lo tanto tampoco podía cancelarlas él (RF-04.1 le exige estar
// asignado).
//
// El orden —cancelar ANTES de borrar el vínculo— es el mismo de
// auth.DarDeBaja y por la misma razón: la cascada cruza a reservation por un
// puerto con su propia transacción, así que el conjunto no puede ser atómico
// sin romper el límite de dominio (docs/06-arquitectura.md §3). Lo que sí se
// puede elegir es qué queda si falla. Con el vínculo ya borrado, un fallo
// dejaba reservas vivas en una materia sin docente y sin forma de reintentar
// (la asignación que había que quitar ya no existe). Cancelando primero, un
// fallo deja todo como estaba y el Admin puede volver a intentar.
//
// Devuelve cuántas reservas se cancelaron: es una operación destructiva
// disparada por un click que dice "quitar docente", y quien la ejecuta
// merece saber qué se llevó puesto.
func (s *Service) RemoverDocenteMateria(ctx context.Context, docenteMateriaID string) (int, error) {
	dm, err := s.repo.BuscarDocenteMateria(ctx, docenteMateriaID)
	if err != nil {
		return 0, err
	}

	quedaOtro, err := s.quedaOtroDocenteActivo(ctx, dm.MateriaID, dm.UsuarioID)
	if err != nil {
		return 0, err
	}

	canceladas := 0
	if !quedaOtro {
		motivo := "Se quitó al único docente asignado a esta materia"
		canceladas, err = s.canceladorReservas.CancelarReservasFuturasDeMateria(ctx, dm.MateriaID, motivo)
		if err != nil {
			return 0, fmt.Errorf("cancelando las reservas de la materia %s (la asignación se conservó para poder reintentar): %w",
				dm.MateriaID, err)
		}
	}

	if err := s.repo.RemoverDocenteMateria(ctx, docenteMateriaID); err != nil {
		return 0, err
	}

	if !quedaOtro {
		// Mismo aviso que la cascada de auth, con su propio tipo de evento:
		// para el Admin que lo lee, "se dio de baja al docente" y "se le
		// quitó la materia" no son la misma noticia.
		s.bus.Publish(eventbus.Evento{
			Tipo: "docente.desasignado.materia-huerfana",
			Payload: map[string]any{
				"usuarioId":          dm.UsuarioID,
				"materiaId":          dm.MateriaID,
				"reservasCanceladas": canceladas,
			},
		})
	}

	return canceladas, nil
}

// quedaOtroDocenteActivo es el equivalente de auth.GestorMateriasDocente.
// QuedaOtroDocenteActivo, resuelto con los puertos que academic ya tiene:
// la lista de asignaciones es propia, y "el usuario sigue activo" es
// justamente lo que responde ValidadorUsuario (RF-02.6 usa el mismo
// criterio para dejar asignar).
func (s *Service) quedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error) {
	docentes, err := s.repo.ListarDocentesDeMateria(ctx, materiaID)
	if err != nil {
		return false, fmt.Errorf("listando docentes de la materia: %w", err)
	}
	for _, d := range docentes {
		if d.UsuarioID == usuarioIDExcluido {
			continue
		}
		activo, err := s.validadorUsuario.ExisteYAprobado(ctx, d.UsuarioID)
		if err != nil {
			return false, fmt.Errorf("validando al docente %s de la materia: %w", d.UsuarioID, err)
		}
		if activo {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error) {
	return s.repo.ListarDocentesDeMateria(ctx, materiaID)
}
