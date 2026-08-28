import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"

import { NuevaReservaPage } from "@/features/reservas/NuevaReservaPage"
import * as reservasApi from "@/features/reservas/api"
import type { MateriaReservable, EquipoDisponible } from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"
import { fechaFuturaEnDias } from "@/test/fechas"

vi.mock("@/features/reservas/api")

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
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/reservas/nueva"]}>
        <Routes>
          <Route path="/reservas/nueva" element={<NuevaReservaPage />} />
          <Route path="/reservas" element={<ListadoDePrueba />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
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
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: materias })
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({ data: equipos })
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
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(reservasApi.crearReservaRecurrente).toHaveBeenCalledWith({
      materiaId: "m1",
      diaSemana: "MARTES",
      horaInicio: "08:00",
      horaFin: "09:00",
      fechaInicio: FECHA,
      fechaFin: FECHA_FIN,
      equipoIds: ["pc1"],
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
    await user.click(screen.getByRole("button", { name: "Confirmar reserva" }))

    expect(await screen.findByText("Listado")).toBeInTheDocument()
    expect(
      screen.getByText(/Reserva recurrente confirmada: todos los martes/)
    ).toBeInTheDocument()
  })

  // RF-04.1: sin materias asignadas no hay nada para reservar.
  it("avisa cuando el docente no está asignado a ninguna materia", async () => {
    vi.mocked(reservasApi.misMaterias).mockResolvedValue({ data: [] })
    renderPagina()

    expect(
      await screen.findByText(/No estás asignado a ninguna materia/)
    ).toBeInTheDocument()
  })
})
