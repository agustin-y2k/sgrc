package application

import (
	"context"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// busEspia guarda lo publicado.
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

func repoConClase(t *testing.T, equipos ...string) *fakeRepo {
	t.Helper()
	return repoConClaseDe(t, 8*time.Hour, 9*time.Hour, equipos...)
}

// repoConClaseCorta arma una clase de media hora: más corta que los cuarenta
// minutos de gracia, así que no se libera nunca y no hay nada que anunciarle.
func repoConClaseCorta(t *testing.T) *fakeRepo {
	t.Helper()
	return repoConClaseDe(t, 8*time.Hour, 8*time.Hour+30*time.Minute, "pc1")
}

func repoConClaseDe(t *testing.T, horaInicio, horaFin time.Duration, equipos ...string) *fakeRepo {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.contactoDeUsuario[docenteAda] = [2]string{"Ada Lovelace", "ada@escuela.edu.ar"}

	creadoPor := docenteAda
	grupo, err := domain.NuevoReservaGrupo(grupoDeClase, "materia1", &creadoPor, "Ada Lovelace",
		aLas(0, 0), horaInicio, horaFin, nil, aLas(0, 0).AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.grupos[grupo.ID] = grupo

	for i, equipoID := range equipos {
		repo.identificadorDeEquipo[equipoID] = i + 1
		r, err := domain.NuevaReservaNormal("res-"+equipoID, grupoDeClase, equipoID, "materia1",
			"Ada Lovelace", &creadoPor, aLas(0, 0), horaInicio, horaFin,
			aLas(0, 0).AddDate(0, 0, -1))
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		repo.reservas[r.ID] = r
	}
	return repo
}

// vigilanteALas arma el barrido para una institución que NO declaró jornada:
// el corte cae a la hora de CIERRE_JORNADA, que es como funcionaba antes de
// que la jornada existiera y sigue siendo el camino de quien elige no
// restringir nada.
func vigilanteALas(repo Repo, bus eventbus.EventBus, hora, minuto int) *Vigilante {
	return NewVigilante(repo, bus, &fakeValidadorJornada{}, func() time.Time { return aLas(hora, minuto) },
		ConfigDeVigilanciaPorDefecto())
}

// vigilanteConJornada arma el barrido para una institución que sí la declaró.
// `cierres` mapea día de la semana a la hora en que cierra ese día, medida
// desde su medianoche: pasa de 24h cuando el tramo cruza.
func vigilanteConJornada(repo Repo, bus eventbus.EventBus, momento time.Time, cierres map[time.Weekday]time.Duration) *Vigilante {
	validador := &fakeValidadorJornada{cierre: func(fecha time.Time) CierreDeJornada {
		fin, abre := cierres[fecha.Weekday()]
		return CierreDeJornada{Declarada: true, Abre: abre, Fin: fin}
	}}
	return NewVigilante(repo, bus, validador, func() time.Time { return momento },
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
// recordatorio sale tarde en vez de perderse.
func TestBarrer_RecordatorioTardioSaleIgual(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 10)); r.Recordatorios != 1 {
		t.Errorf("el recordatorio tardío tiene que salir igual: %+v", r)
	}
}

// ── El aviso de "todavía no las retiraste" (RF-08.20) ───────────────────

func TestBarrer_AvisaALos15DeEmpezada(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 10)); r.AvisosDeNoRetiro != 0 {
		t.Fatalf("a los 10 minutos todavía no: %+v", r)
	}
	if r := barrer(t, vigilanteALas(repo, bus, 8, 15)); r.AvisosDeNoRetiro != 1 {
		t.Fatalf("a los 15 sí: %+v", r)
	}

	eventos := bus.de("reserva.sin-retirar")
	if len(eventos) != 1 {
		t.Fatalf("esperaba 1 aviso, hubo %d", len(eventos))
	}
	aviso := eventos[0].Payload.(eventbus.ReservaSinRetirar)
	// Un aviso por clase con las dos máquinas adentro, no uno por máquina.
	if len(aviso.Equipos) != 2 {
		t.Errorf("el aviso tiene que traer las dos PCs: %+v", aviso.Equipos)
	}
	if aviso.Email != "ada@escuela.edu.ar" || aviso.UsuarioID != docenteAda {
		t.Errorf("falta el contacto del docente: %+v", aviso)
	}
	// El dato accionable: a los cuántos minutos las pierde, no a los cuántos
	// se le avisó.
	if aviso.MinutosDeGracia != 40 {
		t.Errorf("minutosDeGracia = %d, esperaba 40", aviso.MinutosDeGracia)
	}
}

