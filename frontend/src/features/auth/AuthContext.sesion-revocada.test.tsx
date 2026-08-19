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

/** Simula la respuesta del backend para el próximo apiFetch. */
function responderCon(status: number, cuerpo: string) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(cuerpo, {
      status,
      headers: { "content-type": "text/plain" },
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
      "tu sesión se cerró porque la contraseña de esta cuenta cambió; volvé a entrar"
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
 * Volver a abrir la aplicación con el token vencido no es un problema que
 * haya que explicarle a nadie: es lo que pasa siempre. Antes, ese 401 del
 * GET /me del arranque disparaba el mismo aviso que una expulsión en medio
 * del trabajo, y quien abría el sistema en el teléfono se encontraba con un
 * cartel de "token inválido o expirado" que se lee como un error del
 * sistema.
 */
describe("token vencido al abrir la aplicación", () => {
  it("no muestra ningún motivo: va derecho a la pantalla de ingreso", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("un-token-vencido")
    // El GET /me tiene que pasar por el cliente HTTP de verdad: es ahí donde
    // un 401 con token dispara el manejador global de sesión rechazada, que
    // es justamente lo que este test verifica. Con `me` devolviendo un error
    // ya armado, ese camino no se recorre y el test no probaría nada.
    responderCon(401, "token inválido o expirado")
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

  it("pero si la sesión estaba abierta y la revocan, sí dice por qué", async () => {
    await montarConSesion()
    responderCon(401, "tu sesión se cerró porque la contraseña de esta cuenta cambió; volvé a entrar")

    await apiFetch("/api/reservation/reservas").catch(() => {})

    await waitFor(() =>
      expect(screen.getByTestId("motivo")).toHaveTextContent(/la contraseña de esta cuenta cambió/)
    )
  })
})
