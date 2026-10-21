import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as api from "@/features/academico/api"
import { CursosDeCiclo } from "@/features/academico/CursosDeCiclo"
import { EspaciosDeCiclo } from "@/features/academico/EspaciosDeCiclo"
import { EstructuraDelCiclo } from "@/features/academico/EstructuraDelCiclo"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/academico/api")

const ciclo = { id: "ciclo", anio: 2026, activo: true, archivado: false }
function montar(elemento: React.ReactNode) {
  const cliente = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={cliente}>{elemento}</QueryClientProvider>)
}

describe("errores al consultar la estructura académica", () => {
  beforeEach(() => vi.resetAllMocks())

  it("un fallo al leer cursos muestra el error y no ofrece una estructura ficticia vacía", async () => {
    vi.mocked(api.listarCursos).mockRejectedValue(
      new ApiError(500, "No se pudieron consultar los cursos")
    )
    montar(<CursosDeCiclo ciclo={ciclo} />)
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No se pudieron consultar los cursos"
    )
    expect(screen.queryByRole("button", { name: /Crear curso/ })).not.toBeInTheDocument()
  })

  it("un fallo al leer espacios no se presenta como falta de espacios", async () => {
    vi.mocked(api.listarEspacios).mockRejectedValue(
      new ApiError(500, "No se pudieron consultar los espacios")
    )
    montar(<EspaciosDeCiclo ciclo={ciclo} />)
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No se pudieron consultar los espacios"
    )
    expect(screen.queryByText(/Todavía no hay/)).not.toBeInTheDocument()
  })

  it("bloquea importar y descargar cuando no pudo leer la estructura actual", async () => {
    vi.mocked(api.exportarEstructura).mockRejectedValue(
      new ApiError(500, "Estructura no disponible")
    )
    montar(<EstructuraDelCiclo ciclo={ciclo} />)
    expect(await screen.findByRole("alert")).toHaveTextContent("Estructura no disponible")
    expect(screen.getByLabelText("Cargar desde una planilla")).toBeDisabled()
    expect(api.importarEstructura).not.toHaveBeenCalled()
  })

  it("un archivo ilegible muestra el error sin dejar una promesa rechazada", async () => {
    vi.mocked(api.exportarEstructura).mockResolvedValue({ cursos: [] })
    montar(<EstructuraDelCiclo ciclo={ciclo} />)
    const input = screen.getByLabelText("Cargar desde una planilla")
    await waitFor(() => expect(input).toBeEnabled())
    const archivo = new File(["contenido"], "estructura.csv", { type: "text/csv" })
    Object.defineProperty(archivo, "text", {
      value: () => Promise.reject(new Error("lectura fallida")),
    })
    await userEvent.upload(input, archivo)
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No se pudo leer la planilla"
    )
    expect(api.importarEstructura).not.toHaveBeenCalled()
  })
})
