// Package domain contiene los tipos de RF-06 — el snapshot histórico
// permanente que se crea una sola vez al archivar un ciclo lectivo
// (HistoricoUsoEquipo/HistoricoUsoDocente, uno por año — ver
// migrations/001_esquema_inicial.sql, están indexados por `anio`, no por el UUID del
// ciclo), y los resúmenes "en vivo" que se calculan al vuelo para un
// ciclo todavía activo (ResumenUsoEquipo/ResumenUsoDocente, sin persistencia
// propia — son resultados de consulta, no entidades).
package domain

import (
	"errors"
	"fmt"
)

var ErrValorNegativo = errors.New("la cantidad de reservas y los minutos no pueden ser negativos")

// HistoricoUsoEquipo es el snapshot permanente de cuánto se usó un equipo en un
// año lectivo ya archivado (RF-02.4/06.3) — se crea una única vez, en el
// momento de archivar, y nunca se vuelve a modificar. EtiquetaSnapshot,
// IdentificadorSnapshot y CarroNombreSnapshot quedan "congelados" tal como
// estaban en ese momento, porque la PC puede moverse de carro o renumerarse
// después, y el histórico no debe cambiar retroactivamente.
type HistoricoUsoEquipo struct {
	ID       string
	Anio     int
	EquipoID string
	// EtiquetaSnapshot es lo que se muestra: "PC 3" o "Proyector Epson". Los
	// dos de abajo van en 0 y "" si el equipo no estaba en ningún carro
	// — un proyector archivado se leía como "PC 0 ()".
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

// ResumenUsoEquipo/ResumenUsoDocente son resultados de consulta "en vivo"
// (RF-06.1/06.2, para un ciclo todavía activo) — no se persisten, así que
// no llevan snapshot: la PC/el docente todavía existen tal cual son, se
// resuelven en el momento sin necesidad de "congelar" nada.
// ResumenUsoEquipo lleva identificador y carro resueltos, no solo el UUID: un
// reporte que solo muestra IDs no se puede leer. El histórico ya guardaba
// esos datos como snapshot (ver HistoricoUsoEquipo) — acá se resuelven en vivo
// con un JOIN, para que ambos reportes se muestren igual.
type ResumenUsoEquipo struct {
	EquipoID string
	// Etiqueta es cómo se nombra al equipo en el reporte: "PC 3" o
	// "Proyector Epson". Identificador va en 0 y CarroNombre vacío en lo que
	// no está en ningún carro.
	Etiqueta          string
	Identificador     int
	CarroNombre       string
	CantidadReservas  int
	MinutosReservados int
}

type ResumenUsoDocente struct {
	// UsuarioID es nil cuando la cuenta se eliminó definitivamente
	// (RF-01.9): la FK quedó en SET NULL y lo único que sobrevive es el
	// nombre congelado en la reserva. Sus horas se cuentan igual — el
	// reporte del año tiene que cerrar aunque alguien se haya ido.
	UsuarioID         *string
	NombreDocente     string
	CantidadReservas  int
	MinutosReservados int
}

// ResumenIncidenciasEquipo / ResumenIncidenciasCarro implementan RF-06.3:
// incidencias por equipo y por carro. A diferencia del uso de equipos y
// docentes, este reporte NO necesita snapshot histórico — Incidencia nunca
// se elimina (sobrevive al archivado de cualquier ciclo, ver RF-02.4), así
// que siempre se resuelve con una query directa.
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
// carro. Es el número que se lleva a pedir presupuesto: "de sesenta y cuatro
// máquinas, doce están fuera de circulación".
//
// A diferencia del resto de este paquete, no mira un ciclo lectivo ni un
// rango de fechas: es una foto de AHORA. Un equipo roto lo está hoy,
// independientemente del año en que se reportó la falla.
type EstadoDelInventario struct {
	// CarroID y CarroNombre vacíos en la fila de los equipos sueltos, que no
	// cuelgan de ningún carro. Se los cuenta igual: un proyector roto
	// también sale del inventario disponible.
	CarroID     string
	CarroNombre string

	Disponibles     int
	EnMantenimiento int
	FueraDeServicio int
	// Total NO es la suma de los tres: excluye los dados de baja, que ya no
	// son parte del parque. Se expone porque el porcentaje que importa es
	// sobre lo que la escuela todavía tiene.
	Total int
}

// EquipoFueraDeCirculacion es una máquina que hoy no se puede reservar, con
// lo último que se sabe de por qué.
//
// La distinción entre los dos estados NO es qué tan rota está, sino QUIÉN
// puede arreglarla: EN_MANTENIMIENTO es lo que la institución resuelve por
// su cuenta, FUERA_DE_SERVICIO lo que no —sin repuestos, sin autorización,
// sin quien sepa—. La misma falla puede caer en cualquiera de los dos según
// la escuela y el momento, así que el reporte no infiere nada del tipo de
// falla: informa el estado que alguien decidió y la categoría por separado.
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
//
// Las dos cuentas dicen cosas distintas y por eso van las dos: veinte
// incidencias de batería sobre veinte máquinas es un problema de lote; veinte
// sobre la misma máquina es una máquina para dar de baja.
type ResumenPorCategoriaDeFalla struct {
	// Categoria vacía es la fila de "sin clasificar", que se cuenta aparte en
	// vez de esconderse: cuántas fallas nadie pudo diagnosticar es
	// justamente uno de los números que importan.
	Categoria         string
	Total             int
	Abiertas          int
	EquiposAlcanzados int
}
