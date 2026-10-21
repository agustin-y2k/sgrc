package http

import (
	"errors"
	"fmt"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/application"
	"github.com/gofiber/fiber/v2"
)

func TestMapearError_ConflictoDeSeguridad(t *testing.T) {
	err := mapearError(fmt.Errorf("actualizando perfil: %w", application.ErrUsuarioModificado))
	var respuesta *fiber.Error
	if !errors.As(err, &respuesta) || respuesta.Code != fiber.StatusConflict {
		t.Fatalf("una cuenta modificada debe responder 409: %v", err)
	}
}
