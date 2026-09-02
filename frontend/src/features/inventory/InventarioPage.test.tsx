import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import { InventarioPage } from "@/features/inventory/InventarioPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Carro, Equipo } from "@/features/inventory/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/inventory/api")
vi.mock("@/features/auth/AuthContext")

function sesionDe(rol: "ADMIN" | "DOCENTE") {
  vi.mocked(useAuth).mockReturnValue({
    user: { id: "u1", rol },
    isLoading: false,
  } as unknown as ReturnType<typeof useAuth>)
}

function renderInventario() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/inventario"]}>
        <Routes>
          <Route path="/inventario" element={<InventarioPage />} />
          <Route
            path="/inventario/equipos/:equipoId/calendario"
            element={<div>Calendario</div>}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const carros: Carro[] = [{ id: "c1", nombre: "Carro 1", descripcion: "Planta baja" }]

function equipo(over: Partial<Equipo>): Equipo {
  return {
    id: "pc1",
    carroId: "c1",
    identificador: 1,
    numeroSerie: "SERIE-123",
    etiqueta: `PC ${over.identificador ?? 1}`,
    tipo: "PC",
    reservable: true,
    esComputadora: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

describe("InventarioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({ data: carros })
    // Por defecto la institución no presta nada suelto, que es el caso de la
    // mayoría: los tests que sí lo necesitan lo declaran.
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
    // El caso corriente de esta pantalla es un docente mirándola.
    sesionDe("DOCENTE")
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("lista los carros", async () => {
    renderInventario()
    expect(await screen.findByText("Carro 1")).toBeInTheDocument()
    expect(screen.getByText("Planta baja")).toBeInTheDocument()
  })

  // Los equipos se piden recién al abrir el carro: cargarlas todas de entrada
  // sería una consulta por carro en cada visita a la página.
  it("no pide los equipos hasta abrir el carro", async () => {
    renderInventario()
    await screen.findByText("Carro 1")

    expect(inventoryApi.listarEquiposDeCarro).not.toHaveBeenCalled()
  })

  // RF-03.7: el software instalado es el dato por el que un docente entra.
  it("al abrir el carro muestra estado, freezado y software de cada equipo", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [equipo({ freezado: true, softwareInstalado: "AutoCAD 2027" })],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))

    expect(await screen.findByText("PC 1")).toBeInTheDocument()
    expect(screen.getByText("AutoCAD 2027")).toBeInTheDocument()
    expect(screen.getByText("Disponible")).toBeInTheDocument()
    expect(inventoryApi.listarEquiposDeCarro).toHaveBeenCalledWith("c1")
  })

  // RF-03.4: un equipo dado de baja deja de listarse como reservable.
  it("no muestra los equipos dados de baja", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [
        equipo({ id: "pc1", identificador: 1 }),
        equipo({ id: "pc2", identificador: 2, dadoDeBaja: true }),
      ],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))

    expect(await screen.findByText("PC 1")).toBeInTheDocument()
    expect(screen.queryByText("PC 2")).not.toBeInTheDocument()
  })

  it("un equipo fuera de servicio se muestra como tal", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [equipo({ estado: "FUERA_DE_SERVICIO" })],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))

    expect(await screen.findByText("Fuera de servicio")).toBeInTheDocument()
  })

  it("desde un equipo se navega a su calendario (RF-04.4)", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("link", { name: "Ver calendario" }))

    expect(await screen.findByText("Calendario")).toBeInTheDocument()
  })

  // ── Reportar una incidencia (RF-03.5) ────────────────────────────────
  // "Docentes solo pueden reportarlas": esta es la pantalla donde un docente
  // ya está mirando los equipos, así que el reporte va acá y no en una de
  // Admin.

  it("un docente reporta una falla desde el listado de Equipos", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    vi.mocked(inventoryApi.reportarIncidencia).mockResolvedValue({
      id: "i1",
      equipoId: "pc1",
      descripcion: "No arranca",
      gravedad: "GRAVE",
      fecha: "2026-08-03T10:00:00Z",
      enviadoASoporte: false,
      estado: "ABIERTA",
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    await user.type(screen.getByLabelText(/¿Qué le pasa\?/), "No arranca")
    await user.selectOptions(screen.getByLabelText("Gravedad"), "GRAVE")
    await user.click(screen.getByRole("button", { name: "Reportar" }))

    expect(inventoryApi.reportarIncidencia).toHaveBeenCalledWith({
      equipoId: "pc1",
      descripcion: "No arranca",
      gravedad: "GRAVE",
    })
    expect(await screen.findByText(/quedó registrada/)).toBeInTheDocument()
  })

  // Reportar no saca el equipo de circulación (eso es RF-03.8, y lo decide un
  // Admin).
  it("avisa que reportar no saca el equipo de circulación", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    expect(screen.getByText(/no saca el equipo de circulación/)).toBeInTheDocument()
  })

  it("no deja reportar sin describir el problema", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    expect(screen.getByRole("button", { name: "Reportar" })).toBeDisabled()
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(inventoryApi.listarCarros).mockRejectedValue(
      new ApiError(401, "token inválido o expirado")
    )
    renderInventario()

    expect(await screen.findByText("token inválido o expirado")).toBeInTheDocument()
  })

  // ── La categoría de la falla (RF-03.5) ───────────────────────────────

  /**
   * La categoría es lo que después permite contar: sin ella, "cuántas están
   * rotas de batería" solo se responde leyendo las descripciones de a una.
   */
  it("un docente puede clasificar la falla, no solo describirla", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))
    await user.type(screen.getByLabelText(/¿Qué le pasa\?/), "No carga")
    await user.type(screen.getByLabelText(/¿Qué es lo que falla\?/), "batería")
    await user.click(screen.getByRole("button", { name: "Reportar" }))

    expect(inventoryApi.reportarIncidencia).toHaveBeenCalledWith(
      expect.objectContaining({ descripcion: "No carga", categoria: "batería" })
    )
  })

  /** Las categorías ya usadas se ofrecen como sugerencia. */
  it("sugiere las categorías que ya se usaron", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    vi.mocked(inventoryApi.listarCategoriasDeFalla).mockResolvedValue({
      data: ["batería", "pantalla"],
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))

    // El datalist no expone sus opciones por rol accesible, así que se lee
    // del DOM: lo que se verifica es que las sugerencias sean las ya usadas.
    await vi.waitFor(() => {
      const opciones = document.querySelectorAll("#categorias-de-falla option")
      expect([...opciones].map((o) => o.getAttribute("value"))).toEqual([
        "batería",
        "pantalla",
      ])
    })
  })

  /** Sin diagnóstico también se puede reportar. */
  it("deja reportar sin clasificar la falla", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo({})] })
    vi.mocked(inventoryApi.reportarIncidencia).mockResolvedValue({
      id: "i1",
      equipoId: "pc1",
      descripcion: "No enciende",
      gravedad: "MODERADA",
      fecha: "2026-08-03T10:00:00Z",
      enviadoASoporte: false,
      estado: "ABIERTA",
    })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(await screen.findByRole("button", { name: "Reportar problema" }))
    await user.type(screen.getByLabelText(/¿Qué le pasa\?/), "No enciende")

    expect(screen.getByLabelText(/¿Qué es lo que falla\?/)).toHaveValue("")
    await user.click(screen.getByRole("button", { name: "Reportar" }))

    expect(await screen.findByText(/quedó registrada/)).toBeInTheDocument()
    expect(inventoryApi.reportarIncidencia).toHaveBeenCalledWith(
      expect.objectContaining({ categoria: undefined })
    )
  })

  // ── RF-03.15: lo prestable que no está en ningún carro ──────────────
  // Faltaba de esta pantalla: el proyector se podía reservar pero no se veía
  // acá, así que un docente no tenía desde dónde mirar su calendario ni
  // avisar que no anda.
  describe("otros equipos", () => {
    const proyector = equipo({
      id: "eq-proyector",
      carroId: undefined,
      identificador: undefined,
      numeroSerie: undefined,
      etiqueta: "Proyector",
      tipo: "Proyector",
      reservable: true,
    })

    it("los lista con su calendario y con reportar una falla", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [proyector] })
      renderInventario()

      expect(await screen.findByText("Otros equipos")).toBeInTheDocument()
      expect(screen.getByText("Proyector")).toBeInTheDocument()
      expect(screen.getByRole("link", { name: "Ver calendario" })).toHaveAttribute(
        "href",
        "/inventario/equipos/eq-proyector/calendario"
      )
      expect(
        screen.getByRole("button", { name: "Reportar problema" })
      ).toBeInTheDocument()
    })

    // Un cargador no se reserva (RF-03.16), así que su calendario está
    // siempre vacío. Avisar que no anda sí tiene sentido.
    it("lo que no se reserva no ofrece calendario, pero sí reportar una falla", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
        data: [
          equipo({
            ...proyector,
            id: "eq-cargador",
            etiqueta: "Cargador",
            tipo: "Cargador",
            reservable: false,
          }),
        ],
      })
      renderInventario()

      expect(await screen.findByText("Cargador")).toBeInTheDocument()
      expect(
        screen.queryByRole("link", { name: "Ver calendario" })
      ).not.toBeInTheDocument()
      expect(
        screen.getByRole("button", { name: "Reportar problema" })
      ).toBeInTheDocument()
    })

    // Dos columnas de guiones no le dicen nada a nadie: un proyector no está
    // freezado ni tiene software instalado.
    it("no muestra las columnas de una computadora si no hay ninguna", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [proyector] })
      renderInventario()

      expect(await screen.findByText("Otros equipos")).toBeInTheDocument()
      expect(
        screen.queryByRole("columnheader", { name: "Freezada" })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole("columnheader", { name: "Software instalado" })
      ).not.toBeInTheDocument()
    })

    // Cada columna se decide por su dato: una notebook suelta es tipo PC —así
    // que "Freezada" dice algo— pero el alta de un equipo suelto todavía no
    // acepta el software, y una columna vacía no le sirve a nadie.
    it("una notebook suelta muestra Freezada, y el software solo si lo tiene", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
        data: [
          equipo({
            ...proyector,
            id: "eq-notebook",
            etiqueta: "Notebook de dirección",
            tipo: "PC",
            softwareInstalado: "Office",
          }),
        ],
      })
      renderInventario()

      expect(await screen.findByText("Notebook de dirección")).toBeInTheDocument()
      expect(screen.getByRole("columnheader", { name: "Freezada" })).toBeInTheDocument()
      expect(
        screen.getByRole("columnheader", { name: "Software instalado" })
      ).toBeInTheDocument()
    })

    it("una notebook suelta sin software no arrastra la columna vacía", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
        data: [
          equipo({
            ...proyector,
            id: "eq-nb2",
            etiqueta: "Notebook sin cargar",
            tipo: "PC",
            softwareInstalado: undefined,
          }),
        ],
      })
      renderInventario()

      expect(await screen.findByText("Notebook sin cargar")).toBeInTheDocument()
      expect(screen.getByRole("columnheader", { name: "Freezada" })).toBeInTheDocument()
      expect(
        screen.queryByRole("columnheader", { name: "Software instalado" })
      ).not.toBeInTheDocument()
    })

    // Contarle a una escuela que no presta nada suelto que no tiene nada
    // suelto es ruido: la sección entera no aparece.
    it("sin equipos sueltos, la sección no existe", async () => {
      renderInventario()

      expect(await screen.findByText("Carro 1")).toBeInTheDocument()
      expect(screen.queryByText("Otros equipos")).not.toBeInTheDocument()
    })

    it("uno dado de baja no se lista", async () => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
        data: [equipo({ ...proyector, dadoDeBaja: true })],
      })
      renderInventario()

      expect(await screen.findByText("Carro 1")).toBeInTheDocument()
      expect(screen.queryByText("Otros equipos")).not.toBeInTheDocument()
    })
  })

  // RF-03.22: el tipo de equipo es texto libre, así que el sistema no puede
  // saber que un cargador no tiene con qué entrar. Sí sabe si tiene alguna
  // cuenta anotada, y con eso alcanza para no ofrecer un panel vacío.
  describe("cuándo se ofrece «Cómo entrar»", () => {
    it("a un docente, solo en los equipos que tienen alguna cuenta anotada", async () => {
      vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
        data: [
          equipo({ id: "pc1", identificador: 1, tieneCuentas: true }),
          equipo({ id: "pc2", identificador: 2, tieneCuentas: false }),
        ],
      })
      const user = userEvent.setup()
      renderInventario()

      await user.click(await screen.findByRole("button", { name: "Ver equipos" }))

      expect(await screen.findByText("PC 1")).toBeInTheDocument()
      expect(screen.getAllByRole("button", { name: "Cómo entrar" })).toHaveLength(1)
    })

    // Sin esto no habría manera de anotar la primera cuenta: el botón solo
    // aparecería donde ya hay una.
    it("a un Admin, siempre", async () => {
      sesionDe("ADMIN")
      vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
        data: [
          equipo({ id: "pc1", identificador: 1, tieneCuentas: false }),
          equipo({ id: "pc2", identificador: 2, tieneCuentas: false }),
        ],
      })
      const user = userEvent.setup()
      renderInventario()

      await user.click(await screen.findByRole("button", { name: "Ver equipos" }))

      expect(await screen.findByText("PC 1")).toBeInTheDocument()
      expect(screen.getAllByRole("button", { name: "Cómo entrar" })).toHaveLength(2)
    })
  })

  /**
   * En un teléfono la tabla queda cortada a la derecha: "Acciones" se ve como
   * "A" y los botones como "Reportar pro…". Se llega deslizándola de costado,
   * así que no se pierde nada — pero "Reportar problema" es EL motivo por el
   * que un docente entra a esta pantalla, y quedaba fuera de la vista detrás
   * de un gesto que hay que descubrir.
   *
   * Apiladas, las acciones están a la vista. Es lo que ya hace "Mis reservas".
   */
  describe("en una pantalla angosta", () => {
    beforeEach(() => {
      vi.stubGlobal(
        "matchMedia",
        (consulta: string) =>
          ({
            matches: true,
            media: consulta,
            addEventListener: () => {},
            removeEventListener: () => {},
          }) as unknown as MediaQueryList
      )
    })

    afterEach(() => {
      vi.unstubAllGlobals()
    })

    it("apila los equipos en tarjetas, con sus acciones a la vista", async () => {
      sesionDe("DOCENTE")
      vi.mocked(inventoryApi.listarCarros).mockResolvedValue({ data: carros })
      vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
        data: [equipo({ id: "pc1", identificador: 1 })],
      })
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
      renderInventario()

      await userEvent.click(await screen.findByRole("button", { name: "Ver equipos" }))

      // La acción está en el documento sin tener que desplazar nada.
      expect(
        await screen.findByRole("button", { name: "Reportar problema" })
      ).toBeInTheDocument()
      // Y no quedó además la tabla: cada equipo tiene que estar UNA vez.
      expect(screen.queryByRole("table")).not.toBeInTheDocument()
      expect(screen.getAllByText("PC 1")).toHaveLength(1)
    })
  })

  /**
   * Un carro tiene hasta 31 máquinas. Abrir "Cómo entrar" desde la primera
   * dejaba el panel al final de la tabla, treinta filas más abajo y fuera de
   * la pantalla: desde el mostrador parecía que el botón no había hecho nada.
   *
   * El test mira la POSICIÓN, no la existencia: que el panel esté en el
   * documento ya lo cubren los de arriba, y eso pasaba también con el bug.
   */
  it("abre el panel pegado al equipo del que salió, no al final de la lista", async () => {
    sesionDe("DOCENTE")
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({ data: carros })
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [
        equipo({ id: "pc1", identificador: 1, tieneCuentas: true }),
        equipo({ id: "pc2", identificador: 2 }),
        equipo({ id: "pc3", identificador: 3 }),
      ],
    })
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarCuentasDeEquipo).mockResolvedValue({ data: [] })
    const user = userEvent.setup()
    renderInventario()

    await user.click(await screen.findByRole("button", { name: "Ver equipos" }))
    await user.click(screen.getByRole("button", { name: "Cómo entrar" }))

    const panel = await screen.findByText(/Cómo entrar a PC 1/)
    const filas = screen.getAllByRole("row")
    const filaDePC1 = filas.findIndex((f) => f.textContent?.startsWith("PC 1"))
    const filaDelPanel = filas.findIndex((f) => f.contains(panel))

    expect(filaDelPanel).toBe(filaDePC1 + 1)
    // Y no al final: quedan PC 2 y PC 3 debajo.
    expect(filaDelPanel).toBeLessThan(filas.length - 1)
  })
})
