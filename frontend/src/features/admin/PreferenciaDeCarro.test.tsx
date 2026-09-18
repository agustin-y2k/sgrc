import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as adminApi from "@/features/admin/api"
import { PreferenciaDeCarro } from "@/features/admin/PreferenciaDeCarro"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo, PreferenciaDeEquipo } from "@/features/inventory/types"

vi.mock("@/features/admin/api")
vi.mock("@/features/academico/api")
vi.mock("@/features/inventory/api")

function equipo(id: string, identificador: number, over: Partial<Equipo> = {}): Equipo {
  return {
    id,
    identificador,
    etiqueta: `PC ${identificador}`,
    tipo: "Notebook",
    carroId: "carro1",
    reservable: true,
    esComputadora: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-03-01",
    ...over,
  }
}

function montar() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <PreferenciaDeCarro carroId="carro1" onListo={() => {}} />
    </QueryClientProvider>,
  )
}

function renderPanel(
  equipos = [equipo("pc1", 1), equipo("pc2", 2)],
  marcas: PreferenciaDeEquipo[] = [],
  materias = ["Dibujo Técnico"],
) {
  vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: equipos })
  vi.mocked(adminApi.materiasEnUso).mockResolvedValue({ data: materias })
  vi.mocked(adminApi.listarPreferenciasDeCarro).mockResolvedValue({ data: marcas })
  return montar()
}

beforeEach(() => {
  vi.clearAllMocks()
})

test("marca la misma preferencia en todos los equipos del carro", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  await waitFor(() =>
    expect(adminApi.marcarPreferencia).toHaveBeenCalledWith(
      expect.objectContaining({
        equipoIds: ["pc1", "pc2"],
        materiaNombre: "Dibujo Técnico",
        prioridad: 1,
      }),
    ),
  )
})

test("se puede destildar un equipo puntual antes de marcar", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.click(screen.getByRole("checkbox", { name: /PC 1/ }))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  await waitFor(() =>
    expect(adminApi.marcarPreferencia).toHaveBeenCalledWith(
      expect.objectContaining({ equipoIds: ["pc2"] }),
    ),
  )
})

test("no ofrece los equipos que no se reservan", async () => {
  renderPanel([equipo("pc1", 1), equipo("pc3", 3, { reservable: false })])

  await screen.findByText("PC 1")
  expect(screen.queryByText("PC 3")).not.toBeInTheDocument()
})

test("avisa si la prioridad elegida ya la usa otra materia en esos equipos", async () => {
  const usuario = userEvent.setup()
  renderPanel(undefined, [
    {
      id: "p1",
      equipoId: "pc1",
      materiaNombre: "Matemática",
      prioridad: 1,
      alcance: "Matemática",
    },
  ])

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")

  expect(
    await screen.findByText(/ya (son|es) preferentes? de Matemática con prioridad 1/),
  ).toBeInTheDocument()
})

test("no avisa por un empate en un equipo que está destildado", async () => {
  const usuario = userEvent.setup()
  renderPanel(undefined, [
    {
      id: "p1",
      equipoId: "pc1",
      materiaNombre: "Matemática",
      prioridad: 1,
      alcance: "Matemática",
    },
  ])

  await screen.findByText("PC 1")
  await usuario.click(screen.getByRole("checkbox", { name: /PC 1/ }))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")

  await waitFor(() =>
    expect(screen.queryByText(/con prioridad 1/)).not.toBeInTheDocument(),
  )
})

test("cuando el carro no tiene equipos reservables, no se muestra el formulario", async () => {
  renderPanel([])

  expect(
    await screen.findByText("Este carro no tiene equipos reservables."),
  ).toBeInTheDocument()
  expect(screen.queryByRole("button", { name: /marcar/i })).not.toBeInTheDocument()
  expect(screen.queryByLabelText("Materia")).not.toBeInTheDocument()
})

