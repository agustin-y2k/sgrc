package email

import (
	"context"
	"strings"
	"testing"
	"time"
)

func enviadorDePrueba() *EnviadorSMTP {
	fijo := time.Date(2026, 8, 6, 14, 30, 0, 0, time.UTC)
	return NewEnviadorSMTP(Config{
		Host:     "smtp.gmail.com",
		Puerto:   "587",
		Usuario:  "escuela@gmail.com",
		Password: "secreto",
		Desde:    "escuela@gmail.com",
		// Con acento y con "°" a propósito: son los caracteres que obligan a
		// codificar la cabecera (RFC 2047).
		NombreDeQuienEnvia: "Escuela Técnica N° 1",
	}, func() time.Time { return fijo })
}

func TestArmarMensaje_CodificaElAsuntoConAcentos(t *testing.T) {
	msg := string(enviadorDePrueba().armarMensaje("ana@escuela.edu.ar", "Restablecé tu contraseña", "hola"))

	// El asunto crudo no puede aparecer: las cabeceras son ASCII, y sin
	// codificar los acentos llegan rotos.
	if strings.Contains(msg, "Subject: Restablecé") {
		t.Error("el asunto salió sin codificar (RFC 2047)")
	}
	if !strings.Contains(msg, "Subject: =?utf-8?q?") {
		t.Errorf("esperaba el asunto codificado, obtuve:\n%s", msg)
	}
}

func TestArmarMensaje_NombreDelRemitenteCodificadoYConLaDireccion(t *testing.T) {
	msg := string(enviadorDePrueba().armarMensaje("ana@escuela.edu.ar", "Hola", "hola"))

	if !strings.Contains(msg, "<escuela@gmail.com>") {
		t.Errorf("falta la dirección del remitente:\n%s", msg)
	}
	if !strings.Contains(msg, "From: =?utf-8?q?") {
		t.Errorf("el nombre del remitente lleva acento y ° : debería ir codificado:\n%s", msg)
	}
}

func TestArmarMensaje_LlevaMessageIDConElDominioDelRemitente(t *testing.T) {
	msg := string(enviadorDePrueba().armarMensaje("ana@escuela.edu.ar", "Hola", "hola"))

	if !strings.Contains(msg, "Message-ID: <") || !strings.Contains(msg, "@gmail.com>") {
		t.Errorf("esperaba un Message-ID con el dominio del remitente, obtuve:\n%s", msg)
	}
}

func TestArmarMensaje_DosMensajesNoCompartenMessageID(t *testing.T) {
	// El reloj está fijo a propósito: si el ID dependiera solo del instante,
	// dos avisos de la misma barrida saldrían con el mismo, y hay clientes
	// que en ese caso muestran uno solo.
	e := enviadorDePrueba()
	uno := extraerCabecera(t, string(e.armarMensaje("ana@escuela.edu.ar", "Hola", "hola")), "Message-ID")
	otro := extraerCabecera(t, string(e.armarMensaje("beto@escuela.edu.ar", "Hola", "hola")), "Message-ID")

	if uno == otro {
		t.Errorf("dos correos salieron con el mismo Message-ID: %s", uno)
	}
}

func TestArmarMensaje_SinArrobaEnElRemitenteCaeAlHost(t *testing.T) {
	e := NewEnviadorSMTP(Config{Host: "smtp.gmail.com", Desde: "escuela"}, nil)
	msg := string(e.armarMensaje("ana@escuela.edu.ar", "Hola", "hola"))

	if !strings.Contains(msg, "@smtp.gmail.com>") {
		t.Errorf("esperaba el host como dominio del Message-ID, obtuve:\n%s", msg)
	}
}

func extraerCabecera(t *testing.T, mensaje, clave string) string {
	t.Helper()
	for _, linea := range strings.Split(mensaje, "\r\n") {
		if valor, hay := strings.CutPrefix(linea, clave+": "); hay {
			return valor
		}
	}
	t.Fatalf("no encontré la cabecera %q en:\n%s", clave, mensaje)
	return ""
}

func TestArmarMensaje_SinNombreDeRemitenteVaSoloLaDireccion(t *testing.T) {
	e := NewEnviadorSMTP(Config{Host: "smtp.gmail.com", Desde: "escuela@gmail.com"}, nil)
	msg := string(e.armarMensaje("ana@escuela.edu.ar", "Hola", "hola"))

	if !strings.Contains(msg, "From: escuela@gmail.com\r\n") {
		t.Errorf("esperaba el From pelado, obtuve:\n%s", msg)
	}
}

func TestArmarMensaje_TodasLasLineasTerminanEnCRLF(t *testing.T) {
	// El cuerpo se escribe con \n comunes en el código; el RFC 5321 pide
	// CRLF y hay servidores que cortan la sesión sin explicar por qué.
	cuerpo := "Hola Ana:\n\nTu código es 123456.\n"
	msg := string(enviadorDePrueba().armarMensaje("ana@escuela.edu.ar", "Hola", cuerpo))

	for i, linea := range strings.Split(msg, "\r\n") {
		if strings.Contains(linea, "\n") {
			t.Fatalf("la línea %d quedó con un LF suelto: %q", i, linea)
		}
	}
}