func TestBarrer_ElAvisoDeNoRetiroSaleUnaSolaVez(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}
	v := vigilanteALas(repo, bus, 8, 15)

	barrer(t, v)
	for i := 0; i < 10; i++ {
		if r := barrer(t, v); r.AvisosDeNoRetiro != 0 {
			t.Fatalf("corrida %d: volvió a avisar", i+2)
		}
	}
	if len(bus.de("reserva.sin-retirar")) != 1 {
		t.Errorf("salieron %d avisos, esperaba 1", len(bus.de("reserva.sin-retirar")))
	}
}

// TestBarrer_SiRetiroAlgunaNoSeLeAvisaNada: vino al mostrador y eligió qué se
// llevaba. Avisarle de su propia decisión es ruido.
func TestBarrer_SiRetiroAlgunaNoSeLeAvisaNada(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	entregarContraLaReserva(t, repo, "pr1", "pc1", aLas(8, 5))
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 15)); r.AvisosDeNoRetiro != 0 {
		t.Fatalf("retiró una: no hay nada que avisarle: %+v", r)
	}
}

// TestBarrer_NoAvisaSiLaClaseEsMasCortaQueLaGracia: esa reserva no se libera
// nunca, así que anunciarle que va a quedar libre sería mentirle.
func TestBarrer_NoAvisaSiLaClaseEsMasCortaQueLaGracia(t *testing.T) {
	repo := repoConClaseCorta(t)
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 20)); r.AvisosDeNoRetiro != 0 {
		t.Fatalf("una clase de media hora no se libera: no hay nada que anunciar: %+v", r)
	}
}

// TestBarrer_SiVuelveDespuesDeLaGraciaLiberaSinAvisar: el proceso estuvo
// caído y volvió a las 8:45. El aviso llegaría anunciando algo que ya pasó,
// así que no sale: mejor callarse que mentir.
func TestBarrer_SiVuelveDespuesDeLaGraciaLiberaSinAvisar(t *testing.T) {
	repo := repoConClase(t, "pc1")
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 45))

	if resumen.AvisosDeNoRetiro != 0 {
		t.Errorf("el aviso no puede salir junto con la liberación: %+v", resumen)
	}
	if resumen.Liberadas != 1 {
		t.Errorf("pero liberar sí libera: %+v", resumen)
	}
	if len(bus.de("reserva.sin-retirar")) != 0 {
		t.Error("no tiene que salir ningún aviso de no retiro")
	}
}

// ── La liberación ───────────────────────────────────────────────────────

func TestBarrer_LiberaALos40Minutos(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 30)); r.Liberadas != 0 {
		t.Fatalf("a los 30 minutos todavía no: %+v", r)
	}
	publicadosAntesDeLiberar := len(bus.publicados)

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

	// Liberar no publica nada: el aviso al docente ya salió a los 15 minutos
	// (RF-08.20), cuando todavía podía hacer algo.
	if len(bus.publicados) != publicadosAntesDeLiberar {
		t.Errorf("la barrida que liberó publicó %d eventos y no tenía que publicar ninguno: %+v",
			len(bus.publicados)-publicadosAntesDeLiberar, bus.publicados[publicadosAntesDeLiberar:])
	}
}

// TestBarrer_NoLiberaLaQueSeLlevaron es la condición que separa "el docente
// no vino" de "el docente vino": si la máquina está afuera, la reserva está
// cumplida aunque nadie haya apretado nada más.
func TestBarrer_NoLiberaLaQueSeLlevaron(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	// Se llevó la primera.
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Ada"}, aLas(8, 5))
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
}

