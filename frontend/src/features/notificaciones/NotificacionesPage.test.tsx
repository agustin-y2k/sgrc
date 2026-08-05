import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as notificacionesApi from "@/features/notificaciones/api"
import { NotificacionesPage } from "@/features/notificaciones/NotificacionesPage"
import type { Notificacion } from "@/features/notificaciones/types"
import { ApiError } from "@/lib/api-client"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/notificaciones/api")

function notificacion(over: Partial<Notificacion> = {}): Notificacion {
  return {
    id: "n1",
    mensaje: "Tu reserva del 05/08 fue cancelada: acto escolar",
    tipo: "RESERVA_CANCELADA",
    estado: "NO_LEIDA",
    creadaEn: "2026-08-01T11:30:00Z",
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    // Con router: los avisos ofrecen una acción según su tipo, y esos
    // enlaces necesitan contexto de navegación.
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/notificaciones"]}>
        <NotificacionesPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("NotificacionesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion()])
    )
    vi.mocked(notificacionesApi.marcarLeida).mockResolvedValue(undefined)
    vi.mocked(notificacionesApi.marcarTodasLeidas).mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Es el punto del requisito: el motivo con el que un Admin canceló una
  // reserva ajena (RF-04.8) no se ve en ningún otro lado del sistema.
  it("muestra el mensaje de la notificación", async () => {
    renderPagina()

    expect(
      await screen.findByText("Tu reserva del 05/08 fue cancelada: acto escolar")
    ).toBeInTheDocument()
  })

  it("por defecto pide solo las no leídas", async () => {
    renderPagina()

    await waitFor(() => {
      // El segundo argumento es la página (ver components/Paginador).
      expect(notificacionesApi.listarNotificaciones).toHaveBeenCalledWith("NO_LEIDA", 1)
    })
  })

  it("pide todas al tildar 'mostrar también las leídas'", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByLabelText(/Mostrar también las leídas/))

    await waitFor(() => {
      expect(notificacionesApi.listarNotificaciones).toHaveBeenCalledWith(undefined, 1)
    })
  })

  // El aviso de un docente pendiente ofrece ir a aprobarlo: sin eso, el
  // Admin lee "X se registró" y tiene que buscar a mano la pantalla.
  it("el aviso de docente pendiente enlaza con la pantalla de aprobación", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([
        notificacion({
          tipo: "DOCENTE_PENDIENTE",
          mensaje: "Ada Lovelace se registró y está pendiente de aprobación",
        }),
      ])
    )
    renderPagina()

    const boton = await screen.findByRole("link", { name: "Ir a aprobar" })
    expect(boton).toHaveAttribute("href", "/admin/aprobacion")
  })

  // La acción depende del tipo, no del texto: si dependiera del mensaje,
  // cambiarle una palabra rompería el botón sin que nada lo avise.
  it("un aviso general no ofrece ninguna acción", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion({ tipo: "GENERAL", mensaje: "Aviso cualquiera" })])
    )
    renderPagina()

    await screen.findByText("Aviso cualquiera")
    expect(screen.queryByRole("link", { name: "Ir a aprobar" })).not.toBeInTheDocument()
    expect(
      screen.queryByRole("link", { name: "Ver mis reservas" })
    ).not.toBeInTheDocument()
  })

  it("el aviso de cancelación enlaza con las reservas propias", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion({ tipo: "RESERVA_CANCELADA" })])
    )
    renderPagina()

    expect(await screen.findByRole("link", { name: "Ver mis reservas" })).toHaveAttribute(
      "href",
      "/reservas"
    )
  })

  it("marca una notificación como leída", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar como leída" }))

    await waitFor(() => {
      expect(notificacionesApi.marcarLeida).toHaveBeenCalledWith("n1")
    })
  })

  it("marca todas como leídas", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Marcar todas como leídas" })
    )

    await waitFor(() => {
      expect(notificacionesApi.marcarTodasLeidas).toHaveBeenCalled()
    })
  })

  // Sin ninguna sin leer, la acción masiva no tiene sentido.
  it("no ofrece 'marcar todas' si no hay ninguna sin leer", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion({ estado: "LEIDA", leidaEn: "2026-08-01T12:00:00Z" })])
    )
    renderPagina()

    await screen.findByText("Leída")
    expect(
      screen.queryByRole("button", { name: "Marcar todas como leídas" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Marcar como leída" })
    ).not.toBeInTheDocument()
  })

  it("distingue el vacío de 'sin leer' del vacío total", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(paginada([]))
    const user = userEvent.setup()
    renderPagina()

    expect(await screen.findByText("No tenés avisos sin leer.")).toBeInTheDocument()

    await user.click(screen.getByLabelText(/Mostrar también las leídas/))

    expect(await screen.findByText("No tenés ninguna notificación.")).toBeInTheDocument()
  })

  it("muestra el error del backend", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockRejectedValue(
      new ApiError(500, "error interno")
    )
    renderPagina()

    expect(await screen.findByText("error interno")).toBeInTheDocument()
  })
})
