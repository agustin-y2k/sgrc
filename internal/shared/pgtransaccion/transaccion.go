// Package pgtransaccion comparte una transacción entre adaptadores del monolito.
// El contexto transporta la conexión; los servicios conservan sus puertos y
// no importan pgx ni los dominios de otros módulos.
package pgtransaccion

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type clave struct{}
type claveConfirmacion struct{}
type pendientesDeConfirmacion struct {
	callbacks []func(context.Context)
	fallo     error
}

// AlConfirmar difiere efectos en memoria hasta confirmar la transacción
// exterior. Un rollback o un reintento descarta sus callbacks. Sin una
// transacción compartida, el llamador ya confirmó y se ejecuta de inmediato.
func AlConfirmar(ctx context.Context, fn func(context.Context)) {
	if p, ok := ctx.Value(claveConfirmacion{}).(*pendientesDeConfirmacion); ok {
		p.callbacks = append(p.callbacks, fn)
		return
	}
	fn(ctx)
}

type Conexion interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func DeContexto(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(clave{}).(pgx.Tx)
	return tx
}
func Con(ctx context.Context, pool *pgxpool.Pool) Conexion {
	if tx := DeContexto(ctx); tx != nil {
		return tx
	}
	return pool
}

// Abrir permite que operaciones ya transaccionales participen en un cierre.
// Commit/Rollback son del dueño de la conexión, nunca del paso anidado.
type Transaccion struct {
	pgx.Tx
	propia bool
}

func Abrir(ctx context.Context, pool *pgxpool.Pool) (Transaccion, error) {
	if tx := DeContexto(ctx); tx != nil {
		return Transaccion{Tx: tx}, nil
	}
	tx, err := pool.Begin(ctx)
	if err == nil {
		if err = prepararActor(ctx, tx); err != nil {
			_ = tx.Rollback(ctx)
		}
	}
	return Transaccion{Tx: tx, propia: true}, err
}
func (t Transaccion) Commit(ctx context.Context) error {
	if t.propia {
		return t.Tx.Commit(ctx)
	}
	return nil
}
func (t Transaccion) Rollback(ctx context.Context) error {
	if t.propia {
		return t.Tx.Rollback(ctx)
	}
	return nil
}

// Ejecutar comparte una transacción READ COMMITTED. El caso de uso debe tomar
// primero el bloqueo que estabiliza su fuente (en el cierre, ciclo FOR UPDATE;
// las escrituras de reservas toman FOR SHARE). Así las lecturas posteriores
// incluyen escritores que terminaron mientras esperaba ese bloqueo. Usar una
// snapshot SERIALIZABLE anterior a la espera puede omitir esas reservas.
// El callback solo
// puede producir efectos en PostgreSQL: puede repetirse ante una serialización
// o un deadlock, y un error revierte también los pasos de los otros módulos.
func Ejecutar(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	if DeContexto(ctx) != nil {
		return fn(ctx)
	}
	var err error
	for range 3 {
		err = ejecutarUnaVez(ctx, pool, fn)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || (pgErr.Code != "40001" && pgErr.Code != "40P01") {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}
func ejecutarUnaVez(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op después del commit
	if err := prepararActor(ctx, tx); err != nil {
		return err
	}
	pendientes := &pendientesDeConfirmacion{}
	ctxTx := context.WithValue(context.WithValue(ctx, clave{}, tx), claveConfirmacion{}, pendientes)
	if err := fn(ctxTx); err != nil {
		return err
	}
	if pendientes.fallo != nil {
		return pendientes.fallo
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, fn := range pendientes.callbacks {
		fn(ctx)
	}
	return nil
}

// Rechazar hace fallar el commit de una escritura compartida (por ejemplo,
// si no pudo persistirse su evento), sin publicar callbacks de ese intento.
func Rechazar(ctx context.Context, err error) bool {
	if p, ok := ctx.Value(claveConfirmacion{}).(*pendientesDeConfirmacion); ok {
		if p.fallo == nil {
			p.fallo = err
		}
		return true
	}
	return false
}
