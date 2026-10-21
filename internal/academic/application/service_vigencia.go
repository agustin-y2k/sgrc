package application

import (
	"context"

	"github.com/agustin-y2k/sgrc/internal/academic/domain"
)

func (s *Service) CrearCiclo(ctx context.Context, anio int) (*domain.CicloLectivo, error) {
	var res *domain.CicloLectivo
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.crearCiclo(ctx, anio)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) CorregirAnioDeCiclo(ctx context.Context, cicloID string, nuevoAnio int) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.corregirAnioDeCiclo(ctx, cicloID, nuevoAnio) })
}

func (s *Service) EliminarCiclo(ctx context.Context, cicloID string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.eliminarCiclo(ctx, cicloID) })
}

func (s *Service) ArchivarYClonar(ctx context.Context, cicloID string, clonarAAnio *int) (*ResultadoArchivado, error) {
	var res *ResultadoArchivado
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.archivarYClonar(ctx, cicloID, clonarAAnio)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) CrearCurso(ctx context.Context, cicloLectivoID string, anio int, division, modalidad string) (*domain.Curso, error) {
	var res *domain.Curso
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.crearCurso(ctx, cicloLectivoID, anio, division, modalidad)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) EditarCurso(ctx context.Context, cursoID string, anio int, division, modalidad string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.editarCurso(ctx, cursoID, anio, division, modalidad) })
}

func (s *Service) EliminarCurso(ctx context.Context, cursoID string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.eliminarCurso(ctx, cursoID) })
}

func (s *Service) CrearMateria(ctx context.Context, cursoID, nombre string) (*domain.Materia, error) {
	var res *domain.Materia
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.crearMateria(ctx, cursoID, nombre)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) EditarMateria(ctx context.Context, materiaID, nuevoNombre string) (int, error) {
	var res int
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.editarMateria(ctx, materiaID, nuevoNombre)
		return err
	})
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (s *Service) EliminarMateria(ctx context.Context, materiaID string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.eliminarMateria(ctx, materiaID) })
}

func (s *Service) CrearEspacio(ctx context.Context, cicloLectivoID, nombre string) (*domain.Espacio, error) {
	var res *domain.Espacio
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.crearEspacio(ctx, cicloLectivoID, nombre)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) EditarEspacio(ctx context.Context, id, nombre string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.editarEspacio(ctx, id, nombre) })
}

func (s *Service) EliminarEspacio(ctx context.Context, id string) error {
	return s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error { return s.eliminarEspacio(ctx, id) })
}

func (s *Service) ImportarEstructura(ctx context.Context, cicloID string, cursos []CursoConMaterias) (*ResultadoImportacion, error) {
	var res *ResultadoImportacion
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.importarEstructura(ctx, cicloID, cursos)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) CopiarMaterias(ctx context.Context, cursoOrigenID string, cursosDestinoIDs []string) (*ResultadoCopia, error) {
	var res *ResultadoCopia
	err := s.repo.CoordinarAsignaciones(ctx, func(ctx context.Context) error {
		var err error
		res, err = s.copiarMaterias(ctx, cursoOrigenID, cursosDestinoIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
