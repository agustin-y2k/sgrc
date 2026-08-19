package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ramiro/sgrc/internal/academic/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Los pedidos para dictar una materia (ver domain.PedidoDeMateria).
//
// Van en su propio archivo y no en service.go porque son un caso de uso
// distinto de los de ahí: aquellos los hace un Admin sobre la estructura del
// año; estos los inicia un docente y terminan en una conversación fuera del
// sistema.

var (
	ErrYaDictaLaMateria = errors.New("ya estás asignado a esa materia")
	ErrPedidoDuplicado  = errors.New("ya tenés un pedido sin resolver para esa materia")
	// ErrFaltaCursoParaMateriaNueva: al aprobar un pedido de una materia que
	// no existe hay que decir en qué curso crearla. El sistema no lo adivina
	// del texto que escribió el docente ("Robótica de 5°B" es una frase, no
	// un curso), y equivocarse acá crea una materia colgada del curso
	// equivocado, que después hay que borrar a mano.
	ErrFaltaCursoParaMateriaNueva = errors.New("hay que indicar en qué curso se crea la materia")
)

// PedirMateria: un docente pide poder dictar una materia. `materiaID` en nil
// significa que todavía no existe y va escrita a mano.
func (s *Service) PedirMateria(ctx context.Context, usuarioID string, materiaID *string, curso, materia, motivo string) (*domain.PedidoDeMateria, error) {
	// El pedido se arma PRIMERO, antes de tocar la base: así una entrada mal
	// formada —sin motivo, sin materia, con las dos formas a la vez— se
	// rechaza por lo que es (400) y no por lo primero que encuentre una
	// consulta. Si alguien manda un pedido sin motivo sobre una materia que
	// ya pidió, el problema es el motivo que falta, no el duplicado.
	p, err := domain.NuevoPedidoDeMateria(s.nuevoID(), usuarioID, materiaID, curso, materia, motivo, s.ahora())
	if err != nil {
		return nil, err
	}

	// Recién ahora, contra la realidad: que la materia exista, que no la
	// dicte ya, y que no haya pedido lo mismo hace cinco minutos.
	var materiaNombre, cursoNombre string
	var docentes []ContactoDeDocente

	if materiaID != nil {
		m, err := s.repo.BuscarMateriaPorID(ctx, *materiaID)
		if err != nil {
			return nil, err
		}
		materiaNombre = m.Nombre

		if c, err := s.repo.BuscarCursoPorID(ctx, m.CursoID); err == nil {
			cursoNombre = c.Nombre
		}

		asignados, err := s.repo.ListarDocentesDeMateria(ctx, *materiaID)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(asignados))
		for _, dm := range asignados {
			if dm.UsuarioID == usuarioID {
				return nil, ErrYaDictaLaMateria
			}
			ids = append(ids, dm.UsuarioID)
		}
		if docentes, err = s.datosDeUsuario.Contactos(ctx, ids); err != nil {
			// Sin los contactos el pedido se crea igual: lo que se pierde es
			// el aviso a quienes ya la dictan, no el pedido.
			docentes = nil
		}

		abierto, err := s.repo.TienePedidoAbierto(ctx, usuarioID, *materiaID)
		if err != nil {
			return nil, err
		}
		if abierto {
			return nil, ErrPedidoDuplicado
		}
	} else {
		materiaNombre = strings.TrimSpace(materia)
		cursoNombre = strings.TrimSpace(curso)
	}

	if err := s.repo.CrearPedido(ctx, p); err != nil {
		return nil, err
	}

	quien, err := s.datosDeUsuario.Contacto(ctx, usuarioID)
	if err != nil {
		quien = ContactoDeDocente{UsuarioID: usuarioID}
	}

	s.bus.Publish(eventbus.Evento{
		Tipo: "materia.pedido.nuevo",
		Payload: eventbus.PedidoDeMateriaNuevo{
			PedidoID:         p.ID,
			UsuarioID:        usuarioID,
			Nombre:           quien.Nombre,
			MateriaNombre:    materiaNombre,
			CursoNombre:      cursoNombre,
			EsMateriaNueva:   p.EsMateriaNueva(),
			Motivo:           p.Motivo,
			DocentesActuales: aDocentesDeEvento(docentes),
		},
	})
	return p, nil
}

