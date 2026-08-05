import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { InventarioPage } from "@/features/inventory/InventarioPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Carro, PC } from "@/features/inventory/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/inventory/api")

function renderInventario() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/inventario"]}>
        <Routes>
          <Route path="/inventario" element={<InventarioPage />} />
          <Route
            path="/inventario/pcs/:pcId/calendario"
            element={<div>Calendario</div>}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const carros: Carro[] = [{ id: "c1", nombre: "Carro 1", descripcion: "Planta baja" }]

function pc(over: Partial<PC>): PC {
  return {
    id: "pc1",
    carroId: "c1",
    identificador: 1,
    numeroSerie: 123,
    freezado: false,
    estado: "DISPONIBLE",
    dadaDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

describe("InventarioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({ data: carros })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("lista los carros", async () => {
    renderInventario()
    expect(await screen.findByText("Carro 1")).toBeInTheDocument()
    expect(screen.getByText("Planta baja")).toBeInTheDocument()
  })

  // Las PCs se piden recién al abrir el carro: cargarlas todas de entrada
  // sería una consulta por carro en cada visita a la página.
  it("no pide las PCs hasta abrir el carro", async () => {
    renderInventario()
    await screen.findByText("Carro 1")

    expect(inventoryApi.listarPCsDeCarro).not.toHaveBeenCalled()
  })

  // RF-03.7: el software instalado es el dato por el que un docente entra.
  it("al abrir el carro muestra estado, freezado y software de cada PC", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({
      data: [pc({ freezado: true, softwareInstalado: "AutoCAD 2027" })],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))

    expect(await screen.findByText("PC 1")).toBeInTheDocument()
    expect(screen.getByText("AutoCAD 2027")).toBeInTheDocument()
    expect(screen.getByText("Disponible")).toBeInTheDocument()
    expect(inventoryApi.listarPCsDeCarro).toHaveBeenCalledWith("c1")
  })

  // RF-03.4: una PC dada de baja deja de listarse como reservable.
  it("no muestra las PCs dadas de baja", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({
      data: [
        pc({ id: "pc1", identificador: 1 }),
        pc({ id: "pc2", identificador: 2, dadaDeBaja: true }),
      ],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))

    expect(await screen.findByText("PC 1")).toBeInTheDocument()
    expect(screen.queryByText("PC 2")).not.toBeInTheDocument()
  })

  it("una PC fuera de servicio se muestra como tal", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({
      data: [pc({ estado: "FUERA_DE_SERVICIO" })],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))

    expect(await screen.findByText("Fuera de servicio")).toBeInTheDocument()
  })

  it("desde una PC se navega a su calendario (RF-04.4)", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({ data: [pc({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))
    await user.click(await screen.findByRole("link", { name: "Ver calendario" }))

    expect(await screen.findByText("Calendario")).toBeInTheDocument()
  })

  // ── Reportar una incidencia (RF-03.5) ────────────────────────────────
  //
  // "Docentes solo pueden reportarlas": esta es la pantalla donde un docente
  // ya está mirando las PCs, así que el reporte va acá y no en una de Admin.

  it("un docente reporta una falla desde el listado de PCs", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({ data: [pc({})] })
    vi.mocked(inventoryApi.reportarIncidencia).mockResolvedValue({
      id: "i1",
      pcId: "pc1",
      descripcion: "No arranca",
      gravedad: "GRAVE",
      fecha: "2026-08-03T10:00:00Z",
      enviadoDge: false,
      estado: "ABIERTA",
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    await user.type(screen.getByLabelText(/¿Qué le pasa\?/), "No arranca")
    await user.selectOptions(screen.getByLabelText("Gravedad"), "GRAVE")
    await user.click(screen.getByRole("button", { name: "Reportar" }))

    expect(inventoryApi.reportarIncidencia).toHaveBeenCalledWith({
      pcId: "pc1",
      descripcion: "No arranca",
      gravedad: "GRAVE",
    })
    expect(await screen.findByText(/quedó registrada/)).toBeInTheDocument()
  })

  // Reportar no saca la PC de circulación (eso es RF-03.8, y lo decide un
  // Admin). Si la pantalla no lo dijera, el docente podría irse creyendo
  // que la PC ya no se puede reservar.
  it("avisa que reportar no saca la PC de circulación", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({ data: [pc({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    expect(screen.getByText(/no saca la PC de circulación/)).toBeInTheDocument()
  })

  it("no deja reportar sin describir el problema", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({ data: [pc({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver PCs" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    expect(screen.getByRole("button", { name: "Reportar" })).toBeDisabled()
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(inventoryApi.listarCarros).mockRejectedValue(
      new ApiError(401, "token inválido o expirado")
    )
    renderInventario()

    expect(await screen.findByText("token inválido o expirado")).toBeInTheDocument()
  })
})
