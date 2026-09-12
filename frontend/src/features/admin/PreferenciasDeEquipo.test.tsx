import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as academicoApi from "@/features/academico/api"
import * as adminApi from "@/features/admin/api"
import { PreferenciasDeEquipo } from "@/features/admin/PreferenciasDeEquipo"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/academico/api")

function preferencia(over: Partial<PreferenciaDeEquipo> = {}): PreferenciaDeEquipo {
  return {
    id: "pref1",
    equipoId: "pc1",
    materiaNombre: "Dibujo Técnico",
    prioridad: 1,
    alcance: "Dibujo Técnico",
    ...over,
  }
}

function renderPanel(preferencias: PreferenciaDeEquipo[] = []) {
  vi.mocked(adminApi.listarPreferenciasDeEquipo).mockResolvedValue({ data: preferencias })
  vi.mocked(adminApi.materiasEnUso).mockResolvedValue({
    data: ["Dibujo Técnico", "Matemática"],
  })
  // Los cursos y las modalidades se sugieren a partir de lo que la escuela
  // tiene cargado: son los únicos que existen de verdad.
  vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
    data: [{ id: "ciclo1", anio: 2026, activo: true, archivado: false }],
  })
  vi.mocked(academicoApi.listarCursos).mockResolvedValue({
    data: [
      {
        id: "c1",
        cicloLectivoId: "ciclo1",
        nombre: "3°B",
        anio: 3,
        division: "B",
        archivado: false,
      },
      {
        id: "c2",
        cicloLectivoId: "ciclo1",
        nombre: "4°2",
        anio: 4,
        division: "2",
        modalidad: "Electromecánica",
        archivado: false,
      },
    ],
  })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PreferenciasDeEquipo equipoId="pc1" />
    </QueryClientProvider>
  )
}

