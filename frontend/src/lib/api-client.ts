import { getToken } from "@/lib/token-store"

// El backend no tiene un ErrorHandler custom en fiber.New() (ver cmd/main.go)
// — usa el default de Fiber v2, que es
// c.Status(code).SendString(err.Error()): texto plano (Content-Type:
// text/plain), el body ES el mensaje directamente.
export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

type ApiFetchOptions = Omit<RequestInit, "body"> & {
  body?: unknown
}

/** Qué hacer cuando el backend rechaza el token que mandamos. */
type ManejadorDeSesionRechazada = (mensaje: string) => void

let alRechazarLaSesion: ManejadorDeSesionRechazada | null = null

/** Devuelve la función para desregistrarse (útil en el cleanup de un efecto). */
export function registrarManejadorDeSesionRechazada(
  manejador: ManejadorDeSesionRechazada
): () => void {
  alRechazarLaSesion = manejador
  return () => {
    if (alRechazarLaSesion === manejador) alRechazarLaSesion = null
  }
}

async function parseErrorMessage(response: Response): Promise<string> {
  const text = await response.text()
  return text.trim() || response.statusText || "error inesperado"
}

export async function apiFetch<T>(
  path: string,
  { body, headers, ...options }: ApiFetchOptions = {}
): Promise<T> {
  const baseUrl = import.meta.env.VITE_API_URL ?? ""
  const token = getToken()

  const response = await fetch(`${baseUrl}${path}`, {
    ...options,
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (!response.ok) {
    const error = new ApiError(response.status, await parseErrorMessage(response))
    // Solo si HABÍA token: el 401 del login son credenciales equivocadas,
    // no una sesión rechazada, y cerrar sesión ahí no tendría sentido.
    if (response.status === 401 && token) {
      alRechazarLaSesion?.(error.message)
    }
    throw error
  }

  // Varios endpoints responden con c.SendStatus() y sin body JSON (ej.
  const contentType = response.headers.get("content-type") ?? ""
  if (response.status === 204 || !contentType.includes("application/json")) {
    return undefined as T
  }

  return (await response.json()) as T
}

// El backend ya manda el mensaje correcto para cada caso de negocio (ej.
export function getErrorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "Ocurrió un error inesperado"
}
