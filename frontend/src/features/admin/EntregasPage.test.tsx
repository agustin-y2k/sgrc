import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { EntregasPage } from "@/features/admin/EntregasPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo } from "@/features/inventory/types"
import * as reservasApi from "@/features/reservas/api"
import type { Prestamo, ReservaDetallada } from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")
vi.mock("@/features/inventory/api")

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    equipoId: "pc1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-07T08:05:00Z",
    abierto: true,
    demorado: false,
    identificador: 3,
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: "2026-08-07",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    identificador: 3,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "5°A",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function equipoSuelto(over: Partial<Equipo> = {}): Equipo {
  return {
    id: "eq1",
    nombre: "Proyector Epson",
    etiqueta: "Proyector Epson",
    tipo: "PROYECTOR",
    reservable: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
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
    vi.mocked(reservasApi.recibirEquipos).mockResolvedValue({ recibidos: [] })
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    // Una sola consulta trae todo el inventario: la de carro y las sueltas.
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
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
          dadoDeBaja: false,
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

    expect(reservasApi.recibirEquipos).toHaveBeenCalledWith({
      prestamoIds: ["pr1"],
      observaciones: undefined,
    })
  })

  it("recibe varias juntas con una observación común", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo(), prestamo({ id: "pr2", equipoId: "pc2", identificador: 4 })],
    })
    renderPagina()

    const casillas = await screen.findAllByRole("checkbox", { name: /Seleccionar PC/ })
    await user.click(casillas[0])
    await user.click(casillas[1])
    await user.type(screen.getByLabelText(/Observaciones/), "faltó un cargador")
    await user.click(screen.getByRole("button", { name: "Recibir las 2 seleccionadas" }))

    expect(reservasApi.recibirEquipos).toHaveBeenCalledWith({
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
    vi.mocked(reservasApi.recibirEquipos).mockResolvedValue({
      recibidos: [],
      noRecibidos: [{ prestamoId: "pr1", detalle: "esa computadora ya figura devuelta" }],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Recibir" }))

    expect(await screen.findByText(/1 ya figuraba\(n\) adentro/)).toBeInTheDocument()
  })

  it("entrega los equipos de una reserva del día", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar todas" }))
    await user.click(screen.getByRole("button", { name: /Entregar 2 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1", "res2"],
      retiradoPor: undefined,
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
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 4/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res2"],
      retiradoPor: undefined,
    })
  })

  /**
   * El docente manda a un alumno, que es lo habitual. Se anota quién vino,
   * pero el responsable sigue siendo él: reservó él y a él se le reclama si
   * los equipos no vuelven. La pantalla lo dice para que nadie crea que
   * escribir ese nombre le pasa la responsabilidad al alumno.
   */
  it("anota quién retira sin cambiar de quién es la responsabilidad", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.type(screen.getByLabelText(/Quién las retira/), "Juan (alumno)")
    expect(screen.getByText(/quedan igual a cargo del docente/i)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: "Juan (alumno)",
    })
  })

  // Es opcional: a una institución le sirve anotar al alumno y a otra le
  // sobra. Obligarlo llevaría a que se escriba cualquier cosa para seguir.
  it("se puede entregar sin anotar quién retira", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: undefined,
    })
  })

  /**
   * Una máquina ya entregada no se puede volver a entregar, y la pantalla
   * no la ofrece: se cruza por equipoId contra lo que está afuera.
   */
  it("no ofrece para entregar un equipo que ya está afuera", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ equipoId: "pc1" })],
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
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith({
      equipoIds: ["pc1"],
      nombre: "Marta (secretaría)",
      motivo: "trámite",
      devolucionEstimada: undefined,
    })
  })

  /**
   * El aviso de reserva próxima no impide la entrega: el sistema no sabe
   * cuánto va a durar un trámite, así que la decisión es del Admin.
   */
  it("avisa si el equipo entregado suelta tiene una reserva encima", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({
      entregadas: [prestamo()],
      avisos: [
        {
          equipoId: "pc1",
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
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(await screen.findByText(/tiene reserva 2026-08-07 de 10:00 a 11:00/)).toBeInTheDocument()
  })

  /**
   * Un bloqueo administrativo no tiene docente: lo crea un Admin
   * sobre equipos sueltas y no hay nadie esperando para retirarlas. Ofrecerlo
   * para entregar terminaba en un 400 que además tumbaba el lote entero.
   */
  it("no ofrece los bloqueos administrativos para entregar", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          id: "bloq1",
          tipo: "BLOQUEO",
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
      await screen.findByText("No hay ningún equipo entregado.")
    ).toBeInTheDocument()
  })

  /**
   * Entregar contra una reserva ya liberada es legítimo —el docente llegó
   * tarde y el equipo seguía ahí— pero en ese rato otro pudo reservarlo. Sin
   * mostrar el aviso que manda el backend, el Admin se lo entrega al primero
   * y el segundo se encuentra con que no está.
   */
  it("avisa si el equipo que se entrega tiene una reserva de otro encima", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarPorReserva).mockResolvedValue({
      entregadas: [],
      avisos: [
        {
          equipoId: "pc1",
          fecha: "2026-08-11",
          horaInicio: "10:00",
          horaFin: "11:00",
          docente: "Grace Hopper",
        },
      ],
    })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar todas" }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(await screen.findByText(/Grace Hopper/)).toBeInTheDocument()
    expect(screen.getByText(/tiene reserva/)).toBeInTheDocument()
  })

  /**
   * Los dos nombres, y en este orden: primero quien responde, después quien
   * pasó a buscarlo. Con uno solo, quien lee la lista no sabe a quién
   * reclamarle.
   */
  it("la lista de afuera nombra a quien responde y a quien retiró", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [
        prestamo({
          entregadoANombre: "Ada Lovelace",
          retiradoPor: "Juan (alumno)",
        }),
      ],
    })
    renderPagina()

    expect(await screen.findByText(/Ada Lovelace · retiró Juan \(alumno\)/)).toBeInTheDocument()
  })

  // Sin nadie anotado no se inventa un renglón: lo retiró quien responde.
  it("no dice nada de quién retiró cuando no se anotó", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ entregadoANombre: "Ada Lovelace" })],
    })
    renderPagina()

    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument()
    expect(screen.queryByText(/retiró/)).not.toBeInTheDocument()
  })

  /**
   * Lo que más se presta de forma espontánea no son las computadoras de un
   * carro: es el proyector, el cargador, la notebook suelta (RF-03.16). Si
   * la entrega sin reserva solo ofreciera los carros, justamente el caso
   * principal habría que seguir anotándolo en papel.
   */
  it("ofrece también los equipos que no están en ningún carro", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto()],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    expect(
      await screen.findByRole("checkbox", { name: /^Proyector Epson/ })
    ).toBeInTheDocument()
  })

  // No se filtra por `reservable`: un cargador no se reserva pero sí se
  // presta, y ese es su caso principal.
  it("ofrece lo que no se puede reservar pero sí prestar", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto({ id: "eq2", nombre: "Cargador", etiqueta: "Cargador", reservable: false })],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    expect(await screen.findByRole("checkbox", { name: /^Cargador/ })).toBeInTheDocument()
  })

  // Un equipo suelto que ya salió no se puede volver a entregar, igual que
  // una computadora de un carro.
  it("no ofrece un equipo suelto que ya está afuera", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto()],
    })
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ equipoId: "eq1", etiqueta: "Proyector Epson" })],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    // El único que queda con ese nombre es el de "Afuera del laboratorio",
    // que empieza con "Seleccionar": el de la lista de entrega no está.
    expect(
      screen.queryByRole("checkbox", { name: /^Proyector Epson/ })
    ).not.toBeInTheDocument()
  })

  /**
   * El listado sale de UNA consulta al inventario, no de una por carro más
   * otra por los sueltos. Con ocho carros eran nueve idas al servidor para
   * dibujar una lista de casillas, y bastaba con que una fallara para que
   * faltaran equipos sin que nada lo dijera.
   */
  it("arma la lista con una sola consulta al inventario", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await screen.findByRole("checkbox", { name: /^PC 3/ })

    expect(inventoryApi.listarEquipos).toHaveBeenCalledTimes(1)
    expect(inventoryApi.listarEquiposDeCarro).not.toHaveBeenCalled()
  })
})
