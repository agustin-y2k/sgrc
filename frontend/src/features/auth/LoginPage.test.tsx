import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { LoginPage } from "@/features/auth/LoginPage"
import { useAuth } from "@/features/auth/AuthContext"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/AuthContext")

function renderLoginPage(state?: { from: { pathname: string } }) {
  render(
    <MemoryRouter initialEntries={[{ pathname: "/login", state }]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<div>Home</div>} />
        <Route path="/cambiar-password" element={<div>Cambiar password</div>} />
        <Route path="/registro" element={<div>Registro</div>} />
        <Route path="/admin/aprobacion" element={<div>Aprobación</div>} />
      </Routes>
    </MemoryRouter>
  )
}

describe("LoginPage", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("valida el formato de email antes de llamar a login", async () => {
    const login = vi.fn()
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "no-es-un-email")
    await user.type(screen.getByLabelText("Contraseña"), "algo")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    expect(await screen.findByText("Ingresá un email válido")).toBeInTheDocument()
    expect(login).not.toHaveBeenCalled()
  })

  it("con debeCambiarPassword=false, redirige a home", async () => {
    const login = vi.fn().mockResolvedValue({ debeCambiarPassword: false })
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "admin@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    expect(login).toHaveBeenCalledWith("admin@test.com", "password123")
    await waitFor(() => expect(screen.getByText("Home")).toBeInTheDocument())
  })

  it("con debeCambiarPassword=true, redirige a /cambiar-password", async () => {
    const login = vi.fn().mockResolvedValue({ debeCambiarPassword: true })
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "docente@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "temporal123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    await waitFor(() => expect(screen.getByText("Cambiar password")).toBeInTheDocument())
  })

  it("muestra el mensaje de error tal cual lo manda el backend", async () => {
    const login = vi.fn().mockRejectedValue(new ApiError(401, "credenciales inválidas"))
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "nadie@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "mal")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    expect(await screen.findByText("credenciales inválidas")).toBeInTheDocument()
  })

  it("vuelve a la ruta que el usuario quiso abrir sin sesión", async () => {
    const login = vi.fn().mockResolvedValue({ debeCambiarPassword: false })
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage({ from: { pathname: "/admin/aprobacion" } })

    await user.type(screen.getByLabelText("Email"), "admin@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    await waitFor(() => expect(screen.getByText("Aprobación")).toBeInTheDocument())
  })

  it("el cambio de password forzado gana sobre el destino original (RF-01.6)", async () => {
    const login = vi.fn().mockResolvedValue({ debeCambiarPassword: true })
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage({ from: { pathname: "/admin/aprobacion" } })

    await user.type(screen.getByLabelText("Email"), "docente@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "temporal123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    await waitFor(() => expect(screen.getByText("Cambiar password")).toBeInTheDocument())
  })
})
