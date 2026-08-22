package application

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// El barrido de reservas y entregas (RF-08.10 a RF-08.13).

// ConfigDeVigilancia son los plazos que la escuela puede ajustar.
type ConfigDeVigilancia struct {
	// DemoraDelAvisoDeNoRetiro: cuánto se espera desde el inicio de la clase
	// antes de avisarle al docente que todavía no las retiró (RF-08.20).
	DemoraDelAvisoDeNoRetiro time.Duration
	// GraciaDeRetiro: cuánto se espera desde el inicio de la clase antes de
	// liberar una máquina que nadie retiró.
	GraciaDeRetiro time.Duration
	// GraciaTrasEntregaParcial: cuánto se espera desde la última entrega
	// antes de liberar lo que el docente no se llevó.
	GraciaTrasEntregaParcial time.Duration
	// DemoraParaReclamar: cuánto después de la hora de devolución se le
	// reclama a quien tiene la máquina.
	DemoraParaReclamar time.Duration
	// HoraDeCierre (0-23): a partir de qué hora se hace el corte de lo que
	// quedó afuera.
	HoraDeCierre int
}

// PorDefecto usa los valores del dominio. Existe para que los tests y un
// despliegue sin configurar arranquen con lo mismo.
func ConfigDeVigilanciaPorDefecto() ConfigDeVigilancia {
	return ConfigDeVigilancia{
		DemoraDelAvisoDeNoRetiro: domain.DemoraDelAvisoDeNoRetiroPorDefecto,
		GraciaDeRetiro:           domain.GraciaDeRetiroPorDefecto,
		GraciaTrasEntregaParcial: domain.GraciaTrasEntregaParcialPorDefecto,
		DemoraParaReclamar:       domain.DemoraParaReclamarPorDefecto,
		HoraDeCierre:             18,
	}
}

type Vigilante struct {
	repo  Repo
	bus   eventbus.EventBus
	ahora func() time.Time
	cfg   ConfigDeVigilancia
}

func NewVigilante(repo Repo, bus eventbus.EventBus, ahora func() time.Time, cfg ConfigDeVigilancia) *Vigilante {
	return &Vigilante{repo: repo, bus: bus, ahora: ahora, cfg: cfg}
}

// ResumenDelBarrido es lo que hizo esta pasada. Se loguea: es la única forma
// de saber que el barrido está vivo sin mirar la base.
type ResumenDelBarrido struct {
	Recordatorios          int
	AvisosDeNoRetiro       int
	Liberadas              int
	AvisosDeEquipoFaltante int
	Reclamos               int
	AvisosDeCierre         int
}

func (r ResumenDelBarrido) HizoAlgo() bool {
	return r.Recordatorios+r.AvisosDeNoRetiro+r.Liberadas+r.AvisosDeEquipoFaltante+
		r.Reclamos+r.AvisosDeCierre > 0
}

// Barrer corre las seis pasadas.
func (v *Vigilante) Barrer(ctx context.Context) (ResumenDelBarrido, error) {
	ahora := v.ahora()
	var resumen ResumenDelBarrido

	reservas, err := v.repo.ReservasAVigilar(ctx, ahora)
	if err != nil {
		return resumen, fmt.Errorf("leyendo las reservas a vigilar: %w", err)
	}

	// Marcas en memoria: lo que se avisó en esta misma pasada no se vuelve a
	// avisar más abajo, aunque la base todavía no lo refleje.
	avisadas := map[string]bool{}

	resumen.Recordatorios = v.recordar(ctx, reservas, ahora, avisadas)
	resumen.AvisosDeEquipoFaltante = v.avisarEquiposQueNoVolvieron(ctx, reservas, ahora, avisadas)
	resumen.AvisosDeNoRetiro = v.avisarNoRetiro(ctx, reservas, ahora)
	resumen.Liberadas = v.liberarNoRetiradas(ctx, reservas, ahora)

	prestamos, err := v.repo.PrestamosAVigilar(ctx)
	if err != nil {
		return resumen, fmt.Errorf("leyendo los préstamos a vigilar: %w", err)
	}
	resumen.Reclamos = v.reclamarDevoluciones(ctx, prestamos, ahora)
	resumen.AvisosDeCierre = v.cortarLaJornada(ctx, prestamos, ahora)

	return resumen, nil
}

