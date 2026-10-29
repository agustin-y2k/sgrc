package application

import (
	"context"
	"errors"

	"github.com/agustin-y2k/sgrc/internal/configuracion/domain"
)

var ErrDesactualizada = errors.New("otra persona cambió la configuración; actualizá la pantalla antes de guardar")

type Repo interface {
	Leer(context.Context) (domain.Operacion, error)
	Guardar(context.Context, domain.Operacion, string, string) (domain.Operacion, error)
}
type Service struct{ repo Repo }

func NewService(r Repo) *Service                                      { return &Service{r} }
func (s *Service) Leer(ctx context.Context) (domain.Operacion, error) { return s.repo.Leer(ctx) }
func (s *Service) Guardar(ctx context.Context, o domain.Operacion, actor, ip string) (domain.Operacion, error) {
	if err := o.Validar(); err != nil {
		return domain.Operacion{}, err
	}
	return s.repo.Guardar(ctx, o, actor, ip)
}