/** RF-03.21 — las marcas de preferencia de un equipo. */
describe("PreferenciasDeEquipo", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("dice que sin marcas el equipo sale en el orden de siempre", async () => {
    renderPanel()

    expect(await screen.findByText(/Sin marcas/)).toBeInTheDocument()
  })

  it("muestra el alcance y la prioridad de cada marca", async () => {
    renderPanel([
      preferencia({ id: "p1", alcance: "Dibujo Técnico" }),
      preferencia({
        id: "p2",
        materiaNombre: "Matemática",
        anio: 3,
        division: "B",
        prioridad: 2,
        alcance: "Matemática de 3°B",
      }),
    ])

    // Acotado a la lista de marcas: los mismos nombres son opciones del
    // selector de abajo, así que buscarlos sueltos encuentra dos cosas.
    const marcas = within(await screen.findByRole("list"))
    expect(marcas.getByText(/Dibujo Técnico/)).toBeInTheDocument()
    expect(marcas.getByText(/Matemática de 3°B/)).toBeInTheDocument()
    expect(marcas.getByText("Prioridad 2")).toBeInTheDocument()
  })

  // El texto es lo único que impide que la marca se lea como un permiso.
  it("aclara que la marca no excluye a nadie", async () => {
    renderPanel()

    expect(
      await screen.findByText(/cualquiera lo puede reservar igual/i)
    ).toBeInTheDocument()
  })

  it("marca el equipo para la materia elegida", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Materia"), "Matemática")
    await user.type(screen.getByLabelText("Año"), "3")
    await user.type(screen.getByLabelText("División"), "B")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Matemática",
        modalidad: undefined,
        anio: 3,
        division: "B",
        prioridad: 1,
      })
    })
  })

  /**
   * Los tres ejes son independientes: cada uno se sostiene solo y ninguno
   * depende de otro. "Matemática de Electromecánica" es un alcance válido sin
   * decir de qué año.
   */
  it("deja acotar por un eje sin los otros dos", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Materia"), "Matemática")
    await user.type(screen.getByLabelText("Modalidad"), "Electromecánica")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Matemática",
        modalidad: "Electromecánica",
        anio: undefined,
        division: undefined,
        prioridad: 1,
      })
    })
  })

  /**
   * La división y la modalidad de la marca tienen que estar escritas como las
   * del curso de verdad. Por eso se escriben libres y se sugieren las que ya
   * existen.
   */
  it("sugiere las divisiones y las modalidades que la escuela ya tiene", async () => {
    renderPanel()

    // Las dos listas aparecen recién cuando llegan los cursos del ciclo: hasta
    // entonces el campo es libre y sin sugerencias, que es lo correcto.
    const opcionesDe = (etiqueta: string) => {
      const campo = screen.getByLabelText(etiqueta)
      const lista = document.getElementById(campo.getAttribute("list") ?? "")
      return lista ? [...lista.querySelectorAll("option")].map((o) => o.value) : null
    }

    await waitFor(() => expect(opcionesDe("División")).toEqual(["2", "B"]))
    expect(opcionesDe("Modalidad")).toEqual(["Electromecánica"])
  })

  it("sin año manda el alcance sin acotar", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Dibujo Técnico",
        modalidad: undefined,
        anio: undefined,
        division: undefined,
        prioridad: 1,
      })
    })
  })

  // Quitar la marca no cancela nada, así que no hay confirmación de por medio.
  it("quita una marca sin pedir confirmación", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.borrarPreferencia).mockResolvedValue(undefined)
    renderPanel([preferencia({ id: "p1" })])

    await user.click(await screen.findByRole("button", { name: "Quitar" }))

    await waitFor(() => {
      expect(adminApi.borrarPreferencia).toHaveBeenCalledWith("p1")
    })
  })

  it("sin materias cargadas explica dónde se crean", async () => {
    vi.mocked(adminApi.listarPreferenciasDeEquipo).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.materiasEnUso).mockResolvedValue({ data: [] })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <PreferenciasDeEquipo equipoId="pc1" />
      </QueryClientProvider>
    )

    expect(await screen.findByText(/se crean desde Académico/i)).toBeInTheDocument()
  })

  // El backend acepta corregir el alcance y la prioridad desde siempre; la
  // pantalla sólo sabía quitar y volver a cargar los cuatro campos a mano.
  describe("corregir una marca", () => {
    it("carga la marca en el formulario y guarda el cambio", async () => {
      const user = userEvent.setup()
      vi.mocked(adminApi.editarPreferencia).mockResolvedValue(
        preferencia({ id: "p1", prioridad: 3 })
      )
      renderPanel([
        preferencia({ id: "p1", materiaNombre: "Matemática", anio: 4, division: "2" }),
      ])

      await user.click(await screen.findByRole("button", { name: "Editar" }))

      // Los campos llegan con lo que la marca ya decía: corregir la prioridad
      // no puede obligar a reescribir el alcance.
      expect(screen.getByLabelText("Año")).toHaveValue(4)
      expect(screen.getByLabelText("División")).toHaveValue("2")

      await user.selectOptions(screen.getByLabelText("Prioridad"), "3")
      await user.click(screen.getByRole("button", { name: "Guardar" }))

      await waitFor(() => {
        expect(adminApi.editarPreferencia).toHaveBeenCalledWith("p1", {
          modalidad: undefined,
          anio: 4,
          division: "2",
          prioridad: 3,
        })
      })
    })

    // Apuntar a otra materia es otra marca, no una corrección de ésta: el
    // PATCH del backend ni siquiera acepta el campo.
    it("no deja cambiar la materia mientras se corrige", async () => {
      const user = userEvent.setup()
      renderPanel([preferencia({ id: "p1" })])

      await user.click(await screen.findByRole("button", { name: "Editar" }))

      expect(screen.getByLabelText("Materia")).toBeDisabled()
    })

    it("cancelar deja el formulario como estaba", async () => {
      const user = userEvent.setup()
      renderPanel([preferencia({ id: "p1", anio: 4 })])

      await user.click(await screen.findByRole("button", { name: "Editar" }))
      await user.click(screen.getByRole("button", { name: "Cancelar" }))

      expect(screen.getByRole("button", { name: "Marcar" })).toBeInTheDocument()
      expect(screen.getByLabelText("Año")).toHaveValue(null)
      expect(adminApi.editarPreferencia).not.toHaveBeenCalled()
    })
  })
})
