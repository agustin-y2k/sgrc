package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Recuperación de contraseña por autoservicio (RF-01.10).

// SolicitarRecuperacionDePassword genera un código de un solo uso y lo manda
// al email de la persona.
func (s *Service) SolicitarRecuperacionDePassword(ctx context.Context, email string) error {
	if !s.correoHabilitado {
		return ErrRecuperacionNoDisponible
	}

	email = domain.NormalizarEmail(email)
	// Un email mal escrito sí se rechaza: no filtra nada —cualquiera ve que
	// "juan@" no es una dirección— y sin esto el typo más común termina en
	// "revisá tu casilla" sobre un mail que nunca salió.
	if err := domain.ValidarEmail(email); err != nil {
		return err
	}

	// El código se genera y se hashea antes de tocar la base, aunque quizás no
	// haya a quién mandárselo, por el tiempo de respuesta: el hash es argon2 y
	// cuesta cientos de milisegundos, muchísimo más que el resto de la
	// operación.
	codigo, err := s.generarCodigo()
	if err != nil {
		return fmt.Errorf("generando código de recuperación: %w", err)
	}
	codigoHash, err := s.hash(codigo)
	if err != nil {
		return fmt.Errorf("hasheando código de recuperación: %w", err)
	}

	u, err := s.repo.BuscarPorEmail(ctx, email)
	if errors.Is(err, ErrUsuarioNoEncontrado) {
		return nil
	}
	if err != nil {
		return err
	}

	if !u.EstaAprobado() {
		return nil
	}

	if !u.PuedeIngresarConPassword() {
		// Cuenta creada con Google: no hay contraseña que recuperar.
		s.bus.Publish(eventbus.Evento{
			Tipo:    "password.recuperacion.cuenta-google",
			Payload: eventbus.CuentaSoloConGoogle{Email: u.Email, Nombre: u.Nombre},
		})
		return nil
	}

	registro, err := domain.NuevoCodigoRecuperacion(s.nuevoID(), u.ID, codigoHash, s.ahora())
	if err != nil {
		return err
	}
	// Guardar invalida de paso los códigos anteriores: tiene que quedar uno
	// vigente, no dos con cinco intentos cada uno (ver
	// Repo.CrearCodigoRecuperacion).
	if err := s.repo.CrearCodigoRecuperacion(ctx, registro); err != nil {
		return err
	}

	// El correo no se manda desde acá: el bus publica de forma sincrónica en
	// esta goroutine, así que abrir la conexión SMTP adentro dejaría el request
	// esperando a Gmail.
	s.bus.Publish(eventbus.Evento{
		Tipo: "password.recuperacion.solicitada",
		Payload: eventbus.DatosDeRecuperacion{
			Email:             u.Email,
			Nombre:            u.Nombre,
			Codigo:            codigo,
			MinutosDeVigencia: int(domain.VigenciaCodigoRecuperacion.Minutes()),
		},
	})

	return nil
}

// RestablecerPasswordConCodigo cambia la contraseña de una cuenta con el
// código que se mandó al email de su dueño.
func (s *Service) RestablecerPasswordConCodigo(ctx context.Context, email, codigo, passwordNueva string) (string, error) {
	if !s.correoHabilitado {
		return "", ErrRecuperacionNoDisponible
	}

	email = domain.NormalizarEmail(email)
	// La contraseña se valida ANTES de mirar el código: si no, quien elige una
	// de cuatro caracteres se entera después de haber quemado el código y tiene
	// que pedir otro para volver a fallar.
	if len(passwordNueva) < minPasswordLen {
		return "", ErrPasswordCorta
	}

	u, err := s.repo.BuscarPorEmail(ctx, email)
	if errors.Is(err, ErrUsuarioNoEncontrado) {
		// Mismo tiempo y mismo mensaje que un código equivocado, o este
		// endpoint diría qué direcciones existen aunque el otro no lo diga.
		s.consumirTiempoDeVerificacion(codigo)
		return "", ErrCodigoRecuperacionInvalido
	}
	if err != nil {
		return "", err
	}
	if !u.EstaAprobado() || !u.PuedeIngresarConPassword() {
		s.consumirTiempoDeVerificacion(codigo)
		return "", ErrCodigoRecuperacionInvalido
	}

	registro, err := s.repo.BuscarCodigoVigenteDe(ctx, u.ID)
	if errors.Is(err, ErrCodigoNoEncontrado) {
		s.consumirTiempoDeVerificacion(codigo)
		return "", ErrCodigoRecuperacionInvalido
	}
	if err != nil {
		return "", err
	}

	if err := registro.Utilizable(s.ahora()); err != nil {
		return "", traducirErrorDeCodigo(err)
	}

	coincide, err := s.verify(codigo, registro.CodigoHash)
	if err != nil {
		return "", fmt.Errorf("verificando código de recuperación: %w", err)
	}
	if !coincide {
		quemado := registro.RegistrarFallo(s.ahora())
		// El error de guardar tiene prioridad sobre el del código: si el contador
		// no se puede persistir, el tope de intentos no existe y el código queda
		// abierto a fuerza bruta.
		if err := s.repo.GuardarCodigoRecuperacion(ctx, registro); err != nil {
			return "", fmt.Errorf("registrando intento fallido: %w", err)
		}
		if quemado {
			return "", ErrCodigoRecuperacionSinIntentos
		}
		return "", ErrCodigoRecuperacionInvalido
	}

	passwordHash, err := s.hash(passwordNueva)
	if err != nil {
		return "", fmt.Errorf("hasheando la contraseña nueva: %w", err)
	}

	// Las dos escrituras van juntas o no van: sueltas, cualquiera de los dos
	// órdenes deja un estado malo.
	err = s.repo.EnTransaccion(ctx, func(repo Repo) error {
		if err := registro.Usar(s.ahora()); err != nil {
			return traducirErrorDeCodigo(err)
		}
		if err := repo.GuardarCodigoRecuperacion(ctx, registro); err != nil {
			return err
		}

		u.PasswordHash = passwordHash
		u.DebeCambiarPassword = false
		// Las sesiones abiertas se cierran, y acá no hay token nuevo que preservar:
		// este endpoint no inicia sesión a propósito.
		u.InvalidarSesiones()
		return repo.Guardar(ctx, u)
	})
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

// traducirErrorDeCodigo pasa los errores del dominio a los sentinels que
// interfaces/http sabe mapear.
func traducirErrorDeCodigo(err error) error {
	switch {
	case errors.Is(err, domain.ErrCodigoExpirado):
		return ErrCodigoRecuperacionVencido
	case errors.Is(err, domain.ErrDemasiadosIntentos):
		return ErrCodigoRecuperacionSinIntentos
	default:
		return ErrCodigoRecuperacionInvalido
	}
}
