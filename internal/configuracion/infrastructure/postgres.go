package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agustin-y2k/sgrc/internal/configuracion/application"
	"github.com/agustin-y2k/sgrc/internal/configuracion/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/audit"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool} }

// InicializarSiFalta evita releer opciones de despliegue cuando la institución ya decidió sus plazos.
func (r *Repo) InicializarSiFalta(ctx context.Context, valores func() domain.Operacion) error {
	var existe bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM configuracion_operacion WHERE id)`).Scan(&existe); err != nil {
		return err
	}
	if existe {
		return nil
	}
	return r.Inicializar(ctx, valores())
}

// Inicializar importa los valores de instalación sólo una vez; los cambios de la interfaz prevalecen al reiniciar.
func (r *Repo) Inicializar(ctx context.Context, o domain.Operacion) error {
	if err := o.Validar(); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO configuracion_operacion(id,retiro_gracia_minutos,recordatorio_antelacion_minutos,cierre_gracia_minutos,cierre_hora_sin_jornada,licencias_hora_aviso,version) VALUES(true,$1,$2,$3,$4,$5,1) ON CONFLICT(id) DO NOTHING`, o.RetiroGraciaMinutos, o.RecordatorioAntelacionMinutos, o.CierreGraciaMinutos, o.CierreHoraSinJornada, o.LicenciasHoraAviso)
	return err
}
func (r *Repo) Leer(ctx context.Context) (domain.Operacion, error) {
	var o domain.Operacion
	err := pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `SELECT retiro_gracia_minutos,recordatorio_antelacion_minutos,cierre_gracia_minutos,cierre_hora_sin_jornada,licencias_hora_aviso,version FROM configuracion_operacion WHERE id`).Scan(&o.RetiroGraciaMinutos, &o.RecordatorioAntelacionMinutos, &o.CierreGraciaMinutos, &o.CierreHoraSinJornada, &o.LicenciasHoraAviso, &o.Version)
	if err != nil {
		return o, fmt.Errorf("leer configuración operativa: %w", err)
	}
	return o, o.Validar()
}

func (r *Repo) Guardar(ctx context.Context, o domain.Operacion, actor, ip string) (domain.Operacion, error) {
	var result domain.Operacion
	err := pgtransaccion.Coordinar(ctx, r.pool, true, func(ctx context.Context) error {
		anterior, err := r.Leer(ctx)
		if err != nil {
			return err
		}
		if anterior.Version != o.Version {
			return application.ErrDesactualizada
		}
		err = pgtransaccion.Con(ctx, r.pool).QueryRow(ctx, `UPDATE configuracion_operacion SET retiro_gracia_minutos=$1,recordatorio_antelacion_minutos=$2,cierre_gracia_minutos=$3,cierre_hora_sin_jornada=$4,licencias_hora_aviso=$5,version=version+1,actualizada_en=now() WHERE id AND version=$6 RETURNING version`, o.RetiroGraciaMinutos, o.RecordatorioAntelacionMinutos, o.CierreGraciaMinutos, o.CierreHoraSinJornada, o.LicenciasHoraAviso, o.Version).Scan(&o.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrDesactualizada
		}
		if err != nil {
			return err
		}
		if err = audit.NewPostgresAuditor(r.pool).Registrar(ctx, audit.Entrada{UsuarioID: actor, IPOrigen: ip, Accion: audit.ConfiguracionOperativaCambiada, Entidad: "configuracion_operacion", Detalle: map[string]any{"antes": anterior, "despues": o}}); err != nil {
			return err
		}
		result = o
		return nil
	})
	return result, err
}
