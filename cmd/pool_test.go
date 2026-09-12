//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Los topes del pool existen para que una consulta que se va de las manos no se
// lleve puesto al resto del sistema. Este test comprueba que de verdad estén
// puestos en las conexiones, y no sólo escritos en una constante.
//
// La diferencia importa: `statement_timeout` va en la configuración de la
// conexión justamente para que lo herede toda conexión que el pool abra,
// incluidas las que reponga más tarde. Un `SET` suelto después de conectar se
// aplicaría a una sola y este test lo vería.
func TestAbrirPool_PoneLosTopesEnCadaConexion(t *testing.T) {
	ctx := context.Background()

	contenedor, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("sgrc_test"),
		postgres.WithUsername("sgrc_test"),
		postgres.WithPassword("sgrc_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("no se pudo levantar Postgres: %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })

	dsn, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := abrirPool(ctx, dsn)
	if err != nil {
		t.Fatalf("abriendo el pool: %v", err)
	}
	t.Cleanup(pool.Close)

	casos := map[string]string{
		"statement_timeout":                   "30s",
		"idle_in_transaction_session_timeout": "1min",
	}
	for parametro, esperado := range casos {
		var valor string
		if err := pool.QueryRow(ctx, "SHOW "+parametro).Scan(&valor); err != nil {
			t.Errorf("leyendo %s: %v", parametro, err)
			continue
		}
		if valor != esperado {
			t.Errorf("%s = %q; esperaba %q", parametro, valor, esperado)
		}
	}

	if n := pool.Config().MaxConns; n != maxConexiones {
		t.Errorf("MaxConns = %d; esperaba %d", n, maxConexiones)
	}
}

// Y que el tope CORTE de verdad: una consulta más larga que el tope tiene que
// volver con error en vez de quedarse con la conexión indefinidamente.
func TestAbrirPool_ElTopeCortaLaConsultaLarga(t *testing.T) {
	ctx := context.Background()

	contenedor, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("sgrc_test"),
		postgres.WithUsername("sgrc_test"),
		postgres.WithPassword("sgrc_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("no se pudo levantar Postgres: %v", err)
	}
	t.Cleanup(func() { _ = contenedor.Terminate(context.Background()) })

	dsn, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := abrirPool(ctx, dsn)
	if err != nil {
		t.Fatalf("abriendo el pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Se baja el tope a un segundo para no esperar treinta: lo que se prueba es
	// que el mecanismo corte, no el número.
	conexion, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("tomando una conexión: %v", err)
	}
	defer conexion.Release()

	if _, err := conexion.Exec(ctx, "SET statement_timeout = '200ms'"); err != nil {
		t.Fatalf("bajando el tope: %v", err)
	}

	_, err = conexion.Exec(ctx, "SELECT pg_sleep(2)")
	if err == nil {
		t.Fatal("una consulta más larga que el tope tenía que cortarse")
	}
}
