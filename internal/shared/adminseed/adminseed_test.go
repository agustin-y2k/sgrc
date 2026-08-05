package adminseed

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo es un doble de prueba en memoria — sin Postgres, sin red.
type fakeRepo struct {
	existeAdminActivo bool
	errExiste         error
	errCrear          error
	crearLlamado      bool
	emailRecibido     string
	hashRecibido      string
}

func (f *fakeRepo) ExisteAdminActivo(ctx context.Context) (bool, error) {
	return f.existeAdminActivo, f.errExiste
}

func (f *fakeRepo) CrearAdmin(ctx context.Context, email, passwordHash string) error {
	f.crearLlamado = true
	f.emailRecibido = email
	f.hashRecibido = passwordHash
	return f.errCrear
}

func fakeHashOK(password string) (string, error) {
	return "hash-de-" + password, nil
}

func fakeHashFalla(password string) (string, error) {
	return "", errors.New("el hasheo explotó")
}

func TestSembrarSiHaceFalta_YaExisteAdminActivo_NoHaceNada(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: true}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "password123")

	if err != nil {
		t.Fatalf("esperaba nil, obtuve: %v", err)
	}
	if repo.crearLlamado {
		t.Fatal("no debería haber llamado a CrearAdmin si ya existe un admin")
	}
}

func TestSembrarSiHaceFalta_NoExisteAdminActivo_LoCrea(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "password123")

	if err != nil {
		t.Fatalf("esperaba nil, obtuve: %v", err)
	}
	if !repo.crearLlamado {
		t.Fatal("esperaba que CrearAdmin se llamara")
	}
	if repo.emailRecibido != "admin@escuela.edu.ar" {
		t.Errorf("email incorrecto: %q", repo.emailRecibido)
	}
	if repo.hashRecibido != "hash-de-password123" {
		t.Errorf("hash incorrecto: %q", repo.hashRecibido)
	}
}

func TestSembrarSiHaceFalta_EmailVacio_Error(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "", "password123")

	if !errors.Is(err, ErrEnvFaltante) {
		t.Fatalf("esperaba ErrEnvFaltante, obtuve: %v", err)
	}
	if repo.crearLlamado {
		t.Fatal("no debería crear nada si falta el email")
	}
}

func TestSembrarSiHaceFalta_PasswordVacio_Error(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "")

	if !errors.Is(err, ErrEnvFaltante) {
		t.Fatalf("esperaba ErrEnvFaltante, obtuve: %v", err)
	}
}

func TestSembrarSiHaceFalta_PasswordCorta_Error(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "1234567") // 7 chars

	if !errors.Is(err, ErrPasswordCorta) {
		t.Fatalf("esperaba ErrPasswordCorta, obtuve: %v", err)
	}
	if repo.crearLlamado {
		t.Fatal("no debería crear nada con password corta")
	}
}

func TestSembrarSiHaceFalta_PasswordLimiteExacto_NoFalla(t *testing.T) {
	// Exactamente 8 caracteres — el límite no debe rechazarse a sí mismo
	// (bug clásico de off-by-one en validaciones de longitud).
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "12345678")

	if err != nil {
		t.Fatalf("una password de exactamente 8 caracteres no debería fallar: %v", err)
	}
	if !repo.crearLlamado {
		t.Fatal("esperaba que CrearAdmin se llamara con la password límite")
	}
}

func TestSembrarSiHaceFalta_ErrorAlVerificarExistencia_SePropaga(t *testing.T) {
	repo := &fakeRepo{errExiste: errors.New("la base está caída")}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "password123")

	if err == nil {
		t.Fatal("esperaba un error, obtuve nil")
	}
	if repo.crearLlamado {
		t.Fatal("no debería intentar crear nada si falló la verificación previa")
	}
}

func TestSembrarSiHaceFalta_ErrorAlHashear_SePropagaSinCrear(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashFalla, "admin@escuela.edu.ar", "password123")

	if err == nil {
		t.Fatal("esperaba un error de hasheo, obtuve nil")
	}
	if repo.crearLlamado {
		t.Fatal("no debería llamar a CrearAdmin si el hasheo falló antes")
	}
}

func TestSembrarSiHaceFalta_ErrorAlCrear_SePropaga(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false, errCrear: errors.New("email duplicado")}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK, "admin@escuela.edu.ar", "password123")

	if err == nil {
		t.Fatal("esperaba que el error de CrearAdmin se propague")
	}
}

// El caso que este cambio arregla: la base tiene un ADMIN, pero en BAJA. Al
// repo real eso le sale del WHERE (mira estado además de rol); lo que se
// verifica acá es que el seed actúe cuando la respuesta es "no hay ninguno
// activo", en vez de darse por satisfecho con que exista la fila.
func TestSembrarSiHaceFalta_ElUnicoAdminNoEstaActivo_LoVuelveASembrar(t *testing.T) {
	repo := &fakeRepo{existeAdminActivo: false}

	err := SembrarSiHaceFalta(context.Background(), repo, fakeHashOK,
		"admin@escuela.edu.ar", "una-password-larga")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !repo.crearLlamado {
		t.Error("sin ningún admin activo, el sistema queda sin acceso administrativo: hay que sembrarlo")
	}
}
