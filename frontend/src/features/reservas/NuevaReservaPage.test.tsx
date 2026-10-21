import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"

import { NuevaReservaPage } from "@/features/reservas/NuevaReservaPage"
import * as reservasApi from "@/features/reservas/api"
import type {
  MateriaReservable,
  EquipoDisponible,
  EquipoOcupado,
} from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"
import * as disponibilidadApi from "@/features/disponibilidad/api"
import { useAhora } from "@/lib/useAhora"
import { fechaFuturaEnDias } from "@/test/fechas"
import { useAuth } from "@/features/auth/AuthContext"
import { BORRADOR_RESERVA_KEY } from "@/lib/borradores"

vi.mock("@/features/reservas/api")
vi.mock("@/features/auth/AuthContext")
vi.mock("@/features/disponibilidad/api", async (original) => ({
  ...(await original<typeof disponibilidadApi>()),
  jornadaDeLaInstitucion: vi.fn(),
}))
vi.mock("@/lib/useAhora")

// Relativas a hoy y no constantes: los inputs de fecha tienen min=hoy, así
// que una fecha fija deja de poder enviarse apenas queda atrás (ver
// src/test/fechas.ts).
const FECHA = fechaFuturaEnDias(7)
const FECHA_FIN = fechaFuturaEnDias(120)

const materias: MateriaReservable[] = [
  {
    materiaId: "m1",
    materiaNombre: "Matemáticas",
    cursoId: "c1",
    cursoNombre: "1°A",
    cicloId: "cl1",
    cicloAnio: 2026,
  },
]

const equipos: EquipoDisponible[] = [
  {
    equipoId: "pc1",
    identificador: 1,
    etiqueta: "PC 1",
    carroId: "car1",
    carroNombre: "Carro 1",
    freezado: false,
    tramo: "NEUTRAL",
    softwareInstalado: "AutoCAD 2027",
  },
  {
    equipoId: "pc2",
    identificador: 7,
    etiqueta: "PC 7",
    carroId: "car2",
    carroNombre: "Carro 2",
    freezado: true,
    tramo: "NEUTRAL",
  },
]

/**
 * El listado, reducido a lo que este archivo necesita comprobar: que se
 * llegó, y con qué mensaje de confirmación.
 */
function ListadoDePrueba() {
  const { state } = useLocation()
  const confirmacion = (state as { confirmacion?: string } | null)?.confirmacion
  return (
    <div>
      Listado
      {confirmacion && <p>{confirmacion}</p>}
    </div>
  )
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const contenido = () => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/reservas/nueva"]}>
        <Routes>
          <Route path="/reservas/nueva" element={<NuevaReservaPage />} />
          <Route path="/reservas" element={<ListadoDePrueba />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
  const pagina = render(contenido())
  return { ...pagina, queryClient, actualizar: () => pagina.rerender(contenido()) }
}

/** Espera a que el listado de materias haya llegado. */
async function elegirMateria(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole("option", { name: /Matemáticas/ })
  await user.selectOptions(screen.getByLabelText("Materia"), "m1")
}

/** Completa materia + fecha + horario, que es lo que habilita el selector. */
async function completarFranja(user: ReturnType<typeof userEvent.setup>) {
  await elegirMateria(user)
  await user.type(screen.getByLabelText("Fecha"), FECHA)
  await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
  await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
  await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "09")
  await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")
}

