import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { OtrosEquipos } from "@/features/admin/OtrosEquipos"
import * as inventoryApi from "@/features/inventory/api"
import type { PC } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")

function equipo(over: Partial<PC> = {}): PC {
  return {
    id: "eq1",
    etiqueta: "Proyector Epson",
    tipo: "PROYECTOR",
    nombre: "Proyector Epson",
    reservable: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadaDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

function renderSeccion() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <OtrosEquipos />
    </QueryClientProvider>
  )
}

describe("OtrosEquipos", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.crearEquipo).mockResolvedValue(equipo())
  })

  it("muestra los equipos con y sin carro por igual", async () => {
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [
        equipo(),
        equipo({
          id: "eq2",
          tipo: "CARGADOR",
          nombre: "Cargador 1",
          etiqueta: "Cargador 1",
          reservable: false,
        }),
      ],
    })
    renderSeccion()

    expect(await screen.findByText("Proyector Epson")).toBeInTheDocument()
    expect(screen.getByText("Cargador 1")).toBeInTheDocument()
  })

  /**
   * La distinción que importa: el proyector se puede planificar, el cargador
   * se pide en el momento. Sin marcarla, un docente vería dos cargadores
   * entre las máquinas libres cada vez que va a reservar.
   */
  it("distingue lo reservable de lo que solo se presta", async () => {
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [
        equipo(),
        equipo({ id: "eq2", nombre: "Cargador 1", etiqueta: "Cargador 1", reservable: false }),
      ],
    })
    renderSeccion()

    expect(await screen.findByText("Se puede reservar")).toBeInTheDocument()
    expect(screen.getByText("Solo préstamo")).toBeInTheDocument()
  })

  it("da de alta un proyector reservable", async () => {
    const user = userEvent.setup()
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "PROYECTOR")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Proyector Epson")
    await user.click(screen.getByRole("checkbox", { name: /Se puede reservar/ }))
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    expect(adminApi.crearEquipo).toHaveBeenCalledWith({
      tipo: "PROYECTOR",
      nombre: "Proyector Epson",
      reservable: true,
    })
  })

  /**
   * El default es NO reservable: la mayoría de lo que se presta suelto
   * —cargadores, notebooks de repuesto— se pide en el momento, y marcar de
   * más llena la lista de reservas de cosas que nadie planifica.
   */
  it("un equipo nuevo no es reservable salvo que se marque", async () => {
    const user = userEvent.setup()
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "CARGADOR")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Cargador 1")
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    expect(adminApi.crearEquipo).toHaveBeenCalledWith({
      tipo: "CARGADOR",
      nombre: "Cargador 1",
      reservable: false,
    })
  })

  it("ofrece los tipos que ya existen", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [equipo(), equipo({ id: "eq2", tipo: "CARGADOR", nombre: "Cargador 1" })],
    })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))

    // El datalist evita terminar con "PROYECTOR" y "Proyector" como dos
    // tipos distintos, igual que con los nombres de las licencias. Se
    // consulta por selector: las opciones de un datalist no son accesibles
    // por rol, están asociadas al input y no visibles en el árbol.
    const opciones = document.querySelectorAll("#tipos-de-equipo option")
    expect([...opciones].map((o) => o.getAttribute("value"))).toEqual([
      "CARGADOR",
      "PROYECTOR",
    ])
  })

  it("explica el estado cuando no hay ninguno", async () => {
    renderSeccion()

    expect(
      await screen.findByText("No hay ningún equipo cargado todavía.")
    ).toBeInTheDocument()
  })
})