func TestArmarMensaje_SeparaCabecerasDelCuerpoConUnaLineaVacia(t *testing.T) {
	msg := string(enviadorDePrueba().armarMensaje("ana@escuela.edu.ar", "Hola", "el cuerpo"))

	corte := strings.Index(msg, "\r\n\r\n")
	if corte < 0 {
		t.Fatalf("no hay separación entre cabeceras y cuerpo:\n%s", msg)
	}
	if strings.Contains(msg[:corte], "el cuerpo") {
		t.Error("el cuerpo se filtró entre las cabeceras")
	}
	if !strings.Contains(msg[corte:], "el cuerpo") {
		t.Errorf("el cuerpo no aparece después de las cabeceras:\n%s", msg)
	}
}

func TestEnviar_SinDestinatarioNoAbreConexion(t *testing.T) {
	// El host no existe: si intentara conectar, el error sería de red y no
	// ErrSinDestinatario.
	e := NewEnviadorSMTP(Config{Host: "no.existe.invalido", Puerto: "587", Desde: "a@b.com"}, nil)

	if err := e.Enviar(context.Background(), "   ", "Hola", "cuerpo"); err != ErrSinDestinatario {
		t.Fatalf("esperaba ErrSinDestinatario, obtuve %v", err)
	}
}

func TestDeshabilitado_NoFallaNiEnvia(t *testing.T) {
	if err := (Deshabilitado{}).Enviar(context.Background(), "ana@escuela.edu.ar", "Hola", "cuerpo"); err != nil {
		t.Fatalf("el enviador deshabilitado no debería fallar: %v", err)
	}
}

// ══════════════════════════════════════════════════════════════════
// DesdeEntorno
// ══════════════════════════════════════════════════════════════════

func entorno(pares map[string]string) func(string) string {
	return func(clave string) string { return pares[clave] }
}

func TestDesdeEntorno_SinHostQuedaDeshabilitado(t *testing.T) {
	env, err := DesdeEntorno(entorno(map[string]string{}), nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if _, ok := env.(Deshabilitado); !ok {
		t.Fatalf("esperaba el enviador deshabilitado, obtuve %T", env)
	}
}

func TestDesdeEntorno_PuertoPorDefecto587(t *testing.T) {
	env, err := DesdeEntorno(entorno(map[string]string{
		"SMTP_HOST": "smtp.gmail.com",
		"SMTP_FROM": "escuela@gmail.com",
	}), nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if env.(*EnviadorSMTP).cfg.Puerto != "587" {
		t.Errorf("esperaba el puerto 587, obtuve %q", env.(*EnviadorSMTP).cfg.Puerto)
	}
}

func TestDesdeEntorno_SinFromUsaElUsuario(t *testing.T) {
	env, err := DesdeEntorno(entorno(map[string]string{
		"SMTP_HOST":     "smtp.gmail.com",
		"SMTP_USER":     "escuela@gmail.com",
		"SMTP_PASSWORD": "abcd",
	}), nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if desde := env.(*EnviadorSMTP).cfg.Desde; desde != "escuela@gmail.com" {
		t.Errorf("esperaba que el From cayera en el usuario, obtuve %q", desde)
	}
}

func TestDesdeEntorno_HostSinFromNiUsuarioEsError(t *testing.T) {
	_, err := DesdeEntorno(entorno(map[string]string{"SMTP_HOST": "smtp.gmail.com"}), nil)
	if err == nil {
		t.Fatal("una configuración a medias tiene que abortar el arranque, no fallar en cada envío")
	}
}

func TestDesdeEntorno_UsuarioSinPasswordEsError(t *testing.T) {
	_, err := DesdeEntorno(entorno(map[string]string{
		"SMTP_HOST": "smtp.gmail.com",
		"SMTP_USER": "escuela@gmail.com",
		"SMTP_FROM": "escuela@gmail.com",
	}), nil)
	if err == nil {
		t.Fatal("con usuario y sin contraseña la autenticación falla siempre: tiene que abortar el arranque")
	}
}

func TestDesdeEntorno_PuertoInvalido(t *testing.T) {
	for _, puerto := range []string{"no-es-un-numero", "0", "99999", "-1"} {
		_, err := DesdeEntorno(entorno(map[string]string{
			"SMTP_HOST": "smtp.gmail.com",
			"SMTP_FROM": "escuela@gmail.com",
			"SMTP_PORT": puerto,
		}), nil)
		if err == nil {
			t.Errorf("el puerto %q debería ser rechazado", puerto)
		}
	}
}

func TestDesdeEntorno_LeQuitaLosEspaciosALaClaveDeAplicacionDeGmail(t *testing.T) {
	// Google la muestra como "abcd efgh ijkl mnop" y se copia con espacios.
	env, err := DesdeEntorno(entorno(map[string]string{
		"SMTP_HOST":     "smtp.gmail.com",
		"SMTP_USER":     "escuela@gmail.com",
		"SMTP_PASSWORD": "abcd efgh ijkl mnop",
	}), nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pass := env.(*EnviadorSMTP).cfg.Password; pass != "abcdefghijklmnop" {
		t.Errorf("esperaba la clave sin espacios, obtuve %q", pass)
	}
}

func TestDesdeEntorno_EnOtrosServidoresLaClaveNoSeToca(t *testing.T) {
	// Fuera de Gmail, un espacio puede ser parte de la contraseña de verdad.
	env, err := DesdeEntorno(entorno(map[string]string{
		"SMTP_HOST":     "smtp.otroproveedor.com",
		"SMTP_USER":     "escuela",
		"SMTP_PASSWORD": "con espacio",
	}), nil)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if pass := env.(*EnviadorSMTP).cfg.Password; pass != "con espacio" {
		t.Errorf("esperaba la clave intacta, obtuve %q", pass)
	}
}
