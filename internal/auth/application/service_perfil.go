package application

import (
	"context"
	"fmt"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
)

// Los datos propios: cómo se llama quien entró.

// ActualizarMisDatos cambia el nombre y el apellido de la propia cuenta. Es
// autoservicio y no pasa por ninguna aprobación: corregir cómo figura tu
// nombre es del mismo orden que cambiar tu foto de perfil.
//
// versionSesion viene de la sesión autenticada, para no renovar un token
// revocado entre la verificación HTTP y la lectura del usuario.
// Devuelve el usuario ya actualizado y un token nuevo (ver abajo por qué).
func (s *Service) operacionActualizarMisDatos(ctx context.Context, usuarioID, nombre, apellido string, recordarme bool, versionSesion int) (*domain.Usuario, string, error) {
	nombre, apellido, err := domain.NormalizarNombreYApellido(nombre, apellido)
	if err != nil {
		return nil, "", err
	}

	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, "", err
	}

	if !u.EstaAprobado() {
		return nil, "", motivoPorElQueNoEntra(u.Estado)
	}
	if u.VersionSesion != versionSesion {
		return nil, "", ErrUsuarioModificado
	}
	u, err = s.repo.ActualizarPerfil(ctx, u, nombre, apellido)
	if err != nil {
		return nil, "", fmt.Errorf("guardando los datos del perfil: %w", err)
	}

	// El JWT lleva nombre y apellido en los claims (RF-01.2). Hoy ningún
	// handler los lee —solo UserID y Rol—, así que un token con el nombre
	// viejo no rompe nada; aun así se firma uno nuevo, igual que hace
	// CambiarPassword, para que el token no siga afirmando algo que ya es
	// falso.
	token, err := s.firmar(u, recordarme)
	if err != nil {
		return nil, "", fmt.Errorf("firmando token nuevo: %w", err)
	}
	return u, token, nil
}
