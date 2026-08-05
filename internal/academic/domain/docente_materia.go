package domain

import (
	"errors"
	"fmt"
)

// RolDocente informa si el vínculo es titular o suplente — puramente
// informativo, sin incumbencia en permisos del sistema (RF-02.6).
type RolDocente string

const (
	RolTitular  RolDocente = "TITULAR"
	RolSuplente RolDocente = "SUPLENTE"
)

var ErrRolDocenteInvalido = errors.New("rol de docente inválido")

func ParseRolDocente(s string) (RolDocente, error) {
	switch RolDocente(s) {
	case RolTitular, RolSuplente:
		return RolDocente(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrRolDocenteInvalido, s)
	}
}

// DocenteMateria vincula un usuario (docente) a una materia. Una materia
// puede tener más de un docente asignado (titular + suplente, o varios
// simultáneos) — RF-02.6.
type DocenteMateria struct {
	ID        string
	UsuarioID string
	MateriaID string
	Rol       RolDocente
}

func NuevoDocenteMateria(id, usuarioID, materiaID string, rol RolDocente) *DocenteMateria {
	return &DocenteMateria{ID: id, UsuarioID: usuarioID, MateriaID: materiaID, Rol: rol}
}