// ── 1. El recordatorio, una hora antes ──────────────────────────────────

func (v *Vigilante) recordar(ctx context.Context, reservas []ReservaParaVigilar, ahora time.Time, avisadas map[string]bool) int {
	enviados := 0

	for _, grupo := range agruparPorGrupo(reservas) {
		primera := grupo[0]
		if primera.RecordatorioEnviado || primera.GrupoID == nil || !esDeUnDocente(primera) {
			continue
		}
		if !domain.CorrespondeRecordar(primera.Fecha, primera.HoraInicio, primera.HoraFin,
			domain.AntelacionDelRecordatorio, ahora) {
			continue
		}

		aviso := eventbus.RecordatorioDeReserva{
			Email:           primera.DocenteEmail,
			Nombre:          primera.DocenteNombre,
			MateriaNombre:   nombreODefecto(primera.MateriaNombre),
			Fecha:           primera.Fecha,
			HoraInicio:      primera.HoraInicio,
			MinutosDeGracia: int(v.cfg.GraciaDeRetiro.Minutes()),
		}
		if primera.DocenteID != nil {
			aviso.UsuarioID = *primera.DocenteID
		}

		for _, r := range grupo {
			aviso.Equipos = append(aviso.Equipos, r.Etiqueta)
			// La advertencia de la máquina que no volvió viaja DENTRO del
			// recordatorio: si el docente igual va a recibir un correo por esta clase,
			// mandarle dos es el bombardeo que se quiso evitar.
			if v.pcDemorada(r, ahora) && !r.AvisoEquipoNoDisponibleEnviado {
				aviso.EquiposSinDevolver = append(aviso.EquiposSinDevolver, r.Etiqueta)
				v.marcarAvisoDeEquipo(ctx, r.ReservaID, ahora, avisadas)
			}
		}

		v.bus.Publish(eventbus.Evento{Tipo: "reserva.recordatorio", Payload: aviso})
		if err := v.repo.MarcarRecordatorioEnviado(ctx, *primera.GrupoID, ahora); err != nil {
			// El recordatorio ya salió: no marcarlo solo significa que puede
			// repetirse en la próxima pasada, no que se haya perdido.
			log.Printf("barrido: no se pudo marcar el recordatorio del grupo %s (el aviso ya salió): %v",
				*primera.GrupoID, err)
		}
		enviados++
	}

	return enviados
}

// ── 2. El aviso suelto de "tu PC no volvió" ─────────────────────────────

// avisarEquiposQueNoVolvieron cubre la otra mitad de max(detección, inicio −
// 1 h): la demora se detectó DESPUÉS de que el recordatorio ya había salido,
// o falta menos de una hora para la clase.
func (v *Vigilante) avisarEquiposQueNoVolvieron(ctx context.Context, reservas []ReservaParaVigilar, ahora time.Time, avisadas map[string]bool) int {
	enviados := 0

	for _, grupo := range agruparPorGrupo(reservas) {
		primera := grupo[0]
		if !esDeUnDocente(primera) {
			continue
		}
		if !domain.CorrespondeAvisarEquipoNoDisponible(primera.Fecha, primera.HoraInicio, primera.HoraFin,
			domain.AntelacionDelRecordatorio, ahora) {
			continue
		}

		var faltantes []string
		var reservaIDs []string
		for _, r := range grupo {
			if r.AvisoEquipoNoDisponibleEnviado || avisadas[r.ReservaID] {
				continue
			}
			if v.pcDemorada(r, ahora) {
				faltantes = append(faltantes, r.Etiqueta)
				reservaIDs = append(reservaIDs, r.ReservaID)
			}
		}
		if len(faltantes) == 0 {
			continue
		}

		aviso := eventbus.EquipoNoDisponibleParaReserva{
			Email:         primera.DocenteEmail,
			Nombre:        primera.DocenteNombre,
			MateriaNombre: nombreODefecto(primera.MateriaNombre),
			Fecha:         primera.Fecha,
			HoraInicio:    primera.HoraInicio,
			Equipos:       faltantes,
		}
		if primera.DocenteID != nil {
			aviso.UsuarioID = *primera.DocenteID
		}

		v.bus.Publish(eventbus.Evento{Tipo: "reserva.equipo-no-disponible", Payload: aviso})
		for _, id := range reservaIDs {
			v.marcarAvisoDeEquipo(ctx, id, ahora, avisadas)
		}
		enviados++
	}

	return enviados
}

