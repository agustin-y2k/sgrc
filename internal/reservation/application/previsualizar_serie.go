package application

import (
	"context"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
)

type EquipoEnSerie struct {
	EquipoID string `json:"equipoId"`
	Etiqueta string `json:"etiqueta"`
	Estado   string `json:"estado"`
	Motivo   string `json:"motivo,omitempty"`
}

type FechaEnSerie struct {
	Fecha   string          `json:"fecha"`
	Equipos []EquipoEnSerie `json:"equipos"`
}

type PrevisualizacionSerie struct {
	CantidadClases int            `json:"cantidadClases"`
	PuedeCrear     bool           `json:"puedeCrear"`
	EquiposATomar  []string       `json:"equiposATomar"`
	Fechas         []FechaEnSerie `json:"fechas"`
}

// PrevisualizarReservaRecurrente consulta todas las ocurrencias sin guardar
// reservas ni emitir eventos. La creación vuelve a validar dentro de su
// transacción: esta consulta no garantiza ni retiene disponibilidad.
func (s *Service) PrevisualizarReservaRecurrente(ctx context.Context, materiaID, usuarioID string, esAdmin bool, dia domain.DiaSemana, inicio, fin time.Duration, desde, hasta time.Time, equipos []string) (*PrevisualizacionSerie, error) {
	_, fechas, err := s.validarSerie(ctx, materiaID, usuarioID, esAdmin, dia, inicio, fin, desde, hasta, equipos)
	if err != nil {
		return nil, err
	}
	res := &PrevisualizacionSerie{CantidadClases: len(fechas), PuedeCrear: true, EquiposATomar: []string{}, Fechas: make([]FechaEnSerie, 0, len(fechas))}
	tomar := map[string]bool{}
	for _, fecha := range fechas {
		libres, err := s.ListarEquiposDisponiblesEn(ctx, fecha, inicio, fin, materiaID)
		if err != nil {
			return nil, err
		}
		ocupados, err := s.ListarEquiposOcupadosEn(ctx, fecha, inicio, fin, usuarioID, materiaID)
		if err != nil {
			return nil, err
		}
		porEquipo := map[string]EquipoEnSerie{}
		for _, e := range libres {
			porEquipo[e.EquipoID] = EquipoEnSerie{EquipoID: e.EquipoID, Etiqueta: e.Etiqueta, Estado: "LIBRE"}
		}
		for _, e := range ocupados {
			estado, motivo := "NO_DISPONIBLE", "Ocupado: la prioridad o el horario no permiten tomarlo."
			if e.EsBloqueo {
				motivo = "Bloqueado por administración."
			}
			if e.PuedeTomarse {
				estado, motivo = "POR_PRIORIDAD", "Se desplazarán reservas de este equipo en esta fecha."
			}
			porEquipo[e.EquipoID] = EquipoEnSerie{EquipoID: e.EquipoID, Etiqueta: e.Etiqueta, Estado: estado, Motivo: motivo}
		}
		fila := FechaEnSerie{Fecha: fecha.Format("2006-01-02"), Equipos: make([]EquipoEnSerie, 0, len(equipos))}
		for _, id := range equipos {
			e, ok := porEquipo[id]
			if !ok {
				e = EquipoEnSerie{EquipoID: id, Etiqueta: "Equipo seleccionado", Estado: "NO_DISPONIBLE", Motivo: "No está disponible en el inventario para esta franja."}
			}
			if e.Estado == "NO_DISPONIBLE" {
				res.PuedeCrear = false
			}
			if e.Estado == "POR_PRIORIDAD" {
				tomar[id] = true
			}
			fila.Equipos = append(fila.Equipos, e)
		}
		res.Fechas = append(res.Fechas, fila)
	}
	for _, id := range equipos {
		if tomar[id] {
			res.EquiposATomar = append(res.EquiposATomar, id)
		}
	}
	return res, nil
}
