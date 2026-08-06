import { getToken } from "@/lib/token-store"

// El backend no tiene un ErrorHandler custom en fiber.New() (ver
// cmd/main.go) — usa el default de Fiber v2, que es
// c.Status(code).SendString(err.Error()): texto plano
// (Content-Type: text/plain), el body ES el mensaje directamente. NO es
// JSON, ni el {error} que asumíamos al leer el código Go sin correrlo, ni
// el ErrorResponse{error,message,details} de docs/08-api-spec.yaml —
// verificado pegándole al backend real (curl -i), no infiriéndolo del
// código fuente.
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

/**
 * Qué hacer cuando el backend rechaza el token que mandamos.
 *
 * Puede pasar en cualquier request, no solo al arrancar la aplicación: la
 * cuenta se dio de baja (RF-02.8) o alguien cambió su contraseña y eso
 * cerró las sesiones abiertas (RF-01.11). Sin este enganche ese 401 sería
 * un error cualquiera —cartel rojo en la pantalla de turno, la aplicación
 * todavía convencida de que hay sesión— y cada acción siguiente fallaría
 * igual sin que nada llevara al login.
 *
 * Vive acá porque es el único lugar por el que pasan TODOS los requests. Se
 * registra con un callback en vez de importar el contexto para que este
 * módulo siga sin saber nada de React ni de rutas.
 */
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
  // POST /api/auth/registro → 201 con el body "Created" plano, no `{}` —
  // ver internal/auth/interfaces/http/handlers.go Registrar). Parsear eso
  // como JSON tira SyntaxError, así que solo se intenta si el content-type
  // lo declara.
  const contentType = response.headers.get("content-type") ?? ""
  if (response.status === 204 || !contentType.includes("application/json")) {
    return undefined as T
  }

  return (await response.json()) as T
}

// El backend ya manda el mensaje correcto para cada caso de negocio (ej.
// "cuenta en BAJA" vs "email ya registrado", ambos 409 con texto distinto
// — ver internal/auth/interfaces/http/errors.go) — nunca se hardcodea
// texto por status code acá, solo se decide qué mostrar cuando el error
// no vino de la API.
export function getErrorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "Ocurrió un error inesperado"
}
