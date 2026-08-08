package application

import (
	"context"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// busEspia guarda lo publicado. Lo que estos tests miran casi siempre es
// CUÁNTOS eventos salieron: un aviso por clase y no por máquina, y uno solo
// aunque el barrido corra veinte veces.
type busEspia struct {
	publicados []eventbus.Evento
}

func (b *busEspia) Publish(e eventbus.Evento)                      { b.publicados = append(b.publicados, e) }
func (b *busEspia) Subscribe(tipo string, h func(eventbus.Evento)) {}

func (b *busEspia) de(tipo string) []eventbus.Evento {
	var r []eventbus.Evento
	for _, e := range b.publicados {
		if e.Tipo == tipo {
			r = append(r, e)
		}
	}
	return r
}

// El escenario: la clase del 10 de agosto de 2026, de 8 a 9, de Ada, con dos
// máquinas. El reloj se mueve test por test.
const (
	grupoDeClase = "grupo-clase"
	docenteAda   = "docente-ada"
)

func aLas(hora, minuto int) time.Time {
	return time.Date(2026, time.August, 10, hora, minuto, 0, 0, time.UTC)
}

func repoConClase(t *testing.T, pcs ...string) *fakeRepo {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.contactoDeUsuario[docenteAda] = [2]string{"Ada Lovelace", "ada@escuela.edu.ar"}

	creadoPor := docenteAda
	grupo, err := domain.NuevoReservaGrupo(grupoDeClase, "materia1", &creadoPor, "Ada Lovelace",
		aLas(0, 0), 8*time.Hour, 9*time.Hour, nil, aLas(0, 0).AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.grupos[grupo.ID] = grupo

	for i, pcID := range pcs {
		repo.identificadorDePC[pcID] = i + 1
		r, err := domain.NuevaReservaNormal("res-"+pcID, grupoDeClase, pcID, "materia1",
			"Ada Lovelace", &creadoPor, aLas(0, 0), 8*time.Hour, 9*time.Hour,
			aLas(0, 0).AddDate(0, 0, -1))
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		repo.reservas[r.ID] = r
	}
	return repo
}

func vigilanteALas(repo Repo, bus eventbus.EventBus, hora, minuto int) *Vigilante {
	return NewVigilante(repo, bus, func() time.Time { return aLas(hora, minuto) },
		ConfigDeVigilanciaPorDefecto())
}

func barrer(t *testing.T, v *Vigilante) ResumenDelBarrido {
	t.Helper()
	resumen, err := v.Barrer(context.Background())
	if err != nil {
		t.Fatalf("el barrido no debería fallar: %v", err)
	}
	return resumen
}

// ── El recordatorio ─────────────────────────────────────────────────────

func TestBarrer_RecordatorioUnaHoraAntes(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	bus := &busEspia{}

	// A las 6:30 todavía no.
	if r := barrer(t, vigilanteALas(repo, bus, 6, 30)); r.Recordatorios != 0 {
		t.Fatalf("dos horas antes no corresponde recordar: %+v", r)
	}
	// A las 7:00 sí.
	if r := barrer(t, vigilanteALas(repo, bus, 7, 0)); r.Recordatorios != 1 {
		t.Fatalf("una hora antes sí: %+v", r)
	}

	eventos := bus.de("reserva.recordatorio")
	if len(eventos) != 1 {
		t.Fatalf("esperaba 1 recordatorio, hubo %d", len(eventos))
	}
	aviso := eventos[0].Payload.(eventbus.RecordatorioDeReserva)
	// UN aviso por clase con las dos máquinas adentro, no uno por máquina.
	if len(aviso.Equipos) != 2 {
		t.Errorf("el recordatorio tiene que traer las dos PCs: %+v", aviso.Equipos)
	}
	if aviso.Email != "ada@escuela.edu.ar" || aviso.UsuarioID != docenteAda {
		t.Errorf("falta el contacto del docente: %+v", aviso)
	}
	// El plazo viaja en el payload para que el texto no tenga que conocer
	// la configuración del despliegue.
	if aviso.MinutosDeGracia != 40 {
		t.Errorf("minutosDeGracia = %d, esperaba 40", aviso.MinutosDeGracia)
	}
}

func TestBarrer_ElRecordatorioSaleUnaSolaVez(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}
	v := vigilanteALas(repo, bus, 7, 0)

	barrer(t, v)
	for i := 0; i < 10; i++ {
		if r := barrer(t, v); r.Recordatorios != 0 {
			t.Fatalf("corrida %d: volvió a recordar", i+2)
		}
	}
	if len(bus.de("reserva.recordatorio")) != 1 {
		t.Errorf("salieron %d recordatorios, esperaba 1", len(bus.de("reserva.recordatorio")))
	}
}

// TestBarrer_RecordatorioTardioSaleIgual: si el proceso estuvo caído, el
// recordatorio sale tarde en vez de perderse. A las 8:10 todavía sirve saber
// que la reserva se libera a las 8:40.
func TestBarrer_RecordatorioTardioSaleIgual(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 10)); r.Recordatorios != 1 {
		t.Errorf("el recordatorio tardío tiene que salir igual: %+v", r)
	}
}