// ── 3. El aviso de "todavía no las retiraste" ───────────────────────────

// avisarNoRetiro es el único aviso de esta parte del barrido (RF-08.20).
func (v *Vigilante) avisarNoRetiro(ctx context.Context, reservas []ReservaParaVigilar, ahora time.Time) int {
	enviados := 0

	for _, grupo := range agruparPorGrupo(reservas) {
		primera := grupo[0]
		if primera.AvisoSinRetirarEnviado || primera.GrupoID == nil || !esDeUnDocente(primera) {
			continue
		}
		// Vino a buscar aunque sea una: no hay nada que avisarle. Lo que dejó
		// se libera por el plazo corto, y de eso ya se enteró en el mostrador.
		if primera.UltimaEntregaDelGrupo != nil {
			continue
		}
		if !domain.CorrespondeAvisarNoRetiro(primera.Fecha, primera.HoraInicio, primera.HoraFin,
			v.cfg.DemoraDelAvisoDeNoRetiro, v.cfg.GraciaDeRetiro, ahora) {
			continue
		}
		// Si en esta misma pasada la reserva ya se va a liberar —el proceso estuvo
		// caído y volvió pasada la gracia— el aviso llegaría anunciando algo que
		// acaba de pasar.
		if v.correspondeLiberar(primera, ahora) {
			continue
		}

		aviso := eventbus.ReservaSinRetirar{
			Email:           primera.DocenteEmail,
			Nombre:          primera.DocenteNombre,
			MateriaNombre:   nombreODefecto(primera.MateriaNombre),
			Fecha:           primera.Fecha,
			HoraInicio:      primera.HoraInicio,
			MinutosDeGracia: int(v.cfg.GraciaDeRetiro.Minutes()),
		}
		if primera.DocenteID != nil {
			aviso.UsuarioID = *primera.DocenteID
		}
		for _, r := range grupo {
			aviso.Equipos = append(aviso.Equipos, r.Etiqueta)
		}

		v.bus.Publish(eventbus.Evento{Tipo: "reserva.sin-retirar", Payload: aviso})
		if err := v.repo.MarcarAvisoSinRetirarEnviado(ctx, *primera.GrupoID, ahora); err != nil {
			// Mismo criterio que el recordatorio: el aviso ya salió, y no marcarlo
			// solo significa que puede repetirse en la próxima pasada, no que se haya
			// perdido.
			log.Printf("barrido: no se pudo marcar el aviso de no retiro del grupo %s (el aviso ya salió): %v",
				*primera.GrupoID, err)
		}
		enviados++
	}

	return enviados
}

// ── 4. Liberar lo que nadie retiró ──────────────────────────────────────

