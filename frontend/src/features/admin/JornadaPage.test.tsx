import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JornadaPage } from "@/features/admin/JornadaPage"
import type {
  BloqueHorario,
  DiaSemana,
  ImpactoDeJornada,
  ReservaAfectada,
  TramoDeJornada,
} from "@/features/disponibilidad/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query y la pantalla
  // la necesita de verdad.
  ...(await original<typeof disponibilidadApi>()),
  jornadaDeLaInstitucion: vi.fn(),
  reemplazarJornada: vi.fn(),
}))

function bloque(
  diaSemana: DiaSemana,
  horaInicio: string,
  horaFin: string
): BloqueHorario {
  return { id: `${diaSemana}-${horaInicio}`, diaSemana, horaInicio, horaFin }
}

function jornadaCargada(bloques: BloqueHorario[]) {
  vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({
    data: bloques,
    definida: bloques.length > 0,
  })
}

function tramo(
  diaSemana: DiaSemana,
  horaInicio: string,
  horaFin: string
): TramoDeJornada {
  return { diaSemana, horaInicio, horaFin }
}

/**
 * La jornada que se mandó en el último PUT, ordenada.
 *
 * Se ordena para poder compararla sin atarse al orden en que la pantalla la
 * arma: lo que importa es qué jornada queda, no en qué secuencia se listaron
 * los tramos.
 */
function jornadaGuardada(): TramoDeJornada[] {
  const llamadas = vi.mocked(disponibilidadApi.reemplazarJornada).mock.calls
  if (llamadas.length === 0) throw new Error("no se guardó ninguna jornada")
  return [...llamadas[llamadas.length - 1][0]].sort((a, b) =>
    `${a.diaSemana}-${a.horaInicio}`.localeCompare(`${b.diaSemana}-${b.horaInicio}`)
  )
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <JornadaPage />
    </QueryClientProvider>
  )
}

const HABILES: DiaSemana[] = ["LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES"]

/**
 * La tarjeta de edición, para no confundirla con el formulario de alta: los
 * dos tienen los mismos botones de día y los mismos selectores de hora, así
 * que sin acotar la búsqueda las consultas encuentran dos de cada uno.
 */
function enLaEdicion() {
  const tarjeta = screen.getByRole("button", { name: "Guardar" }).closest("li")
  if (tarjeta === null) throw new Error("no hay ninguna tarjeta en edición")
  return within(tarjeta)
}

