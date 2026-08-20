import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import * as authApi from "@/features/auth/api"
import { RegistroConGoogle } from "@/features/auth/RegistroConGoogle"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/api")

// Un ID token sin firmar: acá solo se lee el payload para prellenar el
// formulario. Ver datosDeLaCredencial.
function credencialCon(payload: Record<string, unknown>): string {
  const aBase64Url = (obj: unknown) =>
    btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(obj))))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "")
  return [aBase64Url({ alg: "RS256" }), aBase64Url(payload), "firma"].join(".")
}

const credencialDeAda = credencialCon({
  email: "ada@escuela.edu.ar",
  given_name: "Ada",
  family_name: "Lovelace",
})

function renderRegistro(credencial = credencialDeAda, onRegistrado = vi.fn()) {
  render(
    <MemoryRouter>
      <Routes>
        <Route
          path="/"
          element={
            <RegistroConGoogle credencial={credencial} onRegistrado={onRegistrado} />
          }
        />
        <Route path="/login" element={<div>Login</div>} />
      </Routes>
    </MemoryRouter>
  )
  return onRegistrado
}

/** El cargo y el rol son obligatorios desde RF-01.3, para los dos cargos. */
async function declarar(
  user: ReturnType<typeof userEvent.setup>,
  cargo: RegExp = /^Docente/
) {
  await user.click(screen.getByRole("radio", { name: cargo }))
  await user.selectOptions(screen.getByLabelText("¿Sos titular o suplente?"), "TITULAR")
}

describe("RegistroConGoogle", () => {
  afterEach(() => {
    // clearAllMocks además de restoreAllMocks: restore repone los espías,
    // pero no borra el historial de llamadas de un módulo mockeado con
    // vi.mock, y los tests que verifican que NO se llamó al backend
    // arrastrarían las llamadas de los tests anteriores.
    vi.clearAllMocks()
    vi.restoreAllMocks()
  })

  it("prellena el nombre con lo que trae la cuenta de Google", () => {
    renderRegistro()

    expect(screen.getByLabelText("Nombre")).toHaveValue("Ada")
    expect(screen.getByLabelText("Apellido")).toHaveValue("Lovelace")
    expect(screen.getByText("ada@escuela.edu.ar")).toBeInTheDocument()
  })

  // El email viene firmado dentro del token: es lo único que el backend va
  // a mirar, así que un campo editable ofrecería un cambio sin efecto.
  it("no deja editar el email", () => {
    renderRegistro()

    expect(screen.queryByLabelText("Email")).not.toBeInTheDocument()
  })

  // Con Google no hay contraseña que elegir.
  it("no pide contraseña", () => {
    renderRegistro()

    expect(screen.queryByLabelText("Contraseña")).not.toBeInTheDocument()
  })

  it("manda la credencial junto con lo que va a dictar", async () => {
    vi.mocked(authApi.registrarConGoogle).mockResolvedValue(undefined)
    const onRegistrado = renderRegistro()
    const user = userEvent.setup()

    await declarar(user)
    await user.selectOptions(screen.getByLabelText("Año"), "5")
    await user.selectOptions(screen.getByLabelText("División"), "A")
    await user.type(screen.getByLabelText("Materia"), "Programación")
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await waitFor(() =>
      expect(authApi.registrarConGoogle).toHaveBeenCalledWith({
        credential: credencialDeAda,
        nombre: "Ada",
        apellido: "Lovelace",
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
        cursoSolicitado: "5°A",
        materiaSolicitada: "Programación",
      })
    )
    expect(onRegistrado).toHaveBeenCalled()
  })

  // "No lo declaró" y "lo dejó en blanco" no son dos cosas distintas: el
  // curso vacío se omite en vez de mandarse como cadena vacía.
  it("omite el curso y la materia si quedaron vacíos", async () => {
    vi.mocked(authApi.registrarConGoogle).mockResolvedValue(undefined)
    renderRegistro()
    const user = userEvent.setup()

    await declarar(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await waitFor(() =>
      expect(authApi.registrarConGoogle).toHaveBeenCalledWith({
        credential: credencialDeAda,
        nombre: "Ada",
        apellido: "Lovelace",
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
        cursoSolicitado: undefined,
        materiaSolicitada: undefined,
      })
    )
  })

  // El nombre de una cuenta personal de Google no siempre es el que figura
  // en la escuela.
  it("deja corregir el nombre que trajo Google", async () => {
    vi.mocked(authApi.registrarConGoogle).mockResolvedValue(undefined)
    renderRegistro()
    const user = userEvent.setup()

    await user.clear(screen.getByLabelText("Nombre"))
    await user.type(screen.getByLabelText("Nombre"), "Augusta")
    await declarar(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await waitFor(() =>
      expect(authApi.registrarConGoogle).toHaveBeenCalledWith(
        expect.objectContaining({ nombre: "Augusta" })
      )
    )
  })

  // given_name y family_name no son obligatorios en un ID token.
  it("exige nombre y apellido si el token no los trajo", async () => {
    const sinNombre = credencialCon({ email: "ada@escuela.edu.ar" })
    renderRegistro(sinNombre)
    const user = userEvent.setup()

    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findAllByText("Requerido")).toHaveLength(2)
    expect(authApi.registrarConGoogle).not.toHaveBeenCalled()
  })

  it("muestra el mensaje del backend si la cuenta ya existía", async () => {
    vi.mocked(authApi.registrarConGoogle).mockRejectedValue(
      new ApiError(409, "email ya registrado")
    )
    const onRegistrado = renderRegistro()
    const user = userEvent.setup()

    await declarar(user)
    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("email ya registrado")).toBeInTheDocument()
    expect(onRegistrado).not.toHaveBeenCalled()
  })

  // Si la credencial no se puede leer, el formulario sigue sirviendo: el
  // token se manda igual y el backend dirá si vale o no.
  it("con una credencial ilegible, igual deja completar los datos", () => {
    renderRegistro("no-es-un-jwt")

    expect(screen.getByLabelText("Nombre")).toHaveValue("")
    expect(
      screen.getByText("Faltan unos datos para que un Admin pueda aprobar tu cuenta.")
    ).toBeInTheDocument()
  })

  // El bloque de lo declarado es el mismo componente que en el registro con
  // contraseña, así que acá alcanza con verificar que está montado y que se
  // comporta igual con el otro cargo.
  it("como administrador de sistema, no pregunta qué va a dictar", async () => {
    vi.mocked(authApi.registrarConGoogle).mockResolvedValue(undefined)
    renderRegistro()
    const user = userEvent.setup()

    await declarar(user, /^Administrador de Sistema/)

    expect(screen.queryByText("¿Qué vas a dictar?")).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    await waitFor(() =>
      expect(authApi.registrarConGoogle).toHaveBeenCalledWith(
        expect.objectContaining({
          cargoSolicitado: "ADMIN_SISTEMA",
          cursoSolicitado: undefined,
        })
      )
    )
  })

  it("sin elegir cargo ni rol, no manda nada", async () => {
    renderRegistro()
    const user = userEvent.setup()

    await user.click(screen.getByRole("button", { name: "Crear cuenta" }))

    expect(await screen.findByText("Elegí con qué cargo te registrás")).toBeInTheDocument()
    expect(authApi.registrarConGoogle).not.toHaveBeenCalled()
  })
})
