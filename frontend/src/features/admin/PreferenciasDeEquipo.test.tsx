import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { PreferenciasDeEquipo } from "@/features/admin/PreferenciasDeEquipo"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")

function preferencia(over: Partial<PreferenciaDeEquipo> = {}): PreferenciaDeEquipo {
  return {
    id: "pref1",
    equipoId: "pc1",
    materiaNombre: "Dibujo Técnico",
    prioridad: 1,
    alcance: "Dibujo Técnico",
    ...over,
  }
}

function renderPanel(preferencias: PreferenciaDeEquipo[] = []) {
  vi.mocked(adminApi.listarPreferenciasDeEquipo).mockResolvedValue({ data: preferencias })
  vi.mocked(adminApi.materiasEnUso).mockResolvedValue({
    data: ["Dibujo Técnico", "Matemática"],
  })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PreferenciasDeEquipo equipoId="pc1" />
    </QueryClientProvider>
  )
}

/**
 * RF-03.21 — las marcas de preferencia de un equipo.
 *
 * Lo que se prueba es la propiedad que define la funcionalidad: la marca
 * sólo ordena, así que ponerla y sacarla no arrastra ninguna consecuencia
 * que haya que confirmar ni avisar.
 */
describe("PreferenciasDeEquipo", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("dice que sin marcas el equipo sale en el orden de siempre", async () => {
    renderPanel()

    expect(await screen.findByText(/Sin marcas/)).toBeInTheDocument()
  })

  it("muestra el alcance y la prioridad de cada marca", async () => {
    renderPanel([
      preferencia({ id: "p1", alcance: "Dibujo Técnico" }),
      preferencia({
        id: "p2",
        materiaNombre: "Matemática",
        anio: 3,
        division: "B",
        prioridad: 2,
        alcance: "Matemática de 3°B",
      }),
    ])

    // Acotado a la lista de marcas: los mismos nombres son opciones del
    // selector de abajo, así que buscarlos sueltos encuentra dos cosas.
    const marcas = within(await screen.findByRole("list"))
    expect(marcas.getByText(/Dibujo Técnico/)).toBeInTheDocument()
    expect(marcas.getByText(/Matemática de 3°B/)).toBeInTheDocument()
    expect(marcas.getByText("Prioridad 2")).toBeInTheDocument()
  })

  // El texto es lo único que impide que la marca se lea como un permiso.
  it("aclara que la marca no excluye a nadie", async () => {
    renderPanel()

    expect(
      await screen.findByText(/cualquiera lo puede reservar igual/i)
    ).toBeInTheDocument()
  })

  it("marca el equipo para la materia elegida", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Materia"), "Matemática")
    await user.selectOptions(screen.getByLabelText("Año"), "3")
    await user.selectOptions(screen.getByLabelText("División"), "B")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Matemática",
        anio: 3,
        division: "B",
        prioridad: 1,
      })
    })
  })

  /**
   * Sin año, una división no significa nada: no existen "todas las B". El
   * backend lo rechaza, pero acá directamente no se puede llegar a ese
   * estado.
   */
  it("no deja elegir división sin año", async () => {
    renderPanel()

    expect(await screen.findByLabelText("División")).toBeDisabled()
  })

  it("sin año manda el alcance sin acotar", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Dibujo Técnico",
        anio: undefined,
        division: undefined,
        prioridad: 1,
      })
    })
  })

  // Quitar la marca no cancela nada, así que no hay confirmación de por medio.
  it("quita una marca sin pedir confirmación", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.borrarPreferencia).mockResolvedValue(undefined)
    renderPanel([preferencia({ id: "p1" })])

    await user.click(await screen.findByRole("button", { name: "Quitar" }))

    await waitFor(() => {
      expect(adminApi.borrarPreferencia).toHaveBeenCalledWith("p1")
    })
  })

  it("sin materias cargadas explica dónde se crean", async () => {
    vi.mocked(adminApi.listarPreferenciasDeEquipo).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.materiasEnUso).mockResolvedValue({ data: [] })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <PreferenciasDeEquipo equipoId="pc1" />
      </QueryClientProvider>
    )

    expect(await screen.findByText(/se crean desde Académico/i)).toBeInTheDocument()
  })
})
