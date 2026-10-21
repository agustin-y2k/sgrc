package infrastructure

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

func (r *PostgresRepo) OpcionesDeReposicion(ctx context.Context, equipoID, materiaID string) (application.OpcionesReposicion, error) {
	var opciones application.OpcionesReposicion
	err := r.conexion(ctx).QueryRow(ctx, `SELECT `+etiquetaConCarroSQL("p", "c")+`, m.nombre
		FROM equipo p LEFT JOIN carro c ON c.id=p.carro_id CROSS JOIN materia m WHERE p.id=$1 AND m.id=$2`, equipoID, materiaID).
		Scan(&opciones.EtiquetaOriginal, &opciones.NombreMateria)
	if err != nil {
		return opciones, fmt.Errorf("leyendo origen de reposición: %w", err)
	}
	rows, err := r.conexion(ctx).Query(ctx, `
		WITH ctx_materia AS (
			SELECT m.nombre_norm AS norm, cu.anio, cu.modalidad_norm, cu.division_norm
			FROM materia m JOIN contenedor_de_materia cu ON cu.materia_id=m.id WHERE m.id=$2
		), `+sqlTramosDePreferencia+`
		SELECT p.id, COALESCE(p.identificador,0), `+etiquetaConCarroSQL("p", "c")+`, p.tipo,
			COALESCE(c.id::text,''), COALESCE(c.nombre,''), p.freezado, COALESCE(p.software_instalado,''),
			`+sqlColumnasDePreferencia+`
		FROM equipo p JOIN equipo origen ON origen.id=$1
		LEFT JOIN carro c ON c.id=p.carro_id `+sqlJoinsDePreferencia+`
		WHERE p.estado='DISPONIBLE' AND NOT p.dado_de_baja AND p.reservable
			-- Una marca propia admite ese dispositivo por identidad, sin inferir
			-- requisitos a partir de hardware, software o tipo del equipo original.
			AND (pr.equipo_id IS NOT NULL OR (
				NOT EXISTS (SELECT 1 FROM propia)
				AND (p.carro_id IS NOT NULL OR p.es_computadora)=(origen.carro_id IS NOT NULL OR origen.es_computadora)
				AND (origen.carro_id IS NOT NULL OR origen.es_computadora OR lower(trim(p.tipo))=lower(trim(origen.tipo)))
			))
		ORDER BY CASE WHEN pr.equipo_id IS NOT NULL THEN 0 WHEN aj.equipo_id IS NOT NULL THEN 2 ELSE 1 END,
			COALESCE(pr.prioridad,0), (p.carro_id IS NOT DISTINCT FROM origen.carro_id) DESC,
			c.nombre, p.identificador, p.nombre, p.id
	`, equipoID, materiaID)
	if err != nil {
		return opciones, fmt.Errorf("buscando sustitutos: %w", err)
	}
	defer rows.Close()
	opciones.Equipos, err = escanearEquiposDisponibles(rows)
	return opciones, err
}

func (r *PostgresRepo) GuardarReposiciones(ctx context.Context, reservas []*domain.Reserva) error {
	if r.pool != nil && pgtransaccion.DeContexto(ctx) == nil {
		return fmt.Errorf("la reposición requiere una transacción")
	}
	if _, err := r.conexion(ctx).Exec(ctx, `SET CONSTRAINTS no_solapamiento DEFERRED`); err != nil {
		return fmt.Errorf("preparando reasignaciones: %w", err)
	}
	for _, res := range reservas {
		if err := r.GuardarReserva(ctx, res); err != nil {
			return err
		}
	}
	if _, err := r.conexion(ctx).Exec(ctx, `SET CONSTRAINTS no_solapamiento IMMEDIATE`); err != nil {
		if esViolacionExclusion(err) {
			return application.ErrSolapamiento
		}
		return fmt.Errorf("verificando reasignaciones: %w", err)
	}
	return nil
}