// liberarNoRetiradas es el corazón de la etapa: cumplido el plazo, una
// máquina que nadie vino a buscar deja de bloquear el horario.
func (v *Vigilante) liberarNoRetiradas(ctx context.Context, reservas []ReservaParaVigilar, ahora time.Time) int {
	liberadas := 0

	for _, grupo := range agruparPorGrupo(reservas) {
		primera := grupo[0]
		if !esDeUnDocente(primera) {
			continue
		}
		if !v.correspondeLiberar(primera, ahora) {
			continue
		}

		algunaLiberada := false
		for _, r := range grupo {
			if r.EquipoAfuera {
				continue
			}
			if err := v.liberar(ctx, r.ReservaID, ahora); err != nil {
				log.Printf("barrido: no se pudo liberar la reserva %s: %v", r.ReservaID, err)
				continue
			}
			algunaLiberada = true
			liberadas++
		}
		if !algunaLiberada {
			continue
		}

		if primera.GrupoID != nil {
			v.marcarGrupoSiQuedoTodoSinRetirar(ctx, *primera.GrupoID)
		}
	}

	return liberadas
}

// correspondeLiberar elige cuál de los dos plazos le corre a esta reserva.
func (v *Vigilante) correspondeLiberar(primera ReservaParaVigilar, ahora time.Time) bool {
	if primera.UltimaEntregaDelGrupo != nil {
		return domain.CorrespondeLiberarTrasEntregaParcial(primera.Fecha, primera.HoraInicio, primera.HoraFin,
			*primera.UltimaEntregaDelGrupo, v.cfg.GraciaTrasEntregaParcial, ahora)
	}
	return domain.CorrespondeLiberar(primera.Fecha, primera.HoraInicio, primera.HoraFin,
		v.cfg.GraciaDeRetiro, ahora)
}

func (v *Vigilante) liberar(ctx context.Context, reservaID string, ahora time.Time) error {
	reserva, err := v.repo.BuscarReservaPorID(ctx, reservaID)
	if err != nil {
		return err
	}
	// Releer antes de escribir: entre la consulta del barrido y este momento el
	// docente pudo haber cancelado, o el Admin haber entregado la máquina.
	if reserva.Estado != domain.ReservaConfirmada {
		return nil
	}
	if err := reserva.Liberar(); err != nil {
		return err
	}
	return v.repo.GuardarReserva(ctx, reserva)
}

// marcarGrupoSiQuedoTodoSinRetirar sigue el mismo criterio que
// finalizarGrupoSiCorresponde: el grupo solo cambia cuando ya no queda
// ninguna reserva viva adentro.
func (v *Vigilante) marcarGrupoSiQuedoTodoSinRetirar(ctx context.Context, grupoID string) {
	grupo, err := v.repo.BuscarReservaGrupoPorID(ctx, grupoID)
	if err != nil {
		log.Printf("barrido: no se pudo leer el grupo %s: %v", grupoID, err)
		return
	}
	if grupo.Estado != domain.GrupoConfirmada {
		return
	}

	reservas, err := v.repo.ListarReservasPorGrupo(ctx, grupoID)
	if err != nil {
		log.Printf("barrido: no se pudieron leer las reservas del grupo %s: %v", grupoID, err)
		return
	}
	hayNoRetiradas := false
	for _, r := range reservas {
		if r.Estado == domain.ReservaConfirmada {
			return // todavía queda alguna viva
		}
		if r.Estado == domain.ReservaNoRetirada {
			hayNoRetiradas = true
		}
	}
	if !hayNoRetiradas {
		return
	}

	if err := grupo.CambiarEstado(domain.GrupoNoRetirado); err != nil {
		log.Printf("barrido: no se pudo marcar el grupo %s como no retirado: %v", grupoID, err)
		return
	}
	if err := v.repo.GuardarReservaGrupo(ctx, grupo); err != nil {
		log.Printf("barrido: no se pudo guardar el grupo %s: %v", grupoID, err)
	}
}

// ── 4. Reclamar lo que no volvió ────────────────────────────────────────

