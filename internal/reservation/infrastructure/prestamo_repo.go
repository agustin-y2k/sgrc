package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

// codigoViolacionUnica: SQLSTATE 23505. Es el primero de este paquete —
// reservation no tenía ningún UNIQUE hasta la migración 013, su constraint
// característica es el EXCLUDE de anti-solapamiento, que usa otro código
// (23P01, ver esViolacionExclusion).
const codigoViolacionUnica = "23505"

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnica
}

const columnasPrestamo = `id, equipo_id, reserva_id, entregado_a_usuario_id, entregado_a_nombre, ` +
	`motivo, devolucion_estimada, entregado_por, entregado_en, devuelto_en, recibido_por, observaciones, ` +
	`avisado_demora_en, avisado_cierre_para`

// columnasPrestamoDetallado agrega la ubicación de la PC y, si el préstamo
// salió contra una reserva, el nombre de la materia. Prefijadas porque la
// consulta hace JOIN.
const columnasPrestamoDetallado = `p.id, p.equipo_id, p.reserva_id, p.entregado_a_usuario_id, p.entregado_a_nombre, ` +
	`p.motivo, p.devolucion_estimada, p.entregado_por, p.entregado_en, p.devuelto_en, p.recibido_por, p.observaciones, ` +
	`p.avisado_demora_en, p.avisado_cierre_para, ` +
	`COALESCE(eq.identificador, 0), COALESCE(eq.nombre, 'PC ' || eq.identificador), COALESCE(c.nombre, ''), m.nombre`

// joinsDelPrestamo: la PC y su carro son INNER —un préstamo sin PC no
// existe— pero la reserva y la materia van LEFT, y por dos motivos
// distintos: un préstamo espontáneo nunca tuvo reserva, y uno que sí la
// tuvo puede haberla perdido cuando se archivó el ciclo lectivo (RF-02.4
// borra las reservas; el préstamo sobrevive con reserva_id en NULL).
const joinsDelPrestamo = `
	FROM prestamo p
	JOIN equipo eq ON eq.id = p.equipo_id
	-- LEFT desde la 015: un proyector o un cargador no están en ningún
	-- carro, y con INNER JOIN desaparecían del listado de lo que está
	-- afuera — justo la lista que no puede tener agujeros.
	LEFT JOIN carro c ON c.id = eq.carro_id
	LEFT JOIN reserva r ON r.id = p.reserva_id
	LEFT JOIN materia m ON m.id = r.materia_id`