// entregarContraLaReserva registra que el docente pasó por el mostrador y se
// llevó esa máquina de su propia reserva, que es lo que dispara el plazo
// corto.
func entregarContraLaReserva(t *testing.T, repo *fakeRepo, prestamoID, equipoID string, cuando time.Time) {
	t.Helper()
	reservaID := "res-" + equipoID
	p, err := domain.NuevoPrestamo(prestamoID, domain.DatosDeEntrega{
		EquipoID: equipoID, ReservaID: &reservaID, Nombre: "Ada",
	}, cuando)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
}

// TestBarrer_TrasEntregaParcialLiberaALos15DeLaEntrega: el docente vino 8:05
// y se llevó una de las dos.
func TestBarrer_TrasEntregaParcialLiberaALos15DeLaEntrega(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2")
	entregarContraLaReserva(t, repo, "pr1", "pc1", aLas(8, 5))
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 8, 15)); r.Liberadas != 0 {
		t.Fatalf("a los 10 minutos de la entrega todavía no: %+v", r)
	}
	publicadosAntesDeLiberar := len(bus.publicados)

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 20))

	if resumen.Liberadas != 1 {
		t.Fatalf("esperaba 1 liberada a los 15 de la entrega, obtuve %d", resumen.Liberadas)
	}
	if repo.reservas["res-pc2"].Estado != domain.ReservaNoRetirada {
		t.Errorf("res-pc2 quedó en %s", repo.reservas["res-pc2"].Estado)
	}
	// Vino a dar la clase: el grupo sigue confirmado.
	if repo.grupos[grupoDeClase].Estado != domain.GrupoConfirmada {
		t.Errorf("el grupo quedó en %s, esperaba CONFIRMADA", repo.grupos[grupoDeClase].Estado)
	}
	// Y sobre todo: ni un correo ni una campana. Lo decidió él en el mostrador.
	if len(bus.publicados) != publicadosAntesDeLiberar {
		t.Errorf("la entrega parcial libera en silencio, se publicó: %+v",
			bus.publicados[publicadosAntesDeLiberar:])
	}
}