// ── La liberación ───────────────────────────────────────────────────────

func TestBarrer_LiberaALos40Minutos(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 30)); r.Liberadas != 0 {
		t.Fatalf("a los 30 minutos todavía no: %+v", r)
	}

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 40))

	if resumen.Liberadas != 2 {
		t.Fatalf("esperaba 2 liberadas, obtuve %d", resumen.Liberadas)
	}
	for _, id := range []string{"res-pc1", "res-pc2"} {
		if repo.reservas[id].Estado != domain.ReservaNoRetirada {
			t.Errorf("%s quedó en %s, esperaba NO_RETIRADA", id, repo.reservas[id].Estado)
		}
	}
	// Ninguna se retiró: el grupo entero queda no retirado.
	if repo.grupos[grupoDeClase].Estado != domain.GrupoNoRetirado {
		t.Errorf("el grupo quedó en %s", repo.grupos[grupoDeClase].Estado)
	}

	eventos := bus.de("reserva.no-retirada")
	if len(eventos) != 1 {
		t.Fatalf("esperaba 1 aviso, hubo %d", len(eventos))
	}
	aviso := eventos[0].Payload.(eventbus.ReservasLiberadas)
	if !aviso.TodaLaReserva || len(aviso.Equipos) != 2 {
		t.Errorf("el aviso tiene que decir que no retiró nada: %+v", aviso)
	}
}

// TestBarrer_NoLiberaLaQueSeLlevaron es la condición que separa "el docente
// no vino" de "el docente vino": si la máquina está afuera, la reserva está
// cumplida aunque nadie haya apretado nada más.
func TestBarrer_NoLiberaLaQueSeLlevaron(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	// Se llevó la primera.
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{PCID: "pc1", Nombre: "Ada"}, aLas(8, 5))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 40))

	if resumen.Liberadas != 1 {
		t.Fatalf("esperaba que solo se liberara la que no retiró: %d", resumen.Liberadas)
	}
	if repo.reservas["res-pc1"].Estado != domain.ReservaConfirmada {
		t.Error("la que se llevó no se libera")
	}
	if repo.reservas["res-pc2"].Estado != domain.ReservaNoRetirada {
		t.Error("la que no retiró sí se libera")
	}
	// Vino a dar la clase: el grupo NO pasa a NO_RETIRADA.
	if repo.grupos[grupoDeClase].Estado != domain.GrupoConfirmada {
		t.Errorf("el grupo quedó en %s, esperaba CONFIRMADA", repo.grupos[grupoDeClase].Estado)
	}
	aviso := bus.de("reserva.no-retirada")[0].Payload.(eventbus.ReservasLiberadas)
	if aviso.TodaLaReserva {
		t.Error("retiró una: el aviso no puede decir que no retiró nada")
	}
}

