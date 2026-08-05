package security

import (
	"strings"
	"testing"
)

func TestHashYVerificar_RoundTrip_OK(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("no se pudo hashear: %v", err)
	}

	ok, err := VerifyPassword("password123", hash)
	if err != nil {
		t.Fatalf("no debería fallar la verificación: %v", err)
	}
	if !ok {
		t.Fatal("la contraseña correcta debería verificar OK")
	}
}

func TestVerificar_PasswordIncorrecta_Falla(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("no se pudo hashear: %v", err)
	}

	ok, err := VerifyPassword("password-incorrecta", hash)
	if err != nil {
		t.Fatalf("no debería devolver error, solo false: %v", err)
	}
	if ok {
		t.Fatal("una contraseña incorrecta nunca debería verificar OK")
	}
}

func TestHash_DosLlamadasMismaPassword_SaltDistinto(t *testing.T) {
	hash1, _ := HashPassword("password123")
	hash2, _ := HashPassword("password123")

	if hash1 == hash2 {
		t.Fatal("dos hashes de la misma contraseña no deberían ser idénticos (salt debe ser aleatorio)")
	}

	ok1, _ := VerifyPassword("password123", hash1)
	ok2, _ := VerifyPassword("password123", hash2)
	if !ok1 || !ok2 {
		t.Fatal("ambos hashes independientes deberían verificar la misma contraseña")
	}
}

func TestVerificar_FormatoInvalido_ErrorNoPanic(t *testing.T) {
	casos := []string{
		"",
		"esto-no-tiene-el-formato-correcto",
		"$argon2id$solo-tres-partes",
		"$bcrypt$v=19$m=1,t=1,p=1$salt$hash", // algoritmo equivocado
	}

	for _, c := range casos {
		ok, err := VerifyPassword("cualquier-cosa", c)
		if err == nil {
			t.Errorf("caso %q: esperaba error, obtuve nil", c)
		}
		if ok {
			t.Errorf("caso %q: esperaba false, obtuve true", c)
		}
	}
}

func TestVerificar_Base64Corrupto_ErrorNoPanic(t *testing.T) {
	hash, _ := HashPassword("password123")
	partes := strings.Split(hash, "$")
	partes[4] = "!!!no-es-base64-valido!!!"
	corrupto := strings.Join(partes, "$")

	ok, err := VerifyPassword("password123", corrupto)
	if err == nil {
		t.Fatal("esperaba error con base64 corrupto")
	}
	if ok {
		t.Fatal("no debería verificar OK con un hash corrupto")
	}
}

func TestVerificar_ParametrosEnCero_Rechazado(t *testing.T) {
	fake := "$argon2id$v=19$m=0,t=0,p=0$c29tZXNhbHQ$c29tZWhhc2g"

	ok, err := VerifyPassword("cualquier-cosa", fake)
	if err == nil {
		t.Fatal("esperaba error con parámetros en cero")
	}
	if ok {
		t.Fatal("no debería verificar OK con parámetros degenerados")
	}
}

func TestHash_TamanioDelHashEsConsistente(t *testing.T) {
	hash, err := HashPassword("cualquier-password")
	if err != nil {
		t.Fatalf("no se pudo hashear: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Fatalf("el hash no tiene el prefijo esperado: %q", hash)
	}
	if len(strings.Split(hash, "$")) != 6 {
		t.Fatalf("el hash no tiene la cantidad de partes esperada: %q", hash)
	}
}

func TestHashPassword_PasswordVacia_NoPanikeaYGeneraHashValido(t *testing.T) {
	// HashPassword no impone la política de longitud mínima — eso vive en
	// internal/shared/adminseed (para el seed) e internal/auth/application
	// (para registro/reset normal). Acá solo confirmamos que la función de
	// bajo nivel no panickea con un input vacío, ya que en teoría podría
	// llegar uno si algún caller de más arriba no valida correctamente.
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("no debería fallar con password vacía: %v", err)
	}

	ok, err := VerifyPassword("", hash)
	if err != nil {
		t.Fatalf("no debería fallar verificando password vacía: %v", err)
	}
	if !ok {
		t.Fatal("una password vacía debería verificar OK contra su propio hash")
	}

	ok, err = VerifyPassword("no-vacia", hash)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if ok {
		t.Fatal("una password distinta a la vacía no debería verificar OK")
	}
}