describe("JornadaPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    jornadaCargada([])
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
      definida: true,
    })
  })

  it("sin jornada declarada avisa que no hay restricción", async () => {
    renderPagina()

    expect(
      await screen.findByText(/Todavía no se declaró la jornada/)
    ).toBeInTheDocument()
  })

  // El motivo de toda la pantalla: la escuela típica abre igual de lunes a
  // viernes y eso tiene que ser un formulario, no cinco.
  it("un atajo carga los cinco días en una sola pasada", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    // Una sola llamada con los cinco días adentro, no cinco llamadas: la
    // jornada se guarda entera.
    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("JUEVES", "08:00", "12:00"),
      tramo("LUNES", "08:00", "12:00"),
      tramo("MARTES", "08:00", "12:00"),
      tramo("MIERCOLES", "08:00", "12:00"),
      tramo("VIERNES", "08:00", "12:00"),
    ])
  })

  it("los días también se marcan de a uno", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Sábado" }))
    await user.click(screen.getByRole("button", { name: "Domingo" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("DOMINGO", "08:00", "12:00"),
      tramo("SABADO", "08:00", "12:00"),
    ])
  })

  // Agregar un tramo no puede llevarse puesto lo que ya estaba: con un PUT
  // del conjunto, olvidarse de lo anterior lo borraría.
  it("agregar un tramo conserva los que ya estaban", async () => {
    const user = userEvent.setup()
    jornadaCargada([bloque("LUNES", "08:00", "12:00")])
    renderPagina()

    await screen.findByRole("button", { name: /^Editar Lunes/ })
    await user.click(screen.getByRole("button", { name: "Sábado" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("LUNES", "08:00", "12:00"),
      tramo("SABADO", "08:00", "12:00"),
    ])
  })

  // Es una restricción del formulario, no del sistema: un tramo sin día no
  // existe.
  it("sin ningún día marcado no deja agregar el tramo y dice por qué", async () => {
    renderPagina()

    expect(await screen.findByText(/se puede reservar cualquier día/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Agregar tramo" })).toBeDisabled()
    expect(screen.getByText("Elegí al menos un día para este tramo.")).toBeInTheDocument()
  })

  it("los cinco bloques de la semana se leen como una sola línea", async () => {
    jornadaCargada(HABILES.map((d) => bloque(d, "07:30", "12:30")))
    renderPagina()

    // Una sola línea, no cinco: la lista tiene un único ítem.
    const lineas = await screen.findAllByRole("listitem")
    expect(lineas).toHaveLength(1)
    expect(lineas[0]).toHaveTextContent("Lunes a viernes de 07:30 a 12:30")
  })

  it("editar un tramo agrupado corrige la hora de todos sus días", async () => {
    const user = userEvent.setup()
    jornadaCargada([
      bloque("LUNES", "08:00", "12:00"),
      bloque("MARTES", "08:00", "12:00"),
    ])
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /^Editar Lunes y martes/ })
    )
    await user.selectOptions(enLaEdicion().getByLabelText("Cierra: hora"), "13")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("LUNES", "08:00", "13:00"),
      tramo("MARTES", "08:00", "13:00"),
    ])
  })

  // Editar los días del grupo es la forma natural de decir "el miércoles
  // también" o "el martes ya no".
  it("sacar y agregar días en la edición deja la jornada con los días nuevos", async () => {
    const user = userEvent.setup()
    jornadaCargada([
      bloque("LUNES", "08:00", "12:00"),
      bloque("MARTES", "08:00", "12:00"),
    ])
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /^Editar Lunes y martes/ })
    )
    await user.click(enLaEdicion().getByRole("button", { name: "Martes" }))
    await user.click(enLaEdicion().getByRole("button", { name: "Miércoles" }))
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("LUNES", "08:00", "12:00"),
      tramo("MIERCOLES", "08:00", "12:00"),
    ])
  })

  // El caso de la escuela que abre igual toda la semana salvo un día: ese día
  // se corrige solo, sin desarmar el grupo ni volver a cargarlo.
  it("desde 'día por día' se edita un solo día del tramo", async () => {
    const user = userEvent.setup()
    jornadaCargada(HABILES.map((d) => bloque(d, "08:00", "12:00")))
    renderPagina()

    await user.click(await screen.findByRole("button", { name: /^Día por día/ }))
    await user.click(screen.getByRole("button", { name: "Editar solo Martes" }))
    // Acotado a la fila del martes: el formulario de alta tiene sus propios
    // selectores de hora con la misma etiqueta.
    const filaDelMartes = within(
      screen.getByRole("button", { name: "Guardar martes" }).closest("li")!
    )
    await user.selectOptions(filaDelMartes.getByLabelText("Cierra: hora"), "14")
    await user.click(screen.getByRole("button", { name: "Guardar martes" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    // Solo el martes cambia; los otros cuatro días viajan igual que estaban.
    expect(jornadaGuardada()).toEqual([
      tramo("JUEVES", "08:00", "12:00"),
      tramo("LUNES", "08:00", "12:00"),
      tramo("MARTES", "08:00", "14:00"),
      tramo("MIERCOLES", "08:00", "12:00"),
      tramo("VIERNES", "08:00", "12:00"),
    ])
  })

  it("desde 'día por día' se quita un solo día del tramo", async () => {
    const user = userEvent.setup()
    jornadaCargada(HABILES.map((d) => bloque(d, "08:00", "12:00")))
    renderPagina()

    await user.click(await screen.findByRole("button", { name: /^Día por día/ }))
    await user.click(screen.getByRole("button", { name: "Quitar solo Miércoles" }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([
      tramo("JUEVES", "08:00", "12:00"),
      tramo("LUNES", "08:00", "12:00"),
      tramo("MARTES", "08:00", "12:00"),
      tramo("VIERNES", "08:00", "12:00"),
    ])
  })

  // Un tramo de un solo día no se despliega: sería la misma línea repetida.
  it("un tramo de un día no ofrece 'día por día'", async () => {
    jornadaCargada([bloque("SABADO", "08:00", "12:00")])
    renderPagina()

    await screen.findByRole("button", { name: /^Editar Sábado/ })
    expect(screen.queryByRole("button", { name: /^Día por día/ })).not.toBeInTheDocument()
  })

  it("quitar un tramo agrupado saca todos sus días y deja el resto", async () => {
    const user = userEvent.setup()
    jornadaCargada([
      bloque("LUNES", "08:00", "12:00"),
      bloque("MARTES", "08:00", "12:00"),
      bloque("SABADO", "09:00", "13:00"),
    ])
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /^Quitar Lunes y martes/ })
    )

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    })
    expect(jornadaGuardada()).toEqual([tramo("SABADO", "09:00", "13:00")])
  })

  // Quitar el último tramo deja la jornada vacía, y eso es un pedido legítimo
  // —la escuela deja de restringir— y no un cuerpo que falte.
  it("quitar el único tramo manda una jornada vacía", async () => {
    const user = userEvent.setup()
    jornadaCargada([bloque("LUNES", "08:00", "12:00")])
    renderPagina()

    await user.click(await screen.findByRole("button", { name: /^Quitar Lunes/ }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledWith([], false)
    })
  })

  // El guardado ya no es parcial: entra entera o no entra. Por eso el error
  // es uno solo y no una lista de días a rehacer.
  it("muestra el error del backend sin dejar la jornada a medias", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValue(
      new ApiError(409, "ese bloque se superpone con otro del mismo día")
    )
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    expect(
      await screen.findByText("ese bloque se superpone con otro del mismo día")
    ).toBeInTheDocument()
  })

  it("avisa cuando el tramo cierra al día siguiente", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.selectOptions(screen.getByLabelText("Abre: hora"), "20")
    await user.selectOptions(screen.getByLabelText("Cierra: hora"), "01")

    expect(screen.getByText("Cierra al día siguiente.")).toBeInTheDocument()
  })

  it("no deja cargar un tramo que empieza y termina a la misma hora", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Lunes" }))
    await user.selectOptions(screen.getByLabelText("Cierra: hora"), "08")

    expect(screen.getByRole("button", { name: "Agregar tramo" })).toBeDisabled()
    expect(
      screen.getByText("La hora de cierre no puede ser igual a la de apertura.")
    ).toBeInTheDocument()
  })
})

