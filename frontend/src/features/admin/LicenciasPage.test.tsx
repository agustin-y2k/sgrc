import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as adminApi from "@/features/admin/api"
import { LicenciasPage } from "@/features/admin/LicenciasPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Licencia } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")

function licencia(over: Partial<Licencia> = {}): Licencia {
  return {
    id: "lic1",
    equipoId: "pc1",
    nombre: "AutoCAD 2027",
    diasDuracion: 30,
    diasAviso: 1,
    fechaVencimiento: "2026-09-03",
    diasRestantes: 7,
    estado: "VIGENTE",
    identificador: 3,
    carroId: "c1",
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <LicenciasPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("LicenciasPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    // El inventario entero, en una sola consulta: la lista incluye lo que no
    // está en ningún carro, que también puede tener software licenciado.
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
        {
          id: "suelto1",
          etiqueta: "Notebook de dirección",
          nombre: "Notebook de dirección",
          tipo: "NOTEBOOK",
          reservable: true,
          freezado: false,
          estado: "DISPONIBLE",
          dadoDeBaja: false,
          fechaAlta: "2026-01-01T00:00:00Z",
        },
      ],
    })
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({ data: [licencia()] })
    vi.mocked(adminApi.crearLicencias).mockResolvedValue({ creadas: [licencia()] })
    vi.mocked(adminApi.renovarLicencias).mockResolvedValue({ renovadas: [licencia()] })
    vi.mocked(adminApi.editarLicencia).mockResolvedValue(undefined)
    vi.mocked(adminApi.borrarLicencia).mockResolvedValue(undefined)
  })

  it("muestra el contador en días, no solo la fecha", async () => {
    renderPagina()

    // "Vence el 03/09/2026" obliga a hacer la resta mentalmente, que es
    // justo el trabajo que esta pantalla existe para evitar.
    expect(await screen.findByText(/Vence en 7 días/)).toBeInTheDocument()
    expect(screen.getByText(/PC 3/)).toBeInTheDocument()
  })

  it("dice 'vence mañana' y no 'vence en 1 días'", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [licencia({ diasRestantes: 1, estado: "POR_VENCER" })],
    })
    renderPagina()

    expect(await screen.findByText(/Vence mañana/)).toBeInTheDocument()
  })

  it("dice hace cuántos días venció una licencia vencida", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [licencia({ diasRestantes: -6, estado: "VENCIDA" })],
    })
    renderPagina()

    expect(await screen.findByText(/Venció hace 6 días/)).toBeInTheDocument()
    expect(screen.getByText("Vencida")).toBeInTheDocument()
  })

  /**
   * Una licencia sin fecha es una tarea pendiente —hay que ir hasta la
   * máquina— y no una licencia "sin vencimiento". La pantalla lo tiene que
   * decir con esas palabras.
   */
  it("marca las que todavía no tienen vencimiento cargado", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [
        licencia({
          fechaVencimiento: undefined,
          diasRestantes: undefined,
          estado: "SIN_FECHA",
        }),
      ],
    })
    renderPagina()

    expect(await screen.findByText("Falta cargar el vencimiento")).toBeInTheDocument()
    expect(screen.getByText(/Sin fecha de vencimiento/)).toBeInTheDocument()
  })

  /**
   * La guarda del diseño: "Renovar" corre un contador que ya existe. Si se
   * pudiera apretar sobre una licencia sin verificar, sería la forma fácil
   * de sacarse de encima el aviso poniéndole treinta días que nadie
   * confirmó — el dato falso que todo esto evita.
   */
  it("no deja renovar una licencia sin fecha cargada", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [
        licencia({
          fechaVencimiento: undefined,
          diasRestantes: undefined,
          estado: "SIN_FECHA",
        }),
      ],
    })
    renderPagina()

    expect(await screen.findByRole("button", { name: "Renovar hoy" })).toBeDisabled()
    // Y tampoco se puede meter en una renovación masiva.
    expect(screen.getByRole("checkbox", { name: /Seleccionar AutoCAD/ })).toBeDisabled()
  })

  it("renueva una licencia desde su fila", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Renovar hoy" }))

    expect(adminApi.renovarLicencias).toHaveBeenCalledWith({ licenciaIds: ["lic1"] })
  })

  it("renueva varias de una vez", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [licencia(), licencia({ id: "lic2", equipoId: "pc2", identificador: 4 })],
    })
    renderPagina()

    const casillas = await screen.findAllByRole("checkbox", { name: /Seleccionar/ })
    await user.click(casillas[0])
    await user.click(casillas[1])

    await user.click(screen.getByRole("button", { name: "Renovar las 2 seleccionadas" }))

    expect(adminApi.renovarLicencias).toHaveBeenCalledWith({
      licenciaIds: ["lic1", "lic2"],
      renovadaEl: undefined,
    })
  })

  /**
   * El caso del olvido, que es la mitad del requerimiento: se renovaron el
   * martes y se cargan el jueves. Sin esto el contador queda dos días
   * adelantado y el aviso llega tarde.
   */
  it("permite renovar con una fecha anterior a hoy", async () => {
    const user = userEvent.setup()
    renderPagina()

    const casilla = await screen.findByRole("checkbox", { name: /Seleccionar/ })
    await user.click(casilla)
    await user.type(screen.getByLabelText("Fecha en que se renovaron"), "2026-08-04")
    await user.click(screen.getByRole("button", { name: "Renovar las 1 seleccionadas" }))

    expect(adminApi.renovarLicencias).toHaveBeenCalledWith({
      licenciaIds: ["lic1"],
      renovadaEl: "2026-08-04",
    })
  })

  it("carga la misma licencia en varios equipos de una vez", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Cargar una licencia" }))
    await user.type(screen.getByLabelText("Software"), "SolidWorks")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /Cargar en 1 equipo/ }))

    expect(adminApi.crearLicencias).toHaveBeenCalledWith({
      equipoIds: ["pc1"],
      nombre: "SolidWorks",
      diasDuracion: 30,
      diasAviso: 1,
    })
  })

  /** "Quedan 12 días" es la forma en que el dato aparece en la máquina. */
  it("acepta cargar el vencimiento por los días que faltan", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Cargar una licencia" }))
    await user.type(screen.getByLabelText("Software"), "SolidWorks")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.selectOptions(screen.getByLabelText("¿Cuándo vence?"), "quedan-dias")
    await user.type(screen.getByLabelText("Días que le quedan"), "12")
    await user.click(screen.getByRole("button", { name: /Cargar en 1 equipo/ }))

    expect(adminApi.crearLicencias).toHaveBeenCalledWith(
      expect.objectContaining({ quedanDias: 12 })
    )
  })

  /**
   * El default del formulario es "todavía no sé", no "se renovó hoy": es la
   * única opción que no inventa una fecha.
   */
  it("arranca sin declarar vencimiento", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Cargar una licencia" }))

    expect(screen.getByLabelText("¿Cuándo vence?")).toHaveValue("sin-fecha")
  })

  /**
   * RF-03.11 no distingue: una licencia se carga sobre cualquier equipo. El
   * formulario armaba su lista recorriendo los carros, así que lo que no
   * está en ninguno —una notebook suelta con un CAD licenciado— no aparecía
   * nunca, aunque la API lo aceptaba igual.
   */
  it("ofrece también los equipos que no están en ningún carro", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Cargar una licencia" }))

    expect(
      await screen.findByRole("checkbox", { name: /Notebook de dirección/ })
    ).toBeInTheDocument()
  })

  it("informa cuáles equipos ya tenían la licencia, sin tratarlo como error", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.crearLicencias).mockResolvedValue({
      creadas: [licencia()],
      equiposQueYaLaTenian: ["pc9", "pc8"],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Cargar una licencia" }))
    await user.type(screen.getByLabelText("Software"), "SolidWorks")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /Cargar en 1 equipo/ }))

    expect(await screen.findByText(/2 ya la tenían/)).toBeInTheDocument()
  })

  it("resume arriba qué hay para resolver", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({
      data: [
        licencia({ id: "l1", diasRestantes: -2, estado: "VENCIDA" }),
        licencia({ id: "l2", diasRestantes: 1, estado: "POR_VENCER" }),
        licencia({
          id: "l3",
          fechaVencimiento: undefined,
          diasRestantes: undefined,
          estado: "SIN_FECHA",
        }),
      ],
    })
    renderPagina()

    const resumen = await screen.findByText(/1 vencida/)
    expect(within(resumen).getByText(/por vencer/)).toBeTruthy
    expect(resumen.textContent).toContain("1 por vencer")
    expect(resumen.textContent).toContain("1 sin fecha cargada")
  })

  it("edita el vencimiento de una licencia ya cargada", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    const campoFecha = screen.getByLabelText("Fecha de vencimiento")
    await user.clear(campoFecha)
    await user.type(campoFecha, "2026-10-01")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(adminApi.editarLicencia).toHaveBeenCalledWith(
      "lic1",
      expect.objectContaining({ venceEl: "2026-10-01" })
    )
  })

  it("pide confirmación antes de quitar una licencia", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Quitar" }))
    expect(adminApi.borrarLicencia).not.toHaveBeenCalled()

    // Al abrir la confirmación, el botón de la fila se oculta: el único
    // "Quitar" que queda es el que confirma.
    await user.click(screen.getByRole("button", { name: "Quitar" }))
    expect(adminApi.borrarLicencia).toHaveBeenCalledWith("lic1")
  })

  it("explica qué hacer cuando no hay ninguna cargada", async () => {
    vi.mocked(adminApi.listarLicencias).mockResolvedValue({ data: [] })
    renderPagina()

    // El mensaje tiene que decir explícitamente que se puede cargar sin
    // fecha: si no, quien no sepa el vencimiento va a inventar uno.
    expect(await screen.findByText(/cargala\s+igual sin fecha/)).toBeInTheDocument()
  })
})
