import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as authApi from "@/features/auth/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import * as notificacionesApi from "@/features/notificaciones/api"
import { NotificacionesPage } from "@/features/notificaciones/NotificacionesPage"
import type {
  Notificacion,
  PreferenciaEmail,
  TipoNotificacion,
} from "@/features/notificaciones/types"
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
    email: "ada@escuela.test",
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
  return queryClient
}

describe("NotificacionesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion()])
    )
    vi.mocked(notificacionesApi.marcarLeida).mockResolvedValue(undefined)
    vi.mocked(notificacionesApi.marcarTodasLeidas).mockResolvedValue({ marcadas: 1 })
    // El aviso de spam pide la config del despliegue; sin correo configurado
    // no se dibuja, y estos tests no son sobre eso.
    vi.mocked(authApi.configPublica).mockResolvedValue({
      googleClientId: "",
      remitenteDeCorreo: "avisos@escuela.test",
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
      await screen.findByText(
        /Solo están seleccionados los correos obligatorios de tu cuenta/
      )
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
      await screen.findByText(/Tenés seleccionados 1 de 2 tipos de aviso/)
    ).toBeInTheDocument()
  })

  it("no deja guardar si no se cambió nada", async () => {
    const user = userEvent.setup()
    renderPagina("ADMIN")

    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))

    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled()
  })

  // ── A dónde lleva cada tipo de aviso ──────────────────────────────────
  //
  // Recorre TODOS los tipos, no una muestra. Hasta la 1.18.0 la acción salía
  // de una cadena de `if` y a cinco tipos —los tres pedidos y los dos del
  // buzón— no les tocaba ninguna rama: llegaban a la campana sin botón, sin
  // que nada lo señalara. Ahora la tabla es un Record completo y TypeScript
  // obliga a decidir, pero eso solo garantiza que haya una entrada, no que
  // apunte a donde tiene que apuntar.
  //
  // Los avisos del buzón permiten abrir el panel sin buscarlo manualmente.
  const destinos: { tipo: TipoNotificacion; texto: string | null; href?: string }[] = [
    { tipo: "GENERAL", texto: null },
    { tipo: "DOCENTE_PENDIENTE", texto: "Ir a aprobar", href: "/admin/aprobacion" },
    { tipo: "RESERVA_CANCELADA", texto: "Ver mis reservas", href: "/reservas" },
    { tipo: "RESERVA_REUBICADA", texto: "Ver mis reservas", href: "/reservas" },
    { tipo: "LICENCIA_POR_VENCER", texto: "Ver licencias", href: "/admin/licencias" },
    { tipo: "EQUIPO_SIN_DEVOLVER", texto: "Ver entregas", href: "/admin/entregas" },
    { tipo: "PEDIDO_DE_LIBERACION", texto: "Ver mis reservas", href: "/reservas" },
    {
      tipo: "PEDIDO_DE_MATERIA",
      texto: "Ver los pedidos",
      href: "/admin/pedidos-de-materia",
    },
    { tipo: "PEDIDO_DE_MATERIA_RESUELTO", texto: "Ver mi perfil", href: "/perfil" },
    {
      tipo: "SUGERENCIA",
      texto: "Ver conversación",
      href: "/notificaciones?soporte=consultas",
    },
    {
      tipo: "SUGERENCIA_RESPONDIDA",
      texto: "Ver conversación",
      href: "/notificaciones?soporte=consultas",
    },
  ]

  it.each(destinos)("el aviso $tipo lleva a donde corresponde", async (destino) => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([
        notificacion({ tipo: destino.tipo, mensaje: `aviso de ${destino.tipo}` }),
      ])
    )
    renderPagina()
    await screen.findByText(`aviso de ${destino.tipo}`)

    if (destino.texto === null) {
      // No hay botón de acción: el único que queda es "Marcar como leída".
      const enlaces = screen.queryAllByRole("link")
      expect(enlaces).toHaveLength(0)
      return
    }
    expect(screen.getByRole("link", { name: destino.texto })).toHaveAttribute(
      "href",
      destino.href
    )
  })

  // El Record cubre todos los tipos que declara el backend, y la lista de
  // arriba cubre todo el Record. Si alguien agrega un tipo y se olvida del
  // test, esto lo dice.
  it("la tabla de destinos cubre todos los tipos de aviso", () => {
    const declarados: TipoNotificacion[] = [
      "GENERAL",
      "DOCENTE_PENDIENTE",
      "RESERVA_CANCELADA",
      "RESERVA_REUBICADA",
      "LICENCIA_POR_VENCER",
      "EQUIPO_SIN_DEVOLVER",
      "PEDIDO_DE_LIBERACION",
      "PEDIDO_DE_MATERIA",
      "PEDIDO_DE_MATERIA_RESUELTO",
      "SUGERENCIA",
      "SUGERENCIA_RESPONDIDA",
    ]
    expect(destinos.map((d) => d.tipo).sort()).toEqual(declarados.sort())
  })

  // ── Sacarse un aviso de encima ────────────────────────────────────────
  //
  // Antes se podía marcar leído y nada más: los avisos se acumulaban para
  // siempre.

  it("borra un aviso ya leído", async () => {
    const user = userEvent.setup()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue({
      data: [notificacion({ id: "n1", estado: "LEIDA" })],
      meta: { total: 1, page: 1, pageSize: 20 },
    })
    vi.mocked(notificacionesApi.borrar).mockResolvedValue(undefined)
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Borrar" }))

    await waitFor(() => expect(notificacionesApi.borrar).toHaveBeenCalledWith("n1"))
  })

  // El botón no está en los avisos sin leer: es la misma regla que aplica el
  // servidor, y ofrecerlo para que falle con un 409 sería peor que no tenerlo.
  it("no ofrece borrar un aviso sin leer", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue({
      data: [notificacion({ id: "n1", estado: "NO_LEIDA" })],
      meta: { total: 1, page: 1, pageSize: 20 },
    })
    renderPagina()

    expect(
      await screen.findByRole("button", { name: "Marcar como leída" })
    ).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Borrar" })).toBeNull()
  })

  it("vacía de una vez los avisos leídos", async () => {
    const user = userEvent.setup()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue({
      data: [
        notificacion({ id: "n1", estado: "LEIDA" }),
        notificacion({ id: "n2", estado: "LEIDA" }),
      ],
      meta: { total: 2, page: 1, pageSize: 20 },
    })
    vi.mocked(notificacionesApi.borrarLeidas).mockResolvedValue({ borradas: 2 })
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Borrar todos los avisos leídos" })
    )

    await waitFor(() => expect(notificacionesApi.borrarLeidas).toHaveBeenCalled())
  })
  it("reintenta una lectura fallida sin mostrar vacío ni acciones con datos desactualizados", async () => {
    const user = userEvent.setup()
    const qc = renderPagina()
    await screen.findByText(notificacion().mensaje)
    vi.mocked(notificacionesApi.listarNotificaciones).mockRejectedValueOnce(
      new ApiError(503, "Falló la lectura")
    )
    await qc.invalidateQueries({ queryKey: ["notificaciones", "NO_LEIDA"] })
    await screen.findByText("Falló la lectura")
    expect(screen.queryByText(notificacion().mensaje)).not.toBeInTheDocument()
    expect(screen.queryByText("No tenés avisos sin leer.")).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Marcar todas como leídas" })
    ).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Reintentar avisos" }))
    await screen.findByText(notificacion().mensaje)
  })

  it("el borrado masivo informa la cantidad total aunque exceda la página visible", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion({ estado: "LEIDA" })])
    )
    vi.mocked(notificacionesApi.borrarLeidas).mockResolvedValue({ borradas: 75 })
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Borrar todos los avisos leídos" })
    )
    await screen.findByText("Avisos leídos borrados: 75.")
  })

  it("las acciones masivas bloquean otras mutaciones hasta terminar", async () => {
    let completar!: (r: { marcadas: number }) => void
    vi.mocked(notificacionesApi.marcarTodasLeidas).mockImplementationOnce(
      () =>
        new Promise((r) => {
          completar = r
        })
    )
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Marcar todas como leídas" })
    )
    expect(screen.getByRole("button", { name: "Marcar como leída" })).toBeDisabled()
    expect(screen.getByLabelText("Mostrar también las leídas")).toBeDisabled()
    completar({ marcadas: 3 })
    await screen.findByText("Avisos marcados como leídos: 3.")
  })

  it("vuelve a la primera página al leer el último aviso de la segunda", async () => {
    let leida = false
    vi.mocked(notificacionesApi.listarNotificaciones).mockImplementation(
      async (_, p = 1) => ({
        data: p === 2 && leida ? [] : [notificacion()],
        meta: { total: leida ? 50 : 51, page: p, pageSize: 50 },
      })
    )
    vi.mocked(notificacionesApi.marcarLeida).mockImplementationOnce(async () => {
      leida = true
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(await screen.findByRole("button", { name: "Siguiente" }))
    await screen.findByText("Página 2 de 2")
    await user.click(screen.getByRole("button", { name: "Marcar como leída" }))
    await waitFor(() =>
      expect(notificacionesApi.listarNotificaciones).toHaveBeenLastCalledWith(
        "NO_LEIDA",
        1
      )
    )
  })

  it("el aviso de soporte enlaza al hilo concreto con el identificador codificado", async () => {
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([notificacion({ tipo: "SUGERENCIA_RESPONDIDA", hiloId: "hilo/1" })])
    )
    renderPagina()
    expect(await screen.findByRole("link", { name: "Ver conversación" })).toHaveAttribute(
      "href",
      "/notificaciones?soporte=consultas&hilo=hilo%2F1"
    )
  })

  it("reintenta preferencias y conserva la selección ante un guardado fallido", async () => {
    vi.mocked(notificacionesApi.listarPreferenciasEmail).mockRejectedValueOnce(
      new ApiError(503, "Preferencias inaccesibles")
    )
    vi.mocked(notificacionesApi.guardarPreferenciasEmail).mockRejectedValueOnce(
      new ApiError(503, "Falló el guardado")
    )
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      await screen.findByRole("button", { name: "Reintentar preferencias" })
    )
    await user.click(screen.getByRole("button", { name: "Elegir cuáles" }))
    const campo = await screen.findByRole("checkbox", {
      name: "Cuentas esperando aprobación",
    })
    await user.click(campo)
    await user.click(screen.getByRole("button", { name: "Guardar" }))
    await screen.findByText("Falló el guardado")
    expect(campo).toBeChecked()
    await user.click(screen.getByRole("button", { name: "Guardar" }))
    await screen.findByText("Preferencias guardadas.")
  })

  it("impide editar preferencias mientras se está guardando", async () => {
    let completar!: (r: { data: PreferenciaEmail[] }) => void
    vi.mocked(notificacionesApi.guardarPreferenciasEmail).mockImplementationOnce(
      () =>
        new Promise((r) => {
          completar = r
        })
    )
    const user = userEvent.setup()
    renderPagina()
    await user.click(await screen.findByRole("button", { name: "Elegir cuáles" }))
    const campo = await screen.findByRole("checkbox", {
      name: "Cuentas esperando aprobación",
    })
    await user.click(campo)
    await user.click(screen.getByRole("button", { name: "Guardar" }))
    expect(campo).toBeDisabled()
    completar({ data: [preferencia({ activa: true })] })
    await screen.findByText("Preferencias guardadas.")
  })
})
