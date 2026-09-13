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

async function llenarFormulario(
  user: ReturnType<typeof userEvent.setup>,
  cargo: RegExp = /^Docente/
) {
  await user.type(screen.getByLabelText("Nombre"), "Ana")
  await user.type(screen.getByLabelText("Apellido"), "Docente")
  await user.type(screen.getByLabelText("Email"), "ana@test.com")
  await user.type(screen.getByLabelText("Contraseña"), "password123")
  // El cargo y el rol son obligatorios desde RF-01.3, para los dos cargos.
  await user.click(screen.getByRole("radio", { name: cargo }))
  await user.selectOptions(screen.getByLabelText("¿Sos titular o suplente?"), "TITULAR")

  // Y para un DOCENTE, también el curso y la materia. El campo de materia
  // aparece recién con el curso puesto, así que el orden no es decorativo.
  if (String(cargo).includes("Docente")) {
    await user.type(screen.getByLabelText("Curso o lugar donde trabajás"), "5°A")
    await user.type(screen.getByLabelText("Materia"), "Programación")
  }
}

describe("RegistroPage", () => {
  beforeEach(() => {
    // AvisoDeSpam (RF-05.8) pregunta el remitente al montarse.
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "" })
    // El formulario ofrece los cursos y materias que existen (RF-01.3); sin
    // lista, los campos siguen siendo de texto libre.
    vi.mocked(authApi.opcionesDeRegistro).mockResolvedValue({ lugares: [] })
  })

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

  // ── El cargo declarado (RF-01.3) ──────────────────────────────────────

  // Es lo único que el registro sumó como obligatorio, junto con el rol.
  it("sin elegir cargo ni rol, no manda nada y dice qué falta", async () => {
    const user = userEvent.setup()
    renderRegistroPage()

    await user.type(screen.getByLabelText("Nombre"), "Ana")
    await user.type(screen.getByLabelText("Apellido"), "Docente")
    await user.type(screen.getByLabelText("Email"), "ana@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(
      await screen.findByText("Elegí con qué cargo te registrás")
    ).toBeInTheDocument()
    expect(screen.getByText("Elegí si sos titular o suplente")).toBeInTheDocument()
    expect(authApi.registrar).not.toHaveBeenCalled()
  })

  // Quien administra el laboratorio sin dar clase no tiene qué contestar en
  // "¿qué vas a dictar?": ese bloque directamente no se dibuja.
  it("como administrador de sistema, no pregunta qué va a dictar", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user, /^Administrador de Sistema/)

    expect(screen.queryByText("¿Qué vas a dictar?")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("Materia")).not.toBeInTheDocument()
    // Y se le explica cómo pedirlas si además dicta.
    expect(screen.getByText(/podés pedirlas desde tu perfil/i)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await screen.findByText("Cuenta creada")
    expect(authApi.registrar).toHaveBeenCalledWith(
      expect.objectContaining({
        cargoSolicitado: "ADMIN_SISTEMA",
        rolSolicitado: "TITULAR",
        cursoSolicitado: undefined,
        materiaSolicitada: undefined,
      })
    )
  })

  // Cambiar de tarjeta después de haber escrito la materia no puede dejarla
  // viajando escondida.
  it("lo escrito como docente no viaja si después elige el otro cargo", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    await user.click(screen.getByRole("radio", { name: /^Administrador de Sistema/ }))
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await screen.findByText("Cuenta creada")
    expect(authApi.registrar).toHaveBeenCalledWith(
      expect.objectContaining({ materiaSolicitada: undefined })
    )
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
      cargoSolicitado: "DOCENTE",
      rolSolicitado: "TITULAR",
      // Los dos viajan y no pueden faltar: para un DOCENTE son obligatorios,
      // porque es lo único que el Admin mira para saber a qué asignarlo.
      cursoSolicitado: "5°A",
      materiaSolicitada: "Programación",
    })
  })

  // RF-01.3 + RF-02.6: al aprobar, el Admin necesita saber a qué asignarlo.
  // Sin esto tenía que preguntárselo por fuera del sistema.
  it("manda el curso y la materia que el docente quiere dictar", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user)
    // El curso viaja tal como se escribió: no tiene formato, porque cada
    // institución nombra los suyos como los nombra (RF-02.2).
    await user.type(screen.getByLabelText("Curso o lugar donde trabajás"), "5°A")
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

  // El curso y la materia siguen siendo opcionales: quien todavía no sabe qué
  // va a dictar se registra igual.
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

  // La regla nueva: un docente no se registra sin decir dónde va a estar y qué
  // dicta. Antes era opcional y llegaban cuentas que el Admin no sabía a qué
  // asignar.
  it("no deja registrarse como docente sin curso ni materia", async () => {
    // El mock es del módulo y acumula las llamadas de los tests anteriores:
    // sin esto, "no se llamó" mide toda la suite y no este caso.
    vi.mocked(authApi.registrar).mockClear()
    const user = userEvent.setup()
    renderRegistroPage()

    await user.type(screen.getByLabelText("Nombre"), "Ana")
    await user.type(screen.getByLabelText("Apellido"), "Docente")
    await user.type(screen.getByLabelText("Email"), "ana@test.com")
    await user.type(screen.getByLabelText("Contraseña"), "password123")
    await user.click(screen.getByRole("radio", { name: /^Docente/ }))
    await user.selectOptions(screen.getByLabelText("¿Sos titular o suplente?"), "TITULAR")
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText(/Elegí dónde vas a estar/)).toBeInTheDocument()
    expect(authApi.registrar).not.toHaveBeenCalled()
  })

  // Pero un administrador de sistema sí: no da clase, así que no se le piden.
  it("un administrador de sistema se registra sin curso ni materia", async () => {
    vi.mocked(authApi.registrar).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderRegistroPage()

    await llenarFormulario(user, /^Administrador de Sistema/)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("Cuenta creada")).toBeInTheDocument()
    expect(authApi.registrar).toHaveBeenCalledWith(
      expect.objectContaining({
        cargoSolicitado: "ADMIN_SISTEMA",
        cursoSolicitado: undefined,
        materiaSolicitada: undefined,
      })
    )
  })
})