// ResolverPedido aprueba o rechaza.
//
// Al aprobar hace las dos cosas que el Admin haría a mano: crea la materia
// si no existía —en el curso que él indique— y asigna al docente. Si algo de
// eso falla, el pedido queda pendiente: dar por resuelto lo que no se pudo
// aplicar dejaría a la persona con un "aprobado" y sin poder reservar.
// rolPorDefecto: si la materia todavía no tiene a nadie, quien la pide es su
// titular; si ya la da alguien, el que se suma entra como suplente.
//
// El rol no da ni quita permisos (RF-02.6: es informativo), así que esto no
// cambia lo que cada uno puede hacer. Lo que cuida es el dato: en una escuela
// hay suplentes que cubren un cargo durante años, y también materias sin
// titular con un suplente a cargo. Marcar "titular" a cualquiera que se suma
// deja un registro que después nadie puede leer para saber quién es quién.
// El Admin lo cambia al resolver si sabe que es al revés.
func (s *Service) rolPorDefecto(ctx context.Context, materiaID string) domain.RolDocente {
	asignados, err := s.repo.ListarDocentesDeMateria(ctx, materiaID)
	if err != nil || len(asignados) == 0 {
		return domain.RolTitular
	}
	return domain.RolSuplente
}

func (s *Service) ResolverPedido(ctx context.Context, pedidoID, adminID string, aprobar bool, respuesta string, cursoIDParaMateriaNueva *string, rol *domain.RolDocente) (*domain.PedidoDeMateria, error) {
	p, err := s.repo.BuscarPedidoPorID(ctx, pedidoID)
	if err != nil {
		return nil, err
	}
	if p.Estado != domain.PedidoPendiente {
		return nil, domain.ErrPedidoResuelto
	}

	materiaNombre := p.MateriaSolicitada

	if aprobar {
		materiaID := p.MateriaID

		if p.EsMateriaNueva() {
			if cursoIDParaMateriaNueva == nil || strings.TrimSpace(*cursoIDParaMateriaNueva) == "" {
				return nil, ErrFaltaCursoParaMateriaNueva
			}
			m, err := s.CrearMateria(ctx, *cursoIDParaMateriaNueva, p.MateriaSolicitada)
			if err != nil {
				return nil, fmt.Errorf("creando la materia del pedido: %w", err)
			}
			materiaID = &m.ID
			materiaNombre = m.Nombre
		} else {
			m, err := s.repo.BuscarMateriaPorID(ctx, *materiaID)
			if err != nil {
				return nil, err
			}
			materiaNombre = m.Nombre
		}

		elegido := s.rolPorDefecto(ctx, *materiaID)
		if rol != nil {
			elegido = *rol
		}
		if _, err := s.AsignarDocente(ctx, *materiaID, p.UsuarioID, elegido); err != nil {
			return nil, fmt.Errorf("asignando el docente a la materia: %w", err)
		}
		if err := p.Aprobar(adminID, respuesta, s.ahora()); err != nil {
			return nil, err
		}
	} else {
		if err := p.Rechazar(adminID, respuesta, s.ahora()); err != nil {
			return nil, err
		}
	}

	if err := s.repo.GuardarPedido(ctx, p); err != nil {
		return nil, err
	}

	quien, err := s.datosDeUsuario.Contacto(ctx, p.UsuarioID)
	if err != nil {
		quien = ContactoDeDocente{UsuarioID: p.UsuarioID}
	}

	s.bus.Publish(eventbus.Evento{
		Tipo: "materia.pedido.resuelto",
		Payload: eventbus.PedidoDeMateriaResuelto{
			PedidoID:      p.ID,
			UsuarioID:     p.UsuarioID,
			Email:         quien.Email,
			Nombre:        quien.Nombre,
			MateriaNombre: materiaNombre,
			Aprobado:      aprobar,
			Respuesta:     p.Respuesta,
		},
	})
	return p, nil
}

func (s *Service) ListarPedidos(ctx context.Context, soloPendientes bool) ([]*domain.PedidoDeMateria, error) {
	return s.repo.ListarPedidos(ctx, soloPendientes)
}

func (s *Service) ListarMisPedidos(ctx context.Context, usuarioID string) ([]*domain.PedidoDeMateria, error) {
	return s.repo.ListarPedidosDeUsuario(ctx, usuarioID)
}

func (s *Service) ContarPedidosPendientes(ctx context.Context) (int, error) {
	return s.repo.ContarPedidosPendientes(ctx)
}

func aDocentesDeEvento(cs []ContactoDeDocente) []eventbus.DocenteDeMateria {
	if len(cs) == 0 {
		return nil
	}
	r := make([]eventbus.DocenteDeMateria, 0, len(cs))
	for _, c := range cs {
		r = append(r, eventbus.DocenteDeMateria{UsuarioID: c.UsuarioID, Email: c.Email, Nombre: c.Nombre})
	}
	return r
}
