package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ── Las cuentas de usuario de cada equipo (RF-03.22) ─────────────────────
//
// Una notebook no se abre sola: hay que saber con qué cuenta entrar. En una
// escuela conviven cuentas locales, de Microsoft y de Linux, comunes y de
// administrador, algunas con contraseña y otras libres.
//
// Cargarlas es opcional: un equipo sin cuentas es un equipo del que no
// anotamos nada, no un equipo mal cargado.

// La clase de una cuenta —dónde vive: local a la máquina, en Microsoft, en un
// Linux, en Google— es TEXTO LIBRE, no una lista cerrada.
//
// Es el mismo criterio que el tipo de equipo, y por la misma razón: "local" y
// "Microsoft" no agotan nada. Una escuela que corre RedHat tiene cuentas de
// Linux, otra usa Google Workspace, y con un enum cada una de esas realidades
// pediría una migración y un despliegue para poder anotarse. El formulario
// sugiere las clases ya cargadas para que no convivan "Microsoft" y
// "MICROSOFT".
const MaxLargoClaseCuenta = 30

// PrivilegioDeCuenta es lo que la cuenta puede hacer en la máquina. Se detalla
// SIEMPRE, incluso cuando la contraseña no se muestra: saber que una notebook
// tiene una cuenta de administrador es útil aunque no puedas usarla.
type PrivilegioDeCuenta string

const (
	PrivilegioComun         PrivilegioDeCuenta = "COMUN"
	PrivilegioAdministrador PrivilegioDeCuenta = "ADMINISTRADOR"
)

// VisibilidadDeCuenta decide a quién se le revela la CONTRASEÑA de esta
// cuenta. La marca un ADMIN, cuenta por cuenta.
//
// Es independiente del privilegio a propósito: puede haber una cuenta de
// administrador que todo el mundo usa —la máquina del taller donde hay que
// instalar cosas— y una cuenta común que solo administración debe abrir.
// Deducirla del privilegio se equivocaría en los dos sentidos.
type VisibilidadDeCuenta string

const (
	// VisibilidadPublica: cualquier usuario con sesión iniciada puede ver la
	// contraseña.
	VisibilidadPublica VisibilidadDeCuenta = "PUBLICA"
	// VisibilidadSoloAdmin: la contraseña se le revela únicamente a un ADMIN, y
	// cada vez que se revela queda registrado.
	VisibilidadSoloAdmin VisibilidadDeCuenta = "SOLO_ADMIN"
)

const (
	MaxLargoUsuarioCuenta = 100
	MaxLargoNotasCuenta   = 500
	// MaxLargoPasswordCuenta es holgado a propósito: no es una contraseña que
	// el sistema imponga, es una que ya existe en la máquina y hay que poder
	// anotarla tal como es.
	MaxLargoPasswordCuenta = 200
)

var (
	ErrUsuarioCuentaVacio   = errors.New("hay que indicar el nombre de la cuenta")
	ErrUsuarioCuentaLargo   = fmt.Errorf("el nombre de la cuenta no puede tener más de %d caracteres", MaxLargoUsuarioCuenta)
	ErrClaseCuentaVacia     = errors.New("hay que indicar de qué tipo es la cuenta (local, Microsoft, Linux…)")
	ErrClaseCuentaLarga     = fmt.Errorf("el tipo de cuenta no puede tener más de %d caracteres", MaxLargoClaseCuenta)
	ErrPrivilegioInvalido   = errors.New("el privilegio tiene que ser COMUN o ADMINISTRADOR")
	ErrVisibilidadInvalida  = errors.New("la visibilidad tiene que ser PUBLICA o SOLO_ADMIN")
	ErrNotasCuentaLargas    = fmt.Errorf("las notas no pueden tener más de %d caracteres", MaxLargoNotasCuenta)
	ErrPasswordCuentaLarga  = fmt.Errorf("la contraseña no puede tener más de %d caracteres", MaxLargoPasswordCuenta)
	ErrPasswordSinTenerlaEs = errors.New("no se puede guardar una contraseña en una cuenta marcada como libre: o la cuenta pide contraseña, o no la pide")
)

// ClaseDeCuentaValida normaliza y valida la clase. Conserva la caja —"Microsoft"
// se muestra como se escribió— y colapsa los espacios internos, igual que el
// tipo de equipo.
func ClaseDeCuentaValida(clase string) (string, error) {
	clase = strings.Join(strings.Fields(clase), " ")
	if clase == "" {
		return "", ErrClaseCuentaVacia
	}
	if len([]rune(clase)) > MaxLargoClaseCuenta {
		return "", ErrClaseCuentaLarga
	}
	return clase, nil
}

func ParsePrivilegioDeCuenta(s string) (PrivilegioDeCuenta, error) {
	switch PrivilegioDeCuenta(strings.ToUpper(strings.TrimSpace(s))) {
	case PrivilegioComun:
		return PrivilegioComun, nil
	case PrivilegioAdministrador:
		return PrivilegioAdministrador, nil
	default:
		return "", ErrPrivilegioInvalido
	}
}

func ParseVisibilidadDeCuenta(s string) (VisibilidadDeCuenta, error) {
	switch VisibilidadDeCuenta(strings.ToUpper(strings.TrimSpace(s))) {
	case VisibilidadPublica:
		return VisibilidadPublica, nil
	case VisibilidadSoloAdmin:
		return VisibilidadSoloAdmin, nil
	default:
		return "", ErrVisibilidadInvalida
	}
}

