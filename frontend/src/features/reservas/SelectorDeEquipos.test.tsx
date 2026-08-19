import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as reservasApi from "@/features/reservas/api"
import { SelectorDeEquipos } from "@/features/reservas/SelectorDeEquipos"
import type { EquipoDisponible, EquipoOcupado } from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/reservas/api")

function libre(over: Partial<EquipoDisponible> = {}): EquipoDisponible {
  return {
    equipoId: "pc1",
    identificador: 1,
    etiqueta: "PC 1",
    carroId: "c1",
    carroNombre: "Carro 1",
    freezado: false,
    tramo: "NEUTRAL",
    ...over,
  }
}

function ocupado(over: Partial<EquipoOcupado> = {}): EquipoOcupado {
  return {
    equipoId: "pc2",
    etiqueta: "PC 2",
    carroNombre: "Carro 1",
    reservaId: "res-de-otro",
    docenteNombre: "Grace Hopper",
    materiaNombre: "Programación",
    horaInicio: "08:00",
    horaFin: "10:00",
    puedePedirse: true,
    ...over,
  }
}

function renderSelector(
  respuesta: { data: EquipoDisponible[]; ocupados?: EquipoOcupado[] } = {
    data: [libre()],
  }
) {
  vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue(respuesta)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onCambio = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <SelectorDeEquipos
        fecha="2026-08-11"
        horaInicio="08:00"
        horaFin="09:00"
        seleccionadas={[]}
        onCambio={onCambio}
      />
    </QueryClientProvider>
  )
  return { onCambio }
}

