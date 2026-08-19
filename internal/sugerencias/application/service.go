// Package application orquesta el buzón: escribir un mensaje, listarlo y
// contestarlo. Ver ports.go para los puertos hacia infrastructure/.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"github.com/ramiro/sgrc/internal/sugerencias/domain"
)

// ObtenedorDeUsuario resuelve el nombre y el mail de quien escribió, que
// viven en auth.
//
// Es un puerto y no una consulta con JOIN por dos razones: mantiene a este
// paquete sin saber cómo está guardada una cuenta, y hace explícito que lo
// único que necesita del usuario son tres campos para el aviso.
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

// Escribir guarda el mensaje y avisa a los Admin.
//
// La pantalla y la versión las manda el frontend, no la persona: ver el
// comentario de domain.Sugerencia.
func (s *Service) Escribir(ctx context.Context, usuarioID, tipo, texto, pantalla, version string) (*domain.Sugerencia, error) {
	t, err := domain.ParseTipo(tipo)
	if err != nil {
		return nil, err
	}

	sug, err := domain.Nueva(s.nuevoID(), usuarioID, t, texto, pantalla, version, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Crear(ctx, sug); err != nil {
		return nil, fmt.Errorf("guardando la sugerencia: %w", err)
	}

	// El nombre se resuelve para el aviso, y si falla no se deshace nada: el
	// mensaje ya está guardado, que es lo que no se puede perder. Sin nombre
	// el aviso sale igual, con el texto, que es lo que importa leer.
	nombre, _, err := s.usuario.NombreYEmail(ctx, usuarioID)
	if err != nil {
		nombre = ""
	}

	s.bus.Publish(eventbus.Evento{
		Tipo: "sugerencia.nueva",
		Payload: eventbus.SugerenciaNueva{
			SugerenciaID: sug.ID,
			Quien:        nombre,
			Tipo:         string(sug.Tipo),
			Texto:        sug.Texto,
			Pantalla:     sug.Pantalla,
		},
	})
	return sug, nil
}

// Responder contesta y cierra (ver domain.Sugerencia.Responder).
func (s *Service) Responder(ctx context.Context, sugerenciaID, adminID, respuesta string) (*domain.Sugerencia, error) {
	sug, err := s.repo.BuscarPorID(ctx, sugerenciaID)
	if err != nil {
		return nil, err
	}
	if err := sug.Responder(respuesta, adminID, s.ahora()); err != nil {
		return nil, err
	}
	if err := s.repo.Guardar(ctx, sug); err != nil {
		return nil, fmt.Errorf("guardando la respuesta: %w", err)
	}

	nombre, email, err := s.usuario.NombreYEmail(ctx, sug.UsuarioID)
	if err != nil {
		// Igual que arriba: la respuesta ya está guardada y se ve en la
		// pantalla de quien preguntó. Lo que se pierde es el aviso.
		return sug, nil
	}

	s.bus.Publish(eventbus.Evento{
		Tipo: "sugerencia.respondida",
		Payload: eventbus.SugerenciaRespondida{
			SugerenciaID:  sug.ID,
			UsuarioID:     sug.UsuarioID,
			Email:         email,
			Nombre:        nombre,
			TextoOriginal: sug.Texto,
			Respuesta:     sug.Respuesta,
		},
	})
	return sug, nil
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

// ContarAbiertas alimenta el número del panel del Admin.
func (s *Service) ContarAbiertas(ctx context.Context) (int, error) {
	return s.repo.ContarAbiertas(ctx)
}
