//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// EnTransaccion de inventory, contra Postgres real.
//
// Se prueba acá y no con un fake porque lo que hay que verificar es que el
// ROLLBACK de verdad ocurra: un fake que restaura un mapa puede decir que sí
// mientras la transacción real ni se abrió.

// equipoDePrueba deja un carro con un equipo y devuelve el id del equipo.
func equipoDePrueba(t *testing.T, repo *PostgresRepo, identificador int) string {
	t.Helper()
	ctx := context.Background()

	carro, err := domain.NuevoCarro(NuevoID(), "Carro "+string(rune('A'+identificador)), "")
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearCarro(ctx, carro); err != nil {
		t.Fatalf("creando carro: %v", err)
	}
	e, err := domain.NuevoEquipoDeCarro(NuevoID(), carro.ID, identificador,
		"SERIE-"+string(rune('A'+identificador)), false, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearEquipo(ctx, e); err != nil {
		t.Fatalf("creando equipo: %v", err)
	}
	return e.ID
}

func contarLicencias(t *testing.T, repo *PostgresRepo) int {
	t.Helper()
	todas, err := repo.ListarLicencias(context.Background())
	if err != nil {
		t.Fatalf("listando licencias: %v", err)
	}
	return len(todas)
}

// Lo que esto fija: si algo falla en el medio del lote, no queda NADA escrito.
// Antes de EnTransaccion, las filas anteriores al error quedaban commiteadas y
// el Admin recibía un error sin forma de saber cuáles.
func TestEnTransaccion_DeshaceLoEscritoAntesDelError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	equipo1 := equipoDePrueba(t, repo, 1)
	equipo2 := equipoDePrueba(t, repo, 2)
	seRompio := errors.New("se cayó la conexión")

	err := repo.EnTransaccion(ctx, func(tx application.Repo) error {
		for _, equipoID := range []string{equipo1, equipo2} {
			l, err := domain.NuevaLicencia(NuevoID(), equipoID, "AutoCAD 2027", 30, 1, time.Now())
			if err != nil {
				return err
			}
			creada, err := tx.CrearLicencia(ctx, l)
			if err != nil {
				return err
			}
			if !creada {
				t.Errorf("la licencia en %s tenía que crearse", equipoID)
			}
		}
		return seRompio
	})

	if !errors.Is(err, seRompio) {
		t.Fatalf("esperaba que el error viajara tal cual, obtuve: %v", err)
	}
	if n := contarLicencias(t, repo); n != 0 {
		t.Errorf("quedaron %d licencias escritas; la transacción tenía que deshacerlas", n)
	}
}

func TestEnTransaccion_ConfirmaCuandoNoHayError(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	equipo := equipoDePrueba(t, repo, 1)

	err := repo.EnTransaccion(ctx, func(tx application.Repo) error {
		l, err := domain.NuevaLicencia(NuevoID(), equipo, "AutoCAD 2027", 30, 1, time.Now())
		if err != nil {
			return err
		}
		_, err = tx.CrearLicencia(ctx, l)
		return err
	})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if n := contarLicencias(t, repo); n != 1 {
		t.Errorf("esperaba 1 licencia commiteada, hay %d", n)
	}
}

// El caso que obligó a cambiar CrearLicencia: en Postgres una sentencia que
// falla aborta la transacción ENTERA, así que resolver el duplicado dejando
// reventar el INSERT y atrapando el 23505 haría que todo lo que viene después
// del primer repetido muera con "current transaction is aborted".
//
// Con ON CONFLICT DO NOTHING no hay sentencia fallida: el repetido se saltea y
// la transacción sigue viva. Esto lo comprueba.
func TestEnTransaccion_ElDuplicadoNoMataLaTransaccion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	equipo1 := equipoDePrueba(t, repo, 1)
	equipo2 := equipoDePrueba(t, repo, 2)

	// El primero ya tiene la licencia cargada de antes.
	previa, err := domain.NuevaLicencia(NuevoID(), equipo1, "AutoCAD 2027", 30, 1, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if _, err := repo.CrearLicencia(ctx, previa); err != nil {
		t.Fatalf("creando la licencia previa: %v", err)
	}

	var salteados, creados int
	err = repo.EnTransaccion(ctx, func(tx application.Repo) error {
		// El repetido va PRIMERO a propósito: si abortara la transacción, el
		// segundo moriría y este test lo vería.
		for _, equipoID := range []string{equipo1, equipo2} {
			l, err := domain.NuevaLicencia(NuevoID(), equipoID, "AutoCAD 2027", 30, 1, time.Now())
			if err != nil {
				return err
			}
			creada, err := tx.CrearLicencia(ctx, l)
			if err != nil {
				return err
			}
			if creada {
				creados++
			} else {
				salteados++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("el duplicado no tenía que voltear la transacción: %v", err)
	}
	if creados != 1 || salteados != 1 {
		t.Errorf("esperaba 1 creada y 1 salteada, obtuve %d y %d", creados, salteados)
	}
	if n := contarLicencias(t, repo); n != 2 {
		t.Errorf("esperaba 2 licencias en total, hay %d", n)
	}
}
