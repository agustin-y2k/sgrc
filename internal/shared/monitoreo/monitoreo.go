// Package monitoreo avisa a un servicio externo que un barrido de fondo
// terminó bien.
//
// El problema que resuelve: los tres barridos del sistema corren en
// goroutines del mismo proceso (ver cmd/main.go). Si una muere o se queda
// colgada, el proceso sigue vivo, la web responde, el healthcheck da verde y
// nadie se entera. Lo que deja de pasar —las reservas no se finalizan, los
// avisos de retiro no salen— se descubre semanas después, cuando alguien
// pregunta por qué su reserva sigue abierta.
//
// La forma de detectar eso NO es un aviso cuando algo falla: si la goroutine
// está muerta, tampoco puede avisar. Es al revés — el barrido dice "estoy
// vivo" cada vez que termina bien, y el servicio externo alerta cuando ese
// aviso DEJA de llegar. Es lo que se llama un interruptor de hombre muerto:
// el silencio es la señal.
//
// Por eso solo se avisa el éxito y no el fallo: la alerta salta igual —el
// aviso no llegó— y así el paquete no depende de las convenciones de ningún
// proveedor en particular (unos esperan /fail, otros un parámetro).
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

// timeoutDelAviso: el aviso es un lujo, el barrido es el trabajo. Si el
// servicio externo no contesta, se abandona rápido y el job sigue con lo
// suyo.
const timeoutDelAviso = 10 * time.Second

// Avisador manda la señal de vida. El valor cero no sirve: usar DesdeEntorno.
type Avisador struct {
	urls    map[string]string
	cliente *http.Client
}

// DesdeEntorno arma el avisador leyendo una variable por job. Las que no
// estén configuradas dejan ese job sin aviso, que es el estado por defecto:
// el sistema tiene que poder levantarse sin depender de un servicio externo.
//
// Devuelve error si una URL está escrita pero es inválida. Callar eso sería
// lo peor de los dos mundos: alguien configuró el monitoreo, cree que está
// cubierto, y no lo está.
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
// configurado, para poder decirlo en el log de arranque. Sin eso, la única
// forma de saber si el monitoreo quedó activo es esperar a que falle algo y
// ver si llegó la alerta — que es exactamente cuando no se quiere descubrir
// que no estaba configurado.
func (a *Avisador) JobsConAviso() []string {
	nombres := make([]string, 0, len(a.urls))
	// Se recorre la lista de constantes y no el mapa: el orden de un mapa en
	// Go es aleatorio, y una línea de log que cambia de orden en cada
	// arranque es molesta de comparar entre reinicios.
	for _, job := range []string{JobReservasVencidas, JobBarridoEntregas, JobAvisoLicencias} {
		if _, hay := a.urls[job]; hay {
			nombres = append(nombres, job)
		}
	}
	return nombres
}

// Vive avisa que el barrido terminó bien. No devuelve error a propósito: el
// llamador no puede hacer nada útil con él y no queremos que un problema del
// servicio de monitoreo se confunda con un problema del barrido. Los fallos
// se loguean y ahí termina.
//
// Es sincrónica: tarda como mucho timeoutDelAviso, y corre en la goroutine
// del job, que después vuelve a esperar cinco minutos. Lanzar otra goroutine
// para ahorrar ese tiempo agregaría una carrera con el apagado a cambio de
// nada.
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
		// Que el aviso no salga NO es una falla del sistema: el barrido ya
		// hizo su trabajo. Se loguea para poder distinguir después "el job
		// dejó de correr" de "el job corre pero no llega el aviso", que
		// desde el servicio externo se ven igual.
		log.Printf("aviso de vida de %s: %v", job, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("aviso de vida de %s: el servicio respondió %d", job, resp.StatusCode)
	}
}
