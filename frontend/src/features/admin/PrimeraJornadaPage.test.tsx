import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as disponibilidadApi from "@/features/disponibilidad/api"
import { PrimeraJornadaPage } from "@/features/admin/PrimeraJornadaPage"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query.
  ...(await original<typeof disponibilidadApi>()),
  reemplazarJornada: vi.fn(),
}))

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <PrimeraJornadaPage />
    </QueryClientProvider>
  )
}

describe("PrimeraJornadaPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
      definida: true,
    })
  })

  // El caso que cubre a casi todas las escuelas, y tiene que costar tres
  // clics: el atajo, agregar, guardar.
  it("declara la semana con el atajo de lunes a viernes", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledWith(
      [
        { diaSemana: "LUNES", horaInicio: "08:00", horaFin: "12:00" },
        { diaSemana: "MARTES", horaInicio: "08:00", horaFin: "12:00" },
        { diaSemana: "MIERCOLES", horaInicio: "08:00", horaFin: "12:00" },
        { diaSemana: "JUEVES", horaInicio: "08:00", horaFin: "12:00" },
        { diaSemana: "VIERNES", horaInicio: "08:00", horaFin: "12:00" },
      ],
      false
    )
  })

  // Turno mañana y turno noche: dos tramos para los mismos días, con el
  // mediodía cerrado.
  it("acumula varios tramos antes de guardar", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    await user.click(screen.getByRole("button", { name: "Lunes" }))
    await user.selectOptions(screen.getByLabelText("Abre: hora"), "18")
    await user.selectOptions(screen.getByLabelText("Cierra: hora"), "23")
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledWith(
        [
          { diaSemana: "LUNES", horaInicio: "08:00", horaFin: "12:00" },
          { diaSemana: "LUNES", horaInicio: "18:00", horaFin: "23:00" },
        ],
        false
      )
    })
  })

  // La salida sin declarar nada: quien está evaluando el sistema no sabe
  // todavía qué horario tiene la escuela, y obligarlo a inventar uno es
  // producir el error que esta pantalla vino a evitar.
  it("dejarla libre guarda una jornada vacía", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Dejarla libre por ahora" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledWith([], false)
    })
  })

  // Guardar una jornada de cero tramos por accidente es lo mismo que dejarla
  // libre, y eso tiene que ser una decisión explícita y no un botón apurado.
  it("no se puede guardar sin haber cargado ningún tramo", () => {
    renderPagina()

    expect(screen.getByRole("button", { name: "Guardar la jornada" })).toBeDisabled()
    // Dejarla libre, en cambio, siempre está disponible.
    expect(screen.getByRole("button", { name: "Dejarla libre por ahora" })).toBeEnabled()
  })

  it("un tramo cargado se puede quitar antes de guardar", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Sábado" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    // Por el aria-label y no por el texto "Sábado": ese texto también lo lleva
    // el botón de día del formulario, que sigue ahí.
    const quitar = screen.getByRole("button", { name: "Quitar Sábado de 08:00 a 12:00" })

    await user.click(quitar)

    expect(screen.getByRole("button", { name: "Guardar la jornada" })).toBeDisabled()
  })

  // El encierro: una instalación que venía funcionando SIN jornada declarada
  // llega acá con meses de reservas, así que cualquier horario que declare
  // deja alguna afuera. Sin poder confirmar, el Admin no sale de esta pantalla
  // —el portón lo devuelve— y su única salida sería rendirse y dejarla libre.
  it("puede confirmar aunque el horario deje reservas afuera", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValueOnce(
      new ApiError(409, "el cambio deja reservas fuera de la jornada", {
        error: "el cambio deja reservas fuera de la jornada",
        impacto: {
          reservas: [
            {
              id: "r1",
              fecha: "2026-03-14",
              horaInicio: "09:00",
              horaFin: "11:00",
              equipo: "PC 3",
              materia: "Matemáticas",
              docente: "Ada Lovelace",
            },
          ],
          prestamos: [],
          totalDeReservas: 40,
        },
      })
    )
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    await user.click(await screen.findByRole("button", { name: /Guardar y cancelar 1/ }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(2)
    })
    expect(vi.mocked(disponibilidadApi.reemplazarJornada).mock.calls[1][1]).toBe(true)
  })

  it("muestra el error del backend", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValue(
      new ApiError(409, "ese bloque se superpone con otro del mismo día")
    )
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    expect(
      await screen.findByText("ese bloque se superpone con otro del mismo día")
    ).toBeInTheDocument()
  })
})
