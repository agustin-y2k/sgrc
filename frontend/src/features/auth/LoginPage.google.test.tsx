import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import { LoginPage } from "@/features/auth/LoginPage"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/AuthContext")

/**
 * El botón real lo dibuja Google dentro de un iframe, imposible de apretar
 * desde jsdom.
 */
vi.mock("@/features/auth/BotonGoogle", () => ({
  // Los children son la casilla de mantener la sesión: el componente real la
  // dibuja adentro para que se esconda junto con el botón, así que el doble
  // tiene que hacer lo mismo o los tests de la casilla no verían nada.
  BotonGoogle: ({
    onCredential,
    children,
  }: {
    onCredential: (c: string) => void
    children?: React.ReactNode
  }) => (
    <>
      {children}
      <button type="button" onClick={() => onCredential("el-id-token")}>
        Entrar con Google
      </button>
    </>
  ),
}))

// Muestra a qué ruta se navegó y con qué estado, que es justo lo que hay
// que verificar en el camino "todavía no tenés cuenta".
function Registro() {
  const location = useLocation()
  const state = location.state as { credencialDeGoogle?: string } | null
  return <div>Registro con credencial: {state?.credencialDeGoogle ?? "(ninguna)"}</div>
}

function renderLoginPage(state?: { from: { pathname: string } }) {
  render(
    <MemoryRouter initialEntries={[{ pathname: "/login", state }]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<div>Home</div>} />
        <Route path="/cambiar-password" element={<div>Cambiar password</div>} />
        <Route path="/registro" element={<Registro />} />
        <Route path="/reservas" element={<div>Mis reservas</div>} />
      </Routes>
    </MemoryRouter>
  )
}

// La firma explícita evita que vi.fn() se infiera como "cualquier función":
// sin esto, un test podría resolver con una forma que loginConGoogle nunca
// devuelve y pasaría igual.
type Ingreso = (
  credential: string,
  recordarme?: boolean
) => Promise<{ debeCambiarPassword: boolean }>

function mockAuth(loginConGoogle: Ingreso) {
  vi.mocked(useAuth).mockReturnValue({
    user: null,
    isLoading: false,
    login: vi.fn(),
    loginConGoogle,
    logout: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
  })
}

describe("LoginPage — ingreso con Google", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("con cuenta aprobada, entra y va al home", async () => {
    mockAuth(vi.fn<Ingreso>().mockResolvedValue({ debeCambiarPassword: false }))
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    expect(await screen.findByText("Home")).toBeInTheDocument()
  })

  // Hay dos casillas, una por camino de ingreso. La que manda acá es la que
  // está pegada al botón de Google.
  it("respeta la casilla de mantener la sesión iniciada de Google", async () => {
    const loginConGoogle = vi
      .fn<Ingreso>()
      .mockResolvedValue({ debeCambiarPassword: false })
    mockAuth(loginConGoogle)
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByLabelText("Mantener la sesión iniciada con Google"))
    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    await waitFor(() => expect(loginConGoogle).toHaveBeenCalledWith("el-id-token", true))
  })

  // Las dos casillas son excluyentes: marcar una desmarca la otra. Lo que se
  // afirma acá es la consecuencia, que es la que se puede sentir — con la de
  // contraseña marcada, entrar con Google da la sesión CORTA.
  it("la casilla de contraseña no alarga la sesión de Google", async () => {
    const loginConGoogle = vi
      .fn<Ingreso>()
      .mockResolvedValue({ debeCambiarPassword: false })
    mockAuth(loginConGoogle)
    const user = userEvent.setup()
    renderLoginPage()

    const deContraseña = screen.getByLabelText("Mantener la sesión iniciada")
    const deGoogle = screen.getByLabelText("Mantener la sesión iniciada con Google")

    await user.click(deGoogle)
    expect(deGoogle).toBeChecked()

    await user.click(deContraseña)
    expect(deContraseña).toBeChecked()
    expect(deGoogle).not.toBeChecked()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))
    await waitFor(() => expect(loginConGoogle).toHaveBeenCalledWith("el-id-token", false))
  })

  // <ProtectedRoute> guarda la ruta que se quiso abrir sin sesión; entrar
  // con Google tiene que respetarla igual que el login con contraseña.
  it("vuelve a la ruta que se quiso abrir sin sesión", async () => {
    mockAuth(vi.fn<Ingreso>().mockResolvedValue({ debeCambiarPassword: false }))
    const user = userEvent.setup()
    renderLoginPage({ from: { pathname: "/reservas" } })

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    expect(await screen.findByText("Mis reservas")).toBeInTheDocument()
  })

  // Puede pasar en una cuenta que entra de las dos formas y a la que un
  // Admin le reseteó la contraseña (RF-01.6).
  it("con debeCambiarPassword, manda a cambiar la contraseña", async () => {
    mockAuth(vi.fn<Ingreso>().mockResolvedValue({ debeCambiarPassword: true }))
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    expect(await screen.findByText("Cambiar password")).toBeInTheDocument()
  })

  // El 404 no es un error a mostrar: significa "el token está bien, la cuenta
  // todavía no existe".
  it("sin cuenta todavía, lleva al registro con la credencial", async () => {
    mockAuth(
      vi
        .fn<Ingreso>()
        .mockRejectedValue(new ApiError(404, "todavía no hay ninguna cuenta"))
    )
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    expect(
      await screen.findByText("Registro con credencial: el-id-token")
    ).toBeInTheDocument()
  })

  it("con la cuenta pendiente de aprobación, muestra el mensaje del backend", async () => {
    mockAuth(
      vi
        .fn<Ingreso>()
        .mockRejectedValue(
          new ApiError(
            403,
            "tu cuenta todavía está esperando la aprobación de un Admin — vas a poder entrar apenas la aprueben"
          )
        )
    )
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    expect(
      await screen.findByText(/esperando la aprobación de un Admin/)
    ).toBeInTheDocument()
  })

  it("con el ingreso con Google no configurado, muestra el mensaje del backend", async () => {
    mockAuth(
      vi
        .fn<Ingreso>()
        .mockRejectedValue(
          new ApiError(503, "el ingreso con Google no está configurado en este sistema")
        )
    )
    const user = userEvent.setup()
    renderLoginPage()

    await user.click(screen.getByRole("button", { name: "Entrar con Google" }))

    await waitFor(() =>
      expect(
        screen.getByText("el ingreso con Google no está configurado en este sistema")
      ).toBeInTheDocument()
    )
  })
})
