package pgtransaccion

import (
	"context"
	"net"

	"github.com/jackc/pgx/v5"
)

type claveActor struct{}
type actor struct{ usuario, ip string }

// ConActor transporta identidad verificada hasta la transacción. Los callers
// ajenos a HTTP deben proporcionarla; su ausencia se registra como sistema.
func ConActor(ctx context.Context, usuario, ip string) context.Context {
	if net.ParseIP(ip) == nil {
		ip = ""
	}
	return context.WithValue(ctx, claveActor{}, actor{usuario, ip})
}

func prepararActor(ctx context.Context, tx pgx.Tx) error {
	a, _ := ctx.Value(claveActor{}).(actor)
	_, err := tx.Exec(ctx, `SELECT set_config('sgrc.actor', $1, true), set_config('sgrc.ip', $2, true)`, a.usuario, a.ip)
	return err
}
