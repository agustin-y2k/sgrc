import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { AppLayout } from "@/components/layout/AppLayout"
import { useAuth } from "@/features/auth/AuthContext"
import * as notificacionesApi from "@/features/notificaciones/api"
import type { Notificacion } from "@/features/notificaciones/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/auth/AuthContext")
vi.mock("@/features/notificaciones/api")

function notificacion(id: string): Notificacion {
  return {
    id,
    mensaje: "Se canceló tu reserva",
    tipo: "RESERVA_CANCELADA",
    estado: "NO_LEIDA",
    creadaEn: "2026-08-01T11:30:00Z",
  }
}

function renderLayout(rol: "ADMIN" | "DOCENTE" = "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: {
      id: "u1",
      nombre: "Ada",
      apellido: "Lovelace",
      email: "ada@escuela.edu.ar",
      rol,
      estado: "APROBADA",
      fechaRegistro: "2026-01-01T00:00:00Z",
      fechaAprobacion: null,
      debeCambiarPassword: false,
    },
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
  })

  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AppLayout />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("AppLayout", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(paginada([]))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // RF-05.7: las notificaciones tienen que verse al ingresar al sistema.
  // El contador es lo que hace que alguien entre — nadie abre una bandeja
  // de avisos por las dudas.
  it("muestra la cantidad de avisos sin leer", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion("n1"), notificacion("n2"), notificacion("n3")])
    )
    renderLayout()

    expect(await screen.findByLabelText("3 sin leer")).toBeInTheDocument()
  })

  it("no muestra el contador si no hay avisos sin leer", async () => {
    renderLayout()

    expect(await screen.findByRole("link", { name: /Avisos/ })).toBeInTheDocument()
    expect(screen.queryByLabelText(/sin leer/)).not.toBeInTheDocument()
  })

  it("consulta solo las no leídas para el contador", async () => {
    renderLayout()

    expect(notificacionesApi.listarNotificaciones).toHaveBeenCalledWith("NO_LEIDA")
  })

  it("un docente no ve los enlaces de administración", async () => {
    renderLayout("DOCENTE")

    expect(await screen.findByRole("link", { name: "Reservas" })).toBeInTheDocument()
    // Ni los enlaces ni el grupo que los contiene: sin el botón no hay
    // forma de llegar, y su ausencia es más fácil de comprobar que la de
    // cada enlace por separado.
    expect(
      screen.queryByRole("button", { name: /Administración/ })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: "Académico" })).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: "Usuarios" })).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: "Aprobación" })).not.toBeInTheDocument()
    // RF-04.7 es solo de Admin y cancela reservas ajenas: que no se filtre.
    // El nombre tiene que ser el que la barra usa de verdad (ver
    // ENLACES_ADMIN): con uno viejo la aserción da verdadera siempre y el
    // enlace se podría filtrar sin que nadie se entere.
    expect(
      screen.queryByRole("link", { name: "Bloquear equipos" })
    ).not.toBeInTheDocument()
  })

  // RF-07.2: la disponibilidad de los Admins la consulta cualquier usuario
  // autenticado. El enlace vive al lado de los de administración, así que es
  // fácil que alguien lo mueva adentro del bloque de rol sin querer.
  it("un docente sí ve la disponibilidad de los Admins", async () => {
    renderLayout("DOCENTE")

    expect(
      await screen.findByRole("link", { name: "Horario Admins" })
    ).toBeInTheDocument()
  })

  // Aprobación queda suelta en la barra a propósito: es la única tarea de
  // administración que es diaria y que hace esperar a otra persona.
  it("un Admin ve Aprobación en la barra, sin abrir nada", async () => {
    renderLayout("ADMIN")

    expect(await screen.findByRole("link", { name: "Aprobación" })).toBeInTheDocument()
  })

  it("un Admin llega al resto abriendo el grupo Administración", async () => {
    renderLayout("ADMIN")

    const grupo = await screen.findByRole("button", { name: /Administración/ })
    expect(grupo).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByRole("link", { name: "Académico" })).not.toBeInTheDocument()

    await userEvent.click(grupo)

    expect(grupo).toHaveAttribute("aria-expanded", "true")
    expect(screen.getByRole("link", { name: "Académico" })).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Usuarios" })).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Reportes" })).toBeInTheDocument()
  })

  // Un desplegable que solo se cierra con el mismo botón que lo abrió es
  // una trampa: se aprieta Escape o se hace clic afuera y no pasa nada.
  it("Escape cierra el grupo Administración", async () => {
    renderLayout("ADMIN")

    const grupo = await screen.findByRole("button", { name: /Administración/ })
    await userEvent.click(grupo)
    expect(screen.getByRole("link", { name: "Reportes" })).toBeInTheDocument()

    await userEvent.keyboard("{Escape}")

    expect(screen.queryByRole("link", { name: "Reportes" })).not.toBeInTheDocument()
  })
})
