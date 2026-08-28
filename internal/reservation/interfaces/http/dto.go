// Package http expone las rutas Fiber de reservation — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// ── Requests ────────────────────────────────────────────────────────────
// HoraInicio/HoraFin viajan como "HH:MM" en el JSON — se parsean a
// time.Duration en los handlers (ver parseHora en handlers.go).

type crearReservaRequest struct {
	MateriaID  string   `json:"materiaId"`
	Fecha      string   `json:"fecha"` // "2026-03-09"
	HoraInicio string   `json:"horaInicio"`
	HoraFin    string   `json:"horaFin"`
	EquipoIDs  []string `json:"equipoIds"`
}

type crearReservaRecurrenteRequest struct {
	MateriaID   string   `json:"materiaId"`
	DiaSemana   string   `json:"diaSemana"`
	HoraInicio  string   `json:"horaInicio"`
	HoraFin     string   `json:"horaFin"`
	FechaInicio string   `json:"fechaInicio"`
	FechaFin    string   `json:"fechaFin"`
	EquipoIDs   []string `json:"equipoIds"`
}

type cancelarReservaRequest struct {
	Motivo string `json:"motivo"`
}

type cancelarOcurrenciaRequest struct {
	Motivo   string `json:"motivo"`
	SoloEsta bool   `json:"soloEsta"`
}

type bloquearRequest struct {
	EquipoIDs  []string `json:"equipoIds"`
	Fecha      string   `json:"fecha"`
	HoraInicio string   `json:"horaInicio"`
	HoraFin    string   `json:"horaFin"`
	// Motivo es obligatorio: el bloqueo cancela las clases de otros, y desde
	// se guarda en cada bloqueo, no solo en el aviso de cancelación.
	Motivo string `json:"motivo"`
}

// ── Responses ───────────────────────────────────────────────────────────

type reservaGrupoResponse struct {
	ID                    string  `json:"id"`
	MateriaID             string  `json:"materiaId"`
	CreadoPor             *string `json:"creadoPor,omitempty"`
	NombreDocenteSnapshot string  `json:"nombreDocenteSnapshot"`
	Fecha                 string  `json:"fecha"`
	HoraInicio            string  `json:"horaInicio"`
	HoraFin               string  `json:"horaFin"`
	Estado                string  `json:"estado"`
	ReglaRecurrenciaID    *string `json:"reglaRecurrenciaId,omitempty"`
}

func toReservaGrupoResponse(g *domain.ReservaGrupo) reservaGrupoResponse {
	return reservaGrupoResponse{
		ID: g.ID, MateriaID: g.MateriaID, CreadoPor: g.CreadoPor,
		NombreDocenteSnapshot: g.NombreDocenteSnapshot,
		Fecha:                 g.Fecha.Format("2006-01-02"),
		HoraInicio:            formatHora(g.HoraInicio),
		HoraFin:               formatHora(g.HoraFin),
		Estado:                string(g.Estado),
		ReglaRecurrenciaID:    g.ReglaRecurrenciaID,
	}
}

type reservaResponse struct {
	ID                    string  `json:"id"`
	ReservaGrupoID        *string `json:"reservaGrupoId,omitempty"`
	EquipoID              string  `json:"equipoId"`
	MateriaID             *string `json:"materiaId,omitempty"`
	NombreDocenteSnapshot *string `json:"nombreDocenteSnapshot,omitempty"`
	Fecha                 string  `json:"fecha"`
	HoraInicio            string  `json:"horaInicio"`
	HoraFin               string  `json:"horaFin"`
	Estado                string  `json:"estado"`
	Tipo                  string  `json:"tipo"`
	// MotivoBloqueo solo viene en los BLOQUEO — una reserva normal ya dice
	// para qué es por su materia.
	MotivoBloqueo     string     `json:"motivoBloqueo,omitempty"`
	CreadoPor         *string    `json:"creadoPor,omitempty"`
	CanceladoPor      *string    `json:"canceladoPor,omitempty"`
	MotivoCancelacion *string    `json:"motivoCancelacion,omitempty"`
	CanceladaEn       *time.Time `json:"canceladaEn,omitempty"`
}

func toReservaResponse(r *domain.Reserva) reservaResponse {
	return reservaResponse{
		ID: r.ID, ReservaGrupoID: r.ReservaGrupoID, EquipoID: r.EquipoID, MateriaID: r.MateriaID,
		NombreDocenteSnapshot: r.NombreDocenteSnapshot,
		Fecha:                 r.Fecha.Format("2006-01-02"),
		HoraInicio:            formatHora(r.HoraInicio),
		HoraFin:               formatHora(r.HoraFin),
		Estado:                string(r.Estado),
		Tipo:                  string(r.Tipo),
		MotivoBloqueo:         r.MotivoBloqueo,
		CreadoPor:             r.CreadoPor,
		CanceladoPor:          r.CanceladoPor,
		MotivoCancelacion:     r.MotivoCancelacion,
		CanceladaEn:           r.CanceladaEn,
	}
}

