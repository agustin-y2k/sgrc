package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// ── ParseRol / ParseEstado ─────────────────────────────────────────────

func TestParseRol_Validos(t *testing.T) {
	casos := map[string]Rol{"ADMIN": RolAdmin, "DOCENTE": RolDocente}
	for entrada, esperado := range casos {
		got, err := ParseRol(entrada)
		if err != nil {
			t.Errorf("ParseRol(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseRol(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseRol_Invalido(t *testing.T) {
	casos := []string{"", "admin", "SUPER_ADMIN", "docente ", " ADMIN"}
	for _, c := range casos {
		_, err := ParseRol(c)
		if !errors.Is(err, ErrRolInvalido) {
			t.Errorf("ParseRol(%q): esperaba ErrRolInvalido, obtuve %v", c, err)
		}
	}
}

func TestParseEstado_Validos(t *testing.T) {
	casos := map[string]Estado{
		"PENDIENTE": EstadoPendiente,
		"APROBADA":  EstadoAprobada,
		"RECHAZADA": EstadoRechazada,
		"BAJA":      EstadoBaja,
	}
	for entrada, esperado := range casos {
		got, err := ParseEstado(entrada)
		if err != nil {
			t.Errorf("ParseEstado(%q) no debería fallar: %v", entrada, err)
		}
		if got != esperado {
			t.Errorf("ParseEstado(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParseEstado_Invalido(t *testing.T) {
	casos := []string{"", "pendiente", "ACTIVA", "BAJA "}
	for _, c := range casos {
		_, err := ParseEstado(c)
		if !errors.Is(err, ErrEstadoInvalido) {
			t.Errorf("ParseEstado(%q): esperaba ErrEstadoInvalido, obtuve %v", c, err)
		}
	}
}

// ── Transiciones de estado — todas las combinaciones posibles ───────── Se
// prueban las 16 combinaciones (4 estados x 4 destinos) explícitamente en vez
// de solo los casos "felices", para que un cambio futuro en
// PuedeTransicionarA no pueda abrir una transición no revisada sin que algún
// test lo note.

func TestPuedeTransicionarA_TodasLasCombinaciones(t *testing.T) {
	estados := []Estado{EstadoPendiente, EstadoAprobada, EstadoRechazada, EstadoBaja}

	permitidas := map[[2]Estado]bool{
		{EstadoPendiente, EstadoAprobada}:  true,
		{EstadoPendiente, EstadoRechazada}: true,
		{EstadoAprobada, EstadoBaja}:       true,
	}

	for _, desde := range estados {
		for _, hacia := range estados {
			esperado := permitidas[[2]Estado{desde, hacia}]
			got := desde.PuedeTransicionarA(hacia)
			if got != esperado {
				t.Errorf("PuedeTransicionarA: %s -> %s = %v, esperaba %v", desde, hacia, got, esperado)
			}
		}
	}
}

func TestCambiarEstado_PendienteAAprobada_OK(t *testing.T) {
	u := &Usuario{Estado: EstadoPendiente}
	ahora := time.Now()

	err := u.CambiarEstado(EstadoAprobada, ahora)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Estado != EstadoAprobada {
		t.Errorf("estado final incorrecto: %s", u.Estado)
	}
	if u.FechaAprobacion == nil || !u.FechaAprobacion.Equal(ahora) {
		t.Error("FechaAprobacion debería quedar seteada al aprobar")
	}
}

func TestCambiarEstado_PendienteARechazada_OK_SinFechaAprobacion(t *testing.T) {
	u := &Usuario{Estado: EstadoPendiente}

	err := u.CambiarEstado(EstadoRechazada, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.FechaAprobacion != nil {
		t.Error("una cuenta rechazada nunca debería tener FechaAprobacion seteada")
	}
}

func TestCambiarEstado_AprobadaABaja_OK(t *testing.T) {
	u := &Usuario{Estado: EstadoAprobada}

	err := u.CambiarEstado(EstadoBaja, time.Now())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if u.Estado != EstadoBaja {
		t.Errorf("estado final incorrecto: %s", u.Estado)
	}
}

func TestCambiarEstado_DesdeBaja_SiempreRechazado(t *testing.T) {
	// El caso más importante de todo este archivo: BAJA es terminal.
	destinos := []Estado{EstadoPendiente, EstadoAprobada, EstadoRechazada, EstadoBaja}

	for _, destino := range destinos {
		u := &Usuario{Estado: EstadoBaja}
		err := u.CambiarEstado(destino, time.Now())
		if !errors.Is(err, ErrTransicionInvalida) {
			t.Errorf("BAJA -> %s: esperaba ErrTransicionInvalida, obtuve %v", destino, err)
		}
		if u.Estado != EstadoBaja {
			t.Errorf("BAJA -> %s: el estado cambió a %s, no debería haber cambiado", destino, u.Estado)
		}
	}
}

func TestCambiarEstado_DesdeRechazada_SiempreRechazado(t *testing.T) {
	destinos := []Estado{EstadoPendiente, EstadoAprobada, EstadoRechazada, EstadoBaja}

	for _, destino := range destinos {
		u := &Usuario{Estado: EstadoRechazada}
		err := u.CambiarEstado(destino, time.Now())
		if !errors.Is(err, ErrTransicionInvalida) {
			t.Errorf("RECHAZADA -> %s: esperaba ErrTransicionInvalida, obtuve %v", destino, err)
		}
	}
}

func TestCambiarEstado_PendienteAPendiente_Rechazado(t *testing.T) {
	// No autotransición — PENDIENTE -> PENDIENTE no está en la lista de
	// permitidas y no debería "pasar de largo" por accidente.
	u := &Usuario{Estado: EstadoPendiente}

	err := u.CambiarEstado(EstadoPendiente, time.Now())

	if !errors.Is(err, ErrTransicionInvalida) {
		t.Fatalf("esperaba ErrTransicionInvalida, obtuve %v", err)
	}
}

func TestCambiarEstado_AprobadaAPendiente_Rechazado(t *testing.T) {
	// Una vez aprobada, no hay vuelta a PENDIENTE tampoco.
	u := &Usuario{Estado: EstadoAprobada}

	err := u.CambiarEstado(EstadoPendiente, time.Now())

	if !errors.Is(err, ErrTransicionInvalida) {
		t.Fatalf("esperaba ErrTransicionInvalida, obtuve %v", err)
	}
}

func TestCambiarEstado_ErrorNoModificaElEstado(t *testing.T) {
	u := &Usuario{Estado: EstadoAprobada}

	_ = u.CambiarEstado(EstadoRechazada, time.Now()) // transición inválida

	if u.Estado != EstadoAprobada {
		t.Errorf("un CambiarEstado fallido no debería mutar el estado — quedó en %s", u.Estado)
	}
}

// ── Helpers de lectura ─────────────────────────────────────────────────

func TestEsAdmin_EsDocente(t *testing.T) {
	admin := &Usuario{Rol: RolAdmin}
	docente := &Usuario{Rol: RolDocente}

	if !admin.EsAdmin() || admin.EsDocente() {
		t.Error("un ADMIN debería EsAdmin()==true y EsDocente()==false")
	}
	if !docente.EsDocente() || docente.EsAdmin() {
		t.Error("un DOCENTE debería EsDocente()==true y EsAdmin()==false")
	}
}

func TestEstaAprobado(t *testing.T) {
	casos := map[Estado]bool{
		EstadoPendiente: false,
		EstadoAprobada:  true,
		EstadoRechazada: false,
		EstadoBaja:      false,
	}
	for estado, esperado := range casos {
		u := &Usuario{Estado: estado}
		if u.EstaAprobado() != esperado {
			t.Errorf("EstaAprobado() con estado %s = %v, esperaba %v", estado, u.EstaAprobado(), esperado)
		}
	}
}

// ── Email como identidad (mayúsculas, espacios, formato) ────────────────

// ── NormalizarNombreYApellido ──────────────────────────────────────────

func TestNormalizarNombreYApellido_RecortaYAcepta(t *testing.T) {
	nombre, apellido, err := NormalizarNombreYApellido("  Ada ", " Lovelace  ")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if nombre != "Ada" || apellido != "Lovelace" {
		t.Errorf("obtuve %q %q, esperaba \"Ada\" \"Lovelace\"", nombre, apellido)
	}
}

func TestNormalizarNombreYApellido_Vacio_Error(t *testing.T) {
	casos := []struct{ nombre, apellido string }{
		{"", "Lovelace"},
		{"Ada", ""},
		{"   ", "Lovelace"},
		{"Ada", "\t"},
	}
	for _, c := range casos {
		if _, _, err := NormalizarNombreYApellido(c.nombre, c.apellido); !errors.Is(err, ErrNombreVacio) {
			t.Errorf("NormalizarNombreYApellido(%q, %q): esperaba ErrNombreVacio, obtuve %v",
				c.nombre, c.apellido, err)
		}
	}
}

// El límite se cuenta en caracteres, no en bytes: VARCHAR(100) en Postgres
// son 100 caracteres, y un apellido con eñes ocupa más bytes que letras.
func TestNormalizarNombreYApellido_ElLargoSeCuentaEnCaracteres(t *testing.T) {
	justo := strings.Repeat("ñ", LargoMaxNombre)
	if _, _, err := NormalizarNombreYApellido(justo, "Lovelace"); err != nil {
		t.Errorf("%d eñes tienen que entrar, obtuve %v", LargoMaxNombre, err)
	}

	unoDeMas := strings.Repeat("a", LargoMaxNombre+1)
	if _, _, err := NormalizarNombreYApellido(unoDeMas, "Lovelace"); !errors.Is(err, ErrNombreDemasiadoLargo) {
		t.Errorf("esperaba ErrNombreDemasiadoLargo, obtuve %v", err)
	}
	if _, _, err := NormalizarNombreYApellido("Ada", unoDeMas); !errors.Is(err, ErrNombreDemasiadoLargo) {
		t.Errorf("el apellido también tiene tope, obtuve %v", err)
	}
}

func TestNormalizarEmail(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"juan.perez@escuela.edu.ar", "juan.perez@escuela.edu.ar"},
		{"Juan.Perez@Escuela.Edu.Ar", "juan.perez@escuela.edu.ar"},
		{"  juan.perez@escuela.edu.ar  ", "juan.perez@escuela.edu.ar"},
		{"JUAN.PEREZ@ESCUELA.EDU.AR", "juan.perez@escuela.edu.ar"},
	}
	for _, c := range casos {
		if obtenido := NormalizarEmail(c.entrada); obtenido != c.esperado {
			t.Errorf("NormalizarEmail(%q) = %q, esperaba %q", c.entrada, obtenido, c.esperado)
		}
	}
}

// Las cuatro variantes del mismo buzón tienen que colapsar en una sola
// cadena: es lo que hace que la búsqueda por email encuentre la cuenta sin
// importar cómo la haya tipeado quien entra.
func TestNormalizarEmail_VariantesDelMismoBuzon_Colapsan(t *testing.T) {
	variantes := []string{
		"Juan.Perez@escuela.edu.ar",
		"juan.perez@escuela.edu.ar",
		"JUAN.PEREZ@escuela.edu.ar",
		" Juan.Perez@Escuela.edu.ar ",
	}
	primera := NormalizarEmail(variantes[0])
	for _, v := range variantes[1:] {
		if NormalizarEmail(v) != primera {
			t.Errorf("%q y %q deberían normalizar al mismo valor", variantes[0], v)
		}
	}
}

func TestValidarEmail(t *testing.T) {
	validos := []string{
		"juan.perez@escuela.edu.ar",
		"a@b.co",
		"nombre+etiqueta@dominio.com",
	}
	for _, e := range validos {
		if err := ValidarEmail(e); err != nil {
			t.Errorf("ValidarEmail(%q) debería aceptar, dio: %v", e, err)
		}
	}

	invalidos := []string{
		"no-es-un-email",  // el caso que entraba a la tabla como una cuenta más
		"",                // vacío
		"sin-arroba.com",  // sin @
		"docente@escuela", // dominio sin extensión: siempre es un tipeo incompleto
		"Juan <j@x.com>",  // la forma con nombre no va en un campo "email"
		"dos@arrobas@x.com",
	}
	for _, e := range invalidos {
		if err := ValidarEmail(e); !errors.Is(err, ErrEmailInvalido) {
			t.Errorf("ValidarEmail(%q) debería rechazar con ErrEmailInvalido, dio: %v", e, err)
		}
	}
}

// ── PromoverAAdmin ─────────────────────────────────────────────────────

func TestPromoverAAdmin_DocenteAprobado_OK(t *testing.T) {
	u := &Usuario{Rol: RolDocente, Estado: EstadoAprobada}

	if err := u.PromoverAAdmin(); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !u.EsAdmin() {
		t.Errorf("esperaba rol ADMIN, quedó %s", u.Rol)
	}
}

// Promover una cuenta PENDIENTE sería aprobarla por la puerta de atrás,
// salteándose el paso donde alguien mira quién es esa persona (RF-01.3).
func TestPromoverAAdmin_CuentaNoAprobada_Error(t *testing.T) {
	for _, estado := range []Estado{EstadoPendiente, EstadoRechazada, EstadoBaja} {
		u := &Usuario{Rol: RolDocente, Estado: estado}

		err := u.PromoverAAdmin()

		if !errors.Is(err, ErrPromocionInvalida) {
			t.Errorf("estado %s: esperaba ErrPromocionInvalida, hubo %v", estado, err)
		}
		if u.EsAdmin() {
			t.Errorf("estado %s: no debería haber cambiado el rol", estado)
		}
	}
}

// No es un no-op silencioso: si el Admin apretó "promover" sobre alguien
// que ya lo era, se equivocó de fila y conviene decírselo.
func TestPromoverAAdmin_YaEsAdmin_Error(t *testing.T) {
	u := &Usuario{Rol: RolAdmin, Estado: EstadoAprobada}

	if err := u.PromoverAAdmin(); !errors.Is(err, ErrPromocionInvalida) {
		t.Fatalf("esperaba ErrPromocionInvalida, hubo %v", err)
	}
}

// El mensaje tiene que distinguir los dos motivos: es lo que le dice al
// Admin qué hacer a continuación.
func TestPromoverAAdmin_ElMensajeDiceElMotivo(t *testing.T) {
	yaAdmin := &Usuario{Rol: RolAdmin, Estado: EstadoAprobada}
	pendiente := &Usuario{Rol: RolDocente, Estado: EstadoPendiente}

	if err := yaAdmin.PromoverAAdmin(); !strings.Contains(err.Error(), "ya tiene rol ADMIN") {
		t.Errorf("mensaje poco claro: %v", err)
	}
	if err := pendiente.PromoverAAdmin(); !strings.Contains(err.Error(), "aprobar") {
		t.Errorf("mensaje poco claro: %v", err)
	}
}

// ── DegradarADocente ───────────────────────────────────────────────────

func TestDegradarADocente_AdminAprobado_OK(t *testing.T) {
	u := &Usuario{Rol: RolAdmin, Estado: EstadoAprobada}

	if err := u.DegradarADocente(); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !u.EsDocente() {
		t.Errorf("esperaba rol DOCENTE, quedó %s", u.Rol)
	}
}

// Ida y vuelta: promover y degradar son inversas exactas sobre el rol y no
// dejan rastro en el resto de la cuenta.
func TestDegradarADocente_DeshaceLaPromocion(t *testing.T) {
	u := &Usuario{
		Nombre: "Ada", Email: "ada@escuela.edu.ar", PasswordHash: "hash",
		Rol: RolDocente, Estado: EstadoAprobada,
		CursoSolicitado: "5°A", MateriaSolicitada: "Programación",
	}
	antes := *u

	if err := u.PromoverAAdmin(); err != nil {
		t.Fatalf("promover no debería fallar: %v", err)
	}
	if err := u.DegradarADocente(); err != nil {
		t.Fatalf("degradar no debería fallar: %v", err)
	}

	if *u != antes {
		t.Errorf("la cuenta tendría que haber quedado igual que al principio:\nantes: %+v\nahora: %+v", antes, *u)
	}
}

// Sobre un docente no hay nada que quitar. Mismo criterio que promover a
// alguien que ya es Admin: decirlo, y no un no-op silencioso.
func TestDegradarADocente_YaEsDocente_Error(t *testing.T) {
	u := &Usuario{Rol: RolDocente, Estado: EstadoAprobada}

	if err := u.DegradarADocente(); !errors.Is(err, ErrDegradacionInvalida) {
		t.Fatalf("esperaba ErrDegradacionInvalida, hubo %v", err)
	}
}

// En una cuenta cerrada o pendiente el rol no habilita nada: cambiarlo
// sería tocar el historial sin ningún efecto.
func TestDegradarADocente_CuentaNoAprobada_Error(t *testing.T) {
	for _, estado := range []Estado{EstadoPendiente, EstadoRechazada, EstadoBaja} {
		u := &Usuario{Rol: RolAdmin, Estado: estado}

		err := u.DegradarADocente()

		if !errors.Is(err, ErrDegradacionInvalida) {
			t.Errorf("estado %s: esperaba ErrDegradacionInvalida, hubo %v", estado, err)
		}
		if !u.EsAdmin() {
			t.Errorf("estado %s: no debería haber cambiado el rol", estado)
		}
	}
}

func TestDegradarADocente_ElMensajeDiceElMotivo(t *testing.T) {
	yaDocente := &Usuario{Rol: RolDocente, Estado: EstadoAprobada}
	enBaja := &Usuario{Rol: RolAdmin, Estado: EstadoBaja}

	if err := yaDocente.DegradarADocente(); !strings.Contains(err.Error(), "ya tiene rol DOCENTE") {
		t.Errorf("mensaje poco claro: %v", err)
	}
	if err := enBaja.DegradarADocente(); !strings.Contains(err.Error(), string(EstadoBaja)) {
		t.Errorf("mensaje poco claro: %v", err)
	}
}
