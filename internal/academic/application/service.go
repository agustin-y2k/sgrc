// Package application orquesta los casos de uso de RF-02 (ciclo lectivo,
// cursos, materias, docente_materia).
package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

// Service implementa los casos de uso de academic.
type Service struct {
	repo                Repo
	validadorUsuario    ValidadorUsuario
	validadorReservas   ValidadorReservas
	archivadorHistorico ArchivadorHistorico
	canceladorReservas  CanceladorReservasDeMateria
	// datosDeUsuario y ahora los sumó el pedido para dictar una materia
	// (service_pedidos.go): el pedido lleva fecha, y sus avisos necesitan nombre
	// y correo de quien pide y de quienes ya dictan esa materia.
	datosDeUsuario DatosDeUsuario
	// marcas avisa cuántas preferencias de equipo dejan de aplicar al renombrar
	// una materia. Puede ser nil: el aviso es información, no una regla, y un
	// servicio armado sin este puerto renombra igual.
	marcas  MarcasDeInventario
	ahora   func() time.Time
	nuevoID IDGenerator
	bus     eventbus.EventBus
}

func NewService(
	repo Repo,
	validadorUsuario ValidadorUsuario,
	validadorReservas ValidadorReservas,
	archivadorHistorico ArchivadorHistorico,
	canceladorReservas CanceladorReservasDeMateria,
	datosDeUsuario DatosDeUsuario,
	marcas MarcasDeInventario,
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
		marcas:              marcas,
		nuevoID:             nuevoID,
		ahora:               ahora,
		bus:                 bus,
	}
}

// ── Ciclo lectivo ───────────────────────────────────────────────────────