func (r *PostgresRepo) CrearPrestamo(ctx context.Context, p *domain.Prestamo) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO prestamo (
			id, equipo_id, reserva_id, entregado_a_usuario_id, entregado_a_nombre,
			motivo, devolucion_estimada, entregado_por, entregado_en, devuelto_en,
			recibido_por, observaciones
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, p.ID, p.EquipoID, p.ReservaID, p.EntregadoAUsuarioID, p.EntregadoANombre,
		nullSiVacio(p.Motivo), p.DevolucionEstimada, p.EntregadoPor, p.EntregadoEn,
		p.DevueltoEn, p.RecibidoPor, nullSiVacio(p.Observaciones))
	if err != nil {
		// El único índice único de la tabla es ux_prestamo_abierto, así que
		// no hay ambigüedad sobre cuál se violó: alguien intentó entregar
		// una máquina que ya estaba afuera.
		if esViolacionUnica(err) {
			return application.ErrPCYaPrestada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("registrando la entrega: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarPrestamoPorID(ctx context.Context, id string) (*domain.Prestamo, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columnasPrestamo+` FROM prestamo WHERE id = $1`, id)
	return escanearPrestamo(row)
}

// BuscarPrestamoAbiertoDeEquipo responde "¿dónde está esta máquina?". Devuelve
// ErrPrestamoNoEncontrado si está en el laboratorio.
func (r *PostgresRepo) BuscarPrestamoAbiertoDeEquipo(ctx context.Context, equipoID string) (*domain.Prestamo, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+columnasPrestamo+` FROM prestamo WHERE equipo_id = $1 AND devuelto_en IS NULL`, equipoID)
	return escanearPrestamo(row)
}

func escanearPrestamo(row pgx.Row) (*domain.Prestamo, error) {
	var p domain.Prestamo
	var motivo, observaciones *string

	err := row.Scan(
		&p.ID, &p.EquipoID, &p.ReservaID, &p.EntregadoAUsuarioID, &p.EntregadoANombre,
		&motivo, &p.DevolucionEstimada, &p.EntregadoPor, &p.EntregadoEn,
		&p.DevueltoEn, &p.RecibidoPor, &observaciones,
		&p.AvisadoDemoraEn, &p.AvisadoCierrePara,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrPrestamoNoEncontrado
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando el registro de entrega: %w", err)
	}
	if motivo != nil {
		p.Motivo = *motivo
	}
	if observaciones != nil {
		p.Observaciones = *observaciones
	}
	return &p, nil
}

// GuardarPrestamo solo actualiza lo que cambia al recibir la máquina. Los
// datos de la entrega —qué PC, a quién, contra qué reserva— no se editan:
// si están mal, lo que corresponde es anotar la devolución y registrar la
// entrega correcta, igual que se tacharía un renglón en el papel.
func (r *PostgresRepo) GuardarPrestamo(ctx context.Context, p *domain.Prestamo) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE prestamo SET devuelto_en=$2, recibido_por=$3, observaciones=$4 WHERE id=$1
	`, p.ID, p.DevueltoEn, p.RecibidoPor, nullSiVacio(p.Observaciones))
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando el registro de entrega: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrPrestamoNoEncontrado
	}
	return nil
}

// ListarPrestamosAbiertos: qué hay afuera ahora mismo.
//
// El orden pone primero lo que debía haber vuelto hace más tiempo, y deja al
// final lo que no tiene hora pactada — que no está atrasado, simplemente no
// se le puede reclamar nada. No hace falta pasarle "ahora": ordenar por
// hora de devolución creciente ya deja arriba lo más vencido y, entre lo que
// todavía no venció, lo que vence primero.
func (r *PostgresRepo) ListarPrestamosAbiertos(ctx context.Context) ([]*application.PrestamoDetallado, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasPrestamoDetallado+joinsDelPrestamo+`
		WHERE p.devuelto_en IS NULL
		ORDER BY p.devolucion_estimada ASC NULLS LAST, p.entregado_en ASC`)
	if err != nil {
		return nil, fmt.Errorf("listando lo que está entregado: %w", err)
	}
	return escanearPrestamosDetallados(rows)
}

// ListarPrestamosDeEquipo es el historial de una máquina, de lo más reciente a
// lo más viejo. Incluye el préstamo abierto si lo hay.
func (r *PostgresRepo) ListarPrestamosDeEquipo(ctx context.Context, equipoID string, limite int) ([]*application.PrestamoDetallado, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+columnasPrestamoDetallado+joinsDelPrestamo+`
		WHERE p.equipo_id = $1
		ORDER BY p.entregado_en DESC
		LIMIT $2`, equipoID, limite)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando el historial de entregas del equipo: %w", err)
	}
	return escanearPrestamosDetallados(rows)
}

func escanearPrestamosDetallados(rows pgx.Rows) ([]*application.PrestamoDetallado, error) {
	defer rows.Close()

	var resultado []*application.PrestamoDetallado
	for rows.Next() {
		var p domain.Prestamo
		var d application.PrestamoDetallado
		var motivo, observaciones *string

		err := rows.Scan(
			&p.ID, &p.EquipoID, &p.ReservaID, &p.EntregadoAUsuarioID, &p.EntregadoANombre,
			&motivo, &p.DevolucionEstimada, &p.EntregadoPor, &p.EntregadoEn,
			&p.DevueltoEn, &p.RecibidoPor, &observaciones,
			&p.AvisadoDemoraEn, &p.AvisadoCierrePara,
			&d.Identificador, &d.Etiqueta, &d.CarroNombre, &d.MateriaNombre,
		)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de entrega: %w", err)
		}
		if motivo != nil {
			p.Motivo = *motivo
		}
		if observaciones != nil {
			p.Observaciones = *observaciones
		}
		d.Prestamo = &p
		resultado = append(resultado, &d)
	}
	return resultado, errorDeFilas(rows)
}

// nullSiVacio guarda NULL en vez de una cadena vacía en las columnas de
// texto opcionales (motivo, observaciones). Son cosas distintas: "no se
// anotó nada" no es lo mismo que "se anotó un texto vacío".
func nullSiVacio(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
