import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { OtrosEquipos } from "@/features/admin/OtrosEquipos"
import * as inventoryApi from "@/features/inventory/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Equipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")
vi.mock("@/features/auth/AuthContext")

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
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.crearEquipoSuelto).mockResolvedValue(equipo())
    vi.mocked(adminApi.editarEquipo).mockResolvedValue(undefined)
    vi.mocked(adminApi.darDeBajaEquipo).mockResolvedValue({
      reservasCanceladas: 0,
      docentesNotificados: 0,
    })
    vi.mocked(useAuth).mockReturnValue({
      user: { id: "u1", rol: "ADMIN" },
      isLoading: false,
    } as unknown as ReturnType<typeof useAuth>)
    vi.mocked(inventoryApi.listarCuentasDeEquipo).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarClasesDeCuenta).mockResolvedValue({ data: [] })
  })

  it("muestra los equipos con y sin carro por igual", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
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
   * se pide en el momento.
   */
  it("distingue lo reservable de lo que solo se presta", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipo(),
        equipo({
          id: "eq2",
          nombre: "Cargador 1",
          etiqueta: "Cargador 1",
          reservable: false,
        }),
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
      numeroSerie: "",
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
      numeroSerie: "",
      reservable: false,
    })
  })

  it("ofrece los tipos que ya existen", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipo(), equipo({ id: "eq2", tipo: "CARGADOR", nombre: "Cargador 1" })],
    })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))

    // El datalist evita terminar con "PROYECTOR" y "Proyector" como dos tipos
    // distintos, igual que con los nombres de las licencias.
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
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    const nombre = screen.getByLabelText("¿Cómo lo llaman?")
    await user.clear(nombre)
    await user.type(nombre, "Proyector del SUM")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(adminApi.editarEquipo).toHaveBeenCalledWith("eq1", {
      tipo: "PROYECTOR",
      nombre: "Proyector del SUM",
      numeroSerie: "",
      reservable: true,
    })
  })

  it("el formulario arranca con lo que el equipo ya tenía", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))

    expect(screen.getByLabelText("¿Qué es?")).toHaveValue("PROYECTOR")
    expect(screen.getByLabelText("¿Cómo lo llaman?")).toHaveValue("Proyector Epson")
    expect(screen.getByRole("checkbox", { name: /Se puede reservar/ })).toBeChecked()
  })

  /**
   * El número de serie es opcional para CUALQUIER tipo, no solo para las
   * notebooks: un proyector tiene serie —y es de lo que más se extravía— y un
   * cargador no tiene ninguna. Por eso es un campo que se llena o no, y no dos
   * categorías de equipo.
   */
  it("permite dar de alta con número de serie", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "NOTEBOOK")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Notebook Dirección")
    await user.type(screen.getByLabelText(/Número de serie/), "ABC-123X")
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    expect(adminApi.crearEquipoSuelto).toHaveBeenCalledWith({
      tipo: "NOTEBOOK",
      nombre: "Notebook Dirección",
      numeroSerie: "ABC-123X",
      reservable: false,
    })
  })

  it("dar de alta sin número de serie sigue funcionando", async () => {
    // El cargador: no tiene serie y no hay ninguna que inventar.
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Agregar equipo" }))
    await user.type(screen.getByLabelText("¿Qué es?"), "CARGADOR")
    await user.type(screen.getByLabelText("¿Cómo lo llaman?"), "Cargador 2")
    await user.click(screen.getByRole("button", { name: "Agregar" }))

    // Sin serie se manda vacío, que el backend guarda como NULL. La columna es
    // UNIQUE, y en Postgres eso permite tantos NULL como haga falta: veinte
    // cargadores sin serie conviven sin chocar entre sí.
    expect(adminApi.crearEquipoSuelto).toHaveBeenCalledWith({
      tipo: "CARGADOR",
      nombre: "Cargador 2",
      numeroSerie: "",
      reservable: false,
    })
  })

  /**
   * Los equipos que ya estaban cargados no tienen serie. Sin poder editarla
   * habría que darlos de baja y recrearlos solo para anotarla, perdiendo su
   * historial de préstamos e incidencias.
   */
  it("permite cargarle el número de serie a un equipo que ya existía", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.type(screen.getByLabelText(/Número de serie/), "XYZ-9")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(adminApi.editarEquipo).toHaveBeenCalledWith("eq1", {
      tipo: "PROYECTOR",
      nombre: "Proyector Epson",
      numeroSerie: "XYZ-9",
      reservable: true,
    })
  })

  it("muestra el número de serie en la lista, y nada si no tiene", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipo({ numeroSerie: "ABC-123X" }),
        equipo({ id: "eq2", tipo: "CARGADOR", nombre: "Cargador 1" }),
      ],
    })
    renderSeccion()

    // Es lo que sirve para reclamar un equipo perdido, así que tiene que
    // verse sin entrar a editar.
    expect(await screen.findByText("ABC-123X")).toBeInTheDocument()
    // Y el que no tiene no muestra un hueco: una línea "Serie: —" en cada
    // cargador es ruido en la lista que más se mira.
    expect(screen.queryByText("—")).not.toBeInTheDocument()
  })

  /**
   * Destildar "se puede reservar" no cancela las reservas que ya existen —el
   * backend solo lo saca de la lista de libres—.
   */
  it("avisa que quitar lo reservable no toca las reservas ya hechas", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.click(screen.getByRole("checkbox", { name: /Se puede reservar/ }))

    expect(
      screen.getByText(/Las reservas que ya tenga siguen en pie/)
    ).toBeInTheDocument()
  })

  it("da de baja pidiendo confirmación primero", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))
    expect(adminApi.darDeBajaEquipo).not.toHaveBeenCalled()

    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))
    expect(adminApi.darDeBajaEquipo).toHaveBeenCalledWith("eq1")
  })

  /**
   * El backend rechaza la baja de algo que está afuera: dejaría el préstamo
   * abierto sin ninguna pantalla desde donde cerrarlo.
   */
  it("advierte qué pasa si el equipo está prestado", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))

    expect(screen.getByText(/marcá primero que volvió/)).toBeInTheDocument()
  })

  /** Dar de baja un proyector reservado cancela clases de otros. */
  it("dice cuántas reservas se cancelaron al dar de baja", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
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
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
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

  // RF-03.22: un equipo suelto es el que alguien se lleva, así que saber con
  // qué cuenta se abre importa acá tanto como en las PCs del carro.
  it("abre las cuentas de un equipo suelto", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: "Cómo entrar" }))

    expect(await screen.findByText("Cómo entrar a Proyector Epson")).toBeInTheDocument()
    expect(inventoryApi.listarCuentasDeEquipo).toHaveBeenCalledWith("eq1")
  })

  // ── Estado (RF-03.3 / RF-03.8) ────────────────────────────────────────
  // Un proyector también se rompe. Antes lo único que se podía hacer con él
  // era darlo de baja, que además borra su historial de la vista y libera su
  // nombre para el equipo que lo reemplace.

  it("dice en qué estado está cada equipo", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipo(),
        equipo({
          id: "eq2",
          nombre: "Notebook chica",
          etiqueta: "Notebook chica",
          estado: "EN_MANTENIMIENTO",
        }),
      ],
    })
    renderSeccion()

    expect(await screen.findByText("Disponible")).toBeInTheDocument()
    expect(screen.getByText("En mantenimiento")).toBeInTheDocument()
  })

  it("pasar a mantenimiento pide confirmación y avisa de la cascada", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: /En mantenimiento/ }))

    expect(adminApi.cambiarEstadoEquipo).not.toHaveBeenCalled()
    expect(screen.getByText(/cancela sus reservas futuras/)).toBeInTheDocument()
  })

  it("confirmado, manda el estado nuevo y el motivo", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))
    await user.type(screen.getByLabelText(/Motivo/), "se quemó la lámpara")
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(adminApi.cambiarEstadoEquipo).toHaveBeenCalledWith(
      "eq1",
      "FUERA_DE_SERVICIO",
      "se quemó la lámpara"
    )
  })

  /**
   * Un equipo fuera de servicio vuelve a circulación cuando se arregla: el
   * caso real fue una máquina sin batería que anduvo en cuanto apareció una.
   * Lo irreversible es darla de baja, no este estado.
   */
  it("un equipo fuera de servicio puede volver a circulación", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipo({ estado: "FUERA_DE_SERVICIO" })],
    })
    renderSeccion()

    await user.click(await screen.findByRole("button", { name: /→ Disponible/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(adminApi.cambiarEstadoEquipo).toHaveBeenCalledWith(
      "eq1",
      "DISPONIBLE",
      undefined
    )
  })

  /**
   * Lo único que no es una transición es repetir el estado que ya se tiene:
   * ese botón sí sobra.
   */
  it("no ofrece cambiar al estado en el que el equipo ya está", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipo({ estado: "FUERA_DE_SERVICIO" })],
    })
    renderSeccion()

    await screen.findByText("Proyector Epson")
    expect(
      screen.queryByRole("button", { name: /→ Fuera de servicio/ })
    ).not.toBeInTheDocument()
  })
})
