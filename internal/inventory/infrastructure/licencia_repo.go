package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

const columnasLicencia = `id, equipo_id, nombre, dias_duracion, dias_aviso, fecha_vencimiento, ` +
	`ultima_renovacion, vencimiento_fijado_por, vencimiento_fijado_en, ` +
	`avisado_previo_para, avisado_vencimiento_para, creada_en`

// columnasLicenciaConUbicacion agrega cómo se llama el equipo y dónde está.
const columnasLicenciaConUbicacion = `l.id, l.equipo_id, l.nombre, l.dias_duracion, l.dias_aviso, l.fecha_vencimiento, ` +
	`l.ultima_renovacion, l.vencimiento_fijado_por, l.vencimiento_fijado_en, ` +
	`l.avisado_previo_para, l.avisado_vencimiento_para, l.creada_en, ` +
	`COALESCE(p.nombre, 'PC ' || p.identificador), COALESCE(p.identificador, 0), ` +
	`p.dado_de_baja, COALESCE(c.id::text, ''), COALESCE(c.nombre, '')`

func (r *PostgresRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO licencia_software (
			id, equipo_id, nombre, dias_duracion, dias_aviso, fecha_vencimiento,
			ultima_renovacion, vencimiento_fijado_por, vencimiento_fijado_en,
			avisado_previo_para, avisado_vencimiento_para, creada_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, l.ID, l.EquipoID, l.Nombre, l.DiasDuracion, l.DiasAviso, l.FechaVencimiento,
		l.UltimaRenovacion, l.VencimientoFijadoPor, l.VencimientoFijadoEn,
		l.AvisadoPrevioPara, l.AvisadoVencimientoPara, l.CreadaEn)
	if err != nil {
		// Acá el UNIQUE es uno solo (equipo_id + lower(nombre)), así que a
		// diferencia de CrearEquipo no hay ambigüedad sobre cuál se violó.
		if esViolacionUnica(err) {
			return application.ErrLicenciaDuplicada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando licencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasLicencia+` FROM licencia_software WHERE id = $1`, id)
	return escanearLicencia(row)
}

func escanearLicencia(row pgx.Row) (*domain.LicenciaSoftware, error) {
	var l domain.LicenciaSoftware
	err := row.Scan(
		&l.ID, &l.EquipoID, &l.Nombre, &l.DiasDuracion, &l.DiasAviso, &l.FechaVencimiento,
		&l.UltimaRenovacion, &l.VencimientoFijadoPor, &l.VencimientoFijadoEn,
		&l.AvisadoPrevioPara, &l.AvisadoVencimientoPara, &l.CreadaEn,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrLicenciaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando licencia: %w", err)
	}
	return &l, nil
}

func escanearLicenciaConUbicacion(row pgx.Row) (*application.LicenciaConUbicacion, error) {
	var l domain.LicenciaSoftware
	var u application.LicenciaConUbicacion

	err := row.Scan(
		&l.ID, &l.EquipoID, &l.Nombre, &l.DiasDuracion, &l.DiasAviso, &l.FechaVencimiento,
		&l.UltimaRenovacion, &l.VencimientoFijadoPor, &l.VencimientoFijadoEn,
		&l.AvisadoPrevioPara, &l.AvisadoVencimientoPara, &l.CreadaEn,
		&u.Etiqueta, &u.Identificador, &u.EquipoDadoDeBaja, &u.CarroID, &u.CarroNombre,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrLicenciaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando licencia con ubicación: %w", err)
	}
	u.Licencia = &l
	return &u, nil
}

func (r *PostgresRepo) GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE licencia_software SET
			nombre=$2, dias_duracion=$3, dias_aviso=$4, fecha_vencimiento=$5,
			ultima_renovacion=$6, vencimiento_fijado_por=$7, vencimiento_fijado_en=$8,
			avisado_previo_para=$9, avisado_vencimiento_para=$10
		WHERE id=$1
	`, l.ID, l.Nombre, l.DiasDuracion, l.DiasAviso, l.FechaVencimiento,
		l.UltimaRenovacion, l.VencimientoFijadoPor, l.VencimientoFijadoEn,
		l.AvisadoPrevioPara, l.AvisadoVencimientoPara)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrLicenciaDuplicada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

// MarcarAvisosEnviados toca SOLO las dos marcas.
func (r *PostgresRepo) MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE licencia_software SET
			avisado_previo_para=$2, avisado_vencimiento_para=$3
		WHERE id=$1
	`, l.ID, l.AvisadoPrevioPara, l.AvisadoVencimientoPara)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("marcando avisos de licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) BorrarLicencia(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM licencia_software WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("borrando licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasLicencia+` FROM licencia_software WHERE equipo_id = $1 ORDER BY nombre`, equipoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando licencias del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.LicenciaSoftware
	for rows.Next() {
		l, err := escanearLicencia(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de licencia: %w", err)
		}
		resultado = append(resultado, l)
	}
	return resultado, errorDeFilas(rows)
}

