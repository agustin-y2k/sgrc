// Package application orquesta los casos de uso de RF-06 (reportes de
// uso). ArchivarSnapshotDeCiclo es el lado "reporting" de la cascada de
// archivado de RF-02.4 — ver el comentario en esa función.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/reporting/domain"
)

type Service struct {
	repo        Repo
	infoPC      InfoEquipoParaSnapshot
	infoUsuario InfoUsuarioParaSnapshot
	nuevoID     IDGenerator
}

func NewService(repo Repo, infoPC InfoEquipoParaSnapshot, infoUsuario InfoUsuarioParaSnapshot, nuevoID IDGenerator) *Service {
	return &Service{repo: repo, infoPC: infoPC, infoUsuario: infoUsuario, nuevoID: nuevoID}
}

// ReporteUsoEquipos / ReporteUsoDocentes implementan RF-06.1/06.2 — agregan EN
// VIVO desde reserva/materia/curso. Solo tienen sentido para un ciclo que
// todavía no se archivó: una vez archivado, sus Reserva se borran
// físicamente y esta consulta no encuentra nada — para esos casos está
// HistoricoUsoPCs/HistoricoUsoDocentes en su lugar (RF-06.3, por año).
func (s *Service) ReporteUsoEquipos(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoEquipo, error) {
	if err := validarRango(desde, hasta); err != nil {
		return nil, err
	}
	return s.repo.CalcularUsoEquiposDeCiclo(ctx, cicloID, desde, hasta)
}

func (s *Service) ReporteUsoDocentes(ctx context.Context, cicloID string, desde, hasta *time.Time) ([]domain.ResumenUsoDocente, error) {
	if err := validarRango(desde, hasta); err != nil {
		return nil, err
	}
	return s.repo.CalcularUsoDocentesDeCiclo(ctx, cicloID, desde, hasta)
}

// ReporteIncidenciasPorEquipo / ReporteIncidenciasPorCarro implementan RF-06.3.
func (s *Service) ReporteIncidenciasPorEquipo(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasEquipo, error) {
	if err := validarRango(desde, hasta); err != nil {
		return nil, err
	}
	return s.repo.CalcularIncidenciasPorEquipo(ctx, desde, hasta)
}

func (s *Service) ReporteIncidenciasPorCarro(ctx context.Context, desde, hasta *time.Time) ([]domain.ResumenIncidenciasCarro, error) {
	if err := validarRango(desde, hasta); err != nil {
		return nil, err
	}
	return s.repo.CalcularIncidenciasPorCarro(ctx, desde, hasta)
}

func validarRango(desde, hasta *time.Time) error {
	if desde != nil && hasta != nil && hasta.Before(*desde) {
		return ErrRangoFechasInvalido
	}
	return nil
}

// HistoricoUsoEquipos / HistoricoUsoDocentes implementan RF-06.3 — el
// snapshot ya calculado y persistido de un año lectivo ya archivado.
func (s *Service) HistoricoUsoEquipos(ctx context.Context, anio int) ([]*domain.HistoricoUsoEquipo, error) {
	return s.repo.ListarHistoricoUsoEquipoPorAnio(ctx, anio)
}

func (s *Service) HistoricoUsoDocentes(ctx context.Context, anio int) ([]*domain.HistoricoUsoDocente, error) {
	return s.repo.ListarHistoricoUsoDocentePorAnio(ctx, anio)
}

// ArchivarSnapshotDeCiclo implementa el lado "reporting" de la cascada de
// archivado (RF-02.4/06.3): calcula el uso agregado de cada PC y cada
// docente en este ciclo — TODAVÍA con sus reservas vivas — y lo persiste
// como snapshot permanente bajo el año del ciclo (anio, no cicloID: ver
// domain/historico.go).
//
// Se llama UNA SOLA VEZ, orquestado desde cmd/main.go (el adaptador de
// composición que academic usa para su puerto ArchivadorHistorico), y
// SIEMPRE antes de que reservation borre físicamente las reservas de ese
// ciclo — el orden importa: invertido, no quedaría nada que agregar. Ver
// el mismo criterio en cmd/wiring_adapters.go que ya usan
// inventory/auth hacia reservation.
func (s *Service) ArchivarSnapshotDeCiclo(ctx context.Context, cicloID string, anio int) error {
	// Sin rango de fechas a propósito: el snapshot cubre el ciclo entero.
	usosPC, err := s.repo.CalcularUsoEquiposDeCiclo(ctx, cicloID, nil, nil)
	if err != nil {
		return fmt.Errorf("calculando uso de PCs: %w", err)
	}
	for _, u := range usosPC {
		etiqueta, identificador, carroNombre, err := s.infoPC.EtiquetaYCarroDe(ctx, u.EquipoID)
		if err != nil {
			return fmt.Errorf("obteniendo datos de la PC %s: %w", u.EquipoID, err)
		}
		h, err := domain.NuevoHistoricoUsoEquipo(s.nuevoID(), anio, u.EquipoID, etiqueta, identificador, carroNombre, u.MinutosReservados, u.CantidadReservas)
		if err != nil {
			return err
		}
		if err := s.repo.GuardarHistoricoUsoEquipo(ctx, h); err != nil {
			return fmt.Errorf("guardando histórico de PC %s: %w", u.EquipoID, err)
		}
	}

	usosDocente, err := s.repo.CalcularUsoDocentesDeCiclo(ctx, cicloID, nil, nil)
	if err != nil {
		return fmt.Errorf("calculando uso de docentes: %w", err)
	}
	for _, u := range usosDocente {
		nombre, err := s.infoUsuario.NombreCompletoDe(ctx, u.UsuarioID)
		if err != nil {
			return fmt.Errorf("obteniendo nombre del docente %s: %w", u.UsuarioID, err)
		}
		usuarioID := u.UsuarioID
		h, err := domain.NuevoHistoricoUsoDocente(s.nuevoID(), anio, &usuarioID, nombre, u.CantidadReservas, u.MinutosReservados)
		if err != nil {
			return err
		}
		if err := s.repo.GuardarHistoricoUsoDocente(ctx, h); err != nil {
			return fmt.Errorf("guardando histórico de docente %s: %w", u.UsuarioID, err)
		}
	}

	return nil
}