// TestBarrer_ElPlazoCortoCuentaDesdeLaUltimaEntrega: mientras el Admin sigue
// anotando, el docente sigue en el mostrador.
func TestBarrer_ElPlazoCortoCuentaDesdeLaUltimaEntrega(t *testing.T) {
	repo := repoConClase(t, "pc1", "pc2", "pc3")
	entregarContraLaReserva(t, repo, "pr1", "pc1", aLas(8, 5))
	entregarContraLaReserva(t, repo, "pr2", "pc2", aLas(8, 12))
	bus := &busEspia{}

	// 8:20 son quince minutos de la PRIMERA entrega, pero solo ocho de la
	// última: la pc3 todavía es suya.
	if r := barrer(t, vigilanteALas(repo, bus, 8, 20)); r.Liberadas != 0 {
		t.Fatalf("el plazo corre desde la última entrega: %+v", r)
	}
	if r := barrer(t, vigilanteALas(repo, bus, 8, 27)); r.Liberadas != 1 {
		t.Fatalf("a los quince de la última entrega sí: %+v", r)
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
	if repo.reservas["res-pc1"].Estado != domain.ReservaNoRetirada {
		t.Errorf("res-pc1 quedó en %s", repo.reservas["res-pc1"].Estado)
	}
}

// ── La PC que no volvió ─────────────────────────────────────────────────

// prestamoVencido deja una máquina afuera que tenía que haber vuelto.
func prestamoVencido(t *testing.T, repo *fakeRepo, id, equipoID string, debioVolverA time.Time) *domain.Prestamo {
	t.Helper()
	usuario := "otro-docente"
	p, err := domain.NuevoPrestamo(id, domain.DatosDeEntrega{
		EquipoID: equipoID, Nombre: "Otro Docente", UsuarioID: &usuario,
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
	if resumen.AvisosDeEquipoFaltante != 0 {
		t.Errorf("la advertencia va adentro del recordatorio, no aparte: %+v", resumen)
	}
	aviso := bus.de("reserva.recordatorio")[0].Payload.(eventbus.RecordatorioDeReserva)
	if len(aviso.EquiposSinDevolver) != 1 || aviso.EquiposSinDevolver[0] != "PC 1" {
		t.Errorf("el recordatorio tiene que advertir de la PC 1: %+v", aviso.EquiposSinDevolver)
	}
	if len(bus.de("reserva.equipo-no-disponible")) != 0 {
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

	if resumen.AvisosDeEquipoFaltante != 1 {
		t.Fatalf("esperaba el aviso suelto: %+v", resumen)
	}
	aviso := bus.de("reserva.equipo-no-disponible")[0].Payload.(eventbus.EquipoNoDisponibleParaReserva)
	if len(aviso.Equipos) != 1 {
		t.Errorf("el aviso tiene que nombrar el equipo: %+v", aviso)
	}
}

// TestBarrer_SiElEquipoVuelveATiempoNadieSeEntera es la razón por la que esto
// no bombardea: el caso más común es que alguien se demore quince minutos y
// devuelva.
func TestBarrer_SiElEquipoVuelveATiempoNadieSeEntera(t *testing.T) {
	repo := repoConClase(t, "pc1")
	// La clase es a las 8; la máquina la tenía otro y debía volver a las 7:30, o
	// sea que a las 7:00 —cuando sale el recordatorio— todavía no estaba
	// demorada.
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

	if resumen.AvisosDeEquipoFaltante != 0 {
		t.Errorf("la máquina volvió: el docente no tiene que enterarse de nada")
	}
}

func TestBarrer_ElAvisoDeEquipoFaltanteSaleUnaSolaVez(t *testing.T) {
	repo := repoConClase(t, "pc1")
	prestamoVencido(t, repo, "pr1", "pc1", aLas(6, 30))
	bus := &busEspia{}
	v := vigilanteALas(repo, bus, 7, 30)

	barrer(t, v)
	for i := 0; i < 5; i++ {
		if r := barrer(t, v); r.AvisosDeEquipoFaltante != 0 || r.Recordatorios != 0 {
			t.Fatalf("corrida %d: volvió a avisar (%+v)", i+2, r)
		}
	}
}

// ── El reclamo de devolución ────────────────────────────────────────────

func TestBarrer_ReclamaALosDiezMinutos(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.identificadorDeEquipo["pc1"] = 7
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
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
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
	repo.identificadorDeEquipo["pc1"] = 3
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 22, 0)); r.AvisosDeCierre != 0 {
		t.Fatalf("antes de la hora de cierre no hay corte: %+v", r)
	}

	resumen := barrer(t, vigilanteALas(repo, bus, 23, 0))

	if resumen.AvisosDeCierre != 1 {
		t.Fatalf("esperaba el corte: %+v", resumen)
	}
	aviso := bus.de("prestamo.sin-devolver.cierre")[0].Payload.(eventbus.EquiposSinDevolverAlCierre)
	if len(aviso.Equipos) != 1 || aviso.Equipos[0].Etiqueta != "PC 3" || aviso.Equipos[0].Quien != "Marta" {
		t.Errorf("datos del corte: %+v", aviso.Equipos)
	}
}

// Una máquina que salió para una clase que termina DESPUÉS del cierre no
// "quedó afuera": está en uso. Antes el corte las contaba a todas, así que el
// docente de la próxima reserva recibía un "tu computadora puede no estar"
// que era falso — y con el corte saliendo una sola vez, ese falso positivo se
// come el único aviso del préstamo.
func TestBarrer_ElCorteNoCuentaLaMaquinaQueTodaviaEstaEnHora(t *testing.T) {
	repo := nuevoFakeRepo()
	devuelveALas2330 := aLas(23, 30)
	prestamoVencido(t, repo, "pr1", "pc1", devuelveALas2330)
	bus := &busEspia{}

	// El cierre es a las 23, pero la devolución está pactada para las 23:30.
	if r := barrer(t, vigilanteALas(repo, bus, 23, 0)); r.AvisosDeCierre != 0 {
		t.Fatalf("la máquina todavía está en hora, no quedó afuera: %+v", r)
	}

	// Pasada la hora de devolución sí corresponde, y ahí el aviso es cierto.
	if r := barrer(t, vigilanteALas(repo, bus, 23, 45)); r.AvisosDeCierre != 1 {
		t.Errorf("vencida la devolución el corte tiene que salir: %+v", r)
	}
}

// La contracara: un préstamo espontáneo no tiene hora pactada y por eso
// ExcedioLaDemora nunca lo reclama. Si el corte tampoco lo tomara, una
// máquina prestada en el mostrador podría no generar un solo aviso nunca.
func TestBarrer_ElCorteSiCuentaAlPrestamoSinHoraPactada(t *testing.T) {
	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 23, 0)); r.AvisosDeCierre != 1 {
		t.Errorf("sin hora pactada el corte es el único aviso posible: %+v", r)
	}
}

