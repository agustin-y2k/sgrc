package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/academic/application"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	"github.com/jackc/pgx/v5"
)

// Se consulta después de adquirir la coordinación institucional, compartida
// con el cierre y las nuevas reservas. El estado de todos los ancestros cuenta.
func (r *PostgresRepo) VerificarVigencia(ctx context.Context, entidad, id string) error {
	var consulta string
	var ausente error
	switch entidad {
	case "ciclo":
		consulta = `SELECT NOT archivado FROM ciclo_lectivo WHERE id=$1`
		ausente = application.ErrCicloNoEncontrado
	case "curso":
		consulta = `SELECT NOT (c.archivado OR cl.archivado) FROM curso c JOIN ciclo_lectivo cl ON cl.id=c.ciclo_lectivo_id WHERE c.id=$1`
		ausente = application.ErrCursoNoEncontrado
	case "espacio":
		consulta = `SELECT NOT (e.archivado OR cl.archivado) FROM espacio e JOIN ciclo_lectivo cl ON cl.id=e.ciclo_lectivo_id WHERE e.id=$1`
		ausente = application.ErrEspacioNoEncontrado
	case "materia":
		consulta = `SELECT NOT (m.archivado OR c.archivado OR cl.archivado) FROM materia m JOIN contenedor_de_materia c ON c.materia_id=m.id JOIN ciclo_lectivo cl ON cl.id=c.ciclo_lectivo_id WHERE m.id=$1`
		ausente = application.ErrMateriaNoEncontrada
	default:
		return fmt.Errorf("entidad académica desconocida: %s", entidad)
	}
	var vigente bool
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, consulta, id).Scan(&vigente)
	if errors.Is(err, pgx.ErrNoRows) {
		return ausente
	}
	if esIDInvalido(err) {
		return application.ErrIDInvalido
	}
	if err != nil {
		return err
	}
	if !vigente {
		return application.ErrCicloArchivado
	}
	return nil
}
