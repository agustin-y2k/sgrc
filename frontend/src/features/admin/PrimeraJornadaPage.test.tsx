import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as disponibilidadApi from "@/features/disponibilidad/api"
import { useAuth } from "@/features/auth/AuthContext"
import {
  olvidarPedidoDescartado,
  pedidoDescartado,
} from "@/features/admin/pedidoDeJornada"
import { PrimeraJornadaPage } from "@/features/admin/PrimeraJornadaPage"
import type { ImpactoDeJornada } from "@/features/disponibilidad/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/auth/AuthContext")

const navegar = vi.fn()

vi.mock("react-router", async (original) => ({
  ...(await original<typeof import("react-router")>()),
  useNavigate: () => navegar,
}))

const logoutEspia = vi.fn()

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query.
  ...(await original<typeof disponibilidadApi>()),
  reemplazarJornada: vi.fn(),
}))

/** El 409 que devuelve el backend en vez de aplicar el cambio. */
function unImpactoCon(impacto: Partial<ImpactoDeJornada>) {
  return new ApiError(409, "el cambio deja reservas fuera de la jornada", {
    error: "el cambio deja reservas fuera de la jornada",
    impacto: {
      reservas: [],
      prestamos: [],
      totalAfectadas: 0,
      clasesAfectadas: 0,
      totalDeClases: 0,
      totalDeReservas: 0,
      ...impacto,
    },
  })
}

function renderPagina() {
  vi.mocked(useAuth).mockReturnValue({
    user: null,
    isLoading: false,
    login: vi.fn(),
    logout: logoutEspia,
    loginConGoogle: vi.fn(),
    errorDeSesion: null,
    motivoDeCierre: null,
    refetchUser: vi.fn(),
  })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <PrimeraJornadaPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("PrimeraJornadaPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    olvidarPedidoDescartado()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
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

  // Postergar no guarda nada: una jornada vacía YA es el estado actual, así
  // que no hay qué escribir. Solo se calla el pedido por esta sesión.
  it("dejarla libre no guarda nada y solo posterga el pedido", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Dejarla libre por ahora" }))

    expect(disponibilidadApi.reemplazarJornada).not.toHaveBeenCalled()
    expect(pedidoDescartado()).toBe(true)
  })

  // Es la única pantalla del sistema fuera del layout: sin este botón, quien
  // entró con la cuenta equivocada no tendría ninguna forma de salir.
  it("se puede cerrar sesión desde acá", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Cerrar sesión" }))

    expect(logoutEspia).toHaveBeenCalled()
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
          totalAfectadas: 1,
          clasesAfectadas: 1,
          totalDeClases: 40,
          totalDeReservas: 40,
        },
      })
    )
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    await user.click(
      await screen.findByRole("button", { name: /Guardar y cancelar 1 clase/ })
    )

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(2)
    })
    expect(vi.mocked(disponibilidadApi.reemplazarJornada).mock.calls[1][1]).toBe(true)
  })

  // Al confirmar algo destructivo, el asistente cuenta qué pasó en vez de
  // soltar al Admin adentro del sistema sin decirle nada. Es el único momento
  // del flujo en que puede hacer algo con esa información.
  it("después de cancelar clases muestra el resumen antes de dejar entrar", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValueOnce(
      unImpactoCon({
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
        prestamos: [{ id: "p1", equipo: "PC 3", quien: "Ada Lovelace" }],
        totalAfectadas: 43,
        clasesAfectadas: 12,
      })
    )
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
      reservasCanceladas: 43,
      clasesCanceladas: 12,
    })
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))
    await user.click(await screen.findByRole("button", { name: /Guardar y cancelar/ }))

    expect(await screen.findByText("La jornada quedó declarada")).toBeInTheDocument()
    // En clases, igual que la pregunta, con los equipos al lado.
    expect(screen.getByText(/Se cancelaron/)).toHaveTextContent(
      "Se cancelaron 12 clases (43 equipos)"
    )
    // Y la máquina que ya estaba afuera, que es lo accionable ahora.
    expect(screen.getByText(/PC 3 · la tiene Ada Lovelace/)).toBeInTheDocument()
    expect(screen.getByText(/Conviene avisarles a mano/)).toBeInTheDocument()
    // El formulario ya no está: el paso terminó.
    expect(
      screen.queryByRole("button", { name: "Guardar la jornada" })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }))
    expect(navegar).toHaveBeenCalledWith("/", { replace: true })
  })

  // Sin nada que contar se sale de largo: un paso extra para decir "no pasó
  // nada" es peor que no decirlo.
  it("sin clases canceladas no interpone ningún resumen", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))
    await user.click(screen.getByRole("button", { name: "Guardar la jornada" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalled()
    })
    expect(screen.queryByText("La jornada quedó declarada")).not.toBeInTheDocument()
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
