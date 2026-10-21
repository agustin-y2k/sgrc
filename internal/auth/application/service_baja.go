package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

var ErrBajaPendiente = errors.New("la cuenta quedó en BAJA, pero faltan tareas por completar; reintentá la baja")

// DarDeBaja deshabilita primero la cuenta y registra qué materias procesar.
// Cada materia confirma cancelación y avance juntos; un fallo no impide
// completar las demás. Repetir la operación sólo ejecuta lo pendiente.
func (s *Service) DarDeBaja(ctx context.Context, usuarioID string) error {
	err := s.repo.EnBaja(ctx, usuarioID, func(ctx context.Context, repo Repo) error {
		u, err := repo.BuscarPorID(ctx, usuarioID)
		if err != nil {
			return err
		}
		proceso, err := repo.BuscarBaja(ctx, usuarioID)
		if err != nil {
			return err
		}
		if proceso != nil {
			return nil
		}
		if u.Estado != domain.EstadoBaja {
			if u.EsAdmin() && u.Estado == domain.EstadoAprobada {
				if err := s.verificarNoEsUltimoAdmin(ctx, repo); err != nil {
					return err
				}
			}
			cambio := *u
			if err := cambio.CambiarEstado(domain.EstadoBaja, s.ahora()); err != nil {
				return err
			}
		}
		materias, err := s.gestorMaterias.MateriasDeDocente(ctx, usuarioID)
		if err != nil {
			return fmt.Errorf("listando materias de la cuenta: %w", err)
		}
		if err := repo.PrepararBaja(ctx, usuarioID, materias, s.ahora()); err != nil {
			return err
		}
		if u.Estado != domain.EstadoBaja {
			_, err = repo.CambiarEstado(ctx, u, domain.EstadoBaja, s.ahora())
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	proceso, err := s.repo.BuscarBaja(ctx, usuarioID)
	if err != nil {
		return err
	}
	if proceso == nil {
		return ErrUsuarioNoEncontrado
	}
	if proceso.Completada {
		return nil
	}
	var fallidas []error
	for _, materiaID := range proceso.MateriasPendientes {
		huerfana, canceladas := false, 0
		err := s.repo.EnBaja(ctx, usuarioID, func(ctx context.Context, repo Repo) error {
			huerfana, canceladas = false, 0
			p, err := repo.BuscarBaja(ctx, usuarioID)
			if err != nil {
				return err
			}
			if p == nil {
				return ErrUsuarioNoEncontrado
			}
			pendiente := false
			for _, id := range p.MateriasPendientes {
				if id == materiaID {
					pendiente = true
					break
				}
			}
			if !pendiente {
				return nil
			}
			quedaOtro, err := s.gestorMaterias.QuedaOtroDocenteActivo(ctx, materiaID, usuarioID)
			if err != nil {
				return err
			}
			if !quedaOtro {
				canceladas, err = s.canceladorReservas.CancelarReservasFuturasDeMateria(ctx, materiaID, "El docente asignado a esta materia fue dado de baja")
				if err != nil {
					return err
				}
				huerfana = true
			}
			if err := repo.CompletarMateriaDeBaja(ctx, usuarioID, materiaID, s.ahora()); err != nil {
				return err
			}
			if huerfana {
				eventbus.Emitir(ctx, s.bus, eventbus.Evento{Tipo: "docente.baja.materia-huerfana", Payload: map[string]any{"usuarioId": usuarioID, "materiaId": materiaID, "reservasCanceladas": canceladas}})
			}
			return nil
		})
		if err != nil {
			fallidas = append(fallidas, fmt.Errorf("materia %s: %w", materiaID, err))
			continue
		}
	}
	if len(fallidas) > 0 {
		return fmt.Errorf("la cuenta quedó en BAJA; %d materia(s) siguen pendientes de procesar; reintentá la baja: %w", len(fallidas), errors.Join(append([]error{ErrBajaPendiente}, fallidas...)...))
	}
	err = s.repo.EnBaja(ctx, usuarioID, func(ctx context.Context, repo Repo) error {
		p, err := repo.BuscarBaja(ctx, usuarioID)
		if err != nil {
			return err
		}
		if p == nil {
			return ErrUsuarioNoEncontrado
		}
		if p.Completada {
			return nil
		}
		if len(p.MateriasPendientes) > 0 {
			return ErrBajaPendiente
		}
		if err := s.gestorMaterias.RemoverAsignacionesDeDocente(ctx, usuarioID); err != nil {
			return fmt.Errorf("la cuenta quedó en BAJA; reintentá para completar la remoción de asignaciones: %w", errors.Join(ErrBajaPendiente, err))
		}
		return repo.CompletarBaja(ctx, usuarioID, s.ahora())
	})
	if err != nil {
		return fmt.Errorf("la cuenta quedó en BAJA con limpieza pendiente: %w", errors.Join(ErrBajaPendiente, err))
	}
	return nil
}