// reservaDetalladaResponse agrega a reservaResponse los nombres que hacen
// falta para mostrar la reserva en pantalla.
type reservaDetalladaResponse struct {
	reservaResponse
	// Etiqueta es lo que se muestra: "PC 3" o "Proyector Epson".
	Etiqueta      string `json:"etiqueta"`
	Identificador int    `json:"identificador"`
	CarroNombre   string `json:"carroNombre"`
	MateriaNombre string `json:"materiaNombre,omitempty"`
	CursoNombre   string `json:"cursoNombre,omitempty"`
	// Presente solo si la reserva es parte de una serie recurrente.
	ReglaRecurrenciaID *string `json:"reglaRecurrenciaId,omitempty"`
}

func toReservaDetalladaResponse(d application.ReservaDetallada) reservaDetalladaResponse {
	return reservaDetalladaResponse{
		reservaResponse:    toReservaResponse(d.Reserva),
		Etiqueta:           d.Etiqueta,
		Identificador:      d.Identificador,
		CarroNombre:        d.CarroNombre,
		MateriaNombre:      d.MateriaNombre,
		CursoNombre:        d.CursoNombre,
		ReglaRecurrenciaID: d.ReglaRecurrenciaID,
	}
}

type crearReservaResponse struct {
	Grupo    reservaGrupoResponse `json:"grupo"`
	Reservas []reservaResponse    `json:"reservas"`
}

type crearReservaRecurrenteResponse struct {
	ReglaID string                 `json:"reglaId"`
	Grupos  []reservaGrupoResponse `json:"grupos"`
}

type cancelarOcurrenciaResponse struct {
	ReservasCanceladas int `json:"reservasCanceladas"`
}

type bloquearResponse struct {
	Bloqueos            []reservaResponse `json:"bloqueos"`
	ReservasCanceladas  int               `json:"reservasCanceladas"`
	DocentesNotificados int               `json:"docentesNotificados"`
}

func toBloquearResponse(res *application.ResultadoBloqueo) bloquearResponse {
	bloqueos := make([]reservaResponse, len(res.Bloqueos))
	for i, b := range res.Bloqueos {
		bloqueos[i] = toReservaResponse(b)
	}
	return bloquearResponse{
		Bloqueos: bloqueos, ReservasCanceladas: res.ReservasCanceladas, DocentesNotificados: res.DocentesNotificados,
	}
}

type listarReservasResponse struct {
	Data []reservaDetalladaResponse `json:"data"`
	Meta paginacion.Meta            `json:"meta"`
}

// bloqueCalendarioResponse es lo que RF-04.4 pide mostrar de cada bloque
// ocupado: horario, docente y materia.
type bloqueCalendarioResponse struct {
	ReservaID     string `json:"reservaId"`
	Fecha         string `json:"fecha"`
	HoraInicio    string `json:"horaInicio"`
	HoraFin       string `json:"horaFin"`
	Estado        string `json:"estado"`
	Tipo          string `json:"tipo"`
	Docente       string `json:"docente"`
	MateriaNombre string `json:"materiaNombre,omitempty"`
	CursoNombre   string `json:"cursoNombre,omitempty"`
	MotivoBloqueo string `json:"motivoBloqueo,omitempty"`
}

type calendarioEquipoResponse struct {
	EquipoID string `json:"equipoId"`
	// Etiqueta es cómo se llama el equipo, con su carro: "PC 7 del Carro 2".
	// Vacía si no se pudo resolver, y ahí la pantalla se queda con su título
	// genérico.
	Etiqueta string                     `json:"etiqueta,omitempty"`
	Desde    string                     `json:"desde"`
	Hasta    string                     `json:"hasta"`
	Bloques  []bloqueCalendarioResponse `json:"bloques"`
}

func toBloqueCalendarioResponse(b application.BloqueCalendario) bloqueCalendarioResponse {
	return bloqueCalendarioResponse{
		ReservaID:     b.Reserva.ID,
		Fecha:         b.Reserva.Fecha.Format("2006-01-02"),
		HoraInicio:    formatHora(b.Reserva.HoraInicio),
		HoraFin:       formatHora(b.Reserva.HoraFin),
		Estado:        string(b.Reserva.Estado),
		Tipo:          string(b.Reserva.Tipo),
		Docente:       deref(b.Reserva.NombreDocenteSnapshot),
		MateriaNombre: b.MateriaNombre,
		CursoNombre:   b.CursoNombre,
		MotivoBloqueo: b.Reserva.MotivoBloqueo,
	}
}

