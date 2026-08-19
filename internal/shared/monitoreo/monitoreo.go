// Package monitoreo avisa a un servicio externo que un barrido de fondo
// terminó bien.
package monitoreo

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// Los tres barridos. El nombre viaja en el log cuando algo sale mal, así que
// se parece al que usa el propio job.
const (
	JobReservasVencidas = "reservas-vencidas"
	JobBarridoEntregas  = "barrido-entregas"
	JobAvisoLicencias   = "aviso-licencias"
)

// variableDeEntorno mapea cada job a la variable que lleva su URL. Una por
// job y no una sola para todos: cada barrido tiene su propia frecuencia, así
// que en el servicio externo son tres chequeos distintos, con períodos
// distintos, y cada uno trae su URL.
var variableDeEntorno = map[string]string{
	JobReservasVencidas: "PING_URL_RESERVAS_VENCIDAS",
	JobBarridoEntregas:  "PING_URL_BARRIDO_ENTREGAS",
	JobAvisoLicencias:   "PING_URL_AVISO_LICENCIAS",
}

// timeoutDelAviso: el aviso es un lujo, el barrido es el trabajo.
const timeoutDelAviso = 10 * time.Second

// Avisador manda la señal de vida. El valor cero no sirve: usar DesdeEntorno.
type Avisador struct {
	urls    map[string]string
	cliente *http.Client
}

// DesdeEntorno arma el avisador leyendo una variable por job.
func DesdeEntorno(getenv func(string) string) (*Avisador, error) {
	urls := make(map[string]string)
	for job, variable := range variableDeEntorno {
		crudo := getenv(variable)
		if crudo == "" {
			continue
		}
		u, err := url.Parse(crudo)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("%s no es una URL http(s) válida: %q", variable, crudo)
		}
		urls[job] = crudo
	}

	return &Avisador{
		urls:    urls,
		cliente: &http.Client{Timeout: timeoutDelAviso},
	}, nil
}

// JobsConAviso devuelve los nombres de los barridos que tienen aviso
// configurado, para poder decirlo en el log de arranque.
func (a *Avisador) JobsConAviso() []string {
	nombres := make([]string, 0, len(a.urls))
	// Se recorre la lista de constantes y no el mapa: el orden de un mapa en Go
	// es aleatorio, y una línea de log que cambia de orden en cada arranque es
	// molesta de comparar entre reinicios.
	for _, job := range []string{JobReservasVencidas, JobBarridoEntregas, JobAvisoLicencias} {
		if _, hay := a.urls[job]; hay {
			nombres = append(nombres, job)
		}
	}
	return nombres
}

// Vive avisa que el barrido terminó bien.
func (a *Avisador) Vive(ctx context.Context, job string) {
	destino, hay := a.urls[job]
	if !hay {
		return
	}

	ctx, cancelar := context.WithTimeout(ctx, timeoutDelAviso)
	defer cancelar()

	pedido, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		log.Printf("aviso de vida de %s: no se pudo armar el pedido: %v", job, err)
		return
	}

	resp, err := a.cliente.Do(pedido)
	if err != nil {
		// Que el aviso no salga NO es una falla del sistema: el barrido ya hizo su
		// trabajo.
		log.Printf("aviso de vida de %s: %v", job, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("aviso de vida de %s: el servicio respondió %d", job, resp.StatusCode)
	}
}
