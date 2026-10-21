// Package application orquesta el buzón: escribir un mensaje, listarlo y
// contestarlo. Ver ports.go para los puertos hacia infrastructure/.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/paginacion"
	"github.com/agustin-y2k/sgrc/internal/sugerencias/domain"
)

// ObtenedorDeUsuario resuelve el nombre y el mail de quien escribió, que
// viven en auth.
type ObtenedorDeUsuario interface {
	NombreYEmail(ctx context.Context, usuarioID string) (nombre, email string, err error)
}

type Service struct {
	repo    Repo
	usuario ObtenedorDeUsuario
	nuevoID IDGenerator
	ahora   func() time.Time
	bus     eventbus.EventBus
}

func NewService(repo Repo, usuario ObtenedorDeUsuario, nuevoID IDGenerator, ahora func() time.Time, bus eventbus.EventBus) *Service {
	return &Service{repo: repo, usuario: usuario, nuevoID: nuevoID, ahora: ahora, bus: bus}
}

// Escribir abre un hilo nuevo y avisa a los Admin.
func (s *Service) operacionEscribir(ctx context.Context, usuarioID, tipo, asunto, texto, pantalla, version string) (*domain.Sugerencia, error) {
	t, err := domain.ParseTipo(tipo)
	if err != nil {
		return nil, err
	}

	sug, err := domain.Nueva(s.nuevoID(), s.nuevoID(), usuarioID, t, asunto, texto, pantalla, version, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Crear(ctx, sug); err != nil {
		return nil, fmt.Errorf("guardando la sugerencia: %w", err)
	}

	// El nombre se resuelve para el aviso, y si falla no se deshace nada: el
	// mensaje ya está guardado, que es lo que no se puede perder.
	nombre, _, err := s.usuario.NombreYEmail(ctx, usuarioID)
	if err != nil {
		nombre = ""
	}

	eventbus.Emitir(ctx, s.bus, eventbus.Evento{
		Tipo: "sugerencia.nueva",
		Payload: eventbus.SugerenciaNueva{
			SugerenciaID: sug.ID,
			UsuarioID:    usuarioID,
			Quien:        nombre,
			Tipo:         string(sug.Tipo),
			Asunto:       sug.Asunto,
			Texto:        sug.PrimerMensaje().Texto,
			Pantalla:     sug.Pantalla,
		},
	})
	return sug, nil
}

// Responder agrega un mensaje al hilo. Lo usan los dos lados: si lo escribe
// un Admin el aviso va a quien preguntó, y si lo escribe quien preguntó, a
// los Admin.
//
// Contestar ya no cierra el hilo (ver domain.Sugerencia.Responder): lo cierra
// MarcarResuelta, y un mensaje de quien preguntó lo reabre.
func (s *Service) operacionResponder(ctx context.Context, sugerenciaID, autorID string, deAdmin bool, texto string) (*domain.Sugerencia, error) {
	sug, err := s.repo.BuscarPorID(ctx, sugerenciaID)
	if err != nil {
		return nil, err
	}
	// Solo el dueño del hilo o un Admin pueden escribir en él. El chequeo va
	// acá y no en el handler porque es una regla del caso de uso: una
	// conversación es entre dos, no un tablón.
	if !deAdmin && sug.UsuarioID != autorID {
		return nil, ErrNoEsTuya
	}

	if err := sug.Responder(s.nuevoID(), autorID, deAdmin, texto, s.ahora()); err != nil {
		return nil, err
	}
	if err := s.repo.AgregarMensaje(ctx, sug, sug.UltimoMensaje()); err != nil {
		return nil, fmt.Errorf("guardando la respuesta: %w", err)
	}

	nombre, email, err := s.usuario.NombreYEmail(ctx, sug.UsuarioID)
	if err != nil {
		// El destinatario interno se conoce por ID. La falta del contacto sólo
		// impide el correo; no debe perder el aviso ni el cierre del pendiente.
		nombre, email = "", ""
	}

	if deAdmin {
		eventbus.Emitir(ctx, s.bus, eventbus.Evento{
			Tipo: "sugerencia.respondida",
			Payload: eventbus.SugerenciaRespondida{
				SugerenciaID:  sug.ID,
				UsuarioID:     sug.UsuarioID,
				Email:         email,
				Nombre:        nombre,
				Tipo:          string(sug.Tipo),
				Asunto:        sug.Asunto,
				TextoOriginal: sug.PrimerMensaje().Texto,
				Respuesta:     sug.UltimoMensaje().Texto,
			},
		})
		return sug, nil
	}

	// Escribió quien preguntó: vuelve a los Admin, con el mismo formato que
	// un hilo nuevo salvo por que dice que es una respuesta.
	eventbus.Emitir(ctx, s.bus, eventbus.Evento{
		Tipo: "sugerencia.seguimiento",
		Payload: eventbus.SugerenciaSeguimiento{
			SugerenciaID: sug.ID,
			UsuarioID:    sug.UsuarioID,
			Quien:        nombre,
			Tipo:         string(sug.Tipo),
			Asunto:       sug.Asunto,
			Texto:        sug.UltimoMensaje().Texto,
		},
	})
	return sug, nil
}

// MarcarResuelta cierra el hilo. No manda correo: el aviso útil fue la
// respuesta, y un segundo mail diciendo "lo dimos por terminado" es ruido.
func (s *Service) operacionMarcarResuelta(ctx context.Context, sugerenciaID string) (*domain.Sugerencia, error) {
	sug, err := s.repo.BuscarPorID(ctx, sugerenciaID)
	if err != nil {
		return nil, err
	}
	if err := sug.MarcarResuelta(s.ahora()); err != nil {
		return nil, err
	}
	if err := s.repo.GuardarEstado(ctx, sug); err != nil {
		return nil, fmt.Errorf("cerrando la conversación: %w", err)
	}
	eventbus.Emitir(ctx, s.bus, eventbus.Evento{Tipo: "sugerencia.resuelta", Payload: map[string]string{"usuarioId": sug.UsuarioID, "hiloId": sug.ID}})
	return sug, nil
}

// Obtener permite abrir un aviso sin depender de la página o del filtro del buzón.
func (s *Service) Obtener(ctx context.Context, id, usuarioID string, esAdmin bool) (*domain.Sugerencia, error) {
	hilo, err := s.repo.BuscarPorID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !esAdmin && hilo.UsuarioID != usuarioID {
		return nil, ErrNoEsTuya
	}
	return hilo, nil
}

// ListarTodas es la pantalla del Admin.
func (s *Service) ListarTodas(ctx context.Context, soloAbiertas bool, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	return s.repo.ListarTodas(ctx, soloAbiertas, p)
}

// ListarPropias es lo que ve quien escribió: sus mensajes y las respuestas
// que le dieron.
func (s *Service) ListarPropias(ctx context.Context, usuarioID string, p paginacion.Pagina) ([]*domain.Sugerencia, int, error) {
	return s.repo.ListarDeUsuario(ctx, usuarioID, p)
}
