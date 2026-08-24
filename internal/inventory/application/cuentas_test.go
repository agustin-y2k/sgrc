package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/secretos"
)

// Las cuentas de usuario de cada equipo (RF-03.22).

const (
	esAdmin   = true
	noEsAdmin = false
)

func servicioConEquipo(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "NOTEBOOK", Nombre: "Notebook 1"}
	return servicioSimple(repo), repo
}

func datosDeCuenta(usuario string, visibilidad domain.VisibilidadDeCuenta) domain.DatosDeCuenta {
	return domain.DatosDeCuenta{
		Usuario:       usuario,
		Clase:         "Local",
		Privilegio:    domain.PrivilegioComun,
		Visibilidad:   visibilidad,
		TienePassword: true,
	}
}

// ── La contraseña se guarda cifrada ──────────────────────────────────────

func TestCrearCuenta_GuardaLaPasswordCifrada(t *testing.T) {
	svc, repo := servicioConEquipo(t)

	cuenta, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	guardada := repo.cuentas[cuenta.ID].PasswordCifrada
	if guardada == "" {
		t.Fatal("no se guardó nada")
	}
	// Lo que va a la base no puede ser la contraseña: es el punto de cifrarla
	// antes de que entre al volcado de `make backup`.
	if guardada == "SecretaDeLaMaquina" {
		t.Fatal("la contraseña quedó guardada en claro")
	}
}

func TestRevelarPassword_DevuelveLaOriginal(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	cuenta, _ := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	_, password, err := svc.RevelarPasswordDeCuenta(context.Background(), cuenta.ID, noEsAdmin)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if password != "SecretaDeLaMaquina" {
		t.Fatalf("esperaba la contraseña original, obtuve %q", password)
	}
}

// ── Quién puede ver qué ──────────────────────────────────────────────────

func TestRevelarPassword_ReservadaSinSerAdmin_SeNiega(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	cuenta, _ := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Administrador", domain.VisibilidadSoloAdmin), "SecretaDeLaMaquina")

	_, _, err := svc.RevelarPasswordDeCuenta(context.Background(), cuenta.ID, noEsAdmin)

	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("esperaba ErrNoAutorizado, obtuve %v", err)
	}
}

func TestRevelarPassword_ReservadaSiendoAdmin_SeRevela(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	cuenta, _ := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Administrador", domain.VisibilidadSoloAdmin), "SecretaDeLaMaquina")

	_, password, err := svc.RevelarPasswordDeCuenta(context.Background(), cuenta.ID, esAdmin)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if password != "SecretaDeLaMaquina" {
		t.Fatalf("obtuve %q", password)
	}
}

// El caso que motiva que visibilidad y privilegio sean dos campos: una cuenta
// CON privilegios de administrador puede ser pública, y una común puede ser
// reservada. Deducir una de la otra se equivocaría en los dos sentidos.
func TestRevelarPassword_ElPrivilegioNoDecideQuienLaVe(t *testing.T) {
	svc, _ := servicioConEquipo(t)

	adminPublica := datosDeCuenta("Soporte", domain.VisibilidadPublica)
	adminPublica.Privilegio = domain.PrivilegioAdministrador
	cuentaAdminPublica, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1", adminPublica, "clave-de-soporte")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	comunReservada := datosDeCuenta("Direccion", domain.VisibilidadSoloAdmin)
	cuentaComunReservada, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1", comunReservada, "clave-de-direccion")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Cuenta de ADMINISTRADOR pero pública: un docente la ve.
	if _, _, err := svc.RevelarPasswordDeCuenta(context.Background(), cuentaAdminPublica.ID, noEsAdmin); err != nil {
		t.Errorf("una cuenta pública la ve cualquiera, aunque sea de administrador: %v", err)
	}
	// Cuenta COMÚN pero reservada: un docente no la ve.
	if _, _, err := svc.RevelarPasswordDeCuenta(context.Background(), cuentaComunReservada.ID, noEsAdmin); !errors.Is(err, ErrNoAutorizado) {
		t.Errorf("una cuenta reservada no la ve un docente, aunque sea común: %v", err)
	}
}

// ── El tercer estado: pide contraseña y no la sabemos ────────────────────

func TestRevelarPassword_SinPasswordAnotada_LoDice(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	// TienePassword=true pero sin contraseña: la notebook que alguien
	// configuró y nadie anotó.
	cuenta, _ := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Alumno", domain.VisibilidadPublica), "")

	_, _, err := svc.RevelarPasswordDeCuenta(context.Background(), cuenta.ID, esAdmin)

	if !errors.Is(err, ErrPasswordNoGuardada) {
		t.Fatalf("esperaba ErrPasswordNoGuardada, obtuve %v", err)
	}
}

// ── El listado ───────────────────────────────────────────────────────────

