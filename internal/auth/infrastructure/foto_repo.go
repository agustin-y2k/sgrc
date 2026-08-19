package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/auth/application"
	"github.com/ramiro/sgrc/internal/auth/domain"
)

// La foto de perfil vive en su propia tabla (ver migrations/002): pesa
// cientos de veces más que el resto de la fila del usuario y se lee en una
// sola pantalla, así que adentro de `usuario` se la habría llevado puesta
// cada listado.

func (r *PostgresRepo) GuardarFoto(ctx context.Context, f *domain.FotoDePerfil) error {
	if !domain.EsTipoValido(f.Tipo) {
		// La base también lo rechaza (hay un CHECK), pero ahí el error es
		// una violación de restricción que no sirve para mostrarle a nadie.
		return domain.ErrFotoTipo
	}

	// UPSERT: cambiar la foto es lo normal, y con INSERT pelado había que
	// borrar antes — dos viajes y una ventana en la que la persona se queda
	// sin foto si el segundo falla.
	_, err := r.db.Exec(ctx, `
		INSERT INTO foto_de_perfil (usuario_id, contenido, tipo, actualizada_en)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (usuario_id) DO UPDATE
		   SET contenido = EXCLUDED.contenido,
		       tipo = EXCLUDED.tipo,
		       actualizada_en = EXCLUDED.actualizada_en
	`, f.UsuarioID, f.Contenido, f.Tipo, f.ActualizadaEn)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("guardando la foto: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarFoto(ctx context.Context, usuarioID string) (*domain.FotoDePerfil, error) {
	var f domain.FotoDePerfil
	f.UsuarioID = usuarioID
	err := r.db.QueryRow(ctx,
		`SELECT contenido, tipo, actualizada_en FROM foto_de_perfil WHERE usuario_id = $1`,
		usuarioID).Scan(&f.Contenido, &f.Tipo, &f.ActualizadaEn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrFotoNoExiste
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando la foto: %w", err)
	}
	return &f, nil
}

func (r *PostgresRepo) EliminarFoto(ctx context.Context, usuarioID string) error {
	// Sin fila no es error: quedarse sin foto es el resultado esperado tanto
	// si había una como si no.
	if _, err := r.db.Exec(ctx, `DELETE FROM foto_de_perfil WHERE usuario_id = $1`, usuarioID); err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("borrando la foto: %w", err)
	}
	return nil
}

func (r *PostgresRepo) UsuariosConFoto(ctx context.Context, usuarioIDs []string) (map[string]bool, error) {
	if len(usuarioIDs) == 0 {
		return map[string]bool{}, nil
	}
	// Solo los ids: traerse `contenido` acá sería bajar todas las imágenes
	// de una lista para decidir qué avatares dibujar.
	rows, err := r.db.Query(ctx,
		`SELECT usuario_id FROM foto_de_perfil WHERE usuario_id = ANY($1)`, usuarioIDs)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("buscando quiénes tienen foto: %w", err)
	}
	defer rows.Close()

	conFoto := make(map[string]bool, len(usuarioIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("escaneando id con foto: %w", err)
		}
		conFoto[id] = true
	}
	return conFoto, rows.Err()
}
