import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { AdminRoute } from "@/routes/AdminRoute"

vi.mock("@/features/auth/AuthContext")

const docenteMock: Usuario = {
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

function renderAdminRoute() {
  render(
    <MemoryRouter initialEntries={["/admin/aprobacion"]}>
      <Routes>
        <Route path="/" element={<div>Home</div>} />
        <Route element={<AdminRoute />}>
          <Route path="/admin/aprobacion" element={<div>Aprobación</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

describe("AdminRoute", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("con rol ADMIN, renderiza el contenido", () => {
    vi.mocked(useAuth).mockReturnValue({
      user: { ...docenteMock, rol: "ADMIN" },
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    renderAdminRoute()

    expect(screen.getByText("Aprobación")).toBeInTheDocument()
  })

  it("con rol DOCENTE, redirige a home", () => {
    vi.mocked(useAuth).mockReturnValue({
      user: docenteMock,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      errorDeSesion: null,
      refetchUser: vi.fn(),
    })
    renderAdminRoute()

    expect(screen.getByText("Home")).toBeInTheDocument()
  })
})
