package http

import (
	"fmt"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/gofiber/fiber/v2"
)

func TestErrorCicloCerrado_RespondeConflicto(t *testing.T) {
	err := mapearError(fmt.Errorf("creando grupo: %w", application.ErrCicloCerrado))
	f, ok := err.(*fiber.Error)
	if !ok || f.Code != fiber.StatusConflict {
		t.Fatalf("esperaba HTTP 409: %v", err)
	}
}
