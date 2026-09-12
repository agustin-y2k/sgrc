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

// ErrCicloArchivadoNoSeCorrige: archivar es la forma de cerrar un año, y un año
// cerrado es el registro de lo que pasó. Corregirle el año o borrarlo lo
// reescribiría — y el histórico de uso que el archivado dejó guardado está
// indexado por ESE año, así que cambiarlo dejaría las dos mitades hablando de
// años distintos.
var ErrCicloArchivadoNoSeCorrige = errors.New("un ciclo lectivo archivado no se puede corregir ni eliminar")

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

// CorregirAnio cambia el año del ciclo.
//
// Existe porque el ciclo era la única entidad del sistema sin corrección
// posible: creado con el año equivocado, ocupaba ese año para siempre —el
// índice es único— y encima se llevaba el único lugar de ciclo activo. La
// salida era entrar a la base.
//
// El dominio sólo valida el rango. Las dos condiciones que de verdad limitan
// esto no se pueden ver desde acá y viven en el servicio: que el ciclo no tenga
// reservas —sus fechas quedarían en el año viejo, y el ciclo diría que las
// clases fueron en otro— y que el año de destino no tenga bloqueos
// administrativos, que se atan al ciclo por el año de su fecha y cambiarían de
// dueño en silencio.
func (c *CicloLectivo) CorregirAnio(nuevo int) error {
	if c.Archivado {
		return ErrCicloArchivadoNoSeCorrige
	}
	if nuevo < anioMinimo || nuevo > anioMaximo {
		return fmt.Errorf("%w: %d", ErrAnioInvalido, nuevo)
	}
	c.Anio = nuevo
	return nil
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
