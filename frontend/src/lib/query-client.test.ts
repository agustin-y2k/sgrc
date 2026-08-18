import { ApiError } from "@/lib/api-client"
import { queryClient } from "@/lib/query-client"

/**
 * El reintento de un 4xx no arregla nada y sí retrasa el mensaje: mientras
 * corre, la consulta sigue en isLoading y la pantalla muestra "Cargando…"
 * durante unos siete segundos antes de decir qué pasó. Es lo que hacía parecer
 * un cuelgue a un backend que ya había contestado.
 */
function reintentaConsulta(error: Error, intentosFallidos = 0): boolean {
  const retry = queryClient.getDefaultOptions().queries?.retry
  if (typeof retry !== "function") throw new Error("se esperaba una función de reintento")
  return retry(intentosFallidos, error) as boolean
}

function reintentaMutacion(error: Error, intentosFallidos = 0): boolean {
  const retry = queryClient.getDefaultOptions().mutations?.retry
  if (typeof retry !== "function") throw new Error("se esperaba una función de reintento")
  return retry(intentosFallidos, error) as boolean
}

describe("reintentos del queryClient", () => {
  it("no reintenta un error del cliente: no va a mejorar solo", () => {
    for (const status of [400, 401, 403, 404, 409, 422]) {
      expect(reintentaConsulta(new ApiError(status, "no"))).toBe(false)
    }
  })

  it("sí reintenta lo que puede mejorar solo", () => {
    // Un 502 del proxy mientras el backend se reinicia, o un 500 puntual.
    expect(reintentaConsulta(new ApiError(502, "bad gateway"))).toBe(true)
    expect(reintentaConsulta(new ApiError(500, "error interno"))).toBe(true)
    // Un corte de red no llega como ApiError.
    expect(reintentaConsulta(new TypeError("Failed to fetch"))).toBe(true)
  })

  it("deja de reintentar al tercer intento fallido", () => {
    expect(reintentaConsulta(new ApiError(500, "x"), 2)).toBe(true)
    expect(reintentaConsulta(new ApiError(500, "x"), 3)).toBe(false)
  })

  // Una mutación es una acción que alguien apretó: el cartel al lado del botón
  // tiene que aparecer cuando lo aprieta.
  it("una mutación no reintenta un error del cliente y reintenta una sola vez lo demás", () => {
    expect(reintentaMutacion(new ApiError(409, "ya existe"))).toBe(false)
    expect(reintentaMutacion(new ApiError(503, "no disponible"))).toBe(true)
    expect(reintentaMutacion(new ApiError(503, "no disponible"), 1)).toBe(false)
  })
})