// ── La confirmación de lo que se cancela ────────────────────────────────

describe("JornadaPage — el cambio que deja reservas afuera", () => {
  /** El 409 que devuelve el backend en vez de aplicar el cambio. */
  function rechazaConImpacto(impacto: Partial<ImpactoDeJornada>) {
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValueOnce(
      new ApiError(409, "el cambio deja reservas fuera de la jornada", {
        error: "el cambio deja reservas fuera de la jornada",
        impacto: {
          reservas: [],
          prestamos: [],
          totalAfectadas: (impacto.reservas ?? []).length,
          totalDeReservas: 0,
          ...impacto,
        },
      })
    )
  }

  function reservaAfectada(id: string, docente: string): ReservaAfectada {
    return {
      id,
      fecha: "2026-03-09",
      horaInicio: "13:00",
      horaFin: "15:00",
      equipo: "PC 3",
      materia: "Matemáticas",
      docente,
    }
  }

  beforeEach(() => {
    vi.clearAllMocks()
    jornadaCargada([bloque("LUNES", "08:00", "18:00")])
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
      definida: true,
    })
  })

  async function achicarElTramo(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await screen.findByRole("button", { name: /^Editar Lunes/ }))
    await user.selectOptions(enLaEdicion().getByLabelText("Abre: hora"), "15")
    await user.selectOptions(enLaEdicion().getByLabelText("Cierra: hora"), "16")
    await user.click(screen.getByRole("button", { name: "Guardar" }))
  }

  // Guardar no cancela nada por sí solo: primero se muestra qué se cae y
  // quién decide es el Admin.
  it("no guarda: muestra qué se cancelaría y a cuántos docentes", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [
        reservaAfectada("r1", "Ada Lovelace"),
        reservaAfectada("r2", "Grace H."),
      ],
      totalDeReservas: 10,
    })
    renderPagina()

    await achicarElTramo(user)

    expect(await screen.findByText(/Se van a cancelar/)).toHaveTextContent(
      "Se van a cancelar 2 reservas de 2 docentes"
    )
    // La primera llamada fue sin confirmar, y no hubo una segunda.
    expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
    expect(vi.mocked(disponibilidadApi.reemplazarJornada).mock.calls[0][1]).toBe(false)
  })

  it("confirmar reenvía la misma jornada, ahora con la confirmación", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [reservaAfectada("r1", "Ada Lovelace")],
      totalDeReservas: 10,
    })
    vi.mocked(disponibilidadApi.reemplazarJornada).mockResolvedValue({
      data: [],
      definida: true,
      reservasCanceladas: 1,
    })
    renderPagina()
    await achicarElTramo(user)

    await user.click(await screen.findByRole("button", { name: /Guardar y cancelar 1/ }))

    await waitFor(() => {
      expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(2)
    })
    const [tramos, confirmado] = vi.mocked(disponibilidadApi.reemplazarJornada).mock
      .calls[1]
    expect(confirmado).toBe(true)
    // Exactamente la jornada que se había propuesto, no otra.
    expect(tramos).toEqual([
      { diaSemana: "LUNES", horaInicio: "15:00", horaFin: "16:00" },
    ])
    expect(await screen.findByText(/Se cancelaron 1 reserva/)).toBeInTheDocument()
  })

  it("volver atrás no manda nada y deja la jornada como estaba", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [reservaAfectada("r1", "Ada Lovelace")],
      totalDeReservas: 10,
    })
    renderPagina()
    await achicarElTramo(user)

    await user.click(
      await screen.findByRole("button", { name: "Volver sin cambiar nada" })
    )

    expect(screen.queryByText(/Se van a cancelar/)).not.toBeInTheDocument()
    expect(disponibilidadApi.reemplazarJornada).toHaveBeenCalledTimes(1)
  })

  // El backend recorta la lista, pero el número que decide es el total. Si la
  // pantalla contara las filas que le llegaron, el Admin confirmaría creyendo
  // que cancela cincuenta cuando cancela doscientas.
  it("cuenta las afectadas de verdad, no las que entraron en la lista", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [reservaAfectada("r1", "Ada"), reservaAfectada("r2", "Grace")],
      totalAfectadas: 200,
      totalDeReservas: 1000,
    })
    renderPagina()

    await achicarElTramo(user)

    expect(await screen.findByText(/Se van a cancelar/)).toHaveTextContent(
      "Se van a cancelar 200 reservas"
    )
    expect(screen.getByText("y 198 más")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Guardar y cancelar 200 reservas" })
    ).toBeInTheDocument()
  })

  // El detector del tipeo: un cambio real no suele llevarse casi todo.
  it("avisa distinto cuando el cambio se lleva casi todas las reservas", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [
        reservaAfectada("r1", "Ada"),
        reservaAfectada("r2", "Grace"),
        reservaAfectada("r3", "Alan"),
      ],
      totalDeReservas: 3,
    })
    renderPagina()

    await achicarElTramo(user)

    expect(
      await screen.findByText(/Eso es casi todo lo que hay reservado/)
    ).toBeInTheDocument()
  })

  // Los préstamos se ven pero no se cancelan: la máquina está afuera.
  // La jornada no restringe los préstamos, así que lo que se avisa no es que
  // el préstamo quede fuera de horario: es que su clase deja de existir
  // mientras el docente tiene las máquinas en la mano.
  it("avisa cuáles de las clases canceladas ya tienen la máquina entregada", async () => {
    const user = userEvent.setup()
    rechazaConImpacto({
      reservas: [reservaAfectada("r1", "Marta")],
      prestamos: [{ id: "p1", equipo: "PC 7", quien: "Marta" }],
      totalDeReservas: 4,
    })
    renderPagina()

    await achicarElTramo(user)

    expect(
      await screen.findByText(/ya tiene su computadora entregada/)
    ).toBeInTheDocument()
    expect(screen.getByText(/El préstamo no se cancela/)).toBeInTheDocument()
    expect(screen.getByText(/PC 7 · la tiene Marta/)).toBeInTheDocument()
  })

  // Un 409 por solape de tramos no trae impacto: es un error normal y se
  // muestra como tal, sin pedir confirmar nada.
  it("un 409 sin impacto se muestra como error, no como confirmación", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.reemplazarJornada).mockRejectedValueOnce(
      new ApiError(409, "ese bloque se superpone con otro del mismo día")
    )
    renderPagina()

    await achicarElTramo(user)

    expect(
      await screen.findByText("ese bloque se superpone con otro del mismo día")
    ).toBeInTheDocument()
    expect(screen.queryByText(/Se van a cancelar/)).not.toBeInTheDocument()
  })
})
