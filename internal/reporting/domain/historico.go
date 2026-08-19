// Package domain contiene los tipos de RF-06 — el snapshot histórico
// permanente que se crea una sola vez al archivar un ciclo lectivo
// (HistoricoUsoEquipo/HistoricoUsoDocente, uno por año — ver
// migrations/001_esquema_inicial.sql, están indexados por `anio`, no por el
// UUID del ciclo), y los resúmenes "en vivo" que se calculan al vuelo para un
// ciclo todavía activo (ResumenUsoEquipo/ResumenUsoDocente, sin persistencia
// propia — son resultados de consulta, no entidades).
package domain

import (
	"errors"
	"fmt"
)

var ErrValorNegativo = errors.New("la cantidad de reservas y los minutos no pueden ser negativos")

// HistoricoUsoEquipo es el snapshot permanente de cuánto se usó un equipo en
// un año lectivo ya archivado (RF-02.4/06.3) — se crea una única vez, en el
// momento de archivar, y nunca se vuelve a modificar.
type HistoricoUsoEquipo struct {
	ID       string
	Anio     int
	EquipoID string
	// EtiquetaSnapshot es lo que se muestra: "PC 3" o "Proyector Epson".
	EtiquetaSnapshot      string
	IdentificadorSnapshot int
	CarroNombreSnapshot   string
	MinutosReservados     int
	CantidadReservas      int
}

func NuevoHistoricoUsoEquipo(id string, anio int, equipoID, etiquetaSnapshot string, identificadorSnapshot int, carroNombreSnapshot string, minutosReservados, cantidadReservas int) (*HistoricoUsoEquipo, error) {
	if minutosReservados < 0 || cantidadReservas < 0 {
		return nil, fmt.Errorf("%w: cantidad=%d minutos=%d", ErrValorNegativo, cantidadReservas, minutosReservados)
	}
	return &HistoricoUsoEquipo{
		ID: id, Anio: anio, EquipoID: equipoID, EtiquetaSnapshot: etiquetaSnapshot,
		IdentificadorSnapshot: identificadorSnapshot,
		CarroNombreSnapshot:   carroNombreSnapshot, MinutosReservados: minutosReservados, CantidadReservas: cantidadReservas,
	}, nil
}

// HistoricoUsoDocente es el equivalente de HistoricoUsoEquipo agregado por
// docente.
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

// ResumenUsoEquipo/ResumenUsoDocente son resultados de consulta "en vivo"
// (RF-06.1/06.2, para un ciclo todavía activo) — no se persisten, así que no
// llevan snapshot: la PC/el docente todavía existen tal cual son, se
// resuelven en el momento sin necesidad de "congelar" nada.
type ResumenUsoEquipo struct {
	EquipoID string
	// Etiqueta es cómo se nombra al equipo en el reporte: "PC 3" o "Proyector
	// Epson".
	Etiqueta          string
	Identificador     int
	CarroNombre       string
	CantidadReservas  int
	MinutosReservados int
}

type ResumenUsoDocente struct {
	// UsuarioID es nil cuando la cuenta se eliminó definitivamente (RF-01.9): la
	// FK quedó en SET NULL y lo único que sobrevive es el nombre congelado en la
	// reserva.
	UsuarioID         *string
	NombreDocente     string
	CantidadReservas  int
	MinutosReservados int
}

// ResumenIncidenciasEquipo / ResumenIncidenciasCarro implementan RF-06.3:
// incidencias por equipo y por carro.
type ResumenIncidenciasEquipo struct {
	EquipoID string
	// Ver ResumenUsoEquipo.Etiqueta.
	Etiqueta         string
	Identificador    int
	CarroNombre      string
	Total            int
	Abiertas         int
	EnReparacion     int
	EnviadasASoporte int
	Resueltas        int
	Graves           int
}

type ResumenIncidenciasCarro struct {
	CarroID     string
	CarroNombre string
	Total       int
	Abiertas    int
	Graves      int
}

// ── Estado del parque de equipos (RF-06.5) ──────────────────────────────

// EstadoDelInventario es cuántos equipos hay en cada estado, en total y por
// carro.
type EstadoDelInventario struct {
	// CarroID y CarroNombre vacíos en la fila de los equipos sueltos, que no
	// cuelgan de ningún carro.
	CarroID     string
	CarroNombre string

	Disponibles     int
	EnMantenimiento int
	FueraDeServicio int
	// Total NO es la suma de los tres: excluye los dados de baja, que ya no son
	// parte del parque.
	Total int
}

// EquipoFueraDeCirculacion es una máquina que hoy no se puede reservar, con
// lo último que se sabe de por qué.
type EquipoFueraDeCirculacion struct {
	EquipoID    string
	Etiqueta    string
	CarroNombre string
	Estado      string

	// Lo que sigue sale de la ÚLTIMA incidencia cargada, y puede estar vacío:
	// una máquina se puede pasar a mantenimiento sin haber reportado ninguna
	// falla, y ese hueco es un dato en sí mismo — nadie escribió qué tiene.
	Categoria        string
	UltimaFalla      string
	EstadoIncidencia string
}

// ResumenPorCategoriaDeFalla responde "qué se rompe acá": cuántas incidencias
// de cada tipo, y cuántos equipos distintos alcanzó.
type ResumenPorCategoriaDeFalla struct {
	// Categoria vacía es la fila de "sin clasificar", que se cuenta aparte en
	// vez de esconderse: cuántas fallas nadie pudo diagnosticar es justamente
	// uno de los números que importan.
	Categoria         string
	Total             int
	Abiertas          int
	EquiposAlcanzados int
}
