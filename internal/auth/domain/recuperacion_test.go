package domain

import (
	"errors"
	"testing"
	"time"
)

var momento = time.Date(2026, 8, 6, 14, 0, 0, 0, time.UTC)

func codigoDePrueba(t *testing.T) *CodigoRecuperacion {
	t.Helper()
	c, err := NuevoCodigoRecuperacion("cod-1", "usr-1", "hash-del-codigo", momento)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	return c
}

func TestNuevoCodigoRecuperacion_CalculaElVencimiento(t *testing.T) {
	c := codigoDePrueba(t)

	if esperado := momento.Add(VigenciaCodigoRecuperacion); !c.ExpiraEn.Equal(esperado) {
		t.Errorf("esperaba que venciera a las %s, vence a las %s", esperado, c.ExpiraEn)
	}
	if c.UsadoEn != nil {
		t.Error("un código recién creado no puede estar usado")
	}
	if c.Intentos != 0 {
		t.Errorf("esperaba 0 intentos, hay %d", c.Intentos)
	}
}

func TestNuevoCodigoRecuperacion_RechazaDatosIncompletos(t *testing.T) {
	casos := map[string][3]string{
		"sin id":      {"", "usr-1", "hash"},
		"sin usuario": {"cod-1", "", "hash"},
		"sin hash":    {"cod-1", "usr-1", ""},
	}
	for nombre, args := range casos {
		if _, err := NuevoCodigoRecuperacion(args[0], args[1], args[2], momento); err == nil {
			t.Errorf("%s: esperaba error", nombre)
		}
	}
}

func TestUtilizable_RecienCreadoSirve(t *testing.T) {
	if err := codigoDePrueba(t).Utilizable(momento); err != nil {
		t.Fatalf("un código recién creado tiene que servir: %v", err)
	}
}

func TestUtilizable_JustoAlVencerYaNoSirve(t *testing.T) {
	c := codigoDePrueba(t)

	// Un instante antes todavía sirve.
	if err := c.Utilizable(c.ExpiraEn.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("un instante antes de vencer tiene que servir: %v", err)
	}
	// Exactamente en el vencimiento, ya no: la ventana se cumplió.
	if err := c.Utilizable(c.ExpiraEn); !errors.Is(err, ErrCodigoExpirado) {
		t.Fatalf("esperaba ErrCodigoExpirado en el instante exacto del vencimiento, obtuve %v", err)
	}
}

func TestUtilizable_UsadoNoSirveAunqueNoHayaVencido(t *testing.T) {
	c := codigoDePrueba(t)
	if err := c.Usar(momento); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := c.Utilizable(momento); !errors.Is(err, ErrCodigoInvalido) {
		t.Fatalf("esperaba ErrCodigoInvalido, obtuve %v", err)
	}
}

func TestRegistrarFallo_QuemaElCodigoAlLlegarAlTope(t *testing.T) {
	c := codigoDePrueba(t)

	for i := 1; i < MaxIntentosCodigoRecuperacion; i++ {
		if quemado := c.RegistrarFallo(momento); quemado {
			t.Fatalf("se quemó en el intento %d, esperaba que aguantara hasta %d", i, MaxIntentosCodigoRecuperacion)
		}
		if err := c.Utilizable(momento); err != nil {
			t.Fatalf("después de %d fallos todavía tendría que servir: %v", i, err)
		}
	}

	if quemado := c.RegistrarFallo(momento); !quemado {
		t.Fatalf("el intento %d tendría que quemar el código", MaxIntentosCodigoRecuperacion)
	}
	if c.UsadoEn == nil {
		t.Error("un código quemado tiene que quedar marcado como consumido")
	}
}

func TestRegistrarFallo_ElCodigoQuemadoNoSeReutiliza(t *testing.T) {
	c := codigoDePrueba(t)
	for i := 0; i < MaxIntentosCodigoRecuperacion; i++ {
		c.RegistrarFallo(momento)
	}

	// Que el error sea "inválido" y no "sin intentos" es correcto: al quemarse
	// quedó marcado como usado, y Utilizable evalúa eso primero.
	if err := c.Utilizable(momento); err == nil {
		t.Fatal("un código quemado no puede volver a validar")
	}
	if err := c.Usar(momento); err == nil {
		t.Fatal("un código quemado no se puede consumir")
	}
}

func TestUsar_DosVecesFalla(t *testing.T) {
	c := codigoDePrueba(t)

	if err := c.Usar(momento); err != nil {
		t.Fatalf("el primer uso tiene que funcionar: %v", err)
	}
	if err := c.Usar(momento); err == nil {
		t.Fatal("el segundo uso tiene que fallar: el código sirve UNA vez")
	}
}

func TestUsar_VencidoFalla(t *testing.T) {
	c := codigoDePrueba(t)

	err := c.Usar(c.ExpiraEn.Add(time.Second))
	if !errors.Is(err, ErrCodigoExpirado) {
		t.Fatalf("esperaba ErrCodigoExpirado, obtuve %v", err)
	}
	if c.UsadoEn != nil {
		t.Error("un uso fallido no puede consumir el código")
	}
}
