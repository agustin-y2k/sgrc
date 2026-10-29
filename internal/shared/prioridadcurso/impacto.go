// Package prioridadcurso define el contrato entre estructura académica e inventario.
package prioridadcurso

type Alcance struct {
	Anio      int    `json:"anio"`
	Division  string `json:"division"`
	Modalidad string `json:"modalidad"`
}

type Marca struct {
	ID               string `json:"id"`
	EquipoID         string `json:"equipoId"`
	EquipoEtiqueta   string `json:"equipoEtiqueta"`
	CarroNombre      string `json:"carroNombre"`
	MateriaNombre    string `json:"materiaNombre"`
	Prioridad        int    `json:"prioridad"`
	Exacta           bool   `json:"exacta"`
	Antes            bool   `json:"antes"`
	Despues          bool   `json:"despues"`
	Compartida       bool   `json:"compartida"`
	DestinoID        string `json:"destinoId"`
	DestinoPrioridad int    `json:"destinoPrioridad"`
}
