package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Las cuentas de usuario de cada equipo (RF-03.22).

const columnasCuenta = `id, equipo_id, usuario, clase, privilegio, tiene_password,
	password_cifrada, visibilidad, notas, creada_en, actualizada_en`

func (r *PostgresRepo) CrearCuentaDeEquipo(ctx context.Context, c *domain.CuentaDeEquipo) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO equipo_cuenta (id, equipo_id, usuario, clase, privilegio, tiene_password,
		                           password_cifrada, visibilidad, notas, creada_en, actualizada_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, c.ID, c.EquipoID, c.Usuario, c.Clase, string(c.Privilegio), c.TienePassword,
		nullIfEmpty(c.PasswordCifrada), string(c.Visibilidad), nullIfEmpty(c.Notas),
		c.CreadaEn, c.ActualizadaEn)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrCuentaDeEquipoDuplicada
		}
		// El equipo no existe: la FK es lo que lo detecta, y sin traducirlo
		// sería un 500 en vez de un "ese equipo no existe".
		if esViolacionFK(err) {
			return application.ErrEquipoNoEncontrado
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando la cuenta del equipo: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarCuentaDeEquipoPorID(ctx context.Context, id string) (*domain.CuentaDeEquipo, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasCuenta+` FROM equipo_cuenta WHERE id = $1`, id)
	c, err := escanearCuenta(row)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrCuentaDeEquipoNoEncontrada
		}
		return nil, err
	}
	return c, nil
}

func (r *PostgresRepo) GuardarCuentaDeEquipo(ctx context.Context, c *domain.CuentaDeEquipo) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipo_cuenta SET
			usuario=$2, clase=$3, privilegio=$4, tiene_password=$5,
			password_cifrada=$6, visibilidad=$7, notas=$8, actualizada_en=$9
		WHERE id=$1
	`, c.ID, c.Usuario, c.Clase, string(c.Privilegio), c.TienePassword,
		nullIfEmpty(c.PasswordCifrada), string(c.Visibilidad), nullIfEmpty(c.Notas), c.ActualizadaEn)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrCuentaDeEquipoDuplicada
		}
		if esIDInvalido(err) {
			return application.ErrCuentaDeEquipoNoEncontrada
		}
		return fmt.Errorf("actualizando la cuenta del equipo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCuentaDeEquipoNoEncontrada
	}
	return nil
}

// BorrarCuentaDeEquipo la borra de verdad. A diferencia de un equipo, una
// cuenta no tiene historial que preservar: nada la referencia, y una cuenta
// que ya no existe en la máquina y queda listada es peor que no tenerla —
// alguien la va a intentar usar.
func (r *PostgresRepo) BorrarCuentaDeEquipo(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM equipo_cuenta WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrCuentaDeEquipoNoEncontrada
		}
		return fmt.Errorf("borrando la cuenta del equipo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCuentaDeEquipoNoEncontrada
	}
	return nil
}

// ListarCuentasDeEquipo devuelve las cuentas de un equipo. El orden es estable
// —las de administrador primero, después por nombre— para que la lista no baile
// entre recargas: en una pantalla que se consulta parada frente a la máquina,
// que las filas cambien de lugar hace perder el renglón.
func (r *PostgresRepo) ListarCuentasDeEquipo(ctx context.Context, equipoID string) ([]*domain.CuentaDeEquipo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasCuenta+`
		FROM equipo_cuenta
		WHERE equipo_id = $1
		ORDER BY (privilegio = 'ADMINISTRADOR') DESC, usuario_normalizado
	`, equipoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando las cuentas del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.CuentaDeEquipo
	for rows.Next() {
		c, err := escanearCuenta(rows)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, c)
	}
	return resultado, errorDeFilas(rows)
}

// ClasesDeCuentaUsadas alimenta las sugerencias del formulario, igual que las
// categorías de falla y los tipos de equipo: es lo que evita que convivan
// "Microsoft" y "MICROSOFT" sin cerrar la lista.
func (r *PostgresRepo) ClasesDeCuentaUsadas(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT clase FROM equipo_cuenta ORDER BY clase`)
	if err != nil {
		return nil, fmt.Errorf("listando las clases de cuenta usadas: %w", err)
	}
	defer rows.Close()

	var resultado []string
	for rows.Next() {
		var clase string
		if err := rows.Scan(&clase); err != nil {
			return nil, fmt.Errorf("leyendo una clase de cuenta: %w", err)
		}
		resultado = append(resultado, clase)
	}
	return resultado, errorDeFilas(rows)
}

func escanearCuenta(row pgx.Row) (*domain.CuentaDeEquipo, error) {
	var c domain.CuentaDeEquipo
	var privilegio, visibilidad string
	// Punteros: las dos columnas son NULL-ables y NULL significa algo distinto
	// de la cadena vacía en la de la contraseña.
	var passwordCifrada, notas *string

	err := row.Scan(&c.ID, &c.EquipoID, &c.Usuario, &c.Clase, &privilegio, &c.TienePassword,
		&passwordCifrada, &visibilidad, &notas, &c.CreadaEn, &c.ActualizadaEn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrCuentaDeEquipoNoEncontrada
		}
		return nil, fmt.Errorf("leyendo la cuenta del equipo: %w", err)
	}

	c.Privilegio = domain.PrivilegioDeCuenta(privilegio)
	c.Visibilidad = domain.VisibilidadDeCuenta(visibilidad)
	if passwordCifrada != nil {
		c.PasswordCifrada = *passwordCifrada
	}
	if notas != nil {
		c.Notas = *notas
	}
	return &c, nil
}