describe("NuevaReservaPage", () => {
  it("no interpreta un error de jornada como escuela sin restricciones y conserva el formulario al reintentar", async () => {
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockRejectedValue(
      new ApiError(503, "Jornada inaccesible")
    )
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)
    await screen.findByText(/No pudimos consultar la jornada/)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(reservasApi.equiposDisponibles).not.toHaveBeenCalled()
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({ data: [] })
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    expect(screen.getByLabelText("Fecha")).toHaveValue(FECHA)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
  })
  it("un fallo al refrescar la jornada bloquea incluso con disponibilidad previa y permite recuperar la selección", async () => {
    const user = userEvent.setup()
    const { queryClient } = renderPagina()
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockRejectedValue(
      new ApiError(503, "Jornada inaccesible")
    )
    await queryClient.invalidateQueries({ queryKey: disponibilidadApi.JORNADA_KEY })
    await screen.findByText(/No pudimos consultar la jornada/)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({ data: [] })
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    expect(await screen.findByRole("checkbox", { name: /PC 1/ })).toBeChecked()
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
    )
  })
  it("una franja en curso se admite y deja de confirmarse cuando termina aunque no cambien los campos", async () => {
    const dia = new Date()
    dia.setHours(8, 30, 0, 0)
    vi.mocked(useAhora).mockReturnValue(dia)
    const user = userEvent.setup()
    const { actualizar } = renderPagina()
    await completarFranja(user)
    const hoy = `${dia.getFullYear()}-${String(dia.getMonth() + 1).padStart(2, "0")}-${String(dia.getDate()).padStart(2, "0")}`
    await user.clear(screen.getByLabelText("Fecha"))
    await user.type(screen.getByLabelText("Fecha"), hoy)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
    const fin = new Date(dia)
    fin.setHours(9, 0, 0, 0)
    vi.mocked(useAhora).mockReturnValue(fin)
    actualizar()
    await screen.findByText(/El horario elegido ya terminó/)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(reservasApi.crearReserva).not.toHaveBeenCalled()
  })
  async function prepararSerie(user: ReturnType<typeof userEvent.setup>) {
    renderPagina()
    await elegirMateria(user)
    await user.click(screen.getByRole("button", { name: "Se repite todas las semanas" }))
    await user.type(screen.getByLabelText("Desde"), FECHA)
    await user.type(screen.getByLabelText("Hasta"), FECHA_FIN)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "09")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
  }
  it("requiere consentimiento cuando un equipo libre se toma por prioridad en otra fecha", async () => {
    vi.mocked(reservasApi.previsualizarSerie).mockResolvedValue({
      cantidadClases: 2,
      puedeCrear: true,
      equiposATomar: ["pc1"],
      fechas: [
        {
          fecha: FECHA_FIN,
          equipos: [{ equipoId: "pc1", etiqueta: "PC 1", estado: "POR_PRIORIDAD" }],
        },
      ],
    })
    const user = userEvent.setup()
    await prepararSerie(user)
    await user.click(screen.getByRole("button", { name: "Comprobar serie" }))
    const permiso = await screen.findByRole("checkbox", { name: /Autorizo tomar/ })
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    await user.click(permiso)
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))
    expect(reservasApi.crearReservaRecurrente).toHaveBeenCalledWith(
      expect.objectContaining({ equiposATomar: ["pc1"] })
    )
  })
  it("un conflicto futuro bloquea la serie completa y no crea reservas", async () => {
    vi.mocked(reservasApi.previsualizarSerie).mockResolvedValue({
      cantidadClases: 2,
      puedeCrear: false,
      equiposATomar: [],
      fechas: [
        {
          fecha: FECHA_FIN,
          equipos: [
            {
              equipoId: "pc1",
              etiqueta: "PC 1",
              estado: "NO_DISPONIBLE",
              motivo: "Bloqueado por administración.",
            },
          ],
        },
      ],
    })
    const user = userEvent.setup()
    await prepararSerie(user)
    await user.click(screen.getByRole("button", { name: "Comprobar serie" }))
    await screen.findByText(/Hay equipos no disponibles/)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(reservasApi.crearReservaRecurrente).not.toHaveBeenCalled()
  })
  it("cambiar fechas o equipos invalida la comprobación anterior", async () => {
    const user = userEvent.setup()
    await prepararSerie(user)
    await user.click(screen.getByRole("button", { name: "Comprobar serie" }))
    await screen.findByText(/Todos los equipos pueden reservarse/)
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
    await user.click(screen.getByRole("checkbox", { name: /PC 7/ }))
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(
      screen.queryByText(/Todos los equipos pueden reservarse/)
    ).not.toBeInTheDocument()
  })
  it("recupera el formulario y la selección al volver para la misma cuenta", async () => {
    const user = userEvent.setup()
    const pagina = renderPagina()
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).not.toBeNull()
    pagina.unmount()
    renderPagina()
    await screen.findByRole("checkbox", { name: /PC 1/ })
    expect(screen.getByLabelText("Materia")).toHaveValue("m1")
    expect(screen.getByLabelText("Fecha")).toHaveValue(FECHA)
    expect(screen.getByRole("checkbox", { name: /PC 1/ })).toBeChecked()
    expect(screen.getByText(/Borrador recuperado/)).toBeInTheDocument()
  })
  it("un equipo que dejó de estar disponible bloquea la confirmación del borrador", async () => {
    const user = userEvent.setup()
    const pagina = renderPagina()
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    pagina.unmount()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({ data: [] })
    renderPagina()
    await screen.findByText("No hay ningún equipo libre en esa franja.")
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(reservasApi.crearReserva).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "Limpiar la selección" }))
    expect(screen.getByLabelText("Fecha")).toHaveValue(FECHA)
  })
  it("descarta y confirma sin dejar un borrador residual", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)
    await user.click(screen.getByRole("button", { name: "Descartar borrador" }))
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
    expect(screen.getByLabelText("Fecha")).toHaveValue("")
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))
    await screen.findByText("Listado")
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
  })
  it("buscar otra materia mantiene la seleccionada hasta elegir explícitamente otra", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({
      data: [
        materias[0],
        ...Array.from({ length: 12 }, (_, i) => ({
          ...materias[0],
          materiaId: `otra${i}`,
          materiaNombre: `Dibujo Técnico ${i}`,
        })),
      ],
    })
    renderPagina()
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.type(screen.getByLabelText("Buscar materia o curso"), "dibujo tecnico")
    expect(screen.getByLabelText("Materia")).toHaveValue("m1")
    expect(screen.getByRole("checkbox", { name: /PC 1/ })).toBeChecked()
  })
  beforeEach(() => {
    sessionStorage.clear()
    vi.mocked(useAuth).mockReturnValue({ user: { id: "docente1" } } as ReturnType<
      typeof useAuth
    >)
    vi.clearAllMocks()
    vi.mocked(disponibilidadApi.jornadaDeLaInstitucion).mockResolvedValue({ data: [] })
    vi.mocked(useAhora).mockReturnValue(new Date())
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: materias })
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({ data: equipos })
    vi.mocked(reservasApi.previsualizarSerie).mockResolvedValue({
      cantidadClases: 2,
      puedeCrear: true,
      equiposATomar: [],
      fechas: [],
    })
    vi.mocked(reservasApi.crearReserva).mockResolvedValue({ grupo: {}, reservas: [] })
    vi.mocked(reservasApi.crearReservaRecurrente).mockResolvedValue({
      reglaId: "regla1",
      grupos: [],
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Pedir disponibilidad sin franja completa no tiene sentido y el backend
  // responde 400.
  it("no consulta Equipos hasta tener fecha y horario", async () => {
    renderPagina()
    await screen.findByLabelText("Materia")

    expect(reservasApi.equiposDisponibles).not.toHaveBeenCalled()
    expect(screen.getByText(/Elegí la fecha y el horario/)).toBeInTheDocument()
  })

  it("con la franja completa muestra los equipos libres agrupadas por carro", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    // Por rol y no por label: el Checkbox de Radix es un button[role=checkbox],
    // así que esto además verifica que tenga nombre accesible.
    expect(await screen.findByRole("checkbox", { name: /PC 1/ })).toBeInTheDocument()
    expect(screen.getByText("Carro 1")).toBeInTheDocument()
    expect(screen.getByText("Carro 2")).toBeInTheDocument()
    // RF-03.7: el software es el dato que define la elección.
    expect(screen.getByText("AutoCAD 2027")).toBeInTheDocument()
    // RF-03.21: la materia viaja para que la lista salga ordenada para ella.
    expect(reservasApi.equiposDisponibles).toHaveBeenCalledWith(
      expect.objectContaining({ fecha: FECHA, horaInicio: "08:00", horaFin: "09:00" })
    )
  })

  /** El proyector es reservable pero no está en ningún carro. */
  it("ofrece lo que no está en ningún carro bajo su propio título", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: [
        ...equipos,
        {
          equipoId: "eq1",
          etiqueta: "Proyector Epson",
          tipo: "PROYECTOR",
          carroId: "",
          carroNombre: "",
          freezado: false,
          tramo: "NEUTRAL",
        },
      ],
    })
    renderPagina()
    await completarFranja(user)

    expect(
      await screen.findByRole("checkbox", { name: /Proyector Epson/ })
    ).toBeInTheDocument()
    expect(screen.getByText("Otros equipos")).toBeInTheDocument()
  })

  // RF-04.2: "la lista no está restringida a un solo carro, puede combinar
  // Equipos de carros distintos en la misma reserva".
  it("permite combinar equipos de carros distintos en una sola reserva", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("checkbox", { name: /PC 7/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(reservasApi.crearReserva).toHaveBeenCalledWith({
      materiaId: "m1",
      fecha: FECHA,
      horaInicio: "08:00",
      horaFin: "09:00",
      equipoIds: ["pc1", "pc2"],
      // Los dos estaban libres: nadie a quien desplazar (RF-03.21).
      equiposATomar: [],
    })
  })

  it("no deja confirmar sin al menos un equipo seleccionado", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)
    await screen.findByRole("checkbox", { name: /PC 1/ })

    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
  })

  it("avisa si la hora de fin es igual a la de inicio", async () => {
    const user = userEvent.setup()
    renderPagina()
    await elegirMateria(user)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "10")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "10")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    expect(screen.getByText(/no puede ser igual/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
  })

  // Una hora de fin MENOR que la de inicio ya no es un error: significa que
  // la clase termina al día siguiente, que es como dicta una escuela
  // nocturna.
  it("una franja que cruza la medianoche se puede confirmar, y lo avisa", async () => {
    const user = userEvent.setup()
    renderPagina()
    await elegirMateria(user)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "22")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "01")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    expect(screen.getByText(/termina al día siguiente/)).toBeInTheDocument()
    expect(screen.queryByText(/no puede ser igual/)).not.toBeInTheDocument()
  })

  // Espeja domain.MaxDuracionReserva: sin tope, un 00:00–23:59 bloqueaba la
  // Equipo el día entero.
  it("avisa si la reserva dura más que un turno completo", async () => {
    const user = userEvent.setup()
    renderPagina()
    await elegirMateria(user)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "18")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    expect(screen.getByText(/no puede durar más de 8 horas/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
  })

  // RF-04.5
  it("en modo recurrente manda día de la semana y rango de fechas", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Se repite todas las semanas" })
    )
    await elegirMateria(user)
    await user.selectOptions(screen.getByLabelText("Día de la semana"), "MARTES")
    await user.type(screen.getByLabelText("Desde"), FECHA)
    await user.type(screen.getByLabelText("Hasta"), FECHA_FIN)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "09")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    await user.click(screen.getByRole("button", { name: "Comprobar serie" }))
    await screen.findByText(/Todos los equipos pueden reservarse/)
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(reservasApi.crearReservaRecurrente).toHaveBeenCalledWith({
      materiaId: "m1",
      diaSemana: "MARTES",
      horaInicio: "08:00",
      horaFin: "09:00",
      fechaInicio: FECHA,
      fechaFin: FECHA_FIN,
      equipoIds: ["pc1"],
      equiposATomar: [],
    })
  })

  // RF-04.3: el backend informa qué equipos puntuales están ocupados; ese
  // mensaje se muestra tal cual en vez de uno genérico.
  it("muestra el mensaje de solapamiento del backend", async () => {
    vi.mocked(reservasApi.crearReserva).mockRejectedValue(
      new ApiError(409, "una o más Equipos ya tienen una reserva en ese horario")
    )
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(
      await screen.findByText(/ya tienen una reserva en ese horario/)
    ).toBeInTheDocument()
  })

  // Un botón gris en un formulario de siete campos no dice cuál de los siete
  // falta.
  it("dice qué falta para poder confirmar", async () => {
    renderPagina()
    await screen.findByLabelText("Materia")

    expect(screen.getByText(/elegir la materia/)).toBeInTheDocument()
    expect(screen.getByText(/tildar al menos una computadora/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
  })

  it("deja de pedir lo que ya se completó", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    expect(screen.queryByText(/elegir la materia/)).not.toBeInTheDocument()
    expect(screen.queryByText(/elegir la fecha/)).not.toBeInTheDocument()
    // Lo único que queda.
    expect(
      screen.getByText(/Para confirmar falta tildar al menos una computadora/)
    ).toBeInTheDocument()

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))

    expect(screen.queryByText(/Para confirmar falta/)).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
  })

  // Llegar a otra pantalla no alcanza para saber que salió bien, sobre todo
  // en el modo recurrente donde se crearon varias reservas de una vez.
  it("llega al listado con la confirmación de lo que se reservó", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(
      screen.getByText(/Reserva confirmada .*de 08:00 a 09:00, con 1 computadora\./)
    ).toBeInTheDocument()
  })

  it("la confirmación de una recurrente dice hasta cuándo se repite", async () => {
    const user = userEvent.setup()
    renderPagina()
    await elegirMateria(user)

    await user.click(screen.getByRole("button", { name: "Se repite todas las semanas" }))
    await user.selectOptions(screen.getByLabelText("Día de la semana"), "MARTES")
    await user.type(screen.getByLabelText("Desde"), FECHA)
    await user.type(screen.getByLabelText("Hasta"), FECHA_FIN)
    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "08")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "00")
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "09")
    await user.selectOptions(screen.getByLabelText("Hora de fin: minutos"), "00")

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Comprobar serie" }))
    await screen.findByText(/Todos los equipos pueden reservarse/)
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(
      screen.getByText(/Reserva recurrente confirmada: todos los martes/)
    ).toBeInTheDocument()
  })

  // ── El robo por preferencia (RF-03.21) ──────────────────────────────

  /**
   * Un ocupado que la materia puede tomar y otro que no: el primero se tilda
   * como cualquier equipo libre, el segundo sigue siendo intocable.
   */
  const ocupados: EquipoOcupado[] = [
    {
      equipoId: "pc9",
      etiqueta: "PC 9",
      carroNombre: "Carro 1",
      reservaId: "res-debil",
      docenteNombre: "Ana Pérez",
      materiaNombre: "Historia",
      horaInicio: "08:00",
      horaFin: "09:00",
      puedePedirse: false,
      puedeTomarse: true,
    },
    {
      equipoId: "pc10",
      etiqueta: "PC 10",
      carroNombre: "Carro 1",
      reservaId: "res-fuerte",
      docenteNombre: "Luis Gómez",
      materiaNombre: "Dibujo Técnico",
      horaInicio: "08:00",
      horaFin: "09:00",
      puedePedirse: true,
      puedeTomarse: false,
    },
  ]

  it("deja tildar un equipo ocupado que la materia puede tomar", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: equipos,
      ocupados,
    })
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 9/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    // Va en las DOS listas: en equipoIds porque es un equipo de la reserva, y
    // en equiposATomar porque es lo único que autoriza a desplazar a Ana.
    expect(reservasApi.crearReserva).toHaveBeenCalledWith(
      expect.objectContaining({ equipoIds: ["pc9"], equiposATomar: ["pc9"] })
    )
  })

  // Tildar un equipo LIBRE no autoriza ningún robo: si el docente no eligió
  // nada ocupado, la lista va vacía y la reserva se comporta como siempre.
  it("no manda equiposATomar cuando sólo se tildaron equipos libres", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: equipos,
      ocupados,
    })
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(reservasApi.crearReserva).toHaveBeenCalledWith(
      expect.objectContaining({ equipoIds: ["pc1"], equiposATomar: [] })
    )
  })

  // La contracara: sin marca que le gane, el equipo no se tilda y lo único
  // que queda es pedírselo al dueño (RF-04.12).
  it("sobre lo que no se puede tomar sigue ofreciendo pedirlo, sin casilla", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: equipos,
      ocupados,
    })
    renderPagina()
    await completarFranja(user)

    await screen.findByRole("checkbox", { name: /PC 9/ })
    expect(screen.queryByRole("checkbox", { name: /PC 10/ })).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: /Pedírselo/ })).toBeInTheDocument()
  })

  // RF-03.21 lo pide explícitamente, con el criterio de RF-03.19: quien
  // reserva tiene que enterarse de a cuántos corrió.
  it("la confirmación dice cuántas reservas se cancelaron y a cuántos se avisó", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: equipos,
      ocupados,
    })
    vi.mocked(reservasApi.crearReserva).mockResolvedValue({
      grupo: {},
      reservas: [],
      reservasCanceladas: 2,
      docentesAfectados: 1,
    })
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 9/ }))
    expect(screen.getByRole("status")).toHaveTextContent(
      "1 equipo seleccionado: 0 libres y 1 por prioridad"
    )
    expect(screen.getByText(/Vas a reservar 1 equipo/)).toHaveTextContent(
      "El alcance es esta fecha y este horario"
    )
    expect(screen.getByText(/Vas a reservar 1 equipo/)).toHaveTextContent(
      "pueden quedar faltantes"
    )
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(
      screen.getByText(
        /2 equipos quedaron sin reemplazo disponible y se avisó a 1 docente\./
      )
    ).toBeInTheDocument()
  })

  // El verbo concuerda con el número, que es el caso más común de los dos:
  // desplazar UNA clase. "Se cancelaron 1 reserva" es lo que sale de
  // pluralizar el sustantivo y olvidarse del verbo.
  it("con una sola reserva desplazada, la frase va en singular", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
      data: equipos,
      ocupados,
    })
    vi.mocked(reservasApi.crearReserva).mockResolvedValue({
      grupo: {},
      reservas: [],
      reservasCanceladas: 1,
      docentesAfectados: 1,
    })
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 9/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(
      screen.getByText(/1 equipo quedó sin reemplazo disponible y se avisó a 1 docente\./)
    ).toBeInTheDocument()
  })

  it.each([
    [
      2,
      0,
      /Se repusieron 2 equipos en las reservas desplazadas y se avisó a 2 docentes\./,
    ],
    [
      1,
      1,
      /Se repuso 1 equipo en las reservas desplazadas y 1 equipo quedó sin reemplazo disponible y se avisó a 2 docentes\./,
    ],
  ])(
    "muestra reposiciones y faltantes de toda la cadena (%i, %i)",
    async (reubicadas, canceladas, mensaje) => {
      const user = userEvent.setup()
      vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({
        data: equipos,
        ocupados,
      })
      vi.mocked(reservasApi.crearReserva).mockResolvedValue({
        grupo: {},
        reservas: [],
        reservasReubicadas: reubicadas,
        reservasCanceladas: canceladas,
        docentesAfectados: 2,
      })
      renderPagina()
      await completarFranja(user)
      await user.click(await screen.findByRole("checkbox", { name: /PC 9/ }))
      await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))
      expect(await screen.findByText("Listado")).toBeInTheDocument()
      expect(screen.getByText(mensaje)).toBeInTheDocument()
    }
  )

  // El caso de todos los días: sin robo no se dice nada, para que la frase no
  // arrastre un "se cancelaron 0 reservas" que no le importa a nadie.
  it("no habla de cancelaciones cuando no hubo ninguna", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)

    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(screen.queryByText(/sin reemplazo disponible/)).not.toBeInTheDocument()
  })

  it("ofrece pedir una materia desde el perfil cuando no hay asignaciones", async () => {
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: [] })
    renderPagina()
    expect(
      await screen.findByRole("link", { name: "Solicitá una materia desde Mi perfil" })
    ).toHaveAttribute("href", "/perfil")
  })

  // RF-04.1: sin materias asignadas no hay nada para reservar.
  it("una consulta fallida no dice que el docente no está asignado", async () => {
    vi.mocked(reservasApi.misMaterias).mockRejectedValue(
      new ApiError(500, "No se pudieron leer las materias")
    )
    renderPagina()
    expect(
      await screen.findByText("No se pudieron leer las materias")
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/No estás asignado a ninguna materia/)
    ).not.toBeInTheDocument()
  })

  it("cambiar la franja borra la selección y exige volver a elegir equipos", async () => {
    const user = userEvent.setup()
    renderPagina()
    await completarFranja(user)
    await user.click(await screen.findByRole("checkbox", { name: /PC 1/ }))
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeEnabled()
    await user.selectOptions(screen.getByLabelText("Hora de fin: hora"), "10")
    expect(screen.getByRole("button", { name: "Confirmar reserva" })).toBeDisabled()
    expect(await screen.findByRole("checkbox", { name: /PC 1/ })).not.toBeChecked()
    expect(reservasApi.crearReserva).not.toHaveBeenCalled()
  })

  it("avisa cuando el docente no está asignado a ninguna materia", async () => {
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: [] })
    renderPagina()

    expect(
      await screen.findByText(/No estás asignado a ninguna materia/)
    ).toBeInTheDocument()
  })
})