// El aviso sale UNA vez por préstamo y no se repite nunca más, ni al día
// siguiente. Lo que sostiene la insistencia dejó de ser el correo: la máquina
// figura en "qué hay afuera" hasta que alguien la recibe, y eso no se puede
// tapar marcando un aviso como leído.
func TestBarrer_ElCorteSaleUnaSolaVezPorPrestamo(t *testing.T) {
	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteALas(repo, bus, 23, 0)); r.AvisosDeCierre != 1 {
		t.Fatalf("esperaba el corte: %+v", r)
	}
	for _, minuto := range []int{15, 30, 45} {
		if r := barrer(t, vigilanteALas(repo, bus, 23, minuto)); r.AvisosDeCierre != 0 {
			t.Fatalf("a las 23:%d volvió a cortar el mismo día", minuto)
		}
	}

	// Al día siguiente la máquina sigue afuera y NO se vuelve a avisar.
	manana := NewVigilante(repo, bus, &fakeValidadorJornada{}, func() time.Time {
		return aLas(23, 0).AddDate(0, 0, 1)
	}, ConfigDeVigilanciaPorDefecto())

	if r := barrer(t, manana); r.AvisosDeCierre != 0 {
		t.Errorf("el aviso es por única vez: %+v", r)
	}
}

// ── El corte derivado de la jornada declarada ───────────────────────────

// El corte sale una hora después de que la escuela cierra de verdad. Con la
// hora fija, una que cierra a las 22 recibía el aviso a las 18, con las
// máquinas legítimamente en clase.
func TestBarrer_ElCorteSaleUnaHoraDespuesDelCierreDeclarado(t *testing.T) {
	// El 10 de agosto de 2026 es un lunes (ver aLas).
	cierreALas22 := map[time.Weekday]time.Duration{time.Monday: 22 * time.Hour}

	casos := []struct {
		hora     int
		minuto   int
		esperado int
		porQue   string
	}{
		{18, 0, 0, "a las 18 la escuela todavía está abierta"},
		{22, 30, 0, "recién cerró: la hora de gracia no pasó"},
		{23, 0, 1, "una hora después del cierre, ahí sí"},
	}

	for _, c := range casos {
		repo := nuevoFakeRepo()
		p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		repo.prestamos[p.ID] = p
		bus := &busEspia{}

		v := vigilanteConJornada(repo, bus, aLas(c.hora, c.minuto), cierreALas22)
		if r := barrer(t, v); r.AvisosDeCierre != c.esperado {
			t.Errorf("a las %02d:%02d esperaba %d avisos (%s): %+v",
				c.hora, c.minuto, c.esperado, c.porQue, r)
		}
	}
}