func TestBarrer_NoLiberaUnaClaseYaTerminada(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 9, 30)); r.Liberadas != 0 {
		t.Errorf("una clase terminada no se libera: %+v", r)
	}
}

func TestBarrer_LiberarEsIdempotente(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}
	v := vigilanteALas(repo, bus, 8, 45)

	barrer(t, v)
	for i := 0; i < 5; i++ {
		if r := barrer(t, v); r.Liberadas != 0 {
			t.Fatalf("corrida %d: volvió a liberar", i+2)
		}
	}
	if len(bus.de("reserva.no-retirada")) != 1 {
		t.Errorf("salieron %d avisos, esperaba 1", len(bus.de("reserva.no-retirada")))
	}
}

// ── La PC que no volvió ─────────────────────────────────────────────────

// prestamoVencido deja una máquina afuera que tenía que haber vuelto.
func prestamoVencido(t *testing.T, repo *fakeRepo, id, pcID string, debioVolverA time.Time) *domain.Prestamo {
	t.Helper()
	usuario := "otro-docente"
	p, err := domain.NuevoPrestamo(id, domain.DatosDeEntrega{
		PCID: pcID, Nombre: "Otro Docente", UsuarioID: &usuario,
		DevolucionEstimada: &debioVolverA,
	}, debioVolverA.Add(-time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	repo.contactoDeUsuario[usuario] = [2]string{"Otro Docente", "otro@escuela.edu.ar"}
	return p
}

// TestBarrer_LaAdvertenciaViajaDentroDelRecordatorio: si el docente igual va
// a recibir un correo por esta clase, mandarle dos es el bombardeo que se
// quiso evitar.
func TestBarrer_LaAdvertenciaViajaDentroDelRecordatorio(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	prestamoVencido(t, repo, "pr1", "pc1", aLas(6, 30))
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 7, 0))

	if resumen.Recordatorios != 1 {
		t.Fatalf("esperaba el recordatorio: %+v", resumen)
	}
	if resumen.AvisosDePCFaltante != 0 {
		t.Errorf("la advertencia va adentro del recordatorio, no aparte: %+v", resumen)
	}
	aviso := bus.de("reserva.recordatorio")[0].Payload.(eventbus.RecordatorioDeReserva)
	if len(aviso.EquiposSinDevolver) != 1 || aviso.EquiposSinDevolver[0] != "PC 1" {
		t.Errorf("el recordatorio tiene que advertir de la PC 1: %+v", aviso.EquiposSinDevolver)
	}
	if len(bus.de("reserva.pc-no-disponible")) != 0 {
		t.Error("no tiene que salir un segundo correo")
	}
}

// TestBarrer_AvisoSueltoCuandoLaDemoraEsPosterior es la otra mitad de
// max(detección, inicio − 1 h): el recordatorio ya salió y recién después la
// máquina se pasó de hora.
func TestBarrer_AvisoSueltoCuandoLaDemoraEsPosterior(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	barrer(t, vigilanteALas(repo, bus, 7, 0)) // el recordatorio sale sin advertencia
	prestamoVencido(t, repo, "pr1", "pc1", aLas(7, 30))

	resumen := barrer(t, vigilanteALas(repo, bus, 7, 40))

	if resumen.AvisosDePCFaltante != 1 {
		t.Fatalf("esperaba el aviso suelto: %+v", resumen)
	}
	aviso := bus.de("reserva.pc-no-disponible")[0].Payload.(eventbus.PCNoDisponibleParaReserva)
	if len(aviso.Equipos) != 1 {
		t.Errorf("el aviso tiene que nombrar el equipo: %+v", aviso)
	}
}

