import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as inventoryApi from "@/features/inventory/api"
import type { Carro, Equipo } from "@/features/inventory/types"
import { BloquearEquiposPage } from "@/features/reservas/BloquearEquiposPage"
import * as reservasApi from "@/features/reservas/api"
import type { EquipoDisponible } from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"
import { fechaFuturaEnDias } from "@/test/fechas"

vi.mock("@/features/inventory/api")
vi.mock("@/features/reservas/api")

// Relativa a hoy: el input tiene min=hoy porque el backend rechaza bloquear
// un horario que ya pasó (ver src/test/fechas.ts).
const FECHA = fechaFuturaEnDias(7)

const CARRO: Carro = { id: "carro1", nombre: "Carro A" }

function equipo(over: Partial<Equipo> = {}): Equipo {
  return {
    id: "pc1",
    carroId: "carro1",
    identificador: 1,
    numeroSerie: "SERIE-1001",
    etiqueta: `PC ${over.identificador ?? 1}`,
    tipo: "PC",
    reservable: true,
    esComputadora: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01",
    ...over,
  }
}

function disponible(equipoId: string, identificador: number): EquipoDisponible {
  return {
    equipoId,
    identificador,
    etiqueta: `PC ${identificador}`,
    carroId: "carro1",
    carroNombre: "Carro A",
    freezado: false,
    tramo: "NEUTRAL",
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <BloquearEquiposPage />
    </QueryClientProvider>
  )
}

/** Completa fecha, horario y motivo, que es lo que habilita el botón. */
async function completarFranja(
  user: ReturnType<typeof userEvent.setup>,
  motivo = "Aprender 2026"
) {
  await user.type(await screen.findByLabelText("Fecha"), FECHA)
  await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
  await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
  await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "10")
  await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")
  await user.type(screen.getByLabelText("¿Por qué se bloquean?"), motivo)
}

