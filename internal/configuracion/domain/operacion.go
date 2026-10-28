package domain

import "fmt"

// Operacion contiene decisiones institucionales, sin secretos ni ajustes de infraestructura.
type Operacion struct {
	RetiroGraciaMinutos           int   `json:"retiroGraciaMinutos"`
	RecordatorioAntelacionMinutos int   `json:"recordatorioAntelacionMinutos"`
	CierreGraciaMinutos           int   `json:"cierreGraciaMinutos"`
	CierreHoraSinJornada          int   `json:"cierreHoraSinJornada"`
	LicenciasHoraAviso            int   `json:"licenciasHoraAviso"`
	Version                       int64 `json:"version"`
}

func (o Operacion) Validar() error {
	for _, campo := range []struct {
		nombre          string
		valor, min, max int
	}{
		{"espera para retirar", o.RetiroGraciaMinutos, 1, 1440},
		{"antelación del recordatorio", o.RecordatorioAntelacionMinutos, 1, 1440},
		{"espera después del cierre", o.CierreGraciaMinutos, 0, 720},
		{"hora de cierre sin jornada", o.CierreHoraSinJornada, 0, 23},
		{"hora del aviso de licencias", o.LicenciasHoraAviso, 0, 23},
	} {
		if campo.valor < campo.min || campo.valor > campo.max {
			return fmt.Errorf("%s debe estar entre %d y %d", campo.nombre, campo.min, campo.max)
		}
	}
	if o.Version < 1 {
		return fmt.Errorf("la versión de configuración no es válida")
	}
	return nil
}
