package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/agustin-y2k/sgrc/internal/reservation/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

type reposicionPlanificada struct {
	reserva          *domain.Reserva
	equipo           *EquipoDisponible
	etiquetaOriginal string
	motivo           string
	desplazadoPor    *string
	intentados       map[string]bool
	opciones         *OpcionesReposicion
	conflictos       map[string][]Solapamiento
}

// reponerEnCascada planifica antes de escribir y compara la prioridad sobre
// cada dispositivo, también entre reservas ya reubicadas en el plan. Cada
// pareja reserva/dispositivo se intenta una sola vez para terminar sin ciclos.
// La creación con desplazamientos usa coordinación exclusiva desde el comienzo.
func (s *Service) reponerEnCascada(ctx context.Context, repo Repo, iniciales []*domain.Reserva, equiposGanadores []string, fechasGanadoras []time.Time, horaInicioGanadora, horaFinGanadora time.Duration, motivo string, quien *string, ahora time.Time) (ImpactoDelRobo, []cancelacionPendiente, error) {
	protegidos := make(map[string]bool, len(equiposGanadores))
	for _, id := range equiposGanadores {
		protegidos[id] = true
	}
	vistos := make(map[string]bool)
	var plan []reposicionPlanificada
	var cola []int
	agregar := func(r *domain.Reserva, motivo string, quien *string) {
		vistos[r.ID] = true
		cola = append(cola, len(plan))
		plan = append(plan, reposicionPlanificada{reserva: r, motivo: motivo, desplazadoPor: quien, intentados: make(map[string]bool)})
	}
	ordenarReservasParaReposicion(iniciales)
	for _, r := range iniciales {
		if !vistos[r.ID] {
			agregar(r, motivo, quien)
		}
	}
	for turno := 0; turno < len(cola); turno++ {
		if err := ctx.Err(); err != nil {
			return ImpactoDelRobo{}, nil, err
		}
		i := cola[turno]
		r := plan[i].reserva
		if r.MateriaID == nil {
			return ImpactoDelRobo{}, nil, fmt.Errorf("reserva %s sin materia para reposición", r.ID)
		}
		if plan[i].opciones == nil {
			opciones, err := repo.OpcionesDeReposicion(ctx, r.EquipoID, *r.MateriaID)
			if err != nil {
				return ImpactoDelRobo{}, nil, err
			}
			plan[i].opciones = &opciones
			plan[i].etiquetaOriginal = opciones.EtiquetaOriginal
			plan[i].conflictos = make(map[string][]Solapamiento)
			ids := make([]string, 0, len(opciones.Equipos))
			for _, equipo := range opciones.Equipos {
				ids = append(ids, equipo.EquipoID)
			}
			if len(ids) > 0 {
				conflictos, err := repo.BuscarSolapamientos(ctx, ids, []time.Time{r.Fecha}, r.HoraInicio, r.HoraFin)
				if err != nil {
					return ImpactoDelRobo{}, nil, err
				}
				for _, c := range conflictos {
					plan[i].conflictos[c.EquipoID] = append(plan[i].conflictos[c.EquipoID], c)
				}
			}

		}
		opciones := plan[i].opciones
		var elegido *EquipoDisponible
		var aDesplazar []*domain.Reserva
		var reintentar []int
		// Se prefiere un libre a cualquier ocupado, sin desplazar por comodidad.
		for _, candidato := range opciones.Equipos {
			if plan[i].intentados[candidato.EquipoID] || (protegidos[candidato.EquipoID] && solapaConPedidoGanador(r, fechasGanadoras, horaInicioGanadora, horaFinGanadora)) {
				continue
			}
			ocupantes, permitido := ocupantesDelPlan(plan, candidato, r, ahora)
			if !permitido {
				continue
			}
			conflictos := plan[i].conflictos[candidato.EquipoID]
			var ids []string
			for _, c := range conflictos {
				if !vistos[c.ReservaID] {
					ids = append(ids, c.ReservaID)
				}
			}
			if len(ids) == 0 && len(ocupantes) == 0 {
				copia := candidato
				elegido = &copia
				aDesplazar = nil
				reintentar = nil
				break
			}
			if elegido != nil || candidato.PrioridadPropia == 0 {
				continue
			}
			robables, err := repo.ReservasRobablesPor(ctx, *r.MateriaID, ids)
			if err != nil {
				return ImpactoDelRobo{}, nil, err
			}
			reservas, err := repo.BuscarReservasPorIDs(ctx, ids)
			if err != nil {
				return ImpactoDelRobo{}, nil, err
			}
			var desplazables []*domain.Reserva
			for _, id := range ids {
				ajena := reservas[id]
				if ajena == nil || ajena.Estado != domain.ReservaConfirmada || ajena.Tipo != domain.TipoNormal || !sePuedeRobar(robables, id, ajena.Fecha, ajena.HoraInicio, ahora) {
					desplazables = nil
					break
				}
				desplazables = append(desplazables, ajena)
			}
			if len(desplazables) == len(ids) {
				copia := candidato
				elegido = &copia
				aDesplazar = desplazables
				reintentar = ocupantes
			}
		}
		plan[i].equipo = elegido
		if elegido == nil {
			continue
		}
		plan[i].intentados[elegido.EquipoID] = true
		motivoSiguiente := "Un equipo preferente de " + opciones.NombreMateria + " lo necesitaba"
		for _, j := range reintentar {
			plan[j].equipo = nil
			plan[j].motivo = motivoSiguiente
			plan[j].desplazadoPor = r.CreadoPor
			cola = append(cola, j)
		}
		if len(aDesplazar) > 0 {
			ids := make([]string, 0, len(aDesplazar))
			for _, ajena := range aDesplazar {
				ids = append(ids, ajena.ID)
			}
			bloqueadas, err := repo.BuscarReservasParaActualizar(ctx, ids)
			if err != nil {
				return ImpactoDelRobo{}, nil, err
			}
			ordenarReservasParaReposicion(aDesplazar)
			for _, ajena := range aDesplazar {
				actual := bloqueadas[ajena.ID]
				if actual == nil || actual.Estado != domain.ReservaConfirmada || actual.Version != ajena.Version {
					return ImpactoDelRobo{}, nil, ErrReservaModificada
				}
				if !vistos[actual.ID] {
					agregar(actual, motivoSiguiente, r.CreadoPor)
				}
			}
		}
	}

	// Un desplazamiento posterior puede dejar libre parte de una franja que
	// antes estaba ocupada. Se revisan los faltantes una vez más, sólo contra
	// libres, sin iniciar otra cadena ni repetir desplazamientos.
	for i := range plan {
		if plan[i].equipo != nil {
			continue
		}
		r := plan[i].reserva
		for _, candidato := range plan[i].opciones.Equipos {
			if protegidos[candidato.EquipoID] && solapaConPedidoGanador(r, fechasGanadoras, horaInicioGanadora, horaFinGanadora) {
				continue
			}
			ocupantes, permitido := ocupantesDelPlan(plan, candidato, r, ahora)
			if !permitido || len(ocupantes) > 0 {
				continue
			}
			conflictos := plan[i].conflictos[candidato.EquipoID]
			libre := true
			for _, c := range conflictos {
				if !vistos[c.ReservaID] {
					libre = false
					break
				}
			}
			if libre {
				copia := candidato
				plan[i].equipo = &copia
				break
			}
		}
	}

	var impacto ImpactoDelRobo
	var pendientes []cancelacionPendiente
	docentes := make(map[string]bool)
	grupos := make(map[string]bool)
	// El repositorio valida el resultado completo del lote antes de crear la
	// reserva ganadora. No se escriben asignaciones intermedias del plan.
	var escrituras []*domain.Reserva
	for i := len(plan) - 1; i >= 0; i-- {
		p := plan[i]
		r := p.reserva
		pendiente := cancelacionPendiente{reserva: r, motivo: p.motivo, desplazadoPor: p.desplazadoPor, omitirAviso: r.CreadoPor != nil && quien != nil && *r.CreadoPor == *quien}
		if p.equipo != nil && p.equipo.EquipoID == r.EquipoID {
			continue
		}
		if p.equipo != nil {
			r.EquipoID = p.equipo.EquipoID
			pendiente.reposicion = &eventbus.ReservaReubicada{ReservaID: r.ID, EtiquetaAnterior: p.etiquetaOriginal, EtiquetaNueva: p.equipo.Etiqueta, Fecha: r.Fecha}
			impacto.ReservasReubicadas++
		} else {
			if err := r.Cancelar(p.desplazadoPor, p.motivo, ahora); err != nil {
				return ImpactoDelRobo{}, nil, err
			}
			impacto.ReservasCanceladas++
			if r.ReservaGrupoID != nil {
				grupos[*r.ReservaGrupoID] = true
			}
		}
		escrituras = append(escrituras, r)
		pendientes = append(pendientes, pendiente)
		if r.CreadoPor != nil && (p.desplazadoPor == nil || *r.CreadoPor != *p.desplazadoPor) && (quien == nil || *r.CreadoPor != *quien) {
			docentes[*r.CreadoPor] = true
		}
	}
	if err := repo.GuardarReposiciones(ctx, escrituras); err != nil {
		return ImpactoDelRobo{}, nil, err
	}
	ids := make([]string, 0, len(grupos))
	for id := range grupos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := s.actualizarEstadoGrupo(ctx, repo, id); err != nil {
			return ImpactoDelRobo{}, nil, err
		}
	}
	impacto.DocentesAfectados = len(docentes)
	return impacto, pendientes, nil
}