// La nocturna: el lunes cierra a la 01:00 del martes, así que su corte cae a
// las 02:00 del martes. Es el caso que el dedupe por fecha de calendario
// rompía, y que "una sola vez por préstamo" resuelve sin bookkeeping.
func TestBarrer_LaNocturnaCortaDeMadrugadaDelDiaSiguiente(t *testing.T) {
	// Lunes de 20:00 a 01:00 = cierra a las 25h de su propio lunes.
	nocturna := map[time.Weekday]time.Duration{time.Monday: 25 * time.Hour}

	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(21, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	// 01:30 del martes: la escuela cerró hace media hora, falta la gracia.
	martes0130 := aLas(1, 30).AddDate(0, 0, 1)
	if r := barrer(t, vigilanteConJornada(repo, bus, martes0130, nocturna)); r.AvisosDeCierre != 0 {
		t.Fatalf("todavía no pasó la hora de gracia: %+v", r)
	}

	// 02:00 del martes: una hora después del cierre del lunes.
	martes0200 := aLas(2, 0).AddDate(0, 0, 1)
	if r := barrer(t, vigilanteConJornada(repo, bus, martes0200, nocturna)); r.AvisosDeCierre != 1 {
		t.Errorf("el corte del lunes cae de madrugada del martes: %+v", r)
	}
}

// Un día que la escuela no abre no tiene corte: nadie dejó una máquina afuera
// de una escuela que no abrió. Es lo que hace que un aviso del viernes no se
// repita el sábado ni el domingo.
func TestBarrer_UnDiaCerradoNoTieneCorte(t *testing.T) {
	// Solo abre los viernes; el sábado está cerrado.
	soloViernes := map[time.Weekday]time.Duration{time.Friday: 18 * time.Hour}

	repo := nuevoFakeRepo()
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(9, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	// aLas() da un lunes; el sábado es cinco días después.
	sabado := aLas(20, 0).AddDate(0, 0, 5)
	if sabado.Weekday() != time.Saturday {
		t.Fatalf("el fixture no cae sábado sino %v", sabado.Weekday())
	}

	if r := barrer(t, vigilanteConJornada(repo, bus, sabado, soloViernes)); r.AvisosDeCierre != 0 {
		t.Errorf("un día cerrado no corta: %+v", r)
	}
}

// Una máquina entregada DESPUÉS de que cerró no "quedó" afuera: recién salió.
func TestBarrer_LaMaquinaEntregadaDespuesDelCierreNoQuedoAfuera(t *testing.T) {
	cierreALas18 := map[time.Weekday]time.Duration{time.Monday: 18 * time.Hour}

	repo := nuevoFakeRepo()
	// Sale a las 19, con la escuela ya cerrada.
	p, err := domain.NuevoPrestamo("pr1", domain.DatosDeEntrega{EquipoID: "pc1", Nombre: "Marta"}, aLas(19, 0))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[p.ID] = p
	bus := &busEspia{}

	if r := barrer(t, vigilanteConJornada(repo, bus, aLas(19, 30), cierreALas18)); r.AvisosDeCierre != 0 {
		t.Errorf("salió después del cierre, no quedó afuera: %+v", r)
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

// TestBarrer_NoLiberaUnBloqueoAdministrativo Un bloqueo administrativo
// (RF-04.7) no es una reserva que alguien venga a retirar: es un Admin
// sacando máquinas de circulación para una mesa de examen.
func TestBarrer_NoLiberaUnBloqueoAdministrativo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.identificadorDeEquipo["pc1"] = 1
	bloqueo, err := domain.NuevaReservaBloqueo("bloq1", "pc1", nil,
		aLas(0, 0), 8*time.Hour, 9*time.Hour, "Jornada docente", aLas(0, 0).AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[bloqueo.ID] = bloqueo
	bus := &busEspia{}

	resumen := barrer(t, vigilanteALas(repo, bus, 8, 45))

	if resumen.Liberadas != 0 {
		t.Fatalf("un bloqueo administrativo no se libera: %+v", resumen)
	}
	if repo.reservas["bloq1"].Estado != domain.ReservaConfirmada {
		t.Errorf("el bloqueo quedó en %s", repo.reservas["bloq1"].Estado)
	}
	if len(bus.publicados) != 0 {
		t.Errorf("no hay a quién avisarle: publicó %d eventos", len(bus.publicados))
	}
}
