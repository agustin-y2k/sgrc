import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { EditarCurso } from "./EditarCurso"
import * as api from "./api"
import { ApiError } from "@/lib/api-client"

vi.mock("./api")
const impacto: api.ImpactoCurso = {
  version: "revision-1",
  anterior: { anio: 3, division: "2", modalidad: "Construcción" },
  propuesto: { anio: 3, division: "3", modalidad: "Construcción" },
  puedeConservarExactas: true,
  cambios: [
    {
      id: "marca",
      equipoEtiqueta: "PC 4",
      carroNombre: "Carro 1",
      materiaNombre: "Dibujo",
      prioridad: 1,
      exacta: true,
      antes: true,
      despues: false,
      compartida: true,
      destinoId: "",
      destinoPrioridad: 0,
    },
  ],
}
function mostrar() {
  const guardado = vi.fn().mockResolvedValue(undefined)
  const cancelar = vi.fn()
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}
    >
      <EditarCurso
        curso={{
          id: "curso",
          cicloLectivoId: "ciclo",
          nombre: "3°2",
          anio: 3,
          division: "2",
          modalidad: "Construcción",
          archivado: false,
        }}
        divisiones={[]}
        modalidades={[]}
        onGuardado={guardado}
        onCancelar={cancelar}
      />
    </QueryClientProvider>
  )
  return { guardado, cancelar }
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.previsualizarCurso).mockResolvedValue(impacto)
  vi.mocked(api.editarCurso).mockResolvedValue(undefined)
})
it("muestra el impacto sin guardar y conserva las exactas tras confirmar", async () => {
  const user = userEvent.setup()
  const { guardado } = mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  expect(await screen.findByText(/Revisá las prioridades/)).toBeInTheDocument()
  expect(screen.getByText(/PC 4/)).toBeInTheDocument()
  expect(screen.getByText(/También se usa en otro curso/)).toBeInTheDocument()
  expect(api.editarCurso).not.toHaveBeenCalled()
  await user.click(screen.getByRole("button", { name: "Confirmar cambio de curso" }))
  await waitFor(() => expect(guardado).toHaveBeenCalledOnce())
  expect(api.editarCurso).toHaveBeenCalledWith(
    "curso",
    expect.objectContaining({
      versionImpacto: "revision-1",
      decisionPrioridades: "conservar_exactas",
    })
  )
})
it("permite cancelar sin aplicar cambios", async () => {
  const user = userEvent.setup()
  const { cancelar } = mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await screen.findByText(/Revisá las prioridades/)
  await user.click(screen.getByRole("button", { name: "Cancelar" }))
  expect(cancelar).toHaveBeenCalledOnce()
  expect(api.editarCurso).not.toHaveBeenCalled()
})
it("exige revisar otra vez si cambian los datos del formulario", async () => {
  const user = userEvent.setup()
  mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await screen.findByText(/Revisá las prioridades/)
  await user.selectOptions(screen.getByLabelText("Año del curso"), "4")
  expect(
    screen.queryByRole("button", { name: "Confirmar cambio de curso" })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await waitFor(() => expect(api.previsualizarCurso).toHaveBeenCalledTimes(2))
  expect(api.editarCurso).not.toHaveBeenCalled()
})
it("impide conservar prioridades en conflicto y permite aceptar su alcance", async () => {
  vi.mocked(api.previsualizarCurso).mockResolvedValue({
    ...impacto,
    puedeConservarExactas: false,
  })
  const user = userEvent.setup()
  mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  expect(
    await screen.findByRole("radio", {
      name: "Conservar las prioridades exactas con el curso",
    })
  ).toBeDisabled()
  expect(screen.getByRole("button", { name: "Confirmar cambio de curso" })).toBeDisabled()
  await user.click(screen.getByRole("radio", { name: /Mantener todas las marcas/ }))
  await user.click(screen.getByRole("button", { name: "Confirmar cambio de curso" }))
  expect(api.editarCurso).toHaveBeenCalledWith(
    "curso",
    expect.objectContaining({ decisionPrioridades: "aceptar_alcance" })
  )
})
it("descarta una confirmación obsoleta y vuelve a consultar", async () => {
  vi.mocked(api.editarCurso).mockRejectedValue(
    new ApiError(409, "El curso o sus prioridades cambiaron")
  )
  const user = userEvent.setup()
  mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await user.click(
    await screen.findByRole("button", { name: "Confirmar cambio de curso" })
  )
  expect(
    await screen.findByText("El curso o sus prioridades cambiaron")
  ).toBeInTheDocument()
  expect(
    screen.queryByRole("button", { name: "Confirmar cambio de curso" })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await waitFor(() => expect(api.previsualizarCurso).toHaveBeenCalledTimes(2))
})
it("guarda directamente si la previsualización no afecta prioridades", async () => {
  vi.mocked(api.previsualizarCurso).mockResolvedValue({ ...impacto, cambios: [] })
  const user = userEvent.setup()
  const { guardado } = mostrar()
  await user.click(screen.getByRole("button", { name: "Guardar" }))
  await waitFor(() => expect(guardado).toHaveBeenCalledOnce())
  expect(api.editarCurso).toHaveBeenCalledWith(
    "curso",
    expect.objectContaining({ versionImpacto: "revision-1" })
  )
})