// CuentaDeEquipo es una cuenta con la que se inicia sesión en un equipo.
type CuentaDeEquipo struct {
	ID       string
	EquipoID string
	Usuario  string
	Clase    string
	// Privilegio se muestra siempre, aunque la contraseña no.
	Privilegio PrivilegioDeCuenta
	// TienePassword y PasswordCifrada son dos cosas distintas porque hay TRES
	// estados y no dos:
	//
	//   - TienePassword=false            → la cuenta es libre, se entra sin nada.
	//   - TienePassword=true  + cifrada  → pide contraseña y la tenemos anotada.
	//   - TienePassword=true  + vacía    → pide contraseña y NO la sabemos.
	//
	// Sin el tercero, "no tiene contraseña" y "no sabemos la contraseña" se
	// muestran igual, y esa confusión termina con alguien parado frente a una
	// máquina que no abre.
	TienePassword   bool
	PasswordCifrada string
	Visibilidad     VisibilidadDeCuenta
	Notas           string
	CreadaEn        time.Time
	ActualizadaEn   time.Time
}

// UsuarioDeCuentaValido normaliza y valida el nombre de la cuenta. No se toca
// la caja: `Administrador` y `administrador` se muestran como se escribieron,
// porque es lo que hay que tipear en la pantalla de la máquina. La unicidad
// dentro del equipo sí ignora mayúsculas, y la resuelve la base.
//
// El nombre es OBLIGATORIO, y es lo único de una cuenta que lo es. Lo opcional
// es la cuenta entera: un equipo puede no tener ninguna anotada. Pero una vez
// que se anota una, tiene que decir con qué usuario se entra — si no, no hay
// nada que tipear frente a la máquina y la fila no informa nada.
//
// La notebook que arranca sola, sin pedir nada, se anota con el nombre que se
// vea en su pantalla y `TienePassword` en false.
func UsuarioDeCuentaValido(usuario string) (string, error) {
	usuario = strings.TrimSpace(usuario)
	if usuario == "" {
		return "", ErrUsuarioCuentaVacio
	}
	if len([]rune(usuario)) > MaxLargoUsuarioCuenta {
		return "", ErrUsuarioCuentaLargo
	}
	return usuario, nil
}

// DatosDeCuenta es lo que hace falta para crear o editar una cuenta, con la
// contraseña todavía en claro: cifrarla es responsabilidad de la capa de
// aplicación, que es la que tiene el cifrador.
type DatosDeCuenta struct {
	Usuario       string
	Clase         string
	Privilegio    PrivilegioDeCuenta
	Visibilidad   VisibilidadDeCuenta
	TienePassword bool
	Notas         string
}

// Validar comprueba lo que no depende de la base ni del cifrador.
func (d DatosDeCuenta) Validar() (DatosDeCuenta, error) {
	usuario, err := UsuarioDeCuentaValido(d.Usuario)
	if err != nil {
		return d, err
	}
	d.Usuario = usuario

	clase, err := ClaseDeCuentaValida(d.Clase)
	if err != nil {
		return d, err
	}
	d.Clase = clase

	if d.Privilegio != PrivilegioComun && d.Privilegio != PrivilegioAdministrador {
		return d, ErrPrivilegioInvalido
	}
	if d.Visibilidad != VisibilidadPublica && d.Visibilidad != VisibilidadSoloAdmin {
		return d, ErrVisibilidadInvalida
	}

	d.Notas = strings.TrimSpace(d.Notas)
	if len([]rune(d.Notas)) > MaxLargoNotasCuenta {
		return d, ErrNotasCuentaLargas
	}
	return d, nil
}

// PasswordDeCuentaValida comprueba la contraseña en claro contra el estado
// declarado de la cuenta. Guardar una contraseña en una cuenta marcada como
// libre es la contradicción que la base también rechaza; se atrapa acá para
// poder explicarla en vez de devolver un 500.
func PasswordDeCuentaValida(password string, tienePassword bool) error {
	if password == "" {
		return nil
	}
	if !tienePassword {
		return ErrPasswordSinTenerlaEs
	}
	if len([]rune(password)) > MaxLargoPasswordCuenta {
		return ErrPasswordCuentaLarga
	}
	return nil
}

// NuevaCuentaDeEquipo arma la cuenta ya validada. `passwordCifrada` viene de
// la capa de aplicación, que es la que tiene el cifrador; vacía significa "no
// hay contraseña guardada", que combinado con TienePassword da los tres
// estados de arriba.
func NuevaCuentaDeEquipo(id, equipoID string, datos DatosDeCuenta, passwordCifrada string, ahora time.Time) (*CuentaDeEquipo, error) {
	datos, err := datos.Validar()
	if err != nil {
		return nil, err
	}
	if !datos.TienePassword && passwordCifrada != "" {
		return nil, ErrPasswordSinTenerlaEs
	}
	return &CuentaDeEquipo{
		ID:              id,
		EquipoID:        equipoID,
		Usuario:         datos.Usuario,
		Clase:           datos.Clase,
		Privilegio:      datos.Privilegio,
		Visibilidad:     datos.Visibilidad,
		TienePassword:   datos.TienePassword,
		PasswordCifrada: passwordCifrada,
		Notas:           datos.Notas,
		CreadaEn:        ahora,
		ActualizadaEn:   ahora,
	}, nil
}

// HayPasswordGuardada distingue el segundo estado del tercero: la cuenta pide
// contraseña Y la tenemos anotada.
func (c *CuentaDeEquipo) HayPasswordGuardada() bool {
	return c.TienePassword && c.PasswordCifrada != ""
}

// PuedeVerlaCualquiera dice si la contraseña de esta cuenta se le muestra a
// cualquier usuario con sesión, o solo a un ADMIN. Lo que nunca se oculta es
// la cuenta en sí ni su privilegio.
func (c *CuentaDeEquipo) PuedeVerlaCualquiera() bool {
	return c.Visibilidad == VisibilidadPublica
}
