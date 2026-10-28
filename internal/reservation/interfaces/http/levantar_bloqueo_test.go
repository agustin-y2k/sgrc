package http

import (
	"context"
	"encoding/json"
	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func (r *fakeRepo) BuscarBloqueoParaActualizar(_ context.Context, operacionID string) ([]*domain.Reserva, error) {
	var filas []*domain.Reserva
	for _, reserva := range r.reservas {
		if reserva.Tipo == domain.TipoBloqueo && reserva.BloqueoOperacionID != nil && *reserva.BloqueoOperacionID == operacionID {
			filas = append(filas, reserva)
		}
	}
	sort.Slice(filas, func(i, j int) bool { return filas[i].ID < filas[j].ID })
	return filas, nil
}
func (r *fakeRepo) RegistrarLevantamientoDeBloqueo(context.Context, application.LevantarBloqueoParams, int) error {
	return nil
}

func TestLevantarBloqueo_HTTPPermisosYRepeticion(t *testing.T) {
	for _, tc := range []struct {
		nombre, rol, actor, motivo string
		estado                     int
	}{
		{"sin-sesion", "", "", "", 401}, {"docente", "DOCENTE", "docente", "motivo", 403},
		{"admin-propio", "ADMIN", "admin", "", 200}, {"admin-ajeno-sin-motivo", "ADMIN", "otro", "", 400},
		{"admin-ajeno-con-motivo", "ADMIN", "otro", "Terminó el uso", 200},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			repo := nuevoFakeRepo()
			actor, op := "admin", "operacion"
			for _, id := range []string{"b1", "b2"} {
				repo.reservas[id] = &domain.Reserva{ID: id, EquipoID: "pc-" + id, Tipo: domain.TipoBloqueo, Estado: domain.ReservaConfirmada, CreadoPor: &actor, BloqueoOperacionID: &op}
			}
			app := nuevaAppDeTest(repo)
			pedir := func() int {
				cuerpo, _ := json.Marshal(map[string]string{"motivo": tc.motivo})
				req := httptest.NewRequest("POST", "/api/bloqueos/operacion/levantar", strings.NewReader(string(cuerpo)))
				req.Header.Set("Content-Type", "application/json")
				if tc.rol != "" {
					req.Header.Set("Authorization", "Bearer "+tokenPara(tc.actor, tc.rol))
				}
				res, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				return res.StatusCode
			}
			if estado := pedir(); estado != tc.estado {
				t.Fatalf("estado=%d esperado=%d", estado, tc.estado)
			}
			if tc.estado == 200 {
				if estado := pedir(); estado != 200 {
					t.Fatalf("repetir devolvió %d", estado)
				}
				for _, r := range repo.reservas {
					if r.Estado != domain.ReservaCancelada {
						t.Fatal("quedó una PC bloqueada")
					}
				}
			} else {
				for _, r := range repo.reservas {
					if r.Estado != domain.ReservaConfirmada {
						t.Fatal("el rechazo canceló una PC")
					}
				}
			}
		})
	}
}

func TestLevantarBloqueo_LaRutaIndividualTambienExigeAdmin(t *testing.T) {
	repo := nuevoFakeRepo()
	actor, op := "docente", "operacion"
	repo.reservas["b1"] = &domain.Reserva{ID: "b1", EquipoID: "pc", Tipo: domain.TipoBloqueo, Estado: domain.ReservaConfirmada, CreadoPor: &actor, BloqueoOperacionID: &op}
	app := nuevaAppDeTest(repo)
	req := httptest.NewRequest("POST", "/api/reservas/b1/cancelar", strings.NewReader(`{"motivo":"Intento desde una cuenta degradada"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPara(actor, "DOCENTE"))
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 403 || repo.reservas["b1"].Estado != domain.ReservaConfirmada {
		t.Fatal("la ruta individual permitió eludir el permiso del bloqueo")
	}
}
