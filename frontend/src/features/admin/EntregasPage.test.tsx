import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import * as academicoApi from "@/features/academico/api"
import * as adminApi from "@/features/admin/api"
import { EntregasPage } from "@/features/admin/EntregasPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo } from "@/features/inventory/types"
import type { Curso } from "@/features/academico/types"
import type { Usuario } from "@/features/auth/types"
import * as reservasApi from "@/features/reservas/api"
import type {
  MateriaReservable,
  Prestamo,
  ReservaDetallada,
} from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"
import { hoyISO } from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"
import { useAhora } from "@/lib/useAhora"
import { diaAnteriorISO } from "@/features/reservas/intervalo"

vi.mock("@/features/reservas/api")
vi.mock("@/features/inventory/api")
vi.mock("@/features/academico/api")
vi.mock("@/features/admin/api")
vi.mock("@/lib/useAhora")

function prestamo(over: Partial<Prestamo> = {}): Prestamo {
  return {
    id: "pr1",
    equipoId: "pc1",
    entregadoANombre: "Ada Lovelace",
    entregadoEn: "2026-08-07T08:05:00Z",
    abierto: true,
    demorado: false,
    identificador: 3,
    carroNombre: "Carro 1",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function usuario(over: Partial<Usuario> = {}): Usuario {
  return {
    id: "u1",
    nombre: "Ana",
    apellido: "Gómez",
    email: "ana.gomez@escuela.test",
    rol: "DOCENTE",
    estado: "APROBADA",
    fechaRegistro: "2026-03-01T00:00:00Z",
    fechaAprobacion: "2026-03-02T00:00:00Z",
    debeCambiarPassword: false,
    tienePassword: true,
    vinculadaAGoogle: false,
    ...over,
  }
}

function materiaDe(cursoNombre: string, cursoModalidad?: string): MateriaReservable {
  return {
    materiaId: `m-${cursoNombre}${cursoModalidad ?? ""}`,
    materiaNombre: "Programación",
    cursoId: `c-${cursoNombre}${cursoModalidad ?? ""}`,
    cursoNombre,
    cursoModalidad,
    cicloId: "ciclo1",
    cicloAnio: 2026,
  }
}

/** Un curso del ciclo, para las sugerencias de destino. */
function cursoDelCiclo(nombre: string, modalidad?: string): Curso {
  const [anio, division] = nombre.split("°")
  return {
    id: `c-${nombre}${modalidad ?? ""}`,
    cicloLectivoId: "ciclo1",
    nombre,
    anio: Number(anio),
    division,
    modalidad,
    archivado: false,
  }
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "res1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: hoyISO(),
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    nombreDocenteSnapshot: "Ada Lovelace",
    identificador: 3,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "5°A",
    etiqueta: `PC ${over.identificador ?? 3}`,
    ...over,
  }
}

function equipoSuelto(over: Partial<Equipo> = {}): Equipo {
  return {
    id: "eq1",
    nombre: "Proyector Epson",
    etiqueta: "Proyector Epson",
    tipo: "PROYECTOR",
    reservable: true,
    esComputadora: false,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const pagina = render(
    <QueryClientProvider client={queryClient}>
      <EntregasPage />
    </QueryClientProvider>
  )
  return { ...pagina, queryClient }
}

describe("EntregasPage", () => {
  it("incluye una nocturna de ayer que sigue en curso y excluye las que ya terminaron", async () => {
    const ahora = new Date()
    ahora.setHours(0, 30, 0, 0)
    vi.mocked(useAhora).mockReturnValue(ahora)
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          fecha: diaAnteriorISO(hoyISO()),
          horaInicio: "22:00",
          horaFin: "01:00",
          materiaNombre: "Nocturna",
        }),
        reserva({
          id: "terminada",
          reservaGrupoId: "terminado",
          equipoId: "otra",
          fecha: diaAnteriorISO(hoyISO()),
          horaInicio: "18:00",
          horaFin: "21:00",
          materiaNombre: "Clase de ayer terminada",
        }),
      ])
    )
    renderPagina()
    expect(await screen.findByText("Nocturna · 5°A")).toBeInTheDocument()
    expect(screen.queryByText(/Clase de ayer terminada/)).not.toBeInTheDocument()
  })
  it("un fallo de reservas no anuncia que todas se retiraron y permite reintentar", async () => {
    vi.mocked(reservasApi.listarReservas).mockRejectedValue(
      new ApiError(503, "sin reservas")
    )
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/No pudimos consultar las reservas y sus entregas/)
    expect(
      screen.queryByText(/No queda ninguna reserva de hoy sin retirar/)
    ).not.toBeInTheDocument()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    expect(await screen.findByRole("checkbox", { name: /^PC 3/ })).toBeInTheDocument()
  })
  it("recorre más de 200 reservas y muestra las clases de la última página", async () => {
    const primeras = Array.from({ length: 200 }, (_, i) =>
      reserva({ id: `r-${i}`, equipoId: `pc-${i}`, identificador: i + 1 })
    )
    vi.mocked(reservasApi.listarReservas).mockImplementation(async (f) =>
      f?.page === 2
        ? paginada(
            [
              reserva({
                id: "ultima",
                reservaGrupoId: "ultimo",
                materiaNombre: "Última clase",
              }),
            ],
            { total: 201, page: 2, pageSize: 200 }
          )
        : paginada(primeras, { total: 201, page: 1, pageSize: 200 })
    )
    renderPagina()
    expect(await screen.findByText("Última clase · 5°A")).toBeInTheDocument()
    expect(reservasApi.listarReservas).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2 })
    )
  })
  it("un error de inventario no se presenta como falta de equipos y conserva el nombre al reintentar", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockRejectedValue(
      new ApiError(503, "sin inventario")
    )
    const user = userEvent.setup()
    renderPagina()
    await user.click(screen.getByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Persona de prueba")
    await screen.findByText(/No pudimos verificar qué equipos se pueden entregar/)
    expect(
      screen.queryByText(/No hay equipos disponibles para entregar/)
    ).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Entregar" })).toBeDisabled()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipoSuelto()] })
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await user.click(await screen.findByRole("checkbox", { name: /Proyector Epson/ }))
    expect(screen.getByLabelText("¿A quién?")).toHaveValue("Persona de prueba")
    expect(screen.getByRole("button", { name: "Entregar 1 equipo" })).toBeEnabled()
  })
  it("bloquea una selección que dejó de poder entregarse durante un refresco", async () => {
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipoSuelto()] })
    const user = userEvent.setup()
    const { queryClient } = renderPagina()
    await user.click(screen.getByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Persona de prueba")
    await user.click(await screen.findByRole("checkbox", { name: /Proyector Epson/ }))
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto({ estado: "EN_MANTENIMIENTO" })],
    })
    await queryClient.invalidateQueries({ queryKey: ["equipos"] })
    await screen.findByText(/Cambió la disponibilidad de los equipos elegidos/)
    expect(screen.getByRole("button", { name: "Entregar 1 equipo" })).toBeDisabled()
    expect(reservasApi.entregarSuelta).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: /Limpiar la selección/ }))
    expect(screen.queryByText(/Cambió la disponibilidad/)).not.toBeInTheDocument()
  })
  it.each([false, true])(
    "una salida sin equipos entregados conserva el formulario (reparación: %s)",
    async (reparacion) => {
      vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
        data: [equipoSuelto({ estado: reparacion ? "EN_MANTENIMIENTO" : "DISPONIBLE" })],
      })
      vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({
        entregadas: [],
        noEntregadas: [
          {
            equipoId: "eq1",
            razon: "YA_ENTREGADA",
            detalle: "ese equipo ya está prestado",
          },
        ],
      })
      const user = userEvent.setup()
      renderPagina()
      await user.click(
        screen.getByRole("button", {
          name: reparacion ? "Sacar un equipo a reparación" : "Entregar sin reserva",
        })
      )
      await user.type(
        screen.getByLabelText(reparacion ? "¿Quién se lo lleva?" : "¿A quién?"),
        "Persona de prueba"
      )
      if (reparacion)
        await user.type(
          screen.getByLabelText("¿A dónde va y por qué?"),
          "Service de prueba"
        )
      await user.click(await screen.findByRole("checkbox", { name: /Proyector Epson/ }))
      await user.click(
        screen.getByRole("button", {
          name: reparacion ? "Registrar la salida de 1 equipo" : "Entregar 1 equipo",
        })
      )
      await screen.findByText(/ese equipo ya está prestado/)
      expect(
        screen.getByLabelText(reparacion ? "¿Quién se lo lleva?" : "¿A quién?")
      ).toHaveValue("Persona de prueba")
    }
  )
  it("el modo liberar el resto se reinicia y las reservas se actualizan tras una entrega", async () => {
    const filas = [1, 2, 3, 4].map((i) =>
      reserva({
        id: `r${i}`,
        equipoId: `pc${i}`,
        identificador: i,
        reservaGrupoId: i <= 2 ? "g1" : "g2",
      })
    )
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada(filas))
    vi.mocked(reservasApi.entregarPorReserva).mockImplementationOnce(async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada(filas.map((r) => (r.id === "r2" ? { ...r, estado: "NO_RETIRADA" } : r)))
      )
      vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
        data: [prestamo({ equipoId: "pc1", reservaId: "r1" })],
      })
      return {
        entregadas: [prestamo({ equipoId: "pc1", reservaId: "r1" })],
        liberadas: ["r2"],
      }
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(await screen.findByRole("checkbox", { name: /^PC 1/ }))
    await user.click(screen.getByRole("checkbox", { name: /Es todo lo que se llevó/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo y liberar/ }))
    await screen.findByText(/Quedó libre 1 equipo/)
    expect(await screen.findByText(/reserva liberada/)).toBeInTheDocument()
    await user.click(screen.getByRole("checkbox", { name: /^PC 3/ }))
    expect(
      screen.getByRole("checkbox", { name: /Es todo lo que se llevó/ })
    ).not.toBeChecked()
    await user.click(screen.getByRole("button", { name: "Entregar 1 equipo" }))
    expect(reservasApi.entregarPorReserva).toHaveBeenLastCalledWith(
      expect.objectContaining({ reservaIds: ["r3"], liberarNoEntregadas: false })
    )
  })
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAhora).mockReturnValue(new Date())
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.entregarPorReserva).mockResolvedValue({ entregadas: [] })
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({ entregadas: [] })
    vi.mocked(reservasApi.recibirEquipos).mockResolvedValue({ recibidos: [] })
    // Los cursos del ciclo son la mitad de las sugerencias de destino.
    vi.mocked(academicoApi.listarCiclos).mockResolvedValue({
      data: [{ id: "ciclo1", anio: 2026, activo: true, archivado: false }],
    })
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [cursoDelCiclo("1°4")],
    })
    // Las cuentas aprobadas: se ofrecen para reconocer a quien viene al
    // mostrador, sin dejar de admitir a quien no tiene ninguna.
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue({
      data: [usuario()],
      meta: { total: 1, page: 1, pageSize: 200 },
    })
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1", dadoDeBaja: false }],
    })
    // Una sola consulta trae todo el inventario: la de carro y las sueltas.
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        {
          id: "pc1",
          carroId: "c1",
          identificador: 3,
          numeroSerie: "5CD1234ABC",
          etiqueta: "PC 3",
          tipo: "PC",
          reservable: true,
          esComputadora: true,
          freezado: false,
          estado: "DISPONIBLE",
          dadoDeBaja: false,
          fechaAlta: "2026-01-01T00:00:00Z",
        },
      ],
    })
  })

  it("muestra qué computadoras están afuera y quién las tiene", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ materiaNombre: "Matemáticas" })],
    })
    renderPagina()

    expect(await screen.findByText(/PC 3 · Carro 1/)).toBeInTheDocument()
    expect(screen.getByText(/Ada Lovelace/)).toBeInTheDocument()
  })

  it("marca la demora de las que no volvieron a horario", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ demorado: true, minutosDeDemora: 25 })],
    })
    renderPagina()

    expect(await screen.findByText("25 min tarde")).toBeInTheDocument()
  })

  it("dice horas y minutos cuando la demora pasa de una hora", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ demorado: true, minutosDeDemora: 130 })],
    })
    renderPagina()

    expect(await screen.findByText("2 h 10 min tarde")).toBeInTheDocument()
  })

  /**
   * "Sin hora de devolución" no es un dato faltante: es un préstamo al que no
   * se le puede reclamar nada, y la pantalla tiene que decirlo con esas
   * palabras para que nadie lo lea como un error de carga.
   */
  it("distingue las que no tienen hora de devolución", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ destino: "Biblioteca" })],
    })
    renderPagina()

    expect(await screen.findByText(/sin hora de devolución/)).toBeInTheDocument()
  })

  // La otra mitad de una entrega parcial: declarar que eso es TODO lo que se
  // llevó. Sin marcarlo, la reserva sigue siendo del docente — el caso de "ya
  // vuelvo por las otras dos".
  it("puede declarar que lo entregado es todo, y libera el resto", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2" })])
    )
    renderPagina()

    const casillas = await screen.findAllByRole("checkbox")
    await user.click(casillas[0])
    await user.click(screen.getByLabelText(/Es todo lo que se llevó/))
    await user.click(screen.getByRole("button", { name: /y liberar el resto/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: undefined,
      liberarNoEntregadas: true,
    })
  })

  it("recibe una computadora desde su fila", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /Recibir 1 equipo de Ada Lovelace/ })
    )

    expect(reservasApi.recibirEquipos).toHaveBeenCalledWith({
      prestamoIds: ["pr1"],
      observaciones: undefined,
    })
  })

  it("recibe varias juntas con una observación común", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo(), prestamo({ id: "pr2", equipoId: "pc2", identificador: 4 })],
    })
    renderPagina()

    const casillas = await screen.findAllByRole("checkbox", { name: /Seleccionar PC/ })
    await user.click(casillas[0])
    await user.click(casillas[1])
    await user.type(screen.getByLabelText(/Observaciones/), "faltó un cargador")
    await user.click(
      screen.getByRole("button", { name: "Recibir 2 equipos seleccionados" })
    )

    expect(reservasApi.recibirEquipos).toHaveBeenCalledWith({
      prestamoIds: ["pr1", "pr2"],
      observaciones: "faltó un cargador",
    })
  })

  /**
   * El caso que planteó la escuela: devolver tres de cuatro no necesita nada
   * especial, la que falta simplemente sigue figurando afuera.
   */
  it("avisa cuando alguna ya figuraba adentro", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo()],
    })
    vi.mocked(reservasApi.recibirEquipos).mockResolvedValue({
      recibidos: [],
      noRecibidos: [{ prestamoId: "pr1", detalle: "esa computadora ya figura devuelta" }],
    })
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: /Recibir 1 equipo de Ada Lovelace/ })
    )

    expect(await screen.findByText(/1 equipo ya figuraba adentro/)).toBeInTheDocument()
  })

  it("entrega los equipos de una reserva del día", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar todas" }))
    await user.click(screen.getByRole("button", { name: /Entregar 2 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1", "res2"],
      retiradoPor: undefined,
      liberarNoEntregadas: false,
    })
  })

  /** Retiro parcial: se lleva una de las dos. */
  it("permite entregar solo algunas de la reserva", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "res2", equipoId: "pc2", identificador: 4 })])
    )
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 4/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res2"],
      retiradoPor: undefined,
      liberarNoEntregadas: false,
    })
  })

  /** El docente manda a un alumno, que es lo habitual. */
  it("anota quién retira sin cambiar de quién es la responsabilidad", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.type(screen.getByLabelText(/Quién las retira/), "Juan (alumno)")
    expect(screen.getByText(/quedan igual a cargo del docente/i)).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: "Juan (alumno)",
      liberarNoEntregadas: false,
    })
  })

  // Es opcional: a una institución le sirve anotar al alumno y a otra le
  // sobra. Obligarlo llevaría a que se escriba cualquier cosa para seguir.
  it("se puede entregar sin anotar quién retira", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(reservasApi.entregarPorReserva).toHaveBeenCalledWith({
      reservaIds: ["res1"],
      retiradoPor: undefined,
      liberarNoEntregadas: false,
    })
  })

  /**
   * Una máquina ya entregada no se puede volver a entregar, y la pantalla no
   * la ofrece: se cruza por equipoId contra lo que está afuera.
   */
  it("no ofrece para entregar un equipo que ya está afuera", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ equipoId: "pc1" })],
    })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    expect(
      await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    ).toBeInTheDocument()
  })

  it("entrega sin reserva a alguien que no tiene cuenta", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta (secretaría)")
    await user.type(screen.getByLabelText(/A dónde va/), "Sección Alumnos")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith({
      equipoIds: ["pc1"],
      nombre: "Marta (secretaría)",
      destino: "Sección Alumnos",
      devolucionEstimada: undefined,
    })
  })

  /**
   * El destino se sugiere con los cursos que la escuela tiene cargados y con
   * los lugares que no son un curso, pero el campo es libre: una lista
   * cerrada dejaría sin poder anotarse el primer destino no previsto.
   */
  it("sugiere los cursos del ciclo y los lugares de la escuela como destino", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    const campo = await screen.findByLabelText(/A dónde va/)
    const sugerencias = document.getElementById(campo.getAttribute("list")!)

    expect(sugerencias).toHaveTextContent("")
    const valores = [...sugerencias!.querySelectorAll("option")].map((o) => o.value)
    expect(valores).toContain("1°4")
    expect(valores).toContain("Biblioteca")
  })

  /**
   * Reconocer a quien viene no cambia lo que se puede hacer —el nombre sigue
   * siendo libre— pero sí lo que queda anotado: la entrega se asocia a esa
   * cuenta, así aparece en su historial y el reclamo de devolución le llega.
   */
  it("asocia la entrega a la cuenta cuando el nombre coincide con una", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "ana gómez")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith(
      expect.objectContaining({ nombre: "ana gómez", usuarioId: "u1" })
    )
  })

  // El caso normal del mostrador: quien viene a buscar una máquina para un
  // trámite no tiene cuenta, y la entrega sale igual.
  it("entrega a nombre de alguien que no tiene cuenta, sin asociar ninguna", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta (secretaría)")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith(
      expect.objectContaining({ nombre: "Marta (secretaría)", usuarioId: undefined })
    )
  })

  /**
   * Con un solo curso el destino se completa solo. Es lo más probable, y el
   * Admin lo tiene a la vista para corregirlo.
   */
  it("completa el destino con el curso de quien retira, si da uno solo", async () => {
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({
      data: [materiaDe("1°4")],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Ana Gómez")

    expect(await screen.findByLabelText(/A dónde va/)).toHaveValue("1°4")
  })

  // Puede que la pida para ella, que la lleve a sala de profesores o que no
  // lo declare: lo sugerido se borra y la entrega sale sin destino.
  it("deja borrar el destino que completó solo", async () => {
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({
      data: [materiaDe("1°4")],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Ana Gómez")
    await user.clear(await screen.findByLabelText(/A dónde va/))
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith(
      expect.objectContaining({ destino: undefined })
    )
  })

  // Con dos cursos no hay cuál elegir, así que no se adivina: el campo queda
  // vacío y los dos se ofrecen como sugerencia.
  it("no adivina el destino si la persona da clase en varios cursos", async () => {
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({
      data: [materiaDe("1°4"), materiaDe("2°1°")],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Ana Gómez")
    await screen.findByText(/Da clase en 1°4, 2°1°/)

    expect(screen.getByLabelText(/A dónde va/)).toHaveValue("")
  })

  /**
   * RF-08.26: el destino se guarda con el nombre pelado del curso. En una
   * escuela que numera sus divisiones de corrido el nombre ya identifica, y
   * agregarle la modalidad sólo alarga el registro que alguien va a leer
   * dentro de dos meses.
   */
  it("usa el nombre pelado del curso cuando no se repite", async () => {
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [
        cursoDelCiclo("4°1", "Construcción"),
        cursoDelCiclo("4°2", "Electromecánica"),
      ],
    })
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({
      data: [materiaDe("4°2", "Electromecánica")],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Ana Gómez")

    await waitFor(() => expect(screen.getByLabelText(/A dónde va/)).toHaveValue("4°2"))
  })

  /**
   * Y lleva la modalidad al lado en cuanto ese nombre se repite: dos carreras
   * con su "1°A" son dos lugares distintos, y "1°A" a secas no dice a cuál se
   * fue la máquina.
   */
  it("agrega la modalidad cuando el nombre del curso se repite", async () => {
    vi.mocked(academicoApi.listarCursos).mockResolvedValue({
      data: [cursoDelCiclo("1°A", "Enfermería"), cursoDelCiclo("1°A", "Contabilidad")],
    })
    vi.mocked(academicoApi.materiasDeDocente).mockResolvedValue({
      data: [materiaDe("1°A", "Enfermería")],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Ana Gómez")

    await waitFor(() =>
      expect(screen.getByLabelText(/A dónde va/)).toHaveValue("1°A · Enfermería")
    )
  })

  /**
   * El cuadro se cierra solo al entregar: quien lo completa tiene a alguien
   * esperando enfrente, y apretar «Cerrar» después de entregar es un paso que
   * no decide nada. El resumen queda en la página, arriba de la lista donde
   * las máquinas que salieron ya figuran.
   */
  it("cierra el cuadro solo al entregar y deja el resumen a la vista", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({
      entregadas: [prestamo()],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(await screen.findByText(/Salieron 1 equipo/)).toBeInTheDocument()
    expect(screen.queryByLabelText("¿A quién?")).not.toBeInTheDocument()
    // Y el botón que lo vuelve a abrir está de nuevo en su lugar.
    expect(
      screen.getByRole("button", { name: "Entregar sin reserva" })
    ).toBeInTheDocument()
  })

  /**
   * El aviso de reserva próxima no impide la entrega: el sistema no sabe
   * cuánto va a durar un trámite, así que la decisión es del Admin.
   */
  it("avisa si el equipo entregado suelta tiene una reserva encima", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarSuelta).mockResolvedValue({
      entregadas: [prestamo()],
      avisos: [
        {
          equipoId: "pc1",
          fecha: "2026-08-07",
          horaInicio: "10:00",
          horaFin: "11:00",
          docente: "Ada Lovelace",
        },
      ],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await user.type(screen.getByLabelText("¿A quién?"), "Marta")
    await user.click(await screen.findByRole("checkbox", { name: /^PC 3/ }))
    await user.click(screen.getByRole("button", { name: /^Entregar 1 equipo/ }))

    expect(
      await screen.findByText(/tiene reserva 2026-08-07 de 10:00 a 11:00/)
    ).toBeInTheDocument()
  })

  /**
   * Un bloqueo administrativo no tiene docente: lo crea un Admin sobre
   * equipos sueltas y no hay nadie esperando para retirarlas.
   */
  it("no ofrece los bloqueos administrativos para entregar", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({
          id: "bloq1",
          tipo: "BLOQUEO",
          materiaNombre: undefined,
          nombreDocenteSnapshot: undefined,
        }),
      ])
    )
    renderPagina()

    expect(
      await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    ).toBeInTheDocument()
  })

  /**
   * El listado pagina de a 50 por defecto y un día con ocho clases de ocho
   * máquinas son 64 reservas: sin pedir el máximo, las últimas del día no
   * aparecían para entregar y nada lo avisaba.
   */
  it("pide el máximo de reservas del día para no perder ninguna", async () => {
    renderPagina()

    await screen.findByText(/No queda ninguna reserva de hoy sin retirar/)
    expect(reservasApi.listarReservas).toHaveBeenCalledWith(
      expect.objectContaining({ pageSize: 200 })
    )
  })

  it("explica el estado cuando no hay nada afuera", async () => {
    renderPagina()

    expect(await screen.findByText("No hay ningún equipo entregado.")).toBeInTheDocument()
  })

  /**
   * Entregar contra una reserva ya liberada es legítimo —el docente llegó
   * tarde y el equipo seguía ahí— pero en ese rato otro pudo reservarlo.
   */
  it("avisa si el equipo que se entrega tiene una reserva de otro encima", async () => {
    const user = userEvent.setup()
    vi.mocked(reservasApi.entregarPorReserva).mockResolvedValue({
      entregadas: [],
      avisos: [
        {
          equipoId: "pc1",
          fecha: "2026-08-11",
          horaInicio: "10:00",
          horaFin: "11:00",
          docente: "Grace Hopper",
        },
      ],
    })
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Marcar todas" }))
    await user.click(screen.getByRole("button", { name: /Entregar 1 equipo/ }))

    expect(await screen.findByText(/Grace Hopper/)).toBeInTheDocument()
    expect(screen.getByText(/tiene reserva/)).toBeInTheDocument()
  })

  /**
   * Los dos nombres, y en este orden: primero quien responde, después quien
   * pasó a buscarlo.
   */
  it("la lista de afuera nombra a quien responde y a quien retiró", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [
        prestamo({
          entregadoANombre: "Ada Lovelace",
          retiradoPor: "Juan (alumno)",
        }),
      ],
    })
    renderPagina()

    // Quien RESPONDE encabeza el grupo; quien vino a buscarla se dice al lado
    // de su máquina, que es donde importa: a quien reclamarle no cambió.
    expect(await screen.findByText("Ada Lovelace")).toBeInTheDocument()
    expect(screen.getByText(/retiró Juan \(alumno\)/)).toBeInTheDocument()
  })

  // Sin nadie anotado no se inventa un renglón: lo retiró quien responde.
  it("no dice nada de quién retiró cuando no se anotó", async () => {
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ entregadoANombre: "Ada Lovelace" })],
    })
    renderPagina()

    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument()
    expect(screen.queryByText(/retiró/)).not.toBeInTheDocument()
  })

  /**
   * Lo que más se presta de forma espontánea no son las computadoras de un
   * carro: es el proyector, el cargador, la notebook suelta (RF-03.16).
   */
  it("ofrece también los equipos que no están en ningún carro", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto()],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    expect(
      await screen.findByRole("checkbox", { name: /^Proyector Epson/ })
    ).toBeInTheDocument()
  })

  // No se filtra por `reservable`: un cargador no se reserva pero sí se
  // presta, y ese es su caso principal.
  it("ofrece lo que no se puede reservar pero sí prestar", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipoSuelto({
          id: "eq2",
          nombre: "Cargador",
          etiqueta: "Cargador",
          reservable: false,
        }),
      ],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    expect(await screen.findByRole("checkbox", { name: /^Cargador/ })).toBeInTheDocument()
  })

  // Un equipo suelto que ya salió no se puede volver a entregar, igual que
  // una computadora de un carro.
  it("no ofrece un equipo suelto que ya está afuera", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto()],
    })
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({
      data: [prestamo({ equipoId: "eq1", etiqueta: "Proyector Epson" })],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    // El único que queda con ese nombre es el de "Afuera del laboratorio",
    // que empieza con "Seleccionar": el de la lista de entrega no está.
    expect(
      screen.queryByRole("checkbox", { name: /^Proyector Epson/ })
    ).not.toBeInTheDocument()
  })

  /**
   * El listado sale de UNA consulta al inventario, no de una por carro más
   * otra por los sueltos.
   */
  it("arma la lista con una sola consulta al inventario", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))
    await screen.findByRole("checkbox", { name: /^PC 3/ })

    expect(inventoryApi.listarEquipos).toHaveBeenCalledTimes(1)
    expect(inventoryApi.listarEquiposDeCarro).not.toHaveBeenCalled()
  })

  /**
   * El tipo de un equipo suelto es texto libre: "PROYECTOR" y "Proyector"
   * cargados en momentos distintos son el mismo lugar. Como el encabezado del
   * grupo se dibuja en mayúsculas, agrupar por el texto crudo mostraba dos
   * títulos idénticos con un equipo cada uno — se vio al correr la app.
   */
  it("no parte en dos un tipo escrito con distinta caja", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipoSuelto({ tipo: "PROYECTOR" }),
        equipoSuelto({
          id: "eq2",
          nombre: "Proyector viejo",
          etiqueta: "Proyector viejo",
          tipo: "Proyector",
        }),
      ],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    await screen.findByRole("checkbox", { name: /^Proyector Epson/ })
    expect(screen.getAllByText(/^PROYECTOR$/i)).toHaveLength(1)
    // Y como quedaron juntos, el atajo del grupo los abarca a los dos.
    expect(screen.getByRole("button", { name: "Marcar 2 equipos" })).toBeInTheDocument()
  })

  // ── Lo que no está en circulación (RF-08.17) ─────────────────────────

  /**
   * Un equipo en mantenimiento o fuera de servicio está físicamente acá y no
   * se le da a nadie. Antes se ofrecía como cualquier otro y salía: el
   * mostrador dejaba prestar una máquina que un Admin acababa de marcar como
   * rota.
   */
  it("no ofrece para entregar lo que no está disponible", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipoSuelto({ estado: "EN_MANTENIMIENTO" }),
        equipoSuelto({
          id: "eq2",
          nombre: "Notebook chica",
          etiqueta: "Notebook chica",
          estado: "FUERA_DE_SERVICIO",
        }),
      ],
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Entregar sin reserva" }))

    expect(
      await screen.findByText(/No hay equipos disponibles para entregar/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("checkbox", { name: /^Proyector Epson/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("checkbox", { name: /^Notebook chica/ })
    ).not.toBeInTheDocument()
  })

  /**
   * La salida a reparación es la de al lado y ofrece justo lo contrario: solo
   * lo que no está en condiciones de prestarse. Vive en su propio panel para
   * no meter máquinas rotas en la lista que se usa todos los días.
   */
  it("la salida a reparación ofrece solo lo que no está disponible", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [
        equipoSuelto(),
        equipoSuelto({
          id: "eq2",
          nombre: "Notebook chica",
          etiqueta: "Notebook chica",
          estado: "EN_MANTENIMIENTO",
        }),
      ],
    })
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Sacar un equipo a reparación" })
    )

    expect(
      await screen.findByRole("checkbox", { name: /^Notebook chica/ })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("checkbox", { name: /^Proyector Epson/ })
    ).not.toBeInTheDocument()
  })

  // El destino es la constancia: dentro de dos meses es lo único que explica
  // por qué el equipo no está.
  it("registra la salida a reparación con a dónde va el equipo", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({
      data: [equipoSuelto({ estado: "FUERA_DE_SERVICIO" })],
    })
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Sacar un equipo a reparación" })
    )
    await user.type(screen.getByLabelText(/Quién se lo lleva/), "Service Rossi")
    await user.type(screen.getByLabelText(/A dónde va/), "al service, no enciende")
    await user.click(await screen.findByRole("checkbox", { name: /^Proyector Epson/ }))
    await user.click(
      screen.getByRole("button", { name: /^Registrar la salida de 1 equipo/ })
    )

    expect(reservasApi.entregarSuelta).toHaveBeenCalledWith({
      equipoIds: ["eq1"],
      nombre: "Service Rossi",
      destino: "al service, no enciende",
      devolucionEstimada: undefined,
      salidaAReparacion: true,
    })
  })
})
