import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { useAuth } from "@/features/auth/AuthContext"
import type { Estado, Usuario } from "@/features/auth/types"
import { UsuariosPage } from "@/features/admin/UsuariosPage"
import * as adminApi from "@/features/admin/api"
import { ApiError } from "@/lib/api-client"

vi.mock("@/features/admin/api")
vi.mock("@/features/auth/AuthContext")

const ADMIN: Usuario = {
  id: "admin1",
  nombre: "Admin",
  apellido: "Inicial",
  email: "admin@test.com",
  rol: "ADMIN",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

function usuario(over: Partial<Usuario> & { estado: Estado }): Usuario {
  return {
    id: "u1",
    nombre: "Ada",
    apellido: "Lovelace",
    email: "ada@test.com",
    rol: "DOCENTE",
    fechaRegistro: "2026-01-01T00:00:00Z",
    fechaAprobacion: null,
    debeCambiarPassword: false,
    ...over,
  }
}

function renderPagina() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <UsuariosPage />
    </QueryClientProvider>
  )
}

describe("UsuariosPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useAuth).mockReturnValue({
      user: ADMIN,
      isLoading: false,
      errorDeSesion: null,
      motivoDeCierre: null,
      login: vi.fn(),
      logout: vi.fn(),
      loginConGoogle: vi.fn(),
      refetchUser: vi.fn(),
    })
    vi.mocked(adminApi.cambiarEstadoUsuario).mockResolvedValue(undefined)
    vi.mocked(adminApi.eliminarUsuario).mockResolvedValue(undefined)
    vi.mocked(adminApi.promoverAAdmin).mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  function conUsuarios(...us: Usuario[]) {
    vi.mocked(adminApi.listarUsuarios).mockResolvedValue({
      data: us,
      meta: { total: us.length, page: 1, pageSize: us.length },
    })
  }

  it("lista los usuarios con su estado", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    renderPagina()

    expect(await screen.findByText(/Ada Lovelace/)).toBeInTheDocument()
    expect(screen.getByText("Aprobada")).toBeInTheDocument()
  })

  // RF-02.9: la baja es permanente, no existe reactivar.
  it("dar de baja pide confirmación y avisa que es permanente", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))

    expect(adminApi.cambiarEstadoUsuario).not.toHaveBeenCalled()
    expect(screen.getByText(/es permanente/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar" }))
    expect(adminApi.cambiarEstadoUsuario).toHaveBeenCalledWith("u1", "BAJA")
  })

  // RF-01.9: eliminar es hard delete y solo se permite desde BAJA.
  it("solo ofrece eliminar definitivamente a una cuenta en BAJA", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    renderPagina()
    await screen.findByText(/Ada Lovelace/)

    expect(
      screen.queryByRole("button", { name: "Eliminar definitivamente" })
    ).not.toBeInTheDocument()
  })

  it("eliminar pide confirmación y explica que libera el email", async () => {
    conUsuarios(usuario({ estado: "BAJA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Eliminar definitivamente" })
    )

    expect(adminApi.eliminarUsuario).not.toHaveBeenCalled()
    expect(screen.getByText(/libera el email/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar" }))
    expect(adminApi.eliminarUsuario).toHaveBeenCalledWith("u1")
  })

  // Darse de baja a uno mismo dejaría al Admin fuera del sistema en el acto.
  it("no ofrece darse de baja a uno mismo", async () => {
    conUsuarios(usuario({ id: "admin1", estado: "APROBADA", rol: "ADMIN" }))
    renderPagina()
    await screen.findByText(/\(vos\)/)

    expect(screen.queryByRole("button", { name: "Dar de baja" })).not.toBeInTheDocument()
  })

  // RF-01.6: no hay mails, el Admin comunica la temporal a mano.
  it("al resetear la contraseña muestra la temporal una sola vez", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    vi.mocked(adminApi.resetearPassword).mockResolvedValue({
      passwordTemporal: "Xy7-temporal",
    })
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Resetear contraseña" }))

    expect(await screen.findByText("Xy7-temporal")).toBeInTheDocument()
    expect(screen.getByText(/no se vuelve a mostrar/)).toBeInTheDocument()
  })

  // RF-01.8: el backend rechaza dejar el sistema sin Admins; el mensaje
  // tiene que llegarle al usuario tal cual.
  it("muestra el error del backend al intentar dar de baja al último Admin", async () => {
    conUsuarios(usuario({ estado: "APROBADA", rol: "ADMIN" }))
    vi.mocked(adminApi.cambiarEstadoUsuario).mockRejectedValue(
      new ApiError(409, "no se puede dejar la institución sin ningún Admin activo")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Dar de baja" }))
    await user.click(screen.getByRole("button", { name: "Confirmar" }))

    expect(await screen.findByText(/sin ningún Admin activo/)).toBeInTheDocument()
  })

  // ── Crear otro Admin (RF-01.4) ───────────────────────────────────────
  //
  // Sin esta pantalla la institución se quedaba con la única cuenta que
  // siembra el arranque: el autorregistro crea DOCENTE y no hay ningún
  // endpoint que cambie el rol de una cuenta existente.

  it("crea otro Admin", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    vi.mocked(adminApi.crearAdmin).mockResolvedValue(undefined)
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Crear otro Admin" }))
    await user.type(screen.getByLabelText("Nombre"), "Grace")
    await user.type(screen.getByLabelText("Apellido"), "Hopper")
    await user.type(screen.getByLabelText("Email"), "grace@escuela.edu.ar")
    await user.type(screen.getByLabelText("Contraseña inicial"), "password123")
    await user.click(screen.getByRole("button", { name: "Crear Admin" }))

    expect(adminApi.crearAdmin).toHaveBeenCalledWith({
      nombre: "Grace",
      apellido: "Hopper",
      email: "grace@escuela.edu.ar",
      password: "password123",
    })
    // No hay envío de mails: quien la crea le tiene que pasar la contraseña.
    expect(await screen.findByText(/se la tenés que pasar vos/)).toBeInTheDocument()
  })

  it("no deja crear un Admin con una contraseña más corta que el mínimo", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Crear otro Admin" }))
    await user.type(screen.getByLabelText("Nombre"), "Grace")
    await user.type(screen.getByLabelText("Apellido"), "Hopper")
    await user.type(screen.getByLabelText("Email"), "grace@escuela.edu.ar")
    await user.type(screen.getByLabelText("Contraseña inicial"), "corta")

    expect(screen.getByText(/al menos 8 caracteres/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Crear Admin" })).toBeDisabled()
  })

  it("muestra el error del backend al crear un Admin", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    vi.mocked(adminApi.crearAdmin).mockRejectedValue(
      new ApiError(409, "email ya registrado")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Crear otro Admin" }))
    await user.type(screen.getByLabelText("Nombre"), "Grace")
    await user.type(screen.getByLabelText("Apellido"), "Hopper")
    await user.type(screen.getByLabelText("Email"), "admin@test.com")
    await user.type(screen.getByLabelText("Contraseña inicial"), "password123")
    await user.click(screen.getByRole("button", { name: "Crear Admin" }))

    expect(await screen.findByText("email ya registrado")).toBeInTheDocument()
  })

  it("filtrar por estado vuelve a consultar con ese filtro", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()
    await screen.findByText(/Ada Lovelace/)

    await user.selectOptions(screen.getByLabelText("Estado"), "BAJA")

    // page va siempre: cambiar el filtro además vuelve a la primera página,
    // porque la anterior puede caer más allá del final de la nueva colección.
    expect(adminApi.listarUsuarios).toHaveBeenLastCalledWith({ estado: "BAJA", page: 1 })
  })

  // ── Promover a admin ──────────────────────────────────────────────────

  // Dar permisos de Admin no se puede deshacer desde el sistema, así que
  // pide confirmación igual que una baja aunque no destruya nada.
  it("promover pide confirmación y avisa que no se puede deshacer", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Promover a admin" }))

    expect(adminApi.promoverAAdmin).not.toHaveBeenCalled()
    expect(screen.getByText(/No se puede deshacer/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar" }))
    expect(adminApi.promoverAAdmin).toHaveBeenCalledWith("u1")
  })

  it("se puede volver atrás sin promover", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Promover a admin" }))
    await user.click(screen.getByRole("button", { name: "Volver" }))

    expect(adminApi.promoverAAdmin).not.toHaveBeenCalled()
  })

  // El backend rechaza promover a quien ya es Admin: ofrecer un botón que
  // siempre falla es peor que no ofrecerlo.
  it("no ofrece promover a una cuenta que ya es ADMIN", async () => {
    conUsuarios(usuario({ estado: "APROBADA", rol: "ADMIN" }))
    renderPagina()
    await screen.findByText(/Ada Lovelace/)

    expect(
      screen.queryByRole("button", { name: "Promover a admin" })
    ).not.toBeInTheDocument()
  })

  // Promover una cuenta PENDIENTE sería aprobarla por la puerta de atrás.
  it("no ofrece promover a una cuenta que no está aprobada", async () => {
    conUsuarios(usuario({ estado: "PENDIENTE" }))
    renderPagina()
    await screen.findByText(/Ada Lovelace/)

    expect(
      screen.queryByRole("button", { name: "Promover a admin" })
    ).not.toBeInTheDocument()
  })

  it("muestra el mensaje del backend si la promoción falla", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    vi.mocked(adminApi.promoverAAdmin).mockRejectedValue(
      new ApiError(409, "no se puede promover esta cuenta a ADMIN: ya tiene rol ADMIN")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Promover a admin" }))
    await user.click(screen.getByRole("button", { name: "Confirmar" }))

    expect(await screen.findByText(/ya tiene rol ADMIN/)).toBeInTheDocument()
  })
})
