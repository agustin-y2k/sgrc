package http

import (
	"errors"
	"fmt"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/gofiber/fiber/v2"
)

func TestEscrituraObsoletaDeReserva_RespondeConflicto(t *testing.T) {
	err := mapearError(fmt.Errorf("guardando reserva: %w", application.ErrReservaModificada))
	var httpErr *fiber.Error
	if !errors.As(err, &httpErr) || httpErr.Code != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuvo %v", err)
	}
}