// La cuenta y su privilegio se listan SIEMPRE: esconderlas haría que el
// inventario mienta por omisión. Lo que cambia según quién pregunta es si
// puede revelar la contraseña.
func TestListarCuentas_MuestraTodasYMarcaCualesPuedeVer(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	ctx := context.Background()
	publica, _ := svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Alumno", domain.VisibilidadPublica), "abc")
	reservada, _ := svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Direccion", domain.VisibilidadSoloAdmin), "xyz")

	comoDocente, err := svc.ListarCuentasDeEquipo(ctx, "eq1", noEsAdmin)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if len(comoDocente) != 2 {
		t.Fatalf("las dos cuentas se listan siempre, obtuve %d", len(comoDocente))
	}
	for _, c := range comoDocente {
		switch c.ID {
		case publica.ID:
			if !c.PuedeVerLaPassword {
				t.Error("la pública la puede ver un docente")
			}
		case reservada.ID:
			if c.PuedeVerLaPassword {
				t.Error("la reservada no la puede ver un docente")
			}
		}
	}

	comoAdmin, _ := svc.ListarCuentasDeEquipo(ctx, "eq1", esAdmin)
	for _, c := range comoAdmin {
		if !c.PuedeVerLaPassword {
			t.Errorf("un Admin puede ver todas, falló en %q", c.Usuario)
		}
	}
}

// La contraseña NO viaja en el listado ni para un Admin: se pide de a una para
// que la auditoría distinga "abrió la ficha" de "necesitaba esta contraseña".
func TestListarCuentas_NoDevuelveLaPasswordEnClaro(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	ctx := context.Background()
	svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	cuentas, _ := svc.ListarCuentasDeEquipo(ctx, "eq1", esAdmin)

	for _, c := range cuentas {
		if c.PasswordCifrada == "SecretaDeLaMaquina" {
			t.Fatal("el listado no puede llevar la contraseña en claro")
		}
	}
}

// ── Edición ──────────────────────────────────────────────────────────────

// Cambiar la visibilidad sin volver a mandar la contraseña es el caso normal:
// alguien se da cuenta de que esa cuenta no debería ser pública.
func TestEditarCuenta_CambiarVisibilidadConservaLaPassword(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	ctx := context.Background()
	cuenta, _ := svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	reservada := domain.VisibilidadSoloAdmin
	if _, err := svc.EditarCuentaDeEquipo(ctx, cuenta.ID, EditarCuentaParams{Visibilidad: &reservada}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	_, password, err := svc.RevelarPasswordDeCuenta(ctx, cuenta.ID, esAdmin)
	if err != nil {
		t.Fatalf("la contraseña tenía que seguir ahí: %v", err)
	}
	if password != "SecretaDeLaMaquina" {
		t.Fatalf("obtuve %q", password)
	}
	// Y ahora un docente ya no la ve.
	if _, _, err := svc.RevelarPasswordDeCuenta(ctx, cuenta.ID, noEsAdmin); !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("esperaba ErrNoAutorizado, obtuve %v", err)
	}
}