// Sin los equipos no se sabe si el carro está vacío: decir que no tiene
// ninguno reservable sería inventarle un dato de inventario al Admin.
test("si no se pudieron traer los equipos lo dice, en vez de darlo por vacío", async () => {
  vi.mocked(inventoryApi.listarEquiposDeCarro).mockRejectedValue(new Error("sin red"))
  vi.mocked(adminApi.materiasEnUso).mockResolvedValue({ data: ["Dibujo Técnico"] })
  vi.mocked(adminApi.listarPreferenciasDeCarro).mockResolvedValue({ data: [] })
  montar()

  expect(await screen.findByText(/Ocurrió un error inesperado/)).toBeInTheDocument()
  expect(
    screen.queryByText("Este carro no tiene equipos reservables."),
  ).not.toBeInTheDocument()
})

test("sin materias cargadas explica dónde se crean", async () => {
  renderPanel(undefined, [], [])

  expect(await screen.findByText(/se crean desde Académico/i)).toBeInTheDocument()
  expect(screen.queryByRole("button", { name: /marcar/i })).not.toBeInTheDocument()
})

// El número es lo único que distingue "se marcaron las ocho" de "ya estaban
// marcadas y no pasó nada".
test("informa cuántos equipos quedaron marcados y cuántos ya lo estaban", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [
      {
        id: "p1",
        equipoId: "pc1",
        materiaNombre: "Dibujo Técnico",
        prioridad: 1,
        alcance: "Dibujo Técnico",
      },
      {
        id: "p2",
        equipoId: "pc2",
        materiaNombre: "Dibujo Técnico",
        prioridad: 1,
        alcance: "Dibujo Técnico",
      },
    ],
    equiposQueYaLaTenian: ["pc3", "pc4", "pc5"],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  expect(await screen.findByText(/Se marcaron 2 equipos\./)).toBeInTheDocument()
  expect(
    await screen.findByText(/3 equipos ya tenían esa misma marca\./),
  ).toBeInTheDocument()
})

// "Se marcaron 1 equipos" delata que el texto se armó concatenando.
test("el resultado de un solo equipo se lee en singular", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [
      {
        id: "p1",
        equipoId: "pc1",
        materiaNombre: "Dibujo Técnico",
        prioridad: 1,
        alcance: "Dibujo Técnico",
      },
    ],
    equiposQueYaLaTenian: ["pc2"],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  expect(
    await screen.findByText(/Se marcó 1 equipo\. 1 equipo ya tenía esa misma marca\./),
  ).toBeInTheDocument()
})

// El formulario en blanco tiene que significar lo mismo que la primera vez: si
// la selección de la vuelta anterior sobrevive, marca menos equipos sin decirlo.
test("al marcar se vuelven a tildar los equipos destildados", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.click(screen.getByRole("checkbox", { name: /PC 1/ }))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  await waitFor(() =>
    expect(screen.getByRole("checkbox", { name: /PC 1/ })).toBeChecked(),
  )
})

// Un éxito y un fallo seguidos no pueden quedar los dos en pantalla: se
// contradicen.
test("el resultado anterior se borra al volver a marcar", async () => {
  const usuario = userEvent.setup()
  vi.mocked(adminApi.marcarPreferencia).mockResolvedValue({
    creadas: [],
    equiposQueYaLaTenian: [],
  })
  renderPanel()

  await screen.findByText("PC 1")
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))
  await screen.findByText(/Se marcaron 0 equipos\./)

  vi.mocked(adminApi.marcarPreferencia).mockRejectedValue(new Error("sin red"))
  await usuario.selectOptions(await screen.findByLabelText("Materia"), "Dibujo Técnico")
  await usuario.click(screen.getByRole("button", { name: /marcar/i }))

  expect(await screen.findByText(/Ocurrió un error inesperado/)).toBeInTheDocument()
  expect(screen.queryByText(/Se marcaron 0 equipos\./)).not.toBeInTheDocument()
})

test("el botón que cierra el panel dice Cerrar", async () => {
  renderPanel()

  expect(await screen.findByRole("button", { name: "Cerrar" })).toBeInTheDocument()
})