func (v *Vigilante) reclamarDevoluciones(ctx context.Context, prestamos []PrestamoParaVigilar, ahora time.Time) int {
	var demorados []eventbus.PrestamoDemorado

	for _, p := range prestamos {
		if p.Prestamo.AvisadoDemoraEn != nil {
			continue
		}
		if !p.Prestamo.ExcedioLaDemora(v.cfg.DemoraParaReclamar, ahora) {
			continue
		}
		// Las dos horas se convierten ACÁ, a la zona que trae `ahora` (que es la de
		// la escuela, ver cmd/main.go).
		demorados = append(demorados, eventbus.PrestamoDemorado{
			PrestamoID:      p.Prestamo.ID,
			Etiqueta:        p.Etiqueta,
			CarroNombre:     p.CarroNombre,
			Quien:           p.Prestamo.EntregadoANombre,
			Email:           p.Email,
			EntregadoEn:     p.Prestamo.EntregadoEn.In(ahora.Location()),
			DebioVolverA:    p.Prestamo.DevolucionEstimada.In(ahora.Location()),
			MinutosDeDemora: p.Prestamo.MinutosDeDemora(ahora),
		})
	}
	if len(demorados) == 0 {
		return 0
	}

	// Publicar primero y marcar después, igual que el aviso de licencias: si el
	// proceso se cae en el medio, un reclamo repetido molesta; uno que no sale
	// deja una máquina perdida sin que nadie se entere.
	v.bus.Publish(eventbus.Evento{Tipo: "prestamo.demorado", Payload: eventbus.PrestamosDemorados{Prestamos: demorados}})
	for _, d := range demorados {
		if err := v.repo.MarcarDemoraAvisada(ctx, d.PrestamoID, ahora); err != nil {
			log.Printf("barrido: no se pudo marcar el reclamo del préstamo %s (ya salió): %v", d.PrestamoID, err)
		}
	}
	return len(demorados)
}

// ── 5. El corte de fin de jornada ───────────────────────────────────────

// entraEnElCorte dice si ese préstamo tiene que aparecer en el corte de esta
// jornada.
//
// Vive en una función porque se consulta en los DOS recorridos de
// cortarLaJornada —el que arma el aviso y el que marca lo avisado—, y si las
// condiciones divergieran se avisaría de una máquina sin marcarla: el mismo
// aviso volvería a salir en el próximo barrido, cada cinco minutos.
func entraEnElCorte(p *domain.Prestamo, hoy string, ahora time.Time) bool {
	if p.AvisadoCierrePara != nil && p.AvisadoCierrePara.Format("2006-01-02") == hoy {
		return false
	}
	// Una máquina cuya devolución todavía no venció no "quedó afuera": está
	// en uso legítimo. Es el caso de la clase que termina más tarde que el
	// cierre, y sin este filtro el docente de la próxima reserva recibe un
	// "tu computadora puede no estar" que es falso. Sin hora pactada no hay
	// nada que esperar, y ahí el corte sí corresponde: es el único aviso que
	// un préstamo espontáneo va a generar (ExcedioLaDemora no lo reclama).
	if p.DevolucionEstimada != nil && ahora.Before(*p.DevolucionEstimada) {
		return false
	}
	return true
}

