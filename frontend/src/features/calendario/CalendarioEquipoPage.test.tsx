import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { CalendarioEquipoPage } from "@/features/calendario/CalendarioEquipoPage"
import * as calendarioApi from "@/features/calendario/api"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import type { CalendarioEquipo } from "@/features/calendario/types"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/calendario/api")

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query y la pantalla
  // la necesita de verdad.
  ...(await original<typeof disponibilidadApi>()),
  jornadaDeLaInstitucion: vi.fn(),
}))

/** Declara la jornada como abierta esos días, de 07:00 a 18:00. */
function jornadaAbiertaLos(dias: DiaSemana[]) {
  const data: BloqueHorario[] = dias.map((diaSemana) => ({
    id: `j-${diaSemana}`,
    diaSemana,
    horaInicio: "07:00",
    horaFin: "18:00",
  }))
  vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({
    data,
  })
}

// La semana del 9 al 15 de marzo de 2026 (lunes a domingo).
const LUNES = new Date(2026, 2, 9, 10, 0, 0)

function renderCalendario() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/inventario/equipos/pc1/calendario"]}>
        <Routes>
          <Route
            path="/inventario/equipos/:equipoId/calendario"
            element={<CalendarioEquipoPage />}
          />
          <Route path="/inventario" element={<div>Inventario</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const calendarioMock: CalendarioEquipo = {
  equipoId: "pc1",
  desde: "2026-03-09",
  hasta: "2026-03-15",
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
      tipo: "BLOQUEO",
      docente: "",
      motivoBloqueo: "Jornada docente",
    },
  ],
}

describe("CalendarioEquipoPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(LUNES)
    // Sin jornada declarada no hay restricción, que es como arranca una
    // instalación nueva: los siete días se dibujan.
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({
      data: [],
    })
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

  // Un bloqueo administrativo no tiene materia ni docente: se
  // muestra distinto en vez de dejar campos vacíos.
  it("un bloqueo se rotula con su motivo, no con una categoría fija", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    renderCalendario()

    // Lo que trae a alguien al calendario es "¿por qué no puedo reservar
    // acá?".
    expect(await screen.findByText("Jornada docente")).toBeInTheDocument()
  })

  it("pide el calendario de la semana en curso, de lunes a domingo", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    renderCalendario()

    await screen.findByText("Matemáticas")
    expect(calendarioApi.calendarioDeEquipo).toHaveBeenCalledWith(
      "pc1",
      "2026-03-09",
      "2026-03-15"
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
      "2026-03-22"
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
      "2026-03-08"
    )
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockRejectedValue(
      new ApiError(400, "desde: la fecha debe tener formato YYYY-MM-DD")
    )
    renderCalendario()

    expect(await screen.findByText(/formato YYYY-MM-DD/)).toBeInTheDocument()
  })

  // Con un error, la grilla no se dibuja. Antes se dibujaba igual, así que un
  // equipo que no existe mostraba el cartel Y una semana entera vacía debajo:
  // la pantalla se contradecía a sí misma.
  it("con un error no dibuja la grilla vacía debajo del cartel", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockRejectedValue(
      new ApiError(404, "ese equipo no está en el inventario")
    )
    renderCalendario()

    expect(await screen.findByText(/no está en el inventario/)).toBeInTheDocument()
    // La leyenda es lo último de la grilla: si no está, no se dibujó nada.
    expect(
      screen.queryByText(/Los huecos en blanco están libres/)
    ).not.toBeInTheDocument()
    expect(screen.queryByText("Reserva de clase")).not.toBeInTheDocument()
  })

  // Si la escuela achica su jornada, lo que ya estaba reservado un día que
  // dejó de estar declarado sigue existiendo y sigue ocupando la máquina.
  // Antes la columna no se dibujaba y esa reserva quedaba invisible: la PC
  // figuraba libre en la única pantalla que existe para verla.
  it("dibuja un día que dejó de estar declarado si tiene algo reservado", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    // El jueves 12 tiene un bloqueo cargado, pero la escuela ahora solo abre
    // de lunes a miércoles.
    jornadaAbiertaLos(["LUNES", "MARTES", "MIERCOLES"])
    renderCalendario()

    expect(await screen.findByText("Jueves")).toBeInTheDocument()
    expect(screen.getByText("Jornada docente")).toBeInTheDocument()
    expect(screen.getByText("fuera de la jornada")).toBeInTheDocument()
  })

  // La otra mitad: un día cerrado y sin nada adentro sí se oculta, que es
  // para lo que una escuela declara su jornada.
  it("oculta los días cerrados que no tienen nada reservado", async () => {
    vi.mocked(calendarioApi.calendarioDeEquipo).mockResolvedValue(calendarioMock)
    jornadaAbiertaLos(["LUNES", "MARTES", "MIERCOLES"])
    renderCalendario()

    await screen.findByText("Jueves")
    // Viernes, sábado y domingo están cerrados y vacíos.
    expect(screen.queryByText("Viernes")).not.toBeInTheDocument()
    expect(screen.queryByText("Sábado")).not.toBeInTheDocument()
    expect(screen.queryByText("Domingo")).not.toBeInTheDocument()
    // Y el martes, que está declarado, no lleva el rótulo.
    expect(screen.getByText("Martes")).toBeInTheDocument()
    expect(screen.getAllByText("fuera de la jornada")).toHaveLength(1)
  })
})
