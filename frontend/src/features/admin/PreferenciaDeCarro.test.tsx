import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as academicoApi from "@/features/academico/api"
import * as adminApi from "@/features/admin/api"
import { PreferenciaDeCarro } from "@/features/admin/PreferenciaDeCarro"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo, PreferenciaDeEquipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/academico/api")
vi.mock("@/features/inventory/api")

function equipo(id: string, identificador: number, over: Partial<Equipo> = {}): Equipo {
  return {
    id,
    identificador,
    etiqueta: `PC ${identificador}`,
    tipo: "Notebook",
    carroId: "carro1",
    reservable: true,
    esComputadora: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-03-01",
    ...over,
  }
}

function renderPanel(
  equipos = [equipo("pc1", 1), equipo("pc2", 2)],
  marcas: PreferenciaDeEquipo[] = [],
) {
  vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: equipos })
  vi.mocked(adminApi.materiasEnUso).mockResolvedValue({ data: ["Dibujo Técnico"] })
  vi.mocked(academicoApi.listarCursos).mockResolvedValue({ data: [] })
  vi.mocked(adminApi.listarPreferenciasDeCarro).mockResolvedValue({ data: marcas })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <PreferenciaDeCarro carroId="carro1" onListo={() => {}} />
    </QueryClientProvider>,
  )
}

test("marca la misma preferencia en todos los equipos del carro", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  await waitFor(() =>
    expect(adminApi.marcarPreferencia).toHaveBeenCalledWith(
      expect.objectContaining({
        equipoIds: ["pc1", "pc2"],
        materiaNombre: "Dibujo Técnico",
        prioridad: 1,
      }),
    ),
  )
})

test("se puede destildar un equipo puntual antes de marcar", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.click(screen.getByRole("checkbox", { name: /PC 1/ }))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  await waitFor(() =>
    expect(adminApi.marcarPreferencia).toHaveBeenCalledWith(
      expect.objectContaining({ equipoIds: ["pc2"] }),
    ),
  )
})

test("no ofrece los equipos que no se reservan", async () => {
  renderPanel([equipo("pc1", 1), equipo("pc3", 3, { reservable: false })])

  await screen.findByText("PC 1")
  expect(screen.queryByText("PC 3")).not.toBeInTheDocument()
})

test("avisa si la prioridad elegida ya la usa otra materia en esos equipos", async () => {
  const usuario = userEvent.setup()
  renderPanel(undefined, [
    {
      id: "p1",
      equipoId: "pc1",
      materiaNombre: "Matemática",
      prioridad: 1,
      alcance: "Matemática",
    },
  ])

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")

  expect(
    await screen.findByText(/ya (son|es) preferentes? de Matemática con prioridad 1/),
  ).toBeInTheDocument()
})

test("no avisa por un empate en un equipo que está destildado", async () => {
  const usuario = userEvent.setup()
  renderPanel(undefined, [
    {
      id: "p1",
      equipoId: "pc1",
      materiaNombre: "Matemática",
      prioridad: 1,
      alcance: "Matemática",
    },
  ])

  await screen.findByText("PC 1")
  await usuario.click(screen.getByRole("checkbox", { name: /PC 1/ }))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")

  await waitFor(() =>
    expect(screen.queryByText(/con prioridad 1/)).not.toBeInTheDocument(),
  )
})

test("cuando el carro no tiene equipos reservables, no se muestra el formulario", async () => {
  renderPanel([])

  expect(
    await screen.findByText("Este carro no tiene equipos reservables."),
  ).toBeInTheDocument()
  expect(screen.queryByRole("button", { name: /marcar/i })).not.toBeInTheDocument()
  expect(screen.queryByLabelText("Materia")).not.toBeInTheDocument()
})
