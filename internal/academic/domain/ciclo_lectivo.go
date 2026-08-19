// Package domain contiene las entidades y reglas de negocio puras de academic
// — ciclos lectivos, cursos, materias y la asignación de docentes a materias.
package domain

import (
	"errors"
	"fmt"
)

// ErrAnioInvalido: un año fuera de un rango razonable (protege contra
// errores de tipeo como "20025" o "0", no contra ningún calendario real).
var ErrAnioInvalido = errors.New("año inválido")

// ErrCicloYaArchivado: no se puede archivar dos veces el mismo ciclo.
var ErrCicloYaArchivado = errors.New("el ciclo lectivo ya está archivado")

const (
	anioMinimo = 2000
	anioMaximo = 2100
)

// CicloLectivo es el año académico (RF-02.1).
type CicloLectivo struct {
	ID        string
	Anio      int
	Activo    bool
	Archivado bool
}

// NuevoCicloLectivo crea un ciclo válido, activo y sin archivar.
func NuevoCicloLectivo(id string, anio int) (*CicloLectivo, error) {
	if anio < anioMinimo || anio > anioMaximo {
		return nil, fmt.Errorf("%w: %d", ErrAnioInvalido, anio)
	}
	return &CicloLectivo{ID: id, Anio: anio, Activo: true, Archivado: false}, nil
}

// Archivar marca el ciclo como archivado y ya no activo (RF-02.4). No es
// reversible — un ciclo archivado no vuelve a activarse.
func (c *CicloLectivo) Archivar() error {
	if c.Archivado {
		return ErrCicloYaArchivado
	}
	c.Activo = false
	c.Archivado = true
	return nil
}
