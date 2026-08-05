import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { RegistroPage } from "@/features/auth/RegistroPage"
import * as authApi from "@/features/auth/api"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/api")

function renderRegistroPage() {
  render(
    <MemoryRouter initialEntries={["/registro"]}>
      <Routes>
        <Route path="/registro" element={<RegistroPage />} />
        <Route path="/login" element={<div>Login</div>} />
      </Routes>
    </MemoryRouter>
  )
}

async function llenarFormulario(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Nombre"), "Ana")
  await user.type(screen.getByLabelText("Apellido"), "Docente")
  await user.type(screen.getByLabelText("Email"), "ana@test.com")
  await user.type(screen.getByLabelText("Contraseña"), "password123")
}

describe("RegistroPage", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("valida password de al menos 8 caracteres, espejando RegistroRequest del backend", async () => {
    const user = userEvent.setup()
    renderRegistroPage()

    await user.type(screen.getByLabelText("Nombre"), "Ana")
    await user.type(screen.getByLabelText("Apellido"), "Docente")
    await user.type(screen.getByLabelText("Email"), "ana@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "corta")
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("Mínimo 8 caracteres")).toBeInTheDocument()
    expect(authApi.registrar).not.toHaveBeenCalled()
  })

  it("al registrarse con éxito, muestra la pantalla de pendiente de aprobación", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("Cuenta creada")).toBeInTheDocument()
    expect(authApi.registrar).toHaveBeenCalledWith({
      nombre: "Ana",
      apellido: "Docente",
      email: "ana@test.com",
      password: "password123",
      // Sin completar, no viajan: "no lo declaró" y "lo dejó en blanco" no
      // son dos cosas distintas.
      cursoSolicitado: undefined,
      materiaSolicitada: undefined,
    })
  })

  // RF-01.3 + RF-02.6: al aprobar, el Admin necesita saber a qué asignarlo.
  // Sin esto tenía que preguntárselo por fuera del sistema.
  it("manda el curso y la materia que el docente quiere dictar", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    await user.type(screen.getByLabelText("Curso"), "5°A")
    await user.type(screen.getByLabelText("Materia"), "Programación")
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await screen.findByText("Cuenta creada")
    expect(authApi.registrar).toHaveBeenCalledWith(
      expect.objectContaining({
        cursoSolicitado: "5°A",
        materiaSolicitada: "Programación",
      })
    )
  })

  // Son opcionales: quien todavía no sabe qué va a dictar se registra igual.
  it("deja registrarse sin declarar curso ni materia", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("Cuenta creada")).toBeInTheDocument()
  })

  it("muestra el mensaje específico de RF-01.3 cuando el email pertenece a una cuenta en BAJA", async () => {
    vi.mocked(authApi.registrar).mockRejectedValue(
      new ApiError(
        409,
        "este email pertenece a una cuenta dada de baja — pedile a un Admin que la elimine para poder registrarte de nuevo"
      )
    )
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(
      await screen.findByText(/pedile a un Admin que la elimine/)
    ).toBeInTheDocument()
  })
})