// deref: nombre_docente_snapshot es nullable en la base (los bloqueos
// administrativos no tienen docente).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// equipoDisponibleResponse: RF-04.2 + RF-03.7 — lo que el docente necesita
// para tildar qué equipos reservar (incluye software y freezado para poder
// decidir sin consultar inventory aparte).
type equipoDisponibleResponse struct {
	EquipoID string `json:"equipoId"`
	// Identificador va en 0 para un equipo suelto.
	Identificador int    `json:"identificador,omitempty"`
	Etiqueta      string `json:"etiqueta"`
	// Tipo distingue una PC de un proyector. Texto libre.
	Tipo string `json:"tipo,omitempty"`
	// CarroID y CarroNombre vacíos en un equipo suelto.
	CarroID           string `json:"carroId,omitempty"`
	CarroNombre       string `json:"carroNombre,omitempty"`
	Freezado          bool   `json:"freezado"`
	SoftwareInstalado string `json:"softwareInstalado,omitempty"`
	// Tramo parte la lista en tres bloques según qué materia prefiere este
	// equipo (RF-03.21): PREFERENTE, NEUTRAL o DE_OTRA_MATERIA. La lista ya
	// viene ordenada por tramo; el campo existe para poder titularlos.
	Tramo string `json:"tramo"`
	// Motivo es la frase que explica el tramo ("Preferente para Matemática
	// de 3°B"), armada en el servidor. Vacío en un equipo neutral.
	Motivo string `json:"motivo,omitempty"`
}

type equiposDisponiblesResponse struct {
	Data []equipoDisponibleResponse `json:"data"`
	// Ocupados: la otra mitad de la franja (RF-04.11).
	Ocupados []equipoOcupadoResponse `json:"ocupados"`
}

// equipoOcupadoResponse: un equipo que ya tiene dueño en esa franja.
type equipoOcupadoResponse struct {
	EquipoID    string `json:"equipoId"`
	Etiqueta    string `json:"etiqueta"`
	CarroNombre string `json:"carroNombre,omitempty"`
	// ReservaID es lo que después recibe el pedido de liberación.
	ReservaID string `json:"reservaId,omitempty"`
	// DocenteNombre vacío en un bloqueo administrativo, que no tiene docente
	// detrás; ahí lo que explica la franja es el motivo.
	DocenteNombre string `json:"docenteNombre,omitempty"`
	MateriaNombre string `json:"materiaNombre,omitempty"`
	Motivo        string `json:"motivo,omitempty"`
	// HoraInicio y HoraFin son las de la reserva que lo ocupa, que pueden no
	// coincidir con la franja consultada.
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
	// PuedePedirse lo decide el servidor: false en un bloqueo, en una reserva
	// propia y si esa franja ya empezó.
	PuedePedirse bool `json:"puedePedirse"`
}

func toEquipoOcupadoResponse(o application.EquipoOcupado) equipoOcupadoResponse {
	return equipoOcupadoResponse{
		EquipoID: o.EquipoID, Etiqueta: o.Etiqueta, CarroNombre: o.CarroNombre,
		ReservaID: o.ReservaID, DocenteNombre: o.DocenteNombre,
		MateriaNombre: o.MateriaNombre, Motivo: o.Motivo,
		HoraInicio: formatHora(o.HoraInicio), HoraFin: formatHora(o.HoraFin),
		PuedePedirse: o.PuedePedirse,
	}
}

func toEquipoDisponibleResponse(p application.EquipoDisponible) equipoDisponibleResponse {
	return equipoDisponibleResponse{
		EquipoID: p.EquipoID, Identificador: p.Identificador, Etiqueta: p.Etiqueta, Tipo: p.Tipo,
		CarroID: p.CarroID, CarroNombre: p.CarroNombre,
		Freezado: p.Freezado, SoftwareInstalado: p.SoftwareInstalado,
		Tramo: string(p.Tramo), Motivo: p.MotivoDePreferencia(),
	}
}

// cambiarEquipoRequest — la máquina nueva. Nada más: fecha y horario no se
// tocan, es la misma clase.
type cambiarEquipoRequest struct {
	EquipoID string `json:"equipoId"`
	// SoloEsta: mismo nombre y mismo significado que al cancelar una ocurrencia
	// (RF-04.6).
	SoloEsta *bool `json:"soloEsta"`
}

// pedirLiberacionRequest — RF-04.12. Solo el texto libre: qué reserva se pide
// va en la URL, y quién pide sale del token.
type pedirLiberacionRequest struct {
	Mensaje string `json:"mensaje"`
}
