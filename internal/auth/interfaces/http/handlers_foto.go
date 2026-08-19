package http

import (
	"errors"
	"io"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// La foto de perfil.
//
// Sube y borra cada quien la suya; verla puede cualquier usuario
// autenticado, porque las fotos aparecen al lado del nombre en pantallas
// compartidas (quién tiene una máquina, quién dicta una materia). Sin sesión
// no se sirve ninguna: son fotos de personas de una escuela, no assets
// públicos.

// PUT /api/auth/mi-foto — multipart con el campo `foto`.
func (h *Handler) SubirMiFoto(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}

	archivo, err := c.FormFile("foto")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "hay que mandar una imagen en el campo «foto»")
	}
	// El tamaño se mira ANTES de leer el archivo: sin esto, un envío de
	// 500 MB se copia entero a memoria para recién después rechazarlo.
	if archivo.Size > domain.MaxBytesFoto {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, domain.ErrFotoGrande.Error())
	}

	f, err := archivo.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "no se pudo leer la imagen")
	}
	defer f.Close()

	// LimitReader y no io.ReadAll pelado: `archivo.Size` lo declara quien
	// sube, así que el tope real lo tiene que poner la lectura.
	contenido, err := io.ReadAll(io.LimitReader(f, domain.MaxBytesFoto+1))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "no se pudo leer la imagen")
	}

	foto, err := h.svc.GuardarMiFoto(c.UserContext(), claims.UserID, contenido)
	if err != nil {
		return mapearError(err)
	}
	return c.JSON(fiber.Map{
		"tipo":          foto.Tipo,
		"actualizadaEn": foto.ActualizadaEn,
	})
}

// GET /api/auth/usuarios/{id}/foto — devuelve la imagen.
func (h *Handler) VerFoto(c *fiber.Ctx) error {
	foto, err := h.svc.BuscarFoto(c.UserContext(), c.Params("id"))
	if err != nil {
		if errors.Is(err, domain.ErrFotoNoExiste) {
			// 404 y no una imagen por defecto: quien dibuja el avatar es la
			// interfaz, que ya sabe hacer las iniciales. Mandar un PNG gris
			// desde acá le quitaría la posibilidad de elegir.
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return mapearError(err)
	}

	// El ETag hace que el navegador no vuelva a bajar la misma foto en cada
	// pantalla que la muestre. Es la fecha de actualización: cambia solo
	// cuando la persona cambia la imagen.
	c.Set(fiber.HeaderETag, `"`+foto.ActualizadaEn.UTC().Format("20060102150405")+`"`)
	// Privado: es la cara de una persona. Sin esto, un proxy compartido
	// podría guardarla y servírsela a otro.
	c.Set(fiber.HeaderCacheControl, "private, max-age=300")
	c.Set(fiber.HeaderContentType, foto.Tipo)
	return c.Send(foto.Contenido)
}

// DELETE /api/auth/mi-foto — vuelve a las iniciales.
func (h *Handler) EliminarMiFoto(c *fiber.Ctx) error {
	claims, err := claimsDelContexto(c)
	if err != nil {
		return err
	}
	if err := h.svc.EliminarMiFoto(c.UserContext(), claims.UserID); err != nil {
		return mapearError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
