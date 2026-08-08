import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { OtrosEquipos } from "@/features/admin/OtrosEquipos"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")

function equipo(over: Partial<Equipo> = {}): Equipo {
  return {
    id: "eq1",
    etiqueta: "Proyector Epson",
    tipo: "PROYECTOR",
    nombre: "Proyector Epson",
    reservable: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

function renderSeccion() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <OtrosEquipos />
    </QueryClientProvider>
  )
}

describe("OtrosEquipos", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.crearEquipoSuelto).mockResolvedValue(equipo())
    vi.mocked(adminApi.editarEquipo).mockResolvedValue(undefined)
    vi.mocked(adminApi.darDeBajaEquipo).mockResolvedValue({
      reservasCanceladas: 0,
      docentesNotificados: 0,
    })
  })

  it("muestra los equipos con y sin carro por igual", async () => {
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [
        equipo(),
        equipo({
          id: "eq2",
          tipo: "CARGADOR",
          nombre: "Cargador 1",
          etiqueta: "Cargador 1",
          reservable: false,
        }),
      ],
    })
    renderSeccion()

    expect(await screen.findByText("Proyector Epson")).toBeInTheDocument()
    expect(screen.getByText("Cargador 1")).toBeInTheDocument()
  })

  /**
   * La distinción que importa: un proyector se puede planificar, un cargador
   * se pide en el momento. Sin marcarla, todo lo que se presta al paso
   * aparecería entre las máquinas libres cada vez que alguien va a reservar.
   */
  it("distingue lo reservable de lo que solo se presta", async () => {
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [
        equipo(),
        equipo({ id: "eq2", nombre: "Cargador 1", etiqueta: "Cargador 1", reservable: false }),
      ],
    })
    renderSeccion()

    expect(await screen.findByText("Se puede reservar")).toBeInTheDocument()
    expect(screen.getByText("Solo préstamo")).toBeInTheDocument()
  })

  it("da de alta un proyector reservable", async () => {
    const user = userEvent.setup()
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "PROYECTOR")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Proyector Epson")
    await user.click(screen.getByRole("checkbox", { name: /Se puede reservar/ }))
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    expect(adminApi.crearEquipoSuelto).toHaveBeenCalledWith({
      tipo: "PROYECTOR",
      nombre: "Proyector Epson",
      reservable: true,
    })
  })

  /**
   * El default es NO reservable: la mayoría de lo que se presta suelto
   * —cargadores, notebooks de repuesto— se pide en el momento, y marcar de
   * más llena la lista de reservas de cosas que nadie planifica.
   */
  it("un equipo nuevo no es reservable salvo que se marque", async () => {
    const user = userEvent.setup()
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "CARGADOR")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Cargador 1")
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    expect(adminApi.crearEquipoSuelto).toHaveBeenCalledWith({
      tipo: "CARGADOR",
      nombre: "Cargador 1",
      reservable: false,
    })
  })

  it("ofrece los tipos que ya existen", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({
      data: [equipo(), equipo({ id: "eq2", tipo: "CARGADOR", nombre: "Cargador 1" })],
    })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))

    // El datalist evita terminar con "PROYECTOR" y "Proyector" como dos
    // tipos distintos, igual que con los nombres de las licencias. Se
    // consulta por selector: las opciones de un datalist no son accesibles
    // por rol, están asociadas al input y no visibles en el árbol.
    const opciones = document.querySelectorAll("#tipos-de-equipo option")
    expect([...opciones].map((o) => o.getAttribute("value"))).toEqual([
      "CARGADOR",
      "PROYECTOR",
    ])
  })

  /**
   * Sin esto, un cargador cargado con el nombre mal escrito quedaba así para
   * siempre: la única salida era darlo de baja y volver a crearlo, que además
   * le corta el historial de préstamos.
   */
  it("corrige lo que se cargó mal", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    const nombre = screen.getByLabelText("¿Cómo lo llaman?")
    await user.clear(nombre)
    await user.type(nombre, "Proyector del SUM")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(adminApi.editarEquipo).toHaveBeenCalledWith("eq1", {
      tipo: "PROYECTOR",
      nombre: "Proyector del SUM",
      reservable: true,
    })
  })

  it("el formulario arranca con lo que el equipo ya tenía", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))

    expect(screen.getByLabelText("¿Qué es?")).toHaveValue("PROYECTOR")
    expect(screen.getByLabelText("¿Cómo lo llaman?")).toHaveValue("Proyector Epson")
    expect(screen.getByRole("checkbox", { name: /Se puede reservar/ })).toBeChecked()
  })

  /**
   * Destildar "se puede reservar" no cancela las reservas que ya existen —el
   * backend solo lo saca de la lista de libres—. Si la pantalla no lo dice,
   * el Admin cree que las canceló.
   */
  it("avisa que quitar lo reservable no toca las reservas ya hechas", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.click(screen.getByRole("checkbox", { name: /Se puede reservar/ }))

    expect(screen.getByText(/Las reservas que ya tenga siguen en pie/)).toBeInTheDocument()
  })

  it("da de baja pidiendo confirmación primero", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))
    expect(adminApi.darDeBajaEquipo).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))
    expect(adminApi.darDeBajaEquipo).toHaveBeenCalledWith("eq1")
  })

  /**
   * El backend rechaza la baja de algo que está afuera: dejaría el préstamo
   * abierto sin ninguna pantalla desde donde cerrarlo. La advertencia dice de
   * antemano cuál es la salida, para no toparse con el error.
   */
  it("advierte qué pasa si el equipo está prestado", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))

    expect(screen.getByText(/marcá primero que volvió/)).toBeInTheDocument()
  })

  /**
   * Dar de baja un proyector reservado cancela clases de otros. El backend ya
   * devuelve la cuenta; no mostrarla dejaba al Admin sin saber qué se llevó
   * puesto.
   */
  it("dice cuántas reservas se cancelaron al dar de baja", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    vi.mocked(adminApi.darDeBajaEquipo).mockResolvedValue({
      reservasCanceladas: 3,
      docentesNotificados: 2,
    })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))
    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))

    expect(
      await screen.findByText(/Se cancelaron 3 reservas y se avisó a 2 docentes/)
    ).toBeInTheDocument()
  })

  // Sin reservas afectadas no hay nada que informar: un cartel que dice
  // "se cancelaron 0 reservas" es ruido en el caso normal.
  it("no dice nada si la baja no canceló ninguna reserva", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquiposSueltos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))
    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))

    expect(screen.queryByText(/Se cancelaron/)).not.toBeInTheDocument()
  })

  it("explica el estado cuando no hay ninguno", async () => {
    renderSeccion()

    expect(
      await screen.findByText("No hay ningún equipo cargado todavía.")
    ).toBeInTheDocument()
  })
})
