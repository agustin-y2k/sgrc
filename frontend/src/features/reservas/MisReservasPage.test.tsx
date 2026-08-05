import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes } from "react-router"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { MisReservasPage } from "@/features/reservas/MisReservasPage"
import * as reservasApi from "@/features/reservas/api"
import type { ReservaDetallada } from "@/features/reservas/types"
import { ApiError } from "@/lib/api-client"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")
vi.mock("@/features/auth/AuthContext")

const DOCENTE: Usuario = {
  id: "docente1",
  nombre: "Ada",
  apellido: "Lovelace",
  email: "ada@test.com",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

function mockUsuario(u: Usuario) {
  vi.mocked(useAuth).mockReturnValue({
    user: u,
    isLoading: false,
    errorDeSesion: null,
    login: vi.fn(),
    logout: vi.fn(),
    refetchUser: vi.fn(),
  })
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "r1",
    reservaGrupoId: "grupo1",
    pcId: "pc1",
    fecha: "2026-03-09",
    horaInicio: "08:00",
    horaFin: "09:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    creadoPor: "docente1",
    nombreDocenteSnapshot: "Ada Lovelace",
    pcIdentificador: 1,
    carroNombre: "Carro 1",
    materiaNombre: "Matemáticas",
    cursoNombre: "1°A",
    ...over,
  }
}