// TestBarrer_SiLaPCVuelveATiempoNadieSeEntera es la razón por la que esto no
// bombardea: el caso más común es que alguien se demore quince minutos y
// devuelva.
func TestBarrer_SiLaPCVuelveATiempoNadieSeEntera(t *testing.T) {
	repo := repoConClase(t, "pc1")
	// La clase es a las 8; la máquina la tenía otro y debía volver a las
	// 7:30, o sea que a las 7:00 —cuando sale el recordatorio— todavía no
	// estaba demorada.
	p := prestamoVencido(t, repo, "pr1", "pc1", aLas(7, 30))
	bus := &busEspia{}

	barrer(t, vigilanteALas(repo, bus, 7, 0))
	aviso := bus.de("reserva.recordatorio")[0].Payload.(eventbus.RecordatorioDeReserva)
	if len(aviso.EquiposSinDevolver) != 0 {
		t.Fatalf("a las 7:00 esa máquina no estaba demorada: %+v", aviso.EquiposSinDevolver)
	}

	// Vuelve a las 7:25, antes de pasarse.
	if err := p.Devolver("", "", aLas(7, 25)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	resumen := barrer(t, vigilanteALas(repo, bus, 7, 45))

	if resumen.AvisosDePCFaltante != 0 {
		t.Errorf("la máquina volvió: el docente no tiene que enterarse de nada")
	}
}

func TestBarrer_ElAvisoDePCFaltanteSaleUnaSolaVez(t *testing.T) {
	repo := repoConClase(t, "pc1")
	prestamoVencido(t, repo, "pr1", "pc1", aLas(6, 30))
	bus := &busEspia{}
	v := vigilanteALas(repo, bus, 7, 30)

	barrer(t, v)
	for i := 0; i < 5; i++ {
		if r := barrer(t, v); r.AvisosDePCFaltante != 0 || r.Recordatorios != 0 {
			t.Fatalf("corrida %d: volvió a avisar (%+v)", i+2, r)
		}
	}
}

// ── El reclamo de devolución ────────────────────────────────────────────

func TestBarrer_ReclamaALosDiezMinutos(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.identificadorDePC["pc1"] = 7
	prestamoVencido(t, repo, "pr1", "pc1", aLas(9, 0))
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 9, 5)); r.Reclamos != 0 {
		t.Fatalf("a los 5 minutos todavía no se reclama: %+v", r)
	}

	resumen := barrer(t, vigilanteALas(repo, bus, 9, 10))

	if resumen.Reclamos != 1 {
		t.Fatalf("esperaba 1 reclamo, obtuve %d", resumen.Reclamos)
	}
	aviso := bus.de("prestamo.demorado")[0].Payload.(eventbus.PrestamosDemorados)
	if len(aviso.Prestamos) != 1 {
		t.Fatalf("esperaba 1 préstamo en el aviso: %+v", aviso)
	}
	d := aviso.Prestamos[0]
	if d.Etiqueta != "PC 7" || d.MinutosDeDemora != 10 || d.Email != "otro@escuela.edu.ar" {
		t.Errorf("datos del reclamo: %+v", d)
	}
}

func TestBarrer_ElReclamoSaleUnaSolaVez(t *testing.T) {
	repo := nuevoFakeRepo()
	prestamoVencido(t, repo, "pr1", "pc1", aLas(9, 0))
	bus := &busEspia{}

	barrer(t, vigilanteALas(repo, bus, 9, 15))
	for _, minuto := range []int{20, 30, 45} {
		if r := barrer(t, vigilanteALas(repo, bus, 9, minuto)); r.Reclamos != 0 {
			t.Fatalf("a las 9:%d volvió a reclamar", minuto)
		}
	}
	if len(bus.de("prestamo.demorado")) != 1 {
		t.Errorf("salieron %d reclamos, esperaba 1", len(bus.de("prestamo.demorado")))
	}
}

