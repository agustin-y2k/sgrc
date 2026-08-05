import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { DisponibilidadPage } from "@/features/disponibilidad/DisponibilidadPage"
import type { AdminDisponibilidad, BloqueHorario } from "@/features/disponibilidad/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/disponibilidad/api")
vi.mock("@/features/auth/AuthContext")

const DOCENTE: Usuario = {
  id: "docente1",
  nombre: "Ada",
  apellido: "Lovelace",
  email: "ada@test.com",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

const ADMIN: Usuario = { ...DOCENTE, id: "admin1", nombre: "Grace", rol: "ADMIN" }

function mockUsuario(u: Usuario) {
  vi.mocked(useAuth).mockReturnValue({
    user: u,
    isLoading: false,
    errorDeSesion: null,
    login: vi.fn(),
    logout: vi.fn(),
    refetchUser: vi.fn(),
  })
}

function bloque(over: Partial<BloqueHorario> = {}): BloqueHorario {
  return { id: "b1", diaSemana: "LUNES", horaInicio: "08:00", horaFin: "12:00", ...over }
}

function admin(over: Partial<AdminDisponibilidad> = {}): AdminDisponibilidad {
  return {
    usuarioId: "admin1",
    nombre: "Grace",
    apellido: "Hopper",
    disponibleAhora: true,
    horarioSemanal: [bloque()],
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <DisponibilidadPage />
    </QueryClientProvider>
  )
}

describe("DisponibilidadPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUsuario(DOCENTE)
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [admin()],
    })
    vi.mocked(disponibilidadApi.miHorario).mockResolvedValue({ data: [bloque()] })
    vi.mocked(disponibilidadApi.agregarBloque).mockResolvedValue(bloque({ id: "b2" }))
    vi.mocked(disponibilidadApi.editarBloque).mockResolvedValue(bloque())
    vi.mocked(disponibilidadApi.eliminarBloque).mockResolvedValue(undefined)
    vi.mocked(disponibilidadApi.cargarExcepcion).mockResolvedValue({
      id: "e1",
      fecha: "2026-08-10",
      tipo: "NO_DISPONIBLE",
    })
    vi.mocked(disponibilidadApi.marcarNoDisponibleAhora).mockResolvedValue({
      id: "e1",
      fecha: "2026-08-01",
      tipo: "NO_DISPONIBLE",
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // ── RF-07.2, la consulta ────────────────────────────────────────────

  it("muestra a cada Admin con su estado de ahora y su horario", async () => {
    renderPagina()

    expect(await screen.findByText("Grace Hopper")).toBeInTheDocument()
    expect(screen.getByText("Disponible ahora")).toBeInTheDocument()
    expect(screen.getByText("Lunes 08:00–12:00")).toBeInTheDocument()
  })

  it("distingue al Admin no disponible", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [admin({ disponibleAhora: false })],
    })
    renderPagina()

    expect(await screen.findByText("No disponible")).toBeInTheDocument()
  })

  /**
   * Es lo que evita que la pantalla parezca rota: sin el texto de la
   * excepción, un Admin que hoy no viene se ve como "no disponible" en
   * pleno horario habitual y no hay forma de saber por qué.
   */
  it("explica la excepción de hoy cuando contradice el horario habitual", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [
        admin({
          disponibleAhora: false,
          excepcionHoy: {
            id: "e1",
            fecha: "2026-08-01",
            tipo: "NO_DISPONIBLE",
            motivo: "capacitación",
          },
        }),
      ],
    })
    renderPagina()

    expect(await screen.findByText(/Hoy no viene — capacitación/)).toBeInTheDocument()
  })

  it("muestra el horario alternativo de una excepción HORARIO_MODIFICADO", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [
        admin({
          excepcionHoy: {
            id: "e1",
            fecha: "2026-08-01",
            tipo: "HORARIO_MODIFICADO",
            horaInicio: "14:00",
            horaFin: "18:00",
          },
        }),
      ],
    })
    renderPagina()

    expect(
      await screen.findByText(/Hoy con horario distinto: 14:00–18:00/)
    ).toBeInTheDocument()
  })

  /**
   * El backend ordena por `dia_semana` alfabéticamente (VARCHAR), así que
   * viene JUEVES antes que LUNES. Leído así el horario no se entiende.
   */
  it("ordena el horario semanal como transcurre la semana", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [
        admin({
          horarioSemanal: [
            bloque({ id: "b1", diaSemana: "JUEVES" }),
            bloque({ id: "b2", diaSemana: "LUNES" }),
            bloque({ id: "b3", diaSemana: "MIERCOLES" }),
          ],
        }),
      ],
    })
    renderPagina()

    await screen.findByText("Grace Hopper")
    const etiquetas = screen
      .getAllByText(/08:00–12:00/)
      .map((e) => e.textContent?.split(" ")[0])
    expect(etiquetas).toEqual(["Lunes", "Miércoles", "Jueves"])
  })

  it("avisa cuando el Admin todavía no cargó horario", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockResolvedValue({
      data: [admin({ horarioSemanal: [] })],
    })
    renderPagina()

    expect(await screen.findByText("Sin horario cargado.")).toBeInTheDocument()
  })

  it("muestra el error del backend", async () => {
    vi.mocked(disponibilidadApi.listarDisponibilidadDeAdmins).mockRejectedValue(
      new ApiError(500, "error interno")
    )
    renderPagina()

    expect(await screen.findByText("error interno")).toBeInTheDocument()
  })

  // ── Quién ve qué ────────────────────────────────────────────────────

  it("a un docente no le ofrece cargar horario ni excepciones", async () => {
    renderPagina()

    await screen.findByText("Grace Hopper")
    expect(screen.queryByText("Mi horario")).not.toBeInTheDocument()
    expect(screen.queryByText("Excepciones")).not.toBeInTheDocument()
    expect(disponibilidadApi.miHorario).not.toHaveBeenCalled()
  })

  it("a un Admin le muestra además su propio horario", async () => {
    mockUsuario(ADMIN)
    renderPagina()

    expect(await screen.findByText("Mi horario")).toBeInTheDocument()
    expect(screen.getByText("Excepciones")).toBeInTheDocument()
  })

  // ── RF-07.1 / 07.3, el horario propio ───────────────────────────────

  it("agrega un bloque al horario propio", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.selectOptions(await screen.findByLabelText("Día"), "MIERCOLES")
    await user.selectOptions(screen.getByLabelText("Desde: hora"), "09")
    await user.selectOptions(screen.getByLabelText("Desde: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hasta: hora"), "13")
    await user.selectOptions(screen.getByLabelText("Hasta: minutos"), "00")
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    await waitFor(() => {
      expect(disponibilidadApi.agregarBloque).toHaveBeenCalledWith(
        "MIERCOLES",
        "09:00",
        "13:00"
      )
    })
  })

  it("no deja agregar un bloque que termina antes de empezar", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.selectOptions(await screen.findByLabelText("Hasta: hora"), "07")
    await user.selectOptions(screen.getByLabelText("Hasta: minutos"), "00")

    expect(
      screen.getByText("La hora de fin tiene que ser posterior a la de inicio.")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Agregar" })).toBeDisabled()
  })

  it("edita un bloque existente mandando solo los campos del formulario", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.selectOptions(
      screen.getByLabelText("Hasta: hora", { selector: "#editar-b1-fin-hora" }),
      "10"
    )
    await user.selectOptions(
      screen.getByLabelText("Hasta: minutos", { selector: "#editar-b1-fin-minutos" }),
      "30"
    )
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    await waitFor(() => {
      expect(disponibilidadApi.editarBloque).toHaveBeenCalledWith("b1", {
        diaSemana: "LUNES",
        horaInicio: "08:00",
        horaFin: "10:30",
      })
    })
  })

  it("elimina un bloque", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Eliminar" }))

    await waitFor(() => {
      expect(disponibilidadApi.eliminarBloque).toHaveBeenCalledWith("b1")
    })
  })

  it("avisa al Admin sin horario que figura como no disponible siempre", async () => {
    mockUsuario(ADMIN)
    vi.mocked(disponibilidadApi.miHorario).mockResolvedValue({ data: [] })
    renderPagina()

    expect(
      await screen.findByText(/figurás como no disponible siempre/)
    ).toBeInTheDocument()
  })

  // ── RF-07.4 / 07.5, las excepciones ─────────────────────────────────

  it("marca 'no disponible ahora' en un solo paso", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "No estoy disponible ahora" })
    )

    await waitFor(() => {
      expect(disponibilidadApi.marcarNoDisponibleAhora).toHaveBeenCalled()
    })
    expect(
      await screen.findByText(/no disponible por el resto del día/)
    ).toBeInTheDocument()
  })

  /**
   * El backend rechaza con 400 una NO_DISPONIBLE que traiga horario
   * (chk_excepcion_horario_coherente): los campos de hora no se mandan
   * aunque el formulario tenga valores por defecto cargados.
   */
  it("carga una ausencia puntual sin horario", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Fecha"), "2026-08-10")
    await user.type(screen.getByLabelText("Motivo (opcional)"), "licencia")
    await user.click(screen.getByRole("button", { name: "Guardar excepción" }))

    await waitFor(() => {
      expect(disponibilidadApi.cargarExcepcion).toHaveBeenCalledWith({
        fecha: "2026-08-10",
        tipo: "NO_DISPONIBLE",
        motivo: "licencia",
      })
    })
  })

  it("carga un horario distinto para una fecha puntual", async () => {
    mockUsuario(ADMIN)
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Fecha"), "2026-08-10")
    await user.selectOptions(
      screen.getByLabelText("Qué pasa ese día"),
      "HORARIO_MODIFICADO"
    )
    // Las dos secciones de la pantalla —el horario semanal y la excepción—
    // tienen "Desde"/"Hasta", así que se desambigua por id, igual que antes.
    await user.selectOptions(
      screen.getByLabelText("Desde: hora", { selector: "#excepcion-inicio-hora" }),
      "14"
    )
    await user.selectOptions(
      screen.getByLabelText("Desde: minutos", { selector: "#excepcion-inicio-minutos" }),
      "00"
    )
    await user.selectOptions(
      screen.getByLabelText("Hasta: hora", { selector: "#excepcion-fin-hora" }),
      "18"
    )
    await user.selectOptions(
      screen.getByLabelText("Hasta: minutos", { selector: "#excepcion-fin-minutos" }),
      "00"
    )
    await user.click(screen.getByRole("button", { name: "Guardar excepción" }))

    await waitFor(() => {
      expect(disponibilidadApi.cargarExcepcion).toHaveBeenCalledWith({
        fecha: "2026-08-10",
        tipo: "HORARIO_MODIFICADO",
        horaInicio: "14:00",
        horaFin: "18:00",
      })
    })
  })

  it("no deja guardar una excepción sin fecha", async () => {
    mockUsuario(ADMIN)
    renderPagina()

    expect(
      await screen.findByRole("button", { name: "Guardar excepción" })
    ).toBeDisabled()
  })

  it("muestra el error del backend al cargar una excepción", async () => {
    mockUsuario(ADMIN)
    vi.mocked(disponibilidadApi.cargarExcepcion).mockRejectedValue(
      new ApiError(400, "horaInicio/horaFin no coinciden con el tipo de excepción")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Fecha"), "2026-08-10")
    await user.click(screen.getByRole("button", { name: "Guardar excepción" }))

    expect(
      await screen.findByText("horaInicio/horaFin no coinciden con el tipo de excepción")
    ).toBeInTheDocument()
  })
})