describe("BloquearEquiposPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({ data: [CARRO] })
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [equipo(), equipo({ id: "pc2", identificador: 2 })],
    })
    // Las dos libres: sin reservas que cancelar, salvo que un test diga otra cosa.
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: [disponible("pc1", 1), disponible("pc2", 2)],
    })
    vi.mocked(reservasApi.bloquearEquipos).mockResolvedValue({
      bloqueos: [],
      reservasCanceladas: 0,
      docentesNotificados: 0,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("lista los equipos del inventario agrupadas por carro", async () => {
    renderPagina()

    expect(await screen.findByText("Carro A")).toBeInTheDocument()
    expect(screen.getByLabelText("PC 1")).toBeInTheDocument()
    expect(screen.getByLabelText("PC 2")).toBeInTheDocument()
  })

  /**
   * El backend rechaza el bloqueo ENTERO si viene un equipo que no está
   * DISPONIBLE (ErrEquipoNoDisponible): no la saltea.
   */
  it("no deja elegir un equipo que no está disponible", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [
        equipo(),
        equipo({ id: "pc2", identificador: 2, estado: "FUERA_DE_SERVICIO" }),
      ],
    })
    renderPagina()

    expect(await screen.findByLabelText("PC 2")).toBeDisabled()
    expect(screen.getByLabelText("PC 1")).toBeEnabled()
    expect(screen.getByText("Fuera de servicio")).toBeInTheDocument()
  })

  it("no muestra los equipos dados de baja", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [equipo(), equipo({ id: "pc2", identificador: 2, dadoDeBaja: true })],
    })
    renderPagina()

    await screen.findByLabelText("PC 1")
    expect(screen.queryByLabelText("PC 2")).not.toBeInTheDocument()
  })

  it("no consulta la ocupación hasta tener la franja completa", async () => {
    renderPagina()

    await screen.findByLabelText("PC 1")
    expect(reservasApi.equiposDisponibles).not.toHaveBeenCalled()
    expect(
      screen.getByText(/Completá la fecha y el horario para ver cuáles ya tienen reserva/)
    ).toBeInTheDocument()
  })

  /**
   * Es el punto de la pantalla: el endpoint no simula nada, así que la única
   * forma de saber qué se va a cancelar es cruzar el inventario contra los
   * equipos libres en esa franja.
   */
  it("marca los equipos que ya tienen reserva en la franja elegida", async () => {
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: [disponible("pc1", 1)],
    })
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)

    expect(await screen.findByText("Con reserva en esa franja")).toBeInTheDocument()
  })

  it("avisa cuántas reservas se van a cancelar antes de confirmar", async () => {
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: [disponible("pc1", 1)],
    })
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 2"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))

    expect(
      await screen.findByText(/1 de esos equipos tienen una reserva en esa franja/)
    ).toBeInTheDocument()
    expect(screen.getByText(/no se recuperan/)).toBeInTheDocument()
  })

  it("dice explícitamente cuando no hay ninguna reserva en juego", async () => {
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 1"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))

    expect(
      await screen.findByText(/Ninguna de los equipos elegidas tiene reservas/)
    ).toBeInTheDocument()
  })

  // No se manda nada hasta el segundo botón: la cascada no se deshace.
  it("no bloquea nada con solo apretar 'revisar'", async () => {
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 1"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))

    await screen.findByRole("button", { name: "Confirmar bloqueo" })
    expect(reservasApi.bloquearEquipos).not.toHaveBeenCalled()
  })

  it("bloquea los equipos elegidas al confirmar", async () => {
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 1"))
    await user.click(screen.getByLabelText("PC 2"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))
    await user.click(screen.getByRole("button", { name: "Confirmar bloqueo" }))

    await waitFor(() => {
      expect(reservasApi.bloquearEquipos).toHaveBeenCalledWith({
        equipoIds: ["pc1", "pc2"],
        fecha: FECHA,
        horaInicio: "08:00",
        horaFin: "10:00",
        motivo: "Aprender 2026",
      })
    })
  })

  /**
   * El motivo se intercala tal cual en el aviso a cada docente, así que un
   * bloqueo sin motivo le cancela la clase a alguien sin decirle por qué.
   */
  it("exige un motivo", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Fecha"), FECHA)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "10")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")
    await user.click(screen.getByLabelText("PC 1"))

    expect(screen.getByRole("button", { name: "Revisar bloqueo" })).toBeDisabled()

    await user.type(screen.getByLabelText("¿Por qué se bloquean?"), "Aprender 2026")
    expect(screen.getByRole("button", { name: "Revisar bloqueo" })).toBeEnabled()
  })

  it("no deja bloquear sin elegir ningún equipo", async () => {
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)

    expect(screen.getByRole("button", { name: "Revisar bloqueo" })).toBeDisabled()
  })

  it("avisa si la hora de fin es igual a la de inicio", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.selectOptions(await screen.findByLabelText("Hora de inicio: hora"), "10")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "10")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    expect(
      screen.getByText("La hora de fin no puede ser igual a la de inicio.")
    ).toBeInTheDocument()
  })

  it("informa cuántas reservas se cancelaron y a cuántos docentes se avisó", async () => {
    vi.mocked(reservasApi.bloquearEquipos).mockResolvedValue({
      bloqueos: [],
      reservasCanceladas: 3,
      docentesNotificados: 2,
    })
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 1"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))
    await user.click(screen.getByRole("button", { name: "Confirmar bloqueo" }))

    expect(
      await screen.findByText(/Se cancelaron 3 reservas y se notificó a 2 docentes/)
    ).toBeInTheDocument()
  })

  it("muestra el error del backend", async () => {
    vi.mocked(reservasApi.bloquearEquipos).mockRejectedValue(
      new ApiError(409, "el equipo no está disponible para reservar")
    )
    const user = userEvent.setup()
    renderPagina()

    await completarFranja(user)
    await user.click(await screen.findByLabelText("PC 1"))
    await user.click(screen.getByRole("button", { name: "Revisar bloqueo" }))
    await user.click(screen.getByRole("button", { name: "Confirmar bloqueo" }))

    expect(
      await screen.findByText("el equipo no está disponible para reservar")
    ).toBeInTheDocument()
  })

  /**
   * El motivo no es solo el texto del aviso de cancelación: queda guardado en
   * el bloqueo.
   */
  it("manda el motivo tal como se escribió, sin categoría fija", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user, "Jornada docente")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 1/ }))
    await user.click(screen.getByRole("button", { name: /Revisar/ }))
    await user.click(await screen.findByRole("button", { name: /Confirmar/ }))

    expect(reservasApi.bloquearEquipos).toHaveBeenCalledWith(
      expect.objectContaining({ motivo: "Jornada docente" })
    )
  })

  // La pantalla no puede sugerir que el bloqueo es siempre lo mismo: el
  // sistema no sabe de qué se trata, así que nombra varios casos como ejemplo
  // y ninguno como categoría.
  it("presenta el motivo como texto libre, sin una categoría fija", async () => {
    renderPagina()
    expect(await screen.findByText("Bloquear equipos")).toBeInTheDocument()
    expect(screen.getByText(/una jornada/i)).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/jornada docente/i)).toBeInTheDocument()
  })

  /**
   * Tomar un carro entero para una evaluación es el caso para el que existe
   * esta pantalla, y con tres carros cargados son más de ochenta casillas.
   * Antes había que tildarlas de a una.
   */
  describe("cuando el inventario es grande", () => {
    beforeEach(() => {
      vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
        data: [
          ...Array.from({ length: 9 }, (_, i) =>
            equipo({ id: `pc${i + 1}`, identificador: i + 1 })
          ),
          // Una rota: no se puede tildar de a una, así que tampoco puede
          // entrar por "marcar el carro" — el backend rechaza el bloqueo
          // ENTERO si viene una sola así.
          equipo({ id: "pc10", identificador: 10, estado: "FUERA_DE_SERVICIO" }),
        ],
      })
    })

    it("marca el carro entero salteando las que no se pueden bloquear", async () => {
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Marcar 9 equipos" }))

      expect(await screen.findByText("9 equipos seleccionados.")).toBeInTheDocument()
      expect(screen.getByLabelText("PC 10 (Carro A)")).not.toBeChecked()
    })

    it("filtra con el buscador sin que importen las tildes", async () => {
      const user = userEvent.setup()
      renderPagina()

      await user.type(await screen.findByLabelText("Buscar un equipo"), "carro a")
      expect(screen.getByLabelText("PC 1 (Carro A)")).toBeInTheDocument()

      await user.clear(screen.getByLabelText("Buscar un equipo"))
      await user.type(screen.getByLabelText("Buscar un equipo"), "PC 3")
      expect(screen.getByLabelText("PC 3 (Carro A)")).toBeInTheDocument()
      expect(screen.queryByLabelText("PC 1 (Carro A)")).not.toBeInTheDocument()
    })

    it("limpia la selección de una", async () => {
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Marcar 9 equipos" }))
      await user.click(screen.getByRole("button", { name: "Limpiar la selección" }))

      expect(screen.queryByText(/seleccionados\./)).not.toBeInTheDocument()
    })
  })
})
