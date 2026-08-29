package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/secretos"
)

// ── Las cuentas de usuario de cada equipo (RF-03.22) ─────────────────────
//
// Una notebook no se abre sola: hay que saber con qué cuenta entrar. Cargarlas
// es opcional; un equipo sin cuentas es un equipo del que no anotamos nada.
//
// El reparto de responsabilidades acá es la parte que importa:
//
//   - El DOMINIO valida la forma (nombre no vacío, privilegio conocido, la
//     contradicción de "libre pero con contraseña").
//   - Este paquete cifra y descifra, porque es el que tiene el cifrador.
//   - Y este paquete decide QUIÉN puede ver una contraseña. Es una regla de
//     negocio, no de presentación: si viviera en el handler, una ruta nueva
//     podría devolver la contraseña sin pasar por ella.

// CuentaVisible es una cuenta tal como se le muestra a alguien en particular.
//
// La contraseña NO viaja en el listado, ni siquiera para un ADMIN: se pide
// aparte, de a una, y esa petición es la que queda auditada. Si viajara acá,
// abrir la ficha de un equipo sería revelar todas sus contraseñas de una vez y
// la auditoría no distinguiría "miró la lista" de "necesitaba entrar a esta
// máquina".
type CuentaVisible struct {
	*domain.CuentaDeEquipo
	// PuedeVerLaPassword dice si QUIEN PIDIÓ esta lista puede revelar la
	// contraseña de esta cuenta. Lo resuelve el servidor y no la pantalla: el
	// frontend solo dibuja el botón, no decide.
	PuedeVerLaPassword bool
	// HayPasswordParaVer distingue el tercer estado: la cuenta pide contraseña
	// pero no la tenemos anotada, así que no hay nada que revelar aunque quien
	// mira tenga permiso.
	HayPasswordParaVer bool
}

// puedeRevelar es la regla, en un solo lugar: la contraseña de una cuenta
// PUBLICA la ve cualquiera con sesión, y la de una SOLO_ADMIN únicamente un
// ADMIN. El privilegio de la cuenta no entra en la cuenta: una cuenta de
// administrador puede ser pública y una común puede ser reservada.
func puedeRevelar(c *domain.CuentaDeEquipo, esAdmin bool) bool {
	return esAdmin || c.PuedeVerlaCualquiera()
}

// ListarCuentasDeEquipo devuelve las cuentas de un equipo con lo que quien
// pregunta tiene permitido hacer con cada una. La cuenta y su privilegio se
// listan SIEMPRE: saber que una notebook tiene una cuenta de administrador es
// útil aunque no puedas usarla, y esconderla haría que el inventario mienta
// por omisión.
func (s *Service) ListarCuentasDeEquipo(ctx context.Context, equipoID string, esAdmin bool) ([]CuentaVisible, error) {
	cuentas, err := s.repo.ListarCuentasDeEquipo(ctx, equipoID)
	if err != nil {
		return nil, err
	}

	visibles := make([]CuentaVisible, 0, len(cuentas))
	for _, c := range cuentas {
		visibles = append(visibles, CuentaVisible{
			CuentaDeEquipo:     c,
			PuedeVerLaPassword: puedeRevelar(c, esAdmin),
			HayPasswordParaVer: c.HayPasswordGuardada(),
		})
	}
	return visibles, nil
}

// ClasesDeCuentaUsadas alimenta las sugerencias del formulario.
func (s *Service) ClasesDeCuentaUsadas(ctx context.Context) ([]string, error) {
	return s.repo.ClasesDeCuentaUsadas(ctx)
}

// cifrarPassword traduce el "no hay clave configurada" a un error de esta capa
// para que el handler lo pueda mapear a un 503 con una explicación, en vez de
// a un 500 sin ninguna.
func (s *Service) cifrarPassword(password string, tienePassword bool) (string, error) {
	if err := domain.PasswordDeCuentaValida(password, tienePassword); err != nil {
		return "", err
	}
	if password == "" {
		return "", nil
	}
	cifrada, err := s.cifrador.Cifrar(password)
	if err != nil {
		if errors.Is(err, secretos.ErrSinClave) {
			return "", ErrSinClaveDeCifrado
		}
		return "", fmt.Errorf("cifrando la contraseña de la cuenta: %w", err)
	}
	return cifrada, nil
}