/** RF-04.11 y RF-04.12: la mitad ocupada de la franja. */
describe("SelectorDeEquipos, los que ya tienen dueño", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reservasApi.pedirLiberacion).mockResolvedValue(undefined)
  })

  it("muestra quién tiene cada equipo y en qué franja", async () => {
    renderSelector({ data: [libre()], ocupados: [ocupado()] })

    expect(await screen.findByText("Ya reservados en esa franja")).toBeInTheDocument()
    expect(screen.getByText(/Grace Hopper/)).toBeInTheDocument()
    expect(screen.getByText(/Programación/)).toBeInTheDocument()
    // La franja es la de la reserva que lo ocupa, que puede ser más ancha
    // que la que se pidió: 08:00–09:00 pedidas contra 08:00–10:00 tomadas.
    expect(screen.getByText("08:00 a 10:00")).toBeInTheDocument()
  })

  /**
   * El caso que motivó la funcionalidad: sin equipos libres, antes solo se
   * veía "no hay ninguno" y la pantalla no ofrecía ninguna salida.
   */
  it("sin ninguno libre, sigue mostrando los tomados en vez de darse por vencida", async () => {
    renderSelector({ data: [], ocupados: [ocupado()] })

    expect(
      await screen.findByText(/No queda ninguno libre en esa franja/)
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Pedírselo" })).toBeInTheDocument()
    expect(screen.queryByText("No hay ningún equipo libre en esa franja.")).toBeNull()
  })

  it("sin libres ni tomados, no hay nada que ofrecer", async () => {
    renderSelector({ data: [], ocupados: [] })

    expect(
      await screen.findByText("No hay ningún equipo libre en esa franja.")
    ).toBeInTheDocument()
  })

  it("manda el pedido con el mensaje que se escribió", async () => {
    const user = userEvent.setup()
    renderSelector({ data: [], ocupados: [ocupado()] })

    await user.click(await screen.findByRole("button", { name: "Pedírselo" }))
    await user.type(
      screen.getByLabelText("¿Para qué lo necesitás? (opcional)"),
      "Tengo una evaluación"
    )
    await user.click(screen.getByRole("button", { name: "Enviar pedido" }))

    expect(reservasApi.pedirLiberacion).toHaveBeenCalledWith(
      "res-de-otro",
      "Tengo una evaluación"
    )
  })

  it("el mensaje es opcional", async () => {
    const user = userEvent.setup()
    renderSelector({ data: [], ocupados: [ocupado()] })

    await user.click(await screen.findByRole("button", { name: "Pedírselo" }))
    await user.click(screen.getByRole("button", { name: "Enviar pedido" }))

    expect(reservasApi.pedirLiberacion).toHaveBeenCalledWith("res-de-otro", "")
  })

  /**
   * El pedido no cambia ninguna reserva (RF-04.12), y decirlo importa: si la
   * pantalla diera a entender que el equipo ya quedó libre, el docente se
   * presentaría a buscarlo.
   */
  it("al mandarlo, aclara que la reserva sigue siendo del otro", async () => {
    const user = userEvent.setup()
    renderSelector({ data: [], ocupados: [ocupado()] })

    await user.click(await screen.findByRole("button", { name: "Pedírselo" }))
    await user.click(screen.getByRole("button", { name: "Enviar pedido" }))

    expect(await screen.findByText(/La reserva sigue siendo suya/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Enviar pedido" })).toBeNull()
  })

  it("muestra el error del backend sin perder lo escrito", async () => {
    vi.mocked(reservasApi.pedirLiberacion).mockRejectedValue(
      new ApiError(409, "ya le pediste esos equipos hoy")
    )
    const user = userEvent.setup()
    renderSelector({ data: [], ocupados: [ocupado()] })

    await user.click(await screen.findByRole("button", { name: "Pedírselo" }))
    await user.type(
      screen.getByLabelText("¿Para qué lo necesitás? (opcional)"),
      "Otra vez"
    )
    await user.click(screen.getByRole("button", { name: "Enviar pedido" }))

    expect(await screen.findByText("ya le pediste esos equipos hoy")).toBeInTheDocument()
    expect(screen.getByLabelText("¿Para qué lo necesitás? (opcional)")).toHaveValue(
      "Otra vez"
    )
  })

  it("se puede volver atrás sin mandar nada", async () => {
    const user = userEvent.setup()
    renderSelector({ data: [], ocupados: [ocupado()] })

    await user.click(await screen.findByRole("button", { name: "Pedírselo" }))
    await user.click(screen.getByRole("button", { name: "Cancelar" }))

    expect(await screen.findByRole("button", { name: "Pedírselo" })).toBeInTheDocument()
    expect(reservasApi.pedirLiberacion).not.toHaveBeenCalled()
  })

  /**
   * `puedePedirse` es false en un bloqueo, en una reserva propia y cuando la
   * franja ya empezó.
   */
  it("no ofrece pedir lo que el servidor marcó como no pedible", async () => {
    renderSelector({ data: [], ocupados: [ocupado({ puedePedirse: false })] })

    expect(await screen.findByText(/Grace Hopper/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Pedírselo" })).toBeNull()
  })

  /**
   * Un bloqueo administrativo no tiene docente detrás, así que lo que explica
   * la franja es el motivo que escribió el Admin (RF-04.7).
   */
  it("en un bloqueo muestra el motivo en lugar de un docente", async () => {
    renderSelector({
      data: [],
      ocupados: [
        ocupado({
          docenteNombre: undefined,
          materiaNombre: undefined,
          motivo: "Jornada docente",
          puedePedirse: false,
        }),
      ],
    })

    expect(await screen.findByText("Jornada docente")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Pedírselo" })).toBeNull()
  })

  it("sin motivo ni docente, dice que lo tomó administración", async () => {
    renderSelector({
      data: [],
      ocupados: [
        ocupado({
          docenteNombre: undefined,
          materiaNombre: undefined,
          puedePedirse: false,
        }),
      ],
    })

    expect(await screen.findByText("Tomado por administración")).toBeInTheDocument()
  })

  /**
   * Cada tarjeta lleva su propio estado: abrir una no puede abrir las demás,
   * y el mensaje escrito en una no puede irse en el pedido de la otra.
   */
  it("cada equipo tomado se pide por separado", async () => {
    const user = userEvent.setup()
    renderSelector({
      data: [],
      ocupados: [
        ocupado(),
        ocupado({ equipoId: "pc3", etiqueta: "PC 3", reservaId: "res-de-otro-mas" }),
      ],
    })

    const botones = await screen.findAllByRole("button", { name: "Pedírselo" })
    expect(botones).toHaveLength(2)

    await user.click(botones[1])

    expect(screen.getAllByRole("button", { name: "Pedírselo" })).toHaveLength(1)
    await user.click(screen.getByRole("button", { name: "Enviar pedido" }))
    expect(reservasApi.pedirLiberacion).toHaveBeenCalledWith("res-de-otro-mas", "")
  })

  // Los tomados no son una segunda lista para tildar: no tienen casilla.
  it("no se pueden tildar", async () => {
    renderSelector({ data: [libre()], ocupados: [ocupado()] })

    await screen.findByText("Ya reservados en esa franja")
    expect(screen.getAllByRole("checkbox")).toHaveLength(1)
  })
})

/**
 * RF-03.21 — la lista se parte en bloques según qué materia prefiere cada
 * equipo.
 */
describe("SelectorDeEquipos, tramos de preferencia", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("titula los bloques cuando hay marcas de preferencia", async () => {
    renderSelector({
      data: [
        libre({
          equipoId: "pc1",
          etiqueta: "PC 1",
          tramo: "PREFERENTE",
          motivo: "Preferente para Matemática de 3°B",
        }),
        libre({ equipoId: "pc2", etiqueta: "PC 2", tramo: "NEUTRAL" }),
        libre({
          equipoId: "pc3",
          etiqueta: "PC 3",
          tramo: "DE_OTRA_MATERIA",
          motivo: "Preferente para Dibujo Técnico",
        }),
      ],
    })

    expect(await screen.findByText("Recomendados para esta materia")).toBeInTheDocument()
    expect(screen.getByText("Los demás equipos")).toBeInTheDocument()
    expect(screen.getByText("Preferentes de otra materia")).toBeInTheDocument()
    // El motivo explica el orden. Sin él se ve que la lista cambió pero no
    // qué la ordenó.
    expect(screen.getByText("Preferente para Matemática de 3°B")).toBeInTheDocument()
  })

  /**
   * "Preferentes de otra materia" se lee como una prohibición si no se
   * aclara, y no lo es: la marca sólo ordena.
   */
  it("aclara que los de otra materia se pueden reservar igual", async () => {
    renderSelector({
      data: [
        libre({ equipoId: "pc1", tramo: "NEUTRAL" }),
        libre({ equipoId: "pc3", etiqueta: "PC 3", tramo: "DE_OTRA_MATERIA" }),
      ],
    })

    expect(await screen.findByText(/Se pueden reservar igual/)).toBeInTheDocument()
  })

  it("deja tildar un equipo preferente de otra materia", async () => {
    const user = userEvent.setup()
    const { onCambio } = renderSelector({
      data: [
        libre({ equipoId: "pc1", tramo: "NEUTRAL" }),
        libre({ equipoId: "pc3", etiqueta: "PC 3", tramo: "DE_OTRA_MATERIA" }),
      ],
    })

    await user.click(await screen.findByRole("checkbox", { name: /PC 3/ }))

    expect(onCambio).toHaveBeenCalledWith(["pc3"])
  })

  /**
   * Es el caso normal mientras el inventario no tenga ninguna marca: sin nada
   * que distinguir, ponerle un título al único bloque sería vocabulario nuevo
   * que no dice nada.
   */
  it("sin marcas no muestra ningún título de tramo", async () => {
    renderSelector({
      data: [libre({ equipoId: "pc1" }), libre({ equipoId: "pc2", etiqueta: "PC 2" })],
    })

    expect(await screen.findByRole("checkbox", { name: /PC 1/ })).toBeInTheDocument()
    expect(screen.queryByText("Los demás equipos")).not.toBeInTheDocument()
    expect(screen.queryByText("Recomendados para esta materia")).not.toBeInTheDocument()
  })
})
