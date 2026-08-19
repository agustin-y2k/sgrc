package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// El binario se autochequea: `sgrc-app healthcheck` pega en /health del
// proceso que corre en el mismo contenedor y devuelve 0 o 1. Es la forma de
// tener un HEALTHCHECK en una imagen `FROM scratch`, que no tiene shell ni
// curl ni wget — el único ejecutable adentro es este.

// timeoutAutochequeo cubre conexión + respuesta.
const timeoutAutochequeo = 5 * time.Second

func esInvocacionDeHealthcheck(args []string) bool {
	return len(args) > 1 && args[1] == "healthcheck"
}

// ejecutarHealthcheck devuelve el código de salida: 0 sano, 1 cualquier otra
// cosa.
func ejecutarHealthcheck(puerto string) int {
	url := "http://" + net.JoinHostPort("127.0.0.1", puerto) + "/health"

	cliente := &http.Client{Timeout: timeoutAutochequeo}
	resp, err := cliente.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: %s respondió %d\n", url, resp.StatusCode)
		return 1
	}
	return 0
}
