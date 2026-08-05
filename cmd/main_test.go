package main

import "testing"

// buildDSN se testea porque el modo de falla es especialmente ingrato: una
// contraseña "buena" (larga y aleatoria, como pide RNF-04) tiene muchas
// más chances de traer un carácter reservado de URL que una escrita a mano,
// así que la versión concatenada rompía justo cuando alguien hacía lo
// correcto — y el síntoma es un error de conexión que no menciona la
// contraseña por ningún lado.
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
//
// origenDelFrontend llama a log.Fatal en los casos inválidos, así que el
// test solo puede cubrir los válidos sin matar el proceso de test. Es
// suficiente para lo que importa acá: que un origen bien formado pase tal
// cual (sin recortes ni agregados) hacia el middleware de CORS, que lo
// compara byte a byte contra el header Origin del navegador.
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
