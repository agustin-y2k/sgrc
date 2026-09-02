import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { CuentasDeEquipo } from "@/features/inventory/CuentasDeEquipo"
import * as inventoryApi from "@/features/inventory/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { CuentaDeEquipo, Equipo } from "@/features/inventory/types"

vi.mock("@/features/inventory/api")
vi.mock("@/features/auth/AuthContext")

const notebook: Equipo = {
  id: "eq1",
  etiqueta: "Notebook 1",
  tipo: "NOTEBOOK",
  nombre: "Notebook 1",
  reservable: true,
  esComputadora: false,
  freezado: false,
  estado: "DISPONIBLE",
  dadoDeBaja: false,
  fechaAlta: "2026-01-01T00:00:00Z",
}

function cuenta(over: Partial<CuentaDeEquipo> = {}): CuentaDeEquipo {
  return {
    id: "c1",
    equipoId: "eq1",
    usuario: "alumno",
    clase: "Local",
    privilegio: "COMUN",
    visibilidad: "PUBLICA",
    tienePassword: true,
    hayPasswordParaVer: true,
    puedeVerLaPassword: true,
    ...over,
  }
}

function montar(cuentas: CuentaDeEquipo[], rol: "ADMIN" | "DOCENTE" = "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: { id: "u1", rol },
    isLoading: false,
    login: vi.fn(),
    loginConGoogle: vi.fn(),
    logout: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
  } as unknown as ReturnType<typeof useAuth>)
  vi.mocked(inventoryApi.listarCuentasDeEquipo).mockResolvedValue({ data: cuentas })
  vi.mocked(inventoryApi.listarClasesDeCuenta).mockResolvedValue({ data: ["Local"] })

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <CuentasDeEquipo equipo={notebook} />
    </QueryClientProvider>
  )
}

describe("CuentasDeEquipo", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Cargar cuentas es opcional: no tener ninguna es un estado normal, no un
  // equipo mal cargado.
  it("sin cuentas cargadas, lo dice sin alarmar", async () => {
    montar([])

    expect(await screen.findByText(/No hay ninguna cuenta anotada/)).toBeInTheDocument()
  })

  // La cuenta y su privilegio se muestran SIEMPRE, aunque la contraseña no:
  // saber que una notebook tiene una cuenta de administrador es útil incluso
  // sin poder usarla.
  it("un docente ve la cuenta reservada, con su privilegio, pero no la contraseña", async () => {
    montar([
      cuenta({
        usuario: "Administrador",
        privilegio: "ADMINISTRADOR",
        visibilidad: "SOLO_ADMIN",
        puedeVerLaPassword: false,
      }),
    ])

    // "Administrador" aparece dos veces —el nombre de la cuenta y el badge del
    // privilegio—, así que se afirma sobre los dos.
    expect(await screen.findAllByText("Administrador")).toHaveLength(2)
    expect(screen.getByText("Solo la ven los administradores")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Ver contraseña" })
    ).not.toBeInTheDocument()
  })

  it("un docente sí puede ver la contraseña de una cuenta pública", async () => {
    vi.mocked(inventoryApi.revelarPasswordDeCuenta).mockResolvedValue({
      password: "SecretaDeLaMaquina",
    })
    const user = userEvent.setup()
    montar([cuenta()])

    await user.click(await screen.findByRole("button", { name: "Ver contraseña" }))

    expect(await screen.findByText("SecretaDeLaMaquina")).toBeInTheDocument()
  })

  /**
   * El caso que motiva que visibilidad y privilegio sean dos campos: una
   * cuenta CON privilegios de administrador puede ser de uso común.
   */
  it("una cuenta de administrador marcada pública la ve un docente", async () => {
    montar([
      cuenta({ usuario: "soporte", privilegio: "ADMINISTRADOR", visibilidad: "PUBLICA" }),
    ])

    expect(
      await screen.findByRole("button", { name: "Ver contraseña" })
    ).toBeInTheDocument()
  })

  // Los tres estados de la contraseña.
  it("distingue la cuenta libre de la que tiene una contraseña sin anotar", async () => {
    montar([
      cuenta({
        id: "c1",
        usuario: "libre",
        tienePassword: false,
        hayPasswordParaVer: false,
      }),
      cuenta({
        id: "c2",
        usuario: "sinanotar",
        tienePassword: true,
        hayPasswordParaVer: false,
      }),
    ])

    expect(await screen.findByText("Entra sin contraseña")).toBeInTheDocument()
    // Sin este cartel, "no tiene contraseña" y "no sabemos la contraseña" se
    // ven igual, y alguien queda parado frente a una máquina que no abre.
    expect(screen.getByText("Contraseña no anotada")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Ver contraseña" })
    ).not.toBeInTheDocument()
  })

  it("un docente no ve los botones de gestión", async () => {
    montar([cuenta()])

    await screen.findByText("alumno")
    expect(
      screen.queryByRole("button", { name: "Agregar cuenta" })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Quitar" })).not.toBeInTheDocument()
  })

  it("un Admin sí los ve", async () => {
    montar([cuenta()], "ADMIN")

    expect(
      await screen.findByRole("button", { name: "Agregar cuenta" })
    ).toBeInTheDocument()
    // findBy y no getBy: la fila aparece recién cuando resuelve el listado.
    expect(await screen.findByRole("button", { name: "Editar" })).toBeInTheDocument()
  })

  it("un Admin puede cargar una cuenta nueva", async () => {
    vi.mocked(inventoryApi.crearCuentaDeEquipo).mockResolvedValue(cuenta())
    const user = userEvent.setup()
    montar([], "ADMIN")

    await user.click(await screen.findByRole("button", { name: "Agregar cuenta" }))
    await user.type(screen.getByLabelText(/¿Con qué usuario se entra\?/), "alumno")
    await user.type(screen.getByLabelText(/¿De qué tipo es\?/), "Linux")
    await user.type(screen.getByLabelText(/Contraseña/), "SecretaDeLaMaquina")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(inventoryApi.crearCuentaDeEquipo).toHaveBeenCalledWith("eq1", {
      usuario: "alumno",
      // La clase es texto libre: una escuela con RedHat tiene cuentas de Linux.
      clase: "Linux",
      privilegio: "COMUN",
      // Arranca en lo más restrictivo: una contraseña se comparte a propósito,
      // no por omisión.
      visibilidad: "SOLO_ADMIN",
      tienePassword: true,
      password: "SecretaDeLaMaquina",
      notas: "",
    })
  })
})
