import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { ReportesPage } from "@/features/admin/ReportesPage"
import * as adminApi from "@/features/admin/api"
import * as csv from "@/lib/csv"
import { formatearDuracion } from "@/features/admin/types"

vi.mock("@/features/admin/api")

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ReportesPage />
    </QueryClientProvider>
  )
  return queryClient
}

describe("formatearDuracion", () => {
  it("convierte minutos a un formato legible", () => {
    expect(formatearDuracion(0)).toBe("0min")
    expect(formatearDuracion(45)).toBe("45min")
    expect(formatearDuracion(60)).toBe("1h")
    expect(formatearDuracion(150)).toBe("2h 30min")
  })
})

describe("ReportesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(adminApi.listarCiclos).mockResolvedValue({
      data: [
        { id: "c-viejo", anio: 2025, activo: false, archivado: true },
        { id: "c-activo", anio: 2026, activo: true, archivado: false },
      ],
    })
    vi.mocked(adminApi.reporteUsoEquipos).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          identificador: 7,
          carroNombre: "Carro 1",
          cantidadReservas: 3,
          minutosReservados: 150,
        },
      ],
    })
    vi.mocked(adminApi.reporteUsoDocentes).mockResolvedValue({
      data: [
        {
          usuarioId: "u1",
          nombreDocente: "Ada Lovelace",
          cantidadReservas: 2,
          minutosReservados: 90,
        },
      ],
    })
    vi.mocked(adminApi.reporteIncidenciasPorEquipo).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteIncidenciasPorCarro).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteEquiposFueraDeCirculacion).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteIncidenciasPorCategoria).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoEquipos).mockResolvedValue({
      data: [
        {
          id: "h1",
          anio: 2025,
          equipoId: "pc1",
          etiquetaSnapshot: "PC 4",
          identificadorSnapshot: 4,
          carroNombreSnapshot: "Carro viejo",
          minutosReservados: 300,
          cantidadReservas: 5,
        },
      ],
    })
    vi.mocked(adminApi.historicoUsoDocentes).mockResolvedValue({
      data: [
        {
          id: "hd1",
          anio: 2025,
          usuarioId: "u1",
          nombreDocenteSnapshot: "Grace Hopper",
          cantidadReservas: 4,
          minutosTotales: 240,
        },
      ],
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // El ciclo activo es el default: RF-06.1 habla del "ciclo lectivo activo".
  it("arranca reportando sobre el ciclo activo", async () => {
    renderPagina()

    expect(await screen.findByText("PC 7")).toBeInTheDocument()
    expect(adminApi.reporteUsoEquipos).toHaveBeenCalledWith(
      "c-activo",
      undefined,
      undefined
    )
  })

  // El reporte devolvía solo UUIDs: sin identificador ni carro no se puede
  // leer quién es quién.
  it("muestra la etiqueta y el carro, no el UUID del equipo", async () => {
    renderPagina()

    expect(await screen.findByText("PC 7")).toBeInTheDocument()
    expect(screen.getByText("Carro 1")).toBeInTheDocument()
    expect(screen.queryByText("pc1")).not.toBeInTheDocument()
  })

  it("muestra el nombre del docente y el tiempo en horas", async () => {
    renderPagina()

    expect(await screen.findByText("Ada Lovelace")).toBeInTheDocument()
    expect(screen.getByText("2h 30min")).toBeInTheDocument() // 150 min del equipo
    expect(screen.getByText("1h 30min")).toBeInTheDocument() // 90 min del docente
  })

  // RF-06.1: "filtrable por rango de fechas".
  it("filtrar por fechas vuelve a consultar con el rango", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText("PC 7")

    await user.type(screen.getByLabelText("Desde"), "2026-03-01")

    expect(adminApi.reporteUsoEquipos).toHaveBeenLastCalledWith(
      "c-activo",
      "2026-03-01",
      undefined
    )
  })

  it("cambiar de ciclo vuelve a consultar ese ciclo", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText("PC 7")

    await user.selectOptions(screen.getByLabelText("Ciclo lectivo"), "c-viejo")

    expect(adminApi.reporteUsoEquipos).toHaveBeenLastCalledWith(
      "c-viejo",
      undefined,
      undefined
    )
  })

  // RF-06.3: las incidencias no dependen del ciclo, así que se consultan
  // sin cicloId.
  it("las incidencias se consultan sin ciclo", async () => {
    renderPagina()
    await screen.findByText("PC 7")

    expect(adminApi.reporteIncidenciasPorEquipo).toHaveBeenCalledWith(
      undefined,
      undefined
    )
    expect(adminApi.reporteIncidenciasPorCarro).toHaveBeenCalledWith(undefined, undefined)
  })

  // Un ciclo archivado no tiene reservas (RF-02.4 las borra), así que el
  // vacío tiene que explicarse en vez de parecer un error.
  it("explica el vacío de un ciclo archivado", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockResolvedValue({ data: [] })
    renderPagina()

    expect(await screen.findByText(/en el histórico por año/)).toBeInTheDocument()
  })

  // ── RF-06.4: histórico por año ──────────────────────────────────────

  /**
   * El snapshot se guarda bajo el año y se calcula al archivar (RF-02.4), así
   * que los años consultables son exactamente los de los ciclos archivados.
   */
  it("solo ofrece los años de ciclos archivados", async () => {
    renderPagina()

    const selector = (await screen.findByLabelText("Año")) as HTMLSelectElement
    expect([...selector.options].map((o) => o.value)).toEqual(["2025"])
  })

  it("arranca en el año archivado más reciente", async () => {
    renderPagina()

    await screen.findByLabelText("Año")
    expect(adminApi.historicoUsoEquipos).toHaveBeenCalledWith(2025)
    expect(adminApi.historicoUsoDocentes).toHaveBeenCalledWith(2025)
  })

  // Los nombres son snapshots: el equipo pudo mudarse de carro (RF-03.10) o
  // darse de baja, y el histórico tiene que seguir diciendo dónde estaba.
  it("muestra la etiqueta y el carro que el equipo tenía ese año", async () => {
    renderPagina()

    expect(await screen.findByText("PC 4")).toBeInTheDocument()
    expect(screen.getByText("Carro viejo")).toBeInTheDocument()
    expect(screen.getByText("5h")).toBeInTheDocument() // 300 min
  })

  it("muestra el uso por docente del año", async () => {
    renderPagina()

    expect(await screen.findByText("Grace Hopper")).toBeInTheDocument()
    expect(screen.getByText("4h")).toBeInTheDocument() // 240 min
  })

  /**
   * `usuario_id` quedó en ON DELETE SET NULL, así que borrar la cuenta
   * (RF-01.9) no se lleva el snapshot: el nombre sobrevive sin a quién
   * apuntar, y la fila tiene que seguir apareciendo.
   */
  it("muestra al docente cuya cuenta se eliminó", async () => {
    vi.mocked(adminApi.historicoUsoDocentes).mockResolvedValue({
      data: [
        {
          id: "hd1",
          anio: 2025,
          nombreDocenteSnapshot: "Alan Turing",
          cantidadReservas: 1,
          minutosTotales: 60,
        },
      ],
    })
    renderPagina()

    expect(await screen.findByText("Alan Turing")).toBeInTheDocument()
    expect(screen.getByText("Cuenta eliminada")).toBeInTheDocument()
  })

  it("cambiar de año vuelve a consultar ese año", async () => {
    vi.mocked(adminApi.listarCiclos).mockResolvedValue({
      data: [
        { id: "c-2024", anio: 2024, activo: false, archivado: true },
        { id: "c-2025", anio: 2025, activo: false, archivado: true },
        { id: "c-activo", anio: 2026, activo: true, archivado: false },
      ],
    })
    const user = userEvent.setup()
    renderPagina()

    await user.selectOptions(await screen.findByLabelText("Año"), "2024")

    expect(adminApi.historicoUsoEquipos).toHaveBeenLastCalledWith(2024)
  })

  // Un absoluto suelto no se puede juzgar: "150 minutos" no dice si ese
  // equipo se usó mucho o poco.
  it("muestra el total de cada tabla como contexto de las filas", async () => {
    renderPagina()

    expect(await screen.findByText(/1 equipo usado/)).toBeInTheDocument()
    expect(screen.getByText(/2h 30min en total/)).toBeInTheDocument()
  })

  it("muestra qué parte del total representa cada fila", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          identificador: 7,
          carroNombre: "Carro 1",
          cantidadReservas: 3,
          minutosReservados: 150,
        },
        {
          equipoId: "pc2",
          etiqueta: "PC 8",
          identificador: 8,
          carroNombre: "Carro 1",
          cantidadReservas: 1,
          minutosReservados: 50,
        },
      ],
    })
    renderPagina()

    expect(await screen.findByText("75%")).toBeInTheDocument()
    expect(screen.getByText("25%")).toBeInTheDocument()
  })

  // Lo que se busca en un reporte de uso es "cuál se usa más". Con las filas
  // en el orden en que las devolvió la base hay que leerlas todas.
  it("ordena las filas de mayor a menor uso", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          identificador: 7,
          carroNombre: "Carro 1",
          cantidadReservas: 1,
          minutosReservados: 50,
        },
        {
          equipoId: "pc2",
          etiqueta: "PC 8",
          identificador: 8,
          carroNombre: "Carro 1",
          cantidadReservas: 3,
          minutosReservados: 150,
        },
      ],
    })
    renderPagina()

    await screen.findByText("PC 8")
    const filas = screen.getAllByRole("row").map((f) => f.textContent)
    const primera = filas.findIndex((t) => t?.includes("PC 8"))
    const segunda = filas.findIndex((t) => t?.includes("PC 7"))
    expect(primera).toBeLessThan(segunda)
  })

  // Un reporte que no se puede sacar del sistema sale igual, en una captura
  // de pantalla pegada en un correo.
  it("permite descargar cada tabla como CSV", async () => {
    renderPagina()

    // `waitFor` sobre el total y no un `findAllByRole` suelto: ese resuelve
    // apenas encuentra el primero, y las dos tablas del histórico viven en un
    // componente aparte que dispara sus consultas un instante después.
    await waitFor(() => {
      // Uso por equipo, uso por docente y las dos del histórico. Las de
      // incidencias no aparecen: en este escenario vienen vacías.
      expect(screen.getAllByRole("button", { name: /Descargar CSV/ })).toHaveLength(4)
    })
  })

  it("no ofrece descargar una tabla vacía", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteUsoDocentes).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoEquipos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoDocentes).mockResolvedValue({ data: [] })
    renderPagina()

    await screen.findByText(/No hay reservas en ese ciclo/)
    expect(
      screen.queryByRole("button", { name: /Descargar CSV/ })
    ).not.toBeInTheDocument()
  })

  // Sin ningún ciclo archivado no hay nada que consultar: no tiene sentido
  // mostrar un selector vacío ni pegarle al backend con un año inventado.
  it("no consulta el histórico si todavía no se archivó ningún ciclo", async () => {
    vi.mocked(adminApi.listarCiclos).mockResolvedValue({
      data: [{ id: "c-activo", anio: 2026, activo: true, archivado: false }],
    })
    renderPagina()

    expect(
      await screen.findByText(/Todavía no se archivó ningún ciclo/)
    ).toBeInTheDocument()
    expect(screen.queryByLabelText("Año")).not.toBeInTheDocument()
    expect(adminApi.historicoUsoEquipos).not.toHaveBeenCalled()
  })

  // ── RF-06.5: el estado del parque ───────────────────────────────────

  /** El número que se lleva a pedir presupuesto. */
  it("dice cuántos equipos hay y cuántos están fuera de circulación", async () => {
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({
      data: [
        {
          carroId: "c1",
          carroNombre: "Carro 1",
          disponibles: 20,
          enMantenimiento: 6,
          fueraDeServicio: 4,
          total: 30,
        },
      ],
    })
    renderPagina()

    expect(
      await screen.findByText(/20 de 30 disponibles · 10 fuera de circulación/)
    ).toBeInTheDocument()
  })

  /** La lista que se manda al service. */
  it("lista los equipos fuera de circulación con su falla", async () => {
    vi.mocked(adminApi.reporteEquiposFueraDeCirculacion).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          carroNombre: "Carro 1",
          estado: "EN_MANTENIMIENTO",
          categoria: "batería",
          ultimaFalla: "no carga",
        },
      ],
    })
    renderPagina()

    expect(await screen.findByText("PC 7")).toBeInTheDocument()
    expect(screen.getByText("batería")).toBeInTheDocument()
    expect(screen.getByText("En mantenimiento")).toBeInTheDocument()
  })

  /**
   * Una máquina se puede sacar de circulación sin que nadie escriba qué
   * tiene, y eso hay que verlo: es trabajo pendiente de diagnóstico, no una
   * fila para esconder.
   */
  it("marca las que nadie pudo diagnosticar", async () => {
    vi.mocked(adminApi.reporteEquiposFueraDeCirculacion).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          estado: "FUERA_DE_SERVICIO",
        },
      ],
    })
    renderPagina()

    expect(await screen.findAllByText("Sin diagnosticar")).not.toHaveLength(0)
  })

  it("agrupa las incidencias por tipo de falla", async () => {
    vi.mocked(adminApi.reporteIncidenciasPorCategoria).mockResolvedValue({
      data: [
        { categoria: "batería", total: 12, abiertas: 8, equiposAlcanzados: 12 },
        { categoria: "pantalla", total: 3, abiertas: 1, equiposAlcanzados: 2 },
      ],
    })
    renderPagina()

    expect(
      await screen.findByText(/15 incidencias en 2 tipos de falla/)
    ).toBeInTheDocument()
    expect(screen.getByText("batería")).toBeInTheDocument()
  })

  /**
   * El backend devuelve el desglose por estado desde siempre y la tabla lo
   * tiraba.
   */
  it("desglosa las incidencias por estado, no solo el total", async () => {
    vi.mocked(adminApi.reporteIncidenciasPorEquipo).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          identificador: 7,
          carroNombre: "Carro 1",
          total: 12,
          abiertas: 3,
          enReparacion: 2,
          enviadasASoporte: 1,
          resueltas: 6,
          graves: 4,
        },
      ],
    })
    renderPagina()

    // Por celda y no con toHaveTextContent: buscar "2" en la fila entera lo
    // encuentra adentro de "12" y el test pasaría sin dibujar nada.
    const fila = (await screen.findByText("PC 7")).closest("tr")!
    const celdas = [...fila.querySelectorAll("td")].map((c) => c.textContent)

    expect(celdas).toEqual([
      "PC 7",
      "Carro 1",
      "12",
      "3",
      // Los tres que antes se calculaban y no llegaban a la pantalla.
      "2",
      "1",
      "6",
      "4",
      expect.stringContaining("%"),
    ])
  })
  it("no anuncia ausencia de reservas ni equipos disponibles durante la carga", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockReturnValue(new Promise(() => {}))
    vi.mocked(adminApi.reporteEquiposFueraDeCirculacion).mockReturnValue(
      new Promise(() => {})
    )
    renderPagina()
    await screen.findByText("Consultando uso por equipo…")
    expect(screen.queryByText(/No hay reservas en ese ciclo/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Están todos disponibles/)).not.toBeInTheDocument()
    expect(
      screen.getByText("Consultando equipos fuera de circulación…")
    ).toBeInTheDocument()
  })

  it("un fallo parcial tiene reintento y no oculta los reportes válidos", async () => {
    vi.mocked(adminApi.reporteUsoEquipos).mockRejectedValueOnce(new Error("Caída"))
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/No pudimos consultar uso por equipo/)
    expect(
      screen.getByRole("table", { name: "Horas reservadas por docente" })
    ).toBeInTheDocument()
    expect(screen.queryByText(/No hay reservas en ese ciclo/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Descargar CSV: Uso por equipo" })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await screen.findByRole("table", { name: "Uso por equipo" })
  })

  it("un refresco fallido no permite exportar los datos anteriores", async () => {
    const qc = renderPagina()
    await screen.findByRole("table", { name: "Uso por equipo" })
    vi.mocked(adminApi.reporteUsoEquipos).mockRejectedValueOnce(
      new Error("Falló el refresco")
    )
    await act(async () => {
      await qc.invalidateQueries({ queryKey: ["reporte", "uso-equipos"] })
    })
    await screen.findByText(/No pudimos consultar uso por equipo/)
    expect(
      screen.queryByRole("table", { name: "Uso por equipo" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Descargar CSV: Uso por equipo" })
    ).not.toBeInTheDocument()
  })

  it("bloquea la descarga mientras comprueba una actualización", async () => {
    const qc = renderPagina()
    await screen.findByRole("button", { name: "Descargar CSV: Uso por equipo" })
    let resolver!: (v: Awaited<ReturnType<typeof adminApi.reporteUsoEquipos>>) => void
    vi.mocked(adminApi.reporteUsoEquipos).mockReturnValueOnce(
      new Promise((r) => {
        resolver = r
      })
    )
    let pendiente!: Promise<void>
    await act(async () => {
      pendiente = qc.invalidateQueries({ queryKey: ["reporte", "uso-equipos"] })
    })
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Descargar CSV: Uso por equipo" })
      ).toBeDisabled()
    )
    await act(async () => {
      resolver({ data: [] })
      await pendiente
    })
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Descargar CSV: Uso por equipo" })
      ).not.toBeInTheDocument()
    )
  })

  it("el fallo de ciclos conserva las incidencias y no anuncia un histórico inexistente", async () => {
    vi.mocked(adminApi.listarCiclos).mockRejectedValueOnce(new Error("Sin ciclos"))
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/No pudimos consultar los ciclos lectivos/)
    expect(screen.getByLabelText("Ciclo lectivo")).toBeDisabled()
    expect(adminApi.reporteUsoEquipos).not.toHaveBeenCalled()
    expect(
      screen.queryByText(/Todavía no se archivó ningún ciclo/)
    ).not.toBeInTheDocument()
    expect(adminApi.reporteIncidenciasPorEquipo).toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: "Reintentar" }))
    await screen.findByRole("table", { name: "Uso por equipo" })
  })

  it("un rango invertido bloquea los reportes del período y deja visible el estado actual", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByRole("table", { name: "Uso por equipo" })
    await user.type(screen.getByLabelText("Desde"), "2026-08-05")
    const cantidad = vi.mocked(adminApi.reporteUsoEquipos).mock.calls.length
    await user.type(screen.getByLabelText("Hasta"), "2026-08-04")
    await screen.findAllByText("La fecha Desde no puede ser posterior a Hasta.")
    expect(adminApi.reporteUsoEquipos).toHaveBeenCalledTimes(cantidad)
    expect(
      screen.queryByRole("table", { name: "Uso por equipo" })
    ).not.toBeInTheDocument()
    expect(screen.getByText(/No hay ningún equipo cargado todavía/)).toBeInTheDocument()
    await user.clear(screen.getByLabelText("Hasta"))
    await screen.findByRole("table", { name: "Uso por equipo" })
  })

  it("las exportaciones del estado actual no se etiquetan como un año o período histórico", async () => {
    const descargar = vi.spyOn(csv, "descargarCSV").mockImplementation(() => {})
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({
      data: [
        {
          carroId: "carro",
          carroNombre: "Carro",
          total: 1,
          disponibles: 1,
          enMantenimiento: 0,
          fueraDeServicio: 0,
        },
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await user.type(screen.getByLabelText("Desde"), "2025-02-01")
    await user.click(
      await screen.findByRole("button", {
        name: "Descargar CSV: Estado del parque de equipos",
      })
    )
    expect(descargar).toHaveBeenLastCalledWith(
      "estado-inventario_actual.csv",
      expect.any(Array)
    )
    await screen.findByText(
      /una clase de 10 equipos durante una hora suma 10 reservas y 10 horas de equipo/
    )
  })

  it("sin resolver incluye incidencias en reparación y enviadas a soporte", async () => {
    vi.mocked(adminApi.reporteIncidenciasPorEquipo).mockResolvedValue({
      data: [
        {
          equipoId: "pc1",
          etiqueta: "PC 7",
          identificador: 7,
          carroNombre: "Carro",
          total: 6,
          abiertas: 1,
          enReparacion: 2,
          enviadasASoporte: 1,
          resueltas: 2,
          graves: 1,
        },
      ],
    })
    renderPagina()
    await screen.findByText(/4 sin resolver/)
  })

  it("los reportes vacíos son válidos sin un ciclo creado, salvo las consultas de uso", async () => {
    vi.mocked(adminApi.listarCiclos).mockResolvedValue({ data: [] })
    renderPagina()
    await screen.findAllByText("Creá un ciclo lectivo para consultar su uso.")
    expect(adminApi.reporteUsoEquipos).not.toHaveBeenCalled()
    expect(adminApi.reporteIncidenciasPorEquipo).toHaveBeenCalled()
    expect(screen.getByLabelText("Ciclo lectivo")).toBeDisabled()
  })
})
