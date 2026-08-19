// Package domain son las reglas de la bandeja de soporte: por acá se pide
// ayuda, se cuenta que algo del sistema no anda, o se propone un cambio.
//
// El nombre del paquete quedó de cuando esto era solo un buzón de
// sugerencias. Lo que modela hoy es una conversación con ida y vuelta.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTextoVacio        = errors.New("hay que escribir el mensaje")
	ErrTextoLargo        = fmt.Errorf("el mensaje no puede pasar de %d caracteres", MaxTexto)
	ErrAsuntoVacio       = errors.New("hay que escribir un asunto")
	ErrAsuntoLargo       = fmt.Errorf("el asunto no puede pasar de %d caracteres", MaxAsunto)
	ErrTipoInvalido      = errors.New("tipo de mensaje inválido")
	ErrYaResuelta        = errors.New("esa conversación ya está resuelta")
	ErrPantallaLarga     = errors.New("la pantalla no puede pasar de 200 caracteres")
	ErrSugerenciaNoExist = errors.New("no se encontró esa conversación")
)

// MaxTexto: cuatro mil caracteres son unas dos carillas.
const MaxTexto = 4000

// MaxAsunto es corto a propósito: el asunto se lee en una lista, y uno que no
// entra en un renglón deja de servir para elegir cuál abrir primero.
const MaxAsunto = 150

// Tipo separa "necesito que alguien me ayude ahora" de "no me deja hacer
// algo" y de "estaría bueno que…", que es la primera pregunta que se hace
// quien lee: una cosa hay que atenderla, otra arreglarla y la otra pensarla.
type Tipo string

const (
	// Los nombres llevan el prefijo porque `Sugerencia` a secas es el tipo
	// de más abajo — la entidad, no la clase de mensaje.

	// TipoAyuda es el pedido de soporte: alguien necesita una mano para poder
	// dar su clase. Es el único cuyo correo no se puede desactivar.
	TipoAyuda Tipo = "AYUDA"
	// TipoProblema: algo del sistema no funciona.
	TipoProblema Tipo = "PROBLEMA"
	// TipoSugerencia: estaría bueno que el sistema hiciera otra cosa.
	TipoSugerencia Tipo = "SUGERENCIA"
)

// Tipos son los tres, en el orden en que se ofrecen: primero el urgente.
func Tipos() []Tipo {
	return []Tipo{TipoAyuda, TipoProblema, TipoSugerencia}
}

func ParseTipo(s string) (Tipo, error) {
	for _, t := range Tipos() {
		if Tipo(s) == t {
			return t, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrTipoInvalido, s)
}

// EsPedidoDeAyuda decide si los correos de este hilo son obligatorios. Vive
// en el dominio y no en el módulo de correo porque es una regla del negocio:
// a quien pide ayuda no se le puede fallar el aviso.
func (t Tipo) EsPedidoDeAyuda() bool {
	return t == TipoAyuda
}

type Estado string

const (
	Abierta  Estado = "ABIERTA"
	Resuelta Estado = "RESUELTA"
)

// Mensaje es una intervención del hilo.
type Mensaje struct {
	ID string
	// AutorID en nil si esa cuenta se eliminó.
	AutorID *string
	// DeAdmin dice de qué lado viene, y se guarda en vez de deducirse del rol
	// actual del autor: si a un docente lo ascienden, lo que escribió antes lo
	// escribió como docente.
	DeAdmin   bool
	Texto     string
	EscritoEn time.Time
}

// Sugerencia es una conversación entre quien escribió y el equipo de
// administración.
type Sugerencia struct {
	ID        string
	UsuarioID string
	Tipo      Tipo
	Asunto    string
	Pantalla  string
	Version   string

	Estado            Estado
	CreadaEn          time.Time
	UltimaActividadEn time.Time
	// Mensajes en orden cronológico. El primero es el de quien abrió el hilo.
	Mensajes []Mensaje
}

// Nueva abre el hilo con su primer mensaje.
func Nueva(id, mensajeID, usuarioID string, tipo Tipo, asunto, texto, pantalla, version string, ahora time.Time) (*Sugerencia, error) {
	asunto = strings.TrimSpace(asunto)
	if asunto == "" {
		return nil, ErrAsuntoVacio
	}
	if len([]rune(asunto)) > MaxAsunto {
		return nil, ErrAsuntoLargo
	}
	if len([]rune(pantalla)) > 200 {
		return nil, ErrPantallaLarga
	}

	primero, err := nuevoMensaje(mensajeID, usuarioID, false, texto, ahora)
	if err != nil {
		return nil, err
	}

	return &Sugerencia{
		ID:                id,
		UsuarioID:         usuarioID,
		Tipo:              tipo,
		Asunto:            asunto,
		Pantalla:          strings.TrimSpace(pantalla),
		Version:           strings.TrimSpace(version),
		Estado:            Abierta,
		CreadaEn:          ahora,
		UltimaActividadEn: ahora,
		Mensajes:          []Mensaje{primero},
	}, nil
}

func nuevoMensaje(id, autorID string, deAdmin bool, texto string, ahora time.Time) (Mensaje, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return Mensaje{}, ErrTextoVacio
	}
	if len([]rune(texto)) > MaxTexto {
		return Mensaje{}, ErrTextoLargo
	}
	autor := autorID
	return Mensaje{
		ID:        id,
		AutorID:   &autor,
		DeAdmin:   deAdmin,
		Texto:     texto,
		EscritoEn: ahora,
	}, nil
}

// Responder agrega un mensaje al hilo, venga del lado que venga.
//
// Contestar NO cierra: antes sí, y era el problema — el Admin escribía "fijate
// en Reservas" y la conversación terminaba ahí, sin manera de decirle "ya
// probé y no está". Cerrar es ahora un acto aparte (MarcarResuelta), y un
// mensaje de quien preguntó reabre el hilo: si volvió a escribir, no estaba
// resuelto.
func (s *Sugerencia) Responder(mensajeID, autorID string, deAdmin bool, texto string, ahora time.Time) error {
	m, err := nuevoMensaje(mensajeID, autorID, deAdmin, texto, ahora)
	if err != nil {
		return err
	}

	if !deAdmin {
		s.Estado = Abierta
	}
	s.Mensajes = append(s.Mensajes, m)
	s.UltimaActividadEn = ahora
	return nil
}

// MarcarResuelta cierra el hilo. Lo hace el Admin cuando el tema terminó.
func (s *Sugerencia) MarcarResuelta(ahora time.Time) error {
	if s.Estado == Resuelta {
		return ErrYaResuelta
	}
	s.Estado = Resuelta
	s.UltimaActividadEn = ahora
	return nil
}

// UltimoMensaje es el que se muestra en la lista sin abrir el hilo.
func (s *Sugerencia) UltimoMensaje() Mensaje {
	if len(s.Mensajes) == 0 {
		return Mensaje{}
	}
	return s.Mensajes[len(s.Mensajes)-1]
}

// PrimerMensaje es con lo que abrió el hilo quien lo escribió: es lo que va
// en el correo de aviso, no la última respuesta.
func (s *Sugerencia) PrimerMensaje() Mensaje {
	if len(s.Mensajes) == 0 {
		return Mensaje{}
	}
	return s.Mensajes[0]
}

// EsperaRespuestaDelAdmin dice si la pelota está del lado de administración:
// el hilo está abierto y el último que habló fue quien pidió.
func (s *Sugerencia) EsperaRespuestaDelAdmin() bool {
	return s.Estado == Abierta && !s.UltimoMensaje().DeAdmin
}
