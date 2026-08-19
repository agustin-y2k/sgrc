import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { PerfilPage } from "@/features/perfil/PerfilPage"
import * as perfilApi from "@/features/perfil/api"
import * as reservasApi from "@/features/reservas/api"

vi.mock("@/features/perfil/api")
vi.mock("@/features/reservas/api")
vi.mock("@/features/academico/api")
vi.mock("@/features/auth/AuthContext")

const DOCENTE: Usuario = {
  id: "docente1",
  nombre: "Ada",
  apellido: "Lovelace",
  email: "ada@escuela.edu.ar",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

function montar() {
  vi.mocked(useAuth).mockReturnValue({
    user: DOCENTE,
    isLoading: false,
    errorDeSesion: null,
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser: vi.fn(),
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <PerfilPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("PerfilPage", () => {
  beforeEach(() => {
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: [] })
    vi.mocked(perfilApi.misPedidos).mockResolvedValue({ data: [] })
  })

  it("muestra quién sos y las materias que das", async () => {
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({
      data: [
        {
          materiaId: "m1",
          materiaNombre: "Programación",
          cursoId: "cur1",
          cursoNombre: "1°A",
          cicloId: "c1",
          cicloAnio: 2026,
        },
      ],
    })
    montar()

    expect(await screen.findByText("Ada Lovelace")).toBeInTheDocument()
    expect(await screen.findByText("Programación")).toBeInTheDocument()
  })

  /**
   * Sin materias no se puede reservar nada, y esa es exactamente la situación
   * en la que alguien entra al perfil sin entender por qué el sistema "no lo
   * deja".
   */
  it("sin materias asignadas, explica que por eso no puede reservar", async () => {
    montar()
    expect(
      await screen.findByText(/no vas a poder reservar computadoras/i)
    ).toBeInTheDocument()
  })

  it("muestra el estado de lo que pediste, y lo que te contestaron", async () => {
    vi.mocked(perfilApi.misPedidos).mockResolvedValue({
      data: [
        {
          id: "p1",
          usuarioId: "docente1",
          esMateriaNueva: true,
          materiaSolicitada: "Robótica",
          cursoSolicitado: "5°B",
          motivo: "Me la asignaron",
          estado: "RECHAZADO",
          respuesta: "Hablé con dirección: queda con quien la da hoy.",
          creadoEn: "2026-08-18T10:00:00Z",
        },
      ],
    })
    montar()

    expect(await screen.findByText("Robótica de 5°B")).toBeInTheDocument()
    expect(screen.getByText("No aprobado")).toBeInTheDocument()
    expect(
      screen.getByText(/Hablé con dirección: queda con quien la da hoy\./)
    ).toBeInTheDocument()
  })

  it("la contraseña se cambia desde acá", async () => {
    montar()
    expect(await screen.findByText("Cambiar mi contraseña")).toBeInTheDocument()
  })
})
