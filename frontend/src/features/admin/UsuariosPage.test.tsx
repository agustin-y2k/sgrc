import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { MemoryRouter } from "react-router"
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
      {/* La ficha de lo que declaró una cuenta pendiente enlaza a Académico y
          a Usuarios, así que la página necesita un router. */}
      <MemoryRouter>
        <UsuariosPage />
      </MemoryRouter>
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
    vi.mocked(adminApi.degradarADocente).mockResolvedValue(undefined)
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

  // RF-01.9: eliminar es hard delete y solo se permite sobre una cuenta ya
  // cerrada — los dos estados terminales, BAJA y RECHAZADA.
  it.each(["PENDIENTE", "APROBADA"] as const)(
    "no ofrece eliminar definitivamente a una cuenta en %s",
    async (estado) => {
      conUsuarios(usuario({ estado }))
      renderPagina()
      await screen.findByText(/Ada Lovelace/)

      expect(
        screen.queryByRole("button", { name: "Eliminar definitivamente" })
      ).not.toBeInTheDocument()
    }
  )

  // RECHAZADA tiene que ofrecerlo: es la única salida para un rechazo
  // equivocado, porque esa cuenta no transiciona a ningún otro estado.
  it.each(["BAJA", "RECHAZADA"] as const)(
    "ofrece eliminar definitivamente a una cuenta en %s",
    async (estado) => {
      conUsuarios(usuario({ estado }))
      renderPagina()

      expect(
        await screen.findByRole("button", { name: "Eliminar definitivamente" })
      ).toBeInTheDocument()
    }
  )

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

  // ── Crear otro Admin (RF-01.4) ─────────────────────────────────────── Sin
  // esta pantalla la institución se quedaba con la única cuenta que siembra
  // el arranque: el autorregistro crea DOCENTE y no hay ningún endpoint que
  // cambie el rol de una cuenta existente.

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

  // Dar permisos de Admin cambia quién puede tocar el inventario, el ciclo
  // lectivo y las cuentas de los demás, así que pide confirmación igual que
  // una baja aunque no destruya nada.
  it("promover pide confirmación y explica qué permisos da", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(await screen.findByRole("button", { name: "Promover a admin" }))

    expect(adminApi.promoverAAdmin).not.toHaveBeenCalled()
    expect(screen.getByText(/aprobar cuentas/)).toBeInTheDocument()

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

  // ── Quitar permisos de admin ──────────────────────────────────────────

  it("quitar permisos pide confirmación y aclara que la cuenta sigue abierta", async () => {
    conUsuarios(usuario({ estado: "APROBADA", rol: "ADMIN" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Quitar permisos de admin" })
    )

    expect(adminApi.degradarADocente).not.toHaveBeenCalled()
    expect(screen.getByText(/La cuenta sigue abierta/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Confirmar" }))
    expect(adminApi.degradarADocente).toHaveBeenCalledWith("u1")
  })

  it("se puede volver atrás sin quitar los permisos", async () => {
    conUsuarios(usuario({ estado: "APROBADA", rol: "ADMIN" }))
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Quitar permisos de admin" })
    )
    await user.click(screen.getByRole("button", { name: "Volver" }))

    expect(adminApi.degradarADocente).not.toHaveBeenCalled()
  })

  // Quien se quitara los permisos perdería en el acto esta misma pantalla y
  // dependería de otro Admin para volver atrás.
  it("no ofrece quitarse los permisos a uno mismo", async () => {
    conUsuarios(usuario({ id: "admin1", estado: "APROBADA", rol: "ADMIN" }))
    renderPagina()
    await screen.findByText(/\(vos\)/)

    expect(
      screen.queryByRole("button", { name: "Quitar permisos de admin" })
    ).not.toBeInTheDocument()
  })

  // Sobre un docente no hay permisos que quitar.
  it("no ofrece quitar permisos a un docente", async () => {
    conUsuarios(usuario({ estado: "APROBADA" }))
    renderPagina()
    await screen.findByText(/Ada Lovelace/)

    expect(
      screen.queryByRole("button", { name: "Quitar permisos de admin" })
    ).not.toBeInTheDocument()
  })

  // RF-01.8 lo decide el backend, que es el único que puede contar cuántos
  // Admins activos quedan: la pantalla muestra el motivo que llega.
  it("muestra el motivo cuando el backend rechaza dejar al sistema sin Admins", async () => {
    conUsuarios(usuario({ estado: "APROBADA", rol: "ADMIN" }))
    vi.mocked(adminApi.degradarADocente).mockRejectedValue(
      new ApiError(409, "no se puede dejar al sistema sin ningún admin activo")
    )
    const user = userEvent.setup()
    renderPagina()

    await user.click(
      await screen.findByRole("button", { name: "Quitar permisos de admin" })
    )
    await user.click(screen.getByRole("button", { name: "Confirmar" }))

    expect(
      await screen.findByText("no se puede dejar al sistema sin ningún admin activo")
    ).toBeInTheDocument()
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

  /**
   * Este atajo dejaba aprobar a ciegas: mostraba el nombre, el rol y el estado,
   * y el botón "Aprobar" al lado. Lo que la persona pidió dictar —el dato
   * sobre el que se decide— estaba solo en la pantalla de Aprobación, que se
   * había diseñado justamente para mostrarlo.
   */
  it("muestra qué pidió dictar antes de ofrecer aprobar", async () => {
    conUsuarios(
      usuario({
        estado: "PENDIENTE",
        materiaSolicitada: "Taller de Electrónica",
        cursoSolicitado: "4°A",
        rolSolicitado: "SUPLENTE",
      })
    )
    renderPagina()

    expect(await screen.findByText("Pidió dictar")).toBeInTheDocument()
    expect(
      screen.getByText(/Taller de Electrónica · 4°A · como suplente/)
    ).toBeInTheDocument()
  })

  /**
   * Un estado que ofrece la mitad de sus salidas obliga a irse a otra pantalla
   * para terminar lo que se empezó acá.
   */
  it("ofrece rechazar además de aprobar", async () => {
    conUsuarios(usuario({ estado: "PENDIENTE" }))
    renderPagina()

    expect(await screen.findByRole("button", { name: "Aprobar" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Rechazar" })).toBeInTheDocument()
  })
})
