package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var ahoraDePrueba = time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC)

func TestNueva_ExigeTexto(t *testing.T) {
	_, err := Nueva("s1", "m1", "u1", TipoProblema, "No me deja reservar", "   ", "/reservas", "1.9.0", ahoraDePrueba)
	if !errors.Is(err, ErrTextoVacio) {
		t.Errorf("esperaba ErrTextoVacio, obtuve %v", err)
	}
}

func TestNueva_ExigeAsunto(t *testing.T) {
	_, err := Nueva("s1", "m1", "u1", TipoAyuda, "  ", "necesito una mano", "", "", ahoraDePrueba)
	if !errors.Is(err, ErrAsuntoVacio) {
		t.Errorf("esperaba ErrAsuntoVacio, obtuve %v", err)
	}
}

func TestNueva_TopeDeLargo(t *testing.T) {
	largo := strings.Repeat("a", MaxTexto+1)
	if _, err := Nueva("s1", "m1", "u1", TipoSugerencia, "Una idea", largo, "", "", ahoraDePrueba); !errors.Is(err, ErrTextoLargo) {
		t.Errorf("esperaba ErrTextoLargo, obtuve %v", err)
	}

	asuntoLargo := strings.Repeat("a", MaxAsunto+1)
	if _, err := Nueva("s1", "m1", "u1", TipoSugerencia, asuntoLargo, "texto", "", "", ahoraDePrueba); !errors.Is(err, ErrAsuntoLargo) {
		t.Errorf("esperaba ErrAsuntoLargo, obtuve %v", err)
	}
}

// La pantalla y la versión las completa la aplicación: sin ellas, un "no
// anda" obliga a ir a buscar a quien lo escribió para preguntarle qué estaba
// haciendo.
func TestNueva_GuardaDesdeDondeSeEscribio(t *testing.T) {
	s, err := Nueva("s1", "m1", "u1", TipoProblema, " No me deja reservar ", " no me deja ", " /reservas/nueva ", "1.9.0", ahoraDePrueba)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if s.Asunto != "No me deja reservar" || s.Pantalla != "/reservas/nueva" {
		t.Errorf("no se limpiaron los espacios: %+v", s)
	}
	if s.Estado != Abierta {
		t.Errorf("una conversación nace abierta, no %q", s.Estado)
	}
	// El primer mensaje ES el del hilo: no hay un "texto" aparte.
	if len(s.Mensajes) != 1 || s.PrimerMensaje().Texto != "no me deja" {
		t.Errorf("el hilo tendría que nacer con su primer mensaje: %+v", s.Mensajes)
	}
	if s.PrimerMensaje().DeAdmin {
		t.Error("el primer mensaje lo escribe quien pide, no administración")
	}
}

// Contestar NO cierra: el Admin puede escribir "fijate en Reservas" y quien
// preguntó tiene que poder decirle "ya probé y no está".
func TestResponder_NoCierraElHilo(t *testing.T) {
	s := abierta(t)

	if err := s.Responder("m2", "admin1", true, " fijate en Reservas ", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if s.Estado != Abierta {
		t.Errorf("contestar no cierra: quedó %q", s.Estado)
	}
	if len(s.Mensajes) != 2 || s.UltimoMensaje().Texto != "fijate en Reservas" {
		t.Errorf("esperaba el mensaje del Admin limpio: %+v", s.Mensajes)
	}
	if !s.UltimoMensaje().DeAdmin {
		t.Error("el mensaje tendría que quedar marcado como de administración")
	}
	// Ahora la pelota está del otro lado.
	if s.EsperaRespuestaDelAdmin() {
		t.Error("acaba de contestar el Admin: no debería figurar como pendiente")
	}
}

func TestResponder_ExigeTexto(t *testing.T) {
	s := abierta(t)
	if err := s.Responder("m2", "admin1", true, "   ", ahoraDePrueba); !errors.Is(err, ErrTextoVacio) {
		t.Errorf("esperaba ErrTextoVacio, obtuve %v", err)
	}
	if len(s.Mensajes) != 1 {
		t.Error("un mensaje inválido no puede quedar en el hilo")
	}
}

// Si volvió a escribir, no estaba resuelto.
func TestResponder_DelDocente_ReabreElHilo(t *testing.T) {
	s := abierta(t)
	if err := s.Responder("m2", "admin1", true, "listo", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if err := s.MarcarResuelta(ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if err := s.Responder("m3", "u1", false, "sigue sin andar", ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if s.Estado != Abierta {
		t.Errorf("escribir en un hilo cerrado tiene que reabrirlo, quedó %q", s.Estado)
	}
	if !s.EsperaRespuestaDelAdmin() {
		t.Error("el último que habló fue quien preguntó: está esperando respuesta")
	}
}

// Dos Admin mirando la lista al mismo tiempo.
func TestMarcarResuelta_UnaSolaVez(t *testing.T) {
	s := abierta(t)
	if err := s.MarcarResuelta(ahoraDePrueba); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if err := s.MarcarResuelta(ahoraDePrueba); !errors.Is(err, ErrYaResuelta) {
		t.Errorf("esperaba ErrYaResuelta, obtuve %v", err)
	}
}

// La actividad es lo que ordena la bandeja: un hilo viejo al que le acaban de
// escribir tiene que subir.
func TestResponder_MueveLaUltimaActividad(t *testing.T) {
	s := abierta(t)
	despues := ahoraDePrueba.Add(48 * time.Hour)

	if err := s.Responder("m2", "admin1", true, "ya lo miramos", despues); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if !s.UltimaActividadEn.Equal(despues) {
		t.Errorf("esperaba %v, obtuve %v", despues, s.UltimaActividadEn)
	}
	if !s.CreadaEn.Equal(ahoraDePrueba) {
		t.Error("la fecha de creación no se toca")
	}
}

func TestParseTipo(t *testing.T) {
	for _, valido := range []string{"AYUDA", "SUGERENCIA", "PROBLEMA"} {
		if _, err := ParseTipo(valido); err != nil {
			t.Errorf("%q debería ser válido: %v", valido, err)
		}
	}
	if _, err := ParseTipo("QUEJA"); !errors.Is(err, ErrTipoInvalido) {
		t.Errorf("esperaba ErrTipoInvalido, obtuve %v", err)
	}
}

// De qué tipo es decide si el correo se puede desactivar, así que la
// distinción no es cosmética.
func TestEsPedidoDeAyuda(t *testing.T) {
	if !TipoAyuda.EsPedidoDeAyuda() {
		t.Error("AYUDA es el pedido de soporte")
	}
	for _, otro := range []Tipo{TipoProblema, TipoSugerencia} {
		if otro.EsPedidoDeAyuda() {
			t.Errorf("%s no es un pedido de ayuda", otro)
		}
	}
}

func abierta(t *testing.T) *Sugerencia {
	t.Helper()
	s, err := Nueva("s1", "m1", "u1", TipoProblema, "No me deja reservar", "algo no anda", "/reservas", "1.9.0", ahoraDePrueba)
	if err != nil {
		t.Fatalf("armando la conversación de prueba: %v", err)
	}
	return s
}