func ordenarReservasParaReposicion(rs []*domain.Reserva) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].Fecha.Equal(rs[j].Fecha) {
			return rs[i].Fecha.Before(rs[j].Fecha)
		}
		if rs[i].HoraInicio != rs[j].HoraInicio {
			return rs[i].HoraInicio < rs[j].HoraInicio
		}
		return rs[i].ID < rs[j].ID
	})
}

func intervaloReposicion(r *domain.Reserva) (time.Time, time.Time) {
	inicio := r.Fecha.Add(r.HoraInicio)
	fin := r.Fecha.Add(r.HoraFin)
	if r.HoraFin <= r.HoraInicio {
		fin = fin.AddDate(0, 0, 1)
	}
	return inicio, fin
}

func solapaConPedidoGanador(r *domain.Reserva, fechas []time.Time, horaInicio, horaFin time.Duration) bool {
	inicio, fin := intervaloReposicion(r)
	for _, fecha := range fechas {
		a, b := intervaloReposicion(&domain.Reserva{Fecha: fecha, HoraInicio: horaInicio, HoraFin: horaFin})
		if inicio.Before(b) && a.Before(fin) {
			return true
		}
	}
	return false
}

func ocupantesDelPlan(plan []reposicionPlanificada, candidato EquipoDisponible, r *domain.Reserva, ahora time.Time) ([]int, bool) {
	inicio, fin := intervaloReposicion(r)
	var ocupantes []int
	for i, p := range plan {
		if p.equipo == nil || p.equipo.EquipoID != candidato.EquipoID {
			continue
		}
		a, b := intervaloReposicion(p.reserva)
		if !inicio.Before(b) || !a.Before(fin) {
			continue
		}
		if candidato.PrioridadPropia == 0 || (p.equipo.PrioridadPropia != 0 && candidato.PrioridadPropia >= p.equipo.PrioridadPropia) || !domain.SePuedeRobarPorHorario(p.reserva.Fecha, p.reserva.HoraInicio, ahora) {
			return nil, false
		}
		ocupantes = append(ocupantes, i)
	}
	return ocupantes, true
}

func (s *Service) publicarReposiciones(ctx context.Context, pendientes []cancelacionPendiente) {
	var orden []destinatario
	grupos := make(map[destinatario][]eventbus.ReservaReubicada)
	for _, p := range pendientes {
		if p.omitirAviso || p.reposicion == nil || p.reserva.CreadoPor == nil || (p.desplazadoPor != nil && *p.desplazadoPor == *p.reserva.CreadoPor) {
			continue
		}
		k := destinatario{usuarioID: *p.reserva.CreadoPor, motivo: p.motivo}
		if _, ok := grupos[k]; !ok {
			orden = append(orden, k)
		}
		grupos[k] = append(grupos[k], *p.reposicion)
	}
	if len(orden) == 0 {
		return
	}
	contactos := s.contactosDeLosAfectados(ctx, orden)
	for _, k := range orden {
		eventbus.Emitir(ctx, s.bus, eventbus.Evento{Tipo: "reserva.reubicada", Payload: eventbus.ReubicacionesDeUsuario{UsuarioID: k.usuarioID, Nombre: contactos[k.usuarioID].Nombre, Email: contactos[k.usuarioID].Email, Motivo: k.motivo, Reservas: grupos[k]}})
	}
}
