import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as api from "@/features/reservas/api"
import { hoyISO, type ReservaDetallada } from "@/features/reservas/types"
import { useAhora } from "@/lib/useAhora"
import { paginada } from "@/test/respuestas"
import { EntregarDeUnaReserva } from "./EntregarDeUnaReserva"

vi.mock("@/features/reservas/api")
vi.mock("@/lib/useAhora")

function reserva(
  id: string,
  equipoId: string,
  grupo: string,
  inicio = "15:00",
  fin = "16:00"
): ReservaDetallada {
  return {
    id,
    equipoId,
    reservaGrupoId: grupo,
    fecha: hoyISO(),
    horaInicio: inicio,
    horaFin: fin,
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Docente de prueba",
    materiaNombre: grupo,
    etiqueta: equipoId,
    identificador: 0,
    carroNombre: "",
  }
}
function montar(reservas: ReservaDetallada[]) {
  vi.mocked(api.entregarPorReserva).mockClear()
  const ahora = new Date()
  ahora.setHours(12, 0, 0, 0)
  vi.mocked(useAhora).mockReturnValue(ahora)
  vi.mocked(api.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
  vi.mocked(api.listarReservas).mockResolvedValue(paginada(reservas))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <EntregarDeUnaReserva yaAfuera={new Set()} />
    </QueryClientProvider>
  )
  return client
}

describe("retiro anticipado en el mostrador", () => {
  it("explica el conflicto, lo excluye de Marcar todas y conserva el equipo al completar un retiro parcial", async () => {
    const user = userEvent.setup()
    const tarde = [
      reserva("a", "PC libre", "Clase posterior"),
      reserva("b", "PC ocupada", "Clase posterior"),
    ]
    montar([
      ...tarde,
      reserva("previa", "PC ocupada", "Clase intermedia", "13:00", "14:00"),
    ])
    const grupo = (await screen.findByText("Clase posterior")).closest("div.grid")!
    const contenido = within(grupo as HTMLElement)
    expect(contenido.getByRole("checkbox", { name: "PC ocupada" })).toBeDisabled()
    expect(contenido.getByText(/No se puede adelantar/)).toHaveTextContent(
      "13:00 a 14:00"
    )
    expect(contenido.getByText(/No se puede adelantar/)).toHaveTextContent("se mantiene")
    await user.click(contenido.getByRole("button", { name: "Marcar todas" }))
    expect(contenido.getByRole("checkbox", { name: "PC libre" })).toBeChecked()
    expect(contenido.getByRole("checkbox", { name: "PC ocupada" })).not.toBeChecked()
    await user.click(screen.getByRole("checkbox", { name: /Es todo lo que se llevó/ }))
    vi.mocked(api.entregarPorReserva).mockResolvedValue({
      entregadas: [],
      noEntregadas: [],
    })
    await user.click(
      screen.getByRole("button", { name: "Entregar 1 equipo y liberar el resto" })
    )
    await waitFor(() =>
      expect(api.entregarPorReserva).toHaveBeenCalledWith({
        reservaIds: ["a"],
        equiposEsperados: { a: "PC libre" },
        liberarNoEntregadas: true,
        retiradoPor: undefined,
      })
    )
  })
  it("invalida una selección cuando aparece otra reserva durante el refresco", async () => {
    const user = userEvent.setup()
    const tarde = reserva("a", "PC 1", "Clase posterior")
    const client = montar([tarde])
    await user.click(await screen.findByRole("checkbox", { name: "PC 1" }))
    vi.mocked(api.listarReservas).mockResolvedValue(
      paginada([tarde, reserva("previa", "PC 1", "Clase intermedia", "13:00", "14:00")])
    )
    await client.invalidateQueries({ queryKey: ["reservas"] })
    await screen.findByText(/No se puede adelantar/)
    expect(screen.getByRole("button", { name: "Entregar 1 equipo" })).toBeDisabled()
    expect(api.entregarPorReserva).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "Limpiar la selección" }))
    expect(
      screen.queryByRole("button", { name: "Entregar 1 equipo" })
    ).not.toBeInTheDocument()
  })
})
