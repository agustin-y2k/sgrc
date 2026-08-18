import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { NoEncontrada } from "@/routes/NoEncontrada"

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

function conUsuario(user: Usuario) {
  vi.mocked(useAuth).mockReturnValue({
    user,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
  })
}

function renderEn(ruta: string) {
  render(
    <MemoryRouter initialEntries={[ruta]}>
      <Routes>
        <Route path="/" element={<div>Inicio</div>} />
        <Route path="*" element={<NoEncontrada />} />
      </Routes>
    </MemoryRouter>
  )
}

describe("NoEncontrada", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("explica qué pasó en castellano, sin el error crudo del router", () => {
    conUsuario(docenteMock)
    renderEn("/una-direccion-inventada")

    expect(
      screen.getByRole("heading", { name: /esa página no existe/i })
    ).toBeInTheDocument()
    expect(screen.queryByText(/Unexpected Application Error/i)).not.toBeInTheDocument()
  })

  // Quien se equivocó tipeando ve dónde, y quien llegó por un enlace roto
  // tiene algo concreto que mostrarle a un Admin.
  it("muestra la dirección que se intentó abrir", () => {
    conUsuario(docenteMock)
    renderEn("/admin/reportes-que-no-existen")

    expect(screen.getByText("/admin/reportes-que-no-existen")).toBeInTheDocument()
  })

  it("siempre ofrece una salida al inicio", async () => {
    conUsuario(docenteMock)
    renderEn("/perdido")

    await userEvent.click(screen.getByRole("button", { name: /ir al inicio/i }))
    expect(screen.getByText("Inicio")).toBeInTheDocument()
  })

  // Ofrecerle "Gestión del inventario" a un docente sería mandarlo a una
  // pantalla que AdminRoute no lo deja abrir.
  it("a un docente le ofrece atajos que puede usar", () => {
    conUsuario(docenteMock)
    renderEn("/perdido")

    expect(screen.getByRole("link", { name: /reservar computadoras/i })).toHaveAttribute(
      "href",
      "/reservas/nueva"
    )
    expect(
      screen.queryByRole("link", { name: /aprobar cuentas/i })
    ).not.toBeInTheDocument()
  })

  it("a un Admin le ofrece los suyos", () => {
    conUsuario({ ...docenteMock, rol: "ADMIN" })
    renderEn("/perdido")

    expect(screen.getByRole("link", { name: /aprobar cuentas/i })).toHaveAttribute(
      "href",
      "/admin/aprobacion"
    )
    expect(
      screen.queryByRole("link", { name: /quién te puede ayudar/i })
    ).not.toBeInTheDocument()
  })
})
