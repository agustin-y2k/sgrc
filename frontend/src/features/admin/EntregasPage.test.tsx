import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { EntregasPage } from "@/features/admin/EntregasPage"
import * as inventoryApi from "@/features/inventory/api"
import * as reservasApi from "@/features/reservas/api"
import type { Prestamo, ReservaDetallada } from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")
vi.mock("@/features/inventory/api")

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    pcId: "pc1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-07T08:05:00Z",
    abierto: true,
    demorado: false,
    pcIdentificador: 3,
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.pcIdentificador ?? 3}`,
    ...over,
  }
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    pcId: "pc1",
    fecha: "2026-08-07",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    pcIdentificador: 3,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "5°A",
    etiqueta: `PC ${over.pcIdentificador ?? 3}`,
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <EntregasPage />
    </QueryClientProvider>
  )
}

describe("EntregasPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.entregarPorReserva).mockResolvedValue({ entregadas: [] })
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({ entregadas: [] })
    vi.mocked(reservasApi.recibirPCs).mockResolvedValue({ recibidos: [] })
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({
      data: [
        {
          id: "pc1",
          carroId: "c1",
          identificador: 3,
          numeroSerie: "5CD1234ABC",
          etiqueta: "PC 3",
          tipo: "PC",
          reservable: true,
          freezado: false,
          estado: "DISPONIBLE",
          dadaDeBaja: false,
          fechaAlta: "2026-01-01T00:00:00Z",
        },
      ],
    })
  })

  it("muestra qué computadoras están afuera y quién las tiene", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ materiaNombre: "Matemáticas" })],
    })
    renderPagina()

    expect(await screen.findByText(/PC 3 · Carro 1/)).toBeInTheDocument()
    expect(screen.getByText(/Ada Lovelace/)).toBeInTheDocument()
  })

  it("marca la demora de las que no volvieron a horario", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ demorado: true, minutosDeDemora: 25 })],
    })
    renderPagina()

    expect(await screen.findByText("25 min tarde")).toBeInTheDocument()
  })

  it("dice horas y minutos cuando la demora pasa de una hora", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ demorado: true, minutosDeDemora: 130 })],
    })
    renderPagina()

    expect(await screen.findByText("2 h 10 min tarde")).toBeInTheDocument()
  })

  /**
   * "Sin hora de devolución" no es un dato faltante: es un préstamo al que
   * no se le puede reclamar nada, y la pantalla tiene que decirlo con esas
   * palabras para que nadie lo lea como un error de carga.
   */
  it("distingue las que no tienen hora de devolución", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ motivo: "trámite" })],
    })
    renderPagina()

    expect(await screen.findByText(/sin hora de devolución/)).toBeInTheDocument()
  })

  it("recibe una computadora desde su fila", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Recibir" }))

    expect(reservasApi.recibirPCs).toHaveBeenCalledWith({
      prestamoIds: ["pr1"],
      observaciones: undefined,
    })
  })

  it("recibe varias juntas con una observación común", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo(), prestamo({ id: "pr2", pcId: "pc2", pcIdentificador: 4 })],
    })
    renderPagina()

    const casillas = await screen.findAllByRole("checkbox", { name: /Seleccionar PC/ })
    await user.click(casillas[0])
    await user.click(casillas[1])
    await user.type(screen.getByLabelText(/Observaciones/), "faltó un cargador")
    await user.click(screen.getByRole("button", { name: "Recibir las 2 seleccionadas" }))

    expect(reservasApi.recibirPCs).toHaveBeenCalledWith({
      prestamoIds: ["pr1", "pr2"],
      observaciones: "faltó un cargador",
    })
  })

  /**
   * El caso que planteó la escuela: devolver tres de cuatro no necesita
   * nada especial, la que falta simplemente sigue figurando afuera.
   */
  it("avisa cuando alguna ya figuraba adentro", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    vi.mocked(reservasApi.recibirPCs).mockResolvedValue({
      recibidos: [],
      noRecibidos: [{ prestamoId: "pr1", detalle: "esa computadora ya figura devuelta" }],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Recibir" }))

    expect(await screen.findByText(/1 ya figuraba\(n\) adentro/)).toBeInTheDocument()
  })

  it("entrega las PCs de una reserva del día", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", pcId: "pc2", pcIdentificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar todas" }))
    await user.click(screen.getByRole("button", { name: /Entregar 2 computadora/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1", "res2"],
      nombreAlternativo: undefined,
    })
  })

  /**
   * Retiro parcial: se lleva una de las dos. La otra queda disponible para
   * otro docente, que es el motivo por el que entregar es máquina por
   * máquina y no de a reserva completa.
   */
  it("permite entregar solo algunas de la reserva", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", pcId: "pc2", pcIdentificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 4/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 computadora/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res2"],
      nombreAlternativo: undefined,
    })
  })

  it("registra que se las llevó otra persona", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.type(screen.getByLabelText(/otra persona/), "Juan (alumno)")
    await user.click(screen.getByRole("button", { name: /Entregar 1 computadora/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      nombreAlternativo: "Juan (alumno)",
    })
  })

  /**
   * Una máquina ya entregada no se puede volver a entregar, y la pantalla
   * no la ofrece: se cruza por pcId contra lo que está afuera.
   */
  it("no ofrece para entregar una PC que ya está afuera", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ pcId: "pc1" })],
    })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    expect(
      await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    ).toBeInTheDocument()
  })

  it("entrega sin reserva a alguien que no tiene cuenta", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta (secretaría)")
    await user.type(screen.getByLabelText(/Para qué/), "trámite")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 computadora/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith({
      pcIds: ["pc1"],
      nombre: "Marta (secretaría)",
      motivo: "trámite",
      devolucionEstimada: undefined,
    })
  })

  /**
   * El aviso de reserva próxima no impide la entrega: el sistema no sabe
   * cuánto va a durar un trámite, así que la decisión es del Admin.
   */
  it("avisa si la PC entregada suelta tiene una reserva encima", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({
      entregadas: [prestamo()],
      avisos: [
        {
          pcId: "pc1",
          fecha: "2026-08-07",
          horaInicio: "10:00",
          horaFin: "11:00",
          docente: "Ada Lovelace",
        },
      ],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 computadora/ }))

    expect(await screen.findByText(/tiene reserva 2026-08-07 de 10:00 a 11:00/)).toBeInTheDocument()
  })

  /**
   * Un bloqueo por evaluación estatal no tiene docente: lo crea un Admin
   * sobre PCs sueltas y no hay nadie esperando para retirarlas. Ofrecerlo
   * para entregar terminaba en un 400 que además tumbaba el lote entero.
   */
  it("no ofrece los bloqueos por evaluación para entregar", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          id: "bloq1",
          tipo: "EVALUACION_ESTATAL",
          materiaNombre: undefined,
          nombreDocenteSnapshot: undefined,
        }),
      ])
    )
    renderPagina()

    expect(
      await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    ).toBeInTheDocument()
  })

  /**
   * El listado pagina de a 50 por defecto y un día con ocho clases de ocho
   * máquinas son 64 reservas: sin pedir el máximo, las últimas del día no
   * aparecían para entregar y nada lo avisaba.
   */
  it("pide el máximo de reservas del día para no perder ninguna", async () => {
    renderPagina()

    await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    expect(reservasApi.listarReservas).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 200 })
    )
  })

  it("explica el estado cuando no hay nada afuera", async () => {
    renderPagina()

    expect(
      await screen.findByText("No hay ninguna computadora entregada.")
    ).toBeInTheDocument()
  })
})
