// Package application orquesta los casos de uso de RF-05 (notificaciones
// internas). Ver subscribers.go para cómo se conecta con los eventos que
// auth y reservation ya publican.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

type Service struct {
	repo           Repo
	listadorAdmins ListadorAdmins
	nuevoID        IDGenerator
	ahora          func() time.Time
}

func NewService(repo Repo, listadorAdmins ListadorAdmins, nuevoID IDGenerator, ahora func() time.Time) *Service {
	return &Service{repo: repo, listadorAdmins: listadorAdmins, nuevoID: nuevoID, ahora: ahora}
}

// NotificarUsuario crea una notificación para un usuario puntual. El tipo
// es lo que después le permite a la interfaz ofrecer la acción que
// corresponde sin leer el mensaje (ver domain.Tipo).
func (s *Service) NotificarUsuario(ctx context.Context, usuarioID, mensaje string, tipo domain.Tipo, ref domain.Referencias) (*domain.Notificacion, error) {
	n, err := domain.NuevaNotificacion(s.nuevoID(), usuarioID, mensaje, tipo, ref, s.ahora())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Crear(ctx, n); err != nil {
		return nil, fmt.Errorf("creando notificación: %w", err)
	}
	return n, nil
}

// NotificarATodosLosAdmins implementa el patrón que usan RF-05.4/05.5/05.6
// — un evento le llega a TODOS los Admin en estado APROBADA, no a uno
// solo. Devuelve cuántas notificaciones se crearon.
func (s *Service) NotificarATodosLosAdmins(ctx context.Context, mensaje string, tipo domain.Tipo, ref domain.Referencias) (int, error) {
	adminIDs, err := s.listadorAdmins.IDsDeAdminsAprobados(ctx)
	if err != nil {
		return 0, fmt.Errorf("listando admins aprobados: %w", err)
	}

	creadas := 0
	for _, adminID := range adminIDs {
		if _, err := s.NotificarUsuario(ctx, adminID, mensaje, tipo, ref); err != nil {
			return creadas, err
		}
		creadas++
	}
	return creadas, nil
}

// CerrarAvisosSobreUsuario marca como leídas las notificaciones de un tipo
// que hablan de una persona puntual.
//
// Existe para que el aviso "X está pendiente de aprobación" desaparezca solo
// cuando alguien aprueba o rechaza a X: el aviso pedía una acción y esa
// acción ya se hizo. Alcanza a TODOS los Admin, no solo al que resolvió —
// para los demás el pendiente tampoco existe más, y dejarles el aviso los
// manda a una lista donde esa persona ya no está.
//
// Devuelve cuántas cerró. No es un error que sean cero: puede que el aviso
// ya estuviera leído, o que la cuenta sea anterior a que las notificaciones
// supieran de quién hablaban (ver migración 007).
func (s *Service) CerrarAvisosSobreUsuario(ctx context.Context, sobreUsuarioID string, tipo domain.Tipo) (int, error) {
	pendientes, err := s.repo.ListarNoLeidasSobreUsuario(ctx, sobreUsuarioID, tipo)
	if err != nil {
		return 0, fmt.Errorf("buscando avisos sobre el usuario: %w", err)
	}

	cerradas := 0
	for _, n := range pendientes {
		if err := n.MarcarLeida(s.ahora()); err != nil {
			continue // ya estaba leída: no es un error
		}
		if err := s.repo.Guardar(ctx, n); err != nil {
			return cerradas, fmt.Errorf("cerrando aviso %s: %w", n.ID, err)
		}
		cerradas++
	}
	return cerradas, nil
}

// ObtenerNotificacion es un passthrough directo al repo — usado por
// interfaces/http para verificar la titularidad de una notificación antes
// de dejarla marcar como leída.
func (s *Service) ObtenerNotificacion(ctx context.Context, id string) (*domain.Notificacion, error) {
	return s.repo.BuscarPorID(ctx, id)
}

// MarcarLeida marca una notificación puntual como leída.
func (s *Service) MarcarLeida(ctx context.Context, notificacionID string) error {
	n, err := s.repo.BuscarPorID(ctx, notificacionID)
	if err != nil {
		return err
	}
	if err := n.MarcarLeida(s.ahora()); err != nil {
		return err
	}
	return s.repo.Guardar(ctx, n)
}

// MarcarTodasLeidas marca como leídas todas las notificaciones sin leer
// del usuario, y devuelve cuántas cambiaron (RF-05.7).
func (s *Service) MarcarTodasLeidas(ctx context.Context, usuarioID string) (int, error) {
	return s.repo.MarcarTodasLeidasDe(ctx, usuarioID, s.ahora())
}

// ListarPorUsuario devuelve las notificaciones de un usuario (nil =
// todas, sin filtrar por estado).
func (s *Service) ListarPorUsuario(ctx context.Context, usuarioID string, filtroEstado *domain.Estado, pagina paginacion.Pagina) ([]*domain.Notificacion, int, error) {
	// Una página sin inicializar daría LIMIT 0, o sea una lista vacía sin
	// ningún error: se completa con la ventana por defecto.
	if pagina.Tamanio <= 0 || pagina.Numero <= 0 {
		pagina = paginacion.PorDefecto()
	}
	return s.repo.ListarPorUsuario(ctx, usuarioID, filtroEstado, pagina)
}
