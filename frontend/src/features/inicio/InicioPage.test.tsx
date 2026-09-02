import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"

import * as adminApi from "@/features/admin/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { InicioPage } from "@/features/inicio/InicioPage"
import * as inventoryApi from "@/features/inventory/api"
import type { Equipo } from "@/features/inventory/types"
import * as notificacionesApi from "@/features/notificaciones/api"
import * as reservasApi from "@/features/reservas/api"
import type { ReservaDetallada } from "@/features/reservas/types"
import { paginada } from "@/test/respuestas"

vi.mock("@/features/reservas/api")
vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")
vi.mock("@/features/notificaciones/api")
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

const ADMIN: Usuario = { ...DOCENTE, id: "admin1", nombre: "Grace", rol: "ADMIN" }

function mockUsuario(u: Usuario) {
  vi.mocked(useAuth).mockReturnValue({
    user: u,
    isLoading: false,
    errorDeSesion: null,
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser: vi.fn(),
  })
}

/** Hoy, como lo arma la propia página (fecha local, no UTC). */
function hoy(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function reserva(over: Partial<ReservaDetallada> = {}): ReservaDetallada {
  return {
    id: "r1",
    reservaGrupoId: "grupo1",
    equipoId: "pc1",
    fecha: hoy(),
    horaInicio: "14:00",
    horaFin: "15:00",
    estado: "CONFIRMADA",
    tipo: "NORMAL",
    identificador: 1,
    carroNombre: "Carro 1",
    materiaNombre: "Matemática",
    cursoNombre: "5°A",
    nombreDocenteSnapshot: "Ada Lovelace",
    creadoPor: "docente1",
    ...over,
  } as ReservaDetallada
}

function renderInicio() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <InicioPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

function equipo(over: Partial<Equipo> = {}): Equipo {
  return {
    id: "pc1",
    carroId: "c1",
    identificador: 1,
    etiqueta: "PC 1",
    tipo: "notebook",
    reservable: true,
    esComputadora: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

describe("InicioPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([]))
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue(paginada([]))
    vi.mocked(reservasApi.listarPrestamosAbiertos).mockResolvedValue({ data: [] })
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({ data: [] })
    vi.mocked(reservasApi.cancelarGrupo).mockResolvedValue({ reservasCanceladas: 1 })
    vi.mocked(reservasApi.equiposDisponibles).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarEquipos).mockResolvedValue({ data: [equipo()] })
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    vi.mocked(inventoryApi.listarCategoriasDeFalla).mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("muestra las próximas reservas agrupadas en una sola tarjeta por clase", async () => {
    mockUsuario(DOCENTE)
    // Dos equipos de la MISMA reserva: para el docente fue una sola clase.
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva(), reserva({ id: "r2", equipoId: "pc2", identificador: 2 })])
    )
    renderInicio()

    expect(await screen.findByText(/Matemática/)).toBeInTheDocument()
    expect(screen.getByText("2 computadoras reservadas")).toBeInTheDocument()
    expect(screen.getAllByText(/Matemática/)).toHaveLength(1)
  })

  /**
   * Un docente no lee un "1" grande arriba de la palabra "hoy" y sabe qué
   * hacer con eso: lee el día de su clase.
   */
  it("rotula la clase por el día, no por un contador", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([
        reserva(),
        reserva({ id: "r9", reservaGrupoId: "grupo9", fecha: "2099-12-31" }),
      ])
    )
    renderInicio()

    expect(await screen.findByText(/^Hoy/)).toBeInTheDocument()
    expect(screen.queryByText("clases hoy")).not.toBeInTheDocument()
    expect(screen.queryByText("próximas")).not.toBeInTheDocument()
  })

  it("no muestra las canceladas", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.listarReservas).mockResolvedValue(
      paginada([reserva({ estado: "CANCELADA" })])
    )
    renderInicio()

    expect(
      await screen.findByText(/No tenés ninguna clase con computadoras reservadas/)
    ).toBeInTheDocument()
  })

  /**
   * Con la consulta caída, el estado vacío es una afirmación falsa: el
   * docente cierra la pantalla creyendo que perdió la reserva.
   */
  it("si no pudo consultar las reservas, no dice que no hay ninguna", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(reservasApi.listarReservas).mockRejectedValue(new Error("se cayó la red"))
    renderInicio()

    expect(
      await screen.findByText(/No se pudieron consultar tus reservas/)
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/No tenés ninguna clase con computadoras reservadas/)
    ).not.toBeInTheDocument()
  })

  it("dice cuántos avisos sin leer hay, en una frase", async () => {
    mockUsuario(DOCENTE)
    vi.mocked(notificacionesApi.listarNotificaciones).mockResolvedValue(
      paginada([], { total: 3, pageSize: 50 })
    )
    renderInicio()

    expect(await screen.findByText("Tenés 3 avisos sin leer")).toBeInTheDocument()
  })

  it("a un docente no le pregunta por las cuentas pendientes", async () => {
    mockUsuario(DOCENTE)
    renderInicio()

    expect(await screen.findByText("Tus próximas clases")).toBeInTheDocument()
    // El endpoint es solo de Admin: pedirlo desde un docente sería un 403
    // en cada carga del home.
    expect(adminApi.listarUsuarios).not.toHaveBeenCalled()
    expect(screen.queryByText("por aprobar")).not.toBeInTheDocument()
  })

  /**
   * Lo que sigue es la razón de ser de la pantalla para un docente: las tres
   * cosas que puede necesitar hacer se hacen acá, sin tener que saber en qué
   * sección del sistema vive cada una.
   */
  describe("resolver sin salir del inicio", () => {
    it("cancela una clase", async () => {
      mockUsuario(DOCENTE)
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
      const user = userEvent.setup()
      renderInicio()

      await user.click(await screen.findByRole("button", { name: "Cancelar esta clase" }))
      await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }))

      expect(reservasApi.cancelarGrupo).toHaveBeenCalledWith("grupo1", "", true)
    })

    it("cambia una computadora de una reserva ya hecha", async () => {
      mockUsuario(DOCENTE)
      vi.mocked(reservasApi.listarReservas).mockResolvedValue(paginada([reserva()]))
      const user = userEvent.setup()
      renderInicio()

      await user.click(
        await screen.findByRole("button", { name: "Cambiar una computadora" })
      )

      expect(await screen.findByLabelText("¿Cuál cambiás?")).toBeInTheDocument()
      expect(reservasApi.equiposDisponibles).toHaveBeenCalled()
    })

    it("reporta que una computadora no anda", async () => {
      mockUsuario(DOCENTE)
      const user = userEvent.setup()
      renderInicio()

      await user.click(await screen.findByText("Avisar que una no anda"))
      await user.selectOptions(await screen.findByLabelText("¿Cuál es?"), "pc1")

      // El formulario de reporte es el mismo del inventario, y nombra el
      // equipo por su etiqueta.
      expect(await screen.findByText("Reportar un problema en PC 1")).toBeInTheDocument()
    })
  })

  /** "Inventario", "Disponibilidad": nombres de secciones del sistema. */
  it("nombra los atajos por lo que se hace, no por la sección", async () => {
    mockUsuario(DOCENTE)
    renderInicio()

    expect(await screen.findByText("Ver las computadoras")).toBeInTheDocument()
    expect(screen.getByText("Quién te puede ayudar")).toBeInTheDocument()
    // "Mi perfil" reemplazó al atajo de la contraseña: esa quedó adentro,
    // junto con la foto y las materias.
    expect(screen.getByText("Mi perfil")).toBeInTheDocument()
    // "Pedir ayuda" reemplazó a "Escribirnos": es lo que la persona necesita
    // hacer, y el mismo lugar sirve para contar que algo no anda.
    expect(screen.getByText("Pedir ayuda")).toBeInTheDocument()
    expect(screen.queryByText("Accesos rápidos")).not.toBeInTheDocument()
  })

  it("a un Admin le muestra cuántas cuentas esperan aprobación", async () => {
    mockUsuario(ADMIN)
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue(
      paginada([], { total: 3, pageSize: 50 })
    )
    renderInicio()

    // findByText del número y no del rótulo: el rótulo ya está en pantalla
    // mientras la consulta viaja, así que buscarlo a él pasa de largo y lee
    // el contador todavía en cero.
    const pendientes = (await screen.findByText("3")).closest("a")
    expect(pendientes).toHaveTextContent("por aprobar")
    expect(pendientes).toHaveAttribute("href", "/admin/aprobacion")
  })

  /**
   * El fallo más caro de esta pantalla no es quedarse en blanco: es afirmar
   * que no hay nada.
   */
  describe("cuando una consulta falla", () => {
    it("no muestra cero: muestra que no pudo preguntar", async () => {
      mockUsuario(ADMIN)
      vi.mocked(reservasApi.listarPrestamosAbiertos).mockRejectedValue(
        new Error("se cayó la red")
      )
      renderInicio()

      expect(
        await screen.findByText(/lo que ves acá puede estar incompleto/i)
      ).toBeInTheDocument()

      const afuera = screen.getByText("afuera").closest("a")
      expect(afuera).toHaveTextContent("—")
      expect(afuera).not.toHaveTextContent("0")
    })

    it("el aviso no aparece cuando todo respondió bien", async () => {
      mockUsuario(ADMIN)
      renderInicio()

      await screen.findByText("afuera")
      expect(
        screen.queryByText(/lo que ves acá puede estar incompleto/i)
      ).not.toBeInTheDocument()
    })

    // Que falle lo del mostrador no puede llevarse puesto el resto: las
    // reservas se consultan aparte y su número sigue siendo confiable.
    it("los demás indicadores siguen mostrando lo suyo", async () => {
      mockUsuario(ADMIN)
      vi.mocked(reservasApi.listarPrestamosAbiertos).mockRejectedValue(
        new Error("se cayó la red")
      )
      vi.mocked(adminApi.listarUsuarios).mockResolvedValue(
        paginada([], { total: 3, pageSize: 50 })
      )
      renderInicio()

      const pendientes = (await screen.findByText("3")).closest("a")
      expect(pendientes).toHaveTextContent("por aprobar")
    })
  })

  /**
   * El título viejo decía "Ahora en el laboratorio" sobre una lista de clases
   * en curso — justo las máquinas que se están yendo.
   */
  it("el panel del mostrador habla de la entrega, no de dónde se da la clase", async () => {
    mockUsuario(ADMIN)
    renderInicio()

    expect(await screen.findByText("Para entregar ahora")).toBeInTheDocument()
    expect(screen.queryByText("Ahora en el laboratorio")).not.toBeInTheDocument()
  })

  // "Afuera del laboratorio" ya estaba; faltaba la pregunta dada vuelta.
  it("a un Admin le dice con cuántos equipos cuenta acá", async () => {
    mockUsuario(ADMIN)
    vi.mocked(adminApi.reporteEstadoDelInventario).mockResolvedValue({
      data: [
        {
          carroId: "c1",
          carroNombre: "Carro 1",
          disponibles: 13,
          enMantenimiento: 0,
          fueraDeServicio: 0,
          total: 13,
        },
      ],
    })
    renderInicio()

    expect(await screen.findByText("13 de 13 equipos")).toBeInTheDocument()
  })

  // Es información de mostrador: un docente no opera entregas.
  it("a un docente no le pregunta por el estado del inventario", async () => {
    mockUsuario(DOCENTE)
    renderInicio()

    await screen.findByText("Tus próximas clases")
    expect(adminApi.reporteEstadoDelInventario).not.toHaveBeenCalled()
  })

  /**
   * El orden es la funcionalidad: entregar y recibir se opera todo el día con
   * gente esperando, y los contadores se miran una vez.
   */
  it("al Admin le pone el mostrador antes que los contadores", async () => {
    mockUsuario(ADMIN)
    renderInicio()

    const mostrador = await screen.findByText("Para entregar ahora")
    const contador = screen.getByText("por aprobar")
    expect(
      mostrador.compareDocumentPosition(contador) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  // Bajarlos no es sacarlos: una cuenta pendiente es un docente que no puede
  // trabajar, y nadie la va a buscar si ninguna pantalla la nombra.
  it("los contadores siguen estando, solo que abajo", async () => {
    mockUsuario(ADMIN)
    renderInicio()

    expect(await screen.findByText("por aprobar")).toBeInTheDocument()
    expect(screen.getByText("sin leer")).toBeInTheDocument()
  })
})
