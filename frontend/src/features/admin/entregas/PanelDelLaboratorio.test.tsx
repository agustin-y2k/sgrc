import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import { PanelDelLaboratorio } from "@/features/admin/entregas/PanelDelLaboratorio"
import * as reservasApi from "@/features/reservas/api"
import type { Prestamo, ReservaDetallada } from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")

/**
 * El reloj se fija a las 8:30 de un martes: con la clase de 8 a 9 en curso y
 * la de 10 a 11 por empezar.
 *
 * Sin fijarlo, el test sería una lotería según la hora a la que se corriera
 * —a las 23:50 "dentro de media hora" cae al día siguiente— que es
 * exactamente el tipo de test que empieza a fallar sin que nadie toque nada.
 */
const AHORA = new Date(2026, 7, 11, 8, 30, 0)
const HOY = "2026-08-11"

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    pcId: "pc1",
    fecha: HOY,
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    pcIdentificador: 1,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "5°A",
    etiqueta: `PC ${over.pcIdentificador ?? 1}`,
    ...over,
  }
}

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    pcId: "pc1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-11T08:05:00Z",
    abierto: true,
    demorado: false,
    pcIdentificador: 1,
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.pcIdentificador ?? 1}`,
    ...over,
  }
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <PanelDelLaboratorio />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("PanelDelLaboratorio", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(AHORA)
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.entregarPorReserva).mockResolvedValue({ entregadas: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it("separa la clase en curso de la que todavía no empezó", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva(),
        reserva({
          id: "res2",
          reservaGrupoId: "grupo2",
          pcId: "pc2",
          pcIdentificador: 2,
          horaInicio: "10:00",
          horaFin: "11:00",
          materiaNombre: "Física",
        }),
      ])
    )
    renderPanel()

    // El corte lo hace la hora, no el orden: a las 8:30 la de 8 a 9 está en
    // curso y la de 10 a 11 todavía no empezó.
    expect(await screen.findByText(/1 clase\(s\) en curso/)).toBeInTheDocument()
    expect(screen.getByText(/1 clase\(s\) por empezar/)).toBeInTheDocument()
    expect(screen.getByText(/08:00–09:00 · Matemáticas/)).toBeInTheDocument()
    expect(screen.getByText(/10:00–11:00 · Física/)).toBeInTheDocument()
    // Solo la que está pasando lleva el distintivo.
    expect(screen.getAllByText("En curso")).toHaveLength(1)
  })

  /**
   * "Entregada" o "sin retirar" no sale de la reserva: sale de cruzar sus
   * PCs contra lo que está prestado ahora. La custodia es de la máquina.
   */
  it("distingue las máquinas entregadas de las que siguen adentro", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", pcId: "pc2", pcIdentificador: 2 })])
    )
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPanel()

    expect(await screen.findByText("PC 1 entregada")).toBeInTheDocument()
    expect(screen.getByText("PC 2 sin retirar")).toBeInTheDocument()
  })

  it("marca aparte las que ya se liberaron por no retirarse", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva({ estado: "NO_RETIRADA" })])
    )
    renderPanel()

    // "Liberada" y "sin retirar" no son lo mismo: la primera ya dejó de
    // estar guardada para este docente.
    expect(await screen.findByText("PC 1 liberada")).toBeInTheDocument()
    expect(screen.queryByText("PC 1 sin retirar")).not.toBeInTheDocument()
  })

  it("entrega las máquinas de la clase en curso desde el panel", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", pcId: "pc2", pcIdentificador: 2 })])
    )
    renderPanel()

    await user.click(await screen.findByRole("button", { name: /Entregar todas \(2\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1", "res2"],
      nombreAlternativo: undefined,
    })
  })

  it("solo ofrece entregar las que todavía no salieron", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", pcId: "pc2", pcIdentificador: 2 })])
    )
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPanel()

    await user.click(await screen.findByRole("button", { name: /Entregar \(1\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res2"],
      nombreAlternativo: undefined,
    })
  })

  it("registra que se las llevó otra persona", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPanel()

    await user.click(await screen.findByRole("button", { name: "Se las lleva otro" }))
    await user.type(screen.getByLabelText(/otra persona/), "Juan (alumno)")
    await user.click(screen.getByRole("button", { name: /Entregar todas \(1\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      nombreAlternativo: "Juan (alumno)",
    })
  })

  /**
   * Un bloqueo por evaluación no lo retira nadie: lo crea un Admin para
   * sacar máquinas de circulación, así que ofrecerlo para entregar no
   * significa nada.
   */
  it("no muestra los bloqueos por evaluación", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          tipo: "EVALUACION_ESTATAL",
          materiaNombre: undefined,
          nombreDocenteSnapshot: undefined,
        }),
      ])
    )
    renderPanel()

    expect(
      await screen.findByText("No hay ninguna clase en curso.")
    ).toBeInTheDocument()
  })

  it("pide el día completo sin que la paginación le coma reservas", async () => {
    renderPanel()

    await screen.findByText("No hay ninguna clase en curso.")
    expect(reservasApi.listarReservas).toHaveBeenCalledWith({
      desde: HOY,
      hasta: HOY,
      pageSize: 200,
    })
  })

  it("avisa cuando el día ya terminó", async () => {
    vi.setSystemTime(new Date(2026, 7, 11, 20, 0, 0))
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPanel()

    expect(await screen.findByText(/Hoy ya pasaron 1 clase/)).toBeInTheDocument()
  })
})
