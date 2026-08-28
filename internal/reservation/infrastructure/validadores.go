package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/reservation/application"
)

// ── ValidadorMateria (puerto hacia academic) ───────────────────────────

var _ application.ValidadorMateria = (*ValidadorMateriaPostgres)(nil)

// ValidadorMateriaPostgres consulta docente_materia directamente — a
// propósito NO importa internal/academic (ver docs/06-arquitectura.md §3,
// mismo criterio que ValidadorUsuarioPostgres de academic hacia auth).
type ValidadorMateriaPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorMateriaPostgres(pool *pgxpool.Pool) *ValidadorMateriaPostgres {
	return &ValidadorMateriaPostgres{pool: pool}
}

func (v *ValidadorMateriaPostgres) DocenteEstaAsignado(ctx context.Context, materiaID, usuarioID string) (bool, error) {
	var existe bool
	err := v.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM docente_materia WHERE materia_id = $1 AND usuario_id = $2)`,
		materiaID, usuarioID,
	).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando asignación docente-materia: %w", err)
	}
	return existe, nil
}

// ── ValidadorEquipo (puerto hacia inventory) ────────────────────────────

var _ application.ValidadorEquipo = (*ValidadorEquipoPostgres)(nil)

// ValidadorEquipoPostgres consulta la tabla pc directamente — a propósito NO
// importa internal/inventory.
type ValidadorEquipoPostgres struct {
	pool *pgxpool.Pool
}

func NewValidadorEquipoPostgres(pool *pgxpool.Pool) *ValidadorEquipoPostgres {
	return &ValidadorEquipoPostgres{pool: pool}
}

func (v *ValidadorEquipoPostgres) EquipoDisponibleParaReservar(ctx context.Context, equipoID string) (bool, error) {
	var estado string
	var dadoDeBaja, reservable bool
	err := v.pool.QueryRow(ctx,
		`SELECT estado, dado_de_baja, reservable FROM equipo WHERE id = $1`, equipoID,
	).Scan(&estado, &dadoDeBaja, &reservable)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // no existe → no disponible, pero no es un error
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando disponibilidad del equipo: %w", err)
	}
	// `reservable` no es redundante con la lista de disponibles, que ya lo
	// filtra: un pedido armado a mano no pasa por esa lista, y sin este chequeo
	// se podría reservar un cargador igual (RF-03.16).
	return estado == "DISPONIBLE" && !dadoDeBaja && reservable, nil
}

// EquiposNoReservables es la versión de lote de la de arriba, en una sola
// consulta.
func (v *ValidadorEquipoPostgres) EquiposNoReservables(ctx context.Context, equipoIDs []string) ([]string, error) {
	if len(equipoIDs) == 0 {
		return nil, nil
	}

	rows, err := v.pool.Query(ctx, `
		SELECT id FROM equipo
		WHERE id = ANY($1)
		  AND estado = 'DISPONIBLE'
		  AND dado_de_baja = false
		  AND reservable = true
	`, equipoIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("verificando disponibilidad de los equipos: %w", err)
	}
	defer rows.Close()

	reservables := make(map[string]bool, len(equipoIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("escaneando equipo reservable: %w", err)
		}
		reservables[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recorriendo equipos reservables: %w", err)
	}

	// Se recorre la lista PEDIDA y no el mapa para conservar el orden en que
	// llegaron: el mensaje de error nombra máquinas, y que salgan en otro orden
	// que en la pantalla obliga a buscarlas de a una.
	var noReservables []string
	for _, id := range equipoIDs {
		if !reservables[id] {
			noReservables = append(noReservables, id)
		}
	}
	return noReservables, nil
}

// EquipoEstaEnInventario: existe y no está dada de baja, sin mirar el estado.
// Ver el comentario del puerto: entregar no es lo mismo que reservar.
func (v *ValidadorEquipoPostgres) EquipoEstaEnInventario(ctx context.Context, equipoID string) (bool, error) {
	var dadoDeBaja bool
	err := v.pool.QueryRow(ctx,
		`SELECT dado_de_baja FROM equipo WHERE id = $1`, equipoID,
	).Scan(&dadoDeBaja)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si el equipo está en el inventario: %w", err)
	}
	return !dadoDeBaja, nil
}

// CondicionParaEntregar: si el equipo sigue en el inventario y en qué estado
// de circulación está. Las dos cosas en UNA consulta, porque se pregunta una
// vez por equipo del lote.
func (v *ValidadorEquipoPostgres) CondicionParaEntregar(ctx context.Context, equipoID string) (application.CondicionDeEquipo, error) {
	var dadoDeBaja bool
	var estado string
	err := v.pool.QueryRow(ctx,
		`SELECT dado_de_baja, estado FROM equipo WHERE id = $1`, equipoID,
	).Scan(&dadoDeBaja, &estado)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.CondicionDeEquipo{}, nil
		}
		if esIDInvalido(err) {
			return application.CondicionDeEquipo{}, application.ErrIDInvalido
		}
		return application.CondicionDeEquipo{}, fmt.Errorf("verificando si el equipo puede salir del laboratorio: %w", err)
	}
	return application.CondicionDeEquipo{EnInventario: !dadoDeBaja, Estado: estado}, nil
}

// EtiquetasDeEquipos: cómo se nombra cada equipo, para los avisos de
// cancelación.
func (v *ValidadorEquipoPostgres) EtiquetasDeEquipos(ctx context.Context, equipoIDs []string) (map[string]string, error) {
	etiquetas := make(map[string]string, len(equipoIDs))
	if len(equipoIDs) == 0 {
		return etiquetas, nil
	}

	// COALESCE en este orden: el nombre manda cuando existe (un proyector), y si
	// no, el número MÁS el carro.
	//
	// El carro no es un adorno: el identificador es el número del zócalo, así
	// que se repite en cada carro —"PC 4" existe tres veces en una escuela con
	// tres carros— y un aviso que dice solo "PC 4" no le permite a nadie saber
	// de qué máquina habla. Se dice "del Carro 1" y no "· Carro 1" porque estas
	// etiquetas se leen DENTRO de una frase ("Tu reserva del 28/08 (PC 4 del
	// Carro 1) fue cancelada") y dentro de una lista separada por comas; el
	// punto medio es para la columna de una tabla, que es donde lo usa la
	// pantalla de entregas.
	//
	// El LEFT JOIN es lo que deja pasar a los equipos sueltos, que no tienen
	// carro: para ellos el primer COALESCE ya resolvió con el nombre.
	rows, err := v.pool.Query(ctx,
		`SELECT e.id, `+etiquetaConCarroSQL("e", "c")+`
		   FROM equipo e
		   LEFT JOIN carro c ON c.id = e.carro_id
		  WHERE e.id = ANY($1)`, equipoIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("resolviendo las etiquetas de los equipos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, etiqueta string
		if err := rows.Scan(&id, &etiqueta); err != nil {
			return nil, fmt.Errorf("escaneando la etiqueta de un equipo: %w", err)
		}
		etiquetas[id] = etiqueta
	}
	return etiquetas, rows.Err()
}

// ── ObtenedorNombreDocente (puerto hacia auth) ──────────────────────────

var _ application.ObtenedorNombreDocente = (*ObtenedorNombrePostgres)(nil)

// ObtenedorNombrePostgres consulta la tabla usuario directamente — a
// propósito NO importa internal/auth.
type ObtenedorNombrePostgres struct {
	pool *pgxpool.Pool
}

func NewObtenedorNombrePostgres(pool *pgxpool.Pool) *ObtenedorNombrePostgres {
	return &ObtenedorNombrePostgres{pool: pool}
}

// ContactosDe: a quién y cómo escribirle, para los avisos que salen después
// de una cascada. Los ids que no existen simplemente no vienen en el mapa.
func (o *ObtenedorNombrePostgres) ContactosDe(ctx context.Context, usuarioIDs []string) (map[string]application.Contacto, error) {
	if len(usuarioIDs) == 0 {
		return map[string]application.Contacto{}, nil
	}

	rows, err := o.pool.Query(ctx,
		`SELECT id, nombre, apellido, email FROM usuario WHERE id = ANY($1)`, usuarioIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("obteniendo contactos: %w", err)
	}
	defer rows.Close()

	contactos := make(map[string]application.Contacto, len(usuarioIDs))
	for rows.Next() {
		var id, nombre, apellido, email string
		if err := rows.Scan(&id, &nombre, &apellido, &email); err != nil {
			return nil, fmt.Errorf("escaneando contacto: %w", err)
		}
		contactos[id] = application.Contacto{Nombre: nombre + " " + apellido, Email: email}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando contactos: %w", err)
	}
	return contactos, nil
}

func (o *ObtenedorNombrePostgres) NombreCompletoDe(ctx context.Context, usuarioID string) (string, error) {
	var nombre, apellido string
	err := o.pool.QueryRow(ctx,
		`SELECT nombre, apellido FROM usuario WHERE id = $1`, usuarioID,
	).Scan(&nombre, &apellido)
	if err != nil {
		if esIDInvalido(err) {
			return "", application.ErrIDInvalido
		}
		return "", fmt.Errorf("obteniendo nombre del usuario: %w", err)
	}
	return nombre + " " + apellido, nil
}

// MateriaAceptaReservas implementa la mitad "no archivada" de RF-04.1. Se
// mira también el curso y el ciclo: archivar un ciclo marca los tres niveles
// (ver ArchivarCiclo en academic/infrastructure), pero basta con que
// cualquiera de ellos esté archivado para que la materia ya no admita
// reservas nuevas.
func (v *ValidadorMateriaPostgres) MateriaAceptaReservas(ctx context.Context, materiaID string) (bool, error) {
	var acepta bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM materia m
			JOIN curso c ON c.id = m.curso_id
			JOIN ciclo_lectivo cl ON cl.id = c.ciclo_lectivo_id
			WHERE m.id = $1
			  AND m.archivado = false
			  AND c.archivado = false
			  AND cl.archivado = false
		)
	`, materiaID).Scan(&acepta)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("verificando si la materia acepta reservas: %w", err)
	}
	return acepta, nil
}