// Pasar la cuenta a "libre" tiene que soltar la contraseña guardada: si no,
// quedaría una contraseña colgando de una cuenta que dice no tener ninguna.
func TestEditarCuenta_PasarALibreSueltaLaPassword(t *testing.T) {
	svc, repo := servicioConEquipo(t)
	ctx := context.Background()
	cuenta, _ := svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	libre := false
	if _, err := svc.EditarCuentaDeEquipo(ctx, cuenta.ID, EditarCuentaParams{TienePassword: &libre}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if repo.cuentas[cuenta.ID].PasswordCifrada != "" {
		t.Fatal("una cuenta libre no puede conservar una contraseña guardada")
	}
}

// Borrar la contraseña anotada sin dejar de declarar que la cuenta pide una:
// es lo que pasa cuando alguien la cambió en la máquina y todavía no sabemos
// la nueva.
func TestEditarCuenta_BorrarSoloLaPasswordAnotada(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	ctx := context.Background()
	cuenta, _ := svc.CrearCuentaDeEquipo(ctx, "eq1", datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	vacia := ""
	actualizada, err := svc.EditarCuentaDeEquipo(ctx, cuenta.ID, EditarCuentaParams{Password: &vacia})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !actualizada.TienePassword {
		t.Error("la cuenta sigue pidiendo contraseña")
	}
	if actualizada.HayPasswordGuardada() {
		t.Error("pero ya no la tenemos anotada")
	}
}

func TestCrearCuenta_EquipoInexistente(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearCuentaDeEquipo(context.Background(), "fantasma",
		datosDeCuenta("Alumno", domain.VisibilidadPublica), "abc")

	if !errors.Is(err, ErrEquipoNoEncontrado) {
		t.Fatalf("esperaba ErrEquipoNoEncontrado, obtuve %v", err)
	}
}

// ── Sin CUENTAS_SECRET configurada ───────────────────────────────────────

func servicioSinCifrador(t *testing.T) *Service {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.equipos["eq1"] = &domain.Equipo{ID: "eq1", Tipo: "NOTEBOOK", Nombre: "Notebook 1"}
	sinClave, err := secretos.Nuevo("")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	contadorID = 0
	return NewService(repo, &fakeValidadorReservas{}, idSecuencial, func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}, sinClave)
}

// El despliegue sin CUENTAS_SECRET registra cuentas igual: es una función de
// menos, no un sistema roto.
func TestSinClave_SePuedeRegistrarUnaCuentaSinPassword(t *testing.T) {
	svc := servicioSinCifrador(t)

	datos := datosDeCuenta("Alumno", domain.VisibilidadPublica)
	datos.TienePassword = false

	if _, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1", datos, ""); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

// Lo único que no puede es guardar contraseñas, y lo dice con un error propio
// en vez de romper.
func TestSinClave_GuardarUnaPasswordLoDice(t *testing.T) {
	svc := servicioSinCifrador(t)

	_, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1",
		datosDeCuenta("Alumno", domain.VisibilidadPublica), "SecretaDeLaMaquina")

	if !errors.Is(err, ErrSinClaveDeCifrado) {
		t.Fatalf("esperaba ErrSinClaveDeCifrado, obtuve %v", err)
	}
}

// ── Que todo esto sea OPCIONAL ──────────────────────────────────────────
//
// Anotar con qué usuario y contraseña se entra a un equipo es información
// útil, no un requisito para darlo de alta. Hoy funciona porque nada lo
// impide; estos tests lo fijan, para que una validación agregada más adelante
// no convierta el inventario en un formulario que no se puede completar.

// Un equipo puede vivir sin ninguna cuenta anotada, y eso NO es un error: es
// lo que pasa con todo el inventario que ya existe.
func TestOpcional_UnEquipoSinCuentasEsValido(t *testing.T) {
	svc, repo := servicioConEquipo(t)

	cuentas, err := svc.ListarCuentasDeEquipo(context.Background(), "eq1", noEsAdmin)

	if err != nil {
		t.Fatalf("un equipo sin cuentas no es un error: %v", err)
	}
	if len(cuentas) != 0 {
		t.Fatalf("esperaba ninguna cuenta, obtuve %d", len(cuentas))
	}
	// Y el equipo sigue estando, entero: no cargar cuentas no lo deja a medio
	// dar de alta.
	if repo.equipos["eq1"] == nil {
		t.Fatal("el equipo tiene que existir igual")
	}
}

// Dar de alta un equipo nunca pide cuentas: no hay ningún camino en el que
// falte una y el alta se rechace.
func TestOpcional_DarDeAltaUnEquipoNoPideCuentas(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	equipo, err := svc.CrearEquipo(context.Background(), "NOTEBOOK", "Notebook Dirección", "", true)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	cuentas, err := svc.ListarCuentasDeEquipo(context.Background(), equipo.ID, esAdmin)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cuentas) != 0 {
		t.Fatalf("un equipo recién creado no tiene cuentas, obtuve %d", len(cuentas))
	}
}

// Una cuenta se puede anotar sin contraseña de dos maneras distintas, y las
// dos son válidas.
func TestOpcional_UnaCuentaSePuedeAnotarSinContrasena(t *testing.T) {
	svc, _ := servicioConEquipo(t)
	ctx := context.Background()

	// 1. La cuenta es libre: se entra sin escribir nada.
	libre := datosDeCuenta("kiosco", domain.VisibilidadPublica)
	libre.TienePassword = false
	if _, err := svc.CrearCuentaDeEquipo(ctx, "eq1", libre, ""); err != nil {
		t.Fatalf("una cuenta libre es válida: %v", err)
	}

	// 2. La cuenta pide contraseña pero no la sabemos.
	sinAnotar := datosDeCuenta("alumno", domain.VisibilidadPublica)
	if _, err := svc.CrearCuentaDeEquipo(ctx, "eq1", sinAnotar, ""); err != nil {
		t.Fatalf("una cuenta con contraseña no anotada es válida: %v", err)
	}

	cuentas, _ := svc.ListarCuentasDeEquipo(ctx, "eq1", esAdmin)
	if len(cuentas) != 2 {
		t.Fatalf("esperaba las dos cuentas, obtuve %d", len(cuentas))
	}
	for _, c := range cuentas {
		if c.HayPasswordParaVer {
			t.Errorf("%q no tiene contraseña anotada", c.Usuario)
		}
	}
}

// Y las notas también son opcionales: son un comentario, no un dato del
// equipo.
func TestOpcional_LasNotasSePuedenOmitir(t *testing.T) {
	svc, _ := servicioConEquipo(t)

	datos := datosDeCuenta("alumno", domain.VisibilidadPublica)
	datos.Notas = ""

	if _, err := svc.CrearCuentaDeEquipo(context.Background(), "eq1", datos, "abc"); err != nil {
		t.Fatalf("las notas son opcionales: %v", err)
	}
}
