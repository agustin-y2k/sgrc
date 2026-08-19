// Package domain son las reglas del buzón por donde se cuenta que algo del
// sistema no anda, o que estaría bueno que hiciera otra cosa.
//
// No confundir con las incidencias de inventory: aquellas son sobre una
// computadora que no arranca —marcan el equipo y lo sacan de circulación—;
// esto es sobre el sistema. Son dos buzones distintos porque lo que se hace
// con cada uno también lo es: una incidencia la resuelve el técnico, una
// sugerencia la resuelve quien escribe el software.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTextoVacio        = errors.New("hay que escribir qué pasó o qué se te ocurre")
	ErrTextoLargo        = fmt.Errorf("el mensaje no puede pasar de %d caracteres", MaxTexto)
	ErrTipoInvalido      = errors.New("tipo de mensaje inválido")
	ErrYaResuelta        = errors.New("esa sugerencia ya está resuelta")
	ErrRespuestaVacia    = errors.New("hay que escribir la respuesta")
	ErrRespuestaLarga    = fmt.Errorf("la respuesta no puede pasar de %d caracteres", MaxTexto)
	ErrPantallaLarga     = errors.New("la pantalla no puede pasar de 200 caracteres")
	ErrSugerenciaNoExist = errors.New("no se encontró esa sugerencia")
)

// MaxTexto: cuatro mil caracteres son unas dos carillas. Alcanza de sobra
// para contar un problema con detalle y evita que un pegado accidental de
// media pantalla termine en la base.
const MaxTexto = 4000

// Tipo separa "no me deja hacer algo" de "estaría bueno que…", que es la
// primera pregunta que se hace quien lee: una cosa hay que arreglarla y la
// otra es para pensar.
type Tipo string

const (
	// Los nombres llevan el prefijo porque `Sugerencia` a secas es el tipo
	// de más abajo — la entidad, no la clase de mensaje.
	TipoSugerencia Tipo = "SUGERENCIA"
	TipoProblema   Tipo = "PROBLEMA"
)

func ParseTipo(s string) (Tipo, error) {
	switch Tipo(s) {
	case TipoSugerencia, TipoProblema:
		return Tipo(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrTipoInvalido, s)
	}
}

type Estado string

const (
	Abierta  Estado = "ABIERTA"
	Resuelta Estado = "RESUELTA"
)

// Sugerencia es un mensaje de alguien que usa el sistema.
//
// Pantalla y Version las completa la aplicación, no quien escribe: un "no
// anda" sin saber desde dónde se escribió obliga a ir a buscar a la persona
// para preguntarle, y con alguien que ya se sintió torpe usando el sistema,
// esa conversación no vuelve a pasar. Con la pantalla anotada, quien lee
// puede ir a mirar sin molestar a nadie.
type Sugerencia struct {
	ID        string
	UsuarioID string
	Tipo      Tipo
	Texto     string
	Pantalla  string
	Version   string

	Estado        Estado
	Respuesta     string
	RespondidaPor *string
	RespondidaEn  *time.Time
	CreadaEn      time.Time
}

func Nueva(id, usuarioID string, tipo Tipo, texto, pantalla, version string, ahora time.Time) (*Sugerencia, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, ErrTextoVacio
	}
	if len([]rune(texto)) > MaxTexto {
		return nil, ErrTextoLargo
	}
	if len([]rune(pantalla)) > 200 {
		return nil, ErrPantallaLarga
	}

	return &Sugerencia{
		ID:        id,
		UsuarioID: usuarioID,
		Tipo:      tipo,
		Texto:     texto,
		Pantalla:  strings.TrimSpace(pantalla),
		Version:   strings.TrimSpace(version),
		Estado:    Abierta,
		CreadaEn:  ahora,
	}, nil
}

// Responder deja la respuesta del Admin y da la sugerencia por resuelta.
//
// Las dos cosas van juntas a propósito: responder es lo que la cierra. Una
// respuesta sin cerrar dejaría el mensaje en la lista de pendientes para
// siempre, y cerrar sin responder es lo que hace que la próxima vez nadie
// escriba.
func (s *Sugerencia) Responder(respuesta string, adminID string, ahora time.Time) error {
	if s.Estado == Resuelta {
		return ErrYaResuelta
	}
	respuesta = strings.TrimSpace(respuesta)
	if respuesta == "" {
		return ErrRespuestaVacia
	}
	if len([]rune(respuesta)) > MaxTexto {
		return ErrRespuestaLarga
	}

	s.Estado = Resuelta
	s.Respuesta = respuesta
	s.RespondidaPor = &adminID
	s.RespondidaEn = &ahora
	return nil
}
