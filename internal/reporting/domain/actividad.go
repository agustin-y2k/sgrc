package domain

// Actividad separa planificación de movimientos reales. Las cantidades de
// préstamos son movimientos por equipo; no se suman a las reservas como usos adicionales.
type MetricasActividad struct {
	ReservasSolicitadas          int `json:"reservasSolicitadas"`
	ReservasAtendidas            int `json:"reservasAtendidas"`
	ReservasNoRetiradas          int `json:"reservasNoRetiradas"`
	ReservasCanceladas           int `json:"reservasCanceladas"`
	PrestamosConReserva          int `json:"prestamosConReserva"`
	PrestamosSinReserva          int `json:"prestamosSinReserva"`
	PrestamosOrigenNoDeterminado int `json:"prestamosOrigenNoDeterminado"`
	PrestamosAbiertos            int `json:"prestamosAbiertos"`
	PrestamosDevueltos           int `json:"prestamosDevueltos"`
}
type ActividadEquipo struct {
	EquipoID string `json:"equipoId"`
	Etiqueta string `json:"etiqueta"`
	MetricasActividad
}
type ActividadResponsable struct {
	Identidad string  `json:"identidad"`
	Nombre    string  `json:"nombre"`
	UsuarioID *string `json:"usuarioId,omitempty"`
	MetricasActividad
}
type Actividad struct {
	Version      int                    `json:"version"`
	Anio         int                    `json:"anio"`
	Disponible   bool                   `json:"disponible"`
	Historico    bool                   `json:"historico"`
	Totales      MetricasActividad      `json:"totales"`
	Equipos      []ActividadEquipo      `json:"equipos"`
	Responsables []ActividadResponsable `json:"responsables"`
}
