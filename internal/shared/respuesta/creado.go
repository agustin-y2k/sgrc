// Package respuesta reúne lo que todas las capas HTTP contestan igual.
//
// Vive en shared/ y no en cada módulo por el mismo motivo que
// shared/paginacion: es una sola regla del contrato de la API, y escrita nueve
// veces se despega de a poco —una que pone la cabecera, otra que la olvida—
// hasta que deja de ser una regla.
package respuesta

import "github.com/gofiber/fiber/v2"

// Creado contesta un 201 con el recurso en el cuerpo y su dirección en
// `Location`.
//
// El `Location` es la otra mitad del contrato de un 201: sin él, el cliente
// sabe que se creó algo y no dónde quedó. Faltaba en las 57 respuestas de
// creación del sistema, y no por descuido sino porque no había adónde apuntar
// —ningún recurso tenía GET individual—. Con esos GET ya existiendo, la
// cabecera pasó a poder decir la verdad.
//
// La ruta se arma con el prefijo del recurso y el id. Se pasa el prefijo y no
// la URL entera para que quede claro, al leer el handler, que lo que se publica
// es LA MISMA ruta que después sirve el GET: si una de las dos cambia y la otra
// no, se ve en el mismo archivo.
func Creado(c *fiber.Ctx, prefijo, id string, cuerpo any) error {
	c.Location(prefijo + "/" + id)
	return c.Status(fiber.StatusCreated).JSON(cuerpo)
}