// CrearCiclo implementa RF-02.1: solo puede existir un ciclo activo a la vez.
func (s *Service) crearCiclo(ctx context.Context, anio int) (*domain.CicloLectivo, error) {
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

// CorregirAnioDeCiclo arregla un ciclo creado con el año equivocado (RF-02.1).
//
// Era la única entidad del sistema sin corrección posible, y la que peor lo
// llevaba: el año es único, así que el ciclo mal creado se quedaba con ese año
// para siempre, y además ocupaba el único lugar de ciclo activo.
//
// Las dos condiciones que no puede ver el dominio:
//
//   - **Sin reservas.** Una reserva lleva su propia fecha y no se muda con el
//     ciclo: correr un ciclo de 2026 a 2027 lo dejaría diciendo que sus clases
//     fueron en 2027 cuando las filas dicen 2026. Se reusa la misma pregunta
//     que hace el archivado, que ya cubre los grupos, las recurrencias y los
//     bloqueos del año viejo.
//   - **El año de destino sin bloqueos.** Un bloqueo se atribuye a un ciclo por
//     el año de su fecha, así que mudarse a un año que ya tiene alguno es
//     adoptarlo en silencio.
//
// La tercera —que el año no lo tenga otro ciclo— la decide el índice único de
// la base, no una comprobación previa.
func (s *Service) corregirAnioDeCiclo(ctx context.Context, cicloID string, nuevoAnio int) error {
	c, err := s.repo.BuscarCicloPorID(ctx, cicloID)
	if err != nil {
		return err
	}
	if c.Archivado {
		return domain.ErrCicloArchivadoNoSeCorrige
	}
	// Corregirlo al año que ya tiene no es un error: es un formulario enviado
	// sin cambios, y hacerlo fallar sólo obligaría a la pantalla a comparar
	// antes de mandar.
	if c.Anio == nuevoAnio {
		return nil
	}

	tieneReservas, err := s.validadorReservas.TieneReservasDeCiclo(ctx, cicloID)
	if err != nil {
		return fmt.Errorf("verificando reservas del ciclo: %w", err)
	}
	if tieneReservas {
		return ErrCicloConReservas
	}

	hayBloqueos, err := s.validadorReservas.HayBloqueosEnElAnio(ctx, nuevoAnio)
	if err != nil {
		return fmt.Errorf("verificando bloqueos del año de destino: %w", err)
	}
	if hayBloqueos {
		return ErrAnioConBloqueos
	}

	if err := c.CorregirAnio(nuevoAnio); err != nil {
		return err
	}
	return s.repo.GuardarCiclo(ctx, c)
}

// EliminarCiclo borra un ciclo lectivo — de verdad, no lógicamente.
//
// Es para deshacer un ciclo recién creado, que es el único momento en que un
// ciclo no significa nada todavía. Por eso las dos condiciones:
//
//   - **Sin cursos.** Un ciclo con cursos cargados es cómo se organizó ese año;
//     eso se archiva, no se borra. Además la clave foránea de `curso` lo
//     impediría igual, y el error crudo de la base sería un 500 en vez de una
//     explicación.
//   - **Sin archivar.** Un ciclo archivado es el registro de un año cerrado, y
//     el histórico de uso que el archivado guardó está indexado por ese año:
//     borrar el ciclo dejaría ese histórico hablando de un año que para el
//     sistema no existió.
//
// Un ciclo sin cursos no tiene reservas —una reserva cuelga de una materia, que
// cuelga de un curso—, así que no hace falta preguntarlo por separado.
func (s *Service) eliminarCiclo(ctx context.Context, cicloID string) error {
	c, err := s.repo.BuscarCicloPorID(ctx, cicloID)
	if err != nil {
		return err
	}
	if c.Archivado {
		return domain.ErrCicloArchivadoNoSeCorrige
	}

	cursos, err := s.repo.ListarCursosPorCiclo(ctx, cicloID)
	if err != nil {
		return fmt.Errorf("verificando si el ciclo tiene cursos: %w", err)
	}
	if len(cursos) > 0 {
		return ErrCicloConCursos
	}
	espacios, err := s.repo.ListarEspaciosPorCiclo(ctx, cicloID)
	if err != nil {
		return err
	}
	if len(espacios) > 0 {
		return ErrCicloConCursos
	}

	return s.repo.EliminarCiclo(ctx, cicloID)
}

// ObtenerCiclo es un passthrough al repo, para que el handler pueda registrar
// en la auditoría el año que el ciclo tenía antes de corregirlo o eliminarlo.
func (s *Service) ObtenerCiclo(ctx context.Context, cicloID string) (*domain.CicloLectivo, error) {
	return s.repo.BuscarCicloPorID(ctx, cicloID)
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

// CierreCompletado conserva la intención y la respuesta de un cierre. Una
// repetición con el mismo destino devuelve el resultado original, incluso si
// el cliente perdió la respuesta del commit.
type CierreCompletado struct {
	AnioDestino *int
	Resultado   ResultadoArchivado
}

// ArchivarYClonar confirma snapshot, archivo, limpieza, clonado y resultado
// juntos. Ante una falla no queda ningún paso confirmado por separado.
func (s *Service) archivarYClonar(ctx context.Context, cicloID string, clonarAAnio *int) (*ResultadoArchivado, error) {
	var resultado *ResultadoArchivado
	err := s.repo.EnCierre(ctx, cicloID, func(ctx context.Context) error {
		cierre, err := s.repo.BuscarCierre(ctx, cicloID)
		if err != nil {
			return err
		}
		if cierre != nil {
			if (cierre.AnioDestino == nil) != (clonarAAnio == nil) ||
				(clonarAAnio != nil && *cierre.AnioDestino != *clonarAAnio) {
				return ErrDestinoDeCierreDiferente
			}
			copia := cierre.Resultado
			resultado = &copia
			return nil
		}
		ciclo, err := s.repo.BuscarCicloPorID(ctx, cicloID)
		if err != nil {
			return err
		}
		// Un archivo legado sin constancia de cierre no se reconstruye a ciegas:
		// podría tener un snapshot previo y haber perdido parte de sus reservas.
		copia := *ciclo
		if err := copia.Archivar(); err != nil {
			return err
		}
		var nuevoCiclo *domain.CicloLectivo
		if clonarAAnio != nil {
			nuevoCiclo, err = domain.NuevoCicloLectivo(s.nuevoID(), *clonarAAnio)
			if err != nil {
				return err
			}
			if err := s.verificarAnioLibre(ctx, *clonarAAnio); err != nil {
				return err
			}
		}
		if err := s.archivadorHistorico.GuardarSnapshotDeCiclo(ctx, cicloID, ciclo.Anio); err != nil {
			return fmt.Errorf("guardando snapshot histórico: %w", err)
		}
		if err := s.repo.ArchivarCiclo(ctx, cicloID); err != nil {
			return fmt.Errorf("archivando ciclo: %w", err)
		}
		if err := s.archivadorHistorico.EliminarReservasDeCiclo(ctx, cicloID); err != nil {
			return fmt.Errorf("eliminando reservas (el cierre se revierte; podés reintentarlo): %w", err)
		}
		resultado = &ResultadoArchivado{}
		if nuevoCiclo != nil {
			cursos, materias, err := s.repo.ClonarCicloA(ctx, cicloID, nuevoCiclo)
			if err != nil {
				return fmt.Errorf("clonando ciclo: %w", err)
			}
			resultado.NuevoCicloID = &nuevoCiclo.ID
			resultado.CursosClonados, resultado.MateriasClonadas = cursos, materias
		}
		return s.repo.GuardarCierre(ctx, cicloID, &CierreCompletado{AnioDestino: clonarAAnio, Resultado: *resultado})
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

// verificarAnioLibre falla si ya existe un ciclo para ese año.
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

// ListarMateriasReservables implementa el selector de materias de RF-04.1. Un
// Admin puede reservar para cualquier materia no archivada; un docente, solo
// para las suyas.
func (s *Service) ListarMateriasReservables(ctx context.Context, usuarioID string, esAdmin bool) ([]MateriaReservable, error) {
	if esAdmin {
		return s.repo.ListarMateriasReservables(ctx, nil)
	}
	return s.repo.ListarMateriasReservables(ctx, &usuarioID)
}

// ListarAsignaciones: quién dicta qué en un ciclo. Lo piden las dos pantallas
// que muestran la relación docente-materia, cada una desde su punta.
func (s *Service) ListarAsignaciones(ctx context.Context, cicloID string) ([]AsignacionDocente, error) {
	return s.repo.ListarAsignaciones(ctx, cicloID)
}

// ── Curso ───────────────────────────────────────────────────────────────

func (s *Service) crearCurso(ctx context.Context, cicloLectivoID string, anio int, division, modalidad string) (*domain.Curso, error) {
	if err := s.repo.VerificarVigencia(ctx, "ciclo", cicloLectivoID); err != nil {
		return nil, err
	}

	c, err := domain.NuevoCurso(s.nuevoID(), cicloLectivoID, anio, division, modalidad)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearCurso(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// EditarCurso implementa RF-02.11: corregir el curso mientras el ciclo está
// activo. Los tres datos van juntos porque son un solo curso descrito de tres
// formas — ver domain.Curso.Editar.
// EliminarCurso implementa RF-02.11: solo si ninguna de sus materias tiene
// reservas asociadas.
func (s *Service) eliminarCurso(ctx context.Context, cursoID string) error {
	if err := s.repo.VerificarVigencia(ctx, "curso", cursoID); err != nil {
		return err
	}

	tieneReservas, err := s.validadorReservas.TieneReservasCurso(ctx, cursoID)
	if err != nil {
		return fmt.Errorf("verificando reservas del curso: %w", err)
	}
	if tieneReservas {
		return ErrCursoConReservas
	}
	return s.repo.EliminarCurso(ctx, cursoID)
}

// ObtenerCurso — ver ObtenerCiclo: la API dejaba editar y borrar un curso sin
// poder pedirlo.
func (s *Service) ObtenerCurso(ctx context.Context, cursoID string) (*domain.Curso, error) {
	return s.repo.BuscarCursoPorID(ctx, cursoID)
}

func (s *Service) ListarCursos(ctx context.Context, cicloID string) ([]*domain.Curso, error) {
	return s.repo.ListarCursosPorCiclo(ctx, cicloID)
}

// ── Materia ─────────────────────────────────────────────────────────────

func (s *Service) crearMateria(ctx context.Context, cursoID, nombre string) (*domain.Materia, error) {
	if err := s.repo.VerificarVigencia(ctx, "curso", cursoID); err != nil {
		return nil, err
	}

	m, err := domain.NuevaMateria(s.nuevoID(), cursoID, nombre)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearMateria(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// EditarMateria renombra una materia y devuelve CUÁNTAS marcas de preferencia
// de equipo dejaron de aplicar por ese cambio.
//
// El número no frena nada: renombrar es legítimo (RF-02.11) y corregir un
// nombre mal escrito no puede depender de cuántas máquinas quedaron marcadas.
// Lo que sí hace falta es que el Admin se entere, porque hasta acá la marca
// dejaba de aplicar en silencio y seguía viéndose en la ficha del equipo
// exactamente igual que una que sí vale.
//
// Se cuenta ANTES de guardar, contra el nombre VIEJO: después del UPDATE ya no
// habría forma de saber cuántas apuntaban a él.
func (s *Service) editarMateria(ctx context.Context, materiaID, nuevoNombre string) (int, error) {
	if err := s.repo.VerificarVigencia(ctx, "materia", materiaID); err != nil {
		return 0, err
	}

	m, err := s.repo.BuscarMateriaPorID(ctx, materiaID)
	if err != nil {
		return 0, err
	}

	if m.InternaDeEspacio {
		return 0, ErrMateriaInternaDeEspacio
	}
	nombreViejo := m.Nombre
	if err := m.RenombrarA(nuevoNombre); err != nil {
		return 0, err
	}

	// Si el nombre no cambió de verdad —sólo espacios o mayúsculas— ninguna
	// marca se ve afectada: la comparación de la base ignora las dos cosas.
	marcasAfectadas := 0
	if s.marcas != nil && domain.NormalizarNombre(nombreViejo) != domain.NormalizarNombre(m.Nombre) {
		marcasAfectadas, err = s.marcas.CuantasDejarianDeAplicar(ctx, materiaID, nombreViejo)
		if err != nil {
			// Contar es informativo: que falle no puede impedir corregir un
			// nombre mal escrito. Se sigue con cero y queda en el log.
			log.Printf("academic: no se pudo contar las marcas de equipo afectadas por renombrar %q: %v",
				nombreViejo, err)
			marcasAfectadas = 0
		}
	}

	if err := s.repo.GuardarMateria(ctx, m); err != nil {
		return 0, err
	}
	return marcasAfectadas, nil
}

// EliminarMateria implementa RF-02.11: solo si no tiene reservas asociadas.
func (s *Service) eliminarMateria(ctx context.Context, materiaID string) error {
	if err := s.repo.VerificarVigencia(ctx, "materia", materiaID); err != nil {
		return err
	}

	m, err := s.repo.BuscarMateriaPorID(ctx, materiaID)
	if err != nil {
		return err
	}
	if m.InternaDeEspacio {
		return ErrMateriaInternaDeEspacio
	}
	tieneReservas, err := s.validadorReservas.TieneReservasMateria(ctx, materiaID)
	if err != nil {
		return fmt.Errorf("verificando reservas de la materia: %w", err)
	}
	if tieneReservas {
		return ErrMateriaConReservas
	}
	return s.repo.EliminarMateria(ctx, materiaID)
}

// ObtenerMateria — ídem ObtenerCurso.
func (s *Service) ObtenerMateria(ctx context.Context, materiaID string) (*domain.Materia, error) {
	return s.repo.BuscarMateriaPorID(ctx, materiaID)
}

func (s *Service) ListarMaterias(ctx context.Context, cursoID string) ([]*domain.Materia, error) {
	return s.repo.ListarMateriasPorCurso(ctx, cursoID)
}

// ── DocenteMateria ──────────────────────────────────────────────────────

// AsignarDocente implementa RF-02.6: solo se puede asignar a un usuario que
// existe y está en estado APROBADA (validado a través del puerto hacia auth,
// nunca importando ese paquete directamente).
func (s *Service) AsignarDocente(ctx context.Context, materiaID, usuarioID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	var resultado *domain.DocenteMateria
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		resultado, err = s.asignarDocente(ctx, materiaID, usuarioID, rol)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) asignarDocente(ctx context.Context, materiaID, usuarioID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	if err := s.repo.VerificarVigencia(ctx, "materia", materiaID); err != nil {
		return nil, err
	}

	if _, err := s.repo.BuscarMateriaPorID(ctx, materiaID); err != nil {
		return nil, err
	}

	valido, err := s.validadorUsuario.ExisteYAprobado(ctx, usuarioID)
	if err != nil {
		// El id mal formado se devuelve pelado: es un centinela que la capa
		// HTTP traduce a un 400 usando su propio texto, y envuelto le llegaba
		// al Admin como "validando usuario: el ID indicado no tiene un formato
		// válido" — plomería nuestra en un mensaje que lee una persona. El
		// resto sí se envuelve: va al 500 genérico y ahí el contexto es lo
		// único que queda en el log.
		if errors.Is(err, ErrIDInvalido) {
			return nil, ErrIDInvalido
		}
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
func (s *Service) CambiarRolDocente(ctx context.Context, docenteMateriaID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	var resultado *domain.DocenteMateria
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		resultado, err = s.cambiarRolDocente(ctx, docenteMateriaID, rol)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func (s *Service) cambiarRolDocente(ctx context.Context, docenteMateriaID string, rol domain.RolDocente) (*domain.DocenteMateria, error) {
	dm, err := s.repo.BuscarDocenteMateria(ctx, docenteMateriaID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.VerificarVigencia(ctx, "materia", dm.MateriaID); err != nil {
		return nil, err
	}

	dm.Rol = rol
	if err := s.repo.GuardarDocenteMateria(ctx, dm); err != nil {
		return nil, err
	}
	return dm, nil
}

// RemoverDocenteMateria quita la asignación y, si con eso la materia se queda
// sin ningún docente activo, cancela sus reservas futuras (RF-02.8) y avisa a
// los Admin.
func (s *Service) RemoverDocenteMateria(ctx context.Context, docenteMateriaID string) (int, error) {
	var resultado int
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		resultado, err = s.removerDocenteMateria(ctx, docenteMateriaID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return resultado, nil
}

func (s *Service) removerDocenteMateria(ctx context.Context, docenteMateriaID string) (int, error) {
	dm, err := s.repo.BuscarDocenteMateria(ctx, docenteMateriaID)
	if err != nil {
		return 0, err
	}
	if err := s.repo.VerificarVigencia(ctx, "materia", dm.MateriaID); err != nil {
		return 0, err
	}

	if err := s.repo.RemoverDocenteMateria(ctx, docenteMateriaID); err != nil {
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

	if !quedaOtro {
		// Mismo aviso que la cascada de auth, con su propio tipo de evento: para el
		// Admin que lo lee, "se dio de baja al docente" y "se le quitó la materia"
		// no son la misma noticia.
		eventbus.AlConfirmar(ctx, s.bus, func(ctx context.Context) {
			eventbus.Emitir(ctx, s.bus, eventbus.Evento{
				Tipo: "docente.desasignado.materia-huerfana",
				Payload: map[string]any{
					"usuarioId":          dm.UsuarioID,
					"materiaId":          dm.MateriaID,
					"reservasCanceladas": canceladas,
				},
			})
		})
	}

	return canceladas, nil
}

// quedaOtroDocenteActivo es el equivalente de auth.GestorMateriasDocente.
func (s *Service) quedaOtroDocenteActivo(ctx context.Context, materiaID, usuarioIDExcluido string) (bool, error) {
	docentes, err := s.repo.ListarDocentesDeMateria(ctx, materiaID)
	if err != nil {
		return false, fmt.Errorf("listando docentes de la materia: %w", err)
	}
	otros := make([]string, 0, len(docentes))
	for _, d := range docentes {
		if d.UsuarioID == usuarioIDExcluido {
			continue
		}
		otros = append(otros, d.UsuarioID)
	}
	if len(otros) == 0 {
		return false, nil
	}

	// Una sola consulta para todos: la pregunta es un EXISTS, no una revisión
	// uno por uno. Importa porque esto corre en el camino de una baja, con
	// alguien esperando.
	quedaAlguno, err := s.validadorUsuario.AlgunoAprobado(ctx, otros)
	if err != nil {
		return false, fmt.Errorf("validando a los demás docentes de la materia: %w", err)
	}
	return quedaAlguno, nil
}

func (s *Service) ListarDocentesDeMateria(ctx context.Context, materiaID string) ([]*domain.DocenteMateria, error) {
	return s.repo.ListarDocentesDeMateria(ctx, materiaID)
}

// ── Registro ────────────────────────────────────────────────────────────

// LugarParaRegistro es un lugar donde alguien puede trabajar, reducido a lo que
// hace falta para nombrarlo: "4°2" y, si la escuela usa modalidades,
// "Electromecánica". Sin id — quien se registra no está eligiendo una
// referencia, está diciendo dónde va a estar para que el Admin sepa a qué
// asignarlo.
//
// Tipo distingue un curso de un espacio (RF-02.13), y el formulario lo usa para
// no preguntar una materia donde no se dicta ninguna.
type LugarParaRegistro struct {
	Nombre    string
	Modalidad string
	Tipo      string
	// Materias de ESE lugar. Van anidadas porque el formulario ofrece las del
	// curso elegido, no las 86 de la escuela.
	Materias []string
}

// OpcionesDeRegistro es lo que el formulario de registro ofrece para elegir.
type OpcionesDeRegistro struct {
	Lugares []LugarParaRegistro
}

// OpcionesDeRegistro devuelve los nombres que el formulario de registro ofrece.
//
// No exige sesión: el registro se abre sin haber entrado. Por eso devuelve
// nombres y nada más, y sólo los del ciclo activo.
func (s *Service) OpcionesDeRegistro(ctx context.Context) (OpcionesDeRegistro, error) {
	return s.repo.NombresParaRegistro(ctx)
}

// ── Espacios (RF-02.13) ─────────────────────────────────────────────────
//
// Los lugares de la institución que no son cursos: Dirección, Biblioteca,
// Preceptoría. Tienen materias y se reserva para ellas igual que para las de un
// curso — ver domain.Espacio para por qué no se modelaron aflojando el curso.

func (s *Service) crearEspacio(ctx context.Context, cicloLectivoID, nombre string) (*domain.Espacio, error) {
	if err := s.repo.VerificarVigencia(ctx, "ciclo", cicloLectivoID); err != nil {
		return nil, err
	}

	e, err := domain.NuevoEspacio(s.nuevoID(), cicloLectivoID, nombre)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearEspacio(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) ListarEspacios(ctx context.Context, cicloID string) ([]*domain.Espacio, error) {
	return s.repo.ListarEspaciosPorCiclo(ctx, cicloID)
}

func (s *Service) ObtenerEspacio(ctx context.Context, id string) (*domain.Espacio, error) {
	return s.repo.BuscarEspacioPorID(ctx, id)
}

func (s *Service) editarEspacio(ctx context.Context, id, nombre string) error {
	if err := s.repo.VerificarVigencia(ctx, "espacio", id); err != nil {
		return err
	}

	e, err := s.repo.BuscarEspacioPorID(ctx, id)
	if err != nil {
		return err
	}
	if err := e.Renombrar(nombre); err != nil {
		return err
	}
	return s.repo.GuardarEspacio(ctx, e)
}

// EliminarEspacio: mismo criterio que un curso (RF-02.11) — lo que tiene clases
// dadas no se borra. La pregunta la contesta el módulo de reservas.
func (s *Service) eliminarEspacio(ctx context.Context, id string) error {
	if err := s.repo.VerificarVigencia(ctx, "espacio", id); err != nil {
		return err
	}

	tieneReservas, err := s.validadorReservas.TieneReservasEspacio(ctx, id)
	if err != nil {
		return fmt.Errorf("verificando reservas del espacio: %w", err)
	}
	if tieneReservas {
		return ErrEspacioConReservas
	}
	return s.repo.EliminarEspacio(ctx, id)
}
