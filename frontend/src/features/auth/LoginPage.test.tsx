import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import * as authApi from "@/features/auth/api"
import { LoginPage } from "@/features/auth/LoginPage"
import { useAuth } from "@/features/auth/AuthContext"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/AuthContext")
vi.mock("@/features/auth/api")

function renderLoginPage(
  state?: { from: { pathname: string } } | { aviso: string },
  config: { googleClientId: string; recuperacionPorEmail?: boolean } = {
    googleClientId: "",
    recuperacionPorEmail: true,
  }
) {
  vi.mocked(authApi.configPublica).mockResolvedValue(config)
  render(
    <MemoryRouter initialEntries={[{ pathname: "/login", state }]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<div>Home</div>} />
        <Route path="/cambiar-password" element={<div>Cambiar password</div>} />
        <Route path="/registro" element={<div>Registro</div>} />
        <Route path="/recuperar-password" element={<div>Recuperar</div>} />
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
      motivoDeCierre: null,
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
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "admin@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    // El tercer argumento es la casilla "mantener la sesión iniciada": sin
    // tocarla, false.
    expect(login).toHaveBeenCalledWith("admin@test.com", "password123", false)
    await waitFor(() => expect(screen.getByText("Home")).toBeInTheDocument())
  })

  it("la casilla de mantener la sesión arranca destildada", async () => {
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    renderLoginPage()

    // Que esté apagada por omisión es el punto: en una máquina compartida de
    // la escuela, una sesión de un mes tiene que pedirse a propósito.
    expect(screen.getByLabelText("Mantener la sesión iniciada")).not.toBeChecked()
  })

  it("con la casilla tildada, pide la sesión larga", async () => {
    const login = vi.fn().mockResolvedValue({ debeCambiarPassword: false })
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login,
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage()

    await user.type(screen.getByLabelText("Email"), "admin@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByLabelText("Mantener la sesión iniciada"))
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    expect(login).toHaveBeenCalledWith("admin@test.com", "password123", true)
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
      motivoDeCierre: null,
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
      motivoDeCierre: null,
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
      motivoDeCierre: null,
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
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    const user = userEvent.setup()
    renderLoginPage({ from: { pathname: "/admin/aprobacion" } })

    await user.type(screen.getByLabelText("Email"), "docente@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "temporal123")
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }))

    await waitFor(() => expect(screen.getByText("Cambiar password")).toBeInTheDocument())
  })

  // ── Olvidé mi contraseña ──────────────────────────────────────────

  it("ofrece recuperar la contraseña cuando el despliegue tiene correo", async () => {
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    renderLoginPage(undefined, { googleClientId: "", recuperacionPorEmail: true })

    expect(await screen.findByText("Olvidé mi contraseña")).toBeInTheDocument()
  })

  it("no ofrece recuperar la contraseña sin SMTP configurado", async () => {
    // Sin correo el backend responde 503: el enlace llevaría a un callejón
    // sin salida. La salida en ese caso es que un Admin resetee (RF-01.6).
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    renderLoginPage(undefined, { googleClientId: "", recuperacionPorEmail: false })

    // Se espera a que el formulario esté montado para no aprobar el test
    // por haber mirado antes de que la consulta volviera.
    await screen.findByLabelText("Contraseña")
    expect(screen.queryByText("Olvidé mi contraseña")).not.toBeInTheDocument()
  })

  it("muestra el aviso con el que vuelve la pantalla de recuperación", async () => {
    vi.mocked(useAuth).mockReturnValue({
      user: null,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      motivoDeCierre: null,
      refetchUser: vi.fn(),
    })
    renderLoginPage({ aviso: "Listo. Ya podés entrar con tu contraseña nueva." })

    expect(
      await screen.findByText("Listo. Ya podés entrar con tu contraseña nueva.")
    ).toBeInTheDocument()
  })
})
