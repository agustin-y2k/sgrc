package application

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
)

var ErrEquipoSinRotulo = errors.New("solo se puede renumerar una computadora de carro")
var ErrMotivoRenumeracion = errors.New("indicá un motivo de entre 5 y 500 caracteres para cambiar el rótulo")
var ErrRotuloDesactualizado = errors.New("el rótulo cambió mientras editabas: recargá la ficha antes de volver a renumerar")

// RenumerarEquipo conserva la identidad física, la serie y todas las relaciones.
// La coordinación y la restricción de unicidad cubren cambios concurrentes.
func (s *Service) RenumerarEquipo(ctx context.Context, equipoID string, actual, numero int, motivo string) (map[string]CambioDeCampo, error) {
	if actual <= 0 || numero <= 0 || int64(numero) > 2147483647 {
		return nil, domain.ErrIdentificadorInvalido
	}
	longitud := utf8.RuneCountInString(strings.TrimSpace(motivo))
	if longitud < 5 || longitud > 500 {
		return nil, ErrMotivoRenumeracion
	}
	var cambios map[string]CambioDeCampo
	err := s.repo.Coordinar(ctx, true, func(ctx context.Context) error {
		pc, err := s.repo.BuscarEquipoPorID(ctx, equipoID)
		if err != nil {
			return err
		}
		if pc.DadoDeBaja {
			return domain.ErrEquipoYaDadoDeBaja
		}
		if !pc.EstaEnUnCarro() {
			return ErrEquipoSinRotulo
		}
		if pc.Identificador != actual {
			return ErrRotuloDesactualizado
		}
		antes := *pc
		pc.Identificador = numero
		if err := s.repo.GuardarEquipo(ctx, pc); err != nil {
			return err
		}
		cambios = cambiosDeEquipo(antes, *pc)
		return nil
	})
	return cambios, err
}
