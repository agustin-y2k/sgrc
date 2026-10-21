import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { AuthProvider, useAuth } from "@/features/auth/AuthContext"
import * as authApi from "@/features/auth/api"
import type { Usuario } from "@/features/auth/types"
import { ApiError, apiFetch } from "@/lib/api-client"
import { queryClient } from "@/lib/query-client"
import { clearToken, getToken, setToken } from "@/lib/token-store"

vi.mock("@/features/auth/api")

const cuenta: Usuario = {
  id: "docente",
  nombre: "Ana",
  apellido: "Docente",
  email: "ana@test.local",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

function Probe() {
  const { user, logout, login } = useAuth()
  return (
    <>
      <span>{user?.email ?? "sin sesión"}</span>
      <button onClick={logout}>Salir</button>
      <button onClick={() => void login("otra@test.local", "password").catch(() => {})}>
        Ingresar
      </button>
    </>
  )
}

function diferida<T>() {
  let resolver!: (valor: T) => void
  const promesa = new Promise<T>((resolve) => {
    resolver = resolve
  })
  return { promesa, resolver }
}

describe("aislamiento de sesión", () => {
  beforeEach(() => {
    vi.resetAllMocks()
    clearToken()
    queryClient.clear()
  })
  afterEach(() => {
    vi.restoreAllMocks()
    clearToken()
    queryClient.clear()
  })

  it("cerrar sesión borra consultas y mutaciones privadas", async () => {
    setToken("token-ana")
    vi.mocked(authApi.me).mockResolvedValue(cuenta)
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await screen.findByText(cuenta.email)
    queryClient.setQueryData(["mis-sugerencias"], [{ texto: "consulta privada" }])
    queryClient.getMutationCache().build(queryClient, { mutationKey: ["reset-password"] })
    await userEvent.click(screen.getByText("Salir"))
    expect(getToken()).toBeNull()
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0)
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0)
  })

  it("una hidratación tardía no vuelve a abrir la sesión después de salir", async () => {
    setToken("token-ana")
    const pendiente = diferida<Usuario>()
    vi.mocked(authApi.me).mockReturnValue(pendiente.promesa)
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await waitFor(() => expect(authApi.me).toHaveBeenCalled())
    await userEvent.click(screen.getByText("Salir"))
    await act(async () => pendiente.resolver(cuenta))
    expect(screen.getByText("sin sesión")).toBeInTheDocument()
    expect(getToken()).toBeNull()
  })

  it("un 401 de la sesión anterior no cierra la nueva", async () => {
    setToken("token-ana")
    vi.mocked(authApi.me).mockResolvedValue(cuenta)
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await screen.findByText(cuenta.email)
    const pendiente = diferida<Response>()
    vi.spyOn(globalThis, "fetch").mockReturnValue(pendiente.promesa)
    const resultado = apiFetch("/api/mis-sugerencias").catch((error: unknown) => error)
    setToken("token-nuevo")
    await act(async () => {
      pendiente.resolver(
        new Response("sesión revocada", {
          status: 401,
          headers: { "X-Sesion-Motivo": "revocada" },
        })
      )
      await resultado
    })
    expect(await resultado).toBeInstanceOf(ApiError)
    expect(getToken()).toBe("token-nuevo")
  })

  it("cambiar de cuenta en otra pestaña elimina la caché y verifica la nueva identidad", async () => {
    setToken("token-ana")
    vi.mocked(authApi.me).mockResolvedValue(cuenta)
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>
    )
    await screen.findByText(cuenta.email)
    queryClient.setQueryData(["mis-sugerencias"], ["texto privado"])
    vi.mocked(authApi.me).mockResolvedValue({
      ...cuenta,
      id: "otra",
      email: "otra@test.local",
    })
    await act(async () => {
      setToken("token-otra")
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: "sgrc_token",
          oldValue: "token-ana",
          newValue: "token-otra",
          storageArea: localStorage,
        })
      )
    })
    await screen.findByText("otra@test.local")
    expect(queryClient.getQueryData(["mis-sugerencias"])).toBeUndefined()
  })
})
