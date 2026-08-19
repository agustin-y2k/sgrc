package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramiro/sgrc/internal/academic/application"
	"github.com/ramiro/sgrc/internal/academic/domain"
)

// Los pedidos para dictar una materia (tabla pedido_de_materia).

// Las columnas en un solo lugar: el orden lo comparten el SELECT y el
// escaneo, y separarlos es como se cuela un campo desalineado.
const columnasPedido = `id, usuario_id, materia_id, coalesce(curso_solicitado, ''),
	coalesce(materia_solicitada, ''), motivo, estado, coalesce(respuesta, ''),
	resuelto_por, resuelto_en, creado_en`

func escanearPedido(fila pgx.Row) (*domain.PedidoDeMateria, error) {
	var p domain.PedidoDeMateria
	var estado string
	if err := fila.Scan(
		&p.ID, &p.UsuarioID, &p.MateriaID, &p.CursoSolicitado,
		&p.MateriaSolicitada, &p.Motivo, &estado, &p.Respuesta,
		&p.ResueltoPor, &p.ResueltoEn, &p.CreadoEn,
	); err != nil {
		return nil, err
	}
	p.Estado = domain.EstadoPedido(estado)
	return &p, nil
}

func (r *PostgresRepo) CrearPedido(ctx context.Context, p *domain.PedidoDeMateria) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO pedido_de_materia
			(id, usuario_id, materia_id, curso_solicitado, materia_solicitada, motivo, estado, creado_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, p.ID, p.UsuarioID, p.MateriaID,
		vacioComoNull(p.CursoSolicitado), vacioComoNull(p.MateriaSolicitada),
		p.Motivo, string(p.Estado), p.CreadoEn)
	if err != nil {
		if esViolacionUnica(err) {
			// El índice único parcial: ya hay un pedido sin resolver de esta persona
			// por esta materia.
			return application.ErrPedidoDuplicado
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando el pedido de materia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarPedidoPorID(ctx context.Context, id string) (*domain.PedidoDeMateria, error) {
	fila := r.pool.QueryRow(ctx, `SELECT `+columnasPedido+` FROM pedido_de_materia WHERE id = $1`, id)
	p, err := escanearPedido(fila)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPedidoNoExiste
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando el pedido: %w", err)
	}
	return p, nil
}

func (r *PostgresRepo) GuardarPedido(ctx context.Context, p *domain.PedidoDeMateria) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE pedido_de_materia
		   SET estado = $2, respuesta = $3, resuelto_por = $4, resuelto_en = $5
		 WHERE id = $1
	`, p.ID, string(p.Estado), vacioComoNull(p.Respuesta), p.ResueltoPor, p.ResueltoEn)
	if err != nil {
		return fmt.Errorf("guardando el pedido: %w", err)
	}
	return nil
}

func (r *PostgresRepo) ListarPedidos(ctx context.Context, soloPendientes bool) ([]*domain.PedidoDeMateria, error) {
	// Los pendientes van del más viejo al más nuevo: el que más esperó es el que
	// más urge.
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasPedido+` FROM pedido_de_materia
		 WHERE ($1 = false OR estado = 'PENDIENTE')
		 ORDER BY (estado = 'PENDIENTE') DESC,
		          CASE WHEN estado = 'PENDIENTE' THEN creado_en END ASC,
		          creado_en DESC
	`, soloPendientes)
	if err != nil {
		return nil, fmt.Errorf("listando pedidos: %w", err)
	}
	defer rows.Close()
	return escanearPedidos(rows)
}

func (r *PostgresRepo) ListarPedidosDeUsuario(ctx context.Context, usuarioID string) ([]*domain.PedidoDeMateria, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasPedido+` FROM pedido_de_materia
		 WHERE usuario_id = $1
		 ORDER BY creado_en DESC
	`, usuarioID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando pedidos del usuario: %w", err)
	}
	defer rows.Close()
	return escanearPedidos(rows)
}

func (r *PostgresRepo) ContarPedidosPendientes(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM pedido_de_materia WHERE estado = 'PENDIENTE'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("contando pedidos pendientes: %w", err)
	}
	return n, nil
}

func (r *PostgresRepo) TienePedidoAbierto(ctx context.Context, usuarioID, materiaID string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pedido_de_materia
			 WHERE usuario_id = $1 AND materia_id = $2 AND estado = 'PENDIENTE'
		)
	`, usuarioID, materiaID).Scan(&existe)
	if err != nil {
		if esIDInvalido(err) {
			return false, application.ErrIDInvalido
		}
		return false, fmt.Errorf("buscando pedidos abiertos: %w", err)
	}
	return existe, nil
}

func escanearPedidos(rows pgx.Rows) ([]*domain.PedidoDeMateria, error) {
	var resultado []*domain.PedidoDeMateria
	for rows.Next() {
		p, err := escanearPedido(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando pedido: %w", err)
		}
		resultado = append(resultado, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando pedidos: %w", err)
	}
	return resultado, nil
}

// vacioComoNull: en la base, "no escribió ningún curso" es NULL y no una
// cadena vacía.
func vacioComoNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Datos de usuario para los avisos ───────────────────────────────────

var _ application.DatosDeUsuario = (*DatosDeUsuarioPostgres)(nil)

// DatosDeUsuarioPostgres lee nombre y correo de `usuario`, con el mismo
// criterio que ValidadorUsuarioPostgres: la tabla es compartida, pero
// academic no importa internal/auth.
type DatosDeUsuarioPostgres struct {
	pool *pgxpool.Pool
}

func NewDatosDeUsuarioPostgres(pool *pgxpool.Pool) *DatosDeUsuarioPostgres {
	return &DatosDeUsuarioPostgres{pool: pool}
}

func (d *DatosDeUsuarioPostgres) Contacto(ctx context.Context, usuarioID string) (application.ContactoDeDocente, error) {
	var c application.ContactoDeDocente
	var nombre, apellido string
	err := d.pool.QueryRow(ctx,
		`SELECT nombre, apellido, email FROM usuario WHERE id = $1`, usuarioID).
		Scan(&nombre, &apellido, &c.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, fmt.Errorf("no se encontró el usuario %s", usuarioID)
		}
		if esIDInvalido(err) {
			return c, application.ErrIDInvalido
		}
		return c, fmt.Errorf("buscando datos del usuario: %w", err)
	}
	c.UsuarioID = usuarioID
	c.Nombre = nombre + " " + apellido
	return c, nil
}

func (d *DatosDeUsuarioPostgres) Contactos(ctx context.Context, usuarioIDs []string) ([]application.ContactoDeDocente, error) {
	if len(usuarioIDs) == 0 {
		return nil, nil
	}
	rows, err := d.pool.Query(ctx,
		`SELECT id, nombre, apellido, email FROM usuario WHERE id = ANY($1)`, usuarioIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando datos de los usuarios: %w", err)
	}
	defer rows.Close()

	var resultado []application.ContactoDeDocente
	for rows.Next() {
		var c application.ContactoDeDocente
		var nombre, apellido string
		if err := rows.Scan(&c.UsuarioID, &nombre, &apellido, &c.Email); err != nil {
			return nil, fmt.Errorf("escaneando datos de usuario: %w", err)
		}
		c.Nombre = nombre + " " + apellido
		resultado = append(resultado, c)
	}
	return resultado, rows.Err()
}
