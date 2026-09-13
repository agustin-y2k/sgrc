import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { EspaciosDeCiclo } from "@/features/academico/EspaciosDeCiclo"
import * as academicoApi from "@/features/academico/api"
import type { CicloLectivo, Espacio } from "@/features/academico/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/academico/api")

const CICLO: CicloLectivo = { id: "c1", anio: 2026, activo: true, archivado: false }

function espacio(over: Partial<Espacio> = {}): Espacio {
  return {
    id: "e1",
    cicloLectivoId: "c1",
    nombre: "Biblioteca",
    archivado: false,
    ...over,
  }
}


function renderPanel(espacios: Espacio[] = [], ciclo: CicloLectivo = CICLO) {
  vi.mocked(academicoApi.listarEspacios).mockResolvedValue({ data: espacios })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <EspaciosDeCiclo ciclo={ciclo} />
    </QueryClientProvider>
  )
}

describe("EspaciosDeCiclo", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // Es opcional: la mayoría de las escuelas sólo reserva para cursos, y el
  // vacío tiene que decir eso y no parecer un error.
  it("sin ninguno, explica que es opcional", async () => {
    renderPanel([])

    expect(await screen.findByText(/Es opcional/)).toBeInTheDocument()
  })

  it("crea un lugar", async () => {
    const user = userEvent.setup()
    renderPanel([])
    vi.mocked(academicoApi.crearEspacio).mockResolvedValue(espacio())

    await user.type(await screen.findByLabelText("Nuevo lugar"), "Biblioteca")
    await user.click(screen.getByRole("button", { name: "Agregar lugar" }))

    await waitFor(() => {
      expect(academicoApi.crearEspacio).toHaveBeenCalledWith("c1", "Biblioteca")
    })
  })

  it("renombra", async () => {
    const user = userEvent.setup()
    renderPanel([espacio()])
    vi.mocked(academicoApi.editarEspacio).mockResolvedValue(undefined)

    await user.click(await screen.findByRole("button", { name: "Renombrar" }))
    const campo = screen.getByLabelText("Nombre")
    await user.clear(campo)
    await user.type(campo, "Biblioteca central")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    await waitFor(() => {
      expect(academicoApi.editarEspacio).toHaveBeenCalledWith("e1", "Biblioteca central")
    })
  })

  // Mismo criterio que un curso (RF-02.11): lo que tiene clases dadas no se
  // borra. El mensaje del servidor se muestra tal cual.
  it("muestra el rechazo si alguna materia tiene reservas", async () => {
    const user = userEvent.setup()
    renderPanel([espacio()])
    vi.mocked(academicoApi.eliminarEspacio).mockRejectedValue(
      new ApiError(409, "el espacio tiene materias con reservas: no se puede eliminar")
    )

    await user.click(await screen.findByRole("button", { name: "Eliminar" }))
    await user.click(screen.getByRole("button", { name: "Confirmar" }))

    expect(await screen.findByText(/materias con reservas/)).toBeInTheDocument()
  })

  // Con el ciclo archivado se mira, no se toca — igual que los cursos.
  it("con el ciclo archivado no ofrece crear ni editar", async () => {
    renderPanel([espacio()], { ...CICLO, activo: false, archivado: true })

    await screen.findByText("Biblioteca")
    expect(
      screen.queryByRole("button", { name: "Agregar lugar" })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Renombrar" })).not.toBeInTheDocument()
  })

  // Estos lugares no dictan materias: se reserva para el lugar. Que la pantalla
  // no las mencione es la mitad visible de esa decisión — la otra mitad es que
  // el sistema crea por dentro la fila que hace posible reservar, sin que nadie
  // tenga que inventar una «materia Preceptoría».
  it("no habla de materias en ninguna parte", async () => {
    renderPanel([espacio()])

    await screen.findByText("Biblioteca")
    expect(screen.queryByRole("button", { name: "Materias" })).not.toBeInTheDocument()
    expect(screen.getByText(/no tienen materias/)).toBeInTheDocument()
  })
})
