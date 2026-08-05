// Package adminseed contiene la lógica de decisión de "sembrar el primer
// Admin si hace falta" (RF-01.4), aislada a propósito sin ninguna
// dependencia externa (ni pgx, ni argon2) — solo stdlib. Eso permite
// testearla con un repo/hash falsos, sin necesitar Postgres real ni
// resolver dependencias de red. La implementación real contra Postgres y
// argon2id vive en cmd/ (ver cmd/seed_admin.go), que implementa la
// interfaz Repo y provee la función de hash concreta.
package adminseed

import (
	"context"
	"errors"
	"fmt"
)

// ErrEnvFaltante: SEED_ADMIN_EMAIL o SEED_ADMIN_PASSWORD no están seteados.
var ErrEnvFaltante = errors.New("SEED_ADMIN_EMAIL / SEED_ADMIN_PASSWORD no están configurados")

// ErrPasswordCorta: la contraseña inicial no cumple el mínimo de seguridad.
// Mismo mínimo que RF-01.6/08-api-spec.yaml (minLength: 8) para no crear
// un admin con una contraseña más débil que la que se le exige a cualquier
// otro usuario al cambiarla.
var ErrPasswordCorta = errors.New("la contraseña del admin inicial debe tener al menos 8 caracteres")

const minPasswordLen = 8

// Repo es el único contrato que necesita este paquete — cmd/seed_admin.go
// lo implementa contra Postgres real.
type Repo interface {
	// ExisteAdminActivo: si hay al menos un ADMIN que pueda entrar hoy.
	//
	// El estado importa y antes no se miraba: se contaban los ADMIN sin
	// filtrar, así que una base cuyo único Admin quedó en BAJA (o RECHAZADA)
	// se veía como "ya tiene admin" y el seed no hacía nada. El sistema
	// quedaba sin ningún acceso administrativo y sin forma de recuperarlo
	// desde la aplicación: nadie podía aprobar cuentas, y el único camino
	// era SQL a mano contra producción.
	ExisteAdminActivo(ctx context.Context) (bool, error)

	// CrearAdmin siembra el Admin inicial. Solo se llama cuando no hay
	// ninguno activo.
	CrearAdmin(ctx context.Context, email, passwordHash string) error
}

// HashFunc hashea una contraseña en texto plano. Inyectada en vez de
// llamada directamente para poder testear la lógica de decisión con un
// hash falso, sin depender de argon2 real.
type HashFunc func(password string) (string, error)

// SembrarSiHaceFalta es la lógica completa de RF-01.4, sin tocar Postgres
// ni argon2 directamente — todo eso llega inyectado.
//
// Reglas, en orden:
//  1. Si ya existe un ADMIN que pueda entrar, no hace nada (idempotente).
//  2. Si no hay email/password configurados, error claro (no falla en
//     silencio ni crea un admin con datos vacíos).
//  3. Si la contraseña es demasiado corta, error claro (no crea un admin
//     débil solo porque "algo" se configuró).
//  4. Hashea y crea. Si el hash falla, o si CrearAdmin falla (ej. el email
//     ya existe con otro rol), se propaga el error tal cual — nunca se
//     traga un error silenciosamente.
func SembrarSiHaceFalta(ctx context.Context, repo Repo, hash HashFunc, email, password string) error {
	existe, err := repo.ExisteAdminActivo(ctx)
	if err != nil {
		return fmt.Errorf("verificando si ya existe un admin activo: %w", err)
	}
	if existe {
		return nil
	}

	if email == "" || password == "" {
		return ErrEnvFaltante
	}
	if len(password) < minPasswordLen {
		return ErrPasswordCorta
	}

	hashed, err := hash(password)
	if err != nil {
		return fmt.Errorf("hasheando password del admin inicial: %w", err)
	}

	if err := repo.CrearAdmin(ctx, email, hashed); err != nil {
		return fmt.Errorf("creando admin inicial: %w", err)
	}
	return nil
}
