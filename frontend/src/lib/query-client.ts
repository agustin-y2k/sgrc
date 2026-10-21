import { QueryClient } from "@tanstack/react-query"

import { ApiError } from "@/lib/api-client"

/** Cuántas veces se reintenta algo que sí puede mejorar solo. */
const REINTENTOS = 3

/**
 * Un error del cliente no se reintenta.
 *
 * react-query reintenta tres veces por defecto, y sin configurar nada eso
 * incluía los 4xx. Un 404 no va a volverse 200, ni un 400 se arregla
 * repitiéndolo: lo único que se gana son unos siete segundos de espera antes
 * de que la pantalla diga qué pasó. Mientras corren los reintentos la consulta
 * sigue en `isLoading`, así que lo que la persona ve es un cartel de
 * "Cargando…" que no termina — y después el mensaje real. Es exactamente lo
 * que hacía parecer un cuelgue a un backend que ya había contestado.
 *
 * Lo que sí se reintenta es lo que puede mejorar solo: un corte de red, un 502
 * del proxy mientras se reinicia, un 500 puntual. Ahí el reintento es la razón
 * por la que existe la opción.
 *
 * El 401 queda del lado de los que no se reintentan por partida doble: además
 * de no mejorar, api-client ya lo trató como sesión rechazada y mandó al login
 * (ver registrarManejadorDeSesionRechazada).
 */
function esDelCliente(error: unknown): boolean {
  return error instanceof ApiError && error.status >= 400 && error.status < 500
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (intentosFallidos, error) =>
        !esDelCliente(error) && intentosFallidos < REINTENTOS,
    },
    mutations: {
      // Si el servidor confirmó una escritura pero se perdió su respuesta,
      // repetirla puede crear otra reserva, préstamo o mensaje. El usuario
      // debe poder revisar el estado antes de decidir otro intento.
      retry: false,
    },
  },
})
