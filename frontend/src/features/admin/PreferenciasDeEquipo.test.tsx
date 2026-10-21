import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { PreferenciasDeEquipo } from "@/features/admin/PreferenciasDeEquipo"
import type { PreferenciaDeEquipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")

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
  vi.mocked(adminApi.catalogoDePrioridades).mockResolvedValue({
    data: ["Dibujo Técnico", "Matemática"].flatMap((materiaNombre) => [
      {
        materiaId: `${materiaNombre}-1`,
        materiaNombre,
        contenedorId: "c1",
        contenedorNombre: "3°B",
        tipo: "CURSO" as const,
        anio: 3,
        division: "B",
      },
      {
        materiaId: `${materiaNombre}-2`,
        materiaNombre,
        contenedorId: "c2",
        contenedorNombre: "4°2",
        tipo: "CURSO" as const,
        modalidad: "Electromecánica",
        anio: 4,
        division: "2",
      },
    ]),
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
    const marcas = within(
      await screen.findByRole("list", { name: "Marcas de este equipo" })
    )
    expect(marcas.getByText(/Dibujo Técnico/)).toBeInTheDocument()
    expect(marcas.getByText(/Matemática de 3°B/)).toBeInTheDocument()
    expect(marcas.getByText("Prioridad 2")).toBeInTheDocument()
  })

  // El texto es lo único que impide que la marca se lea como un permiso.
  it("aclara que la marca no excluye a nadie", async () => {
    renderPanel()

    expect(
      await screen.findByText(
        /Si el equipo está libre, cualquier materia puede reservarlo/i
      )
    ).toBeInTheDocument()
  })

  it("explica el orden numérico, el alcance y las protecciones de la reposición", async () => {
    const user = userEvent.setup()
    renderPanel()
    const prioridad = await screen.findByLabelText("Prioridad")
    expect(prioridad).toHaveAccessibleDescription(
      "1 = mayor prioridad; 9 = menor prioridad."
    )
    await user.click(screen.getByText("Cómo se resuelven las prioridades"))
    expect(screen.getByText(/En un empate ninguna desplaza/)).toBeVisible()
    expect(screen.getByText(/se usa la más específica/)).toBeVisible()
    expect(screen.getByText(/reservas que empiezan en una hora o menos/)).toBeVisible()
    expect(screen.getByText(/primero busca libres/)).toBeVisible()
  })

  it("marca el equipo para la materia elegida", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(
      await screen.findByLabelText("Modalidad"),
      "__SIN_ATRIBUTO__"
    )
    await user.selectOptions(screen.getByLabelText("Año"), "3")
    await user.selectOptions(screen.getByLabelText("División"), "B")
    await user.selectOptions(screen.getByLabelText("Materia"), "Matemática")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Matemática",
        modalidad: undefined,
        anio: 3,
        division: "B",
        prioridad: 1,
        cursoExacto: true,
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

    await user.selectOptions(await screen.findByLabelText("Alcance"), "GENERAL")
    await user.selectOptions(screen.getByLabelText("Modalidad"), "Electromecánica")
    await user.selectOptions(screen.getByLabelText("Materia"), "Matemática")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Matemática",
        modalidad: "Electromecánica",
        anio: undefined,
        division: undefined,
        prioridad: 1,
        cursoExacto: false,
      })
    })
  })

  it("requiere elegir modalidad, año y división antes de la materia", async () => {
    const user = userEvent.setup()
    renderPanel()
    expect(await screen.findByLabelText("Año")).toBeDisabled()
    expect(screen.getByLabelText("División")).toBeDisabled()
    expect(screen.getByLabelText("Materia")).toBeDisabled()
    expect(screen.getByRole("button", { name: "Marcar" })).toBeDisabled()
    await user.selectOptions(screen.getByLabelText("Modalidad"), "Electromecánica")
    expect(
      within(screen.getByLabelText("Año"))
        .getAllByRole("option")
        .map((o) => o.getAttribute("value"))
    ).toEqual(["", "4"])
    await user.selectOptions(screen.getByLabelText("Año"), "4")
    await user.selectOptions(screen.getByLabelText("División"), "2")
    await user.selectOptions(screen.getByLabelText("Materia"), "Matemática")
    expect(screen.getByRole("button", { name: "Marcar" })).toBeEnabled()
    await user.selectOptions(screen.getByLabelText("Modalidad"), "__SIN_ATRIBUTO__")
    expect(screen.getByLabelText("Año")).toHaveValue("")
    expect(screen.getByLabelText("División")).toHaveValue("")
    expect(screen.getByLabelText("Materia")).toHaveValue("")
    expect(screen.getByRole("button", { name: "Marcar" })).toBeDisabled()
    expect(adminApi.marcarPreferencia).not.toHaveBeenCalled()
  })

  it("sin año manda el alcance sin acotar", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({ creadas: [] })
    renderPanel()

    await user.selectOptions(await screen.findByLabelText("Alcance"), "GENERAL")
    await user.selectOptions(screen.getByLabelText("Materia"), "Dibujo Técnico")
    await user.click(screen.getByRole("button", { name: "Marcar" }))

    await waitFor(() => {
      expect(adminApi.marcarPreferencia).toHaveBeenCalledWith({
        equipoIds: ["pc1"],
        materiaNombre: "Dibujo Técnico",
        modalidad: undefined,
        anio: undefined,
        division: undefined,
        prioridad: 1,
        cursoExacto: false,
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

  test("avisa cuando la prioridad elegida ya la usa otra materia", async () => {
    const usuario = userEvent.setup()
    renderPanel([preferencia({ materiaNombre: "Matemática", prioridad: 1 })])

    await screen.findByRole("list", { name: "Marcas de este equipo" })
    await usuario.selectOptions(await screen.findByLabelText("Alcance"), "GENERAL")
    await usuario.selectOptions(screen.getByLabelText("Materia"), "Dibujo Técnico")

    expect(
      await screen.findByText(/ya es preferente de Matemática con prioridad 1/)
    ).toBeInTheDocument()
  })

  it("sin materias cargadas explica dónde se crean", async () => {
    vi.mocked(adminApi.listarPreferenciasDeEquipo).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.catalogoDePrioridades).mockResolvedValue({ data: [] })
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
      expect(screen.getByLabelText("Año")).toHaveValue("4")
      expect(screen.getByLabelText("División")).toHaveValue("2")

      await user.selectOptions(screen.getByLabelText("Prioridad"), "3")
      await user.click(screen.getByRole("button", { name: "Guardar" }))

      await waitFor(() => {
        expect(adminApi.editarPreferencia).toHaveBeenCalledWith("p1", {
          modalidad: undefined,
          anio: 4,
          division: "2",
          prioridad: 3,
          cursoExacto: false,
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

    // Subirle la prioridad a una marca hasta la de otra es la forma natural de
    // "hacer que ésta mande", y deja el empate exacto del que hay que avisar.
    it("avisa si al corregir la prioridad queda empatada con otra materia", async () => {
      const user = userEvent.setup()
      renderPanel([
        preferencia({ id: "p1", materiaNombre: "Dibujo Técnico", prioridad: 1 }),
        preferencia({
          id: "p2",
          materiaNombre: "Matemática",
          prioridad: 2,
          alcance: "Matemática",
        }),
      ])

      const marcas = within(
        await screen.findByRole("list", { name: "Marcas de este equipo" })
      )
      const deMatematica = marcas.getByText(/Matemática/).closest("li")!
      await user.click(within(deMatematica).getByRole("button", { name: "Editar" }))
      await user.selectOptions(screen.getByLabelText("Prioridad"), "1")

      expect(
        await screen.findByText(/ya es preferente de Dibujo Técnico con prioridad 1/)
      ).toBeInTheDocument()
    })

    it("cancelar deja el formulario como estaba", async () => {
      const user = userEvent.setup()
      renderPanel([preferencia({ id: "p1", anio: 4 })])

      await user.click(await screen.findByRole("button", { name: "Editar" }))
      await user.click(screen.getByRole("button", { name: "Cancelar" }))

      expect(screen.getByRole("button", { name: "Marcar" })).toBeInTheDocument()
      expect(screen.getByLabelText("Año")).toHaveValue("")
      expect(adminApi.editarPreferencia).not.toHaveBeenCalled()
    })
  })
})
