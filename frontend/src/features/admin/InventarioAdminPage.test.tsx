import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { InventarioAdminPage } from "@/features/admin/InventarioAdminPage"
import * as adminApi from "@/features/admin/api"
import * as inventoryApi from "@/features/inventory/api"
import type { Incidencia, PC } from "@/features/inventory/types"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/admin/api")
vi.mock("@/features/inventory/api")

function pc(over: Partial<PC> = {}): PC {
  return {
    id: "pc1",
    carroId: "c1",
    identificador: 1,
    numeroSerie: 12345,
    freezado: false,
    estado: "DISPONIBLE",
    dadaDeBaja: false,
    fechaAlta: "2026-01-01T00:00:00Z",
    ...over,
  }
}

function incidencia(over: Partial<Incidencia> = {}): Incidencia {
  return {
    id: "i1",
    pcId: "pc1",
    descripcion: "No arranca",
    gravedad: "GRAVE",
    fecha: "2026-08-01T10:00:00Z",
    enviadoDge: false,
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
  await user.click(await screen.findByRole("button", { name: "Gestionar PCs" }))
}

describe("InventarioAdminPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [{ id: "c1", nombre: "Carro 1" }],
    })
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({ data: [pc()] })
    vi.mocked(adminApi.cambiarEstadoPC).mockResolvedValue(undefined)
    vi.mocked(adminApi.darDeBajaPC).mockResolvedValue(undefined)
    vi.mocked(adminApi.crearCarro).mockResolvedValue({ id: "c2", nombre: "Carro 2" })
    vi.mocked(adminApi.editarCarro).mockResolvedValue(undefined)
    vi.mocked(adminApi.crearPC).mockResolvedValue(pc({ id: "pc9", identificador: 9 }))
    vi.mocked(adminApi.editarPC).mockResolvedValue(undefined)
    vi.mocked(adminApi.editarIncidencia).mockResolvedValue(undefined)
    vi.mocked(inventoryApi.listarIncidenciasDePC).mockResolvedValue({
      data: [incidencia()],
    })
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

  // RF-03.8: sacar una PC de DISPONIBLE cancela sus reservas futuras y NO
  // se restauran al volver. Un solo click no puede alcanzar.
  it("cambiar a fuera de servicio pide confirmación y avisa de la cascada", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: /Fuera de servicio/ }))

    expect(adminApi.cambiarEstadoPC).not.toHaveBeenCalled()
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

    expect(adminApi.cambiarEstadoPC).toHaveBeenCalledWith(
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

    expect(adminApi.cambiarEstadoPC).toHaveBeenCalledWith(
      "pc1",
      "EN_MANTENIMIENTO",
      undefined
    )
  })

  it("no ofrece cambiar al estado en el que la PC ya está", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await screen.findByText(/PC 1/)

    expect(screen.queryByRole("button", { name: /→ Disponible/ })).not.toBeInTheDocument()
  })

  // RF-03.9: la baja dispara la misma cascada.
  it("dar de baja una PC pide confirmación", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))

    expect(adminApi.darDeBajaPC).not.toHaveBeenCalled()
    expect(screen.getByText(/la saca del inventario/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar baja" }))
    expect(adminApi.darDeBajaPC).toHaveBeenCalledWith("pc1")
  })

  it("las PCs ya dadas de baja no se listan", async () => {
    vi.mocked(inventoryApi.listarPCsDeCarro).mockResolvedValue({
      data: [
        pc({ id: "pc1", identificador: 1 }),
        pc({ id: "pc2", identificador: 2, dadaDeBaja: true }),
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    expect(await screen.findByText(/PC 1/)).toBeInTheDocument()
    expect(screen.queryByText(/PC 2/)).not.toBeInTheDocument()
  })

  // ── Alta de PCs (RF-03.2) ────────────────────────────────────────────
  //
  // Es la operación que faltaba y sin la cual un despliegue limpio no sirve
  // para nada: se podía crear el carro pero no meterle una sola PC, y sin
  // PCs nadie puede reservar.

  it("agrega una PC al carro", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.type(await screen.findByLabelText("Identificador"), "7")
    await user.type(screen.getByLabelText("Número de serie"), "998877")
    await user.type(screen.getByLabelText("Software instalado"), "AutoCAD 2027")
    await user.click(screen.getByRole("button", { name: "Agregar PC" }))

    expect(adminApi.crearPC).toHaveBeenCalledWith("c1", {
      identificador: 7,
      numeroSerie: 998877,
      freezado: false,
      cpu: undefined,
      ram: undefined,
      sistemaOperativo: undefined,
      softwareInstalado: "AutoCAD 2027",
    })
  })

  // Los dos son enteros (RF-03.2). Sin este chequeo, Number("abc") manda
  // NaN y el backend responde un 400 que no explica cuál de los dos campos
  // está mal.
  it("no deja agregar una PC con identificador no numérico", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.type(await screen.findByLabelText("Identificador"), "siete")
    await user.type(screen.getByLabelText("Número de serie"), "998877")

    expect(screen.getByText(/números enteros/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Agregar PC" })).toBeDisabled()
  })

  // ── Edición de PCs (RF-03.4 / RF-03.10) ──────────────────────────────

  it("edita el software instalado de una PC", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    const software = screen.getByLabelText("Software instalado")
    await user.clear(software)
    await user.type(software, "Office 2021")
    await user.click(screen.getByRole("button", { name: "Guardar cambios" }))

    expect(adminApi.editarPC).toHaveBeenCalledWith("pc1", {
      carroId: undefined, // no se movió de carro
      freezado: false,
      cpu: undefined,
      ram: undefined,
      sistemaOperativo: undefined,
      softwareInstalado: "Office 2021",
    })
  })

  // RF-03.10
  it("mueve una PC a otro carro", async () => {
    vi.mocked(inventoryApi.listarCarros).mockResolvedValue({
      data: [
        { id: "c1", nombre: "Carro 1" },
        { id: "c2", nombre: "Carro 2" },
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await user.click((await screen.findAllByRole("button", { name: "Gestionar PCs" }))[0])

    await user.click(await screen.findByRole("button", { name: "Editar" }))
    await user.selectOptions(screen.getByLabelText("Carro"), "c2")

    expect(screen.getByText(/se va a mover de carro/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Guardar cambios" }))

    expect(adminApi.editarPC).toHaveBeenCalledWith(
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

    expect(screen.queryByLabelText("Identificador")).not.toBeInTheDocument()
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

  // ── Incidencias (RF-03.5 / RF-03.6) ──────────────────────────────────
  //
  // El ciclo de vida completo existía en el backend y no se podía tocar
  // desde ningún lado: los reportes de RF-06 mostraban estadísticas de
  // incidencias que nadie podía cargar ni cerrar.

  it("lista las incidencias de una PC", async () => {
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
      marcarEnviadaDGE: undefined,
    })
  })

  // RF-03.6: el envío a DGE guarda la fecha, por eso es su propia acción y
  // no un estado más.
  it("marca una incidencia como enviada a DGE", async () => {
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    await user.click(await screen.findByRole("button", { name: "Marcar enviada a DGE" }))

    expect(adminApi.editarIncidencia).toHaveBeenCalledWith("i1", {
      estado: undefined,
      marcarEnviadaDGE: true,
    })
  })

  it("una incidencia ya enviada a DGE muestra la fecha y no vuelve a ofrecerlo", async () => {
    vi.mocked(inventoryApi.listarIncidenciasDePC).mockResolvedValue({
      data: [
        incidencia({
          estado: "ENVIADA_DGE",
          enviadoDge: true,
          fechaEnvioDge: "2026-08-02T10:00:00Z",
        }),
      ],
    })
    const user = userEvent.setup()
    renderPagina()
    await abrirCarro(user)
    await user.click(await screen.findByRole("button", { name: "Incidencias" }))

    expect(await screen.findByText(/enviada a DGE el 02\/08\/2026/)).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Marcar enviada a DGE" })
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
})
