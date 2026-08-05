package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAuditor escribe cada Entrada en audit_log.
type PostgresAuditor struct {
	pool *pgxpool.Pool
}

func NewPostgresAuditor(pool *pgxpool.Pool) *PostgresAuditor {
	return &PostgresAuditor{pool: pool}
}

func (a *PostgresAuditor) Registrar(ctx context.Context, e Entrada) error {
	var detalle []byte
	if len(e.Detalle) > 0 {
		b, err := json.Marshal(e.Detalle)
		if err != nil {
			return fmt.Errorf("serializando detalle de auditoría: %w", err)
		}
		detalle = b
	}

	var ip *string
	if e.IPOrigen != "" {
		ip = &e.IPOrigen
	}

	_, err := a.pool.Exec(ctx, `
		INSERT INTO audit_log (usuario_id, accion, entidad, entidad_id, detalle, ip_origen)
		VALUES ($1, $2, $3, $4, $5, $6::inet)
	`, e.UsuarioID, e.Accion, e.Entidad, e.EntidadID, detalle, ip)
	if err != nil {
		return fmt.Errorf("insertando audit_log: %w", err)
	}
	return nil
}
