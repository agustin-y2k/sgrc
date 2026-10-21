package application

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/agustin-y2k/sgrc/internal/inventory/domain"
	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
)

// AvisadorDeLicencias es el barrido diario que avisa qué licencias hay que
// renovar (RF-03.14 / RF-05.9).
type AvisadorDeLicencias struct {
	repo  Repo
	bus   eventbus.EventBus
	ahora func() time.Time
}

func NewAvisadorDeLicencias(repo Repo, bus eventbus.EventBus, ahora func() time.Time) *AvisadorDeLicencias {
	return &AvisadorDeLicencias{repo: repo, bus: bus, ahora: ahora}
}

// Barrer revisa las licencias y, si hay algo que avisar, publica UN evento
// con todo junto.
func (a *AvisadorDeLicencias) barrer(ctx context.Context) (int, error) {
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
			// Candidata por el filtro grueso pero sin nada que avisar hoy: típicamente
			// una que ya recibió su aviso previo y todavía no llegó al día del
			// vencimiento.
			continue
		}
		avisadas = append(avisadas, l)
	}

	if len(avisadas) == 0 {
		return 0, nil
	}

	eventbus.Emitir(ctx, a.bus, eventbus.Evento{Tipo: "licencia.por-vencer", Payload: aviso})

	// La marca y el evento comparten transacción. Cualquier fallo impide
	// confirmar el barrido durable.
	for _, l := range avisadas {
		if err := a.repo.MarcarAvisosEnviados(ctx, l); err != nil {
			pgtransaccion.Rechazar(ctx, err)
			log.Printf("aviso de licencias: no se pudo marcar como avisada la licencia %s "+
				"(el barrido durable se revierte): %v", l.ID, err)
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
		Identificador:    c.Identificador,
		CarroNombre:      c.CarroNombre,
		FechaVencimiento: *c.Licencia.FechaVencimiento,
		DiasRestantes:    dias,
	}
}

func (a *AvisadorDeLicencias) Barrer(ctx context.Context) (int, error) {
	var res int
	err := a.repo.Coordinar(ctx, true, func(ctx context.Context) error { var err error; res, err = a.barrer(ctx); return err })
	if err != nil {
		return 0, err
	}
	return res, nil
}
