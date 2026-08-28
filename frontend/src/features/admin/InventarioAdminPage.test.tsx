import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { InventarioAdminPage } from "@/features/admin/InventarioAdminPage"
import * as adminApi from "@/features/admin/api"
import * as inventoryApi from "@/features/inventory/api"
import { useAuth } from "@/features/auth/AuthContext"
import type { Incidencia, Equipo } from "@/features/inventory/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")
vi.mock("@/features/auth/AuthContext")

function equipo(over: Partial<Equipo> = {}): Equipo {
  // La etiqueta la resuelve el backend a partir del identificador; acá se
  // deriva del override para que no queden inconsistentes.
  const identificador = over.identificador ?? 1
  return {
    id: "pc1",
    carroId: "c1",
    identificador: 1,
    numeroSerie: "5CD1234ABC",
    etiqueta: `PC ${identificador}`,
    tipo: "PC",
    reservable: true,
    freezado: false,
    estado: "DISPONIBLE",
    dadoDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

function incidencia(over: Partial<Incidencia> = {}): Incidencia {
  return {
    id: "i1",
    equipoId: "pc1",
    descripcion: "No arranca",
    gravedad: "GRAVE",
    fecha: "2026-08-01T10:00:00Z",
    enviadoASoporte: false,
    estado: "ABIERTA",
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <InventarioAdminPage />
    </QueryClientProvider>
  )
}

async function abrirCarro(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Gestionar equipos" }))
}

describe("InventarioAdminPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({ data: [equipo()] })
    vi.mocked(adminApi.cambiarEstadoEquipo).mockResolvedValue({
      reservasCanceladas: 0,
      docentesNotificados: 0,
    })
    vi.mocked(adminApi.darDeBajaEquipo).mockResolvedValue({
      reservasCanceladas: 0,
      docentesNotificados: 0,
    })
    vi.mocked(adminApi.crearCarro).mockResolvedValue({ id: "c2", nombre: "Carro 2" })
    vi.mocked(adminApi.editarCarro).mockResolvedValue(undefined)
    vi.mocked(adminApi.crearEquipoDeCarro).mockResolvedValue(
      equipo({ id: "pc9", identificador: 9 })
    )
    vi.mocked(adminApi.editarEquipo).mockResolvedValue(undefined)
    vi.mocked(adminApi.editarIncidencia).mockResolvedValue(undefined)
    vi.mocked(inventoryApi.listarIncidenciasDeEquipo).mockResolvedValue({
      data: [incidencia()],
    })
    vi.mocked(useAuth).mockReturnValue({
      user: { id: "u1", rol: "ADMIN" },
      isLoading: false,
    } as unknown as ReturnType<typeof useAuth>)
    vi.mocked(inventoryApi.listarCuentasDeEquipo).mockResolvedValue({ data: [] })
    vi.mocked(inventoryApi.listarClasesDeCuenta).mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("crea un carro", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Nombre"), "Carro 2")
    await user.click(screen.getByRole("button", { name: "Crear carro" }))

    expect(adminApi.crearCarro).toHaveBeenCalledWith({
      nombre: "Carro 2",
      descripcion: undefined,
    })
  })

  // RF-03.8: sacar un equipo de DISPONIBLE cancela sus reservas futuras y NO
  // se restauran al volver. Un solo click no puede alcanzar.
  it("cambiar a fuera de servicio pide confirmación y avisa de la cascada", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))

    expect(adminApi.cambiarEstadoEquipo).not.toHaveBeenCalled()
    expect(screen.getByText(/cancela todas sus reservas futuras/)).toBeInTheDocument()
    expect(screen.getByText(/no se restauran solas/)).toBeInTheDocument()
  })

  it("confirmado, manda el estado nuevo y el motivo", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))
    await user.type(screen.getByLabelText(/Motivo/), "no arranca")
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(adminApi.cambiarEstadoEquipo).toHaveBeenCalledWith(
      "pc1",
      "FUERA_DE_SERVICIO",
      "no arranca"
    )
  })

  // El motivo es opcional: si no se escribe, el backend arma uno por
  // defecto para la notificación (RF-03.8).
  it("sin motivo manda undefined en vez de string vacío", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /En mantenimiento/i }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(adminApi.cambiarEstadoEquipo).toHaveBeenCalledWith(
      "pc1",
      "EN_MANTENIMIENTO",
      undefined
    )
  })

  /**
   * Si el cambio falla, el mensaje va DENTRO del recuadro de confirmación.
   * Estaba arriba del listado entero: con un carro de treinta máquinas, un
   * error sobre la número 27 salía fuera de la pantalla y "Confirmar cambio"
   * parecía no hacer nada.
   */
  it("si el cambio de estado falla, lo dice al lado del botón", async () => {
    vi.mocked(adminApi.cambiarEstadoEquipo).mockRejectedValue(
      new ApiError(409, "ese equipo está prestado")
    )
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /En mantenimiento/i }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    const mensaje = await screen.findByText("ese equipo está prestado")
    // El recuadro sigue abierto y el mensaje está adentro: en el mismo panel
    // que el botón que se apretó, no en otra parte de la página.
    const panel = screen
      .getByRole("button", { name: "Confirmar cambio" })
      .closest(".rounded-md.border")
    expect(panel).toContainElement(mensaje)
  })

  // Un equipo fuera de servicio vuelve a circulación cuando se arregla.
  it("un equipo fuera de servicio puede volver a disponible", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [equipo({ estado: "FUERA_DE_SERVICIO" })],
    })
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /→ Disponible/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(adminApi.cambiarEstadoEquipo).toHaveBeenCalledWith(
      "pc1",
      "DISPONIBLE",
      undefined
    )
  })

  it("no ofrece cambiar al estado en el que el equipo ya está", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await screen.findByText(/PC 1/)

    expect(screen.queryByRole("button", { name: /→ Disponible/ })).not.toBeInTheDocument()
  })

  // RF-03.9: la baja dispara la misma cascada.
  it("dar de baja un equipo pide confirmación", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))

    expect(adminApi.darDeBajaEquipo).not.toHaveBeenCalled()
    expect(screen.getByText(/la saca del inventario/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))
    expect(adminApi.darDeBajaEquipo).toHaveBeenCalledWith("pc1")
  })

  it("los equipos ya dados de baja no se listan", async () => {
    vi.mocked(inventoryApi.listarEquiposDeCarro).mockResolvedValue({
      data: [
        equipo({ id: "pc1", identificador: 1 }),
        equipo({ id: "pc2", identificador: 2, dadoDeBaja: true }),
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    expect(await screen.findByText(/PC 1/)).toBeInTheDocument()
    expect(screen.queryByText(/PC 2/)).not.toBeInTheDocument()
  })

  // ── Alta de equipos (RF-03.2) ────────────────────────────────────────────
  // Es la operación que faltaba y sin la cual un despliegue limpio no sirve
  // para nada: se podía crear el carro pero no meterle un solo equipo, y sin
  // equipos nadie puede reservar.

  it("agrega un equipo al carro", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.type(await screen.findByLabelText("Número de máquina"), "7")
    // Con letras: es como son los de fábrica, y exigirle dígitos era lo que
    // impedía cargar un equipo con el dato real de la etiqueta.
    await user.type(screen.getByLabelText("Número de serie"), "PF2K9L3M")
    await user.type(screen.getByLabelText("Software instalado"), "AutoCAD 2027")
    await user.click(screen.getByRole("button", { name: "Agregar al carro" }))

    expect(adminApi.crearEquipoDeCarro).toHaveBeenCalledWith("c1", {
      identificador: 7,
      numeroSerie: "PF2K9L3M",
      freezado: false,
      cpu: undefined,
      ram: undefined,
      sistemaOperativo: undefined,
      softwareInstalado: "AutoCAD 2027",
    })
  })

  // El número de máquina SÍ es entero: es el que está pintado en la máquina y
  // nombra el zócalo que ocupa en el carro.
  it("no deja agregar un equipo con identificador no numérico", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.type(await screen.findByLabelText("Número de máquina"), "siete")
    await user.type(screen.getByLabelText("Número de serie"), "PF2K9L3M")

    expect(screen.getByText(/va sin letras/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Agregar al carro" })).toBeDisabled()
  })

  // Y el número de serie NO: la regla vale en la otra dirección, y esta es la
  // que rompía.
  it("acepta un número de serie con letras", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.type(await screen.findByLabelText("Número de máquina"), "7")
    await user.type(screen.getByLabelText("Número de serie"), "5CD1234ABC")

    expect(screen.queryByText(/va sin letras/)).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Agregar al carro" })).toBeEnabled()
  })

  // ── Edición de Equipos (RF-03.4 / RF-03.10) ──────────────────────────────

  it("edita el software instalado de un equipo", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    const software = screen.getByLabelText("Software instalado")
    await user.clear(software)
    await user.type(software, "Office 2021")
    await user.click(screen.getByRole("button", { name: "Guardar cambios" }))

    expect(adminApi.editarEquipo).toHaveBeenCalledWith("pc1", {
      carroId: undefined, // no se movió de carro
      freezado: false,
      cpu: undefined,
      ram: undefined,
      sistemaOperativo: undefined,
      softwareInstalado: "Office 2021",
    })
  })

  // RF-03.10
  it("mueve un equipo a otro carro", async () => {
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [
        { id: "c1", nombre: "Carro 1" },
        { id: "c2", nombre: "Carro 2" },
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click(
      (await screen.findAllByRole("button", { name: "Gestionar equipos" }))[0]
    )

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.selectOptions(screen.getByLabelText("Carro"), "c2")

    expect(screen.getByText(/se va a mover de carro/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Guardar cambios" }))

    expect(adminApi.editarEquipo).toHaveBeenCalledWith(
      "pc1",
      expect.objectContaining({ carroId: "c2" })
    )
  })

  // El identificador y el número de serie identifican al equipo: el PATCH
  // del backend no los acepta, así que la pantalla no los ofrece.
  it("al editar no ofrece cambiar identificador ni número de serie", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Editar" }))

    expect(screen.queryByLabelText("Número de máquina")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("Número de serie")).not.toBeInTheDocument()
  })

  // ── Carro (RF-03.1) ──────────────────────────────────────────────────

  it("renombra un carro", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Editar carro" }))
    const nombre = screen.getByLabelText("Nombre del carro")
    await user.clear(nombre)
    await user.type(nombre, "Carro de informática")
    await user.click(screen.getByRole("button", { name: "Guardar" }))

    expect(adminApi.editarCarro).toHaveBeenCalledWith("c1", {
      nombre: "Carro de informática",
      descripcion: "",
    })
  })

  // ── Incidencias (RF-03.5 / RF-03.6) ────────────────────────────────── El
  // ciclo de vida completo existía en el backend y no se podía tocar desde
  // ningún lado: los reportes de RF-06 mostraban estadísticas de incidencias
  // que nadie podía cargar ni cerrar.

  it("lista las incidencias de un equipo", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    expect(await screen.findByText("No arranca")).toBeInTheDocument()
    expect(screen.getByText("Grave")).toBeInTheDocument()
    expect(screen.getByText("Abierta")).toBeInTheDocument()
  })

  it("cierra una incidencia", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    await user.click(await screen.findByRole("button", { name: "→ Resuelta" }))

    expect(adminApi.editarIncidencia).toHaveBeenCalledWith("i1", {
      estado: "RESUELTA",
      marcarEnviadaASoporte: undefined,
    })
  })

  // RF-03.6: el envío a soporte guarda la fecha, por eso es su propia acción y
  // no un estado más.
  it("marca una incidencia como enviada a soporte técnico", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    await user.click(
      await screen.findByRole("button", { name: "Marcar enviada a soporte" })
    )

    expect(adminApi.editarIncidencia).toHaveBeenCalledWith("i1", {
      estado: undefined,
      marcarEnviadaASoporte: true,
    })
  })

  it("una incidencia ya enviada a soporte muestra la fecha y no vuelve a ofrecerlo", async () => {
    vi.mocked(inventoryApi.listarIncidenciasDeEquipo).mockResolvedValue({
      data: [
        incidencia({
          estado: "ENVIADA_A_SOPORTE",
          enviadoASoporte: true,
          fechaEnvioASoporte: "2026-08-02T10:00:00Z",
        }),
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    expect(
      await screen.findByText(/enviada a soporte el 02\/08\/2026/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Marcar enviada a soporte" })
    ).not.toBeInTheDocument()
  })

  it("muestra el error del backend tal cual", async () => {
    vi.mocked(adminApi.crearCarro).mockRejectedValue(
      new ApiError(409, "ya existe un carro con ese nombre")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.type(await screen.findByLabelText("Nombre"), "Carro 1")
    await user.click(screen.getByRole("button", { name: "Crear carro" }))

    expect(
      await screen.findByText("ya existe un carro con ese nombre")
    ).toBeInTheDocument()
  })

  /** Sacar una máquina de circulación cancela clases de otros docentes. */
  it("dice cuántas reservas canceló el cambio de estado", async () => {
    const user = userEvent.setup()
    vi.mocked(adminApi.cambiarEstadoEquipo).mockResolvedValue({
      reservasCanceladas: 2,
      docentesNotificados: 1,
    })
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Gestionar equipos" }))
    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(
      await screen.findByText(/Se cancelaron 2 reservas y se avisó a 1 docente/)
    ).toBeInTheDocument()
  })

  // El caso normal —la máquina no tenía nada reservado— no muestra nada:
  // "se cancelaron 0 reservas" es ruido en la operación de todos los días.
  it("no dice nada cuando no había reservas que cancelar", async () => {
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Gestionar equipos" }))
    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))
    await user.click(screen.getByRole("button", { name: "Confirmar cambio" }))

    expect(screen.queryByText(/Se cancelaron/)).not.toBeInTheDocument()
  })

  // RF-03.22: las cuentas se consultan desde Computadoras, pero se CARGAN
  // desde acá, que es donde un Admin viene a completar lo que se sabe de un
  // equipo. Sin este botón la función existía y no se encontraba.
  it("abre las cuentas de un equipo desde la gestión del inventario", async () => {
    const user = userEvent.setup()
    vi.mocked(inventoryApi.listarCuentasDeEquipo).mockResolvedValue({
      data: [
        {
          id: "cu1",
          equipoId: "pc1",
          usuario: "alumno",
          clase: "Local",
          privilegio: "COMUN",
          visibilidad: "PUBLICA",
          tienePassword: false,
          hayPasswordParaVer: false,
          puedeVerLaPassword: true,
        },
      ],
    })
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Cómo entrar" }))

    expect(await screen.findByText("Cómo entrar a PC 1")).toBeInTheDocument()
    expect(await screen.findByText("alumno")).toBeInTheDocument()
    // Y desde acá se pueden cargar, que es el motivo de que el botón exista.
    expect(screen.getByRole("button", { name: "Agregar cuenta" })).toBeInTheDocument()
    expect(inventoryApi.listarCuentasDeEquipo).toHaveBeenCalledWith("pc1")
  })
})
