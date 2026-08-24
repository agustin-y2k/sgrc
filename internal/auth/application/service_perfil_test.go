package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramiro/sgrc/internal/auth/domain"
)

// ── ActualizarMisDatos ──────────────────────────────────────────────────

func TestActualizarMisDatos_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ada", Apellido: "Byron"}
	svc := nuevoServicioDeTest(repo)

	u, token, err := svc.ActualizarMisDatos(context.Background(), "u1", "Ada", "Lovelace", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Apellido != "Lovelace" {
		t.Errorf("esperaba el apellido nuevo, obtuve %q", u.Apellido)
	}
	if repo.usuarios["u1"].Apellido != "Lovelace" {
		t.Error("el cambio tiene que quedar guardado, no solo en el objeto devuelto")
	}
	// El token viejo sigue diciendo "Ada Byron" en los claims. Ningún handler
	// los lee, pero un token que afirma algo falso no se devuelve.
	if token == "" {
		t.Error("debería devolver un token nuevo con el nombre corregido")
	}
}

func TestActualizarMisDatos_RecortaEspacios(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1"}
	svc := nuevoServicioDeTest(repo)

	u, _, err := svc.ActualizarMisDatos(context.Background(), "u1", "  Ada  ", "  Lovelace ", false)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Nombre != "Ada" || u.Apellido != "Lovelace" {
		t.Errorf("esperaba los espacios recortados, obtuve %q %q", u.Nombre, u.Apellido)
	}
}

// Un nombre de solo espacios queda vacío después del trim: es lo mismo que no
// mandar nada, y no puede dejar a alguien sin nombre en las reservas.
func TestActualizarMisDatos_Vacio_Error(t *testing.T) {
	casos := []struct {
		caso     string
		nombre   string
		apellido string
	}{
		{"sin nombre", "", "Lovelace"},
		{"sin apellido", "Ada", ""},
		{"nombre de solo espacios", "   ", "Lovelace"},
	}
	for _, c := range casos {
		t.Run(c.caso, func(t *testing.T) {
			repo := nuevoFakeRepo()
			repo.usuarios["u1"] = &domain.Usuario{ID: "u1", Nombre: "Ada", Apellido: "Byron"}
			svc := nuevoServicioDeTest(repo)

			_, _, err := svc.ActualizarMisDatos(context.Background(), "u1", c.nombre, c.apellido, false)

			if !errors.Is(err, domain.ErrNombreVacio) {
				t.Fatalf("esperaba ErrNombreVacio, obtuve %v", err)
			}
			if repo.usuarios["u1"].Apellido != "Byron" {
				t.Error("un pedido inválido no puede tocar lo que ya estaba guardado")
			}
		})
	}
}

// La columna es VARCHAR(100): sin esta validación, el largo lo cortaba
// solamente zod en el navegador y un cliente que no fuera el formulario hacía
// fallar el INSERT con un 500.
func TestActualizarMisDatos_DemasiadoLargo_Error(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1"}
	svc := nuevoServicioDeTest(repo)

	_, _, err := svc.ActualizarMisDatos(context.Background(), "u1",
		strings.Repeat("a", domain.LargoMaxNombre+1), "Lovelace", false)

	if !errors.Is(err, domain.ErrNombreDemasiadoLargo) {
		t.Fatalf("esperaba ErrNombreDemasiadoLargo, obtuve %v", err)
	}
}

// Justo en el límite entra — y con eñes, que ocupan más bytes que letras.
func TestActualizarMisDatos_EnElLimite_OK(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.usuarios["u1"] = &domain.Usuario{ID: "u1"}
	svc := nuevoServicioDeTest(repo)

	nombre := strings.Repeat("ñ", domain.LargoMaxNombre)
	if _, _, err := svc.ActualizarMisDatos(context.Background(), "u1", nombre, "Lovelace", false); err != nil {
		t.Fatalf("%d caracteres tienen que entrar: %v", domain.LargoMaxNombre, err)
	}
}

func TestActualizarMisDatos_UsuarioInexistente_Error(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, _, err := svc.ActualizarMisDatos(context.Background(), "fantasma", "Ada", "Lovelace", false)

	if !errors.Is(err, ErrUsuarioNoEncontrado) {
		t.Fatalf("esperaba ErrUsuarioNoEncontrado, obtuve %v", err)
	}
}
