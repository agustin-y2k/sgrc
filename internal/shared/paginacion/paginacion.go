// Package paginacion define la ventana de resultados que un listado
// devuelve, para que los tres endpoints que pueden crecer sin límite
// —reservas, notificaciones y usuarios— la interpreten igual.
//
// Existe como paquete compartido y no como un tipo por módulo por la misma
// razón que shared/middleware: es contrato de la API (los mismos parámetros
// de query, los mismos límites, el mismo `meta` en la respuesta), no una
// regla de negocio de ninguno de los dominios. Duplicarlo tres veces es cómo
// tres endpoints terminan con tres tamaños máximos distintos.
//
// Los listados acotados por el propio dominio no se paginan: ciclos, cursos,
// materias, carros y PCs los crea un Admin a mano y son decenas de filas por
// año. Paginarlos sería agregar parámetros que nadie va a usar y complicar
// las pantallas que hoy los muestran completos.
package paginacion

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	// TamanioPorDefecto es lo que devuelve un listado cuando el cliente no
	// pide nada. Entra cómodo en una pantalla con scroll y evita que un
	// cliente viejo —que no manda los parámetros— se traiga el año entero.
	TamanioPorDefecto = 50

	// TamanioMaximo es el techo que puede pedir un cliente. Sin techo, el
	// parámetro no protege de nada: alcanza con pedir pageSize=100000 para
	// volver al listado sin cota que esto vino a arreglar.
	TamanioMaximo = 200
)

var (
	ErrPaginaInvalida  = errors.New("el número de página tiene que ser un entero mayor o igual a 1")
	ErrTamanioInvalido = fmt.Errorf("el tamaño de página tiene que ser un entero entre 1 y %d", TamanioMaximo)
)

// Pagina es una ventana de resultados. Numero es 1-based porque es lo que
// viaja en la query string y lo que se muestra en pantalla; el desplazamiento
// 0-based que necesita SQL lo calcula Offset.
type Pagina struct {
	Numero  int
	Tamanio int
}

// PorDefecto es la ventana que se usa cuando el cliente no pide ninguna.
func PorDefecto() Pagina {
	return Pagina{Numero: 1, Tamanio: TamanioPorDefecto}
}

// Parsear interpreta los parámetros crudos `page` y `pageSize` de la query
// string. Los dos vacíos dan la ventana por defecto: el endpoint sigue
// funcionando sin que el cliente sepa que existe la paginación.
//
// Un valor mal formado es un error y no un default silencioso: quien manda
// `page=abc` está pidiendo algo distinto de lo que va a recibir, y devolverle
// la primera página como si nada es cómo un bug de paginación en el cliente
// pasa desapercibido.
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

// Meta es el campo `meta` de un listado paginado. Vive acá y no en el dto.go
// de cada paquete para que los tres endpoints devuelvan exactamente la misma
// forma: el cliente tiene un solo tipo que entender, y agregar un campo
// (ej. totalPages) no deja a dos de tres endpoints atrás.
//
// Total es el total de filas que matchean el filtro, no las de esta página:
// es lo único con lo que el cliente puede saber si hay una siguiente.
type Meta struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func (p Pagina) Meta(total int) Meta {
	return Meta{Total: total, Page: p.Numero, PageSize: p.Tamanio}
}
