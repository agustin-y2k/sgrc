package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func datosValidos() DatosDeCuenta {
	return DatosDeCuenta{
		Usuario:       "Alumno",
		Clase:         "Local",
		Privilegio:    PrivilegioComun,
		Visibilidad:   VisibilidadSoloAdmin,
		TienePassword: true,
	}
}

func TestNuevaCuentaDeEquipo_OK(t *testing.T) {
	ahora := time.Now()

	c, err := NuevaCuentaDeEquipo("c1", "eq1", datosValidos(), "cifrado", ahora)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if c.Usuario != "Alumno" {
		t.Errorf("usuario: %q", c.Usuario)
	}
	if !c.HayPasswordGuardada() {
		t.Error("con contraseña cifrada tendría que haber password guardada")
	}
}

// La caja del nombre no se toca: es lo que hay que tipear en la pantalla de la
// máquina. Lo único que se recorta son los bordes.
func TestUsuarioDeCuentaValido_ConservaLaCajaYRecortaBordes(t *testing.T) {
	usuario, err := UsuarioDeCuentaValido("  Administrador  ")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if usuario != "Administrador" {
		t.Fatalf("esperaba Administrador, obtuve %q", usuario)
	}
}

func TestDatosDeCuenta_Validar_Rechazos(t *testing.T) {
	casos := []struct {
		nombre   string
		ajustar  func(*DatosDeCuenta)
		esperado error
	}{
		{"usuario vacío", func(d *DatosDeCuenta) { d.Usuario = "   " }, ErrUsuarioCuentaVacio},
		{"usuario largo", func(d *DatosDeCuenta) {
			d.Usuario = strings.Repeat("a", MaxLargoUsuarioCuenta+1)
		}, ErrUsuarioCuentaLargo},
		{"clase vacía", func(d *DatosDeCuenta) { d.Clase = "   " }, ErrClaseCuentaVacia},
		{"clase larga", func(d *DatosDeCuenta) {
			d.Clase = strings.Repeat("c", MaxLargoClaseCuenta+1)
		}, ErrClaseCuentaLarga},
		{"privilegio inventado", func(d *DatosDeCuenta) { d.Privilegio = "ROOT" }, ErrPrivilegioInvalido},
		{"visibilidad inventada", func(d *DatosDeCuenta) { d.Visibilidad = "TODOS" }, ErrVisibilidadInvalida},
		{"notas largas", func(d *DatosDeCuenta) {
			d.Notas = strings.Repeat("n", MaxLargoNotasCuenta+1)
		}, ErrNotasCuentaLargas},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := datosValidos()
			c.ajustar(&d)
			if _, err := d.Validar(); !errors.Is(err, c.esperado) {
				t.Fatalf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

// ── Los tres estados de la contraseña ────────────────────────────────────
//
// No son dos. Sin el tercero, "la cuenta no pide contraseña" y "pide una que
// no sabemos" se muestran igual, y esa confusión termina con alguien parado
// frente a una máquina que no abre.

func TestCuenta_LibreSinContrasena(t *testing.T) {
	d := datosValidos()
	d.TienePassword = false

	c, err := NuevaCuentaDeEquipo("c1", "eq1", d, "", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if c.TienePassword {
		t.Error("la cuenta es libre")
	}
	if c.HayPasswordGuardada() {
		t.Error("una cuenta libre no tiene contraseña guardada")
	}
}

func TestCuenta_PideContrasenaQueNoSabemos(t *testing.T) {
	// Es el caso real de la notebook que alguien configuró y nadie anotó.
	c, err := NuevaCuentaDeEquipo("c1", "eq1", datosValidos(), "", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !c.TienePassword {
		t.Error("la cuenta sí pide contraseña")
	}
	if c.HayPasswordGuardada() {
		t.Error("pero no la tenemos anotada, así que no hay nada que revelar")
	}
}

// La contradicción: una cuenta marcada como libre con una contraseña guardada.
// La base también lo rechaza; se atrapa acá para explicarlo en vez de devolver
// un 500.
func TestCuenta_LibrePeroConContrasena_SeRechaza(t *testing.T) {
	d := datosValidos()
	d.TienePassword = false

	_, err := NuevaCuentaDeEquipo("c1", "eq1", d, "cifrado", time.Now())

	if !errors.Is(err, ErrPasswordSinTenerlaEs) {
		t.Fatalf("esperaba ErrPasswordSinTenerlaEs, obtuve %v", err)
	}
}

func TestPasswordDeCuentaValida(t *testing.T) {
	if err := PasswordDeCuentaValida("", false); err != nil {
		t.Errorf("una cuenta libre sin contraseña es válida: %v", err)
	}
	if err := PasswordDeCuentaValida("secreta", true); err != nil {
		t.Errorf("con contraseña declarada es válida: %v", err)
	}
	if err := PasswordDeCuentaValida("secreta", false); !errors.Is(err, ErrPasswordSinTenerlaEs) {
		t.Errorf("esperaba ErrPasswordSinTenerlaEs, obtuve %v", err)
	}
	larga := strings.Repeat("x", MaxLargoPasswordCuenta+1)
	if err := PasswordDeCuentaValida(larga, true); !errors.Is(err, ErrPasswordCuentaLarga) {
		t.Errorf("esperaba ErrPasswordCuentaLarga, obtuve %v", err)
	}
}

// ── La visibilidad es de la cuenta, no del privilegio ────────────────────

// El caso que motiva que sean dos campos: una cuenta de administrador que
// todo el mundo usa, y una cuenta común que solo administración debe abrir.
// Deducir una de la otra se equivocaría en los dos sentidos.
func TestVisibilidad_EsIndependienteDelPrivilegio(t *testing.T) {
	adminPublica := datosValidos()
	adminPublica.Privilegio = PrivilegioAdministrador
	adminPublica.Visibilidad = VisibilidadPublica

	comunReservada := datosValidos()
	comunReservada.Privilegio = PrivilegioComun
	comunReservada.Visibilidad = VisibilidadSoloAdmin

	a, err := NuevaCuentaDeEquipo("c1", "eq1", adminPublica, "cifrado", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	b, err := NuevaCuentaDeEquipo("c2", "eq1", comunReservada, "cifrado", time.Now())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !a.PuedeVerlaCualquiera() {
		t.Error("una cuenta de administrador marcada PUBLICA la ve cualquiera")
	}
	if b.PuedeVerlaCualquiera() {
		t.Error("una cuenta común marcada SOLO_ADMIN no la ve cualquiera")
	}
}

func TestParseos(t *testing.T) {
	if p, err := ParsePrivilegioDeCuenta("administrador"); err != nil || p != PrivilegioAdministrador {
		t.Errorf("privilegio: %v %v", p, err)
	}
	if v, err := ParseVisibilidadDeCuenta("publica"); err != nil || v != VisibilidadPublica {
		t.Errorf("visibilidad: %v %v", v, err)
	}
}

// La clase NO es una lista cerrada: una escuela con RedHat tiene cuentas de
// Linux, y con un enum eso pediría una migración para poder anotarse.
func TestClaseDeCuenta_AceptaCualquierTexto(t *testing.T) {
	for _, clase := range []string{"Local", "Microsoft", "Linux", "RedHat IdM", "Google Workspace"} {
		d := datosValidos()
		d.Clase = clase
		if _, err := d.Validar(); err != nil {
			t.Errorf("%q tendría que ser válida: %v", clase, err)
		}
	}
}

// Se conserva la caja —es lo que hay que tipear en la máquina— y se colapsan
// los espacios internos, igual que el tipo de equipo.
func TestClaseDeCuentaValida_NormalizaEspaciosYConservaLaCaja(t *testing.T) {
	clase, err := ClaseDeCuentaValida("  RedHat   IdM  ")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if clase != "RedHat IdM" {
		t.Fatalf("esperaba \"RedHat IdM\", obtuve %q", clase)
	}
}
