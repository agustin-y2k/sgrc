package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PedidoDeMateria es un docente pidiendo poder dictar —y por lo tanto
// reservar computadoras para— una materia más.
//
// Al registrarse ya se podía decir qué materia se dicta (usuario.
// materia_solicitada, texto libre para el caso de que todavía no exista).
// Esto es lo mismo, pero repetible: a un docente le dan una materia nueva en
// mayo, y hasta ahora la única salida era encontrar a un Admin y pedirle que
// lo cargara a mano.
//
// **Aprobarlo es una decisión de una persona, y el sistema no la toma.** Que
// alguien tenga una materia habilita a reservar equipos para ella, y quien
// ya la da puede quedarse sin máquinas porque el otro llegó antes (tocarle
// las reservas no puede: eso ya está prohibido en reservation). Si el pedido
// es legítimo se sabe hablando con la persona o con los directivos, no
// mirando una pantalla. Lo que hace el sistema es dejarlo escrito con su
// motivo, avisarle a quien ya la dicta, y guardar quién resolvió qué.
type PedidoDeMateria struct {
	ID        string
	UsuarioID string

	// Las dos formas de pedir, excluyentes:
	//
	//   MateriaID          → existe y se eligió de la lista.
	//   CursoSolicitado /
	//   MateriaSolicitada  → no existe todavía y va escrita a mano.
	MateriaID         *string
	CursoSolicitado   string
	MateriaSolicitada string

	Motivo string

	Estado      EstadoPedido
	Respuesta   string
	ResueltoPor *string
	ResueltoEn  *time.Time
	CreadoEn    time.Time
}

type EstadoPedido string

const (
	PedidoPendiente EstadoPedido = "PENDIENTE"
	PedidoAprobado  EstadoPedido = "APROBADO"
	PedidoRechazado EstadoPedido = "RECHAZADO"
)

const (
	MaxMotivo    = 1000
	MaxRespuesta = 1000
)

var (
	ErrPedidoSinMateria = errors.New("hay que elegir una materia o escribir cuál es")
	ErrPedidoDobleForma = errors.New("hay que elegir una materia de la lista o escribir una, no las dos")
	ErrMotivoVacio      = errors.New("hay que explicar por qué la pedís")
	ErrMotivoLargo      = fmt.Errorf("el motivo no puede pasar de %d caracteres", MaxMotivo)
	ErrRespuestaLarga   = fmt.Errorf("la respuesta no puede pasar de %d caracteres", MaxRespuesta)
	ErrPedidoResuelto   = errors.New("ese pedido ya fue resuelto")
	ErrPedidoNoExiste   = errors.New("no se encontró ese pedido")
	ErrRechazoSinMotivo = errors.New("al rechazar hay que explicar por qué")
)

// NuevoPedidoDeMateria arma el pedido. `materiaID` en nil significa que la
// materia todavía no existe y va escrita a mano.
func NuevoPedidoDeMateria(id, usuarioID string, materiaID *string, cursoSolicitado, materiaSolicitada, motivo string, ahora time.Time) (*PedidoDeMateria, error) {
	materiaSolicitada = strings.TrimSpace(materiaSolicitada)
	cursoSolicitado = strings.TrimSpace(cursoSolicitado)
	motivo = strings.TrimSpace(motivo)

	tieneID := materiaID != nil && strings.TrimSpace(*materiaID) != ""
	tieneTexto := materiaSolicitada != ""

	switch {
	case tieneID && tieneTexto:
		return nil, ErrPedidoDobleForma
	case !tieneID && !tieneTexto:
		return nil, ErrPedidoSinMateria
	}

	// El motivo es obligatorio y no un campo más: es lo único con lo que
	// cuenta quien decide antes de ir a preguntar, y tener que escribirlo
	// hace pensar dos veces a quien pide de más.
	if motivo == "" {
		return nil, ErrMotivoVacio
	}
	if len([]rune(motivo)) > MaxMotivo {
		return nil, ErrMotivoLargo
	}

	p := &PedidoDeMateria{
		ID: id, UsuarioID: usuarioID, Motivo: motivo,
		Estado: PedidoPendiente, CreadoEn: ahora,
	}
	if tieneID {
		limpio := strings.TrimSpace(*materiaID)
		p.MateriaID = &limpio
	} else {
		p.MateriaSolicitada = materiaSolicitada
		p.CursoSolicitado = cursoSolicitado
	}
	return p, nil
}

// EsMateriaNueva: la materia no existe y hay que crearla al aprobar.
func (p *PedidoDeMateria) EsMateriaNueva() bool { return p.MateriaID == nil }

// Aprobar deja el pedido resuelto. La respuesta es opcional: en un "sí" el
// resultado ya se ve solo —la materia aparece en la lista al reservar—.
func (p *PedidoDeMateria) Aprobar(adminID, respuesta string, ahora time.Time) error {
	return p.resolver(PedidoAprobado, adminID, respuesta, ahora)
}

// Rechazar exige explicación. Un "no" sin motivo manda a la persona a
// preguntar por qué, y esa conversación empieza mal: quien pidió ya se
// expuso escribiendo para qué la quería.
func (p *PedidoDeMateria) Rechazar(adminID, respuesta string, ahora time.Time) error {
	if strings.TrimSpace(respuesta) == "" {
		return ErrRechazoSinMotivo
	}
	return p.resolver(PedidoRechazado, adminID, respuesta, ahora)
}

func (p *PedidoDeMateria) resolver(estado EstadoPedido, adminID, respuesta string, ahora time.Time) error {
	if p.Estado != PedidoPendiente {
		return ErrPedidoResuelto
	}
	respuesta = strings.TrimSpace(respuesta)
	if len([]rune(respuesta)) > MaxRespuesta {
		return ErrRespuestaLarga
	}

	p.Estado = estado
	p.Respuesta = respuesta
	p.ResueltoPor = &adminID
	p.ResueltoEn = &ahora
	return nil
}
