// Package paginacion define la ventana de resultados que un listado devuelve,
// para que los tres endpoints que pueden crecer sin límite —reservas,
// notificaciones y usuarios— la interpreten igual.
package paginacion

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	// TamanioPorDefecto es lo que devuelve un listado cuando el cliente no pide
	// nada.
	TamanioPorDefecto = 50

	// TamanioMaximo es el techo que puede pedir un cliente.
	TamanioMaximo = 200
)

var (
	ErrPaginaInvalida  = errors.New("el número de página tiene que ser un entero mayor o igual a 1")
	ErrTamanioInvalido = fmt.Errorf("el tamaño de página tiene que ser un entero entre 1 y %d", TamanioMaximo)
)

// Pagina es una ventana de resultados.
type Pagina struct {
	Numero  int
	Tamanio int
}

// PorDefecto es la ventana que se usa cuando el cliente no pide ninguna.
func PorDefecto() Pagina {
	return Pagina{Numero: 1, Tamanio: TamanioPorDefecto}
}

// Parsear interpreta los parámetros crudos `page` y `pageSize` de la query
// string.
func Parsear(page, pageSize string) (Pagina, error) {
	p := PorDefecto()

	if page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 {
			return Pagina{}, fmt.Errorf("%w: %q", ErrPaginaInvalida, page)
		}
		p.Numero = n
	}

	if pageSize != "" {
		n, err := strconv.Atoi(pageSize)
		if err != nil || n < 1 || n > TamanioMaximo {
			return Pagina{}, fmt.Errorf("%w: %q", ErrTamanioInvalido, pageSize)
		}
		p.Tamanio = n
	}

	return p, nil
}

// Limit y Offset son lo que va directo al SQL.
func (p Pagina) Limit() int { return p.Tamanio }

func (p Pagina) Offset() int { return (p.Numero - 1) * p.Tamanio }

// Meta es el campo `meta` de un listado paginado.
type Meta struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func (p Pagina) Meta(total int) Meta {
	return Meta{Total: total, Page: p.Numero, PageSize: p.Tamanio}
}
