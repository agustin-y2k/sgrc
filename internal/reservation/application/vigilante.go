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
	// GraciaDeRetiro: cuánto se espera desde el inicio de la clase antes de
	// liberar una máquina que nadie retiró.
	GraciaDeRetiro time.Duration
	// GraciaTrasEntregaParcial: cuánto se espera desde la última entrega
	// antes de liberar lo que el docente no se llevó.
	GraciaTrasEntregaParcial time.Duration
	// HoraDeCierre (0-23) es el corte de lo que quedó afuera para una
	// institución que NO declaró jornada. Cuando la declaró, el corte sale de
	// ahí: es la hora a la que cierra ese día puntual, más una hora de gracia.
	HoraDeCierre int
}

// PorDefecto usa los valores del dominio. Existe para que los tests y un
// despliegue sin configurar arranquen con lo mismo.
func ConfigDeVigilanciaPorDefecto() ConfigDeVigilancia {
	return ConfigDeVigilancia{
		GraciaDeRetiro:           domain.GraciaDeRetiroPorDefecto,
		GraciaTrasEntregaParcial: domain.GraciaTrasEntregaParcialPorDefecto,
		HoraDeCierre:             23,
	}
}

type Vigilante struct {
	repo Repo
	bus  eventbus.EventBus
	// validadorJornada dice cuándo cierra la escuela cada día. El corte de
	// "qué quedó afuera" sale de ahí y no de una hora fija: con una hora fija,
	// una escuela que cierra a las 22 recibía el aviso a las 18, con las
	// máquinas legítimamente en clase.
	validadorJornada ValidadorJornada
	// validadorMostrador dice si había alguien operando el sistema (RF-07.6).
	// Sin eso, dos de las tres pasadas que quedan estarían leyendo la ausencia
	// de registros como si fuera la ausencia de docentes.
	validadorMostrador ValidadorMostrador
	ahora              func() time.Time
	cfg                ConfigDeVigilancia
}

func NewVigilante(repo Repo, bus eventbus.EventBus, validadorJornada ValidadorJornada, validadorMostrador ValidadorMostrador, ahora func() time.Time, cfg ConfigDeVigilancia) *Vigilante {
	return &Vigilante{
		repo: repo, bus: bus, validadorJornada: validadorJornada,
		validadorMostrador: validadorMostrador, ahora: ahora, cfg: cfg,
	}
}

// ResumenDelBarrido es lo que hizo esta pasada. Se loguea: es la única forma
// de saber que el barrido está vivo sin mirar la base.
type ResumenDelBarrido struct {
	Recordatorios  int
	Liberadas      int
	AvisosDeCierre int
	// MostradorSinAtender: esta pasada encontró el mostrador sin atender y se
	// salteó lo que no podía concluir (RF-07.6). Se loguea porque un barrido
	// que no hace nada porque no hay nadie y uno que no hace nada porque está
	// roto se ven idénticos desde afuera, y esa diferencia importa.
	MostradorSinAtender bool
}

func (r ResumenDelBarrido) HizoAlgo() bool {
	if r.MostradorSinAtender {
		return true
	}
	return r.Recordatorios+r.Liberadas+r.AvisosDeCierre > 0
}

// Barrer corre las tres pasadas.
//
// Dos de ellas concluyen algo a partir de lo que NO está registrado —que nadie
// retiró, qué quedó afuera— y por eso preguntan antes si había alguien
// operando el sistema (RF-07.6). Sin mostrador atendido se saltean enteras: lo
// que el barrido ve en ese caso no es la realidad del laboratorio sino su
// propio silencio. La tercera, el recordatorio, corre siempre porque no
// deduce nada.
func (v *Vigilante) Barrer(ctx context.Context) (ResumenDelBarrido, error) {
	ahora := v.ahora()
	var resumen ResumenDelBarrido

	mostrador, err := v.validadorMostrador.MostradorEn(ctx, ahora)
	if err != nil {
		// Se corta el barrido entero en vez de seguir asumiendo que sí hay
		// alguien: si no se puede saber, lo único que no se puede hacer es
		// liberarle la reserva a un docente por las dudas. La próxima pasada es
		// en cinco minutos y el job registra el fallo en sus métricas.
		return resumen, fmt.Errorf("resolviendo si el mostrador está atendido: %w", err)
	}
	opera := mostrador.Opera()

	reservas, err := v.repo.ReservasAVigilar(ctx, ahora)
	if err != nil {
		return resumen, fmt.Errorf("leyendo las reservas a vigilar: %w", err)
	}

	// El recordatorio corre SIEMPRE: no concluye nada de lo que falta en la
	// base, solo le dice al docente que en un rato tiene clase. Que no haya un
	// Admin en el mostrador no hace que la clase deje de existir.
	resumen.Recordatorios = v.recordar(ctx, reservas, ahora)

	if opera {
		resumen.Liberadas = v.liberarNoRetiradas(ctx, reservas, ahora)
	} else {
		resumen.MostradorSinAtender = true
	}

	prestamos, err := v.repo.PrestamosAVigilar(ctx)
	if err != nil {
		return resumen, fmt.Errorf("leyendo los préstamos a vigilar: %w", err)
	}
	resumen.AvisosDeCierre = v.cortarLaJornada(ctx, prestamos, ahora)

	return resumen, nil
}