// cortarLaJornada avisa qué máquinas quedaron afuera al terminar el día.
func (v *Vigilante) cortarLaJornada(ctx context.Context, prestamos []PrestamoParaVigilar, ahora time.Time) int {
	if ahora.Hour() < v.cfg.HoraDeCierre {
		return 0
	}
	hoy := ahora.Format("2006-01-02")

	var afuera []eventbus.EquipoSinDevolverAlCierre
	for _, p := range prestamos {
		if !entraEnElCorte(p.Prestamo, hoy, ahora) {
			continue
		}
		pc := eventbus.EquipoSinDevolverAlCierre{
			Etiqueta:    p.Etiqueta,
			CarroNombre: p.CarroNombre,
			Quien:       p.Prestamo.EntregadoANombre,
			// En la zona de la escuela, por lo mismo que en reclamarDevoluciones.
			DesdeCuando: p.Prestamo.EntregadoEn.In(ahora.Location()),
		}
		v.completarProximaReserva(ctx, p.Prestamo.EquipoID, ahora, &pc)
		afuera = append(afuera, pc)
	}
	if len(afuera) == 0 {
		return 0
	}

	v.bus.Publish(eventbus.Evento{Tipo: "prestamo.sin-devolver.cierre", Payload: eventbus.EquiposSinDevolverAlCierre{Equipos: afuera}})
	for _, p := range prestamos {
		if !entraEnElCorte(p.Prestamo, hoy, ahora) {
			continue
		}
		if err := v.repo.MarcarCierreAvisado(ctx, p.Prestamo.ID, ahora); err != nil {
			log.Printf("barrido: no se pudo marcar el cierre del préstamo %s (ya salió): %v", p.Prestamo.ID, err)
		}
	}
	return len(afuera)
}

// completarProximaReserva busca a quién le va a faltar esa máquina.
func (v *Vigilante) completarProximaReserva(ctx context.Context, equipoID string, ahora time.Time, destino *eventbus.EquipoSinDevolverAlCierre) {
	proxima, err := v.repo.ProximaReservaDeEquipo(ctx, equipoID, ahora)
	if err != nil {
		log.Printf("barrido: no se pudo resolver la próxima reserva del equipo %s: %v", equipoID, err)
		return
	}
	if proxima == nil {
		return
	}
	destino.ProximoUsuarioID = proxima.UsuarioID
	destino.ProximoEmail = proxima.Email
	destino.ProximoNombre = proxima.Nombre
	destino.ProximaFecha = proxima.Fecha
	destino.ProximaHora = proxima.HoraInicio
}

// ── Auxiliares ──────────────────────────────────────────────────────────

// pcDemorada: la máquina de esa reserva está afuera y pasada de hora.
func (v *Vigilante) pcDemorada(r ReservaParaVigilar, ahora time.Time) bool {
	return r.EquipoAfuera && r.EquipoDebioVolverA != nil && ahora.After(*r.EquipoDebioVolverA)
}

func (v *Vigilante) marcarAvisoDeEquipo(ctx context.Context, reservaID string, ahora time.Time, avisadas map[string]bool) {
	avisadas[reservaID] = true
	if err := v.repo.MarcarAvisoEquipoNoDisponible(ctx, reservaID, ahora); err != nil {
		log.Printf("barrido: no se pudo marcar el aviso de equipo faltante de la reserva %s (ya salió): %v",
			reservaID, err)
	}
}

// agruparPorGrupo junta las reservas de una misma clase, conservando el orden
// en que vinieron.
func agruparPorGrupo(reservas []ReservaParaVigilar) [][]ReservaParaVigilar {
	var orden []string
	porGrupo := map[string][]ReservaParaVigilar{}

	for _, r := range reservas {
		clave := r.ReservaID
		if r.GrupoID != nil {
			clave = *r.GrupoID
		}
		if _, visto := porGrupo[clave]; !visto {
			orden = append(orden, clave)
		}
		porGrupo[clave] = append(porGrupo[clave], r)
	}

	grupos := make([][]ReservaParaVigilar, 0, len(orden))
	for _, clave := range orden {
		grupos = append(grupos, porGrupo[clave])
	}
	return grupos
}

// esDeUnDocente distingue la clase de alguien de un bloqueo administrativo
// estatal (RF-04.7).
func esDeUnDocente(r ReservaParaVigilar) bool {
	return r.Tipo == domain.TipoNormal
}

// nombreODefecto es una guarda contra nil, no una rama de negocio: las tres
// pasadas que la usan ya descartaron los bloqueos, y una reserva normal
// siempre tiene materia (lo exige un CHECK).
func nombreODefecto(nombre *string) string {
	if nombre == nil || *nombre == "" {
		return "una clase"
	}
	return *nombre
}
