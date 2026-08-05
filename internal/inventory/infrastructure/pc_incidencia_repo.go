package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// ── PC ──────────────────────────────────────────────────────────────────

const columnasPC = `id, carro_id, identificador, numero_serie, freezado, cpu, ram, sistema_operativo, software_instalado, estado, dada_de_baja, fecha_baja, fecha_alta`

func (r *PostgresRepo) CrearPC(ctx context.Context, pc *domain.PC) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO pc (id, carro_id, identificador, numero_serie, freezado, cpu, ram, sistema_operativo, software_instalado, estado, dada_de_baja, fecha_alta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, pc.ID, pc.CarroID, pc.Identificador, pc.NumeroSerie, pc.Freezado,
		nullIfEmpty(pc.CPU), nullIfEmpty(pc.RAM), nullIfEmpty(pc.SistemaOperativo), nullIfEmpty(pc.SoftwareInstalado),
		string(pc.Estado), pc.DadaDeBaja, pc.FechaAlta)
	if err != nil {
		if esViolacionUnica(err) {
			// No podemos distinguir cuál de las dos UNIQUE constraints
			// (carro_id+identificador, o numero_serie global) disparó sin
			// parsear el nombre de la constraint del error de Postgres.
			// application/ ya no tiene forma de saber cuál era la
			// intención sin esa info, así que devolvemos el más común en
			// la práctica (identificador duplicado) — si hace falta
			// distinguir con precisión más adelante, ahí se parsea
			// pgErr.ConstraintName.
			return application.ErrIdentificadorDuplicado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando PC: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarPCPorID(ctx context.Context, id string) (*domain.PC, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasPC+` FROM pc WHERE id = $1`, id)
	return escanearPC(row)
}

func escanearPC(row pgx.Row) (*domain.PC, error) {
	var pc domain.PC
	var cpu, ram, so, software *string
	var estadoStr string

	err := row.Scan(
		&pc.ID, &pc.CarroID, &pc.Identificador, &pc.NumeroSerie, &pc.Freezado,
		&cpu, &ram, &so, &software,
		&estadoStr, &pc.DadaDeBaja, &pc.FechaBaja, &pc.FechaAlta,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrPCNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando PC: %w", err)
	}

	estado, err := domain.ParseEstadoPC(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para PC %s: %w", pc.ID, err)
	}
	pc.Estado = estado
	if cpu != nil {
		pc.CPU = *cpu
	}
	if ram != nil {
		pc.RAM = *ram
	}
	if so != nil {
		pc.SistemaOperativo = *so
	}
	if software != nil {
		pc.SoftwareInstalado = *software
	}

	return &pc, nil
}

func (r *PostgresRepo) GuardarPC(ctx context.Context, pc *domain.PC) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pc SET
			carro_id=$2, identificador=$3, numero_serie=$4, freezado=$5,
			cpu=$6, ram=$7, sistema_operativo=$8, software_instalado=$9,
			estado=$10, dada_de_baja=$11, fecha_baja=$12
		WHERE id=$1
	`, pc.ID, pc.CarroID, pc.Identificador, pc.NumeroSerie, pc.Freezado,
		nullIfEmpty(pc.CPU), nullIfEmpty(pc.RAM), nullIfEmpty(pc.SistemaOperativo), nullIfEmpty(pc.SoftwareInstalado),
		string(pc.Estado), pc.DadaDeBaja, pc.FechaBaja)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrIdentificadorDuplicado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando PC: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrPCNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarPCsPorCarro(ctx context.Context, carroID string) ([]*domain.PC, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columnasPC+` FROM pc WHERE carro_id = $1 ORDER BY identificador`, carroID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando PCs: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.PC
	for rows.Next() {
		pc, err := escanearPC(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de PC: %w", err)
		}
		resultado = append(resultado, pc)
	}
	return resultado, errorDeFilas(rows)
}

// ── Incidencia ──────────────────────────────────────────────────────────

const columnasIncidencia = `id, pc_id, reportado_por, descripcion, gravedad, fecha, enviado_dge, fecha_envio_dge, estado`

func (r *PostgresRepo) CrearIncidencia(ctx context.Context, i *domain.Incidencia) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO incidencia (id, pc_id, reportado_por, descripcion, gravedad, fecha, enviado_dge, estado)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, i.ID, i.PCID, i.ReportadoPor, i.Descripcion, string(i.Gravedad), i.Fecha, i.EnviadoDGE, string(i.Estado))
	if err != nil {
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando incidencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarIncidenciaPorID(ctx context.Context, id string) (*domain.Incidencia, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasIncidencia+` FROM incidencia WHERE id = $1`, id)
	return escanearIncidencia(row)
}

func escanearIncidencia(row pgx.Row) (*domain.Incidencia, error) {
	var i domain.Incidencia
	var gravedadStr, estadoStr string

	err := row.Scan(
		&i.ID, &i.PCID, &i.ReportadoPor, &i.Descripcion, &gravedadStr,
		&i.Fecha, &i.EnviadoDGE, &i.FechaEnvioDGE, &estadoStr,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrIncidenciaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando incidencia: %w", err)
	}

	gravedad, err := domain.ParseGravedad(gravedadStr)
	if err != nil {
		return nil, fmt.Errorf("gravedad inválida en la base para incidencia %s: %w", i.ID, err)
	}
	estado, err := domain.ParseEstadoIncidencia(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para incidencia %s: %w", i.ID, err)
	}
	i.Gravedad = gravedad
	i.Estado = estado

	return &i, nil
}

func (r *PostgresRepo) GuardarIncidencia(ctx context.Context, i *domain.Incidencia) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE incidencia SET
			descripcion=$2, gravedad=$3, enviado_dge=$4, fecha_envio_dge=$5, estado=$6
		WHERE id=$1
	`, i.ID, i.Descripcion, string(i.Gravedad), i.EnviadoDGE, i.FechaEnvioDGE, string(i.Estado))
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando incidencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrIncidenciaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarIncidenciasPorPC(ctx context.Context, pcID string) ([]*domain.Incidencia, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columnasIncidencia+` FROM incidencia WHERE pc_id = $1 ORDER BY fecha DESC`, pcID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando incidencias: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Incidencia
	for rows.Next() {
		i, err := escanearIncidencia(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de incidencia: %w", err)
		}
		resultado = append(resultado, i)
	}
	return resultado, errorDeFilas(rows)
}

// nullIfEmpty convierte un string vacío en nil para que se guarde como
// NULL en columnas opcionales (cpu, ram, sistema_operativo,
// software_instalado) en vez de una cadena vacía — son cosas distintas:
// "no se cargó este dato" vs. "se cargó una cadena vacía a propósito".
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