// ── 1. El recordatorio, una hora antes ──────────────────────────────────

func (v *Vigilante) recordar(ctx context.Context, reservas []ReservaParaVigilar, ahora time.Time) int {
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

// ── 2. Liberar lo que nadie retiró ──────────────────────────────────────

// liberarNoRetiradas es el corazón de la etapa: cumplido el plazo, una
// máquina que nadie vino a buscar deja de bloquear el horario.
//
// Libera EN SILENCIO, y ahora es la única pasada que toca una reserva sin
// avisar nada a nadie: el aviso de los 15 minutos (RF-08.20) ya no existe. Es
// deliberado —la liberación le devuelve la máquina al resto de la escuela, no
// es un reproche— y por eso mismo depende del mostrador: es lo único que el
// barrido ESCRIBE, y escribirlo sin nadie que haya registrado las entregas es
// quitarle la reserva a un docente que quizás sí vino (RF-07.6).
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

// ── 3. El corte de fin de jornada ───────────────────────────────────────

// entraEnElCorte dice si ese préstamo tiene que aparecer en el corte de esta
// jornada.
//
// Vive en una función porque se consulta en los DOS recorridos de
// cortarLaJornada —el que arma el aviso y el que marca lo avisado—, y si las
// condiciones divergieran se avisaría de una máquina sin marcarla: el mismo
// aviso volvería a salir en el próximo barrido, cada cinco minutos.
func entraEnElCorte(p *domain.Prestamo, cierre time.Time, ahora time.Time) bool {
	// Una sola vez por préstamo, y por eso alcanza con mirar si la marca
	// existe. Antes el aviso se repetía cada día que la máquina siguiera
	// afuera, así que había que comparar contra la fecha; ahora lo que sostiene
	// la insistencia es que la máquina aparece en "qué hay afuera" hasta que
	// alguien la recibe, y eso no se puede tapar con un correo leído.
	if p.AvisadoCierrePara != nil {
		return false
	}
	// La máquina tiene que haber estado afuera ANTES de que cerrara. Una que
	// se entregó después del cierre no "quedó" afuera: recién salió.
	if p.EntregadoEn.After(cierre) {
		return false
	}
	// Una máquina cuya devolución todavía no venció no "quedó afuera": está
	// en uso legítimo. Es el caso de la clase que termina más tarde que el
	// cierre, y sin este filtro el docente de la próxima reserva recibe un
	// "tu computadora puede no estar" que es falso. Sin hora pactada no hay
	// nada que esperar, y ahí el corte sí corresponde: es el único aviso que
	// un préstamo espontáneo va a generar.
	if p.DevolucionEstimada != nil && ahora.Before(*p.DevolucionEstimada) {
		return false
	}
	return true
}

// momentoDelCorte devuelve cuándo cerró la jornada que acaba de terminar, o
// false si todavía no corresponde cortar.
//
// Se miran hoy y ayer, en ese orden. La jornada de ayer puede terminar hoy de
// madrugada —una nocturna que declara el lunes de 20:00 a 01:00 cierra su
// lunes a la 01:00 del martes— y su corte cae una hora después, ya entrado el
// día siguiente. Hoy primero porque, si los dos cortes ya pasaron, el que
// vale es el más reciente.
//
// Un día sin tramos no tiene corte: nadie dejó una máquina afuera de una
// escuela que no abrió. Es lo que hace que un viernes a la noche no se
// repita el sábado ni el domingo.
//
// Sin jornada declarada no hay de dónde deducir nada y se cae a la hora
// configurada, que es el comportamiento que el sistema tenía antes de que la
// jornada existiera.
func (v *Vigilante) momentoDelCorte(ctx context.Context, ahora time.Time) (time.Time, bool) {
	for _, deHaceDias := range []int{0, 1} {
		fecha := ahora.AddDate(0, 0, -deHaceDias)
		cierre, err := v.validadorJornada.CierreDeLaJornada(ctx, fecha)
		if err != nil {
			log.Printf("barrido: no se pudo leer el cierre de la jornada, se usa la hora configurada: %v", err)
			return v.corteConfigurado(ahora)
		}
		if !cierre.Declarada {
			return v.corteConfigurado(ahora)
		}
		if !cierre.Abre {
			continue
		}
		momento := medianocheDe(fecha).Add(cierre.Fin)
		if ahora.Before(momento.Add(GraciaDespuesDelCierre)) {
			continue
		}
		// Un cierre de hace más de un día ya no es "el cierre que acaba de
		// pasar": es historia. Sin este tope, el viernes a las 18 seguiría
		// siendo un corte válido el sábado a la noche, y una escuela que no
		// abre el fin de semana estaría cortando igual — que es exactamente lo
		// que se quiso evitar.
		//
		// Se loguea porque es una pérdida silenciosa: si el barrido estuvo
		// caído más de un día, los cortes de esos días no salen nunca y la
		// única señal sería notar que faltó un correo. Las máquinas siguen
		// visibles en la pantalla de entregas, que es lo que hace que esto sea
		// un aviso perdido y no una máquina perdida.
		if ahora.Sub(momento) >= 24*time.Hour {
			log.Printf("barrido: el corte del %s ya pasó hace más de un día y no se va a avisar "+
				"(¿el barrido estuvo caído?); lo que siga afuera se ve en la pantalla de entregas",
				momento.Format("2006-01-02 15:04"))
			continue
		}
		return momento, true
	}
	return time.Time{}, false
}

// GraciaDespuesDelCierre es cuánto se espera desde que cerró la escuela antes
// de avisar qué quedó afuera. No es configurable: es el rato en que alguien
// que se demoró guardando todavía puede aparecer, y una hora alcanza en
// cualquier institución.
const GraciaDespuesDelCierre = time.Hour

// corteConfigurado es el camino de la institución que eligió no declarar
// jornada: la hora de CIERRE_JORNADA, del día de hoy.
func (v *Vigilante) corteConfigurado(ahora time.Time) (time.Time, bool) {
	if ahora.Hour() < v.cfg.HoraDeCierre {
		return time.Time{}, false
	}
	return medianocheDe(ahora).Add(time.Duration(v.cfg.HoraDeCierre) * time.Hour), true
}

func medianocheDe(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// cortarLaJornada avisa qué máquinas quedaron afuera al terminar el día.
func (v *Vigilante) cortarLaJornada(ctx context.Context, prestamos []PrestamoParaVigilar, ahora time.Time) int {
	cierre, hayCorte := v.momentoDelCorte(ctx, ahora)
	if !hayCorte {
		return 0
	}

	// La pregunta acá NO es la del resto del barrido. El corte sale una hora
	// después de que cerró la escuela, cuando el Admin ya se fue a su casa:
	// preguntar por este instante daría que no hay nadie siempre, y el corte
	// no saldría nunca. Lo que importa es si hubo alguien operando durante la
	// jornada que se está cerrando, porque de eso depende que la lista de
	// "qué quedó afuera" tenga algo que ver con la realidad (RF-07.6).
	mostrador, err := v.validadorMostrador.MostradorEseDia(ctx, cierre)
	if err != nil {
		log.Printf("barrido: no se pudo resolver si hubo alguien en el mostrador el %s, "+
			"no se corta la jornada: %v", cierre.Format("2006-01-02"), err)
		return 0
	}
	if !mostrador.Opera() {
		// Sin marcar nada: si nadie atendió, tampoco hay de qué avisar, y
		// marcar los préstamos como avisados perdería el corte del día que
		// alguien sí atienda. Lo que esté afuera se sigue viendo en la
		// pantalla de entregas.
		return 0
	}

	var afuera []eventbus.EquipoSinDevolverAlCierre
	for _, p := range prestamos {
		if !entraEnElCorte(p.Prestamo, cierre, ahora) {
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
		if !entraEnElCorte(p.Prestamo, cierre, ahora) {
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
