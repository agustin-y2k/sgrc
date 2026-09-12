// Package domain es la lectura del registro de auditoría: qué es una entrada y
// qué se puede preguntarle.
//
// La ESCRITURA vive en internal/shared/audit y se queda ahí. No es una
// distracción: auditar es transversal —lo hace cada módulo, como loguear—, así
// que el puerto de escritura tiene que estar donde todos lo alcancen sin
// importarse entre sí. Leer, en cambio, es una función más del sistema, con su
// pantalla y sus permisos, y le corresponde un módulo propio como a cualquier
// otra.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// MaxDiasDeRango es el ancho máximo de una consulta con fechas. Es un tope de
// sanidad, no una regla: existe para que un rango tipeado mal —«desde 1970»— no
// se lleve la tabla entera por delante.
const MaxDiasDeRango = 366

var (
	ErrRangoInvertido = errors.New("la fecha «desde» tiene que ser anterior a la fecha «hasta»")
	ErrRangoLargo     = fmt.Errorf("el rango no puede abarcar más de %d días", MaxDiasDeRango)
)

// Entrada es una fila del registro, lista para mostrar.
//
// Lo que la distingue de audit.Entrada (la de escritura) es que trae resuelto
// el nombre de quien hizo la acción. Y que ese nombre puede FALTAR: `usuario_id`
// no tiene clave foránea a propósito, para que lo que hizo una cuenta sobreviva
// a su eliminación (RF-01.9). Una entrada cuyo actor ya no existe sigue siendo
// la respuesta correcta a «¿quién borró esto?», sólo que la contesta con un
// identificador en vez de con un nombre.
type Entrada struct {
	ID        string
	UsuarioID string
	// ActorNombre es "Nombre Apellido", o vacío si esa cuenta ya se eliminó.
	ActorNombre string
	Accion      string
	Entidad     string
	EntidadID   string
	// Detalle es el JSON tal como se guardó. No se interpreta acá: cada acción
	// guarda lo suyo, y darle forma a eso en el dominio obligaría a conocer las
	// treinta acciones del catálogo y a tocarlo con cada una nueva.
	Detalle  []byte
	IPOrigen string
	CreadoEn time.Time
}

// Filtro es lo que se le puede preguntar al registro. Todos los campos son
// opcionales y se combinan con Y: sin ninguno, devuelve lo último que pasó.
type Filtro struct {
	// Accion y Entidad son coincidencia exacta: son valores de un catálogo
	// cerrado que el propio sistema escribe, no texto que alguien tipea.
	Accion  string
	Entidad string
	// EntidadID contesta «¿quién tocó esta cosa?», que es la pregunta que se
	// hace cuando un equipo aparece con algo cambiado.
	EntidadID string
	// UsuarioID contesta «¿qué hizo esta persona?».
	UsuarioID string
	// Desde y Hasta acotan por fecha. Hasta es INCLUSIVO del día entero: quien
	// escribe "hasta el 5" espera que entre lo que pasó el 5 a las 23:00, y la
	// conversión a un límite exclusivo la hace quien arma el filtro.
	Desde *time.Time
	Hasta *time.Time
}

// Validar comprueba lo único que puede estar mal en un filtro: el rango.
func (f Filtro) Validar() error {
	if f.Desde == nil || f.Hasta == nil {
		return nil
	}
	if f.Hasta.Before(*f.Desde) {
		return ErrRangoInvertido
	}
	if f.Hasta.Sub(*f.Desde) > MaxDiasDeRango*24*time.Hour {
		return ErrRangoLargo
	}
	return nil
}
