package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/academic/application"
	"github.com/agustin-y2k/sgrc/internal/academic/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/prioridadcurso"
	"github.com/gofiber/fiber/v2"
)

type marcasCursoDePrueba struct {
	fakeMarcas
	conservadas bool
}

func (m *marcasCursoDePrueba) PrioridadesDeCurso(ctx context.Context, id string, destino prioridadcurso.Alcance) ([]prioridadcurso.Marca, error) {
	return []prioridadcurso.Marca{{ID: "marca", EquipoID: "pc", MateriaNombre: "Dibujo", Antes: true, Exacta: true, Prioridad: 1}}, nil
}
func (m *marcasCursoDePrueba) ConservarPrioridadesDeCurso(ctx context.Context, id string, destino prioridadcurso.Alcance) error {
	m.conservadas = true
	return nil
}

func TestHTTP_CursoPrioridades_ExigeRevisionYPermiso(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.ciclos["ciclo"] = &domain.CicloLectivo{ID: "ciclo", Activo: true}
	repo.cursos["curso"] = &domain.Curso{ID: "curso", CicloLectivoID: "ciclo", Anio: 3, Division: "2"}
	marcas := &marcasCursoDePrueba{}
	svc := application.NewService(repo, nil, nil, nil, nil, nil, marcas, idSecuencial, relojDeTest, nil)
	app := fiber.New()
	RegisterRoutes(app, NewHandler(svc, fakeAuditor{}), registroDePrueba.Autenticacion(testSecret))
	pedir := func(metodo, ruta, rol string, cuerpo any, estado int) *application.ImpactoCurso {
		t.Helper()
		req := httptest.NewRequest(metodo, ruta, jsonBody(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		if rol != "" {
			req.Header.Set("Authorization", "Bearer "+tokenPara("prueba-prioridades", rol))
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != estado {
			t.Fatalf("%s %s: %d, esperado %d", metodo, ruta, resp.StatusCode, estado)
		}
		if metodo == "POST" && estado == 200 {
			var i application.ImpactoCurso
			if err := json.NewDecoder(resp.Body).Decode(&i); err != nil {
				t.Fatal(err)
			}
			return &i
		}
		return nil
	}
	datos := map[string]any{"anio": 4, "division": "2"}
	for _, tc := range []struct {
		rol    string
		estado int
	}{{"", 401}, {"DOCENTE", 403}} {
		pedir("POST", "/api/cursos/curso/impacto-prioridades", tc.rol, datos, tc.estado)
	}
	pedir("POST", "/api/cursos/curso/impacto-prioridades", "ADMIN", map[string]any{"anio": 0}, 400)
	impacto := pedir("POST", "/api/cursos/curso/impacto-prioridades", "ADMIN", datos, 200)
	if repo.cursos["curso"].Anio != 3 || marcas.conservadas {
		t.Fatal("la previsualización escribió datos")
	}
	pedir("PATCH", "/api/cursos/curso", "ADMIN", datos, 409)
	datos["versionImpacto"] = impacto.Version
	datos["decisionPrioridades"] = "desconocida"
	pedir("PATCH", "/api/cursos/curso", "ADMIN", datos, 400)
	datos["decisionPrioridades"] = application.ConservarExactas
	pedir("PATCH", "/api/cursos/curso", "ADMIN", datos, 200)
	if !marcas.conservadas || repo.cursos["curso"].Anio != 4 {
		t.Fatal("no aplicó juntos el curso y la conservación")
	}
	pedir("PATCH", "/api/cursos/curso", "ADMIN", datos, 409)
}
