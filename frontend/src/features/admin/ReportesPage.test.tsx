import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { ReportesPage } from "@/features/admin/ReportesPage"
import * as adminApi from "@/features/admin/api"
import { formatearDuracion } from "@/features/admin/types"

vi.mock("@/features/admin/api")

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <ReportesPage />
    </QueryClientProvider>
  )
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
    vi.mocked(adminApi.reporteUsoPCs).mockResolvedValue({
      data: [
        {
          pcId: "pc1",
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
    vi.mocked(adminApi.reporteIncidenciasPorPC).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteIncidenciasPorCarro).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoPCs).mockResolvedValue({
      data: [
        {
          id: "h1",
          anio: 2025,
          pcId: "pc1",
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
    expect(adminApi.reporteUsoPCs).toHaveBeenCalledWith("c-activo", undefined, undefined)
  })

  // El reporte devolvía solo UUIDs: sin identificador ni carro no se puede
  // leer quién es quién.
  it("muestra identificador y carro, no el UUID de la PC", async () => {
    renderPagina()

    expect(await screen.findByText("PC 7")).toBeInTheDocument()
    expect(screen.getByText("Carro 1")).toBeInTheDocument()
    expect(screen.queryByText("pc1")).not.toBeInTheDocument()
  })

  it("muestra el nombre del docente y el tiempo en horas", async () => {
    renderPagina()

    expect(await screen.findByText("Ada Lovelace")).toBeInTheDocument()
    expect(screen.getByText("2h 30min")).toBeInTheDocument() // 150 min de la PC
    expect(screen.getByText("1h 30min")).toBeInTheDocument() // 90 min del docente
  })

  // RF-06.1: "filtrable por rango de fechas".
  it("filtrar por fechas vuelve a consultar con el rango", async () => {
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText("PC 7")

    await user.type(screen.getByLabelText("Desde"), "2026-03-01")

    expect(adminApi.reporteUsoPCs).toHaveBeenLastCalledWith(
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

    expect(adminApi.reporteUsoPCs).toHaveBeenLastCalledWith(
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

    expect(adminApi.reporteIncidenciasPorPC).toHaveBeenCalledWith(undefined, undefined)
    expect(adminApi.reporteIncidenciasPorCarro).toHaveBeenCalledWith(undefined, undefined)
  })

  // Un ciclo archivado no tiene reservas (RF-02.4 las borra), así que el
  // vacío tiene que explicarse en vez de parecer un error.
  it("explica el vacío de un ciclo archivado", async () => {
    vi.mocked(adminApi.reporteUsoPCs).mockResolvedValue({ data: [] })
    renderPagina()

    expect(await screen.findByText(/en el histórico por año/)).toBeInTheDocument()
  })

  // ── RF-06.4: histórico por año ──────────────────────────────────────

  /**
   * El snapshot se guarda bajo el año y se calcula al archivar (RF-02.4),
   * así que los años consultables son exactamente los de los ciclos
   * archivados. Consultar el año del ciclo activo devolvería siempre vacío.
   */
  it("solo ofrece los años de ciclos archivados", async () => {
    renderPagina()

    const selector = (await screen.findByLabelText("Año")) as HTMLSelectElement
    expect([...selector.options].map((o) => o.value)).toEqual(["2025"])
  })

  it("arranca en el año archivado más reciente", async () => {
    renderPagina()

    await screen.findByLabelText("Año")
    expect(adminApi.historicoUsoPCs).toHaveBeenCalledWith(2025)
    expect(adminApi.historicoUsoDocentes).toHaveBeenCalledWith(2025)
  })

  // Los nombres son snapshots: la PC pudo mudarse de carro (RF-03.10) o
  // darse de baja, y el histórico tiene que seguir diciendo dónde estaba.
  it("muestra el identificador y el carro que la PC tenía ese año", async () => {
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

    expect(adminApi.historicoUsoPCs).toHaveBeenLastCalledWith(2024)
  })

  // Un absoluto suelto no se puede juzgar: "150 minutos" no dice si esa PC
  // se usó mucho o poco. El total de la tabla es el denominador que hace
  // legible cada fila.
  it("muestra el total de cada tabla como contexto de las filas", async () => {
    renderPagina()

    expect(await screen.findByText(/1 PC usada/)).toBeInTheDocument()
    expect(screen.getByText(/2h 30min en total/)).toBeInTheDocument()
  })

  it("muestra qué parte del total representa cada fila", async () => {
    vi.mocked(adminApi.reporteUsoPCs).mockResolvedValue({
      data: [
        {
          pcId: "pc1",
          identificador: 7,
          carroNombre: "Carro 1",
          cantidadReservas: 3,
          minutosReservados: 150,
        },
        {
          pcId: "pc2",
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
    vi.mocked(adminApi.reporteUsoPCs).mockResolvedValue({
      data: [
        {
          pcId: "pc1",
          identificador: 7,
          carroNombre: "Carro 1",
          cantidadReservas: 1,
          minutosReservados: 50,
        },
        {
          pcId: "pc2",
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
    // apenas encuentra el primero, y las dos tablas del histórico viven en
    // un componente aparte que dispara sus consultas un instante después.
    // Sin esperar el total, el test contaba solo las dos del ciclo activo.
    await waitFor(() => {
      // Uso por PC, uso por docente y las dos del histórico. Las de
      // incidencias no aparecen: en este escenario vienen vacías.
      expect(screen.getAllByRole("button", { name: "Descargar CSV" })).toHaveLength(4)
    })
  })

  it("no ofrece descargar una tabla vacía", async () => {
    vi.mocked(adminApi.reporteUsoPCs).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteUsoDocentes).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoPCs).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.historicoUsoDocentes).mockResolvedValue({ data: [] })
    renderPagina()

    await screen.findByText(/No hay reservas en ese ciclo/)
    expect(
      screen.queryByRole("button", { name: "Descargar CSV" })
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
    expect(adminApi.historicoUsoPCs).not.toHaveBeenCalled()
  })
})
