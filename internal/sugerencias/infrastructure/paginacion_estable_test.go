//go:build integration

package infrastructure

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/agustin-y2k/sgrc/internal/sugerencias/domain"
)

func TestBuzon_PaginacionConActividadEmpatada(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	usuario := crearUsuarioDeTest(t, pool)
	ahora := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	// Mismos timestamps e inserción opuesta al desempate: no se puede depender
	// del orden físico de las filas al cambiar LIMIT u OFFSET.
	var esperados []string
	for i := 15; i >= 1; i-- {
		hilo := hiloDePrueba(t, usuario, ahora)
		hilo.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		if err := repo.Crear(ctx, hilo); err != nil {
			t.Fatal(err)
		}
		esperados = append(esperados, hilo.ID)
	}
	slices.Sort(esperados)
	for nombre, listar := range map[string]func(paginacion.Pagina) ([]*domain.Sugerencia, int, error){
		"administracion": func(p paginacion.Pagina) ([]*domain.Sugerencia, int, error) { return repo.ListarTodas(ctx, false, p) },
		"propios": func(p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
			return repo.ListarDeUsuario(ctx, usuario, p)
		},
	} {
		t.Run(nombre, func(t *testing.T) {
			for _, tamanio := range []int{3, 5} {
				var obtenidos []string
				for numero := 1; numero <= len(esperados)/tamanio; numero++ {
					hilos, total, err := listar(paginacion.Pagina{Numero: numero, Tamanio: tamanio})
					if err != nil || total != len(esperados) {
						t.Fatalf("listado: total %d, err %v", total, err)
					}
					for _, h := range hilos {
						obtenidos = append(obtenidos, h.ID)
					}
				}
				if !slices.Equal(obtenidos, esperados) {
					t.Errorf("tamaño %d: orden inestable o hilos repetidos/omitidos: %v; se esperaba %v", tamanio, obtenidos, esperados)
				}
			}
		})
	}
}