// CrearCuentaDeEquipo registra una cuenta en un equipo. `password` viene en
// claro y no sale de acá sin cifrar.
func (s *Service) CrearCuentaDeEquipo(ctx context.Context, equipoID string, datos domain.DatosDeCuenta, password string) (*domain.CuentaDeEquipo, error) {
	// Que el equipo exista se comprueba antes de cifrar nada: si no, un
	// equipoID equivocado igual haría el trabajo de cifrado para después
	// rebotar contra la clave foránea.
	if _, err := s.repo.BuscarEquipoPorID(ctx, equipoID); err != nil {
		return nil, err
	}

	cifrada, err := s.cifrarPassword(password, datos.TienePassword)
	if err != nil {
		return nil, err
	}

	cuenta, err := domain.NuevaCuentaDeEquipo(s.nuevoID(), equipoID, datos, cifrada, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.CrearCuentaDeEquipo(ctx, cuenta); err != nil {
		return nil, err
	}
	return cuenta, nil
}

// EditarCuentaParams son los campos editables. Todos punteros: nil es "no
// tocar", que es lo que permite cambiar solo la visibilidad sin mandar de
// nuevo la contraseña.
type EditarCuentaParams struct {
	Usuario       *string
	Clase         *string
	Privilegio    *domain.PrivilegioDeCuenta
	Visibilidad   *domain.VisibilidadDeCuenta
	TienePassword *bool
	Notas         *string
	// Password nil = dejar la que estaba. Cadena vacía = borrar la anotada,
	// que es distinto: la cuenta puede seguir pidiendo contraseña y nosotros
	// pasar a no saberla, por ejemplo cuando alguien la cambió en la máquina.
	Password *string
}

func (s *Service) EditarCuentaDeEquipo(ctx context.Context, cuentaID string, params EditarCuentaParams) (*domain.CuentaDeEquipo, error) {
	cuenta, err := s.repo.BuscarCuentaDeEquipoPorID(ctx, cuentaID)
	if err != nil {
		return nil, err
	}

	datos := domain.DatosDeCuenta{
		Usuario:       cuenta.Usuario,
		Clase:         cuenta.Clase,
		Privilegio:    cuenta.Privilegio,
		Visibilidad:   cuenta.Visibilidad,
		TienePassword: cuenta.TienePassword,
		Notas:         cuenta.Notas,
	}
	if params.Usuario != nil {
		datos.Usuario = *params.Usuario
	}
	if params.Clase != nil {
		datos.Clase = *params.Clase
	}
	if params.Privilegio != nil {
		datos.Privilegio = *params.Privilegio
	}
	if params.Visibilidad != nil {
		datos.Visibilidad = *params.Visibilidad
	}
	if params.TienePassword != nil {
		datos.TienePassword = *params.TienePassword
	}
	if params.Notas != nil {
		datos.Notas = *params.Notas
	}

	datos, err = datos.Validar()
	if err != nil {
		return nil, err
	}

	cifrada := cuenta.PasswordCifrada
	switch {
	case params.Password != nil:
		// Se manda una contraseña nueva, o el vacío para borrar la anotada.
		cifrada, err = s.cifrarPassword(*params.Password, datos.TienePassword)
		if err != nil {
			return nil, err
		}
	case !datos.TienePassword:
		// Pasar una cuenta a "libre" sin mandar contraseña tiene que soltar la
		// que estaba guardada: si no, quedaría una contraseña cifrada colgando
		// de una cuenta que dice no tener ninguna — justo la contradicción que
		// la base rechaza.
		cifrada = ""
	}

	cuenta.Usuario = datos.Usuario
	cuenta.Clase = datos.Clase
	cuenta.Privilegio = datos.Privilegio
	cuenta.Visibilidad = datos.Visibilidad
	cuenta.TienePassword = datos.TienePassword
	cuenta.Notas = datos.Notas
	cuenta.PasswordCifrada = cifrada
	cuenta.ActualizadaEn = s.ahora()

	if err := s.repo.GuardarCuentaDeEquipo(ctx, cuenta); err != nil {
		return nil, err
	}
	return cuenta, nil
}

func (s *Service) BorrarCuentaDeEquipo(ctx context.Context, cuentaID string) error {
	return s.repo.BorrarCuentaDeEquipo(ctx, cuentaID)
}

// RevelarPasswordDeCuenta devuelve la contraseña en claro, si quien pregunta
// puede verla. Es una operación aparte del listado a propósito: así queda una
// petición por contraseña efectivamente mirada, que es lo que la auditoría
// necesita registrar.
//
// Devuelve también la cuenta para que el handler pueda auditar de qué equipo y
// de qué usuario se trataba sin volver a buscarla.
func (s *Service) RevelarPasswordDeCuenta(ctx context.Context, cuentaID string, esAdmin bool) (*domain.CuentaDeEquipo, string, error) {
	cuenta, err := s.repo.BuscarCuentaDeEquipoPorID(ctx, cuentaID)
	if err != nil {
		return nil, "", err
	}

	if !puedeRevelar(cuenta, esAdmin) {
		return nil, "", ErrNoAutorizado
	}
	if !cuenta.HayPasswordGuardada() {
		return nil, "", ErrPasswordNoGuardada
	}

	password, err := s.cifrador.Descifrar(cuenta.PasswordCifrada)
	if err != nil {
		if errors.Is(err, secretos.ErrSinClave) {
			return nil, "", ErrSinClaveDeCifrado
		}
		// Lo guardado no se pudo descifrar: casi siempre es CUENTAS_SECRET
		// cambiada. Se dice tal cual, porque la salida es volver a cargar la
		// contraseña y no hay nada que el sistema pueda hacer solo.
		//
		// Va envuelto en ErrPasswordIlegible y no como error suelto: así el
		// mapeo HTTP lo reconoce y contesta con esta explicación. Suelto caía
		// al 500 "error interno", que no le dice nada a nadie ni deja rastro en
		// el log. La causa original se conserva en la cadena para quien lea el
		// error completo.
		return nil, "", fmt.Errorf("%w (%v)", ErrPasswordIlegible, err)
	}
	return cuenta, password, nil
}
