package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/eventbus"
)

func TestPlazosConfigurables_SeReleenYUnFalloNoLibera(t *testing.T) {
	repo := repoConClase(t, "pc1")
	cfg := ConfigDeVigilanciaPorDefecto()
	var fallo bool
	v := vigilanteALas(repo, eventbus.NewInMemoryEventBus(), 8, 20).ConConfiguracion(func(context.Context) (ConfigDeVigilancia, error) {
		if fallo {
			return cfg, errors.New("sin configuración")
		}
		return cfg, nil
	})
	if res := barrer(t, v); res.Liberadas != 0 {
		t.Fatal("liberó antes de los 40 minutos")
	}
	fallo = true
	if _, err := v.Barrer(context.Background()); err == nil {
		t.Fatal("ignoró el fallo de configuración")
	}
	fallo = false
	cfg.GraciaDeRetiro = 10 * time.Minute
	if res := barrer(t, v); res.Liberadas != 1 {
		t.Fatal("no aplicó la configuración nueva en el siguiente barrido")
	}
}
func TestPlazosConfigurables_RecordatorioYCierre(t *testing.T) {
	repo := repoConClase(t, "pc1")
	v := vigilanteALas(repo, eventbus.NewInMemoryEventBus(), 6, 30)
	cfg := ConfigDeVigilanciaPorDefecto()
	v.ConConfiguracion(func(context.Context) (ConfigDeVigilancia, error) { return cfg, nil })
	if res := barrer(t, v); res.Recordatorios != 0 {
		t.Fatal("recordó con 90 minutos de antelación usando 60")
	}
	cfg.AntelacionRecordatorio = 120 * time.Minute
	if res := barrer(t, v); res.Recordatorios != 1 {
		t.Fatal("no aplicó la antelación configurada")
	}
	cierre := vigilanteConJornada(repo, eventbus.NewInMemoryEventBus(), aLas(18, 20), map[time.Weekday]time.Duration{aLas(18, 20).Weekday(): 18 * time.Hour})
	cfg.GraciaCierre = 30 * time.Minute
	cierre.ConConfiguracion(func(context.Context) (ConfigDeVigilancia, error) { return cfg, nil })
	_, _, ok := cierre.momentoYFechaDelCorte(context.Background(), aLas(18, 20))
	if ok {
		t.Fatal("cortó antes de la espera")
	}
	// La lectura dinámica se aplica al barrido; la comprobación aislada usa su copia de configuración.
	copia := *cierre
	copia.cfg = cfg
	_, _, ok = copia.momentoYFechaDelCorte(context.Background(), aLas(18, 20))
	if ok {
		t.Fatal("cortó antes de los 30 minutos")
	}
	copia.cfg.GraciaCierre = 0
	_, _, ok = copia.momentoYFechaDelCorte(context.Background(), aLas(18, 20))
	if !ok {
		t.Fatal("cero minutos no permitió cortar al cierre")
	}
}
