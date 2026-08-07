package application

import (
	"context"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// busEspia guarda lo publicado para poder afirmar cuántos eventos salieron,
// que es el punto de todo esto: ocho licencias del mismo carro tienen que
// dar UN aviso.
type busEspia struct {
	publicados []eventbus.Evento
}

func (b *busEspia) Publish(e eventbus.Evento)                      { b.publicados = append(b.publicados, e) }
func (b *busEspia) Subscribe(tipo string, h func(eventbus.Evento)) {}

func (b *busEspia) ultimoAviso(t *testing.T) eventbus.AvisoDeLicencias {
	t.Helper()
	if len(b.publicados) == 0 {
		t.Fatal("no se publicó ningún evento")
	}
	aviso, ok := b.publicados[len(b.publicados)-1].Payload.(eventbus.AvisoDeLicencias)
	if !ok {
		t.Fatalf("payload inesperado: %+v", b.publicados[len(b.publicados)-1].Payload)
	}
	return aviso
}

// avisadorDeTest fija el reloj en el 7 de agosto de 2026, a media tarde
// (para que el recorte a día tenga algo que recortar).
func avisadorDeTest(repo Repo, bus eventbus.EventBus) *AvisadorDeLicencias {
	return NewAvisadorDeLicencias(repo, bus, func() time.Time {
		return time.Date(2026, time.August, 7, 15, 40, 0, 0, time.UTC)
	})
}

func licenciaEnPC(t *testing.T, repo *fakeRepo, id, pcID, nombre string, vencimiento *time.Time, diasAviso int) *domain.LicenciaSoftware {
	t.Helper()
	l, err := domain.NuevaLicencia(id, pcID, nombre, 30, diasAviso, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if vencimiento != nil {
		l.FijarVencimiento(*vencimiento, "admin-1", time.Now())
	}
	repo.licencias[id] = l
	return l
}

func TestBarrer_LicenciaQueVenceManana(t *testing.T) {
	repo := repoConCarroYPCs(1)
	vence := dia(2026, time.August, 8)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &vence, 1)
	bus := &busEspia{}

	n, err := avisadorDeTest(repo, bus).Barrer(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 1 {
		t.Fatalf("esperaba 1 licencia avisada, obtuve %d", n)
	}
	aviso := bus.ultimoAviso(t)
	if len(aviso.PorVencer) != 1 || len(aviso.Vencidas) != 0 {
		t.Fatalf("esperaba 1 por vencer y 0 vencidas, obtuve %d y %d", len(aviso.PorVencer), len(aviso.Vencidas))
	}
	l := aviso.PorVencer[0]
	if l.Nombre != "AutoCAD 2027" || l.DiasRestantes != 1 {
		t.Errorf("aviso incompleto: %+v", l)
	}
	// La ubicación viaja resuelta: el aviso lo lee alguien que tiene que ir
	// hasta la máquina.
	if l.PCIdentificador != 1 || l.CarroNombre != "Carro 1" {
		t.Errorf("falta la ubicación: PC %d del carro %q", l.PCIdentificador, l.CarroNombre)
	}
}

// TestBarrer_DosCorridasElMismoDiaAvisanUnaVez es la prueba de la
// idempotencia completa: el job corre cada hora y el contenedor se
// reinicia, pero el mail sale uno solo.
func TestBarrer_DosCorridasElMismoDiaAvisanUnaVez(t *testing.T) {
	repo := repoConCarroYPCs(1)
	vence := dia(2026, time.August, 8)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &vence, 1)
	bus := &busEspia{}
	avisador := avisadorDeTest(repo, bus)
	ctx := context.Background()

	if _, err := avisador.Barrer(ctx); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	for i := 0; i < 10; i++ {
		n, err := avisador.Barrer(ctx)
		if err != nil {
			t.Fatalf("corrida %d: %v", i+2, err)
		}
		if n != 0 {
			t.Fatalf("corrida %d: volvió a avisar %d licencias", i+2, n)
		}
	}

	if len(bus.publicados) != 1 {
		t.Errorf("se publicaron %d eventos, esperaba 1", len(bus.publicados))
	}
}

func TestBarrer_LicenciaVencidaVaAlGrupoDeVencidas(t *testing.T) {
	repo := repoConCarroYPCs(1)
	vencio := dia(2026, time.August, 4)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &vencio, 1)
	bus := &busEspia{}

	if _, err := avisadorDeTest(repo, bus).Barrer(context.Background()); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	aviso := bus.ultimoAviso(t)
	if len(aviso.Vencidas) != 1 || len(aviso.PorVencer) != 0 {
		t.Fatalf("esperaba 1 vencida y 0 por vencer, obtuve %d y %d", len(aviso.Vencidas), len(aviso.PorVencer))
	}
	if aviso.Vencidas[0].DiasRestantes != -3 {
		t.Errorf("diasRestantes = %d, esperaba -3", aviso.Vencidas[0].DiasRestantes)
	}
}

