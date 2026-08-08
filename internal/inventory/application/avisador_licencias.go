package application

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// AvisadorDeLicencias es el barrido diario que avisa qué licencias hay que
// renovar (RF-03.14 / RF-05.9).
//
// Es un tipo aparte del Service y no un método suyo, por el mismo criterio
// que Mensajero en notification: no comparte casi nada con él —no genera
// IDs, no valida entradas de nadie— y su disparador no es un request HTTP
// sino un reloj. Mezclarlos obligaría a inyectarle el bus al Service, que
// hoy no publica ningún evento.
type AvisadorDeLicencias struct {
	repo  Repo
	bus   eventbus.EventBus
	ahora func() time.Time
}

func NewAvisadorDeLicencias(repo Repo, bus eventbus.EventBus, ahora func() time.Time) *AvisadorDeLicencias {
	return &AvisadorDeLicencias{repo: repo, bus: bus, ahora: ahora}
}

// Barrer revisa las licencias y, si hay algo que avisar, publica UN evento
// con todo junto. Devuelve cuántas licencias entraron en el aviso.
//
// Está pensado para correr muchas veces por día sin consecuencias: la
// idempotencia la dan las dos marcas de cada licencia (ver
// domain.CorrespondeAvisoPrevio). Diez reinicios del contenedor en la misma
// jornada mandan un mail, no diez.
//
// ORDEN: primero publica, después marca. Al revés sería más prolijo
// —marcar y recién ahí avisar— pero el modo de falla es peor: si el proceso
// se cae entre las dos cosas, con ese orden la licencia queda marcada como
// avisada sin que el aviso haya salido, y nadie se entera nunca. Así, lo
// que puede pasar es que un aviso salga dos veces. Un mail repetido molesta;
// un vencimiento que pasa en silencio es justamente lo que esta
// funcionalidad existe para evitar.
func (a *AvisadorDeLicencias) Barrer(ctx context.Context) (int, error) {
	hoy := domain.Dia(a.ahora())

	candidatas, err := a.repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		return 0, fmt.Errorf("buscando licencias por vencer: %w", err)
	}

	var aviso eventbus.AvisoDeLicencias
	var avisadas []*domain.LicenciaSoftware

	for _, c := range candidatas {
		l := c.Licencia
		switch {
		case l.CorrespondeAvisoPrevio(hoy):
			aviso.PorVencer = append(aviso.PorVencer, aLicenciaDelAviso(c, hoy))
			l.MarcarAvisoPrevioEnviado()
		case l.CorrespondeAvisoDeVencimiento(hoy):
			aviso.Vencidas = append(aviso.Vencidas, aLicenciaDelAviso(c, hoy))
			l.MarcarAvisoDeVencimientoEnviado()
		default:
			// Candidata por el filtro grueso pero sin nada que avisar hoy:
			// típicamente una que ya recibió su aviso previo y todavía no
			// llegó al día del vencimiento.
			continue
		}
		avisadas = append(avisadas, l)
	}

	if len(avisadas) == 0 {
		return 0, nil
	}

	a.bus.Publish(eventbus.Evento{Tipo: "licencia.por-vencer", Payload: aviso})

	// Un fallo al marcar no invalida el aviso, que ya salió: se loguea y se
	// sigue con las demás. La consecuencia de no marcar es que esa licencia
	// vuelva a avisar en la próxima barrida, no que se pierda nada.
	for _, l := range avisadas {
		if err := a.repo.MarcarAvisosEnviados(ctx, l); err != nil {
			log.Printf("aviso de licencias: no se pudo marcar como avisada la licencia %s "+
				"(el aviso ya salió; puede repetirse en la próxima barrida): %v", l.ID, err)
		}
	}

	return len(avisadas), nil
}

func aLicenciaDelAviso(c *LicenciaConUbicacion, hoy time.Time) eventbus.LicenciaPorVencer {
	dias, _ := c.Licencia.DiasRestantes(hoy)
	return eventbus.LicenciaPorVencer{
		LicenciaID:       c.Licencia.ID,
		Nombre:           c.Licencia.Nombre,
		Etiqueta:         c.Etiqueta,
		PCIdentificador:  c.PCIdentificador,
		CarroNombre:      c.CarroNombre,
		FechaVencimiento: *c.Licencia.FechaVencimiento,
		DiasRestantes:    dias,
	}
}
