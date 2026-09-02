// Package application orquesta los casos de uso de RF-05 (notificaciones
// internas).
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/notification/domain"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
	"strings"
)

type Service struct {
	repo           Repo
	listadorAdmins ListadorAdmins
	preferencias   PreferenciasEmail
	nuevoID        IDGenerator
	ahora          func() time.Time
}

func NewService(repo Repo, listadorAdmins ListadorAdmins, preferencias PreferenciasEmail, nuevoID IDGenerator, ahora func() time.Time) *Service {
	return &Service{
		repo:           repo,
		listadorAdmins: listadorAdmins,
		preferencias:   preferencias,
		nuevoID:        nuevoID,
		ahora:          ahora,
	}
}

// NotificarUsuario crea una notificación para un usuario puntual.
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

// NotificarATodosLosAdmins implementa el patrón que usan RF-05.4/05.5/05.6 —
// un evento le llega a TODOS los Admin en estado APROBADA, no a uno solo.
func (s *Service) NotificarATodosLosAdmins(ctx context.Context, mensaje string, tipo domain.Tipo, ref domain.Referencias) (int, error) {
	adminIDs, err := s.listadorAdmins.IDsDeAdminsAprobados(ctx)
	if err != nil {
		return 0, fmt.Errorf("listando admins aprobados: %w", err)
	}

	creadas := 0
	var fallidos []string
	for _, adminID := range adminIDs {
		if _, err := s.NotificarUsuario(ctx, adminID, mensaje, tipo, ref); err != nil {
			fallidos = append(fallidos, fmt.Sprintf("%s (%v)", adminID, err))
			continue
		}
		creadas++
	}
	if len(fallidos) > 0 {
		return creadas, fmt.Errorf("no se pudo notificar a %d de %d admins: %s",
			len(fallidos), len(adminIDs), strings.Join(fallidos, "; "))
	}
	return creadas, nil
}

// NotificarATodosLosAdminsSinRepetir es NotificarATodosLosAdmins con una
// condición: a un Admin que ya tiene un aviso sin leer de ese tipo sobre esa
// misma persona, no se le escribe otro.
//
// Existe por el buzón. Una conversación de ida y vuelta —seis mensajes de la
// misma persona sobre el mismo tema— dejaba seis avisos sin leer en la
// campana de CADA Admin, y ninguno decía nada que el primero no dijera ya:
// "fulano escribió". El aviso no es un registro de mensajes, que para eso
// está el hilo; es un "andá a mirar", y con uno alcanza hasta que alguien
// vaya.
//
// La condición es POR ADMIN y no global: que uno haya leído el suyo no
// significa que los otros tres se hayan enterado.
func (s *Service) NotificarATodosLosAdminsSinRepetir(ctx context.Context, sobreUsuarioID, mensaje string, tipo domain.Tipo) (int, error) {
	adminIDs, err := s.listadorAdmins.IDsDeAdminsAprobados(ctx)
	if err != nil {
		return 0, fmt.Errorf("listando admins aprobados: %w", err)
	}

	// Una sola consulta para todos: devuelve los avisos sin leer de ese tipo
	// sobre esa persona, de quien sea.
	pendientes, err := s.repo.ListarNoLeidasSobreUsuario(ctx, sobreUsuarioID, tipo)
	if err != nil {
		return 0, fmt.Errorf("buscando avisos pendientes sobre el usuario: %w", err)
	}
	yaAvisado := make(map[string]bool, len(pendientes))
	for _, n := range pendientes {
		yaAvisado[n.UsuarioID] = true
	}

	ref := domain.Referencias{SobreUsuarioID: &sobreUsuarioID}
	creadas := 0
	var fallidos []string
	for _, adminID := range adminIDs {
		if yaAvisado[adminID] {
			continue
		}
		if _, err := s.NotificarUsuario(ctx, adminID, mensaje, tipo, ref); err != nil {
			fallidos = append(fallidos, fmt.Sprintf("%s (%v)", adminID, err))
			continue
		}
		creadas++
	}
	if len(fallidos) > 0 {
		return creadas, fmt.Errorf("no se pudo notificar a %d de %d admins: %s",
			len(fallidos), len(adminIDs), strings.Join(fallidos, "; "))
	}
	return creadas, nil
}

// CerrarAvisosSobreUsuario marca como leídas las notificaciones de un tipo
// que hablan de una persona puntual.
//
// Es lo que hace que un aviso dirigido a TODOS los Admin no se convierta en
// trabajo para todos: cuando uno lo resuelve —aprueba la cuenta, contesta el
// hilo, decide el pedido de materia—, el pendiente deja de estar pendiente
// para los demás. Sin esto, el que resuelve tacha el suyo y los otros tres se
// quedan con un aviso sin leer sobre algo que ya no existe, para siempre.
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

// CerrarAvisosPendientesDe cierra todos los avisos sin leer de un tipo, para
// todos los Admin, porque lo que los motivaba dejó de existir.
//
// La diferencia con CerrarAvisosSobreUsuario es de qué habla el aviso. Aquel
// habla de una persona y se cierra cuando esa persona se resuelve. Estos
// hablan de un CONJUNTO —"hay licencias por renovar", "quedaron equipos
// afuera"— que además se arma de nuevo cada vez: las licencias no vencen
// todas el mismo día, y un aviso junta las que caen esa mañana.
//
// Por eso no alcanza con "se renovó una". El aviso deja de tener sentido
// cuando NO QUEDA NINGUNA pendiente, y quien sabe eso es el módulo dueño
// —inventory para las licencias, reservation para los equipos— que lo manda
// contado en el evento. Acá no se cuenta nada: se cierra o no se cierra.
func (s *Service) CerrarAvisosPendientesDe(ctx context.Context, tipo domain.Tipo) (int, error) {
	cerradas, err := s.repo.MarcarLeidasPorTipo(ctx, tipo, s.ahora())
	if err != nil {
		return 0, fmt.Errorf("cerrando los avisos de tipo %s: %w", tipo, err)
	}
	return cerradas, nil
}

// ObtenerNotificacion es un passthrough directo al repo — usado por
// interfaces/http para verificar la titularidad de una notificación antes de
// dejarla marcar como leída.
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

// ── Qué copias por correo quiere cada Admin (RF-05.13) ──────────────────

// CategoriasDeEmail devuelve qué copias por correo recibe hoy un usuario: lo
// que eligió, y para lo que no eligió, el valor por defecto de esa categoría.
// Solo las que le corresponden por su rol.
func (s *Service) CategoriasDeEmail(ctx context.Context, usuarioID string, esAdmin bool) ([]domain.CategoriaEmail, error) {
	elegidas, err := s.preferencias.ElegidasDe(ctx, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("leyendo las preferencias de correo: %w", err)
	}
	return domain.EfectivasPara(elegidas, esAdmin), nil
}

// GuardarCategoriasDeEmail deja al usuario suscrito exactamente a esas
// categorías, y devuelve cómo quedó. A partir de acá los valores por defecto
// no cuentan más para esta persona: guardar el panel es pronunciarse sobre
// todas las casillas que vio, incluidas las que dejó destildadas.
func (s *Service) GuardarCategoriasDeEmail(ctx context.Context, usuarioID string, categorias []domain.CategoriaEmail, esAdmin bool) ([]domain.CategoriaEmail, error) {
	decisiones := domain.Decisiones(categorias, esAdmin)
	if err := s.preferencias.Reemplazar(ctx, usuarioID, decisiones); err != nil {
		return nil, fmt.Errorf("guardando las preferencias de correo: %w", err)
	}
	return domain.EfectivasPara(decisiones, esAdmin), nil
}
