import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { ProtectedRoute, PublicOnlyRoute } from "@/routes/ProtectedRoute"

vi.mock("@/features/auth/AuthContext")

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
    refetchUser: vi.fn(),
    ...overrides,
  })
}

function renderProtected(initialEntry: string) {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/login" element={<div>Login</div>} />
        <Route element={<ProtectedRoute />}>
          <Route path="/" element={<div>Home protegido</div>} />
          <Route path="/cambiar-password" element={<div>Cambiar password</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

describe("ProtectedRoute", () => {
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
