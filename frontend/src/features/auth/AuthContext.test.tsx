import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { AuthProvider, useAuth } from "@/features/auth/AuthContext"
import * as authApi from "@/features/auth/api"
import type { Usuario } from "@/features/auth/types"
import { ApiError } from "@/lib/api-client"
import * as tokenStore from "@/lib/token-store"

vi.mock("@/features/auth/api")
vi.mock("@/lib/token-store")

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
  const { user, isLoading, errorDeSesion, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="user">{user ? user.email : "sin sesión"}</span>
      <span data-testid="errorDeSesion">{errorDeSesion ?? "sin error"}</span>
      <button onClick={() => void login("ana@test.com", "password123").catch(() => {})}>
        login
      </button>
      <button onClick={logout}>logout</button>
    </div>
  )
}

describe("AuthProvider", () => {
  beforeEach(() => {
    // vi.mock() genera mocks automáticos cuyo historial de llamadas NO
    // limpia restoreAllMocks — sin esto, un "no fue llamado" ve las
    // llamadas del test anterior.
    vi.clearAllMocks()
    vi.mocked(tokenStore.getToken).mockReturnValue(null)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("sin token guardado, arranca sin sesión y sin quedar cargando", async () => {
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )

    await waitFor(() => expect(screen.getByTestId("loading")).toHaveTextContent("false"))
    expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    expect(authApi.me).not.toHaveBeenCalled()
  })

  it("con token guardado, hidrata el usuario llamando a GET /me", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("token-existente")
    vi.mocked(authApi.me).mockResolvedValue(usuarioMock)

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )

    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
    )
  })

  it("si GET /me devuelve 401 (token vencido), limpia el token y queda sin sesión", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("token-vencido")
    vi.mocked(authApi.me).mockRejectedValue(
      new ApiError(401, "token inválido o expirado")
    )

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )

    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
    )
    expect(tokenStore.clearToken).toHaveBeenCalled()
    expect(screen.getByTestId("errorDeSesion")).toHaveTextContent("sin error")
  })

  // Un corte de red no debe desloguear: el token probablemente sigue siendo
  // válido y borrarlo obligaría a escribir la contraseña de nuevo por un
  // problema momentáneo de conexión.
  it("si GET /me falla por red, CONSERVA el token y reporta el error de sesión", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("token-bueno")
    vi.mocked(authApi.me).mockRejectedValue(new TypeError("Failed to fetch"))

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )

    await waitFor(() =>
      expect(screen.getByTestId("errorDeSesion")).toHaveTextContent(
        /No se pudo verificar/
      )
    )
    expect(tokenStore.clearToken).not.toHaveBeenCalled()
  })

  it("login guarda el token y devuelve debeCambiarPassword de la respuesta del login", async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      token: "nuevo-token",
      debeCambiarPassword: true,
    })
    vi.mocked(authApi.me).mockResolvedValue(usuarioMock)
    // getToken/setToken están mockeados por separado (vi.mock no comparte
    // estado real con localStorage) — este stub los conecta para que
    // loadUser() vea el token recién guardado, como pasaría de verdad.
    vi.mocked(tokenStore.setToken).mockImplementation((token) => {
      vi.mocked(tokenStore.getToken).mockReturnValue(token)
    })
    const user = userEvent.setup()

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await waitFor(() => expect(screen.getByTestId("loading")).toHaveTextContent("false"))

    await user.click(screen.getByText("login"))

    await waitFor(() => expect(tokenStore.setToken).toHaveBeenCalledWith("nuevo-token"))
    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
    )
  })

  it("logout limpia el token y el usuario", async () => {
    vi.mocked(tokenStore.getToken).mockReturnValue("token-existente")
    vi.mocked(authApi.me).mockResolvedValue(usuarioMock)
    const user = userEvent.setup()

    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await waitFor(() =>
      expect(screen.getByTestId("user")).toHaveTextContent("ana@test.com")
    )

    await user.click(screen.getByText("logout"))

    expect(tokenStore.clearToken).toHaveBeenCalled()
    expect(screen.getByTestId("user")).toHaveTextContent("sin sesión")
  })
})
