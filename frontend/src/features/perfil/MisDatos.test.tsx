import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { useAuth } from "@/features/auth/AuthContext"
import type { Usuario } from "@/features/auth/types"
import { MisDatos } from "@/features/perfil/MisDatos"
import * as perfilApi from "@/features/perfil/api"
import { ApiError } from "@/lib/api-client"
import { getToken } from "@/lib/token-store"

vi.mock("@/features/perfil/api")
vi.mock("@/features/auth/AuthContext")

const DOCENTE: Usuario = {
  id: "docente1",
  nombre: "Ada",
  apellido: "Byron",
  email: "ada@escuela.edu.ar",
  rol: "DOCENTE",
  estado: "APROBADA",
  fechaRegistro: "2026-01-01T00:00:00Z",
  fechaAprobacion: null,
  debeCambiarPassword: false,
}

const refetchUser = vi.fn()

function montar(usuario: Usuario = DOCENTE) {
  vi.mocked(useAuth).mockReturnValue({
    user: usuario,
    isLoading: false,
    errorDeSesion: null,
    motivoDeCierre: null,
    login: vi.fn(),
    logout: vi.fn(),
    loginConGoogle: vi.fn(),
    refetchUser,
  })
  return render(<MisDatos usuario={usuario} />)
}

describe("MisDatos", () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it("muestra el nombre y el email sin abrir nada", () => {
    montar()
    expect(screen.getByText("Ada Byron")).toBeInTheDocument()
    expect(screen.getByText("ada@escuela.edu.ar")).toBeInTheDocument()
  })

  it("guarda el nombre nuevo y actualiza la sesión", async () => {
    vi.mocked(perfilApi.actualizarMisDatos).mockResolvedValue({
      usuario: { ...DOCENTE, apellido: "Lovelace" },
      token: "token-nuevo",
    })
    const usuario = userEvent.setup()
    montar()

    await usuario.click(screen.getByRole("button", { name: /cambiar mi nombre/i }))
    const apellido = screen.getByLabelText("Apellido")
    await usuario.clear(apellido)
    await usuario.type(apellido, "Lovelace")
    await usuario.click(screen.getByRole("button", { name: "Guardar" }))

    await waitFor(() =>
      expect(perfilApi.actualizarMisDatos).toHaveBeenCalledWith({
        nombre: "Ada",
        apellido: "Lovelace",
      })
    )
    // El token viejo lleva el nombre viejo en los claims.
    expect(getToken()).toBe("token-nuevo")
    // Sin esto, el saludo del inicio y el menú de arriba seguirían diciendo
    // "Ada Byron" hasta el próximo ingreso.
    expect(refetchUser).toHaveBeenCalled()
  })

  it("un nombre vacío no llega al servidor", async () => {
    const usuario = userEvent.setup()
    montar()

    await usuario.click(screen.getByRole("button", { name: /cambiar mi nombre/i }))
    await usuario.clear(screen.getByLabelText("Nombre"))
    await usuario.click(screen.getByRole("button", { name: "Guardar" }))

    expect(await screen.findByText("Requerido")).toBeInTheDocument()
    expect(perfilApi.actualizarMisDatos).not.toHaveBeenCalled()
  })

  it("si el servidor rechaza el cambio, lo dice y no cierra el formulario", async () => {
    // ApiError y no un Error pelado: getErrorMessage solo muestra el texto
    // del backend cuando llegó por la API.
    vi.mocked(perfilApi.actualizarMisDatos).mockRejectedValue(
      new ApiError(400, "el nombre y el apellido son obligatorios")
    )
    const usuario = userEvent.setup()
    montar()

    await usuario.click(screen.getByRole("button", { name: /cambiar mi nombre/i }))
    await usuario.click(screen.getByRole("button", { name: "Guardar" }))

    expect(
      await screen.findByText(/el nombre y el apellido son obligatorios/i)
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Nombre")).toBeInTheDocument()
  })

  it("cancelar descarta lo escrito", async () => {
    const usuario = userEvent.setup()
    montar()

    await usuario.click(screen.getByRole("button", { name: /cambiar mi nombre/i }))
    await usuario.clear(screen.getByLabelText("Nombre"))
    await usuario.type(screen.getByLabelText("Nombre"), "Grace")
    await usuario.click(screen.getByRole("button", { name: "Cancelar" }))

    expect(screen.getByText("Ada Byron")).toBeInTheDocument()

    // Al volver a abrirlo se ve lo guardado, no el borrador abandonado.
    await usuario.click(screen.getByRole("button", { name: /cambiar mi nombre/i }))
    expect(screen.getByLabelText("Nombre")).toHaveValue("Ada")
  })
})
