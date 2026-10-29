package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/prioridadcurso"
)

var (
	ErrRevisarPrioridadesCurso    = errors.New("el cambio afecta prioridades de dispositivos; revisá el impacto antes de guardar")
	ErrImpactoCursoDesactualizado = errors.New("el curso o sus prioridades cambiaron; volvé a revisar el impacto antes de guardar")
	ErrDecisionPrioridadesCurso   = errors.New("elegí conservar las prioridades exactas o aceptar el nuevo alcance")
	ErrConflictoPrioridadesCurso  = errors.New("hay prioridades exactas distintas en el destino; revisalas en inventario antes de conservarlas")
)

const (
	ConservarExactas = "conservar_exactas"
	AceptarAlcance   = "aceptar_alcance"
)

type ConfirmacionCurso struct {
	Version  string `json:"versionImpacto"`
	Decision string `json:"decisionPrioridades"`
}

type ImpactoCurso struct {
	Version               string                 `json:"version"`
	Anterior              prioridadcurso.Alcance `json:"anterior"`
	Propuesto             prioridadcurso.Alcance `json:"propuesto"`
	Cambios               []prioridadcurso.Marca `json:"cambios"`
	PuedeConservarExactas bool                   `json:"puedeConservarExactas"`
}

func (s *Service) prepararEdicionCurso(ctx context.Context, cursoID string, anio int, division, modalidad string) (*domain.Curso, *ImpactoCurso, error) {
	if err := s.repo.VerificarVigencia(ctx, "curso", cursoID); err != nil {
		return nil, nil, err
	}
	actual, err := s.repo.BuscarCursoPorID(ctx, cursoID)
	if err != nil {
		return nil, nil, err
	}
	nuevo := *actual
	if err := nuevo.Editar(anio, division, modalidad); err != nil {
		return nil, nil, err
	}
	i := &ImpactoCurso{Anterior: prioridadcurso.Alcance{Anio: actual.Anio, Division: actual.Division, Modalidad: actual.Modalidad},
		Propuesto: prioridadcurso.Alcance{Anio: nuevo.Anio, Division: nuevo.Division, Modalidad: nuevo.Modalidad},
		Cambios:   []prioridadcurso.Marca{}, PuedeConservarExactas: true}
	var marcas []prioridadcurso.Marca
	if s.marcas != nil {
		marcas, err = s.marcas.PrioridadesDeCurso(ctx, cursoID, i.Propuesto)
		if err != nil {
			return nil, nil, err
		}
	}
	for _, m := range marcas {
		if m.Antes == m.Despues {
			continue
		}
		i.Cambios = append(i.Cambios, m)
		if m.Exacta && m.Antes && m.DestinoID != "" && m.DestinoPrioridad != m.Prioridad {
			i.PuedeConservarExactas = false
		}
	}
	// La versión incluye también las reglas que siguen aplicando y el curso de origen.
	// Así, confirmar un impacto anterior nunca pisa una edición concurrente.
	contenido, err := json.Marshal(struct {
		Actual    *domain.Curso
		Propuesto prioridadcurso.Alcance
		Marcas    []prioridadcurso.Marca
	}{actual, i.Propuesto, marcas})
	if err != nil {
		return nil, nil, err
	}
	hash := sha256.Sum256(contenido)
	i.Version = hex.EncodeToString(hash[:])
	return &nuevo, i, nil
}

func (s *Service) PrevisualizarEdicionCurso(ctx context.Context, cursoID string, anio int, division, modalidad string) (*ImpactoCurso, error) {
	var impacto *ImpactoCurso
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		_, impacto, err = s.prepararEdicionCurso(ctx, cursoID, anio, division, modalidad)
		return err
	})
	return impacto, err
}

func (s *Service) editarCursoConPrioridades(ctx context.Context, cursoID string, anio int, division, modalidad string, confirmacion ConfirmacionCurso) error {
	if confirmacion.Decision != "" && confirmacion.Decision != ConservarExactas && confirmacion.Decision != AceptarAlcance {
		return ErrDecisionPrioridadesCurso
	}
	nuevo, impacto, err := s.prepararEdicionCurso(ctx, cursoID, anio, division, modalidad)
	if err != nil {
		return err
	}
	if confirmacion.Version != "" && confirmacion.Version != impacto.Version {
		return ErrImpactoCursoDesactualizado
	}
	if len(impacto.Cambios) > 0 {
		if confirmacion.Version == "" {
			return ErrRevisarPrioridadesCurso
		}
		if confirmacion.Decision == "" {
			return ErrDecisionPrioridadesCurso
		}
		if confirmacion.Decision == ConservarExactas {
			if !impacto.PuedeConservarExactas {
				return ErrConflictoPrioridadesCurso
			}
			if err := s.marcas.ConservarPrioridadesDeCurso(ctx, cursoID, impacto.Propuesto); err != nil {
				return err
			}
		}
	}
	return s.repo.GuardarCurso(ctx, nuevo)
}
