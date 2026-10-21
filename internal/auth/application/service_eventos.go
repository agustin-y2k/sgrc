package application

import (
	"context"
	"github.com/agustin-y2k/sgrc/internal/auth/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

func (s *Service) Registrar(ctx context.Context, nombre, apellido, email, password string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	var r0 *domain.Usuario
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionRegistrar(ctx, nombre, apellido, email, password, solicitud)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) CrearAdmin(ctx context.Context, creadoPorID, nombre, apellido, email, password string) (*domain.Usuario, error) {
	var r0 *domain.Usuario
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionCrearAdmin(ctx, creadoPorID, nombre, apellido, email, password)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) LoginConGoogle(ctx context.Context, idToken string, recordarme bool) (*LoginResultado, error) {
	var r0 *LoginResultado
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionLoginConGoogle(ctx, idToken, recordarme)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) RegistrarConGoogle(ctx context.Context, idToken, nombre, apellido string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	var r0 *domain.Usuario
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionRegistrarConGoogle(ctx, idToken, nombre, apellido, solicitud)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) Aprobar(ctx context.Context, usuarioID, actorID string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionAprobar(ctx, usuarioID, actorID) })
}
func (s *Service) Rechazar(ctx context.Context, usuarioID string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionRechazar(ctx, usuarioID) })
}
func (s *Service) ResetearPassword(ctx context.Context, usuarioID string) (string, error) {
	var r0 string
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionResetearPassword(ctx, usuarioID)
		return err
	})
	if err != nil {
		return "", err
	}
	return r0, nil
}
func (s *Service) CambiarPassword(ctx context.Context, usuarioID, passwordActual, passwordNueva string, recordarme bool) (string, error) {
	var r0 string
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionCambiarPassword(ctx, usuarioID, passwordActual, passwordNueva, recordarme)
		return err
	})
	if err != nil {
		return "", err
	}
	return r0, nil
}
func (s *Service) PromoverAAdmin(ctx context.Context, usuarioID string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionPromoverAAdmin(ctx, usuarioID) })
}
func (s *Service) DegradarADocente(ctx context.Context, usuarioID, solicitanteID string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionDegradarADocente(ctx, usuarioID, solicitanteID) })
}
func (s *Service) EliminarDefinitivamente(ctx context.Context, usuarioID string) (*ResultadoEliminacion, error) {
	var r0 *ResultadoEliminacion
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionEliminarDefinitivamente(ctx, usuarioID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) SolicitarRecuperacionDePassword(ctx context.Context, email string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionSolicitarRecuperacionDePassword(ctx, email) })
}
func (s *Service) ActualizarMisDatos(ctx context.Context, usuarioID, nombre, apellido string, recordarme bool, versionSesion int) (*domain.Usuario, string, error) {
	var r0 *domain.Usuario
	var r1 string
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, r1, err = s.operacionActualizarMisDatos(ctx, usuarioID, nombre, apellido, recordarme, versionSesion)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return r0, r1, nil
}
func (s *Service) GuardarMiFoto(ctx context.Context, usuarioID string, contenido []byte) (*domain.FotoDePerfil, error) {
	var r0 *domain.FotoDePerfil
	err := eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error {
		var err error
		r0, err = s.operacionGuardarMiFoto(ctx, usuarioID, contenido)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r0, nil
}
func (s *Service) EliminarMiFoto(ctx context.Context, usuarioID string) error {
	return eventbus.EnOperacion(ctx, s.bus, func(ctx context.Context) error { return s.operacionEliminarMiFoto(ctx, usuarioID) })
}
