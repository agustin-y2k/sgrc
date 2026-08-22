// Package http expone las rutas Fiber de availability — ver
// docs/08-api-spec.yaml para el contrato completo de cada endpoint.
package http

import (
	"time"

	"github.com/ramiro/sgrc/internal/availability/application"
	"github.com/ramiro/sgrc/internal/availability/domain"
)

// ── Requests ────────────────────────────────────────────────────────────
// HoraInicio/HoraFin viajan como "HH:MM" en el JSON — se parsean a
// time.Duration en los handlers (ver parseHora en parsing.go).

type bloqueRequest struct {
	DiaSemana  string `json:"diaSemana"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
}

// editarBloqueRequest: todos los campos opcionales — PATCH parcial (RF-07.3).
type editarBloqueRequest struct {
	DiaSemana  *string `json:"diaSemana"`
	HoraInicio *string `json:"horaInicio"`
	HoraFin    *string `json:"horaFin"`
}

// jornadaRequest es la jornada COMPLETA de la institución: lo que se manda es
// lo que queda, y lo que no se manda se borra. Reusa bloqueRequest porque un
// tramo de jornada tiene exactamente los mismos tres campos que uno del
// horario de un Admin.
//
// La lista vacía —o ausente— es un valor legítimo y no un cuerpo incompleto:
// es la institución declarando que no restringe ni días ni horarios.
type jornadaRequest struct {
	Tramos []bloqueRequest `json:"tramos"`
	// Confirmado en true es el Admin haciéndose cargo de que se cancelen las
	// reservas que quedan afuera. Sin él, un cambio con impacto se rechaza y
	// devuelve el detalle en vez de aplicarse.
	Confirmado bool `json:"confirmado"`
}

// ── Lo que un cambio de jornada deja afuera ──────────────────────────────

type reservaAfectadaResponse struct {
	ID         string `json:"id"`
	Fecha      string `json:"fecha"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
	Equipo     string `json:"equipo"`
	Materia    string `json:"materia"`
	Docente    string `json:"docente"`
}

type prestamoAfectadoResponse struct {
	ID     string `json:"id"`
	Equipo string `json:"equipo"`
	Quien  string `json:"quien"`
	// DevolucionEstimada siempre viene: un préstamo sin hora pactada no tiene
	// nada que comparar contra la jornada y no llega hasta acá.
	DevolucionEstimada string `json:"devolucionEstimada"`
}

// MaxAfectadasEnLaRespuesta acota la lista que viaja al cliente.
//
// El conteo es el dato que decide; la lista solo sirve para reconocer qué se
// está por perder, y para eso alcanza con las primeras. Sin tope, justamente
// el caso que esto vino a detectar —el tipeo que se lleva todo— devolvería
// miles de filas y las pintaría todas en pantalla.
const MaxAfectadasEnLaRespuesta = 50

type impactoResponse struct {
	// Reservas viene recortada a MaxAfectadasEnLaRespuesta. Las que se van a
	// cancelar son TotalAfectadas, no len(Reservas).
	Reservas       []reservaAfectadaResponse  `json:"reservas"`
	Prestamos      []prestamoAfectadoResponse `json:"prestamos"`
	TotalAfectadas int                        `json:"totalAfectadas"`
	// TotalDeReservas: cuántas hay en total, afectadas o no, para poder leer
	// el tamaño de lo que se cancela contra el tamaño de lo que hay.
	TotalDeReservas int `json:"totalDeReservas"`
}

