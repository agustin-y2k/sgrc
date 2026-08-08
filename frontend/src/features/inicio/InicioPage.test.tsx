import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"

import * as adminApi from "@/features/admin/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { InicioPage } from "@/features/inicio/InicioPage"
import * as notificacionesApi from "@/features/notificaciones/api"
import * as reservasApi from "@/features/reservas/api"
import type { ReservaDetallada } from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")
vi.mock("@/features/admin/api")
vi.mock("@/features/notificaciones/api")
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
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser: vi.fn(),
  })
}

/** Hoy, como lo arma la propia página (fecha local, no UTC). */
function hoy(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "r1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: hoy(),
    horaInicio: "14:00",
    horaFin: "15:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    identificador: 1,
    carroNombre: "Carro 1",
    materiaNombre: "Matemática",
    cursoNombre: "5°A",
    nombreDocenteSnapshot: "Ada Lovelace",
    creadoPor: "docente1",
    ...over,
  } as ReservaDetallada
}

function renderInicio() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <InicioPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("InicioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([]))
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("muestra las próximas reservas agrupadas en una sola línea por clase", async () => {
    mockUsuario(DOCENTE)
    // Dos equipos de la MISMA reserva: para el docente fue una sola clase.
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "r2", equipoId: "pc2", identificador: 2 })])
    )
    renderInicio()

    expect(await screen.findByText(/Matemática/)).toBeInTheDocument()
    expect(screen.getByText("2 equipos")).toBeInTheDocument()
    expect(screen.getAllByText(/Matemática/)).toHaveLength(1)
  })

  it("cuenta como 'clase hoy' solo lo de hoy", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva(),
        reserva({ id: "r9", reservaGrupoId: "grupo9", fecha: "2099-12-31" }),
      ])
    )
    renderInicio()

    // Una hoy, dos próximas en total.
    expect(await screen.findByText("clase hoy")).toBeInTheDocument()
    const hoyEnlace = screen.getByText("clase hoy").closest("a")
    expect(hoyEnlace).toHaveTextContent("1")
    const proximas = screen.getByText("próximas").closest("a")
    expect(proximas).toHaveTextContent("2")
  })

  it("no cuenta las canceladas", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva({ estado: "CANCELADA" })])
    )
    renderInicio()

    expect(await screen.findByText(/No tenés reservas próximas/)).toBeInTheDocument()
  })

  it("a un docente no le pregunta por las cuentas pendientes", async () => {
    mockUsuario(DOCENTE)
    renderInicio()

    expect(await screen.findByText(/No tenés reservas próximas/)).toBeInTheDocument()
    // El endpoint es solo de Admin: pedirlo desde un docente sería un 403
    // en cada carga del home.
    expect(adminApi.listarUsuarios).not.toHaveBeenCalled()
    expect(screen.queryByText("por aprobar")).not.toBeInTheDocument()
  })

  it("a un Admin le muestra cuántas cuentas esperan aprobación", async () => {
    mockUsuario(ADMIN)
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue(
      paginada([], { total: 3, pageSize: 50 })
    )
    renderInicio()

    // findByText del número y no del rótulo: el rótulo ya está en pantalla
    // mientras la consulta viaja, así que buscarlo a él pasa de largo y lee
    // el contador todavía en cero.
    const pendientes = (await screen.findByText("3")).closest("a")
    expect(pendientes).toHaveTextContent("por aprobar")
    expect(pendientes).toHaveAttribute("href", "/admin/aprobacion")
  })

  /**
   * El fallo más caro de esta pantalla no es quedarse en blanco: es afirmar
   * que no hay nada. Un Admin que lee "0 afuera" cierra el laboratorio con
   * las computadoras todavía prestadas.
   */
  describe("cuando una consulta falla", () => {
    it("no muestra cero: muestra que no pudo preguntar", async () => {
      mockUsuario(ADMIN)
      vi.mocked(reservasApi.listarPrestamosAbiertos).mockRejectedValue(
        new Error("se cayó la red")
      )
      renderInicio()

      expect(await screen.findByText(/lo que ves acá puede estar incompleto/i)).toBeInTheDocument()

      const afuera = screen.getByText("afuera").closest("a")
      expect(afuera).toHaveTextContent("—")
      expect(afuera).not.toHaveTextContent("0")
    })

    it("el aviso no aparece cuando todo respondió bien", async () => {
      mockUsuario(ADMIN)
      renderInicio()

      await screen.findByText("afuera")
      expect(
        screen.queryByText(/lo que ves acá puede estar incompleto/i)
      ).not.toBeInTheDocument()
    })

    // Que falle lo del mostrador no puede llevarse puesto el resto: las
    // reservas se consultan aparte y su número sigue siendo confiable.
    it("los demás indicadores siguen mostrando lo suyo", async () => {
      mockUsuario(ADMIN)
      vi.mocked(reservasApi.listarPrestamosAbiertos).mockRejectedValue(
        new Error("se cayó la red")
      )
      vi.mocked(adminApi.listarUsuarios).mockResolvedValue(
        paginada([], { total: 3, pageSize: 50 })
      )
      renderInicio()

      const pendientes = (await screen.findByText("3")).closest("a")
      expect(pendientes).toHaveTextContent("por aprobar")
    })
  })
})