// ordenDeLaPantalla pone primero lo que hay que resolver.
const ordenDeLaPantalla = `ORDER BY l.fecha_vencimiento IS NOT NULL, l.fecha_vencimiento, ` +
	`COALESCE(c.nombre, ''), COALESCE(p.identificador, 0), COALESCE(p.nombre, ''), l.nombre`

func (r *PostgresRepo) ListarLicencias(ctx context.Context) ([]*application.LicenciaConUbicacion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasLicenciaConUbicacion+`
		FROM licencia_software l
		JOIN equipo p ON p.id = l.equipo_id
		-- LEFT: un equipo suelto puede tener software licenciado igual que los
		-- del carro, y con INNER su licencia no llega a la pantalla — se vence
		-- sin que nadie la vea.
		LEFT JOIN carro c ON c.id = p.carro_id
	`+ordenDeLaPantalla)
	if err != nil {
		return nil, fmt.Errorf("listando licencias: %w", err)
	}
	return escanearFilasConUbicacion(rows)
}

// ContarPendientesDeRenovar cuenta las licencias que hoy están POR_VENCER o
// VENCIDA, sin importar si ya se avisó de ellas.
//
// Es una pregunta distinta de la de ListarCandidatasAAviso, y la diferencia es
// justo la marca de aviso: una licencia que ya avisó y nadie renovó deja de
// ser candidata a un aviso NUEVO, pero sigue pendiente de resolver. Lo que se
// pregunta acá es "¿queda algo por renovar?", que es lo que decide si el aviso
// de la campana todavía tiene a qué apuntar.
func (r *PostgresRepo) ContarPendientesDeRenovar(ctx context.Context, hoy time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM licencia_software l
		JOIN equipo p ON p.id = l.equipo_id
		WHERE l.fecha_vencimiento IS NOT NULL
		  AND p.dado_de_baja = false
		  AND l.fecha_vencimiento - l.dias_aviso <= $1
	`, domain.Dia(hoy)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contando licencias pendientes de renovar: %w", err)
	}
	return n, nil
}

// ListarCandidatasAAviso es el filtro grueso del job.
func (r *PostgresRepo) ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*application.LicenciaConUbicacion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasLicenciaConUbicacion+`
		FROM licencia_software l
		JOIN equipo p ON p.id = l.equipo_id
		-- LEFT por lo mismo que en ListarLicencias, y acá es peor: con INNER
		-- JOIN la licencia de un equipo suelto no era candidata a aviso
		-- NUNCA, así que el correo no salía y nadie se enteraba.
		LEFT JOIN carro c ON c.id = p.carro_id
		WHERE l.fecha_vencimiento IS NOT NULL
		  AND p.dado_de_baja = false
		  AND l.fecha_vencimiento - l.dias_aviso <= $1
		  AND (l.avisado_previo_para IS DISTINCT FROM l.fecha_vencimiento
		       OR l.avisado_vencimiento_para IS DISTINCT FROM l.fecha_vencimiento)
	`+ordenDeLaPantalla, domain.Dia(hoy))
	if err != nil {
		return nil, fmt.Errorf("listando licencias candidatas a aviso: %w", err)
	}
	return escanearFilasConUbicacion(rows)
}

func escanearFilasConUbicacion(rows pgx.Rows) ([]*application.LicenciaConUbicacion, error) {
	defer rows.Close()

	var resultado []*application.LicenciaConUbicacion
	for rows.Next() {
		u, err := escanearLicenciaConUbicacion(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de licencia: %w", err)
		}
		resultado = append(resultado, u)
	}
	return resultado, errorDeFilas(rows)
}