func toImpactoResponse(i *application.ImpactoDeJornada) impactoResponse {
	afectadas := i.Reservas
	if len(afectadas) > MaxAfectadasEnLaRespuesta {
		afectadas = afectadas[:MaxAfectadasEnLaRespuesta]
	}

	r := impactoResponse{
		Reservas:        make([]reservaAfectadaResponse, len(afectadas)),
		Prestamos:       make([]prestamoAfectadoResponse, len(i.Prestamos)),
		TotalAfectadas:  len(i.Reservas),
		TotalDeReservas: i.TotalDeReservas,
	}
	for n, res := range afectadas {
		r.Reservas[n] = reservaAfectadaResponse{
			ID:         res.ID,
			Fecha:      res.Fecha.Format("2006-01-02"),
			HoraInicio: formatHora(res.HoraInicio),
			HoraFin:    formatHora(res.HoraFin),
			Equipo:     res.Equipo,
			Materia:    res.Materia,
			Docente:    res.Docente,
		}
	}
	for n, p := range i.Prestamos {
		r.Prestamos[n] = prestamoAfectadoResponse{
			ID:                 p.ID,
			Equipo:             p.Equipo,
			Quien:              p.Quien,
			DevolucionEstimada: p.DevolucionEstimada.Format(time.RFC3339),
		}
	}
	return r
}

type excepcionRequest struct {
	Fecha      string  `json:"fecha"`
	Tipo       string  `json:"tipo"`
	HoraInicio *string `json:"horaInicio"`
	HoraFin    *string `json:"horaFin"`
	Motivo     *string `json:"motivo"`
}

// ── Responses ───────────────────────────────────────────────────────────

type bloqueResponse struct {
	ID         string `json:"id"`
	DiaSemana  string `json:"diaSemana"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
}

func toBloqueResponse(b *domain.BloqueHorario) bloqueResponse {
	return bloqueResponse{
		ID:         b.ID,
		DiaSemana:  string(b.DiaSemana),
		HoraInicio: formatHora(b.HoraInicio),
		HoraFin:    formatHora(b.HoraFin),
	}
}

type excepcionResponse struct {
	ID         string  `json:"id"`
	Fecha      string  `json:"fecha"`
	Tipo       string  `json:"tipo"`
	HoraInicio *string `json:"horaInicio,omitempty"`
	HoraFin    *string `json:"horaFin,omitempty"`
	Motivo     *string `json:"motivo,omitempty"`
}

func toExcepcionResponse(e *domain.Excepcion) excepcionResponse {
	resp := excepcionResponse{
		ID: e.ID, Fecha: e.Fecha.Format("2006-01-02"), Tipo: string(e.Tipo), Motivo: e.Motivo,
	}
	if e.HoraInicio != nil {
		s := formatHora(*e.HoraInicio)
		resp.HoraInicio = &s
	}
	if e.HoraFin != nil {
		s := formatHora(*e.HoraFin)
		resp.HoraFin = &s
	}
	return resp
}

type adminDisponibilidadResponse struct {
	UsuarioID       string             `json:"usuarioId"`
	Nombre          string             `json:"nombre"`
	Apellido        string             `json:"apellido"`
	DisponibleAhora bool               `json:"disponibleAhora"`
	ExcepcionHoy    *excepcionResponse `json:"excepcionHoy,omitempty"`
	HorarioSemanal  []bloqueResponse   `json:"horarioSemanal"`
}

func toAdminDisponibilidadResponse(a application.AdminDisponibilidad) adminDisponibilidadResponse {
	horario := make([]bloqueResponse, len(a.HorarioSemanal))
	for i, b := range a.HorarioSemanal {
		horario[i] = toBloqueResponse(b)
	}
	var excepcion *excepcionResponse
	if a.ExcepcionHoy != nil {
		e := toExcepcionResponse(a.ExcepcionHoy)
		excepcion = &e
	}
	return adminDisponibilidadResponse{
		UsuarioID:       a.UsuarioID,
		Nombre:          a.Nombre,
		Apellido:        a.Apellido,
		DisponibleAhora: a.DisponibleAhora,
		ExcepcionHoy:    excepcion,
		HorarioSemanal:  horario,
	}
}

// toBloqueJornadaResponse reusa bloqueResponse: los dos tienen los mismos
// cuatro campos y el mismo formato de hora.
func toBloqueJornadaResponse(b *domain.BloqueJornada) bloqueResponse {
	return bloqueResponse{
		ID:         b.ID,
		DiaSemana:  string(b.DiaSemana),
		HoraInicio: formatHora(b.HoraInicio),
		HoraFin:    formatHora(b.HoraFin),
	}
}
