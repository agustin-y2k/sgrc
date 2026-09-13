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
 */
const AHORA = new Date(2026, 7, 11, 8, 30, 0)
const HOY = "2026-08-11"

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: HOY,
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    identificador: 1,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "5°A",
    etiqueta: `PC ${over.identificador ?? 1}`,
    ...over,
  }
}

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    equipoId: "pc1",
    reservaId: "res1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-11T08:05:00Z",
    abierto: true,
    demorado: false,
    identificador: 1,
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.identificador ?? 1}`,
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
          equipoId: "pc2",
          identificador: 2,
          horaInicio: "10:00",
          horaFin: "11:00",
          materiaNombre: "Física",
        }),
      ])
    )
    renderPanel()

    // El corte lo hace la hora, no el orden: a las 8:30 la de 8 a 9 está en
    // curso y la de 10 a 11 todavía no empezó.
    expect(await screen.findByText(/1 clase en curso/)).toBeInTheDocument()
    expect(screen.getByText(/1 por empezar/)).toBeInTheDocument()
    expect(screen.getByText(/08:00–09:00 · Matemáticas/)).toBeInTheDocument()
    expect(screen.getByText(/10:00–11:00 · Física/)).toBeInTheDocument()
    // Solo la que está pasando lleva el distintivo.
    expect(screen.getAllByText("En curso")).toHaveLength(1)
  })

  /**
   * "Entregada" o "sin retirar" no sale de la reserva: sale de cruzar sus
   * Equipos contra lo que está prestado ahora.
   */
  it("distingue las máquinas entregadas de las que siguen adentro", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 2 })])
    )
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPanel()

    // El resumen cuenta; los chips nombran solo lo que hay que ir a buscar.
    // Doce chips "PC 7 · Carro 1 entregada" eran doscientos píxeles para
    // contar hasta doce.
    expect(await screen.findByText("1 entregada de 2 computadoras")).toBeInTheDocument()
    expect(screen.getByText("PC 2 · Carro 1 sin retirar")).toBeInTheDocument()
  })

  it("marca aparte las que ya se liberaron por no retirarse", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva({ estado: "NO_RETIRADA" })])
    )
    renderPanel()

    // "Liberada" y "sin retirar" no son lo mismo: la primera ya dejó de
    // estar guardada para este docente.
    expect(await screen.findByText("PC 1 · Carro 1 liberada")).toBeInTheDocument()
    expect(screen.queryByText("PC 1 · Carro 1 sin retirar")).not.toBeInTheDocument()
  })

  it("entrega las máquinas de la clase en curso desde el panel", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 2 })])
    )
    renderPanel()

    await user.click(await screen.findByRole("button", { name: /Entregar todas \(2\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1", "res2"],
      retiradoPor: undefined,
    })
  })

  it("solo ofrece entregar las que todavía no salieron", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 2 })])
    )
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPanel()

    await user.click(await screen.findByRole("button", { name: /Entregar \(1\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res2"],
      retiradoPor: undefined,
    })
  })

  it("anota quién retira sin cambiar de quién es la responsabilidad", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPanel()

    await user.click(
      await screen.findByRole("button", { name: "Anotar quién las retira" })
    )
    await user.type(screen.getByLabelText(/Quién las retira/), "Juan (alumno)")
    // El mostrador lo dice sin que haya que abrir nada: anotar al alumno no
    // le saca la responsabilidad al docente.
    expect(screen.getByText(/Quedan igual a cargo de Ada Lovelace/)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: /Entregar todas \(1\)/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: "Juan (alumno)",
    })
  })

  /**
   * Un bloqueo administrativo no lo retira nadie: lo crea un Admin para sacar
   * máquinas de circulación, así que ofrecerlo para entregar no significa
   * nada.
   */
  it("no muestra los bloqueos administrativos", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          tipo: "BLOQUEO",
          materiaNombre: undefined,
          nombreDocenteSnapshot: undefined,
        }),
      ])
    )
    renderPanel()

    expect(
      await screen.findByText("Hoy no hay ninguna clase con equipos reservados.")
    ).toBeInTheDocument()
  })

  it("pide el día completo sin que la paginación le coma reservas", async () => {
    renderPanel()

    await screen.findByText("Hoy no hay ninguna clase con equipos reservados.")
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

    expect(await screen.findByText(/Hoy ya pasó 1 clase/)).toBeInTheDocument()
  })

  /**
   * El bug que destapó cargar un día de verdad: nueve de once clases decían
   * tener entregadas máquinas que nadie había sacado.
   *
   * La misma PC la reservan varias clases a lo largo del día. Marcando por
   * EQUIPO, una entrega a las once pintaba de verde el día entero — y el
   * botón ofrecía "Entregar (2)" cuando faltaban las diez. Lo que dice si una
   * clase ya recibió sus máquinas es su propia reserva, no el equipo.
   */
  it("no da por entregada una clase porque su equipo salió con otra", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          id: "res-tarde",
          reservaGrupoId: "grupo-tarde",
          horaInicio: "10:00",
          horaFin: "11:00",
          materiaNombre: "Física",
        }),
      ])
    )
    // El mismo equipo (pc1), pero prestado por OTRA reserva.
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ reservaId: "res-de-la-mañana" })],
    })
    renderPanel()

    expect(await screen.findByText(/0 entregadas de 1 computadora/)).toBeInTheDocument()
  })

  /**
   * Y la otra mitad: tampoco se puede ofrecer para entregar, porque la
   * máquina está físicamente afuera. Decir por qué es más útil que dejarla
   * en "sin retirar" y que la entrega falle contra el servidor.
   *
   * En una clase que todavía no empezó eso se cuenta, no se enumera: sus
   * máquinas están con la clase de ahora y van a volver antes.
   */
  it("cuenta las que siguen afuera en vez de ofrecer entregarlas", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          id: "res-tarde",
          reservaGrupoId: "grupo-tarde",
          horaInicio: "10:00",
          horaFin: "11:00",
        }),
      ])
    )
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ reservaId: "res-de-la-mañana" })],
    })
    renderPanel()

    expect(await screen.findByText(/1 todavía con otra clase/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /Entregar/ })).not.toBeInTheDocument()
  })

  /**
   * El detalle equipo por equipo solo en la clase que está pasando: es la
   * única en la que alguien va a ir a buscar una máquina ahora.
   *
   * Con un día a full eran doce chips naranjas por cada clase futura, ciento
   * treinta píxeles cada una, contando algo que a las once no se hace.
   */
  it("nombra máquina por máquina solo en la clase en curso", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        // En curso a las 8:30.
        reserva(),
        // A las 10, con su propio equipo.
        reserva({
          id: "res-tarde",
          reservaGrupoId: "grupo-tarde",
          equipoId: "pc2",
          identificador: 2,
          horaInicio: "10:00",
          horaFin: "11:00",
        }),
      ])
    )
    renderPanel()

    expect(await screen.findByText("PC 1 · Carro 1 sin retirar")).toBeInTheDocument()
    expect(screen.queryByText("PC 2 · Carro 1 sin retirar")).not.toBeInTheDocument()
    // Pero el resumen está en las dos: es lo que reemplaza a los chips.
    expect(screen.getAllByText(/0 entregadas de 1 computadora/)).toHaveLength(2)
  })

  /**
   * El identificador es el número del zócalo, así que "PC 1" existe una vez
   * por carro. En esta pantalla alguien va FÍSICAMENTE a buscar la máquina:
   * tres chips que dicen "PC 1" no le dicen a dónde ir. El dato ya venía en la
   * respuesta (`carroNombre`), solo que no se dibujaba.
   */
  it("dice de qué carro es cada equipo de la clase", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({ id: "r1", equipoId: "pc-a", carroNombre: "Carro 1" }),
        reserva({ id: "r2", equipoId: "pc-b", carroNombre: "Carro EDUTEC" }),
      ])
    )

    renderPanel()

    expect(await screen.findByText("PC 1 · Carro 1 sin retirar")).toBeInTheDocument()
    expect(await screen.findByText("PC 1 · Carro EDUTEC sin retirar")).toBeInTheDocument()
  })
})
