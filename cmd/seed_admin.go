package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/adminseed"
	"github.com/ramiro/sgrc/internal/shared/security"
)

// pgxUsuarioRepo implementa adminseed.Repo contra Postgres real. Es la
// única pieza de este archivo que sabe que existe una base de datos —
// adminseed.SembrarSiHaceFalta no lo sabe, por diseño (ver
// internal/shared/adminseed/adminseed.go).
type pgxUsuarioRepo struct {
	pool *pgxpool.Pool
}

// ExisteAdminActivo mira el estado además del rol: un ADMIN en BAJA no
// puede entrar, así que para lo que decide esta consulta —si el sistema
// tiene acceso administrativo— es como si no estuviera.
func (r *pgxUsuarioRepo) ExisteAdminActivo(ctx context.Context) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM usuario WHERE rol = 'ADMIN' AND estado = 'APROBADA')`,
	).Scan(&existe)
	return existe, err
}

// CrearAdmin siembra —o reactiva— la cuenta administrativa inicial.
//
// El ON CONFLICT no es una comodidad: solo se llega acá cuando NO hay ningún
// Admin activo, y el caso que lo hace necesario es justamente ese, que la
// cuenta del SEED_ADMIN_EMAIL exista pero esté en BAJA. Con un INSERT pelado
// eso terminaba en violación de unicidad, y como main.go corta el arranque
// si el seed falla, la base sin admin usable se convertía en un contenedor
// que no levanta: el sistema quedaba inaccesible Y apagado.
//
// Reescribir el hash es deliberado y es lo que hace que el rescate sirva:
// quien puede editar el .env del servidor es quien administra la escuela, y
// si no supiera la contraseña de esa cuenta no habría forma de volver a
// entrar. La operación queda a la vista en el log del arranque.
func (r *pgxUsuarioRepo) CrearAdmin(ctx context.Context, email, passwordHash string) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO usuario (nombre, apellido, email, password_hash, rol, estado, fecha_aprobacion)
		VALUES ('Admin', 'Inicial', $1, $2, 'ADMIN', 'APROBADA', now())
		ON CONFLICT (email) DO UPDATE
		SET rol = 'ADMIN', estado = 'APROBADA', password_hash = EXCLUDED.password_hash,
		    fecha_aprobacion = now(), debe_cambiar_password = TRUE
	`, email, passwordHash)
	if err != nil {
		return err
	}
	// El INSERT y el UPDATE devuelven 1 fila los dos, así que el log tiene
	// que decir cuál fue: reactivar una cuenta existente pisa su contraseña
	// y no es algo que deba pasar en silencio.
	if tag.RowsAffected() > 0 {
		log.Printf("admin inicial: cuenta %s lista (no había ningún ADMIN en estado APROBADA)", email)
	}
	return nil
}

// seedAdminSiHaceFalta es el punto de entrada que llama main.go — envuelve
// adminseed.SembrarSiHaceFalta con las piezas concretas (Postgres +
// argon2id vía internal/shared/security + variables de entorno reales).
// El hash usa la misma función que va a usar internal/auth para login y
// registro normal — un solo criterio de hasheo en todo el sistema.
func seedAdminSiHaceFalta(ctx context.Context, pool *pgxpool.Pool, getenv func(string) string) error {
	repo := &pgxUsuarioRepo{pool: pool}
	// Se normaliza con la misma función que usa el registro: si el .env
	// trae "Admin@Escuela.edu.ar", la fila tiene que quedar igual que si esa
	// cuenta se hubiera creado desde la aplicación, o el ON CONFLICT (email)
	// de arriba compararía contra otra forma de la misma dirección.
	email := authdomain.NormalizarEmail(getenv("SEED_ADMIN_EMAIL"))
	password := getenv("SEED_ADMIN_PASSWORD")
	return adminseed.SembrarSiHaceFalta(ctx, repo, security.HashPassword, email, password)
}