// TestBarrer_SinHoraPactadaNoSeReclama: "vengo en un rato" es una respuesta
// válida. Esas máquinas aparecen recién en el corte de fin de jornada.
func TestBarrer_SinHoraPactadaNoSeReclama(t *testing.T) {
	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{PCID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 15, 0)); r.Reclamos != 0 {
		t.Errorf("sin hora pactada no se reclama: %+v", r)
	}
}

// ── El corte de fin de jornada ──────────────────────────────────────────

func TestBarrer_CorteDeJornada(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.identificadorDePC["pc1"] = 3
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{PCID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 17, 0)); r.AvisosDeCierre != 0 {
		t.Fatalf("antes de la hora de cierre no hay corte: %+v", r)
	}

	resumen := barrer(t, vigilanteALas(repo, bus, 18, 0))

	if resumen.AvisosDeCierre != 1 {
		t.Fatalf("esperaba el corte: %+v", resumen)
	}
	aviso := bus.de("prestamo.sin-devolver.cierre")[0].Payload.(eventbus.PCsSinDevolverAlCierre)
	if len(aviso.PCs) != 1 || aviso.PCs[0].Etiqueta != "PC 3" || aviso.PCs[0].Quien != "Marta" {
		t.Errorf("datos del corte: %+v", aviso.PCs)
	}
}

func TestBarrer_ElCorteSaleUnaVezPorDiaYSeRepiteAlSiguiente(t *testing.T) {
	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{PCID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	barrer(t, vigilanteALas(repo, bus, 18, 0))
	for _, minuto := range []int{15, 30, 45} {
		if r := barrer(t, vigilanteALas(repo, bus, 18, minuto)); r.AvisosDeCierre != 0 {
			t.Fatalf("a las 18:%d volvió a cortar el mismo día", minuto)
		}
	}

	// Al día siguiente la máquina sigue afuera: vuelve a aparecer. Por eso
	// la marca es la fecha de la jornada y no un booleano.
	manana := NewVigilante(repo, bus, func() time.Time {
		return aLas(18, 0).AddDate(0, 0, 1)
	}, ConfigDeVigilanciaPorDefecto())

	if r := barrer(t, manana); r.AvisosDeCierre != 1 {
		t.Errorf("al día siguiente tiene que volver a avisar: %+v", r)
	}
}

func TestBarrer_SinNadaQueHacerNoPublicaNada(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 6, 0))

	if resumen.HizoAlgo() {
		t.Errorf("no debería haber hecho nada: %+v", resumen)
	}
	if len(bus.publicados) != 0 {
		t.Errorf("publicó %d eventos", len(bus.publicados))
	}
}

// TestBarrer_NoLiberaUnBloqueoPorEvaluacion
//
// Un bloqueo por evaluación estatal (RF-04.7) no es una reserva que alguien
// venga a retirar: es un Admin sacando máquinas de circulación para una mesa
// de examen. Liberarlo a los cuarenta minutos dejaría que otro docente
// reserve una computadora que está siendo usada en un examen, con el examen
// en curso.
func TestBarrer_NoLiberaUnBloqueoPorEvaluacion(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.identificadorDePC["pc1"] = 1
	bloqueo, err := domain.NuevaReservaEvaluacion("bloq1", "pc1", nil,
		aLas(0, 0), 8*time.Hour, 9*time.Hour, aLas(0, 0).AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[bloqueo.ID] = bloqueo
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 45))

	if resumen.Liberadas != 0 {
		t.Fatalf("un bloqueo por evaluación no se libera: %+v", resumen)
	}
	if repo.reservas["bloq1"].Estado != domain.ReservaConfirmada {
		t.Errorf("el bloqueo quedó en %s", repo.reservas["bloq1"].Estado)
	}
	if len(bus.publicados) != 0 {
		t.Errorf("no hay a quién avisarle: publicó %d eventos", len(bus.publicados))
	}
}