function renderPagina(estado?: { confirmacion: string }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[{ pathname: "/reservas", state: estado }]}>
        <Routes>
          <Route path="/reservas" element={<MisReservasPage />} />
          <Route path="/reservas/nueva" element={<div>Nueva</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe("MisReservasPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.cancelarReserva).mockResolvedValue(undefined)
    vi.mocked(reservasApi.cancelarGrupo).mockResolvedValue({ reservasCanceladas: 3 })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  // El otro extremo de lo que deja NuevaReservaPage al navegar acá: sin
  // esto, la única señal de que la reserva se creó era haber cambiado de
  // pantalla.
  it("muestra la confirmación con la que se llega desde Nueva reserva", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina({ confirmacion: "Reserva confirmada para el lunes 9 de marzo." })

    expect(
      await screen.findByText("Reserva confirmada para el lunes 9 de marzo.")
    ).toBeInTheDocument()
  })

  it("no inventa una confirmación si se entra directo al listado", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    await screen.findByText("Matemáticas — 1°A")
    expect(screen.queryByRole("status")).not.toBeInTheDocument()
  })

  it("lista las reservas con su materia, horario y estado", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
    renderPagina()

    expect(await screen.findByText("Matemáticas — 1°A")).toBeInTheDocument()
    expect(screen.getByText("Lunes, 9 de marzo")).toBeInTheDocument()
    expect(screen.getByText("Confirmada")).toBeInTheDocument()
  })

  // El glosario define ReservaGrupo como "la reserva tal como la percibe el
  // docente". Antes la API devolvía una fila por PC sin identificarla, así
  // que una reserva de varias PCs se veía como N tarjetas idénticas.
  describe("agrupación por reserva", () => {
    const tresPCs = [
      reserva({ id: "r1", pcId: "pc1", pcIdentificador: 3 }),
      reserva({ id: "r2", pcId: "pc2", pcIdentificador: 7 }),
      reserva({ id: "r3", pcId: "pc3", pcIdentificador: 12, carroNombre: "Carro 2" }),
    ]

    it("muestra una sola tarjeta con todas sus PCs", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada(tresPCs))
      renderPagina()

      expect(await screen.findByText("PC 3 · Carro 1")).toBeInTheDocument()
      expect(screen.getByText("PC 7 · Carro 1")).toBeInTheDocument()
      // RF-04.2: una reserva puede combinar PCs de carros distintos.
      expect(screen.getByText("PC 12 · Carro 2")).toBeInTheDocument()

      // Una sola tarjeta, no tres: un solo botón de cancelar.
      expect(screen.getAllByRole("button", { name: "Cancelar" })).toHaveLength(1)
    })

    it("separa las reservas de grupos distintos", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([
          reserva({ id: "r1", reservaGrupoId: "grupo1" }),
          reserva({ id: "r2", reservaGrupoId: "grupo2", fecha: "2026-03-10" }),
        ])
      )
      renderPagina()

      await screen.findByText("Lunes, 9 de marzo")
      expect(screen.getByText("Martes, 10 de marzo")).toBeInTheDocument()
      expect(screen.getAllByRole("button", { name: "Cancelar" })).toHaveLength(2)
    })

    // RF-04.7 / RF-03.8: una cascada puede cancelar algunas PCs y dejar el
    // resto en pie. El motivo de cada una explica por qué la reserva quedó
    // incompleta.
    it("muestra el motivo de cada PC cancelada dentro del grupo", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([
          reserva({ id: "r1", pcIdentificador: 3 }),
          reserva({
            id: "r2",
            pcIdentificador: 7,
            estado: "CANCELADA",
            motivoCancelacion: "la PC pasó a FUERA_DE_SERVICIO",
          }),
        ])
      )
      renderPagina()

      expect(
        await screen.findByText("PC 7: la PC pasó a FUERA_DE_SERVICIO")
      ).toBeInTheDocument()
      // El grupo sigue vivo porque queda una PC confirmada.
      expect(screen.getByText("Confirmada")).toBeInTheDocument()
      expect(screen.getByRole("button", { name: "Cancelar" })).toBeInTheDocument()
    })
  })

  describe("cancelación", () => {
    // Cancelar la propia no exige motivo (RF-04.8 solo lo pide para ajenas).
    it("cancelar una reserva propia sin motivo funciona", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      expect(reservasApi.cancelarGrupo).toHaveBeenCalledWith("grupo1", "", true)
    })

    // RF-04.8: un Admin cancelando la reserva de un docente debe dar motivo,
    // porque ese texto es el que recibe el docente en la notificación.
    it("un Admin no puede cancelar una reserva ajena sin motivo", async () => {
      mockUsuario({ ...DOCENTE, id: "admin1", rol: "ADMIN" })
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([reserva({ creadoPor: "otroDocente" })])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))

      expect(screen.getByRole("button", { name: "Confirmar cancelación" })).toBeDisabled()
      expect(screen.getByText(/el motivo es obligatorio/i)).toBeInTheDocument()
    })

    it("con motivo, el Admin sí puede cancelar la reserva ajena", async () => {
      mockUsuario({ ...DOCENTE, id: "admin1", rol: "ADMIN" })
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([reserva({ creadoPor: "otroDocente" })])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))
      await user.type(screen.getByLabelText(/Motivo/), "se necesita el laboratorio")
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      expect(reservasApi.cancelarGrupo).toHaveBeenCalledWith(
        "grupo1",
        "se necesita el laboratorio",
        true
      )
    })

    // RF-04.6: la elección se aplica "a todas las PCs del grupo en esa fecha
    // (o rango)". Antes "solo esta fecha" llamaba a cancelarReserva y
    // liberaba UNA sola PC, que no es lo que dice el requisito ni lo que
    // sugiere el texto de la opción.
    it("'solo esta fecha' cancela el grupo entero, no una PC suelta", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([
          reserva({ id: "r1", pcIdentificador: 3, reglaRecurrenciaId: "regla1" }),
          reserva({ id: "r2", pcIdentificador: 7, reglaRecurrenciaId: "regla1" }),
        ])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      expect(reservasApi.cancelarGrupo).toHaveBeenCalledWith("grupo1", "", true)
      expect(reservasApi.cancelarReserva).not.toHaveBeenCalled()
    })

    it("'esta y todas las siguientes' alcanza a las fechas futuras", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([reserva({ reglaRecurrenciaId: "regla1" })])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))
      await user.click(screen.getByLabelText("Esta fecha y todas las siguientes"))
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      expect(reservasApi.cancelarGrupo).toHaveBeenCalledWith("grupo1", "", false)
    })

    // El alcance de la serie solo tiene sentido si la reserva es recurrente.
    // Antes se usaba reservaGrupoId como proxy, que tienen TODAS las
    // reservas normales, así que la opción aparecía siempre.
    it("una reserva puntual no ofrece el alcance de la serie", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([reserva({ reglaRecurrenciaId: undefined })])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))

      expect(
        screen.queryByText("Esta fecha y todas las siguientes")
      ).not.toBeInTheDocument()
    })

    it("avisa cuántas PCs se van a cancelar", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([
          reserva({ id: "r1", pcIdentificador: 3 }),
          reserva({ id: "r2", pcIdentificador: 7 }),
        ])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Cancelar" }))

      expect(
        screen.getByText(/se cancelan las 2 PCs de esta reserva/i)
      ).toBeInTheDocument()
    })
  })

  it("una reserva cancelada muestra su motivo y no ofrece cancelarla de nuevo", async () => {
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva({ estado: "CANCELADA", motivoCancelacion: "PC fuera de servicio" }),
      ])
    )
    renderPagina()

    expect(await screen.findByText(/PC fuera de servicio/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Cancelar" })).not.toBeInTheDocument()
  })

  // Sin controles de página, las reservas más allá de las primeras 50
  // desaparecerían de la pantalla sin que nada lo diga.
  describe("paginación", () => {
    it("no muestra los controles si todo entra en una página", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
      renderPagina()

      await screen.findByText("Matemáticas — 1°A")
      expect(screen.queryByRole("button", { name: "Siguiente" })).not.toBeInTheDocument()
    })

    it("pide la página siguiente y muestra cuántas reservas hay en total", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([reserva()], { total: 120, page: 1, pageSize: 50 })
      )
      const user = userEvent.setup()
      renderPagina()

      expect(await screen.findByText(/de 120 reservas/)).toBeInTheDocument()
      expect(screen.getByText("Página 1 de 3")).toBeInTheDocument()
      expect(screen.getByRole("button", { name: "Anterior" })).toBeDisabled()

      await user.click(screen.getByRole("button", { name: "Siguiente" }))

      await waitFor(() => {
        expect(reservasApi.listarReservas).toHaveBeenLastCalledWith({
          incluirCanceladas: false,
          page: 2,
        })
      })
    })
  })

  // ── Bloqueos por evaluación (RF-04.7) ────────────────────────────────
  //
  // No tienen ReservaGrupo en la base —no son la reserva de nadie— pero para
  // el Admin que los creó fueron UNA operación: eligió varias PCs, una
  // fecha y un horario, y confirmó una vez.
  describe("bloqueos por evaluación", () => {
    // Los ve el Admin que los creó: al docente el backend le fuerza el
    // filtro por creador, así que nunca le aparecen.
    beforeEach(() => {
      mockUsuario({ ...DOCENTE, id: "admin1", rol: "ADMIN" })
    })

    const bloqueo = (id: string, pc: number): ReservaDetallada =>
      reserva({
        id,
        reservaGrupoId: undefined,
        tipo: "EVALUACION_ESTATAL",
        creadoPor: "admin1",
        materiaNombre: undefined,
        cursoNombre: undefined,
        nombreDocenteSnapshot: undefined,
        pcId: `pc${pc}`,
        pcIdentificador: pc,
      })

    it("junta las PCs de un mismo bloqueo en una sola tarjeta", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([bloqueo("b1", 1), bloqueo("b2", 2), bloqueo("b3", 3)])
      )
      renderPagina()

      // Una tarjeta, no tres: antes bloquear tres PCs se veía como tres
      // bloqueos distintos.
      expect(
        await screen.findByText(/Bloqueo por evaluación · 3 PCs/)
      ).toBeInTheDocument()
      expect(screen.getAllByRole("button", { name: "Levantar bloqueo" })).toHaveLength(1)
      expect(screen.getByText("PC 1 · Carro 1")).toBeInTheDocument()
      expect(screen.getByText("PC 3 · Carro 1")).toBeInTheDocument()
    })

    // El botón no hacía nada: el panel de confirmación exigía un grupoId que
    // un bloqueo no tiene, así que nunca se abría.
    it("levantar el bloqueo abre la confirmación", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([bloqueo("b1", 1), bloqueo("b2", 2)])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Levantar bloqueo" }))

      expect(screen.getByText(/Se liberan 2 PCs de este bloqueo/)).toBeInTheDocument()
      expect(
        screen.getByRole("button", { name: "Confirmar cancelación" })
      ).toBeInTheDocument()
    })

    // Liberar solo una PC dejaría el aula a medio bloquear sin que nada lo
    // diga: la tarjeta representa la operación completa.
    it("libera todas las PCs del bloqueo, no solo la primera", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([bloqueo("b1", 1), bloqueo("b2", 2), bloqueo("b3", 3)])
      )
      const user = userEvent.setup()
      renderPagina()

      await user.click(await screen.findByRole("button", { name: "Levantar bloqueo" }))
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      await waitFor(() => {
        expect(reservasApi.cancelarReserva).toHaveBeenCalledTimes(3)
      })
      for (const id of ["b1", "b2", "b3"]) {
        expect(reservasApi.cancelarReserva).toHaveBeenCalledWith(id, "")
      }
    })

    // Dos bloqueos distintos —otro horario— son dos tarjetas.
    it("no junta bloqueos de franjas distintas", async () => {
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(
        paginada([
          bloqueo("b1", 1),
          { ...bloqueo("b2", 2), horaInicio: "10:00", horaFin: "12:00" },
        ])
      )
      renderPagina()

      expect(await screen.findAllByText(/Bloqueo por evaluación · 1 PC/)).toHaveLength(2)
    })
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(reservasApi.listarReservas).mockRejectedValue(
      new ApiError(401, "token inválido o expirado")
    )
    renderPagina()

    expect(await screen.findByText("token inválido o expirado")).toBeInTheDocument()
  })
})
