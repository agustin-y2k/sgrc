import { getToken } from "@/lib/token-store"

// El backend no tiene un ErrorHandler custom en fiber.New() (ver cmd/main.go)
// — usa el default de Fiber v2, que es
// c.Status(code).SendString(err.Error()): texto plano (Content-Type:
// text/plain), el body ES el mensaje directamente.
export class ApiError extends Error {
  status: number
  /**
   * El cuerpo del rechazo, ya parseado, cuando vino como JSON.
   *
   * Casi todos los errores del backend son una frase y alcanza con `message`.
   * Unos pocos traen además el detalle de por qué no se pudo —qué reservas
   * quedarían fuera de la jornada nueva, por ejemplo— y la pantalla necesita
   * mostrarlo, no solo decir que algo falló.
   */
  cuerpo?: unknown

  constructor(status: number, message: string, cuerpo?: unknown) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.cuerpo = cuerpo
  }
}

type ApiFetchOptions = Omit<RequestInit, "body"> & {
  body?: unknown
}

/**
 * Por qué el backend rechazó el token, tal como lo manda en el header
 * `X-Sesion-Motivo` (ver internal/shared/middleware/jwt.go). No se decide por
 * el texto del mensaje: ese texto es para leer, y atarle lógica haría que
 * cualquier retoque de redacción rompiera el comportamiento sin que nadie se
 * entere.
 */
export type MotivoDeRechazo =
  | "expirada"
  | "invalida"
  | "revocada"
  | "password-cambiada"
  /** El backend no mandó el header (versión anterior, o un proxy lo comió). */
  | "desconocido"

const MOTIVOS_CONOCIDOS = new Set<MotivoDeRechazo>([
  "expirada",
  "invalida",
  "revocada",
  "password-cambiada",
])

function motivoDeRechazo(response: Response): MotivoDeRechazo {
  const crudo = response.headers.get("X-Sesion-Motivo") ?? ""
  return MOTIVOS_CONOCIDOS.has(crudo as MotivoDeRechazo)
    ? (crudo as MotivoDeRechazo)
    : "desconocido"
}

/** Qué hacer cuando el backend rechaza el token que mandamos. */
type ManejadorDeSesionRechazada = (mensaje: string, motivo: MotivoDeRechazo) => void

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

/**
 * El mensaje y, si vino, el detalle estructurado.
 *
 * La mayoría de los rechazos son texto plano —Fiber devuelve así los
 * fiber.NewError— y ese texto ES el mensaje. Unos pocos responden un objeto
 * JSON con `error` adentro: sin este parseo, la pantalla le mostraría al
 * usuario el JSON crudo como si fuera una frase.
 */
async function parseErrorBody(
  response: Response
): Promise<{ mensaje: string; cuerpo?: unknown }> {
  const text = await response.text()
  const porDefecto = text.trim() || response.statusText || "error inesperado"

  if (!(response.headers.get("content-type") ?? "").includes("application/json")) {
    return { mensaje: porDefecto }
  }
  try {
    const cuerpo: unknown = JSON.parse(text)
    if (typeof cuerpo === "object" && cuerpo !== null && "error" in cuerpo) {
      const { error } = cuerpo as { error: unknown }
      if (typeof error === "string" && error !== "") {
        return { mensaje: error, cuerpo }
      }
    }
    return { mensaje: porDefecto, cuerpo }
  } catch {
    // Un JSON mal formado no es motivo para perder el error original.
    return { mensaje: porDefecto }
  }
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
    const { mensaje, cuerpo } = await parseErrorBody(response)
    const error = new ApiError(response.status, mensaje, cuerpo)
    // Solo si HABÍA token: el 401 del login son credenciales equivocadas,
    // no una sesión rechazada, y cerrar sesión ahí no tendría sentido.
    if (response.status === 401 && token) {
      alRechazarLaSesion?.(error.message, motivoDeRechazo(response))
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
