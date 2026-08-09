import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { CambiarEquipoDeReserva } from "@/features/reservas/CambiarEquipoDeReserva"
import * as reservasApi from "@/features/reservas/api"
import type { GrupoDeReservas, ReservaDetallada } from "@/features/reservas/types"

vi.mock("@/features/reservas/api")

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: "2026-08-11",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    identificador: 3,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function grupo(reservas: ReservaDetallada[]): GrupoDeReservas {
  return {
    grupoId: "grupo1",
    esBloqueo: false,
    esRecurrente: false,
    fecha: "2026-08-11",
    horaInicio: "08:00",
    horaFin: "09:00",
    materiaNombre: "Matemáticas",
    nombreDocenteSnapshot: "Ada Lovelace",
    reservas,
  }
}

function renderComponente(g: GrupoDeReservas = grupo([reserva()])) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onListo = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <CambiarEquipoDeReserva grupo={g} onListo={onListo} />
    </QueryClientProvider>
  )
  return { onListo }
}

describe("CambiarEquipoDeReserva", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: [
        {
          equipoId: "pc9",
          identificador: 9,
          etiqueta: "PC 9",
          carroId: "c1",
          carroNombre: "Carro 1",
          freezado: false,
          softwareInstalado: "AutoCAD 2027",
        },
      ],
    })
    vi.mocked(reservasApi.cambiarEquipoDeReserva).mockResolvedValue(reserva({ equipoId: "pc9" }))
  })

  it("cambia la máquina sin tocar la clase", async () => {
    const user = userEvent.setup()
    const { onListo } = renderComponente()

    await screen.findByRole("option", { name: /PC 9/ })
    await user.selectOptions(screen.getByLabelText("¿Por cuál?"), "pc9")
    await user.click(screen.getByRole("button", { name: "Cambiar" }))

    expect(reservasApi.cambiarEquipoDeReserva).toHaveBeenCalledWith("res1", "pc9")
    expect(onListo).toHaveBeenCalled()
  })

  it("busca las libres en la misma franja de la reserva", async () => {
    renderComponente()

    await screen.findByLabelText("¿Por cuál?")
    expect(reservasApi.equiposDisponibles).toHaveBeenCalledWith("2026-08-11", "08:00", "09:00")
  })

  /**
   * El software instalado es el dato por el que se elige una máquina
   * (RF-03.7): cambiar a una que no tenga el programa de la clase no
   * resuelve nada.
   */
  it("muestra el software de cada opción", async () => {
    renderComponente()

    expect(await screen.findByRole("option", { name: /AutoCAD 2027/ })).toBeInTheDocument()
  })

  it("deja elegir cuál de las reservadas se cambia", async () => {
    const user = userEvent.setup()
    renderComponente(
      grupo([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 4 })])
    )

    await user.selectOptions(screen.getByLabelText("¿Cuál cambiás?"), "res2")
    await screen.findByRole("option", { name: /PC 9/ })
    await user.selectOptions(screen.getByLabelText("¿Por cuál?"), "pc9")
    await user.click(screen.getByRole("button", { name: "Cambiar" }))

    expect(reservasApi.cambiarEquipoDeReserva).toHaveBeenCalledWith("res2", "pc9")
  })

  it("no ofrece cambiar las que ya no están confirmadas", async () => {
    renderComponente(
      grupo([
        reserva(),
        reserva({ id: "res2", equipoId: "pc2", identificador: 4, estado: "NO_RETIRADA" }),
      ])
    )

    const select = await screen.findByLabelText("¿Cuál cambiás?")
    expect(select.querySelectorAll("option")).toHaveLength(1)
  })

  it("avisa cuando no hay ninguna libre en ese horario", async () => {
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({ data: [] })
    renderComponente()

    expect(
      await screen.findByText(/No hay ninguna computadora libre en ese horario/)
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Cambiar" })).toBeDisabled()
  })
})
