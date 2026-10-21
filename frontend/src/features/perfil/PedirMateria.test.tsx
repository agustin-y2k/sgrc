import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as academicoApi from "@/features/academico/api"
import * as perfilApi from "@/features/perfil/api"
import { PedirMateria } from "@/features/perfil/PedirMateria"

vi.mock("@/features/academico/api")
vi.mock("@/features/perfil/api")

function renderPedido() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PedirMateria />
    </QueryClientProvider>
  )
  return queryClient
}

describe("PedirMateria: consulta del catálogo", () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
      data: [{ id: "ciclo1", anio: 2026, activo: true, archivado: false }],
    })
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [
        {
          id: "curso1",
          cicloLectivoId: "ciclo1",
          nombre: "3°1",
          anio: 3,
          archivado: false,
        },
      ],
    })
    vi.mocked(academicoApi.listarMaterias).mockResolvedValue({
      data: [
        {
          id: "materia1",
          cursoId: "curso1",
          espacioId: "",
          nombre: "TIC",
          archivado: false,
        },
      ],
    })
  })

  it.each([
    ["el ciclo lectivo", "listarCiclos"],
    ["los cursos", "listarCursos"],
    ["las materias", "listarMaterias"],
  ] as const)(
    "explica el fallo de %s y permite reintentar conservando el motivo",
    async (nombre, consulta) => {
      vi.mocked(academicoApi[consulta]).mockRejectedValueOnce(new Error("sin conexión"))
      const user = userEvent.setup()
      renderPedido()
      expect(await screen.findByRole("alert")).toHaveTextContent(
        `No pudimos consultar ${nombre}.`
      )
      expect(screen.getByLabelText("¿Cuál materia?")).toBeDisabled()
      await user.type(
        screen.getByLabelText("¿Por qué la pedís?"),
        "Me asignaron el curso"
      )
      expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeDisabled()
      await user.click(screen.getByRole("button", { name: "Reintentar" }))
      await screen.findByRole("option", { name: "TIC" })
      await user.selectOptions(screen.getByLabelText("¿Cuál materia?"), "materia1")
      expect(screen.getByLabelText("¿Por qué la pedís?")).toHaveValue(
        "Me asignaron el curso"
      )
      expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeEnabled()
    }
  )

  it("distingue la carga de un catálogo vacío", () => {
    vi.mocked(academicoApi.listarCiclos).mockReturnValue(new Promise(() => {}))
    renderPedido()
    expect(screen.getByRole("status")).toHaveTextContent("Cargando materias")
    expect(screen.getByLabelText("¿Cuál materia?")).toBeDisabled()
    expect(
      screen.queryByText(/No hay un ciclo|Todavía no hay materias/)
    ).not.toBeInTheDocument()
  })

  it("busca sin tildes por nombre y filtra cursos de distintas modalidades", async () => {
    const user = userEvent.setup()
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [
        {
          id: "curso1",
          cicloLectivoId: "ciclo1",
          nombre: "3°1",
          anio: 3,
          archivado: false,
          modalidad: "Construcción",
        },
        {
          id: "curso2",
          cicloLectivoId: "ciclo1",
          nombre: "3°1",
          anio: 3,
          archivado: false,
          modalidad: "Electromecánica",
        },
      ],
    })
    vi.mocked(academicoApi.listarMaterias).mockImplementation(async (id) => ({
      data: [
        {
          id: `materia-${id}`,
          cursoId: id,
          espacioId: "",
          nombre: id === "curso1" ? "Dibujo Técnico" : "Matemática",
          archivado: false,
        },
      ],
    }))
    renderPedido()
    await screen.findByRole("option", { name: "Dibujo Técnico" })
    await user.selectOptions(screen.getByLabelText("Filtrar por curso"), "curso2")
    await user.type(screen.getByLabelText("Buscar materia"), "matematica")
    expect(screen.getByRole("option", { name: "Matemática" })).toBeInTheDocument()
    expect(
      screen.queryByRole("option", { name: "Dibujo Técnico" })
    ).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText("¿Cuál materia?"), "materia-curso2")
    await user.type(screen.getByLabelText("¿Por qué la pedís?"), "Me asignaron el curso")
    expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeEnabled()
    await user.clear(screen.getByLabelText("Buscar materia"))
    await user.type(screen.getByLabelText("Buscar materia"), "no existe")
    expect(screen.getByText(/Ninguna materia coincide/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeDisabled()
    expect(screen.getByLabelText("¿Por qué la pedís?")).toHaveValue(
      "Me asignaron el curso"
    )
  })

  it.each(["ciclo", "cursos", "materias"] as const)(
    "informa la ausencia real de %s después de una respuesta exitosa",
    async (nivel) => {
      if (nivel === "ciclo")
        vi.mocked(academicoApi.listarCiclos).mockResolvedValue({ data: [] })
      if (nivel === "cursos")
        vi.mocked(academicoApi.listarCursos).mockResolvedValue({ data: [] })
      if (nivel === "materias")
        vi.mocked(academicoApi.listarMaterias).mockResolvedValue({ data: [] })
      renderPedido()
      const mensaje =
        nivel === "ciclo"
          ? /No hay un ciclo lectivo activo/
          : nivel === "cursos"
            ? /todavía no tiene cursos/
            : /Todavía no hay materias/
      expect(await screen.findByText(mensaje)).toBeInTheDocument()
      expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeDisabled()
      expect(screen.queryByRole("alert")).not.toBeInTheDocument()
    }
  )

  it("bloquea una selección anterior si falla la actualización y si la materia desaparece", async () => {
    const user = userEvent.setup()
    const qc = renderPedido()
    await screen.findByRole("option", { name: "TIC" })
    await user.selectOptions(screen.getByLabelText("¿Cuál materia?"), "materia1")
    await user.type(screen.getByLabelText("¿Por qué la pedís?"), "Nuevo curso")
    expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeEnabled()
    vi.mocked(academicoApi.listarMaterias).mockRejectedValueOnce(
      new Error("sin conexión")
    )
    await act(async () => {
      await qc.refetchQueries({ queryKey: ["materias-de-todos-los-cursos"] })
    })
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No pudimos consultar las materias"
    )
    expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeDisabled()
    vi.mocked(academicoApi.listarMaterias).mockResolvedValue({ data: [] })
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await screen.findByText(/Todavía no hay materias/)
    expect(screen.getByRole("button", { name: "Mandar el pedido" })).toBeDisabled()
    expect(perfilApi.pedirMateriaExistente).not.toHaveBeenCalled()
  })

  it("permite pedir una materia nueva aunque no se pudo consultar el catálogo", async () => {
    vi.mocked(academicoApi.listarCiclos).mockRejectedValue(new Error("sin conexión"))
    const user = userEvent.setup()
    renderPedido()
    await screen.findByRole("alert")
    await user.click(screen.getByRole("button", { name: "Todavía no está cargada" }))
    await user.type(screen.getByLabelText("¿Cómo se llama la materia?"), "Robótica")
    await user.type(screen.getByLabelText("¿Por qué la pedís?"), "Nuevo taller")
    await user.click(screen.getByRole("button", { name: "Mandar el pedido" }))
    expect(perfilApi.pedirMateriaNueva).toHaveBeenCalledWith(
      "Robótica",
      "",
      "Nuevo taller"
    )
    expect(perfilApi.pedirMateriaExistente).not.toHaveBeenCalled()
  })
})