// TestBarrer_OchoPCsDelMismoCarroDanUnSoloAviso es la lección que el
// proyecto ya aprendió con las cancelaciones de reservas: un evento por
// fila afectada llena la campana de avisos idénticos.
func TestBarrer_OchoPCsDelMismoCarroDanUnSoloAviso(t *testing.T) {
	repo := repoConCarroYPCs(8)
	vence := dia(2026, time.August, 8)
	for i := 1; i <= 8; i++ {
		pcID := "pc-" + string(rune('0'+i))
		licenciaEnPC(t, repo, "lic-"+string(rune('0'+i)), pcID, "AutoCAD 2027", &vence, 1)
	}
	bus := &busEspia{}

	n, err := avisadorDeTest(repo, bus).Barrer(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 8 {
		t.Errorf("esperaba 8 licencias avisadas, obtuve %d", n)
	}
	if len(bus.publicados) != 1 {
		t.Fatalf("se publicaron %d eventos, esperaba 1 con las ocho adentro", len(bus.publicados))
	}
	if len(bus.ultimoAviso(t).PorVencer) != 8 {
		t.Errorf("el aviso debería traer las 8 licencias, trae %d", len(bus.ultimoAviso(t).PorVencer))
	}
}

func TestBarrer_SinNadaQueAvisarNoPublica(t *testing.T) {
	repo := repoConCarroYPCs(2)
	lejos := dia(2026, time.December, 1)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &lejos, 1)
	// Y una sin fecha: cargada pero todavía sin verificar contra la máquina.
	licenciaEnPC(t, repo, "lic-2", "pc-2", "SolidWorks", nil, 1)
	bus := &busEspia{}

	n, err := avisadorDeTest(repo, bus).Barrer(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 0 {
		t.Errorf("no debería avisar nada, avisó %d", n)
	}
	if len(bus.publicados) != 0 {
		t.Errorf("no debería publicar ningún evento, publicó %d", len(bus.publicados))
	}
}

// TestBarrer_UnaLicenciaSinFechaNoAvisaNunca: el estado "a verificar" es
// silencioso a propósito. Si avisara, la única forma de callar el aviso
// sería inventarle una fecha.
func TestBarrer_UnaLicenciaSinFechaNoAvisaNunca(t *testing.T) {
	repo := repoConCarroYPCs(1)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", nil, 365)
	bus := &busEspia{}

	n, err := avisadorDeTest(repo, bus).Barrer(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 0 || len(bus.publicados) != 0 {
		t.Errorf("una licencia sin fecha no puede generar avisos: n=%d eventos=%d", n, len(bus.publicados))
	}
}

func TestBarrer_PCDadaDeBajaNoAvisa(t *testing.T) {
	repo := repoConCarroYPCs(1)
	repo.pcs["pc-1"].DadaDeBaja = true
	vence := dia(2026, time.August, 8)
	licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &vence, 1)
	bus := &busEspia{}

	n, err := avisadorDeTest(repo, bus).Barrer(context.Background())

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 0 {
		t.Errorf("una PC dada de baja no debería generar avisos, generó %d", n)
	}
}

// TestBarrer_RenovarReabreElAviso cierra el ciclo completo: aviso, el Admin
// renueva, y el vencimiento nuevo vuelve a avisar a su tiempo.
func TestBarrer_RenovarReabreElAviso(t *testing.T) {
	repo := repoConCarroYPCs(1)
	vence := dia(2026, time.August, 8)
	l := licenciaEnPC(t, repo, "lic-1", "pc-1", "AutoCAD 2027", &vence, 1)
	bus := &busEspia{}
	ctx := context.Background()

	if _, err := avisadorDeTest(repo, bus).Barrer(ctx); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := l.Renovar(dia(2026, time.August, 7), "admin-1", time.Now()); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// El día previo al vencimiento nuevo (6 de septiembre).
	avisadorEnSeptiembre := NewAvisadorDeLicencias(repo, bus, func() time.Time {
		return time.Date(2026, time.September, 5, 7, 30, 0, 0, time.UTC)
	})
	n, err := avisadorEnSeptiembre.Barrer(ctx)

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if n != 1 {
		t.Errorf("tras renovar, el ciclo nuevo debería volver a avisar: n=%d", n)
	}
	if len(bus.publicados) != 2 {
		t.Errorf("esperaba 2 eventos (uno por ciclo), hubo %d", len(bus.publicados))
	}
}
