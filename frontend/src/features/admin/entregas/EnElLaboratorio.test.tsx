import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { MemoryRouter } from "react-router"

import * as adminApi from "@/features/admin/api"
import { EnElLaboratorio } from "@/features/admin/entregas/EnElLaboratorio"
import * as reservasApi from "@/features/reservas/api"
import type { EstadoDelInventario } from "@/features/admin/types"
import type { Prestamo } from "@/features/reservas/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/reservas/api")

function carro(over: Partial<EstadoDelInventario> = {}): EstadoDelInventario {
  return {
    carroId: "c1",
    carroNombre: "Carro 1",
    disponibles: 13,
    enMantenimiento: 0,
    fueraDeServicio: 0,
    total: 13,
    ...over,
  }
}

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    equipoId: "pc1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-11T08:05:00Z",
    abierto: true,
    demorado: false,
    ...over,
  }
}

function renderIndicador() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <EnElLaboratorio />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("EnElLaboratorio", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({ data: [carro()] })
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  /**
   * La pregunta que responde: con cuántas máquinas se cuenta si alguien
   * golpea la puerta ahora.
   */
  it("descuenta del inventario lo que está afuera", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo(), prestamo({ id: "pr2", equipoId: "pc2" })],
    })
    renderIndicador()

    await waitFor(() =>
      expect(screen.getByText("acá ahora").closest("a")).toHaveTextContent("11 de 13")
    )
  })

  /**
   * "Estar acá" y "poder entregarse" no son lo mismo: una máquina en
   * mantenimiento está en el laboratorio y no se le da a nadie.
   */
  it("cuenta aparte las que están acá pero fuera de circulación", async () => {
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({
      data: [
        carro({ disponibles: 10, enMantenimiento: 2, fueraDeServicio: 1, total: 13 }),
      ],
    })
    renderIndicador()

    expect(await screen.findByText("3 no se pueden usar")).toBeInTheDocument()
    expect(screen.getByText("acá ahora").closest("a")).toHaveTextContent("13 de 13")
  })

  // Los equipos sueltos vienen en su propia fila, sin carro. Si el total se
  // leyera de una sola fila, un proyector no contaría.
  it("suma todos los carros y también lo que no está en ninguno", async () => {
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({
      data: [
        carro({ disponibles: 13, total: 13 }),
        carro({ carroId: "c2", carroNombre: "Carro 2", disponibles: 8, total: 8 }),
        { disponibles: 1, enMantenimiento: 0, fueraDeServicio: 0, total: 1 },
      ],
    })
    renderIndicador()

    await waitFor(() =>
      expect(screen.getByText("acá ahora").closest("a")).toHaveTextContent("22 de 22")
    )
  })

  /**
   * Un fallo de red no puede leerse como "no hay nada afuera": el Admin
   * cerraría el laboratorio con máquinas todavía prestadas.
   */
  it("no inventa un número cuando la consulta falla", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockRejectedValue(new Error("sin red"))
    renderIndicador()

    expect(await screen.findByText("No se pudo consultar")).toBeInTheDocument()
    expect(screen.queryByText(/de 13/)).not.toBeInTheDocument()
  })

  // El espacio entre el número y su referencia va en el TEXTO y no en un
  // margen: con `ml-1` a secas un lector de pantalla dice "once de trece"
  // pegado, "11de 13".
  it("el total se lee separado del número", async () => {
    renderIndicador()

    await waitFor(() => {
      const celda = screen.getByText("acá ahora").closest("a")
      expect(celda?.querySelector("p")?.textContent).toBe("13 de 13")
    })
  })
})
