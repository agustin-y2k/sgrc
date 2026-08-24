import { render, screen, waitFor } from "@testing-library/react"

import { AuthProvider, useAuth } from "@/features/auth/AuthContext"
import * as authApi from "@/features/auth/api"
import type { Usuario } from "@/features/auth/types"
import { apiFetch, ApiError } from "@/lib/api-client"
import * as tokenStore from "@/lib/token-store"

vi.mock("@/features/auth/api")
vi.mock("@/lib/token-store")

// Revocación de sesión desde el backend.

const usuarioMock: Usuario = {
  id: "1",
  nombre: "Ana",
  apellido: "Docente",
  email: "ana@test.com",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: "2026-01-02T00:00:00Z",
  debeCambiarPassword: false,
}

function Probe() {
  const { user, isLoading, motivoDeCierre } = useAuth()
  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="user">{user ? user.email : "sin sesión"}</span>
      <span data-testid="motivo">{motivoDeCierre ?? "sin motivo"}</span>
    </div>
  )
}

/** Monta el provider con una sesión ya abierta. */
async function montarConSesion() {
  vi.mocked(tokenStore.getToken).mockReturnValue("un-token")
  vi.mocked(authApi.me).mockResolvedValue(usuarioMock)

  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>
  )
  await waitFor(() =>
    expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
  )
}

/**
 * Simula la respuesta del backend para el próximo apiFetch.
 *
 * `motivo` es el header `X-Sesion-Motivo` con el que el backend acompaña cada
 * 401 (ver internal/shared/middleware/jwt.go). Omitirlo simula un backend que
 * no lo manda, que es un caso real: un despliegue viejo, o un proxy que filtra
 * headers.
 */
function responderCon(status: number, cuerpo: string, motivo?: string) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(cuerpo, {
      status,
      headers: {
        "content-type": "text/plain",
        ...(motivo ? { "X-Sesion-Motivo": motivo } : {}),
      },
    })
  )
}

describe("sesión revocada por el backend", () => {
  beforeEach(() => {
    // Mismo motivo que en AuthContext.test.tsx: vi.mock() genera mocks
    // automáticos cuyo historial de llamadas NO limpia restoreAllMocks, así
    // que un "no fue llamado" ve las llamadas del test anterior.
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("un 401 en cualquier request cierra la sesión y explica por qué", async () => {
    await montarConSesion()
    responderCon(
      401,
      "tu sesión se cerró porque la contraseña de esta cuenta cambió; volvé a entrar",
      "password-cambiada"
    )

    // Un request cualquiera del sistema, no /me ni el login.
    await expect(apiFetch("/api/reservation/reservas")).rejects.toBeInstanceOf(ApiError)

    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    )
    // El token se borra: si no, <ProtectedRoute> mandaría al login y el
    // login volvería a entrar en bucle con un token que ya no vale.
    expect(tokenStore.clearToken).toHaveBeenCalled()
    expect(screen.getByTestId("motivo")).toHaveTextContent(
      "la contraseña de esta cuenta cambió"
    )
  })

  it("un 403 no cierra la sesión", async () => {
    // 403 es "no tenés permiso para esto", no "tu sesión no vale". Cerrar
    // sesión ahí echaría a un docente por tocar una pantalla de Admin.
    await montarConSesion()
    responderCon(403, "no autorizado")

    await expect(apiFetch("/api/auth/usuarios")).rejects.toBeInstanceOf(ApiError)

    expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
    expect(screen.getByTestId("motivo")).toHaveTextContent("sin motivo")
  })

  it("un 401 sin token guardado no cierra nada", async () => {
    // Es el 401 del login con credenciales equivocadas: no hay sesión que
    // cerrar, y el mensaje lo muestra el propio formulario.
    vi.mocked(tokenStore.getToken).mockReturnValue(null)
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await waitFor(() => expect(screen.getByTestId("loading")).toHaveTextContent("false"))

    responderCon(401, "credenciales inválidas")
    await expect(apiFetch("/api/auth/login", { method: "POST" })).rejects.toBeInstanceOf(
      ApiError
    )

    expect(tokenStore.clearToken).not.toHaveBeenCalled()
    expect(screen.getByTestId("motivo")).toHaveTextContent("sin motivo")
  })

  it("un 500 no cierra la sesión", async () => {
    await montarConSesion()
    responderCon(500, "error interno")

    await expect(apiFetch("/api/reservation/reservas")).rejects.toBeInstanceOf(ApiError)

    expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
  })
})

/**
 * Qué se le cuenta a la persona cuando su sesión deja de valer, y qué no.
 *
 * La regla NO es "¿estaba usando el sistema?" sino "¿por qué la cerraron?".
 * Con la regla vieja, una pestaña abierta toda la noche amanecía con un cartel
 * de "token inválido o expirado" —el vencimiento le pasa a todo el mundo todos
 * los días y no hay nada que explicar—, mientras que una expulsión real pasaba
 * desapercibida si ocurría antes de que la pantalla terminara de cargar.
 */
describe("qué se explica al cerrarse una sesión", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("una sesión ABIERTA que vence no muestra ningún cartel", async () => {
    // El caso de la pestaña que quedó abierta: el vencimiento no es una
    // novedad que valga la pena anunciar.
    await montarConSesion()
    responderCon(401, "la sesión venció", "expirada")

    await apiFetch("/api/reservation/reservas").catch(() => {})

    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    )
    expect(screen.getByTestId("motivo")).toHaveTextContent("sin motivo")
  })

  it("volver a abrir la aplicación con el token vencido tampoco", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("un-token-vencido")
    // El GET /me tiene que pasar por el cliente HTTP de verdad: es ahí donde
    // un 401 con token dispara el manejador global de sesión rechazada, que
    // es justamente lo que este test verifica. Con `me` devolviendo un error
    // ya armado, ese camino no se recorre y el test no probaría nada.
    responderCon(401, "la sesión venció", "expirada")
    vi.mocked(authApi.me).mockImplementation(() => apiFetch("/api/auth/me"))

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )

    await waitFor(() => expect(screen.getByTestId("loading")).toHaveTextContent("false"))
    expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    expect(screen.getByTestId("motivo")).toHaveTextContent("sin motivo")
  })

  it("una contraseña cambiada sí se explica", async () => {
    await montarConSesion()
    responderCon(
      401,
      "tu sesión se cerró porque la contraseña de esta cuenta cambió; volvé a entrar",
      "password-cambiada"
    )

    await apiFetch("/api/reservation/reservas").catch(() => {})

    await waitFor(() =>
      expect(screen.getByTestId("motivo")).toHaveTextContent(
        /la contraseña de esta cuenta cambió/
      )
    )
  })

  it("una cuenta dada de baja sí se explica", async () => {
    // Sin el cartel, quien fue dado de baja reintenta contra un login que le
    // acepta la contraseña y lo vuelve a echar, sin decirle nunca por qué.
    await montarConSesion()
    responderCon(401, "la sesión ya no es válida", "revocada")

    await apiFetch("/api/reservation/reservas").catch(() => {})

    await waitFor(() =>
      expect(screen.getByTestId("motivo")).toHaveTextContent("la sesión ya no es válida")
    )
  })

  it("un backend que no manda el motivo se trata como el caso normal", async () => {
    // Antes que mostrar un cartel que no sabemos si corresponde, no mostrar
    // ninguno: la sesión se cierra igual, que es lo que importa.
    await montarConSesion()
    responderCon(401, "token inválido o expirado")

    await apiFetch("/api/reservation/reservas").catch(() => {})

    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    )
    expect(screen.getByTestId("motivo")).toHaveTextContent("sin motivo")
  })
})
