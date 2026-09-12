import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { MarcasHuerfanas } from "@/features/admin/MarcasHuerfanas"
import type { PreferenciaHuerfana } from "@/features/inventory/types"

vi.mock("@/features/admin/api")

function huerfana(over: Partial<PreferenciaHuerfana> = {}): PreferenciaHuerfana {
  return {
    id: "pref1",
    materiaNombre: "Matemática",
    alcance: "Matemática de 4°2",
    prioridad: 1,
    equipoEtiqueta: "PC 3",
    carroNombre: "Carro 1",
    equipoDadoDeBaja: false,
    ...over,
  }
}

function renderPanel(huerfanas: PreferenciaHuerfana[]) {
  vi.mocked(adminApi.listarPreferenciasHuerfanas).mockResolvedValue({ data: huerfanas })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MarcasHuerfanas />
    </QueryClientProvider>
  )
}

describe("MarcasHuerfanas", () => {
  beforeEach(() => {
    vi.resetAllMocks()
  })

  it("muestra a qué materia apunta la marca y qué máquina quedó marcada", async () => {
    renderPanel([huerfana()])

    expect(await screen.findByText("Matemática de 4°2")).toBeInTheDocument()
    // Sin el carro, "PC 3" no identifica ninguna máquina: hay una por carro.
    expect(screen.getByText(/Carro 1 · PC 3/)).toBeInTheDocument()
  })

  // Cero huérfanas es el estado sano, y es el de casi siempre: una tarjeta fija
  // diciendo que no pasa nada sería ruido permanente en la pantalla de
  // inventario.
  it("no ocupa lugar cuando no hay ninguna", async () => {
    renderPanel([])

    await waitFor(() => {
      expect(adminApi.listarPreferenciasHuerfanas).toHaveBeenCalled()
    })
    expect(screen.queryByText("Marcas que quedaron sin materia")).not.toBeInTheDocument()
  })

  it("avisa cuando el equipo marcado ya está retirado", async () => {
    renderPanel([huerfana({ equipoDadoDeBaja: true })])

    expect(await screen.findByText("Equipo retirado")).toBeInTheDocument()
  })

  it("quita la marca", async () => {
    renderPanel([huerfana()])
    vi.mocked(adminApi.borrarPreferencia).mockResolvedValue(undefined)
    const user = userEvent.setup()

    await user.click(await screen.findByRole("button", { name: "Quitar" }))

    await waitFor(() => {
      expect(adminApi.borrarPreferencia).toHaveBeenCalledWith("pref1")
    })
  })

  // Quien entró acá vino a gestionar carros y equipos. Que falle la consulta de
  // las marcas no puede tapar el inventario con un error rojo.
  it("se calla si la consulta falla", async () => {
    vi.mocked(adminApi.listarPreferenciasHuerfanas).mockRejectedValue(
      new Error("error interno")
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <MarcasHuerfanas />
      </QueryClientProvider>
    )

    await waitFor(() => {
      expect(adminApi.listarPreferenciasHuerfanas).toHaveBeenCalled()
    })
    expect(screen.queryByText(/error interno/)).not.toBeInTheDocument()
  })
})
