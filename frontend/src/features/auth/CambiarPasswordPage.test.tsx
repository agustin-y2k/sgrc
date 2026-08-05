import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import * as authApi from "@/features/auth/api"
import { useAuth } from "@/features/auth/AuthContext"
import { CambiarPasswordPage } from "@/features/auth/CambiarPasswordPage"
import { ApiError } from "@/lib/api-client"
import { getToken, setToken } from "@/lib/token-store"

vi.mock("@/features/auth/AuthContext")
vi.mock("@/features/auth/api")

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/cambiar-password"]}>
      <Routes>
        <Route path="/cambiar-password" element={<CambiarPasswordPage />} />
        <Route path="/" element={<div>Home</div>} />
      </Routes>
    </MemoryRouter>
  )
}

async function completarYEnviar() {
  const user = userEvent.setup()
  await user.type(screen.getByLabelText("Contraseña actual"), "temporal123")
  await user.type(screen.getByLabelText("Contraseña nueva"), "unapasswordlarga")
  await user.click(screen.getByRole("button", { name: "Cambiar contraseña" }))
}

describe("CambiarPasswordPage", () => {
  beforeEach(() => {
    // restoreAllMocks no resetea los contadores de un vi.mock de módulo:
    // sin esto, las llamadas se acumulan entre tests y el que verifica
    // "no llamó al backend" ve las de los anteriores.
    vi.clearAllMocks()
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  // RF-01.6: el token con el que se llega acá lleva debeCambiarPassword=true
  // congelado en los claims, y el backend responde 403 a todo lo demás
  // mientras siga así. Si no se guardara el token nuevo, quien acaba de
  // cambiar la contraseña quedaría bloqueado por su propio cambio exitoso.
  it("reemplaza el token con el que devuelve el backend", async () => {
    setToken("token-viejo-con-password-vencida")
    vi.mocked(authApi.cambiarPassword).mockResolvedValue({ token: "token-nuevo" })

    renderPage()
    await completarYEnviar()

    await waitFor(() => {
      expect(getToken()).toBe("token-nuevo")
    })
  })

  it("navega al inicio después de cambiarla", async () => {
    vi.mocked(authApi.cambiarPassword).mockResolvedValue({ token: "token-nuevo" })

    renderPage()
    await completarYEnviar()

    expect(await screen.findByText("Home")).toBeInTheDocument()
  })

  it("refresca el usuario para que ProtectedRoute deje de redirigir acá", async () => {
    const refetchUser = vi.fn()
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      errorDeSesion: null,
      refetchUser,
    })
    vi.mocked(authApi.cambiarPassword).mockResolvedValue({ token: "token-nuevo" })

    renderPage()
    await completarYEnviar()

    await waitFor(() => {
      expect(refetchUser).toHaveBeenCalled()
    })
  })

  it("muestra el error del backend y no toca el token si falla", async () => {
    setToken("token-viejo")
    vi.mocked(authApi.cambiarPassword).mockRejectedValue(
      new ApiError(401, "credenciales inválidas")
    )

    renderPage()
    await completarYEnviar()

    expect(await screen.findByText("credenciales inválidas")).toBeInTheDocument()
    expect(getToken()).toBe("token-viejo")
  })

  it("exige al menos 8 caracteres antes de llamar al backend", async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByLabelText("Contraseña actual"), "temporal123")
    await user.type(screen.getByLabelText("Contraseña nueva"), "corta")
    await user.click(screen.getByRole("button", { name: "Cambiar contraseña" }))

    expect(await screen.findByText("Mínimo 8 caracteres")).toBeInTheDocument()
    expect(authApi.cambiarPassword).not.toHaveBeenCalled()
  })
})
