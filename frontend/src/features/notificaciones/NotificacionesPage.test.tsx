import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as authApi from "@/features/auth/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import * as notificacionesApi from "@/features/notificaciones/api"
import { NotificacionesPage } from "@/features/notificaciones/NotificacionesPage"
import type { Notificacion, PreferenciaEmail } from "@/features/notificaciones/types"
import { ApiError } from "@/lib/api-client"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/notificaciones/api")
vi.mock("@/features/auth/api")
vi.mock("@/features/auth/AuthContext")

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

function usuario(rol: Usuario["rol"]): Usuario {
  return {
    id: rol === "ADMIN" ? "admin1" : "docente1",
    nombre: "Ada",
    apellido: "Lovelace",
    email: "ada@escuela.edu.ar",
    rol,
    estado: "APROBADA",
    fechaRegistro: "2026-01-01T00:00:00Z",
    fechaAprobacion: null,
    debeCambiarPassword: false,
  }
}

function preferencia(over: Partial<PreferenciaEmail> = {}): PreferenciaEmail {
  return {
    categoria: "CUENTA_PENDIENTE",
    grupo: "ADMINISTRACION",
    etiqueta: "Cuentas esperando aprobación",
    descripcion: "Cada vez que alguien se registra y queda pendiente.",
    activa: false,
    fija: false,
    ...over,
  }
}

function renderPagina(rol: Usuario["rol"] = "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: usuario(rol),
    isLoading: false,
    errorDeSesion: null,
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser: vi.fn(),
  })
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
    // El aviso de spam pide la config del despliegue; sin correo configurado
    // no se dibuja, y estos tests no son sobre eso.
    vi.mocked(authApi.configPublica).mockResolvedValue({
      googleClientId: "",
      remitenteDeCorreo: "avisos@escuela.edu.ar",
    })
    vi.mocked(notificacionesApi.listarPreferenciasEmail).mockResolvedValue({
      data: [
        preferencia({
          categoria: "RECUPERACION_DE_CUENTA",
          grupo: "CUENTA",
          etiqueta: "Recuperar tu contraseña",
          activa: true,
          fija: true,
        }),
        preferencia(),
        preferencia({ categoria: "SUGERENCIA", etiqueta: "Mensajes del buzón" }),
      ],
    })
    vi.mocked(notificacionesApi.guardarPreferenciasEmail).mockImplementation(
      async (categorias) => ({
        data: [
          preferencia({
            categoria: "RECUPERACION_DE_CUENTA",
            grupo: "CUENTA",
            etiqueta: "Recuperar tu contraseña",
            activa: true,
            fija: true,
          }),
          preferencia({ activa: categorias.includes("CUENTA_PENDIENTE") }),
          preferencia({
            categoria: "SUGERENCIA",
            etiqueta: "Mensajes del buzón",
            activa: categorias.includes("SUGERENCIA"),
          }),
        ],
      })
    )
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

  // ── Copias por correo (RF-05.13) ──────────────────────────────────────

  it("dice qué llega por correo cuando no hay nada tildado", async () => {
    renderPagina("ADMIN")

    // Las de la cuenta salen igual, así que el texto no puede decir "ninguna".
    expect(
      await screen.findByText(/Solo te llegan por correo los avisos de tu cuenta/)
    ).toBeInTheDocument()
  })

  it("el docente también ve el panel: recibe correos por sus reservas", async () => {
    renderPagina("DOCENTE")

    expect(await screen.findByText("Copias por correo")).toBeInTheDocument()
    expect(notificacionesApi.listarPreferenciasEmail).toHaveBeenCalled()
  })

  it("las de la cuenta se muestran tildadas y sin poder tocarse", async () => {
    const user = userEvent.setup()
    renderPagina("DOCENTE")

    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))

    const fija = screen.getByRole("checkbox", { name: /Recuperar tu contraseña/ })
    expect(fija).toBeChecked()
    expect(fija).toBeDisabled()
  })

  // Lo que el panel NO hace: apagar avisos. Es lo primero que hay que
  // entender antes de tocar una casilla.
  it("aclara que los avisos siguen llegando a esta pantalla", async () => {
    const user = userEvent.setup()
    renderPagina("DOCENTE")

    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))

    expect(
      screen.getByText(/te van a seguir apareciendo en esta pantalla/)
    ).toBeInTheDocument()
  })

  it("tildar una casilla y guardar manda solo esa categoría", async () => {
    const user = userEvent.setup()
    renderPagina("ADMIN")

    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))
    await user.click(await screen.findByRole("checkbox", { name: "Mensajes del buzón" }))
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    // El segundo argumento lo agrega react-query (el contexto de la mutación).
    await waitFor(() =>
      expect(notificacionesApi.guardarPreferenciasEmail).toHaveBeenCalledWith(
        ["SUGERENCIA"],
        expect.anything()
      )
    )
    // Y el resumen deja de decir que solo llegan los de la cuenta. Cuenta 2
    // elegibles: la fija no entra en el total.
    expect(
      await screen.findByText(/Te llegan por correo 1 de 2 tipos de aviso/)
    ).toBeInTheDocument()
  })

  it("no deja guardar si no se cambió nada", async () => {
    const user = userEvent.setup()
    renderPagina("ADMIN")

    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))

    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled()
  })
})
