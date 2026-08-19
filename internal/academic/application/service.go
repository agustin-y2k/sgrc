// Package application orquesta los casos de uso de RF-02 (ciclo lectivo,
// cursos, materias, docente_materia).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	// datosDeUsuario y ahora los sumó el pedido para dictar una materia
	// (service_pedidos.go): el pedido lleva fecha, y sus avisos necesitan
	// nombre y correo de quien pide y de quienes ya dictan esa materia.
	datosDeUsuario DatosDeUsuario
	ahora          func() time.Time
	nuevoID        IDGenerator
	bus            eventbus.EventBus
}

func NewService(
	repo Repo,
	validadorUsuario ValidadorUsuario,
	validadorReservas ValidadorReservas,
	archivadorHistorico ArchivadorHistorico,
	canceladorReservas CanceladorReservasDeMateria,
	datosDeUsuario DatosDeUsuario,
	nuevoID IDGenerator,
	ahora func() time.Time,
	bus eventbus.EventBus,
) *Service {
	return &Service{
		repo:                repo,
		validadorUsuario:    validadorUsuario,
		validadorReservas:   validadorReservas,
		archivadorHistorico: archivadorHistorico,
		canceladorReservas:  canceladorReservas,
		datosDeUsuario:      datosDeUsuario,
		nuevoID:             nuevoID,
		ahora:               ahora,
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
// Con el borrado en el medio, un fallo al archivar dejaría el año entero de
// reservas borrado y el ciclo sin archivar, sin vuelta atrás posible.
//
// Ese reintento tiene que funcionar de verdad: sin el reintento explícito
// de abajo moriría en ErrCicloYaArchivado sin llegar al borrado, y sin el
// ON CONFLICT DO NOTHING del snapshot (ver reporting/infrastructure)
// chocaría contra las filas que la corrida anterior ya escribió. El único
// camino para completar la limpieza sería SQL a mano contra producción.
//
// El paso 4 —clonar— quedaba afuera de ese razonamiento, y era el que más
// lo necesitaba: es el ÚNICO que puede fallar por una causa que el Admin
// provocó sin querer (pedir un año que ya existe), y fallaba DESPUÉS del
// borrado irreversible. El resultado era un 409 con el año viejo ya
// destruido y sin forma de completar el clonado desde la interfaz: había
// que cargar a mano los cursos y materias del año nuevo. Se ataca por los
// dos lados:
//
//   - Todo lo que se puede saber del clonado ANTES de tocar nada se
//     verifica antes: que el año sea válido y que no exista ya. El caso
//     probable —un año repetido— ahora falla con el ciclo viejo intacto.
//   - Si igual falla por algo transitorio, el reintento lo completa: un
//     clonado pendiente habilita volver a entrar, igual que la limpieza.
func (s *Service) ArchivarYClonar(ctx context.Context, cicloID string, clonarAAnio *int) (*ResultadoArchivado, error) {
	ciclo, err := s.repo.BuscarCicloPorID(ctx, cicloID)
	if err != nil {
		return nil, err
	}

	// El ciclo nuevo se construye y se valida acá arriba, antes del primer
	// paso destructivo, aunque recién se use al final. Construirlo abajo
	// dejaba que un año inválido o repetido abortara la operación con las
	// reservas del año viejo ya borradas.
	var nuevoCiclo *domain.CicloLectivo
	if clonarAAnio != nil {
		nuevoCiclo, err = domain.NuevoCicloLectivo(s.nuevoID(), *clonarAAnio)
		if err != nil {
			return nil, err
		}
		if err := s.verificarAnioLibre(ctx, *clonarAAnio); err != nil {
			return nil, err
		}
	}

	if err := ciclo.Archivar(); err != nil {
		// Archivar dos veces sigue siendo un error (RF-02.4) — salvo que
		// haya quedado algo a medias de un intento anterior: reservas sin
		// borrar (falla entre el paso 2 y el 3) o el clonado sin hacer
		// (falla en el 4). En esos casos esto no es "archivar de nuevo"
		// sino terminar lo que faltó, y los pasos que siguen son
		// idempotentes: el snapshot no duplica, archivar ya archivado no
		// cambia nada y el borrado no encuentra qué borrar.
		if !errors.Is(err, domain.ErrCicloYaArchivado) {
			return nil, err
		}
		limpiezaPendiente, errValidacion := s.validadorReservas.TieneReservasDeCiclo(ctx, cicloID)
		if errValidacion != nil {
			return nil, fmt.Errorf("verificando si quedó limpieza pendiente del ciclo: %w", errValidacion)
		}
		if !limpiezaPendiente && nuevoCiclo == nil {
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

	if nuevoCiclo != nil {
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

// verificarAnioLibre falla si ya existe un ciclo para ese año.
//
// La base ya lo garantiza con un UNIQUE, y ClonarCicloA traduce esa
// violación al mismo error. La diferencia es CUÁNDO se entera cada uno: la
// constraint salta al final, con el año viejo ya borrado; esto salta antes
// de tocar nada. No es una validación duplicada por descuido — es la
// comprobación temprana de algo cuya garantía real sigue estando en la base.
//
// Se resuelve listando en vez de con una consulta puntual porque los ciclos
// son uno por año: son decenas de filas en la vida entera del sistema, y no
// vale ampliar el contrato del repositorio para eso.
func (s *Service) verificarAnioLibre(ctx context.Context, anio int) error {
	ciclos, err := s.repo.ListarCiclos(ctx, nil)
	if err != nil {
		return fmt.Errorf("verificando si el año destino está libre: %w", err)
	}
	for _, c := range ciclos {
		if c.Anio == anio {
			return ErrCicloYaTieneAnio
		}
	}
	return nil
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

// CambiarRolDocente pasa un vínculo existente de titular a suplente o al
// revés, sin tocar nada más.
//
// Existe como operación propia y no como "quitar y volver a asignar" porque
// ese camino pasa por RemoverDocenteMateria, que si el docente es el único de
// la materia le cancela todas las reservas futuras (RF-02.8). Corregir un rol
// mal cargado no es sacar a nadie de la materia y no puede costar las clases
// ya reservadas.
//
// No revalida al usuario contra auth: el vínculo ya existe, y una cuenta que
// mientras tanto dejó de estar aprobada es un problema de esa cuenta, no algo
// que deba impedir corregir un rol.
func (s *Service) CambiarRolDocente(ctx context.Context, docenteMateriaID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	dm, err := s.repo.BuscarDocenteMateria(ctx, docenteMateriaID)
	if err != nil {
		return nil, err
	}

	dm.Rol = rol
	if err := s.repo.GuardarDocenteMateria(ctx, dm); err != nil {
		return nil, err
	}
	return dm, nil
}

// RemoverDocenteMateria quita la asignación y, si con eso la materia se
// queda sin ningún docente activo, cancela sus reservas futuras (RF-02.8) y
// avisa a los Admin.
//
// No puede ser un passthrough al repo. Este camino llega al MISMO estado
// que auth.DarDeBaja —una materia sin docente a cargo— así que necesita las
// mismas consecuencias: sin ellas quedan reservas futuras vivas a nombre de
// un docente que ya no tiene la materia asignada y que por eso tampoco
// puede cancelarlas él (RF-04.1 le exige estar asignado).
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
