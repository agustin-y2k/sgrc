package main

import (
	"testing"
	"time"
)

// buildDSN se testea porque el modo de falla es especialmente ingrato: una
// contraseña "buena" (larga y aleatoria, como pide RNF-04) tiene muchas más
// chances de traer un carácter reservado de URL que una escrita a mano, así
// que la versión concatenada rompía justo cuando alguien hacía lo correcto —
// y el síntoma es un error de conexión que no menciona la contraseña por
// ningún lado.
func TestBuildDSN_EscapaCaracteresReservadosDeLaContraseña(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "sgrc_db")
	t.Setenv("POSTGRES_USER", "sgrc_app")
	// Un `@` parte el host, un `/` parte el nombre de la base, un `#`
	// trunca todo lo que sigue.
	t.Setenv("POSTGRES_PASSWORD", "p@ss/w#rd:1")

	dsn := buildDSN()

	esperado := "postgres://sgrc_app:p%40ss%2Fw%23rd%3A1@postgres:5432/sgrc_db?sslmode=disable"
	if dsn != esperado {
		t.Errorf("DSN mal armado:\n  obtuve:   %s\n  esperaba: %s", dsn, esperado)
	}
}

func TestBuildDSN_CasoSimple(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "sgrc_db")
	t.Setenv("POSTGRES_USER", "sgrc_app")
	t.Setenv("POSTGRES_PASSWORD", "secreto")

	if dsn := buildDSN(); dsn != "postgres://sgrc_app:secreto@postgres:5432/sgrc_db?sslmode=disable" {
		t.Errorf("DSN mal armado: %s", dsn)
	}
}

// ── FRONTEND_ORIGIN ────────────────────────────────────────────────────
// origenDelFrontend llama a log.Fatal en los casos inválidos, así que el test
// solo puede cubrir los válidos sin matar el proceso de test.
func TestOrigenDelFrontend_ValoresValidos(t *testing.T) {
	casos := []struct{ env, esperado string }{
		{"https://sgrc.tuinstitucion.edu.ar", "https://sgrc.tuinstitucion.edu.ar"},
		{"http://localhost:5173", "http://localhost:5173"},
		// Los espacios alrededor son el error de tipeo más común al pegar el
		// valor en el .env, y no cambian el origen: se recortan.
		{"  https://sgrc.tuinstitucion.edu.ar  ", "https://sgrc.tuinstitucion.edu.ar"},
	}

	for _, c := range casos {
		t.Setenv("FRONTEND_ORIGIN", c.env)
		if obtenido := origenDelFrontend(); obtenido != c.esperado {
			t.Errorf("origenDelFrontend() con %q = %q, esperaba %q", c.env, obtenido, c.esperado)
		}
	}
}

// LICENCIAS_INTERVALO_MINUTOS existe para poder ver el ciclo de un aviso sin
// esperar una hora. Lo que este test protege es el DEFAULT: si un despliegue
// que no la declara terminara revisando cada minuto, el job pasaría de correr
// 24 veces por día a 1440 sin que nadie lo haya pedido.
func TestIntervaloDeLicencias_DefaultYValores(t *testing.T) {
	casos := []struct {
		nombre   string
		env      string
		esperado time.Duration
	}{
		{"sin declarar, el default", "", time.Hour},
		{"vacía es como no declararla", "   ", time.Hour},
		{"el valor de prueba", "1", time.Minute},
		{"un valor intermedio", "15", 15 * time.Minute},
		{"el tope, un día", "1440", 24 * time.Hour},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Setenv("LICENCIAS_INTERVALO_MINUTOS", c.env)
			if obtenido := intervaloDeLicencias(); obtenido != c.esperado {
				t.Errorf("intervaloDeLicencias() con %q = %v, esperaba %v", c.env, obtenido, c.esperado)
			}
		})
	}
}

// LICENCIAS_HORA_AVISO decide a partir de qué hora puede salir el aviso del
// día. Nunca tuvo test, y su default importa: con un 0 mal leído el correo
// saldría a medianoche.
func TestHoraAvisoLicencias_DefaultYValores(t *testing.T) {
	casos := []struct {
		env      string
		esperado int
	}{
		{"", horaAvisoLicenciasPorDefecto},
		{"  ", horaAvisoLicenciasPorDefecto},
		{"0", 0},
		{"7", 7},
		{"23", 23},
	}

	for _, c := range casos {
		t.Setenv("LICENCIAS_HORA_AVISO", c.env)
		if obtenido := horaAvisoLicencias(); obtenido != c.esperado {
			t.Errorf("horaAvisoLicencias() con %q = %d, esperaba %d", c.env, obtenido, c.esperado)
		}
	}
}
