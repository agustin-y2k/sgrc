package http

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/application"
	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

func TestContextoPrestamos_RespuestaConservaProcedenciaSinReserva(t *testing.T) {
	c := &domain.ContextoAcademicoPrestamo{ReservaIDOriginal: "reserva-original", MateriaID: "materia", MateriaNombre: "Dibujo", ContenedorID: "curso", ContenedorTipo: "CURSO", ContenedorNombre: "3°2", CursoAnio: 3, CursoDivision: "2", Modalidad: "Construcción", CicloID: "ciclo", CicloAnio: 2026}
	p := &domain.Prestamo{ID: "prestamo", EquipoID: "pc", EntregadoANombre: "Docente", ContextoAcademico: c}
	nombreActual := "Nombre corregido"
	for _, respuesta := range []prestamoResponse{toPrestamoResponse(p, time.Now()), toPrestamoDetalladoResponse(&application.PrestamoDetallado{Prestamo: p, MateriaNombre: &nombreActual}, time.Now())} {
		if respuesta.ContextoAcademico != c || respuesta.MateriaNombre == nil || *respuesta.MateriaNombre != "Dibujo" {
			t.Fatalf("se sustituyó el contexto histórico: %+v", respuesta)
		}
		contenido, err := json.Marshal(respuesta)
		if err != nil {
			t.Fatal(err)
		}
		var cuerpo map[string]any
		if err := json.Unmarshal(contenido, &cuerpo); err != nil {
			t.Fatal(err)
		}
		if _, existe := cuerpo["reservaId"]; existe {
			t.Fatal("se inventó un vínculo activo después del cierre")
		}
		contexto := cuerpo["contextoAcademico"].(map[string]any)
		if contexto["reservaIdOriginal"] != "reserva-original" || contexto["cicloAnio"] != float64(2026) || contexto["reconstruido"] != false {
			t.Fatalf("respuesta incompleta: %s", contenido)
		}
	}
}
