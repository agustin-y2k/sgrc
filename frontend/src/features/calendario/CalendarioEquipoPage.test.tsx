import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { CalendarioEquipoPage } from "@/features/calendario/CalendarioEquipoPage"
import * as calendarioApi from "@/features/calendario/api"
import type { CalendarioEquipo } from "@/features/calendario/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/calendario/api")

// La semana del 9 al 14 de marzo de 2026 (lunes a sábado).
const LUNES = new Date(2026, 2, 9, 10, 0, 0)

function renderCalendario() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/inventario/equipos/pc1/calendario"]}>
        <Routes>
          <Route path="/inventario/equipos/:equipoId/calendario" element={<CalendarioEquipoPage />} />
          <Route path="/inventario" element={<div>Inventario</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const calendarioMock: CalendarioEquipo = {
  equipoId: "pc1",
  desde: "2026-03-09",
  hasta: "2026-03-14",
  bloques: [
    {
      reservaId: "r1",
      fecha: "2026-03-10",
      horaInicio: "08:00",
      horaFin: "09:30",
      estado: "CONFIRMADA",
      tipo: "NORMAL",
      docente: "Ada Lovelace",
      materiaNombre: "Matemáticas",
      cursoNombre: "1°A",
    },
    {
      reservaId: "r2",
      fecha: "2026-03-12",
      horaInicio: "14:00",
      horaFin: "16:00",
      estado: "CONFIRMADA",
      tipo: "EVALUACION_ESTATAL",
      docente: "",
    },
  ],
}

describe("CalendarioEquipoPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(LUNES)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  // RF-04.4: los bloques muestran docente, materia y horario.
  it("muestra docente, materia, curso y horario de cada bloque", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    renderCalendario()

    expect(await screen.findByText("Matemáticas")).toBeInTheDocument()
    expect(screen.getByText("1°A")).toBeInTheDocument()
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument()
    expect(screen.getByText("08:00–09:30")).toBeInTheDocument()
  })

  // Un bloqueo por evaluación estatal no tiene materia ni docente: se
  // muestra distinto en vez de dejar campos vacíos.
  it("distingue el bloqueo por evaluación estatal", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    renderCalendario()

    expect(await screen.findByText("Evaluación estatal")).toBeInTheDocument()
  })

  it("pide el calendario de la semana en curso, de lunes a sábado", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    renderCalendario()

    await screen.findByText("Matemáticas")
    expect(calendarioApi.calendarioDeEquipo).toHaveBeenCalledWith(
      "pc1",
      "2026-03-09",
      "2026-03-14"
    )
  })

  it("navegar a la semana siguiente vuelve a consultar el rango corrido", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    const user = userEvent.setup()
    renderCalendario()
    await screen.findByText("Matemáticas")

    await user.click(screen.getByRole("button", { name: /Semana siguiente/ }))

    expect(calendarioApi.calendarioDeEquipo).toHaveBeenLastCalledWith(
      "pc1",
      "2026-03-16",
      "2026-03-21"
    )
  })

  it("navegar a la semana anterior vuelve a consultar el rango corrido", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    const user = userEvent.setup()
    renderCalendario()
    await screen.findByText("Matemáticas")

    await user.click(screen.getByRole("button", { name: /Semana anterior/ }))

    expect(calendarioApi.calendarioDeEquipo).toHaveBeenLastCalledWith(
      "pc1",
      "2026-03-02",
      "2026-03-07"
    )
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockRejectedValue(
      new ApiError(400, "desde: la fecha debe tener formato YYYY-MM-DD")
    )
    renderCalendario()

    expect(await screen.findByText(/formato YYYY-MM-DD/)).toBeInTheDocument()
  })
})
