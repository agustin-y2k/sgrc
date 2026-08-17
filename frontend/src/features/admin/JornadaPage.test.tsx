import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as disponibilidadApi from "@/features/disponibilidad/api"
import { JornadaPage } from "@/features/admin/JornadaPage"
import type { BloqueHorario, DiaSemana } from "@/features/disponibilidad/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/disponibilidad/api", async (original) => ({
  // JORNADA_KEY no es una llamada: es la clave de react-query y la pantalla
  // la necesita de verdad.
  ...(await original<typeof disponibilidadApi>()),
  jornadaDeLaInstitucion: vi.fn(),
  agregarBloqueDeJornada: vi.fn(),
  editarBloqueDeJornada: vi.fn(),
  eliminarBloqueDeJornada: vi.fn(),
}))

function bloque(
  diaSemana: DiaSemana,
  horaInicio: string,
  horaFin: string
): BloqueHorario {
  return { id: `${diaSemana}-${horaInicio}`, diaSemana, horaInicio, horaFin }
}

function jornadaCargada(bloques: BloqueHorario[]) {
  vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({ data: bloques })
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
    vi.mocked(disponibilidadApi.agregarBloqueDeJornada).mockResolvedValue(
      bloque("LUNES", "08:00", "12:00")
    )
    vi.mocked(disponibilidadApi.editarBloqueDeJornada).mockResolvedValue(
      bloque("LUNES", "08:00", "13:00")
    )
    vi.mocked(disponibilidadApi.eliminarBloqueDeJornada).mockResolvedValue(undefined)
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

    await waitFor(() => {
      expect(disponibilidadApi.agregarBloqueDeJornada).toHaveBeenCalledTimes(5)
    })
    expect(disponibilidadApi.agregarBloqueDeJornada).toHaveBeenCalledWith(
      "LUNES",
      "08:00",
      "12:00"
    )
    expect(disponibilidadApi.agregarBloqueDeJornada).toHaveBeenCalledWith(
      "VIERNES",
      "08:00",
      "12:00"
    )
  })

  it("los días también se marcan de a uno", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Sábado" }))
    await user.click(screen.getByRole("button", { name: "Domingo" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    await waitFor(() => {
      expect(disponibilidadApi.agregarBloqueDeJornada).toHaveBeenCalledTimes(2)
    })
  })

  // Es una restricción del formulario, no del sistema: un tramo sin día no
  // existe. La jornada SIN declarar sigue queriendo decir "todo vale", y el
  // cartel de arriba es el que lo dice.
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

  // Lo que faltaba: hasta acá un tramo solo se podía quitar y volver a cargar.
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
      expect(disponibilidadApi.editarBloqueDeJornada).toHaveBeenCalledTimes(2)
    })
    expect(disponibilidadApi.editarBloqueDeJornada).toHaveBeenCalledWith("LUNES-08:00", {
      horaInicio: "08:00",
      horaFin: "13:00",
    })
    expect(disponibilidadApi.agregarBloqueDeJornada).not.toHaveBeenCalled()
  })

  // Editar los días del grupo es la forma natural de decir "el miércoles
  // también" o "el martes ya no": no debería obligar a borrar y recargar.
  it("sacar y agregar días en la edición borra y crea solo lo que cambió", async () => {
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
      expect(disponibilidadApi.eliminarBloqueDeJornada).toHaveBeenCalledWith(
        "MARTES-08:00"
      )
    })
    expect(disponibilidadApi.agregarBloqueDeJornada).toHaveBeenCalledWith(
      "MIERCOLES",
      "08:00",
      "12:00"
    )
    // El lunes no se tocó: no cambió ni de día ni de horario.
    expect(disponibilidadApi.editarBloqueDeJornada).not.toHaveBeenCalled()
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
      expect(disponibilidadApi.editarBloqueDeJornada).toHaveBeenCalledTimes(1)
    })
    expect(disponibilidadApi.editarBloqueDeJornada).toHaveBeenCalledWith("MARTES-08:00", {
      horaInicio: "08:00",
      horaFin: "14:00",
    })
    // Los otros cuatro días no se tocaron.
    expect(disponibilidadApi.eliminarBloqueDeJornada).not.toHaveBeenCalled()
    expect(disponibilidadApi.agregarBloqueDeJornada).not.toHaveBeenCalled()
  })

  it("desde 'día por día' se quita un solo día del tramo", async () => {
    const user = userEvent.setup()
    jornadaCargada(HABILES.map((d) => bloque(d, "08:00", "12:00")))
    renderPagina()

    await user.click(await screen.findByRole("button", { name: /^Día por día/ }))
    await user.click(screen.getByRole("button", { name: "Quitar solo Miércoles" }))

    await waitFor(() => {
      expect(disponibilidadApi.eliminarBloqueDeJornada).toHaveBeenCalledTimes(1)
    })
    expect(disponibilidadApi.eliminarBloqueDeJornada).toHaveBeenCalledWith(
      "MIERCOLES-08:00"
    )
  })

  // Un tramo de un solo día no se despliega: sería la misma línea repetida.
  it("un tramo de un día no ofrece 'día por día'", async () => {
    jornadaCargada([bloque("SABADO", "08:00", "12:00")])
    renderPagina()

    await screen.findByRole("button", { name: /^Editar Sábado/ })
    expect(screen.queryByRole("button", { name: /^Día por día/ })).not.toBeInTheDocument()
  })

  it("quitar un tramo agrupado borra los bloques de todos sus días", async () => {
    const user = userEvent.setup()
    jornadaCargada([
      bloque("LUNES", "08:00", "12:00"),
      bloque("MARTES", "08:00", "12:00"),
    ])
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /^Quitar Lunes y martes/ })
    )

    await waitFor(() => {
      expect(disponibilidadApi.eliminarBloqueDeJornada).toHaveBeenCalledTimes(2)
    })
  })

  // El guardado es parcial por naturaleza: el backend valida un bloque por
  // vez. Si el martes pisa otro tramo, el resto de la semana igual entró.
  it("nombra los días que quedaron sin guardar", async () => {
    const user = userEvent.setup()
    vi.mocked(disponibilidadApi.agregarBloqueDeJornada).mockImplementation((dia) =>
      dia === "MARTES"
        ? Promise.reject(
            new ApiError(409, "ese bloque se superpone con otro del mismo día")
          )
        : Promise.resolve(bloque(dia, "08:00", "12:00"))
    )
    renderPagina()
    await screen.findByText(/Todavía no se declaró la jornada/)

    await user.click(screen.getByRole("button", { name: "Lunes a viernes" }))
    await user.click(screen.getByRole("button", { name: "Agregar tramo" }))

    expect(await screen.findByText(/Martes: /)).toHaveTextContent(
      "ese bloque se superpone con otro del mismo día"
    )
    expect(screen.queryByText(/Lunes: /)).not.toBeInTheDocument()
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
