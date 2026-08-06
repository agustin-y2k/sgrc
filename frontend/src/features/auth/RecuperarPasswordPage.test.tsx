import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"

import * as authApi from "@/features/auth/api"
import { RecuperarPasswordPage } from "@/features/auth/RecuperarPasswordPage"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/api")

// Muestra el aviso con el que se vuelve al login, que es la única señal de
// que el cambio salió bien (el endpoint responde 204, sin body ni token).
function Login() {
  const location = useLocation()
  const state = location.state as { aviso?: string } | null
  return <div>Login. Aviso: {state?.aviso ?? "(ninguno)"}</div>
}

function renderPagina() {
  render(
    <MemoryRouter initialEntries={["/recuperar-password"]}>
      <Routes>
        <Route path="/recuperar-password" element={<RecuperarPasswordPage />} />
        <Route path="/login" element={<Login />} />
      </Routes>
    </MemoryRouter>
  )
}

/** Completa el paso 1 y deja la pantalla en el formulario del código. */
async function pedirCodigo(
  user: ReturnType<typeof userEvent.setup>,
  email = "ana@escuela.edu.ar"
) {
  await user.type(screen.getByLabelText("Email"), email)
  await user.click(screen.getByRole("button", { name: "Enviarme el código" }))
  await screen.findByLabelText("Código")
}

describe("RecuperarPasswordPage", () => {
  beforeEach(() => {
    vi.mocked(authApi.olvidePassword).mockResolvedValue({ mensaje: "ok" })
    vi.mocked(authApi.restablecerPassword).mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("valida el formato del email antes de pedir nada", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.type(screen.getByLabelText("Email"), "no-es-un-email")
    await user.click(screen.getByRole("button", { name: "Enviarme el código" }))

    expect(await screen.findByText("Ingresá un email válido")).toBeInTheDocument()
    expect(authApi.olvidePassword).not.toHaveBeenCalled()
  })

  it("pasa al paso del código con el email que se escribió", async () => {
    const user = userEvent.setup()
    renderPagina()

    await pedirCodigo(user)

    expect(authApi.olvidePassword).toHaveBeenCalledWith({ email: "ana@escuela.edu.ar" })
    expect(screen.getByText(/ana@escuela\.edu\.ar/)).toBeInTheDocument()
  })

  it("avanza igual con un email que no existe", async () => {
    // El backend responde 202 exista o no la cuenta, y la pantalla NO puede
    // desmentirlo: decir "ese email no está registrado" convertiría el
    // formulario en un padrón de los docentes de la escuela.
    const user = userEvent.setup()
    renderPagina()

    await pedirCodigo(user, "nadie@escuela.edu.ar")

    expect(screen.getByLabelText("Código")).toBeInTheDocument()
    expect(screen.queryByText(/no está registrado/i)).not.toBeInTheDocument()
  })

  it("exige que el código sean 6 números", async () => {
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.type(screen.getByLabelText("Código"), "12ab")
    await user.type(screen.getByLabelText("Contraseña nueva"), "contraseña-larga")
    await user.type(screen.getByLabelText("Repetir contraseña"), "contraseña-larga")
    await user.click(screen.getByRole("button", { name: "Cambiar mi contraseña" }))

    expect(await screen.findByText("El código son 6 números")).toBeInTheDocument()
    expect(authApi.restablecerPassword).not.toHaveBeenCalled()
  })

  it("no manda nada si las dos contraseñas no coinciden", async () => {
    // Acá la persona elige a ciegas y con un código que se consume: un
    // carácter de más la dejaría afuera y con el código gastado.
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.type(screen.getByLabelText("Código"), "482913")
    await user.type(screen.getByLabelText("Contraseña nueva"), "contraseña-larga")
    await user.type(screen.getByLabelText("Repetir contraseña"), "contraseña-larga-x")
    await user.click(screen.getByRole("button", { name: "Cambiar mi contraseña" }))

    expect(await screen.findByText("Las contraseñas no coinciden")).toBeInTheDocument()
    expect(authApi.restablecerPassword).not.toHaveBeenCalled()
  })

  it("exige el mínimo de 8 caracteres", async () => {
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.type(screen.getByLabelText("Código"), "482913")
    await user.type(screen.getByLabelText("Contraseña nueva"), "corta")
    await user.type(screen.getByLabelText("Repetir contraseña"), "corta")
    await user.click(screen.getByRole("button", { name: "Cambiar mi contraseña" }))

    expect(await screen.findByText("Mínimo 8 caracteres")).toBeInTheDocument()
    expect(authApi.restablecerPassword).not.toHaveBeenCalled()
  })

  it("cambia la contraseña y vuelve al login con el aviso", async () => {
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.type(screen.getByLabelText("Código"), "482913")
    await user.type(screen.getByLabelText("Contraseña nueva"), "contraseña-larga")
    await user.type(screen.getByLabelText("Repetir contraseña"), "contraseña-larga")
    await user.click(screen.getByRole("button", { name: "Cambiar mi contraseña" }))

    await waitFor(() =>
      expect(authApi.restablecerPassword).toHaveBeenCalledWith({
        email: "ana@escuela.edu.ar",
        codigo: "482913",
        passwordNueva: "contraseña-larga",
      })
    )
    expect(
      await screen.findByText(/Ya podés entrar con tu contraseña nueva/)
    ).toBeInTheDocument()
  })

  it("muestra el mensaje del backend cuando el código no sirve", async () => {
    vi.mocked(authApi.restablecerPassword).mockRejectedValue(
      new ApiError(400, "el código venció; pedí uno nuevo")
    )
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.type(screen.getByLabelText("Código"), "482913")
    await user.type(screen.getByLabelText("Contraseña nueva"), "contraseña-larga")
    await user.type(screen.getByLabelText("Repetir contraseña"), "contraseña-larga")
    await user.click(screen.getByRole("button", { name: "Cambiar mi contraseña" }))

    // El texto viene del backend tal cual: distingue "venció" de "está mal",
    // que es lo que le dice a la persona si tiene que pedir otro o retipear.
    expect(
      await screen.findByText("el código venció; pedí uno nuevo")
    ).toBeInTheDocument()
    // Y se queda en el paso 2 para poder reintentar.
    expect(screen.getByLabelText("Código")).toBeInTheDocument()
  })

  it("permite volver atrás para usar otro email o pedir otro código", async () => {
    const user = userEvent.setup()
    renderPagina()
    await pedirCodigo(user)

    await user.click(
      screen.getByRole("button", { name: "Usar otro email o pedir un código nuevo" })
    )

    expect(await screen.findByLabelText("Email")).toBeInTheDocument()
    expect(screen.queryByLabelText("Código")).not.toBeInTheDocument()
  })

  it("avisa si el despliegue no tiene correo configurado", async () => {
    vi.mocked(authApi.olvidePassword).mockRejectedValue(
      new ApiError(503, "la recuperación de contraseña por email no está configurada")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.type(screen.getByLabelText("Email"), "ana@escuela.edu.ar")
    await user.click(screen.getByRole("button", { name: "Enviarme el código" }))

    expect(await screen.findByText(/no está configurada/)).toBeInTheDocument()
    // No avanza al paso 2: no hay ningún código en camino.
    expect(screen.queryByLabelText("Código")).not.toBeInTheDocument()
  })
})
