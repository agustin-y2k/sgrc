//go:build integration

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	authapp "github.com/agustin-y2k/sgrc/internal/auth/application"
	authdomain "github.com/agustin-y2k/sgrc/internal/auth/domain"
	authinfra "github.com/agustin-y2k/sgrc/internal/auth/infrastructure"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func servicioRecuperacionReal(pool *pgxpool.Pool) *authapp.Service {
	return servicioRecuperacionConBus(pool, eventbus.NewInMemoryEventBus())
}
func servicioRecuperacionConBus(pool *pgxpool.Pool, bus eventbus.EventBus) *authapp.Service {
	return authapp.NewService(authinfra.NewPostgresRepo(pool), bus, func(s string) (string, error) { return "hash:" + s, nil }, func(s, h string) (bool, error) { return h == "hash:"+s, nil }, nil, uuid.NewString, nil, func() (string, error) { return "123456", nil }, func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }, nil, nil, nil, true)
}
func codigoRecuperacionReal(t *testing.T, pool *pgxpool.Pool, usuario string) *authdomain.CodigoRecuperacion {
	t.Helper()
	c, err := authdomain.NuevoCodigoRecuperacion(uuid.NewString(), usuario, "hash:123456", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := authinfra.NewPostgresRepo(pool).CrearCodigoRecuperacion(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestRecuperacionSerializada_ReemplazoIntentosConsumoYRollback(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	for _, modo := range []string{"reemplazos", "intentos", "consumo", "rollback"} {
		t.Run(modo, func(t *testing.T) {
			e := prepararBajaReal(t, pool, false)
			svc := servicioRecuperacionReal(pool)
			repo := authinfra.NewPostgresRepo(pool)
			c := codigoRecuperacionReal(t, pool, e.usuario)
			if modo == "rollback" {
				quitar := falloBajaSQL(t, pool, "usuario", "UPDATE", "")
				_, err := svc.RestablecerPasswordConCodigo(ctx, "ada@test.local", "123456", "password-nueva")
				quitar()
				if err == nil {
					t.Fatal("se ocultó el fallo de contraseña")
				}
				recargado, err := repo.BuscarCodigoVigenteDe(ctx, e.usuario)
				if err != nil || recargado.ID != c.ID || recargado.UsadoEn != nil {
					t.Fatalf("consumió un código sin cambiar la contraseña: %+v %v", recargado, err)
				}
				return
			}
			n := 5
			if modo == "consumo" {
				n = 2
			}
			var wg sync.WaitGroup
			resultados := make(chan error, n)
			inicio := make(chan struct{})
			for range n {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-inicio
					if modo == "reemplazos" {
						nuevo, err := authdomain.NuevoCodigoRecuperacion(uuid.NewString(), e.usuario, "hash:654321", c.CreadoEn.Add(time.Second))
						if err == nil {
							err = repo.CrearCodigoRecuperacion(ctx, nuevo)
						}
						resultados <- err
						return
					}
					codigo := "equivocado"
					if modo == "consumo" {
						codigo = "123456"
					}
					_, err := svc.RestablecerPasswordConCodigo(ctx, "ada@test.local", codigo, "password-nueva")
					resultados <- err
				}()
			}
			close(inicio)
			wg.Wait()
			close(resultados)
			exitos := 0
			for err := range resultados {
				if err == nil {
					exitos++
				} else if modo == "reemplazos" {
					t.Fatal(err)
				} else if modo == "consumo" && errors.Is(err, authapp.ErrUsuarioModificado) {
					// Otro consumo confirmó la contraseña después de la lectura inicial.
					// Se rechaza la copia obsoleta sin sobrescribir ese cambio.
				} else if !errors.Is(err, authapp.ErrCodigoRecuperacionInvalido) && !errors.Is(err, authapp.ErrCodigoRecuperacionSinIntentos) {
					t.Fatal(err)
				}
			}
			switch modo {
			case "reemplazos":
				var activos int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM codigo_recuperacion WHERE usuario_id=$1 AND usado_en IS NULL`, e.usuario).Scan(&activos); err != nil || activos != 1 {
					t.Fatalf("códigos simultáneos: %d %v", activos, err)
				}
			case "intentos":
				var intentos int
				var usado bool
				if err := pool.QueryRow(ctx, `SELECT intentos,usado_en IS NOT NULL FROM codigo_recuperacion WHERE id=$1`, c.ID).Scan(&intentos, &usado); err != nil || intentos != 5 || !usado {
					t.Fatalf("se perdieron intentos: %d %t %v", intentos, usado, err)
				}
			case "consumo":
				if exitos != 1 {
					t.Fatalf("consumos exitosos=%d", exitos)
				}
				u, err := repo.BuscarPorID(ctx, e.usuario)
				if err != nil || u.PasswordHash != "hash:password-nueva" || u.VersionSesion != 1 {
					t.Fatalf("contraseña/sesión: %+v %v", u, err)
				}
			}
		})
	}
}
func TestAprobacionAtribuida_RecargaFechaYActor(t *testing.T) {
	pool := baseVacia(t)
	ctx := context.Background()
	prepararBajaReal(t, pool, false)
	var actor string
	if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN'`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	svc := servicioRecuperacionReal(pool)
	repo := authinfra.NewPostgresRepo(pool)
	nuevo, err := svc.CrearAdmin(ctx, actor, "Nueva", "Admin", "nueva@test.local", "password123")
	if err != nil {
		t.Fatal(err)
	}
	pendiente := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Nuevo','Docente','nuevo@test.local','hash','DOCENTE','PENDIENTE')`, pendiente)
	if err := svc.Aprobar(ctx, pendiente, actor); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{nuevo.ID, pendiente} {
		u, err := repo.BuscarPorID(ctx, id)
		if err != nil || u.AprobadoPor == nil || *u.AprobadoPor != actor || u.FechaAprobacion == nil {
			t.Fatalf("aprobación sin atribución: %+v %v", u, err)
		}
	}
}
