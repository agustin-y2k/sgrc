import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { HistoricoPorAnio } from "./HistoricoPorAnio"
import * as api from "./api"

vi.mock("./api")

function mostrar() {
  const cliente = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={cliente}>
      <HistoricoPorAnio
        ciclos={[
          { id: "cerrado", anio: 2025, activo: false, archivado: true },
          { id: "anterior", anio: 2024, activo: false, archivado: true },
        ]}
      />
    </QueryClientProvider>
  )
  return cliente
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.historicoUsoEquipos).mockResolvedValue({ data: [] })
  vi.mocked(api.historicoUsoDocentes).mockResolvedValue({ data: [] })
})

it("una consulta pendiente no asegura que el año se archivó sin reservas", async () => {
  let resolver!: (v: { data: [] }) => void
  vi.mocked(api.historicoUsoEquipos).mockReturnValue(
    new Promise((r) => {
      resolver = r
    })
  )
  vi.mocked(api.historicoUsoDocentes).mockReturnValue(new Promise(() => {}))
  mostrar()
  expect(screen.getByText("Consultando histórico de equipos…")).toBeInTheDocument()
  expect(screen.queryByText(/sin ninguna reserva registrada/)).not.toBeInTheDocument()
  await act(async () => resolver({ data: [] }))
  expect(await screen.findByText(/sin ninguna reserva registrada/)).toBeInTheDocument()
  expect(screen.getByText("Consultando histórico de docentes…")).toBeInTheDocument()
})

it("un fallo de equipos se puede reintentar sin ocultar el histórico válido de docentes", async () => {
  vi.mocked(api.historicoUsoEquipos).mockRejectedValueOnce(new Error("Sin conexión"))
  vi.mocked(api.historicoUsoDocentes).mockResolvedValue({
    data: [
      {
        id: "doc",
        anio: 2025,
        usuarioId: "u",
        nombreDocenteSnapshot: "Docente Histórico",
        cantidadReservas: 1,
        minutosTotales: 60,
      },
    ],
  })
  const user = userEvent.setup()
  mostrar()
  await screen.findByText(/No pudimos consultar el histórico de equipos/)
  expect(screen.getByText("Docente Histórico")).toBeInTheDocument()
  expect(screen.queryByText(/sin ninguna reserva registrada/)).not.toBeInTheDocument()
  await user.click(screen.getByRole("button", { name: "Reintentar" }))
  await screen.findByText(/sin ninguna reserva registrada/)
  expect(screen.queryByText(/No pudimos consultar el histórico/)).not.toBeInTheDocument()
})

it("un fallo de docentes no se anuncia como año sin uso", async () => {
  vi.mocked(api.historicoUsoEquipos).mockReturnValue(new Promise(() => {}))
  vi.mocked(api.historicoUsoDocentes).mockRejectedValueOnce(new Error("Sin conexión"))
  mostrar()
  await screen.findByText(/No pudimos consultar el histórico de docentes/)
  expect(screen.queryByText(/sin ninguna reserva registrada/)).not.toBeInTheDocument()
})

it("cambiar de año no reutiliza el vacío del año anterior mientras carga", async () => {
  const user = userEvent.setup()
  mostrar()
  await waitFor(() =>
    expect(screen.getAllByText(/sin ninguna reserva registrada/)).toHaveLength(2)
  )
  vi.mocked(api.historicoUsoEquipos).mockReturnValue(new Promise(() => {}))
  vi.mocked(api.historicoUsoDocentes).mockReturnValue(new Promise(() => {}))
  await user.selectOptions(screen.getByLabelText("Año"), "2024")
  expect(screen.queryByText(/sin ninguna reserva registrada/)).not.toBeInTheDocument()
  expect(screen.getByText("Consultando histórico de equipos…")).toBeInTheDocument()
  expect(api.historicoUsoDocentes).toHaveBeenLastCalledWith(2024)
})
