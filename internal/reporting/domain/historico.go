// Package domain contiene los tipos de RF-06 — el snapshot histórico
// permanente que se crea una sola vez al archivar un ciclo lectivo
// (HistoricoUsoPC/HistoricoUsoDocente, uno por año — ver
// migrations/001_init.sql, están indexados por `anio`, no por el UUID del
// ciclo), y los resúmenes "en vivo" que se calculan al vuelo para un
// ciclo todavía activo (ResumenUsoPC/ResumenUsoDocente, sin persistencia
// propia — son resultados de consulta, no entidades).
package domain

import (
	"errors"
	"fmt"
)

var ErrValorNegativo = errors.New("la cantidad de reservas y los minutos no pueden ser negativos")

// HistoricoUsoPC es el snapshot permanente de cuánto se usó una PC en un
// año lectivo ya archivado (RF-02.4/06.3) — se crea una única vez, en el
// momento de archivar, y nunca se vuelve a modificar. IdentificadorSnapshot
// y CarroNombreSnapshot quedan "congelados" tal como estaban en ese
// momento, porque la PC puede moverse de carro o renumerarse después, y
// el histórico no debe cambiar retroactivamente.
type HistoricoUsoPC struct {
	ID                    string
	Anio                  int
	PCID                  string
	IdentificadorSnapshot int
	CarroNombreSnapshot   string
	MinutosReservados     int
	CantidadReservas      int
}

func NuevoHistoricoUsoPC(id string, anio int, pcID string, identificadorSnapshot int, carroNombreSnapshot string, minutosReservados, cantidadReservas int) (*HistoricoUsoPC, error) {
	if minutosReservados < 0 || cantidadReservas < 0 {
		return nil, fmt.Errorf("%w: cantidad=%d minutos=%d", ErrValorNegativo, cantidadReservas, minutosReservados)
	}
	return &HistoricoUsoPC{
		ID: id, Anio: anio, PCID: pcID, IdentificadorSnapshot: identificadorSnapshot,
		CarroNombreSnapshot: carroNombreSnapshot, MinutosReservados: minutosReservados, CantidadReservas: cantidadReservas,
	}, nil
}

// HistoricoUsoDocente es el equivalente de HistoricoUsoPC agregado por
// docente. UsuarioID es nullable (la FK es SET NULL) — si el docente se
// elimina definitivamente más adelante, el histórico se conserva vía
// NombreDocenteSnapshot igual.
type HistoricoUsoDocente struct {
	ID                    string
	Anio                  int
	UsuarioID             *string
	NombreDocenteSnapshot string
	CantidadReservas      int
	MinutosTotales        int
}

func NuevoHistoricoUsoDocente(id string, anio int, usuarioID *string, nombreDocenteSnapshot string, cantidadReservas, minutosTotales int) (*HistoricoUsoDocente, error) {
	if cantidadReservas < 0 || minutosTotales < 0 {
		return nil, fmt.Errorf("%w: cantidad=%d minutos=%d", ErrValorNegativo, cantidadReservas, minutosTotales)
	}
	return &HistoricoUsoDocente{
		ID: id, Anio: anio, UsuarioID: usuarioID, NombreDocenteSnapshot: nombreDocenteSnapshot,
		CantidadReservas: cantidadReservas, MinutosTotales: minutosTotales,
	}, nil
}

// ResumenUsoPC/ResumenUsoDocente son resultados de consulta "en vivo"
// (RF-06.1/06.2, para un ciclo todavía activo) — no se persisten, así que
// no llevan snapshot: la PC/el docente todavía existen tal cual son, se
// resuelven en el momento sin necesidad de "congelar" nada.
// ResumenUsoPC lleva identificador y carro resueltos, no solo el UUID: un
// reporte que solo muestra IDs no se puede leer. El histórico ya guardaba
// esos datos como snapshot (ver HistoricoUsoPC) — acá se resuelven en vivo
// con un JOIN, para que ambos reportes se muestren igual.
type ResumenUsoPC struct {
	PCID              string
	Identificador     int
	CarroNombre       string
	CantidadReservas  int
	MinutosReservados int
}

type ResumenUsoDocente struct {
	UsuarioID         string
	NombreDocente     string
	CantidadReservas  int
	MinutosReservados int
}

// ResumenIncidenciasPC / ResumenIncidenciasCarro implementan RF-06.3:
// incidencias por equipo y por carro. A diferencia del uso de PCs y
// docentes, este reporte NO necesita snapshot histórico — Incidencia nunca
// se elimina (sobrevive al archivado de cualquier ciclo, ver RF-02.4), así
// que siempre se resuelve con una query directa.
type ResumenIncidenciasPC struct {
	PCID          string
	Identificador int
	CarroNombre   string
	Total         int
	Abiertas      int
	EnReparacion  int
	EnviadasDGE   int
	Resueltas     int
	Graves        int
}

type ResumenIncidenciasCarro struct {
	CarroID     string
	CarroNombre string
	Total       int
	Abiertas    int
	Graves      int
}
