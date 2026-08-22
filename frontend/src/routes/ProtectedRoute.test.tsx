import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import {
  descartarPedido,
  olvidarPedidoDescartado,
} from "@/features/admin/pedidoDeJornada"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import {
  ProtectedRoute,
  PublicOnlyRoute,
  RUTA_PRIMERA_JORNADA,
} from "@/routes/ProtectedRoute"

vi.mock("@/features/auth/AuthContext")

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query.
  ...(await original<typeof disponibilidadApi>()),
  jornadaDeLaInstitucion: vi.fn(),
}))

const adminMock: Usuario = {
  id: "2",
  nombre: "Berta",
  apellido: "Admin",
  email: "berta@test.com",
  rol: "ADMIN",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

/** La escuela declaró (o no) al menos un tramo. */
function conJornada(declarada: boolean) {
  vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({
    data: declarada
      ? [{ id: "j1", diaSemana: "LUNES", horaInicio: "08:00", horaFin: "18:00" }]
      : [],
  })
}

const usuarioMock: Usuario = {
  id: "1",
  nombre: "Ana",
  apellido: "Docente",
  email: "ana@test.com",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

function mockAuth(overrides: Partial<ReturnType<typeof useAuth>>) {
  vi.mocked(useAuth).mockReturnValue({
    user: null,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
    ...overrides,
  })
}

function renderProtected(initialEntry: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/login" element={<div>Login</div>} />
          <Route element={<ProtectedRoute />}>
            <Route path="/" element={<div>Home protegido</div>} />
            <Route path="/cambiar-password" element={<div>Cambiar password</div>} />
            <Route
              path={RUTA_PRIMERA_JORNADA}
              element={<div>Declarar la primera jornada</div>}
            />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("ProtectedRoute", () => {
  beforeEach(() => {
    // Las llamadas se cuentan por test: sin limpiar, el "no se le pregunta al
    // docente" vería las consultas que hicieron los tests de Admin de arriba.
    vi.clearAllMocks()
    // Cada test arranca sin el pedido postergado: es una sesión nueva.
    olvidarPedidoDescartado()
    conJornada(true)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("mientras carga, no redirige ni renderiza el contenido protegido", () => {
    mockAuth({ isLoading: true })
    renderProtected("/")

    expect(screen.getByText("Cargando…")).toBeInTheDocument()
    expect(screen.queryByText("Home protegido")).not.toBeInTheDocument()
  })

  it("sin user, redirige a /login", () => {
    mockAuth({ user: null })
    renderProtected("/")

    expect(screen.getByText("Login")).toBeInTheDocument()
  })

  it("con user y sin debeCambiarPassword, renderiza el contenido protegido", () => {
    mockAuth({ user: usuarioMock })
    renderProtected("/")

    expect(screen.getByText("Home protegido")).toBeInTheDocument()
  })

  it("con debeCambiarPassword, fuerza el redirect a /cambiar-password (RF-01.6)", () => {
    mockAuth({ user: { ...usuarioMock, debeCambiarPassword: true } })
    renderProtected("/")

    expect(screen.getByText("Cambiar password")).toBeInTheDocument()
  })

  it("con debeCambiarPassword y ya en /cambiar-password, no entra en loop de redirect", () => {
    mockAuth({ user: { ...usuarioMock, debeCambiarPassword: true } })
    renderProtected("/cambiar-password")

    expect(screen.getByText("Cambiar password")).toBeInTheDocument()
  })

  // ── El portón de la jornada ──────────────────────────────────────────

  it("un Admin que entra sin jornada declarada va a declararla", async () => {
    conJornada(false)
    mockAuth({ user: adminMock })
    renderProtected("/")

    expect(await screen.findByText("Declarar la primera jornada")).toBeInTheDocument()
  })

  it("con la jornada declarada no se pregunta nada", async () => {
    conJornada(true)
    mockAuth({ user: adminMock })
    renderProtected("/")

    expect(await screen.findByText("Home protegido")).toBeInTheDocument()
    expect(screen.queryByText("Declarar la primera jornada")).not.toBeInTheDocument()
  })

  // La molestia deliberada: sin horario declarado se puede trabajar, pero el
  // pedido vuelve. Postergarlo vale para esta sesión y nada más.
  it("postergado, deja pasar por el resto de la sesión", async () => {
    conJornada(false)
    mockAuth({ user: adminMock })
    descartarPedido()

    renderProtected("/")

    expect(await screen.findByText("Home protegido")).toBeInTheDocument()
  })

  it("cerrar sesión vuelve a poner el pedido para el próximo inicio", async () => {
    conJornada(false)
    mockAuth({ user: adminMock })
    descartarPedido()
    olvidarPedidoDescartado()

    renderProtected("/")

    expect(await screen.findByText("Declarar la primera jornada")).toBeInTheDocument()
  })

  // Un docente no puede declarar la jornada, así que bloquearlo dejaría a la
  // escuela entera esperando a que entre un Admin.
  it("a un docente no se le pregunta por la jornada", async () => {
    conJornada(false)
    mockAuth({ user: usuarioMock })
    renderProtected("/")

    expect(await screen.findByText("Home protegido")).toBeInTheDocument()
    expect(disponibilidadApi.jornadaDeLaInstitucion).not.toHaveBeenCalled()
  })

  // Los dos portones en fila, y en este orden: la contraseña es de la persona
  // y la jornada es de la escuela.
  it("la contraseña temporal se cambia antes de declarar la jornada", async () => {
    conJornada(false)
    mockAuth({ user: { ...adminMock, debeCambiarPassword: true } })
    renderProtected("/")

    expect(await screen.findByText("Cambiar password")).toBeInTheDocument()
  })

  // Si la consulta falla no se bloquea: lo peor que pasa es no preguntar esta
  // vez, mientras que bloquear ante la duda deja al Admin sin sistema.
  it("si no se puede saber si hay jornada, no se bloquea", async () => {
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockRejectedValue(
      new Error("sin red")
    )
    mockAuth({ user: adminMock })
    renderProtected("/")

    expect(await screen.findByText("Home protegido")).toBeInTheDocument()
  })
})

describe("PublicOnlyRoute", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  function renderPublicOnly() {
    render(
      <MemoryRouter initialEntries={["/login"]}>
        <Routes>
          <Route path="/" element={<div>Home</div>} />
          <Route element={<PublicOnlyRoute />}>
            <Route path="/login" element={<div>Login</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    )
  }

  it("sin user, muestra la página pública", () => {
    mockAuth({ user: null })
    renderPublicOnly()

    expect(screen.getByText("Login")).toBeInTheDocument()
  })

  it("con user ya logueado, redirige a home en vez de mostrar login", () => {
    mockAuth({ user: usuarioMock })
    renderPublicOnly()

    expect(screen.getByText("Home")).toBeInTheDocument()
  })
})
