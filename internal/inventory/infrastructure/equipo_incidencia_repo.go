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

const columnasEquipo = `id, carro_id, identificador, numero_serie, freezado, cpu, ram, sistema_operativo, software_instalado, estado, dado_de_baja, fecha_baja, fecha_alta, tipo, nombre, reservable`

func (r *PostgresRepo) CrearEquipo(ctx context.Context, pc *domain.Equipo) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO equipo (id, carro_id, identificador, numero_serie, freezado, cpu, ram, sistema_operativo, software_instalado, estado, dado_de_baja, fecha_alta, tipo, nombre, reservable)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, pc.ID, nullIfEmpty(pc.CarroID), nullSiCero(pc.Identificador), nullIfEmpty(pc.NumeroSerie), pc.Freezado,
		nullIfEmpty(pc.CPU), nullIfEmpty(pc.RAM), nullIfEmpty(pc.SistemaOperativo), nullIfEmpty(pc.SoftwareInstalado),
		string(pc.Estado), pc.DadaDeBaja, pc.FechaAlta, pc.Tipo, nullIfEmpty(pc.Nombre), pc.Reservable)
	if err != nil {
		if esViolacionUnica(err) {
			return errorDeUnicidadDePC(err)
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando equipo: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarEquipoPorID(ctx context.Context, id string) (*domain.Equipo, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasEquipo+` FROM equipo WHERE id = $1`, id)
	return escanearPC(row)
}

func escanearPC(row pgx.Row) (*domain.Equipo, error) {
	var pc domain.Equipo
	var cpu, ram, so, software, carroID, numeroSerie, nombre *string
	var identificador *int
	var estadoStr string

	err := row.Scan(
		&pc.ID, &carroID, &identificador, &numeroSerie, &pc.Freezado,
		&cpu, &ram, &so, &software,
		&estadoStr, &pc.DadaDeBaja, &pc.FechaBaja, &pc.FechaAlta,
		&pc.Tipo, &nombre, &pc.Reservable,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrPCNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando equipo: %w", err)
	}

	estado, err := domain.ParseEstadoEquipo(estadoStr)
	if err != nil {
		return nil, fmt.Errorf("estado inválido en la base para el equipo %s: %w", pc.ID, err)
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
	// Los cuatro que desde la 015 pueden faltar: un proyector no tiene carro
	// ni número ni serie, y una PC de carro no tiene nombre.
	if carroID != nil {
		pc.CarroID = *carroID
	}
	if identificador != nil {
		pc.Identificador = *identificador
	}
	if numeroSerie != nil {
		pc.NumeroSerie = *numeroSerie
	}
	if nombre != nil {
		pc.Nombre = *nombre
	}

	return &pc, nil
}

func (r *PostgresRepo) GuardarEquipo(ctx context.Context, pc *domain.Equipo) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipo SET
			carro_id=$2, identificador=$3, numero_serie=$4, freezado=$5,
			cpu=$6, ram=$7, sistema_operativo=$8, software_instalado=$9,
			estado=$10, dado_de_baja=$11, fecha_baja=$12,
			tipo=$13, nombre=$14, reservable=$15
		WHERE id=$1
	`, pc.ID, nullIfEmpty(pc.CarroID), nullSiCero(pc.Identificador), nullIfEmpty(pc.NumeroSerie), pc.Freezado,
		nullIfEmpty(pc.CPU), nullIfEmpty(pc.RAM), nullIfEmpty(pc.SistemaOperativo), nullIfEmpty(pc.SoftwareInstalado),
		string(pc.Estado), pc.DadaDeBaja, pc.FechaBaja,
		pc.Tipo, nullIfEmpty(pc.Nombre), pc.Reservable)
	if err != nil {
		if esViolacionUnica(err) {
			return errorDeUnicidadDePC(err)
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando equipo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrPCNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarEquiposPorCarro(ctx context.Context, carroID string) ([]*domain.Equipo, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columnasEquipo+` FROM equipo WHERE carro_id = $1 ORDER BY identificador`, carroID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando equipos: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Equipo
	for rows.Next() {
		pc, err := escanearPC(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de equipo: %w", err)
		}
		resultado = append(resultado, pc)
	}
	return resultado, errorDeFilas(rows)
}

// ── Incidencia ──────────────────────────────────────────────────────────

const columnasIncidencia = `id, equipo_id, reportado_por, descripcion, gravedad, fecha, enviado_dge, fecha_envio_dge, estado`

func (r *PostgresRepo) CrearIncidencia(ctx context.Context, i *domain.Incidencia) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO incidencia (id, equipo_id, reportado_por, descripcion, gravedad, fecha, enviado_dge, estado)
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

func (r *PostgresRepo) ListarIncidenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.Incidencia, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columnasIncidencia+` FROM incidencia WHERE equipo_id = $1 ORDER BY fecha DESC`, equipoID)
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

// errorDeUnicidadDePC traduce cuál de las tres restricciones de unicidad de
// `pc` se violó. Sin esto, cargar un segundo "Cargador 1" respondía "ya
// existe una PC con ese identificador", que no le dice nada a quien está
// dando de alta un cargador.
func errorDeUnicidadDePC(err error) error {
	switch nombreDeConstraint(err) {
	case "ux_equipo_suelto_nombre":
		return application.ErrNombreDeEquipoDuplicado
	case "pc_numero_serie_key":
		return application.ErrNumeroSerieDuplicado
	default:
		// UNIQUE (carro_id, identificador), que es el caso habitual.
		return application.ErrIdentificadorDuplicado
	}
}

// nullSiCero es lo mismo que nullIfEmpty pero para el identificador: desde
// la 015 un equipo suelto no tiene número, y guardar 0 lo haría chocar
// contra el CHECK de la 001 (identificador > 0) además de mentir.
func nullSiCero(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
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

// ListarEquiposSueltos: lo prestable que NO está en ningún carro — el
// proyector, los cargadores, las notebooks de otro modelo.
//
// Es una consulta aparte y no un filtro de ListarPCsPorCarro porque no
// responde la misma pregunta: aquella arma la ficha de un carro, y esta es
// la sección "Otros equipos" del inventario, que existe justamente porque
// estas cosas no pertenecen a ninguno.
func (r *PostgresRepo) ListarEquiposSueltos(ctx context.Context) ([]*domain.Equipo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasEquipo+` FROM equipo WHERE carro_id IS NULL ORDER BY tipo, nombre`)
	if err != nil {
		return nil, fmt.Errorf("listando equipos sueltos: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.Equipo
	for rows.Next() {
		pc, err := escanearPC(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de equipo: %w", err)
		}
		resultado = append(resultado, pc)
	}
	return resultado, errorDeFilas(rows)
}
